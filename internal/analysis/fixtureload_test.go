package analysis

import (
	"fmt"
	"testing"

	"armband/internal/fpl"
)

// loadEngine builds an engine over a synthetic calendar.
//
// Four clubs, ten gameweeks, and one fixture per club per gameweek except where
// `doubles` and `blanks` say otherwise. Each subject club has a dedicated
// opponent — club 1 always plays club 5, club 2 club 6 — so that blanking one
// subject does not blank another and every assertion below is about the club it
// names. Nothing is finished, so `NextEvent` is gameweek 1 and the whole
// calendar is upcoming, which is the state the shipped scoring path is always in
// at a deadline.
//
// It goes through NewEngineFull rather than assigning `byTeamUpcoming` directly,
// because the anchor this file exists to pin is decided in `buildFixtureIndex`
// and by `NextEvent`. A test that hand-built the index would pass with either
// anchor and pin neither.
func loadEngine(t *testing.T, doubles, blanks map[int][]int) *Engine {
	t.Helper()
	return loadEngineOver(t, 10, doubles, blanks)
}

// loadEngineOver is loadEngine with the length of the season named, for the
// tests about what happens when the calendar runs out.
func loadEngineOver(t *testing.T, weeks int, doubles, blanks map[int][]int) *Engine {
	t.Helper()
	const clubs = 4

	b := &fpl.Bootstrap{
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	// Distinct short names, because `PlayerMetrics.Team` carries the short name
	// and `Optimize` counts the three-per-club cap on it — one shared name makes
	// the whole league read as a single club and no legal fifteen exists.
	for id := 1; id <= 2*clubs; id++ {
		b.Teams = append(b.Teams, fpl.Team{
			ID: id, Name: fmt.Sprintf("Club %d", id),
			ShortName: fmt.Sprintf("C%d", id),
		})
	}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i})
	}

	skipped := func(team, gw int) bool {
		for _, g := range blanks[team] {
			if g == gw {
				return true
			}
		}
		return false
	}
	twice := func(team, gw int) bool {
		for _, g := range doubles[team] {
			if g == gw {
				return true
			}
		}
		return false
	}

	var fx []fpl.Fixture
	next := 0
	for gw := 1; gw <= weeks; gw++ {
		for team := 1; team <= clubs; team++ {
			if skipped(team, gw) {
				continue
			}
			n := 1
			if twice(team, gw) {
				n = 2
			}
			opponent := team + clubs
			for i := 0; i < n; i++ {
				ev := gw
				next++
				fx = append(fx, fpl.Fixture{
					ID: next, Event: &ev, TeamH: team, TeamA: opponent,
					TeamHDifficulty: 3, TeamADifficulty: 3,
				})
			}
		}
	}

	w := DefaultWeights()
	return NewEngineFull(b, fx, w, Congestion{}, RoleRisk{})
}

// TestFixtureLoadCountsDoublesAndBlanks — Score is a per-gameweek expectation
// built from per-90 rates, and nothing in that arithmetic knows how many times
// the club plays. Without this a double gameweek scores the same as a single one
// and a blank scores as though the club had played, which is a factor of two in
// the week it matters.
//
// It exercises `fixtureLoadFor`, which is the function on the scoring path. The
// version of this test it replaces exercised `fixturesPerGameweek`, whose only
// callers in the tree were that test's own assertions — one quantity with two
// implementations, where the measured one was not the one that ran, and the two
// disagreed on precisely the leading blank below.
func TestFixtureLoadCountsDoublesAndBlanks(t *testing.T) {
	// Club 2 blanks the imminent gameweek. This is the case the pre-fix anchor
	// could not express AT ALL: it started the window at the club's next
	// FIXTURE, so at horizon 1 the window was one gameweek wide and contained a
	// fixture by construction. The load was >= 1 always, and this assertion
	// fails on that code.
	e := loadEngine(t, nil, map[int][]int{2: {1}})
	if got := e.fixtureLoadFor(2, 1); got != 0 {
		t.Errorf("a club that blanks the imminent gameweek reads %.3f matches per "+
			"gameweek, want 0. Anchoring on the club's next fixture instead of the "+
			"next gameweek slides the window past the blank and it disappears", got)
	}
	if got := e.fixtureLoadFor(1, 1); got != 1 {
		t.Errorf("a club playing once reads %.3f, want 1", got)
	}
	// One blank in five gameweeks, so four matches over five weeks.
	if got := e.fixtureLoadFor(2, 5); got != 0.8 {
		t.Errorf("one blank inside a five-gameweek horizon reads %.3f, want 0.8", got)
	}

	// The audit found zero doubles mis-counted in 3,554 club-gameweeks, so the
	// doubles half is what the fix must NOT break.
	d := loadEngine(t, map[int][]int{3: {1}}, nil)
	if got := d.fixtureLoadFor(3, 1); got != 2 {
		t.Errorf("a doubling club reads %.3f at horizon 1, want 2", got)
	}
	if got := d.fixtureLoadFor(3, 5); got != 1.2 {
		t.Errorf("a double diluted over five gameweeks reads %.3f, want 1.2", got)
	}
	// A double three weeks out must not read as an imminent one.
	later := loadEngine(t, map[int][]int{3: {4}}, nil)
	if got := later.fixtureLoadFor(3, 1); got != 1 {
		t.Errorf("a club doubling in GW4 reads %.3f in GW1, want 1", got)
	}

	// A blank and a double inside one horizon cancel on the average, which is
	// what "matches per gameweek" means and is not a special case.
	both := loadEngine(t, map[int][]int{2: {3}}, map[int][]int{2: {1}})
	if got := both.fixtureLoadFor(2, 5); got != 1 {
		t.Errorf("a blank and a double inside five gameweeks read %.3f, want 1", got)
	}

	// A club with no remaining fixtures is UNKNOWN, not blanking, and the two
	// are different facts. Zeroing it would erase a player rather than report
	// that nothing is known about his calendar.
	if got := e.fixtureLoadFor(99, 1); got != 1 {
		t.Errorf("a club absent from the fixture index reads %.3f, want 1", got)
	}
}

// TestTheFixtureIndexIsInGameweekOrder — `fixtureLoadFor` stops scanning a
// club's fixtures at the first one past the end of its window, and the dedupe
// that builds `upcomingGWs` compares only against the last gameweek appended.
// Both are correct only because `buildFixtureIndex` sorts before it appends.
//
// It is pinned here because the failure is silent and doubly wrong: an unsorted
// index truncates the window early AND lets `upcomingGWs` hold a gameweek twice,
// which inflates the denominator. The synthetic calendar is built in ascending
// order, so it cannot exercise the sort on its own — the fixtures are shuffled
// first, which is the whole point of the test.
func TestTheFixtureIndexIsInGameweekOrder(t *testing.T) {
	e := loadEngine(t, map[int][]int{2: {3}}, map[int][]int{3: {2}})
	// Reverse the fixture list and rebuild: the index must come back identical.
	for i, j := 0, len(e.Fixtures)-1; i < j; i, j = i+1, j-1 {
		e.Fixtures[i], e.Fixtures[j] = e.Fixtures[j], e.Fixtures[i]
	}
	e.buildFixtureIndex()

	for team, fx := range e.byTeamUpcoming {
		for i := 1; i < len(fx); i++ {
			if fx[i].Event < fx[i-1].Event {
				t.Fatalf("club %d holds GW%d after GW%d. fixtureLoadFor breaks out of "+
					"this scan on the first event past its window, so an unsorted list "+
					"silently truncates the count", team, fx[i].Event, fx[i-1].Event)
			}
		}
	}
	seen := map[int]bool{}
	for i, gw := range e.upcomingGWs {
		if seen[gw] {
			t.Fatalf("upcomingGWs holds GW%d twice; it is the window's DENOMINATOR, "+
				"so a duplicate reports every club as playing less often than it does", gw)
		}
		seen[gw] = true
		if i > 0 && gw < e.upcomingGWs[i-1] {
			t.Fatalf("upcomingGWs is not ascending at index %d (GW%d after GW%d)",
				i, gw, e.upcomingGWs[i-1])
		}
	}
	// And the loads themselves are unchanged by the shuffle, which is the
	// property the two structural checks above exist to protect.
	if got := e.fixtureLoadFor(3, 1); got != 1 {
		t.Errorf("club 3 reads %.3f at horizon 1 after the fixture list was "+
			"shuffled, want 1", got)
	}
	if got := e.fixtureLoadFor(2, 5); got != 1.2 {
		t.Errorf("club 2's GW3 double reads %.3f over five gameweeks after the "+
			"fixture list was shuffled, want 1.2", got)
	}
}

// TestFixtureLoadHonoursTheSkipSet — the load must be counted over the gameweeks
// this engine SCORES, not over the calendar.
//
// Two callers depend on it. `engineAt` isolates a single gameweek by skipping
// every round before it, so a horizon-1 engine whose skip set covers GW1..GW27
// is asking about GW28 — and reading `byTeamUpcoming` raw answered about GW1
// instead, which put an imminent double's 2.0 on every projected week. And
// `ApplyFreeHitToScoring` removes the free-hit week from the permanent squad's
// horizon, where counting a round the squad will not field is the error the skip
// set exists to prevent.
func TestFixtureLoadHonoursTheSkipSet(t *testing.T) {
	e := loadEngine(t, map[int][]int{2: {1}}, nil)

	if got := e.fixtureLoadFor(2, 1); got != 2 {
		t.Fatalf("the double is in GW1 and reads %.3f; the rest of this test is "+
			"vacuous if it is not there to be skipped past", got)
	}

	// Isolate GW2, the way engineAt does.
	e.SetSkipGameweeks([]int{1})
	if got := e.fixtureLoadFor(2, 1); got != 1 {
		t.Errorf("with GW1 skipped, the GW2 view of a club that doubled in GW1 "+
			"reads %.3f, want 1. A skipped week's fixtures are still being counted, "+
			"so every projected week inherits the imminent week's load", got)
	}
	// The window must EXTEND past a skipped week rather than shortening, which
	// is the rule TeamFixtures already follows.
	if got := e.fixtureLoadFor(1, 5); got != 1 {
		t.Errorf("an ordinary club reads %.3f over five scored gameweeks with one "+
			"week skipped, want 1", got)
	}
}

// TestFixtureLoadWindowEndsWithTheSeason — the denominator is the number of
// gameweeks the window actually found.
//
// With two rounds left and a horizon of five, a club playing both plays one
// match per gameweek; dividing by five instead reports 0.4 and marks down every
// player in the league for the last weeks of a season. It is invisible on the
// shipped scoring path, where the horizon is 1, and reachable from the transfer
// objective, where it is not.
func TestFixtureLoadWindowEndsWithTheSeason(t *testing.T) {
	e := loadEngineOver(t, 2, nil, nil)
	if got := e.fixtureLoadFor(1, 5); got != 1 {
		t.Errorf("with two gameweeks left and a horizon of five, a club playing "+
			"both reads %.3f, want 1", got)
	}
}

// TestElementsWithoutFixturesNamesOnlyRealBlanks — the free-hit exclusion has to
// separate "does not play" from "nothing is known", because it is applied to
// selection and an over-broad exclusion silently shrinks the pool.
func TestElementsWithoutFixturesNamesOnlyRealBlanks(t *testing.T) {
	e := loadEngine(t, nil, map[int][]int{2: {1}})
	e.Boot.Elements = []fpl.Element{
		{ID: 1, Team: 1, ElementType: 3}, {ID: 2, Team: 2, ElementType: 3},
		{ID: 3, Team: 2, ElementType: 4}, {ID: 4, Team: 3, ElementType: 2},
	}
	// The engine's horizon has to be the single gameweek a free hit is played in.
	e.Weights.Horizon = 1

	got := map[int]bool{}
	for _, id := range e.ElementsWithoutFixtures() {
		got[id] = true
	}
	if len(got) != 2 || !got[2] || !got[3] {
		t.Errorf("the blanking club's two players are %v; want exactly ids 2 and 3", got)
	}

	// Over five gameweeks club 2 plays four of them, so it is not blanking and
	// nobody is excluded.
	e.Weights.Horizon = 5
	if ids := e.ElementsWithoutFixtures(); len(ids) != 0 {
		t.Errorf("over a five-gameweek horizon a club with one blank is still "+
			"playing, but %v were excluded", ids)
	}
}

// TestATotalBlankIsWorthNothingToTheTransferSearch — the transfer objective must
// price a club that plays no match in the whole window at zero, and a bare
// `FixtureLoad > 0` test cannot do it.
//
// `xiValueForTransfer` multiplies `Score` by `FixtureLoad` for every candidate.
// Its guard used to be `FixtureLoad > 0`, meaning "did Metrics populate this",
// which was sound only while `fixtureLoadFor` could not return a real 0. Once it
// could, the guard read a genuine blank as an unset field and skipped the
// multiply — leaving the one footballer who certainly scores nothing valued at
// full score, in the search that decides who to buy.
//
// The window this needs is a horizon of 2 to 4, which is not exotic:
// `EffectiveHorizon` shortens the transfer horizon to the gap before a planned
// wildcard, and the archive holds blank runs of two and three consecutive
// rounds. At horizon 1 `loadInScore` is true and the guard short-circuits before
// reaching the load at all, so a test written there would pass on the bug.
func TestATotalBlankIsWorthNothingToTheTransferSearch(t *testing.T) {
	if !fixtureLoadTransfers {
		t.Skip("fixture load is switched off for transfers")
	}
	// A club with no fixture anywhere in the window, as Metrics reports it above
	// horizon 1: the load is a real zero and Score has NOT been scaled by it.
	blanked := loadSeamSquad()
	blanked[7].FixtureLoad = 0
	blanked[7].loadInScore = false

	full := XIValue(loadSeamSquad())
	if got := XIValue(blanked); got >= full {
		t.Errorf("XIValue is %.4f with a squad member whose club plays no match in "+
			"the whole window and %.4f with everyone playing. A player who cannot "+
			"appear must be worth strictly less, or the transfer search under-prices "+
			"selling him and over-prices buying him", got, full)
	}

	// The mechanism has to be the multiplier and nothing else: the same row at a
	// load of 1 must recover the untouched value exactly.
	restored := loadSeamSquad()
	restored[7].FixtureLoad = 1
	if got := XIValue(restored); got != full {
		t.Errorf("a load of 1 gives %.4f against %.4f for the untouched squad; the "+
			"multiplier is doing something other than scaling", got, full)
	}
}
