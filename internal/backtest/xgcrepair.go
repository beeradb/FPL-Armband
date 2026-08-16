package backtest

import (
	"os"
	"sort"
)

// # Expected goals conceded, for the seasons that carry none
//
// `expected_goals_conceded` is absent as a *column* from 2018-19, 2019-20, 2020-21
// and 2021-22, and zero for gameweeks 1-15 of 2022-23 — the same boundary as `xG`
// and `starts`, because it is the same event: FPL added a batch of fields in
// December 2022 and never backfilled the weekly history.
//
// That is not a cosmetic hole. `baseXP90` gates **both** the clean sheet and the
// goals-conceded deduction on `XGC90 > 0` (metrics.go:1895 and :1902), so in those
// seasons every defender and keeper is scored with **neither term** — 26-45% of
// their points. The replay still *pays* them correctly, because realised points come
// from the archive; what it gets wrong is who it **picks**.
//
// On the six-season grid that is **18 of 36 cells, not the 12 this record inherited**.
// The played seasons are 2020-21 through 2025-26; the hole is total in 2020-21 and
// 2021-22 and partial in 2022-23, and since `PointInTime` accumulates from GW1, **all
// six** 2022-23 entries see the repaired GW1-15 rows rather than only the early ones.
// The old 12 is a count of *fully* blind played seasons and it does not survive the
// repair reaching a partial hole. Anything acting through the clean sheet was inert in
// all eighteen.
//
// # Why this is a reconstruction and not a harvest
//
// `applyXGRepair` fills xG and xA from Understat, which publishes per-match player
// figures. It declines to fill xGC, and the reason recorded there answers the wrong
// question: it says a prorated club rate would delete the substitution channel the
// per-player figure carries, worth +0.140 / +0.067 / +0.007 pts/gw across
// substitution terciles. True — and the choice was never "per-player figure against
// club rate", it was "reconstruction against **nothing**". A term switched off
// entirely does not carry the substitution channel either.
//
// The chain needs no new data at all, which is what makes this the cheapest real
// repair available:
//
//	repaired player xG  ->  club xG per gameweek  ->  the opponent's xGA  ->  prorate by minutes
//
// A club's expected goals conceded in a match *is* its opponent's expected goals
// scored, by definition, and the archive already carries repaired per-player xG for
// exactly the seasons missing xGC. The fixture list supplies the pairing.
//
// # What is exact and what is approximate
//
//   - **The pairing is exact.** Every fixture names both clubs and its event.
//   - **The prorating step is exact for an ever-present**, trivially: a player who
//     went the full 90 in a single-fixture gameweek faced his club's whole exposure,
//     so there is nothing to prorate. (The supporting figure often quoted here —
//     team-mates who both played 90 agreeing to a median spread of 0.0000 across
//     714-740 team-gameweeks — is the 2026-08-12 audit's and has no producing test in
//     this repository.) ⚠️ **Do not read that as the CHAIN being exact for an
//     ever-present**: measured, that population still carries 3.9% mean absolute error
//     and sits 0.9% high. Two different claims, run together in an earlier version of
//     this comment. ⚠️⚠️ **And 3.9% is the FPL-fed figure — on the input this repair
//     actually runs on it is 16-20%. See the transport block below.**
//   - **The minutes split inside a double gameweek is approximate**, and it is the
//     only place this invents anything. The repair rows are keyed by gameweek rather
//     than by fixture, so a player's two matches in one week cannot be separated and
//     his share is taken across both. Single-fixture gameweeks — the overwhelming
//     majority — are unaffected.
//   - **The substitution channel is preserved in form and not in detail, and the
//     proration is NOT linear in truth.** A player withdrawn at 60 gets 2/3 of the
//     club figure. ⚠️ An earlier version of this line called that "right on average";
//     measured against the real column it is not. Actual xGC over the
//     minutes-prorated share, by minutes played:
//
//	minutes   2023-24  2024-25  2025-26
//	1-29        1.525    1.434    1.637
//	30-59       1.065    1.015    1.007
//	60-89       0.950    0.957    0.924
//
//     So a late substitute really faces about 1.5x his prorated share — games open up —
//     and a withdrawn starter about 0.95x, meaning the reconstruction **over-credits a
//     substituted starter by 4-8% per appearance** and under-credits a late substitute
//     heavily. That is the objection the original refusal raised, it is why every
//     reconstructed row is flagged, and it is the mechanism behind the partial-minutes
//     ratio of 0.9853 in the block below.
//
//     **At player-SEASON level the two errors largely cancel**, which is why this is a
//     caveat and not a defect: for defenders and keepers with 900+ minutes, season
//     `XGC90` reconstructed over actual runs 0.983-1.014 across substitution terciles
//     in all three seasons — within ±1.7%, non-monotone, about 0.015 points per 90
//     through the clean sheet. ⚠️ **Measured FPL-fed; re-measured and transported
//     2026-08-16.** The band reproduces: under a named cut (share of scored minutes not
//     prorated exactly — ⚠️ that is **proration** exposure, not the **substitution**
//     exposure this line's own wording claims; they differ on fully-played double
//     gameweeks, and the transport block below says what that costs) and a named
//     estimator (ratio of totals over per-player season
//     XGC90), arm A spans **0.9857 to 1.0100** on the three full seasons, inside
//     0.983-1.014 and within ±1.7%. Its Understat-fed counterpart does not separate from
//     it — signed high−low contrast +0.0061 against a threshold of 0.0116 — so the
//     transport question is a **tie that leans**, not a confirmation that the two agree.
//     **Read the tercile block at the end of the transport section below** for what that
//     null can and cannot carry, and for why the stake is bounded either way.
//   - **A mid-season transfer is attributed to the wrong club.** `Player.Team` is
//     where a player *finished*, so a January move files his August exposure against
//     his new club's opponents. Counted against the fixture list this is **0.32% to
//     1.17% of appearances, 7 to 15 players a season** — and it occurs at the same
//     rate in the seasons the method is validated on, so it is already inside the
//     reported error rather than being an error on top of it. Closing it needs the
//     club a player actually played for, per gameweek, which `GW` does not carry.
//
// Read `xgcScale` for the one calibrated constant.

// xgcScale is the divisor applied to the club figure before prorating, and it is
// **1: there is no correction, and the calibration this repair was expected to need
// does not exist.**
//
// # The overshoot it was meant to correct is an estimator artifact in its SIZE
//
// The archive audit that proposed this repair recorded "a consistent 1.02-1.04
// overshoot that wants a calibrated offset exactly as the existing xG repair has
// one". Measured by TestDiagXGCReconstruction across 38,973 player-gameweeks in the
// four seasons carrying real xGC, the pooled ratio of reconstructed to actual is
// **0.9994**, spanning 0.9962 to 1.0020 by season.
//
// The difference is the estimator rather than the data. Restricted to the population
// the audit used — single-fixture club-gameweeks read off an ever-present — the same
// runs give a **ratio of totals of 1.009 to 1.014** and a **mean of per-club ratios of
// 1.020 to 1.037**, on 404 to 740 club-gameweeks. A mean of ratios is biased upward
// when the denominator is small and variable, and a club's expected goals in one
// gameweek is exactly that. This project's record already carries this failure once:
// the perfect-price-timing bound moved when it was re-estimated under an
// equal-weighted rather than a gameweek-weighted estimator, and the gap was briefly
// blamed on an unrelated code fix before being traced to the estimator. Twice now.
//
// ⚠️ **What is refuted is the SIZE, not the sign.** On the audit's own population the
// ratio of totals is above 1 in all four seasons, so a consistent overshoot of about
// **1%** survives; what does not survive is 2-4%. Do not read this block as "there is
// no overshoot". And the diagnosis is inference from a value match rather than a
// re-run: the audit's prototype no longer exists, its recorded MAE triple
// (5.1/3.4/5.0) does not reproduce under any assignment of seasons, and the n it
// records belongs to a different check in its own text. Same population to a good
// approximation, not provably the same implementation.
//
// # The pooled 0.9994 is a CANCELLATION, and that is a mechanism rather than a null
//
// Split by population, the two halves sit on opposite sides of 1:
//
//	ever-present, single fixture   n 18,531   ratio 1.0088   corr 0.977   MAE  3.9%
//	partial minutes                n 20,442   ratio 0.9853               MAE 29.7%
//
// ⚠️ **Both rows are FPL-fed. See the transport block below.** The 18,531 is also the
// pooled ever-present count across the four seasons (2,914 + 5,184 + 5,172 + 5,261), so
// a per-season figure quoted from that population is not this number.
//
// So the pooled figure is two biases of opposite sign meeting in the middle, weighted
// by the xGC mass each population carries. **A pooled null built this way would
// survive both errors growing**, which is why the split is reported and the pooled
// number is not the one to read when asking whether the chain is right.
//
// The direction is what linear minutes-prorating predicts against a late-loaded
// danger profile: a player withdrawn at 60 is credited two thirds of his club's
// exposure, and this record's goal-timing finding puts the closing twenty to thirty
// real minutes at roughly 1.2x the match average — so he faced less than two thirds
// and is over-credited, while a late substitute is under-credited. That is
// **consistent with** the recorded timing figure rather than a second measurement of
// it, and it is a hypothesis about the residual rather than a fitted correction.
//
// Note also that the pooled correlation of 0.937 is largely a **minutes** correlation:
// both sides scale with how long a player was on the pitch. The ever-present figure of
// 0.977 is the one that speaks to per-match fidelity.
//
// # Why the constant is 1, and the argmax argument is the WEAKEST of the three
//
//   - **It is not transportable.** `xgcScale` is applied only inside applyXGCRepair,
//     which runs only on the Understat-fed seasons — and every estimate of it comes
//     from the FPL-fed seasons where it will never run. Fitting a constant on one
//     population and applying it to another is precisely what the transport question
//     below forbids. That settles it before anything else is needed.
//   - **The inherited uncertainty is one to two orders of magnitude larger.**
//     `borrowed_ratio` in `stats/understat_xg_backfill.py` records the per-season
//     provider offsets as 1.031 / 1.091 / 1.128 / 1.104 and ships the plain mean,
//     bounding the residual level error at about half that spread — **4-5%** on the
//     backfilled seasons. Reconstructed xGC is *linear* in that offset, so it inherits
//     the same 4-5%. Arguing over 0.9994 against 1.0000 (0.06%) or 1.0088 (0.9%)
//     inside a ±4-5% envelope is not calibration.
//     ⚠️ **That envelope is now MEASURED rather than argued (2026-08-13).** The
//     transport run's leave-one-out borrow misses each season's own in-season ratio by
//     **+7.7% / −0.2% / −4.3% / −2.1%**, so the half-spread reasoning was right in kind
//     and slightly low: 4-5% is the typical case and one season of four exceeds it.
//     Strengthens this bullet; moves no constant. Note the two ratios are different
//     estimators — `borrowed_ratio`'s are 900+-minute harvests, the in-season one is a
//     ratio of totals over the full population — so recomputing +7.7% from the four
//     figures above gives +7.4%, and neither is wrong.
//   - **A uniform xGC scaling barely reorders within a position.** ⚠️ Not "cannot":
//     it preserves the ordering of the clean-sheet TERM, which is monotone in xGC,
//     but `Score` also carries the goals-conceded deduction and `exp(-xGC)` is
//     convex, so two defenders differing elsewhere can swap. The size is what makes
//     it safe — at xGC90 ~ 1.35 a 1% move is about 0.014 pts/90 against a
//     defender's ~4.5. The recorded "a bias shared by every player in a position is
//     not an ordering error" precedent is about an ADDITIVE bias and must not be
//     cited for a multiplicative scaling.
//
// Only after all three does the argmax point apply: fitting 0.9994 would be fitting
// the fourth decimal of a noisy estimate.
//
// # The transport assumption was untestable-looking and was merely untested — RUN
// # 2026-08-13, and it FAILS
//
// The validation runs on seasons whose player xG is **FPL's own**, and the repair runs
// on seasons whose player xG is **Understat's, rescaled by the backfill's provider
// offset**.
//
// ⚠️ Note what is *not* the risk: per-player redistribution inside a club cancels
// exactly, because clubXGPerGameweek sums the whole club before anything else
// happens. The two things that genuinely threaten transport are **club-match-level
// disagreement between the two xG models** and **cross-club misattribution**, since
// both this chain and the validation key on `Player.Team`.
//
// ⚠️ **Run 2026-08-13, and it does not transport. Every fidelity figure above
// describes the FPL-fed seasons and overstates the repaired ones by 4-6x on the sharp
// population and about 1.7x over all appearances.**
// `TestDiagXGCTransport` runs this function on both inputs against the same truth —
// `stats/understat_xg_backfill.py --transport` writes the Understat side, rescaled by
// the leave-one-out borrowed offset. On ever-presents, where the proration is exact by
// construction so the residual is purely the two xG models disagreeing about a club's
// match, mean absolute error goes from **3.0-5.2% to 16.0-20.2%**. Read 2023-24 as the
// clean case: its borrowed offset is wrong by 0.2%, so almost none of its 3.2% → 16.0%
// is a level effect.
//
// **The four paired per-season deltas carry a standard error and it is worth stating**:
// 12.8 / 12.8 / 14.8 / 15.0 points, mean 13.85, season-clustered SE 0.608, **t 22.79 on
// df 3, p 0.0002**, every leave-one-season-out subset under p 0.003. `established`.
// ⚠️ **Do not defend it with a player-row n instead** — a first write-up quoted "n =
// 5,184 cells", which is 2023-24's ever-present count alone rather than the pooled
// 18,531, and the cell axis is wrong regardless: for an ever-present in a single-fixture
// gameweek both the reconstruction and the truth ARE the club figure, so every such row
// in a club-match carries the identical relative error. The independent unit is the
// club-match, ~7.2 cells each.
//
// The **level** moves too — pooled 0.9289 / 0.9983 / 1.0471 / 1.0213 — and it is fully
// explained by what the borrowed offset gets wrong per season (+7.7% / −0.2% / −4.3% /
// −2.1%), **taken as arm B over arm A rather than against 1.0**: done that way the
// residual is under 0.04% in all four seasons, where the absolute baseline leaves
// −0.37% on 2023-24. That agreement doubles as the coverage proof — a crosswalk losing
// mass would show arm B running light, and it does not. Do not read the level as a
// finding: the four ratios average 0.9989 with no consistent sign, so there is nothing
// to correct, and the reason is that it is noise in a borrowed constant rather than a
// bias — not that a shared level is "invisible to an argmax", which is too strong, since
// exp(-xGC) is convex and a shared scaling does move the spread.
//
// The **ordering** — Spearman of reconstructed against actual season xGC90 across
// keepers and defenders with 900+ minutes, the pre-registered discriminator — falls in
// all four seasons, 0.9598→0.9408, 0.9659→0.9446, 0.9830→0.9537, 0.9204→0.8683. The
// sign test is p = 0.0625 and the season-clustered t is −4.03 (df 3, p 0.028), but
// **three of four leave-one-season-out subsets miss 0.05** (0.066 / 0.076 / 0.102 /
// 0.018), so **suggestive**, not established. The subset that fails hardest drops
// 2024-25, so that is the season the ordering claim can least spare.
//
// ⚠️ 2022-23 is scored on **GW16-38 only** — every column of that row, not just the
// Spearman — because `scoreXGCArm` skips rows carrying no real xGC figure. Its
// 900-minute filter therefore selects harder (n 120 against 143-155). Read its *levels*
// beside the other three rather than pooled; its *deltas* are legitimate replicates,
// since the contrast is paired within season.
//
// ⚠️ **The headline statistic is not the pre-registered one.** Spearman was
// pre-registered and came back suggestive; ever-present MAE got the strong grade. That
// is not statistic-shopping — ever-present MAE is the pre-existing validation's own
// statistic (1.0088 at 3.9%), so arm A re-scoring it is a re-run of a committed
// benchmark — but the asymmetry should be stated rather than met by surprise.
//
// ⚠️ **This is not a verdict on whether to keep the repair.** It compares the chain on
// Understat input against the chain on FPL input; what the replay runs is the chain
// against **nothing**, since baseXP90 gates both the clean sheet and the goals-conceded
// deduction on XGC90 > 0. A reconstruction carrying 16-20% per-match error can still be
// a large improvement on a term switched off entirely, and nothing here measures that.
//
// The crosswalk is not the mechanism: it carries 1.0000 / 1.0000 / 1.0000 / 0.9979 of
// FPL's own xG mass over the scored window, with the goals anchor exact on every cell in
// three seasons and 4 cells disagreeing in 2025-26. The residual is club-match-level
// provider disagreement, which is exactly the exposure named above and not the
// per-player one that cancels.
//
// ⚠️ **A thin crosswalk was the one failure this apparatus could not see, and the guard
// for it is new.** An unmapped player gets zero, so partial delivery pushes arm B's
// error UP — producing exactly this finding. The original liveness check compared the
// two arms for float equality, which catches only total delivery failure, and total
// failure was already unreachable because `readTransportXG` fatals on a missing file and
// on zero rows. `TestDiagXGCTransport` now asserts the transported xG **mass** against
// FPL's own over the scored rows. Read the level agreement above as the same check by
// another route.
//
// **This is a provider statistic more than a chain statistic.** The chain is linear in
// club xG, so its relative error simply IS the opponent club's xG relative error: there
// is nothing here to damp a provider gap, which is why the effect is this large.
//
// ## The tercile contrast does not resolve; the recorded band reproduces
//
// Run 2026-08-16, `scoreXGCArm`'s tercile column plus `stats/xgc_tercile_transport.R`.
// This settles the ⚠️ left open on the cancellation bullet above. **No replay: the arm-B
// inputs were already banked under `stats/cells/xgc-transport/`.**
//
// The population and the cut. Keepers and defenders with 900+ scored minutes — the same
// filter the Spearman already uses — split into terciles on the share of a player's
// scored minutes that did *not* arrive on a row the proration got exactly right, meaning
// `minutes == 90*n` for a gameweek with n fixtures. Exposure is a **minutes** property
// and `withTransportXG` replaces xG only, so both arms cut the same players into the same
// buckets and the contrast is paired at player level. n per bucket is 40 to 53.
//
// ⚠️ **This is PRORATION exposure, not SUBSTITUTION exposure**, and the claim being
// re-scored is worded in substitutions. The first cut written here used the ever-present
// predicate for both this and `everN`, which booked a 90+90 double as fully exposed even
// though the proration hands such a player his club's whole two-match xGA exactly.
// That mislabelled 18-71% of the population by season, moved 22% of player-seasons
// between terciles, and manufactured an apparent 2022-23 anomaly — its median exposure
// read 0.256 against 0.137-0.184 elsewhere, and on the corrected predicate reads **0.124
// against 0.120-0.148**, the anomaly gone entirely. Every figure below is the corrected
// cut. **Do not re-merge the two predicates**: they are two questions, and `everN` is
// pinned to the recorded 1.0088 by the positive control.
//
// ⚠️ **18-71% and 22% are NOT reproducible from this checkout**: both need the
// contaminated cut, and only the corrected exposure and tercile are banked. What is
// checkable is `TestTheProrationExposureCutIsNotTheEverPresentCut`'s **213 / 117 / 48 /
// 54** disagreeing player-gameweeks, and the corrected median exposure of **0.124 / 0.133
// / 0.120 / 0.148**. Read the two percentages as the correction's own account of what it
// fixed, not as measurements a reader can re-derive.
//
// **The estimator is the ratio of totals over per-player season XGC90** — every player
// weighted equally in *rate* space, not by minutes, which would be a third quantity
// again. The mean of per-player ratios is printed beside it, because the recorded
// 0.983-1.014 does not say which it was. It runs **−0.08% to +1.10%** higher, and the gap
// is not noise-shaped: it **increases with exposure in 7 of 8 season-arms**, so
// mean-of-ratios *manufactures* tercile structure. That, rather than the generic
// small-denominator argument, is why ratio-of-totals is the headline.
//
// **The FPL-fed control REPRODUCES the recorded band.** On the three full seasons arm A
// spans **0.9857 to 1.0100**, inside 0.983-1.014, at a max deviation from 1 of 1.43% — so
// "within ±1.7%" holds too. Mean-of-ratios spans 0.9897 to 1.0129, also inside. 2022-23
// is the exception at 1.0231, and it is the GW16-38 season the claim's own "all three
// seasons" excludes. ⚠️ **An earlier version of this block declared the band withdrawn.
// That was the contaminated cut above, not the data**, and the withdrawal is itself
// withdrawn. The band stands, now with a named cut and a named estimator behind it.
//
// **Read the transported ratio recentred, not raw.** The UST arm carries a whole-season
// level from the borrowed provider offset — its whole-population ratio is 0.9372 /
// 0.9979 / 1.0504 / 1.0217 against the FPL arm's 1.0109 / 0.9964 / 1.0056 / 1.0016.
// That is the **same level** as the pooled 0.9289 / 0.9983 / 1.0471 / 1.0213 recorded
// above, on a narrower population and a different estimator — rate-space over 900+ minute
// keepers and defenders against level-space over all appearances, 0.8pp apart on
// 2022-23 — so it is not literally "the same finding". Raw, the UST tercile ratios span
// **0.9235 to 1.0555**, and all of that is the offset.
//
//	recentred, tercile ratio / whole-population ratio
//	season     FPL: low    mid   high  |  UST: low    mid   high
//	2022-23       0.9939 0.9942 1.0121 |     0.9855 0.9974 1.0176
//	2023-24       1.0088 1.0024 0.9893 |     1.0073 0.9956 0.9978
//	2024-25       1.0016 1.0044 0.9939 |     0.9997 1.0048 0.9952
//	2025-26       1.0017 1.0054 0.9923 |     0.9992 1.0124 0.9873
//
// ⚠️ **The recentring carries a constraint that costs a degree of freedom.** The
// whole-population ratio is the act90-weighted combination of the three bucket ratios, so
// the three recentred values satisfy an act90-weighted mean of **exactly 1** — asserted in
// the R script, and holding to 2.2e-16 in all eight season-arms. The three buckets are
// therefore **two** degrees of freedom, Holm over six rows is conservative rather than
// wrong, and the single statistic to read is the signed **high−low**. Three bucket tests
// are not three pieces of evidence.
//
// **Every paired UST−FPL contrast is inside its own detection threshold.**
// Season-clustered on four seasons, `thresh` = t_crit(3) × SE = 3.182 × SE: low −0.0036
// (thresh 0.0052), mid +0.0010 (0.0092), high +0.0025 (0.0093), the signed high−low
// **+0.0061 (0.0116)**, the spread +0.0043 (0.0166). Holm 0.579 or above on all five.
//
// ⚠️ **On the three full seasons one row DOES clear its own threshold, and an earlier
// version of this line said nothing did.** `UST−FPL recentred low` reads **−0.0020
// against a thresh of 0.0013**, |t| 6.57 on df 2, raw p 0.0224 — the smallest of five,
// Holm **0.1121**, so it carries nothing alone. It is worth stating because it runs the
// **same sign** as the four-season low (−0.0036), so it is the second leg of the lean
// below rather than an unrelated row.
//
// ⚠️ **It is a tie that LEANS, and an earlier version of this block said "not close".**
// A second construction of the same contrast agrees, which is why the lean is worth
// recording at all. The continuous version — OLS of `log(rec90_ust/rec90_fpl)` on
// exposure, club-clustered within season, then season-clustered across — reads per-season
// slopes **+0.0256 / +0.0232 / +0.0008 / −0.0017**, pooled **+0.0120, SE 0.0072, t 1.66**,
// against a threshold of 0.0229. Times the observed high−low exposure gap of 0.386 it
// implies **+0.0046**, against the tercile contrast's +0.0061.
//
// ⚠️ **The continuous form is NOT sharper than the tercile cut, and an earlier version of
// this line claimed it was.** That was true at the contaminated cut (1.94 against 1.59)
// and is false now: **t 1.66 against 1.68**, effect over threshold 0.524 against 0.526,
// effect over MDE80 0.40 in both. What the second form buys is corroboration, not power.
// ⚠️ And "positive in three of four seasons" counts a 2024-25 slope of **+0.0008**, so
// read the sign count as two seasons and two nulls.
//
// ⚠️ **The lean is a FOUR-season statement and it roughly halves on the three full ones.**
// Dropping 2022-23 — the GW16-38 season held apart everywhere else here — the pooled slope
// is **+0.0074, SE 0.0079, t 0.94**, positive in 2 of 3, and the tercile high−low is
// +0.0035 against a thresh of 0.0155. Note the asymmetry rather than meeting it by
// surprise: **the band reproduction above is quoted on the three-season set and the lean
// on the four.** Both are banked; quote both.
//
// The honest reading: **the point estimate runs toward the transported arm over-crediting
// high-exposure players by about half a percentage point, and this instrument cannot
// resolve an effect that size.**
//
// **What the null can and cannot carry.** It is a tie, so it does not show the two inputs
// behave identically — it shows they cannot be separated by this instrument. What it
// *can* refute is a magnitude: the 80%-power MDE on the signed high−low contrast is
// **0.0151**. ⚠️ **Compare it to the right thing.** The recorded band is 0.031 *wide* but
// ±1.7% *deep*, and the MDE is on a within-season **paired** contrast while the band is a
// range over three seasons and three terciles of a **level** — related quantities, not
// the same one. Taking the width: this comparison would have found the cancellation
// breaking outright, and a degradation of exactly half the band (0.0155) sits
// **marginally inside** 80% power rather than outside it. ⚠️ An earlier version of this
// line said a half-band degradation could not have been seen; 0.0155 > 0.0151, so that
// was arithmetically backwards. Anything materially under half is unreachable.
//
// **And the stake is bounded regardless — but size it against the canary, not in pts/90.**
// ⚠️ The "about **0.015 points per 90**" on the cancellation bullet is **unsourced**: it
// entered in prose with no producing run, and it does not agree with this file's own
// arithmetic 190 lines up, where at `XGC90` ~ 1.35 a **1%** move is 0.014 pts/90 — which
// makes the ±1.7% band ~**0.023** and its full 3.1% width ~**0.042**. **Do not quote
// 0.015 until someone derives it.** ⚠️ That figure was also computed for a **uniform**
// scaling, which this record holds is *not* an ordering error, whereas the tercile effect
// is within-position and exposure-dependent — the case the same record holds *is* one. So
// it is the wrong instrument for this bound twice over.
//
// The bound that does hold is the recorded **canary**: halving *every* clean sheet costs
// **−21.6 a season on `HOLD` against its own threshold of 28**, and a 1.5-3% move in
// `XGC90` is a 2-4% move in the clean-sheet probability (`d ln p / d ln x = −x = −1.35`)
// — under a fiftieth of the canary, so under a point a season before any of it is asked
// to survive an ordering. That is unit-consistent, and it is this record's own
// instruction to size a candidate against a canary. ⚠️ "Two orders below anything the
// replay resolves" is **withdrawn**: it set a per-player pts/90 displacement against a
// per-season squad threshold, which is not a ratio of comparable things. On the canary
// bound, and with the 4-6× per-match error and the ~5% level error above dwarfing it:
// **do not re-run this.**
//
// ⚠️ **Do not read the per-arm `spread` (max − min) rows as evidence.** Max minus min is
// bounded below by zero and inflated by noise, so a t against zero is mechanical — the
// same defect this record already names against the chip-timing differences. FPL-fed it
// reads +0.0153, t 7.19, Holm 0.0222, and that clearance means nothing. The *signed*
// high−low is −0.0046 on four seasons and **−0.0122 (t −3.31, p 0.0805) on the three full
// seasons**, against a thresh of 0.0158. ⚠️ That second figure is **the largest signed
// structure anywhere in this run**, it is the statistic the degrees-of-freedom note above
// nominates as *the* one to read, and it still does not clear — quote both sets rather
// than only the quiet one. The paired spread contrast in the list above is **biased toward
// finding a transport failure** — the UST arm's bootstrap SEs run larger than the FPL
// arm's, **median 1.69×, spanning 0.88 to 2.35 over the twelve buckets and below 1 in one
// of them**, so under a null of identical true structure `E[spread_UST]` exceeds
// `E[spread_FPL]` — and it does not find one. That is the correct defence of it; "the bias
// is common to both arms" was the wrong one and is withdrawn. (An earlier version of this
// line said the SE ratio was "1.5-2×"; four of twelve buckets are below 1.5.)
//
// ⚠️ **One FPL-fed bucket reading is post-hoc and does not survive Holm.** On the three
// full seasons the high-exposure bucket sits 0.81% below its season mean at t −6.01,
// p 0.027 on df 2 — the smallest of six raw p-values, Holm 0.1597. Three further reasons
// it carries nothing: the three-season triple is low +0.0040, mid +0.0041, high
// **−0.0081**, so it is a step at the top bucket rather than the **monotone** gradient a
// substitution-driven bias must produce; by the zero-sum constraint above the high bucket
// is very nearly the arithmetic complement of the other two; and the SE is estimated from
// three numbers on df 2, where `t_crit` is 4.303. The four-season version of the identical
// quantity is **−0.0031, t −0.60**.
//
// ⚠️⚠️ **The FPL-fed gradient runs the WRONG WAY for the mechanism this is testing, and
// that is a fact about the cut rather than about the chain.** The mechanism paragraph at
// the top of this file — and `xgcrepair_test.go`'s copy of the argument — has the
// reconstruction **over**-crediting a withdrawn starter (actual over prorated 0.950 /
// 0.957 / 0.924 at 60-89 minutes), so in a 900+ minute keeper-and-defender population the
// high-exposure bucket should sit **above** 1. It sits **below the low bucket in 3 of 4
// seasons** — raw 2023-24 descends 1.0051 / 0.9987 / 0.9857. Two readings this run cannot
// separate: either the cut is dominated by the *under*-credited leg (short appearances and
// blanks, which is what PRORATION exposure collects and SUBSTITUTION exposure would not),
// or the mechanism is wrong for this population. **So read "the band reproduces" as a band
// of the right width reproducing, not as the cancellation claim being confirmed on its own
// mechanism.** Separating the two needs a minutes-band decomposition of exposure inside
// the 900+ population, and the canary bound above says it is not worth buying.
//
// **Data state.** The archive as loaded, no switches set: `FPL_NO_XG_REPAIR`,
// `FPL_NO_XGC_REPAIR` and `FPL_NO_XG_AGGREGATE` are all unset, and none of the three
// governs these figures — arm A reads FPL's own native xGC column and arm B reads the
// banked transport CSVs, and rows carrying no real xGC figure are skipped by
// `scoreXGCArm`, so nothing here is scored against reconstructed output. The four seasons
// are the ones carrying a real xGC column; 2022-23 is GW16-38 only, so it is reported
// both in and out.
//
// **Liveness, and there are two because the obvious one is on the wrong quantity.**
//
//   - **Raw movement.** The transported reconstruction moves on **570 of 570** scored
//     player-seasons, 494 of them (86.7%) by more than 1%. `TestDiagXGCTransport` fatals
//     below 90% *on the 570-of-570 quantity*, not on the 86.7% one.
//   - ⚠️ **That guard cannot fire against the reading above.** A pure per-season **scalar**
//     rescale of arm A would pass it at 100% while forcing every recentred contrast to be
//     exactly zero *by construction*, since the recentring divides by the same
//     season-arm's whole-population ratio and a scalar cancels. So raw movement cannot
//     tell a real null from a degenerate one — the signature failure wearing the clothes
//     of a confirmation.
//   - **The guard with power** is the within-season dispersion of the per-player arm
//     ratio `rec90_ust/rec90_fpl`, which is 0 by construction under a rescale. Measured:
//     **CV 4.61% / 4.33% / 4.36% / 4.76%**. The R script stops below 0.5%. That is the
//     number to quote beside the null, not the 570 of 570.
const xgcScale = 1.0

// noXGCRepair isolates the xGC reconstruction from the rest of the backfill.
//
// FPL_NO_XG_REPAIR already turns the whole repair off, which answers "what are these
// seasons worth with no expected goals at all". This narrower hatch answers the
// question that has a shipped-grid cell in it: what does giving defenders and keepers
// a clean sheet in 18 of 36 cells actually do — see the header, which corrects the 12
// this record inherited. Those are different arms and a sweep
// needs to be able to run either.
//
// Read on every call rather than cached at process start, matching noXGRepair.
//
// ⚠️ **That per-call read is NOT enough to make this a sweep arm, and the comment here
// used to claim it was.** The switch is re-read on every `Load`, but the *season* is
// memoised: `loadSeason` holds a process-global `seasonCache`, and `runPolicySweep`
// calls `loadPairs` **once, before the variant loop**. So the repair runs exactly once
// per season per binary, at whatever the environment said at first parse.
//
// An arm written the way `FPL_NO_AVAILABILITY` and `FPL_UNREGISTERED_POOL` are — which
// mutate the environment inside the variant's `apply` — would therefore be **completely
// inert**: both arms replay the identical already-repaired season, every cell comes out
// byte-identical, and the sweep reports a clean tight null on the exact change it was
// built to measure. That is this package's signature failure and it is queued,
// so it is worth the paragraph.
//
// **Sweep this hatch as two separate process runs**, the way `FPL_NO_XG_REPAIR` was.
// The two diagnostics that use it here call `Load` directly and bypass the memo for
// this reason — see TestTheXGCRepairHasAWorkingEscapeHatch.
func noXGCRepair() bool { return os.Getenv("FPL_NO_XGC_REPAIR") != "" }

// xgcRepairResult reports what the reconstruction did, on the same terms as
// xgRepairResult: a repair that silently did nothing must not read as one that ran.
type xgcRepairResult struct {
	Applied int // player-gameweeks given a reconstructed figure
	Skipped int // player-gameweeks that already carried one — never overwritten

	// NoClubMatch is player-gameweeks dropped because the player's club played no
	// fixture that gameweek, so there was no club figure to share out.
	//
	// Counted, because otherwise this struct does not reconcile: those rows are
	// skipped inside reconstructedXGC and would appear in none of the other three,
	// so Applied + Skipped + Empty would silently fall short of the appearances in
	// the window. In the ordinary case that is a handful of mid-season transfers —
	// exactly 9 in 2021-22.
	//
	// The reason it is worth a field rather than a comment is the degenerate case.
	// If a season in the repair table ever loads with no fixture list, clubMatches
	// is empty, **every** row lands here, and without this counter the result reads
	// Applied 0, Skipped 0, Empty 0 — byte-identical to "there was nothing to
	// repair", which is the null this package keeps mistaking for a measurement.
	NoClubMatch int

	// Empty is player-gameweeks whose reconstruction came out at exactly zero,
	// which are deliberately left unwritten rather than written as zero.
	//
	// Writing a zero would achieve nothing and cost the two properties this repair
	// is meant to have. `baseXP90` gates the clean sheet and the goals-conceded
	// deduction on `XGC90 > 0`, so a stored zero disables the terms exactly as an
	// absent value does — and because the repair only ever fills a zero, a row
	// written as zero is refilled on the next load, which breaks idempotence for a
	// repair that runs on every Load rather than once.
	Empty int

	SumXGC float64 // total added

	// AggFilled and AggKept are the season-aggregate half, and they matter for a
	// prior-only season: PreSeason and newPriorIndex read the season total, not
	// the weekly rows, so a season whose aggregate stays zero is a season whose
	// successor is still blind.
	//
	// AggKept must be **zero on every NoAggregate season**, on the same terms as
	// the xG side: the table says those seasons have no aggregate, so a player
	// who turns out to have one means the table and the archive disagree.
	// TestDiagXGCCoverage asserts it.
	AggFilled int
	AggKept   int
	AggXGC    float64
}

// clubMatches counts each club's fixtures per gameweek.
//
// The denominator for every share below, and the reason a blank gameweek is counted
// rather than divided by: a club with no match has no expected goals conceded, and
// dividing by zero to discover that would be a NaN propagating into a score.
func clubMatches(s *Season) map[[2]int]int {
	out := make(map[[2]int]int, 20*38)
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		out[[2]int{f.TeamH, *f.Event}]++
		out[[2]int{f.TeamA, *f.Event}]++
	}
	return out
}

// clubXGPerGameweek sums each club's players' expected goals, by gameweek.
//
// Summed in element order so the result is reproducible. Floating-point addition is
// not associative and this feeds a score, which feeds a discrete fifteen — the same
// reasoning weeklyXGTotals carries, one layer up.
func clubXGPerGameweek(s *Season) map[[2]int]float64 {
	out := make(map[[2]int]float64, 20*38)
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		for gw := 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.XG == 0 {
				continue
			}
			out[[2]int{p.Team, gw}] += g.XG
		}
	}
	return out
}

// clubXGAPerGameweek pairs each club's gameweek through the fixture list and returns
// what its opponents were expected to score against it.
//
// The one approximation is in the middle line: a club playing twice in a gameweek has
// its expected goals split evenly between the two matches, because the repair rows
// are keyed by gameweek and cannot say which match they belong to. Everywhere else
// this is definitional.
func clubXGAPerGameweek(s *Season, scale float64) map[[2]int]float64 {
	xg, matches := clubXGPerGameweek(s), clubMatches(s)
	perMatch := func(club, gw int) float64 {
		n := matches[[2]int{club, gw}]
		if n == 0 {
			return 0
		}
		return xg[[2]int{club, gw}] / float64(n) / scale
	}
	out := make(map[[2]int]float64, 20*38)
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		out[[2]int{f.TeamH, *f.Event}] += perMatch(f.TeamA, *f.Event)
		out[[2]int{f.TeamA, *f.Event}] += perMatch(f.TeamH, *f.Event)
	}
	return out
}

// reconstructedXGC returns each player-gameweek's expected goals conceded, derived
// from the opposition's expected goals and prorated by minutes.
//
// **This is the single implementation, and the diagnostic that validates it calls
// this function rather than reimplementing the chain.** A diagnostic carrying its own
// copy of the thing it is checking is this project's signature failure — it has
// shipped twice, in TestDiagSixtyMinutes and TestDiagPlaysAtAll — and it is worst
// here, because the whole claim being made is that the method reproduces the truth
// on the seasons that have it.
//
// Returned as a map rather than written in place so the validation can compare it
// against real data without mutating a season that has none of this wrong.
// The second return is the count of appearances dropped because the club played no
// fixture that gameweek — see xgcRepairResult.NoClubMatch for why it is counted rather
// than silently skipped.
func reconstructedXGC(s *Season, scale float64) (map[int]map[int]float64, int) {
	xga, matches := clubXGAPerGameweek(s, scale), clubMatches(s)
	var noMatch int
	out := make(map[int]map[int]float64, len(s.Players))
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		for gw := 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.Minutes <= 0 {
				continue
			}
			n := matches[[2]int{p.Team, gw}]
			if n == 0 {
				// His club played no match this gameweek, which happens for a
				// player who moved clubs mid-season: Player.Team is where he
				// finished, so his August rows are filed against a club that
				// may have been blank. Measured across the archive this is 1
				// to 15 player-gameweeks a season. There is no club figure to
				// share out, so he gets nothing rather than a guess.
				noMatch++
				continue
			}
			share := float64(g.Minutes) / (90 * float64(n))
			if out[id] == nil {
				out[id] = make(map[int]float64, 4)
			}
			out[id][gw] = xga[[2]int{p.Team, gw}] * share
		}
	}
	return out, noMatch
}

// applyXGCRepair writes the reconstruction into the seasons that carry no xGC.
//
// Called from applyXGRepair, after the weekly xG half has run, because the chain
// consumes exactly that output — running it first would reconstruct every one of
// these seasons from zero expected goals and produce a season-long clean sheet for
// everybody, which is a worse error than the hole it replaces.
//
// It fills only a zero, like every other half of this repair, so it is idempotent and
// degrades to a no-op if FPL ever publishes the weekly history.
func (s *Season) applyXGCRepair() xgcRepairResult {
	var res xgcRepairResult
	spec, ok := xgRepairs[s.Name]
	if !ok {
		// A season absent from the table is not repaired, and saying so here
		// rather than leaning on the caller's guard is the difference between a
		// decision and an accident: without this the zero-valued spec gives a
		// window of GW0-0, which drops every row for a reason no reader would
		// guess. The table is deliberate — see xgRepairs.
		return res
	}
	rec, noMatch := reconstructedXGC(s, xgcScale)
	res.NoClubMatch = noMatch
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		for _, gw := range sortedGameweeks(rec[id]) {
			if gw < spec.FirstGW || gw > spec.LastGW {
				// 2022-23 carries real xGC from GW16, and the window is what
				// keeps the two out of each other's way. Without it the
				// reconstruction would be a no-op there anyway — it only fills
				// zeroes — but "would be a no-op anyway" is how the two-thirds
				// season got repaired with a leak in the first place.
				continue
			}
			g := p.GWs[gw]
			if g.XGC != 0 {
				res.Skipped++
				continue
			}
			if rec[id][gw] == 0 {
				res.Empty++
				continue
			}
			g.XGC = rec[id][gw]
			g.XGCReconstructed = true
			p.GWs[gw] = g
			res.Applied++
			res.SumXGC += g.XGC
		}
	}
	if spec.NoAggregate && !noXGAggregate() {
		res.AggFilled, res.AggKept, res.AggXGC = s.rebuildXGCAggregates()
	}
	return res
}

// rebuildXGCAggregates fills the season TOTAL from the weekly rows, for the same
// reason rebuildXGAggregates does and with the same safety argument.
//
// The aggregate is what a *prior* is read through — `PreSeason` and the prior index
// behind `newPriorIndex` both take season totals — so a season whose weekly rows are
// repaired and whose total is left at zero hands its successor a squad built with no
// expected goals conceded at all. That is the defect rebuildXGAggregates was written
// for, arriving on the other statistic.
//
// It is not the point-in-time leak that summing a season normally is: the total
// consumed is last season's relative to the one being played, `PointInTime`
// accumulates weekly rows and never touches this field, and no quantity is invented
// or moved between weeks. See rebuildXGAggregates, which carries the argument in
// full.
func (s *Season) rebuildXGCAggregates() (filled, kept int, sum float64) {
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		// weeklyTotal rather than an inline walk: this loop used to be its own copy
		// of the one in xgrepair.go, and so did the test checking it. Same gameweek
		// order, same additions, so every repaired aggregate is bit-identical.
		xgc := weeklyTotal(p, func(g GW) float64 { return g.XGC })
		if xgc == 0 {
			continue
		}
		if p.XGC != 0 {
			kept++
			continue
		}
		p.XGC = xgc
		filled++
		sum += xgc
	}
	return filled, kept, sum
}

// sortedGameweeks is a reproducible walk over a per-gameweek map.
func sortedGameweeks(m map[int]float64) []int {
	out := make([]int, 0, len(m))
	for gw := range m {
		out = append(out, gw)
	}
	sort.Ints(out)
	return out
}
