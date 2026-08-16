package backtest

import (
	"math"
	"os"
	"sort"
	"testing"
)

// TestDiagXGCCoverage is the "did it actually fire, and is the answer the right size"
// check, which the correlation diagnostic deliberately does not ask.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagXGCCoverage -v
//
// TestDiagXGCReconstruction scores the *method* against seasons that already have the
// data. This one runs on the seasons that do not, where there is nothing to score
// against — so it checks the two things that can still be checked, and they are the
// two ways a repair fails silently in this package's record.
//
//   - **Coverage.** A repair that applied nothing looks exactly like a season that
//     never had the statistic, and `baseXP90` gates the clean sheet and the
//     goals-conceded deduction on `XGC90 > 0`, so the difference is 26-45% of every
//     defender's and keeper's score.
//   - **Level.** A reconstruction can be complete and on the wrong scale, which
//     nothing downstream would report. The anchor is football rather than a fitted
//     figure: a Premier League club concedes about 1.3 to 1.5 goals a match, so a
//     mean reconstructed club xGA far outside that is wrong however clean the
//     correlations were. The seasons that carry real xGC are printed in the same
//     table as the comparison, rather than the bound being asserted from memory.
func TestDiagXGCCoverage(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	// Every season the grid can reach, repaired and real together, so the repaired
	// ones are read beside a baseline rather than against an expectation.
	seasons := []string{
		"2018-19", "2019-20", "2020-21", "2021-22",
		"2022-23", "2023-24", "2024-25", "2025-26",
	}
	repairedNow := map[string]bool{
		"2018-19": true, "2019-20": true, "2020-21": true, "2021-22": true,
	}

	t.Log("season    appearances  with xGC  reconstructed   mean club xGA/match  " +
		"season totals rebuilt")
	for _, name := range seasons {
		s := loadSeason(t, cfg, name)

		var apps, withXGC, recon int
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for gw := 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes <= 0 {
					continue
				}
				apps++
				if g.XGC > 0 {
					withXGC++
				}
				if g.XGCReconstructed {
					recon++
				}
			}
		}

		// Club xGA per match, read off the ever-presents so it is a club figure
		// rather than a minutes-weighted average of shares — and beside it the
		// goals that club actually conceded in the same matches, which is what
		// makes the level check a measurement rather than a remembered band.
		matches := clubMatches(s)
		type clubGW struct{ xga, goals float64 }
		seen := map[[2]int]clubGW{}
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for gw := 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes != 90 || g.XGC <= 0 {
					continue
				}
				if matches[[2]int{p.Team, gw}] == 1 {
					seen[[2]int{p.Team, gw}] = clubGW{g.XGC, float64(g.GoalsConceded)}
				}
			}
		}
		var sum, goals float64
		keys := make([][2]int, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i][0] != keys[j][0] {
				return keys[i][0] < keys[j][0]
			}
			return keys[i][1] < keys[j][1]
		})
		for _, k := range keys {
			sum += seen[k].xga
			goals += seen[k].goals
		}
		mean, meanGoals := 0.0, 0.0
		if len(keys) > 0 {
			mean = sum / float64(len(keys))
			meanGoals = goals / float64(len(keys))
		}

		t.Logf("%-9s %11d  %7.1f%%  %13d   %8.3f vs %.3f goals (%.0f%%)  %d",
			name, apps, 100*float64(withXGC)/float64(apps), recon, mean, meanGoals,
			100*mean/meanGoals, s.XGRepair.XGC.AggFilled)

		if !repairedNow[name] {
			continue
		}
		// The hole this repair exists to fill was total: 0 of 10,543 appearances
		// in 2019-20, 0 of 9,905 in 2020-21, 0 of 9,788 in 2021-22. Anything much
		// short of complete now means the chain is dropping clubs or gameweeks.
		if cov := float64(withXGC) / float64(apps); cov < 0.95 {
			t.Errorf("%s: only %.1f%% of appearances carry xGC after the repair, "+
				"and this season carried none at all before it — the chain is "+
				"losing clubs or gameweeks", name, 100*cov)
		}
		// The level check, against this season's own football rather than against a
		// band somebody remembered. Expected goals and goals are different
		// quantities and need not agree closely on a single match, but over a
		// whole season a club's expected goals conceded lands near the goals it
		// actually conceded — the seasons that carry real xGC print 96-101% of
		// goals in the same column, which is what sets the tolerance here.
		if r := mean / meanGoals; r < 0.80 || r > 1.20 {
			t.Errorf("%s: mean reconstructed club xGA is %.3f a match against %.3f "+
				"goals actually conceded, %.0f%%. That is the wrong size and every "+
				"clean sheet in the season is mispriced",
				name, mean, meanGoals, 100*r)
		}
		if s.XGRepair.XGC.AggFilled == 0 {
			t.Errorf("%s: no season xGC totals rebuilt. The aggregate is what a "+
				"PRIOR is read through, so its successor is still blind", name)
		}
		// The table says this season has no xGC aggregate. A player who turns out
		// to have one means the table and the archive disagree, and the table is
		// what decides whether the rebuild runs at all.
		if k := s.XGRepair.XGC.AggKept; k != 0 {
			t.Errorf("%s: %d players already carried a season xGC total on a season "+
				"the repair table marks as having none — the table and the archive "+
				"disagree", name, k)
		}
		// Every appearance in the window is accounted for by exactly one counter.
		// Without this, the degenerate case where a season loads with no fixture
		// list reads Applied 0 / Skipped 0 / Empty 0, which is indistinguishable
		// from "there was nothing to repair".
		x := s.XGRepair.XGC
		if sum := x.Applied + x.Skipped + x.Empty + x.NoClubMatch; sum != apps {
			t.Errorf("%s: the repair accounts for %d appearances (applied %d, "+
				"skipped %d, empty %d, no club match %d) against %d in the season — "+
				"a row is being dropped by a path that does not count it",
				name, sum, x.Applied, x.Skipped, x.Empty, x.NoClubMatch, apps)
		}
	}
}

// TestTheXGCRepairHasAWorkingEscapeHatch pins the paired-arm switch.
//
// This package's recorded failure is a hatch that reaches one consumer of two, or one
// that reads a repaired cache and reports the two arms as identical — which looks
// exactly like a real null result. The xGC reconstruction is a shipping change to 12
// of 36 cells, so the arm that turns it off has to be real before any figure measured
// with it is worth anything.
//
// Both hatches are checked, because they answer different questions:
// FPL_NO_XG_REPAIR removes expected goals entirely, and FPL_NO_XGC_REPAIR removes only
// the conceded half while leaving the attacking backfill in place.
func TestTheXGCRepairHasAWorkingEscapeHatch(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	const name = "2021-22"

	count := func(t *testing.T) (rows int, sum float64) {
		t.Helper()
		// Load rather than loadSeason: the process-global cache would hand back
		// whichever arm got there first, which is precisely the failure being
		// guarded against.
		s, err := Load(t.Context(), cfg.CacheDir, name)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range sortedSeasonPlayerIDs(s) {
			for _, g := range s.Players[id].GWs {
				if g.XGC != 0 {
					rows++
					sum += g.XGC
				}
			}
		}
		return rows, sum
	}

	onRows, onSum := count(t)
	if onRows == 0 {
		t.Fatal("no xGC at all with the repair on, so the hatch below would pass on a corpse")
	}
	t.Logf("%s repaired: %d rows carrying xGC, %.1f total", name, onRows, onSum)

	t.Run("FPL_NO_XGC_REPAIR", func(t *testing.T) {
		t.Setenv("FPL_NO_XGC_REPAIR", "1")
		rows, _ := count(t)
		if rows != 0 {
			t.Errorf("%d rows still carry xGC with the reconstruction switched off. "+
				"This season has no expected_goals_conceded column in the archive, "+
				"so anything here came from the repair and the switch did not reach "+
				"it", rows)
		}
	})

	t.Run("FPL_NO_XG_REPAIR", func(t *testing.T) {
		t.Setenv("FPL_NO_XG_REPAIR", "1")
		rows, _ := count(t)
		if rows != 0 {
			t.Errorf("%d rows carry xGC with the whole backfill off. The chain is fed "+
				"by repaired xG, so with no xG there is nothing to pair through the "+
				"fixture list and this must be zero", rows)
		}
	})

	// And back on afterwards, byte for byte. A hatch that leaves the process in a
	// different state than it found it turns every later cell in the same binary
	// into a silently different arm.
	backRows, backSum := count(t)
	if backRows != onRows || math.Abs(backSum-onSum) > 1e-9 {
		t.Errorf("after the hatches, %d rows and %.6f total, want %d and %.6f — "+
			"the repair is not reproducible across a toggle",
			backRows, backSum, onRows, onSum)
	}
}
