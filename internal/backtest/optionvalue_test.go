package backtest

import (
	"path/filepath"
	"reflect"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// The option-value levers' guards.
//
// Four things are pinned here, and each of them is a way the build could have
// looked finished and done nothing:
//
//   - **every default reproduces the shipped behaviour**, so no banked figure in
//     `stats/snapshots/` becomes non-comparable with a figure taken after this;
//   - **every lever is independently switchable**, because the likely end state
//     has chip placement on and banking off, and a future default that moves one
//     and not the others is only expressible if none implies another;
//   - **the schema positions**, because a column dropped between two counted
//     blocks is invisible to a test indexing from either end;
//   - **the mediators nest**, so a zero in one is attributable.

// TestTheOptionValueLeversAreOffByDefault is invariant 1: shipped defaults must
// be byte-identical to the behaviour before this change.
//
// ⚠️ **It asserts on the CONFIG defaults and on SimConfig's zero value, which
// are two different things and both matter.** The replay never reads
// `config.Config` — every sweep builds a `SimConfig` literal — so a default that
// was safe in the file and unsafe in the struct would leave every sweep changed
// while this test passed on the half nobody replays.
func TestTheOptionValueLeversAreOffByDefault(t *testing.T) {
	d := config.Default().OptionValue
	for _, c := range []struct {
		name string
		on   bool
	}{
		{"taper_free_transfer_value", d.TaperFreeTransferValue},
		{"wildcard trigger", d.Wildcard.Enabled},
		{"bench boost trigger", d.BenchBoost.Enabled},
		{"free hit trigger", d.FreeHit.Enabled},
	} {
		if c.on {
			t.Errorf("%s ships ON. Every lever here must default off: the replay "+
				"compares arms, so off-by-default costs nothing in power, while "+
				"defaulting on makes every banked figure taken before this change "+
				"non-comparable with every one taken after it.", c.name)
		}
	}
	if d.Any() {
		t.Errorf("config.Default().OptionValue.Any() is true with every lever off")
	}

	// The replay's own zero value. A sweep that sets nothing must run the shipped
	// policy, and each of these is the field the corresponding lever branches on.
	var z SimConfig
	if z.TaperFreeTransferValue || z.WildcardTrigger ||
		z.BenchBoostTrigger || z.FreeHitTrigger {
		t.Errorf("a zero SimConfig has an option-value lever on: %+v", z)
	}
	// And the curve's own no-ops, which is what makes an unset knob a no-op
	// rather than a zero — the rule a raw multiply by an unset congestion penalty
	// broke for two seasons of replays.
	if got := analysis.TransferHoldFactor(analysis.OptionWindow{}, 1, 1,
		analysis.OptionPricing{}); got != 1 {
		t.Errorf("TransferHoldFactor on an unusable window is %v, want 1 — an "+
			"unknown window must leave the charge alone, not zero it", got)
	}
	if got := analysis.MoveLimit(2, 1, 0, 0); got != 3 {
		t.Errorf("MoveLimit with an unset ceiling is %d, want 3 — zero means the "+
			"shipped default, not a ban on hits", got)
	}
}

// TestTheOptionValueLeversAreIndependent is invariant 2 of the four above: no
// lever may imply another, and none may be gated on a block-level flag.
//
// # Why this is a test and not a convention
//
// The end state this build is aimed at is **chip placement on and banking off**,
// arrived at by measurement. Chip placement rests on near-arithmetic mechanism —
// the fixed-offset control lands a chip on an ordinary week about 78% of the time,
// so the gap between good and bad placement is large — while banking's case rests
// on the reserved-exit and forced-move arguments, both unmeasured, in a harness
// this record says has no configuration where banking acts for a reason
// attributable to banking. A master switch would make that end state
// inexpressible, and a master switch is exactly what somebody tidying four bools
// into one reaches for.
//
// It checks the CONSUMERS rather than the struct: each lever is set alone and the
// three others' branches must stay unreachable. Naming the consumer is the check;
// naming a package is not.
func TestTheOptionValueLeversAreIndependent(t *testing.T) {
	for _, c := range []struct {
		name string
		set  func(*SimConfig)
		// others reports whether any lever the arm did NOT set is live.
		others func(SimConfig) bool
	}{
		{"taper", func(c *SimConfig) { c.TaperFreeTransferValue = true },
			func(c SimConfig) bool {
				return c.WildcardTrigger || c.BenchBoostTrigger || c.FreeHitTrigger
			}},
		{"wildcard", func(c *SimConfig) { c.WildcardTrigger = true },
			func(c SimConfig) bool {
				return c.TaperFreeTransferValue || c.BenchBoostTrigger || c.FreeHitTrigger
			}},
		{"bench boost", func(c *SimConfig) { c.BenchBoostTrigger = true },
			func(c SimConfig) bool {
				return c.TaperFreeTransferValue || c.WildcardTrigger || c.FreeHitTrigger
			}},
		{"free hit", func(c *SimConfig) { c.FreeHitTrigger = true },
			func(c SimConfig) bool {
				return c.TaperFreeTransferValue || c.WildcardTrigger || c.BenchBoostTrigger
			}},
	} {
		var sc SimConfig
		c.set(&sc)
		if c.others(sc) {
			t.Errorf("setting the %s lever turned another on: %+v", c.name, sc)
		}
		// And the trigger machinery agrees: with one lever on, the other two
		// chips are ineligible in every gameweek whatever the calendar says.
		trig := newChipTriggers(sc)
		for _, k := range []struct {
			slot chipSlot
			on   bool
		}{
			{slotWildcard, sc.WildcardTrigger},
			{slotBenchBoost, sc.BenchBoostTrigger},
			{slotFreeHit, sc.FreeHitTrigger},
		} {
			if got := trig.eligible(k.slot, 5, k.on); got != k.on {
				t.Errorf("with only the %s lever set, chip %q is eligible=%v and "+
					"its switch is %v", c.name, k.slot, got, k.on)
			}
		}
	}
}

// TestTheOptionBlockSitsBetweenTheFixtureRunsAndTheDose pins the schema position,
// in the mould of every other block's assertion in this package.
func TestTheOptionBlockSitsBetweenTheFixtureRunsAndTheDose(t *testing.T) {
	want := []string{
		"ftv_weeks", "ftv_priced_weeks", "ftv_gate_calls", "ftv_flips",
		"ftv_mean_charge", "ftv_mean_load",
		"wc_trig_weeks", "wc_trig_weighed", "wc_trig_gw", "wc_trig_value", "wc_trig_bar",
		"bb_trig_weeks", "bb_trig_weighed", "bb_trig_gw", "bb_trig_value", "bb_trig_bar",
		"fh_trig_weeks", "fh_trig_weighed", "fh_trig_gw", "fh_trig_value", "fh_trig_bar",
		"prep_weeks", "prep_credit_weeks", "prep_bench_sum", "prep_captain_sum",
	}
	if optionCols != len(want) {
		t.Fatalf("optionCols is %d and the block is %d columns", optionCols, len(want))
	}
	at := optionBlockAt()
	if got := cellHeader[at : at+optionCols]; !reflect.DeepEqual(got, want) {
		t.Fatalf("the %d option-value columns are %v, want %v", optionCols, got, want)
	}
	if before := cellHeader[at-1]; before != "band_exposure" {
		t.Fatalf("the column before the option block is %q, want the fixture-run "+
			"block's last column — the three funnels are one region", before)
	}
	if after := cellHeader[at+optionCols]; after != "dose_act_doubles" {
		t.Fatalf("the column after the option block is %q, want the dose block's "+
			"first column", after)
	}
	if n := len(withoutOptionBlock()); n != len(cellHeader)-optionCols {
		t.Fatalf("withoutOptionBlock has %d columns, want %d",
			n, len(cellHeader)-optionCols)
	}
}

// TestTheDoseBlockSitsBetweenTheOptionFunnelsAndTheChips pins the other new
// block, and states in an assertion the thing its comment says: the dose is a
// function of the season and the entry gameweek alone, so it is NOT a mediator.
func TestTheDoseBlockSitsBetweenTheOptionFunnelsAndTheChips(t *testing.T) {
	want := []string{
		"dose_act_doubles", "dose_act_blanks",
		"dose_late_doubles", "dose_late_blanks",
	}
	if doseCols != len(want) {
		t.Fatalf("doseCols is %d and the block is %d columns", doseCols, len(want))
	}
	at := doseBlockAt()
	if got := cellHeader[at : at+doseCols]; !reflect.DeepEqual(got, want) {
		t.Fatalf("the %d dose columns are %v, want %v", doseCols, got, want)
	}
	if before := cellHeader[at-1]; before != "prep_captain_sum" {
		t.Fatalf("the column before the dose block is %q, want the option block's "+
			"last column", before)
	}
	if after := cellHeader[at+doseCols]; after != "bench_boost_gw" {
		t.Fatalf("the column after the dose block is %q, want the chip block's "+
			"first column", after)
	}
	if n := len(withoutDoseBlock()); n != len(cellHeader)-doseCols {
		t.Fatalf("withoutDoseBlock has %d columns, want %d",
			n, len(cellHeader)-doseCols)
	}
	// The dose survives an infeasible cell, unlike every mediator beside it. That
	// is the arm rule rather than an oversight: an infeasible cell still has a
	// season and an entry gameweek, so it still has a dose, and clearing it would
	// report a cell with no doubles in a season that had them.
	inf := cellRow{HasDose: true, ActDoubles: 7, LateBlanks: 3,
		HasBanking: true, TransferHold: TransferHoldMediator{ConsultedWeeks: 9}}.
		asInfeasible()
	if !inf.HasDose || inf.ActDoubles != 7 || inf.LateBlanks != 3 {
		t.Errorf("asInfeasible cleared the dose block: %+v", inf)
	}
	if inf.TransferHold.ConsultedWeeks != 0 {
		t.Errorf("asInfeasible left the taper funnel populated on a cell that "+
			"played no gameweek: %+v", inf.TransferHold)
	}
}

// TestTheOptionFunnelsNest pins the inequalities every reading of the block rests
// on, in the mould of TestTheBankingFunnelNests.
//
// ⚠️ **`ftv_flips <= ftv_gate_calls` and NOT `<= ftv_weeks`.** A week asks the
// gate once per candidate, so the flip count has its own denominator; asserting
// one chain all the way down would fail on any week that offered a funded pair and
// a solo swap, for a reason that is not a defect.
func TestTheOptionFunnelsNest(t *testing.T) {
	for _, c := range []struct {
		name string
		row  cellRow
		bad  bool
	}{
		{"priced beyond consulted", cellRow{
			TransferHold: TransferHoldMediator{ConsultedWeeks: 3, PricedWeeks: 4}}, true},
		{"flips beyond gate calls", cellRow{
			TransferHold: TransferHoldMediator{ConsultedWeeks: 3, GateCalls: 2, Flips: 3}}, true},
		{"flips beyond weeks is fine", cellRow{
			TransferHold: TransferHoldMediator{ConsultedWeeks: 3, GateCalls: 9, Flips: 5}}, false},
		{"weighed beyond consulted", cellRow{
			WildcardTrig: ChipTriggerMediator{ConsultedWeeks: 2, WeighedWeeks: 3}}, true},
		{"fired without weighing", cellRow{
			WildcardTrig: ChipTriggerMediator{ConsultedWeeks: 2, WeighedWeeks: 0, FiredGW: 7}}, true},
		{"credit beyond consulted", cellRow{
			ChipPrep: ChipPrepMediator{ConsultedWeeks: 1, CreditWeeks: 2}}, true},
		{"an ordinary row", cellRow{
			TransferHold: TransferHoldMediator{ConsultedWeeks: 30, PricedWeeks: 30,
				GateCalls: 61, Flips: 4},
			WildcardTrig: ChipTriggerMediator{ConsultedWeeks: 30, WeighedWeeks: 30, FiredGW: 12},
			ChipPrep:     ChipPrepMediator{ConsultedWeeks: 30, CreditWeeks: 5}}, false},
	} {
		if got := optionFunnelBroken(c.row); got != c.bad {
			t.Errorf("%s: broken=%v, want %v", c.name, got, c.bad)
		}
	}
}

// optionFunnelBroken is the nesting rule, in one place so the test above and any
// diagnostic reading a banked file cannot disagree about what a valid row is.
func optionFunnelBroken(r cellRow) bool {
	if r.TransferHold.PricedWeeks > r.TransferHold.ConsultedWeeks {
		return true
	}
	if r.TransferHold.Flips > r.TransferHold.GateCalls {
		return true
	}
	if r.ChipPrep.CreditWeeks > r.ChipPrep.ConsultedWeeks {
		return true
	}
	for _, m := range []ChipTriggerMediator{r.WildcardTrig, r.BenchBoostTrig, r.FreeHitTrig} {
		if m.WeighedWeeks > m.ConsultedWeeks {
			return true
		}
		if m.FiredGW > 0 && m.WeighedWeeks == 0 {
			return true
		}
	}
	return false
}

// TestTheOptionValueCurveHasTheShapeItClaims pins the three properties
// analysis.OptionDecay's comment asserts, because each one is a claim rather than
// an implementation detail and a "simplification" could quietly drop any of them.
func TestTheOptionValueCurveHasTheShapeItClaims(t *testing.T) {
	const k = 8
	// Exactly zero at expiry. Not small — zero. An option that cannot be
	// exercised again is worth nothing to hold, and this is the one boundary the
	// model is confident about.
	if got := analysis.OptionDecay(0, k); got != 0 {
		t.Errorf("decay at expiry is %v, want exactly 0", got)
	}
	if got := analysis.TransferHoldFactor(analysis.TransferExpiry(), 38, 1,
		analysis.OptionPricing{}); got != 0 {
		t.Errorf("a transfer held at GW38 is charged x%v, want x0 — it can never "+
			"be spent, so holding it is worth nothing", got)
	}
	// Monotone, and saturating below 1.
	prev := -1.0
	for r := 0.0; r <= 40; r++ {
		got := analysis.OptionDecay(r, k)
		if got < prev {
			t.Fatalf("decay is not monotone: %v at %v after %v", got, r, prev)
		}
		if got >= 1 {
			t.Fatalf("decay reached %v at %v gameweeks; it must approach 1 and "+
				"never reach it", got, r)
		}
		prev = got
	}
	// Half at the half-life, which is what the constant's name claims.
	if got := analysis.OptionDecay(k, k); got != 0.5 {
		t.Errorf("decay at the half-life is %v, want 0.5", got)
	}
	// Congestion rises with fixture density and floors at zero rather than going
	// negative. A blank window means no forced demand, not negative demand.
	if a, b := analysis.CongestionFactor(1, 1), analysis.CongestionFactor(2, 1); b <= a {
		t.Errorf("congestion does not rise with load: %v at 1.0 against %v at 2.0", a, b)
	}
	if got := analysis.CongestionFactor(0, 4); got != 0 {
		t.Errorf("congestion at a total blank with sensitivity 4 is %v, want 0 — "+
			"floored, not negative", got)
	}
}

// TestAFirstSetChipsBarExpiresAtTheReset pins the half of the window rule that
// the season gate owns, and it is the one a reader is most likely to get wrong:
// the expiry of a chip is a property of the SET, not of the season's end.
func TestAFirstSetChipsBarExpiresAtTheReset(t *testing.T) {
	// A one-set season: everything runs to GW38 whatever week it is played in.
	for _, gw := range []int{1, 19, 20, 38} {
		if got := triggerWindow("2023-24", gw).Expiry; got != 38 {
			t.Errorf("2023-24 GW%d expiry is %d, want 38 — that season granted "+
				"one set, which runs the whole way", gw, got)
		}
	}
	// A two-set season: before the reset a chip is spending the first set, which
	// expires after GW19, so its bar must be zero AT GW19.
	if got := triggerWindow("2025-26", 5).Expiry; got != ChipResetGW-1 {
		t.Errorf("2025-26 GW5 expiry is %d, want %d", got, ChipResetGW-1)
	}
	if got := triggerWindow("2025-26", ChipResetGW).Expiry; got != 38 {
		t.Errorf("2025-26 GW%d expiry is %d, want 38 — from the reset a chip is "+
			"spending the second set", ChipResetGW, got)
	}
	bar := analysis.ChipBarAt(16, triggerWindow("2025-26", ChipResetGW-1),
		ChipResetGW-1, 1, analysis.OptionPricing{})
	if bar != 0 {
		t.Errorf("the first set's bar at GW%d is %v, want 0 — the set expires "+
			"after that week and an unplayed chip is simply lost",
			ChipResetGW-1, bar)
	}
}

// TestASplitChipPlanIsAccepted is Phase 0's other repair, and it is a
// silent-cell-loss regression rather than a style fix.
//
// `anchoredPlan` puts 2025-26's chips at GW33 and GW34. `ValidateChipSets` refuses
// a FIRST-set chip at or after ChipResetGW, and `runPolicySweep` records the
// refusal as an infeasible cell rather than fatalling — so an anchored arm lost
// **all six 2025-26 cells** while every printed number stayed plausible.
func TestASplitChipPlanIsAccepted(t *testing.T) {
	late := analysis.ChipPlan{BenchBoost: 33, FreeHit: 34}
	if err := ValidateChipSets("2025-26", late, analysis.ChipPlan{}); err == nil {
		t.Fatalf("the unsplit plan is accepted, so this test no longer exercises " +
			"the refusal it exists for")
	}
	sch := SplitChipSets("2025-26", late)
	if err := ValidateChipSets("2025-26", sch.First, sch.Second); err != nil {
		t.Errorf("the split plan is still refused: %v", err)
	}
	if sch.Second.BenchBoost != 33 || sch.Second.FreeHit != 34 {
		t.Errorf("the late chips did not move into the second set: %+v", sch)
	}
	if sch.First != (analysis.ChipPlan{}) {
		t.Errorf("the first set carries %+v, want nothing", sch.First)
	}
	// A one-set season is the identity, which is what makes it safe to apply to
	// every planner rather than to the ones somebody remembered.
	for _, season := range []string{"2020-21", "2022-23", "2024-25"} {
		p := analysis.ChipPlan{Wildcard: 4, BenchBoost: 26, FreeHit: 29, TripleCaptain: 34}
		got := SplitChipSets(season, p)
		if got.First != p || got.Second != (analysis.ChipPlan{}) {
			t.Errorf("%s: SplitChipSets is not the identity: %+v", season, got)
		}
	}
	// And it does not hide a plan that is illegal for a second reason. Two chips
	// in one gameweek is still two chips in one gameweek — the split moves a chip
	// between sets and never moves its week.
	clash := analysis.ChipPlan{BenchBoost: 30, FreeHit: 30}
	cs := SplitChipSets("2025-26", clash)
	if err := ValidateChipSets("2025-26", cs.First, cs.Second); err == nil {
		t.Errorf("a plan playing two chips in GW30 was accepted after the split; " +
			"the split must not repair a collision it cannot see")
	}
}

// TestTheHitCeilingIsReachableAndDefaultsToOne is Phase 0's first repair.
//
// `analysis.MoveLimit` clamped the hit allowance to 1 UNCONDITIONALLY, so an arm
// at `MaxHits: 2` was byte-identical to shipped and read as a null — and the
// funded-pair branch carried the same clamp as a literal `hitsNeeded <= 1`, so
// lifting one without the other would have widened the limit while the pair
// refused anything that used the extra move.
func TestTheHitCeilingIsReachableAndDefaultsToOne(t *testing.T) {
	// Byte-identity at the default: an unset ceiling behaves exactly as the
	// unconditional clamp did.
	for _, c := range []struct{ free, hits, cap, want int }{
		{2, 1, 0, 3}, {2, 3, 0, 3}, {2, 1, 2, 2}, {0, 0, 0, 0}, {5, 2, 0, 6},
	} {
		if got := analysis.MoveLimit(c.free, c.hits, c.cap, 0); got != c.want {
			t.Errorf("MoveLimit(%d,%d,%d,0) = %d, want %d — an unset ceiling must "+
				"reproduce the old unconditional clamp",
				c.free, c.hits, c.cap, got, c.want)
		}
	}
	// And it is now reachable, which it was not.
	if got := analysis.MoveLimit(1, 2, 0, 2); got != 3 {
		t.Errorf("MoveLimit at a ceiling of 2 is %d, want 3 — the routine two-hit "+
			"week must be expressible", got)
	}
	// The ceiling wins over MaxHits, which is what a ceiling is.
	if got := analysis.MoveLimit(1, 5, 0, 2); got != 3 {
		t.Errorf("MoveLimit(free=1, hits=5, ceiling=2) = %d, want 3", got)
	}
	// SimConfig's resolver and MoveLimit must agree about an unset field, or the
	// funded-pair branch and the limit would disagree about the same week.
	var z SimConfig
	if z.hitCeiling() != analysis.DefaultHitCeiling {
		t.Errorf("a zero SimConfig resolves the ceiling to %d, want %d",
			z.hitCeiling(), analysis.DefaultHitCeiling)
	}
	if config.Default().Review.HitCeiling != analysis.DefaultHitCeiling {
		t.Errorf("the shipped config ceiling is %d, want %d",
			config.Default().Review.HitCeiling, analysis.DefaultHitCeiling)
	}
}

// TestTheDoseWindowsAreTheOnesTheyClaim pins the two dose windows against the
// arithmetic their column comments state, because either one is easy to write as
// the other and the difference is a systematic bias rather than noise.
//
// Read off a synthetic calendar rather than the archive, so it exercises the
// windowing and not the fixture list — a test that needs the network to check an
// off-by-one is a test that skips when the answer matters.
func TestTheDoseWindowsAreTheOnesTheyClaim(t *testing.T) {
	// Club-gameweeks doubling, by gameweek. GW3 is the entry week, so its dose
	// belongs to neither window: the opening fifteen is chosen at that deadline
	// and no transfer can be banked into it.
	doubling := map[int]int{3: 5, 4: 2, 6: 4, 12: 6}
	blanking := map[int]int{3: 1, 5: 3, 20: 8}
	got := doseOver(doubling, blanking, 3, 5)
	if got.ActDoubles != 2+4+6 {
		t.Errorf("actionable doubles are %d, want %d — the window is [start+1, 38] "+
			"and the entry week's own double is football the cell scores rather "+
			"than dose it can act on", got.ActDoubles, 2+4+6)
	}
	if got.ActBlanks != 3+8 {
		t.Errorf("actionable blanks are %d, want %d", got.ActBlanks, 3+8)
	}
	// The opening squad is built on a 5-gameweek horizon from GW3, so GW4 to GW8
	// were already visible to it. What is left for the transfer policy is GW9
	// onward.
	if got.LateDoubles != 6 {
		t.Errorf("late doubles are %d, want 6 — the window is [start+H+1, 38] and "+
			"GW4 and GW6 were priced by the opening squad's own horizon",
			got.LateDoubles)
	}
	if got.LateBlanks != 8 {
		t.Errorf("late blanks are %d, want 8", got.LateBlanks)
	}
	// Late is always a subset of actionable, which is the invariant a reader
	// checks a banked row against.
	if got.LateDoubles > got.ActDoubles || got.LateBlanks > got.ActBlanks {
		t.Errorf("the late dose exceeds the actionable one: %+v", got)
	}
}

// TestTheCellWriterEmitsEveryNewColumn is invariant 3 applied to the schema half:
// a column in the header with nothing writing it produces a ragged file, and the
// reverse produces a silently misaligned one.
//
// It writes one row through the real sink and reads it back by NAME, which is the
// only check that catches a value landing under a neighbour's header.
func TestTheCellWriterEmitsEveryNewColumn(t *testing.T) {
	row := cellRow{
		Sweep: "OPT", RunID: "r", Variant: "v", Season: "2025-26",
		StartGW: 1, Weeks: 38,
		HasBanking:      true,
		BankingMediator: BankingMediator{DecisionWeeks: 37, ConsultedWeeks: 0},
		TransferHold: TransferHoldMediator{
			ConsultedWeeks: 37, PricedWeeks: 37, GateCalls: 51, Flips: 6,
			ChargeSum: 37, LoadSum: 37},
		WildcardTrig: ChipTriggerMediator{ConsultedWeeks: 9, WeighedWeeks: 9,
			FiredGW: 12, FiredValue: 16, FiredBar: 8},
		BenchBoostTrig: ChipTriggerMediator{ConsultedWeeks: 20, WeighedWeeks: 18},
		FreeHitTrig:    ChipTriggerMediator{ConsultedWeeks: 20, WeighedWeeks: 20},
		ChipPrep: ChipPrepMediator{ConsultedWeeks: 37, CreditWeeks: 4,
			BenchSum: 0.8, CaptainSum: 0.4},
		HasDose:    true,
		ActDoubles: 31, ActBlanks: 22, LateDoubles: 29, LateBlanks: 20,
	}
	rec := readOneCell(t, row)
	for _, c := range []struct {
		col  string
		want string
	}{
		{"ftv_weeks", "37"}, {"ftv_priced_weeks", "37"},
		{"ftv_gate_calls", "51"}, {"ftv_flips", "6"},
		{"ftv_mean_charge", "1"}, {"ftv_mean_load", "1"},
		{"wc_trig_weeks", "9"}, {"wc_trig_weighed", "9"}, {"wc_trig_gw", "12"},
		{"wc_trig_value", "16"}, {"wc_trig_bar", "8"},
		{"bb_trig_weeks", "20"}, {"bb_trig_weighed", "18"}, {"bb_trig_gw", ""},
		{"fh_trig_weeks", "20"}, {"fh_trig_weighed", "20"}, {"fh_trig_gw", ""},
		{"prep_weeks", "37"}, {"prep_credit_weeks", "4"},
		{"prep_bench_sum", "0.8"}, {"prep_captain_sum", "0.4"},
		{"dose_act_doubles", "31"}, {"dose_act_blanks", "22"},
		{"dose_late_doubles", "29"}, {"dose_late_blanks", "20"},
	} {
		if got := rec[c.col]; got != c.want {
			t.Errorf("%s is %q, want %q", c.col, got, c.want)
		}
	}
	// And the counts really are counts: nothing in the block may be divided by
	// `weeks`, whose denominator is decision_weeks and not the season length.
	if rec["ftv_weeks"] == perGW(37, 38) {
		t.Errorf("ftv_weeks was normalised by weeks; its denominator is " +
			"decision_weeks and it is a count either way")
	}
}

// readOneCell writes one row through the real sink and reads it back keyed by
// column NAME.
//
// By name rather than by index, because `cellHeader` and `cellSink.cell` are
// positional mirrors of each other and a value landing under a neighbour's header
// is silent on every index-based check. Same argument as `sampleRow`'s, applied to
// a row a test constructs itself.
func readOneCell(t *testing.T, row cellRow) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	row.RunID = sink.run()
	sink.cell(row)
	sink.close()
	_, rows := readCells(t, path)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	return rows[0]
}
