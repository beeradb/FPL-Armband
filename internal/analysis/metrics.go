package analysis

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"armband/internal/fpl"
)

// Weights tune the scoring model. They live in config so the user can shift the
// agent's philosophy without touching code.
type Weights struct {
	// Horizon is how many upcoming gameweeks the fixture outlook covers.
	Horizon int `json:"fixture_horizon"`
	// MinutesHalfLife weights recent gameweeks more heavily when estimating
	// minutes, in gameweeks. Zero uses the flat season-to-date total.
	//
	// Minutes are a statement about the present. A player who lost his place six
	// weeks ago still reads as a starter on a season average, and the season
	// average is all FPL's bootstrap publishes. Predicting the next five
	// gameweeks' minutes across three replayed seasons, a half-life of 2 is 8.9%
	// better than the flat total (MAE 18.98 against 20.83) and every half-life
	// from 2 to 8 beats it.
	//
	// This does *not* extend to rates. The same test on points and on xG+xA says
	// sharp recency is actively worse — "last 3 games" is 19% worse than the
	// season average on both — because underlying quality is stable and short
	// windows chase finishing variance. Only minutes are weighted.
	//
	// # Why 4 and not 2
	//
	// The effect is asymmetric in a way the minutes-prediction test cannot see.
	// A short half-life is right about a player *losing* his place — five blanks
	// is decisive — and wrong about one *gaining* it, because two starts do not
	// make anyone nailed. Replaying three seasons, the error on the player being
	// sold and the player being bought move in opposite directions:
	//
	//	half-life   points   transfers   sell error   buy error
	//	flat          2152          63       -0.70       -0.04
	//	2             2149         119       -0.20       -0.95
	//	4             2199         114       -0.19       -0.29
	//	8             2204          80       -0.32       -0.20
	//
	// Points are flat from 3 to 8 and the choice is really about the other two
	// columns. 4 is the shortest setting that beat the flat total in all three
	// seasons while keeping the buy-side error small; 8 scores the same with
	// 30% fewer transfers but loses 2023-24. Treat anything in 3-8 as equivalent
	// on the evidence available.
	//
	// Using it needs per-gameweek history, which the bootstrap does not carry:
	// Engine.Recent must be supplied from element-summary or an archive.
	MinutesHalfLife float64 `json:"minutes_half_life"`

	// RateHalfLife weights recent gameweeks more heavily when estimating a
	// player's *output* rates — his form — in gameweeks. Zero uses the flat
	// season-to-date figure, which is what ships.
	//
	// Kept separate from MinutesHalfLife because the two were measured to want
	// opposite things. Minutes are a statement about the present and reward
	// sharp recency: a half-life of 2 predicts the next five gameweeks' minutes
	// 8.9% better than the flat total. Rates are a statement about quality and
	// punish it: "last 3 games" is 19% *worse* than the season average on both
	// points and underlying, because a short window chases finishing variance.
	//
	// Gentle recency did predict slightly better — 2.1% on points MAE at a
	// half-life of 8 — and lost points at every setting when it was last wired
	// into the replay. That measurement predates the scoring fixes and was taken
	// on the single-path policy metric, which is the class of finding that has
	// most often reversed, so the knob exists to re-run it.
	RateHalfLife float64 `json:"rate_half_life"`

	// PriorHalfLife blends seasons before the immediate prior into it, in
	// seasons. Zero uses the immediate prior alone.
	//
	// Only players whose immediate prior is thin AND NON-ZERO reach back —
	// ShouldBlendPrior is the gate and carries both halves of the reasoning. A
	// full season is the best evidence there is about a player, and smoothing an
	// older one into it dilutes genuine improvement, which is most players most
	// of the time. A season of no minutes at all is a different fact again: the
	// model already answers it through shrinkToLeague, and blending replaces that
	// with a season at least two years old.
	//
	// Isak is the case: 694 minutes in 2025-26 with a broken leg at 0.346
	// xG+xA per 90, against 2758 and 2253 minutes at 0.781 and 0.915 before it.
	// Blending puts him at 0.726 while leaving a consistent fringe player like
	// Diop at 0.063.
	//
	// Off by default. Two replayed seasons put it 7 points down, but neither
	// contains the case it is for — in both, the prior season is a healthy one —
	// so that number measures the cost and not the benefit.
	//
	// The non-zero half of the gate is new and is why the shipped number above
	// may understate the cost. Ungated, one-gameweek-ahead prediction over six
	// seasons had the setting order the whole field measurably WORSE — Δ rank
	// correlation −0.00225 at half-life 1, season-clustered t −3.70 — and the gate
	// removes about ninety per cent of that, to −0.00020 at t −0.52. The default
	// did not change: this is what it would now do if it were turned on, and no
	// replay has priced it. See cmd/priorblend.
	PriorHalfLife float64 `json:"prior_half_life"`

	// BonusWeight scales the historical bonus-points term. Default 1.5 — the
	// EVIDENCE end of a schedule, APPROACHED and never reached as current-season
	// minutes accumulate: with blend_rate_k 8 an ever-present sits at ~1.33 after 38
	// full matches, so the applied range is 0.5 to ~1.33. BonusPriorWeight (0.5) is
	// the prior end; see bonusWeightFor.
	//
	// ⚠️ The 1.0 the sweep table below peaks at is THIS field under the retired FLAT
	// regime — swept in a0bb150, before BonusPriorWeight existed, and still reachable
	// today as bonus_prior_weight: -1, which short-circuits the schedule. It is not
	// "some other knob", and it is not this field's default under the schedule.
	// Saying "Default 1" here outlived 10da7a2 and was misread once as "BonusWeight
	// ships at 1.0".
	//
	// The term is circular: BPS is driven by goals, assists, clean sheets, saves
	// and defensive actions, every one of which the model already prices, so
	// adding a player's own bonus rate counts them twice. It is worth 16% of the
	// mean score and 25% of Haaland's.
	//
	// Scoring 248 player-seasons from the prior season alone and grouping by the
	// size of the term shows the same signature the set-piece bonus had:
	//
	//	bonus/90 group     n    bias   term   bias-term
	//	lowest quarter    62  +0.531  0.085     +0.617
	//	second            62  +0.487  0.226     +0.713
	//	third             62  +0.217  0.363     +0.580
	//	highest quarter   62  -0.159  0.726     +0.567
	//
	// The under-prediction shrinks monotonically as the term grows and turns
	// negative at the top: high-bonus players are over-credited by roughly their
	// own term. Removing it collapses the spread across groups from 0.690 to
	// 0.146, leaving a near-uniform under-prediction of about 0.6 per 90.
	//
	// That residual matters less than it looks. A uniform bias does not reorder
	// anyone, and the optimiser consumes an ordering.
	//
	// # The curve peaks at 1.0 in both directions
	//
	//	weight  2023-24  2024-25  2025-26   mean
	//	0.00       2095     2244     2083   2141
	//	0.50       2128     2269     2111   2169
	//	1.00       2115     2377     2132   2208
	//	1.25       2141     2191     2081   2138
	//	2.00       2051     2206     1977   2078
	//
	// The full historical rate is already the right amount: discounting it for
	// being circular loses 67 points a season, and leaning further into it loses
	// 70. Bonus is not a tiebreaker in this model — it is 16% of the mean score
	// and the second-largest term for a premium, behind only expected goals.
	//
	// It is a genuine player property rather than season noise: bonus/90
	// correlates +0.469 between 2024-25 and 2025-26 across 103 players with
	// 1,800 minutes in both. That is real and moderate, which is consistent with
	// a term worth carrying at face value and not worth amplifying.
	BonusWeight float64 `json:"bonus_weight"`

	// BandStrength scales the 3/14/3 attack/defence band adjustment. Zero
	// disables it, which is the shipped setting; see bands.go for the
	// measurement behind the band sizes.
	// DefConCleanCoupling links the clean sheet to a defender's own defensive
	// workload, which the model otherwise treats as independent of it. See
	// defconClean in teamstrength.go. Zero disables it.
	DefConCleanCoupling float64 `json:"defcon_clean_coupling"`

	// BlankRunPenalty multiplies expected minutes for a player in the first few
	// gameweeks of an absence, where the recency weighting has not yet caught
	// up. See blankRunFactor for the measurement. 1.0 disables it.
	//
	// Anything <= 0 is treated as unset and takes the default, because unlike
	// BonusPriorWeight there is no useful zero here: it would assert that one
	// blank means never playing again, which the data contradicts outright — 18%
	// of them vanish, not 100%.
	BlankRunPenalty float64 `json:"blank_run_penalty"`

	// BonusPriorWeight is the bonus weight applied when a player's bonus rate is
	// entirely last season's, with BonusWeight applied when it is entirely this
	// season's and the two interpolated between. See bonusWeightFor for the
	// measurement. Negative disables the schedule and applies BonusWeight
	// throughout, which is what shipped before it was measured.
	BonusPriorWeight float64 `json:"bonus_prior_weight"`

	BandStrength float64 `json:"band_strength"`

	// FixtureWeight blends fixture adjustment into the base rate.
	// 0 = ignore fixtures entirely, 1 = fully fixture-adjusted.
	FixtureWeight float64 `json:"fixture_weight"`
	// SetPieceWeight scales the points-per-90 bonus credited for set-piece duty.
	//
	// **Defaults to 0, and that is deliberate.** FPL's `expected_goals` already
	// includes penalties, and `expected_assists` already includes corners and
	// free kicks, so crediting set-piece duty on top counts the same output
	// twice. Measured per appearance on midfielders and forwards who start and
	// finish, first-choice penalty takers were over-predicted by 0.400 points
	// while their set-piece term was worth 0.393 — the entire bonus was
	// redundant. Zeroing it collapsed the taker-versus-non-taker spread from
	// 0.58 to 0.21. See TestSetPieceBonusDoubleCountsPenalties.
	//
	// The one thing this gives up is pricing a *newly appointed* taker, whose
	// expected goals contain no penalties yet. The `penalties #1` flags are
	// still reported, so that case is visible to the agent even though it is
	// not scored. Fixing it properly needs non-penalty xG, which FPL does not
	// publish and which FBref stopped publishing in January 2026.
	SetPieceWeight float64 `json:"set_piece_weight"`
	// minutes_floor was removed. It claimed to be "the total minutes below which
	// a player is treated as a rotation/sample-size risk and discounted", was
	// reported to the agent in those words, and no scoring path ever read it.
	// Both jobs it described are done elsewhere and were measured there: sample
	// size by BlendRateK and shrinkToLeague, rotation by MinutesRating. A knob
	// that documents a behaviour the model does not have is worse than no knob,
	// because it is the agent's stated reason for trusting a number.

	// BenchWeight is how much bench players count when scoring a 15-man squad.
	// Defaults to DefaultBenchWeight, which is the measured value; it used to
	// default to 0.15 independently and disagree with it.
	BenchWeight float64 `json:"bench_weight"`
	// MinutesWeight is the exponent applied to minutes reliability. 1.0 scales
	// expected points linearly with expected minutes, which is the
	// mathematically neutral choice. Above 1.0 punishes rotation risk harder,
	// which is usually right for FPL: a blank from a benched player costs you
	// the captaincy, the transfer and the bench slot, not just the points.
	MinutesWeight float64 `json:"minutes_weight"`
	// MinutesWeightByPosition scales how hard the minutes penalty bites per
	// position, as a fraction of the global setting's severity. 1.0 applies
	// MinutesWeight in full; 0.75 applies three quarters of its bite; 0 makes
	// the position's minutes penalty neutral.
	//
	// Midfielders default to 0.75. A midfielder on 65 minutes is often a
	// genuine starter who gets subbed in winning positions, and midfield
	// returns (goals, assists, defensive contributions) accrue in the minutes
	// he does play, where a defender's clean sheet is all or nothing. Note the
	// asymmetry is smaller than it looks: the clean sheet needs 60 minutes, not
	// 90, the same threshold as appearance points.
	MinutesWeightByPosition map[string]float64 `json:"minutes_weight_by_position"`

	// RestPlayers are players expected to be rested or eased back in after a
	// summer tournament. Names or FPL ids.
	RestPlayers []string `json:"rest_players"`
	// RestRegions are FPL nationality codes to apply the same factor to.
	// Use the `nations` command to find the code for a country.
	RestRegions []int `json:"rest_regions"`
	// RestMinutesFactor multiplies the expected MINUTES of flagged players
	// returning from a summer tournament (0.83 = -17% of minutes, prorated
	// across the horizon by restFactor). It is not a Score multiplier — see
	// blendFor for why it sits on the minutes channel.
	//
	// # Measurement
	//
	// TestDiagRestPooled measures it across the two tournament summers the
	// archive reaches — Euro 2020 / Copa 2021 into 2021-22, and Euro 2024 /
	// Copa 2024 into 2024-25 — comparing each flagged man's opening two
	// gameweeks against the same man's openings in the seasons he had an
	// ordinary summer, all baselined on the *previous* season's rate:
	//
	//	                  minutes   points
	//	group means         0.87     0.79
	//	per player (n=33)   0.83     0.70
	//
	// The shipped 0.83 is the per-player figure from the minutes column. That
	// estimator is preferred over the group means' 0.87 because it is the
	// within-player one — restricted to the 33 men observed in both a
	// post-tournament and a control season — and it is the only one that yields
	// a standard error. The evidence cannot actually separate the two; 0.83 to
	// 0.87 should be read as one answer.
	//
	// The points column is deliberately NOT used to set this. The convex minutes
	// exponent already converts a minutes factor into a points effect, so fitting
	// this to a points outcome would count the same convexity twice. At the
	// shipped exponent of 1.25 a 0.83 minutes factor lands at 0.83^1.25 = 0.79 on
	// Score, which is almost exactly the measured points figure — the model's own
	// convexity reproduces the observed points effect from the observed minutes
	// effect, which is the check worth having.
	//
	// The 95% intervals are wide — [0.60, 1.06] on minutes, [0.30, 1.10] on
	// points — so this is a consistent direction rather than a precise number.
	// Both cohorts and both metrics agree in sign.
	//
	// Do not set 0.85. It is not a measured value — it was an interpolation
	// between the two estimators — and on the 2026/27 pre-season pool it happens
	// to land on a search instability: TestNoPremiumSquadBeatsTheOptimum fails
	// there and passes at 0.83, 0.87, 0.90, 0.80 and 0.75. That is a latent
	// weakness in Optimize rather than anything about rest, but it is a live
	// tripwire and worth knowing about.
	//
	// # It is an availability effect, not a quality one
	//
	// Splitting the measurement by channel is what put it here. Per-90 output
	// does not reliably move — the two cohorts disagree on its sign (+0.106 and
	// -0.231) and pool to about -0.05 — while minutes (-0.202, -0.055) and the
	// share falling below half their usual minutes (+9pp, +17pp) agree in sign
	// in both. So the rates are left alone and only minutes are scaled.
	//
	// This is the same channel as the managerial-change term, which is switched
	// off. They differ in the thing that decides whether to price it at all: a
	// new manager's minutes loss is cancelled by the survivors' per-90 gains,
	// and rest's is not.
	//
	// # The baseline has to predate the tournament
	//
	// An earlier attempt baselined each player against his own GW3-12 of the
	// same season and found no effect at all. That design conditions on a
	// post-treatment outcome: a player eased in through August who never got
	// going is filtered out of his own treatment group. Correcting it flipped
	// the sign. Do not re-derive this against a same-season baseline.
	//
	// # It only holds for the sharp list
	//
	// This is calibrated against DefaultRestPlayers' rule — on the pitch for the
	// majority of his semi-final. Widening the group to squad membership dilutes
	// it to nothing, because an unused substitute had a long summer in a hotel,
	// not a punishing one. A nationality-level version of the same test finds no
	// effect whatsoever, which is why RestRegions is empty.
	RestMinutesFactor float64 `json:"rest_minutes_factor"`

	// LegacyRestDiscount carries the pre-rename "rest_discount" key, which was a
	// Score multiplier rather than a minutes one. It exists only so config.Load
	// can migrate an old file instead of silently ignoring the key and quietly
	// dropping the term — the failure mode this codebase has hit repeatedly.
	// Nothing reads it during scoring.
	LegacyRestDiscount float64 `json:"rest_discount,omitempty"`
	// RestGameweeks is how many gameweeks from the season's start the discount
	// applies for. Beyond that, players are assumed fully reintegrated.
	//
	// Keep this short. Players get three weeks of mandatory rest after a
	// tournament, not three weeks of absence — in 2026 the final was 19 July,
	// so they were back in training around 9 August and had twelve days of
	// pre-season before the GW1 deadline on 21 August. An earlier default of 4
	// ran the discount to 12 September, nearly two months after the final.
	RestGameweeks int `json:"rest_gameweeks"`

	// BlendMinutesK and BlendRateK are the prior's strength when mixing last
	// season into this one — see blend.go, where both are measured rather than
	// chosen. Higher means slower to believe the current season.
	BlendMinutesK float64 `json:"blend_minutes_k"`
	BlendRateK    float64 `json:"blend_rate_k"`

	// LeagueShrinkK is shrinkToLeague's own strength — how fast a player with no
	// prior at all (a promoted club's starter, an arrival from abroad) is
	// trusted on his own current-season sample rather than his position's
	// league-wide rate. It reused BlendRateK until split out and measured
	// separately, and the split did not change the value.
	//
	// Out-of-sample, predicting the next five gameweeks' xG/90 from a
	// season-to-date blend over 2,700 zero-prior player-cutoffs across four
	// seasons, K=2 beats the shared K=8 in THREE of four seasons (MAE
	// 0.0774-0.0811 against 0.0761-0.0839) — ⚠️ corrected 2026-08-15 from "every
	// season individually", which those very ranges refute: K=8's best season
	// (0.0761) beats K=2's best (0.0774), and it is 2022-23 that dissents, the
	// season repaired the day after this was measured. Wired into the replay it is the
	// "recency on rates" failure again: HOLD is flat (+0.0095/gw, t=0.03) but
	// POLICY is negative at K=2 (-0.843/gw, t=-1.94, ~-32/season) — a better
	// predictor buying accuracy on the average zero-prior player and paying
	// for it in variance on the ones a transfer search actually reaches for,
	// exactly the population the debut-explosion bug this term exists to fix
	// lives in. K=4 is milder but still costs POLICY (t=-1.26). Stays at 8.
	LeagueShrinkK float64 `json:"league_shrink_k"`

	// TournamentAbsences record mid-season international tournaments that
	// overlapped the season the aggregate stats come from. Their participants
	// were unavailable for league football, not rotated out of it, so those
	// matches are removed from the denominator rather than counted as evidence
	// of rotation risk. See TournamentAbsence.
	TournamentAbsences []TournamentAbsence `json:"tournament_absences"`
}

// TournamentAbsence is a squad-wide absence from league football for an
// international tournament that ran *during* the season the aggregates come
// from — as distinct from RestPlayers, which handles the summer tournament
// immediately before the season now starting.
//
// The two are different problems. A summer tournament shortens a player's
// off-season, so it discounts his *score*. A mid-season tournament removes him
// from matches he was never available for, so it corrupts the *denominator*:
// minutes and starts are divided by 38 whether or not 38 was ever on offer.
//
// Without this, a player who left for four weeks reads as a rotation risk.
// AFCON 2025 ran 21 December to 18 January; Senegal won it and Nigeria finished
// third, so their Premier League contingent missed up to six league matches
// each. Iliman Ndiaye's 2,780 minutes over 32 starts is 73 min/gw against 38 —
// "likely starter" — and 87 min/gw against the 32 he was available for, which
// is nailed. The model had no way to tell that apart from being dropped.
//
// Injuries deliberately do NOT belong here. The model cannot distinguish
// "injured for three months" from "not picked", and guessing would hand back
// minutes to players who genuinely lost their place. A tournament is different:
// participation is a matter of public record, the dates are fixed, and every
// call-up is knowable in advance.
type TournamentAbsence struct {
	// Name identifies the tournament and how far this group went, e.g.
	// "AFCON 2025 — finalists". Reported verbatim on the player.
	Name string `json:"name"`
	// Matches is the number of league fixtures this group missed. It is capped
	// by the player's own record, so over-estimating cannot invent minutes.
	Matches int `json:"matches"`
	// Players are names or FPL ids, matched exactly as RestPlayers are.
	Players []string `json:"players"`
}

func DefaultWeights() Weights {
	return Weights{
		Horizon: 5,
		// Bonus is a schedule, not a constant: 0.5 where the rate is entirely last
		// season's, 1.5 where it is entirely this season's, interpolated on the
		// evidence share between. Measured on the held opening fifteen over four
		// seasons and three start points it beats a flat 1.0 by 383 held and 141
		// with transfers, and wins at every start point. See bonusWeightFor.
		BonusWeight:      1.5,
		BonusPriorWeight: 0.5,
		// Couples the clean sheet to a defender's own defensive workload. Ships
		// at the smallest value tested that helps both metrics; the mechanism is
		// well measured and the size is not. See defconCleanFactor.
		DefConCleanCoupling: 0.3,
		BlankRunPenalty:     0.75,
		FixtureWeight:       0.65,
		MinutesHalfLife:     4,
		SetPieceWeight:      0.0, // see the field comment: it double-counts penalties
		BenchWeight:         DefaultBenchWeight,
		MinutesWeight:       1.25,
		// Midfielders carry three quarters of the rotation severity. This was
		// nearly deleted as an unmeasured assertion — swept against the old
		// reliability mix it spanned 13 points, which is nothing — and that sweep
		// was invalidated by changing the mix. Re-run against minutes-only
		// reliability the range spans 226 points, with a plateau from 0 to 0.75
		// (8264-8342) and a cliff at 0.9 (8123).
		//
		// The two knobs act on the same players, which is why they are coupled.
		// The old mix credited a starter who is substituted for having started, so
		// a midfielder taken off at 65 minutes kept most of his rating. Minutes
		// only removed that prop, and midfielders are the position it was holding
		// up — so the per-position scale went from decorative to load-bearing the
		// moment the mix changed. 0 is nominally best by 38 over 0.75; the plateau
		// cannot separate them, so the incumbent stays.
		MinutesWeightByPosition: map[string]float64{
			"GKP": 1.0, "DEF": 1.0, "MID": 0.75, "FWD": 1.0,
		},
		BlendMinutesK:      5,
		BlendRateK:         8,
		LeagueShrinkK:      8,
		RestPlayers:        DefaultRestPlayers(),
		RestRegions:        []int{},
		RestMinutesFactor:  0.83,
		RestGameweeks:      2,
		TournamentAbsences: DefaultTournamentAbsences(),
	}
}

// DefaultTournamentAbsences lists the mid-season international tournaments that
// overlapped the 2025/26 Premier League season, whose participants' aggregate
// stats therefore understate how nailed they are.
//
// Like the European campaigns and the rest list, this is season-specific and
// must be cleared and re-derived every summer. It describes the season the
// *stats* came from, not the season being played.
//
// # AFCON 2025
//
// Held in Morocco from 21 December 2025 to 18 January 2026 — squarely inside
// the congested festive programme. Thirty-three Premier League players left in
// December and the deepest runs cost six league matches each.
//
// Grouped by how far the nation went, because that is what determines the
// absence. Senegal won it, beating hosts Morocco in the final; Nigeria finished
// third; Egypt and Morocco went out in the semi-finals; holders Ivory Coast
// lost in the quarter-finals.
//
// # Being on the list is a claim of participation, not of nationality
//
// Ghana is deliberately absent. Antoine Semenyo holds a Ghanaian passport and
// started 37 of 38 league matches, which is proof he did not go — blanket-
// flagging a nationality would have handed him minutes he never missed.
//
// Over-estimating Matches is safe (it is capped by the player's own record);
// listing a player who did not go is not. TestTournamentAbsenceNamesResolve
// fails loudly if a name stops matching, because the correction would otherwise
// silently stop applying.
func DefaultTournamentAbsences() []TournamentAbsence {
	return []TournamentAbsence{
		{
			Name:    "AFCON 2025 — Senegal (winners)",
			Matches: 6,
			Players: []string{
				"Iliman Ndiaye", "Ismaïla Sarr", "Pape Matar Sarr", "Habib Diarra",
			},
		},
		{
			Name:    "AFCON 2025 — Nigeria (third place)",
			Matches: 6,
			Players: []string{"Alex Iwobi", "Calvin Bassey", "Ola Aina"},
		},
		{
			Name:    "AFCON 2025 — Morocco (hosts, semi-finalists)",
			Matches: 6,
			Players: []string{
				"Chemsdine Talbi", "Noussair Mazraoui", "Amine Adli", "Issa Diop",
			},
		},
		{
			Name:    "AFCON 2025 — Egypt (semi-finalists)",
			Matches: 6,
			Players: []string{"Omar Marmoush"},
		},
		{
			Name:    "AFCON 2025 — Ivory Coast (quarter-finalists)",
			Matches: 5,
			Players: []string{
				"Amad Diallo", "Ibrahim Sangaré", "Evann Guessand", "Simon Adingra",
			},
		},
		{
			// Cameroon, Algeria and Burkina Faso were all out by the
			// quarter-finals. Three is the conservative figure — under-crediting
			// a genuine absence costs less than inventing one.
			Name:    "AFCON 2025 — group and last-16 exits",
			Matches: 3,
			Players: []string{
				"Bryan Mbeumo", "Carlos Baleba", "Rayan Aït-Nouri", "Dango Ouattara",
			},
		},
	}
}

// DefaultRestPlayers lists the Premier League players who actually carried a
// deep 2026 World Cup run, and therefore have the shortest off-season before
// the league starts. Like the European campaigns, this is season-specific and
// must be cleared and re-derived every summer.
//
// The four semi-finalists were Spain (who beat Argentina 1-0 after extra time in
// the final on 19 July), Argentina, France and England — all still playing under
// four weeks before the Premier League opener. Quarter-final exits finished a
// week earlier and are not flagged.
//
// # Squad membership is not the test
//
// The selection rule is: on the pitch for the majority of his semi-final —
// started, or on before half-time. A semi-final is where a manager fields his
// strongest available side, so the teamsheet is the best single read on who
// actually accumulated tournament minutes.
//
// This matters enormously. Of the 36 Premier League players across the four
// squads, only 17 clear the bar. Bukayo Saka, Ollie Watkins, Eberechi Eze,
// David Raya, Martín Zubimendi, Yéremy Pino, both Hendersons and six others
// were unused substitutes in their semi-final — they had a long summer in a
// hotel, not a punishing one, and discounting them was simply wrong.
//
// The threshold is not fragile. Maxence Lacroix came on at 30 minutes for the
// injured Saliba and played 60; the next-longest appearance off any of the four
// benches was Rayan Cherki's 18 minutes. There is nothing in between to argue
// about.
//
// Names are full names as the FPL API spells them, including accents, because
// matching is exact-then-fuzzy and surnames collide — there are two Martínezes
// here.
//
// RestRegions is deliberately left empty. Flagging a nationality would discount
// every English, Spanish, French and Argentine player in the game, most of whom
// spent the summer on a beach.
func DefaultRestPlayers() []string {
	return []string{
		// France — semi-final 14 Jul (lost 0-2 to Spain), third-place play-off 18 Jul.
		"William Saliba", "Lucas Digne", "Maxence Lacroix",
		// Spain — semi-final 14 Jul, final 19 Jul (winners).
		"Pedro Porro Sauceda", "Rodrigo 'Rodri' Hernandez Cascante",
		// England — semi-final 15 Jul (lost 1-2 to Argentina), third-place play-off 18 Jul.
		"Jordan Pickford", "Reece James", "Marc Guéhi", "Djed Spence",
		"Declan Rice", "Elliot Anderson", "Morgan Rogers",
		// Argentina — semi-final 15 Jul, final 19 Jul (runners-up).
		"Emiliano Martínez Romero", "Lisandro Martínez", "Cristian Romero",
		"Enzo Fernández", "Alexis Mac Allister",
	}
}

// FPL scoring constants.
var goalPoints = map[int]float64{1: 10, 2: 6, 3: 5, 4: 4}
var cleanSheetPoints = map[int]float64{1: 4, 2: 4, 3: 1, 4: 0}

// concedeBlock is how many goals conceded in a single match cost a point:
// −1 per 2 for keepers and defenders. Midfielders and forwards take no
// deduction and are absent from the map.
//
// This was missing from the model entirely, which made goalkeepers — who have
// no attacking output to dilute it — the worst-calibrated position in the game,
// over-predicted by 0.65 points per 90. Reconstructing seventeen keepers'
// seasons from FPL's published rules missed by 25.8 points each without this
// term and 1.9 with it.
var concedeBlock = map[int]int{1: 2, 2: 2}

const (
	assistPoints = 3.0
	// Appearance points are a two-step, and FPL names the steps: short_play is 1
	// point for recording any minutes at all, long_play is 2 at sixty minutes or
	// more. appearancePoints is the long_play value, which is what a per-90 rate
	// should carry — a full match always clears the hour. The step between them is
	// priced per gameweek, in appearanceFactor.
	appearancePoints = 2.0
	shortPlayPoints  = 1.0
	// Defensive contribution: 2 pts at 10 CBIT (DEF) or 12 CBIRT (MID/FWD).
	defConPoints = 2.0
	// savesBlock is how many saves in a match earn a goalkeeper a point.
	savesBlock = 3
	// Card deductions, as positive magnitudes — FPL publishes them negative and
	// the model subtracts them. Named rather than inline so
	// TestScoringConstantsMatchFPL can assert them against the published table;
	// they were two bare literals inside baseXP90, which is exactly the shape of
	// constant this project has repeatedly found to be unchecked.
	yellowCardPoints = 1.0
	redCardPoints    = 3.0
)

// PlayerMetrics is the derived view of a player the agent reasons over.
type PlayerMetrics struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Team     string  `json:"team"`
	Position string  `json:"position"`
	Price    float64 `json:"price_m"`

	Status     string `json:"availability"`
	News       string `json:"news,omitempty"`
	ChancePlay *int   `json:"chance_of_playing_next_round,omitempty"`

	Minutes         int     `json:"minutes"`
	Starts          int     `json:"starts"`
	ExpectedMinutes float64 `json:"expected_minutes_per_gw"`
	// SettledMinutes is ExpectedMinutes with the post-tournament rest factor
	// taken back out — what this player is expected to play once he is eased
	// back in. It is what the rotation-risk *filters* screen on.
	//
	// The distinction matters because those filters are cliffs. A nailed starter
	// on 58 expected minutes who is discounted to 54 for the opening fortnight
	// would drop under a 55-minute floor and vanish from the squad pool
	// altogether — not be scored slightly lower, but be un-pickable. That is a
	// far bigger claim than the evidence supports: the filter asks whether a
	// player holds a regular place, which is a question about his settled role,
	// not about a two-gameweek easing-in.
	//
	// So Score and the rotation_risk band read the rested figure, and pool
	// membership reads this one.
	SettledMinutes float64 `json:"settled_minutes_per_gw,omitempty"`
	StartShare     float64 `json:"start_share"`         // fraction of available matches started
	MinutesRating  float64 `json:"minutes_reliability"` // 0-1
	RotationRisk   string  `json:"rotation_risk"`

	// PriorWeight is the share of this player's rates taken from the current
	// season rather than last. 1.0 pre-season and for anyone with no prior;
	// it climbs as the season accumulates evidence. See blend.go.
	PriorWeight float64 `json:"current_season_weight"`
	// DefCon90 is defensive contributions per 90, blended like the other rates.
	DefCon90 float64 `json:"defensive_contribution_per_90"`
	// Bonus90, Saves90, Yellow90 and Red90 are counting stats per 90, blended
	// for the same reason: raw, they are the most explosive terms early on.
	Bonus90  float64 `json:"bonus_per_90"`
	Saves90  float64 `json:"saves_per_90"`
	Yellow90 float64 `json:"yellow_per_90"`
	Red90    float64 `json:"red_per_90"`

	// MatchesAvailable is the denominator behind ExpectedMinutes and
	// StartShare: league matches the player could have been picked for. It is
	// 38 unless a mid-season tournament took him away — see TournamentAbsence.
	MatchesAvailable int `json:"matches_available"`
	// TournamentAbsence names that tournament, when one applies, so the number
	// above can be explained rather than asserted.
	TournamentAbsence string `json:"mid_season_tournament,omitempty"`

	// NewSigning marks a player who joined this club after the previous season
	// ended, so his stats above were accumulated elsewhere.
	NewSigning bool   `json:"new_signing,omitempty"`
	JoinedDate string `json:"joined_club,omitempty"`

	// RestRisk is set when the player is flagged as needing post-tournament
	// rest, and carries the discount applied to his score.
	RestRisk          string  `json:"rest_risk,omitempty"`
	RestMinutesFactor float64 `json:"rest_minutes_factor,omitempty"`

	// Congestion captures European, international and travel load over the
	// horizon. 1.0 means no extra load; 0.85 means a 15% expected-minutes hit.
	Congestion      float64  `json:"congestion_factor"`
	CongestionNotes []string `json:"congestion_reasons,omitempty"`

	// RoleFactor prices uncertainty about whether the player's statistical
	// record still applies: a summer transfer, or a new manager at the club.
	RoleFactor float64  `json:"role_certainty_factor"`
	RoleNotes  []string `json:"role_risk_reasons,omitempty"`

	// AvailabilityFactor is the discount for being injured, suspended or
	// doubtful — FPL's own status flag turned into a multiplier on Score.
	//
	// Reported for the standing reason that every scoring term here is a
	// reported multiplier: it is the term that most often explains a low number
	// on a player who is otherwise obviously good, and until now it was the one
	// factor in the Score expression with no field to read it from. A consumer
	// could infer it from Status and ChancePlay, but inferring it means a second
	// implementation of availabilityFactor's table, which is this project's
	// most-repeated bug.
	//
	// Deliberately NOT `omitempty`. The value that matters most is 0 — a
	// ruled-out player, whose Score is zero for that reason and no other — and
	// omitempty would drop exactly that case while keeping the uninformative
	// 1.0s.
	AvailabilityFactor float64 `json:"availability_factor"`

	XG90  float64 `json:"xg_per_90"`
	XA90  float64 `json:"xa_per_90"`
	XGI90 float64 `json:"xgi_per_90"`
	XGC90 float64 `json:"xgc_per_90"`
	// FixtureLoad is matches per gameweek over the horizon: 1.0 normally, above
	// it for a double, below for a blank. Reported because it is a multiplier on
	// Score and every scoring term here is a reported multiplier.
	FixtureLoad float64 `json:"fixtures_per_gameweek"`
	// loadInScore records whether Score already carries FixtureLoad, so a second
	// consumer cannot apply it twice. Unexported deliberately: it is an internal
	// bookkeeping fact rather than a scoring term, and tool output is replayed on
	// every API call, so it must not reach the JSON.
	//
	// It exists because the condition has two consumers and they are in different
	// files. XIValue used to assert the collision was impossible in a comment —
	// "that path picks the eleven through BestXI and never calls this" — which
	// was true when written and was falsified by SimConfig.AnticipateChips, which
	// puts the *transfer* engine at horizon 1 in the gameweek before a chip and
	// so squares the multiplier for a doubling club. One quantity with two
	// implementations and an invariant comment a later change broke is this
	// package's most expensive recorded bug class; carrying the fact on the value
	// rather than re-deriving the condition is what stops it recurring.
	loadInScore bool
	// loadSet records that FixtureLoad was computed, which is a different fact
	// from its value.
	//
	// It became necessary when `fixtureLoadFor` learned to return a real 0 for a
	// club that blanks the whole window. Before that, `FixtureLoad > 0` was a
	// sound test for "this came from Metrics", because the function could not
	// return anything else; afterwards it silently reads a genuine blank as an
	// unpopulated field, and the two callers below both respond by *skipping the
	// multiply* — so the one player who certainly scores nothing is valued at his
	// full score in the transfer objective. That is the very defect the anchor fix
	// exists to remove, surviving in the objective the fix does not touch.
	//
	// Reachable at a horizon of 2 to 4, which is not exotic: `EffectiveHorizon`
	// shortens the transfer horizon to the gap before a planned wildcard, and the
	// archive holds blank runs of two and three consecutive rounds. It is NOT
	// reachable at horizon 1, where `loadInScore` is true and the second guard
	// short-circuits before reading the load at all.
	//
	// Unexported for the same reason `loadInScore` is: bookkeeping, not a scoring
	// term, and tool output is replayed on every API call.
	loadSet bool
	// TeamXGCFactor is the club-level correction applied to XGC90, if any.
	// Reported because every scoring term is a reported multiplier: a defender
	// marked down for his club's changed back line should say so.
	TeamXGCFactor float64 `json:"team_xgc_factor,omitempty"`

	// TeamFormFactor is the attacking mirror: the club-level correction applied to
	// XG90 and XA90 when the club-form blend is on. Reported for the same reason,
	// and absent from the JSON when it is 1, which is every shipped run.
	TeamFormFactor float64 `json:"team_form_factor,omitempty"`

	// XGScale and XAScale are the position's conversion from FPL's expected
	// figures to the events FPL awards points for, measured across the league
	// each run. XG90 and XA90 above are FPL's raw numbers; scoring multiplies
	// them by these. See calibrateExpectedStats.
	//
	// Reported here, on the full single-player view, rather than on the compact
	// tool row — they are per-position constants, so repeating them on every row
	// of every search would be paid for on each replayed API call for no gain.
	XGScale float64 `json:"xg_to_goals_scale"`
	XAScale float64 `json:"xa_to_assists_scale"`

	Goals       int     `json:"goals"`
	Assists     int     `json:"assists"`
	TotalPoints int     `json:"total_points"`
	PPG         float64 `json:"points_per_game"`
	Form        float64 `json:"form"`
	Bonus       int     `json:"bonus"`
	Ownership   float64 `json:"selected_by_percent"`

	// Finishing is actual goals minus xG — positive means overperforming
	// (likely to regress), negative means due.
	Finishing float64 `json:"goals_minus_xg"`

	PenaltyOrder  *int   `json:"penalty_order,omitempty"`
	CornerOrder   *int   `json:"corner_fk_order,omitempty"`
	DirectFKOrder *int   `json:"direct_fk_order,omitempty"`
	SetPieceNote  string `json:"set_piece_note,omitempty"`

	// FixtureRuns describes the next Horizon gameweeks.
	AvgDifficulty float64        `json:"avg_fixture_difficulty"`
	Fixtures      []FixtureBrief `json:"upcoming_fixtures"`

	// Expected points per 90 minutes, before and after fixture adjustment.
	BaseXP90       float64 `json:"base_xp_per_90"`
	SetPieceXP90   float64 `json:"set_piece_xp_per_90"`
	FixtureAdjXP90 float64 `json:"fixture_adjusted_xp_per_90"`

	// Score is the headline number: expected points per gameweek over the
	// horizon, accounting for fixtures, minutes risk and availability.
	Score      float64 `json:"score"`
	ValueScore float64 `json:"score_per_million"`
}

type FixtureBrief struct {
	Event    int    `json:"gameweek"`
	Opponent string `json:"opponent"`
	// OpponentID is needed to look the opponent up in the attack/defence bands,
	// which the short name cannot do.
	OpponentID int  `json:"-"`
	Home       bool `json:"home"`
	Difficulty int  `json:"difficulty"`
}

// fixtureLoadFor is how many matches a club plays per gameweek over the next
// `horizon` gameweeks — 1.0 normally, up to 2.0 in a double, and 0 in a blank.
//
// # The assumption this removes
//
// Score is a per-gameweek expectation assembled from per-90 rates, and nothing
// in that arithmetic knows how many times the club actually plays. A player at a
// club with a double gameweek therefore scored exactly the same as one playing
// once, and a player whose club blanks scored as though it had a fixture. Both
// are wrong by a factor of two in the week that matters, where most terms in
// this model argue over a few percent.
//
// Doubles and blanks are not rare: 10 to 42 team-gameweeks a season each, and
// they are precisely when bench boost and triple captain are played.
//
// # Averaged over the horizon, not applied to one week
//
// The count is fixtures divided by *distinct gameweeks* in the window, so a
// double three weeks out is diluted across the horizon rather than treated as
// though it were imminent. That matches how the fixture-difficulty multipliers
// already work and keeps the horizon meaning one thing.
//
// It is deliberately not clamped upward: a genuine double really is worth two
// matches. It is clamped below at zero only to keep a nonsensical fixture list
// from producing a negative score.
//
// # Where it goes matters more than the term itself
//
// Applied to every score it *loses*. Confined to the horizon-1 view — the one
// that picks the eleven actually fielded — it is the largest and most reliable
// gain measured anywhere in this project:
//
//	                             POLICY/gw       t    HOLD/gw       t
//	load on, XI on the horizon      +0.677   +1.05     -1.654   -1.87
//	load everywhere + weekly XI     +1.764   +2.97     -1.654   -1.87
//	load on the XI only             +0.874   +5.74     +0.000   +0.00
//
// **+33 points a season at t = 5.74, with squad selection byte-identical.**
//
// The reason the placement decides it: starting a player who plays twice this
// Saturday is *free*, because you already own him. Buying one for a double three
// weeks out is speculative — the opening fifteen is built before a gameweek is
// played, when every double is distant and several transfer windows will have
// had a chance to move it first. So the same term is a certainty on the eleven
// and a bet on the squad, and applying it to both charges the bet against the
// certainty.
//
// Note the middle row has the largest point estimate and four times the standard
// error, and it damages the held metric. A bigger number arrived at less
// reliably, while degrading the quieter metric, is the signature of a path
// effect rather than a better model — which is why the smaller, cleaner arm is
// the one that ships.
//
// One caveat on the size: the replay's fixture list is final, so it knows about
// doubles from GW1, where FPL announces them only as cup rounds resolve. The
// direction and the placement are safe; treat +33 as an optimistic figure.
//
// # The window is the CALENDAR's next gameweeks, not the club's next fixtures
//
// This shipped for two seasons anchored on `all[0].Event` — the club's next
// fixture — and that is the one anchor that cannot express a blank. If the club
// does not play the imminent gameweek, its first remaining fixture is a week
// later and the window slides forward with it, so the blank simply disappears.
// At horizon 1 the window was `[first, first]`, which contains a fixture by
// construction, so the load was **>= 1 always** and the "playing not at all"
// half of the comment above described a case that had never once executed.
//
// Measured against the archive's true fixture count over every club-gameweek in
// the six-season grid, the old anchor missed **170 blanks** —
// 44/61/22/23/10/10 from 2020-21 to 2025-26 — and **zero doubles**, in 4,540
// comparisons. So the +33 above is a pure doubles result and this fix is not
// covered by it. `TestDiagFixtureLoadMatchesTheArchive` is that comparison.
//
// The anchor is therefore `upcomingGWs` — the rounds the fixture list says are
// still to be played — minus the skip set, so a window of N means N gameweeks
// this engine actually scores. Honouring the skip set is what makes the term
// correct inside `WeekViews`: `engineAt` isolates one gameweek by skipping every
// round before it, and reading `byTeamUpcoming` raw ignored that, so a club with
// an imminent double had every player's score doubled in every projected week.
//
// The denominator is the number of gameweeks the window actually found, which
// matters only at the end of a season: with two rounds left and a horizon of 5,
// a club playing both reads 1.0 rather than 0.4. At horizon 1 — the shipped
// scoring path — it is always 1.
func (e *Engine) fixtureLoadFor(teamID, horizon int) float64 {
	return e.fixtureLoadAfter(teamID, horizon, 0)
}

// fixtureLoadAfter is fixtureLoadFor over a window that starts strictly after
// gameweek `after`, which is what a HOLDING question needs.
//
// `after` of 0 is fixtureLoadFor exactly, and every scoring caller passes it —
// scoring asks what the club plays in the rounds ahead INCLUDING the imminent one,
// because that is the football being scored.
//
// Option value asks the opposite: what a held option insures against is the run it
// might be spent into, which begins next week. `OptionWindow.Remaining` already
// excludes the current gameweek for that reason, and a congestion factor that
// included it disagreed with the decay inside the same product — so for a chip,
// the very double the chip was being played for raised the bar it had to clear.
//
// Parameterised rather than copied. A second density function would be one
// quantity with two implementations at the smallest scale, which is the scale this
// project's copies actually appear at.
func (e *Engine) fixtureLoadAfter(teamID, horizon, after int) float64 {
	all := e.byTeamUpcoming[teamID]
	if len(all) == 0 {
		// No remaining fixtures at all is *unknown*, not a blank, and the two
		// are different facts. Leave the score alone rather than zeroing it.
		return 1
	}
	skip := e.skipSet()
	first, last, weeks := e.loadWindow(horizon, skip, after)
	if weeks == 0 {
		return 1
	}
	n := 0
	for _, f := range all {
		if f.Event > last {
			// buildFixtureIndex appends in gameweek order.
			break
		}
		if f.Event < first || skip[f.Event] {
			continue
		}
		n++
	}
	return float64(n) / float64(weeks)
}

// loadWindow is the span of gameweeks a load is averaged over: the next
// `horizon` rounds still to be played that this engine is not skipping.
//
// It returns the bounds rather than the set, which is what lets the callers scan
// a club's fixtures once. Every gameweek strictly inside `[first, last]` is
// either in the window or in the skip set, so a bounds check plus a skip lookup
// decides membership exactly.
//
// `weeks` is 0 when the season has no rounds left, which callers read as
// "unknown" for the same reason an empty fixture list is.
func (e *Engine) loadWindow(horizon int, skip map[int]bool, after int) (first, last, weeks int) {
	if horizon < 1 {
		horizon = 1
	}
	for _, gw := range e.upcomingGWs {
		// `after` of 0 admits everything, since gameweeks start at 1 — so the
		// scoring callers are unchanged and this cannot alter a scored number.
		if skip[gw] || gw <= after {
			continue
		}
		if weeks == 0 {
			first = gw
		}
		last, weeks = gw, weeks+1
		if weeks == horizon {
			break
		}
	}
	return first, last, weeks
}

// skipSet reads the gameweeks this engine does not score.
//
// One accessor rather than an inline lock at each reader: the map is replaced
// wholesale by SetSkipGameweeks, so a caller that took two separate reads could
// see two different sets inside one calculation.
func (e *Engine) skipSet() map[int]bool {
	e.skipMu.RLock()
	defer e.skipMu.RUnlock()
	return e.skipGameweeks
}

// ElementsWithoutFixtures lists the element ids whose club plays no match in any
// gameweek this engine scores.
//
// It exists for the free hit, and for selection rather than for scoring. Zeroing
// a blanking player's Score keeps him out of the ELEVEN, because `BestXI` ranks
// on the score — but a squad builder still has four bench slots to fill and is
// indifferent between two players worth nothing, so it fills them with whoever
// is cheapest, blank or not. A free hit exists to field fifteen playable
// footballers in a week most clubs do not play; a bench that cannot come on is
// exactly the cover it was spent to buy.
//
// A club with no remaining fixtures at all is *absent* from the index and is not
// listed here, matching FixtureCountsIn: "does not play" and "unknown" are
// different facts and only the first is a blank.
func (e *Engine) ElementsWithoutFixtures() []int {
	skip := e.skipSet()
	first, last, weeks := e.loadWindow(e.Weights.Horizon, skip, 0)
	if weeks == 0 {
		return nil
	}
	blank := map[int]bool{}
	for teamID, fx := range e.byTeamUpcoming {
		plays := false
		for _, f := range fx {
			if f.Event >= first && f.Event <= last && !skip[f.Event] {
				plays = true
				break
			}
		}
		if !plays {
			blank[teamID] = true
		}
	}
	if len(blank) == 0 {
		return nil
	}
	var out []int
	for i := range e.Boot.Elements {
		if blank[e.Boot.Elements[i].Team] {
			out = append(out, e.Boot.Elements[i].ID)
		}
	}
	return out
}

// FixtureLoadIsNotable reports whether a fixture load is far enough from one match
// a gameweek to be worth telling a reader about.
//
// A display concern living here so there is one of it. The CLI and the agent tool
// layer both suppress an ordinary fixture run — the tool because the field is
// replayed on every later API call, the CLI to keep a player line readable — and two
// spellings of one threshold is the shape this project has been bitten by twice
// (DefaultBenchWeight against Weights.BenchWeight, fixtureSensitivePart against
// baseXP90). Rounded to two places, which is the precision either one displays.
//
// ⚠️ **A load of exactly 0 reads as "unset" here and is therefore NOT reported.**
// That was safe while `fixtureLoadFor` could not return 0; it can now, and a
// blanking club at a horizon of 1 is exactly 0. `PlayerMetrics.FixtureLoad`'s own
// zero value is 0 too, so the two cannot be told apart on the number alone, and
// the same conflation sits in `playerRow.Load` (`omitempty`), `noteFixtureLoad`
// (`r.Load != 0`) and `present.corrections` (`!= 0 && != 1`) — four spellings of
// one assumption. Separating them means a pointer or a second field through three
// packages, so it is recorded rather than done.
//
// The exposure today is narrow and is not nothing. The card page and the agent
// both run at the shipped horizon of 5, where a blank reads 0.8 and is reported
// normally. It goes silent only at a configured horizon of 1 — and in `WeekViews`,
// which is horizon 1 by construction but names its blanks through `Blanks` and
// `Opponents` instead, so nothing there is lost.
func FixtureLoadIsNotable(load float64) bool {
	return load > 0 && math.Abs(math.Round(load*100)/100-1) > 1e-9
}

// FixtureLoadInScore reports whether PlayerMetrics.Score already carries the
// fixture-load multiplier on this engine.
//
// It normally does not. The term ships confined to the horizon-1 view — the one
// that picks the eleven actually fielded — plus the transfer objective inside
// XIValue, so a five-gameweek engine reports FixtureLoad and leaves Score alone.
// Configure the horizon to 1 and the same engine *is* the imminent-week view, and
// Score does carry it.
//
// Exported because "is this multiplier already in that number" is precisely what
// the agent has to be told, and because a caller re-deriving the condition would
// be the second copy of it — the bug class behind fixtureSensitivePart drifting
// from baseXP90, where one quantity had two implementations and the one that ran
// was not the one being reasoned about. Metrics calls this too, so there is one.
func (e *Engine) FixtureLoadInScore() bool {
	return fixtureLoadScaling && (!fixtureLoadWeeklyOnly || e.Weights.Horizon == 1)
}

// attackMultiplier maps FPL fixture difficulty (1 easiest .. 5 hardest) onto a
// scaling factor for attacking returns.
func attackMultiplier(d int) float64 {
	switch d {
	case 1:
		return ladder(1.30, atkFixtureScale)
	case 2:
		return ladder(1.15, atkFixtureScale)
	case 3:
		return 1.00
	case 4:
		return ladder(0.85, atkFixtureScale)
	case 5:
		return ladder(0.72, atkFixtureScale)
	}
	return 1.0
}

// defenceMultiplier scales expected goals conceded. Harder fixture => concede more.
func defenceMultiplier(d int) float64 {
	switch d {
	case 1:
		return ladder(0.70, defFixtureScale)
	case 2:
		return ladder(0.85, defFixtureScale)
	case 3:
		return 1.00
	case 4:
		return ladder(1.20, defFixtureScale)
	case 5:
		return ladder(1.40, defFixtureScale)
	}
	return 1.0
}

// Engine computes metrics for every player from a bootstrap + fixture list.
type Engine struct {
	Boot     *fpl.Bootstrap
	Fixtures []fpl.Fixture
	Weights  Weights

	// Cong models European, international and travel load.
	Cong Congestion

	// Role prices uncertainty about a player's role: new signings, new managers.
	Role RoleRisk

	// Chips is the chip schedule — both sets — which shapes the horizon and
	// bench weighting. It is a schedule rather than a single ChipPlan because
	// FPL grants two sets from 2025-26 and a plan that can hold only one of each
	// chip silently drops the second.
	Chips ChipSchedule

	// SellPrices is what each owned player raises, in tenths, from FPL's
	// my-team endpoint. Nil means sell-at-market, which is right pre-season and
	// whenever no session is configured. Anything building a SquadState should
	// pass it through, or the transfer search prices sales at market and spends
	// half of every price rise twice.
	SellPrices map[int]int

	// Entry is the manager id these budget figures describe, or 0 when no squad
	// is being tracked.
	//
	// It is what separates a hypothetical from a failure. With no entry there is
	// no budget to establish and £100m is a perfectly good question to ask. With
	// one, a budget that cannot be established means the squad could not be
	// priced — the API is down, or the id is wrong — and planning against a
	// guessed £100m would be answering a question nobody asked.
	Entry int

	// SquadValue is the selling value of the fifteen, in tenths, as FPL itself
	// reports it. Nil when no squad has been priced. See AssemblyBudget.
	SquadValue *int

	// HypotheticalBudget is the money to plan with when there is no Entry, in
	// tenths. Zero means DefaultBudget. It exists so the squad builder is usable
	// mid-season for projections by someone whose team it is not.
	HypotheticalBudget int

	// Bank is money not in the squad, in tenths, from the same reconstruction
	// that produced SellPrices.
	//
	// It is a pointer because an unknown bank and an empty one are the same
	// number and completely different facts. A caller that reads a missing
	// balance as £0.0m does not fail — it silently searches with nothing to
	// spend, which looks exactly like a squad with no affordable upgrade. That
	// is the quietest way to lose a transfer: every number downstream still
	// renders and the tool reports no move worth making.
	Bank *int

	// priceForecasts is what the review layer was told about tonight's price
	// changes, from a third-party estimator. Per-run and never persisted: a
	// forecast is true for one evening, and a stale one argues for urgency that
	// has already expired. See priceforecast.go.
	priceMu        sync.RWMutex
	priceForecasts map[int]PriceForecast

	// teamStrength is each club's goals for and against per match, blended
	// between FPL's pre-season rating and this season's finished fixtures. Built
	// once under teamOnce because Metrics runs concurrently and an unguarded map
	// build is a fatal "concurrent map writes". See teamstrength.go.
	teamOnce       sync.Once
	teamStrength   map[int]TeamRates
	leagueConceded float64
	leagueScored   float64

	// weekEngine scores on the imminent fixture rather than the horizon, for
	// picking the eleven that would actually be fielded. See WeekEngine.
	weekOnce   sync.Once
	weekEngine *Engine

	// Budget records whether SellPrices are real or assumed. Unverified is not
	// a neutral state: it means every sale is priced at market, the search
	// believes it has half of every price rise that it does not have, and it
	// will recommend upgrades that cannot actually be afforded. That is worth
	// roughly 31 points a season in the replay, so it is reported loudly rather
	// than logged.
	Budget BudgetTrust

	// skipGameweeks are gameweeks the squad is not scored on.
	//
	// A free hit fields an entirely different, temporary fifteen for one week
	// and hands the permanent squad back afterwards, so how the permanent squad
	// would have done that week is irrelevant to how it should be built. Judging
	// it on a blank or a nasty fixture it will never actually play is a real
	// error: it drags the squad toward covering a week that has already been
	// solved by other means.
	//
	// The set is deliberately not derived from ChipPlan alone. The analysis
	// layer knows things the plan does not — that a free hit is about to be
	// brought forward, or that a chip is being held for a double it has spotted
	// — and this is the mechanism for it to say so. Use SetSkipGameweeks.
	skipMu        sync.RWMutex
	skipGameweeks map[int]bool

	byTeamUpcoming map[int][]FixtureBrief

	// upcomingGWs is every gameweek byTeamUpcoming holds a fixture in, ascending
	// and distinct. It is the calendar fixtureLoadFor averages over, and it is
	// read off the fixture list rather than counted forward from the next event
	// so that a cancelled or wholly rearranged round does not dilute every week
	// after it — the same argument upcomingEvents makes for the week views.
	//
	// ⚠️ **Written by buildFixtureIndex, which is NOT called only once, and this
	// slice is NOT lock-guarded.** `ApplyChipPlan` calls it a second time when a
	// planned wildcard shortens the horizon — from a tool handler the runner fans
	// out through an errgroup — so this slice, `byTeamUpcoming` and
	// `Weights.Horizon` are all written unguarded while other tools are scoring
	// players off them. Reproduced under `-race`.
	//
	// **Unfixed, and recorded rather than asserted away.** The race predates this
	// field on the other two, and guarding it properly means taking the whole
	// fixture index and the weights under a lock — a different subsystem, and one
	// that wants its own measurement of the read-path cost, since `fixtureLoadFor`
	// runs once per player per scoring pass. An earlier version of this comment
	// claimed the field was built once and needed no lock; it is wrong, and it is
	// the kind of wrong that makes the next reader skip a lock they need.
	//
	// The skip set has its own lock (`skipMu`) and is applied on top at read time.
	upcomingGWs []int

	// congMu guards Cong and congestion. update_competition_status rewrites them
	// mid-run while other tools are scoring players off them, and the tool runner
	// runs those tools concurrently.
	congMu     sync.RWMutex
	congestion *congestionState

	// The name-to-id lookups are resolved once and then only read. They must be
	// built under a sync.Once rather than a plain nil check: the SDK's tool
	// runner executes tool calls concurrently in an errgroup, so two searches in
	// the same turn both reach Metrics at once, and two goroutines racing to
	// populate the same map is an unrecoverable "concurrent map writes" fatal
	// error — not a panic you can recover from. This crashed a live run.
	restOnce      sync.Once
	restPlayerIDs map[int]bool
	bandOnce      sync.Once
	bandCache     bands
	confirmedOnce sync.Once
	confirmedIDs  map[int]bool
	absenceOnce   sync.Once
	absenceByID   map[int]playerAbsence

	// Priors holds last season's totals, so the model has something to fall
	// back on once FPL overwrites its aggregates at GW1. Optional: nil means no
	// blending, which is correct pre-season and merely thin afterwards. Set once
	// at construction and read-only thereafter.
	Priors PriorSeason

	// Recent supplies recency-weighted minutes for the current season. Optional
	// — nil falls back to flat season-to-date aggregates.
	Recent RecentForm

	// MinutesOverride replaces the derived minutes estimate for specific
	// players, keyed by permanent player code. Set by the analysis layer for
	// the cases the data cannot see; see blendFor.
	//
	// ⚠️ **Read it through minutesOverrideFor and write it through
	// SetMinutesOverride — never touch the map directly from another package.**
	// The doc comment used to say "set by the analysis layer", which was true
	// when it was written and stopped being true when `set_player_status`
	// shipped: that tool mutates this map from a tool handler, and the tool
	// runner fans a turn's calls out through an errgroup. A bare write here
	// alongside any scoring read is `fatal error: concurrent map writes`, which
	// Go does not allow a program to recover from — so it kills a run outright,
	// after the tokens for that turn have already been paid for.
	//
	// This is the same hazard `congMu` and `skipMu` already guard, and it is the
	// one mutable field on Engine that was missed. See overrideMu.
	MinutesOverride map[int]float64

	// TeamXGCFactor multiplies a club's expected goals conceded, keyed by FPL
	// team id. Set by the analysis layer when a defence's record no longer
	// describes the defence — the case a per-player override cannot express,
	// because the quantity that changed belongs to the club. See
	// config.TeamOverride.
	TeamXGCFactor map[int]float64

	// TeamForm supplies each club's recent against season-long expected goals, for
	// the attacking mirror of TeamXGCFactor. Nil by default and inert unless
	// FPL_TEAM_FORM is set; see teamform.go for what it is and why it is off.
	TeamForm TeamFormSource
	teamForm teamFormFactors

	// MinutesOverrideUntil is the last gameweek each override describes, keyed
	// the same way. Absent or zero means indefinite, which applies the override
	// flat. Present, it is prorated across the horizon — see prorateOverride,
	// because "out until GW12" is a claim about particular gameweeks and not
	// about every week the model happens to be averaging over.
	//
	// Guarded by overrideMu, like MinutesOverride. The two are written together
	// and must be read together, or a player can pick up one run's minutes with
	// another run's expiry.
	MinutesOverrideUntil map[int]int

	// overrideMu guards MinutesOverride and MinutesOverrideUntil.
	//
	// Copy-on-write, matching SetCompetitionWindows: a writer builds a new map
	// and swaps it, so a reader holding the previous one is never looking at a
	// map being mutated underneath it. That matters more than lock granularity
	// here — blendFor runs per player, and several searches run at once.
	overrideMu sync.RWMutex

	// rules is FPL's points table as it stood in the season being scored, for
	// the four channels the model computes rather than reads: goals, assists,
	// the clean sheet and the goals-conceded block.
	//
	// # Why per season, and why not read the package tables
	//
	// `goalPoints` and its siblings are **today's** rules, and
	// `TestScoringConstantsMatchFPL` asserts them against FPL's published
	// `game_config` on every run. That is what keeps them honest, and it is the
	// same mechanism that would force the *next* rule change backwards over the
	// whole archive — so a replay of 2019-20 would score a goalkeeper's goal at
	// whatever FPL pays in 2027, silently. `BankLimitFor` and `DefconScoredIn`
	// exist to stop exactly that for the transfer bank and for defensive
	// contribution. This is the same thing for the scoring rules themselves.
	//
	// # Why it is derived and not assigned
	//
	// It comes from `Boot.Season` in `NewEngineFull`, so there is no assignment
	// for a patch to miss. `Simulate` builds three engines — transfers, the
	// eleven, and `Hold` — and a patch that wired two and missed one is this
	// project's most expensive silent bug; `WeekEngine` and `Plan`'s horizon
	// engine re-construct from `e.Boot`, so they inherit the pin for free. A
	// bootstrap fetched from the API carries no season, which means the live
	// game, which means today's rules.
	//
	// Set once at construction and read-only thereafter, so it needs no lock.
	rules ScoringRules

	// xScale converts FPL's expected stats into the events it actually pays
	// for, per position. Built once in the constructor — never lazily, so it
	// needs no lock. See calibrateExpectedStats.
	xScale map[int]ConversionScale
	// leagueRates is the fallback prior for players with none of their own.
	// Built once in the constructor alongside xScale.
	leagueRates map[int]leagueRate
}

// ConversionScale is the per-position conversion from FPL's expected-goal and
// expected-assist figures to the goals and assists FPL awards points for.
//
// Exported because the xPoints instrument prices its own expected side through
// the same quantity — see XPointsResidual. The engine and the instrument share
// this type and CalibrationRatio, so the clamp and the thin-sample floor have
// one implementation. ⚠️ They do NOT share the input POPULATION, and that is a
// real difference rather than an oversight: calibrateExpectedStats sums the
// bootstrap's season-to-date aggregates over players registered by the cutoff,
// while the instrument sums an archived season's gameweek rows whole. See
// Season.calibrateConversion for why the instrument wants the second.
type ConversionScale struct {
	Goals   float64
	Assists float64
}

// PriorSeason is last season's totals, looked up by FPL's permanent player
// code. Narrow on purpose: internal/analysis stays free of the fetching and
// caching in internal/priors, and tests can substitute a fake.
type PriorSeason interface {
	Get(code int) (*PriorPlayer, bool)
}

// PriorPlayer is what the blend needs from a completed season.
//
// Counting stats are here for the same reason the expected ones are: divided by
// a couple of matches they explode. Two bonus points in a 22-minute cameo reads
// as 8.18 bonus points a gameweek, which is more than any player has ever
// averaged, and it went straight into the score because these terms bypassed the
// blend entirely.
type PriorPlayer struct {
	Minutes int
	Starts  int
	XG      float64
	XA      float64
	XGC     float64
	DefCon  int
	Bonus   int
	Saves   int
	Yellow  int
	Red     int
}

// playerAbsence is the resolved mid-season tournament absence for one player.
type playerAbsence struct {
	Name    string
	Matches int
}

func NewEngine(boot *fpl.Bootstrap, fixtures []fpl.Fixture, w Weights) *Engine {
	return NewEngineFull(boot, fixtures, w, DefaultCongestion(), DefaultRoleRisk())
}

// NewEngineWithCongestion builds an engine with an explicit congestion model.
func NewEngineWithCongestion(boot *fpl.Bootstrap, fixtures []fpl.Fixture, w Weights, cg Congestion) *Engine {
	return NewEngineFull(boot, fixtures, w, cg, DefaultRoleRisk())
}

// NewEngineFull builds an engine with every model explicitly supplied.
//
// The scoring rules come from the bootstrap's own season rather than from an
// argument or a setter, so there is nothing for a caller to forget. See
// `Engine.rules` and `fpl.Bootstrap.Season`.
func NewEngineFull(boot *fpl.Bootstrap, fixtures []fpl.Fixture, w Weights, cg Congestion, rr RoleRisk) *Engine {
	e := &Engine{Boot: boot, Fixtures: fixtures, Weights: w, Cong: cg, Role: rr,
		rules: ScoringRulesFor(boot.Season)}
	e.buildFixtureIndex()
	e.buildCongestionState()
	e.calibrateExpectedStats()
	e.calibrateLeagueRates()
	return e
}

// minCalibrationSample is the expected-event total below which a conversion
// ratio is noise rather than signal.
const minCalibrationSample = 20.0

// calibrateExpectedStats derives, per position, how FPL's expected figures
// convert into the events FPL actually awards points for.
//
// xG and an FPL goal are the same event, so that ratio should sit near 1 — and
// for midfielders (0.986) and forwards (0.971) it does. Defenders convert at
// 0.781: their shots are set-piece headers and six-yard scrambles, which xG
// models rate more generously than defenders finish them.
//
// xA and an FPL assist are *not* the same event, and the gap is large. FPL pays
// an assist for winning a penalty that is scored, for a shot parried to a
// team-mate, and for deflected passes — none of which an expected-assists model
// counts. Across the league that is 786 assists against 572 xA, a ratio of
// 1.373, and it is not evenly spread: forwards convert at 2.288 because they
// win most of the penalties.
//
// A single league-wide factor would be wrong in both directions at once —
// taxing forwards 4% on goals to correct a defender problem. So the ratios are
// per position.
//
// These are computed from live data on every run rather than hardcoded, so they
// re-derive themselves each season and cannot go stale. Pre-season they are
// last season's totals, which is the right prior for a model whose inputs are
// also last season's.
func (e *Engine) calibrateExpectedStats() {
	type totals struct{ goals, xG, assists, xA float64 }
	sums := map[int]*totals{}
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		t := sums[el.ElementType]
		if t == nil {
			t = &totals{}
			sums[el.ElementType] = t
		}
		t.goals += float64(el.GoalsScored)
		t.xG += el.ExpectedGoals.Float()
		t.assists += float64(el.Assists)
		t.xA += el.ExpectedAssists.Float()
	}

	e.xScale = make(map[int]ConversionScale, len(sums))
	for pos, t := range sums {
		e.xScale[pos] = ConversionScale{
			Goals:   CalibrationRatio(t.goals, t.xG),
			Assists: CalibrationRatio(t.assists, t.xA),
		}
	}
}

// CalibrationRatio is actual/expected, guarded against thin samples. Keepers
// score 11 goals from 0.2 expected across a season — a ratio of 69 that would
// price every goalkeeper as a striker — so anything under a meaningful
// expected total falls back to neutral, and the result is clamped either way.
func CalibrationRatio(actual, expected float64) float64 {
	if expected < minCalibrationSample {
		return 1
	}
	return clamp(actual/expected, 0.5, 3.0)
}

// scaleFor returns the conversion factors for a position, defaulting to neutral
// for any position the bootstrap did not cover.
func (e *Engine) scaleFor(pos int) ConversionScale {
	if s, ok := e.xScale[pos]; ok {
		return s
	}
	return ConversionScale{Goals: 1, Assists: 1}
}

// buildFixtureIndex collects every unfinished fixture from the next event
// onward, per club and in gameweek order, plus the distinct gameweeks they fall
// in. It is not bounded by the horizon, whatever this comment said before:
// `TeamFixtures` and `fixtureLoadFor` both window it themselves, and
// `FixtureCountsIn` and the agent's ten-fixture view need it to reach further
// than either.
//
// The gameweek order is load-bearing rather than incidental — `fixtureLoadFor`
// stops scanning a club's list once it passes the end of its window.
func (e *Engine) buildFixtureIndex() {
	e.byTeamUpcoming = map[int][]FixtureBrief{}
	// Rebuilt, not appended to: ApplyChipPlan calls this a second time when a
	// planned wildcard shortens the horizon.
	e.upcomingGWs = nil

	next := e.Boot.NextEvent()
	fromEvent := 1
	if next != nil {
		fromEvent = next.ID
	}

	sorted := make([]fpl.Fixture, len(e.Fixtures))
	copy(sorted, e.Fixtures)
	sort.Slice(sorted, func(i, j int) bool {
		ei, ej := 999, 999
		if sorted[i].Event != nil {
			ei = *sorted[i].Event
		}
		if sorted[j].Event != nil {
			ej = *sorted[j].Event
		}
		return ei < ej
	})

	for _, f := range sorted {
		if f.Finished || f.Event == nil || *f.Event < fromEvent {
			continue
		}
		home := e.Boot.TeamByID(f.TeamH)
		away := e.Boot.TeamByID(f.TeamA)
		if home == nil || away == nil {
			continue
		}
		e.byTeamUpcoming[f.TeamH] = append(e.byTeamUpcoming[f.TeamH], FixtureBrief{
			Event: *f.Event, Opponent: away.ShortName, OpponentID: f.TeamA,
			Home: true, Difficulty: f.TeamHDifficulty,
		})
		e.byTeamUpcoming[f.TeamA] = append(e.byTeamUpcoming[f.TeamA], FixtureBrief{
			Event: *f.Event, Opponent: home.ShortName, OpponentID: f.TeamH,
			Home: false, Difficulty: f.TeamADifficulty,
		})
		if n := len(e.upcomingGWs); n == 0 || e.upcomingGWs[n-1] != *f.Event {
			e.upcomingGWs = append(e.upcomingGWs, *f.Event)
		}
	}
}

// FixtureCountsIn returns how many fixtures each club plays in one gameweek,
// keyed by the short name PlayerMetrics.Team carries. A doubling club is 2 and a
// blanking one 0.
//
// It exists because `FixtureLoad` is an average over the horizon and cannot
// answer a question about a *particular* week. A bench boost pays in one
// gameweek, so valuing a squad for it needs that week's own fixture count:
// averaged over five weeks a doubling club reads 1.2, which prices the double at
// a fifth of its strength. See analysis.ChipCredit.WeekLoad.
//
// A club with no fixture at all in that week is present with a count of zero,
// which is what a blank is. A club absent from the index entirely — one with no
// remaining fixtures — is absent here too, and callers read that as 1 rather
// than as a blank, because "unknown" and "does not play" are different.
func (e *Engine) FixtureCountsIn(gw int) map[string]float64 {
	out := make(map[string]float64, len(e.byTeamUpcoming))
	for teamID, fx := range e.byTeamUpcoming {
		t := e.Boot.TeamByID(teamID)
		if t == nil {
			continue
		}
		var n float64
		for _, f := range fx {
			if f.Event == gw {
				n++
			}
		}
		out[t.ShortName] = n
	}
	return out
}

// TeamFixtures returns the next n fixtures a team will be scored on.
//
// Skipped gameweeks are dropped and the window extends past them, so n means n
// gameweeks that count rather than n gameweeks on the calendar. Shortening the
// horizon instead would quietly make every player look worse in the weeks
// around a free hit.
func (e *Engine) TeamFixtures(teamID, n int) []FixtureBrief {
	all := e.byTeamUpcoming[teamID]
	skip := e.skipSet()

	if len(skip) == 0 {
		if n > len(all) {
			n = len(all)
		}
		return all[:n]
	}

	out := make([]FixtureBrief, 0, n)
	for _, f := range all {
		if len(out) == n {
			break
		}
		if skip[f.Event] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// SetSkipGameweeks replaces the set of gameweeks the squad is not scored on.
//
// Copy-on-write under a lock, for the same reason SetCompetitionWindows is: the
// tool runner fans calls out through an errgroup, so a reader may be holding the
// previous map while this runs.
//
// # This is engine-level configuration, not a per-call argument
//
// It is deliberately not exposed as a tool parameter. Setting it for one call
// and restoring it afterwards looks harmless and is not: tool calls run
// concurrently, so a second search in the same turn would be scored against the
// first one's skip set, and whichever finished last would leave its value
// behind. That is the same shape as the config write race that lost three of
// five overrides on the first live run.
//
// Set it once, from the chip plan, before the engine is used. A scenario that
// needs a different set needs its own engine.
func (e *Engine) SetSkipGameweeks(gws []int) {
	m := make(map[int]bool, len(gws))
	for _, gw := range gws {
		if gw > 0 {
			m[gw] = true
		}
	}
	e.skipMu.Lock()
	e.skipGameweeks = m
	e.skipMu.Unlock()
}

// SkipGameweeks reports which gameweeks are excluded from scoring.
func (e *Engine) SkipGameweeks() []int {
	e.skipMu.RLock()
	defer e.skipMu.RUnlock()
	out := make([]int, 0, len(e.skipGameweeks))
	for gw := range e.skipGameweeks {
		out = append(out, gw)
	}
	sort.Ints(out)
	return out
}

// Metrics computes the derived view for a single player.
func (e *Engine) Metrics(el *fpl.Element) PlayerMetrics {
	team := e.Boot.TeamByID(el.Team)
	teamName := "?"
	if team != nil {
		teamName = team.ShortName
	}

	m := PlayerMetrics{
		ID:       el.ID,
		Name:     el.WebName,
		Team:     teamName,
		Position: e.Boot.PositionShort(el.ElementType),
		Price:    el.PriceM(),

		Status:     el.StatusLabel(),
		News:       el.News,
		ChancePlay: el.ChanceOfPlayingNextRound,

		Minutes:          el.Minutes,
		Starts:           el.Starts,
		ExpectedMinutes:  e.expectedMinutes(el),
		StartShare:       float64(el.Starts) / float64(e.matchesAvailable(el)),
		MatchesAvailable: e.matchesAvailable(el),

		XG90:  el.ExpectedGoalsPer90.Float(),
		XA90:  el.ExpectedAssistsPer90.Float(),
		XGI90: el.ExpectedGIPer90.Float(),
		XGC90: el.ExpectedGCPer90.Float(),

		Goals:       el.GoalsScored,
		Assists:     el.Assists,
		TotalPoints: el.TotalPoints,
		PPG:         el.PointsPerGame.Float(),
		Form:        el.Form.Float(),
		Bonus:       el.Bonus,
		Ownership:   el.SelectedByPercent.Float(),

		Finishing: float64(el.GoalsScored) - el.ExpectedGoals.Float(),

		PenaltyOrder:  el.PenaltiesOrder,
		CornerOrder:   el.CornersAndIndirectFKOrder,
		DirectFKOrder: el.DirectFreekicksOrder,
	}

	// Per-90 rates are only meaningful with a real sample. Fall back to
	// deriving them from totals when FPL leaves the per-90 fields empty.
	if el.Minutes > 0 {
		per90 := 90.0 / float64(el.Minutes)
		if m.XG90 == 0 {
			m.XG90 = el.ExpectedGoals.Float() * per90
		}
		if m.XA90 == 0 {
			m.XA90 = el.ExpectedAssists.Float() * per90
		}
		if m.XGI90 == 0 {
			m.XGI90 = m.XG90 + m.XA90
		}
		if m.XGC90 == 0 {
			m.XGC90 = el.ExpectedGoalsConceded.Float() * per90
		}
	}

	// Mix last season in, in proportion to how little of this one there is.
	// Pre-season this is a no-op — FPL's totals already are last season.
	b := e.blendFor(el, m)
	m.XG90, m.XA90, m.XGC90 = b.XG90, b.XA90, b.XGC90
	// A club-level correction from the analysis layer, for a defence whose
	// record was earned by a back line that no longer exists. Applied to the
	// input rather than to the answer, so the clean sheet, the goals-conceded
	// block and the keeper's saves all recompute from one corrected number.
	if f := e.TeamXGCFactor[el.Team]; f > 0 && f != 1 {
		m.XGC90 *= f
		m.TeamXGCFactor = f
	}
	// The attacking mirror, and applied to the same channel for the same reason:
	// correct the input and let the goal term, the assist term and everything
	// reading XGI90 recompute from one number. Off unless FPL_TEAM_FORM is set.
	if f := e.teamFormFactor(el.Team); f != 1 {
		m.XG90 *= f
		m.XA90 *= f
		m.TeamFormFactor = f
	}
	m.XGI90 = m.XG90 + m.XA90
	m.DefCon90 = b.DefCon90
	m.Bonus90, m.Saves90 = b.Bonus90, b.Saves90
	m.Yellow90, m.Red90 = b.Yellow90, b.Red90
	// ExpectedMinutes carries the rest factor, so the reported figure and the
	// rotation_risk band agree with the score. SettledMinutes takes it back out,
	// for the pool filters — see the field comment.
	m.ExpectedMinutes = b.MinutesPerMatch
	m.SettledMinutes = b.MinutesPerMatch
	if _, f := e.restFactor(el); f > 0 && f < 1 {
		m.SettledMinutes = b.MinutesPerMatch / f
	}
	m.StartShare = b.StartShare
	m.PriorWeight = b.Weight

	m.SetPieceNote = setPieceNote(el)
	sc := e.scaleFor(el.ElementType)
	m.XGScale, m.XAScale = sc.Goals, sc.Assists
	m.TournamentAbsence = e.tournamentAbsence(el).Name
	m.MinutesRating = reliabilityFrom(b, e.minutesExponent(el.ElementType))
	m.RotationRisk = rotationLabel(m.ExpectedMinutes)
	m.NewSigning, m.JoinedDate = e.newSigning(el)

	// The door. Everything below this line PRICES a player, and a position the
	// season's rules have no entry for cannot be priced. Everything above it is
	// descriptive — who he is, what he costs, what he has recorded, how much he
	// plays — and none of it needs a points table, so it is filled either way.
	//
	// ⚠️ **Key presence, not value.** `Prices` asks whether the table HAS the
	// position, because a stored zero is a real FPL rule in this table's siblings
	// — a forward's clean sheet pays exactly 0 — so a value test cannot tell "FPL
	// pays nothing here" from "this position is not in the table". Go's bare map
	// index returns the same thing for both, which is the whole defect.
	//
	// # The population, which is real
	//
	// FPL ran assistant managers as `element_type` 5 for 2024-25. They reach both
	// paths: the captured live GW38 bootstrap publishes 20 of them as `MNG`, and
	// the replay's own bootstrap carries the same 20 by `through` **26** — an
	// entry point the shipped sweep grid uses, so this is a deadline the replay
	// really picks a squad at rather than the last week of a season nobody enters
	// at.
	//
	// ⚠️ **26 is the earliest entry point that carries them, not the onset**, and
	// two versions of this comment have now got that wrong in different ways. The
	// onset is `through` **23**, read off the archive rather than off a probe grid:
	// their first `merged_gw.csv` rows are GW23 and `registeredBy` admits a player
	// from his first row. No sweep entry point samples 23, which is why a probe
	// kept reporting whichever of {26, 37} it happened to check.
	//
	// Read through a bare `goalPoints[pos]` their goals channel prices at the
	// map's zero while appearance, bonus and cards stay realised, so `BaseXP90`
	// came back at exactly 2.0 — a plausible number with a whole channel deleted,
	// which is the silent-null shape this record calls its signature failure.
	//
	// # Why a return and not a panic
	//
	// A panic here would have killed `armband squad` for a live user through
	// 2024-25, and kills the replay at every 2024-25 entry point from GW26 on.
	// ⚠️ **This said "GW37" alone**, from the same sampling artefact corrected
	// above. That is not the loud
	// failure it looks like; it is a correct program refusing to run on a payload
	// FPL really served. `ScoringRules.GoalPoints` is the panic, one layer in,
	// and it is what fails if this door is removed or if a sixth pricing site is
	// added behind it.
	//
	// ⚠️ **This changes no ranking today and that is not the same as being
	// inert.** `Score` is already zero for this population by another route —
	// they record no minutes — and `PointInTimeWith` publishes element_types 1-4,
	// so `squadQuota` cannot hold one. What moves is `BaseXP90`, 2.0 to 0, on 20
	// rows of one season, 40 player-cutoffs across the six-season grid. What it
	// buys is the day a position arrives with minutes on it.
	//
	// ⚠️ **A refused row must not assert something FALSE about the player, and a
	// first version of this door did.** Three of the fields below this line are
	// *multipliers whose neutral value is 1*, and leaving them at Go's zero does
	// not read as "not computed" — it reads as "ruled out". `present/card.go`
	// renders `availability x0.00` through a branch whose own comment says a zero
	// there "is the whole explanation of the score above it, so it is the one
	// value that must never be filtered out as empty", and `tools.go` passes
	// `avail_factor` through a POINTER precisely so a ruled-out player's 0.0
	// survives to the agent. So an assistant manager would have been reported to a
	// human and to the model as unavailable, which is a different and wronger
	// claim than "unpriced".
	//
	// They need no points table, so they are computed. What stays zero is the
	// priced half and only that.
	if unpricedPositionGuard && !e.rules.Prices(el.ElementType) {
		m.Congestion, m.CongestionNotes = e.CongestionFactor(el)
		m.RoleFactor, m.RoleNotes = e.roleFactor(el, m.NewSigning)
		m.AvailabilityFactor = availabilityFactor(el)
		return m
	}

	m.BaseXP90 = e.baseXP90(el, m)
	m.SetPieceXP90 = e.setPieceXP90(el, m)

	// ⚠️ At a horizon of 1 in a DOUBLE gameweek this is one leg of the two.
	// `TeamFixtures` counts fixtures, `fixtureLoadFor` counts gameweeks, so the
	// second leg arrives as a multiplier: the week is priced `2·f(d1)` rather
	// than `f(d1)+f(d2)`, charging both matches at the first one's difficulty.
	// The gap is bounded by the spread of the fixture ladders — `attackMultiplier`
	// runs 1.30 to 0.72 and `defenceMultiplier` 0.70 to 1.40 — so at most about a
	// quarter of the fixture-sensitive part of one match, and exactly zero when
	// the two legs share a difficulty, which is the common case.
	//
	// `m.Fixtures` reports the same single leg to the agent and the CLI, so a
	// double gameweek shows one opponent where a manager would expect two.
	//
	// Known and unfixed, recorded rather than silently carried. It is a
	// *magnitude* error inside a week the model already knows is a double, not
	// the blank-shaped one `fixtureLoadFor` documents.
	fx := e.TeamFixtures(el.Team, e.Weights.Horizon)
	m.Fixtures = fx
	m.AvgDifficulty = averageDifficulty(fx)
	m.FixtureAdjXP90 = e.fixtureAdjustedXP90(el, m, fx)

	// Fixture congestion: European midweeks, international duty and the travel
	// cost of getting back. This scales expected minutes, not per-90 quality.
	m.Congestion, m.CongestionNotes = e.CongestionFactor(el)

	// Role uncertainty: a new signing's numbers were earned elsewhere, and a
	// new manager makes every selection assumption provisional.
	m.RoleFactor, m.RoleNotes = e.roleFactor(el, m.NewSigning)

	// Rate terms scale with time on the pitch; threshold terms do not. Splitting
	// them is the whole of thresholdXP90's purpose — see there for why the two
	// were being scaled identically and what that cost.
	minutes := m.MinutesRating
	rate, thresholdPart, perGW := m.FixtureAdjXP90, 0.0, 0.0
	var appearPart, cleanPart float64
	if thresholdSplit {
		appearPart, cleanPart = e.thresholdParts(el, m, fx)
		thresholdPart = appearPart + cleanPart
		// Defensive contribution is a threshold on actions rather than on
		// minutes, so it is not scaled at all — it is recomputed at the
		// exposure the player gets. Subtract what the per-90 estimate already
		// counted and add the corrected figure, which is per gameweek.
		if el.Minutes > 0 {
			rate -= defconPer90(el.ElementType, m.DefCon90)
			perGW = e.defconPerGameweek(el.ElementType, m)
		}
		if thresholdPart > rate {
			// A negative rate part is not a thing. Scale BOTH halves by the same
			// factor rather than clamping the total: they are now multiplied by
			// different probabilities below, so trimming one of them would
			// silently change the appearance-to-clean-sheet ratio whenever this
			// bites — a different bug wearing the old one's clothes.
			if thresholdPart > 0 {
				k := rate / thresholdPart
				appearPart *= k
				cleanPart *= k
			}
			thresholdPart = rate
		}
		rate -= thresholdPart
		if rate < 0 {
			rate = 0
		}
	}
	// The two threshold halves take different probabilities, because FPL pays them
	// on different events: appearance once for turning up and again at the hour,
	// the clean sheet only at the hour. See appearanceFactor.
	m.AvailabilityFactor = availabilityFactor(el)
	m.Score = (rate*minutes +
		appearPart*appearanceFactor(m.ExpectedMinutes) +
		cleanPart*playsSixty(m.ExpectedMinutes) +
		perGW) *
		m.Congestion * m.RoleFactor * m.AvailabilityFactor
	// A gameweek is not a match. Everything above is a per-gameweek expectation
	// built from per-90 rates, and it silently assumes one fixture a week — so a
	// club playing twice scored identically to one playing once, and a club
	// playing not at all scored as though it had. See fixtureLoadFor.
	m.FixtureLoad = e.fixtureLoadFor(el.Team, e.Weights.Horizon)
	m.loadSet = true
	if e.FixtureLoadInScore() {
		m.Score *= m.FixtureLoad
		m.loadInScore = true
	}

	// Post-tournament rest: players returning late from a summer international
	// are routinely eased back in over the opening weeks.
	// Reporting only. The factor was already applied to minutes inside blendFor,
	// so multiplying Score here as well would charge for it twice.
	if reason, factor := e.restFactor(el); factor < 1 {
		m.RestRisk = reason
		m.RestMinutesFactor = factor
	}

	if m.Price > 0 {
		m.ValueScore = m.Score / m.Price
	}
	return m
}

// newSigning reports whether the player joined his current club after the
// previous season ended — meaning his stats were earned at a different club and
// his role in the new side is unproven.
func (e *Engine) newSigning(el *fpl.Element) (bool, string) {
	if el.TeamJoinDate == "" {
		return false, ""
	}
	joined, err := time.Parse("2006-01-02", el.TeamJoinDate)
	if err != nil {
		return false, ""
	}
	// Anything from May onwards of the current calendar year is this summer's
	// window, relative to the first gameweek's deadline.
	ref := e.seasonStart()
	if joined.After(ref.AddDate(0, -4, 0)) {
		return true, el.TeamJoinDate
	}
	return false, ""
}

func (e *Engine) seasonStart() time.Time {
	for i := range e.Boot.Events {
		if e.Boot.Events[i].ID == 1 {
			return e.Boot.Events[i].DeadlineTime
		}
	}
	return time.Now()
}

// restFactor is the configured post-tournament minutes factor, prorated across
// the fixture horizon. It multiplies expected minutes, not Score — see blendFor.
//
// Proration matters, and getting it wrong is the same mistake the European
// penalty made before it was date-gated: what this feeds is averaged over the
// next Horizon gameweeks, so applying the raw factor to it asserts that the
// player is eased in during *every* one of them. He is not — he is eased back
// in over the opening weeks and normal thereafter.
//
// With a 5-gameweek horizon, a 2-gameweek rest window and a 0.83 factor, the
// honest figure at GW1 is (2×0.83 + 3×1.00) / 5 = 0.93, and it decays to 0.97 at
// GW2 and 1.00 from GW3. The unprorated version cost a flagged player minutes
// across two months of fixtures he would be entirely fresh for.
func (e *Engine) restFactor(el *fpl.Element) (string, float64) {
	w := e.Weights
	if w.RestMinutesFactor <= 0 || w.RestMinutesFactor >= 1 || w.RestGameweeks <= 0 {
		return "", 1
	}
	next := e.Boot.NextEvent()
	if next == nil || next.ID > w.RestGameweeks {
		return "", 1
	}

	// Share of the horizon that actually falls inside the rest window.
	horizon := w.Horizon
	if horizon < 1 {
		horizon = 1
	}
	affected := w.RestGameweeks - next.ID + 1
	if affected > horizon {
		affected = horizon
	}
	factor := (float64(affected)*w.RestMinutesFactor + float64(horizon-affected)) / float64(horizon)

	e.restOnce.Do(func() {
		ids := map[int]bool{}
		for _, name := range w.RestPlayers {
			var id int
			if _, err := fmt.Sscanf(strings.TrimSpace(name), "%d", &id); err == nil && id > 0 {
				ids[id] = true
				continue
			}
			for _, match := range e.Boot.FindPlayers(name) {
				ids[match.ID] = true
				break
			}
		}
		e.restPlayerIDs = ids
	})
	note := fmt.Sprintf("post-tournament rest: minutes x%.2f over GW%d-%d, %.2f across the %d-gameweek horizon",
		w.RestMinutesFactor, next.ID, next.ID+affected-1, factor, horizon)

	if e.restPlayerIDs[el.ID] {
		return note, factor
	}
	if el.Region != nil {
		for _, r := range w.RestRegions {
			if r == *el.Region {
				return fmt.Sprintf("nationality group %d — %s", r, note), factor
			}
		}
	}
	return "", 1
}

// GameweeksPerSeason is the number of Premier League matches per club.
const GameweeksPerSeason = 38

// minutesReliability estimates what share of a full match a player is expected
// to be on the pitch for in a given gameweek.
//
// Do NOT use FPL's starts_per_90 field here. It measures "when this player
// appears, does he start", which sits at roughly 1.0 for almost every player in
// the game — a 25-minute-per-week squad rotation option scores the same as an
// ever-present. The only figures that carry rotation risk are minutes and
// starts measured against the full season.
func (e *Engine) minutesReliability(el *fpl.Element) float64 {
	if el.Minutes == 0 {
		return 0
	}

	return reliabilityFrom(e.blendFor(el, PlayerMetrics{}), e.minutesExponent(el.ElementType))
}

// reliabilityFrom turns blended minutes into a 0-1 rating.
//
// # It used to mix in start share, and dropping that is worth 180 points
//
// The rating was 0.6 x minutes-per-match + 0.4 x start-share, on the reasoning
// that the two fail differently: a player averaging 60 minutes by starting
// every week and being substituted is safer than one averaging 60 by starting
// two thirds of the time, because the second carries blank weeks you cannot
// plan around. The reasoning is sound and the conclusion was wrong. Swept over
// four seasons the mix is monotone in favour of minutes — 8137 / 8219 / 8124 /
// 8163 / 8304 at shares of 0.3 / 0.5 / 0.6 / 0.8 / 1.0 — and minutes alone wins
// three of the four, including the held-out season.
//
// The reason is that this is an *expectation*. Score is expected points, and
// what a player is expected to return is governed by how long he is on the
// pitch; whether he starts is a statement about the variance around that. The
// blank-week risk the start-share term was reaching for is real, but it is a
// different quantity — P(he records no minutes at all) — and it has its own home
// in blankRate, where it prices the bench slots that exist to cover it.
// Mixing a variance concern into a mean double-purposed one number.
//
// Note that blankRate no longer reads StartShare either: it is one minus the
// appearance estimator, which is also in mean minutes. That is not this finding
// being reversed by the back door. This function scales the *rate* terms — goals,
// assists, defensive contributions, bonus — where the 180 points were measured;
// the appearance and clean-sheet *thresholds* are a different quantity, and
// whether start share belongs in them was measured separately and separately
// rejected. See appearance.go.
//
// The exponent was re-checked at the new mix rather than assumed: 1.25 still
// wins (8304 against 8281 at 1.0 and 8238 at 1.5), so the two knobs are not
// trading off against each other.
func reliabilityFrom(b blend, exponent float64) float64 {
	minutesShare := clamp(b.MinutesPerMatch/90.0, 0, 1)
	startShare := clamp(b.StartShare, 0, 1)
	w := reliabilityMinutesShare
	return clamp(math.Pow(w*minutesShare+(1-w)*startShare, exponent), 0, 1)
}

// minutesExponent returns the convexity exponent for a position. Above 1 the
// rotation penalty is disproportionate; the per-position scale attenuates how
// much of that severity applies.
func (e *Engine) minutesExponent(elementType int) float64 {
	w := e.Weights.MinutesWeight
	if w <= 0 {
		w = 1
	}
	if minutesWeightSet {
		w = minutesWeightOverride
	}
	pos := e.Boot.PositionShort(elementType)
	scale, ok := e.Weights.MinutesWeightByPosition[pos]
	if v, over := posMinutesScaleOverride[pos]; over {
		scale, ok = v, true
	}
	if !ok {
		return w
	}
	if scale < 0 {
		scale = 0
	}
	// Scale the severity — the excess over neutral — not the exponent itself,
	// so 0.75 means "three quarters as harsh", not "an exponent of 0.75".
	return 1 + (w-1)*scale
}

// expectedMinutes is the average minutes per gameweek across the matches the
// player was available for. Dividing by a flat 38 instead would report a player
// who spent four weeks at a mid-season tournament as a rotation risk — see
// TournamentAbsence.
func (e *Engine) expectedMinutes(el *fpl.Element) float64 {
	return float64(el.Minutes) / float64(e.matchesAvailable(el))
}

// tournamentAbsence returns the mid-season tournament this player left for and
// the number of league matches it cost him, or the zero value if none applies.
//
// The figure is capped by his own record: he cannot have missed more matches
// than he failed to start. That cap is what makes the list safe to hand-
// maintain — over-stating Matches, or wrongly including someone who turned out
// to play through the window, cannot invent minutes he never played.
func (e *Engine) tournamentAbsence(el *fpl.Element) playerAbsence {
	e.absenceOnce.Do(func() {
		// Built under a sync.Once for the same reason as restPlayerIDs: the
		// tool runner reaches Metrics from several goroutines at once.
		byID := map[int]playerAbsence{}
		for _, t := range e.Weights.TournamentAbsences {
			if t.Matches <= 0 {
				continue
			}
			for _, name := range t.Players {
				a := playerAbsence{Name: t.Name, Matches: t.Matches}
				var id int
				if _, err := fmt.Sscanf(strings.TrimSpace(name), "%d", &id); err == nil && id > 0 {
					byID[id] = a
					continue
				}
				for _, match := range e.Boot.FindPlayers(name) {
					byID[match.ID] = a
					break
				}
			}
		}
		e.absenceByID = byID
	})

	// The list describes the season the aggregates came from. Once gameweeks are
	// played, FPL has overwritten those aggregates with this season's, and last
	// summer's list describes data that is no longer in hand. A tournament
	// inside the *current* season needs its own entries and this guard revisited.
	if e.GameweeksPlayed() > 0 {
		return playerAbsence{}
	}

	a, ok := e.absenceByID[el.ID]
	if !ok {
		return playerAbsence{}
	}
	if unstarted := e.DataWindow() - el.Starts; a.Matches > unstarted {
		a.Matches = unstarted
	}
	if a.Matches <= 0 {
		return playerAbsence{}
	}
	return a
}

// GameweeksPlayed is how many gameweeks have finished. Zero before the season
// starts.
func (e *Engine) GameweeksPlayed() int {
	n := 0
	for _, ev := range e.Boot.Events {
		if ev.Finished {
			n++
		}
	}
	return n
}

// DataWindow is the number of league matches FPL's aggregate stats cover, and
// therefore the correct denominator for turning them into per-gameweek rates.
//
// Before the season starts this is a full 38: FPL carries last season's totals
// until GW1 completes, so `minutes` of 3065 means 3065 across 38 matches. Once
// gameweeks are played the aggregates reset and accumulate, so the window is
// however many have finished — after GW3, `minutes` of 270 means 270 across 3.
//
// Getting this wrong is catastrophic rather than merely inaccurate. Dividing one
// gameweek's 90 minutes by 38 reports an ever-present as 2.4 minutes per
// gameweek, which puts every player in the game into the "fringe" band and
// scores them at roughly 1% of their true value. Nothing recovers a truthful
// band until about GW29, so the optimiser would spend most of the season
// choosing between numbers that are all near zero.
func (e *Engine) DataWindow() int {
	if played := e.GameweeksPlayed(); played > 0 {
		return played
	}
	return GameweeksPerSeason
}

// ScaledMinMinutes converts a minutes floor written as a SEASON TOTAL into the
// figure to compare against the aggregates available right now.
//
// # Why this is a function and not three tokens at each call site
//
// Getting it wrong is on this project's recorded list of things that have already
// bitten: FPL's aggregates reset at GW1, so `minutes` covers only the matches played
// so far. Compared against an unscaled season floor after one gameweek, every player
// in the game fails it, the pool is empty and the optimiser errors outright.
//
// The expression was written twice — here and in the agent's player filter — which
// is one quantity with two implementations of exactly the shape this record
// catalogues. `TestDataWindowTracksTheSeason` pins `DataWindow`; it does not pin the
// scaling, so a caller could have kept the window and dropped the divisor.
func (e *Engine) ScaledMinMinutes(seasonTotal int) int {
	return seasonTotal * e.DataWindow() / GameweeksPerSeason
}

// matchesAvailable is the denominator for minutes and starts: the league
// matches the player could actually have been picked for, out of those the
// aggregate stats cover.
func (e *Engine) matchesAvailable(el *fpl.Element) int {
	n := e.DataWindow() - e.tournamentAbsence(el).Matches
	if n < 1 {
		n = 1
	}
	return n
}

// rotationLabel turns expected minutes into a plain-language risk band.
func rotationLabel(expMins float64) string {
	switch {
	case expMins >= 75:
		return "nailed"
	case expMins >= 60:
		return "likely starter"
	case expMins >= 40:
		return "rotation risk"
	case expMins >= 20:
		return "squad player"
	default:
		return "fringe"
	}
}

// availabilityFactor discounts injured, suspended or doubtful players.
func availabilityFactor(el *fpl.Element) float64 {
	if el.ChanceOfPlayingNextRound != nil {
		return clamp(float64(*el.ChanceOfPlayingNextRound)/100.0, 0, 1)
	}
	switch el.Status {
	case "a":
		return 1.0
	case "d":
		return 0.5
	case "i", "s", "u", "n":
		return 0.0
	}
	return 1.0
}

// baseXP90 estimates FPL points per 90 minutes from underlying numbers,
// using the actual scoring rules rather than a black-box rating.
func (e *Engine) baseXP90(el *fpl.Element, m PlayerMetrics) float64 {
	pos := el.ElementType

	xp := appearancePoints
	// Expected stats are converted to awarded events first — see
	// calibrateExpectedStats. FPL's assist is a broader event than xA measures.
	sc := e.scaleFor(pos)
	xp += m.XG90 * sc.Goals * e.rules.GoalPoints(pos)
	xp += m.XA90 * sc.Assists * e.rules.Assist

	// Clean sheet probability from expected goals conceded, Poisson P(0 goals).
	if csPts := e.rules.CleanSheetPoints(pos); csPts > 0 && m.XGC90 > 0 {
		xp += cleanSheetProb(m.XGC90, 1, e.defconCleanFactor(pos, m.DefCon90)) * csPts
	}

	// Goals conceded: −1 per 2 in a match, keepers and defenders only. Counted
	// in whole blocks that reset each match, so a side conceding one goal a game
	// is never deducted at all — E[floor(X/2)], not xGC/2.
	if blk := e.rules.ConcedeBlock[pos]; blk > 0 && m.XGC90 > 0 {
		xp -= poissonFloorDiv(blk, m.XGC90)
	}

	if el.Minutes > 0 {
		// Goalkeeper saves: 1 point per 3 saves, awarded per match and rounded
		// down, so the remainder does not carry into the next game. Dividing a
		// season total by three credits every one of those discarded remainders.
		if pos == 1 {
			xp += poissonFloorDiv(savesBlock, m.Saves90)
		}

		// Defensive contribution: 2 points for clearing 10 CBIT in a match as a
		// defender, or 12 CBIRT as anyone else. The award is per match and
		// all-or-nothing — nine actions score nothing — so what is wanted is the
		// probability of clearing the bar, P(X >= threshold), exactly as the
		// clean-sheet term above takes P(0 goals) from expected goals conceded.
		//
		// This used to be a linear ramp, clamp(dc/threshold, 0, 1), which reads
		// "averaging 70% of the bar earns 70% of the bonus". It does not: it
		// earns however often you actually clear the bar, which at 70% of the
		// bar is about 17%. Because the line rises at a fixed rate while the
		// true probability is still near zero, the error is a hump peaking at
		// roughly 0.7x the bar:
		//
		//	dc/90     2     5     7    10    12    16
		//	ramp   0.40  1.00  1.40  2.00  2.00  2.00
		//	true   0.00  0.06  0.34  1.08  1.52  1.91
		//
		// That is not a rounding error, and it was not evenly spread. FPL set
		// the thresholds near what a busy outfielder actually achieves, so 86%
		// of defenders and 82% of midfielders sat in the 0.5-1.1x band where the
		// approximation is worst — a mean overstatement of +0.95 and +1.00
		// points per 90 respectively, against +0.73 for forwards and zero for
		// keepers, who record no CBIT and are scored on saves instead.
		//
		// The ramp also compressed the spread within a position, because it
		// capped at the bar while the real probability keeps climbing past it.
		// Senesi averages 11.5 and clears the bar 71% of matches; Truffert
		// averages 8.0 and clears it 28%. True gap 0.85 points; the ramp showed
		// 0.40, under half of it, so elite defensive defenders were rated too
		// close to mediocre ones.
		//
		// Poisson is the one-parameter choice available: a season average per 90
		// is the only input the API gives. Real CBIT is overdispersed relative to
		// Poisson — a defender against a possession side racks up 20, against a
		// weak one 4 — and fatter tails pull P(X >= k) toward 0.5, so this
		// slightly understates players below the bar. Fixing that needs
		// per-match counts to fit a dispersion parameter on, and the API does
		// not retain them across a season boundary. Do not guess one.
		xp += defconPer90(pos, m.DefCon90)

		// Bonus points, from historical rate, scheduled by how much of that rate
		// is current-season evidence rather than last season's. See bonusWeightFor.
		xp += m.Bonus90 * e.bonusWeightFor(el)

		// Card deductions.
		xp -= m.Yellow90 * yellowCardPoints
		xp -= m.Red90 * redCardPoints
	}

	return math.Max(xp, 0)
}

// setPieceXP90 credits penalty and set-piece duty, which is the most reliable
// source of repeatable points in FPL.
func (e *Engine) setPieceXP90(el *fpl.Element, m PlayerMetrics) float64 {
	var bonus float64

	// Penalties: roughly 0.11 pens per 90 for a first-choice taker on a
	// mid-table side, ~76% conversion.
	if o := el.PenaltiesOrder; o != nil {
		switch *o {
		case 1:
			bonus += 0.11 * 0.76 * e.rules.GoalPoints(el.ElementType)
		case 2:
			bonus += 0.03 * 0.76 * e.rules.GoalPoints(el.ElementType)
		}
	}
	// Corners and indirect free kicks: assist source.
	if o := el.CornersAndIndirectFKOrder; o != nil && *o == 1 {
		bonus += 0.09 * e.rules.Assist
	}
	// Direct free kicks: small goal and assist source.
	if o := el.DirectFreekicksOrder; o != nil && *o == 1 {
		bonus += 0.025*e.rules.GoalPoints(el.ElementType) + 0.03*e.rules.Assist
	}

	return bonus * e.Weights.SetPieceWeight
}

// fixtureAdjustedXP90 re-runs the attacking and clean-sheet components against
// the difficulty of each of the upcoming fixtures.
//
// # Why each fixture, and not the average of them
//
// This used to average the two difficulty multipliers over the horizon into a
// single attacking and a single defensive number, and evaluate the estimate once
// at those averages. That is exact for the *linear* terms — goals and assists are
// a rate multiplied by the attacking multiplier, so scoring five fixtures at
// their mean difficulty and averaging five separately-scored fixtures give the
// same answer — and wrong for the clean sheet, which is exp(-lambda x def) and
// therefore **convex** in the multiplier.
//
// For a convex function the average of the values is at least the value at the
// average (Jensen's inequality), with equality only when every fixture in the run
// has identical difficulty. So the old form understated the mean clean-sheet
// probability, and — the part that actually matters for a ranking — it compressed
// the gap between an easy run and a hard one, for defenders and keepers only.
//
// Size, at a defender's 1.3 expected goals conceded per 90 and the shipped
// 0.70-to-1.40 range: a five-fixture run split between the two extremes has a
// true mean clean-sheet probability near 0.283 against 0.256 evaluated at the
// averaged multiplier — about 0.11 points per 90 before fixture_weight damps it,
// so 0.02 to 0.07 for a typical run. Exactly zero for attackers.
//
// The refactor also removes a bug class. The fixture-insensitive remainder is
// computed as base minus the fixture-sensitive part, so the two expressions have
// to agree term for term, and they had silently drifted apart twice. They are now
// one function evaluated at different arguments — fixtureSensitiveAt(m, pos, 1, 1)
// is what "the fixture-sensitive part at neutral difficulty" means — so a term
// added to one cannot go missing from the other.
func (e *Engine) fixtureAdjustedXP90(el *fpl.Element, m PlayerMetrics, fx []FixtureBrief) float64 {
	base := m.BaseXP90 + m.SetPieceXP90
	if len(fx) == 0 {
		return base
	}
	pos := el.ElementType

	// Score each fixture at its own difficulty and average the results.
	var adjusted float64
	for _, f := range fx {
		atk, def := e.fixtureMultipliersFor(f)
		adjusted += e.fixtureSensitiveAt(m, pos, atk, def)
	}
	adjusted /= float64(len(fx))
	// Fixture-insensitive remainder (defcon, bonus, cards, set pieces).
	adjusted += base - e.fixtureSensitiveAt(m, pos, 1, 1)

	// Blend: FixtureWeight=1 trusts the fixture adjustment fully, 0 ignores it.
	w := clamp(e.Weights.FixtureWeight, 0, 1)
	return math.Max(base*(1-w)+adjusted*w, 0)
}

// fixtureMultipliersFor is one fixture's attacking and defensive difficulty
// multipliers, including the band adjustment.
//
// Attack and defence bands, if enabled, adjust each fixture individually:
// attacking returns by the opponent's defensive band, goals conceded by the
// opponent's attacking band. See bands.go.
// FixtureMultipliersFor exposes one fixture's difficulty multipliers, for the
// same reason DefconCleanFactorFor and CleanSheetTermFor are exposed: a
// calibration that cannot see a factor the engine applies is fitting a different
// quantity from the one the engine scores.
//
// ⚠️ It is exposed for CALIBRATION and not as a scoring entry point. `def` here
// is a MODELLED quantity — it comes from FPL's difficulty rank and this project's
// own band adjustment, or from the magnitude path — so a fit against
// `XGC90 x def` calibrates one part of the model against another. That bounds the
// scored path rather than calibrating it, and any figure derived from it has to
// say so.
func (e *Engine) FixtureMultipliersFor(f FixtureBrief) (atk, def float64) {
	return e.fixtureMultipliersFor(f)
}

func (e *Engine) fixtureMultipliersFor(f FixtureBrief) (atk, def float64) {
	if magnitudeDifficulty {
		// Difficulty from how good the opponent actually is, rather than from
		// FPL's integer rank. See teamstrength.go.
		return e.magnitudeAttack(f.OpponentID), e.magnitudeDefence(f.OpponentID)
	}
	bs := e.Weights.BandStrength
	return attackMultiplier(f.Difficulty) * e.attackBandAdj(f.OpponentID, bs),
		defenceMultiplier(f.Difficulty) * e.defenceBandAdj(f.OpponentID, bs)
}

// thresholdXP90 is the part of the per-90 estimate FPL pays as a step at sixty
// minutes rather than in proportion to time on the pitch.
//
// Two terms qualify and they are large. Appearance points are 1 below sixty
// minutes and 2 at or above, and the clean sheet pays 4 to a defender or keeper
// and 1 to a midfielder at sixty, nothing below. A starter taken off at seventy
// banks all of both — the model credited him roughly 0.73 of each, because
// Score multiplied the whole per-90 figure by minutes reliability.
//
// Together they are 61% of a defender's per-90 score, 34% of a midfielder's and
// 29% of a forward's, so mis-scaling them is not a rounding error and it is not
// even across positions.
//
// It returns the same blend of base and fixture-adjusted values that
// fixtureAdjustedXP90 produces, so subtracting it from that total leaves
// exactly the rate part.
//
// # The two halves no longer share a probability
//
// thresholdParts returns them separately, because FPL pays them on different
// events. Appearance is 1 for turning up and 2 at the hour, so its expectation is
// P(appears) + P(60+); the clean sheet pays only at the hour, so it stays on
// P(60+) alone. Scaling both by P(60+) — which is what this did — credits a
// fifty-minute appearance nothing where FPL pays one.
//
// The split is exactly additive: appearance + cleanSheet reproduces this function
// to the last bit. That matters beyond tidiness, because fixtureAdjustedXP90
// subtracts the threshold total from baseXP90 to leave the rate remainder, and
// TestThresholdAndAdjustedUseTheSameFixtureRule pins that the two use the same
// fixture rule so the subtraction cancels. A split that were merely close would
// break the remainder silently.
func (e *Engine) thresholdXP90(el *fpl.Element, m PlayerMetrics, fx []FixtureBrief) float64 {
	appearance, cleanSheet := e.thresholdParts(el, m, fx)
	return appearance + cleanSheet
}

// thresholdParts is thresholdXP90's two halves, which Score scales by different
// probabilities. See there for why they are separate.
//
// Appearance points do not depend on the opponent, so they need no fixture
// averaging at all — only the clean sheet is convex in the defensive multiplier.
func (e *Engine) thresholdParts(el *fpl.Element, m PlayerMetrics, fx []FixtureBrief) (appearance, cleanSheet float64) {
	pos := el.ElementType
	neutral := e.cleanSheetSensitiveAt(m, pos, 1)
	if len(fx) == 0 {
		return appearancePoints, neutral
	}
	// Averaged per fixture rather than at the averaged multiplier, for the same
	// reason fixtureAdjustedXP90 is: the clean sheet is convex in the defensive
	// multiplier, so the two are not the same number. Both must use the same rule
	// or the subtraction that separates the rate part from the threshold part
	// would not cancel.
	var adjusted float64
	for _, f := range fx {
		_, def := e.fixtureMultipliersFor(f)
		adjusted += e.cleanSheetSensitiveAt(m, pos, def)
	}
	adjusted /= float64(len(fx))
	w := clamp(e.Weights.FixtureWeight, 0, 1)
	return appearancePoints, neutral*(1-w) + adjusted*w
}

// thresholdSensitiveAt is the threshold pair — appearance points and the clean
// sheet — at one fixture's defensive multiplier. Appearance points do not depend
// on the opponent; the clean sheet does.
func (e *Engine) thresholdSensitiveAt(m PlayerMetrics, pos int, def float64) float64 {
	return appearancePoints + e.cleanSheetSensitiveAt(m, pos, def)
}

// cleanSheetSensitiveAt is the clean-sheet half on its own, at one fixture's
// defensive multiplier.
//
// It is separate because it is the only half that depends on the opponent, and
// because Score scales the two halves by different probabilities: the clean sheet
// pays only at sixty minutes, while appearance pays once for turning up and again
// at the hour. thresholdSensitiveAt remains the sum, so callers that want the pair
// — and the tests that pin the remainder subtraction — are unaffected.
func (e *Engine) cleanSheetSensitiveAt(m PlayerMetrics, pos int, def float64) float64 {
	csPts := e.rules.CleanSheetPoints(pos)
	if csPts <= 0 || m.XGC90 <= 0 {
		return 0
	}
	cf := e.defconCleanFactor(pos, m.DefCon90)
	return cleanSheetProb(m.XGC90, def, cf) * csPts
}

// bonusWeightFor scales the bonus term by how much of the player's bonus rate
// is evidence from this season rather than a summary of last one.
//
// The term is circular by construction — BPS is driven by goals, assists, clean
// sheets, saves and defensive actions, all of which the model already prices —
// and it survives that because it also captures plenty the model never sees:
// passes completed, tackles won, key passes, big chances created, recoveries.
// What it is worth therefore depends entirely on whether the rate describes the
// player now or the player a year ago at possibly another club.
//
// Measured on the held opening fifteen over four seasons, split by the gameweek
// the entry began at, the two regimes disagree in opposite directions:
//
//	weight   from GW1   from GW11   from GW21
//	0            6626        5473        3271
//	0.5          6659        5496        3473
//	1.0          6341        5617        3530
//	1.5          6306        5761        3619
//
// Monotone harmful before a ball is kicked, monotone helpful once ten gameweeks
// have been played. A single constant cannot be right in both places, and the
// aggregate across start points is non-monotone precisely because it averages
// the two.
//
// The schedule interpolates between BonusPriorWeight, applied when the rate is
// entirely last season's, and BonusWeight, applied when it is entirely this
// one. Evidence is the same n90/(n90+k) share blendFor uses to mix the rate in
// the first place — blend.Weight cannot be reused because it reads 1.0
// pre-season, when FPL's totals *are* last season and there is nothing to blend.
func (e *Engine) bonusWeightFor(el *fpl.Element) float64 {
	hi := e.Weights.BonusWeight
	lo := e.Weights.BonusPriorWeight
	if lo < 0 {
		return hi
	}
	return lo + (hi-lo)*e.bonusEvidence(el)
}

// bonusEvidence is the share of a player's bonus rate that comes from the
// current season: zero before a ball is kicked, rising as he plays.
func (e *Engine) bonusEvidence(el *fpl.Element) float64 {
	if e.GameweeksPlayed() == 0 {
		return 0
	}
	n90 := float64(el.Minutes) / 90
	k := e.Weights.BlendRateK
	if k <= 0 {
		return 1
	}
	return clamp(n90/(n90+k), 0, 1)
}

// defconThreshold is the defensive-contribution bar: ten actions for a
// defender, twelve for everyone else. FPL's rule, not a model choice.
func defconThreshold(pos int) int {
	if pos == 2 {
		return 10
	}
	return 12
}

// defconPer90 is the defensive-contribution term as the model has always
// computed it: the chance of clearing the bar over a full ninety minutes.
func defconPer90(pos int, dc90 float64) float64 {
	if dc90 <= 0 {
		return 0
	}
	return poissonAtLeast(defconThreshold(pos), dc90) * defConPoints
}

// defconPerGameweek is the same term computed at the exposure the player
// actually gets, which is not the same thing and not a scaled version of it.
//
// The bar is a count of actions *in a match*. A player on sixty minutes has two
// thirds of the chances to reach ten, so his probability of clearing it falls
// faster than his minutes do — P(Poisson(dc90 x m/90) >= k), which is convex in
// m near the bar. The model instead took the full-ninety probability and scaled
// the resulting points by minutes reliability, which falls *slower*. So where
// appearance points and the clean sheet were under-credited for part-timers,
// this was over-credited, and by more.
//
// A defender on 10 actions per 90 clearing a bar of 10 is the worst case: over
// ninety minutes he clears it 54% of the time, over sixty 8%. Scaled the old
// way he was credited 33%.
//
// Exposure is the mean minutes he plays *when he appears* rather than his mean
// over all gameweeks, because a blank contributes no actions and no points
// rather than a fraction of both.
//
// P(appears) comes from appearsInGameweek, which is the model's one estimator of
// it. This used to read 1 - blankRate instead, and that was a second, independent
// estimator whose constant had been fitted only over start share 0.70 and up: a
// fringe defender at start share zero was credited a 62.4% blank rate whatever his
// minutes said, which both scaled this term down and inflated the exposure it is
// computed at. See the note at the top of appearance.go.
func (e *Engine) defconPerGameweek(pos int, m PlayerMetrics) float64 {
	if m.DefCon90 <= 0 || m.ExpectedMinutes <= 0 {
		return 0
	}
	appears := clamp(appearsInGameweek(m), 0, 1)
	if appears <= 0 {
		return 0
	}
	mins := clamp(m.ExpectedMinutes/appears, 0, 90)
	return appears * poissonAtLeast(defconThreshold(pos), m.DefCon90*mins/90) * defConPoints
}

// fixtureSensitiveAt is every component of the per-90 estimate that depends on
// the opponent, evaluated against one fixture's attacking and defensive
// multipliers. At (1, 1) it is the same quantity at neutral difficulty, which is
// what fixtureAdjustedXP90 subtracts from baseXP90 to leave the remainder.
//
// It must mirror baseXP90 term for term, and this is now the only way to say it.
// It did not always: the clean sheet was written here as exp(-XGC90) while
// baseXP90 computes exp(-cleanSheetXGCFactor x XGC90 x defconCleanFactor), so
// every change to either factor silently desynchronised the remainder from the
// base it is subtracted from. Harmless while both extra factors were 1; not
// harmless once DefConCleanCoupling shipped at 0.3. Having one function serve
// both the neutral and the adjusted case is what makes that drift structurally
// impossible rather than merely tested for — the same one-implementation-per-
// quantity rule the bench weight and the inference layer are held to.
func (e *Engine) fixtureSensitiveAt(m PlayerMetrics, pos int, atk, def float64) float64 {
	sc := e.scaleFor(pos)
	p := appearancePoints
	p += m.XG90 * sc.Goals * atk * e.rules.GoalPoints(pos)
	p += m.XA90 * sc.Assists * atk * e.rules.Assist
	if csPts := e.rules.CleanSheetPoints(pos); csPts > 0 && m.XGC90 > 0 {
		p += cleanSheetProb(m.XGC90, def, e.defconCleanFactor(pos, m.DefCon90)) * csPts
	}
	// Goals conceded scales with the opponent too, so it belongs here rather
	// than in the carried-across remainder.
	if blk := e.rules.ConcedeBlock[pos]; blk > 0 && m.XGC90 > 0 {
		p -= poissonFloorDiv(blk, m.XGC90*def)
	}
	// Saves scale with the opponent as hard as anything else does. Measured
	// within-keeper, saves against a given opponent run 1.46 to 0.75 — a factor
	// of 1.96 against the defensive ladder's own 0.70-to-1.40, which is 2.0.
	// They were carried across unchanged, so a keeper facing a strong attack
	// lost clean-sheet value and gained nothing for the shots that attack forces
	// him to face. Half a trade-off.
	if pos == 1 && savesFixtureAdjust {
		p += poissonFloorDiv(savesBlock, m.Saves90*def)
	}
	return p
}

func setPieceNote(el *fpl.Element) string {
	var parts []string
	if o := el.PenaltiesOrder; o != nil {
		parts = append(parts, "penalties #"+itoa(*o))
	}
	if o := el.CornersAndIndirectFKOrder; o != nil {
		parts = append(parts, "corners/indirect FK #"+itoa(*o))
	}
	if o := el.DirectFreekicksOrder; o != nil {
		parts = append(parts, "direct FK #"+itoa(*o))
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += ", " + p
	}
	return out
}

func averageDifficulty(fx []FixtureBrief) float64 {
	if len(fx) == 0 {
		return 0
	}
	var sum int
	for _, f := range fx {
		sum += f.Difficulty
	}
	return float64(sum) / float64(len(fx))
}

// AllMetrics computes metrics for every player in the game.
func (e *Engine) AllMetrics() []PlayerMetrics {
	out := make([]PlayerMetrics, 0, len(e.Boot.Elements))
	for i := range e.Boot.Elements {
		out = append(out, e.Metrics(&e.Boot.Elements[i]))
	}
	return out
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// poissonAtLeast returns P(X >= k) for X ~ Poisson(lambda) — the chance of
// clearing a per-match threshold given only an average rate.
//
// Summed from the bottom because k is small (10 or 12) and the terms below it
// are the large ones; accumulating the tail directly would add many negligible
// terms to a small total and lose precision.
func poissonAtLeast(k int, lambda float64) float64 {
	if lambda <= 0 {
		return 0
	}
	if k <= 0 {
		return 1
	}
	below, term := 0.0, math.Exp(-lambda)
	for i := 0; i < k; i++ {
		below += term
		term *= lambda / float64(i+1)
	}
	return clamp(1-below, 0, 1)
}

// poissonFloorDiv returns E[floor(X/d)] for X ~ Poisson(lambda) — the expected
// number of whole d-sized blocks in a single match.
//
// FPL awards saves and deducts for goals conceded in whole blocks that reset
// every match: three saves is a point, two is nothing, and the remainder does
// not carry into the next game. Dividing a season total by d instead credits
// every one of those discarded remainders — the same threshold-versus-
// accumulation error the defensive-contribution term used to make.
//
// The difference is not small. Two goals conceded per game is a full point a
// match; two goals spread over two games is nothing at all, and E[floor(X/2)]
// is what tells them apart. Fitting seventeen keepers' seasons, per-match
// blocks for both saves and goals conceded reproduced actual points to within
// 1.9 of ~120, against 3.8 to 11.3 for every other combination.
func poissonFloorDiv(d int, lambda float64) float64 {
	if lambda <= 0 || d <= 0 {
		return 0
	}
	// Run out until the Poisson tail is negligible. Twelve standard deviations
	// past the mean is far beyond any football scoreline.
	limit := int(lambda+12*math.Sqrt(lambda)) + 40
	total, term := 0.0, math.Exp(-lambda)
	for k := 0; k < limit; k++ {
		total += float64(k/d) * term
		term *= lambda / float64(k+1)
	}
	return total
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

// SetMinutesOverride installs a minutes correction for one player, keyed by
// permanent player code, expiring after gameweek `until` (0 means indefinite).
//
// # Why this exists rather than a bare map write
//
// `set_player_status` mutates these maps from a tool handler, and the tool
// runner executes a turn's tool calls in PARALLEL through an errgroup. The
// original code did `e.MinutesOverride[code] = v` directly, with the scoring
// path reading the same map unguarded from blendFor and squad.go. Two overrides
// in one turn — which the prompt actively asks for, since every override marked
// CHECK is part of the week's work — or one override alongside any scoring tool
// is `fatal error: concurrent map writes`. Go does not permit recovery from
// that, so the run dies after its tokens are spent.
//
// Copy-on-write rather than a mutex around each access: a writer clones, edits
// and swaps, so a reader that already holds a map is safe without holding a
// lock for the whole of a scoring pass.
func (e *Engine) SetMinutesOverride(code int, minutes float64, until int) {
	e.overrideMu.Lock()
	defer e.overrideMu.Unlock()

	mins := make(map[int]float64, len(e.MinutesOverride)+1)
	for k, v := range e.MinutesOverride {
		mins[k] = v
	}
	mins[code] = minutes
	e.MinutesOverride = mins

	ends := make(map[int]int, len(e.MinutesOverrideUntil)+1)
	for k, v := range e.MinutesOverrideUntil {
		ends[k] = v
	}
	if until > 0 {
		ends[code] = until
	} else {
		delete(ends, code)
	}
	e.MinutesOverrideUntil = ends
}

// ClearMinutesOverride removes a player's correction, if any.
func (e *Engine) ClearMinutesOverride(code int) {
	e.overrideMu.Lock()
	defer e.overrideMu.Unlock()

	if _, ok := e.MinutesOverride[code]; ok {
		mins := make(map[int]float64, len(e.MinutesOverride))
		for k, v := range e.MinutesOverride {
			if k != code {
				mins[k] = v
			}
		}
		e.MinutesOverride = mins
	}
	if _, ok := e.MinutesOverrideUntil[code]; ok {
		ends := make(map[int]int, len(e.MinutesOverrideUntil))
		for k, v := range e.MinutesOverrideUntil {
			if k != code {
				ends[k] = v
			}
		}
		e.MinutesOverrideUntil = ends
	}
}

// minutesOverrideFor reads a player's correction and its expiry together.
//
// Together on purpose: read separately under separate locks, a player can pick
// up one write's minutes with another write's expiry, which is a silently wrong
// prorating rather than a crash.
func (e *Engine) minutesOverrideFor(code int) (mins float64, until int, ok bool) {
	e.overrideMu.RLock()
	defer e.overrideMu.RUnlock()
	mins, ok = e.MinutesOverride[code]
	until = e.MinutesOverrideUntil[code]
	return mins, until, ok
}

// hasMinutesOverrides reports whether any correction is installed.
func (e *Engine) hasMinutesOverrides() bool {
	e.overrideMu.RLock()
	defer e.overrideMu.RUnlock()
	return len(e.MinutesOverride) > 0
}
