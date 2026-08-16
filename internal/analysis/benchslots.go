package analysis

import "os"

// What a bench slot is worth is a property of the eleven in front of it.
//
// The shipped slot weights are a fixed tuple — 2.4 / 1.0 / 0.4 / 0.2 — standing
// in for "how often is this slot reached". That ordering is right and the
// numbers were chosen by sweeping, which is to say by asserting a shape and
// checking it did not lose. It cannot express the thing that actually varies: a
// bench behind eleven ever-presents is worth much less than the same bench
// behind a fragile side, and the tuple gives both the same credit.
//
// FPL's autosub rule says exactly what a slot is worth. A bench player is used
// when a starter records *no minutes at all*, taken in bench order, so:
//
//   - the reserve keeper is worth P(the starting keeper blanks) — he covers one
//     player and no others;
//   - the first outfield substitute is worth P(at least one outfield starter
//     blanks);
//   - the second, P(at least two); the third, P(at least three).
//
// Blanks are close enough to independent across a squad — the correlated case,
// a whole club's fixture being postponed, is a blank gameweek the model handles
// elsewhere — so the count of blanking starters is Poisson-binomial and those
// four probabilities are exactly computable by a ten-step convolution.
//
// # The blank rate is P(no minutes), and it has one estimator
//
// A bench player is reached only when a starter records no minutes at all, so
// what prices these slots is exactly one minus the probability that the starter
// in front of him appears. That quantity lives in appearance.go — see blankRate
// there, and the note at the top of that file for why it used to be computed
// twice, from two different statistics, with the version used here fitted only
// over start share 0.70 and up.
//
// Nothing in this file changed when the two were unified except which estimator
// blankRate consults: an eleven is still priced by its members' blank
// probabilities, and the Poisson-binomial convolution below is untouched.

// referenceBlankRate is the blank rate of a typical eleven's member, used to
// keep BenchWeight's scale meaning what it was calibrated to mean.
//
// The measured rate is 0.066 for the 0.85-0.95 start-share band and 0.122 for
// 0.75-0.85; an optimised eleven sits between them and nearer the top.
const referenceBlankRate = 0.09

// slotProbabilities returns P(slot is reached) for the reserve keeper and the
// three outfield bench slots, given the eleven they sit behind.
//
// Cold entry point. The search reaches the same code through its own scratch,
// because this convolution ran inside the objective and allocated a fresh,
// growing slice per outfield player — ten allocations per evaluation, for a
// ten-step recurrence over at most eleven floats.
func slotProbabilities(xi []PlayerMetrics) (gk float64, outfield [3]float64) {
	var sc xiScratch
	return sc.slotProbabilities(xi)
}

func (sc *xiScratch) slotProbabilities(xi []PlayerMetrics) (gk float64, outfield [3]float64) {
	// P(exactly k of the outfield ten blank), by convolution, over two buffers
	// swapped each step. next is zeroed explicitly because the version this
	// replaces got its zeroes from a fresh make, and the two additions into each
	// cell have to land in the same order to give the same float: cell k receives
	// dist[k-1]*b from the previous step and dist[k]*(1-b) from this one.
	dist, next := sc.dist[:1], sc.next[:]
	dist[0] = 1
	if len(xi) >= len(sc.dist) {
		// More players than the fixed buffers hold. Cannot happen for an eleven,
		// so this is a bound rather than an assumption: fall back to the heap
		// instead of truncating a distribution.
		d := make([]float64, 1, len(xi)+1)
		d[0] = 1
		dist, next = d, make([]float64, len(xi)+1)
	}
	for _, p := range xi {
		if p.Position == "GKP" {
			gk = blankRate(p)
			continue
		}
		b := blankRate(p)
		n := next[:len(dist)+1]
		for i := range n {
			n[i] = 0
		}
		for k, v := range dist {
			n[k] += v * (1 - b)
			n[k+1] += v * b
		}
		dist, next = n, dist[:cap(dist)]
	}
	// P(at least i) for i = 1, 2, 3.
	tail := 1.0
	for i := 0; i < 3; i++ {
		if i < len(dist) {
			tail -= dist[i]
		}
		outfield[i] = clamp(tail, 0, 1)
	}
	return gk, outfield
}

// benchSlotScale converts slot probabilities into weights on BenchWeight's
// scale, where the four slots of a typical eleven sum to four.
//
// Without it the change would be two changes at once: a new shape *and* a
// roughly sixfold cut in what the bench is worth overall, since the raw
// probabilities sum to about 0.7 rather than 4. BenchWeight owns the scale and
// was swept on that basis, so this keeps the comparison honest — the same
// reason FPL_BENCH_SLOTS renormalises.
//
// The reference is fixed rather than per-squad, which is the entire point. A
// per-squad normalisation would rescale every squad's bench to the same total
// and throw away exactly the signal this exists to capture: that depth behind a
// fragile eleven is worth more than depth behind a sound one.
//
// The reference player is specified by the blank rate he must have rather than by
// a start share, because only one of the two estimators reads a start share. That
// keeps this constant numerically identical across the unification — see
// metricsWithBlankRate.
var benchSlotScale = func() float64 {
	ref := make([]PlayerMetrics, 11)
	for i := range ref {
		ref[i] = metricsWithBlankRate(referenceBlankRate)
	}
	ref[0].Position = "GKP"
	gk, out := slotProbabilities(ref)
	total := gk + out[0] + out[1] + out[2]
	if total <= 0 {
		return 1
	}
	return 4 / total
}()

// derivedBenchSlots reports whether bench slots are priced from the eleven's
// own blank probabilities rather than the fixed tuple. Set FPL_FIXED_BENCH_SLOTS=1
// to restore the tuple and re-measure.
var derivedBenchSlots = os.Getenv("FPL_FIXED_BENCH_SLOTS") == "" &&
	os.Getenv("FPL_BENCH_SLOTS") == ""

// benchSlotWeightsFor prices each bench slot against the eleven in front of it.
func benchSlotWeightsFor(xi []PlayerMetrics) (outfield [3]float64, gk float64) {
	var sc xiScratch
	return sc.benchSlotWeightsFor(xi)
}

func (sc *xiScratch) benchSlotWeightsFor(xi []PlayerMetrics) (outfield [3]float64, gk float64) {
	g, out := sc.slotProbabilities(xi)
	for i := range out {
		outfield[i] = out[i] * benchSlotScale
	}
	return outfield, g * benchSlotScale
}
