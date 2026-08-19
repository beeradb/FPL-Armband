// Package webui carries the client application: the two pages, the design system and the
// self-hosted fonts, compiled into the binary.
//
// # Why the assets are embedded rather than read from disk
//
// The long-term targets are a Wails desktop build and a website, with `armband serve` as
// the local host of the same thing. A Wails build has no directory to read from and no
// network to fetch from, so anything the page needs at first paint has to be inside the
// binary. Embedding also means `armband serve` cannot half-work: a missing stylesheet is
// a build failure here, not a blank page in front of a reader.
//
// # The split between pages and static
//
// assets/pages holds the two documents, and each is reachable at exactly one URL — "/"
// and "/app". assets/static holds everything a document references, under /assets/.
// The two directories are not interchangeable: were the pages also served under /assets/,
// the app would have two URLs, and this project's most expensive recurring bug is one
// thing with two implementations. One page, one route.
//
// # What this package does not do
//
// It renders nothing and computes nothing. Every number the app displays arrives as JSON
// from internal/viewmodel; this package is the shell that fetches it. Keeping the two
// apart is what lets Wails bind to the view model directly, without an HTTP server in
// between.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

// assets is the whole client application. The //go:embed directive deliberately names the
// directory rather than a glob: a glob silently skips files beginning with "." or "_",
// and a font subset that fails to ship is invisible until a reader sees the fallback.
//
//go:embed assets
var assets embed.FS

// Static is the tree served under /assets/ — the stylesheet, the generated font CSS, the
// woff2 subsets and the mark.
func Static() fs.FS {
	sub, err := fs.Sub(assets, "assets/static")
	if err != nil {
		// Unreachable: the path is a compile-time constant and embed guarantees it
		// exists. Panicking beats returning an error no caller could act on.
		panic("webui: embedded assets/static missing: " + err.Error())
	}
	return sub
}

// Page returns one document by name — "landing" or "app". It returns an error rather than
// panicking on an unknown name so a route typo surfaces as a 500 with a message, not a
// dead server.
func Page(name string) ([]byte, error) {
	return assets.ReadFile("assets/pages/" + name + ".html")
}

// StaticHandler serves Static() under the given prefix.
//
// The cache headers are deliberately weak: these assets change whenever the binary does,
// and the binary is rebuilt constantly during development. A long max-age would mean a
// reader staring at a stale stylesheet and reaching for a hard refresh, which is exactly
// the class of confusion this project's no-cache page rule already exists to avoid. When
// the website ships, hashed filenames — not a longer max-age — are the answer.
func StaticHandler(prefix string) http.Handler {
	fileServer := http.FileServer(http.FS(Static()))
	return http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	}))
}
