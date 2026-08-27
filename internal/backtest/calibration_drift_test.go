package backtest

// Is the model's calibration stable through a season, or does it drift?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagCalibrationDrift -v -timeout 1h
//
// Three flat corrections of genuinely measured biases have now lost points —
// recency on rates, min_gain, and the buy-side discount — because the optimiser
// consumes an ordering and a correction applied equally to every candidate
// cannot change one. So this is deliberately **not** a hunt for another flat
// adjustment.
//
// It asks a different question: does the size of the error *move* through the
// season? A bias that is constant is invisible to an argmax and harmless. A bias
// that is large in September and small in March is something else entirely —
// it makes any measurement pooled across periods a mixture of two regimes, and
// that is a source of noise rather than of bias.
//
// That matters because of what the sweeps have been doing all along. Entry
// points at GW1, 6, 11, 16, 21 and 26 are pooled into one standard error on the
// assumption they are the same measurement repeated. If the model is
// systematically over-confident early and calibrated late, they are not.
//
// # What is compared
//
// For each cutoff, the model is built from data through that gameweek and each
// player's Score — expected points per gameweek — is compared against what he
// actually scored over the following five. Restricted to players with a real
// role, since the interesting question is whether the model is calibrated where
// it is making decisions, not whether it correctly rates someone who never
// plays.
//
// The ratio matters more than the difference: a model that predicts 4.0 and gets
// 3.6 is doing something different from one that predicts 1.0 and gets 0.6, and
// pooling absolute errors across a season would let the high scorers dominate.

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagCalibrationDrift(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	// The accuracy snapshot's headline model figure. See modelcsv_test.go.
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	pairs := sweepPairNames()
	cutoffs := []int{4, 8, 12, 16, 20, 24, 28, 32}
	const window = 5

	type obs struct{ pred, act float64 }
	byCut := map[int][]obs{}

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)

		for _, cut := range cutoffs {
			boot, fx := PointInTime(cur, prior, cut)
			e := analysis.NewEngineFull(boot, fx, cfg.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = idx
			e.Recent = newRecentIndexWith(cur, cut,
				cfg.Weights.MinutesHalfLife, cfg.Weights.RateHalfLife)

			for i := range boot.Elements {
				el := &boot.Elements[i]
				p := cur.Players[el.ID]
				if p == nil {
					continue
				}
				m := e.Metrics(el)
				// Players the model would actually consider. Rating a reserve
				// correctly is not the question.
				if m.Score < 2.0 || m.ExpectedMinutes < 45 {
					continue
				}
				var pts float64
				weeks := 0
				for gw := cut + 1; gw <= cut+window && gw <= 38; gw++ {
					if g, ok := p.GWs[gw]; ok {
						pts += float64(g.Points)
					}
					weeks++
				}
				if weeks == 0 {
					continue
				}
				byCut[cut] = append(byCut[cut], obs{
					pred: m.Score, act: pts / float64(weeks),
				})
			}
		}
	}

	fmt.Printf("\nExpected against actual points per gameweek, by when the model was built.\n")
	fmt.Printf("Players scoring 2.0+ with 45+ expected minutes — the ones decisions are\n")
	fmt.Printf("made about. Predicting the next %d gameweeks, %s pooled.\n\n",
		window, seasonsLabel(len(pairs)))
	fmt.Printf("%-8s %6s %9s %9s %9s %9s %9s\n",
		"through", "n", "expected", "actual", "ratio", "bias", "MAE")

	var ratios []float64
	var cuts []int
	for c := range byCut {
		cuts = append(cuts, c)
	}
	sort.Ints(cuts)
	for _, c := range cuts {
		rows := byCut[c]
		if len(rows) < 20 {
			continue
		}
		var p, a, abs float64
		for _, r := range rows {
			p += r.pred
			a += r.act
			abs += math.Abs(r.act - r.pred)
		}
		n := float64(len(rows))
		ratio := 0.0
		if p > 0 {
			ratio = a / p
		}
		ratios = append(ratios, ratio)
		fmt.Printf("%-8d %6d %9.3f %9.3f %9.3f %+9.3f %9.3f\n",
			c, len(rows), p/n, a/n, ratio, (a-p)/n, abs/n)
		// Predicted beside actual, then the ratio of the two, because the finding is
		// that actual stays flat while predicted rises — which is only visible if
		// the two columns are adjacent.
		sink.emitAll("calibration_drift",
			fmt.Sprintf("%d season pairs, next %d gameweeks", len(pairs), window),
			fmt.Sprintf("model built through GW%d", c), len(rows),
			measure{"predicted", p / n},
			measure{"actual", a / n},
			measure{"ratio", ratio},
			measure{"bias", (a - p) / n},
			measure{"mean absolute error", abs / n})
	}

	if len(ratios) > 1 {
		lo, hi := ratios[0], ratios[0]
		var sum float64
		for _, r := range ratios {
			sum += r
			if r < lo {
				lo = r
			}
			if r > hi {
				hi = r
			}
		}
		fmt.Printf("\nratio spans %.3f to %.3f, mean %.3f — a spread of %.1f%%\n",
			lo, hi, sum/float64(len(ratios)), 100*(hi-lo))
	}

	fmt.Printf("\nA flat ratio means the model is equally calibrated all season: the bias is\n")
	fmt.Printf("then invisible to an argmax and pooling periods is safe. A ratio that\n")
	fmt.Printf("drifts means measurements pooled across entry points mix two regimes,\n")
	fmt.Printf("which is noise rather than bias and would not show up as one.\n")
}
