package backtest

import (
	"testing"

	"armband/internal/analysis"
)

// TestDiagOptimizerIsNotDeterministic records a defect found while measuring something
// else, and it contradicts a claim this project relies on everywhere.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagOptimizerIsNotDeterministic -v
//
// # The claim it refutes
//
// AGENTS.md states, in the definition of the jitter floor and again in the noise
// section, that "the replay is deterministic: the same inputs always give the same
// output" and that its jitter is *sensitivity* rather than randomness. **That is false.**
// `Optimize` called repeatedly on **one engine**, with byte-identical inputs, returns two
// different fifteens with different objective values — measured at 48.643364 and
// 48.206244 on the 2023-24 pre-season pool, a spread of 0.44 in `XIScore`.
//
// # Why it matters more than its size
//
// 0.44 pts/gw is about 17 points a season, which is inside the recorded noise floor and
// therefore easy to dismiss. The reason not to is structural:
//
//   - **It is not float reassociation.** The season parses are identical to twelve
//     decimal places across runs, and the two answers differ by 0.44 rather than by
//     1e-13. One of the two is simply a worse squad, so this is search quality, not
//     rounding.
//   - **Every invariance check in the record is weakened by it.** "HOLD is byte-identical
//     across all six settings" is the strongest form of evidence this project uses, and
//     it is only sound if a rerun reproduces. Some cells do reproduce and some do not,
//     which means an invariance that held may have held by luck.
//   - **It is a component of the jitter floor that nobody has separated.** The floor is
//     attributed entirely to discreteness, and part of it is this.
//
// # What is known and what is not
//
// Known: it is inside `analysis.Optimize`, it survives `FPL_NO_FUNDED_UPGRADE=1`, and it
// does not come from the data or the engine build. Not known: which stage. The greedy
// seed sorts its pool, `squadSlice` sorts by id, `frontier` sorts by price then score
// stably, and there is no `rand`, no `sync.Pool`, no goroutine and no time limit in the
// package — so the remaining candidate is a map iteration whose order reaches a
// comparison, which is the defect class the clean-sheet diagnostic already hit.
//
// # Why it is not fixed here
//
// It is the optimiser. Making it deterministic changes which squad every cell in the
// record was built from, so it is a measurement pass of its own and not a side effect of
// an archive change — the same call the `starts` reconstruction and the duplicate-row
// defect both made. DIAG-gated so it does not fail the build; it fails **within** DIAG,
// so anyone running the diagnostics sees it.
func TestDiagOptimizerIsNotDeterministic(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	prior := loadSeason(t, cfg, "2022-23")
	cur := loadSeason(t, cfg, "2023-24")

	boot, fx := PointInTime(cur, prior, 0)
	e := analysis.NewEngineFull(boot, fx, cfg.Weights, analysis.Congestion{},
		analysis.RoleRisk{})

	const runs = 6
	seen := map[float64]int{}
	for i := 0; i < runs; i++ {
		sq, err := e.Optimize(analysis.OptimizeRequest{
			MinMinutes: 600, MinExpectedMinutes: 55,
			BenchWeight: analysis.DefaultBenchWeight, Budget: 1000,
		})
		if err != nil {
			t.Fatal(err)
		}
		var ids []int
		for _, p := range sq.Players {
			ids = append(ids, p.ID)
		}
		seen[sq.XIScore]++
		t.Logf("run %d: XIScore %.6f  squad %v", i, sq.XIScore, ids)
	}
	if len(seen) > 1 {
		var lo, hi float64
		for s := range seen {
			if lo == 0 || s < lo {
				lo = s
			}
			if s > hi {
				hi = s
			}
		}
		t.Errorf("Optimize returned %d distinct answers in %d identical calls on one "+
			"engine, XIScore %.6f to %.6f (spread %.6f): %v.\n"+
			"This is the recorded defect, not a new one — see the note on this test. "+
			"The replay is NOT deterministic, so an invariance that reads "+
			"byte-identical may have read so by luck, and part of the jitter floor is "+
			"this rather than discreteness.",
			len(seen), runs, lo, hi, hi-lo, seen)
	}
}
