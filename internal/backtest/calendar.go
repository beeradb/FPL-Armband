package backtest

import "armband/internal/fpl"

// The season's calendar, read from the fixture list.
//
// ⚠️ **Moved out of `calendar_diag_test.go` and into shipped code**, because
// `DoseFor` needs it and a non-test file cannot call a test one. Nothing about it
// changed; the diagnostic that used to own it now imports nothing new, because it
// is the same package. It is here rather than in `season.go` so the counting stays
// beside the thing that documents why it is counted from fixtures — see dose.go.

// teamGameweeks counts, for one season, how many matches each club plays in each
// gameweek. A club with two is doubling; a club absent from a played gameweek is
// blanking.
//
// Built from the fixture list rather than from players' GW rows, because a blank
// is precisely the case where no player row exists — counting rows would make a
// blank invisible, which is the bug the doubles fix had in the other direction.
func teamGameweeks(fx []fpl.Fixture) (played map[int]bool, count map[int]map[int]int, teams map[int]bool) {
	played = map[int]bool{}
	count = map[int]map[int]int{}
	teams = map[int]bool{}
	for _, f := range fx {
		if f.Event == nil {
			continue
		}
		gw := *f.Event
		played[gw] = true
		if count[gw] == nil {
			count[gw] = map[int]int{}
		}
		count[gw][f.TeamH]++
		count[gw][f.TeamA]++
		teams[f.TeamH] = true
		teams[f.TeamA] = true
	}
	return played, count, teams
}
