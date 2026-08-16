package backtest

// Is exp(-xGC) the right clean-sheet probability?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCleanSheet -v
//
// baseXP90 prices the clean sheet as math.Exp(-m.XGC90) times the position's
// points — a Poisson on expected goals conceded, evaluated at zero. It is a
// bigger term than the defensive contribution, worth 26-45% of a defender's or
// keeper's score, and unlike defcon it has never been checked.
//
// The check is direct. Take every match a player finished, treat his
// expected_goals_conceded for that match as the team's, and compare the clean
// sheets that actually happened against the ones the model predicts. Two things
// can go wrong and they are separable:
//
//   - Poisson can be the wrong shape. Team goals conceded are plausibly
//     overdispersed, and fatter tails push P(0) *up*, not down, so the sign is
//     not obvious in advance.
//   - xGC can be biased. If it systematically under- or over-states goals, the
//     clean-sheet rate is wrong even with a perfect distribution.
//
// Reporting the mean predicted rate against the mean actual rate separates
// them: bucketing by predicted rate shows the shape, and the pooled totals show
// the bias.
//
// ⚠️ **What this scores is not what the model scores (2026-08-15).** It uses each
// match's realised expected_goals_conceded as the regressor; baseXP90 scores
// m.XGC90 — blended toward a prior season, shrunk, point-in-time. The aggregate
// over-prediction differs between the two, and it does: 1.281 here against 1.052
// on native XGC90 rows.
//
// ⚠️ **The over-prediction here has TWO mechanisms and neither replaces the
// other.** CROSS-MATCH CONVEXITY explains the gap between this regressor and
// XGC90 — exp() is convex, so E[exp(-x)]/exp(-mean x) grows with the dispersion
// of x, and realised match xGC (sd 0.848) is far more dispersed than XGC90 — and
// it predicts that gap to 0.3%. A SHOT-LEVEL Jensen gap explains why `exp(-x)`
// over-predicts on realised x at all: `exp(-Sum x_i)` exceeds
// `Prod(1-x_i) ~= exp(-c*Sum x_i)` with c > 1.
//
// ⚠️ **The shot-level wedge's SIZE is not established here.** Solving
// `mean(exp(-c*x)) = observed rate` on these rows returns c = 1.283, but that fit
// is exactly identified — one parameter, one moment — so it reproduces that
// moment by construction and cannot test any mechanism. stats/xg_provider_scale.py's
// c = 1.27 is a different season on a different feed (2015/16, 175 of 380 shared
// fixtures, and FPL uses Opta which that script says it does not identify);
// season-matched the comparable figure is 1.3291.
//
// **The 30.5%/23.8% this file produces is a property of THIS regressor**, and it is the number quoted in AGENTS.md,
// docs/accuracy.md, the snapshot registry and sweep.go. It stays as the
// shape-and-bias decomposition it was written for; every quote of its SIZE must
// name the regressor. TestDiagCleanSheetRegressor is the pair.

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"testing"
)

func TestDiagCleanSheetPoisson(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	// Per-observation rows for R, because the bucket-mean fit below CANNOT
	// carry a verdict and has twice been read as though it could.
	//
	// Binning first and regressing the six bucket means is biased toward the
	// offset family: exp is convex, so the mean of exp is not exp of the mean,
	// and backing an effective x out of a bucket mean inherits that. Measured
	// on the same rows, the bucket-mean estimator moves the fit by about
	// a +0.10, b -0.09 against the per-observation MLE ON NATIVE xGC ROWS
	// (FPL_NO_XGC_REPAIR=1), which is the population that carries the verdict,
	// and by about half that on an unswitched dump -- the same size as the
	// intercept it is being used to establish. Name the data state beside either.
	//
	// So the buckets stay, for display and for reading a shape off, and the
	// inference goes to stats/cs_calibration.R, which fits the binomial GLM
	// with a log link -- under which -ln(p) = a + b*x is the model exactly.
	// This is the project's standing division: Go for the engine, R for the
	// inference, CSV as the contract.
	rows := newRowDump(os.Getenv("FPL_CS_ROWS"), t.Fatalf,
		"season", "gw", "team", "xgc", "goals_conceded", "clean_sheet")
	defer rows.close()

	type bucket struct {
		lo, hi     float64
		n          float64
		pred, act  float64
		xgc, goals float64
	}
	buckets := []*bucket{
		{lo: 0.0, hi: 0.7}, {lo: 0.7, hi: 1.0}, {lo: 1.0, hi: 1.3},
		{lo: 1.3, hi: 1.6}, {lo: 1.6, hi: 2.0}, {lo: 2.0, hi: 99},
	}
	var n, pred, act, sumXGC, sumGoals float64

	// Named so every label below counts the population that ran. needsSweep
	// admits a season with FPL's own expected goals and a comparable transfer
	// path, so this list grows by one every summer and a literal in the header —
	// or worse, in the snapshot's grid string — would go stale silently.
	seasons := playedSeasons(needsSweep)

	for _, sn := range seasons {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		// One observation per team-match, not per player: eleven team-mates
		// share a clean sheet, and counting them separately would multiply the
		// apparent sample by eleven while adding no information at all.
		//
		// **Players are visited in id order, and that is load-bearing.** s.Players is
		// a map, Go randomises map iteration, and the dedup below keeps whichever
		// player it reached first — so the *representative* of each team-match varied
		// from run to run. Team-mates do not always agree: a substitute who played 90
		// minutes across two fixtures of a double gameweek carries a different
		// accumulated xGC from a starter who played one. Unfixed, this diagnostic
		// returned pooled figures differing by 0.7% between identical runs, which
		// would make every accuracy snapshot report a movement and train its reader
		// to ignore the diff. TestModelDiagnosticsAreReproducible pins it.
		seen := map[[2]int]bool{}
		for _, id := range sortedPlayerIDs(s) {
			p := s.Players[id]
			if p.Type == 4 {
				continue // forwards record no clean sheet
			}
			// Gameweeks in order, for the same reason players are. Ranging
			// the map leaves the AGGREGATES right — addition commutes — but
			// emits the per-observation dump in a different order every run,
			// so two identical runs give files differing on ~3300 lines that
			// cannot be diffed. In a file whose comment above explains why a
			// diagnostic that moves between identical runs trains its reader
			// to ignore the diff, that is not a detail.
			gws := make([]int, 0, len(p.GWs))
			for gw := range p.GWs {
				gws = append(gws, gw)
			}
			sort.Ints(gws)
			for _, gw := range gws {
				g := p.GWs[gw]
				// A double gameweek is not one match, and this diagnostic is
				// about one. `loadGameweeks` accumulates, so a doubled row
				// carries xGC summed over both fixtures while `CleanSheets > 0`
				// still reads "at least one". Comparing them asks
				// P(zero goals across BOTH matches) against P(a clean sheet
				// in EITHER) -- the intersection against the union -- which
				// reads as a huge over-prediction that is entirely an artefact.
				// The doubled rows are few and carry a large share of the fitted
				// intercept; the measured figures are in the note.
				//
				// ⚠️ This guard is the one four sibling diagnostics got in
				// adfde43, the day after the doubles fix (89fa973, 2026-08-08),
				// and this file — written 08-07 — did not. See cssplit_test.go,
				// csbias_test.go, csvalue_test.go, goaltiming_test.go. The
				// determinism comment above already knew doubles reached this
				// loop and treated it as a tie-break question only.
				if g.Fixtures != 1 {
					continue
				}
				if g.Minutes < 90 || g.XGC <= 0 {
					continue // a partial match is not the team's full xGC
				}
				key := [2]int{p.Team, gw}
				if seen[key] {
					continue
				}
				seen[key] = true

				cs := 0.0
				if g.CleanSheets > 0 {
					cs = 1
				}
				rows.write(sn, strconv.Itoa(gw), strconv.Itoa(p.Team),
					strconv.FormatFloat(g.XGC, 'f', -1, 64),
					strconv.Itoa(g.GoalsConceded),
					strconv.FormatFloat(cs, 'f', -1, 64))
				n++
				pred += math.Exp(-g.XGC)
				act += cs
				sumXGC += g.XGC
				sumGoals += float64(g.GoalsConceded)
				for _, b := range buckets {
					if g.XGC >= b.lo && g.XGC < b.hi {
						b.n++
						b.pred += math.Exp(-g.XGC)
						b.act += cs
						b.xgc += g.XGC
						b.goals += float64(g.GoalsConceded)
					}
				}
			}
		}
	}

	fmt.Printf("\nClean sheets: exp(-xGC) against what happened.\n")
	fmt.Printf("One row per team-match, %s, players who finished the match.\n\n", seasonsLabel(len(seasons)))
	fmt.Printf("%-14s %7s %10s %10s %9s %9s %9s\n",
		"team xGC", "n", "predicted", "actual", "error", "mean xGC", "mean GC")
	for _, b := range buckets {
		if b.n == 0 {
			continue
		}
		fmt.Printf("%.1f - %-8.1f %7.0f %10.3f %10.3f %+9.3f %9.2f %9.2f\n",
			b.lo, b.hi, b.n, b.pred/b.n, b.act/b.n, (b.pred-b.act)/b.n,
			b.xgc/b.n, b.goals/b.n)
	}
	fmt.Printf("\npooled: n %.0f, predicted %.3f, actual %.3f, error %+.4f\n",
		n, pred/n, act/n, (pred-act)/n)
	fmt.Printf("xGC calibration: mean expected %.3f against mean conceded %.3f (%+.1f%%)\n",
		sumXGC/n, sumGoals/n, 100*(sumXGC/sumGoals-1))

	// --- IS THE MISCALIBRATION A SCALE OR AN OFFSET? ------------------------
	//
	// `FPL_CS_XGC_FACTOR` is a MULTIPLIER on xGC inside the clean-sheet term, so
	// it can only express a correction of the form exp(-f*x). If what the data
	// wants is exp(-(a+x)) — a constant multiplicative rescale of every clean-sheet
	// probability — then no value of f fits, and a ladder over f is measuring a
	// family the data has already rejected.
	//
	// The two are distinguishable and the test is cheap. Per bucket, the factor
	// that would zero it is ln(actual)/ln(predicted) — since pred = exp(-x_eff),
	// setting exp(-f*x_eff) = act gives f = ln(act)/ln(pred). If the miscalibration
	// is a scale, that factor is CONSTANT across buckets. If it is an offset, the
	// factor FALLS as xGC rises, because a fixed additive term is a shrinking
	// fraction of a growing exponent.
	//
	// Then the same question as a fit: regress -ln(actual) on -ln(predicted),
	// weighted by n. A pure scale is a = 0, b = f. A pure offset is a > 0, b = 1.
	//
	// ⚠️ Derived from bucket means, so it carries a Jensen bias — exp is convex, so
	// the mean of exp is not exp of the mean. It is a discriminator between two
	// shapes, not an estimate of either parameter.
	fmt.Printf("\nthe factor that would zero each bucket, and what shape it implies\n")
	fmt.Printf("%-14s %7s %10s %10s %10s\n", "xGC bucket", "n", "mean xGC", "f_zero", "resid f=1")
	var sw, sx, sy, sxx, sxy float64
	for _, b := range buckets {
		if b.n == 0 {
			continue
		}
		pm, am := b.pred/b.n, b.act/b.n
		if pm <= 0 || pm >= 1 || am <= 0 || am >= 1 {
			continue
		}
		f := math.Log(am) / math.Log(pm)
		fmt.Printf("%-14s %7.0f %10.3f %10.3f %+10.3f\n",
			fmt.Sprintf("%.1f-%.1f", b.lo, b.hi), b.n, b.xgc/b.n, f, pm-am)
		x, y, w := -math.Log(pm), -math.Log(am), b.n
		sw += w
		sx += w * x
		sy += w * y
		sxx += w * x * x
		sxy += w * x * y
	}
	if sw > 0 {
		den := sw*sxx - sx*sx
		if den != 0 {
			bb := (sw*sxy - sx*sy) / den
			aa := (sy - bb*sx) / sw
			fmt.Printf("\nweighted fit  -ln(actual) = a + b * -ln(predicted):  "+
				"a = %+.4f, b = %+.4f\n", aa, bb)
			fmt.Printf("  a pure SCALE is a = 0 with b = the factor; a pure OFFSET is " +
				"a > 0 with b = 1.\n")
			fmt.Printf("  An offset means the correction FPL_CS_XGC_FACTOR can express is " +
				"the wrong family:\n")
			fmt.Printf("  a flat rescale of every clean-sheet probability, which is the " +
				"shared level the\n")
			fmt.Printf("  ordering rule already says an argmax cannot see.\n")
		}
	}

	// Only the two pooled figures go to the snapshot, not the six buckets. The
	// bucket table's job is to show that the bias is not in xGC but in the Poisson
	// applied to it; the snapshot's job is to record the size of the error and
	// whether it moved, and a position-wide level shift is the one kind of bias
	// this project has established an argmax cannot see.
	csGrid := fmt.Sprintf("one row per team-match, %s with expected goals", seasonsLabel(len(seasons)))
	sink.emitAll("clean_sheet_calibration", csGrid,
		"clean sheet rate, all team-matches pooled", int(n),
		measure{"predicted", pred / n},
		measure{"actual", act / n},
		measure{"error", (pred - act) / n},
		measure{"points per match for a defender", 4 * (pred - act) / n})
	sink.emitAll("clean_sheet_calibration", csGrid,
		"expected against actual goals conceded", int(n),
		measure{"predicted", sumXGC / n},
		measure{"actual", sumGoals / n},
		measure{"error", (sumXGC - sumGoals) / n})

	// What the error is worth. A clean sheet pays four to a defender or keeper,
	// so an error in the rate is four times that in points per match.
	fmt.Printf("\nAt 4 points a clean sheet that is %+.3f points per match for a\n",
		4*(pred-act)/n)
	fmt.Printf("defender or keeper, against a defender's total of roughly 4.5.\n")

	// The obvious alternative is to keep Poisson and correct the rate it is
	// evaluated at. A single multiplier on xGC is the cheapest such fix, so
	// report the one that would zero the pooled error.
	best, bestErr := 1.0, math.Inf(1)
	for k := 0.70; k <= 1.30001; k += 0.01 {
		var p2 float64
		for _, sn := range playedSeasons(needsSweep) {
			_ = sn
		}
		p2 = 0
		// Recomputing the sum needs the raw values; approximate with the
		// bucket means, which is enough to locate the multiplier to 0.01.
		for _, b := range buckets {
			if b.n > 0 {
				p2 += b.n * math.Exp(-k*b.xgc/b.n)
			}
		}
		if e := math.Abs(p2/n - act/n); e < bestErr {
			best, bestErr = k, e
		}
	}
	fmt.Printf("\nA multiplier of %.2f on xGC would zero the pooled error (residual %.4f).\n",
		best, bestErr)
	fmt.Printf("Treat that as a description of the gap, not a fix: it is fitted on\n")
	fmt.Printf("the same %s it is measured against.\n", seasonsLabel(len(seasons)))
}
