package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"armband/internal/analysis"
	"armband/internal/capture"
	"armband/internal/config"
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
		{"/", "text/html", "Pick your XI"},
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

// TestTheGateAcceptsAnythingAndKeepsNothing pins the placeholder's whole contract.
//
// Accepting anything is deliberate: the gate stands in for a signup flow that does not
// exist, and refusing would pretend to a capability nothing behind it has. Keeping nothing
// is the half worth testing hardest — the alternative is a file of personal data in a
// working tree with no retention answer, created before anyone decided one was wanted.
func TestTheGateAcceptsAnythingAndKeepsNothing(t *testing.T) {
	s := &squadServer{}

	for _, method := range []string{"POST", "PUT"} {
		form := url.Values{"email": {"someone@example.com"}}
		req := httptest.NewRequest(method, "/gate", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "127.0.0.1:8080"
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		if w.Code != http.StatusSeeOther {
			t.Errorf("%s /gate answered %d, want 303", method, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "/app" {
			t.Errorf("%s /gate redirected to %q, want /app", method, loc)
		}
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

// TestTheGateRejectsSomethingThatIsNotAnAddress pins the one check it does make. This is
// about telling a reader he has fat-fingered his address, not about validating identity.
func TestTheGateRejectsSomethingThatIsNotAnAddress(t *testing.T) {
	s := &squadServer{}
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
