package backtest

import (
	"encoding/json"
	"math"
	"sort"
	"testing"

	"armband/internal/fpl"
)

// xgcSeasonsWithRealData is the seasons whose archive carries a real
// `expected_goals_conceded` column, which are the only ones the reconstruction can be
// validated against.
//
// 2022-23 is included and it is two thirds of a season: the column is zero for
// gameweeks 1-15 and real from GW16, the same boundary as `starts` and `xG`. The
// validation reads only rows that carry a real figure, so the partial season
// contributes its real part rather than being thrown away.
func xgcSeasonsWithRealData() []string {
	return []string{"2022-23", "2023-24", "2024-25", "2025-26"}
}

// TestDiagXGCReconstruction validates the xGC chain against the truth, and it is where
// xgcScale comes from.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagXGCReconstruction -v
//
// # The design, which is the part that matters
//
// The reconstruction is run **in full** on seasons that already have real xGC, and
// then compared against it. Nothing about those seasons is special to the method: it
// reads per-player xG, the fixture list and minutes, all of which the missing seasons
// also have once the Understat backfill has run. So this is the method being scored on
// data it does not otherwise see, which is the closest thing to a held-out test the
// archive permits.
//
// It calls `reconstructedXGC` — the shipped function — rather than reimplementing the
// chain. A diagnostic carrying its own copy of the thing it checks has shipped twice in
// this package and been wrong both times.
//
// # What it reports
//
//   - **The raw scale**, actual over reconstructed, pooled and per season. This is
//     the constant, and it is reported before correction so a reader can see what is
//     being corrected for.
//   - **Correlation and MAE at player-gameweek level**, after correction. The error
//     that survives is the substitution channel and the double-gameweek split, which
//     is what the flag on every reconstructed row exists to warn about.
//   - **The ever-present check**, which is the sharp one. For a player who played
//     the full 90 in a single-fixture gameweek the reconstruction should be *exact*
//     up to the scale, because per-player xGC simply is the club figure. If that
//     population does not tighten, the method is wrong rather than noisy.
func TestDiagXGCReconstruction(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type acc struct {
		n                   int
		sumA, sumR          float64
		sumAbs              float64
		sumAA, sumRR, sumAR float64
		// Ever-presents and everyone else, kept apart because the pooled ratio
		// turns out to be a CANCELLATION of the two rather than a null. Splitting
		// them is what converts a residual into a mechanism.
		everN                           int
		everSumAbs, everSumA, everSumR  float64
		everSumAA, everSumRR, everSumAR float64
		partN                           int
		partSumAbs, partSumA, partSumR  float64
	}
	pooled := acc{}
	seasonScale := map[string]float64{}

	t.Log("season    n        scale   corr    MAE  MAE%   ever-present n   ever MAE%")
	for _, name := range xgcSeasonsWithRealData() {
		s := loadSeason(t, cfg, name)
		// Scale 1 deliberately: this measures the overshoot rather than applying
		// it, and applying it here would make the reported scale a statement
		// about xgcScale's own value.
		rec, _ := reconstructedXGC(s, 1.0)
		matches := clubMatches(s)

		a := acc{}
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for _, gw := range sortedGameweeks(rec[id]) {
				g := p.GWs[gw]
				// Only rows carrying a real figure. In 2022-23 that excludes
				// GW1-15, where actual is zero because the column is empty and
				// not because nobody threatened the goal.
				if g.XGC <= 0 || g.XGCReconstructed {
					continue
				}
				act, r := g.XGC, rec[id][gw]
				a.n++
				a.sumA += act
				a.sumR += r
				a.sumAbs += math.Abs(act - r)
				a.sumAA += act * act
				a.sumRR += r * r
				a.sumAR += act * r
				if g.Minutes == 90 && matches[[2]int{p.Team, gw}] == 1 {
					a.everN++
					a.everSumA += act
					a.everSumR += r
					a.everSumAbs += math.Abs(act - r)
					a.everSumAA += act * act
					a.everSumRR += r * r
					a.everSumAR += act * r
				} else {
					a.partN++
					a.partSumA += act
					a.partSumR += r
					a.partSumAbs += math.Abs(act - r)
				}
			}
		}
		if a.n == 0 {
			t.Fatalf("%s: no rows with real xGC — the season list is wrong", name)
		}
		scale := a.sumR / a.sumA
		seasonScale[name] = scale
		t.Logf("%-9s %-7d %6.4f  %5.3f  %5.3f %5.1f%%  %-14d %6.1f%%",
			name, a.n, scale, corr(a.sumAR, a.sumA, a.sumR, a.sumAA, a.sumRR, a.n),
			a.sumAbs/float64(a.n), 100*a.sumAbs/a.sumA,
			a.everN, 100*a.everSumAbs/a.everSumA)

		pooled.n += a.n
		pooled.sumA += a.sumA
		pooled.sumR += a.sumR
		pooled.sumAbs += a.sumAbs
		pooled.sumAA += a.sumAA
		pooled.sumRR += a.sumRR
		pooled.sumAR += a.sumAR
		pooled.everN += a.everN
		pooled.everSumA += a.everSumA
		pooled.everSumR += a.everSumR
		pooled.everSumAbs += a.everSumAbs
		pooled.everSumAA += a.everSumAA
		pooled.everSumRR += a.everSumRR
		pooled.everSumAR += a.everSumAR
		pooled.partN += a.partN
		pooled.partSumA += a.partSumA
		pooled.partSumR += a.partSumR
		pooled.partSumAbs += a.partSumAbs
	}

	scale := pooled.sumR / pooled.sumA
	t.Logf("pooled    %-7d %6.4f  %5.3f  %5.3f %5.1f%%  %-14d %6.1f%%",
		pooled.n, scale,
		corr(pooled.sumAR, pooled.sumA, pooled.sumR, pooled.sumAA, pooled.sumRR, pooled.n),
		pooled.sumAbs/float64(pooled.n), 100*pooled.sumAbs/pooled.sumA,
		pooled.everN, 100*pooled.everSumAbs/pooled.everSumA)

	// **The pooled ratio is a cancellation, and this is the line that shows it.**
	//
	// The two populations sit on opposite sides of 1: ever-presents high, partial
	// appearances low. So 0.9994 is not "the chain is unbiased" — it is two biases of
	// opposite sign meeting in the middle, weighted by how much xGC mass each carries.
	// Read these two rows rather than the pooled one when asking whether the method is
	// right, because a pooled null built this way would survive both errors growing.
	//
	// The direction is what linear minutes-prorating predicts against a late-loaded
	// danger profile: a player withdrawn at 60 is credited 2/3 of the club's exposure,
	// and this record's own goal-timing finding puts the closing twenty to thirty real
	// minutes at roughly 1.2x the match average — so he faced less than 2/3 and the
	// reconstruction over-credits him, while a late substitute is under-credited. That
	// is a mechanism rather than a residual, and it is **consistent with** the recorded
	// timing figure rather than a second measurement of it.
	t.Logf("ever-present  n %-6d ratio %.4f  corr %.3f  MAE %.1f%% of mean",
		pooled.everN, pooled.everSumR/pooled.everSumA,
		corr(pooled.everSumAR, pooled.everSumA, pooled.everSumR,
			pooled.everSumAA, pooled.everSumRR, pooled.everN),
		100*pooled.everSumAbs/pooled.everSumA)
	t.Logf("partial mins  n %-6d ratio %.4f              MAE %.1f%% of mean",
		pooled.partN, pooled.partSumR/pooled.partSumA,
		100*pooled.partSumAbs/pooled.partSumA)

	// The per-season spread is what says whether one constant is defensible at all.
	// A scale that swings season to season is a different quantity each year and
	// should not be a constant; the record's own standard is a shape rather than an
	// argmax, and for a single number the shape wanted here is "flat".
	lo, hi := math.Inf(1), math.Inf(-1)
	names := make([]string, 0, len(seasonScale))
	for n := range seasonScale {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		lo, hi = math.Min(lo, seasonScale[n]), math.Max(hi, seasonScale[n])
	}
	t.Logf("per-season scale spans %.4f to %.4f, a range of %.1f%% of the pooled value",
		lo, hi, 100*(hi-lo)/scale)

	// The gate is 1.5%, which is wider than the 0.6% the four seasons span and
	// narrower than the 2-4% correction this repair was expected to need. It is a
	// check that the identity still holds rather than a check on a fitted value —
	// see xgcScale, which ships at 1 on mechanism.
	if math.Abs(scale-xgcScale) > 0.015 {
		t.Errorf("reconstructed over actual is %.4f and xgcScale is %.4f. Either the "+
			"identity that a club's xGC is its opponents' xG has stopped holding on "+
			"this archive, or the chain has a bug; a 1.5%% gate is loose enough that "+
			"this is not the fourth decimal moving", scale, xgcScale)
	}

	// The two estimators, side by side, because the archive audit recorded "a
	// consistent 1.02-1.04 overshoot that wants a calibrated offset" and the pooled
	// figure above does not reproduce it. A ratio of totals and a mean of per-club
	// ratios are different quantities, and the second is biased upward whenever the
	// denominator is small and variable — which a club's expected goals in one
	// gameweek certainly is. Reporting both says which of the two the recorded
	// figure was, rather than leaving a disagreement on the record with no cause.
	t.Log("")
	t.Log("club-gameweek estimators   ratio of totals   mean of ratios   n")
	for _, name := range xgcSeasonsWithRealData() {
		s := loadSeason(t, cfg, name)
		rec, _ := reconstructedXGC(s, 1.0)
		matches := clubMatches(s)

		// Actual club xGA read off the ever-presents, which is how the audit
		// derived it: a player who went the full 90 in a single-fixture gameweek
		// records the club figure exactly.
		actual := map[[2]int]float64{}
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for gw := 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes != 90 || g.XGC <= 0 || g.XGCReconstructed {
					continue
				}
				if matches[[2]int{p.Team, gw}] == 1 {
					actual[[2]int{p.Team, gw}] = g.XGC
				}
			}
		}
		var sumA, sumR, sumRatio float64
		var n int
		keys := make([][2]int, 0, len(actual))
		for k := range actual {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i][0] != keys[j][0] {
				return keys[i][0] < keys[j][0]
			}
			return keys[i][1] < keys[j][1]
		})
		for _, k := range keys {
			// The club's reconstructed xGA is the same quantity for every one of
			// its players, so recover it from any ever-present's share of 1.
			var r float64
			for _, id := range sortedSeasonPlayerIDs(s) {
				p := s.Players[id]
				if p.Team != k[0] {
					continue
				}
				if g, ok := p.GWs[k[1]]; ok && g.Minutes == 90 {
					r = rec[id][k[1]]
					break
				}
			}
			if r == 0 {
				continue
			}
			n++
			sumA += actual[k]
			sumR += r
			sumRatio += r / actual[k]
		}
		t.Logf("%-26s %15.4f   %14.4f   %d", name, sumR/sumA, sumRatio/float64(n), n)
	}
}

// corr is Pearson's r from running sums.
func corr(sumXY, sumX, sumY, sumXX, sumYY float64, n int) float64 {
	fn := float64(n)
	num := sumXY - sumX*sumY/fn
	den := math.Sqrt((sumXX - sumX*sumX/fn) * (sumYY - sumY*sumY/fn))
	if den == 0 {
		return 0
	}
	return num / den
}

// TestTheXGCReconstructionOnlyFillsHolesAndIsIdempotent pins the two properties every
// half of this repair shares, on a stub where the answer is arithmetic.
//
// The stub is one gameweek, two clubs, one fixture. Club 1's two players are expected
// to score 1.5 between them, so club 2 concedes 1.5 before the scale — and club 2's
// keeper, who played the full 90, must receive exactly that divided by xgcScale. His
// team-mate, on for 45, must receive exactly half of it. That is the whole method, and
// on numbers this small a sign error or a misplaced denominator is visible rather than
// plausible.
func TestTheXGCReconstructionOnlyFillsHolesAndIsIdempotent(t *testing.T) {
	// A season the repair table names, because applyXGCRepair refuses one it does
	// not — the window comes from that table and an unlisted season has none.
	s := &Season{
		Name: "2021-22",
		Players: map[int]*Player{
			1: {ID: 1, Team: 1, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1, XG: 1.0}}},
			2: {ID: 2, Team: 1, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1, XG: 0.5}}},
			3: {ID: 3, Team: 2, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1}}},
			4: {ID: 4, Team: 2, GWs: map[int]GW{1: {Minutes: 45, Fixtures: 1}}},
			// Already carries a figure, so the repair must leave it alone.
			5: {ID: 5, Team: 2, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1, XGC: 9.0}}},
		},
		Fixtures: []fpl.Fixture{{ID: 1, Event: intp(1), TeamH: 1, TeamA: 2}},
	}
	res := s.applyXGCRepair()

	want := 1.5 / xgcScale
	for _, tc := range []struct {
		id   int
		want float64
		why  string
	}{
		{3, want, "an ever-present concedes the whole club figure"},
		{4, want / 2, "half a match is half the exposure"},
		{5, 9.0, "a row that already had a value is never overwritten"},
		{1, 0, "the club that did the threatening concedes its opponent's xG, which is nil here"},
	} {
		if got := s.Players[tc.id].GWs[1].XGC; math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("element %d: xGC %.6f, want %.6f — %s", tc.id, got, tc.want, tc.why)
		}
	}
	// Two applied, not four. Club 1's players face an opponent with no expected
	// goals at all, so their reconstruction is exactly zero and is left unwritten —
	// see xgcRepairResult.Empty for why a stored zero is worse than an absent one.
	if res.Applied != 2 || res.Skipped != 1 || res.Empty != 2 {
		t.Errorf("applied %d skipped %d empty %d, want 2, 1 and 2",
			res.Applied, res.Skipped, res.Empty)
	}
	if !s.Players[3].GWs[1].XGCReconstructed || s.Players[5].GWs[1].XGCReconstructed {
		t.Error("the reconstructed flag does not distinguish a derived row from a real one, " +
			"which is the guard-rail against quoting this as evidence about substitutions")
	}

	// Idempotence. A second pass must fill nothing, or a season loaded twice in one
	// process drifts — and this repair runs on every Load rather than once, because
	// it is applied after the cache.
	before := s.Players[3].GWs[1].XGC
	again := s.applyXGCRepair()
	if again.Applied != 0 || s.Players[3].GWs[1].XGC != before {
		t.Errorf("a second pass applied %d rows and moved xGC from %.6f to %.6f; "+
			"the repair must only ever fill a zero",
			again.Applied, before, s.Players[3].GWs[1].XGC)
	}
}

// TestTheXGCSwitchWorksOnACacheHit is the ungated twin of
// TestXGRepairSwitchWorksOnACacheHit, and it exists because its absence was a real gap.
//
// The xGC hatch's cache-seam property was tested only under DIAG, against the real
// archive. So `go test ./...` — the documented gate, and the one anybody actually runs
// — exercised the xG hatch's seam and not this one. If somebody later "tidies"
// `applyXGCRepair` into `fetch`, where the repair would be baked into the cached bytes,
// the ungated suite would stay green and `FPL_NO_XGC_REPAIR` would silently report both
// arms as identical. That is the shape of this package's signature failure and it is
// exactly what the sibling test was written to prevent, so having only one of the two
// was worse than it looked.
//
// Synthetic and offline: two clubs, one fixture, one gameweek, no network.
func TestTheXGCSwitchWorksOnACacheHit(t *testing.T) {
	// A repaired season, with xG present and xGC absent — the state the archive
	// leaves 2021-22 in.
	orig := &Season{
		Name: "2021-22",
		Players: map[int]*Player{
			1: {ID: 1, Code: 1001, Team: 1, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1, XG: 1.5}}},
			2: {ID: 2, Code: 1002, Team: 2, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1}}},
		},
		Fixtures: []fpl.Fixture{{ID: 1, Event: intp(1), TeamH: 1, TeamA: 2}},
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// What is cached must be the UNREPAIRED archive. If the repair ran before the
	// write, or XGCReconstructed were serialised, this is where it would show.
	var cached Season
	if err := json.Unmarshal(b, &cached); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if g := cached.Players[2].GWs[1]; g.XGC != 0 || g.XGCReconstructed {
		t.Errorf("the cached season already carries reconstructed xGC (%.4f, flag %v); "+
			"the repair must run after the cache is written, or the escape hatch reads "+
			"a repaired cache and reports both arms as identical",
			g.XGC, g.XGCReconstructed)
	}

	load := func(off bool) *Season {
		var s Season
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if off {
			t.Setenv("FPL_NO_XGC_REPAIR", "1")
		} else {
			t.Setenv("FPL_NO_XGC_REPAIR", "")
		}
		out, err := repaired(&s)
		if err != nil {
			t.Fatalf("repaired: %v", err)
		}
		return out
	}

	on, off := load(false), load(true)
	if off.XGRepair.XGC.Applied != 0 || off.Players[2].GWs[1].XGC != 0 {
		t.Errorf("with the switch off the reconstruction applied %d rows and wrote "+
			"%.4f from a cache hit", off.XGRepair.XGC.Applied, off.Players[2].GWs[1].XGC)
	}
	// The arms have to be *capable* of differing, or this passes on a corpse.
	if on.XGRepair.XGC.Applied == 0 {
		t.Error("with the switch on the reconstruction applied nothing, so the arm " +
			"above proves nothing")
	}
	if got, want := on.Players[2].GWs[1].XGC, 1.5/xgcScale; math.Abs(got-want) > 1e-9 {
		t.Errorf("repaired xGC %.6f, want %.6f", got, want)
	}
}

// TestABlankGameweekReconstructsToNothingRatherThanNaN is the divide-by-zero, and it
// is a real case rather than a hypothetical: a player who moves clubs in January has
// his August rows filed against the club he finished at, which may have been blank
// that week. Measured across the archive that is 1 to 15 player-gameweeks a season.
//
// A NaN here would propagate silently into Score and out through the optimiser, which
// compares it and gets false from every comparison.
func TestABlankGameweekReconstructsToNothingRatherThanNaN(t *testing.T) {
	s := &Season{
		Name: "2021-22",
		Players: map[int]*Player{
			1: {ID: 1, Team: 1, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1, XG: 1.0}}},
			// Club 3 has no fixture in gameweek 1 at all.
			2: {ID: 2, Team: 3, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1}}},
		},
		Fixtures: []fpl.Fixture{{ID: 1, Event: intp(1), TeamH: 1, TeamA: 2}},
	}
	s.applyXGCRepair()
	got := s.Players[2].GWs[1].XGC
	if math.IsNaN(got) || math.IsInf(got, 0) || got != 0 {
		t.Errorf("a blank gameweek reconstructed to %v, want exactly 0", got)
	}
}
