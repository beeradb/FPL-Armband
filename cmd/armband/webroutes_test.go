package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/capture"
	"armband/internal/config"
	"armband/internal/signup"
	"armband/internal/viewmodel"
)

// fixtureNow is pinned two days before the GW1 deadline the committed capture carries. Any
// instant would do for determinism; this one is chosen so the page reads the way the design
// was drawn — pre-season, with the first deadline still ahead.
var fixtureNow = time.Date(2026, 8, 19, 13, 48, 0, 0, time.UTC)

// fixtureServer builds a server over the committed GW1 capture: no network, no clock, and
// byte-identical inputs on every run.
//
// EntryID is left at zero deliberately. It is what keeps the build off the network — the
// transfer board returns early with a stated reason rather than fetching an owned squad —
// and it is also the honest state for a pre-season page, where there is no squad to price.
func fixtureServer(t *testing.T) *squadServer {
	t.Helper()
	return fixtureServerNamed(t, nil)
}

// fixtureServerNamed is fixtureServer with a hook to rename a player before the engine is
// built. It exists for the escaping test, which needs a name the browser would execute if
// the client interpolated it raw.
func fixtureServerNamed(t *testing.T, rename func(name string) string) *squadServer {
	t.Helper()
	dir := filepath.Join("..", "..", "data", "captures", capture.LiveCapture)
	boot, fixtures, err := capture.Replay(dir)
	if err != nil {
		// Not a skip. The capture is committed, so an unreadable one is a broken
		// repository rather than an absent dependency, and skipping would turn every
		// assertion below into a silent pass.
		t.Fatalf("replaying the committed capture: %v", err)
	}
	if rename != nil {
		for i := range boot.Elements {
			boot.Elements[i].WebName = rename(boot.Elements[i].WebName)
		}
	}
	cfg := config.Default()
	cfg.EntryID = 0

	/* Two standing overrides, so the fixture exercises the paths an empty config cannot.
	   A default config has no roster overrides at all, which means the overrides panel
	   renders its empty state, the "needs a re-check" flag never fires, and the badge on
	   a player card is never drawn -- three surfaces that would be screenshotted blank
	   and pass forever.

	   They are keyed on players from the capture itself rather than invented codes,
	   because the action handler refuses a code the bootstrap does not contain, and a
	   fixture that quietly failed that check would prove nothing. The capture is fixed,
	   so the players it picks are fixed too.

	   The dates are relative to the pinned clock: one checked yesterday, one checked
	   forty days ago. The second is what makes the stale treatment visible. */
	fresh := fixtureNow.AddDate(0, 0, -1).Format("2006-01-02")
	stale := fixtureNow.AddDate(0, 0, -40).Format("2006-01-02")
	mins := 88.0
	cfg.Roster.Minutes = []config.RosterOverride{{
		Code: boot.Elements[0].Code, Name: boot.Elements[0].WebName,
		Reason:          "Named first choice for the season. His record is backup minutes; the role is not.",
		SetOn:           stale,
		LastChecked:     stale,
		ExpectedMinutes: &mins,
	}}
	cfg.Roster.Exclude = []config.RosterOverride{{
		Code: boot.Elements[1].Code, Name: boot.Elements[1].WebName,
		Reason:      "Long-term injury with no named return date.",
		SetOn:       fresh,
		LastChecked: fresh,
	}}

	e := analysis.NewEngineFull(boot, fixtures, cfg.Weights, cfg.Congestion, cfg.RoleRisk)
	return &squadServer{
		token:  "pinnedtoken",
		cfg:    &cfg,
		engine: e,
		weeks:  cfg.Weights.Horizon,
		clock:  func() time.Time { return fixtureNow },
		// Pinned, so two requests in one test see the same fifteen. Without a cookie
		// between them each would mint its own seed and get a different varied squad.
		seed: func() int64 { return 20260819 },
	}
}

// TestTheFixtureMatchesWhatProductionBuildsAtGW1 asserts the premise behind this
// fixture's entry in internal/backtest's unwiredBaseline.
//
// That guard exists because an engine with no recency index silently falls back to flat
// season minutes -- the field is populated and the value is wrong, which is the worst
// shape a bug can take. The fixture is exempt on the grounds that the LIVE binary builds
// the same unwired engine at GW1, since cmd/armband/main.go gates both the recency fetch
// and the priors load behind GameweeksPlayed() > 0.
//
// A comment saying so would rot the first time the capture was repointed at a mid-season
// week, and the exemption would then be quietly wrong in the flattering direction. This
// asserts it instead.
func TestTheFixtureMatchesWhatProductionBuildsAtGW1(t *testing.T) {
	s := fixtureServer(t)
	if played := s.engine.GameweeksPlayed(); played != 0 {
		t.Fatalf("the fixture capture has %d gameweeks played. The live path wires a "+
			"recency index and priors above zero, so this engine is no longer what "+
			"production builds -- either repoint the capture at a pre-season one, or "+
			"wire both here and remove the entry in internal/backtest's "+
			"unwiredBaseline.", played)
	}
	if s.engine.Recent != nil {
		t.Error("the fixture engine has a recency index; production has none at GW1")
	}
}

func get(t *testing.T, s *squadServer, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	// httptest stamps Host as example.com, which the loopback gate refuses. The tests
	// speak as a browser on the loopback interface.
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

// TestTheApplicationIsServedOnItsOwnRoutes pins the route table.
func TestTheApplicationIsServedOnItsOwnRoutes(t *testing.T) {
	s := &squadServer{}
	for _, tc := range []struct {
		path, wantType, wantBody string
	}{
		// The hero, which is what proves the LANDING page was served rather than
		// the app shell.
		//
		// ⚠️ Pin a CONTIGUOUS fragment. The second line is
		// `You make the <span class="u">call.</span>` — the highlight span splits
		// it, so "You make the call" does not appear in the markup at all and an
		// assertion on it fails against a page that is perfectly correct. The
		// first line carries no span.
		//
		// The wording has moved three times in two days ("See the working", "See
		// why.", now this), so this pins the shortest fragment that still means
		// the landing page and not something else.
		{"/", "text/html", "make the case"},
		{"/app", "text/html", "FPL Armband"},
		{"/assets/armband.css", "text/css", "--band"},
		{"/assets/fonts.css", "text/css", "@font-face"},
		{"/assets/fonts/inter-latin.woff2", "font/woff2", ""},
	} {
		w := get(t, s, tc.path)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200", tc.path, w.Code)
			continue
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, tc.wantType) {
			t.Errorf("GET %s served %q, want %s", tc.path, ct, tc.wantType)
		}
		if tc.wantBody != "" && !strings.Contains(w.Body.String(), tc.wantBody) {
			t.Errorf("GET %s does not contain %q", tc.path, tc.wantBody)
		}
	}
}

// TestTheDocumentsRefuseToBeFramed pins the clickjacking header on the new pages.
//
// It mattered on the old page because the lock and boot forms carried a valid token from
// the framed page's own DOM. It matters more now: the application will carry every control
// the reader has, and the token gate cannot see that a click did not come from him.
func TestTheDocumentsRefuseToBeFramed(t *testing.T) {
	s := &squadServer{}
	for _, path := range []string{"/", "/app"} {
		w := get(t, s, path)
		if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("GET %s answered X-Frame-Options %q, want DENY", path, got)
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s answered X-Content-Type-Options %q, want nosniff", path, got)
		}
	}
}

// TestUnknownRoutesStill404 replaces the old two-route assertion, which stopped meaning
// anything once there were six.
func TestUnknownRoutesStill404(t *testing.T) {
	s := &squadServer{}
	for _, path := range []string{"/other", "/api", "/api/", "/api/nothing", "/assets", "/app/"} {
		if w := get(t, s, path); w.Code != http.StatusNotFound {
			t.Errorf("GET %s answered %d, want 404", path, w.Code)
		}
	}
}

// TestTheAssetTreeIsNotAFileSystem pins that the asset route cannot be walked out of.
//
// The tree is an embedded fs.FS rather than a directory, so there is nothing above it to
// reach — but the assertion is cheap and the failure it would catch is total.
func TestTheAssetTreeIsNotAFileSystem(t *testing.T) {
	s := &squadServer{}
	for _, path := range []string{
		"/assets/../pages/app.html",
		"/assets/%2e%2e/pages/app.html",
		"/assets/../../config.json",
	} {
		w := get(t, s, path)
		if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "doctype") {
			t.Errorf("GET %s served a document from outside the asset tree", path)
		}
	}
}

// recordingStore is a signup.Store that keeps what it was given, and can be told to fail.
type recordingStore struct {
	got  []signup.Record
	fail error
}

func (r *recordingStore) Add(_ context.Context, rec signup.Record) error {
	if r.fail != nil {
		return r.fail
	}
	r.got = append(r.got, rec)
	return nil
}
func (r *recordingStore) Close() {}

// TestTheGateRecordsWhatItAccepts is the test that turns the capture on.
//
// The route's previous contract was that it kept NOTHING, and that was correct while
// there was nowhere to put an address with a retention answer behind it. This is the
// replacement claim, and it is the half worth testing hardest now: a gate that answers
// success without writing is indistinguishable, from the page, from one that works.
func TestTheGateRecordsWhatItAccepts(t *testing.T) {
	for _, method := range []string{"POST", "PUT"} {
		store := &recordingStore{}
		s := &squadServer{signups: store}

		// A display name and surrounding whitespace, because Clean is what strips
		// them and the handler must be the thing that calls it.
		form := url.Values{"email": {"  Someone <Someone@Example.com>  "}}
		req := httptest.NewRequest(method, "/gate", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("%s /gate answered %d, want 204", method, w.Code)
		}
		if len(store.got) != 1 {
			t.Fatalf("%s /gate recorded %d submissions, want 1", method, len(store.got))
		}
		if got := store.got[0].Email; got != "Someone@Example.com" {
			t.Errorf("%s /gate recorded %q, want the parsed address with its case kept",
				method, got)
		}
		if store.got[0].Source != signup.SourceForm {
			t.Errorf("%s /gate recorded source %q, want %q",
				method, store.got[0].Source, signup.SourceForm)
		}
		// A typed address is a claim. Only an identity provider may set this, and a
		// gate that marked its own input verified would make the column meaningless.
		if store.got[0].Verified {
			t.Errorf("%s /gate marked a typed address verified", method)
		}

		// The cookie rides with the successful write and nowhere else.
		var gate *http.Cookie
		for _, c := range w.Result().Cookies() {
			if c.Name == gateCookieName {
				gate = c
			}
		}
		if gate == nil {
			t.Fatalf("%s /gate set no %s cookie", method, gateCookieName)
		}
		if !gate.HttpOnly || gate.SameSite != http.SameSiteStrictMode {
			t.Errorf("the gate cookie is HttpOnly=%v SameSite=%v; want true and Strict",
				gate.HttpOnly, gate.SameSite)
		}
		// No MaxAge and no Expires: it dies with the browser, like the override store.
		if gate.MaxAge != 0 || !gate.Expires.IsZero() {
			t.Errorf("the gate cookie outlives the browser session (MaxAge %d, Expires %v)",
				gate.MaxAge, gate.Expires)
		}
	}
}

// TestTheGateRefusesWhenTheWriteFails pins the direction the failure runs in.
//
// The landing page's original bug was a form that reported success and sent nothing. A
// gate that let a reader through over a failed write is that same bug with a database
// behind it, and it is worse: the reader believes they have signed up and will not retry.
func TestTheGateRefusesWhenTheWriteFails(t *testing.T) {
	s := &squadServer{signups: &recordingStore{fail: errors.New("the database is down")}}
	form := url.Values{"email": {"someone@example.com"}}
	req := httptest.NewRequest("POST", "/gate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("/gate answered %d over a failed write, want 500", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == gateCookieName {
			t.Error("/gate set the gate cookie over a failed write, so the reader " +
				"is marked as having signed up when nothing was recorded")
		}
	}
	// The reader's address must not travel in the message a failed write produces.
	if strings.Contains(w.Body.String(), "someone@example.com") {
		t.Error("/gate put the submitted address in its error body")
	}
}

// TestTheGateRefusesWhenNoStoreIsConfigured is the test that stops this change
// re-shipping the bug it was written to remove.
//
// The tempting behaviour is to accept and discard, as this route did before there was
// anywhere to put an address. But 204 means "recorded" to landing.js, so a deployment that
// had lost its database URL — a misspelled env var, an empty Secret — would tell every
// reader they had signed up, forever, while the table stayed empty and nothing failed.
// That is the original defect with a database behind it.
//
// It costs nothing locally: the local landing page posts to the live site and never
// reaches this handler at all.
func TestTheGateRefusesWhenNoStoreIsConfigured(t *testing.T) {
	s := &squadServer{}

	for _, method := range []string{"POST", "PUT"} {
		form := url.Values{"email": {"someone@example.com"}}
		req := httptest.NewRequest(method, "/gate", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s /gate answered %d with no store, want 503. Anything in the "+
				"2xx range tells the reader they signed up when nothing was "+
				"written.", method, w.Code)
		}
		for _, c := range w.Result().Cookies() {
			if c.Name == gateCookieName {
				t.Errorf("%s /gate set the gate cookie with no store, marking a "+
					"reader as signed up when nothing was recorded", method)
			}
		}
	}
}

// TestTheGateRejectsSomethingThatIsNotAnAddress pins the one check it does make. This is
// about telling a reader he has fat-fingered his address, not about validating identity.
func TestTheGateRejectsSomethingThatIsNotAnAddress(t *testing.T) {
	store := &recordingStore{}
	s := &squadServer{signups: store}
	defer func() {
		// The order matters and the test would not otherwise see it: a handler
		// that recorded before validating would still answer 400 and look correct.
		if len(store.got) != 0 {
			t.Errorf("/gate recorded %d rows for input it then rejected",
				len(store.got))
		}
	}()
	form := url.Values{"email": {"not an address"}}
	req := httptest.NewRequest("POST", "/gate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("the gate answered %d for a malformed address, want 400", w.Code)
	}

	// A GET is not a submission.
	if w := get(t, s, "/gate"); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /gate answered %d, want 405", w.Code)
	}
}

// TestTheStateEndpointAnswersTheContract is the end-to-end read: a real engine over the
// committed capture, through the real optimiser, out as the client's JSON.
func TestTheStateEndpointAnswersTheContract(t *testing.T) {
	s := fixtureServer(t)
	w := get(t, s, "/api/state")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/state answered %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("the state is served as %q", ct)
	}
	// A cached squad is the bug this server's whole no-cache design exists to avoid.
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("the state is served with Cache-Control %q, want no-store", cc)
	}
	// Never. A cross-origin page may send this request but must not be able to read the
	// answer, and that property is the absence of this header.
	if h := w.Header().Get("Access-Control-Allow-Origin"); h != "" {
		t.Errorf("the state carries Access-Control-Allow-Origin %q — any page in the "+
			"reader's browser could then read his squad", h)
	}

	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the state did not decode: %v", err)
	}

	if len(st.Squad.Players) != 15 {
		t.Errorf("the squad has %d players, want 15", len(st.Squad.Players))
	}
	if len(st.Squad.XI) != 11 {
		t.Errorf("the eleven has %d players, want 11", len(st.Squad.XI))
	}
	if len(st.Squad.Bench) != 4 {
		t.Errorf("the bench has %d players, want 4", len(st.Squad.Bench))
	}
	if st.Squad.Captain == 0 {
		t.Error("no captain was set; the armband is what this product is named after")
	}
	if st.Squad.Formation == "" {
		t.Error("no formation was reported")
	}
	if !st.Now.Equal(fixtureNow) {
		t.Errorf("the state's clock is %v, want the injected %v", st.Now, fixtureNow)
	}
	if len(st.Gameweeks) == 0 {
		t.Error("the gameweek rail is empty; there is nothing for the rail to draw")
	}
	if len(st.Clubs) != 20 {
		t.Errorf("the club list has %d entries, want 20", len(st.Clubs))
	}
	if len(st.Market.Rows) == 0 {
		t.Error("the market is empty; the Players tab would have nothing in it")
	}
	if st.Market.Gate <= 0 {
		t.Error("the market reports no transfer gate, so no row can be judged actionable")
	}
	if st.Session.Store != "session" {
		t.Errorf("the state says writes land in %q, want session", st.Session.Store)
	}
}

// TestEveryPlayerInTheStateIsDrawable pins that no card can render blank.
//
// The design's card carries a name, a club, a position, a price, a projection and a role
// band. A player missing any of them is not a smaller card, it is a hole in the pitch — and
// with real data that is a per-player failure a spot check would miss.
func TestEveryPlayerInTheStateIsDrawable(t *testing.T) {
	s := fixtureServer(t)
	w := get(t, s, "/api/state")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/state answered %d: %s", w.Code, w.Body.String())
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}

	check := func(where string, p viewmodel.Player) {
		switch {
		case p.ID == 0:
			t.Errorf("%s: a player has no id", where)
		case p.Code == 0:
			t.Errorf("%s: %s has no permanent code", where, p.Name)
		case p.Name == "":
			t.Errorf("%s: player %d has no name", where, p.ID)
		case p.Club == "":
			t.Errorf("%s: %s has no club, so his card has no colour bar", where, p.Name)
		case p.Pos == "":
			t.Errorf("%s: %s has no position, so no pitch row will take him", where, p.Name)
		case p.Price <= 0:
			t.Errorf("%s: %s is priced at %v", where, p.Name, p.Price)
		case p.Role == "":
			t.Errorf("%s: %s has no role band, and the design speaks in roles", where, p.Name)
		}
	}
	for _, p := range st.Squad.Players {
		check("squad", p)
	}
	for _, r := range st.Market.Rows {
		check("market", r.Player)
	}
}

// TestTheSquadsElevenAndBenchAreDisjointAndComplete pins the identity the client mutates.
//
// The client holds the eleven and the bench as id lists and drags cards between them. If a
// referenced id were missing from Players, or a player appeared in both lists, the pitch
// would render a card with no data or the same footballer twice — and the client, which is
// forbidden from computing, has no way to notice.
func TestTheSquadsElevenAndBenchAreDisjointAndComplete(t *testing.T) {
	s := fixtureServer(t)
	w := get(t, s, "/api/state")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/state answered %d", w.Code)
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}

	known := map[int]string{}
	for _, p := range st.Squad.Players {
		if prev, dup := known[p.ID]; dup {
			t.Errorf("player id %d appears twice in the squad (%s and %s)", p.ID, prev, p.Name)
		}
		known[p.ID] = p.Name
	}
	seen := map[int]bool{}
	for _, id := range append(append([]int{}, st.Squad.XI...), st.Squad.Bench...) {
		if _, ok := known[id]; !ok {
			t.Errorf("the eleven or bench names player %d, who is not in the squad", id)
		}
		if seen[id] {
			t.Errorf("player %d (%s) is in both the eleven and the bench", id, known[id])
		}
		seen[id] = true
	}
	if len(seen) != len(known) {
		t.Errorf("%d of the %d players are placed; every one of the fifteen must be "+
			"either in the eleven or on the bench", len(seen), len(known))
	}
	for _, id := range []int{st.Squad.Captain, st.Squad.Vice} {
		if id == 0 {
			continue
		}
		var inXI bool
		for _, x := range st.Squad.XI {
			if x == id {
				inXI = true
			}
		}
		if !inXI {
			t.Errorf("player %d wears an armband from outside the eleven", id)
		}
	}
}

// TestTheGateEchoesOnlyLoopbackOrigins pins the one CORS exception in this server.
//
// The rule everywhere else is that no route may grow an Access-Control-Allow-Origin
// header, because the loopback bind and the Host check are what stop a foreign page
// reading the squad. /gate is the exception, on its content: it answers 204 with no body.
// The exception must stay exactly that narrow — a public origin asking to read the answer
// to somebody else's submission is a thing to refuse.
func TestTheGateEchoesOnlyLoopbackOrigins(t *testing.T) {
	for _, tc := range []struct {
		origin string
		// echo is the Access-Control-Allow-Origin expected back, "" for none.
		echo string
		// accepted says whether the WRITE is allowed to happen at all. This is the
		// half that matters: withholding the echo hides the receipt, not the effect.
		accepted bool
	}{
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080", true},
		{"http://localhost:9999", "http://localhost:9999", true},
		{"http://[::1]:8080", "http://[::1]:8080", true},
		// The site's own origin. Same-origin needs no echo and must still be served.
		{signupOrigin, "", true},
		// Absent Origin: a same-origin form post, or a non-browser client. Allowed,
		// because refusing buys nothing — anything that can omit a header can forge
		// one — and would break every client that does not send it.
		{"", "", true},
		{"https://evil.example", "", false},
		// The rebinding shape the Host check exists for, spelled as an origin.
		{"http://127.0.0.1.evil.example", "", false},
		// A loopback NAME over https is somebody's proxy; no local armband serves one.
		{"https://localhost:8080", "", false},
	} {
		store := &recordingStore{}
		s := &squadServer{signups: store}
		form := url.Values{"email": {"someone@example.com"}}
		req := httptest.NewRequest("POST", "/gate", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		req.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != tc.echo {
			t.Errorf("origin %q was echoed as %q, want %q", tc.origin, got, tc.echo)
		}
		// ⚠️ The load-bearing assertion. CORS never stops a simple cross-origin POST
		// from being SENT, so a gate that merely withheld the header would let any
		// page on the internet insert rows into the one table here holding personal
		// data — and only decline to tell it so. The write must not happen.
		if wrote := len(store.got) > 0; wrote != tc.accepted {
			t.Errorf("origin %q: wrote=%v, want %v — withholding the CORS header "+
				"hides the answer, not the effect", tc.origin, wrote, tc.accepted)
		}
		if !tc.accepted && w.Code != http.StatusForbidden {
			t.Errorf("origin %q answered %d, want 403", tc.origin, w.Code)
		}
		// Credentials must never be allowed cross-origin here: the gate cookie is
		// worth nothing locally, and allowing it would make the echo above a much
		// larger grant than the empty body it is justified by.
		if w.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("origin %q got Access-Control-Allow-Credentials", tc.origin)
		}
	}
}

// TestTheSignupOriginIsSpelledOnceInEffect pins the two spellings equal.
//
// signupOrigin enters the Content-Security-Policy; landing.js is where the fetch actually
// goes. One quantity with two implementations is this project's signature failure, and
// this pair fails particularly quietly: changing one leaves a page whose own policy blocks
// its only control, which no build and no other test would notice.
func TestTheSignupOriginIsSpelledOnceInEffect(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "webui", "assets",
		"static", "landing.js"))
	if err != nil {
		t.Fatalf("reading landing.js: %v", err)
	}
	js := string(raw)

	// The whole URL, built from BOTH constants rather than spelled here. routeGate is
	// what the mux actually serves, so renaming the route without touching the script
	// fails this test instead of silently 404ing every submission.
	want := "'" + signupOrigin + routeGate + "'"
	if !strings.Contains(js, want) {
		t.Errorf("landing.js does not name %s, so the Content-Security-Policy built "+
			"from signupOrigin does not describe where the page posts", want)
	}
	// ⚠️ Containment is not use. A previous version of this test asserted only that the
	// URL APPEARED in the file, which would pass with the constant left assigned and
	// dead beside a fetch that had been reverted to a relative '/gate' — the exact
	// regression the absolute URL exists to prevent, and it would put a local reader's
	// address in a local list nobody collects. So the fetch must be the thing that
	// takes it.
	if !strings.Contains(js, "fetch(GATE,") {
		t.Error("landing.js does not fetch(GATE, ...), so the constant it declares " +
			"may be dead and the submission going somewhere else")
	}
	// And nothing else may be fetched from the landing page. A second destination
	// would be blocked by connect-src at runtime, which is a broken page rather than
	// a failing test — so it is caught here.
	if n := strings.Count(js, "fetch("); n != 1 {
		t.Errorf("landing.js makes %d fetch calls, want exactly 1", n)
	}
}

// TestThePolicyPermitsTheGateItShips is the mirror of the test above, on the header rather
// than the script. A check that the policy NAMES the origin is worth little without one
// that the page can actually reach what it names.
func TestThePolicyPermitsTheGateItShips(t *testing.T) {
	s := &squadServer{}
	req := httptest.NewRequest("GET", "/", nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' "+signupOrigin) {
		t.Errorf("the landing page's connect-src does not permit %s, so its signup "+
			"form cannot post. Policy: %s", signupOrigin, csp)
	}
	// The exception is one named host on one directive, not a loosening. A foreign
	// origin on script-src would give an injected string somewhere to run from, which
	// is what the whole out-of-line-script arrangement exists to prevent.
	if strings.Contains(csp, "script-src 'self' http") {
		t.Errorf("script-src grew a foreign origin: %s", csp)
	}

	// The mirror, and the half with the power: a check that the landing page CAN
	// reach the origin proves nothing about whether the application was widened
	// alongside it. /app holds the squad and must reach nothing but itself.
	req = httptest.NewRequest("GET", "/app", nil)
	req.Host = "127.0.0.1:8080"
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if app := w.Header().Get("Content-Security-Policy"); strings.Contains(app, signupOrigin) {
		t.Errorf("the application's policy names %s. That page renders FPL's prose "+
			"by innerHTML, and connect-src 'self' is what stops an injected string "+
			"sending the squad anywhere. Policy: %s", signupOrigin, app)
	}
}
