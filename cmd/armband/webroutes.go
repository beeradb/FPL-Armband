package main

import (
	"encoding/json"
	"fmt"
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
	prefixAssets = "/assets/"
)

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
	// The pages are HTML and the assets are typed by extension; nothing here should ever
	// be re-interpreted as another type on a sniffing browser.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
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
