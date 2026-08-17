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

// TestAWildcardIsNotBuiltOnOneWeeksBlanks — a wildcard fifteen is KEPT, so it
// must not be chosen on the fixtures of the single week the chip is played in.
//
// This is the trap the blank fix created and it exists only because the fix
// works. `WeekViews` built both rebuilt squads on `engineAt(gw)`, a horizon-1
// engine, where `FixtureLoadInScore()` is true — so once `fixtureLoadFor` could
// express a blank, every blanking club's score was zero before `Optimize` ranked
// on it. A wildcard planned for a heavy blank (2023-24 GW29 blanked twelve clubs
// of twenty) would then return a permanent squad drawn entirely from the clubs
// that happened to play that week: a free-hit squad presented as a wildcard.
// Before the fix a blanking club read >= 1 and the distortion could not arise.
//
// The free hit is the opposite and its own assertion is below: one round IS its
// whole horizon, so excluding the blanking clubs is correct there.
func TestAWildcardIsNotBuiltOnOneWeeksBlanks(t *testing.T) {
	// Club 3 blanks GW2 — the week both chips are planned for. Its players are
	// the best in the league, so over a horizon they are exactly who a permanent
	// squad should buy, and in GW2 alone they are worth nothing. Eight clubs at a
	// flat price, so the only thing separating the two builds is the window.
	e := loadEngine(t, nil, map[int][]int{3: {2}})
	e.Weights.Horizon = 5
	id := 0
	for club := 1; club <= 8; club++ {
		for _, pos := range []int{1, 1, 2, 2, 2, 3, 3, 3, 4, 4} {
			id++
			el := fpl.Element{
				ID: id, Team: club, ElementType: pos, WebName: "P", NowCost: 45,
				Status: "a", Minutes: 2500, Starts: 30, TotalPoints: 100,
			}
			if club == 3 {
				el.ExpectedGoalsPer90 = fpl.Num(0.6)
				el.ExpectedAssistsPer90 = fpl.Num(0.4)
				el.TotalPoints = 200
			}
			e.Boot.Elements = append(e.Boot.Elements, el)
		}
	}
	budget := 1000
	e.SquadValue, e.Bank = &budget, new(int)

	built := func(chip string) []PlayerMetrics {
		e.Chips = ChipSchedule{}
		switch chip {
		case "Wildcard":
			e.Chips.First.Wildcard = 2
		case "Free Hit":
			e.Chips.First.FreeHit = 2
		}
		for _, v := range e.WeekViews(nil, 2) {
			if v.Event == 2 && v.Rebuilt {
				return v.Squad
			}
		}
		t.Fatalf("no rebuilt squad came back for a planned %s at GW2", chip)
		return nil
	}

	from3 := func(sq []PlayerMetrics) int {
		n := 0
		for _, p := range sq {
			if el := e.Boot.ElementByID(p.ID); el != nil && el.Team == 3 {
				n++
			}
		}
		return n
	}

	// The free hit plays for one round, so the blanking club must be absent.
	if n := from3(built("Free Hit")); n != 0 {
		t.Errorf("the free-hit fifteen holds %d players from the club that blanks "+
			"that week; it fields them for exactly that round", n)
	}
	// The wildcard is kept, so a single blank must not empty the club out of it.
	// Club 3 is the strongest in the league over any horizon, and a horizon-5
	// build takes its full three-per-club allowance; a horizon-1 build at GW2
	// takes none. Measured both ways before this assertion was written.
	if n := from3(built("Wildcard")); n == 0 {
		t.Error("the wildcard fifteen holds nobody from the club that blanks its " +
			"own week. That squad is KEPT, so it must be built over the horizon — " +
			"a one-week view zeroes every blanking club and returns a free-hit " +
			"squad under a wildcard's name")
	}
}
