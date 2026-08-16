// Command priorblend measures what `prior_half_life` does to the model's
// predictions, on the population the setting can actually reach.
//
// # The question
//
// `prior_half_life` blends the season *before last* into a player's prior, but
// only for players whose last season was thin — under `priors.ThinSeason`, 1,710
// minutes, half a season. It ships off. The recorded reason is that blending cost
// about 7 points a season when it was tested on two replayed seasons, with the
// honest caveat that neither of those seasons contained the case the feature
// exists for: a good player whose last season is an injury artefact.
//
// So the recorded figure measured the cost and never the benefit. This measures
// the benefit, on the instrument built for the question — prediction error, split
// into its systematic and its random half — rather than on the replay, where an
// effect this size cannot be seen at all.
//
// # Why this is a command and not a test
//
// The prediction benchmark lives in `internal/backtest`'s test files and its
// helpers are unexported, so an arm that varies `SimConfig.PriorHalfLife` cannot
// be added from outside the package. It also predicts from gameweek 6, which is
// past the window this setting matters most in: with no current-season football
// played the prior *is* the estimate, and the opening squad is the decision the
// setting was written for.
//
// The output is deliberately the *same CSV schema* as that benchmark, so
// `stats/prediction_inference.R` reads it unchanged. Inference lives in R; this
// prints means and counts and no standard error, no t, and no verdict word.
//
// # Usage
//
//	go run ./cmd/priorblend -csv /tmp/pb.csv
//	python3 scripts/pbsummary.py /tmp/pb.csv "injury shaped: "
//
// # THE GATE IS NOW IN THE MODEL, AND THIS SECTION IS THE RUN THAT MOTIVATED IT
//
// Everything below was measured when `prior_half_life` was UNGATED — it blended
// for any thin last season, zero minutes included. It is kept verbatim because it
// is the evidence the gate was built on, and because two of its arms still exist.
//
// `analysis.ShouldBlendPrior` now confines the blend to a thin but NON-ZERO last
// season, on all three paths.
//
// ⚠️ **EVERY TABLE FROM HERE DOWN TO "What this does not say" WAS MEASURED BEFORE THE
// xGC REPAIR AT `7cb769e`, and none of it is reproducible without
// `FPL_NO_XGC_REPAIR=1`.** The repair is not neutral to this experiment: see the
// era-hazard block below, where it flips half-life 2's sign. The asymmetry is by
// construction, because only the blend arm reaches back into the seasons that
// carried no xGC — which is the same point this file spent a paragraph getting
// wrong, two hundred lines down. `currentSeasons` is 2020-21 to 2025-26 and priors
// reach 2018-19, so four of six current seasons and every prior season are inside
// the repaired range.
//
// The figures measured ON the repaired archive are exactly two: the seven-arm ladder
// under "The statistic that decides this", and the era-hazard table itself. **The
// whole-field ladder below has not been re-run repaired at any half-life**, only at
// the two channel arms. Read everything between here and that section as the
// pre-repair record.
//
// ## The load-bearing result is a wiring check, not a number
//
// `prior_half_life 1` and `prior_half_life 1, injury cases only` are now THE SAME
// INDEX — compared player by player in all six season pairs, 666 to 865 priors
// each, no disagreement — and every figure they produce is identical to the last
// printed digit. So the by-population arm's result transfers to the shipped code
// with no inferential step: the two arms are not similar, they are the same
// computation, and the ordering figure below is the one already measured.
//
// That is worth more than the re-run itself, because the re-run added exactly TWO
// genuinely new statistics: the whole-field ordering at half-lives other than 1.
// Everything else is either identical to the ungated run by construction (the
// injury, absent, control and premium populations) or an n-weighted combination
// of two sub-populations already reported (the whole-field bias and mse).
//
// ## The two populations, and which of them is a measurement
//
//   - Injury-shaped: byte-identical to the ungated run in every arm. Bias −0.2244
//     shipped to −0.0813 at half-life 1, spread 2.1536 to 2.1498. The gate never
//     touched these players, so this is the ungated measurement restated.
//   - Absent: identical to shipped in every arm, all six seasons, exactly. This is
//     a CONSTRUCTION, not a measurement, and it should not be read as one — those
//     players are dropped from the index in both arms and `shrinkToLeague` reads
//     `Engine.leagueRates`, built from the bootstrap alone, so their predictions
//     cannot depend on any other player's prior. What it verifies is that the gate
//     is actually reached on the path the benchmark exercises. The MEASUREMENT
//     that justifies the gate is the ungated +0.0754 over-prediction, and it was
//     already made.
//
// ## The ordering, which is what decides it — and it is NOT monotone
//
// Δ rank correlation against shipped, paired within the gameweek. Half-life 0 is a
// rung of this ladder and its value is exactly 0.000000 by definition; leaving the
// anchor out is what made an earlier version of this block call the gated ladder
// "monotone", which it is not.
//
//	half-life   gated      excl 2022-23   ungated     gate effect
//	0           0.000000   0.000000       0.000000    —
//	0.125      +0.000142  +0.000104       —           —
//	0.25       +0.000431  +0.000217       —           —
//	0.5        +0.000332  +0.000032      −0.001704   +0.002035
//	1          −0.000204  −0.000530      −0.002250   +0.002046
//	2          −0.000752  −0.001088      −0.002810   +0.002058
//
// Three things, in order of how much they matter.
//
// **The gate removes a LEVEL, not a slope.** Its effect is +0.00204 ± 0.5% across
// a fourfold change in half-life, and the residual increments are identical gated
// and ungated (−0.00053/−0.00055 against −0.00055/−0.00056). Two scalars generate
// the whole table. The mechanism is that `BlendPriors` weights a season by recency
// AND minutes, so for a player with no minutes last season the older season
// dominates at any half-life: the absent harm is a STEP, the injury effect is a
// DOSE RESPONSE. "Removes about ninety per cent of it" was the wrong shape of
// statement.
//
// **The curve rises to an interior maximum near 0.25 and then declines through
// zero.** That is a shape rather than an argmax — three consecutive positive rungs
// returning smoothly to a known anchor — and it survives dropping 2022-23, which
// halves it without removing it. But the residual slope beyond the maximum is
// negative and it is the same slope the ungated run has, so what the ladder says
// is that MORE of this feature orders the field worse.
//
// **Nothing resolves, and the gameweek-clustered t is not evidence here.** The
// treatment is constant within a player-season — the prior index does not change
// across gameweeks — so 227 gameweek clusters are near-replicates of six
// season-level quantities and clustering must be at the level of assignment. The
// design effect is 2.5x. On season clustering (df 5, critical t 2.571) the best
// rung is +1.87 and the worst −1.51; dropping 2022-23 gives +2.18 and −2.45
// against a critical 2.776.
//
// ⚠️ **2022-23 carries more than it should and this instrument behaves worst in
// it.** 99% of the +0.00033 at half-life 0.5 is that one season, and it sits about
// +0.002 above the other five in every arm of both runs. Its top-twenty signed
// error is +1.92 over GW16-38 against ±0.25 for every other season-half, stepping
// at GW17 — where both the World Cup restart and the archive's native xG seam sit,
// and they cannot be separated from this file. Note that dropping it is a
// leave-one-out on a season chosen for being anomalous, which is its own selection.
//
// ## The tail, where the earlier text was reassuring and the data is not
//
// Pooled, the signed error over the twenty highest-predicted players moves toward
// zero (+0.2206 shipped to +0.2016 at half-life 1) and does not resolve. But FIVE
// OF SIX SEASONS MOVE AWAY from zero and 2022-23 alone supplies more than the whole
// pooled change; excluding it the paired difference is +0.0131, season-clustered
// t +3.06 at df 4. So nothing here says the top of the distribution got less
// over-rated, and the direction the other five seasons give is the one the argmax
// rule cares about.
//
// ## The statistic that was emitted all along and never read
//
// `emitGameweek` writes `rank_corr` per POPULATION, so the ordering *within* the
// treated players has been in every CSV this command has ever produced. Every table
// above reports the whole field instead. It isolates the INFORMATION channel: a
// uniform uplift to a selected population leaves within-population ordering exactly
// unchanged, so a movement here cannot be a level shift.
//
// ⚠️ **It is NOT "the statistic that decides this", which is what this section was
// first called.** A uniform uplift to a selected subgroup is not nothing — it
// relocates that subgroup within the whole-field ordering, which is the ordering the
// optimiser actually consumes. The "a bias shared by every player in a position is
// not an ordering error" precedent does **not** transfer, because that rests on FPL
// forcing five defenders and two keepers and **there is no quota on thin-season
// players**. Only the whole-field figure sees the level channel.
//
// The `channel: minutes only` arm proves it inside this very experiment:
// within-population +0.00017 at t 0.11 — no ordering information at all — while it
// moves the whole field at -0.00040, t -4.33, six of six seasons, under both loads. A
// pure rescaling did something, at the largest |t| in the run, and the
// within-population statistic was blind to it by construction.
//
// **So both must be read** — within-population for information, whole field for
// level. They agree on the dose arms here (whole-field ladder +0.00014 / +0.00045 /
// +0.00045 / +0.00009 / -0.00031 at t 1.77 / 1.75 / 1.41 / 0.28 / -1.00), so the
// verdict stands; it stands on two legs rather than on one deciding number.
//
// It is not zero. On the repaired archive, half-life 0.25 gives +0.00233 with all six
// seasons positive and rates-only +0.00234 at t +3.28. See the table under "the era
// hazard" for the full ladder under both loads.
//
// **But nothing survives multiplicity, and that is the honest headline.** Seven arms
// on one population, Holm-corrected:
//
//	arm                  pooled     t      raw p    Holm
//	channel: rates only  +0.00234  +3.28   0.0221   0.1544
//	half-life 0.25       +0.00233  +2.90   0.0336   0.2016
//	half-life 0.5        +0.00402  +2.81   0.0374   0.2016
//	half-life 1          +0.00332  +1.99   0.1037   0.4150
//	half-life 2          +0.00213  +1.21   0.2821   0.8463
//	half-life 0.125      +0.00044  +1.15   0.3011   0.8463
//	channel: minutes     +0.00018  +0.11   0.9141   0.9141
//
// So the affirmative case for turning this on does not exist at the standard this
// record demands of a constant. Read the ladder as a shape that has not resolved.
//
// ## The one stable signal is NEGATIVE, and it does not survive Holm either
//
// Across the whole field with the absent players dropped, `channel: minutes only`
// makes the ordering worse in all six seasons, and again in all six under the
// unrepaired archive — at -0.00040 (t -4.34) repaired and -0.00040 (t -4.43)
// unrepaired. It is the only figure in this command stable across the repair.
//
// ⚠️ **Re-measured 2026-08-14, after this command learned to set NoXG/NoXGC/NoDefCon
// from what each season actually loaded.** That moved the unrepaired arm and only it,
// which is now checked rather than asserted: between the two arms, 2023-24, 2024-25
// and 2025-26 are identical to the digit and only the three seasons the xG backfill
// reaches move at all.
//
//	arm         population       rank_corr   t(season)  df   raw p   negative
//	repaired    popEveryone      -0.000389     -4.83     5   0.0048    6 of 6
//	unrepaired  popEveryone      -0.000493     -4.06     5   0.0097    6 of 6
//	unrepaired  popFieldSound    -0.00051      -4.66     5     -       6 of 6
//
// The REPAIRED arm reproduces the recorded -0.00039 / t -4.82 / p 0.0048 to the
// recorded precision (it prints t -4.83), so the Holm figure below is untouched.
// Quote the finding there.
//
// The unrepaired arm is the replication and is carried by the SIGN TEST, not by its
// own corrected p. ⚠️ That corrected p is 8 x 0.0097 = 0.078, which is BONFERRONI and
// not Holm -- Holm would rank this arm second behind the repaired arm's 0.0048 and
// multiply by 7, giving 0.068. Both exceed 0.05 so nothing downstream turns on it, but
// a mislabelled correction in a record file gets copied. And by this block's own
// reasoning -- the two loads share five-sixths of their data -- the replication is
// arguably one hypothesis measured twice rather than a second family member at all.
//
// ⚠️ **Three corrections to how that was first written up here.** It is twelve arms
// over **six independent seasons**, not twelve draws — the two loads share
// five-sixths of their data, and the repair is exactly zero in 2025-26, so that
// season contributes two identical observations. The sign evidence is p = 0.031.
//
// It is **not exempt from the multiplicity correction applied to the positive arms**,
// and on this population it does not survive one: t = -4.34 at df 5 is a raw p of
// 0.0074, which the seven-arm Holm family takes to **0.052**. Calling it a refutation
// was correcting the arms this record wanted to disbelieve while exempting the one it
// believed.
//
// **But quote it on `popEveryone`, where it does survive** — -0.00039 at t -4.82, raw
// p 0.0048, **Holm over eight arms 0.0385**. The figures above are on
// `popFieldSound`, which this file's own comment marks SUPERSEDED, so the strongest
// negative finding in the run was being quoted off a retired population. On the
// genuine whole field it clears the same bar the affirmative arms fail.
//
// It still points away from the obvious candidate. The project's strongest recorded
// precedent is that recency on MINUTES removes a bias and works while recency on
// RATES buys accuracy and loses points, which argues for shipping a minutes-only
// blend; on the statistic the optimiser consumes, minutes-only buys no ordering
// inside the treated group and costs ordering across the field. ⚠️ Do not extend
// that to "rates-only is the best arm on both" — no whole-field figure for the
// rates-only arm is published here.
//
// ⚠️ **And the minutes result has the shape of a rescaling by this section's own
// criterion.** Within-population +0.00018 (t 0.11) against whole-field -0.00040 is
// exactly the fingerprint defined above for a level shift rather than an ordering
// intervention — and `channelPriors` already measures this channel as "a
// near-constant +3.36 a gameweek ... a fixed-size shift rather than a correction
// sized to the error". Whether that shift is a falsehood or merely mis-sized is
// decided by the BIAS column for the two channel arms, which is not published. The
// precedent it argues against was also established on **replayed points**, and an
// ordering statistic cannot overturn a points result in either direction.
//
// The precedent does not transfer, and the reason is worth stating because it is a
// distinction the record does not currently draw. Minutes recency is about WHEN
// within a season to weight, and it removes a staleness bias — a dropped player
// reading as an ever-present. This is about WHICH SEASON to trust, and what a thin
// season gets wrong is the player's rate, not his minutes. His minutes are the one
// thing the thin season records correctly: he really was injured. Blending old
// minutes in therefore imports a falsehood, which is exactly what the field-ordering
// column shows. ⚠️ Written after seeing the result; it is a mechanism to test, not
// one that was confirmed.
//
// ## The tail resolves, in the FAVOURABLE direction, and it is a level statistic
//
// Recorded because a verdict of "the affirmative case does not exist" cannot omit the
// one family of statistics that resolves decisively the other way, in the same CSV.
// Signed error over the twenty highest-predicted players INSIDE the treated
// population, paired and season-clustered, repaired load:
//
//	arm          delta tail      t    seasons   Holm(8)
//	0.125          +0.00113    1.86   +++++-     0.245
//	0.25           +0.02827    4.68   ++++++     0.016
//	0.5            +0.08372    7.51   ++++++     0.0026
//	1              +0.14665   11.78   ++++++     0.0006
//	2              +0.19539   10.62   ++++++     0.0008
//
// Shipped sits at -0.3828 and every dose moves toward zero without overshooting
// (-0.3816 / -0.3545 / -0.2990 / -0.2361 / -0.1874). Monotone across five settings,
// unanimous across six seasons, Holm-surviving — the three things this record accepts
// in place of an argmax.
//
// **And it is not independent evidence of information.** A signed error over the top
// of a distribution is a LEVEL statistic, so it is exactly what the rescaling that
// within-population ordering was chosen to discount produces: the treated players were
// under-predicted, every dose raises them, the tail error shrinks. It belongs in the
// record with that explanation attached rather than left for the next reader to find
// in ten seconds and conclude it was buried.
//
// ## Mediator
//
// 75.1 to 86.1 per cent of the injury-shaped player-gameweeks change, by 0.088 to
// 0.228 **predicted** points a gameweek at half-life 1 — the model's own score,
// not realised points — and **0.0 per cent of the absent ones change, in any
// season under any arm**.
//
// ## What the gated re-run is IN-SAMPLE for
//
// The population split was introduced in the same commit that ran the ungated
// benchmark, and the gate was chosen because that split showed the absent half
// behaving badly. The gated run is on the same six seasons. So "unchanged
// populations" is true and is NOT pre-registration: there is no held-out season
// here. The gate's case rests on the mechanism — `shrinkToLeague` is a designed
// answer for a player with no usable history and a two-year-old season is not —
// and the mechanism would stand if the numbers had come out differently.
//
// # What it found (UNGATED — the run that motivated the gate)
//
// Six current seasons (2020-21 through 2025-26), gameweeks 1 to 38, one gameweek
// ahead: 227 gameweek clusters and six season clusters, so five degrees of freedom
// and a critical t of 2.571. Points per gameweek. Bias is predicted minus actual,
// so a NEGATIVE bias is under-prediction; spread is the rest of the error, from
// the identity mse = bias squared plus spread squared. Every t below is the
// SEASON-clustered one, which is the conservative reading and the level the replay
// is forced to work at.
//
//	injury shaped (n = 14,337)   bias     spread   mse vs shipped     bias      spread²
//	shipped                    -0.2244   2.1536   —                  —         —
//	half-life 0.5              -0.1446   2.1494   -0.047  (t -8.20)  (+6.99)   (-4.44)
//	half-life 1                -0.0813   2.1498   -0.060  (t -6.76)  (+7.34)   (-2.40)
//	half-life 2                -0.0377   2.1516   -0.058  (t -3.77)  (+7.47)   (-1.41)
//
//	absent all season (n = 1,381)
//	shipped                    -0.1054   1.5838   —
//	half-life 1                +0.0754   1.6200   WORSE
//
// The two halves of the population move in opposite directions, which is why they
// are never pooled. On the injury-shaped half the under-rating falls by 36, 64 and
// 83 per cent, and — correcting what an earlier version of this comment claimed —
// the spread does not merely hold, it FALLS TOO, carrying about a third of the mse
// gain. Both halves of the error improve, so this is not a bias-for-variance trade
// in either direction. The mse curve has an interior optimum near half-life 1,
// which is a shape rather than an argmax over five swept values.
//
// ⚠️ Two corrections to that last sentence, from review, neither touching the
// figures. There are FOUR points, not five — three arms plus shipped — and the two
// contenders are 0.060 and 0.058 apart with no paired t reported between *them*,
// only against shipped. Downgrade it to "consistent with a shrinkage curve, whose
// location is not resolved". And the bias reduction is near-mechanical in
// DIRECTION: the population is selected on "thin last season", the model is known
// in advance to under-rate it, and any dose that raises predictions on an
// under-predicted population reduces bias until it overshoots. The informative
// parts are that it has not overshot pooled by half-life 2, and that the spread
// falls too — though the spread's own t fades with dose (−4.44, −2.40, −1.41),
// which is the bias-for-variance signature arriving at the large end.
//
// On the absent half the bias crosses zero into OVER-prediction and every error
// figure gets worse. That is the disqualifying condition from the brief being met
// on exactly the population where `shrinkToLeague` fires: shipped gives a player
// with no minutes at all no prior, shrinks him to his position's league rate, and
// the blend replaces that with a season at least two years old.
//
// # The mediator, and the ceiling nobody had noticed
//
// 75 to 81 per cent of the case population's player-gameweeks change, by 0.09 to
// 0.17 points a gameweek, so this is a real intervention rather than a null. Every
// observation that does NOT change is a player FPL had flagged unavailable, whose
// Score is exactly zero under both arms because availabilityFactor multiplies the
// whole of it — about a fifth of the population, and a ceiling on what any prior
// can do for it.
//
// # The ordering, which is what the optimiser consumes
//
// Paired within the gameweek, over the whole field, with nobody dropped:
//
//	arm                        Δ rank correlation   t (gameweek)   t (season)
//	half-life 0.5                    -0.00170          -6.70         -3.57
//	half-life 1                      -0.00225          -6.88         -3.70
//	half-life 2                      -0.00281          -7.12         -3.88
//	half-life 1, injury cases only   -0.00020          -1.05         -0.52
//
// The setting as written makes the model order the field slightly but very
// reliably WORSE, monotonically in the half-life. Restricting it to players who
// actually played some of last season removes about ninety per cent of that and
// leaves a residual indistinguishable from zero on both clusterings — the same
// field, the same players, one gate changed. The signed error over the twenty
// highest-predicted players moves slightly TOWARD zero (+0.2206 shipped to +0.2016)
// and does not resolve, so nothing here says the top of the distribution got more
// over-rated.
//
// ⚠️ That fourth row is the arm the gate reproduces, and the reproduction is exact
// — see the block at the top. Do NOT read the first three rows as what
// prior_half_life does now: they are what it did before the gate, and the gated
// ladder runs +0.00033 / −0.00020 / −0.00075.
//
// # What this does not say
//
// Nothing about points. A rank correlation has no conversion into a season total
// on this harness, and the project's hardest-won result is that a better predictor
// can make a worse policy. The replay is the only thing that prices a change, and
// prior_half_life is currently unwired on every replay path: FPL_WEIGHT sets
// Weights.PriorHalfLife, which only the live element-summary path reads, while the
// replay reads SimConfig.PriorHalfLife and populates SimConfig.OlderPriors nowhere
// outside this command and one test.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"armband/internal/analysis"
	"armband/internal/backtest"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/priors"
)

// The grid, and the era hazard it was nearly built around.
//
// The RAW archive carries no expected goals at all before 2022-23 — 2018-19
// through 2021-22 read exactly 0.0 league-wide against 1,068 to 1,199 afterwards.
// That looks like a trap for this experiment specifically: the blend mixes the
// prior season's rates with an older season's, so an older season recorded at
// zero expected goals would drag a player's expected goals toward zero, and the
// arm would then differ from shipped for a reason with nothing to do with the
// feature.
//
// It is not a trap, because backtest.Load already repairs it. xgrepair.go
// backfills 2018-19 through 2021-22 from Understat — the weekly rows AND, through
// rebuildXGAggregates, the season TOTALS, which are exactly what the prior index
// reads — plus 2022-23's missing first fifteen gameweeks. So every season this
// tool loads carries expected goals, and there is one grid rather than two.
//
// The first version of this file had two grids, split on an era boundary the
// loader had already removed, and it discarded 2023-24 to honour the split. The
// verification behind it was made against the CACHED JSON, which is written
// before the repair is applied: the right question asked of the wrong artefact.
// Recorded because it is this project's signature failure arriving somewhere new
// — check what the code loads, never what the file on disk holds.
//
// One channel genuinely is still absent and is worth naming. XGC is deliberately
// NOT repaired, so per-player goals conceded really is zero in the pre-2022-23
// priors — identically in both arms, so it cancels rather than biasing anything.
//
// ⚠️ **Both halves of that sentence are now false, and the second was never sound.**
// `applyXGCRepair` shipped at `7cb769e` and fills exactly those seasons, so the
// channel is no longer absent. And it never did cancel: "identical in both arms"
// confused *the same archive hole* with *the same effect on both arms*, which is the
// kind of error a paired design is supposed to prevent and this one did not.
//
// ⚠️ **The first explanation given for that — "only the BLEND arm reaches back into
// the pre-2022-23 seasons" — is itself WRONG for half the grid, and review caught
// it.** For current seasons 2020-21, 2021-22 and 2022-23 the *immediate* prior is
// itself a pre-2022-23 season, so the shipped baseline is repaired too. Measured on
// the shipped arm's own within-population rank correlation, repaired minus
// unrepaired: **+0.00492 / +0.00419 / -0.00202** in those three, and **exactly
// 0.00000** in 2023-24, 2024-25 and 2025-26. So the asymmetry is real only in
// 2023-24 and 2024-25; in 2022-23 the paired difference moves mainly because the
// BASELINE got worse. The repair does not leave the shipped arm untouched — it
// changes both arms and by different amounts, which is a weaker and more accurate
// statement than the one first recorded here.
//
// ⚠️ **And the diff-in-diff is carried by one season.** Per season, (arm - shipped)
// repaired minus unrepaired at half-life 1: -0.00267 / -0.00361 / +0.00128 /
// **+0.01142** / +0.00406 / 0.00000. Two seasons run the other way and 2025-26 is
// **structurally inert** — the repair reaches none of it — which by this record's
// own rule is not a tie. Read the repair contrast as five live seasons with 2023-24
// dominant.
//
// Measured rather than argued, by running this command twice on the same code with
// `FPL_NO_XGC_REPAIR` set and unset. Δ rank correlation within the injury-shaped
// population, season-clustered, six seasons:
//
//	arm                repair off            repair on
//	half-life 0.125    +0.00063 (t +1.23)    +0.00044 (t +1.15)
//	half-life 0.25     +0.00247 (t +3.14)    +0.00233 (t +2.90)
//	half-life 0.5      +0.00298 (t +3.10)    +0.00402 (t +2.81)
//	half-life 1        +0.00158 (t +0.84)    +0.00332 (t +1.99)
//	half-life 2        -0.00110 (t -0.40)    +0.00213 (t +1.21)
//	channel: minutes   +0.00025 (t +0.14)    +0.00018 (t +0.11)
//	channel: rates     +0.00126 (t +1.72)    +0.00234 (t +3.28)
//
// The repair-off column reproduces the pre-repair run to the last printed digit,
// which is what licenses reading the two columns as one changed thing.
//
// Two readings, and only the first is supported. **The ladder's decay at high dose
// is a repair artefact**: half-life 2 was negative and is now positive, so "more of
// this feature orders the treated players worse" was partly a statement about the
// archive rather than about the feature. **The channel split moves the way a
// mechanism predicts** — expected goals conceded is a RATE, so repairing it should
// move the rates channel and leave the minutes channel alone, and the rates arm
// gains +0.0011 while the minutes arm moves -0.00007. ⚠️ That mechanism was written
// down AFTER seeing the split, so it is a hypothesis the next run can test and not
// a prediction this one confirmed.
//
// 2018-19 is prior-only (the archive publishes no teams.csv for it) so it can be
// an older season but never a current one. 2019-20 is left out as a CURRENT
// season and kept as a prior: it is the season COVID stopped for three months,
// with frozen prices and renumbered restart gameweeks, and predicting through
// that measures the pandemic rather than the setting.
var currentSeasons = []string{
	"2020-21", "2021-22", "2022-23", "2023-24", "2024-25", "2025-26",
}

// oldestSeason is the earliest season a blend may reach back to. The archive's
// floor, not a judgement.
const oldestSeason = "2018-19"

// seasonsBack is how many seasons before the immediate prior are offered to the
// blend. Two, so every cell is offered the same depth and depth is not confounded
// with season — a cell with three older seasons pulls harder than one with one,
// and that difference would read as a season effect.
const seasonsBack = 2

// The populations. Each is a filter on the player, fixed before any prediction is
// made and computed from the prior seasons alone, so nothing here is selected on
// the model or on the outcome.
//
// The names are spelled out because they are printed, and they are chosen so that
// no name is a substring of another: stats/prediction_inference.R selects a
// population by fixed substring and fails when a selector matches more than one.
// The populations were nested — everyone contains reached, reached contains the
// case — and UNDER THE GATE THEY ARE NOT, which matters here because this is
// where the R-selector contract is stated. `popAbsent` is inside `popCase` and
// also inside `popControl`, because the gate declines those players, so `popCase`
// is no longer a subset of `popReached`. Everything else holds: the case still
// splits into injury-shaped and absent, and premium still sits inside
// injury-shaped.
//
// Nothing downstream breaks on the overlap — `emitGameweek` writes one row per
// population and no name is a substring of another — but a reader who assumes
// the nesting will double-count.
//
// `reached` and `the case` differ, and the difference is the whole reason both
// are here. The SETTING reaches every player with a thin prior and any older
// season at all — including a fringe player with 300 minutes last year and 400
// the year before, for whom blending changes very little and means very little.
// THE CASE is what the feature was written for and what the work queue counted: a thin
// last season with a **full** one behind it. Reporting only the first would
// dilute the effect with players nobody is arguing about; reporting only the
// second would hide what turning the setting on actually does to the squad pool.
const (
	popEveryone = "every registered player whose club played"
	// popFieldSound is the field with the absent players removed, and it exists
	// to answer one question: the whole-field ordering gets slightly worse when
	// the setting is on, and this asks whether that is the absent players' doing.
	// It is the field an "injury cases only" version of the feature would order,
	// which cannot be built here — the gate lives in newPriorIndexMulti — but can
	// be measured, because dropping those players from the field is exactly what
	// not blending them would do to this statistic.
	popFieldSound = "the field with the absent players dropped from it"
	popReached    = "reached: the setting changes this player's prior at all"
	popCase       = "the case: thin last season, a full season behind it"
	popInjury     = "the case, injury shaped: he played some of last season"
	popAbsent     = "the case, absent: no minutes at all last season"
	popPremium    = "the case, injury shaped, older season worth 4.0+ points per 90"
	popControl    = "control: the setting cannot reach this player"
)

var populationOrder = []string{
	popEveryone, popFieldSound, popReached, popCase, popInjury, popAbsent,
	popPremium, popControl,
}

// premiumPer90 is the bar for "his older season was actually good", in FPL points
// per 90 minutes. Asserted, not measured: it is the work queue's own figure, chosen so
// the subset is roughly the players a squad would consider, and it is a reporting
// split rather than a constant anything runs on.
const premiumPer90 = 4.0

// Category labels, copied from the benchmark's schema so one R script reads both
// files. Defined by the REALISED outcome, which conditions on the outcome and
// therefore flatters a noisier predictor in the extreme buckets.
const (
	catZeros   = "Zeros: recorded no minutes"
	catBlanks  = "Blanks: played, 2 points or fewer"
	catTickers = "Tickers: 3 or 4 points"
	catHaulers = "Haulers: 5 or more points"
	catAll     = "all categories"
)

func returnCategory(minutes, points int) string {
	switch {
	case minutes == 0:
		return catZeros
	case points <= 2:
		return catBlanks
	case points <= 4:
		return catTickers
	default:
		return catHaulers
	}
}

// tailSize is how many of the highest-predicted players a gameweek's tail figure
// covers, matching the benchmark: fifteen is a squad, so the top twenty is
// roughly the set a transfer search chooses between.
const tailSize = 20

func main() {
	var (
		cacheDir  = flag.String("cache", ".cache/fpl", "archive cache directory")
		cfgPath   = flag.String("config", "config.json", "shipped config; the weights every arm starts from")
		csvPath   = flag.String("csv", "", "write the prediction CSV here (schema: stats/prediction_inference.R)")
		halfLives = flag.String("half-lives", "0.5,1,2", "prior_half_life settings to run beside shipped")
		firstGW   = flag.Int("first-gw", 1, "first gameweek predicted; 1 includes the pre-season view")
	)
	flag.Parse()

	// The shipped config, not config.Default(). A diagnostic that measures the
	// defaults while reporting the shipped settings is the silence this project
	// has already shipped once.
	if _, err := os.Stat(*cfgPath); err != nil {
		log.Fatalf("config not readable at %s: %v\nEvery figure would otherwise come "+
			"from config.Default() with nothing to say so.", *cfgPath, err)
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("loading %s: %v", *cfgPath, err)
	}
	// The baseline arm has to be the shipped model. Note the shipped value that
	// matters to a REPLAY is SimConfig.PriorHalfLife, which no config file can
	// set — see the note on the wiring gap in the README of this experiment — so
	// this checks the field a human would have edited.
	if cfg.Weights.PriorHalfLife != 0 {
		log.Fatalf("config sets prior_half_life to %v; the baseline arm would not be "+
			"the shipped model", cfg.Weights.PriorHalfLife)
	}

	arms, err := parseHalfLives(*halfLives)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	seasons := map[string]*backtest.Season{}
	load := func(name string) *backtest.Season {
		if s, ok := seasons[name]; ok {
			return s
		}
		s, err := backtest.Load(ctx, *cacheDir, name)
		if err != nil {
			log.Fatalf("loading %s: %v", name, err)
		}
		seasons[name] = s
		return s
	}

	fmt.Print(intro(*firstGW))

	sink, err := openSink(*csvPath)
	if err != nil {
		log.Fatal(err)
	}
	defer sink.close()

	for _, curName := range currentSeasons {
		cur := load(curName)
		if err := cur.PlayableAsCurrent(); err != nil {
			log.Fatalf("%v", err)
		}
		prior := load(seasonBefore(curName))
		older := olderSeasons(curName, load)
		runPair(cfg, cur, prior, older, arms, *firstGW, sink)
	}
	if path := sink.path(); path != "" {
		fmt.Printf("wrote %s\n", path)
	}
}

func intro(firstGW int) string {
	return fmt.Sprintf(`PRIOR-BLEND PREDICTION BENCHMARK

What prior_half_life does to the model's own one-gameweek-ahead prediction, on
the players it can reach.

The arms differ in one field of SimConfig: PriorHalfLife, plus the older seasons
it needs to have anything to blend. Everything else — the weights, the recency
index, the team-form source — is what ships, and the engine is built by
backtest.EngineAt so it is wired exactly as the replay wires it.

Predictions start at gameweek %d. The standard benchmark starts at 6 because its
naive baselines need five gameweeks of history; there is no naive baseline here,
and gameweeks 1 to 5 are where a prior is most of the estimate, so starting later
would measure the setting where it matters least.

Every figure below is a MEAN or a COUNT. No standard error, no t, no verdict:
inference is stats/prediction_inference.R's job.

`, firstGW)
}

// parseHalfLives turns the flag into arms, rejecting a zero because that is the
// baseline and two baselines would break the pairing in R.
func parseHalfLives(s string) ([]float64, error) {
	var out []float64
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil, fmt.Errorf("half-life %q: %w", f, err)
		}
		if v <= 0 {
			return nil, fmt.Errorf("half-life %v is the shipped baseline, which is added "+
				"automatically; listing it again would give R two baselines", v)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no half-lives given")
	}
	return out, nil
}

// seasonBefore turns "2024-25" into "2023-24".
// seasonBefore is backtest.PriorSeasonName with this binary's error policy: an
// unparseable season name is an operator mistake in an experiment, so it stops.
//
// It was a third implementation of the same parse, and the three disagreed on
// malformed input while agreeing on everything well-formed — see the note on
// `cmd/armband`'s copy.
func seasonBefore(name string) string {
	prior, err := backtest.PriorSeasonName(name)
	if err != nil {
		log.Fatalf("season name %q is not YYYY-YY: %v", name, err)
	}
	return prior
}

// olderSeasons is the seasons offered to the blend behind the immediate prior,
// most recent first, stopping at the archive's floor.
func olderSeasons(curName string, load func(string) *backtest.Season) []*backtest.Season {
	var out []*backtest.Season
	name := seasonBefore(seasonBefore(curName))
	for i := 0; i < seasonsBack; i++ {
		if name < oldestSeason {
			break
		}
		out = append(out, load(name))
		name = seasonBefore(name)
	}
	return out
}

// reach is what the setting can do to one player, decided before any prediction.
type reach struct {
	pops []string // every population he belongs to
	// priorMinutes and blendedMinutes are the mediator: what the setting
	// actually changes about the prior it feeds the model.
	priorMinutes   int
	blendedMinutes int
	// zeroPrior marks a player the setting reaches who recorded NO minutes last
	// season. Shipped gives him no prior at all and shrinks him to the league;
	// the arm hands him a season that is at least two years old. He is the
	// mechanism the population split exists to separate, and he is excluded from
	// popFieldSound whether or not his older season was a full one.
	zeroPrior bool
}

// classify sorts every player in the current season into populations, from the
// prior seasons alone.
//
// A player is REACHED when the model would actually blend for him:
// analysis.ShouldBlendPrior says yes about his last season — thin, and not zero —
// and at least one offered older season records minutes. That predicate is called
// rather than restated, so this classifier cannot drift from the three
// implementations of the gate the way a fourth copy of the rule could.
//
// A player absent from the prior season entirely is not reached either, because
// the blend iterates the prior season's codes — so he has no prior in either arm
// and shrinkToLeague handles him identically in both.
//
// # The case populations are deliberately NOT gate-aware
//
// popCase, popInjury and popAbsent are defined from the prior seasons alone and
// keep the definitions they had before the gate existed, so the figures they
// produce are comparable with the ungated run rather than being a different
// population wearing the same name. Under the gate popAbsent no longer moves at
// all, and that showing up as an exact zero is the result, not a bug.
func classify(cur, prior *backtest.Season, older []*backtest.Season, halfLife float64) map[int]reach {
	priorByCode := prior.ByCode()
	olderByCode := make([]map[int]*backtest.Player, len(older))
	for i, s := range older {
		olderByCode[i] = s.ByCode()
	}

	out := map[int]reach{}
	// store adds the derived populations that are defined by exclusion, so every
	// exit below assigns them and none can forget to. popFieldSound is the field
	// minus the absent players, which is what an injury-cases-only version of the
	// feature would leave behind.
	store := func(id int, r reach) {
		if !r.zeroPrior {
			r.pops = append(r.pops, popFieldSound)
		}
		out[id] = r
	}
	for _, p := range cur.Players {
		if p.Code == 0 {
			continue
		}
		r := reach{pops: []string{popEveryone}}
		q, inPrior := priorByCode[p.Code]
		if !inPrior || !couldReachTheCase(q.Minutes) {
			r.pops = append(r.pops, popControl)
			store(p.ID, r)
			continue
		}
		r.priorMinutes = q.Minutes

		// The blend as BlendPriors computes it, so the mediator reports the
		// number the model is actually handed rather than an estimate of it.
		hist := []analysis.PriorSeasonStats{}
		if q.Minutes > 0 {
			hist = append(hist, seasonStats(prior, q, 0))
		}
		var best float64 // the best FULL older season's points per 90
		var full bool    // did any older season reach priors.ThinSeason minutes
		for i, m := range olderByCode {
			o, ok := m[p.Code]
			if !ok || o.Minutes == 0 {
				continue
			}
			hist = append(hist, seasonStats(older[i], o, i+1))
			if o.Minutes < priors.ThinSeason {
				continue
			}
			full = true
			if per90 := float64(o.TotalPoints) * 90 / float64(o.Minutes); per90 > best {
				best = per90
			}
		}
		// zeroPrior drives popFieldSound, and its definition is pinned to what the
		// UNGATED run used so the two files describe the same set of players. The
		// `len(hist) >= 1` half is what makes it so: before the gate this line sat
		// after the control exit, which a zero-minute player passed only if some
		// older season carried minutes, and hoisting it above that exit silently
		// dropped 9,705 more observations from popFieldSound. Nothing quoted
		// depended on it, and one name over two definitions across the two runs
		// whose entire purpose is comparability is the failure this file is a
		// catalogue of.
		//
		// popFieldSound is SUPERSEDED and kept only for that comparison. It was
		// how the ungated run approximated the field an injury-cases-only feature
		// would order, by dropping those players; the gated arm IS that field, so
		// the approximation has nothing left to do.
		r.zeroPrior = q.Minutes == 0 && len(hist) >= 1

		// Reached or not, decided by the shipped gate and by whether there is a
		// second season to blend with. One season is not a blend: BlendPriors of a
		// single season returns that season, so the arms would be identical.
		//
		// analysis.ShouldBlendPrior is called rather than restated. Before the gate
		// existed this line read `q.Minutes > 0 && len(hist) < 2`, which let a
		// player with no minutes at all be reached by a single older season — and
		// the census then reported "Lössl 0 to 2777" for players the model now
		// leaves entirely alone. A diagnostic carrying its own copy of the rule it
		// is checking is this project's most expensive recurring bug.
		blended := analysis.ShouldBlendPrior(q.Minutes) && len(hist) >= 2
		if blended {
			r.blendedMinutes = analysis.BlendPriors(hist, halfLife).Minutes
		} else {
			// What he is handed instead, so the census reports the model's answer
			// and not a hypothetical: his own thin season, or nothing at all.
			r.blendedMinutes = q.Minutes
		}
		if blended {
			r.pops = append(r.pops, popReached)
		} else {
			// popControl is "the setting does not change this player", and under
			// the gate it OVERLAPS popAbsent rather than being disjoint from it.
			// That overlap is the finding stated as a population: an absent player
			// is now in the control, and the control's standing check — every arm
			// byte-identical to shipped there — applies to him.
			r.pops = append(r.pops, popControl)
		}
		// popCase and its two halves are defined from the prior seasons alone and
		// are NOT gate-aware — see the note on this function. A player in popAbsent
		// is now, by construction, identical under every arm, and the R output
		// reading exactly zero for him is the measurement.
		if full {
			r.pops = append(r.pops, popCase)
			if q.Minutes == 0 {
				r.pops = append(r.pops, popAbsent)
			} else {
				r.pops = append(r.pops, popInjury)
				if best >= premiumPer90 {
					r.pops = append(r.pops, popPremium)
				}
			}
		}
		store(p.ID, r)
	}
	return out
}

// couldReachTheCase is "his last season was not a full one" — the upper half of
// the bar, and the only thing the classifier's first exit needs to decide.
//
// It reads the shipped predicate rather than restating `>= priors.ThinSeason`,
// which is equivalent only while the bar's COMPARISON matches as well as its
// value: flip ShouldBlendPrior to `<=` and a player at exactly 1,710 minutes is
// filed as "the setting cannot reach this player" by the census while the model
// blends him. That is the census lying about the model, which is the class of
// defect this file has already shipped once.
//
// Zero minutes passes, because popAbsent still has to be classified — the gate
// declines him and the measurement of that decline is the point.
func couldReachTheCase(lastSeasonMinutes int) bool {
	return lastSeasonMinutes == 0 || analysis.ShouldBlendPrior(lastSeasonMinutes)
}

// seasonStats is the blend input for one archived season, capability included.
//
// ⚠️ It carried no capability flags until 2026-08-14, and that is worse than it
// sounds: A3 taught the *replay* to set them and left this binary behind, so the two
// disagreed about what a season measured — the exact defect A3 exists to close,
// re-created inside the commit closing it. Going through backtest.PriorStatsFrom,
// which takes the capability as a required argument, is what makes that unspellable
// rather than merely fixed.
//
// The caller hoists CapabilityOf out of its loop: the probe is an O(players) scan.
func seasonStats(s *backtest.Season, p *backtest.Player, ago int) analysis.PriorSeasonStats {
	return backtest.PriorStatsFrom(p, ago, backtest.CapabilityOf(s))
}

// injuryOnlyPriors is the prior index for the arm that blends an older season in
// for injury-shaped cases and leaves everybody else exactly as shipped.
//
// # Why this arm is the point
//
// The setting as written reaches two populations that behave oppositely. A player
// with a thin but non-zero last season is helped: his bias shrinks and his spread
// does not move. A player who recorded NO minutes at all is harmed: shipped
// routes him to shrinkToLeague, which is a defensible answer, and the arm replaces
// it with a season at least two years old. Whole-field ordering gets worse with
// the setting on, and dropping the second group from the field flips that — but
// dropping players from a field is not the same experiment as not blending them,
// because rank correlation is not decomposable over a partition. This arm IS that
// experiment: the same field, the same players, one gate changed.
//
// # How it is checked
//
// It rebuilds the whole prior index rather than patching one, which is a second
// expression of newPriorIndexMulti and therefore exactly the bug class this
// project keeps shipping. It is checked rather than trusted: the control
// population — every player the setting cannot reach — is emitted for every arm,
// and this arm must come back byte-identical to shipped there. A rebuild that got
// the untouched majority wrong would move that population, loudly, in the table
// the R script already prints.
func injuryOnlyPriors(cur, prior *backtest.Season, older []*backtest.Season,
	halfLife float64) analysis.PriorSeason {

	byCode := make([]map[int]*backtest.Player, 0, len(older)+1)
	byCode = append(byCode, prior.ByCode())
	for _, s := range older {
		byCode = append(byCode, s.ByCode())
	}

	m := map[int]*analysis.PriorPlayer{}
	for code, q := range byCode[0] {
		// Shipped's rule first, and unconditionally: a player with no minutes
		// last season gets no prior at all, exactly as prior_half_life 0 leaves
		// him, so shrinkToLeague still handles him. That single line is the whole
		// of the difference between this arm and prior_half_life 1.
		if q.Minutes == 0 {
			continue
		}
		// The predicate rather than `>= priors.ThinSeason`, for the reason given
		// on couldReachTheCase: a restated bar tracks the model only while the
		// comparison matches as well as the value. This arm's entire claim is
		// that it is the same rule the model runs, so it must not carry its own
		// copy of it.
		if !analysis.ShouldBlendPrior(q.Minutes) {
			p := priorOf(q)
			m[code] = &p
			continue
		}
		hist := []analysis.PriorSeasonStats{seasonStats(prior, q, 0)}
		for i := 1; i < len(byCode); i++ {
			if o, ok := byCode[i][code]; ok && o.Minutes > 0 {
				hist = append(hist, seasonStats(older[i-1], o, i))
			}
		}
		var p analysis.PriorPlayer
		if len(hist) < 2 {
			p = hist[0].PriorPlayer
		} else {
			p = analysis.BlendPriors(hist, halfLife)
		}
		if p.Minutes > 0 {
			m[code] = &p
		}
	}
	return codePriors(m)
}

// priorOf is the archive player as the model's prior type. One expression, used
// by both the classifier and the injury-only index.
// channelPriors splits the blend into its two channels, so the question "which
// half of this is doing the work" can be answered rather than reasoned about.
//
// # Why the split is a rescaling and not a second blend
//
// BlendPriors moves two things. Minutes and starts are recency-weighted. The rate
// totals are recency-AND-minutes weighted and then rescaled by
// `blendedMins / rateW`, so that `per90(XG, Minutes)` returns the rate that was
// actually estimated. The rate therefore survives the rescaling and the totals do
// not mean anything on their own.
//
// So to hold one channel at shipped while the other moves, the totals have to be
// re-expressed against the minutes being kept:
//
//   - minutes only: take the blended minutes, and scale the SHIPPED rate totals so
//     their per-90 is unchanged — `XG * blendedMins / shippedMins`;
//   - rates only: keep the shipped minutes, and scale the BLENDED rate totals the
//     same way, so the blended per-90 arrives on an unblended minutes base.
//
// Both arms are otherwise identical to the gated blend, including the gate itself,
// so a difference between them is the channel and nothing else.
//
// The motivation is measured rather than assumed: on the one channel with ground
// truth, the blend adds a near-constant +3.36 minutes a gameweek while the error it
// corrects ranges from +1.25 to −3.16 across seasons. That is a fixed-size shift
// rather than a correction sized to the error, and if the harm is confined to the
// minutes channel then blending rates alone may keep the benefit without it.
func channelPriors(cur, prior *backtest.Season, older []*backtest.Season,
	halfLife float64, minutesChannel bool) analysis.PriorSeason {

	byCode := make([]map[int]*backtest.Player, 0, len(older)+1)
	byCode = append(byCode, prior.ByCode())
	for _, s := range older {
		byCode = append(byCode, s.ByCode())
	}

	m := map[int]*analysis.PriorPlayer{}
	for code, q := range byCode[0] {
		// The gate, unchanged: an absent player gets no prior at all in either
		// arm, so shrinkToLeague still handles him and the channels are compared
		// on the population the feature actually reaches.
		if q.Minutes == 0 {
			continue
		}
		shipped := priorOf(q)
		if !analysis.ShouldBlendPrior(q.Minutes) {
			m[code] = &shipped
			continue
		}
		hist := []analysis.PriorSeasonStats{seasonStats(prior, q, 0)}
		for i := 1; i < len(byCode); i++ {
			if o, ok := byCode[i][code]; ok && o.Minutes > 0 {
				hist = append(hist, seasonStats(older[i-1], o, i))
			}
		}
		if len(hist) < 2 {
			m[code] = &shipped
			continue
		}
		blended := analysis.BlendPriors(hist, halfLife)
		if blended.Minutes <= 0 || shipped.Minutes <= 0 {
			continue
		}
		// ⚠️ Both arms rescale EVERY rate, DefCon included. It was omitted from both
		// until 2026-08-14 — the fifth and sixth copies of the field list A3 unified,
		// and the two the first pass did not reach. The control they are compared
		// against is `priorOf`, which does carry it, so from 2026-27 a defcon-shaped
		// difference would have been attributed to the channel.
		//
		// The two arms are exact mirrors, so they are one function called twice
		// rather than two field lists. Omitting a field from *one* of two copies is
		// precisely the failure above, and with one copy it is unspellable.
		p := blended
		if minutesChannel {
			p = graftRates(blended, shipped) // blended minutes, shipped rates
		} else {
			p = graftRates(shipped, blended) // shipped minutes, blended rates
		}
		if p.Minutes > 0 {
			m[code] = &p
		}
	}
	return codePriors(m)
}

// priorOf projects an archived season for this experiment.
//
// It delegates to `backtest.PriorFrom` rather than repeating the field list. It was
// the fourth copy of that list and, like the other three, it omitted `DefCon` — so
// this binary's ordering statistics were computed against a prior that differed from
// the live path's by one statistic, for reasons nobody had chosen.
func priorOf(p *backtest.Player) analysis.PriorPlayer { return backtest.PriorFrom(p) }

// graftRates is one channel arm: take the minutes and starts from one prior and
// every rate from the other, rescaled onto that minutes base so its per-90 is
// untouched.
//
// # Why one function rather than the two it replaces
//
// The two arms of `channelPriors` are exact mirrors — swap which prior supplies
// minutes and which supplies rates and one becomes the other — so writing them out
// separately meant maintaining the field list twice inside the very experiment
// whose control is a third copy of it. That is not hypothetical here: `DefCon` was
// missing from both arms until 2026-08-14 while `priorOf` carried it, so the
// channel this binary reports would have absorbed a defcon-shaped difference from
// 2026-27 onward.
//
// `k` is computed the same way round in both arms because it always converts the
// rate-holder's totals onto the minutes-holder's base, so the arithmetic is
// bit-identical to the two literals it replaces rather than merely equivalent.
func graftRates(minutesFrom, ratesFrom analysis.PriorPlayer) analysis.PriorPlayer {
	k := float64(minutesFrom.Minutes) / float64(ratesFrom.Minutes)
	return analysis.PriorPlayer{
		Minutes: minutesFrom.Minutes, Starts: minutesFrom.Starts,
		XG: ratesFrom.XG * k, XA: ratesFrom.XA * k, XGC: ratesFrom.XGC * k,
		DefCon: int(float64(ratesFrom.DefCon)*k + 0.5),
		Bonus:  int(float64(ratesFrom.Bonus)*k + 0.5), Saves: int(float64(ratesFrom.Saves)*k + 0.5),
		Yellow: int(float64(ratesFrom.Yellow)*k + 0.5), Red: int(float64(ratesFrom.Red)*k + 0.5),
	}
}

type codePriors map[int]*analysis.PriorPlayer

func (c codePriors) Get(code int) (*analysis.PriorPlayer, bool) { p, ok := c[code]; return p, ok }

// clubGameweeks is how many fixtures each club has in each gameweek: 1 normally,
// 2 in a double, absent when it blanks.
//
// From the fixture list rather than from the presence of a per-gameweek row, and
// that is the whole point: a missing row is then a genuine zero, which is the
// observation this experiment most needs to keep. A player whose prior is blended
// upward and who then does not play is exactly the case where the feature costs
// something, and filtering him out would measure only the half that helps.
func clubGameweeks(s *backtest.Season) map[int]map[int]int {
	out := map[int]map[int]int{}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		for _, t := range []int{f.TeamH, f.TeamA} {
			if out[t] == nil {
				out[t] = map[int]int{}
			}
			out[t][*f.Event]++
		}
	}
	return out
}

// row is one player-gameweek under one arm.
type row struct {
	id       int
	pops     []string
	predPts  float64
	actPts   float64
	predMins float64
	actMins  float64
	category string
}

// arm is one setting of the experiment: a label, the SimConfig that produces it,
// and optionally a prior index that replaces the one the SimConfig would build.
//
// The override exists for one arm and it is the arm worth shipping — see
// injuryOnlyPriors. It is applied AFTER EngineAt, which is the only seam that
// does not require editing internal/backtest.
type arm struct {
	label    string
	cfg      backtest.SimConfig
	override analysis.PriorSeason // nil means use what the SimConfig builds
}

func runPair(cfg config.Config, cur, prior *backtest.Season, older []*backtest.Season,
	halfLives []float64, firstGW int, sink *sink) {

	// A one-gameweek-ahead prediction wants the one-gameweek view of the model:
	// Score averages fixture difficulty over Weights.Horizon gameweeks, and the
	// fixture-load term is confined to the horizon-1 view. Same choice the
	// standard benchmark makes, for the same reason.
	w := cfg.Weights
	w.Horizon = 1

	base := backtest.SimConfig{Weights: w}
	arms := []arm{{label: "shipped: prior_half_life 0", cfg: base}}
	for _, hl := range halfLives {
		c := base
		c.PriorHalfLife = hl
		c.OlderPriors = older
		arms = append(arms, arm{label: fmt.Sprintf("prior_half_life %g", hl), cfg: c})
	}

	played := clubGameweeks(cur)
	// Membership does not depend on the half-life — the gate is "thin prior plus
	// an older season" — so the half-life passed here only sets the mediator's
	// blended-minutes column, and it is the first candidate arm's.
	who := classify(cur, prior, older, halfLives[0])

	// The fourth arm: blend, but only for a player who actually played some of
	// last season. See injuryOnlyPriors for why it is the interesting one and
	// how it is checked.
	const injuryHalfLife = 1
	arms = append(arms, arm{
		label:    "prior_half_life 1, injury cases only",
		cfg:      base,
		override: injuryOnlyPriors(cur, prior, older, injuryHalfLife),
	})

	// The two channel arms. Same gate, same half-life; only which half of the
	// blend is allowed through differs.
	arms = append(arms,
		arm{
			label:    "channel: minutes only",
			cfg:      base,
			override: channelPriors(cur, prior, older, injuryHalfLife, true),
		},
		arm{
			label:    "channel: rates only",
			cfg:      base,
			override: channelPriors(cur, prior, older, injuryHalfLife, false),
		})

	names := make([]string, len(older))
	for i, s := range older {
		names[i] = s.Name
	}
	fmt.Printf("### %s (prior %s, older %s)\n\n", cur.Name, prior.Name,
		strings.Join(names, ", "))
	reportCensus(cur, who)
	reportGateWiring(cur, prior, older, arms, injuryHalfLife)

	// The baseline's predictions, kept per gameweek so the mediator is a paired
	// difference on the identical player-gameweeks rather than a difference of
	// two averages over possibly different sets.
	baseline := map[int]map[int]float64{}
	// The mediator is split by sub-population because the gate's whole claim is
	// that the two halves behave differently, and a pooled count would hide the
	// half that is supposed to have stopped moving. popAbsent reading 0.0% moved
	// is the gate working; anything else is the gate not being wired.
	medPops := []string{popCase, popInjury, popAbsent}
	med := make([]map[string]*mediator, len(arms))
	for i := range med {
		med[i] = map[string]*mediator{}
		for _, p := range medPops {
			med[i][p] = &mediator{}
		}
	}

	for ai, a := range arms {
		for gw := firstGW; gw <= 38; gw++ {
			e, boot := backtest.EngineAt(cur, prior, gw-1, a.cfg)
			if a.override != nil {
				e.Priors = a.override
			}
			rows := collect(cur, e, boot, who, played, gw)
			sink.emitGameweek(a.label, ai == 0, cur.Name, prior.Name, gw, rows)
			if ai == 0 {
				m := make(map[int]float64, len(rows))
				for _, r := range rows {
					m[r.id] = r.predPts
				}
				baseline[gw] = m
				continue
			}
			for _, p := range medPops {
				med[ai][p].add(p, rows, baseline[gw])
			}
		}
	}
	reportMediator(arms[1:], med[1:], medPops)
}

// reportGateWiring checks the model's own gate against this command's hand-built
// one, and it is the check the whole experiment now turns on.
//
// # What it compares and why that is the question
//
// `injuryOnlyPriors` is the by-population arm: a prior index built here, in this
// command, that blends for a thin-but-played last season and leaves everybody
// else exactly as shipped. It is what the benchmark measured before the gate
// existed, and the arm whose ordering result was indistinguishable from zero.
//
// The other side is the index `backtest.EngineAt` builds from a SimConfig with
// `PriorHalfLife` set — the model's own machinery, `newPriorIndexMulti`, wired as
// the replay wires it. With the gate wired into that function the two must now be
// the SAME INDEX, player for player.
//
// If they agree, the by-population result transfers to the shipped code without
// an inferential step: the two arms are not similar, they are identical, so the
// R output for them is the same file twice and any difference in the printed
// figures is a bug in this tool rather than a finding.
//
// If they disagree, that is the finding — the model's gate is not the gate that
// was measured, and every figure below is about something else. Which is why this
// prints a count and named examples rather than a verdict word: it is evidence,
// not a pass mark.
func reportGateWiring(cur, prior *backtest.Season, older []*backtest.Season,
	arms []arm, halfLife float64) {

	var modelCfg *backtest.SimConfig
	for i := range arms {
		if arms[i].override == nil && arms[i].cfg.PriorHalfLife == halfLife {
			modelCfg = &arms[i].cfg
			break
		}
	}
	if modelCfg == nil {
		fmt.Printf("gate wiring: no arm at prior_half_life %g, so the model's gate is "+
			"unchecked this run. Pass -half-lives with %g in it.\n\n", halfLife, halfLife)
		return
	}
	mine := injuryOnlyPriors(cur, prior, older, halfLife)
	// Any gameweek: the prior index does not depend on how much of the current
	// season has been played, so gameweek 1 is as good as any and is cheapest.
	e, _ := backtest.EngineAt(cur, prior, 0, *modelCfg)

	// The prior season's codes and no others. Both indexes only ever emit a code
	// that appears there, so a code found only in an older season is "no prior"
	// on both sides and would be counted as agreement without either index
	// having been asked anything. An earlier version unioned the older seasons in
	// and inflated the denominator with exactly those.
	codes := prior.ByCode()
	name := map[int]string{}
	for _, p := range cur.Players {
		if p.Code > 0 {
			name[p.Code] = p.WebName
		}
	}

	var held, differ int
	var examples []string
	list := make([]int, 0, len(codes))
	for code := range codes {
		list = append(list, code)
	}
	sort.Ints(list) // ranging a map would make the examples change between runs
	for _, code := range list {
		a, aok := mine.Get(code)
		b, bok := e.Priors.Get(code)
		if aok == bok && (!aok || *a == *b) {
			held++
			continue
		}
		differ++
		if len(examples) < 4 {
			who := name[code]
			if who == "" {
				who = fmt.Sprintf("code %d", code)
			}
			examples = append(examples, fmt.Sprintf("%s (by population %s, model %s)",
				who, describePrior(a, aok), describePrior(b, bok)))
		}
	}
	fmt.Printf("gate wiring: %d of %d priors identical between the by-population arm and "+
		"prior_half_life %g as the model builds it", held, held+differ, halfLife)
	if differ == 0 {
		fmt.Print("; the two arms are the same index.\n\n")
		return
	}
	fmt.Printf(", %d DIFFER.\n  %s\n\n", differ, strings.Join(examples, "\n  "))
}

func describePrior(p *analysis.PriorPlayer, ok bool) string {
	if !ok {
		return "no prior"
	}
	return fmt.Sprintf("%d min", p.Minutes)
}

func collect(cur *backtest.Season, e *analysis.Engine, boot *fpl.Bootstrap,
	who map[int]reach, played map[int]map[int]int, gw int) []row {

	out := make([]row, 0, len(boot.Elements))
	for i := range boot.Elements {
		el := &boot.Elements[i]
		p := cur.Players[el.ID]
		if p == nil {
			continue
		}
		if played[el.Team][gw] == 0 {
			continue // his club blanked; there was nothing to predict
		}
		r, ok := who[el.ID]
		if !ok {
			continue
		}
		g := p.GWs[gw] // absent means he recorded nothing, which is a genuine zero

		m := e.Metrics(el)
		// Expected minutes and the rates are per MATCH; the fixture load converts
		// them to per GAMEWEEK, which is the unit the realised figure is in. The
		// recorded units bug arrived from the other direction.
		load := m.FixtureLoad
		if load <= 0 {
			load = 1
		}
		out = append(out, row{
			id:       el.ID,
			pops:     r.pops,
			predPts:  m.Score,
			actPts:   float64(g.Points),
			predMins: m.ExpectedMinutes * load,
			actMins:  float64(g.Minutes),
			category: returnCategory(g.Minutes, g.Points),
		})
	}
	// Fixed order so the tail selection resolves the same way every run.
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// --- the mediator ----------------------------------------------------------

// mediator answers "did the setting change anything at all", which is what
// separates "there is nothing to see" from "this instrument cannot see it".
type mediator struct {
	n       int
	moved   int
	inert   int // predicted exactly zero in both arms: nothing a prior could move
	sumAbs  float64
	sumDiff float64
}

func (m *mediator) add(pop string, rows []row, base map[int]float64) {
	for _, r := range rows {
		if !has(r.pops, pop) {
			continue
		}
		b, ok := base[r.id]
		if !ok {
			continue
		}
		d := r.predPts - b
		m.n++
		if math.Abs(d) > 1e-9 {
			m.moved++
		}
		// A player FPL has flagged unavailable scores exactly zero, because
		// availabilityFactor multiplies the whole of Score. No prior can move
		// that, and this population is disproportionately made of such players
		// by construction — which is a fact about the ceiling on the setting,
		// not a defect in the measurement.
		if r.predPts == 0 && b == 0 {
			m.inert++
		}
		m.sumAbs += math.Abs(d)
		m.sumDiff += d
	}
}

func reportCensus(cur *backtest.Season, who map[int]reach) {
	count := map[string]int{}
	for _, r := range who {
		for _, p := range r.pops {
			count[p]++
		}
	}
	fmt.Printf("%-62s %6s\n", "population", "players")
	for _, p := range populationOrder {
		fmt.Printf("%-62s %6d\n", p, count[p])
	}

	// The largest movements, named and split by mechanism, so the population can
	// be eyeballed against the players it is supposed to contain. Named examples
	// are how a population filter gets caught selecting the wrong people.
	for _, pop := range []string{popInjury, popAbsent} {
		type nm struct {
			name       string
			from, to   int
			difference int
		}
		var list []nm
		for id, r := range who {
			if !has(r.pops, pop) || r.blendedMinutes <= r.priorMinutes {
				continue
			}
			if p := cur.Players[id]; p != nil {
				list = append(list, nm{p.WebName, r.priorMinutes, r.blendedMinutes,
					r.blendedMinutes - r.priorMinutes})
			}
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].difference != list[j].difference {
				return list[i].difference > list[j].difference
			}
			return list[i].name < list[j].name
		})
		if len(list) > 4 {
			list = list[:4]
		}
		fmt.Printf("\nlargest prior-minutes moves, %s\n  ", shortPop(pop))
		if len(list) == 0 {
			fmt.Print("none")
		}
		for i, x := range list {
			if i > 0 {
				fmt.Print("; ")
			}
			fmt.Printf("%s %d to %d", x.name, x.from, x.to)
		}
		fmt.Println()
	}
	fmt.Println()
}

func shortPop(p string) string {
	if i := strings.Index(p, ":"); i > 0 {
		return p[:i]
	}
	return p
}

func reportMediator(arms []arm, med []map[string]*mediator, pops []string) {
	fmt.Printf("%-40s %-24s %9s %8s %8s %12s %12s\n",
		"population", "arm", "obs", "moved", "inert", "mean |diff|", "mean diff")
	for _, pop := range pops {
		for i, a := range arms {
			m := med[i][pop]
			if m == nil || m.n == 0 {
				fmt.Printf("%-40s %-24s %9d %8s %8s %12s %12s\n",
					shortPop(pop), a.label, 0, "-", "-", "-", "-")
				continue
			}
			fmt.Printf("%-40s %-24s %9d %7.1f%% %7.1f%% %12.4f %12.4f\n",
				shortPop(pop), a.label, m.n,
				100*float64(m.moved)/float64(m.n), 100*float64(m.inert)/float64(m.n),
				m.sumAbs/float64(m.n), m.sumDiff/float64(m.n))
		}
	}
	fmt.Println()
	fmt.Println("obs is player-gameweeks in the named population. moved is the share whose predicted")
	fmt.Println("points changed at all. inert is the share predicted at exactly zero under BOTH arms,")
	fmt.Println("which is a player FPL had flagged unavailable — availabilityFactor multiplies the")
	fmt.Println("whole Score, so no prior can move him. diff is the arm's Score minus shipped, in FPL")
	fmt.Println("points per gameweek, so positive means the arm rates him higher.")
	fmt.Println()
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// --- the CSV ---------------------------------------------------------------

// The schema is the prediction benchmark's, byte for byte, so
// stats/prediction_inference.R reads this file with no changes and the two
// instruments produce comparable numbers. See internal/backtest/predictioncsv_test.go
// for why the rows are per-gameweek sufficient statistics rather than per
// observation: the unit of replication is a gameweek, and a paired difference
// between two arms is the difference of their per-cluster sums because both arms
// score the identical observations.
var header = []string{
	"run_id", "variant", "is_baseline",
	"season", "prior_season", "gw",
	"population", "target", "predictor", "category",
	"n", "sum_abs_err", "sum_sq_err", "sum_pred", "sum_act",
	"rank_corr", "tail_signed_err",
}

type sink struct {
	f     *os.File
	w     *csv.Writer
	runID string
}

func openSink(path string) (*sink, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	s := &sink{f: f, w: csv.NewWriter(f), runID: time.Now().UTC().Format("20060102T150405Z")}
	if err := s.w.Write(header); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

func (s *sink) close() {
	if s == nil {
		return
	}
	s.w.Flush()
	s.f.Close()
}

func (s *sink) path() string {
	if s == nil {
		return ""
	}
	return s.f.Name()
}

type errAcc struct {
	n               int
	sumAbs, sumSq   float64
	sumPred, sumAct float64
}

func (a *errAcc) add(pred, act float64) {
	d := pred - act
	a.n++
	a.sumAbs += math.Abs(d)
	a.sumSq += d * d
	a.sumPred += pred
	a.sumAct += act
}

func (s *sink) emitGameweek(variant string, isBaseline bool, season, priorSeason string,
	gw int, rows []row) {
	if s == nil || len(rows) == 0 {
		return
	}
	for _, pop := range populationOrder {
		sel := make([]row, 0, len(rows))
		for _, r := range rows {
			if has(r.pops, pop) {
				sel = append(sel, r)
			}
		}
		if len(sel) == 0 {
			continue
		}
		for _, target := range []string{"points", "minutes"} {
			pick := func(r row) (float64, float64) {
				if target == "minutes" {
					return r.predMins, r.actMins
				}
				return r.predPts, r.actPts
			}
			// The two gameweek-level scalars belong to the points target only and
			// to the all-categories row, which exists exactly once.
			var rankCol, tailCol string
			if target == "points" {
				preds := make([]float64, len(sel))
				acts := make([]float64, len(sel))
				for i, r := range sel {
					preds[i], acts[i] = r.predPts, r.actPts
				}
				if rho, ok := spearman(preds, acts); ok {
					rankCol = strconv.FormatFloat(rho, 'f', 6, 64)
				}
				if v, ok := tailSignedError(preds, acts, tailSize); ok {
					tailCol = strconv.FormatFloat(v, 'f', 6, 64)
				}
			}
			byCat := map[string]*errAcc{}
			for _, r := range sel {
				pred, act := pick(r)
				for _, c := range []string{catAll, r.category} {
					a := byCat[c]
					if a == nil {
						a = &errAcc{}
						byCat[c] = a
					}
					a.add(pred, act)
				}
			}
			cats := make([]string, 0, len(byCat))
			for c := range byCat {
				cats = append(cats, c)
			}
			sort.Strings(cats)
			for _, c := range cats {
				a := byCat[c]
				rank, tail := "", ""
				if c == catAll {
					rank, tail = rankCol, tailCol
				}
				_ = s.w.Write([]string{
					s.runID, variant, strconv.FormatBool(isBaseline),
					season, priorSeason, strconv.Itoa(gw),
					pop, target, "model", c,
					strconv.Itoa(a.n),
					f(a.sumAbs), f(a.sumSq), f(a.sumPred), f(a.sumAct),
					rank, tail,
				})
			}
		}
	}
	s.w.Flush()
}

func f(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }

// spearman is the rank correlation between two equal-length series, with ties
// given their average rank. Reports false when it is undefined — fewer than two
// observations, or one side constant.
func spearman(a, b []float64) (float64, bool) {
	if len(a) != len(b) || len(a) < 2 {
		return 0, false
	}
	ra, rb := ranks(a), ranks(b)
	var ma, mb float64
	for i := range ra {
		ma += ra[i]
		mb += rb[i]
	}
	ma /= float64(len(ra))
	mb /= float64(len(rb))
	var num, da, db float64
	for i := range ra {
		x, y := ra[i]-ma, rb[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da == 0 || db == 0 {
		return 0, false
	}
	return num / math.Sqrt(da*db), true
}

func ranks(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return v[idx[i]] < v[idx[j]] })
	out := make([]float64, len(v))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		avg := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			out[idx[k]] = avg
		}
		i = j + 1
	}
	return out
}

// tailSignedError is the mean of predicted minus actual over the n
// highest-predicted observations — the set an argmax picks from. Positive means
// the top of the predicted distribution is over-rated.
func tailSignedError(pred, act []float64, n int) (float64, bool) {
	if len(pred) != len(act) || len(pred) < n {
		return 0, false
	}
	idx := make([]int, len(pred))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		if pred[idx[i]] != pred[idx[j]] {
			return pred[idx[i]] > pred[idx[j]]
		}
		return idx[i] < idx[j]
	})
	var sum float64
	for _, i := range idx[:n] {
		sum += pred[i] - act[i]
	}
	return sum / float64(n), true
}
