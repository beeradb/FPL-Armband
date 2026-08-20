package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
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

// TestTheSessionRouteGatesWritesByWhatTheyCanReach.
//
// The token used to be required unconditionally. It is now required where a save can reach
// config.json — under `-persist` — and a same-origin check stands in for it where a save can
// only reach the caller's own cookie. The public deployment has no way to hand a token to a
// reader, so an unconditional token meant the planner drew every control and refused every
// one of them.
//
// ⚠️ The token was doing a SECOND job, and this test exists mostly to pin the replacement.
// A cross-origin page cannot be allowed to write, `SameSite=Strict` notwithstanding: the
// reader's cookie is withheld, the server sees an empty session, and the answer's Set-Cookie
// replaces the real one. Destruction, not disclosure, and unrecoverable from the page.
func TestTheSessionRouteGatesWritesByWhatTheyCanReach(t *testing.T) {
	send := func(s *squadServer, method string, hdr map[string]string) int {
		req := httptest.NewRequest(method, routeSession, strings.NewReader(`{"v":1}`))
		req.Host = "127.0.0.1:8080"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)
		return w.Code
	}

	// A save that cannot reach the file needs no token — that is the whole point.
	s := fixtureServer(t)
	if got := send(s, "PUT", map[string]string{"Sec-Fetch-Site": "same-origin"}); got != http.StatusOK {
		t.Errorf("a same-origin PUT with no token answered %d, want 200. Without this the "+
			"public planner draws every control and refuses every one.", got)
	}

	// But a cross-origin one is refused, with or without a session of its own.
	for _, site := range []string{"cross-site", "same-site"} {
		if got := send(s, "PUT", map[string]string{"Sec-Fetch-Site": site}); got != http.StatusForbidden {
			t.Errorf("a %s PUT answered %d, want 403 — a page in the reader's browser "+
				"could otherwise replace their stored team", site, got)
		}
	}

	// POST is gone. A cross-origin POST with a simple content type is sent with NO
	// preflight, so accepting POST here would hand that page the write path whatever the
	// origin check said about requests it can see.
	if got := send(s, "POST", map[string]string{"Sec-Fetch-Site": "same-origin"}); got != http.StatusMethodNotAllowed {
		t.Errorf("POST answered %d, want 405. A cross-origin POST needs no preflight, so "+
			"it is the one method that must not be accepted here.", got)
	}

	// Under -persist a save reaches config.json, and there the token is the boundary —
	// same-origin is not enough, because the attacker that matters is a local process.
	p := fixtureServer(t)
	p.persist = true
	p.cfgPath = filepath.Join(t.TempDir(), "config.json")
	if got := send(p, "PUT", map[string]string{"Sec-Fetch-Site": "same-origin"}); got != http.StatusForbidden {
		t.Errorf("under -persist a tokenless PUT answered %d, want 403 — it would write a "+
			"standing override that binds every future agent run", got)
	}
	if got := send(p, "PUT", map[string]string{
		"Sec-Fetch-Site": "same-origin", "X-Armband-Token": p.token,
	}); got != http.StatusOK {
		t.Errorf("under -persist a tokened PUT answered %d, want 200", got)
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

// TestTheOverBudgetSquadIsServedRatherThanRefused verifies that the session endpoint
// accepts and returns a squad without validating the budget.
//
// This is deliberate: the budget is a fact about the reader's entry (not stored here), and
// an over-budget squad is a valid state to ask the optimizer about (wildcards, hypotheticals).
// The endpoint must not validate budget, only the integrity of the squad itself.
//
// The test pins the design choice documented in webroutes.go (validateSession, ~line
// 742): budget is NOT checked, so a reader can submit a squad for analysis even if it
// exceeds their available funds.
//
// ⚠️ A squad built from the reader's own current fifteen is within budget BY CONSTRUCTION
// — the server already validated it once to get it there — so a test that reuses it would
// pass identically whether or not this endpoint checks budget at all. This one instead
// picks the most expensive legal fifteen the fixture's own player pool can field (highest
// price per position slot, respecting the three-per-club cap validateSession enforces),
// which comfortably clears analysis.DefaultBudget (£100.0m).
func TestTheOverBudgetSquadIsServedRatherThanRefused(t *testing.T) {
	s := fixtureServer(t)

	squad15, totalTenths := priciestLegalSquad(t, s)
	if totalTenths <= analysis.DefaultBudget {
		t.Fatalf("the priciest legal squad the fixture can field costs %.1fm, which does not "+
			"exceed the %.1fm default budget -- this test needs an over-budget squad to prove "+
			"anything", float64(totalTenths)/10, float64(analysis.DefaultBudget)/10)
	}

	sess := session{
		Version: sessionVersion,
		Squad:   squad15,
		XI:      squad15[:11],
		Bench:   squad15[11:],
		Captain: squad15[0],
		Vice:    squad15[1],
	}

	w, state := put(t, s, sess, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("an over-budget squad was refused with %d: %s -- validateSession's own "+
			"comment says budget is deliberately not checked", w.Code, w.Body.String())
	}

	if len(state.Squad.Players) != 15 {
		t.Errorf("returned squad has %d players, want 15", len(state.Squad.Players))
	}

	// The key assertion: the endpoint served a genuinely unaffordable squad rather than
	// refusing it, and says so in the figure it hands back -- a negative bank, not merely
	// a 200 status code that a coincidentally-affordable squad would also have produced.
	if state.Squad.Bank >= 0 {
		t.Errorf("bank = %.1f for a squad costing %.1fm against a %.1fm budget, want negative -- "+
			"the endpoint should report the shortfall, not silently clamp or ignore it",
			state.Squad.Bank, float64(totalTenths)/10, float64(analysis.DefaultBudget)/10)
	}
}

// priciestLegalSquad builds the most expensive fifteen the fixture's player pool can
// field that still satisfies validateSession's shape rules (2 GKP/5 DEF/5 MID/3 FWD, at
// most three players from any one club) -- everything EXCEPT budget, which is exactly the
// rule TestTheOverBudgetSquadIsServedRatherThanRefused exists to show is not enforced.
func priciestLegalSquad(t *testing.T, s *squadServer) (codes []int, totalTenths int) {
	t.Helper()
	need := map[string]int{"GKP": 2, "DEF": 5, "MID": 5, "FWD": 3}
	byPos := map[string][]fpl.Element{}
	for _, el := range s.engine.Boot.Elements {
		pos := s.engine.Boot.PositionShort(el.ElementType)
		byPos[pos] = append(byPos[pos], el)
	}
	clubCount := map[int]int{}
	for pos, n := range need {
		els := byPos[pos]
		sort.Slice(els, func(i, j int) bool { return els[i].NowCost > els[j].NowCost })
		picked := 0
		for _, el := range els {
			if picked == n {
				break
			}
			if clubCount[el.Team] >= 3 {
				continue
			}
			codes = append(codes, el.Code)
			totalTenths += el.NowCost
			clubCount[el.Team]++
			picked++
		}
		if picked != n {
			t.Fatalf("could not fill %d %s slots under the three-per-club cap; the fixture's "+
				"pool is too thin for this test", n, pos)
		}
	}
	if len(codes) != 15 {
		t.Fatalf("built %d codes, want 15", len(codes))
	}
	return codes, totalTenths
}
