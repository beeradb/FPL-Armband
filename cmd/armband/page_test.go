package main

import (
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// TestALockedPlayerKeepsHisLockBadgeUnderAMinutesOverride pins a review
// finding: pageOverrides fills each element's single badge slot from three
// lists in order, and the minutes list used to overwrite the lock — so a
// locked player with a minutes correction rendered an OFF lock button, and
// clicking it re-wrote the lock with the page's canned reason, clearing
// MustStart and the agent's provenance. The same overwrite lost an excluded
// player's EXCL badge, which the watchlist's skip set reads to keep him off
// the candidate list.
func TestALockedPlayerKeepsHisLockBadgeUnderAMinutesOverride(t *testing.T) {
	e := &analysis.Engine{Boot: &fpl.Bootstrap{Elements: []fpl.Element{
		{ID: 5, Code: 456, WebName: "Saka"},
	}}}
	squad := []analysis.PlayerMetrics{{ID: 5, Name: "Saka", Position: "MID"}}
	mins := 60.0

	cfg := config.Config{}
	cfg.Roster.Lock = []config.RosterOverride{{Code: 456, Name: "Saka", Reason: "must not sell"}}
	cfg.Roster.Minutes = []config.RosterOverride{{Code: 456, Name: "Saka", ExpectedMinutes: &mins}}
	bound, _, _ := pageOverrides(cfg, e, squad, time.Now())
	if ov, ok := bound[5]; !ok || ov.Kind != "lock" {
		t.Errorf("the badge slot is %+v; a minutes correction overwrote the lock, "+
			"which renders an OFF lock button on a locked player", ov)
	}

	cfg2 := config.Config{}
	cfg2.Roster.Exclude = []config.RosterOverride{{Code: 456, Name: "Saka"}}
	cfg2.Roster.Minutes = []config.RosterOverride{{Code: 456, Name: "Saka", ExpectedMinutes: &mins}}
	bound2, _, _ := pageOverrides(cfg2, e, squad, time.Now())
	if ov, ok := bound2[5]; !ok || ov.Kind != "exclude" {
		t.Errorf("the badge slot is %+v; an exclusion lost its slot to a minutes "+
			"correction, and the watchlist's skip set reads that slot", ov)
	}
}
