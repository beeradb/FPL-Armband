package backtest

import (
	"testing"

	"armband/internal/analysis"
)

// TestFullAnchoredPlanFillsBothSets pins the functional plan's central claim:
// EVERY chip the season grants is placed, in the set that grants it, even when
// a set's window holds no qualifying week. 2025-26 is the hard case: its first
// half holds no doubles or blanks at all, so the first set must land entirely
// on fallbacks, and nothing may collide within a set.
func TestFullAnchoredPlanFillsBothSets(t *testing.T) {
	cfg := loadConfig(t)
	cur := loadSeason(t, cfg, "2025-26")
	sch := FullAnchoredPlan(cur, 1)

	check := func(p analysis.ChipPlan, lo, hi int, label string) {
		weeks := []int{p.Wildcard, p.BenchBoost, p.FreeHit, p.TripleCaptain}
		seen := map[int]bool{}
		for _, gw := range weeks {
			if gw < lo || gw > hi {
				t.Fatalf("%s: chip at GW%d outside [%d, %d]: %+v", label, gw, lo, hi, p)
			}
			if seen[gw] {
				t.Fatalf("%s: two chips on GW%d: %+v", label, gw, p)
			}
			seen[gw] = true
		}
	}
	check(sch.First, 2, ChipResetGW-1, "first set")
	check(sch.Second, ChipResetGW, 38, "second set")
	if err := ValidateChipSets("2025-26", sch.First, sch.Second); err != nil {
		t.Fatalf("the full plan does not validate: %v", err)
	}
	t.Logf("2025-26 first:  %+v", sch.First)
	t.Logf("2025-26 second: %+v", sch.Second)
}

// TestFullAnchoredPlanOneSetSeason pins the one-set shape: four chips, one
// plan, nothing in the second set, and the whole thing validates.
func TestFullAnchoredPlanOneSetSeason(t *testing.T) {
	cfg := loadConfig(t)
	cur := loadSeason(t, cfg, "2024-25")
	sch := FullAnchoredPlan(cur, 1)
	if sch.Second != (analysis.ChipPlan{}) {
		t.Fatalf("a one-set season got a second-set plan: %+v", sch.Second)
	}
	for _, gw := range []int{sch.First.Wildcard, sch.First.BenchBoost,
		sch.First.FreeHit, sch.First.TripleCaptain} {
		if gw < 2 || gw > 38 {
			t.Fatalf("chip at GW%d outside [2, 38]: %+v", gw, sch.First)
		}
	}
	if err := ValidateChipSets("2024-25", sch.First, sch.Second); err != nil {
		t.Fatalf("the full plan does not validate: %v", err)
	}
	t.Logf("2024-25 first: %+v", sch.First)
}

// TestTheFreeHitRecordsItsBorrowedSquad pins the fix for the free-hit week's
// page: the borrowed fifteen is recorded on the week, every one of its clubs
// has a fixture that week, and the permanent squad is untouched.
func TestTheFreeHitRecordsItsBorrowedSquad(t *testing.T) {
	cfg := loadConfig(t)
	cur := loadSeason(t, cfg, "2025-26")
	pri := loadSeason(t, cfg, "2024-25")
	sc := SimConfig{
		Weights: cfg.Weights, MinGain: 0.4, MinGainHit: 3.0,
		BankUpTo: 5, FreeCost: 2.0, Budget: 1000, WeeklyXI: true,
		Chips2: analysis.ChipPlan{BenchBoost: 33, FreeHit: 34},
	}
	res, err := Simulate(cur, pri, sc)
	if err != nil {
		t.Fatal(err)
	}
	clubFix := map[int]map[int]int{}
	for _, f := range cur.Fixtures {
		if f.Event == nil {
			continue
		}
		if clubFix[*f.Event] == nil {
			clubFix[*f.Event] = map[int]int{}
		}
		clubFix[*f.Event][f.TeamH]++
		clubFix[*f.Event][f.TeamA]++
	}
	var fhWeek *Week
	for i := range res.Weeks {
		if res.Weeks[i].FreeHit {
			fhWeek = &res.Weeks[i]
		}
	}
	if fhWeek == nil {
		t.Fatal("no free-hit week played")
	}
	if len(fhWeek.FreeHitSquad) != 15 {
		t.Fatalf("the free-hit week recorded %d borrowed players, want 15",
			len(fhWeek.FreeHitSquad))
	}
	for _, id := range fhWeek.FreeHitSquad {
		p := cur.Players[id]
		if p == nil {
			t.Fatalf("borrowed player %d not in the season", id)
		}
		if clubFix[fhWeek.GW][p.Team] == 0 {
			t.Fatalf("borrowed player %s (team %d) has no fixture in the free-hit week GW%d",
				p.WebName, p.Team, fhWeek.GW)
		}
	}
	t.Logf("free-hit week GW%d: 15 borrowed players, all with fixtures", fhWeek.GW)
}
