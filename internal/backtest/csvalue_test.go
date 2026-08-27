package backtest

// What is the exposure effect on clean sheets worth, and who gets it?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCleanSheetExposure -v
//
// TestDiagCleanSheetSplit establishes that a clean sheet is a player event: FPL
// pays it for reaching 60 minutes without conceding *while on the pitch*, so a
// defender substituted at 70 minutes keeps it when his side concedes at 85.
//
// This sizes that in points and asks who collects it. The prediction is that it
// accrues to players who are routinely substituted — full-backs, in practice —
// and FPL's data cannot distinguish them from centre-backs, because both are DEF.
// So if the effect sorts by a player's typical minutes, that is a systematic,
// role-driven difference the model has no other channel for.
//
// The counterfactual is a TEAM-event model: whoever played the full match defines
// what the club "really" did that week, and anyone who beat it did so by leaving
// early. That is the model a club-level expected-goals-conceded rate implies, so
// the gap is exactly what such a change would throw away.

import (
	"fmt"
	"sort"
	"testing"
)

func TestDiagCleanSheetExposure(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	const csPoints = 4.0 // defenders and keepers

	fmt.Printf("\n=== What the exposure rule is worth, against a team-event counterfactual\n")
	fmt.Printf("Defenders and keepers, single-fixture gameweeks, 60+ minutes played.\n")
	fmt.Printf("The counterfactual is what a club-level rate implies: the outcome of the\n")
	fmt.Printf("team-mates who played the FULL match. Positive gain means the exposure rule\n")
	fmt.Printf("paid him more than a team-event model would have.\n\n")
	fmt.Printf("%-9s %9s %9s %9s %11s %11s\n",
		"season", "quals", "gained", "lost", "netPts/app", "netPts/38gw")

	type row struct{ typicalMins, gain float64 }
	var pooled []row

	// Named so the tercile header below counts this list rather than restating
	// its length in prose that nothing keeps honest.
	seasons := []string{"2022-23", "2023-24", "2024-25", "2025-26"}

	for _, name := range seasons {
		cur := loadSeason(t, cfg, name)

		// Typical minutes when he appears at all, as a proxy for role: a full-back
		// who is habitually withdrawn reads lower than a centre-back.
		typical := map[int]float64{}
		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			var mins, apps float64
			for _, g := range p.GWs {
				if g.Fixtures != 1 || g.Minutes == 0 {
					continue
				}
				mins += float64(g.Minutes)
				apps++
			}
			if apps >= 5 {
				typical[id] = mins / apps
			}
		}

		type qual struct {
			id   int
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
				if g.Fixtures != 1 || g.Minutes < 60 {
					continue
				}
				byTeamGW[[2]int{p.Team, gw}] = append(byTeamGW[[2]int{p.Team, gw}],
					qual{id: id, mins: g.Minutes, cs: g.CleanSheets})
			}
		}

		var quals, gained, lost int
		var net float64
		for _, qs := range byTeamGW {
			// Establish the club's outcome from the full-match players.
			full, fullCS := 0, 0
			for _, q := range qs {
				if q.mins >= 90 {
					full++
					if q.cs > 0 {
						fullCS++
					}
				}
			}
			// No full-match reference, or the reference disagrees with itself
			// (which cannot happen under the rule and would mean a data problem).
			if full == 0 || (fullCS != 0 && fullCS != full) {
				continue
			}
			teamCS := fullCS > 0
			for _, q := range qs {
				if q.mins >= 90 {
					continue // he IS the reference
				}
				quals++
				got := q.cs > 0
				switch {
				case got && !teamCS:
					gained++
					net += csPoints
					if tm, ok := typical[q.id]; ok {
						pooled = append(pooled, row{tm, csPoints})
					}
				case !got && teamCS:
					lost++
					net -= csPoints
					if tm, ok := typical[q.id]; ok {
						pooled = append(pooled, row{tm, -csPoints})
					}
				default:
					if tm, ok := typical[q.id]; ok {
						pooled = append(pooled, row{tm, 0})
					}
				}
			}
		}
		if quals == 0 {
			continue
		}
		perApp := net / float64(quals)
		fmt.Printf("%-9s %9d %9d %9d %11.4f %11.2f\n",
			name, quals, gained, lost, perApp, perApp*38)
	}

	// Does it sort by role? Terciles of typical minutes among the sub-90 group.
	sort.Slice(pooled, func(i, j int) bool { return pooled[i].typicalMins < pooled[j].typicalMins })
	fmt.Printf("\n=== Who collects it, by typical minutes when he appears\n")
	fmt.Printf("Sub-90 appearances only, %s pooled. If the gain sorts downward\n", seasonsLabel(len(seasons)))
	fmt.Printf("with typical minutes, it is a role effect FPL's position data cannot see.\n\n")
	fmt.Printf("%-22s %8s %10s %11s\n", "typical minutes", "n", "meanMins", "netPts/app")
	n := len(pooled)
	if n >= 30 {
		for i, lab := range []string{"lowest third", "middle third", "highest third"} {
			lo, hi := i*n/3, (i+1)*n/3
			seg := pooled[lo:hi]
			var mm, g float64
			for _, r := range seg {
				mm += r.typicalMins
				g += r.gain
			}
			fmt.Printf("%-22s %8d %10.1f %11.4f\n",
				lab, len(seg), mm/float64(len(seg)), g/float64(len(seg)))
		}
	}
	fmt.Printf("\nA club-level expected-goals-conceded rate would price every one of these\n")
	fmt.Printf("appearances at the team outcome, discarding whatever the netPts column holds.\n")
}
