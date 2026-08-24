package main

import (
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

func gains(gs ...float64) []analysis.Plan {
	out := make([]analysis.Plan, len(gs))
	for i, g := range gs {
		out[i] = analysis.Plan{GainPerGW: g, Transfers: 1}
	}
	return out
}

// The band is measured from the BEST plan, not between neighbours.
//
// This is the case that makes chaining wrong: 0.90, 0.60, 0.30 has every neighbour
// within 0.41 of the next, but the third is 0.60 behind the first and is not the same
// answer as it. A pairwise walk would swallow the whole list and tell a reader three
// moves are equivalent when the last is plainly worse.
func TestTheBandIsMeasuredFromTheBestPlanNotBetweenNeighbours(t *testing.T) {
	got := equivalentTo(gains(0.90, 0.60, 0.30), 0.41)
	if len(got) != 2 {
		t.Fatalf("want 2 plans within 0.41 of 0.90, got %d: %v", len(got), got)
	}
	if got[1].GainPerGW != 0.60 {
		t.Errorf("second plan = %.2f, want 0.60", got[1].GainPerGW)
	}
}

func TestTheBandStopsAtTheFirstPlanOutsideIt(t *testing.T) {
	for _, c := range []struct {
		name string
		in   []analysis.Plan
		want int
	}{
		{"all inside", gains(1.00, 0.80, 0.65), 3},
		{"one inside", gains(1.00, 0.50), 1},
		{"exactly on the edge is inside", gains(1.00, 0.59), 2},
		{"just outside", gains(1.00, 0.58), 1},
		{"single plan", gains(0.42), 1},
		{"no plans", nil, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := equivalentTo(c.in, 0.41); len(got) != c.want {
				t.Errorf("got %d plans, want %d", len(got), c.want)
			}
		})
	}
}

// ⚠️ A zero band must not collapse the answer to "nothing is equivalent to anything".
//
// MinSeparableGain arrived after every deployed config.json was written, so an un-backfilled
// config reads 0. config.Load backfills it, and this is the second half of that guard: if the
// backfill is ever dropped, the band degrades to showing everything rather than silently
// telling a reader the top pick stands alone when the model has no such opinion.
func TestAZeroBandDoesNotSilentlyClaimTheTopPickStandsAlone(t *testing.T) {
	in := gains(1.00, 0.99, 0.98)
	if got := equivalentTo(in, 0); len(got) != len(in) {
		t.Errorf("a zero band returned %d of %d plans; it must not narrow the answer",
			len(got), len(in))
	}
}

// The backfilled default must actually be the measured figure. docs/accuracy.md puts the
// model's own top-20 over-rating at 0.41 pts/gw, and this field exists to be that number.
// A drift here is silent: the band just gets wider or narrower.
func TestTheShippedBandMatchesTheMeasuredOverRating(t *testing.T) {
	if got := defaultSeparableGain(); got != 0.41 {
		t.Errorf("shipped MinSeparableGain = %v, want 0.41 — the top-20 over-rating in "+
			"docs/accuracy.md. If the model changed, re-derive it there FIRST and update "+
			"both together.", got)
	}
}

// A one-member band prints NOTHING. There is no tie, and a heading over a single row
// would imply the others were beaten rather than absent.
func TestASingleMemberBandPrintsNothing(t *testing.T) {
	var b strings.Builder
	printSeparableBand(&b, gains(1.00, 0.20, 0.10), 0.41)
	if b.String() != "" {
		t.Errorf("nothing is tied with the leader, so nothing is owed:\n%s", b.String())
	}
}

// The band must not be rendered as a ranking. "Also considered" and "next best" both say
// the model put these below the recommendation; within the band it has no such opinion,
// and saying so is the whole point of the feature.
func TestTheBandIsNotPresentedAsARanking(t *testing.T) {
	var b strings.Builder
	printSeparableBand(&b, gains(0.72, 0.55, 0.48), 0.41)
	got := b.String()

	for _, banned := range []string{"next best", "also considered", "runner"} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Errorf("the band reads as a ranking (%q):\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "same answer") {
		t.Errorf("the band should say the moves are equivalent:\n%s", got)
	}
	if !strings.Contains(got, "cannot see") {
		t.Errorf("the band should tell the reader what to break the tie on:\n%s", got)
	}
}

// The API must tell the client WHICH plans are equivalent, so the page never re-derives
// the threshold.
//
// The band is config.Review.MinSeparableGain and the grouping is equivalentTo(). A client
// that recomputed either would be a second implementation of a decision this repository
// has already paid for duplicating elsewhere — and it would drift the first time the
// top-20 bias is re-measured and the config default moves.
func TestTheTransferDocumentCarriesTheEquivalentSetAndItsBand(t *testing.T) {
	cfg := config.Default()
	band := cfg.Review.MinSeparableGain

	board := transferBoard{Free: 1, Plans: gains(1.00, 0.80, 0.20)}
	doc := transferAnswer(&board, cfg, nil)

	if doc.Equivalent != 2 {
		t.Errorf("equivalent = %d, want 2 (1.00 and 0.80 are within %.2f; 0.20 is not)",
			doc.Equivalent, band)
	}
	if doc.Band != band {
		t.Errorf("separable_band = %v, want the configured %v", doc.Band, band)
	}
}

// A one-plan week must report exactly one equivalent, never zero. The page renders the
// first `equivalent` plans, so a zero here would show the reader nothing at all while the
// document plainly carries a recommendation.
func TestASinglePlanReportsOneEquivalentNotZero(t *testing.T) {
	doc := transferAnswer(&transferBoard{Free: 1, Plans: gains(0.9)}, config.Default(), nil)
	if doc.Equivalent != 1 {
		t.Errorf("equivalent = %d, want 1", doc.Equivalent)
	}
}
