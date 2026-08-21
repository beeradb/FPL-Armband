package webui_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"armband/internal/webui"
)

// The phone's way out of a sheet is a LABELLED BAR, and the sheet fills the screen.
//
// The complaint this answers was "the sheets have close buttons nobody can reach", and the
// first reading of it was wrong in a way worth keeping: the ✕ IS reachable, and the evidence
// that it was not came from a layout golden shot at a height no phone has, so the harness
// reported the absence of a state as a defect. What is actually wrong is different. A sheet
// clamped to 88vh leaves a strip of the pitch showing above it; that strip reads as tappable,
// the scrim swallows the tap, and a corner glyph never says where dismissing goes.
//
// So below 720px the sheet fills the viewport and a bar across the top says, in words, what
// closing does. This test pins that arrangement rather than a picture of it — the layout
// goldens are a local check by decision (see visual_test.go), so anything that must not
// regress silently is asserted over the markup, where it runs everywhere.
func TestThePhoneSheetHasALabelledWayBack(t *testing.T) {
	page, err := webui.Page("app")
	if err != nil {
		t.Fatalf("Page(\"app\"): %v", err)
	}
	app := string(page)

	back := regexp.MustCompile(`(?s)<button[^>]*id="sheetback"[^>]*>(.*?)</button>`).FindStringSubmatch(app)
	if back == nil {
		t.Fatal(`the app page carries no #sheetback control. The phone's dismissal is a ` +
			`labelled bar; without it the only way out of a full-screen sheet is a corner glyph.`)
	}

	// A label, not only an arrow. The glyph is decorative and marked aria-hidden; the words
	// are the control. Strip tags, then require real letters.
	text := strings.TrimSpace(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(back[1], " "))
	if !regexp.MustCompile(`[A-Za-z]{3,}`).MatchString(text) {
		t.Errorf("#sheetback reads %q — a glyph with no words. The whole point of the bar over "+
			"the ✕ is that it says where back goes.", text)
	}
	if !strings.Contains(app, `id="sheetbacklabel"`) {
		t.Error("#sheetbacklabel is gone, so the bar can no longer name the view it returns " +
			"to; setView writes that label.")
	}

	css := read(t)

	// Above 720px the bar must not show: there the sheet is a centred dialog with room
	// around it and the ✕ is an easy target. Two dismissals on one screen is the clutter
	// the primary-page work spent itself removing.
	if !regexp.MustCompile(`\.sheetback\s*\{[^}]*display\s*:\s*none`).MatchString(baseOf(css)) {
		t.Error("`.sheetback` is not display:none outside a media query, so the desktop dialog " +
			"grows a second close control")
	}

	phone := mobileBlocksOf(css)
	if !regexp.MustCompile(`\.sheetback\s*\{[^}]*display\s*:\s*flex`).MatchString(phone) {
		t.Error("`.sheetback` is never shown under a mobile breakpoint, so the phone has the " +
			"full-screen sheet and no labelled way out of it")
	}
	// Full-screen: the 88vh clamp is what left the tappable-looking strip.
	if regexp.MustCompile(`\.sheet\s*\{[^}]*max-height\s*:\s*\d+vh`).MatchString(phone) {
		t.Error("`.sheet` is clamped to a vh height under a mobile breakpoint. That leaves a " +
			"strip of the page above it which reads as tappable and is not — the route is " +
			"full-screen deliberately.")
	}
}

// mobileBlocksOf returns every `@media (max-width: N)` block with N <= 720, concatenated.
func mobileBlocksOf(css string) string {
	head := regexp.MustCompile(`@media[^{]*max-width:\s*(\d+)px[^{]*\{`)
	var out strings.Builder
	for _, loc := range head.FindAllStringSubmatchIndex(css, -1) {
		width, err := strconv.Atoi(css[loc[2]:loc[3]])
		if err != nil || width > mediaMin {
			continue // wider than a phone; not the block this rule belongs in
		}
		depth, i := 1, loc[1]
		for ; i < len(css) && depth > 0; i++ {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		out.WriteString(css[loc[1]:i])
	}
	return out.String()
}

// baseOf returns the stylesheet with every media block removed.
func baseOf(css string) string {
	head := regexp.MustCompile(`@media[^{]*\{`)
	for {
		loc := head.FindStringIndex(css)
		if loc == nil {
			return css
		}
		depth, i := 1, loc[1]
		for ; i < len(css) && depth > 0; i++ {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		css = css[:loc[0]] + css[i:]
	}
}
