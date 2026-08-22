package webui

import (
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"
)

// pages are the documents, by the name Page takes.
var pages = []string{"landing", "app", "team"}

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

// externalOrigins are the ONLY external origins any document here may name, and the
// allowlist is the point: the guard below stopped being "no external strings" and became
// "no external LOADS, plus these two origins and nothing else".
//
// Two things arrived that are genuinely external and genuinely correct, and the old guard
// could not tell them from a missing asset:
//
//   - The og:/twitter: share tags. Both specs require ABSOLUTE urls — a scraper resolves
//     them against nothing — so there is no relative spelling that works. These are read
//     by Facebook, X and Slack from OUR server; the webview never fetches them.
//   - The footer's GitHub and Method links. A navigation the reader chooses, not a
//     subresource the document pulls in before it can paint.
//
// The distinction that matters is LOADED versus NAVIGATED-TO. The guard's own reason is
// "an external reference is not a slow first paint, it is a missing asset" — that is a
// statement about subresources, and it stays absolute for them: src=, <link href=> and
// CSS url() are still checked against the whole document with no exemption at all.
//
// ⚠️ Do not widen this to a bare "skip anchors and meta". Pinning the origins means a
// third one — a CDN, an analytics host, a font provider — still fails this test, which is
// the failure the guard was written to catch. Adding an origin here should feel like a
// decision, because it is one.
var externalOrigins = []string{
	"https://fplarmband.com",
	"https://github.com/beeradb/FPL-Armband",
}

// anchorHref, metaContent and dataGate find the three places an allowlisted origin may
// appear. They are deliberately narrow: an <a> element's href, a <meta> element's content,
// and a gate form's data-gate. dataGate is the odd one in with the other two rather than
// with a plain subresource: gate.js reads it and does exactly one explicit, script-driven
// fetch to it (see gate.js's own doc comment) — a NAVIGATION the page's own code chooses to
// make, not something the embedded webview loads on parse the way <img src> or <link
// href> would. The old landing.js carried this same origin as a JS string constant, which
// this scan never saw at all; moving it onto the form's markup (so one attribute serves
// three configurations rather than three copies of a fetch — see the design record) made
// it visible here for the first time, and it belongs on the allowlist path, not blocked.
var (
	anchorHref  = regexp.MustCompile(`(?i)<a\b[^>]*?\shref="([^"]*)"`)
	metaContent = regexp.MustCompile(`(?i)<meta\b[^>]*?\scontent="([^"]*)"`)
	dataGate    = regexp.MustCompile(`(?i)\sdata-gate="([^"]*)"`)
)

func scannable(body []byte) string {
	s := string(body)
	s = cssComment.ReplaceAllString(s, " ")
	s = htmlnote.ReplaceAllString(s, " ")
	s = dataURI.ReplaceAllString(s, "url()")
	return s
}

// originAllowed reports whether an external reference names one of the origins the
// documents may name at all. Prefix, not equality: the GitHub entry pins the repository,
// so a link to some other repository on the same host still fails.
func originAllowed(ref string) bool {
	for _, o := range externalOrigins {
		if strings.HasPrefix(ref, o) {
			return true
		}
	}
	return false
}

// scanExternal splits a document's external references into the two kinds that are not
// the same problem: origins NAMED in a navigation or share tag, which must be
// allowlisted, and subresources LOADED by the document, which must not be external at all.
//
// It is a function rather than a closure inside the test so that
// TestTheExternalHostGuardStillFiresOnASubresource can drive it with synthetic documents.
// A guard that was narrowed needs a test proving it still catches what it was written for,
// or the narrowing is indistinguishable from deleting it.
func scanExternal(raw []byte) (badOrigins, loads []string) {
	body := scannable(raw)

	// Anchors, meta values and data-gate attributes are checked against the allowlist and
	// then REMOVED, so that whatever survives to externalRef below is a genuine
	// subresource. Removing them is what keeps the remaining scan absolute rather than
	// advisory.
	for _, re := range []*regexp.Regexp{anchorHref, metaContent, dataGate} {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			if externalRef.MatchString(m[1]) && !originAllowed(m[1]) {
				badOrigins = append(badOrigins, m[1])
			}
		}
		body = re.ReplaceAllString(body, "<elided>")
	}
	if m := externalRef.FindString(body); m != "" {
		loads = append(loads, m)
	}
	return badOrigins, loads
}

// TestTheExternalHostGuardStillFiresOnASubresource is the proof that narrowing the guard
// did not empty it.
//
// TestTheEmbeddedAppReachesNoExternalHost used to reject any external-looking string
// anywhere in a document. It cannot any more: og:/twitter: tags must carry absolute urls
// by specification, and the footer links out to the repository on purpose. So the guard
// now distinguishes a LOAD from a NAVIGATION — and a distinction is exactly the kind of
// change that quietly stops catching anything. These cases pin that it still does.
func TestTheExternalHostGuardStillFiresOnASubresource(t *testing.T) {
	cases := []struct {
		name       string
		doc        string
		wantOrigin bool
		wantLoad   bool
	}{{
		name: "a CDN stylesheet is still caught",
		doc:  `<link rel="stylesheet" href="https://cdn.example.com/a.css">`,
		// A <link> is not an <a>: it loads.
		wantLoad: true,
	}, {
		name:     "an external script is still caught",
		doc:      `<script src="https://analytics.example.com/t.js"></script>`,
		wantLoad: true,
	}, {
		name:     "a remote image is still caught",
		doc:      `<img src="https://images.example.com/hero.png">`,
		wantLoad: true,
	}, {
		name:     "a remote font in CSS is still caught",
		doc:      `<style>@font-face{src:url(https://fonts.example.com/x.woff2);}</style>`,
		wantLoad: true,
	}, {
		name:       "an anchor to an unlisted origin is caught",
		doc:        `<a href="https://twitter.com/someone">follow</a>`,
		wantOrigin: true,
	}, {
		name:       "a share tag pointing off-origin is caught",
		doc:        `<meta property="og:image" content="https://evil.example.com/x.png">`,
		wantOrigin: true,
	}, {
		name:       "another repository on the allowed host is still caught",
		doc:        `<a href="https://github.com/someone-else/other">src</a>`,
		wantOrigin: true,
	}, {
		name: "the real share tags and footer links pass",
		doc: `<meta property="og:url" content="https://fplarmband.com/">` +
			`<meta property="og:image" content="https://fplarmband.com/assets/og-image.png">` +
			`<a href="https://github.com/beeradb/FPL-Armband">GitHub</a>` +
			`<a href="/privacy">Privacy</a>`,
	}, {
		name:       "a data-gate naming an unlisted origin is caught",
		doc:        `<form class="gatecard" data-gate="https://evil.example.com/gate">`,
		wantOrigin: true,
	}, {
		name: "the real gate form's data-gate passes",
		doc:  `<form class="gatecard" data-gate="https://fplarmband.com/gate">`,
	}, {
		name: "a comment mentioning an origin is not a reference",
		doc:  `<!-- see https://ogp.me/ for why these are absolute -->`,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			origins, loads := scanExternal([]byte(c.doc))
			if got := len(origins) > 0; got != c.wantOrigin {
				t.Errorf("disallowed-origin = %v, want %v (found %q)", got, c.wantOrigin, origins)
			}
			if got := len(loads) > 0; got != c.wantLoad {
				t.Errorf("external-load = %v, want %v (found %q)", got, c.wantLoad, loads)
			}
		})
	}
}

func TestTheEmbeddedAppReachesNoExternalHost(t *testing.T) {
	static := Static()
	var checked int

	check := func(name string, raw []byte) {
		checked++
		badOrigins, loads := scanExternal(raw)
		for _, o := range badOrigins {
			t.Errorf("%s names the external origin %q, which is not in externalOrigins. "+
				"Navigations and share metadata may leave this host, but only to origins "+
				"listed there on purpose.", name, o)
		}
		for _, m := range loads {
			t.Errorf("%s loads an external subresource (%q). The app ships into an embedded "+
				"webview with no network; every asset must be served from the binary. "+
				"(Navigations and <meta> share tags are checked separately, against "+
				"externalOrigins — this is neither.)", name, m)
		}
		if strings.Contains(scannable(raw), "@import") {
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
		case ref == "/" || ref == "/app" || ref == "/armband-team":
			// The page routes. Served by cmd/armband/serve.go, not from static.
			return
		case ref == "/privacy":
			// The privacy notice. It is a SITE surface rather than an application one:
			// this binary neither embeds nor serves it, and a local `armband serve` will
			// answer 404 for it. It is published in front of this process by the
			// deployment that fronts the public site, which is why the footer link is a
			// same-origin path and there is no document here to resolve it to.
			//
			// ⚠️ Not an oversight, and not a page waiting to be added. If you are adding
			// pages to assets/pages/, this is deliberately not one of them.
			return
		case externalRef.MatchString(ref):
			// An absolute reference to another origin. There is nothing in the embedded
			// tree for it to resolve TO, so resolving it here would report a false
			// missing asset — which is exactly what happened when the footer's GitHub
			// and Method links landed: path.Join turned them into
			// "pages/https:/github.com/...".
			//
			// This is not a hole. Which external origins may appear at all, and in which
			// elements, is pinned by TestTheEmbeddedAppReachesNoExternalHost against
			// externalOrigins. That test owns the question; this one owns local files.
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
