package backtest

import (
	"fmt"
	"strings"

	"armband/internal/analysis"
)

// ChipResetGW is the first gameweek of the second chip set.
//
// FPL's rule is that the first set expires after GW19 and a fresh set becomes
// available from GW20. A chip unplayed by the deadline is simply lost.
const ChipResetGW = 20

// ChipSetsFor is how many sets of chips a season granted.
//
// The rule arrived for 2025-26 and this record carries an explicit warning against
// projecting it backwards. Across all six archived first halves there are 15
// doubling club-gameweeks out of 189, and 11 of the 15 are one COVID-rescheduled
// 2020-21 round — so a first-half arm is collinear with "a chip on a plain week" in
// five seasons of six, not in all of them. Seasons are gated rather than assumed for
// the same reason BankLimitFor exists: replaying a season under a rule nobody was
// playing under produces a number that describes no game.
func ChipSetsFor(season string) int {
	if season < "2025-26" {
		return 1
	}
	return 2
}

// ChipSetsForced is a deliberate counterfactual: replay an older season under
// TODAY'S chip rules rather than the ones it was played under.
//
// ⚠️ **This is exactly what the warning above tells you not to do, and it is
// enabled on purpose.** The user's ruling, 2026-08-25: *"We can retroactively
// project the current chip rules backwards. The data is still valid... We are
// just applying current chip rules to old data, but the underlying points are
// the same."*
//
// **That is right, and the warning above conflates two things.** Player scores,
// minutes and fixtures are untouched; only the chip ALLOWANCE is counterfactual.
// For a product that has to advise under the current rules, "what is today's chip
// strategy worth on real football" is the question, and history is a supply of
// fixtures rather than a claim about what anyone did. It scores a game nobody
// played, and that is the point of it.
//
// ⚠️ **What survives the override is the POWER warning, and it is not small.**
// Across all six archived first halves there are 15 doubling club-gameweeks out
// of 189, and 11 of those 15 are one COVID-rescheduled 2020-21 round. So a
// first-half chip arm is collinear with "a chip on a plain week" in five seasons
// of six: granting a second set backwards buys chip observations whose first half
// can distinguish almost nothing. **Expect the extra set to add variance and
// little signal**, and never read a null in a projected first half as a fact
// about chips.
//
// ⚠️ **A figure measured under this is not comparable with one measured without
// it.** It is a different game. It is not fingerprinted — it is a SimConfig
// field, so it lands in the cell's own arm label rather than in the sidecar's
// environment block; label the arm.
const ChipSetsForced = 2

// chipSlot names one of the four chips. These are `analysis.ChipSchedule`'s own
// slot names with the set suffix left off, which that package reads as the first
// set — the helpers below always ask across both sets, so the suffix would be
// noise here.
//
// The lookups this file used to carry — a chipWeek switch, plays and nextChip —
// were a second implementation of what ChipSchedule now provides, and the
// duplication was the live risk rather than the line count: the live agent had no
// two-set support at all while this package did, so the two layers disagreed
// about whether a March decision could see a second-set wildcard. They delegate.
type chipSlot = string

// Aliased to the analysis constants rather than re-spelled. Re-spelling them
// here would be a fourth copy of the eight names, and the readers they are
// passed to (`Plays`, `Next`) cannot report a name that does not resolve — they
// return the nothing-planned answer, so a drifted literal is silent.
const (
	slotWildcard      chipSlot = analysis.SlotWildcard
	slotFreeHit       chipSlot = analysis.SlotFreeHit
	slotBenchBoost    chipSlot = analysis.SlotBenchBoost
	slotTripleCaptain chipSlot = analysis.SlotTripleCaptain
)

// schedule is both sets as the one type that can hold them.
func (c SimConfig) schedule() analysis.ChipSchedule {
	return analysis.ChipSchedule{First: c.Chips, Second: c.Chips2}
}

func chipWeek(p analysis.ChipPlan, k chipSlot) int {
	gw, _ := (analysis.ChipSchedule{First: p}).Get(k)
	return gw
}

// plays is whether either set plays chip k in this gameweek.
func (c SimConfig) plays(k chipSlot, gw int) bool { return c.schedule().Plays(k, gw) }

// nextChip is the earliest week at or after `from` that plays chip k, or 0.
func (c SimConfig) nextChip(k chipSlot, from int) int { return c.schedule().Next(k, from) }

// anyChips is whether a plan plays anything at all.
func anyChips(p analysis.ChipPlan) bool { return p != analysis.ChipPlan{} }

// SplitChipSets routes a single-set plan's chips into the sets the season
// actually granted them from.
//
// # The defect it repairs, and why it is here rather than in the planner
//
// A chip PLANNER answers "which gameweek is this chip worth playing in". Which of
// FPL's two sets that week draws from is bookkeeping about the calendar, not about
// football, and every planner that computed a week had to know the rule
// separately — which is how none of them did.
//
// The cost was silent and total. `anchoredPlan` puts 2025-26's free hit at GW34
// and its bench boost at GW33; `ChipSetsFor("2025-26")` is 2, so
// `ValidateChipSets` refuses a first-set chip at or after `ChipResetGW`;
// `runPolicySweep` records the refusal as an INFEASIBLE cell rather than
// fatalling. So an anchored-chips arm quietly lost **all six 2025-26 cells** while
// every printed number stayed plausible — a comparison that never ran, wearing the
// clothes of a season that did. Found by the blanks-and-doubles census, which
// counts the refusals rather than assuming there are none.
//
// # It is the identity in a one-set season
//
// `ChipSetsFor` gates it, so every season before 2025-26 gets its plan back
// unchanged in `First` and an empty `Second` — which is what makes this safe to
// apply to every planner rather than to the ones somebody remembered. A plan that
// was already legal stays byte-identical.
//
// ⚠️ **It cannot repair a plan that is illegal for a second reason.** Two chips in
// one gameweek is still two chips in one gameweek, and `ValidateChipSets` still
// refuses it — the split moves a chip between sets and never moves its week. That
// is deliberate: a planner that collides with itself has a bug this must not hide.
func SplitChipSets(season string, p analysis.ChipPlan) analysis.ChipSchedule {
	return splitChipSets(ChipSetsFor(season), p)
}

// SplitChipSetsWith is SplitChipSets under an explicit set count, for an arm
// deliberately replaying an older season under today's rules. `sets` of 0 means
// "ask the season", which is every ordinary caller. See ChipSetsForced.
func SplitChipSetsWith(season string, sets int, p analysis.ChipPlan) analysis.ChipSchedule {
	if sets <= 0 {
		sets = ChipSetsFor(season)
	}
	return splitChipSets(sets, p)
}

func splitChipSets(sets int, p analysis.ChipPlan) analysis.ChipSchedule {
	if sets < 2 {
		return analysis.ChipSchedule{First: p}
	}
	var out analysis.ChipSchedule
	for _, n := range []struct {
		k    chipSlot
		week int
	}{
		{slotWildcard, p.Wildcard},
		{slotFreeHit, p.FreeHit},
		{slotBenchBoost, p.BenchBoost},
		{slotTripleCaptain, p.TripleCaptain},
	} {
		if n.week <= 0 {
			continue
		}
		set := 1
		if n.week >= ChipResetGW {
			set = 2
		}
		// The slot names carry no suffix here — see chipSlot — so the set is
		// appended rather than assumed.
		//
		// ⚠️ **An out-of-range week is DROPPED, not panicked on.** `Set` rejects
		// anything outside 0..38 and the guard above covers only the lower end, so
		// a planner returning 39 crashed here — in a two-set season only — under a
		// comment arguing it was impossible while naming just the lower bound. No
		// shipped planner does that (`sightedWeeks` and `controlWeeks` both bound
		// at 38), so the defect was the argument rather than the behaviour, and the
		// fix is to make the behaviour match what this function is: a ROUTER.
		// Judging a plan is `ValidateChipSets`'s job, and a planner emitting a week
		// nobody can play has a bug of its own that a panic here would mislabel.
		if n.week > 38 {
			continue
		}
		if err := out.Set(fmt.Sprintf("%s%d", n.k, set), n.week); err != nil {
			// Unreachable given both bounds, and a dropped chip rather than a
			// panic for the same reason: this routes, it does not validate.
			continue
		}
	}
	return out
}

// ValidateChipSets checks a two-set plan against the rules FPL actually enforces,
// and against what the season being replayed granted.
//
// This is a hard error rather than a warning. A chip placed in a week it could not
// be played in is not a slightly-wrong setting: the simulator compares an int to a
// gameweek, so a second-set bench boost at GW8 would simply *play* at GW8, and the
// result is a season with two bench boosts in the first half. Nothing downstream
// would report that, and the points would look ordinary.
func ValidateChipSets(season string, first, second analysis.ChipPlan) error {
	return validateChipSets(season, ChipSetsFor(season), first, second)
}

// ValidateChipSetsWith validates under an explicit set count, so an arm replaying
// an older season under today's rules is checked against the rules it declares
// rather than refused by the ones the season had. `sets` of 0 asks the season.
func ValidateChipSetsWith(season string, sets int, first, second analysis.ChipPlan) error {
	if sets <= 0 {
		sets = ChipSetsFor(season)
	}
	return validateChipSets(season, sets, first, second)
}

// ChipSetLapse declares that an arm's plan deliberately does not spend one of
// the season's granted chip sets — a bitmask because a two-set season can
// waste either half independently.
//
// It exists so `ValidateChipSpend` can tell "this arm has a bug" from "this
// arm is measuring what happens when a set goes unspent", which several
// diagnostics in this package already do on purpose — the two-regime sweep's
// own arm labels say "FIRST SET WASTED" in so many words. Without a way to
// declare that, the guard below would either miss the real bug class or
// refuse every deliberate one alongside it.
type ChipSetLapse uint8

const (
	LapseFirstSet ChipSetLapse = 1 << iota
	LapseSecondSet
)

// chipSetWindow is the reachable window for the numbered set (1 or 2) of a
// season granting `sets` sets, entered at `start` — the same construction
// `FullAnchoredPlan` uses to decide where each chip may land, factored out
// here so the planner and this guard cannot drift into two implementations
// of one boundary.
//
// ok is false when the season does not grant that set at all (`set > sets`)
// or the entry point leaves no week in it (a late entrant reaching a set
// after it has already expired) — either way nothing is owed for that set,
// and the caller must skip it rather than read lo/hi.
func chipSetWindow(sets, set, start int) (lo, hi int, ok bool) {
	if set < 1 || set > sets {
		return 0, 0, false
	}
	lo, hi = start+1, 38
	if sets == 2 && set == 1 {
		hi = ChipResetGW - 1
	} else if sets == 2 {
		lo = ChipResetGW
		if start+1 > lo {
			lo = start + 1
		}
	}
	if lo < 1 {
		lo = 1
	}
	if lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

// ValidateChipSpend checks that every chip set a season actually granted, and
// that the entry point can still reach, was fully spent — all four chips
// placed somewhere in that set's window — unless the caller has declared the
// set a deliberate lapse.
//
// # Why this exists beside ValidateChipSets
//
// `ValidateChipSets` refuses a plan that breaks FPL's rules — a chip in a
// week its set cannot reach, two chips in one gameweek. It says nothing about
// a plan that is perfectly LEGAL and simply never touches half the calendar:
// every chip lands inside its own set's window, so nothing is illegal, and
// the second set's four chips are quietly worth zero. That is not a rule
// violation, it is a manager who forgot he had a wildcard left — and it has
// happened to this codebase once already, silently, in a diagnostic that
// concentrated its whole plan in one half of the season and cost the
// measurement roughly a season's worth of chips before anyone noticed.
//
// sets<=0 asks the season via ChipSetsFor — the same resolution
// ValidateChipSets itself uses, never a sweep's own declared constant, so
// this cannot be fooled by an arm that only THINKS it is replaying under
// today's rules.
func ValidateChipSpend(season string, start int, first, second analysis.ChipPlan) error {
	return validateChipSpend(season, ChipSetsFor(season), start, 0, first, second)
}

// ValidateChipSpendWith is ValidateChipSpend under an explicit set count and a
// declared lapse. `sets` of 0 asks the season, exactly as ValidateChipSetsWith
// does.
func ValidateChipSpendWith(season string, sets, start int, lapse ChipSetLapse, first, second analysis.ChipPlan) error {
	if sets <= 0 {
		sets = ChipSetsFor(season)
	}
	return validateChipSpend(season, sets, start, lapse, first, second)
}

// chipSlotNames pairs PlacedChips' four fields with the names validateChipSets
// already prints, so a "which slot is missing" error says a chip's name
// rather than making a reader cross-reference a struct field.
var chipSlotNames = []struct {
	name string
	gw   func(analysis.ChipPlan) int
}{
	{"wildcard", func(p analysis.ChipPlan) int { return p.Wildcard }},
	{"bench boost", func(p analysis.ChipPlan) int { return p.BenchBoost }},
	{"free hit", func(p analysis.ChipPlan) int { return p.FreeHit }},
	{"triple captain", func(p analysis.ChipPlan) int { return p.TripleCaptain }},
}

// missingChipNames lists which of a plan's four chips have no week, for an
// error a reader can act on without cross-referencing ChipPlan's fields.
func missingChipNames(p analysis.ChipPlan) []string {
	var out []string
	for _, n := range chipSlotNames {
		if n.gw(p) <= 0 {
			out = append(out, n.name)
		}
	}
	return out
}

func validateChipSpend(season string, sets, start int, lapse ChipSetLapse, first, second analysis.ChipPlan) error {
	// The blanket exemption: an arm running no chip plan at all is the
	// no-chip control every banked sweep cell historically used, not a
	// manager who forgot his chips. Only the total absence of a plan is
	// exempt — a partial one still owes an explanation, via the lapse bit.
	if !anyChips(first) && !anyChips(second) {
		return nil
	}
	plans := []struct {
		set   int
		bit   ChipSetLapse
		plan  analysis.ChipPlan
		label string
	}{
		{1, LapseFirstSet, first, "first"},
		{2, LapseSecondSet, second, "second"},
	}
	for _, p := range plans {
		lo, hi, ok := chipSetWindow(sets, p.set, start)
		if !ok {
			// Not granted, or granted but already out of reach from this
			// entry point — either way nothing is owed and there is
			// nothing to declare a lapse against.
			continue
		}
		placed := PlacedChips(p.plan)
		declared := lapse&p.bit != 0
		switch {
		case declared && placed == 4:
			return fmt.Errorf("%s: the %s set (GW%d-%d) is declared a deliberate "+
				"lapse but is fully spent (4/4) — a stale declaration, or the plan "+
				"changed underneath it", season, p.label, lo, hi)
		case !declared && placed != 4:
			return fmt.Errorf("%s: the %s set (GW%d-%d, entered GW%d) is only "+
				"%d/4 chips spent and not declared a lapse — missing %s",
				season, p.label, lo, hi, start, placed, strings.Join(missingChipNames(p.plan), ", "))
		}
	}
	return nil
}

func validateChipSets(season string, sets int, first, second analysis.ChipPlan) error {
	if anyChips(second) && sets < 2 {
		return fmt.Errorf("%s granted one set of chips, not two: "+
			"the reset arrived for 2025-26, and replaying an older season under it "+
			"scores a game nobody played", season)
	}
	named := []struct {
		name string
		k    chipSlot
	}{
		{"wildcard", slotWildcard}, {"free hit", slotFreeHit},
		{"bench boost", slotBenchBoost}, {"triple captain", slotTripleCaptain},
	}
	for _, n := range named {
		// The first set expires at the GW19 deadline. Only checked when the
		// season HAS a second set: before the rule existed a wildcard in March
		// was ordinary play, and rejecting it would refuse every recorded plan.
		if w := chipWeek(first, n.k); sets >= 2 && w >= ChipResetGW {
			return fmt.Errorf("first-set %s at GW%d: the first set expires after GW%d",
				n.name, w, ChipResetGW-1)
		}
		if w := chipWeek(second, n.k); w > 0 && w < ChipResetGW {
			return fmt.Errorf("second-set %s at GW%d: the second set opens at GW%d",
				n.name, w, ChipResetGW)
		}
	}
	// One chip per gameweek, across both sets — FPL's rule, and the simulator
	// assumes it: the scoring switch picks bench boost before triple captain, so
	// two chips in one week would silently drop the second.
	seen := map[int]string{}
	for _, p := range []analysis.ChipPlan{first, second} {
		for _, n := range named {
			w := chipWeek(p, n.k)
			if w == 0 {
				continue
			}
			if prev, ok := seen[w]; ok {
				return fmt.Errorf("GW%d plays both %s and %s: FPL allows one chip a week",
					w, prev, n.name)
			}
			seen[w] = n.name
		}
	}
	return nil
}
