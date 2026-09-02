package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// matchesAvailable is a rate DENOMINATOR, so it must count the matches the player's
// own CLUB has played — not the league-wide count of finished gameweeks.
//
// Clubs do not play in step, and FPL's per-gameweek `finished` flag lags the last
// final whistle. In that window a club's fixture is over and its players' minutes
// are in the numerator while the gameweek is not yet in the denominator. Measured
// two gameweeks into 2026-27: every element carried matches_available 1 while ten
// clubs had two finished fixtures, so a keeper on two full games read 180 minutes
// per match. Fed into the positional league rate that produced a goalkeeper baseline
// of 135, shrinkToLeague then handed 112.5 expected minutes to keepers who had never
// played — 49 players projected above the 90 a gameweek contains.
func TestMatchesAvailableCountsTheClubsOwnFinishedFixtures(t *testing.T) {
	const club, other = 7, 9
	boot := &fpl.Bootstrap{
		Events: []fpl.Event{
			{ID: 1, Finished: true},
			// GW2's football is played but FPL has not flipped the flag, which is
			// the window this exists to survive.
			{ID: 2, Finished: false, IsCurrent: true},
			{ID: 3, Finished: false, IsNext: true},
		},
	}
	e := &Engine{Boot: boot, Fixtures: []fpl.Fixture{
		{TeamH: club, TeamA: other, FinishedProvisional: true},
		{TeamH: other, TeamA: club, FinishedProvisional: true},
	}}
	if got := e.DataWindow(); got != 1 {
		t.Fatalf("DataWindow = %d, want 1 — the premise of this test is that the "+
			"league-wide count lags", got)
	}
	el := &fpl.Element{Team: club, Minutes: 180}
	if got := e.matchesAvailable(el); got != 2 {
		t.Fatalf("matchesAvailable = %d, want 2: his club has played twice, so 180 "+
			"minutes is 90 a match and not %d", got, 180/max(got, 1))
	}
}

// A club that genuinely has played fewer matches must not be given the league's
// count — the fix takes the larger of the two, so it can only ever raise a
// denominator that was too small, never shrink a correct one.
func TestMatchesAvailableNeverShrinksBelowTheLeagueWindow(t *testing.T) {
	boot := &fpl.Bootstrap{Events: []fpl.Event{
		{ID: 1, Finished: true}, {ID: 2, Finished: true}, {ID: 3, Finished: false, IsNext: true},
	}}
	// A club with one postponed fixture: the league has finished two gameweeks,
	// this club has played once.
	e := &Engine{Boot: boot, Fixtures: []fpl.Fixture{{TeamH: 4, TeamA: 5, FinishedProvisional: true}}}
	if got := e.matchesAvailable(&fpl.Element{Team: 4}); got != 2 {
		t.Fatalf("matchesAvailable = %d, want 2 — the league window still applies "+
			"when a club is behind it", got)
	}
}

// Pre-season the window is a whole season and no fixture has finished, so the
// club-level count must not collapse the denominator to zero.
func TestMatchesAvailablePreSeason(t *testing.T) {
	boot := &fpl.Bootstrap{Events: []fpl.Event{{ID: 1, Finished: false, IsNext: true}}}
	e := &Engine{Boot: boot}
	if got := e.matchesAvailable(&fpl.Element{Team: 3}); got != GameweeksPerSeason {
		t.Fatalf("matchesAvailable = %d, want %d pre-season", got, GameweeksPerSeason)
	}
}
