package webui_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"armband/internal/browsertest"
)

// The contrast suite: WCAG 1.4.3 over the pages as a browser actually paints them.
//
// This is the check `typescale_test.go` says is owed. Its header comment names the reason a
// stylesheet scan cannot do this job — "compositing and inheritance mean a stylesheet cannot
// answer the question" — and there is a second reason that is the one this project was
// actually caught by: BOTH existing guards read `assets/static/armband.css` and nothing else.
// `landing.html` carries its own <style> block and `app.js` sets styles inline, so neither is
// visible to any scan in this repository. A rendered check covers them by construction rather
// than by somebody remembering to add a second path.
//
// The defect it exists to catch is not hypothetical. `--ink3` shipped as #64788A and failed
// 4.5:1 on every surface in the system — 3.95 on --panel, 4.32 on --bg, 3.20 on --panel3 —
// across the ~53 declarations that colour every price, date, club code and column head. It was
// specified correctly in the design handoff, built wrong on day one, and survived for months
// because nothing could see it.
//
// # What this enforces, and what it deliberately does not
//
// Enforced: 4.5:1 for text, 3:1 for large text (>=24px, or >=18.66px at weight >=700). The
// large-text exemption is not a convenience — without it the check fails the 62px hero and is
// simply wrong about the standard.
//
// NOT enforced: 3:1 on borders, hairlines and decorative boundaries. That is a recorded
// decision rather than an omission. WCAG 1.4.11 exempts a decorative boundary, and the
// readability work declined to lift every hairline because doing so makes the page louder,
// which is the opposite of what was asked for; `--line` sits at about 1.33 against `--panel`
// on purpose. `.btn.ghost` — transparent, with a border at 1.57:1 — is a real usability
// problem recorded elsewhere, and it is not a contrast-test failure. This test must not
// conflate the two.
//
// The scoping matters more than the coverage. A test that cries wolf earns a blanket
// exemption list, and that is how the rule would actually die — so every exemption here is
// narrow, written at the point it is taken, and reported in the output rather than being
// silent. The probe's own decisions, including the five hard cases, are documented at the top
// of testdata/contrast_probe.js.

// The two rules, from WCAG 2.1 success criterion 1.4.3.
const (
	normalText = 4.5
	largeText  = 3.0

	// Large text is 18pt, or 14pt bold, expressed in the CSS px this design is written in.
	largePx     = 24.0
	largeBoldPx = 18.66
	boldWeight  = 700
)

// A page that renders almost nothing still publishes a valid, empty answer, and an empty
// answer passes every assertion below it. This is the floor that turns that into a failure.
// It is well under the smallest observed screen so it can only fire on a page that did not
// render at all.
const minRunsPerScreen = 15

type contrastRun struct {
	At     string  `json:"at"`
	Text   string  `json:"text"`
	FG     string  `json:"fg"`
	BG     string  `json:"bg"`
	Size   float64 `json:"size"`
	Weight int     `json:"weight"`
	Ratio  float64 `json:"ratio"`
}

type contrastResult struct {
	URL        string        `json:"url"`
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	Runs       []contrastRun `json:"runs"`
	Exempt     []exemptRun   `json:"exempt"`
	Unmeasured []string      `json:"unmeasured"`
	Error      string        `json:"error"`
}

type exemptRun struct {
	Reason string `json:"reason"`
	At     string `json:"at"`
	Text   string `json:"text"`
}

// required is the ratio this run has to clear.
func (r contrastRun) required() float64 {
	if r.Size >= largePx || (r.Size >= largeBoldPx && r.Weight >= boldWeight) {
		return largeText
	}
	return normalText
}

// The screens. Both pages at a desktop width and at a true phone, plus the app's four views
// and the replacement sheet — each is a different set of text, and a view that is
// `display:none` contributes nothing to the view beside it.
//
// The phone is 390x844 and the height is load-bearing for the same reason the layout suite's
// phone cases are: everything sized in `vh`, or positioned `fixed` or `sticky`, behaves
// differently at a height a device actually has, and every earlier shot in this package was
// taken at a height no device has. That blindness had already produced one wrong finding.
var contrastScreens = []struct {
	name    string
	fixture string
	path    string
}{
	{"landing", "gameweek-one", "/?probe=1"},
	{"armband-team", "gameweek-one", "/armband-team?probe=1"},
	{"pitch", "gameweek-one", "/app?probe=1#pitch"},
	{"players", "gameweek-one", "/app?probe=1#players"},
	{"news", "gameweek-one", "/app?probe=1#news"},
	{"brief", "gameweek-one", "/app?probe=1#brief"},
	{"picker", "gameweek-one", "/app?probe=1#replace-542"},

	// The states live data will not hand you: an empty pool, a flagged player, a blank
	// gameweek. They carry warning and error inks that no other screen renders.
	{"edges-pitch", "edges", "/app?probe=1#pitch"},
	{"edges-players", "edges", "/app?probe=1#players"},
}

func TestRenderedTextClearsTheContrastFloor(t *testing.T) {
	browser := browsertest.Find(t)

	servers := map[string]*httptest.Server{}
	for _, s := range contrastScreens {
		if servers[s.fixture] == nil {
			servers[s.fixture] = serve(t, s.fixture)
		}
	}

	views := []struct {
		name string
		view browsertest.Viewport
	}{
		{"desktop", desktop(900)},
		{"phone", phone()},
	}

	for _, s := range contrastScreens {
		for _, v := range views {
			t.Run(s.name+"-"+v.name, func(t *testing.T) {
				res := probeContrast(t, browser, servers[s.fixture].URL, s.path, v.view, minRunsPerScreen)
				if res.Width != v.view.Width || res.Height != v.view.Height {
					t.Fatalf("the page laid out at %dx%d, not the %dx%d asked for. A phone "+
						"check at the wrong width is not a phone check, and nothing else here "+
						"would say so.", res.Width, res.Height, v.view.Width, v.view.Height)
				}
				reportContrast(t, res)
			})
		}
	}
}

// probeContrast renders path THROUGH THE FRAME, which is the only way to get the viewport
// that was asked for; see browsertest.FrameHTML.
func probeContrast(t *testing.T, browser, origin, path string, v browsertest.Viewport,
	minRuns int) contrastResult {
	t.Helper()
	frame := origin + "/probe-frame?src=" + url.QueryEscape(origin+path) +
		"&w=" + strconv.Itoa(v.Width) + "&h=" + strconv.Itoa(v.Height)

	var res contrastResult
	raw := browsertest.Probe(t, browser, frame, browsertest.OuterFor(v))
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("the probe published something that is not its own payload: %v\n%s",
			err, truncate(string(raw), 400))
	}
	if res.Error != "" {
		t.Fatalf("the probe threw inside the page, so nothing was measured:\n%s", res.Error)
	}
	if len(res.Runs) < minRuns {
		t.Fatalf("only %d run(s) of text on %s at %dx%d, wanted at least %d. A screen this "+
			"empty means the page did not render, and an empty answer passes every check "+
			"below it.", len(res.Runs), origin+path, v.Width, v.Height, minRuns)
	}
	return res
}

// reportContrast fails on the runs below their floor, and LOGS what was set aside.
//
// Logging the exemptions is the part that keeps the rule alive. An exemption nobody can see
// is an exemption nobody re-examines, and the failure mode this test is most exposed to is
// not a missed violation — it is a growing quiet list.
func reportContrast(t *testing.T, res contrastResult) {
	t.Helper()

	type group struct {
		run   contrastRun
		count int
	}
	worst := map[string]*group{}
	for _, r := range res.Runs {
		if r.Ratio >= r.required() {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%g|%d", r.At, r.FG, r.BG, r.Size, r.Weight)
		if g, ok := worst[key]; ok {
			g.count++
			continue
		}
		worst[key] = &group{run: r, count: 1}
	}

	reasons := map[string]int{}
	for _, e := range res.Exempt {
		reasons[e.Reason]++
	}
	var summary []string
	for r, n := range reasons {
		summary = append(summary, fmt.Sprintf("%d × %s", n, r))
	}
	sort.Strings(summary)
	t.Logf("%dx%d, %d run(s) measured; set aside: %s",
		res.Width, res.Height, len(res.Runs), strings.Join(summary, ", "))

	// The tightest pairing on the screen, logged whether or not it passes. A pass says only
	// that nothing is below the floor; this says how much of the floor is left, which is what
	// tells a reader whether the next token change has room in it.
	tightest := res.Runs[0]
	for _, r := range res.Runs {
		if r.Ratio/r.required() < tightest.Ratio/tightest.required() {
			tightest = r
		}
	}
	t.Logf("tightest: %.2f:1 against %.1f — %s on %s at %gpx/%d — %s",
		tightest.Ratio, tightest.required(), tightest.FG, tightest.BG,
		tightest.Size, tightest.Weight, tightest.At)
	for _, u := range res.Unmeasured {
		t.Logf("NOT measured — %s", u)
	}

	if len(worst) == 0 {
		return
	}
	groups := make([]*group, 0, len(worst))
	for _, g := range worst {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].run.Ratio != groups[j].run.Ratio {
			return groups[i].run.Ratio < groups[j].run.Ratio
		}
		return groups[i].run.At < groups[j].run.At
	})

	var b strings.Builder
	fmt.Fprintf(&b, "%d text pairing(s) below the WCAG AA floor at %dx%d:\n",
		len(groups), res.Width, res.Height)
	for _, g := range groups {
		fmt.Fprintf(&b, "  %.2f:1 (needs %.1f) — %s on %s at %gpx/%d, ×%d\n      %s\n      %q\n",
			g.run.Ratio, g.run.required(), g.run.FG, g.run.BG, g.run.Size, g.run.Weight,
			g.count, g.run.At, g.run.Text)
	}
	b.WriteString("\nFix the token or the rule. An exemption here needs a written reason at " +
		"the point it is taken, in testdata/contrast_probe.js — never a bare list.")
	t.Error(b.String())
}

// probed adds the contrast probe to a page. Used by serve when a request asks for it.
//
// It returns an error rather than calling t.Fatalf because its caller is an HTTP handler
// goroutine, where a Fatalf hangs the request instead of failing the test.
func probed(page []byte) ([]byte, error) {
	return insertBeforeBody(page, `<script src="/probe.js"></script>`)
}

func insertBeforeBody(page []byte, s string) ([]byte, error) {
	const marker = "</body>"
	i := bytes.LastIndex(page, []byte(marker))
	if i < 0 {
		return nil, fmt.Errorf("the page has no %s to insert before", marker)
	}
	out := make([]byte, 0, len(page)+len(s))
	out = append(out, page[:i]...)
	out = append(out, []byte(s)...)
	return append(out, page[i:]...), nil
}

// brokenTokens are the values this design is known to have shipped and to have been wrong.
// Used only by the liveness check below; see withBrokenToken.
var brokenTokens = map[string]string{
	// What --ink3 actually shipped as, and what nothing in this repository could see.
	"ink3": "#64788A",
}

// brokenToken puts a known-bad token value back, for the one test that has to fail.
//
// The query parameter is a KEY into brokenTokens, never a value, so a request cannot inject
// arbitrary CSS even into this test-only page.
func brokenToken(page []byte, token string) ([]byte, error) {
	value, ok := brokenTokens[token]
	if !ok {
		return nil, fmt.Errorf("no recorded broken value for --%s", token)
	}
	return insertBeforeBody(page, "<style>:root{--"+token+":"+value+"}</style>")
}

// The stacks in testdata/contrast_stacks.html, and the colours a browser really paints for
// them. Every value is arithmetic over the fixture's own declarations: #ffffff at .6 over
// #000000 is 153 on each channel, so these are literals rather than roundings.
//
// ⚠️ Read the pairs, not the ids. Each case is chosen so that a WRONG resolution lands at the
// other end of the scale rather than slightly off — a canvas-black answer where the true one
// is panel-white — because a check whose failure is a small number is a check somebody widens
// the tolerance on.
var knownStacks = []struct {
	id, fg, bg, what string
}{
	{"s-opaque", "#808080", "#ffffff",
		"an opaque panel over the page: the PANEL paints, not the canvas"},
	{"s-inherited", "#808080", "#ffffff",
		"the painting ancestor is the grandparent; the parent paints nothing"},
	{"s-alpha", "#000000", "#999999",
		"a translucent layer composited against what is actually beneath it"},
	{"s-group", "#ffffff", "#666666",
		"an opacity GROUP: the text is faded once with the group, not again against " +
			"the already-faded background"},
	{"s-gradient", "#808080", "#ffffff",
		"a gradient, at its worst stop rather than its declared background-color"},
	{"s-pseudo", "#808080", "#ffffff",
		"a pseudo-element's OWN background, not the host's"},
}

// TestTheProbeResolvesTheBackgroundThatIsActuallyPainted is the arm with power over the
// instrument rather than over the design.
//
// It exists because the first version of this suite did not work, and everything else in the
// file passed anyway. The fold was written bottom-up and consumed top-down, so the first
// opaque layer it met was the page canvas and it stopped there: every run on every screen was
// scored against `--bg`, the darkest surface in the system, and the inks are light — so the
// error ran in the flattering direction and sixteen screens came back clean. `collectBackdrops`,
// the gradient enumeration and the whole of hard case 3 were dead code downstream of it. Two
// reviewers found it independently; nothing in the suite did.
//
// The liveness arm below could not have found it either, and that is the sharper lesson: it
// re-injects `--ink3` at #64788A, which fails against `--bg` as well as against everything
// else, so it caught its 191 runs whether or not the resolution worked at all. A canary is
// judged against the failure it is supposed to gate, and that one gated the tokens while the
// thing that broke was the stack.
//
// So this arm pins the STACK, on documents whose answer is arithmetic: it fails if a
// background is resolved to the wrong layer, in either direction, and it cannot be satisfied
// by a page that happens to be dark.
func TestTheProbeResolvesTheBackgroundThatIsActuallyPainted(t *testing.T) {
	browser := browsertest.Find(t)
	srv := serve(t, "gameweek-one")

	res := probeContrast(t, browser, srv.URL, "/stacks", desktop(900), len(knownStacks))

	byID := map[string]contrastRun{}
	for _, r := range res.Runs {
		for _, k := range knownStacks {
			if strings.Contains(r.At, "#"+k.id) {
				byID[k.id] = r
			}
		}
	}
	for _, k := range knownStacks {
		got, ok := byID[k.id]
		if !ok {
			t.Errorf("#%s was never measured — %s.\nA case that does not run is not a case "+
				"that passed.", k.id, k.what)
			continue
		}
		want := contrastBetween(k.fg, k.bg)
		if !strings.EqualFold(got.FG, k.fg) || !strings.EqualFold(got.BG, k.bg) {
			t.Errorf("#%s resolved to %s on %s; it is painted %s on %s.\n  %s",
				k.id, got.FG, got.BG, k.fg, k.bg, k.what)
			continue
		}
		if math.Abs(got.Ratio-want) > 0.02 {
			t.Errorf("#%s reports %.3f for %s on %s; that pair is %.3f",
				k.id, got.Ratio, got.FG, got.BG, want)
		}
	}
}

// TestTheContrastCheckSeesTheDefectItWasBuiltFor puts `--ink3` back to #64788A and requires
// the check to fail.
//
// A check that passes is not evidence that it works. This suite's whole risk is the shape
// this project keeps meeting: a comparison that never ran — a probe that measured nothing, a
// walk that resolved every background to the same safe colour, a filter that quietly excluded
// the interesting text — all of which look exactly like a clean page. Confinement is a code
// fact and re-running it can only fail; the check with power is the one that must MOVE.
//
// So the arm is the original defect. #64788A is 3.95 on --panel, 4.32 on --bg and 3.20 on
// --panel3, it colours the whole secondary information layer, and it shipped for months. If
// this test ever passes, the instrument is broken and everything above it is a silent pass.
func TestTheContrastCheckSeesTheDefectItWasBuiltFor(t *testing.T) {
	browser := browsertest.Find(t)
	srv := serve(t, "gameweek-one")

	res := probeContrast(t, browser, srv.URL, "/app?probe=1&regress=ink3#players", desktop(900), minRunsPerScreen)

	var caught []contrastRun
	for _, r := range res.Runs {
		if r.Ratio < r.required() {
			caught = append(caught, r)
		}
	}
	if len(caught) == 0 {
		t.Fatalf("--ink3 was put back to %s and the check found nothing. The 4.5:1 rule is "+
			"not being applied to anything a reader would notice, and every pass above this "+
			"line means nothing.", brokenTokens["ink3"])
	}
	for _, r := range caught {
		if !strings.EqualFold(r.FG, brokenTokens["ink3"]) {
			t.Errorf("the broken token was --ink3 (%s) and this failure is %s on %s at %s — "+
				"either the injection reached further than it should, or the page has a "+
				"second problem this suite is not otherwise reporting",
				brokenTokens["ink3"], r.FG, r.BG, r.At)
		}
	}
	t.Logf("caught %d run(s) at the shipped-and-wrong value, worst %.2f:1",
		len(caught), caught[0].Ratio)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---------------------------------------------------------------------------------------
// The token comments, which are a different question and take a different instrument.

var (
	hexToken   = regexp.MustCompile(`^\s*--([a-z0-9-]+):\s*(#[0-9A-Fa-f]{6})\s*;`)
	tokenLine  = regexp.MustCompile(`^\s*--([a-z0-9-]+):\s*(#[0-9A-Fa-f]{6})\s*;\s*/\*(.*)$`)
	tripleForm = regexp.MustCompile(`^\s*(\d+\.\d+)\s*/\s*(\d+\.\d+)\s*/\s*(\d+\.\d+)`)
	againstOne = regexp.MustCompile(`^\s*(\d+\.\d+)\s+vs\s+--([a-z0-9-]+)`)
)

// hexTokens is every `--name: #rrggbb;` the stylesheet declares.
func hexTokens(t *testing.T) map[string]string {
	t.Helper()
	tokens := map[string]string{}
	for _, line := range strings.Split(read(t), "\n") {
		if m := hexToken.FindStringSubmatch(line); m != nil {
			tokens[m[1]] = m[2]
		}
	}
	if len(tokens) == 0 {
		t.Fatal("parsed no hex tokens from the stylesheet; the scan is broken, not the CSS")
	}
	return tokens
}

// TestTheTokenCommentsStateTheRatioTheTokensHave recomputes the ratios the `:root` block
// claims for itself.
//
// A scan is the right instrument here and a browser would be the wrong one, which is the
// opposite of the test above: these comments are claims about two literal hex values in the
// same file, so there is nothing to composite and nothing to inherit. AGENTS.md names this
// class of comment as one not to strip, because it carries the data that justifies a
// constant — and a comment that carries data has to be right, or it is worse than no comment,
// since it reads as a guarantee.
//
// It found a wrong digit on its first run: `--line` was commented "1.31 vs --panel" and
// #223039 on #10171F is 1.3311.
func TestTheTokenCommentsStateTheRatioTheTokensHave(t *testing.T) {
	css := read(t)
	tokens := hexTokens(t)

	// The triple form is `/* 15.94 / 17.43 / 12.91 */`, and the `:root` comment above it
	// states the surfaces it is against.
	surfaces := []string{"panel", "bg", "panel3"}

	var wrong []string
	checked := 0
	for _, line := range strings.Split(css, "\n") {
		m := tokenLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, hex, comment := m[1], m[2], m[3]

		if g := tripleForm.FindStringSubmatch(comment); g != nil {
			for i, s := range surfaces {
				claimed, _ := strconv.ParseFloat(g[i+1], 64)
				got, err := contrastOf(tokens, hex, s)
				if err != nil {
					t.Fatalf("--%s: %v", name, err)
				}
				checked++
				if math.Abs(got-claimed) > 0.005 {
					wrong = append(wrong, fmt.Sprintf(
						"--%s claims %.2f against --%s; %s on %s is %.4f",
						name, claimed, s, hex, tokens[s], got))
				}
			}
			continue
		}
		if g := againstOne.FindStringSubmatch(comment); g != nil {
			claimed, _ := strconv.ParseFloat(g[1], 64)
			got, err := contrastOf(tokens, hex, g[2])
			if err != nil {
				t.Fatalf("--%s: %v", name, err)
			}
			checked++
			if math.Abs(got-claimed) > 0.005 {
				wrong = append(wrong, fmt.Sprintf(
					"--%s claims %.2f against --%s; %s on %s is %.4f",
					name, claimed, g[2], hex, tokens[g[2]], got))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no token comment states a ratio; either the comments were stripped — which " +
			"AGENTS.md says not to do — or this scan no longer matches how they are written")
	}
	if len(wrong) > 0 {
		t.Errorf("%d token comment(s) state a ratio the tokens do not have:\n  %s\n\n"+
			"These comments are the record of what justified a value. Recompute rather than "+
			"deleting.", len(wrong), strings.Join(wrong, "\n  "))
	}
}

func contrastOf(tokens map[string]string, hex, surface string) (float64, error) {
	bg, ok := tokens[surface]
	if !ok {
		return 0, fmt.Errorf("the comment names --%s, which is not a hex token", surface)
	}
	return contrastBetween(hex, bg), nil
}

// contrastBetween is WCAG 2.1's ratio: (L1 + 0.05) / (L2 + 0.05), L1 the lighter of the two.
// Named that way round rather than sorted in place, because a two-line swap of a pair reads
// to `TestTheCopiedExpressionsHaveOneImplementation` as a running top-two and it is not one.
func contrastBetween(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	lighter, darker := math.Max(la, lb), math.Min(la, lb)
	return (lighter + 0.05) / (darker + 0.05)
}

// relativeLuminance is WCAG 2.1's definition, and it is the ONE implementation on the Go
// side. The probe carries the same arithmetic in JavaScript because it has to run inside the
// page; the two are pinned against each other by
// TestTheTwoLuminanceImplementationsAgree.
func relativeLuminance(hex string) float64 {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		panic("not a hex colour: " + hex)
	}
	channel := func(c float64) float64 {
		c /= 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r := channel(float64((v >> 16) & 0xFF))
	g := channel(float64((v >> 8) & 0xFF))
	b := channel(float64(v & 0xFF))
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// TestTheTwoLuminanceImplementationsAgree pins the Go and JavaScript copies of the WCAG
// luminance formula to each other.
//
// One quantity with two implementations is this project's signature failure, and here the
// duplication is unavoidable — one copy has to run inside the page. So the copies are pinned
// instead: every run reports the pair of colours it ended up with as well as the ratio
// between them, and Go recomputes the ratio from that pair.
//
// ⚠️ It pins the FORMULA and says nothing about the RESOLUTION. Both sides use the pair the
// probe reports, so a probe that resolved every background to the same wrong colour would
// agree with itself here perfectly — which is exactly what happened, and what
// TestTheProbeResolvesTheBackgroundThatIsActuallyPainted exists to catch instead. Two guards,
// two questions; neither covers the other.
func TestTheTwoLuminanceImplementationsAgree(t *testing.T) {
	browser := browsertest.Find(t)
	srv := serve(t, "gameweek-one")

	res := probeContrast(t, browser, srv.URL, "/?probe=1", desktop(900), minRunsPerScreen)

	// The tolerance is RELATIVE, and it covers one thing: the probe composites in floating
	// point and reports its colours rounded to 8 bits per channel, so Go is recomputing from
	// a rounded pair. Half a unit on a channel moves a ratio of 11.7 by about 0.02, which is
	// 0.2% — while a formula differing in a coefficient or in the sRGB transfer function
	// would be out by whole percent. The largest deviation seen is logged every run, so the
	// headroom is visible rather than assumed.
	const tolerance = 0.01

	var worst float64
	var worstAt string
	for _, r := range res.Runs {
		want := contrastBetween(r.FG, r.BG)
		rel := math.Abs(r.Ratio-want) / want
		if rel > worst {
			worst, worstAt = rel, fmt.Sprintf("%.3f against %.3f for %s on %s — %s",
				r.Ratio, want, r.FG, r.BG, r.At)
		}
		if rel > tolerance {
			t.Errorf("the page computed %.3f for %s on %s; this package computes %.3f "+
				"(%.2f%% apart) — %s", r.Ratio, r.FG, r.BG, want, rel*100, r.At)
		}
	}
	if len(res.Runs) < minRunsPerScreen {
		t.Fatalf("only %d pairing(s) compared, so the two implementations were barely "+
			"compared at all. That is a comparison that did not run, not a pass.", len(res.Runs))
	}
	t.Logf("%d pairing(s) agree; the widest gap is %.3f%% — %s",
		len(res.Runs), worst*100, worstAt)
}
