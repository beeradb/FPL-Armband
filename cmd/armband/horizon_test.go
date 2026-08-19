package main

import (
	"net/http"
	"strconv"
	"testing"
)

// TestAPlannedWildcardDoesNotShortenTheNextReadersHorizon.
//
// The server holds one engine for every request. ApplyChipPlan shortens Weights.Horizon
// when a wildcard is planned — right for the squad being built, and permanent unless it is
// undone. So a reader who planned a wildcard for gameweek 3 truncated the engine to two
// gameweeks and left it there: for himself after he removed the chip, and for every other
// reader served by the same process.
//
// It was latent while the chip plan came only from config and never moved. It became
// reachable the moment a reader could place a chip, which is what this branch added.
func TestAPlannedWildcardDoesNotShortenTheNextReadersHorizon(t *testing.T) {
	s := fixtureServer(t)

	before := getWith(t, s, routeState, nil)
	if before.Horizon <= 1 {
		t.Fatalf("the fixture serves a horizon of %d, so this test cannot detect a "+
			"truncation", before.Horizon)
	}

	// A wildcard as early as the competition allows one, which is where the truncation
	// bites hardest.
	var week int
	for _, g := range before.Gameweeks {
		for _, c := range g.Playable {
			if c.Key == "wildcard" {
				week = g.Number
			}
		}
		if week != 0 {
			break
		}
	}
	if week == 0 {
		t.Skip("the fixture's gameweeks offer no wildcard, so nothing here can truncate")
	}

	w, planned := put(t, s, session{
		Version: sessionVersion,
		Chips:   map[string]string{strconv.Itoa(week): "wildcard"},
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("planning the wildcard answered %d: %s", w.Code, w.Body.String())
	}
	t.Logf("with a wildcard at GW%d the page serves a horizon of %d", week, planned.Horizon)

	// A different reader, with no cookie and no chips at all.
	after := getWith(t, s, routeState, nil)
	if after.Horizon != before.Horizon {
		t.Errorf("a reader with no chips is served a horizon of %d, and was served %d "+
			"before somebody else planned a wildcard. The chip plan mutated the shared "+
			"engine and nothing put it back.", after.Horizon, before.Horizon)
	}

	// And the same reader, having removed it. Same defect, one seat closer.
	w2, cleared := put(t, s, session{Version: sessionVersion}, sessionCookie(t, w))
	if w2.Code != http.StatusOK {
		t.Fatalf("clearing the chip answered %d: %s", w2.Code, w2.Body.String())
	}
	if cleared.Horizon != before.Horizon {
		t.Errorf("after removing the wildcard the reader is served a horizon of %d, not "+
			"the %d they started with", cleared.Horizon, before.Horizon)
	}
}
