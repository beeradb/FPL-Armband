package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// TestWeekViewsPriceEachWeeksOwnFixtures — a projected week must be scored on
// the fixtures of THAT week.
//
// `engineAt` isolates a gameweek by skipping every round before it, which is
// what `TeamFixtures` honours and what `fixtureLoadFor` did not. Reading the
// fixture index raw meant every projected week inherited the IMMINENT week's
// load: a club doubling in GW28 read 2.000 at the GW29, GW30 and GW31 views, so
// `XIScore`, `Expected`, that week's eleven and its captain were all built on a
// double that had already been played.
//
// The rebuilt-squad case is why this reaches selection rather than display.
// `WeekViews` calls `Optimize` for a planned wildcard or free hit BEFORE its own
// blank guard runs, so a recommended fifteen was chosen on the doubled scores.
func TestWeekViewsPriceEachWeeksOwnFixtures(t *testing.T) {
	// Club 2 doubles in GW1 and club 3 blanks in GW2.
	e := loadEngine(t, map[int][]int{2: {1}}, map[int][]int{3: {2}})
	for club := 1; club <= 4; club++ {
		e.Boot.Elements = append(e.Boot.Elements, fpl.Element{
			ID: club, Team: club, ElementType: 3, WebName: "P", NowCost: 50,
			Minutes: 2000, Starts: 25, TotalPoints: 100,
		})
	}

	var squad []PlayerMetrics
	for i := range e.Boot.Elements {
		squad = append(squad, e.Metrics(&e.Boot.Elements[i]))
	}

	want := map[int]map[int]float64{ // gameweek -> club -> matches
		1: {1: 1, 2: 2, 3: 1, 4: 1},
		2: {1: 1, 2: 1, 3: 0, 4: 1},
		3: {1: 1, 2: 1, 3: 1, 4: 1},
	}
	views := e.WeekViews(squad, 3)
	if len(views) != 3 {
		t.Fatalf("got %d week views, want 3", len(views))
	}
	for _, v := range views {
		for _, p := range v.Squad {
			if got := p.FixtureLoad; got != want[v.Event][p.ID] {
				t.Errorf("GW%d club %d: fixture load %.3f, want %.3f. The week view is "+
					"pricing another week's fixtures", v.Event, p.ID, got, want[v.Event][p.ID])
			}
		}
	}
}
