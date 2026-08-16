package analysis

import "testing"

// TestBlankRunDiscountsTheOnsetOfAnAbsence pins the shape rather than the size.
//
// The measured correction is a plateau across one to three blanks with a cliff
// either side (see blankRunFactor). Both cliffs carry a mechanism: at zero there
// is nothing to detect, and by four the exponential minutes average has caught
// up on its own, so discounting further would double-count. A regression that
// turned the plateau into a ramp, or let it run past the window, would be
// exactly the double-count the data says not to make.
func TestBlankRunDiscountsTheOnsetOfAnAbsence(t *testing.T) {
	if !blankRunAdjust {
		t.Skip("FPL_NO_BLANK_RUN is set")
	}
	e := &Engine{Weights: DefaultWeights()}
	if got := e.blankRunFactor(0); got != 1 {
		t.Errorf("a player who is playing got %.3f, want 1", got)
	}
	for run := 1; run <= blankRunMax; run++ {
		got := e.blankRunFactor(run)
		if !(got > 0 && got < 1) {
			t.Errorf("run of %d got %.3f, want a discount strictly inside (0,1)", run, got)
		}
		if got != e.blankRunFactor(1) {
			t.Errorf("run of %d got %.3f but run of 1 got %.3f — the measured "+
				"correction is flat across the window, not a ramp",
				run, got, e.blankRunFactor(1))
		}
	}
	// Past the window the recency weighting has already done the work.
	if got := e.blankRunFactor(blankRunMax + 1); got != 1 {
		t.Errorf("run of %d got %.3f, want 1 — beyond the window the exponential "+
			"average has caught up and a further discount double-counts",
			blankRunMax+1, got)
	}
	if got := e.blankRunFactor(20); got != 1 {
		t.Errorf("long-term absentee got %.3f, want 1", got)
	}
}
