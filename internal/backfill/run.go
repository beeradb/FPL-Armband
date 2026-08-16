package backfill

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"armband/internal/capture"
	"armband/internal/wayback"
)

// Source is the two things a backfill needs from the Internet Archive.
//
// An interface rather than the concrete client for one reason that earns its keep:
// the leak this package exists to prevent is a *selection* bug, and a selection bug
// is only testable if the crawls can be made up. `*wayback.Client` satisfies it, and
// so does a table of synthetic snapshots — which is how the provisional-calendar
// regression is pinned without touching the network.
type Source interface {
	Index(ctx context.Context, target string, from, to time.Time, refresh bool) ([]wayback.Snapshot, error)
	Fetch(ctx context.Context, s wayback.Snapshot) ([]byte, error)
}

// Options configures one season's backfill.
type Options struct {
	Root   string // the captures root, e.g. data/captures
	Season string // "2020-21"

	// PerGameweek is the cadence: how many crawls to keep per deadline.
	//
	// One is the required cadence and the only one run so far, because one is the
	// decision-relevant unit — a manager acts at the deadline and the state that
	// matters is the state then. Denser is a flag rather than a default because
	// the availability *trajectory* is one of the questions this data is meant to
	// open, and answering it will want several reads inside a gameweek. Every one
	// of them is stored against the same deadline and passes the same check, so
	// raising this cannot make the store less honest, only larger.
	PerGameweek int

	// Kickoffs maps gameweek to the earliest kickoff the season archive records.
	// Required: it is the independent witness the recovered calendar is checked
	// against, and without it a mis-dated season passes silently.
	Kickoffs map[int]time.Time

	Version      string // the armband revision doing the work, recorded per capture
	RefreshIndex bool   // re-query CDX rather than reading the cached index
	Log          func(string)
}

func (o Options) log(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
}

// Row is one gameweek's outcome.
type Row struct {
	Event    int
	Deadline time.Time

	// Found says a usable pre-deadline crawl exists for this gameweek, whether it
	// was stored on this run or was already there.
	//
	// A row describes the STORE, not the run: a re-run that fetched nothing must
	// still report the true coverage rather than an empty table, so there is
	// deliberately no per-row "written this time" field. The run's own total is
	// `Result.Stored`.
	Found bool
	Have  int // captures covering this gameweek

	At           time.Time // the crawl backing this gameweek, latest if several
	HoursBefore  float64
	EventsBehind int // deadlines already passed at crawl time; -1 unknown. See capture.Quality

	// Err is why a gameweek has nothing, in words. Populated for every gap,
	// because a gap with no reason attached is indistinguishable from one nobody
	// looked at — and this package's recorded failure mode is a repair that
	// silently applies nothing.
	Err string
}

// Result is a season's backfill outcome.
type Result struct {
	Season    string
	Deadlines *Deadlines
	Rows      []Row

	Crawls  int // crawls the Archive holds in this season's window
	Stored  int // captures written on this run
	Covered int // gameweeks with a usable pre-deadline crawl
	Fresh   int // of those, ones not predating an earlier deadline
	Bytes   int // stored bytes written on this run
}

// CoveragePct is the fraction of the 38 gameweeks with a usable pre-deadline crawl.
func (r Result) CoveragePct() float64 { return 100 * float64(r.Covered) / 38 }

// Run backfills one season.
//
// # Two passes, and why the first one is only an estimate
//
// Pass one learns a provisional calendar from a single mid-season crawl, because FPL
// publishes all 38 deadlines in every payload and asking per gameweek would be 38
// requests for 38 copies of one answer. Pass two selects and fetches one crawl per
// deadline.
//
// The word "provisional" is load-bearing and was the one correction this design
// needed. **FPL's calendar moves within a season**: on 2020-21 a crawl from 26
// January gives deadlines for GW25 onward that are up to 17.5 hours *later* than the
// truth, because those fixtures were subsequently moved into Friday evening slots.
// Trusting one calendar for all 38 would have SELECTED a crawl from after the real
// deadline in **6 of 38 gameweeks** — GW25, 28, 29, 31, 32 and 35, all after GW24.
// None of them would have been *stored*: `capture.VerifyPreDeadline` refuses a body
// whose own payload puts it past the deadline, so the naive design loses those six as
// visible gaps rather than leaking them. That is the second mechanism earning its
// keep, and it is why the calendar bug costs coverage rather than hindsight.
// `resolve` recovers them by re-reading each gameweek's deadline from a crawl taken
// near it.
//
// # Idempotence
//
// A second run over a complete season makes **no network requests at all**: the
// calendar is cached beside the captures and the captures are their own record of
// what has been fetched. That matters more than convenience here, because the
// alternative is re-querying somebody else's free index every time somebody wants to
// look at the coverage table.
func Run(ctx context.Context, c Source, o Options) (*Result, error) {
	if o.PerGameweek < 1 {
		o.PerGameweek = 1
	}
	if len(o.Kickoffs) == 0 {
		return nil, fmt.Errorf("backfilling %s needs the season archive's kickoff times to "+
			"check the recovered calendar against; refusing to run unchecked", o.Season)
	}
	res := &Result{Season: o.Season}

	store, err := capture.Open(o.Root)
	if err != nil {
		return nil, err
	}

	// The calendar first: it is cached because it carries a finding — the
	// cross-check — that no refetch would reproduce.
	dl, err := LoadDeadlines(o.Root, o.Season)
	if err == nil {
		o.log("calendar for %s read from disk (%d gameweeks, checked against %d archive "+
			"kickoffs, median deadline-to-kickoff %.1f h)",
			o.Season, len(dl.By), dl.Check.Compared, dl.Check.MedianGapHours)
	}

	// What is already on disk. Counted before any request, so a complete season
	// costs nothing to re-run.
	missing := 0
	for gw := 1; gw <= 38; gw++ {
		if store.Count(o.Season, gw) < o.PerGameweek {
			missing++
		}
	}
	if dl != nil && missing == 0 {
		o.log("%s is already complete at %d capture(s) per gameweek; no requests made",
			o.Season, o.PerGameweek)
		return finish(res, store, o, dl), nil
	}

	from, to, err := SeasonWindow(o.Season)
	if err != nil {
		return nil, err
	}
	o.log("querying the Internet Archive's index for crawls of %s between %s and %s "+
		"(this is slow, and it is someone else's infrastructure)",
		Target, from.Format("2006-01-02"), to.Format("2006-01-02"))
	snaps, err := c.Index(ctx, Target, from, to, o.RefreshIndex)
	if err != nil {
		return nil, err
	}
	res.Crawls = len(snaps)
	if len(snaps) == 0 {
		return nil, fmt.Errorf("the Internet Archive holds no 200-status crawls of %s "+
			"between %s and %s: %s cannot be backfilled from this source",
			Target, from.Format("2006-01-02"), to.Format("2006-01-02"), o.Season)
	}
	o.log("the index holds %d crawls across %d distinct days", len(snaps), distinctDays(snaps))

	if dl == nil {
		mid := midpoint(o.Kickoffs)
		o.log("pass one: reading the gameweek calendar from a crawl near %s", mid.Format("2006-01-02"))
		dl, err = DiscoverDeadlines(ctx, c, o.Season, snaps, mid, o.Kickoffs)
		if err != nil {
			return nil, err
		}
		if err := SaveDeadlines(o.Root, dl); err != nil {
			return nil, err
		}
		o.log("calendar recovered: %d gameweeks, GW1 deadline %s, GW38 deadline %s",
			len(dl.By), dl.By[1].Format("2006-01-02 15:04Z"), dl.By[38].Format("2006-01-02 15:04Z"))
		o.log("cross-check against the season archive: %d gameweeks compared, %d inverted, "+
			"median deadline-to-first-kickoff %.1f h (FPL's rule is 1.5 h)",
			dl.Check.Compared, dl.Check.Inverted, dl.Check.MedianGapHours)
	}

	if p := dl.Check.Provisional(); len(p) > 0 && len(dl.Confirmed) == 0 {
		o.log("%d of %d gameweeks carry a provisional deadline in the discovery crawl "+
			"(worst %.1f h out); each is re-read from a crawl taken near it",
			len(p), dl.Check.Compared, dl.Check.WorstInvertedHours)
	}

	// Pass two.
	dirty := false
	for _, gw := range dl.Sorted() {
		have := store.Count(o.Season, gw)
		if have >= o.PerGameweek {
			continue
		}
		confirmed, err := o.resolve(ctx, c, res, dl, snaps, gw, have)
		if err != nil {
			o.log("GW%d: %v", gw, err)
		}
		if confirmed && !dl.Confirmed[gw] {
			if dl.Confirmed == nil {
				dl.Confirmed = map[int]bool{}
			}
			dl.Confirmed[gw] = true
			dirty = true
		}
	}
	if dirty {
		// The confirmed deadlines are worth keeping: they are what makes a re-run
		// and the coverage report correct without refetching, and they are strictly
		// better than what the discovery crawl claimed.
		if err := SaveDeadlines(o.Root, dl); err != nil {
			o.log("could not save the confirmed calendar: %v", err)
		}
	}

	// Re-index, because the store was opened before anything was written.
	store, err = capture.Open(o.Root)
	if err != nil {
		return nil, err
	}
	return finish(res, store, o, dl), nil
}

// resolve fetches and stores the crawls for one gameweek, discovering that
// gameweek's true deadline along the way.
//
// # Why this is not just "select, fetch, store"
//
// The design this backfill was specified with reads all 38 deadlines from one
// mid-season crawl. That is unsafe, and the cross-check caught it: FPL's calendar
// moves within a season. Measured on 2020-21, a crawl from 27 January carries
// deadlines for GW25 onwards that are **up to 17.5 hours later than the true ones**,
// because those fixtures were subsequently moved to Friday evening slots and the
// deadline moved with them. Selecting "the last crawl strictly before a deadline that
// is seventeen hours too late" selects a crawl from *after* the real deadline — the
// exact leak this whole exercise is arranged to prevent, arriving through the
// calendar rather than through the selection rule.
//
// So the deadline is treated as an estimate to be tightened rather than a fact:
//
//  1. Start from a conservative bound — the earlier of the provisional deadline and
//     ninety minutes before the archive's first kickoff for the gameweek, which is
//     FPL's own rule and is independent of any crawl.
//  2. Select the last crawl before it and fetch.
//  3. Ask that crawl what the deadline was. A crawl from inside a gameweek's own
//     run-up carries the final figure.
//  4. If it says we are late, step back and try again; if it says we could have gone
//     later, widen and try again. Either way progress is monotone, so it terminates.
//
// In the common case — a crawl already inside the run-up — step 3 confirms the bound
// and it is one fetch, exactly as the simple version would have been.
func (o Options) resolve(ctx context.Context, c Source, res *Result,
	dl *Deadlines, snaps []wayback.Snapshot, gw, have int) (confirmed bool, err error) {

	bound := dl.By[gw]

	// hardBound is a ceiling nothing may widen past: ninety minutes before the
	// gameweek's first kickoff, which is FPL's own rule applied to the season
	// archive. It is the one estimate here that cannot be provisional, because it
	// comes from matches that were played rather than from a plan.
	//
	// It has to be a ceiling and not merely a starting point. The loop below widens
	// the bound to whatever deadline a verified crawl reports, and a crawl can report
	// a deadline FPL had not yet corrected — so a payload still claiming Saturday
	// 11:00 while the match kicked off Friday evening would widen the window across
	// the real deadline, and the next crawl selected inside it could be post-match.
	// The payload self-check would usually catch that, but only usually: it asks FPL
	// whether the deadline has passed, and in this scenario FPL is the thing that is
	// wrong.
	var hardBound time.Time
	if k, ok := o.Kickoffs[gw]; ok && !k.IsZero() {
		hardBound = k.Add(-90 * time.Minute)
		if bound.IsZero() || hardBound.Before(bound) {
			bound = hardBound
		}
	}
	if bound.IsZero() {
		return false, fmt.Errorf("no deadline and no kickoff, so nothing can be selected against")
	}

	const attempts = 4
	var chosen *wayback.Snapshot
	var chosenBody []byte

	for i := 0; i < attempts; i++ {
		s, ok := SelectPreDeadline(snaps, bound)
		if !ok {
			break
		}
		if chosen != nil && !s.At.After(chosen.At) {
			break // no improvement available; keep what verified
		}
		body, err := c.Fetch(ctx, s)
		if err != nil {
			o.log("GW%d: fetching %s failed: %v", gw, s.At.Format(time.RFC3339), err)
			bound = s.At // step back past the crawl that would not come down
			continue
		}
		q, err := capture.VerifyPreDeadline(body, gw, s.At)
		if err != nil {
			// The crawl's own payload says this is at or after the deadline.
			// Tighten to the deadline IT reports, which is a better estimate than
			// the one we came in with, and step back.
			next := s.At
			if !q.Deadline.IsZero() && q.Deadline.Before(bound) {
				next = q.Deadline
			}
			if !next.Before(bound) {
				next = s.At // guarantee the bound strictly decreases
			}
			bound = next
			continue
		}

		snapshot := s
		chosen, chosenBody = &snapshot, body

		// The crawl is authoritative about its own gameweek's deadline. Record it,
		// and if it is later than the bound we selected against there may be a
		// better crawl still available.
		dl.By[gw] = q.Deadline
		confirmed = true
		if q.Deadline.After(bound) && (hardBound.IsZero() || q.Deadline.Before(hardBound)) {
			bound = q.Deadline
			continue
		}
		break
	}

	if chosen == nil {
		return confirmed, nil
	}

	stored := o.store(res, gw, *chosen, chosenBody)

	// A denser cadence takes the rest of the run-up behind the one just resolved,
	// now against a deadline that is known rather than provisional.
	if want := o.PerGameweek - have - stored; want > 0 {
		var after time.Time
		if gw > 1 {
			after = dl.By[gw-1]
		}
		for _, s := range SelectWindow(snaps, after, chosen.At, want) {
			body, err := c.Fetch(ctx, s)
			if err != nil {
				o.log("GW%d: fetching %s failed: %v", gw, s.At.Format(time.RFC3339), err)
				continue
			}
			stored += o.store(res, gw, s, body)
		}
	}
	return confirmed, nil
}

// store writes one crawl and counts it. The refusal path is loud and is never retried
// under another name: it means the body could not prove it predates the deadline,
// which is the one thing this whole exercise is for.
func (o Options) store(res *Result, gw int, s wayback.Snapshot, body []byte) int {
	dir := filepath.Join(capture.SeasonDir(o.Root, o.Season), capture.BackfillDirName(gw, s.At))
	if at, err := capture.CapturedAtOf(dir); err == nil {
		if at.Equal(s.At.UTC()) {
			return 0 // this exact crawl is already filed under this gameweek
		}
		// Same gameweek, same MINUTE, different crawl. Directory names are
		// minute-resolution while crawl times are second-resolution, so this is
		// reachable — only at a denser cadence, but silently dropping the second
		// crawl would leave a run reporting a capture it did not take.
		o.log("GW%d: %s already holds the crawl at %s, so the one at %s is not stored; "+
			"two crawls in one minute collide under minute-resolution names",
			gw, filepath.Base(dir), at.Format(time.RFC3339), s.At.UTC().Format(time.RFC3339))
		return 0
	}
	path, m, err := capture.Backfilled{
		Root: o.Root, Season: o.Season, Event: gw, Body: body, At: s.At,
		Source: s.RawURL(), Stamp: s.At.UTC().Format(wayback.TimestampLayout),
		Digest: s.Digest, Version: o.Version,
	}.Write()
	if err != nil {
		o.log("GW%d: %v", gw, err)
		return 0
	}
	res.Stored++
	for _, f := range m.Files {
		res.Bytes += f.Stored
	}
	// Two decimals, not one. A genuine capture 51 seconds before a deadline reads as
	// "0.0 h before" at one decimal, which is exactly the value this store forbids —
	// so the honest tightest case displays as the thing it is not allowed to be.
	o.log("GW%d: stored %s (%.2f h before the deadline)", gw, filepath.Base(path), *m.HoursToDeadline)
	return 1
}

// finish builds the per-gameweek rows from what is on disk, which is deliberately
// not from what this run did: the report must describe the store, so that a re-run
// that fetched nothing still prints the true coverage rather than an empty table.
func finish(res *Result, store *capture.Store, o Options, dl *Deadlines) *Result {
	res.Deadlines = dl
	res.Rows = Rows(store, o.Season, dl)
	for _, r := range res.Rows {
		if !r.Found {
			continue
		}
		res.Covered++
		if r.EventsBehind == 0 {
			res.Fresh++
		}
	}
	return res
}

// Rows builds the 38-row coverage table for a season from what is on disk.
//
// Exported and shared with the offline `-coverage` report, rather than each building
// its own. The two once differed by exactly one column — `EventsBehind` printed a real
// figure during a fetch and "—" when reporting — which is the shape this project keeps
// paying for: one quantity with two implementations, where the one you look at most
// is the one that knows less. There is no reason for the offline path to know less,
// since everything it needs is in the bytes already stored.
//
// `dl` may be nil, in which case the deadline column comes from the manifests alone.
func Rows(store *capture.Store, season string, dl *Deadlines) []Row {
	out := make([]Row, 0, 38)
	for gw := 1; gw <= 38; gw++ {
		row := Row{Event: gw, EventsBehind: -1}
		if dl != nil {
			row.Deadline = dl.By[gw]
		}
		row.Have = store.Count(season, gw)
		if row.Have == 0 {
			row.Err = "no crawl of bootstrap-static before this deadline is held by the " +
				"Internet Archive"
			out = append(out, row)
			continue
		}
		a, err := store.At(season, gw)
		if err != nil {
			row.Err = err.Error()
			out = append(out, row)
			continue
		}
		row.At = a.SnapshotAt
		row.HoursBefore = a.HoursBefore
		row.EventsBehind = a.EventsBehind
		if row.Deadline.IsZero() {
			row.Deadline = a.Deadline
		}

		// The stored capture must still prove its own timing. The write-time check
		// is not enough on its own: it ran once, against bytes that could since have
		// been replaced, hand-placed or truncated, and this is the only place the
		// proof is re-run outside the test suite.
		//
		// A failure here is an integrity failure, not a quality note, so the row does
		// NOT count as coverage. Swallowing it — which an earlier version did, by
		// testing `err == nil` and carrying on — meant a capture whose bytes no
		// longer predate its deadline printed as an ordinary covered gameweek with a
		// dash in the staleness column and no message anywhere in the run.
		if body, err := readBootstrap(store, season, gw); err != nil {
			row.Err = "the stored payload could not be read: " + err.Error()
			out = append(out, row)
			continue
		} else if _, err := capture.VerifyPreDeadline(body, gw, a.SnapshotAt); err != nil {
			row.Err = "the stored capture no longer proves it predates this deadline: " +
				err.Error()
			out = append(out, row)
			continue
		}
		row.Found = true
		out = append(out, row)
	}
	return out
}

// readBootstrap pulls the stored payload for one gameweek back off disk.
func readBootstrap(store *capture.Store, season string, gw int) ([]byte, error) {
	dir, ok := store.Dir(season, gw)
	if !ok {
		return nil, fmt.Errorf("no capture for GW%d", gw)
	}
	return capture.Read(dir, capture.BootstrapEndpoint)
}

// midpoint is the middle of the football, used to pick the crawl the calendar is read
// from. Taken from the season archive's kickoffs rather than from the query window,
// which is deliberately wider than a season and whose midpoint can land in a summer.
func midpoint(kickoffs map[int]time.Time) time.Time {
	var ts []time.Time
	for _, k := range kickoffs {
		if !k.IsZero() {
			ts = append(ts, k)
		}
	}
	if len(ts) == 0 {
		return time.Time{}
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
	return ts[len(ts)/2]
}

func distinctDays(snaps []wayback.Snapshot) int {
	days := map[string]bool{}
	for _, s := range snaps {
		days[s.At.UTC().Format("2006-01-02")] = true
	}
	return len(days)
}
