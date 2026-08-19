package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

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
// cross-origin locally, which is what the CORS echo in allowLoopbackOrigin is for.
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

// servePage writes one of the embedded documents.
func (s *squadServer) servePage(w http.ResponseWriter, name string) {
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
	_, _ = w.Write(body)
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

// state answers the client contract.
func (s *squadServer) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	so := readSessionOverrides(r)
	cfg := s.effectiveCfgFrom(so)
	b, err := buildSquadPage(r.Context(), cfg, s.client, s.engine, s.weeks, true, s.now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	b.Page.Token = s.token
	b.Page.SessionMode = !s.persist
	if !s.persist {
		markSessionOverrides(&b.Page, so)
	}

	st, err := viewmodel.Build(viewmodel.Input{
		Page:    b.Page,
		Boot:    s.engine.Boot,
		Cfg:     cfg,
		Now:     s.now(),
		Persist: s.persist,
	})
	if err != nil {
		// Build's only failure is a number encoding/json would refuse. Answering 500
		// with its message names the field; letting the encoder fail would produce a
		// half-written 200 the client would try to parse.
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Marshalled to a buffer before anything is written, so an encoding failure answers
	// a status the client can act on rather than truncating a 200 mid-document. The
	// served page already does this for the same reason.
	raw, err := json.Marshal(st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: marshalling the state: %v\n", err)
		http.Error(w, "encoding the state failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The state is rebuilt per request and must never be reused: a cached squad is the
	// bug the whole no-cache design of this server exists to avoid.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}
