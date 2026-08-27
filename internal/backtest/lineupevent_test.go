package backtest

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
)

// TestDiagLineupEventValue prices perfect lineup knowledge PER GAMEWEEK rather
// than per season, because the season total is the wrong instrument for it.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagLineupEventValue -v
//
// # Why the season total cannot answer this and this can
//
// `OracleLineups` — perfect knowledge of who is picked, priced at each player's
// own conditional average — measures **+1.932 points a gameweek held, about 73 a
// season, at a clustered t of 1.32**. It does not resolve, and its own detection
// threshold is about 177 a season. That reads as "we cannot tell", and it has been
// mistaken for "there is nothing there".
//
// The arithmetic says otherwise. Roughly 6% of players miss a given week, so an
// eleven carries about 0.66 blanking starters a week and about 25 a season. At one
// to two and a half points recovered per event that is 25 to 63 points — the same
// order as the measured 73. **The effect is almost certainly real; the instrument
// simply cannot see it.**
//
// The reason is that a season total is one number per cell, and the quantity
// varies enormously between seasons: in a year where the optimiser happens to buy
// fifteen nailed players the oracle is worth nearly nothing, and in an injury-strewn
// one it is worth a great deal. That is genuine heterogeneity rather than noise, and
// no number of entry points reduces it.
//
// So this decomposes the season total into the two things it multiplies:
//
//	season value  =  how often the knowledge fires  x  what it is worth when it does
//
// The **rate** is what varies by season. The **value per firing** need not, and if it
// is stable it can be estimated far more precisely than the product — hundreds of
// gameweeks rather than twenty-four cells. Reporting them separately says which half
// the uncertainty lives in, which a single total cannot.
//
// # What is held fixed, and why that is the point
//
// Both arms hold the **same opening fifteen** — the one the shipped model buys — and
// differ only in what the weekly re-pick knows. So this measures the XI-selection
// layer alone: *given the squad you own, what is knowing the lineups worth?* That is
// deliberately narrower than the oracle's own figure, which also lets the knowledge
// change which fifteen is bought. The narrower question is the one with a clean
// counterfactual, and it is the one the "you get eighteen chances a season to bench a
// non-starter" argument is actually about.
//
// ⚠️ **This is not a replacement for the season figure and must not be quoted as
// one.** A per-event gain does not aggregate to a season gain by multiplication when
// a policy is in the loop — the record's most-repeated finding is that a better
// predictor can make a worse policy. What this can establish is whether the effect
// exists and how large each firing is; whether the policy converts it is the season
// measurement's job, and that one still does not resolve.
func TestDiagLineupEventValue(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	type ev struct {
		season string
		start  int
		arm    string
		diffs  []int // per-gameweek difference, oracle minus shipped
	}
	var all []ev

	for _, pair := range sweepPairNames() {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[0], err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatalf("loading %s: %v", pair[1], err)
		}
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

			// One squad, bought by the shipped model, held by both arms. The
			// oracle is switched on only for the weekly re-pick, so a difference
			// is the XI decision and cannot be the squad.
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("simulating %s@%d: %v", pair[1], start, err)
			}
			held := res.OpeningSquad

			base := HoldWeekly(cur, prior, sc, held)

			// Three arms over the same held fifteen. The two restrictions
			// partition the population the unrestricted arm covers, so reading
			// them against each other says WHERE the value is rather than only
			// how much of it there is — and a restricted arm is sparse, which on
			// this harness is about an order of magnitude easier to resolve.
			for _, a := range lineupArms {
				oc := a.apply(sc)
				if err := oc.Oracles.Validate(); err != nil {
					t.Fatalf("%s: %v", a.label, err)
				}
				with := HoldWeekly(cur, prior, oc, held)

				if len(base) != len(with) {
					t.Fatalf("%s@%d %s: %d weeks against %d",
						pair[1], start, a.label, len(base), len(with))
				}
				d := make([]int, len(base))
				for i := range base {
					d[i] = with[i] - base[i]
				}
				all = append(all, ev{season: pair[1], start: start, arm: a.label, diffs: d})
			}
		}
	}

	// The mediator first. An arm that never fires is "could not run", not "no
	// effect", and this project has confused the two before.
	fmt.Printf("\nPerfect knowledge of who plays, priced per gameweek, on two channels\n")
	fmt.Printf("Same opening fifteen in every arm; only the weekly re-pick differs.\n")
	fmt.Printf("lineups  rewrites the recency index, so it reaches ExpectedMinutes.\n")
	fmt.Printf("features rewrites Status, so it reaches availabilityFactor.\n")
	fmt.Printf("flagged/unflagged split on the model's OWN reconstruction at the\n")
	fmt.Printf("cutoff, with no oracle applied, so the restriction is defined by\n")
	fmt.Printf("what the model could have known rather than by hindsight.\n\n")
	fmt.Printf("%-16s %8s %10s %8s %8s %13s %12s\n",
		"arm", "weeks", "changed", "better", "worse", "pts/firing", "pts/season")

	byArm := map[string][]ev{}
	for _, e := range all {
		byArm[e.arm] = append(byArm[e.arm], e)
	}
	seasonOf := map[string]map[string][]int{}
	for _, a := range lineupArms {
		var weeks, fired, gains, losses int
		var total float64
		seasonOf[a.label] = map[string][]int{}
		for _, e := range byArm[a.label] {
			seasonOf[a.label][e.season] = append(seasonOf[a.label][e.season], e.diffs...)
			for _, d := range e.diffs {
				weeks++
				total += float64(d)
				switch {
				case d > 0:
					fired++
					gains++
				case d < 0:
					fired++
					losses++
				}
			}
		}
		if weeks == 0 {
			t.Fatalf("arm %q scored no gameweeks; it could not run", a.label)
		}
		fmt.Printf("%-16s %8d %9.1f%% %8d %8d %13.3f %12.1f\n",
			a.label, weeks, 100*float64(fired)/float64(weeks), gains, losses,
			safeDiv(total, fired), total/float64(weeks)*38)
	}

	// Split by whether the season's `starts` column is real. Before 2022-23 it is
	// absent and reconstructStarts infers it by ranking minutes, biased toward
	// making fringe players look nailed — so on those seasons the "oracle" is
	// confidently wrong rather than perfect, and pooling the two answers a
	// different question from the one asked.
	realStarts := map[string]bool{"2023-24": true, "2024-25": true, "2025-26": true}
	fmt.Printf("\nby data quality, points a season\n")
	fmt.Printf("A reconstructed season is not a perfect oracle: it is a confident wrong one.\n\n")
	fmt.Printf("%-16s %14s %16s\n", "arm", "real starts", "reconstructed")
	for _, a := range lineupArms {
		var rw, cw int
		var rt, ct float64
		for season, d := range seasonOf[a.label] {
			for _, v := range d {
				if realStarts[season] {
					rw++
					rt += float64(v)
				} else {
					cw++
					ct += float64(v)
				}
			}
		}
		fmt.Printf("%-16s %14.1f %16.1f\n", a.label,
			safeDiv(rt, rw)*38, safeDiv(ct, cw)*38)
	}

	// Per season and arm, so the rate and the value per firing can be read apart.
	// The claim under test in the unrestricted arm was that the RATE is what
	// varies by season while the VALUE per firing is steadier; the restrictions
	// say whether that holds on each half of the population separately.
	fmt.Printf("\nper season\n")
	fmt.Printf("%-16s %-10s %8s %10s %14s %14s\n",
		"arm", "season", "weeks", "fired %", "pts/firing", "pts/season")
	for _, a := range lineupArms {
		names := make([]string, 0, len(seasonOf[a.label]))
		for k := range seasonOf[a.label] {
			names = append(names, k)
		}
		sort.Strings(names)
		var perFiring, perSeason []float64
		for _, name := range names {
			d := seasonOf[a.label][name]
			var f int
			var tot float64
			for _, v := range d {
				tot += float64(v)
				if v != 0 {
					f++
				}
			}
			perFiring = append(perFiring, safeDiv(tot, f))
			perSeason = append(perSeason, tot/float64(len(d))*38)
			fmt.Printf("%-16s %-10s %8d %9.1f%% %14.3f %14.1f\n",
				a.label, name, len(d), 100*float64(f)/float64(len(d)),
				safeDiv(tot, f), tot/float64(len(d))*38)
		}
		fmt.Printf("%-16s %-10s %8s %10s %14.3f %14.1f\n",
			"", "spread", "", "", sd(perFiring), sd(perSeason))
	}

	fmt.Printf("\nA flagged arm bounds perfect USE of information the model already has.\n")
	fmt.Printf("An unflagged arm bounds what no amount of reading a flag could have told\n")
	fmt.Printf("you, which is what a lineup-prediction source is actually for.\n")
	fmt.Printf("\nlineups:flagged reading exactly 0.000 is the DECLARED invariance, not a\n")
	fmt.Printf("null: a flagged player's Score is multiplied by zero before his minutes\n")
	fmt.Printf("are consulted, so that arm cannot fire. If it ever moves, the zero in\n")
	fmt.Printf("availabilityFactor has gone, and every figure here needs re-deriving.\n")
	fmt.Printf("\nInference belongs in R; no standard error or verdict is printed here.\n")
}

// lineupArms partitions the players a lineups oracle may know about. "flagged"
// is what statusAt reconstructs at the cutoff, with no oracle applied — the
// availability the model already had — so the two restrictions are a partition
// of the whole pool and their difference is the population, nothing else.
//
// Note statusAt on the replay emits only a/u/i and never d: there is no
// percentage flag in the archive. So "flagged" here is coarser than FPL's live
// status, and this bounds the coarse version.
var lineupArms = []struct {
	label string
	// apply turns the shipped cell config into this arm's. Returning the config
	// rather than mutating it keeps every arm a pure function of the baseline,
	// so no arm can inherit a field an earlier one set.
	apply func(SimConfig) SimConfig
}{
	// The minutes channel: OracleLineups, restricted three ways. The two
	// restrictions partition the pool the unrestricted arm covers.
	{"lineups: all", func(c SimConfig) SimConfig {
		c.Oracles = Oracles{Info: OracleLineups}
		return c
	}},
	{"lineups: flagged", func(c SimConfig) SimConfig {
		c.Oracles = Oracles{Info: OracleLineups}
		c.lineupCovers = func(_ *Player, st string) bool { return st != "a" }
		return c
	}},
	{"lineups: unflagged", func(c SimConfig) SimConfig {
		c.Oracles = Oracles{Info: OracleLineups}
		c.lineupCovers = func(_ *Player, st string) bool { return st == "a" }
		return c
	}},
	// The availability channel: OracleFeatures, the same three ways. This is the
	// channel the lineups oracle cannot reach for a flagged player, because
	// availabilityFactor zeroes his Score before ExpectedMinutes is consulted —
	// which is why "lineups: flagged" is expected to read exactly 0.000 and
	// "features: flagged" is not.
	{"features: all", func(c SimConfig) SimConfig {
		c.Oracles = Oracles{Info: OracleFeatures}
		return c
	}},
	{"features: flagged", func(c SimConfig) SimConfig {
		c.Oracles = Oracles{Info: OracleFeatures, FeatureScope: FeaturesFlagged}
		return c
	}},
	{"features: unflagged", func(c SimConfig) SimConfig {
		c.Oracles = Oracles{Info: OracleFeatures, FeatureScope: FeaturesUnflagged}
		return c
	}},
}

func safeDiv(a float64, n int) float64 {
	if n == 0 {
		return math.NaN()
	}
	return a / float64(n)
}
