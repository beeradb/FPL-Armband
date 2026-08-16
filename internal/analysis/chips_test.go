package analysis

import (
	"context"
	"testing"
	"time"

	"armband/internal/fpl"
)

// one is a schedule holding a single set, which is what most of these cases are
// about — the second set has its own tests in chipschedule_test.go.
func one(p ChipPlan) ChipSchedule { return ChipSchedule{First: p} }

func chipEngine(t *testing.T, plan ChipSchedule) *Engine {
	t.Helper()
	c := fpl.New(t.TempDir(), 24*time.Hour)
	ctx := context.Background()
	boot, err := c.Bootstrap(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	fx, err := c.Fixtures(ctx)
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	e := NewEngine(boot, fx, DefaultWeights())
	e.Chips = plan
	return e
}

// Only one chip set should be reported, and each chip should appear once.
func TestChipWindowsReportCurrentHalfOnly(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))
	seen := map[string]int{}
	for _, w := range e.ChipWindows() {
		seen[w.Name]++
		if w.Start > w.Stop {
			t.Errorf("%s has an inverted window GW%d-%d", w.Label, w.Start, w.Stop)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no chip windows reported")
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("chip %s reported %d times; expected one window per chip", name, n)
		}
	}
}

func TestChipPlanValidation(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))

	// Two chips in the same gameweek is illegal.
	problems := e.ValidateChipPlan(one(ChipPlan{BenchBoost: 5, TripleCaptain: 5}))
	var sawClash bool
	for _, p := range problems {
		if contains(p, "only one chip may be played per gameweek") {
			sawClash = true
		}
	}
	if !sawClash {
		t.Errorf("two chips in GW5 not flagged; got %v", problems)
	}

	// A wildcard in GW1 is outside its GW2-19 window.
	problems = e.ValidateChipPlan(one(ChipPlan{Wildcard: 1}))
	var sawWindow bool
	for _, p := range problems {
		if contains(p, "Wildcard planned for GW1") {
			sawWindow = true
		}
	}
	if !sawWindow {
		t.Errorf("wildcard in GW1 not flagged as out of window; got %v", problems)
	}
}

// A fully-planned first set must produce NO problems at all.
//
// The shipped config.json carries a flat four-chip plan, which loads as the first
// set. A first draft of the two-set validation reported every unplanned
// SECOND-set chip as "unplanned and expires after GW38" — and in a two-set season
// those windows stop at 38, so all four fired from GW1 onward. Four permanent
// lines on a legal plan, where the old code reported none, printed by
// `armband brief` and replayed to the agent in `get_chip_plan`'s issues array
// on every call. The comment above the loop claimed to be preventing exactly
// that. Found by review.
func TestAFullyPlannedCurrentSetIsClean(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))

	// Placed inside each chip's own current window, whatever the season says
	// those are, and in distinct gameweeks — the plan has to be legal for the
	// absence of problems to mean anything.
	var plan ChipSchedule
	used := map[int]bool{}
	for _, w := range e.ChipWindows() {
		gw := w.Start
		for used[gw] && gw < w.Stop {
			gw++
		}
		used[gw] = true
		if err := plan.Set(w.Name, gw); err != nil {
			t.Fatalf("placing %s: %v", w.Name, err)
		}
	}
	if len(plan.All()) == 0 {
		t.Skip("no chip windows are open, so there is nothing to plan")
	}

	if problems := e.ValidateChipPlan(plan); len(problems) > 0 {
		t.Errorf("a fully-planned current set reported %d problem(s):\n  %v\n\n"+
			"Every chip in play is planned, inside its own window, in a distinct "+
			"gameweek. Anything reported here is noise on a legal plan, and it is "+
			"replayed to the agent on every call.", len(problems), problems)
	}
}

// A legacy flat plan with a second-half chip must not be called out-of-window.
//
// `UnmarshalJSON` reads a flat plan wholly as set 1, because it carries no set
// information. Checking a set-1 slot only against the set-1 window then made
// "bench boost GW34" — legal, and the form docs/configuration.md tells a user to
// write — report as outside its GW1-19 window. That is a FALSE error and a
// regression against the behaviour this function had when it read the current
// set. The chip is in a legal week and filed under the wrong set, which is a
// different problem with a different fix. Found by review.
func TestASecondHalfChipInAFlatPlanIsNotCalledOutOfWindow(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))

	windows := e.chipWindowsByKind()["bboost"]
	if len(windows) < 2 {
		t.Skip("this season grants one set of bench boosts; nothing to mis-file")
	}
	second := windows[1]

	// A flat plan, exactly as UnmarshalJSON would produce it from config.json.
	plan := ChipSchedule{First: ChipPlan{BenchBoost: second.Start}}
	for _, p := range e.ValidateChipPlan(plan) {
		if contains(p, "but its window is") {
			t.Errorf("a bench boost at GW%d, inside the real GW%d-%d window, was "+
				"reported as out of window: %q", second.Start, second.Start, second.Stop, p)
		}
	}
}

// A wildcard shortens the horizon the current squad must serve.
func TestWildcardShortensHorizon(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))
	full, why := e.EffectiveHorizon(one(ChipPlan{}))
	if why != "" || full != e.Weights.Horizon {
		t.Errorf("unplanned chips should leave the horizon alone, got %d (%s)", full, why)
	}

	short, why := e.EffectiveHorizon(one(ChipPlan{Wildcard: 3}))
	if short != 2 {
		t.Errorf("wildcard at GW3 from GW1 should give a 2-gameweek horizon, got %d", short)
	}
	if why == "" {
		t.Error("horizon reduction reported without an explanation")
	}
}

// TestAFreeHitDoesNotShortenTheHorizon — the chip fields a separate temporary
// fifteen for one gameweek and hands the permanent squad straight back, so the
// squad still has to be good afterwards.
//
// This is a regression test for a disagreement rather than for a crash. The
// function truncated at a free hit as well as a wildcard, while
// `backtest.SimConfig.chipCredit` deliberately did not, so two implementations of
// "which chips end this squad's life" gave different answers about the same chip.
// The week a free hit removes is modelled by `ApplyFreeHitToScoring`, and
// counting it here as well both double-counted it and got its shape wrong.
func TestAFreeHitDoesNotShortenTheHorizon(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))
	full := e.Weights.Horizon

	got, why := e.EffectiveHorizon(one(ChipPlan{FreeHit: 3}))
	if got != full {
		t.Errorf("free hit at GW3 shortened the horizon to %d (%q) — it replaces one "+
			"gameweek's eleven, not the squad, and the fifteen is handed back at GW4",
			got, why)
	}
	if why != "" {
		t.Errorf("free hit reported a horizon reduction: %q", why)
	}

	// And the wildcard still wins when both are planned, because that one really
	// does end the squad.
	both, why := e.EffectiveHorizon(one(ChipPlan{FreeHit: 2, Wildcard: 4}))
	if both != 3 {
		t.Errorf("free hit GW2 + wildcard GW4 from GW1 should give 3 gameweeks, got %d (%q)",
			both, why)
	}
}

// A bench boost inside the horizon must raise the bench weight.
func TestBenchBoostRaisesBenchWeight(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))
	base, why := e.SuggestBenchWeight(one(ChipPlan{}))
	if why != "" || base != e.Weights.BenchWeight {
		t.Errorf("no bench boost should leave bench weight alone, got %.3f", base)
	}

	boosted, why := e.SuggestBenchWeight(one(ChipPlan{BenchBoost: 2}))
	if boosted <= e.Weights.BenchWeight {
		t.Errorf("bench boost should raise bench weight, got %.3f", boosted)
	}
	if why == "" {
		t.Error("bench weight raised without an explanation")
	}

	// A bench boost beyond the horizon should not affect this squad.
	far, _ := e.SuggestBenchWeight(one(ChipPlan{BenchBoost: 30}))
	if far != e.Weights.BenchWeight {
		t.Errorf("a bench boost outside the horizon should not change bench weight, got %.3f", far)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// TestSkipGameweeksExtendsTheWindow — a skipped gameweek must be dropped from
// the fixture run *and* replaced, so the horizon still covers the number of
// gameweeks it claims to. Shortening it instead would make every player around
// a free hit look worse than he is, which is the opposite of the intent.
func TestSkipGameweeksExtendsTheWindow(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var team int
	for _, tm := range e.Boot.Teams {
		if len(e.TeamFixtures(tm.ID, 6)) >= 6 {
			team = tm.ID
			break
		}
	}
	if team == 0 {
		t.Skip("no team with six upcoming fixtures")
	}

	before := e.TeamFixtures(team, 5)
	skipped := before[1].Event
	e.SetSkipGameweeks([]int{skipped})
	after := e.TeamFixtures(team, 5)

	if len(after) != len(before) {
		t.Errorf("skipping one gameweek left %d fixtures, want %d — the window "+
			"shortened instead of extending", len(after), len(before))
	}
	for _, f := range after {
		if f.Event == skipped {
			t.Errorf("gameweek %d was skipped but still appears", skipped)
		}
	}
	if got := e.SkipGameweeks(); len(got) != 1 || got[0] != skipped {
		t.Errorf("SkipGameweeks reports %v, want [%d]", got, skipped)
	}

	// And it must be reversible.
	e.SetSkipGameweeks(nil)
	if len(e.TeamFixtures(team, 5)) != len(before) {
		t.Error("clearing the skip set did not restore the original window")
	}
}
