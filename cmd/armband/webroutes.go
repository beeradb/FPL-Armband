package main

import (
	"bytes"
	"context"
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
	"armband/internal/fpl"
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
	// routeAbout is the marketing/gate document, landing.html -- moved off "/" when the
	// app itself became the root, so it still has an address for whoever links to it.
	routeAbout   = "/about"
	routeGate    = "/gate"
	routeState   = "/api/state"
	routeSession = "/api/session"
	// routeArmbandTeam and routeArmbandTeamState are the spectator page: what the site's
	// own squad (config.EntryID) is actually doing, always, for anyone. Deliberately not a
	// data-view inside /app -- it must never share a document with the interactive builder,
	// so a reader (or a link) cannot land on the tool's team-selection surface by accident
	// while looking for the house team. Ungated like the landing page, for the same reason:
	// proof-of-use is a marketing surface, not the product behind the gate. See armbandteam.go.
	routeArmbandTeam      = "/armband-team"
	routeArmbandTeamState = "/api/armband-team"
	// routeImport is the team-ID import write path. See importteam.go.
	routeImport = "/api/import"
	// routeResults is the on-demand per-gameweek results read for the SESSION's own
	// imported entry — the results document buildResults assembles, generalised off
	// armbandTeamState's config.EntryID path to any reader who has imported a team. See
	// results.go.
	routeResults = "/api/results"
	// routeTransfers is the on-demand transfer-suggestion read for the SESSION's own
	// imported entry and current pitch — GET only, no CSRF token, computed fresh on every
	// request. See apitransfers.go.
	routeTransfers = "/api/transfers"
	// routeMetrics serves this process's own Prometheus metrics — the
	// staleness signal behind internal/fpl.Client's deliberate stale-fallback,
	// plus the HTTP and pipeline-timing series alongside it. See metrics.go's
	// doc comment for why it needs no token: keeping it off the public
	// internet is the deployment's job, not this handler's.
	routeMetrics = "/metrics"
	prefixAssets = "/assets/"

	// prefixPlayer serves one footballer's history behind /api/player/{code} — a code, not
	// the season-scoped element id every other route here uses. See viewmodel.PlayerDetail's
	// doc comment for why a read still keys on the permanent identifier. A prefix route,
	// like assets, because the code varies; see playerdetail.go.
	prefixPlayer = "/api/player/"
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
// on landing.html's gate forms' data-gate attribute, where assets/static/gate.js reads it
// to decide where the fetch actually goes. One quantity with two implementations is this
// project's signature failure, so the two are pinned equal by
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
//
// ⚠️ THE FPL API CANNOT BE ADDED HERE, AND THE REASON IS NOT THE ONE ABOVE. The paragraph
// above is an XSS argument, which invites the reading that a sufficiently careful case
// could justify letting the browser call fantasy.premierleague.com directly and save the
// server a round trip. It could not, because the browser would be refused anyway:
//
//	curl -sI -H 'Origin: https://fplarmband.com' \
//	  https://fantasy.premierleague.com/api/bootstrap-static/
//
// answers 200 with NO Access-Control-Allow-Origin of any kind, and additionally
// cross-origin-resource-policy: same-origin. Measured 2026-08-22. So a fetch from the page
// completes and the response is then withheld from it — widening this directive buys a
// broken feature and a weaker policy, in that order. Anything the client needs from FPL is
// proxied by this server; GET /api/results is the pattern.
//
// ga4 adds a second, independent exception to the same directive, and it is ALWAYS false
// for page != "landing" -- checked inside this function, not only by the caller, so a
// mistake in servePage's call cannot widen the application's policy. See scriptSrcFor for
// the other directive GA4 needs alongside this one; they travel together because a script
// that ships but cannot POST its events is a widened policy spent on tracking nobody.
func connectSrcFor(page string, ga4 bool) string {
	if page != "landing" {
		return "connect-src 'self'"
	}
	// Named rather than wildcarded, and one host. It buys an injected string on
	// this page exactly one destination, under the same ownership as this binary.
	// Adding a second needs a reason of the same kind — 'self' plus a named peer
	// is a policy; a list of convenient hosts is not.
	src := "connect-src 'self' " + signupOrigin
	if ga4 {
		// GA4 posts measurement events to a subdomain of google-analytics.com, and
		// the gtag script loaded from googletagmanager.com makes its own further
		// fetches back to itself -- both wildcarded because Google serves this
		// traffic from a rotating subdomain, unlike signupOrigin's one fixed host.
		src += " https://*.google-analytics.com https://*.googletagmanager.com"
	}
	return src
}

// scriptSrcFor gains exactly one host, and only on the landing page with GA4 configured:
// googletagmanager.com, where the GA4 loader analytics.js asks the browser to run is
// served from. /app never widens this directive under any config; see connectSrcFor's
// doc comment for why that split is load-bearing.
func scriptSrcFor(page string, ga4 bool) string {
	if page == "landing" && ga4 {
		return "script-src 'self' https://www.googletagmanager.com"
	}
	return "script-src 'self'"
}

// ga4EnvVar is spelled once and read from both the CSP decision (servePage, through
// ga4Configured) and the meta-tag fill (withGA4) -- one quantity, named once, so a rename
// cannot desync the widened policy from the id that policy exists to permit.
const ga4EnvVar = "ARMBAND_GA4_ID"

// ga4Configured reports whether this process should widen the landing page's policy and
// load the GA4 script. Read fresh per request rather than cached at startup: env is the
// one channel this binary takes config from that nothing here watches for a change, and a
// per-request Getenv costs nothing next to the rest of this handler.
func ga4Configured() bool {
	return os.Getenv(ga4EnvVar) != ""
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
	// gate.js are separate files rather than inline blocks -- a policy that permits
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
	//
	// ga4 is computed once here and threaded through the three *For functions below,
	// rather than each of them calling ga4Configured itself: it is deliberately forced to
	// false by `name == "landing" &&`, so a landing-only widening cannot leak onto /app
	// even if ga4Configured somehow answered true while name was "app".
	ga4 := name == "landing" && ga4Configured()
	w.Header().Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		scriptSrcFor(name, ga4),
		"style-src 'self' 'unsafe-inline'",
		// img-src does not widen for GA4: gtag.js reports over fetch/XHR, not an
		// <img> beacon, so there is nothing here that would use a wider img-src --
		// unlike script-src and connect-src, which the loader and its reporting
		// genuinely need.
		"img-src 'self' data:",
		"font-src 'self'",
		connectSrcFor(name, ga4),
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"form-action 'self'",
	}, "; "))
	// The pages are HTML and the assets are typed by extension; nothing here should ever
	// be re-interpreted as another type on a sniffing browser.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The GA4 meta tag is filled BEFORE the auth branch, deliberately -- unlike the
	// token, which only ever fills on the authed write below. The landing page's real
	// audience is an anonymous first-time visitor, who takes the !authed branch a few
	// lines down and would never reach withToken; gating this fill the same way would
	// mean GA4 silently never fires for the traffic it exists to measure. Applying it
	// here, once, covers both branches without duplicating the fill.
	if name == "landing" {
		body = withGA4(body)
	}
	// Unconditional, unlike withToken below: the signup ask is not a capability, so there
	// is no authed/anonymous split to gate it on -- an anonymous visitor is exactly who it
	// exists to reach. See withSignups' own comment for what it fills and why "landing"
	// never gets a call: that document carries its own gate form already.
	if name == "app" {
		// withSignups makes these body bytes depend on the request's signup cookie, so a
		// cache sitting in front of this process (the deployment caches "/" for 60s --
		// see instruments.go's comment on httpRequests) must not serve one visitor's
		// filled-or-empty meta tag to the next visitor with different cookie state. Vary
		// is scoped to this branch, not set for every page: /about's body never depends
		// on the cookie, so it carries no Vary and stays as cacheable as before.
		w.Header().Set("Vary", "Cookie")
		body = s.withSignups(r, body)
	}
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

// ga4Meta is the landing page's twin of tokenMeta, filled the same way -- but see
// servePage's call site for why it is NOT gated on s.authed the way withToken is. This
// tag exists only in landing.html; app.html carries no placeholder for it.
//
// The embedded file ships with the placeholder EMPTY, so a copy of the document taken
// off disk, or a local `armband serve` with ARMBAND_GA4_ID unset, carries no id at all.
const ga4Meta = `<meta name="armband-ga4" content="">`

// withGA4 fills the landing page's GA4 meta tag from ga4EnvVar. Unlike withToken this is
// a package function rather than a method: the id comes from the process environment,
// not from anything held on squadServer.
func withGA4(body []byte) []byte {
	id := os.Getenv(ga4EnvVar)
	if id == "" {
		return body
	}
	filled := `<meta name="armband-ga4" content="` + html.EscapeString(id) + `">`
	return bytes.Replace(body, []byte(ga4Meta), []byte(filled), 1)
}

// signupsMeta is app.html's placeholder for whether the client should offer an email ask.
// landing.html carries no copy of this tag: it has its own gate form already, and asking a
// visitor there a second way to give the same address would be the one page doubling up on
// itself.
//
// The embedded file ships with the placeholder EMPTY, so a copy of the document taken off
// disk, or a build where the fill never runs, asks for nothing -- the safe direction, since
// the alternative is a form the client cannot tell is dead.
const signupsMeta = `<meta name="armband-signups" content="">`

// withSignups fills app.html's signup-ask meta tag with "1" exactly when there is something
// to gain by asking: a store is configured (otherwise /gate 503s on every submission -- an
// operator's local `armband serve` with no signup DSN set, which must never be shown a form
// that cannot succeed) AND this request does not already carry the signup cookie (otherwise
// the ask is repeated at someone who has already given an address). Unlike withToken it is
// not gated on s.authed: the ask has nothing to do with the write capability, and per-request
// cookie state is exactly why this is a method rather than a package function the way
// withGA4 is -- withGA4 reads only the process environment.
func (s *squadServer) withSignups(r *http.Request, body []byte) []byte {
	if s.signups == nil || s.hasSignedUp(r) {
		return body
	}
	filled := `<meta name="armband-signups" content="1">`
	return bytes.Replace(body, []byte(signupsMeta), []byte(filled), 1)
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
	// means "recorded" to gate.js, so a deployment that had lost its database URL —
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
		Name:     signupCookieName,
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
// and sets no cookies. The signup cookie is worth nothing locally anyway: the app is open
// and reachable directly, with no email required first.
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

// signupCookieName marks a reader who has already given an email address through the
// landing form.
//
// ⚠️ The Go identifier changed (it was gateCookieName) but the wire value did NOT: it is
// still "fpl_gate", not "fpl_signup". The app no longer gates on anything, so nothing here
// is a gate any more -- but a reader who already carries the old cookie from before this
// change shipped must not be asked again, and the cookie's identity to a browser is its
// string value, never the Go name pointing at it. Renaming the constant without renaming
// the wire value is deliberate for exactly that reason.
const signupCookieName = "fpl_gate"

// hasSignedUp reports whether the request carries the signup form's cookie -- nothing more.
//
// It used to gate /app and /api/import; it no longer gates anything, because there is
// nothing left in this application that requires an email address first. What it answers
// now is narrower and purely informational: has this visitor already given one, so the
// client knows whether asking again would be pointless. See withSignups, its one caller.
func (s *squadServer) hasSignedUp(r *http.Request) bool {
	c, err := r.Cookie(signupCookieName)
	return err == nil && c.Value != ""
}

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
	sess := s.readValidSession(r)
	// The free-transfer allowance needs one FPL round trip, resolved BEFORE the render
	// lock — see freeTransfersFor's own comment for why that ordering may not move.
	free, hist, freeErr := s.freeTransfersFor(r.Context(), sess)

	defer s.lockRender("state")()

	if sess.Seed == 0 && !sess.Optimised && s.authed(r) {
		sess.Seed = s.nextSeed()
		if err := sess.write(w); err != nil {
			fmt.Fprintf(os.Stderr, "serve: storing the session seed: %v\n", err)
		}
	}
	s.answerState(w, r, sess, free, hist, freeErr)
}

// answerState builds and writes the document for a session. Shared by the read and the
// write route so the two cannot disagree about what the state IS.
func (s *squadServer) answerState(w http.ResponseWriter, r *http.Request, sess session, free int, hist *fpl.EntryHistory, freeErr error) {
	body, err := s.buildState(r, sess, free, hist, freeErr)
	if err != nil {
		// The full error is for whoever is running the server -- a viewmodel marshal
		// failure like "State.Squad.Players[3].XP is NaN, which encoding/json cannot
		// marshal" names a Go type, not something the browser's reader did or can
		// fix. The response says only that the build failed and what to try.
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		http.Error(w, "the squad could not be built just now — try reloading", http.StatusInternalServerError)
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

// freeTransfersFor resolves the free-transfer allowance for a session's imported entry, or
// fpl.UnlimitedTransfers when there is nothing to resolve (no imported entry, or the read
// failed — see fpl.FreeTransfers' own doc comment for why an unknown allowance answers
// unlimited rather than a guess of 1). A non-nil error means the read failed and the caller
// should carry that forward as State.Transfers.FreeUnknown, rather than silently treating
// it the same as "before GW1".
//
// It also returns the *fpl.EntryHistory it read (nil alongside the unlimited/error cases
// above) — the SAME cached round trip, not a second one — because buildState needs it for
// one more thing beyond the free-transfer count: fpl.EarliestResultEvent, the bound the
// gameweek rail's past-tab tabs use (see viewmodel.Input.EarliestResultEvent's own
// comment). A caller that only needs the count is free to ignore the second value.
//
// It is a method squadServer's three buildState callers (state, saveSession, importTeam)
// each invoke BEFORE taking the render lock, and buildState itself takes the resolved
// answer as a parameter rather than calling this. buildState runs under s.mu, this makes an
// outbound FPL call, and importTeam's own step 6 states the rule this exists to keep
// visible: holding the render lock across an outbound call would stall every OTHER
// visitor's render behind this one's network latency.
func (s *squadServer) freeTransfersFor(ctx context.Context, sess session) (int, *fpl.EntryHistory, error) {
	if sess.Entry == 0 || s.client == nil {
		return fpl.UnlimitedTransfers, nil, nil
	}
	h, err := s.client.History(ctx, sess.Entry)
	if err != nil {
		return fpl.UnlimitedTransfers, nil, err
	}
	return fpl.FreeTransfers(h), h, nil
}

// buildState produces the document for a session, or an error naming what went wrong.
//
// free/hist/freeErr are the free-transfer allowance and the history it was read from,
// resolved by the caller via freeTransfersFor BEFORE the render lock — see that method's
// own comment.
func (s *squadServer) buildState(r *http.Request, sess session, free int, hist *fpl.EntryHistory, freeErr error) ([]byte, error) {
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

	now := s.now()
	st, err := viewmodel.Build(viewmodel.Input{
		Page:      b.Page,
		Boot:      s.engine.Boot,
		Cfg:       cfg,
		Now:       now,
		Persist:   s.persist,
		Session:   sess.arrangement(),
		Optimised: sess.Optimised,
		Saved:     len(sess.Squad) == 15,
		// The same condition saveSession applies, so the page offers exactly the controls
		// the write route will accept. Deriving it here from the request rather than
		// re-deciding it in the client is what keeps the two from disagreeing.
		Writable: !s.sessionWriteNeedsToken() || s.tokenOK(r.Header.Get("X-Armband-Token")) ||
			s.authed(r),
		// cfg.Chips is the exact schedule buildSquadPage installed on the engine for
		// this request (session placements included — effectiveCfgFrom already
		// merged them), so the rail's chip window agrees with the week views built
		// from the same schedule.
		Chips:           cfg.Chips,
		NewsChecked:     newsChecked(s.client, now),
		NewsReadChecked: newsReadChecked(cfg, now),
		OverrideEffects: b.OverrideEffects,
		Import:          buildImport(s.engine.Boot.Events, sess),
		// No Entry/History here on purpose: this is the interactive builder's
		// document, and a manager's record has nothing to do with building it. It is
		// fetched only by the two results documents — armbandTeamState for
		// /armband-team, apiResults for GET /api/results — see those functions.
		//
		// EarliestResultEvent is the one exception, and it is not a manager's record —
		// it is a single bound the rail needs to know how far back its past-gameweek
		// tabs may reach. hist is the SAME History freeTransfersFor already read for
		// the free-transfer count above (no second round trip); fpl.EarliestResultEvent
		// on a nil hist answers 0, which buildGameweeks reads as "add no past tabs".
		EarliestResultEvent: fpl.EarliestResultEvent(hist),
	})
	if err != nil {
		// Build's only failure is a number encoding/json would refuse. Naming the field
		// beats letting the encoder fail into a half-written 200 the client would parse.
		return nil, err
	}
	// Present only for a reader with an imported entry — absent otherwise, which is how
	// the client knows there is nothing to draw. See viewmodel.Transfers' own comment.
	if sess.Entry != 0 {
		st.Transfers = buildTransfersBlock(s.engine, sess, free, freeErr)
	}
	// Marshalled here rather than streamed, so a failure is an error a caller can answer
	// with a status instead of a truncated body.
	return json.Marshal(st)
}

// buildTransfersBlock assembles State.Transfers for a session with an imported entry: the
// free-transfer allowance the caller already resolved, and Squad diffed against Base — see
// session.Base's own doc comment for what the two lists are.
func buildTransfersBlock(e *analysis.Engine, sess session, free int, freeErr error) *viewmodel.Transfers {
	t := &viewmodel.Transfers{
		Free:        free,
		FreeUnknown: freeErr != nil,
		BaseEvent:   sess.BaseEvent,
	}
	if len(sess.Base) == 0 {
		// The only path today that imports a squad and leaves Base empty is a free-hit
		// import (importTeam step 8) — a cookie written before this field existed would
		// read the same way and self-heals on the reader's next import. Both read as "no
		// baseline"; free-hit is the only cause this build can name with any confidence.
		t.NoBaseline = true
		t.FreeHitBase = true
		return t
	}
	// FPL's current event has moved past the week Base was fetched for: the reader's
	// real squad may have changed since, and this diff would describe a week that is
	// over rather than what they actually hold. The count and cost are withheld rather
	// than shown against a baseline this build no longer trusts — see session.Base's own
	// doc comment. importWindow is a pure function over the bootstrap's events, so this
	// costs no network call.
	importEvent, _, _ := importWindow(e.Boot.Events)
	if importEvent > sess.BaseEvent {
		t.BaselineStale = true
		return t
	}
	t.Moves = diffSquadAgainstBase(e, sess.Base, sess.Squad)
	// Hits/Cost are zero when Free is unlimited (nothing to overspend) or unknown (no
	// allowance to compare against) — see viewmodel.Transfers.Hits' own comment.
	if free != fpl.UnlimitedTransfers && !t.FreeUnknown {
		if hits := len(t.Moves) - free; hits > 0 {
			t.Hits = hits
			t.Cost = hits * fpl.HitCost
		}
	}
	return t
}

// diffSquadAgainstBase is a transfer: position-matched Squad against Base, out-for-in.
//
// Both lists are already permanent player codes — session.Base and session.Squad — so this
// is arithmetic over two []int, not a search: group what left by position, group what
// arrived by position, and pair them off within each position. That pairing always exhausts
// both sides, because Base and Squad are each a legal fifteen (2 GKP/5 DEF/5 MID/3 FWD —
// validateSession enforces it on write), so the count leaving a position always equals the
// count arriving in it.
//
// Not internal/backtest.diffSquads, which is a different type over []PlayerMetrics and
// stranded in the replay harness — see the design note this implements for why exporting it
// would be a worse fit than this small, separate function.
func diffSquadAgainstBase(e *analysis.Engine, base, squad []int) []viewmodel.Move {
	inSquad, inBase := map[int]bool{}, map[int]bool{}
	for _, c := range squad {
		inSquad[c] = true
	}
	for _, c := range base {
		inBase[c] = true
	}

	metrics := func(code int) (analysis.PlayerMetrics, bool) {
		el := e.Boot.ElementByCode(code)
		if el == nil {
			return analysis.PlayerMetrics{}, false
		}
		return e.Metrics(el), true
	}
	byPosition := func(codes []int) map[string][]int {
		m := map[string][]int{}
		for _, c := range codes {
			if pm, ok := metrics(c); ok {
				m[pm.Position] = append(m[pm.Position], c)
			}
		}
		return m
	}

	var outCodes, inCodes []int
	for _, c := range base {
		if !inSquad[c] {
			outCodes = append(outCodes, c)
		}
	}
	for _, c := range squad {
		if !inBase[c] {
			inCodes = append(inCodes, c)
		}
	}
	outByPos, inByPos := byPosition(outCodes), byPosition(inCodes)

	var moves []viewmodel.Move
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		out, in := outByPos[pos], inByPos[pos]
		n := len(out)
		if len(in) < n {
			n = len(in)
		}
		for i := 0; i < n; i++ {
			om, oOK := metrics(out[i])
			im, iOK := metrics(in[i])
			if !oOK || !iOK {
				continue
			}
			moves = append(moves, viewmodel.Move{
				Pos:     pos,
				OutCode: out[i], OutName: om.Name, OutClub: om.Team, OutPrice: om.Price,
				InCode: in[i], InName: im.Name, InClub: im.Team, InPrice: im.Price,
			})
		}
	}
	return moves
}

// houseTeamSources fetches the site's own manager record for the footer widget —
// config.EntryID's Entry and History, through the same cached client call every other
// caller of Entry/History in this codebase uses for an operator-configured id (see
// Client.EntryUncached's doc comment on why that trust boundary is what makes the disk
// cache safe here).
//
// A fetch failure returns (nil, nil) rather than an error: this is supplementary
// proof-of-use, not the squad the page exists to build, and an FPL hiccup on this call
// must not turn the whole page into a 500. viewmodel.Build already treats a nil Entry as
// "no house team to show" — the same honest-absence rule State.Import's own fields follow.
func houseTeamSources(ctx context.Context, client *fpl.Client, entryID int) (*fpl.Entry, *fpl.EntryHistory) {
	if client == nil || entryID == 0 {
		return nil, nil
	}
	entry, err := client.Entry(ctx, entryID)
	if err != nil {
		return nil, nil
	}
	history, err := client.History(ctx, entryID)
	if err != nil {
		return entry, nil
	}
	return entry, history
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
// # Why the token is required only under -persist
//
// The token exists for one reason, recorded on `authed`: a page is served to anything that
// can make a request, so a token embedded in it is no boundary against a local process — and
// under `-persist` that process could drive a standing override into config.json, which then
// binds every future agent run.
//
// That reasoning is entirely about `-persist`. With it off, a save reaches exactly one thing:
// the caller's own HttpOnly cookie. It cannot touch the file, no other reader can read it,
// and it dies with the browser session. Requiring a token there protects nothing and costs
// the whole feature — a public deployment has no way to hand a token to a reader, so the
// planner would draw every control and refuse every one of them, which is precisely the
// defect this surface was rewritten to remove.
//
// So the gate follows the thing it guards. What stays open is CPU, not data: every save
// rebuilds the page, and Optimize runs the optimiser under the global mutex. That is a
// rate-limiting problem, filed as one, and not an argument for a token nobody can deliver.
//
// ⚠️ The token was ALSO doing a second job, and dropping it without replacing that job is a
// CSRF hole. This route used to accept POST as well as PUT, and a cross-origin page can make
// a POST with a text/plain body and no preflight — a "simple request" — so any page open in
// the reader's browser could have overwritten their stored team the moment the token stopped
// being required. `SameSite=Strict` does not save it: the reader's cookie is withheld, the
// server sees an empty session, and the ANSWER's Set-Cookie replaces the real one.
//
// Two things close it, and both are needed:
//
//   - **PUT only.** PUT is not a simple method, so a cross-origin PUT forces a preflight,
//     and no route here answers OPTIONS. That alone ends the browser attack.
//   - **A same-origin check** where there is no token, as defence in depth for anything that
//     reaches the handler another way.
//
// What is deliberately NOT defended against is a local process with no browser: curl can
// PUT here all day. That is the token's job, and with `-persist` off all it can write is its
// own cookie — which is nobody's session but its own.
func (s *squadServer) sessionWriteNeedsToken() bool { return s.persist }

// sameOrigin reports whether a browser says this request came from this site.
//
// `Sec-Fetch-Site` is set by the browser and cannot be forged by page script, so it is the
// load-bearing half. A request without it is not a browser request — curl, a test, the Wails
// binding — and those are the token's problem rather than CSRF's, so their absence is not
// treated as a failure here.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
		// No browser said anything. Fall through to Origin, which most clients omit too.
	default:
		return false // cross-site or same-site, both of which mean "another origin asked"
	}
	if o := r.Header.Get("Origin"); o != "" {
		return strings.TrimPrefix(strings.TrimPrefix(o, "https://"), "http://") == r.Host
	}
	return true
}

func (s *squadServer) saveSession(w http.ResponseWriter, r *http.Request) {
	// Clamped before the mutex and before the token check, for the same reason /action
	// clamps: the body is the one thing a caller can make arbitrarily expensive.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)

	// PUT only. POST was accepted here and it was a hole: a cross-origin POST with a
	// text/plain body is a "simple request", so it is sent with no preflight and any page in
	// the reader's browser could have replaced their stored team. PUT forces a preflight
	// that nothing here answers.
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "PUT")
		http.Error(w, "the session takes a PUT", http.StatusMethodNotAllowed)
		return
	}

	// Refused BEFORE the mutex, and that ordering is the point.
	//
	// The lock serialises every render on this server, and a render is seconds. Checking
	// admission after taking it means a request that is going to be refused still queues
	// behind one — so the cheapest request an attacker can send costs a mutex acquisition,
	// and a flood of them is a denial of service made entirely of 403s. Neither check reads
	// server state that the lock protects: one is a constant-time compare against a token
	// fixed at startup, the other reads request headers.
	if s.tokenOK(r.Header.Get("X-Armband-Token")) {
		// The token is the strongest claim available and settles it.
	} else if s.sessionWriteNeedsToken() {
		http.Error(w, "missing or wrong token", http.StatusForbidden)
		return
	} else if !sameOrigin(r) {
		http.Error(w, "cross-origin writes are refused", http.StatusForbidden)
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

	// The free-transfer allowance, resolved BEFORE the render lock — decoding and
	// validating the body above need neither the lock nor the network, so this is the
	// earliest point `in.Entry` is known. See freeTransfersFor's own comment for why the
	// FPL call may not happen under s.mu.
	free, hist, freeErr := s.freeTransfersFor(r.Context(), in)

	defer s.lockRender("session")()

	// Under -persist the corrections leave the session and enter the file.
	//
	// Done BEFORE the state is built, so the document this request answers with is rendered
	// from the config the reader will actually get back on their next load, rather than
	// from a session that is about to be emptied.
	if s.persist {
		var err error
		if in, err = s.persistCorrections(in); err != nil {
			// The empty-cfgPath case is already a fixed, safe sentence (see
			// persistCorrections). A real write failure -- config.Save wrapping
			// os.CreateTemp/os.Rename -- carries the server's own filesystem path in
			// its *PathError string, e.g. "saving config: open
			// /home/alice/.config/fplagent/.config-183920481.json: permission
			// denied". Same treatment as answerState and saveSession's buildState
			// branch: log it, tell the reader only that it did not land.
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
			http.Error(w, "that could not be saved just now — try again", http.StatusInternalServerError)
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
	body, err := s.buildState(r, in, free, hist, freeErr)
	if err != nil {
		// Same reasoning as answerState: log the Go error, tell the reader only that
		// the save did not land.
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		http.Error(w, "that change could not be saved just now — try again", http.StatusInternalServerError)
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
		// Said without the flag name: this reaches a browser, and "-persist" answers a
		// question only the person who started the server can act on. What the reader
		// needs is the fact, not the switch that controls it.
		return in, fmt.Errorf("there is nowhere set up to save your corrections " +
			"permanently, so they were not saved")
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
			}, nil); err != nil {
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
		// Base is a code list arriving from a cookie a hand-crafted PUT /api/session
		// could set to anything, same as every other list here — see this function's
		// own doc comment on why Entry gets the same re-check treatment.
		"base": in.Base,
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
	// Entry is a raw int straight out of a cookie a hand-crafted PUT /api/session could
	// set to anything. It is re-checked against fpl.ParseEntryID's own range rather than
	// trusted because it was in the session already — the import route validated it once,
	// on the way in, but this route (and readValidSession, on every later read) must not
	// assume every session it is handed came from there.
	if in.Entry != 0 && !fpl.EntryIDInRange(in.Entry) {
		return fmt.Errorf("entry id %d is out of range", in.Entry)
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
