package backtest

import (
	"os"
	"strings"
	"testing"

	"armband/internal/analysis"
)

type holdCell struct {
	pr   seasonPair
	sc   SimConfig
	held []int
}

func newestHoldCell(t *testing.T) holdCell {
	t.Helper()
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)
	pr := pairs[len(pairs)-1] // newest season, one cell
	sc := sweepConfig(cfg, 26, false)
	held, err := openingHeld(pr.Cur, pr.Prior, sc)
	if err != nil {
		t.Fatalf("opening squad: %v", err)
	}
	return holdCell{pr: pr, sc: sc, held: held}
}

// TestHoldIgnoresWeeklyXI — HoldCaptaincyWeekly never read WeeklyXI. Flipping
// the flag must not move HOLD, or every diagnostic that sets it and then calls
// Hold() has been scoring a third thing.
func TestHoldIgnoresWeeklyXI(t *testing.T) {
	c := newestHoldCell(t)
	scOn := c.sc
	scOn.WeeklyXI = true
	off := Hold(c.pr.Cur, c.pr.Prior, c.sc, c.held)
	on := Hold(c.pr.Cur, c.pr.Prior, scOn, c.held)
	if off != on {
		t.Fatalf("Hold moved when WeeklyXI flipped: off=%d on=%d — HoldCaptaincyWeekly must not read the flag", off, on)
	}
}

// TestHoldFieldingZeroMatchesShippedHold — the zero HoldFielding is
// HoldCaptaincyWeekly, not a second loop. After the ship that is horizon 1.
func TestHoldFieldingZeroMatchesShippedHold(t *testing.T) {
	c := newestHoldCell(t)
	ship := HoldCaptaincyWeekly(c.pr.Cur, c.pr.Prior, c.sc, c.held)
	zero := HoldCaptaincyWithFielding(c.pr.Cur, c.pr.Prior, c.sc, c.held, HoldFielding{})
	if len(ship.Full) != len(zero.Full) {
		t.Fatalf("weeks %d vs %d", len(ship.Full), len(zero.Full))
	}
	for i := range ship.Full {
		if ship.Full[i] != zero.Full[i] {
			t.Fatalf("GW%d zero %d != shipped HOLD %d", ship.GW[i], zero.Full[i], ship.Full[i])
		}
		if ship.Captain[i] != zero.Captain[i] {
			t.Fatalf("GW%d captain zero %d != shipped %d", ship.GW[i], zero.Captain[i], ship.Captain[i])
		}
	}
}

// TestHoldWeeklyPickMatchesWeekEngine — HOLD's weekly XI and captain on a
// held fifteen are the same ones WeekEngine would pick from the same
// reconstruction. That is the product seam; a miss here means the replay
// HOLD metric is a third thing.
func TestHoldWeeklyPickMatchesWeekEngine(t *testing.T) {
	c := newestHoldCell(t)
	if c.sc.Weights.Horizon == 1 {
		t.Fatal("configured horizon is already 1, so this would not distinguish WeekEngine from the parent")
	}
	ship := HoldCaptaincyWeekly(c.pr.Cur, c.pr.Prior, c.sc, c.held)
	idx := c.sc.priors(c.pr.Cur, c.pr.Prior)
	matched := 0
	for i, gw := range ship.GW {
		b, fx := PointInTimeWith(c.pr.Cur, c.pr.Prior, gw-1, c.sc.Oracles)
		e := analysis.NewEngineFull(b, fx, c.sc.Weights, analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = idx
		e.Recent = c.sc.recentIndex(c.pr.Cur, gw-1)
		e.TeamForm = newTeamFormIndex(c.pr.Cur, gw-1)
		e.Tiebreak = c.sc.Tiebreak
		if e.Weights.Horizon != c.sc.Weights.Horizon {
			t.Fatalf("parent engine horizon %d, want configured %d", e.Weights.Horizon, c.sc.Weights.Horizon)
		}
		wk := e.WeekEngine()
		if wk.Weights.Horizon != 1 {
			t.Fatalf("WeekEngine horizon %d, want 1", wk.Weights.Horizon)
		}
		if !wk.FixtureLoadInScore() {
			t.Fatal("WeekEngine FixtureLoadInScore is false, so this would pin a view nothing multiplies")
		}
		xi, _, cap, _ := pickXI(wk, c.held)
		if !sameIDSet(xi, ship.XI[i]) {
			t.Fatalf("GW%d HOLD XI %v != WeekEngine %v", gw, ship.XI[i], xi)
		}
		if cap != ship.Captain[i] {
			t.Fatalf("GW%d HOLD captain %d != WeekEngine %d", gw, ship.Captain[i], cap)
		}
		matched++
	}
	if matched == 0 {
		t.Fatal("no weeks compared")
	}
}

// TestHoldWeeklyPickMovesOnFixtureLoad — confinement's liveness half. Horizon-1
// HOLD must differ from the legacy horizon-5 pick, and from horizon 1 with
// load off, or the ship never ran.
func TestHoldWeeklyPickMovesOnFixtureLoad(t *testing.T) {
	c := newestHoldCell(t)
	ship := HoldCaptaincyWeekly(c.pr.Cur, c.pr.Prior, c.sc, c.held)
	legacy := HoldCaptaincyWithFielding(c.pr.Cur, c.pr.Prior, c.sc, c.held, HoldFielding{Horizon: 5})
	noload := HoldCaptaincyWithFielding(c.pr.Cur, c.pr.Prior, c.sc, c.held, HoldFielding{Horizon: 1, DisableLoad: true})
	xi, cap, _ := fieldingMediator(c.pr.Cur, legacy, ship)
	if xi == 0 && cap == 0 {
		t.Fatal("HOLD horizon 1 is byte-identical to horizon 5 on this cell — the weekly pick is not seeing this week's fixtures")
	}
	xi2, cap2, _ := fieldingMediator(c.pr.Cur, noload, ship)
	if xi2 == 0 && cap2 == 0 {
		t.Fatal("HOLD horizon 1 with load off is byte-identical to load on — FixtureLoad is not in Score on the weekly pick")
	}
}

// TestHoldDoesNotShortenOpeningOrTransferHorizon — the wildcard trap: a
// rebuilt fifteen must not be built on the horizon-1 week engine. HOLD's
// weekly pick going to 1 must not leak into Optimize or the transfer engine.
func TestHoldDoesNotShortenOpeningOrTransferHorizon(t *testing.T) {
	c := newestHoldCell(t)
	if c.sc.Weights.Horizon != 5 {
		t.Fatalf("configured horizon %d, this test pins the shipped 5", c.sc.Weights.Horizon)
	}
	if c.sc.WeeklyXI {
		t.Fatal("newestHoldCell set WeeklyXI — POLICY fielding is a different knob")
	}
	e, _ := EngineAt(c.pr.Cur, c.pr.Prior, c.sc.startGW()-1, c.sc)
	if e.Weights.Horizon != 5 {
		t.Fatalf("opening/transfer engine at horizon %d", e.Weights.Horizon)
	}
	before := c.sc.Weights.Horizon
	_ = HoldCaptaincyWeekly(c.pr.Cur, c.pr.Prior, c.sc, c.held)
	if c.sc.Weights.Horizon != before {
		t.Fatalf("HoldCaptaincyWeekly mutated cfg.Weights.Horizon: %d -> %d", before, c.sc.Weights.Horizon)
	}

	src, err := os.ReadFile("simulate.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	// pb, pf, cfg.Weights are the transfer engine's NewEngineFull arguments.
	// Written this way so this file does not contain a second NewEngineFull
	// call-shape that TestEveryScoringEngineGetsRecency would count as unwired.
	if !strings.Contains(body, "pb, pf, cfg.Weights") {
		t.Error("the transfer engine is no longer built at cfg.Weights — a horizon-1 rebuild there is the wildcard trap")
	}
	if !strings.Contains(body, "if cfg.WeeklyXI {\n\t\t\tvw.Horizon = 1") {
		t.Error("POLICY's horizon-1 fielding is no longer gated on WeeklyXI")
	}
	if !strings.Contains(body, "horizon := 1") {
		t.Error("HOLD's shipped weekly pick is no longer the horizon := 1 default")
	}
}
