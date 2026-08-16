package backfill

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"armband/internal/capture"
	"armband/internal/wayback"
)

// fakeArchive serves synthetic crawls, so the selection rule can be tested against a
// calendar that moves — which is the case that broke the original design and which no
// live fetch would reproduce on demand.
type fakeArchive struct {
	snaps []wayback.Snapshot

	// calendarAt returns the deadlines FPL was publishing at a given moment. This is
	// the whole point of the fake: a real payload's `events[]` is a *snapshot of a
	// plan*, and the plan changes.
	calendarAt func(at time.Time) map[int]time.Time

	fetches int
}

func (f *fakeArchive) Index(context.Context, string, time.Time, time.Time, bool) ([]wayback.Snapshot, error) {
	return f.snaps, nil
}

func (f *fakeArchive) Fetch(_ context.Context, s wayback.Snapshot) ([]byte, error) {
	f.fetches++
	cal := f.calendarAt(s.At)
	next := 0
	for gw := 1; gw <= 38; gw++ {
		if d, ok := cal[gw]; ok && s.At.Before(d) {
			next = gw
			break
		}
	}
	var payload struct {
		Events   []capture.Event `json:"events"`
		Elements []struct {
			Code   int    `json:"code"`
			Status string `json:"status"`
		} `json:"elements"`
	}
	for gw := 1; gw <= 38; gw++ {
		d, ok := cal[gw]
		if !ok {
			continue
		}
		payload.Events = append(payload.Events, capture.Event{
			ID: gw, DeadlineTime: d,
			Finished: next > 0 && gw < next,
			IsNext:   gw == next,
		})
	}
	payload.Elements = append(payload.Elements, struct {
		Code   int    `json:"code"`
		Status string `json:"status"`
	}{Code: 1000, Status: "a"})
	return json.Marshal(payload)
}

// TestAProvisionalCalendarDoesNotLeak is the regression for a bug that was found in
// the live data and would otherwise have shipped silently.
//
// # The bug
//
// The design this was specified with reads all 38 deadlines from one mid-season crawl
// and selects the last crawl strictly before each. On 2020-21 that is unsafe: FPL's
// crawl of 26 January 2021 gives GW25's deadline as 20 February 11:00, and the true
// deadline was 19 February 18:30 — seventeen and a half hours earlier, because the
// gameweek's opening fixture had since been moved to Friday night. Two crawls exist in
// between, so the naive rule selects the one from 20 February 10:46, which is
// **sixteen hours after the real deadline and after Friday's match had been played**.
// Across the season it happens in 6 of 38 gameweeks, all in the back third.
//
// # What the fix has to do
//
// Not trust the calendar. This test builds exactly that situation — a calendar that
// says one thing in January and another in February, with crawls sitting in the gap —
// and asserts the stored capture predates the *true* deadline. It also asserts the
// resolved deadline is written back, because a re-run reads it from disk and would
// otherwise repeat the mistake for free.
func TestAProvisionalCalendarDoesNotLeak(t *testing.T) {
	const moved = 25 // the gameweek whose fixture gets pulled to Friday night

	published, kickoffs := season() // 38 weeks, deadline 90 minutes before kickoff

	// What actually happened to GW25: its opening fixture moved to Friday evening,
	// 17.5 hours earlier, and FPL's deadline moved with it.
	kickoffs[moved] = kickoffs[moved].Add(-17*time.Hour - 30*time.Minute)
	trueDeadline := kickoffs[moved].Add(-90 * time.Minute)
	provisional := published[moved] // what a crawl from earlier in the season still claims

	// FPL published the change a fortnight out, which is roughly the real notice
	// period for a televised re-scheduling.
	revisedFrom := trueDeadline.Add(-14 * 24 * time.Hour)
	cal := func(a time.Time) map[int]time.Time {
		out := map[int]time.Time{}
		for k, v := range published {
			out[k] = v
		}
		if !a.Before(revisedFrom) {
			out[moved] = trueDeadline
		}
		return out
	}

	// One honest crawl a few hours before every deadline as published at the time,
	// plus two in GW25's leak window — after the true deadline and before the
	// provisional one. Those two are the trap.
	f := &fakeArchive{calendarAt: cal}
	for gw := 1; gw <= 38; gw++ {
		d := published[gw]
		if gw == moved {
			d = trueDeadline
		}
		f.snaps = append(f.snaps, wayback.Snapshot{At: d.Add(-3 * time.Hour), Original: "https://x/"})
	}
	leaky := []time.Time{trueDeadline.Add(2 * time.Hour), provisional.Add(-15 * time.Minute)}
	for _, ts := range leaky {
		f.snaps = append(f.snaps, wayback.Snapshot{At: ts, Original: "https://x/"})
	}

	// First, pin the hazard itself, so this test still means something if somebody
	// later decides the resolver is over-engineering. Selecting against the
	// provisional deadline — the design as originally specified — reaches a crawl
	// from after the true one.
	naive, ok := SelectPreDeadline(f.snaps, provisional)
	if !ok || naive.At.Before(trueDeadline) {
		t.Fatalf("the fixture no longer contains the hazard: selecting against the "+
			"provisional deadline gives %v, which is already honest", naive.At)
	}
	t.Logf("selecting against the provisional deadline would take the crawl at %s, "+
		"%.1f h AFTER the true deadline of %s",
		naive.At.Format(time.RFC3339), naive.At.Sub(trueDeadline).Hours(),
		trueDeadline.Format(time.RFC3339))

	root := t.TempDir()
	res, err := Run(context.Background(), f, Options{
		Root: root, Season: "2020-21", PerGameweek: 1, Kickoffs: kickoffs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Covered != 38 {
		t.Fatalf("covered %d gameweeks, want 38 — note that refusing the leaky crawl and "+
			"reporting a gap would also be honest, but it loses a gameweek that is "+
			"recoverable", res.Covered)
	}

	store, err := capture.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.At("2020-21", moved)
	if err != nil {
		t.Fatal(err)
	}
	if !a.SnapshotAt.Before(trueDeadline) {
		t.Fatalf("LEAK — GW%d was stored from a crawl at %s, at or after the true deadline "+
			"of %s. This is the provisional-calendar bug: the crawl looks fine against the "+
			"deadline published earlier in the season, and carries a match already played.",
			moved, a.SnapshotAt.Format(time.RFC3339), trueDeadline.Format(time.RFC3339))
	}
	if want := trueDeadline.Add(-3 * time.Hour); !a.SnapshotAt.Equal(want) {
		t.Errorf("GW%d stored from %s, want the last honest crawl at %s",
			moved, a.SnapshotAt.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if !a.Deadline.Equal(trueDeadline) {
		t.Errorf("GW%d's recorded deadline is %s, want the resolved %s — a manifest carrying "+
			"the provisional figure would misreport how fresh the capture was",
			moved, a.Deadline.Format(time.RFC3339), trueDeadline.Format(time.RFC3339))
	}

	// The resolved calendar must be written back, or a re-run repeats the mistake.
	dl, err := LoadDeadlines(root, "2020-21")
	if err != nil {
		t.Fatal(err)
	}
	if !dl.By[moved].Equal(trueDeadline) {
		t.Errorf("the saved calendar still has GW%d at %s", moved, dl.By[moved])
	}
	if !dl.Confirmed[moved] {
		t.Errorf("GW%d's deadline was resolved from a crawl in its own run-up but is not "+
			"marked confirmed, so a later reader cannot tell it from a provisional one", moved)
	}
}

// TestARerunOverACompleteSeasonMakesNoRequests pins idempotence.
//
// It matters beyond convenience: the alternative is re-querying somebody else's free
// index every time anyone looks at a coverage table, and this whole package runs on
// infrastructure donated by a charity.
func TestARerunOverACompleteSeasonMakesNoRequests(t *testing.T) {
	deadlines, kickoffs := season()
	f := &fakeArchive{calendarAt: func(time.Time) map[int]time.Time { return deadlines }}
	for gw := 1; gw <= 38; gw++ {
		f.snaps = append(f.snaps, wayback.Snapshot{
			At: deadlines[gw].Add(-3 * time.Hour), Original: "https://x/"})
	}
	root := t.TempDir()
	opts := Options{Root: root, Season: "2020-21", PerGameweek: 1, Kickoffs: kickoffs}

	res, err := Run(context.Background(), f, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Covered != 38 {
		t.Fatalf("covered %d of 38 gameweeks on the first run", res.Covered)
	}
	first := f.fetches
	if first == 0 {
		t.Fatal("the first run fetched nothing")
	}

	if _, err := Run(context.Background(), f, opts); err != nil {
		t.Fatal(err)
	}
	if f.fetches != first {
		t.Errorf("a re-run of a complete season fetched %d more crawls; it must make no "+
			"requests at all", f.fetches-first)
	}
}

// TestAGameweekWithNoPrecedingCrawlIsReportedAsAGap pins the loud path end to end.
//
// A repair that silently applies nothing is this area's recorded failure mode, twice.
// The gameweek must come back as absent, with a reason, and must not borrow the crawl
// from a neighbouring week without saying so.
func TestAGameweekWithNoPrecedingCrawlIsReportedAsAGap(t *testing.T) {
	deadlines, kickoffs := season()

	// The Archive has nothing at all before GW11's deadline — the shape of a season
	// whose early crawls were never taken. Those ten gameweeks have no honest
	// evidence and must come back empty rather than borrowing a later crawl.
	f := &fakeArchive{calendarAt: func(time.Time) map[int]time.Time { return deadlines }}
	for gw := 11; gw <= 38; gw++ {
		f.snaps = append(f.snaps, wayback.Snapshot{
			At: deadlines[gw].Add(-3 * time.Hour), Original: "https://x/"})
	}

	root := t.TempDir()
	res, err := Run(context.Background(), f, Options{
		Root: root, Season: "2020-21", PerGameweek: 1, Kickoffs: kickoffs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Covered != 28 {
		t.Fatalf("covered %d gameweeks, want the 28 that have a preceding crawl", res.Covered)
	}
	for _, r := range res.Rows[:10] {
		if r.Found {
			t.Errorf("GW%d reads as found, but no crawl precedes its deadline", r.Event)
		}
		if r.Err == "" {
			t.Errorf("GW%d is a gap with no reason attached, which reads later as a "+
				"gameweek nobody looked at", r.Event)
		}
	}
	store, err := capture.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.At("2020-21", 3); err == nil {
		t.Error("GW3 has no capture but reads back as data")
	}
}
