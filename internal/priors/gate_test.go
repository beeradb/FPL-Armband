package priors

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"armband/internal/analysis"
)

// TestTheBlendGateIsThinAndNonZero pins this package's expression of the
// prior-blend gate against analysis.ShouldBlendPrior, which is the rule.
//
// # Why the expectation is derived rather than written down
//
// The rule has three implementations — this one, recent.blendPast and
// backtest.newPriorIndexMulti — and a table of expected minutes written by hand
// would be a FOURTH statement of it, free to drift from the other three in
// exactly the way this test exists to catch. So the test asks the shipped
// predicate what should happen and checks only that this path did it. Change the
// bar and all three tests move together; change one implementation and only it
// fails.
//
// # The two exclusions are excluded differently, and that matters
//
// A full last season is handed back as itself. A last season of NO minutes is
// also handed back as itself — carrying Minutes 0 — and that is the load-bearing
// half: downstream, blendRates gates on `!ok || p.Minutes == 0` and sends exactly
// that player to shrinkToLeague. A zero-minute prior is therefore not a gap in
// the index, it is the signal that routes him to the machinery that handles him.
// Blending him instead replaces a league-rate estimate with a season at least two
// years old, and that is the half of prior_half_life that measured worse.
func TestTheBlendGateIsThinAndNonZero(t *testing.T) {
	const (
		code       = 4242
		olderMins  = 3000 // full seasons behind him, so a blend has somewhere to go
		olderBonus = 30
		halfLife   = 1.0
	)
	// TWO older seasons, which is what cmd/priorblend offers and what the live
	// path offers, and it is not decoration. With only one, a zero-minute last
	// season leaves a single usable season and the existing `len(hist) < 2` early
	// return already declines to blend — so the ungated code would pass this test
	// for the wrong reason and the gate would be untested on the case it is for.
	seasons := []string{"2025-26", "2024-25", "2023-24"}

	// Every value that can sit either side of the two boundaries, plus the
	// boundaries themselves. 1709/1710 straddles the thin bar; 0/1 straddles the
	// played-at-all bar, which is the one this test was added for.
	for _, lastMinutes := range []int{0, 1, 90, 900, ThinSeason - 1, ThinSeason, ThinSeason + 1, 3420} {
		dir := t.TempDir()
		write := func(season string, minutes int) {
			s := Season{Name: season, Players: map[int]*Player{
				code: {Code: code, WebName: "Test", Minutes: minutes,
					Starts: minutes / 90, Bonus: olderBonus, XG: 12.0},
			}}
			b, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "priors-"+season+".json"), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write(seasons[0], lastMinutes)
		for _, s := range seasons[1:] {
			write(s, olderMins)
		}

		// Cached seasons only: Load never reaches the network when the file is
		// present, so this test does not touch the FPL archive or the internet.
		got, err := LoadBlended(context.Background(), dir, seasons, halfLife)
		if err != nil {
			t.Fatalf("last season %d minutes: LoadBlended: %v", lastMinutes, err)
		}
		p, ok := got.Get(code)
		if !ok {
			t.Fatalf("last season %d minutes: the player vanished from the blended "+
				"season. This path keeps every player it was given; only "+
				"internal/backtest's index drops one, and it drops him deliberately.",
				lastMinutes)
		}

		if analysis.ShouldBlendPrior(lastMinutes) {
			if p.Minutes == lastMinutes {
				t.Errorf("last season %d minutes: the prior came back at %d, unchanged. "+
					"analysis.ShouldBlendPrior says this player is thin but played, which "+
					"is the one population the feature exists for — a good player whose "+
					"last season is an injury artefact — and the older season was not "+
					"folded in at all.", lastMinutes, p.Minutes)
			}
			continue
		}
		if p.Minutes != lastMinutes {
			why := "a full season stands alone: it is the best evidence there is, and " +
				"smoothing an older one into it dilutes genuine improvement"
			if lastMinutes == 0 {
				why = "no minutes at all is not a thin sample but a different fact, and " +
					"the zero must survive to the index or blendRates will not send him " +
					"to shrinkToLeague"
			}
			t.Errorf("last season %d minutes: the prior came back at %d, so older seasons "+
				"were blended in. %s.", lastMinutes, p.Minutes, why)
		}
	}
}
