package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// penaltyOrder returns a player's set-piece penalty rank, 0 for no duty.
func penaltyOrder(el *fpl.Element) int {
	if el.PenaltiesOrder == nil {
		return 0
	}
	return *el.PenaltiesOrder
}

// calibrationByPenaltyDuty measures, per appearance, how far predictions sit
// above actual returns for first-choice takers versus players with no duty.
//
// Restricted to midfielders and forwards who start and finish, where
// appearances equal starts and points-per-start is therefore clean.
func calibrationByPenaltyDuty(t *testing.T, e *Engine) (taker, none float64, n int) {
	t.Helper()
	var tSum, nSum float64
	var tN, nN int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		m := e.Metrics(el)
		if m.Position != "MID" && m.Position != "FWD" {
			continue
		}
		if m.Starts < 20 || m.Minutes == 0 {
			continue
		}
		perApp := float64(m.Minutes) / float64(m.Starts)
		if perApp < 85 || perApp > 92 {
			continue
		}
		predicted := 2 + (m.BaseXP90+m.SetPieceXP90-2)*(perApp/90)
		actual := float64(m.TotalPoints) / float64(m.Starts)
		switch penaltyOrder(el) {
		case 1:
			tSum += predicted - actual
			tN++
		case 0:
			nSum += predicted - actual
			nN++
		}
	}
	if tN < 4 || nN < 10 {
		t.Skip("not enough start-and-finish players to measure penalty calibration")
	}
	return tSum / float64(tN), nSum / float64(nN), tN + nN
}

// TestSetPieceBonusDoubleCountsPenalties is the evidence for SetPieceWeight
// defaulting to 0, and the guard against someone restoring it.
//
// FPL's expected_goals already contains penalties. Crediting a first-choice
// taker again for holding the duty counts the same spot kicks twice, and it
// shows up as a systematic over-prediction of takers relative to everyone else.
// The test asserts the gap is small at the default and demonstrably large when
// the bonus is switched back on.
func TestSetPieceBonusDoubleCountsPenalties(t *testing.T) {
	off := DefaultWeights()
	if off.SetPieceWeight != 0 {
		t.Fatalf("SetPieceWeight defaults to %v; this test documents why it must be 0", off.SetPieceWeight)
	}
	takerOff, noneOff, n := calibrationByPenaltyDuty(t, roleEngine(t, off, DefaultRoleRisk()))
	gapOff := takerOff - noneOff

	on := DefaultWeights()
	on.SetPieceWeight = 1.0
	takerOn, noneOn, _ := calibrationByPenaltyDuty(t, roleEngine(t, on, DefaultRoleRisk()))
	gapOn := takerOn - noneOn

	t.Logf("n=%d", n)
	t.Logf("  weight 0 : #1 takers %+.3f, no duty %+.3f, gap %.3f", takerOff, noneOff, gapOff)
	t.Logf("  weight 1 : #1 takers %+.3f, no duty %+.3f, gap %.3f", takerOn, noneOn, gapOn)

	if gapOn <= gapOff {
		t.Errorf("restoring the set-piece bonus did not widen the taker bias (%.3f -> %.3f); "+
			"either the double-count is gone or the measurement is broken", gapOff, gapOn)
	}
	if gapOff > 0.30 {
		t.Errorf("penalty takers are still over-predicted by %.3f per appearance relative to "+
			"players with no duty, even with the bonus off", gapOff)
	}
}

// TestSetPieceNotesSurviveAZeroWeight — zeroing the score contribution must not
// hide the information. The agent still needs to see who takes penalties so it
// can reason about a newly appointed taker, which the model no longer prices.
func TestSetPieceNotesSurviveAZeroWeight(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var noted, scored int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if penaltyOrder(el) != 1 || el.Minutes < 900 {
			continue
		}
		m := e.Metrics(el)
		if m.SetPieceNote != "" {
			noted++
		}
		if m.SetPieceXP90 != 0 {
			scored++
		}
	}
	if noted == 0 {
		t.Error("no first-choice penalty taker carries a set-piece note; the duty is now invisible")
	}
	if scored != 0 {
		t.Errorf("%d players still score from the set-piece term at weight 0", scored)
	}
	t.Logf("%d first-choice takers flagged, %d scored", noted, scored)
}
