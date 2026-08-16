package backtest

// Where does the season-to-season spread actually come from?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagVarianceDecomposition -v -timeout 2h
//
// AGENTS.md records the coarsest possible split of this question: `hold` versus
// `policy` for the same perturbation moves −12 and +51, so squad selection is
// stable and the transfer path carries most of the sensitivity. That bundles
// four different mechanisms into "hold" without separating them: which fifteen
// you own, who starts each week, who wears the armband, and whether a blank
// gets covered by the bench. Captaincy in particular has never been isolated,
// despite doubling a single player being an obvious source of variance on its
// own.
//
// # The five layers, each built from primitives that already ship
//
// No scoring code changes for this — every layer is `weekPoints` called with a
// different XI/bench/captain, which is exactly the shape `HoldWeekly` already
// uses for the whole-hold figure.
//
//	frozen base       XI picked ONCE at the start, never re-picked, no captain
//	                   bonus (captain id 0), no autosub (bench omitted). Pure
//	                   squad-selection stability: what fifteen you happened to
//	                   buy, nothing else.
//	frozen + captain   same frozen XI, captain bonus restored — but the captain
//	                   is also fixed at whoever was picked on day one. Isolates
//	                   what the armband is worth *by itself*, holding who starts
//	                   constant.
//	weekly XI+captain  XI and captain re-picked every week from current scores,
//	                   still no autosub. Adds "which of my fifteen do I start,
//	                   and who do I captain, given what I know this week" on top
//	                   of the frozen baseline.
//	+ autosub          the same weekly pick, with the bench available to cover a
//	                   blank. This is `HoldWeekly` exactly.
//	+ transfers        the full policy. Already measured everywhere else in this
//	                   file; included for completeness of the ladder.
//
// Each step differs from the one before it in exactly one mechanism, so the
// difference between two adjacent rows is that mechanism's contribution — to
// the season total under the "static" heading, and to the *sensitivity* under
// a nudge, which is the question AGENTS.md actually asks. `MinutesWeight`
// 1.25 -> 1.00 is reused as the nudge because it is already characterised on
// `HOLD`: -0.717 pts/gw, t = -2.06.
//
// # Reading the sensitivity table
//
// If a layer's own paired difference is close to zero, that mechanism is not
// where `HOLD`'s sensitivity to this nudge lives, whatever its static point
// total looks like. A large static contribution and a near-zero sensitivity is
// a real and useful combination: the mechanism matters for points, and is
// stable regardless of how the underlying scores move.
//
// # This ladder answers "where do the points come from", not "which metric to tune on"
//
// Every layer here freezes the *eleven* at the day-one pick and adds mechanisms
// back one at a time, so no rung of it is a usable substitute for HOLD — a
// frozen eleven is not a squad anybody fields. The related but different
// question, "is there a lower-noise instrument that still responds to a scoring
// constant", is asked by the two captaincy rungs in `HoldCaptaincy`
// (`hold_fixedcap` and `hold_nocap`), which keep the weekly eleven and vary only
// the armband. Every ordinary sweep emits those; this decomposition does not.

import (
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

// layeredWeekly returns, for every gameweek from cfg.startGW() to 38, the four
// point totals described above. held is the fixed fifteen; no transfers are
// made, matching HoldWeekly.
func layeredWeekly(cur, prior *Season, cfg SimConfig, held []int) (frozenBase, frozenCaptain, weeklyNoSub, weeklyAutosub []int) {
	idx := newPriorIndexMulti(append([]*Season{prior}, cfg.OlderPriors...), cfg.PriorHalfLife)
	start := cfg.startGW()

	// The frozen XI and captain are picked once, from the engine as it stood
	// the week the squad was bought — the same point PointInTime is called from
	// for the opening squad everywhere else in this file.
	fb, ff := PointInTime(cur, prior, start-1)
	fe := analysis.NewEngineFull(fb, ff, cfg.Weights, analysis.Congestion{}, analysis.RoleRisk{})
	fe.Priors = idx
	fe.Recent = newRecentIndexWith(cur, start-1, cfg.minutesHalfLife(), cfg.Weights.RateHalfLife)
	frozenXI, _, frozenCapt, frozenVice := pickXI(fe, held)
	frozenPlayers := idsToPlayers(cur, frozenXI)

	for gw := start; gw <= 38; gw++ {
		b, fx := PointInTime(cur, prior, gw-1)
		e := analysis.NewEngineFull(b, fx, cfg.Weights, analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = idx
		e.Recent = newRecentIndexWith(cur, gw-1, cfg.minutesHalfLife(), cfg.Weights.RateHalfLife)
		wxi, wbench, wcaptain, wvice := pickXI(e, held)
		wxiPlayers := idsToPlayers(cur, wxi)
		wbenchPlayers := idsToPlayers(cur, wbench)

		frozenBase = append(frozenBase, weekPoints(frozenPlayers, nil, gw, 0, 0))
		frozenCaptain = append(frozenCaptain, weekPoints(frozenPlayers, nil, gw, frozenCapt, frozenVice))
		weeklyNoSub = append(weeklyNoSub, weekPoints(wxiPlayers, nil, gw, wcaptain, wvice))
		weeklyAutosub = append(weeklyAutosub, weekPoints(wxiPlayers, wbenchPlayers, gw, wcaptain, wvice))
	}
	return frozenBase, frozenCaptain, weeklyNoSub, weeklyAutosub
}

// sumFloats is sumInts for a metric whose per-gameweek figure is an expectation
// rather than a score — the accumulated-xPoints rungs.
func sumFloats(xs []float64) float64 {
	var n float64
	for _, x := range xs {
		n += x
	}
	return n
}

func sumInts(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

func TestDiagVarianceDecomposition(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	// Parsed once for the whole test rather than reloaded inside each arm, which
	// is what the old inline Load did — the second arm was paying to parse four
	// archives it had already parsed.
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	// The per-cell CSV, when FPL_CELLS is set. Every standard error, df and
	// p-value for this decomposition is computed in stats/sweep_inference.R from
	// these rows — see the note on the marginal table at the foot of this test.
	sink, err := openCellSink(os.Getenv("FPL_CELLS"))
	if err != nil {
		t.Fatal(err)
	}
	defer sink.close()
	label := os.Getenv("EXP")
	if label == "" {
		label = t.Name()
	}
	sweep := sink.sweepLabel(label)

	type layerTotals struct{ base, capt, weekly, autosub, policy float64 }
	// keyed "season@start", per gameweek-normalised
	shipped := map[string]layerTotals{}
	nudged := map[string]layerTotals{}

	// Built once each, because the means file joins to the cells file on the
	// label and two constructions of one string is the thing this whole change is
	// about. Drift would be caught — every metric shares the label, so nothing
	// would join — but caught loudly is not the same as impossible.
	nudgeTo := 1.00
	if cfg.Weights.MinutesWeight != 1.25 {
		nudgeTo = cfg.Weights.MinutesWeight - 0.25 // stay off whatever is shipped
	}
	shippedLabel := fmt.Sprintf("MinutesWeight=%.2f (ships)", cfg.Weights.MinutesWeight)
	nudgedLabel := fmt.Sprintf("MinutesWeight=%.2f", nudgeTo)

	// The two arms, declared as policyVariants rather than as two bare labels.
	//
	// This test emits cells and a means file, so it needs everything a sweep needs
	// and it was getting only some of it. The arms carry an `apply` so that
	// oraclesOf can read the hindsight each one installs off a probe config —
	// which is how the means rows below learn their stamp, and how this path
	// reaches validateOracleArms.
	arms := []policyVariant{
		{label: shippedLabel, apply: func(sc *SimConfig) {
			sc.Weights.MinutesWeight = cfg.Weights.MinutesWeight
		}},
		{label: nudgedLabel, apply: func(sc *SimConfig) {
			sc.Weights.MinutesWeight = nudgeTo
		}},
	}

	// The same stamp runPolicySweep writes, for the same reason: any cells file is
	// unattributable without one, and this test emits cells too. Both arms are
	// declared up front so a kill leaves the gap visible. Written before the probe
	// below, so the environment it fingerprints is the one the run started in.
	writeSweepProvenance(t, sweep, sink, cfg, arms, pairs, starts)

	// What hindsight each arm installs, and the refusals that go with it.
	//
	// This path used to call neither. Its cells were stamped from sc.Oracles —
	// which folds OraclesFromEnv — while its means rows were stamped
	// Oracles{}.Stamp() unconditionally, so a run with FPL_ORACLE_PRICES exported
	// wrote a cells file saying "info:prices" and a means file saying "-" for the
	// same numbers. Two expressions of one quantity, disagreeing, in the one column
	// that exists to prevent exactly that.
	//
	// Routing through validateOracleArms is the other half. A stray environment
	// oracle here oracles *both* arms including the baseline, which is the case
	// TestAStrayEnvironmentOracleIsRefusedRatherThanInherited pins and which this
	// test would otherwise have measured as hindsight against hindsight.
	for i := range arms {
		arms[i].oracles = oraclesOf(cfg, starts[0], arms[i])
	}
	if err := validateOracleArms(arms); err != nil {
		t.Fatal(err)
	}
	printOracleBanner(arms)
	// Derived from what actually ran, never from a literal. Both arms carry the
	// same state — validateOracleArms has just refused the case where they do not.
	stamp := arms[0].oracles.Stamp()

	// variantIndex 0 is the shipped arm, which is what R pairs against.
	run := func(minutesWeight float64, variant string, variantIndex int) map[string]layerTotals {
		out := map[string]layerTotals{}
		for _, pair := range pairs {
			prior, cur := pair.Prior, pair.Cur
			for _, start := range starts {
				sc := sweepConfig(cfg, start, true)
				sc.Weights.MinutesWeight = minutesWeight
				// The same check runPolicySweep makes: an arm whose hindsight
				// varied by cell would make the per-arm stamp — and the means
				// rows built from it — a lie.
				if sc.Oracles != arms[variantIndex].oracles {
					t.Fatalf("arm %q installs %s at %s@%d and %s at the probe: an "+
						"arm's hindsight must not depend on the cell",
						variant, sc.Oracles.Stamp(), pair.Name, start,
						arms[variantIndex].oracles.Stamp())
				}

				// Identity first, so a cell that cannot be simulated is still a
				// flagged row rather than a hole nobody can see. This used to
				// t.Fatal, which threw away hours of completed replay over one
				// arm that could not field a legal fifteen — and a variant being
				// infeasible is a result about the variant, which is exactly the
				// convention runPolicySweep already follows.
				row := cellRow{
					Sweep: sweep, RunID: sink.run(), Variant: variant,
					VariantIndex: variantIndex, IsBaseline: variantIndex == 0,
					Season: pair.Name, PriorSeason: pair.PriorName,
					StartGW: start, BankUpTo: sc.BankUpTo,
				}.under(sc.Oracles)
				res, err := Simulate(cur, prior, sc)
				if err != nil {
					fmt.Printf("  infeasible: %s @%d %s: %v\n",
						pair.Name, start, variant, err)
					sink.cell(row.asInfeasible())
					continue
				}
				base, capt, weekly, autosub := layeredWeekly(cur, prior, sc, res.OpeningSquad)
				weeks := float64(len(base))
				if weeks == 0 {
					// Unreachable today — layeredWeekly runs startGW()..38 and
					// startGW clamps to [1,38], so weeks >= 1 always. Flagged
					// rather than skipped anyway, because the reason it is
					// unreachable lives in another function.
					sink.cell(row.asInfeasible())
					continue
				}
				out[fmt.Sprintf("%s@%d", pair.Name, start)] = layerTotals{
					base:    float64(sumInts(base)) / weeks,
					capt:    float64(sumInts(capt)) / weeks,
					weekly:  float64(sumInts(weekly)) / weeks,
					autosub: float64(sumInts(autosub)) / weeks,
					policy:  float64(res.Points) / weeks,
				}
				// The autosub layer *is* HOLD and the transfer layer *is*
				// POLICY, so only the three intermediate layers need their own
				// columns. Emitting the raw totals as well as the per-gameweek
				// figures lets R re-derive the normalisation and check it.
				row.Weeks = int(weeks)
				row.PolicyPoints, row.HoldPoints = res.Points, sumInts(autosub)
				row.Moves, row.Hits = res.Transfers, res.Hits
				row.HasLayers = true
				row.Frozen, row.FrozenCaptain, row.Weekly =
					sumInts(base), sumInts(capt), sumInts(weekly)
				sink.cell(row)
			}
		}
		return out
	}

	shipped = run(cfg.Weights.MinutesWeight, shippedLabel, 0)
	nudged = run(nudgeTo, nudgedLabel, 1)

	fmt.Printf("\nLayered decomposition, shipped MinutesWeight %.2f, %s.\n",
		cfg.Weights.MinutesWeight, gridLabel(len(pairs), len(starts)))
	fmt.Printf("Points are per gameweek played.\n\n")
	fmt.Printf("=== A. Static totals — where a season's points come from ===\n\n")
	fmt.Printf("%-28s %10s %10s\n", "layer", "mean pts/gw", "cumulative +")

	var bBase, bCapt, bWeekly, bAuto, bPolicy []float64
	for _, v := range shipped {
		bBase = append(bBase, v.base)
		bCapt = append(bCapt, v.capt)
		bWeekly = append(bWeekly, v.weekly)
		bAuto = append(bAuto, v.autosub)
		bPolicy = append(bPolicy, v.policy)
	}
	mBase, mCapt, mWeekly, mAuto, mPolicy := meanOf(bBase), meanOf(bCapt), meanOf(bWeekly), meanOf(bAuto), meanOf(bPolicy)
	fmt.Printf("%-28s %10.3f %10s\n", "frozen XI, no captain", mBase, "-")
	fmt.Printf("%-28s %10.3f %+10.3f\n", "+ captain (still frozen)", mCapt, mCapt-mBase)
	fmt.Printf("%-28s %10.3f %+10.3f\n", "+ weekly XI & captaincy", mWeekly, mWeekly-mCapt)
	fmt.Printf("%-28s %10.3f %+10.3f\n", "+ autosubs (= HOLD)", mAuto, mAuto-mWeekly)
	fmt.Printf("%-28s %10.3f %+10.3f\n", "+ transfers (= POLICY)", mPolicy, mPolicy-mAuto)
	fmt.Printf("\nEach '+' row is that mechanism's average contribution, holding everything before\n")
	fmt.Printf("it fixed. Multiply by ~38 for a season-scale figure.\n")

	fmt.Printf("\n=== B. Sensitivity — how much each layer moves under a nudge ===\n\n")
	fmt.Printf("Nudge: MinutesWeight %.2f -> %.2f. Already characterised on HOLD alone:\n", cfg.Weights.MinutesWeight, nudgeTo)
	// A RECORDED figure, and its "24 cells" is deliberately a literal: it was
	// measured when the default grid was four seasons, and deriving it would
	// relabel a past measurement with the grid running now. It said "this
	// session", which read as the run in front of you and is exactly the
	// confusion the derived label above removes.
	fmt.Printf("-0.717 pts/gw, t = -2.06 (measured at 24 cells, the four-season grid).\n\n")

	// Means only. Every SE, df and p-value for this table now comes from
	// stats/sweep_inference.R, reading the cells written above.
	extract := func(get func(layerTotals) float64) (mean float64, n int) {
		var diffs []float64
		for key, base := range shipped {
			nud, ok := nudged[key]
			if !ok {
				continue
			}
			diffs = append(diffs, get(nud)-get(base))
		}
		return meanOf(diffs), len(diffs)
	}

	fmt.Printf("%-28s %9s %7s\n", "layer", "mean/gw", "n")
	// metric names match the CSV columns R reads, so its reproduction check
	// covers the layers and marginals too rather than only policy and hold.
	report := func(label, metric string, get func(layerTotals) float64) {
		m, n := extract(get)
		fmt.Printf("%-28s %+9.3f %7d\n", label, m, n)
		sink.mean(sweep, metric, nudgedLabel, shippedLabel, 1, m, n, stamp)
	}
	report("frozen XI, no captain", "frozen", func(v layerTotals) float64 { return v.base })
	report("+ captain (frozen XI)", "frozen_captain", func(v layerTotals) float64 { return v.capt })
	report("+ weekly XI & captaincy", "weekly", func(v layerTotals) float64 { return v.weekly })
	report("+ autosubs (= HOLD)", "hold", func(v layerTotals) float64 { return v.autosub })
	report("+ transfers (= POLICY)", "policy", func(v layerTotals) float64 { return v.policy })

	fmt.Printf("\nAlso the marginal (this layer minus the previous one), which is where\n")
	fmt.Printf("each mechanism's OWN share of the nudge's sensitivity lives:\n\n")
	fmt.Printf("%-28s %9s %7s\n", "marginal layer", "mean/gw", "n")
	marginal := func(label, metric string, hi, lo func(layerTotals) float64) {
		// Differenced **per cell**, not by combining two layers' standard
		// errors. The old version computed the marginal SE as the root-sum-square
		// of two adjacent cumulative SEs, which assumes the layers are
		// independent — and adjacent layers differ by one mechanism on the *same*
		// 24 cells and the same weeks, so they are almost perfectly correlated.
		// The test documented that as invalid and printed it anyway.
		//
		// The fix is not a better approximation. A marginal is itself a per-cell
		// quantity — (hi - lo) within a cell, then nudged minus shipped — so
		// there is nothing to approximate: it goes through exactly the same
		// paired machinery as any other metric, and R computes its SE from the
		// derived columns.
		m, n := extract(func(v layerTotals) float64 { return hi(v) - lo(v) })
		fmt.Printf("%-28s %+9.3f %7d\n", label, m, n)
		sink.mean(sweep, metric, nudgedLabel, shippedLabel, 1, m, n, stamp)
	}
	marginal("captaincy alone", "m_captain",
		func(v layerTotals) float64 { return v.capt }, func(v layerTotals) float64 { return v.base })
	marginal("weekly XI repick alone", "m_weekly_xi",
		func(v layerTotals) float64 { return v.weekly }, func(v layerTotals) float64 { return v.capt })
	marginal("autosubs alone", "m_autosub",
		func(v layerTotals) float64 { return v.autosub }, func(v layerTotals) float64 { return v.weekly })
	marginal("transfers alone", "m_transfers",
		func(v layerTotals) float64 { return v.policy }, func(v layerTotals) float64 { return v.autosub })

	fmt.Printf("\nA mechanism whose own marginal is near zero is not where this nudge's\n")
	fmt.Printf("sensitivity lives, regardless of how large its static contribution in\n")
	fmt.Printf("section A is. For SEs, df and multiplicity-adjusted p-values:\n")
	if sink == nil {
		fmt.Printf("re-run with FPL_CELLS=/tmp/cells.csv, then\n")
	}
	fmt.Printf("  Rscript stats/sweep_inference.R <the FPL_CELLS path>\n")
}
