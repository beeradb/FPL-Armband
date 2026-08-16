package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BootstrapEndpoint is the one endpoint a backfilled capture carries.
//
// A live capture also archives `/fixtures/`, which a backfilled one does not: the
// fixture list for a finished season is already in the season archive, complete and
// with results, so recovering a crawl of it would spend somebody else's bandwidth to
// obtain a worse copy of something on disk. What is *not* anywhere else, and is the
// entire reason this exists, is the availability block inside bootstrap-static.
const BootstrapEndpoint = "/bootstrap-static/"

// SeasonDir is where a backfilled season's captures live: one directory per season
// under the same root as the live series.
//
// Nested rather than flat, for two reasons that both bite in practice. `capture
// -list` reports the gap between consecutive captures so a stalled weekly series is
// visible, and six seasons of history interleaved into that list would show a
// three-month "gap" every summer that means nothing. And a flat directory keyed only
// by timestamp cannot answer "which season is this" without opening every manifest.
//
// `List` skips any directory whose name is not a capture timestamp, so adding these
// does not disturb the existing live series or the command that reads it.
func SeasonDir(root, season string) string {
	return filepath.Join(root, season)
}

// BackfillDirName names a recovered capture's directory: the gameweek it is evidence
// for, then when it was crawled.
//
// # Why this differs from the live series, which is timestamp-only
//
// A live capture is taken *for* the deadline it is nearest, one per week, so its
// timestamp identifies it. A recovered one cannot rely on that. Where the Archive
// crawled FPL twice in a fortnight — which happens, coverage runs from two days in
// three to nearly daily depending on the season — the last crawl before GW12's
// deadline can also be the last crawl before GW13's, and it is honest evidence for
// both. Under timestamp-only naming the second write collides with the first and one
// of the two gameweeks silently loses its row.
//
// Leading the name with a zero-padded gameweek also means `ls` sorts by gameweek and
// a hole is visible as a hole, which is the property the coverage report exists to
// preserve and is worth having in the filesystem too.
func BackfillDirName(event int, at time.Time) string {
	return fmt.Sprintf("GW%02d-%s", event, DirName(at))
}

// CapturedAtOf reads back when an existing capture was crawled.
//
// It exists so a caller can tell "this exact crawl is already stored" from "a
// different crawl collides with it". Directory names are minute-resolution while
// crawl times are second-resolution, so two crawls a few seconds apart land on one
// name — rare, and reachable only at a denser cadence, but the difference between
// "already have it" and "silently dropped one" is not one to infer from a path.
func CapturedAtOf(dir string) (time.Time, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return time.Time{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return time.Time{}, err
	}
	return m.CapturedAt.UTC(), nil
}

// Backfilled is one recovered capture, ready to be written.
type Backfilled struct {
	Root    string // the captures root, e.g. data/captures
	Season  string // "2020-21"
	Event   int    // the gameweek whose deadline this capture is evidence for
	Body    []byte // the uncompressed bootstrap-static JSON
	At      time.Time
	Source  string // the exact URL fetched
	Stamp   string // the Archive's own crawl identifier
	Digest  string // the Archive's content hash
	Version string // the armband revision doing the backfill
}

// Write stores a recovered capture in the live capture layout, refusing any that
// cannot be shown to predate its deadline.
//
// # The refusal is the point
//
// A backfilled capture is only worth anything if it is honest about time, and the
// failure mode is silent: a snapshot crawled an hour *after* a deadline contains
// team news the manager did not have, produces entirely plausible figures, and
// nothing downstream can tell. So the check happens here, at the only place bytes
// enter the store, rather than at the point of use where it could be forgotten. A
// capture that fails it is not stored at all — not stored-with-a-warning, because
// this package has twice shipped a repair that silently applied nothing, and a
// quarantined row that later reads as data is the same shape.
func (b Backfilled) Write() (string, *Manifest, error) {
	q, err := VerifyPreDeadline(b.Body, b.Event, b.At)
	if err != nil {
		return "", nil, fmt.Errorf("refusing to store a GW%d capture for %s: %w",
			b.Event, b.Season, err)
	}

	dir := filepath.Join(SeasonDir(b.Root, b.Season), BackfillDirName(b.Event, b.At))
	if _, err := os.Stat(dir); err == nil {
		return "", nil, fmt.Errorf("capture %s already exists: a capture is evidence about "+
			"a moment and is never overwritten", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}

	sum := sha256.Sum256(b.Body)
	name := FileName(BootstrapEndpoint)
	stored, err := writeGz(filepath.Join(dir, name), b.Body)
	if err != nil {
		return "", nil, fmt.Errorf("writing %s: %w", name, err)
	}

	hours := q.Deadline.Sub(b.At.UTC()).Hours()
	deadline := q.Deadline
	m := &Manifest{
		CapturedAt:      b.At.UTC(),
		Event:           b.Event,
		EventDeadline:   &deadline,
		HoursToDeadline: &hours,
		Version:         b.Version,
		Files: []File{{
			Endpoint: BootstrapEndpoint, Name: name,
			Bytes: len(b.Body), Stored: stored,
			SHA256: hex.EncodeToString(sum[:]),
		}},
		Backfill: &Backfill{
			Season: b.Season, Source: b.Source,
			WaybackTimestamp: b.Stamp, Digest: b.Digest,
		},
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
		return "", nil, err
	}
	return dir, m, nil
}

// Quality is what a capture's own payload says about how fresh it was.
type Quality struct {
	// Deadline is the target gameweek's deadline, read from this payload rather
	// than from anywhere else, so the manifest cannot describe a moment the bytes
	// beside it disagree with.
	Deadline time.Time

	// HoursBefore is how long before that deadline the crawl happened. Always
	// positive: a non-positive value is refused, not recorded.
	HoursBefore float64

	// EventsBehind is how many deadlines had ALREADY passed between this crawl
	// and the target one — 0 when the payload's own `is_next` is the target
	// gameweek, 1 when the crawl predates the previous gameweek's deadline too,
	// and so on. **-1 means the payload named no next gameweek at all**, which is
	// unknown rather than fresh — distinguished because a zero that quietly means
	// "could not tell" is the shape that has cost this project real measurements.
	//
	// This is a better staleness measure than the hours, and it is the one to
	// read first. Twenty hours before a deadline is fresh in a normal week and
	// nine days before it is not, but "nine days" alone does not say whether the
	// intervening week had a gameweek in it — over an international break it did
	// not, and the team news is as good as it ever was. EventsBehind answers that
	// directly, from FPL's own bookkeeping, with no calendar arithmetic.
	EventsBehind int
}

// VerifyPreDeadline proves from the payload alone that a capture predates a
// gameweek's deadline.
//
// # Why it takes no external clock
//
// The obvious check is "the Wayback timestamp is before the deadline", and it is a
// good check, but it trusts a third party's crawler clock for the one property the
// whole exercise rests on. This check does not need it: FPL's payload carries its
// own calendar, so a body claiming to predate GW12's deadline can be interrogated
// about GW12's deadline, about whether GW12 had finished, and about which gameweek
// FPL itself considered next. A crawl taken after the deadline fails all three from
// the inside.
//
// The three are not redundant. `is_next` catches aiming at the wrong gameweek — the
// payload disagreeing about which decision it informs. The deadline comparison
// catches being late for the right one. `finished` catches a body from long after
// the fact, such as an end-of-season re-crawl that a naive timestamp match might
// otherwise accept.
func VerifyPreDeadline(boot []byte, event int, at time.Time) (Quality, error) {
	var q Quality
	events, err := ParseEvents(boot)
	if err != nil {
		return q, err
	}
	if len(events) == 0 {
		return q, fmt.Errorf("the payload carries no events[], so it cannot be dated " +
			"from the inside and is not usable as point-in-time evidence")
	}

	var target *Event
	next := 0
	for i := range events {
		if events[i].ID == event {
			target = &events[i]
		}
		if events[i].IsNext {
			next = events[i].ID
		}
	}
	if target == nil {
		return q, fmt.Errorf("the payload's events[] has no GW%d, so this body is not "+
			"about the season it was filed under", event)
	}

	q.Deadline = target.DeadlineTime.UTC()
	q.HoursBefore = q.Deadline.Sub(at.UTC()).Hours()

	if !at.UTC().Before(q.Deadline) {
		return q, fmt.Errorf("the crawl at %s is not strictly before the GW%d deadline "+
			"at %s (%.2f h), so it may carry team news the manager did not have",
			at.UTC().Format(time.RFC3339), event, q.Deadline.Format(time.RFC3339), -q.HoursBefore)
	}
	if target.Finished {
		return q, fmt.Errorf("the payload already reports GW%d as finished, so it was "+
			"served after the gameweek was played", event)
	}
	if next > event {
		return q, fmt.Errorf("the payload's own is_next is GW%d, which is past the GW%d "+
			"deadline this capture claims to precede", next, event)
	}
	if next > 0 {
		q.EventsBehind = event - next
	} else {
		q.EventsBehind = -1
	}
	return q, nil
}
