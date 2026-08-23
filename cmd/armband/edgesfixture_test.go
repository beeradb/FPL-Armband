package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"armband/internal/viewmodel"
)

// TestWriteTheEdgesFixture derives the awkward-state fixture from the ordinary one.
//
// Run with the same flag as the generator beside it:
//
//	go test ./cmd/armband/ -run TestWriteThe -update
//
// # Why these states, and why they are derived rather than captured
//
// The layouts most likely to be broken are the ones nobody looks at, and they are exactly
// the ones live data will not produce on demand. A club does not blank to order, the
// optimiser does not hand you an illegal shape, and a market is only empty when a filter
// says so. Waiting for the season to produce them is not a test strategy.
//
// Deriving them from the real document rather than writing one by hand keeps every other
// field honest — the names, the prices, the fixture strips are all real, so a shot of a
// blank gameweek is a shot of this page with a blank gameweek in it rather than a shot of
// something invented.
//
// What it covers:
//
//   - a blanking club, so the FDR strip and the "no fixture" copy render;
//   - a ruled-out player, whose availability is 0 and whose projection is therefore zero —
//     the case a card is most likely to draw as though nothing were wrong;
//   - a very long name, because the design's compact card has 68px at 390px and every
//     truncation rule in it is untested by ordinary names;
//   - an empty market, which is the honest-empty-state the design calls for;
//   - an override that has never been checked, and one that lapsed, which are two
//     different amber treatments;
//   - a squad with no vice-captain, which the replay once forfeited the armband over.
func TestWriteTheEdgesFixture(t *testing.T) {
	if !*updateGoldens {
		t.Skip("generator; run with -update to rewrite the edges fixture")
	}

	dir := filepath.Join("..", "..", "internal", "webui", "testdata", "state")
	raw, err := os.ReadFile(filepath.Join(dir, "gameweek-one.json"))
	if err != nil {
		t.Fatalf("the ordinary fixture must exist first: %v", err)
	}
	var st viewmodel.State
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}

	if len(st.Squad.Players) < 4 || len(st.Market.Rows) < 1 {
		t.Fatalf("the ordinary fixture is too small to derive edges from")
	}

	// A club that blanks: strip its fixtures, and every player of that club with it.
	blanking := st.Squad.Players[0].Club
	for i := range st.Squad.Players {
		if st.Squad.Players[i].Club == blanking {
			st.Squad.Players[i].Fixtures = nil
			st.Squad.Players[i].AvgDifficulty = 0
		}
	}

	// A ruled-out player: availability 0 means the score is zero for that reason and no
	// other, which is the one case Player.Availability exists to carry.
	st.Squad.Players[1].Availability = 0
	st.Squad.Players[1].XP = 0
	st.Squad.Players[1].Status = "i"
	st.Squad.Players[1].News = "Knee injury — expected back mid-October. Assessed at 25% for this round."
	st.Squad.Players[1].Role = "fringe"

	// The other two doubt levels FPL's scale carries, so a single fixture draws all THREE
	// newsflag colours at once (2026-08-22): --flag-doubt at 75%, --warn at 25% -- 50% is
	// the same colour as 25% (cardHtml treats 50% and 25% as one severity, per the design
	// record), so a fourth player at 50% would add nothing new to the shot. XP is scaled by
	// the new availability, same as the ruled-out player above -- these two started at
	// availability 1, so the factor IS the new value.
	st.Squad.Players[4].Availability = 0.75
	st.Squad.Players[4].XP *= 0.75
	st.Squad.Players[4].Status = "d"
	st.Squad.Players[4].News = "Knock in training — assessed fit, but a late call."
	st.Squad.Players[6].Availability = 0.25
	st.Squad.Players[6].XP *= 0.25
	st.Squad.Players[6].Status = "d"
	st.Squad.Players[6].News = "Groin strain — touch and go, expected to be assessed on the day."

	// A name with nowhere to go on a 68px card.
	st.Squad.Players[2].Name = "Højbjerg-Şahin"
	st.Squad.Players[3].Name = "Alexander-Arnold Fernández"

	// No vice-captain. The replay once forfeited the armband entirely when the captain did
	// not play and no vice existed, so the absence is worth drawing.
	st.Squad.Vice = 0

	// An empty market, with the counts to match. The design is explicit that this must be
	// an honest empty state rather than a silently truncated list.
	st.Market.Rows = nil
	st.Market.Count = 0
	st.Market.Clearing = 0

	// Two override treatments that the ordinary fixture does not carry.
	if len(st.Overrides.Live) > 0 {
		st.Overrides.Live[0].NeverChecked = true
		st.Overrides.Live[0].CheckAge = 61
		st.Overrides.Live[0].NeedsCheck = true
		st.Overrides.Live[0].Flag = "CHECK — never verified, set 61d ago"
		st.Overrides.Live[0].Checked = ""
	}
	st.Overrides.Lapsed = append(st.Overrides.Lapsed, viewmodel.Override{
		Kind: "minutes", Code: 999999, Label: "MIN 20", Player: "A Lapsed Correction",
		Club: "TOT", Pos: "MID", SetOn: "2026-06-01", Until: "lapses after GW1",
		Checked: "2026-06-01 (79d)", CheckAge: 79, Lapsed: true,
		Reason: "Set for pre-season only, and past its gameweek — kept rather than dropped, " +
			"because deleting it would hide that a player's treatment changed.",
	})
	st.Overrides.Due = 1
	st.Overrides.Oldest = st.Overrides.Live[0].Player + ", 61 days"

	pretty, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "edges.json")
	if err := os.WriteFile(path, append(pretty, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(pretty))
}
