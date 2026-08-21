package webui

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The landing page's hero preview draws a squad on a pitch and states three numbers about
// it: the formation, the sum of the eleven, and the sum again with the captain doubled.
// Every one of them is hand-written markup, and each is derived from the other two — so a
// card that is edited without arithmetic is a card that lies, on the page selling the
// product's arithmetic.
//
// ⚠️ THIS HAS SHIPPED TWICE. The card claimed 3-5-2 and drew nine players; the prices
// summed to 38.79 under a pill claiming 46.5. That was fixed in 42e0b99, REGRESSED in
// 2b911a6 when the landing page was rebuilt to lead on team news, and was public across
// four pins before a reader noticed the pitch had nine players. It was fixed again in
// e3a44fc, which added a comment stating the invariant — and a comment is not an
// assertion. This test is the assertion.
//
// The layout goldens do not cover it and cannot be made to. They are pictures: `-update`
// rewrites the shot inside the same commit that breaks the page, which is exactly how the
// regression was banked, and TestLayout skips itself on CI runners anyway. This test reads
// the shipped markup, needs no browser, and runs everywhere.
func TestTheHeroPeekAddsUp(t *testing.T) {
	body, err := Page("landing")
	if err != nil {
		t.Fatalf("Page(\"landing\"): %v", err)
	}
	// scannable strips HTML comments, so the comment above the block — which states these
	// same numbers in prose — cannot be mistaken for the markup.
	src := scannable(body)

	bar := only(t, src, `<div class="bar">`, "the preview's title bar")
	turf := between(t, src, `<div class="turf">`, `<div class="foot">`, "the pitch")
	foot := only(t, src, `<div class="foot">`, "the preview's footer")

	// ---- what the card SAYS -------------------------------------------------------
	def, mid, fwd := formation(t, bar)
	claimed := def + mid + fwd + 1 // the keeper is never in the formation string
	withCaptain := money(t, barTotal, bar, "the total in the title bar")
	eleven := money(t, footTotal, foot, "the total in the footer pill")

	// ---- what the card DRAWS ------------------------------------------------------
	rows := drawnPerLine(turf)
	if len(rows) != 4 {
		t.Fatalf("the pitch draws %d lines, want 4 (keeper, defence, midfield, attack). "+
			"The lines ARE the formation: a fifth line has no meaning in %d-%d-%d.",
			len(rows), def, mid, fwd)
	}
	want := [4]int{1, def, mid, fwd}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("line %d draws %d players, but the bar claims %d-%d-%d, which wants %d. "+
				"Lines are keeper/defence/midfield/attack in order.",
				i+1, rows[i], def, mid, fwd, w)
		}
	}

	drawn := 0
	for _, n := range rows {
		drawn += n
	}
	if drawn != claimed {
		t.Errorf("the pitch draws %d players and the bar claims %d-%d-%d, which is %d. "+
			"This is the defect that shipped twice: an eleven that is not eleven, under an "+
			"H1 promising eleven.", drawn, def, mid, fwd, claimed)
	}

	// ---- and whether the two agree ------------------------------------------------
	prices := allPrices(turf)
	if len(prices) != drawn {
		t.Fatalf("%d cards drawn but %d prices found; every card carries exactly one .p",
			drawn, len(prices))
	}
	sum := 0
	for _, p := range prices {
		sum += p
	}
	if sum != eleven {
		t.Errorf("the drawn prices sum to %s, but the footer pill claims %s. The pill is a "+
			"total OF these cards; a card added, removed or repriced moves it.",
			pounds(sum), pounds(eleven))
	}

	caps := captainPrices(turf)
	if len(caps) != 1 {
		t.Fatalf("%d cards carry .mini.cap, want exactly 1 — an eleven has one captain", len(caps))
	}
	if got := sum + caps[0]; got != withCaptain {
		t.Errorf("the eleven sum to %s and the captain's %s counts again, which is %s, but "+
			"the title bar claims %s. The bar is the score WITH the armband.",
			pounds(sum), pounds(caps[0]), pounds(got), pounds(withCaptain))
	}
}

var (
	formationRe = regexp.MustCompile(`(\d)-(\d)-(\d)`)
	barTotal    = regexp.MustCompile(`([\d.]+)\s+points`)
	footTotal   = regexp.MustCompile(`▲\s*([\d.]+)`)
	minilineRe  = regexp.MustCompile(`<div class="miniline">`)
	miniRe      = regexp.MustCompile(`class="mini[ "]`)
	priceRe     = regexp.MustCompile(`class="p">([\d.]+)<`)
	capPriceRe  = regexp.MustCompile(`(?s)class="mini cap".*?class="p">([\d.]+)<`)
)

// formation reads the N-N-N the bar claims. Fatal rather than skipped: a bar that no longer
// states a formation has not made this test irrelevant, it has removed the thing the pitch
// is checked against, and that decision should be visible here.
func formation(t *testing.T, bar string) (def, mid, fwd int) {
	t.Helper()
	m := formationRe.FindStringSubmatch(bar)
	if m == nil {
		t.Fatalf("the title bar states no N-N-N formation: %q", strings.TrimSpace(bar))
	}
	return atoi(t, m[1]), atoi(t, m[2]), atoi(t, m[3])
}

// money parses one figure to whole pence, so the comparisons are integer equality. The
// threshold here is EXACT: these are hand-written constants that must agree by
// construction, not measurements with a tolerance.
func money(t *testing.T, re *regexp.Regexp, in, what string) int {
	t.Helper()
	m := re.FindStringSubmatch(in)
	if m == nil {
		t.Fatalf("no figure found in %s: %q", what, strings.TrimSpace(in))
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("%s reads %q, which is not a number: %v", what, m[1], err)
	}
	return int(math.Round(f * 100))
}

func pounds(pence int) string {
	return strconv.FormatFloat(float64(pence)/100, 'f', 2, 64)
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("%q is not a number: %v", s, err)
	}
	return n
}

// drawnPerLine counts the cards on each line of the pitch, in order.
func drawnPerLine(turf string) []int {
	starts := minilineRe.FindAllStringIndex(turf, -1)
	rows := make([]int, 0, len(starts))
	for i, s := range starts {
		end := len(turf)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		rows = append(rows, len(miniRe.FindAllString(turf[s[0]:end], -1)))
	}
	return rows
}

func allPrices(turf string) []int {
	ms := priceRe.FindAllStringSubmatch(turf, -1)
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		f, _ := strconv.ParseFloat(m[1], 64)
		out = append(out, int(math.Round(f*100)))
	}
	return out
}

func captainPrices(turf string) []int {
	ms := capPriceRe.FindAllStringSubmatch(turf, -1)
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		f, _ := strconv.ParseFloat(m[1], 64)
		out = append(out, int(math.Round(f*100)))
	}
	return out
}

// only returns the run of markup starting at one marker that must appear exactly once. A
// second pitch on the page would otherwise be checked against the first one's numbers.
func only(t *testing.T, src, marker, what string) string {
	t.Helper()
	if n := strings.Count(src, marker); n != 1 {
		t.Fatalf("%s appears %d times, want exactly 1 (%q)", what, n, marker)
	}
	i := strings.Index(src, marker)
	end := i + 400
	if end > len(src) {
		end = len(src)
	}
	return src[i:end]
}

func between(t *testing.T, src, open, close, what string) string {
	t.Helper()
	i := strings.Index(src, open)
	j := strings.Index(src, close)
	if i < 0 || j < 0 || j < i {
		t.Fatalf("could not find %s between %q and %q", what, open, close)
	}
	return src[i+len(open) : j]
}
