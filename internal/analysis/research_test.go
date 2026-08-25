package analysis

import "testing"

// TestResearchTargetsFindTheZeroDataBlindSpot is the case that motivated the
// whole step. A promoted club's first-choice defender and their fourth-choice
// goalkeeper both score 0.00, because neither has Premier League minutes. The
// model cannot separate them, so the list has to surface them for a human or an
// agent to settle.
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
				if p.Score != 0 {
					t.Errorf("%s has no minutes but scores %.2f", p.Name, p.Score)
				}
			}
		}
	}
	if !found {
		t.Error("no zero-minutes player was surfaced despite meaningful ownership — " +
			"promoted-club starters are exactly what this step exists to catch")
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
