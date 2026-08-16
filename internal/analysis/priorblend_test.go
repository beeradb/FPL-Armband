package analysis

import (
	"math"
	"testing"
)

// TestBlendPriorsDiscountsAnInjuredSeason is the case the whole thing exists for.
//
// Isak's real record: 2025-26 was 694 minutes at 0.346 xG+xA per 90 with a
// broken leg; the three before were 2758, 2253 and 1520 minutes at 0.781, 0.915
// and 0.561. Judging him on the most recent season alone judges him on the
// injury.
func TestBlendPriorsDiscountsAnInjuredSeason(t *testing.T) {
	isak := []PriorSeasonStats{
		{PriorPlayer: PriorPlayer{Minutes: 694, Starts: 8, XG: 2.0, XA: 0.7}, SeasonsAgo: 0},
		{PriorPlayer: PriorPlayer{Minutes: 2758, Starts: 34, XG: 18.0, XA: 5.9}, SeasonsAgo: 1},
		{PriorPlayer: PriorPlayer{Minutes: 2253, Starts: 27, XG: 17.0, XA: 5.9}, SeasonsAgo: 2},
	}
	got := BlendPriors(isak, 1.5)

	rate := (got.XG + got.XA) * 90 / float64(got.Minutes)
	recentOnly := (2.0 + 0.7) * 90 / 694
	t.Logf("blended: %d mins, %.3f xGI/90 (most recent season alone: %.3f)",
		got.Minutes, rate, recentOnly)

	if rate <= recentOnly*1.5 {
		t.Errorf("blended rate %.3f is barely above the injured season's %.3f; the "+
			"minutes weighting is not doing its job", rate, recentOnly)
	}
	// Minutes must NOT be minutes-weighted, or an injured season is erased and
	// he reads as an ever-present.
	if got.Minutes >= 2253 {
		t.Errorf("blended minutes %d — the injured season has been weighted away",
			got.Minutes)
	}
	if got.Minutes <= 694 {
		t.Errorf("blended minutes %d — the healthy seasons are not counted", got.Minutes)
	}
}

// TestBlendPriorsDoesNotRescueAConsistentFringePlayer — the counterpart. Diop
// has four seasons at roughly 0.04 xGI/90 and declining minutes; looking further
// back must not inflate him.
func TestBlendPriorsDoesNotRescueAConsistentFringePlayer(t *testing.T) {
	diop := []PriorSeasonStats{
		{PriorPlayer: PriorPlayer{Minutes: 812, Starts: 8, XG: 0.6, XA: 0.3}, SeasonsAgo: 0},
		{PriorPlayer: PriorPlayer{Minutes: 1334, Starts: 15, XG: 0.4, XA: 0.2}, SeasonsAgo: 1},
		{PriorPlayer: PriorPlayer{Minutes: 1423, Starts: 16, XG: 0.5, XA: 0.2}, SeasonsAgo: 2},
	}
	got := BlendPriors(diop, 1.5)
	rate := (got.XG + got.XA) * 90 / float64(got.Minutes)
	t.Logf("blended: %d mins, %.3f xGI/90", got.Minutes, rate)
	if rate > 0.08 {
		t.Errorf("a consistent fringe player blended up to %.3f xGI/90", rate)
	}
}

// TestBlendPriorsPreservesRatesForASteadyPlayer — three identical seasons must
// blend to that same season, or the estimator is distorting rather than
// combining.
func TestBlendPriorsPreservesRatesForASteadyPlayer(t *testing.T) {
	one := PriorPlayer{Minutes: 3000, Starts: 34, XG: 15, XA: 6, XGC: 40,
		DefCon: 200, Bonus: 12, Saves: 0, Yellow: 5, Red: 0}
	got := BlendPriors([]PriorSeasonStats{
		{PriorPlayer: one, SeasonsAgo: 0},
		{PriorPlayer: one, SeasonsAgo: 1},
		{PriorPlayer: one, SeasonsAgo: 2},
	}, 1.5)

	if got.Minutes != one.Minutes || got.Starts != one.Starts {
		t.Errorf("minutes/starts moved: %d/%d against %d/%d",
			got.Minutes, got.Starts, one.Minutes, one.Starts)
	}
	for _, c := range []struct {
		name     string
		was, now float64
	}{
		{"xG", one.XG, got.XG}, {"xA", one.XA, got.XA}, {"xGC", one.XGC, got.XGC},
	} {
		if math.Abs(c.was-c.now) > 0.01 {
			t.Errorf("%s moved from %.2f to %.2f blending a player with himself",
				c.name, c.was, c.now)
		}
	}
	if got.DefCon != one.DefCon || got.Bonus != one.Bonus || got.Yellow != one.Yellow {
		t.Errorf("counting stats moved: %+v", got)
	}
}

// TestBlendPriorsHandlesNothing — a player with no minutes anywhere must not
// divide by zero or invent a record.
func TestBlendPriorsHandlesNothing(t *testing.T) {
	if got := BlendPriors(nil, 1.5); got.Minutes != 0 {
		t.Errorf("empty input produced %+v", got)
	}
	got := BlendPriors([]PriorSeasonStats{
		{PriorPlayer: PriorPlayer{Minutes: 0, XG: 5}, SeasonsAgo: 0},
	}, 1.5)
	if got.Minutes != 0 || got.XG != 0 {
		t.Errorf("a season with no minutes produced %+v", got)
	}
}

// TestSeasonBeforeFormatting moved to cmd/armband, and the move is the finding.
//
// It used to live here with a local re-implementation of the arithmetic, justified
// as "the arithmetic is the part worth pinning, not the location". That is the
// standing rule "a diagnostic must never carry its own copy of the thing it is
// checking", and it cost what the rule predicts: when `seasonBefore` was
// consolidated onto `backtest.PriorSeasonName` it began answering the archive's
// two-digit form for a four-digit input, and this test went on passing because it
// was exercising the deleted code. See cmd/armband/seasonbefore_test.go, which can
// only call the real function.

// TestBonusIsAScheduleNotAConstant records why the flat weight was replaced.
//
// The bonus term is circular in the strict sense: BPS is driven by goals,
// assists, clean sheets, saves and defensive actions, all of which the model
// already prices. It survives because BPS also rewards plenty the model never
// sees — passes completed, tackles won, key passes, big chances created,
// recoveries — so it is badly calibrated and informative at once.
//
// What decides its worth is whether the rate describes the player now or the
// player a year ago at possibly another club. Held opening fifteen, four
// seasons, by the gameweek the entry began at:
//
//	weight   from GW1   from GW11   from GW21
//	0            6626        5473        3271
//	0.5          6659        5496        3473
//	1.0          6341        5617        3530
//	1.5          6306        5761        3619
//
// Monotone harmful before a ball is kicked, monotone helpful after ten
// gameweeks. Interpolating between the two ends on the share of the rate that
// is current-season evidence beats the flat constant on both metrics and at
// every start point:
//
//	prior/evidence   hold@1  hold@11  hold@21    HOLD   POLICY
//	flat 1.0           6341     5617     3530   15488    17847
//	0.5 / 1.5          6671     5633     3567   15871    17988
//	0/2.0              6632     5655     3593   15880    17951
//
// 0.5/1.5 ships: it wins the policy metric outright, is within 9 of the best
// held, and is the smaller claim. 0.5/2.0 was rejected despite a similar total
// because it *loses* at a GW21 start, so it is not uniformly better.
func TestBonusIsAScheduleNotAConstant(t *testing.T) {
	w := DefaultWeights()
	if w.BonusWeight != 1.5 {
		t.Errorf("BonusWeight is %v, want 1.5 — the evidence end of the schedule", w.BonusWeight)
	}
	if w.BonusPriorWeight != 0.5 {
		t.Errorf("BonusPriorWeight is %v, want 0.5 — the end applied when the bonus rate "+
			"is entirely last season's", w.BonusPriorWeight)
	}
	if !(w.BonusPriorWeight < w.BonusWeight) {
		t.Error("the prior end must sit below the evidence end, or the schedule is backwards")
	}
}

// dunkHistory is a real long-serving centre-half's history_past as FPL returned
// it on 2026-08-13, most recent first, with `NoExpected` set exactly where the
// feed carries "0.00" beside real minutes.
//
// Real rather than invented because the defect is about a shape the payload
// actually has: five full seasons of genuine football that report no expected
// goals conceded because the statistic did not exist yet.
func dunkHistory(flagged bool) []PriorSeasonStats {
	type row struct {
		mins int
		xgc  float64
		old  bool
	}
	rows := []row{
		{2837, 41.90, false}, // 2025/26
		{2081, 35.69, false}, // 2024/25
		{2869, 49.76, false}, // 2023/24
		{3240, 46.43, false}, // 2022/23
		{2573, 0, true},      // 2021/22 — real minutes, no such statistic
		{2932, 0, true},      // 2020/21
		{3230, 0, true},      // 2019/20
		{3151, 0, true},      // 2018/19
		{3420, 0, true},      // 2017/18
	}
	out := make([]PriorSeasonStats, 0, len(rows))
	for i, r := range rows {
		out = append(out, PriorSeasonStats{
			PriorPlayer: PriorPlayer{Minutes: r.mins, XGC: r.xgc},
			SeasonsAgo:  i,
			NoXG:        flagged && r.old,
			NoXGC:       flagged && r.old,
		})
	}
	return out
}

// TestAnAbsentSeasonDoesNotDiluteAPrior is the regression for the defect that a
// missing statistic arrives from FPL as an explicit zero.
//
// # Why this asserts a DIFFERENCE and not just a value
//
// The failure being guarded against is silent by construction: with the flags
// unset the blend still returns a plausible number, just a smaller one. So a test
// that only checked the fixed arm would pass identically if `NoExpected` were
// deleted and every season were treated as measured — the inert-feature failure
// this project has shipped six times. Both arms are computed and the gap is
// asserted, so deleting the flag fails here rather than somewhere downstream.
func TestAnAbsentSeasonDoesNotDiluteAPrior(t *testing.T) {
	const halfLife = 2.0

	fixed := BlendPriors(dunkHistory(true), halfLife)
	broken := BlendPriors(dunkHistory(false), halfLife)

	rate := func(p PriorPlayer) float64 { return p.XGC / float64(p.Minutes) * 90 }

	// The blended minutes must be IDENTICAL. Those five seasons are real
	// football and the player really did play them; what is missing is one
	// statistic, not the season. A fix that dropped the rows outright would
	// change his minutes and would be wrong.
	if fixed.Minutes != broken.Minutes {
		t.Errorf("blended minutes moved, %d against %d: the flag must exclude a "+
			"season from the statistics it cannot supply, not from the blend",
			fixed.Minutes, broken.Minutes)
	}

	// The corrected rate is the minutes-and-recency-weighted rate over the four
	// seasons that measured it, computed here independently of the code under
	// test rather than copied from its output.
	var num, den float64
	for _, s := range dunkHistory(true) {
		if s.NoXG {
			continue
		}
		w := math.Pow(0.5, float64(s.SeasonsAgo)/halfLife)
		num += s.XGC * w
		den += float64(s.Minutes) * w
	}
	want := num / den * 90
	// Tolerance, not exactness, and the reason is in the return type rather than
	// in the arithmetic: BlendPriors reports Minutes as a rounded int, so
	// recovering a rate by dividing by it carries up to half a minute of error —
	// about 0.02% on a 2,700-minute blend. Asserting 1e-9 here would be asserting
	// that a documented rounding does not happen.
	if got := rate(fixed); math.Abs(got-want)/want > 5e-4 {
		t.Errorf("corrected xGC per 90 = %.6f, want %.6f", got, want)
	}

	// And the size of what was being lost, so a reader knows why this matters.
	lost := 1 - rate(broken)/rate(fixed)
	if lost < 0.10 {
		t.Errorf("the unflagged blend loses only %.1f%% of the rate; this test is "+
			"meant to demonstrate a material dilution and no longer does", 100*lost)
	}
	t.Logf("unflagged blend under-rates his expected goals conceded by %.1f%% "+
		"(%.4f against %.4f per 90)", 100*lost, rate(broken), rate(fixed))

	// A player with nothing measured comes back zero rather than wrong, which is
	// what the shipped single-season read gives him too — every season available
	// reports the statistic as zero.
	var allOld []PriorSeasonStats
	for i, m := range []int{3000, 2800, 2600} {
		allOld = append(allOld, PriorSeasonStats{
			PriorPlayer: PriorPlayer{Minutes: m}, SeasonsAgo: i, NoXG: true, NoXGC: true,
		})
	}
	if p := BlendPriors(allOld, halfLife); p.XGC != 0 || p.Minutes == 0 {
		t.Errorf("a player with no measured season should keep his minutes and "+
			"report no expected goals conceded, got minutes %d xGC %.3f",
			p.Minutes, p.XGC)
	}
}
