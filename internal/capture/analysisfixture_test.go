package capture

import (
	"os"
	"strings"
	"testing"
)

// internal/analysis builds its deterministic test engines from LiveCapture, and
// cannot say so out loud. e2e/scripts/live-capture.js is the same shape one layer
// further out: a Node harness cannot import a Go constant either.
//
// TestTheScoringPathCannotSeeRecoveredTeamNews (internal/backfill) forbids
// internal/analysis and internal/backtest from importing this package at all — the
// recovery machinery must not be reachable from the scoring path, because every
// figure in the record was measured with backtest.statusAt and wiring real
// availability in reprices all of it. So that package hardcodes the directory name
// rather than referencing the constant. e2e/scripts/live-capture.js hardcodes it for
// the ordinary reason: it is JavaScript, and this package's Go constant does not
// reach it.
//
// A hardcoded duplicate of a name that lives here is exactly the drift this
// repository keeps recording, and its standing answer is to pin every spelling
// against this one rather than trust a convention — the same treatment
// seasonCachePrefix gets from TestTheSeasonCacheVersionMatchesItsPythonReaders.
// This is that pin, and it lives HERE because this side may read the others and
// they may not read this.
func TestEveryFixtureNamesTheLiveCapture(t *testing.T) {
	for _, fixture := range []string{
		"../analysis/capturetestengine_test.go",
		"../../e2e/scripts/live-capture.js",
	} {
		b, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatalf("reading %s: %v — if that file moved, this pin has to move with it, "+
				"or it is free to drift onto a capture nothing checks", fixture, err)
		}
		if !strings.Contains(string(b), LiveCapture) {
			t.Errorf("%s does not name %q anywhere.\n\nIt hardcodes a capture directory "+
				"because it cannot import this package, and that hardcoded name has "+
				"drifted from LiveCapture. Either update it, or update LiveCapture and "+
				"this pin together — silently reading a different capture is how a "+
				"deterministic suite stops testing what its comments claim.",
				fixture, LiveCapture)
		}
	}
}
