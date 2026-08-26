package backtest

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
)

// The claim this instrument exists to make: a bench change and a starter change
// are not the same distance, and the move count cannot tell them apart.
//
// It is built on a REAL squad rather than a synthetic one, because `BestXI` runs
// a formation search and a hand-made fifteen that cannot field a legal eleven
// would pass this test for the wrong reason.
func TestXIDriftSeparatesBenchFromStarters(t *testing.T) {
	cfg := loadConfig(t)
	pair := loadPairsOrSkip(t, cfg)
	if len(pair) == 0 {
		t.Skip("no seasons")
	}
	sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
	e, _ := EngineAt(pair[0].Cur, pair[0].Prior, 1, sc)

	fresh, ok := repairSquad(e, nil, 1000, 0, sc)
	if !ok || len(fresh) != 15 {
		t.Skip("optimiser could not build a squad")
	}

	// Rank the optimum's own fifteen, so "starter" and "bench" are this squad's
	// own ordering rather than an assumption about who is good.
	type sp struct {
		id    int
		score float64
	}
	var ranked []sp
	for _, id := range fresh {
		if el := e.Boot.ElementByID(id); el != nil {
			ranked = append(ranked, sp{id, e.Metrics(el).Score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	drop := func(id int) []int {
		out := make([]int, 0, 14)
		for _, x := range fresh {
			if x != id {
				out = append(out, x)
			}
		}
		return out
	}
	best, worst := ranked[0], ranked[len(ranked)-1]

	lostBest := xiPoints(e, drop(best.id))
	lostWorst := xiPoints(e, drop(worst.id))
	full := xiPoints(e, fresh)

	// Removing the best player must cost the eleven more than removing the worst.
	// Both are ONE change by changesBetween, which is the point.
	if full-lostBest <= full-lostWorst {
		t.Errorf("removing the best player (%.2f) cost the XI %.2f, removing the "+
			"worst (%.2f) cost %.2f — the measure is not separating them",
			best.score, full-lostBest, worst.score, full-lostWorst)
	}
	t.Logf("XI %.1f | without best (%.2f) %.1f, cost %.2f | without worst (%.2f) %.1f, cost %.2f "+
		"— both are ONE change by the old count",
		full, best.score, lostBest, full-lostBest, worst.score, lostWorst, full-lostWorst)
}

// A squad IS the optimum, so its drift must be exactly zero. This is what makes
// a non-zero reading mean something.
func TestXIDriftIsZeroAgainstItself(t *testing.T) {
	cfg := loadConfig(t)
	pair := loadPairsOrSkip(t, cfg)
	if len(pair) == 0 {
		t.Skip("no seasons")
	}
	sc := SimConfig{Weights: cfg.Weights, StartGW: 1}
	e, _ := EngineAt(pair[0].Cur, pair[0].Prior, 1, sc)

	fresh, ok := repairSquad(e, nil, 1000, 0, sc)
	if !ok {
		t.Skip("optimiser could not build a squad")
	}
	d, ok := xiDriftOf(e, fresh, 1000, 0, sc)
	if !ok {
		t.Fatal("no reading against the optimiser's own squad")
	}
	if d.Drift != 0 || d.Changes != 0 {
		t.Errorf("the optimum drifts from itself: %.4f points, %d changes", d.Drift, d.Changes)
	}
}

// What the two measures say about the same squads, on real data.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagXIDrift -v -timeout 30m
//
// ⚠️ NO INFERENCE HERE. Season-clustered SEs live in stats/sweep_inference.R and
// nowhere else — TestInferenceLivesInOnePlace enforces it. This prints the
// per-season row and the correlation between the two measures; the reading is
// the reader's.
func TestDiagXIDrift(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	starts := sweepStarts()

	fmt.Printf("\n=== how far is the opening squad from ideal, in POINTS on the XI?\n")
	fmt.Printf("Against the old move count over all fifteen, on the same cells.\n\n")
	fmt.Printf("%-9s %5s  %8s %8s %8s  %7s %9s\n",
		"season", "entry", "heldXI", "freshXI", "drift", "changes", "trigCost")

	var drifts, changes, costs []float64
	for _, pr := range loadPairsOrSkip(t, cfg) {
		for _, start := range starts {
			sc := SimConfig{Weights: cfg.Weights, StartGW: start}
			e, _ := EngineAt(pr.Cur, pr.Prior, start-1, sc)
			held, ok := repairSquad(e, nil, 1000, 0, sc)
			if !ok {
				continue
			}
			// Drift the squad deliberately: hold it at the entry cutoff, then ask
			// how far it is from the optimum FIVE gameweeks later. That gap is
			// what a manager actually accumulates.
			later, _ := EngineAt(pr.Cur, pr.Prior, start+4, sc)
			d, ok := xiDriftOf(later, held, 1000, 0, sc)
			if !ok {
				continue
			}
			// What the LIVE trigger would read for the same squad: the hit price
			// of repairing by transfers, 4 x max(0, changes - free). One free
			// transfer is the ordinary allowance and is what makes the two
			// measures comparable at all — see repairCostAndDrift.
			cost := repairCostOf(d.Changes, 1)
			drifts, changes = append(drifts, d.Drift), append(changes, float64(d.Changes))
			costs = append(costs, cost)
			fmt.Printf("%-9s %5d  %8.1f %8.1f %8.2f  %7d %9.1f\n",
				pr.Cur.Name, start, d.Held, d.Fresh, d.Drift, d.Changes, cost)
		}
	}
	if len(drifts) < 2 {
		t.Skip("not enough cells")
	}
	fmt.Printf("\ncells %d | mean drift %.2f points on the XI | mean changes %.2f of 15\n",
		len(drifts), meanOf(drifts), meanOf(changes))
	fmt.Printf("correlation between the two measures: %.3f\n", corrOf(drifts, changes))
	fmt.Printf("\nAgainst what the LIVE wildcard trigger actually reads:\n")
	fmt.Printf("  mean trigger cost %.1f points (hits) | correlation with XI drift %.3f\n",
		meanOf(costs), corrOf(drifts, costs))
	fmt.Printf("⚠️ These are DIFFERENT QUANTITIES, not two units for one number: the\n")
	fmt.Printf("trigger cost is a ONE-OFF hit price and the drift is a PER-GAMEWEEK\n")
	fmt.Printf("rate. ChipBarAt is calibrated against the first. The correlation says\n")
	fmt.Printf("how much the trigger's ranking would change, NOT what to swap in.\n")
	fmt.Printf("\n⚠️ Drift is an ARGMAX distance and is never zero against a fresh optimum.\n")
	fmt.Printf("Read the series or a control contrast, never one number as points left on the table.\n")
}

// corrOf is Pearson's r. Local to this diagnostic on purpose: it is a descriptive
// print, not inference, and putting it beside meanOf keeps the one-place rule
// about SEs and critical values intact.
func corrOf(a, b []float64) float64 {
	if len(a) != len(b) || len(a) < 2 {
		return 0
	}
	ma, mb := meanOf(a), meanOf(b)
	var num, da, db float64
	for i := range a {
		x, y := a[i]-ma, b[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0
	}
	return num / math.Sqrt(da*db)
}
