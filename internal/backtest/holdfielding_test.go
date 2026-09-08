package backtest

import (
	"testing"

	"armband/internal/analysis"
)

// TestHoldIgnoresWeeklyXI — HoldCaptaincyWeekly never read WeeklyXI. Flipping
// the flag must not move HOLD, or every diagnostic that sets it and then calls
// Hold() has been scoring a third thing.
func TestHoldIgnoresWeeklyXI(t *testing.T) {
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)
	pr := pairs[len(pairs)-1] // newest season, one cell
	scOff := sweepConfig(cfg, 26, false)
	scOn := sweepConfig(cfg, 26, true)
	e, _ := EngineAt(pr.Cur, pr.Prior, scOff.startGW()-1, scOff)
	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: scOff.Budget, MinMinutes: 600,
		MinExpectedMinutes: scOff.resolvedMinExpectedMinutes(),
		BenchWeight:        scOff.openingBenchWeight(),
	})
	if err != nil {
		t.Fatalf("opening squad: %v", err)
	}
	held := make([]int, 0, 15)
	for _, p := range sq.Players {
		held = append(held, p.ID)
	}
	if len(held) != 15 {
		t.Fatalf("opening fifteen has %d", len(held))
	}
	off := Hold(pr.Cur, pr.Prior, scOff, held)
	on := Hold(pr.Cur, pr.Prior, scOn, held)
	if off != on {
		t.Fatalf("Hold moved when WeeklyXI flipped: off=%d on=%d — HoldCaptaincyWeekly must not read the flag", off, on)
	}
}

// TestHoldFieldingZeroMatchesShippedHold — the diagnostic's A0 is the zero
// HoldFielding, which must be HoldCaptaincyWeekly, not a second loop.
func TestHoldFieldingZeroMatchesShippedHold(t *testing.T) {
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)
	pr := pairs[len(pairs)-1]
	sc := sweepConfig(cfg, 26, false)
	e, _ := EngineAt(pr.Cur, pr.Prior, sc.startGW()-1, sc)
	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: sc.Budget, MinMinutes: 600,
		MinExpectedMinutes: sc.resolvedMinExpectedMinutes(),
		BenchWeight:        sc.openingBenchWeight(),
	})
	if err != nil {
		t.Fatalf("opening squad: %v", err)
	}
	held := make([]int, 0, 15)
	for _, p := range sq.Players {
		held = append(held, p.ID)
	}
	ship := HoldCaptaincyWeekly(pr.Cur, pr.Prior, sc, held)
	a0 := HoldCaptaincyWithFielding(pr.Cur, pr.Prior, sc, held, HoldFielding{})
	if len(ship.Full) != len(a0.Full) {
		t.Fatalf("weeks %d vs %d", len(ship.Full), len(a0.Full))
	}
	for i := range ship.Full {
		if ship.Full[i] != a0.Full[i] {
			t.Fatalf("GW%d A0 %d != shipped HOLD %d", ship.GW[i], a0.Full[i], ship.Full[i])
		}
		if ship.Captain[i] != a0.Captain[i] {
			t.Fatalf("GW%d captain A0 %d != shipped %d", ship.GW[i], a0.Captain[i], ship.Captain[i])
		}
	}
}
