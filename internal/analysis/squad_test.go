package analysis

import (
	"context"
	"testing"
	"time"

	"armband/internal/fpl"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	ctx := context.Background()
	boot, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	fx, err := c.Fixtures(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	// No skipDuringLiveGW1Gap here on purpose: some callers (datawindow_test.go)
	// immediately call playGameweeks to fully override the live state, and a
	// skip here would abort before that override ever ran. Callers that do NOT
	// override the state call it themselves — see this function's callers.
	return NewEngine(boot, fx, DefaultWeights())
}

func TestOptimizeProducesLegalSquad(t *testing.T) {
	e := testEngine(t)
	skipDuringLiveGW1Gap(t, e)

	sq, err := e.Optimize(OptimizeRequest{MinMinutes: 500})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	if len(sq.Players) != SquadSize {
		t.Errorf("squad size = %d, want %d", len(sq.Players), SquadSize)
	}
	if len(sq.StartingXI) != 11 {
		t.Errorf("XI size = %d, want 11", len(sq.StartingXI))
	}
	if len(sq.Bench) != 4 {
		t.Errorf("bench size = %d, want 4", len(sq.Bench))
	}

	pos := map[string]int{}
	for _, p := range sq.Players {
		pos[p.Position]++
	}
	for want, n := range squadQuota {
		if pos[want] != n {
			t.Errorf("position %s = %d, want %d", want, pos[want], n)
		}
	}

	for club, n := range sq.ClubCounts {
		if n > MaxPerClub {
			t.Errorf("club %s has %d players, limit is %d", club, n, MaxPerClub)
		}
	}

	if sq.TotalCost > 100.0 {
		t.Errorf("total cost £%.1fm exceeds £100.0m budget", sq.TotalCost)
	}
	if sq.Remaining < 0 {
		t.Errorf("negative remaining budget: %.1f", sq.Remaining)
	}

	// Every starter should be someone we'd actually field.
	for _, p := range sq.StartingXI {
		if p.Score <= 0 {
			t.Errorf("starter %s has non-positive score %.2f", p.Name, p.Score)
		}
	}

	t.Logf("formation %s, cost £%.1fm, XI score %.1f", sq.Formation, sq.TotalCost, sq.XIScore)
	for _, p := range sq.StartingXI {
		t.Logf("  XI  %-3s %-16s %-4s £%.1fm  score %.2f  fdr %.1f", p.Position, p.Name, p.Team, p.Price, p.Score, p.AvgDifficulty)
	}
	for _, p := range sq.Bench {
		t.Logf("  SUB %-3s %-16s %-4s £%.1fm  score %.2f", p.Position, p.Name, p.Team, p.Price, p.Score)
	}
}

func TestOptimizeRespectsLocksAndExclusions(t *testing.T) {
	e := testEngine(t)

	base, err := e.Optimize(OptimizeRequest{MinMinutes: 500})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Exclude the top scorer and lock a player who was not selected.
	excluded := base.StartingXI[0].ID
	var lockID int
	inBase := map[int]bool{}
	for _, p := range base.Players {
		inBase[p.ID] = true
	}
	for _, m := range e.AllMetrics() {
		if !inBase[m.ID] && m.Position == "MID" && m.Minutes > 1500 && m.Price < 7.0 {
			lockID = m.ID
			break
		}
	}
	if lockID == 0 {
		t.Skip("no suitable lock candidate found")
	}

	sq, err := e.Optimize(OptimizeRequest{
		MinMinutes: 500,
		LockIDs:    []int{lockID},
		ExcludeIDs: []int{excluded},
	})
	if err != nil {
		t.Fatalf("Optimize with constraints: %v", err)
	}

	found := false
	for _, p := range sq.Players {
		if p.ID == excluded {
			t.Errorf("excluded player %d appears in squad", excluded)
		}
		if p.ID == lockID {
			found = true
		}
	}
	if !found {
		t.Errorf("locked player %d missing from squad", lockID)
	}
	if sq.TotalCost > 100.0 {
		t.Errorf("total cost £%.1fm exceeds budget", sq.TotalCost)
	}
}

func TestBestXIFormationIsLegal(t *testing.T) {
	e := testEngine(t)
	sq, err := e.Optimize(OptimizeRequest{MinMinutes: 500})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	pos := map[string]int{}
	for _, p := range sq.StartingXI {
		pos[p.Position]++
	}
	if pos["GKP"] != 1 {
		t.Errorf("XI has %d keepers, want 1", pos["GKP"])
	}
	for _, k := range []string{"DEF", "MID", "FWD"} {
		if pos[k] < xiMin[k] || pos[k] > xiMax[k] {
			t.Errorf("XI has %d %s, want %d-%d", pos[k], k, xiMin[k], xiMax[k])
		}
	}
}

// TestMinutesReliabilityTracksExpectedMinutes guards against the regression
// where minutes reliability was derived from FPL's starts_per_90 field. That
// field measures "when this player appears, does he start", which is ~1.0 for
// nearly every player, so a 25-minute-per-week rotation option scored the same
// as an ever-present.
func TestMinutesReliabilityTracksExpectedMinutes(t *testing.T) {
	e := testEngine(t)

	var nailed, fringe []PlayerMetrics
	for _, m := range e.AllMetrics() {
		switch {
		case m.ExpectedMinutes >= 78:
			nailed = append(nailed, m)
		case m.ExpectedMinutes > 0 && m.ExpectedMinutes <= 30:
			fringe = append(fringe, m)
		}
	}
	if len(nailed) == 0 || len(fringe) == 0 {
		t.Skip("dataset lacks both nailed and fringe players")
	}

	var worstNailed = 1.0
	for _, m := range nailed {
		if m.MinutesRating < worstNailed {
			worstNailed = m.MinutesRating
		}
	}
	var bestFringe float64
	for _, m := range fringe {
		if m.MinutesRating > bestFringe {
			bestFringe = m.MinutesRating
		}
	}

	if bestFringe >= worstNailed {
		t.Errorf("fringe players rate as high as nailed ones: best fringe %.3f >= worst nailed %.3f",
			bestFringe, worstNailed)
	}
	if bestFringe > 0.5 {
		t.Errorf("a player averaging <=30 min/gw rated %.3f; rotation risk is not being penalised", bestFringe)
	}
	t.Logf("worst nailed %.3f, best fringe %.3f", worstNailed, bestFringe)
}

func TestOptimizeRespectsExpectedMinutesFloor(t *testing.T) {
	e := testEngine(t)
	// ⚠️ Two, not the GW1 gap alone. skipDuringLiveGW1Gap closes only the window
	// where some clubs have played and some have not; it says nothing about how
	// much evidence a minutes floor needs to be satisfiable. With ONE gameweek
	// played the league's expected-minutes estimates are drawn from a single
	// match, and the optimiser can legitimately find no eleven that all clear 60
	// — the assertion then fails on a data state rather than on a defect.
	//
	// Observed 2026-08-30, one gameweek in and GW2 stuck unfinished for two days:
	// this passed at 08:17 and failed at 10:05 on unchanged code, which is the
	// signature of a live-data coupling rather than a regression. Two mirrors
	// corroboratingMatches, this package's own bar for trustworthy minutes, and
	// the same literal internal/agent's optimize_test.go already uses.
	skipUntilLiveEvidence(t, e, 2)
	sq, err := e.Optimize(OptimizeRequest{MinExpectedMinutes: 60, BenchWeight: 0.02})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	for _, p := range sq.StartingXI {
		// Cheap fodder is exempt from the floor, but must not reach the XI.
		if p.ExpectedMinutes < 60 {
			t.Errorf("XI contains %s at %.1f expected minutes, below the 60 floor",
				p.Name, p.ExpectedMinutes)
		}
	}
}
