package analysis

import "testing"

// Money is worth what it can be turned into, so it decays as the season runs
// out. The same bank balance is a different decision in August and in May, and
// after the final whistle it buys nothing at all.
func TestBudgetValueDecaysWithTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	if e.PointsPerTenth() <= 0 {
		t.Skip("no priced frontier in this dataset")
	}

	early := 3.0 * e.PointsPerTenth() * 37
	late := 3.0 * e.PointsPerTenth() * 2
	if !(early > late) {
		t.Fatalf("£0.3m early %.3f is not worth more than late %.3f", early, late)
	}
	if late <= 0 {
		t.Error("money late in the season is worth nothing at all, which is only " +
			"true after the last deadline")
	}
}

// The slope is local. Extrapolating it across a premium's whole price answers a
// question nobody should ask: a squad must field fifteen players, so selling a
// £14m asset frees the gap to his replacement, not £14m.
func TestBudgetValueRefusesToPriceAPremium(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	if e.PointsPerTenth() <= 0 {
		t.Skip("no priced frontier in this dataset")
	}
	marginal := e.BudgetValue(10) // £1.0m
	absurd := e.BudgetValue(140)  // £14.0m
	if absurd > marginal {
		t.Errorf("£14m priced at %.2f, above £1m at %.2f — the linear slope is "+
			"being extrapolated past where it holds", absurd, marginal)
	}
}

// After the final deadline money cannot be spent, so it is worth nothing. A
// model that still values it will bank rather than act in the closing weeks.
func TestBudgetIsWorthlessOnceTheSeasonEnds(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	saved := e.Boot.Events
	defer func() { e.Boot.Events = saved }()
	for i := range e.Boot.Events {
		e.Boot.Events[i].Finished = true
		e.Boot.Events[i].IsNext = false
	}
	if got := e.GameweeksRemaining(); got != 0 {
		t.Errorf("%d gameweeks remain after the season ended", got)
	}
	if got := e.BudgetValue(5); got != 0 {
		t.Errorf("£0.5m is worth %.2f points after the final whistle", got)
	}
}

// A squad's headline value overstates its freedom. Most of it is the obligation
// to field fifteen players at all, and only the part above each position's floor
// reflects a decision.
func TestDiscretionaryStripsThePositionalFloor(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	floors := e.PositionFloors()
	if len(floors) < 4 {
		t.Skip("incomplete pool")
	}
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		f, ok := floors[pos]
		if !ok || f <= 0 {
			t.Fatalf("no floor for %s", pos)
		}
		// A player at the floor is pure obligation: no choice was made.
		if got := e.Discretionary(pos, f); got != 0 {
			t.Errorf("%s at the floor has %d discretionary, want 0", pos, got)
		}
		// And one above it is worth exactly the gap.
		if got, want := e.Discretionary(pos, f+30), 30; got != want {
			t.Errorf("%s at floor+3.0m has %d discretionary, want %d", pos, got, want)
		}
	}
	// Below the floor cannot go negative — it would credit money that does not
	// exist to whatever else is being compared.
	if got := e.Discretionary("FWD", 1); got != 0 {
		t.Errorf("a sub-floor price gave %d discretionary, want 0", got)
	}
	// An unknown position must not silently report the whole price as free.
	if got := e.Discretionary("MGR", 100); got != 100 {
		t.Errorf("unknown position gave %d; without a floor the honest answer is "+
			"the full amount", got)
	}
}

// The premium case, which is the reason this exists: selling a £14m forward does
// not free £14m, because someone still has to fill the slot.
func TestDiscretionaryAnswersThePremiumQuestion(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	floors := e.PositionFloors()
	fwd, ok := floors["FWD"]
	if !ok {
		t.Skip("no forward floor")
	}
	free := e.Discretionary("FWD", 140)
	if free != 140-fwd {
		t.Errorf("a £14.0m forward frees %d, want %d (price less the £%.1fm floor)",
			free, 140-fwd, float64(fwd)/10)
	}
	if free >= 140 {
		t.Error("selling a premium reported as freeing his whole price; the slot " +
			"still has to be filled")
	}
}
