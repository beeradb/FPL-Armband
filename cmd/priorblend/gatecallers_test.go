package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllThreePathsCallTheSharedGate is the one check that sees all three
// implementations of the prior-blend rule at once.
//
// # Why a source check and not a behavioural one
//
// Each of the three packages has its own `TestTheBlendGateIsThinAndNonZero`,
// which drives that path against `analysis.ShouldBlendPrior` and is the real
// evidence. What none of them can see is the others: a behavioural test proves
// its own path obeys the rule TODAY, and it keeps passing if a fourth path
// appears, or if one path stops calling the shared predicate and reimplements it
// with the same answer — until somebody edits one copy.
//
// This test reads the three source files and requires the call. It is a weaker
// statement about behaviour and a stronger one about structure, which is the half
// the behavioural tests cannot make. It lives here because this is the only
// package that imports all three, and because it costs one file and no new
// dependency.
//
// # The failure it is aimed at
//
// This project's most-repeated bug is one quantity with several expressions and
// the measured one not being the one that runs. The prior-blend gate arrived with
// three implementations and its bar with four declarations; a change wired into
// two of the three would leave the replay pricing a feature the live command does
// not run, with nothing failing and every figure looking plausible. That is not
// hypothetical here: the gate WAS applied on the two archive paths and defeated on
// the live one, because blendPast discards the evidence before the gate can read
// it. This test passed throughout — the call was present and the rule was still
// wrong.
//
// # Three things it cannot see, stated because the one above is the fourth
//
//   - It matches on the selector NAME, so any receiver satisfies it. A local shim
//     called ShouldBlendPrior passes.
//   - It matches a call whose result is DISCARDED. `_ = analysis.ShouldBlendPrior(m)`
//     beside a restated inline gate passes, which is the silent-no-op shape this
//     exists to catch.
//   - It names three files by hand, so a fourth path is invisible.
//
// Closing those would mean re-implementing the gate inside the guard, which is the
// disease. The behavioural TestTheBlendGateIsThinAndNonZero in each package is the
// real evidence and would catch all three; this test is only the structural claim
// that there is one rule rather than several.
func TestAllThreePathsCallTheSharedGate(t *testing.T) {
	// path -> the function whose body must call the gate. Named individually so
	// a rename fails loudly rather than passing vacuously.
	for _, c := range []struct{ file, fn, what string }{
		{"../../internal/priors/priors.go", "LoadBlended",
			"the CSV archive's blended prior — currently reachable from NO command, " +
				"since cmd/armband calls priors.Load for the single-season fallback " +
				"and recent.LoadPriors for the blend, so this one is exported, " +
				"documented and dead. Gated anyway: dead code that is wrong is how a " +
				"path comes back wrong when somebody re-wires it"},
		{"../../internal/recent/priors.go", "blendPast",
			"the LIVE blended prior, from FPL's own history_past — this is the one " +
				"cmd/armband actually runs when prior_half_life is set"},
		{"../../internal/backtest/simulate.go", "newPriorIndexMulti",
			"the replay's prior index — every recorded figure about prior_half_life " +
				"came through this one"},
	} {
		if !bodyCalls(t, c.file, c.fn, "ShouldBlendPrior") {
			t.Errorf("%s does not call analysis.ShouldBlendPrior. It is %s, and it "+
				"decides which players get older seasons folded into their prior. A "+
				"path with its own copy of that rule is free to disagree with the "+
				"other two about the same footballer, silently.", c.fn, c.what)
		}
	}
}

// calibrationAccessor is one exported wrapper that exists so a CALIBRATION can
// see a factor the engine applies, and for no other reason.
//
// The next such accessor is a ROW here, not a new test — the same reason the
// prior-blend paths above are a table. A bespoke guard per accessor stops one
// divergence; a table stops the next accessor.
type calibrationAccessor struct {
	// name is the exported identifier, matched on the NAME alone.
	name string
	// declaredIn is the file the wrapper is declared in, so a rename or a move
	// fails loudly here rather than turning the row into a no-op.
	declaredIn string
	// applies is what a production caller would end up applying twice, in words
	// a reader who has not just been in that code can check an offender against.
	applies string
}

// TestTheCalibrationAccessorsHaveNoProductionCallers requires that the exported
// calibration wrappers are reached from tests and from nothing else.
//
// # What they are and why the confinement is the whole safety argument
//
// Each returns a factor or a term the engine has already applied, or is about to,
// exposed so a calibration can see what the scored path actually applies. So a
// NON-TEST caller that multiplies by the result applies it TWICE. That is not a
// divergence between two expressions — it is one expression used in a place it was
// not written for, and no behavioural test of either side would see it, because
// both sides are right. What sees it is who calls, which is why this is a scan and
// not an assertion about values.
//
// # ⚠️ Confinement is NOT equivalence, and two of these need both
//
// Three of the five are one-line delegations to the unexported function the engine
// runs — `FixtureMultipliersFor` to `fixtureMultipliersFor`, `DefconCleanFactorFor`
// to `defconCleanFactor`, `DefconTermFor` to `defconPer90`. Those cannot diverge
// from the scored path, so confinement is the whole of what they need.
//
// **`CleanSheetTermFor` and `SavesTermFor` delegate to nothing.** Each re-expresses
// its term — `CleanSheetTermFor` is `cleanSheetProb(xgc90, 1, 1)` times the
// position's clean-sheet points, which is `cleanSheetSensitiveAt` and `baseXP90`'s
// clean-sheet term written a third time with the fixture and defcon factors pinned
// to 1. So it CAN drift: a change applied at the *caller* rather than inside
// `cleanSheetProb` — the shape the open `FPL_CS_XGC_FACTOR` work would take — moves
// `baseXP90` and leaves this behind, and the calibration then fits a term the
// engine no longer scores. **This scan would report nothing**, because it only
// watches who calls.
//
// That is what `internal/analysis/cleansheetprob_test.go`'s property check is for,
// and it is load-bearing rather than redundant: it pins `CleanSheetTermFor` against
// the written-out expression over 20,000 draws, beside the same check for
// `cleanSheetSensitiveAt`. ⚠️ **Do not delete it as "comparing a function with
// itself".** An earlier revision of this comment said all of these were
// delegations, which is exactly the reading that would justify deleting it.
//
// # Three things it cannot see, stated because a scan is a tripwire and not a proof
//
// This project's source scans match an IDIOM keyed on one spelling of it. Passing
// means "the spellings this scan knows are absent", never "there are no copies":
//
//   - It matches on the identifier NAME with no type information, so a method of
//     the same name on some other type would be reported as an offender, and an
//     accessor renamed in only one of these two places passes vacuously — which is
//     why every row names its declaring file and the scan insists the declaration
//     is there.
//   - It reads `*_test.go` as "a test". A non-test file placed under a directory
//     nobody runs is still production as far as this scan is concerned, and a test
//     helper that a production path imports is still a test file.
//   - It cannot see a caller reached through anything but the identifier — a value
//     handed across a package boundary, a `reflect` lookup, or a generated file
//     written after the scan runs.
//
// Closing those would mean type-checking the repository inside the guard, which is
// a great deal of machinery to protect five small functions. The confinement is
// the claim; the tripwire is what enforces the spelling of it.
func TestTheCalibrationAccessorsHaveNoProductionCallers(t *testing.T) {
	accessors := []calibrationAccessor{
		{"FixtureMultipliersFor", "internal/analysis/metrics.go",
			"one fixture's attacking and defensive difficulty multipliers, which " +
				"baseXP90 has already folded into every per-fixture term"},
		{"DefconCleanFactorFor", "internal/analysis/teamstrength.go",
			"the defcon/clean-sheet coupling, which cleanSheetProb's caller already " +
				"applies to the expected goals conceded it scores from"},
		{"CleanSheetTermFor", "internal/analysis/teamstrength.go",
			"the clean sheet's own points per 90, which is a COMPONENT of baseXP90 " +
				"rather than an adjustment to it"},
		// The two below complete the family. They were left out of the first
		// revision because the brief named three — and a hand-maintained list
		// missing two members that sit in the same file, one of them named in
		// another row's own doc comment, is the failure mode this table exists to
		// retire. Both pass today.
		{"DefconTermFor", "internal/analysis/teamstrength.go",
			"the defensive-contribution points per 90, another COMPONENT of " +
				"baseXP90 — and one AGENTS.md records as about half redundant with " +
				"the clean sheet for defenders, so double-applying it compounds"},
		{"SavesTermFor", "internal/analysis/teamstrength.go",
			"a keeper's save points per 90, a third re-expression of baseXP90's " +
				"own poissonFloorDiv(savesBlock, Saves90)"},
	}

	// Reach: every .go file in THIS checkout. Stated as "everything" deliberately —
	// a skip list keyed on directory names is a reach that has to be re-checked
	// every time a directory is added, and this tree is small enough that walking
	// all of it costs nothing. internal/stats/copies_test.go keeps its own walker
	// because it is a line-based scan in another package and cannot import this
	// one; the two answer different questions and are not two spellings of one
	// quantity.
	//
	// ⚠️ "This checkout" is load-bearing, and the naive walk got it wrong. A nested
	// checkout of the SAME repository is skipped, because this repository keeps
	// sibling working trees inside itself and a walk that descends into them scans
	// other branches' source under this branch's name. That breaks the guard in
	// both directions: a sibling carrying an experimental production caller fails a
	// branch that never wrote one, and — the worse half — a sibling's test file
	// satisfies the liveness check below, so a branch that deleted its own last
	// test caller still passes. It was also 27x slower.
	//
	// The rule is "a directory that is itself a checkout", detected by a `.git`
	// entry rather than by name, so it holds for a worktree (a `.git` file), a
	// clone (a `.git` directory) and a vendored repository alike, and does not go
	// stale when the directory those live in is renamed.
	root := "../.."
	declared := map[string][]string{} // accessor -> every file declaring that name
	tested := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable corner of the tree is not a scan's business
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
				return filepath.SkipDir // a nested checkout — see the note on root
			}
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Parsed rather than grepped, and with comments DROPPED: the doc comment
		// on each of these three names the other two, so a text scan would report
		// the documentation as an offender.
		parsed, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			t.Errorf("parse %s: %v", rel, perr)
			return nil
		}
		isTest := strings.HasSuffix(rel, "_test.go")
		for _, a := range accessors {
			for _, at := range referencesTo(parsed, a.name) {
				if at == declarationSite {
					declared[a.name] = append(declared[a.name], rel)
					continue
				}
				if isTest {
					tested[a.name] = true
					continue
				}
				t.Errorf("%s references analysis.%s, which is exported for "+
					"CALIBRATION only. It returns %s, so a production caller that "+
					"multiplies by it applies that factor twice — and because the "+
					"accessor delegates to the very function the engine runs, both "+
					"sides agree and nothing fails. If this call is deliberate, the "+
					"scored path should read the unexported function directly and "+
					"this row should say so.", rel, a.name, a.applies)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	// The liveness half. A confinement check over a name that no longer exists, or
	// that nothing calls, passes for the wrong reason — it is the "byte-identical
	// because the comparison never ran" shape, in a guard.
	for _, a := range accessors {
		// Exactly one declaration, in the file the row names. "Somewhere in the
		// repository" would not do: a second declaration of the same name is what
		// makes the name-matching above ambiguous, and a MOVED declaration means
		// the row's `applies` text — the only statement of what a double
		// application would cost — is describing a function nobody has re-read.
		if got := declared[a.name]; len(got) != 1 || got[0] != a.declaredIn {
			t.Errorf("expected exactly one declaration of %s, in %s; found %v. The "+
				"guard is following a seam that has been renamed, moved or "+
				"duplicated, so update this row deliberately rather than letting it "+
				"pass vacuously — and if there are now two, the confinement check "+
				"above cannot tell their callers apart", a.name, a.declaredIn, got)
		}
		if !tested[a.name] {
			t.Errorf("nothing in a _test.go file calls %s. Its entire reason to be "+
				"exported is that a calibration reads it, so an accessor with no "+
				"test caller is dead exported surface — delete it, or this guard is "+
				"watching a name rather than a path", a.name)
		}
	}
}

// declarationSite marks a reference that IS the declaration of the name, rather
// than a use of it.
const declarationSite = "declaration"

// referencesTo returns one entry per occurrence of the identifier in the file,
// each either [declarationSite] or a use. It counts USES rather than calls, so a
// caller that takes the function as a value — `f := e.FixtureMultipliersFor` —
// is caught too; that is a real path to applying the factor and a CallExpr scan
// would miss it.
func referencesTo(f *ast.File, name string) []string {
	var out []string
	// The declaring identifier of a matching func is skipped explicitly and its
	// body walked by hand. Letting the default walk reach `FuncDecl.Name` would
	// report the declaration as a use of itself, and the file that declares the
	// accessor is a production file, so the guard would fail on its own subject.
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		ast.Inspect(n, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.FuncDecl:
				if d.Name != nil && d.Name.Name == name {
					out = append(out, declarationSite)
					if d.Recv != nil {
						walk(d.Recv)
					}
					walk(d.Type)
					if d.Body != nil {
						// A self-reference inside the wrapper would be a use
						// like any other, so the body is still walked.
						walk(d.Body)
					}
					return false
				}
			case *ast.Ident:
				if d.Name == name {
					out = append(out, "use")
				}
			}
			return true
		})
	}
	walk(f)
	return out
}

// bodyCalls reports whether the named function in the named file calls the named
// selector, e.g. analysis.ShouldBlendPrior. It fails the test when the function
// is absent, so a rename cannot turn the guard into a no-op.
func bodyCalls(t *testing.T, file, fn, callee string) bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	found, called := false, false
	for _, decl := range parsed.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn || fd.Body == nil {
			continue
		}
		found = true
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == callee {
				called = true
			}
			return true
		})
	}
	if !found {
		t.Errorf("no function named %q in %s — the guard is following a seam that "+
			"has been renamed or removed, so update it deliberately rather than "+
			"letting it pass vacuously", fn, file)
	}
	return found && called
}
