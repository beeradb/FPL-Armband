package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestASecondSaveDoesNotResurrectADismissedOverride is the bug a single-PUT test cannot see.
//
// The client rebuilds its pending session from the served document on every load, which is
// deliberate — the server is the one that knows what is stored. But `dismissed` was not IN
// the document, so it rebuilt as an empty list, and the next unrelated save wrote that
// emptiness back. Clear an override, then drag one player to the bench, and the override
// returns with nothing to explain it.
//
// It is the same defect the remove button had — a change that appears to happen and does
// not — arriving by the opposite route: the first save worked and a later one undid it.
func TestASecondSaveDoesNotResurrectADismissedOverride(t *testing.T) {
	s := fixtureServer(t)

	before := getWith(t, s, routeState, nil)
	var target int
	for _, o := range before.Overrides.Live {
		if o.Code != 0 {
			target = o.Code
			break
		}
	}
	if target == 0 {
		t.Fatal("the fixture carries no override with a player to clear")
	}

	// One: dismiss it.
	w, after := put(t, s, session{Version: sessionVersion, Dismissed: []int{target}}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("the dismissal answered %d: %s", w.Code, w.Body.String())
	}
	if !hasID(after.Session.Dismissed, target) {
		t.Fatalf("the answer does not carry the dismissal back. The client rebuilds its "+
			"pending session from this document, so a field the document omits is a field "+
			"the next save will overwrite with nothing. Dismissed: %v", after.Session.Dismissed)
	}
	cookie := sessionCookie(t, w)

	// Two: a completely unrelated change, sent the way the client sends it — the whole
	// pending session, rebuilt from the document above.
	second := session{
		Version:   sessionVersion,
		Dismissed: after.Session.Dismissed,
		Lock:      after.Session.Locked,
		Exclude:   after.Session.Blocked,
		Chips:     map[string]string{"2": "bboost"},
	}
	w2, third := put(t, s, second, cookie)
	if w2.Code != http.StatusOK {
		t.Fatalf("the second save answered %d: %s", w2.Code, w2.Body.String())
	}
	for _, o := range third.Overrides.Live {
		if o.Code == target {
			t.Errorf("%s is binding again after an unrelated save. The dismissal did not "+
				"survive the round trip.", o.Player)
		}
	}
}

// TestThePageDoesNotHandOutTheWriteTokenToAnyCaller.
//
// The token has to reach the page — the client puts it on every write — and a document is
// readable by anything that can make a request. Any local process could curl "/", lift the
// token out of the meta tag, and drive the write path; under -persist that writes a standing
// override into config.json which then binds every future agent run. The loopback bind and
// the Host check do not help, because that attacker is not a browser.
func TestThePageDoesNotHandOutTheWriteTokenToAnyCaller(t *testing.T) {
	s := fixtureServer(t)

	// A caller who never presented the token gets the shell.
	plain := get(t, s, routeLanding)
	if plain.Code != http.StatusOK {
		t.Fatalf("GET / answered %d", plain.Code)
	}
	if strings.Contains(plain.Body.String(), s.token) {
		t.Error("GET / handed the write token to a caller that never presented it. " +
			"Any local process can read it and drive the write path.")
	}

	// The printed URL carries it, and the answer grants the cookie.
	req := httptest.NewRequest("GET", routeLanding+"?t="+s.token, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	var auth *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == authCookieName {
			auth = c
		}
	}
	if auth == nil {
		t.Fatal("opening the printed URL granted no auth cookie, so the page can never write")
	}
	if !auth.HttpOnly || auth.SameSite != http.SameSiteStrictMode {
		t.Errorf("the auth cookie is not hardened: %+v", auth)
	}

	// And with it, the page carries the token.
	req2 := httptest.NewRequest("GET", routeLanding, nil)
	req2.Host = "127.0.0.1:8080"
	req2.AddCookie(auth)
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)
	if !strings.Contains(w2.Body.String(), s.token) {
		t.Error("a browser that presented the token was not given it back, so nothing on " +
			"the page can save")
	}
}

// TestAnIllegalSquadIsRefused pins the shapes that used to be storable.
//
// A fifteen of one player passed the old check, produced an empty eleven and a zero
// captain, and was then stored in an HttpOnly cookie the page cannot clear — so every
// reload rebuilt the same broken state.
func TestAnIllegalSquadIsRefused(t *testing.T) {
	s := fixtureServer(t)
	first := getWith(t, s, routeState, nil)
	if len(first.Squad.Players) != 15 {
		t.Fatal("the fixture squad is not fifteen")
	}
	legal := make([]int, 0, 15)
	for _, p := range first.Squad.Players {
		legal = append(legal, p.Code)
	}

	same := make([]int, 15)
	for i := range same {
		same[i] = legal[0]
	}

	for _, tc := range []struct {
		name string
		sess session
	}{
		{"the same player fifteen times", session{Version: sessionVersion, Squad: same}},
		{"fourteen players", session{Version: sessionVersion, Squad: legal[:14]}},
		{"an eleven naming someone outside the squad",
			session{Version: sessionVersion, Squad: legal, XI: append(append([]int{}, legal[:10]...), 999999)}},
		{"a captain outside the eleven",
			session{Version: sessionVersion, Squad: legal, XI: legal[:11], Captain: legal[14]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, _ := put(t, s, tc.sess, nil)
			if w.Code != http.StatusBadRequest {
				t.Errorf("answered %d, want 400. It would be stored in a cookie the page "+
					"cannot clear, and every reload would rebuild it.", w.Code)
			}
		})
	}

	// And the legal one still passes, or the guard above proves nothing.
	if w, _ := put(t, s, session{Version: sessionVersion, Squad: legal}, nil); w.Code != http.StatusOK {
		t.Errorf("a legal fifteen was refused with %d: %s", w.Code, w.Body.String())
	}
}
