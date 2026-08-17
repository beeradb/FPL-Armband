package backtest

// Does the free hit field footballers who are actually playing?
//
// The chip exists for one thing: a round most clubs do not play. 2023-24 GW29
// blanked twelve clubs of twenty, and it is the canonical FA Cup blank a free
// hit is saved for. Measured before the fix, on real archived seasons:
//
//	2023-24 GW18 ( 2/20 clubs blank): fifteen held  3 blanking players, best XI 2
//	2023-24 GW29 (12/20 clubs blank): fifteen held 13 blanking players, best XI 9
//	2024-25 GW29 ( 4/20 clubs blank): fifteen held  5 blanking players, best XI 5
//	2025-26 GW34 ( 6/20 clubs blank): fifteen held  6 blanking players, best XI 4
//
// At GW29 2023-24 the chip would have fielded two footballers.
//
// Two things had to be true for that, and this file pins both. `fixtureLoadFor`
// could not express a blank at all, so a blanking player's Score was untouched;
// and even with the score at zero the builder still has four bench slots to
// fill, is indifferent between two players worth nothing, and takes whoever is
// cheapest. So the guard has to reach the POOL — see freeHitSquad.

import (
	"testing"

	"armband/internal/analysis"
)

// TestFreeHitNeverFieldsABlankingClub builds the free-hit fifteen exactly as
// Simulate does, at the archive's own blank gameweeks.
//
// The gameweeks are found rather than written down: which rounds blank is a fact
// about one season's cup draws, and a hardcoded list stops exercising anything
// the moment the grid moves.
func TestFreeHitNeverFieldsABlankingClub(t *testing.T) {
	cfg := loadConfig(t)

	checked := 0
	for _, pair := range sweepPairNames() {
		cur, prior := loadSeason(t, cfg, pair[1]), loadSeason(t, cfg, pair[0])

		plays := map[[2]int]bool{}
		rounds := map[int]int{}
		for _, f := range cur.Fixtures {
			if f.Event == nil {
				continue
			}
			plays[[2]int{f.TeamH, *f.Event}] = true
			plays[[2]int{f.TeamA, *f.Event}] = true
			rounds[*f.Event]++
		}

		// The season's worst blank round, which is the week the chip is for and
		// the week a builder blind to blanks fails hardest. One per season keeps
		// this test in seconds; the whole ladder is the same assertion twenty
		// times over.
		gw, blanking := 0, 0
		for round := 2; round <= 38; round++ {
			if rounds[round] == 0 {
				continue // a round nobody plays; there is no chip week here
			}
			n := 0
			for club := range clubIDs(cur) {
				if !plays[[2]int{club, round}] {
					n++
				}
			}
			if n > blanking {
				gw, blanking = round, n
			}
		}
		// A week everybody plays cannot fail this test, so counting it would
		// inflate the coverage figure the assertion at the bottom rests on.
		if blanking < 4 {
			continue
		}
		vb, vf := PointInTimeWith(cur, prior, gw-1, Oracles{})
		w := cfg.Weights
		w.Horizon = 1
		he := analysis.NewEngineFull(vb, vf, w, analysis.Congestion{}, analysis.RoleRisk{})

		// Two budgets, because the guard binds through the BUDGET. A blanking
		// player is worth zero and so is a bench slot's fourth-choice keeper, and
		// a builder with money to spare buys the playing one anyway; it is the
		// squad that cannot afford fifteen playing footballers that reaches for a
		// cheap one who cannot appear. Swept over the six seasons' own blank
		// rounds at 82.0/90.0/100.0/110.0, the score-zero effect alone left a
		// blanking player in the fifteen in 17 builds of 160 — so a test at one
		// generous budget would pass on the scoring fix and pin nothing about the
		// exclusion. 82.0 is the low end a free hit is realistically spent from.
		for _, budget := range []int{820, 1000} {
			sq, err := he.Optimize(analysis.OptimizeRequest{
				Budget: budget, MinMinutes: 600, MinExpectedMinutes: 55,
				ExcludeIDs: he.ElementsWithoutFixtures(),
			})
			if err != nil {
				t.Errorf("%s GW%d (%d of 20 clubs blank, £%.1fm): the free-hit fifteen "+
					"could not be built: %v. Refusing to build is better than fielding "+
					"a blank, but a chip that cannot be spent on the week it exists "+
					"for is its own defect",
					cur.Name, gw, blanking, float64(budget)/10, err)
				continue
			}
			checked++
			held := 0
			for _, p := range sq.Players {
				el := vb.ElementByID(p.ID)
				if el == nil {
					continue
				}
				if !plays[[2]int{el.Team, gw}] {
					held++
				}
			}
			if held > 0 {
				t.Errorf("%s GW%d (%d of 20 clubs blank, £%.1fm): the free-hit fifteen "+
					"holds %d footballers whose club has no fixture",
					cur.Name, gw, blanking, float64(budget)/10, held)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no blank gameweek was reached, so the pass above is the archive " +
			"not loading rather than the chip behaving")
	}
	t.Logf("checked %d blank gameweeks across the grid", checked)
}

func clubIDs(s *Season) map[int]bool {
	out := map[int]bool{}
	for _, f := range s.Fixtures {
		out[f.TeamH], out[f.TeamA] = true, true
	}
	return out
}
