package analysis

import "testing"

// A squad where the money for one upgrade is spread across three slots, which
// is the shape the single-downgrade paired move cannot reach.
func fundingFixture() (current []PlayerMetrics, pool []PlayerMetrics) {
	id := 0
	add := func(pos, team string, price, score float64) PlayerMetrics {
		id++
		return PlayerMetrics{ID: id, Name: pos + team, Position: pos, Team: team,
			Price: price, Score: score, Status: "available"}
	}
	// 2 GKP, 5 DEF, 5 MID, 3 FWD.
	current = []PlayerMetrics{
		add("GKP", "AAA", 5.0, 3.0), add("GKP", "BBB", 4.5, 3.3),
		add("DEF", "CCC", 6.0, 4.0), add("DEF", "DDD", 5.5, 3.9),
		add("DEF", "EEE", 5.0, 3.5), add("DEF", "FFF", 5.0, 3.4),
		add("DEF", "GGG", 4.5, 3.2),
		add("MID", "HHH", 9.0, 5.5), add("MID", "III", 8.0, 5.0),
		add("MID", "JJJ", 7.0, 4.5), add("MID", "KKK", 6.0, 4.0),
		add("MID", "LLL", 5.0, 3.5),
		add("FWD", "MMM", 9.0, 5.2), add("FWD", "NNN", 7.0, 4.4),
		// The weak link: cheap and contributing nothing.
		add("FWD", "OOO", 4.5, 0.1),
	}
	// Cheaper alternatives, each freeing 0.5, and one strong forward at +1.5.
	//
	// The three funding sources sit in different positions on purpose.
	// positionFrontier keeps only the best few scorers at each price point, so
	// stacking three £4.5m defenders here would see the third pruned before the
	// knapsack ever saw it — and the test would fail for a reason that has
	// nothing to do with what it is checking.
	pool = append(pool, current...)
	pool = append(pool,
		add("GKP", "PPP", 4.5, 2.9), // frees 0.5 from the 5.0 keeper
		add("DEF", "QQQ", 4.5, 3.3), // frees 0.5 from a 5.0 defender
		add("MID", "TTT", 4.5, 3.4), // frees 0.5 from the 5.0 midfielder
		add("FWD", "SSS", 7.0, 4.6), // the upgrade, +2.5 over OOO
	)
	return current, pool
}

// TestFundingCombosSpreadsAcrossSlots is the unit-level guard on the knapsack.
// The reconstruction is the part that breaks silently: a rolling DP array
// computes the right *value* but its parent pointers are overwritten, so
// walking them yields combinations the DP never chose. Those then fail the
// legality check and disappear, which looks exactly like "the move set did not
// help" rather than like a bug.
func TestFundingCombosSpreadsAcrossSlots(t *testing.T) {
	current, pool := fundingFixture()
	selected := map[int]PlayerMetrics{}
	clubCount := map[string]int{}
	for _, p := range current {
		selected[p.ID] = p
		clubCount[p.Team]++
	}
	cheap := cheapestByPosition(pool, 14)

	// Fund £1.5m, which no single downgrade here can cover.
	w := make([]float64, len(current))
	for i := range w {
		w[i] = 1
	}
	combos := fundingCombos(current, selected, clubCount, cheap,
		map[int]bool{}, map[int]bool{}, 15, 6, w, 0)
	if len(combos) == 0 {
		t.Fatal("no funding combination found for a £1.5m shortfall")
	}

	var sawMulti bool
	for _, combo := range combos {
		freed := 0
		seen := map[int]bool{}
		for idx, in := range combo {
			if idx < 0 || idx >= len(current) {
				t.Fatalf("combo names slot %d, outside the squad", idx)
			}
			out := current[idx]
			if in.Position != out.Position {
				t.Errorf("slot %d is %s but was filled with a %s",
					idx, out.Position, in.Position)
			}
			if in.Price >= out.Price {
				t.Errorf("slot %d 'downgrade' costs %.1f against %.1f",
					idx, in.Price, out.Price)
			}
			if seen[in.ID] {
				t.Errorf("player %d appears twice in one combination", in.ID)
			}
			seen[in.ID] = true
			freed += priceUnits(out) - priceUnits(in)
		}
		if freed < 15 {
			t.Errorf("combination frees only %d of the 15 needed", freed)
		}
		if len(combo) > 1 {
			sawMulti = true
		}
	}
	if !sawMulti {
		t.Error("every combination used a single downgrade; the multi-slot case " +
			"is the whole point of the knapsack")
	}
}

// A shortfall no combination can cover must come back empty rather than
// returning something that does not actually fund the move.
func TestFundingCombosGivesUpWhenItCannotPay(t *testing.T) {
	current, pool := fundingFixture()
	selected := map[int]PlayerMetrics{}
	clubCount := map[string]int{}
	for _, p := range current {
		selected[p.ID] = p
		clubCount[p.Team]++
	}
	cheap := cheapestByPosition(pool, 14)

	w := make([]float64, len(current))
	for i := range w {
		w[i] = 1
	}
	for _, combo := range fundingCombos(current, selected, clubCount, cheap,
		map[int]bool{}, map[int]bool{}, 500, 6, w, 0) {
		freed := 0
		for idx, in := range combo {
			freed += priceUnits(current[idx]) - priceUnits(in)
		}
		if freed < 500 {
			t.Fatalf("returned a combination freeing %d against a shortfall of 500", freed)
		}
	}
}

// Locked players must never be sold to fund something else.
func TestFundingCombosNeverSellsALockedPlayer(t *testing.T) {
	current, pool := fundingFixture()
	selected := map[int]PlayerMetrics{}
	clubCount := map[string]int{}
	for _, p := range current {
		selected[p.ID] = p
		clubCount[p.Team]++
	}
	cheap := cheapestByPosition(pool, 14)

	w := make([]float64, len(current))
	for i := range w {
		w[i] = 1
	}
	locked := map[int]bool{current[0].ID: true, current[2].ID: true}
	for _, combo := range fundingCombos(current, selected, clubCount, cheap,
		map[int]bool{}, locked, 15, 6, w, 0) {
		for idx := range combo {
			if locked[current[idx].ID] {
				t.Errorf("funding sold locked player %s", current[idx].Name)
			}
		}
	}
}

// TestFundedUpgradeBeatsSingleDowngrade is the end-to-end guard on the phase.
//
// The fixture is built so the only improving move needs two downgrades to fund
// one upgrade: the weak forward can be replaced for £1.5m, and no single slot
// frees that much. With the phase off, the local search cannot reach it.
//
// This is worth pinning because the phase failed silently three separate ways
// while being written — a broken DP reconstruction, a proxy blind to the bench,
// and candidate sets of the wrong shape — and every one of them presented
// identically, as "the optimiser is unchanged".
func TestFundedUpgradeBeatsSingleDowngrade(t *testing.T) {
	current, pool := fundingFixture()
	selected := map[int]PlayerMetrics{}
	clubCount := map[string]int{}
	spend := 0
	for _, p := range current {
		selected[p.ID] = p
		clubCount[p.Team]++
		spend += priceUnits(p)
	}
	budget := spend // no money in the bank: every upgrade must be funded

	e := &Engine{}
	frontierByPos := positionFrontier(pool, 2)
	const benchWeight = 0.15
	before := objectiveWith(current, benchWeight, nil, false)

	got, score, cost, ok := e.fundedUpgrade(current, selected, clubCount,
		spend, budget, frontierByPos, benchWeight, false, map[int]bool{}, nil, before,
		changeBudget{}, 0)
	if !ok {
		t.Fatal("no funded upgrade found; the multi-downgrade phase is inert")
	}
	if score <= before {
		t.Errorf("returned a move scoring %.4f against %.4f", score, before)
	}
	if cost > budget {
		t.Errorf("result costs %d over a budget of %d", cost, budget)
	}
	if !squadIsLegal(got, budget) {
		t.Error("returned an illegal squad")
	}

	// It must actually be the multi-slot restructure: the weak forward upgraded,
	// paid for by more than one downgrade.
	in := map[int]bool{}
	for _, p := range got {
		in[p.ID] = true
	}
	changed := 0
	for _, p := range current {
		if !in[p.ID] {
			changed++
		}
	}
	if changed < 3 {
		t.Errorf("only %d players changed; this should be one upgrade funded by "+
			"at least two downgrades", changed)
	}
	t.Logf("objective %.4f -> %.4f, %d players changed", before, score, changed)
}
