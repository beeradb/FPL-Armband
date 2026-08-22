package viewmodel

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
)

var pinned = time.Date(2026, 8, 19, 13, 48, 0, 0, time.UTC)

// samplePage is a small but complete page: a keeper, two outfielders, a bench, a market
// row, an override and two gameweeks. Small enough to assert on by hand, complete enough
// that every branch of Build runs.
func samplePage() present.Page {
	gk := analysis.PlayerMetrics{
		ID: 1, Name: "Kinsky", Team: "TOT", Position: "GKP", Price: 4.5,
		Score: 3.48, FixtureAdjXP90: 3.65, ExpectedMinutes: 88, RotationRisk: "nailed",
		MinutesRating: 0.98, StartShare: 0.9, Ownership: 19.2, AvailabilityFactor: 1,
		AvgDifficulty: 2.8,
		Fixtures: []analysis.FixtureBrief{
			{Event: 1, Opponent: "BRE", Home: false, Difficulty: 3},
			{Event: 2, Opponent: "NEW", Home: true, Difficulty: 2},
		},
	}
	def := analysis.PlayerMetrics{
		ID: 2, Name: "Kadıoğlu", Team: "BHA", Position: "DEF", Price: 4.5,
		Score: 3.20, ExpectedMinutes: 72, RotationRisk: "likely starter",
		AvailabilityFactor: 1,
	}
	fwd := analysis.PlayerMetrics{
		ID: 3, Name: "Haaland", Team: "MCI", Position: "FWD", Price: 15.5,
		Score: 5.55, ExpectedMinutes: 80, RotationRisk: "nailed", AvailabilityFactor: 1,
	}
	bench := analysis.PlayerMetrics{
		ID: 4, Name: "Woodman", Team: "LIV", Position: "GKP", Price: 4.0,
		Score: 0.21, ExpectedMinutes: 5, RotationRisk: "fringe", AvailabilityFactor: 1,
	}
	cand := analysis.PlayerMetrics{
		ID: 100, Name: "Palmer", Team: "CHE", Position: "MID", Price: 9.5,
		Score: 4.02, ExpectedMinutes: 51, RotationRisk: "rotation risk", AvailabilityFactor: 1,
	}

	return present.Page{
		Horizon: 5,
		Token:   "pinnedtoken",
		Teams:   []string{"BHA", "CHE", "LIV", "MCI", "TOT"},
		Codes:   map[int]int{1: 111, 2: 222, 3: 333, 4: 444, 100: 555},
		Squad: analysis.Squad{
			Players:        []analysis.PlayerMetrics{gk, def, fwd, bench},
			StartingXI:     []analysis.PlayerMetrics{gk, def, fwd},
			Bench:          []analysis.PlayerMetrics{bench},
			Formation:      "3-5-2",
			Captain:        fwd,
			ViceCaptain:    def,
			XIScore:        12.23,
			ExpectedPoints: 17.78,
			TotalCost:      100.0,
			Remaining:      0.0,
			ClubCounts:     map[string]int{"TOT": 1, "BHA": 1, "MCI": 1, "LIV": 1},
		},
		Weeks: []analysis.WeekView{
			{Event: 1, Formation: "3-5-2", Expected: 52.3},
			{Event: 2, Formation: "3-4-3", Expected: 49.1, Chip: "Bench Boost"},
		},
		Overrides: map[int]present.Override{
			1: {Kind: "minutes", Code: 111, Label: "MIN 88", Player: "Kinsky",
				Reason: "named first choice", CheckAge: 2},
		},
		Reasoning: &present.Reasoning{
			Overrides: []present.Override{
				{Kind: "minutes", Code: 111, Label: "MIN 88", Player: "Kinsky", CheckAge: 2},
				{Kind: "lock", Code: 333, Label: "LOCK", Player: "Haaland",
					CheckAge: 40, NeedsCheck: true},
			},
			Blind:  []string{"Defences whose numbers no longer describe the side"},
			Policy: present.Policy{MinGainTransfer: 0.4, MinGainHit: 3.0, BankUpTo: 5},
		},
		Watch: &present.Watchlist{
			Rows:       []present.WatchRow{{Player: cand, Delta: 0.94, ClearsGate: true}},
			Benchmarks: []present.WatchBenchmark{{Position: "MID", Name: "Thiago", Score: 3.69, Price: 8.0}},
			Count:      1, Clearing: 1, Gate: 0.4,
		},
	}
}

func build(t *testing.T, p present.Page) *State {
	t.Helper()
	s, err := Build(Input{
		Page: p,
		Boot: &fpl.Bootstrap{Events: []fpl.Event{
			{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC), IsNext: true},
			{ID: 2, DeadlineTime: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)},
		}},
		Cfg: config.Config{},
		Now: pinned,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return s
}

// TestTheStateRoundTripsThroughJSON is the contract's shallowest guarantee and the one
// that breaks first: every client reads this over the wire.
func TestTheStateRoundTripsThroughJSON(t *testing.T) {
	s := build(t, samplePage())
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshalling the state: %v", err)
	}
	var back State
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshalling the state: %v", err)
	}
	if back.Horizon != s.Horizon || len(back.Squad.Players) != len(s.Squad.Players) {
		t.Errorf("the state did not survive a round trip: %d players and horizon %d, "+
			"want %d and %d", len(back.Squad.Players), back.Horizon,
			len(s.Squad.Players), s.Horizon)
	}
	if !back.Now.Equal(pinned) {
		t.Errorf("Now came back as %v, want %v", back.Now, pinned)
	}
}

// TestBuildRefusesANonFiniteNumber pins the guard, and pins it in both directions.
//
// encoding/json REFUSES NaN and Inf outright, so one bad float does not produce one bad
// field — it produces a 500 for the whole document, with a message naming neither the
// player nor the field. Every score here is a division somewhere upstream and a player
// with zero expected minutes is a division this model performs, so this is a live risk
// rather than a theoretical one.
func TestBuildRefusesANonFiniteNumber(t *testing.T) {
	// The liveness half: the same page without the bad value must build.
	if _, err := Build(Input{Page: samplePage(), Boot: &fpl.Bootstrap{}, Now: pinned}); err != nil {
		t.Fatalf("the clean page failed to build, so the guard below proves nothing: %v", err)
	}

	for _, tc := range []struct {
		name string
		set  func(p *present.Page)
	}{
		{"a player's score", func(p *present.Page) {
			p.Squad.Players[0].Score = math.NaN()
		}},
		{"a market delta", func(p *present.Page) {
			p.Watch.Rows[0].Delta = math.Inf(1)
		}},
		{"a squad total", func(p *present.Page) {
			p.Squad.ExpectedPoints = math.Inf(-1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := samplePage()
			tc.set(&p)
			_, err := Build(Input{Page: p, Boot: &fpl.Bootstrap{}, Now: pinned})
			if err == nil {
				t.Fatal("Build accepted a non-finite number; encoding/json will refuse " +
					"the whole document later, where nothing names the field")
			}
			if !strings.Contains(err.Error(), "viewmodel:") {
				t.Errorf("the error does not identify the field: %v", err)
			}
			// And it must actually be unmarshallable, or the guard is guarding nothing.
			if _, jsonErr := json.Marshal(math.NaN()); jsonErr == nil {
				t.Error("encoding/json now accepts NaN; this guard's premise has changed")
			}
		})
	}
}

// TestEveryPlayerCarriesHisPermanentCode pins the one identifier a write may be keyed on.
//
// Element ids are reassigned every summer. An override stored against one comes back in
// August attached to a different footballer, which this project has already shipped once
// — hence config keying every override on the permanent code. The client can only obey
// that rule if the code reaches it.
func TestEveryPlayerCarriesHisPermanentCode(t *testing.T) {
	s := build(t, samplePage())
	for _, p := range s.Squad.Players {
		if p.Code == 0 {
			t.Errorf("%s (id %d) reached the client with no permanent code; a write "+
				"keyed on the element id alone would attach to a different player "+
				"next season", p.Name, p.ID)
		}
	}
	for _, r := range s.Market.Rows {
		if r.Player.Code == 0 {
			t.Errorf("market row %s has no permanent code", r.Player.Name)
		}
	}
}

// TestTheBenchScoreIsThePlainSum pins which of the two bench numbers this is.
//
// The plain sum is what a bench boost would pay. The optimiser's bench VALUE weights each
// slot by the chance it is ever used and is far smaller. Presenting one as the other
// would misprice a chip decision, so the choice is pinned rather than left to a reader of
// the field name.
func TestTheBenchScoreIsThePlainSum(t *testing.T) {
	p := samplePage()
	s := build(t, p)
	var want float64
	for _, m := range p.Squad.Bench {
		want += m.Score
	}
	if math.Abs(s.Squad.BenchScore-want) > 1e-9 {
		t.Errorf("BenchScore is %v, want the plain sum %v", s.Squad.BenchScore, want)
	}
}

// TestTheGameweekRailIsNotCapped pins the design note that the planning window slides.
//
// HANDOFF.md is explicit that the count must not be hard-coded — the prototype froze it
// at six and the copy rule forbids saying "the next five gameweeks" anywhere. A cap here
// would be invisible until a chip was planned outside the window, which is most of the
// reason anyone opens the page.
func TestTheGameweekRailIsNotCapped(t *testing.T) {
	p := samplePage()
	p.Weeks = nil
	for gw := 1; gw <= 14; gw++ {
		p.Weeks = append(p.Weeks, analysis.WeekView{Event: gw, Expected: float64(gw)})
	}
	s := build(t, p)
	if len(s.Gameweeks) != 14 {
		t.Errorf("the rail carries %d gameweeks, want all 14 the build produced", len(s.Gameweeks))
	}
}

// TestTheCurrentGameweekComesFromTheBootstrap pins that the NOW dot is not guessed from
// position in the list.
func TestTheCurrentGameweekComesFromTheBootstrap(t *testing.T) {
	s := build(t, samplePage())
	var marked []int
	for _, g := range s.Gameweeks {
		if g.Current {
			marked = append(marked, g.Number)
		}
	}
	if len(marked) != 1 || marked[0] != 1 {
		t.Errorf("gameweeks marked current: %v, want exactly [1] — the bootstrap's "+
			"is_next event", marked)
	}
	if s.Gameweeks[0].Deadline.IsZero() {
		t.Error("the rail carries no deadline; the countdown has nothing to count to")
	}
	if s.Gameweeks[1].Chip != "Bench Boost" {
		t.Errorf("the chip planned for GW2 is %q, want Bench Boost", s.Gameweeks[1].Chip)
	}
}

// TestClosedGameweeksDropFromTheRailUnlessImported pins the two rules the rail's "GW1 is
// closed" behaviour depends on: Current moves to the next gameweek whose deadline has not
// passed — never FPL's own IsCurrent/IsNext flags, which lag a real deadline (see
// buildGameweeks's own comment for the observed 24-minute case) — and a closed gameweek
// is dropped from the rail entirely unless the reader has imported real picks for it.
func TestClosedGameweeksDropFromTheRailUnlessImported(t *testing.T) {
	p := samplePage()
	boot := &fpl.Bootstrap{Events: []fpl.Event{
		// IsNext still true here on purpose: FPL had not yet flipped it 24 minutes past
		// this exact deadline in production, and Now below is 30 minutes past it. If
		// buildGameweeks used IsNext/IsCurrent instead of the deadline, this fixture
		// would still say GW1, and the test would pass by accident.
		{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC), IsNext: true},
		{ID: 2, DeadlineTime: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)},
	}}
	afterGW1Deadline := time.Date(2026, 8, 21, 18, 0, 0, 0, time.UTC)

	notImported, err := Build(Input{Page: p, Boot: boot, Now: afterGW1Deadline})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, gw := range notImported.Gameweeks {
		if gw.Number == 1 {
			t.Errorf("GW1 is still on the rail after its deadline with no import, want it dropped: %+v", gw)
		}
	}
	if len(notImported.Gameweeks) == 0 || notImported.Gameweeks[0].Number != 2 || !notImported.Gameweeks[0].Current {
		t.Errorf("Gameweeks = %+v, want GW2 first and marked Current", notImported.Gameweeks)
	}

	imported, err := Build(Input{Page: p, Boot: boot, Now: afterGW1Deadline, Import: Import{Entry: 12345}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var gw1 *Gameweek
	for i := range imported.Gameweeks {
		if imported.Gameweeks[i].Number == 1 {
			gw1 = &imported.Gameweeks[i]
		}
	}
	if gw1 == nil {
		t.Fatal("GW1 is missing once the reader has imported; want it present and marked Closed")
	}
	if !gw1.Closed {
		t.Error("GW1.Closed = false, want true — its deadline has passed")
	}
	if gw1.Current {
		t.Error("GW1.Current = true, want false — it is no longer the gameweek to act on")
	}
}

// TestResultsIsAbsentWithoutAnEntry pins the honest-absence rule: no config.EntryID (or
// a failed fetch, from the caller's point of view — build() supplies no Entry either
// way) means no house team to show, not a zeroed one a client might render as "GW0 · 0 pts".
func TestResultsIsAbsentWithoutAnEntry(t *testing.T) {
	s := build(t, samplePage())
	if s.Results != nil {
		t.Errorf("Results = %+v, want nil — no Entry was given", s.Results)
	}
}

// TestResultsResultFieldsAreCarriedNotDerived replaces
// TestResultsProjectedIsTheScoreBugsFigureNotTheRails, deleted with it: CurrentEvent and
// CurrentProjected are gone from the contract (see Results's own comment — pairing the
// rail's Current gameweek with the score bug's Squad.Expected was routinely labelling one
// week's projection next to another week's actual eleven), so there is no longer a "which
// number comes first" invariant to pin.
//
// What replaces it: ResultEvent/ResultState/EventAverage must be exactly what the caller
// (armbandTeamState, via latestClosedEvent and houseLiveSources) hands in through Input,
// never recomputed from Boot or Gameweeks here — this package has no license to decide
// which gameweek is being described, the same rule Import's own comment states. This
// fixture's Boot carries a DIFFERENT current gameweek (GW1, IsNext) from the ResultEvent
// Input asserts (GW2) specifically so a Build that fell back to deriving it from Boot
// would be caught rather than passing by accident.
func TestResultsResultFieldsAreCarriedNotDerived(t *testing.T) {
	rank := 412311
	gwRank := 88214
	history := &fpl.EntryHistory{Current: []struct {
		Event              int  `json:"event"`
		Points             int  `json:"points"`
		TotalPoints        int  `json:"total_points"`
		Rank               *int `json:"rank"`
		OverallRank        *int `json:"overall_rank"`
		Bank               int  `json:"bank"`
		Value              int  `json:"value"`
		EventTransfers     int  `json:"event_transfers"`
		EventTransfersCost int  `json:"event_transfers_cost"`
		PointsOnBench      int  `json:"points_on_bench"`
	}{
		{Event: 2, Points: 62, Rank: &gwRank, EventTransfersCost: 4, PointsOnBench: 8},
	}}
	s, err := Build(Input{
		Page: samplePage(),
		Boot: &fpl.Bootstrap{Events: []fpl.Event{
			{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC), IsNext: true},
			{ID: 2, DeadlineTime: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)},
		}},
		Cfg: config.Config{},
		Now: pinned,
		Entry: &fpl.Entry{
			SummaryOverallPoints: 236,
			SummaryOverallRank:   rank,
		},
		History:      history,
		ResultEvent:  2,
		ResultState:  "final",
		EventAverage: 56,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if s.Results == nil {
		t.Fatal("Results is nil, want a value — Entry was given")
	}
	if s.Results.ResultEvent != 2 {
		t.Errorf("ResultEvent = %d, want 2 (Input's, not Boot's IsNext GW1)", s.Results.ResultEvent)
	}
	if s.Results.ResultState != "final" {
		t.Errorf("ResultState = %q, want %q — carried from Input, not derived here", s.Results.ResultState, "final")
	}
	if s.Results.EventAverage != 56 {
		t.Errorf("EventAverage = %d, want 56", s.Results.EventAverage)
	}
	if s.Results.OverallPoints != 236 || s.Results.OverallRank != rank {
		t.Errorf("OverallPoints/OverallRank = %d/%d, want 236/%d", s.Results.OverallPoints, s.Results.OverallRank, rank)
	}
	if len(s.Results.History) != 1 {
		t.Fatalf("History = %+v, want one entry", s.Results.History)
	}
	got := s.Results.History[0]
	if got.Event != 2 || got.Points != 62 {
		t.Errorf("History[0] Event/Points = %d/%d, want 2/62", got.Event, got.Points)
	}
	if got.Rank == nil || *got.Rank != gwRank {
		t.Errorf("History[0].Rank = %v, want a pointer to %d", got.Rank, gwRank)
	}
	if got.Hit != 4 {
		t.Errorf("History[0].Hit = %d, want 4 (EventTransfersCost)", got.Hit)
	}
	if got.BenchPoints != 8 {
		t.Errorf("History[0].BenchPoints = %d, want 8 (PointsOnBench)", got.BenchPoints)
	}
}

// TestResultsMultiplierIsKeyedByCodeNotID pins the plumbing gotcha the redesign
// introduced: Multiplier arrives keyed by permanent CODE, the same keyspace
// arrangement.Mult/XI/Bench/Captain already use (see picksToFixed), while every other
// lookup inside buildResults — byID, live.ByID — is keyed by element ID. samplePage's
// Codes map gives Haaland ID 3 and Code 333, deliberately different numbers, so a
// buildResults that reads multiplier[p.ID] instead of multiplier[p.Code] finds nothing
// and this test catches it: every card would render at multiplier 0, i.e. as bench.
func TestResultsMultiplierIsKeyedByCodeNotID(t *testing.T) {
	s, err := Build(Input{
		Page: samplePage(),
		Boot: &fpl.Bootstrap{Events: []fpl.Event{
			{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC), IsNext: true},
		}},
		Cfg:   config.Config{},
		Now:   pinned,
		Entry: &fpl.Entry{SummaryOverallPoints: 236},
		// Keyed by CODE (333 = Haaland, 222 = Kadıoğlu — see samplePage's Codes map),
		// not by the IDs (3, 2) those same players carry.
		Multiplier: map[int]int{333: 2, 222: 1},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byID := map[int]TeamPlayer{}
	for _, p := range s.Results.XI {
		byID[p.ID] = p
	}
	if got := byID[3].Multiplier; got != 2 {
		t.Errorf("Haaland (id 3, code 333) Multiplier = %d, want 2 — captain. "+
			"A result of 0 means the lookup used p.ID instead of p.Code.", got)
	}
	if got := byID[2].Multiplier; got != 1 {
		t.Errorf("Kadıoğlu (id 2, code 222) Multiplier = %d, want 1", got)
	}
}

// TestResultsLiveStatsGateOnMatchStatus pins the three rules buildResults's live-data
// half enforces: a goalkeeper gets Saves and never DefCon; an outfielder whose match has
// kicked off gets DefCon and DefConReached, computed against analysis.DefConThreshold for
// his real element type; and a player whose match has NOT kicked off gets neither, even
// though the live payload already has a (zero) stats row for him -- "the match has not
// started" must render as "no verdict yet", not as a false "failed the bar".
func TestResultsLiveStatsGateOnMatchStatus(t *testing.T) {
	s, err := Build(Input{
		Page: samplePage(),
		Boot: &fpl.Bootstrap{
			Events: []fpl.Event{{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC), IsCurrent: true}},
			Elements: []fpl.Element{
				{ID: 1, ElementType: 1}, // Kinsky, GKP
				{ID: 2, ElementType: 2}, // Kadıoğlu, DEF
				{ID: 3, ElementType: 4}, // Haaland, FWD
			},
		},
		Cfg:   config.Config{},
		Now:   pinned,
		Entry: &fpl.Entry{SummaryOverallPoints: 236},
		Live: &fpl.EventLive{Elements: []fpl.LiveElement{
			{ID: 1, Stats: fpl.LiveStats{Minutes: 90, Saves: 4}},
			{ID: 2, Stats: fpl.LiveStats{Minutes: 90, DefensiveContribution: 11}}, // clears the DEF bar (10)
			{ID: 3, Stats: fpl.LiveStats{Minutes: 0}},                             // his match has not kicked off
		}},
		MatchStatus: map[string]string{"TOT": "live", "BHA": "live", "MCI": "scheduled"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byID := map[int]TeamPlayer{}
	for _, p := range s.Results.XI {
		byID[p.ID] = p
	}

	gk := byID[1]
	if gk.Saves == nil || *gk.Saves != 4 {
		t.Errorf("Kinsky (GKP) Saves = %v, want a pointer to 4", gk.Saves)
	}
	if gk.DefCon != nil || gk.DefConReached != nil {
		t.Errorf("Kinsky (GKP) DefCon/DefConReached = %v/%v, want nil/nil — keepers have no DC", gk.DefCon, gk.DefConReached)
	}

	def := byID[2]
	if def.DefCon == nil || *def.DefCon != 11 {
		t.Fatalf("Kadıoğlu DefCon = %v, want a pointer to 11", def.DefCon)
	}
	if def.DefConReached == nil || !*def.DefConReached {
		t.Errorf("Kadıoğlu DefConReached = %v, want true — 11 clears the defender bar of 10", def.DefConReached)
	}
	if def.Saves != nil {
		t.Errorf("Kadıoğlu (DEF) Saves = %v, want nil — outfielders have no saves", def.Saves)
	}

	fwd := byID[3]
	if fwd.MatchStatus != "scheduled" {
		t.Fatalf("Haaland MatchStatus = %q, want %q", fwd.MatchStatus, "scheduled")
	}
	if fwd.DefCon != nil || fwd.DefConReached != nil {
		t.Errorf("Haaland DefCon/DefConReached = %v/%v, want nil/nil — his match has not started, "+
			"so there is no verdict to render yet, not a failing one", fwd.DefCon, fwd.DefConReached)
	}
}

// TestResultsFulltimeStillGetsDefConAndSaves pins the highest-risk line in the
// 2026-08-22 three-way match_status change: "fulltime" (played out, bonus not yet
// applied) MUST gate into buildResults's DefCon/Saves branch the same way "live" and
// "finished" already do. Dropping it from that condition silently strips DefCon and
// saves from every ended-but-unsettled match — a regression in the OPPOSITE direction
// from the live-dot/asterisk bug the same change fixes. If this test fails, that
// condition lost "fulltime" again.
func TestResultsFulltimeStillGetsDefConAndSaves(t *testing.T) {
	s, err := Build(Input{
		Page: samplePage(),
		Boot: &fpl.Bootstrap{
			Events: []fpl.Event{{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC), IsCurrent: true}},
			Elements: []fpl.Element{
				{ID: 1, ElementType: 1}, // Kinsky, GKP
				{ID: 2, ElementType: 2}, // Kadıoğlu, DEF
				{ID: 3, ElementType: 4}, // Haaland, FWD
			},
		},
		Cfg:   config.Config{},
		Now:   pinned,
		Entry: &fpl.Entry{SummaryOverallPoints: 236},
		Live: &fpl.EventLive{Elements: []fpl.LiveElement{
			{ID: 1, Stats: fpl.LiveStats{Minutes: 90, Saves: 4}},
			{ID: 2, Stats: fpl.LiveStats{Minutes: 90, DefensiveContribution: 11}}, // clears the DEF bar (10)
		}},
		MatchStatus: map[string]string{"TOT": "fulltime", "BHA": "fulltime"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byID := map[int]TeamPlayer{}
	for _, p := range s.Results.XI {
		byID[p.ID] = p
	}

	gk := byID[1]
	if gk.Saves == nil || *gk.Saves != 4 {
		t.Errorf("Kinsky (GKP, fulltime) Saves = %v, want a pointer to 4 — fulltime must gate "+
			"the same as live/finished", gk.Saves)
	}

	def := byID[2]
	if def.DefCon == nil || *def.DefCon != 11 {
		t.Fatalf("Kadıoğlu (DEF, fulltime) DefCon = %v, want a pointer to 11", def.DefCon)
	}
	if def.DefConReached == nil || !*def.DefConReached {
		t.Errorf("Kadıoğlu (DEF, fulltime) DefConReached = %v, want true", def.DefConReached)
	}
}

// TestResultsRosterIsTrimmedNotRebuilt pins that the spectator page's XI/Bench are the
// SAME fifteen the interactive builder has -- read off Squad.Players by Squad.XI/Bench's
// own ID order, never re-optimised -- and that a TeamPlayer carries an opponent (for the
// glyph the interactive builder already draws) but none of the interactive vocabulary
// (role, reliability, override) that a spectator page has no use for.
//
// Opponent is asserted against Opponent, not samplePage's Kinsky.Fixtures (which
// happens to start at event 1, the SAME gameweek this test's ResultEvent asks for,
// so it would pass even under the pre-fix Fixtures[0] read this test used to pin —
// TestResultsOpponentIsResultEventsFixtureNotTheNextOne below is the case that tells
// the two sources apart).
func TestResultsRosterIsTrimmedNotRebuilt(t *testing.T) {
	s, err := Build(Input{
		Page: samplePage(),
		Boot: &fpl.Bootstrap{Events: []fpl.Event{
			{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC), IsNext: true},
		}},
		Cfg:         config.Config{},
		Now:         pinned,
		Entry:       &fpl.Entry{SummaryOverallPoints: 236},
		ResultEvent: 1,
		Opponent: map[string]Fixture{
			"TOT": {Gameweek: 1, Opponent: "BRE", Home: false, Difficulty: 3},
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(s.Results.XI) != len(s.Squad.XI) {
		t.Fatalf("Results.XI has %d players, want %d — one per Squad.XI id", len(s.Results.XI), len(s.Squad.XI))
	}
	if len(s.Results.Bench) != len(s.Squad.Bench) {
		t.Fatalf("Results.Bench has %d players, want %d", len(s.Results.Bench), len(s.Squad.Bench))
	}
	gk := s.Results.XI[0]
	if gk.ID != s.Squad.XI[0] || gk.Name != "Kinsky" {
		t.Errorf("Results.XI[0] = %+v, want Kinsky (id %d) — same order as Squad.XI", gk, s.Squad.XI[0])
	}
	if gk.Opponent == nil || gk.Opponent.Opponent != "BRE" || gk.Opponent.Home {
		t.Errorf("Kinsky's Opponent = %+v, want the away BRE fixture Opponent gives him", gk.Opponent)
	}
}

// TestResultsLivePointsSumsTheSquadOnlyWhileLive pins the 2026-08-22 defect:
// history[last].points is FPL's own EntryHistory figure, which reads 0 for a gameweek
// FPL has not finished scoring -- exactly the state a reader is most likely to check this
// page in, live staging showed 0 POINTS above eleven cards visibly scoring. LivePoints
// exists to replace that cell while ResultState is "live"; see its own doc comment.
//
// samplePage's captain (Haaland, id 3, code 333) carries Multiplier 2 here, and the
// bench player (Woodman, id 4, code 444) carries non-zero live points at multiplier 0 --
// a regression that forgot the multiplier (captain's double, bench's zero) or that summed
// the bench unconditionally would both be caught: the multiplier-blind sum is 2+6+9+5=22,
// the bench-counting-without-multiplier sum is 2+6+18+5=31, and the correct answer,
// below, is neither. The gameweek's Hit (4) is subtracted so the live figure means the
// same thing as the settled one once FPL finishes scoring.
func TestResultsLivePointsSumsTheSquadOnlyWhileLive(t *testing.T) {
	rank := 88214
	input := func(resultState string) Input {
		return Input{
			Page: samplePage(),
			Boot: &fpl.Bootstrap{
				Events: []fpl.Event{{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC)}},
				Elements: []fpl.Element{
					{ID: 1, ElementType: 1}, {ID: 2, ElementType: 2}, {ID: 3, ElementType: 4}, {ID: 4, ElementType: 1},
				},
			},
			Cfg:   config.Config{},
			Now:   pinned,
			Entry: &fpl.Entry{SummaryOverallPoints: 236},
			History: &fpl.EntryHistory{Current: []struct {
				Event              int  `json:"event"`
				Points             int  `json:"points"`
				TotalPoints        int  `json:"total_points"`
				Rank               *int `json:"rank"`
				OverallRank        *int `json:"overall_rank"`
				Bank               int  `json:"bank"`
				Value              int  `json:"value"`
				EventTransfers     int  `json:"event_transfers"`
				EventTransfersCost int  `json:"event_transfers_cost"`
				PointsOnBench      int  `json:"points_on_bench"`
			}{
				{Event: 1, Points: 0, Rank: &rank, EventTransfersCost: 4},
			}},
			Live: &fpl.EventLive{Elements: []fpl.LiveElement{
				{ID: 1, Stats: fpl.LiveStats{Minutes: 90, TotalPoints: 2}},
				{ID: 2, Stats: fpl.LiveStats{Minutes: 90, TotalPoints: 6}},
				{ID: 3, Stats: fpl.LiveStats{Minutes: 90, TotalPoints: 9}}, // captain, multiplier 2
				{ID: 4, Stats: fpl.LiveStats{Minutes: 0, TotalPoints: 5}},  // bench, multiplier 0
			}},
			// Keyed by CODE, the same keyspace Multiplier always uses (see
			// TestResultsMultiplierIsKeyedByCodeNotID).
			Multiplier:   map[int]int{111: 1, 222: 1, 333: 2, 444: 0},
			ResultEvent:  1,
			ResultState:  resultState,
			EventAverage: 56,
		}
	}

	live, err := Build(input("live"))
	if err != nil {
		t.Fatalf("Build (live): %v", err)
	}
	const want = 2*1 + 6*1 + 9*2 + 5*0 - 4 // = 22, the transfer hit already subtracted
	if live.Results.LivePoints != want {
		t.Errorf("LivePoints = %d, want %d (multiplier-weighted XI+bench sum, hit subtracted)",
			live.Results.LivePoints, want)
	}

	final, err := Build(input("final"))
	if err != nil {
		t.Fatalf("Build (final): %v", err)
	}
	if final.Results.LivePoints != 0 {
		t.Errorf("LivePoints = %d on a FINAL gameweek, want 0 -- it is set only while live",
			final.Results.LivePoints)
	}
}

// TestResultsOpponentIsResultEventsFixtureNotTheNextOne pins the 2026-08-22 defect:
// buildResults used to read a player's own p.Fixtures[0] for the opponent chip, but
// that list is the model's forward-looking fixture window, anchored on the NEXT planning
// gameweek (analysis.Engine's own fromEvent) -- not ResultEvent, the gameweek this page
// actually reports on. The moment ResultEvent's deadline passes, fromEvent moves beyond
// it and that gameweek's own fixture drops out of Fixtures entirely, so Fixtures[0]
// silently became the NEXT gameweek's opponent beside THIS gameweek's goals, assists and
// points -- caught live 2026-08-22 at result_event 1 showing the GW2 fixture.
//
// Kinsky's Fixtures here starts at event 2 (his GW1 fixture is deliberately absent,
// matching production once fromEvent has rolled past GW1) while Opponent carries
// the true GW1 fixture. A regression back to p.Fixtures[0] resolves to NEW (home) and
// fails this test; the fix must resolve to BRE (away) from Opponent instead.
func TestResultsOpponentIsResultEventsFixtureNotTheNextOne(t *testing.T) {
	p := samplePage()
	p.Squad.Players = append([]analysis.PlayerMetrics(nil), p.Squad.Players...)
	p.Squad.Players[0].Fixtures = []analysis.FixtureBrief{
		{Event: 2, Opponent: "NEW", Home: true, Difficulty: 2},
	}
	s, err := Build(Input{
		Page: p,
		Boot: &fpl.Bootstrap{Events: []fpl.Event{
			{ID: 1, DeadlineTime: time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC)},
		}},
		Cfg:         config.Config{},
		Now:         pinned,
		Entry:       &fpl.Entry{SummaryOverallPoints: 236},
		ResultEvent: 1,
		ResultState: "live",
		Opponent: map[string]Fixture{
			"TOT": {Gameweek: 1, Opponent: "BRE", Home: false, Difficulty: 3},
			// BHA (Kadıoğlu) deliberately absent -- a blank gameweek for that club --
			// so this also pins that an absent club gets a nil Opponent, never a
			// fallback to Fixtures[0].
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byID := map[int]TeamPlayer{}
	for _, tp := range s.Results.XI {
		byID[tp.ID] = tp
	}
	gk := byID[1]
	if gk.Opponent == nil || gk.Opponent.Opponent != "BRE" || gk.Opponent.Home {
		t.Fatalf("Kinsky's Opponent = %+v, want the GW1 away BRE fixture (ResultEvent) "+
			"from Opponent, not GW2's home NEW from Fixtures[0]", gk.Opponent)
	}
	def := byID[2]
	if def.Opponent != nil {
		t.Errorf("Kadıoğlu's Opponent = %+v, want nil — BHA has no fixture in "+
			"Opponent for this gameweek (a blank gameweek), and there must be no "+
			"fallback to his own Fixtures list", def.Opponent)
	}
}

// TestTheOverrideCountsComeFromOneImplementation pins that the API's "how many need a
// check" is asked of the same code the page asks, rather than recounted here. Two counts
// of one quantity is the failure this codebase names most often.
func TestTheOverrideCountsComeFromOneImplementation(t *testing.T) {
	p := samplePage()
	s := build(t, p)
	if s.Overrides.Due != p.Reasoning.Due() {
		t.Errorf("Due is %d, the renderer says %d", s.Overrides.Due, p.Reasoning.Due())
	}
	if s.Overrides.Oldest != p.Reasoning.Oldest() {
		t.Errorf("Oldest is %q, the renderer says %q", s.Overrides.Oldest, p.Reasoning.Oldest())
	}
	if s.Overrides.Due != 1 {
		t.Errorf("Due is %d, want 1 — Haaland's lock is 40 days old", s.Overrides.Due)
	}
}

// TestTheSessionStoreIsStated pins that the client is told where a write lands rather
// than assuming. The design ships copy promising work is saved; which store is doing the
// saving decides whether that promise survives closing the browser.
func TestTheSessionStoreIsStated(t *testing.T) {
	s := build(t, samplePage())
	if s.Session.Store != "session" {
		t.Errorf("store is %q, want session by default", s.Session.Store)
	}

	// Writable is CARRIED, not derived.
	//
	// It used to be `p.Token != ""`, which stopped being the right question when the token
	// became necessary only where a save can reach config.json: the public deployment has no
	// token and can still write, because a save there reaches nothing but the caller's own
	// cookie. Only the caller knows whether its write route will accept this request, so it
	// says, and this asserts the contract carries the answer rather than re-deciding it.
	if s.Session.Writable {
		t.Error("Writable defaulted to true. It must be the caller's statement, and a " +
			"caller that says nothing is not a caller that may write.")
	}
	writable, err := Build(Input{
		Page: samplePage(), Boot: &fpl.Bootstrap{}, Now: pinned, Writable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !writable.Session.Writable {
		t.Error("a caller that said it may write is reported unwritable, so the page will " +
			"hide controls that work")
	}

	persisted, err := Build(Input{Page: samplePage(), Boot: &fpl.Bootstrap{}, Now: pinned, Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Session.Store != "config" {
		t.Errorf("under -persist the store is %q, want config", persisted.Session.Store)
	}
}

// TestARoleBandIsNeverInventedHere pins the passthrough.
//
// analysis.rotationLabel has FIVE bands and HANDOFF.md names four. The client renders
// whatever arrives; nothing in this package or below it may collapse "squad player" into
// a neighbouring band to make the design's list fit, because the thresholds belong to the
// model and a second copy of them is a second answer.
func TestARoleBandIsNeverInventedHere(t *testing.T) {
	p := samplePage()
	p.Squad.Players[0].RotationRisk = "squad player"
	s := build(t, p)
	if s.Squad.Players[0].Role != "squad player" {
		t.Errorf("the role arrived as %q; the band was rewritten on the way out",
			s.Squad.Players[0].Role)
	}
}

// TestMinutesReachTheClientAsANumber is a regression guard, and it replaces a test that
// pinned the phrasing of a pre-formatted ModelledMinutes string.
//
// That string was Minutes wrapped as "90 → 72 modelled". The client aliased it into the
// slot its arithmetic used, so Math.round() rendered "NaN" on the player card and in the
// News tab's standing band, and two sorts compared strings and ordered nothing. It
// reached production.
//
// So the property worth pinning is not a phrase, it is a TYPE: the field the client does
// arithmetic on must serialise as a JSON number. A future change that helpfully
// pre-formats it re-creates the same defect, and this fails instead.
func TestMinutesReachTheClientAsANumber(t *testing.T) {
	s := build(t, samplePage())
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshalling the state: %v", err)
	}
	var raw struct {
		Squad struct {
			Players []map[string]json.RawMessage `json:"players"`
		} `json:"squad"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("re-reading the state: %v", err)
	}
	if len(raw.Squad.Players) == 0 {
		t.Fatal("the sample page produced no players, so this proves nothing")
	}
	for i, p := range raw.Squad.Players {
		v, ok := p["minutes"]
		if !ok {
			t.Fatalf("player %d carries no minutes field at all", i)
		}
		var n float64
		if err := json.Unmarshal(v, &n); err != nil {
			t.Fatalf("player %d: minutes is %s, which is not a number. The client rounds "+
				"and sorts on this; a string here renders NaN and orders nothing.", i, v)
		}
	}
}

// TestNewsCarriesBothFreshnessLinesAsGiven pins that News.Checked and News.ReadChecked
// are copied from the caller rather than reformatted or recomputed — this package has no
// clock of its own to compute "ago" against, and app.js:1388's own argument for why the
// client may not compute staleness applies just as much to this package.
func TestNewsCarriesBothFreshnessLinesAsGiven(t *testing.T) {
	s, err := Build(Input{
		Page: samplePage(), Boot: &fpl.Bootstrap{}, Now: pinned,
		NewsChecked:     "FPL status last read 3 minutes ago · next read at 15:00",
		NewsReadChecked: "Team news last read 2 days ago",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.News.Checked != "FPL status last read 3 minutes ago · next read at 15:00" {
		t.Errorf("News.Checked = %q, was not carried through unchanged", s.News.Checked)
	}
	if s.News.ReadChecked != "Team news last read 2 days ago" {
		t.Errorf("News.ReadChecked = %q, was not carried through unchanged", s.News.ReadChecked)
	}
}

// TestNewsItemsCoverBothSourcesAndOnlyAttachEffectWhereOneWasGiven is the shape test for
// State.News.Items: an FPL-status row drawn off a flagged squad player, a REPORTED row per
// live override, and an Effect ONLY on the row whose code Input.OverrideEffects named — a
// lock changes which squad is built, not this player's own score, so it must come back
// with no Effect rather than a zero that would read as "no effect" instead of "not
// applicable".
func TestNewsItemsCoverBothSourcesAndOnlyAttachEffectWhereOneWasGiven(t *testing.T) {
	p := samplePage()
	p.Squad.Players[0].AvailabilityFactor = 0.75
	p.Squad.Players[0].Status = "doubtful"
	p.Squad.Players[0].News = "75% chance of playing — knock"

	s, err := Build(Input{
		Page: p, Boot: &fpl.Bootstrap{}, Now: pinned,
		OverrideEffects: map[int]Effect{
			111: {Label: "pts a week", Was: "4.20", Now: "3.15", Direction: "down"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var fpl_, reported int
	var minutesRow, lockRow *NewsItem
	for i := range s.News.Items {
		it := &s.News.Items[i]
		switch it.Source {
		case "FPL":
			fpl_++
			if it.Body != "75% chance of playing — knock" {
				t.Errorf("FPL row body = %q", it.Body)
			}
		case "REPORTED":
			reported++
			switch it.PlayerCode {
			case 111:
				minutesRow = it
			case 333:
				lockRow = it
			}
		default:
			t.Errorf("unexpected news source %q", it.Source)
		}
	}
	if fpl_ != 1 {
		t.Errorf("%d FPL-status rows, want exactly 1 (only Kinsky is flagged)", fpl_)
	}
	if reported != 2 {
		t.Errorf("%d REPORTED rows, want exactly 2 (both live overrides)", reported)
	}
	if minutesRow == nil {
		t.Fatal("no REPORTED row for code 111 (the minutes override)")
	}
	if minutesRow.Effect == nil {
		t.Fatal("the minutes override's row carries no Effect, but one was given for its code")
	}
	if minutesRow.Effect.Was != "4.20" || minutesRow.Effect.Now != "3.15" || minutesRow.Effect.Direction != "down" {
		t.Errorf("Effect = %+v, want the exact values passed in", *minutesRow.Effect)
	}
	if lockRow == nil {
		t.Fatal("no REPORTED row for code 333 (the lock override)")
	}
	if lockRow.Effect != nil {
		t.Errorf("the lock override's row carries an Effect (%+v); a lock does not change "+
			"this player's own score, so it must stay absent, not a zeroed struct", *lockRow.Effect)
	}
}

// TestChipWindowReportsWhatTheScheduleHasSpentAlready is the view-model half of the
// app.js:625 fix: the window's own last gameweek and how many chips are unspent, agreeing
// with analysis.Engine.ChipWindowStatusAt regardless of which gameweek the rail is
// currently showing.
func TestChipWindowReportsWhatTheScheduleHasSpentAlready(t *testing.T) {
	p := samplePage()
	p.Weeks = []analysis.WeekView{
		{Event: 15, Formation: "3-5-2", Expected: 52.3},
		{Event: 25, Formation: "3-5-2", Expected: 50.1},
	}
	boot := &fpl.Bootstrap{
		Events: []fpl.Event{{ID: 15, IsNext: true}},
		Chips: []fpl.Chip{
			{Name: "wildcard", StartEvent: 2, StopEvent: 19},
			{Name: "freehit", StartEvent: 2, StopEvent: 19},
			{Name: "bboost", StartEvent: 1, StopEvent: 19},
			{Name: "3xc", StartEvent: 1, StopEvent: 19},
			{Name: "wildcard", StartEvent: 20, StopEvent: 38},
			{Name: "freehit", StartEvent: 20, StopEvent: 38},
			{Name: "bboost", StartEvent: 20, StopEvent: 38},
			{Name: "3xc", StartEvent: 20, StopEvent: 38},
		},
	}
	var chips analysis.ChipSchedule
	chips.First.BenchBoost = 5 // played behind GW15, outside "current + upcoming"

	s, err := Build(Input{Page: p, Boot: boot, Cfg: config.Config{}, Now: pinned, Chips: chips})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Gameweeks) != 2 {
		t.Fatalf("%d gameweeks, want 2", len(s.Gameweeks))
	}
	first := s.Gameweeks[0].ChipWindow
	if first == nil {
		t.Fatal("GW15 carries no chip window")
	}
	if first.EndsGW != 19 || first.Size != 4 || first.Remaining != 3 {
		t.Errorf("GW15 window = %+v, want EndsGW 19, Size 4, Remaining 3 (bench boost "+
			"already spent at GW5, invisible on the rail alone)", *first)
	}
	second := s.Gameweeks[1].ChipWindow
	if second == nil {
		t.Fatal("GW25 carries no chip window")
	}
	if second.EndsGW != 38 || second.Remaining != 4 {
		t.Errorf("GW25 window = %+v, want EndsGW 38, Remaining 4 (second window untouched)", *second)
	}
}
