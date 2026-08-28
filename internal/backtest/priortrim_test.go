package backtest

import "testing"

// The trim drops last season's dead rubber from the prior. These pin the three
// properties that decide whether an arm varying it measures the trim or
// something else.

// trimSeason is one player who played every gameweek, with minutes that make the
// trimmed week identifiable: 90 all season except a token 5 in GW38, the shape a
// rotated dead rubber actually takes.
func trimSeason(finalMinutes int) *Season {
	p := &Player{ID: 1, Code: 4242, GWs: map[int]GW{}}
	total, starts := 0, 0
	for gw := 1; gw <= 38; gw++ {
		m := 90
		if gw == 38 {
			m = finalMinutes
		}
		p.GWs[gw] = GW{Minutes: m, Fixtures: 1}
		total += m
		if m >= 60 {
			starts++
		}
	}
	p.Minutes, p.Starts = total, starts
	return &Season{Name: "2025-26", Players: map[int]*Player{1: p}}
}

// ⚠️ **A trim of zero must be byte-identical to no trim at all.** The arm has to
// ship inert: if the default changed anything, every existing figure measured on
// this prior would silently move and be attributed to whatever else was in the
// same run.
func TestPriorTrimOfZeroChangesNothing(t *testing.T) {
	// ⚠️ Compared against a trim at 38, NOT against another trim of 0. The
	// fixture's last gameweek is 38, so trimming there keeps every week and is
	// the shipped behaviour expressed a second way — which is what makes this an
	// assertion rather than a restatement. An earlier version of this test
	// compared two identical calls and therefore tested determinism while
	// claiming to test inertness.
	s := trimSeason(5)
	for _, hl := range []float64{2, 4, 8, 12} {
		want, wok := newPriorIndexRecent(s, hl, 0, 38).Get(4242)
		got, gok := newPriorIndexRecent(s, hl, 0, 0).Get(4242)
		if !wok || !gok {
			t.Fatalf("half-life %v: the fixture player should be present", hl)
		}
		if got.Minutes != want.Minutes || got.Starts != want.Starts {
			t.Errorf("half-life %v: a trim of 0 must keep every gameweek, so it has to "+
				"match a trim at the season's last one. Got minutes %d/starts %d against "+
				"%d/%d", hl, got.Minutes, got.Starts, want.Minutes, want.Starts)
		}
	}
}

// The point of the trim: GW38's minutes must stop reaching the prior.
//
// The fixture plays 90 all season and 5 in GW38, so an untrimmed recency prior
// is dragged toward 5 — hardest at the sharpest half-life, since GW38 carries
// the most weight. Trimming at 37 must remove that entirely.
func TestPriorTrimRemovesTheDeadRubber(t *testing.T) {
	const sharp = 2.0

	rotated, ok := newPriorIndexRecent(trimSeason(5), sharp, 0, 0).Get(4242)
	if !ok {
		t.Fatal("fixture player missing")
	}
	trimmed, ok := newPriorIndexRecent(trimSeason(5), sharp, 0, 37).Get(4242)
	if !ok {
		t.Fatal("fixture player missing")
	}
	if trimmed.Minutes <= rotated.Minutes {
		t.Errorf("trimming a 5-minute GW38 off a 90-minute season should RAISE the "+
			"prior's minutes; got %d trimmed against %d untrimmed",
			trimmed.Minutes, rotated.Minutes)
	}

	// ⚠️ And it must be the WEEK being dropped, not the low number. A player who
	// played 90 in GW38 too should read almost the same trimmed or not — if he
	// does not, the trim is changing more than the dead rubber.
	full := trimSeason(90)
	a, _ := newPriorIndexRecent(full, sharp, 0, 0).Get(4242)
	b, _ := newPriorIndexRecent(full, sharp, 0, 37).Get(4242)
	if diff := a.Minutes - b.Minutes; diff > 2 || diff < -2 {
		t.Errorf("for a player who played 90 in every week including GW38, trimming "+
			"GW38 should barely move the prior; got %d against %d", b.Minutes, a.Minutes)
	}
}

// ⚠️ **THE TRIM MUST NOT FLATTEN THE GRADIENT.** Removing the ordering would be
// testing the flat prior again, which is already measured and already wins — so
// an arm that did that would answer a question the record has closed while
// appearing to answer a new one.
//
// GW37 must still outweigh GW1 under a trim at 37. This is asserted through the
// prior the code actually produces rather than by re-deriving the weight
// formula, so it fails if the anchor moves to a fixed gameweek instead of the
// player's own last appearance inside the window.
func TestTheTrimStillWeightsGW37AboveTheOpeningWeek(t *testing.T) {
	const hl = 4.0

	// Two players, identical except for WHEN their good football happened: one
	// played 90s late and 10s early, the other the reverse. Both are trimmed at
	// 37, so GW38 is out of the picture for both.
	late := &Player{ID: 1, Code: 1, GWs: map[int]GW{}}
	early := &Player{ID: 2, Code: 2, GWs: map[int]GW{}}
	for gw := 1; gw <= 38; gw++ {
		lm, em := 10, 90
		if gw >= 20 {
			lm, em = 90, 10
		}
		late.GWs[gw] = GW{Minutes: lm, Fixtures: 1}
		early.GWs[gw] = GW{Minutes: em, Fixtures: 1}
		late.Minutes += lm
		early.Minutes += em
	}
	late.Starts, early.Starts = 19, 19
	s := &Season{Name: "2025-26", Players: map[int]*Player{1: late, 2: early}}

	idx := newPriorIndexRecent(s, hl, 0, 37)
	lp, ok1 := idx.Get(1)
	ep, ok2 := idx.Get(2)
	if !ok1 || !ok2 {
		t.Fatal("both fixture players should be present")
	}
	if lp.Minutes <= ep.Minutes {
		t.Errorf("under a trim at GW37 the recent weeks must still outweigh the opening "+
			"ones: the player who played 90s from GW20 reads %d, the one who played "+
			"90s until GW19 reads %d. Equal or reversed means the trim flattened the "+
			"gradient and the arm is re-testing the flat prior", lp.Minutes, ep.Minutes)
	}
}
