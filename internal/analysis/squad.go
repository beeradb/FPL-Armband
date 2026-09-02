package analysis

import (
	"fmt"
	"os"
	"slices"
	"time"
)

// The optimiser's sorts run on PlayerMetrics, which is 592 bytes, and they run
// inside the objective — so they are evaluated tens of millions of times in one
// Optimize call. sort.SliceStable reaches them through reflect.Swapper, which
// allocates a scratch element per sort and moves those 592 bytes through an
// indirect call; slices.SortStableFunc is generic and assigns directly.
//
// # Why this cannot change an answer
//
// Every comparator below is derived from its predecessor mechanically: cmp(a,b)
// is -1 where the old less(a,b) held, +1 where less(b,a) held, and 0 otherwise.
// A stable sort's output permutation is uniquely determined by that ordering, so
// the two produce identical slices — including under ties, which is the part
// that matters. Tie order is load-bearing here: bestXIWith sorts each position
// by score and slotProbabilities convolves the resulting eleven in order, so two
// players tied on Score but differing in minutes are not interchangeable. See
// TestStableTieOrderFollowsInputOrder.
//
// Written out longhand rather than via cmp.Compare on purpose. cmp.Compare
// orders NaN below everything, where `a.Score > b.Score` treats a NaN score as
// tied with all comers and leaves the stable sort to keep input order. Those
// differ, and only the second is what shipped.
//
// The *unstable* sort.Slice calls in funding.go and swaps.go are deliberately
// left alone: there the output permutation is not determined by the ordering
// alone, so swapping the algorithm could legitimately reorder ties.
func byScoreDesc(a, b PlayerMetrics) int {
	if a.Score > b.Score {
		return -1
	}
	if b.Score > a.Score {
		return 1
	}
	return 0
}

// permuteByKeys stably orders vals by the parallel key slices, exactly as the
// old stable value sort with the same comparator did: keyMust[i] true sorts
// first, within equal keyMust a larger keyScore sorts first, and full ties
// keep input order — the stable index sort reproduces that, because tied
// indices stay in input order and ties are all the stability ever carried.
// The values are then permuted in place by cycle-walking, each moved once.
// perm is grow-only scratch, rebuilt on every call.
//
// # Why sort indices at all
//
// The objective's sorts ran over PlayerMetrics, which is 592 bytes, with
// comparators doing map lookups per comparison; a CPU profile of the
// optimiser put the sort core at ~37% of the run flat. Indices are eight
// bytes, the comparator reads two parallel scalar arrays, and the final
// permutation moves each value exactly once — the same total order, the same
// stable ties, none of the memmove churn. keyMust may be nil for a pure
// score sort.
func permuteByKeys[T any](vals []T, keyMust []bool, keyScore []float64, perm []int) []int {
	n := len(vals)
	perm = perm[:0]
	for i := 0; i < n; i++ {
		perm = append(perm, i)
	}
	slices.SortStableFunc(perm, func(i, j int) int {
		if keyMust != nil {
			if keyMust[i] != keyMust[j] {
				if keyMust[i] {
					return -1
				}
				return 1
			}
		}
		if keyScore[i] > keyScore[j] {
			return -1
		}
		if keyScore[j] > keyScore[i] {
			return 1
		}
		return 0
	})
	// Apply in place: vals'[k] = vals[perm[k]], walking each cycle once. -1
	// marks a visited slot; the permutation is rebuilt from scratch next call.
	for start := 0; start < n; start++ {
		if perm[start] < 0 {
			continue
		}
		k, cur := start, vals[start]
		for {
			next := perm[k]
			if next == start {
				vals[k] = cur
				perm[k] = -1
				break
			}
			vals[k] = vals[next]
			perm[k] = -1
			k = next
		}
	}
	return perm
}

// foldPair processes a prefix record's (captain, vice) pair through the same
// sequential update, in that order. A sequence's record folds into any prior
// state exactly as replaying the sequence would: each update step is a pure
// function of (captain, vice, x) — captain becomes max(captain, x), vice
// becomes max(vice, min(old captain, x)) — and the record is the sequential
// fold from zero. The equivalence is pinned by
// TestThePairFoldMatchesSequentialPlay over hundreds of thousands of tied
// random sequences.
// promote is the armband's running top-two update, and it is the ONE
// implementation of it.
//
// The eleven's value counts its best scorer twice — once in the sum, once as the
// captain — plus ViceCaptainWeight times the runner-up, so every place that
// values an eleven has to keep a running (captain, vice) pair. That update was
// written out four times: here via foldPair, in xiValueShrunk, in
// xiValueOfParts, and in bestFormation's prefix-record builder. It is pure
// comparison and assignment with no arithmetic in it, so extracting it cannot
// move a bit — which is exactly why there was never a reason for four copies.
//
// The update is NOT a plain max/second-max: an x equal to the current captain
// does not promote, so ties keep the incumbent. That is what makes the ordering
// stable, and it is why this is a named function rather than a sort.
func promote(c, v, x float64) (float64, float64) {
	if x > c {
		return x, c
	}
	if x > v {
		return c, x
	}
	return c, v
}

func foldPair(c, v, p, q float64) (float64, float64) {
	c, v = promote(c, v, p)
	c, v = promote(c, v, q)
	return c, v
}

// Squad selection constraints, matching FPL's rules.
const (
	SquadSize     = 15
	MaxPerClub    = 3
	DefaultBudget = 1000 // tenths of a million, i.e. £100.0m
)

// The shipped pool floors, which every squad-building caller must pass.
//
// ⚠️ **ZERO IS NOT "USE THE DEFAULT", IT IS "NO FLOOR AT ALL".** `Optimize`
// applies nothing implicitly: `clearsMinutesFloor` returns true for everybody at
// `minMinutes == 0`, and `cutByExpectedMinutes` returns early without cutting at
// `minExpected <= 0`. So a caller that omits these does not get the shipped
// behaviour — it gets a laxer optimiser that will offer rotation risks and
// injured returnees no other surface in this project would show.
//
// They exist as constants because the literals were repeated at four call sites
// and a fifth was written omitting them entirely, which is how
// `armband drift` came to compare a real squad against an optimum built with no
// floor. This is the same family as the recorded bug where a minutes floor had
// THREE copies and the fix reached one of them.
//
// ⚠️ **`PoolMinMinutes` is a SEASON TOTAL and must never be compared raw.**
// `Optimize` scales it per club through `ScaledMinMinutesFor`, which is why the
// number here is 600 rather than a per-gameweek figure. A caller comparing 600
// against fresh-season aggregates directly is the exact defect that emptied the
// transfer pool for seven gameweeks.
const (
	PoolMinMinutes         = 600
	PoolMinExpectedMinutes = 55
)

var squadQuota = map[string]int{"GKP": 2, "DEF": 5, "MID": 5, "FWD": 3}

// XI formation bounds: exactly 1 keeper, 11 outfield slots total.
var xiMin = map[string]int{"GKP": 1, "DEF": 3, "MID": 2, "FWD": 1}
var xiMax = map[string]int{"GKP": 1, "DEF": 5, "MID": 5, "FWD": 3}

// XISize is how many players FPL fields.
const XISize = 11

// PositionForElementType maps FPL's numeric element_type to the position short
// name the rest of this package keys on.
func PositionForElementType(t int) string {
	switch t {
	case 1:
		return "GKP"
	case 2:
		return "DEF"
	case 3:
		return "MID"
	case 4:
		return "FWD"
	}
	return ""
}

// LegalFormation reports whether an eleven with these position counts is one
// FPL would allow: exactly one keeper, three to five defenders, two to five
// midfielders, one to three forwards, eleven in total.
//
// Exported because the *replay* needs it too. FPL only makes an autosub when
// the resulting eleven is still legal — a blanking defender in a
// three-at-the-back side is simply not replaced if every bench outfielder
// would take it to two — and the backtest scored autosubs without that check.
// Sharing this rather than restating the bounds in package backtest keeps the
// two from drifting, which this codebase has been bitten by before.
func LegalFormation(byPosition map[string]int) bool {
	total := 0
	for pos, min := range xiMin {
		n := byPosition[pos]
		if n < min || n > xiMax[pos] {
			return false
		}
		total += n
	}
	return total == XISize
}

// ViceCaptainWeight is the chance the captain records no minutes and the armband
// passes to the vice-captain.
//
// It is a probability, not a preference. A nailed captain blanks through injury,
// suspension or a late rotation call perhaps one week in twelve, which is where
// 0.08 comes from. Zero reproduces the old captain-only behaviour exactly.
//
// The term is small by construction — about 0.4 on an eleven worth 55 — and it
// only changes anything when two squads are close and one has a better
// second-best. That is precisely the comparison it exists to fix.
//
// # It is correct and, so far, inert
//
// Adding it left three replayed seasons byte-identical: 2131/2288/2103 before
// and after. It is computed — two elevens with the same total and the same best
// differ by exactly ViceCaptainWeight times the gap in their second-best — but
// it never reorders a decision, because a squad's second-best is highly
// correlated with its total, so every candidate shifts by a similar amount.
//
// Kept anyway, on the grounds that it is a rules-correctness fix rather than a
// search heuristic: FPL really does double the vice-captain, and the objective
// should say what the game pays. That is a different case from the heuristics
// this codebase has reverted for measuring zero, which added complexity for no
// gain rather than removing an inaccuracy.
//
// 0.08 is a guess at how often a nailed captain records no minutes. It is
// measurable from the archive and has not been measured, because calibrating a
// term that changes no decision is polishing. If it ever starts mattering,
// measure it before tuning it.
const ViceCaptainWeight = 0.08

// DefaultBenchWeight is what a bench slot is worth, as a fraction of the
// player's score, when building a fifteen.
//
// It was 0.02 — near enough zero, on the reasoning that FPL pays nothing for a
// player who does not appear. That is true of a player who never appears and
// false of a bench, because autosubs, rotation and injuries mean bench players
// do appear: across the 2024-25 replay the four bench slots returned 419 real
// points.
//
// The misspecification was inert while the local search was too weak to exploit
// it. It stopped being inert when the multi-downgrade funding phase landed:
// given a correct search, "bench quality is nearly free to sell" is an
// instruction, and it was followed — a 419-point bench traded down to 75 to buy
// 0.38 of modelled XI, costing 116 real points on the season.
//
// Swept over three replayed seasons, opening squad only:
//
//	0.02  2169     0.10  2197     0.20  2146
//	0.05  2197     0.13  2192     0.30  2129
//	0.08  2197     0.15  2159
//
// A plateau from 0.05 to 0.13 and a cliff after it. 0.10 is the middle. Two
// caveats: only 2024-25 moves at all across that plateau, so this is n=1 in
// practice, and 2197 is still marginally below the 2208 the *broken* optimiser
// scored — inside season-to-season noise, but not a win to claim.
//
// # A constant is the wrong shape for this
//
// Score already contains the player's own MinutesRating, so this factor is not
// modelling whether he plays for his club. It is modelling P(he is needed),
// which is a property of the eleven rather than of him: a bench keeper is
// needed only if one specific player misses, a bench outfielder can cover
// several slots, and an eleven of nailed starters needs its bench far less
// often than a fragile one. None of that can be expressed by one number, which
// is probably why the curve is a plateau with a cliff rather than a real
// optimum. The principled version scales each bench slot by the aggregate
// blank-probability of the XI slots it could cover, which is computable from
// ExpectedMinutes.
//
// # It is the default for Weights.BenchWeight too, and that had drifted
//
// Weights.BenchWeight is what Optimize falls back to when a caller passes zero,
// and it shipped at 0.15 — one step past the cliff, and the second-worst value
// in the sweep above. Every caller that named a weight was fine; optimize_squad
// passes whatever the agent supplies, so an agent that omitted bench_weight got
// 0.15 while the replay that measured the constant used 0.10. Two constants for
// one quantity is how that happens, so there is now one.
const DefaultBenchWeight = 0.10

// OptimizeRequest describes a squad-building problem.
type OptimizeRequest struct {
	// Budget in tenths of a million (1000 = £100.0m).
	Budget int `json:"budget_tenths"`
	// LockIDs must appear in the final squad.
	LockIDs []int `json:"lock_player_ids"`
	// StartIDs must appear in the starting eleven, not merely the squad.
	// Implies LockIDs.
	StartIDs []int `json:"start_player_ids"`
	// ExcludeIDs must not appear.
	ExcludeIDs []int `json:"exclude_player_ids"`
	// CurrentSquad is the fifteen you already own, by element id. With
	// MaxChanges it turns squad construction into squad *revision*, which is the
	// in-season problem: the same objective and the same search, bounded by how
	// many transfers you are willing to spend.
	//
	// Leave it empty to build from nothing, which is the pre-season and wildcard
	// case.
	CurrentSquad []int `json:"current_squad_ids,omitempty"`
	// MaxChanges caps how many of CurrentSquad's players may be replaced. Zero
	// means no limit. It counts the difference between the answer and
	// CurrentSquad, not the moves taken to get there, so the search cannot spend
	// the budget selling a player and buying him back.
	MaxChanges int `json:"max_changes,omitempty"`
	// MinMinutes filters out players below this many total minutes played,
	// expressed as a full-season figure. It is scaled to however many matches
	// the data actually covers, so a floor of 600 means 600 pre-season and about
	// 47 after three gameweeks. Unscaled it would exclude every player in the
	// game until roughly GW26 and the optimiser would fail outright.
	MinMinutes int `json:"min_minutes"`
	// MinExpectedMinutes filters out players averaging fewer than this many
	// minutes per gameweek — the direct rotation-risk control. Around 60 keeps
	// only likely starters; 75 keeps only nailed ones.
	MinExpectedMinutes float64 `json:"min_expected_minutes"`

	// PriceOverride replaces a player's price for this request only, in tenths
	// of a million, keyed by element id. It answers "what would I build if this
	// player cost that tomorrow" — a price change the model cannot foresee but
	// the analysis layer can read off transfer traffic or the press.
	//
	// # Why it is on the request and not the engine
	//
	// The tool runner fans calls out through an errgroup, so setting engine
	// state for one call and restoring it afterwards corrupts every other search
	// in the same turn. That hazard is already documented for skipped gameweeks
	// and is the reason this is per-request: it is applied to a local copy of
	// the pool and touches nothing shared.
	//
	// # Why prices deserve an override when most quantities do not
	//
	// The two directions are not symmetric, and only one of them matters. A rise
	// on a player you *own* returns half when you sell, so it barely changes
	// what you can do. A rise on one you *want* costs the full amount, and a
	// fall on one you own comes off your buying power in full.
	//
	// TestDiagBudgetJitter measures the consequence: taking money away reshapes
	// a squad far more often than adding it, because the optimiser already
	// spends everything it has. So the useful question is rarely "what will my
	// team be worth" and almost always "what will I no longer be able to
	// afford" — which needs per-player prices, not a budget total.
	PriceOverride map[int]int
	// BenchMinExpectedMinutes applies a looser floor to bench slots, so cheap
	// non-playing fodder is still allowed where it belongs.
	BenchMinExpectedMinutes float64 `json:"bench_min_expected_minutes"`
	// IncludeUnavailable keeps injured/suspended players in the candidate pool.
	IncludeUnavailable bool `json:"include_unavailable"`
	// BenchWeight overrides the engine default when > 0.
	BenchWeight float64 `json:"bench_weight"`

	// BenchBoost builds the fifteen for a week the bench boost chip is played,
	// where FPL pays all fifteen rather than the eleven plus autosubs. The
	// objective then counts every bench player at full value and ignores both
	// BenchWeight and the slot probabilities, which exist only to price how
	// often a substitute is actually used.
	//
	// # Why this is a squad-building option and not a scoring one
	//
	// The chip is worth what the bench is worth, and the bench is worth almost
	// nothing in a squad built under the ordinary objective — `XIValue` credits
	// bench players at zero, so the fifteen converges on eleven good players and
	// four who cannot cover. Measuring the chip on that squad measures a floor.
	//
	// The play it exists for is the standard one: wildcard into a squad with a
	// real bench, boost it the following week, then transfer the surplus bench
	// value back out. That is a *different squad*, reachable only if the builder
	// knows the chip is coming — which is exactly what this flag tells it. The
	// free hit is the same shape for a blank gameweek, and the wildcard is what
	// makes the whole sequence affordable.
	//
	// # Not the same thing as SuggestBenchWeight, and both are wanted
	//
	// `SuggestBenchWeight` already raises the bench weight for a planned bench
	// boost, amortised over the horizon: `base + (1-base)/horizon`. That is the
	// right answer to "my squad must serve five gameweeks and one of them is the
	// boost" — the bench scores in full once and is a hedge the other four
	// times, so the credit is spread.
	//
	// This flag answers the different question "build me the squad I am going to
	// boost", where the boost week is the *point* of the build rather than one
	// week in five. Setting it makes benchValue ignore BenchWeight entirely, so
	// the two cannot fight: if both are set, the boost wins, which is correct
	// because it is the more specific instruction.
	BenchBoost bool `json:"bench_boost,omitempty"`
}

// Squad is a solved 15-man squad.
type Squad struct {
	Players     []PlayerMetrics `json:"squad"`
	StartingXI  []PlayerMetrics `json:"starting_xi"`
	Bench       []PlayerMetrics `json:"bench"`
	Formation   string          `json:"formation"`
	Captain     PlayerMetrics   `json:"captain"`
	ViceCaptain PlayerMetrics   `json:"vice_captain"`

	TotalCost float64 `json:"total_cost_m"`
	Remaining float64 `json:"budget_remaining_m"`
	// XIScore is the plain sum of the eleven.
	XIScore float64 `json:"starting_xi_score"`
	// ExpectedPoints is XIScore plus the captain's score again — what the team
	// is actually expected to return, since the armband doubles one player.
	ExpectedPoints float64        `json:"expected_points_with_captain"`
	SquadScore     float64        `json:"squad_score"`
	ClubCounts     map[string]int `json:"club_counts"`
}

// Optimize builds the highest-scoring legal 15-man squad it can find.
//
// This is a multi-dimensional knapsack (budget, positional quotas, club limits)
// with a rugged objective, because the value of a player depends on which ten
// others are around him. The search is therefore in three stages:
//
//  1. a greedy value-per-million seed, polished by steepest-ascent swaps;
//  2. an exact dynamic-programming solve of the starting eleven for every legal
//     formation, giving a second set of starting points (see dpseed.go);
//  3. a local search from the best of those, keeping whichever result wins.
//
// Stage 2 exists because stages 1 and 3 alone are not enough. A local search
// only reaches what is uphill from where it starts, and restructuring a squad —
// dropping a £6.0m goalkeeper to fund a £15.5m striker — is downhill at every
// individual step. TestOptimizerIsNeverWorseThanAnExactSeed holds the line.
//
// ObserveOptimize, if set, is handed this call's wall-clock duration when it
// returns. Nil by default and left that way by every caller except cmd/armband's
// serve, which is the one process this is expensive enough to page on — see
// cmd/armband/instruments.go. A package-level var rather than a field on Engine:
// internal/analysis stays free of a metrics dependency (this package computes,
// it does not observe itself), and weekview.go's engineAtHorizon builds fresh
// Engines by hand-copying named fields, so a per-Engine hook field would
// silently fail to reach them while a package-level var needs no propagation
// at all.
var ObserveOptimize func(time.Duration)

// fillCandidateCost is the greedy fill loop's one point of indirection over
// the admissibility bound it consults before committing to a candidate — see
// fillBound's doc for what the shipped bound does and why. Its default body
// is exactly the expression the loop used to call directly; production code
// never reassigns it. It exists purely as a seam for
// TestSquadFillBoundDifferentialAgreesOnLandscapesTheOldBoundAlreadyClears
// (squadfillbounddiff_test.go), which drives this REAL Optimize call
// through minCostToFillAsShippedBefore94068f30 — the bound fillBound
// replaced — to check that the replacement changed nothing on any landscape
// the old bound already completed, without a second copy of this loop.
var fillCandidateCost = func(fb *fillBound, pool []PlayerMetrics, selected map[int]PlayerMetrics, posCount, clubCount map[string]int, pending PlayerMetrics, remaining int) int {
	return fb.cost(posCount, clubCount, boundParams{id: pending.ID, pos: pending.Position, team: pending.Team}, remaining)
}

// observeGreedySeed, if set, is handed the greedy fill's raw output — the
// squad selected before DP seeding or the local search runs — and its spend
// in tenths of a million. Nil in production. Exists so
// squadfillbounddiff_test.go can inspect the greedy seed in isolation: the
// DP seeds Optimize builds afterwards (see stage 2 on Optimize's own doc)
// are constructed independently of the greedy fill and can win regardless of
// which bound the fill used, which would hide a divergence in the fill loop
// itself behind an unaffected final answer.
var observeGreedySeed func(seed []PlayerMetrics, spend int)

func (e *Engine) Optimize(req OptimizeRequest) (*Squad, error) {
	if ObserveOptimize != nil {
		start := time.Now()
		defer func() { ObserveOptimize(time.Since(start)) }()
	}
	budget := req.Budget
	if budget <= 0 {
		budget = DefaultBudget
	}
	benchWeight := e.resolveBenchWeight(req)
	// Under bench boost every one of the fifteen scores, so benchWeight and the
	// slot probabilities are both bypassed entirely — see benchValue.
	boost := req.BenchBoost

	excluded := map[int]bool{}
	for _, id := range req.ExcludeIDs {
		excluded[id] = true
	}
	// Forced starters are a stricter form of lock: in the squad *and* in the
	// eleven. Every candidate squad has to be scored on the eleven it will
	// actually field, or the search optimises for one selection and the final
	// pick produces another.
	mustStart := mustStartSet(req)

	// Fold them into LockIDs once, here, rather than teaching each of the four
	// places that read it about a second list. A parallel list is how a forced
	// starter ends up pre-placed in the greedy squad and absent from the DP
	// seeds, which rejects every seed and quietly drops him from the answer
	// altogether — worse than not forcing him at all.
	locked := map[int]bool{}
	for _, id := range req.LockIDs {
		locked[id] = true
	}
	for _, id := range req.StartIDs {
		if !locked[id] {
			locked[id] = true
			req.LockIDs = append(req.LockIDs, id)
		}
	}

	// Scale the minutes floor to the data window: it is written as a season
	// total, but early in a season the aggregates only cover a few matches.
	//
	// Per CLUB, not once for the whole pool. During the live GW1 gap — the
	// season kicked off, no gameweek finished yet — two clubs are not on the
	// same clock, so there is no single window that is true of everybody. See
	// Engine.minutesFloorWindow. Memoised rather than resolved per candidate
	// because the window walks the fixture list: twenty lookups instead of six
	// hundred, for the same answer.
	//
	// Memoised ON DEMAND, and not pre-filled from `e.Boot.Teams`, which is what
	// this was first written as. A pre-filled map answers a club it has never
	// heard of with Go's zero value, and a floor of zero is not a conservative
	// default — `clearsMinutesFloor` reads it as "any minutes at all clears
	// this", so the screen would be silently OFF for that club's whole squad.
	// That is the same failure direction as the bug this function exists to fix,
	// arriving through the repair. `Boot.Teams` and `Boot.Elements` come from one
	// payload so the miss should be unreachable, which is exactly why it would
	// not be noticed.
	minMinutes := map[int]int{}
	floorFor := func(teamID int) int {
		if v, ok := minMinutes[teamID]; ok {
			return v
		}
		v := e.scaledMinMinutesFor(teamID, req)
		minMinutes[teamID] = v
		return v
	}

	// Build the candidate pool.
	all := e.AllMetrics()
	// Selection inside the model's own noise band, before anything reads a Score.
	// Applied here rather than at any of the individual sorts so the ordering and
	// the objective can never disagree: every consumer below — the greedy fill,
	// the DP seeds, polish, bestXIWith — sees one set of numbers. The nudge comes
	// back out of the reported squad at the bottom of this function; see
	// tiebreak.go for why it must.
	tieNudges := applyTiebreak(all, e.Tiebreak)
	byID := map[int]PlayerMetrics{}
	for _, m := range all {
		byID[m.ID] = m
	}

	// clearsMinutesFloor is the total-minutes half of the same idea, and is a
	// free function so its two exemptions can be table-tested without building a
	// legal fifteen. See TestCheapBodiesAndCorrectedPlayersClearTheMinutesFloor.
	//
	// Bench fodder is exempt from the expected-minutes floor: a £4.0m reserve
	// who never plays is exactly what belongs in slots 12-15, and excluding him
	// would force real money onto the bench. The expected-minutes half is
	// cutByExpectedMinutes, for the reason given there.
	fodderPrice := resolvedFodderPrice(req)

	// Per-request price changes, applied to this pool only. byID is rebuilt from
	// the same values so the locked-player lookup and the pool agree.
	if len(req.PriceOverride) > 0 {
		for i := range all {
			if v, ok := req.PriceOverride[all[i].ID]; ok && v > 0 {
				all[i].Price = float64(v) / 10
				all[i].ValueScore = 0
				if all[i].Price > 0 {
					all[i].ValueScore = all[i].Score / all[i].Price
				}
				byID[all[i].ID] = all[i]
			}
		}
	}

	// Element ids whose minutes the judgement layer has corrected. The override
	// is keyed by permanent player code — element ids are reassigned every
	// summer — so it has to be mapped through the bootstrap to be usable here.
	overridden := map[int]bool{}
	if e.hasMinutesOverrides() && e.Boot != nil {
		for i := range e.Boot.Elements {
			el := &e.Boot.Elements[i]
			if _, _, _, ok := e.minutesOverrideFor(el.Code); ok {
				overridden[el.ID] = true
			}
		}
	}

	var pool []PlayerMetrics
	for _, m := range all {
		if excluded[m.ID] {
			continue
		}
		if locked[m.ID] {
			pool = append(pool, m)
			continue
		}
		// The total-minutes floor is a sample-size test: too little football
		// behind him and his rates are not believable. Two populations are
		// exempt, for exactly the reasons the expected-minutes cliff below
		// already exempts them — this check used to have neither exemption, and
		// running first it made both of the cliff's unreachable.
		//
		// Cheap bodies, because the squad rules force you to own fifteen players
		// and roughly four of those are compliance slots. A £4.0m reserve keeper
		// who never plays is what belongs there, and excluding him puts real
		// money on the bench. That is not hypothetical: with no £4.0m keeper in
		// the pool the optimiser had to buy two at £4.5m, and the spare £0.5m
		// came out of the eleven.
		//
		// And players the layer has corrected, because an override is a
		// statement about a role the record does not show — precisely the case
		// where the raw season total is uninformative. `blend.go` already
		// promises this: it says the score, "whether he clears the minutes
		// floor", and whether the optimiser wants him are all recomputed from
		// the override rather than dictated. The floor was reading `el.Minutes`,
		// which the override never touches, so two deliberately corrected
		// Coventry defenders were scored, reported to the agent, and then
		// silently unbuyable — the optimiser took a 0.48 player at the same
		// £4.0m instead. `swaps.go` has no floor at all, so the transfer search
		// would have offered them the following week, which is the recorded
		// "an override the transfer search ignores is worse than no override"
		// failure with the two solvers swapped.
		if !reachesExpectedMinutesCut(m, floorFor(m.TeamID), fodderPrice,
			overridden[m.ID], req.IncludeUnavailable) {
			continue
		}
		if cutByExpectedMinutes(m, req.MinExpectedMinutes, fodderPrice,
			req.BenchMinExpectedMinutes) {
			continue
		}
		pool = append(pool, m)
	}

	// Reject a request no search can answer, before any seeding runs. This is
	// the earliest point where both byID and the folded req.LockIDs are valid:
	// StartIDs is folded into LockIDs at the top of this function, so the
	// forced starters are covered by the existence half as well.
	if err := checkLocksAndForcedStarts(req, mustStart, byID); err != nil {
		return nil, err
	}

	// Seed: take locked players first, then fill each position greedily by
	// score-per-million so the budget stretches across all 15 slots.
	selected := map[int]PlayerMetrics{}
	posCount := map[string]int{}
	clubCount := map[string]int{}
	spend := 0

	// fb is the two-tier admissible bound's scratch — see its doc for what it
	// holds and why. Built once from pool, before anything is added, so its
	// static structure (price order per position and per club) is ready for
	// the locked-player adds below as well as the greedy fill.
	fb := buildFillBound(pool)

	add := func(m PlayerMetrics) {
		selected[m.ID] = m
		posCount[m.Position]++
		clubCount[m.Team]++
		spend += int(m.Price*10 + 0.5)
		// A locked player can be absent from pool — locks validate against
		// byID, not the filtered pool — in which case there is nothing in fb
		// to mark: he was never a bound candidate to begin with.
		if idx, ok := fb.poolIndexByID[m.ID]; ok {
			fb.picked[idx] = true
		}
	}

	for _, id := range req.LockIDs {
		m := byID[id]
		if posCount[m.Position] >= squadQuota[m.Position] {
			return nil, fmt.Errorf("locked players exceed the quota for %s", m.Position)
		}
		if clubCount[m.Team] >= MaxPerClub {
			return nil, fmt.Errorf("locked players exceed the %d-player limit for %s", MaxPerClub, m.Team)
		}
		add(m)
	}
	if spend > budget {
		return nil, fmt.Errorf("locked players cost £%.1fm, over the £%.1fm budget",
			float64(spend)/10, float64(budget)/10)
	}

	// Up-front feasibility: the same admissible bound at k=0 (nothing
	// pending), so "can a legal fifteen be completed at all" is answered by
	// the identical machinery the fill loop below trusts, rather than a
	// second implementation of the question. See fillBound.exact.
	if cost := fb.exact(posCount, clubCount, boundParams{id: -1}); cost >= boundInfeasible {
		clubCap := fb.numClubs * MaxPerClub
		if clubCap < SquadSize {
			plural := "s"
			if fb.numClubs == 1 {
				plural = ""
			}
			return nil, fmt.Errorf("could not fill a legal 15-man squad: only %d club%s "+
				"in the pool, so at most %d squad slots are available — relax min_minutes "+
				"or the exclude list", fb.numClubs, plural, clubCap)
		}
		return nil, fmt.Errorf("could not fill a legal 15-man squad: no combination of "+
			"available players satisfies both the position quotas and the %d-per-club "+
			"limit — relax min_minutes or the exclude list", MaxPerClub)
	} else if cost > budget-spend {
		return nil, fmt.Errorf("could not fill a legal 15-man squad: the cheapest legal "+
			"completion costs £%.1fm against £%.1fm remaining — try raising the budget",
			float64(cost)/10, float64(budget-spend)/10)
	}

	byValue := make([]PlayerMetrics, len(pool))
	copy(byValue, pool)
	slices.SortStableFunc(byValue, func(a, b PlayerMetrics) int {
		if a.ValueScore > b.ValueScore {
			return -1
		}
		if b.ValueScore > a.ValueScore {
			return 1
		}
		return 0
	})

	canAdd := func(m PlayerMetrics) bool {
		if _, in := selected[m.ID]; in {
			return false
		}
		if posCount[m.Position] >= squadQuota[m.Position] {
			return false
		}
		if !clubHeadroom(clubCount, m.Team) {
			return false
		}
		return true
	}

	// Fill greedily, reserving enough budget to complete the remaining slots
	// with the cheapest LEGAL completion — position quotas and the club cap
	// both — at each unfilled position. See fillBound.cost.
	for len(selected) < SquadSize {
		best := -1
		for i, m := range byValue {
			if !canAdd(m) {
				continue
			}
			cost := int(m.Price*10 + 0.5)
			remaining := budget - spend - cost
			if fillCandidateCost(fb, pool, selected, posCount, clubCount, m, remaining) > remaining {
				continue
			}
			best = i
			break
		}
		if best < 0 {
			return nil, fmt.Errorf("could not fill a legal 15-man squad within £%.1fm — try raising the budget or relaxing min_minutes", float64(budget)/10)
		}
		add(byValue[best])
	}

	if observeGreedySeed != nil {
		observeGreedySeed(squadSlice(selected), spend)
	}

	changes := changeBudget{Max: req.MaxChanges}
	if len(req.CurrentSquad) > 0 {
		changes.Baseline = make(map[int]bool, len(req.CurrentSquad))
		for _, id := range req.CurrentSquad {
			changes.Baseline[id] = true
		}
	}

	// A bounded revision starts from the squad you own. The greedy build is the
	// wrong starting point when only a couple of players may move: it is a
	// different fifteen, so almost every step back toward yours is spent on the
	// budget rather than on improving anything.
	//
	// Every failure below is an ERROR rather than a fall back to the greedy
	// build, and that is the whole point of this block. The greedy fifteen is
	// unbounded — it ignores MaxChanges entirely — so falling back to it answers
	// "revise my squad by at most k" with "here is a different squad", scored and
	// returned as though it were a k-change revision. That is a fallback in the
	// expensive direction: the caller acts on a plan it cannot afford in
	// transfers. A wrong length, an id the engine cannot price and a lock the
	// seed cannot hold were all swallowed by one silent `if` here.
	seedSquad := squadSlice(selected)
	if !changes.unlimited() {
		if len(req.CurrentSquad) != SquadSize {
			return nil, fmt.Errorf("current squad has %d players, want %d", len(req.CurrentSquad), SquadSize)
		}
		owned, err := ownedSquad(req.CurrentSquad, byID)
		if err != nil {
			return nil, err
		}
		seedSquad = owned

		// A lock the seed cannot hold. checkLocksAndForcedStarts has already
		// confirmed every locked id names a player the engine knows; this is the
		// stricter bounded-path question of whether he is one of the fifteen the
		// revision starts from. polish only refuses to REMOVE a lock (dpseed.go's
		// `if locked[out.ID]`) and nothing inserts one, and the DP seeds — which
		// do pre-place locked players — are skipped under a bound. So a lock
		// outside CurrentSquad is simply never bought, and the caller gets a
		// squad without him and a nil error.
		//
		// Forced starters arrive here folded into locked/req.LockIDs (see the
		// fold near the top of this function), so must_start is covered by the
		// same check — a forced starter the current squad does not own was
		// dropped from the fifteen entirely, which is worse than not forcing him.
		//
		// Deliberately NOT taken: evicting a held player to make room and
		// charging the change budget for it. That is a feature with its own
		// legality and budget reasoning, not a bug fix.
		if !holdsLocks(seedSquad, locked) {
			held := make(map[int]bool, len(seedSquad))
			for _, p := range seedSquad {
				held[p.ID] = true
			}
			// req.LockIDs order, not the map's: a request missing two locks names
			// the same one on every run.
			for _, id := range req.LockIDs {
				if !held[id] {
					return nil, fmt.Errorf("locked player id %d is not in the current squad, and a bounded revision cannot transfer him in — raise max_changes or clear him from lock_players/must_start", id)
				}
			}
		}
	}

	// The inner half of the stage timings buildSquadPage prints: that one says
	// how much of the page build is Optimize, this one says which of Optimize's
	// three stages it is. Same switch, same stderr, same reason it is kept.
	timing := os.Getenv("FPL_SERVE_TIMINGS") != ""
	lastT := time.Now()
	markT := func(l string) {
		if !timing {
			return
		}
		now := time.Now()
		fmt.Fprintf(os.Stderr, "TIMING   %-16s %6.0fms\n", l, float64(now.Sub(lastT).Milliseconds()))
		lastT = now
	}

	current, bestScore, spend := e.polish(seedSquad, pool, budget, benchWeight, boost, locked, mustStart, true, changes)
	markT("greedy polish")

	// A local search explores from wherever it starts, and one greedy start is
	// not enough — funding a premium by restructuring a different position is
	// downhill at every individual step, so steepest ascent never takes it.
	// Restart from an exact DP solution for each formation and keep the best.
	//
	// Locked players are pre-placed in the seeds rather than skipping seeding,
	// so scenario runs ("what if I keep this striker?") get the same quality of
	// search as an unconstrained one.
	lockedPlayers := make([]PlayerMetrics, 0, len(req.LockIDs))
	for _, id := range req.LockIDs {
		lockedPlayers = append(lockedPlayers, byID[id])
	}

	// Each seed is already exact for its own formation, so rank them as they
	// stand and pay for a local search only on the winner. Polishing all nine
	// costs ten times the search for no better answer.
	// DP seeds solve each formation from scratch over the whole pool, which is a
	// different problem once the answer must stay within a few players of one
	// you already own: every seed is a fresh fifteen, so all of them are further
	// away than the budget allows, and repairing one back into range is not what
	// repairClubs is for. Under a bound the squad you have is the seed, and this
	// whole phase is skipped.
	//
	// Skipping it is load-bearing rather than an optimisation: a seed that
	// ignores the bound scores far higher than any legal revision, wins the
	// ranking, and is returned — so the bound is honoured all the way through
	// the local search and then thrown away at the last step.
	var bestSeed []PlayerMetrics
	bestSeedScore := 0.0
	seeds := [][]PlayerMetrics{}
	if changes.unlimited() {
		seeds = e.dpSeeds(pool, budget, lockedPlayers)
		markT("dp seeds")
	}
	for _, seed := range seeds {
		seed = repairClubs(seed, pool, budget)
		if seed == nil || !squadIsLegal(seed, budget) || !holdsLocks(seed, locked) {
			continue
		}
		if s := objectiveWith(seed, benchWeight, mustStart, boost); s > bestSeedScore {
			bestSeed, bestSeedScore = seed, s
		}
	}
	if bestSeed != nil {
		cand, score, cost := e.polish(bestSeed, pool, budget, benchWeight, boost, locked, mustStart, true, changes)
		markT("seed polish")
		if score > bestScore+1e-9 && squadIsLegal(cand, budget) && holdsLocks(cand, locked) {
			current, bestScore, spend = cand, score, cost
		}
	}

	xi, bench, formation := bestXIWith(current, mustStart)
	// The tiebreak has chosen the fifteen; from here every number is a forecast
	// and must not carry it. Order is already fixed — bestXIWith has run — so
	// removing the nudge cannot reshuffle the eleven, and SquadScore is restated
	// from the clean scores below for the same reason XIScore is.
	removeTiebreak(current, tieNudges)
	removeTiebreak(xi, tieNudges)
	removeTiebreak(bench, tieNudges)
	sq := &Squad{
		Players:    sortForDisplay(current),
		StartingXI: xi,
		Bench:      bench,
		Formation:  formation,
		TotalCost:  float64(spend) / 10,
		Remaining:  float64(budget-spend) / 10,
		SquadScore: bestScore,
		// Tallied from the fifteen being returned, never carried forward from
		// the greedy fill's `clubCount`. That map is the SEARCH's bookkeeping:
		// `add` is its only writer, so it stops being updated the moment the
		// fill loop ends, and both stages after it — polish's steepest-ascent
		// swaps and the DP seeds — keep their own club counts and replace
		// players without touching it. Reporting it described a squad that had
		// usually stopped existing: one real call returned ARS×3, MUN×2, CHE×2
		// while the map said ARS 0, MUN 0, CHE 1 and named four clubs (LEE,
		// SUN, BOU, NEW) the squad did not hold at all. It still summed to
		// fifteen and still obeyed the three-per-club cap, which is why the
		// tests asserting only `n <= MaxPerClub` never saw it.
		//
		// This is a REPORTING field, and only ever was: the cap is enforced
		// during the search by canAdd/clubHeadroom/squadIsLegal, each of which
		// reads counts local to the stage doing the work. Two readers take
		// this map as the truth about club exposure — the agent tool JSON,
		// which is replayed into every later API call, and the web planner's
		// pitch.
		ClubCounts: clubCountsOf(current),
	}
	for _, p := range xi {
		sq.XIScore += p.Score
	}
	restateSquadScoreExTiebreak(sq, tieNudges)
	if len(xi) > 0 {
		// bestXI returns the eleven sorted by score, so the armband goes to the
		// front of it by construction.
		sq.Captain = xi[0]
		if len(xi) > 1 {
			sq.ViceCaptain = xi[1]
		}
	}
	sq.ExpectedPoints = sq.XIScore + sq.Captain.Score
	return sq, nil
}

// checkLocksAndForcedStarts rejects a request no search can answer, before any
// search runs. Two conditions, both about the request rather than the football:
//
//   - every locked id has to name a player the engine knows. StartIDs are
//     folded into LockIDs before this is called, so forced starters are covered
//     here too.
//   - the forced starters have to fit one legal formation.
//
// The second is the one this function exists for. bestFormation skips any
// formation that cannot seat `forced`, so a forced set no formation seats
// rejects EVERY formation: it returns ok=false, materialise leaves the pick
// empty, and the caller gets a squad with a zero-length eleven, all fifteen on
// the bench, an empty formation string, a zero-value captain and an objective
// quietly degenerated to benchValue over the whole squad. No error is raised
// anywhere on that path, so forcing both of your keepers to start collapsed the
// optimiser silently.
//
// ⚠️ FITTING IS A JOINT CONDITION, not a per-position one, and reading it as
// per-position is how the first fix of this bug left the collapse reachable.
// bestFormation needs ONE (d, m, f) with d+m+f == XISize-1, each inside its own
// xiMin..xiMax band and each at least its forced count. The bands are
// contiguous and xiMax sums to 13 outfield, above the ten places available, so
// such a triple exists exactly when no position is over its own xiMax AND the
// tightest eleven the forced set admits — max(xiMin, forced) summed over the
// outfield — still fits in ten. Five forced defenders beside five forced
// midfielders clears every position's own bound (both are exactly xiMax) and
// needs eleven outfield places: the aggregate is the half that catches that,
// and the per-position half is what catches two keepers, GKP being the one
// place squadQuota (2) exceeds xiMax (1).
//
// Bounded by xiMin and xiMax, the same tables bestFormation bounds itself with,
// so "how many can start here" has one implementation. Counted off mustStart —
// the forced-eleven set Optimize has already resolved and threads into the
// search — so a repeated id in StartIDs contributes once, exactly as it does in
// xiScratch.split, which walks the squad. A position posIdx does not recognise
// is skipped for that same reason: split skips those players, so they never
// reach `forced` and counting them here would reject a request the search
// handles perfectly well.
//
// Extracted from Optimize rather than left inline because the repository runs a
// complexity ratchet that only turns down — see restateSquadScoreExTiebreak,
// pulled out for the same reason. The existence loop comes with it: both halves
// check the request against byID, neither reads anything the other does not,
// and moving one branch out of Optimize is what pays for the new ones.
func checkLocksAndForcedStarts(req OptimizeRequest, mustStart map[int]bool, byID map[int]PlayerMetrics) error {
	for _, id := range req.LockIDs {
		if _, ok := byID[id]; !ok {
			return fmt.Errorf("locked player id %d not found", id)
		}
	}

	var forced [4]int
	for id := range mustStart {
		if i := posIdx(byID[id].Position); i >= 0 {
			forced[i]++
		}
	}

	// posNames order rather than the map's, so a request that overflows two
	// positions names the same one on every run.
	for i, pos := range posNames {
		if forced[i] > xiMax[pos] {
			return fmt.Errorf("%d players are forced to start at %s, but only %d can start at %s: clear one from must_start",
				forced[i], pos, xiMax[pos], pos)
		}
	}

	outfield := max(xiMin["DEF"], forced[1]) + max(xiMin["MID"], forced[2]) + max(xiMin["FWD"], forced[3])
	if outfield > XISize-1 {
		return fmt.Errorf("forced starters cannot fit one legal formation: %d DEF, %d MID and %d FWD need %d outfield places once every position's minimum is filled, and an eleven has %d: clear one from must_start",
			forced[1], forced[2], forced[3], outfield, XISize-1)
	}
	return nil
}

// restateSquadScoreExTiebreak puts SquadScore on the same basis as every other
// figure the struct carries.
//
// `bestScore` is the objective the search maximised, which includes the tiebreak
// nudge; XIScore beside it is clean. Reporting both would put two conventions in
// one struct, and the nudge is a selection device rather than a forecast — see
// tiebreak.go. With no tiebreak active there is nothing to restate and the
// objective already IS the clean sum, so the map is empty and this returns.
//
// Extracted from Optimize rather than left inline because the repository runs a
// complexity ratchet that only turns down, and this block was the two points that
// pushed Optimize from 64 to 66.
func restateSquadScoreExTiebreak(sq *Squad, nudges map[int]float64) {
	if len(nudges) == 0 {
		return
	}
	sq.SquadScore = 0
	for _, p := range sq.Players {
		sq.SquadScore += p.Score
	}
}

// boundInfeasible signals that no legal completion exists — either tier's
// bound overflowing this means the true minimum does too, since a lower
// bound cannot be infeasible while the truth is not.
const boundInfeasible = 1 << 30

// boundParams is what fillBound.cost and .exact are being asked to complete
// AROUND: a real candidate the greedy fill is about to commit to (id, pos and
// team all set), or the zero value for the up-front check, which asks the
// identical question about the whole squad with nothing pending — pos and
// team as "" and id as -1 never match a real player or a real club name, so
// every subtraction below is simply skipped.
type boundParams struct {
	id   int
	pos  string
	team string
}

// fillBound is the greedy fill's admissibility bound: the cheapest way to
// complete the squad after hypothetically adding the candidate named by
// boundParams, so the fill never commits to a player that stampedes the
// budget into a corner it cannot legally get out of.
//
// # Why the obvious fix is wrong
//
// The true question is "cheapest legal completion under position quotas AND
// the 3-per-club cap" — a transportation problem over two partition
// matroids. The tempting patch is a running per-club counter shared across
// every position as the old sort is walked once. That over-estimates, and is
// therefore INADMISSIBLE — a bound that can exceed the true cheapest
// completion silently rejects reachable squads, turning a feasibility bug
// into a search-quality regression instead of fixing it. Counterexample:
// cap[A]=1, need one GKP and one DEF; club A has a £4.0m GKP and a £4.0m DEF;
// the cheapest elsewhere are a £4.5m GKP and a £10.0m DEF. A shared counter
// spends A's one slot on whichever position it reaches first and pays
// £14.0m; the true optimum — GKP from A, DEF from elsewhere — is £8.5m.
//
// # Two tiers, shipped together
//
// Tier 1 (tier1) is a lower bound: for each needed position, walk its
// candidates price-ascending, giving THAT position the club's FULL headroom
// on its own (not shared with any other position's walk). Dropping the
// cross-position sharing constraint can only lower the optimum — for any
// feasible completion T, T restricted to position p is itself feasible for
// p's own relaxed subproblem, so tier1's sum is <= cost(T) for every legal T,
// hence <= the true minimum. And each position's own subproblem — cheapest k
// under one club's cap — is a partition matroid, so ascending-price greedy is
// exactly optimal for it. That makes tier1 strictly tighter than the old
// bound (its feasible set is the old bound's superset), so there is no old
// bound left to max() against.
//
// Tier 1 ALONE still dead-ends in exactly the thin-pool regime that produced
// the bug, so it must not be described as closing it: cap[A]=1, need one DEF
// and one FWD, A has a £4.0m DEF and a £4.5m FWD, next cheapest elsewhere are
// £4.5m and £9.0m. Tier 1 gives each position A's slot for free and reports
// 8.5; the truth is 9.0, because only one of DEF/FWD can actually come from
// A. Tier 1 makes the failure rarer, not impossible.
//
// Tier 2 (tier2) is the exact answer: a DP over clubs against the SHARED
// position-need state, so a club's cap really is spent once across every
// position rather than once per position. Rejection on tier 1 alone is
// always correct (tier1 <= true), so tier2 only has to run when tier1 says
// "might be affordable" — cheap-reject, expensive-only-on-accept.
//
// The corollary: the bound only changes a decision when
// old(x) <= remaining < new(x); since new(x) <= true(x), that means x is
// genuinely uncompletable, and the greedy only ever adds players, so
// committing to x always ends in the "could not fill" error anyway. So on
// every landscape where Optimize currently succeeds, the greedy seed this
// produces is byte-identical to what the old bound produced — only
// currently-failing runs change behaviour. That collapses the moment either
// tier over-estimates by even a tenth of a million, which is why
// admissibility is the one property this file cannot trade away for
// simplicity or speed.
//
// # What is static and what is live
//
// fillBound holds only the STRUCTURE built once from pool — which player
// sits where, and price order within each position and each (position,
// club) — never a second copy of a quantity that changes during the fill.
// posCount and clubCount stay the single map Optimize already threads
// through; tier1 and tier2 read them live on every call rather than
// mirroring them, which is deliberate: a mirror that can drift out of sync
// with the map it shadows is this package's signature failure.
type fillBound struct {
	// id, price and club are parallel to the pool fillBound was built from.
	id    []int
	price []int32
	club  []int32 // dense club id, see clubIdx

	// picked marks a pool index as already committed to `selected`. Set by
	// Optimize's add() closure as the fill proceeds; never cleared, because
	// the greedy fill only ever adds during one Optimize call.
	picked []bool

	// clubName maps a dense club id back to the name posCount/clubCount are
	// keyed by, so tier1/tier2 can read those live maps without holding a
	// second copy of the counts themselves.
	clubName []string
	clubIdx  map[string]int32
	numClubs int

	// poolIndexByID resolves a committed player back to his pool slot so
	// add() can mark him picked. A locked player can be absent from pool —
	// locks validate against byID, not the filtered pool — so this needs the
	// ", ok" guard at every call site; a miss means he was never a bound
	// candidate to begin with, which is correct, not an error.
	poolIndexByID map[int]int32

	// posOrder[p] lists pool indices of position p (posIdx order: GKP, DEF,
	// MID, FWD) in ascending price order, for tier 1's per-position walk.
	posOrder [4][]int32
	// clubPosOrder[p][club] is the same, restricted to one club, for tier
	// 2's per-(club,position) cheapest-k lookup.
	clubPosOrder [4][][]int32

	// used is tier 1's per-call, per-club counter. Reset at the start of
	// EVERY position's own walk, not once per tier1() call — Tier 1's
	// relaxation gives each position the club's full headroom
	// independently, which is the whole reason it is only a lower bound (see
	// the type doc). Hoisted here so resetting it is a plain write loop
	// rather than a fresh map per call.
	used []int32

	// dp/next are tier 2's DP buffers, sized at the state space's true upper
	// bound: (GKP need+1) x (DEF need+1) x (MID need+1) x (FWD need+1) <=
	// 3*6*6*4 = 432 (quotas are 2/5/5/3, plus one each for the zero state).
	// Fixed rather than grown, so no Tier 2 call ever allocates one.
	dp, next [432]int
}

// buildFillBound builds fillBound's static structure once, O(pool log pool).
// Nothing here changes for the rest of one Optimize call.
func buildFillBound(pool []PlayerMetrics) *fillBound {
	fb := &fillBound{
		clubIdx:       map[string]int32{},
		poolIndexByID: make(map[int]int32, len(pool)),
	}
	n := len(pool)
	fb.id = make([]int, n)
	fb.price = make([]int32, n)
	fb.club = make([]int32, n)
	fb.picked = make([]bool, n)
	posOf := make([]int8, n)

	for i, m := range pool {
		fb.id[i] = m.ID
		fb.price[i] = int32(priceUnits(m))
		cid, ok := fb.clubIdx[m.Team]
		if !ok {
			cid = int32(len(fb.clubName))
			fb.clubIdx[m.Team] = cid
			fb.clubName = append(fb.clubName, m.Team)
		}
		fb.club[i] = cid
		posOf[i] = int8(posIdx(m.Position))
		fb.poolIndexByID[m.ID] = int32(i)
	}
	fb.numClubs = len(fb.clubName)
	fb.used = make([]int32, fb.numClubs)

	// Ascending-price index lists, built once. Ties are broken by pool
	// order, which is fine: only price SUMS escape this structure, never
	// player identities, so tie order here cannot leak into the answer —
	// this design structurally cannot reproduce the dpSeeds map-order bug
	// class.
	type kv struct {
		idx   int32
		price int32
	}
	var perPos [4][]kv
	perClubPos := make(map[int32]*[4][]kv, fb.numClubs)
	for i := 0; i < n; i++ {
		p := posOf[i]
		if p < 0 {
			continue
		}
		e := kv{int32(i), fb.price[i]}
		perPos[p] = append(perPos[p], e)
		cid := fb.club[i]
		cp, ok := perClubPos[cid]
		if !ok {
			cp = &[4][]kv{}
			perClubPos[cid] = cp
		}
		cp[p] = append(cp[p], e)
	}
	sortKV := func(s []kv) {
		slices.SortFunc(s, func(a, b kv) int { return int(a.price) - int(b.price) })
	}
	for p := 0; p < 4; p++ {
		sortKV(perPos[p])
		fb.posOrder[p] = make([]int32, len(perPos[p]))
		for i, e := range perPos[p] {
			fb.posOrder[p][i] = e.idx
		}
		fb.clubPosOrder[p] = make([][]int32, fb.numClubs)
	}
	for cid, cp := range perClubPos {
		for p := 0; p < 4; p++ {
			sortKV(cp[p])
			out := make([]int32, len(cp[p]))
			for i, e := range cp[p] {
				out[i] = e.idx
			}
			fb.clubPosOrder[p][cid] = out
		}
	}
	return fb
}

// need returns how many of each position are still required (posIdx order),
// after also reserving one for p.pos if it names a position — used by both
// tiers so they agree on exactly the same subproblem.
func fillNeed(posCount map[string]int, p boundParams) (out [4]int, total int) {
	for i, pos := range posNames {
		n := squadQuota[pos] - posCount[pos]
		if pos == p.pos {
			n--
		}
		if n < 0 {
			n = 0
		}
		out[i] = n
		total += n
	}
	return out, total
}

// cost is the fill loop's hot-path entry point: tier 1's cheap reject first,
// tier 2's exact answer only when tier 1 does not already settle it. remaining
// is the budget left AFTER hypothetically paying for p itself, so the caller's
// "is this candidate affordable" check is simply cost(...) > remaining.
func (fb *fillBound) cost(posCount, clubCount map[string]int, p boundParams, remaining int) int {
	b := fb.tier1(posCount, clubCount, p)
	if b > remaining {
		// Tier 1 rejecting is always correct on its own (tier1 <= true), so
		// there is nothing tier 2 could add here except the exact number of
		// a rejection nobody asked for.
		return b
	}
	return fb.tier2(posCount, clubCount, p)
}

// exact is the true minimum completion cost, unconditionally — used by the
// up-front feasibility check, which has no "remaining" to compare against
// yet and wants a real answer rather than the fill loop's early-reject.
func (fb *fillBound) exact(posCount, clubCount map[string]int, p boundParams) int {
	b := fb.tier1(posCount, clubCount, p)
	if b >= boundInfeasible {
		return b
	}
	return fb.tier2(posCount, clubCount, p)
}

// tier1 is the per-position, club-relaxed lower bound described on fillBound.
func (fb *fillBound) tier1(posCount, clubCount map[string]int, p boundParams) int {
	pendingClub, hasPendingClub := fb.clubIdx[p.team]
	need, _ := fillNeed(posCount, p)
	total := 0
	for i := 0; i < 4; i++ {
		if need[i] == 0 {
			continue
		}
		c, ok := fb.tier1Position(i, p.id, clubCount, pendingClub, hasPendingClub, need[i])
		if !ok {
			return boundInfeasible
		}
		total += c
	}
	return total
}

// tier1Position finds the cheapest `need` candidates of position p (posIdx
// index), giving the position the WHOLE of every club's headroom to itself —
// see the admissibility argument on fillBound.
func (fb *fillBound) tier1Position(pos int, pendingID int, clubCount map[string]int, pendingClub int32, hasPendingClub bool, need int) (int, bool) {
	for i := range fb.used {
		fb.used[i] = 0
	}
	got, cost := 0, 0
	for _, idx := range fb.posOrder[pos] {
		if fb.picked[idx] || int(fb.id[idx]) == pendingID {
			continue
		}
		cid := fb.club[idx]
		limit := int32(MaxPerClub) - int32(clubCount[fb.clubName[cid]])
		if hasPendingClub && cid == pendingClub {
			limit--
		}
		if limit < 0 {
			// canAdd already guarantees clubCount[p.team] < MaxPerClub before
			// a real pending player is added, so this means that
			// precondition broke somewhere upstream. Clamped rather than
			// left negative — not because a negative value would compare
			// differently here (fb.used starts at 0, and 0 < a negative
			// number is already false) but so nothing downstream that reads
			// `limit` as a capacity ever sees an impossible one.
			limit = 0
		}
		if fb.used[cid] >= limit {
			continue
		}
		fb.used[cid]++
		cost += int(fb.price[idx])
		got++
		if got == need {
			return cost, true
		}
	}
	return 0, false
}

// tier2 is the exact bound: a DP over clubs, folding each club's own
// cheapest allocation across ALL FOUR positions into one state indexed by
// remaining need — so a club's 3-player cap is spent once, shared across
// positions, rather than once per position as tier 1 allows. See fillBound's
// doc for why this only runs on tier 1's accept path.
func (fb *fillBound) tier2(posCount, clubCount map[string]int, p boundParams) int {
	need, total := fillNeed(posCount, p)
	if total == 0 {
		return 0
	}
	pendingClub, hasPendingClub := fb.clubIdx[p.team]

	dims := [4]int{need[0] + 1, need[1] + 1, need[2] + 1, need[3] + 1}
	size := dims[0] * dims[1] * dims[2] * dims[3]
	idx := func(g, d, m, f int) int { return ((g*dims[1]+d)*dims[2]+m)*dims[3] + f }

	dp := fb.dp[:size]
	next := fb.next[:size]
	for i := range dp {
		dp[i] = boundInfeasible
	}
	dp[idx(need[0], need[1], need[2], need[3])] = 0

	var cum [4][4]int // cum[pos][k] = price of the k cheapest at this club, this position
	var cumLen [4]int // how many of cum[pos] are filled in, including cum[pos][0]=0

	for cid := int32(0); cid < int32(fb.numClubs); cid++ {
		headroom := MaxPerClub - clubCount[fb.clubName[cid]]
		if hasPendingClub && cid == pendingClub {
			headroom--
		}
		if headroom <= 0 {
			continue
		}
		anyCandidate := false
		for pos := 0; pos < 4; pos++ {
			maxK := headroom
			if need[pos] < maxK {
				maxK = need[pos]
			}
			cumLen[pos] = fb.cumulative(pos, cid, p.id, maxK, &cum[pos])
			if cumLen[pos] > 1 {
				anyCandidate = true
			}
		}
		if !anyCandidate {
			continue
		}

		copy(next, dp)
		for g := 0; g <= need[0]; g++ {
			maxG := cumLen[0] - 1
			if maxG > g {
				maxG = g
			}
			for d := 0; d <= need[1]; d++ {
				maxD := cumLen[1] - 1
				if maxD > d {
					maxD = d
				}
				for m := 0; m <= need[2]; m++ {
					maxM := cumLen[2] - 1
					if maxM > m {
						maxM = m
					}
					for f := 0; f <= need[3]; f++ {
						maxF := cumLen[3] - 1
						if maxF > f {
							maxF = f
						}
						base := dp[idx(g, d, m, f)]
						if base >= boundInfeasible {
							continue
						}
						for ag := 0; ag <= maxG; ag++ {
							for ad := 0; ad <= maxD; ad++ {
								if ag+ad > headroom {
									break
								}
								for am := 0; am <= maxM; am++ {
									if ag+ad+am > headroom {
										break
									}
									for af := 0; af <= maxF; af++ {
										if ag+ad+am+af > headroom {
											break
										}
										cost := base + cum[0][ag] + cum[1][ad] + cum[2][am] + cum[3][af]
										ni := idx(g-ag, d-ad, m-am, f-af)
										if cost < next[ni] {
											next[ni] = cost
										}
									}
								}
							}
						}
					}
				}
			}
		}
		copy(dp, next)
	}

	return dp[idx(0, 0, 0, 0)]
}

// cumulative fills cum[0..k] with the price of the k cheapest available
// (unpicked, non-pending) candidates at (position pos, club cid), k capped
// at maxK (<= MaxPerClub, so cum's fixed [4]int never overflows), and
// returns how many entries it wrote — k+1, since cum[0] is always the empty
// allocation at price 0.
func (fb *fillBound) cumulative(pos int, cid int32, pendingID int, maxK int, cum *[4]int) int {
	cum[0] = 0
	got := 0
	for _, idx := range fb.clubPosOrder[pos][cid] {
		if got >= maxK {
			break
		}
		if fb.picked[idx] || int(fb.id[idx]) == pendingID {
			continue
		}
		got++
		cum[got] = cum[got-1] + int(fb.price[idx])
	}
	return got + 1
}

// cheapestByPosition returns the n cheapest candidates per position — the
// bench-fodder pool. Ties on price are broken by score so we get the best
// available player at the price floor.
func cheapestByPosition(pool []PlayerMetrics, n int) map[string][]PlayerMetrics {
	out := map[string][]PlayerMetrics{}
	byPos := map[string][]PlayerMetrics{}
	for _, m := range pool {
		byPos[m.Position] = append(byPos[m.Position], m)
	}
	for pos, ms := range byPos {
		slices.SortStableFunc(ms, func(a, b PlayerMetrics) int {
			if a.Price != b.Price {
				if a.Price < b.Price {
					return -1
				}
				return 1
			}
			return byScoreDesc(a, b)
		})
		if len(ms) > n {
			ms = ms[:n]
		}
		out[pos] = ms
	}
	return out
}

// strongestByPosition returns the n highest-scoring candidates per position.
func strongestByPosition(pool []PlayerMetrics, n int) map[string][]PlayerMetrics {
	out := map[string][]PlayerMetrics{}
	byPos := map[string][]PlayerMetrics{}
	for _, m := range pool {
		byPos[m.Position] = append(byPos[m.Position], m)
	}
	for pos, ms := range byPos {
		slices.SortStableFunc(ms, byScoreDesc)
		if len(ms) > n {
			ms = ms[:n]
		}
		out[pos] = ms
	}
	return out
}

func clubCountAfter(counts map[string]int, leaving, joining string) int {
	c := counts[joining]
	if leaving == joining {
		return c
	}
	return c + 1
}

// clubCountsOf tallies a settled squad by club, for reporting.
//
// The distinction against every other club-count map in this file is the tense:
// clubHeadroom, clubCountAfter and clubsLegalAfterPair all read a
// squad-IN-PROGRESS count that the stage doing the search maintains as it
// works, and they answer "may this move be made". This one is handed a finished
// list of players and answers "what did we end up with". Do not route the
// search through it — recounting fifteen players per candidate move is the cost
// the incremental maps exist to avoid — and do not report an incremental map,
// which is the bug at Optimize's return that this function replaced.
//
// The two other places that put ClubCounts on a struct — the web planner's
// hand-built fifteen in cmd/armband/squadchoice.go and buildChipTeam in
// internal/viewmodel — already tally the FINAL list in a pass they need for the
// cost sum anyway, and are correct for that reason.
func clubCountsOf(players []PlayerMetrics) map[string]int {
	counts := make(map[string]int, len(players))
	for _, p := range players {
		counts[p.Team]++
	}
	return counts
}

// clubHeadroom reports whether club has a free slot under the MaxPerClub
// cap, given the squad-in-progress counts. The one-line seam canAdd and the
// fill loop route the plain "is there room to add one more" question
// through, so it and clubCountAfter (the "after a simultaneous leave and
// join" question) stay two names for two different questions rather than
// each being re-derived at its call site.
func clubHeadroom(counts map[string]int, club string) bool {
	return counts[club] < MaxPerClub
}

// clubsLegalAfterPair checks the 3-per-club limit for a simultaneous two-player
// change, which single-swap arithmetic gets wrong when both touch the same club.
func clubsLegalAfterPair(counts map[string]int, downOut, downIn, upOut, upIn PlayerMetrics) bool {
	delta := map[string]int{}
	delta[downOut.Team]--
	delta[upOut.Team]--
	delta[downIn.Team]++
	delta[upIn.Team]++
	for club, d := range delta {
		if counts[club]+d > MaxPerClub {
			return false
		}
	}
	return true
}

// objective scores a 15-man squad as its best XI plus a discounted bench.
// xiValue is what a starting eleven is actually worth: its total, plus the
// captain's score a second time, because the armband doubles one player.
//
// Within a single squad this is constant across formations — every position
// starts at least one player, so the squad's highest scorer is in the eleven
// whatever the shape. It matters when comparing *different squads*. Two squads
// totalling 49.5 are not worth the same if one is built around a 6.3 and the
// other is flat, and without this term the optimiser was indifferent between
// them.
// # The vice-captain
//
// FPL doubles the vice-captain when the captain records no minutes, so the
// armband is worth more than the captain alone. Score already prices the
// captain's own availability — it is expected points, minutes included — so the
// first term is correct as it stands. What is missing is the branch where he
// does not appear at all and the armband falls to someone else.
//
// The expected bonus is therefore captain + P(captain blanks) x vice, and a
// squad with a strong second-best is genuinely worth more than one without.
// ViceCaptainWeight is that probability; it is small, and so is the term.
// # The captain term doubles the estimate's worst error
//
// `captain` is a **maximum over estimates**, and a maximum over noisy estimates
// is biased upward even when every individual estimate is unbiased — the
// winner's curse. Worse, the optimiser then searches for the squad that
// maximises this, so the bias is not merely inherited, it is sought.
//
// Measured, that is the objective's only real blind spot.
// TestDiagObjectiveDivergence asks two searches what they would do from the same
// squad and scores both against what happened. Overall the objective is sound —
// the stronger search's extra modelled gain does buy extra realised gain — with
// one exception, buying a player over £9.0m:
//
//	most expensive bought    n    excess modelled   excess realised
//	under £6.0m            147            -0.240            +0.101
//	£6.0-9.0m               41            +0.738            +0.902
//	£9.0m and up            23            +1.460            -0.258
//
// A premium acquisition *becomes* the highest scorer, so his score is counted
// twice — once in the sum, once as captain — and the buy-side over-rating that
// TestDiagTransferError puts at 0.53 pts/gw is doubled with it.
//
// captainShrink pulls the captain term toward the runner-up:
//
//	captain_effective = vice + shrink x (captain - vice)
//
// The shape is targeted rather than global. When the best two are close the term
// is untouched, which is right — either could be the true best, so the maximum is
// credible. When the best is far clear of the field, which is exactly what
// buying a premium produces, it is damped hardest.
//
// # It does not work, and the reason is worth more than the fix
//
// Swept over six entry points and four seasons, paired against no shrink:
//
//	shrink   transfers (POLICY)      opening fifteen (HOLD)
//	0.85     -0.053/gw  t=-1.32      +0.000/gw  t=+0.00
//	0.70     -0.032/gw  t=-0.19      -0.110/gw  t=-1.01
//	0.50     -0.006/gw  t=-0.03      -0.150/gw  t=-1.01
//
// Every value is negative or zero on both metrics and nothing is distinguishable
// from noise. **It ships at 1.0 — disabled — and the mechanism it was built on
// is retained only as a warning.**
//
// The diagnosis: the defect is real but this fix is aimed at the wrong half of
// it. The measured error is in the *estimate of a player being bought*, and the
// captain term merely doubles whatever that estimate says. Shrinking the term
// therefore penalises a correctly-rated incumbent star exactly as hard as a
// speculative purchase — Haaland at 5.25 loses the same credit as a premium the
// model has over-rated — so it removes real value alongside the error and nets
// out.
//
// What that implies for the next attempt: the correction belongs on the buy
// side, conditioned on the player being *acquired*, not on the objective's
// treatment of whoever happens to be top. That is the same conclusion the
// minutes work reached from the other direction — correct the input, not the
// answer.
//
// FPL_CAPTAIN_SHRINK re-measures it.
// xiValue takes the shrink explicitly so the two consumers can differ.
//
// The captain term counts the squad's highest scorer twice, and a maximum over
// noisy estimates is biased upward. Shrinking it toward the runner-up was
// measured as negative or zero on both metrics — but it was applied *here*,
// which every caller reaches, while the defect it was built for was measured on
// transfers alone: TestDiagObjectiveDivergence compares two searches from a
// common squad and finds the objective claiming +1.460 pts/gw on a £9.0m+ buy
// and delivering −0.258.
//
// Squad construction needs the term at full strength for a different reason —
// it is what stops the optimiser being indifferent between a flat eleven and one
// built around a star. So the shrink is passed in: 1.0 from squad building,
// captainShrink from XIValue. Same seam that let fixture load reach transfers
// without touching the opening fifteen.
// It returns the armband term alongside the total, for the one caller that needs
// to pay for it a second time — a planned triple captain adds one further copy of
// exactly this quantity, and recomputing "what the armband is worth" beside it
// would be a second implementation of the captaincy rule. This package's
// signature failure is one quantity with two implementations; four instances are
// on the record.
func xiValueShrunk(pick []PlayerMetrics, shrink float64) (total, armband float64) {
	var sum, captain, vice float64
	for _, p := range pick {
		sum += p.Score
		captain, vice = promote(captain, vice, p.Score)
	}
	armband = captain
	if shrink < 1 && captain > vice {
		armband = vice + shrink*(captain-vice)
	}
	return sum + armband + ViceCaptainWeight*vice, armband
}

// xiValue is the eleven's worth to squad construction: the armband at full
// strength, which is what stops the optimiser being indifferent between a flat
// eleven and one built around a star.
//
// It is `xiValueShrunk` at shrink 1 and nothing more. It used to be that function's
// body written out again, eighteen lines below a comment saying that recomputing
// "what the armband is worth" beside it would be a second implementation of the
// captaincy rule and that this package's signature failure is one quantity with two
// implementations. Both loops walked the same slice in the same order and returned
// the same expression, so this is bit-identical rather than merely equivalent —
// with `shrink >= 1` the shrink branch is skipped and `armband` is `captain`.
//
// ⚠️ `xiValueOfParts` is a THIRD copy of this arithmetic and must not be folded in.
// It walks four sorted position slices instead of one eleven, deliberately, to avoid
// building eight 11-player slices per evaluation — and floating-point addition is
// not associative, so summing the same scores in a different order lands a ULP away.
// The objective feeds an argmax where a ULP flips a player, so merging it would
// change replayed seasons. One quantity, two implementations, one of them justified.
func xiValue(pick []PlayerMetrics) float64 {
	v, _ := xiValueShrunk(pick, 1)
	return v
}

func objective(squad []PlayerMetrics, benchWeight float64, boost bool) float64 {
	return objectiveWith(squad, benchWeight, nil, boost)
}

// ObjectiveFor scores a squad on **the quantity Optimize maximises**, resolving
// every parameter from req exactly as Optimize resolves it.
//
// # Why this is exported, and why it is not XIValue
//
// A diagnostic that wants to ask "did the search return its own argmax?" has to
// compare two squads on the objective the search was actually climbing. That is
// not `XIValue` — the transfer search's scoring function, which ignores the bench
// entirely — and it is not `Squad.XIScore` or `Squad.ExpectedPoints`, neither of
// which carries the bench weighting. Scoring the comparison on any of those
// answers a different question and would read as a search defect whenever the
// bench differs, which is most of the time.
//
// The parameter resolution is the whole point: `BenchWeight` falls back to the
// engine's weight when unset, bench boost bypasses the bench weighting
// altogether, and `StartIDs` forces an eleven that every candidate must be
// scored on. A caller that reproduced those rules by hand would be a second
// implementation of them, which is the failure this file already had once with
// the pool filter. See CutByExpectedMinutesFloor.
//
// ⚠️ It calls the same two resolvers Optimize calls rather than restating them.
// A first version inlined the BenchWeight fallback and hardcoded a nil
// mustStart, which review caught: the doc promised "every parameter from req"
// while silently dropping StartIDs, and the signature made that a trap, since a
// caller with forced starters would get a squad scored on the free best eleven —
// reliably higher than the one the search climbed, and so reading as the search
// leaving points on the table in every cell.
func (e *Engine) ObjectiveFor(squad []PlayerMetrics, req OptimizeRequest) float64 {
	return objectiveWith(squad, e.resolveBenchWeight(req), mustStartSet(req), req.BenchBoost)
}

// resolveBenchWeight is the one place the BenchWeight fallback lives.
//
// Extracted because Optimize and ObjectiveFor must agree: if a third fallback is
// ever added — a per-position weight, a chip-aware scale — a second copy would
// keep returning the old number and any diagnostic built on it would silently
// score a function the search does not climb.
func (e *Engine) resolveBenchWeight(req OptimizeRequest) float64 {
	if req.BenchWeight > 0 {
		return req.BenchWeight
	}
	return e.Weights.BenchWeight
}

// mustStartSet is the forced-eleven set, resolved once so Optimize and
// ObjectiveFor cannot disagree about it. Empty is not nil-equivalent to callers
// that range it, but map reads on both return false, which is all the objective
// does with it.
func mustStartSet(req OptimizeRequest) map[int]bool {
	out := make(map[int]bool, len(req.StartIDs))
	for _, id := range req.StartIDs {
		out[id] = true
	}
	return out
}

// objectiveWith is the cold entry point: it allocates a scratch, uses it once and
// throws it away. The search holds its own scratch and calls sc.objective, which
// is the same code — see xiScratch.
func objectiveWith(squad []PlayerMetrics, benchWeight float64, mustStart map[int]bool, boost bool) float64 {
	var sc xiScratch
	return sc.objective(squad, benchWeight, mustStart, boost)
}

// xiScratch is the reusable working memory for one search.
//
// The objective allocated 176KB and 61 blocks per evaluation, and the local
// search evaluates it tens of millions of times per Optimize — so the optimiser
// spent its time in GC assist rather than arithmetic, and a CPU profile of it was
// almost entirely runtime.scanObject, madvise and scheduler wait. Every buffer
// here exists to take one of those allocations out of the inner loop.
//
// # It must not be shared between goroutines
//
// The tool runner fans calls out through an errgroup, so two searches can run at
// once. This is therefore a local of polish and of fundedUpgrade, passed down by
// pointer, and it is deliberately NOT hung off Engine — that is the recorded
// concurrent-map-write hazard, which took down a live run. A zero xiScratch is
// valid and grows itself on first use, so there is no initialisation to guard and
// nothing to reset between squads.
type xiScratch struct {
	// byPos holds the squad split by position and sorted, indexed by posIdx.
	byPos [4][]PlayerMetrics
	// pick and bench are the materialised eleven and bench.
	pick, bench []PlayerMetrics
	// outfield is benchValue's working copy of the non-keeper bench.
	outfield []PlayerMetrics
	// trial and trial2 back replaceInto, so a candidate squad is written over
	// the previous one instead of allocating a fresh fifteen per evaluation.
	trial, trial2 []PlayerMetrics
	// dist and next are the Poisson-binomial convolution's two buffers. Sized
	// for an eleven; slotProbabilities falls back to allocating if ever handed
	// more, so the bound is an optimisation rather than an assumption.
	dist, next [16]float64
	// sortPerm, sortMust and sortScore back the index sorts below: the
	// objective's four sorts compare keys that are fixed during a search
	// (must-start flags and scores), so each sort runs over an index
	// permutation and the values are permuted once. bench* serves the bench
	// sort in materialise, plain* the two pure byScoreDesc sorts.
	sortPerm   [4][]int
	sortMust   [4][]bool
	sortScore  [4][]float64
	benchPerm  []int
	benchMust  []bool
	benchScore []float64
	plainPerm  []int
	plainScore []float64
	// pickIDs holds the eleven's ids in pick order, so the bench's membership
	// test scans eight-byte ints rather than striding a 592-byte struct per
	// comparison. It is the same test on the same ids in the same order.
	//
	// ⚠️ Valid only until materialise's final permute, which reorders sc.pick
	// into score order and leaves this alone. Nothing reads it outside
	// materialise, and a reader who took it afterwards as "the eleven's ids"
	// would have element 0 disagreeing with the captain.
	pickIDs []int
	// prefSum, prefCap and prefVice are bestFormation's per-position prefix
	// records: entry k holds the sum, captain and vice of the first k players
	// of that position, so each formation's total is a constant-time fold
	// instead of an eleven-player replay.
	prefSum  [4][]float64
	prefCap  [4][]float64
	prefVice [4][]float64
}

// posIdx is a position's fixed slot in the scratch arrays, replacing a string
// hash in the innermost loop.
//
// It returns -1 for anything else, and the callers below skip those players
// rather than dropping them: an unrecognised position cannot be in a formation,
// so such a player falls through to the bench, which is exactly what the
// map-keyed version did.
func posIdx(pos string) int {
	switch pos {
	case "GKP":
		return 0
	case "DEF":
		return 1
	case "MID":
		return 2
	case "FWD":
		return 3
	}
	return -1
}

var posNames = [4]string{"GKP", "DEF", "MID", "FWD"}

// xiValueOfParts is xiValue over an eleven that has not been materialised.
//
// The eight candidate formations differ only in how many of each position they
// take, so the old code built eight 11-player slices — 8 x 11 x 592 bytes per
// evaluation — purely to hand each to xiValue. Walking the four sorted position
// slices in the same order costs nothing and adds the same numbers.
//
// The order is load-bearing and is GKP, DEF, MID, FWD, matching the order the
// old code appended them in: floating-point addition is not associative, so
// summing the same eleven scores in a different order can land a ULP away, and
// the objective feeds an argmax where a ULP flips a player.
func xiValueOfParts(parts [4][]PlayerMetrics) float64 {
	var sum, captain, vice float64
	for _, ps := range parts {
		for _, p := range ps {
			sum += p.Score
			captain, vice = promote(captain, vice, p.Score)
		}
	}
	return sum + captain + ViceCaptainWeight*vice
}

// split fills sc.byPos from the squad and sorts each position, returning how many
// players are forced to start at each.
func (sc *xiScratch) split(squad []PlayerMetrics, mustStart map[int]bool) (forced [4]int) {
	for i := range sc.byPos {
		sc.byPos[i] = sc.byPos[i][:0]
		sc.sortMust[i] = sc.sortMust[i][:0]
		sc.sortScore[i] = sc.sortScore[i][:0]
	}
	for _, p := range squad {
		i := posIdx(p.Position)
		if i < 0 {
			continue
		}
		// One lookup, read twice. The key and the map are the same on both
		// reads, so this is the same value the second lookup returned.
		must := mustStart[p.ID]
		sc.byPos[i] = append(sc.byPos[i], p)
		sc.sortMust[i] = append(sc.sortMust[i], must)
		sc.sortScore[i] = append(sc.sortScore[i], p.Score)
		if must {
			forced[i]++
		}
	}
	for i := range sc.byPos {
		sc.sortPerm[i] = permuteByKeys(sc.byPos[i], sc.sortMust[i], sc.sortScore[i], sc.sortPerm[i])
	}
	return forced
}

// bestFormation picks the eleven, returning the counts rather than the players.
//
// Ties go to the first formation reached, which is why the loop bounds and their
// order are preserved exactly: `>` rather than `>=`, d outermost, then m, then f.
// Accepting a tie later instead changes the formation string and the bench order
// while leaving the objective identical — the quietest possible regression, and
// one TestOptimizerDiffHarnessHasTeeth injects deliberately.
func (sc *xiScratch) bestFormation(forced [4]int) (bd, bm, bf int, best float64, ok bool) {
	// Hoisted out of the loop: these were four map lookups per iteration.
	dMin, dMax := xiMin["DEF"], xiMax["DEF"]
	mMin, mMax := xiMin["MID"], xiMax["MID"]
	fMin, fMax := xiMin["FWD"], xiMax["FWD"]
	nGK, nDEF, nMID, nFWD := len(sc.byPos[0]), len(sc.byPos[1]), len(sc.byPos[2]), len(sc.byPos[3])

	best = -1.0
	if nGK < 1 {
		return 0, 0, 0, best, false
	}

	// Prefix records per position: entry k is the sequential fold of the first
	// k players, so a formation's total folds the four positions' records
	// instead of replaying its eleven players.
	//
	// The ARMBAND half is exact. Each record's (captain, vice) is the same
	// promote() chain over the same players in the same order, and foldPair
	// merges two records exactly as replaying the second into the first would —
	// pinned by TestThePairFoldMatchesSequentialPlay over 200k tied sequences.
	//
	// ⚠️ The SUM half is a RE-ASSOCIATION, not a preserved order, and this
	// comment claimed the opposite until 2026-08-19. Each record accumulates its
	// own position left to right and the loop below then adds four partials,
	// g + d + m + f, where the code this replaced ran ONE accumulator across all
	// eleven. Floating-point addition is not associative, so the two groupings
	// differ in the last bits on about 39% of elevens with continuous scores
	// (200k trials, max gap 4.3e-14) — and 0% wherever scores are exact
	// multiples of 0.25, because sums of eleven of those are exactly
	// representable. optimizerdiff_test.go's corpus was entirely of that kind
	// until 2026-08-19 and so could not see this restructuring at all; it now
	// carries a continuous arm, and its comment records why a decimal grid must
	// not be added.
	//
	// It is BOUNDED rather than harmless. Both callers discard `best` and
	// recompute xiValue over the sorted eleven, so the only channel is the
	// `total > best` comparison below: it can change the chosen formation only
	// where two formations land within a ULP of each other. Measured, that is
	// 0 of 200k on continuous scores and 3.0% once scores are coarse enough for
	// two formations to tie EXACTLY — the tie is the hazard, not the gap. A
	// 36-cell replay A/B across six seasons came back exactly equal on every
	// cell. Do not widen this into a claim of bit-identity.
	build := func(pos int) {
		var sum, cap, vice float64
		ps := sc.byPos[pos]
		sc.prefSum[pos] = sc.prefSum[pos][:0]
		sc.prefCap[pos] = sc.prefCap[pos][:0]
		sc.prefVice[pos] = sc.prefVice[pos][:0]
		sc.prefSum[pos] = append(sc.prefSum[pos], 0)
		sc.prefCap[pos] = append(sc.prefCap[pos], 0)
		sc.prefVice[pos] = append(sc.prefVice[pos], 0)
		for _, p := range ps {
			sum += p.Score
			cap, vice = promote(cap, vice, p.Score)
			sc.prefSum[pos] = append(sc.prefSum[pos], sum)
			sc.prefCap[pos] = append(sc.prefCap[pos], cap)
			sc.prefVice[pos] = append(sc.prefVice[pos], vice)
		}
	}
	for pos := 0; pos < 4; pos++ {
		build(pos)
	}

	// The keeper is walked first and is constant across formations.
	gkCap, gkVice := sc.prefCap[0][1], sc.prefVice[0][1]

	for d := dMin; d <= dMax; d++ {
		for m := mMin; m <= mMax; m++ {
			for f := fMin; f <= fMax; f++ {
				if 1+d+m+f != 11 {
					continue
				}
				if nDEF < d || nMID < m || nFWD < f {
					continue
				}
				if forced[0] > 1 || forced[1] > d || forced[2] > m || forced[3] > f {
					continue
				}
				cap, vice := foldPair(gkCap, gkVice, sc.prefCap[1][d], sc.prefVice[1][d])
				cap, vice = foldPair(cap, vice, sc.prefCap[2][m], sc.prefVice[2][m])
				cap, vice = foldPair(cap, vice, sc.prefCap[3][f], sc.prefVice[3][f])
				sum := sc.prefSum[0][1] + sc.prefSum[1][d] + sc.prefSum[2][m] + sc.prefSum[3][f]
				total := sum + cap + ViceCaptainWeight*vice
				if total > best {
					best, bd, bm, bf, ok = total, d, m, f, true
				}
			}
		}
	}
	return bd, bm, bf, best, ok
}

// materialise writes the chosen eleven and the bench into the scratch, in the
// same orders the map-keyed version produced.
func (sc *xiScratch) materialise(squad []PlayerMetrics, d, m, f int, ok bool) {
	// The eleven, its ids and its sort keys are built in one pass. The keys used
	// to be gathered in a second walk of sc.pick, which re-read eleven
	// 592-byte structs to take one float out of each; taking Score and ID as
	// each player is appended reads the same values in the same order.
	sc.pick = sc.pick[:0]
	sc.pickIDs = sc.pickIDs[:0]
	sc.plainScore = sc.plainScore[:0]
	if ok {
		// GKP, DEF, MID, FWD — the append order is load-bearing, as in
		// xiValueOfParts: it fixes the summation order of the eleven scores.
		for i, n := range [4]int{1, d, m, f} {
			src := sc.byPos[i][:n]
			for j := range src {
				sc.pick = append(sc.pick, src[j])
				sc.pickIDs = append(sc.pickIDs, src[j].ID)
				sc.plainScore = append(sc.plainScore, src[j].Score)
			}
		}
	}

	// Bench is everyone not in the eleven, in squad order. Membership is by ID
	// over eleven entries rather than through a map: a fifteen-by-eleven scan is
	// cheaper than hashing, and it reproduces the old behaviour exactly even in
	// the degenerate case of a duplicated id, where both copies were excluded.
	// The scan reads sc.pickIDs rather than sc.pick, which is the same ids in
	// the same order over eight-byte elements instead of 592-byte ones.
	//
	// Bench order: reserve keeper last, outfield by descending score. Those keys
	// are gathered here rather than in a second walk of sc.bench, for the same
	// reason as the eleven's above.
	sc.bench = sc.bench[:0]
	sc.benchMust = sc.benchMust[:0]
	sc.benchScore = sc.benchScore[:0]
	for i := range squad {
		p := &squad[i]
		inXI := false
		for _, id := range sc.pickIDs {
			if id == p.ID {
				inXI = true
				break
			}
		}
		if !inXI {
			sc.bench = append(sc.bench, *p)
			sc.benchMust = append(sc.benchMust, p.Position != "GKP")
			sc.benchScore = append(sc.benchScore, p.Score)
		}
	}
	sc.benchPerm = permuteByKeys(sc.bench, sc.benchMust, sc.benchScore, sc.benchPerm)
	sc.plainPerm = permuteByKeys(sc.pick, nil, sc.plainScore, sc.plainPerm)
}

// objective is the hot path: the whole evaluation with nothing allocated once the
// scratch has grown.
//
// xiValue is recomputed over the *sorted* eleven rather than reusing the value
// bestFormation compared on, which is computed over the eleven in build order.
// Those are the same eleven summed in two different orders and can therefore
// differ in the last bits, and it was the sorted one that shipped.
func (sc *xiScratch) objective(squad []PlayerMetrics, benchWeight float64, mustStart map[int]bool, boost bool) float64 {
	forced := sc.split(squad, mustStart)
	d, m, f, _, ok := sc.bestFormation(forced)
	sc.materialise(squad, d, m, f, ok)
	return xiValue(sc.pick) + sc.benchValue(sc.pick, sc.bench, benchWeight, boost)
}

// Bench slots are not interchangeable, and a flat multiplier says they are.
//
// A bench player earns his place three ways, and every one of them requires him
// to actually play for his club: FPL only autosubs a player who recorded
// minutes, he can be promoted into the XI when it is re-picked, and he covers an
// injury without a transfer being spent. A £4.0m third-choice keeper who never
// appears delivers none of the three, which is why the popular hedge is a cheap
// *playing* defender or midfielder rather than the cheapest body available.
//
// Score already carries the player's own MinutesRating, so it distinguishes
// those two on its own. What it cannot express is that the slots differ:
//
//   - The reserve keeper covers exactly one player, and only when that player
//     records no minutes at all. It is the least valuable slot on the bench by
//     some distance.
//   - The first outfield substitute is the one that actually comes on, and can
//     cover several XI positions subject to the formation staying legal.
//   - The third is reached only when three starters blank in the same week.
//
// These weights are relative to benchWeight and sum to four, so benchWeight
// stays the overall scale and this only redistributes it.
//
// # 2.4 / 1.0 / 0.4 / 0.2, replacing 1.9 / 1.2 / 0.6 / 0.3
//
// Swept via FPL_BENCH_SLOTS across four seasons. It is the only shape tried
// that beats the old one both on the three tuned seasons (+30 mean) and on
// held-out 2022-23 (+27), which is the whole of the case for it — the mean
// alone would not be enough, since a shape sweep picking its own argmax out of
// six is exactly how this file has manufactured effects that size before.
//
// It steepens the ordering the comment above describes rather than changing it:
// more of the credit on the substitute who actually comes on, less on the third
// outfielder and the reserve keeper, who are reached only in the weeks the
// squad has already gone wrong.
//
// Two things this does not settle. Flat weights are 51 worse on the tuned three
// and 79 *better* on 2022-23, so "shaped beats flat" is not established by
// anything — only "this shape beats that shape" is. And 2023-24 and 2025-26
// return identical totals for five of six shapes, so the sensitivity lives in
// two seasons and the rest of the evidence is inert. The principled version,
// weighting each slot by the blank-probability of the XI slots it could cover,
// is still the better answer than a fourth hand-set tuple.
//
// ⚠️ Those two figures are RETRACTED as magnitudes, and the paragraph's
// conclusion — "shaped beats flat is not established by anything" — survives and
// is now measured at 36 cells rather than four.
//
// Re-measured 2026-08-13 by TestDiagBenchShape, four arms, paired on HOLD
// against flat: +5.1 a season for this tuple, +2.2 for the predecessor, −6.8 for
// the derived weights, against per-arm thresholds of 17 to 40. A span of about
// 12 points across four pricing schemes, none resolving, is a tie.
//
// The 51 and the 79 are retracted for the simplest reason available: each rests
// on FOUR cells, which is inside the standing rule that a verdict at twelve
// cells or fewer is unverified. They are also POLICY totals at a GW1 entry,
// where the current archive puts flat 177-214 AHEAD — so neither number
// reproduces on its own design, and the sign of the whole question depends on
// which metric is read.
//
// ⚠️ Do NOT read any of this as evidence against the derived weights. The
// derived arm is the lowest of the four and that is not usable: 94% of its gap
// is one cell of 36, the sign reverses when that cell is dropped, and the
// derived path does not renormalise to sum 4 as these tuples do — so it varies
// effective benchWeight as well as shape. See benchslots.go.
//
// See stats/findings/2026-08-13-benchshape.md.
//
// Sweepable via FPL_BENCH_SLOTS; see benchSlotWeights, which renormalises so
// the shape can be varied without also varying benchWeight's scale.
var benchOutfieldWeights, benchGKWeight = benchSlotWeights()

// benchValue is the cold entry point; the search reaches the same code through
// its own scratch.
func benchValue(xi, bench []PlayerMetrics, benchWeight float64, boost bool) float64 {
	var sc xiScratch
	return sc.benchValue(xi, bench, benchWeight, boost)
}

func (sc *xiScratch) benchValue(xi, bench []PlayerMetrics, benchWeight float64, boost bool) float64 {
	// Bench boost pays every one of the fifteen, so there is no "probability
	// this slot is reached" left to discount by and no scale to apply: all four
	// bench players score with certainty, exactly like a starter.
	//
	// This is the one case where the bench is not a hedge. Everywhere else the
	// slot weights price how often a substitute is actually used, which is why
	// they are small; under the chip they are all 1.0 by definition.
	//
	// Checked before the flat-bench branch and before the scratch convolution,
	// because under the chip neither is a question that arises.
	if boost {
		var s float64
		for _, p := range bench {
			s += p.Score
		}
		return s
	}

	if flatBenchWeight {
		var s float64
		for _, p := range bench {
			s += p.Score * benchWeight
		}
		return s
	}

	// Priced against the eleven this bench actually sits behind, unless the
	// fixed tuple has been asked for. See benchslots.go.
	slots, gkSlot := benchOutfieldWeights, benchGKWeight
	if derivedBenchSlots {
		slots, gkSlot = sc.benchSlotWeightsFor(xi)
	}

	outfield := sc.outfield[:0]
	var s float64
	for _, p := range bench {
		if p.Position == "GKP" {
			s += p.Score * benchWeight * gkSlot
			continue
		}
		outfield = append(outfield, p)
	}
	// Best first: that is the order a manager sets, and the order FPL
	// substitutes in.
	sc.plainScore = sc.plainScore[:0]
	for _, p := range outfield {
		sc.plainScore = append(sc.plainScore, p.Score)
	}
	sc.plainPerm = permuteByKeys(outfield, nil, sc.plainScore, sc.plainPerm)
	for i, p := range outfield {
		w := slots[len(slots)-1]
		if i < len(slots) {
			w = slots[i]
		}
		s += p.Score * benchWeight * w
	}
	sc.outfield = outfield
	return s
}

// flatBenchWeight restores the old uniform bench credit. Set FPL_FLAT_BENCH=1
// to measure the shaped weights against it; see FPL_NO_FUNDED_UPGRADE for the
// same pattern. Never changed in normal operation.
var flatBenchWeight = os.Getenv("FPL_FLAT_BENCH") != ""

// bestXI picks the highest-scoring legal starting eleven from a 15-man squad.
func bestXI(squad []PlayerMetrics) (xi, bench []PlayerMetrics, formation string) {
	return bestXIWith(squad, nil)
}

// bestXIWith picks the eleven, forcing the given players to start.
//
// Squad membership and selection are different guarantees, and conflating them
// wastes the money the constraint was meant to spend. Locking Isak put him in
// the fifteen; the optimiser then satisfied that in the cheapest way available,
// parking £9.0m on the bench at 0.53 pts/gw and building the eleven around it.
// That is the opposite of what "the squad has to be built around him" means.
//
// A player is forced by sorting him to the front of his position, so the top-n
// slice always contains him, and by skipping formations with too few slots at
// that position to seat everyone forced there.
// This is the materialising wrapper over the same core the search uses, so there
// is one implementation of "which eleven" rather than a fast one and a slow one
// that have to be kept in step. That distinction is this codebase's own bug
// class — DefaultBenchWeight against Weights.BenchWeight, fixtureSensitivePart
// drifting from baseXP90 — and the rule it settled on is that a duplicate which
// is *checked* is a pipeline test while a duplicate that is merely watched is the
// bug. Here there is nothing to check because there is nothing duplicated: the
// only difference is that this copies the result out and formats the formation,
// which the objective never needs.
func bestXIWith(squad []PlayerMetrics, mustStart map[int]bool) (xi, bench []PlayerMetrics, formation string) {
	var sc xiScratch
	forced := sc.split(squad, mustStart)
	d, m, f, _, ok := sc.bestFormation(forced)
	sc.materialise(squad, d, m, f, ok)

	if ok {
		formation = fmt.Sprintf("%d-%d-%d", d, m, f)
	}
	// Copied out rather than handed over: the caller keeps these, and the scratch
	// is about to be reused. Returning the buffers themselves is how a reused
	// buffer becomes a silent aliasing bug.
	xi = append([]PlayerMetrics(nil), sc.pick...)
	if len(sc.bench) > 0 {
		bench = append([]PlayerMetrics(nil), sc.bench...)
	}
	return xi, bench, formation
}

func squadSlice(m map[int]PlayerMetrics) []PlayerMetrics {
	out := make([]PlayerMetrics, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	slices.SortStableFunc(out, func(a, b PlayerMetrics) int {
		if a.ID < b.ID {
			return -1
		}
		if b.ID < a.ID {
			return 1
		}
		return 0
	})
	return out
}

func replace(squad []PlayerMetrics, outID int, in PlayerMetrics) []PlayerMetrics {
	return replaceInto(nil, squad, outID, in)
}

// replaceInto is replace writing into a caller-owned buffer, so the search does
// not allocate a fresh fifteen — 8.9KB — for every candidate it scores.
//
// # Two things about it that are load-bearing
//
// The incoming player goes at the *outgoing* player's index, not on the end. A
// trial squad is therefore neither id-ordered nor grouped by position, and since
// tie order is observable in the objective that is part of the answer rather than
// an implementation detail. See TestReplaceKeepsIncomingAtOutgoingIndex.
//
// And dst must not alias squad. Every caller passes a scratch buffer distinct
// from the squad it is reading, which is why the pairs loop needs two — and it
// is what lets the copy below be one bulk move rather than fifteen appends.
//
// The fifteen are copied in one memmove and the outgoing slot is then overwritten
// in place, which is the same fifteen in the same order: every index takes the
// value it took before, and any index whose id matched takes `in`, exactly as the
// element-wise loop did. The scan afterwards reads ids only, so the 8.9KB of
// struct copying happens once through memmove instead of through fifteen
// bounds-checked appends.
func replaceInto(dst, squad []PlayerMetrics, outID int, in PlayerMetrics) []PlayerMetrics {
	out := append(dst[:0], squad...)
	// Every match is replaced, not merely the first — the loop this replaces had
	// no break either, and a squad holding a duplicated id must not come back
	// with one copy swapped and one kept.
	for i := range out {
		if out[i].ID == outID {
			out[i] = in
		}
	}
	return out
}

var posOrder = map[string]int{"GKP": 0, "DEF": 1, "MID": 2, "FWD": 3}

func sortForDisplay(squad []PlayerMetrics) []PlayerMetrics {
	out := make([]PlayerMetrics, len(squad))
	copy(out, squad)
	slices.SortStableFunc(out, func(a, b PlayerMetrics) int {
		if posOrder[a.Position] != posOrder[b.Position] {
			if posOrder[a.Position] < posOrder[b.Position] {
				return -1
			}
			return 1
		}
		return byScoreDesc(a, b)
	})
	return out
}

// BestXI picks the highest-scoring legal eleven from a squad, returning it
// sorted by score so the captain is first.
//
// Exported for the backtest, which re-picks the eleven every simulated week
// exactly as a manager would rather than freezing the opening selection.
func BestXI(squad []PlayerMetrics) (xi, bench []PlayerMetrics, formation string) {
	return bestXI(squad)
}

// XIPoints is what a squad's best legal eleven scores, summed on Score.
//
// # Why this is here rather than in the backtest, where it was written
//
// It is the unit of **squad drift** — how far a held fifteen has fallen behind
// what could be built instead — and drift is only a replay quantity by accident
// of where it was first needed. A live manager wants the same number about his
// own team, so the function has two callers in different packages and must have
// one definition. `internal/backtest` cannot host it, because nothing in the
// product may depend on the replay harness.
//
// ⚠️ **It reuses [BestXI] rather than taking the top eleven by Score, and that is
// not an optimisation.** The top eleven by Score fields illegal formations — four
// goalkeepers, no forward — and so flatters a squad that is strong in one
// position and empty in another. Drift measured that way would read a lopsided
// squad as healthy.
//
// ⚠️ **A player the bootstrap does not know is skipped, not scored as zero.** A
// squad carrying an id from a different season would otherwise report a drift
// made of absences, which reads exactly like a squad that has decayed.
func XIPoints(e *Engine, squad []int) float64 {
	var ms []PlayerMetrics
	for _, id := range squad {
		if el := e.Boot.ElementByID(id); el != nil {
			ms = append(ms, e.Metrics(el))
		}
	}
	xi, _, _ := BestXI(ms)
	var sum float64
	for _, p := range xi {
		sum += p.Score
	}
	return sum
}

// ownedSquad resolves the caller's squad ids against the scored pool, in a
// stable order. A single unresolvable id is an error, not a shorter squad: a
// partial squad is not a legal starting point, and the caller's only sensible
// response is to say which id it could not price.
//
// It names the FIRST miss in `ids` order rather than collecting them, so the
// message is deterministic — ranging a map here would name a different id on
// different runs of the same request.
func ownedSquad(ids []int, byID map[int]PlayerMetrics) ([]PlayerMetrics, error) {
	out := make([]PlayerMetrics, 0, len(ids))
	for _, id := range ids {
		m, ok := byID[id]
		if !ok || m.ID == 0 {
			return nil, fmt.Errorf("current squad player id %d not found", id)
		}
		out = append(out, m)
	}
	return out, nil
}

// clearsMinutesFloor reports whether a candidate has enough recorded football
// behind him for his rates to be believable, or is exempt from needing it.
//
// The floor is a sample-size test, and two populations legitimately have no
// sample. Both were being dropped silently, because this check ran before the
// expected-minutes cliff and did not carry the cliff's exemptions.
//
// A cheap body, because FPL forces you to own fifteen players and roughly four
// of those are compliance slots. Excluding the £4.0m reserve keeper who never
// plays does not save money, it moves money onto the bench: with no £4.0m keeper
// in the pool the optimiser bought two at £4.5m and the spare £0.5m came out of
// the eleven.
//
// And a player the judgement layer has corrected, because an override is a claim
// about a role the record does not show — precisely the case where the record is
// the wrong thing to screen on. `blend.go` already promises this behaviour in so
// many words: the score, "whether he clears the minutes floor", and whether the
// optimiser wants him are all supposed to be recomputed from the override rather
// than dictated by it.
func clearsMinutesFloor(m PlayerMetrics, minMinutes int, fodderPrice float64, overridden bool) bool {
	if m.Minutes >= minMinutes {
		return true
	}
	if overridden {
		return true
	}
	// fodderPrice is zero when the caller has asked for every squad slot to clear
	// a minutes bar, which switches this exemption off rather than needing a
	// second flag.
	return m.Price <= fodderPrice
}

// BenchFodderPrice is the price at or below which a player is exempt from the
// expected-minutes floor, because a £4.0m reserve who never plays is exactly
// what belongs in squad slots 12-15.
const BenchFodderPrice = 4.5

// cutByExpectedMinutes reports whether the expected-minutes floor removes m from
// an opening-squad pool. It is the expected-minutes half of the pool filter, as
// clearsMinutesFloor is the total-minutes half.
//
// # Why this is a function rather than four lines inside Optimize
//
// A diagnostic that wants to count the removed population — which is the only way
// to tell "the optimiser declined these players" from "these players were never
// offered" — otherwise has to restate the rule, and a restatement is a second
// implementation of one quantity. That is this record's signature failure, and
// AGENTS.md names a diagnostic as the worst place for it, because a diagnostic is
// what everything else is checked against.
//
// It was nearly shipped that way: TestDiagFloorPopulation's first version
// hardcoded `SettledMinutes < floor && Price > 4.5`, which is correct today and
// silently wrong the moment either the field or the exemption moves. The two
// halves it would have missed are the `<= 0` guard and BenchMinExpectedMinutes.
//
// Screened on **settled** minutes, not the rested figure — a fortnight of being
// eased back in must not make a regular un-pickable. See
// PlayerMetrics.SettledMinutes.
func cutByExpectedMinutes(m PlayerMetrics, minExpected, fodderPrice, benchMinExpected float64) bool {
	if minExpected <= 0 || m.SettledMinutes >= minExpected {
		return false
	}
	// Keep him only if he can serve as cheap bench fodder.
	return m.Price > fodderPrice || m.SettledMinutes < benchMinExpected
}

// CutByExpectedMinutesFloor reports whether Optimize's expected-minutes floor
// would drop m from the pool built for req, resolving the bench-fodder exemption
// exactly as Optimize resolves it.
//
// Exported for diagnostics that count the removed population. Nothing on the
// scoring path calls it; Optimize uses the unexported predicate directly.
//
// ⚠️ **Whole-field, not pool-removal.** It answers "does m fail the floor
// predicate", which over `AllMetrics()` counts players the floor never got to
// judge — the total-minutes floor and the availability screen both run FIRST in
// Optimize, so a permanently injured 200-minute reserve is counted here and was
// already gone by the time the floor ran. The strictly smaller pool-removal
// count needs ReachesExpectedMinutesCut as well, and the two answer different
// questions: the whole-field figure describes the predicate, the pool-removal
// figure describes what the floor actually did.
func CutByExpectedMinutesFloor(m PlayerMetrics, req OptimizeRequest) bool {
	return cutByExpectedMinutes(m, req.MinExpectedMinutes, resolvedFodderPrice(req),
		req.BenchMinExpectedMinutes)
}

// resolvedFodderPrice and scaledMinMinutes are the two thresholds Optimize
// derives from a request, extracted so nothing re-derives them.
//
// They were each written out three times — in Optimize, in
// CutByExpectedMinutesFloor, and again the moment ReachesExpectedMinutesCut was
// added — which is this record's signature failure arriving by the usual route:
// not a rewrite, just one more caller that needed the same number.
func resolvedFodderPrice(req OptimizeRequest) float64 {
	if req.BenchMinExpectedMinutes > 0 {
		return 0 // caller wants every squad slot to clear a minutes bar
	}
	return BenchFodderPrice
}

// It forwards to the exported ScaledMinMinutesFor rather than repeating the
// expression, because `internal/agent` needs the same scaling and cannot reach an
// unexported method — that second copy is what B4 was about. Taking the request is
// this caller's convenience; the arithmetic has one home.
//
// The club is not this caller's convenience: the window the floor scales to is
// per club during the live GW1 gap, so a floor resolved without one is wrong for
// most of the league for several days a season. See Engine.minutesFloorWindow.
func (e *Engine) scaledMinMinutesFor(teamID int, req OptimizeRequest) int {
	return e.ScaledMinMinutesFor(teamID, req.MinMinutes)
}

// reachesExpectedMinutesCut reports whether m survives the two screens Optimize
// applies BEFORE the expected-minutes floor, so the floor is the reason he goes.
//
// Extracted from Optimize's pool loop rather than restated beside it — Optimize
// calls this, so there is one implementation and a diagnostic cannot drift from
// the consumer. That is the failure this file's neighbours were built to avoid
// twice already (see cutByExpectedMinutes).
func reachesExpectedMinutesCut(m PlayerMetrics, minMinutes int, fodderPrice float64,
	overridden, includeUnavailable bool) bool {
	if !clearsMinutesFloor(m, minMinutes, fodderPrice, overridden) {
		return false
	}
	if includeUnavailable {
		return true
	}
	switch m.Status {
	case "injured", "suspended", "unavailable", "not in squad":
		return false
	}
	return true
}

// ReachesExpectedMinutesCut reports whether m is still in the pool when
// Optimize's expected-minutes floor is applied, resolving every threshold from
// req and the engine exactly as Optimize does.
//
// Pair it with CutByExpectedMinutesFloor to count the population the floor
// actually removes: `Reaches && Cut`. Counting `Cut` alone over `AllMetrics()`
// is the whole-field figure, which is strictly larger and is the quantity a
// recorded 229.7-a-build measured — while the recorded 96-126 may have been
// this one. Reporting both is what tells those two apart.
func (e *Engine) ReachesExpectedMinutesCut(m PlayerMetrics, req OptimizeRequest) bool {
	overridden := false
	if e.hasMinutesOverrides() && e.Boot != nil {
		for i := range e.Boot.Elements {
			el := &e.Boot.Elements[i]
			if el.ID != m.ID {
				continue
			}
			if _, _, _, ok := e.minutesOverrideFor(el.Code); ok {
				overridden = true
			}
			break
		}
	}
	return reachesExpectedMinutesCut(m, e.scaledMinMinutesFor(m.TeamID, req),
		resolvedFodderPrice(req), overridden, req.IncludeUnavailable)
}
