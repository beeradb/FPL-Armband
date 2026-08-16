package backtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// nonlinearTransform is one function whose output is not a linear image of its
// input, together with why a diagnostic that applies it to the wrong quantity
// cannot be read as a statement about the model.
//
// **The next transform to guard is a ROW here, not a new test.** That is the
// standing remedy this file is an instance of: extend the scan that already
// exists rather than add a bespoke guard per site.
type nonlinearTransform struct {
	// pkg is the qualifier, empty for a call to a function in this package.
	pkg, fn string
	// why says what the convexity does, in words somebody who has not just been
	// in that code can check an offender against.
	why string
}

func (n nonlinearTransform) label() string {
	if n.pkg == "" {
		return n.fn
	}
	return n.pkg + "." + n.fn
}

// regressorSanction is one file's permitted count of archive-rooted transforms,
// and the argument for it.
//
// The unit counted is one (call, archive field) pair, so `math.Exp(-g.XGC*g.Saves)`
// is two. Counted per FILE and checked in BOTH directions, on the same terms as
// TestTheCopiedExpressionsHaveOneImplementation: a file carrying FEWER than its
// recorded count fails too, so an exemption cannot outlive the site it excuses.
// `why` is not decoration — an exemption whose argument nobody can restate is a
// silent hole, which is the failure this whole guard exists to close.
//
// ⚠️ Per-file counting inherits that scan's known seam: a new site added to a
// sanctioned file *in the same edit that deletes one of its sanctioned sites*
// passes. Pinning to an occurrence is not available — the two sanctioned lines in
// `cleansheet_calibration_test.go` are the same text — and both halves of such an
// edit are visible in a diff of the file, which is the case a per-file count was
// never the only defence for.
type regressorSanction struct {
	n   int
	why string
}

// regressorHit is one (call, archive field) pair the scan found.
type regressorHit struct {
	line  int
	label string
	field string
	why   string
}

// nonlinearTransforms are the calls this scan follows.
//
// `math.Exp` covers the logistic idiom 1/(1+math.Exp(-x)) without a second entry,
// since the call is inside it either way.
var nonlinearTransforms = []nonlinearTransform{{
	pkg: "math", fn: "Exp",
	why: "exp is convex, so E[exp(-x)] depends on the DISPERSION of x and not " +
		"only on its mean. Two regressors with the same mean and different " +
		"spreads give different aggregate predictions, which is exactly how a " +
		"calibration measured on realised match xGC came out 1.281 where the " +
		"same question on the model's own XGC90 reads 1.052",
}, {
	pkg: "math", fn: "Floor",
	why: "a floor is a step function: it discards everything below the next " +
		"integer, so an input on a different scale from the one the engine " +
		"floors lands in a different bucket rather than a proportionally " +
		"different place",
}, {
	pkg: "", fn: "poissonFloorDiv",
	why: "the saves block: P(at least d saves) summed over blocks, which is " +
		"a Poisson tail and therefore convex in the rate. It is unexported in " +
		"internal/analysis and so unreachable from here today — listed so a " +
		"copy pasted into this package is caught on arrival rather than after " +
		"it has produced a figure",
}}

// TestNonlinearTransformsScoreTheModelsOwnRegressor is a source scan for the
// wrong-regressor class: a nonlinear transform applied to a field of an ARCHIVE
// row, where the scoring path applies it to a field of `analysis.PlayerMetrics`.
//
//	pred += math.Exp(-g.XGC)          // g is an archive gameweek row
//	cleanSheetProb(m.XGC90, def, cf)  // what the engine actually evaluates
//
// # Why this exists
//
// A calibration was evaluated against a regressor the scoring path does not
// consume. `baseXP90` prices the clean sheet through `cleanSheetProb` on
// `m.XGC90` — a per-90 rate, blended toward a prior season, shrunk, read
// point-in-time, and multiplied inside the exponent by a fixture and a defcon
// factor. The diagnostic that measured it used realised single-match `XGC`.
// Those are different quantities with different dispersions, and `exp` is convex,
// so the *gap between the two* is construction rather than football. The
// resulting figure reached five surfaces — this repository's memory file,
// `docs/accuracy.md`, the snapshot renderer, `internal/analysis/sweep.go` and a
// reviewer's own brief — before anybody asked which quantity it had been fitted
// against.
//
// Two standing rules already forbade it: *a constant fitted against a proxy for
// its input is fitted to the proxy's noise too*, and *check what a multiplier
// multiplies before calibrating it*. Both were written down; it shipped anyway,
// for months. **A documented rule is demonstrably not sufficient for this class**
// — hence a mechanical check.
//
// # How it decides
//
// The archive-side field names are read out of `GW` and `Player` in `season.go`
// and the model-side ones out of `analysis.PlayerMetrics`, so the guarded set is
// DERIVED rather than transcribed and a field added to the archive is guarded the
// day it lands. A name carried by both structs is not guarded — see the limits.
// Local identifiers assigned exactly once in the enclosing function are
// substituted, so `x := g.XGC; math.Exp(-x)` is caught as well as the direct
// form; an identifier bound twice — including by a closure parameter — is left
// alone, so shadowing cannot manufacture a root.
//
// [TestTheRegressorScanFindsWhatItClaims] is the positive control, and it is not
// optional: every other assertion here is a *count*, so once the debt list below
// is emptied — which its own comment tells you to aim for — a walk that returned
// nothing for every argument would pass this test silently. That is this
// project's byte-identical null wearing a guard's clothes.
//
// # Seven things it does NOT reach, stated because a guard that appears to cover
// a class and does not is worse than none
//
//   - **It catches a mismatched REGRESSOR. It cannot catch a matched regressor
//     with a mismatched POPULATION.** Fitting on ever-presents and scoring
//     everyone passes it clean, and so does fitting on rows the engine's own
//     gates would have dropped — including the one live instance of that, which
//     is `Player.Team` being the END-OF-SEASON club, so a mid-season transferee's
//     earlier gameweeks are filed under a club he was not playing for.
//   - **The `XGC` / `XGC90` naming distinction it leans on is LUCK, not
//     convention.** Where a diagnostic and the engine spell one field the same
//     way for differently-processed values, a syntactic scan is blind — and that
//     is not hypothetical here: `Assists`, `Bonus`, `Goals`, `ID`, `Minutes`,
//     `Starts` and `TotalPoints` are archive fields whose names `PlayerMetrics`
//     also carries. The `t.Logf`'d set is authoritative; that list is a snapshot
//     of it.
//   - **The collision runs the other way too, so this scan CAN over-report.** A
//     struct declared inside a test that happens to spell a field the way the
//     archive does matches, whatever it holds — `bpsRow.Saves` is the live
//     instance, and it is in `sanctioned` below for that reason as much as for
//     its own.
//   - **An archive value laundered through an accumulator escapes.** `b.xgc +=
//     g.XGC` followed by `math.Exp(-k*b.xgc/b.n)` roots in a bucket field, not in
//     `GW`, and this scan follows single assignments rather than dataflow. That
//     shape is live in `cleansheet_calibration_test.go` today. By the same token
//     `freegates_test.go`'s `math.Exp(-r.maxXGC)` needs no exemption — it does not
//     match — and that is a coincidence rather than the guard agreeing that its
//     bounds are legitimate.
//   - **Only function bodies are walked.** A package-level
//     `var x = math.Exp(-row.XGC)` is invisible.
//   - **The reach is two directories**, named in `regressorScanRoots`:
//     `internal/backtest` and `cmd/priorblend`, which are the trees that can name
//     an archive row at all — `internal/analysis` cannot import this package, and
//     `cmd/armband` does not name `backtest.Player`. That last clause is a fact
//     about today's imports rather than a boundary anything enforces.
//   - **It says nothing about `E[max]`** in the captaincy term, which is
//     nonlinear in a way no regressor check addresses at all.
//
// **Nothing here is a measurement.** Extending a source scan moves no replayed
// point, so no detection threshold applies to it, and a green run proves that the
// scan ran rather than that the class is closed.
func TestNonlinearTransformsScoreTheModelsOwnRegressor(t *testing.T) {
	fset := token.NewFileSet()
	guarded := guardedArchiveFields(t, fset)

	// The debt list. Each entry is a site that applies a transform above to an
	// archive-named field ON PURPOSE, with the argument for it.
	sanctioned := map[string]regressorSanction{
		"cleansheet_calibration_test.go": {2, "" +
			"TestDiagCleanSheetPoisson IS the realised-match-xGC arm, deliberately " +
			"and with the mismatch stated at length in its own header. It is kept " +
			"as the shape-and-bias decomposition it was written for, and " +
			"TestDiagCleanSheetRegressor is its pair on the model's own regressor. " +
			"Two occurrences: the pooled accumulator and the per-bucket one. " +
			"⚠️ Its `needed mult` figures may not be quoted without naming the " +
			"regressor — that omission is what this whole scan exists to stop " +
			"repeating"},
		"bpsrules_test.go": {1, "" +
			"newBPS floors a big-chance share of ORDINARY SAVES, which is FPL's own " +
			"integer rule and has no PlayerMetrics counterpart — the model does not " +
			"price BPS through a per-90 rate at all, so there is no other quantity " +
			"this could be applied to. ⚠️ It is also this scan's worked OVER-REPORT: " +
			"`r` is a local `bpsRow`, not an archive `GW`, and the two spell `Saves` " +
			"the same way. So this entry can never be retired by fixing anything, " +
			"which is the one respect in which the debt list does not only shrink"},
		"modelrepro_test.go": {1, "" +
			"TestModelDiagnosticsAreReproducible pins that a reduction which SELECTS " +
			"one of several equivalent rows selects the same ones twice. It is not a " +
			"statement about the model and its `pred` is never read as one — the " +
			"regressor is incidental. Read its header before quoting anything it " +
			"computes, and before assuming what it reaches"},
	}

	seen := map[string]int{}
	var offenders []string
	for _, path := range regressorScanFiles(t) {
		base := filepath.Base(path)
		if base == regressorScanSelf {
			continue // this file names the transforms and the fields it scans for
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		for _, h := range scanFileForHits(fset, file, guarded) {
			seen[base]++
			if seen[base] <= sanctioned[base].n {
				continue
			}
			offenders = append(offenders, fmt.Sprintf(
				"%s:%d  %s applied to the archive field .%s\n      %s",
				base, h.line, h.label, h.field, h.why))
		}
	}

	if len(offenders) > 0 {
		t.Errorf("a nonlinear transform is being applied to a field of an ARCHIVE "+
			"row:\n  %s\n\nThe engine does not score that quantity. It scores a "+
			"field of analysis.PlayerMetrics — a per-90 rate, blended toward a "+
			"prior season, shrunk, and evaluated point-in-time — and a convex "+
			"transform of one is not a statement about the other.\n\nIf you are "+
			"measuring what the model does, take the regressor off e.Metrics(el) "+
			"as TestDiagCleanSheetRegressor does. If the realised quantity is the "+
			"point — a bound, a determinism pin, a deliberately-labelled arm — or "+
			"if the field is archive-named but not archive-held, add the file to "+
			"`sanctioned` above WITH THE ARGUMENT, and never quote its size "+
			"without naming the regressor.",
			strings.Join(offenders, "\n  "))
	}

	// The debt list must shrink. A sanctioned file that no longer carries its
	// occurrences has been corrected, and leaving it listed records a debt that
	// has been paid — which is how a debt list stops being read.
	var stale []string
	for base, s := range sanctioned {
		if seen[base] < s.n {
			stale = append(stale, fmt.Sprintf("%s carries %d, listed as %d — %s",
				base, seen[base], s.n, s.why))
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("an exemption outlived the site it excused:\n  %s\n\nLower the "+
			"count in `sanctioned` or delete the entry.", strings.Join(stale, "\n  "))
	}
}

// TestTheRegressorScanFindsWhatItClaims is the positive control, and the sibling
// above cannot stand without it.
//
// Every assertion in the sibling is a COUNT against a debt list. Counts are a
// real check while the list is non-empty — they pin `seen` to exactly the
// sanctioned number in both directions — but the list is meant to shrink, and an
// empty one leaves a walk that returns nothing for every argument passing green
// and measuring nothing. Same failure as a byte-identical season under an
// intervention: indistinguishable in the output from a guard that ran and found
// nothing.
//
// So the cases below are synthetic sources, parsed from strings, exercising the
// walk directly. Half of them are NEGATIVE, and those are the ones that stop the
// scan from being widened into a nuisance — each is a shape the header promises
// escapes, and two of them are regressions for over-reports that were live when
// this file was first written.
func TestTheRegressorScanFindsWhatItClaims(t *testing.T) {
	fset := token.NewFileSet()
	guarded := guardedArchiveFields(t, fset)

	for _, c := range []struct {
		name, body string
		want       []string
		why        string
	}{{
		name: "direct selector",
		body: "func f(g GW) float64 { return math.Exp(-g.XGC) }",
		want: []string{"XGC"},
		why:  "the recorded defect, in the shape it actually shipped",
	}, {
		name: "one-hop local",
		body: "func f(g GW) float64 { x := g.XGC; return math.Exp(-x) }",
		want: []string{"XGC"},
		why:  "the shape a next author writes, and the whole reason locals are followed",
	}, {
		name: "two-hop local",
		body: "func f(g GW) float64 { x := g.XGC; y := x * 2; return math.Exp(-y) }",
		want: []string{"XGC"},
		why:  "substitution has to compose or it only catches the literal paste",
	}, {
		name: "unqualified transform",
		body: "func f(g GW) float64 { return poissonFloorDiv(2, g.Saves) }",
		want: []string{"Saves"},
		why:  "a transform in this package has no package qualifier to match on",
	}, {
		name: "reassigned local",
		body: "func f(g GW) float64 { x := g.XGC; x = 1; return math.Exp(-x) }",
		why: "an identifier bound twice has no ONE defining expression, so following " +
			"either would be a guess. Fails open, which is the documented direction",
	}, {
		name: "closure parameter shadowing an archive-rooted local",
		body: "func f(g GW) float64 { x := g.XGC; _ = x; " +
			"h := func(x float64) float64 { return math.Exp(-x) }; return h(1) }",
		why: "the parameter is the binding in scope. Counting parameters is what " +
			"makes the header's 'shadowing is self-protecting' claim true rather " +
			"than true only of `:=`",
	}, {
		name: "a field spelled like an archive-rooted local",
		body: "func f(g GW, b *bucket) { xgc := g.XGC; _ = xgc; " +
			"b.pred += math.Exp(-b.xgc / b.n) }",
		why: "`b.xgc` is a BUCKET field that happens to be spelled like the local. " +
			"Resolving a selector's field NAME through the local table reported " +
			"cleansheet_calibration_test.go's accumulator as archive-rooted — the " +
			"exact line the header promises escapes. The names must MATCH or this " +
			"case tests nothing",
	}, {
		name: "accumulated through a running total",
		body: "func f(rows []GW) float64 { var total float64; " +
			"for _, g := range rows { total += g.XGC }; return math.Exp(-total) }",
		why: "the header's stated blind spot: an archive value laundered through an " +
			"accumulator. If this ever starts firing the header is wrong, not the code",
	}, {
		name: "range variable is an element, not the collection",
		body: "func f(p *Player) float64 { s := 0.0; " +
			"for _, g := range p.GWs { s += g.Points }; return math.Exp(-s) }",
		why: "recording `g` as defined by `p.GWs` would root every range body at " +
			"whatever the collection's selector happens to be called",
	}, {
		name: "a transform that is not in the set",
		body: "func f(g GW) float64 { return math.Sqrt(g.XGC) }",
		why: "sqrt is monotone and concave but it is a summary statistic here, not " +
			"a scoring transform; admitting it would put every SE and RMS in the " +
			"package on the debt list",
	}} {
		src := "package backtest\n\nimport \"math\"\n\nvar _ = math.Sqrt\n\n" + c.body + "\n"
		file, err := parser.ParseFile(fset, c.name+".go", src, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []string
		for _, h := range scanFileForHits(fset, file, guarded) {
			got = append(got, h.field)
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: scan found %v, want %v — %s", c.name, got, c.want, c.why)
		}
	}
}

// regressorScanSelf is this file, which carries the transform names and the
// sanction list and would otherwise report itself.
const regressorScanSelf = "regressorscan_test.go"

// regressorScanRoots are the directories scanned, relative to this package.
//
// The two trees that can name an archive row: this package, and `cmd/priorblend`,
// which imports it and builds `backtest.Player` values directly. `internal/analysis`
// cannot import this package at all, which is the architecture boundary rather
// than a choice made here.
var regressorScanRoots = []string{".", filepath.Join("..", "..", "cmd", "priorblend")}

// regressorScanFiles lists every Go file in scope, tests included.
//
// Tests INCLUDED because the diagnostics are where this class lives, and
// non-tests included too because a diagnostic's reduction extracted into a helper
// would otherwise leave the scan's reach behind it. Fails when a root yields
// nothing, so a moved directory cannot silently empty the scan.
func regressorScanFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, root := range regressorScanRoots {
		files, err := filepath.Glob(filepath.Join(root, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("no Go sources under %s — this guard is scanning the wrong "+
				"tree, which is indistinguishable from finding nothing", root)
		}
		out = append(out, files...)
	}
	return out
}

// guardedArchiveFields is the archive-side field set minus the model-side one.
//
// Derived from the declarations rather than transcribed: a transcribed list is a
// second implementation of the archive's schema and would go stale on the next
// field added. It logs both the guarded set and the shared names it therefore
// cannot see, because a scan whose reach is assumed is worse than one whose reach
// is known.
func guardedArchiveFields(t *testing.T, fset *token.FileSet) map[string]bool {
	t.Helper()
	archive := map[string]bool{}
	for _, s := range []string{"GW", "Player"} {
		for f := range numericFields(t, fset, "season.go", s) {
			archive[f] = true
		}
	}
	model := numericFields(t, fset, filepath.Join("..", "analysis", "metrics.go"),
		"PlayerMetrics")

	guarded, shared := map[string]bool{}, []string{}
	for f := range archive {
		if model[f] {
			shared = append(shared, f)
			continue
		}
		guarded[f] = true
	}
	sort.Strings(shared)
	t.Logf("guarded archive-only fields: %s", strings.Join(sortedNames(guarded), " "))
	t.Logf("UNGUARDED, shared with PlayerMetrics: %s", strings.Join(shared, " "))

	// Vacuity check, on the same terms as the "found" test in the prior-blend gate
	// scan: a rename that empties the guarded set must fail loudly rather than pass
	// silently. XGC is named individually because it is the field the recorded
	// defect was on.
	if !guarded["XGC"] || len(guarded) < 5 {
		t.Fatalf("the guarded set is %v — the structs this scan reads have been "+
			"renamed or moved, so it is passing vacuously rather than checking "+
			"anything", sortedNames(guarded))
	}
	if !model["XGC90"] {
		t.Fatal("analysis.PlayerMetrics no longer carries XGC90, so the model-side " +
			"set this subtracts is not the one the engine scores")
	}
	return guarded
}

// scanFileForHits is the walk, shared by the scan and its positive control so
// there is one implementation of what "the scan finds" means.
func scanFileForHits(fset *token.FileSet, file *ast.File, guarded map[string]bool) []regressorHit {
	var out []regressorHit
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		defs := singleAssignments(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			tr, ok := matchTransform(call)
			if !ok {
				return true
			}
			for _, arg := range call.Args {
				for _, f := range rootFields(arg, defs, guarded) {
					out = append(out, regressorHit{
						line:  fset.Position(call.Pos()).Line,
						label: tr.label(), field: f, why: tr.why,
					})
				}
			}
			return true
		})
	}
	return out
}

// matchTransform reports which of the listed transforms this call invokes.
//
// It matches on the selector NAME, so any receiver spelled `math` satisfies it.
// That is the same weakness the prior-blend gate scan records and the same trade:
// closing it means resolving types, and a local shim called `math` is not the
// shape anybody writes by accident.
func matchTransform(call *ast.CallExpr) (nonlinearTransform, bool) {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		pkg, ok := fn.X.(*ast.Ident)
		if !ok {
			return nonlinearTransform{}, false
		}
		for _, t := range nonlinearTransforms {
			if t.pkg == pkg.Name && t.fn == fn.Sel.Name {
				return t, true
			}
		}
	case *ast.Ident:
		for _, t := range nonlinearTransforms {
			if t.pkg == "" && t.fn == fn.Name {
				return t, true
			}
		}
	}
	return nonlinearTransform{}, false
}

// singleAssignments maps each identifier BOUND EXACTLY ONCE in the function to
// the expression it was bound to.
//
// Exactly once is the whole discipline. An identifier written twice has no one
// defining expression, and following either would be a guess — so it is left
// unresolved, which also means shadowing and reassignment cannot manufacture a
// false root.
//
// ⚠️ **Parameters, named results and receivers are counted**, including every
// nested closure's, and that is not tidiness: without it a closure parameter
// spelled like an outer local silently inherited the outer local's definition,
// so `x := g.XGC` anywhere in the function made every `func(x float64)` below it
// read as archive-rooted. Counted rather than defined, since a parameter has no
// defining expression here. Tuple assignments are paired positionally and skipped
// when the arities differ, since a multi-value call has no per-target expression.
func singleAssignments(fd *ast.FuncDecl) map[string]ast.Expr {
	count := map[string]int{}
	defs := map[string]ast.Expr{}
	bindNames := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, nm := range f.Names {
				if nm.Name != "_" {
					count[nm.Name]++
				}
			}
		}
	}
	note := func(lhs, rhs []ast.Expr) {
		for i, l := range lhs {
			id, ok := l.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			count[id.Name]++
			if len(lhs) == len(rhs) {
				defs[id.Name] = rhs[i]
			}
		}
	}
	bindNames(fd.Recv)
	bindNames(fd.Type.Params)
	bindNames(fd.Type.Results)
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.FuncLit:
			bindNames(s.Type.Params)
			bindNames(s.Type.Results)
		case *ast.AssignStmt:
			note(s.Lhs, s.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, 0, len(s.Names))
			for _, nm := range s.Names {
				lhs = append(lhs, nm)
			}
			note(lhs, s.Values)
		case *ast.RangeStmt:
			// A range variable's value is an ELEMENT of what is ranged, not the
			// collection, so recording the collection as its definition would
			// root `for _, g := range p.GWs` at `.GWs`. Counted so the name is
			// never resolved to something else, and deliberately not defined.
			for _, e := range []ast.Expr{s.Key, s.Value} {
				if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
					count[id.Name]++
				}
			}
		}
		return true
	})
	for name, c := range count {
		if c != 1 {
			delete(defs, name)
		}
	}
	return defs
}

// rootFields returns the guarded archive field names an expression reaches,
// following single-assignment locals to a bounded depth.
//
// ⚠️ **A selector's field name is never looked up in the local table.** It is a
// field, not a variable, and resolving it reported `b.xgc` as archive-rooted
// whenever the same function happened to hold a local `xgc := g.XGC` — turning
// the accumulator this scan promises to miss into an offender it could only be
// silenced about by raising a sanction. Only the RECEIVER is followed.
//
// The depth cap is what keeps a chain of aliases from recursing forever; the
// `seen` set does the same for a self-referential one. Four levels is more than
// any live site needs and the cap is not load-bearing — an expression that
// exceeds it is simply not resolved, which fails open rather than shut.
func rootFields(e ast.Expr, defs map[string]ast.Expr, guarded map[string]bool) []string {
	out := map[string]bool{}
	var walk func(ast.Expr, int, map[string]bool)
	walk = func(e ast.Expr, depth int, seen map[string]bool) {
		if e == nil || depth > 4 {
			return
		}
		var inspect func(ast.Node) bool
		inspect = func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if guarded[x.Sel.Name] {
					out[x.Sel.Name] = true
				}
				ast.Inspect(x.X, inspect)
				return false
			case *ast.Ident:
				if seen[x.Name] {
					return true
				}
				def, ok := defs[x.Name]
				if !ok {
					return true
				}
				next := map[string]bool{x.Name: true}
				for k := range seen {
					next[k] = true
				}
				walk(def, depth+1, next)
			}
			return true
		}
		ast.Inspect(e, inspect)
	}
	walk(e, 0, map[string]bool{})
	return sortedNames(out)
}

// numericFields returns the numeric field names of the named struct in the named
// file. Numeric because a regressor is a number: a string cannot be the argument
// of a convex transform, and admitting `WebName` would put it into a set compared
// against every selector in two packages.
//
// ⚠️ It is not a *categorical* filter. `Player.Type`, `Player.Team` and
// `Player.Code` are `int` and therefore guarded, so a transform applied to one
// reports an offender whose message talks about regressors. That is confusing
// rather than wrong, and narrowing it would mean a hand-maintained list of which
// ints are quantities — one more transcription of the schema.
//
// It fails the test when the struct is absent, so a rename cannot quietly turn
// the guarded set into the empty one.
func numericFields(t *testing.T, fset *token.FileSet, path, name string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]bool{}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != name {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		found = true
		for _, f := range st.Fields.List {
			id, ok := f.Type.(*ast.Ident)
			if !ok {
				continue
			}
			switch id.Name {
			case "int", "int64", "float32", "float64":
			default:
				continue
			}
			for _, nm := range f.Names {
				out[nm.Name] = true
			}
		}
		return true
	})
	if !found {
		t.Fatalf("no struct %q in %s — this scan derives its field sets from that "+
			"declaration, so a rename must fail here rather than empty the set",
			name, path)
	}
	return out
}

func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
