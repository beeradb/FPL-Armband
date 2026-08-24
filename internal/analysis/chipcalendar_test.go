package analysis

import (
	"strings"
	"testing"
)

// The held-squad blanks and doubles a caller passes in. Two blank weeks and two
// double weeks, deliberately of different sizes, so a test can tell "the biggest
// week" from "the first week".
func fixtureWeeks() (blanks, doubles map[int][]string) {
	blanks = map[int][]string{
		18: {"Gvardiol", "Haaland"},
		29: {"Isak", "Palmer", "Saka", "Watkins"},
		31: {},
	}
	doubles = map[int][]string{
		25: {"Salah"},
		34: {"Gvardiol", "Haaland", "Salah"},
	}
	return blanks, doubles
}

func plan(t *testing.T, fh, bb, tc, wc int) ChipSchedule {
	t.Helper()
	return ChipSchedule{First: ChipPlan{
		FreeHit: fh, BenchBoost: bb, TripleCaptain: tc, Wildcard: wc,
	}}
}

// A chip already on the right kind of week is confirmed, not nagged. The
// previous shape of this surface had no opinion at all, so the failure mode
// being guarded is a note that fires on a correct plan.
func TestChipCalendarNotesConfirmsAPlanAlreadyOnTheCalendar(t *testing.T) {
	blanks, doubles := fixtureWeeks()
	notes := ChipCalendarNotes(plan(t, 29, 34, 34, 0), blanks, doubles)
	if len(notes) != 3 {
		t.Fatalf("want one note per anchored chip, got %d: %v", len(notes), notes)
	}
	for _, n := range notes {
		if !strings.Contains(n, "already sits on the calendar") {
			t.Errorf("a chip on the right week should be confirmed, got %q", n)
		}
	}
	if !strings.Contains(notes[0], "Isak, Palmer, Saka, Watkins") {
		t.Errorf("the confirmation should name who is affected, got %q", notes[0])
	}
}

// The point of the surface: a legal week that is an ordinary one.
func TestChipCalendarNotesFlagsAnOrdinaryWeekAndOffersTheAlternatives(t *testing.T) {
	blanks, doubles := fixtureWeeks()
	notes := ChipCalendarNotes(plan(t, 12, 0, 0, 0), blanks, doubles)
	if len(notes) != 1 {
		t.Fatalf("want exactly the free hit note, got %v", notes)
	}
	n := notes[0]
	for _, want := range []string{"Free Hit", "GW12", "ordinary week", "GW18 (2)", "GW29 (4)"} {
		if !strings.Contains(n, want) {
			t.Errorf("note should contain %q, got %q", want, n)
		}
	}
	// GW31 is in the map with nobody in it — an irregular league week that does
	// not touch this squad. Offering it would send a manager to a week that does
	// nothing for him, which is the bug this case exists for.
	if strings.Contains(n, "GW31") {
		t.Errorf("a week affecting none of the held fifteen must not be offered: %q", n)
	}
}

// A free hit wants a blank and a bench boost wants a double, and mixing them up
// would produce confident advice pointing exactly the wrong way.
func TestChipCalendarNotesDoesNotOfferBlanksForABenchBoost(t *testing.T) {
	blanks, doubles := fixtureWeeks()
	notes := ChipCalendarNotes(plan(t, 0, 12, 0, 0), blanks, doubles)
	if len(notes) != 1 {
		t.Fatalf("want exactly the bench boost note, got %v", notes)
	}
	n := notes[0]
	if !strings.Contains(n, "GW25 (1)") || !strings.Contains(n, "GW34 (3)") {
		t.Errorf("bench boost should be offered the doubles, got %q", n)
	}
	for _, blank := range []string{"GW18", "GW29"} {
		if strings.Contains(n, blank) {
			t.Errorf("bench boost must not be offered blank week %s: %q", blank, n)
		}
	}
}

// The wildcard is not calendar-anchored here, and that is a decision rather than
// an omission — see wantsWeek, and TestDiagAnchoredChips' "no arm plays a
// wildcard". A future refactor that adds it should have to delete this test.
func TestChipCalendarNotesSaysNothingAboutTheWildcard(t *testing.T) {
	blanks, doubles := fixtureWeeks()
	for _, n := range ChipCalendarNotes(plan(t, 0, 0, 0, 12), blanks, doubles) {
		t.Errorf("the wildcard is deliberately unanchored, got %q", n)
	}
}

// Before any blank or double is scheduled the honest output is silence: the
// command already prints "None scheduled yet" above this section, and a second
// empty-handed paragraph would read as advice.
func TestChipCalendarNotesIsSilentBeforeAnyIrregularWeekIsKnown(t *testing.T) {
	empty := map[int][]string{}
	for _, n := range ChipCalendarNotes(plan(t, 12, 13, 14, 15), empty, empty) {
		t.Errorf("nothing is known yet, so nothing should be claimed, got %q", n)
	}
}

// An unplanned chip is not a problem to report. Zero means unplanned, per
// ChipPlan's own doc comment, and nagging about it would make the section fire
// for every manager who has not filled the plan in.
func TestChipCalendarNotesIgnoresUnplannedChips(t *testing.T) {
	blanks, doubles := fixtureWeeks()
	if notes := ChipCalendarNotes(plan(t, 0, 0, 0, 0), blanks, doubles); len(notes) != 0 {
		t.Errorf("an empty plan has nothing to say against, got %v", notes)
	}
}

// ⚠️ The one thing this surface must never do. Whether anchoring a chip on the
// calendar is worth a measurable amount is not a shipped figure — the only
// thing that measures it is a DIAG-only sweep — so a points claim appearing
// here would be a number the repo cannot support.
func TestChipCalendarNotesNeverQuotesAPointsFigure(t *testing.T) {
	blanks, doubles := fixtureWeeks()
	all := append(
		ChipCalendarNotes(plan(t, 12, 13, 14, 15), blanks, doubles),
		ChipCalendarNotes(plan(t, 29, 34, 34, 0), blanks, doubles)...)
	for _, n := range all {
		for _, banned := range []string{"point", "pts", "worth", "gain", "expect"} {
			if strings.Contains(strings.ToLower(n), banned) {
				t.Errorf("note quotes or implies a value claim (%q): %q", banned, n)
			}
		}
	}
}

// The second set is a real slot, and a plan that uses it must be read against
// the calendar too — a rule keyed to the first set only would go quiet exactly
// when a season grants two sets of chips.
func TestChipCalendarNotesReadsTheSecondSet(t *testing.T) {
	blanks, doubles := fixtureWeeks()
	s := ChipSchedule{Second: ChipPlan{FreeHit: 12}}
	notes := ChipCalendarNotes(s, blanks, doubles)
	if len(notes) != 1 {
		t.Fatalf("want the second-set free hit note, got %v", notes)
	}
	if !strings.Contains(notes[0], "second set") {
		t.Errorf("the note should say which set it is about, got %q", notes[0])
	}
}
