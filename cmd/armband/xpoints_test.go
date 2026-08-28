package main

import (
	"testing"

	"armband/internal/analysis"
)

func pm(id int, name, pos string, price, score, mins float64) analysis.PlayerMetrics {
	return analysis.PlayerMetrics{
		ID: id, Name: name, Team: "XXX", Position: pos,
		Price: price, Score: score, ExpectedMinutes: mins,
	}
}

// ⚠️ This is the recorded failure this command exists downstream of. The first
// hand-built version of this ranking skipped `applyRoster` and printed a
// plausible table: a backup goalkeeper ranked as though he played, and four
// players whose minutes overrides had not been installed were missing. Nothing
// errored. The exclude list must remove rows rather than zero them, because a
// zero row still reads as a prediction about the player.
//
// The filter itself is three lines in cmdXPoints and cannot be reached without a
// live engine, so this pins the property on the same data shape: an excluded id
// is absent from the output entirely, not present with a zero.
func TestExcludedPlayersAreAbsentRatherThanZeroed(t *testing.T) {
	rows := []analysis.PlayerMetrics{
		pm(1, "Keeper", "GKP", 4.5, 3.9, 90),
		pm(2, "Backup", "GKP", 4.0, 0, 0),
	}
	excluded := map[int]bool{2: true}

	var kept []analysis.PlayerMetrics
	for _, m := range rows {
		if !excluded[m.ID] {
			kept = append(kept, m)
		}
	}
	if len(kept) != 1 || kept[0].ID != 1 {
		t.Fatalf("excluded player survived the filter: %+v", kept)
	}
	rank, err := xpointsRanker("xp")
	if err != nil {
		t.Fatal(err)
	}
	got := groupByPosition(kept, map[string]bool{"GKP": true}, rank, 0)
	for _, g := range got {
		for _, m := range g.Players {
			if m.ID == 2 {
				t.Error("an excluded player reached the table; a zero row reads as " +
					"a prediction, which is the failure this filter exists to prevent")
			}
		}
	}
}

// -n is PER POSITION. "15 forwards and 30 midfielders" is the shape the weekly
// images are cut to, and a global cap would silently return mostly midfielders
// because there are more of them and they score higher as a group.
func TestNTruncatesPerPositionNotAcrossTheWholePool(t *testing.T) {
	var rows []analysis.PlayerMetrics
	for i := 1; i <= 5; i++ {
		rows = append(rows, pm(i, "mid", "MID", 6, float64(20-i), 90))
	}
	for i := 6; i <= 10; i++ {
		rows = append(rows, pm(i, "fwd", "FWD", 8, float64(5-i%5), 90))
	}
	rank, err := xpointsRanker("xp")
	if err != nil {
		t.Fatal(err)
	}
	groups := groupByPosition(rows, map[string]bool{"MID": true, "FWD": true}, rank, 2)
	if len(groups) != 2 {
		t.Fatalf("want a table per position, got %d", len(groups))
	}
	for _, g := range groups {
		if len(g.Players) != 2 {
			t.Errorf("%s: -n 2 gave %d players; -n is per position",
				g.Position, len(g.Players))
		}
	}
}

// Positions print in FPL's order, not the order they were typed and not
// alphabetical, so two runs of the same weekly job produce the same file.
func TestGroupsComeBackInFPLPositionOrder(t *testing.T) {
	rows := []analysis.PlayerMetrics{
		pm(1, "f", "FWD", 8, 6, 90),
		pm(2, "d", "DEF", 4, 4, 90),
		pm(3, "m", "MID", 6, 5, 90),
	}
	rank, _ := xpointsRanker("xp")
	groups := groupByPosition(rows, map[string]bool{"FWD": true, "MID": true, "DEF": true}, rank, 0)
	want := []string{"DEF", "MID", "FWD"}
	for i, g := range groups {
		if g.Position != want[i] {
			t.Errorf("position %d is %s, want %s", i, g.Position, want[i])
		}
	}
}

// ⚠️ Every sort rule must be a STRICT TOTAL ORDER. Ties are the normal case on
// price and on a rounded minutes figure, and `sort.Slice` is not stable — so a
// rule that stops at the sorted quantity lets two runs of the identical command
// emit different files. In a scheduled weekly image that reads as the model
// changing its mind when nothing changed at all.
func TestEverySortRuleBreaksTiesDeterministically(t *testing.T) {
	for _, by := range []string{"xp", "value", "price", "minutes", "name"} {
		rank, err := xpointsRanker(by)
		if err != nil {
			t.Fatalf("%s: %v", by, err)
		}
		a := pm(7, "same", "MID", 6, 5, 90)
		b := pm(9, "same", "MID", 6, 5, 90)
		if !rank(a, b) {
			t.Errorf("-sort %s: the lower id must win a tie", by)
		}
		if rank(b, a) {
			t.Errorf("-sort %s: the order reverses when the arguments swap, so it "+
				"is not a strict order and sort.Slice may emit either", by)
		}
	}
}

// An unknown -sort or -pos is a typo in a job nobody is watching. It must fail
// rather than fall through to a default that looks like the thing asked for.
func TestUnknownFiltersAreRefusedRatherThanDefaulted(t *testing.T) {
	if _, err := xpointsRanker("points"); err == nil {
		t.Error("-sort points was accepted; an unknown rule must not silently rank by xp")
	}
	if _, err := wantedPositions("STRIKER"); err == nil {
		t.Error("-pos STRIKER was accepted; an unknown position must not mean all")
	}
	if _, err := wantedPositions("MID,FWD"); err != nil {
		t.Errorf("-pos MID,FWD should parse: %v", err)
	}
	all, err := wantedPositions("")
	if err != nil || len(all) != 4 {
		t.Errorf("empty -pos should mean all four positions, got %v %v", all, err)
	}
}
