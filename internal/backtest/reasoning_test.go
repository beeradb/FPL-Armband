package backtest

// What did the model think, week by week, about specific players?
//
//	DIAG=1 FPL_TRACE="M.Salah,Haaland,Wissa" go test ./internal/backtest \
//	    -run TestDiagTrace -v -timeout 30m
//
// A season replay reports what the policy *did*. This reports what it *believed*
// at the moment it decided, which is the only way to tell a bad judgement from a
// slow one — and they need different fixes.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"armband/internal/analysis"
)

func TestDiagTrace(t *testing.T) {
	requireDiag(t)
	names := strings.Split(os.Getenv("FPL_TRACE"), ",")
	if len(names) == 0 || names[0] == "" {
		t.Skip("set FPL_TRACE=Name1,Name2")
	}
	season := os.Getenv("FPL_SEASON")
	if season == "" {
		season = "2025-26"
	}
	priorName := os.Getenv("FPL_PRIOR")
	if priorName == "" {
		priorName = "2024-25"
	}

	cfg := loadConfig(t)
	ctx := context.Background()
	prior, err := Load(ctx, cfg.CacheDir, priorName)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := Load(ctx, cfg.CacheDir, season)
	if err != nil {
		t.Fatal(err)
	}
	idx := newPriorIndex(prior)

	for _, raw := range names {
		want := strings.TrimSpace(raw)
		fmt.Printf("\n=== %s — what the model believed, and what happened\n\n", want)
		fmt.Printf("%-6s %8s %9s %9s %9s   %8s %8s\n",
			"decide", "score", "exp mins", "xG/90", "this-szn", "actual", "actual")
		fmt.Printf("%-6s %8s %9s %9s %9s   %8s %8s\n",
			"at gw", "", "", "", "weight", "mins", "pts")

		for cut := 1; cut <= 30; cut += 2 {
			boot, fx := PointInTime(cur, prior, cut)
			e := analysis.NewEngineFull(boot, fx, cfg.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = idx
			e.Recent = newRecentIndexWith(cur, cut,
				cfg.Weights.MinutesHalfLife, cfg.Weights.RateHalfLife)

			var found *analysis.PlayerMetrics
			for i := range boot.Elements {
				el := &boot.Elements[i]
				if el.WebName != want {
					continue
				}
				m := e.Metrics(el)
				found = &m
				break
			}
			if found == nil {
				continue
			}
			// What he actually did in the gameweek just decided for.
			var mins, pts int
			for _, p := range cur.Players {
				if p.WebName == want {
					if g, ok := p.GWs[cut+1]; ok {
						mins, pts = g.Minutes, g.Points
					}
					break
				}
			}
			fmt.Printf("%-6d %8.2f %9.1f %9.3f %9.2f   %8d %8d\n",
				cut, found.Score, found.ExpectedMinutes, found.XG90,
				found.PriorWeight, mins, pts)
		}
	}
	fmt.Printf("\n'this-szn weight' is how much of the estimate comes from this season\n")
	fmt.Printf("rather than the prior: n/(n+k) with BlendRateK, so it rises as evidence\n")
	fmt.Printf("accumulates. A score that stays high while actuals collapse means the\n")
	fmt.Printf("prior is still carrying it.\n")
}
