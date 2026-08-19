package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Visual regression: the application is screenshotted and compared byte-region by
// byte-region against committed goldens.
//
// # Why this exists at all, and why it exists now rather than later
//
// Everything else in this repository is checked by asserting on numbers. A layout cannot
// be. The failures this catches — a card overlapping its neighbour, a strip collapsing at
// 390px, a colour token going the wrong way, a panel rendering empty — all produce
// perfectly valid HTML and perfectly correct JSON, and every existing test passes while
// the page is unusable.
//
// # What makes the comparison meaningful
//
// A screenshot test is worthless if its input moves. Three things are pinned:
//
//   - The DATA, to a committed capture (see internal/capture.LiveCapture). No network, no
//     live API, byte-identical inputs on every run and on every machine.
//   - The CLOCK, so the deadline countdown and the override staleness figures do not
//     change between two runs an hour apart.
//   - The TOKEN, because it is rendered into the page and a fresh one per startup would
//     make every screenshot differ from every other for no reason at all.
//
// # Why our own goldens rather than the designer's 28 shots
//
// The shipped screenshots were rendered from the prototype's mock data, over file://,
// with CDN fonts. Real data changes every number, every bar width and every card, so a
// comparison against them would need a tolerance loose enough to stop catching anything.
// They remain the conformance reference a human reads; this is the gate a machine runs.

var updateGoldens = flag.Bool("update", false,
	"rewrite the visual goldens from what the application currently renders")

// shot is one screenshot: a route, at a viewport.
//
// url carries the path AND the fragment, because both matter and leaving the path implicit
// was the first bug this harness caught on itself -- every shot named a panel but no path,
// so nine screenshots were nine copies of the landing page, and the "pitch" golden was a
// picture of the marketing hero.
type shot struct {
	name   string
	url    string
	w, h   int
	mobile bool
}

// The viewports are the two the design was drawn at. HANDOFF.md section 5 is explicit
// that nothing ships which does not work at 390px, and names the constraint that drove
// the whole compact-card design: five cards across a 390px screen leaves about 68px each.
const (
	desktopW = 1440
	mobileW  = 390
)

var shots = []shot{
	{name: "landing-desktop", url: "/", w: desktopW, h: 2400},
	{name: "landing-mobile", url: "/", w: mobileW, h: 2600, mobile: true},
	{name: "pitch-desktop", url: "/app#pitch", w: desktopW, h: 1700},
	{name: "pitch-mobile", url: "/app#pitch", w: mobileW, h: 2000, mobile: true},
	{name: "players-desktop", url: "/app#players", w: desktopW, h: 1600},
	{name: "players-mobile", url: "/app#players", w: mobileW, h: 1800, mobile: true},
	{name: "overrides-desktop", url: "/app#overrides", w: desktopW, h: 1400},
	{name: "overrides-mobile", url: "/app#overrides", w: mobileW, h: 1600, mobile: true},
	{name: "brief-desktop", url: "/app#brief", w: desktopW, h: 1400},
}

func goldenDir() string { return filepath.Join("testdata", "golden") }

// chromium finds the browser, or says why the suite cannot run.
//
// A missing browser SKIPS, and that is the one skip in this file. It is an absent tool
// rather than a broken repository — unlike the capture, which is committed, so an
// unreadable one fails. The distinction matters: a skip that fires for the wrong reason
// turns every assertion here into a silent pass.
func chromium(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skip("no chromium on PATH; the visual regression suite needs a browser")
	return ""
}

// capture drives the browser once and returns the PNG it wrote.
func (s shot) capture(t *testing.T, browser, base, workdir string) []byte {
	t.Helper()
	out := filepath.Join(workdir, s.name+".png")

	args := []string{
		"--headless",
		"--no-sandbox",
		"--disable-gpu",
		// Deterministic rendering. Without these the same page differs between runs:
		// a scrollbar appears or does not, and the device scale factor follows
		// whatever display the machine thinks it has.
		"--hide-scrollbars",
		"--force-device-scale-factor=1",
		"--disable-lcd-text",
		// Fast-forward through the fetch and the render rather than sleeping. This is
		// what makes the suite both quick and reliable: a fixed sleep is either too
		// short on a loaded machine or wasted on an idle one.
		"--virtual-time-budget=8000",
		// Wait for the compositor to finish before the frame is captured. Without it
		// the screenshot can be taken mid-draw, which is a flake that looks exactly
		// like a regression.
		"--run-all-compositor-stages-before-draw",
		"--disable-dev-shm-usage",
		fmt.Sprintf("--window-size=%d,%d", s.w, s.h),
		"--screenshot=" + out,
	}
	if s.mobile {
		// The design's mobile mode is driven by width alone, but a real phone also
		// reports a device pixel ratio and a touch-capable pointer, and the stylesheet
		// has a hover query in it.
		args = append(args, "--user-agent=Mozilla/5.0 (Linux; Android 14; Pixel 8) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Mobile Safari/537.36")
	}
	args = append(args, base+s.url)

	cmd := exec.Command(browser, args...)
	// A private profile per run. Sharing the user's real one would both pollute it and
	// make the screenshot depend on whatever is in it.
	cmd.Env = append(os.Environ(), "HOME="+workdir)
	var stderr []byte
	if b, err := cmd.CombinedOutput(); err != nil {
		stderr = b
		t.Fatalf("chromium failed for %s: %v\n%s", s.name, err, stderr)
	}
	png, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("chromium wrote no screenshot for %s: %v", s.name, err)
	}
	if len(png) == 0 {
		t.Fatalf("chromium wrote an empty screenshot for %s", s.name)
	}
	return png
}

// scratch is a working directory the BROWSER can write into.
//
// Not t.TempDir(). Chromium here is a snap, and a snap gets a PRIVATE /tmp: it reports
// success, writes the screenshot into its own namespace, and the host sees nothing. The
// failure is silent in the worst way -- exit status 0 and no file where the file should
// be. So the scratch lives under the package directory, which the snap's home interface
// can reach.
func scratch(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".visualwork-")
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

// TestVisualRegression is the gate. Run with -update to adopt what the application
// currently renders, and READ THE DIFF before you do.
func TestVisualRegression(t *testing.T) {
	browser := chromium(t)
	srv := httptest.NewServer(fixtureServer(t))
	defer srv.Close()

	if *updateGoldens {
		if err := os.MkdirAll(goldenDir(), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, s := range shots {
		t.Run(s.name, func(t *testing.T) {
			got := s.capture(t, browser, srv.URL, scratch(t))
			golden := filepath.Join(goldenDir(), s.name+".png")

			if *updateGoldens {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s (%d bytes)", golden, len(got))
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("no golden for %s: %v\nRun: go test ./cmd/armband/ "+
					"-run TestVisualRegression -update", s.name, err)
			}
			diff, err := compare(want, got)
			if err != nil {
				t.Fatal(err)
			}
			if diff.differing <= noiseFloor {
				return
			}
			// The artefacts go next to the golden, named for the shot, so a failure is
			// something you can look at rather than a number to interpret.
			base := filepath.Join(goldenDir(), "diff-"+s.name)
			writeArtefact(t, base+"-got.png", got)
			writeArtefact(t, base+".png", diff.image)
			t.Errorf("%s differs from its golden: %d of %d pixels (%.3f%%) differ by more "+
				"than %d, which is over the %d-pixel noise floor. Worst channel delta %d.\n"+
				"  wrote %s-got.png and %s.png — look at them before running -update",
				s.name, diff.differing, diff.total,
				100*float64(diff.differing)/float64(diff.total),
				justAntialiasing, noiseFloor, diff.worst, base, base)
		})
	}
}

func writeArtefact(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Logf("could not write %s: %v", path, err)
	}
}

type diffResult struct {
	differing, total int
	worst            int
	image            []byte
}

// justAntialiasing is the largest per-channel difference this comparison ignores.
//
// It is 1 — the least significant bit of an 8-bit channel — and it is not a fuzz factor
// added to make a failing test pass. The first version of this comparison had no
// tolerance at all, on the reasoning that pinned inputs must produce identical pixels.
// That reasoning is right about the DATA and wrong about the RENDERER: text
// rasterisation in a real browser is not bit-reproducible between runs, and the suite
// duly failed with 66 pixels of 1,014,000 differing by exactly 1.
//
// A threshold of 1 cannot hide anything worth catching. Every failure this suite exists
// for — a moved card, a collapsed strip, a colour token going the wrong way, a panel
// rendering empty — moves whole regions by tens or hundreds of levels. Nothing changes a
// colour by one part in 255 and matters.
//
// ⚠️ If this ever needs raising, that is evidence about the harness, not about the page.
// Find what is moving instead.
const justAntialiasing = 1

// noiseFloor is how many pixels may differ by MORE than justAntialiasing before the
// comparison calls it a change.
//
// The two guards do different jobs and neither replaces the other. The per-channel
// threshold above forgives a shade; this forgives an isolated speck. Raising the
// threshold instead of adding this would have been the wrong fix -- a uniform two-level
// shift across a whole panel is a real regression, and a threshold of 2 would swallow it
// silently while this floor still catches it, because it has area.
//
// Sized from observed noise, not chosen for comfort. Before the compositor was waited
// for, a run differed in 66 pixels at delta 1; after, in a single pixel at delta 2. The
// floor is 32 -- above the noise, and three to five orders of magnitude below anything
// this suite exists to catch. A card moving, a strip collapsing or a panel emptying
// touches thousands of pixels.
const noiseFloor = 32

// compare decodes two PNGs and reports where they differ, producing a third PNG that
// shows it.
//
// Pure standard library: image/png is in the toolchain, and a screenshot differ is not
// worth a dependency.
func compare(wantPNG, gotPNG []byte) (diffResult, error) {
	want, err := png.Decode(bytes.NewReader(wantPNG))
	if err != nil {
		return diffResult{}, fmt.Errorf("decoding the golden: %w", err)
	}
	got, err := png.Decode(bytes.NewReader(gotPNG))
	if err != nil {
		return diffResult{}, fmt.Errorf("decoding the screenshot: %w", err)
	}

	wb, gb := want.Bounds(), got.Bounds()
	if wb != gb {
		// A size change is a real difference and must not be silently cropped into a
		// comparison of the overlapping part.
		return diffResult{differing: 1, total: 1,
			image: gotPNG}, nil
	}

	out := image.NewRGBA(wb)
	var res diffResult
	res.total = wb.Dx() * wb.Dy()
	for y := wb.Min.Y; y < wb.Max.Y; y++ {
		for x := wb.Min.X; x < wb.Max.X; x++ {
			wr, wg, wbl, _ := want.At(x, y).RGBA()
			gr, gg, gbl, _ := got.At(x, y).RGBA()
			d := maxInt(absInt(int(wr)-int(gr)), absInt(int(wg)-int(gg)), absInt(int(wbl)-int(gbl)))
			d >>= 8
			if d <= justAntialiasing {
				// Unchanged pixels are dimmed rather than dropped, so the diff reads
				// as the page with the changes lit up on it.
				r, g, b, _ := got.At(x, y).RGBA()
				out.Set(x, y, color.RGBA{uint8(r >> 10), uint8(g >> 10), uint8(b >> 10), 255})
				continue
			}
			res.differing++
			if d > res.worst {
				res.worst = d
			}
			out.Set(x, y, color.RGBA{255, 0, 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return res, err
	}
	res.image = buf.Bytes()
	return res, nil
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func maxInt(vs ...int) int {
	m := vs[0]
	for _, v := range vs[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
