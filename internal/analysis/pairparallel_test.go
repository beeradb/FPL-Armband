package analysis

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"testing"
)

// The paired scan of polish's local search runs in parallel. This file is the
// evidence that it returns the same answer as the serial scan it replaced, and
// the same answer every time.
//
// # Why an oracle rather than a reading of the diff
//
// The recorded failure mode of this package is a faster path that quietly
// returns a DIFFERENT legal squad while every existing test passes, because the
// existing tests assert invariants — the squad is legal, the eleven has eleven
// players — and a different legal squad satisfies all of them. See
// optimizerdiff_test.go, which makes the same argument about the objective.
//
// The paired scan is worse than that, because its accept test is
// `s > best.score+1e-9` against a moving best: two candidates a hair apart both
// look "as good", and which one wins is decided by the order they are seen in.
// A racing reduction therefore does not produce a wrong squad, it produces a
// DIFFERENT squad on each page load from identical inputs — a class of defect
// that shows up as a user seeing the recommendation change under them, and as
// every byte-identical invariance in the research record ceasing to be evidence.
// TestDiagDPSeedOrderIsNotDeterministic records the last time this bit.
//
// So: refPolish below is the serial search copied VERBATIM from dpseed.go at
// the commit before this work, kept as a frozen oracle, and the assertions are
// bit equality via math.Float64bits over both the chosen fifteen and its score.
//
// Unlike optimizerdiff_test.go's oracle this one deliberately DOES share the
// objective, replaceInto and fundedUpgrade with the shipped code. What changed
// is the iteration and reduction of the pair scan and nothing else, so freezing
// the objective here as well would test a change that was not made — and the
// objective already has its own frozen oracle in that file.
//
// Do not tidy refPolish and do not let it drift toward the implementation. If a
// later change to polish makes it fail to compile, copy the NEW serial search in
// beside it rather than editing this one to match.

func (e *Engine) refPolish(start []PlayerMetrics, pool []PlayerMetrics, budget int,
	benchWeight float64, boost bool, locked, mustStart map[int]bool, paired bool,
	changes changeBudget) ([]PlayerMetrics, float64, int) {

	selected := map[int]PlayerMetrics{}
	posCount := map[string]int{}
	clubCount := map[string]int{}
	spend := 0
	add := func(m PlayerMetrics) {
		selected[m.ID] = m
		posCount[m.Position]++
		clubCount[m.Team]++
		spend += int(m.Price*10 + 0.5)
	}
	for _, p := range start {
		add(p)
	}

	// One scratch for this whole search. A local rather than engine state on
	// purpose: the tool runner runs searches concurrently, and shared mutable
	// state on Engine is the recorded concurrent-map-write hazard.
	sc := &xiScratch{}
	// Likewise the funded-upgrade phase's own scratch — see fundingScratch.
	fs := &fundingScratch{}

	// Steepest-ascent local search: repeatedly apply the single swap that
	// improves the objective most, until no swap helps.
	current := squadSlice(selected)
	bestScore := sc.objective(current, benchWeight, mustStart, boost)
	// The starting squad may already differ from the baseline — a DP seed does,
	// and so does a repaired one — so measure rather than assume zero.
	spent := changes.distance(current)

	cheapByPos := cheapestByPosition(pool, 14)
	strongByPos := strongestByPosition(pool, 30)
	frontierByPos := positionFrontier(pool, 2)

	// The three move types are wrapped so their order can change. Order is free
	// when the budget is unbounded and decisive when it is not: run to
	// convergence on single swaps first and, at a budget of two, the first
	// change goes on the best single swap while a pair costs two — so pairs are
	// never considered at all. Multi-change moves have to get first claim.
	runSingles := func() {
		for iter := 0; iter < 200; iter++ {
			type move struct {
				out, in PlayerMetrics
				score   float64
			}
			bestMove := move{score: bestScore}

			for _, out := range current {
				for _, in := range pool {
					if _, already := selected[in.ID]; already {
						continue
					}
					if in.Position != out.Position {
						continue
					}
					if locked[out.ID] {
						continue
					}
					newSpend := spend - int(out.Price*10+0.5) + int(in.Price*10+0.5)
					if newSpend > budget {
						continue
					}
					// Club limit after the swap: out leaves, in joins. Same
					// question runPairs asks via clubCountAfter below —
					// provably identical to a hand-rolled counter, so it is
					// one implementation rather than two of the same check.
					if clubCountAfter(clubCount, out.Team, in.Team) > MaxPerClub {
						continue
					}

					if !changes.unlimited() && spent+changes.delta(out, in) > changes.Max {
						continue
					}
					sc.trial = replaceInto(sc.trial, current, out.ID, in)
					if s := sc.objective(sc.trial, benchWeight, mustStart, boost); s > bestMove.score+1e-9 {
						bestMove = move{out: out, in: in, score: s}
					}
				}
			}

			if bestMove.in.ID == 0 {
				break
			}
			spent += changes.delta(bestMove.out, bestMove.in)
			delete(selected, bestMove.out.ID)
			posCount[bestMove.out.Position]--
			clubCount[bestMove.out.Team]--
			spend -= int(bestMove.out.Price*10 + 0.5)
			add(bestMove.in)
			current = squadSlice(selected)
			bestScore = bestMove.score
		}

	}

	// Paired downgrade-and-upgrade. A single 1-for-1 swap can never fund a
	// premium starter by dropping to cheap bench fodder: the downgrade alone
	// lowers the objective, so steepest-ascent rejects it and the search stalls
	// in a local optimum. This move evaluates both halves together, which is
	// what a human manager actually does when building a squad.
	runPairs := func() {
		for iter := 0; iter < 60; iter++ {
			type pairMove struct {
				downOut, downIn PlayerMetrics
				upOut, upIn     PlayerMetrics
				score           float64
			}
			best := pairMove{score: bestScore}

			for _, downOut := range current {
				if locked[downOut.ID] {
					continue
				}
				for _, downIn := range cheapByPos[downOut.Position] {
					if _, already := selected[downIn.ID]; already {
						continue
					}
					freed := int(downOut.Price*10+0.5) - int(downIn.Price*10+0.5)
					if freed <= 0 {
						continue
					}
					if c := clubCountAfter(clubCount, downOut.Team, downIn.Team); c > MaxPerClub {
						continue
					}

					for _, upOut := range current {
						if upOut.ID == downOut.ID || locked[upOut.ID] {
							continue
						}
						for _, upIn := range strongByPos[upOut.Position] {
							if upIn.ID == downIn.ID {
								continue
							}
							if _, already := selected[upIn.ID]; already {
								continue
							}
							newSpend := spend - int(downOut.Price*10+0.5) + int(downIn.Price*10+0.5) -
								int(upOut.Price*10+0.5) + int(upIn.Price*10+0.5)
							if newSpend > budget {
								continue
							}
							if !clubsLegalAfterPair(clubCount, downOut, downIn, upOut, upIn) {
								continue
							}

							d := changes.delta(downOut, downIn) + changes.delta(upOut, upIn)
							if !changes.unlimited() && spent+d > changes.Max {
								continue
							}
							// Two buffers: the second replace reads the first's
							// output, so they must not be the same array.
							sc.trial = replaceInto(sc.trial, current, downOut.ID, downIn)
							sc.trial2 = replaceInto(sc.trial2, sc.trial, upOut.ID, upIn)
							if s := sc.objective(sc.trial2, benchWeight, mustStart, boost); s > best.score+1e-9 {
								best = pairMove{downOut, downIn, upOut, upIn, s}
							}
						}
					}
				}
			}

			if best.downIn.ID == 0 {
				break
			}
			spent += changes.delta(best.downOut, best.downIn) +
				changes.delta(best.upOut, best.upIn)
			for _, rm := range []PlayerMetrics{best.downOut, best.upOut} {
				delete(selected, rm.ID)
				posCount[rm.Position]--
				clubCount[rm.Team]--
				spend -= int(rm.Price*10 + 0.5)
			}
			add(best.downIn)
			add(best.upIn)
			current = squadSlice(selected)
			bestScore = best.score
		}

	}

	// Several downgrades funding one upgrade. The paired phase above frees money
	// from exactly one player, which cannot reach an upgrade whose cost is spread
	// across three cheap slots — see funding.go.
	if !fundedUpgradeEnabled {
		return current, bestScore, spend
	}
	runFunded := func() {
		for iter := 0; iter < 30; iter++ {
			cand, score, cost, ok := e.fundedUpgrade(sc, fs, current, selected, clubCount,
				spend, budget, frontierByPos,
				benchWeight, boost, locked, mustStart, bestScore, changes, spent)
			if !ok {
				break
			}
			selected = map[int]PlayerMetrics{}
			posCount = map[string]int{}
			clubCount = map[string]int{}
			spend = 0
			for _, p := range cand {
				add(p)
			}
			current = squadSlice(selected)
			bestScore, spend = score, cost
			spent = changes.distance(current)
		}

	}

	if changes.unlimited() {
		// Unchanged from before the budget existed, deliberately: this ordering
		// is what every measured result in this package was produced under.
		runSingles()
		if !paired {
			return current, bestScore, spend
		}
		runPairs()
		if fundedUpgradeEnabled {
			runFunded()
		}
		return current, bestScore, spend
	}

	// Bounded: the expensive moves choose first.
	if paired {
		if fundedUpgradeEnabled {
			runFunded()
		}
		runPairs()
	}
	runSingles()
	return current, bestScore, spend
}

// ---------------------------------------------------------------------------
// The corpus.
//
// One landscape is not evidence. Budget is the generator for the same reason
// searchquality_test.go gives: every £0.5m changes which combinations are
// affordable across the whole pool, where a weight sweep moves a dozen players
// and then stops moving anything at all. The other axes are the ones that
// change which branches of the pair scan are live: a bench weight (so the bench
// half of the objective is not constant), the bench-boost flag (which bypasses
// the slot weights entirely), locks (which skip whole downOut and upOut rows),
// forced starters (which change the formation search), and a change budget
// (which is what makes the reduction reject moves the scan accepted).
// ---------------------------------------------------------------------------

type pairCase struct {
	label       string
	pool, squad []PlayerMetrics
	budget      int
	benchWeight float64
	boost       bool
	locked      map[int]bool
	mustStart   map[int]bool
	changes     changeBudget
}

func pairCases(t testing.TB) []pairCase {
	t.Helper()
	var out []pairCase

	for _, n := range []int{200, 400, 600} {
		pool := benchPool(n)
		squad := benchSquad(pool)
		if len(squad) != SquadSize {
			t.Fatalf("pool %d: fixture squad has %d players", n, len(squad))
		}
		spend := 0
		for _, p := range squad {
			spend += priceUnits(p)
		}

		// Headroom sweep. At +0 the search can only trade sideways; by +160 it
		// can restructure, which is the case the paired move exists for.
		for _, extra := range []int{0, 20, 60, 120, 160} {
			budget := spend + extra
			out = append(out, pairCase{
				label: fmt.Sprintf("pool%d/budget+%d", n, extra),
				pool:  pool, squad: squad, budget: budget,
				benchWeight: DefaultBenchWeight,
				locked:      map[int]bool{},
			})
		}

		budget := spend + 100
		for _, bw := range []float64{0.0, 0.15, DefaultBenchWeight, 0.6} {
			out = append(out, pairCase{
				label: fmt.Sprintf("pool%d/bench%.2f", n, bw),
				pool:  pool, squad: squad, budget: budget,
				benchWeight: bw,
				locked:      map[int]bool{},
			})
		}

		out = append(out, pairCase{
			label: fmt.Sprintf("pool%d/benchboost", n),
			pool:  pool, squad: squad, budget: budget,
			benchWeight: DefaultBenchWeight, boost: true,
			locked: map[int]bool{},
		})

		// Locks: the cheapest and the dearest of the fifteen, so both a
		// downOut row and an upOut row are cut out of the scan.
		cheapest, dearest := 0, 0
		for i := range squad {
			if priceUnits(squad[i]) < priceUnits(squad[cheapest]) {
				cheapest = i
			}
			if priceUnits(squad[i]) > priceUnits(squad[dearest]) {
				dearest = i
			}
		}
		out = append(out, pairCase{
			label: fmt.Sprintf("pool%d/locked", n),
			pool:  pool, squad: squad, budget: budget,
			benchWeight: DefaultBenchWeight,
			locked: map[int]bool{
				squad[cheapest].ID: true,
				squad[dearest].ID:  true,
			},
		})

		out = append(out, pairCase{
			label: fmt.Sprintf("pool%d/muststart", n),
			pool:  pool, squad: squad, budget: budget,
			benchWeight: DefaultBenchWeight,
			locked:      map[int]bool{squad[dearest].ID: true},
			mustStart:   map[int]bool{squad[dearest].ID: true},
		})

		// A bounded revision, which reverses the phase order in polish and puts
		// the pair scan first — see the `if paired` branch there.
		baseline := make(map[int]bool, len(squad))
		for _, p := range squad {
			baseline[p.ID] = true
		}
		for _, max := range []int{1, 2, 5} {
			out = append(out, pairCase{
				label: fmt.Sprintf("pool%d/changes%d", n, max),
				pool:  pool, squad: squad, budget: budget,
				benchWeight: DefaultBenchWeight,
				locked:      map[int]bool{},
				changes:     changeBudget{Baseline: baseline, Max: max},
			})
		}
	}
	return out
}

// squadFingerprint is the answer, byte for byte: the fifteen in the order the
// search returned them — order is observable, see squadSlice and repairClubs —
// with every id and every score's exact bits, plus the objective and the spend.
//
// Bits rather than a formatted float, for the reason optimizerdiff_test.go
// gives: the objective feeds an argmax, so a difference of one ULP flips a
// player, and a tolerance cannot tell "the same computation" from "a
// nearly-identical one".
func squadFingerprint(squad []PlayerMetrics, score float64, spend int) string {
	var b []byte
	for _, p := range squad {
		b = fmt.Appendf(b, "%d:%016x|", p.ID, math.Float64bits(p.Score))
	}
	return fmt.Sprintf("%s score=%016x spend=%d", b, math.Float64bits(score), spend)
}

func runPairCase(e *Engine, c pairCase) string {
	got, score, spend := e.polish(c.squad, c.pool, c.budget, c.benchWeight,
		c.boost, c.locked, c.mustStart, true, c.changes)
	return squadFingerprint(got, score, spend)
}

func runPairCaseSerial(e *Engine, c pairCase) string {
	got, score, spend := e.refPolish(c.squad, c.pool, c.budget, c.benchWeight,
		c.boost, c.locked, c.mustStart, true, c.changes)
	return squadFingerprint(got, score, spend)
}

// withGOMAXPROCS runs fn at a fixed P count and restores the old one.
func withGOMAXPROCS(t *testing.T, n int, fn func()) {
	t.Helper()
	old := runtime.GOMAXPROCS(n)
	defer runtime.GOMAXPROCS(old)
	fn()
}

// TestPairScanMatchesTheSerialSearch is the differential test: the parallel
// scan against a frozen copy of the serial one, over the whole corpus, at every
// worker count that matters.
//
// GOMAXPROCS 1 is not the trivial arm. A reduction bug that depends on which
// worker saw a candidate is invisible there, so a green single-worker run is
// exactly the false comfort this has to be run at 2, 3 and 8 to rule out.
func TestPairScanMatchesTheSerialSearch(t *testing.T) {
	cases := pairCases(t)
	if len(cases) < 30 {
		t.Fatalf("only %d cases; this test is evidence only in bulk", len(cases))
	}
	e := &Engine{}

	// The oracle is serial, so compute it once, at whatever P the test binary
	// was started with.
	want := make([]string, len(cases))
	for i, c := range cases {
		want[i] = runPairCaseSerial(e, c)
	}
	t.Logf("comparing %d cases against the frozen serial search", len(cases))

	for _, procs := range []int{1, 2, 3, 8} {
		withGOMAXPROCS(t, procs, func() {
			for i, c := range cases {
				if got := runPairCase(e, c); got != want[i] {
					t.Fatalf("GOMAXPROCS=%d %s:\n parallel %s\n serial   %s",
						procs, c.label, got, want[i])
				}
			}
		})
	}
}

// TestPairScanIsDeterministicAcrossWorkerCounts runs the same input many times
// and requires the same fifteen every time, at four worker counts.
//
// Repetition is the point. A racing reduction does not fail every run — it
// fails whenever two near-equal candidates land in a different order, which on
// a loaded box is a minority of runs. One comparison would pass and the site
// would still change its recommendation between page loads.
func TestPairScanIsDeterministicAcrossWorkerCounts(t *testing.T) {
	// A subset of the corpus, not all of it: this arm pays 20 runs per case per
	// worker count, and the differential test above is what covers breadth.
	// These three between them exercise both phase orders the pair scan runs
	// under: unbounded with enough headroom to restructure, which is the
	// production shape, and a bounded revision, which puts the pair scan
	// first — see the `if paired` branch in polish.
	want := map[string]bool{
		"pool600/budget+160": true,
		"pool400/budget+120": true,
		"pool600/changes2":   true,
	}
	var cases []pairCase
	for _, c := range pairCases(t) {
		if want[c.label] {
			cases = append(cases, c)
		}
	}
	if len(cases) != len(want) {
		t.Fatalf("selected %d of %d named cases; the corpus labels moved", len(cases), len(want))
	}
	e := &Engine{}
	const repeats = 20

	var seenWorkers []int
	observePairScan = func(workers, chunks int) { seenWorkers = append(seenWorkers, workers) }
	defer func() { observePairScan = nil }()

	first := map[string]string{}
	for _, procs := range []int{1, 2, 3, 8} {
		withGOMAXPROCS(t, procs, func() {
			for _, c := range cases {
				for r := 0; r < repeats; r++ {
					got := runPairCase(e, c)
					prev, ok := first[c.label]
					if !ok {
						first[c.label] = got
						continue
					}
					if got != prev {
						t.Fatalf("%s: run %d at GOMAXPROCS=%d differs from the first run\n got  %s\n want %s",
							c.label, r, procs, got, prev)
					}
				}
			}
		})
	}

	// Liveness. Without this the test above passes just as happily on a scan
	// that never left one goroutine, which is the state this change exists to
	// end.
	max := 0
	for _, w := range seenWorkers {
		if w > max {
			max = w
		}
	}
	if len(seenWorkers) == 0 {
		t.Fatal("the paired scan never ran: nothing to be deterministic about")
	}
	if runtime.NumCPU() > 1 && max < 2 {
		t.Fatalf("the paired scan never used more than one worker over %d iterations "+
			"on a %d-CPU box; the determinism above is vacuous", len(seenWorkers), runtime.NumCPU())
	}
}

// TestReducingPerChunkBestsWouldChangeTheAnswer is the teeth of the design.
//
// The cheap parallel reduction — let each chunk keep its own best, then fold
// the chunk bests together in index order — LOOKS order-preserving and is not,
// because the accept test compares against a threshold that moves as the scan
// proceeds. A chunk evaluated from the iteration's entry score can offer a
// candidate the full scan would have suppressed with something from an earlier
// chunk. That is why runPairs records every accepted candidate and replays it.
//
// This asserts the trap is real rather than defended against on a hunch: over a
// seeded corpus of score sequences the two rules must DISAGREE. If a later
// change made them agree, the buffering in runPairs would be dead weight and
// this test is what says so.
func TestReducingPerChunkBestsWouldChangeTheAnswer(t *testing.T) {
	const eps = 1e-9

	// Both rules return the winning candidate's identity — chunk and offset —
	// because "the same score" is not the question. Two candidates a hair apart
	// are two different fifteens.
	type at struct{ chunk, idx int }
	miss := at{-1, -1}

	// full is the shipped rule: one pass, one moving threshold.
	full := func(entry float64, chunks [][]float64) at {
		best, win := entry, miss
		for c, ch := range chunks {
			for i, s := range ch {
				if s > best+eps {
					best, win = s, at{c, i}
				}
			}
		}
		return win
	}
	// perChunk is the tempting one: each chunk resolved from the entry score,
	// then the chunk answers folded in index order.
	perChunk := func(entry float64, chunks [][]float64) at {
		type res struct {
			score float64
			where at
			ok    bool
		}
		local := make([]res, len(chunks))
		for c, ch := range chunks {
			b := entry
			for i, s := range ch {
				if s > b+eps {
					b, local[c] = s, res{s, at{c, i}, true}
				}
			}
		}
		best, win := entry, miss
		for _, r := range local {
			if r.ok && r.score > best+eps {
				best, win = r.score, r.where
			}
		}
		return win
	}

	// The minimal witness. Chunk 0 holds a candidate that suppresses the first
	// of chunk 1 but not the second; run alone, chunk 1 stops at its first and
	// offers a candidate the full scan rejected, so the two rules pick
	// different squads. Stated in units of the search's own epsilon.
	witness := [][]float64{{10 * eps}, {10.5 * eps, 11.2 * eps}}
	if gotFull, gotChunk := full(0, witness), perChunk(0, witness); gotFull == gotChunk {
		t.Fatalf("the stated witness no longer separates the two reductions: both picked %v", gotFull)
	} else {
		t.Logf("witness: full scan picks %v, per-chunk reduction picks %v", gotFull, gotChunk)
	}

	// And in bulk, on sequences shaped like the scan's: many candidates within
	// an epsilon or two of each other.
	rng := rand.New(rand.NewSource(20260825))
	disagree := 0
	const trials = 20000
	for i := 0; i < trials; i++ {
		chunks := make([][]float64, 1+rng.Intn(6))
		for c := range chunks {
			chunks[c] = make([]float64, 1+rng.Intn(5))
			for j := range chunks[c] {
				chunks[c][j] = float64(rng.Intn(24)) * eps / 2
			}
		}
		if full(0, chunks) != perChunk(0, chunks) {
			disagree++
		}
	}
	if disagree == 0 {
		t.Fatal("per-chunk reduction agreed on every one of the trials; " +
			"either the generator stopped producing near-ties or the rule changed")
	}
	t.Logf("per-chunk reduction picks a different candidate on %d of %d sequences (%.1f%%)",
		disagree, trials, 100*float64(disagree)/trials)
}

// BenchmarkPairScan is the before/after, in one binary and on one fixture.
//
// Both arms run the same search over the same squad and pool; the only
// difference between them is the pair scan's iteration. Measuring them in the
// same process is not a convenience — this box runs several agents at once, so
// two separate `go test` invocations minutes apart are not comparable, and the
// ratio between two arms interleaved by the benchmark driver is.
//
// Run it across worker counts with -cpu:
//
//	go test ./internal/analysis -run XXX -bench BenchmarkPairScan -cpu 1,3,8
func BenchmarkPairScan(b *testing.B) {
	pool := benchPool(600)
	squad := benchSquad(pool)
	if len(squad) != SquadSize {
		b.Fatalf("fixture squad has %d players", len(squad))
	}
	spend := 0
	for _, p := range squad {
		spend += priceUnits(p)
	}
	// The headroom matters: with none, the paired move has nothing to fund and
	// the scan the benchmark exists to measure barely runs.
	budget := spend + 160
	e := &Engine{}

	b.Run("serial", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			got, score, _ := e.refPolish(squad, pool, budget, DefaultBenchWeight,
				false, map[int]bool{}, nil, true, changeBudget{})
			if len(got) != SquadSize || score == 0 {
				b.Fatal("degenerate result")
			}
		}
	})

	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			got, score, _ := e.polish(squad, pool, budget, DefaultBenchWeight,
				false, map[int]bool{}, nil, true, changeBudget{})
			if len(got) != SquadSize || score == 0 {
				b.Fatal("degenerate result")
			}
		}
	})
}
