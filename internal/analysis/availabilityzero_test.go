package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// TestAFlaggedPlayerScoresExactlyZero pins the multiplication that makes a whole
// family of measurements come out at exactly 0.000 rather than merely small.
//
// # Why this is worth a test of its own
//
// `Score` ends in `* availabilityFactor(el)`, and that factor is literally 0 for
// a player FPL marks injured, suspended, unavailable or on loan. Everything
// upstream — expected minutes, the rate blend, the recency index, the fixture
// ladder — is multiplied away.
//
// That turns out to be load-bearing for the oracle harness. Restricting the
// lineups oracle to players the model's own reconstruction flags returns 0.0% of
// 918 gameweeks changed, and the unflagged arm reproduces the unrestricted arm
// digit for digit. That is not a null result, it is this zero: the oracle
// rewrites minutes, and a flagged player's minutes multiply by nothing. A
// reviewer had to trace four files to establish it, so it is written down here as
// an executable statement instead.
//
// **If this test ever fails, several recorded figures need re-deriving** — the
// backtest declares `lineups: flagged` at exactly 0.000 as an invariance, and
// that declaration is only true while this holds.
//
// The zero is deliberate and is not being argued against. FPL pays a player who
// does not feature exactly nothing, so scoring him at zero is what the game says.
// What the test protects is the *sharpness*: a change to 0.05, defensible on its
// own terms, would silently convert an exact invariance into an approximate one.
func TestAFlaggedPlayerScoresExactlyZero(t *testing.T) {
	// The four codes availabilityFactor zeroes, and the two it does not, so the
	// test states the whole rule rather than one corner of it.
	for _, tc := range []struct {
		status string
		want   float64
	}{
		{"i", 0},   // injured
		{"s", 0},   // suspended
		{"u", 0},   // unavailable — left the club, or not in the squad
		{"n", 0},   // on loan / ineligible
		{"a", 1},   // available
		{"d", 0.5}, // doubtful. Unreachable from the replay's reconstruction,
		// which emits only a/u/i — but reachable live, and from the
		// recovered-capture oracle, so it belongs in the rule.
	} {
		el := &fpl.Element{Status: tc.status}
		if got := availabilityFactor(el); got != tc.want {
			t.Errorf("availabilityFactor(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}

	// And the percentage wins over the flag when it is present, which is why an
	// arm carrying the percentage over a reconstructed status would price two
	// populations on two different availability models.
	chance := 25
	el := &fpl.Element{Status: "i", ChanceOfPlayingNextRound: &chance}
	if got := availabilityFactor(el); got != 0.25 {
		t.Errorf("a published chance must override the flag: got %v, want 0.25", got)
	}
}
