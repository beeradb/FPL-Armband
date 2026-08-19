package webui

import (
	"regexp"
	"strings"
	"testing"
)

// TestNoPageCarriesInlineScript is what keeps the Content-Security-Policy satisfiable.
//
// The policy sets `script-src 'self'` with no 'unsafe-inline'. That is only tenable while
// nothing in these documents is inline script — and the natural way to add a handler to a
// page is `onclick="..."`, which is exactly what the policy forbids. Whoever does it will
// see the control silently stop working in a browser and nothing at all in the tests.
//
// Both pages shipped with inline handlers: the landing form's two `onsubmit` attributes,
// which also meant the gate submitted nowhere. They are in a file now.
//
// Inline <style> is deliberately NOT checked. The policy allows it, because a stylesheet
// buys a defacement where a script buys a same-origin read of the reader's squad, and the
// landing page carries a large one.
func TestNoPageCarriesInlineScript(t *testing.T) {
	// An event-handler attribute, and a <script> without a src.
	handler := regexp.MustCompile(`(?i)\son[a-z]+\s*=\s*["']`)
	inline := regexp.MustCompile(`(?i)<script(?:\s+(?:defer|async|type="[^"]*")\s*)*>`)

	for _, name := range pages {
		body, err := Page(name)
		if err != nil {
			t.Fatalf("Page(%q): %v", name, err)
		}
		src := scannable(body)

		if m := handler.FindString(src); m != "" {
			t.Errorf("%s carries the inline handler %q. script-src 'self' forbids it, so "+
				"it will not run — put it in a file under assets/static and reference it.",
				name, strings.TrimSpace(m))
		}
		if m := inline.FindString(src); m != "" {
			t.Errorf("%s carries an inline <script> block (%q). The policy forbids it; the "+
				"scripts are separate files for exactly this reason.", name, m)
		}
		// And the script it does reference must be one that ships.
		if !strings.Contains(src, `src="/assets/`) {
			t.Errorf("%s references no external script at all", name)
		}
	}
}

// TestTheLandingFormCanActuallyReachTheGate pins a gap that every other test missed.
//
// POST /gate exists, is tested, validates, sets its cookie and redirects. And no shipped
// page could reach it: both forms were `onsubmit="event.preventDefault()"` with no action,
// no method, and no name on the input — so the reader saw "check your inbox" and nothing
// was ever sent. A server-side test of an endpoint says nothing about whether anything
// calls it.
func TestTheLandingFormCanActuallyReachTheGate(t *testing.T) {
	landing, err := Page("landing")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(landing), "form class=\"gatecard\"") {
		t.Fatal("the landing page has no gate form; this test is looking at the wrong page")
	}

	js, err := readStatic("landing.js")
	if err != nil {
		t.Fatalf("the landing page's script is not embedded: %v", err)
	}
	for _, want := range []string{"/gate", "input.name = 'email'", "POST"} {
		if !strings.Contains(js, want) {
			t.Errorf("landing.js does not mention %q. The handler reads a form field named "+
				"email and answers a POST; a script missing any of that submits nowhere.", want)
		}
	}
}

func readStatic(name string) (string, error) {
	b, err := Static().Open(name)
	if err != nil {
		return "", err
	}
	defer b.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := b.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String(), nil
}
