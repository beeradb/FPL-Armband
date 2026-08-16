// Package present renders squads and transfers for a human — a pitch in the
// terminal, or a self-contained HTML file.
//
// # Why this is its own package
//
// It computes nothing. Every number it prints is read off analysis.Squad,
// analysis.Plan or analysis.PlayerMetrics, and if a figure is not already on one
// of those it does not belong here — the standing rule in this project is that a
// scoring term is computed once, in internal/analysis, and reported everywhere
// else. A presentation layer that derives a number is a second implementation of
// it, and this codebase's most-repeated bug is two implementations of one
// quantity disagreeing.
//
// # Width is measured in runes, and that is a fix rather than a preference
//
// The previous squad printer used `%-18s`, which pads by BYTES. Every name with
// an accent in it — Ødegaard, Martínez, Guéhi, Areola, Sánchez — is longer in
// bytes than in characters, so those rows were padded short and the column ran
// ragged exactly for the players most likely to be in a good squad. `pad` and
// `truncate` here work in runes. See TestPaddingIsMeasuredInRunes.
//
// This is deliberately not a general table library. It knows about football.
package present

import (
	"os"
	"strings"
	"unicode/utf8"
)

// Theme decides whether ANSI escapes are emitted. The zero value is colourless,
// so a caller that forgets to set it degrades to plain text rather than emitting
// escape codes into a file — which is the failure mode that matters here, since
// the same renderers feed both a terminal and a redirect.
type Theme struct {
	Colour bool
}

// TerminalTheme honours NO_COLOR, which this project already respects elsewhere.
func TerminalTheme() Theme { return Theme{Colour: os.Getenv("NO_COLOR") == ""} }

// PlainTheme never colours. Used for the HTML renderers' text fallbacks and by
// tests, so an assertion compares football rather than escape sequences.
func PlainTheme() Theme { return Theme{} }

const (
	reset     = "\033[0m"
	boldSeq   = "\033[1m"
	dimSeq    = "\033[2m"
	greenSeq  = "\033[32m"
	yellowSeq = "\033[33m"
	cyanSeq   = "\033[36m"
	redSeq    = "\033[31m"
	greySeq   = "\033[90m"
)

func (t Theme) wrap(seq, s string) string {
	if !t.Colour || s == "" {
		return s
	}
	return seq + s + reset
}

func (t Theme) bold(s string) string   { return t.wrap(boldSeq, s) }
func (t Theme) dim(s string) string    { return t.wrap(dimSeq, s) }
func (t Theme) green(s string) string  { return t.wrap(greenSeq, s) }
func (t Theme) yellow(s string) string { return t.wrap(yellowSeq, s) }
func (t Theme) cyan(s string) string   { return t.wrap(cyanSeq, s) }
func (t Theme) red(s string) string    { return t.wrap(redSeq, s) }
func (t Theme) grey(s string) string   { return t.wrap(greySeq, s) }

// posColour gives each position a stable colour, so a formation reads at a
// glance without the row labels being scanned.
func (t Theme) posColour(pos, s string) string {
	switch pos {
	case "GKP":
		return t.yellow(s)
	case "DEF":
		return t.cyan(s)
	case "MID":
		return t.green(s)
	case "FWD":
		return t.red(s)
	}
	return s
}

// width is the printable width of s, in characters rather than bytes, ignoring
// any ANSI escapes already embedded in it.
func width(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}

// stripANSI removes escape sequences so a coloured cell can still be measured.
// Padding a string that contains escapes by its byte or even its rune length
// counts the invisible bytes and shreds the column.
func stripANSI(s string) string {
	if !strings.Contains(s, "\033") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\033' {
			j := i
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
			break
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// truncate shortens s to n printable characters, marking the cut with an
// ellipsis so a clipped name is visibly clipped rather than silently wrong.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// pad left-aligns s in a field n characters wide.
func pad(s string, n int) string {
	if d := n - width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// padLeft right-aligns s in a field n characters wide. Numbers go here.
func padLeft(s string, n int) string {
	if d := n - width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// centre places s in a field n characters wide, biasing the extra space right,
// which is what a formation row wants so the pitch does not drift left.
func centre(s string, n int) string {
	d := n - width(s)
	if d <= 0 {
		return s
	}
	l := d / 2
	return strings.Repeat(" ", l) + s + strings.Repeat(" ", d-l)
}
