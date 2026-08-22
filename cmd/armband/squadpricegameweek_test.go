package main

import (
	"testing"

	"armband/internal/fpl"
)

// TestSquadPriceGameweekPicksTheLiveGapsCurrentEvent is the regression for the
// 2026-08-22 production incident: for minutes after PR #45's merge,
// https://fplarmband.com/api/state returned HTTP 500 "the squad could not be
// built just now" for the real production entry.
//
// PR #45 correctly closed AssemblyBudget's own copy of this defect family
// (case e.Entry != 0 && e.GameweeksPlayed() == 0 -> case e.Entry != 0 &&
// !e.SeasonHasStarted()), which had been silently papering over a SIXTH
// instance one call site upstream: run()'s switch treated played == 0 as
// "nothing has been bought, assume the full £100m allowance" whenever the
// season had started but no gameweek had FINISHED yet — the whole multi-day
// gap between a gameweek's first kickoff and its last final whistle, during
// which the entry's GW1 squad is in fact already locked in. That skipped
// client.SquadPrices entirely, left engine.Bank/SquadValue nil, and
// AssemblyBudget's now-correct hard error surfaced as a user-facing 500
// instead of the harmless-looking wrong default it used to be.
//
// squadPriceGameweek is the fix's pure core: which gameweek to price the
// squad's picks against. played answers it once a gameweek has finished;
// during a live gap only Bootstrap.CurrentEvent() (FPL's own is_current
// flag) can, which is a genuinely different signal from GameweeksPlayed(),
// not a boolean substitute for it. The answer is the LATER of the two, not
// whichever one happens to be nonzero — see
// TestSquadPriceGameweekTracksEveryGameweeksOwnGap below for why "prefer
// played whenever it's nonzero" is a second copy of this exact incident.
func TestSquadPriceGameweekPicksTheLiveGapsCurrentEvent(t *testing.T) {
	cur := &fpl.Event{ID: 1}
	for _, c := range []struct {
		name   string
		played int
		cur    *fpl.Event
		want   int
	}{
		{"a finished gameweek is trusted on its own number", 3, nil, 3},
		{"the live gap: no gameweek finished, but one is live", 0, cur, 1},
		{"neither signal answers: no gameweek to price against", 0, nil, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := squadPriceGameweek(c.played, c.cur); got != c.want {
				t.Errorf("squadPriceGameweek(%d, %v) = %d, want %d", c.played, c.cur, got, c.want)
			}
		})
	}
}

// TestSquadPriceGameweekTracksEveryGameweeksOwnGap guards a review finding
// against this fix's own first draft: preferring played whenever it was
// nonzero fixed GW1's live gap but reintroduced the identical defect every
// later gameweek's own gap, silently, forever, one week later each time.
//
// is_current tracks whose DEADLINE has passed, not whose matches have
// FINISHED. So once GW1 finishes (played becomes 1), the same multi-day gap
// recurs the instant GW2's deadline passes and before GW2's own matches
// finish: GameweeksPlayed() is still 1 while CurrentEvent().ID is 2, and a
// transfer made for GW2 is already locked in. The first draft's
// squadPriceGameweek(1, &Event{ID: 2}) returned 1 — GW1's stale picks — which
// is this incident's exact defect, one gameweek later. run() builds the
// engine once and cmdServe reuses it for the whole process's uptime, and this
// gap spans days most weeks, so a long-running server would have priced
// every squad wrong for as long as it stayed up across that boundary, with
// no error and a plausible-looking number.
func TestSquadPriceGameweekTracksEveryGameweeksOwnGap(t *testing.T) {
	if got, want := squadPriceGameweek(1, &fpl.Event{ID: 2}), 2; got != want {
		t.Errorf("squadPriceGameweek(1, GW2 current) = %d, want %d — GW2's deadline has "+
			"passed and its picks are locked in, even though GW1 is the last FINISHED "+
			"gameweek; pricing against GW1 here silently misses a transfer", got, want)
	}
	// The reverse must not happen: a stale CurrentEvent that lags behind a
	// gameweek that has already finished must never win.
	if got, want := squadPriceGameweek(4, &fpl.Event{ID: 3}), 4; got != want {
		t.Errorf("squadPriceGameweek(4, stale GW3 current) = %d, want %d — a finished "+
			"gameweek later than the reported current event must still win", got, want)
	}
}
