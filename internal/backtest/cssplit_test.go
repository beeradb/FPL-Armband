package backtest

// Is a clean sheet a team event, or a player event?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCleanSheetSplit -v
//
// FPL awards a clean sheet for reaching 60 minutes without conceding *while on
// the pitch*. So a full-back substituted at 70 minutes with the score still 0-0
// keeps his four points when his side concedes at 85, and the centre-back who
// stayed on gets nothing. If that is right, two team-mates who both played 60+
// minutes in the same match can disagree about the clean sheet — and the ones who
// collected it should have played FEWER minutes, because leaving early is what
// protected them.
//
// This matters because a queued change proposed replacing each player's own
// expected goals conceded with a club rate, on the argument that a clean sheet is
// one team event and eleven different probabilities for it must be wrong. If the
// split below is common, that argument is wrong: the per-player figure is partly
// measuring a real difference in exposure, not noise about a shared quantity.
//
// Restricted to single-fixture gameweeks, because the archive's per-gameweek rows
// are totals across a club's fixtures that week and a double would make both
// minutes and clean sheets ambiguous.
//
// Restricted to defenders and keepers, for whom the clean sheet is 4 points and
// worth arguing about, rather than a midfielder's 1.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagCleanSheetSplit(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	fmt.Printf("\n=== Do team-mates who both played 60+ minutes disagree about a clean sheet?\n")
	fmt.Printf("Defenders and keepers only, single-fixture gameweeks only.\n")
	fmt.Printf("A split team-gameweek is one where some qualifiers got the clean sheet and\n")
	fmt.Printf("others did not. If FPL's rule is exposure-based, the ones who GOT it should\n")
	fmt.Printf("have played fewer minutes.\n\n")
	fmt.Printf("%-9s %8s %8s %7s %10s %10s %9s\n",
		"season", "teamGWs", "splits", "share", "minsWithCS", "minsNoCS", "diff")

	for _, name := range []string{"2022-23", "2023-24", "2024-25", "2025-26"} {
		cur := loadSeason(t, cfg, name)

		// team -> gw -> qualifying players
		type qual struct {
			mins int
			cs   int
		}
		byTeamGW := map[[2]int][]qual{}

		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			if p.Type > 2 {
				continue
			}
			for gw, g := range p.GWs {
				// Single fixture only, and he must have qualified on minutes.
				if g.Fixtures != 1 || g.Minutes < 60 {
					continue
				}
				k := [2]int{p.Team, gw}
				byTeamGW[k] = append(byTeamGW[k], qual{mins: g.Minutes, cs: g.CleanSheets})
			}
		}

		var teamGWs, splits int
		var mWith, mWithout float64
		var nWith, nWithout int
		for _, qs := range byTeamGW {
			if len(qs) < 2 {
				continue
			}
			teamGWs++
			var got, missed int
			for _, q := range qs {
				if q.cs > 0 {
					got++
				} else {
					missed++
				}
			}
			if got == 0 || missed == 0 {
				continue
			}
			splits++
			for _, q := range qs {
				if q.cs > 0 {
					mWith += float64(q.mins)
					nWith++
				} else {
					mWithout += float64(q.mins)
					nWithout++
				}
			}
		}
		if teamGWs == 0 {
			continue
		}
		aw, an := 0.0, 0.0
		if nWith > 0 {
			aw = mWith / float64(nWith)
		}
		if nWithout > 0 {
			an = mWithout / float64(nWithout)
		}
		fmt.Printf("%-9s %8d %8d %6.1f%% %10.1f %10.1f %9.1f\n",
			name, teamGWs, splits, 100*float64(splits)/float64(teamGWs), aw, an, aw-an)
	}

	fmt.Printf("\nA non-zero split share proves the clean sheet is a PLAYER event conditioned\n")
	fmt.Printf("on his own minutes window, not a team event. A negative diff proves the\n")
	fmt.Printf("mechanism: the team-mates who collected it are the ones who left early.\n")
}
