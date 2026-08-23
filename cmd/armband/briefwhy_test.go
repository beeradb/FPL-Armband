package main

import (
	"strings"
	"testing"

	"armband/internal/analysis"
)

func whyPlayer(id int, name, team, pos string, price, score float64) analysis.PlayerMetrics {
	return analysis.PlayerMetrics{ID: id, Name: name, Team: team, Position: pos,
		Price: price, Score: score, ExpectedMinutes: 85, AvgDifficulty: 3}
}

// whySquad builds a legally shaped fifteen — 2/5/5/3 — whose outfielders all score far
// too well to be worth replacing, so the only swap any search can find is at keeper.
//
// The shape matters. RankSwaps ranks on what a move does to the ELEVEN, and XIValue
// cannot pick an eleven out of a squad that has no legal formation in it — so a
// one-player fixture scores zero before and after every swap, every gain prunes to
// nothing, and the section under test reports "no improvement at any price" no matter
// what is in the pool. That is a property of the fixture, not of the code, and it made
// the first draft of these tests pass and fail for the wrong reasons.
//
// Both keepers are priced identically so that the shortfall is the same whichever one
// the search elects to sell, which is what makes the assertions deterministic.
func whySquad() []analysis.PlayerMetrics {
	var sq []analysis.PlayerMetrics
	sq = append(sq,
		whyPlayer(1, "Starter", "AAA", "GKP", 4.5, 3.0),
		whyPlayer(2, "Backup", "AAB", "GKP", 4.5, 2.0))
	id := 10
	for _, c := range []struct {
		pos string
		n   int
	}{{"DEF", 5}, {"MID", 5}, {"FWD", 3}} {
		for i := 0; i < c.n; i++ {
			sq = append(sq, whyPlayer(id, c.pos, "C"+string(rune('A'+i)), c.pos, 5.0, 50.0))
			id++
		}
	}
	return sq
}

func whyClubCounts(sq []analysis.PlayerMetrics) map[string]int {
	m := map[string]int{}
	for _, p := range sq {
		m[p.Team]++
	}
	return m
}

// The nearest miss must price the SALE as well as the purchase.
//
// This is a bug that shipped in the first draft of this section and was caught only by
// running the command: NewSquadState leaves SquadState.Sell nil and the engine's own
// sellPrice falls back to the player's price when the key is missing. Reading the map
// directly skips that fallback and silently reads 0, so the shortfall came out as the
// whole purchase price — £6.0m to replace a £4.5m keeper with a £6.0m one, when the
// true answer is £1.5m. Nothing errors; the number is simply four times too big, on a
// line whose entire job is to tell a reader what the budget cost him.
func TestTheNearestMissPricesTheSaleAndNotJustThePurchase(t *testing.T) {
	var b strings.Builder
	// One £4.5m keeper owned; a £6.0m keeper available who scores more. With no money
	// in the bank the shortfall is 6.0 - 4.5 = £1.5m.
	squad := whySquad()
	pool := []analysis.PlayerMetrics{whyPlayer(99, "Dear", "ZZZ", "GKP", 6.0, 4.0)}
	sq := &analysis.Squad{Players: squad, StartingXI: squad, Remaining: 0.0,
		ClubCounts: whyClubCounts(squad)}

	briefWhyThisFifteen(&b, pool, 6, sq)
	got := b.String()

	if !strings.Contains(got, "£1.5m more") {
		t.Errorf("nearest miss should need £1.5m (6.0 buy minus 4.5 sale), got:\n%s", got)
	}
	if strings.Contains(got, "£6.0m more") {
		t.Error("the shortfall ignored the sale price and reported the whole purchase")
	}
}

// The bank the reader really has must reduce the shortfall, or the line overstates
// what is out of reach.
func TestTheNearestMissSubtractsTheBank(t *testing.T) {
	var b strings.Builder
	squad := whySquad()
	pool := []analysis.PlayerMetrics{whyPlayer(99, "Dear", "ZZZ", "GKP", 6.0, 4.0)}
	// £1.0m in hand against a £1.5m gap leaves £0.5m short.
	sq := &analysis.Squad{Players: squad, StartingXI: squad, Remaining: 1.0,
		ClubCounts: whyClubCounts(squad)}

	briefWhyThisFifteen(&b, pool, 6, sq)
	if got := b.String(); !strings.Contains(got, "£0.5m more") {
		t.Errorf("the bank should reduce the shortfall to £0.5m, got:\n%s", got)
	}
}

// A squad nothing can improve at any price must say so, rather than printing a
// nearest miss it does not have. The empty case is the one that silently prints a
// half-sentence if the "found" flag is ignored.
func TestNoImprovementAtAnyPriceIsStatedRatherThanLeftBlank(t *testing.T) {
	var b strings.Builder
	squad := whySquad()
	// Outscored by every keeper already owned, so no search can want him at any price.
	pool := []analysis.PlayerMetrics{whyPlayer(99, "Worse", "ZZZ", "GKP", 6.0, 0.1)}
	sq := &analysis.Squad{Players: squad, StartingXI: squad, Remaining: 0.0,
		ClubCounts: whyClubCounts(squad)}

	briefWhyThisFifteen(&b, pool, 6, sq)
	got := b.String()
	if !strings.Contains(got, "no money would help") {
		t.Errorf("a squad nothing improves should say so, got:\n%s", got)
	}
	if strings.Contains(got, "nearest miss") {
		t.Error("a nearest miss was printed when there is no improving swap at any price")
	}
}

// Every club at the limit gets named, and the list reads as a sentence.
//
// strings.Join with " and " produced "COV and HUL and IPS" on a real run. The join is
// the assertion here; the club count is what makes three clubs reachable at all.
func TestFullClubsAreNamedAsASentence(t *testing.T) {
	var b strings.Builder
	squad := whySquad()
	sq := &analysis.Squad{Players: squad, StartingXI: squad, Remaining: 0.0,
		ClubCounts: map[string]int{"COV": 3, "HUL": 3, "IPS": 3, "ARS": 2}}

	briefWhyThisFifteen(&b, nil, 6, sq)
	got := b.String()
	if !strings.Contains(got, "COV, HUL and IPS") {
		t.Errorf("full clubs should read as a sentence, got:\n%s", got)
	}
	if strings.Contains(got, "ARS") {
		t.Error("a club under the limit was reported as full")
	}
}

func TestJoinAnd(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"A"}, "A"},
		{[]string{"A", "B"}, "A and B"},
		{[]string{"A", "B", "C"}, "A, B and C"},
	} {
		if got := joinAnd(c.in); got != c.want {
			t.Errorf("joinAnd(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// topPlans must not panic on a list shorter than the cap. The recommend path slices
// [1:] off its result, so an off-by-one here is a crash on a one-plan week.
func TestTopPlansIsSafeBelowTheCap(t *testing.T) {
	for n := 0; n < 5; n++ {
		plans := make([]analysis.Plan, n)
		got := topPlans(plans, 3)
		if want := min(n, 3); len(got) != want {
			t.Errorf("topPlans(%d plans, 3) returned %d, want %d", n, len(got), want)
		}
		_ = got[min(len(got), 1):] // the slice the recommend path takes
	}
}
