package webui

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

// This file verifies stripCSS and stripHTML two ways: targeted synthetic cases for the
// specific traps requirement 2 of the strip-comments work named (a string delimiter or a
// URL inside a comment, and the reverse -- a comment delimiter inside a string), and a
// structural check run over every real embedded asset, proving the only difference
// between the shipped source and what Static()/Page() serve is comment bytes, nothing
// reordered or invented. TestTheServedStylesheetParsesTheSameRules in webui_test.go
// (browser-driven) is the other half: this file proves WHAT was removed; that one proves
// the result still means what it meant.

// delimPair is one comment shape: block delimiters (close == "" means "to end of line",
// used for JS "//") paired with an open marker.
type delimPair struct{ open, close string }

// commentSpanAt reports the end of a comment starting at raw[i:], for one of delims, and
// whether raw[i:] is a comment at all. It is deliberately independent of
// cssCommentEnd/htmlCommentEnd/skipLineComment/skipBlockComment in strip.go and
// stripjs.go: the point of assertOnlyCommentSpansRemoved is to check the STRIPPED OUTPUT
// against a second, separately-written notion of "comment shaped", not to re-run strip.go's
// own idea of one.
func commentSpanAt(raw []byte, i int, delims []delimPair) (end int, ok bool) {
	for _, d := range delims {
		if !bytes.HasPrefix(raw[i:], []byte(d.open)) {
			continue
		}
		if d.close == "" { // line comment: runs to (not including) the next newline
			j := i + len(d.open)
			for j < len(raw) && raw[j] != '\n' {
				j++
			}
			return j, true
		}
		if rel := bytes.Index(raw[i+len(d.open):], []byte(d.close)); rel >= 0 {
			return i + len(d.open) + rel + len(d.close), true
		}
		return len(raw), true
	}
	return 0, false
}

// assertOnlyCommentSpansRemoved walks raw and stripped in lock-step and requires that
// every point where they diverge is exactly a deleted comment span (raw[i:] matches one of
// delims) -- and that nothing else differs: no byte inserted, no byte reordered, nothing
// outside a comment span changed. A preserved comment (a `/*!` block, a conditional HTML
// comment, a sourceMappingURL directive) produces no divergence at all here, since stripped
// still carries it verbatim -- this function never needs to know that rule exists. A page
// with an inline <style> or <script> block is checked against BOTH that language's
// delimiters and HTML's in the same pass, which is why delims is a list rather than one
// pair: landing.html's own "<!-- -->" comments and its inline stylesheet's "/* */" ones
// are removed by two different strip.go code paths but must both show up here as
// legitimate divergences.
func assertOnlyCommentSpansRemoved(t *testing.T, raw, stripped []byte, delims []delimPair) {
	t.Helper()
	ri, si := 0, 0
	for ri < len(raw) && si < len(stripped) {
		if raw[ri] == stripped[si] {
			ri++
			si++
			continue
		}
		if end, ok := commentSpanAt(raw, ri, delims); ok {
			ri = end
			continue
		}
		t.Fatalf("byte %d: raw has %q, stripped has %q, and raw does not start a comment "+
			"there -- something other than a comment changed.\nraw context:      %q\nstripped context: %q",
			ri, raw[ri], stripped[si], window(raw, ri), window(stripped, si))
	}
	// Anything left in raw must be one final deleted (unterminated) comment.
	if ri < len(raw) {
		if _, ok := commentSpanAt(raw, ri, delims); !ok {
			t.Fatalf("raw has %d trailing bytes stripped dropped that are not a comment: %q", len(raw)-ri, window(raw, ri))
		}
	}
	if si < len(stripped) {
		t.Fatalf("stripped has %d trailing bytes with no counterpart in raw: %q", len(stripped)-si, window(stripped, si))
	}
}

func window(b []byte, i int) []byte {
	end := i + 40
	if end > len(b) {
		end = len(b)
	}
	return b[i:end]
}

// TestStripCSSOnlyRemovesComments runs the structural check above against every .css file
// this project actually ships, proving stripCSS's output on the real corpus is raw with
// nothing but /* */ spans deleted (and any /*! */ block left in place, which shows up as
// no divergence at all).
func TestStripCSSOnlyRemovesComments(t *testing.T) {
	sub, err := fs.Sub(assets, "assets/static")
	if err != nil {
		t.Fatal(err)
	}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !bytes.HasSuffix([]byte(p), []byte(".css")) {
			return err
		}
		raw, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		t.Run(p, func(t *testing.T) {
			assertOnlyCommentSpansRemoved(t, raw, stripCSS(raw), []delimPair{{"/*", "*/"}})
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStripHTMLOnlyRemovesComments does the same for every .html page, including its
// inline <style> block: landing.html carries a large one, itself containing CSS comments
// that get the CSS treatment rather than the HTML one, which is why both delimiter shapes
// are passed in -- TestStripHTMLStripsCSSInsideInlineStyle below pins that the CSS half is
// actually exercised there and not just tolerated as a no-op.
func TestStripHTMLOnlyRemovesComments(t *testing.T) {
	entries, err := fs.ReadDir(assets, "assets/pages")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := assets.ReadFile("assets/pages/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		t.Run(e.Name(), func(t *testing.T) {
			stripped := stripHTML(raw)
			assertOnlyCommentSpansRemoved(t, raw, stripped, []delimPair{{"<!--", "-->"}, {"/*", "*/"}})
		})
	}
}

// TestStripHTMLStripsCSSInsideInlineStyle pins that landing.html's inline <style> block
// -- the one place this project's CSS does not live in its own .css file -- gets the same
// treatment as armband.css and fonts.css, rather than being left alone because it happens
// to sit inside a .html file.
func TestStripHTMLStripsCSSInsideInlineStyle(t *testing.T) {
	raw, err := assets.ReadFile("assets/pages/landing.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("/* ---- landing-only ---- */")) {
		t.Fatal("landing.html no longer carries the inline <style> comment this test exercises; update the fixture")
	}
	stripped := stripHTML(raw)
	if bytes.Contains(stripped, []byte("/* ---- landing-only ---- */")) {
		t.Error("stripHTML left a CSS comment inside the inline <style> block")
	}
	if !bytes.Contains(stripped, []byte(".lbg{position:fixed")) {
		t.Error("stripHTML dropped real CSS from the inline <style> block, not just its comment")
	}
}

// TestStripCSSPreservesStringsAndURLs is the CSS half of requirement 2: a comment
// containing a string delimiter or a URL must not corrupt stripping, and a string or
// url() containing something that LOOKS like a comment delimiter must survive untouched.
func TestStripCSSPreservesStringsAndURLs(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "comment containing a quote",
			src:  `/* it's a "quote" in here */ .a{color:red}`,
			want: ` .a{color:red}`,
		},
		{
			name: "comment containing a URL",
			src:  `/* see https://example.com/path/to/thing */ .a{color:red}`,
			want: ` .a{color:red}`,
		},
		{
			name: "string containing a comment delimiter",
			src:  `.a{content:"/* not a comment */"}`,
			want: `.a{content:"/* not a comment */"}`,
		},
		{
			name: "unquoted url() containing a query string",
			src:  `.a{background:url(img.png?x=1&y=2)}`,
			want: `.a{background:url(img.png?x=1&y=2)}`,
		},
		{
			name: "quoted url() containing a paren and a comment-like sequence",
			src:  `.a{background:url("data:image/svg+xml;utf8,<svg>/* not a comment (parens) */</svg>")}`,
			want: `.a{background:url("data:image/svg+xml;utf8,<svg>/* not a comment (parens) */</svg>")}`,
		},
		{
			name: "license comment preserved",
			src:  "/*! Copyright 2026 */\n.a{color:red}",
			want: "/*! Copyright 2026 */\n.a{color:red}",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(stripCSS([]byte(c.src)))
			if got != c.want {
				t.Errorf("stripCSS(%q) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestStripCSSPreservesCharset pins requirement 3's other CSS case. @charset is not a
// comment -- it is a rule, and per the CSS spec it must be the very first thing in the
// stylesheet to take effect at all, with nothing (not even a comment) ahead of it. This
// project's own CSS carries no @charset today (checked: none of the shipped files use
// one), so stripCSS's handling of it has never been exercised by the real corpus the way
// TestStripCSSOnlyRemovesComments exercises everything else -- it survives only because
// stripCSS never rewrites non-comment bytes, which is a claim about the CODE, not a
// property anything here had actually run. This test runs it: a leading @charset followed
// by a comment must come out with the @charset untouched and the comment gone.
// TestStripCSSPreservesCharset pins the one item on the preservation list that had
// no test of its own.
//
// ⚠️ It was argued safe "by construction" — the stripper never rewrites non-comment
// bytes, so a leading `@charset` survives with no special case. That reasoning is
// correct, AND it is the shape of reasoning this project has been wrong about
// repeatedly, so it gets a test rather than an argument.
//
// `@charset` is the strictest rule in CSS: it is honoured ONLY as the very first
// bytes of the sheet. A stripper that removed a leading comment and left the
// declaration one byte later would still be "correct" about comments and would
// silently change the sheet's encoding. No shipped CSS carries one today, which is
// exactly why nothing would have caught a regression here.
func TestStripCSSPreservesCharset(t *testing.T) {
	// Exact equality, not Contains: it pins WHERE the comment bytes went, not just
	// that the declaration survived somewhere.
	src := "@charset \"utf-8\";\n/* explains the rest of the file */\n.a{color:red}"
	want := "@charset \"utf-8\";\n\n.a{color:red}"
	got := string(stripCSS([]byte(src)))
	if got != want {
		t.Errorf("stripCSS(%q)\n  got:  %q\n  want: %q", src, got, want)
	}
	if !strings.HasPrefix(got, "@charset \"utf-8\";") {
		t.Errorf("@charset is no longer the first thing in the stylesheet: %q", got)
	}

	// The converse, which the first case cannot catch: a declaration INSIDE a
	// comment must not survive. A stripper that preserved `@charset` by pattern
	// rather than by position would pass above and fail here.
	inComment := string(stripCSS([]byte("/* @charset \"iso-8859-1\"; */\n.a{color:red}")))
	if strings.Contains(inComment, "iso-8859-1") {
		t.Errorf("a commented-out @charset was resurrected: %q", inComment)
	}
}

// TestStripHTMLPreservesConditionalComments pins requirement 3's HTML case: a conditional
// comment changes what a browser parses, so it is not a comment stripCSS/stripHTML may
// remove.
func TestStripHTMLPreservesConditionalComments(t *testing.T) {
	src := `<p>before</p><!--[if IE]><p>ie only</p><![endif]--><p>after</p>`
	got := string(stripHTML([]byte(src)))
	if got != src {
		t.Errorf("stripHTML altered a conditional comment:\n got:  %q\nwant: %q", got, src)
	}
}

// TestStripHTMLPreservesOrdinaryCommentsNot is the control for the test above: an
// ordinary comment with the same shape but no "[if" must still be removed, or the
// conditional-comment carve-out would be silently swallowing everything.
func TestStripHTMLPreservesOrdinaryCommentsNot(t *testing.T) {
	src := `<p>before</p><!-- just a note --><p>after</p>`
	got := string(stripHTML([]byte(src)))
	want := `<p>before</p><p>after</p>`
	if got != want {
		t.Errorf("stripHTML = %q, want %q", got, want)
	}
}
