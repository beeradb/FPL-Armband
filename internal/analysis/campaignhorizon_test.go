package analysis

import (
	"testing"
	"time"

	"armband/internal/fpl"
)

// TestCampaignsOverGameweeksSeesWhatTodayCannot pins the difference between the two
// questions a competition display can ask.
//
// ActiveCampaigns asks "what is live on this date". That is right for a status
// listing and wrong for anything reported next to a score, because a score is
// computed over a horizon of several gameweeks: a Champions League matchday a
// fortnight out is invisible to the date question and fully priced by the model.
//
// The failure is silent and reads as good news. `armband brief` asked the date
// question for a five-gameweek horizon and printed "league only" for all twenty
// clubs, immediately under an instruction telling the agent that competition status
// "changes every score below" — so the one paragraph most likely to be believed was
// the one that was wrong.
func TestCampaignsOverGameweeksSeesWhatTodayCannot(t *testing.T) {
	// Five weekly gameweeks from 21 August, the shape of an opening horizon.
	gw1 := time.Date(2026, 8, 21, 11, 0, 0, 0, time.UTC)
	var events []fpl.Event
	for i := 0; i < 5; i++ {
		events = append(events, fpl.Event{ID: i + 1, DeadlineTime: gw1.AddDate(0, 0, 7*i)})
	}

	const club = "ARS"
	e := &Engine{
		Boot: &fpl.Bootstrap{Events: events},
		Cong: Congestion{
			European: map[string][]CompetitionWindow{club: {
				// Starts after GW1 and inside the horizon — the case the date
				// question misses.
				{Competition: "UCL", Start: "2026-09-08"},
			}},
			DomesticCups: map[string][]CompetitionWindow{club: {
				// Already finished before the horizon opens.
				{Competition: "FAC", Start: "2026-01-04", End: "2026-05-16"},
			}},
		},
	}

	// The date question, asked on the day the brief would run.
	if got := e.ActiveCampaigns(club, gw1); len(got) != 0 {
		t.Fatalf("ActiveCampaigns on the GW1 deadline returned %d windows, want 0 — "+
			"the fixture is set up so that today sees nothing and the horizon sees "+
			"something, which is the whole comparison", len(got))
	}

	got := e.CampaignsOverGameweeks(club, 1, 5)
	if len(got) != 1 {
		t.Fatalf("the horizon question returned %d windows, want 1 (UCL, which starts "+
			"inside GW1-5); a finished cup must not be carried and a live campaign "+
			"must not be dropped", len(got))
	}
	if got[0].Window.Competition != "UCL" {
		t.Errorf("returned %q, want UCL — the finished FA Cup window leaked through",
			got[0].Window.Competition)
	}
	// 8 September falls between the GW3 and GW4 deadlines (4 and 11 September), and
	// a window with no MatchDates covers every gameweek it spans, so the commitment
	// starts at GW4 and runs to the end of the horizon.
	if want := []int{4, 5}; !sameInts(got[0].Gameweeks, want) {
		t.Errorf("UCL touches gameweeks %v, want %v", got[0].Gameweeks, want)
	}

	// A club with nothing configured must come back empty rather than inheriting
	// another club's campaign — a shared-map bug here would show as every club
	// carrying a European penalty.
	if other := e.CampaignsOverGameweeks("BUR", 1, 5); len(other) != 0 {
		t.Errorf("an unconfigured club returned %d windows, want 0", len(other))
	}

	// Gameweeks with no published deadline are skipped rather than guessed at, so a
	// range running past the calendar simply yields less. Asking beyond the end
	// must not report the campaign as continuing.
	if past := e.CampaignsOverGameweeks(club, 30, 38); len(past) != 0 {
		t.Errorf("a range with no published deadlines returned %d windows, want 0 — "+
			"an unknown deadline is not evidence of a fixture", len(past))
	}
}

// TestCampaignsOverGameweeksHonoursMatchDates — the horizon question must use the
// same predicate scoring uses, or the brief and the congestion factor describe
// different seasons.
//
// CoversGameweek treats a window that lists specific fixture dates as affecting
// only the gameweeks within a few days of one, because that is when a midweek match
// actually costs a manager anything. A display that ignored MatchDates would report
// a club as committed for every week of a campaign it plays four matches in.
func TestCampaignsOverGameweeksHonoursMatchDates(t *testing.T) {
	gw1 := time.Date(2026, 8, 21, 11, 0, 0, 0, time.UTC)
	var events []fpl.Event
	for i := 0; i < 5; i++ {
		events = append(events, fpl.Event{ID: i + 1, DeadlineTime: gw1.AddDate(0, 0, 7*i)})
	}
	const club = "CHE"
	e := &Engine{
		Boot: &fpl.Bootstrap{Events: events},
		Cong: Congestion{European: map[string][]CompetitionWindow{club: {{
			Competition: "UEL", Start: "2026-08-01",
			// One listed match, three days after the GW3 deadline.
			MatchDates: []string{"2026-09-07"},
		}}}},
	}

	got := e.CampaignsOverGameweeks(club, 1, 5)
	if len(got) != 1 {
		t.Fatalf("returned %d windows, want 1", len(got))
	}
	// Within five days either side of 7 September: the GW3 deadline (4 Sept) and the
	// GW4 one (11 Sept) are 3 and 4 days away, GW2 (28 Aug) is 10 and out.
	if want := []int{3, 4}; !sameInts(got[0].Gameweeks, want) {
		t.Errorf("a single listed match touches gameweeks %v, want %v — a window with "+
			"MatchDates must not read as a commitment in every week it spans",
			got[0].Gameweeks, want)
	}
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
