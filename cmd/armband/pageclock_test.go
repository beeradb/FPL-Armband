package main

import (
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// TestThePageClockDecidesStaleness is the liveness half of injecting a clock.
//
// Threading `now` through buildSquadPage only helps if the value arrives
// somewhere that reads it. The standing rule here is that a confinement check on
// a path which cannot carry the effect confirms nothing, and must be paired with
// a check that MOVES — so this asserts the override's staleness flips purely on
// the clock, with the config, the engine and the squad all held fixed.
//
// The threshold itself is config's (roster.go: stale at seven days), and it is
// deliberately not restated here. This test would still be honest if the rule
// changed to five days or ten; what it pins is that the page asks the clock at
// all, which is the thing a refactor can silently drop.
func TestThePageClockDecidesStaleness(t *testing.T) {
	e := &analysis.Engine{Boot: &fpl.Bootstrap{Elements: []fpl.Element{
		{ID: 5, Code: 456, WebName: "Saka"},
	}}}
	squad := []analysis.PlayerMetrics{{ID: 5, Name: "Saka", Position: "MID"}}

	checked := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	cfg := config.Config{}
	cfg.Roster.Lock = []config.RosterOverride{{
		Code: 456, Name: "Saka", Reason: "must not sell",
		SetOn:       checked.Format("2006-01-02"),
		LastChecked: checked.Format("2006-01-02"),
	}}

	fresh, _, _ := pageOverrides(cfg, e, squad, checked)
	if fresh[5].NeedsCheck {
		t.Error("an override checked today reads as needing a re-check; " +
			"the page is not reading the clock it was handed")
	}

	// Far enough past any plausible threshold that this test does not become a
	// second copy of the rule's constant.
	late, _, _ := pageOverrides(cfg, e, squad, checked.AddDate(0, 0, 60))
	if !late[5].NeedsCheck {
		t.Error("an override last checked 60 days ago reads as fresh; " +
			"the staleness rule is not seeing the injected clock")
	}
	if late[5].CheckAge != 60 {
		t.Errorf("CheckAge is %d days, want 60 — the age is measured against "+
			"something other than the clock the caller passed", late[5].CheckAge)
	}
}

// TestTheServerClockDefaultsToTheWallClock pins the nil case, because the whole
// point of a nil-means-real clock is that every non-test construction of
// squadServer keeps working without saying anything.
func TestTheServerClockDefaultsToTheWallClock(t *testing.T) {
	var s squadServer
	before := time.Now()
	got := s.now()
	if got.Before(before) || got.After(time.Now()) {
		t.Errorf("a squadServer with no clock returned %v, which is not now", got)
	}

	pinned := time.Date(2026, 8, 21, 17, 30, 0, 0, time.UTC)
	s.clock = func() time.Time { return pinned }
	if !s.now().Equal(pinned) {
		t.Errorf("an injected clock returned %v, want %v", s.now(), pinned)
	}
}
