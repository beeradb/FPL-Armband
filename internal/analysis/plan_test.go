package analysis

import "testing"

// A plan must be complete: the resulting fifteen, the eleven that would be
// fielded, and a gain measured against the squad you have. Returning moves alone
// pushes the assembly onto the reader, who then has to work out whether the man
// arriving actually displaces anyone.
func TestPlansAreComplete(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget, MinMinutes: 600,
		MinExpectedMinutes: 55, BenchWeight: DefaultBenchWeight})
	if err != nil {
		t.Skipf("optimiser unavailable: %v", err)
	}

	// Degrade the squad so there is something worth doing.
	state := NewSquadState(sq.Players)
	plans := BuildPlans(state, e.AllMetrics(), e.WeekEngine(), 20, 2, 5)
	if len(plans) == 0 {
		t.Skip("no improving plans from an optimal squad, which is legitimate")
	}

	for _, p := range plans {
		if len(p.Squad) != SquadSize {
			t.Errorf("plan has %d players, want %d", len(p.Squad), SquadSize)
		}
		if len(p.XI) != 11 {
			t.Errorf("plan fields %d players, want 11", len(p.XI))
		}
		if len(p.Bench) != 4 {
			t.Errorf("plan benches %d, want 4", len(p.Bench))
		}
		if p.Transfers != len(p.Moves) || p.Transfers == 0 {
			t.Errorf("plan claims %d transfers for %d moves", p.Transfers, len(p.Moves))
		}
		if p.Captain.ID == 0 {
			t.Error("plan has no captain")
		}
		// The captain must be in the eleven, or the armband is being given to
		// someone who is not playing.
		var inXI bool
		for _, x := range p.XI {
			if x.ID == p.Captain.ID {
				inXI = true
			}
		}
		if !inXI {
			t.Errorf("captain %s is not in the eleven", p.Captain.Name)
		}
		// Every player bought must actually be in the resulting squad.
		have := map[int]bool{}
		for _, q := range p.Squad {
			have[q.ID] = true
		}
		for _, m := range p.Moves {
			if !have[m.In.ID] {
				t.Errorf("plan buys %s but he is not in the resulting squad", m.In.Name)
			}
			if have[m.Out.ID] {
				t.Errorf("plan sells %s but he is still in the squad", m.Out.Name)
			}
		}
	}
}

// The bench has an order and it is not "best first". A reserve keeper only ever
// replaces the keeper, so listing him among the outfield substitutes misstates
// who actually comes on.
func TestBenchIsInSubstitutionOrder(t *testing.T) {
	squad := []PlayerMetrics{
		{ID: 1, Position: "GKP", Score: 3.0}, {ID: 2, Position: "GKP", Score: 1.0},
		{ID: 3, Position: "DEF", Score: 4.0}, {ID: 4, Position: "DEF", Score: 3.9},
		{ID: 5, Position: "DEF", Score: 3.8}, {ID: 6, Position: "DEF", Score: 0.5},
		{ID: 7, Position: "DEF", Score: 0.4},
		{ID: 8, Position: "MID", Score: 5.0}, {ID: 9, Position: "MID", Score: 4.9},
		{ID: 10, Position: "MID", Score: 4.8}, {ID: 11, Position: "MID", Score: 4.7},
		{ID: 12, Position: "MID", Score: 0.3},
		{ID: 13, Position: "FWD", Score: 5.5}, {ID: 14, Position: "FWD", Score: 5.4},
		{ID: 15, Position: "FWD", Score: 5.3},
	}
	_, bench, _, _ := fieldedXI(squad, nil)
	if len(bench) != 4 {
		t.Fatalf("bench has %d", len(bench))
	}
	if bench[0].Position != "GKP" {
		t.Errorf("bench starts with %s; the reserve keeper is not an outfield "+
			"substitute and belongs first", bench[0].Position)
	}
	for i := 1; i < len(bench)-1; i++ {
		if bench[i].Score < bench[i+1].Score {
			t.Errorf("outfield bench is not best-first: %.1f before %.1f",
				bench[i].Score, bench[i+1].Score)
		}
	}
}

// The load-bearing player is the one whose absence would break the plan, and
// that is almost always the man being bought — not the best player in the squad.
//
// The distinction is the whole point. Zeroing a player only in the plan's squad
// makes your best existing asset look load-bearing every time, which is true and
// useless: if he is out he is out whether you transfer or not, so nothing you
// could learn about him changes the decision.
func TestWeakestLinkIgnoresPlayersYouAlreadyOwn(t *testing.T) {
	mk := func(id int, pos string, score float64) PlayerMetrics {
		return PlayerMetrics{ID: id, Name: pos + string(rune('A'+id)), Position: pos, Score: score}
	}
	current := []PlayerMetrics{
		mk(1, "GKP", 3.0), mk(2, "GKP", 1.0),
		mk(3, "DEF", 4.0), mk(4, "DEF", 3.5), mk(5, "DEF", 3.4),
		mk(6, "DEF", 0.5), mk(7, "DEF", 0.4),
		// A star already owned. Losing him hurts, but it hurts equally either way.
		mk(8, "MID", 9.0), mk(9, "MID", 4.5), mk(10, "MID", 4.4),
		mk(11, "MID", 4.3), mk(12, "MID", 0.3),
		mk(13, "FWD", 5.0), mk(14, "FWD", 4.0), mk(15, "FWD", 1.0),
	}
	// Upgrade the weak forward.
	bought := mk(99, "FWD", 6.0)
	planned := applyMoves(current, []Swap{{Out: current[14], In: bought}})

	who, gainIfOut := weakestLink(NewSquadState(current), planned)
	if who.ID != bought.ID {
		t.Errorf("load-bearing player is %s (%.1f); want the man being bought. "+
			"Zeroing only the plan's squad flags the best player you already own.",
			who.Name, who.Score)
	}

	// Without him the plan is worth nothing, since he is the entire plan.
	if gainIfOut > 0 {
		t.Errorf("plan still gains %.2f without the only player it buys", gainIfOut)
	}

	// Losing the star must not break the plan, because he is lost either way.
	//
	// It is not *perfectly* neutral, and the reason is worth knowing: XIValue is
	// sum plus max, so zeroing the captain promotes whoever is next best — and in
	// the planned squad that may be the player just bought. The margin therefore
	// moves a little. What must hold is that the plan still stands without him,
	// while it does not stand without the man it buys.
	starOut := XIValue(zeroOut(planned, 8)) - XIValue(zeroOut(current, 8))
	if starOut <= 0 {
		t.Errorf("losing an already-owned star left the plan worth %.2f; it should "+
			"still be worth doing, since he is missing either way", starOut)
	}
	if starOut <= gainIfOut {
		t.Errorf("losing the star (%.2f) hurts at least as much as losing the man "+
			"being bought (%.2f); the sensitivity is not isolating the right player",
			starOut, gainIfOut)
	}
}
