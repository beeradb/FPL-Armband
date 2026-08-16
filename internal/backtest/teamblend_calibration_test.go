package backtest

// When does this season's evidence about a club beat last season's?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTeamBlend -v
//
// Fixture difficulty is FPL's integer 1-5, set pre-season and barely moved. Any
// attempt to replace it with something proportional to how bad a defence
// actually is needs a *ratio worth believing*, and over three September matches
// "this side concedes twice the median" is mostly noise. So the blend comes
// first: how much weight does a club's record this season deserve against its
// record last season, as a function of matches played?
//
// This is the team-level version of BlendRateK, and it is measured the same way
// — out of sample, predicting what has not happened yet. For each club and each
// cutoff, predict the *remainder* of the season from the record up to that
// point, from last season alone, and from a blend at each k. The k that
// minimises error is the answer, and the cutoff where k stops helping is the
// point at which this season can be trusted on its own.
//
// Deliberately not measured on the replay. The replay could not resolve
// BlendRateK either, and AGENTS.md is explicit that predictive calibration is
// what discriminates where a points sweep cannot.

import (
	"fmt"
	"math"
	"os"
	"testing"
)

// promotedConceded and promotedScored are the base rates for a club with no
// Premier League record, measured in TestDiagPromotedPrior: promoted sides
// concede 2.03 a match against 1.40 for established ones.
const (
	promotedConceded = 2.03
	promotedScored   = 1.05
)

func TestDiagTeamBlend(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	// Derived from a requirement rather than pasted: consecutive pairs whose
	// played season carries FPL's own expected goals. That is three pairs today
	// and becomes more if a season is added, without an edit here.
	pairs := gridFor(needsSweep)[1:]
	get := func(sn string) *Season { return loadSeason(t, cfg, sn) }

	// perMatch returns each club's goals conceded and scored per match, by
	// gameweek, keyed on short name so it survives the season boundary.
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

	cutoffs := []int{3, 5, 8, 11, 15, 20, 25}
	ks := []float64{0, 5, 10, 15, 20, 30, 45, 70, 100}

	for _, which := range []string{"conceded", "scored"} {
		fmt.Printf("\n=== goals %s per match: rms error predicting the rest of the season\n\n", which)
		fmt.Printf("%-8s", "played")
		for _, k := range ks {
			label := fmt.Sprintf("k=%g", k)
			if k == 0 {
				label = "this only"
			}
			if k == 100 {
				label = "prior~only"
			}
			fmt.Printf("%11s", label)
		}
		fmt.Printf("%11s\n", "best k")

		for _, n := range cutoffs {
			sse := make([]float64, len(ks))
			count := 0.0
			for _, pair := range pairs {
				prev, cur := get(pair[0]), get(pair[1])
				prevPM, curPM := perMatch(prev), perMatch(cur)
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
					prior := base
					if q, ok := prevPM[club]; ok {
						if which == "conceded" {
							prior = meanOf(q.conceded)
						} else {
							prior = meanOf(q.scored)
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
				fmt.Printf("%11.4f", math.Sqrt(sse[i]/count))
			}
			fmt.Printf("%11g\n", ks[bestI])

			// Three of the nine columns go to the accuracy snapshot: believing this
			// season outright, believing last season outright, and the best blend
			// available. That is the whole shape of the finding — the two extremes
			// bracket the blend — without carrying a nine-column table into a
			// summary document.
			//
			// This is the only *predictor comparison* the project can currently
			// run: two estimators of the same future quantity, scored out of
			// sample. There is no equivalent for player minutes, points or expected
			// goals — those MAE figures are frozen prose in internal/analysis and
			// nothing recomputes them, which the snapshot says rather than papers
			// over.
			rms := func(i int) float64 { return math.Sqrt(sse[i] / count) }
			sink.emitAll("team_rate_predictors",
				fmt.Sprintf("%d season pairs, predicting the rest of the season",
					len(pairs)),
				fmt.Sprintf("goals %s, after %d matches played", which, n),
				int(count),
				measure{"this season only (error)", rms(0)},
				measure{"last season only (error)", rms(len(ks) - 1)},
				measure{"best blend of the two (error)", rms(bestI)},
				measure{"prior strength that blend used, in matches", ks[bestI]})
		}
	}
	fmt.Printf("\n'this only' is k=0, believing the season to date outright.\n")
	fmt.Printf("'prior~only' is k=100, which at these cutoffs is last season alone.\n")
	fmt.Printf("Promoted clubs take the measured base rate as their prior: %.2f conceded, %.2f scored.\n",
		promotedConceded, promotedScored)
}
