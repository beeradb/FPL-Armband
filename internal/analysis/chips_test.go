package analysis

import (
	"context"
	"reflect"
	"testing"
	"time"

	"armband/internal/fpl"
)

// one is a schedule holding a single set, which is what most of these cases are
// about — the second set has its own tests in chipschedule_test.go.
func one(p ChipPlan) ChipSchedule { return ChipSchedule{First: p} }

// upcomingGW is the gameweek `EffectiveHorizon` and `ValidateChipPlan` measure
// everything else against — `Boot.NextEvent().ID`, or 1 when there is no next
// event.
//
// ⚠️ Every chip gameweek in this file is expressed RELATIVE to it, and that is
// not tidiness. `chipEngine` fetches the LIVE bootstrap, so "the upcoming
// gameweek" moves with the real season — while these tests used to hardcode
// `Wildcard: 3` and assert a two-gameweek horizon, which is the right answer
// only while `upcomingGW == 1`. They passed all pre-season and went red the
// morning GW1 finished, on code nobody had touched, and took two other packages'
// CI down with them.
//
// Relative arithmetic beats skipping the window: the assertions stay live all
// season instead of going quiet for the ten months when a regression is most
// likely to reach them.
// ⚠️ Delegates rather than re-deriving. This was one of three spellings of
// "which gameweek is being decided"; `Engine.UpcomingGW` is now the only
// implementation, and its doc comment carries the rest of the story.
func upcomingGW(e *Engine) int { return e.UpcomingGW() }

func chipEngine(t *testing.T, plan ChipSchedule) *Engine {
	t.Helper()
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
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
	up := upcomingGW(e)

	// Two chips in the same gameweek is illegal. WHICH gameweek is immaterial —
	// only that both chips name the same one — so it is expressed relative to
	// now. Written as a literal GW5 it asserted the clash rule until GW5 went
	// by and then asserted it about a gameweek in the past, where the validator
	// has a different and more urgent complaint.
	clash := up + 1
	problems := e.ValidateChipPlan(one(ChipPlan{BenchBoost: clash, TripleCaptain: clash}))
	var sawClash bool
	for _, p := range problems {
		if contains(p, "only one chip may be played per gameweek") {
			sawClash = true
		}
	}
	if !sawClash {
		t.Errorf("two chips in GW%d not flagged; got %v", clash, problems)
	}

	// ⚠️ A genuinely FIXED gameweek, and the one case the relative rule does not
	// cover: the wildcard's window opens at GW2 because that is the competition's
	// rule, so GW1 is outside it in every week of every season. Named rather than
	// written inline so it reads as a boundary rather than as a date somebody
	// forgot to update — and so the guard in chipgameweekliteral_test.go, which
	// cannot tell the two apart, is answered explicitly.
	const beforeTheWildcardWindow = 1
	problems = e.ValidateChipPlan(one(ChipPlan{Wildcard: beforeTheWildcardWindow}))
	var sawProblem bool
	for _, p := range problems {
		if contains(p, "Wildcard planned for GW1") {
			sawProblem = true
		}
	}
	if !sawProblem {
		t.Errorf("wildcard in GW1 not flagged; got %v", problems)
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
	skipDuringLiveGW1Gap(t, e)

	// Placed inside each chip's own current window, whatever the season says
	// those are, and in distinct gameweeks — the plan has to be legal for the
	// absence of problems to mean anything.
	var plan ChipSchedule
	used := map[int]bool{}
	up := upcomingGW(e)
	for _, w := range e.ChipWindows() {
		// ⚠️ Not `w.Start` — a window that OPENED in the past is still current,
		// and placing a chip at its start plants it in a gameweek that has
		// already gone. That is what ValidateChipPlan rejected as "Triple Captain
		// planned for GW1, which has already passed" the morning GW1 finished:
		// the window still starts at 1 all season, so this test broke itself.
		gw := w.Start
		if gw < up {
			gw = up
		}
		for used[gw] && gw < w.Stop {
			gw++
		}
		if gw > w.Stop {
			continue // window has closed; nothing legal to place in it
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
	skipDuringLiveGW1Gap(t, e)
	full, why := e.EffectiveHorizon(one(ChipPlan{}))
	if why != "" || full != e.Weights.Horizon {
		t.Errorf("unplanned chips should leave the horizon alone, got %d (%s)", full, why)
	}

	// Two gameweeks out from whatever is upcoming, not a hardcoded GW3 — see
	// upcomingGW. EffectiveHorizon's span is `gw - nextGW`, so this asks for 2
	// in every week of the season rather than only in the first.
	wc := upcomingGW(e) + 2
	short, why := e.EffectiveHorizon(one(ChipPlan{Wildcard: wc}))
	if short != 2 {
		t.Errorf("wildcard at GW%d, two out from the upcoming GW%d, should give a "+
			"2-gameweek horizon, got %d", wc, upcomingGW(e), short)
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
	skipDuringLiveGW1Gap(t, e)
	full := e.Weights.Horizon

	// Relative to the upcoming gameweek, not a hardcoded GW3 — see upcomingGW.
	up := upcomingGW(e)
	got, why := e.EffectiveHorizon(one(ChipPlan{FreeHit: up + 2}))
	if got != full {
		t.Errorf("free hit at GW%d shortened the horizon to %d (%q) — it replaces one "+
			"gameweek's eleven, not the squad, and the fifteen is handed back the "+
			"week after", up+2, got, why)
	}
	if why != "" {
		t.Errorf("free hit reported a horizon reduction: %q", why)
	}

	// And the wildcard still wins when both are planned, because that one really
	// does end the squad. Three gameweeks out gives a span of 3.
	both, why := e.EffectiveHorizon(one(ChipPlan{FreeHit: up + 1, Wildcard: up + 3}))
	if both != 3 {
		t.Errorf("free hit GW%d + wildcard GW%d, from the upcoming GW%d, should give "+
			"3 gameweeks, got %d (%q)", up+1, up+3, up, both, why)
	}
}

// A bench boost inside the horizon must raise the bench weight.
//
// ⚠️ The gameweeks are RELATIVE to the upcoming one, as they are in every other
// test in this file. They were literals — `BenchBoost: 2` for the inside-the-
// horizon case — and a literal gameweek stops being inside the horizon the
// moment the season passes it. GW2's deadline passed on 2026-08-28 at 17:30 UTC
// and this test began failing at that minute, on a tree nobody had touched, for
// a model that was behaving correctly: a boost planned for a gameweek already
// played is outside the horizon, so declining to raise the bench weight was the
// right answer to the wrong question. `upcomingGW` is what the rest of the file
// uses and it cannot go stale.
//
// ⚠️ Its two siblings above already derived the gameweek this way; this was
// the odd copy out, which is why the failure looked like a code regression
// rather than the calendar moving. `chipgameweekliteral_test.go` now fails
// on a chip gameweek written as a literal in any live-API test here, so the
// next copy cannot be written silently.
func TestBenchBoostRaisesBenchWeight(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))
	up := upcomingGW(e)
	base, why := e.SuggestBenchWeight(one(ChipPlan{}))
	if why != "" || base != e.Weights.BenchWeight {
		t.Errorf("no bench boost should leave bench weight alone, got %.3f", base)
	}

	boosted, why := e.SuggestBenchWeight(one(ChipPlan{BenchBoost: up}))
	if boosted <= e.Weights.BenchWeight {
		t.Errorf("bench boost at the upcoming GW%d should raise bench weight, "+
			"got %.3f", up, boosted)
	}
	if why == "" {
		t.Error("bench weight raised without an explanation")
	}

	// A bench boost beyond the horizon should not affect this squad. Expressed
	// as the horizon's own far side rather than as 30, which is only "beyond"
	// while the season is young enough.
	far, _ := e.SuggestBenchWeight(one(ChipPlan{BenchBoost: up + e.Weights.Horizon + 1}))
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

// TestApplyChipPlanDoesNotRebuildFixtureIndex confirms that ApplyChipPlan does
// not call buildFixtureIndex a second time when shortening the horizon for a
// planned wildcard. The fixture index must not be rebuilt.
func TestApplyChipPlanDoesNotRebuildFixtureIndex(t *testing.T) {
	e := chipEngine(t, one(ChipPlan{}))
	up := upcomingGW(e)

	// Plan a wildcard inside the horizon so EffectiveHorizon shortens it.
	wc := up + 2
	if wc > 38 || e.Weights.Horizon <= 2 {
		t.Skip("no gameweek left at which a planned wildcard would shorten the horizon")
	}
	e.Chips = one(ChipPlan{Wildcard: wc})

	// Verify the plan will actually shorten the horizon.
	shortened, _ := e.EffectiveHorizon(e.Chips)
	if shortened >= e.Weights.Horizon {
		t.Skipf("wildcard at GW%d does not shorten horizon %d", wc, e.Weights.Horizon)
	}

	// Capture the current fixture index underlying pointers. If buildFixtureIndex
	// is called a second time, these will be replaced.
	oldByTeamUpcomingPtr := reflect.ValueOf(e.byTeamUpcoming).Pointer()
	oldUpcomingGWsPtr := reflect.ValueOf(e.upcomingGWs).Pointer()

	// Apply the chip plan, which shortens the horizon.
	req := &OptimizeRequest{}
	notes := e.ApplyChipPlan(req)
	if len(notes) == 0 {
		t.Error("ApplyChipPlan returned no notes, so the horizon change was not applied")
	}

	// Confirm the fixture index was NOT rebuilt. If buildFixtureIndex was called,
	// the underlying map and slice would have been replaced with new allocations.
	if newByTeamUpcomingPtr := reflect.ValueOf(e.byTeamUpcoming).Pointer(); newByTeamUpcomingPtr != oldByTeamUpcomingPtr {
		t.Error("ApplyChipPlan called buildFixtureIndex, allocating a new byTeamUpcoming map")
	}
	if newUpcomingGWsPtr := reflect.ValueOf(e.upcomingGWs).Pointer(); newUpcomingGWsPtr != oldUpcomingGWsPtr {
		t.Error("ApplyChipPlan called buildFixtureIndex, allocating a new upcomingGWs slice")
	}
}
