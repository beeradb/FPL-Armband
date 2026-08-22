package webui_test

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/webui"
)

// ⚠️ -update rewrites EVERY golden, not only the ones that failed: the branch below
// writes and returns before any comparison runs. So a shot your change cannot have
// touched still comes back rewritten, and the render is only deterministic to within
// browsertest.NoiseFloor — a rewrite can bank a difference the comparison would have
// forgiven. Observed: phone-picker.png came back modified on a run that changed only
// the landing page, and reverting it left the suite green. Check `git status` after
// every -update and revert whatever your change cannot explain.
var update = flag.Bool("update", false,
	"rewrite the layout goldens from what the application currently renders. "+
		"⚠️ EVERY golden, not only the failing ones — check `git status` afterwards "+
		"and revert what your change cannot explain")

// The layout suite: the application rendered against fixed documents.
//
// It asserts LAYOUT and nothing else — that a card does not overlap its neighbour, that a
// strip does not collapse at 390px, that a colour token has not gone the wrong way, that a
// panel is not empty. See statefixture_test.go for why the documents are fixed rather than
// produced by the model, which is the question this suite used to conflate with layout and
// could only ever answer for one gameweek.

// serve stands up the application over a fixed state document.
//
// Deliberately not cmd/armband's server. Nothing here needs the optimiser, the capture, a
// clock or a token: the pages and the assets come from this package, and /api/state is a
// file. That makes a shot cost a browser launch rather than a browser launch plus a second
// of optimiser, and it means a model change cannot move a golden.
func serve(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "state", fixture+".json"))
	if err != nil {
		t.Fatalf("reading the state fixture: %v", err)
	}
	// Decoded once here so a fixture that no longer fits the contract fails as a test
	// rather than as a strange screenshot.
	loadState(t, fixture)

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.StaticHandler("/assets/"))
	mux.HandleFunc("/probe.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("testdata", "contrast_probe.js"))
	})
	// The stacks whose composited answer is known by construction. Not part of the
	// application: it is what gives the contrast suite power over its own resolution.
	mux.HandleFunc("/stacks", func(w http.ResponseWriter, r *http.Request) {
		page, err := os.ReadFile(filepath.Join("testdata", "contrast_stacks.html"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		withProbe, err := probed(page)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(withProbe)
	})
	// The frame that gives a probed page an exact viewport. See browsertest.FrameHTML for
	// why a phone cannot simply be asked for with --window-size.
	mux.HandleFunc("/probe-frame", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		width, _ := strconv.Atoi(q.Get("w"))
		height, _ := strconv.Atoi(q.Get("h"))
		src := q.Get("src")
		if width <= 0 || height <= 0 || src == "" {
			http.Error(w, "the frame needs src, w and h", 400)
			return
		}
		// A frame source is a page on this server and nothing else. `javascript:` and
		// `data:` are the two schemes that would turn a query parameter into code; there is
		// nothing here to steal and the listener lives for one test, so this is not a
		// vulnerability being closed — it is the handler refusing input it has no use for.
		if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
			http.Error(w, "the frame renders http(s) pages only", 400)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(browsertest.FrameHTML(src,
			browsertest.Viewport{Width: width, Height: height}))
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
	})
	// /armband-team is a separate document from /app on purpose (see
	// cmd/armband/webroutes.go's routeArmbandTeam comment) -- its data happens to come
	// from the same fixture file here only because Results already rides along in
	// State for this test's convenience; the real server builds it from an entirely
	// different, session-less path (armbandTeamState).
	mux.HandleFunc("/api/armband-team", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := "landing"
		if r.URL.Path == "/app" {
			name = "app"
		} else if r.URL.Path == "/armband-team" {
			name = "team"
		} else if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		page, err := webui.Page(name)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// ?probe=1 adds the contrast probe, and ONLY then: the goldens above must be shot
		// off the same document the application serves, so the probe is opt-in per request
		// rather than a second server that would drift from this one. See contrast_test.go.
		//
		// These report through http.Error rather than t.Fatalf. A Fatalf here would run
		// runtime.Goexit on the HANDLER's goroutine, so the request would never complete and
		// the browser would hang — surfacing as "published no probe result", which names
		// neither the cause nor the file.
		if r.URL.Query().Get("probe") != "" {
			if page, err = probed(page); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		if tok := r.URL.Query().Get("regress"); tok != "" {
			if page, err = brokenToken(page, tok); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

type shot struct {
	name    string
	fixture string
	url     string
	view    browsertest.Viewport
}

const (
	desktopW = 1440
	mobileW  = 390
)

func desktop(h int) browsertest.Viewport { return browsertest.Viewport{Width: desktopW, Height: h} }
func mobile(h int) browsertest.Viewport {
	return browsertest.Viewport{Width: mobileW, Height: h, Mobile: true}
}

// phone is a real device viewport, and it is the only one here that is not chosen to fit a
// whole page into one image.
//
// 390x844 is the iPhone 14/15/16 and the shortest of the common sizes, which makes it the
// height at which a viewport-relative clamp binds first. Everything sized in `vh`, or
// positioned `fixed` or `sticky`, behaves differently here from the tall shots above — and
// differently is the point, because the tall shots cannot reach these states at all.
func phone() browsertest.Viewport {
	return browsertest.Viewport{Width: mobileW, Height: 844, Mobile: true}
}

// The screens. Both viewports for anything whose mobile mode is a different layout rather
// than the same one narrower — HANDOFF.md section 5 is explicit that nothing ships which
// does not work at 390px, and names the constraint that drove the compact card: five across
// a 390px screen leaves about 68px each.
var shots = []shot{
	{"landing-desktop", "gameweek-one", "/", desktop(2400)},
	{"landing-mobile", "gameweek-one", "/", mobile(2600)},
	{"pitch-desktop", "gameweek-one", "/app#pitch", desktop(1700)},
	{"pitch-mobile", "gameweek-one", "/app#pitch", mobile(2000)},
	{"players-desktop", "gameweek-one", "/app#players", desktop(1600)},
	{"players-mobile", "gameweek-one", "/app#players", mobile(1800)},
	{"news-desktop", "gameweek-one", "/app#news", desktop(1400)},
	{"news-mobile", "gameweek-one", "/app#news", mobile(1600)},
	{"brief-desktop", "gameweek-one", "/app#brief", desktop(1400)},
	{"armband-team-desktop", "gameweek-one", "/armband-team", desktop(1400)},
	{"armband-team-mobile", "gameweek-one", "/armband-team", mobile(2200)},

	// The team-import control, on the Pitch where it now lives.
	//
	// ⚠️ Until 2026-08-22 this feature had NO shot at any width, in either of its
	// positions, and the `import-offered` fixture that exists precisely to render it was
	// referenced only by statefixture_test.go's decode check. So the one screen a new
	// reader is most likely to meet first was the one screen the visual suite could not
	// see. It is also invisible in the `gameweek-one` shots and always was, for a reason
	// worth stating so nobody "fixes" it there: that fixture's import window is CLOSED,
	// so the panel renders hidden and a diff of those shots proves nothing about this
	// control either way.
	//
	// Two fixtures because the control has two states a screenshot can hold still, and
	// they are the two the design turns on: the offer (panel auto-open, no entry on
	// record) and the aftermath (entry on record, panel shut, #squadsource carrying the
	// provenance line and its "Change team" button). The third state — the change panel
	// actually open — needs a click, and there is no deep link for it the way there is
	// for the replacement picker, so it is deliberately not shot rather than shot wrong.
	{"import-offered-desktop", "import-offered", "/app#pitch", desktop(1800)},
	{"import-offered-mobile", "import-offered", "/app#pitch", mobile(2200)},
	{"import-imported-desktop", "import-imported", "/app#pitch", desktop(1700)},

	// The states live data will not hand you on demand, and which are therefore the most
	// likely to be broken and the least likely to be looked at.
	{"edges-pitch-desktop", "edges", "/app#pitch", desktop(1700)},
	{"edges-pitch-mobile", "edges", "/app#pitch", mobile(2000)},
	{"edges-players-desktop", "edges", "/app#players", desktop(1200)},
	{"edges-news-desktop", "edges", "/app#news", desktop(1200)},

	// The replacement picker, reached by its deep link. It is a panel that otherwise only
	// exists after a tap, and the states worth seeing are the ones a screenshot can hold
	// still: the default position-and-budget view, and the same panel at 390px where the
	// sheet becomes a bottom sheet and the staged bar has to stay reachable.
	{"picker-desktop", "gameweek-one", "/app#replace-542", desktop(1500)},
	{"picker-mobile", "gameweek-one", "/app#replace-542", mobile(1700)},

	// The rotation-risk nudge headline, rendered when no player is flagged but one player
	// carries rotation risk with no news. The third headline had no coverage until this fixture.
	{"rotation-risk-desktop", "rotation-risk", "/app#pitch", desktop(1700)},

	// ---- and the same screens at a height a real device actually has -------
	//
	// Every shot above is deliberately TALL, because its job is to capture a whole page in
	// one image. That makes them blind to everything sized against the viewport: at
	// mobile(1700) the sheet's `max-height:88vh` resolves to about 1500px, the clamp never
	// binds, and the sheet is simply content-sized. So the shot cannot show the sheet
	// SCROLLING, which is the state its sticky header exists to serve.
	//
	// That blindness produced a wrong finding on 2026-08-19: the picker's close button was
	// reported as pushed off-screen, reasoned from picker-mobile. It is not — the header is
	// `position:sticky` inside an `overflow-y:auto` sheet, and on a real phone it stays
	// pinned. The evidence for the bug was an artefact of the instrument.
	//
	// `armband.css` carries 9 `vh` lengths, 3 `position:fixed` and 7 `position:sticky`
	// rules, and until these cases existed not one of them was exercised at a height any
	// device has. 844 is the iPhone 14/15/16 viewport and the shortest of the common ones,
	// so it is the height at which a clamp binds first.
	//
	// ⚠️ These are NOT full-page shots and must not be "fixed" by making them taller. A
	// taller viewport is precisely what stops them testing anything.
	{"phone-pitch", "gameweek-one", "/app#pitch", phone()},
	{"phone-picker", "gameweek-one", "/app#replace-542", phone()},
	{"phone-sheet-edges", "edges", "/app#pitch", phone()},

	// The buy-mode tray for the redesigned market row controls.
	{"buytray-desktop", "gameweek-one", "/app#buy-411", desktop(1500)},
	{"phone-buytray", "gameweek-one", "/app#buy-411", phone()},
}

func goldenDir() string { return filepath.Join("testdata", "golden") }

// ⚠️ NOT RUN IN CI, and as of 2026-08-21 that is a DECISION rather than a suppression.
//
// These goldens are machine-dependent. On GitHub's runners every shot differs from the
// committed PNG by a worst channel delta of 2 out of 255 — a uniform renderer difference,
// not a visual change — and the comparison allows only browsertest.NoiseFloor pixels to
// differ by more than 1, so thousands of pixels a hair over that threshold blow the budget
// instantly. The same content passes locally. `main` was red on six subtests continuously
// from 2026-08-19, and this block went in on 2026-08-20 to stop a red gate training people
// to merge over it.
//
// It is skipped rather than LOOSENED, deliberately. Raising the magnitude tolerance to 2
// would make CI green and would also blind the check to any real change smaller than that,
// on every screen, forever — and nobody would know it had stopped catching things.
//
// ⚠️ THE EARLIER VERSION OF THIS COMMENT ENDED "When it is fixed, DELETE this block". That
// sentence is withdrawn. It described work nobody had scheduled, which is worse than the
// skip: it reads as a debt somebody is clearing, so nobody does. The choice was put and
// taken on 2026-08-21 — pin a browser build in a container for both local runs and CI, or
// accept that these are a LOCAL check and defend the invariants another way. The second was
// chosen.
//
// So the standing arrangement, not a stopgap:
//
//   - In CI this suite proves the pages RENDER. It does not prove they render correctly,
//     and it is not pretending to.
//   - Locally it still COMPARES, which is where it has always caught regressions, and it
//     stays the thing to run before shipping anything visual.
//   - What must not regress silently is asserted over the markup instead, where it needs no
//     browser and runs everywhere. TestTheHeroPeekAddsUp is the pattern: the landing peek
//     drew nine players under a 3-5-2 label twice, and BOTH goldens were regenerated inside
//     the commit that broke it, so the pictures were updated to match the bug. A golden
//     defends nothing against the change that rewrites it. An assertion does.
//
// ⚠️ Reviving the CI comparison means pinning the renderer, not raising the tolerance. If
// somebody does that, delete this block — but do not delete it merely because the skip is
// annoying.
func TestLayout(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "" && os.Getenv("FPL_LAYOUT_GOLDENS") == "" {
		t.Skip("layout goldens are a LOCAL check by decision, not a pending fix: they are " +
			"machine-dependent on CI runners (worst channel delta 2/255). Compared in full " +
			"locally; invariants that must not regress silently are asserted over the markup " +
			"instead. Set FPL_LAYOUT_GOLDENS=1 to force. See the comment above this test.")
	}

	browser := browsertest.Find(t)

	if *update {
		if err := os.MkdirAll(goldenDir(), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// One server per fixture rather than one per shot.
	servers := map[string]*httptest.Server{}
	for _, s := range shots {
		if servers[s.fixture] == nil {
			servers[s.fixture] = serve(t, s.fixture)
		}
	}

	for _, s := range shots {
		t.Run(s.name, func(t *testing.T) {
			got := browsertest.Screenshot(t, browser, servers[s.fixture].URL+s.url, s.view)
			golden := filepath.Join(goldenDir(), s.name+".png")

			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("wrote %s (%d bytes)", golden, len(got))
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("no golden for %s: %v\nRun: go test ./internal/webui/ "+
					"-run TestLayout -update", s.name, err)
			}
			diff, err := browsertest.Compare(want, got)
			if err != nil {
				t.Fatal(err)
			}
			if diff.OK() {
				return
			}
			base := filepath.Join(goldenDir(), "diff-"+s.name)
			write(t, base+"-got.png", got)
			write(t, base+".png", diff.Image)
			if diff.Resized {
				t.Errorf("%s is a different SIZE from its golden — the layout's height or "+
					"width changed, and a pixel comparison cannot say anything useful about "+
					"that.\n  wrote %s-got.png", s.name, base)
				return
			}
			t.Errorf("%s differs from its golden: %d of %d pixels (%.3f%%) differ by more "+
				"than %d, over the %d-pixel noise floor. Worst channel delta %d.\n"+
				"  wrote %s-got.png and %s.png — look at them before running -update",
				s.name, diff.Differing, diff.Total,
				100*float64(diff.Differing)/float64(diff.Total),
				browsertest.JustAntialiasing, browsertest.NoiseFloor, diff.Worst, base, base)
		})
	}
}

func write(t *testing.T, path string, body []byte) {
	t.Helper()
	if len(body) == 0 {
		return
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Logf("could not write %s: %v", path, err)
	}
}

// TestTheFixtureServerIsNotTheProductServer records why this package stands up its own.
//
// It is one assertion and one paragraph, and the paragraph is the point: if someone later
// wires these shots to cmd/armband's server to "test the real thing", the suite goes back
// to being a picture of one gameweek's model output, and every model change churns the
// goldens again.
func TestTheFixtureServerIsNotTheProductServer(t *testing.T) {
	srv := serve(t, "gameweek-one")
	res, err := http.Get(srv.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatalf("the fixture server did not serve a decodable state: %v", err)
	}
	if _, ok := doc["squad"]; !ok {
		t.Error("the served fixture has no squad")
	}
}
