package viewmodel

import (
	"strings"
	"testing"

	"armband/internal/fpl"
)

// sumChannels sums every line EXCEPT a trailing "Captain (×N)" line, which is not an FPL
// scoring channel -- see scoreBreakdown's own comment for why it is appended separately,
// after the channels below already reconcile against the undoubled Points.
func sumChannels(lines []ScoreLine) int {
	sum := 0
	for _, l := range lines {
		if strings.HasPrefix(l.Label, "Captain") {
			continue
		}
		sum += l.Points
	}
	return sum
}

// TestScoreBreakdownReconcilesExactly is Defect 4's pinning test: the spec's own
// requirement is that the computed channels sum EXACTLY to FPL's total_points, for every
// player in the fixture below, which deliberately exercises every channel
// scoreBreakdown emits — including a negative total (own goal + missed penalty
// outweighing the appearance point), which ScoreLine's own comment says must render
// correctly, not be treated as a special case.
func TestScoreBreakdownReconcilesExactly(t *testing.T) {
	cases := []struct {
		name  string
		pos   int
		stats fpl.LiveStats
	}{
		{"GKP: saves + bonus", 1, fpl.LiveStats{
			Minutes: 90, Saves: 4, Bonus: 2, TotalPoints: 5,
		}},
		{"GKP: penalty save + concede block + saves", 1, fpl.LiveStats{
			Minutes: 90, Saves: 6, PenaltiesSaved: 1, GoalsConceded: 3, TotalPoints: 8,
		}},
		{"DEF: clean sheet + DefCon + bonus", 2, fpl.LiveStats{
			Minutes: 90, CleanSheets: 1, DefensiveContribution: 11, Bonus: 3, TotalPoints: 11,
		}},
		{"DEF: red card + concede, short of an hour", 2, fpl.LiveStats{
			Minutes: 45, RedCards: 1, GoalsConceded: 2, TotalPoints: -3,
		}},
		{"MID: brace + assist + yellow + bonus", 3, fpl.LiveStats{
			Minutes: 65, GoalsScored: 2, Assists: 1, YellowCards: 1, Bonus: 1, TotalPoints: 15,
		}},
		{"FWD: own goal + penalty miss, no bonus", 4, fpl.LiveStats{
			Minutes: 90, OwnGoals: 1, PenaltiesMissed: 1, TotalPoints: -2,
		}},
		{"unplayed: every channel zero", 3, fpl.LiveStats{
			Minutes: 0, TotalPoints: 0,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := scoreBreakdown(c.pos, c.stats, c.stats.TotalPoints, 1)
			if got := sumChannels(lines); got != c.stats.TotalPoints {
				t.Errorf("sum of breakdown channels = %d, want %d (FPL's total_points) — lines: %+v",
					got, c.stats.TotalPoints, lines)
			}
		})
	}
}

// TestScoreBreakdownCaptainLineCarriesTheDifference pins the spec's other reconciliation
// requirement: the breakdown is attached to .tppts, which shows the DOUBLED (or
// tripled) figure for a captain, so the full line list — channels plus the trailing
// "Captain (×N)" line — must sum to Points*Multiplier, not to the undoubled Points.
func TestScoreBreakdownCaptainLineCarriesTheDifference(t *testing.T) {
	stats := fpl.LiveStats{Minutes: 65, GoalsScored: 2, Assists: 1, YellowCards: 1, Bonus: 1, TotalPoints: 15}
	lines := scoreBreakdown(3, stats, stats.TotalPoints, 2)

	last := lines[len(lines)-1]
	if !strings.HasPrefix(last.Label, "Captain") {
		t.Fatalf("last line = %+v, want a trailing Captain line", last)
	}
	if last.Points != stats.TotalPoints {
		t.Errorf("Captain line Points = %d, want %d (the doubled half)", last.Points, stats.TotalPoints)
	}

	total := 0
	for _, l := range lines {
		total += l.Points
	}
	if want := stats.TotalPoints * 2; total != want {
		t.Errorf("full breakdown sum = %d, want %d (Points × Multiplier, the number .tppts shows)", total, want)
	}
}

// TestScoreBreakdownWithheldOnMismatch pins the spec's refusal rule: a breakdown that
// does not sum exactly to FPL's own total_points must not render at all, and must not
// invent a balancing line to force agreement. Constructed with an inconsistent
// TotalPoints (a stats row FPL could not actually produce) so DecomposeMatch's own total
// disagrees with it by construction.
func TestScoreBreakdownWithheldOnMismatch(t *testing.T) {
	stats := fpl.LiveStats{Minutes: 90, GoalsScored: 1, TotalPoints: 999}
	if lines := scoreBreakdown(4, stats, stats.TotalPoints, 1); lines != nil {
		t.Errorf("scoreBreakdown = %+v, want nil — the computed channels do not sum to 999 "+
			"and a disagreeing breakdown must be withheld, not shown wrong", lines)
	}
}
