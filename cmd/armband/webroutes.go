package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/signup"
	"armband/internal/viewmodel"
	"armband/internal/webui"
)

// The routes the client application is served on.
//
// # Why the pages are static and the data is not
//
// The old page was rendered whole on every GET, which is why it could never be stale. The
// application inverts that: the document is a fixed shell served from the binary, and the
// numbers arrive from /api/state, which still rebuilds from scratch on every request. The
// staleness property is unchanged — it has simply moved to the half that carries the
// numbers, which is the only half that can go stale.
//
// # Why /api/state does not check the token
//
// The token gates WRITES. It always has: GET / never checked it either, and the token
// exists in the printed URL so the page can put it in its forms. A read of the same squad
// the page already displays is not a new exposure.
//
// What does the protecting is the pair the server already had: the loopback bind, and the
// Host check against DNS rebinding. A page on another origin can SEND a request here, but
// with no Access-Control-Allow-Origin in the response it cannot read the answer — so
// nothing here may ever grow a CORS header without a threat model saying why.
const (
	routeLanding = "/"
	routeApp     = "/app"
	routeGate    = "/gate"
	routeState   = "/api/state"
	routeSession = "/api/session"
	prefixAssets = "/assets/"
)

// signupOrigin is where the landing page's gate posts, from wherever it is served.
//
// # Why an absolute origin rather than a relative path
//
// The same binary serves the public site and a local `armband serve`. A relative /gate
// would mean a reader who signed up against a local copy landed in a local list nobody
// ever collects, which is the same lost submission as the discard this change removed.
// Absolute, this is same-origin in production — the common case, costing nothing — and
// cross-origin locally, which is what the CORS echo in allowGateOrigin is for.
//
// ⚠️ This value is spelled TWICE: here, where it enters the Content-Security-Policy, and
// in assets/static/landing.js, where the fetch actually goes. One quantity with two
// implementations is this project's signature failure, so the two are pinned equal by
// TestTheSignupOriginIsSpelledOnceInEffect. A change here without the other is a page
// whose own policy blocks its only control.
const signupOrigin = "https://fplarmband.com"

// connectSrcFor is the one directive that differs between the two documents.
//
// The landing page needs to reach signupOrigin, because its gate posts there from wherever
// the page is served. The APPLICATION does not, and must not: /app is the page that renders
// FPL's prose and player names by innerHTML, so it is the one an injected string could ride
// into — and connect-src 'self' is precisely what stops such a string sending the squad
// anywhere. Widening it there to save a branch here would trade the guarantee that matters
// for the page that does not need it.
//
// Two documents, two policies, and the difference is one entry on one directive. If a third
// document ever appears, this stops being a switch and starts being a table.
func connectSrcFor(page string) string {
	if page == "landing" {
		// Named rather than wildcarded, and one host. It buys an injected string on
		// this page exactly one destination, under the same ownership as this binary.
		// Adding a second needs a reason of the same kind — 'self' plus a named peer
		// is a policy; a list of convenient hosts is not.
		return "connect-src 'self' " + signupOrigin
	}
	return "connect-src 'self'"
}

// authCookieName marks a browser that has presented the printed token.
//
// It exists because the token has to reach the page — the client puts it on every write —
// and a document is readable by anything that can make a request. Without this, any local
// process could `curl /app`, lift the token out of the meta tag, and drive the write path;
// under -persist that means writing a standing override into config.json, which then binds
// every future agent run. The loopback bind and the Host check do not help, because the
// attacker there is not a browser.
//
// So the token is handed out only in exchange for the token: open the URL the server
// printed, which carries ?t=, and the answer sets this cookie. A requester that never had
// the token gets the shell and a page that says where to find the real URL.
const authCookieName = "fpl_auth"

// authed reports whether this request may be given the write token.
func (s *squadServer) authed(r *http.Request) bool {
	if s.token == "" {
		return false
	}
	if c, err := r.Cookie(authCookieName); err == nil && s.tokenOK(c.Value) {
		return true
	}
	// The printed URL itself, which is how a browser gets the cookie in the first place.
	return s.tokenOK(r.URL.Query().Get("t"))
}

// grantAuth sets the cookie when the request presented the token in its URL.
func (s *squadServer) grantAuth(w http.ResponseWriter, r *http.Request) {
	if !s.tokenOK(r.URL.Query().Get("t")) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    s.token,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		HttpOnly: true,
	})
}

// servePage writes one of the embedded documents.
func (s *squadServer) servePage(w http.ResponseWriter, r *http.Request, name string) {
	body, err := webui.Page(name)
	if err != nil {
		// A route naming a page that is not embedded is a programming error, but it
		// must not take the server down: the reader is mid-task and the other routes
		// still work.
		fmt.Fprintf(os.Stderr, "serve: page %q: %v\n", name, err)
		http.Error(w, "that page is not built into this binary", http.StatusInternalServerError)
		return
	}
	// The application is never meant to be framed. A hostile page could otherwise iframe
	// it and clickjack the controls, and the token gate cannot see that a click did not
	// come from the reader.
	w.Header().Set("X-Frame-Options", "DENY")
	// The policy the out-of-line scripts were for.
	//
	// The client renders by innerHTML and the strings it interpolates -- player names,
	// FPL's news prose, the reasoning on an override -- are written by nobody here.
	// Escaping is the guard and it is tested through a real browser, but escaping is one
	// missed sink away from failing, and three were missed on the first pass. This is what
	// stands behind it: with no inline script permitted, a name that reaches the DOM as
	// markup still cannot execute, and connect-src 'self' means it could not send anything
	// anywhere if it did.
	//
	// script-src has no 'unsafe-inline'. That is the whole point, and it is why app.js and
	// landing.js are separate files rather than inline blocks -- a policy that permits
	// inline script permits whatever an injection manages to open, which is most of the
	// value gone.
	//
	// style-src does allow it. The landing page carries an inline <style> block, and a
	// stylesheet is a far weaker instrument than a script: the attack it buys is a
	// defacement rather than a same-origin read of the squad. Removing it is worth doing
	// and is not worth blocking this on.
	//
	// frame-ancestors 'none' says what X-Frame-Options above says, in the header that
	// superseded it; both are set because the older one is what some tooling still reads.
	w.Header().Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		connectSrcFor(name),
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"form-action 'self'",
	}, "; "))
	// The pages are HTML and the assets are typed by extension; nothing here should ever
	// be re-interpreted as another type on a sniffing browser.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.grantAuth(w, r)
	if !s.authed(r) {
		// The shell, with no capability in it. The page's own boot will read an empty
		// token and every write will be refused, so it says what to do instead.
		_, _ = w.Write(body)
		return
	}
	_, _ = w.Write(s.withToken(body))
}

// tokenMeta is the placeholder the embedded document carries, and what the server fills in.
//
// The token goes in the PAGE and not in /api/state. That endpoint is an unauthenticated
// read -- deliberately, since it shows the same squad the page does -- and putting a write
// capability in its body would turn every read into a handout of one. The retired page
// carried its token in the markup for the same reason.
//
// The embedded file ships with the placeholder EMPTY, so a copy of the document taken off
// disk carries no capability at all.
const tokenMeta = `<meta name="armband-token" content="">`

func (s *squadServer) withToken(body []byte) []byte {
	if s.token == "" {
		return body
	}
	filled := `<meta name="armband-token" content="` + html.EscapeString(s.token) + `">`
	return bytes.Replace(body, []byte(tokenMeta), []byte(filled), 1)
}

// gate accepts the landing page's email form and records the address.
//
// # What changed, and what did not
//
// This used to accept anything and store NOTHING, because there was nowhere to put an
// address that had a retention answer behind it. There is now: signup.Store, Postgres in
// the deployment. What did not change is the validation — shape only. A gate that refused
// addresses it merely disapproved of would reject real ones, and whether an address
// receives mail is answered by sending mail to it.
//
// The address is still never logged. "We only wrote it to stderr" is still having written
// it down, and stderr is the one sink here with no retention answer at all.
//
// # Why 204 and not the redirect it used to answer
//
// The landing page served from a LOCAL `armband serve` posts to the live site, so the
// reader is captured once wherever they came from. That request is cross-origin, and a
// redirect to /app would be followed by fetch and then blocked — /app sends no CORS
// headers and never should — turning a submission that SUCCEEDED into a network error the
// page reports as a failure. 204 says "recorded, nothing to fetch"; the page navigates
// itself, which it was doing anyway.
func (s *squadServer) gate(w http.ResponseWriter, r *http.Request) {
	// Clamped before anything else parses the body, for the same reason /action clamps:
	// the body is the one thing a caller can make arbitrarily expensive, and this form
	// carries one short field.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.Header().Set("Allow", "POST, PUT")
		http.Error(w, "the gate takes a POST or a PUT", http.StatusMethodNotAllowed)
		return
	}
	if !allowGateOrigin(w, r) {
		// REFUSED, not merely unacknowledged. Withholding the CORS header would
		// hide the ANSWER from a foreign page while still doing the write, which is
		// not a control at all: a simple cross-origin POST is sent either way, so
		// every visitor to a hostile page would insert a row and only be denied the
		// receipt. This route is the one write path open to the internet.
		http.Error(w, "the gate does not take submissions from that origin",
			http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "unreadable form", http.StatusBadRequest)
		return
	}
	addr, err := signup.Clean(r.PostFormValue("email"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// No store configured means this server cannot do the one thing this route is for,
	// and it says so rather than answering success.
	//
	// The temptation is to accept and discard, as this route did before there was
	// anywhere to put an address. That is precisely the bug the change removed: 204
	// means "recorded" to landing.js, so a deployment that had lost its database URL —
	// a misspelled env var, an empty Secret — would tell every reader they had signed
	// up, forever, while the table stayed empty and nothing failed. A 503 is loud, and
	// it costs nothing locally because the local landing page posts to the live site
	// and never arrives here at all.
	if s.signups == nil {
		http.Error(w, "this server is not recording signups", http.StatusServiceUnavailable)
		return
	}
	// The request's context, so a reader who navigates away does not leave a query
	// running, and the deployment's proxy timeout bounds this rather than nothing
	// bounding it.
	if err := s.signups.Add(r.Context(), signup.Record{
		Email:  addr,
		Source: signup.SourceForm,
		// A typed address is a claim, never a proof. Only an identity provider
		// sets this.
		Verified: false,
	}); err != nil {
		// Answering 500 rather than letting the reader through is the whole point.
		// A gate that says "check your inbox" over a failed write is the exact
		// silent failure this route was rewritten to remove — and the reader can
		// retry, which they cannot do if nobody tells them.
		fmt.Fprintf(os.Stderr, "%s\n", dim("gate: "+err.Error()))
		http.Error(w, "we could not record that just now — please try again",
			http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     gateCookieName,
		Value:    "1",
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		HttpOnly: true,
	})
	w.WriteHeader(http.StatusNoContent)
}

// allowGateOrigin decides whether a submission may proceed, and echoes CORS when it may.
//
// # Why this is a REFUSAL and not just a withheld header
//
// CORS is not access control. A cross-origin POST of form-encoded data is a "simple
// request": the browser sends it, the server acts on it, and the response headers decide
// only whether the CALLING PAGE may read the answer. Declining to echo an origin therefore
// hides the receipt and not the effect — so a hostile page could have every one of its
// visitors' browsers insert a row into the one table here holding personal data, and learn
// nothing, which it did not want to anyway.
//
// This route is the only write path on this server open to the internet. So the origin is
// CHECKED, and a request from anywhere unexpected is refused before the write.
//
// A browser always sends Origin on a cross-origin POST, so this stops the drive-by case it
// is aimed at. It is not a defence against a non-browser client, which can send any header
// it likes — nothing at this layer could be. What bounds that case is the deployment's
// per-IP rate limit, and it is a bound on volume rather than on intent.
//
// # Why loopback is allowed at all
//
// A LOCAL armband serves the same landing page, which posts here so that one address lands
// in one list. That request is cross-origin and its page must be able to read the answer,
// or a submission that succeeded reports as a network failure. The echo is safe on this
// route's CONTENT rather than on convenience: /gate answers 204 with no body, so there is
// nothing for a page to learn that it did not supply itself.
//
// Access-Control-Allow-Credentials is deliberately absent, so the cross-origin case sends
// and sets no cookies. The gate cookie is worth nothing locally anyway: /app is reachable
// directly, exactly as the design's "I have access" link expects.
func allowGateOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	// Absent means a same-origin form post or a non-browser client. Refusing it would
	// break `curl` and every same-origin browser that omits the header, and it buys
	// nothing: anything that can omit a header can also forge one.
	if origin == "" {
		return true
	}
	// The site's own origin, which is what the public page sends. Same-origin needs no
	// echo — the browser does not check CORS on a same-origin response.
	if origin == signupOrigin {
		return true
	}
	u, err := url.Parse(origin)
	// http only. A loopback name over https would be somebody's proxy, and there is no
	// local armband that serves one.
	if err != nil || u.Scheme != "http" || !loopbackHost(u.Host) {
		return false
	}
	// Echoed rather than "*", so the header names exactly the origin that asked and a
	// cache cannot serve it to another one. Vary says the same thing to the cache.
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")
	return true
}

// gateCookieName marks a reader who has been through the landing form.
//
// It gates nothing today — /app is reachable directly, exactly as the design's "I have
// access" link expects. It exists so the landing page can tell a returning reader apart
// from a new one, and so the eventual real flow has somewhere to put its answer.
const gateCookieName = "fpl_gate"

// state answers the client contract, for the reader's own team where they have one.
//
// A GET may WRITE the cookie, which is worth naming because it looks wrong. A new session
// has no seed, and a seed drawn per request would hand the reader a different squad on
// every reload -- the exact staleness complaint this is meant to fix, inverted. So the
// first authed GET mints one and stores it, and every later GET is a pure read.
//
// ⚠️ Only an AUTHED request may mint, and that is a security property rather than a tidiness
// one. The route is otherwise open, which is defensible for a read of a squad the page
// already shows. Writing made it something else: any page in the reader's browser could
// fetch this, and because SameSite=Strict withholds their real cookie the server would see
// an empty session, mint a seed and answer Set-Cookie -- silently replacing the reader's
// team, arrangement, armband, corrections and chip placements with an empty session, in an
// HttpOnly cookie the page cannot read back to warn them. The attacker learns nothing and
// destroys everything, which is the worst trade available.
//
// A caller without the token still gets a document. It is built on a zero seed, so it is the
// straight optimum rather than a varied squad -- a fine answer to "what does the model
// think", and not a store.
func (s *squadServer) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := s.readValidSession(r)
	if sess.Seed == 0 && !sess.Optimised && s.authed(r) {
		sess.Seed = s.nextSeed()
		if err := sess.write(w); err != nil {
			fmt.Fprintf(os.Stderr, "serve: storing the session seed: %v\n", err)
		}
	}
	s.answerState(w, r, sess)
}

// answerState builds and writes the document for a session. Shared by the read and the
// write route so the two cannot disagree about what the state IS.
func (s *squadServer) answerState(w http.ResponseWriter, r *http.Request, sess session) {
	body, err := s.buildState(r, sess)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeState(w, body)
}

// writeState sends an already-marshalled document.
func writeState(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The state is rebuilt per request and must never be reused: a cached squad is the
	// bug the whole no-cache design of this server exists to avoid.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// buildState produces the document for a session, or an error naming what went wrong.
func (s *squadServer) buildState(r *http.Request, sess session) ([]byte, error) {
	cfg := s.effectiveCfgFrom(sess)
	b, err := buildSquadPage(r.Context(), cfg, s.client, s.engine, pageOpts{
		Weeks:     s.weeks,
		WantPage:  true,
		Now:       s.now(),
		Fixed:     sess.Squad,
		Seed:      sess.Seed,
		Optimised: sess.Optimised,
		Arrange: arrangement{
			XI: sess.XI, Bench: sess.Bench, Captain: sess.Captain, Vice: sess.Vice,
		},
	})
	if err != nil {
		return nil, err
	}
	b.Page.Token = s.token
	b.Page.SessionMode = !s.persist
	if !s.persist {
		markSessionOverrides(&b.Page, sess)
	}

	st, err := viewmodel.Build(viewmodel.Input{
		Page:      b.Page,
		Boot:      s.engine.Boot,
		Cfg:       cfg,
		Now:       s.now(),
		Persist:   s.persist,
		Session:   sess.arrangement(),
		Optimised: sess.Optimised,
		Saved:     len(sess.Squad) == 15,
	})
	if err != nil {
		// Build's only failure is a number encoding/json would refuse. Naming the field
		// beats letting the encoder fail into a half-written 200 the client would parse.
		return nil, err
	}
	// Marshalled here rather than streamed, so a failure is an error a caller can answer
	// with a status instead of a truncated body.
	return json.Marshal(st)
}

// saveSession stores the reader's team and answers with the recomputed document.
//
// # Why the answer is the whole state rather than an acknowledgement
//
// The client changes something, the server stores it, and the model may then say something
// different -- a blocked player leaves the squad, a dismissed override changes the
// projection. Answering "ok" would leave the page to guess at the consequences, which is
// how a client ends up recomputing model quantities. Answering with the document means the
// page never holds an opinion the server has not seen.
//
// # Why this is a write route with a token and /api/state is not
//
// It changes stored state. The token has always gated writes here, and this is one.
func (s *squadServer) saveSession(w http.ResponseWriter, r *http.Request) {
	// Clamped before the mutex and before the token check, for the same reason /action
	// clamps: the body is the one thing a caller can make arbitrarily expensive.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)

	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		w.Header().Set("Allow", "PUT, POST")
		http.Error(w, "the session takes a PUT", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.tokenOK(r.Header.Get("X-Armband-Token")) {
		http.Error(w, "missing or wrong token", http.StatusForbidden)
		return
	}

	var in session
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "unreadable session: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.Version = sessionVersion
	// Settled before anything reads it, so the document this request answers with and the
	// cookie it stores are the same session.
	in = in.settled()

	// Every code must resolve, because a code the bootstrap does not carry becomes a
	// nameless row the reader cannot clear. The client only ever sends codes it was given,
	// so a miss means a stale page rather than a hostile one -- but it is still refused.
	if err := s.validateSession(in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Under -persist the corrections leave the session and enter the file.
	//
	// Done BEFORE the state is built, so the document this request answers with is rendered
	// from the config the reader will actually get back on their next load, rather than
	// from a session that is about to be emptied.
	if s.persist {
		var err error
		if in, err = s.persistCorrections(in); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// The state is built BEFORE the cookie is written.
	//
	// The other order stages a Set-Cookie and can then fail rendering, which stores a
	// session that cannot be rendered — in an HttpOnly cookie the page cannot clear, so
	// every later request rebuilds from it and fails the same way, and the reader has to
	// open devtools to recover. /action already gets this right and says so: the change is
	// saved before it is adopted, and a failure leaves everything as it was.
	body, err := s.buildState(r, in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := in.write(w); err != nil {
		// The ceiling is the interesting failure: a browser drops an oversized cookie
		// silently, and the reader's work would vanish with no error anywhere.
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	writeState(w, body)
}

// persistCorrections moves the reader's locks and blocks out of the session and into
// config.json, and hands back the session with them removed.
//
// It exists so that -persist means what it says. Without it the flag changed only which
// store the document CLAIMED to be using: the corrections stayed in the cookie, the file was
// never opened, and `store: "config"` was a label rather than a fact.
//
// Removing them from the session as they are written is the part that matters. Two stores
// holding the same correction is how a dismissal comes back: the reader clears it from the
// file, the session still carries it, and the next build applies it again from the copy
// nobody was looking at.
//
// The file is written before the session is adopted, so a failed save leaves both stores
// exactly as they were — the same order /action uses, and for the same reason.
func (s *squadServer) persistCorrections(in session) (session, error) {
	if len(in.Lock) == 0 && len(in.Exclude) == 0 && len(in.Dismissed) == 0 {
		return in, nil
	}
	if s.cfgPath == "" {
		return in, fmt.Errorf("there is no config path, so -persist has nowhere to write " +
			"— your corrections were not saved")
	}

	today := s.now().Format("2006-01-02")
	next := *s.cfg
	// The bootstrap, not AllMetrics: an override names a footballer, and a code the pool
	// has filtered out still has one. A nameless override is one the reader cannot identify
	// in the file afterwards.
	name := func(code int) string {
		for i := range s.engine.Boot.Elements {
			if el := &s.engine.Boot.Elements[i]; el.Code == code {
				return el.WebName
			}
		}
		return ""
	}
	set := func(kind string, codes []int, why string) error {
		for _, code := range codes {
			if err := next.Roster.Set(kind, config.RosterOverride{
				Code: code, Name: name(code), Reason: why,
				SetOn: today, LastChecked: today,
			}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := set("lock", in.Lock, "locked from the planner"); err != nil {
		return in, err
	}
	if err := set("exclude", in.Exclude, "blocked from the planner"); err != nil {
		return in, err
	}
	// A dismissal under -persist is a deletion from the file, not a suppression of it.
	// Anything else would leave the reader unable to remove an override from the one store
	// the flag exists to edit.
	for _, code := range in.Dismissed {
		_ = next.Roster.Remove("lock", code)
		_ = next.Roster.Remove("exclude", code)
		_ = next.Roster.Remove("minutes", code)
	}

	if err := config.Save(s.cfgPath, next); err != nil {
		return in, fmt.Errorf("saving config: %w", err)
	}
	s.cfg = &next
	in.Lock, in.Exclude, in.Dismissed = nil, nil, nil
	return in, nil
}

// validateSession refuses anything that is not a legal FPL squad.
//
// # Why this is more than a well-formedness check
//
// It used to check only that every code resolved and that the lists were the right length.
// That accepted a fifteen of the SAME player fifteen times: bestFormation then finds no
// legal shape, returns nothing, and the page renders an empty eleven with a zero captain —
// which is then stored in an HttpOnly cookie the page itself cannot clear, so every reload
// rebuilds the same broken state and the reader has to go into devtools to escape.
//
// It also accepted a squad of any budget, any club distribution and any position mix, which
// matters because this planner is being positioned as somewhere transfers are decided. A
// fifteen that could never exist prices moves that could never be made.
//
// Budget is deliberately NOT checked. The reader's money is a fact about their entry that
// this store does not carry, and a squad over budget is a state the optimiser can legitimately
// be asked about (a wildcard, a hypothetical). The rules checked here are the ones that make
// a fifteen a fifteen.
func (s *squadServer) validateSession(in session) error {
	type known struct {
		pos  string
		club int
	}
	byCode := map[int]known{}
	for i := range s.engine.Boot.Elements {
		el := &s.engine.Boot.Elements[i]
		byCode[el.Code] = known{pos: s.engine.Boot.PositionShort(el.ElementType), club: el.Team}
	}

	// Bounded before anything is walked. A list is only ever as long as a squad, and the
	// cookie ceiling is not a substitute — session.empty() short-circuits before it for a
	// payload that carries only a bench.
	const maxList = 64
	for name, group := range map[string][]int{
		"squad": in.Squad, "xi": in.XI, "bench": in.Bench,
		"lock": in.Lock, "exclude": in.Exclude, "dismissed": in.Dismissed,
	} {
		if len(group) > maxList {
			return fmt.Errorf("%s carries %d players, which is more than any squad has", name, len(group))
		}
		for _, code := range group {
			if code != 0 {
				if _, ok := byCode[code]; !ok {
					return fmt.Errorf("no player has code %d", code)
				}
			}
		}
	}
	for _, code := range []int{in.Captain, in.Vice} {
		if code != 0 {
			if _, ok := byCode[code]; !ok {
				return fmt.Errorf("no player has code %d", code)
			}
		}
	}
	if len(in.Chips) > 64 {
		return fmt.Errorf("%d chip placements, which is more than a season has gameweeks", len(in.Chips))
	}
	// A placement the competition would refuse is refused here.
	//
	// The rail only draws the chips analysis.PlayableChips returns, so the page cannot
	// produce one of these. The endpoint could, and the result would be silent rather than
	// visible: the placement stores in a cookie the page cannot clear, and ApplyChipPlan
	// bends the whole squad build around a chip that can never be played.
	//
	// Checked against the same function the rail is drawn from, so there is one statement
	// of when a chip may be played rather than a validator and a renderer that drift.
	usedGW := map[int]bool{}
	for week, key := range in.Chips {
		gw, err := strconv.Atoi(week)
		if err != nil {
			return fmt.Errorf("%q is not a gameweek", week)
		}
		if usedGW[gw] {
			return fmt.Errorf("two chips placed in gameweek %d, and the game allows one", gw)
		}
		usedGW[gw] = true
		legal := false
		for _, k := range analysis.PlayableChips(s.engine.Boot, gw) {
			if k == key {
				legal = true
				break
			}
		}
		if !legal {
			return fmt.Errorf("%s cannot be played in gameweek %d", analysis.ChipLabel(key), gw)
		}
	}

	if len(in.Squad) == 0 {
		// No team stored: the corrections above are all there is to check.
		return nil
	}
	if len(in.Squad) != 15 {
		return fmt.Errorf("a squad is fifteen players, not %d", len(in.Squad))
	}

	seen := map[int]bool{}
	pos := map[string]int{}
	club := map[int]int{}
	for _, code := range in.Squad {
		if seen[code] {
			return fmt.Errorf("player %d appears twice; a squad is fifteen different players", code)
		}
		seen[code] = true
		k := byCode[code]
		pos[k.pos]++
		club[k.club]++
	}
	for want, n := range map[string]int{"GKP": 2, "DEF": 5, "MID": 5, "FWD": 3} {
		if pos[want] != n {
			return fmt.Errorf("a squad holds %d %s, not %d", n, want, pos[want])
		}
	}
	for id, n := range club {
		if n > 3 {
			return fmt.Errorf("%d players from one club; the limit is three", n)
		}
		_ = id
	}

	if len(in.XI) != 0 {
		if len(in.XI) != 11 {
			return fmt.Errorf("an eleven is eleven players, not %d", len(in.XI))
		}
		inXI := map[int]bool{}
		for _, code := range in.XI {
			if !seen[code] {
				return fmt.Errorf("player %d is in the eleven but not in the squad", code)
			}
			if inXI[code] {
				return fmt.Errorf("player %d is in the eleven twice", code)
			}
			inXI[code] = true
		}
		for _, code := range []int{in.Captain, in.Vice} {
			if code != 0 && !inXI[code] {
				return fmt.Errorf("player %d wears an armband from outside the eleven", code)
			}
		}
		if in.Captain != 0 && in.Captain == in.Vice {
			return fmt.Errorf("the captain and the vice-captain are the same player")
		}
	}
	return nil
}
