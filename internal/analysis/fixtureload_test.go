package analysis

import "testing"

// TestFixtureLoadCountsDoublesAndBlanks — Score is a per-gameweek expectation
// built from per-90 rates, and nothing in that arithmetic knows how many times
// the club plays. Without this a double gameweek scored the same as a single
// one, which is a factor of two in the week it matters.
func TestFixtureLoadCountsDoublesAndBlanks(t *testing.T) {
	e := &Engine{}
	brief := func(events ...int) []FixtureBrief {
		var out []FixtureBrief
		for _, ev := range events {
			out = append(out, FixtureBrief{Event: ev})
		}
		return out
	}

	if got := e.fixturesPerGameweek(brief(5, 6, 7, 8, 9)); got != 1 {
		t.Errorf("five ordinary gameweeks gave %.3f, want 1", got)
	}
	// A double: five fixtures inside four gameweeks.
	if got := e.fixturesPerGameweek(brief(5, 6, 6, 7, 8)); got != 1.25 {
		t.Errorf("a double gave %.3f, want 1.25", got)
	}
	// A blank: five fixtures spread over six gameweeks. Counting *distinct*
	// gameweeks would report 1.0 here and miss it entirely, which is why the
	// denominator is the span.
	if got := e.fixturesPerGameweek(brief(5, 6, 8, 9, 10)); got >= 1 {
		t.Errorf("a blank gave %.3f, want below 1", got)
	}
	// No fixtures at all is not information; leave the score alone.
	if got := e.fixturesPerGameweek(nil); got != 1 {
		t.Errorf("empty fixture list gave %.3f, want 1", got)
	}
	// A lone fixture spans one gameweek and must not read as a double.
	if got := e.fixturesPerGameweek(brief(12)); got != 1 {
		t.Errorf("single fixture gave %.3f, want 1", got)
	}
}
