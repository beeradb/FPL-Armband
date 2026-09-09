package webui

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// gateFormOpen matches a <form ...> opening tag carrying a data-gate attribute --
// landing.html's two static mounts and app.js's two dynamically-rendered ones (the News
// panel's ask and the Pitch tab's nudge). firstInputAfter matches the next <input ...>
// tag, which is always the email field on every one of these forms.
var (
	gateFormOpen    = regexp.MustCompile(`(?is)<form\b[^>]*\sdata-gate="([^"]*)"[^>]*>`)
	firstInputAfter = regexp.MustCompile(`(?is)<input\b[^>]*>`)
)

// TestEveryGateFormHasANativeFallback pins the fix for a bug a QA pass found: every one of
// the four gate forms submits by fetch (see gate.js), intercepting `submit` and calling
// preventDefault() before the browser's own default action ever runs. That default is what
// fires when the script FAILS to run at all -- blocked by an ad blocker, a network error
// before it loads, a future CSP mistake -- and an unconfigured <form> defaults to method
// GET against the current URL, which discards whatever the reader typed rather than
// reporting a failure. Two things are needed for the fallback to actually work, and
// neither may depend on gate.js having run:
//
//  1. action + method="post", matching data-gate exactly, so the native submission goes to
//     the same place the fetch would have. See cmd/armband/webroutes.go's formActionFor for
//     the matching Content-Security-Policy exception landing.html's absolute destination
//     needs (app.js's forms carry a relative /gate, always same-origin, and need none).
//  2. A STATIC name="email" on the input. gate.js's wireOne sets this at runtime
//     (`input.name = 'email'`) for the forms app.js renders by innerHTML, which is fine
//     for the fetch path but useless for the fallback: a form field with no name attribute
//     submits nothing at all, by the HTML spec, regardless of which method carries it.
//
// autocomplete="email" isn't part of that failure mode -- nothing breaks without it -- but
// it is the same one-line, no-behaviour-change improvement this fix travels with, so it is
// pinned alongside the two load-bearing checks rather than left to rot unwitnessed.
func TestEveryGateFormHasANativeFallback(t *testing.T) {
	check := func(name, body string) {
		forms := gateFormOpen.FindAllStringSubmatchIndex(body, -1)
		if len(forms) == 0 {
			t.Fatalf("%s: found no <form data-gate=...> -- the scan itself is broken, or "+
				"every gate form has been removed from this document", name)
		}
		for _, m := range forms {
			tag := body[m[0]:m[1]]
			dest := body[m[2]:m[3]]

			if !strings.Contains(tag, `action="`+dest+`"`) {
				t.Errorf("%s: <form data-gate=%q> carries no action=%q -- a script "+
					"failure falls back to a GET on the current page and silently "+
					"discards the typed address. Tag: %s", name, dest, dest, tag)
			}
			if !strings.Contains(tag, `method="post"`) {
				t.Errorf("%s: <form data-gate=%q> carries no method=\"post\" -- its "+
					"native fallback would GET instead of POST. Tag: %s", name, dest, tag)
			}

			rest := body[m[1]:]
			input := firstInputAfter.FindString(rest)
			if input == "" {
				t.Fatalf("%s: <form data-gate=%q> has no following <input>", name, dest)
			}
			if !strings.Contains(input, `name="email"`) {
				t.Errorf("%s: the gate form's <input> carries no static name=\"email\". "+
					"gate.js's wireOne sets this at runtime -- fine for the fetch path, "+
					"useless for the native fallback, since an unnamed field submits "+
					"nothing at all. Input: %s", name, input)
			}
			if !strings.Contains(input, `autocomplete="email"`) {
				t.Errorf("%s: the gate form's <input> carries no autocomplete=\"email\". "+
					"Input: %s", name, input)
			}
		}
	}

	landing, err := Page("landing")
	if err != nil {
		t.Fatalf("Page(\"landing\"): %v", err)
	}
	check("landing.html", string(landing))

	appjs, err := fs.ReadFile(Static(), "app.js")
	if err != nil {
		t.Fatalf("reading static/app.js: %v", err)
	}
	check("app.js", string(appjs))
}
