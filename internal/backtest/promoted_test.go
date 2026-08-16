package backtest

// What do we know about a promoted club before it kicks a ball?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagPromotedPrior -v
//
// A team-strength blend needs a prior, and a promoted club has no Premier
// League record to blend against — the team-level version of the problem
// shrinkToLeague solves for players. Three candidates, in increasing order of
// how much they claim:
//
//   - the league average, which says nothing about the club;
//   - the average of *promoted* clubs in earlier seasons, which is the base rate
//     for coming up and is measurable from the archive alone;
//   - FPL's own pre-season strength rating for that specific club, which is the
//     only club-specific thing anyone knows in August.
//
// The third is worth having only if it beats the second, and that is decidable.

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
)

func TestDiagPromotedPrior(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	ctx := context.Background()
	seasons := []string{"2022-23", "2023-24", "2024-25", "2025-26"}

	load := map[string]*Season{}
	for _, sn := range seasons {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		load[sn] = s
	}
	prevOf := map[string]string{
		"2023-24": "2022-23", "2024-25": "2023-24", "2025-26": "2024-25",
	}

	type obs struct {
		season, club       string
		promoted           bool
		strength, conceded float64
	}
	var all []obs

	for _, sn := range seasons {
		s := load[sn]
		// Goals conceded per match, from the fixtures actually played.
		gc, games := map[int]float64{}, map[int]float64{}
		for _, f := range s.Fixtures {
			if f.TeamHScore == nil || f.TeamAScore == nil {
				continue
			}
			gc[f.TeamH] += float64(*f.TeamAScore)
			gc[f.TeamA] += float64(*f.TeamHScore)
			games[f.TeamH]++
			games[f.TeamA]++
		}
		prevClubs := map[string]bool{}
		if p, ok := load[prevOf[sn]]; ok {
			for _, tm := range p.Teams {
				prevClubs[tm.ShortName] = true
			}
		}
		for _, tm := range s.Teams {
			if games[tm.ID] == 0 {
				continue
			}
			promoted := len(prevClubs) > 0 && !prevClubs[tm.ShortName]
			all = append(all, obs{
				season: sn, club: tm.ShortName, promoted: promoted,
				// FPL rates defence higher = better, so a weaker defence is a
				// lower number. Averaged over home and away.
				strength: float64(tm.StrengthDefenceHome+tm.StrengthDefenceAway) / 2,
				conceded: gc[tm.ID] / games[tm.ID],
			})
		}
	}

	fmt.Printf("\nFPL's pre-season defensive strength against goals actually conceded.\n\n")
	var pro, est []obs
	for _, o := range all {
		if o.promoted {
			pro = append(pro, o)
		} else if o.season != "2022-23" {
			est = append(est, o)
		}
	}
	mean := func(xs []obs, f func(obs) float64) float64 {
		if len(xs) == 0 {
			return 0
		}
		var s float64
		for _, x := range xs {
			s += f(x)
		}
		return s / float64(len(xs))
	}
	conc := func(o obs) float64 { return o.conceded }
	str := func(o obs) float64 { return o.strength }

	fmt.Printf("promoted clubs   n=%2d  mean strength %6.0f  conceded %.2f/match\n",
		len(pro), mean(pro, str), mean(pro, conc))
	fmt.Printf("established      n=%2d  mean strength %6.0f  conceded %.2f/match\n",
		len(est), mean(est, str), mean(est, conc))

	fmt.Printf("\nEvery promoted club, ordered by FPL's rating of it:\n\n")
	sort.Slice(pro, func(i, j int) bool { return pro[i].strength > pro[j].strength })
	for _, o := range pro {
		fmt.Printf("  %-8s %-5s strength %5.0f   conceded %.2f\n",
			o.season, o.club, o.strength, o.conceded)
	}

	// Does FPL's club-specific rating beat "all promoted clubs are the same"?
	base := mean(pro, conc)
	var sseBase, sseFit float64
	// Fit conceded = a + b*strength on the promoted cohort only.
	mx, my := mean(pro, str), base
	var num, den float64
	for _, o := range pro {
		num += (o.strength - mx) * (o.conceded - my)
		den += (o.strength - mx) * (o.strength - mx)
	}
	b := 0.0
	if den > 0 {
		b = num / den
	}
	a := my - b*mx
	for _, o := range pro {
		sseBase += (o.conceded - base) * (o.conceded - base)
		p := a + b*o.strength
		sseFit += (o.conceded - p) * (o.conceded - p)
	}
	fmt.Printf("\ncohort average alone:      rms %.3f goals/match\n",
		math.Sqrt(sseBase/float64(len(pro))))
	fmt.Printf("FPL strength, fitted:      rms %.3f  (R2 %.3f, slope %+.5f per point)\n",
		math.Sqrt(sseFit/float64(len(pro))), 1-sseFit/sseBase, b)
	fmt.Printf("\nn is %d promoted clubs, so this is suggestive at best.\n", len(pro))
}
