package analysis

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// gameweeksPlayedBoolSanction is one file's permitted count of direct
// `GameweeksPlayed()` vs. `0` boolean comparisons, and the argument for
// keeping it — the same idiom TestTheCopiedExpressionsHaveOneImplementation
// and TestNonlinearTransformsScoreTheModelsOwnRegressor use for their own
// debt lists.
//
// Counted per FILE and checked in BOTH directions: a file carrying FEWER
// occurrences than its recorded count fails too, so an exemption cannot
// outlive the site it excuses. Keyed by path relative to the repo root
// rather than by basename, because this repo has more than one `main.go`
// and more than one `metrics.go`.
type gameweeksPlayedBoolSanction struct {
	n   int
	why string
}

// gameweeksPlayedBoolHit is one occurrence the scan found.
type gameweeksPlayedBoolHit struct {
	line int
	expr string
}

// TestGameweeksPlayedIsNeverUsedAsASeasonStartedBoolean is a source scan for
// the bug family AGENTS.md records under the DataWindow live-GW1-gap entry:
// `GameweeksPlayed() == 0` / `GameweeksPlayed() > 0` (and their negations)
// used as a stand-in for "has the season meaningfully started", when the
// correct predicate is SeasonHasStarted() (any fixture anywhere has kicked
// off) or, for a per-population question, the shared inLiveGameweekGap()
// helper.
//
// # Why this exists
//
// GameweeksPlayed() counts a gameweek only once EVERY fixture in it has
// finished. SeasonHasStarted() answers true the moment the season's FIRST
// ball is kicked, which can be days earlier — a Premier League gameweek
// spans Friday to Monday, and FPL zeroes the whole league's aggregates at
// the first kickoff, not at the last final whistle. The two answer alike all
// season except that multi-day span, which is exactly why this shipped six
// times independently, on live incidents, rather than being caught by
// review: `engine.GameweeksPlayed() > 0` and `engine.SeasonHasStarted()`
// share no tokens, so nothing forces a reader comparing two call sites to
// notice they are competing answers to one question. See AGENTS.md's
// "Things that have already bitten" (the DataWindow/live-GW1-gap entry) for
// the six recorded sites — #39, #42, the squad pool's minutes floor,
// bonusEvidence and AssemblyBudget (both #45), and squadPriceGameweek — and
// SeasonHasStarted's own doc comment on metrics.go for the mechanism.
//
// A seventh, live instance was found while writing this scan:
// tournamentAbsence's own gate read `e.GameweeksPlayed() > 0` until the same
// change that added this test replaced it with `e.SeasonHasStarted()` — see
// that function's comment and TestTournamentAbsenceStopsAtKickoffNotAtGameweekFinish.
//
// # What it catches
//
// Only the DIRECT boolean-condition shape: a `*ast.BinaryExpr` whose
// operator is one of `== != < <= > >=`, where one operand is a call whose
// selector name is `GameweeksPlayed` (any receiver — `e.`, `engine.`,
// `tb.Engine.`, …) and the other operand is the integer literal `0`. Both
// operand orders count: `GameweeksPlayed() == 0` and `0 == GameweeksPlayed()`.
//
// # What it does NOT catch — stated because a guard that reads like it
// covers a class and does not is worse than none
//
//   - **A literal other than 0.** `engine.GameweeksPlayed() > 1` in
//     cmd/armband/asof.go is a deliberately different comparison — its own
//     comment names the live path's `> 0` and explains why the replay tool
//     wants `> 1` — and this scan must not reach it: the anti-pattern is
//     specifically about the 0/not-0 boundary, and flagging every literal
//     would turn a targeted guard into a nuisance around a correct site.
//   - **Assignment to a variable, compared later.** `played :=
//     e.GameweeksPlayed(); ... if played > 0` is the shape DataWindow and
//     blendRatesCode use ON PURPOSE (see their own comments — the recency
//     index is deliberately gated on gameweeks FINISHED, not started, since
//     an index built over one still-live gameweek adds nothing over
//     el.Minutes itself). Catching it needs dataflow tracing — following the
//     single assignment to `played` the way
//     TestNonlinearTransformsScoreTheModelsOwnRegressor's `singleAssignments`
//     does for a different class — and nothing here reuses that machinery.
//     **This is a real limitation, not an oversight**: a future call site
//     that copies GameweeksPlayed() into a local before comparing it is
//     invisible to this scan.
//   - **Test files.** `_test.go` sources are excluded from the walk. They
//     legitimately assert a synthetic engine's exact calendar state as a
//     setup precondition — `if e.GameweeksPlayed() != 0 { t.Fatalf("setup: ...") }`
//     appears throughout this package's own tests — which is a fixture check,
//     not the gate-substitution bug this scan exists to catch. Including
//     them would mean allowlisting dozens of correct assertions to guard
//     against a shape that was never the defect.
//   - **Only function bodies are walked**, the same limit
//     TestNonlinearTransformsScoreTheModelsOwnRegressor states for its own
//     scan. A package-level `var x = engine.GameweeksPlayed() == 0` is
//     invisible to it.
//
// Nothing here is a measurement: extending a source scan moves no replayed
// point, so no detection threshold applies to it, and a green run proves the
// scan ran, not that the class is closed.
func TestGameweeksPlayedIsNeverUsedAsASeasonStartedBoolean(t *testing.T) {
	root := gameweeksPlayedScanRoot(t)

	// The debt list. Each entry is a site that compares GameweeksPlayed() to
	// 0 directly ON PURPOSE, with the argument for it. Keyed by path relative
	// to the repo root.
	sanctioned := map[string]gameweeksPlayedBoolSanction{
		"internal/analysis/metrics.go": {1, "" +
			"inLiveGameweekGap's own definition: `e.GameweeksPlayed() == 0 && " +
			"e.SeasonHasStarted()`. This is not a stand-in for SeasonHasStarted " +
			"— it IS the definition of the multi-day gap between the two, the " +
			"one place in the codebase entitled to name GameweeksPlayed()==0 " +
			"directly, because it is naming the gap itself rather than trying " +
			"to answer 'has the season started' with half the information"},
		"cmd/armband/main.go": {1, "" +
			"gates FETCHING the recency index (internal/recent match history) on " +
			"`engine.GameweeksPlayed() > 0`, matched deliberately with " +
			"blendRatesCode's own `played > 0` read of the same quantity — " +
			"blend.go's comment calls the two 'a matched pair' and warns against " +
			"changing one without the other. A recency index built over a " +
			"single, still-live gameweek adds nothing over el.Minutes itself, " +
			"so this is an evidence COUNT worth having before spending a fetch " +
			"on it, not the provenance question SeasonHasStarted answers " +
			"differently"},
	}

	seen := map[string]int{}
	var offenders []string
	scanned := 0
	for _, sub := range []string{"internal", "cmd"} {
		dir := filepath.Join(root, sub)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}
			scanned++
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)

			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return fmt.Errorf("parsing %s: %w", rel, err)
			}
			for _, h := range gameweeksPlayedBoolHits(fset, file) {
				seen[rel]++
				if seen[rel] <= sanctioned[rel].n {
					continue
				}
				offenders = append(offenders, fmt.Sprintf("%s:%d  %s", rel, h.line, h.expr))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned == 0 {
		t.Fatal("scanned no source files under internal/ or cmd/ — this guard is " +
			"looking at the wrong tree")
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("GameweeksPlayed() is being compared directly to 0, the exact "+
			"stand-in for \"has the season started\" this project has already "+
			"shipped six times:\n  %s\n\nGameweeksPlayed() counts a gameweek only "+
			"once every fixture in it has FINISHED. FPL zeroes the whole league's "+
			"bootstrap aggregates the moment the season's FIRST fixture kicks off "+
			"— days earlier. Use SeasonHasStarted() for \"has the season started\", "+
			"or the shared inLiveGameweekGap() helper for a per-population "+
			"question during the gap itself. If this comparison is deliberate and "+
			"the argument for it is solid, add the file to `sanctioned` above WITH "+
			"THE ARGUMENT.", strings.Join(offenders, "\n  "))
	}

	// The debt list must shrink, not merely hold steady. A sanctioned file that
	// no longer carries its occurrences has been corrected, and leaving it
	// listed records a debt that has already been paid.
	var stale []string
	for path, s := range sanctioned {
		if seen[path] < s.n {
			stale = append(stale, fmt.Sprintf("%s carries %d, listed as %d — %s",
				path, seen[path], s.n, s.why))
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("an exemption outlived the site it excused:\n  %s\n\nLower the "+
			"count in `sanctioned` or delete the entry.", strings.Join(stale, "\n  "))
	}
}

// gameweeksPlayedBoolHits walks one parsed file's function bodies for the
// direct comparison shape. Shared with the positive control below so there
// is one implementation of what "the scan finds" means.
func gameweeksPlayedBoolHits(fset *token.FileSet, file *ast.File) []gameweeksPlayedBoolHit {
	var out []gameweeksPlayedBoolHit
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			switch bin.Op {
			case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			default:
				return true
			}
			call, lit := matchGameweeksPlayedZero(bin.X, bin.Y)
			if call == nil {
				call, lit = matchGameweeksPlayedZero(bin.Y, bin.X)
			}
			if call == nil {
				return true
			}
			_ = lit
			out = append(out, gameweeksPlayedBoolHit{
				line: fset.Position(bin.Pos()).Line,
				expr: exprString(bin),
			})
			return true
		})
	}
	return out
}

// matchGameweeksPlayedZero reports whether `call` is a call to
// `X.GameweeksPlayed()` and `lit` is the integer literal 0. Order matters —
// the caller tries both (call, lit) and (lit, call) — so this only ever
// checks one direction.
func matchGameweeksPlayedZero(call, lit ast.Expr) (ast.Expr, ast.Expr) {
	ce, ok := call.(*ast.CallExpr)
	if !ok || len(ce.Args) != 0 {
		return nil, nil
	}
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "GameweeksPlayed" {
		return nil, nil
	}
	bl, ok := lit.(*ast.BasicLit)
	if !ok || bl.Kind != token.INT || bl.Value != "0" {
		return nil, nil
	}
	return call, lit
}

// exprString renders a binary expression for the failure message without
// pulling in go/printer for one line of output.
func exprString(bin *ast.BinaryExpr) string {
	render := func(e ast.Expr) string {
		switch v := e.(type) {
		case *ast.CallExpr:
			if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
				return sel.Sel.Name + "()"
			}
		case *ast.BasicLit:
			return v.Value
		}
		return "?"
	}
	return render(bin.X) + " " + bin.Op.String() + " " + render(bin.Y)
}

// gameweeksPlayedScanRoot walks up from the test's own working directory to
// the module root, so the guard does not depend on where `go test` was
// invoked from. Also the vacuity check: fails loudly if the `GameweeksPlayed`
// method itself is gone, rather than silently scanning for a symbol nobody
// defines any more.
func gameweeksPlayedScanRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if !strings.Contains(readFileOrEmpty(filepath.Join(dir, "internal", "analysis", "metrics.go")),
				"func (e *Engine) GameweeksPlayed() int") {
				t.Fatal("Engine.GameweeksPlayed no longer exists at its expected " +
					"signature — this scan is looking for a symbol that moved or " +
					"was renamed, so it would pass vacuously rather than checking " +
					"anything")
			}
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find the module root above the test's working directory")
	return ""
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// TestTheGameweeksPlayedBoolScanFindsWhatItClaims is the positive control,
// and the sibling above cannot stand without it — on the same reasoning
// TestTheRegressorScanFindsWhatItClaims gives for its own sibling: every
// assertion above is a COUNT against a debt list, so once that list is
// empty a walk returning nothing for every case would pass green while
// checking nothing. Same failure as a byte-identical season under an
// intervention: indistinguishable in the output from a guard that ran and
// found nothing.
//
// So the cases below are synthetic sources, parsed from strings, exercising
// the walk directly. Several are NEGATIVE, and those are the ones that keep
// this scan from becoming a nuisance: each is a shape the header above
// promises escapes.
func TestTheGameweeksPlayedBoolScanFindsWhatItClaims(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       int
		why        string
	}{{
		name: "direct equals zero",
		body: "func f(e *Engine) bool { return e.GameweeksPlayed() == 0 }",
		want: 1,
		why:  "the recorded defect's most common shape",
	}, {
		name: "direct greater than zero",
		body: "func f(e *Engine) bool { return e.GameweeksPlayed() > 0 }",
		want: 1,
		why:  "the tournamentAbsence shape found live while writing this scan",
	}, {
		name: "reversed operand order",
		body: "func f(e *Engine) bool { return 0 == e.GameweeksPlayed() }",
		want: 1,
		why:  "the literal can be written on either side of ==",
	}, {
		name: "not-equal and less-or-equal negations",
		body: "func f(e *Engine) bool { return e.GameweeksPlayed() != 0 || e.GameweeksPlayed() <= 0 }",
		want: 2,
		why:  "the negated spellings the header promises this scan also catches",
	}, {
		name: "arbitrary receiver expression",
		body: "func f(tb *Toolbox) bool { return tb.Engine.GameweeksPlayed() == 0 }",
		want: 1,
		why:  "the match is on the selector NAME, not a specific receiver identifier",
	}, {
		name: "literal other than zero is not the anti-pattern",
		body: "func f(e *Engine) bool { return e.GameweeksPlayed() > 1 }",
		want: 0,
		why:  "cmd/armband/asof.go's deliberately different threshold — the anti-pattern is specifically about the 0/not-0 boundary",
	}, {
		name: "assigned to a variable, compared later",
		body: "func f(e *Engine) bool { played := e.GameweeksPlayed(); return played > 0 }",
		want: 0,
		why:  "the documented blind spot: dataflow tracing is out of scope, and this is the exact shape DataWindow and blendRatesCode use on purpose",
	}, {
		name: "SeasonHasStarted is a different method",
		body: "func f(e *Engine) bool { return e.SeasonHasStarted() }",
		want: 0,
		why:  "the correct predicate must never itself be flagged",
	}, {
		name: "comparison against another call, not a literal",
		body: "func f(e *Engine) bool { return e.GameweeksPlayed() == e.UpcomingGW() }",
		want: 0,
		why:  "only a literal 0 on the other side is the pattern; a call on both sides is a different comparison entirely",
	}} {
		fset := token.NewFileSet()
		src := "package p\n\n" + c.body + "\n"
		file, err := parser.ParseFile(fset, c.name+".go", src, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := len(gameweeksPlayedBoolHits(fset, file))
		if got != c.want {
			t.Errorf("%s: scan found %d hits, want %d — %s", c.name, got, c.want, c.why)
		}
	}
}
