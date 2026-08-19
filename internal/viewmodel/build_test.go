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
	if !s.Session.Writable {
		t.Error("a page built with a token reports itself unwritable")
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
