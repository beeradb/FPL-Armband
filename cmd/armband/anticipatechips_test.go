package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestAnticipateChipsReachesTheReplayAsAPair is the arrival guard for
// ReviewPolicy.AnticipateChips, in the same source-scanning shape as
// TestTheOptionValueBlockReachesTheReplay: a config field with no line in
// cmdBacktest's SimConfig mapping is a setting that never arrives, and the
// user gets a byte-identical result from a knob the replay never saw.
//
// This one carries an extra obligation `AnticipateChips` alone did not have.
// `backtest.SimConfig` has TWO fields — AnticipateChips and AnticipateGate —
// and the replay's own comment on AnticipateGate records that a mismatched
// pair (scoring a move on a shortened horizon while still charging it over
// the full one) was measured at -17 points a season. ReviewPolicy therefore
// exposes only ONE config field and cmdBacktest must drive both SimConfig
// fields from it, never from two independently-settable config fields. The
// source scan checks both halves: `cfg.Review.AnticipateChips` reaches the
// mapping, and nothing in it reads a hypothetical `cfg.Review.AnticipateGate`
// — the shape a future edit would take if someone "finished" this by giving
// the gate its own knob.
func TestAnticipateChipsReachesTheReplayAsAPair(t *testing.T) {
	src, err := os.ReadFile("backtest.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "backtest.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "cmdBacktest" || fd.Body == nil {
			continue
		}
		body = string(src[fset.Position(fd.Body.Pos()).Offset:fset.Position(fd.Body.End()).Offset])
	}
	if body == "" {
		t.Fatal("no cmdBacktest in cmd/armband — the guard is following a seam " +
			"that has been renamed or removed, so update it deliberately rather " +
			"than letting it pass vacuously")
	}

	if strings.Count(body, "cfg.Review.AnticipateChips") != 2 {
		t.Errorf("cmdBacktest does not read cfg.Review.AnticipateChips exactly twice " +
			"(once for SimConfig.AnticipateChips, once for SimConfig.AnticipateGate). " +
			"A config field read fewer times than that is a setting that never fully " +
			"arrives; read more times from a second config field is the -17-point-a-season " +
			"mismatch this field exists to make impossible")
	}
	if strings.Contains(body, "cfg.Review.AnticipateGate") {
		t.Error("cmdBacktest reads a cfg.Review.AnticipateGate that ReviewPolicy does not " +
			"expose — AnticipateGate must be derived from AnticipateChips, never " +
			"independently settable, or a manager can reproduce the measured -17-point-a-season " +
			"mismatched pair by only setting one of them")
	}
}
