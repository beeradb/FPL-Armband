package capture

import (
	"os"
	"strings"
	"testing"
)

// internal/analysis builds its deterministic test engines from LiveCapture, and
// cannot say so out loud.
//
// TestTheScoringPathCannotSeeRecoveredTeamNews (internal/backfill) forbids
// internal/analysis and internal/backtest from importing this package at all — the
// recovery machinery must not be reachable from the scoring path, because every
// figure in the record was measured with backtest.statusAt and wiring real
// availability in reprices all of it. So that package hardcodes the directory name
// rather than referencing the constant.
//
// A hardcoded duplicate of a name that lives here is exactly the drift this
// repository keeps recording, and its standing answer is to pin the two spellings
// against one another rather than trust a convention — the same treatment
// seasonCachePrefix gets from TestTheSeasonCacheVersionMatchesItsPythonReaders.
// This is that pin, and it lives HERE because this side may read the other and the
// other may not read this.
func TestTheAnalysisFixtureNamesTheLiveCapture(t *testing.T) {
	const fixture = "../analysis/capturetestengine_test.go"
	b, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading %s: %v — if that file moved, this pin has to move with it, "+
			"or internal/analysis is free to drift onto a capture nothing checks", fixture, err)
	}
	if !strings.Contains(string(b), LiveCapture) {
		t.Errorf("%s does not name %q anywhere.\n\nIt hardcodes a capture directory "+
			"because it may not import this package, and that hardcoded name has "+
			"drifted from LiveCapture. Either update it, or update LiveCapture and "+
			"this pin together — silently reading a different capture is how a "+
			"deterministic suite stops testing what its comments claim.",
			fixture, LiveCapture)
	}
}
