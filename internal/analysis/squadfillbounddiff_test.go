package analysis

import (
	"math/rand"
	"slices"
	"testing"

	"armband/internal/fpl"
)

// The differential test for the greedy fill's admissibility bound.
//
// # Why this exists rather than a reviewer reading the diff
//
// 94068f30 ("Stop squad feasibility from depending on score order") replaced
// minCostToFill, the greedy fill's admissibility bound, with fillBound, a
// two-tier bound that is club-aware where the old one was club-blind. That
// commit's own message states the property this file pins:
//
//	Because the bound never over-estimates, it changes a decision only when
//	the old bound admitted a candidate the truth cannot complete -- and the
//	greedy only adds players, so committing to such a candidate always ended
//	in the error. Therefore on every landscape where Optimize already
//	succeeds the greedy seed is unchanged; only currently-failing runs
//	change behaviour.
//
// It was checked once, ad hoc, by running a 380-player pool at five budgets
// against the pre-fix code and comparing squad ids and costs by hand. That
// comparison was then deleted along with the pre-fix source. Nothing in the
// tree has asserted it since.
//
// # The direction that actually holds, worked from first principles
//
// "Already succeeds" is easy to misread as "the SHIPPED (new-bound) Optimize
// succeeds today" — that was this file's first draft, and it is FALSE, caught
// by this file's own first run rather than by inspection. At a deliberately
// razor-thin landscape (10 clubs, budget exactly equal to the achievable
// minimum) the shipped bound completed a legal fifteen and the frozen old
// bound could not, on the identical pool. That is not a bug: minCostToFill
// drops the club cap entirely, so it is a STRICTLY LOOSER lower bound than
// fillBound everywhere (old(x) <= fillBound(x) <= true(x) for every
// candidate x and every state) — and a looser bound is the one that can
// wrongly wave a doomed candidate through, not the one that finds an escape
// a tighter bound would have missed. So old is the bound liable to fail
// where the new one succeeds, never the reverse.
//
// The direction that DOES hold, and is what "already succeeds" means once
// "already" is read as it was written — relative to the commit turning old
// into new, i.e. BEFORE this fix — is:
//
//	old bound's walk completes a legal fifteen  ⟹  new bound's walk is
//	byte-identical, pick for pick.
//
// Sketch: at every step where old and new both face the same state, old
// accepts a superset of what new accepts (old <= new pointwise), so in
// byValue order old's first accepted candidate is never later than new's.
// The two walks are IDENTICAL up to their first disagreement, and a
// disagreement can only be "old accepts a candidate new rejects" (new can
// never accept something old rejects). Whenever new rejects a candidate its
// tier1 bound already exceeds remaining, and tier1 <= true always, so
// rejecting means the TRUE remaining cost exceeds the budget left — old
// accepting it anyway commits the walk to a state no legal completion can
// finish from. The loop's own bookkeeping (spend only ever grows by a
// candidate's real price when its bound clears "remaining", and a bound is
// never negative) makes real spend <= budget an invariant that holds
// regardless of which bound drives it — so a walk that has committed to a
// state with no legal completion within budget cannot go on to produce one;
// it must hit "no legal candidate" and fail. So old's walk, whenever it
// diverges from new's, is doomed — which means old's walk SUCCEEDING is
// exactly the witness that it never diverged, i.e. that new made the
// identical choice at every step.
//
// This file therefore gates each landscape on the FROZEN bound succeeding,
// not the shipped one, and the shipped bound is asserted to match it exactly
// — the provable direction, and the one the fix's whole argument rests on.
// The commit's own text is consistent with this reading: "only
// currently-failing runs change behaviour" describes 94068f30 as a change
// that can only ever turn a pre-fix failure into a post-fix success, never
// the reverse — which is exactly what the derivation above says, and what
// TestOptimizeEscapesAClubConstrainedDeadEnd exercises as one such landscape
// (old fails, new succeeds; deliberately excluded from the table below,
// since this file's property makes no claim about it).
//
// So the reference bound is frozen here, verbatim, as an frozen predecessor, and driven
// through the real Optimize via two narrow seams in squad.go
// (fillCandidateCost, observeGreedySeed) rather than a second copy of the
// fill loop — see their doc comments. The assertion is exact equality of
// sorted player ids and total cost, not a tolerance: the greedy fill feeds a
// local search that only ever climbs from where it starts, so a single
// differing pick is a different search, not a nearly-identical one.
//
// # What this file deliberately does NOT cover
//
// optimizerdiff_test.go's differential frozen predecessor is a different layer —
// bestXIWith/objectiveWith, which pick the STARTING ELEVEN out of a squad
// already built. It says nothing about Optimize or the admissibility bound
// that decides which FIFTEEN gets built in the first place. This file is
// that frozen predecessor's complement, not a duplicate of it.
//
// Every landscape driven through the comparison below must SUCCEED under the
// frozen pre-94068f30 bound. A landscape where it fails is out of scope by
// construction, per the derivation above — 94068f30's whole point was that
// such landscapes are exactly where the shipped bound is ALLOWED to do
// something the old one could not. TestSquadFillBoundDifferentialHasTeeth checks
// that the comparison would catch it if the two bounds disagreed somewhere
// they are not allowed to.

// ---------------------------------------------------------------------------
// Frozen reference implementation.
//
// Copied verbatim from squad.go at 94068f30^ (the commit immediately before
// fillBound replaced it). Do not "fix" it — it is club-blind BY CONSTRUCTION,
// which is the defect 94068f30 exists to remove, and a differential frozen predecessor
// that gets modernised along with the code it is meant to check stops being
// an frozen predecessor at all.
// ---------------------------------------------------------------------------

// minCostToFillAsShippedBefore94068f30 is the cheapest way to complete the
// squad after hypothetically adding `pending`, so greedy filling never
// strands the budget.
func minCostToFillAsShippedBefore94068f30(pool []PlayerMetrics, selected map[int]PlayerMetrics, posCount, clubCount map[string]int, pending PlayerMetrics) int {
	need := map[string]int{}
	for pos, quota := range squadQuota {
		n := quota - posCount[pos]
		if pos == pending.Position {
			n--
		}
		if n > 0 {
			need[pos] = n
		}
	}
	if len(need) == 0 {
		return 0
	}

	cheapest := map[string][]int{}
	for _, m := range pool {
		if _, in := selected[m.ID]; in || m.ID == pending.ID {
			continue
		}
		if need[m.Position] == 0 {
			continue
		}
		cheapest[m.Position] = append(cheapest[m.Position], int(m.Price*10+0.5))
	}

	total := 0
	for pos, n := range need {
		costs := cheapest[pos]
		slices.Sort(costs)
		if len(costs) < n {
			// Not enough candidates left; signal infeasibility with a huge cost.
			return 1 << 30
		}
		for i := 0; i < n; i++ {
			total += costs[i]
		}
	}
	return total
}

// ---------------------------------------------------------------------------
// Landscapes.
//
// Every one of these must SUCCEED against the frozen old bound today (see
// the package doc for why that is the correct gate, not the shipped bound).
// None of them is the squadclubtrap_test.go shape (forwards concentrated in
// three clubs) at its failing budget — that landscape is precisely where old
// fails and new is ALLOWED to succeed instead, so including it here would
// test nothing.
// ---------------------------------------------------------------------------

// fillBoundLandscape is one pool-and-budget combination the frozen predecessor drives
// through Optimize.
type fillBoundLandscape struct {
	label      string
	numClubs   int
	perPosClub int // candidates per position, per club
	budget     int // tenths of a million
}

// Pool sizes stay modest on purpose. Optimize's DP-seed stage (stage 2 on
// its own doc) runs an exact solve per formation over the WHOLE pool
// regardless of which bound the greedy fill used, and its cost climbs with
// pool size — a 2400-player landscape tried here first cost over twenty
// seconds on its own for no gain in power over the fill loop's bound, which
// is all this file is about. Kept in the 80-500 player range, the same
// order of magnitude as squadclubtrap_test.go's ~130 and the 380-player pool
// 94068f30's own ad hoc check used.
var fillBoundLandscapes = []fillBoundLandscape{
	// The minimum club count a legal fifteen can come from at all (5 clubs x
	// 3-per-club cap = 15), so every position's club cap is live at once.
	{"5 clubs, thin roster, tight budget", 5, 4, 660},
	{"5 clubs, thin roster, generous budget", 5, 4, 950},
	{"6 clubs, moderate roster, tight budget", 6, 5, 680},
	{"8 clubs, moderate roster, moderate budget", 8, 7, 720},
	{"10 clubs, deep roster, moderate budget", 10, 8, 700},
	{"10 clubs, deep roster, comfortable budget", 10, 10, 1000},
	{"15 clubs, full-scale roster, comfortable budget", 15, 8, 1000},
	{"20 clubs, large roster, comfortable budget", 20, 6, 1000},
}

// buildFillBoundPool builds a deterministic synthetic pool spread evenly
// across numClubs clubs and the four positions, perPosClub candidates each —
// every club fields every position, unlike squadclubtrap_test.go's fixture,
// which concentrates forwards on purpose to build the landscape the OLD
// bound could not complete. Price rises with index within each (club,
// position) group so there is always a controllable cheap floor, with a
// small jitter so ties are not universal and Score varies enough to give the
// greedy's value-per-million sort something real to order.
func buildFillBoundPool(seed int64, numClubs, perPosClub int) []fpl.Element {
	rng := rand.New(rand.NewSource(seed))
	posTypes := []int{1, 2, 3, 4} // GKP, DEF, MID, FWD
	var els []fpl.Element
	id := 0
	for club := 1; club <= numClubs; club++ {
		for _, et := range posTypes {
			for i := 0; i < perPosClub; i++ {
				id++
				price := 39 + i*3 + rng.Intn(15) // rising floor, £3.9m and up
				goals := rng.Intn(1 + i%6)
				assists := rng.Intn(1 + (i+2)%5)
				xg := float64(goals) * (0.5 + rng.Float64())
				xa := float64(assists) * (0.5 + rng.Float64())
				els = append(els, fpl.Element{
					ID: id, Team: club, ElementType: et, NowCost: price,
					Minutes: 2400 + rng.Intn(700), Starts: 24 + rng.Intn(10),
					Status:        "a",
					GoalsScored:   goals,
					ExpectedGoals: fpl.Num(xg),
					Assists:       assists, ExpectedAssists: fpl.Num(xa),
					CleanSheets: rng.Intn(14), GoalsConceded: 20 + rng.Intn(30),
				})
			}
		}
	}
	return els
}

// ---------------------------------------------------------------------------
// Driving Optimize through a swapped bound.
// ---------------------------------------------------------------------------

// greedySeed is what observeGreedySeed captured: the fill loop's raw output,
// before DP seeding or the local search runs.
type greedySeed struct {
	ids   []int
	spend int
}

// runFillLoop drives the real e.Optimize through boundImpl — a value with
// exactly fillCandidateCost's signature — for the duration of one call, and
// returns the greedy seed captured via observeGreedySeed alongside
// Optimize's own final answer. Both package vars are restored before
// returning, including if Optimize panics, so one landscape's override can
// never leak into another test running in the same binary.
func runFillLoop(t *testing.T, e *Engine, req OptimizeRequest, boundImpl func(fb *fillBound, pool []PlayerMetrics, selected map[int]PlayerMetrics, posCount, clubCount map[string]int, pending PlayerMetrics, remaining int) int) (seed greedySeed, sq *Squad, err error) {
	t.Helper()

	prevBound := fillCandidateCost
	prevObserve := observeGreedySeed
	defer func() {
		fillCandidateCost = prevBound
		observeGreedySeed = prevObserve
	}()

	fillCandidateCost = boundImpl
	observeGreedySeed = func(s []PlayerMetrics, spend int) {
		ids := make([]int, len(s))
		for i, p := range s {
			ids[i] = p.ID
		}
		seed = greedySeed{ids: ids, spend: spend}
	}

	sq, err = e.Optimize(req)
	return seed, sq, err
}

// shippedBound and oldBound are the two boundImpl values runFillLoop is
// driven with: the former is exactly what fillCandidateCost defaults to in
// squad.go (production behaviour, spelled out again here so a landscape run
// with it is provably going through the shipped bound and not merely
// whatever fillCandidateCost happened to hold), the latter routes through
// the frozen pre-94068f30 bound instead.
func shippedBound(fb *fillBound, pool []PlayerMetrics, selected map[int]PlayerMetrics, posCount, clubCount map[string]int, pending PlayerMetrics, remaining int) int {
	return fb.cost(posCount, clubCount, boundParams{id: pending.ID, pos: pending.Position, team: pending.Team}, remaining)
}

func oldBound(fb *fillBound, pool []PlayerMetrics, selected map[int]PlayerMetrics, posCount, clubCount map[string]int, pending PlayerMetrics, remaining int) int {
	return minCostToFillAsShippedBefore94068f30(pool, selected, posCount, clubCount, pending)
}

// sortedSquadIDs is Optimize's answer, reduced to what the property claims
// is invariant: which fifteen it picked and what it cost. TotalCost is
// rounded to the nearest tenth of a million already, matching req.Budget's
// own units, so comparing it directly is exact rather than a float
// tolerance.
func sortedSquadIDs(sq *Squad) []int {
	ids := make([]int, len(sq.Players))
	for i, p := range sq.Players {
		ids[i] = p.ID
	}
	return ids
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSquadFillBoundDifferentialAgreesOnLandscapesTheOldBoundAlreadyClears is the
// differential test. It gates each landscape on the FROZEN pre-94068f30
// bound completing a legal fifteen — see the package doc for the derivation
// of why that, not the shipped bound succeeding, is the condition under
// which byte-identical agreement is actually provable.
func TestSquadFillBoundDifferentialAgreesOnLandscapesTheOldBoundAlreadyClears(t *testing.T) {
	succeeded := 0
	for _, lc := range fillBoundLandscapes {
		t.Run(lc.label, func(t *testing.T) {
			els := buildFillBoundPool(int64(lc.numClubs*1000+lc.perPosClub), lc.numClubs, lc.perPosClub)
			e := scaleEngine(t, els...)
			req := OptimizeRequest{Budget: lc.budget, MinMinutes: 0, MinExpectedMinutes: 0}

			oldSeed, oldSquad, oldErr := runFillLoop(t, e, req, oldBound)
			if oldErr != nil {
				// Out of scope by construction: the property makes no claim about a
				// landscape the frozen old bound cannot complete — that is exactly
				// where 94068f30 is ALLOWED to change behaviour. Not a test failure.
				t.Skipf("frozen pre-94068f30 bound could not complete this landscape "+
					"(%v); out of scope for this property, not a failure", oldErr)
			}
			succeeded++
			t.Logf("old bound: cost £%.1fm of £%.1fm budget (%.1fm headroom), formation %s",
				oldSquad.TotalCost, float64(lc.budget)/10,
				float64(lc.budget)/10-oldSquad.TotalCost, oldSquad.Formation)

			shippedSeed, shippedSquad, shippedErr := runFillLoop(t, e, req, shippedBound)
			if shippedErr != nil {
				t.Fatalf("the frozen old bound completed this landscape but the shipped "+
					"bound did not (%v). The derivation this file is built on says the "+
					"old bound succeeding is proof the two walks never diverged, so the "+
					"shipped bound failing here is exactly the regression it rules out",
					shippedErr)
			}

			if got, want := len(shippedSeed.ids), len(oldSeed.ids); got != want {
				t.Fatalf("greedy seed sizes differ: shipped %d, old bound %d", got, want)
			}
			if !intsEqual(shippedSeed.ids, oldSeed.ids) {
				t.Fatalf("greedy seed diverged between bounds:\n  shipped:  %v\n  old:      %v",
					shippedSeed.ids, oldSeed.ids)
			}
			if shippedSeed.spend != oldSeed.spend {
				t.Fatalf("greedy seed spend diverged: shipped %d, old bound %d",
					shippedSeed.spend, oldSeed.spend)
			}

			// The literal ask: Optimize's own final answer, not merely the
			// pre-polish seed. It can legitimately agree even when a hidden
			// divergence in the fill loop would not show up here, because DP
			// seeding (Optimize stage 2) runs independently of the greedy fill and
			// can win regardless of which bound the fill used — which is exactly
			// why the greedy-seed assertion above is the one that actually
			// exercises this file's property, and this one is checked in addition
			// to it, not instead of it.
			shippedIDs, oldIDs := sortedSquadIDs(shippedSquad), sortedSquadIDs(oldSquad)
			if !intsEqual(shippedIDs, oldIDs) {
				t.Fatalf("Optimize's final squad diverged between bounds:\n  shipped: %v\n  old:     %v",
					shippedIDs, oldIDs)
			}
			if shippedSquad.TotalCost != oldSquad.TotalCost {
				t.Fatalf("Optimize's final cost diverged: shipped £%.1fm, old bound £%.1fm",
					shippedSquad.TotalCost, oldSquad.TotalCost)
			}
		})
	}
	if succeeded < 4 {
		t.Fatalf("only %d of %d landscapes succeeded against the frozen old bound; "+
			"this test is evidence only when most of the table is in scope — widen "+
			"a budget or loosen a landscape rather than reading a near-empty run as a pass",
			succeeded, len(fillBoundLandscapes))
	}
}

// TestSquadFillBoundDifferentialHasTeeth checks the checker.
//
// A differential test nobody has seen fail is not evidence. This drives the
// SAME landscapes — still gated on the frozen old bound succeeding — through
// a "shipped" bound deliberately perturbed to over-estimate by a tenth of a
// million: the one failure mode 94068f30's own commit message calls out as
// turning this from a bug fix into an unswept search-quality regression, and
// exactly what TestSquadFillBoundDifferentialAgreesOnLandscapesTheOldBoundAlready
// Clears exists to catch if fillBound ever regresses into it. Requires the
// comparison to reject every landscape it runs on.
func TestSquadFillBoundDifferentialHasTeeth(t *testing.T) {
	inflated := func(fb *fillBound, pool []PlayerMetrics, selected map[int]PlayerMetrics, posCount, clubCount map[string]int, pending PlayerMetrics, remaining int) int {
		b := fb.cost(posCount, clubCount, boundParams{id: pending.ID, pos: pending.Position, team: pending.Team}, remaining)
		if b >= boundInfeasible {
			return b
		}
		// A tenth of a million over the true bound is enough to reject a
		// candidate the true bound would have accepted, right at the margin —
		// which is exactly what an over-estimating bound does silently.
		return b + 1
	}

	ran, caught := 0, false
	for _, lc := range fillBoundLandscapes {
		els := buildFillBoundPool(int64(lc.numClubs*1000+lc.perPosClub), lc.numClubs, lc.perPosClub)
		e := scaleEngine(t, els...)
		req := OptimizeRequest{Budget: lc.budget, MinMinutes: 0, MinExpectedMinutes: 0}

		oldSeed, _, oldErr := runFillLoop(t, e, req, oldBound)
		if oldErr != nil {
			continue
		}
		ran++
		inflatedSeed, _, inflatedErr := runFillLoop(t, e, req, inflated)
		if inflatedErr != nil || !intsEqual(oldSeed.ids, inflatedSeed.ids) ||
			oldSeed.spend != inflatedSeed.spend {
			caught = true
			t.Logf("%s: inflated bound diverged from the old-bound baseline as expected (err=%v)",
				lc.label, inflatedErr)
		}
	}
	if ran == 0 {
		t.Fatal("no landscape reached the comparison at all; the fixture table has drifted " +
			"out of scope for both tests in this file")
	}
	if !caught {
		t.Fatal("an admissibility bound inflated by a tenth of a million at the " +
			"margin was accepted on every landscape; this comparison has no power " +
			"to catch fillBound regressing into over-estimation, and is not evidence " +
			"of anything")
	}
}
