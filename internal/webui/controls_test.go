package webui_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"armband/internal/browsertest"
	"armband/internal/webui"
)

// TestEveryControlOnACardReachesTheServer.
//
// # The defect
//
// The lock and block icons on a squad card mutated a JavaScript Set and called renderAll().
// The badge lit up, the model never heard about it, and a reload dropped it — the same
// defect as the remove button that started this branch, surviving on the control a reader
// reaches for first, because it is the one on the pitch. The player SHEET's versions of the
// same two actions saved correctly, so the behaviour depended on which surface you used.
//
// # Why this is a browser test and not a unit test
//
// The thing that was wrong is a click handler. A Go test can assert the endpoint stores what
// it is sent — several do — and would have passed throughout, because nothing was ever sent.
// The only witness is a real click in a real document.
//
// It asserts a REQUEST, not a repaint. A control that repaints and sends nothing is exactly
// what is being pinned, so a test satisfied by the badge would pass on the broken code.
func TestEveryControlOnACardReachesTheServer(t *testing.T) {
	browser := browsertest.Find(t)

	body, err := os.ReadFile(filepath.Join("testdata", "state", "gameweek-one.json"))
	if err != nil {
		t.Fatalf("reading the state fixture: %v", err)
	}

	var mu sync.Mutex
	var puts []map[string]any

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.StaticHandler("/assets/"))
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		puts = append(puts, got)
		mu.Unlock()
		// Answered with the unchanged document. The point is that the request happened;
		// what the server would have decided is cmd/armband's business and is tested there.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/probe.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(probeClicks))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		page, err := webui.Page("app")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		// The shipped page, plus one same-origin script that does the clicking. The
		// application itself is unmodified, which is the point: a test against a rewritten
		// copy of the client would pass while the client stayed broken.
		page = append(page, []byte(`<script src="/probe.js"></script>`)...)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dom := browsertest.DumpDOM(t, browser, srv.URL+"/app")
	report := between(dom, "PROBE:", ":END")
	if report == "" {
		t.Fatalf("the probe never reported, so nothing was clicked and this test asserts "+
			"nothing. DOM was %d bytes", len(dom))
	}
	if !strings.HasPrefix(report, "ok ") {
		t.Fatalf("the probe could not run: %s", report)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(puts) == 0 {
		t.Fatalf("clicking the lock and block icons on a card sent NOTHING to the server. "+
			"The badge changes and the model does not, and a reload drops it. Probe: %s",
			report)
	}

	// Both actions, not just whichever one happened to be wired.
	var sawLock, sawBlock bool
	for _, p := range puts {
		if len(asInts(p["lock"])) > 0 {
			sawLock = true
		}
		if len(asInts(p["excl"])) > 0 {
			sawBlock = true
		}
	}
	if !sawLock {
		t.Errorf("no request carried a lock, over %d requests: %v", len(puts), puts)
	}
	if !sawBlock {
		t.Errorf("no request carried a block, over %d requests: %v", len(puts), puts)
	}
}

// probeClicks drives the two icons and reports what it managed to do.
//
// It waits on the application's own fetch of /api/state rather than a timer: the pitch does
// not exist until that lands, and a timer would make this test flaky on a loaded machine and
// — worse — silently vacuous if it ever fired early.
const probeClicks = `(function(){
  function report(msg){
    var el=document.createElement('div');
    el.id='probe';
    el.textContent='PROBE:'+msg+':END';
    document.body.appendChild(el);
  }
  var tries=0;
  function go(){
    var lock=document.querySelector('.card [data-act="lock"]');
    var block=document.querySelector('.card [data-act="block"]');
    if(!lock||!block){
      if(++tries>100){ report('no card icons after '+tries+' tries'); return; }
      setTimeout(go,50); return;
    }
    lock.click();
    // The saves are chained, so the second click does not need to wait for the first.
    var other=document.querySelector('.card [data-act="block"]');
    if(other) other.click();
    setTimeout(function(){ report('ok clicked lock and block'); }, 600);
  }
  window.addEventListener('load', go);
})();`

func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func asInts(v any) []int {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(raw))
	for _, x := range raw {
		if f, ok := x.(float64); ok {
			out = append(out, int(f))
		}
	}
	return out
}
