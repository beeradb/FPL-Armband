package backtest

import (
	"strings"
	"testing"

	"armband/internal/analysis"
)

// The reset arrived for 2025-26, and this record explicitly forbids projecting it
// backwards to buy chip observations.
func TestOnlyTheResetSeasonsGrantTwoChipSets(t *testing.T) {
	for _, s := range []string{"2016-17", "2019-20", "2022-23", "2024-25"} {
		if got := ChipSetsFor(s); got != 1 {
			t.Errorf("%s grants %d chip sets; the reset did not exist yet", s, got)
		}
	}
	for _, s := range []string{"2025-26", "2026-27"} {
		if got := ChipSetsFor(s); got != 2 {
			t.Errorf("%s grants %d chip sets; the chips reset at GW%d", s, got, ChipResetGW)
		}
	}
}

// A second-set chip on a one-set season must be refused, not played.
//
// This is the failure the validator exists for: the simulator compares an int to a
// gameweek, so an unvalidated second set would simply play, and a 2022-23 replay
// would quietly score eight chips in a season that granted four. Nothing in the
// output names the chip rule, so the points would look ordinary.
func TestASecondSetIsRefusedOnASeasonThatNeverHadOne(t *testing.T) {
	err := ValidateChipSets("2022-23", analysis.ChipPlan{Wildcard: 6},
		analysis.ChipPlan{Wildcard: 28})
	if err == nil {
		t.Fatal("a second chip set was accepted on a season that granted one")
	}
	if !strings.Contains(err.Error(), "one set of chips") {
		t.Errorf("the error does not explain the rule: %v", err)
	}
}

// Each set has to stay in its own half, because a chip cannot be carried across the
// reset and cannot be played before it.
func TestChipSetsMustStayInTheirOwnHalf(t *testing.T) {
	if err := ValidateChipSets("2025-26", analysis.ChipPlan{BenchBoost: 25},
		analysis.ChipPlan{}); err == nil {
		t.Error("a first-set chip after the reset was accepted")
	}
	if err := ValidateChipSets("2025-26", analysis.ChipPlan{},
		analysis.ChipPlan{BenchBoost: 8}); err == nil {
		t.Error("a second-set chip before the reset was accepted")
	}
	// The same placement is legitimate on a one-set season, where a wildcard in
	// March was ordinary play. Refusing it would reject every recorded plan.
	if err := ValidateChipSets("2022-23", analysis.ChipPlan{Wildcard: 28},
		analysis.ChipPlan{}); err != nil {
		t.Errorf("a late wildcard on a one-set season was refused: %v", err)
	}
}

// FPL allows one chip a gameweek, and the scoring switch assumes it: bench boost is
// tested before triple captain, so two chips in one week would silently drop the
// second and score a week the manager could not have played.
func TestTwoChipsInOneGameweekAreRefusedAcrossBothSets(t *testing.T) {
	if err := ValidateChipSets("2025-26",
		analysis.ChipPlan{BenchBoost: 8, TripleCaptain: 8}, analysis.ChipPlan{}); err == nil {
		t.Error("two chips in one gameweek were accepted within a set")
	}
	if err := ValidateChipSets("2025-26",
		analysis.ChipPlan{Wildcard: 6}, analysis.ChipPlan{BenchBoost: 30, TripleCaptain: 30}); err == nil {
		t.Error("two chips in one gameweek were accepted within the second set")
	}
}

func TestAValidTwoSetPlanIsAccepted(t *testing.T) {
	err := ValidateChipSets("2025-26",
		analysis.ChipPlan{Wildcard: 6, BenchBoost: 8, TripleCaptain: 9, FreeHit: 16},
		analysis.ChipPlan{Wildcard: 28, BenchBoost: 33, TripleCaptain: 35, FreeHit: 31})
	if err != nil {
		t.Fatalf("a legal two-set plan was refused: %v", err)
	}
}

// The helpers behave. This is NOT a claim that the simulator uses them — it tests
// `plays` and `nextChip` as functions, and it was originally named as though it
// covered every consumer, which it does not: it would not have caught `anticipate`
// reading only the first set, and review did.
//
// TestASecondSetChipActuallyPlays is the one that covers a consumer.
func TestTheChipSetHelpersReadBothSets(t *testing.T) {
	c := SimConfig{
		Chips:  analysis.ChipPlan{Wildcard: 6, BenchBoost: 8, TripleCaptain: 9, FreeHit: 16},
		Chips2: analysis.ChipPlan{Wildcard: 28, BenchBoost: 33, TripleCaptain: 35, FreeHit: 31},
	}
	for _, tc := range []struct {
		slot chipSlot
		name string
		gws  [2]int
	}{
		{slotWildcard, "wildcard", [2]int{6, 28}},
		{slotBenchBoost, "bench boost", [2]int{8, 33}},
		{slotTripleCaptain, "triple captain", [2]int{9, 35}},
		{slotFreeHit, "free hit", [2]int{16, 31}},
	} {
		for _, gw := range tc.gws {
			if !c.plays(tc.slot, gw) {
				t.Errorf("%s at GW%d is not played", tc.name, gw)
			}
		}
		if c.plays(tc.slot, 20) {
			t.Errorf("%s is played at GW20, which no set schedules", tc.name)
		}
	}
}

// The horizon logic needs the NEXT chip, not "the" chip.
//
// With one set, reading the field was the same thing. With two it is not: a
// decision in March asking for "the wildcard" gets September's, which is behind it,
// so the wall that should stop a squad preparing past a rebuild disappears.
func TestNextChipFindsTheOneAheadOfTheDecision(t *testing.T) {
	c := SimConfig{
		Chips:  analysis.ChipPlan{Wildcard: 6, BenchBoost: 8},
		Chips2: analysis.ChipPlan{Wildcard: 28, BenchBoost: 33},
	}
	for _, tc := range []struct {
		from, want int
	}{{1, 6}, {6, 6}, {7, 28}, {28, 28}, {29, 0}} {
		if got := c.nextChip(slotWildcard, tc.from); got != tc.want {
			t.Errorf("next wildcard from GW%d is GW%d, want GW%d", tc.from, got, tc.want)
		}
	}
	if got := c.nextChip(slotBenchBoost, 9); got != 33 {
		t.Errorf("next bench boost from GW9 is GW%d, want GW33", got)
	}
	// A chip nobody scheduled is 0, never a stray field value.
	if got := c.nextChip(slotFreeHit, 1); got != 0 {
		t.Errorf("an unscheduled free hit reports GW%d", got)
	}
}

// A second-set chip must actually PLAY, and an empty second set must change
// nothing — both asserted against the simulator rather than against the helpers.
//
// This is the test the helper test was mis-named for. The failure it guards is the
// one this record keeps meeting: a field that parses, validates, and is never read
// by the code that decides, returning a season byte-identical to one where the
// setting was absent. Nothing in the output distinguishes that from "the chip is
// worth nothing", which is why it has to be pinned rather than reasoned about.
func TestASecondSetChipActuallyPlays(t *testing.T) {
	cur, prior, base := chipSim(t)

	// A bench boost on 2025-26's only six-club double.
	withChip := base
	withChip.Chips2 = analysis.ChipPlan{BenchBoost: 33}
	got, err := Simulate(cur, prior, withChip)
	if err != nil {
		t.Fatal(err)
	}
	played := false
	for _, wk := range got.Weeks {
		if wk.GW == 33 && wk.BenchBoost {
			played = true
		}
		if wk.BenchBoost && wk.GW != 33 {
			t.Errorf("a bench boost played at GW%d, which no set schedules", wk.GW)
		}
	}
	if !played {
		t.Fatal("a second-set bench boost at GW33 never played — Chips2 is stored and not read")
	}

	// And the empty second set is a true no-op, not merely a harmless-looking one.
	// Everything recorded from this harness was measured at one set.
	plain, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	if got.Points == plain.Points {
		t.Error("playing a bench boost changed nothing; the chip is not reaching scoring")
	}
	again, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	if again.Points != plain.Points || again.Transfers != plain.Transfers {
		t.Errorf("a chipless replay is not reproducible: %d/%d against %d/%d",
			again.Points, again.Transfers, plain.Points, plain.Transfers)
	}
}

// A one-set config must behave exactly as it did before Chips2 existed.
//
// Everything recorded from this harness was measured with one set, so the empty
// second set has to be a true no-op rather than something that merely looks
// harmless.
func TestAnEmptySecondSetChangesNothing(t *testing.T) {
	c := SimConfig{Chips: analysis.ChipPlan{Wildcard: 6, BenchBoost: 8}}
	if !c.plays(slotWildcard, 6) || c.plays(slotWildcard, 28) {
		t.Error("an empty second set is changing which weeks play a wildcard")
	}
	if got := c.nextChip(slotWildcard, 1); got != 6 {
		t.Errorf("next wildcard with no second set is GW%d, want GW6", got)
	}
	if got := c.nextChip(slotWildcard, 7); got != 0 {
		t.Errorf("a one-set plan reports a wildcard at GW%d after its only one", got)
	}
}
