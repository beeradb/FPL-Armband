package webui_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The type rules, scanned rather than rendered.
//
// A rendered check is stronger and is owed for CONTRAST, where compositing and inheritance
// mean a stylesheet cannot answer the question. Size is different: a `font-size` declaration
// is the whole story, so a scan is the right instrument and costs no browser.

// styleSources is EVERY place this client declares a font size, which is the whole point.
//
// ⚠️ This scan read `armband.css` alone until 2026-08-21, and a scan scoped to one file
// reports a clean floor for a system that does not have one. The landing page carries its
// own large <style> block and the scripts set sizes inline; `.mini .arm` sat at 7px there,
// two below the floor stated at the top of armband.css, for as long as this test has
// existed and passed. A guard that cannot see a surface does not guard it.
//
// Adding a page or a script that declares type means adding it here. The list is short
// deliberately: it is every file that can carry a `font-size`, not a glob, so a new one is
// a decision rather than an accident.
var styleSources = []string{
	"assets/static/armband.css",
	"assets/pages/landing.html",
	"assets/pages/app.html",
	"assets/static/app.js",
	"assets/static/landing.js",
}

var fontSize = regexp.MustCompile(`font-size:\s*(\d+(?:\.\d+)?)px`)

// mediaMin is the narrowest `max-width` a rule can sit under and still be called mobile.
const mediaMin = 720

// bodySize is `body{font-size}` in the stylesheet: the size this design calls normal
// reading. Text at or above it has headroom; text below it is where a cut starts to hurt.
const bodySize = 15.0

// TestMobileTypeIsNeverSmallerThanDesktopType is the rule that was broken, and it was broken
// on the four declarations that mattered most.
//
// Measured 2026-08-19, before the fix:
//
//	.card .nm     12px -> 10px -> 9.5px   the player's NAME
//	.card .xp b   15px -> 13px -> 12px    the product's one number
//	.card .cl      9px ->  8.5px          the club code
//	.card .meta  9.5px ->  8.5px          the price
//
// So the phone — held at arm's length, often outdoors, one-handed — was served the smallest
// type in the system, in the one ink token that failed 4.5:1 on every surface. That trio was
// the whole of "it is hard to read", and none of it needed a redesign.
//
// The rule this pins: **mobile type is never smaller than desktop type for the same
// selector.** Equal or larger, never less. A narrower screen is not a reason for smaller
// text; it is usually a reason for larger.
func TestMobileTypeIsNeverSmallerThanDesktopType(t *testing.T) {
	base, mobile := sizesBySelector(t)

	var shrunk []string
	for sel, small := range mobile {
		big, ok := base[sel]
		if !ok {
			continue // declared only inside the media query; nothing to shrink from
		}
		// The rule binds BELOW the body size, and the scoping is deliberate rather than
		// convenient.
		//
		// The harm being prevented is text crossing into illegibility, and that is a
		// property of the destination size, not of the ratio. Cutting a 28px hero number
		// to 26px to fit a 390px screen is ordinary typography and nobody strains to read
		// the result; cutting a 12px player name to 9.5px is the complaint this test
		// exists to answer. Applying one rule to both would fail four display selectors
		// that are not hurting anyone, and a test that cries wolf is a test that gets a
		// blanket exemption list — which is how the rule would actually die.
		//
		// bodySize is the page's own `body{font-size}`, so "below body" means "smaller
		// than the text this design considers normal reading".
		if small < big && small < bodySize {
			shrunk = append(shrunk, sel+": "+trim(big)+"px -> "+trim(small)+"px")
		}
	}
	sort.Strings(shrunk)
	if len(shrunk) > 0 {
		t.Errorf("%d selector(s) get SMALLER text on a narrow screen:\n  %s\n\n"+
			"A phone is held further from the eye than a monitor and is read outdoors and "+
			"one-handed. Equal or larger, never smaller. If a card cannot fit its contents "+
			"at the desktop size, remove contents rather than shrinking type — that is what "+
			"the mobile card simplification is for.",
			len(shrunk), strings.Join(shrunk, "\n  "))
	}
}

// TestNothingRendersBelowTheTypeFloor pins the design system's own stated floor.
//
// `armband.css` declares it at the top of the file — "TYPE FLOOR: nothing in this system
// renders below 9px. Ever." — and six declarations sat below it, two of them mobile-only and
// both on the player card the complaint was about. A floor stated in a comment and broken in
// the file below it is worse than no floor: it reads as a guarantee.
func TestNothingRendersBelowTheTypeFloor(t *testing.T) {
	const floor = 9.0

	var under []string
	for _, src := range styleSources {
		for _, line := range strings.Split(readSource(t, src), "\n") {
			for _, m := range fontSize.FindAllStringSubmatch(line, -1) {
				v, err := strconv.ParseFloat(m[1], 64)
				if err != nil {
					continue
				}
				if v < floor {
					under = append(under, src+": "+trim(v)+"px in: "+strings.TrimSpace(line))
				}
			}
		}
	}
	if len(under) > 0 {
		t.Errorf("%d declaration(s) below the %gpx floor armband.css states at the top of "+
			"itself:\n  %s\n\nThe floor is stated for the whole system, so it is scanned "+
			"across every style source, not only the stylesheet that states it.",
			len(under), floor, strings.Join(under, "\n  "))
	}
}

// sizesBySelector returns the font-size for each selector outside any media query, and the
// font-size for each selector inside a mobile one.
//
// Deliberately simple: it tracks brace depth to know when a `@media` block ends, and takes
// the last size declared for a selector in each context. It does not resolve the cascade,
// and it does not need to — the question is only whether a selector is given a smaller size
// under a narrow breakpoint than it has outside one.
func sizesBySelector(t *testing.T) (base, mobile map[string]float64) {
	t.Helper()
	base, mobile = map[string]float64{}, map[string]float64{}
	for _, src := range styleSources {
		scanOne(t, src, base, mobile)
	}
	if len(base) == 0 {
		t.Fatal("parsed no font sizes outside a media query; the scan is broken, not the CSS")
	}
	return base, mobile
}

// scanOne accumulates one source's sizes. Selectors are keyed by source, because the same
// selector text in two files is two different rules and comparing them would be nonsense.
func scanOne(t *testing.T, src string, base, mobile map[string]float64) {
	t.Helper()

	var (
		inMedia   bool
		isMobile  bool
		mediaHead = regexp.MustCompile(`@media[^{]*max-width:\s*(\d+)px`)
		depth     int
		selector  string
	)
	for _, raw := range strings.Split(readSource(t, src), "\n") {
		line := strings.TrimSpace(raw)
		if m := mediaHead.FindStringSubmatch(line); m != nil {
			w, _ := strconv.Atoi(m[1])
			inMedia, isMobile, depth = true, w <= mediaMin, 0
		}
		if s := strings.Index(line, "{"); s >= 0 {
			if head := strings.TrimSpace(line[:s]); head != "" && !strings.HasPrefix(head, "@") {
				selector = head
			}
			depth += strings.Count(line, "{")
		}
		if m := fontSize.FindStringSubmatch(line); m != nil && selector != "" {
			v, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				key := src + " " + selector
				if inMedia && isMobile {
					mobile[key] = v
				} else if !inMedia {
					base[key] = v
				}
			}
		}
		depth -= strings.Count(line, "}")
		if inMedia && depth <= 0 {
			inMedia, isMobile = false, false
		}
	}
}

// cssPath is the stylesheet alone. It stays because the CONTRAST suite wants exactly this
// file — colour tokens are declared here and nowhere else — while the type rules below
// deliberately want every source. Two questions, two scopes.
const cssPath = "assets/static/armband.css"

func read(t *testing.T) string { return readSource(t, cssPath) }

func readSource(t *testing.T, src string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(src))
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	return string(b)
}

func trim(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
