package backtest

// Where does the objective diverge from reality?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagObjectiveDivergence -v -timeout 2h
//
// The unified bounded-revision search loses to the bespoke pair-then-singles
// policy by 0.771 points per gameweek (t = -2.52) while making the *same* number
// of transfers — 502 against 496. It is not over-eager, it is picking worse
// moves at the same volume.
//
// That is a statement about the **objective**, not the search. Both maximise
// XIValue; unified simply maximises it harder, over a larger space. If a
// stronger search reliably finds worse football, then XIValue is systematically
// over-valuing something, and the moves unified reaches for that bespoke does
// not are exactly the sample where that something is concentrated.
//
// # Why this compares at a common squad
//
// The naive design runs both policies and diffs their transfers. It does not
// work: the moment they disagree they hold different fifteens, so every later
// decision is made from a different position and nothing after the first
// divergence is attributable.
//
// So the path is canonical — the bespoke policy plays the season — and at every
// gameweek *both* searches are asked what they would do **from the squad
// bespoke actually holds**. Neither proposal is applied. That isolates the
// decision rule from the path, the same trick Judge uses for a single move.
//
// # What is measured
//
// For each disagreement, both proposals get:
//
//	modelled gain   XIValue(after) - XIValue(before), the objective's own answer
//	realised gain   what the players actually scored over the decision horizon
//
// Unified's modelled gain is >= bespoke's by construction, since it searches a
// superset. The question is whether that **excess modelled gain buys any excess
// realised gain**. If it does not, the objective is being exploited, and the
// buckets say where.
//
// # A deliberate approximation
//
// Selling prices are taken at market rather than from the wallet, which is not
// what Simulate does. It applies identically to both arms, so the *comparison*
// is unaffected; only the absolute affordability of a given move shifts
// slightly. Recorded because it would matter if these numbers were ever read as
// a policy result rather than as a contrast.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

type divergence struct {
	gw int
	// modelled and realised gains for each arm, per gameweek of the horizon.
	bespokeModelled, bespokeRealised float64
	unifiedModelled, unifiedRealised float64
	// Features of the unified proposal, for bucketing.
	moves       int
	maxInPrice  float64
	minInMins   float64
	inPositions string
}

func (d divergence) excessModelled() float64 { return d.unifiedModelled - d.bespokeModelled }
func (d divergence) excessRealised() float64 { return d.unifiedRealised - d.bespokeRealised }

func TestDiagObjectiveDivergence(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	pairs := sweepPairNames()
	starts := []int{1, 11, 21}

	var all []divergence

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)

		for _, start := range starts {
			base := sweepConfig(cfg, start, false)
			res, err := Simulate(cur, prior, base)
			if err != nil {
				t.Fatal(err)
			}

			for wi, wk := range res.Weeks {
				if wi == 0 || len(wk.Squad) != 15 {
					continue
				}
				gw := wk.GW
				// The squad the decision was made *from* is last week's.
				held := res.Weeks[wi-1].Squad
				if len(held) != 15 {
					continue
				}
				bank := res.Weeks[wi-1].Bank

				pb, pf := PointInTime(cur, prior, gw-1)
				e := analysis.NewEngineFull(pb, pf, cfg.Weights,
					analysis.Congestion{}, analysis.RoleRisk{})
				e.Priors = idx
				e.Recent = newRecentIndexWith(cur, gw-1,
					base.minutesHalfLife(), cfg.Weights.RateHalfLife)

				horizon := effectiveHorizon(cfg.Weights.Horizon, gw)
				weeks := int(horizon)
				if weeks < 1 {
					continue
				}
				// The *real* allowance this week, not an assumed one. The policy
				// banks up to five free transfers, so a week can carry six
				// moves; capping the diagnostic at two tested a far more
				// restricted search than the one whose loss is being explained,
				// and never exercised the large-change weeks at all.
				free := res.Weeks[wi-1].Free
				if free < 1 {
					free = 1
				}
				limit := moveLimit(free, base.MaxHits, base.MaxMoves, base.HitCeiling)

				bMove, _, bIn := bestSwap(e, held, bank, nil, analysis.ChipCredit{})
				var bProposal []Move
				if bIn.ID != 0 {
					bProposal = []Move{bMove}
				}
				uProposal, _ := unifiedDecide(e, cur, held, bank, free, limit, gw,
					horizon, base.FreeCost, base, nil)

				if sameMoves(bProposal, uProposal) {
					continue
				}
				d := divergence{
					gw:              gw,
					bespokeModelled: modelledGain(e, held, bProposal),
					unifiedModelled: modelledGain(e, held, uProposal),
					bespokeRealised: realisedGain(cur, bProposal, gw, weeks),
					unifiedRealised: realisedGain(cur, uProposal, gw, weeks),
					moves:           len(uProposal),
					minInMins:       999,
				}
				byID := map[int]analysis.PlayerMetrics{}
				for _, m := range e.AllMetrics() {
					byID[m.ID] = m
				}
				var pos []string
				for _, mv := range uProposal {
					m := byID[mv.InID]
					if m.Price > d.maxInPrice {
						d.maxInPrice = m.Price
					}
					if m.ExpectedMinutes < d.minInMins {
						d.minInMins = m.ExpectedMinutes
					}
					pos = append(pos, m.Position)
				}
				sort.Strings(pos)
				d.inPositions = fmt.Sprint(pos)
				all = append(all, d)
			}
		}
	}

	if len(all) < 20 {
		t.Skipf("only %d disagreements", len(all))
	}

	fmt.Printf("\n%d gameweeks where the two searches disagree, %s.\n",
		len(all), gridLabel(len(pairs), len(starts)))
	fmt.Printf("Both asked from the same squad; neither applied. Gains are per gameweek\n")
	fmt.Printf("over the decision horizon.\n\n")

	// overvaluation is excess modelled minus excess realised, per disagreement:
	// what the objective claims unified's extra freedom is worth, minus what it
	// actually delivered. Its mean is the "pure over-valuation" figure quoted in
	// AGENTS.md; a one-sample t against zero (11 degrees of freedom or fewer,
	// depending on bucket size) is the significance test that number never had.
	summarise := func(label string, rows []divergence) {
		if len(rows) == 0 {
			return
		}
		var em, er, um, ur, bm, br float64
		var overvals []float64
		for _, r := range rows {
			em += r.excessModelled()
			er += r.excessRealised()
			um += r.unifiedModelled
			ur += r.unifiedRealised
			bm += r.bespokeModelled
			br += r.bespokeRealised
			overvals = append(overvals, r.excessModelled()-r.excessRealised())
		}
		n := float64(len(rows))
		fmt.Printf("%-22s %5d %9.3f %9.3f %9.3f %9.3f %+10.3f %+10.3f\n",
			label, len(rows), bm/n, br/n, um/n, ur/n, em/n, er/n)
		if ovm, ovse := meanSE(overvals); ovse > 0 {
			t := ovm / ovse
			verdict := "noise"
			switch a := math.Abs(t); {
			case a >= 3:
				verdict = "STRONG"
			case a >= 2:
				verdict = "real"
			case a >= 1:
				verdict = "weak"
			}
			fmt.Printf("%-22s %5s over-valuation %+.3f  SE %.3f  t %+.2f  %s\n",
				"", "", ovm, ovse, t, verdict)
		}
	}

	fmt.Printf("%-22s %5s %9s %9s %9s %9s %10s %10s\n",
		"", "n", "besp mdl", "besp real", "uni mdl", "uni real",
		"excess mdl", "excess real")
	summarise("all disagreements", all)

	fmt.Printf("\nby size of the unified proposal:\n")
	for _, k := range []int{1, 2, 3, 4} {
		var rows []divergence
		for _, r := range all {
			if r.moves == k {
				rows = append(rows, r)
			}
		}
		summarise(fmt.Sprintf("  %d player(s) changed", k), rows)
	}

	fmt.Printf("\nby the most expensive player bought:\n")
	for _, b := range []struct {
		label  string
		lo, hi float64
	}{{"  under 6.0m", 0, 6.0}, {"  6.0-9.0m", 6.0, 9.0}, {"  9.0m and up", 9.0, 99}} {
		var rows []divergence
		for _, r := range all {
			if r.maxInPrice >= b.lo && r.maxInPrice < b.hi {
				rows = append(rows, r)
			}
		}
		summarise(b.label, rows)
	}

	fmt.Printf("\nby the least-nailed player bought (expected minutes per gameweek):\n")
	for _, b := range []struct {
		label  string
		lo, hi float64
	}{{"  under 60", 0, 60}, {"  60-75", 60, 75}, {"  75 and up", 75, 999}} {
		var rows []divergence
		for _, r := range all {
			if r.minInMins >= b.lo && r.minInMins < b.hi {
				rows = append(rows, r)
			}
		}
		summarise(b.label, rows)
	}

	fmt.Printf("\n'excess mdl' is what the objective says unified's extra freedom is worth.\n")
	fmt.Printf("'excess real' is what it actually delivered. A large positive excess\n")
	fmt.Printf("modelled against a flat or negative excess realised is the objective\n")
	fmt.Printf("being exploited, and the buckets say where it is worst.\n")
}

func sameMoves(a, b []Move) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[[2]int]bool{}
	for _, m := range a {
		seen[[2]int{m.OutID, m.InID}] = true
	}
	for _, m := range b {
		if !seen[[2]int{m.OutID, m.InID}] {
			return false
		}
	}
	return true
}

// modelledGain is what the objective believes a proposal is worth, per gameweek.
func modelledGain(e *analysis.Engine, held []int, moves []Move) float64 {
	if len(moves) == 0 {
		return 0
	}
	before := analysis.XIValue(squadMetrics(e, held))
	after := held
	for _, mv := range moves {
		after = applyMove(after, mv)
	}
	return analysis.XIValue(squadMetrics(e, after)) - before
}

// realisedGain is what the players involved actually returned, per gameweek.
func realisedGain(s *Season, moves []Move, gw, weeks int) float64 {
	if len(moves) == 0 || weeks < 1 {
		return 0
	}
	var net float64
	for _, mv := range moves {
		net += float64(pointsOver(s, mv.InID, gw, weeks) -
			pointsOver(s, mv.OutID, gw, weeks))
	}
	return net / float64(weeks)
}
