package backtest

import (
	"fmt"
	"math"
	"math/rand"
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

// TestWildcardValueOverNextPricesTheLookahead pins the pure arithmetic of the
// lookahead pricing, which had NO coverage at all until the rule that reads it
// was wired.
//
// ⚠️ **Both `xiDriftSeries` and `wildcardValueOverNext` shipped uncalled.** A
// grep of the package at the merge of #94 found neither reachable from any test,
// diagnostic or production path — the tests in this file exercise `xiPoints`,
// `xiDriftOf` and `repairSquad` and stop short of both. They were described in
// that PR as "built, tested and unused"; only two of those three were true.
// These cases exist so the trigger reading them is standing on something.
func TestWildcardValueOverNextPricesTheLookahead(t *testing.T) {
	// A flat series with the repair already affordable: no hits either way, so
	// the value is only the drift still ahead, which can only fall as weeks are
	// spent suffering it. Waiting is never better, so the peak is now.
	t.Run("flat drift with a free repair peaks now", func(t *testing.T) {
		got := wildcardValueOverNext([]float64{2, 2, 2, 2}, 1, 1, 5)
		if got.PeakAt != 0 {
			t.Errorf("PeakAt = %d, want 0: every week waited spends drift that "+
				"cannot be recovered and buys no hit saving", got.PeakAt)
		}
		if got.Now != 8 {
			t.Errorf("Now = %v, want 8 — the whole series, with no hit cost to add", got.Now)
		}
		for i := 1; i < len(got.Value); i++ {
			if got.Value[i] > got.Value[i-1] {
				t.Fatalf("Value rose at k=%d on a flat series: %v", i, got.Value)
			}
		}
	})

	// ⚠️ **PeakAt IS DEGENERATE: it is 0 for every non-negative drift series, so a
	// rule gating on `PeakAt == 0` is inert by construction.** This case was
	// written expecting the opposite — a cliff in week 3 ought to be worth
	// waiting for — and it failed, which is how the degeneracy was found. It is
	// kept, inverted, as the pin.
	//
	// The arithmetic makes it unavoidable. `Value[k]` is
	// `sum(drift[k:]) + repairCostOf(changes, free+k)`, and BOTH terms are
	// non-increasing in k: remaining drift can only shrink as weeks are spent
	// suffering it, and the avoided hit cost can only fall as free transfers
	// accrue. A sum of two non-increasing terms is non-increasing, so the argmax
	// is always k=0. `[0 0 0 40]` with six changes reads 60, 56, 52, 48, 4.
	//
	// **This is not a bug in the arithmetic; it is a missing term.** With the
	// rebuild target held FIXED — one `fresh` squad optimised today and scored in
	// every future week — playing immediately is genuinely optimal, because
	// waiting spends drift to buy nothing. Real managers wait because the rebuild
	// they would do in three weeks is a DIFFERENT and better-informed squad aimed
	// at a different fixture run. That is term 3 in wildcardValueOverNext's own
	// doc comment, the one it says it cannot price — and without it the function
	// is not merely "systematically early" as that comment warns, it has no
	// timing content at all.
	//
	// Pricing it properly needs `fresh` re-optimised at each k, which is one
	// `Optimize` per lookahead week per decision — the expensive call in this
	// package, and the reason it was not simply done here.
	t.Run("PeakAt is degenerate: a future cliff still peaks now", func(t *testing.T) {
		got := wildcardValueOverNext([]float64{0, 0, 0, 40}, 6, 1, 5)
		if got.PeakAt != 0 {
			t.Fatalf("PeakAt = %d. If this now finds a future peak the degeneracy "+
				"has been fixed — check that a rule gating on PeakAt == 0 is still "+
				"the rule you want, and rewrite this test rather than deleting it: "+
				"%v", got.PeakAt, got.Value)
		}
	})

	// The general statement, so the degeneracy cannot be reintroduced quietly
	// after a fix or missed if the shape changes.
	//
	// ⚠️ **20,000 draws because that is the number the write-up quotes.** The
	// first version ran 5,000 while the note beside it claimed 20,000, taken from
	// a scratch test that was deleted — so the note's headline figure was not
	// reproducible from a checkout at all. A count in a record that no committed
	// code produces is not a measurement, and the cheap fix is to make the code
	// produce it rather than to quietly restate the claim smaller.
	t.Run("no non-negative series ever peaks in the future", func(t *testing.T) {
		r := rand.New(rand.NewSource(1))
		for i := 0; i < 20000; i++ {
			d := make([]float64, 1+r.Intn(8))
			for j := range d {
				d[j] = r.Float64() * 30
			}
			changes, free, cap := r.Intn(15), r.Intn(5), 1+r.Intn(5)
			if got := wildcardValueOverNext(d, changes, free, cap); got.PeakAt != 0 {
				t.Fatalf("PeakAt = %d on non-negative drift %v (changes %d, free %d, cap %d): %v",
					got.PeakAt, d, changes, free, cap, got.Value)
			}
		}
	})

	// The hit saving is what makes waiting pay, and it stops paying at the bank
	// cap: once the allowance stops accruing, waiting only spends drift. So a
	// peak can never sit past the week the cap is reached.
	t.Run("the hit saving stops at the bank cap", func(t *testing.T) {
		const free, cap = 1, 2
		got := wildcardValueOverNext([]float64{0, 0, 0, 0, 0, 0}, 6, free, cap)
		if got.PeakAt > cap-free {
			t.Errorf("PeakAt = %d, past the %d weeks it takes to reach the bank cap "+
				"of %d from %d free. Beyond that the allowance no longer grows, so "+
				"waiting buys nothing and the value must be flat, not rising: %v",
				got.PeakAt, cap-free, cap, free, got.Value)
		}
	})

	// Value has one entry per k in 0..len(drift) INCLUSIVE — playing after the
	// whole lookahead is a real option and reads zero drift remaining.
	t.Run("Value covers every k including one past the series", func(t *testing.T) {
		drift := []float64{1, 2, 3}
		got := wildcardValueOverNext(drift, 0, 0, 5)
		if len(got.Value) != len(drift)+1 {
			t.Errorf("len(Value) = %d, want %d", len(got.Value), len(drift)+1)
		}
		if got.Now != got.Value[0] {
			t.Errorf("Now = %v but Value[0] = %v; they are the same reading", got.Now, got.Value[0])
		}
	})

	// ⚠️ **The degeneracy is a property of the INPUT, not of the function**, and
	// this is the case that establishes that rather than leaving it assumed: a
	// series allowed to go negative CAN peak in the future, because a negative
	// week makes `sum(drift[k:])` rise as that week is passed.
	//
	// No frequency is asserted. How often it happens is entirely an artefact of
	// the draw distribution, and an earlier write-up quoted one seed's answer —
	// "6,735 of 20,000" — as though it described the football. It does not: a
	// negative week is the held eleven out-scoring the rebuilt one, which is
	// noise, and `TestDiagWildcardLookaheadValue` finds it in 0 of 36 real cells.
	t.Run("a negative week CAN produce a future peak", func(t *testing.T) {
		r := rand.New(rand.NewSource(2))
		var peaked int
		for i := 0; i < 20000; i++ {
			d := make([]float64, 1+r.Intn(8))
			for j := range d {
				d[j] = r.Float64()*30 - 10
			}
			if wildcardValueOverNext(d, r.Intn(15), r.Intn(5), 1+r.Intn(5)).PeakAt != 0 {
				peaked++
			}
		}
		if peaked == 0 {
			t.Error("no negative-drift series peaked in 20000 draws. Then the " +
				"degeneracy is a property of the FUNCTION rather than of " +
				"non-negative input, and the finding that reads it the other way " +
				"needs rewriting, not this test.")
		}
	})

	// A rule reading this must not fire on an empty lookahead, and the boundary
	// is silent rather than a panic today.
	t.Run("an empty series is a single zero-drift option", func(t *testing.T) {
		got := wildcardValueOverNext(nil, 3, 0, 5)
		if len(got.Value) != 1 || got.PeakAt != 0 {
			t.Errorf("nil drift gave Value %v PeakAt %d, want one entry at k=0", got.Value, got.PeakAt)
		}
	})
}
