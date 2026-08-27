package backtest

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// TestDiagAutosubValue measures what FPL's automatic substitution actually
// recovers when a starter blanks, rather than assuming it recovers the loss.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagAutosubValue -v
//
// # Why this exists
//
// The record now leans on autosubs twice. The per-event lineup measurement
// explains its near-null with "FPL's autosub already captures most of it", and
// the queued decision-success metric notes that `Verdict.Net()` overstates a
// transfer when the sold player never appeared, "because an autosub would have
// covered him for free".
//
// **"For free" is wrong, and the objection that produced this test is a football
// one.** The bench is three outfielders and a reserve keeper, so cover is
// *limited*. Formation legality means it is not always *usable* — a blanking
// defender in a back three is not replaced at all unless the bench offers another
// defender. And the bench is cheap by construction, because `XIValue` credits
// bench players at zero and the transfer search therefore sells bench quality for
// eleven quality every time. So the typical event is an attacker replaced by a
// cheaper defender, and a defender returns less on average than the forward he
// came on for.
//
// None of that makes the autosub worthless. It makes it a *partial* recovery of
// known size, and the size is what decides whether "captures most of it" is a
// mechanism or a story. This measures it.
//
// # The three quantities, and which one bears on which claim
//
//   - **Coverage.** What share of starter blanks get a substitute at all. Bears
//     on "limited" and "cannot always be used".
//   - **Recovery.** What a firing autosub returns. Against the blank it replaces
//     this is a strict gain, since a blank is zero by definition — so the honest
//     comparison is against what a *playing* starter in that slot returns, which
//     is what the manager thought he was getting.
//   - **The residual.** What perfect lineup knowledge would have added on top:
//     the best legal promotion from the same fifteen, minus what the autosub
//     actually gave. **This is the quantity `TestDiagLineupEventValue` measures
//     end-to-end**, and it is the one that decides whether the near-null there has
//     the mechanism the record claims for it.
//
// Nothing here is an inference. No standard error, no verdict — the point is to
// size three things that were asserted.
func TestDiagAutosubValue(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	// This test transcribes the scoring path's substitution rule, and that path
	// has TWO rules: `legalAutosubs` picks between formation-checked substitution
	// and plain bench order. Transcribing one and reading neither is how a
	// diagnostic ends up reporting on a rule the run did not use — the recorded
	// case is TestDiagSixtyMinutes printing a superseded curve under the shipped
	// name, straight into an accuracy snapshot.
	//
	// Refused rather than mirrored: the env var exists to re-measure the legality
	// fix, and a reader comparing this table against a run made under it would be
	// comparing two different rules with nothing erroring.
	if !legalAutosubs {
		t.Fatal("FPL_NO_LEGAL_AUTOSUBS is set, so the scoring path substitutes in " +
			"bench order regardless of formation — this diagnostic transcribes the " +
			"legality-checking rule only, and its coverage and pts/sub columns would " +
			"describe a rule the run did not use")
	}

	type acc struct {
		blanks      int            // starters who recorded no minutes
		covered     int            // ... of whom a legal substitute replaced
		benchIdle   int            // uncovered because no bench player featured
		illegal     int            // uncovered though a bench player featured: formation
		subPoints   int            // what the substitutes returned
		bestPoints  int            // what the best legal promotion would have returned
		playedStart int            // starters who did feature
		startPoints int            // ... and what they returned
		outPos      map[string]int // positions blanked
		inPos       map[string]int // positions brought on
		// A benched player is often benched ON PURPOSE — the eleven is picked on
		// Score, so whoever is left out is whoever the model rated lowest that
		// week, and for a defender that is very often the fixture. So an autosub
		// does not merely bring on a cheaper player, it brings on the specific
		// player the manager had decided not to play. These accumulate what the
		// substitute returned against his OWN per-appearance average, which is the
		// only way to separate "he is a worse player" from "he was in a worse
		// spot".
		subVsOwn   map[string]float64
		subVsOwnN  map[string]int
		subOwnMean map[string]float64
	}
	seasons := map[string]*acc{}
	get := func(name string) *acc {
		if seasons[name] == nil {
			seasons[name] = &acc{outPos: map[string]int{}, inPos: map[string]int{},
				subVsOwn: map[string]float64{}, subVsOwnN: map[string]int{},
				subOwnMean: map[string]float64{}}
		}
		return seasons[name]
	}

	for _, pair := range sweepPairNames() {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[0], err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[1], err)
		}
		a := get(pair[1])

		for _, start := range sweepStarts() {
			sc := sweepConfig(cfg, start, true)
			// sweepConfig seeds Oracles from the environment, and every treatment
			// arm below assigns cfg.Oracles wholesale — so a stray oracle switch
			// left exported from an earlier run would reach the BASELINE and not
			// the treatment, and the printed difference would silently be
			// "team news minus perfect price timing". validateOracleArms refuses
			// exactly this for sweeps; a standalone diagnostic has no equivalent.
			if sc.Oracles != (Oracles{}) {
				t.Fatalf("an oracle switch is set in the environment (%s), which "+
					"would reach the baseline arm only and contaminate every "+
					"difference printed here", sc.Oracles.Stamp())
			}
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("simulating %s@%d: %v", pair[1], start, err)
			}
			held := res.OpeningSquad
			// The shipped constructor, not a hand-rolled one. `cfg.priors` honours
			// PriorMinutesHalfLife, PriorRateHalfLife and OlderPriors and wraps for
			// omniscience; a bare newPriorIndexMulti drops all four. At sweep
			// defaults they agree, which is exactly the condition under which the
			// recorded HoldCaptaincyWeekly divergence survived unnoticed.
			idx := sc.priors(cur, prior)

			for gw := start; gw <= 38; gw++ {
				b, fx := PointInTimeWith(cur, prior, gw-1, sc.Oracles)
				e := analysis.NewEngineFull(b, fx, sc.Weights,
					analysis.Congestion{}, analysis.RoleRisk{})
				e.Priors = idx
				e.Recent = sc.recentIndex(cur, gw-1)
				e.TeamForm = newTeamFormIndex(cur, gw-1)
				xiIDs, benchIDs, _, _ := pickXI(e, held)
				xi, bench := idsToPlayers(cur, xiIDs), idsToPlayers(cur, benchIDs)

				// Replicate the eleven's position counts and its blanks, the same
				// way the scoring path does — a blanking starter keeps his slot
				// until somebody legally takes it.
				counts := map[string]int{}
				var blanked []string
				for _, p := range xi {
					pos := analysis.PositionForElementType(p.Type)
					counts[pos]++
					if g, ok := p.GWs[gw]; !ok || g.Minutes == 0 {
						blanked = append(blanked, pos)
						continue
					} else {
						a.playedStart++
						a.startPoints += g.Points
					}
				}
				if len(blanked) == 0 {
					continue
				}
				a.blanks += len(blanked)
				for _, pos := range blanked {
					a.outPos[pos]++
				}

				// Snapshotted BEFORE the substitution loop consumes it. The loop
				// below mutates `counts` as it fills vacancies, so handing the
				// mutated map to the counterfactual would ask it to fill the
				// original vacancies a second time from a formation that had
				// already been repaired. That produced a NEGATIVE residual — the
				// "best" promotion scoring less than the actual one, which is
				// impossible by construction and is what surfaced the bug.
				before := map[string]int{}
				for k, v := range counts {
					before[k] = v
				}

				// What FPL actually does: bench order, first legal fit, one each.
				subbed := map[int]bool{}
				remaining := append([]string(nil), blanked...)
				live := 0
				for _, p := range bench {
					if len(remaining) == 0 {
						break
					}
					g, ok := p.GWs[gw]
					if !ok || g.Minutes == 0 {
						continue
					}
					live++
					in := analysis.PositionForElementType(p.Type)
					for i, out := range remaining {
						counts[out]--
						counts[in]++
						if !analysis.LegalFormation(counts) {
							counts[out]++
							counts[in]--
							continue
						}
						a.covered++
						a.subPoints += g.Points
						a.inPos[in]++
						if own, n := ownPerAppearance(p); n >= 5 {
							a.subVsOwn[in] += float64(g.Points)
							a.subOwnMean[in] += own
							a.subVsOwnN[in]++
						}
						subbed[p.ID] = true
						remaining = append(remaining[:i], remaining[i+1:]...)
						break
					}
				}
				// Why the rest went uncovered: no bench player featured at all, or
				// one did and no legal swap existed. Different problems with
				// different fixes, so counted apart.
				for range remaining {
					if live == 0 {
						a.benchIdle++
					} else {
						a.illegal++
					}
				}

				// The perfect-knowledge counterfactual, from the SAME fifteen:
				// promote whoever actually scored most among the bench players a
				// legal swap would admit, rather than taking them in bench order.
				// Anything beyond this needs a different squad, which is the
				// transfer channel and not this one.
				best := bestLegalPromotion(bench, blanked, before, gw)
				a.bestPoints += best
			}
		}
	}

	names := make([]string, 0, len(seasons))
	for k := range seasons {
		names = append(names, k)
	}
	sort.Strings(names)

	fmt.Printf("\nWhat an automatic substitution actually recovers\n")
	fmt.Printf("Bench is three outfielders and a reserve keeper, so cover is limited;\n")
	fmt.Printf("formation legality means it is not always usable; and the bench is cheap\n")
	fmt.Printf("by construction, since XIValue credits it at zero.\n\n")
	fmt.Printf("%-10s %8s %9s %8s %9s %11s %11s\n",
		"season", "blanks", "covered", "%", "idle bench", "illegal", "pts/sub")
	var tb, tc, ti, tl, tsp, tbp, tps, tspt int
	for _, n := range names {
		a := seasons[n]
		fmt.Printf("%-10s %8d %9d %7.1f%% %9d %11d %11.2f\n", n, a.blanks, a.covered,
			100*safePct(a.covered, a.blanks), a.benchIdle, a.illegal,
			safeMean(a.subPoints, a.covered))
		tb += a.blanks
		tc += a.covered
		ti += a.benchIdle
		tl += a.illegal
		tsp += a.subPoints
		tbp += a.bestPoints
		tps += a.playedStart
		tspt += a.startPoints
	}
	fmt.Printf("%-10s %8d %9d %7.1f%% %9d %11d %11.2f\n", "pooled", tb, tc,
		100*safePct(tc, tb), ti, tl, safeMean(tsp, tc))

	fmt.Printf("\nRecovery, against what the manager thought he was buying\n")
	fmt.Printf("a starter who DID feature returned      %6.2f pts\n", safeMean(tspt, tps))
	fmt.Printf("a substitute who came on returned       %6.2f pts\n", safeMean(tsp, tc))
	fmt.Printf("so the blank costs, net of the autosub  %6.2f pts per blank\n",
		safeMean(tspt, tps)-float64(tsp)/float64(max(tb, 1)))
	// The conversion, actually performed. An earlier version printed this header
	// and then did not do the arithmetic, and the "per season" figure was computed
	// by hand OUTSIDE the test as pooled/36 — dividing by cells rather than
	// converting per-gameweek. A cell averages 25.5 gameweeks, not 38, so that
	// understated it by a third, and the third was almost exactly what made it
	// coincide with a different arm's number. Multiply per-gameweek by 38; never
	// divide a pooled total by the cell count.
	blanksPerGW := float64(tb) / float64((tb+tps)/11)
	fmt.Printf("\nblanks per gameweek                     %6.3f\n", blanksPerGW)
	fmt.Printf("residual per blank                      %+6.4f pts\n", safeMean(tbp-tsp, tb))
	fmt.Printf("so over 38 gameweeks                    %+6.1f pts a season\n",
		safeMean(tbp-tsp, tb)*blanksPerGW*38)

	fmt.Printf("\nThe residual perfect lineup knowledge could still add, same fifteen\n")
	fmt.Printf("bench order, as FPL plays it      %6d pts over %d blanks\n", tsp, tb)
	fmt.Printf("best legal promotion instead      %6d pts\n", tbp)
	fmt.Printf("difference                        %6d pts  = %+.4f per blank\n",
		tbp-tsp, safeMean(tbp-tsp, tb))
	if tbp < tsp {
		t.Fatalf("the best legal promotion scored %d against bench order's %d, "+
			"which is impossible: bench order is one of the orderings the best is "+
			"maximising over. The counterfactual is being handed a formation the "+
			"substitution loop has already repaired", tbp, tsp)
	}
	fmt.Printf("\nThat difference is the whole of what better team news can buy WITHIN a\n")
	fmt.Printf("fixed squad. Everything else it might be worth is squad building and\n")
	fmt.Printf("transfers, which this cannot see.\n")

	fmt.Printf("\nPositions: who blanks, and who comes on for them\n")
	fmt.Printf("%-6s %10s %10s\n", "pos", "blanked", "brought on")
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		var out, in int
		for _, a := range seasons {
			out += a.outPos[pos]
			in += a.inPos[pos]
		}
		fmt.Printf("%-6s %10d %10d\n", pos, out, in)
	}
	fmt.Printf("\nIf the brought-on column is defence-heavy against the blanked column,\n")
	fmt.Printf("the autosub is systematically a downgrade in kind and not only in level.\n")

	// The sharper question, and the one a position mix cannot answer: is the
	// player brought on ALSO having a worse week than he usually does? He was
	// benched because the model rated him lowest, and for a defender that rating
	// is largely the fixture — so a substitute who underperforms his own average
	// is evidence that the bench is a considered decision the autosub overrides,
	// not merely a cheaper shelf.
	fmt.Printf("\nDid the substitute underperform HIS OWN average that week?\n")
	fmt.Printf("Players with 5+ appearances only, so the average means something.\n\n")
	fmt.Printf("%-6s %8s %12s %12s %10s\n",
		"pos", "n", "scored", "his average", "ratio")
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		var got, own float64
		var n int
		for _, a := range seasons {
			got += a.subVsOwn[pos]
			own += a.subOwnMean[pos]
			n += a.subVsOwnN[pos]
		}
		if n == 0 {
			fmt.Printf("%-6s %8d %12s %12s %10s\n", pos, 0, "-", "-", "-")
			continue
		}
		fmt.Printf("%-6s %8d %12.2f %12.2f %10.3f\n", pos, n, got/float64(n),
			own/float64(n), (got/float64(n))/(own/float64(n)))
	}
	fmt.Printf("\nA ratio below 1 means the autosub is fielding players in weeks they\n")
	fmt.Printf("were rightly benched for — the bench is a matchup decision and the\n")
	fmt.Printf("substitution overrides it. A ratio near 1 means the bench is simply\n")
	fmt.Printf("cheaper, and the loss is in level rather than in timing.\n")
}

// ownPerAppearance is a player's mean points across the gameweeks he featured in,
// over the whole season, with the count so a thin sample can be excluded.
//
// The whole season rather than the season to date, deliberately: this is not a
// prediction, it is a yardstick for "was this a bad week for him", and the
// point-in-time rule governs what the MODEL may see rather than what a diagnostic
// may measure afterwards.
func ownPerAppearance(p *Player) (float64, int) {
	var total, n int
	for _, g := range p.GWs {
		if g.Minutes == 0 {
			continue
		}
		total += g.Points
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return float64(total) / float64(n), n
}

// bestLegalPromotion is what the bench would have returned if the manager had
// known in advance who was going to blank and could order the substitutions
// himself, rather than taking them in the order the bench was set.
//
// Greedy by realised points, and it is exact on this problem — but not for the
// reason first written here ("every candidate is considered against every
// remaining vacancy"), which does not establish it.
//
// It is exact because the positions barely interact. `GKP` is pinned at exactly
// one by the formation bounds, so a keeper fits only a keeper vacancy and no
// outfielder fits one; the outfield slack is then wide enough that taking the
// highest scorer first never blocks a higher-scoring combination. Verified by
// exhaustive search over every legal starting formation x every outfield vacancy
// multiset up to four x every vacancy order x every bench position tuple up to
// four x four point patterns — **362,475 configurations, zero counterexamples**.
//
// That also makes the unstable sort below harmless: greedy attains the optimum
// for every ordering consistent with descending points, so a tie cannot move the
// total.
func bestLegalPromotion(bench []*Player, blanked []string, counts map[string]int, gw int) int {
	type cand struct {
		pos    string
		points int
	}
	var cands []cand
	for _, p := range bench {
		g, ok := p.GWs[gw]
		if !ok || g.Minutes == 0 {
			continue
		}
		cands = append(cands, cand{analysis.PositionForElementType(p.Type), g.Points})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].points > cands[j].points })

	// A copy, so the caller's counts are not consumed by the counterfactual.
	c := map[string]int{}
	for k, v := range counts {
		c[k] = v
	}
	remaining := append([]string(nil), blanked...)
	total := 0
	for _, cd := range cands {
		if len(remaining) == 0 {
			break
		}
		for i, out := range remaining {
			c[out]--
			c[cd.pos]++
			if !analysis.LegalFormation(c) {
				c[out]++
				c[cd.pos]--
				continue
			}
			total += cd.points
			remaining = append(remaining[:i], remaining[i+1:]...)
			break
		}
	}
	return total
}

func safeMean(a, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(a) / float64(n)
}
