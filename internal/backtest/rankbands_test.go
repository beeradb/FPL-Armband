package backtest

import "testing"

// Price-rank membership decides which players two different diagnostics score,
// so it is one implementation with two callers. These pin the properties both of
// them rely on.

// ⚠️ The tie-break is the whole point. Callers build their rows by ranging a MAP,
// so input order is randomised per run; `sort.Slice` is not stable; and FPL
// prices move in 0.1 steps, so ties are everywhere. Two runs of the candidate-set
// diagnostic once disagreed by up to 0.007 of skill on the same population —
// the size of the differences these tables are read for.
func TestPriceRankOrderIsTotalSoTiedPricesCannotReorderBetweenRuns(t *testing.T) {
	// Every player costs exactly the same, which is the worst case: without a
	// tie-break the order is whatever the caller's map produced.
	ids := []int{7, 3, 91, 12, 45, 2}
	prices := []float64{5.0, 5.0, 5.0, 5.0, 5.0, 5.0}
	want := []int{2, 3, 7, 12, 45, 91}

	got := priceRankOrder(ids, prices)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("all prices equal, so the order is by id ascending: want %v, got %v",
				want, got)
		}
	}

	// The same players presented in a different order must rank identically.
	shuffled := []int{45, 2, 91, 7, 12, 3}
	again := priceRankOrder(shuffled, prices)
	for i := range want {
		if again[i] != want[i] {
			t.Fatalf("input order must not reach the output — this is the run-to-run "+
				"defect the tie-break exists for: want %v, got %v", want, again)
		}
	}
}

// Price dominates; the id only separates equals. A tie-break that outranked
// price would silently rank by player id.
func TestPriceRankOrderPutsTheExpensiveFirstAndBreaksOnlyTies(t *testing.T) {
	ids := []int{1, 2, 3, 4}
	prices := []float64{4.5, 12.3, 12.3, 7.0}
	want := []int{2, 3, 4, 1}
	got := priceRankOrder(ids, prices)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v (12.3s first, id-ordered; then 7.0; then 4.5), got %v",
				want, got)
		}
	}
}

// Parallel slices that have drifted apart would rank players by other players'
// prices — a silent, plausible-looking wrong answer. It stops instead.
func TestPriceRankOrderRefusesMismatchedInputs(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("mismatched ids and prices must panic rather than rank players " +
				"against prices that are not theirs")
		}
	}()
	priceRankOrder([]int{1, 2, 3}, []float64{5.0, 6.0})
}

// Windows are 1-indexed and inclusive, and they must not overlap — a player
// counted in two bands would appear twice in what is meant to be a partition.
func TestRankWindowIsOneIndexedInclusiveAndDisjoint(t *testing.T) {
	order := []int{10, 20, 30, 40, 50, 60}

	first := rankWindow(order, 1, 2)
	if len(first) != 2 || !first[10] || !first[20] {
		t.Errorf("ranks 1-2 are the first two ids; got %v", first)
	}
	second := rankWindow(order, 3, 4)
	if len(second) != 2 || !second[30] || !second[40] {
		t.Errorf("ranks 3-4 are the third and fourth; got %v", second)
	}
	for id := range first {
		if second[id] {
			t.Errorf("id %d is in two windows; the bands are meant to partition", id)
		}
	}
}

// A window running past the end contributes what it has. Dropping the gameweek
// instead would silently remove the weeks with fewest priced players, which is a
// population change disguised as a missing row.
func TestRankWindowTruncatesRatherThanDroppingAShortGameweek(t *testing.T) {
	order := []int{1, 2, 3}
	got := rankWindow(order, 2, 10)
	if len(got) != 2 || !got[2] || !got[3] {
		t.Errorf("ranks 2-10 of a three-long order are ids 2 and 3; got %v", got)
	}
	if beyond := rankWindow(order, 9, 12); len(beyond) != 0 {
		t.Errorf("a window entirely past the end is empty, not an error; got %v", beyond)
	}
}

// ⚠️ The printed table shows a SUBSET of what the CSV carries, never a different
// list. Two lists maintained separately that are supposed to nest are one
// quantity with two implementations waiting to happen, and the report and the
// sink silently covering different populations is what
// TestPredictionCellsSumToTheReportedTotals exists to catch.
func TestEveryPrintedPopulationIsAlsoEmitted(t *testing.T) {
	rows := bandRows(60)
	emitted := map[string]bool{}
	for _, ps := range emittedPopulations(rows) {
		emitted[ps.name] = true
	}
	for _, pop := range populationOrder {
		if !emitted[pop] {
			t.Errorf("%q is printed but not emitted, so the CSV cannot reproduce the "+
				"printed table", pop)
		}
	}
	for _, b := range candidateBands {
		if name := priceBandPopulation(b); !emitted[name] {
			t.Errorf("band %v is declared but not emitted; nothing downstream could "+
				"select it", b)
		}
	}
}

// The bands must actually partition the players they cover, and cover the ranks
// they claim. A band that quietly held the whole population would read as a
// top-ten result computed on everybody.
func TestTheEmittedBandsPartitionTheTopOfThePriceOrder(t *testing.T) {
	rows := bandRows(60)
	byName := map[string][]playerGW{}
	for _, ps := range emittedPopulations(rows) {
		byName[ps.name] = ps.rows
	}

	seen := map[int]string{}
	for _, b := range candidateBands {
		name := priceBandPopulation(b)
		sel := byName[name]
		if want := b[1] - b[0] + 1; len(sel) != want {
			t.Errorf("%s should hold %d players; got %d", name, want, len(sel))
		}
		for _, r := range sel {
			if prev, dup := seen[r.id]; dup {
				t.Errorf("player %d is in both %s and %s; the bands are disjoint by "+
					"construction", r.id, prev, name)
			}
			seen[r.id] = name
		}
	}

	// The bands cover ranks 1..50 of 60 players, so ten must be left out. A band
	// set that swept in everybody would defeat the purpose of banding.
	if len(seen) != 50 {
		t.Errorf("the declared bands span ranks 1-50, so 50 of the 60 players are "+
			"covered; got %d", len(seen))
	}

	// The most expensive player must land in the first band, not merely somewhere.
	top := priceRankOrder(idsOf(rows), pricesOf(rows))[0]
	if got := seen[top]; got != priceBandPopulation(candidateBands[0]) {
		t.Errorf("the most expensive player belongs to the first band; found him in %q", got)
	}
}

// bandRows builds n players with distinct descending prices, so rank is
// unambiguous and a membership error is visible rather than masked by ties.
func bandRows(n int) []playerGW {
	out := make([]playerGW, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, playerGW{
			id:       i + 1,
			relevant: true,
			price:    float64(150 - i),
			category: catTickers,
			points:   []float64{1, 1, 1},
			minutes:  []float64{1, 1, 1},
			xgi:      []float64{0, 0, 0},
		})
	}
	return out
}

func idsOf(rows []playerGW) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.id
	}
	return out
}

func pricesOf(rows []playerGW) []float64 {
	out := make([]float64, len(rows))
	for i, r := range rows {
		out[i] = r.price
	}
	return out
}
