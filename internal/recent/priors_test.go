package recent

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// isakPast is his real record as FPL reports it, oldest first — which is the
// order the API returns and the order that has to be reversed.
func isakPast() []fpl.PastSeason {
	return []fpl.PastSeason{
		{SeasonName: "2022/23", Minutes: 1520, Starts: 17, ExpectedGoals: fpl.Num(8.11), ExpectedAssists: fpl.Num(1.37)},
		{SeasonName: "2023/24", Minutes: 2253, Starts: 27, ExpectedGoals: fpl.Num(20.22), ExpectedAssists: fpl.Num(2.68)},
		{SeasonName: "2024/25", Minutes: 2758, Starts: 34, ExpectedGoals: fpl.Num(20.33), ExpectedAssists: fpl.Num(3.59)},
		{SeasonName: "2025/26", Minutes: 694, Starts: 8, ExpectedGoals: fpl.Num(2.59), ExpectedAssists: fpl.Num(0.08)},
	}
}

// TestBlendPastReversesFPLsOrder is the bug this would have if it trusted the
// order it is given: FPL returns oldest first, so a naive walk would treat the
// oldest season as the most recent and weight it hardest.
func TestBlendPastReversesFPLsOrder(t *testing.T) {
	got, ok := blendPast(isakPast(), 1.5)
	if !ok {
		t.Fatal("no prior from four seasons")
	}
	rate := (got.XG + got.XA) * 90 / float64(got.Minutes)
	t.Logf("blended: %d mins, %.3f xGI/90", got.Minutes, rate)

	// Weighted to the recent injured season, not to 2022-23. If the order were
	// reversed the oldest season (0.561 xGI/90) would dominate instead.
	if rate < 0.6 || rate > 0.85 {
		t.Errorf("blended rate %.3f is outside the range the recent healthy seasons "+
			"imply; the season ordering is probably reversed", rate)
	}
	if got.Minutes >= 2253 || got.Minutes <= 694 {
		t.Errorf("blended minutes %d should sit between the injured season and the "+
			"healthy ones", got.Minutes)
	}
}

// TestBlendPastLeavesAFullSeasonAlone — a player with a complete recent season
// must not have older ones smoothed into him. That dilutes genuine improvement,
// which is most players most of the time.
func TestBlendPastLeavesAFullSeasonAlone(t *testing.T) {
	past := []fpl.PastSeason{
		{SeasonName: "2024/25", Minutes: 1000, Starts: 11, ExpectedGoals: fpl.Num(2)},
		{SeasonName: "2025/26", Minutes: 3000, Starts: 34, ExpectedGoals: fpl.Num(18)},
	}
	got, ok := blendPast(past, 1.5)
	if !ok {
		t.Fatal("no prior")
	}
	if got.Minutes != 3000 || math.Abs(got.XG-18) > 0.01 {
		t.Errorf("a 3000-minute season was blended: %+v", got)
	}
}

// TestBlendPastDegradesToOneSeason — with blending off, or only one season on
// record, the result is the most recent season unchanged. That makes this a
// strict generalisation of the single-season prior.
func TestBlendPastDegradesToOneSeason(t *testing.T) {
	thin := []fpl.PastSeason{{SeasonName: "2025/26", Minutes: 694, Starts: 8, ExpectedGoals: fpl.Num(2.59)}}
	got, ok := blendPast(thin, 1.5)
	if !ok || got.Minutes != 694 {
		t.Errorf("one season produced %+v", got)
	}
	got, ok = blendPast(isakPast(), 0)
	if !ok || got.Minutes != 694 {
		t.Errorf("half-life 0 should give the most recent season, got %+v", got)
	}
	if _, ok := blendPast(nil, 1.5); ok {
		t.Error("no history produced a prior")
	}
	if _, ok := blendPast([]fpl.PastSeason{{SeasonName: "x", Minutes: 0}}, 1.5); ok {
		t.Error("a season with no minutes produced a prior")
	}
}

// TestSeasonsAgoCountsCalendarIndexNotSurvivingRows verifies that SeasonsAgo is
// the chronological distance from the most recent season, not the count of
// surviving (non-zero-minute) rows. This is load-bearing for exponential
// weighting: 0.5^(SeasonsAgo/halfLife) must decay with calendar time.
//
// The bug was that internal/recent counted surviving rows while internal/priors
// counted calendar positions, causing the same player's season to be weighted
// differently on the two paths.
func TestSeasonsAgoCountsCalendarIndexNotSurvivingRows(t *testing.T) {
	// A history with a zero-minute season in the middle.
	// Oldest first, as FPL returns it.
	past := []fpl.PastSeason{
		{SeasonName: "2023/24", Minutes: 1900, Starts: 20, ExpectedGoals: fpl.Num(10.0)},    // 3 years ago
		{SeasonName: "2024/25", Minutes: 0, Starts: 0},                                      // 2 years ago, skipped
		{SeasonName: "2025/26", Minutes: 800, Starts: 10, ExpectedGoals: fpl.Num(2.5)},     // 1 year ago (thin)
	}

	// With blending on, this should gate on the thin season.
	got, ok := blendPast(past, 2.0)
	if !ok {
		t.Fatal("thin season with history should blend")
	}

	// The blended result must weight the 3-year-old season at 0.5^(2/2.0) = 0.5,
	// not at 0.5^(1/2.0) ≈ 0.707 (which would be wrong if it counted surviving rows).
	// Expected xG/minute rate:
	//   (10.0 * 0.5 + 2.5 * 1.0) / (1900 * 0.5 + 800 * 1.0)
	//   = (5.0 + 2.5) / (950 + 800)
	//   = 7.5 / 1750
	//   ≈ 0.00429
	rate := got.XG / float64(got.Minutes)
	expectedRate := 7.5 / 1750.0
	const tolerance = 0.0001
	if math.Abs(rate-expectedRate) > tolerance {
		t.Errorf("blended xG rate %.6f, want %.6f (%.2f%% off); "+
			"SeasonsAgo is likely counting surviving rows, not calendar distance",
			rate, expectedRate, math.Abs(rate-expectedRate)/expectedRate*100)
	}
}

// TestTheLivePriorPathFlagsSeasonsFPLDidNotMeasure is the other half of the
// regression, and it is the half that would actually rot.
//
// analysis.BlendPriors now honours NoExpected, but honouring a flag nobody sets
// is worth nothing — that is the byte-identical null this record calls its
// signature failure, and the live path is where it would land silently, because
// nothing downstream can tell a diluted rate from a real one.
func TestTheLivePriorPathFlagsSeasonsFPLDidNotMeasure(t *testing.T) {
	// Dunk's history_past as FPL returned it on 2026-08-13, oldest first, which
	// is the order the API uses.
	past := []fpl.PastSeason{
		{SeasonName: "2017/18", Minutes: 3420},
		{SeasonName: "2018/19", Minutes: 3151},
		{SeasonName: "2021/22", Minutes: 2573},
		{SeasonName: "2022/23", Minutes: 3240, ExpectedGoalsConceded: 46.43},
		{SeasonName: "2023/24", Minutes: 2869, ExpectedGoalsConceded: 49.76},
		{SeasonName: "2024/25", Minutes: 2081, ExpectedGoalsConceded: 35.69,
			DefensiveContribution: 126},
	}
	// A thin most recent season, so the blend gate opens and the older seasons
	// are actually reached. Without this the function returns hist[0] and the
	// test would pass while measuring nothing.
	past = append(past, fpl.PastSeason{
		SeasonName: "2025/26", Minutes: 900, ExpectedGoalsConceded: 13.0,
		DefensiveContribution: 70,
	})

	got, ok := blendPast(past, 2.0)
	if !ok {
		t.Fatal("blendPast declined a thin most recent season with real history")
	}

	// The rate must sit in the range the measured seasons support. Diluted by the
	// three blind seasons it lands near 1.1; measured properly it is near 1.4.
	rate := got.XGC / float64(got.Minutes) * 90
	if rate < 1.30 {
		t.Errorf("blended xGC of %.4f per 90 is below what the measured seasons "+
			"support, so the pre-2022/23 zeroes are still being blended in as "+
			"real observations", rate)
	}

	// Same for defensive contribution, whose boundary is a different season —
	// 2024/25 in the API against 2025-26 in the archive. A single shared flag
	// would get one of the two wrong.
	if got.DefCon == 0 {
		t.Error("defensive contribution blended to zero: the seasons that do " +
			"carry it are being averaged against ones that never could")
	}
}
