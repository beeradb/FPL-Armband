package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestSquadBuildersDoNotNameABenchWeight counts the places that hardcode what a
// bench player is worth.
//
// "Bench weight" is how much a substitute's expected score counts when scoring a
// fifteen: 1.0 would treat him like a starter, 0.0 like a body who will never
// play. It is one quantity and there is one place to set it — config.json's
// `bench_weight`, which config.Load backfills from analysis.DefaultBenchWeight —
// and Optimize reads it whenever a caller leaves OptimizeRequest.BenchWeight at
// zero.
//
// Two commands disagreed for the whole life of the project. `armband squad`
// passed 0.02 and `brief`'s model-optimal squad passed 0.10, so two outputs each
// described as "the best fifteen from this budget" printed different fifteens from
// the same money — and because both named a value, the config key bound neither.
//
// # Why this is a source scan and not a runtime assertion
//
// TestBenchWeightHasOneDefault already compares the two *defaults*
// (analysis.DefaultBenchWeight against Weights.BenchWeight, which had drifted
// apart once). It structurally cannot see a literal at a call site: the two
// defaults can agree perfectly while a caller overrides both. The failure mode
// here is a *new* call site naming its own value, and nothing observes that at run
// time — the squad it builds is legal, plausible, and quietly scored on a
// different objective from every other squad in the program.
//
// So the scan is over the source, in the style of TestEveryScoringEngineGetsRecency
// (which counts the scoring engines a replay builds) and TestTheGridIsDeclaredOnce
// (which fails if a file pastes the replay grid back in).
//
// # What counts as naming a value
//
// A numeric literal other than zero, or DefaultBenchWeight itself. The constant is
// as much of a problem as the number: passing it shadows the config key just as
// completely, which is how brief.go came to ignore a `bench_weight` sitting in the
// file. Anything computed — cfg.Weights.BenchWeight, in.BenchWeight, an
// environment override, a helper call — passes, because that is a caller resolving
// the value from somewhere rather than inventing one.
//
// A name standing in for a literal counts too, and that case is worth spelling out
// because it is the *most likely* way this regrows: naming a magic number is what
// someone does in response to being asked about one, so `const openingBench = 0.02`
// two lines above the call site would otherwise walk straight through. Idents are
// resolved against same-file const and var declarations for exactly that reason.
//
// What still gets through, and is accepted scope: arithmetic (DefaultBenchWeight/2),
// a positional struct literal, a value fetched out of a map, and a multi-assignment.
// Each is more contrived than the plain literal this is built to catch, and a scan
// that tried to evaluate expressions would be a type checker.
func TestSquadBuildersDoNotNameABenchWeight(t *testing.T) {
	// Each entry is a file allowed to name a value, and the reason. The test also
	// fails when an entry stops being needed, so removing a pin removes its licence
	// rather than leaving a hole for the next call site to fall through.
	allowed := map[string]string{
		"internal/analysis/metrics.go": "DefaultWeights() is where the config default " +
			"is declared; this is the one definition the rest of the program reads",
		"cmd/armband/backtest.go": "the replay's stage-one squad print is pinned on " +
			"purpose, so it holds still while a FPL_BENCH_WEIGHT sweep moves the " +
			"simulation's own fifteen — comparing three identical benches is how a " +
			"bench-weight sweep once went unchecked",
	}
	used := map[string]bool{}

	root := filepath.Join("..", "..")
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Our own Go source only. .claude holds nested worktrees of this same
			// repository, which would otherwise be scanned as if they were this one.
			switch d.Name() {
			case ".git", ".cache", ".claude", "vendor":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)

		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		literals := literalNames(f)
		ast.Inspect(f, func(n ast.Node) bool {
			value := benchWeightAssignment(n)
			if value == nil {
				return true
			}
			named, what := namesABenchWeight(value, literals)
			if !named {
				return true
			}
			if _, ok := allowed[rel]; ok {
				used[rel] = true
				return true
			}
			offenders = append(offenders, fmt.Sprintf("%s:%d sets BenchWeight to %s",
				rel, fset.Position(value.Pos()).Line, what))
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(offenders) > 0 {
		t.Errorf("bench weight is set in config.json and read by Optimize whenever a "+
			"caller leaves OptimizeRequest.BenchWeight at zero; these name their own "+
			"value instead:\n  %s\n\nLeave the field unset unless the call site has a "+
			"reason the config cannot express — and if it does, add it to this test's "+
			"allowlist along with that reason.", strings.Join(offenders, "\n  "))
	}
	for path, why := range allowed {
		if !used[path] {
			t.Errorf("%s is allowlisted here (%s) but no longer names a bench weight; "+
				"drop the entry, so the next call site does not inherit its licence",
				path, why)
		}
	}
}

// benchWeightAssignment returns the expression a node assigns to a BenchWeight
// field, or nil. Both spellings count: the struct-literal key and a later
// assignment to the field.
func benchWeightAssignment(n ast.Node) ast.Expr {
	switch v := n.(type) {
	case *ast.KeyValueExpr:
		if id, ok := v.Key.(*ast.Ident); ok && id.Name == "BenchWeight" {
			return v.Value
		}
	case *ast.AssignStmt:
		if len(v.Lhs) != 1 || len(v.Rhs) != 1 {
			return nil
		}
		if sel, ok := v.Lhs[0].(*ast.SelectorExpr); ok && sel.Sel.Name == "BenchWeight" {
			return v.Rhs[0]
		}
	}
	return nil
}

// namesABenchWeight reports whether an expression hardcodes a bench weight, and
// how, for the failure message. literals maps same-file const and var names to the
// numeric literal each was declared with, so a name standing in for a number is
// treated as the number.
//
// A bare zero is not hardcoding: it is the documented way to say "use the
// configured weight", which is the behaviour this test exists to protect.
func namesABenchWeight(e ast.Expr, literals map[string]string) (bool, string) {
	const shadows = "DefaultBenchWeight, which shadows the config key as completely as a literal does"
	switch v := e.(type) {
	case *ast.BasicLit:
		if !isNonZeroNumber(v) {
			return false, ""
		}
		return true, "the literal " + v.Value
	case *ast.Ident:
		if v.Name == "DefaultBenchWeight" {
			return true, shadows
		}
		if lit, ok := literals[v.Name]; ok {
			return true, v.Name + ", declared in this file as " + lit
		}
	case *ast.SelectorExpr:
		if v.Sel.Name == "DefaultBenchWeight" {
			return true, shadows
		}
	}
	return false, ""
}

// literalNames collects same-file const and var names declared with a non-zero
// numeric literal — `const openingBench = 0.02`, or `bw := 0.02`.
//
// Deliberately not a type checker. It resolves the one indirection that costs
// nothing to write and would otherwise defeat the whole scan, and does not attempt
// arithmetic, cross-package constants or reassignment.
func literalNames(f *ast.File) map[string]string {
	out := map[string]string{}
	record := func(names []*ast.Ident, values []ast.Expr) {
		if len(names) != len(values) {
			return
		}
		for i, n := range names {
			if lit, ok := values[i].(*ast.BasicLit); ok && isNonZeroNumber(lit) {
				out[n.Name] = lit.Value
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ValueSpec:
			record(v.Names, v.Values)
		case *ast.AssignStmt:
			if v.Tok != token.DEFINE {
				return true
			}
			var names []*ast.Ident
			for _, l := range v.Lhs {
				id, ok := l.(*ast.Ident)
				if !ok {
					return true
				}
				names = append(names, id)
			}
			record(names, v.Rhs)
		}
		return true
	})
	return out
}

// isNonZeroNumber reports whether a literal is a number other than zero. Zero in
// any spelling means "use the configured weight" and is the point of the fix.
func isNonZeroNumber(lit *ast.BasicLit) bool {
	if lit.Kind != token.INT && lit.Kind != token.FLOAT {
		return false
	}
	v, err := strconv.ParseFloat(lit.Value, 64)
	return err == nil && v != 0
}
