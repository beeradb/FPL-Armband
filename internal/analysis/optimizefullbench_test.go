package analysis

import (
	"fmt"
	"math/rand"
	"testing"

	"armband/internal/fpl"
)

// This file benchmarks the phases of Engine.Optimize that
// optimizerbench_test.go does not reach: the full end-to-end call, the
// exact-DP seeding stage, and the seed-fill phase's single hottest
// allocation site. It shares benchPool/benchSquad from that file rather than
// building a second fixture mechanism.
//
// # BenchmarkPolish's "cannot be benchmarked without the live API" claim
//
// optimizerbench_test.go's doc comment on BenchmarkPolish says Optimize
// "cannot be benchmarked without the live API, since AllMetrics runs the
// whole scoring pipeline over a bootstrap." That is only half right:
// AllMetrics does run the whole scoring pipeline, but the pipeline runs over
// whatever *fpl.Bootstrap it is handed — live or synthetic. NewEngineFull
// takes any bootstrap, and enginescale_test.go's scaleEngine and
// minutesfloorwindow_test.go's partialGameweekField already build one by
// hand and call Metrics/Optimize on it with no network call anywhere. This
// file does the same at the pool sizes the rest of the package benchmarks
// at. That earlier comment is not touched here — it lives in a file another
// agent is concurrently editing.

// benchOptimizeEngine builds an *Engine over a synthetic, deterministic
// bootstrap with no network call: 20 clubs and n players cycling the same
// 1-GKP/5-DEF/5-MID/2-FWD-per-13 pattern benchPool uses, each assigned a
// club by a seeded RNG (not by map iteration, so the pool order and every
// downstream sort are reproducible run to run).
//
// Every player's score comes from a PriorPlayer, exactly as
// minutesfloorwindow_test.go's partialGameweekField does for the same
// reason: pre-season, FPL's own aggregates are zero, and blendFor reads a
// pre-season player entirely from the prior (TestBlendIsANoOpPreSeason:
// PriorWeight is 1 with no in-season minutes). No Event or Fixture is
// marked started or finished, so the engine sits in the same pure pre-season
// state scaleEngine uses — DataWindow is a flat 38 for every club, and
// nothing here depends on the live-gameweek-gap machinery.
//
// Prices sit on the real £0.1m grid (39 to 150) and the underlying rates
// vary on a coarse band, deliberately, for the same reason benchPool's own
// comment gives: a pool of distinct floats hides the tie-order trap this
// package has already been bitten by (TestStableTieOrderFollowsInputOrder).
func benchOptimizeEngine(n int) *Engine {
	const teams = 20
	rng := rand.New(rand.NewSource(20260825))

	b := &fpl.Bootstrap{
		Season: "2026-27",
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	for i := 1; i <= teams; i++ {
		b.Teams = append(b.Teams, fpl.Team{
			ID: i, Name: fmt.Sprintf("Club %02d", i), ShortName: fmt.Sprintf("C%02d", i),
			Strength: 3,
		})
	}
	for i := 1; i <= GameweeksPerSeason; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek", IsNext: i == 1})
	}

	// element_type cycle: 1 GKP, 5 DEF, 5 MID, 2 FWD per 13 — the same shape
	// benchPool's quota slice uses, so the two fixtures produce comparable
	// position depth at the same n.
	posCycle := []int{1, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3, 4, 4}

	priors := fakePriors{}
	for i := 0; i < n; i++ {
		pos := posCycle[i%len(posCycle)]
		team := 1 + rng.Intn(teams)
		price := 39 + rng.Intn(112) // tenths of £m, 3.9 to 15.0
		// A coarse 0-159 band drives every rate below, so ties are common —
		// deliberately, matching benchPool's own reasoning.
		band := rng.Intn(160)
		id := i + 1
		b.Elements = append(b.Elements, fpl.Element{
			ID: id, Code: id, Team: team, ElementType: pos,
			WebName: fmt.Sprintf("p%04d", id),
			NowCost: price, Status: "a",
		})
		p := &PriorPlayer{Minutes: 3200, Starts: 36}
		switch pos {
		case 1: // GKP: only clean-sheet involvement matters here.
			p.XGC = 20 + float64(band)*0.2
		case 2: // DEF
			p.XG = float64(band) * 0.02
			p.XA = float64(band) * 0.03
			p.XGC = 25 + float64(band)*0.15
		case 3: // MID
			p.XG = float64(band) * 0.06
			p.XA = float64(band) * 0.05
		case 4: // FWD
			p.XG = float64(band) * 0.12
			p.XA = float64(band) * 0.04
		}
		priors[id] = p
	}

	e := NewEngineFull(b, nil, DefaultWeights(), Congestion{}, RoleRisk{})
	e.Priors = priors
	return e
}

// BenchmarkOptimize is the headline number BenchmarkPolish's doc comment
// says does not exist: the full Engine.Optimize call, end to end, at the
// same pool sizes BenchmarkPolish uses so the two are directly comparable.
//
// ⚠️ It is slow, because it is a strict superset of everything
// BenchmarkPolish measures — see the breakdown recorded in this file's
// companion report. BenchmarkPolish alone runs 273-366ms/op; a single
// BenchmarkOptimize/pool600 op was measured at ~1.4s on this machine. At the
// default -benchtime=1s that is roughly one iteration, which is exactly what
// b.N should do here — do not add -benchtime without deliberately budgeting
// for it (`go test ./internal/analysis/ -bench BenchmarkOptimize -benchtime 5x`
// runs five full builds at pool 600).
//
// Benchmarks do not run under a plain `go test` (only under `-bench`), so
// this does not slow down `go test ./internal/analysis/...` for anyone.
func BenchmarkOptimize(b *testing.B) {
	for _, n := range []int{200, 600} {
		b.Run(fmt.Sprintf("pool%d", n), func(b *testing.B) {
			e := benchOptimizeEngine(n)
			req := OptimizeRequest{Budget: DefaultBudget}

			// Warm every sync.Once-guarded lazy cache on Engine — team
			// strength, the rest-player set, the band cache, the confirmed-
			// starter set, the tournament-absence set — once before timing,
			// so the first b.N iteration does not pay a one-time setup cost
			// that every later call skips. See the Engine struct's own
			// comments on teamOnce/restOnce/bandOnce/confirmedOnce/absenceOnce
			// in metrics.go.
			if _, err := e.Optimize(req); err != nil {
				b.Fatalf("warm-up Optimize: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sq, err := e.Optimize(req)
				if err != nil {
					b.Fatalf("Optimize: %v", err)
				}
				if len(sq.Players) != SquadSize {
					b.Fatal("degenerate result")
				}
			}
		})
	}
}

// BenchmarkGreedyFill is a proxy, not the seed-fill phase whole.
//
// The greedy fill loop — sort by value, then repeatedly add the best
// affordable candidate that still leaves the remaining slots fillable — is
// inlined directly inside Optimize (squad.go, roughly lines 651-670) and is
// not a separately callable function. Extracting it would be a production
// code change, which this task does not permit; reproducing its control
// flow here as test-only code would be a second implementation of exactly
// the logic BenchmarkOptimize above already exercises for real, which is
// this project's own recorded signature failure ("one quantity, two
// implementations").
//
// What genuinely IS callable in isolation, and what the loop's own doc
// comment names as the reason it is expensive — the admissibility bound is
// consulted once per candidate per slot — is fillBound itself. This
// benchmarks that directly, at a realistic call shape: a squad
// partially filled the way the real loop would have left it (a few players
// already selected, respecting position and club quotas), asked whether one
// further candidate still leaves the rest of the squad fillable within
// budget. That is the honest measurement of the identified hot site; the
// outer loop's own overhead (the sort, the affordability check, the
// bookkeeping) is comparatively cheap and not this benchmark's subject.
func BenchmarkGreedyFill(b *testing.B) {
	for _, n := range []int{200, 600} {
		b.Run(fmt.Sprintf("pool%d", n), func(b *testing.B) {
			pool := benchPool(n)

			selected := map[int]PlayerMetrics{}
			posCount := map[string]int{}
			clubCount := map[string]int{}
			for _, m := range pool {
				if len(selected) >= 8 {
					break
				}
				if posCount[m.Position] >= squadQuota[m.Position] || clubCount[m.Team] >= MaxPerClub {
					continue
				}
				selected[m.ID] = m
				posCount[m.Position]++
				clubCount[m.Team]++
			}
			var pending PlayerMetrics
			for _, m := range pool {
				if _, in := selected[m.ID]; !in {
					pending = m
					break
				}
			}

			// buildFillBound hoists the per-pool index and price tables once,
			// exactly as Optimize does at squad.go:608 — so the timed loop
			// below measures the per-candidate cost the fill loop actually
			// pays, not the one-off setup it pays before the first slot.
			fb := buildFillBound(pool)
			for _, m := range selected {
				if idx, ok := fb.poolIndexByID[m.ID]; ok {
					fb.picked[idx] = true
				}
			}
			// Whatever a squad this far in would plausibly have left; the
			// bound's cost does not depend on the figure, only on how much of
			// the squad is still unfilled.
			const remaining = 500

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got := fb.cost(posCount, clubCount,
					boundParams{id: pending.ID, pos: pending.Position, team: pending.Team},
					remaining)
				if got < 0 {
					b.Fatal("impossible cost")
				}
			}
		})
	}
}

// BenchmarkDPSeeds is the third major phase Optimize runs: an exact DP solve
// of the starting eleven for every legal formation (dpseed.go). It is
// called directly on a zero-value *Engine, matching BenchmarkPolish's own
// pattern — confirmed safe by reading both dpSeeds and seedFor, neither of
// which reads any field off the receiver; the receiver exists only so
// dpSeeds can call e.seedFor as a method.
//
// # Why there are two budget arms
//
// combine (dpseed.go) is O(budget^2), so what budget the DP runs at matters a
// great deal to what this benchmark reports. The original, single-arm version
// ran at budget = benchSquad's own cost + 60 — 1740 + 60 = 1800 tenths for
// benchSquad(benchPool(n)) — which is not what a real Optimize call does: a
// real call runs the DP at req.Budget, DefaultBudget (1000 tenths) absent an
// override. 1800 against 1000 is not a small difference once squared, and it
// made this benchmark report roughly 91ms/op where the true cost at the
// budget Optimize actually uses is roughly 30ms/op.
//
// "tight" is kept, unrenamed, so a regression here is still visible against
// its own prior numbers; "DefaultBudget" is the one that describes
// production and is the one to quote.
func BenchmarkDPSeeds(b *testing.B) {
	for _, n := range []int{200, 600} {
		pool := benchPool(n)
		squad := benchSquad(pool)
		spend := 0
		for _, p := range squad {
			spend += priceUnits(p)
		}
		e := &Engine{}
		arms := []struct {
			name   string
			budget int
		}{
			{"tight", spend + 60},
			{"DefaultBudget", DefaultBudget},
		}
		for _, arm := range arms {
			b.Run(fmt.Sprintf("pool%d/%s", n, arm.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					seeds := e.dpSeeds(pool, arm.budget, nil)
					if len(seeds) == 0 {
						b.Fatal("no seeds")
					}
				}
			})
		}
	}
}
