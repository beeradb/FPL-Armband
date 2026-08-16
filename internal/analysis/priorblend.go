package analysis

import "math"

// Blending several past seasons into one prior.
//
// A single prior season is one sample, and sometimes it is the wrong one. Isak
// played 694 minutes in 2025-26 with a broken leg and produced 0.346 xG+xA per
// 90; the three seasons before that read 2758, 2253 and 1520 minutes at 0.781,
// 0.915 and 0.561. Judging him on the most recent season alone is judging him on
// his injury.
//
// # Minutes and rates need different weights
//
// Rates are weighted by minutes as well as recency, because minutes are the
// evidence behind a rate: 2,758 minutes says far more about a player's scoring
// than 694 does. That falls out of estimating the rate as a ratio of weighted
// sums rather than an average of per-season rates.
//
// Minutes are weighted by recency *only*. Weighting minutes by minutes is
// circular and would erase exactly the information wanted — a player with three
// full seasons and one injured one should read as somewhat injury-prone, not as
// an ever-present.
//
// This is the same split found within a season: minutes are a statement about
// the present and rates about quality. See Weights.MinutesHalfLife.
type PriorSeasonStats struct {
	PriorPlayer
	// SeasonsAgo is 0 for the most recently completed season.
	SeasonsAgo int

	// NoXG, NoXGC and NoDefCon mark a season whose source carries no expected
	// goals and assists, no expected goals conceded, and no defensive
	// contribution respectively.
	//
	// They exist because the absence arrives as an explicit **zero** rather than
	// as a missing field. FPL's `history_past` returns `"0.00"` for all three
	// expected statistics in every season before 2022/23 while reporting three
	// thousand real minutes, so a centre-half with five such seasons blends five
	// genuine-looking zeroes into his expected goals conceded and reads as having
	// conceded nothing.
	//
	// # Why three flags and not one
	//
	// This was a single `NoExpected` covering xG, xA and xGC together, and that
	// cannot express a state the archive actually produces. `rebuildXGAggregates`
	// runs inside `applyXGRepair`; `rebuildXGCAggregates` runs only inside
	// `applyXGCRepair`, which is skipped under `FPL_NO_XGC_REPAIR=1` — a switch
	// AGENTS.md *instructs* people to set to reproduce recorded figures. In that
	// state a season comes back with **real xG and xA aggregates beside an xGC
	// aggregate of exactly zero**, and one flag must either discard two genuine
	// statistics or blend a false one. xG and xA stay together because nothing
	// separates them: they are repaired by the same pass from the same harvest.
	//
	// # The zero value, and the claim that used to justify it
	//
	// The zero value is "this season has everything", so a caller that has not
	// been taught the boundaries gets the previous behaviour.
	//
	// ⚠️ That default used to be justified here by the claim that "the archive
	// paths reach these seasons through the expected-goals repair and genuinely do
	// have the figures, so flagging them would be wrong". **That is false**, under
	// either `FPL_NO_XGC_REPAIR` or `FPL_NO_XG_AGGREGATE`, and it is why three of
	// the four archive-side projections were written without these flags and read
	// as deliberate. The default is a compatibility choice, not a statement about
	// the archive — so a caller must set the flags from what its source *actually
	// loaded*, at season level, post-repair. Never from a season name: the repair
	// moves the boundary, and a name predates the repair.
	//
	// ⚠️ And there is no shared `hasDefCon(season)` to reach for, deliberately.
	// Three sources have three different boundaries — FPL `history_past` from
	// 2024/25, the vaastav archive from 2025-26, olbauday's `playerstats.csv` from
	// 2025-26 — so one implementation for a quantity with three values would be
	// this record's signature failure inverted. The flag belongs to the source.
	//
	// ⚠️ This is the same defect the replay path had and fixed at `7cb769e`, where
	// a zero in the pre-2022-23 seasons diluted the *blended* defender's xGC while
	// the shipped single-season read was untouched — asymmetric by construction.
	// The repair fixed the archive; nothing reaches FPL's API, so the live path
	// kept the bug. One quantity, two implementations, and only one was repaired.
	NoXG     bool
	NoXGC    bool
	NoDefCon bool
}

// ThinSeason is the minutes below which the most recent completed season stops
// being trusted on its own and older ones are blended into it. Half a
// thirty-eight match season at ninety minutes.
//
// This is the ONE declaration. priors.ThinSeason, recent.ThinSeason and
// backtest's thinPrior are aliases of it rather than repetitions — three
// packages that cannot import one another all import this one, so the bar has a
// natural home here even though nothing in this package gates on it.
//
// The value is asserted, not measured: it is "half a season", chosen because a
// player with less than that has not shown enough for the season to stand alone.
// Every recorded figure about prior_half_life was measured at it.
const ThinSeason = 1710

// ShouldBlendPrior decides whether older seasons are folded into the most recent
// one, given the minutes that player recorded in the most recent season on
// record. It is the whole of the prior-blend gate.
//
// Two exclusions, and they are excluded for opposite reasons:
//
//   - A FULL season stands alone. It is the best evidence there is about a
//     player, and smoothing an older season into it dilutes genuine improvement —
//     which is most players, most of the time.
//
//   - NO minutes at all is not a thin sample, it is a different fact. The model
//     already has an answer for a player with no usable history: blendRates sends
//     him to shrinkToLeague, which pulls his rates to his position's league-wide
//     figures. Handing him a season at least two years old replaces a defensible
//     estimate with a stale one, and measurement says it pushes him past the
//     truth into OVER-rating where the thin-but-played case is under-rated.
//
// So the feature is for one population — a player whose last season is an injury
// artefact — and this is the line that confines it to them. The zero-minute
// player falls through to whatever the caller does when blending is off, which on
// every path is the shipped answer.
func ShouldBlendPrior(lastSeasonMinutes int) bool {
	return lastSeasonMinutes > 0 && lastSeasonMinutes < ThinSeason
}

// BlendPriors collapses several seasons into the single prior the model reasons
// from. halfLife is in seasons; 1.0 halves a season's weight for each year back.
func BlendPriors(seasons []PriorSeasonStats, halfLife float64) PriorPlayer {
	if len(seasons) == 0 {
		return PriorPlayer{}
	}
	if halfLife <= 0 {
		halfLife = 1
	}

	// Four denominators, not one. A season the feed did not measure must leave
	// the numerator AND the denominator of that statistic alone: counting its
	// minutes while contributing no expected goals is precisely how an absence
	// becomes a measured zero, which is the failure the three flags document.
	var minW, rateW, xgW, xgcW, dcW float64             // weight denominators
	var mins, starts float64                            // recency-weighted
	var xg, xa, xgc, dc, bonus, saves, yel, red float64 // recency-and-minutes-weighted
	for _, s := range seasons {
		if s.Minutes <= 0 {
			continue
		}
		w := math.Pow(0.5, float64(s.SeasonsAgo)/halfLife)
		minW += w
		mins += float64(s.Minutes) * w
		starts += float64(s.Starts) * w

		// Weighting the numerator by w and the denominator by w*minutes makes
		// the result a minutes-weighted rate without a second pass.
		rateW += float64(s.Minutes) * w
		if !s.NoXG {
			xgW += float64(s.Minutes) * w
			xg += s.XG * w
			xa += s.XA * w
		}
		if !s.NoXGC {
			xgcW += float64(s.Minutes) * w
			xgc += s.XGC * w
		}
		if !s.NoDefCon {
			dcW += float64(s.Minutes) * w
			dc += float64(s.DefCon) * w
		}
		bonus += float64(s.Bonus) * w
		saves += float64(s.Saves) * w
		yel += float64(s.Yellow) * w
		red += float64(s.Red) * w
	}
	if minW == 0 || rateW == 0 {
		return PriorPlayer{}
	}

	// Report the blend as a season's worth of totals, because that is what the
	// consumer divides by. Rates are converted back through the blended minutes
	// so per90(XG, Minutes) returns the rate that was actually estimated.
	blendedMins := mins / minW
	scale := blendedMins / rateW
	// Each statistic converts back through the minutes that actually measured
	// it. Where nothing did, the scale is zero and the statistic comes back zero
	// — which is not a regression: every season available for this player also
	// reports it as zero, so it is exactly what the shipped single-season read
	// gives him, and "what the shipped model does with him is the bar".
	//
	// Three scales rather than two, because expected goals conceded can be absent
	// while expected goals are present — see the note on NoXGC.
	xgScale, xgcScale, dcScale := 0.0, 0.0, 0.0
	if xgW > 0 {
		xgScale = blendedMins / xgW
	}
	if xgcW > 0 {
		xgcScale = blendedMins / xgcW
	}
	if dcW > 0 {
		dcScale = blendedMins / dcW
	}
	return PriorPlayer{
		Minutes: int(blendedMins + 0.5),
		Starts:  int(starts/minW + 0.5),
		XG:      xg * xgScale,
		XA:      xa * xgScale,
		XGC:     xgc * xgcScale,
		DefCon:  int(dc*dcScale + 0.5),
		Bonus:   int(bonus*scale + 0.5),
		Saves:   int(saves*scale + 0.5),
		Yellow:  int(yel*scale + 0.5),
		Red:     int(red*scale + 0.5),
	}
}
