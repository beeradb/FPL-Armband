package backtest

import (
	"fmt"
	"os"
	"testing"
)

// DOES THE UNKNOWN-PLAYER OVER-STATEMENT SURVIVE THE FIRST WHISTLE?
//
//	FPL_INSEASON_CSV=/tmp/inseason.csv DIAG=1 go test ./internal/backtest \
//	    -run TestDiagInSeasonUnknownLevel -v -count=1 -timeout 60m
//
// # Why this exists, and why the pre-season answer does not cover it
//
// A companion diagnostic measured, on the PRE-SEASON engine, that the league
// fallback over-states a player with no prior-season minutes: against the known
// population scored identically, MID 1.89x and FWD 1.48x resolve under Holm,
// DEF 1.55x and GK 4.47x do not, and all four are above 1 in all six seasons.
//
// `Weights.UnknownPriorShare` is the lever that would correct it. **It cannot
// correct any of the above once a ball has been kicked**, because it is applied
// in `unknownPriorRates`, which `blendRatesCode` reaches only from its
// `!SeasonHasStarted()` branch. The in-season path calls `shrinkToLeague`
// directly and never sees the share.
//
// That was checked through the surface rather than read off the code: at GW2 of
// a live season, `armband transfers` is byte-identical with the share at 1.0 and
// at 0.55.
//
// So there are two separate questions and the pre-season measurement answers
// only the first:
//
//	pre-season   is the opening fifteen built on over-stated unknowns?  MEASURED
//	in-season    is a GW2-GW10 TRANSFER TARGET over-stated the same way?  here
//
// The second is the one that decides whether anything is wrong with a live
// season *now*, and it is not implied by the first. The in-season path weights
// the league fallback by the player's own accumulated minutes,
// `wMin = n90/(n90 + BlendMinutesK)`, so early in a season the fallback
// dominates and late in one it does not. Whether that produces the same excess,
// a smaller one, or none, is an empirical question about a different code path.
//
// # What is measured
//
// The same ratio-of-ratios the pre-season diagnostic uses, so the two are
// directly comparable: predicted per-match minutes over actual, for the
// no-history stratum, divided by the same quantity for the known stratum in the
// same season and position. Conventions that would otherwise have to be argued
// about — what `ExpectedMinutes` is a rate of, how blank gameweeks enter a
// window average — cancel in that ratio.
//
// Entry is after GW1 has been played, which is the earliest point that is
// genuinely in-season and the point a manager is at when this matters most.
//
// ⚠️ **Ordering is deliberately NOT re-measured here.** The pre-season finding
// was that the fallback is constant within a position and therefore orders
// nobody; in-season it is mixed with the player's own accumulated rate, so it is
// no longer constant and a rho would be measuring the mixture rather than the
// fallback. That is a different question and it needs its own design.
//
// # PRE-REGISTERED, before running
//
//   - **If the over-statement is a property of the league fallback**, it appears
//     here too at GW2, at a similar or slightly smaller size — the fallback still
//     dominates when a player has almost no minutes of his own.
//   - **If it was an artefact of the pre-season path specifically**, the excess
//     here is near 1 and nothing is wrong with a live season.
//   - **If it is LARGER here**, that would be a surprise and would want
//     explaining before it is acted on, not after.
//
// ⚠️ Whatever this shows, it prices nothing. It says whether a miscalibration
// reaches the live path, not whether correcting it wins points — and correcting
// a measured bias has lost this project points five times.
func TestDiagInSeasonUnknownLevel(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	// Entry after GW1, and the window that follows it. Ten gameweeks to match
	// the pre-season diagnostic's window exactly, so the two excesses are
	// comparable rather than merely similar-looking.
	const through = 1
	const window = 10

	fmt.Printf("\n=== DOES THE UNKNOWN OVER-STATEMENT REACH THE IN-SEASON PATH?\n")
	fmt.Printf("Entry after GW%d; predicted per-match minutes against GW%d-%d actual.\n",
		through, through+1, through+window)
	fmt.Printf("⚠️ Ratio of the NO-history stratum's over-statement to the KNOWN\n")
	fmt.Printf("stratum's, so the units and the blank-gameweek convention cancel.\n")
	fmt.Printf("Above 1 is over-statement. The pre-season reading was GK 4.47,\n")
	fmt.Printf("DEF 1.55, MID 1.89, FWD 1.48 — compare against those.\n\n")
	fmt.Printf("  %-9s %-4s %6s %10s %10s %8s\n",
		"season", "pos", "n", "pred/act", "known", "excess")

	var csv *os.File
	if path := os.Getenv("FPL_INSEASON_CSV"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("FPL_INSEASON_CSV=%s: %v", path, err)
		}
		defer f.Close()
		csv = f
		fmt.Fprintln(csv, "season,stratum,pos,n,pred_off,pred_on,actual,window,price_tilt")
	}

	sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
	for _, pr := range loadPairsOrSkip(t, cfg) {
		// ⚠️ `through` is the point-in-time boundary, so nothing after GW1 is
		// visible to the engine. That is the whole reason this is a replay
		// question and not one a live season can answer: the answer needs
		// minutes the model must not be allowed to see.
		e, _ := EngineAt(pr.Cur, pr.Prior, through, sc)

		priorMins := map[int]int{}
		if pr.Prior != nil {
			for _, q := range pr.Prior.Players {
				if q.Code != 0 {
					priorMins[q.Code] += q.Minutes
				}
			}
		}

		// Accumulated per (stratum, position): the predicted rate, the realised
		// window total, and the count.
		type acc struct {
			pred, actual float64
			n            int
		}
		cells := map[string]map[int]*acc{}

		for id, p := range pr.Cur.Players {
			el := e.Boot.ElementByID(id)
			if el == nil {
				continue
			}
			var actual float64
			for gw := through + 1; gw <= through+window; gw++ {
				if g, ok := p.GWs[gw]; ok {
					actual += float64(g.Minutes)
				}
			}
			key := "has history"
			if priorMins[p.Code] == 0 {
				key = "NO history"
			}
			if cells[key] == nil {
				cells[key] = map[int]*acc{}
			}
			a := cells[key][el.ElementType]
			if a == nil {
				a = &acc{}
				cells[key][el.ElementType] = a
			}
			a.pred += e.Metrics(el).ExpectedMinutes
			a.actual += actual
			a.n++
		}

		for _, key := range []string{"NO history", "has history"} {
			for _, pos := range []int{1, 2, 3, 4} {
				a := cells[key][pos]
				if a == nil || a.n < 4 {
					continue
				}
				n := float64(a.n)
				if csv != nil {
					fmt.Fprintf(csv, "%s,%s,%s,%d,%.4f,%.4f,%.4f,%d,%.3f\n",
						pr.Name, key, positionNames[pos], a.n,
						a.pred/n, a.pred/n, a.actual/n, window, 0.0)
				}
			}
			// Printed per position beside the known stratum's own figure, so a
			// reader sees the control rather than being asked to trust it.
			for _, pos := range []int{1, 2, 3, 4} {
				u := cells[key][pos]
				k := cells["has history"][pos]
				if u == nil || k == nil || u.n < 4 || k.n < 4 || key != "NO history" {
					continue
				}
				ur := (u.pred / float64(u.n)) / ((u.actual / float64(u.n)) / window)
				kr := (k.pred / float64(k.n)) / ((k.actual / float64(k.n)) / window)
				fmt.Printf("  %-9s %-4s %6d %10.3f %10.3f %8.3f\n",
					pr.Name, positionNames[pos], u.n, ur, kr, ur/kr)
			}
		}
	}

	fmt.Printf("\n⚠️ Per season and position, printed rather than pooled: the\n")
	fmt.Printf("inference is R's, on stats/unknown_prior_ranks.R --levels, which\n")
	fmt.Printf("reads the CSV this writes in the same schema as the pre-season one.\n")
	fmt.Printf("⚠️ A near-1 excess here means the pre-season finding does NOT reach\n")
	fmt.Printf("a live season, and that UnknownPriorShare being pre-season-only is\n")
	fmt.Printf("correct rather than a gap. A large one means the opposite.\n")
}
