package analysis

import "math"

// Option value: what an unexercised option is worth to hold, and how that decays.
//
// # The one quantity four levers were each modelling as a constant
//
// A banked transfer, a wildcard, a free hit and a bench boost are all **options
// whose value decays as the window they can be exercised over shrinks**, and this
// repository priced every one of them with a fixed number:
//
//   - `free_transfer_value` is 2.0 in every gameweek, and the recorded rule that
//     it "must not taper as the season ends" is a **consequence of calling it a
//     confidence threshold**, not a measurement. Call it an opportunity cost and
//     tapering becomes mandatory, because the future the transfer is saved for
//     does shrink and is empty at GW38. The label is exactly what is in dispute.
//   - a wildcard has a reservation price — how bad the squad must be before it is
//     worth spending — and nothing here lowers it toward the chip's expiry, where
//     an unplayed chip is simply lost.
//   - `chipBarBenchBoost` 16 and `chipBarTripleCaptain` 12 are **fixed bars on
//     expiring chips**, and this record already says they are asserted rather than
//     measured, and that a bar set too low flatters timing while one set too high
//     flatters the threshold rule.
//
// ⚠️ **That one shape explains all four is the strongest thing in its favour and
// is NOT evidence.** A story that unifies four unrelated nulls at once is precisely
// the shape this record has been wrong about before. Everything here ships OFF;
// what it buys is that the four are now expressible and separable, not that any of
// them is better.
//
// # One implementation, four consumers
//
// Four copies of a decay curve is this project's signature failure — one quantity
// with several implementations — and it has two source scans against it. So the
// curve is [OptionDecay], the congestion factor is [CongestionFactor], the two are
// combined exactly once in [OptionValueAt], and the four levers reach it through
// named faces ([TransferHoldFactor], [ChipReservationAt], [ChipBarAt]) that add a
// base price and nothing else. `TestTheCopiedExpressionsHaveOneImplementation`
// carries a row keyed on the saturating-ratio idiom, so a fifth consumer that
// retypes `x / (x + k)` fails rather than diverging quietly.
//
// # What looks forward, and what does not
//
// The decay reads the **calendar** — how many gameweeks are left in which this
// option could still be exercised — and the congestion factor reads the **forward
// fixture list**. Both are genuinely forward-looking, and both are facts about the
// schedule rather than predictions about football.
//
// ⚠️ **Nothing here makes the weekly transfer decision itself forward-looking.**
// `decide` is greedy per week and prices no future state in either arm; what these
// terms change is the PRICE it puts on today's options, not the set of futures it
// considers. So a lever built on this changes the *shape* of the policy — the
// charge is no longer one number all season — while leaving the search exactly as
// myopic as it was. Read a movement as "the policy became more or less reluctant
// to spend", never as "the policy started planning".

// OptionWindow is the span of gameweeks an option can still be exercised over.
//
// `Expiry` is the LAST gameweek in which exercising is still possible, and the
// decay is exactly zero there — which is the boundary condition the whole model
// turns on. For a banked transfer that is GW38: a transfer held at the GW38
// deadline can never be spent, so holding it is worth nothing. For a first-set
// chip in a two-set season it is `ChipResetGW - 1`, because the set expires; for a
// second-set chip, and for any chip in a one-set season, it is 38.
//
// A zero `Expiry` means "no window is known", and every consumer here treats that
// as a **no-op** rather than as an expiry of gameweek zero. That is the recorded
// rule about an unset penalty: a raw multiply by an unset value zeroed every
// congested player's score for two seasons of replays, and an unset window that
// read as "expired" would zero every held option instead.
type OptionWindow struct {
	Expiry int
}

// Usable reports whether this window says anything. See OptionWindow.
func (w OptionWindow) Usable() bool { return w.Expiry > 0 }

// Remaining is how many gameweeks after `gw` the option could still be exercised
// in, floored at zero.
//
// Measured from `gw` rather than including it, because every consumer here is
// asking about HOLDING: the question is what the option is worth *unspent*, and an
// option that can only be exercised this week has no holding value at all. At
// `gw == Expiry` this is 0 and the decay is 0, which is the use-it-or-lose-it week.
func (w OptionWindow) Remaining(gw int) float64 {
	if !w.Usable() {
		return 0
	}
	if n := w.Expiry - gw; n > 0 {
		return float64(n)
	}
	return 0
}

// DefaultOptionHalfLife is the gameweeks of remaining window at which holding an
// option is worth half of what a very long window would make it worth.
//
// **Asserted, not measured.** Nothing in this repository has ever varied any of
// the four constants this curve replaces — `free_transfer_value` is stamped at 2
// in every `*.provenance.csv` there is — so there is no ladder to read a shape
// off, and this record's detection threshold is 33 points a season on `HOLD` and
// 70 on `POLICY` against constants that are mostly worth 11 to 34. Picking 8 from
// a sweep would be an argmax; picking it from mechanism is at least honest about
// what it is.
//
// The mechanism it asserts: the shipped decision horizon is 5 gameweeks, so an
// option with a window several times that is, for decision purposes, unconstrained
// — the difference between 20 weeks left and 30 is not something a five-week
// search can act on, while the difference between 0 and 3 is everything. 8 puts
// the half-way point just past the horizon, which is the shortest window in which
// the search can still see a whole alternative plan.
const DefaultOptionHalfLife = 8.0

// OptionDecay is how much of an option's holding value survives, given the
// gameweeks of window left. In [0, 1].
//
// # The shape, and what it deliberately does not claim
//
//	decay = remaining / (remaining + halfLife)
//
// Three properties, and each is a claim worth stating separately:
//
//   - **Exactly 0 at expiry.** Not "small", not "floored at some minimum" — an
//     option that cannot be exercised again is worth nothing to hold, and this is
//     the one boundary the whole model is confident about.
//   - **Monotone increasing**, so more window is never worth less.
//   - **Saturating.** It approaches 1 and never reaches it. A linear ramp would
//     assert that 30 weeks of window is 50% more valuable than 20, which is a
//     quantitative claim nobody here has measured; saturating asserts only that
//     both are "a lot", which is what is actually known.
//
// ⚠️ **`halfLife` of zero or less means the DEFAULT, not "no decay".** Same rule
// as everywhere else in this file: an unset knob is a no-op, and a literal zero
// half-life would make the curve 1 everywhere except at expiry, which is a
// different model rather than an absent one.
func OptionDecay(remaining, halfLife float64) float64 {
	if halfLife <= 0 {
		halfLife = DefaultOptionHalfLife
	}
	if remaining <= 0 {
		return 0
	}
	return remaining / (remaining + halfLife)
}

// DefaultCongestionSensitivity is how hard the forced-demand channel leans on
// fixture density.
//
// **Asserted, not measured.** At 1.0 a club playing two matches a gameweek doubles
// the holding value of a transfer, which is the strongest reading of "congestion
// causes injuries" the mechanism supports and is deliberately an upper end rather
// than a middle: the lever ships off, so the value that matters is the one an arm
// would be run at, and an arm run at a sensitivity too small to move anything is a
// comparison that never ran.
const DefaultCongestionSensitivity = 1.0

// CongestionFactor turns a forward fixture density into a multiplier on holding
// value, at or above zero.
//
//	factor = 1 + sensitivity x (load - 1)
//
// `load` is matches per club per gameweek over the window looked at, so 1.0 is an
// ordinary run, 2.0 is every club doubling, and 0 is a total blank. At the default
// sensitivity a doubling week doubles the value of holding a transfer and a
// blanking week takes it to zero.
//
// # This is NOT the congestion block, and the difference is the point
//
// `analysis.Congestion` is eight penalties — European campaigns, domestic cups,
// travel load, short rest — every one of which ships at 1.00 and is inert, with
// `TestTheShippedCongestionBlockIsInert` making re-enabling any of them
// deliberate. Those act on **minutes**, they are hand-maintained per season from
// sources outside the FPL API, and they price a named competition's effect on a
// named player.
//
// This is a different quantity on a different side of the model: it is read off
// the **fixture list the engine already holds**, it is per club rather than per
// player, it needs no hand maintenance, and it acts on the **price of a transfer**
// rather than on any player's score. Nothing here re-enables anything there, and
// a change to one says nothing about the other.
//
// # Why more football makes a held transfer worth more
//
//	value of holding ~= P(forced move soon) x cost of being caught short
//
// Congestion drives the first term: more matches in less time is more injuries and
// more rotation, and a widely-owned premium breaking is a transfer you must make
// whether you wanted to or not. The cost of being caught short is a hit, or
// fielding somebody who is not playing. So the option to absorb a compulsory move
// without paying for it is worth most exactly when compulsory moves are likeliest.
//
// ⚠️ **The congestion spike and the double gameweeks arrive in the SAME WEEKS**,
// because both come out of one fixture pile-up — cup rounds and European ties
// displacing league matches into midweek. So this factor raises the price of
// spending a transfer in precisely the weeks a doubles-chasing policy most wants to
// spend one. That tension is real and is deliberately not designed away: the two
// readings are both true, and which dominates is exactly what an arm would measure.
func CongestionFactor(load, sensitivity float64) float64 {
	if sensitivity <= 0 {
		sensitivity = DefaultCongestionSensitivity
	}
	f := 1 + sensitivity*(load-1)
	if f < 0 {
		return 0
	}
	return f
}

// DefaultCongestionHorizon is how many gameweeks forward the fixture density is
// read over.
//
// **Asserted**, and set to the shipped decision horizon of 5 on the argument that
// the forced-demand risk a held transfer insures against is the risk over the
// window the decision itself can see. A longer window would average a pile-up away
// against ordinary weeks either side of it; a shorter one would make the signal a
// property of one fixture round, which is not what "a congested run" means.
const DefaultCongestionHorizon = 5

// OptionPricing is the knobs the curve takes, so a consumer passes one value
// rather than three positional numbers it can transpose.
//
// All zero means all defaults, which is what an unset struct field gives — again
// the unset-is-a-no-op rule rather than unset-is-zero.
type OptionPricing struct {
	// HalfLife is OptionDecay's half-life in gameweeks. Zero means the default.
	HalfLife float64 `json:"half_life_gameweeks"`
	// CongestionSensitivity is CongestionFactor's slope. Zero means the default.
	CongestionSensitivity float64 `json:"congestion_sensitivity"`
	// CongestionHorizon is how many gameweeks the forward fixture density is read
	// over. Zero means the default.
	CongestionHorizon int `json:"congestion_horizon"`
}

// Horizon resolves CongestionHorizon's zero to the default, so the several
// consumers cannot disagree about what an unset field means.
func (p OptionPricing) Horizon() int {
	if p.CongestionHorizon <= 0 {
		return DefaultCongestionHorizon
	}
	return p.CongestionHorizon
}

// OptionValue is what holding one option is worth, decomposed.
//
// Every field is reported rather than only the product, for the same reason every
// scoring term in this project is a separate reported multiplier: a number the
// agent can explain beats a number it can only assert, and a decomposition is what
// lets a reader see that a lever was inert because the window was long rather than
// because the congestion was flat.
type OptionValue struct {
	// Remaining is the gameweeks of exercise window left after this one.
	Remaining float64
	// Decay is OptionDecay(Remaining, ...) divided by its own season mean.
	//
	// ⚠️ **NOT in [0, 1].** It is exactly 0 at expiry and above 1 early, because it
	// is normalised to average 1 over the option's whole window — see
	// MeanOptionDecay. An un-normalised curve would make every taper arm a level
	// cut as well as a schedule, and every half-life ladder a level ladder.
	Decay float64
	// Load is the forward fixture density the congestion factor was read from,
	// in matches per club per gameweek. 1.0 is an ordinary run.
	Load float64
	// Congestion is CongestionFactor(Load, ...), at or above zero.
	Congestion float64
	// Factor is Decay x Congestion: the multiplier a base price is scaled by.
	//
	// ⚠️ **It is not bounded by 1.** A long window in a congested run is worth
	// MORE than the base price, which is the whole content of the insurance
	// reading — a term that could only ever discount would be a taper with a name
	// on it rather than an option value.
	Factor float64
}

// MeanOptionDecay is the average of [OptionDecay] over every gameweek the option
// exists in, `[1, Expiry]`.
//
// # It exists because taper-versus-flat was NOT a shape contrast without it
//
// `OptionDecay` is monotone decreasing and bounded by 1, so its mean over a season
// is strictly below 1 and **no half-life reproduces the flat constant** — as
// `h → ∞` the curve tends to 0, not 1. So switching the taper on lowered the
// AVERAGE charge as well as giving it a schedule, and every rung of a half-life
// ladder moved the average too. At `FreeCost` 2.0 over a full season the mean
// charge reads:
//
//	h = 3    1.561
//	h = 8    1.241
//	h = 16   0.957
//	h = 30   0.693
//
// A factor of 2.3 across the ladder, all of it level. And the flat level has
// **never been varied in any banked sweep** — every `*.provenance.csv` stamps
// `free_transfer_value` at 2 — so the level is the untested prior question and a
// taper arm run without this would be confounded with it. Found in review before
// any arm was run.
//
// Dividing by this mean makes the taper **mean-preserving**: the same average
// charge as flat, redistributed across the season. That is what makes a
// taper-versus-flat difference attributable to the SHAPE.
//
// ⚠️ **The consequence is that the factor now EXCEEDS 1 early**, which is not a
// side effect to be tidied away — it is what mean preservation means. A charge that
// only ever fell would be a discount with a schedule attached, and the option
// reading says holding is worth more early *and* less late.
//
// ⚠️ **De-confounding is exact only over the whole window.** A cell entering at
// GW26 decides in `[27, 38]`, where the normalised curve still averages below 1, so
// its mean charge remains lower than flat. The residual is entry-point dependent
// and is the reason an arm on this must be read against the entry gameweek rather
// than pooled — which is the same argument the dose columns make. Anchoring at the
// season's start instead would fix the first week and not the mean, so it does not
// discharge this.
//
// ⚠️ **The reference window is `[1, Expiry]`, not the cell's own.** One number per
// half-life, identical for the replay and for the live path, so the two cannot
// disagree about what a transfer costs in a given gameweek — which they would if
// the normaliser moved with a cell's entry point.
func MeanOptionDecay(w OptionWindow, halfLife float64) float64 {
	if !w.Usable() {
		return 1
	}
	sum := 0.0
	for gw := 1; gw <= w.Expiry; gw++ {
		sum += OptionDecay(w.Remaining(gw), halfLife)
	}
	if sum <= 0 {
		return 1
	}
	return sum / float64(w.Expiry)
}

// OptionValueAt prices one held option. This is the single implementation the
// four levers share; see the file header.
//
// `load` is matches per club per gameweek over whatever window the caller thinks
// the forced-demand risk lives in — [Engine.FixtureCongestion] computes it from
// the clubs a squad holds. Pass 1.0 for "ordinary", which makes the congestion
// factor exactly 1 and leaves the decay alone.
//
// An unusable window returns the zero value, whose `Factor` is 0. ⚠️ **That is a
// refusal to price, and a caller must not multiply by it blindly** — see
// [TransferHoldFactor], which returns 1 in that case rather than zeroing the
// charge. The two are different answers to "we do not know", and this record has
// paid for the difference once already.
func OptionValueAt(w OptionWindow, gw int, load float64, p OptionPricing) OptionValue {
	if !w.Usable() {
		return OptionValue{}
	}
	v := OptionValue{Remaining: w.Remaining(gw), Load: load}
	// Mean-preserving, so a taper-versus-flat difference is a SHAPE difference and
	// not a level cut wearing a schedule. See MeanOptionDecay for the arithmetic
	// and for the residual this does not remove.
	v.Decay = OptionDecay(v.Remaining, p.HalfLife) / MeanOptionDecay(w, p.HalfLife)
	v.Congestion = CongestionFactor(load, p.CongestionSensitivity)
	v.Factor = v.Decay * v.Congestion
	return v
}

// TransferHoldFactor is what a free transfer's charge is multiplied by in this
// gameweek: the first of the four consumers.
//
// # What it prices that the shipped constant does not
//
// `free_transfer_value` is charged when a transfer is SPENT, so it is the price of
// giving up the option to hold. This record's own re-reading of the banking null is
// that the rule prices the held transfer as **enumeration capacity** — "can I build
// a bigger funded package next week" — and that channel is measured inert: the two
// arms enumerated the identical candidate list in **224 of 226** weeks, because
// `RankPairs` builds a multi-downgrade set only for upgrades no single funding sale
// can reach. So banking today measures the one meaning of a held transfer that
// happens to be dead.
//
// Two meanings had no term at all, and both are in this factor:
//
//   - **Reserved exit.** A fixture-driven move normally costs twice — once to make
//     it and once to unwind it — and a held transfer is what pays the second bill.
//     Its worth is therefore the DECAY term: the more weeks left in which you might
//     want to unwind, the more an exit is worth holding, and at GW38 there is
//     nothing left to unwind into so it is worth exactly nothing. The season's end
//     is a free exit, which is the same statement from the other side.
//   - **Insurance against forced demand.** A compulsory transfer — the injury you
//     must answer — costs a hit if you have nothing in hand. Its worth is the
//     CONGESTION term, because compulsory moves cluster where the fixtures do.
//
// ⚠️ **Both enter through the CHARGE, which is a reluctance to spend rather than a
// term in any forward valuation.** `decide` is greedy and prices no future state,
// so there is nowhere else for them to go without a valuation that scores a move
// over the weeks it will actually be held — a model change rather than a knob. Do
// not read a movement in an arm built on this as evidence that the policy learned
// to plan.
//
// Returns 1 — a no-op — when the window says nothing, rather than 0. A charge of
// zero is "every transfer is free", which is a real and very different policy, and
// getting an unset knob to mean it is the exact bug this record's `usable()` rule
// exists to stop.
func TransferHoldFactor(w OptionWindow, gw int, load float64, p OptionPricing) float64 {
	if !w.Usable() {
		return 1
	}
	return OptionValueAt(w, gw, load, p).Factor
}

// ChipReservationAt is what a chip's state trigger must be beaten by, in points:
// the second consumer.
//
// A wildcard is worth holding because a worse squad might come along. That is an
// option, and its reservation price should fall toward the chip's expiry until, in
// the last week it could be played, it is zero — use it or lose it. `base` is what
// holding is worth with an unconstrained window ahead; the returned figure is what
// this week's repair cost actually has to clear.
//
// Returns 0 on an unusable window, and here that IS the right no-op: an unknown
// expiry means no reservation, so the trigger falls back to firing whenever the
// repair cost is positive. The opposite default — an infinite reservation — would
// be a trigger that never fires, which is indistinguishable from one that is not
// wired, and this record has been caught by exactly that shape.
func ChipReservationAt(base float64, w OptionWindow, gw int, load float64, p OptionPricing) float64 {
	if !w.Usable() {
		return 0
	}
	return clampNonNegative(base * OptionValueAt(w, gw, load, p).Factor)
}

// ChipBarAt is the points bar a scoring chip must clear to be worth playing in
// this gameweek: the third and fourth consumers, bench boost and free hit.
//
// Identical arithmetic to [ChipReservationAt] and deliberately a separate name
// rather than one function with a comment. They are the same quantity — the value
// of not exercising yet — but a reader following a bench boost's bar should not
// have to satisfy himself that a wildcard's reservation is the same thing, and a
// future divergence between the two would then be a diff rather than an argument.
// ⚠️ If they ever need to differ, the difference goes in the BASE, not in a second
// curve.
//
// A fixed bar on an expiring chip is what ships today (`chipBarBenchBoost` 16,
// `chipBarTripleCaptain` 12, both asserted), and its failure mode is visible in
// this record already: an unplayed chip scores nothing at all, so a bar that does
// not fall toward the expiry refuses weeks that were the best remaining offer.
func ChipBarAt(base float64, w OptionWindow, gw int, load float64, p OptionPricing) float64 {
	return ChipReservationAt(base, w, gw, load, p)
}

// TransferHoldFactorFor is [TransferHoldFactor] with the squad's own congestion
// read for it: the whole quantity in one call.
//
// It exists because four sites want it — the replay's `decide`, the live banking
// rule, the agent's swap tool and the live transfer command — and each of them
// composing `SquadCongestion`, `Horizon()` and `TransferHoldFactor` by hand is
// three chances to read the load over a different window. That is this project's
// signature failure at the smallest possible scale, and the smallest scale is
// where it actually happens.
//
// ⚠️ **The load window starts at `gw+1`, not at `gw`.** The two halves of the
// factor have to agree about whether this gameweek is inside the window, and
// [OptionWindow.Remaining] excludes it — correctly, since the question is what the
// option is worth UNSPENT and an option exercisable only this week has no holding
// value. Reading the load from `gw` disagreed, and for a chip the consequence was
// perverse: the very double the chip is being played for raised the bar it had to
// clear. Found in review.
//
// The shift goes through `fixtureLoadAfter`, which is `fixtureLoadFor` with the
// window's lower bound parameterised — one density function, not two.
func (e *Engine) TransferHoldFactorFor(held []int, gw int, p OptionPricing) float64 {
	return TransferHoldFactor(TransferExpiry(), gw, e.HoldingCongestion(held, gw, p), p)
}

// HoldingCongestion is the forward fixture density relevant to HOLDING an option
// in gameweek `gw`: the squad's clubs over `[gw+1, gw+horizon]`.
//
// See TransferHoldFactorFor for why the window excludes `gw` itself. Separate from
// SquadCongestion because a caller asking "how congested is the run I am about to
// play" wants the window that includes now, and a caller asking "how much forced
// demand am I insuring against" does not.
func (e *Engine) HoldingCongestion(held []int, gw int, p OptionPricing) float64 {
	if e == nil || e.Boot == nil {
		return 1
	}
	teams := make([]int, 0, len(held))
	for _, id := range held {
		if el := e.Boot.ElementByID(id); el != nil {
			teams = append(teams, el.Team)
		}
	}
	if len(teams) == 0 {
		return 1
	}
	sum := 0.0
	for _, t := range teams {
		sum += e.fixtureLoadAfter(t, p.Horizon(), gw)
	}
	return sum / float64(len(teams))
}

// SquadHasABlank reports whether any club the squad holds plays no match in this
// engine's imminent gameweek.
//
// The free-hit rule's pre-filter, exported because the rule lives in
// `internal/backtest` and this is a fixture-count question the scoring path
// already answers. It reads `fixtureLoadFor` at a horizon of 1, so it inherits the
// anchor fix that made a blank expressible at all — anchored on the club's next
// FIXTURE the window slides past a blank and the answer is always false.
//
// ⚠️ **"This engine's imminent gameweek" is whatever round the engine's fixture
// list says is next**, so a caller reconstructing at `gw-1` gets `gw`. There is no
// gameweek parameter on purpose: one would be a second idea of which round is
// imminent, sitting beside `upcomingGWs`, and disagreeing with it silently.
func (e *Engine) SquadHasABlank(held []int) bool {
	if e == nil || e.Boot == nil {
		return false
	}
	seen := map[int]bool{}
	for _, id := range held {
		el := e.Boot.ElementByID(id)
		if el == nil || seen[el.Team] {
			continue
		}
		seen[el.Team] = true
		if e.fixtureLoadFor(el.Team, 1) == 0 {
			return true
		}
	}
	return false
}

// FixtureCongestion is the forward fixture density a set of clubs faces: matches
// per club per gameweek over the next `horizon` gameweeks.
//
// 1.0 is an ordinary run, above 1 is a pile-up, below 1 means blanks. It is the
// mean of `fixtureLoadFor` over the clubs given, which is the same quantity the
// scoring path already uses for doubles and blanks — so this adds a consumer
// rather than a second measurement of fixture density, and inherits the anchor fix
// that made a blank expressible at all.
//
// ⚠️ **It is a mean over CLUBS, not over players**, so a squad holding three
// Arsenal players counts Arsenal three times. That is deliberate: the quantity
// wanted is the risk to THIS squad, and three players at a congested club is three
// times the exposure. Passing a de-duplicated club list gives the other reading,
// and the two differ — name which one a figure came from.
//
// An empty list returns 1, the ordinary-run value, so a caller with no squad gets
// a no-op rather than a zero.
func (e *Engine) FixtureCongestion(teamIDs []int, horizon int) float64 {
	if len(teamIDs) == 0 {
		return 1
	}
	sum := 0.0
	for _, id := range teamIDs {
		sum += e.fixtureLoadFor(id, horizon)
	}
	return sum / float64(len(teamIDs))
}

// SquadCongestion is FixtureCongestion for a squad given as element ids.
//
// The two exist separately because the callers hold different things: the replay
// holds element ids and the live path holds a squad, while a diagnostic asking
// "what does the calendar look like" holds clubs. Mapping element to club is one
// bootstrap lookup and doing it at each caller is how the two would drift.
//
// Elements that do not resolve are skipped rather than counted as an ordinary run:
// an id nobody can find is missing information, and averaging a 1.0 into the mean
// for it would quietly pull a congested squad back toward normal.
func (e *Engine) SquadCongestion(ids []int, horizon int) float64 {
	if e == nil || e.Boot == nil {
		return 1
	}
	teams := make([]int, 0, len(ids))
	for _, id := range ids {
		if el := e.Boot.ElementByID(id); el != nil {
			teams = append(teams, el.Team)
		}
	}
	return e.FixtureCongestion(teams, horizon)
}

// ChipExpiry is the last gameweek a chip from this set can be played in.
//
// One implementation because three levers need it and it is exactly the sort of
// two-line rule that gets retyped: FPL expires the first set after GW19 in a
// two-set season, so its expiry is `resetGW - 1`, and everything else runs to 38.
//
// `resetGW` of zero, or a season with one set, means 38 — the whole season — which
// is the correct reading for every archived season before 2025-26.
//
// It takes the reset gameweek rather than the season name because the season gate
// lives in `backtest.ChipSetsFor`, which this package must not import; the caller
// that knows the season passes 0 for a one-set season.
func ChipExpiry(set, resetGW int) OptionWindow {
	if resetGW <= 0 || set != 1 {
		return OptionWindow{Expiry: 38}
	}
	return OptionWindow{Expiry: resetGW - 1}
}

// TransferExpiry is the last gameweek a banked transfer could be spent in.
//
// Always the end of the season, and it is a function rather than the literal 38
// for one reason: it is the boundary that makes `free_transfer_value` taper at
// all, and a reader who finds `38` inline has to work out whether it is a season
// length, a gameweek count or a horizon. `math.MaxInt` is not an option — an
// unbounded window would make the decay 1 forever, which is the constant this
// replaces.
func TransferExpiry() OptionWindow { return OptionWindow{Expiry: 38} }

// clampNonNegative keeps a price from going below zero. Named rather than inlined
// so the two chip faces and the transfer face cannot disagree about whether a
// negative base is an error or a zero; it is a zero, because a negative bar means
// "play it whatever happens" and that is a legitimate configuration.
func clampNonNegative(v float64) float64 { return math.Max(0, v) }
