package main

import (
	"net/http"
	"testing"
)

// The vs-model readout was comparing a squad's own eleven against a COPY of that same
// eleven -- app.js line 332 set S.modelXi from sq.xi, the reader's own arrangement, not the
// model's independent answer -- so the HUD always read a difference of 0 and always printed
// "matches our pick", even for a squad wildly different from what the model would pick.
//
// buildSquadPage now computes analysis.Squad.XIChanges/XIGap once, for whichever of the
// reload/Optimize/varied-build paths produced the squad (see page.go's own comment beside
// `mark("optimize")`). These two tests are the positive and negative control, run through the
// real HTTP surface -- fixtureServer's session store and /api/state -- rather than a bare call
// into squadFromCodes, because that surface is where the reader's arrangement and the model's
// own answer actually meet.

// TestTheVsModelReadoutMatchesAnArrangementTheModelPicked is the negative control: a stored
// eleven that is already the model's own pick for its fifteen must report XIChanges == 0 and
// XIGap == 0 -- not nil, which would mean the comparison never ran, and not some nonzero drift
// manufactured by asking a second time.
func TestTheVsModelReadoutMatchesAnArrangementTheModelPicked(t *testing.T) {
	s := fixtureServer(t)

	fresh := getWith(t, s, routeState, nil)
	if len(fresh.Squad.Players) != 15 {
		t.Fatalf("the opening squad has %d players", len(fresh.Squad.Players))
	}
	if fresh.Squad.XIChanges == nil {
		t.Fatal("XIChanges is nil on the opening squad -- the comparison did not run for a " +
			"freshly built squad, which already IS the model's own pick")
	}
	if *fresh.Squad.XIChanges != 0 {
		t.Fatalf("a freshly built squad reports XIChanges = %d against its own pick, want 0",
			*fresh.Squad.XIChanges)
	}
	if fresh.Squad.XIGap == nil || *fresh.Squad.XIGap != 0 {
		t.Fatalf("a freshly built squad reports XIGap = %v against its own pick, want 0",
			fresh.Squad.XIGap)
	}

	// Save that exact arrangement back explicitly -- the round trip through the reload path
	// (squadFromCodes) must not manufacture a difference out of an arrangement that already
	// matches by construction.
	codeOf := map[int]int{}
	var squad, xi, bench []int
	for _, p := range fresh.Squad.Players {
		codeOf[p.ID] = p.Code
		squad = append(squad, p.Code)
	}
	for _, id := range fresh.Squad.XI {
		xi = append(xi, codeOf[id])
	}
	for _, id := range fresh.Squad.Bench {
		bench = append(bench, codeOf[id])
	}

	w, saved := put(t, s, session{Version: sessionVersion, Squad: squad, XI: xi, Bench: bench}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the save answered %d: %s", w.Code, w.Body.String())
	}
	if saved.Squad.XIChanges == nil {
		t.Fatal("XIChanges is nil for a stored arrangement -- the comparison did not run")
	}
	if *saved.Squad.XIChanges != 0 {
		t.Errorf("an arrangement matching the model's own pick reports XIChanges = %d, want 0",
			*saved.Squad.XIChanges)
	}
	if saved.Squad.XIGap == nil || *saved.Squad.XIGap != 0 {
		t.Errorf("an arrangement matching the model's own pick reports XIGap = %v, want 0",
			saved.Squad.XIGap)
	}
}

// TestTheVsModelReadoutFlagsAMismatchedArrangement is the positive control: swap a starter for
// a strictly worse, same-position substitute -- a legal eleven the model would not pick -- and
// the comparison must say so, in both the count and the points. This is the exact scenario the
// shipped bug got wrong: the mismatch is real, and the old client-side comparison (against a
// copy of its own arrangement) always read 0 and always printed "matches our pick" anyway.
func TestTheVsModelReadoutFlagsAMismatchedArrangement(t *testing.T) {
	s := fixtureServer(t)

	fresh := getWith(t, s, routeState, nil)
	if len(fresh.Squad.Players) != 15 {
		t.Fatalf("the opening squad has %d players", len(fresh.Squad.Players))
	}

	byID := map[int]struct {
		code  int
		pos   string
		score float64
	}{}
	for _, p := range fresh.Squad.Players {
		byID[p.ID] = struct {
			code  int
			pos   string
			score float64
		}{p.Code, p.Pos, p.XP}
	}

	// Find a starter and a same-position substitute where the substitute scores less --
	// benching the starter for him is a legal, strictly worse swap the model would not make.
	// Every pair is tried rather than the first, so a fixture with no matching pair fails
	// loudly instead of silently skipping the assertion.
	var outID, inID int
outer:
	for _, x := range fresh.Squad.XI {
		xp := byID[x]
		if xp.pos == "GKP" {
			continue // avoid re-deriving keeper eligibility rules for one swap
		}
		for _, b := range fresh.Squad.Bench {
			bp := byID[b]
			if bp.pos == xp.pos && bp.score < xp.score {
				outID, inID = x, b
				break outer
			}
		}
	}
	if outID == 0 {
		t.Fatal("no same-position substitute scores less than a starter in this fixture -- " +
			"cannot build a mismatched arrangement")
	}

	var squad, xi, bench []int
	for _, p := range fresh.Squad.Players {
		squad = append(squad, p.Code)
	}
	for _, id := range fresh.Squad.XI {
		if id == outID {
			id = inID
		}
		xi = append(xi, byID[id].code)
	}
	for _, id := range fresh.Squad.Bench {
		if id == inID {
			id = outID
		}
		bench = append(bench, byID[id].code)
	}

	w, mismatched := put(t, s, session{Version: sessionVersion, Squad: squad, XI: xi, Bench: bench}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the save answered %d: %s", w.Code, w.Body.String())
	}
	if mismatched.Squad.XIChanges == nil {
		t.Fatal("XIChanges is nil for a stored, deliberately mismatched arrangement")
	}
	if *mismatched.Squad.XIChanges == 0 {
		t.Fatal("a deliberately mismatched arrangement reports XIChanges == 0 -- this is the " +
			"exact defect the vs-model readout shipped with")
	}
	if mismatched.Squad.XIGap == nil || *mismatched.Squad.XIGap <= 0 {
		t.Fatalf("a strictly worse arrangement reports XIGap = %v, want > 0", mismatched.Squad.XIGap)
	}
}
