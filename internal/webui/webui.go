// Package webui carries the client application: the pages, the design system and the
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
// assets/pages holds one document per page, and each document is reachable at exactly one
// URL. assets/static holds everything a document references, under /assets/. The two
// directories are not interchangeable: were the pages also served under /assets/, a
// document would have two URLs, and this project's most expensive recurring bug is one
// thing with two implementations. One page, one route — that now includes "/app", which is
// not a document at all but a redirect to "/", where the app document is served. See
// routeFor in cmd/armband/webroutes.go for the current routes.
//
// # What this package does not do
//
// It renders nothing and computes nothing. The client it carries is meant to compute
// nothing either — every model figure arrives as JSON from internal/viewmodel — and that is
// an intention with a guard on part of it, not a property of the whole.
//
// TestThePageHeadlineIsTheModelsNumber covers the projection and the cards. Three surfaces
// were recorded here as still computing. Two are now discharged and one is not, so the list
// is kept rather than trimmed — a debt paid silently reads as still owed, and one left
// standing among paid ones reads as paid.
//
//   - The formations rail, which picked and totalled its own eleven on a plain sum where
//     analysis.bestFormation maximises sum plus captain plus vice. DISCHARGED: renderShapes
//     is deleted, and deliberately not replaced by another client computation; a
//     server-computed replacement is filed as deferred rather than rebuilt under a new name.
//   - The player sheet's derivation panel, which spelled out an arithmetic the model does
//     not use. DISCHARGED: the panel is gone. What replaced it shows the model's own inputs
//     and a total the server sent.
//   - The two captain fallbacks, which apply their own rule. STILL OWED — a swap that moves
//     the captain out of the eleven reassigns the armband client-side, by position rather
//     than by anything the model would use.
//
// Read the last of those as work owed rather than as a claim already kept.
//
// Keeping the shell and the contract apart is what lets Wails bind to the view model
// directly, without an HTTP server in between.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
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
		// A directory is not an asset. http.FileServer answers one with an autoindex --
		// an HTML page listing the tree, served from a route whose every other response
		// is a stylesheet or a font. Nothing secret is in there, but a surface that
		// generates HTML nobody wrote is a surface, and the application never links to
		// one.
		// The prefix is already stripped here, so the tree's own root arrives as an
		// empty path rather than one ending in a separator. Both are directories.
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		// The pages set this and so should everything else. A font or a stylesheet
		// re-interpreted as HTML by a sniffing browser is the whole reason the header
		// exists, and the asset route was the one place it was missing.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	}))
}
