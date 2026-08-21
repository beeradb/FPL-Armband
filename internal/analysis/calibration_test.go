package analysis

import (
	"testing"
)

func TestCalibrationRatioGuardsThinSamples(t *testing.T) {
	// Goalkeepers score about 11 goals a season from ~0.2 expected. Trusting
	// that ratio would price every keeper as a striker.
	if got := CalibrationRatio(11, 0.2); got != 1 {
		t.Errorf("a sample of 0.2 expected events produced a ratio of %v, want the neutral 1", got)
	}
	if got := CalibrationRatio(0, 0); got != 1 {
		t.Errorf("empty sample produced %v, want 1", got)
	}
	// Real samples pass through, clamped.
	if got := CalibrationRatio(786, 572.4); got < 1.3 || got > 1.4 {
		t.Errorf("league assists/xA = %v, expected ~1.37", got)
	}
	if got := CalibrationRatio(1000, 100); got != 3.0 {
		t.Errorf("an absurd ratio was not clamped: %v", got)
	}
	if got := CalibrationRatio(1, 100); got != 0.5 {
		t.Errorf("an absurd ratio was not clamped: %v", got)
	}
}

// TestExpectedStatsAreCalibratedPerPosition checks the live ratios come out in
// the range the data supports, and — the point of doing it per position — that
// forwards are not taxed by the defenders' poor xG conversion.
func TestExpectedStatsAreCalibratedPerPosition(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	skipDuringLiveGW1Gap(t, e)
	if len(e.xScale) == 0 {
		t.Fatal("no calibration was computed")
	}

	for pos := 1; pos <= 4; pos++ {
		s := e.scaleFor(pos)
		if s.Goals <= 0 || s.Assists <= 0 {
			t.Errorf("position %d has a non-positive scale: %+v", pos, s)
		}
		if s.Goals < 0.5 || s.Goals > 3.0 || s.Assists < 0.5 || s.Assists > 3.0 {
			t.Errorf("position %d scale outside the clamp: %+v", pos, s)
		}
	}

	// An FPL assist is a broader event than xA models — it pays for winning a
	// penalty, for a parried shot turned in, for deflections. So outfield
	// players must convert xA at better than parity.
	for _, pos := range []int{2, 3, 4} {
		if s := e.scaleFor(pos); s.Assists <= 1.0 {
			t.Errorf("position %d converts xA at %.3f; FPL's assist is strictly broader than xA",
				pos, s.Assists)
		}
	}

	// xG and an FPL goal are the same event, so midfielders and forwards should
	// sit near parity even though defenders do not.
	for _, pos := range []int{3, 4} {
		if s := e.scaleFor(pos); s.Goals < 0.85 || s.Goals > 1.15 {
			t.Errorf("position %d converts xG at %.3f, expected near parity", pos, s.Goals)
		}
	}
}

// TestUnknownPositionIsNeutral guards the fallback: a position the bootstrap did
// not cover must not silently zero out a player's attacking return.
func TestUnknownPositionIsNeutral(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	if s := e.scaleFor(99); s.Goals != 1 || s.Assists != 1 {
		t.Errorf("unknown position returned %+v, want neutral 1/1", s)
	}
}

// TestCalibrationIsReportedOnPlayers keeps the term explainable: the agent must
// be able to say why a number differs from FPL's raw xG or xA.
func TestCalibrationIsReportedOnPlayers(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var checked int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Minutes < 900 {
			continue
		}
		m := e.Metrics(el)
		want := e.scaleFor(el.ElementType)
		if m.XGScale != want.Goals || m.XAScale != want.Assists {
			t.Fatalf("%s reports %.3f/%.3f, engine holds %.3f/%.3f",
				el.WebName, m.XGScale, m.XAScale, want.Goals, want.Assists)
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no players with enough minutes")
	}
}
