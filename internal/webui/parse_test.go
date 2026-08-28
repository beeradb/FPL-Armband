package webui_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/webui"
)

// TestTheServedStylesheetParsesTheSameRules is the "same rules parse" half of the
// strip-comments work's verification requirement, run through an actual browser rather
// than asserted from a Go-side notion of CSS grammar this project does not otherwise
// need. strip_test.go's TestStripCSSOnlyRemovesComments proves stripCSS deletes nothing
// but comment-shaped spans from armband.css; this test proves that deletion changes
// nothing a browser's own CSSOM can see: the same number of rules, with the same
// selector and declaration text, for the raw file and for what Static() now serves.
//
// armband.css was chosen because it is the file this work measured (143,129 bytes, 46.1%
// comment) and the one every page depends on for first paint.
func TestTheServedStylesheetParsesTheSameRules(t *testing.T) {
	browser := browsertest.Find(t)

	raw, err := os.ReadFile(filepath.Join("assets", "static", "armband.css"))
	if err != nil {
		t.Fatalf("reading the source stylesheet: %v", err)
	}
	stripped, err := fs.ReadFile(webui.Static(), "armband.css")
	if err != nil {
		t.Fatalf("reading armband.css from Static(): %v", err)
	}
	if len(stripped) >= len(raw) {
		t.Fatalf("Static() served %d bytes for armband.css, no smaller than the %d-byte "+
			"source -- stripping did not run", len(stripped), len(raw))
	}

	rawResult := parseCSSInBrowser(t, browser, raw)
	if rawResult.Error != "" {
		t.Fatalf("browser could not parse the SOURCE stylesheet (a bug in the fixture, not "+
			"the strip): %s", rawResult.Error)
	}
	if rawResult.Count == 0 {
		t.Fatal("the source stylesheet parsed to zero rules; the probe did not work")
	}

	strippedResult := parseCSSInBrowser(t, browser, stripped)
	if strippedResult.Error != "" {
		t.Fatalf("browser could not parse the STRIPPED stylesheet: %s", strippedResult.Error)
	}

	if strippedResult.Count != rawResult.Count {
		t.Errorf("rule count changed: source has %d, stripped has %d", rawResult.Count, strippedResult.Count)
	}
	if !reflect.DeepEqual(rawResult.Rules, strippedResult.Rules) {
		n := len(rawResult.Rules)
		if len(strippedResult.Rules) < n {
			n = len(strippedResult.Rules)
		}
		for i := 0; i < n; i++ {
			if rawResult.Rules[i] != strippedResult.Rules[i] {
				t.Fatalf("rule %d differs after stripping:\n source:   %s\n stripped: %s",
					i, rawResult.Rules[i], strippedResult.Rules[i])
			}
		}
		t.Fatalf("rule lists differ in length: source %d, stripped %d", len(rawResult.Rules), len(strippedResult.Rules))
	}
}

// cssParseResult is what cssParseProbe publishes.
type cssParseResult struct {
	Count int      `json:"count"`
	Rules []string `json:"rules"`
	Error string   `json:"error"`
}

// cssParseProbe loads exactly one stylesheet, served at /sheet.css by parseCSSInBrowser,
// and publishes the CSSOM rules the browser's own parser built from it -- the question
// this test asks, answered by the same engine a reader's browser uses, not by a
// Go-side re-implementation of CSS grammar.
const cssParseProbe = `<!doctype html>
<meta charset="utf-8">
<title>css parse probe</title>
<link rel="stylesheet" href="/sheet.css">
<script>
window.addEventListener('load', function () {
  var payload;
  try {
    var sheet = document.styleSheets[0];
    var rules = Array.prototype.map.call(sheet.cssRules, function (r) { return r.cssText; });
    payload = { count: rules.length, rules: rules };
  } catch (e) {
    payload = { error: String((e && e.stack) || e) };
  }
  var el = document.createElement('script');
  el.type = 'application/json';
  el.id = ` + "`" + browsertest.ProbeID + "`" + `;
  el.textContent = btoa(unescape(encodeURIComponent(JSON.stringify(payload))));
  document.body.appendChild(el);
});
</script>
`

// parseCSSInBrowser serves css at /sheet.css and cssParseProbe at /, and returns what the
// probe published.
func parseCSSInBrowser(t *testing.T, browser string, css []byte) cssParseResult {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(cssParseProbe))
	})
	mux.HandleFunc("/sheet.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write(css)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out := browsertest.Probe(t, browser, srv.URL+"/", browsertest.Viewport{Width: 800, Height: 600})
	var res cssParseResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("the probe published something that is not its own payload: %v\n%s", err, out)
	}
	return res
}
