package analysis

import "slices"

// Exact dynamic-programming seeds for the squad optimiser.
//
// The optimiser proper is a steepest-ascent local search. It is fast and it
// handles every constraint (locks, exclusions, club limits, minutes floors),
// but it explores from a single greedy starting point, and a squad is a
// multi-dimensional knapsack with a rugged objective — a local optimum is not
// necessarily a good one.
//
// It stalled on a real question. Asked whether Haaland fits, single and paired
// swaps from the greedy seed said funding him cost about 3.3 points per
// gameweek. An exact search over the same pool found a 3-4-3 that fits him for
// 0.26, by moving money out of the goalkeeper rather than out of the outfield —
// a restructure no sequence of local swaps reaches, because every step of it is
// downhill.
//
// So: solve each formation exactly by DP, hand the results to the local search
// as starting points, and keep the best. The DP relaxes the club limit (which a
// cost-indexed knapsack cannot express) and approximates the bench as its
// cheapest legal filling; the local search then repairs both. A seed does not
// need to be legal or complete, only good enough to start from.
type dpPoint struct {
	score float64
	ok    bool
	sel   []int // indices into the position pool; only set on position curves
	split int   // cost taken by the left operand; only set on combined curves
}

// dpCurve maps "cost at most c" to the best achievable score.
type dpCurve []dpPoint

// positionCurve solves "best k players from pool for at most c" for every k up
// to kmax and every cost up to budget.
func positionCurve(pool []PlayerMetrics, kmax, budget int) []dpCurve {
	curves := make([]dpCurve, kmax+1)
	for k := range curves {
		curves[k] = make(dpCurve, budget+1)
	}
	curves[0][0] = dpPoint{ok: true}

	for i, p := range pool {
		cost := priceUnits(p)
		if cost > budget {
			continue
		}
		for k := kmax; k >= 1; k-- {
			prev, cur := curves[k-1], curves[k]
			for c := budget - cost; c >= 0; c-- {
				src := prev[c]
				if !src.ok {
					continue
				}
				if got := src.score + p.Score; !cur[c+cost].ok || got > cur[c+cost].score {
					sel := make([]int, len(src.sel), len(src.sel)+1)
					copy(sel, src.sel)
					cur[c+cost] = dpPoint{score: got, ok: true, sel: append(sel, i)}
				}
			}
		}
	}
	for k := range curves {
		prefixMax(curves[k])
	}
	return curves
}

// prefixMax rewrites a curve so index c means "at most c" rather than "exactly c".
func prefixMax(c dpCurve) {
	best := dpPoint{}
	for i := range c {
		if c[i].ok && (!best.ok || c[i].score > best.score) {
			best = c[i]
		}
		c[i] = best
	}
}

// combine convolves two curves, recording where the cost was split so the
// selection can be recovered afterwards.
func combine(a, b dpCurve) dpCurve {
	out := make(dpCurve, len(a))
	for c := range out {
		for c1 := 0; c1 <= c; c1++ {
			x, y := a[c1], b[c-c1]
			if !x.ok || !y.ok {
				continue
			}
			if got := x.score + y.score; !out[c].ok || got > out[c].score {
				out[c] = dpPoint{score: got, ok: true, split: c1}
			}
		}
	}
	return out
}

func priceUnits(p PlayerMetrics) int { return int(p.Price*10 + 0.5) }

// dpSeeds returns one candidate squad per legal formation, each solved exactly
// for its starting eleven. Squads may violate the club limit or fall short of
// fifteen players; the caller repairs them.
func (e *Engine) dpSeeds(pool []PlayerMetrics, budget int, locked []PlayerMetrics) [][]PlayerMetrics {
	// Locked players are pre-placed: they consume budget and quota, and the DP
	// solves for what is left. Without this the seeds would drop them, so
	// seeding had to be skipped whenever a lock was set — which left exactly
	// the scenario-testing path ("what if I keep this striker?") running the
	// weaker greedy-only search.
	lockedAt := map[string]int{}
	lockedCost := 0
	isLocked := map[int]bool{}
	for _, p := range locked {
		lockedAt[p.Position]++
		lockedCost += priceUnits(p)
		isLocked[p.ID] = true
	}
	budget -= lockedCost
	if budget < 0 {
		return nil
	}

	byPos := map[string][]PlayerMetrics{}
	for _, p := range pool {
		if isLocked[p.ID] {
			continue
		}
		byPos[p.Position] = append(byPos[p.Position], p)
	}
	// Trim each position to its efficient frontier: at each price point only
	// the few best scorers can ever be worth picking. This keeps the DP small
	// without changing the answer in any case that matters.
	for pos := range byPos {
		byPos[pos] = frontier(byPos[pos], 4)
	}
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		if lockedAt[pos] > squadQuota[pos] {
			return nil
		}
		if len(byPos[pos]) < squadQuota[pos]-lockedAt[pos] {
			return nil // cannot form a legal squad from this pool
		}
	}

	curves := map[string][]dpCurve{}
	for pos, quota := range squadQuota {
		curves[pos] = positionCurve(byPos[pos], quota-lockedAt[pos], budget)
	}

	var seeds [][]PlayerMetrics
	for d := xiMin["DEF"]; d <= xiMax["DEF"]; d++ {
		for m := xiMin["MID"]; m <= xiMax["MID"]; m++ {
			for f := xiMin["FWD"]; f <= xiMax["FWD"]; f++ {
				if 1+d+m+f != 11 {
					continue
				}
				if s := e.seedFor(byPos, curves, budget, d, m, f, locked, lockedAt); s != nil {
					seeds = append(seeds, s)
				}
			}
		}
	}
	return seeds
}

// seedFor solves one formation: reserve the cheapest legal bench, spend the
// rest on the best possible eleven, then fill the bench for real.
func (e *Engine) seedFor(byPos map[string][]PlayerMetrics, curves map[string][]dpCurve,
	budget, d, m, f int, locked []PlayerMetrics, lockedAt map[string]int) []PlayerMetrics {

	xiCount := map[string]int{"GKP": 1, "DEF": d, "MID": m, "FWD": f}

	// Locked players take their positions' starting slots first; the DP fills
	// whatever remains of the eleven and then the bench.
	dpXI := map[string]int{}
	for pos, n := range xiCount {
		dpXI[pos] = n - lockedAt[pos]
		if dpXI[pos] < 0 {
			return nil // more locked at this position than the formation starts
		}
	}

	// Reserve the cheapest players that could fill the bench slots, so the XI
	// budget is what is genuinely spendable.
	//
	// Sort the costs here rather than relying on how frontier() happens to order
	// its output. Indexing that ordering from the wrong end reserved the most
	// expensive possible bench instead of the cheapest, cut tens of millions off
	// the XI budget, and quietly made every seed too poor to afford a premium —
	// which is precisely the class of squad the DP exists to find.
	reserved := 0
	for pos, quota := range squadQuota {
		need := quota - lockedAt[pos] - dpXI[pos]
		if need <= 0 {
			continue
		}
		costs := make([]int, 0, len(byPos[pos]))
		for _, p := range byPos[pos] {
			costs = append(costs, priceUnits(p))
		}
		slices.Sort(costs)
		for i := 0; i < need && i < len(costs); i++ {
			reserved += costs[i]
		}
	}
	xiBudget := budget - reserved
	if xiBudget <= 0 {
		return nil
	}

	// Combine in a fixed order so the splits can be unwound.
	gk := curves["GKP"][dpXI["GKP"]]
	c2 := combine(gk, curves["DEF"][dpXI["DEF"]])
	c3 := combine(c2, curves["MID"][dpXI["MID"]])
	c4 := combine(c3, curves["FWD"][dpXI["FWD"]])
	if xiBudget >= len(c4) {
		xiBudget = len(c4) - 1
	}
	if !c4[xiBudget].ok {
		return nil
	}

	// Unwind: c4 -> (c3, FWD), c3 -> (c2, MID), c2 -> (GKP, DEF).
	s3 := c4[xiBudget].split
	fwdCost := xiBudget - s3
	s2 := c3[s3].split
	midCost := s3 - s2
	s1 := c2[s2].split
	defCost := s2 - s1

	picks := []struct {
		pos  string
		k    int
		cost int
	}{{"GKP", dpXI["GKP"], s1}, {"DEF", dpXI["DEF"], defCost},
		{"MID", dpXI["MID"], midCost}, {"FWD", dpXI["FWD"], fwdCost}}

	squad := append([]PlayerMetrics(nil), locked...)
	taken := map[int]bool{}
	for _, p := range locked {
		taken[p.ID] = true
	}
	for _, p := range picks {
		cell := curves[p.pos][p.k][p.cost]
		if !cell.ok {
			return nil
		}
		for _, idx := range cell.sel {
			pm := byPos[p.pos][idx]
			squad = append(squad, pm)
			taken[pm.ID] = true
		}
	}

	// Fill the bench with the cheapest players not already picked.
	//
	// Iterate posNames, NOT the squadQuota map. The *set* of bench players is the
	// same either way, since each position draws only from its own candidates — but
	// `squad` is a slice and its ORDER is read twice downstream: repairClubs scans it
	// in slice order to choose which player to drop from an over-limit club, and the
	// seed-ranking loop in Optimize compares objectiveWith(seed) across formations
	// with a bare `>`, over a sum taken in squad order. Ranging the map therefore made
	// `Optimize` return different fifteens from byte-identical inputs — measured at
	// 12 distinct seed orders from 12 identical dpSeeds calls, changing the final
	// answer on about one landscape in seventy. See TestDiagDPSeedOrderIsNotDeterministic.
	//
	// That mattered beyond the squad: every byte-identical invariance in the research
	// record is only evidence if the search is deterministic.
	spend := 0
	for _, p := range squad {
		spend += priceUnits(p)
	}
	for _, pos := range posNames {
		quota := squadQuota[pos]
		need := quota - lockedAt[pos] - dpXI[pos]
		cheap := append([]PlayerMetrics(nil), byPos[pos]...)
		slices.SortStableFunc(cheap, func(a, b PlayerMetrics) int {
			if a.Price != b.Price {
				if a.Price < b.Price {
					return -1
				}
				return 1
			}
			return byScoreDesc(a, b)
		})
		for _, c := range cheap {
			if need == 0 {
				break
			}
			if taken[c.ID] {
				continue
			}
			squad = append(squad, c)
			taken[c.ID] = true
			spend += priceUnits(c)
			need--
		}
		if need > 0 {
			return nil
		}
	}
	if spend > budget+lockedSpend(locked) || len(squad) != SquadSize {
		return nil
	}
	return squad
}

// frontier keeps, at each distinct price, only the best few scorers. Anyone
// else is dominated: same cost, less return, and the only reason to prefer him
// would be a club limit, which the local search resolves afterwards.
func frontier(ps []PlayerMetrics, perPrice int) []PlayerMetrics {
	byPrice := map[int][]PlayerMetrics{}
	for _, p := range ps {
		u := priceUnits(p)
		byPrice[u] = append(byPrice[u], p)
	}
	prices := make([]int, 0, len(byPrice))
	for u := range byPrice {
		prices = append(prices, u)
	}
	slices.Sort(prices)

	var out []PlayerMetrics
	for _, u := range prices {
		g := byPrice[u]
		slices.SortStableFunc(g, byScoreDesc)
		if len(g) > perPrice {
			g = g[:perPrice]
		}
		out = append(out, g...)
	}
	return out
}

// polish runs the optimiser's local search from a given starting squad and
// returns the improved squad, its objective, and its cost.
//
// Extracted so the search can be started from several places. It used to run
// once, from a single greedy seed; it now also runs from each exact DP seed,
// and the caller keeps whichever result is best. Every constraint is enforced
// here, so a seed may be illegal — over the club limit, say — and still be a
// useful place to start.
// paired controls the expensive second phase. It exists to escape the greedy
// seed's local optimum, so it is worth its cost there and wasted on a DP seed,
// which is already exact for its formation.
// changeBudget bounds how far the search may move from a squad you already own.
//
// It is what makes one search serve both jobs. Building a fifteen from nothing
// is the same problem as changing an existing one, except that in-season the
// scarce resource is transfers rather than only money. Baseline holds the squad
// you have; Max is how many of its players may end up replaced. A wildcard sets
// Max to the squad size, a normal week to the free transfers available plus one.
//
// Max counts *edit distance from Baseline*, not moves applied. Counting moves
// would let the search spend its budget swapping a player out and back in, and
// charge for two transfers that cancel.
type changeBudget struct {
	Baseline map[int]bool
	Max      int
}

// unlimited reports whether the budget imposes no constraint.
func (c changeBudget) unlimited() bool { return c.Baseline == nil || c.Max <= 0 }

// delta is how much a single out-for-in swap moves the edit distance.
func (c changeBudget) delta(out, in PlayerMetrics) int {
	if c.unlimited() {
		return 0
	}
	switch {
	case c.Baseline[out.ID] && !c.Baseline[in.ID]:
		return 1
	case !c.Baseline[out.ID] && c.Baseline[in.ID]:
		return -1
	}
	return 0
}

// distance is how many of squad's players are not in the baseline.
func (c changeBudget) distance(squad []PlayerMetrics) int {
	if c.Baseline == nil {
		return 0
	}
	n := 0
	for _, p := range squad {
		if !c.Baseline[p.ID] {
			n++
		}
	}
	return n
}

func (e *Engine) polish(start []PlayerMetrics, pool []PlayerMetrics, budget int,
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
			cand, score, cost, ok := e.fundedUpgrade(current, selected, clubCount,
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

// repairClubs swaps out surplus players until no club exceeds MaxPerClub,
// preferring to drop the lowest scorer at the offending club and replace him
// with the best affordable alternative in the same position.
//
// The DP cannot express the club limit — a cost-indexed knapsack has no room
// for "at most three from Arsenal" — so it is relaxed there and repaired here.
// Returns nil if the squad cannot be made legal, in which case the seed is
// simply discarded.
func repairClubs(squad []PlayerMetrics, pool []PlayerMetrics, budget int) []PlayerMetrics {
	out := append([]PlayerMetrics(nil), squad...)

	for guard := 0; guard < SquadSize; guard++ {
		counts := map[string]int{}
		spend := 0
		in := map[int]bool{}
		for _, p := range out {
			counts[p.Team]++
			spend += priceUnits(p)
			in[p.ID] = true
		}

		// Pick the offending club by NAME, not by map order. Two clubs really are
		// over the limit simultaneously on real seeds, and `break` on the first hit
		// made which one gets repaired first depend on Go's randomised map
		// iteration. The loop repairs until none is over, so the orders usually
		// converge — but "usually" is not what an invariance check needs.
		over := ""
		for team, n := range counts {
			if n > MaxPerClub && (over == "" || team < over) {
				over = team
			}
		}
		if over == "" {
			return out
		}

		// Drop the weakest player from the offending club.
		worst := -1
		for i, p := range out {
			if p.Team == over && (worst < 0 || p.Score < out[worst].Score) {
				worst = i
			}
		}
		if worst < 0 {
			return nil
		}
		gone := out[worst]

		best := -1
		for i, cand := range pool {
			if in[cand.ID] || cand.Position != gone.Position {
				continue
			}
			// Deliberately NOT clubCountAfter/clubHeadroom: this squad is
			// already illegal (that is why repairClubs is running at all),
			// so counts[cand.Team] can already be over MaxPerClub for a club
			// OTHER than gone's — the shared helpers assume a legal starting
			// point and would reject a same-club replacement that is exactly
			// what is wanted here (cand.Team == gone.Team keeps the count
			// unchanged, which is always allowed regardless of how high it
			// already sits). Folding this in would change what
			// dpseedorder_test.go pins.
			if counts[cand.Team] >= MaxPerClub && cand.Team != gone.Team {
				continue
			}
			if spend-priceUnits(gone)+priceUnits(cand) > budget {
				continue
			}
			if best < 0 || cand.Score > pool[best].Score {
				best = i
			}
		}
		if best < 0 {
			return nil
		}
		out[worst] = pool[best]
	}
	return nil
}

// squadIsLegal reports whether a squad satisfies every hard FPL constraint.
func squadIsLegal(squad []PlayerMetrics, budget int) bool {
	if len(squad) != SquadSize {
		return false
	}
	pos := map[string]int{}
	club := map[string]int{}
	spend := 0
	seen := map[int]bool{}
	for _, p := range squad {
		if seen[p.ID] {
			return false
		}
		seen[p.ID] = true
		pos[p.Position]++
		club[p.Team]++
		spend += priceUnits(p)
	}
	if spend > budget {
		return false
	}
	for p, want := range squadQuota {
		if pos[p] != want {
			return false
		}
	}
	for _, n := range club {
		if n > MaxPerClub {
			return false
		}
	}
	return true
}

// lockedSpend is the cost of the pre-placed players, which seedFor already
// subtracted from its working budget.
func lockedSpend(locked []PlayerMetrics) int {
	t := 0
	for _, p := range locked {
		t += priceUnits(p)
	}
	return t
}

// holdsLocks reports whether every locked player is still present. repairClubs
// and the local search can both move players, so a seed's locks are re-checked
// rather than assumed — a seed that loses one is simply discarded.
func holdsLocks(squad []PlayerMetrics, locked map[int]bool) bool {
	if len(locked) == 0 {
		return true
	}
	have := make(map[int]bool, len(squad))
	for _, p := range squad {
		have[p.ID] = true
	}
	for id := range locked {
		if !have[id] {
			return false
		}
	}
	return true
}
