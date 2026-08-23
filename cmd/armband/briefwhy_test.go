package main

import (
	"strings"
	"testing"

	"armband/internal/analysis"
)

func whyPlayer(id int, name, team, pos string, price, score float64) analysis.PlayerMetrics {
	return analysis.PlayerMetrics{ID: id, Name: name, Team: team, Position: pos,
		Price: price, Score: score, ExpectedMinutes: 85, AvgDifficulty: 3}
}

// Money is counted in tenths everywhere else in this codebase, and the bank line has to
// agree. £0.04m left is not "some money left", and a float compared against zero would
// say it was.
//
// The line earns its place late in the season rather than now: at £0.0m it reads as
// noise, at £0.1m or £0.2m it is the constraint deciding what a reader can do.
func TestTheBankLineRoundsToTenthsRatherThanComparingAFloatToZero(t *testing.T) {
	for _, c := range []struct {
		remaining float64
		want      string
	}{
		{0.0, "The budget is spent"},
		{0.04, "The budget is spent"}, // rounds to 0 tenths
		{0.1, "£0.1m left over"},      // the case this line exists for
		{2.5, "£2.5m left over"},
	} {
		var b strings.Builder
		briefWhyThisFifteen(&b, 6, &analysis.Squad{Remaining: c.remaining})
		if got := b.String(); !strings.Contains(got, c.want) {
			t.Errorf("£%.2fm remaining: want %q, got:\n%s", c.remaining, c.want, got)
		}
	}
}

// Every club at the limit gets named, and the list reads as a sentence.
//
// strings.Join with " and " produced "COV and HUL and IPS" on a real run.
func TestFullClubsAreNamedAsASentence(t *testing.T) {
	var b strings.Builder
	briefWhyThisFifteen(&b, 6, &analysis.Squad{
		ClubCounts: map[string]int{"COV": 3, "HUL": 3, "IPS": 3, "ARS": 2}})

	got := b.String()
	if !strings.Contains(got, "COV, HUL and IPS") {
		t.Errorf("full clubs should read as a sentence, got:\n%s", got)
	}
	if strings.Contains(got, "ARS") {
		t.Error("a club under the limit was reported as full")
	}
}

// A squad with no club at the limit must not print an empty or dangling bullet.
func TestNoFullClubsPrintsNoClubBullet(t *testing.T) {
	var b strings.Builder
	briefWhyThisFifteen(&b, 6, &analysis.Squad{ClubCounts: map[string]int{"ARS": 2}})
	if got := b.String(); strings.Contains(got, "full") {
		t.Errorf("no club is at the limit, so no club bullet is owed:\n%s", got)
	}
}

// The horizon is the answer to "over what?" for every number in this section, so it is
// stated rather than left to the reader.
func TestTheHorizonIsStated(t *testing.T) {
	var b strings.Builder
	briefWhyThisFifteen(&b, 4, &analysis.Squad{})
	if got := b.String(); !strings.Contains(got, "next 4 gameweeks") {
		t.Errorf("the horizon should be named, got:\n%s", got)
	}
}

// ⚠️ The withdrawn half must stay withdrawn. An earlier version named the best upgrade
// the budget could not reach and the cash it would take. It was cut because a reader
// cannot conjure money, so a shortfall he cannot close is not an option — and because
// the search behind it was wrong twice (it read the whole player pool rather than the
// one Optimize was allowed to use, and it scored candidates on a different objective
// from the one the optimiser climbs). Reintroducing it here rather than on the transfer
// surface would bring both back.
func TestTheSectionDoesNotSearchForUpgrades(t *testing.T) {
	var b strings.Builder
	briefWhyThisFifteen(&b, 6, &analysis.Squad{
		Players:    []analysis.PlayerMetrics{whyPlayer(1, "A", "AAA", "GKP", 4.5, 3)},
		ClubCounts: map[string]int{"AAA": 1}})

	got := strings.ToLower(b.String())
	for _, banned := range []string{"nearest miss", "more than you have", "would improve"} {
		if strings.Contains(got, banned) {
			t.Errorf("the withdrawn upgrade search is back (%q):\n%s", banned, b.String())
		}
	}
}

func TestJoinAnd(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"A"}, "A"},
		{[]string{"A", "B"}, "A and B"},
		{[]string{"A", "B", "C"}, "A, B and C"},
	} {
		if got := joinAnd(c.in); got != c.want {
			t.Errorf("joinAnd(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// topPlans must not panic on a list shorter than the cap. The recommend path slices
// [1:] off its result, so an off-by-one here is a crash on a one-plan week.
func TestTopPlansIsSafeBelowTheCap(t *testing.T) {
	for n := 0; n < 5; n++ {
		got := topPlans(make([]analysis.Plan, n), 3)
		if want := min(n, 3); len(got) != want {
			t.Errorf("topPlans(%d plans, 3) returned %d, want %d", n, len(got), want)
		}
		if n >= 1 {
			_ = got[1:] // the exact slice transfers.go takes after best := plans[0]
		}
	}
}
