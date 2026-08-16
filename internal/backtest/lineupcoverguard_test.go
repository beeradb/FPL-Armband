package backtest

import (
	"strings"
	"testing"
)

// TestALineupRestrictionNeedsTheLineupsOracle pins the guard in recentIndex.
//
// A guard whose only exercise is the path it guards is one nobody has ever seen
// fire. This one refuses `lineupCovers` set without `OracleLineups`, and the
// failure it prevents is the shape this package keeps paying for: an arm labelled
// "minutes: flagged" that quietly ran the *unrestricted* minutes oracle would
// reproduce the unrestricted arm digit for digit, and the record would read that
// as the structural inertness the flagged lineups arm genuinely has. Two arms
// agreeing exactly is evidence here, so anything that can manufacture it without
// the mechanism has to be impossible rather than merely discouraged.
//
// `FeatureScope` gets the same refusal from `Oracles.Validate`. `lineupCovers`
// cannot: it is unexported and lives on `SimConfig` rather than on `Oracles`, so
// the validator cannot see it and the check has to sit where the value is read.
func TestALineupRestrictionNeedsTheLineupsOracle(t *testing.T) {
	covers := func(_ *Player, st string) bool { return st != "a" }
	s := &Season{Name: "2025-26"}

	for _, tc := range []struct {
		name    string
		oracles Oracles
		wantPan bool
	}{
		{"no oracle at all", Oracles{}, true},
		{"the wrong oracle", Oracles{Info: OracleMinutes}, true},
		{"a Status oracle, not a minutes one", Oracles{Info: OracleFeatures}, true},
		{"the right oracle", Oracles{Info: OracleLineups}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := SimConfig{Oracles: tc.oracles, lineupCovers: covers}
			defer func() {
				r := recover()
				switch {
				case r == nil && tc.wantPan:
					t.Fatal("the restriction was accepted and would have been " +
						"silently dropped, so the arm would report the " +
						"unrestricted oracle under a restricted label")
				case r != nil && !tc.wantPan:
					t.Fatalf("the correct combination was refused: %v", r)
				case r != nil:
					if msg, ok := r.(string); !ok ||
						!strings.Contains(msg, "lineupCovers") {
						t.Fatalf("the panic does not name the field a reader has "+
							"to go and find: %v", r)
					}
				}
			}()
			// Gameweek 0 keeps this cheap: newRecentIndexWith returns nil before a
			// gameweek is played, so the guard is reached and nothing else is.
			cfg.recentIndex(s, 0)
		})
	}
}
