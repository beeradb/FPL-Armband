package webui

import (
	"bytes"
	"io/fs"
	"path"
	"sync"
	"time"
)

// This file caches the comment-stripped form of every embedded page and stylesheet/script,
// once, and serves it in place of the raw embedded bytes. See strip.go and stripjs.go for
// what "stripped" means; this file only owns WHEN it runs (once, lazily, on first use --
// not per request, and not at package init, so a test that never touches webui pays
// nothing for it) and WHERE the result is served from (Page and Static, below).
//
// Only .css, .js and .html are stripped. Everything else Static() serves -- the woff2
// subsets, the PNGs and SVGs, site.webmanifest -- passes through untouched: none of it
// carries comments in the sense this project's own CSS and JS do, and a JSON-ish format
// like the manifest has no comment syntax to strip in the first place.

var (
	stripOnceGuard sync.Once
	strippedPages  map[string][]byte
	strippedStatic *strippedFS
)

// stripOnce builds strippedPages and strippedStatic from the embedded assets tree. It
// panics on failure, matching Static()'s existing behaviour: everything it reads is a
// compile-time-constant path inside //go:embed assets, so an error here means the embed
// itself is broken, which is unreachable in a binary that built at all.
func stripOnce() {
	stripOnceGuard.Do(func() {
		entries, err := fs.ReadDir(assets, "assets/pages")
		if err != nil {
			panic("webui: embedded assets/pages missing: " + err.Error())
		}
		pages := make(map[string][]byte, len(entries))
		for _, e := range entries {
			if e.IsDir() || path.Ext(e.Name()) != ".html" {
				continue
			}
			b, err := assets.ReadFile("assets/pages/" + e.Name())
			if err != nil {
				panic("webui: reading assets/pages/" + e.Name() + ": " + err.Error())
			}
			name := e.Name()[:len(e.Name())-len(".html")]
			pages[name] = stripHTML(b)
		}
		strippedPages = pages

		sub, err := fs.Sub(assets, "assets/static")
		if err != nil {
			panic("webui: embedded assets/static missing: " + err.Error())
		}
		strippedStatic, err = newStrippedFS(sub)
		if err != nil {
			panic("webui: stripping assets/static: " + err.Error())
		}
	})
}

// strippedFS serves the same tree as its underlying fs.FS, except that .css and .js files
// answer with their comment-stripped bytes instead of the embedded ones. Everything else
// -- directory listings included, which is what lets fs.WalkDir and http.FileServer's own
// directory rejection keep working unmodified -- is delegated straight through.
type strippedFS struct {
	underlying fs.FS
	stripped   map[string][]byte // built once in newStrippedFS; read-only after that
}

func newStrippedFS(underlying fs.FS) (*strippedFS, error) {
	sf := &strippedFS{underlying: underlying, stripped: map[string][]byte{}}
	err := fs.WalkDir(underlying, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		var strip func([]byte) []byte
		switch path.Ext(p) {
		case ".css":
			strip = stripCSS
		case ".js":
			strip = stripJS
		default:
			return nil
		}
		b, err := fs.ReadFile(underlying, p)
		if err != nil {
			return err
		}
		sf.stripped[p] = strip(b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sf, nil
}

// Open implements fs.FS. A path this instance stripped is answered from the cached bytes
// via memFile; everything else -- binary assets and directories alike -- opens straight
// through the underlying embedded tree, unchanged from Static()'s behaviour before
// stripping existed.
func (sf *strippedFS) Open(name string) (fs.File, error) {
	b, ok := sf.stripped[name]
	if !ok {
		return sf.underlying.Open(name)
	}
	fi, err := fs.Stat(sf.underlying, name)
	if err != nil {
		return nil, err
	}
	return &memFile{
		Reader: bytes.NewReader(b),
		info:   memFileInfo{name: fi.Name(), size: int64(len(b)), mode: fi.Mode(), modTime: fi.ModTime()},
	}, nil
}

// memFile is a stripped file's fs.File. It wraps a *bytes.Reader over the cached,
// already-stripped bytes -- shared read-only across every request that opens this path
// concurrently, since bytes.Reader never mutates the slice it reads -- so http.FileServer
// gets a real io.ReadSeeker and can still answer Range requests correctly.
type memFile struct {
	*bytes.Reader
	info memFileInfo
}

func (m *memFile) Stat() (fs.FileInfo, error) { return m.info, nil }
func (m *memFile) Close() error               { return nil }

// memFileInfo is a stripped file's fs.FileInfo. Name, mode and mod time are copied from
// the underlying embedded file so caching semantics (embed.FS reports a zero ModTime,
// which is why StaticHandler's cache headers are no-cache rather than relying on
// Last-Modified) are unchanged; only Size reflects the stripped length.
type memFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return i.mode }
func (i memFileInfo) ModTime() time.Time { return i.modTime }
func (i memFileInfo) IsDir() bool        { return false }
func (i memFileInfo) Sys() any           { return nil }
