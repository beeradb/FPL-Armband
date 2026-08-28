package analysis

import "armband/internal/fpl"

// Blending last season into this one.
//
// FPL's aggregates reset at GW1, so from the first whistle the model reasons
// from whatever has happened since — two matches at GW2. That is too little to
// judge anyone by, and last season is gone from the API by then. internal/priors
// keeps it; this decides how much of it to still believe.
//
// The shape is the usual shrinkage estimator: as current-season evidence
// accumulates it displaces the prior.
//
//	weight = n / (n + k)
//
// n is current-season evidence and k is the prior's strength in the same units,
// so at n == k the two are believed equally.
//
// # Both values of k are measured, not chosen
//
// Calibrated against the 2025-26 season, predicting each player's rest-of-season
// output from a blend of what had happened so far and what came before:
//
//	expected-stat rates   k = 8 matches   MAE 0.0511 xG/90
//	                                      16% better than ignoring last season
//	                                      14% better than ignoring this one
//
//	minutes per match     k = 5 matches   MAE 18.74 minutes
//	                                      15% better than either extreme
//
// Both curves are flat around the minimum — k=5 to k=12 for rates, k=3 to k=8
// for minutes — so neither is fitted to noise.
//
// The rate figure is a clean out-of-sample calibration: 2024-25 as the prior,
// 2025-26 as the outcome, 218 players. The minutes figure is weaker, because
// the 2024-25 dataset carries no minutes column, so the first half of 2025-26
// stands in as the prior. That prior is 19 matches of evidence rather than 38,
// which if anything argues for a slightly larger k than 5. Re-derive it once a
// full season with minutes on both sides is available.
type blend struct {
	// Weight is the share given to current-season data, 1.0 pre-season.
	Weight float64

	MinutesPerMatch float64
	StartShare      float64
	// GateMinutesPerMatch is what the eligibility floor (SettledMinutes) reads
	// instead of MinutesPerMatch, when GateMinutesSet is true — see
	// shrinkToLeague's own comment. GateMinutesSet false means "no
	// divergence, read MinutesPerMatch": a bool rather than a zero sentinel,
	// because a genuinely no-minutes player's raw figure IS zero and must
	// still win over a shrunk-toward-league-average positive one. Every path
	// that does not deliberately create a gap leaves both unset rather than
	// duplicating MinutesPerMatch into GateMinutesPerMatch, so a reader
	// auditing for a second implementation of "expected minutes" finds one
	// real quantity and one explicit, named exception, not two competing
	// copies.
	GateMinutesPerMatch float64
	GateMinutesSet      bool
	XG90                float64
	XA90                float64
	XGC90               float64
	DefCon90            float64

	// Counting stats, per 90. Raw, these are the most explosive terms in the
	// model early in a season: they divide a whole number by a fraction of a
	// match.
	Bonus90  float64
	Saves90  float64
	Yellow90 float64
	Red90    float64
}

// blendFor returns the rates to score a player on, mixing this season with last
// in proportion to how much of this season there is.
//
// Pre-season it is a no-op: FPL still carries last season's totals, so those
// *are* the current numbers and there is nothing to blend them with.
// RecentPlayer is a recency-weighted view of what a player has been doing
// lately, as distinct from what he has done all season.
type RecentPlayer struct {
	MinutesPerMatch float64
	StartShare      float64
	// Matches is how many gameweeks the estimate is built from.
	Matches int
	// BlankRun is how many consecutive gameweeks, ending at the most recent
	// one his club played, he recorded no minutes in. See blankRunFactor: the
	// exponential average under-reacts to the *onset* of an absence, and a run
	// is the signature of a state rather than of a bad week.
	BlankRun int

	// Rates are the same gameweeks weighted the same way, but for output rather
	// than availability — "form". Zero when the supplier does not compute them,
	// which is the default: see RateHalfLife for why this is separate from the
	// minutes weighting rather than the same knob.
	//
	// Minutes90 is how many 90s the weighting is spread over, so the caller can
	// tell a weighted rate built on two appearances from one built on twenty.
	Minutes90 float64
	XG90      float64
	XA90      float64
	XGC90     float64
	DefCon90  float64
	Bonus90   float64
	Saves90   float64
}

// RecentForm supplies recency-weighted current-season rates. Optional: without
// it the model uses flat season-to-date aggregates, which is what FPL's
// bootstrap publishes.
type RecentForm interface {
	Get(code int) (RecentPlayer, bool)
}

// blendFor is blendRates plus the post-tournament rest adjustment.
//
// Rest lands here, on minutes, rather than on Score at the end, because that is
// the channel it was measured in: per-90 output does not reliably move for a
// player back from a tournament, but his minutes and his chance of being held
// out both do. See Weights.RestMinutesFactor.
//
// Putting it here also means everything downstream agrees with it — the minutes
// rating, ExpectedMinutes and the rotation_risk band the agent is told to treat
// as a first-class filter all move together. Applied at Score, a rested player
// still reported full minutes and still banded as "nailed".
//
// The rates are deliberately untouched: a tired player plays less, not worse.
// prorateOverride spreads a minutes override across the horizon it is judged
// over, when the analysis layer said how long it lasts.
//
// An override with a return date is a claim about *particular gameweeks*: "out
// until GW12" says nothing about GW13. The horizon averages five, so applying
// the override flat asserts the injury lasts as long as the model happens to be
// looking — which over-penalises a player returning inside the window and, for
// a short override on a long horizon, is simply the wrong number.
//
// This is the same proration restFactor already does for the post-tournament
// window, and it is what makes an expected return date usable through the
// mechanism that works — correcting the number — rather than through an
// exclusion, which asserts a conclusion the optimiser cannot decline.
//
// An override with no end date is indefinite and applies flat, which is right:
// nobody has said when it stops.
func (e *Engine) prorateOverride(code int, override, natural float64) float64 {
	_, until, _, _ := e.minutesOverrideFor(code)
	if until <= 0 {
		return override
	}
	next := e.Boot.NextEvent()
	if next == nil {
		return override
	}
	horizon := e.Weights.Horizon
	if horizon < 1 {
		horizon = 1
	}
	affected := until - next.ID + 1
	if affected <= 0 {
		// Already lapsed. config.ActiveMinutes normally drops it before we get
		// here, so this is a belt-and-braces path rather than the usual one.
		return natural
	}
	if affected >= horizon {
		return override
	}
	return (float64(affected)*override + float64(horizon-affected)*natural) / float64(horizon)
}

// reassertMinutesOverride re-applies el's minutes override to b, if one is set
// and ignoreCode is not suppressing it — after some earlier step in
// blendRatesCode has touched b.MinutesPerMatch or b.StartShare.
//
// It exists because the override is applied once, early, before the current-
// and prior-season blend runs — and every step after that point is a
// weighted average that pulls toward some OTHER number. An override is a
// statement of fact, not a sample (see the comment where it is first
// applied), so a step that overwrites b.MinutesPerMatch has to hand control
// back rather than leaving the override diluted or erased by construction.
// blendRatesCode has two such steps today, the pre-season prior injection and
// the current/prior-season minutes blend, and both call this rather than
// repeating the same four lines: a copy already drifted once (the second
// call site simply had no reassertion at all until it was added here) and a
// third copy would be how it drifts again.
func (e *Engine) reassertMinutesOverride(el *fpl.Element, ignoreCode int, b *blend) {
	if ignoreCode != 0 && el.Code == ignoreCode {
		return
	}
	if v, _, _, ok := e.minutesOverrideFor(el.Code); ok {
		v = e.prorateOverride(el.Code, v, b.MinutesPerMatch)
		b.MinutesPerMatch = v
		b.StartShare = clamp(v/90, 0, 1)
	}
}

// corroboratingMatches is how many matches of THIS season's own minutes are
// enough to trust an estimate that has no real prior season behind it.
//
// Asserted, not measured — it is a display threshold that never reaches
// Score, so it is not the kind of constant this project's replay harness can
// resolve. It exists only to rule out the specific failure rotationLabel's
// caller was built to fix: a single 90-minute cameo, divided by the one match
// that has been available so far, reads identically to a settled starter. Two
// matches is the minimal fix that is no longer "one match's worth of raw
// evidence" — the exact phrase this was reported against.
const corroboratingMatches = 2.0

// minutesCorroborated reports whether m's expected-minutes estimate has more
// behind it than the point estimate alone — see rotationLabel, which is the
// only caller. A flat threshold on ExpectedMinutes cannot tell a season's
// worth of appearances from a single cameo that happened to run the full
// ninety, and both used to read "nailed" identically.
//
// Corroboration exists when any of the following holds, checked independently
// because each is sufficient on its own and none is required:
//
//   - a real prior Premier League season is on file (p.Minutes > 0) — the
//     same test blendRatesCode already uses to route to shrinkToLeague,
//     exposed here rather than re-derived, so this can never disagree with
//     which branch actually priced the player;
//   - enough of the CURRENT season has been played that the raw figure is not
//     resting on one match (see corroboratingMatches);
//   - a manual override is set AND the analyst who set it explicitly marked
//     it confirmed (config.RosterOverride.Confirmed) — a settled fact, not a
//     hedge.
//
// The third bullet used to be a floor on the override's own magnitude
// (ExpectedMinutes >= 80), on the theory that a hedge and a confident
// assertion cluster at different values. They did, for the six overrides
// that theory was read off — until Tzolis shipped at 82 with a reason that
// itself says "Set to 82 rather than a nailed 85 as this is still only his
// second competitive appearance for the club": an explicit hedge, at a value
// the floor still waved through, because nothing in RosterOverride let this
// function tell "82, asserted as settled" from "82, hedged". A magnitude can
// never carry that distinction on its own; this project does not parse
// free-text reasoning either, so the only place left for it is a field the
// analyst sets directly. See config.RosterOverride.Confirmed.
//
// Deliberately NOT satisfied by an override alone, at any value or with the
// field unset: an override the analyst did not explicitly vouch for is real
// evidence for ExpectedMinutes itself, but not for "settled beyond doubt" —
// the two are different claims, and only the analyst can make the second one.
//
// ignoreCode suppresses el's own override, mirroring blendRatesCode's
// ignoreCode — used by NaturalMetrics to ask what the estimate would be
// without this player's own correction, where that correction obviously
// cannot also stand as the evidence for it.
func (e *Engine) minutesCorroborated(el *fpl.Element, ignoreCode int) bool {
	if e.Priors != nil {
		if p, ok := e.Priors.Get(el.Code); ok && p.Minutes > 0 {
			return true
		}
	}
	if n90 := float64(el.Minutes) / 90; n90 >= corroboratingMatches {
		return true
	}
	if ignoreCode == 0 || el.Code != ignoreCode {
		if _, _, confirmed, ok := e.minutesOverrideFor(el.Code); ok && confirmed {
			return true
		}
	}
	return false
}

func (e *Engine) blendFor(el *fpl.Element, m PlayerMetrics) blend {
	return e.blendForCode(el, m, 0)
}

// blendForCode is blendFor with ignoreCode's minutes override suppressed, if
// el carries one. It exists only for NaturalMetrics — see there — and is not
// a second implementation: blendFor is now the ignoreCode==0 case of this
// one function, so the override logic cannot drift between "the score" and
// "the score without the override" the way this codebase has seen a
// duplicated expression drift before (model.md §3, fixtureSensitiveAt).
func (e *Engine) blendForCode(el *fpl.Element, m PlayerMetrics, ignoreCode int) blend {
	b := e.blendRatesCode(el, m, ignoreCode)

	// A minutes correction from the analysis layer is a statement of fact about
	// what this player will actually play, so it already accounts for the
	// summer. Discounting it again would double-count.
	if ignoreCode == 0 || el.Code != ignoreCode {
		if _, _, _, ok := e.minutesOverrideFor(el.Code); ok {
			return b
		}
	}
	if _, f := e.restFactor(el); f < 1 {
		b.MinutesPerMatch *= f
		b.StartShare = clamp(b.StartShare*f, 0, 1)
	}
	return b
}

// blankRunFactor discounts a player who has just stopped playing, over and above
// what the recency weighting already does.
//
// # The gap it fills
//
// MinutesHalfLife weights recent gameweeks, which is the documented fix for a
// dropped player reading as an ever-present. It is not enough at the *onset* of
// an absence: one blank after a run of starts barely moves a half-life-4
// average, and empirically it means something much worse than a bad week.
//
// TestDiagAvailability, four seasons, 7,354 player-cutoffs, established players
// only (5+ appearances averaging 60+ minutes), predicting mean minutes over the
// next five gameweeks. Restricted to players FPL had *not* flagged, so this is
// signal the model does not already have through availabilityFactor:
//
// ⚠️ **The `actual` and `bias` columns are from a superseded data state. `n`,
// `expected` and `vanished` re-run.** Same test, same four seasons, 2026-08-16:
// n 5801/552/221/136/486 against 5721/564/222/138/489; `expected` within 0.4% on
// the run-0 row and within 1.8% on every row; `vanished` at
// 2.0/18.3/24.9/27.2/35.2% against the recorded 2/18/25/28/35, so **the ninefold
// vanish figure is reproduced rather than retracted**. What moved is realised
// `actual`: +2.0 minutes a gameweek at run 0. So every `bias` here, and every
// ratio derived from one, is a pre-fix figure.
//
// The cause is **the doubles fix**, and it separates from this checkout rather
// than staying a bare guess. This table landed in 66e2a18 (2026-08-08 08:02);
// the fix is 89fa973 (17:54 the same day) and is not an ancestor of it. And the
// mechanism predicts the *differential* signature, which is the part worth
// trusting: `newRecentIndexWith` divides weighted minutes by the weighted
// **fixture** count, so `expected` is per-match and invariant to the fix, while
// this test's `actual` is `future/weeks` and its established filter is
// `mins/appearances`, both per **gameweek**. Predicted: `expected` flat,
// `actual` up, `n` up. Observed: all three. The sibling guess — that "four
// seasons" then meant a different four — is **refuted**: 66e2a18's hard-coded
// pairs predict 2022-23 through 2025-26, which is today's four exactly.
// *Hypothesis*, on attribution only: the 2026-08-12 duplicate-row drop is an
// unexcluded co-mover, much smaller and running the other way on 2025-26.
//
//	trailing blanks    n     expected  actual    bias   vanished
//	0                  5721      69.4    64.9    -4.5      2%
//	1                   564      54.0    38.0   -15.9     18%
//	2                   222      44.1    30.4   -13.7     25%
//	3                   138      36.2    25.9   -10.2     28%
//	4 or more           489      23.4    24.0    +0.6     35%
//
// "vanished" is the share who record no minutes at all over the window. One
// unflagged blank multiplies that risk by nine.
//
// # Why a plateau and not a curve
//
// Taken relative to the run-0 row — which de-levels the model's general tendency
// to over-predict minutes — the correction wanted is 0.752, 0.737 and 0.765 at
// one, two and three blanks, and 1.097 at four or more. That is a flat plateau
// with a cliff either side, which is the structure AGENTS.md requires before
// believing a constant, and it is one number rather than five fitted ones.
//
// ⚠️ **The de-levelling was justified here as removing a "harmlessly
// position-wide" bias, and that claim is now measured and FALSE.** The table
// above is pooled over positions and never carried a split.
// `TestDiagAvailabilityByPosition` re-runs the same calibration cut by position;
// `stats/blank_run_position.R` does the inference. Six seasons, eight cutoffs,
// 8,849 unflagged run-0 player-cutoffs, `FPL_NO_BLANK_RUN=1`:
//
//	position   ratio    players   verdict
//	GKP        0.9990        62   apart from the outfield on both SEs, six seasons
//	DEF        0.9582       334
//	MID        0.9550       335
//	FWD        0.9380        98
//
// The headline is the **omnibus**, because it tests the actual question and
// chooses no pair: F(3,15) = 15.85, p = 6.4e-05 with season blocked, Friedman
// p = 0.0051. ⚠️ It is a season × position statistic — each cell is one
// observation — so it shares the blind spot described below and has no
// player-clustered counterpart. What makes it safe to lead on is that the three
// GKP pairs carrying it *do* survive one.
//
// The **ordering** is the most robust statement here: GKP is highest of the four
// in 5 of 6 seasons and FWD lowest in 5 of 6, each p = 0.0046 on
// Binomial(6, 1/4). Two things that p does not carry. It is uncorrected over the
// eight position × tail statistics printed beside it (Bonferroni 0.037, still
// clearing). And within-season ranking removes *within-season* dependence and
// nothing else — Binomial(6, 1/4) still assumes six independent season draws,
// which a recurring cohort is exactly what violates: 62 keepers carry the GKP
// column at a mean of 2.55 seasons each. An established goalkeeper's minutes are
// barely over-predicted at all; an established forward's are by 6%.
//
// ⚠️ **Quote the GKP contrasts and not the outfield ones.** All three GKP pairs
// survive a *player*-clustered SE (bootstrap t 4.02, 4.32, 3.86). DEF−FWD and
// MID−FWD reach t 2.72 and 3.03 on the season-means SE, clear no Holm bar there
// either (0.087 each), and collapse to 1.32 and 1.08 under player clustering.
// The two pairs that *do* clear Holm on the season-means SE are GKP−DEF and
// GKP−FWD, and both also survive player clustering, which is the agreement that
// matters. The reason is general and is now a standing hazard: **players are
// crossed with seasons, not nested in them**, so a cohort of the same
// footballers appears in all six season means and `sd/sqrt(6)` comes out too
// small — MID−FWD's season-means SE of 0.0052 is below its own within-season
// sampling floor of 0.0140, and three of the six pairs sit below theirs, which
// luck at df 5 does not explain. Neither estimator dominates: season clustering
// generalises over *football* and a player bootstrap over *footballers*, and the
// GKP result is the one both of them and the ordering agree on.
//
// The four-season grid is a *nested subset*, so it corroborates rather than
// replicates: the GKP point estimates move by less than 0.006 and the omnibus
// still rejects there (F(3,9) = 7.01, p = 0.0099), but no GKP pair clears Holm
// at df 3 and the pair that does — MID−FWD — is one of the two that player
// clustering removes. Do not cite it.
//
// # Why the exemption does not cover this, stated precisely
//
// A bias constant within each position and differing between them is, read
// literally, the exemption's own case: FPL forces 2/5/5/3, so a per-position
// level cannot change how many of each you own. The grounds that actually bite
// are AGENTS.md's own qualifications. `Optimize` is a knapsack against one
// budget, so a per-position level still moves money between positions and
// changes *which* keeper and *which* defenders are bought. More decisively,
// `blankRunFactor` applies to a **subset within** each position — the blank-run
// players — and not to their positional peers, so an error in its level is a
// within-position relative weighting error by construction. And the undamaged
// sibling rule applies unqualified: **a measured bias does not imply a
// correction exists.**
//
// `xgcrepair.go`'s scope rule is also engaged — the discrepancy is *reported*
// additively in the `bias` column above and *removed* multiplicatively here, and
// that precedent "is about an ADDITIVE bias and must not be cited for a
// multiplicative scaling". Note the order: the precedent could not have been
// cited for this removal in the first place, so the measurement is a second and
// independent ground rather than the only one. Measured both ways, the
// falsification survives the additive parameterisation too (GKP−FWD, Holm
// 0.049). Do **not** read an ordering into 2-of-6 pairs clearing
// multiplicatively against 1-of-6 additively: that is two sides of an arbitrary
// line on the same rows.
//
// ⚠️ **None of this reaches replayed points**, and the reason is recorded in
// AGENTS.md rather than here: `MinExpectedMinutes` is a cliff at 55 rather than
// a discount, and it already removes most one-blank players and effectively all
// of those with two or more — so the population this term refines is largely
// outside the optimiser's pool. Everything above is a statement about the
// mechanism, not a claim that a points arm exists to run.
//
// What this does and does not reach. It is load-bearing for the **level only**.
// The **shape** survives either way — the de-levelled and raw triples are equally
// flat and the 4+ row is above 1 in both — so the case for the term itself is
// untouched. On magnitude, read the banked run rather than this comment's own
// table: today's divisor is 0.960 on six seasons and 0.968 on four, so undoing
// the de-levelling is worth **3-4%**, not the 7% the superseded 0.935 implied.
//
// ⚠️ Banked and easy to miss: **the plateau differs by position too.** GKP's
// de-levelled plateau is 0.556 on six seasons, 95% CI [0.402, 0.735], which
// excludes the shipped 0.75; and GKP's and FWD's `4 or more` rows read 0.775 and
// 0.854 rather than above 1, so the upper cliff is a *pooled* statement. That
// design is far worse powered — 111 GKP plateau rows, 13 to 39 in some cells —
// and no contrast in it clears Holm, so this is **not** a second falsification.
// It is recorded so "the shape survives" is not read as "position does not reach
// the shape". Tables 5 and 6 of the banked run.
// **0.75 is deliberately unchanged**: what the constant should be, given a
// divisor that is not shared, is a separate question that this measurement does
// not answer, and moving a live scoring constant on a calibration argument alone
// is not something this record permits. ⚠️ The whole result is **post-hoc** —
// the population and estimator were inherited from the calibration above and the
// multiplicity correction was fixed before any p-value, but testing position at
// all was decided after seeing GKP sit apart. Figures, console and provenance in
// `stats/snapshots/2026-08-16-blank-run-position/`.
//
// The mechanism explains both cliffs. At zero blanks there is nothing to
// detect. By four the exponential average has caught up on its own and a further
// discount would double-count — which is exactly what the +0.6 row says.
//
// # Why this is safe for an argmax
//
// It removes a **bias** rather than trading bias for variance: a player who has
// stopped playing really has stopped playing. That is the same test minutes
// recency passed and rate recency failed, and it is the standing rule in
// AGENTS.md for extending recency to a new term.
//
// FPL_NO_BLANK_RUN=1 restores the old behaviour.
func (e *Engine) blankRunFactor(run int) float64 {
	if !blankRunAdjust || run < 1 || run > blankRunMax {
		return 1
	}
	p := e.Weights.BlankRunPenalty
	if p <= 0 {
		p = blankRunPenalty
	}
	return p
}

func (e *Engine) blendRates(el *fpl.Element, m PlayerMetrics) blend {
	return e.blendRatesCode(el, m, 0)
}

// blendRatesCode is blendRates with ignoreCode's minutes override suppressed
// at both places blendRates reads one. See blendForCode for why this is a
// parameter on the one implementation rather than a second copy of it.
func (e *Engine) blendRatesCode(el *fpl.Element, m PlayerMetrics, ignoreCode int) blend {
	// played gates the recency index below (Recent != nil && played > 0), not
	// bonus/minutes evidence — leave it on GameweeksPlayed, deliberately: a
	// recency index built over a single, still-live gameweek adds nothing over
	// el.Minutes itself. cmd/armband/main.go gates FETCHING that same index on
	// GameweeksPlayed() > 0 too (search "engine.GameweeksPlayed() > 0" there) —
	// the two are a matched pair. Changing either alone is safe on its own (a
	// wasted fetch, or a nil index read here); changing them apart is not.
	played := e.GameweeksPlayed()
	avail := e.matchesAvailable(el)
	minsPerMatch := float64(el.Minutes) / float64(avail)
	startShare := float64(el.Starts) / float64(avail)

	// Minutes are a statement about the present, not the season. A player who
	// lost his place six weeks ago still reads as a starter on the season
	// average, and predicting the next five gameweeks' minutes from a
	// half-life-2 weighting is 8.9% better than from the flat total across
	// three replayed seasons. Rates are the opposite — see the comment on
	// RateHalfLife — so only minutes are weighted here.
	var form *RecentPlayer
	if e.Recent != nil && played > 0 {
		if r, ok := e.Recent.Get(el.Code); ok && r.Matches > 0 {
			minsPerMatch, startShare = r.MinutesPerMatch, r.StartShare
			// The exponential average is right about a settled player and about
			// one long gone, and wrong in between. See blankRunFactor.
			if f := e.blankRunFactor(r.BlankRun); f < 1 {
				minsPerMatch *= f
				startShare = clamp(startShare*f, 0, 1)
			}
			if e.Weights.RateHalfLife > 0 && r.Minutes90 > 0 {
				form = &r
			}
		}
	}

	// A minutes correction from the analysis layer beats anything derived from
	// the data, because it exists precisely for the cases the data gets wrong:
	// a leg break that makes an ever-present read as fringe, a promoted club's
	// starter with no top-flight record, a player known to be out.
	//
	// It is deliberately the only thing the layer can assert here. Everything
	// downstream — the score, whether he clears the minutes floor, whether the
	// optimiser wants him at his price — is recomputed rather than dictated, so
	// a correction that is wrong costs a mis-scored player rather than a
	// mandated one. Unlike the blend it is not shrunk toward anything: it is a
	// statement of fact, not a sample.
	if ignoreCode == 0 || el.Code != ignoreCode {
		if v, _, _, ok := e.minutesOverrideFor(el.Code); ok {
			v = e.prorateOverride(el.Code, v, minsPerMatch)
			minsPerMatch = v
			startShare = clamp(v/90, 0, 1)
		}
	}

	b := blend{
		Weight:          1,
		MinutesPerMatch: minsPerMatch,
		StartShare:      startShare,
		XG90:            m.XG90,
		XA90:            m.XA90,
		XGC90:           m.XGC90,
		DefCon90:        defCon90(el),
		Bonus90:         rate90(el.Bonus, el.Minutes),
		Saves90:         rate90(el.Saves, el.Minutes),
		Yellow90:        rate90(el.YellowCards, el.Minutes),
		Red90:           rate90(el.RedCards, el.Minutes),
	}
	// Form: the same current-season rates, recency-weighted. It replaces the
	// flat season figure on the current-season side of the blend only — the
	// prior is still the prior, and the blend weight still comes from how much
	// football has been played, not from how recent it was.
	if form != nil {
		b.XG90, b.XA90, b.XGC90 = form.XG90, form.XA90, form.XGC90
		b.DefCon90, b.Bonus90, b.Saves90 = form.DefCon90, form.Bonus90, form.Saves90
	}
	if e.Priors == nil {
		return b
	}
	if !e.SeasonHasStarted() {
		// Not played == 0 — see SeasonHasStarted's own comment. GameweeksPlayed
		// stays 0 for the whole multi-day span between a gameweek's first
		// kickoff and its last final whistle, so a club that has already played
		// took this branch too, returning a debutant with no prior completely
		// raw instead of through shrinkToLeague below. Third instance of
		// 1a6f0a3's defect, this time in the blend rather than the
		// prior-loading gate.
		//
		// Pre-season there is nothing to blend against — FPL's totals *are*
		// last season. But the prior may be a better summary of last season
		// than last season is: with PriorHalfLife set, a thin year has older
		// ones folded into it, which is the whole point of the mechanism and
		// is most valuable at exactly the moment there is no current data.
		//
		// Without blending the prior is identical to what the element carries,
		// so this is a no-op then.
		p, hasPrior := e.Priors.Get(el.Code)
		switch {
		case hasPrior && p.Minutes > 0 && p.Minutes != el.Minutes:
			b.MinutesPerMatch = float64(p.Minutes) / GameweeksPerSeason
			b.StartShare = float64(p.Starts) / GameweeksPerSeason
			b.XG90 = per90(p.XG, p.Minutes)
			b.XA90 = per90(p.XA, p.Minutes)
			b.XGC90 = per90(p.XGC, p.Minutes)
			b.DefCon90 = per90(float64(p.DefCon), p.Minutes)
			b.Bonus90 = per90(float64(p.Bonus), p.Minutes)
			b.Saves90 = per90(float64(p.Saves), p.Minutes)
			b.Yellow90 = per90(float64(p.Yellow), p.Minutes)
			b.Red90 = per90(float64(p.Red), p.Minutes)

		case !hasPrior || p.Minutes == 0:
			// ⚠️ **A PLAYER WITH NO PRIOR USED TO RETURN FROM HERE AT ZERO
			// MINUTES, and that is the fourth instance of the defect the comment
			// above names.** The three recorded before it were all the
			// in-season half of the same mistake; this is the pre-season half,
			// and it is the one that builds the opening fifteen.
			//
			// The in-season path sends exactly this case to shrinkToLeague —
			// "no prior of his own, a promoted club's player or an arrival from
			// abroad" — and pre-season simply did not, so a summer signing, a
			// promoted regular and a player returning from abroad all left this
			// function with expected minutes of **exactly zero**.
			//
			// Measured before the fix: across six seasons, 122 to 284 players a
			// season read 0.0000, so Spearman against their coming minutes was
			// UNDEFINED — the model had no ordering over a fifth of the pool it
			// picks an opening squad from. That is also why the shipped config
			// carries thirteen hand-written minutes overrides, several of whose
			// texts say it outright: "scores 0.00 only because he has no Premier
			// League minutes".
			//
			// ⚠️ **Zero is not a weak prior, it is a wrong one.** Nothing is
			// known about these players, and the honest expression of that is
			// the position's league average, which is what shrinkToLeague
			// supplies and what the in-season path has used since 2026-08-23.
			// UnknownPriorShare scales how much of that fallback he receives, so
			// the sweep can reach the old zero as an ARM. It is 1 in every
			// shipped configuration; see its own comment for why 0 is a claim
			// rather than an off switch.
			shrunk := e.shrinkToLeague(el, b)
			if s := e.Weights.UnknownPriorShare; s < 1 {
				if s < 0 {
					s = 0
				}
				shrunk.MinutesPerMatch *= s
				shrunk.StartShare *= s
			}
			b = shrunk
		}
		// A minutes correction still wins over everything.
		e.reassertMinutesOverride(el, ignoreCode, &b)
		return b
	}
	p, ok := e.Priors.Get(el.Code)
	if !ok || p.Minutes == 0 {
		// No prior of his own — a promoted club's player, or an arrival from
		// abroad. Shrink toward the league rate for his position instead of
		// believing one match.
		//
		// Without this a newcomer who takes three bonus points on his debut
		// reads as three bonus points a gameweek for the rest of the season,
		// and the optimiser will pay a four-point hit to buy him. The replay
		// found exactly that: a GW2 transfer valued at +8.50 a gameweek, into a
		// player with ninety minutes of Premier League football to his name.
		//
		// Weight stays at what the sample justifies, so the reported figure
		// still says how thin the evidence is.
		b = e.shrinkToLeague(el, b)
		// shrinkToLeague now mixes MinutesPerMatch/StartShare toward the
		// league rate along with everything else — see its own comment — and
		// has no idea a manual override already set them. A minutes
		// correction beats anything derived from the data (the comment two
		// screens up) and must not be diluted back toward a league average
		// any more than blendForCode lets it be diluted toward a prior
		// season: reassert it, matching the identical call the pre-season
		// branch above already makes for the same reason.
		e.reassertMinutesOverride(el, ignoreCode, &b)
		return b
	}

	mix := func(cur, prior, n, k float64) float64 {
		w := n / (n + k)
		return w*cur + (1-w)*prior
	}

	// Minutes are judged per match, so the evidence is matches played — but the
	// matches THIS PLAYER'S CLUB has played, not the league-wide GameweeksPlayed.
	// That distinction is TeamMatchesStarted's whole point (see its own comment):
	// a Premier League gameweek spans Friday to Monday, GameweeksPlayed stays 0
	// for the entire span until the LAST fixture in it finishes, and FPL does not
	// wait — it resets a player's aggregates the instant his own club kicks off.
	//
	// n used to be GameweeksPlayed, which is the same broken quantity #40's
	// incident report named without changing: "blended that already-corrected
	// figure straight back toward his prior with weight n/(n+k), n =
	// GameweeksPlayed()". #40 worked around it downstream, by reasserting a
	// standing override after the mix ran; it left n itself, and therefore
	// every player with NO override, still wrong. During the gap — a club
	// that has already played, while some gameweek fixture elsewhere has not
	// — n was 0 for every one of that club's players regardless of how much
	// football they actually had on record, so w = n/(n+k) stayed at 0 and
	// MinutesPerMatch/StartShare came back as exactly last season's rate,
	// discarding real evidence that a player's role has already changed this
	// season — a transfer, a new manager, form already diverging from last
	// year's pattern.
	//
	// This is TeamMatchesFinished(el.Team), NOT TeamMatchesStarted, and NOT
	// avail (matchesAvailable) above. Two distinct failure modes on two
	// sides of the same fix:
	//
	// avail is a per-90 RATE DENOMINATOR, so it falls back to the full
	// pre-season 38 whenever this club itself has not yet kicked off, on the
	// correct assumption that a truly pre-season el.Minutes is a 38-match
	// season total. That fallback is wrong read as an EVIDENCE COUNT:
	// mid-gap, a club that has not yet played its own fixture has
	// el.Minutes == 0 (FPL has already zeroed the whole league's aggregates
	// the moment SeasonHasStarted, not per club — confirmed live, contrary
	// to TeamMatchesStarted's own comment, which describes only the
	// opposite disagreement), and avail's 38-fallback then read that genuine
	// zero as "38 matches of evidence he doesn't play", collapsing
	// MinutesPerMatch to a sliver of his prior rate for every player at a
	// club that simply had not kicked off yet.
	//
	// TeamMatchesStarted has no such fallback and fixes that side, but it
	// introduces the opposite mistake at the boundary: a fixture that has
	// KICKED OFF but not yet finished counts as a full match, and
	// el.Minutes for that club is whatever the LIVE match has accumulated
	// so far — a nailed starter's 47 minutes into his 90, not his eventual
	// total. Dividing that partial figure by "1 match played" and blending
	// it in with real weight understates his true rate for as long as the
	// match is live, then silently corrects itself once the match's numbers
	// are final — a smaller-grained version of exactly the mistake this
	// whole fix exists to remove: counting an event before it has produced
	// the evidence it is being asked to stand for. TeamMatchesFinished has
	// no such window: a fixture counts once its score and stats are locked
	// in (gated on FinishedProvisional, not Finished — see that field's own
	// comment: Finished itself lags full time by many hours live, which
	// would silently reintroduce a worse version of this same staleness).
	n := float64(e.TeamMatchesFinished(el.Team))
	priorPerMatch := float64(p.Minutes) / GameweeksPerSeason
	priorStarts := float64(p.Starts) / GameweeksPerSeason
	b.MinutesPerMatch = mix(b.MinutesPerMatch, priorPerMatch, n, e.Weights.BlendMinutesK)
	b.StartShare = mix(b.StartShare, priorStarts, n, e.Weights.BlendMinutesK)

	// A minutes correction still wins over everything — see the identical
	// reassertion in the pre-season branch above.
	//
	// Without this, a player with a genuine PRIOR season on record (as
	// distinct from the no-prior case above, which shrinkToLeague already
	// leaves alone) had his override blended straight back toward that
	// prior by the two lines above, and the blend weight is n/(n+k) with n
	// the CURRENT season's matches played — which is 0 or 1 for most of the
	// season's first few gameweeks. A backup goalkeeper handed a "he is
	// nailed now" override read as a backup goalkeeper again the moment his
	// club had a prior season on file, because w was too small for the
	// override to survive the mix. Caught live: n == 0 in the gap between a
	// gameweek's first kickoff and its last final whistle (SeasonHasStarted
	// true, GameweeksPlayed 0) reverts the override completely — Kinsky's
	// 88-minute correction returned exactly his prior season's rate,
	// 630 minutes / 38 = 16.6, because w was 0/(0+k) = 0. But the blend
	// weakens the override even once n is positive and small, which is why
	// the fix is a full reassertion rather than a workaround for n == 0
	// alone: blend.go's own comment on the override says "unlike the blend
	// it is not shrunk toward anything", and the two lines above did
	// exactly that.
	e.reassertMinutesOverride(el, ignoreCode, &b)

	// Per-90 rates are judged per 90 played, so the evidence is 90s played —
	// a substitute who has appeared six times has far less than six matches of
	// it, and the blend should know that.
	n90 := float64(el.Minutes) / 90
	rk := e.Weights.BlendRateK
	b.XG90 = mix(b.XG90, per90(p.XG, p.Minutes), n90, rk)
	b.XA90 = mix(b.XA90, per90(p.XA, p.Minutes), n90, rk)
	b.XGC90 = mix(b.XGC90, per90(p.XGC, p.Minutes), n90, rk)
	b.DefCon90 = mix(b.DefCon90, per90(float64(p.DefCon), p.Minutes), n90, rk)
	b.Bonus90 = mix(b.Bonus90, per90(float64(p.Bonus), p.Minutes), n90, rk)
	b.Saves90 = mix(b.Saves90, per90(float64(p.Saves), p.Minutes), n90, rk)
	b.Yellow90 = mix(b.Yellow90, per90(float64(p.Yellow), p.Minutes), n90, rk)
	b.Red90 = mix(b.Red90, per90(float64(p.Red), p.Minutes), n90, rk)
	b.Weight = n90 / (n90 + rk)
	return b
}

// rate90 converts a counting stat to a per-90 rate.
func rate90(count, minutes int) float64 { return per90(float64(count), minutes) }

func per90(total float64, minutes int) float64 {
	if minutes <= 0 {
		return 0
	}
	return total * 90 / float64(minutes)
}

// defCon90 reads FPL's per-90 defensive contribution, falling back to the
// season total when the per-90 field is absent.
func defCon90(el *fpl.Element) float64 {
	if v := el.DefensiveContributionPer90.Float(); v > 0 {
		return v
	}
	if el.DefensiveContribution > 0 && el.Minutes > 0 {
		return float64(el.DefensiveContribution) * 90 / float64(el.Minutes)
	}
	return 0
}

// shrinkToLeague pulls a player with no prior toward his position's league-wide
// rates AND league-wide playing time, using the same weighting as an ordinary
// blend.
//
// Minutes used to be left alone here — only the counting-stat rates were
// shrunk, on the reasoning that a one-match debutant's 90 minutes is a
// statement of fact, not a sample to shrink. That left the trap this
// function exists to close on the RATE side wide open on the VOLUME side:
// MinutesRating (reliabilityFrom) is a bare function of MinutesPerMatch and
// StartShare, with no term for how many matches they rest on, so a
// promoted-club debutant who starts and plays a full ninety in his first
// match — HUL's McBurnie and Belloumi, 2026-27 GW1 — reads as StartShare
// 1.000, ExpectedMinutes 90, MinutesRating ~1.0: full certainty from n=1,
// the identical "his debut reads as forever" mistake described below for
// rates, just carried by the volume term instead of the rate term. It
// concentrated the wildcard/free-hit builder on exactly these players — up
// to 5 of 15 squad slots in the reported incident — because a well-shrunk,
// modest rate multiplied by a false-certain full-90 volume still outscores
// an established player whose volume is honestly uncertain.
//
// Extended 2026-08-23 to shrink MinutesPerMatch and StartShare too, on
// BlendMinutesK rather than LeagueShrinkK — the established-prior path
// below already keeps a volume weight separate from its rate weight
// (BlendRateK vs BlendMinutesK, shipped at 8 vs 5), so the no-prior path
// reuses that same split rather than inventing a third constant or
// conflating volume with rate. This is still NOT a calibrated result —
// BlendMinutesK was fit for the established-prior mix, not for this one —
// but it reuses a constant this project has already measured once
// (constants-and-sweeps) rather than asserting a fresh, untested number.
//
// ⚠️ The shrunk MinutesPerMatch/StartShare feed Score, which is the whole
// point — but they must NOT reach the eligibility floor
// (cutByExpectedMinutes, read off SettledMinutes). A first draft let them:
// even at the gentler BlendMinutesK, shrinking a heavy-blank gameweek's
// affordable pool toward the league average pushed enough of it below the
// minutes floor to make a £82.0m free hit unbuildable within budget
// (TestFreeHitNeverFieldsABlankingClub, 2021-22 GW18). That failure mode is
// not about the magnitude of the shrink; it is a category error — a floor
// asking "does he currently get picked" is a fact about today's team sheet,
// answered by GateMinutesPerMatch below, not a confidence question the
// SCORE side's uncertainty-shrink has any business narrowing. Both draws
// from the same real observation; only which question each answers differs.
// Ships because the ranking failure it closes is live and reproduced;
// BlendMinutesK's use here (as opposed to a dedicated constant) is still a
// candidate for the same backtest calibration LeagueShrinkK itself is owed.
func (e *Engine) shrinkToLeague(el *fpl.Element, b blend) blend {
	base, ok := e.leagueRates[el.ElementType]
	if !ok {
		return b
	}
	n90 := float64(el.Minutes) / 90
	k := e.Weights.LeagueShrinkK
	w := n90 / (n90 + k)
	mix := func(cur, league, weight float64) float64 {
		return weight*cur + (1-weight)*league
	}
	b.XG90 = mix(b.XG90, base.XG90, w)
	b.XA90 = mix(b.XA90, base.XA90, w)
	b.XGC90 = mix(b.XGC90, base.XGC90, w)
	b.DefCon90 = mix(b.DefCon90, base.DefCon90, w)
	b.Bonus90 = mix(b.Bonus90, base.Bonus90, w)
	b.Saves90 = mix(b.Saves90, base.Saves90, w)
	b.Yellow90 = mix(b.Yellow90, base.Yellow90, w)
	b.Red90 = mix(b.Red90, base.Red90, w)
	// Volume shrinks unconditionally, like every rate above it.
	//
	// ⚠️ IT WAS GATED ON `e.GameweeksPlayed() == 0` UNTIL 2026-08-25, AND THE
	// GATE WAS A WORKAROUND FOR A BUG IN THE OPTIMISER, NOT A STATEMENT ABOUT
	// EVIDENCE. Do not reintroduce it. The history is worth the lines because
	// the same mistake has now been made twice.
	//
	// e41d5bd2 (2026-08-23) added the volume shrink and scoped it to the live
	// GW1 gap, reasoning that past that window "n90 grows every week he plays
	// ... and w above is already correctly close to 1 by then". That is false
	// exactly where it has to be true. GameweeksPlayed() counts FINISHED
	// gameweeks, so the gate shut the instant GW1 went final — at n90 = 1,
	// where wMin = 1/(1+5) = 1/6, not near 1. A debutant with one 90-minute
	// appearance therefore read 90.0 minutes and a 1.00 start share: perfect
	// certainty about the player the model knew least about. Observed live on
	// 2026-08-25 with three promoted-club players in a published wildcard XI.
	//
	// The gate looked load-bearing because removing it failed
	// TestFreeHitNeverFieldsABlankingClub at 2021-22 GW18. That failure was
	// not real. A legal fifteen existed at £65.3m against the £82.0m budget —
	// £16.7m of headroom — and Optimize reported it could not be built,
	// because its admissibility bound ignored the three-per-club cap and let a
	// one-shot greedy walk into a corner it could not reverse. Shrinking
	// minutes cannot change a price; it changed ValueScore, hence the sort
	// order, hence which corner. Feasibility depended on score order. That is
	// fixed in fillBound (squad.go), and with it fixed this shrink is
	// unconditional and that test passes across all 12 blank gameweeks.
	//
	// TestAFinishedGameweekDoesNotMakeADebutantLookNailed pins the property
	// this restores: flipping Event.Finished alone, with no new football
	// played, must not change a debutant's reported volume.
	//
	// GateMinutesPerMatch is captured BEFORE the volume mix, so the
	// eligibility floor keeps judging "does he currently get picked" off what
	// he has actually done, unshrunk — see this function's own comment on why
	// that gate must not move even though Score does.
	b.GateMinutesPerMatch = b.MinutesPerMatch
	b.GateMinutesSet = true
	// Volume uses BlendMinutesK, not LeagueShrinkK — the same split the
	// established-prior path two screens down already keeps (BlendRateK for
	// rates, BlendMinutesK for MinutesPerMatch/StartShare, shipped at 5
	// against 8). Reusing LeagueShrinkK here would conflate two quantities
	// that path deliberately tunes apart.
	wMin := n90 / (n90 + e.Weights.BlendMinutesK)
	// ⚠️ The tilt multiplies the LEAGUE term and nothing else, so it fades with
	// evidence by construction rather than by a guard: the mix already weights
	// this term by `1 - wMin`, which is 1 for a player with no history and goes
	// to 0 as he plays. A player the model knows is untouched.
	leagueMin, leagueStart := base.MinutesPerMatch, base.StartShare
	tilt := e.priceMinutesTilt(el)
	if tilt != 1 {
		leagueMin *= tilt
		leagueStart *= tilt
	}
	b.MinutesPerMatch = mix(b.MinutesPerMatch, leagueMin, wMin)
	b.StartShare = mix(b.StartShare, leagueStart, wMin)
	// ⚠️ **Clamped only when the tilt actually fired, and that is not fussiness.**
	// `mix` is a convex combination, so with the lever off both outputs are
	// bounded by inputs this function did not change — meaning an unconditional
	// clamp here could only ever alter an EXISTING value, silently, under cover
	// of a knob that ships at zero. A lever that changes shipped behaviour while
	// switched off is the worst shape a lever can have.
	//
	// With the tilt on, the multiplier can push an individual past certainty
	// even though it is centred, and a start share above 1 would propagate as
	// confidence nobody has.
	if tilt != 1 {
		if b.StartShare > 1 {
			b.StartShare = 1
		}
		if b.MinutesPerMatch > 90 {
			b.MinutesPerMatch = 90
		}
	}
	b.Weight = w
	return b
}

// priceMinutesTilt is the multiplier this player's price earns on the
// league-average volume fallback. Returns exactly 1 when the lever is off, so
// the caller's `!= 1` check makes the whole feature free when unused.
//
// # Centred, and that is the load-bearing property
//
// The tilt is `1 + w*(2p - 1)` where `p` is the player's price percentile inside
// his own position. A median-priced player gets exactly 1; the most expensive
// gets `1 + w` and the cheapest `1 - w`. So it REORDERS without inflating — what
// one player gains another loses, and the position's average volume is
// unchanged.
//
// A tilt that raised everyone would be a change to the league fallback dressed
// as a signal, and it would show up as a points effect that had nothing to do
// with ranking anybody.
//
// ⚠️ **Percentile within POSITION, not across the league.** Goalkeepers and
// forwards live on different price scales, so a league-wide percentile would
// read every keeper as cheap and tilt the whole position down — a systematic
// change to one position's minutes, which is not what this is for.
//
// ⚠️ **Ties share a percentile.** FPL prices cluster hard at the bottom, where
// dozens of players sit at exactly 4.0m; splitting them by index would invent an
// ordering the price does not contain and hand it to an argmax.
// priceTiltFadesByGW is the gameweek at which the price tilt reaches zero.
//
// ⚠️ **It fades on the CALENDAR, and that is a claim about the SIGNAL rather
// than about the player.** The existing fade — the `1 - wMin` weight on the
// league term — is per player: it shrinks as HE accumulates minutes, which is
// the right shape for "how much do we still need a prior". This one is
// different and both are needed.
//
// Price is an expert judgement **at the season boundary**: FPL sets it before a
// ball is kicked, and nobody owns anybody yet. It stops being that as the season
// runs, because FPL revises price on transfer activity — so by GW10 it is partly
// the crowd chasing form, which is the thing the measurement showed is WEAKER
// than the model where history exists.
//
// Without a calendar fade a January signing with three appearances would be
// tilted as hard as an August one, on a price that by then encodes months of
// bandwagon rather than a pre-season forecast. The evidence behind this lever
// covers GW1-10 ordering and says nothing about that case.
//
// ⚠️ Not swept, and deliberately not a tuning target: it expresses WHEN the
// signal stops being what was measured, not where an optimum lies. Eleven is the
// middle of the owner's stated "by ten to twelve gameweeks we should fully trust
// actual data", and the fade is linear so there is no cliff for a squad to sit
// either side of.
const priceTiltFadesByGW = 11

func (e *Engine) priceMinutesTilt(el *fpl.Element) float64 {
	w := e.Weights.PriceMinutesPrior
	if w <= 0 || el == nil {
		return 1
	}
	// Linear from full weight pre-season to nothing by priceTiltFadesByGW.
	if played := e.GameweeksPlayed(); played > 0 {
		if played >= priceTiltFadesByGW {
			return 1
		}
		w *= 1 - float64(played)/float64(priceTiltFadesByGW)
	}
	// ⚠️ **The percentile map is computed once per Engine and never refreshed, and
	// `serve` builds ONE Engine at startup for every later reader.** The fade
	// above governs the tilt's WEIGHT, not this map's freshness, so they do not
	// cover each other: a server left up for days would keep tilting against
	// prices frozen at boot, while FPL revises price continuously on transfer
	// activity.
	//
	// Inert as shipped — `PriceMinutesPrior` is 0, so the guard above returns
	// before `Do` is ever reached — and this is the same write-once idiom as
	// `restOnce`, `bandOnce` and `confirmedOnce` beside it, which read data that
	// really is fixed for a season. Price is not.
	//
	// **So anyone turning this knob on for a long-lived `serve` owes it a
	// refresh, not just a sweep.** Recorded here rather than fixed because the
	// fix belongs with the decision to ship the lever: an invalidation hook for a
	// knob that is off is a mechanism with no caller, which this project has
	// already paid for once.
	e.priceOnce.Do(func() {
		byPos := map[int][]int{}
		for i := range e.Boot.Elements {
			p := &e.Boot.Elements[i]
			byPos[p.ElementType] = append(byPos[p.ElementType], p.NowCost)
		}
		pct := map[int]float64{}
		for i := range e.Boot.Elements {
			p := &e.Boot.Elements[i]
			costs := byPos[p.ElementType]
			if len(costs) < 2 {
				continue
			}
			// Share of the position priced strictly below him, plus half the
			// share priced the same — the mid-rank convention, so a block of
			// tied 4.0m players all land on one percentile instead of being
			// spread across a range the price never distinguished.
			below, equal := 0, 0
			for _, c := range costs {
				switch {
				case c < p.NowCost:
					below++
				case c == p.NowCost:
					equal++
				}
			}
			pct[p.ID] = (float64(below) + float64(equal)/2) / float64(len(costs))
		}
		e.pricePctile = pct
	})
	p, ok := e.pricePctile[el.ID]
	if !ok {
		return 1
	}
	return 1 + w*(2*p-1)
}

// leagueRate is a position's aggregate per-90 output, used as the fallback
// prior for players who have none of their own.
type leagueRate struct {
	XG90, XA90, XGC90, DefCon90 float64
	Bonus90, Saves90            float64
	Yellow90, Red90             float64
	// MinutesPerMatch and StartShare are the position's average PLAYING TIME,
	// not scoring output — the fallback for a no-prior player's own volume,
	// the same way the fields above are the fallback for his own rate. See
	// shrinkToLeague's own comment for why this exists alongside them.
	MinutesPerMatch, StartShare float64
}

// calibrateLeagueRates totals each position's output and minutes, so the
// fallback prior is the league's actual rate rather than a guess. Built once at
// construction and read-only thereafter.
func (e *Engine) calibrateLeagueRates() {
	type acc struct {
		mins, avail, starts                     float64
		xg, xa, xgc, dc, bonus, saves, yel, red float64
	}
	sums := map[int]*acc{}
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Minutes == 0 {
			continue
		}
		a := sums[el.ElementType]
		if a == nil {
			a = &acc{}
			sums[el.ElementType] = a
		}
		m := float64(el.Minutes)
		a.mins += m
		a.avail += float64(e.matchesAvailable(el))
		a.starts += float64(el.Starts)
		a.xg += el.ExpectedGoals.Float()
		a.xa += el.ExpectedAssists.Float()
		a.xgc += el.ExpectedGoalsConceded.Float()
		a.dc += defCon90(el) * m / 90
		a.bonus += float64(el.Bonus)
		a.saves += float64(el.Saves)
		a.yel += float64(el.YellowCards)
		a.red += float64(el.RedCards)
	}
	e.leagueRates = map[int]leagueRate{}
	for pos, a := range sums {
		if a.mins <= 0 {
			continue
		}
		r := 90 / a.mins
		lr := leagueRate{
			XG90: a.xg * r, XA90: a.xa * r, XGC90: a.xgc * r, DefCon90: a.dc * r,
			Bonus90: a.bonus * r, Saves90: a.saves * r,
			Yellow90: a.yel * r, Red90: a.red * r,
		}
		if a.avail > 0 {
			lr.MinutesPerMatch = a.mins / a.avail
			lr.StartShare = a.starts / a.avail
		}
		e.leagueRates[pos] = lr
	}
}
