package backtest

import (
	"fmt"

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
	if ChipSetsFor(season) < 2 {
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
		// appended rather than assumed. `Set` returns an error only for a name that
		// does not resolve or a week outside 1..38, and both are impossible on the
		// literals above given the `n.week <= 0` guard, so a failure here would be a
		// change to ChipSchedule rather than bad input from a caller.
		if err := out.Set(fmt.Sprintf("%s%d", n.k, set), n.week); err != nil {
			panic("SplitChipSets: " + err.Error())
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
	sets := ChipSetsFor(season)
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
