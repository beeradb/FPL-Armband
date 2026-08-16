package backtest

// Who does the pool floor actually remove, and would the optimiser have bought
// any of them?
//
//	DIAG=1 go test ./internal/backtest -run '^TestDiagFloorPopulation$' -v -timeout 60m
//
// # Why this exists: a perturbational null cannot say why it is null
//
// `TestDiagMetricReach` reports that removing `MinExpectedMinutes` entirely is
// byte-identical on every live column, and block J's twelve-cell table reports
// the same change as 112 `HOLD` points — which is **0.333 pts/gw, 12.7 a season**
// once converted, below this metric's median threshold of 33. Neither run can
// settle the disagreement on points, because neither is powered to. What can be
// settled exactly, and with no replay at all, is the **mediator**: does the
// opening fifteen change when the floor is lifted?
//
// That is the static half the reach run left owed (`stats/snapshots/
// 2026-08-13-reach/FINDINGS.md`, "What is owed" §1). A purely perturbational map
// reproduces, in its own zero cells, the failure it exists to fix: "did not move"
// and "was not read" are the same output. This re-derives the mechanism instead
// of asserting it, by counting the removed population directly and then asking
// whether any member of it survives into the floorless optimum.
//
// # The floor's rule is not the one the neighbouring probe uses
//
// `TestDiagTeamNewsReach` screens on `ExpectedMinutes`. The consumer does not.
// `squad.go:456-460` drops a player only if **`SettledMinutes` < floor AND
// `Price` > `fodderPrice`**, and `fodderPrice` is 4.5 whenever
// `BenchMinExpectedMinutes` is zero, which is every call the replay makes. So the
// removed set is *expensive players who rarely start*, not *players who rarely
// start* — a £4.0m reserve who never plays is deliberately kept, because
// excluding him would force real money onto the bench. Screening on the wrong
// field or forgetting the price exemption both overstate the population several
// times over, and this file is the place that mistake would be invisible.
//
// It also screens on **settled** rather than rested minutes: a fortnight of being
// eased back in must not make a regular un-pickable.
//
// # What each column means
//
//	removed     players failing the floor predicate over the WHOLE FIELD, on the
//	              exact rule above. This is the recorded 229.7-a-build quantity
//	pool        of those, the ones still in the pool when the floor ran — past
//	              the total-minutes floor and the availability screen, both of
//	              which Optimize applies FIRST. Strictly smaller, and it is what
//	              the floor actually DID rather than what its predicate says. The
//	              recorded 96-126 may be this column; reporting both is what can
//	              dissolve that disagreement instead of leaving it open
//	top removed the highest-scoring one, which is what a reader wants to see
//	              before believing the cut population is junk
//	differs     how many of the fifteen change when the floor is lifted to none.
//	              THIS IS THE MEDIATOR. Zero means the floor could not have paid,
//	              whatever any points column says
//	entered     removed players appearing in the floorless fifteen. It is the
//	              mechanism behind `differs`: the floor can only matter through a
//	              player it removed who would otherwise have been bought
//
// `entered` is bounded above by `differs` and is usually equal to it; they come
// apart when lifting the floor reshuffles the squad around a player it did not
// remove, which is a budget effect rather than a floor effect.
//
// `entered` is keyed on the WHOLE-FIELD set deliberately, not on `pool`. It
// feeds the `discriminating` guard below, and the smaller set would make
// `entered` smaller, hence more cells counted as able to convict the shipped
// search. The superset is the conservative choice for a guard.
//
// # Both optimisations are the replay's own
//
// The request mirrors `simulate.go:1040-1043` field for field — `Budget` from the
// sweep config, `MinMinutes: 600`, `BenchWeight` from `openingBenchWeight()` — and
// the engine comes from `EngineAt`, not `analysis.NewEngineFull`. A measurement
// was once withdrawn for the latter: it leaves `Recent` and `Priors` nil, so
// `ExpectedMinutes` silently falls back to the flat season-to-date mean and the
// number still looks like expected minutes.
//
// No metric is reported and there is no `p`. This is a count.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// floorLabel names a MinExpectedMinutes arm by the value that reproduces it.
//
// The no-floor arm must print as `-1` and never as a bare "no floor", because
// `MinExpectedMinutes = 0` is the SHIPPED floor — see resolvedMinExpectedMinutes.
// A reader who takes "no floor" from a results table and sets 0 to reproduce it gets
// the baseline, and the two arms then agree to the byte. That reads as a perfect
// reproduction and is the intervention silently not running: the byte-identical-null
// signature this package already has three other guards against.
//
// # It prints the RESOLVED value, and that is the fix rather than a detail
//
// `floorLabel(0)` must not print "floor=0", which a human reads as no floor and which
// is the shipped floor. Labelling by what the build actually applies makes 0 and the
// shipped rung the same label, which is what they are, and it keeps the round trip
// honest for every input rather than only the ones anybody thought to list.
//
// `%g` rather than `%.0f`: a fractional rung is legal and `%.0f` would round 62.5 to
// "floor=62", a label that reproduces a DIFFERENT arm. The guard cannot catch that on
// a value nobody added to its table, so the format has to be lossless instead.
//
// # It asks resolvedMinExpectedMinutes for the shipped value and never spells it
//
// That function's own comment says a second copy of this switch is the hazard — "if
// the default moved to 60, a probe carrying its own switch would keep building the 55
// arm, keep printing a clean result, and read as a perfect reproduction". A literal 55
// here would be exactly that copy, three lines under the warning.
//
// One function rather than one per sweep block: it was written twice and the two
// copies had already drifted in both the word and the format — "floor=55 (ships)" in
// one block and "55 (shipped)" in the other, for the same arm.
func floorLabel(f float64) string {
	resolved := SimConfig{MinExpectedMinutes: f}.resolvedMinExpectedMinutes()
	shipped := SimConfig{}.resolvedMinExpectedMinutes()
	switch {
	case f < 0:
		return fmt.Sprintf("floor=%g (no floor; 0 would mean the shipped %g)", f, shipped)
	case resolved == shipped:
		return fmt.Sprintf("floor=%g (ships)", resolved)
	}
	return fmt.Sprintf("floor=%g", resolved)
}

// TestTheFloorLabelNamesAValueThatReproducesTheArm closes the gap the label itself
// is the whole of: a results table is the only thing a later reader has, so a rung
// labelled by a value that does not rebuild it is a reproduction instruction that
// silently builds a different arm.
//
// The specific trap is `MinExpectedMinutes = 0`, which means the SHIPPED 55 and not
// "no floor" — so "no floor" as a bare label sends a reader to 0, and the arm they
// get agrees with the baseline to the byte. This asserts the round trip rather than
// the spelling: parse the value back out of the label, resolve it the way the build
// resolves it, and require the floor the rung is supposed to be measuring.
func TestTheFloorLabelNamesAValueThatReproducesTheArm(t *testing.T) {
	// The two sweeps' own rungs, plus the three classes they happen not to contain.
	// A table that lists only the rungs in use can go stale on nothing and is blind
	// to everything else — 0 is the trap this whole item is about, and a fractional
	// rung is the one a lossy format would silently round into a different arm.
	shipped := SimConfig{}.resolvedMinExpectedMinutes()
	for _, tc := range []struct {
		set  float64 // what the sweep assigns to MinExpectedMinutes
		want float64 // the floor the opening build must then apply
	}{
		{-1, 0}, {30, 30}, {45, 45}, {55, 55}, {65, 65}, {75, 75},
		{0, shipped}, // the trap: 0 is the shipped floor, NOT no floor
		{62.5, 62.5}, // a fractional rung must not be rounded into another arm
		{-40, 0},     // any negative means no floor, not just -1
	} {
		label := floorLabel(tc.set)
		// The value named in the LABEL, not the value the sweep happened to use.
		// These are the same number only if the label is honest, which is the point.
		var named float64
		if _, err := fmt.Sscanf(label, "floor=%g", &named); err != nil {
			t.Errorf("floorLabel(%v) = %q, which does not open with a parseable "+
				"floor=<value>. The label is a reproduction instruction, and a "+
				"reader cannot follow one they cannot read a number out of.",
				tc.set, label)
			continue
		}
		got := SimConfig{MinExpectedMinutes: named}.resolvedMinExpectedMinutes()
		if got != tc.want {
			t.Errorf("floorLabel(%v) = %q, but setting MinExpectedMinutes=%v "+
				"resolves to a floor of %v, not %v.\n\n"+
				"A reader reproducing this arm from its label builds a different "+
				"one, and where %v is the shipped floor the two then agree to the "+
				"byte — the byte-identical null with the intervention silently "+
				"not run.", tc.set, label, named, got, tc.want, got)
		}
	}

	// Pinned separately because it is the instance this was filed against, and
	// because it fails through a different door: an edit that drops the number
	// back out of the label is caught above only by Sscanf refusing it.
	if lbl := floorLabel(-1); !strings.Contains(lbl, "-1") {
		t.Errorf("floorLabel(-1) = %q and does not name -1. Only a NEGATIVE value "+
			"means no floor; 0 is the shipped 55.", lbl)
	}
}

func TestDiagFloorPopulation(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	starts := sweepStarts()

	fmt.Printf("\n=== the pool floor's population, and whether it reaches the fifteen\n\n")
	fmt.Printf("Removed = analysis.CutByExpectedMinutesFloor, the pool filter's own\n")
	fmt.Printf("predicate: SettledMinutes < 55 AND Price > BenchFodderPrice (4.5).\n")
	fmt.Printf("differs = squad slots changing when the floor is lifted to none.\n")
	fmt.Printf("This is the mediator: differs = 0 means the floor cannot have paid.\n\n")

	fmt.Printf("obj floored / obj none is the objective Optimize maximises, on both\n")
	fmt.Printf("squads. The floorless pool is a superset, so obj none > obj floored\n")
	fmt.Printf("means the floored run did not return its own argmax.\n\n")

	fmt.Printf("removed = whole-field, over AllMetrics(). pool = what the floor\n")
	fmt.Printf("actually took: also past the total-minutes floor and the\n")
	fmt.Printf("availability screen, which Optimize applies BEFORE it.\n\n")

	fmt.Printf("%-9s %4s %8s %7s %7s %8s  %10s %10s %8s  %s\n",
		"season", "gw", "removed", "pool", "differs", "entered",
		"obj floor", "obj none", "delta", "top removed")

	var cells, moved, totalRemoved, totalPool, missed, canTest int
	bySeason := map[string]int{}

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])

		for _, gw := range starts {
			sc := sweepConfig(cfg, gw, false)
			// through = gw-1: this deadline's gameweek is not yet known, which is
			// the convention Simulate passes and EngineAt takes.
			e, _ := EngineAt(cur, prior, gw-1, sc)

			// The floor through the opening build's own resolver, not a copy of
			// its switch. This was a third copy until review found it, and it is
			// the copy that decides which arm counts as shipped.
			floor := sc.resolvedMinExpectedMinutes()

			req := func(minExp float64) analysis.OptimizeRequest {
				return analysis.OptimizeRequest{
					Budget: sc.Budget, MinMinutes: 600,
					MinExpectedMinutes: minExp,
					BenchWeight:        sc.openingBenchWeight(),
				}
			}
			withFloor, err := e.Optimize(req(floor))
			if err != nil {
				// Infeasible is a result about the arm, not a broken harness —
				// a high enough floor leaves too few players for a legal fifteen.
				fmt.Printf("%-9s %4d  infeasible with floor: %v\n", cur.Name, gw, err)
				continue
			}
			noFloor, err := e.Optimize(req(0))
			if err != nil {
				t.Fatalf("%s@%d: the floorless pool is a superset and cannot be "+
					"infeasible where the floored one was not: %v", cur.Name, gw, err)
			}

			// The removed set, through the consumer's own predicate rather than a
			// restatement of it. See analysis.CutByExpectedMinutesFloor for why
			// this is not four lines inline: a diagnostic carrying its own copy
			// of the thing it is checking is what this record calls its worst
			// place for that failure, and the first version of this file did it.
			// Two populations, not one, and the difference is the point.
			//
			// `removed` is whole-field: everyone who fails the floor predicate
			// over AllMetrics(). `pool` is what the floor actually took, which
			// needs the two screens Optimize runs FIRST — the total-minutes
			// floor and the availability status cut. A permanently injured
			// reserve with 200 minutes fails the floor predicate and was
			// already gone before the floor ran, so counting him describes the
			// predicate rather than the floor.
			//
			// The recorded 229.7 a build is the whole-field figure and the
			// recorded 96-126 may be this one; reporting both is what can
			// dissolve that disagreement instead of leaving it open.
			removed := map[int]analysis.PlayerMetrics{}
			poolRemoved := map[int]analysis.PlayerMetrics{}
			for _, m := range e.AllMetrics() {
				if !analysis.CutByExpectedMinutesFloor(m, req(floor)) {
					continue
				}
				removed[m.ID] = m
				if e.ReachesExpectedMinutesCut(m, req(floor)) {
					poolRemoved[m.ID] = m
				}
			}

			held := map[int]bool{}
			for _, p := range withFloor.Players {
				held[p.ID] = true
			}
			differs, entered := 0, 0
			for _, p := range noFloor.Players {
				if !held[p.ID] {
					differs++
					if _, ok := removed[p.ID]; ok {
						entered++
					}
				}
			}

			top := "-"
			if len(removed) > 0 {
				best := make([]analysis.PlayerMetrics, 0, len(removed))
				for _, m := range removed {
					best = append(best, m)
				}
				// Ties broken by ID so the table is reproducible: ranging a map
				// is the non-determinism this package already pinned once.
				sort.Slice(best, func(i, j int) bool {
					if best[i].Score != best[j].Score {
						return best[i].Score > best[j].Score
					}
					return best[i].ID < best[j].ID
				})
				top = fmt.Sprintf("%s (%.2f, £%.1fm, %.0f min)",
					best[0].Name, best[0].Score, best[0].Price, best[0].SettledMinutes)
			}

			// The objective the search was actually climbing, on both squads.
			//
			// This is the test of whether the SHIPPED run returned its own
			// argmax — but it needs `entered` to be sound, and that guard is the
			// whole finding rather than a detail.
			//
			// A higher floorless objective only convicts the floored run if the
			// floorless squad was REACHABLE under the floor, which means it
			// contains no player the floor removed (entered == 0). Then the
			// squad was legal, affordable and in-pool for the floored run, which
			// returned something worse on its own objective — a plain search
			// defect, with no appeal to a mis-specified objective available,
			// because it is the objective's own number saying so.
			//
			// With entered > 0 the floorless squad uses a player the floor
			// excluded, so it was never available and a higher score is the
			// floor's genuine cost rather than a search failure. Scoring those
			// together would report the floor working as designed as a bug.
			objFloor := e.ObjectiveFor(withFloor.Players, req(floor))
			objNone := e.ObjectiveFor(noFloor.Players, req(0))

			// A cell can only test the shipped run if the two squads differ AND
			// the floorless one is reachable under the floor. With differs == 0
			// the squads are the same set — Optimize keys `selected` by id, so
			// fifteen distinct ids matching means set equality — and the
			// objective is equal BY CONSTRUCTION, not by measurement.
			//
			// Counting those as passes is this record's signature failure: a
			// cell where the intervention could not run is not a tie. Review
			// caught it here after the summary two paragraphs down had already
			// applied the rule correctly to `moved`.
			discriminating := differs > 0 && entered == 0
			missedArgmax := discriminating && objNone > objFloor+1e-9

			verdict := ""
			switch {
			case missedArgmax:
				verdict = "  <-- SHIPPED RUN MISSED ITS ARGMAX"
			case objNone > objFloor+1e-9:
				verdict = "  (floor's real cost: uses a removed player)"
			case objFloor > objNone+1e-9:
				verdict = "  (floorless wandered)"
			}

			fmt.Printf("%-9s %4d %8d %7d %7d %8d  %10.4f %10.4f %+8.4f%s  %s\n",
				cur.Name, gw, len(removed), len(poolRemoved), differs, entered,
				objFloor, objNone, objNone-objFloor, verdict, top)
			if discriminating {
				canTest++
			}
			if missedArgmax {
				missed++
			}

			cells++
			totalRemoved += len(removed)
			totalPool += len(poolRemoved)
			if differs > 0 {
				moved++
				bySeason[cur.Name]++
			}
		}
	}

	fmt.Printf("\n%d of %d cells have an opening fifteen that changes when the floor\n",
		moved, cells)
	fmt.Printf("is lifted; mean removed population %.1f players a build whole-field,\n",
		float64(totalRemoved)/float64(max(cells, 1)))
	fmt.Printf("%.1f of them still in the pool when the floor ran. Quote the second\n",
		float64(totalPool)/float64(max(cells, 1)))
	fmt.Printf("for what the floor DID and the first for what its predicate says;\n")
	fmt.Printf("the recorded 229.7 is whole-field and the recorded 96-126 may be\n")
	fmt.Printf("the pool figure, which is what these two columns can now settle.\n")
	if moved == 0 {
		fmt.Printf("\nZero is a STATIC null, not a perturbational one: the population is\n")
		fmt.Printf("counted and non-empty, and the optimiser declines all of it. The\n")
		fmt.Printf("floor is inert downward at shipped config, and block J's 12.7 a\n")
		fmt.Printf("season cannot have come from the opening build on this data state.\n")
	} else {
		fmt.Printf("\nCells that move, by season: %v\n", bySeason)
		fmt.Printf("A points figure is quotable only over these; the rest are\n")
		fmt.Printf("structural zeros and pooling them dilutes toward zero.\n")
	}
	fmt.Printf("\n**%d of %d DISCRIMINATING builds** have the shipped run missing its\n", missed, canTest)
	fmt.Printf("argmax. Quote that denominator, never %d.\n\n", cells)
	fmt.Printf("A build can test the shipped run only if the two squads differ and\n")
	fmt.Printf("the floorless one was reachable under the floor (differs > 0 and\n")
	fmt.Printf("entered == 0). Where differs == 0 the squads are the same set and\n")
	fmt.Printf("the objectives are equal BY CONSTRUCTION; where entered > 0 the\n")
	fmt.Printf("floorless squad uses a removed player and was never available, so\n")
	fmt.Printf("a higher score there is the floor working, not a defect. Neither\n")
	fmt.Printf("can ever convict, so pooling them manufactures a denominator.\n")
	if canTest == 0 {
		fmt.Printf("\nZERO discriminating builds: this run says NOTHING about whether\n")
		fmt.Printf("the shipped search returns its argmax. Do not read it as a pass.\n")
	}
	fmt.Printf("\nThe other direction needs no denominator and is the durable result:\n")
	fmt.Printf("a strictly larger pool returning a WORSE optimum convicts the search\n")
	fmt.Printf("outright, on any cell where it happens, because an exact optimiser\n")
	fmt.Printf("cannot do it.\n")

	fmt.Printf("\nSimple-effect null: shipped bench shape, no chips, bespoke search.\n")
}
