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

	// Ten gameweeks to match the pre-season diagnostic's window exactly, so the
	// excesses are comparable rather than merely similar-looking.
	const window = 10

	// ⚠️ **Several entry points, because ONE cannot tell a persistent defect
	// from an early-season transient**, and those want different fixes.
	//
	// The hypothesis under test is that `shrinkToLeague` weights a player's own
	// rate by `wMin = n90/(n90 + BlendMinutesK)`, so a player who does not play
	// keeps `n90 = 0`, keeps `wMin = 0`, and keeps receiving the league average
	// in full — the evidence that would displace it being the very quantity that
	// stays zero.
	//
	// It makes a falsifiable split prediction, and the split is the test:
	//
	//	no prior, HAS played by now    the model has evidence -> excess shrinks
	//	no prior, STILL has not        the model never learns -> excess persists
	//
	// If instead both decay, this is a transient nobody needs to fix. If both
	// persist, the mechanism is not what is claimed here and the story is wrong
	// even though the defect is real.
	entries := []int{1, 5, 10, 20}

	fmt.Printf("\n=== DOES THE UNKNOWN OVER-STATEMENT PERSIST, OR IS IT A TRANSIENT?\n")
	fmt.Printf("Predicted per-match minutes against the next %d gameweeks actual,\n", window)
	fmt.Printf("as a ratio to the KNOWN stratum measured identically, so the units\n")
	fmt.Printf("and the blank-gameweek convention cancel. Above 1 is over-statement.\n")
	fmt.Printf("Pre-season read GK 4.47, DEF 1.55, MID 1.89, FWD 1.48.\n\n")
	fmt.Printf("⚠️ The no-prior players are SPLIT on whether they have played yet.\n")
	fmt.Printf("The claim under test is that the model cannot learn from a player\n")
	fmt.Printf("who does not play, because the weight on his own rate is driven by\n")
	fmt.Printf("the minutes he has not got. If so, 'unplayed' stays high all season\n")
	fmt.Printf("while 'played' falls away.\n\n")
	fmt.Printf("  %-6s %-9s %-4s %6s %10s %10s %8s\n",
		"entry", "season", "pos", "n", "pred/act", "known", "excess")

	var csv *os.File
	if path := os.Getenv("FPL_INSEASON_CSV"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("FPL_INSEASON_CSV=%s: %v", path, err)
		}
		defer f.Close()
		csv = f
		fmt.Fprintln(csv, "season,stratum,pos,n,pred_off,pred_on,actual,window,price_tilt,entry_gw")
	}

	sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
	pairs := loadPairsOrSkip(t, cfg)
	for _, through := range entries {
		for _, pr := range pairs {
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
				// ⚠️ The split that tests the mechanism. "Played" means he has any
				// minutes at all BEFORE the entry point — the evidence `wMin` is
				// built from. A player with none has `n90 = 0`, so his own rate
				// carries no weight and he takes the league average whole.
				var playedSoFar int
				for gw := 1; gw <= through; gw++ {
					if g, ok := p.GWs[gw]; ok {
						playedSoFar += g.Minutes
					}
				}
				key := "has history"
				if priorMins[p.Code] == 0 {
					key = "unknown unplayed"
					if playedSoFar > 0 {
						key = "unknown played"
					}
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

			for _, key := range []string{"unknown unplayed", "unknown played", "has history"} {
				for _, pos := range []int{1, 2, 3, 4} {
					a := cells[key][pos]
					if a == nil || a.n < 4 {
						continue
					}
					n := float64(a.n)
					if csv != nil {
						// ⚠️ The entry point is its OWN COLUMN, not glued into the
						// stratum name. A composite key reads fine until something
						// downstream wants to group by one half of it — and a
						// stratum name carrying a COMMA silently shifts every
						// later column, which is how the first version of this
						// file wrote an unreadable CSV.
						fmt.Fprintf(csv, "%s,%s,%s,%d,%.4f,%.4f,%.4f,%d,%.3f,%d\n",
							pr.Name, key, positionNames[pos], a.n,
							a.pred/n, a.pred/n, a.actual/n, window, 0.0, through)
					}
				}
				// Printed per position beside the known stratum's own figure, so a
				// reader sees the control rather than being asked to trust it.
				for _, pos := range []int{1, 2, 3, 4} {
					u := cells[key][pos]
					k := cells["has history"][pos]
					if u == nil || k == nil || u.n < 4 || k.n < 4 || key == "has history" {
						continue
					}
					if u.actual == 0 {
						// Nobody in the cell played a single minute in the window.
						// The ratio is undefined, and printing a huge number would
						// read as a measurement rather than a division by nothing.
						fmt.Printf("  GW%-4d %-9s %-4s %6d %10s %10s %8s  %s\n",
							through, pr.Name, positionNames[pos], u.n,
							"none", "-", "-", key)
						continue
					}
					ur := (u.pred / float64(u.n)) / ((u.actual / float64(u.n)) / window)
					kr := (k.pred / float64(k.n)) / ((k.actual / float64(k.n)) / window)
					fmt.Printf("  GW%-4d %-9s %-4s %6d %10.3f %10.3f %8.3f  %s\n",
						through, pr.Name, positionNames[pos], u.n, ur, kr, ur/kr, key)
				}
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
