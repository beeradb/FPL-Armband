package analysis

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The second set is the whole reason this type exists, and the failures it
// guards against are silent ones: a plan that expresses only half of what was
// written, or a slot name that resolves to the wrong chip. Neither shows up in a
// points total — the season simply plays fewer chips than the author asked for.

func TestEverySlotIsAddressableAndDistinct(t *testing.T) {
	slots := ChipSlots()
	if len(slots) != 8 {
		t.Fatalf("got %d slots, want 8 — four chips in each of two sets", len(slots))
	}

	// Written one at a time and read back one at a time: if two names resolved
	// to the same field, the later write would overwrite the earlier and the
	// read-back would disagree.
	var s ChipSchedule
	for i, slot := range slots {
		if err := s.Set(slot, i+1); err != nil {
			t.Fatalf("Set(%q): %v", slot, err)
		}
	}
	for i, slot := range slots {
		got, err := s.Get(slot)
		if err != nil {
			t.Fatalf("Get(%q): %v", slot, err)
		}
		if got != i+1 {
			t.Errorf("slot %s round-tripped to GW%d, want GW%d — two slots share a field", slot, got, i+1)
		}
	}
	if n := len(s.All()); n != 8 {
		t.Errorf("All() reported %d planned chips, want 8", n)
	}
}

// A bare name has always meant the first set, and a plan written before the
// second set existed must keep meaning what it meant.
func TestABareSlotNameMeansTheFirstSet(t *testing.T) {
	for _, bare := range []string{"wc", "fh", "bb", "tc", "wildcard", "free_hit", "bench_boost", "3xc"} {
		var s ChipSchedule
		if err := s.Set(bare, 7); err != nil {
			t.Fatalf("Set(%q): %v", bare, err)
		}
		if s.Second != (ChipPlan{}) {
			t.Errorf("%q wrote into the second set", bare)
		}
		if s.First == (ChipPlan{}) {
			t.Errorf("%q wrote into neither set", bare)
		}
	}
}

func TestAnUnknownSlotIsAnError(t *testing.T) {
	var s ChipSchedule
	for _, bad := range []string{"", "wc3", "wildcard3", "bench", "xx1", "3"} {
		if err := s.Set(bad, 5); err == nil {
			t.Errorf("Set(%q) was accepted; an unrecognised chip must fail loudly "+
				"rather than plan nothing", bad)
		}
	}
	if s != (ChipSchedule{}) {
		t.Error("a rejected Set still modified the schedule")
	}
}

// SetAll is all-or-nothing. A half-applied plan is one nobody wrote, and it
// would be played without complaint.
func TestSetAllIsAllOrNothing(t *testing.T) {
	s := ChipSchedule{First: ChipPlan{Wildcard: 4}}
	before := s
	err := s.SetAll(map[string]int{"bb1": 14, "nonsense": 3, "tc2": 30})
	if err == nil {
		t.Fatal("SetAll accepted an unknown slot")
	}
	if s != before {
		t.Errorf("SetAll left a partly-applied plan: %+v", s)
	}

	if err := s.SetAll(map[string]int{"bb1": 14, "tc2": 30}); err != nil {
		t.Fatalf("SetAll: %v", err)
	}
	if s.First.BenchBoost != 14 || s.Second.TripleCaptain != 30 || s.First.Wildcard != 4 {
		t.Errorf("SetAll did not apply across both sets, or dropped what was there: %+v", s)
	}
}

func TestScheduleRoundTripsThroughItsOwnSyntax(t *testing.T) {
	want := ChipSchedule{
		First:  ChipPlan{Wildcard: 4, BenchBoost: 14},
		Second: ChipPlan{FreeHit: 29, TripleCaptain: 34},
	}
	got, err := ParseChipSchedule(want.String())
	if err != nil {
		t.Fatalf("parsing %q: %v", want.String(), err)
	}
	if got != want {
		t.Errorf("round trip through %q gave %+v, want %+v", want.String(), got, want)
	}
}

// The suffix must not be trimmed off a name that legitimately ends in a digit.
// "3xc" is FPL's own name for the triple captain and ends in a letter, but the
// rule that produced it — strip any trailing digit — would read "3" as a chip.
func TestSlotParsingDoesNotEatALegitimateName(t *testing.T) {
	var s ChipSchedule
	if err := s.Set("3xc", 9); err != nil {
		t.Fatalf(`Set("3xc"): %v`, err)
	}
	if s.First.TripleCaptain != 9 {
		t.Errorf(`"3xc" did not resolve to the triple captain: %+v`, s)
	}
	if err := s.Set("3xc2", 33); err != nil {
		t.Fatalf(`Set("3xc2"): %v`, err)
	}
	if s.Second.TripleCaptain != 33 {
		t.Errorf(`"3xc2" did not resolve to the second-set triple captain: %+v`, s)
	}
}

// The config backfill. An existing config.json carries a flat single-set object,
// and it must keep loading — as the FIRST set, which is the only one the seasons
// those files were written for granted.
func TestALegacyFlatChipPlanStillLoads(t *testing.T) {
	const legacy = `{"wildcard_gameweek":6,"free_hit_gameweek":16,` +
		`"bench_boost_gameweek":8,"triple_captain_gameweek":9}`
	var s ChipSchedule
	if err := json.Unmarshal([]byte(legacy), &s); err != nil {
		t.Fatalf("legacy chip_plan no longer loads: %v", err)
	}
	want := ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9}
	if s.First != want {
		t.Errorf("legacy plan loaded as %+v, want it in the first set as %+v", s.First, want)
	}
	if s.Second != (ChipPlan{}) {
		t.Errorf("legacy plan invented a second set: %+v", s.Second)
	}
}

// A one-set plan must marshal back to the FLAT form it was read from.
//
// Otherwise the first save after upgrading rewrites every existing config.json
// — an unexplained diff on a tracked file — and worse, it is a one-way door: a
// consumer still typed ChipPlan reads the two-set object as all zeros with no
// error, losing the whole plan. Found by the security review of this change.
func TestASingleSetScheduleStillWritesTheFlatForm(t *testing.T) {
	s := ChipSchedule{First: ChipPlan{Wildcard: 6, BenchBoost: 8}}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	if err := json.Unmarshal(b, &probe); err != nil {
		t.Fatal(err)
	}
	if _, two := probe["first"]; two {
		t.Errorf("a one-set schedule wrote the two-set form (%s); every existing "+
			"config.json would be rewritten on first save", b)
	}
	if probe["wildcard_gameweek"] != float64(6) {
		t.Errorf("flat form lost the wildcard: %s", b)
	}

	// And an old reader must still get the plan out of it.
	var legacy ChipPlan
	if err := json.Unmarshal(b, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != s.First {
		t.Errorf("a ChipPlan-typed reader got %+v, want %+v", legacy, s.First)
	}
}

func TestANullChipPlanLeavesTheReceiverAlone(t *testing.T) {
	s := ChipSchedule{First: ChipPlan{Wildcard: 6}}
	if err := s.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatal(err)
	}
	if s.First.Wildcard != 6 {
		t.Error("a JSON null zeroed the schedule; the Unmarshaler convention is a " +
			"no-op, and zeroing silently loses a non-zero default")
	}
}

// A mixed object is not a plan anybody wrote on purpose, and taking either
// branch silently discards the other's keys.
func TestAMixedChipPlanFormIsRefused(t *testing.T) {
	var s ChipSchedule
	err := s.UnmarshalJSON([]byte(`{"first":{"wildcard_gameweek":4},"bench_boost_gameweek":14}`))
	if err == nil {
		t.Error("a config mixing the flat and two-set forms was accepted; the flat " +
			"sibling would be dropped with nothing saying so")
	}
}

// UnmarshalJSON was the one way into the type that skipped the range check Set
// and ParseChipSchedule both enforce.
func TestAnOutOfRangeGameweekIsRefusedOnLoadToo(t *testing.T) {
	for _, bad := range []string{
		`{"wildcard_gameweek":99}`,
		`{"free_hit_gameweek":-3}`,
		`{"first":{"bench_boost_gameweek":40}}`,
	} {
		var s ChipSchedule
		if err := s.UnmarshalJSON([]byte(bad)); err == nil {
			t.Errorf("%s loaded without complaint; Set rejects the same value", bad)
		}
	}
}

func TestTheTwoSetFormSurvivesJSON(t *testing.T) {
	want := ChipSchedule{
		First:  ChipPlan{Wildcard: 4, BenchBoost: 14},
		Second: ChipPlan{Wildcard: 24, BenchBoost: 34},
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got ChipSchedule
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("round trip through JSON gave %+v, want %+v", got, want)
	}
}

// Next is what stops a two-set plan reverting to single-set behaviour. A
// decision in March handed September's wildcard sees a rebuild behind it and
// prepares as though none were coming.
func TestNextFindsTheChipAheadRatherThanTheFieldsContents(t *testing.T) {
	s := ChipSchedule{
		First:  ChipPlan{Wildcard: 6},
		Second: ChipPlan{Wildcard: 28},
	}
	for _, c := range []struct{ from, want int }{
		{1, 6}, {6, 6}, {7, 28}, {28, 28}, {29, 0},
	} {
		if got := s.Next("wc", c.from); got != c.want {
			t.Errorf("Next(wc, %d) = %d, want %d", c.from, got, c.want)
		}
	}
}

func TestWeeksReportsBothSets(t *testing.T) {
	s := ChipSchedule{First: ChipPlan{FreeHit: 18}, Second: ChipPlan{FreeHit: 29}}
	if got := s.Weeks(SlotFreeHit); !reflect.DeepEqual(got, []int{18, 29}) {
		t.Errorf("Weeks(fh) = %v, want [18 29] — a second free hit that goes "+
			"unreported is one the squad is still being optimised for", got)
	}
}

// A bare name asks across both sets; a suffixed one asks about that set alone.
//
// The readers parsed the suffix and then discarded it, so `Next("wc2", …)` read
// as "the second wildcard" and meant "either wildcard" — a wrong answer that
// looks like the right one, and the writers (`Set`, `Get`) already honoured the
// suffix, so the two halves of the type disagreed. Found by review.
func TestASuffixedSlotAsksAboutThatSetAlone(t *testing.T) {
	s := ChipSchedule{
		First:  ChipPlan{Wildcard: 6, FreeHit: 18},
		Second: ChipPlan{Wildcard: 28, FreeHit: 29},
	}
	for _, c := range []struct {
		slot  string
		next  int
		weeks []int
	}{
		{"wc", 6, []int{6, 28}},
		{"wc1", 6, []int{6}},
		{"wc2", 28, []int{28}},
		{"fh2", 29, []int{29}},
	} {
		if got := s.Next(c.slot, 1); got != c.next {
			t.Errorf("Next(%s, 1) = %d, want %d", c.slot, got, c.next)
		}
		if got := s.Weeks(c.slot); !reflect.DeepEqual(got, c.weeks) {
			t.Errorf("Weeks(%s) = %v, want %v", c.slot, got, c.weeks)
		}
	}
	if !s.Plays("wc2", 28) || s.Plays("wc1", 28) {
		t.Error("Plays ignored the set suffix")
	}
}

// From is what the replay uses to hand a decision only the chips still ahead of
// it, and it must not collapse the two sets while doing so.
func TestFromKeepsBothSetsAndDropsWhatIsPast(t *testing.T) {
	s := ChipSchedule{
		First:  ChipPlan{Wildcard: 6, FreeHit: 18},
		Second: ChipPlan{Wildcard: 28, FreeHit: 29},
	}
	got := s.From(19)
	want := ChipSchedule{Second: ChipPlan{Wildcard: 28, FreeHit: 29}}
	if got != want {
		t.Errorf("From(19) = %+v, want %+v", got, want)
	}
	if got := s.From(1); got != s {
		t.Errorf("From(1) dropped something that is still ahead: %+v", got)
	}
}

func TestEntriesAreOrderedAndLabelTheSecondSet(t *testing.T) {
	s := ChipSchedule{
		First:  ChipPlan{BenchBoost: 14},
		Second: ChipPlan{Wildcard: 24},
	}
	got := s.Entries()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	// First set before second, whatever the gameweeks say.
	if got[0].Slot != "bb1" || got[1].Slot != "wc2" {
		t.Errorf("entries out of canonical order: %+v", got)
	}
	if got[0].Label != "Bench Boost" {
		t.Errorf("first-set label is %q, want an unsuffixed one", got[0].Label)
	}
	if got[1].Label != "Wildcard (second set)" {
		t.Errorf("second-set label is %q; a reader cannot tell the sets apart", got[1].Label)
	}
}
