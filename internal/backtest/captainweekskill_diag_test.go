package backtest

// Can this model's own expected-points projection pick a triple-captain week?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCaptainWeekSkill -v -timeout 20m
//
// # Why this exists, and why the replay could not answer it
//
// `TestDiagTripleCaptainMatchup` asks whether timing the chip on projected
// points beats a fixed offset, scored on POLICY over a season path. That
// comparison's detection threshold is 4.19 a season-path and it reads +2.25.
//
// ⚠️ The original justification for this file was that the replay's threshold
// (11.3) EXCEEDED the gain the projections imply (~6.8), so a perfect rule would
// still read "unresolved". Both numbers were per_gw readings of an event count
// and that argument is withdrawn — the replay can resolve an effect this size.
// What survives is that this instrument is far better powered and answers a
// different question, the one the product turns on: if our own expected points
// cannot pick the week, the feature has no basis.
//
// This measures that directly. It compares the REALISED points the chosen
// captain actually scored in the chosen week against the same player in the
// week a fixed offset would have picked. No squad, no transfers, no season
// path — one decision, scored on what happened.
//
// # Why this is better powered
//
// The unit is a DECISION, not a season. Six seasons x six entry points is 36
// decisions rather than 6 season-paths, and every one of them is a direct
// comparison of two realised numbers with none of the replay's transfer noise
// between them. The cost of that is that it measures the chip's own payoff and
// not its effect on a squad — which is the right trade for this question.
//
// # ⚠️ What a positive result here would and would not license
//
// It would say the projections rank weeks better than an arbitrary offset does.
// It would NOT say the chip is worth playing, nor reproduce a season-points
// figure — for that the replay is still the instrument, and it still cannot
// resolve an effect this size. Do not add these numbers to a POLICY total.
//
// ⚠️ The chosen player is the best of `captainCandidates` by SEASON xPoints at
// the entry cutoff, which is a proxy for ownership rather than a squad — see
// captainweek.go. A premium this squad never bought would overstate the gain.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagCaptainWeekSkill(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	starts := sweepStarts()

	fmt.Printf("\n=== can our own xPoints pick a triple-captain week?\n")
	fmt.Printf("Realised points scored by the chosen captain in the chosen week,\n")
	fmt.Printf("against the same rule's player in a fixed-offset week. One\n")
	fmt.Printf("decision per cell; no squad and no transfer path.\n\n")
	fmt.Printf("%-9s %5s  %-14s %5s %6s %6s  %5s %6s %6s  %7s\n",
		"season", "entry", "captain", "aGW", "aProj", "aReal", "cGW", "cProj", "cReal", "diff")

	var diffs []float64
	var projs []float64
	var wins, losses, ties int
	bySeason := map[string][]float64{}
	var seasonNames []string

	for _, pair := range loadPairsOrSkip(t, cfg) {
		for _, start := range starts {
			sc := SimConfig{Weights: cfg.Weights, StartGW: start}

			// start-1: EngineAt's `through` is the last COMPLETED gameweek. See
			// the ChipPlannerXP branch in Simulate for what passing `start` cost.
			through := start - 1

			seasonEngine, boot := EngineAt(pair.Cur, pair.Prior, through, sc)
			cands := TopCaptainCandidates(seasonEngine, captainCandidates)
			if len(cands) == 0 {
				continue
			}
			weekCfg := sc
			weekCfg.Weights.Horizon = 1
			weekEngine, _ := EngineAt(pair.Cur, pair.Prior, through, weekCfg)
			capXP := BestCaptainXPByGameweek(weekEngine, start, cands)
			if len(capXP) == 0 {
				continue
			}

			aGW := tcMatchupAnchored(pair.Cur, start, capXP).TripleCaptain
			cGW := tcMatchupControl(pair.Cur, start, capXP).TripleCaptain
			if aGW == 0 || cGW == 0 {
				continue
			}
			// ⚠️ A cell where the two rules AGREE is a true zero and stays in the
			// denominator. Dropping it would make this "mean gain per DISCORDANT
			// decision", which is not what the figure is read as and inflates any
			// season-scale conversion built on it.

			// Who the rule would actually captain in each week: the best of the
			// candidates in THAT week, which is the same choice the timing rule
			// made when it scored the week.
			aID, aProj := bestCandidateIn(weekEngine, cands, aGW)
			cID, cProj := bestCandidateIn(weekEngine, cands, cGW)
			if aID == 0 || cID == 0 {
				continue
			}

			aReal := realisedPoints(pair.Cur, aID, aGW)
			cReal := realisedPoints(pair.Cur, cID, cGW)
			d := float64(aReal - cReal)
			diffs = append(diffs, d)
			if _, seen := bySeason[pair.Cur.Name]; !seen {
				seasonNames = append(seasonNames, pair.Cur.Name)
			}
			bySeason[pair.Cur.Name] = append(bySeason[pair.Cur.Name], d)
			projs = append(projs, aProj-cProj)
			switch {
			case d > 0:
				wins++
			case d < 0:
				losses++
			default:
				ties++
			}

			name := "?"
			if el := boot.ElementByID(aID); el != nil {
				name = el.WebName
			}
			fmt.Printf("%-9s %5d  %-14s %5d %6.1f %6d  %5d %6.1f %6d  %+7.0f\n",
				pair.Cur.Name, start, name, aGW, aProj, aReal, cGW, cProj, cReal, d)
		}
	}

	if len(bySeason) < 2 {
		t.Skip("not enough seasons")
	}
	// ⚠️ NO INFERENCE HERE, deliberately. Season-clustered SEs are computed in
	// stats/sweep_inference.R and nowhere else, and `TestInferenceLivesInOnePlace`
	// enforces it — a second implementation is how two stale critical values
	// survived a grid widening. This block used to compute the clustered SE and t
	// itself and was caught by that guard.
	//
	// The unit here is a DECISION, not a season-path cell, so this is not a sweep
	// and cannot emit cells for R. What it emits instead is the per-season means,
	// which is the input any clustered reading needs — and the clustering is then
	// an explicit choice the reader makes, not one buried in a diagnostic.
	//
	// ⚠️ Clustering IS the right reading, and the reason is worth stating where
	// the numbers are produced: cells inside a season share that season's archive,
	// so treating them as independent overstates t — it did, 4.11 against 2.92,
	// until review caught it. The same objection retires a "30 of 34" win count:
	// 34 cells are not 34 binomial trials when the effective n is the season
	// count. Read the per-season row, cluster on it, and quote the threshold.
	fmt.Printf("\ncells %d over %d seasons | anchored better in %d, worse in %d, tied in %d\n",
		len(diffs), len(seasonNames), wins, losses, ties)
	fmt.Printf("⚠️ the win/loss count is NOT %d independent trials — the effective n is "+
		"the season count. Do not read it as a binomial.\n", len(diffs))
	fmt.Printf("\nper-season mean realised gain (pts/chip) — CLUSTER ON THIS ROW:\n  ")
	for _, s := range seasonNames {
		fmt.Printf("%s %+.2f   ", s, meanOf(bySeason[s]))
	}
	fmt.Printf("\n\nunclustered cell mean: realised %+.2f, projected %+.2f pts/chip\n",
		meanOf(diffs), meanOf(projs))
	fmt.Printf("⚠️ that cell mean carries NO standard error on purpose. Take the SE and t " +
		"from the per-season row above, against tCrit95(seasons-1), and report the " +
		"threshold beside the effect.\n")
	fmt.Printf("\nThe projected column is what the rule BELIEVED it was buying; the\n")
	fmt.Printf("realised column is what it got. A realised gain near the projected\n")
	fmt.Printf("one says the projections rank weeks honestly. A realised gain near\n")
	fmt.Printf("zero says they do not, whatever the season-points replay reports.\n")
	fmt.Printf("⚠️ These are chip-week points, NOT a season total. Do not add them\n")
	fmt.Printf("to a POLICY figure.\n")
}

// bestCandidateIn is which candidate projects highest in one gameweek, and by
// how much — the same choice the timing rule made when it scored that week, so
// the two cannot disagree about who is being captained.
//
// It isolates the gameweek exactly as BestCaptainXPByGameweek does, and leaves
// the skip set empty on return.
func bestCandidateIn(week *analysis.Engine, cands []int, gw int) (int, float64) {
	if week == nil || gw < 1 {
		return 0, 0
	}
	defer week.SetSkipGameweeks(nil)
	skip := make([]int, 0, 37)
	for other := 1; other <= 38; other++ {
		if other != gw {
			skip = append(skip, other)
		}
	}
	week.SetSkipGameweeks(skip)

	want := make(map[int]bool, len(cands))
	for _, id := range cands {
		want[id] = true
	}
	bestID, best := 0, 0.0
	for _, m := range week.AllMetrics() {
		if want[m.ID] && m.Score > best {
			bestID, best = m.ID, m.Score
		}
	}
	return bestID, best
}

func realisedPoints(cur *Season, id, gw int) int {
	for _, p := range cur.Players {
		if p.ID == id {
			return p.GWs[gw].Points
		}
	}
	return 0
}
