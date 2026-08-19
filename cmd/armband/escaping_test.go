package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/viewmodel"
)

// The escaping guarantee, tested through a real browser.
//
// # Why this is not a source scan
//
// The client renders by assigning innerHTML — 24 times — and the strings it interpolates
// are player names, FPL's own `news` prose, and the reasoning attached to an override.
// None of those is written by this program. A grep for esc() would prove that the helper
// is called somewhere; only rendering the page proves that a name containing markup comes
// out as text.
//
// # Why it matters here more than it did before
//
// The Go template this application replaces escaped everything for free, and
// internal/present pins that with TestEveryNameIsEscapedInEveryView. Moving rendering to
// the client dropped the guarantee silently: nothing failed, no test noticed, and the page
// looked identical — because the mock data the design shipped with contained no markup.
//
// A name is not a hypothetical injection point. FPL publishes what clubs register, this
// project already carries players with accents, apostrophes and hyphens, and `news` is
// free prose FPL writes.

// nastyName is what a hostile registration would look like. Three payloads in one string:
// a tag that fires without any user interaction, an attribute-breaking quote, and a
// closing tag that would end the element early if the value were pasted in raw.
const nastyName = `<img src=x onerror="window.__pwned=1">"><script>window.__pwned=2</script>`

// TestAPlayerNamedLikeMarkupRendersAsText loads the real page in a real browser with a
// player whose name is an attack, and asserts the DOM came out inert.
func TestAPlayerNamedLikeMarkupRendersAsText(t *testing.T) {
	browser := chromium(t)

	renamed := false
	s := fixtureServerNamed(t, func(name string) string {
		// One player only. Renaming the whole league would work too, but a single
		// hostile name among six hundred ordinary ones is the realistic shape and it
		// keeps the rest of the page recognisable in a failure.
		if !renamed && name != "" {
			renamed = true
			return nastyName
		}
		return name
	})
	if !renamed {
		t.Fatal("no player was renamed; the fixture has no elements")
	}

	srv := httptest.NewServer(s)
	defer srv.Close()

	// The API must carry the name FAITHFULLY. Escaping belongs at the point of
	// rendering, not in the data: an API that pre-escaped would be lying about what FPL
	// published, and would double-escape the moment a second client did its own job.
	//
	// Asserted on the DECODED value rather than the wire bytes. Go's encoding/json
	// escapes "<" to "\u003c" by default, so the raw body never contains the literal
	// even when the name is carried perfectly — an assertion on the bytes would fail
	// against correct behaviour, which is how a guard gets deleted instead of fixed.
	w := get(t, s, "/api/state")
	if w.Code != 200 {
		t.Fatalf("GET /api/state answered %d", w.Code)
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the state did not decode: %v", err)
	}
	var carried bool
	for _, p := range st.Squad.Players {
		if p.Name == nastyName {
			carried = true
		}
	}
	for _, r := range st.Market.Rows {
		if r.Player.Name == nastyName {
			carried = true
		}
	}
	if !carried {
		t.Fatal("the hostile name reached neither the squad nor the market, so the DOM " +
			"assertions below would prove nothing. Either the API altered the name — " +
			"escaping belongs at the render, not in the data — or the renamed player is " +
			"not drawn on this page and the test needs a different one.")
	}

	dom := dumpDOM(t, browser, srv.URL+"/app#players")

	// The payload must be present as TEXT — if it vanished entirely the test would pass
	// for the wrong reason, having proved only that the player was filtered out.
	if !strings.Contains(dom, "&lt;img src=x") && !strings.Contains(dom, "&lt;img") {
		t.Errorf("the hostile name does not appear escaped in the DOM at all. Either the " +
			"player is not rendered — in which case this test proves nothing and needs a " +
			"different player — or the escaping removed rather than neutralised it.")
	}
	// And it must not have become elements.
	for _, live := range []string{
		`<img src=x onerror`,
		`<script>window.__pwned`,
	} {
		if strings.Contains(dom, live) {
			t.Errorf("the DOM contains live markup from a player's name: %q\n"+
				"A name is not a hypothetical injection point — FPL publishes what clubs "+
				"register, and this page renders by innerHTML.", live)
		}
	}
}

// TestOverrideProseRendersAsText is the same guarantee on the other free-text channel.
//
// Override reasoning is the one string on the page that a HUMAN types, and stage two makes
// it writable from the browser. The reason it is safe today is that it round-trips through
// config, where sanitiseReason bounds it — but sanitiseReason strips control characters and
// caps the length, and does nothing whatever about markup. Rendering is the only thing
// standing between a typed reason and the DOM.
func TestOverrideProseRendersAsText(t *testing.T) {
	browser := chromium(t)

	s := fixtureServer(t)
	cfg := *s.cfg
	cfg.Roster.Minutes[0].Reason = "He is nailed " + nastyName
	s.cfg = &cfg

	srv := httptest.NewServer(s)
	defer srv.Close()

	dom := dumpDOM(t, browser, srv.URL+"/app#overrides")
	if !strings.Contains(dom, "He is nailed") {
		t.Fatal("the override's reasoning is not on the page, so this proves nothing")
	}
	if strings.Contains(dom, `<img src=x onerror`) || strings.Contains(dom, `<script>window.__pwned`) {
		t.Error("an override's reasoning reached the DOM as live markup. Stage two makes " +
			"this field writable from the browser.")
	}
}

// dumpDOM renders a URL and returns the DOM the browser ended up with.
//
// --dump-dom rather than a screenshot, because the question here is what the parser built,
// not what it looked like. An injected <script> is invisible in a picture.
func dumpDOM(t *testing.T, browser, url string) string {
	t.Helper()
	work := scratch(t)
	out := filepath.Join(work, "dom.html")

	cmd := exec.Command(browser,
		"--headless",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		// Long enough for the fetch and the render. The same fast-forward the
		// screenshots use, so neither depends on how loaded the machine is.
		"--virtual-time-budget=8000",
		"--dump-dom",
		url,
	)
	cmd.Env = append(os.Environ(), "HOME="+work)
	body, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium could not render %s: %v", url, err)
	}
	if len(body) < 500 {
		t.Fatalf("chromium returned %d bytes of DOM for %s; the page did not render",
			len(body), url)
	}
	if err := os.WriteFile(out, body, 0o644); err != nil {
		t.Logf("could not keep the DOM for inspection: %v", err)
	}
	return string(body)
}
