package main

import (
	"net/http"
	"strconv"
	"testing"

	"armband/internal/viewmodel"
)

// Do the controls change the MODEL, or only the page?
//
// Every test in this file exists because a control shipped that changed a badge and nothing
// else, and the test written beside it asserted the badge. The rule these encode: assert the
// squad, the projection or the fifteen — never the label that describes them.

// TestBlockingAPlayerRemovesHimFromTheSquad.
//
// Blocking meant "never picked, anywhere" and left the player on the pitch. The stored
// fifteen outranked the optimiser request that carried the exclusion, and the only visible
// change was a badge — with three comments in the source asserting the opposite.
func TestBlockingAPlayerRemovesHimFromTheSquad(t *testing.T) {
	s := fixtureServer(t)

	first := getWith(t, s, routeState, nil)
	if len(first.Squad.Players) != 15 {
		t.Fatalf("the opening squad has %d players", len(first.Squad.Players))
	}
	// Someone in the eleven, so his absence is unmistakable.
	var victim, victimCode int
	for _, id := range first.Squad.XI {
		for _, p := range first.Squad.Players {
			if p.ID == id && p.Pos != "GKP" {
				victim, victimCode = p.ID, p.Code
				break
			}
		}
		if victim != 0 {
			break
		}
	}
	if victim == 0 {
		t.Fatal("no outfield starter to block")
	}

	// Sent exactly as the client sends it: the stored fifteen AND the new block.
	squad := make([]int, 0, 15)
	for _, p := range first.Squad.Players {
		squad = append(squad, p.Code)
	}
	w, after := put(t, s, session{
		Version: sessionVersion, Squad: squad, Exclude: []int{victimCode},
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the block answered %d: %s", w.Code, w.Body.String())
	}

	for _, p := range after.Squad.Players {
		if p.Code == victimCode {
			t.Fatalf("%s is still in the fifteen after being blocked. Block means never "+
				"picked, anywhere — the stored squad is outranking the correction.", p.Name)
		}
	}
	if len(after.Squad.Players) != 15 {
		t.Errorf("the squad is %d players after the block; the hole was not refilled",
			len(after.Squad.Players))
	}
}

// TestUnblockingAPlayerMakesHimAvailableAgain.
//
// A session exclusion was re-added after the dismissal filter, so the cross on it was inert
// by construction — and a blocked player is dropped from the market, so there was no row
// left to open his sheet from either. The only recovery was deleting the cookie.
//
// "Available" is the assertion, not "in the market": the model is free to answer an unblock
// by BUYING him, and on the committed capture it does. Asserting the market row would then
// fail on a correct unblock, which is the mirror of the defect being pinned.
func TestUnblockingAPlayerMakesHimAvailableAgain(t *testing.T) {
	s := fixtureServer(t)

	offered := func(st viewmodel.State, code int) bool {
		for _, r := range st.Market.Rows {
			if r.Player.Code == code {
				return true
			}
		}
		for _, p := range st.Squad.Players {
			if p.Code == code {
				return true
			}
		}
		return false
	}

	first := getWith(t, s, routeState, nil)
	if len(first.Market.Rows) == 0 {
		t.Fatal("the fixture market is empty")
	}
	victim := first.Market.Rows[0].Player.Code

	w, blocked := put(t, s, session{Version: sessionVersion, Exclude: []int{victim}}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the block answered %d", w.Code)
	}
	if offered(blocked, victim) {
		t.Fatal("a blocked player is still offered")
	}

	// Now clear it, the way the page does: the exclusion is still in the session and the
	// dismissal names the same player.
	w2, cleared := put(t, s, session{
		Version: sessionVersion, Exclude: []int{victim}, Dismissed: []int{victim},
	}, sessionCookie(t, w))
	if w2.Code != http.StatusOK {
		t.Fatalf("the unblock answered %d: %s", w2.Code, w2.Body.String())
	}
	if !offered(cleared, victim) {
		t.Error("the player is available nowhere after his block was cleared. A block that " +
			"cannot be undone leaves the reader with no recovery but the cookie.")
	}
	// And the document must agree with the model, because the page redraws its badges from
	// exactly this list. A block the model is not applying, still drawn as a block, is the
	// same defect one layer up.
	if hasID(cleared.Session.Blocked, victim) {
		t.Errorf("the document still lists %d as blocked while the model has freed him: %v",
			victim, cleared.Session.Blocked)
	}
}

// TestTheBankIsNotZeroOnAReload.
//
// squadFromCodes built the squad by hand and never set Remaining, so every reload reported
// an empty bank — and the replacement picker spends exactly that figure, so it hid every
// affordable target and refused funded moves with the arithmetic on screen.
func TestTheBankIsNotZeroOnAReload(t *testing.T) {
	s := fixtureServer(t)

	fresh := getWith(t, s, routeState, nil)
	squad := make([]int, 0, 15)
	for _, p := range fresh.Squad.Players {
		squad = append(squad, p.Code)
	}

	w, saved := put(t, s, session{Version: sessionVersion, Squad: squad}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the save answered %d", w.Code)
	}
	reloaded := getWith(t, s, routeState, sessionCookie(t, w))

	// The same fifteen must be worth the same money however it was built.
	if saved.Squad.Cost != fresh.Squad.Cost {
		t.Errorf("the stored squad costs %.1f and the fresh one %.1f", saved.Squad.Cost, fresh.Squad.Cost)
	}
	for _, st := range []struct {
		name string
		bank float64
	}{{"the save", saved.Squad.Bank}, {"the reload", reloaded.Squad.Bank}} {
		if st.bank != fresh.Squad.Bank {
			t.Errorf("%s reports a bank of £%.1fm, the fresh build £%.1fm. The picker "+
				"spends this figure, so a wrong one refuses transfers the reader can afford.",
				st.name, st.bank, fresh.Squad.Bank)
		}
	}
}

// TestPlacingAChipChangesTheProjection.
//
// The placement went into the cookie, came back in the document, and was thrown away: the
// engine's plan is the config's, set once at startup. The button flickered on and off and
// the projection never moved, beside copy promising it would re-run under the chip's rules.
func TestPlacingAChipChangesTheProjection(t *testing.T) {
	s := fixtureServer(t)

	before := getWith(t, s, routeState, nil)
	if len(before.Gameweeks) == 0 {
		t.Fatal("no gameweeks to place a chip in")
	}
	// A bench boost, in the first week that allows one.
	var week int
	for _, g := range before.Gameweeks {
		for _, c := range g.Playable {
			if c.Key == "bboost" {
				week = g.Number
				break
			}
		}
		if week != 0 {
			break
		}
	}
	if week == 0 {
		t.Fatal("no gameweek offers a bench boost")
	}

	w, after := put(t, s, session{
		Version: sessionVersion,
		Chips:   map[string]string{strconv.Itoa(week): "bboost"},
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("placing the chip answered %d: %s", w.Code, w.Body.String())
	}

	var placed string
	for _, g := range after.Gameweeks {
		if g.Number == week {
			placed = g.Chip
		}
	}
	if placed == "" {
		t.Fatalf("gameweek %d reports no chip after one was placed there. The placement "+
			"reached the cookie and not the model.", week)
	}
	if placed != "Bench Boost" {
		t.Errorf("gameweek %d reports the chip as %q, want Bench Boost", week, placed)
	}
}
