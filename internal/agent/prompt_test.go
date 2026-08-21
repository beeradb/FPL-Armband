package agent

import (
	"strings"
	"testing"

	"armband/internal/config"
	"armband/internal/fpl"
)

// The system prompt's "Standing player overrides" section had the same gap
// cmd/armband's briefOverrides once had (see its own TestTheOverridesSectionLists-
// EveryKindThatBindsTheSquad): it read LOCKED IN and EXCLUDED but never Minutes or
// Teams, so the agent never learned why a minutes correction put a player in the
// squad, and had no way at all to receive a flag-only note — a reason attached to a
// player with no expected_minutes value, recording a judgement call (e.g. a role
// change) rather than a fact the model can compute for itself.
func TestTheSystemPromptListsEveryKindOfOverride(t *testing.T) {
	mins := func(v float64) *float64 { return &v }
	cfg := config.Config{Roster: config.Roster{
		Lock: []config.RosterOverride{
			{Name: "LockedPlayer", Reason: "locked", SetOn: "2026-08-01"},
		},
		Exclude: []config.RosterOverride{
			{Name: "ExcludedPlayer", Reason: "excluded", SetOn: "2026-08-01"},
		},
		Minutes: []config.RosterOverride{
			{Name: "WrittenUpKeeper", Reason: "named first choice", SetOn: "2026-08-01",
				ExpectedMinutes: mins(88)},
			// The case this test exists for: no expected_minutes at all.
			{Name: "FlaggedMidfielder", Reason: "deeper role this season, watch for fewer returns",
				SetOn: "2026-08-01"},
		},
		Teams: []config.TeamOverride{
			{Team: "ARS", XGCFactor: 1.15, Reason: "first choice CB out", SetOn: "2026-08-01"},
		},
	}}

	out := SystemPrompt(cfg, &fpl.Bootstrap{})

	for _, want := range []struct{ what, why string }{
		{"LockedPlayer", "a lock"},
		{"ExcludedPlayer", "an exclusion"},
		{"WrittenUpKeeper", "a minutes override with a value set"},
		{"FlaggedMidfielder", "a minutes override with NO value set — the case that motivated this test"},
		{"ARS", "a club correction"},
	} {
		if !strings.Contains(out, want.what) {
			t.Errorf("%s is missing from the system prompt's overrides section, so the agent "+
				"never learns about it (%s)\n\n%s", want.what, want.why, out)
		}
	}

	if !strings.Contains(out, "88") {
		t.Error("the numeric minutes override is listed without the minutes value it sets")
	}
	if !strings.Contains(out, "1.15") {
		t.Error("the club correction is listed without its factor")
	}
	// The flag-only entry must not read as if it set a number — "88" appearing near
	// FlaggedMidfielder would be the WrittenUpKeeper row bleeding into an assertion
	// that was supposed to be about the other player, so check its own line directly.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "FlaggedMidfielder") && strings.Contains(line, "->") {
			t.Errorf("FlaggedMidfielder has no expected_minutes, so its row must not carry "+
				"a minutes arrow/value: %s", line)
		}
	}
}
