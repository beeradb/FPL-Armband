package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/webui"
)

// TestTheChipMenuHeaderReadsTheViewedWeeksWindow — CHIPWIN is the live week's
// window. The menu must read the VIEWED week's chip_window, copied onto GWS in
// hydrate, because EndsGW and Remaining flip at the GW19/38 reset and Remaining
// drops a chip planned for a week before the one being asked about.
func TestTheChipMenuHeaderReadsTheViewedWeeksWindow(t *testing.T) {
	browser := browsertest.Find(t)

	harness := `<!doctype html><meta charset="utf-8"><title>chip window: viewed week</title>
<h1>Chip-window harness</h1>
<p>Not part of the application. Loads the shipped app.js so the menu header is
the one that ships, not a copy.</p>
<div id="out">no html: chipMenuHtml never ran</div>
<script src="/assets/app.js"></script>
<script src="/probe.js"></script>`
	probeJS := `window.addEventListener('load', function(){
  S.gw = 20;
  CHIPWIN = {endsGw:19, remaining:4, size:4};
  GWS = [];
  var week = {chip:null, playable:[], chipWin:{endsGw:38, remaining:2, size:4}};
  document.getElementById('out').textContent = 'HTML:' + chipMenuHtml(week) + ':END';
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
	i := strings.Index(dom, "HTML:")
	j := strings.Index(dom, ":END")
	if i < 0 || j < i {
		t.Fatalf("the harness produced no menu HTML, so chipMenuHtml never ran. DOM was %d bytes", len(dom))
	}
	html := dom[i+len("HTML:") : j]
	if strings.Contains(html, "ends after GW19") {
		t.Errorf("the menu header used the live week's window (GW19) while the viewed week is GW20, whose window ends at 38")
	}
	if strings.Contains(html, "4 of 4 left") {
		t.Errorf("the menu header used the live week's remaining count (4) while the viewed week has 2 left")
	}
	if !strings.Contains(html, "ends after GW38") {
		t.Errorf("the menu header did not show the viewed week's window end. html=%s", html)
	}
	if !strings.Contains(html, "2 of 4 left") {
		t.Errorf("the menu header did not show the viewed week's remaining count. html=%s", html)
	}
}
