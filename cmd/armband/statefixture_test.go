package main

import (
	"encoding/json"
	"flag"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"armband/internal/viewmodel"
)

// updateGoldens gates the two generators in this package. They write fixtures that
// internal/webui's layout suite renders, so regenerating is a deliberate act with a diff to
// read, not something a plain test run does.
var updateGoldens = flag.Bool("update", false,
	"regenerate the state fixtures under internal/webui/testdata/state")

// TestWriteTheStateFixture regenerates internal/webui/testdata/state/gameweek-one.json
// from a real run over the committed capture.
//
// It is a generator, not an assertion, and it only writes under -update. Run it when
// viewmodel.State gains or loses a field:
//
//	go test ./cmd/armband/ -run TestWriteTheStateFixture -update
//
// # Why the fixture is generated once rather than read live
//
// The visual suite used to render whatever the optimiser produced. That made every golden
// a picture of one gameweek's answer: it tested GW1 and could not be widened, because the
// squad changes weekly and the prices change even when it does not. A model change churned
// nine images, which teaches a reviewer to run -update without looking.
//
// Generating the document once and committing it separates the two questions. The layout
// suite renders a fixed document and only moves when the layout moves. Whether the page
// draws what the model says is asserted separately, as a relation, by
// TestThePageHeadlineIsTheModelsNumber — which holds in any gameweek because it compares
// the page against the model rather than against a picture.
//
// The fixture is realistic rather than invented because it came from a real run. It is not
// re-derived, so it will drift from what the model would produce today; that is the point.
func TestWriteTheStateFixture(t *testing.T) {
	if !*updateGoldens {
		t.Skip("generator; run with -update to rewrite the state fixture")
	}

	s := fixtureServer(t)
	srv := httptest.NewServer(s)
	defer srv.Close()

	w := get(t, s, "/api/state")
	if w.Code != 200 {
		t.Fatalf("GET /api/state answered %d: %s", w.Code, w.Body.String())
	}

	var st viewmodel.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}

	// The market is trimmed, and that is what makes this a fixture rather than a dump.
	//
	// A live run carries 576 candidates and the document comes out at 762 KB — too large
	// to read, so nobody would, and a fixture nobody reads is a fixture nobody notices
	// going wrong. Sixty rows is past the 40 the mobile list shows before "Load the
	// rest", so pagination, the empty state and both layouts are all still exercised.
	//
	// Count and Clearing are trimmed with it. A fixture that says 576 while carrying 60
	// is internally inconsistent, and the page would print a number it cannot show.
	const marketRows = 60
	if len(st.Market.Rows) > marketRows {
		st.Market.Rows = st.Market.Rows[:marketRows]
	}
	st.Market.Count = len(st.Market.Rows)
	clearing := 0
	for _, r := range st.Market.Rows {
		if r.ClearsGate {
			clearing++
		}
	}
	st.Market.Clearing = clearing

	// Re-encoded with indentation rather than written raw. The fixture is a reviewed
	// artefact: a diff on it should show which field changed, not one very long line.
	doc := st
	pretty, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join("..", "..", "internal", "webui", "testdata", "state", "gameweek-one.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(pretty, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(pretty))
}
