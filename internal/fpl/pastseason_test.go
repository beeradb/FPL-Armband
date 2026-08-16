package fpl

import "testing"

// TestPastSeasonKnowsWhichStatisticsItsFeedCarried pins the two boundaries in
// FPL's history_past, and the goalkeeper case that forbids the obvious shortcut.
//
// The values are from a live read on 2026-08-13 of players whose careers span
// both boundaries. They are not expected to change: FPL introduced the expected
// statistics in December 2022 and defensive contribution in 2024/25, and a
// completed season's aggregate does not move.
func TestPastSeasonKnowsWhichStatisticsItsFeedCarried(t *testing.T) {
	for _, c := range []struct {
		season           string
		expected, defcon bool
		note             string
	}{
		{"2017/18", false, false, "Dunk: 3420 minutes, all three expected fields '0.00'"},
		{"2018/19", false, false, "Dunk: 3151 minutes, '0.00'"},
		{"2021/22", false, false, "Gabriel: 3063 minutes, '0.00' — the last blind season"},
		{"2022/23", true, false, "Gabriel: xGC 41.84 — the expected statistics arrive"},
		{"2023/24", true, false, "Dunk: xGC 49.76, defcon still 0"},
		{"2024/25", true, true, "Dunk defcon 126, Gabriel 159 — defcon arrives"},
		{"2025/26", true, true, "Dunk defcon 231"},
	} {
		p := PastSeason{SeasonName: c.season}
		if got := p.HasExpected(); got != c.expected {
			t.Errorf("%s HasExpected = %v, want %v (%s)", c.season, got, c.expected, c.note)
		}
		if got := p.HasDefCon(); got != c.defcon {
			t.Errorf("%s HasDefCon = %v, want %v (%s)", c.season, got, c.defcon, c.note)
		}
	}

	// The reason the boundary is a season and not a zero test, and it is not
	// hypothetical: Pope recorded expected_goals of exactly 0.00 in 2022/23, a
	// season the data fully covers. A rule keyed on the value would discard a
	// real observation for every goalkeeper in the game.
	pope := PastSeason{SeasonName: "2022/23", Minutes: 3261, ExpectedGoals: 0}
	if !pope.HasExpected() {
		t.Error("a goalkeeper's genuine 0.00 expected goals in 2022/23 must still " +
			"count as measured; keying on the value instead of the season would " +
			"silently drop it")
	}

	// An unparseable label must answer "no data" rather than "all data": dropping
	// a season from statistics it cannot supply is recoverable, blending zeroes
	// into a rate is not.
	for _, bad := range []string{"", "n/a", "20"} {
		p := PastSeason{SeasonName: bad}
		if p.HasExpected() || p.HasDefCon() {
			t.Errorf("unparseable season %q must fail closed, got expected=%v defcon=%v",
				bad, p.HasExpected(), p.HasDefCon())
		}
	}
}
