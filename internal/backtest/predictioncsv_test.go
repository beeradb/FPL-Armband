package backtest

// The CSV contract for the prediction benchmark.
//
//	FPL_PREDICTION_CSV=/tmp/prediction.csv DIAG=1 \
//	    go test ./internal/backtest -run TestDiagPredictionBenchmark -v -timeout 60m
//	Rscript stats/prediction_inference.R /tmp/prediction.csv
//
// # Why sufficient statistics per gameweek rather than one row per observation
//
// The obvious shape is one row per player-gameweek per arm, which would be about
// 360,000 rows. It is not the right shape, and the reason is worth stating
// because "emit per-observation data and let R do the inference" is the standing
// rule and this is a deliberate reading of it rather than an exception to it.
//
// The unit of replication here is a **gameweek**, not a player: every player in a
// gameweek is exposed to the same football, so their errors are correlated and no
// standard error may treat them as independent draws. The quantities R needs are
// therefore per-gameweek totals, and the five columns emitted — the count, the
// summed absolute error, the summed squared error, the summed prediction and the
// summed outcome — are **exactly sufficient** for every downstream figure:
//
//   - mean absolute error is sum_abs_err / n;
//   - mean squared error is sum_sq_err / n, and root-mean-square error its root;
//   - bias is (sum_pred − sum_act) / n;
//   - the error spread follows from the two, since mean squared error equals
//     bias squared plus spread squared;
//   - a **paired** difference between two arms is the difference of their sums,
//     because both arms score the identical set of observations — same seasons,
//     same gameweeks, same population filter — so summing then differencing and
//     differencing then summing are the same arithmetic.
//
// Per-observation rows would buy only the ability to cluster *below* the cluster,
// which is not a thing anyone wants, at ten times the file size. What they would
// also buy is a way to get the pairing wrong, since it would then have to be
// reconstructed by joining on a player id.
//
// The two figures that are *not* decomposable into per-observation sums — the
// within-gameweek rank correlation and the signed error over the highest-predicted
// players — are gameweek-level scalars by construction, and are carried on the
// row that already has one row per gameweek. They are **blank** on every other
// row rather than zero, because an unmeasured quantity and a quantity measured at
// zero are different facts and only one of them is a number R will average.

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"sync"
)

var predictionHeader = []string{
	"run_id", "variant", "is_baseline",
	"season", "prior_season", "gw",
	"population", "target", "predictor", "category",
	"n", "sum_abs_err", "sum_sq_err", "sum_pred", "sum_act",
	"rank_corr", "tail_signed_err",
}

// predictionSink appends benchmark rows to FPL_PREDICTION_CSV.
//
// A nil *predictionSink is usable and does nothing, exactly as modelSink and
// cellSink are, so the emit call is unconditional and the environment variable is
// checked in one place.
type predictionSink struct {
	mu    sync.Mutex
	f     *os.File
	w     *csv.Writer
	runID string
}

func openPredictionSink(path string) (*predictionSink, error) {
	if path == "" {
		return nil, nil
	}
	f, created, err := openAppendCSV(path)
	if err != nil {
		return nil, err
	}
	s := &predictionSink{f: f, w: csv.NewWriter(f), runID: runIDForProcess()}
	if created {
		if err := s.w.Write(predictionHeader); err != nil {
			f.Close()
			return nil, err
		}
		s.w.Flush()
		return s, nil
	}
	// The same schema check the other two sinks get, for the same reason:
	// appending under a header from a different build produces a ragged file
	// rather than an obviously broken one, and the likely reader is a
	// half-remembered path.
	if err := checkHeader(path, predictionHeader); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

func openPredictionSinkFor(logf func(string, ...any)) *predictionSink {
	s, err := openPredictionSink(os.Getenv("FPL_PREDICTION_CSV"))
	if err != nil {
		logf("FPL_PREDICTION_CSV not written: %v", err)
		return nil
	}
	return s
}

func (s *predictionSink) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w.Flush()
	s.f.Close()
}

// priceBandPopulation names the CSV population for one price-rank window.
//
// ⚠️ The rank is over the players priced in THIS gameweek, so "1-10" means the
// ten most expensive that week and not a season-long top ten. Membership is
// per-gameweek for the same reason the candidate-set diagnostic's is: a season
// union scores a player in every week once he entered the set in any of them,
// and a live system knows its own top ten at decision time where it cannot know
// a union.
func priceBandPopulation(b [2]int) string {
	return fmt.Sprintf("price rank %d-%d", b[0], b[1])
}

// populationSelection is one named cut of a gameweek's rows.
type populationSelection struct {
	name string
	rows []playerGW
}

// emittedPopulations is every population the CSV carries for one gameweek: the
// two the printed benchmark reports, plus one per price-rank band.
//
// # Why the bands are emitted and not printed
//
// The printed table is a human summary, and four more populations across every
// target and predictor would bury the two readings it exists to show. The CSV is
// the machine contract `stats/prediction_inference.R` consumes, where an extra
// population costs a `--population=` flag and nothing else.
//
// ⚠️ **The printed populations are a SUBSET of the emitted ones, never a
// different list.** `TestEveryPrintedPopulationIsAlsoEmitted` pins that, because
// two lists that are supposed to nest and are maintained separately are one
// quantity with two implementations waiting to happen.
//
// ⚠️ **The bands share a ranking RULE with the candidate-set diagnostic, not a
// row set.** Both call priceRankOrder, so neither can invent its own tie-break;
// but this benchmark and that diagnostic filter players differently before
// ranking, so "price rank 1-10" here and `Pband1-10` there are the same
// definition over two populations. Do not quote a figure from one as if it were
// the other.
func emittedPopulations(rows []playerGW) []populationSelection {
	out := make([]populationSelection, 0, len(populationOrder)+len(candidateBands))
	for _, pop := range populationOrder {
		sel := rows
		if pop == popRelevant {
			sel = make([]playerGW, 0, len(rows))
			for _, r := range rows {
				if r.relevant {
					sel = append(sel, r)
				}
			}
		}
		out = append(out, populationSelection{pop, sel})
	}

	ids, prices := make([]int, len(rows)), make([]float64, len(rows))
	for i, r := range rows {
		ids[i], prices[i] = r.id, r.price
	}
	order := priceRankOrder(ids, prices)
	for _, b := range candidateBands {
		in := rankWindow(order, b[0], b[1])
		sel := make([]playerGW, 0, len(in))
		for _, r := range rows {
			if in[r.id] {
				sel = append(sel, r)
			}
		}
		out = append(out, populationSelection{priceBandPopulation(b), sel})
	}
	return out
}

// emitGameweek writes one row per (population, target, predictor, category) for
// a single replayed gameweek.
//
// It recomputes the accumulation rather than reading the arm's running totals,
// because those are cumulative over the whole grid and what a cluster-robust
// standard error needs is the per-gameweek contribution. Two accumulations of the
// same quantity is the bug class this project has shipped repeatedly, so note the
// difference: these are not two implementations of one number, they are the same
// arithmetic at two granularities, and the coarse one is exactly the sum of the
// fine ones. `TestPredictionCellsSumToTheReportedTotals` pins that.
func (s *predictionSink) emitGameweek(variant, season, priorSeason string, gw int,
	rows []playerGW) {
	if s == nil || len(rows) == 0 {
		return
	}
	isBaseline := strconvBool(variant == armShipped)

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ps := range emittedPopulations(rows) {
		pop, sel := ps.name, ps.rows
		if len(sel) == 0 {
			continue
		}
		for _, tg := range predTargets() {
			for pi, name := range predictorNames {
				// The two gameweek-level scalars belong to the points target
				// only, and to one row per (population, predictor) — the
				// all-categories row, which exists exactly once.
				var rankCol, tailCol string
				if tg.name == "points" {
					preds := make([]float64, len(sel))
					acts := make([]float64, len(sel))
					for i, r := range sel {
						preds[i], acts[i] = r.points[pi], r.actPoints
					}
					if rho, ok := spearman(preds, acts); ok {
						rankCol = fmtFloat(rho)
					}
					if v, ok := tailSignedError(preds, acts, tailSize); ok {
						tailCol = fmtFloat(v)
					}
				}
				byCat := map[string]*errAcc{}
				for _, r := range sel {
					pred, act := r.pred(tg.name, pi), r.act(tg.name)
					for _, c := range []string{catAll, r.category} {
						a := byCat[c]
						if a == nil {
							a = &errAcc{}
							byCat[c] = a
						}
						a.add(pred, act)
					}
				}
				for _, cat := range categoryOrder {
					a := byCat[cat]
					if a == nil || a.n == 0 {
						continue
					}
					rc, tc := "", ""
					if cat == catAll {
						rc, tc = rankCol, tailCol
					}
					_ = s.w.Write([]string{
						s.runID, variant, isBaseline,
						season, priorSeason, strconv.Itoa(gw),
						pop, tg.name, name, cat,
						strconv.Itoa(a.n),
						fmtFloat(a.sumAbs), fmtFloat(a.sumSq),
						fmtFloat(a.sumPred), fmtFloat(a.sumAct),
						rc, tc,
					})
				}
			}
		}
	}
	// Flushed per gameweek rather than per row: these runs get killed under load
	// on this machine and a partial file is worth having, but a flush per row
	// would be several thousand syscalls a gameweek for no extra safety.
	s.w.Flush()
}

func fmtFloat(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func strconvBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
