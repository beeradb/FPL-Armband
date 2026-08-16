package backtest

// Tier 1 for the minutes oracle, at the seam it actually perturbs.
//
//	go test ./internal/backtest -run TestMinutesOracle -v
//
// # Why the bootstrap diff is not enough on its own
//
// `Engine.Recent` is a **second channel into the same quantity**. A Tier 1 case
// covering only the bootstrap would leave a minutes oracle free to perturb every
// per-90 rate through the back door while the declaration reported a clean sheet —
// and "minutes multiply into every rate, so a minutes oracle that touched one
// would quietly become a points oracle" is the specific failure the
// oracle-design document names as the reason Tier 1 exists at all.
//
// So there are two guarantees, not one. `tierOneCases` asserts the bootstrap comes
// back byte-identical, which is the stronger claim of the two and the one that
// makes "never a rate" achievable. This asserts that at the seam the oracle *does*
// perturb, only the four minutes fields move.

import (
	"reflect"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// minutesFields are the RecentPlayer fields OracleMinutes may rewrite.
//
// Named, and checked by reflection against the struct rather than by a
// hand-written comparison, so a field added to analysis.RecentPlayer is covered
// the day it lands instead of the day somebody remembers this test.
var minutesFields = map[string]bool{
	"MinutesPerMatch": true,
	"StartShare":      true,
	"Matches":         true,
	"BlankRun":        true,
}

// TestMinutesOraclePerturbsOnlyMinutes diffs the honest recency index against the
// oracled one, field by field, over every season in the grid and every gameweek.
//
// Table-driven over **both** oracles that perturb this seam. They are two
// resolutions of one quantity travelling down one channel, so a guarantee that
// covered only one of them would leave the other free to reach a per-90 rate with
// its declaration reporting a clean sheet.
func TestMinutesOraclePerturbsOnlyMinutes(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the season archive")
	}
	for _, oracle := range []InfoOracle{OracleMinutes, OracleLineups} {
		t.Run(infoName[oracle], func(t *testing.T) {
			// A rate half-life so the honest index actually populates its rate
			// fields. With the shipped zero they are all zero on both sides and the
			// test would pass without ever exercising the thing it exists to check —
			// a null result indistinguishable from a real one, which is the failure
			// this whole tier is about.
			cfg := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4, RateHalfLife: 8}}
			oracled := cfg
			oracled.Oracles = Oracles{Info: oracle}

			moved := map[string]int{}
			for _, pair := range sweepPairNames() {
				cur := loadForInputDiff(t, pair[1])
				for _, through := range []int{1, 5, 12, 19, 26, 33, 37} {
					base := cfg.recentIndex(cur, through)
					got := oracled.recentIndex(cur, through)
					if base == nil || got == nil {
						t.Fatalf("%s through GW%d: a nil index; the honest one is nil "+
							"only pre-season and the oracled one never is", pair[1], through)
					}
					for _, p := range cur.Players {
						if p.Code == 0 {
							continue
						}
						want, hadWant := base.Get(p.Code)
						have, hadHave := got.Get(p.Code)
						if !hadHave {
							if hadWant {
								t.Fatalf("%s GW%d: the oracle dropped player %d, who the "+
									"honest index knows about", pair[1], through, p.ID)
							}
							continue
						}
						diffRecent(t, want, have, moved, pair[1], through, p.ID)
					}
				}
			}

			// The mediator. A declared field that never moves means the oracle is
			// wired and inert, which produces a clean null indistinguishable from a
			// real one.
			for f := range minutesFields {
				if f == "BlankRun" {
					// BlankRun is deliberately *zeroed* rather than corrected, and the
					// honest index leaves it at zero for almost everyone, so it is not
					// a mediator. Excluded by name rather than silently, so that
					// reading this test tells you which fields are evidence.
					continue
				}
				if moved[f] == 0 {
					t.Errorf("%s never changed %s across the whole grid — an oracle "+
						"that changes nothing measures nothing, and reports it as a "+
						"null result", infoName[oracle], f)
				}
			}
		})
	}
}

// TestLineupsKnowsSelectionAndNotQuantity is the whole claim of the split, on the
// synthetic season where the arithmetic is checkable.
//
// The two arms must agree exactly about *who is picked* and disagree about *how
// long he stays on*. If they agreed on both, the decomposition would be measuring
// nothing; if they disagreed about selection, the residual would be measuring the
// classifier rather than the within-state variation.
func TestLineupsKnowsSelectionAndNotQuantity(t *testing.T) {
	// Starts every week and is hooked at wildly different times: 90 in the odd
	// gameweeks, 30 in the even ones, so his own conditional average is 60 and
	// never equal to what he actually plays. That is the whole distinction — the
	// selection fact is identical every week and the quantity is not.
	swings := map[int]GW{}
	for gw := 1; gw <= 38; gw++ {
		mins := 30
		if gw%2 == 1 {
			mins = 90
		}
		swings[gw] = GW{Minutes: mins, Starts: 1, Fixtures: 1}
	}
	// An ever-present, for the control: with no within-state variation the two
	// arms must agree exactly, which is what makes the disagreement above a
	// statement about variation rather than about the wiring.
	steady := map[int]GW{}
	// A permanent substitute, to pin that the two states are priced apart.
	bench := map[int]GW{}
	for gw := 1; gw <= 38; gw++ {
		steady[gw] = GW{Minutes: 90, Starts: 1, Fixtures: 1}
		bench[gw] = GW{Minutes: 20, Fixtures: 1}
	}

	s := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Code: 101, Team: 1, Type: 3, GWs: swings},
		2: {ID: 2, Code: 102, Team: 1, Type: 3, GWs: steady},
		3: {ID: 3, Code: 103, Team: 1, Type: 3, GWs: bench},
	}}
	s.Fixtures = everyClubPlaysEveryWeek(1)

	base := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4, Horizon: 5}}
	mins, lineups := base, base
	mins.Oracles = Oracles{Info: OracleMinutes}
	lineups.Oracles = Oracles{Info: OracleLineups}
	// Through GW10, so the window is GW11-15: three 90s and two 30s.
	mIdx, lIdx := mins.recentIndex(s, 10), lineups.recentIndex(s, 10)

	m, _ := mIdx.Get(101)
	l, _ := lIdx.Get(101)
	if m.StartShare != 1 || l.StartShare != 1 {
		t.Errorf("start share reads %v under minutes and %v under lineups, want 1 "+
			"under both — selection is the fact BOTH arms are given, and the two "+
			"must not differ about it", m.StartShare, l.StartShare)
	}
	if want := (90.0*3 + 30.0*2) / 5; m.MinutesPerMatch != want {
		t.Errorf("the minutes arm reads %v over GW11-15, want the realised %v",
			m.MinutesPerMatch, want)
	}
	if l.MinutesPerMatch != 60 {
		t.Errorf("the lineups arm prices his starts at %v, want his own conditional "+
			"average of 60 — a conditional average is exactly what cannot see when "+
			"the hook comes", l.MinutesPerMatch)
	}

	// The control: no within-state variation, so no residual.
	m, _ = mIdx.Get(102)
	l, _ = lIdx.Get(102)
	if m.MinutesPerMatch != 90 || l.MinutesPerMatch != 90 {
		t.Errorf("the ever-present reads %v under minutes and %v under lineups; "+
			"with nothing varying within the state the two arms must agree",
			m.MinutesPerMatch, l.MinutesPerMatch)
	}

	// And the two states are priced apart rather than pooled.
	l, _ = lIdx.Get(103)
	if l.MinutesPerMatch != 20 {
		t.Errorf("the permanent substitute reads %v under lineups, want his own "+
			"conditional substitute average of 20 — pooling the two states would "+
			"price him near a starter", l.MinutesPerMatch)
	}
	if l.StartShare != 0 {
		t.Errorf("the permanent substitute reads a start share of %v, want 0",
			l.StartShare)
	}
}

// TestADoubleDoesNotInventASubstituteAppearance is the regression test for the
// commonest double-gameweek shape.
//
// A player who starts one leg and is left out of the other records
// `Starts: 1, Minutes: 90`. Crediting a substitute appearance for the leg he did
// not start prices him at about (start + sub)/2 where the truth is start/2 —
// roughly 50 minutes a match against 45. That shape occurs 418 times across the
// four archive seasons, against 319 for the opposite error the first version of
// this code documented, so the undocumented direction was the larger one. It lands
// only on the lineups arm, which is to say directly on the residual.
func TestADoubleDoesNotInventASubstituteAppearance(t *testing.T) {
	// Two clubs so the conditional prices come from somewhere other than the
	// player under test: club 1 plays a double in GW3, club 2 plays singles.
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Code: 101, Team: 1, Type: 3, GWs: map[int]GW{
			1: {Minutes: 90, Starts: 1, Fixtures: 1},
			2: {Minutes: 90, Starts: 1, Fixtures: 1},
			// The double: started one leg, not in the squad for the other.
			3: {Minutes: 90, Starts: 1, Fixtures: 2},
		}},
		2: {ID: 2, Code: 102, Team: 2, Type: 3, GWs: startEverySingleWeek()},
		// A permanent substitute, so the season supports a substitute price at
		// all. Without one, forPlayer returns false for everybody and the lineups
		// arm falls through to the honest index — which would make this test pass
		// or fail for a reason that has nothing to do with doubles.
		3: {ID: 3, Code: 103, Team: 2, Type: 3, GWs: benchEverySingleWeek()},
	}}
	s.Fixtures = append(everyClubPlaysEveryWeek(2), clubFixture(1, 1),
		clubFixture(1, 2), clubFixture(1, 3), clubFixture(1, 3))

	cfg := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4, Horizon: 1}}
	lineups, mins := cfg, cfg
	lineups.Oracles = Oracles{Info: OracleLineups}
	mins.Oracles = Oracles{Info: OracleMinutes}

	// Through GW2, so the window is the double alone: two club fixtures, one
	// start, ninety minutes.
	m, _ := mins.recentIndex(s, 2).Get(101)
	if m.MinutesPerMatch != 45 {
		t.Fatalf("the minutes arm reads %v over the double, want 90/2 = 45",
			m.MinutesPerMatch)
	}
	l, ok := lineups.recentIndex(s, 2).Get(101)
	if !ok {
		t.Fatal("no lineups entry for the doubling player")
	}
	// Asserted against the prices rather than against a literal: he has only two
	// single-fixture rows of his own, so his start resolves at the position rung,
	// and pinning a number here would be pinning the fixture's arithmetic instead
	// of the rule under test.
	cm, ok := newConditionalTable(s).forPlayer(101, 3)
	if !ok {
		t.Fatal("the fixture supports no conditional prices, so the lineups arm " +
			"falls through to the honest index and this test checks nothing")
	}
	if want := cm.start / 2; l.MinutesPerMatch != want {
		t.Errorf("the lineups arm reads %v over the double, want one start and one "+
			"absence over two fixtures = %v. Crediting a substitute appearance for "+
			"the leg he did not start would give %v, above what the minutes arm "+
			"reports, and that error lands on the residual the decomposition exists "+
			"to produce", l.MinutesPerMatch, want, (cm.start+cm.sub)/2)
	}

	// And the shape that *is* a substitute appearance still reads as one: started
	// one leg and came on in the other, so minutes exceed ninety per start.
	s.Players[1].GWs[3] = GW{Minutes: 110, Starts: 1, Fixtures: 2}
	l, _ = lineups.recentIndex(s, 2).Get(101)
	if want := (cm.start + cm.sub) / 2; l.MinutesPerMatch != want {
		t.Errorf("with 110 minutes over a double the lineups arm reads %v, want %v "+
			"— minutes beyond ninety per started fixture can only come from a "+
			"fixture he did not start", l.MinutesPerMatch, want)
	}
}

func startEverySingleWeek() map[int]GW {
	out := map[int]GW{}
	for gw := 1; gw <= 38; gw++ {
		out[gw] = GW{Minutes: 80, Starts: 1, Fixtures: 1}
	}
	return out
}

func benchEverySingleWeek() map[int]GW {
	out := map[int]GW{}
	for gw := 1; gw <= 38; gw++ {
		out[gw] = GW{Minutes: 20, Fixtures: 1}
	}
	return out
}

// TestASubstituteIsNotAStarter guards the value that would make the lineups arm a
// silently different oracle.
//
// "A substitute who regularly plays 70 minutes is not a real player, and a
// conditional average that implies one is a bug." A substitute appearance is
// 15-25 minutes across every season and outfield position, and — the part that
// matters for scoring — it does not reach sixty, which is where appearance points
// and the clean sheet both step.
func TestConditionalMinutesAreRealistic(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the season archive")
	}
	for _, pair := range sweepPairNames() {
		cur := loadForInputDiff(t, pair[1])
		tab := newConditionalTable(cur)
		league, ok := tab.leagueMinutes()
		if !ok {
			t.Fatalf("%s produced no league conditional average at all, so the "+
				"lineups oracle would emit nothing for every player and report a "+
				"clean null", pair[1])
		}
		if league.start < 75 || league.start > 90 {
			t.Errorf("%s prices a start at %.1f minutes, want 75-90 — the archive "+
				"measures 83 to 87", pair[1], league.start)
		}
		if league.sub < 10 || league.sub >= 60 {
			t.Errorf("%s prices a substitute appearance at %.1f minutes, want 10-60 "+
				"and measured at about 18. At or above sixty it would clear the "+
				"appearance-point and clean-sheet thresholds, which is the one thing "+
				"a substitute structurally cannot do", pair[1], league.sub)
		}
		for pos := 1; pos <= 4; pos++ {
			// -1 is not a real player code, so this resolves at the position rung.
			cm, ok := tab.forPlayer(-1, pos)
			if !ok {
				t.Errorf("%s position %d has no conditional average", pair[1], pos)
				continue
			}
			if cm.start < cm.sub {
				t.Errorf("%s position %d prices a start at %.1f and a substitute "+
					"appearance at %.1f", pair[1], pos, cm.start, cm.sub)
			}
		}
	}
}

// TestMinutesAndLineupsCannotBeComposed pins the refusal.
//
// They rewrite the same field on the same seam, so an arm carrying both is one
// oracle silently winning over the other while its stamp names a decomposition it
// never ran.
func TestMinutesAndLineupsCannotBeComposed(t *testing.T) {
	err := Oracles{Info: OracleMinutes | OracleLineups}.Validate()
	if err == nil {
		t.Fatal("Validate accepted minutes and lineups in one arm; the two are " +
			"resolutions of one quantity and composing them measures neither")
	}
	for _, o := range []Oracles{{Info: OracleMinutes}, {Info: OracleLineups}} {
		if err := o.Validate(); err != nil {
			t.Errorf("%s alone is refused: %v", o.Stamp(), err)
		}
	}
}

// diffRecent compares two RecentPlayer values field by field.
//
// Reflection rather than a hand-written list, the same choice diffBootstraps
// makes: a hand-written list silently stops covering the field somebody adds to
// analysis.RecentPlayer next, and this guards exactly the change nobody thinks to
// re-check.
func diffRecent(t *testing.T, want, got analysis.RecentPlayer, moved map[string]int,
	season string, through, id int) {
	t.Helper()
	a, b := reflect.ValueOf(want), reflect.ValueOf(got)
	typ := a.Type()
	for f := 0; f < typ.NumField(); f++ {
		name := typ.Field(f).Name
		if a.Field(f).Interface() == b.Field(f).Interface() {
			continue
		}
		if !minutesFields[name] {
			t.Fatalf("%s through GW%d: player %d's %s changed from %v to %v. "+
				"OracleMinutes may rewrite minutes and starts and nothing else — a "+
				"per-90 rate moving means it has stopped bounding perfect rotation "+
				"knowledge and started bounding knowing who will score",
				season, through, id, name,
				a.Field(f).Interface(), b.Field(f).Interface())
		}
		moved[name]++
	}
}

// TestMinutesOracleIsAboutTheFutureNotThePast pins what "perfect minutes" means,
// on a synthetic season where the answer is arithmetic rather than archival.
func TestMinutesOracleIsAboutTheFutureNotThePast(t *testing.T) {
	s := oracleToySeason()

	cfg := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4, Horizon: 2}}
	cfg.Oracles = Oracles{Info: OracleMinutes}
	idx := cfg.recentIndex(s, 2)

	got, ok := idx.Get(101)
	if !ok {
		t.Fatal("the about-to-stop player has no entry")
	}
	if got.MinutesPerMatch != 0 {
		t.Errorf("a player who never plays again reads %v minutes per match, want 0 "+
			"— this is the population OracleAvailability catches only when the "+
			"season total is zero, and it is the whole point of generalising it",
			got.MinutesPerMatch)
	}

	got, ok = idx.Get(102)
	if !ok {
		t.Fatal("a player the honest index has never seen got no entry; the oracle " +
			"must be able to add one, or a returning injury stays invisible")
	}
	if got.MinutesPerMatch != 90 || got.StartShare != 1 {
		t.Errorf("the about-to-start player reads %v/%v, want 90/1",
			got.MinutesPerMatch, got.StartShare)
	}

	got, _ = idx.Get(103)
	if got.MinutesPerMatch != 90 {
		t.Errorf("a double gameweek reads %v minutes per match, want 90 — "+
			"MinutesPerMatch is a statement about how much of a MATCH he plays",
			got.MinutesPerMatch)
	}

	// And with the oracle off, nothing changes: the honest index knows only that
	// player 1 has been an ever-present and has never heard of player 2.
	plain := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4}}.recentIndex(s, 2)
	if r, _ := plain.Get(101); r.MinutesPerMatch != 90 {
		t.Errorf("without the oracle the ever-present reads %v, want 90", r.MinutesPerMatch)
	}
	if _, ok := plain.Get(102); ok {
		t.Error("without the oracle a player who has not appeared has an entry")
	}
}

// TestTheOracleWindowMovesWithTheSeason is the regression test for the defect
// that split this oracle in two.
//
// The first version divided the whole remainder of the season by the whole
// remainder's fixtures, so a player absent for six weeks and then fully fit
// arrived as a mildly-reduced ever-present in *every* week, including the weeks
// he was out. A bounded window makes the number a statement about the decision
// being taken: zero while he is out, ninety once he is back.
func TestTheOracleWindowMovesWithTheSeason(t *testing.T) {
	gws := map[int]GW{}
	for gw := 1; gw <= 38; gw++ {
		switch {
		case gw >= 22 && gw <= 27:
			// Out. The archive would usually omit the row entirely, which the next
			// test covers; here it is present and empty, the other real shape.
			gws[gw] = GW{Fixtures: 1}
		default:
			gws[gw] = GW{Minutes: 90, Starts: 1, Fixtures: 1}
		}
	}
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Code: 101, Team: 1, GWs: gws},
	}}
	s.Fixtures = everyClubPlaysEveryWeek(1)

	cfg := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4, Horizon: 5}}
	cfg.Oracles = Oracles{Info: OracleMinutes}

	// Decided at GW21, looking at GW22-26: he is in hospital for all of it.
	if r, _ := cfg.recentIndex(s, 21).Get(101); r.MinutesPerMatch != 0 {
		t.Errorf("at the GW21 decision the absent player reads %v minutes per "+
			"match, want 0 — a season average would report about 74, which is the "+
			"defect this window exists to remove", r.MinutesPerMatch)
	}
	// Decided at GW27, looking at GW28-32: he is back and playing every minute.
	if r, _ := cfg.recentIndex(s, 27).Get(101); r.MinutesPerMatch != 90 {
		t.Errorf("at the GW27 decision the returning player reads %v minutes per "+
			"match, want 90", r.MinutesPerMatch)
	}
	// And the two must differ, which is the whole claim: one scalar per player
	// for the season cannot express a trajectory.
	a, _ := cfg.recentIndex(s, 21).Get(101)
	b, _ := cfg.recentIndex(s, 27).Get(101)
	if a.MinutesPerMatch == b.MinutesPerMatch {
		t.Error("the oracle reports the same minutes before and after a six-week " +
			"absence, so it is answering a season-average question rather than the " +
			"question the weekly decision asks")
	}
}

// TestAMissingRowIsAnAbsence pins the denominator.
//
// The archive omits a player's gameweek row when he is not in the squad at all —
// about 3,000 of 30,000 club-gameweeks a season, and disproportionately the
// injured and departed. Dividing his minutes by the gameweeks he *has a row for*
// therefore reads a player who vanishes as an ever-present over exactly the weeks
// he is missing, which is the same blindness the window fixes, in a second place.
func TestAMissingRowIsAnAbsence(t *testing.T) {
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Code: 101, Team: 1, GWs: map[int]GW{
			// Plays GW6, then has no row at all for GW7-10.
			6: {Minutes: 90, Starts: 1, Fixtures: 1},
		}},
	}}
	s.Fixtures = everyClubPlaysEveryWeek(1)

	cfg := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4, Horizon: 4}}
	cfg.Oracles = Oracles{Info: OracleMinutes}

	// Through GW6, window GW7-10: four club fixtures, no rows, no football.
	r, ok := cfg.recentIndex(s, 6).Get(101)
	if !ok {
		t.Fatal("no entry for a player whose club plays four more times")
	}
	if r.MinutesPerMatch != 0 || r.Matches != 4 {
		t.Errorf("a player with no rows over four club fixtures reads %v minutes "+
			"per match over %d matches, want 0 over 4 — counting only his own rows "+
			"would divide by zero football and report him as an ever-present",
			r.MinutesPerMatch, r.Matches)
	}

	// Through GW5, window GW6-9: one appearance in four fixtures.
	if r, _ := cfg.recentIndex(s, 5).Get(101); r.MinutesPerMatch != 90.0/4 {
		t.Errorf("one full match in four club fixtures reads %v, want %v",
			r.MinutesPerMatch, 90.0/4)
	}
}

// TestAClubWithNoFixturesLeavesTheHonestIndexAlone covers the degenerate window.
//
// There is nothing for an oracle to be right about when no football is played, so
// it must say nothing rather than say zero — reporting zero would tell the model
// a fit player is about to stop, purely because his club blanks.
func TestAClubWithNoFixturesLeavesTheHonestIndexAlone(t *testing.T) {
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Code: 101, Team: 1, GWs: map[int]GW{
			1: {Minutes: 90, Starts: 1, Fixtures: 1},
			2: {Minutes: 90, Starts: 1, Fixtures: 1},
		}},
	}}
	// The club plays GW1 and GW2 and never again.
	s.Fixtures = everyClubPlaysEveryWeek(1)[:2]

	cfg := SimConfig{Weights: analysis.Weights{MinutesHalfLife: 4, Horizon: 5}}
	cfg.Oracles = Oracles{Info: OracleMinutes}
	r, ok := cfg.recentIndex(s, 2).Get(101)
	if !ok {
		t.Fatal("the oracle dropped a player the honest index knows about")
	}
	if r.MinutesPerMatch != 90 {
		t.Errorf("with no fixtures in the window the player reads %v, want the "+
			"honest index's 90 — an empty window is not evidence of an absence",
			r.MinutesPerMatch)
	}
}

// oracleToySeason is the three-player fixture both oracles' arithmetic tests use.
func oracleToySeason() *Season {
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		// Played every minute up to the cutoff and then stops: the case the
		// availability oracle is blind to, and the whole reason this one exists.
		1: {ID: 1, Code: 101, Team: 1, GWs: map[int]GW{
			1: {Minutes: 90, Starts: 1, Fixtures: 1},
			2: {Minutes: 90, Starts: 1, Fixtures: 1},
			3: {Minutes: 0, Fixtures: 1},
			4: {Minutes: 0, Fixtures: 1},
		}},
		// Has not played yet and is about to be an ever-present. The honest index
		// has no entry for him at all before GW3, so the oracle must be able to
		// *add* one rather than only to correct one.
		2: {ID: 2, Code: 102, Team: 1, GWs: map[int]GW{
			3: {Minutes: 90, Starts: 1, Fixtures: 1},
			4: {Minutes: 90, Starts: 1, Fixtures: 1},
		}},
		// A double gameweek: 180 minutes over two fixtures is 90 per *match*, not
		// 180 per gameweek. Getting this wrong would predict 180 for every single
		// gameweek that follows.
		3: {ID: 3, Code: 103, Team: 2, GWs: map[int]GW{
			3: {Minutes: 180, Starts: 2, Fixtures: 2},
		}},
	}}
	// Club 1 plays once a week; club 2 plays twice in GW3 and not in GW4, so
	// player 103's window is the double alone.
	s.Fixtures = everyClubPlaysEveryWeek(1)
	s.Fixtures = append(s.Fixtures, clubFixture(2, 3), clubFixture(2, 3))
	return s
}

// everyClubPlaysEveryWeek is a 38-gameweek calendar for one club.
func everyClubPlaysEveryWeek(team int) []fpl.Fixture {
	out := make([]fpl.Fixture, 0, 38)
	for gw := 1; gw <= 38; gw++ {
		out = append(out, clubFixture(team, gw))
	}
	return out
}

func clubFixture(team, gw int) fpl.Fixture {
	e := gw
	return fpl.Fixture{Event: &e, TeamH: team, TeamA: 99}
}
