package backtest

// Does the clean sheet over-predict against the regressor the MODEL actually
// scores on, rather than against realised match xGC?
//
//	DIAG=1 FPL_CSREG_ROWS=/tmp/csreg.csv \
//	  scripts/replay -run TestDiagCleanSheetRegressor -v -timeout 2h
//
// # Why this exists, and why it blocks the 2x2
//
// `TestDiagCleanSheetPoisson` and `stats/cs_calibration.R` fit -ln p against
// **realised single-match xGC** and return P = 0.9046 exp(-1.1731x). The model
// does not score on realised match xGC. It scores on `m.XGC90` — a per-player
// per-90 rate, blended toward a prior season, shrunk, and then multiplied by a
// fixture multiplier. Those are different regressors with different variances,
// and a Poisson exponent is convex, so the aggregate over-prediction differs
// between them by construction.
//
// `cs_calibration.R`'s own header spots the risk and resolves it by ASSERTION,
// in the direction that favours its fit: "the model's own exponent is ... far
// smoother than realised match xGC. So a slope above 1 measured here is if
// anything an understatement of what the model's own regressor wants. That is a
// mechanism argument, not a measurement." This is the measurement.
//
// The standing rules it serves are two: **a constant fitted against a proxy for
// its input is fitted to the proxy's noise too**, and **check what a multiplier
// multiplies before calibrating it**.
//
// # The pairing, and why it is point-in-time
//
// One row per (season, gameweek, team): the model's `XGC90` for that club's
// representative defender, computed from an engine built at the PREVIOUS
// gameweek, against whether the club actually kept a clean sheet in this one.
// That is the decision the model faces — it prices a clean sheet it has not seen
// — and it is the only pairing that cannot leak. `PointInTime` strips scores and
// the Finished flag after the cutoff, and `TestPointInTimeHidesFutureResults`
// pins it.
//
// The representative is the DEF or GKP with the most minutes to the cutoff, ties
// broken by element id. One row per team-match, not per player: eleven
// team-mates share one clean sheet, and counting them separately multiplies the
// apparent sample by eleven while adding no information. Players and gameweeks
// are visited in sorted order because Go randomises map iteration and this file
// emits a per-observation dump — the sibling calibration returned figures
// differing by 0.7% between identical runs before that was fixed.
//
// # Guards, each of which the siblings pay for
//
//   - `Fixtures != 1` is dropped. A doubled row carries xGC summed over both
//     matches while CleanSheets > 0 still reads "at least one", so comparing them
//     asks P(zero across BOTH) against P(one in EITHER) — the intersection
//     against the union. That artefact is what made a "wrong family" refutation
//     wrong, and it is the guard this file's own ancestor lacked.
//   - Minutes >= 90, so the representative saw the whole match his club's clean
//     sheet is recorded against.
//   - XGC90 > 0, which is also the gate `baseXP90` itself applies — a player
//     scored with no clean-sheet term at all is not evidence about the term.
//   - Cutoff >= 6, so the blend has some current-season evidence. Stated as a
//     constant rather than buried: at cutoff 1 the regressor is almost entirely
//     last season's, which is a different quantity.
//
// # The fixture path — added 2026-08-15, and PRE-REGISTERED here before it ran
//
// The neutral regressor is `XGC90`. The fixture-sensitive path evaluates
// `XGC90 x def` (`fixtureSensitiveAt`), so a club facing a hard fixture is scored
// on a larger exponent. That is the only path on which `FPL_CS_XGC_FACTOR` could
// still be live after the neutral refit found nothing to correct, so it is worth
// its own dump — written to FPL_CSREG_DEF_ROWS, with `xgc` holding the PRODUCT so
// stats/cs_calibration.R fits it unchanged.
//
//   - **P1. It is a BOUND, not a calibration, whatever it returns.** `def` is a
//     MODELLED quantity — FPL's difficulty rank times this project's own band
//     adjustment — so this fits one part of the model against another. A slope
//     away from 1 here does not localise the error to the clean sheet: it could
//     as easily be the difficulty ladder. **No arm of this can license moving
//     `FPL_CS_XGC_FACTOR` on its own.**
//   - **P2. Expected close to the neutral result, and that is the null.** `def`
//     is a narrow multiplier (roughly 0.7 to 1.4) applied to a regressor whose
//     own spread is wider, so the product's dispersion is not much larger than
//     `XGC90`'s. If b moves a long way from the neutral 0.9922, suspect the
//     difficulty ladder rather than the clean sheet.
//   - **P3. The direction that would be interesting.** `b > 1` on the product
//     while `b ~ 1` on the neutral regressor would say the model under-reacts to
//     fixture difficulty *inside the clean sheet specifically* — which is the one
//     reading that would reopen the factor. The record's prior is against it:
//     "the model's response over a five-game horizon is about right; it is only
//     wrong about a single match".
//   - **P4. Population.** Same guards as the neutral arm, plus: the fixture the
//     engine actually pointed at must be the one that was played. A club whose
//     next fixture is not gameweek `cut+1` — a blank — is skipped, not defaulted
//     to `def = 1`, because defaulting would silently mix the two regressors.
//
// The `cf` factor is carried in both dumps as its own column rather than folded
// into `xgc`, so the two can be fitted separately.
//
// # What this cannot answer
//
// It reports TWO strata and the headline is the strict one. The reconstructed
// -xGC seasons carry 16-20% club-match error, which enters this fit as
// errors-in-variables on the regressor — the exact bias being measured — so
// pooling them would confound the finding with its own mechanism. They are
// printed as context because dropping them silently would hide whether the two
// agree, and the CSV carries a season column so either can be refitted.

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// csRegressorMinCut is the first cutoff whose blend carries meaningful
// current-season evidence. Below it the regressor is essentially last season's
// rate, which is a different quantity from the one the model uses in anger.
const csRegressorMinCut = 6

// nativeXGCSeasons are the seasons whose weekly expected_goals_conceded is FPL's
// own rather than reconstructed. Named as an enumeration rather than derived
// from a cutoff date because the archive event is a fact about FPL adding a
// field, and 2022-23 is mixed — its GW1-15 are reconstructed — so it is excluded
// from the clean stratum even though it carries native data for most of the
// season. This is the same population stats/cs_calibration.R fits on, minus
// 2022-23, so the two are comparable and this one is the stricter.
var nativeXGCSeasons = map[string]bool{
	"2023-24": true, "2024-25": true, "2025-26": true,
}

func TestDiagCleanSheetRegressor(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	ctx := context.Background()
	cfg := loadConfig(t)

	rows := newRowDump(os.Getenv("FPL_CSREG_ROWS"), t.Fatalf,
		// ⚠️ The regressor column is named `xgc` and NOT `xgc90`, deliberately:
		// stats/cs_calibration.R's input contract requires that name, and the
		// binomial GLM with a log link it runs is exactly the fit wanted here.
		// Forking a second R script to rename one column would be a second
		// implementation of one fit.
		//
		// The cost is real and is the reason for this comment: two dumps now
		// share a schema and mean DIFFERENT things — the sibling's `xgc` is
		// realised single-match xGC, this one's is the model's blended,
		// shrunk, point-in-time `XGC90`. **The whole finding is the difference
		// between them**, so never pool the two files, and name which dump a
		// figure came from. Read "check which FILE a number came from" one
		// level down: right schema, wrong quantity.
		"season", "gw", "team", "xgc", "clean_sheet", "element", "cf")
	defRows := newRowDump(os.Getenv("FPL_CSREG_DEF_ROWS"), t.Fatalf,
		// `xgc` here is the PRODUCT XGC90 x def — the fixture path's exponent —
		// so cs_calibration.R fits it unchanged. The neutral value is kept
		// beside it as `xgc90` so the two are reconcilable from one file.
		"season", "gw", "team", "xgc", "clean_sheet", "element", "cf", "def", "xgc90")
	defer rows.close()
	defer defRows.close()

	type bucket struct {
		lo, hi    float64
		n         float64
		pred, act float64
		sumX      float64
	}
	newBuckets := func() []*bucket {
		return []*bucket{
			{lo: 0.0, hi: 1.0}, {lo: 1.0, hi: 1.3}, {lo: 1.3, hi: 1.6}, {lo: 1.6, hi: 99},
		}
	}
	// Two strata, reported side by side rather than one filtered population.
	//
	// A reconstructed regressor carries 16-20% club-match error, which enters
	// this fit as errors-in-variables ON THE REGRESSOR — which is the exact bias
	// being measured, so pooling the two would confound the finding with its own
	// mechanism. Filtering them out instead would throw away half the grid and
	// hide whether the two agree. Both are printed; the NATIVE table is the
	// headline and the pooled one is context.
	buckets := newBuckets()
	nativeBuckets := newBuckets()
	var n, pred, act, sumX float64
	var nn, npred, nact, nsumX float64

	// # The two unsized biases, sized here rather than disclosed and left
	//
	// Both were recorded as running TOWARD this diagnostic's own conclusion,
	// which is the direction that most needs a number rather than a caveat.
	//
	//  1. SELECTION. The 90-minute guard drops a fifth of team-matches, and the
	//     dropped set is enriched in red cards, injuries and chasing the game —
	//     plausibly worse defensive matches. If so it raises `actual` among the
	//     kept and shrinks the apparent over-prediction. Sized by comparing the
	//     clean-sheet rate of kept against dropped, on a FIXTURE-derived outcome
	//     (the opponent's score) rather than the representative's own
	//     `CleanSheets`, because FPL awards that only at sixty minutes — so on
	//     exactly the dropped rows the player-level field is unusable and would
	//     have manufactured the comparison it is meant to test.
	//  2. THE COUPLING. The engine evaluates cleanSheetProb(xgc, def, cf); this
	//     diagnostic's `pred` omits `cf` (defconCleanFactor), which is 1 for
	//     keepers but not for a defender with a defensive-contribution rate.
	//     Sized by carrying a second predicted total with `cf` applied.
	var keptCS, keptN, dropCS, dropN float64
	var keptPred, dropPred float64
	var dn, dpred, dact, dsumDef float64 // the fixture path
	var predCF float64                   // kept rows, with defconCleanFactor applied
	var cfRows float64                   // how many of them the coupling actually moves

	// Bound once so the printed population below names the seasons this run actually
	// walked. sweepPairNames is six by default and FPL_SWEEP_SEASONS moves it, so a
	// literal here would read "all six seasons" over a four-season run.
	csPairs := sweepPairNames()
	for _, pair := range csPairs {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatal(err)
		}
		idx := newPriorIndex(prior)

		for cut := csRegressorMinCut; cut <= 37; cut++ {
			gw := cut + 1

			boot, fx := PointInTime(cur, prior, cut)
			e := analysis.NewEngineFull(boot, fx, cfg.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = idx
			e.Recent = newRecentIndexWith(cur, cut,
				cfg.Weights.MinutesHalfLife, cfg.Weights.RateHalfLife)

			// Minutes to the cutoff decide each club's representative. Counted
			// here rather than read off a metric so the choice cannot depend on
			// anything the engine smooths.
			type cand struct {
				mins  int
				id    int
				xgc90 float64
				cf    float64 // defconCleanFactor, for the coupling sizing
			}
			best := map[int]*cand{}

			// The fixture-derived clean sheet for every club playing exactly one
			// match this gameweek. Independent of who played, which is the whole
			// point: it is the only outcome the dropped rows can carry.
			played := map[int]int{}
			teamCS := map[int]bool{}
			for _, f := range cur.Fixtures {
				if f.Event == nil || *f.Event != gw {
					continue
				}
				if f.TeamHScore == nil || f.TeamAScore == nil {
					continue // not played, so it is not evidence either way
				}
				played[f.TeamH]++
				played[f.TeamA]++
				teamCS[f.TeamH] = *f.TeamAScore == 0
				teamCS[f.TeamA] = *f.TeamHScore == 0
			}

			for i := range boot.Elements {
				el := &boot.Elements[i]
				if el.ElementType > 2 {
					continue // only defenders and keepers collect a clean sheet
				}
				p := cur.Players[el.ID]
				if p == nil {
					continue
				}
				mins := 0
				for g := 1; g <= cut; g++ {
					if r, ok := p.GWs[g]; ok {
						mins += r.Minutes
					}
				}
				if mins == 0 {
					continue
				}
				m := e.Metrics(el)
				if m.XGC90 <= 0 {
					continue // the gate baseXP90 itself applies
				}
				c := &cand{mins: mins, id: el.ID, xgc90: m.XGC90,
					cf: e.DefconCleanFactorFor(el.ElementType, m.DefCon90)}
				if b, ok := best[el.Team]; !ok || c.mins > b.mins ||
					(c.mins == b.mins && c.id < b.id) {
					best[el.Team] = c
				}
			}

			teams := make([]int, 0, len(best))
			for id := range best {
				teams = append(teams, id)
			}
			sort.Ints(teams)

			for _, team := range teams {
				c := best[team]
				p := cur.Players[c.id]
				if p == nil {
					continue
				}
				g, ok := p.GWs[gw]
				if !ok {
					continue
				}
				if g.Fixtures != 1 {
					continue // intersection-against-union artefact; see the header
				}

				// SIZING 1, and it must be accumulated BEFORE the 90-minute
				// guard returns, because the dropped rows are the comparison.
				// The outcome here is fixture-derived, so it exists whether the
				// representative played 90 or 9.
				if teamCS0, ok := teamCS[team]; ok && played[team] == 1 {
					v := 0.0
					if teamCS0 {
						v = 1
					}
					// The predicted value is available for a dropped row too:
					// XGC90 is a pre-cutoff estimate and does not depend on
					// whether the representative went on to finish the match.
					// Carrying it is what turns a bias DIRECTION into a
					// corrected ratio.
					pr := math.Exp(-c.xgc90)
					if g.Minutes >= 90 {
						keptCS += v
						keptPred += pr
						keptN++
					} else {
						dropCS += v
						dropPred += pr
						dropN++
					}
				}

				if g.Minutes < 90 {
					continue
				}
				cs := 0.0
				if g.CleanSheets > 0 {
					cs = 1
				}

				// SIZING 2: what the engine would have predicted, coupling included.
				predCF += math.Exp(-c.xgc90 * c.cf)
				if c.cf != 1 {
					cfRows++
				}

				x := c.xgc90
				rows.write(pair[1], strconv.Itoa(gw), strconv.Itoa(team),
					strconv.FormatFloat(x, 'f', -1, 64),
					strconv.FormatFloat(cs, 'f', -1, 64),
					strconv.Itoa(c.id),
					strconv.FormatFloat(c.cf, 'f', -1, 64))

				// The fixture path. P4: the fixture the engine pointed at must be
				// the one that was played, so a club whose next fixture is not
				// this gameweek is SKIPPED rather than defaulted to def = 1 —
				// defaulting would silently mix the two regressors in one column.
				if fx := e.TeamFixtures(team, 1); len(fx) == 1 && fx[0].Event == gw {
					_, def := e.FixtureMultipliersFor(fx[0])
					xd := x * def
					defRows.write(pair[1], strconv.Itoa(gw), strconv.Itoa(team),
						strconv.FormatFloat(xd, 'f', -1, 64),
						strconv.FormatFloat(cs, 'f', -1, 64),
						strconv.Itoa(c.id),
						strconv.FormatFloat(c.cf, 'f', -1, 64),
						strconv.FormatFloat(def, 'f', -1, 64),
						strconv.FormatFloat(x, 'f', -1, 64))
					dn++
					dpred += math.Exp(-xd)
					dact += cs
					dsumDef += def
				}

				n++
				pred += math.Exp(-x)
				act += cs
				sumX += x
				for _, b := range buckets {
					if x >= b.lo && x < b.hi {
						b.n++
						b.pred += math.Exp(-x)
						b.act += cs
						b.sumX += x
					}
				}
				if nativeXGCSeasons[pair[1]] {
					nn++
					npred += math.Exp(-x)
					nact += cs
					nsumX += x
					for _, b := range nativeBuckets {
						if x >= b.lo && x < b.hi {
							b.n++
							b.pred += math.Exp(-x)
							b.act += cs
							b.sumX += x
						}
					}
				}
			}
		}
	}

	if n == 0 {
		t.Fatal("no observations: the population is empty, which is a bug in the guards rather than a result")
	}

	table := func(title string, bs []*bucket, tn, tpred, tact, tsumX float64) {
		fmt.Printf("\n%s\n", title)
		fmt.Printf("%-12s %7s %11s %9s %9s %9s %11s\n",
			"XGC90", "n", "predicted", "actual", "ratio", "mean x", "needed mult")
		for _, b := range bs {
			if b.n == 0 {
				continue
			}
			ratio := 0.0
			if b.act > 0 {
				ratio = b.pred / b.act
			}
			needed := 0.0
			if b.pred > 0 {
				needed = b.act / b.pred
			}
			fmt.Printf("%-12s %7.0f %11.4f %9.4f %9.3f %9.3f %11.3f\n",
				fmt.Sprintf("%.1f-%.1f", b.lo, b.hi), b.n, b.pred/b.n, b.act/b.n,
				ratio, b.sumX/b.n, needed)
		}
		if tn == 0 || tact == 0 || tpred == 0 {
			return
		}
		fmt.Printf("%-12s %7.0f %11.4f %9.4f %9.3f %9.3f %11.3f\n",
			"ALL", tn, tpred/tn, tact/tn, tpred/tact, tsumX/tn, tact/tpred)
	}

	fmt.Printf("\nClean sheets against the MODEL'S OWN regressor (XGC90, point-in-time).\n")
	fmt.Printf("One row per team-gameweek; predicted at cut, realised at cut+1.\n")
	fmt.Printf("Single-fixture gameweeks, representative played 90.\n")
	table("NATIVE xGC ONLY (2023-24, 2024-25, 2025-26) — the headline.",
		nativeBuckets, nn, npred, nact, nsumX)
	table("POOLED, including reconstructed-xGC seasons — CONTEXT, not the headline.\n"+
		"A reconstructed regressor carries 16-20% club-match error, which is\n"+
		"errors-in-variables on the very quantity being calibrated.",
		buckets, n, pred, act, sumX)

	fmt.Printf("\n--- THE FIXTURE PATH: XGC90 x def, what fixtureSensitiveAt scores ---\n")
	if dn > 0 {
		fmt.Printf("  n %.0f of %.0f neutral rows (the rest blank in this gameweek)\n", dn, n)
		fmt.Printf("  mean def multiplier            %.4f\n", dsumDef/dn)
		fmt.Printf("  predicted %.4f   actual %.4f   ratio %.4f\n",
			dpred/dn, dact/dn, dpred/dact)
		fmt.Printf("  the neutral arm on the same question: ratio %.4f\n", pred/act)
		fmt.Printf("\n  ⚠️ P1: this is a BOUND, not a calibration. `def` is MODELLED — FPL's\n")
		fmt.Printf("  difficulty rank times this project's band adjustment — so a slope away\n")
		fmt.Printf("  from 1 here does not localise the error to the clean sheet. It could as\n")
		fmt.Printf("  easily be the difficulty ladder. Fit the dump with cs_calibration.R;\n")
		fmt.Printf("  Go prints no verdict.\n")
	} else {
		fmt.Printf("  no rows: every club blanked, or TeamFixtures returned nothing.\n")
		fmt.Printf("  That is a bug in the guards rather than a result.\n")
	}

	fmt.Printf("\n--- SIZING 1: what the 90-minute guard selects on ---\n")
	fmt.Printf("Clean-sheet rate from the FIXTURE (opponent scored 0), so it exists for\n")
	fmt.Printf("dropped rows too. Single-fixture club-gameweeks only, all %s.\n\n", seasonsLabel(len(csPairs)))
	if keptN > 0 && dropN > 0 {
		kr, dr := keptCS/keptN, dropCS/dropN
		fmt.Printf("  kept (rep played 90)      n %6.0f   clean-sheet rate %.4f\n", keptN, kr)
		fmt.Printf("  dropped (rep under 90)    n %6.0f   clean-sheet rate %.4f\n", dropN, dr)
		fmt.Printf("  dropped share            %.1f%%\n", 100*dropN/(keptN+dropN))
		fmt.Printf("  kept minus dropped       %+.4f\n", kr-dr)
		fmt.Printf("\n  A POSITIVE gap is the feared direction: the guard keeps the better\n")
		fmt.Printf("  defensive matches, raising `actual` among the kept and SHRINKING the\n")
		fmt.Printf("  apparent over-prediction. The corrected figure, which is the point of\n")
		fmt.Printf("  carrying `pred` on the dropped rows too:\n\n")
		fmt.Printf("    ratio on KEPT only (what the fit reports)  %.4f\n", keptPred/keptCS)
		fmt.Printf("    ratio on KEPT + DROPPED (unselected)       %.4f\n",
			(keptPred+dropPred)/(keptCS+dropCS))
		fmt.Printf("\n  The second is the honest one for a LEVEL claim, and it is the larger.\n")
		fmt.Printf("  ⚠️ It is still not the population the model scores: it conditions on a\n")
		fmt.Printf("  club having a most-played defender or keeper with a positive XGC90 at\n")
		fmt.Printf("  all, and it prices every club by that one player. It removes the\n")
		fmt.Printf("  90-minute selection and nothing else.\n")
	} else {
		fmt.Printf("  no comparison available: kept %.0f dropped %.0f\n", keptN, dropN)
	}

	fmt.Printf("\n--- SIZING 2: the defcon/clean-sheet coupling this dump omits ---\n")
	if n > 0 {
		fmt.Printf("  predicted, coupling OMITTED (what the fit used)  %.4f\n", pred/n)
		fmt.Printf("  predicted, coupling APPLIED (what the engine does) %.4f\n", predCF/n)
		fmt.Printf("  rows the coupling moves at all: %.0f of %.0f (%.1f%%)\n",
			cfRows, n, 100*cfRows/n)
		fmt.Printf("  ratio against actual, coupling applied: %.4f (omitted: %.4f)\n",
			predCF/act, pred/act)
		fmt.Printf("\n  ⚠️ Read the DIRECTION off these numbers, not off a mechanism story. A\n")
		fmt.Printf("  first version of this block asserted the coupling 'can only LOWER a\n")
		fmt.Printf("  predicted clean sheet'; the aggregate says otherwise, because the\n")
		fmt.Printf("  factor sits BELOW 1 for most defenders it touches and exp(-x*cf) then\n")
		fmt.Printf("  rises. The omission does run toward this file's conclusion — applying\n")
		fmt.Printf("  the coupling moves the ratio further from 1, not closer — but for the\n")
		fmt.Printf("  opposite mechanical reason to the one asserted.\n")
	}

	fmt.Printf("\nRead the `needed mult` column as the shape question: a CONSTANT column\n")
	fmt.Printf("wants a flat scale, a FALLING column wants the factor, and a RISING\n")
	fmt.Printf("column says the factor's sign is wrong. The fit itself is R's job —\n")
	fmt.Printf("FPL_CSREG_ROWS then stats/cs_calibration.R. Go prints no verdict.\n")
}
