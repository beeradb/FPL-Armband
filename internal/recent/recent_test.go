package recent

import (
	"context"
	"math"
	"testing"
	"time"

	"armband/internal/fpl"
)

// TestWeighFavoursRecentMatches is the whole point: a player who has stopped
// playing must read as such, not as the ever-present his season total says he
// is.
func TestWeighFavoursRecentMatches(t *testing.T) {
	// Ten full matches, then five on the bench.
	var h []fpl.HistoryEntry
	for gw := 1; gw <= 10; gw++ {
		h = append(h, fpl.HistoryEntry{Round: gw, Minutes: 90, Starts: 1})
	}
	for gw := 11; gw <= 15; gw++ {
		h = append(h, fpl.HistoryEntry{Round: gw, Minutes: 0, Starts: 0})
	}

	flat := 900.0 / 15 // what the season total reports
	got, ok := weigh(h, 15, 3)
	if !ok {
		t.Fatal("no estimate from fifteen matches")
	}
	t.Logf("season average %.1f minutes, recency-weighted %.1f", flat, got.MinutesPerMatch)
	if got.MinutesPerMatch >= flat {
		t.Errorf("recency-weighted minutes %.1f are not below the season average %.1f",
			got.MinutesPerMatch, flat)
	}
	if got.Matches != 15 {
		t.Errorf("counted %d matches, want 15", got.Matches)
	}

	// The property that matters is monotonicity: a shorter half-life must weigh
	// the recent blanks more heavily. Asserting a particular figure would just
	// pin an arithmetic identity and break on any recalibration.
	var last float64 = 1e9
	for _, hl := range []float64{12, 8, 5, 3, 2, 1} {
		g, _ := weigh(h, 15, hl)
		t.Logf("  half-life %-4.0f minutes %.1f  starts %.2f", hl, g.MinutesPerMatch, g.StartShare)
		if g.MinutesPerMatch >= last {
			t.Errorf("half-life %.0f gives %.1f minutes, not below the %.1f of the longer one",
				hl, g.MinutesPerMatch, last)
		}
		last = g.MinutesPerMatch
	}
	// At a one-gameweek half-life the five blanks should dominate almost
	// entirely: the oldest full match is worth 2^-14 of the newest blank.
	if g, _ := weigh(h, 15, 1); g.MinutesPerMatch > 5 {
		t.Errorf("at a one-week half-life five blanks still read as %.1f minutes",
			g.MinutesPerMatch)
	}
}

// TestWeighIsFlatAtAnInfiniteHalfLife — the weighting must reduce to the plain
// average, or the default is not a strict generalisation of the old behaviour.
func TestWeighIsFlatAtALongHalfLife(t *testing.T) {
	var h []fpl.HistoryEntry
	for gw := 1; gw <= 10; gw++ {
		m := 0
		if gw <= 5 {
			m = 90
		}
		h = append(h, fpl.HistoryEntry{Round: gw, Minutes: m})
	}
	got, _ := weigh(h, 10, 1e9)
	if math.Abs(got.MinutesPerMatch-45) > 0.01 {
		t.Errorf("at an effectively infinite half-life the estimate is %.2f, want the plain "+
			"average of 45", got.MinutesPerMatch)
	}
}

// TestWeighCountsMatchesNotGameweeks — a double gameweek is two matches and a
// blank is none, which is what "minutes when he plays" means.
func TestWeighCountsMatchesNotGameweeks(t *testing.T) {
	h := []fpl.HistoryEntry{
		{Round: 5, Minutes: 90, Starts: 1},
		{Round: 5, Minutes: 90, Starts: 1}, // same gameweek, second fixture
	}
	got, ok := weigh(h, 5, 2)
	if !ok || got.Matches != 2 {
		t.Fatalf("a double gameweek counted as %d matches, want 2", got.Matches)
	}
	if math.Abs(got.MinutesPerMatch-90) > 0.01 {
		t.Errorf("minutes per match %.2f, want 90", got.MinutesPerMatch)
	}
}

// TestWeighIgnoresTheFuture — history rows past the last completed gameweek
// would be hindsight, and the replay has been bitten by exactly that class of
// leak before.
func TestWeighIgnoresTheFuture(t *testing.T) {
	h := []fpl.HistoryEntry{
		{Round: 1, Minutes: 90, Starts: 1},
		{Round: 9, Minutes: 0, Starts: 0}, // not played yet
	}
	got, ok := weigh(h, 1, 2)
	if !ok {
		t.Fatal("no estimate")
	}
	if got.Matches != 1 {
		t.Errorf("counted %d matches through GW1, want 1", got.Matches)
	}
	if got.MinutesPerMatch != 90 {
		t.Errorf("minutes per match %.1f; a later gameweek has leaked in", got.MinutesPerMatch)
	}
}

// TestNoHistoryIsNotAnEstimate — a player with nothing to weight must report
// absent so the model falls back to his season average rather than reading zero.
func TestNoHistoryIsNotAnEstimate(t *testing.T) {
	if _, ok := weigh(nil, 10, 2); ok {
		t.Error("an empty history produced an estimate")
	}
	if _, ok := weigh([]fpl.HistoryEntry{{Round: 20, Minutes: 90}}, 10, 2); ok {
		t.Error("a history entirely in the future produced an estimate")
	}
}

// TestFormGetOnNil — Load can fail, and the caller leaves Engine.Recent nil.
func TestFormGetOnNil(t *testing.T) {
	var f *Form
	if _, ok := f.Get(123); ok {
		t.Error("a nil Form claimed to know a player")
	}
}

// TestLoadAgainstTheLiveAPI exercises the fetch and concurrency path for real,
// on a handful of players so the test is not 500 requests.
//
// Pre-season every history is empty, so this checks the plumbing rather than the
// numbers: requests succeed, nothing panics under concurrency, and a player with
// no matches yet is reported absent rather than as zero minutes.
func TestLoadAgainstTheLiveAPI(t *testing.T) {
	c := fpl.New(t.TempDir(), time.Hour)
	ctx := context.Background()
	full, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}

	// A cut-down bootstrap: eight real players, and a gameweek marked finished
	// so Load has something to count back from.
	boot := &fpl.Bootstrap{Events: []fpl.Event{{ID: 1, Finished: true}}}
	for i := range full.Elements {
		el := full.Elements[i]
		if el.Code == 0 {
			continue
		}
		el.Minutes = 90 // force him into the fetch set
		boot.Elements = append(boot.Elements, el)
		if len(boot.Elements) == 8 {
			break
		}
	}
	if len(boot.Elements) < 8 {
		t.Skip("not enough players")
	}

	f, err := Load(ctx, c, boot, 3, DefaultConcurrency)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Logf("%d fetched, %d failed of %d requested", f.Fetched, f.Failed, len(boot.Elements))
	if f.Failed > 0 {
		t.Errorf("%d of %d history requests failed", f.Failed, len(boot.Elements))
	}
	// Whatever came back, a code that was never requested must not resolve.
	if _, ok := f.Get(-1); ok {
		t.Error("an unknown code resolved to a player")
	}
	for _, el := range boot.Elements {
		p, ok := f.Get(el.Code)
		if !ok {
			continue // no matches played yet, which is correct pre-season
		}
		if p.Matches <= 0 || p.MinutesPerMatch < 0 || p.MinutesPerMatch > 120 {
			t.Errorf("%s: implausible estimate %+v", el.WebName, p)
		}
	}
}

// TestLoadRejectsAnUnstartedSeason — with no finished gameweek there is nothing
// to weight, and silently returning an empty Form would look like full coverage.
func TestLoadRejectsAnUnstartedSeason(t *testing.T) {
	c := fpl.New(t.TempDir(), time.Hour)
	boot := &fpl.Bootstrap{Events: []fpl.Event{{ID: 1, Finished: false}}}
	if _, err := Load(context.Background(), c, boot, 3, 2); err == nil {
		t.Error("Load accepted a season that has not started")
	}
	if _, err := Load(context.Background(), c, boot, 0, 2); err == nil {
		t.Error("Load accepted a zero half-life")
	}
}
