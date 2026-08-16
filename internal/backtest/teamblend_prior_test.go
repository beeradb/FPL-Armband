package backtest

// Which k does the prior that actually ships want?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTeamBlendPriorSource -v
//
// TestDiagTeamBlend fitted teamConcededK=25 and teamScoredK=13 against a prior
// built from *last season's goals*. buildTeamRates does not use that prior — it
// cannot, because the FPL API publishes no team history — and uses
// priorFromStrength instead, FPL's own rating mapped to goals.
//
// So the two shipped constants were fitted on one prior and are applied to
// another. That is not obviously wrong, but it is not measured either, and k is
// precisely the weight given to the prior: a weaker prior wants a smaller k,
// because there is less worth holding on to. This runs the same out-of-sample
// design over both priors and reports the k each one wants.
//
// It also prints what the archive carries for team strength. The live payload
// pre-season has strength null and every granular rating zero — verified
// against the August 2026 bootstrap — so if the archive carries granular values
// it is a *later* snapshot, and the replay is exercising a different branch of
// priorFromStrength from the one that runs live in August.

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

func TestDiagTeamBlendPriorSource(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	pairs := [][2]string{
		{"2022-23", "2023-24"}, {"2023-24", "2024-25"}, {"2024-25", "2025-26"},
	}
	get := func(sn string) *Season { return loadSeason(t, cfg, sn) }

	type rec struct{ conceded, scored []float64 }
	perMatch := func(s *Season) map[string]*rec {
		name := shortNames(s)
		out := map[string]*rec{}
		for _, f := range s.Fixtures {
			if f.TeamHScore == nil || f.TeamAScore == nil || f.Event == nil {
				continue
			}
			h, a := name[f.TeamH], name[f.TeamA]
			if out[h] == nil {
				out[h] = &rec{}
			}
			if out[a] == nil {
				out[a] = &rec{}
			}
			out[h].conceded = append(out[h].conceded, float64(*f.TeamAScore))
			out[h].scored = append(out[h].scored, float64(*f.TeamHScore))
			out[a].conceded = append(out[a].conceded, float64(*f.TeamHScore))
			out[a].scored = append(out[a].scored, float64(*f.TeamAScore))
		}
		return out
	}

	// What does the archive actually carry? The live pre-season payload has all
	// of these at zero or null, so a granular value here is a later snapshot.
	fmt.Printf("\n=== what the archive carries for team strength ===\n\n")
	fmt.Printf("%-9s %-5s %8s %7s %7s %7s\n",
		"season", "club", "strength", "ovr_h", "def_h", "atk_h")
	for _, pair := range pairs {
		cur := get(pair[1])
		for i, tm := range cur.Teams {
			if i >= 3 {
				break
			}
			fmt.Printf("%-9s %-5s %8d %7d %7d %7d\n", cur.Name, tm.ShortName,
				tm.Strength, tm.StrengthOverallHome, tm.StrengthDefenceHome,
				tm.StrengthAttackHome)
		}
	}

	// strengthPrior indexes each season's rating-derived prior by club short
	// name, using the exported map so this cannot drift from what ships.
	//
	// coarse forces the branch that actually runs live in August. Verified
	// against the August 2026 bootstrap: pre-season every club has strength null
	// and all four granular ratings zero, so priorFromStrength falls through to
	// the 1-5 map. The archive carries granular values, so without this the
	// replay only ever measures a branch that cannot run at the GW1 deadline.
	strengthPrior := func(s *Season, coarse bool) map[string][2]float64 {
		out := map[string][2]float64{}
		for i := range s.Teams {
			tm := &s.Teams[i]
			if coarse {
				// Reproduce the live pre-season payload exactly: granular
				// ratings absent, the coarse 1-5 value present.
				stub := fpl.Team{
					ShortName: tm.ShortName,
					Strength:  tm.Strength,
				}
				c, sc := analysis.PriorFromStrength(&stub)
				out[tm.ShortName] = [2]float64{c, sc}
				continue
			}
			c, sc := analysis.PriorFromStrength(tm)
			out[tm.ShortName] = [2]float64{c, sc}
		}
		return out
	}

	cutoffs := []int{3, 5, 8, 11, 15, 20, 25}
	ks := []float64{0, 10, 20, 30, 45, 70, 100, 150, 250, 500}
	shipConceded, shipScored := analysis.TeamBlendK()

	for _, which := range []string{"conceded", "scored"} {
		for _, source := range []string{
			"last season's goals",
			"FPL strength rating (granular — replay only)",
			"FPL strength rating (coarse 1-5 — what runs in August)",
		} {
			fmt.Printf("\n=== goals %s, prior = %s: rms predicting the rest of the season\n\n",
				which, source)
			fmt.Printf("%-8s", "played")
			for _, k := range ks {
				label := fmt.Sprintf("k=%g", k)
				if k == 0 {
					label = "this only"
				}
				fmt.Printf("%10s", label)
			}
			fmt.Printf("%10s\n", "best k")

			var bestOverall []float64
			for _, n := range cutoffs {
				sse := make([]float64, len(ks))
				count := 0.0
				for _, pair := range pairs {
					prev, cur := get(pair[0]), get(pair[1])
					prevPM, curPM := perMatch(prev), perMatch(cur)
					sp := strengthPrior(cur, strings.Contains(source, "coarse"))
					for club, r := range curPM {
						series := r.conceded
						base := promotedConceded
						if which == "scored" {
							series = r.scored
							base = promotedScored
						}
						if len(series) < n+5 {
							continue
						}
						var prior float64
						if source == "last season's goals" {
							prior = base
							if q, ok := prevPM[club]; ok {
								if which == "conceded" {
									prior = meanOf(q.conceded)
								} else {
									prior = meanOf(q.scored)
								}
							}
						} else {
							v, ok := sp[club]
							if !ok {
								continue
							}
							prior = v[0]
							if which == "scored" {
								prior = v[1]
							}
						}
						soFar := meanOf(series[:n])
						rest := meanOf(series[n:])
						count++
						for i, k := range ks {
							w := float64(n) / (float64(n) + k)
							pred := w*soFar + (1-w)*prior
							sse[i] += (pred - rest) * (pred - rest)
						}
					}
				}
				if count == 0 {
					continue
				}
				bestI := 0
				for i := range sse {
					if sse[i] < sse[bestI] {
						bestI = i
					}
				}
				fmt.Printf("%-8d", n)
				for i := range ks {
					fmt.Printf("%10.4f", math.Sqrt(sse[i]/count))
				}
				fmt.Printf("%10g\n", ks[bestI])
				bestOverall = append(bestOverall, ks[bestI])
			}
			ship := shipConceded
			if which == "scored" {
				ship = shipScored
			}
			fmt.Printf("\nshipped k for %s is %g; best k by cutoff above: %v\n",
				which, ship, bestOverall)
		}
	}

	fmt.Printf("\nA prior is only worth the weight its accuracy earns. If the two prior\n")
	fmt.Printf("sources want different k, the shipped constant is fitted on the wrong one.\n")
}
