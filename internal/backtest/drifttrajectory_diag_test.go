package backtest

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// IS THE GAP FROM IDEAL MONOTONIC, AND DOES IT PLATEAU?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDriftTrajectory -v -count=1 -timeout 180m
//
// # Why this is a different question from every wildcard sweep in the record
//
// Every trigger measured so far collapses a season to ONE number — did firing on
// reading X beat not firing — and none of them resolved. This measures the SHAPE
// of the gap instead: one trajectory of up to 38 readings per cell rather than
// one number, which is a different estimand and enormously better powered.
//
// # What the shape decides, which is the whole point
//
// Let `D(s)` be the gap from ideal `s` weeks after a reset, and let a wildcard
// reset it to zero. Playing at week `t` of a remaining window `W` costs
//
//	∫₀ᵗ D + ∫₀^(W−t) D
//
// whose derivative is `D(t) − D(W−t)`. So:
//
//   - **If D rises without limit, the optimum is t = W/2** — the midpoint of the
//     remaining season, which no arm in this record has tested.
//   - **If D plateaus at level L after week k, the derivative is ZERO for every t
//     with both t and W−t past k.** Every firing time in [k, W−k] is then exactly
//     as good as every other.
//
// ⚠️ **The second case would explain every null in the chip record.** With W = 38
// and a knee anywhere around 10, the flat region is roughly GW10-GW28 — and every
// drift trigger measured fires inside it (median GW 14-15). A flat optimum is not
// a weak effect; it is the absence of a decision, and it reads in a points sweep
// exactly like noise.
//
// It also predicts which arm should look BAD: the shipped cost rule fires at
// median GW 9, outside that window on the early side, and it is the one arm that
// reads clearly negative. That is a prediction this diagnostic can refute.
//
// # The three trajectories
//
//  1. **FROZEN** — the opening fifteen, never transferred. The literal conjecture.
//  2. **ONE FREE PER WEEK** — at most one change a week, never a hit. This is the
//     real alternative to a wildcard, and the one a rule of thumb about hits lives
//     in: it is the gap that accumulates DESPITE spending the allowance.
//  3. **FLOOR** — last week's optimum, read this week. ⚠️ **A gap from a fresh
//     argmax is never zero**, so a rising frozen curve proves nothing until the
//     floor curve is subtracted. This is that control, and it is also the direct
//     reading of how much one gameweek of data rewrites the world.
//
// # The ledger: cost of moving against cost of waiting
//
// The gap is a RATE — points bled per week by not acting — so the decision is a
// ledger, not a threshold. **Waiting costs the gap every week it continues.
// Moving costs 4 points a hit, once.** The question is therefore never "is the
// gap big" but "how many weeks until moving pays for itself".
//
// At sampled weeks the squad is revised with at most `k` changes and then **held
// forward unchanged for `driftHoldWeeks` beside the frozen one**, both scored
// week by week. `OptimizeRequest.MaxChanges` counts the difference from the
// current fifteen rather than the moves taken, so the search cannot sell a player
// and buy him back, and the ladder is an honest best-k.
//
// ⚠️ **Persistence is the whole question and a snapshot cannot see it.** Closing
// six points today is worth eight points of hits only if the edge SURVIVES; if
// the ideal churns away next week, the transfer bought one gameweek and the hits
// were spent on nothing. An earlier version of this diagnostic reported the
// day-of closure and would have made hits look unconditionally cheap.
//
// # Three things that would fake a plateau
//
//  1. **The gap is bounded above** by the distance from the ideal XI to the worst
//     legal one. A flat tail may be saturation rather than equilibrium. Printed
//     beside the curve so it can be checked rather than assumed.
//  2. **The selling tax grows all season** — a squad that never sells can afford
//     less of the ideal as prices rise, so part of any rise is money, not decay.
//     The frozen budget is printed for that reason.
//  3. **Late entries have short windows**, so the tail of the mean curve is a mean
//     over fewer and different cells. The cell count is printed at every step.
var driftLadderMax = 6

// driftLadderEvery samples the ladder rather than running it weekly: it costs
// driftLadderMax optimisations a week where the trajectory costs two, and the
// knee does not need weekly resolution.
var driftLadderEvery = 5

// driftHoldWeeks is how long a revised squad is held before the ledger is read.
//
// ⚠️ **This is the whole difference between a snapshot and a decision.** Closing
// the gap this week is worth a hit only if the edge survives; if the ideal churns
// away immediately, the transfer bought one gameweek and the 4 points were spent
// on nothing. Eight weeks is long enough for a payback to appear and short enough
// that most sampled weeks still have room before GW38.
var driftHoldWeeks = 8

// holdPoint is one (weeks held, cumulative gain) reading, so the payback curve is
// built from measurements rather than from dividing a total by a horizon.
type holdPoint struct {
	week int
	gain float64
}

func TestDiagDriftTrajectory(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	starts := sweepStarts()

	fmt.Printf("\n=== THE GAP FROM IDEAL, WEEK BY WEEK, IN POINTS ON THE XI\n")
	fmt.Printf("Aligned on WEEKS SINCE ENTRY, not on gameweek: the conjecture is\n")
	fmt.Printf("about accumulation since a reset, and entries differ.\n")
	fmt.Printf("⚠️ Read the FLOOR column before the frozen one. A gap from a fresh\n")
	fmt.Printf("argmax is never zero, so only the excess over the floor is decay.\n\n")

	// series[s] accumulates every cell's reading at s weeks since entry.
	type step struct{ frozen, free, floor, budget []float64 }
	series := map[int]*step{}
	byGap := map[int]map[int][]float64{} // gap bucket -> k -> cumulative gain
	hold := map[int][]holdPoint{}        // k -> (weeks held, cumulative gain)
	// ⚠️ **Stratified by the GAMEWEEK the decision was taken**, which is the same
	// question the wildcard timing asks at a smaller scale and forty times a
	// season instead of once. A transfer at GW2 chases an ideal built from one
	// gameweek of data; if that ideal is mostly noise, the player bought is not
	// the one who still looks right at GW6 and the hit was spent on a target that
	// evaporated. If early moves pay back less than identical late ones, early
	// hits are a gamble — measured rather than asserted.
	byWeek := map[int]map[int][]float64{} // week band -> k -> gain over the hold
	// ⚠️ Per ENTRY POINT as well as pooled. The pooled curve is a mean over a
	// changing set of cells — entries run out of season at different weeks, and
	// the biggest jumps in the pooled column sit exactly on the steps where cells
	// drop out. A cohort keeps the same cells at every week, so a plateau in one
	// is a plateau rather than a change of population.
	cohort := map[int]map[int][]float64{} // entry -> weeks since entry -> gap
	// Per CELL, because the detection question is whether one squad's optimal
	// week differs from another's — a mean trajectory cannot answer that.
	var runs []driftRun
	var spear, incUp []float64

	for _, pr := range loadPairsOrSkip(t, cfg) {
		for _, start := range starts {
			sc := SimConfig{Weights: cfg.Weights, StartGW: start, BankUpTo: 5}
			// ⚠️ **Horizon 1, and the single-gameweek reading is the POINT.**
			// The gap is a RATE — points bled per week by not acting — and that
			// is the unit the decision is denominated in: waiting costs the gap
			// every week it continues, moving costs 4 a hit once. Averaging the
			// gap over a five-week window smooths away the very quantity the
			// ledger needs, and was a wrong turn taken and reversed here.
			//
			// It also means a large one-week floor is a RESULT rather than a
			// nuisance: if an optimum one week old already carries most of the
			// gap, then most of the gap is churn no transfer can catch, and
			// chasing it is chasing noise. That is only visible at horizon 1.
			sc.Weights.Horizon = 1

			e0, _ := EngineAt(pr.Cur, pr.Prior, start-1, sc)
			frozen, ok := repairSquad(e0, nil, 1000, 0, sc)
			if !ok {
				continue
			}
			// One wallet per arm, so the frozen arm's budget cannot move with the
			// free arm's transfers — the confound clone() exists for.
			fw, vw := newWallet(1000), newWallet(1000)
			for _, id := range frozen {
				p := marketPrice(pr.Cur, id, start-1)
				fw.buy(id, p)
				vw.buy(id, p)
			}
			free := append([]int(nil), frozen...)
			var prevIdeal []int
			var cellD []float64

			for gw := start; gw <= 38; gw++ {
				ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
				fb := fw.value(pr.Cur, frozen, gw-1)
				ideal, ok := repairSquad(ew, nil, fb, 0, sc)
				if !ok {
					break
				}
				s := gw - start
				st := series[s]
				if st == nil {
					st = &step{}
					series[s] = st
				}
				dz := xiPoints(ew, ideal) - xiPoints(ew, frozen)
				st.frozen = append(st.frozen, dz)
				if cohort[start] == nil {
					cohort[start] = map[int][]float64{}
				}
				cohort[start][s] = append(cohort[start][s], dz)
				st.budget = append(st.budget, float64(fb)/10)
				cellD = append(cellD, dz)

				// The floor: last week's optimum read against this week's, which
				// is the argmax churn a squad cannot avoid by any decision.
				if prevIdeal != nil {
					st.floor = append(st.floor, xiPoints(ew, ideal)-xiPoints(ew, prevIdeal))
				}
				prevIdeal = ideal

				// The no-hit arm: at most one change this week, applied and kept.
				vb := vw.value(pr.Cur, free, gw-1)
				if revised, ok := reviseSquad(ew, free, vb, 0, sc, 1); ok {
					settle(pr.Cur, vw, free, revised, gw-1)
					free = revised
				}
				st.free = append(st.free, xiPoints(ew, ideal)-xiPoints(ew, free))

				if s%driftLadderEvery == 0 && s > 0 && gw+driftHoldWeeks <= 38 {
					// ⚠️ **The ledger, not a snapshot.** Revising with k changes
					// closes some of the gap THIS week; whether that was worth 4
					// a hit depends entirely on whether the edge SURVIVES. So
					// each revised squad is held forward unchanged beside the
					// frozen one and both are scored week by week, which is the
					// cost-of-moving against the cost-of-waiting directly.
					b := gapBucket(dz)
					if byGap[b] == nil {
						byGap[b] = map[int][]float64{}
					}
					for k := 1; k <= driftLadderMax; k++ {
						rev, ok := reviseSquad(ew, frozen, fb, 0, sc, k)
						if !ok {
							continue
						}
						var cum float64
						for w := 0; w < driftHoldWeeks; w++ {
							fe, _ := EngineAt(pr.Cur, pr.Prior, gw+w-1, sc)
							cum += xiPoints(fe, rev) - xiPoints(fe, frozen)
							hold[k] = append(hold[k], holdPoint{w + 1, cum})
						}
						byGap[b][k] = append(byGap[b][k], cum)
						wb := weekBand(gw)
						if byWeek[wb] == nil {
							byWeek[wb] = map[int][]float64{}
						}
						byWeek[wb][k] = append(byWeek[wb][k], cum)
					}
				}
			}
			runs = append(runs, driftRun{pr.Name, start, append([]float64(nil), cellD...)})
			if len(cellD) >= 6 {
				spear = append(spear, spearmanVsIndex(cellD))
				incUp = append(incUp, risingFraction(cellD))
			}
		}
	}
	if len(series) == 0 {
		t.Skip("no cells")
	}

	fmt.Printf("⚠️ `D(1)` is the gap carried by an optimum ONE week old — the churn no\n")
	fmt.Printf("decision avoids. It is not a noise floor to subtract: it is D at s=1,\n")
	fmt.Printf("so the honest read is whether D(s) grows AWAY from it.\n\n")
	fmt.Printf("%6s %6s  %9s %9s %9s  %9s %9s\n",
		"week", "cells", "FROZEN", "over D(1)", "1-free/wk", "D(1)", "budget")
	var keys []int
	for s := range series {
		keys = append(keys, s)
	}
	sort.Ints(keys)
	for _, s := range keys {
		st := series[s]
		if len(st.frozen) < 3 {
			continue
		}
		fl := meanOf(st.floor)
		fmt.Printf("%6d %6d  %9.2f %9.2f %9.2f  %9.2f %9.1f\n",
			s, len(st.frozen), meanOf(st.frozen), meanOf(st.frozen)-fl,
			meanOf(st.free), fl, meanOf(st.budget))
	}

	fmt.Printf("\n=== THE SAME CURVE PER ENTRY COHORT — composition held fixed\n")
	fmt.Printf("Each row is ONE entry point followed all season, so the cells never\n")
	fmt.Printf("change. A plateau here is a plateau; a plateau in the pooled table\n")
	fmt.Printf("above may only be short-window cells dropping out.\n\n")
	var entries []int
	for e := range cohort {
		entries = append(entries, e)
	}
	sort.Ints(entries)
	fmt.Printf("%6s %5s", "entry", "n")
	for w := 2; w <= 36; w += 4 {
		fmt.Printf(" %6s", fmt.Sprintf("s=%d", w))
	}
	fmt.Printf("\n")
	for _, e := range entries {
		fmt.Printf("%6d %5d", e, len(cohort[e][1]))
		for w := 2; w <= 36; w += 4 {
			if v := cohort[e][w]; len(v) > 0 {
				fmt.Printf(" %6.1f", meanOf(v))
			} else {
				fmt.Printf(" %6s", "-")
			}
		}
		fmt.Printf("\n")
	}

	// The worth of resetting, computed from the measured curve, against the
	// decay the engine already ships.
	printResetWorth(cohort)
	printFirstHalfChoice(runs)

	fmt.Printf("\n=== IS IT MONOTONIC? Per-cell Spearman of the gap against week\n")
	sort.Float64s(spear)
	pos := 0
	for _, r := range spear {
		if r > 0 {
			pos++
		}
	}
	fmt.Printf("cells %d | mean rho %+.3f | median %+.3f | rising in %d of %d\n",
		len(spear), meanOf(spear), spear[len(spear)/2], pos, len(spear))
	fmt.Printf("mean fraction of weeks where the gap grew: %.3f (0.5 is a coin flip)\n",
		meanOf(incUp))
	fmt.Printf("⚠️ rho near +1 is monotone RISING; near 0 is a walk; the plateau\n")
	fmt.Printf("shows up as rho well below 1 with the LEVEL still high.\n")

	fmt.Printf("\n=== COST OF MOVING AGAINST COST OF WAITING\n")
	fmt.Printf("A revised squad is HELD FORWARD beside the frozen one and both are\n")
	fmt.Printf("scored weekly, so `gain` is what k transfers actually banked over the\n")
	fmt.Printf("hold — not what they closed on the day. `hits` is 4 a change beyond\n")
	fmt.Printf("one free. **Payback is the week the gain first covers the hits.**\n\n")
	fmt.Printf("%8s %6s", "changes", "hits")
	for w := 1; w <= driftHoldWeeks; w++ {
		fmt.Printf(" %6s", fmt.Sprintf("+%dwk", w))
	}
	fmt.Printf("  %8s\n", "payback")
	for k := 1; k <= driftLadderMax; k++ {
		byWeek := map[int][]float64{}
		for _, hp := range hold[k] {
			byWeek[hp.week] = append(byWeek[hp.week], hp.gain)
		}
		if len(byWeek[1]) < 3 {
			continue
		}
		cost := 4 * float64(k-1)
		fmt.Printf("%8d %6.0f", k, cost)
		payback := 0
		for w := 1; w <= driftHoldWeeks; w++ {
			g := meanOf(byWeek[w])
			fmt.Printf(" %6.1f", g)
			if payback == 0 && g >= cost {
				payback = w
			}
		}
		if payback == 0 {
			fmt.Printf("  %8s\n", "never")
		} else {
			fmt.Printf("  %8d\n", payback)
		}
	}
	// ⚠️ **Against k=1, not against zero.** A manager who waits still gets his
	// free transfer, so "do nothing ever" is not the alternative to acting and a
	// ladder read against it overstates every row. The k=1 row IS the
	// wait-and-spend-the-free-transfer arm, so the decision-relevant quantity is
	// each row's excess over it against 4 points a change beyond the first.
	base := map[int]float64{}
	for _, hp := range hold[1] {
		if hp.week == driftHoldWeeks {
			base[hp.week] = base[hp.week] + hp.gain
		}
	}
	var n1 int
	for _, hp := range hold[1] {
		if hp.week == driftHoldWeeks {
			n1++
		}
	}
	if n1 > 0 {
		b := base[driftHoldWeeks] / float64(n1)
		fmt.Printf("\n=== THE SAME LEDGER AGAINST SPENDING YOUR FREE TRANSFER\n")
		fmt.Printf("k=1 is what waiting actually looks like — one free change a week is\n")
		fmt.Printf("not 'doing nothing'. Reading the rows above against ZERO overstates\n")
		fmt.Printf("every one of them; this is the honest comparison.\n\n")
		fmt.Printf("%8s %8s %10s %8s %10s %s\n",
			"changes", "hits", "over k=1", "cost", "marginal", "verdict")
		var prev float64
		for k := 2; k <= driftLadderMax; k++ {
			var sum float64
			var n int
			for _, hp := range hold[k] {
				if hp.week == driftHoldWeeks {
					sum += hp.gain
					n++
				}
			}
			if n == 0 {
				continue
			}
			over := sum/float64(n) - b
			cost := 4 * float64(k-1)
			verdict := "worth it"
			if over < cost {
				verdict = "NOT worth it"
			}
			fmt.Printf("%8d %8.0f %10.1f %8.0f %10.1f %s\n",
				k, cost, over, cost, over-prev, verdict)
			prev = over
		}
		fmt.Printf("\n⚠️ Both arms are held UNCHANGED through the window after the first\n")
		fmt.Printf("decision, so neither keeps spending its allowance. That biases both\n")
		fmt.Printf("sides the same way and the DIFFERENCE far less than the levels — but\n")
		fmt.Printf("it is not zero, and a squad that moved more may need fewer moves later.\n")
	}

	fmt.Printf("\n⚠️ `never` inside %d weeks is NOT proof a move is wrong — it is the\n", driftHoldWeeks)
	fmt.Printf("statement that this window could not see the payback. A squad held to\n")
	fmt.Printf("GW38 has a longer runway than any sampled week here does.\n")

	fmt.Printf("\n=== THE SAME LEDGER, BY HOW FAR THE SQUAD ALREADY IS FROM IDEAL\n")
	fmt.Printf("The decision form: a manager can see his gap and is choosing between\n")
	fmt.Printf("k transfers and spending the chip. Cells are the gain over %d weeks\n", driftHoldWeeks)
	fmt.Printf("held; subtract 4 a change beyond one free to get the net.\n\n")
	fmt.Printf("%12s %5s %7s %7s %7s %7s %7s %7s\n",
		"gap bucket", "n", "k=1", "k=2", "k=3", "k=4", "k=5", "k=6")
	var buckets []int
	for b := range byGap {
		buckets = append(buckets, b)
	}
	sort.Ints(buckets)
	for _, b := range buckets {
		row := byGap[b]
		if len(row[1]) < 3 {
			continue
		}
		fmt.Printf("%12s %5d", gapBucketName(b), len(row[1]))
		for k := 1; k <= driftLadderMax; k++ {
			fmt.Printf(" %7.2f", meanOf(row[k]))
		}
		fmt.Printf("\n")
	}
	fmt.Printf("\n=== THE SAME LEDGER, BY WHEN THE DECISION WAS TAKEN\n")
	fmt.Printf("Is a hit at GW2 the same buy as a hit at GW14? An early move chases an\n")
	fmt.Printf("ideal built from almost no data, so the target may evaporate — the same\n")
	fmt.Printf("mechanism as rebuilding before the template exists, at a smaller scale\n")
	fmt.Printf("and forty times a season. Cells are gain over %d weeks held.\n\n", driftHoldWeeks)
	fmt.Printf("%12s %5s %7s %7s %7s %7s %7s %7s\n",
		"decided", "n", "k=1", "k=2", "k=3", "k=4", "k=5", "k=6")
	var wbs []int
	for b := range byWeek {
		wbs = append(wbs, b)
	}
	sort.Ints(wbs)
	for _, b := range wbs {
		row := byWeek[b]
		if len(row[1]) < 3 {
			continue
		}
		fmt.Printf("%12s %5d", weekBandName(b), len(row[1]))
		for k := 1; k <= driftLadderMax; k++ {
			fmt.Printf(" %7.2f", meanOf(row[k]))
		}
		fmt.Printf("\n")
	}
	fmt.Printf("\n⚠️ **Two confounds cut in OPPOSITE directions here and neither is\n")
	fmt.Printf("removed.** The eligible pool differs by week — `MinMinutes` is scaled to\n")
	fmt.Printf("matches played — so an early optimiser chooses from a thinner, noisier\n")
	fmt.Printf("set, which would depress early gains for a reason that is not about\n")
	fmt.Printf("information. Against that, an early buy locks in a price before the\n")
	fmt.Printf("season's rises, which flatters it. A flat row is therefore NOT evidence\n")
	fmt.Printf("of no effect; it is consistent with two effects cancelling.\n")

	fmt.Printf("\n⚠️ **A wildcard is the k=15 row with the hits set to zero**, and it is\n")
	fmt.Printf("not in this table. Read the ceiling instead: the FROZEN column of the\n")
	fmt.Printf("trajectory is the whole gap, so a rebuild banks at most that a week.\n")
	fmt.Printf("The chip is worth spending when no affordable k gets close to it.\n")
}

// reviseSquad is repairSquad bounded by how many of the held fifteen may change.
//
// MaxChanges counts the DIFFERENCE from CurrentSquad rather than the moves taken,
// so the search cannot spend the budget selling a player and buying him back —
// which is what makes the ladder an honest "best k transfers" rather than an
// upper bound on one.
func reviseSquad(e *analysis.Engine, held []int, budget int, minExp float64,
	cfg SimConfig, maxChanges int) ([]int, bool) {

	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: budget, MinMinutes: 600, MinExpectedMinutes: minExp,
		BenchWeight:  cfg.openingBenchWeight(),
		CurrentSquad: held, MaxChanges: maxChanges,
	})
	if err != nil {
		return nil, false
	}
	out := make([]int, 0, 15)
	for _, p := range sq.Players {
		out = append(out, p.ID)
	}
	return out, true
}

// settle moves the wallet from one squad to another at market, so the no-hit
// arm's budget carries FPL's half-of-any-rise selling rule like a real one.
func settle(cur *Season, w *wallet, from, to []int, gw int) {
	keep := map[int]bool{}
	for _, id := range to {
		keep[id] = true
	}
	had := map[int]bool{}
	for _, id := range from {
		had[id] = true
		if !keep[id] {
			w.sell(id, marketPrice(cur, id, gw))
		}
	}
	for _, id := range to {
		if !had[id] {
			w.buy(id, marketPrice(cur, id, gw))
		}
	}
}

// spearmanVsIndex is the rank correlation of a series against its own position —
// the standard non-parametric test for monotone trend, and the right one here
// because the conjecture is about ORDER rather than about a linear rate.
func spearmanVsIndex(v []float64) float64 {
	idx := make([]float64, len(v))
	for i := range v {
		idx[i] = float64(i)
	}
	return corrOf(rankOf(v), idx)
}

func rankOf(v []float64) []float64 {
	type pair struct {
		x float64
		i int
	}
	ps := make([]pair, len(v))
	for i, x := range v {
		ps[i] = pair{x, i}
	}
	sort.Slice(ps, func(a, b int) bool { return ps[a].x < ps[b].x })
	out := make([]float64, len(v))
	for r, p := range ps {
		out[p.i] = float64(r)
	}
	return out
}

// risingFraction is how often the next reading is at least the current one. A
// monotone series reads 1.0 and a random walk reads about 0.5, so it separates
// "grows" from "wanders upward on average" — which a mean trajectory cannot.
func risingFraction(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	var up int
	for i := 1; i < len(v); i++ {
		if v[i] >= v[i-1] {
			up++
		}
	}
	return float64(up) / float64(len(v)-1)
}

// gapBucket puts a gap reading into a band a manager could recognise. The bands
// are wide because the ladder is read within them and a narrow band is a small
// sample, not a precise answer.
func gapBucket(d float64) int {
	switch {
	case d < 5:
		return 0
	case d < 10:
		return 1
	case d < 15:
		return 2
	case d < 20:
		return 3
	}
	return 4
}

func gapBucketName(b int) string {
	return [...]string{"under 5", "5-10", "10-15", "15-20", "20+"}[b]
}

// printResetWorth puts the two curves this whole line of work is about on one
// axis: how much a rebuild is WORTH at each week, measured, against the option
// decay the engine already charges.
//
// # The arithmetic
//
// Resetting at week t means carrying D(0..38−t) instead of D(t..38), so its worth
// is the difference of those two sums. ⚠️ **That is flat, not peaked, over a whole
// interval.** For any t where the squad has already plateaued (t ≥ k) and the
// runway still exceeds the ramp (38−t ≥ k), the difference collapses to L·k/2 — a
// constant. So the answer is a WINDOW [k, 38−k], and the plateau week k sets its
// width: k=10 gives GW10-28, k=16 gives GW16-22.
//
// ⚠️ **This is the peak `wildcardValueOverNext` could not have.** That function
// summed the remaining drift of a FIXED stale squad, so both its terms fell with k
// and its argmax was always now. Here the second term is the re-drift of a squad
// that was actually rebuilt, which is what makes an interior optimum possible at
// all.
//
// # What the comparison can and cannot say
//
// `OptionDecay` is `remaining/(remaining+halfLife)`, mean-normalised — a charge
// multiplier fitted to a different reading, and `optionvalue.go` says so in as
// many words: the terms enter "through the CHARGE, which is a reluctance to spend
// rather than a term in any forward valuation". This is that missing forward
// valuation, so a disagreement between the two shapes is expected and is not by
// itself evidence the shipped curve is wrong for its own job.
func printResetWorth(cohort map[int]map[int][]float64) {
	// The longest cohort is the only one that sees a full season, and mixing
	// cohorts is the composition problem this section exists to avoid.
	best, bestLen := 0, 0
	for e, byWeek := range cohort {
		if len(byWeek) > bestLen {
			best, bestLen = e, len(byWeek)
		}
	}
	d := make([]float64, 0, bestLen)
	for s := 0; s < bestLen; s++ {
		v := cohort[best][s]
		if len(v) == 0 {
			break
		}
		d = append(d, meanOf(v))
	}
	if len(d) < 12 {
		return
	}
	at := func(i int) float64 {
		if i >= len(d) {
			return d[len(d)-1] // hold the last reading rather than invent one
		}
		return d[i]
	}
	n := len(d) - 1
	worth := make([]float64, n+1)
	for t := 0; t <= n; t++ {
		var carry, fresh float64
		for s := t; s <= n; s++ {
			carry += at(s)
		}
		for u := 0; u <= n-t; u++ {
			fresh += at(u)
		}
		worth[t] = carry - fresh
	}
	var peak float64
	var peakAt int
	for t, w := range worth {
		if w > peak {
			peak, peakAt = w, t
		}
	}
	fmt.Printf("\n=== WORTH OF A REBUILD, MEASURED, AGAINST THE SHIPPED DECAY\n")
	fmt.Printf("Cohort: entry GW%d, %d weeks. `worth` is the drift a reset avoids over\n", best, len(d))
	fmt.Printf("the rest of the season. `decay` is OptionDecay(remaining, %.0f), the\n", analysis.DefaultOptionHalfLife)
	fmt.Printf("charge the engine already applies. Both shown as a share of their own max,\n")
	fmt.Printf("because one is in points and the other is a multiplier.\n\n")
	fmt.Printf("%6s %9s %8s %8s\n", "week", "worth", "share", "decay")
	w38 := analysis.OptionWindow{Expiry: 38}
	for t := 0; t <= n; t += 2 {
		gw := best + t
		if gw > 38 {
			break
		}
		dec := analysis.OptionDecay(float64(38-gw), analysis.DefaultOptionHalfLife)
		fmt.Printf("%6d %9.1f %8.2f %8.2f\n", gw, worth[t], worth[t]/peak, dec/analysis.OptionDecay(float64(38-best), analysis.DefaultOptionHalfLife))
	}
	_ = w38
	fmt.Printf("\npeak worth %.1f at GW%d\n", peak, best+peakAt)
	fmt.Printf("⚠️ Read the WINDOW, not the peak. The worth is flat wherever the squad\n")
	fmt.Printf("has plateaued and the runway still exceeds the ramp, so an argmax over a\n")
	fmt.Printf("flat region is noise picking a week. Report where it is within a few\n")
	fmt.Printf("points of its maximum, which is the actual answer.\n")
	var lo, hi int = -1, -1
	for t := 0; t <= n; t++ {
		if worth[t] >= 0.9*peak {
			if lo < 0 {
				lo = t
			}
			hi = t
		}
	}
	if lo >= 0 {
		fmt.Printf("within 10%% of peak: GW%d to GW%d\n", best+lo, best+hi)
	}
}

// printFirstHalfChoice answers the only wildcard question drift can settle: when
// to play the FIRST-half chip, and whether the week can be detected.
//
// # Why the window ends at ChipResetGW and not at GW38
//
// **There are two wildcards, one per half.** The second is a CALENDAR decision —
// it targets doubles and blanks — so it is not a drift question and is measured
// elsewhere. What that means for this one is structural rather than cosmetic:
// **the first chip's runway ends when the second arrives**, because the second
// resets the drift anyway. Scoring the first chip's benefit to GW38 credits it
// with weeks the second chip was always going to fix, which inflates it and
// pushes its optimum far too late. An earlier version of this file did exactly
// that.
//
// # The two worlds, and which one we are in
//
// Carrying drift across the window with one reset at week t costs
// `Σ(0..t-1) D + Σ(0..W-t) D`, minimised when the two segments are equal — the
// MIDPOINT, if D is rising and has not plateaued inside the window.
//
//   - If every squad's optimum is near that midpoint, **no detection is needed
//     and the rule is a calendar rule**: play it around the middle of the first
//     half. That would also explain why every state trigger in this record reads
//     null — they are all trying to detect a week that does not move.
//   - If optima spread out and track something observable early, a **detector
//     exists** and is worth building.
//
// ⚠️ **REGRET decides whether either matters.** If playing at a fixed week costs
// almost nothing against a cell's own oracle, the detection question is moot
// however it comes out. That is reported first for exactly that reason.
func printFirstHalfChoice(runs []driftRun) {
	// Only entries with a real first-half window to decide in. An entry at GW16
	// has three weeks before the second chip arrives and is not making this
	// decision at all.
	const minWindow = 8
	type row struct {
		season           string
		start, oracle, w int
		best, atMid, at5 float64
		early            float64
	}
	var rows []row
	for _, r := range runs {
		w := ChipResetGW - r.start // weeks of window before the second chip
		if w < minWindow || len(r.d) <= w {
			continue
		}
		cost := func(t int) float64 {
			var c float64
			for s := 0; s < t; s++ {
				c += r.d[s]
			}
			for u := 0; u <= w-t; u++ {
				c += r.d[u]
			}
			return c
		}
		best, at := cost(0), 0
		for t := 1; t <= w; t++ {
			if c := cost(t); c < best {
				best, at = c, t
			}
		}
		// The observable a detector would have: the gap already carried in the
		// first few weeks, which is all a live rule can see at decision time.
		var early float64
		for s := 1; s <= 4 && s < len(r.d); s++ {
			early += r.d[s]
		}
		rows = append(rows, row{r.season, r.start, at, w, best, cost(w / 2), cost(5), early / 4})
	}
	if len(rows) < 4 {
		return
	}

	fmt.Printf("\n=== WHEN TO PLAY THE FIRST-HALF WILDCARD, AND CAN IT BE DETECTED\n")
	fmt.Printf("Window is entry to GW%d, where the SECOND chip takes over — the first\n", ChipResetGW)
	fmt.Printf("chip's runway ends there because the second resets the drift anyway.\n")
	fmt.Printf("`oracle` is the best week for THAT cell, with hindsight.\n\n")
	fmt.Printf("%-9s %6s %5s %8s %10s %10s %10s\n",
		"season", "entry", "wks", "oracle", "carried", "reg@mid", "reg@+5")
	var oracles, regMid, reg5, earlies []float64
	for _, r := range rows {
		fmt.Printf("%-9s %6d %5d %8d %10.1f %10.1f %10.1f\n",
			r.season, r.start, r.w, r.start+r.oracle, r.best,
			r.atMid-r.best, r.at5-r.best)
		oracles = append(oracles, float64(r.oracle))
		regMid = append(regMid, r.atMid-r.best)
		reg5 = append(reg5, r.at5-r.best)
		earlies = append(earlies, r.early)
	}

	fmt.Printf("\nREGRET FIRST, because it decides whether the rest matters.\n")
	fmt.Printf("Playing at the window's MIDPOINT costs %.1f points against a cell's own\n", meanOf(regMid))
	fmt.Printf("oracle; playing at entry+5 costs %.1f. Carried drift itself averages\n", meanOf(reg5))
	fmt.Printf("%.1f, so the midpoint rule gives up %.1f%% of what the decision is worth.\n",
		meanOf(collect(rows, func(r row) float64 { return r.best })),
		100*meanOf(regMid)/meanOf(collect(rows, func(r row) float64 { return r.best })))
	fmt.Printf("⚠️ If that share is small, NO detector can be worth much — the ceiling\n")
	fmt.Printf("on any state rule is the regret a fixed calendar rule already pays.\n")

	fmt.Printf("\nDO THE OPTIMA MOVE? oracle week spread: min %.0f median %.0f max %.0f\n",
		minOf(oracles), medianOf(oracles), maxOf(oracles))
	fmt.Printf("correlation between EARLY drift (weeks 1-4, all a live rule can see)\n")
	fmt.Printf("and the oracle week: %+.3f over %d cells\n", corrOf(earlies, oracles), len(rows))
	fmt.Printf("⚠️ A NEGATIVE correlation is the detectable case — a squad drifting fast\n")
	fmt.Printf("early should reset sooner. Near zero means the oracle week is not\n")
	fmt.Printf("predictable from what a manager can see, whatever its spread.\n")
}

// driftRun is one cell's gap trajectory, kept per cell rather than pooled because
// the detection question is whether optima MOVE between squads.
type driftRun struct {
	season string
	start  int
	d      []float64
}

func collect[T any](in []T, f func(T) float64) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

func minOf(v []float64) float64 {
	m := v[0]
	for _, x := range v {
		if x < m {
			m = x
		}
	}
	return m
}

func medianOf(v []float64) float64 {
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	return c[len(c)/2]
}

// weekBand groups a decision by how much season had been played when it was
// taken. The bands are coarse because the ledger is read inside them.
func weekBand(gw int) int {
	switch {
	case gw <= 5:
		return 0
	case gw <= 10:
		return 1
	case gw <= 15:
		return 2
	case gw <= 25:
		return 3
	}
	return 4
}

func weekBandName(b int) string {
	return [...]string{"GW1-5", "GW6-10", "GW11-15", "GW16-25", "GW26+"}[b]
}
