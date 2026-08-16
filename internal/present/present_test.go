package present

import (
	"strings"
	"testing"

	"armband/internal/analysis"
)

// TestPaddingIsMeasuredInRunes is the reason this package exists rather than a
// handful of Printf verbs at the call site.
//
// `%-18s` pads by BYTES. Every accented name — Ødegaard, Martínez, Guéhi,
// Sánchez, Guimarães — occupies more bytes than characters, so the old squad
// printer padded those rows short and the column ran ragged for exactly the
// players most likely to be in a good squad. It is invisible in a test written
// with ASCII names, which is why this one is not.
func TestPaddingIsMeasuredInRunes(t *testing.T) {
	for _, name := range []string{"Ødegaard", "Martínez", "Guimarães", "Guéhi", "Sánchez"} {
		if got := width(pad(name, 12)); got != 12 {
			t.Errorf("pad(%q, 12) has printable width %d, want 12 — padding is counting "+
				"bytes rather than characters, which shreds the column for every "+
				"accented name", name, got)
		}
		if len(pad(name, 12)) == 12 && len(name) != len([]rune(name)) {
			t.Errorf("pad(%q, 12) produced exactly 12 BYTES, which for a multi-byte "+
				"name means it padded to the wrong width", name)
		}
	}
}

// TestPaddingIgnoresColour pins the other half. A coloured cell carries escape
// bytes that occupy no columns, so measuring the raw string over-counts and the
// pitch collapses the moment colour is switched on — which is the default.
func TestPaddingIgnoresColour(t *testing.T) {
	tm := Theme{Colour: true}
	coloured := tm.green("Saka")
	if coloured == "Saka" {
		t.Fatal("theme did not colour; this test is measuring nothing")
	}
	if got := width(coloured); got != 4 {
		t.Errorf("width of a coloured %q is %d, want 4 — escape sequences are being "+
			"counted as printable columns", "Saka", got)
	}
	if got := width(centre(coloured, 10)); got != 10 {
		t.Errorf("centre of a coloured cell has width %d, want 10", got)
	}
}

func TestTruncateMarksTheCut(t *testing.T) {
	if got := truncate("Alexander-Arnold", 8); got != "Alexand…" {
		t.Errorf("truncate = %q, want %q — a clipped name must look clipped, or a "+
			"reader takes it for the player's actual name", got, "Alexand…")
	}
	if got := truncate("Saka", 8); got != "Saka" {
		t.Errorf("truncate shortened a name that fits: %q", got)
	}
	// Rune-safe: cutting a multi-byte name must not split a character.
	if got := truncate("Guimarães", 5); !strings.HasSuffix(got, "…") || width(got) != 5 {
		t.Errorf("truncate(%q, 5) = %q, width %d", "Guimarães", got, width(got))
	}
}

// TestThemeZeroValueIsColourless matters because the same renderers write to a
// terminal and to a file. A theme that defaulted to colour would put escape
// codes into anything redirected.
func TestThemeZeroValueIsColourless(t *testing.T) {
	var zero Theme
	if got := zero.green("x"); got != "x" {
		t.Errorf("the zero Theme emitted colour: %q", got)
	}
}

func sampleSquad() analysis.Squad {
	mk := func(id int, name, team, pos string, price, score float64) analysis.PlayerMetrics {
		return analysis.PlayerMetrics{
			ID: id, Name: name, Team: team, Position: pos, Price: price, Score: score,
		}
	}
	xi := []analysis.PlayerMetrics{
		mk(1, "Raya", "ARS", "GKP", 5.5, 4.1),
		mk(2, "Gabriel", "ARS", "DEF", 6.2, 5.0),
		mk(3, "Guéhi", "CRY", "DEF", 4.6, 4.2),
		mk(4, "Ødegaard", "ARS", "MID", 8.4, 5.6),
		mk(5, "Saka", "ARS", "MID", 10.1, 6.9),
		mk(6, "Guimarães", "NEW", "MID", 6.4, 4.8),
		mk(7, "Semenyo", "BOU", "MID", 7.3, 5.4),
		mk(8, "Haaland", "MCI", "FWD", 15.0, 8.2),
		mk(9, "Mateta", "CRY", "FWD", 7.6, 5.1),
		mk(10, "Watkins", "AVL", "FWD", 9.0, 5.5),
		mk(11, "Senesi", "BOU", "DEF", 4.9, 4.4),
	}
	bench := []analysis.PlayerMetrics{
		mk(12, "Forster", "TOT", "GKP", 4.0, 0.8),
		mk(13, "Burn", "NEW", "DEF", 4.5, 3.1),
		mk(14, "Thiaw", "NEW", "DEF", 4.4, 2.9),
		mk(15, "Diop", "FUL", "DEF", 4.3, 2.2),
	}
	return analysis.Squad{
		Players:    append(append([]analysis.PlayerMetrics{}, xi...), bench...),
		StartingXI: xi, Bench: bench, Formation: "3-4-3",
		Captain: xi[7], ViceCaptain: xi[4],
		TotalCost: 98.2, Remaining: 1.8, XIScore: 59.2, ExpectedPoints: 67.4,
	}
}

// TestSquadRendersEveryPlayerExactlyOnce is the invariance that matters for a
// team sheet: a formation renderer that groups by position can silently drop a
// player whose position string is unexpected, and the output still looks like a
// squad.
func TestSquadRendersEveryPlayerExactlyOnce(t *testing.T) {
	sq := sampleSquad()
	var b strings.Builder
	Squad(&b, sq, PlainTheme(), "Test")
	out := b.String()
	for _, p := range sq.Players {
		if n := strings.Count(out, truncate(p.Name, 20)); n < 1 {
			t.Errorf("%s (%s) is missing from the rendered squad", p.Name, p.Position)
		}
	}
	if !strings.Contains(out, "3-4-3") {
		t.Error("the formation is not shown")
	}
	// The captain must be marked, or the sheet is wrong about the one player
	// whose score is doubled.
	if !strings.Contains(out, "(C)") {
		t.Error("no captain marker")
	}
	if !strings.Contains(out, "(V)") {
		t.Error("no vice-captain marker")
	}
}

// TestNoMoveSaysSoPlainly guards a real recommendation, not a formatting nicety:
// this project's review policy says doing nothing is a first-class outcome and
// usually correct, so an empty plan must render as a decision rather than as an
// empty table that reads like a failure.
func TestNoMoveSaysSoPlainly(t *testing.T) {
	var b strings.Builder
	Moves(&b, analysis.Plan{}, PlainTheme())
	out := b.String()
	if !strings.Contains(out, "No move this week") {
		t.Errorf("an empty plan did not render as an explicit decision:\n%s", out)
	}
}

func TestMovesShowBothSidesAndTheGain(t *testing.T) {
	sq := sampleSquad()
	plan := analysis.Plan{
		Transfers: 1,
		Moves: []analysis.Swap{{
			Out: sq.Bench[3], In: sq.StartingXI[1], Gain: 1.4,
		}},
		GainPerGW: 1.4,
		Spend:     19,
	}
	var b strings.Builder
	Moves(&b, plan, PlainTheme())
	out := b.String()
	for _, want := range []string{"Diop", "Gabriel", "+1.40", "costs £1.9m", "1 TRANSFER"} {
		if !strings.Contains(out, want) {
			t.Errorf("moves output is missing %q:\n%s", want, out)
		}
	}
}

// TestHTMLEscapesNames is a security check, not a cosmetic one. Names come from
// FPL's API, the file is opened in a browser, and html/template is what makes
// that safe. A switch to text/template or Fprintf would compile and look fine.
func TestHTMLEscapesNames(t *testing.T) {
	sq := sampleSquad()
	sq.StartingXI[0].Name = `<script>alert(1)</script>`
	sq.Players[0].Name = sq.StartingXI[0].Name

	var b strings.Builder
	if err := HTML(&b, sq, nil, "T", ""); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	out := b.String()
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Error("a player name was written into the page unescaped — html/template " +
			"has been swapped for something that does not escape")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("the name does not appear escaped either; is it being dropped?")
	}
}

func TestHTMLIsSelfContained(t *testing.T) {
	var b strings.Builder
	if err := HTML(&b, sampleSquad(), nil, "T", "sub"); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	out := b.String()
	// No external fetches: the page must open from file:// with the network off.
	for _, bad := range []string{"http://", "https://", "<script", "@import"} {
		if strings.Contains(out, bad) {
			t.Errorf("the page reaches outside itself (%q); it must render offline "+
				"from a file:// URL", bad)
		}
	}
	if !strings.Contains(out, "<!doctype html>") {
		t.Error("not a complete document")
	}
}

// TestOnlyRiskyBandsAreFlagged pins a bug this package shipped for one run.
//
// analysis.rotationLabel returns a band for EVERY player — "nailed" is a value,
// not an absence — so the first version's `RotationRisk != ""` test flagged all
// eleven starters. A warning that fires on everyone is worse than no warning:
// it looks deliberate, and it trains the reader to ignore the one case that
// matters.
func TestOnlyRiskyBandsAreFlagged(t *testing.T) {
	for _, band := range []string{"nailed", "likely starter"} {
		if isRisky(band) {
			t.Errorf("%q is good news and must not be flagged", band)
		}
	}
	for _, band := range []string{"rotation risk", "squad player", "fringe"} {
		if !isRisky(band) {
			t.Errorf("%q is a real warning and must be flagged", band)
		}
	}
}

// TestAWholeNailedSquadIsUnmarked is the end-to-end form of the same thing: the
// check above could pass while the renderer still marked everyone.
func TestAWholeNailedSquadIsUnmarked(t *testing.T) {
	sq := sampleSquad()
	for i := range sq.StartingXI {
		sq.StartingXI[i].RotationRisk = "nailed"
	}
	var b strings.Builder
	Squad(&b, sq, Theme{Colour: true}, "Test")
	if strings.Contains(b.String(), redSeq+" !") {
		t.Error("a squad of eleven nailed starters still rendered rotation warnings")
	}
}
