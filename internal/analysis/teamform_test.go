package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// mockTeamFormSource is a test implementation of TeamFormSource.
type mockTeamFormSource struct {
	data map[int][5]interface{} // teamID -> [recent, season, recentMatches, seasonMatches, ok]
}

func (m mockTeamFormSource) TeamForm(teamID int) (recent, season float64, recentMatches, seasonMatches int, ok bool) {
	vals, found := m.data[teamID]
	if !found {
		return 0, 0, 0, 0, false
	}
	return vals[0].(float64), vals[1].(float64), vals[2].(int), vals[3].(int), vals[4].(bool)
}

// TestTeamFormFactorRespectsMatchCountGate ensures teamFormFactor correctly gates
// on the match-count floor. This is the regression test for the dead-code bug where
// teamFormMinMatches was declared but not checked in analysis.teamFormFactor.
func TestTeamFormFactorRespectsMatchCountGate(t *testing.T) {
	// Save and restore the global weight so this test doesn't affect others.
	oldWeight := teamFormWeight
	defer func() { teamFormWeight = oldWeight }()

	// Set the weight to enable the feature.
	SetTeamFormWeight(0.5)

	// Create a mock engine with mock boot data and mock source.
	e := &Engine{
		Boot: &fpl.Bootstrap{
			Teams: []fpl.Team{
				{ID: 1, ShortName: "ARS"},
				{ID: 2, ShortName: "AVL"},
				{ID: 3, ShortName: "BHA"},
			},
		},
		teamForm: teamFormFactors{},
	}

	// Create a mock source with three clubs:
	// - Club 1: sufficient matches in both windows -> should be included
	// - Club 2: sufficient season matches but insufficient recent matches -> should be excluded
	// - Club 3: insufficient matches in both windows -> should be excluded
	e.TeamForm = mockTeamFormSource{
		data: map[int][5]interface{}{
			1: {1.1, 0.95, 5, 5, true}, // recent=1.1, season=0.95, recentMatches=5, seasonMatches=5, ok=true
			2: {1.2, 1.0, 3, 6, true},  // recentMatches=3 (below threshold), seasonMatches=6
			3: {1.0, 1.0, 2, 2, true},  // both below threshold
		},
	}

	// Call teamFormFactor, which will build the cache.
	factor1 := e.teamFormFactor(1)
	factor2 := e.teamFormFactor(2)
	factor3 := e.teamFormFactor(3)

	// Club 1 should have a non-neutral factor since both windows have >= 4 matches.
	if factor1 == 1.0 {
		t.Errorf("club 1 should have a correction applied, got factor %v", factor1)
	}

	// Club 2 should return neutral (1.0) because recent matches (3) < teamFormMinMatches (4).
	if factor2 != 1.0 {
		t.Errorf("club 2 with insufficient recent matches should be gated out and return 1.0, got %v", factor2)
	}

	// Club 3 should return neutral (1.0) because both windows are below the threshold.
	if factor3 != 1.0 {
		t.Errorf("club 3 with insufficient matches should be gated out and return 1.0, got %v", factor3)
	}
}

// TestTeamFormFactorIsInertWhenWeightIsZero verifies that the feature is completely
// disabled when the weight is zero.
func TestTeamFormFactorIsInertWhenWeightIsZero(t *testing.T) {
	oldWeight := teamFormWeight
	defer func() { teamFormWeight = oldWeight }()

	SetTeamFormWeight(0)

	e := &Engine{
		Boot: &fpl.Bootstrap{Teams: []fpl.Team{{ID: 1}}},
		TeamForm: mockTeamFormSource{
			data: map[int][5]interface{}{
				1: {1.5, 0.9, 5, 5, true},
			},
		},
		teamForm: teamFormFactors{},
	}

	// With weight 0, should always return neutral factor.
	if factor := e.teamFormFactor(1); factor != 1.0 {
		t.Errorf("with weight 0, teamFormFactor should return 1.0, got %v", factor)
	}
}

// TestTeamFormFactorIsInertWhenSourceIsNil verifies that the feature degrades
// gracefully when no source is available.
func TestTeamFormFactorIsInertWhenSourceIsNil(t *testing.T) {
	oldWeight := teamFormWeight
	defer func() { teamFormWeight = oldWeight }()

	SetTeamFormWeight(0.5)

	e := &Engine{
		Boot:     &fpl.Bootstrap{Teams: []fpl.Team{{ID: 1}}},
		TeamForm: nil, // No source available
		teamForm: teamFormFactors{},
	}

	// With no source, should always return neutral factor.
	if factor := e.teamFormFactor(1); factor != 1.0 {
		t.Errorf("with no source, teamFormFactor should return 1.0, got %v", factor)
	}
}
