package analysis

import (
	"fmt"
	"math/rand"
	"testing"
)

// This file tests fillBound (squad.go) — the two-tier admissible bound the
// greedy fill in Optimize consults before committing a candidate. The one
// property that matters is ADMISSIBILITY: tier1 must never exceed the true
// cheapest legal completion, and tier2 (the exact solve) must equal it.
// Over-estimating by even £0.1m silently rejects a reachable squad and turns
// this from a feasibility fix into an unmeasured search-quality regression.

// mkFillPlayer builds a minimal PlayerMetrics for exercising fillBound
// directly — only ID, Position, Team and Price are read by it.
func mkFillPlayer(id int, pos, team string, priceTenths int) PlayerMetrics {
	return PlayerMetrics{ID: id, Name: fmt.Sprintf("p%d", id), Position: pos,
		Team: team, Price: float64(priceTenths) / 10}
}

// needPosCount turns a desired [GKP,DEF,MID,FWD] need into the posCount map
// fillNeed expects, so a hand test can say what it needs directly instead of
// reverse-engineering it from squadQuota.
func needPosCount(need [4]int) map[string]int {
	out := map[string]int{}
	for i, pos := range posNames {
		out[pos] = squadQuota[pos] - need[i]
	}
	return out
}

// TestTier1IsALowerBoundNotAlwaysTight is the plan's first worked
// counterexample: club A capped at one more player (clubCount[A]=2, so
// MaxPerClub-2=1 of headroom), needing one GKP and one DEF. A holds a £4.0m
// GKP and a £4.0m DEF; the cheapest elsewhere are a £4.5m GKP and a £10.0m
// DEF.
//
// The tempting "fix" — a club counter shared across the whole sort — takes
// BOTH of A's players and reports £14.0m, which is inadmissible: it exceeds
// the true optimum of £8.5m (DEF from A, GKP elsewhere), so it would reject
// a reachable squad. Tier 1 gives each position A's slot independently and
// reports £8.0m (GKP from A + DEF from A, ignoring that only one can really
// come from A) — a valid LOWER bound (8.0 <= 8.5) but not tight, which is
// exactly why Tier 2 has to run before a candidate is actually accepted.
func TestTier1IsALowerBoundNotAlwaysTight(t *testing.T) {
	pool := []PlayerMetrics{
		mkFillPlayer(1, "GKP", "A", 40),
		mkFillPlayer(2, "DEF", "A", 40),
		mkFillPlayer(3, "GKP", "B", 45),
		mkFillPlayer(4, "DEF", "B", 100),
	}
	fb := buildFillBound(pool)
	posCount := needPosCount([4]int{1, 1, 0, 0})
	clubCount := map[string]int{"A": 2} // headroom(A) = 3-2 = 1
	p := boundParams{id: -1}

	if got := fb.tier1(posCount, clubCount, p); got != 80 {
		t.Errorf("tier1 = %d, want 80 (£4.0m GKP@A + £4.0m DEF@A, the club-relaxed sum)", got)
	}
	if got := fb.exact(posCount, clubCount, p); got != 85 {
		t.Errorf("exact = %d, want 85 (£4.0m DEF@A + £4.5m GKP@B — only one player may "+
			"actually come from A)", got)
	}
}

// TestTier1AloneDeadEndsButTier2Solves is the plan's second worked
// counterexample, and the reason both tiers must ship together: club A
// capped at one more (clubCount[A]=2), needing one DEF and one FWD. A holds
// a £4.0m DEF and a £4.5m FWD; elsewhere the cheapest are a £4.5m DEF and a
// £9.0m FWD.
//
// Tier 1 reports £8.5m (DEF@A + FWD@A, each given A's slot independently),
// which is NOT achievable — only one of the two can really come from A. The
// true minimum is £9.0m (FWD from A, DEF elsewhere — cheaper than the
// reverse because the elsewhere-FWD is the expensive one). A remaining
// budget between those two numbers is exactly where "Tier 1 alone" would
// wrongly accept a candidate it cannot actually complete on: this is not a
// smaller version of the bug, it IS the bug, just relocated one tier up.
func TestTier1AloneDeadEndsButTier2Solves(t *testing.T) {
	pool := []PlayerMetrics{
		mkFillPlayer(1, "DEF", "A", 40),
		mkFillPlayer(2, "FWD", "A", 45),
		mkFillPlayer(3, "DEF", "B", 45),
		mkFillPlayer(4, "FWD", "B", 90),
	}
	fb := buildFillBound(pool)
	posCount := needPosCount([4]int{0, 1, 0, 1})
	clubCount := map[string]int{"A": 2}
	p := boundParams{id: -1}

	if got := fb.tier1(posCount, clubCount, p); got != 85 {
		t.Errorf("tier1 = %d, want 85 — the club-relaxed sum that a Tier-1-only accept "+
			"gate would wrongly trust", got)
	}
	if got := fb.exact(posCount, clubCount, p); got != 90 {
		t.Errorf("exact = %d, want 90, the true cheapest legal completion", got)
	}

	// A remaining budget of 86-89 is exactly the trap: Tier 1 says yes,
	// reality says no. Confirm cost() — the hot-path entry point — agrees
	// with exact() rather than stopping at Tier 1's optimistic number.
	for _, remaining := range []int{85, 87, 89, 90} {
		got := fb.cost(posCount, clubCount, p, remaining)
		want := 90
		if got != want {
			t.Errorf("cost(remaining=%d) = %d, want %d (the exact answer, not tier1's 85)",
				remaining, got, want)
		}
	}
}

// randomFillBoundPool builds a small synthetic pool: 2-6 clubs, 8-20
// players, spread round-robin across the four positions so no position
// starves the brute-force oracle below of candidates to choose from.
func randomFillBoundPool(rng *rand.Rand) ([]PlayerMetrics, []string) {
	numClubs := 2 + rng.Intn(5) // 2..6
	clubs := make([]string, numClubs)
	for i := range clubs {
		clubs[i] = fmt.Sprintf("C%d", i)
	}
	n := 8 + rng.Intn(17) // 8..24
	pool := make([]PlayerMetrics, 0, n)
	for i := 0; i < n; i++ {
		pos := posNames[i%4]
		team := clubs[rng.Intn(numClubs)]
		price := 30 + rng.Intn(150) // tenths: £3.0m-£17.9m
		pool = append(pool, mkFillPlayer(i+1, pos, team, price))
	}
	return pool, clubs
}

// bruteForceMinCompletion is the admissibility test's ground truth,
// deliberately independent of fillBound's own code: it enumerates every
// combination of `need[pos]` candidates per position, checks the club cap
// on their UNION (the constraint tier1 relaxes and tier2 does not), and
// returns the cheapest that satisfies it. Small pools keep this tractable —
// see randomFillBoundPool's bounds.
func bruteForceMinCompletion(pool []PlayerMetrics, picked map[int]bool, p boundParams,
	need [4]int, clubCount map[string]int) (cost int, feasible bool) {
	var byPos [4][]PlayerMetrics
	for _, m := range pool {
		if picked[m.ID] || m.ID == p.id {
			continue
		}
		byPos[posIdx(m.Position)] = append(byPos[posIdx(m.Position)], m)
	}

	headroom := func(club string) int {
		h := MaxPerClub - clubCount[club]
		if club == p.team && p.team != "" {
			h--
		}
		if h < 0 {
			h = 0
		}
		return h
	}

	// combos enumerates every size-k subset of items (as index sets),
	// calling emit with each.
	var combos func(items []PlayerMetrics, k int, start int, chosen []PlayerMetrics, emit func([]PlayerMetrics))
	combos = func(items []PlayerMetrics, k int, start int, chosen []PlayerMetrics, emit func([]PlayerMetrics)) {
		if k == 0 {
			emit(chosen)
			return
		}
		for i := start; i <= len(items)-k; i++ {
			combos(items, k-1, i+1, append(chosen, items[i]), emit)
		}
	}

	var gkpSets, defSets, midSets, fwdSets [][]PlayerMetrics
	targets := [4]*[][]PlayerMetrics{&gkpSets, &defSets, &midSets, &fwdSets}
	for i := 0; i < 4; i++ {
		if need[i] == 0 {
			*targets[i] = [][]PlayerMetrics{nil}
			continue
		}
		if len(byPos[i]) < need[i] {
			return 0, false // not even enough candidates to try
		}
		combos(byPos[i], need[i], 0, nil, func(s []PlayerMetrics) {
			cp := append([]PlayerMetrics(nil), s...)
			*targets[i] = append(*targets[i], cp)
		})
	}

	best := -1
	for _, gs := range gkpSets {
		for _, ds := range defSets {
			for _, ms := range midSets {
				for _, fs := range fwdSets {
					clubUsed := map[string]int{}
					total := 0
					for _, group := range [][]PlayerMetrics{gs, ds, ms, fs} {
						for _, pl := range group {
							clubUsed[pl.Team]++
							total += priceUnits(pl)
						}
					}
					ok := true
					for club, n := range clubUsed {
						if n > headroom(club) {
							ok = false
							break
						}
					}
					if ok && (best < 0 || total < best) {
						best = total
					}
				}
			}
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// TestFillBoundIsAdmissible is the property test the whole fix stands or
// falls on. For randomised small pools and randomised
// selected/posCount/clubCount/pending states it checks, against an
// independent brute-force oracle: tier1 never exceeds the truth, and exact
// (tier1-then-tier2) equals it. Seeded deterministically.
func TestFillBoundIsAdmissible(t *testing.T) {
	rng := rand.New(rand.NewSource(20260825))

	const trials = 300
	for trial := 0; trial < trials; trial++ {
		pool, clubs := randomFillBoundPool(rng)
		fb := buildFillBound(pool)

		picked := map[int]bool{}
		for _, m := range pool {
			if rng.Intn(4) == 0 { // ~25% already committed
				picked[m.ID] = true
				fb.picked[fb.poolIndexByID[m.ID]] = true
			}
		}

		// Small needs, chosen directly rather than derived from a simulated
		// partial squad, so the brute-force oracle's combinatorics stay fast.
		need := [4]int{rng.Intn(3), rng.Intn(3), rng.Intn(3), rng.Intn(3)}
		posCount := needPosCount(need)

		clubCount := map[string]int{}
		for _, c := range clubs {
			clubCount[c] = rng.Intn(MaxPerClub) // 0..2: headroom stays interesting
		}

		p := boundParams{id: -1}
		var avail []PlayerMetrics
		for _, m := range pool {
			if !picked[m.ID] {
				avail = append(avail, m)
			}
		}
		if len(avail) > 0 && rng.Intn(2) == 0 {
			cand := avail[rng.Intn(len(avail))]
			// A real pending candidate only ever reaches this point via
			// canAdd, which already guarantees his club has headroom —
			// mirror that precondition here.
			if clubCount[cand.Team] < MaxPerClub {
				p = boundParams{id: cand.ID, pos: cand.Position, team: cand.Team}
			}
		}

		wantNeed, _ := fillNeed(posCount, p)
		truth, feasible := bruteForceMinCompletion(pool, picked, p, wantNeed, clubCount)

		b := fb.tier1(posCount, clubCount, p)
		e := fb.exact(posCount, clubCount, p)

		if !feasible {
			if e < boundInfeasible {
				t.Fatalf("trial %d: exact()=%d claims a completion exists, but the brute "+
					"force oracle found none (need=%v, clubs=%v)", trial, e, wantNeed, clubs)
			}
			continue
		}
		if b > truth {
			t.Fatalf("trial %d: tier1()=%d exceeds the true cheapest completion %d — "+
				"INADMISSIBLE (need=%v, clubs=%v)", trial, b, truth, wantNeed, clubs)
		}
		if e != truth {
			t.Fatalf("trial %d: exact()=%d, want the true cheapest completion %d "+
				"(need=%v, clubs=%v)", trial, e, truth, wantNeed, clubs)
		}
	}
}

// BenchmarkFillBoundTier1 defends the allocation claim on fillBound's doc
// mechanically: Tier 1 is the path every rejected candidate runs, so it has
// to be allocation-free for the fill loop's ~9000-call worst case to stay
// cheap. Tier 2 is deliberately not benchmarked for zero allocs here — it
// only runs on tier1's accept path, a few times per Optimize call rather
// than thousands, and its DP buffers are hoisted on fillBound but its
// combinatorics are not required to be alloc-free the way Tier 1's reject
// path is.
func BenchmarkFillBoundTier1(b *testing.B) {
	pool := benchPool(600)
	fb := buildFillBound(pool)
	posCount := map[string]int{"GKP": 1, "DEF": 3, "MID": 3, "FWD": 2}
	clubCount := map[string]int{}
	for _, m := range pool {
		clubCount[m.Team] = 0
	}
	p := boundParams{id: pool[0].ID, pos: pool[0].Position, team: pool[0].Team}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := fb.tier1(posCount, clubCount, p); got < 0 {
			b.Fatal("negative bound")
		}
	}
}
