package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestDerivedEnginesCarryEverySource is the wiring guard the backtest package's
// TestEveryScoringEngineGetsRecency and TestEveryScoringEngineGetsTeamForm
// structurally cannot supply.
//
// Both of those read `simulate.go` and count engines built there. An Engine can
// also be derived *from another Engine* — `WeekEngine` and `engineAt`, both
// horizon-1 views — and those live here, in a different package, in files no
// grep over simulate.go will ever open. `Engine.TeamForm` arrived on one side of
// a merge and was copied into neither, and nothing failed: `teamFormWeight`
// ships at zero, so the field is inert until somebody sweeps the flag, at which
// point the fielded eleven would have been scored on one view of the league and
// the transfer decision on another. That reads as a property of the blend rather
// than as a wiring gap, which is exactly how the recency version of this bug
// survived a whole round of measurement.
//
// # The list is derived, not written down
//
// `NewEngineFull` sets a fixed set of fields from its arguments; every other
// exported field is something a caller assigns afterwards, and therefore
// something a derived engine has to carry or silently lose. Both halves are read
// off the code — the constructor's composite literal by AST, the field set by
// reflection — so adding a field to Engine fails this test until it is either
// wired or deliberately handled. A hand-maintained exemption list would be one
// more list that outlives its situation, which is a failure mode this project has
// hit more than once.
//
// Deliberately out of scope, and there are two cases rather than the one the
// first version of this comment named. The **skip set** is not carried on
// purpose: a free hit removes a week from the scoring horizon, and "who do I
// field this week" is a different question — `WeekEngine` documents that.
// **`priceForecasts`** is not carried either, and that one is an omission rather
// than a decision; it is set through `SetPriceForecasts` and read only off the
// top-level engine today, so it is inert on a derived one. Both are unexported,
// so reflection cannot see them and a rule encoded here would be a second
// hand-maintained list. They are named instead.
//
// Also out of scope, and worth knowing before trusting this guard further: it
// checks **presence, not lock discipline**. `e.Cong` is read unguarded by both
// builders while `SetCompetitionWindows` writes it under `congMu`, and both are
// reachable from concurrent tool handlers. That predates this test and is not
// what it is for.
func TestDerivedEnginesCarryEverySource(t *testing.T) {
	fset := token.NewFileSet()
	files := parsePackage(t, fset)

	set := constructorFields(t, files)
	if len(set) == 0 {
		t.Fatal("NewEngineFull sets no fields on the Engine literal — the guard is " +
			"reading a constructor that has been rewritten, so update it deliberately " +
			"rather than letting it pass vacuously")
	}

	var required []string
	rt := reflect.TypeOf(Engine{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() || set[f.Name] {
			continue
		}
		required = append(required, f.Name)
	}
	sort.Strings(required)
	if len(required) == 0 {
		t.Fatal("no exported Engine field is left for a derived engine to carry, " +
			"which cannot be right while Priors and Recent exist")
	}

	builders := derivedBuilders(t, files)
	// Two today. The count is asserted so a third one added elsewhere in the
	// package is noticed here rather than after it has quietly scored a season
	// on half the sources.
	if len(builders) < 2 {
		t.Fatalf("found %d method(s) on *Engine that build another Engine; there are "+
			"two (WeekEngine and engineAt), so the detector has stopped matching",
			len(builders))
	}

	for name, copied := range builders {
		var missing []string
		for _, f := range required {
			if !copied[f] {
				missing = append(missing, f)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s builds a derived Engine without carrying %s. The constructor "+
				"cannot supply these — a caller assigns them — so the derived engine "+
				"scores from a different set of facts than the engine it came from, "+
				"and nothing fails while the missing field happens to be inert.",
				name, strings.Join(missing, ", "))
		}
	}

	// And the two must agree with each other, which is a stronger statement than
	// both clearing the required list: it catches a field carried by one path and
	// not the other even before anyone decides whether it is required.
	var names []string
	for name := range builders {
		names = append(names, name)
	}
	sort.Strings(names)
	for i := 1; i < len(names); i++ {
		a, b := names[0], names[i]
		if diff := symmetricDiff(builders[a], builders[b]); len(diff) > 0 {
			t.Errorf("%s and %s carry different field sets; they differ on %s. Two "+
				"horizon-1 views of one engine that disagree about what they inherit "+
				"will disagree about what a player is worth.",
				a, b, strings.Join(diff, ", "))
		}
	}
}

// parsePackage parses every non-test file in this package.
func parsePackage(t *testing.T, fset *token.FileSet) []*ast.File {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []*ast.File
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		out = append(out, f)
	}
	return out
}

// constructorFields is the set of Engine fields NewEngineFull sets from its
// arguments, read off its composite literal.
func constructorFields(t *testing.T, files []*ast.File) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name.Name != "NewEngineFull" || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "Engine" {
					return true
				}
				for _, el := range lit.Elts {
					if kv, ok := el.(*ast.KeyValueExpr); ok {
						if k, ok := kv.Key.(*ast.Ident); ok {
							set[k.Name] = true
						}
					}
				}
				return true
			})
		}
	}
	return set
}

// derivedBuilders finds every method on *Engine that constructs another Engine,
// and returns the field names it copies from its receiver, keyed by method name.
//
// A copy is `<local>.F = <receiver>.F` — same field on both sides. An assignment
// that renamed a field mid-copy would not be counted, which is the conservative
// direction: it would be reported as missing rather than silently accepted.
func derivedBuilders(t *testing.T, files []*ast.File) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); !ok || id.Name != "Engine" {
				continue
			}
			if len(fd.Recv.List[0].Names) == 0 {
				continue
			}
			recv := fd.Recv.List[0].Names[0].Name

			builds := false
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "NewEngineFull" {
					builds = true
				}
				return true
			})
			if !builds {
				continue
			}

			copied := map[string]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				lhs, ok := as.Lhs[0].(*ast.SelectorExpr)
				if !ok {
					return true
				}
				rhs, ok := as.Rhs[0].(*ast.SelectorExpr)
				if !ok {
					return true
				}
				rid, ok := rhs.X.(*ast.Ident)
				if !ok || rid.Name != recv {
					return true
				}
				if lhs.Sel.Name == rhs.Sel.Name {
					copied[lhs.Sel.Name] = true
				}
				return true
			})
			out[fd.Name.Name] = copied
		}
	}
	return out
}

func symmetricDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	for k := range b {
		if !a[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
