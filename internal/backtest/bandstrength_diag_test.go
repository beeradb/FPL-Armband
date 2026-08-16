package backtest

// Is the 3/14/3 attack/defence band adjustment worth anything on today's archive?
//
//	DIAG=1 EXP=bandstrength FPL_CELLS=/tmp/bandstrength.csv \
//	  scripts/replay -run TestDiagBandStrength -v -timeout 2h
//
// # Why this exists
//
// `BandStrength` ships at 0 — the bands are computed and then multiplied by
// nothing. The verdict behind that zero is a pair of absolute season totals in
// bands.go: "2195 at full strength against 2208 for leaving FPL's blended FDR
// alone", with the refutation resting on **twelve points on 2022-23 as the
// held-out season**.
//
// Three things are wrong with citing that today, and none of them is that the
// number was wrong when it was taken.
//
//   - **It is an absolute total.** This record's own convention is that absolute
//     point totals are not comparable across eras of this codebase, and lists
//     four contamination events between then and now that move a season by up to
//     115 points. A 13-point gap between two totals is far inside that.
//   - **It was a `POLICY` figure deciding a scoring constant, and that is
//     recovered from the code rather than inferred.** At the commit that produced
//     it (`bad797c`), `cmd/armband/backtest.go` applies `FPL_BAND_STRENGTH` to
//     `base` — the `SimConfig` handed to the three transfer policies — while the
//     hold row on the next lines is built from a fresh
//     `backtest.SimConfig{Weights: cfg.Weights}` that never sees `base`. The hold
//     baseline was therefore **byte-identical across that entire sweep by
//     construction**, and only the POLICY rows could move. Corroborating but
//     weaker: 2208 is exactly the mean of the `BonusWeight` table's
//     2115/2377/2132, which the record labels three GW1 `POLICY` cells, and it
//     recurs in simulate.go's `FreeCost` comment, which is POLICY on mechanism
//     since transfer constants are byte-identical on `HOLD`. So the constant was
//     decided on the metric with roughly four times the threshold — 33 against 18
//     on this run's own arms.
//   - ⚠️ **The arm that actually decided it was QUARTER strength, and this run
//     does not re-run it.** `bands.go` quotes only the full-strength arm, but
//     `bad797c`'s message records the sweep: *"Quarter strength gains +18 on the
//     three tuned seasons but that is the best of six swept values against a ±20
//     noise floor, and out of sample on 2022-23 it loses 12."* That is an argmax
//     over six values whose winner sits **inside its own stated noise floor** —
//     the winner's curse, self-documented. This sweep runs 0 / 1 / 2, so
//     **s = 0.25 remains unrun** and the deciding arm has not been re-measured on
//     the repaired archive. Anyone reading this run as "the recorded verdict was
//     re-tested" should read that sentence again.
//   - **The data underneath it changed, on half this grid rather than a sixth.**
//     The often-quoted "6 of 24 cells, all 2022-23" is a *four-season* fact. On the
//     grid this runs, `FPL_NO_XGC_REPAIR` moves **18 of 36 cells** — 2020-21,
//     2021-22 and 2022-23 — and `baseXP90` gates the clean sheet and the concede
//     deduction on `XGC90 > 0`, so before the reconstruction `defenceBandAdj`, the
//     larger half of the adjustment, multiplied a term that did not exist there.
//   - **It was never banked.** `BandStrength` appears in `stats/snapshots/` only
//     as prose. There is no row for it under any `cells/` directory, so the
//     figures cannot be re-derived from anything in this repository — only
//     re-measured. That is what this run is for.
//
// # Pre-registration
//
// Written before the run, because afterwards there is always a story.
//
//   - **P0, the mediator, checked before any points column is read.**
//     `TestBandStrengthArrivesOnTheScoredPath` already establishes arrival: at a
//     GW19 cutoff on the replay's own point-in-time bootstrap, turning the knob
//     from 0 to 1 moves **the overwhelming majority of the 503 scored players**.
//     ⚠️ The exact count is deliberately not written down: it alternates between
//     469 and 494 across runs, because at that cutoff the third-best attack is a
//     two-way tie and the band assignment is itself non-deterministic — see
//     banddeterminism_test.go. The assertion is `moved > 0`, which is stable; a
//     pre-registration is the worst possible place for a figure that cannot be
//     re-derived, and quoting one here would have been this branch's own finding
//     used against it. This is not decoration. The
//     recorded bug in exactly this code is that `loadFixtures` dropped scorelines,
//     so `teamBands` saw no finished matches and every adjustment returned 1 — a
//     null that measured nothing while looking clean. The second mediator is
//     `squad_hash`: the count of cells whose opening fifteen differs from the
//     baseline's. A flat result with an inert mediator is not a measurement.
//   - **P0b, the confinement check, paired with the liveness check that must
//     move.** `bandMinMatches` is 5 and the opening squad is built at cutoff
//     `start-1`, so at `StartGW = 1` the cutoff is 0, `teamBands` returns
//     `ready: false` and both adjustments return exactly 1. **The opening fifteen
//     must therefore be byte-identical between arms in all six GW1 cells.** A GW1
//     cell whose `squad_hash` moved is a point-in-time leak — it would mean
//     `teamBands` saw a finished fixture before a ball was kicked — and is the
//     razor-sharp form of the `loadFixtures` bug above. The liveness half is that
//     `squad_hash` **must** move somewhere in the other thirty, or the knob is
//     inert on the squad the metric holds. So the denominator for squad movement
//     is **30, not 36**, and GW6 (cutoff 5) is marginal by construction and read
//     neither way.
//   - **P1, the directional claim.** None. This is a re-measurement of a verdict,
//     not a proposal, and the prior on it is genuinely two-sided: the descriptive
//     effect in bands.go is large and real (a defender facing a bottom-three
//     attack returns 21-41% above his own average), while every recorded attempt
//     to *act* on fixture difficulty in this project has failed to win points.
//   - **P2, the expected outcome is UNRESOLVED, and this run CANNOT refute the
//     recorded magnitude.** The whole fixture-difficulty family is recorded as
//     unresolvable at current scoring, and zeroing the defensive response entirely
//     costs 20 points — inside noise. To reject a recorded effect of ~0.09 pts/gw
//     the confidence half-width would have to be well below it, and at
//     `t_crit(5) = 2.571` on a HOLD standard error of this order it is tens of
//     points a season. The recorded 13 points will sit comfortably *inside* the
//     interval and is therefore confirmed by nothing. **The recorded figure is
//     retired on provenance, not on points** — see the header. The threshold
//     reported must come from **these** cells via `stats/variance_components.R`
//     and never from the record's global HOLD median of 33.
//   - **P3, what would change the verdict, and on which single column.** `hold` —
//     realised HOLD points — is the **sole deciding column**. The sweep reports six
//     metric families and choosing among them afterwards is the argmax defect one
//     level above the arms. `hold_xpoints` cannot rescue a null: the pilot finding
//     is that on HOLD the proxy cuts standard errors 20-25% *and attenuates the
//     means with them*, so |t| went down on five of six contrasts. If it resolves
//     where `hold` does not, that is an anomaly to explain and not a result. Only
//     a paired difference clearing its own `t_crit(df) x SE x 38` threshold
//     counts; a sign, a rank or a leave-one-season-out pattern does not. A
//     favourable reading is a report and not a change — moving a shipped constant
//     is a separate decision from measuring it.
//   - **P4, 2022-23 is reported separately, and carries no data-state privilege
//     here.** It is the season the original held-out verdict was taken on, which
//     is the only reason it is broken out. ⚠️ It is **not** the only repaired
//     season on this grid: "6 of 24 cells, all 2022-23" is a *four-season* fact,
//     and on the extended grid `FPL_NO_XGC_REPAIR` moves **18 of these 36 cells** —
//     2020-21, 2021-22 and 2022-23. So the re-measurement is owed on half the grid
//     rather than a sixth of it, and that is the stronger argument for this run
//     than the held-out one: `baseXP90` gates the clean sheet and the concede
//     deduction on `XGC90 > 0`, so before the reconstruction `defenceBandAdj` — the
//     larger half of the adjustment by bands.go's own table — multiplied a term
//     that did not exist in those cells. One season of six: **point estimate and
//     sign only, no SE, no t, no p, no threshold.** At S = 1 the season-clustered
//     estimator is degenerate, and the six entry points inside a season are nested
//     rather than independent, so a naive standard error over them is badly
//     optimistic too.
//     ⚠️ **Read 2022-23 on the metric the original check used, which was
//     `POLICY`.** At s = 1 it reads **+29.4 a season on `HOLD` and −16.3 on
//     `POLICY`**; at s = 2, −43.7 and −2.9. So on the metric the held-out
//     refutation was actually taken on, 2022-23 keeps the **same sign as that
//     refutation** and does not reverse. Any sentence of the form "the held-out
//     season now reverses" is crossing metrics, and it is the most quotable wrong
//     claim available from this run.
//
// # Metric and grid
//
// **HOLD.** `BandStrength` scales a fixture multiplier that prices every player,
// so it is a scoring constant, and HOLD excludes the transfer path's 303-point
// noise.
//
// POLICY is collected and is **a second, under-powered question rather than
// context**. There is a named mechanism by which the bands could pay a transfer
// and not a fifteen, and it is already in this codebase: `SquadFixtureWeight`
// exists on the hypothesis that difficulty is worth more to a transfer, which
// deliberately buys a run of fixtures, than to a fifteen picked before any
// fixture is near. So POLICY is quoted with **its own threshold** from these
// cells, never HOLD's, its absolute totals are not comparable with anything in
// the record because every cell runs a five-transfer bank regardless of season,
// and a POLICY-only reading does not move the constant — it would open a
// `SquadFixtureWeight`-style split arm, which is a different measurement.
//
// The grid is the shipped six seasons by six entry points, 36 cells per arm.
// All arms run **in one process** — `BandStrength` is a `SimConfig.Weights`
// field, so `apply` sets it directly — no env var **on this path**, no second
// process and no separate `run_id` to pool across. (`FPL_BAND_STRENGTH` does
// exist, but only `cmd/armband backtest` reads it, and its `> 0` guard means it
// cannot express 0 or a negative. `FPL_WEIGHT=band=` reaches the live CLI. Neither
// touches this sweep.)
//
// # The third arm is a canary, not a candidate
//
// `band_strength 2` sizes the instrument, on the precedent the clean-sheet 2x2
// set: halving *every* clean sheet cost −21.6 against its own threshold of 28 and
// still did not resolve, which is how that family was established as unmeasurable
// rather than merely unresolved. At s = 2 the defensive adjustment is ±0.50,
// against the within-player effect bands.go measures at +41%/−27% for defenders
// facing the extreme attacking bands — so it is roughly "apply the measured
// defensive effect in full", a defensible dose rather than a strawman.
//
// **It is read as a statement about detectability and never about the sign at
// s = 1.** Points need not be monotone in s: `Optimize` re-solves a knapsack
// under a changed objective, so a doubled dose is a different squad rather than a
// scaled one. **The decision rests on the `1 against 0` contrast alone**, and the
// canary is excluded from it.
//
// ⚠️ **What the canary CANNOT license, written here because the first draft of
// this file claimed it.** A canary sizes an instrument only if it *scales*. The
// clean-sheet precedent works because its canary was a strict amplification —
// roughly 4x the candidate, with the sign predicted — so "the canary does not
// resolve" divides down into "the candidate is about 4x below detection". Neither
// condition holds here. The dose ratio is 2x, so a canary failing a threshold of
// 27 bounds the candidate only at |e| < 13.5 a season, which lands *on* the
// observed +13.6 and therefore excludes nothing. And the linearity the division
// needs is refuted by the run itself: the canary came back **smaller and
// opposite**, not larger. The same concession that makes a non-monotone result
// defensible — a re-solved knapsack — is what voids the sizing inference.
//
// So a non-resolving canary of this kind buys **"unresolved twice"**, not
// "unmeasurable on this harness". Reserve that phrase for the structural cases.
// The genuinely structural point this grid does support is narrower and is in P0b:
// at a GW1 entry the fifteen is bought at cutoff 0 with the bands not ready and
// `HOLD` never re-buys, so that column cannot express the intervention's squad
// channel at all.
//
// # Data state
//
// Shipped defaults, all repairs on: none of `FPL_NO_XG_REPAIR`,
// `FPL_NO_XGC_REPAIR` or `FPL_NO_XG_AGGREGATE` is set, which is the whole point —
// the repaired archive is the data state the original verdict does not have.

import (
	"os"
	"testing"
)

func TestDiagBandStrength(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	// The shipped zero is the baseline, so a positive paired difference means the
	// bands are worth having — the sign a reader expects from the question. Same
	// convention as TestDiagBenchShape, where flat is the baseline.
	//
	// `setting` reads the value back off the applied config rather than declaring
	// it, so the CSV's column cannot describe a setting the cell did not have.
	band := func(strength float64) func(sc *SimConfig) {
		return func(sc *SimConfig) { sc.Weights.BandStrength = strength }
	}
	strengthOf := func(sc SimConfig) float64 { return sc.Weights.BandStrength }

	runPolicySweep(t, []policyVariant{
		{
			label:   "band_strength 0 — FPL's blended FDR alone (ships)",
			apply:   band(0),
			setting: strengthOf,
		},
		{
			label:   "band_strength 1 — the 3/14/3 adjustment at full strength",
			apply:   band(1),
			setting: strengthOf,
		},
		{
			// The canary. Diagnostic only — see the header. It is NOT a candidate
			// setting and is not part of the decision contrast.
			label:   "band_strength 2 — CANARY, double dose, sizes the instrument",
			apply:   band(2),
			setting: strengthOf,
		},
	}, starts)
}
