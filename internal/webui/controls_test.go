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

// TestNewsIgnoreReachesTheServer was here and is RETIRED, because the control it drove no
// longer exists anywhere in the product.
//
// It clicked Ignore on a config-sourced row in the News tab and asserted a PUT carrying a
// dismissal reached the server -- guarding the defect where a control repaints the DOM and
// sends nothing. That property still matters and is still covered for every surviving
// control by TestEveryControlOnACardReachesTheServer above, which drives the same
// save() -> PUT /api/session path.
//
// What went: the News tab now carries no per-row control at all, and Your instructions
// stopped listing config entries, which took ignoreOverride()'s last caller with it. Framed
// as news rather than as a setting, "don't count this" is not an offer the reader is being
// made in v1.
//
// ⚠️ Do not restore this test against a different button to keep the coverage. The dismissal
// path it asserted is unreachable from the client, so a test pointed at any other control
// would be asserting the shared save() plumbing under a misleading name -- and that plumbing
// already has its own test. If suppression returns, this test returns with it, pointed at
// the real control.

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

// TestTheMarketRowsLockAndLeaveOutReachTheServer verifies that the redesigned market row
// controls — Lock in and Leave out — reach the server and are not lost on client-side mutation.
//
// The Lock in control uses an arm-then-confirm pattern (first click arms, second click confirms),
// while Leave out is a single click. Both must send PUT requests carrying their respective fields
// (lock and excl).
func TestTheMarketRowsLockAndLeaveOutReachTheServer(t *testing.T) {
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
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/probe.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(probeMarketRowClicks))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		page, err := webui.Page("app")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		page = append(page, []byte(`<script src="/probe.js"></script>`)...)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dom := browsertest.DumpDOM(t, browser, srv.URL+"/app#players")
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
		t.Fatalf("clicking market row controls sent NOTHING to the server. Probe: %s",
			report)
	}

	// Verify both actions were wired: lock (arm-then-confirm) and block (leave out).
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

// TestLeftOutUndoReachesTheServer pins the Left-out panel's Undo button.
//
// # The defect
//
// renderLeftOut() built its Undo button from codeToId(code), which looks the player up in
// P.concat(POOL) to get an element id for toggleCorrection(id, kind). But a market player
// who has just been excluded is removed from BOTH lists -- watchlistFor (cmd/armband/page.go)
// strips excluded players out of the market rows entirely, and he was never in the squad to
// begin with. So codeToId returned 0, toggleCorrection got id 0, and it failed with a false
// "That player has no code, so we can't save that" toast -- Undo never reached the server for
// exactly the case it exists to handle.
//
// The fix routes the panel's click handler through toggleCorrectionByCode(code, kind)
// directly, since the panel already holds the permanent code from the server's Override
// record and never needed the id at all.
//
// # Why this is a browser test
//
// The break is entirely client-side -- a lookup against the wrong in-memory lists -- so a Go
// test against the endpoint would pass throughout, because a broken client never sends the
// request. This drives a real click in a real document, the same way
// TestTheMarketRowsLockAndLeaveOutReachTheServer does for Lock in and Leave out.
//
// The mock /api/session handler here is stateful (unlike the other tests in this file, which
// always echo the same fixture back): it has to actually remove the excluded player from the
// market rows and add him to market.excluded with session:true, or the Left-out panel would
// never render an Undo button for a player still sitting in the market table -- which is
// exactly the situation the real server produces and the one this bug needs to reproduce.
func TestLeftOutUndoReachesTheServer(t *testing.T) {
	browser := browsertest.Find(t)

	base, err := os.ReadFile(filepath.Join("testdata", "state", "gameweek-one.json"))
	if err != nil {
		t.Fatalf("reading the state fixture: %v", err)
	}

	// Haaland, the fixture's first market row: an unowned candidate, not in the squad, so
	// excluding him removes him from POOL without touching P -- the exact split the real
	// watchlistFor produces for a session exclusion.
	const haalandID = 411
	const haalandCode = 223094

	var mu sync.Mutex
	var puts []map[string]any
	excluded := false

	buildDoc := func() []byte {
		var doc map[string]any
		if err := json.Unmarshal(base, &doc); err != nil {
			t.Fatalf("re-parsing the fixture: %v", err)
		}
		if !excluded {
			out, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshalling the fixture: %v", err)
			}
			return out
		}
		market := doc["market"].(map[string]any)
		rows := market["rows"].([]any)
		kept := rows[:0]
		for _, rv := range rows {
			row := rv.(map[string]any)
			player := row["player"].(map[string]any)
			if code, _ := player["code"].(float64); code == haalandCode {
				continue
			}
			kept = append(kept, rv)
		}
		market["rows"] = kept
		market["excluded"] = append(market["excluded"].([]any), map[string]any{
			"kind": "exclude", "code": haalandCode, "session": true,
			"label": "EXCL", "reason": "left out this session",
			"player": "Haaland", "club": "MCI", "pos": "FWD",
		})
		sess := doc["session"].(map[string]any)
		sess["blocked"] = []any{float64(haalandCode)}
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshalling the mutated fixture: %v", err)
		}
		return out
	}

	mux := http.NewServeMux()
	mux.Handle("/assets/", webui.StaticHandler("/assets/"))
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		doc := buildDoc()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(doc)
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		puts = append(puts, got)
		// The first PUT (Leave out) carries the code in `excl`; the second (Undo) is
		// supposed to carry it in `dis` instead, clearing the exclusion. Either one
		// landing here at all is the thing the old code never managed for this case.
		for _, c := range asInts(got["excl"]) {
			if c == haalandCode {
				excluded = true
			}
		}
		for _, c := range asInts(got["dis"]) {
			if c == haalandCode {
				excluded = false
			}
		}
		doc := buildDoc()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(doc)
	})
	mux.HandleFunc("/probe.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(probeLeftOutUndoClicks))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		page, err := webui.Page("app")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		page = append(page, []byte(`<script src="/probe.js"></script>`)...)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dom := browsertest.DumpDOM(t, browser, srv.URL+"/app#players")
	report := between(dom, "PROBE:", ":END")
	if report == "" {
		t.Fatalf("the probe never reported, so nothing was clicked and this test asserts "+
			"nothing. DOM was %d bytes", len(dom))
	}
	if !strings.HasPrefix(report, "ok ") {
		t.Fatalf("the probe could not run: %s. This is the false-error path if it names the "+
			"toast: \"That player has no code, so we can't save that.\"", report)
	}

	mu.Lock()
	defer mu.Unlock()

	var sawExclude, sawUndo bool
	for _, p := range puts {
		if inInts(asInts(p["excl"]), haalandID) || inInts(asInts(p["excl"]), haalandCode) {
			sawExclude = true
		}
		if inInts(asInts(p["dis"]), haalandCode) {
			sawUndo = true
		}
	}
	if !sawExclude {
		t.Fatalf("clicking Leave out never reached the server, over %d requests: %v",
			len(puts), puts)
	}
	if !sawUndo {
		t.Errorf("clicking Undo in the Left-out panel never sent a dismissal for player "+
			"code %d, over %d requests: %v -- this is the bug: codeToId(code) cannot find a "+
			"player who has just been excluded from the market, so the click either does "+
			"nothing or fires the false \"no code\" toast instead of a real PUT",
			haalandCode, len(puts), puts)
	}
}

func inInts(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// probeLeftOutUndoClicks drives Leave out on Haaland's market row, waits for the Left-out
// panel's Undo button to appear, and clicks it.
const probeLeftOutUndoClicks = `(function(){
  function report(msg){
    var el=document.createElement('div');
    el.id='probe';
    el.textContent='PROBE:'+msg+':END';
    document.body.appendChild(el);
  }
  var tries=0;
  function clickBlock(){
    var btn=document.querySelector('#ptbody tr[data-id="411"] [data-act="block"]');
    if(!btn){
      if(++tries>100){ report('no row for player 411 after '+tries+' tries'); return; }
      setTimeout(clickBlock,50); return;
    }
    btn.click();
    setTimeout(waitForUndo, 300);
  }
  var tries2=0;
  function waitForUndo(){
    var undo=document.querySelector('#leftout [data-excl="undo"]');
    if(!undo){
      if(++tries2>100){ report('no undo button in the left-out panel after '+tries2+' tries'); return; }
      setTimeout(waitForUndo,50); return;
    }
    undo.click();
    setTimeout(function(){ report('ok clicked leave out and undo'); }, 300);
  }
  window.addEventListener('load', clickBlock);
})();`

// probeMarketRowClicks drives the Lock in and Leave out controls on a market row and reports
// what it managed to do. It waits for the players panel to load before clicking.
const probeMarketRowClicks = `(function(){
  function report(msg){
    var el=document.createElement('div');
    el.id='probe';
    el.textContent='PROBE:'+msg+':END';
    document.body.appendChild(el);
  }
  var tries=0;
  function go(){
    // Find a market row with both controls.
    var lockBtn=document.querySelector('#ptbody [data-act="lock"]');
    var blockBtn=document.querySelector('#ptbody [data-act="block"]');
    if(!lockBtn||!blockBtn){
      if(++tries>100){ report('no market rows after '+tries+' tries'); return; }
      setTimeout(go,50); return;
    }
    // Click lock in (arms first, confirms second).
    lockBtn.click();
    // Wait a tick for the arm strip to render.
    setTimeout(function(){
      // Find and click the confirmation button (data-act="armgo" on the armed row).
      var confirmBtn=document.querySelector('[data-armgo]');
      if(confirmBtn){ confirmBtn.click(); }
      // Then click leave out.
      setTimeout(function(){ blockBtn.click(); report('ok clicked lock and block'); }, 300);
    }, 300);
  }
  window.addEventListener('load', go);
})();`
