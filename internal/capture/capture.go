// Package capture archives the live FPL payload, dated and immutable, so that
// point-in-time questions become answerable later.
//
// # Why this exists
//
// The season archive this project replays is an **end-of-season shape**, and every
// question it now cares about is a mid-season one. Four are blocked on precisely
// that and no amount of cleverness unblocks them:
//
//   - re-deriving `BlendMinutesK`, which needs the injuries that *resolve* — a player
//     out from September to November finishes the season fit and looks fine
//     throughout, so an end-of-season snapshot cannot show him;
//   - calibrating `availabilityFactor`: does a 75% flag really correspond to about
//     75% of a healthy player's minutes? `players_raw.csv` carries
//     `chance_of_playing` as one value per player per season, and `merged_gw.csv`
//     carries no availability columns at all — both checked against the headers
//     rather than assumed;
//   - the availability *trajectory*, whose transient-absence half is the part worth
//     the most and is unmeasurable on the archive;
//   - penalty duty, where `penalties_order` is an end-of-season snapshot, so a
//     January change reads as duty from GW1.
//
// The quantity all four serve is the largest information bound in the record, and
// every figure this comment has carried for it has since been corrected — three times
// now, which is itself the argument for capturing the data rather than re-arguing the
// estimate. **Nothing in the family resolves.** Against a common baseline on one
// 24-cell grid: perfect *lineups* — who starts, who is benched, who is absent — is
// **≈73 a season held (CR2 t = 1.32)**, perfect *minutes* is **≈47 (t = 0.62)**, and
// perfect team news, the binary case, is **≈14 (t = 3.19)**, which reaches its own
// threshold of 14 but is p = 0.0497 restated rather than a second witness and fails
// Holm at 0.149. Perfect price timing is +15, also unresolved.
//
// Two retracted figures are worth naming, because both were cited here as ground
// truth. The **+322** once given for team news was a raw twelve-cell total read as a
// season figure. The **+183** that replaced it as "the only oracle that resolves" is
// not a wrong number but a wrongly *labelled* one: it measures a **season-average**
// window and resolves as one, and it never bounded knowing the trajectory — which is
// the whole of what this capture is for. Do not reinstate it here.
//
// The ordering is why this capture is worth having. The value is not in "never buy a
// player who has left" — the archive can already reconstruct most of that — but in
// rotation, cameos, lost places and the injuries that *resolve*, which is exactly the
// population an end-of-season snapshot cannot show and a live weekly capture can.
//
// # Why it is safe where an external mirror was not
//
// This project deliberately rejected a community CSV mirror for live use, because a
// previous one stopped updating mid-life and stale minutes are *worse than none* —
// they report a dropped player as still starting. This is the opposite arrangement:
// our own capture of our own live fetch, kept for later analysis, and **never read by
// the live path**. Nothing here is a cache. An immutable finished record is a
// different risk profile from a live weekly signal, which the research record already
// draws as a distinction.
//
// # What it does not do
//
// It yields nothing this season. By next spring there is a mid-season shape for the
// four questions above; by the season after, a full season of our own history to
// calibrate against. That is the cost of not having started earlier, and it is the
// argument for starting now rather than when convenient.
package capture

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Endpoints are the paths captured, in a fixed order so a manifest diffs cleanly.
//
// Both are public and both are already fetched by every ordinary run, so a capture
// costs two requests. `bootstrap-static` is where the availability fields live —
// `status`, `chance_of_playing_next_round`, `chance_of_playing_this_round`, `news`,
// `news_added` — alongside prices, `cost_change_*`, `selected_by_percent` and
// transfers in and out, every one of them point-in-time correct at the moment of
// capture and none of them recoverable afterwards.
//
// `element-summary/{id}` is deliberately **not** here. It would be 500-odd requests
// for per-player history that is cumulative and therefore reconstructable from the
// weekly `bootstrap-static` series, so it multiplies the cost by 250 to record
// something the capture already implies.
//
// Spelled through the two constants rather than as literals, so the endpoint names have
// one spelling in this package. A capture written under one spelling and read under
// another is a miss that looks exactly like an absent capture.
var Endpoints = []string{
	BootstrapEndpoint,
	FixturesEndpoint,
}

// Fetcher is the one method a capture needs. Narrow on purpose: an interface this
// small can be faked in a test without a network, and it makes it structurally
// impossible for a capture to reach an authenticated endpoint through some other
// method on the client.
type Fetcher interface {
	Raw(ctx context.Context, path string) ([]byte, error)
}

// File records one archived body.
type File struct {
	Endpoint string `json:"endpoint"`
	Name     string `json:"name"`
	Bytes    int    `json:"bytes"`          // uncompressed
	Stored   int    `json:"stored_bytes"`   // on disk, gzipped
	SHA256   string `json:"sha256"`         // of the UNCOMPRESSED body
	Note     string `json:"note,omitempty"` // set when the endpoint failed
}

// Manifest describes a capture. It is written inside the directory as well as being
// implied by the directory's name, because a path can be renamed, copied or
// reconstructed by hand and then it is no longer evidence of anything.
type Manifest struct {
	CapturedAt time.Time `json:"captured_at"`
	Files      []File    `json:"files"`

	// Deadline context, which is the whole reason to capture *before* a deadline
	// rather than at a convenient hour. The information state that matters is the
	// one a decision is taken in.
	Event           int        `json:"event,omitempty"`
	EventDeadline   *time.Time `json:"event_deadline,omitempty"`
	HoursToDeadline *float64   `json:"hours_to_deadline,omitempty"`

	// Provenance, so a capture can be tied to the code that took it the way a
	// snapshot is. Cheap to record and impossible to recover later.
	Version string `json:"fplagent_version,omitempty"`

	// Backfill is set only on a capture recovered from the Internet Archive, and
	// nil on one this project took itself.
	//
	// A pointer rather than a bare struct so that the distinction survives a JSON
	// round-trip in both directions: an old manifest written before this field
	// existed reads back as nil, which is the truth about it, and a live capture
	// does not grow an empty block implying somebody tried. The two provenances
	// are genuinely different evidence — one is our own fetch, the other is a
	// third party's crawl of a page we did not see — and a reader that cannot
	// tell them apart would be unable to say which.
	Backfill *Backfill `json:"backfill,omitempty"`
}

// Backfill records where a recovered capture came from.
//
// Recorded in full rather than as a flag because the claim "this is FPL's payload
// from 11 September 2020" is only as good as its provenance, and the whole of the
// provenance is a URL, a crawl timestamp and a content digest. All three are cheap
// now and unrecoverable later.
type Backfill struct {
	// Season is the FPL season this capture belongs to, as the archive names it:
	// "2020-21". Present because a capture's directory can be moved and the
	// deadline alone does not name a season.
	Season string `json:"season"`

	// Source is the exact URL fetched, including the `id_` marker that asks the
	// Archive for the unrewritten original bytes.
	Source string `json:"source"`

	// WaybackTimestamp is the Archive's own identifier for this crawl, in its
	// `20060102150405` form. Kept as the string it serves rather than only as a
	// parsed time, so a refetch can be requested byte-for-byte.
	WaybackTimestamp string `json:"wayback_timestamp"`

	// Digest is the Archive's content hash for the crawl. Stored so that a later
	// refetch can be checked for having returned the same body — the Archive is a
	// third party and this is the only handle on whether its answer moved.
	Digest string `json:"digest,omitempty"`
}

// Event is one gameweek as bootstrap-static describes it: when its deadline was and
// where the season had got to when the payload was served.
//
// Parsed from the captured bytes rather than from a second fetch, so the manifest
// cannot describe a different moment from the files beside it.
//
// `DeadlineTime` is FPL's own field and is the authority on when a decision had to
// be taken — no other source in this project carries it, and the season archive
// carries kickoff times instead, which are a different quantity an hour and a half
// later.
type Event struct {
	ID           int       `json:"id"`
	DeadlineTime time.Time `json:"deadline_time"`
	Finished     bool      `json:"finished"`
	IsNext       bool      `json:"is_next"`
	IsCurrent    bool      `json:"is_current"`
}

// ParseEvents reads the gameweek calendar out of a bootstrap-static body.
//
// One implementation, used by the live capture path, the backfill and the honesty
// diagnostic alike. This project's signature failure is one quantity with two
// implementations where the measured one is not the one that runs, and a private
// copy of this parse living beside an exported one would be that shape exactly.
func ParseEvents(boot []byte) ([]Event, error) {
	var d struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(boot, &d); err != nil {
		return nil, fmt.Errorf("reading events from a bootstrap-static body: %w", err)
	}
	return d.Events, nil
}

// DirName is the capture directory's name: a UTC timestamp to the minute.
//
// UTC and not local, because a series of captures is a time series and a local
// clock changes by an hour twice a season. Sorting lexically then sorts
// chronologically, which is a property the snapshot directories deliberately do NOT
// have — theirs are date-then-commit, so a lexical sort looks chronological and is
// wrong the moment two share a day. Recorded there as a defect that shipped twice.
func DirName(at time.Time) string {
	return at.UTC().Format("2006-01-02T1504Z")
}

// Take archives every endpoint into a new dated directory under root.
//
// It refuses to write into a directory that already exists. A capture is evidence
// about a moment and overwriting one destroys the only copy — so a second capture in
// the same minute is an error rather than a silent replacement. That is the same
// reasoning as the R inference script refusing to overwrite another sweep's output.
//
// A failing endpoint does not abort the capture: the others are still evidence, and
// a partial capture that says which part is missing is worth much more than nothing.
// The failure is recorded in the manifest as a note on the file, never as a dropped
// entry — a dropped entry reads later as an endpoint nobody tried, which is a
// different and weaker claim that looks like a clean run. The same rule the sweep
// harness applies to an infeasible cell.
func Take(ctx context.Context, f Fetcher, root string, at time.Time, version string) (string, *Manifest, error) {
	dir := filepath.Join(root, DirName(at))
	if _, err := os.Stat(dir); err == nil {
		return "", nil, fmt.Errorf("capture %s already exists: a capture is evidence about a "+
			"moment and is never overwritten", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}

	m := &Manifest{CapturedAt: at.UTC(), Version: version}
	var boot []byte
	for _, ep := range Endpoints {
		name := FileName(ep)
		body, err := f.Raw(ctx, ep)
		if err != nil {
			m.Files = append(m.Files, File{Endpoint: ep, Name: name, Note: err.Error()})
			continue
		}
		if ep == "/bootstrap-static/" {
			boot = body
		}
		sum := sha256.Sum256(body)
		stored, err := writeGz(filepath.Join(dir, name), body)
		if err != nil {
			return "", nil, fmt.Errorf("writing %s: %w", name, err)
		}
		m.Files = append(m.Files, File{
			Endpoint: ep, Name: name, Bytes: len(body), Stored: stored,
			SHA256: hex.EncodeToString(sum[:]),
		})
	}

	if boot != nil {
		annotateDeadline(m, boot, at)
	}

	// The manifest is written last, so a killed capture leaves a directory with no
	// manifest rather than a manifest describing files that are not there. An
	// unmanifested directory is obviously incomplete; a lying manifest is not.
	if err := writeJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
		return "", nil, err
	}
	return dir, m, nil
}

// FileName is the stored name for an endpoint: its path, flattened, gzipped.
//
// `/bootstrap-static/` becomes `bootstrap-static.json.gz`. Leading and trailing
// slashes vanish and interior ones become underscores, so a future per-entry
// endpoint like `/element-summary/123/` lands on `element-summary_123.json.gz`
// rather than trying to create a subdirectory.
func FileName(endpoint string) string {
	return strings.ReplaceAll(strings.Trim(endpoint, "/"), "/", "_") + ".json.gz"
}

// annotateDeadline records which gameweek this capture sits against and how long
// was left. `is_next` is preferred over `is_current` because the decision a capture
// is meant to inform is the one taken at the *upcoming* deadline.
func annotateDeadline(m *Manifest, boot []byte, at time.Time) {
	events, err := ParseEvents(boot)
	if err != nil {
		return
	}
	pick := -1
	for i, e := range events {
		if e.IsNext {
			pick = i
			break
		}
	}
	if pick < 0 {
		for i, e := range events {
			if e.IsCurrent {
				pick = i
				break
			}
		}
	}
	if pick < 0 {
		return
	}
	e := events[pick]
	m.Event = e.ID
	dl := e.DeadlineTime.UTC()
	m.EventDeadline = &dl
	h := dl.Sub(at.UTC()).Hours()
	m.HoursToDeadline = &h
}

// writeGz stores body gzipped and returns the number of bytes on disk.
//
// Gzipped because a season is 38-odd captures of a payload approaching a megabyte,
// and because these are meant to be committed: this repository has no remote, so a
// worktree can be deleted with its session and an uncommitted capture is simply
// gone. Committing is the only durability available here, which makes size a real
// consideration rather than a tidiness one.
func writeGz(path string, body []byte) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	zw := gzip.NewWriter(f)
	if _, err := zw.Write(body); err != nil {
		f.Close()
		return 0, err
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return int(fi.Size()), nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// List returns the capture directory names under root, oldest first.
func List(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := time.Parse("2006-01-02T1504Z", e.Name()); err != nil {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out) // lexical is chronological; see DirName
	return out, nil
}

// Read returns a captured body, decompressed, from a capture directory.
func Read(dir, endpoint string) ([]byte, error) {
	f, err := os.Open(filepath.Join(dir, FileName(endpoint)))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}
