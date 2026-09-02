package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"armband/internal/analysis"
)

// BenchmarkBuildState times the real /api/state pipeline over the committed GW1
// capture -- no network, no clock, byte-identical inputs every run (see
// fixtureServer). It exists because optimizerbench_test.go's benchmarks are
// deliberately narrow: BenchmarkPolish et al. measure the local search's own
// arithmetic in isolation, but AllMetrics' scoring pass over the whole
// candidate pool, buildSquadPage's page assembly, and viewmodel.Build all run
// on every request too and none of them had a benchmark at all -- see that
// file's own comment on BenchmarkPolish for why Optimize itself was called
// unbenchmarkable ("cannot be benchmarked without the live API"), which the
// committed capture makes untrue.
//
// Three sub-benchmarks, because a page reaches buildState by one of three
// routes that cost wildly different amounts (see pageOpts' own comment) and
// averaging them would hide a regression in any one behind the other two:
//
//   - fresh: no session at all, the first load of a new visitor -- a varied
//     squad, no search.
//   - optimized: sess.Optimised -- the Optimize button, the true search, and
//     the one this suite exists for: it is the "our bottleneck" path.
//   - known_squad: a session with a saved fifteen already -- the reload path,
//     which pageOpts' own comment says should need arithmetic
//     (analysis.BestXI) rather than a search. Kept as the cheap baseline: if
//     this one regresses too, the problem is not the search.
func BenchmarkBuildState(b *testing.B) {
	s := fixtureServer(b)
	req := httptest.NewRequest("GET", routeState, nil)
	req.Host = "127.0.0.1:8080"

	run := func(b *testing.B, sess session) {
		b.Helper()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := s.buildState(req, sess, 0, nil, nil, nil, analysis.BudgetTrust{}); err != nil {
				b.Fatal(err)
			}
		}
	}

	b.Run("fresh", func(b *testing.B) {
		run(b, session{})
	})

	b.Run("optimized", func(b *testing.B) {
		run(b, session{Optimised: true})
	})

	b.Run("known_squad", func(b *testing.B) {
		run(b, knownSquadSession(b, s))
	})
}

// knownSquadSession runs one untimed optimize pass to get a real, legal
// fifteen out of the fixture engine, then converts it from element ids (what
// analysis.Squad and the optimiser deal in) to permanent codes (what session
// stores -- see session's own doc comment on why the two keyspaces must
// never be crossed silently). Mirrors the byElement[el.ID] = el.Code idiom
// webroutes.go and importteam.go already use for this exact conversion.
func knownSquadSession(b *testing.B, s *squadServer) session {
	b.Helper()
	cfg := s.effectiveCfgFrom(session{})
	build, err := buildSquadPage(context.Background(), cfg, s.client, s.engine, pageOpts{
		Weeks: s.weeks, WantPage: false, Now: fixtureNow, Optimised: true,
	})
	if err != nil {
		b.Fatalf("seeding a known squad: %v", err)
	}

	byElement := map[int]int{}
	for _, el := range s.engine.Boot.Elements {
		byElement[el.ID] = el.Code
	}
	toCodes := func(ids []int) []int {
		out := make([]int, len(ids))
		for i, id := range ids {
			out[i] = byElement[id]
		}
		return out
	}

	sq := build.Squad
	squadIDs := make([]int, len(sq.Players))
	for i, p := range sq.Players {
		squadIDs[i] = p.ID
	}
	xiIDs := make([]int, len(sq.StartingXI))
	for i, p := range sq.StartingXI {
		xiIDs[i] = p.ID
	}
	benchIDs := make([]int, len(sq.Bench))
	for i, p := range sq.Bench {
		benchIDs[i] = p.ID
	}

	return session{
		Squad:   toCodes(squadIDs),
		XI:      toCodes(xiIDs),
		Bench:   toCodes(benchIDs),
		Captain: byElement[sq.Captain.ID],
		Vice:    byElement[sq.ViceCaptain.ID],
	}
}
