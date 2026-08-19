package main

import (
	"testing"

	"armband/internal/config"
)

// TestTheAnalysisLayersLocksDoNotBindThePlanner is the distinction between a correction to
// what the model KNOWS and a decision the model MADE.
//
// A minutes override is team news — "he is nailed, the record cannot see it yet" — and the
// planner wants it. A lock is the agent's own conclusion for its own weekly recommendation:
// build every squad around this player. Inheriting that would give the reader a squad built
// around somebody else's choice while presenting it as a suggestion, which is a constraint
// wearing a suggestion's clothes.
//
// The reader's own locks are a different thing entirely and are asserted below to survive.
func TestTheAnalysisLayersLocksDoNotBindThePlanner(t *testing.T) {
	s := fixtureServer(t)

	// A lock the analysis layer might plausibly have written: an expensive player the
	// optimiser would not otherwise buy, so his presence is evidence the lock bound.
	var dear config.RosterOverride
	var dearCode int
	best := 0.0
	for i := range s.engine.Boot.Elements {
		el := &s.engine.Boot.Elements[i]
		if p := float64(el.NowCost) / 10; p > best {
			best, dearCode = p, el.Code
			dear = config.RosterOverride{
				Code: el.Code, Name: el.WebName,
				Reason: "the agent decided the squad is built around him", SetOn: "2026-08-01",
			}
		}
	}
	if dearCode == 0 {
		t.Fatal("no players in the fixture")
	}

	cfg := *s.cfg
	cfg.Roster.Lock = []config.RosterOverride{dear}
	cfg.Roster.Minutes = append([]config.RosterOverride(nil), cfg.Roster.Minutes...)
	s.cfg = &cfg

	planner := s.effectiveCfgFrom(session{})
	if len(planner.Roster.Lock) != 0 {
		t.Errorf("the planner inherited %d config lock(s): %+v\n"+
			"A lock is the analysis layer's decision, not team news, and the reader is the "+
			"one choosing here.", len(planner.Roster.Lock), planner.Roster.Lock)
	}

	// Team news survives, because that is a correction to what the model can see.
	if len(planner.Roster.Minutes) != len(cfg.Roster.Minutes) {
		t.Errorf("the planner dropped %d minutes override(s); team news must survive",
			len(cfg.Roster.Minutes)-len(planner.Roster.Minutes))
	}

	// And the READER's own lock is his, so it binds.
	mine := s.effectiveCfgFrom(session{Lock: []int{dearCode}})
	if len(mine.Roster.Lock) != 1 || mine.Roster.Lock[0].Code != dearCode {
		t.Errorf("the reader's own lock did not bind: %+v", mine.Roster.Lock)
	}
}

// TestThePreviewScoresOverAShorterHorizon pins that the flag reaches the engine and that
// the client is told, rather than left to assume the configured window.
//
// It goes through applyPreviewHorizon rather than assigning Weights.Horizon, because the
// assignment was the part that was never in doubt. The earlier version set the field itself
// and would have passed just as well with the flag deleted.
func TestThePreviewScoresOverAShorterHorizon(t *testing.T) {
	s := fixtureServer(t)

	configured := s.engine.Weights.Horizon
	note := applyPreviewHorizon(s.engine, 4)
	if s.engine.Weights.Horizon != 4 {
		t.Fatalf("the flag left the engine on a horizon of %d", s.engine.Weights.Horizon)
	}
	if note == "" {
		t.Errorf("narrowing the horizon from %d to 4 said nothing. Every projection on the "+
			"page moves with this and the reader has no other way to know.", configured)
	}
	// Zero is how a reader keeps the configured window, and it must not be an error or a
	// narrowing to nothing.
	if again := applyPreviewHorizon(s.engine, 0); again != "" {
		t.Errorf("-horizon 0 changed something: %q", again)
	}
	if s.engine.Weights.Horizon != 4 {
		t.Errorf("-horizon 0 moved the engine to %d", s.engine.Weights.Horizon)
	}
	// And asking for the window it is already on says nothing rather than repeating itself.
	if again := applyPreviewHorizon(s.engine, 4); again != "" {
		t.Errorf("re-applying the same horizon announced a change: %q", again)
	}

	st := getWith(t, s, routeState, nil)
	if st.Horizon != 4 {
		t.Errorf("the state reports a horizon of %d, want 4. The page prints projections "+
			"over this window and cannot say so if it is not told.", st.Horizon)
	}
	for _, p := range st.Squad.Players {
		if len(p.Fixtures) > 4 {
			t.Errorf("%s carries %d fixtures on a four-gameweek horizon", p.Name, len(p.Fixtures))
			break
		}
	}
}
