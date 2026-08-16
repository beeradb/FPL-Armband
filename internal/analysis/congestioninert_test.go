package analysis

import "testing"

// TestTheShippedCongestionBlockIsInert pins every congestion penalty at 1.0.
//
// # Why a test rather than eight comments
//
// All eight now ship at 1.0, so the block changes no score. Five got there by
// measurement — the three European penalties and both rest penalties measured as
// nothing on the channel they are applied to — and three by the weaker but
// sufficient argument that an unmeasured multiplier which moves a score is not
// neutral, and 1.0 is.
//
// Eight separate constants at 1.0 is an accident waiting to be undone. Without a
// guard, re-enabling an unmeasured discount is a one-character edit that nothing
// notices, which is exactly how the long-haul penalty came to be applying 14% to
// Brazilians and Argentines while its own comment said it was inert.
//
// This is the complement of TestUnsetPenaltyIsANoOp, which guards the neighbouring
// trap: a penalty left at ZERO once multiplied instead of being skipped, so a
// replayed December scored every congested player at nought. That test catches a
// missing value; this one catches a value that is present, live, and unmeasured.
//
// # Deleting the block is not the same as zeroing it, and is not what this says
//
// The machinery and the four hand-maintained season lists stay. They are still
// reported — `armband congestion` prints them and the agent reads them — and the
// judgement layer is the part of this system measured as worth the most. What
// changes is the blast radius: a stale European entry can no
// longer mis-SCORE a player, only mis-inform a human. Say that when describing the
// summer maintenance task, which this file elsewhere describes as failing silently
// in a way that costs points.
//
// ⚠️ **Corrected 2026-08-15: this said "a stale European entry OR A WRONG REST NAME",
// and the rest half is false. `DefaultRestPlayers` is live on the scoring path.**
//
// Two different things answer to "rest" and only one of them is in this block. The
// penalties measured as nothing here are `ShortRestPenalty` and `VeryShortRest` —
// fixture congestion. `rest_players` is the post-tournament list, and it reaches
// minutes through a separate mechanism: `blendFor` applies `restFactor` at
// `blend.go:165`, multiplying `MinutesPerMatch` and `StartShare` by
// `rest_minutes_factor` (0.83). So a wrong NAME on that list really does mis-score
// a player, and this comment told the next maintainer it could not.
//
// ⚠️ Do not check this by grepping `restFactor`: its other call site,
// `metrics.go:1590`, is labelled "Reporting only. The factor was already applied to
// minutes inside blendFor" — which reads as a refutation and is not one. That is
// why the false claim survived in four places.
//
// It is live at **GW1 and GW2 only** — `metrics.go:1649` returns 1 once
// `next.ID > RestGameweeks` and `rest_gameweeks` is 2 — which is precisely the two
// gameweeks in front of whoever does the summer maintenance.
//
// If the idea is revisited, the right channel is minutes rather than Score. Under
// four days' rest genuinely costs 4.3% of minutes, but on Score the measured effect
// is *positive* — points up 2.7%, points per 90 up 7.2% — because who plays a
// midweek round is chosen and the chosen are the trusted. That is why five of these
// are 1.0 despite a real underlying effect, and why measuring the remaining three on
// this channel would measure the wrong quantity.
func TestTheShippedCongestionBlockIsInert(t *testing.T) {
	cg := DefaultCongestion()

	for _, c := range []struct {
		name string
		got  float64
	}{
		{"UCLPenalty", cg.UCLPenalty},
		{"UELPenalty", cg.UELPenalty},
		{"UECLPenalty", cg.UECLPenalty},
		{"DomesticCupPenalty", cg.DomesticCupPenalty},
		{"ShortRestPenalty", cg.ShortRestPenalty},
		{"VeryShortRest", cg.VeryShortRest},
		{"PostBreakPenalty", cg.PostBreakPenalty},
		{"LongHaulPenalty", cg.LongHaulPenalty},
	} {
		if c.got != 1.0 {
			t.Errorf("%s ships at %v, want 1.0.\n"+
				"Every congestion penalty is deliberately neutral: five measured as "+
				"nothing and three are unmeasured, and an unmeasured multiplier that "+
				"moves a score is not neutral. If you are re-enabling this on new "+
				"evidence, measure it on MINUTES rather than Score and say so here — "+
				"see the note on this test for why the channel is the whole issue.",
				c.name, c.got)
		}
	}
}
