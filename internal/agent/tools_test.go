package agent

import (
	"testing"

	"armband/internal/fpl"
)

// TestTheAgentIsNotToldAnUnmeasuredSeasonWasZero guards the agent-facing half of
// FPL's explicit-zero problem.
//
// It matters more than the scoring half, for two reasons the diff cannot show.
// This path is live on every `seasons: true` lookup rather than gated behind a
// setting that ships off, and tool output is replayed on every subsequent API
// call — so a wrong number here is paid for repeatedly and can anchor a whole
// conversation.
func TestTheAgentIsNotToldAnUnmeasuredSeasonWasZero(t *testing.T) {
	got := pastSeasonsForTool([]fpl.PastSeason{
		// Dunk's real 2018/19: a full season of football, and FPL reports
		// "0.00" for every expected statistic because none existed yet.
		{SeasonName: "2018/19", Minutes: 3151, GoalsConceded: 56},
		// 2023/24 has the expected statistics but not defensive contribution.
		{SeasonName: "2023/24", Minutes: 2869, ExpectedGoalsConceded: 49.76},
		// 2024/25 has both.
		{SeasonName: "2024/25", Minutes: 2081, ExpectedGoalsConceded: 35.69,
			DefensiveContribution: 126},
	})
	if len(got) != 3 {
		t.Fatalf("got %d seasons, want 3", len(got))
	}

	// The season itself must survive: the minutes and goals conceded are real
	// and are the whole reason an agent asks for past seasons.
	if got[0]["minutes"] != float64(3151) || got[0]["goals_conceded"] != float64(56) {
		t.Errorf("2018/19 lost its real statistics: %v", got[0])
	}
	for _, k := range []string{"expected_goals", "expected_assists",
		"expected_goals_conceded", "defensive_contribution"} {
		if v, ok := got[0][k]; ok {
			t.Errorf("2018/19 still reports %s = %v; FPL never measured it, and a "+
				"zero handed to the model is a number rather than a gap", k, v)
		}
	}

	// A season that DOES carry the statistic must keep it — the guard must not
	// be a blanket strip, which would read as a fix while deleting real evidence.
	if got[1]["expected_goals_conceded"] != 49.76 {
		t.Errorf("2023/24 lost its real xGC: %v", got[1]["expected_goals_conceded"])
	}
	if _, ok := got[1]["defensive_contribution"]; ok {
		t.Error("2023/24 still reports defensive contribution, which arrives in 2024/25")
	}
	if got[2]["defensive_contribution"] != float64(126) {
		t.Errorf("2024/25 lost its real defcon: %v", got[2]["defensive_contribution"])
	}
}
