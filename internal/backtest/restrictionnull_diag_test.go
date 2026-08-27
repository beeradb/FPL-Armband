package backtest

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"testing"

	"armband/internal/stats"
)

// WHAT WOULD THE TOP-40 RANK CORRELATION BE IF THE MODEL HAD NO TAIL WEAKNESS?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagRestrictionNull -v -count=1 -timeout 90m
//
// # Why a null is not optional here
//
// Restricting to the forty highest-projected players in a gameweek attenuates
// rank correlation **by range restriction alone**, with no defect of any kind.
// The measured figure on that set is **0.140**, and without knowing what it
// SHOULD be, that number licenses nothing.
//
// ⚠️ **This project has already recorded and withdrawn one finding for exactly
// this omission.** An earlier pass read a fall from 0.498 to 0.28-0.39 on
// candidate sets as a 27-45% deficit; a truncation estimate put the no-defect
// expectation at 0.22-0.30, at or above the observation, and the finding
// evaporated. Worse, the null that pass PROPOSED would have manufactured it — it
// matched random subsets on the OUTCOME distribution while selection happens on
// the PREDICTOR, which is a milder truncation and therefore too lenient a null.
//
// # The construction, which replicates the SELECTION rather than the margins
//
// For each gameweek, with the real predictions and the real realised points:
//
//  1. Convert the GLOBAL Spearman correlation to a Gaussian one — Pearson
//     `r = 2·sin(π·ρ/6)` — so the synthetic pairs carry the dependence the model
//     actually has, uniformly across the whole distribution.
//  2. Draw n bivariate-normal pairs at that correlation.
//  3. Back-transform onto the EMPIRICAL margins: the synthetic prediction takes
//     the real prediction distribution's value at its own rank, and likewise for
//     points. Only the dependence STRUCTURE is synthetic.
//  4. Select the top 40 by synthetic prediction — the same rule the real set uses
//     — and compute Spearman on that subset.
//
// Because step 1 gives the synthetic data **uniform dependence with no tail
// weakness by construction**, whatever step 4 produces is what pure restriction
// costs. A real figure BELOW that band is evidence of a genuinely weaker tail; a
// figure inside it is nothing.
//
// ⚠️ **The margins are empirical on purpose.** FPL points are zero-inflated
// integers and nothing about them is Gaussian; imposing a normal margin would
// change the tie structure, which is what a rank statistic reads. Only the copula
// is parametric.
//
// ⚠️ **This is a null for the ORDERING, not for the policy.** The optimiser is a
// knapsack: mis-orderings between players who never compete for the same slot
// under a binding budget cost nothing. A rho below the band would be a real
// defect in the statistic and still might be worth no points.
//
// # RESULT, and why it does not settle the question
//
// The Gaussian null reads BELOW in **6 of 6** seasons — observed ~0.139 against a
// null mean of ~0.230. The empirical null reads below in **4 of 6**, one of those
// by 0.003 (0.178 against 0.181), with 2024-25 and 2025-26 inside. **The
// unanimity was partly the assumed dependence shape.**
//
// ⚠️ **Both nulls are flawed, in opposite directions, and neither is fixable
// here.** The Gaussian imposes a shape football may not have. The empirical pools
// pairs ACROSS weeks, so its synthetic top-40 can draw from high- and low-scoring
// weeks at once, which likely inflates it and makes the observation look worse.
//
// The obvious repair — resample WITHIN each week — does not work: resampling
// pairs preserves the joint distribution exactly, so it reproduces any tail
// weakness and returns the observed value trivially. **"No tail weakness" is not
// definable without assuming a dependence family**, and the verdict moves with
// the family. That is an identification limit, not a shortfall of effort.
//
// **So: suggestive, not established. Do not record a tail defect from this.** The
// instrument that can settle it is replay points, because the question that
// matters is whether a mis-ordering reaches a decision, and only the knapsack
// knows that.
func TestDiagRestrictionNull(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	const reps = 200
	const topN = 40

	fmt.Printf("\n=== WHAT RESTRICTION ALONE COSTS, AND WHAT WE MEASURE\n")
	fmt.Printf("Null: same selection rule, same margins, dependence uniform at the\n")
	fmt.Printf("model's own GLOBAL level — so any gap is tail weakness, not truncation.\n\n")
	fmt.Printf("%-9s %8s %9s %9s %9s %9s %9s\n",
		"season", "globalρ", "gauss p05", "gauss", "empir p05", "empir", "observed")

	for _, pr := range loadPairsOrSkip(t, cfg) {
		sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
		sc.Weights.Horizon = 1

		type wk struct{ pred, act []float64 }
		var weeks []wk
		var allP, allA []float64
		for gw := 1; gw <= 38; gw++ {
			ew, _ := EngineAt(pr.Cur, pr.Prior, gw-1, sc)
			var p, a []float64
			for id, pl := range pr.Cur.Players {
				g, ok := pl.GWs[gw]
				if !ok || g.Fixtures == 0 {
					continue
				}
				if el := ew.Boot.ElementByID(id); el != nil {
					p = append(p, ew.Metrics(el).Score)
					a = append(a, float64(g.Points))
				}
			}
			if len(p) > topN+10 {
				weeks = append(weeks, wk{p, a})
				allP, allA = append(allP, p...), append(allA, a...)
			}
		}
		if len(weeks) < 20 {
			continue
		}
		globalRho := stats.Spearman(allP, allA)
		// Spearman -> Gaussian copula correlation.
		r := 2 * math.Sin(math.Pi*globalRho/6)

		// Observed: the real top-40 by real prediction, per week.
		var obs []float64
		for _, w := range weeks {
			obs = append(obs, topRho(w.pred, w.act, topN))
		}
		observed := meanOf(obs)

		rng := rand.New(rand.NewSource(11))
		nulls := make([]float64, 0, reps)
		for rep := 0; rep < reps; rep++ {
			var per []float64
			for _, w := range weeks {
				n := len(w.pred)
				sp := append([]float64(nil), w.pred...)
				sa := append([]float64(nil), w.act...)
				sort.Float64s(sp)
				sort.Float64s(sa)
				z1 := make([]float64, n)
				z2 := make([]float64, n)
				for i := 0; i < n; i++ {
					a := rng.NormFloat64()
					z1[i] = a
					z2[i] = r*a + math.Sqrt(1-r*r)*rng.NormFloat64()
				}
				// Back-transform onto the empirical margins by rank.
				p := byRank(z1, sp)
				q := byRank(z2, sa)
				per = append(per, topRho(p, q, topN))
			}
			nulls = append(nulls, meanOf(per))
		}
		sort.Float64s(nulls)
		lo, mid := nulls[reps/20], meanOf(nulls)

		// ⚠️ **The assumption-free null.** The Gaussian copula above imposes a
		// dependence SHAPE; if football's real structure is thinner in the tail at
		// the same global correlation, part of any gap is that assumption rather
		// than the model. This one resamples whole (prediction, points) PAIRS from
		// the season's own pool, so it carries the empirical dependence including
		// whatever tail behaviour the data actually has, and destroys only the
		// week structure.
		//
		// Read the two together: below the Gaussian band and INSIDE the empirical
		// one would mean the tail weakness is a global property of the model
		// rather than something specific to the restricted set.
		var enulls []float64
		for rep := 0; rep < reps; rep++ {
			var per []float64
			for _, w := range weeks {
				n := len(w.pred)
				p := make([]float64, n)
				q := make([]float64, n)
				for i := 0; i < n; i++ {
					j := rng.Intn(len(allP))
					p[i], q[i] = allP[j], allA[j]
				}
				per = append(per, topRho(p, q, topN))
			}
			enulls = append(enulls, meanOf(per))
		}
		sort.Float64s(enulls)
		elo, emid := enulls[reps/20], meanOf(enulls)

		fmt.Printf("%-9s %8.3f %9.3f %9.3f %9.3f %9.3f %9.3f\n",
			pr.Name, globalRho, lo, mid, elo, emid, observed)
	}
	fmt.Printf("\n`gauss` imposes a dependence SHAPE at the measured global correlation.\n")
	fmt.Printf("`empir` resamples real pairs, so it carries the data's own tail structure\n")
	fmt.Printf("and assumes nothing. ⚠️ **Believe the empirical column** where they differ.\n")
	fmt.Printf("\n⚠️ BELOW the band is the only outcome that is evidence of a defect.\n")
	fmt.Printf("Inside it means the top-40 ordering is exactly as good as the model's\n")
	fmt.Printf("global dependence implies, and the low absolute number is truncation.\n")
	fmt.Printf("⚠️ Even BELOW would be a defect in the STATISTIC, not a cost in points —\n")
	fmt.Printf("the optimiser is a knapsack and mis-orderings across price tiers are free.\n")
}

// topRho is the rank correlation among the topN by the first vector.
func topRho(pred, act []float64, topN int) float64 {
	idx := make([]int, len(pred))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return pred[idx[a]] > pred[idx[b]] })
	if len(idx) > topN {
		idx = idx[:topN]
	}
	p := make([]float64, 0, len(idx))
	q := make([]float64, 0, len(idx))
	for _, i := range idx {
		p, q = append(p, pred[i]), append(q, act[i])
	}
	return stats.Spearman(p, q)
}

// byRank puts the sorted margin values in the order the latent draw ranks them,
// so the result has the margin's own distribution and the latent's dependence.
func byRank(z, sortedMargin []float64) []float64 {
	idx := make([]int, len(z))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return z[idx[a]] < z[idx[b]] })
	out := make([]float64, len(z))
	for r, i := range idx {
		out[i] = sortedMargin[r]
	}
	return out
}
