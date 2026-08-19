package webui_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/present"
	"armband/internal/webui"
)

// TestTheGateIsDecidedTheSameWayInBothLanguages runs one table through both copies.
//
// # Why a copy exists at all
//
// This client computes no model quantity, with one exception. The market's rows carry
// `clears_gate` from the server, but the replacement picker's delta is against the player
// being REPLACED and the server sends deltas against the weakest starter — so for the picker
// there is no server answer to mirror, and the client decides.
//
// # Why a test rather than a comment
//
// The two implementations had already diverged before anyone noticed. Go compared raw floats
// and the client compared at two decimal places, so a delta of 0.3999 against a 0.40 gate
// printed "+0.40" and got a below-the-gate badge from the server while the picker called the
// same move worth making. The rounding is right — a row contradicting itself on one line is
// worse than either answer — and it now lives in Go, with this holding the copy to it.
//
// This is the repo's standing remedy for one quantity with two implementations, applied
// where deleting the second copy is not available: pin them together, and run the pin.
func TestTheGateIsDecidedTheSameWayInBothLanguages(t *testing.T) {
	browser := browsertest.Find(t)

	// The cases that separate the two rules. Exact representation matters here, so these
	// are the values a float64 actually holds rather than the ones a reader would type.
	type probe struct {
		Delta float64 `json:"d"`
		Gate  float64 `json:"g"`
	}
	probes := []probe{
		{0.40, 0.40},   // equal at the printed precision, and at full precision
		{0.3999, 0.40}, // prints "+0.40": the divergence that started this
		{0.39, 0.40},   // prints "+0.39": below by any rule
		{0.395, 0.40},  // the half-way case, where a rounding convention shows
		{0.404, 0.40},
		{0.4049, 0.405},
		{1.0, 0.40},
		{-0.5, 0.40},
		{0.0, 0.40},
		{0.5, 0.0},  // an unset gate clears NOTHING, rather than everything
		{-0.5, 0.0}, // and the same on the other side of zero
		{0.5, -1.0}, // a negative gate is not a gate
		{0.004, 0.0049},
		{0.0051, 0.0049},

		// The cases that separate math.Round(v*100)/100 from toFixed(2). Every one of
		// these is a delta sitting one tie below a plausible gate, and the table above
		// contains none of them — which is why this test passed while the two rules
		// disagreed. The shipped gate is 0.40, where they happen to agree; these are the
		// gates a sweep might move it to.
		{0.295, 0.30},
		{0.495, 0.50},
		{0.245, 0.25},
		{0.345, 0.35},
		{0.595, 0.60},
		{0.695, 0.70},
		{0.745, 0.75},
		{0.995, 1.00},
		{1.495, 1.50},
		{1.995, 2.00},
		{0.015, 0.02},
		{0.045, 0.05},
		{0.105, 0.11},
		{0.155, 0.16},
	}

	// The harness: the real app.js, a STATE carrying the gate, and one call per probe. The
	// answers are written into the document, which is what DumpDOM can read back.
	//
	// app.js runs its own start-up against a page with none of its elements, and may throw
	// doing so. That is fine and is why the probes run from a listener rather than inline:
	// function declarations are hoisted, so clearsGate exists whatever the rest of the file
	// did.
	encoded, err := json.Marshal(probes)
	if err != nil {
		t.Fatal(err)
	}
	// The page carries its own explanation, partly because anyone who lands on it while
	// debugging deserves one, and partly because browsertest.DumpDOM treats a very short
	// document as a page that failed to render — a floor that is right for the real pages
	// and would otherwise fail this one for being small.
	harness := `<!doctype html><meta charset="utf-8"><title>transfer gate: Go against the client</title>
<h1>Transfer gate equivalence harness</h1>
<p>This page is not part of the application. It exists so a test can run the client's copy
of the transfer-gate rule over a table of deltas and gates, and compare every answer against
present.ClearsGate in Go. It loads the real /assets/app.js rather than a copy of the
function, so what is tested is what ships.</p>
<p>The answers are the digit string below, one character per probe, in the order the test
sent them: 1 clears the gate, 0 does not.</p>
<div id="out">no answers: the client's rule never ran</div>
<script src="/assets/app.js"></script>
<script src="/probe.js"></script>`
	probeJS := `window.addEventListener('load', function(){
  var probes = ` + string(encoded) + `;
  var out = [];
  for (var i = 0; i < probes.length; i++) {
    STATE = {market: {gate: probes[i].g}};
    out.push(clearsGate(probes[i].d) ? '1' : '0');
  }
  document.getElementById('out').textContent = 'GATE:' + out.join('') + ':END';
});`

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.StaticHandler("/assets/"))
	mux.HandleFunc("/probe.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(probeJS))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(harness))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dom := browsertest.DumpDOM(t, browser, srv.URL+"/")
	i := strings.Index(dom, "GATE:")
	j := strings.Index(dom, ":END")
	if i < 0 || j < i {
		t.Fatalf("the harness produced no answers, so the client's rule was never run. "+
			"A silent pass here is worse than a failure. DOM was %d bytes", len(dom))
	}
	got := dom[i+len("GATE:") : j]
	if len(got) != len(probes) {
		t.Fatalf("the client answered %d of %d probes: %q", len(got), len(probes), got)
	}

	var want strings.Builder
	for _, p := range probes {
		if present.ClearsGate(p.Delta, p.Gate) {
			want.WriteByte('1')
		} else {
			want.WriteByte('0')
		}
	}
	if got != want.String() {
		var diff []string
		for k, p := range probes {
			if got[k] != want.String()[k] {
				diff = append(diff, fmt.Sprintf("delta %g against gate %g: Go says %c, the "+
					"client says %c", p.Delta, p.Gate, want.String()[k], got[k]))
			}
		}
		t.Errorf("the transfer gate is decided differently in Go and in the client:\n  %s\n"+
			"The rule lives in present.ClearsGate. Change it there, then change the mirror "+
			"in app.js to match — never the other way round.", strings.Join(diff, "\n  "))
	}
}
