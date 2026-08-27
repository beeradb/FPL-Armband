package analysis

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"armband/internal/fpl"
)

// ⚠️ These build their own bootstrap instead of calling `testEngine`, and that is
// the point rather than a shortcut. The first version of this file used the live
// API and every case SKIPPED on a 429, which would have shipped an unrun guard
// reading as green — the exact failure this repository has a standing rule
// about. `chipWindowsByKind` reads nothing but `boot.Chips`, so a season's set
// count is cheap to state exactly, and stating it exactly is also the only way to
// test the ONE-set branch, which the live feed cannot produce this year.
//
// ⚠️ A ONE-SET season's first window runs to GW38, not GW19. `ChipSchedule.First`
// says so directly: the first set "expires at the GW19 deadline in a two-set
// season, and runs the whole season in a one-set one". An earlier version of this
// fixture used 19 for both, which made the one-set test unable to see that the
// code was quoting a two-set deadline at a one-set season. A synthetic bootstrap
// is only worth having while it stays faithful to the shape it stands in for.
func chipBoot(sets int) *fpl.Bootstrap {
	b := &fpl.Bootstrap{}
	firstStop := 38
	if sets >= 2 {
		firstStop = 19
	}
	for _, name := range []string{"wildcard", "freehit", "bboost", "3xc"} {
		b.Chips = append(b.Chips, fpl.Chip{Name: name, Number: 1, StartEvent: 1, StopEvent: firstStop})
		if sets >= 2 {
			b.Chips = append(b.Chips, fpl.Chip{Name: name, Number: 2, StartEvent: 20, StopEvent: 38})
		}
	}
	return b
}

// The defect review found: the deadline was a hardcoded GW19, so a one-set
// season — where the single set runs to GW38 — was told its chips expire
// nineteen gameweeks early.
func TestUnplannedChipsQuotesTheSeasonsOwnDeadline(t *testing.T) {
	one := strings.Join((&Engine{Boot: chipBoot(1)}).UnplannedChips(ChipSchedule{}), " | ")
	if strings.Contains(one, "GW19") {
		t.Errorf("a one-set season's chips run to GW38; quoting GW19 forfeits them "+
			"nineteen weeks early. got %q", one)
	}
	if !strings.Contains(one, "GW38") {
		t.Errorf("the one granted set runs to GW38 and the message should say so; got %q", one)
	}

	two := strings.Join((&Engine{Boot: chipBoot(2)}).UnplannedChips(ChipSchedule{}), " | ")
	if !strings.Contains(two, "GW19") || !strings.Contains(two, "GW38") {
		t.Errorf("a two-set season loses the first set after GW19 and the second after "+
			"GW38, and each message names its OWN set's date; got %q", two)
	}
}

// The deadline must belong to the set being reported. The trailing form named
// the first set's date even when only the second was flagged, which handed a
// reader a date for chips they had already planned.
func TestUnplannedChipsNamesTheDeadlineOfTheSetItFlagged(t *testing.T) {
	e := &Engine{Boot: chipBoot(2)}
	// First set fully planned; only the second is unspent.
	s := ChipSchedule{First: ChipPlan{Wildcard: 6, BenchBoost: 8, TripleCaptain: 9, FreeHit: 16}}
	got := strings.Join(e.UnplannedChips(s), " | ")
	if strings.Contains(got, "GW19") {
		t.Errorf("only the second set is unplanned, so the first set's GW19 deadline is "+
			"not this reader's problem; got %q", got)
	}
	if !strings.Contains(got, "GW38") {
		t.Errorf("the unplanned second set is lost after GW38; got %q", got)
	}
}

// The defect this function exists for: a season grants two sets of chips, the
// plan fills one, and every existing surface calls the plan fine.
//
// `ValidateChipPlan` iterates the chips a plan NAMES, so an unplanned set
// produces zero iterations and zero messages. That is correct for what it
// promises — "an empty result means it is legal", and an unspent chip is legal —
// and it is why completeness needed a function of its own.
func TestUnplannedChipsSeesASetNobodyPlanned(t *testing.T) {
	e := &Engine{Boot: chipBoot(2)}

	// The shipped shape as of 2026-08-27: first set filled, second untouched.
	s := ChipSchedule{First: ChipPlan{Wildcard: 6, BenchBoost: 8, TripleCaptain: 9, FreeHit: 16}}

	if problems := e.ValidateChipPlan(s); len(problems) > 0 {
		t.Fatalf("precondition: this plan is meant to be LEGAL, so the defect is that nothing "+
			"notices the gap — but ValidateChipPlan reported %v", problems)
	}

	got := e.UnplannedChips(s)
	if len(got) == 0 {
		t.Fatal("a whole granted set is unplanned and UnplannedChips said nothing — " +
			"which is the exact blindness it was written to remove")
	}
	joined := strings.Join(got, " | ")
	if !strings.Contains(joined, "second set") {
		t.Errorf("the unplanned set is the second one; got %q", joined)
	}
	if !strings.Contains(joined, "NONE") {
		t.Errorf("no chip at all is planned for that set, which is worth saying more "+
			"loudly than a partial gap; got %q", joined)
	}
	if !strings.Contains(joined, "GW38") {
		t.Errorf("the message must name the unplanned set's own expiry, because "+
			"unplanned and forfeited converge as the season runs on; got %q", joined)
	}
}

// A plan that spends everything the season granted must stay silent, or the
// advisory becomes noise a reader learns to skip.
func TestUnplannedChipsIsSilentOnAFullPlan(t *testing.T) {
	e := &Engine{Boot: chipBoot(2)}
	full := ChipSchedule{
		First:  ChipPlan{Wildcard: 6, BenchBoost: 8, TripleCaptain: 9, FreeHit: 16},
		Second: ChipPlan{Wildcard: 22, BenchBoost: 26, TripleCaptain: 30, FreeHit: 34},
	}
	if got := e.UnplannedChips(full); len(got) != 0 {
		t.Errorf("every granted chip is planned, so this must be silent; got %v", got)
	}
}

// ⚠️ A ONE-set season must never be told it has a second set unplanned — that
// would invent a chip the competition never granted. The count comes from the
// boot, not from an assumption, and this is the case the live feed cannot
// currently produce.
func TestUnplannedChipsNeverInventsASetTheSeasonDidNotGrant(t *testing.T) {
	e := &Engine{Boot: chipBoot(1)}
	got := strings.Join(e.UnplannedChips(ChipSchedule{}), " | ")
	if strings.Contains(got, "second set") {
		t.Errorf("this season grants one set, so nothing may claim a second; got %q", got)
	}
	if !strings.Contains(got, "first set") {
		t.Errorf("the one granted set is entirely unplanned and that is worth saying; got %q", got)
	}
}

// A partially filled set reads differently from an empty one, because the action
// is different: fill the gap, rather than plan the set.
func TestUnplannedChipsDistinguishesAPartialGapFromAnEmptySet(t *testing.T) {
	e := &Engine{Boot: chipBoot(2)}
	s := ChipSchedule{
		First:  ChipPlan{Wildcard: 6, BenchBoost: 8, TripleCaptain: 9, FreeHit: 16},
		Second: ChipPlan{Wildcard: 22},
	}
	joined := strings.Join(e.UnplannedChips(s), " | ")
	if strings.Contains(joined, "NONE") {
		t.Errorf("one chip IS planned in the second set, so this is a gap and not an "+
			"empty set; got %q", joined)
	}
	if !strings.Contains(joined, "second set") {
		t.Errorf("three of the second set's four are unplanned; got %q", joined)
	}
}

// The shipped config is the thing that was actually wrong, so it is the thing
// pinned. Reads config.json rather than restating its values, because a test
// carrying its own copy of the plan stops tracking the plan.
//
// ⚠️ This does NOT assert the shipped plan is wrong — leaving a second set open
// is the user's call and may be deliberate. It asserts that whatever the plan
// is, this function DESCRIBES it rather than staying silent, which is the
// behaviour that was missing.
func TestTheShippedChipPlanIsDescribedRatherThanPassedOver(t *testing.T) {
	b, err := os.ReadFile("../../config.json")
	if err != nil {
		t.Skipf("no shipped config to read: %v", err)
	}
	var top struct {
		Chips json.RawMessage `json:"chip_plan"`
	}
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	var s ChipSchedule
	if err := json.Unmarshal(top.Chips, &s); err != nil {
		t.Fatalf("the shipped chip_plan does not parse: %v", err)
	}

	e := &Engine{Boot: chipBoot(2)}
	got := e.UnplannedChips(s)
	unspent := s.Second.Wildcard == 0 && s.Second.BenchBoost == 0 &&
		s.Second.TripleCaptain == 0 && s.Second.FreeHit == 0
	if unspent && len(got) == 0 {
		t.Error("in a two-set season the shipped plan leaves the second set entirely " +
			"unspent, and this reported nothing")
	}
	t.Logf("shipped plan against a two-set season: %v", got)
}
