package backtest

import (
	"armband/internal/analysis"
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
// to arithmetic — the census measured the fixed-offset control landing a chip on
// an ordinary week about 78% of the time, so the gap between good and bad
// placement is large — while banking's case rests on the reserved-exit and
// forced-move arguments, both unmeasured, in a harness this record says has **no
// configuration in which banking acts for a reason attributable to banking**. A
// future default that moves one lever and not the others is only expressible if
// the switches are independent, and a sweep that isolates one is only possible for
// the same reason.
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
	// fired maps a chip slot to the gameweek its rule played it in, 0 for a rule
	// that has not fired.
	fired map[chipSlot]int
	// med is each rule's funnel. Pointers so a caller can accumulate into one
	// without re-storing it, which is the shape that lost a count once already.
	med map[chipSlot]*ChipTriggerMediator
}

func newChipTriggers(cfg SimConfig) *chipTriggers {
	t := &chipTriggers{
		cfg:   cfg,
		fired: map[chipSlot]int{},
		med:   map[chipSlot]*ChipTriggerMediator{},
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
	return t.cfg.plays(k, gw) || (t.fired[k] != 0 && t.fired[k] == gw)
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
func (t *chipTriggers) eligible(k chipSlot, gw int, on bool) bool {
	if t == nil || !on {
		return false
	}
	if t.fired[k] != 0 {
		return false
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

	m := t.med[k]
	m.ConsultedWeeks++
	if !ok {
		return false
	}
	bar := analysis.ChipBarAt(base, triggerWindow(season, gw), gw, load, t.cfg.OptionPricing)
	m.WeighedWeeks++
	m.ValueSum += value
	m.BarSum += bar
	if value <= bar {
		return false
	}
	t.fired[k] = gw
	m.FiredGW, m.FiredValue, m.FiredBar = gw, value, bar
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
// seasons and two entry points (`TestDiagWildcardTrigger`). The magnitude it
// measures is **model churn, not squad decay**: one gameweek after the fifteen is
// bought at the model's own optimum, that optimum has moved by five to nine
// players, so the cost reads 20 to 36 points in exactly the week the model knows
// least. The recorded closure of the wildcard-trigger line stands.
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

	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: wal.value(cur, held, gw-1), MinMinutes: 600,
		MinExpectedMinutes: minExp, BenchWeight: cfg.openingBenchWeight(),
	})
	if err != nil {
		return 0, false
	}
	want := make(map[int]bool, 15)
	for _, p := range sq.Players {
		want[p.ID] = true
	}
	changes := 0
	for _, id := range held {
		if !want[id] {
			changes++
		}
	}
	hits := changes - free
	if hits < 0 {
		hits = 0
	}
	return HitCost * float64(hits), true
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
