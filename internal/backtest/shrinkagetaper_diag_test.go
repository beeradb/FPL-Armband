package backtest

import (
	"fmt"
	"os"
	"testing"
)

// WHAT IS THE RESIDUAL MADE OF, AND WOULD TAPERING THE PRIOR REMOVE IT?
//
//	FPL_TAPER_CSV=/tmp/taper.csv DIAG=1 go test ./internal/backtest \
//	    -run TestDiagShrinkageTaper -v -count=1 -timeout 60m
//
// # Two questions that turn out to be one
//
// **Do successive benchings need to penalise harder?** `blankRunFactor` is FLAT
// — 0.75 for a run of 1 to 3 — and returns 1, no penalty at all, for a run of 4
// or more. Its comment argues that by four the exponential average has caught up
// on its own and a further discount would double-count.
//
// **Does the prior need a taper?** The volume weight is `n/(n+k)` with
// `BlendMinutesK = 5`. That asymptotes to 1 and never reaches it, so a player
// with twenty matches of evidence still carries 5/25 = 20% of the league average
// he is being compared against.
//
// They are the same question asked from two ends: after a long run of zeroes,
// what is left of the league term, and is what remains the thing that keeps the
// estimate high?
//
// # ⚠️ A story that did NOT survive its own arithmetic
//
// The obvious account is that the residual is exactly the retained league
// weight: predicted = (1-w) x league. Checked against the measured figures it
// does not close. At GW20 the unplayed group reads 8.6 with w = 0.8, implying a
// league term of 43; at GW1 it reads 59.3 with w = 1/6, implying 71. Those
// cannot both be the same league average.
//
// The composition of the "unplayed" group changes with the entry point — more
// players, different positions, and `leagueRates` are themselves recomputed from
// the current season — so the two are not comparable and the identity cannot be
// inverted. That is why this diagnostic VARIES k and reads the response, rather
// than asserting a decomposition.
//
// # What is measured
//
// The same predicted-against-actual the in-season diagnostic uses, for the
// no-prior players who have not played, at several entry points and several
// values of `BlendMinutesK`. One engine per (entry, k), the same players scored
// by each, so the population is held fixed and k is the only declared variable.
//
// # PRE-REGISTERED, before running
//
//   - **If the residual is the retained league weight**, lowering k moves the
//     estimate toward the actual roughly in proportion to the change in
//     `(1-n/(n+k))`, and the effect GROWS with the entry gameweek, because n does.
//   - **If it is something else** — the recency average not reading zero, a
//     floor, an override — the response to k is weak and flat across entry
//     points, and a taper is not the lever.
//   - ⚠️ **Either way this is calibration and prices nothing.** k is a live
//     scoring constant tuned on the WHOLE population against a minutes MAE; that
//     the tail wants a different value is not on its own an argument for moving
//     it, and "the tail wants it lower" is exactly the shape an argmax produces.
//     A tuning claim needs the replay, not this.
func TestDiagShrinkageTaper(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	const window = 10
	entries := []int{1, 5, 10, 20}
	// Shipped is 5. Its own calibration comment records the minimum as flat from
	// 3 to 8, so 2 and 12 sit just outside the range that calibration could not
	// separate — far enough to move the answer if k is the lever at all.
	ks := []float64{2, 5, 12}

	fmt.Printf("\n=== IS THE RESIDUAL THE RETAINED LEAGUE WEIGHT?\n")
	fmt.Printf("No-prior players who have NOT played, by entry gameweek and by\n")
	fmt.Printf("BlendMinutesK. Shipped k is %.0f. `1-w` is the share of the league\n",
		cfg.Weights.BlendMinutesK)
	fmt.Printf("average still carried at that many matches of evidence.\n\n")
	fmt.Printf("  %-6s %5s %6s %6s %9s %9s\n",
		"entry", "k", "1-w", "n", "predicted", "actual")

	var csv *os.File
	if path := os.Getenv("FPL_TAPER_CSV"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("FPL_TAPER_CSV=%s: %v", path, err)
		}
		defer f.Close()
		csv = f
		fmt.Fprintln(csv, "season,entry_gw,k,n,pred,actual,window")
	}

	pairs := loadPairsOrSkip(t, cfg)
	for _, through := range entries {
		for _, k := range ks {
			sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
			sc.Weights.BlendMinutesK = k

			var pred, actual float64
			var n int
			for _, pr := range pairs {
				e, _ := EngineAt(pr.Cur, pr.Prior, through, sc)

				priorMins := map[int]int{}
				if pr.Prior != nil {
					for _, q := range pr.Prior.Players {
						if q.Code != 0 {
							priorMins[q.Code] += q.Minutes
						}
					}
				}

				var sp, sa float64
				var sn int
				for id, p := range pr.Cur.Players {
					if priorMins[p.Code] != 0 {
						continue // has a prior season; a different population
					}
					el := e.Boot.ElementByID(id)
					if el == nil {
						continue
					}
					var played int
					for gw := 1; gw <= through; gw++ {
						if g, ok := p.GWs[gw]; ok {
							played += g.Minutes
						}
					}
					if played > 0 {
						continue // has played; the group that is already calibrated
					}
					var got float64
					for gw := through + 1; gw <= through+window; gw++ {
						if g, ok := p.GWs[gw]; ok {
							got += float64(g.Minutes)
						}
					}
					sp += e.Metrics(el).ExpectedMinutes
					sa += got / window
					sn++
				}
				if sn == 0 {
					continue
				}
				pred += sp
				actual += sa
				n += sn
				if csv != nil {
					fmt.Fprintf(csv, "%s,%d,%.0f,%d,%.4f,%.4f,%d\n",
						pr.Name, through, k, sn, sp/float64(sn), sa/float64(sn), window)
				}
			}
			if n == 0 {
				continue
			}
			// The share of the league term still carried, at this entry point's
			// typical evidence count. `through` stands in for n: a club has
			// played about one match per gameweek.
			retained := k / (float64(through) + k)
			fmt.Printf("  GW%-4d %5.0f %6.3f %6d %9.1f %9.1f\n",
				through, k, retained, n, pred/float64(n), actual/float64(n))
		}
	}

	fmt.Printf("\n⚠️ Read the RESPONSE to k, not the levels. If the residual is\n")
	fmt.Printf("the retained league weight, predicted tracks `1-w` and the spread\n")
	fmt.Printf("across k widens with the entry gameweek. If it barely moves, the\n")
	fmt.Printf("league term is not what is holding the estimate up and a taper is\n")
	fmt.Printf("the wrong lever.\n")
	fmt.Printf("⚠️ k is a LIVE scoring constant calibrated on the whole population.\n")
	fmt.Printf("That this tail prefers a different value is not an argument for\n")
	fmt.Printf("moving it — that is what an argmax over a subgroup looks like.\n")
}
