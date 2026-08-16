package analysis

import "testing"

// TestCongestionPenaltiesAreMeasured pins the constants that were measured to
// 1.0, so restoring one is a deliberate act with evidence behind it.
//
// Three of these looked obviously right and are worth nothing: European
// football, three days' rest, and the week after an international break. See
// TestDiagEuropeanPenalty and TestDiagCongestionRest in internal/backtest.
func TestCongestionPenaltiesAreMeasured(t *testing.T) {
	c := DefaultCongestion()
	for _, tc := range []struct {
		name string
		got  float64
		why  string
	}{
		{"UCLPenalty", c.UCLPenalty,
			"minutes 0.98 against a control of players at clubs not in Europe, interval covering no effect; per-90 -0.007 once compared within bands of prior output"},
		{"UELPenalty", c.UELPenalty, "minutes 0.97 against the same control, interval covering no effect"},
		{"UECLPenalty", c.UECLPenalty, "minutes 1.05 against the same control — if anything the wrong way"},
		{"VeryShortRest", c.VeryShortRest, "minutes 0.989 within-player, interval [0.966, 1.013]"},
		{"PostBreakPenalty", c.PostBreakPenalty, "minutes 0.992 within-player, interval [0.975, 1.009]"},
		{"ShortRestPenalty", c.ShortRestPenalty,
			"minutes really do fall 4.3%, but this multiplies Score and the Score effect is positive (+2.7% points, +7.2% per 90) because who plays midweek is selected; the minutes finding belongs on the minutes channel"},
	} {
		if tc.got != 1.0 {
			t.Errorf("%s is %v, want 1.0 — %s", tc.name, tc.got, tc.why)
		}
	}
}

// TestUnsetPenaltyIsANoOp — zero means "not configured", never "score this
// player at nothing".
//
// The rest branch multiplied by the raw value while the European branch
// guarded, and the only thing hiding it was that rest days need fixture kickoff
// times which the backtest archive did not carry. Adding them turned an empty
// Congestion — which is exactly what Simulate builds every engine with — into a
// score of zero for every player at a club on a short turnaround, worth 28 and
// 113 points on two replayed seasons.
func TestUnsetPenaltyIsANoOp(t *testing.T) {
	for _, p := range []float64{0, -1, 1.5} {
		if got := usable(p); got != 1 {
			t.Errorf("usable(%v) = %v, want 1", p, got)
		}
	}
	if got := usable(0.9); got != 0.9 {
		t.Errorf("usable(0.9) = %v, want 0.9 — a real penalty must survive", got)
	}
}

// TestReliabilityIsMinutesOnly pins the mix, which was swept over four seasons.
//
// The start-share term is not merely unhelpful: it costs 180 points across the
// four, and it loses to minutes alone on three of them including the held-out
// season. See reliabilityFrom for why an expectation should not carry a
// variance term.
func TestReliabilityIsMinutesOnly(t *testing.T) {
	if reliabilityMinutesShare != 1.0 {
		t.Fatalf("reliabilityMinutesShare is %v, want 1.0", reliabilityMinutesShare)
	}
	// A player who starts every week and is always subbed at 60 must rate below
	// one who plays the full 90, which the old mix largely hid by crediting him
	// for starting.
	subbed := reliabilityFrom(blend{MinutesPerMatch: 60, StartShare: 1}, 1)
	full := reliabilityFrom(blend{MinutesPerMatch: 90, StartShare: 1}, 1)
	if !(subbed < full) {
		t.Errorf("60 minutes rates %.3f against 90 minutes at %.3f", subbed, full)
	}
	// Start share must no longer move the answer at all: it has one job now,
	// and that job is blankRate.
	a := reliabilityFrom(blend{MinutesPerMatch: 60, StartShare: 1.0}, 1.25)
	b := reliabilityFrom(blend{MinutesPerMatch: 60, StartShare: 0.2}, 1.25)
	if a != b {
		t.Errorf("start share still moves reliability: %.4f vs %.4f", a, b)
	}
}
