package analysis

// The differential test for the funding phase's scratch-reuse rewrite.
//
// fundingCombos and fundedUpgrade were rewritten to index the DP cells with
// pointers into a reused scratch (fundingScratch) instead of allocating a
// fresh map[int]opt per slot and a fresh []cell layer per call — see
// funding.go's package comment on fundOpt/fundCell/fundingScratch. None of
// that is supposed to change a single number: it is storage and allocation
// lifetime only, never iteration order or arithmetic.
//
// Following optimizerdiff_test.go's own pattern (see its doc comment for why
// a reviewer reading the diff is not enough here), the pre-rewrite
// implementations are frozen below, verbatim, as an oracle, and every result
// is compared to the shipped scratch-based path by exact equality — a slice
// deep-equal for a reconstructed squad, math.Float64bits for a score. A
// tolerance would hide exactly the class of bug this guards: the DP's parent
// pointers walking into a cell some other slot has since overwritten, which
// this codebase has already shipped once as a rolling (non-layered) array
// and had to un-ship.
//
// The oracle is deliberately run with a SHARED, REUSED fs across every trial
// in the loop below (never a fresh one per case) — the harshest exercise of
// the scratch's grow/shrink/clear logic across varying slot counts and
// maxFreed values, which is exactly where a stale-buffer bug would show.
//
// bestXIWith and objectiveWith are called directly rather than re-frozen: they
// are the unmodified cold entry points this change routes fundedUpgrade
// through instead (sc.split/bestFormation/materialise, sc.objective), and
// they already have their own bit-exact oracle in optimizerdiff_test.go.
// Re-freezing them here would be a second implementation of that guard, not a
// stronger one.

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// Frozen reference implementations, copied verbatim from funding.go at the
// commit before the scratch-reuse rewrite. Do not "tidy" them and do not make
// them share code with fundingCombos/fundedUpgrade.
// ---------------------------------------------------------------------------

func refFundingCombos(current []PlayerMetrics, selected map[int]PlayerMetrics,
	clubCount map[string]int, downByPos map[string][]PlayerMetrics,
	skip map[int]bool, locked map[int]bool, shortfall int, limit int,
	slotWeight []float64, reserved int,
) []map[int]PlayerMetrics {

	if shortfall <= 0 {
		return nil
	}
	maxFreed := shortfall + fundingSlack

	type refOpt struct {
		in    PlayerMetrics
		freed int
		delta float64
	}
	slots := make([][]refOpt, len(current))
	for i, out := range current {
		if skip[out.ID] || locked[out.ID] {
			continue
		}
		bestAt := map[int]refOpt{}
		for _, in := range downByPos[out.Position] {
			if _, already := selected[in.ID]; already {
				continue
			}
			if in.ID == reserved {
				continue
			}
			freed := priceUnits(out) - priceUnits(in)
			if freed <= 0 || freed > maxFreed {
				continue
			}
			if clubCountAfter(clubCount, out.Team, in.Team) > MaxPerClub {
				continue
			}
			d := (in.Score - out.Score) * slotWeight[i]
			if cur, ok := bestAt[freed]; !ok || d > cur.delta {
				bestAt[freed] = refOpt{in, freed, d}
			}
		}
		for _, o := range bestAt {
			slots[i] = append(slots[i], o)
		}
		sort.Slice(slots[i], func(a, b int) bool { return slots[i][a].freed < slots[i][b].freed })
	}

	type refCell struct {
		delta float64
		used  bool
		in    PlayerMetrics
		took  bool
		prevF int
	}
	layers := make([][]refCell, len(slots)+1)
	layers[0] = make([]refCell, maxFreed+1)
	layers[0][0].used = true

	for i := range slots {
		prev := layers[i]
		next := make([]refCell, maxFreed+1)
		copy(next, prev)
		for f := range next {
			if prev[f].used {
				next[f] = refCell{prev[f].delta, true, PlayerMetrics{}, false, f}
			}
		}
		for f := 0; f <= maxFreed; f++ {
			if !prev[f].used {
				continue
			}
			for _, o := range slots[i] {
				nf := f + o.freed
				if nf > maxFreed {
					nf = maxFreed
				}
				nd := prev[f].delta + o.delta
				if !next[nf].used || nd > next[nf].delta {
					next[nf] = refCell{nd, true, o.in, true, f}
				}
			}
		}
		layers[i+1] = next
	}
	final := layers[len(slots)]

	type reach struct {
		f     int
		delta float64
	}
	var ends []reach
	for f := shortfall; f <= maxFreed; f++ {
		if final[f].used {
			ends = append(ends, reach{f, final[f].delta})
		}
	}
	sort.Slice(ends, func(a, b int) bool { return ends[a].delta > ends[b].delta })
	if len(ends) > limit {
		ends = ends[:limit]
	}

	var out []map[int]PlayerMetrics
	for _, e := range ends {
		combo := map[int]PlayerMetrics{}
		used := map[int]bool{}
		f := e.f
		dup := false
		for i := len(slots); i > 0; i-- {
			c := layers[i][f]
			if !c.used {
				break
			}
			if c.took {
				if used[c.in.ID] {
					dup = true
					break
				}
				used[c.in.ID] = true
				combo[i-1] = c.in
			}
			f = c.prevF
		}
		if !dup && len(combo) > 0 {
			out = append(out, combo)
		}
	}
	return out
}

func refFundedUpgrade(current []PlayerMetrics, selected map[int]PlayerMetrics,
	clubCount map[string]int, spend, budget int,
	frontierByPos map[string][]PlayerMetrics,
	benchWeight float64, boost bool, locked, mustStart map[int]bool, bestScore float64,
	changes changeBudget, spent int,
) (squad []PlayerMetrics, score float64, cost int, ok bool) {

	xi, _, _ := bestXIWith(current, mustStart)
	inXI := map[int]bool{}
	for _, p := range xi {
		inXI[p.ID] = true
	}
	slotWeight := make([]float64, len(current))
	for i, p := range current {
		slotWeight[i] = benchWeight
		if inXI[p.ID] {
			slotWeight[i] = 1
		}
	}

	best := bestScore
	for _, upOut := range current {
		if locked[upOut.ID] {
			continue
		}
		for _, upIn := range frontierByPos[upOut.Position] {
			if _, already := selected[upIn.ID]; already {
				continue
			}
			if upIn.Score <= upOut.Score {
				continue
			}
			if clubCountAfter(clubCount, upOut.Team, upIn.Team) > MaxPerClub {
				continue
			}
			shortfall := spend - priceUnits(upOut) + priceUnits(upIn) - budget
			if shortfall <= 0 {
				continue
			}

			skip := map[int]bool{upOut.ID: true}
			combos := refFundingCombos(current, selected, clubCount, frontierByPos,
				skip, locked, shortfall, fundingCandidates, slotWeight, upIn.ID)

			for _, combo := range combos {
				trial := append([]PlayerMetrics(nil), current...)
				for idx, in := range combo {
					trial[idx] = in
				}
				for i := range trial {
					if trial[i].ID == upOut.ID {
						trial[i] = upIn
					}
				}
				if !squadIsLegal(trial, budget) || !holdsLocks(trial, locked) {
					continue
				}
				if !changes.unlimited() && changes.distance(trial) > changes.Max {
					continue
				}
				s := objectiveWith(trial, benchWeight, mustStart, boost)
				if s > best+1e-9 {
					total := 0
					for _, p := range trial {
						total += priceUnits(p)
					}
					best, squad, cost, ok = s, trial, total, true
				}
			}
		}
	}
	return squad, best, cost, ok
}

// ---------------------------------------------------------------------------
// Randomised fixtures.
// ---------------------------------------------------------------------------

// randFundingFixture builds a legal 2/5/5/3 squad and an oversupplied pool
// around it, on a coarse price/score grid so ties are common — the same
// reasoning benchPool's own comment gives: a fixture of distinct floats hides
// exactly the tie-break trap (fundingCombos' "first wins" rule at equal
// delta) this harness exists to catch.
func randFundingFixture(rng *rand.Rand) (current, pool []PlayerMetrics) {
	quota := []string{
		"GKP", "GKP",
		"DEF", "DEF", "DEF", "DEF", "DEF",
		"MID", "MID", "MID", "MID", "MID",
		"FWD", "FWD", "FWD",
	}
	teams := make([]string, 8)
	for i := range teams {
		teams[i] = fmt.Sprintf("T%02d", i)
	}
	id := 0
	add := func(pos string) PlayerMetrics {
		id++
		return PlayerMetrics{
			ID:       id,
			Name:     fmt.Sprintf("%s%d", pos, id),
			Position: pos,
			Team:     teams[rng.Intn(len(teams))],
			Price:    float64(30+rng.Intn(100)) / 10, // 3.0-12.9
			Score:    float64(rng.Intn(120)) * 0.05,   // coarse grid, ties common
			Status:   "available",
		}
	}
	for _, pos := range quota {
		current = append(current, add(pos))
	}
	pool = append(pool, current...)
	for i := 0; i < 60; i++ {
		pool = append(pool, add(quota[rng.Intn(len(quota))]))
	}
	return current, pool
}

// squadWeights assigns a random starter/bench-shaped weight per slot — 1 or a
// small bench weight — mirroring what fundedUpgrade actually hands
// fundingCombos, rather than the all-ones weight the unit tests in
// funding_test.go use.
func squadWeights(rng *rand.Rand, n int, benchWeight float64) []float64 {
	w := make([]float64, n)
	for i := range w {
		if rng.Intn(3) == 0 {
			w[i] = benchWeight
		} else {
			w[i] = 1
		}
	}
	return w
}

// TestFundingScratchMatchesTheFrozenOracle is the byte-identity harness the
// scratch-reuse rewrite is gated on. It runs both fundingCombos and
// fundedUpgrade — sharing one fundingScratch/xiScratch pair across every
// trial, deliberately, to exercise the buffer growing and shrinking between
// differently-shaped calls — against the frozen pre-rewrite oracle above, and
// requires exact equality on every trial.
func TestFundingScratchMatchesTheFrozenOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20260825))
	const benchWeight = 0.15
	e := &Engine{}
	sc := &xiScratch{}
	fs := &fundingScratch{}

	const trials = 60
	var combosCompared, upgradesCompared int
	for trial := 0; trial < trials; trial++ {
		current, pool := randFundingFixture(rng)
		selected := map[int]PlayerMetrics{}
		clubCount := map[string]int{}
		spend := 0
		for _, p := range current {
			selected[p.ID] = p
			clubCount[p.Team]++
			spend += priceUnits(p)
		}
		frontierByPos := positionFrontier(pool, 2)

		// fundingCombos directly, at a handful of shortfalls and locked sets,
		// with weights shaped like the real starter/bench split.
		for _, shortfall := range []int{5, 15, 30, 55} {
			locked := map[int]bool{}
			if trial%4 == 0 {
				locked[current[trial%len(current)].ID] = true
			}
			w := squadWeights(rng, len(current), benchWeight)

			want := refFundingCombos(current, selected, clubCount, frontierByPos,
				map[int]bool{}, locked, shortfall, fundingCandidates, w, 0)
			got := fundingCombos(fs, current, selected, clubCount, frontierByPos,
				map[int]bool{}, locked, shortfall, fundingCandidates, w, 0)
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("trial %d shortfall %d: fundingCombos diverged from the oracle\nwant: %+v\ngot:  %+v",
					trial, shortfall, want, got)
			}
			combosCompared++
		}

		// fundedUpgrade end to end, at budgets that force the funded phase to
		// fire (tight) and one with a little slack.
		for _, slack := range []int{0, -10, -25} {
			budget := spend + slack
			before := objectiveWith(current, benchWeight, nil, false)

			wantSquad, wantScore, wantCost, wantOK := refFundedUpgrade(current, selected, clubCount,
				spend, budget, frontierByPos, benchWeight, false, map[int]bool{}, nil, before,
				changeBudget{}, 0)
			gotSquad, gotScore, gotCost, gotOK := e.fundedUpgrade(sc, fs, current, selected, clubCount,
				spend, budget, frontierByPos, benchWeight, false, map[int]bool{}, nil, before,
				changeBudget{}, 0)

			if wantOK != gotOK {
				t.Fatalf("trial %d slack %d: ok diverged: want %v got %v", trial, slack, wantOK, gotOK)
			}
			if wantOK {
				if !reflect.DeepEqual(wantSquad, gotSquad) {
					t.Fatalf("trial %d slack %d: squad diverged\nwant: %+v\ngot:  %+v",
						trial, slack, wantSquad, gotSquad)
				}
				if math.Float64bits(wantScore) != math.Float64bits(gotScore) {
					t.Fatalf("trial %d slack %d: score diverged: want %v (%x) got %v (%x)",
						trial, slack, wantScore, math.Float64bits(wantScore),
						gotScore, math.Float64bits(gotScore))
				}
				if wantCost != gotCost {
					t.Fatalf("trial %d slack %d: cost diverged: want %d got %d", trial, slack, wantCost, gotCost)
				}
			}
			upgradesCompared++
		}
	}
	t.Logf("compared %d fundingCombos calls and %d fundedUpgrade calls against the frozen oracle",
		combosCompared, upgradesCompared)
}
