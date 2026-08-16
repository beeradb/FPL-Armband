package backtest

import "testing"

// TestCapabilityFollowsTheDataNotTheSeasonName is the guard for A3's second defect,
// and it pins the property that forced `NoExpected` to split in two.
//
// # What it stops
//
// The obvious implementation of "does this season have expected goals" is a season
// name compared against a boundary year. That is wrong here, and wrong silently: the
// repairs *move* the boundary, so a name-keyed predicate would flag a repaired season
// as unmeasured and the blend would discard real figures — an absence invented from
// data rather than data lost to an absence.
//
// Reading the loaded totals instead makes the answer follow every switch with nothing
// here knowing they exist.
//
// # The state one flag could not express
//
// `rebuildXGAggregates` runs inside `applyXGRepair`; `rebuildXGCAggregates` runs only
// inside `applyXGCRepair`. So a season can hold **real xG beside a zero xGC**, and a
// single flag covering both must either discard the xG or blend the false xGC. The
// third case below is that state, constructed directly.
func TestCapabilityFollowsTheDataNotTheSeasonName(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		p                         Player
		wantXG, wantXGC, wantDefC bool
		why                       string
	}{{
		name:   "a season carrying everything",
		p:      Player{ID: 1, Minutes: 900, XG: 2.5, XGC: 9.5, DefCon: 40},
		wantXG: true, wantXGC: true, wantDefC: true,
		why: "all three totals are non-zero",
	}, {
		name: "an unrepaired old season",
		p:    Player{ID: 1, Minutes: 900},
		why:  "no total is non-zero, so nothing was measured",
	}, {
		name:   "xG repaired, xGC not — the state that forced the split",
		p:      Player{ID: 1, Minutes: 900, XG: 2.5},
		wantXG: true,
		why: "FPL_NO_XGC_REPAIR leaves real xG beside a zero xGC, and one flag " +
			"would have to discard the xG or blend the zero",
	}, {
		name:     "defcon only",
		p:        Player{ID: 1, Minutes: 900, DefCon: 40},
		wantDefC: true,
		why:      "the three statistics are independent and must answer independently",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Season{Name: "2021-22", Players: map[int]*Player{1: &tc.p}}
			if got := s.HasXG(); got != tc.wantXG {
				t.Errorf("HasXG = %v, want %v — %s", got, tc.wantXG, tc.why)
			}
			if got := s.HasXGC(); got != tc.wantXGC {
				t.Errorf("HasXGC = %v, want %v — %s", got, tc.wantXGC, tc.why)
			}
			if got := s.HasDefCon(); got != tc.wantDefC {
				t.Errorf("HasDefCon = %v, want %v — %s", got, tc.wantDefC, tc.why)
			}
		})
	}
}

// TestCapabilityIsAskedOfTheSeasonNotThePlayer pins the other half.
//
// Asked per player, "has expected goals conceded" is false for most forwards and
// "has expected goals" is false for most goalkeepers — so a per-player flag would
// make the blend discard exactly the statistic each position is priced on. The
// question is whether the FEED measured it, which is a fact about the season.
func TestCapabilityIsAskedOfTheSeasonNotThePlayer(t *testing.T) {
	// A forward with no defensive contribution and a keeper with no expected goals,
	// in a season that measured both.
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Type: 4, Minutes: 900, XG: 8.0, XGC: 0, DefCon: 0},
		2: {ID: 2, Type: 1, Minutes: 900, XG: 0, XGC: 30.0, DefCon: 55},
	}}
	if !s.HasXG() || !s.HasXGC() || !s.HasDefCon() {
		t.Errorf("HasXG=%v HasXGC=%v HasDefCon=%v — one non-zero total anywhere "+
			"means the season measured the statistic. A per-player answer would "+
			"strip the forward's defcon and the keeper's expected goals, which is "+
			"the data each is priced on",
			s.HasXG(), s.HasXGC(), s.HasDefCon())
	}
}
