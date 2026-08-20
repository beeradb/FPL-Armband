// Package browsertest drives a headless browser for tests.
//
// It exists because two packages need it and the setup is full of traps that are expensive
// to rediscover. It follows the convention of net/http/httptest: a normal package that
// imports testing, used only from tests.
//
// # The traps, recorded here so they are found once
//
// Chromium on this machine is a SNAP, and a snap gets a private /tmp. It reports success,
// writes the screenshot into its own namespace, and the host sees nothing — exit status 0
// and no file where the file should be. So Scratch never returns t.TempDir().
//
// A render is not deterministic by default. Without --hide-scrollbars a scrollbar appears
// or does not; without --force-device-scale-factor the ratio follows whatever display the
// machine believes in; without --run-all-compositor-stages-before-draw the frame can be
// captured mid-draw. TZ is pinned because any page formatting a date with
// toLocaleDateString otherwise renders differently in every timezone, which makes a golden
// reproducible only where it was generated.
//
// The window is not the viewport either, and the two ways out of this browser disagree about
// which. --dump-dom will not give a page a window narrower than 500px whatever --window-size
// says, while --screenshot goes through a device-metrics override and honours 390 exactly. A
// phone measured through the wrong one is still narrow enough to keep every mobile rule, so
// it looks right and nothing says otherwise. See minDumpDOMWidth and FrameHTML.
//
// And the browser needs a deadline of its own. `go test ./...` runs packages in parallel,
// so a browser here competes with the replay suite for the same cores; without a deadline a
// wedged one hangs until the test binary's ten-minute limit and takes the package's output
// with it. The deadline is loose on purpose — its job is to turn a wedge into a fast
// failure, not to catch a slow render.
package browsertest

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// deadline is how long any single browser invocation may take.
//
// Deliberately loose. Both 60s and 180s fired on full-suite runs and passed on their own a
// moment later — a flake introduced by the guard against flakes.
const deadline = 300 * time.Second

// Find returns the browser, or skips.
//
// A missing browser is an absent tool rather than a broken repository, which is the one
// thing in these suites that legitimately skips. Everything else — the committed capture,
// the committed fixtures — fails, because a skip that fires for the wrong reason turns
// every assertion behind it into a silent pass.
func Find(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("no chromium on PATH; this suite needs a browser")
	return ""
}

// Scratch is a directory the BROWSER can write into, under the package directory.
//
// Not t.TempDir(). See the package comment: a snap's /tmp is not the host's.
func Scratch(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".browserwork-")
	if err != nil {
		t.Fatalf("making a scratch directory the browser can write to: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// Viewport is a window size, and whether to present as a phone.
type Viewport struct {
	Width, Height int
	// Mobile sends a phone user-agent as well as the width. The design's mobile mode is
	// driven by width alone, but the stylesheet carries a hover query and a real phone
	// reports a coarse pointer.
	Mobile bool
}

func (v Viewport) args() []string {
	a := []string{fmt.Sprintf("--window-size=%d,%d", v.Width, v.Height)}
	if v.Mobile {
		a = append(a, "--user-agent=Mozilla/5.0 (Linux; Android 14; Pixel 8) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Mobile Safari/537.36")
	}
	return a
}

func base(work string) []string {
	return []string{
		"--headless",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--hide-scrollbars",
		"--force-device-scale-factor=1",
		"--disable-lcd-text",
		// Fast-forward through the fetch and the render rather than sleeping: a fixed
		// sleep is either too short on a loaded machine or wasted on an idle one.
		"--virtual-time-budget=8000",
		// Wait for the compositor before the frame is captured, or the screenshot can be
		// taken mid-draw — a flake that looks exactly like a regression.
		"--run-all-compositor-stages-before-draw",
	}
}

func run(t *testing.T, browser, work string, args []string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	cmd := exec.CommandContext(ctx, browser, args...)
	// A private profile per run, and a pinned timezone. HOME points at the scratch so the
	// browser neither reads nor pollutes the user's real profile.
	cmd.Env = append(os.Environ(), "HOME="+work, "TZ=UTC")
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("the browser did not finish within %s", deadline)
	}
	return out, err
}

// Screenshot renders url at v and returns the PNG.
func Screenshot(t *testing.T, browser, url string, v Viewport) []byte {
	t.Helper()
	work := Scratch(t)
	out := filepath.Join(work, "shot.png")

	args := append(base(work), v.args()...)
	args = append(args, "--screenshot="+out, url)
	if _, err := run(t, browser, work, args); err != nil {
		t.Fatalf("screenshotting %s: %v", url, err)
	}
	png, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the browser wrote no screenshot for %s: %v", url, err)
	}
	if len(png) == 0 {
		t.Fatalf("the browser wrote an empty screenshot for %s", url)
	}
	return png
}

// DumpDOM renders url and returns the DOM the parser built.
//
// Not a screenshot, because the question it answers is what the parser built rather than
// what it looked like — an injected <script> is invisible in a picture.
func DumpDOM(t *testing.T, browser, url string) string {
	t.Helper()
	work := Scratch(t)

	args := append(base(work), "--dump-dom", url)
	body, err := run(t, browser, work, args)
	if err != nil {
		t.Fatalf("rendering %s: %v", url, err)
	}
	if len(body) < 500 {
		t.Fatalf("the browser returned %d bytes of DOM for %s; the page did not render",
			len(body), url)
	}
	return string(body)
}

// ProbeID is the id of the element a page publishes its answer in. See Probe.
const ProbeID = "browsertest-probe"

// FrameID is the id of the iframe FrameHTML builds.
const FrameID = "browsertest-frame"

// minDumpDOMWidth is the narrowest window --dump-dom will give a page, whatever
// --window-size says.
//
// MEASURED, 2026-08-19, Chromium 151: asking for 390x844 and reporting window.innerWidth
// from inside the page gives 500x757. --screenshot does NOT have this floor — it drives the
// capture through a device-metrics override, which is why the committed phone goldens really
// are 390 wide and really do show a 390 layout. So the two paths out of this browser
// disagree about the viewport, and only one of them says so.
//
// That matters more here than anywhere else in this package: a phone check run at 500px is
// not a phone check. It is wide enough to keep every `max-width:720px` rule, so the page
// still LOOKS mobile and nothing announces the difference — the same shape as the wrong
// finding the layout goldens produced by rendering a phone at a height no phone has.
// FrameHTML is the way round it.
const minDumpDOMWidth = 500

// FrameHTML is a wrapper document that renders src at EXACTLY v.Width x v.Height.
//
// An iframe has its own layout viewport, so media queries, `vh` lengths and `position:fixed`
// inside it resolve against the iframe's box rather than the browser window — which is the
// only way to get a true 390x844 out of --dump-dom, whose window has a 500px floor. It works
// even where the frame is larger than the window that holds it, because an iframe's layout
// does not depend on how much of it happens to be visible.
//
// The framed page is expected to publish its own result; a probe running inside a frame must
// write into the PARENT document, because --dump-dom serialises the main frame only.
func FrameHTML(src string, v Viewport) []byte {
	return []byte(fmt.Sprintf(`<!doctype html>
<meta charset="utf-8">
<title>browsertest frame</title>
<style>html,body{margin:0;padding:0;background:#000;overflow:hidden}
iframe{display:block;border:0;width:%dpx;height:%dpx}</style>
<iframe id="%s" src="%s"></iframe>
`, v.Width, v.Height, FrameID, html.EscapeString(src)))
}

// OuterFor is the window size to give the browser when rendering FrameHTML at v.
//
// The frame decides the layout, so this only has to be big enough not to be silly — but it
// must respect the floor above, or the request is quietly rewritten and a reader comparing
// flags with reality finds them disagreeing.
func OuterFor(v Viewport) Viewport {
	out := v
	if out.Width < minDumpDOMWidth {
		out.Width = minDumpDOMWidth
	}
	return out
}

// Probe renders url at v and returns the JSON the page published for the test.
//
// Screenshot and DumpDOM ask what a page LOOKS like and what its parser BUILT. Probe asks
// the third question — what the rendered page computes about itself — which is the only one
// that can reach layout, inheritance and compositing. The page does the computing, because
// there is nowhere else those answers exist; the test decides what to do with them.
//
// The transport is deliberately dull. --dump-dom is the only channel out of this browser, so
// the page publishes into
//
//	<script type="application/json" id="browsertest-probe">BASE64</script>
//
// and Probe decodes it. Base64 rather than raw JSON because an HTML serialisation escapes
// text and a script element ends at the first literal "</script", so any payload carrying a
// quote, an angle bracket or a non-ASCII character would need a quoting rule that the page
// and this function would then have to agree on forever. Base64 needs no such agreement.
//
// A page that publishes nothing is a FAILURE, not an empty result: the usual cause is that
// the page threw before it got there, and returning "no findings" would turn every assertion
// downstream into a silent pass.
func Probe(t *testing.T, browser, url string, v Viewport) []byte {
	t.Helper()
	work := Scratch(t)

	args := append(base(work), v.args()...)
	args = append(args, "--dump-dom", url)
	body, err := run(t, browser, work, args)
	if err != nil {
		t.Fatalf("rendering %s: %v", url, err)
	}

	open := `id="` + ProbeID + `">`
	i := bytes.Index(body, []byte(open))
	if i < 0 {
		t.Fatalf("%s published no probe result (%d bytes of DOM). The page did not reach "+
			"the point where it publishes: run the same URL in a browser and read the console.",
			url, len(body))
	}
	rest := body[i+len(open):]
	j := bytes.Index(rest, []byte("</script>"))
	if j < 0 {
		t.Fatalf("%s published an unterminated probe result", url)
	}
	out, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(rest[:j])))
	if err != nil {
		t.Fatalf("%s published a probe result that is not base64: %v", url, err)
	}
	return out
}
