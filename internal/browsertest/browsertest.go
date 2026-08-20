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
// And the browser needs a deadline of its own. `go test ./...` runs packages in parallel,
// so a browser here competes with the replay suite for the same cores; without a deadline a
// wedged one hangs until the test binary's ten-minute limit and takes the package's output
// with it. The deadline is loose on purpose — its job is to turn a wedge into a fast
// failure, not to catch a slow render.
package browsertest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
//
// ⚠️ EXPERIMENT — this order and the log line below are the whole content of this branch.
// A probe, not a pin. See the pull request.
//
// The layout goldens have never passed in CI, from the commit that introduced them onward.
// The renderer is an unpinned input to a pixel-exact comparison, and TWO things differ
// between the machine that generates the goldens and the machine that checks them:
//
//	development machine   arm64,  snap Chromium 151.0.7922.108   ← rendered the goldens
//	GitHub ubuntu-latest  amd64,  Chromium 151.0.7922.0          ← what CI was picking
//	                      amd64,  Google Chrome 151.0.7922.108   ← also present, never reached
//
// Every golden comes back differing by a worst channel delta of 2 of 255 over thousands of
// pixels, on shots no commit has touched. That is text rasterisation, not layout.
//
// ⚠️ THE ARCHITECTURE DIFFERENCE CANNOT BE REMOVED BY CHOOSING A BROWSER. Both of the
// runner's browsers are amd64, and the arm64 runner image carries no Chromium or Chrome at
// all — only Firefox — so moving the job there would make Find SKIP, which is a vacuous
// pass and worse than a red build. Reordering therefore holds the browser VERSION constant
// and leaves the instruction set as the only remaining difference.
//
// So this branch narrows the question rather than answering it outright:
//
//	green  →  version was the whole story; arm64 vs amd64 does not move these pixels, and
//	          a version pin plus an assertion on it is the fix.
//	red    →  the difference survives an identical version, so it is the architecture, the
//	          packaging (snap vs deb) or the font stack underneath. A pixel-exact golden is
//	          then not portable between these two machines at all, and the fix is a pinned
//	          renderer both machines can run, or an instrument that is not pixel-exact.
//
// Neither answer is available from the development machine: it cannot execute an amd64
// browser (no qemu, no binfmt) and no container runtime is installed. CI is the only place
// this can be measured, which is why it is asked as a branch.
//
// The log line below is what makes either answer readable. Without it the run reports a
// pixel delta and not what rendered it, which is how this went undiagnosed from 4691fae
// onward.
func Find(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			t.Logf("renderer: %s | %s | GOARCH=%s", p, browserVersion(p), runtime.GOARCH)
			return p
		}
	}
	t.Skip("no chromium on PATH; this suite needs a browser")
	return ""
}

// browserVersion reports the browser's own version string, for the log line in Find.
//
// Best-effort by design: this is diagnostic output, and a browser that will not answer
// --version is not a reason to fail a suite that is about to drive it successfully. The
// error text is returned in place of the version so the log still says something true.
func browserVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "version unavailable: " + err.Error()
	}
	return strings.TrimSpace(string(out))
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
