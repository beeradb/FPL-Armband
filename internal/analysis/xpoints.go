package analysis

import (
	"fmt"
	"math"
)

// Expected points from REALISED underlying — the xPoints instrument.
//
// # What it is for
//
// A single decision scored on realised points is largely finishing variance. This
// prices the channels where that variance lives — goals against xG, assists against
// xA, the clean sheet and the concede deduction against expected goals conceded —
// off what the player's underlying numbers actually were, and leaves every other
// channel realised.
//
// # Why it is a residual rather than a re-scoring
//
// `xPoints = points - residual`, where the residual is
// `realised_contribution - expected_contribution` on the four replaced channels
// only. Every other channel keeps its realised value automatically, without this file
// having to know how FPL priced it in the season being replayed.
//
// **The full left-realised set, against FPL's own published table.** Two rulebook
// audits found this list incomplete — it named five and there are eight, with the
// penalty and own-goal keys missing, which is precisely the omission the engine's
// `TestFPLPaysNothingTheModelDoesNotPrice` exists to make impossible one file away:
//
//	long_play, short_play    appearance — not conversion variance
//	bonus                    ⚠️ NOT independent of the replaced channels, see below
//	saves                    divisor unpublished, and restructured for 2026/27
//	defensive_contribution   2025-26 only
//	yellow_cards, red_cards
//	own_goals                also inflates the scorer's own club concede, see below
//	penalties_missed         ⚠️ xG already includes the penalty, see below
//	penalties_saved
//	special_multiplier       not a per-event term; the identity at 1
//
// ⚠️ **And `goals_conceded` is left realised on DOUBLE gameweeks**, which belongs on
// this list even though the key is otherwise replaced: the per-match split is not
// recoverable from an accumulated row, so the channel is refused there. A reader
// auditing the left-realised claim would not otherwise find it.
//
// `TestXPointsAccountsForEveryPaidScoringKey` pins the set against
// `GameConfig.PaidScoringKeys()`, so a key FPL adds cannot land here unclassified.
//
// That matters more than it looks. The archive spans three bonus regimes, defensive
// contribution exists only from 2025-26, and appearance points are a two-step whose
// boundary this package prices per gameweek. Re-scoring a gameweek from scratch
// would need every one of those right for every season; subtracting a residual needs
// only the four channels being replaced. It is what lets the 2026/27 bonus and saves
// changes arrive with no work here at all.
//
// # ⚠️ "A channel it does not replace cannot be got wrong" is TRUE of the level and FALSE of the variance
//
// The claim holds for anything **independent** of what is replaced. Bonus is not
// independent: BPS pays goals, assists and clean sheets, so the conversion luck the
// residual strips walks back in through the bonus column.
//
// Measured (`stats/xpoints_channel_audit.py`, three native-xG seasons, 60+ minutes),
// regressing realised bonus on the residual this file removes:
//
//	attackers (MID+FWD, n 12,104)   corr 0.606   slope 0.252
//	defenders (GKP+DEF, n 11,049)   corr 0.479   slope 0.156
//
// So for every point of conversion luck removed, about **a quarter of it returns**
// for an attacker. The instrument is under-smoothed, worst where it is used most.
// ⚠️ It biases an xPoints arm **toward** a realised-points arm, so part of any
// recovered fraction measured against one is leakage rather than information in the
// underlying. Found independently by both audits, by different estimators.
//
// Not fixed, and deliberately: modelling expected bonus is a closed line in this
// record — the bonus term double-counts goals, assists and clean sheets by
// construction. **Report the leak beside the figure; do not replace the channel.**
//
// Two smaller cases of the same shape, both checked and both benign:
//
//   - **Penalties.** FPL's xG includes the penalty at ~0.79, so a missed penalty is
//     credited its expected value while the realised −2 stays in `Points`. That is
//     not a double charge: over attempts the two cancel exactly, leaving **dispersion
//     rather than bias** — a miss week reads about 1.5 points above the event's true
//     worth, at 11-25 misses a season league-wide. Verified rather than assumed: the
//     minimum xG on a missed-penalty-no-goal row is exactly 0.79.
//   - **Own goals.** The −2 is realised and uncorrelated. The quieter effect is that
//     an own goal increments the scorer's own club's conceded total while xGC — the
//     opponents' xG — never predicted it, which pushes GK/DEF realised clean sheets
//     below `exp(-xgc)`. That is a **component of** the clean-sheet over-prediction
//     measured below, not an addition to it. Do not count it twice.
//
// # ⚠️ The clean sheet must be priced PER FIXTURE
//
// `season.go` accumulates a gameweek's rows, so a double gameweek arrives as ONE
// entry with `Fixtures = 2`, `XGC` summed over both matches and `CleanSheets`
// counting up to 2. `exp(-xgc)` on that total is `exp(-(x1+x2))`, where the
// expectation over two matches is `exp(-x1) + exp(-x2)`. Through a non-linear
// function the accumulated total is not the same quantity, and it under-predicts
// badly: 248 double rows in the archive carry 109 real clean sheets against 37
// predicted by the summed form.
//
// This is the recorded doubles class — "a double gameweek is two archive rows" —
// arriving through a NON-LINEAR function, which is exactly where the
// `(element, fixture)` guard that catches the linear version cannot see it. It has
// already cost one published figure, moving a clean-sheet over-prediction from 28.3%
// to 33.1%. `TestXPointsPricesTheCleanSheetPerFixture` fails if the summed form
// returns.
//
// Splitting the accumulated xGC evenly across the fixtures is exact for a single
// fixture and an approximation for a double, since the two matches had different
// opponents. It is the right functional form where the summed version is not.
//
// # The POINT VALUES are the engine's own; the CONVERSION SCALE is now shared too
//
// goalPoints, cleanSheetPoints, assistPoints and concedeBlock are this package's,
// and TestScoringConstantsMatchFPL asserts them against FPL's published
// game_config. stats/xpoints_common.py carries a second copy for the offline probes
// and is pinned to these by TestTheXPointsScriptsShareTheScoringTable.
//
// ⚠️ **Those four tables are TODAY'S rules and this file no longer reads them
// directly.** It takes an `ScoringRules` — the table that was in force in the
// season being priced — because the test that keeps the constants honest is also
// the mechanism that would have forced a rule change backwards over the whole
// archive. See `scoringrules.go`, which is the `BankLimitFor` of the instrument.
//
// The expected side of the attacking channels is priced through a ConversionScale,
// supplied by the caller and applied to XG and XA before the point values.
//
// # Why the scale is here, and the argument that is NOT the reason
//
// **A raw xG is not an FPL goal and a raw xA is emphatically not an FPL assist.**
// FPL pays an assist for winning a penalty that is scored, for a shot parried to a
// team-mate and for a deflected pass, none of which an expected-assists model
// counts; a forward's assists/xA runs at 2.13 / 2.08 / 2.11 across the three native
// seasons, which is definitional rather than noise. Differencing a realised count
// against an unscaled expectation therefore does not measure conversion luck — it
// measures conversion luck plus a fixed per-position offset.
//
// ⚠️ **The tempting argument — "baseXP90 applies scaleFor(pos), so mirror it" — is
// NOT the reason, and it is the one a reader will reach for.** The engine's own scale
// is not a stable target in either direction, so mirroring it is not a thing that can
// be done. Two mechanisms, both measured:
//
// The replay's bootstrap is reconstructed season-to-date by PointInTime, so
// calibrateExpectedStats runs on season-to-date totals and minCalibrationSample (20.0)
// gates each channel until the league clears 20 expected events. Cumulative crossings,
// measured over the three native seasons:
//
//	          DEF xG    MID xG   FWD xG   FWD xA    GKP
//	  crosses  GW5-6      GW2     GW2-3   GW10-20   never
//
// So the engine's scale is neutral on DEF-goals and GKP through the opening block and
// on FWD-assists until GW10-20, while MID and FWD goals go live from GW2-3. ⚠️ **An
// earlier version of this paragraph said "exactly 1.0 for every position at GW1-6"
// and put the defender crossing at "about GW7". Both are wrong** — two of four
// positions are live from GW2.
//
// ⚠️ **And at GW1 it is not neutral at all**: `PointInTimeWith` delegates `through <= 0`
// to `PreSeasonWith`, which fills the expected columns from the PRIOR season's totals,
// so the opening squad is built on that season's fitted ratio — DEF ~0.85, FWD assists
// ~2.1. The GW1 cell is the exact opposite of the claim it replaced.
//
// The case for the scale stands on the paragraph above and does not need baseXP90.
//
// # The bias this removes, and the larger one it does not
//
// Measured by code review 2026-08-14 over the three native-xG seasons, mean residual
// per appearance (rows with minutes > 0). ⚠️ **The clean-sheet column is quoted on two
// denominators in this record — per 60+ row and per appearance — and they must not be
// subtracted from one another:**
//
//	          assists   goals     clean sheet       clean sheet
//	                              (per 60+ row)   (per appearance)
//	  GKP         .        .         -0.304           -0.293
//	  DEF      +0.035   -0.041       -0.328           -0.239
//	  MID      +0.092   +0.003       -0.068           -0.041
//	  FWD      +0.151   -0.015          n/a              n/a
//
// The quantity that matters is the DEFENDER-MINUS-FORWARD gap, because
// perfectGateXPoints compares ACROSS positions — it sums xPointsOver(in) minus
// xPointsOver(out) over packages whose in and out players routinely differ in
// position, so the record's "a bias shared by every player in a position is not an
// ordering error" rule does not protect it. The gap is 0.37-0.40 per appearance,
// roughly 1.8-2.0 over the shipped five-gameweek window — ⚠️ on the assumption of an
// appearance in each of the five, since the denominator is PER APPEARANCE and
// xPointsOver sums over gameweeks including blanks. It is an upper bound on a real
// package rather than a central case.
//
// ⚠️ **This scale closes about a third of it and the clean sheet carries the rest.**
// The attacking terms are the SMALL ones for a defender: goals -0.041 and assists
// +0.035 against a clean sheet of -0.239. Zeroing both attacking channels takes the
// gap from ~0.37 to ~0.24, about 35%, leaving ~1.2 of the ~1.8 five-week head start
// standing. The repair is also ASYMMETRIC, which is the durable part: forwards have
// no clean-sheet channel, so the scale reaches the forward side and barely touches
// the defender side, whose residual is dominated by a channel this change does not
// touch.
//
// ⚠️ **And fixing it is NOT `FPL_CS_XGC_FACTOR`'s 2x2**, which a first version of
// this paragraph claimed. That knob is an *engine* scoring term — read at
// `teamstrength.go` and three sites in `metrics.go`, all `baseXP90`'s family — and
// **this file never reads it**, so no setting of it moves this residual by any
// amount. `ExpectedCleanSheets` here is a bare `f * exp(-xgc/f)`. The record also
// says of that line that the mechanism covers a flat rescale and NOT `f`, while
// what this residual carries is a cross-position LEVEL offset, so even the
// analogy points at the other half of that 2x2.
// ⚠️ That cross-reference was qualified 2026-08-15: a flat rescale is **not**
// ordering-inert within a position either, since it multiplies one additive
// component of `Score`. The point here is unaffected — this file never reads
// either knob — but do not carry the "flat rescale is exempt" half onward.
//
// The instrument's own clean-sheet level error has a named mechanism a hundred
// lines below: the **shot-level Jensen gap**, `exp(-Sum x)` against
// `Prod (1-x_i)`, measured at **1.27 on two independent providers**
// (`stats/xg_provider_scale.py`). ⚠️ That comparison lands on **175 of 380**
// shared fixtures and is **2015/16**, while FPL's feed is Opta which that script
// says it does not identify — so quote the order, not the constant. And the
// shot-level gap is one of TWO mechanisms behind the clean sheet's calibration;
// the other is cross-match convexity, which is what separates the two regressors.
// Neither replaces the other. If it is ever repaired it is a flat rescale of
// this file's own `ExpectedCleanSheets`, and it is unbuilt and unmeasured. A
// separate open line either way.
//
// ⚠️ **Do not quote a per-position NET figure from the table above.** A recorded
// "+0.29 for a defender" does not reconstruct from these columns under either
// denominator and is withdrawn; quote the gap, with its denominator and season set.
//
// # ⚠️ The fit is IN SAMPLE, and that changes what this instrument can see
//
// The scale is fitted on the same season's rows the residual is then evaluated on,
// so the position-mean attacking residual afterwards is **exactly zero, by
// arithmetic rather than approximately**. Three consequences, none of them optional
// reading:
//
//   - The instrument reports WITHIN-position conversion deviation only. It can no
//     longer see a position-level or season-level conversion effect, and it can no
//     longer re-derive the bias table above — stats/xpoints_bias.py deliberately
//     keeps the unscaled residual and is now the only way to observe that quantity.
//   - Cross-season xPoints LEVELS are on a per-season recentred scale and are not
//     comparable. Paired within-cell differences remain comparisons of ONE metric,
//     because the scale is identical in every arm — but ⚠️ **they are NOT
//     numerically unchanged, and every banked one is superseded.** "Unaffected"
//     here means the recentring cancels, never that the figure holds: `+85.4` is
//     itself a paired within-cell difference and it is in the superseded list
//     below. A first version of this bullet said "unaffected" flat, which licensed
//     exactly the re-quoting that list exists to stop.
//   - The 35% above is attained exactly rather than approached. An earlier draft of
//     this change called it a ceiling the fix would fall short of; that is true of an
//     out-of-sample or point-in-time scale and false of this one.
//
// **Arm-invariance is the property that makes it safe, not "the consumers are
// hindsight".** A scale fitted on LEAGUE-WIDE rows for a (season, position) is
// identical in every arm of a paired comparison, so hold_xpoints and policy_xpoints
// differences stay comparisons of one metric. ⚠️ **It must therefore never be
// computed from the SQUAD's rows or from a CELL's window** — either would make the
// metric arm-dependent and quietly turn a paired difference into two metrics. See
// Season.calibrateConversion, and TestTheConversionScaleIsSeasonGlobal.
//
// # Fitted per season, and what that costs
//
// Per season rather than pooled, because three of the six grid seasons carry
// REPAIRED xG — 2020-21 and 2021-22 in full, 2022-23 for GW1-15 — and a pooled scale
// would carry the provider and backfill scale difference straight into the
// instrument for half the grid. A per-season scale absorbs it.
//
// ⚠️ **The cost is real and is not negligible, contrary to a first draft of this
// change.** Defender goals/xG reads 0.933 / 0.850 / 0.757 over the three full native
// seasons: sd 0.088, about what counting noise on ~140 defender goals a season
// predicts. Defender xG is ~0.0433 per appearance, so at 6 points a goal that wobble
// injects about +/-0.023 points per appearance of season-common shock against the
// 0.040 per appearance of bias it removes on that channel — over half, and it does
// not average away across defenders, being one shock per season per position. The
// forward and midfield assists side, which is where the definitional 2.1x sits, is
// stable to 2-3%. Net: the stable half of this repair is the attacker side.
//
// ⚠️ **The scale is fitted on repaired xG for three of six seasons, so
// hold_xpoints and policy_xpoints LEVELS now carry a data state through a
// season-global quantity.** AGENTS.md's "name the data state or do not quote a
// recorded level" attaches to them from here.
//
// # What this supersedes
//
// The gate arm's **+85.4 a season on realised UNDERLYING** (variant 2, CR2 t 4.76
// against that comparison's own threshold of 46.0), the **0.645 recovered fraction**
// with its Fieller CI, and **AxisTransferGateResidual**, which reads xPointsOver too.
// ⚠️ **The points arm's +132 is NOT superseded** — it calls pointsOver and never
// touches this file — and it is the CONFINEMENT CHECK for the re-run: it must come
// back byte-identical, exactly as it did for the 2026-08-14 corrections. Until the
// re-run lands, only the qualitative reading survives, and it survives as a
// HYPOTHESIS rather than a measurement.
//
// ✅ **THE RE-RUN LANDED, 2026-08-15 (`xpoints-scaled-gate-rerun`), and the
// replacements are these.** Marked here rather than edited above, so the superseded
// figures stay findable. Full table and caveats in the header of
// `internal/backtest/gatexpoints_diag_test.go`; banked at
// `stats/snapshots/2026-08-15-gatescaled/`.
//
//   - the underlying arm: **+2.2294 pts/gw, +84.7 a season**, CR2 t 3.77 against
//     that comparison's own threshold of **57.7** — clears. ⚠️ **Do not read the SE
//     movement as the instrument getting noisier**: it rose on this arm (0.471 →
//     0.591) and FELL 20% on the residual arm (0.604 → 0.486), the level barely
//     moved (−0.6 a season), and an SE on 5 df carries ~32% sampling spread, so
//     ±25% is well under one SD. What actually moved is the between-season
//     distribution, which is what a per-SEASON fitted scale should act on.
//   - the recovered fraction: **0.6402, Fieller 95% CI [0.325, 0.813]**. What
//     survives is an INFORMATION statement: the underlying criterion recovers
//     roughly two thirds of what the points criterion does, and all three biases
//     push the fraction UP, so that shortfall is hard to escape rather than soft.
//     It **rejects 0.89** (t −3.96) and 1.00 (t −5.87) as facts about the fraction,
//     and cannot reject 0.50 (t +1.42) or the six-season 0.414 (t +2.051).
//     ⚠️ **0.89 was "the share a constant inside the gate family would need" and
//     that reading is DEMOTED, 2026-08-15**: it is `sig_season/perfect` on the
//     four-season bank that ratio came from, and it does not transport — 0.696 on
//     those same four seasons out of the later bank (`82fc8e0`, clean), a different
//     run and not a data-state effect ALONE, with the channels not separated; and
//     0.414 on six. ⚠️ **"The load-bearing result" is withdrawn with it** — a bar
//     this record has declared foreign to the comparison cannot be one.
//     Failing to reject 0.50 leaves the pre-registered bar UNRESOLVED for the
//     second time, and the CI is WIDER than the one it replaces; the bar is retired
//     rather than re-run.
//   - `AxisTransferGateResidual`: **+2.2546 pts/gw (+85.7)** on realised points, and
//     **−0.828 (−31.5 a season)** on the discriminator.
//
// ⚠️ **A third unsized bias on the fraction, and this file is its source:** the scale
// is fitted IN SAMPLE, so the underlying criterion enjoys a season-global fit no
// deployable criterion has. The fraction is therefore **optimistic** for anything
// shippable, on top of the bonus leak (inflates) and the points arm optimising the
// scored quantity (deflates). The LOSO question below is the same question.
//
// ⚠️ **The confinement check passed but proves nothing** — confinement is a code fact
// about pointsOver, not a measurement. The check with power was the opposite one:
// `hold_xpoints` had to MOVE, and did, in 144 of 144 cells, while `hold_points`,
// `squad_hash` and the baseline's `policy_points` moved in 0 of 144.
//
// ⚠️ **How far the criterion actually moves: about 0.7 points over the five-gameweek
// window on a defender-in / forward-out package** (0.59 / 0.93 / 0.63 by season). The
// movement is the ATTACKING component alone — mean attacking residual per appearance
// is DEF ~-0.006 against FWD ~+0.137, so ~0.14 per appearance-pair. ⚠️ **An earlier
// version said 1.2-1.9 and it was 2-3x too large**: it reused the *residual* head
// start and the *original* head start from the section above as if either were the
// delta. It also claimed this was "the same order as many packages' distance from the
// net > 0 threshold", which rested on the inflated figure and on a distribution of
// package margins nobody has produced. Both are withdrawn.
//
// stats/xpoints_variance.py and stats/xpoints_permove.py report properties of the
// instrument's residual, so their banked figures are superseded on the same ground,
// and so are the resident xppilot figures — the 20-25% HOLD SE cuts, the 30-60% on
// POLICY, and the t 2.54 on xPoints are all hold_xpoints/policy_xpoints numbers.
//
// ⚠️ **Open and unrun: whether a LEAVE-ONE-SEASON-OUT scale is the better trade.**
// The in-sample fit removes ~0.040 per appearance of defender-goals bias and injects
// ~0.023 of season-common shock doing it. A held-out scale would trade less noise for
// less bias removal; nothing here decides which is better, and no arm has been run.
//
// # ⚠️ RETRACTED: the clean sheet is NOT compared against the wrong event
//
// This section claimed `XGC` is a while-on-pitch quantity while the clean sheet FPL
// pays is a **team-match** event, so the two were different events and the comparison
// a category error. **The premise is false.**
//
// FPL pays a clean sheet for **not conceding while on the pitch**, having played
// sixty minutes — exactly what `exp(-xgc)` models. Two independent adjudications
// settled it against FPL's own accounting, and `docs/model.md` §4e already said so:
//
//   - Six seasons, **22,605** GK/DEF rows at 60+ minutes: `clean_sheets == 1` **iff**
//     the player's own `goals_conceded == 0`, **zero exceptions**.
//   - **89 rows carry a clean sheet in a match their club conceded in** — impossible
//     under a team-match rule — and **88 of the 89 reconstruct `total_points` with
//     the four points included**, so FPL paid for them.
//   - Per fixture, **77 of 77** within-club disagreements run "the substituted man is
//     credited, the ninety-minute man is not"; the reverse has no instances.
//
// **What survives is a third the size and does not resolve.** Matched within a
// single-fixture club-gameweek the band gap falls from −0.034/−0.059/−0.088 to
// **−0.0196 clean sheets** (n 1,185, iid t −3.18, **season-clustered t −2.11 against
// t_crit 4.303 at df 2**), with one season carrying it. That is **3-4% of the
// clean-sheet channel** — not "the bigger half" of anything.
//
// ⚠️ **Do not "fix" this by fetching club match-level xGC.** That would *introduce*
// the mismatch this section wrongly claimed to find, and `docs/model.md` §4e names it
// as a proposal that has been made before and would delete a real effect.
//
// **The rule this cost, worth more than the finding: a band split does not identify a
// mechanism.** Splitting a population on the variable a story is about will show the
// story's sign whenever the groups differ in anything else — here two thirds of the
// gap was club selection. Compare *within* the cluster before writing a cause down.
//
// The real mechanism for the level error is a **Jensen gap**: `exp(-Σx)` exceeds
// `Π(1-xᵢ)` over the individual shots faced. See `stats/xg_provider_scale.py`, which
// measures the scale at **1.27 on two independent providers over the same 380
// matches**, agreeing to 0.9%. Measurement only.
//
// ⚠️ **Corrected 2026-08-15: this said `FPL_CS_XGC_FACTOR` is "a closed line that
// lost points on a non-monotone curve". The line is OPEN.** AGENTS.md's scoring-model
// section says so in bold — it wants a 2x2 of `f` against a flat P(CS) scale, with
// both arms taken from the joint fit (`f` 1.173, flat 0.905) and never from the
// constrained ones. What was retracted on 2026-08-14 was the *ladder that priced the
// clean-sheet over-prediction* and the "wrong family" refutation of it, not the knob.
//
// This matters more than a stale comment usually does, because **this is the comment
// a grep for the identifier finds first.** Anyone scoping the 2x2 reads "closed line"
// here and stops, which is the failure mode the record's own "a title alone does not
// stop an idea being rebuilt" rule exists to prevent, running in reverse: a wrong
// verdict here stops a live idea being *measured*.

// XPointsGW is one player-gameweek as the instrument needs to see it. It is a
// parameter object rather than a long argument list because six of the fields are
// counts and three are rates, and a caller transposing two of them would produce a
// plausible number rather than a compile error.
type XPointsGW struct {
	Position      int // element_type
	Fixtures      int // matches accumulated into this row; 2 in a double
	Minutes       int
	Points        int
	Goals         int
	Assists       int
	CleanSheets   int
	GoalsConceded int
	XG            float64
	XA            float64
	XGC           float64
}

// ExpectedCleanSheets prices the clean sheet per fixture: f * exp(-xgc/f).
//
// Exported because it is the step most likely to be reimplemented by a caller who
// only needs "the clean-sheet expectation", and a second implementation of this
// particular expression is the one this package can least afford.
func ExpectedCleanSheets(xgc float64, fixtures int) float64 {
	f := fixtures
	if f < 1 {
		f = 1
	}
	return float64(f) * math.Exp(-xgc/float64(f))
}

// XPointsResidual is realised minus expected on the four replaced channels.
//
// Positive means the player out-performed his underlying — he converted more than
// the model of his chances says he should have.
//
// The scale is a SECOND ARGUMENT rather than a field on XPointsGW, and deliberately:
// a field would default to the zero value, which prices xG and xA at zero — that is
// not a small error but the "zero both attacking channels" arm, in the direction this
// repair argues for, produced silently. As an argument, omitting it is a compile
// error at every call site. The panic below is the second line of defence, for a
// caller who passes a zero-valued struct explicitly.
//
// The season's rules are a THIRD argument on exactly the same argument, and a
// zero-valued ScoringRules is refused by the position guard below rather than
// pricing every channel at nothing.
func XPointsResidual(g XPointsGW, s ConversionScale, r ScoringRules) float64 {
	// ⚠️ Before the minutes guard, deliberately.
	//
	// A position the rules have no entry for is a programming error whatever the
	// row happens to contain, and putting this second would make the refusal
	// depend on the row: `element_type` 5 — the 2024-25 assistant managers, 312
	// player-gameweeks from 322 archive rows, carrying 1,861 points — records no
	// minutes on ANY of them, so a guard behind the blank-gameweek return would
	// never fire on the one population it exists for.
	//
	// Read through a bare `goalPoints[g.Position]` an unpriced position returns
	// the map's zero, so its goals channel is deleted and the row still scores a
	// plausible number. That is this record's signature failure — a null that
	// looks like a result — and the fix is the one AGENTS.md's standing rules
	// name: fail fast when a precondition is not met.
	//
	// Nothing on the replay path can reach this today. `PointInTimeWith`
	// publishes element_types 1-4 only, so a manager resolves to position "?",
	// `squadQuota["?"]` is 0 and no squad can hold one. That is a fact about a
	// caller and not about this function, which is why the guard is here.
	if !r.Prices(g.Position) {
		panic(fmt.Sprintf("analysis: XPointsResidual has no scoring rules for "+
			"element_type %d in season %q (rules price %v) — a position with no "+
			"goal value must not be scored as one with a goal value of zero; see "+
			"ScoringRulesFor", g.Position, r.Season, r.Goal))
	}

	if g.Minutes <= 0 {
		// No appearance, no conversion. Every channel below is zero anyway, but
		// saying so here keeps a blank gameweek from depending on that being true
		// of each term separately.
		return 0
	}

	// ⚠️ Gated on the UNDERLYING being present, not on the realised counts.
	//
	// Rows with goals and no xG exist in quantity — every pre-2022-23 season before
	// the repair, and the package's own week fixtures build rows that carry no xG,
	// xA or xGC at all. On those the scale cannot change the arithmetic, so
	// demanding one would turn a no-op into a crash. Where there IS underlying to
	// price, a missing scale is a programming error and must not be survivable.
	if g.XG > 0 || g.XA > 0 {
		if s.Goals <= 0 || s.Assists <= 0 {
			panic(fmt.Sprintf("analysis: XPointsResidual needs a conversion scale for "+
				"position %d, got %+v — see Season.calibrateConversion", g.Position, s))
		}
	}

	res := (float64(g.Goals)-g.XG*s.Goals)*r.Goal[g.Position] +
		(float64(g.Assists)-g.XA*s.Assists)*r.Assist

	// ⚠️ Gated on XGC > 0, exactly as baseXP90 gates its own clean-sheet term.
	//
	// A zero xGC is MISSING DATA, not a guaranteed clean sheet, and the difference
	// is the whole term: exp(-0) = 1, so an ungated version hands every player in
	// every season without xGC a certain clean sheet and then charges him the full
	// four points for not having kept it. Four archived seasons carry no xGC
	// natively and 2022-23 carries none before GW16, so this is not a corner case —
	// it is most of the archive.
	//
	// The same reasoning as the engine's, which is why the gate is the same: a
	// season where the intervention cannot run must return nothing, not a confident
	// wrong number.
	//
	// FPL also pays the clean sheet only from sixty minutes, and only to positions
	// it pays at all — forwards score 0, so the term must not manufacture an
	// expectation for them.
	if g.XGC > 0 && g.Minutes >= 60 && r.CleanSheet[g.Position] > 0 {
		// ⚠️ The number of clean sheets ON OFFER is not the club's fixture count.
		//
		// Fixtures is how many matches the CLUB played; a clean sheet needs sixty
		// minutes in a PARTICULAR match. A player who started one leg of a double
		// and sat out the other can realise at most one, and pricing him at
		// 2*exp(-xgc/2) charges an expectation he could never have met.
		//
		// Measured: 320 archive rows have two fixtures, 60-119 minutes and non-zero
		// xGC, and the uncapped form inflates them by 505 points in total, 110 of
		// them by more than 2 points each. Kiwior 2023-24 GW34 — two fixtures, 90
		// minutes, xgc 0.14, one clean sheet — was priced at an expected 1.865
		// against a realisable maximum of 1, handing him +3.46.
		//
		// This is the file's own headline bug one step further in: the fixture count
		// is right for splitting the xGC and wrong for counting the chances.
		//
		// Minutes/60 is the best available bound on matches played to the hour,
		// since the archive accumulates a gameweek's rows and the per-match split is
		// not recoverable. It is exact for the common cases (90 over two fixtures is
		// one; 180 is two) and never exceeds what was on offer.
		eligible := g.Minutes / 60
		if eligible > g.Fixtures {
			eligible = g.Fixtures
		}
		perMatch := ExpectedCleanSheets(g.XGC, g.Fixtures) / float64(maxInt(g.Fixtures, 1))
		res += (float64(g.CleanSheets) - float64(eligible)*perMatch) *
			r.CleanSheet[g.Position]
	}

	// The concede deduction, which the Python probes omit. Realised is
	// -floor(conceded/d); expected is -E[floor(X/d)] for X ~ Poisson(xgc), which is
	// what baseXP90 prices and is NOT xgc/d — the floor is not linear.
	//
	// ⚠️ Per fixture again, and for the same reason. A double's accumulated
	// GoalsConceded is floor-divided once in FPL's own scoring (once per match), so
	// the realised side is approximated here by dividing the total, which is exact
	// whenever one of the two matches concedes an even count and off by at most one
	// point otherwise. The expected side is scaled per fixture to match.
	//
	// ⚠️ **Single fixtures only, and no sixty-minute gate.** Two corrections, both
	// from code review:
	//
	// The deduction applies from the first minute a keeper or defender is on the
	// pitch — it is not the clean sheet's sixty-minute rule — and baseXP90 has no
	// minutes condition on it either. Gating it left ~110 points a season of real
	// deduction un-replaced on sub-hour appearances.
	//
	// The double is refused rather than approximated. FPL deducts per MATCH, so what
	// is already inside g.Points is floor(a/d) + floor(b/d), while the accumulated
	// row can only offer floor((a+b)/d). Those differ by exactly one whenever both
	// matches concede an odd number — about a fifth of GK/DEF doubles at real
	// scoring rates — and because the residual subtracts a realised value MORE
	// negative than the one inside Points, the error lands as a phantom +1 on
	// xPoints. The per-match split is not recoverable from an accumulated row, so
	// the honest move is to leave the channel realised on doubles: this file's whole
	// design is that a channel it cannot get right is one it does not replace.
	if d, ok := r.ConcedeBlock[g.Position]; ok && g.XGC > 0 && g.Fixtures == 1 {
		realised := -float64(g.GoalsConceded / d)
		expected := -poissonFloorDiv(d, g.XGC)
		res += realised - expected
	}
	return res
}

// XPoints is the gameweek's realised points with the conversion residual removed.
func XPoints(g XPointsGW, s ConversionScale, r ScoringRules) float64 {
	return float64(g.Points) - XPointsResidual(g, s, r)
}
