package analysis

import (
	"strings"
	"testing"

	"armband/internal/fpl"
)

// noAbsences returns weights with the mid-season tournament correction off, to
// compare against.
func noAbsences() Weights {
	w := DefaultWeights()
	w.TournamentAbsences = []TournamentAbsence{}
	return w
}

// TestTournamentAbsenceNamesResolve is the loud-failure guard. The correction is
// applied by name, so a spelling that stops matching does not error — it just
// silently stops crediting the player, and he quietly reverts to looking like a
// rotation risk.
func TestTournamentAbsenceNamesResolve(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	seen := map[int]string{}
	for _, tour := range DefaultTournamentAbsences() {
		if tour.Matches <= 0 {
			t.Errorf("%q has Matches=%d — the entry does nothing", tour.Name, tour.Matches)
		}
		if tour.Matches >= GameweeksPerSeason {
			t.Errorf("%q claims %d missed matches of %d", tour.Name, tour.Matches, GameweeksPerSeason)
		}
		for _, name := range tour.Players {
			matches := e.Boot.FindPlayers(name)
			if len(matches) == 0 {
				t.Errorf("%q (%s) matches no FPL player — check spelling and accents", name, tour.Name)
				continue
			}
			el := matches[0]
			full := el.FirstName + " " + el.SecondName
			if !strings.EqualFold(full, name) && !strings.EqualFold(el.WebName, name) {
				t.Errorf("%q resolved only fuzzily, to %q — ambiguous entries pick the wrong player", name, full)
			}
			if prev, dup := seen[el.ID]; dup {
				t.Errorf("%q and %q both resolve to element %d (%s)", prev, name, el.ID, el.WebName)
			}
			seen[el.ID] = name
		}
	}
}

// TestTournamentAbsenceRaisesMinutesForParticipants is the point of the change:
// a player who spent four weeks at a mid-season tournament should not be scored
// as though he was dropped for those matches.
func TestTournamentAbsenceRaisesMinutesForParticipants(t *testing.T) {
	on := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	off := roleEngine(t, noAbsences(), DefaultRoleRisk())

	var checked int
	for i := range on.Boot.Elements {
		el := &on.Boot.Elements[i]
		if el.Minutes == 0 {
			continue
		}
		if on.TeamMatchesStarted(el.Team) > 0 {
			// This assertion's GameweeksPerSeason-a.Matches is specifically the
			// PRE-SEASON window — the one matchesAvailable falls back to before a
			// club has kicked off. Once his own club's fixture has started,
			// matchesAvailable correctly narrows to TeamMatchesStarted instead
			// (see that function's own comment), which this test is not the
			// place to duplicate just to keep pace with a live match.
			continue
		}
		a := on.tournamentAbsence(el)
		if a.Matches == 0 {
			continue
		}
		checked++

		if got := on.matchesAvailable(el); got != GameweeksPerSeason-a.Matches {
			t.Errorf("%s: matchesAvailable = %d, want %d", el.WebName, got, GameweeksPerSeason-a.Matches)
		}
		if off.matchesAvailable(el) != GameweeksPerSeason {
			t.Errorf("%s: denominator moved with the correction disabled", el.WebName)
		}
		if on.expectedMinutes(el) <= off.expectedMinutes(el) {
			t.Errorf("%s: expected minutes %.1f did not rise above %.1f",
				el.WebName, on.expectedMinutes(el), off.expectedMinutes(el))
		}
		if on.minutesReliability(el) <= off.minutesReliability(el) {
			t.Errorf("%s: minutes reliability %.3f did not rise above %.3f",
				el.WebName, on.minutesReliability(el), off.minutesReliability(el))
		}
		if r := on.minutesReliability(el); r > 1 {
			t.Errorf("%s: minutes reliability %.3f exceeds 1", el.WebName, r)
		}
		if on.Metrics(el).TournamentAbsence == "" {
			t.Errorf("%s: correction applied but not reported on PlayerMetrics", el.WebName)
		}
	}
	if checked == 0 {
		t.Skip("no listed participant has minutes in this dataset")
	}
}

// TestTournamentAbsenceIsCappedByStarts guards the Semenyo case: a player who
// started nearly every match plainly did not miss six of them, so a wrong entry
// on the list must not invent minutes for him.
func TestTournamentAbsenceIsCappedByStarts(t *testing.T) {
	base := roleEngine(t, noAbsences(), DefaultRoleRisk())

	var ever *fpl.Element
	for i := range base.Boot.Elements {
		el := &base.Boot.Elements[i]
		if el.Starts > GameweeksPerSeason-3 && (ever == nil || el.Starts > ever.Starts) {
			ever = el
		}
	}
	if ever == nil {
		t.Skip("no near-ever-present in this dataset")
	}

	w := noAbsences()
	w.TournamentAbsences = []TournamentAbsence{{
		Name:    "fictional tournament",
		Matches: 6,
		Players: []string{ever.FirstName + " " + ever.SecondName},
	}}
	e := roleEngine(t, w, DefaultRoleRisk())

	want := GameweeksPerSeason - ever.Starts
	if got := e.matchesAvailable(ever); got != GameweeksPerSeason-want {
		t.Errorf("%s started %d/%d but was credited %d missed matches, want at most %d",
			ever.WebName, ever.Starts, GameweeksPerSeason, GameweeksPerSeason-got, want)
	}
	if e.minutesReliability(ever) > 1 {
		t.Errorf("%s: reliability exceeded 1 after the correction", ever.WebName)
	}
}

// TestTournamentAbsenceLeavesEveryoneElseAlone confirms the correction is
// targeted — it must not shift the scores of players who did not go.
func TestTournamentAbsenceLeavesEveryoneElseAlone(t *testing.T) {
	on := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	off := roleEngine(t, noAbsences(), DefaultRoleRisk())

	for i := range on.Boot.Elements {
		el := &on.Boot.Elements[i]
		if on.tournamentAbsence(el).Matches > 0 {
			continue
		}
		if on.minutesReliability(el) != off.minutesReliability(el) {
			t.Fatalf("%s is not on any list but his reliability changed: %.4f -> %.4f",
				el.WebName, off.minutesReliability(el), on.minutesReliability(el))
		}
	}
}
