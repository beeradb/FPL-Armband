package main

import (
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// TestEveryAgentPromptIsNonEmpty pins the precondition cmdAgent enforces.
//
// cmdAgent rejects an empty prompt rather than sending one, because an empty
// prompt would bill a request, print nothing and write no report — the silent
// no-op shape this package guards against everywhere else. That guard is the
// last line of defence and it fires in the worst possible place: `due` calls
// writeDueState BEFORE cmdAgent, and checkDue then refuses to run again for
// that gameweek, so a prompt builder that ever returned "" would consume a
// scheduled review permanently, with no report and no retry.
//
// This test moves the failure from the cron path to the build. Both builders
// are checked against an engine carrying an EMPTY bootstrap, which is the
// degenerate case: NextEvent returns nil, the gameweek name falls back to
// prose, and the prompt must still be a real instruction rather than the empty
// string. (The bootstrap has to be non-nil — NextEvent is a pointer method and
// both builders call it unguarded.)
//
// Retiring `ask` is what made this worth pinning. While it existed a caller
// could pass arbitrary text, so an empty prompt was a user error the command
// checked for itself; now every prompt is built in this file, and a builder
// returning "" is a programming error nothing else would catch.
func TestEveryAgentPromptIsNonEmpty(t *testing.T) {
	e := &analysis.Engine{Boot: &fpl.Bootstrap{}}

	for _, tc := range []struct {
		name  string
		build func(*analysis.Engine) string
	}{
		{"advicePrompt", advicePrompt},
		{"reviewPrompt", reviewPrompt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.build(e)
			if strings.TrimSpace(got) == "" {
				t.Fatalf("%s returned an empty prompt on a zero engine; cmdAgent "+
					"would reject it, and under `due` that costs the gameweek's review", tc.name)
			}
			// A prompt that degraded to the fallback gameweek wording is fine.
			// One that lost its instructions is not — guard against a builder
			// that returns only a heading.
			if len(got) < 200 {
				t.Errorf("%s returned only %d bytes, which is too short to be the "+
					"full instruction:\n%s", tc.name, len(got), got)
			}
		})
	}
}
