package main

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"armband/internal/analysis"
	"armband/internal/stats"
)

// What the opening squad's variety costs.
//
// # Why this is a census and not a sample
//
// buildVariedSquad filters the optimum's fifteen to its non-goalkeeper, non-captain members
// — twelve players on the committed capture — and excludes two. The seed therefore draws
// uniformly from C(12,2) = 66 exclusion pairs, and the whole population can be enumerated
// in about a minute and a half.
//
// The first version of this sampled five seeds and quoted the mean. Review enumerated the
// population and refuted it: the true mean is 0.53 and the five seeds gave 0.88, which sits
// above the p95 of the five-seed sampling distribution. Worse, those five draws produced
// only TWO distinct gap values, because the constrained squad snaps to a small set of
// alternatives — so the effective sample was about two, not five. "Several seeds, because
// one is an anecdote" is answered by enumeration, not by four more anecdotes.
//
// The lesson generalises past this test: when a mechanism's sample space is small enough to
// count, no figure about it should ever carry a sampling error.
//
// # What the census does NOT establish
//
// The gap is the model's own projected difference at one instant, on one capture, at one
// gameweek, under one horizon. It is not a measured points cost, and it must not be
// multiplied by 38 — that convention belongs to a paired realised-points difference per
// gameweek played in a replay cell, and none of the three things it assumes hold here.
//
// The sign of the REALISED cost is not established either. The unconstrained optimum is an
// argmax over players, so by this record's own winner's-curse reasoning it reaches for
// whichever player the model most over-rates; excluding two of its members and
// re-optimising is a mild de-biasing move. That is a hypothesis, not a finding — but it
// means the projected gap is more likely an over-statement of the realised cost than an
// under-statement.

// varietyCensus is a full enumeration of the exclusion pairs, computed once per package run.
//
// Two tests read it and it costs 66 optimisations — about 75 seconds — so enumerating per
// test doubled the package's wall clock to obtain a second copy of the same answer. Sharing
// it is safe because the census is deterministic by construction: `Optimize` is pinned
// deterministic by three tests, and the pairs are enumerated in sorted order.
//
// The error is carried rather than fataled inside the once, because whichever test happens
// to run first owns the *testing.T in there — and a Fatal on another test's T reports the
// failure against the wrong name.
var (
	censusOnce sync.Once
	censusGaps []float64
	censusBest *analysis.Squad
	censusErr  error
)

func varietyCensus(t *testing.T) (gaps []float64, best *analysis.Squad) {
	t.Helper()
	censusOnce.Do(func() { censusGaps, censusBest, censusErr = runVarietyCensus(t) })
	if censusErr != nil {
		t.Fatal(censusErr)
	}
	return censusGaps, censusBest
}

func runVarietyCensus(t *testing.T) (gaps []float64, best *analysis.Squad, err error) {
	t.Helper()
	s := fixtureServer(t)
	e := s.engine

	req := analysis.OptimizeRequest{
		Budget:             1000,
		MinMinutes:         600,
		MinExpectedMinutes: 55,
	}
	best, err = e.Optimize(req)
	if err != nil {
		return nil, nil, err
	}

	var candidates []int
	for _, m := range best.Players {
		if m.ID == best.Captain.ID || m.Position == "GKP" {
			continue
		}
		candidates = append(candidates, m.ID)
	}
	sort.Ints(candidates)

	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			varied := req
			varied.ExcludeIDs = []int{candidates[i], candidates[j]}
			got, oerr := e.Optimize(varied)
			if oerr != nil {
				return nil, nil, fmt.Errorf("excluding %d and %d: %w",
					candidates[i], candidates[j], oerr)
			}
			gaps = append(gaps, best.ExpectedPoints-got.ExpectedPoints)
		}
	}
	return gaps, best, nil
}

// TestTheVarietyCostIsBounded enumerates every squad the mechanism can serve and bounds the
// worst one.
//
// The bound is the point. A mean says nothing a reader needs: nobody is served the mean.
// What matters is the worst fifteen anyone can be given, and after a census that is a fact
// rather than an order statistic.
func TestTheVarietyCostIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("the census runs 66 optimisations; skipped under -short")
	}
	gaps, best := varietyCensus(t)
	if len(gaps) == 0 {
		t.Fatal("the census enumerated nothing")
	}
	sorted := append([]float64(nil), gaps...)
	sort.Float64s(sorted)
	worst := sorted[len(sorted)-1]
	var total float64
	for _, g := range gaps {
		total += g
	}
	mean := total / float64(len(gaps))
	median := stats.Median(gaps)

	t.Logf("census of %d exclusion pairs: min %.3f, mean %.3f, median %.3f, max %.3f "+
		"pts/gw against an optimum projecting %.2f (worst is %.1f%% of it)",
		len(gaps), sorted[0], mean, median, worst, best.ExpectedPoints,
		100*worst/best.ExpectedPoints)

	// The ceiling, stated as a share of the projection rather than as points.
	//
	// An absolute figure would be a number about this capture at this gameweek; a share is
	// the thing that stays meaningful when the projection moves. Five per cent is the
	// judgement: past that, the default squad is not a different good answer, it is one the
	// model does not recommend, and a reader who never presses Optimize deserves better.
	const ceiling = 0.05
	if share := worst / best.ExpectedPoints; share > ceiling {
		t.Errorf("the worst squad the variety mechanism can serve projects %.3f below the "+
			"optimum, %.1f%% of %.2f — over the %.0f%% ceiling. Lower varietyExclusions, or "+
			"draw the exclusions from more replaceable players.",
			worst, 100*share, best.ExpectedPoints, 100*ceiling)
	}
}

// TestAConstrainedSquadDoesNotBeatTheOptimumByMoreThanSearchSlack.
//
// A constrained answer beating the unconstrained one looks like a broken objective, and the
// first version of this test said so at a threshold of 1e-4. It is not: `Optimize` is a
// heuristic — a greedy seed, an exact per-formation DP, then a local search — so it can miss
// the true optimum by a little, and the constrained run can land better than the
// unconstrained one by exactly that slack.
//
// Four of the 66 pairs do, by 0.097. At 1e-4 this test would have failed for that reason
// roughly a quarter of the time on any change to the seeds, the capture or the pool, with a
// message blaming the objective. The threshold now sits above the search's own slack, so
// what it detects is an inversion large enough to be real.
func TestAConstrainedSquadDoesNotBeatTheOptimumByMoreThanSearchSlack(t *testing.T) {
	if testing.Short() {
		t.Skip("the census runs 66 optimisations; skipped under -short")
	}
	gaps, best := varietyCensus(t)

	// Generous against the heuristic, tight against a real inversion: the measured slack is
	// about 0.1, and a genuine objective fault would be worth points rather than hundredths.
	const slack = 0.5
	for _, g := range gaps {
		if g < -slack {
			t.Errorf("a constrained squad projects %.3f MORE than the unconstrained optimum "+
				"(%.2f), past the %.1f the local search can plausibly leave on the table. "+
				"That is an objective fault rather than search slack.",
				-g, best.ExpectedPoints, slack)
		}
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

// TestVarietyActuallyVaries checks the mechanism delivers what it costs points for.
//
// A cost with no benefit is the worst of both, and "the squad is different" is the one
// claim here that can be checked exactly rather than projected.
func TestVarietyActuallyVaries(t *testing.T) {
	s := fixtureServer(t)
	req := analysis.OptimizeRequest{Budget: 1000, MinMinutes: 600, MinExpectedMinutes: 55}
	best, err := s.engine.Optimize(req)
	if err != nil {
		t.Fatal(err)
	}
	varied, err := buildVariedSquad(s.engine, req, 20260819)
	if err != nil {
		t.Fatal(err)
	}
	in := map[int]bool{}
	for _, p := range best.Players {
		in[p.ID] = true
	}
	changed := 0
	for _, p := range varied.Players {
		if !in[p.ID] {
			changed++
		}
	}
	if changed < 2 {
		t.Errorf("the varied squad differs from the optimum by %d players; two are excluded, "+
			"so fewer than two changes means the exclusions are being replaced by themselves",
			changed)
	}
	t.Logf("the varied fifteen differs from the optimum by %d players", changed)
}

// TestTheOptimiserIsUntouchedByVariety pins that the randomness is entirely in the request.
//
// Determinism inside Optimize is what the replay's reproducibility rests on.
func TestTheOptimiserIsUntouchedByVariety(t *testing.T) {
	s := fixtureServer(t)
	req := analysis.OptimizeRequest{Budget: 1000, MinMinutes: 600, MinExpectedMinutes: 55}

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
			"Variety must live entirely in the REQUEST.", squadKey(a), squadKey(b))
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
	sort.Ints(ids)
	return fmt.Sprint(ids)
}
