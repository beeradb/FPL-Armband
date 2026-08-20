package viewmodel

import (
	"encoding/json"
	"testing"

	"armband/internal/fpl"
)

// TestBuildPlayerDetailNilInputs checks the two absences this endpoint promises to say so
// about rather than paper over: no summary at all, and no past seasons on record.
func TestBuildPlayerDetailNilInputs(t *testing.T) {
	if d := BuildPlayerDetail(nil, "MID", nil); d == nil || d.LastSeason != nil || d.Gameweeks != nil {
		t.Fatalf("BuildPlayerDetail(nil, ...) = %+v, want an empty, non-nil PlayerDetail", d)
	}
	d := BuildPlayerDetail(&fpl.ElementSummary{}, "MID", nil)
	if d.LastSeason != nil {
		t.Errorf("LastSeason = %+v, want nil for a summary with no history_past at all", d.LastSeason)
	}
	if d.Gameweeks != nil {
		t.Errorf("Gameweeks = %+v, want nil for a summary with no history at all", d.Gameweeks)
	}
}

// TestBuildPlayerDetailReadsTheLastSeason checks the oldest-first trap directly: FPL returns
// history_past oldest first, so "last season" must be the LAST element, and a wrong reading
// here would silently show a two-or-more-year-old season labelled as last year's.
func TestBuildPlayerDetailReadsTheLastSeason(t *testing.T) {
	es := &fpl.ElementSummary{
		HistoryPast: []fpl.PastSeason{
			{SeasonName: "2022/23", TotalPoints: 40, Minutes: 900},
			{SeasonName: "2023/24", TotalPoints: 120, Minutes: 2200},
			{SeasonName: "2024/25", TotalPoints: 200, Minutes: 3000},
		},
	}
	d := BuildPlayerDetail(es, "MID", nil)
	if d.LastSeason == nil || d.LastSeason.Season != "2024/25" {
		t.Fatalf("LastSeason = %+v, want the LAST element (2024/25), not the first", d.LastSeason)
	}
}

// TestBuildPlayerDetailPer90GuardsZeroMinutes checks the one division this function performs:
// zero minutes must return zero, not the NaN that would otherwise fail to marshal and take
// the whole document down with it.
func TestBuildPlayerDetailPer90GuardsZeroMinutes(t *testing.T) {
	es := &fpl.ElementSummary{
		HistoryPast: []fpl.PastSeason{{SeasonName: "2024/25", TotalPoints: 0, Minutes: 0}},
	}
	d := BuildPlayerDetail(es, "MID", nil)
	if d.LastSeason.PointsPer90 != 0 {
		t.Errorf("PointsPer90 = %v on zero minutes, want 0", d.LastSeason.PointsPer90)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshalling a zero-minutes season: %v", err)
	}
	_ = b
}

// TestBuildPlayerDetailCleanSheetsAreGatedByPosition pins the omission both ways: present
// (even at zero) for a defender or keeper, absent entirely for a midfielder or forward.
func TestBuildPlayerDetailCleanSheetsAreGatedByPosition(t *testing.T) {
	es := &fpl.ElementSummary{
		HistoryPast: []fpl.PastSeason{{SeasonName: "2024/25", CleanSheets: 0}},
	}
	for _, tc := range []struct {
		pos     string
		wantNil bool
	}{
		{"GKP", false}, {"DEF", false}, {"MID", true}, {"FWD", true},
	} {
		d := BuildPlayerDetail(es, tc.pos, nil)
		gotNil := d.LastSeason.CleanSheets == nil
		if gotNil != tc.wantNil {
			t.Errorf("pos %s: CleanSheets nil=%v, want nil=%v", tc.pos, gotNil, tc.wantNil)
		}
	}
}

// TestBuildPlayerDetailResolvesOpponents checks that a gameweek's opponent is translated
// through the caller's team map rather than left as FPL's bare team id -- the client must
// never be handed an id it would need a second lookup table to read.
func TestBuildPlayerDetailResolvesOpponents(t *testing.T) {
	es := &fpl.ElementSummary{
		History: []fpl.HistoryEntry{
			{Round: 1, OpponentTeam: 7, WasHome: true, Minutes: 90, Starts: 1, TotalPoints: 6},
		},
	}
	d := BuildPlayerDetail(es, "MID", map[int]string{7: "CHE"})
	if len(d.Gameweeks) != 1 || d.Gameweeks[0].Opponent != "CHE" {
		t.Fatalf("Gameweeks = %+v, want opponent resolved to CHE", d.Gameweeks)
	}
	if !d.Gameweeks[0].Started {
		t.Errorf("Started = false for a row with Starts=1")
	}
}
