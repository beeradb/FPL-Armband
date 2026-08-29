// Sizes defensive club concentration in the replay BEFORE anything is built to
// constrain it.
//
// # Why this runs first
//
// The proposal is a cap on goalkeepers-plus-defenders from one club, on the
// argument that a clean sheet is all-or-nothing and shared, so three defensive
// units at one club ride a single result. Building that cap means teaching the
// optimiser's feasibility bound, its DP seed and its pair scan about a second
// counter, and a narrowing constraint that the bound does not know about is
// optimistic — it can claim a fill is reachable when it is not.
//
// That is a lot of search surface to disturb for an effect whose size nobody has
// measured. If the shipped optimiser rarely stacks a defence, the cap cannot be
// worth the 70-point threshold this record uses and the line closes here for the
// price of one diagnostic.
//
// ⚠️ **This measures EXPOSURE, not cost.** It reports how often the concentration
// occurs and what those units scored, and it deliberately does not compare
// against a capped arm — there is no capped arm yet. A reader who takes the
// points columns as the value of a cap has read a frequency table as an effect
// size.
//
// ⚠️ **Bonus is not part of this.** Bonus is paid regardless of the result and is
// negatively covariant among teammates competing for one pool, so it carries the
// opposite correlation structure to a clean sheet and pooling the two is what an
// earlier framing of this arm got wrong. Only GKP and DEF are counted.
package backtest

import (
	"fmt"
	"sort"
	"testing"
)

// defensive positions in the archive's own element_type encoding.
const (
	typeGKP = 1
	typeDEF = 2
)

func TestDiagDefensiveClubConcentration(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type bucket struct {
		weeks     int
		unitPts   int
		unitCount int
		blanked   int // defensive units at a stacked club scoring <= 2 that week
	}
	// keyed by the maximum number of defensive units held at any one club
	overall := map[int]*bucket{}
	seasonMax := map[string]map[int]int{}

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		res, err := Simulate(cur, prior, sweepConfig(cfg, 1, false))
		if err != nil {
			t.Fatalf("%s: %v", pair[1], err)
		}
		seasonMax[pair[1]] = map[int]int{}

		for _, w := range res.Weeks {
			// The XI, not the fifteen. A benched defender shares the club's
			// result too, but he is not being paid for it either way, so
			// counting him would inflate the exposure this arm is about.
			perClub := map[int][]int{}
			for _, id := range w.XI {
				p, ok := cur.Players[id]
				if !ok || (p.Type != typeGKP && p.Type != typeDEF) {
					continue
				}
				perClub[p.Team] = append(perClub[p.Team], id)
			}
			maxUnits, maxClub := 0, 0
			for club, ids := range perClub {
				if len(ids) > maxUnits || (len(ids) == maxUnits && club < maxClub) {
					maxUnits, maxClub = len(ids), club
				}
			}
			b := overall[maxUnits]
			if b == nil {
				b = &bucket{}
				overall[maxUnits] = b
			}
			b.weeks++
			seasonMax[pair[1]][maxUnits]++
			for _, id := range perClub[maxClub] {
				g, ok := cur.Players[id].GWs[w.GW]
				if !ok {
					continue
				}
				b.unitCount++
				b.unitPts += g.Points
				if g.Points <= 2 {
					b.blanked++
				}
			}
		}
	}

	fmt.Println("\nDEFENSIVE CLUB CONCENTRATION IN THE REPLAY")
	fmt.Println("Most GKP+DEF the shipped optimiser held at ONE club, per started XI.")
	fmt.Println("Points are that club's stacked units only, in that gameweek.")
	fmt.Printf("\n  %5s %8s %8s %10s %10s\n", "units", "weeks", "share", "pts/unit", "<=2 pts")
	var keys []int
	total := 0
	for k, b := range overall {
		keys = append(keys, k)
		total += b.weeks
	}
	sort.Ints(keys)
	for _, k := range keys {
		b := overall[k]
		per, blank := 0.0, 0.0
		if b.unitCount > 0 {
			per = float64(b.unitPts) / float64(b.unitCount)
			blank = 100 * float64(b.blanked) / float64(b.unitCount)
		}
		fmt.Printf("  %5d %8d %7.1f%% %10.2f %9.1f%%\n",
			k, b.weeks, 100*float64(b.weeks)/float64(total), per, blank)
	}

	fmt.Printf("\n  per season, share of gameweeks at each maximum:\n")
	var seasons []string
	for s := range seasonMax {
		seasons = append(seasons, s)
	}
	sort.Strings(seasons)
	for _, s := range seasons {
		n := 0
		for _, c := range seasonMax[s] {
			n += c
		}
		fmt.Printf("    %-9s", s)
		for _, k := range keys {
			fmt.Printf(" %d:%4.0f%%", k, 100*float64(seasonMax[s][k])/float64(n))
		}
		fmt.Println()
	}

	// The decision this diagnostic exists to inform, stated as arithmetic rather
	// than left to the reader. Three units at one club is the shape that burned
	// GW1 2026-27; if the replay almost never reaches it, a cap on it cannot pay.
	three := 0
	if b := overall[3]; b != nil {
		three = b.weeks
	}
	fmt.Printf("\n  three-or-more at one club: %d of %d gameweeks (%.1f%%)\n",
		three, total, 100*float64(three)/float64(total))
	fmt.Println("\n  ⚠️ Exposure only. No capped arm has been run, so no column here is the")
	fmt.Println("  value of a cap. See the work note before quoting any of it.")
}
