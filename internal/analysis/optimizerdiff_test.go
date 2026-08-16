package analysis

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"
)

// The differential test for the optimiser's hot path.
//
// # Why this exists rather than a reviewer reading the diff
//
// The three changes below it are performance work on the objective function:
// buffer reuse, a non-reflective sort, and a compact per-player record in place
// of a 592-byte struct. None of them is supposed to change a single number. This
// codebase's recorded failure mode is exactly that — a faster or "cleaner" path
// that quietly returns a different squad while every existing test passes,
// because the existing tests assert invariants (the squad is legal, the XI has
// eleven players) and an optimiser that picks a *different* legal squad passes
// all of them.
//
// So the reference implementations are frozen here, verbatim, as an oracle, and
// the assertion is bit equality via math.Float64bits rather than a tolerance. A
// tolerance would be wrong twice over: the objective feeds an argmax, so a
// difference of one ULP flips a player and changes a season, and a tolerance
// cannot distinguish "the same computation" from "a nearly-identical one".
//
// # The two traps this has to cover, both of which are real
//
//  1. **Stable-sort tie order depends on input order.** Ties on Score are
//     common — bestXIWith sorts each position by score, and slotProbabilities
//     then convolves blank rates *in the resulting order*. Two players tied on
//     Score with different ExpectedMinutes have different blank rates, so which
//     of them the stable sort keeps first changes the objective. Input order is
//     therefore load-bearing, and any change that normalises, id-sorts or
//     re-sorts the input silently changes the answer.
//     TestStableTieOrderFollowsInputOrder pins that it is load-bearing.
//
//  2. **replace puts the incoming player at the outgoing player's index**, so a
//     trial squad is NOT id-ordered, and the singles loop evaluates thousands of
//     them. A fast path that assumed sorted or id-ordered input would be correct
//     on every hand-written fixture and wrong on every squad the search actually
//     scores. The generator below therefore builds cases through replace, not
//     just by shuffling.
//
// TestOptimizerDiffHarnessHasTeeth runs a deliberately-broken implementation
// through the same comparison and fails if it is accepted, so the harness cannot
// rot into one that would pass anything.

// ---------------------------------------------------------------------------
// Frozen reference implementations.
//
// Copied verbatim from squad.go and benchslots.go at the commit before this
// work. Self-contained on purpose: they call each other rather than the shipped
// functions, so they stay a fixed oracle however the shipped ones are rewritten.
// Do not "tidy" them and do not make them share code with the implementation.
// ---------------------------------------------------------------------------

func refXIValue(pick []PlayerMetrics) float64 {
	var sum, captain, vice float64
	for _, p := range pick {
		sum += p.Score
		switch {
		case p.Score > captain:
			captain, vice = p.Score, captain
		case p.Score > vice:
			vice = p.Score
		}
	}
	return sum + captain + ViceCaptainWeight*vice
}

func refSlotProbabilities(xi []PlayerMetrics) (gk float64, outfield [3]float64) {
	dist := []float64{1}
	for _, p := range xi {
		if p.Position == "GKP" {
			gk = blankRate(p)
			continue
		}
		b := blankRate(p)
		next := make([]float64, len(dist)+1)
		for k, v := range dist {
			next[k] += v * (1 - b)
			next[k+1] += v * b
		}
		dist = next
	}
	tail := 1.0
	for i := 0; i < 3; i++ {
		if i < len(dist) {
			tail -= dist[i]
		}
		outfield[i] = clamp(tail, 0, 1)
	}
	return gk, outfield
}

func refBenchSlotWeightsFor(xi []PlayerMetrics) (outfield [3]float64, gk float64) {
	g, out := refSlotProbabilities(xi)
	for i := range out {
		outfield[i] = out[i] * benchSlotScale
	}
	return outfield, g * benchSlotScale
}

func refBenchValue(xi, bench []PlayerMetrics, benchWeight float64) float64 {
	if flatBenchWeight {
		var s float64
		for _, p := range bench {
			s += p.Score * benchWeight
		}
		return s
	}

	slots, gkSlot := benchOutfieldWeights, benchGKWeight
	if derivedBenchSlots {
		slots, gkSlot = refBenchSlotWeightsFor(xi)
	}

	outfield := make([]PlayerMetrics, 0, len(bench))
	var s float64
	for _, p := range bench {
		if p.Position == "GKP" {
			s += p.Score * benchWeight * gkSlot
			continue
		}
		outfield = append(outfield, p)
	}
	sort.SliceStable(outfield, func(i, j int) bool {
		return outfield[i].Score > outfield[j].Score
	})
	for i, p := range outfield {
		w := slots[len(slots)-1]
		if i < len(slots) {
			w = slots[i]
		}
		s += p.Score * benchWeight * w
	}
	return s
}

func refBestXIWith(squad []PlayerMetrics, mustStart map[int]bool) (xi, bench []PlayerMetrics, formation string) {
	byPos := map[string][]PlayerMetrics{}
	forced := map[string]int{}
	for _, p := range squad {
		byPos[p.Position] = append(byPos[p.Position], p)
		if mustStart[p.ID] {
			forced[p.Position]++
		}
	}
	for pos := range byPos {
		sort.SliceStable(byPos[pos], func(i, j int) bool {
			a, b := byPos[pos][i], byPos[pos][j]
			if mustStart[a.ID] != mustStart[b.ID] {
				return mustStart[a.ID]
			}
			return a.Score > b.Score
		})
	}

	bestTotal := -1.0
	var bestPick []PlayerMetrics
	var bestForm string

	for d := xiMin["DEF"]; d <= xiMax["DEF"]; d++ {
		for m := xiMin["MID"]; m <= xiMax["MID"]; m++ {
			for f := xiMin["FWD"]; f <= xiMax["FWD"]; f++ {
				if 1+d+m+f != 11 {
					continue
				}
				if len(byPos["GKP"]) < 1 || len(byPos["DEF"]) < d ||
					len(byPos["MID"]) < m || len(byPos["FWD"]) < f {
					continue
				}
				if forced["GKP"] > 1 || forced["DEF"] > d ||
					forced["MID"] > m || forced["FWD"] > f {
					continue
				}
				pick := append([]PlayerMetrics{}, byPos["GKP"][:1]...)
				pick = append(pick, byPos["DEF"][:d]...)
				pick = append(pick, byPos["MID"][:m]...)
				pick = append(pick, byPos["FWD"][:f]...)

				total := refXIValue(pick)
				if total > bestTotal {
					bestTotal = total
					bestPick = pick
					bestForm = fmt.Sprintf("%d-%d-%d", d, m, f)
				}
			}
		}
	}

	inXI := map[int]bool{}
	for _, p := range bestPick {
		inXI[p.ID] = true
	}
	for _, p := range squad {
		if !inXI[p.ID] {
			bench = append(bench, p)
		}
	}
	sort.SliceStable(bench, func(i, j int) bool {
		if (bench[i].Position == "GKP") != (bench[j].Position == "GKP") {
			return bench[j].Position == "GKP"
		}
		return bench[i].Score > bench[j].Score
	})

	sort.SliceStable(bestPick, func(i, j int) bool { return bestPick[i].Score > bestPick[j].Score })
	return bestPick, bench, bestForm
}

func refObjectiveWith(squad []PlayerMetrics, benchWeight float64, mustStart map[int]bool) float64 {
	xi, bench, _ := refBestXIWith(squad, mustStart)
	return refXIValue(xi) + refBenchValue(xi, bench, benchWeight)
}

// ---------------------------------------------------------------------------
// Case generation
// ---------------------------------------------------------------------------

type diffCase struct {
	label      string
	squad      []PlayerMetrics
	mustStart  map[int]bool
	benchWeigh float64
}

// diffPlayer builds a candidate with ties made likely on purpose. Score sits on
// a coarse grid so two players collide often, while ExpectedMinutes is drawn
// independently — which is what makes a tie observable in the objective, since
// blankRate reads the minutes and slotProbabilities convolves in sorted order.
func diffPlayer(rng *rand.Rand, id int, pos string, scoreGrid int) PlayerMetrics {
	return PlayerMetrics{
		ID:              id,
		Name:            fmt.Sprintf("p%d", id),
		Position:        pos,
		Team:            fmt.Sprintf("T%02d", rng.Intn(20)),
		Price:           float64(39+rng.Intn(112)) / 10,
		Score:           float64(rng.Intn(scoreGrid)) * 0.25,
		ExpectedMinutes: float64(rng.Intn(91)),
		StartShare:      float64(rng.Intn(101)) / 100,
		Status:          "available",
	}
}

var diffQuota = []struct {
	pos string
	n   int
}{{"GKP", 2}, {"DEF", 5}, {"MID", 5}, {"FWD", 3}}

func diffSquad(rng *rand.Rand, scoreGrid int) []PlayerMetrics {
	out := make([]PlayerMetrics, 0, SquadSize)
	id := 0
	for _, q := range diffQuota {
		for i := 0; i < q.n; i++ {
			id++
			out = append(out, diffPlayer(rng, id, q.pos, scoreGrid))
		}
	}
	return out
}

// optimizerDiffCases covers the shapes the search actually produces.
func optimizerDiffCases(t *testing.T) []diffCase {
	t.Helper()
	rng := rand.New(rand.NewSource(4634))
	var cases []diffCase

	weights := []float64{0.02, DefaultBenchWeight, 0.15, 0.35}

	// Score grids from "everybody ties" to "ties are rare".
	for _, grid := range []int{1, 2, 3, 8, 40, 400} {
		for rep := 0; rep < 60; rep++ {
			squad := diffSquad(rng, grid)
			bw := weights[rng.Intn(len(weights))]

			cases = append(cases, diffCase{
				label:      fmt.Sprintf("grid%d/rep%d/plain", grid, rep),
				squad:      squad,
				benchWeigh: bw,
			})

			// Shuffled: same multiset, different input order. Under ties this is
			// a genuinely different problem, and both implementations must agree
			// on each ordering separately.
			shuf := append([]PlayerMetrics(nil), squad...)
			rng.Shuffle(len(shuf), func(i, j int) { shuf[i], shuf[j] = shuf[j], shuf[i] })
			cases = append(cases, diffCase{
				label:      fmt.Sprintf("grid%d/rep%d/shuffled", grid, rep),
				squad:      shuf,
				benchWeigh: bw,
			})

			// Trap 2: a trial squad exactly as the singles loop builds it — the
			// incoming player sits at the *outgoing* player's index, so the slice
			// is neither id-ordered nor position-grouped.
			victim := squad[rng.Intn(len(squad))]
			incoming := diffPlayer(rng, 900+rep, victim.Position, grid)
			trial := replace(squad, victim.ID, incoming)
			cases = append(cases, diffCase{
				label:      fmt.Sprintf("grid%d/rep%d/replaced", grid, rep),
				squad:      trial,
				benchWeigh: bw,
			})

			// And a doubly-replaced squad, as runPairs builds it.
			v2 := trial[rng.Intn(len(trial))]
			in2 := diffPlayer(rng, 1900+rep, v2.Position, grid)
			cases = append(cases, diffCase{
				label:      fmt.Sprintf("grid%d/rep%d/replaced2", grid, rep),
				squad:      replace(trial, v2.ID, in2),
				benchWeigh: bw,
			})

			// Forced starters, including counts that rule formations out.
			for _, n := range []int{1, 2, 4} {
				ms := map[int]bool{}
				perm := rng.Perm(len(squad))
				for i := 0; i < n && i < len(perm); i++ {
					ms[squad[perm[i]].ID] = true
				}
				cases = append(cases, diffCase{
					label:      fmt.Sprintf("grid%d/rep%d/forced%d", grid, rep, n),
					squad:      squad,
					mustStart:  ms,
					benchWeigh: bw,
				})
			}

			// An empty non-nil map must behave exactly like nil.
			cases = append(cases, diffCase{
				label:      fmt.Sprintf("grid%d/rep%d/emptyforced", grid, rep),
				squad:      squad,
				mustStart:  map[int]bool{},
				benchWeigh: bw,
			})
		}
	}

	// Every player identical: the maximal-tie case, where any instability shows.
	for rep := 0; rep < 20; rep++ {
		squad := diffSquad(rng, 1)
		for i := range squad {
			squad[i].Score = 4.0
			squad[i].ExpectedMinutes = 70
			squad[i].Price = 5.0
		}
		// One field left distinct, so a reordering is detectable at all.
		for i := range squad {
			squad[i].Name = fmt.Sprintf("clone%02d", i)
		}
		cases = append(cases, diffCase{
			label:      fmt.Sprintf("clones/rep%d", rep),
			squad:      squad,
			benchWeigh: DefaultBenchWeight,
		})
	}

	// Zero and negative scores: the objective seeds bestTotal at -1.0, so a
	// squad that scores below it exercises a branch nothing else reaches.
	for rep := 0; rep < 20; rep++ {
		squad := diffSquad(rng, 4)
		for i := range squad {
			squad[i].Score = 0
		}
		cases = append(cases, diffCase{
			label:      fmt.Sprintf("zeroscores/rep%d", rep),
			squad:      squad,
			benchWeigh: DefaultBenchWeight,
		})
	}

	return cases
}

// ---------------------------------------------------------------------------
// Comparison
// ---------------------------------------------------------------------------

// bits renders a float's exact representation, so a failure message names the
// ULP rather than printing two identical-looking decimals.
func bits(f float64) string { return fmt.Sprintf("%v (0x%016x)", f, math.Float64bits(f)) }

func sameFloat(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b)
}

// sameSelection compares two XI-or-bench slices for identical membership *and*
// ordering. Ordering is not cosmetic here: the eleven is returned sorted so the
// captain is element zero, and slotProbabilities convolves the bench weights in
// the order given.
func sameSelection(got, want []PlayerMetrics) error {
	if len(got) != len(want) {
		return fmt.Errorf("length %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			return fmt.Errorf("index %d is id %d, want id %d", i, got[i].ID, want[i].ID)
		}
		if !sameFloat(got[i].Score, want[i].Score) {
			return fmt.Errorf("index %d score %s, want %s", i,
				bits(got[i].Score), bits(want[i].Score))
		}
		if got[i].Name != want[i].Name {
			return fmt.Errorf("index %d is %q, want %q", i, got[i].Name, want[i].Name)
		}
	}
	return nil
}

// xiFunc is the shape of bestXIWith, so the comparison can be pointed at a
// deliberately-broken implementation to prove it has teeth.
type xiFunc func(squad []PlayerMetrics, mustStart map[int]bool) (xi, bench []PlayerMetrics, formation string)

// checkAgainstReference is the whole assertion, factored out so that
// TestOptimizerDiffHarnessHasTeeth can run a known-bad function through the
// identical checks. It returns the first disagreement it finds.
func checkAgainstReference(cases []diffCase, impl xiFunc) error {
	for _, c := range cases {
		// The implementation must not mutate its input: the search reuses the
		// squad slice for the next candidate, so an in-place sort would corrupt
		// every subsequent evaluation.
		before := append([]PlayerMetrics(nil), c.squad...)

		wantXI, wantBench, wantForm := refBestXIWith(c.squad, c.mustStart)
		gotXI, gotBench, gotForm := impl(c.squad, c.mustStart)

		if err := sameSelection(c.squad, before); err != nil {
			return fmt.Errorf("%s: implementation mutated its input squad: %w", c.label, err)
		}
		if gotForm != wantForm {
			return fmt.Errorf("%s: formation %q, want %q", c.label, gotForm, wantForm)
		}
		if err := sameSelection(gotXI, wantXI); err != nil {
			return fmt.Errorf("%s: XI: %w", c.label, err)
		}
		if err := sameSelection(gotBench, wantBench); err != nil {
			return fmt.Errorf("%s: bench: %w", c.label, err)
		}
	}
	return nil
}

// TestOptimizerHotPathIsBitExact is the differential test.
func TestOptimizerHotPathIsBitExact(t *testing.T) {
	cases := optimizerDiffCases(t)
	if len(cases) < 2000 {
		t.Fatalf("only %d cases generated; this test is evidence only in bulk", len(cases))
	}
	t.Logf("comparing %d cases", len(cases))

	if err := checkAgainstReference(cases, bestXIWith); err != nil {
		t.Fatalf("bestXIWith diverged from the frozen reference: %v", err)
	}

	// The objective is the number the search actually compares, so it gets its
	// own bit-exact assertion rather than being inferred from the selection.
	for _, c := range cases {
		want := refObjectiveWith(c.squad, c.benchWeigh, c.mustStart)
		got := objectiveWith(c.squad, c.benchWeigh, c.mustStart, false)
		if !sameFloat(got, want) {
			t.Fatalf("%s: objectiveWith = %s, want %s", c.label, bits(got), bits(want))
		}
	}

	// And the two halves separately, so a failure localises.
	for _, c := range cases {
		xi, bench, _ := refBestXIWith(c.squad, c.mustStart)
		if got, want := xiValue(xi), refXIValue(xi); !sameFloat(got, want) {
			t.Fatalf("%s: xiValue = %s, want %s", c.label, bits(got), bits(want))
		}
		if got, want := benchValue(xi, bench, c.benchWeigh, false),
			refBenchValue(xi, bench, c.benchWeigh); !sameFloat(got, want) {
			t.Fatalf("%s: benchValue = %s, want %s", c.label, bits(got), bits(want))
		}
		gk, out := slotProbabilities(xi)
		refGK, refOut := refSlotProbabilities(xi)
		if !sameFloat(gk, refGK) {
			t.Fatalf("%s: slotProbabilities gk = %s, want %s", c.label, bits(gk), bits(refGK))
		}
		for i := range out {
			if !sameFloat(out[i], refOut[i]) {
				t.Fatalf("%s: slotProbabilities[%d] = %s, want %s", c.label,
					i, bits(out[i]), bits(refOut[i]))
			}
		}
	}
}

// TestOptimizerDiffHarnessHasTeeth checks the checker.
//
// A differential test that cannot fail is worse than no test, because it reads
// as evidence. This runs implementations with each of the specific defects the
// optimisation could plausibly introduce, and requires the comparison above to
// reject every one of them. The recorded precedent is the append test that
// passed because it supplied its own distinguishing inputs: a test that cannot
// be shown to fail has not been shown to do anything.
func TestOptimizerDiffHarnessHasTeeth(t *testing.T) {
	cases := optimizerDiffCases(t)

	broken := map[string]xiFunc{
		// An unstable sort. Identical on distinct scores, different under ties —
		// which is precisely trap 1, and the reason the shipped sorts must stay
		// stable rather than merely "sorted".
		"unstable position sort": func(squad []PlayerMetrics, mustStart map[int]bool) ([]PlayerMetrics, []PlayerMetrics, string) {
			return brokenBestXI(squad, mustStart, brokenUnstable)
		},
		// Normalising the input order before sorting. This is the tempting
		// "optimisation" — id-order the squad once and reuse it — and it is
		// wrong for the same reason.
		"id-sorted input": func(squad []PlayerMetrics, mustStart map[int]bool) ([]PlayerMetrics, []PlayerMetrics, string) {
			return brokenBestXI(squad, mustStart, brokenIDSort)
		},
		// Reversing the tie-break, i.e. taking the last tied player instead of
		// the first.
		"reversed ties": func(squad []PlayerMetrics, mustStart map[int]bool) ([]PlayerMetrics, []PlayerMetrics, string) {
			return brokenBestXI(squad, mustStart, brokenReverseTies)
		},
		// Accepting a formation on >= rather than >, so an equal-scoring later
		// formation wins. Changes the formation string and the bench order
		// without changing the objective at all — the quietest of the four.
		"ties go to the later formation": func(squad []PlayerMetrics, mustStart map[int]bool) ([]PlayerMetrics, []PlayerMetrics, string) {
			return brokenBestXI(squad, mustStart, brokenGE)
		},
	}

	for name, impl := range broken {
		t.Run(name, func(t *testing.T) {
			if err := checkAgainstReference(cases, impl); err == nil {
				t.Fatalf("the comparison accepted a deliberately broken %s; "+
					"it is not evidence of anything", name)
			}
		})
	}
}

type brokenMode int

const (
	brokenUnstable brokenMode = iota
	brokenIDSort
	brokenReverseTies
	brokenGE
)

// brokenBestXI is the reference with one defect injected, selected by mode.
func brokenBestXI(squad []PlayerMetrics, mustStart map[int]bool, mode brokenMode) (xi, bench []PlayerMetrics, formation string) {
	work := append([]PlayerMetrics(nil), squad...)
	if mode == brokenIDSort {
		sort.SliceStable(work, func(i, j int) bool { return work[i].ID < work[j].ID })
	}

	byPos := map[string][]PlayerMetrics{}
	forced := map[string]int{}
	for _, p := range work {
		byPos[p.Position] = append(byPos[p.Position], p)
		if mustStart[p.ID] {
			forced[p.Position]++
		}
	}
	for pos := range byPos {
		less := func(i, j int) bool {
			a, b := byPos[pos][i], byPos[pos][j]
			if mustStart[a.ID] != mustStart[b.ID] {
				return mustStart[a.ID]
			}
			if mode == brokenReverseTies && a.Score == b.Score {
				return a.ID > b.ID
			}
			return a.Score > b.Score
		}
		if mode == brokenUnstable {
			// A genuine unstable sort: reverse first, then sort. On distinct keys
			// this is identical; under ties it lands on the opposite order.
			for l, r := 0, len(byPos[pos])-1; l < r; l, r = l+1, r-1 {
				byPos[pos][l], byPos[pos][r] = byPos[pos][r], byPos[pos][l]
			}
			sort.SliceStable(byPos[pos], less)
		} else {
			sort.SliceStable(byPos[pos], less)
		}
	}

	bestTotal := -1.0
	var bestPick []PlayerMetrics
	var bestForm string
	for d := xiMin["DEF"]; d <= xiMax["DEF"]; d++ {
		for m := xiMin["MID"]; m <= xiMax["MID"]; m++ {
			for f := xiMin["FWD"]; f <= xiMax["FWD"]; f++ {
				if 1+d+m+f != 11 {
					continue
				}
				if len(byPos["GKP"]) < 1 || len(byPos["DEF"]) < d ||
					len(byPos["MID"]) < m || len(byPos["FWD"]) < f {
					continue
				}
				if forced["GKP"] > 1 || forced["DEF"] > d ||
					forced["MID"] > m || forced["FWD"] > f {
					continue
				}
				pick := append([]PlayerMetrics{}, byPos["GKP"][:1]...)
				pick = append(pick, byPos["DEF"][:d]...)
				pick = append(pick, byPos["MID"][:m]...)
				pick = append(pick, byPos["FWD"][:f]...)
				total := refXIValue(pick)
				better := total > bestTotal
				if mode == brokenGE {
					better = total >= bestTotal
				}
				if better {
					bestTotal, bestPick = total, pick
					bestForm = fmt.Sprintf("%d-%d-%d", d, m, f)
				}
			}
		}
	}

	inXI := map[int]bool{}
	for _, p := range bestPick {
		inXI[p.ID] = true
	}
	for _, p := range work {
		if !inXI[p.ID] {
			bench = append(bench, p)
		}
	}
	sort.SliceStable(bench, func(i, j int) bool {
		if (bench[i].Position == "GKP") != (bench[j].Position == "GKP") {
			return bench[j].Position == "GKP"
		}
		return bench[i].Score > bench[j].Score
	})
	sort.SliceStable(bestPick, func(i, j int) bool { return bestPick[i].Score > bestPick[j].Score })
	return bestPick, bench, bestForm
}

// TestStableTieOrderFollowsInputOrder pins trap 1 as a property of the model
// rather than an accident of the code.
//
// Two squads with the same players in a different order can score differently,
// and that is correct behaviour, not a bug to be normalised away: players tied
// on Score are not interchangeable, because blankRate reads their minutes and
// slotProbabilities convolves the eleven in sorted order. So the stable sort's
// choice of which tied player starts is observable in the objective.
//
// If this test ever fails because the objective became order-independent,
// somebody has removed the bench-slot derivation or sorted the input — either
// way the differential test above is no longer covering what it claims to.
func TestStableTieOrderFollowsInputOrder(t *testing.T) {
	if !derivedBenchSlots {
		t.Skip("bench slots are the fixed tuple; the objective cannot see tie order")
	}

	// The tie has to sit at the *margin of the eleven*, not anywhere in it. Two
	// tied players who both start are interchangeable in the convolution and the
	// float result can come out identical — the first version of this fixture
	// tied the two best defenders and measured nothing. Here the third and
	// fourth defenders tie, the winning formation starts three, so exactly one
	// of them plays and the other is benched. They differ sharply in expected
	// minutes, so which one starts moves the whole blank-rate convolution.
	mk := func(id int, pos string, score, mins float64) PlayerMetrics {
		return PlayerMetrics{ID: id, Name: fmt.Sprintf("p%d", id), Position: pos,
			Team: fmt.Sprintf("T%d", id), Price: 5.0, Score: score,
			ExpectedMinutes: mins, Status: "available"}
	}
	squad := []PlayerMetrics{
		mk(1, "GKP", 3.0, 90), mk(2, "GKP", 2.0, 10),
		mk(3, "DEF", 5.0, 85), mk(4, "DEF", 4.5, 85),
		// The tied pair, on the 3-4 boundary: identical score, opposite minutes.
		mk(5, "DEF", 3.0, 90), mk(6, "DEF", 3.0, 20),
		mk(7, "DEF", 1.0, 80),
		mk(8, "MID", 5.0, 85), mk(9, "MID", 4.8, 85), mk(10, "MID", 4.6, 85),
		mk(11, "MID", 4.4, 85), mk(12, "MID", 4.2, 85),
		mk(13, "FWD", 5.5, 85), mk(14, "FWD", 5.3, 85), mk(15, "FWD", 4.0, 85),
	}

	// Confirm the fixture really does bench one of the tied pair, so a pass
	// cannot come from both of them starting.
	xi, _, form := bestXIWith(squad, nil)
	started := 0
	for _, p := range xi {
		if p.ID == 5 || p.ID == 6 {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("fixture drifted: formation %s starts %d of the tied pair, want 1",
			form, started)
	}

	swapped := append([]PlayerMetrics(nil), squad...)
	swapped[4], swapped[5] = swapped[5], swapped[4]

	a := objectiveWith(squad, DefaultBenchWeight, nil, false)
	b := objectiveWith(swapped, DefaultBenchWeight, nil, false)

	if sameFloat(a, b) {
		t.Fatalf("swapping two tied defenders left the objective at %s; tie order "+
			"is supposed to be observable, so the differential test's shuffled "+
			"cases are not testing what they claim", bits(a))
	}

	// And both orderings must still match the reference exactly.
	if got, want := a, refObjectiveWith(squad, DefaultBenchWeight, nil); !sameFloat(got, want) {
		t.Errorf("original order: got %s, want %s", bits(got), bits(want))
	}
	if got, want := b, refObjectiveWith(swapped, DefaultBenchWeight, nil); !sameFloat(got, want) {
		t.Errorf("swapped order: got %s, want %s", bits(got), bits(want))
	}
}

// TestReplaceKeepsIncomingAtOutgoingIndex pins trap 2.
//
// replace deliberately writes the incoming player into the outgoing player's
// slot rather than appending him, so a trial squad is neither id-ordered nor
// grouped by position. The singles and pairs loops score thousands of these, so
// any fast path that assumes an ordering is wrong on the whole hot path while
// being right on every tidy fixture.
func TestReplaceKeepsIncomingAtOutgoingIndex(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	squad := diffSquad(rng, 40)

	// Replace a defender in the middle of the DEF block.
	victim := squad[4]
	if victim.Position != "DEF" {
		t.Fatalf("fixture drifted: index 4 is %s", victim.Position)
	}
	in := diffPlayer(rng, 500, "DEF", 40)
	got := replace(squad, victim.ID, in)

	if got[4].ID != in.ID {
		t.Errorf("incoming player is at index %d, want the outgoing index 4",
			indexOfID(got, in.ID))
	}
	if len(got) != len(squad) {
		t.Errorf("replace changed the squad size to %d", len(got))
	}
	// The point of the trap: this slice is not id-ordered.
	ordered := true
	for i := 1; i < len(got); i++ {
		if got[i].ID < got[i-1].ID {
			ordered = false
			break
		}
	}
	if ordered {
		t.Error("the fixture produced an id-ordered trial squad, so it does not " +
			"exercise the trap it exists for")
	}
	// And the objective agrees with the reference on it.
	if a, b := objectiveWith(got, DefaultBenchWeight, nil, false),
		refObjectiveWith(got, DefaultBenchWeight, nil); !sameFloat(a, b) {
		t.Errorf("objective on a replaced squad: got %s, want %s", bits(a), bits(b))
	}
}

func indexOfID(s []PlayerMetrics, id int) int {
	for i, p := range s {
		if p.ID == id {
			return i
		}
	}
	return -1
}
