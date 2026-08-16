package backtest

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// The wiring half of the engine's per-season scoring rules.
//
// `internal/analysis/enginescoringrules_test.go` pins the arithmetic: an engine
// over a bootstrap naming a season scores under that season's table. These pin
// that the replay's bootstraps actually name their season — because a pin nobody
// sets is a **byte-identical null**, which is indistinguishable from a pin that
// does nothing, and this record has been caught by that shape more than once.

// TestEveryReplayBootstrapCarriesItsSeason is the guard the "Simulate builds
// three engines" trap demands, taken one layer lower than that trap sits.
//
// # Why here and not on the engines
//
// `TestEveryScoringEngineGetsRecency` counts `analysis.NewEngineFull` calls
// against `.Recent =` assignments, because the recency index is attached to each
// engine by hand and a patch once wired two of three. The scoring rules are
// deliberately **not** attached that way: `NewEngineFull` derives them from
// `Boot.Season`, so every engine built from a named bootstrap is pinned and there
// is no assignment to miss — including `WeekEngine` and `engineAt`, which rebuild
// from `e.Boot` and which the recency guard cannot see at all.
//
// That moves the whole exposure onto the two places a replay bootstrap is
// constructed. This is the guard for those.
//
// # Both halves, because a source scan alone would be a code fact
//
//   - **Behaviour**: every bootstrap `PointInTime` and `PreSeason` return names
//     its season, and an engine built from one prices a goalkeeper's goal under
//     that season's rules rather than today's;
//   - **Source**: no third construction of `fpl.Bootstrap` has appeared in this
//     package without a `Season` field. A new one would be invisible to the
//     behavioural half, since nothing would call it yet.
func TestEveryReplayBootstrapCarriesItsSeason(t *testing.T) {
	cc := loadConfig(t)
	// 2020-21 because it is on the far side of the one rule change this
	// repository can demonstrate, so "pinned" and "today's table" are
	// distinguishable. A season on the near side would pass either way.
	const season = "2020-21"
	cur, err := Load(context.Background(), cc.CacheDir, season)
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(context.Background(), cc.CacheDir, "2019-20")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	want := analysis.ScoringRulesFor(season).Goal[1]
	if want == analysis.ScoringRulesFor("").Goal[1] {
		t.Fatalf("%s and the live game both price a goalkeeper's goal at %v, so a "+
			"bootstrap that lost its season would be indistinguishable from one "+
			"that kept it and this test is vacuous", season, want)
	}

	// Through `EngineAt`, which is the seam `Simulate` wires its engines the same
	// way as — rather than a bare `NewEngineFull`, which would be an engine with no
	// priors and no recency index and therefore not the one the replay scores with.
	// `TestEveryScoringEngineGetsRecency` refuses the bare form, and it is right to:
	// a guard that checks the rules on an engine nobody builds is checking a
	// different engine.
	sc := sweepConfig(cc, 1, true)
	for name, through := range map[string]int{"PreSeason": 0, "PointInTime": 20} {
		e, b := EngineAt(cur, prior, through, sc)
		if b.Season != season {
			t.Errorf("%s returned a bootstrap naming season %q, want %q. Every "+
				"engine built from it takes TODAY's scoring rules, so the replay "+
				"scores a finished season under a rule nobody played under — and "+
				"nothing fails, because the wrong table is a valid table",
				name, b.Season, season)
			continue
		}
		// And the season reaches the arithmetic, which is the half a field check
		// cannot see: a bootstrap that carried the name and an engine that ignored
		// it would pass the assertion above.
		if got := e.ScoringRules().Goal[1]; got != want {
			t.Errorf("%s: an engine built from this bootstrap prices a goalkeeper's "+
				"goal at %v, want %s's %v", name, got, season, want)
		}
		// And the derived engine, which is where an `Engine.Rules` FIELD would have
		// silently lost the pin: `WeekEngine` rebuilds from `e.Boot`, so it inherits
		// the season and nothing has to be copied.
		if got := e.WeekEngine().ScoringRules().Goal[1]; got != want {
			t.Errorf("%s: the horizon-1 engine that picks the fielded eleven prices "+
				"a goalkeeper's goal at %v, want %v — the eleven and the transfer "+
				"decision are on different points tables", name, got, want)
		}
	}
}

// TestNoReplayBootstrapIsBuiltWithoutASeason is the source half of the guard
// above, and it is the half that survives a new call site nobody calls yet.
//
// A new `fpl.Bootstrap` literal would be invisible to a behavioural test until
// something reached it, and by then it would have replayed a season under the
// wrong table. Parsed rather than grepped, so a `Season` named in a comment or a
// string cannot satisfy it.
//
// ⚠️ **It walks the REPOSITORY, not this package, and scoping it to
// `internal/backtest` was the first version.** `Season` lives on
// `fpl.Bootstrap`, so the exposure is repo-wide even though today's only two
// literals are not: `cmd/priorblend` and `cmd/flagfit` build engines over archive
// data, and `internal/capture` and `internal/backfill` hold whole archived
// `bootstrap-static` payloads. A fabricated bootstrap in any of them would be
// unpinned, would silently score a finished season under today's table, and
// would have been invisible to both halves of this guard — which is the exact
// argument for the source half existing at all.
//
// The walk skips `.git` and `.claude` for the reason the neighbouring scans
// record: `.claude/worktrees` holds other branches' entire checkouts, and a
// scanner that descends into them reports and exempts files that are not this
// branch's. Invisible from inside a worktree, where that directory is empty.
func TestNoReplayBootstrapIsBuiltWithoutASeason(t *testing.T) {
	fset := token.NewFileSet()
	found := 0
	err := filepath.Walk("../..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Bootstrap" {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "fpl" {
				return true
			}
			found++
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Season" {
					return true
				}
			}
			t.Errorf("%s:%d builds an fpl.Bootstrap with no Season. Every engine "+
				"built from it silently takes today's scoring rules, which is the "+
				"defect analysis.ScoringRulesFor exists to stop — set it from the "+
				"season this bootstrap is reconstructing, or say here why an empty "+
				"season (the LIVE game) is right for it",
				path, fset.Position(lit.Pos()).Line)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	// ⚠️ A FAILURE, not a skip. A scan that finds nothing reads exactly like a
	// scan that found no problem, and this one is looking for a literal whose
	// spelling a refactor could easily change.
	if found < 2 {
		t.Fatalf("found %d fpl.Bootstrap literal(s) in the repository; there are "+
			"two (PointInTimeWith and PreSeasonWith), so the detector has stopped "+
			"matching and this guard is passing vacuously", found)
	}
}
