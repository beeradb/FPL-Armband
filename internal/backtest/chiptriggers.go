package backtest

import (
	"fmt"
	"os"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// oneWeekEngine builds a horizon-1 engine on an already-reconstructed bootstrap.
//
// A chip pays in ONE gameweek, so anything valuing one has to read a one-gameweek
// view: a five-week engine dilutes a doubling bench across four weeks the chip
// will never be played in, which is the error `fixtureLoadFor`'s confinement to
// the horizon-1 view exists to avoid, arriving through the chip instead of the
// eleven.
//
// One function because three callers want it — the free hit's fielded fifteen,
// the free hit's TRIGGER and the bench boost's — and the construction is five
// lines nobody would import for. The priors, the recency index and the team-form
// index are passed in rather than rebuilt, so this shares the decision's own
// state instead of forming a second opinion from the same archive.
func oneWeekEngine(b *fpl.Bootstrap, fx []fpl.Fixture, w analysis.Weights,
	idx analysis.PriorSeason, recent analysis.RecentForm,
	form analysis.TeamFormSource) *analysis.Engine {

	w.Horizon = 1
	e := analysis.NewEngineFull(b, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
	e.Priors = idx
	e.Recent = recent
	e.TeamForm = form
	return e
}

// Chip state triggers: playing a chip because of what the squad and the calendar
// are, rather than because a plan named a gameweek.
//
// # Four levers, four switches, and deliberately no master flag
//
// The wildcard, the bench boost and the free hit each have their own switch, and
// so does the free-transfer taper in `decide`. None implies another and none is
// gated on a block-level bool. That is not tidiness: the likely end state has
// **chip placement on and banking off**, because the placement mechanism is close
// to arithmetic while banking's case rests on the reserved-exit and forced-move
// arguments, both unmeasured, in a harness this record says has **no configuration
// in which banking acts for a reason attributable to banking**. A future default
// that moves one lever and not the others is only expressible if the switches are
// independent, and a sweep that isolates one is only possible for the same reason.
//
// ⚠️ **The placement figure behind that sentence was ASSERTED and is now measured,
// and it was wrong.** An earlier draft said the fixed-offset control lands a chip
// on an ordinary week "about 78% of the time" with nothing having been run. The
// census, four seasons at six entry points, gives **83% pooled** over the three
// chips it places — and by chip **83% / 92% / 67%**, a spread this record would
// call a separate finding. `stats/findings/2026-08-17-chip-placement-census.md`.
// ⚠️ It is a **floor on how often the control is badly placed**, not an estimate
// of what better placement pays: "on the feature" asks only whether the round
// carries any doubling or blanking club, and the recorded verdict on calendar
// anchoring is still a clean null with an MDE of 34-37 per season-path.
//
// Everything still ships OFF. `TestTheOptionValueLeversAreIndependent` fails if a
// lever starts implying another.
//
// # What is shared, and what is not
//
// The BAR every rule is measured against comes from one place —
// `analysis.ChipBarAt`, which is `analysis.OptionValueAt` with a base price — so
// the decay curve has one implementation across all four levers. What each rule
// contributes is its own **reading**: the points a chip is worth in this gameweek,
// which is a different quantity per chip and cannot be shared.
//
// # Every reading is a model estimate, never a result
//
// `Week.BenchBoostGain` and `Week.TripleCaptainGain` are what the chip WOULD have
// been worth, computed after the gameweek is scored. They are hindsight and a
// trigger must not read them. Everything here is computed from the pre-deadline
// engine — the same reconstruction the transfer decision runs on — so a triggered
// chip is a decision a manager could have taken.
//
// ⚠️ That makes the readings **worse** than the diagnostic's, and the gap is the
// point: the recorded chip-timing figures are an oracle's, and a state rule is
// what a deployable policy can reach.

// chipTriggers is the per-season state of the chip state rules: which have fired,
// in which gameweek, and each one's mediator.
//
// One struct rather than three flags because the rules constrain each other —
// FPL allows one chip a week, so a fired bench boost has to be visible to the free
// hit's eligibility test — and because a fired chip must be visible to the scoring
// switch, which is a fourth reader.
type chipTriggers struct {
	cfg SimConfig
	// firedAll maps a chip slot to EVERY gameweek its rule played it in — one per
	// chip set, not one per season. See firedWeeks for why a single week was
	// wrong and how it hid.
	firedAll map[chipSlot][]int
	// warned is the once-per-run latch for warnZeroBar.
	warned map[chipSlot]bool
	// reset is the first gameweek of the second set for this run, or 0 when the
	// season grants one set. Resolved once at construction from the season's own
	// rule or the arm's ChipSets override, so the firing rule and SplitChipSetsWith
	// cannot disagree about where the boundary is.
	reset int
	// med is each rule's funnel. Pointers so a caller can accumulate into one
	// without re-storing it, which is the shape that lost a count once already.
	med map[chipSlot]*ChipTriggerMediator
}

func newChipTriggers(cfg SimConfig, season string) *chipTriggers {
	sets := cfg.ChipSets
	if sets <= 0 {
		sets = ChipSetsFor(season)
	}
	reset := 0
	if sets >= 2 {
		reset = ChipResetGW
	}
	t := &chipTriggers{
		cfg:      cfg,
		reset:    reset,
		firedAll: map[chipSlot][]int{},
		warned:   map[chipSlot]bool{},
		med:      map[chipSlot]*ChipTriggerMediator{},
	}
	for _, k := range []chipSlot{slotWildcard, slotFreeHit, slotBenchBoost} {
		t.med[k] = &ChipTriggerMediator{}
	}
	return t
}

// plays is whether this chip is played in this gameweek, by a plan or by a rule.
//
// It replaces `cfg.plays` at the sites a trigger can reach, and delegates for the
// plan half rather than re-reading the schedule. A second reading of "is a chip
// planned here" is exactly the copy `ChipSchedule` was written to abolish.
func (t *chipTriggers) plays(k chipSlot, gw int) bool {
	if t == nil {
		return false
	}
	if t.cfg.plays(k, gw) {
		return true
	}
	for _, w := range t.firedWeeks(k) {
		if w == gw {
			return true
		}
	}
	return false
}

// anyPlays is whether ANY chip occupies this gameweek, planned or fired.
//
// FPL allows one chip a week and `Simulate`'s scoring switch picks bench boost
// before triple captain, so two chips in one week silently drops the second.
// `ValidateChipSets` enforces that for plans and cannot see a rule's firing, which
// is why the rules enforce it themselves.
func (t *chipTriggers) anyPlays(gw int) bool {
	for _, k := range []chipSlot{
		slotWildcard, slotFreeHit, slotBenchBoost, slotTripleCaptain,
	} {
		if t.plays(k, gw) {
			return true
		}
	}
	return false
}

// eligible is whether a rule may be consulted for this chip in this gameweek.
//
// Four conditions, and each one is a way the rule could otherwise produce a season
// FPL would not have allowed:
//
//   - the rule is switched on;
//   - it has not already fired, since a chip is spent once;
//   - **no plan places this chip ANYWHERE in the season**, because a rule that
//     fires for a chip a plan also plays gives the season two of them, and
//     `ValidateChipSets` runs before the first gameweek so it cannot catch it;
//     ⚠️ this read `nextChip(k, gw) != 0` — planned from HERE ON — and that is
//     wrong in the one direction that matters: once the plan's own week is past,
//     the rule became eligible again and played a SECOND bench boost, in a season
//     that had already spent it. Found by
//     `TestATriggeredChipDoesNotCollideWithAPlannedOne` rather than reasoned
//     about, which is why that test exists;
//   - no other chip occupies this gameweek.
//
// ⚠️ It does NOT check the chip's window, because a window is a property of the
// SET and a trigger does not know which set it is spending until it fires. That is
// handled by the bar: `triggerWindow` gives the option the expiry of whichever set
// this gameweek falls in, so a first-set chip in a two-set season is priced against
// GW19 and the decay takes its bar to zero there.
// firedWeeks is every gameweek this rule has already spent the chip in.
//
// ⚠️ A slice, not a single week, because **FPL grants TWO sets** from 2025-26 and
// a rule that stops at one is not playing the game — it spends a first-half
// wildcard and then declines the second-half one, or holds out for the doubles
// and lets the first set expire unused. A set unplayed by the GW19 deadline is
// LOST, which the two-regime chip work measured as worth +11.9 to +15.6 a
// season-path on its own.
//
// The first version tracked one week per slot and every arm of the drift sweep
// therefore spent a single wildcard. That did not read as a bug: it read as
// "higher bars are better", because a low bar burned the only wildcard in GW14
// and left nothing for the second half. **The rule was answering a question
// nobody asked.**
func (t *chipTriggers) firedWeeks(k chipSlot) []int {
	if t == nil {
		return nil
	}
	return t.firedAll[k]
}

// setOfGW is which chip set a gameweek draws from. One set means every week is
// set 1, so a one-set season keeps the old once-a-season behaviour exactly.
func (t *chipTriggers) setOfGW(gw int) int {
	if t.reset > 0 && gw >= t.reset {
		return 2
	}
	return 1
}

// ⚠️ A trigger enabled with a ZERO bar fires on anything above zero, which is
// not a rule — it is "play the chip the first week it would help at all".
//
// It is silent, and it has already produced a wrong published figure. Two sweeps
// on 2026-08-26 set `WildcardTrigger = true` without a reservation, because
// `sweepConfig` does not map `config.OptionValue` into `SimConfig` — only
// `cmd/armband/optionvalue.go` does, for the live path — so `config.json`'s
// absent `option_value` block left the bar at 0. Both arms were labelled "the
// shipped rule". They fired at median GW9 and produced a banked mechanism claim
// that does not reproduce at the real bar of 12.
//
// So: name it. A caller that genuinely wants a zero bar says so with an explicit
// 0 and this stays quiet about it — `optionjoin_test.go` does exactly that — but
// a caller that simply forgot gets told once per run.
func (t *chipTriggers) warnZeroBar(k chipSlot, base float64) {
	if base != 0 || t.warned[k] {
		return
	}
	t.warned[k] = true
	fmt.Fprintf(os.Stderr, "⚠️ chip trigger %v enabled with a ZERO bar: it will "+
		"fire on any reading above zero, which is not the shipped rule. "+
		"sweepConfig does not map config.OptionValue into SimConfig — set "+
		"WildcardReservation (shipped: %.0f) explicitly.\n",
		k, config.DefaultWildcardReservation)
}

func (t *chipTriggers) eligible(k chipSlot, gw int, on bool) bool {
	if t == nil || !on {
		return false
	}
	// Counted BEFORE every guard below, which is the whole point of the column:
	// `ConsultedWeeks == 0` is also what a lever that ran and was blocked all
	// season looks like, and a plan owning the chip blocks it all season. See
	// ChipTriggerMediator.OfferedWeeks.
	if m := t.med[k]; m != nil {
		m.OfferedWeeks++
	}
	// Once per SET, not once per season — see firedWeeks. The season string is
	// not available here, so the set is resolved by the caller's own gameweek
	// against the same reset week SplitChipSetsWith uses.
	for _, w := range t.firedAll[k] {
		if t.setOfGW(w) == t.setOfGW(gw) {
			return false
		}
	}
	// Both sets, and the whole season rather than the remainder — `Weeks` asks a
	// bare slot name, which `ChipSchedule` reads across both sets. A conservative
	// refusal: a two-set season where the plan spends the first set and the rule
	// could legitimately spend the second is declined too. That is deliberate
	// until an arm needs it, because the alternative failure is a season playing
	// two of one chip and nothing downstream reporting it.
	if len(t.cfg.schedule().Weeks(k)) > 0 {
		return false
	}
	return !t.anyPlays(gw)
}

// consult records that a rule was asked, weighs a reading against the bar, and
// fires if it clears.
//
// `value` is what the chip is worth in this gameweek by the model's own estimate,
// and `ok` is whether that estimate could be formed at all — a squad that could
// not be priced, or a rebuild that failed, is "no reading" rather than a reading
// of zero, and the two license opposite conclusions.
//
// A strict `>`, so a tie declines. Same direction and same reason as
// `analysis.PreferWaiting`: playing costs the option irreversibly, and a rule that
// spent on equal value would trade a certainty for an estimate.
func (t *chipTriggers) consult(k chipSlot, gw int, season string, base, load float64,
	value float64, ok bool) bool {

	t.warnZeroBar(k, base)
	return t.consultAt(k, gw, value, ok, func() float64 {
		return analysis.ChipBarAt(base, triggerWindow(season, gw), gw, load, t.cfg.OptionPricing)
	})
}

// consultAt is `consult` with the bar supplied rather than derived, for a rule
// whose reading is not in the units `ChipBarAt` was fitted to.
//
// ⚠️ **This exists because a bar is calibrated against a QUANTITY, and swapping
// the quantity under it is not a change of units.** `ChipBarAt` prices a
// one-off repair cost in hit points and decays it through the option window;
// the XI-drift rule reads a PER-GAMEWEEK rate. Routing drift through that bar
// would compare a rate against a price and report the mismatch as a strategy
// result — the estimator-swap error this record has made twice in one day.
//
// The bar is a thunk so the expensive `ChipBarAt` call is still made only after
// `ok`, exactly as before.
func (t *chipTriggers) consultAt(k chipSlot, gw int, value float64, ok bool,
	barOf func() float64) bool {

	m := t.med[k]
	m.ConsultedWeeks++
	if !ok {
		return false
	}
	bar := barOf()
	m.WeighedWeeks++
	m.ValueSum += value
	m.BarSum += bar
	if value <= bar {
		return false
	}
	t.firedAll[k] = append(t.firedAll[k], gw)
	m.FiredGWs = append(m.FiredGWs, gw)
	// FIRST firing only. Overwriting made the scalar fields describe the second
	// set's firing in a two-set season while claiming to describe "the" one —
	// see ChipTriggerMediator.FiredGW.
	if m.FiredGW == 0 {
		m.FiredGW, m.FiredValue, m.FiredBar = gw, value, bar
	}
	return true
}

// triggerWindow is the option window a chip played in this gameweek would be
// spending: the set that gameweek falls in, and that set's expiry.
//
// The season gate lives here rather than in `analysis` because `ChipSetsFor` is
// this package's — the same placement `DefconScoredIn` and `BankLimitFor` have,
// and for the same reason: replaying a season under a rule nobody was playing
// under produces a number describing no game.
func triggerWindow(season string, gw int) analysis.OptionWindow {
	reset := 0
	if ChipSetsFor(season) >= 2 {
		reset = ChipResetGW
	}
	set := 2
	if reset > 0 && gw < reset {
		set = 1
	}
	return analysis.ChipExpiry(set, reset)
}

// repairCost is what it would cost in points, in hits, to reach the squad the
// model actually wants from the squad the manager holds.
//
// # Why a magnitude and not a transfer count
//
// "Do not build a state trigger for the wildcard" is a closed line, and its stated
// reason is that the tested trigger — the literal reading of *"cannot fix it with
// free transfers"* — **measures transfer scarcity rather than squad quality, so it
// fires at GW2 when the model has least data**. That reason is about the QUANTITY,
// not about state triggers: a count of moves needed is large whenever few
// transfers are in hand, whatever the squad is worth.
//
// A repair cost in points is not a move count. It is `4 x max(0, changes - free)`
// — the hits, and only the hits — so a squad the free transfers can reach costs
// nothing to repair however many moves it takes.
//
// ⚠️ **That did not rescue it. Measured 2026-08-17, the trigger still fires in the
// cell's second week in 8 of 8 cells** at the shipped reservation, over four
// seasons and two entry points (`TestDiagWildcardTrigger`). Costs are 20/36/24/32
// points at a GW1 entry and 12/12/12/4 at GW16 — points, and `free` is 2 at the
// firing week, so `cost/4 + 2` puts the implied player changes at 7/11/8/10 and
// 5/5/5/3. The recorded closure of the wildcard-trigger line stands.
//
// ⚠️ **WHY it fires is a hypothesis this diagnostic cannot settle.** Churn (a rate:
// the model has just re-scored everyone) and a standing held-versus-fresh gap (a
// level: any constrained fifteen sits some distance from an unconstrained argmax)
// both predict firing in week two, and the rule fires once and stops, so the cost is
// never observed as a series on a fixed squad. The level reading is the better
// supported — the GW16 cells fire with fifteen gameweeks behind them, and the one
// arm that sees the cost twice reads flat-to-rising — and it is what the prescription
// below assumes.
//
// ⚠️ **Do not raise the reservation to compensate** — at 20 it still fires within
// four weeks, because the cost being measured does not fall. What the mechanism
// wants is a different quantity: a repair cost against a squad the model still
// endorses, rather than against a fresh argmax over the whole pool. Unbuilt.
//
// # It costs a full rebuild every consulted week
//
// `Optimize` is the expensive call in this package and this makes one per week the
// rule is eligible in. That is affordable only because the lever ships off; an arm
// that turns it on pays for it. It is deliberately the SAME call `playWildcard`
// makes, minus the bench-boost flag, so the repair cost describes the squad the
// wildcard would actually build rather than a second opinion about it.
//
// `ok` is false when the rebuild fails — the pool is empty, the budget cannot be
// established — which is no reading rather than a repair cost of zero. A zero
// would fire nothing and read identically to a squad in perfect shape.
func repairCost(e *analysis.Engine, cur *Season, wal *wallet, held []int, gw, free int,
	minExp float64, cfg SimConfig) (cost float64, ok bool) {

	cost, _, _, _, ok = repairCostAndDrift(e, cur, wal, held, gw, free, minExp, cfg)
	return cost, ok
}

// repairCostAndDrift is `repairCost` plus the XI's expected-points distance from
// the same fresh optimum, from ONE `Optimize` call.
//
// # Why both, and why from one call
//
// `repairCost` prices the repair as `4 x max(0, changes - free)` — the hits it
// would take to reach the optimum by transfers. That is a MOVE COUNT wearing
// points, and it treats a 4.0m bench swap exactly like losing a captain. The
// user's objection is the reason `xidrift.go` exists: *"switching a benched
// player is basically never worth a transfer."*
//
// `repairChanges` already builds the fresh fifteen and discards it, so the drift
// costs nothing beyond two `BestXI` searches — and `Optimize` is the expensive
// call in this package, so a second one would have been the whole cost.
//
// ⚠️ **The drift does NOT decide anything yet, deliberately.** The two quantities
// are not interchangeable: the cost is a ONE-OFF hit price and the drift is a
// PER-GAMEWEEK rate, and `ChipBarAt` was calibrated against the first. Swapping
// the estimator under a bar fitted to the other is the error this record has
// already made twice in one day. Measure the disagreement first, then decide.
// ⚠️ **It returns the FRESH FIFTEEN and its change count too**, which the
// lookahead rule needs and which this function already builds and used to throw
// away. `Optimize` is the expensive call in this package, so a caller that
// rebuilt the optimum to get at it would have paid twice for the one call this
// function exists to make once. `changes` is returned already resolved through
// `RepairCountsXIOnly`, so a caller cannot pick the wrong count by accident.
func repairCostAndDrift(e *analysis.Engine, cur *Season, wal *wallet, held []int,
	gw, free int, minExp float64, cfg SimConfig) (cost, drift float64, fresh []int, changes int, ok bool) {

	budget := wal.value(cur, held, gw-1)
	fresh, ok = repairSquad(e, held, budget, minExp, cfg)
	if !ok {
		return 0, 0, nil, 0, false
	}
	// ⚠️ WHICH COUNT feeds the hit price is a config choice, because the raw one
	// is measurably wrong for it: fired on `changesBetween`, the shipped wildcard
	// rule leaves the policy taking MORE hits. See changesInXI.
	changes = changesBetween(held, fresh)
	if cfg.RepairCountsXIOnly {
		changes = changesInXI(e, held, fresh)
	}
	return repairCostOf(changes, free), xiPoints(e, fresh) - xiPoints(e, held), fresh, changes, true
}

// repairChanges is how many of the held fifteen a fresh unconstrained optimum
// would replace, at a stated budget.
//
// Split out of `repairCost` because the change count and the hit price are two
// different quantities and the observer that reports the repair cost as a time
// series wants both — a cost is `max(0, changes - free)` clipped and scaled, so a
// series in cost alone cannot be told from a series in the allowance. One
// implementation rather than two: `repairCost` is this plus the price, and
// nothing else counts held-versus-fresh distance.
//
// The BUDGET is a parameter rather than derived here, for the one reason a caller
// would legitimately want a different one: `wallet.value` is the squad's SELLING
// value, so FPL's half-of-any-rise rule is already charged on every held player,
// and an observer asking how much of a standing gap is that friction has to be
// able to price the same squad at market. That is a diagnostic's question and it
// must not become a second definition of what the trigger reads.
//
// ⚠️ It is an argmax over the whole pool, so `analysis.Engine.Optimize` is the
// expensive call — one per invocation, never nested inside a per-candidate loop.
func repairChanges(e *analysis.Engine, held []int, budget int, minExp float64,
	cfg SimConfig) (int, bool) {

	fresh, ok := repairSquad(e, held, budget, minExp, cfg)
	if !ok {
		return 0, false
	}
	return changesBetween(held, fresh), true
}

// repairSquad is the fresh unconstrained optimum itself, alongside which
// `repairChanges` counts how much of `held` it would replace. One
// implementation: both quantities come from this single `Optimize` call, and
// the worldview-rewrite column (`RepairWeek.FreshChurn`) needs the squad, not
// merely the count.
func repairSquad(e *analysis.Engine, held []int, budget int, minExp float64,
	cfg SimConfig) ([]int, bool) {

	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: budget, MinMinutes: 600,
		MinExpectedMinutes: minExp, BenchWeight: cfg.openingBenchWeight(),
	})
	if err != nil {
		return nil, false
	}
	fresh := make([]int, 0, 15)
	for _, p := range sq.Players {
		fresh = append(fresh, p.ID)
	}
	return fresh, true
}

// changesBetween counts the held players absent from the fresh fifteen.
func changesBetween(held, fresh []int) int {
	want := make(map[int]bool, len(fresh))
	for _, id := range fresh {
		want[id] = true
	}
	changes := 0
	for _, id := range held {
		if !want[id] {
			changes++
		}
	}
	return changes
}

// repairCostOf prices a change count in points: the hits, and only the hits.
func repairCostOf(changes, free int) float64 {
	hits := changes - free
	if hits < 0 {
		hits = 0
	}
	return HitCost * float64(hits)
}

// observeRepair takes one gameweek's reading of the held-versus-fresh distance,
// on the evolving fifteen and on the frozen opening one, and prices the frozen
// one a second time with the selling tax off.
//
// **It decides nothing.** It is handed the engine the decision will run on and
// returns a struct; the caller appends it to the result and no branch reads it.
// That is the whole design point — a repair cost that could act is the wildcard
// state trigger, which is a closed line.
//
// Three `Optimize` calls, one per reading, none nested: `Optimize` is the
// expensive call in this package and a per-candidate loop around it is what makes
// a diagnostic unaffordable. The three differ only in the BUDGET they are given,
// so they cannot be shared — a different budget is a different knapsack.
//
// `evolveFree` is the allowance the week's decision will actually hold and
// `frozenFree` the allowance an arm that never transfers would carry. Both are
// passed in rather than derived, because the accrual rule lives in `decide` and a
// second expression of it here is the drift this package keeps paying for.
// The second return is this week's fresh optimum, handed back so the next
// week can measure how far it moves; nil when the rebuild failed.
func observeRepair(e *analysis.Engine, cur *Season, wal, frozenWal *wallet,
	held, opening, prevFresh []int, gw, evolveFree, frozenFree int, minExp float64,
	cfg SimConfig) (RepairWeek, []int) {

	// The allowance the week will actually have. Reading `free` raw would price
	// the repair against one transfer too few — the same correction the trigger
	// site carries, and for the same reason.
	avail := evolveFree
	if avail < cfg.BankUpTo {
		avail++
	}
	obs := RepairWeek{GW: gw, Free: avail, FrozenFree: frozenFree}

	obs.Budget = wal.value(cur, held, gw-1)
	// The fresh squad is kept, not merely counted: its week-to-week movement is
	// the worldview-rewrite column, and the EVOLVING arm computes it anyway.
	fresh, ok := repairSquad(e, held, obs.Budget, minExp, cfg)
	obs.OK = ok
	if obs.OK {
		obs.Changes = changesBetween(held, fresh)
		obs.Cost = repairCostOf(obs.Changes, obs.Free)
		if prevFresh != nil {
			obs.FreshChurn = changesBetween(prevFresh, fresh)
			obs.FreshChurnOK = true
		}
	}

	obs.FrozenBudget = frozenWal.value(cur, opening, gw-1)
	obs.FrozenChanges, obs.FrozenOK = repairChanges(e, opening, obs.FrozenBudget, minExp, cfg)
	if obs.FrozenOK {
		obs.FrozenCost = repairCostOf(obs.FrozenChanges, obs.FrozenFree)
	}

	// The same frozen fifteen at MARKET value: what the budget would be if a sale
	// raised the headline price rather than what was paid plus half of any rise.
	// The gap between this change count and the one above is the friction channel,
	// and it is the obvious confound for a frozen series that rises — the tax on a
	// squad that never sells grows all season.
	obs.FrozenGrossBudget = frozenWal.bank
	for _, id := range opening {
		if cur.Players[id] != nil {
			obs.FrozenGrossBudget += marketPrice(cur, id, gw-1)
		}
	}
	obs.FrozenGrossChanges, obs.FrozenGrossOK =
		repairChanges(e, opening, obs.FrozenGrossBudget, minExp, cfg)
	return obs, fresh
}

// squadBlanks reports whether any club the squad holds plays no match in this
// gameweek.
//
// The free-hit rule's pre-filter. It is a **necessary condition** rather than a
// heuristic: the chip fields a temporary fifteen for one round and hands the
// permanent squad straight back, so its entire value is having players who play
// where the held squad's do not. In a round where every owned club has a fixture,
// no rebuild can be worth a chip — the held squad is already eleven playing
// footballers, and a marginally better eleven is what ordinary transfers are for.
//
// It exists for cost. `freeHitValue` runs a full `Optimize`, the expensive call in
// this package, and the free-hit rule never becomes ineligible on its own — so
// without this the lever pays for roughly 37 rebuilds a cell. Read off
// `fixtureLoadAfter` at the club level, which is the same fixture-count machinery
// the scoring path uses, so it inherits the anchor fix that made a blank
// expressible at all.
//
// ⚠️ **It skips the REBUILD, not the reading**, so a filtered week is not counted
// as weighed and the mediator's `consulted > weighed` gap says how often this
// fired. Reporting a filtered week as weighed-and-refused would claim a reading
// nobody computed.
// It delegates to `analysis.Engine.SquadHasABlank` rather than counting fixtures
// here: which round is imminent is the engine's own idea, and a second one in this
// package would disagree with it the first time a skip set was set.
func squadBlanks(e *analysis.Engine, held []int) bool {
	return e.SquadHasABlank(held)
}

// benchBoostValue is what playing the boost this gameweek is worth by the model's
// estimate: the four benched players' expected points.
//
// Read off a horizon-1 engine, because the chip pays in ONE gameweek. Reading it
// off the five-week engine would dilute a doubling bench across four weeks it will
// never be played in — the same error `fixtureLoadFor`'s confinement to the
// horizon-1 view exists to avoid, arriving through the chip instead of the eleven.
//
// ⚠️ **A double is worth less than twice one match to a bench place, and the
// appearance channel is why.** A double adds **+1.84 appearance points against a
// theoretical +2** for a nailed elite player, and only **+1.32** for a premium
// selected on prior information — so a doubles-targeting rule cannot claim the
// tight detection threshold a certain mechanism would earn. That sizes ONE input
// to this reading and is not a verdict on this lever; nothing here is weakened on
// the strength of it.
//
// `ok` is false when no player resolves, which is a broken reconstruction rather
// than a worthless bench.
func benchBoostValue(e *analysis.Engine, bench []int) (float64, bool) {
	if len(bench) == 0 {
		return 0, false
	}
	sum, n := 0.0, 0
	for _, id := range bench {
		el := e.Boot.ElementByID(id)
		if el == nil {
			continue
		}
		sum += e.Metrics(el).Score
		n++
	}
	if n == 0 {
		return 0, false
	}
	return sum, true
}

// freeHitValue is what fielding a temporary fifteen this gameweek is worth: the
// best eleven the free-hit build could field, less the best eleven the permanent
// squad could.
//
// Both elevens are valued on the SAME horizon-1 engine, which is what makes the
// difference a difference rather than two numbers from two models. The free-hit
// builder is the shipped one — `freeHitSquad`, blank-guarded through the pool
// rather than only through the objective — so the reading describes the squad the
// chip would actually field.
//
// ⚠️ **`analysis.XIValue` counts the armband again**, which is what the transfer
// objective scores a squad by. That is the right unit for a comparison of two
// fifteens the manager might field, and it is NOT a points prediction: it
// over-states a week by roughly one captain. Both sides carry it, so the
// difference is unaffected; a level read off this would not be.
//
// ⚠️ **The default bar of 16 is inherited from the bench boost across a DIFFERENT
// QUANTITY, and whether it lies in a live region is unknown.** `benchBoostValue`
// sums four benched players' `Score` — a bounded thing, roughly 8 to 14 — while
// this is a fresh argmax over the whole pool less the held squad, which is the same
// held-versus-fresh gap the wildcard diagnostic found dominating the repair cost.
// There is no reason the two live on one scale, and "carrying 16 across keeps the
// level constant so the change under test is the SHAPE" holds for the bench boost
// and **not** for this. `fh_trig_weighed` and `fh_trig_value` answer it the first
// time the lever runs: a bar far below the readings fires in week one, one far
// above never fires, and the mediator says which happened.
//
// `ok` is false when the temporary squad cannot be built, for the reason
// `freeHitSquad`'s own caller documents: a swallowed build failure makes a chip
// that did not run indistinguishable from one that was worth nothing.
func freeHitValue(e *analysis.Engine, cur *Season, wal *wallet, held []int, gw int,
	minExp float64, cfg SimConfig) (float64, bool) {

	temp, err := freeHitSquad(e, cur, wal, held, gw, minExp, cfg)
	if err != nil {
		return 0, false
	}
	value := func(ids []int) float64 {
		var squad []analysis.PlayerMetrics
		for _, id := range ids {
			if el := e.Boot.ElementByID(id); el != nil {
				squad = append(squad, e.Metrics(el))
			}
		}
		xi, _, _ := analysis.BestXI(squad)
		return analysis.XIValue(xi)
	}
	return value(temp) - value(held), true
}

// wildcardLookahead is how many gameweeks the lookahead wildcard rule prices
// across. Zero takes the decision horizon — the window the ordinary transfer
// decision is already judged over, so the chip is not being asked about a run of
// fixtures the rest of the engine cannot see.
func (c SimConfig) wildcardLookahead() int {
	if c.WildcardLookahead > 0 {
		return c.WildcardLookahead
	}
	return c.decisionHorizon()
}
