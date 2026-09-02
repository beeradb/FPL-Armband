package backtest

// The CSV contract for the *model* half of the accuracy snapshot.
//
//	FPL_MODEL_CSV=/tmp/model.csv DIAG=1 \
//	    go test ./internal/backtest -run TestDiagCalibrationDrift -v -timeout 60m
//	armband snapshot --model=/tmp/model.csv --cells=/tmp/cells.csv
//
// # Two halves that must not be blurred
//
// cellcsv_test.go carries the *harness* half: one row per replayed cell, from
// which R answers "what size of effect could this design see at all". This file
// carries the *model* half: is the scoring model right about football, measured
// against outcomes rather than against another setting of itself.
//
// They are different questions and they fail differently. A model can be
// well-calibrated while the harness cannot resolve any change to it — which is
// this project's actual situation — and reading one number as though it answered
// the other is how "unresolved" came to be written up as "no effect" in both
// directions.
//
// # Why a CSV rather than parsing the logs
//
// Every diagnostic here already prints a formatted table. Scraping those tables
// would make the snapshot depend on column widths in a Printf, which is the kind
// of coupling that breaks silently: a widened column yields a parse that finds
// nothing, and a snapshot with a section missing looks much like a snapshot with
// a section that had nothing to say. Emitting the numbers alongside the table
// costs each diagnostic three lines and cannot misread itself.
//
// # The shape is deliberately long, not wide
//
// One row per (diagnostic, group, measure). The six diagnostics measure genuinely
// different quantities — a ratio of predicted to actual points, a median error
// per gameweek, a clear-rate, a per-position bias — and a wide schema would need
// a column per quantity, most of them blank, plus an edit here every time a
// diagnostic learns to report something new. A long schema needs neither.
//
// What the renderer supplies instead is prose: each diagnostic's registry entry
// in internal/snapshot says in one sentence what it measures and which direction
// is better, because "ratio 0.899" means nothing to a reader who has to guess
// whether high is good.

import (
	"encoding/csv"
	"os"
	"sort"
	"strconv"
	"sync"
)

// modelRow is one measured quantity from one diagnostic.
//
// Group is free text and is printed verbatim as a table row label, so it should
// read as a description rather than as a key: "model built through GW16", "buy
// side (player in)", "defenders, highest defcon third".
type modelRow struct {
	Diagnostic string // stable slug, e.g. "calibration_drift"
	Grid       string // the population, e.g. "4 season pairs, next 5 gameweeks"
	Group      string // the table row this belongs to
	N          int    // sample size behind it; 0 when not meaningful
	Measure    string // the column, e.g. "predicted", "actual", "ratio"
	Value      float64
}

var modelHeader = []string{"diagnostic", "run_id", "grid", "group", "n", "measure", "value"}

// modelSink appends model-accuracy rows to FPL_MODEL_CSV.
//
// A nil *modelSink is usable and does nothing, exactly as cellSink is, so a
// diagnostic's emit calls are unconditional and the env var is checked in one
// place. Every diagnostic keeps printing its human table whether or not the sink
// is open — the CSV is an addition, not a replacement, and a diagnostic that only
// spoke CSV would stop being readable at the terminal where it is mostly used.
type modelSink struct {
	mu    sync.Mutex
	f     *os.File
	w     *csv.Writer
	runID string
}

func openModelSink(path string) (*modelSink, error) {
	if path == "" {
		return nil, nil
	}
	f, created, err := openAppendCSV(path)
	if err != nil {
		return nil, err
	}
	s := &modelSink{f: f, w: csv.NewWriter(f), runID: runIDForProcess()}
	if created {
		if err := s.w.Write(modelHeader); err != nil {
			f.Close()
			return nil, err
		}
		s.w.Flush()
		return s, nil
	}
	// Same schema check the cells file gets, for the same reason: appending under
	// a header from a different build produces a ragged file rather than an
	// obviously broken one, and the likely reader is a half-remembered path.
	if err := checkHeader(path, modelHeader); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// emit writes one row, flushing immediately.
//
// Flushed per row because these diagnostics replay whole seasons and get killed
// under load on this machine; a partial file is worth having, and a partial file
// is also *detectable* downstream, where a diagnostic that emitted nothing is
// reported as not run rather than as having found nothing.
func (s *modelSink) emit(r modelRow) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.w.Write([]string{
		r.Diagnostic, s.runID, r.Grid, r.Group, strconv.Itoa(r.N), r.Measure,
		strconv.FormatFloat(r.Value, 'g', -1, 64),
	})
	s.w.Flush()
}

// measure is one named column, carried as a slice so the *order* survives.
//
// A map was the obvious shape and was wrong: Go randomises map iteration, so
// emitting from one either produces an unstable file — which makes two model CSVs
// undiffable — or has to be sorted, which puts the columns in alphabetical order.
// Alphabetical is actively unhelpful here: it renders "predicted" after "mae" and
// separates the two columns a reader is meant to compare. The diagnostic knows the
// order its table should read in, so it says.
type measure struct {
	Name  string
	Value float64
}

// emitAll is the common case: several measures of one group, in the order given.
func (s *modelSink) emitAll(diagnostic, grid, group string, n int, ms ...measure) {
	if s == nil {
		return
	}
	for _, m := range ms {
		s.emit(modelRow{
			Diagnostic: diagnostic, Grid: grid, Group: group, N: n,
			Measure: m.Name, Value: m.Value,
		})
	}
}

func (s *modelSink) close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w.Flush()
	s.f.Close()
}

// sortedPlayerIDs visits a season's players in a fixed order.
//
// Season.Players is a map and Go randomises map iteration, which is harmless while
// a diagnostic only accumulates — addition commutes — and *not* harmless the moment
// one deduplicates. The clean-sheet diagnostic keeps one representative per
// team-match, so whichever player it reached first decided the figure, and identical
// runs disagreed by 0.7%. That would put a spurious movement in every accuracy
// snapshot's diff, which is worse than no diff: a reader who learns the comparison
// always shows changes stops reading it.
//
// Reach for this in any diagnostic that picks *one* of several equivalent rows.
func sortedPlayerIDs(s *Season) []int {
	out := make([]int, 0, len(s.Players))
	for id := range s.Players {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// openModelSinkFor is the one line a diagnostic adds. It never fails the test: a
// diagnostic's job is to measure football, and a bad CSV path should not lose a
// twenty-minute replay that has already produced a printed table.
func openModelSinkFor(logf func(string, ...any)) *modelSink {
	s, err := openModelSink(os.Getenv("FPL_MODEL_CSV"))
	if err != nil {
		logf("FPL_MODEL_CSV not written: %v", err)
		return nil
	}
	return s
}
