package present

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
)

// Doc renders the briefing's Markdown as a page in the same visual language as the
// squad and replay pages.
//
// # Why a converter and not a Markdown library
//
// The brief is not arbitrary Markdown — this program writes it, so the subset is
// known and closed: headings, tables, bullets, blockquotes, rules, and inline bold,
// italic and code. A general parser would be a dependency and a much larger surface
// to be wrong in, for a document whose author is the same binary doing the reading.
//
// The trade is stated rather than hidden: anything outside that subset renders as
// plain text rather than silently as markup. If the brief grows a construct this
// does not know, the text still appears — which is the failure direction to choose,
// because the alternative is a section quietly vanishing from a document whose whole
// job is to be complete.
//
// # Escaping
//
// Every span goes through html.EscapeString before any tag is added. Player and club
// names come from FPL and nobody here controls them; this file is written to disk and
// opened in a browser, so it gets the same care as the pages that render those names
// as cards.
func Doc(w io.Writer, md, title, subtitle string) error {
	var b strings.Builder
	b.WriteString(`<!doctype html>` + "\n")
	b.WriteString(`<html lang="en"><head><meta charset="utf-8">` + "\n")
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">` + "\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>%s%s</style></head><body>\n",
		html.EscapeString(title), paletteCSS, docCSS)
	b.WriteString(`<div class="doc">` + "\n")
	if subtitle != "" {
		fmt.Fprintf(&b, `<p class="sub">%s</p>`+"\n", html.EscapeString(subtitle))
	}
	renderMarkdown(&b, md)
	b.WriteString("</div>\n</body></html>\n")
	_, err := io.WriteString(w, b.String())
	return err
}

var (
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItal   = regexp.MustCompile(`\*([^*]+)\*`)
	reCode   = regexp.MustCompile("`([^`]+)`")
	reHeader = regexp.MustCompile(`^(#{1,4})\s+(.*)$`)
	reBullet = regexp.MustCompile(`^[-*]\s+(.*)$`)
)

// inline escapes a span and then applies the inline marks, in that order. Escaping
// after would turn the tags this adds back into text.
func inline(s string) string {
	s = html.EscapeString(s)
	s = reCode.ReplaceAllString(s, "<code>$1</code>")
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = reItal.ReplaceAllString(s, "<em>$1</em>")
	return s
}

func renderMarkdown(b *strings.Builder, md string) {
	lines := strings.Split(md, "\n")
	inList, inQuote := false, false
	closeBlocks := func() {
		if inList {
			b.WriteString("</ul>\n")
			inList = false
		}
		if inQuote {
			b.WriteString("</blockquote>\n")
			inQuote = false
		}
	}

	for i := 0; i < len(lines); i++ {
		ln := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimSpace(ln)

		switch {
		case trimmed == "":
			closeBlocks()

		case trimmed == "---":
			closeBlocks()
			b.WriteString("<hr>\n")

		case reHeader.MatchString(trimmed):
			closeBlocks()
			m := reHeader.FindStringSubmatch(trimmed)
			n := len(m[1])
			fmt.Fprintf(b, "<h%d>%s</h%d>\n", n, inline(m[2]), n)

		// A table is recognised by its separator row, which is the only
		// unambiguous marker: a line of pipes could otherwise be prose.
		case strings.HasPrefix(trimmed, "|") && i+1 < len(lines) &&
			strings.Contains(lines[i+1], "---"):
			closeBlocks()
			i = writeTable(b, lines, i)

		case strings.HasPrefix(trimmed, ">"):
			if !inQuote {
				closeBlocks()
				b.WriteString("<blockquote>\n")
				inQuote = true
			}
			fmt.Fprintf(b, "<p>%s</p>\n", inline(strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))))

		case reBullet.MatchString(trimmed):
			if !inList {
				closeBlocks()
				b.WriteString("<ul>\n")
				inList = true
			}
			fmt.Fprintf(b, "<li>%s</li>\n", inline(reBullet.FindStringSubmatch(trimmed)[1]))

		default:
			closeBlocks()
			fmt.Fprintf(b, "<p>%s</p>\n", inline(trimmed))
		}
	}
	closeBlocks()
}

// writeTable consumes a Markdown table starting at lines[i] and returns the index of
// its last line. Wide tables get their own scroll container: the brief carries a
// twenty-club fixture grid, and a page that scrolls sideways as a whole is unusable.
func writeTable(b *strings.Builder, lines []string, i int) int {
	cells := func(s string) []string {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "|")
		s = strings.TrimSuffix(s, "|")
		out := strings.Split(s, "|")
		for j := range out {
			out[j] = strings.TrimSpace(out[j])
		}
		return out
	}

	b.WriteString(`<div class="tscroll"><table>` + "\n<thead><tr>")
	for _, c := range cells(lines[i]) {
		fmt.Fprintf(b, "<th>%s</th>", inline(c))
	}
	b.WriteString("</tr></thead>\n<tbody>\n")

	j := i + 2 // skip the separator row
	for ; j < len(lines); j++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[j]), "|") {
			break
		}
		b.WriteString("<tr>")
		for _, c := range cells(lines[j]) {
			fmt.Fprintf(b, "<td>%s</td>", inline(c))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody></table></div>\n")
	return j - 1
}
