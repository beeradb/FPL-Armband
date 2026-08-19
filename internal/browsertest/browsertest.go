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
