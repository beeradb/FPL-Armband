package present

import (
	"strings"
	"testing"
)

func sampleReplay() []ReplayWeek {
	return []ReplayWeek{
		{GW: 1, Points: 68, Captain: "Salah", CaptainPts: 16, Value: 1000, Bank: 0,
			Squad: []ReplayPlayer{
				{Name: "Raya", Team: "ARS", Position: "GKP", Points: 6, Started: true, Opponent: "BOU (H)"},
				{Name: "Salah", Team: "LIV", Position: "MID", Points: 8, Started: true, Captain: true, Opponent: "NEW (A)"},
				{Name: "Benchy", Team: "BUR", Position: "FWD", Points: 9, Started: false, Opponent: "EVE (H)"},
			}},
		{GW: 2, Points: 45, Captain: "Salah", CaptainPts: 10, Value: 993, Bank: 10,
			Squad: []ReplayPlayer{
				{Name: "Raya", Team: "ARS", Position: "GKP", Points: 2, Started: true, Opponent: "CHE (A)"},
				{Name: "Semenyo", Team: "BOU", Position: "MID", Points: 5, Started: true, New: true, Opponent: "LEE (H)"},
			},
			Moves: []ReplayMove{{Out: "Mitoma", In: "Semenyo", Gain: 1.8,
				OutGot: 18, InGot: 33}}},
	}
}

// A replay page must say its numbers are results, never predictions.
//
// Both pages render through one template, which is the point — but it means a
// missing flag silently relabels a finished season as a forecast. "48 pts
// projected" over a gameweek that has already been played is the kind of wrong that
// reads as fine.
func TestReplayLabelsPointsAsScoredNotProjected(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, sampleReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "pts scored") {
		t.Error("a replay does not say its points were scored")
	}
	if strings.Contains(out, "pts projected") {
		t.Error("a replay is labelling results as projections")
	}
}

// The transfer strip is the reason this page exists: it carries both what the model
// believed and what actually happened, because only the second says whether the move
// was right.
func TestReplayShowsBothSidesOfEveryMoveAndWhatTheyReturned(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, sampleReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"Mitoma", "Semenyo", "1.80", "33", "18"} {
		if !strings.Contains(out, want) {
			t.Errorf("the move strip is missing %q — a reader cannot tell whether "+
				"the transfer was right", want)
		}
	}
	// GW1 buys nothing, and an absent strip must say so rather than look like a
	// week whose transfers were forgotten.
	if !strings.Contains(out, "No transfer") {
		t.Error("a week with no transfer does not say so")
	}
}

// Same contract as the squad page: opens from file:// with nothing fetched.
func TestReplayPageIsSelfContained(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, sampleReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, "<script") {
		t.Error("the replay page carries script; the week tabs are CSS-only by design")
	}
	for _, bad := range []string{"http://", "https://"} {
		if strings.Contains(out, bad) {
			t.Errorf("the replay page fetches %s", bad)
		}
	}
}

func TestReplayRefusesAnEmptySeason(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, nil, "T", ""); err == nil {
		t.Error("rendering no gameweeks produced a page instead of an error")
	}
}

// A card with no opponent means a BLANK gameweek, and the template says so in red.
// So an unset field is not a cosmetic gap — it is a confident wrong answer, and the
// first version of HTMLReplay never populated it, which made every card in every week
// read "no fixture".
//
// The assertion is deliberately two-sided. Checking only that opponents appear would
// pass on a page that also cried blank everywhere; checking only that "no fixture" is
// absent would pass on a page that had silently stopped reporting real blanks.
func TestReplayShowsOpponentsAndKeepsBlanksMeaningful(t *testing.T) {
	weeks := sampleReplay()
	var b strings.Builder
	if err := HTMLReplay(&b, weeks, "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"BOU (H)", "NEW (A)"} {
		if !strings.Contains(out, want) {
			t.Errorf("opponent %q is missing from the pitch", want)
		}
	}
	if strings.Contains(out, "opp blank") {
		t.Error("a player WITH an opponent is being flagged as a blank gameweek; " +
			"every row does this when Opponent is never populated")
	}

	// And the real blank still reports as one.
	weeks[0].Squad[0].Opponent = ""
	var b2 strings.Builder
	if err := HTMLReplay(&b2, weeks, "T", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b2.String(), "opp blank") {
		t.Error("a genuine blank gameweek no longer reports as one")
	}
}

// chipReplay is a season with two chips and a hit, which is the combination the
// page has to keep straight: a chip week that also took a hit must show both.
func chipReplay() []ReplayWeek {
	wk := sampleReplay()
	wk[0].Chip = "wildcard"
	wk[1].Chip = "bench boost"
	wk[1].HitCost = 4
	wk[1].Moves[0].Hit = true
	return wk
}

// A chip is the week a reader scans thirty-eight tabs looking for, so it has to be
// findable from the strip without opening anything.
//
// The tab carried the chip name as bare text before this: true, and lost among 38
// monospace labels. The assertion is on the badge AND on the summary, because those
// are two different questions — "which week was it" and "was one played at all".
func TestReplayMarksChipWeeksInTheTabsAndTheSummary(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, chipReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `class="haschip"`) {
		t.Error("a chip week is not marked in the gameweek selector")
	}
	if !strings.Contains(out, `<span class="tc">WC</span>`) {
		t.Error("the wildcard tab carries no chip badge")
	}
	if !strings.Contains(out, `<span class="tc">BB</span>`) {
		t.Error("the bench boost tab carries no chip badge")
	}
	// The summary must name the week, not merely that a chip existed.
	if !strings.Contains(out, "wildcard GW1") || !strings.Contains(out, "bench boost GW2") {
		t.Error("the summary does not say which gameweek each chip was played in")
	}
}

// A season with no chips must SAY so.
//
// Silence is the failure this is written against: a page showing nothing where the
// chips would be reads as "chips were played and went unmarked", which is a
// different and wrong claim from "this replay had no chip plan". Same shape as the
// blank-versus-missing-fixture bug — a hole that renders as a confident wrong
// answer is worse than one that renders as nothing.
func TestReplayWithoutChipsSaysNoneRatherThanShowingNothing(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, sampleReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "none played") {
		t.Error("a chipless replay does not say its chips were never played")
	}
	if strings.Contains(out, `class="tc"`) {
		t.Error("a chipless replay is showing a chip badge")
	}
}

// A hit has to be visible in all three places a reader might look for it: the
// season summary, the week it was taken, and the move that charged it.
//
// "36 transfers, 8 points of hits" in a facts row is true and useless — it cannot
// answer which moves paid, and the per-week points column shows the net figure with
// no sign that four points came off it.
func TestReplayShowsHitsOnTheSeasonTheWeekAndTheMove(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, chipReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "on 1 of 1 moves") {
		t.Error("the summary does not say how many moves took a hit")
	}
	if !strings.Contains(out, `class="hitmark"`) {
		t.Error("the week that took a hit is not marked in its header")
	}
	// The header shows the arithmetic, not just the net: a week that scored 60
	// after a −4 must read "64 − 4 hit = 60", or the deduction is invisible.
	if !strings.Contains(out, "hit</span> = ") {
		t.Error("the week header does not show gross − hit = net")
	}
	if !strings.Contains(out, `class="tc hitcost"`) {
		t.Error("the gameweek tab does not carry the hit cost")
	}
	if !strings.Contains(out, `<span class="tag hit">`) {
		t.Error("the move that took the hit is not marked")
	}
}

// A move with no hit must say "free" rather than leaving the slot empty, so a
// missing badge can never be read as a missing field.
func TestReplayMarksFreeMovesExplicitly(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, sampleReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `<span class="tag free">free</span>`) {
		t.Error("a free move is not labelled, so an absent hit badge is ambiguous")
	}
	if strings.Contains(out, `<span class="tag hit">`) {
		t.Error("a season with no hits is showing a hit badge")
	}
}

// Every transfer of the season must appear in one list, and the placeholder that
// used to occupy that section must be gone when there is something to show.
//
// This is deliberately two-sided. Asserting only that the moves appear would pass
// on a page that ALSO printed the placeholder underneath them, which is exactly
// what shipped: the season Transfers section fell through to NoPlan and showed
// "Every number on this page already happened" and not one of the 36 moves.
func TestReplayListsEveryTransferAndDropsThePlaceholder(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, chipReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "Every transfer, in order") {
		t.Error("the season has no combined transfer list")
	}
	// The per-week strip also renders the move, so count rather than presence:
	// once in the week panel, once in the season list.
	if n := strings.Count(out, "Semenyo</span>"); n < 2 {
		t.Errorf("the move appears %d times; expected it in both the week and the season list", n)
	}
	if !strings.Contains(out, `<span class="gw">GW2</span>`) {
		t.Error("the season transfer list does not say which gameweek each move was made in")
	}
	if strings.Contains(out, "No transfers. The gate found nothing") {
		t.Error("the empty-season placeholder is showing on a season that made transfers")
	}
}

// The season's transfer count must come from the replay, not from counting rows.
//
// A wildcard replaces the fifteen in one act: the replay adds those swaps to the
// season total but records no out-for-in Move for them, because no single player
// was replaced by another. Deriving the headline from the rendered rows produced a
// page reporting 30 transfers for a season the terminal called 37 — one quantity,
// two implementations, which is the failure this project is named for.
//
// Both halves are asserted. The count has to be right, AND the gap has to be
// explained: a headline of 37 over a list of 30 with nothing said is a reader
// hunting for seven missing rows.
func TestReplayCountsTransfersAsTheReplayDidNotAsRowsRendered(t *testing.T) {
	wk := chipReplay()
	// A wildcard week: seven swaps counted, none individually recorded.
	wk[0].Transfers = 7
	wk[0].Rebuilt = true
	wk[1].Transfers = 1

	var b strings.Builder
	if err := HTMLReplay(&b, wk, "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `<div class="v">8`) {
		t.Error("the season transfer count is derived from rendered rows, not from the replay")
	}
	if !strings.Contains(out, "7 in a chip rebuild") {
		t.Error("the headline count exceeds the list with no explanation of the gap")
	}
	if !strings.Contains(out, "7 further transfers are not listed here") {
		t.Error("the transfer list does not say what it is missing")
	}
	if !strings.Contains(out, "Rebuilt fifteen") {
		t.Error("a wildcard week is not marked as a rebuild")
	}
}

// A caller that carries no per-week count still gets a correct total, so the field
// being optional cannot silently zero the headline.
func TestReplayFallsBackToCountingMovesWhenNoCountIsCarried(t *testing.T) {
	var b strings.Builder
	if err := HTMLReplay(&b, sampleReplay(), "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `<div class="v">1`) {
		t.Error("a replay carrying no transfer count reports no transfers")
	}
	if strings.Contains(out, "in a chip rebuild") {
		t.Error("a season with nothing unlisted is claiming unlisted transfers")
	}
}

// The other side of the same coin: a season that genuinely made no transfers must
// say so, rather than rendering an empty section a reader reads as a bug.
func TestReplayWithNoTransfersSaysSo(t *testing.T) {
	wk := sampleReplay()
	wk[1].Moves = nil
	var b strings.Builder
	if err := HTMLReplay(&b, wk, "T", ""); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "No transfers. The gate found nothing") {
		t.Error("a season with no transfers does not say why the section is empty")
	}
	if strings.Contains(out, "Every transfer, in order") {
		t.Error("a season with no transfers is showing a transfer list")
	}
}
