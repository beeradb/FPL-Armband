package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// What a page on another origin can do to the reader's saved work.
//
// The threat model is a single-user tool bound to loopback, so the realistic attacker is
// another local process or another page in the same browser — not a remote one. That makes
// disclosure mostly moot and destruction the thing worth guarding, because the store is an
// HttpOnly cookie the page cannot read back to warn anybody.

// TestAnUnauthedReadDoesNotOverwriteTheReadersSession.
//
// GET /api/state minted a seed and stored it. Any page open in the reader's browser could
// fetch it, and because SameSite=Strict withholds the real cookie the server saw an empty
// session, minted a fresh seed, and answered Set-Cookie — replacing the reader's fifteen,
// arrangement, armband, corrections and chip placements with nothing. The attacker learns
// nothing and destroys everything.
func TestAnUnauthedReadDoesNotOverwriteTheReadersSession(t *testing.T) {
	s := fixtureServer(t)

	req := httptest.NewRequest("GET", routeState, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("an unauthed read answered %d; it is meant to stay readable", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			t.Errorf("an unauthed GET set the session cookie (%q). A cross-origin fetch "+
				"can make this request, and the reader cannot read the cookie back to "+
				"discover their team was replaced.", c.Value)
		}
	}
}

// TestAnAuthedReadStillMintsASeed is the other half, and without it the test above passes
// on a server that simply never mints — which would silently disable the varied opening
// squad the seed exists for.
func TestAnAuthedReadStillMintsASeed(t *testing.T) {
	s := fixtureServer(t)

	req := httptest.NewRequest("GET", routeState+"?t="+s.token, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("an authed read answered %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			return
		}
	}
	t.Error("an authed first read minted no seed, so every reload reshuffles the reader's " +
		"opening squad — the staleness complaint this feature exists to fix, inverted")
}

// TestAStoredSessionThatWouldBeRefusedIsDiscarded.
//
// validateSession guarded the write path only. Cookies are not scoped by port, so anything
// the browser loads from another service on 127.0.0.1 can set this one's cookie. A fifteen
// the write path refuses was then rebuilt on every read: an empty eleven, a zero captain,
// and a client that throws on it — stored in an HttpOnly cookie the page cannot clear, so
// the reader's only escape was devtools.
func TestAStoredSessionThatWouldBeRefusedIsDiscarded(t *testing.T) {
	s := fixtureServer(t)

	first := getWith(t, s, routeState, nil)
	if len(first.Squad.Players) != 15 {
		t.Fatal("the fixture squad is not fifteen")
	}
	one := first.Squad.Players[0].Code

	// The shape the write path refuses, planted directly in the cookie.
	same := make([]int, 15)
	for i := range same {
		same[i] = one
	}
	raw, err := json.Marshal(session{Version: sessionVersion, Squad: same})
	if err != nil {
		t.Fatal(err)
	}
	planted := &http.Cookie{
		Name:  sessionCookieName,
		Value: base64.StdEncoding.EncodeToString(raw),
	}

	// It must be refused on the way in, or this test is checking nothing.
	if w, _ := put(t, s, session{Version: sessionVersion, Squad: same}, nil); w.Code != http.StatusBadRequest {
		t.Fatalf("the write path accepted the illegal fifteen with %d", w.Code)
	}

	st := getWith(t, s, routeState, planted)
	if len(st.Squad.Players) != 15 {
		t.Fatalf("the planted cookie produced a squad of %d players; a session the write "+
			"path refuses must be discarded on read, not rebuilt", len(st.Squad.Players))
	}
	if len(st.Squad.XI) != 11 {
		t.Errorf("the eleven is %d players", len(st.Squad.XI))
	}
	if st.Squad.Captain == 0 {
		t.Error("there is no captain, which is the state the client throws on")
	}
}

// TestAServerWithNoTokenGrantsNothing.
//
// subtle.ConstantTimeCompare on two zero-length slices returns 1, so an empty token made
// tokenOK("") true and opened every write route. `authed` guards that case explicitly, so
// the two disagreed about what an unconfigured server means. cmdServe cannot build one
// today — it returns on a token error — which is exactly why this is worth pinning rather
// than relying on.
func TestAServerWithNoTokenGrantsNothing(t *testing.T) {
	s := fixtureServer(t)
	s.token = ""

	if s.tokenOK("") {
		t.Error("a server with no token accepted an empty token, opening every write route")
	}
	if s.tokenOK("anything") {
		t.Error("a server with no token accepted a non-empty token")
	}

	req := httptest.NewRequest("GET", routeApp+"?t=", nil)
	req.Host = "127.0.0.1:8080"
	if s.authed(req) {
		t.Error("authed and tokenOK disagree about a server with no token")
	}
}
