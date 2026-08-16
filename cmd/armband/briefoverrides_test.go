package main

import (
	"strings"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// Every kind of override that binds the squad must appear in the overrides section.
//
// This failed silently once, and the way it failed is the reason for the test rather
// than a code review. `Roster` carries four kinds — lock, exclude, minutes and team —
// and the section read two. Nothing errored: the brief printed a "Standing player
// overrides" heading, listed the two exclusions under it, and looked complete.
//
// What it cost is specific. On the 2026-08-13 GW1 brief, Kinsky was in the starting XI
// and van Ewijk on the bench *because* hand-set minutes overrides put them there —
// Kinsky's own note says he "scores 0.41 at £4.5m off 630 minutes as a backup" — and a
// reader had no way to learn that from the document. The section's own text promises
// the opposite: "binding on every squad build and transfer search below".
//
// So the assertion is on the CATEGORIES, not on the wording. A test that checked the
// two kinds that already worked would have passed throughout the defect.
func TestTheOverridesSectionListsEveryKindThatBindsTheSquad(t *testing.T) {
	mins := func(v float64) *float64 { return &v }
	cfg := config.Config{Roster: config.Roster{
		Lock: []config.RosterOverride{
			{Name: "LockedPlayer", Reason: "locked", SetOn: "2026-08-01"},
		},
		Exclude: []config.RosterOverride{
			{Name: "ExcludedPlayer", Reason: "excluded", SetOn: "2026-08-01"},
		},
		Minutes: []config.RosterOverride{
			// The case that actually bit: a backup written UP into the squad.
			{Name: "WrittenUpKeeper", Reason: "named first choice", SetOn: "2026-08-01",
				ExpectedMinutes: mins(88)},
			// And the opposite sign, which must be distinguishable from it.
			{Name: "WrittenDownDefender", Reason: "still rehabilitating",
				SetOn: "2026-08-01", ExpectedMinutes: mins(15)},
		},
		Teams: []config.TeamOverride{
			{Team: "ARS", XGCFactor: 1.15, Reason: "first choice CB out",
				SetOn: "2026-08-01"},
		},
	}}

	e := &analysis.Engine{Boot: &fpl.Bootstrap{}}
	var b strings.Builder
	briefOverrides(&b, cfg, e, time.Now())
	out := b.String()

	for _, want := range []struct{ what, why string }{
		{"LockedPlayer", "a lock"},
		{"ExcludedPlayer", "an exclusion"},
		{"WrittenUpKeeper", "a minutes override — the kind that silently put a " +
			"backup keeper in the starting XI"},
		{"WrittenDownDefender", "a minutes override in the other direction"},
		{"ARS", "a club correction"},
	} {
		if !strings.Contains(out, want.what) {
			t.Errorf("%s is missing from the overrides section, so the brief describes "+
				"a different model from the one that produced its squad (%s)\n\n%s",
				want.what, want.why, out)
		}
	}

	// The VALUE has to be visible, not just the fact of an override: 88 for a backup
	// and 15 for an injured player are opposite interventions, and "minutes override"
	// alone does not say which is holding a player in the squad.
	if !strings.Contains(out, "88") {
		t.Error("a minutes override is listed without the minutes it sets — a reader " +
			"cannot tell a player written up from one written down")
	}
	if !strings.Contains(out, "1.15") {
		t.Error("a club correction is listed without its factor")
	}
}

// An empty roster must still print nothing, so a squad built with no overrides does
// not grow an empty section claiming there are some.
func TestTheOverridesSectionStaysSilentWhenThereAreNone(t *testing.T) {
	var b strings.Builder
	briefOverrides(&b, config.Config{}, &analysis.Engine{Boot: &fpl.Bootstrap{}},
		time.Now())
	if b.Len() != 0 {
		t.Errorf("an empty roster produced an overrides section:\n%s", b.String())
	}
}
