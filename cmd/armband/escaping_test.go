package main

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/viewmodel"
)

// The escaping guarantee, tested through a real browser.
//
// # Why this is not a source scan
//
// The client renders by assigning innerHTML, and the strings it interpolates are player
// names, club names, fixture opponents and the reasoning attached to an override. None of
// those is written by this program. A grep for esc() proves the helper is called
// somewhere; only rendering the page proves that a name containing markup comes out as
// text.
//
// # Why it matters here more than it did before
//
// The Go template this application replaces escaped everything for free, and
// internal/present pins that with TestEveryNameIsEscapedInEveryView. Moving rendering to
// the client dropped the guarantee silently: nothing failed, no test noticed, and the page
// looked identical — because the mock data the design shipped with contained no markup.
//
// # What the first version of this test got wrong
//
// It is worth recording, because both mistakes make a security test pass while proving
// nothing, and both are easy to repeat.
//
//  1. It looked for the literal `<img src=x onerror` in the dumped DOM. --dump-dom
//     re-serialises what the parser BUILT, so a live element comes back with its
//     attributes quoted — `<img src="x" onerror="...">` — and the needle could never
//     match however broken the page was. The assertions below match the SHAPE of a live
//     element instead, which survives any quoting the serialiser chooses.
//  2. It renamed one arbitrary player. Which panels that exercises depends on where he
//     happens to sit: a player in the eleven never appears in the formation rail, which
//     lists the players a shape would bring IN. Three unescaped sinks survived that test,
//     one of them reachable on first paint. Every player is renamed now, so no render
//     path can be missed by luck.

// nastyName is what a hostile registration would look like. Three payloads in one string:
// a tag that fires with no user interaction, an attribute-breaking quote, and a closing
// tag that would end the element early if the value were pasted in raw.
const nastyName = `<img src=x onerror="window.__pwned=1">"><script>window.__pwned=2</script>`

// liveHandler and liveScript match the payload EXECUTING, not the payload appearing.
//
// This distinction took three attempts and is the whole difficulty of the test.
//
//  1. Matching the literal `<img src=x onerror` never fires: --dump-dom re-serialises
//     what the parser built, so a live element comes back with quoted attributes.
//
//  2. Matching ` onerror=` fires on correct output: esc() encodes the angle brackets and
//     the quotes but not the word between them, so escaped text still reads
//     `&lt;img src=x onerror=&quot;...`.
//
//  3. Matching `<tag ... onerror=` fires on correct output too, because that escaped
//     text sits inside a title="..." attribute and there is no unescaped `>` to stop
//     the wildcard reaching it.
//
//  4. Matching a real quote alone fires on TEXT. A text node needs no quote escaping, so
//     .textContent output serialises as `&lt;img src=x onerror="window.__pwned=1"&gt;` —
//     angle brackets encoded, quotes raw. That is perfectly safe and matched anyway.
//
// Three things together identify a live element and nothing else does: a real `<`, a span
// with no real `>` in it (which is what keeps the match inside one tag rather than running
// from an enclosing element into its text), and a real quote before the handler body.
// Escaping breaks the first or the third; a text node fails the second.
//
// The lesson generalises: to ask whether markup is LIVE, match something only execution
// could have produced, never the name of the thing you are afraid of.
var (
	liveHandler = regexp.MustCompile(`(?i)<[a-z][^>]*\son(error|load|click|mouseover)\s*=\s*"window\.__pwned`)
	liveScript  = regexp.MustCompile(`(?i)<script(?:\s[^>]*)?>[^<]*__pwned`)
)

// assertInert fails if the DOM contains anything the parser built from a payload.
func assertInert(t *testing.T, where, dom string) {
	t.Helper()
	if m := liveHandler.FindString(dom); m != "" {
		t.Errorf("%s: the DOM carries a live element with an inline event handler (%q). "+
			"The design uses none, so this was built from data — a name or a reason "+
			"reached innerHTML unescaped.", where, strings.TrimSpace(m))
	}
	if m := liveScript.FindString(dom); m != "" {
		t.Errorf("%s: the DOM carries an injected script element (%q)", where, m)
	}
	// The payload must still be PRESENT, as text. If it vanished the test would pass for
	// the wrong reason, having proved only that the data never reached the page.
	if !strings.Contains(dom, "&lt;img") {
		t.Errorf("%s: the hostile string does not appear escaped anywhere. Either nothing "+
			"rendered, or the escaping dropped the value rather than neutralising it — "+
			"and a test that asserts only the absence of an attack passes on a blank page.",
			where)
	}
}

// TestEveryPanelRendersAHostileNameAsText renames every player in the capture and walks
// all four panels in a real browser.
func TestEveryPanelRendersAHostileNameAsText(t *testing.T) {
	browser := browsertest.Find(t)

	s := fixtureServerNamed(t, func(string) string { return nastyName })
	srv := httptest.NewServer(s)
	defer srv.Close()

	// The API must carry the name FAITHFULLY. Escaping belongs at the point of rendering,
	// not in the data: an API that pre-escaped would be lying about what FPL published,
	// and would double-escape as soon as a second client did its own job.
	//
	// Asserted on the DECODED value, not the wire bytes. Go's encoding/json escapes "<"
	// to "<" by default, so the raw body never contains the literal even when the
	// name is carried perfectly — an assertion on the bytes fails against correct
	// behaviour, which is how a guard gets deleted instead of fixed.
	w := get(t, s, "/api/state")
	if w.Code != 200 {
		t.Fatalf("GET /api/state answered %d", w.Code)
	}
	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the state did not decode: %v", err)
	}
	if len(st.Squad.Players) == 0 || st.Squad.Players[0].Name != nastyName {
		t.Fatal("the API did not carry the player's real name, so nothing below is being " +
			"tested. Escaping belongs at the render, not in the data.")
	}

	// One dump covers every panel.
	//
	// The four panels are four <section>s in a single document and the tab switch only
	// toggles `hidden` -- renderAll draws all of them on every load. Dumping once per
	// panel launched four browsers to read the same bytes, which is three browsers of
	// nothing on a machine already running the rest of the suite in parallel.
	dom := browsertest.DumpDOM(t, browser, srv.URL+"/app#pitch")

	// The panels' own markers, so a document that somehow rendered only one of them
	// fails here rather than passing on a partial page. The formations rail was the
	// sink that survived the first version of this test; it is deleted now (renderShapes
	// is gone, along with the client-side model computation it did), replaced by Your
	// instructions, which is the new pitch-side marker below.
	for _, marker := range []string{
		// Matched as it appears in the DOM, not as it appears on screen -- the
		// stylesheet uppercases this heading, and the first version of this list
		// asserted the screenshot's text against the document's.
		"Your instructions",
		"view-players",
		"view-news",
		// view-brief was here. The Brief tab is deleted -- it held a verdict that was
		// never wired through and said so, plus the model's blind spots. Removed from
		// this list rather than the list being shortened silently, because the point of
		// the list is that it enumerates every panel the walk must reach.
	} {
		if !strings.Contains(dom, marker) {
			t.Errorf("the rendered document does not contain %q, so this test is not "+
				"covering every panel", marker)
		}
	}
	assertInert(t, "the application", dom)

	// The replacement picker, which the walk above cannot reach.
	//
	// It renders only after a tap, so a dump of the four panels never contains it — and it
	// is the newest surface AND the only place in the whole client where FPL's `news` prose
	// reaches the DOM. The single new sink for untrusted external text had no browser-level
	// escaping test until this line. The deep link exists partly for that.
	var target int
	for _, p := range st.Squad.Players {
		if p.Pos == "MID" {
			target = p.ID
			break
		}
	}
	if target == 0 {
		t.Fatal("no midfielder in the fixture, so the picker cannot be opened")
	}
	picker := browsertest.DumpDOM(t, browser, srv.URL+"/app#replace-"+strconv.Itoa(target))
	if !strings.Contains(picker, "to spend") {
		t.Fatalf("the picker did not open on #replace-%d, so nothing below is tested", target)
	}
	assertInert(t, "the replacement picker", picker)
}

// TestOverrideProseRendersAsText covers the other free-text channel.
//
// Override reasoning is the one string on the page a HUMAN types, and stage two makes it
// writable from the browser. What makes it safe today is not sanitiseReason — that strips
// control characters and caps the length, and does nothing whatever about markup.
// Rendering is the only thing between a typed reason and the DOM.
func TestOverrideProseRendersAsText(t *testing.T) {
	browser := browsertest.Find(t)

	s := fixtureServer(t)
	cfg := *s.cfg
	cfg.Roster.Minutes[0].Reason = "He is nailed " + nastyName
	s.cfg = &cfg

	srv := httptest.NewServer(s)
	defer srv.Close()

	dom := browsertest.DumpDOM(t, browser, srv.URL+"/app#news")
	if !strings.Contains(dom, "He is nailed") {
		t.Fatal("the override's reasoning is not on the page, so this proves nothing")
	}
	assertInert(t, "override reasoning", dom)
}
