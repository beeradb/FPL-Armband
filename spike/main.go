// Command spike answers one question and then deletes itself: does the strict
// Content-Security-Policy that cmd/armband/webroutes.go ships survive being served
// through a Wails webview, on macOS's `wails://wails` custom scheme and on Windows's
// `http://wails.localhost`?
//
// It matters because the desktop build's whole design rests on handing the entire
// request chain to the existing *squadServer via assetserver.Options.Middleware. If
// `connect-src 'self'` does not resolve for a non-special scheme under WKWebView,
// fetch('/api/state') is blocked and the desktop build needs a relaxed policy -- a
// decision to take deliberately, not to discover after the packaging work.
//
// It is machine-checkable with nobody at a screen, which is the point: a GUI process
// on a CI runner has no console and cannot be screenshotted. The verdict leaves the
// webview as an HTTP request and becomes this process's exit status.
//
// This is a THROWAWAY in a nested module. It is not part of the armband binary, it is
// not built by `go build ./...` from the repository root, and the branch carrying it
// is deleted once it has answered.
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// assets is a COPY of internal/webui/assets, with exactly one edit: app.html loads
// /assets/spike.js alongside the real /assets/app.js. Copied rather than imported
// because this is a separate module, which is what keeps the root go.mod free of
// Wails until the real change lands.
//
//go:embed assets
var assets embed.FS

// csp is the policy under test, read from a file the workflow regenerates from
// cmd/armband/webroutes.go and diffs. A retyped policy would let this spike pass
// against a document nobody ships.
//
//go:embed csp.txt
var csp string

// verdict carries what the page managed to tell us. Every field is a string because
// it arrives as a query parameter and the point is to print it, not to compute on it.
type verdict struct {
	via    string
	fields map[string]string
}

var (
	mu      sync.Mutex
	seen    []string // every path the webview asked for, in order
	done    = make(chan verdict, 4)
	marker  string
	timeout = 90 * time.Second
)

func note(path string) {
	mu.Lock()
	defer mu.Unlock()
	seen = append(seen, path)
}

func handler() http.Handler {
	static, err := fs.Sub(assets, "assets/static")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/assets/", http.FileServer(http.FS(static)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		note(r.Method + " " + r.URL.Path)

		switch {
		case r.URL.Path == "/":
			// Served AT "/", never a redirect to /app: Wails' runtime injection
			// matches only "/" and "*/index.html", so a 303 would land the webview
			// on a document with no ipc.js.
			body, err := assets.ReadFile("assets/pages/app.html")
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Security-Policy", strings.TrimSpace(csp))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(body)

		case strings.HasPrefix(r.URL.Path, "/assets/"):
			w.Header().Set("X-Content-Type-Options", "nosniff")
			files.ServeHTTP(w, r)

		case r.URL.Path == "/api/state":
			// Stands in for the real view model. The probe only needs to prove the
			// request was ALLOWED and the body came back intact.
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(map[string]string{"marker": marker})

		case r.URL.Path == "/spike/ok":
			report(r.URL.Query())
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write([]byte{})

		default:
			http.NotFound(w, r)
		}
	})
}

func report(q url.Values) {
	f := map[string]string{}
	for k := range q {
		f[k] = q.Get(k)
	}
	select {
	case done <- verdict{via: f["via"], fields: f}:
	default:
	}
}

func main() {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fmt.Println("spike: cannot generate a marker:", err)
		os.Exit(1)
	}
	marker = hex.EncodeToString(b)

	fmt.Println("spike: policy under test:", strings.TrimSpace(csp))
	fmt.Println("spike: marker:", marker)

	h := handler()

	go func() {
		var v verdict
		select {
		case v = <-done:
		case <-time.After(timeout):
			finish(nil, "the page never reported within "+timeout.String())
			return
		}
		finish(&v, "")
	}()

	err := wails.Run(&options.App{
		Title:  "wails csp spike",
		Width:  1200,
		Height: 900,
		AssetServer: &assetserver.Options{
			// The whole bet, in one line: Middleware receives the default asset
			// handler and is free to ignore it, so this hands the entire request
			// chain to a handler we own -- which is how the real desktop build
			// will reuse *squadServer verbatim.
			Middleware: func(http.Handler) http.Handler { return h },
		},
	})
	if err != nil {
		finish(nil, "wails.Run: "+err.Error())
	}
	// wails.Run returning at all means the window was closed before the page
	// reported. That is a failure: nobody closed it.
	finish(nil, "the window closed before the page reported")
}

// finish prints everything worth knowing and sets the exit status. It is the only
// place that decides, and it never returns.
func finish(v *verdict, failure string) {
	mu.Lock()
	requested := append([]string(nil), seen...)
	mu.Unlock()

	fmt.Println()
	fmt.Println("=== what the webview requested ===")
	if len(requested) == 0 {
		fmt.Println("  (nothing -- the webview never issued a request)")
	}
	for _, p := range requested {
		fmt.Println(" ", p)
	}

	fmt.Println()
	fmt.Println("=== what the page reported ===")
	if v == nil {
		fmt.Println("  (nothing)")
		fmt.Println()
		fmt.Println("VERDICT: FAIL --", failure)
		os.Exit(1)
	}

	keys := make([]string, 0, len(v.fields))
	for k := range v.fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-8s %s\n", k, v.fields[k])
	}

	fmt.Println()
	switch {
	case v.fields["state"] != "ok":
		fmt.Println("VERDICT: FAIL -- the page ran but fetch('/api/state') was refused.")
		fmt.Println("  connect-src 'self' does not resolve on this scheme. The desktop")
		fmt.Println("  build needs a relaxed policy, decided deliberately.")
		os.Exit(1)
	case v.fields["marker"] != marker:
		fmt.Printf("VERDICT: FAIL -- marker mismatch: got %q, want %q\n", v.fields["marker"], marker)
		os.Exit(1)
	default:
		fmt.Println("VERDICT: PASS -- an external script from 'self' executed, and")
		fmt.Println("  fetch('/api/state') round-tripped the marker, under the shipped policy.")
		os.Exit(0)
	}
}
