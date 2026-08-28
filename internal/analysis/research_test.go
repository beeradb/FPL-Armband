package analysis

import (
	"strings"
	"testing"
)

// TestResearchTargetsFindTheZeroDataBlindSpot is the case that motivated the
// whole step: a player with meaningful ownership and no minutes for the model to
// read has to reach a human, because the model cannot settle him.
//
// ⚠️ **This test used to assert the DEFECT.** It required `p.Score == 0` for
// every such player, and failed loudly the moment a player with no prior season
// stopped leaving `blendRatesCode` at zero expected minutes. That assertion was
// a faithful description of the behaviour when it was written and became a lock
// on a bug — the shape worth recognising is a test that says "this must stay
// broken" without ever using the word.
//
// What it pins now is the surfacing, which is the step's actual job, and the
// split that makes the advice attached to it correct.
func TestResearchTargetsFindTheZeroDataBlindSpot(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	cats := e.ResearchTargets(nil)
	if len(cats) == 0 {
		t.Fatal("no research categories produced")
	}

	var found bool
	for _, c := range cats {
		for _, p := range c.Targets {
			if p.Minutes == 0 && p.Ownership >= researchOwnershipFloor {
				found = true
			}
		}
	}
	if !found {
		t.Error("no zero-minutes player was surfaced despite meaningful ownership — " +
			"promoted-club starters are exactly what this step exists to catch")
	}
}

// TestTheGuessedAndTheEvidencedAreSeparateCategories pins the split, because the
// two populations want opposite advice and one list cannot carry both.
//
// A player the model has never seen is scored from his position's league
// average, which over-states his minutes; a player it has a full prior season
// for is simply yet to play. Told apart, the first is "discount this" and the
// second is "check for an injury". Told together — which is what `Minutes == 0`
// alone gives — the section has to pick one recommendation and is wrong for half
// its rows.
//
// ⚠️ The engine here has NO prior index, which is a supported state
// `blendRatesCode` has always guarded and this step did not: reaching through a
// nil `Priors` panicked the whole research step. With no priors nobody has one,
// so every surfaced player must land in the guessing category and none in the
// evidenced one. That degenerate case is the assertion, because it is also the
// regression test for the panic.
func TestTheGuessedAndTheEvidencedAreSeparateCategories(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	if e.Priors != nil {
		t.Fatal("this test needs an engine with no prior index, which is what " +
			"roleEngine builds; something now sets Priors and the nil-guard " +
			"regression below would no longer be exercised")
	}

	var guessed, evidenced int
	for _, c := range e.ResearchTargets(nil) {
		switch {
		case strings.Contains(c.Name, "No Premier League history"):
			guessed = len(c.Targets)
		case strings.Contains(c.Name, "full prior season"):
			evidenced = len(c.Targets)
		}
	}
	if guessed == 0 {
		t.Error("no player reached the guessing category, but with a nil prior " +
			"index every zero-minutes player has no prior by definition")
	}
	if evidenced != 0 {
		t.Errorf("%d players were called evidence-backed by an engine that holds "+
			"no prior season at all", evidenced)
	}
}

// TestResearchTargetsAreNotDuplicated — a player belongs to one blind spot, so
// the agent never spends two searches on the same name.
func TestResearchTargetsAreNotDuplicated(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	seen := map[int]string{}
	for _, c := range e.ResearchTargets(nil) {
		for _, p := range c.Targets {
			if prev, dup := seen[p.ID]; dup {
				t.Errorf("%s appears under both %q and %q", p.Name, prev, c.Name)
			}
			seen[p.ID] = c.Name
		}
	}
}

// TestResearchTargetsStayBounded keeps the step cheap. Each target is roughly
// one web search, and an unbounded list quietly turns a free analysis step into
// an expensive agent run.
func TestResearchTargetsStayBounded(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	total := 0
	for _, c := range e.ResearchTargets(nil) {
		if len(c.Targets) > researchPerCategory {
			t.Errorf("category %q has %d targets, cap is %d", c.Name, len(c.Targets), researchPerCategory)
		}
		if c.Why == "" || c.Ask == "" {
			t.Errorf("category %q has no reason or no question; the agent needs both", c.Name)
		}
		total += len(c.Targets)
	}
	t.Logf("%d targets across %d categories", total, len(e.ResearchTargets(nil)))
}

// TestResearchTargetsIncludeShakySquadMembers — the recommendation depends on
// these players starting, and last season's minutes do not establish that.
func TestResearchTargetsIncludeShakySquadMembers(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	// minutesCorroborated needs corroboratingMatches (2) before a squad member
	// can read "nailed" rather than "likely starter".
	skipUntilLiveEvidence(t, e, corroboratingMatches)
	sq, err := e.Optimize(OptimizeRequest{
		Budget: DefaultBudget, MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02,
	})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}

	var shaky []PlayerMetrics
	for _, p := range sq.Players {
		if p.RotationRisk != "nailed" {
			shaky = append(shaky, p)
		}
	}
	if len(shaky) == 0 {
		t.Skip("every squad member is nailed")
	}

	surfaced := map[int]bool{}
	for _, c := range e.ResearchTargets(sq.Players) {
		for _, p := range c.Targets {
			surfaced[p.ID] = true
		}
	}
	// Squad members may be claimed by an earlier category; what matters is that
	// they appear somewhere, not which group they land in.
	var missed []string
	for _, p := range shaky {
		if !surfaced[p.ID] {
			missed = append(missed, p.Name)
		}
	}
	if len(missed) > 0 {
		t.Errorf("squad members not nailed and not flagged for research: %v", missed)
	}
}
