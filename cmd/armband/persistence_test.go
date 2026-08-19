package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"armband/internal/viewmodel"
)

// The persistence contract, tested through the HTTP surface rather than the cookie.
//
// The question is not "does a cookie get written" — it did before, and a reader's work
// still vanished on reload, because nothing the page changed ever reached it. The question
// is whether a change made through the API is still there on the next request, and whether
// the model has actually been told.

// put sends a session and returns the recomputed state along with the response.
func put(t *testing.T, s *squadServer, sess session, cookie *http.Cookie) (*httptest.ResponseRecorder, viewmodel.State) {
	t.Helper()
	body, err := json.Marshal(sess)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PUT", routeSession, bytes.NewReader(body))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Armband-Token", s.token)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var st viewmodel.State
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
			t.Fatalf("the session answer did not decode: %v", err)
		}
	}
	return w, st
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie in the response")
	return nil
}

func getWith(t *testing.T, s *squadServer, path string, cookie *http.Cookie) viewmodel.State {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Host = "127.0.0.1:8080"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d: %s", path, w.Code, w.Body.String())
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// TestATeamSurvivesAReload is the whole point of the session store.
//
// It arranges a team that is NOT the one the model picked — a player moved out of the
// eleven — saves it, and reads it back as a fresh request would. The arrangement has to
// come back exactly, not be re-derived: if the answer were recomputed with BestXI the
// reader would drag a player to the bench, reload, and find him starting again with
// nothing to explain it.
func TestATeamSurvivesAReload(t *testing.T) {
	s := fixtureServer(t)

	first := getWith(t, s, routeState, nil)
	if len(first.Squad.Players) != 15 {
		t.Fatalf("the opening state has %d players", len(first.Squad.Players))
	}

	codeOf := map[int]int{}
	for _, p := range first.Squad.Players {
		codeOf[p.ID] = p.Code
	}

	// Swap a starter for a substitute in the same position, which is a legal change the
	// model did not make and which keeps the shape valid without any further reasoning.
	//
	// Every pair is tried rather than the first: taking the first starter and hoping the
	// bench matches his position made this skip, and a test that skips proves nothing.
	var out, in int
	for _, x := range first.Squad.XI {
		for _, b := range first.Squad.Bench {
			if posOf(first, x) != "GKP" && posOf(first, x) == posOf(first, b) {
				out, in = x, b
				break
			}
		}
		if out != 0 {
			break
		}
	}
	if out == 0 || in == 0 {
		t.Fatal("no starter shares a position with any substitute, so there is no " +
			"like-for-like swap to test. That is a fixture problem, not a pass.")
	}

	xi := []int{}
	for _, id := range first.Squad.XI {
		if id == out {
			id = in
		}
		xi = append(xi, codeOf[id])
	}
	bench := []int{}
	for _, id := range first.Squad.Bench {
		if id == in {
			id = out
		}
		bench = append(bench, codeOf[id])
	}
	squad := []int{}
	for _, p := range first.Squad.Players {
		squad = append(squad, p.Code)
	}

	w, saved := put(t, s, session{
		Version: sessionVersion, Squad: squad, XI: xi, Bench: bench,
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT %s answered %d: %s", routeSession, w.Code, w.Body.String())
	}
	if !hasID(saved.Squad.XI, in) {
		t.Errorf("the answer to the save does not field the substituted player")
	}

	// And now the reload: a fresh request carrying only the cookie.
	back := getWith(t, s, routeState, sessionCookie(t, w))
	if !hasID(back.Squad.XI, in) {
		t.Errorf("after a reload the eleven is %v; the reader's arrangement was not "+
			"restored, so their work was lost", back.Squad.XI)
	}
	if hasID(back.Squad.XI, out) {
		t.Errorf("after a reload the benched player is starting again — the arrangement " +
			"was re-derived from the model rather than read back")
	}
	if !back.Session.Saved {
		t.Error("the state does not report itself as a saved team, so the page cannot say so")
	}
}

func posOf(st viewmodel.State, id int) string {
	for _, p := range st.Squad.Players {
		if p.ID == id {
			return p.Pos
		}
	}
	return ""
}

func hasID(ids []int, id int) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// TestDismissingAnOverrideChangesTheModel is bug 1.
//
// The remove button used to filter a JavaScript array: the row vanished and the model went
// on applying the correction, so the squad did not move. That is what "nothing gets
// updated" looked like — the only thing that had happened was the row disappearing.
//
// The assertion is therefore not that the row goes. It is that the OVERRIDE stops binding.
func TestDismissingAnOverrideChangesTheModel(t *testing.T) {
	s := fixtureServer(t)

	before := getWith(t, s, routeState, nil)
	if len(before.Overrides.Live) == 0 {
		t.Fatal("the fixture carries no overrides, so this proves nothing")
	}
	var target viewmodel.Override
	for _, o := range before.Overrides.Live {
		if o.Code != 0 {
			target = o
			break
		}
	}
	if target.Code == 0 {
		t.Fatal("no override with a player to clear")
	}

	w, after := put(t, s, session{Version: sessionVersion, Dismissed: []int{target.Code}}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT answered %d: %s", w.Code, w.Body.String())
	}

	for _, o := range after.Overrides.Live {
		if o.Code == target.Code {
			t.Errorf("%s is still binding after being dismissed", o.Player)
		}
	}
	if len(after.Overrides.Live) >= len(before.Overrides.Live) {
		t.Errorf("the override count went from %d to %d; dismissing changed nothing",
			len(before.Overrides.Live), len(after.Overrides.Live))
	}

	// And it survives the reload, or the reader dismisses it again on every visit.
	back := getWith(t, s, routeState, sessionCookie(t, w))
	for _, o := range back.Overrides.Live {
		if o.Code == target.Code {
			t.Errorf("%s came back after a reload", o.Player)
		}
	}

	// The FILE is untouched. A browser must not edit the standing record; that is what
	// `serve -persist` is for.
	if len(s.cfg.Roster.Minutes)+len(s.cfg.Roster.Exclude)+len(s.cfg.Roster.Lock) == 0 {
		t.Error("dismissing emptied the real config; a session must not edit the record")
	}
}

// TestOptimizeReturnsTheModelsBest is bug 3's other half.
func TestOptimizeReturnsTheModelsBest(t *testing.T) {
	s := fixtureServer(t)

	varied := getWith(t, s, routeState, nil)
	if varied.Session.Optimised {
		t.Fatal("the opening squad reports itself as optimised; there is no variety to test")
	}

	w, best := put(t, s, session{Version: sessionVersion, Optimised: true}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT answered %d: %s", w.Code, w.Body.String())
	}
	if !best.Session.Optimised {
		t.Error("after Optimize the state does not report itself optimised")
	}
	// The optimum must be at least as good as the varied squad. It is the same objective
	// answered without two players removed from it, so anything else means the variety is
	// not what it claims to be.
	if best.Squad.Expected < varied.Squad.Expected-0.0001 {
		t.Errorf("the optimised squad projects %.2f against the varied squad's %.2f — "+
			"the variety is not a constrained optimum, it is just a worse answer",
			best.Squad.Expected, varied.Squad.Expected)
	}
	if best.Squad.Expected == varied.Squad.Expected {
		t.Logf("the varied and optimal squads project identically (%.2f); the seed happened "+
			"to exclude two players the optimiser could replace exactly", best.Squad.Expected)
	}
}

// TestTheSessionRouteRefusesWithoutTheToken pins that a write is a write.
func TestTheSessionRouteRefusesWithoutTheToken(t *testing.T) {
	s := fixtureServer(t)
	req := httptest.NewRequest("PUT", routeSession, strings.NewReader(`{"v":1}`))
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("an untokened PUT answered %d, want 403 — any page in the reader's "+
			"browser could otherwise rearrange his team", w.Code)
	}
}

// TestTheSessionRouteRefusesAnUnknownPlayer pins the validation.
func TestTheSessionRouteRefusesAnUnknownPlayer(t *testing.T) {
	s := fixtureServer(t)
	w, _ := put(t, s, session{Version: sessionVersion, Lock: []int{424242}}, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("a PUT naming an unknown code answered %d, want 400 — it would render "+
			"as a nameless row the reader cannot clear", w.Code)
	}
}
