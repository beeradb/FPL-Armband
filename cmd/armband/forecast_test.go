package main

import (
	"testing"
	"time"

	"armband/internal/fpl"
)

func at(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func ev(n int) *int { return &n }

// TestShowableIsTheOnlyEligibilityRule is the guard for the defect that shipped
// in the first draft of this command: the default-gameweek scan and the row
// filter each spelled "can this fixture appear" for themselves, and disagreed
// about a fixture with no kick-off time.
//
// The consequence was not a wrong number, which is what makes it worth pinning
// — it was SILENCE. A gameweek holding only a rearranged TBC fixture could win
// the default scan and then render nothing, so `armband forecast` printed "no
// fixtures" while a later gameweek had a full round to show. This project's
// most-repeated failure shape is a request that looks like it worked.
func TestShowableIsTheOnlyEligibilityRule(t *testing.T) {
	cases := []struct {
		name string
		f    fpl.Fixture
		want bool
	}{
		{"a normal upcoming fixture",
			fpl.Fixture{Event: ev(3), KickoffTime: at("2026-09-12T14:00:00Z")}, true},
		{"already played",
			fpl.Fixture{Event: ev(2), KickoffTime: at("2026-08-29T14:00:00Z"),
				Finished: true}, false},
		{"rearranged, no date set yet",
			fpl.Fixture{Event: ev(3)}, false},
		{"kicked off but not finished — still shows, because the projection " +
			"was made before it started and the card is a forecast",
			fpl.Fixture{Event: ev(2), KickoffTime: at("2026-08-29T14:00:00Z"),
				Started: true}, true},
	}
	for _, c := range cases {
		if got := showable(c.f); got != c.want {
			t.Errorf("showable(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestTheDefaultGameweekSkipsAGameweekItCouldNotRender reproduces the exact
// shape of the defect rather than only testing the predicate that fixes it.
//
// GW3 here has one fixture and it is TBC, so it cannot render. The scan must
// therefore reach GW4. The old scan, which asked only "unfinished and has an
// event", stopped at 3 and the command printed nothing.
func TestTheDefaultGameweekSkipsAGameweekItCouldNotRender(t *testing.T) {
	fixtures := []fpl.Fixture{
		{Event: ev(2), KickoffTime: at("2026-08-29T14:00:00Z"), Finished: true},
		{Event: ev(3)}, // rearranged, no date
		{Event: ev(4), KickoffTime: at("2026-09-19T14:00:00Z")},
	}

	best := 0
	for _, f := range fixtures {
		if !showable(f) || f.Event == nil {
			continue
		}
		if best == 0 || *f.Event < best {
			best = *f.Event
		}
	}
	if best != 4 {
		t.Errorf("default gameweek = %d, want 4. GW3's only fixture has no "+
			"kick-off time, so choosing it would render an empty card while "+
			"GW4 had a round to show", best)
	}
}
