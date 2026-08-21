package main

import (
	"testing"

	"armband/internal/fpl"
)

func TestImportWindowClosesWithNoEvents(t *testing.T) {
	importEvent, nextEvent, open := importWindow(nil)
	if open || importEvent != 0 || nextEvent != 0 {
		t.Errorf("importWindow(nil) = %d, %d, %v, want 0, 0, false", importEvent, nextEvent, open)
	}
}

// TestImportWindowClosesBeforeGW1Deadline is the pre-season case: only an IsNext event
// exists, naming gameweek 1, and nothing is IsCurrent yet because no gameweek has been
// played. This must close, since there is no squad anywhere to import — today's shipped
// GW1 behaviour.
func TestImportWindowClosesBeforeGW1Deadline(t *testing.T) {
	events := []fpl.Event{{ID: 1, IsNext: true}}
	importEvent, nextEvent, open := importWindow(events)
	if open || importEvent != 0 || nextEvent != 0 {
		t.Errorf("importWindow before GW1's deadline = %d, %d, %v, want 0, 0, false",
			importEvent, nextEvent, open)
	}
}

// TestImportWindowOpensRightAfterGW1Deadline pins the earliest open state: GW1 has just
// become IsCurrent (its deadline passed) and GW2 is IsNext.
func TestImportWindowOpensRightAfterGW1Deadline(t *testing.T) {
	events := []fpl.Event{
		{ID: 1, IsCurrent: true},
		{ID: 2, IsNext: true},
	}
	importEvent, nextEvent, open := importWindow(events)
	if !open {
		t.Fatal("importWindow did not open right after GW1's deadline")
	}
	if importEvent != 1 {
		t.Errorf("importEvent = %d, want 1 (GW1's own picks)", importEvent)
	}
	if nextEvent != 2 {
		t.Errorf("nextEvent = %d, want 2 (the gameweek being planned)", nextEvent)
	}
}

// TestImportWindowOpensMidSeason is the ordinary case for most of the season.
func TestImportWindowOpensMidSeason(t *testing.T) {
	events := []fpl.Event{
		{ID: 9, Finished: true},
		{ID: 10, IsCurrent: true},
		{ID: 11, IsNext: true},
	}
	importEvent, nextEvent, open := importWindow(events)
	if !open || importEvent != 10 || nextEvent != 11 {
		t.Errorf("importWindow mid-season = %d, %d, %v, want 10, 11, true",
			importEvent, nextEvent, open)
	}
}

// TestImportWindowClosesAtSeasonEnd — no event is IsNext once the season has finished, and
// that closes the gate rather than falling back to the last IsCurrent gameweek: there is
// nothing left to plan for, so the ordinary opening-squad flow is the right answer.
func TestImportWindowClosesAtSeasonEnd(t *testing.T) {
	events := []fpl.Event{
		{ID: 37, Finished: true},
		{ID: 38, Finished: true, IsCurrent: true},
	}
	importEvent, nextEvent, open := importWindow(events)
	if open || importEvent != 0 || nextEvent != 0 {
		t.Errorf("importWindow at season end = %d, %d, %v, want 0, 0, false",
			importEvent, nextEvent, open)
	}
}
