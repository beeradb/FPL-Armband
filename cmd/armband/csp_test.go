package main

import (
	"net/http"
	"strings"
	"testing"
)

// TestTheDocumentsCarryAPolicyThatForbidsInlineScript pins the header the out-of-line
// scripts exist for.
//
// Escaping is the primary guard against a player name reaching the DOM as markup, and it
// is tested through a real browser. But escaping is one missed sink away from failing, and
// three were missed on the first pass of this very change — so the policy is what stands
// behind it: with no inline script permitted, a name that does reach the DOM as markup
// still cannot run, and connect-src 'self' means it could not send the squad anywhere if
// it did.
//
// The assertion that matters is the ABSENCE of 'unsafe-inline' in script-src. A policy
// carrying it is worse than none, because it reads like protection and provides almost
// none: whatever an injection opens, it permits.
func TestTheDocumentsCarryAPolicyThatForbidsInlineScript(t *testing.T) {
	s := &squadServer{}
	for _, path := range []string{"/", "/app"} {
		csp := get(t, s, path).Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Errorf("GET %s carries no Content-Security-Policy", path)
			continue
		}

		directives := map[string]string{}
		for _, d := range strings.Split(csp, ";") {
			d = strings.TrimSpace(d)
			if name, value, ok := strings.Cut(d, " "); ok {
				directives[name] = value
			} else {
				directives[d] = ""
			}
		}

		script, ok := directives["script-src"]
		if !ok {
			t.Errorf("GET %s: the policy names no script-src", path)
		} else if strings.Contains(script, "unsafe-inline") || strings.Contains(script, "unsafe-eval") {
			t.Errorf("GET %s: script-src is %q. A policy that permits inline script permits "+
				"whatever an injected string opens, which is most of the value gone — and "+
				"it is why app.js and landing.js are separate files.", path, script)
		}

		// connect-src is the one directive that differs by document, so it is
		// asserted below rather than here. The landing page reaches the signup
		// origin; the application must not.
		for name, want := range map[string]string{
			"default-src":     "'self'",
			"frame-ancestors": "'none'",
			"base-uri":        "'none'",
		} {
			if got := directives[name]; got != want {
				t.Errorf("GET %s: %s is %q, want %q", path, name, got, want)
			}
		}

		// ⚠️ The APPLICATION's connect-src must stay exactly 'self'. That is the
		// directive this whole test is written around: /app renders FPL's prose and
		// player names by innerHTML, so it is the document an injected string could
		// ride into, and 'self' is what stops such a string sending the squad
		// anywhere. The landing page renders nothing untrusted and carries the one
		// named peer its signup form posts to.
		want := "'self'"
		if path == "/" {
			want = "'self' " + signupOrigin
		}
		if got := directives["connect-src"]; got != want {
			t.Errorf("GET %s: connect-src is %q, want %q", path, got, want)
		}
	}
}

// TestTheAssetRouteIsNotAFileBrowser pins two things http.FileServer does by default that
// this route should not.
func TestTheAssetRouteIsNotAFileBrowser(t *testing.T) {
	s := &squadServer{}

	// A directory listing is HTML nobody wrote, served from a route whose every other
	// answer is a stylesheet or a font.
	for _, path := range []string{"/assets/", "/assets/fonts/"} {
		if w := get(t, s, path); w.Code != http.StatusNotFound {
			t.Errorf("GET %s answered %d and served a directory listing; want 404",
				path, w.Code)
		}
	}

	// And every asset gets the same sniffing protection the documents get.
	for _, path := range []string{
		"/assets/armband.css",
		"/assets/app.js",
		"/assets/fonts/inter-latin.woff2",
	} {
		w := get(t, s, path)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s answered %d", path, w.Code)
			continue
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s answered X-Content-Type-Options %q, want nosniff", path, got)
		}
	}
}
