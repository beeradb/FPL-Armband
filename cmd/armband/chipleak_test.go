package main

import (
	"testing"

	"armband/internal/analysis"
)

// TestTheSiteServesNoChipPlanOfItsOwn pins that a reader inherits none of the owner's
// chip strategy.
//
// `config.json` has two consumers: the owner's own CLI and agent runs, and the server.
// Most of it is meant to be shared — the roster overrides ARE the published team-news
// research, and serving them is the point. `chip_plan` is the exception. It is one
// manager's strategy for one entry, and `applyTo` used to fold the reader's placements
// INTO it rather than starting from nothing, so every visitor who had never touched a
// chip was served the owner's plan.
//
// That was not cosmetic. `EffectiveHorizon` shortens the optimiser to the gameweeks
// before the next planned wildcard, so a reader's fifteen was built for a horizon
// truncated by a chip he had not planned, could not see, and had no way to remove.
//
// The assertion is on the whole schedule rather than a field, so a chip added to
// `ChipPlan` later cannot leak through a test that names only the four.
func TestTheSiteServesNoChipPlanOfItsOwn(t *testing.T) {
	s := fixtureServer(t)

	owner := *s.cfg
	owner.Chips = analysis.ChipSchedule{
		First:  analysis.ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9},
		Second: analysis.ChipPlan{Wildcard: 20, FreeHit: 36, BenchBoost: 38, TripleCaptain: 37},
	}
	if owner.Chips == (analysis.ChipSchedule{}) {
		t.Fatal("the fixture's owner plan is empty, so this test would pass on a broken build")
	}

	today := s.now().Format("2006-01-02")
	got := session{Version: sessionVersion}.applyTo(forPlanner(owner), s.engine, today)

	if got.Chips != (analysis.ChipSchedule{}) {
		t.Fatalf("a reader with no chips of his own was served the owner's plan: %+v", got.Chips)
	}
}

// TestAReadersOwnChipStillReachesThePlanner is the other half, and it is why the fix is
// a change of base rather than a deletion: the site still has to act on the chips the
// reader places himself. Without this, emptying the plan would look correct and would
// have silently disabled the chip buttons.
func TestAReadersOwnChipStillReachesThePlanner(t *testing.T) {
	s := fixtureServer(t)

	owner := *s.cfg
	owner.Chips = analysis.ChipSchedule{
		First: analysis.ChipPlan{Wildcard: 6, BenchBoost: 8},
	}

	today := s.now().Format("2006-01-02")
	sess := session{Version: sessionVersion, Chips: map[string]string{"12": "bboost"}}
	got := sess.applyTo(forPlanner(owner), s.engine, today)

	if got.Chips.First.BenchBoost != 12 {
		t.Fatalf("the reader placed a bench boost at GW12 and the planner got %+v", got.Chips)
	}
	if got.Chips.First.Wildcard != 0 {
		t.Fatalf("the owner's GW6 wildcard leaked alongside the reader's chip: %+v", got.Chips)
	}
}

// TestPersistKeepsTheOwnersOwnChipPlan is the other side of the strip, and it is the
// regression an over-eager fix introduces.
//
// `serve.go` deliberately skips `forPlanner` under `-persist`: there the page IS the
// owner's, he writes to `config.json` from it, and his own settings must bind. A first
// version of this fix stripped the plan inside `chipsInto` instead, which runs on every
// path — so it would have taken the owner's chips off his own page, silently, with the
// leak test still green.
func TestPersistKeepsTheOwnersOwnChipPlan(t *testing.T) {
	s := fixtureServer(t)

	owner := *s.cfg
	owner.Chips = analysis.ChipSchedule{
		First: analysis.ChipPlan{Wildcard: 6, BenchBoost: 8},
	}

	today := s.now().Format("2006-01-02")
	// -persist passes the config through UNSTRIPPED, exactly as serve.go does.
	got := session{Version: sessionVersion}.applyTo(owner, s.engine, today)

	if got.Chips.First.Wildcard != 6 || got.Chips.First.BenchBoost != 8 {
		t.Fatalf("under -persist the owner lost his own chip plan: %+v", got.Chips)
	}
}
