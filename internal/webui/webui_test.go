package webui

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// pages are the two documents, by the name Page takes.
var pages = []string{"landing", "app"}

// TestEveryPageIsEmbeddedAndNonEmpty is the shallowest possible check, and it exists
// because the failure it catches is silent. A //go:embed directive that matches nothing
// compiles; the error arrives as a blank browser window at the moment someone is trying
// to look at something else.
func TestEveryPageIsEmbeddedAndNonEmpty(t *testing.T) {
	for _, name := range pages {
		b, err := Page(name)
		if err != nil {
			t.Errorf("Page(%q): %v", name, err)
			continue
		}
		if len(b) < 1024 {
			t.Errorf("Page(%q) is %d bytes — that is not a page", name, len(b))
		}
		if !strings.Contains(string(b), "<!doctype html>") {
			t.Errorf("Page(%q) does not start like an HTML document", name)
		}
	}
}

// TestPageRejectsAnUnknownName pins that a route typo is an error a handler can answer,
// not a panic that takes the server with it.
func TestPageRejectsAnUnknownName(t *testing.T) {
	if _, err := Page("../static/armband"); err == nil {
		t.Error("Page accepted a traversing name; it must not reach outside assets/pages")
	}
	if _, err := Page("nosuchpage"); err == nil {
		t.Error("Page accepted an unknown name")
	}
}

// externalRef matches a reference to another host. This is the same guard
// internal/present carries for the exported page (TestTheThreeViewPageIsStillSelfContained),
// restated here because the reason is stronger for this app, not weaker: it ships into a
// Wails webview with no network at all, so an external reference is not a slow first
// paint, it is a missing asset.
var externalRef = regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)

// cssComment and dataURI are stripped before scanning, and neither exclusion is a
// loosening — they are what makes the scan mean what it says.
//
// A comment is not a reference: armband.css carries a note explaining why the fonts are
// deliberately NOT @import'ed, and a scan that reads its own explanation as a violation
// teaches everyone to ignore it. A data: URI is not a fetch either: the lock and block
// glyphs are inline SVG, and every inline SVG declares the xmlns
// http://www.w3.org/2000/svg, which is an XML namespace name and not an address anything
// resolves. Matching those would make the test fire on correct code, which is how a guard
// gets deleted rather than fixed.
var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	dataURI    = regexp.MustCompile(`(?s)url\(\s*["']?data:[^)]*\)`)
	htmlnote   = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func scannable(body []byte) string {
	s := string(body)
	s = cssComment.ReplaceAllString(s, " ")
	s = htmlnote.ReplaceAllString(s, " ")
	s = dataURI.ReplaceAllString(s, "url()")
	return s
}

func TestTheEmbeddedAppReachesNoExternalHost(t *testing.T) {
	static := Static()
	var checked int

	check := func(name string, raw []byte) {
		checked++
		body := scannable(raw)
		if m := externalRef.FindString(body); m != "" {
			t.Errorf("%s references an external host (%q). The app ships into an embedded "+
				"webview with no network; every asset must be served from the binary.", name, m)
		}
		if strings.Contains(body, "@import") {
			t.Errorf("%s uses @import, which chains a second round trip before first paint. "+
				"HANDOFF.md section 6 rules it out; use a <link>.", name)
		}
	}

	for _, name := range pages {
		b, err := Page(name)
		if err != nil {
			t.Fatalf("Page(%q): %v", name, err)
		}
		check("pages/"+name+".html", b)
	}
	err := fs.WalkDir(static, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".css") {
			return err
		}
		b, readErr := fs.ReadFile(static, p)
		if readErr != nil {
			return readErr
		}
		check("static/"+p, b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 4 {
		t.Fatalf("only checked %d files; the walk is not reaching the assets", checked)
	}
}

// assetRef finds the local files a document or stylesheet depends on: href/src attributes
// and CSS url() targets.
var assetRef = regexp.MustCompile(`(?:href|src)="([^"]+)"|url\(["']?([^"')]+)["']?\)`)

// TestEveryReferencedAssetResolves walks the references out of the pages and the
// stylesheets and resolves each one against the embedded tree.
//
// This is the test that would have caught the vendoring mistake it was written after: the
// pages were rewired from "armband.css" to "/assets/armband.css" and the file itself was
// moved in a separate step, and nothing in between would have failed.
func TestEveryReferencedAssetResolves(t *testing.T) {
	static := Static()

	resolve := func(from, ref string) {
		switch {
		case ref == "" || strings.HasPrefix(ref, "#"),
			strings.HasPrefix(ref, "data:"),
			strings.HasPrefix(ref, "mailto:"):
			return
		case ref == "/" || ref == "/app":
			// The two page routes. Served by cmd/armband/serve.go, not from static.
			return
		}
		if !strings.HasPrefix(ref, "/assets/") {
			// A relative reference inside a stylesheet, e.g. fonts/inter-latin.woff2.
			ref = path.Join(path.Dir(from), ref)
		} else {
			ref = strings.TrimPrefix(ref, "/assets/")
		}
		if _, err := fs.Stat(static, ref); err != nil {
			t.Errorf("%s references %q, which is not in the embedded tree", from, ref)
		}
	}

	for _, name := range pages {
		b, err := Page(name)
		if err != nil {
			t.Fatalf("Page(%q): %v", name, err)
		}
		for _, m := range assetRef.FindAllStringSubmatch(string(b), -1) {
			resolve("pages/"+name+".html", m[1]+m[2])
		}
	}

	err := fs.WalkDir(static, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".css") {
			return err
		}
		b, readErr := fs.ReadFile(static, p)
		if readErr != nil {
			return readErr
		}
		for _, m := range assetRef.FindAllStringSubmatch(string(b), -1) {
			resolve(p, m[1]+m[2])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestTheFontsCoverTheNamesFootballersActuallyHave pins the latin-ext subset.
//
// This project has already paid once for treating player names as ASCII — the squad
// printer padded by bytes, so every name with an accent ran the column ragged, and those
// are exactly the names a good squad is full of. Dropping latin-ext to save 118K would be
// the same mistake wearing different clothes: Kadıoğlu, Ødegaard, Guéhi and Martínez
// would render in the fallback face while everyone around them rendered in Inter.
func TestTheFontsCoverTheNamesFootballersActuallyHave(t *testing.T) {
	static := Static()
	css, err := fs.ReadFile(static, "fonts.css")
	if err != nil {
		t.Fatalf("reading fonts.css: %v", err)
	}
	for _, family := range []string{"Inter", "Plus Jakarta Sans", "JetBrains Mono"} {
		if !strings.Contains(string(css), "/* "+family+" ") {
			t.Errorf("fonts.css declares no %s face", family)
		}
	}
	// U+0131 (dotless i, as in Kadıoğlu) and U+011F (g with breve) live in latin-ext.
	// Its unicode-range is the block U+0100-02BA.
	if !strings.Contains(string(css), "U+0100-02BA") {
		t.Error("fonts.css carries no latin-ext unicode-range. Player names need it — " +
			"see the comment at the top of the file before removing the subset.")
	}
}

// TestTheFontFilesAreDeduplicated pins the de-duplication, because the naive version of
// the generator is the one someone will reach for.
//
// All three families are variable fonts: Google serves one file per (family, subset) and
// references it from every weight. Saving one file per @font-face rule stores the same
// bytes four times. The generator keys files by content hash for this reason; this test
// fails if that is undone.
func TestTheFontFilesAreDeduplicated(t *testing.T) {
	static := Static()
	entries, err := fs.ReadDir(static, "fonts")
	if err != nil {
		t.Fatalf("reading fonts/: %v", err)
	}
	// Three families times two subsets. A per-weight generator would produce 20.
	if len(entries) != 6 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("fonts/ holds %d files, want 6 (three families x two subsets): %v\n"+
			"These are variable fonts — one file serves every weight. See fontsubset.py.",
			len(entries), names)
	}
	for _, e := range entries {
		if strings.ContainsAny(strings.TrimSuffix(e.Name(), ".woff2"), "0123456789") {
			t.Errorf("%s is named for a weight. Variable fonts have no per-weight file; "+
				"a numbered name means the de-duplication was removed.", e.Name())
		}
	}
}
