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
	for _, path := range []string{"/", "/about"} {
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
		// directive this whole test is written around: the app (now served at "/")
		// renders FPL's prose and player names by innerHTML, so it is the document an
		// injected string could ride into, and 'self' is what stops such a string
		// sending the squad anywhere. The landing page (/about) renders nothing
		// untrusted and carries the one named peer its signup form posts to.
		want := "'self'"
		if path == routeAbout {
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

// TestGA4WidensOnlyTheLandingPagesCSPWhenConfigured is the test a reviewer checks first,
// per the plan this implements: the app's CSP (served at "/" now) must be provably
// unchanged by ARMBAND_GA4_ID, in every case, configured or not. The app is the page that
// renders FPL's prose and player names by innerHTML, and connect-src 'self' is what stops
// an injected string from exfiltrating the reader's squad if escaping ever fails somewhere
// — GA4 must never be able to move that boundary.
func TestGA4WidensOnlyTheLandingPagesCSPWhenConfigured(t *testing.T) {
	s := &squadServer{}

	baselineApp := get(t, s, "/").Header().Get("Content-Security-Policy")
	baselineLanding := get(t, s, routeAbout).Header().Get("Content-Security-Policy")

	t.Setenv("ARMBAND_GA4_ID", "G-TESTID123")

	widenedApp := get(t, s, "/").Header().Get("Content-Security-Policy")
	widenedLanding := get(t, s, routeAbout).Header().Get("Content-Security-Policy")

	// The one assertion that matters most: the app is BYTE-IDENTICAL, configured or not.
	if widenedApp != baselineApp {
		t.Fatalf("the app's CSP changed when ARMBAND_GA4_ID was set.\nbefore: %s\nafter:  %s",
			baselineApp, widenedApp)
	}

	if widenedLanding == baselineLanding {
		t.Fatal("the landing page's CSP did not change when ARMBAND_GA4_ID was set")
	}
	for _, want := range []string{
		"https://www.googletagmanager.com", // script-src: the GA4 loader itself
		"https://*.google-analytics.com",   // connect-src: measurement events
		"https://*.googletagmanager.com",   // connect-src: the loader's own fetches
	} {
		if !strings.Contains(widenedLanding, want) {
			t.Errorf("landing CSP with ARMBAND_GA4_ID set does not contain %q: %s",
				want, widenedLanding)
		}
	}
	// img-src does NOT widen for GA4: gtag.js reports over fetch/XHR, not an <img>
	// beacon, so there is nothing that would use a wider img-src. Widening it anyway
	// was caught by security review as unused capability and removed -- this pins that
	// img-src stays exactly what the unconfigured page already carries.
	var imgSrc string
	for _, d := range strings.Split(widenedLanding, ";") {
		d = strings.TrimSpace(d)
		if strings.HasPrefix(d, "img-src ") {
			imgSrc = d
			break
		}
	}
	if imgSrc != "img-src 'self' data:" {
		t.Errorf("landing img-src with GA4 configured is %q, want it unwidened", imgSrc)
	}
	// script-src alone must gain exactly the one named host, never 'unsafe-inline' or
	// 'unsafe-eval' — scoped to that one directive, because style-src legitimately
	// carries 'unsafe-inline' already (the landing page's inline <style> block, unrelated
	// to GA4) and a whole-header substring check would false-positive on it.
	var scriptSrc string
	for _, d := range strings.Split(widenedLanding, ";") {
		d = strings.TrimSpace(d)
		if strings.HasPrefix(d, "script-src ") {
			scriptSrc = d
			break
		}
	}
	if scriptSrc == "" {
		t.Fatalf("landing CSP with GA4 configured names no script-src: %s", widenedLanding)
	}
	if strings.Contains(scriptSrc, "unsafe-inline") || strings.Contains(scriptSrc, "unsafe-eval") {
		t.Errorf("landing script-src with GA4 configured is %q, which permits inline/eval "+
			"script", scriptSrc)
	}

	// Unset again — a leftover widening after the env var is cleared would mean the
	// policy is not actually reading it live.
	t.Setenv("ARMBAND_GA4_ID", "")
	restoredLanding := get(t, s, routeAbout).Header().Get("Content-Security-Policy")
	if restoredLanding != baselineLanding {
		t.Errorf("landing CSP with ARMBAND_GA4_ID unset again is %q, want the original %q",
			restoredLanding, baselineLanding)
	}
}
