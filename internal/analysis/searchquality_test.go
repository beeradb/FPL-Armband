package analysis

import (
	"fmt"
	"os"
	"testing"
)

// Search-quality harness.
//
// TestNoPremiumSquadBeatsTheOptimum asks the right question — can the
// unconstrained search reach a squad that locking a premium reaches? — but it
// asks it about one landscape, whichever the live data happens to produce
// today. That is enough to catch a failure and not enough to measure one, and
// measuring is what a fix needs: a change that repairs the single landscape in
// front of you is indistinguishable from one that moves the failure elsewhere.
//
//	DIAG=1 go test ./internal/analysis -run TestDiagSearchQuality -v -timeout 3600s
//
// # Budget is the landscape generator, and the choice matters
//
// The obvious generator is to sweep a scoring weight, and the first version of
// this swept the post-tournament rest factor. That was close to useless: rest
// only moves a dozen flagged players, and once they are priced out of the squad
// the answer stops moving entirely — every factor from 0.70 to 0.78 returned a
// byte-identical optimum. A sweep like that reports whatever the one landscape
// it actually generated happens to do, dressed up as a rate.
//
// Budget genuinely reparameterises the knapsack: every £0.5m changes which
// combinations are affordable, right across the pool. Weight sweeps are kept
// alongside it for variety, not as the main source.
func TestDiagSearchQuality(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	report(t, "multi-downgrade funding ON", true)
}

func TestDiagSearchQualityBefore(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	report(t, "multi-downgrade funding OFF (baseline)", false)
}

type landscape struct {
	name   string
	budget int
	tweak  func(*Weights)
}

func landscapes() []landscape {
	var out []landscape
	for b := 950; b <= 1050; b += 10 {
		out = append(out, landscape{fmt.Sprintf("budget %.1f", float64(b)/10), b, nil})
	}
	for _, bw := range []float64{0.85, 1.15} {
		bw := bw
		out = append(out, landscape{
			fmt.Sprintf("bonus %.2f", bw), DefaultBudget,
			func(w *Weights) { w.BonusWeight = bw },
		})
	}
	for _, mw := range []float64{1.0, 1.5} {
		mw := mw
		out = append(out, landscape{
			fmt.Sprintf("minutes exp %.2f", mw), DefaultBudget,
			func(w *Weights) { w.MinutesWeight = mw },
		})
	}
	return out
}

func report(t *testing.T, label string, enabled bool) {
	was := fundedUpgradeEnabled
	fundedUpgradeEnabled = enabled
	defer func() { fundedUpgradeEnabled = was }()

	fmt.Printf("\n%s\n\n", label)
	fmt.Printf("  %-18s %10s %10s\n", "landscape", "optimum", "excess")

	var fails, n int
	var worst float64
	var worstAt string
	for _, ls := range landscapes() {
		w := DefaultWeights()
		if ls.tweak != nil {
			ls.tweak(&w)
		}
		e := roleEngine(t, w, DefaultRoleRisk())

		free, err := e.Optimize(OptimizeRequest{Budget: ls.budget})
		if err != nil {
			continue
		}
		best := objective(free.Players, e.Weights.BenchWeight, false)

		// Only the handful of most expensive players: those are the ones a local
		// search cannot walk to, and locking every £9m+ player turns a sweep into
		// an hour.
		gap := 0.0
		for _, m := range topPriced(e, 5) {
			got, err := e.Optimize(OptimizeRequest{Budget: ls.budget, LockIDs: []int{m.ID}})
			if err != nil {
				continue
			}
			if d := objective(got.Players, e.Weights.BenchWeight, false) - best; d > gap {
				gap = d
			}
		}
		n++
		flag := ""
		if gap > 1e-6 {
			fails++
			flag = "  <- FAIL"
		}
		if gap > worst {
			worst, worstAt = gap, ls.name
		}
		fmt.Printf("  %-18s %10.4f %+10.4f%s\n", ls.name, best, gap, flag)
	}
	fmt.Printf("\n  %d of %d landscapes fail (%.0f%%), worst gap %+.4f at %s\n",
		fails, n, 100*float64(fails)/float64(n), worst, worstAt)
}

func topPriced(e *Engine, n int) []PlayerMetrics {
	var out []PlayerMetrics
	for _, m := range e.AllMetrics() {
		if m.Score > 0 && m.Status == "available" {
			out = append(out, m)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Price > out[i].Price {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}
