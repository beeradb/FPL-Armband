package webui

import "bytes"

// This file strips comments from CSS and HTML on the way OUT of the binary. The source
// tree keeps every comment -- AGENTS.md is explicit that a comment here carries the
// reasoning that justified a constant, and several of armband.css's do. What a browser
// downloads does not need any of that: 46% of armband.css's bytes, measured 2026-08-27,
// are comment, and every one of them is render-blocking CSS a reader waits on before first
// paint. Stripping happens once, at startup, in webui.go's stripOnce -- this file only
// knows how to strip one buffer, not when to run or how to cache the result.
//
// It is NOT a minifier. Whitespace, formatting and rule order are untouched; only comment
// bytes are removed. A bundler or a build step is deliberately not part of this project
// (see webui.go's own doc comment on why assets are embedded, not built), and a minifier
// is the same kind of thing by another name.

// stripCSS removes CSS comments from src, except a leading `/*!` license block, which
// convention (and several vendored stylesheets elsewhere) treats as required attribution
// that must survive minification. Nothing else here uses that marker, so this is a no-op
// on today's files -- it is here because a license header is exactly the kind of thing
// that arrives without warning in a dependency bump, and the rule should already exist
// when it does.
//
// Strings and url() are scanned as opaque spans before any comment check runs, so a
// content property or a data: URI carrying a literal "/*" is copied through unchanged
// rather than read as the start of a comment. armband.css's icon glyphs are inline SVG
// inside url("data:image/svg+xml..."), which is exactly this case.
func stripCSS(src []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(src))
	n := len(src)
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '/' && i+1 < n && src[i+1] == '*':
			end := cssCommentEnd(src, i)
			if i+2 < n && src[i+2] == '!' {
				out.Write(src[i:end])
			}
			i = end
		case (c == 'u' || c == 'U') && hasFoldPrefix(src[i:], "url("):
			end := cssURLEnd(src, i)
			out.Write(src[i:end])
			i = end
		case c == '"' || c == '\'':
			end := cssStringEnd(src, i, c)
			out.Write(src[i:end])
			i = end
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.Bytes()
}

// cssCommentEnd returns the index just past the "*/" that closes the comment starting at
// i (which must point at the leading "/*"). An unterminated comment -- malformed input --
// runs to end of file rather than panicking; a stylesheet this project ships is never
// meant to hit that branch, but a scanner that indexes past len(src) on bad input is a
// worse failure than stripping too much of it.
func cssCommentEnd(src []byte, i int) int {
	if rel := bytes.Index(src[i+2:], []byte("*/")); rel >= 0 {
		return i + 2 + rel + 2
	}
	return len(src)
}

// cssStringEnd returns the index just past the closing quote matching the one at i,
// honouring backslash escapes so an escaped quote does not end the string early.
func cssStringEnd(src []byte, i int, quote byte) int {
	n := len(src)
	for j := i + 1; j < n; j++ {
		switch src[j] {
		case '\\':
			j++ // skip the escaped character, whatever it is
		case quote:
			return j + 1
		}
	}
	return n
}

// cssURLEnd returns the index just past the ")" closing the url(...) starting at i (which
// must point at "url(", case-insensitively). A quoted argument is scanned with
// cssStringEnd so a quoted url() with a stray ")" or "/*" inside the string is not
// mistaken for the closing paren or a comment; an unquoted argument is scanned to the
// first unescaped ")", per the CSS url-token grammar.
func cssURLEnd(src []byte, i int) int {
	n := len(src)
	j := i + 4
	for j < n {
		switch src[j] {
		case '"', '\'':
			j = cssStringEnd(src, j, src[j])
		case ')':
			return j + 1
		case '\\':
			j += 2
		default:
			j++
		}
	}
	return n
}

// hasFoldPrefix reports whether b starts with prefix, ASCII case-insensitively. CSS
// keywords and HTML tag/attribute names are case-insensitive; this project's own assets
// are consistently lower-case, but a dependency or a hand-edit is not guaranteed to be.
func hasFoldPrefix(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for k := 0; k < len(prefix); k++ {
		bc, pc := b[k], prefix[k]
		if 'A' <= bc && bc <= 'Z' {
			bc += 'a' - 'A'
		}
		if 'A' <= pc && pc <= 'Z' {
			pc += 'a' - 'A'
		}
		if bc != pc {
			return false
		}
	}
	return true
}

// stripHTML removes HTML comments from src, except a conditional comment (`<!--[if ...`
// or the `<![endif]-->` half of a downlevel-revealed one) -- IE is dead, but a comment
// that changes what a browser parses is not a comment in the sense this function strips,
// and losing that distinction silently changes behaviour rather than just shrinking a
// download.
//
// A <script> or <style> element's text content is never scanned for "<!--": that is not
// HTML comment syntax there, it is JavaScript or CSS source text, and the two languages
// disagree about what a comment looks like. Each is instead handed to stripJS or stripCSS,
// so an inline block gets the same treatment as the external files it sits beside -- this
// project's landing page carries one such inline <style>. Everything else, including tag
// and attribute text, is copied through unexamined: a comment cannot open inside a tag
// (the parser is not looking for one there), so scanning for "<!--" only outside tags is
// correct, not merely convenient.
func stripHTML(src []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(src))
	n := len(src)
	for i := 0; i < n; {
		switch {
		case hasFoldPrefix(src[i:], "<!--"):
			end, preserve := htmlCommentEnd(src, i)
			if preserve {
				out.Write(src[i:end])
			}
			i = end
		case hasFoldPrefix(src[i:], "<script"):
			tagEnd := htmlTagEnd(src, i)
			out.Write(src[i:tagEnd])
			bodyEnd, closeEnd := htmlRawTextEnd(src, tagEnd, "</script")
			out.Write(stripJS(src[tagEnd:bodyEnd]))
			out.Write(src[bodyEnd:closeEnd])
			i = closeEnd
		case hasFoldPrefix(src[i:], "<style"):
			tagEnd := htmlTagEnd(src, i)
			out.Write(src[i:tagEnd])
			bodyEnd, closeEnd := htmlRawTextEnd(src, tagEnd, "</style")
			out.Write(stripCSS(src[tagEnd:bodyEnd]))
			out.Write(src[bodyEnd:closeEnd])
			i = closeEnd
		case src[i] == '<':
			end := htmlTagEnd(src, i)
			out.Write(src[i:end])
			i = end
		default:
			out.WriteByte(src[i])
			i++
		}
	}
	return out.Bytes()
}

// htmlCommentEnd returns the index just past the "-->" closing the comment starting at i,
// and whether the comment must be preserved verbatim (a conditional comment, or one this
// scanner could not safely bound). An unterminated comment is preserved rather than
// dropped: silently deleting the rest of a malformed document loses content, where keeping
// an un-strippable span merely loses a few bytes of savings on input that was never valid
// HTML to begin with.
func htmlCommentEnd(src []byte, i int) (end int, preserve bool) {
	rel := bytes.Index(src[i+4:], []byte("-->"))
	if rel < 0 {
		return len(src), true
	}
	end = i + 4 + rel + 3
	content := src[i+4 : i+4+rel]
	trimmed := bytes.TrimSpace(content)
	if hasFoldPrefix(trimmed, "[if") || bytes.HasPrefix(trimmed, []byte("[endif]")) {
		return end, true
	}
	return end, false
}

// htmlTagEnd returns the index just past the ">" closing the tag starting at i (which must
// point at "<"), honouring quoted attribute values so a ">" inside one does not end the
// tag early. An unterminated tag runs to end of file, the same defensive fallback
// cssCommentEnd and htmlCommentEnd use.
func htmlTagEnd(src []byte, i int) int {
	n := len(src)
	j := i + 1
	var quote byte
	for j < n {
		c := src[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return j + 1
		}
		j++
	}
	return n
}

// htmlRawTextEnd finds the raw-text body of a <script> or <style> element: it returns
// where that body ends (the start of the case-insensitive closeTag match, e.g. "</script")
// and where the whole closing tag ends (just past its ">"). Per the HTML spec neither
// element's content is markup, so nothing inside it -- not even something that looks like
// a nested "<script>" -- ends the element early; only the literal close tag does. If the
// close tag is missing (malformed input), both returned indices are len(src): the entire
// remainder is treated as body and stripped as JS or CSS, with no closing tag to copy.
func htmlRawTextEnd(src []byte, start int, closeTag string) (bodyEnd, closeEnd int) {
	rel := indexFold(src[start:], closeTag)
	if rel < 0 {
		return len(src), len(src)
	}
	bodyEnd = start + rel
	closeEnd = htmlTagEnd(src, bodyEnd)
	return bodyEnd, closeEnd
}

// indexFold is bytes.Index, ASCII case-insensitive. Go's stdlib has no case-insensitive
// byte-slice search; strings.Index would need a string(src) conversion for every raw-text
// element, which unnecessarily copies the (potentially quite large) remainder of the
// document.
func indexFold(src []byte, substr string) int {
	if substr == "" {
		return 0
	}
	n, m := len(src), len(substr)
	for i := 0; i+m <= n; i++ {
		if hasFoldPrefix(src[i:i+m], substr) {
			return i
		}
	}
	return -1
}
