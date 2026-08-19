package main

import (
	"fmt"
	"testing"

	"armband/internal/analysis"
)

// What the opening squad's variety COSTS, measured rather than assumed.
//
// A varied squad is a real optimum of a question with two players removed from it, so it is
// worse than the unconstrained answer by construction. The question is by how much, and it
// is not a question anyone should answer by looking at one screenshot: the whole discipline
// of this project is that a number nobody measured is a number nobody should quote.
//
// The figure is per gameweek. Multiply by 38 for the season scale this record uses.

// TestTheVarietyCostIsKnown reports the gap across several seeds and fails only if it is
// large enough to be a bad recommendation rather than a different one.
//
// The ceiling is deliberately generous and deliberately present. Variety that costs a
// tenth of a point is not variety; variety that costs five is the tool recommending a squad
// it does not believe in. Somewhere between those is a judgement, and this pins where it
// was made so that changing varietyExclusions has to argue with a number.
func TestTheVarietyCostIsKnown(t *testing.T) {
	s := fixtureServer(t)
	e := s.engine

	req := analysis.OptimizeRequest{
		Budget:             1000,
		MinMinutes:         600,
		MinExpectedMinutes: 55,
	}
	best, err := e.Optimize(req)
	if err != nil {
		t.Fatal(err)
	}

	// Several seeds, because one is an anecdote. These are arbitrary and fixed, so the
	// measurement is reproducible.
	seeds := []int64{1, 20260819, 777, 424242, 99}
	var worst, total float64
	for _, seed := range seeds {
		varied, err := buildVariedSquad(e, req, seed)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		gap := best.ExpectedPoints - varied.ExpectedPoints
		total += gap
		if gap > worst {
			worst = gap
		}
		if gap < -0.0001 {
			t.Errorf("seed %d produced a squad projecting MORE than the unconstrained "+
				"optimum (%.3f against %.3f). A constrained answer cannot beat the "+
				"question it constrains — something is wrong with the objective.",
				seed, varied.ExpectedPoints, best.ExpectedPoints)
		}
	}
	mean := total / float64(len(seeds))
	t.Logf("variety costs %.2f pts/gw on average and %.2f at worst, over %d seeds "+
		"(%.0f and %.0f a season)", mean, worst, len(seeds), mean*38, worst*38)

	// The ceiling. Two points a gameweek is 76 a season, which is past "a different good
	// squad" and into "a squad the model does not recommend".
	const ceiling = 2.0
	if worst > ceiling {
		t.Errorf("the varied opening squad is %.2f pts/gw worse than the optimum at worst, "+
			"over the %.1f ceiling. That is no longer variety — lower varietyExclusions, "+
			"or draw the exclusions from more replaceable players.", worst, ceiling)
	}
}

// TestVarietyIsStableForASeed pins the property the whole design rests on: a reader who
// reloads has not asked for a different team.
func TestVarietyIsStableForASeed(t *testing.T) {
	s := fixtureServer(t)
	req := analysis.OptimizeRequest{Budget: 1000, MinMinutes: 600, MinExpectedMinutes: 55}

	first, err := buildVariedSquad(s.engine, req, 20260819)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildVariedSquad(s.engine, req, 20260819)
	if err != nil {
		t.Fatal(err)
	}
	if squadKey(first) != squadKey(second) {
		t.Errorf("the same seed produced two different squads:\n  %s\n  %s\n"+
			"A reload would reshuffle the reader's team, which is the staleness complaint "+
			"this feature exists to fix, inverted.", squadKey(first), squadKey(second))
	}

	other, err := buildVariedSquad(s.engine, req, 777)
	if err != nil {
		t.Fatal(err)
	}
	if squadKey(other) == squadKey(first) {
		t.Error("two different seeds produced the same squad, so the seed is not being " +
			"used and there is no variety at all")
	}
}

// TestTheOptimiserIsUntouchedByVariety pins that the randomness is entirely in the request.
//
// Determinism inside Optimize is load-bearing and pinned elsewhere — the replay is the
// instrument every scoring claim in this project rests on. This asserts the same objective,
// asked twice, still answers identically while variety is in use.
func TestTheOptimiserIsUntouchedByVariety(t *testing.T) {
	s := fixtureServer(t)
	req := analysis.OptimizeRequest{Budget: 1000, MinMinutes: 600, MinExpectedMinutes: 55}

	// A varied build runs in between, so any state it left behind would show here.
	a, err := s.engine.Optimize(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildVariedSquad(s.engine, req, 424242); err != nil {
		t.Fatal(err)
	}
	b, err := s.engine.Optimize(req)
	if err != nil {
		t.Fatal(err)
	}
	if squadKey(a) != squadKey(b) {
		t.Errorf("the optimiser answered differently either side of a varied build:\n  %s\n  %s\n"+
			"Variety must live entirely in the REQUEST — the replay's reproducibility "+
			"depends on Optimize being deterministic.", squadKey(a), squadKey(b))
	}
	if a.ExpectedPoints != b.ExpectedPoints {
		t.Errorf("the optimum projects %.6f then %.6f", a.ExpectedPoints, b.ExpectedPoints)
	}
}

func squadKey(sq *analysis.Squad) string {
	ids := make([]int, 0, len(sq.Players))
	for _, p := range sq.Players {
		ids = append(ids, p.ID)
	}
	// Sorted so a reordering is not read as a different squad.
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	return fmt.Sprint(ids)
}
