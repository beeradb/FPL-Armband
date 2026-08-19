package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/mail"
	"os"
	"strings"

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
		"connect-src 'self'",
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

// gate accepts the landing page's email form and lets the reader through.
//
// It accepts anything shaped like an address and STORES NOTHING. That is the whole
// behaviour, and it is deliberate on both halves.
//
// Accepting anything, because the gate is a placeholder for a signup flow that does not
// exist yet, and a gate that refused would be pretending to a capability nothing behind it
// has. Storing nothing, because the alternative is a file of personal data sitting in a
// working tree with no retention answer, created before anyone decided one was wanted.
// When the real flow lands it can decide; until then there is nothing here to leak.
//
// The address is deliberately not logged either. "We only wrote it to stderr" is still
// having written it down.
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "unreadable form", http.StatusBadRequest)
		return
	}
	// Shape only, and only so the page can say something useful about an obvious typo.
	// Nothing downstream depends on the answer.
	addr := strings.TrimSpace(r.PostFormValue("email"))
	if _, err := mail.ParseAddress(addr); err != nil {
		http.Error(w, "that does not look like an email address", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     gateCookieName,
		Value:    "1",
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		HttpOnly: true,
	})
	fmt.Fprintln(os.Stderr, dim("gate: a submission was accepted and discarded"))
	http.Redirect(w, r, routeApp, http.StatusSeeOther)
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
// first GET mints one and stores it, and every later GET is a pure read.
func (s *squadServer) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := readSession(r)
	if sess.Seed == 0 && !sess.Optimised {
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
