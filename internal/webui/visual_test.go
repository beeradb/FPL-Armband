package webui_test

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/webui"
)

var update = flag.Bool("update", false,
	"rewrite the layout goldens from what the application currently renders")

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
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := "landing"
		if r.URL.Path == "/app" {
			name = "app"
		} else if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		page, err := webui.Page(name)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
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
	{"overrides-desktop", "gameweek-one", "/app#overrides", desktop(1400)},
	{"overrides-mobile", "gameweek-one", "/app#overrides", mobile(1600)},
	{"brief-desktop", "gameweek-one", "/app#brief", desktop(1400)},

	// The states live data will not hand you on demand, and which are therefore the most
	// likely to be broken and the least likely to be looked at.
	{"edges-pitch-desktop", "edges", "/app#pitch", desktop(1700)},
	{"edges-pitch-mobile", "edges", "/app#pitch", mobile(2000)},
	{"edges-players-desktop", "edges", "/app#players", desktop(1200)},
	{"edges-overrides-desktop", "edges", "/app#overrides", desktop(1200)},
}

func goldenDir() string { return filepath.Join("testdata", "golden") }

func TestLayout(t *testing.T) {
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
