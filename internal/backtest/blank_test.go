package backtest

// Are blank gameweeks scored correctly, and does a blanking captain cost the
// armband bonus the replay should be crediting?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBlankHandling -v -timeout 1h
//
// "Check the blank side too" started as one question — a club with no fixture
// produces no row in the archive, which `p.GWs[gw]` reads back as the zero
// value, and that is the correct signal for "did not play". This confirms that
// directly: every blank team-gameweek in the archive is matched to a squad
// player with zero minutes recorded, not a missing or malformed entry.
//
// It surfaces a second, larger question along the way. `pickXI` returns a
// single captain — `chosen[0].ID`, the top scorer — and `weekPoints` gives him
// the doubled bonus only if he actually records minutes that week. There is no
// fallback. FPL's real rule is that the armband passes to the vice-captain when
// the captain blanks for *any* reason, and this project already has a measured
// estimate of how often that happens: `ViceCaptainWeight` = 0.08, "a nailed
// captain blanks through injury, suspension or a late rotation call perhaps one
// week in twelve" — but that estimate has only ever been used inside the
// squad-selection *objective* (`xiValue`), never applied when the replay scores
// a gameweek that actually happened.
//
// A blank gameweek is the cleanest sub-case to measure this with, because it is
// know-in-advance and deterministic: if the model's own captain choice belongs
// to a club with no fixture, the replay is silently forfeiting the doubled
// bonus for that week, in a case a real manager would never be caught by.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagBlankHandling(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	pairs := sweepPairNames()
	starts := sweepStarts()

	// Part 1: confirm a blank team-gameweek reads as zero minutes, not a
	// missing or malformed row, for every squad player it touches.
	blankRows, malformed := 0, 0
	for _, pair := range pairs {
		cur := loadSeason(t, cfg, pair[1])
		played := map[[2]int]bool{} // (team, gw)
		for _, f := range cur.Fixtures {
			if f.Event == nil {
				continue
			}
			played[[2]int{f.TeamH, *f.Event}] = true
			played[[2]int{f.TeamA, *f.Event}] = true
		}
		for _, p := range cur.Players {
			if p.Minutes < 900 {
				continue // established players only
			}
			for gw := 1; gw <= 38; gw++ {
				if played[[2]int{p.Team, gw}] {
					continue
				}
				// A blank gameweek for this player's club.
				g, ok := p.GWs[gw]
				if !ok {
					blankRows++
					continue
				}
				if g.Minutes != 0 {
					malformed++
				} else {
					blankRows++
				}
			}
		}
	}
	fmt.Printf("\n=== Part 1: blank team-gameweeks, established players (900+ mins) ===\n\n")
	fmt.Printf("blank rows reading as zero minutes (correct): %d\n", blankRows)
	fmt.Printf("blank rows with nonzero minutes recorded (would be a bug): %d\n", malformed)

	// Part 2: how often does the model's own captain choice blank, and what
	// does the missing vice-captain bonus cost.
	type obs struct {
		captainScore   float64 // modelled score of the player picked as captain
		captainBlank   bool    // did he record zero minutes that gameweek
		blankIsFixture bool    // specifically because his club had no fixture
		viceBonus      int     // what the vice-captain (2nd highest scorer in the XI) would have added
	}
	var all []obs

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)
		played := map[[2]int]bool{}
		for _, f := range cur.Fixtures {
			if f.Event == nil {
				continue
			}
			played[[2]int{f.TeamH, *f.Event}] = true
			played[[2]int{f.TeamA, *f.Event}] = true
		}

		for _, start := range starts {
			sc := sweepConfig(cfg, start, true)
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatal(err)
			}
			held := res.OpeningSquad

			for gw := start; gw <= 38; gw++ {
				b, fx := PointInTime(cur, prior, gw-1)
				e := analysis.NewEngineFull(b, fx, sc.Weights, analysis.Congestion{}, analysis.RoleRisk{})
				e.Priors = idx
				e.Recent = newRecentIndexWith(cur, gw-1, sc.minutesHalfLife(), sc.Weights.RateHalfLife)
				xi, _, captain, viceID := pickXI(e, held)
				if captain == 0 || len(xi) == 0 {
					continue
				}

				var metrics []analysis.PlayerMetrics
				for _, id := range xi {
					if el := e.Boot.ElementByID(id); el != nil {
						metrics = append(metrics, e.Metrics(el))
					}
				}
				capScore := 0.0
				for _, m := range metrics {
					if m.ID == captain {
						capScore = m.Score
					}
				}

				cp := cur.Players[captain]
				if cp == nil {
					continue
				}
				g, ok := cp.GWs[gw]
				blanked := !ok || g.Minutes == 0
				isFixtureBlank := !played[[2]int{cp.Team, gw}]

				viceBonus := 0
				if blanked && viceID != 0 {
					if vp := cur.Players[viceID]; vp != nil {
						if vg, ok := vp.GWs[gw]; ok {
							viceBonus = vg.Points
						}
					}
				}
				all = append(all, obs{
					captainScore: capScore, captainBlank: blanked,
					blankIsFixture: isFixtureBlank, viceBonus: viceBonus,
				})
			}
		}
	}

	if len(all) == 0 {
		t.Skip("no observations")
	}

	blanks, fixtureBlanks, totalVice, weeks := 0, 0, 0, len(all)
	for _, o := range all {
		if o.captainBlank {
			blanks++
			totalVice += o.viceBonus
			if o.blankIsFixture {
				fixtureBlanks++
			}
		}
	}

	fmt.Printf("\n=== Part 2: the picked captain, and the vice-captain bonus never credited ===\n\n")
	fmt.Printf("gameweek-decisions examined: %d\n", weeks)
	fmt.Printf("weeks the model's own captain choice recorded zero minutes: %d (%.1f%%)\n",
		blanks, 100*float64(blanks)/float64(weeks))
	fmt.Printf("  of those, caused specifically by his club having no fixture: %d (%.1f%% of blanks)\n",
		fixtureBlanks, 100*float64(fixtureBlanks)/float64(max(blanks, 1)))
	fmt.Printf("total vice-captain points the replay never credits: %d\n", totalVice)
	fmt.Printf("mean points per gameweek-decision: %.4f\n", float64(totalVice)/float64(weeks))
	fmt.Printf("scaled to a season (x38): %.1f points\n", float64(totalVice)/float64(weeks)*38)

	fmt.Printf("\nFor comparison, ViceCaptainWeight (0.08) already estimates a captain blanks\n")
	fmt.Printf("about one week in twelve (8.3%%) inside the squad-selection objective.\n")
	fmt.Printf("The measured rate above is what the replay's own captain choice actually does.\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
