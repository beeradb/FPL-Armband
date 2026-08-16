package analysis

import "testing"

// TestSellPriceHalvesTheRise pins FPL's selling rule, which is the whole reason
// SquadState carries Sell at all: a rise is shared with FPL and a fall is not.
// Pricing sales at the market value hands the policy budget it does not have,
// and the error grows all season as prices drift.
func TestSellPriceHalvesTheRise(t *testing.T) {
	squad := []PlayerMetrics{
		{ID: 1, Price: 5.6, Position: "MID", Team: "AAA"},
		{ID: 2, Price: 4.4, Position: "DEF", Team: "BBB"},
		{ID: 3, Price: 7.0, Position: "FWD", Team: "CCC"},
	}
	s := NewSquadState(squad)

	// Nothing tracked: sell at market. That is right pre-season.
	for _, p := range squad {
		if got, want := s.sellPrice(p), tenths(p.Price); got != want {
			t.Errorf("untracked %d sells for %d, want market %d", p.ID, got, want)
		}
	}

	// Bought at 5.0, now 5.6: the 0.6 rise is halved to 0.3, so 5.3.
	// Bought at 5.0, now 4.4: the fall is taken in full, so 4.4.
	// Bought at 7.0, now 7.0: unchanged.
	s.Sell = map[int]int{1: 53, 2: 44, 3: 70}
	for _, tc := range []struct {
		id, want int
		why      string
	}{
		{1, 53, "a 0.6 rise is shared, giving 5.3 not 5.6"},
		{2, 44, "a fall is taken in full"},
		{3, 70, "an unchanged price sells for what it cost"},
	} {
		var p PlayerMetrics
		for _, q := range squad {
			if q.ID == tc.id {
				p = q
			}
		}
		if got := s.sellPrice(p); got != tc.want {
			t.Errorf("player %d sells for %d, want %d — %s", tc.id, got, tc.want, tc.why)
		}
	}
}

// A risen player raises less than he is worth, so fewer moves are affordable.
// If this stops holding, the search is spending money the manager does not have.
func TestRisenPlayersFundLessThanTheirMarketPrice(t *testing.T) {
	squad := make([]PlayerMetrics, 0, SquadSize)
	id := 0
	add := func(pos, team string, price, score float64) {
		id++
		squad = append(squad, PlayerMetrics{ID: id, Name: pos + team, Position: pos,
			Team: team, Price: price, Score: score, Status: "available"})
	}
	add("GKP", "A", 5.0, 3.0)
	add("GKP", "B", 4.0, 1.0)
	for i := 0; i < 5; i++ {
		add("DEF", string(rune('C'+i)), 5.0, 3.0)
	}
	for i := 0; i < 5; i++ {
		add("MID", string(rune('H'+i)), 6.0, 4.0)
	}
	for i := 0; i < 3; i++ {
		add("FWD", string(rune('N'+i)), 7.0, 4.0)
	}

	// A target one notch above what the squad's midfielder is worth at market.
	target := PlayerMetrics{ID: 99, Name: "target", Position: "MID", Team: "ZZZ",
		Price: 6.5, Score: 9.0, Status: "available"}
	cands := append(append([]PlayerMetrics(nil), squad...), target)

	atMarket := NewSquadState(squad)
	if got := RankSwaps(atMarket, cands, 5); len(got) == 0 {
		t.Fatal("no swap found at market prices; the fixture cannot show anything")
	}

	// Same squad, but every midfielder was bought 1.0 cheaper, so each sells for
	// 0.5 less than his market price and the same £0.5m bank no longer covers it.
	risen := NewSquadState(squad)
	risen.Sell = map[int]int{}
	for _, p := range squad {
		if p.Position == "MID" {
			risen.Sell[p.ID] = tenths(p.Price) - 5
		}
	}
	for _, sw := range RankSwaps(risen, cands, 5) {
		if sw.In.ID == target.ID {
			t.Error("bought a target the squad can no longer afford: selling prices " +
				"were ignored")
		}
	}
}
