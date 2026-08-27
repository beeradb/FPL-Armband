package backtest

import (
	"testing"

	"armband/internal/analysis"
)

// TestDiagOptimizerDeterminismFactors gives the prevalence census its teeth and
// then isolates why it reads zero.
//
//	DIAG=1 FPL_UNREGISTERED_POOL=1 \
//	  go test ./internal/backtest -run TestDiagOptimizerDeterminismFactors -v -timeout 60m
//
// **The env var is required on current HEAD and that is the point.** 7baf5b4
// removed unregistered players from the pool, which changed the 2023-24 landscape
// enough that the recorded reproducer now returns one stable answer (48.344527)
// instead of two. FPL_UNREGISTERED_POOL restores the pool every recorded figure in
// AGENTS.md was measured on, so this runs against the landscape the record came
// from. Without it every arm below reads stable and the positive control fails,
// which is the test refusing to be read rather than a fix.
//
// TestDiagOptimizerDeterminismPrevalence finds 0 of 72 replay landscapes
// unstable, while TestDiagOptimizerIsNotDeterministic fails every time on
// 2023-24 pre-season. Those are the same season and the same entry point, so one
// of the two is measuring something other than what it claims — and a harness
// that reports a clean null without ever having been shown to detect the effect
// is the failure this package records as "a null result indistinguishable from a
// real one".
//
// The two calls differ in four things. This runs them one at a time from the
// recorded-defect end, so the arm that removes the instability names the cause.
// The first arm is the positive control and **must** come back unstable; the test
// fails if it does not, because everything after it is then uninterpretable.
func TestDiagOptimizerDeterminismFactors(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	prior := loadSeason(t, cfg, "2022-23")
	cur := loadSeason(t, cfg, "2023-24")
	sim := sweepConfig(cfg, 1, false)
	boot, fx := PointInTime(cur, prior, 0)

	const runs = 8

	// Each arm names one difference between the recorded reproducer and the way
	// Simulate actually builds its opening squad.
	arms := []struct {
		name        string
		priors      bool
		sweepWeight bool
		openingBW   bool
	}{
		{"reproducer: no priors, config weights, DefaultBenchWeight", false, false, false},
		{"+ priors (what Simulate sets)", true, false, false},
		{"+ sweep weights", false, true, false},
		{"+ opening bench weight", false, false, true},
		{"all three: what the replay actually runs", true, true, true},
	}

	unstable := map[string]bool{}
	for _, a := range arms {
		w := cfg.Weights
		if a.sweepWeight {
			w = sim.Weights
		}
		e := analysis.NewEngineFull(boot, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
		if a.priors {
			e.Priors = newPriorIndexMulti([]*Season{prior}, sim.PriorHalfLife)
		}
		bw := analysis.DefaultBenchWeight
		if a.openingBW {
			bw = sim.openingBenchWeight()
		}

		seen := map[float64]int{}
		for i := 0; i < runs; i++ {
			sq, err := e.Optimize(analysis.OptimizeRequest{
				MinMinutes: 600, MinExpectedMinutes: 55,
				BenchWeight: bw, Budget: 1000,
			})
			if err != nil {
				t.Fatalf("%s: %v", a.name, err)
			}
			seen[sq.XIScore]++
		}
		var lo, hi float64
		for s := range seen {
			if lo == 0 || s < lo {
				lo = s
			}
			if s > hi {
				hi = s
			}
		}
		unstable[a.name] = len(seen) > 1
		t.Logf("%-58s distinct=%d  spread=%.6f  %v", a.name, len(seen), hi-lo, seen)
	}

	if !unstable[arms[0].name] {
		t.Fatalf("POSITIVE CONTROL FAILED: the recorded reproducer came back stable, "+
			"so this harness cannot detect the defect and every other row above is "+
			"uninterpretable. Do not read the prevalence census until this passes.\n"+
			"arm: %s", arms[0].name)
	}
}
