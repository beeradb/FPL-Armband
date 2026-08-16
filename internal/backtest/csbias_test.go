package backtest

// Does the model under-rate defenders who are habitually substituted?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagSubstitutionBias -v
//
// Being withdrawn at 70 minutes cuts both ways for a player. He forgoes twenty
// minutes of chances to score, assist, make defensive contributions and earn
// bonus — and he locks in a clean sheet his team-mates can still lose. So the
// NET effect on the player is not one-directional.
//
// The MODEL's error might be. At metrics.go:1389 the score is
//
//	rate*minutes + thresholdPart*playsSixty(ExpectedMinutes) + perGW
//
// The rate terms are prorated by minutes, so the forgone attacking chances are
// already priced. The clean sheet sits in thresholdPart, scaled by the
// probability of REACHING sixty minutes — a qualification, not an exposure
// window. Nothing credits him for the minutes he was protected from.
//
// If that asymmetry matters, the model should under-predict points for regularly
// substituted defenders relative to ever-present ones. Keepers are the control:
// they are almost never withdrawn, so their terciles should show no such gradient.
//
// # Two design points that decide whether this measures anything
//
// The tercile is assigned from minutes BEFORE the cutoff and scored on gameweeks
// AFTER it. Bucketing on the whole season would condition on the outcome.
//
// And the population is restricted to established starters — 8+ appearances
// averaging 60+ minutes before the cutoff. Without that, the low tercile fills
// with fringe players and the comparison stops being about substitution at all.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagSubstitutionBias(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	const cut = 19

	type obs struct {
		typical float64 // mean minutes per appearance, before the cutoff
		score   float64 // what the model said, per gameweek
		actual  float64 // what he returned, per gameweek, after the cutoff
	}
	byPos := map[int][]obs{}

	// Named so the caveat printed under the table counts the grid that ran.
	pairs := xgPairNames()

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)

		boot, fx := PointInTime(cur, prior, cut)
		e := analysis.NewEngineFull(boot, fx, cfg.Weights,
			analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = idx
		e.Recent = newRecentIndexWith(cur, cut,
			cfg.Weights.MinutesHalfLife, cfg.Weights.RateHalfLife)

		for i := range boot.Elements {
			el := &boot.Elements[i]
			if el.ElementType > 2 { // keepers and defenders only
				continue
			}
			p := cur.Players[el.ID]
			if p == nil {
				continue
			}

			// Role, from before the cutoff only.
			var mins, apps float64
			for gw := 1; gw <= cut; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes == 0 || g.Fixtures != 1 {
					continue
				}
				mins += float64(g.Minutes)
				apps++
			}
			if apps < 8 {
				continue
			}
			typical := mins / apps
			if typical < 60 {
				continue // not an established starter; a different population
			}

			// What followed.
			var pts, weeks float64
			for gw := cut + 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok {
					continue // blank gameweek: his club did not play
				}
				pts += float64(g.Points)
				weeks++
			}
			if weeks < 8 {
				continue
			}
			byPos[el.ElementType] = append(byPos[el.ElementType], obs{
				typical: typical,
				score:   e.Metrics(el).Score,
				actual:  pts / weeks,
			})
		}
	}

	fmt.Printf("\n=== Does the model under-rate habitually substituted defenders?\n")
	fmt.Printf("Established starters only (8+ appearances averaging 60+ minutes before\n")
	// One sentence, one Printf. Split across two it read "Three" / "seasons with
	// expected goals", which no scan over string literals can see as a grid label
	// — and the label three lines below it counts the same population.
	fmt.Printf("GW%d), bucketed by those pre-cutoff minutes, scored on GW%d-38.\n", cut, cut+1)
	fmt.Printf("%s with expected goals in the prior season.\n", seasonsLabel(len(pairs)))
	fmt.Printf("bias = actual minus model, points per gameweek. POSITIVE means the model\n")
	fmt.Printf("under-rated him. Keepers are the control: they are not substituted, so\n")
	fmt.Printf("their gradient should be flat.\n\n")
	fmt.Printf("%-9s %-16s %5s %9s %8s %8s %8s\n",
		"position", "tercile", "n", "meanMins", "model", "actual", "bias")

	for _, pos := range []int{2, 1} {
		rows := byPos[pos]
		name := "DEF"
		if pos == 1 {
			name = "GKP (control)"
		}
		if len(rows) < 12 {
			fmt.Printf("%-9s too few rows (%d)\n", name, len(rows))
			continue
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].typical < rows[j].typical })
		n := len(rows)
		var biases []float64
		for i, lab := range []string{"most substituted", "middle", "most ever-present"} {
			lo, hi := i*n/3, (i+1)*n/3
			seg := rows[lo:hi]
			var mm, sc, ac float64
			for _, r := range seg {
				mm += r.typical
				sc += r.score
				ac += r.actual
			}
			k := float64(len(seg))
			bias := (ac - sc) / k
			biases = append(biases, bias)
			fmt.Printf("%-9s %-16s %5d %9.1f %8.3f %8.3f %+8.3f\n",
				name, lab, len(seg), mm/k, sc/k, ac/k, bias)
		}
		fmt.Printf("%-9s %-16s %5s %9s %8s %8s %+8.3f\n",
			name, "gradient", "", "", "", "", biases[0]-biases[2])
	}

	fmt.Printf("\nRead the gradient: positive means the most-substituted third is under-rated\n")
	fmt.Printf("relative to the most ever-present third. A defender gradient with a flat\n")
	fmt.Printf("keeper control is the signature of the missing exposure term. Note this is\n")
	fmt.Printf("one cutoff on %s and is confounded by anything else that travels\n", seasonsLabel(len(pairs)))
	fmt.Printf("with being withdrawn — squad quality, rotation, fitness.\n")
}
