package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// playGameweeks marks the first n gameweeks finished, so the engine believes
// that much of the season has been played.
func playGameweeks(t *testing.T, e *Engine, n int) {
	t.Helper()
	for i := range e.Boot.Events {
		e.Boot.Events[i].Finished = i < n
	}
	if got := e.GameweeksPlayed(); got != n {
		t.Fatalf("set up %d finished gameweeks, engine sees %d", n, got)
	}
}

// findEverPresent returns a player who started nearly every match last season,
// to stand in for an ever-present during the simulated season.
func findEverPresent(t *testing.T, e *Engine) *fpl.Element {
	t.Helper()
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Starts >= 35 && el.Minutes >= 3000 && el.ElementType == 3 {
			// Skip anyone on the post-tournament rest list. That term scales
			// expected minutes, so a flagged player legitimately reports below 90
			// and would fail callers asserting about something else entirely —
			// the data window, rate shrinkage, the recency hook.
			if _, f := e.restFactor(el); f < 1 {
				continue
			}
			// And skip anyone unavailable. Every caller compares scores before
			// and after some change, and an injured player scores zero both
			// times, so the comparison silently proves nothing.
			if availabilityFactor(el) == 0 {
				continue
			}
			return el
		}
	}
	t.Skip("no unrested ever-present midfielder in the dataset")
	return nil
}

// TestDataWindowTracksTheSeason is the guard on the bug that made the model
// unusable the moment a ball was kicked.
//
// FPL's `minutes` field carries last season's total until GW1 completes, then
// resets and accumulates. The denominator has to follow it. Dividing one
// gameweek's 90 minutes by a fixed 38 reports an ever-present as 2.4 minutes per
// gameweek — every player in the game lands in the "fringe" band, scored at
// about 1% of true value, and nothing recovers until roughly GW29.
func TestDataWindowTracksTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	if got := e.DataWindow(); got != GameweeksPerSeason {
		t.Errorf("pre-season data window is %d, want %d — FPL still holds last "+
			"season's totals, so the window is a full season", got, GameweeksPerSeason)
	}

	el := findEverPresent(t, e)
	trueMins, trueStarts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = trueMins, trueStarts }()

	for _, gw := range []int{1, 2, 3, 5, 10, 19, 28, 38} {
		playGameweeks(t, e, gw)
		if got := e.DataWindow(); got != gw {
			t.Fatalf("after GW%d the data window is %d, want %d", gw, got, gw)
		}

		// An ever-present: he has played every minute of every match so far.
		el.Minutes, el.Starts = gw*90, gw
		m := e.Metrics(el)

		if m.ExpectedMinutes < 85 || m.ExpectedMinutes > 95 {
			t.Errorf("after GW%d an ever-present shows %.1f expected minutes, want ~90",
				gw, m.ExpectedMinutes)
		}
		if m.RotationRisk != "nailed" {
			t.Errorf("after GW%d an ever-present is banded %q, want nailed",
				gw, m.RotationRisk)
		}
		if m.MinutesRating < 0.9 {
			t.Errorf("after GW%d an ever-present has reliability %.3f, want ~1.0",
				gw, m.MinutesRating)
		}
	}
}

// TestMinutesFloorScalesWithTheSeason guards the second half of the same bug. A
// 600-minute floor is written as a season total; applied unscaled it excludes
// every player in the game until about GW26, and the optimiser fails outright
// rather than returning a bad squad.
func TestMinutesFloorScalesWithTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	defer playGameweeks(t, e, 0)

	for _, gw := range []int{1, 3, 8} {
		playGameweeks(t, e, gw)
		sq, err := e.Optimize(OptimizeRequest{
			Budget: DefaultBudget, MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02,
		})
		if err != nil {
			t.Fatalf("after GW%d the optimiser failed with a season-scale minutes floor: %v", gw, err)
		}
		if !squadIsLegal(sq.Players, DefaultBudget) {
			t.Errorf("after GW%d the optimiser returned an illegal squad", gw)
		}
	}
}

// TestTournamentAbsencesStopApplyingInSeason — the absence list describes the
// season the aggregates came from. Once gameweeks are played FPL has overwritten
// those aggregates, so last summer's list describes data that is no longer in
// hand and must not shrink the denominator.
func TestTournamentAbsencesStopApplyingInSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	defer playGameweeks(t, e, 0)

	var listed *fpl.Element
	for i := range e.Boot.Elements {
		if e.tournamentAbsence(&e.Boot.Elements[i]).Matches > 0 {
			listed = &e.Boot.Elements[i]
			break
		}
	}
	if listed == nil {
		t.Skip("no player carries a tournament absence")
	}
	if e.matchesAvailable(listed) >= GameweeksPerSeason {
		t.Error("pre-season, a listed player's denominator should be reduced")
	}

	playGameweeks(t, e, 5)
	if a := e.tournamentAbsence(listed); a.Matches != 0 {
		t.Errorf("%s still carries a %d-match absence from last season's list "+
			"after 5 gameweeks of this one", listed.WebName, a.Matches)
	}
	if got := e.matchesAvailable(listed); got != 5 {
		t.Errorf("%s has a denominator of %d after 5 gameweeks, want 5", listed.WebName, got)
	}
}
