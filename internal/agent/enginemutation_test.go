package agent

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// TestResearchTargetsLeavesTheSharedEngineAlone is the agent-side twin of
// cmd/armband's TestAPlannedWildcardDoesNotShortenTheNextReadersHorizon.
//
// The toolbox holds one engine for the whole run. `research_targets` used to
// call Engine.ApplyChipPlan on it, which writes Weights.Horizon and rebuilds the
// fixture index, and nothing put either back — so a user with a wildcard in
// `chip_plan` had every later tool call in the review scored on a shortened
// horizon and every earlier one on the full horizon. See Toolbox.researchSquad.
//
// The wildcard is placed relative to the live next gameweek rather than at a
// fixed week, because a fixed week goes stale the moment the season moves past
// it and this test would then pass by measuring nothing.
func TestResearchTargetsLeavesTheSharedEngineAlone(t *testing.T) {
	tb := testToolbox(t)
	e := tb.Engine

	// ⚠️ Was an inline re-derivation of Engine.UpcomingGW, written because the
	// analysis package's own helper is unexported test-only code and this is a
	// different package. That is how a quantity acquires a third implementation.
	next := e.UpcomingGW()
	// Inside the horizon, so EffectiveHorizon has something to shorten. Anything
	// at or beyond it would make this test unable to detect the defect.
	wc := next + 2
	if wc > 38 || e.Weights.Horizon <= 2 {
		t.Skip("no gameweek left at which a planned wildcard would shorten the horizon")
	}
	e.Chips = analysis.ChipSchedule{First: analysis.ChipPlan{Wildcard: wc}}

	// The precondition, asserted rather than assumed: if the plan does not shorten
	// anything then a passing test below proves nothing at all.
	shortened, why := e.EffectiveHorizon(e.Chips)
	if why == "" || shortened >= e.Weights.Horizon {
		t.Skipf("a wildcard at GW%d does not shorten a horizon of %d, so this test "+
			"cannot detect the mutation", wc, e.Weights.Horizon)
	}

	before := e.Weights.Horizon
	if squad := tb.researchSquad(); len(squad) == 0 {
		t.Skip("the optimiser built no squad, so nothing here exercised the engine")
	}
	if got := e.Weights.Horizon; got != before {
		t.Errorf("research_targets left the shared engine on a horizon of %d, and it "+
			"was %d before. A wildcard planned for GW%d truncated the engine and "+
			"nothing put it back, so every later tool call in this run scores on a "+
			"different horizon from the ones asked first.", got, before, wc)
	}
}

// TestNoAgentToolAppliesTheChipPlanToTheSharedEngine is the tripwire, and it is
// the half of this pair that stops the NEXT copy rather than this one.
//
// Engine.ApplyChipPlan mutates its receiver. That is correct for a caller that
// is building the squad the plan describes and owns the engine for the duration
// — the web builder does it, under a save/restore — and it is never correct from
// a tool handler, because the tool runner fans a turn's calls out through an
// errgroup and the toolbox's engine is shared by all of them. A save/restore
// would not make it correct either: the siblings read the fixture index during
// the window.
//
// A source scan rather than a behavioural test per tool, for the reason this
// project's other scans give: a behavioural test stops one divergence, a scan
// stops the next copy.
//
// It WALKS rather than reading one directory, which is the idiom
// TestTheCopiedExpressionsHaveOneImplementation and
// TestTheSharedCellQuantitiesHaveOneImplementation already use. There is no
// subpackage under internal/agent today, so a non-recursive scan would be inert
// and would stay green on the day somebody adds one — a guard that silently
// stops covering its own package is worse than no guard, because it still reads
// like coverage.
//
// It is scoped to this package tree rather than the repository because the call
// is LEGITIMATE elsewhere: the web builder owns its engine for the request and
// guards it, and the replay mutates a per-cell engine on purpose. A repo-wide
// scan would need an allowlist of the callers that are fine, and an allowlist is
// the thing that goes stale.
//
// ⚠️ Like every scan here it matches an IDIOM — a handler reaching
// ApplyChipPlan through a helper in another package is invisible to it, so this
// is a tripwire and not a proof.
func TestNoAgentToolAppliesTheChipPlanToTheSharedEngine(t *testing.T) {
	scanned := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for i, line := range strings.Split(string(src), "\n") {
			code := line
			// Line comments only. A call inside a block comment would be a false
			// failure, which is the safe direction; a missed real call is not.
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx]
			}
			if strings.Contains(code, "ApplyChipPlan(") {
				t.Errorf("%s:%d calls ApplyChipPlan, which MUTATES the engine's "+
					"horizon and rebuilds its fixture index. The toolbox engine is "+
					"shared by every tool in the turn and the runner fans them out "+
					"through an errgroup, so this leaks into every later call and "+
					"races the ones running beside it. Build what you need without "+
					"writing to the shared engine — see Toolbox.researchSquad.",
					path, i+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the package tree: %v", err)
	}
	if scanned == 0 {
		t.Fatal("scanned no source files, so this guard proved nothing")
	}
}
