package analysis

import (
	"math"
	"sort"
)

// Judging a change to an existing squad.
//
// Optimize builds a squad from nothing. Once a season is running the question is
// different — which one or two players to change — and answering it needs the
// same two things Optimize already knows, which every ad-hoc transfer search in
// this codebase has had to be taught separately:
//
//   - A change is worth what it does to the *eleven*, not to the player. A bench
//     player's points do not count, so upgrading a reserve who still will not
//     start buys nothing. Ranking by the player's own score gain sends transfers
//     to the cheap end of the squad and leaves declining starters in place.
//
//   - A premium cannot be reached one swap at a time. The downgrade that funds
//     him lowers the eleven on its own, so a search that only accepts improving
//     moves rejects it before the upgrade is ever considered. This is the same
//     problem dpseed.go solves for the opening squad.
//
// Both were measured on replayed seasons; see internal/backtest and the notes in
// AGENTS.md. Anything that proposes transfers should use these rather than roll
// its own comparison.

// XIValue is the objective a squad change is judged on: the best eleven this
// fifteen can field, plus the captain's score counted a second time.
//
// It is the same quantity Optimize maximises, which is the point — a weekly
// transfer search that optimises something else will disagree with the squad
// builder and be wrong in the same direction every week.
// The transfer search and the squad optimiser must value an eleven identically,
// or a move that improves one can look worse to the other. This delegates rather
// than reimplementing -- the duplicate previously drifted when the vice-captain
// term was added to only one of them.
// XIValue is what an eleven is worth, and it is the transfer search's objective.
//
// # Fixture load is applied here and nowhere else
//
// A club playing twice in a gameweek is worth roughly twice as much that week,
// and the model's per-gameweek Score does not know it. Scaling Score by
// FixtureLoad everywhere *loses*: it costs about 53 points a season on the held
// opening fifteen, because a squad picked before a ball is kicked trades present
// quality for a double that is months away and several transfer windows will
// have had a chance to move it first.
//
// But the *transfer* decision genuinely should see one coming — a double three
// weeks out is exactly the kind of thing a transfer is for, and confining the
// signal to the imminent gameweek leaves the weekly decision blind to it.
//
// Those two consumers separate cleanly here, and only here. XIValue is reached
// from RankSwaps, RankPairs, BuildPlans and the unified search — every one of
// them a transfer decision. Squad construction goes through the unexported
// xiValue via objective and bestXI, and the eleven is picked through BestXI, so
// neither sees this. That is why the adjustment sits in the exported wrapper
// rather than in Score: it is the one place where "what is this squad worth for
// a transfer decision" is asked and nothing else is.
//
// The double-count is refused per player rather than assumed away. This used to
// read "no double-count with the horizon-1 view … that path picks the eleven
// through BestXI and never calls this", which was true when written and false
// once SimConfig.AnticipateChips existed: it puts the *transfer* engine — the one
// that calls this — at horizon 1 in the gameweek before a chip, where
// FixtureLoadInScore() is true and Score already carries the multiplier. A
// doubling club was then valued 4x instead of 2x, in exactly the weeks the chip
// work cares about. See PlayerMetrics.loadInScore.
//
// **The guard is per player, so a squad assembled from two engines at different
// horizons is a silent mixture** — some members carrying the multiplier in Score
// and some not, each correct on its own and the sum meaning nothing. No caller
// does that today: every fifteen reaching here comes from one engine's
// `AllMetrics`. That used to be an unwritten *impossibility* and the loadInScore
// fact made it an unwritten *invariant*, which is a weaker thing and worth
// writing down.
func XIValue(squad []PlayerMetrics) float64 {
	return xiValueForTransfer(squad, ChipCredit{})
}

// ChipCredit is what a chip planned inside the decision's horizon adds to a
// squad's value, per gameweek.
//
// # Two of the four chips need the squad built toward them, and neither could say so
//
// The bench boost pays all fifteen and the triple captain pays the armband a
// third time, so both are worth more to *some* squads than others and both are
// therefore preparation problems: hold fifteen playable footballers into the boost
// week, own the right premium in the tripled one. The other two are not. A free
// hit fields a separate temporary fifteen and hands the squad back, so it asks
// nothing of the permanent squad — what it wants is for the permanent squad to
// ignore that week, which is Engine.ApplyFreeHitToScoring and is already wired.
// A wildcard *is* the rebuild, and what precedes it is a shortened horizon, which
// is Engine.EffectiveHorizon and is also already wired. Both of those reach the
// replay through SimConfig.anticipate.
//
// So the hole this closes is specific: `anticipate` calls ApplyChipPlan and
// throws away the bench weight it returns, because the transfer objective had
// nowhere to put it, and nothing anywhere expressed the triple captain at all.
//
// # Both are one week's payment spread over the horizon
//
// A squad serving H gameweeks, one of which carries the chip, earns the chip's
// premium once. So each field below is a *fraction of the horizon*: 1/H when the
// chip is inside it, zero otherwise. No constant is introduced by either.
//
// They are separate fields, and separately switchable in the replay, because
// folding two levers into one arm measures their sum and neither — the same
// mistake AnticipateGate exists to avoid.
type ChipCredit struct {
	// Bench is the fraction of the horizon the bench boost falls in.
	//
	// Everywhere else in this package a bench weight prices a *hedge*: how often
	// a substitute is actually used, which is what benchValue's slot
	// probabilities compute and why they are small. Under the chip the bench is
	// not a hedge — FPL pays all fifteen, with certainty — so the slot
	// probabilities are bypassed exactly as OptimizeRequest.BenchBoost bypasses
	// them, and the whole of the discount lives here.
	//
	// This is SuggestBenchWeight's amortisation minus its `base` term. The
	// transfer objective credits an ordinary bench at nothing, so carrying the
	// hedge value across too would be two changes at once and neither would be
	// measurable.
	Bench float64
	// Captain is the fraction of the horizon the triple captain falls in.
	//
	// The chip pays the armband once more than the ordinary week already does, so
	// the credit is one further copy of the armband xiValueShrunk already
	// computed, rather than a fresh reading of who the captain is. That reuse is
	// the point: one quantity with two implementations is this package's
	// signature failure.
	//
	// It takes the *shrunk* armband because this is the transfer objective, where
	// the captain term is pulled toward the runner-up — the search is an argmax
	// over players and pays full price for whichever premium it most over-rates.
	// ⚠️ **At shipped config that protection is dormant, and the measured arm did
	// not run under it.** `captainShrink` defaults to 1.0 (see sweep.go), and
	// xiValueShrunk shrinks only below 1, so shipped the credit is one further
	// copy of the *raw* armband and is exactly `Captain x max(XI score)`. It can
	// therefore only move when a candidate changes the squad's single highest
	// score, which is a much narrower set than "changes the captaincy" — and is
	// the mechanism behind the measured null.
	Captain float64

	// WeekLoad is how many fixtures each club plays in the chip's own gameweek,
	// keyed by the club name PlayerMetrics.Team carries. A doubling club is 2, a
	// blanking one 0, and anything absent from the map is 1.
	//
	// # Without it the credit buys a good bench, not a doubling one
	//
	// `PlayerMetrics.FixtureLoad` is matches per gameweek *averaged over the
	// horizon*, so on a five-week horizon a club that plays twice in the boost
	// week arrives at 6/5 = 1.2 rather than 2. Crediting `Score` directly
	// therefore priced bench *quality* and priced the double at a fifth of its
	// strength — which is not the mechanism the chip pays for, and is not what
	// "build toward the double" means. The first version of this did exactly
	// that, and the effect it measured is a floor for that reason rather than an
	// estimate.
	//
	// The bench term consequently strips the horizon average back out of each
	// player's score and re-applies this week's own count. Nil leaves the old
	// behaviour, which is what every caller that does not know the chip's week
	// gets.
	WeekLoad map[string]float64
}

func (c ChipCredit) zero() bool { return c.Bench <= 0 && c.Captain <= 0 }

// benchWeekScore is what a bench player is expected to return *in the chip's own
// gameweek*, given a score that may already carry the horizon-average fixture
// load.
//
// The two cases are exactly the ones xiValueForTransfer's adjustment creates: a
// score carries the load when the player already had it folded in
// (`loadInScore`) or when the transfer path has just applied it
// (`fixtureLoadTransfers`). In both, dividing by `FixtureLoad` recovers the
// load-free per-gameweek rate, which is what this week's own count multiplies.
func (c ChipCredit) benchWeekScore(p PlayerMetrics) float64 {
	if len(c.WeekLoad) == 0 {
		return p.Score
	}
	load, ok := c.WeekLoad[p.Team]
	if !ok {
		load = 1
	}
	base := p.Score
	// `FixtureLoad > 0` guards the DIVISION here rather than standing in for
	// `loadSet`, so both conditions are wanted: an unset load must not be divided
	// by, and a real zero must not either. A player who blanks the whole window
	// carries Score 0 when the load is in it, and this week's own count is then
	// the only thing that can lift him — which it does not, because a club that
	// blanks the window blanks this week too.
	if p.loadSet && p.FixtureLoad > 0 && (p.loadInScore || fixtureLoadTransfers) {
		base = p.Score / p.FixtureLoad
	}
	return base * load
}

// xiValueForTransfer is XIValue with a planned chip's premium credited.
//
// # It leaves the ordinary week byte-identical
//
// A zero credit is the shipped objective, reached by the exported XIValue above,
// and each branch is skipped rather than multiplied by zero. Squad construction
// goes through the unexported xiValue and cannot see this at all, which is the
// same seam fixture load uses — so HOLD is the confinement check for any
// measurement of this: it must not move.
func xiValueForTransfer(squad []PlayerMetrics, credit ChipCredit) float64 {
	if fixtureLoadTransfers {
		adj := make([]PlayerMetrics, len(squad))
		copy(adj, squad)
		for i := range adj {
			// Gated on `loadSet`, not on `FixtureLoad > 0`. The two agreed until
			// `fixtureLoadFor` learned to return a real 0 for a club that blanks the
			// whole window; after that, `> 0` reads a genuine blank as an unset field
			// and SKIPS the multiply, valuing a footballer who certainly scores
			// nothing at his full score. See PlayerMetrics.loadSet for why that is
			// reachable at a horizon of 2 to 4 and not at 1.
			if adj[i].loadSet && !adj[i].loadInScore {
				adj[i].Score *= adj[i].FixtureLoad
			}
		}
		squad = adj
	}
	xi, bench, _ := bestXI(squad)
	v, armband := xiValueShrunk(xi, captainShrink)
	if credit.zero() {
		return v
	}
	// The eleven is still picked to maximise the eleven, not the blend. That is
	// the model rather than an approximation of it: in every one of the H weeks
	// the manager fields his best XI, and in one of them the chip pays on top.
	for _, p := range bench {
		v += credit.Bench * credit.benchWeekScore(p)
	}
	return v + credit.Captain*armband
}

// Swap is one player leaving and one arriving.
type Swap struct {
	Out, In PlayerMetrics
	// Gain is the change in XIValue, per gameweek.
	Gain float64
}

// SquadState is the ownership and club-count bookkeeping a transfer search needs.
type SquadState struct {
	Players []PlayerMetrics
	Owned   map[int]bool
	Clubs   map[string]int

	// Sell is what each owned player raises when sold, in tenths, keyed by
	// element id. FPL does not let you sell at the market price: you get what
	// you paid plus half of any rise since, rounded down to the nearest £0.1m,
	// while a fall is taken in full. A squad of risen players is therefore worth
	// materially less than its headline value, and pricing transfers at market
	// hands the policy budget it does not have.
	//
	// Nil means "sell at market", which is right pre-season, when nothing has
	// moved, and for any caller that does not track purchase prices.
	Sell map[int]int

	// Chip credits a bench boost or triple captain planned inside this decision's
	// horizon, so the search can build toward it. The zero value — the default —
	// is the shipped objective, which knows about no chip at all.
	//
	// It lives here, on the per-call state, rather than on Engine, because the
	// tool runner fans searches out concurrently and per-call engine state
	// corrupts the other searches in the same turn. That is a recorded crash, not
	// a hypothetical. It is also why this is not a package-level switch.
	//
	// See ChipCredit for what the numbers mean and why the other two chips need
	// nothing here.
	Chip ChipCredit
}

// value is what this decision judges a fifteen by: the eleven, plus whatever a
// planned chip adds. Every search in this package must go through it rather than
// calling XIValue, or an arm that sets a credit will silently score half its
// candidates on the other objective.
func (s SquadState) value(squad []PlayerMetrics) float64 {
	return xiValueForTransfer(squad, s.Chip)
}

// sellPrice is what leaving player p raises. Falls back to his market price.
func (s SquadState) sellPrice(p PlayerMetrics) int {
	if v, ok := s.Sell[p.ID]; ok {
		return v
	}
	return tenths(p.Price)
}

// NewSquadState indexes a fifteen for transfer searching.
func NewSquadState(squad []PlayerMetrics) SquadState {
	s := SquadState{Players: squad, Owned: map[int]bool{}, Clubs: map[string]int{}}
	for _, p := range squad {
		s.Owned[p.ID] = true
		s.Clubs[p.Team]++
	}
	return s
}

// allows reports whether replacing the players at the given squad indices with
// the given arrivals keeps every club within the limit.
func (s SquadState) allows(outIdx []int, in []PlayerMetrics) bool {
	c := make(map[string]int, len(s.Clubs))
	for k, v := range s.Clubs {
		c[k] = v
	}
	for _, i := range outIdx {
		c[s.Players[i].Team]--
	}
	for _, p := range in {
		c[p.Team]++
	}
	for _, n := range c {
		if n > MaxPerClub {
			return false
		}
	}
	return true
}

// RankSwaps returns every legal one-for-one change, best first, scored by what
// it does to the eleven.
//
// bank is in tenths of a million, matching FPL's own units.
// discountIncoming charges a flat penalty against a player being *acquired*,
// leaving everyone already owned alone. It ships at zero — see "It does not
// work", below — so this is dormant instrumentation, not live behaviour.
//
// # The asymmetry it was built for, which is not there
//
// A transfer search is an argmax over some six hundred players, so the one it
// picks is disproportionately the one whose estimate is too high — the winner's
// curse. That is the argument this code was built on, and it is worth stating
// that **the asymmetry it predicts is not present at shipped config.**
//
// The 0.53 per-gameweek buy-side over-rating once quoted here came from a body
// of evidence measured with the transfer gate's min_gain at 0.7, which was
// retracted. Re-measured at 327 moves under the shipped 0.4, the buy side is
// over-rated by **0.230 at the median** and *under*-rated by **0.079 in the
// mean**, and buy-minus-sell is **+0.051** — the incoming player is marginally
// *better* calibrated than the outgoing one, the opposite of the predicted sign.
// The sell side is −0.282 median, not the +0.065 this comment used to claim.
//
// The one place the mechanism does survive is availability, and it is a much
// larger effect than the curse ever was: a sold player who keeps playing is
// −0.100 a gameweek, against **−2.223** for the 13% who stop. The error on the
// way out is about who *disappears*, not about who was over-rated.
//
// # Why this is not min_gain by another name
//
// min_gain raises the bar on the *gain*, which is symmetric — it cannot tell an
// over-rated arrival from an under-rated departure — and it was raised to
// exactly this figure and then retracted when the direction reversed under
// re-measurement.
//
// Correcting the incoming player's score instead is a different operation. It
// flows through XIValue, which means it flows through the **captain term**,
// which is where the damage was argued to be. xiValue counts the squad's
// highest scorer twice, so a premium acquisition has his over-estimate doubled,
// and a flat charge on the gain cannot reproduce that shape while this does.
//
// The gap that argument was built on has not held up either. The 1.72
// over-valuation on £9.0m+ buys is **retracted**: re-measured it is **+1.242
// with a standard error of 1.019, t = +1.22**, on 23 rows — never a measurement.
// The mechanism remains arithmetically real, since the captain term genuinely
// does double whatever error the top scorer carries; what is gone is any
// measured size for it.
//
// It is deliberately *not* scaled by price. Price is the proxy the divergence
// table happened to bucket on; the mechanism is estimation error at the top of
// the distribution, and the captain term already supplies the amplification
// where it belongs. Adding a price term as well would fit the bucket rather
// than the cause.
//
// The reported Swap keeps the player's real score, so the agent sees what the
// model actually thinks of him. Only the trial evaluation is discounted.
//
// # It does not work, and the argument above is where it fails
//
// Swept over six entry points and four seasons, paired against no discount:
//
//	discount   points/gw       t     transfers (from 512)
//	0.25          -0.018   -0.06                     423
//	0.53          -0.610   -1.81                     322
//	0.80          -0.914   -2.33                     226
//
// The measured value makes the policy worse and 0.80 is significantly worse.
// HOLD is exactly 0.000 throughout, confirming it touches only the transfer
// search. **It ships at zero.**
//
// The reasoning above is wrong in a way worth keeping. The captain channel is
// real, but it is nowhere near enough to make this a *selection* correction:
// applied to every incoming player alike, the discount cannot change **which**
// player is the best buy, only whether any buy clears the bar. Transfers
// collapse from 512 to 226, so empirically it behaves exactly like min_gain — a
// volume brake — despite the different mechanism. And volume is the one thing
// this project has confirmed three times over does not move the season.
//
// That is the third flat correction of a genuinely measured bias to lose points,
// after recency on rates and min_gain itself. The rule they establish:
// **the optimiser consumes an ordering, so a correction applied equally to every
// candidate cannot help it, however well measured the bias is.** Any correction
// that could work here has to be *differential* — discounting some incoming
// players more than others.
//
// Which leaves the honest open question. The only differential signal to hand is
// price, and it is rejected above as fitting the bucket rather than the cause.
// So either a principled differential signal exists and has not been found, or
// this bias is not correctable with the information the model has.
func discountIncoming(m PlayerMetrics) PlayerMetrics {
	if buyDiscount <= 0 {
		return m
	}
	m.Score -= buyDiscount
	if m.Score < 0 {
		m.Score = 0
	}
	return m
}

func RankSwaps(s SquadState, candidates []PlayerMetrics, bank int) []Swap {
	base := s.value(s.Players)
	trial := make([]PlayerMetrics, len(s.Players))

	var out []Swap
	for i, cur := range s.Players {
		for _, cand := range candidates {
			if s.Owned[cand.ID] || cand.Position != cur.Position {
				continue
			}
			// XIValue is monotone in every player's score: swapping in someone
			// who scores no more than the man he replaces can never raise the
			// eleven. Dropping those without scoring them is exact, not a
			// heuristic, and it is what keeps this affordable now that a
			// five-transfer bank means six of these searches in a single week.
			//
			// **It stays exact under a chip credit, and that was checked rather
			// than assumed.** The first version of this exempted the bench credit
			// from the prune, on an argument that a lower-scoring arrival might
			// raise the total by rearranging who sits on the bench. The exemption
			// was wrong twice over. The objective with the eleven still chosen to
			// maximise the eleven is
			//
			//	(1-b)*sum(XI) + armband + ViceCaptainWeight*vice + b*sum(all fifteen)
			//
			// and every term of that is monotone nondecreasing in each player's
			// score, for any b — so a cheaper arrival cannot raise it. Checked
			// empirically as well as analytically: 200,000 randomised same-position
			// downgrades over b in {0.1, 0.2, 0.5, 1.0, 1.5} and captain credits in
			// {0, 0.2, 1} produced zero cases where a lower-scoring arrival raised
			// xiValueForTransfer.
			//
			// It also left the two searches inconsistent, which is the tell: the
			// same prune in RankPairs below was never exempted, so if the concern
			// had been real the pair search would have been scoring a subset of its
			// own objective in every chip week while this one did not.
			priced := discountIncoming(cand)
			if priced.Score <= cur.Score {
				continue
			}
			if tenths(cand.Price)-s.sellPrice(cur) > bank {
				continue
			}
			if !s.allows([]int{i}, []PlayerMetrics{cand}) {
				continue
			}
			copy(trial, s.Players)
			trial[i] = priced
			gain := s.value(trial) - base
			if gain <= 0 {
				continue
			}
			out = append(out, Swap{Out: cur, In: cand, Gain: gain})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Gain > out[b].Gain })
	return out
}

// Pair is a set of downgrades and the upgrade they pay for, taken as one
// decision because none of them stands up alone.
type Pair struct {
	// Up is the player the move exists to buy; Downs are the sales that fund
	// him, cheapest-consequence first.
	Up    Swap
	Downs []Swap
	// Gain is the change in XIValue from making all of them, per gameweek.
	Gain float64
}

// Moves is the total number of transfers the pair costs.
func (p Pair) Moves() int { return 1 + len(p.Downs) }

// RankPairs returns the best funded-premium moves: an upgrade no single swap
// could afford, plus the sales that pay for it.
//
// maxDowns caps how many sales may fund one upgrade, and is the caller's
// transfer budget minus the upgrade itself. With the modern five-transfer bank
// a manager really can sell three players to buy one, and restricting this to a
// single funding sale left the same premiums unreachable that the one-for-one
// search did — only at a higher price point.
//
// Only upgrades that are unreachable on their own are considered; anything
// affordable outright is RankSwaps's job.
//
// # Cost control
//
// The exhaustive search is every combination of squad slots against every
// combination of replacements, which is far too large to score properly.
// Candidates are cut to a price frontier per position, then ranked by a cheap
// proxy — the sum of the players' own score deltas — and only the best few
// hundred are scored on the eleven.
//
// The proxy is exactly what misleads a greedy search, so it is used only to
// filter, never to choose: every surviving candidate is evaluated with XIValue.
// An earlier version picked the funding sales greedily by money freed per point
// given up and skipped the exact comparison. It was measurably worse — a mean
// of 2076 across three replayed seasons against 2158 for scoring the options
// properly — because the cheapest-looking sale is often not the one that leaves
// the best eleven.
func RankPairs(s SquadState, candidates []PlayerMetrics, bank, maxDowns, limit int) []Pair {
	if len(s.Players) == 0 || maxDowns < 1 {
		return nil
	}
	base := s.value(s.Players)
	frontier := PriceFrontier(candidates, s.Owned)

	// Every sale available at every slot, with what it frees and what it costs.
	type downOpt struct {
		slot  int
		in    PlayerMetrics
		freed int
		loss  float64
	}
	var downs []downOpt
	for j, cur := range s.Players {
		for _, d := range frontier[cur.Position] {
			// What a downgrade frees is the selling price of the man leaving
			// minus the market price of the man arriving, not the difference
			// between two market prices.
			if freed := s.sellPrice(cur) - tenths(d.Price); freed > 0 {
				downs = append(downs, downOpt{slot: j, in: d, freed: freed,
					loss: cur.Score - d.Score})
			}
		}
	}
	// Ordered by money freed per point given up, used only to build the
	// multi-sale combinations below.
	byEfficiency := append([]downOpt(nil), downs...)
	sort.SliceStable(byEfficiency, func(a, b int) bool {
		ea := float64(byEfficiency[a].freed) / math.Max(byEfficiency[a].loss, 0.01)
		eb := float64(byEfficiency[b].freed) / math.Max(byEfficiency[b].loss, 0.01)
		if ea != eb {
			return ea > eb
		}
		return byEfficiency[a].freed > byEfficiency[b].freed
	})

	type cand struct {
		slot  int
		up    PlayerMetrics
		set   []downOpt
		proxy float64
		spend int
	}
	var cands []cand
	for i, cur := range s.Players {
		for _, up := range frontier[cur.Position] {
			if up.Score <= cur.Score {
				continue
			}
			needed := tenths(up.Price) - s.sellPrice(cur) - bank
			if needed <= 0 {
				continue // a single swap reaches this one
			}
			upDelta := up.Score - cur.Score
			upSpend := tenths(up.Price) - s.sellPrice(cur)

			add := func(set []downOpt) {
				proxy, spend := upDelta, upSpend
				for _, d := range set {
					proxy -= d.loss
					spend -= d.freed
				}
				cands = append(cands, cand{slot: i, up: up,
					set: append([]downOpt(nil), set...), proxy: proxy, spend: spend})
			}

			// Every single sale that covers the shortfall on its own. These are
			// the cheapest moves available and are enumerated exhaustively.
			var single bool
			for _, d := range downs {
				if d.slot == i || d.in.ID == up.ID || d.freed < needed {
					continue
				}
				add([]downOpt{d})
				single = true
			}
			// Only reach for more transfers when one will not do. Selling two
			// players to raise money one sale could have raised is strictly
			// worse: it costs an extra transfer for the same eleven.
			if single || maxDowns < 2 {
				continue
			}
			used := map[int]bool{i: true}
			var set []downOpt
			raised := 0
			for _, d := range byEfficiency {
				if raised >= needed || len(set) == maxDowns {
					break
				}
				if used[d.slot] || d.in.ID == up.ID {
					continue
				}
				used[d.slot] = true
				set = append(set, d)
				raised += d.freed
			}
			if raised >= needed {
				add(set)
			}
		}
	}
	if len(cands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].proxy > cands[b].proxy })
	const scored = 400
	if len(cands) > scored {
		cands = cands[:scored]
	}

	trial := make([]PlayerMetrics, len(s.Players))
	var out []Pair
	for _, c := range cands {
		if c.spend > bank {
			continue
		}
		outIdx := []int{c.slot}
		arrivals := []PlayerMetrics{c.up}
		for _, d := range c.set {
			outIdx = append(outIdx, d.slot)
			arrivals = append(arrivals, d.in)
		}
		if !s.allows(outIdx, arrivals) {
			continue
		}
		copy(trial, s.Players)
		trial[c.slot] = discountIncoming(c.up)
		for _, d := range c.set {
			trial[d.slot] = discountIncoming(d.in)
		}
		gain := s.value(trial) - base
		if gain <= 0 {
			continue
		}
		p := Pair{Up: Swap{Out: s.Players[c.slot], In: c.up}, Gain: gain}
		for _, d := range c.set {
			p.Downs = append(p.Downs, Swap{Out: s.Players[d.slot], In: d.in})
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Gain != out[b].Gain {
			return out[a].Gain > out[b].Gain
		}
		// Same eleven for fewer transfers is strictly better.
		return out[a].Moves() < out[b].Moves()
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// PriceFrontier is, per position, the best-scoring available player at each
// price, ordered by ascending price.
//
// Anyone outscored by someone at or below his price can never be the right
// answer, so dropping him costs nothing and keeps the pair search tractable.
func PriceFrontier(candidates []PlayerMetrics, owned map[int]bool) map[string][]PlayerMetrics {
	byPos := map[string][]PlayerMetrics{}
	for _, m := range candidates {
		if owned[m.ID] {
			continue
		}
		byPos[m.Position] = append(byPos[m.Position], m)
	}
	out := make(map[string][]PlayerMetrics, len(byPos))
	for pos, ms := range byPos {
		out[pos] = frontierOf(ms)
	}
	return out
}

func frontierOf(ms []PlayerMetrics) []PlayerMetrics {
	ms = append([]PlayerMetrics(nil), ms...)
	sort.Slice(ms, func(a, b int) bool { return ms[a].Price < ms[b].Price })
	var out []PlayerMetrics
	best := math.Inf(-1)
	for _, m := range ms {
		if m.Score > best {
			best = m.Score
			out = append(out, m)
		}
	}
	return out
}

// tenths converts a price in millions to FPL's integer tenths, so budget
// arithmetic never accumulates float error.
func tenths(price float64) int { return int(price*10 + 0.5) }

// Tenths converts a price in millions to the tenths this package counts money in.
//
// Exported because cmd/armband had grown a third spelling of it. The three agreed on
// every price the game can produce, which is why nothing caught them; they disagreed
// on negatives, and a fourth would not have to be so lucky.
func Tenths(price float64) int { return tenths(price) }
