package backtest

// Do grouped moves buy REACH?
//
//	DIAG=1 \
//	    go test ./internal/backtest -run TestDiagGroupedMoveReach -v -timeout 120m
//
// # STATUS: run at the full grid. The answer is in AGENTS.md, "Reach is not the problem"
//
// 588 decision points, 24 cells (four season pairs by six entry points), about nine
// minutes — the optimiser is roughly six times faster as of 7c065c4 and this was
// budgeted at an hour when the question was queued. Three earlier attempts were killed,
// twice by a session limit and once by the machine with no output at all.
//
// **The answer splits by denominator, and that is the finding.**
//
// Per PACKAGE, reassuring: 97.6% of worth-taking two-move packages are reachable
// (n = 166), three-move 68.0% (n = 50), and all 20 unreachable packages are refused by
// the GATE — zero club, zero money, so the arithmetic prediction about money holds.
//
// Per ASSET, not reassuring: 44 of 174 asset-seasons are touched (25.3%), and
// **M.Salah 2024-25 is unreachable in 6 of the 9 packages that want him.** There IS a
// systematic block on individual premiums.
//
// The reconciliation is that forgone value over those 20 packages is +0.018 pts/gw at
// the mean and -0.013 at the median — the gate is refusing packages the model roughly
// breaks even on, so the lever is the valuation rather than the price of a package.
// Read the PER-ASSET forgone column before believing that of any one asset: it is
// +0.787 for Salah and +1.675 for McNeil, both positive, which is the branch where the
// structure is the lever and not the valuation.
//
// # Every figure above is post-repair, and the pre-repair ones were wrong
//
// This file had no representation of the route decide() ACTUALLY walks most often at
// three moves — a funded pair, then the singles loop spending what is left. See
// reachByPairThenSteps. Three-move reach read 56.0% where it is 68.0%, the unreachable
// asset-season count read 54 where it is 44, and the taken-package anchor FAILED: the
// policy took a package this file called unreachable. Any reach figure recorded before
// that fix is a floor, and the three-move ones are materially wrong.
//
// **An eight-cell run of this same diagnostic reported "no systematic block" and was
// wrong.** It found no asset-season unreachable more than twice; four times the data
// produced 6 of 9. A count needs no significance machinery, and that is NOT the same
// claim as a count being insensitive to sample size.
//
// Read the self-check lines the run prints before anything else. There are two, both
// asserted FATALLY so the run produces no table when either fails — an earlier version
// used t.Errorf, and its tables were printed, grepped and quoted while the second check
// was failing, because nobody looked at the exit status.
//
//   - of the weeks where the policy made exactly one move, this file's gate
//     reconstruction must accept 100%. At 24 cells it accepts 222 of 222.
//   - every multi-move package the policy actually TOOK must be inside the permissive
//     bound. At 24 cells all 112 are. 111 of the 112 are inside the strict bound too;
//     the one miss is the recorded sell-price staleness, not a missing route, which is
//     why the anchor is on reachable() and the strict rate is reported as a number.
//
// # A second, separate defect: the recorded figures do not reproduce
//
// AGENTS.md's "xG repaired" reach column reads 2-move 97.7% (n = 171), 3-move 58.0%,
// multi-move 88.7%, 52 of 177 asset-seasons, forgone mean -0.060. Re-run at HEAD on the
// UNFIXED code — so the only difference from that run should be nothing — this file
// produces 97.6% (n = 166), 56.0%, 88.0%, 54 of 174, +0.013. The diagnostic is
// deterministic (two identical 8-cell runs are byte-identical bar the timing line) and
// no Go file changed between that commit and this one, so that column came from a tree
// state that cannot be reconstructed. Do not diff new figures against it.
//
// # The question, and why it is a count rather than a comparison
//
// Every restraint ever tried on the unified bounded-revision search restrained
// **price**: a per-move gain threshold, an escalating multi-move surcharge, a
// squad-pool floor. All of them changed essentially nothing, and the
// re-diagnosis in AGENTS.md says why — unified makes roughly the *same number of
// moves* as the bespoke policy and simply gets less for them. Volume was never
// the problem, so every restraint was aimed at the wrong quantity.
//
// The quantity nobody measured is **reach**. At a given squad, is the best two-
// or three-move revision actually reachable by a sequence of moves each of which
// the shipped gate would accept **on its own**? That is a count, not a
// difference of two season totals, so it has no significance problem — which
// matters here, because the per-comparison detection threshold on the transfer
// metric has a median of about 70 points a season and the two-arm sweep this
// question is usually asked with would most likely return "unresolved".
//
// Both answers are useful and they point in opposite directions:
//
//   - **Reach essentially always available.** Then the bespoke structure — one
//     funded pair, then singles, each gated separately — loses nothing, the
//     unified search has no expressive advantage to recover, and that whole line
//     of work closes. The surcharge machinery can go.
//   - **Reach frequently unavailable.** Then there is value the current
//     structure cannot express, and unified's recorded loss is reframed from "it
//     makes worse decisions" to "right idea, wrong objective" — which points at
//     §1 of the transfer-policy design document rather than at more restraint. It is
//     explicitly *not* a case for shipping unified, whose loss was measured at
//     similar volume.
//
// # The path is canonical, which is not optional
//
// The moment two decision rules disagree they hold different fifteens and
// nothing after that is attributable to the rule. So the shipped bespoke policy
// plays the season, and at every weekly decision point the questions are asked
// **at the squad it actually holds**, applying nothing. Same trick as
// TestDiagObjectiveDivergence and Judge.
//
// # What "reachable" means, precisely
//
// A k-move package is reachable by individually-accepted steps if some ordering
// of its moves exists such that every step, evaluated at the *intermediate*
// squad, would be accepted by the shipped singles loop in decide(). Evaluating
// the intermediates is the whole point: a package can be unreachable because its
// first leg looks bad in isolation, which is the documented "every step of a
// restructure is downhill" problem dpseed.go exists to solve on the
// squad-building side.
//
// Reachability is about the **gate**, not the sign of the objective. A step
// whose gain is positive but below the charge is not individually accepted.
//
// **Steps are one of THREE routes, and the third was missing for a while.**
// decide() is not a singles loop with a pair bolted on the front; it is a pair
// branch that falls through into the singles loop with `limit -= n`. So bespoke
// reaches a package three ways, and all three are counted:
//
//	steps    every leg accepted alone, in some ordering
//	pair     the whole package IS one funded pair RankPairs offers
//	pair+1   a funded pair covers part of it and the singles loop accepts the
//	         rest, which needs at least three moves to exist at all
//
// Leaving out the third made the file refuse a package the policy demonstrably
// took, and it is why the three-move figure was low. See reachByPairThenSteps.
//
// # The population must be filtered to packages worth taking, or reach is zero
// by construction
//
// This is the trap, and the first version of the diagnostic fell into it. XIValue
// gain is monotone in adding any improving move, so "the best 3-move package" is
// almost always "the best 2-move package plus a marginal third". The gate then
// refuses that third leg — **correctly**, because it is below the charge — and
// the package reads as unreachable. Measured that way, reach is near zero and
// means nothing.
//
// So the primary population is multi-move optima that clear the **group** bar
// bespoke already applies to a funded pair: gain at least MinGain, at most one
// hit, and a package value after transfer costs that beats spending the free
// transfer on the best single move and keeping the four points. That is a
// package the policy would *want*. Asking whether those are reachable is the
// real question; the unfiltered figures are printed underneath for completeness.
//
// # At shipped config the group bar and the singles gate are the same threshold,
// which is what makes the primary population sharp
//
// Worth writing out, because it says exactly which packages the primary count is
// about. At MinGain 0.4, FreeCost 2.0 and horizon 5 the singles gate with a free
// transfer in hand reduces to `gain >= 0.4` — both of its clauses demand that,
// which is the documented no-op. Now take a two-move package with two free
// transfers and gains g1 for the best single and g2 for the package. The group
// bar is
//
//	g2*5 - 2*FreeCost > g1*5 - FreeCost   <=>   g2 - g1 > 0.4
//
// and the gate on a second greedy rung is exactly `marginal gain >= 0.4`. So for
// any package the greedy ladder produces, **worth-taking and gate-reachable are
// the same condition** up to the boundary.
//
// The consequence is that the primary population is not "multi-move packages" in
// general. It is precisely the packages that **beat the greedy ladder** — where
// the whole is worth more than any sequence of individually-best steps. That is
// complementarity: a downgrade that loses value on its own and funds an upgrade
// that more than repays it, which is the Haaland-behind-Salah case the design
// item is named for. A count over that population is the reach question with
// nothing else mixed into it.
//
// # Three blocking reasons, kept apart
//
// A package can fail to be reachable for reasons that are not the same fact:
//
//	club        some club exceeds MaxPerClub at an intermediate squad, in every
//	            ordering. Position counts cannot break, since each leg is a
//	            same-position swap, but club counts can.
//	money       no ordering is affordable throughout, even ignoring club limits.
//	club+money  each constraint is satisfiable alone and not together.
//	gate        some ordering is affordable and club-legal throughout, and none
//	            of those clears the gate at every step.
//
// **The money count is predicted to be zero, and that prediction is arithmetic
// rather than hopeful.** Each leg's cash effect is order-independent, so putting
// every money-freeing leg first makes the running bank rise to a maximum and
// then fall to its final value; if the package is affordable as a whole, both
// ends are non-negative and therefore so is every intermediate. A non-zero money
// count would mean the *whole-package* affordability check disagrees with the
// leg-by-leg one, which would be a finding about the budget arithmetic rather
// than about reach. `club+money` is a genuine third thing — the ordering money
// wants can be the one the club limit forbids — and collapsing it into "money"
// is what made the first run's money column non-zero.
//
// # Bespoke is not "singles only", and the comparison respects that
//
// RankPairs already prices restraint structurally: a funded pair must beat
// spending the free transfer on the best single move and keeping the four
// points. So the honest question is what bespoke can *actually* reach, which is
// the union of two routes — the singles ladder, and the funded pair. The two are
// reported separately, and so is the distinction between RankPairs *proposing* a
// package (the structure can express it) and its rule *accepting* it (the policy
// would take it). Only the first is a reach question; the second is a price
// question, and this project has confirmed three times over that price is not
// where the points are.
//
// # What "the best k-move revision" is here, and why it is not one search
//
// The first version took the k-optimum from Optimize alone, the way
// unifiedDecide does, and the result was uninterpretable: the mean gain
// *forgone* came out **negative**, meaning the packages being tested for reach
// were routinely worse than what the bespoke policy had already done that week.
// Two handicaps caused it. Optimize maximises the unexported `xiValue`, which
// carries no fixture load, while the gate and every gain here use `XIValue`,
// which does; and unifiedDecide applies a squad-pool floor that `RankSwaps` does
// not, so its space is not a superset of bespoke's.
//
// So the k-optimum is the best package of that size **by XIValue** over every
// search this codebase has:
//
//	greedy ladder    the best single swap, then the best from there, then again.
//	                 This is literally what the bespoke singles loop produces, so
//	                 including it can only make reach look *more* available,
//	                 which is the conservative direction for a reach shortfall.
//	RankSwaps        the single-swap frontier (k = 1).
//	RankPairs        the funded-premium family, one upgrade against n sales.
//	Optimize         the general bounded revision, with the pool floor removed so
//	                 its space contains bespoke's.
//
// Which source supplied the winner is reported, because "the general search never
// wins" and "the general search wins and bespoke cannot take it" are different
// findings.
//
// # Approximations, each recorded because it would matter if these were policy
// numbers
//
//   - **Selling prices are one gameweek stale.** decide() prices at gw-1;
//     Week.Sell for the week the decision is made *from* is priced at gw-2, and
//     that is the freshest real selling value SimResult carries. Market prices
//     would be worse — they hand the search money FPL does not give you. The
//     self-check below is what detects it if it matters.
//   - **The money term on the gain side is omitted**, so the test refuses to run
//     unless budgetWeight is zero, which is what ships.
//
// # The self-check on the duplicated gate
//
// This file mirrors decide()'s acceptance arithmetic, and a mirror is this
// package's signature bug class. So it is *checked* rather than merely watched:
// in every week where the shipped policy made exactly one move, that move must
// pass this file's single-step gate at the squad it was made from. The agreement
// rate is printed, and anything below 100% means the reconstruction — sell
// prices, free-transfer count, or the gate itself — has drifted and no other
// number here should be believed.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// reachKMax is how many changes are enumerated. Three is the question as posed;
// beyond it the permutation count and the Optimize calls both grow while the
// recorded policy makes three-or-more-move weeks five times in four seasons.
const reachKMax = 3

// premiumPrice is the price at which the objective-divergence work located its
// damage, and the bucket this diagnostic breaks reach down by.
const premiumPrice = 9.0

// assetKey identifies one player in one season.
//
// Element ids are reassigned every summer, so an id alone is not a player; a display
// name is not one either, for the reasons set out on reachCase.inAssets. Within a
// season the id is exact, and within-season is the unit the reach question is about.
// The name is deliberately NOT a field: this struct is used as a map key, so a
// display-only field in it would be part of the identity whatever a comment claimed,
// and the whole point of the type is that names do not identify players. Names are
// looked up separately for printing.
type assetKey struct {
	season string
	id     int
}

// reachCase is one (decision point, package size) question.
type reachCase struct {
	season string
	start  int
	gw     int
	// moves is how many players the best revision of this size changes.
	moves int
	// gain is the package's XIValue delta per gameweek; shippedGain is the same
	// quantity for what the policy actually did that week.
	gain, shippedGain float64
	// worth is whether the package clears the group bar bespoke applies to a
	// funded pair — see the header. Only worth packages are in the primary
	// population.
	worth bool
	// reachSteps is reachability by individually-accepted single swaps.
	//
	// pairProposed is whether RankPairs offers this exact package anywhere in its
	// ranking, which is the question of whether the bespoke structure can express
	// it at all. pairChosen is the stricter version: RankPairs ranks it *first*,
	// which is the only pair decide() ever looks at. The gap between them is
	// search truncation rather than a gate refusal, and conflating the two would
	// credit the policy with reaching packages it never evaluates.
	reachSteps, pairProposed, pairChosen bool
	// hybridProposed and hybridChosen are the route decide() ACTUALLY walks and
	// that this file originally had no representation of: one funded pair, and
	// then the singles loop spending whatever allowance is left.
	//
	// decide() runs the pair branch and then falls through to `for range limit`
	// with `limit -= n` — so a three-move week is routinely a two-move pair plus
	// one individually-accepted single, and it is NOT any pair RankPairs offers
	// (a pair is one upgrade against n sales, so a pair-plus-single is not one).
	// Before these fields existed such a package could only be reached if all
	// three legs cleared the gate alone, which the funding leg never does — that
	// is the entire reason the pair mechanism exists. It read as "gate", and it
	// broke the taken-package anchor: 2025-26 start 1 gw 19 is B.Fernandes→Saka
	// funded by J.Timber→Thiaw (a pair, combined +1.585) plus Mbeumo→Cunha
	// (+0.687 on its own), and the funding leg is +0.395 against a MinGain of
	// 0.4. The policy took it; this file called it unreachable.
	//
	// hybridProposed uses a pair at any rank, hybridChosen the top-ranked one,
	// mirroring pairProposed and pairChosen. The whole-package case (the pair IS
	// the package) stays in those fields, so the two existing columns keep
	// meaning exactly what they meant.
	hybridProposed, hybridChosen bool
	// taken is whether this package is exactly what the policy did that week. It
	// is the anchor on the whole measurement: a taken package must be reachable,
	// and its forgone value must be zero.
	taken bool
	// blocked is "", "club", "money", "club+money" or "gate".
	blocked string
	// source is which search supplied the winning package: "greedy", "swaps",
	// "pairs" or "optimize".
	source     string
	maxInPrice float64
	// inAssets identifies the incoming players so unreachability can be counted per
	// ASSET rather than per decision point.
	//
	// The rate over decision points is the wrong denominator for the question that
	// matters. FPL has maybe fifty assets anyone would actually want, and the same
	// premium recurs across many weeks — so one asset being systematically
	// unreachable bites repeatedly through a season while still reading as "5 of
	// 53". A per-week rate cannot distinguish five different assets missed once
	// from one asset missed five times, and those are very different problems.
	//
	// # The key is (season, element id) and NOT the display name
	//
	// The first version of this keyed on `Name` and it was wrong in both directions
	// at once, which is worth recording because it is a bug class this file already
	// carries twice. Verified against the archive:
	//
	//   2022-23 web_name "Álvarez"    = code 461358 = Julián Álvarez
	//   2023-24 web_name "J.Alvarez"  = code 461358 = the SAME footballer, renamed
	//   2023-24 web_name "Álvarez"    = code 213999 = Edson Álvarez, someone else
	//
	// So a name key SPLIT one player across seasons and COLLIDED two players inside
	// one. That is why overrides here are keyed on the permanent player code, and it
	// is the same shape as `FindPlayers` once returning Rodrigo Bentancur for "Rodri".
	//
	// The permanent code would be the ideal key and `PlayerMetrics` does not carry
	// it; plumbing it through a shipped struct for a diagnostic is the wrong trade,
	// and it would owe a `playerRow` field by this project's own rule. (season,
	// element id) is unambiguous *within* a season, which is the honest unit anyway
	// — a decision point lives in one season, and the ~50-asset universe the question
	// is about is a within-season universe. The cost is that the same footballer in
	// two seasons counts twice, which understates concentration rather than
	// inventing it.
	inAssets []assetKey
}

// reachable is whether the bespoke structure can get to this package at all,
// either by individually-accepted steps or as a funded pair it can express.
//
// This is deliberately the *permissive* reading: it counts a pair RankPairs ranks
// twentieth, which decide() would never look at. Permissive is the conservative
// direction for the claim being tested — a shortfall found against this bound is
// a real shortfall, and "reach is always available" measured this way is the
// weaker of the two statements. reachedStrict is the other end of the bracket.
func (c reachCase) reachable() bool {
	return c.reachSteps || c.pairProposed || c.hybridProposed
}

// reachedStrict is reachability by routes decide() actually evaluates: the
// singles ladder, the single top-ranked funded pair, or that pair followed by
// individually-accepted singles.
//
// It is a LOWER bound and it pays for a recorded approximation. Rank 0 is a
// property of RankPairs' gain ordering, and this file feeds RankPairs sell
// prices one gameweek stale — Week.Sell for the week a decision is made from is
// priced at gw-2 where decide() prices at gw-1. A tenth of a million moves a
// pair off rank 0, or takes it out of the pair family entirely by making the
// upgrade affordable as a single swap. So the strict column can miss a package
// the policy demonstrably took, which is why the taken-package anchor is
// asserted against reachable() and the strict rate on taken packages is
// reported as a number instead.
func (c reachCase) reachedStrict() bool {
	return c.reachSteps || c.pairChosen || c.hybridChosen
}

func (c reachCase) forgone() float64 { return c.gain - c.shippedGain }

func TestDiagGroupedMoveReach(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	if budgetWeight != 0 {
		t.Skipf("FPL_BUDGET_WEIGHT=%v: the gate reconstruction here omits the "+
			"money term, so it would silently test a different gate", budgetWeight)
	}
	// RankSwaps and RankPairs score the incoming player through
	// discountIncoming, so their Gain is not XIValue's delta unless the discount
	// is zero — which is what ships. stepGain and modelledGain here rescore the
	// squad undiscounted, so a non-zero discount would apply the gate to a
	// different quantity than decide() applies it to, in the direction that makes
	// reach look more available. Refused rather than warned about, because the
	// self-check below would only catch it if it happened to flip a decision.
	if analysis.BuyDiscount() != 0 {
		t.Skipf("FPL_BUY_DISCOUNT=%v: the gain this file computes is XIValue's "+
			"delta, which is only the gate's own quantity at a zero discount",
			analysis.BuyDiscount())
	}
	if unifiedTransfers {
		t.Skip("FPL_UNIFIED_TRANSFERS: decide() takes the unified branch, so the " +
			"bespoke gate mirrored here is not the gate that ran")
	}
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	// Display names for the asset table, keyed the same way. Separate from the
	// identity on purpose — see assetKey.
	assetNames := map[assetKey]string{}

	// The full season grid, always. This diagnostic pays for up to reachKMax
	// Optimize calls at every decision point, so it is expensive and the
	// temptation is a per-diagnostic switch that trims the seasons. That is
	// exactly what TestTheGridIsDeclaredOnce exists to prevent — a diagnostic
	// able to measure a different season population from the sweeps it is quoted
	// beside, silently. Cost is controlled on the *paths* instead, through
	// FPL_SWEEP_STARTS, which is the established mechanism and is fingerprinted.
	pairs := sweepPairNames()
	starts := sweepStarts()

	var all []reachCase
	// The self-check: single-move weeks the shipped policy actually took, and
	// how many of them this file's gate reconstruction accepts.
	singleWeeks, singleAgreed := 0, 0
	decisions := 0

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)

		for _, start := range starts {
			// A sweep config, because the population being described is a
			// contrast between decision rules rather than what a season would
			// have scored — the same reason every paired sweep pins the modern
			// five-transfer bank. Stated rather than implied: 2022-23 and
			// 2023-24 are replayed under a bank they did not have, which keeps
			// the weekly allowance comparable across cells at the cost of
			// historical fidelity in two of them.
			base := sweepConfig(cfg, start, false)
			res, err := Simulate(cur, prior, base)
			if err != nil {
				t.Fatal(err)
			}
			byGW := map[int][]Move{}
			for _, mv := range res.Moves {
				byGW[mv.GW] = append(byGW[mv.GW], mv)
			}

			for wi := 1; wi < len(res.Weeks); wi++ {
				wk := res.Weeks[wi]
				prev := res.Weeks[wi-1]
				gw := wk.GW
				held := prev.Squad
				if len(held) != analysis.SquadSize || len(wk.Squad) != analysis.SquadSize {
					continue
				}
				bank := prev.Bank
				sell := prev.Sell

				pb, pf := PointInTime(cur, prior, gw-1)
				e := analysis.NewEngineFull(pb, pf, cfg.Weights,
					analysis.Congestion{}, analysis.RoleRisk{})
				e.Priors = idx
				e.Recent = newRecentIndexWith(cur, gw-1,
					base.minutesHalfLife(), cfg.Weights.RateHalfLife)

				decisionH := base.DecisionHorizon
				if decisionH <= 0 {
					decisionH = cfg.Weights.Horizon
				}
				horizon := effectiveHorizon(decisionH, gw)
				// The allowance decide() would have, reconstructed the way
				// decide() computes it: the count carried out of last week, plus
				// one, capped at the bank limit.
				free := prev.Free
				if free < base.BankUpTo {
					free++
				}
				limit := moveLimit(free, base.MaxHits, base.MaxMoves)
				if limit < 1 {
					continue
				}
				decisions++

				byID := map[int]analysis.PlayerMetrics{}
				for _, m := range e.AllMetrics() {
					byID[m.ID] = m
				}
				market := func(id int) int { return marketPrice(cur, id, gw-1) }
				shipped := byGW[gw]
				shippedGain := modelledGain(e, held, shipped)

				if len(shipped) == 1 {
					singleWeeks++
					g := stepGain(e, held, shipped[0])
					if reachGateAccepts(g, horizon, free > 0, 0, base) {
						singleAgreed++
					}
				}

				best, soloGain, pairSets := bestPackages(e, byID, held, bank, limit,
					gw, base, sell, market)
				// The alternative to any package is never "do nothing": it is to
				// spend the free transfer on the best single move and keep the
				// four points. Same arithmetic as decide()'s pair branch.
				soloValue := 0.0
				if soloGain >= base.MinGain && soloGain*horizon >= base.FreeCost {
					soloValue = soloGain*horizon - base.FreeCost
				}

				for n := 1; n <= reachKMax; n++ {
					pkg, ok := best[n]
					if !ok {
						continue
					}
					c := reachCase{
						season: pair[1], start: start, gw: gw,
						moves:       n,
						gain:        pkg.gain,
						shippedGain: shippedGain,
						source:      pkg.source,
						worth: groupWorthTaking(pkg.gain, n, free, limit,
							horizon, soloValue, base),
					}
					for _, mv := range pkg.moves {
						if p := byID[mv.InID].Price; p > c.maxInPrice {
							c.maxInPrice = p
						}
						c.inAssets = append(c.inAssets,
							assetKey{season: pair[1], id: mv.InID})
						assetNames[assetKey{season: pair[1], id: mv.InID}] = byID[mv.InID].Name
					}
					c.taken = len(shipped) == n && sameMoves(shipped, pkg.moves)
					if n < 2 {
						// A one-move optimum is reachable by definition if it
						// clears the gate. Recorded so the denominator is honest,
						// and because it must agree with what the policy did.
						c.reachSteps = reachGateAccepts(c.gain, horizon, free > 0, 0, base)
						if !c.reachSteps {
							c.blocked = "gate"
						}
						all = append(all, c)
						continue
					}
					for i, set := range pairSets {
						if sameMoves(set, pkg.moves) {
							c.pairProposed = true
							// Rank zero is the only pair bestPair hands decide().
							c.pairChosen = i == 0
							break
						}
					}
					c.reachSteps, c.blocked = reachBySteps(e, byID, held, pkg.moves,
						bank, free, horizon, base, sell, market)
					c.hybridProposed, c.hybridChosen = reachByPairThenSteps(e, byID,
						held, pkg.moves, pairSets, bank, free, horizon, base, sell,
						market)
					if c.reachable() {
						c.blocked = ""
					}
					all = append(all, c)
				}
			}
		}
	}

	if decisions == 0 {
		t.Skip("no decision points")
	}

	fmt.Printf("\nREACH: is the best multi-move revision reachable by moves each\n")
	fmt.Printf("of which the shipped gate would accept on its own?\n\n")
	fmt.Printf("%d decision points, %d cells (%d season pairs x %d entry points),\n",
		decisions, len(pairs)*len(starts), len(pairs), len(starts))
	fmt.Printf("path canonical: the shipped bespoke policy plays the season and every\n")
	fmt.Printf("question is asked at the squad it actually holds. Nothing is applied.\n\n")

	agreement := 0.0
	if singleWeeks > 0 {
		agreement = 100 * float64(singleAgreed) / float64(singleWeeks)
	}
	fmt.Printf("self-check: of %d weeks where the policy made exactly one move, this\n",
		singleWeeks)
	fmt.Printf("file's gate reconstruction accepts %d (%.1f%%). Below 100%% means the\n",
		singleAgreed, agreement)
	fmt.Printf("reconstruction has drifted and nothing below should be believed.\n")
	// Asserted, not printed. A validation step written as "this should be exact"
	// rather than "assert this is exact" finds nothing, which is a failure this
	// project has already paid for once — the tie-rule check that found the
	// duplicate archive rows only worked because it was an assertion.
	//
	// And asserted FATALLY, before a single table is printed. An earlier version
	// used t.Errorf, which fails the run and prints everything anyway — and both
	// runs whose tables were written into AGENTS.md were failing the second anchor
	// below. The tables were read by grepping for headings and the exit status was
	// never looked at, so a self-check that says "nothing below should be
	// believed" while printing the numbers below it is not a self-check. Every
	// failure is collected and named first, because the cases are the diagnosis
	// material, and then the run stops without producing anything quotable.
	var broken []string
	if singleWeeks > 0 && singleAgreed != singleWeeks {
		broken = append(broken, fmt.Sprintf("gate reconstruction drifted: %d of %d "+
			"single-move weeks the policy took are refused by this file's gate",
			singleWeeks-singleAgreed, singleWeeks))
	}
	// The second anchor, and the one that would catch a wrong package rather than
	// a wrong gate: a package the policy actually took must be reachable by
	// construction, and its forgone value must be zero.
	//
	// It is checked against the PERMISSIVE bound and not the strict one, which is
	// a distinction with a reason rather than a relaxation to make it pass. See
	// the note on price staleness in reachedStrict.
	takenMulti, takenStrict := 0, 0
	for _, c := range all {
		if !c.taken {
			continue
		}
		if c.moves >= 2 {
			takenMulti++
			if c.reachedStrict() {
				takenStrict++
			}
		}
		if !c.reachable() {
			broken = append(broken, fmt.Sprintf("%s start %d gw %d: the policy took "+
				"this %d-move package and this file calls it unreachable (%s)",
				c.season, c.start, c.gw, c.moves, c.blocked))
		}
		if d := c.forgone(); d < -1e-9 || d > 1e-9 {
			broken = append(broken, fmt.Sprintf("%s start %d gw %d: the policy took "+
				"this %d-move package and its forgone value is %+.6f rather than 0 "+
				"— the package gain and the shipped gain are not the same quantity",
				c.season, c.start, c.gw, c.moves, d))
		}
	}
	if len(broken) > 0 {
		for _, b := range broken {
			t.Log(b)
		}
		t.Fatalf("%d self-check failures listed above. Every number in this "+
			"diagnostic is computed through the reconstruction they check, so no "+
			"table is printed — there is nothing here worth quoting until they "+
			"are 0.", len(broken))
	}
	if takenMulti > 0 {
		fmt.Printf("\nsecond self-check: of %d multi-move packages the policy actually\n",
			takenMulti)
		fmt.Printf("took, all %d are inside the permissive bound (asserted above) and\n",
			takenMulti)
		fmt.Printf("%d are also inside the strict one. That gap is this file's sell\n",
			takenStrict)
		fmt.Printf("prices being one gameweek stale, which reorders RankPairs by a\n")
		fmt.Printf("tenth of a million and moves the pair decide() took off rank 0 —\n")
		fmt.Printf("so read 'strict' as a lower bound that pays for that staleness.\n")
	}

	pick := func(want func(reachCase) bool) []reachCase {
		var rows []reachCase
		for _, c := range all {
			if want(c) {
				rows = append(rows, c)
			}
		}
		return rows
	}
	worthN := func(n int) func(reachCase) bool {
		return func(c reachCase) bool { return c.worth && c.moves == n }
	}
	w2 := pick(worthN(2))
	w3 := pick(worthN(3))
	wMulti := pick(func(c reachCase) bool { return c.worth && c.moves >= 2 })

	head := func() {
		fmt.Printf("%-30s %6s %8s %8s %8s %8s %8s %8s %9s %9s\n",
			"", "n", "reach", "strict", "steps", "pair@0", "pair@n", "pair+1",
			"mean gain", "mean fgn")
	}

	weeksWorth := map[string]bool{}
	for _, c := range wMulti {
		weeksWorth[fmt.Sprintf("%s|%d|%d", c.season, c.start, c.gw)] = true
	}
	fmt.Printf("\n%d of %d decision points (%.1f%%) have a multi-move optimum worth\n",
		len(weeksWorth), decisions, 100*float64(len(weeksWorth))/float64(decisions))
	fmt.Printf("taking at some size — that is the population the reach question is about.\n")

	fmt.Printf("\nPRIMARY — multi-move optima that clear the group bar, i.e. packages\n")
	fmt.Printf("the policy would want. The two reach columns bracket the answer:\n")
	fmt.Printf("  reach   permissive — steps, a funded pair at ANY rank, or a pair at\n")
	fmt.Printf("          any rank followed by individually-accepted singles\n")
	fmt.Printf("  strict  the same three routes, with the pair restricted to the ONE\n")
	fmt.Printf("          top-ranked pair decide() looks at\n")
	fmt.Printf("The gap between them is search truncation in bestPair, not a gate\n")
	fmt.Printf("refusal. The route columns are the three decide() actually walks:\n")
	fmt.Printf("  steps   the gate question proper — is there an ordering every leg of\n")
	fmt.Printf("          which the singles loop would accept on its own?\n")
	fmt.Printf("  pair@0  the whole package is the top-ranked funded pair\n")
	fmt.Printf("  pair@n  the whole package is a funded pair at any rank\n")
	fmt.Printf("  pair+1  a funded pair covers PART of it and the singles loop accepts\n")
	fmt.Printf("          the rest, which is what decide() does after `limit -= n`.\n")
	fmt.Printf("          Zero at 2 moves by construction: there the pair is the whole\n")
	fmt.Printf("          package. This route was missing until it broke the anchor.\n")
	head()
	reachSummarise("  2-move optimum", w2)
	reachSummarise("  3-move optimum", w3)
	reachSummarise("  multi-move (2 or 3)", wMulti)

	fmt.Printf("\nwhy an unreachable worth-taking package is unreachable:\n")
	fmt.Printf("%-30s %7s %7s %7s %10s %7s\n",
		"", "unreach", "club", "money", "club+money", "gate")
	for _, b := range []struct {
		label string
		rows  []reachCase
	}{{"  2-move optimum", w2}, {"  3-move optimum", w3}, {"  multi-move", wMulti}} {
		var un, club, money, both, gate int
		for _, c := range b.rows {
			if c.reachable() {
				continue
			}
			un++
			switch c.blocked {
			case "club":
				club++
			case "money":
				money++
			case "club+money":
				both++
			case "gate":
				gate++
			}
		}
		fmt.Printf("%-30s %7d %7d %7d %10d %7d\n", b.label, un, club, money, both, gate)
	}
	fmt.Printf("\nthe money column is predicted to be 0 by arithmetic (order the\n")
	fmt.Printf("money-freeing legs first and every intermediate bank lies between the\n")
	fmt.Printf("opening and closing one). A non-zero entry is a budget finding, not a\n")
	fmt.Printf("reach one. club+money is a genuine interaction and is not that.\n")

	fmt.Printf("\nwhich search supplied the best worth-taking package of each size:\n")
	fmt.Printf("%-30s %6s %8s %8s %8s %8s\n",
		"", "n", "greedy", "swaps", "pairs", "optimize")
	for _, b := range []struct {
		label string
		rows  []reachCase
	}{{"  2-move optimum", w2}, {"  3-move optimum", w3}} {
		counts := map[string]int{}
		for _, c := range b.rows {
			counts[c.source]++
		}
		fmt.Printf("%-30s %6d %8d %8d %8d %8d\n", b.label, len(b.rows),
			counts["greedy"], counts["swaps"], counts["pairs"], counts["optimize"])
	}

	fmt.Printf("\nworth-taking multi-move reach by the most expensive player bought:\n")
	head()
	var cheap, premium []reachCase
	for _, c := range wMulti {
		if c.maxInPrice >= premiumPrice {
			premium = append(premium, c)
		} else {
			cheap = append(cheap, c)
		}
	}
	reachSummarise(fmt.Sprintf("  under %.1fm", premiumPrice), cheap)
	reachSummarise(fmt.Sprintf("  %.1fm and up", premiumPrice), premium)

	// Reach by the package's OWN modelled gain, which is the cut that decides
	// whether the headline reach figure means anything.
	//
	// "97.6% of worth-taking two-move packages are reachable" is a marginal, and a
	// marginal is exactly the wrong statistic if unreachability concentrates at the
	// top. This project's central lesson is that a transfer search is an ARGMAX, so
	// it lives in the tail of the estimate distribution rather than the middle — a
	// structure that reaches 97.6% of packages while missing the best ones would be
	// worse than one that reaches 80% uniformly, and the two are indistinguishable
	// in the table above.
	//
	// Two marginals already hint that way and neither settles it: three-move
	// packages carry a higher mean gain than two-move ones (+3.211 against +2.409)
	// and reach far less often (63.6% against 97.6%), and premium packages reach
	// less often than cheap ones while being the only bucket whose forgone value is
	// positive. Both are confounded — size and price each travel with other things.
	// Gain is the quantity the objective actually maximises, so it is the honest cut.
	fmt.Printf("\nworth-taking multi-move reach by the package's OWN modelled gain,\n")
	fmt.Printf("which is the cut that decides whether the headline figure means\n")
	fmt.Printf("anything: a structure reaching 97%% of packages while missing the\n")
	fmt.Printf("BEST ones is worse than one reaching 80%% uniformly, and the two look\n")
	fmt.Printf("identical in every table above. Quartiles, worst gain first:\n")
	head()
	byGain := append([]reachCase(nil), wMulti...)
	sort.Slice(byGain, func(i, j int) bool { return byGain[i].gain < byGain[j].gain })
	for q := 0; q < 4; q++ {
		lo := q * len(byGain) / 4
		hi := (q + 1) * len(byGain) / 4
		if lo >= hi {
			continue
		}
		reachSummarise(fmt.Sprintf("  Q%d (gain %.2f to %.2f)", q+1,
			byGain[lo].gain, byGain[hi-1].gain), byGain[lo:hi])
	}

	// And the very top, printed per row rather than aggregated. A quartile of
	// thirteen still averages, and the question is about the extreme: if the single
	// most valuable package the policy could want is reachable, that is worth more
	// than any percentage.
	fmt.Printf("\nthe five highest-gain worth-taking packages, individually — the\n")
	fmt.Printf("extreme is the question, and a quartile still averages:\n")
	fmt.Printf("%-28s %6s %8s %8s %9s %9s\n",
		"", "moves", "reach", "steps", "gain", "forgone")
	for i := len(byGain) - 1; i >= 0 && i >= len(byGain)-5; i-- {
		c := byGain[i]
		fmt.Printf("  %-26s %6d %8v %8v %9.3f %9.3f\n",
			fmt.Sprintf("%s GW%d (start %d)", c.season, c.gw, c.start),
			c.moves, c.reachable(), c.reachSteps, c.gain, c.forgone())
	}

	// Per-ASSET concentration, which is the denominator the question actually wants.
	//
	// FPL has maybe fifty assets anyone would seriously want, and the same premium
	// recurs across many weeks. So the per-decision-point rate cannot distinguish
	// five different assets missed once each from ONE asset missed five times — and
	// the second is a systematic block on something we care about, where the first
	// is noise. If the pool of assets involved in worth-taking packages is small and
	// the unreachable set is concentrated inside it, the marginal rate understates
	// the problem badly.
	fmt.Printf("\nconcentration by ASSET-SEASON, which is the denominator that matters:\n")
	fmt.Printf("FPL has maybe fifty assets anyone would want and the same premium\n")
	fmt.Printf("recurs weekly, so a per-week rate cannot tell five assets missed once\n")
	fmt.Printf("from ONE asset missed five times. The second is systematic.\n")
	fmt.Printf("Keyed on (season, element id), NOT the display name — see assetKey.\n")
	inAll, inUnreach := map[assetKey]int{}, map[assetKey]int{}
	for _, c := range wMulti {
		for _, a := range c.inAssets {
			inAll[a]++
			if !c.reachable() {
				inUnreach[a]++
			}
		}
	}
	fmt.Printf("  distinct incoming asset-seasons across worth-taking packages: %d\n", len(inAll))
	fmt.Printf("  distinct incoming asset-seasons in the UNREACHABLE set:       %d\n", len(inUnreach))

	// Printed per asset, because "how concentrated" is a claim about which players
	// and a count cannot carry it. The systematic case is one row with a high
	// unreachable count against a high package count.
	// Forgone value per asset, over that asset's UNREACHABLE packages only.
	//
	// This is the column that separates the two readings of a concentrated block, and
	// they imply opposite actions:
	//
	//   forgone NEGATIVE — the packages wanting this asset were worth less than what
	//     the policy did instead, so the gate refused something the model OVER-VALUED
	//     and the lever is the valuation.
	//   forgone POSITIVE — there was real value the structure could not express, which
	//     is genuine complementarity: a funding downgrade that loses on its own and an
	//     upgrade that more than repays it. The lever is then the structure.
	//
	// A mean over every unreachable package cannot separate them, and reporting only
	// the pooled mean is how "the gate refuses what the model over-values" got asserted
	// for an asset nobody had checked. Salah scored 344 points in 2024-25, so for that
	// row the over-valuation reading is not obviously the right one.
	//
	// The unreachable count is also split by PACKAGE SIZE, because the two sizes
	// have different standing. Two-move reach has replicated at 97.6% across every
	// grid this has run on, and two-move reach cannot involve the pair+single route
	// at all. Three-move reach is the figure that was wrong before
	// reachByPairThenSteps existed, and it is the thinner population. So a row whose
	// misses are all at three moves is a weaker claim than one that loses two-move
	// packages, and the headline per-asset finding is only as solid as its split.
	fgnUnreach := map[assetKey]float64{}
	un2, un3 := map[assetKey]int{}, map[assetKey]int{}
	for _, c := range wMulti {
		if c.reachable() {
			continue
		}
		for _, a := range c.inAssets {
			fgnUnreach[a] += c.forgone()
			if c.moves == 2 {
				un2[a]++
			} else {
				un3[a]++
			}
		}
	}

	type nc struct {
		a      assetKey
		n, u   int
		u2, u3 int
		fgn    float64
	}
	var byAsset []nc
	for a, k := range inAll {
		byAsset = append(byAsset, nc{a, k, inUnreach[a], un2[a], un3[a], fgnUnreach[a]})
	}
	sort.Slice(byAsset, func(i, j int) bool {
		if byAsset[i].u != byAsset[j].u {
			return byAsset[i].u > byAsset[j].u
		}
		if byAsset[i].n != byAsset[j].n {
			return byAsset[i].n > byAsset[j].n
		}
		// Deterministic: map iteration order is randomised, and this table SELECTS
		// which rows to print. The recorded clean-sheet diagnostic produced an
		// unstable published figure for exactly this reason — accumulation over a
		// map is safe, selection is not.
		if byAsset[i].a.season != byAsset[j].a.season {
			return byAsset[i].a.season < byAsset[j].a.season
		}
		return byAsset[i].a.id < byAsset[j].a.id
	})
	fmt.Printf("\n  forgone is summed over this asset's UNREACHABLE packages: negative\n")
	fmt.Printf("  means the policy did BETTER than the package it could not reach, so\n")
	fmt.Printf("  the gate refused something over-valued; positive means real value the\n")
	fmt.Printf("  structure cannot express. The two imply opposite fixes.\n")
	fmt.Printf("  'at 2' and 'at 3' split the unreachable count by package size. Two-\n")
	fmt.Printf("  move reach has replicated at 97.6%% on every grid and cannot involve\n")
	fmt.Printf("  the pair+single route; three-move is the thinner, weaker population.\n")
	fmt.Printf("\n  %-22s %-10s %10s %12s %6s %6s %10s\n",
		"asset", "season", "packages", "unreachable", "at 2", "at 3", "forgone")
	for i, a := range byAsset {
		if i >= 14 && a.u == 0 {
			break
		}
		fmt.Printf("  %-22s %-10s %10d %12d %6d %6d %10.3f\n",
			assetNames[a.a], a.a.season, a.n, a.u, a.u2, a.u3, a.fgn)
	}

	fmt.Printf("\ndistribution of modelled gain forgone, per gameweek, over the\n")
	fmt.Printf("unreachable worth-taking multi-move packages (package gain minus what\n")
	fmt.Printf("the policy actually did that week):\n")
	var forgone []float64
	for _, c := range wMulti {
		if !c.reachable() {
			forgone = append(forgone, c.forgone())
		}
	}
	printForgone(forgone)

	fmt.Printf("\nFOR COMPLETENESS — the same questions over every multi-move optimum,\n")
	fmt.Printf("including those below the group bar. Reach here is near zero by\n")
	fmt.Printf("construction: XIValue gain is monotone in adding an improving move, so\n")
	fmt.Printf("the best k-move package is usually the best (k-1)-move one plus a\n")
	fmt.Printf("marginal leg the gate is right to refuse. Do not read it as lost value.\n")
	head()
	reachSummarise("  1-move optimum", pick(func(c reachCase) bool { return c.moves == 1 }))
	reachSummarise("  2-move optimum", pick(func(c reachCase) bool { return c.moves == 2 }))
	reachSummarise("  3-move optimum", pick(func(c reachCase) bool { return c.moves == 3 }))

	grid := fmt.Sprintf("%d season pairs x %d entry points, %d decision points",
		len(pairs), len(starts), decisions)
	sink.emitAll("grouped_move_reach", grid, "gate reconstruction self-check",
		singleWeeks, measure{"agreement percent", agreement})
	for _, b := range []struct {
		label string
		rows  []reachCase
	}{
		{"worth-taking 2-move optimum", w2},
		{"worth-taking 3-move optimum", w3},
		{"worth-taking multi-move (2 or 3)", wMulti},
		{fmt.Sprintf("worth-taking multi-move, under %.1fm", premiumPrice), cheap},
		{fmt.Sprintf("worth-taking multi-move, %.1fm and up", premiumPrice), premium},
		{"every 2-move optimum", pick(func(c reachCase) bool { return c.moves == 2 })},
		{"every 3-move optimum", pick(func(c reachCase) bool { return c.moves == 3 })},
	} {
		if len(b.rows) == 0 {
			continue
		}
		sink.emitAll("grouped_move_reach", grid, b.label, len(b.rows),
			reachMeasures(b.rows)...)
	}
}

func printForgone(forgone []float64) {
	if len(forgone) == 0 {
		fmt.Printf("  none — every such package was reachable.\n")
		return
	}
	sort.Float64s(forgone)
	q := func(p float64) float64 { return forgone[int(p*float64(len(forgone)-1))] }
	var sum float64
	for _, v := range forgone {
		sum += v
	}
	fmt.Printf("  n %d   mean %+.3f   min %+.3f   p25 %+.3f   median %+.3f   "+
		"p75 %+.3f   max %+.3f\n", len(forgone), sum/float64(len(forgone)),
		forgone[0], q(0.25), q(0.5), q(0.75), forgone[len(forgone)-1])
}

// reachSummarise prints one row: how often the optimum was reachable, by which
// route, and what it and the shortfall were worth.
func reachSummarise(label string, rows []reachCase) {
	if len(rows) == 0 {
		fmt.Printf("%-30s %6d\n", label, 0)
		return
	}
	n := float64(len(rows))
	var reach, strict, steps, chosen, pair, hyb int
	var gain, fgn float64
	for _, c := range rows {
		if c.reachable() {
			reach++
		}
		if c.reachedStrict() {
			strict++
		}
		if c.reachSteps {
			steps++
		}
		if c.pairChosen {
			chosen++
		}
		if c.pairProposed {
			pair++
		}
		if c.hybridProposed {
			hyb++
		}
		gain += c.gain
		fgn += c.forgone()
	}
	pc := func(k int) float64 { return 100 * float64(k) / n }
	fmt.Printf("%-30s %6d %7.1f%% %7.1f%% %7.1f%% %7.1f%% %7.1f%% %7.1f%% %+9.3f %+9.3f\n",
		label, len(rows), pc(reach), pc(strict), pc(steps), pc(chosen), pc(pair),
		pc(hyb), gain/n, fgn/n)
}

func reachMeasures(rows []reachCase) []measure {
	n := float64(len(rows))
	var reach, strict, steps, chosen, pair, hyb, club, money, both, gate int
	var gain, fgn float64
	for _, c := range rows {
		if c.reachedStrict() {
			strict++
		}
		if c.pairChosen {
			chosen++
		}
		if c.reachable() {
			reach++
		} else {
			switch c.blocked {
			case "club":
				club++
			case "money":
				money++
			case "club+money":
				both++
			case "gate":
				gate++
			}
		}
		if c.reachSteps {
			steps++
		}
		if c.pairProposed {
			pair++
		}
		if c.hybridProposed {
			hyb++
		}
		gain += c.gain
		fgn += c.forgone()
	}
	return []measure{
		{"reachable percent", 100 * float64(reach) / n},
		{"reachable by routes decide evaluates percent", 100 * float64(strict) / n},
		{"reachable by steps percent", 100 * float64(steps) / n},
		{"top-ranked funded pair percent", 100 * float64(chosen) / n},
		{"proposed by funded pair at any rank percent", 100 * float64(pair) / n},
		{"reachable as a funded pair then singles percent", 100 * float64(hyb) / n},
		{"blocked by club limit", float64(club)},
		{"blocked by money", float64(money)},
		{"blocked by club and money together", float64(both)},
		{"blocked by gate", float64(gate)},
		{"mean package gain", gain / n},
		{"mean gain forgone", fgn / n},
	}
}

// reachPackage is one candidate revision and what the transfer objective makes
// of it.
type reachPackage struct {
	moves  []Move
	gain   float64
	source string
}

// bestPackages returns, for each move count up to reachKMax, the best revision
// any search in this codebase can find, scored by XIValue — plus the best single
// swap's gain, which is the alternative every group is judged against, and every
// package RankPairs proposed, which the caller needs for the expressiveness
// question and which is far too expensive to compute twice.
//
// The union matters more than any one member. Taking the k-optimum from Optimize
// alone measured reach into a space that is neither bespoke's nor scored on
// bespoke's objective, and produced a negative mean shortfall — the packages
// under test were worse than what the policy had already done. Including the
// greedy ladder in particular biases *toward* reach being available, which is the
// direction that makes a reach shortfall harder rather than easier to claim.
func bestPackages(e *analysis.Engine, byID map[int]analysis.PlayerMetrics,
	held []int, bank, limit, gw int, cfg SimConfig, sell map[int]int,
	market func(int) int) (map[int]reachPackage, float64, [][]Move) {

	out := map[int]reachPackage{}
	consider := func(source string, moves []Move) {
		n := len(moves)
		if n < 1 || n > reachKMax {
			return
		}
		g := modelledGain(e, held, moves)
		if cur, ok := out[n]; ok && cur.gain >= g {
			return
		}
		out[n] = reachPackage{moves: append([]Move(nil), moves...), gain: g, source: source}
	}

	pool := e.AllMetrics()
	soloGain := 0.0

	// The greedy ladder: exactly what the bespoke singles loop walks. Money and
	// selling prices are carried forward, since a player bought this week sells
	// for what was just paid for him.
	cur := append([]int(nil), held...)
	curBank := bank
	curSell := make(map[int]int, len(sell)+reachKMax)
	for k, v := range sell {
		curSell[k] = v
	}
	var ladder []Move
	// A player sold at one rung is no longer owned, so the next rung's search
	// would happily buy him back — a package that spends two transfers to change
	// nothing. Anyone already touched is skipped rather than the whole rung
	// abandoned, which is also what makes the ladder a fair comparison: the
	// bespoke loop re-reads the board between moves and would not undo itself.
	touched := map[int]bool{}
	for depth := 0; depth < reachKMax; depth++ {
		st := analysis.NewSquadState(squadMetrics(e, cur))
		st.Sell = curSell
		swaps := analysis.RankSwaps(st, pool, curBank)
		pick := -1
		for i, sw := range swaps {
			if !touched[sw.Out.ID] && !touched[sw.In.ID] {
				pick = i
				break
			}
		}
		if pick < 0 {
			break
		}
		mv := swapToMove(swaps[pick], gw)
		if depth == 0 {
			// The single-swap frontier, reported as its own source so "the best
			// one-move revision" is bespoke's own answer by construction.
			soloGain = swaps[pick].Gain
			consider("swaps", []Move{mv})
		}
		ladder = append(ladder, mv)
		consider("greedy", ladder)
		sellOut := market(mv.OutID)
		if v, ok := curSell[mv.OutID]; ok {
			sellOut = v
		}
		curBank += sellOut - market(mv.InID)
		curSell[mv.InID] = market(mv.InID)
		touched[mv.OutID], touched[mv.InID] = true, true
		cur = applyMove(cur, mv)
	}

	// The funded-premium family, which is bespoke's other route.
	pairSets := proposedPairs(e, held, bank, limit, gw, cfg, sell)
	for _, set := range pairSets {
		consider("pairs", set)
	}

	// The general bounded revision. The pool floor unifiedDecide applies is
	// removed here: handicapping this arm with a filter RankSwaps does not have
	// would measure the filter rather than the search, and AGENTS.md records the
	// floor as worth -40 inside this very search.
	value := 0
	for _, id := range held {
		if v, ok := sell[id]; ok {
			value += v
		} else {
			value += market(id)
		}
	}
	for k := 1; k <= reachKMax && k <= limit; k++ {
		got, err := e.Optimize(analysis.OptimizeRequest{
			Budget: value + bank, BenchWeight: 0,
			CurrentSquad: held, MaxChanges: k,
		})
		if err != nil {
			continue
		}
		if moves := diffSquads(held, byID, got.Players, gw); len(moves) > 0 {
			consider("optimize", moves)
		}
	}
	return out, soloGain, pairSets
}

// proposedPairs is every funded-premium package RankPairs offers from this
// squad, as move sets. It is the answer to "can the bespoke structure express
// this at all", which is separate from whether its acceptance rule takes it.
func proposedPairs(e *analysis.Engine, held []int, bank, limit, gw int,
	cfg SimConfig, sell map[int]int) [][]Move {

	if limit < 2 {
		return nil
	}
	maxDowns := limit - 1
	if cfg.MaxFundingSales > 0 && cfg.MaxFundingSales < maxDowns {
		maxDowns = cfg.MaxFundingSales
	}
	st := analysis.NewSquadState(squadMetrics(e, held))
	st.Sell = sell
	// Twenty rather than one: bestPair takes only the top-ranked pair, but the
	// question here is what the structure can reach, and a package it ranks
	// second is still one it can express.
	pairs := analysis.RankPairs(st, e.AllMetrics(), bank, maxDowns, 20)
	out := make([][]Move, 0, len(pairs))
	for _, p := range pairs {
		moves := []Move{swapToMove(p.Up, gw)}
		for _, d := range p.Downs {
			moves = append(moves, swapToMove(d, gw))
		}
		out = append(out, moves)
	}
	return out
}

// swapToMove is the one-line translation the replay's Move type needs.
func swapToMove(sw analysis.Swap, gw int) Move {
	return Move{
		GW: gw, Out: sw.Out.Name, In: sw.In.Name,
		OutID: sw.Out.ID, InID: sw.In.ID,
		OutScore: sw.Out.Score, InScore: sw.In.Score,
	}
}

// groupWorthTaking mirrors decide()'s pair-acceptance rule: a grouped move must
// clear MinGain, fit the allowance for at most one hit, and beat spending the
// free transfer on the best single move and keeping the four points.
//
// It is what separates "the objective likes this package" from "the policy would
// want this package", and without it reach is zero by construction — see the
// header.
func groupWorthTaking(gain float64, n, free, limit int,
	horizon, soloValue float64, cfg SimConfig) bool {

	if n > limit || gain < cfg.MinGain {
		return false
	}
	hitsNeeded := 0
	if free < n {
		hitsNeeded = n - free
	}
	if hitsNeeded > 1 {
		return false
	}
	value := gain*horizon - HitCost*float64(hitsNeeded) -
		cfg.FreeCost*float64(n-hitsNeeded)
	if value <= soloValue {
		return false
	}
	if hitsNeeded > 0 {
		return value-soloValue >= cfg.MinGainHit
	}
	return true
}

// reachGateAccepts mirrors the singles loop in decide(): a free transfer is
// spent when one is in hand, otherwise a hit is taken and has to clear
// MinGainHit after paying four points.
//
// The money term is omitted deliberately — budgetWeight ships at zero and the
// caller refuses to run otherwise, which is checked rather than assumed.
//
// This is a duplicate of shipped policy arithmetic, which is this package's
// signature bug class. It is checked every run against the moves the policy
// actually made; see the self-check in the header.
func reachGateAccepts(gain, horizon float64, haveFree bool, hitsUsed int,
	cfg SimConfig) bool {

	if haveFree {
		return gain >= cfg.MinGain && gain*horizon >= cfg.FreeCost
	}
	return hitsUsed < cfg.MaxHits && gain*horizon-HitCost >= cfg.MinGainHit
}

// stepGain is what one swap does to the eleven, per gameweek — the same quantity
// RankSwaps ranks on and the gate is applied to.
func stepGain(e *analysis.Engine, held []int, mv Move) float64 {
	before := analysis.XIValue(squadMetrics(e, held))
	return analysis.XIValue(squadMetrics(e, applyMove(held, mv))) - before
}

// reachBySteps reports whether some ordering of a package's moves is accepted
// leg by leg, and if not, which constraint every ordering ran into.
//
// Four passes rather than one, so the blocking reasons do not mask each other:
// club alone, money alone, the two together, then the gate on top. A single pass
// that broke on the first failure would report whichever constraint happened to
// bite first in whichever ordering was tried first, and collapsing club-and-money
// into "money" is what made the first run's money column non-zero.
func reachBySteps(e *analysis.Engine, byID map[int]analysis.PlayerMetrics,
	held []int, moves []Move, bank, free int, horizon float64, cfg SimConfig,
	sell map[int]int, market func(int) int) (bool, string) {

	sellOf := func(id int) int {
		if v, ok := sell[id]; ok {
			return v
		}
		return market(id)
	}
	clubs := map[string]int{}
	for _, id := range held {
		clubs[byID[id].Team]++
	}

	// walk applies one ordering and reports how far it got.
	walk := func(order []int, checkClub, checkMoney, checkGate bool) bool {
		squad := append([]int(nil), held...)
		c := make(map[string]int, len(clubs))
		for k, v := range clubs {
			c[k] = v
		}
		b := bank
		freeLeft, hitsUsed := free, 0
		for _, i := range order {
			mv := moves[i]
			c[byID[mv.OutID].Team]--
			c[byID[mv.InID].Team]++
			if checkClub && c[byID[mv.InID].Team] > analysis.MaxPerClub {
				return false
			}
			cost := market(mv.InID) - sellOf(mv.OutID)
			if checkMoney && cost > b {
				return false
			}
			if checkGate {
				g := stepGain(e, squad, mv)
				if !reachGateAccepts(g, horizon, freeLeft > 0, hitsUsed, cfg) {
					return false
				}
			}
			b -= cost
			if freeLeft > 0 {
				freeLeft--
			} else {
				hitsUsed++
			}
			squad = applyMove(squad, mv)
		}
		return true
	}

	orders := reachOrderings(len(moves))
	any := func(checkClub, checkMoney, checkGate bool) bool {
		for _, o := range orders {
			if walk(o, checkClub, checkMoney, checkGate) {
				return true
			}
		}
		return false
	}
	switch {
	case any(true, true, true):
		return true, ""
	case !any(true, false, false):
		return false, "club"
	case !any(false, true, false):
		return false, "money"
	case !any(true, true, false):
		return false, "club+money"
	default:
		return false, "gate"
	}
}

// reachByPairThenSteps is the route decide() actually walks, and the one this
// file was missing: one funded pair, then the singles loop spends what is left.
//
// # Why it has to be here
//
// decide() takes the pair branch, does `limit -= n`, and falls straight through
// into `for range make([]struct{}, limit)`. So a three-move week is routinely a
// two-move pair plus one single, and that is not expressible as ANY pair —
// RankPairs offers one upgrade against n sales, and a pair-plus-single is a
// different shape. Without this the only routes on offer were "every leg clears
// the gate alone" and "the whole package is one pair", and a pair-plus-single
// package satisfies neither: its funding leg is below MinGain by construction,
// which is the whole reason the pair mechanism exists.
//
// That is not a marginal omission. It made the taken-package anchor fail — the
// policy took a package this file called unreachable — and it is the leading
// candidate for the low three-move reach figure, since three is the smallest
// size at which pair-plus-single exists.
//
// # What is checked, and what is deliberately not
//
// The pair is applied as a BLOCK: FPL confirms a multi-transfer move as one
// batch and RankPairs itself checks affordability and club legality on the
// resulting squad, so the intermediate state inside a pair is not a state the
// game ever holds. Total cost must fit the bank and the post-pair squad must be
// club-legal. The trailing singles are then checked one at a time — bank, club
// and gate — because each is a separate confirmation, which is exactly what
// reachBySteps does.
//
// The pair leg is NOT required to clear the gate: no funded pair does, which is
// why it is a pair. Nor is it required to clear decide()'s pair-ACCEPTANCE rule.
// That is the same "proposing, not accepting" convention pairProposed already
// follows — the price question is answered once, at package level, by the
// worth-taking filter. Both choices are the permissive direction, which is the
// conservative one for a claim that reach is available.
//
// Returns (any rank, rank 0), mirroring pairProposed and pairChosen.
func reachByPairThenSteps(e *analysis.Engine, byID map[int]analysis.PlayerMetrics,
	held []int, moves []Move, pairSets [][]Move, bank, free int, horizon float64,
	cfg SimConfig, sell map[int]int, market func(int) int) (bool, bool) {

	// A pair plus a single needs at least three moves. At two, the pair IS the
	// package and pairProposed/pairChosen already answer it.
	if len(moves) < 3 {
		return false, false
	}
	sellOf := func(id int) int {
		if v, ok := sell[id]; ok {
			return v
		}
		return market(id)
	}
	key := func(mv Move) [2]int { return [2]int{mv.OutID, mv.InID} }
	inPkg := map[[2]int]int{}
	for i, mv := range moves {
		inPkg[key(mv)] = i
	}

	anyRank, rank0 := false, false
	for rank, set := range pairSets {
		if len(set) < 2 || len(set) >= len(moves) {
			continue
		}
		// The pair has to be a subset of this package, or taking it does not
		// move toward this package at all.
		used := make([]bool, len(moves))
		subset := true
		for _, mv := range set {
			i, ok := inPkg[key(mv)]
			if !ok {
				subset = false
				break
			}
			used[i] = true
		}
		if !subset {
			continue
		}

		// Apply the pair as one batch: the whole cost against the bank, and club
		// legality on the squad it leaves.
		squad := append([]int(nil), held...)
		clubs := map[string]int{}
		for _, id := range held {
			clubs[byID[id].Team]++
		}
		cost := 0
		for _, mv := range set {
			cost += market(mv.InID) - sellOf(mv.OutID)
			clubs[byID[mv.OutID].Team]--
			clubs[byID[mv.InID].Team]++
			squad = applyMove(squad, mv)
		}
		if cost > bank {
			continue
		}
		legal := true
		for _, n := range clubs {
			if n > analysis.MaxPerClub {
				legal = false
				break
			}
		}
		if !legal {
			continue
		}
		// The allowance the singles loop inherits, computed the way decide()
		// computes it: hits cover whatever the free transfers do not, and they
		// count against MaxHits for the rest of the week.
		n := len(set)
		hitsUsed := 0
		if free < n {
			hitsUsed = n - free
		}
		if hitsUsed > cfg.MaxHits {
			continue
		}
		freeLeft := free - (n - hitsUsed)
		if freeLeft < 0 {
			freeLeft = 0
		}

		var rest []int
		for i := range moves {
			if !used[i] {
				rest = append(rest, i)
			}
		}
		reached := false
		for _, order := range reachOrderings(len(rest)) {
			sq := append([]int(nil), squad...)
			c := make(map[string]int, len(clubs))
			for k, v := range clubs {
				c[k] = v
			}
			b := bank - cost
			fl, hu := freeLeft, hitsUsed
			ok := true
			for _, oi := range order {
				mv := moves[rest[oi]]
				c[byID[mv.OutID].Team]--
				c[byID[mv.InID].Team]++
				if c[byID[mv.InID].Team] > analysis.MaxPerClub {
					ok = false
					break
				}
				legCost := market(mv.InID) - sellOf(mv.OutID)
				if legCost > b {
					ok = false
					break
				}
				if !reachGateAccepts(stepGain(e, sq, mv), horizon, fl > 0, hu, cfg) {
					ok = false
					break
				}
				b -= legCost
				if fl > 0 {
					fl--
				} else {
					hu++
				}
				sq = applyMove(sq, mv)
			}
			if ok {
				reached = true
				break
			}
		}
		if !reached {
			continue
		}
		anyRank = true
		if rank == 0 {
			rank0 = true
		}
		if rank0 {
			break
		}
	}
	return anyRank, rank0
}

// reachOrderings is every permutation of 0..n-1. n is at most reachKMax, so this
// is six orderings at worst.
func reachOrderings(n int) [][]int {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return [][]int{{0}}
	}
	var out [][]int
	for i := 0; i < n; i++ {
		rest := make([]int, 0, n-1)
		for j := 0; j < n; j++ {
			if j != i {
				rest = append(rest, j)
			}
		}
		for _, sub := range reachOrderings(len(rest)) {
			order := []int{i}
			for _, s := range sub {
				order = append(order, rest[s])
			}
			out = append(out, order)
		}
	}
	return out
}
