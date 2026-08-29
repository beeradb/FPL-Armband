package backtest

// Where does TestDiagAttackDefenceCoherence's pooled fixture-blind lean
// actually come from?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagAttackDefenceLeanDecomposition -v -timeout 60m
//
// # What this gates
//
// TestDiagAttackDefenceCoherence (coherence_diag_test.go) reports a pooled
// fixture-blind mean signed log divergence between the attack side's
// fixture-blind expected goals and the defence side's fixture-blind implied
// goals conceded — its `blindD` distStats' `meanD` field, currently about
// +0.0707 in log units ("attack predicts about 7% more than defence implies
// conceded", pooled and unweighted across six seasons). That number is a
// single aggregate over 2350 directed fixture observations built from five
// different model quantities per club (XG90, XGScale, ExpectedMinutes,
// XGC90, the defensive-contribution clean factor `cf`). A reader who wants
// to know WHERE +0.0707 comes from — is it a calibration ratio, a minutes
// artefact, or the raw rate gap? — has no way to ask the coherence
// diagnostic that question, because it never separates those five
// quantities' contributions. This diagnostic answers it, by an EXACT
// algebraic identity, not a second estimate.
//
// # The identity, and why it is exact
//
// Every club's fixture-blind attacking rate and fixture-blind implied
// concession rate — `cohClubModel.rate()` and `cohClubModel.defRate()`,
// exactly as coherence_diag_test.go already computes them — factor with NO
// approximation into four named ratios. Writing `emSum` for the club's total
// registered ExpectedMinutes (EM) at the cutoff — the same minutes evidence
// that feeds both the attack and the defence side, since the archive has
// one roster, not two:
//
//	rate    = S x C x 11 x mwmA
//	defRate = K x mwmB
//
//	mwmA = (sum XG90 x EM)          / emSum         EM-weighted mean XG90
//	S    = (sum XG90 x XGScale x EM) / (sum XG90 x EM) XG90.EM-weighted mean XGScale
//	C    = emSum / 990                               minutes coverage (990 = 11 x 90)
//	mwmB = (sum XGC90 x EM)          / emSum         EM-weighted mean XGC90
//	K    = (sum XGC90 x cf x EM)     / (sum XGC90 x EM) net cf effect
//
// This is not approximate, and not fitted: substitute the definitions and
// every sum but one cancels.
//
//	S x C x 11 x mwmA
//	  = (sum XG90.XGScale.EM / sum XG90.EM) x (emSum/990) x 11 x (sum XG90.EM / emSum)
//	  = (sum XG90.XGScale.EM) x 11 / 990                      -- both "sum XG90.EM" cancel,
//	  = (sum XG90.XGScale.EM) / 90                            --    and C.11 = emSum/90
//	  = rate                                                  -- cohClubModel.rate()'s own definition
//
//	K x mwmB = (sum XGC90.cf.EM / sum XGC90.EM) x (sum XGC90.EM / emSum)
//	         = (sum XGC90.cf.EM) / emSum = defRate            -- cohClubModel.defRate()'s own definition
//
// Taking logs turns the two products into sums: `ln rate = lnS + lnC + ln11
// + ln(mwmA)`, `ln defRate = lnK + ln(mwmB)`. The coherence diagnostic's
// pooled fixture-blind `meanD` is, by its own definition,
// `mean_obs[ln(rate[attacker]) - ln(defRate[defender])]` — a mean of
// per-observation differences of exactly those two logs, over the identical
// directed fixture set (`cohObs`, one row per fixture per direction).
// Substituting and regrouping — again exact, since a mean of sums is the sum
// of the means:
//
//	meanD = lnS + lnC - lnK + G
//	  lnS = mean_obs[ ln S[attacker_obs]    ]
//	  lnC = mean_obs[ ln C[attacker_obs]    ]
//	  lnK = mean_obs[ ln K[defender_obs]    ]
//	  G   = ln11 + mean_obs[ln mwmA[attacker_obs]] - mean_obs[ln mwmB[defender_obs]]
//
// # Why "meanD" here means the RAW ln(rate)-ln(defRate) figure, not
// cohImpliedLambda's calibrated reading — read this before comparing numbers
//
// coherence_diag_test.go's actual `blindD` is computed through
// `cohImpliedLambda`, which is `factor x defRate x defMul - ln(scale)`, not
// bare `defRate`. That is a POISON for this decomposition whenever `scale !=
// 1`: `ln(factor x defRate - ln(scale))` does not split into `ln(factor) +
// ln(defRate)` the way `ln(factor x defRate)` would, because of the
// subtraction inside the log. So this diagnostic's identity targets the RAW
// quantity `mean_obs[ln(rate[attacker]) - ln(defRate[defender])]` directly —
// always exact, because it never involves `factor` or `scale` at all — and
// separately prints coherence_diag_test.go's actual `blindD.meanD` (computed
// through `cohImpliedLambda` with the run's real `factor`/`scale`) beside it
// for cross-checking. **At the shipped defaults, `cleanSheetXGCFactor` =
// `cleanSheetScale` = 1.0 (`analysis.CleanSheetState()`, printed below), so
// `factor x defRate - ln(scale)` reduces to exactly `defRate`** and the two
// readings are numerically identical — the same provenance-guard fact
// coherence_diag_test.go's own header already states about its
// with-calibration and structural readings, restated here because it is the
// reason this decomposition's target and the coherence diagnostic's printed
// `blindD.meanD` are the same number in THIS run and would not necessarily
// stay so under `FPL_CS_XGC_FACTOR` / `FPL_CS_SCALE`. `cf` is untouched by
// either constant and appears ONLY inside K — this diagnostic does not fold
// `cleanSheetXGCFactor`/`cleanSheetScale` into K or G, and the lean itself
// stays measured against the raw `XGC90 x cf` quantity throughout.
//
// The secondary "adjusted" reading — the run's actual calibration constants
// AND actual fixture multipliers, i.e. coherence_diag_test.go's `withD` — is
// printed as a plain comparison number and is NOT decomposed further. Both
// the fixture-multiplier asymmetry (two ladders, different gains, keyed on
// two different difficulty integers) and the calibration-constant asymmetry
// are already documented at length in coherence_diag_test.go's header; nothing
// here adds to that account, and decomposing a quantity that is not a clean
// product of S/C/K/G/mwmA/mwmB would require inventing a fifth wedge with no
// closed form, exactly the kind of unprincipled cut this file's identity
// otherwise avoids.
//
// # Why this decomposition does not fall into the wedge trap
//
// AGENTS.md's standing rule: "Do not quote the parts of a decomposition
// that telescopes. Quote the product; never the wedges." That rule exists
// because a decomposition can be constructed so that one wedge is a FITTED
// free parameter solved to match a single moment — the clean-sheet factor
// `f` is the worked example, and quoting `f` alone credits a mechanism with
// something that is actually the fit closing a gap by construction.
//
// None of S, C, K, G, mwmA or mwmB is fitted to anything. Every one is a
// directly computed weighted mean or ratio of quantities the model has
// ALREADY measured before this diagnostic runs — XG90, XGScale,
// ExpectedMinutes, XGC90, cf — with no free parameter solved against the
// divergence this diagnostic reports or against any other single moment.
// `XGScale` itself is fitted, but it is fitted by `calibrateExpectedStats`
// against the league's own realised goals, once, for a completely different
// purpose than reproducing this lean — S is a re-aggregation of an existing
// fitted quantity, not a new fit introduced to make the identity close.
// Because every wedge is a plain computed statistic and the identity is
// linear-algebraic (sums of logs of an exact product), there is no
// alternative "cut" of `rate = S x C x 11 x mwmA` that reproduces the same
// product with different wedge values — the split is canonical, not chosen
// after the fact to flatter a story. That is the operational difference
// from the clean-sheet case this rule warns about, and it is why quoting
// lnS, lnC, lnK and G individually below is safe where quoting a fitted `f`
// alone would not be.
//
// # Position on double gameweeks
//
// This diagnostic reuses `cohObs` and `cohSeasonBuild` unchanged, so it
// inherits coherence_diag_test.go's documented position on doubles verbatim
// — see that file's header, "Position on double gameweeks, stated
// explicitly": the coherence comparison (and therefore this decomposition,
// which reads the identical `rate`/`defRate` pair per fixture) is built per
// FIXTURE, one row per direction, with no team-gameweek averaging; only the
// realised-goals secondary reading (which this file does not use at all)
// shares one team-gameweek figure across a double's two fixtures.
//
// # What would falsify this
//
// The identity residual — `meanD_direct - (lnS + lnC - lnK + G)` — not
// closing to float tolerance, per season and pooled. That residual is
// printed beside every row below and the test FAILS if it exceeds
// tolerance; it is the diagnostic's own correctness gate, not a warning a
// reader could miss. Tolerance is 1e-9: each observation's contribution to
// the identity involves on the order of ten float64 multiplications,
// divisions and `math.Log` calls, each carrying relative error on the order
// of machine epsilon (~2.2e-16); accumulated over the pooled set (order
// 10^3 observations) via two independently-coded paths — the direct
// `ln(rate)-ln(defRate)` computation and the four-term `lnS+lnC-lnK+G` sum —
// worst-case error compounds to something comfortably under 1e-9, while
// 1e-9 is still far below anything that could be mistaken for a genuine
// non-closure (a real algebra bug moves this by orders of magnitude, not by
// a few times machine epsilon).
//
// # What this changes
//
// Nothing. This is arithmetic on numbers TestDiagAttackDefenceCoherence
// already computes — no scoring term moves, no config changes, no new
// threshold. Sanity-check only, stated rather than assumed: prior work
// separately measured `C = modelMinutes/990` averaging close to 0.99 and `S`
// (a pooled mean XGScale-like ratio) averaging close to 0.98
// (`TestDiagTeamGoalShare`, "expected minutes per club per match" — the
// SAME `modelMinutes/990` quantity `C` is here, just attached to the lean
// decomposition rather than measured standalone). This diagnostic reports
// what it actually measures whether or not that expectation holds; a
// divergence is a prompt to suspect THIS implementation first (per its own
// falsification test below), not license to adjust the printed number
// toward the expectation.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// cohS, cohC, cohMwmA, cohK, cohMwmB read the four log-decomposable factors
// and the two EM-weighted means off a cohClubModel — see this file's header
// for the algebra and coherence_diag_test.go's cohClubModel doc comment for
// the field names. Each is a bare ratio of two of that struct's sums; there
// is no gating here beyond what the caller already applied by only reaching
// a club through rate()/defRate() succeeding (see cohSeasonBuild), which
// guarantees every denominator here is strictly positive — see the proof
// sketch in TestLeanDecompositionReadingsAreWiredCorrectly's synthetic case
// and the field-level reasoning in cohClubModel's own doc comment.
func cohS(c *cohClubModel) float64    { return c.xgXgscaleEmSum / c.xgEmSum }
func cohC(c *cohClubModel) float64    { return c.emSum / 990 }
func cohMwmA(c *cohClubModel) float64 { return c.xgEmSum / c.emSum }
func cohK(c *cohClubModel) float64    { return c.xgcCfEmSum / c.xgcEmSum }
func cohMwmB(c *cohClubModel) float64 { return c.xgcEmSum / c.emSum }

// cohLn11 is ln(11), the number of players on a pitch — the constant term in
// G. Named so the identity's own printed derivation and its code agree on
// where "11" enters.
var cohLn11 = math.Log(11)

func TestDiagAttackDefenceLeanDecomposition(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	factor, scale := analysis.CleanSheetState()
	fmt.Printf("\n=== the calibration constants this decomposition's identity does NOT depend on\n")
	fmt.Printf("cleanSheetXGCFactor = %.4f, cleanSheetScale = %.4f (analysis.CleanSheetState()).\n", factor, scale)
	fmt.Printf("The primary reading below is mean_obs[ln(rate)-ln(defRate)], the raw fixture-blind\n")
	fmt.Printf("quantity neither constant touches. At these shipped defaults (1.0, 1.0) that raw\n")
	fmt.Printf("reading is NUMERICALLY IDENTICAL to coherence_diag_test.go's blindD.meanD, which IS\n")
	fmt.Printf("read through cleanSheetXGCFactor/cleanSheetScale — see this file's header for why\n")
	fmt.Printf("that identity would not survive a non-default scale. This is a provenance guard, not\n")
	fmt.Printf("a second finding.\n")
	fmt.Printf("TeamXGCFactor: nil in this diagnostic's engine, exactly as coherence_diag_test.go's\n")
	fmt.Printf("own engine — never populated outside cmd/armband's applyTeamOverrides.\n")

	fmt.Printf("\n=== coverage: the rows behind each fit, through GW%d (matches the coherence diagnostic)\n", cohCutoff)
	fmt.Printf("%-10s %10s %10s %12s\n", "season", "xG rows", "xGC rows", "xGC rebuilt")

	type seasonRow struct {
		season string
		obs    []cohObs
		clubs  map[int]*cohClubModel
	}
	var rows []seasonRow
	var all []cohObs

	for _, pair := range sweepPairNames() {
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatal(err)
		}
		xgRows, xgcRows, xgcRebuilt := cohCoverage(cur)
		var rebuiltPct float64
		if xgcRows > 0 {
			rebuiltPct = 100 * xgcRebuilt / xgcRows
		}
		fmt.Printf("%-10s %10.0f %10.0f %11.0f%%\n", cur.Name, xgRows, xgcRows, rebuiltPct)

		obs, clubs, _, _, _, err := cohSeasonBuild(ctx, cfg, pair, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(obs) == 0 {
			continue
		}
		rows = append(rows, seasonRow{season: cur.Name, obs: obs, clubs: clubs})
		all = append(all, obs...)
	}

	if len(rows) < 2 {
		t.Skipf("only %d season(s) produced observations; there is no between-season "+
			"spread to report", len(rows))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].season < rows[j].season })

	// decompose computes meanD_direct, lnS, lnC, lnK, G and the residual for
	// one set of observations against the club map its OWN season built —
	// used per season and once pooled over the concatenated `all`.
	//
	// clubOf resolves an observation's attacker/defender to the cohClubModel
	// it was built from. Every observation in `obs` came from cohSeasonBuild
	// paired with exactly one `clubs` map (its own season's), so passing the
	// wrong map here would be a bug this function's own caller controls, not
	// something an observation carries a season tag for — cohObs has no
	// club-ID field, only the short names used for printing, so the caller
	// supplies the map explicitly rather than this function guessing it.
	type decomp struct {
		n                                                    int
		meanDDirect, lnS, lnC, lnK, g, identitySum, residual float64
	}
	decompose := func(obs []cohObs, clubOf func(o cohObs) (atk, def *cohClubModel, ok bool)) decomp {
		var d decomp
		var sumDirect, sumLnS, sumLnC, sumLnK, sumLnMwmA, sumLnMwmB float64
		for _, o := range obs {
			atk, def, ok := clubOf(o)
			if !ok {
				continue
			}
			// o.rate/o.defRate are exactly atk.rate()/def.defRate() —
			// cohSeasonBuild only ever constructs a cohObs from a rate/defRate
			// pair it read off these same two maps — so using the struct's own
			// fields (rather than recomputing atk.rate()/def.defRate()) is the
			// SAME number, not a second independent read.
			if o.rate <= 0 || o.defRate <= 0 {
				continue
			}
			sumDirect += math.Log(o.rate) - math.Log(o.defRate)
			sumLnS += math.Log(cohS(atk))
			sumLnC += math.Log(cohC(atk))
			sumLnK += math.Log(cohK(def))
			sumLnMwmA += math.Log(cohMwmA(atk))
			sumLnMwmB += math.Log(cohMwmB(def))
			d.n++
		}
		if d.n == 0 {
			return d
		}
		n := float64(d.n)
		d.meanDDirect = sumDirect / n
		d.lnS = sumLnS / n
		d.lnC = sumLnC / n
		d.lnK = sumLnK / n
		d.g = cohLn11 + sumLnMwmA/n - sumLnMwmB/n
		d.identitySum = d.lnS + d.lnC - d.lnK + d.g
		d.residual = d.meanDDirect - d.identitySum
		return d
	}

	// clubOfFor builds the clubOf closure for one season's own club map — a
	// club map is per-season (team IDs are not stable identifiers across the
	// pooled set the way they are within one season's boot), so the pooled
	// decomposition below cannot share one clubOf across seasons and instead
	// decomposes each season's slice against its OWN map, exactly like the
	// per-season rows.
	clubOfFor := func(clubs map[int]*cohClubModel) func(o cohObs) (*cohClubModel, *cohClubModel, bool) {
		return func(o cohObs) (*cohClubModel, *cohClubModel, bool) {
			atk, ok1 := clubs[o.attackerID]
			def, ok2 := clubs[o.defenderID]
			return atk, def, ok1 && ok2
		}
	}

	// withDMean is the secondary, NOT-decomposed reading: the run's actual
	// calibration constants AND actual fixture multipliers — identical to
	// coherence_diag_test.go's withD.meanD.
	withDMean := func(obs []cohObs) (float64, int) {
		var sum float64
		var n int
		for _, o := range obs {
			if v, ok := cohDivergence(o, factor, scale); ok {
				sum += v
				n++
			}
		}
		if n == 0 {
			return 0, 0
		}
		return sum / float64(n), n
	}

	const tol = 1e-9 // see header, "What would falsify this"

	fmt.Printf("\n=== the identity, per season and pooled\n")
	fmt.Printf("lnS/lnC/lnK/G are in log units and sum (as lnS+lnC-lnK+G) to meanD_direct, the\n")
	fmt.Printf("RAW mean_obs[ln(rate)-ln(defRate)] — see header for why this, not blindD.meanD\n")
	fmt.Printf("itself, is the exact target. `adjusted` is the secondary, non-decomposed reading.\n")
	fmt.Printf("`resid` is meanD_direct minus the four-term sum and must be within %.0e.\n\n", tol)
	fmt.Printf("%-10s %6s %9s %9s %9s %9s %9s %9s %10s %9s\n",
		"season", "n", "meanD", "lnS", "lnC", "lnK", "G", "sum", "resid", "adjusted")

	assertCloses := func(label string, d decomp) {
		t.Helper()
		if math.Abs(d.residual) > tol {
			t.Fatalf("%s: identity does not close — meanD_direct %.10f, lnS+lnC-lnK+G %.10f, "+
				"residual %.3e exceeds tolerance %.0e. The algebra in this file's header is exact; "+
				"a residual this large means the CODE does not match it, not that the identity is "+
				"approximate.", label, d.meanDDirect, d.identitySum, d.residual, tol)
		}
	}

	for _, r := range rows {
		d := decompose(r.obs, clubOfFor(r.clubs))
		adj, _ := withDMean(r.obs)
		fmt.Printf("%-10s %6d %9.4f %9.4f %9.4f %9.4f %9.4f %9.4f %10.2e %9.4f\n",
			r.season, d.n, d.meanDDirect, d.lnS, d.lnC, d.lnK, d.g, d.identitySum, d.residual, adj)
		assertCloses(r.season, d)
		sink.emitAll("attack_defence_lean_decomposition", "GW19 fit, GW20-38 scored", r.season,
			d.n,
			measure{"meanD_direct (raw, fixture-blind)", d.meanDDirect},
			measure{"lnS", d.lnS}, measure{"lnC", d.lnC}, measure{"lnK", d.lnK}, measure{"G", d.g},
			measure{"identity residual", d.residual},
			measure{"adjusted (secondary, not decomposed)", adj})
	}

	// Pooled: each season decomposed against its OWN club map (team IDs are
	// season-scoped), then combined the same way meanD itself pools — an
	// unweighted mean over the concatenated observation set, exactly how
	// coherence_diag_test.go's own `all`/blindD/withD are pooled, never a
	// mean of per-season means.
	var pooledN int
	var sumDirect, sumLnS, sumLnC, sumLnK, sumG float64
	for _, r := range rows {
		d := decompose(r.obs, clubOfFor(r.clubs))
		pooledN += d.n
		sumDirect += d.meanDDirect * float64(d.n)
		sumLnS += d.lnS * float64(d.n)
		sumLnC += d.lnC * float64(d.n)
		sumLnK += d.lnK * float64(d.n)
		sumG += d.g * float64(d.n)
	}
	var pooled decomp
	if pooledN > 0 {
		n := float64(pooledN)
		pooled.n = pooledN
		pooled.meanDDirect = sumDirect / n
		pooled.lnS = sumLnS / n
		pooled.lnC = sumLnC / n
		pooled.lnK = sumLnK / n
		pooled.g = sumG / n
		pooled.identitySum = pooled.lnS + pooled.lnC - pooled.lnK + pooled.g
		pooled.residual = pooled.meanDDirect - pooled.identitySum
	}
	adjAll, _ := withDMean(all)
	fmt.Printf("%-10s %6d %9.4f %9.4f %9.4f %9.4f %9.4f %9.4f %10.2e %9.4f   POOLED\n",
		"", pooled.n, pooled.meanDDirect, pooled.lnS, pooled.lnC, pooled.lnK, pooled.g,
		pooled.identitySum, pooled.residual, adjAll)
	assertCloses("POOLED", pooled)
	sink.emitAll("attack_defence_lean_decomposition", "GW19 fit, GW20-38 scored", "pooled",
		pooled.n,
		measure{"meanD_direct (raw, fixture-blind)", pooled.meanDDirect},
		measure{"lnS", pooled.lnS}, measure{"lnC", pooled.lnC}, measure{"lnK", pooled.lnK},
		measure{"G", pooled.g},
		measure{"identity residual", pooled.residual},
		measure{"adjusted (secondary, not decomposed)", adjAll})

	fmt.Printf("\n=== sanity check against prior work — NOT a target, a divergence is reported either way\n")
	fmt.Printf("Prior work (TestDiagTeamGoalShare's \"expected minutes per club per match\") measured\n")
	fmt.Printf("modelMinutes/990 averaging close to 0.99 across club-seasons — exactly this file's C.\n")
	fmt.Printf("S was separately expected near 0.98. Measured here, pooled: S = %.4f (lnS = %.4f),\n",
		math.Exp(pooled.lnS), pooled.lnS)
	fmt.Printf("C = %.4f (lnC = %.4f). ", math.Exp(pooled.lnC), pooled.lnC)
	nearS := math.Abs(math.Exp(pooled.lnS)-0.9822) < 0.01
	nearC := math.Abs(math.Exp(pooled.lnC)-980.0/990.0) < 0.01
	if nearS && nearC {
		fmt.Printf("Both land within 0.01 of the prior-work figures.\n")
	} else {
		fmt.Printf("At least one diverges from the prior-work figures by more than 0.01 — reported\n")
		fmt.Printf("as measured; the first suspect is this implementation, not the prior measurement.\n")
	}
	fmt.Printf("\n-lnK + G carries the remainder: %.4f (of the pooled meanD_direct %.4f).\n",
		-pooled.lnK+pooled.g, pooled.meanDDirect)

	fmt.Printf("\nThis diagnostic authorises nothing: it is an exact re-expression of a number\n")
	fmt.Printf("TestDiagAttackDefenceCoherence already prints, not a new measurement, and no\n")
	fmt.Printf("scoring term moves because of anything printed here.\n")
}

// TestLeanDecompositionReadingsAreWiredCorrectly pins the two things about
// this decomposition that could be wrong while looking plausible: that the
// per-club identity (S x C x 11 x mwmA == rate, K x mwmB == defRate) holds
// bit-for-bit on a hand-built cohClubModel, and that the season-level sum
// (lnS + lnC - lnK + G) reproduces a directly hand-computed meanD on a small
// synthetic observation set — catching a sign error or a misplaced average
// (per-club instead of per-observation) before this ever needs the archive.
// Runs without DIAG and without the archive.
func TestLeanDecompositionReadingsAreWiredCorrectly(t *testing.T) {
	// (a) Per-club identity on one hand-built cohClubModel. These five sums
	// are free — the identity is a tautological rearrangement that holds for
	// ANY positive values, not a fact about real football — so any positive
	// numbers exercise it; chosen here so every intermediate ratio is a clean
	// decimal to make a hand check easy:
	//   rate() = xgXgscaleEmSum/90 = 99/90 = 1.1
	//   S = xgXgscaleEmSum/xgEmSum = 99/90 = 1.1, C = emSum/990 = 900/990 = 0.909090...
	//   mwmA = xgEmSum/emSum = 90/900 = 0.1  =>  S*C*11*mwmA = 1.1*0.909090...*11*0.1 = 1.1
	//   defRate() = xgcCfEmSum/emSum = 88/900 = 0.097777...
	//   K = xgcCfEmSum/xgcEmSum = 88/80 = 1.1, mwmB = xgcEmSum/emSum = 80/900 = 0.088888...
	//   K*mwmB = 1.1*0.088888... = 0.097777...
	c := &cohClubModel{emSum: 900, xgEmSum: 90, xgXgscaleEmSum: 99, xgcEmSum: 80, xgcCfEmSum: 88}
	const idTol = 1e-12

	gotRate := c.rate()
	viaIdentity := cohS(c) * cohC(c) * 11 * cohMwmA(c)
	if math.Abs(gotRate-viaIdentity) > idTol {
		t.Fatalf("S*C*11*mwmA = %.12f, want rate() = %.12f (within %.0e)", viaIdentity, gotRate, idTol)
	}
	if math.Abs(gotRate-1.1) > idTol {
		t.Fatalf("rate() = %.12f, want the hand-computed 1.1", gotRate)
	}

	gotDefRate, ok := c.defRate()
	if !ok {
		t.Fatalf("defRate() reported ok=false for a strictly positive synthetic club")
	}
	viaIdentity2 := cohK(c) * cohMwmB(c)
	if math.Abs(gotDefRate-viaIdentity2) > idTol {
		t.Fatalf("K*mwmB = %.12f, want defRate() = %.12f (within %.0e)", viaIdentity2, gotDefRate, idTol)
	}
	wantDefRate := 88.0 / 900.0
	if math.Abs(gotDefRate-wantDefRate) > idTol {
		t.Fatalf("defRate() = %.12f, want the hand-computed %.12f", gotDefRate, wantDefRate)
	}

	// (b) Season-level sum on a small synthetic set of three cohObs-like
	// observations across three clubs, built asymmetrically on purpose: club
	// X attacks twice and defends once, so a bug that averaged PER CLUB
	// instead of PER OBSERVATION (giving X's contribution weight 1 instead of
	// 2) would move lnS/lnC/lnMwmA away from the direct computation, and a
	// sign error on lnK or a dropped ln11 in G would move the sum away from
	// meanD_direct by a large, obvious amount — not lost in tolerance.
	clubX := &cohClubModel{emSum: 900, xgEmSum: 90, xgXgscaleEmSum: 99, xgcEmSum: 80, xgcCfEmSum: 84}
	clubY := &cohClubModel{emSum: 850, xgEmSum: 68, xgXgscaleEmSum: 71.4, xgcEmSum: 76.5, xgcCfEmSum: 68.85}
	clubZ := &cohClubModel{emSum: 950, xgEmSum: 114, xgXgscaleEmSum: 102.6, xgcEmSum: 66.5, xgcCfEmSum: 73.15}
	clubs := map[int]*cohClubModel{1: clubX, 2: clubY, 3: clubZ}

	mk := func(atkID, defID int) cohObs {
		atk, def := clubs[atkID], clubs[defID]
		dr, ok := def.defRate()
		if !ok {
			t.Fatalf("synthetic defender club %d has no defRate", defID)
		}
		return cohObs{
			attackerID: atkID, defenderID: defID,
			rate: atk.rate(), atkMul: 1,
			defRate: dr, defMul: 1,
		}
	}
	obs := []cohObs{
		mk(1, 2), // X attacks Y
		mk(1, 3), // X attacks Z
		mk(2, 1), // Y attacks X
	}

	var sumDirect, sumLnS, sumLnC, sumLnK, sumLnMwmA, sumLnMwmB float64
	for _, o := range obs {
		atk, def := clubs[o.attackerID], clubs[o.defenderID]
		sumDirect += math.Log(o.rate) - math.Log(o.defRate)
		sumLnS += math.Log(cohS(atk))
		sumLnC += math.Log(cohC(atk))
		sumLnK += math.Log(cohK(def))
		sumLnMwmA += math.Log(cohMwmA(atk))
		sumLnMwmB += math.Log(cohMwmB(def))
	}
	n := float64(len(obs))
	meanDDirect := sumDirect / n
	lnS := sumLnS / n
	lnC := sumLnC / n
	lnK := sumLnK / n
	g := cohLn11 + sumLnMwmA/n - sumLnMwmB/n
	identitySum := lnS + lnC - lnK + g

	if math.Abs(meanDDirect-identitySum) > idTol {
		t.Fatalf("season-level identity does not close on the synthetic set: "+
			"meanD_direct = %.12f, lnS+lnC-lnK+G = %.12f, diff %.3e exceeds %.0e — "+
			"a sign error or a per-club-instead-of-per-observation average would show up here",
			meanDDirect, identitySum, meanDDirect-identitySum, idTol)
	}

	// A deliberately wrong combination — dropping G — must NOT close, so this
	// test would actually have caught the class of bug it exists for.
	wrongSum := lnS + lnC - lnK
	if math.Abs(meanDDirect-wrongSum) <= idTol {
		t.Fatalf("test construction error: dropping G still closed the identity on this " +
			"synthetic set, so this test cannot discriminate a missing-G bug — pick different " +
			"synthetic values")
	}
}
