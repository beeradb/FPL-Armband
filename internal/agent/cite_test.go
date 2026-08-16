package agent

import (
	"strings"
	"testing"
	"time"

	"armband/internal/config"
	"armband/internal/fpl"
)

// The model wraps web-sourced claims in <cite index="..."> tags. They were
// landing verbatim in the terminal and the Markdown report, where the indices
// mean nothing to a reader.
func TestCiteTagStripping(t *testing.T) {
	cases := []struct{ in, want string }{
		{`<cite index="16-8">Opening round pushed back.</cite>`, "Opening round pushed back."},
		{`<cite index="8-5,8-6,8-7">Three clubs.</cite>`, "Three clubs."},
		{"plain text, no tags", "plain text, no tags"},
		{`before <cite index="1-1">middle</cite> after`, "before middle after"},
		{`<CITE INDEX="2-2">upper</CITE>`, "upper"},
		// Must not eat unrelated angle-bracket content.
		{"a < b and c > d", "a < b and c > d"},
		{"<citation>keep this</citation>", "<citation>keep this</citation>"},
	}
	for _, tc := range cases {
		if got := citeTag.ReplaceAllString(tc.in, ""); got != tc.want {
			t.Errorf("stripping %q\n got %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

// TestPromptRequiresRecordingOverrides — set_player_status exists so a finding
// survives the run that made it, and the first live review that had it available
// made exactly the finding it is for (Saliba out, Arsenal's clean-sheet rate no
// longer credible) and left it in prose. The tool being described was not enough;
// the workflow has to ask for it at the point the decision is made.
func TestPromptRequiresRecordingOverrides(t *testing.T) {
	cfg := config.Default()
	p := SystemPrompt(cfg, &fpl.Bootstrap{})
	for _, want := range []string{
		"set_player_status",
		"until_gameweek",
		"standing_overrides",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt never mentions %q, so the agent has no instruction "+
				"to persist what it learns", want)
		}
	}
	// The distinction that keeps the roster from filling with opinions.
	if !strings.Contains(p, "availability and role, not for taste") {
		t.Error("the prompt does not say overrides are for availability and role only")
	}
}

// TestPromptFlagsOverridesDueACheck — recording a finding is only half of it. An
// expiry date is a guess, and a player back early is the expensive error: the
// exclusion holds, he is never considered, and nothing surfaces it because the
// squad simply never contains him. The brief has to put the stale ones in front
// of the agent by name.
func TestPromptFlagsOverridesDueACheck(t *testing.T) {
	cfg := config.Default()
	cfg.Roster.Exclude = []config.RosterOverride{
		{Code: 1, Name: "LongInjured", Reason: "back", SetOn: "2020-01-01",
			LastChecked: "2020-01-01", UntilGameweek: 30},
		{Code: 2, Name: "JustChecked", Reason: "rotation", SetOn: "2020-01-01",
			LastChecked: time.Now().Format("2006-01-02"), UntilGameweek: 30},
	}
	p := SystemPrompt(cfg, &fpl.Bootstrap{})

	if !strings.Contains(p, "LongInjured") || !strings.Contains(p, "JustChecked") {
		t.Fatal("standing overrides are not listed in the brief at all")
	}
	stale := strings.Index(p, "LongInjured")
	line := p[stale:]
	if end := strings.Index(line, "\n"); end > 0 {
		line = line[:end]
	}
	if !strings.Contains(line, "CHECK") {
		t.Errorf("an override unchecked since 2020 is not flagged: %q", line)
	}
	if !strings.Contains(p, "back early is the expensive error") {
		t.Error("the brief does not explain which direction of error costs more")
	}
}
