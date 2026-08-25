package agent

import (
	"strings"
	"testing"
)

// TestOptimizeSpendsTheRealBudget — omitting budget_m must mean the money that
// exists, not £100m.
//
// £100m was hardcoded as the default and is only right in August. In-season the
// question "what is the best fifteen available" is asked with this squad's
// selling value plus the bank, and a squad built at £100m is either poorer than
// the manager really is or, worse, unaffordable. Neither shows up in the output:
// the squad is legal, the arithmetic adds up, and only the deadline disagrees.
func TestOptimizeSpendsTheRealBudget(t *testing.T) {
	tb := testToolbox(t)
	// Early in a live season most players score 0 — testToolbox does not load a
	// prior season the way cmd/armband's live server does — so the optimiser has
	// nothing worth spending the extra budget on and underspends.
	//
	// ⚠️ This skipped only while `GameweeksPlayed() == 0`, i.e. mid-GW1, and the
	// reasoning above already said why that is the wrong boundary: the problem is
	// that most players score 0, and one COMPLETED gameweek does not fix that.
	// Measured 2026-08-25, one gameweek in — 307 of 612 players carried a
	// non-zero score, and this failed at £92.5m of a £105.4m budget.
	//
	// Two mirrors `corroboratingMatches` in internal/analysis, that package's own
	// bar for "this player's minutes are trustworthy". It is unexported, hence
	// the literal.
	const matchesNeeded = 2
	if played := tb.Engine.GameweeksPlayed(); tb.Engine.SeasonHasStarted() && played < matchesNeeded {
		t.Skipf("live season has played %d gameweek(s); this test needs %d before "+
			"enough of the league carries a score for the optimiser to have "+
			"anything to spend a budget on. Not a defect.", played, matchesNeeded)
	}

	// A tracked in-season squad that could not be priced must refuse to build.
	// The season state is forced so this runs in August too — a check that
	// quietly stops applying for most of the year is not a check — and put back
	// afterwards, because the gameweek count also drives the data window every
	// score below is computed from.
	func() {
		finished := make([]bool, len(tb.Engine.Boot.Events))
		for i := range tb.Engine.Boot.Events {
			finished[i] = tb.Engine.Boot.Events[i].Finished
			tb.Engine.Boot.Events[i].Finished = i < 3
		}
		defer func() {
			for i := range tb.Engine.Boot.Events {
				tb.Engine.Boot.Events[i].Finished = finished[i]
			}
		}()
		tb.Engine.Entry, tb.Engine.SquadValue, tb.Engine.Bank = 12345, nil, nil
		if _, err := tb.optimizeSquadFor(optimizeSquadInput{}); err == nil {
			t.Error("built a squad for an entry whose budget could not be established")
		}
	}()

	// Priced: value plus bank, and nothing near £100m, so a regression to the
	// constant is visible rather than plausible.
	value, bank := 1041, 13
	tb.Engine.Entry, tb.Engine.SquadValue, tb.Engine.Bank = 12345, &value, &bank
	out, err := tb.optimizeSquadFor(optimizeSquadInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got := out["budget_m"]; got != 105.4 {
		t.Errorf("budget_m is %v, want 105.4 — £104.1m of squad plus £1.3m banked", got)
	}
	if src, _ := out["budget_source"].(string); !strings.Contains(src, "bank") {
		t.Errorf("budget_source is %q and does not say where the money came from", src)
	}
	cost, ok := out["total_cost_m"].(float64)
	if !ok {
		t.Fatalf("total_cost_m is %v", out["total_cost_m"])
	}
	if cost > 105.4 {
		t.Errorf("squad costs £%.1fm against a £105.4m budget", cost)
	}
	// The point of the larger budget is that it gets used. A squad that still
	// costs £100m or less is one the old constant would have built.
	if cost <= 100.0 {
		t.Errorf("squad costs £%.1fm with £105.4m available; the extra money is not "+
			"reaching the optimiser", cost)
	}

	// An explicit value is a what-if and outranks the real budget.
	ninety := 90.0
	out, err = tb.optimizeSquadFor(optimizeSquadInput{Budget: &ninety})
	if err != nil {
		t.Fatal(err)
	}
	if got := out["budget_m"]; got != 90.0 {
		t.Errorf("budget_m is %v when the caller asked for £90.0m", got)
	}
	if cost, _ := out["total_cost_m"].(float64); cost > 90.0 {
		t.Errorf("squad costs £%.1fm against the £90.0m asked for", cost)
	}
}
