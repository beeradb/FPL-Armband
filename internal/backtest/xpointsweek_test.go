package backtest

// The accumulated-xPoints instrument, pinned where it can actually go wrong.
//
// `weekScoreWithChip` carries two running totals through one set of decisions —
// which eleven, who blanked, who is legally substituted in, whose armband it ends
// up being. The decisions are shared by construction, so what these tests are for
// is the *other* half: that the xPoints total is fed the player's **xPoints** at
// every point the points total is fed his points, and in particular where a value
// is added twice.
//
// # Why the armband is the case worth a test of its own
//
// Every other branch adds a player once, so substituting the wrong quantity there
// changes the total by that player's residual and would show up in any comparison
// against a hand-summed season. The armband adds a *second* copy, and the most
// natural way to get this wrong — carry the captain's `Points` alongside his
// xPoints and double the wrong one — produces a total that is still the right
// order of magnitude, still moves with the squad, and is wrong by exactly one
// player's conversion luck a week. That is the size of the effects the pilot this
// instrument exists for is trying to see.
//
// The fixture therefore gives the vice-captain a **large, known residual**: he
// scores a goal off 0.2 xG, so his points and his xPoints differ by 3.2 and the
// two candidate implementations cannot agree by accident. A fixture where every
// player's xPoints equals his points would pass under the mutation it is named
// for, which is the failure this file was written after.

import (
	"context"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// xpWeek is the gameweek every test here scores.
const xpWeek = 5

// mkXPointsSeason is mkSeason with conversion luck in it.
//
// mkSeason's rows carry no xG, xA or xGC at all, so `analysis.XPointsResidual`
// returns zero for every one of them and xPoints is identically points — which is
// exactly the fixture that cannot tell the two apart. Two forwards are given
// underlying numbers here:
//
//	14 (the vice)  1 goal off 0.20 xG  -> residual +3.2, xPoints 2.8 against 6
//	15 (an XI man) 0 goals off 0.90 xG -> residual -3.6, xPoints 9.6 against 6
//
// The second one exists so the *base* sum differs from the base points too, in
// the opposite direction. A test whose only non-zero residual is the doubled
// player's cannot distinguish "the armband doubles xPoints" from "the whole week
// is scored on xPoints".
func mkXPointsSeason() *Season {
	s := mkSeason()
	s.Players[14].GWs[xpWeek] = GW{
		Points: 6, Minutes: 90, Fixtures: 1, Value: 80, Goals: 1, XG: 0.20,
	}
	s.Players[15].GWs[xpWeek] = GW{
		Points: 6, Minutes: 90, Fixtures: 1, Value: 80, Goals: 0, XG: 0.90,
	}
	// A season built by hand still has to be calibrated, because a loaded one is —
	// `repaired()` does this for every season Load returns, and a fixture that
	// skipped it would be scoring through a zero scale, which XPointsResidual
	// refuses. On a fixture this small the expected totals are far below
	// minCalibrationSample, so every position falls back to neutral 1.0 and the
	// residuals in the comment above are unchanged. That is the point: this
	// fixture is about the armband, not about the scale.
	//
	// Re-resolved rather than left to `mkSeason`, because the rows above were
	// written after it ran and the fit is over the rows.
	s.resolveInstrumentInputs()
	return s
}

var xpXI = []int{1, 3, 4, 5, 8, 9, 10, 11, 13, 14, 15}

// baseXPointsOf sums the eleven's own xPoints with nobody doubled, straight off
// analysis.XPoints rather than off anything in replay.go — so the expectation is
// independent of the code under test.
func baseXPointsOf(s *Season, ids []int, gw int) float64 {
	var n float64
	for _, id := range ids {
		p := s.Players[id]
		g := p.GWs[gw]
		if g.Minutes == 0 {
			continue
		}
		n += analysis.XPoints(analysis.XPointsGW{
			Position: p.Type, Fixtures: g.Fixtures, Minutes: g.Minutes,
			Points: g.Points, Goals: g.Goals, Assists: g.Assists,
			CleanSheets: g.CleanSheets, GoalsConceded: g.GoalsConceded,
			XG: g.XG, XA: g.XA, XGC: g.XGC,
		}, p.Conversion, p.Rules)
	}
	return n
}

// notIf renders a boolean into a sentence that reads either way round.
func notIf(b bool) string {
	if b {
		return ""
	}
	return " not"
}

func closeEnough(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

// TestTheViceCaptainIsDoubledOnXPointsNotOnPoints is the pinning test the
// accumulated-xPoints protocol requires before any sweep is run on the metric.
//
// The captain blanks, the vice played, and the fallback fires. What must be added
// twice is the vice's **xPoints** — 2.8 here — and not his realised 6.
func TestTheViceCaptainIsDoubledOnXPointsNotOnPoints(t *testing.T) {
	old := viceCaptainFallback
	viceCaptainFallback = true
	defer func() { viceCaptainFallback = old }()

	s := mkXPointsSeason()
	// The captain blanks; the vice plays. Same shape as
	// TestViceCaptainCoversABlankCaptain, which pins the points half.
	s.Players[13].GWs[xpWeek] = GW{Points: 0, Minutes: 0, Value: 80}

	xi := idsToPlayers(s, xpXI)
	got := weekScore(xi, nil, xpWeek, 13, 14)

	base := baseXPointsOf(s, xpXI, xpWeek)
	viceG := s.Players[14].GWs[xpWeek]
	viceXP := analysis.XPoints(analysis.XPointsGW{
		Position: s.Players[14].Type, Fixtures: viceG.Fixtures,
		Minutes: viceG.Minutes, Points: viceG.Points,
		Goals: viceG.Goals, XG: viceG.XG,
	}, s.Players[14].Conversion, s.Players[14].Rules)

	// The fixture has to be able to fail. If the vice's xPoints equalled his
	// points, doubling either would give the same total and this test would pass
	// under the mutation it exists to catch.
	if closeEnough(viceXP, float64(viceG.Points)) {
		t.Fatalf("the vice's xPoints (%v) equals his points (%d); this fixture "+
			"cannot distinguish the two quantities and the test below is vacuous",
			viceXP, viceG.Points)
	}

	if want := base + viceXP; !closeEnough(got.XPoints, want) {
		t.Errorf("week xPoints %v, want %v (base %v + the VICE's xPoints %v). "+
			"Doubling his realised points instead would give %v",
			got.XPoints, want, base, viceXP, base+float64(viceG.Points))
	}
	// And the points half is untouched by any of this — weekPointsWithChip is a
	// projection of the same call, so a change here would mean the instrument has
	// reached the metric it is supposed to sit beside.
	if got.Points != weekPoints(xi, nil, xpWeek, 13, 14) {
		t.Errorf("weekScore and weekPoints disagree: %d against %d",
			got.Points, weekPoints(xi, nil, xpWeek, 13, 14))
	}
}

// TestNoViceFallbackMeansNoXPointsDoubling is the other half of the pin, and it
// is what makes the first half a statement about the fallback rather than about
// arithmetic.
//
// With `viceCaptainFallback` off — the arm the positive control runs — a blanking
// captain forfeits the armband outright, so the xPoints total must be the bare
// eleven with nothing added twice.
func TestNoViceFallbackMeansNoXPointsDoubling(t *testing.T) {
	old := viceCaptainFallback
	viceCaptainFallback = false
	defer func() { viceCaptainFallback = old }()

	s := mkXPointsSeason()
	s.Players[13].GWs[xpWeek] = GW{Points: 0, Minutes: 0, Value: 80}

	xi := idsToPlayers(s, xpXI)
	got := weekScore(xi, nil, xpWeek, 13, 14)

	base := baseXPointsOf(s, xpXI, xpWeek)
	if !closeEnough(got.XPoints, base) {
		t.Errorf("with the fallback off, week xPoints %v, want the bare eleven %v",
			got.XPoints, base)
	}
	// The contrast between the two tests is the thing the control measures, and
	// it must be the vice's xPoints rather than his points.
	viceG := s.Players[14].GWs[xpWeek]
	if closeEnough(got.XPoints, base+float64(viceG.Points)) {
		t.Error("the fallback appears to be doubling realised points")
	}
}

// TestTheXPointsTotalIsNotJustTheWeekPoints guards the degenerate pass.
//
// If `xPointsOf` ever returned `g.Points` — a plausible "simplification" for a
// season with no underlying data, and what every archived season before 2016 would
// force — every test here would still pass on a fixture with no residuals, and the
// whole instrument would be a rename of the points column. This asserts the two
// totals genuinely differ on a week where the underlying says they should.
func TestTheXPointsTotalIsNotJustTheWeekPoints(t *testing.T) {
	old := viceCaptainFallback
	viceCaptainFallback = true
	defer func() { viceCaptainFallback = old }()

	s := mkXPointsSeason()
	xi := idsToPlayers(s, xpXI)
	got := weekScore(xi, nil, xpWeek, 13, 14)

	if closeEnough(got.XPoints, float64(got.Points)) {
		t.Fatalf("xPoints %v equals points %d on a week carrying 1 goal off 0.20 xG "+
			"and 0 goals off 0.90 xG — the residual is not being applied at all",
			got.XPoints, got.Points)
	}
	// Player 15's residual is negative and the vice is not doubled here (the
	// captain played), so the week must read ABOVE its realised points: 15
	// under-performed his chances by 3.6 and 14 over-performed his by 3.2.
	if got.XPoints <= float64(got.Points) {
		t.Errorf("xPoints %v is not above points %d, but the eleven's net residual "+
			"is negative", got.XPoints, got.Points)
	}
}

// TestBenchBoostAndAutosubsCarryXPointsToo pins the two remaining branches that
// add a player to the total.
//
// Both are places a mirror implementation would have had to re-derive the
// selection rule, which is why there is no mirror — but "the branch exists in the
// shared function" is not the same as "the branch adds the right quantity", and
// only the second is checkable.
func TestBenchBoostAndAutosubsCarryXPointsToo(t *testing.T) {
	old := viceCaptainFallback
	viceCaptainFallback = true
	defer func() { viceCaptainFallback = old }()

	s := mkXPointsSeason()
	// Move the two players with underlying onto the bench so the branch under
	// test is the only way their xPoints can reach the total.
	xiIDs := []int{1, 3, 4, 5, 6, 8, 9, 10, 11, 12, 13}
	benchIDs := []int{2, 7, 14, 15}
	xi, bench := idsToPlayers(s, xiIDs), idsToPlayers(s, benchIDs)

	// Bench boost: everybody who played scores, nobody is substituted.
	bb := weekScoreWithChip(xi, bench, xpWeek, 13, 0, chipBenchBoost)
	want := baseXPointsOf(s, xiIDs, xpWeek) + baseXPointsOf(s, benchIDs, xpWeek) +
		xPointsOf(s.Players[13], s.Players[13].GWs[xpWeek])
	if !closeEnough(bb.XPoints, want) {
		t.Errorf("bench-boosted week xPoints %v, want %v", bb.XPoints, want)
	}

	// Autosubs: a starting forward blanks and the bench forward with the known
	// residual comes on for him. The substitute must arrive on both metrics.
	s.Players[13].GWs[xpWeek] = GW{Points: 0, Minutes: 0, Value: 80}
	xi = idsToPlayers(s, xiIDs)
	sub := weekScore(xi, bench, xpWeek, 0, 0)
	// Who comes on is the legal-autosub rule's business and this test does not
	// re-derive it — it reads the answer off the points half, which
	// TestAutosubsCoverABlank and the formation tests already pin, and then asks
	// only whether the xPoints half moved by the SAME player's xPoints.
	//
	// It happens to be a case worth having: the eleven is 4-5-1, so replacing the
	// blanking forward with a defender would leave no forward and is refused, and
	// the substitute is the bench forward with the +3.2 residual. His points (6)
	// and his xPoints (2.8) are far apart, so the branch cannot pass by adding the
	// wrong one.
	bare := baseXPointsOf(s, xiIDs, xpWeek)
	barePts := 0
	for _, p := range xi {
		if g := p.GWs[xpWeek]; g.Minutes > 0 {
			barePts += g.Points
		}
	}
	incPts := sub.Points - barePts
	if incPts <= 0 {
		t.Fatalf("autosub added nothing on points: %d against a bare eleven of %d",
			sub.Points, barePts)
	}
	var came *Player
	for _, id := range benchIDs {
		p := s.Players[id]
		if g := p.GWs[xpWeek]; g.Minutes > 0 && g.Points == incPts {
			came = p
			break
		}
	}
	if came == nil {
		t.Fatalf("no bench player scored the %d points the autosub added", incPts)
	}
	wantInc := xPointsOf(came, came.GWs[xpWeek])
	if closeEnough(wantInc, float64(incPts)) {
		t.Fatalf("the substitute's xPoints (%v) equals his points (%d); this "+
			"branch's fixture cannot distinguish them", wantInc, incPts)
	}
	if inc := sub.XPoints - bare; !closeEnough(inc, wantInc) {
		t.Errorf("the autosub added %v to xPoints, want the substitute's own "+
			"xPoints %v (his realised points were %d)", inc, wantInc, incPts)
	}
}

// TestTheXPointsSeasonTotalsAreBuiltLikeTheirPointsTwins is the season-level half
// of the pin, on a real archive.
//
// Three things can only go wrong above the week scorer, and none of them is
// visible in a constructed week:
//
//   - `SimResult.XPoints` not netting the hit cost. Points subtracts it, and a
//     metric that does not is answering a different question about the same
//     policy — a paired difference between the two would price transfers twice.
//   - the HOLD rungs' xPoints slices drifting out of alignment with their points
//     slices, which would make `hold_xpoints` a season of a different length.
//   - the instrument being a rename of the points column on a real season, which
//     is what a silently-inert residual would look like.
//
// Skips when the archive is unreachable, like every other test here that needs
// one, and enters late so it costs one short replay rather than a full season.
func TestTheXPointsSeasonTotalsAreBuiltLikeTheirPointsTwins(t *testing.T) {
	ctx := context.Background()
	cc := loadConfig(t)
	// 2023-24, and the season is chosen rather than convenient: at a GW21 entry it
	// is a cell where the weekly armband and the day-one one actually disagree
	// (8 weeks of 18), which check 3 below needs and 2024-25 does not provide —
	// the model captains the same player from GW21 to May there, so a rung fed the
	// wrong twin is byte-identical and the check cannot fail. It also carries
	// native xG, xA and xGC, which check 4 needs.
	cur, err := Load(ctx, cc.CacheDir, "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(ctx, cc.CacheDir, "2022-23")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	// ⚠️ **Deliberately not the shipped policy, and the reason is the hit cost.**
	// At shipped settings this cell takes no hits at all — checked at three entry
	// points — so the `- HitCost` half of the identity below would be `- 0` and
	// would pass under a build that had dropped the subtraction entirely. That is
	// the vacuous-pass shape this file's header is about, one level up.
	//
	// So the policy is loosened until hits actually happen: one free transfer
	// banked, no confidence charge on spending it, and a hit gate that accepts
	// anything. **No figure is read off this run** — it is a structural check on
	// how two totals are assembled, not a measurement, and nothing here is
	// comparable with any recorded number.
	cfg := SimConfig{
		Weights: config.Default().Weights, Budget: analysis.DefaultBudget,
		StartGW:  21,
		BankUpTo: 1, FreeCost: 0, MaxHits: 2, MinGainHit: -1000,
	}
	res, err := Simulate(cur, prior, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// 1. The season total is the weeks' gross, less the hit cost — the same
	//    arithmetic Points does, over the same weeks.
	var grossXP float64
	grossPts := 0
	for _, w := range res.Weeks {
		grossXP += w.GrossXP
		grossPts += w.Gross
	}
	if want := grossPts - res.HitCost; res.Points != want {
		t.Fatalf("Points %d is not the weeks' gross %d less hits %d — the "+
			"invariant this test compares xPoints against does not itself hold",
			res.Points, grossPts, res.HitCost)
	}
	if want := grossXP - float64(res.HitCost); !closeEnough(res.XPoints, want) {
		t.Errorf("XPoints %v is not the weeks' gross %v less the hit cost %d",
			res.XPoints, grossXP, res.HitCost)
	}
	// And the check above is only a check if the cell took hits. A zero hit cost
	// makes it `x - 0 == x`, which a build that never subtracted would also pass —
	// so this fails rather than reporting a pass the run could not have produced.
	if res.HitCost == 0 {
		t.Error("this cell took no hits, so the `- HitCost` half of the identity " +
			"above is vacuous; the policy loosening above has stopped working")
	}

	// 2. The HOLD rungs are the same length on both metrics, gameweek for
	//    gameweek. A shorter xPoints slice would silently shorten the season.
	hc := HoldCaptaincyWeekly(cur, prior, cfg, res.OpeningSquad)
	for _, rung := range []struct {
		name     string
		pts, xp  int
		replayed int
	}{
		{"Full", len(hc.Full), len(hc.FullXP), len(res.Weeks)},
		{"FixedCaptain", len(hc.FixedCaptain), len(hc.FixedCaptainXP), len(res.Weeks)},
		{"NoCaptain", len(hc.NoCaptain), len(hc.NoCaptainXP), len(res.Weeks)},
	} {
		if rung.pts != rung.xp || rung.pts != rung.replayed {
			t.Errorf("%s: %d points weeks, %d xPoints weeks, %d replayed weeks",
				rung.name, rung.pts, rung.xp, rung.replayed)
		}
	}

	// 3. Each xPoints rung is ITS OWN points rung's mirror.
	//
	// ⚠️ Written after a mutation survived: assigning `FixedCaptain`'s xPoints to
	// `FullXP` passed every other check in this file. Nothing above compares the
	// rungs to each other, so `hold_xpoints` could silently have been the
	// pinned-armband rung while `hold_points` was HOLD — two instruments of
	// different things reported as a pair, which is exactly the desynchronised
	// mirror this package keeps paying for.
	//
	// The exact form for the Full rung: HOLD minus HOLD-with-nobody-doubled is, by
	// construction, one player's return — whoever actually wore the armband, which
	// is the captain if he played and the vice if the fallback fired. Autosubs are
	// identical across the two rungs (captain id 0 changes no substitution), so the
	// difference is that player and nothing else. It must therefore be his POINTS
	// on the points rung and his XPOINTS on the xPoints rung.
	if len(hc.GW) != len(hc.Full) {
		t.Fatalf("the per-gameweek detail is %d long against %d rungs",
			len(hc.GW), len(hc.Full))
	}
	for i, gw := range hc.GW {
		var wantPts int
		var wantXP float64
		inXI := map[int]bool{}
		for _, id := range hc.XI[i] {
			inXI[id] = true
		}
		armband := 0
		if c := cur.Players[hc.Captain[i]]; c != nil && inXI[c.ID] && c.GWs[gw].Minutes > 0 {
			armband = c.ID
		} else if v := cur.Players[hc.Vice[i]]; viceCaptainFallback && v != nil &&
			inXI[v.ID] && v.GWs[gw].Minutes > 0 {
			armband = v.ID
		}
		if p := cur.Players[armband]; p != nil {
			wantPts = p.GWs[gw].Points
			wantXP = xPointsOf(p, p.GWs[gw])
		}
		if got := hc.Full[i] - hc.NoCaptain[i]; got != wantPts {
			t.Fatalf("GW%d: the armband is worth %d points on the points rungs, "+
				"want %d — this test's model of who is doubled is wrong, so the "+
				"xPoints check below would be checking the wrong thing", gw, got, wantPts)
		}
		if got := hc.FullXP[i] - hc.NoCaptainXP[i]; !closeEnough(got, wantXP) {
			t.Errorf("GW%d: the armband is worth %v on the xPoints rungs, want the "+
				"doubled player's own xPoints %v", gw, got, wantXP)
		}
	}
	// And the coarse mirror for the pair the identity above cannot reach: two
	// rungs that differ on points must differ on xPoints, or one of them has been
	// fed the other's weekly score.
	sameI := func(a, b []int) bool {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	sameF := func(a, b []float64) bool {
		for i := range a {
			if !closeEnough(a[i], b[i]) {
				return false
			}
		}
		return true
	}
	for _, m := range []struct {
		name   string
		pts    bool
		xp     bool
		reason string
	}{
		{"Full vs FixedCaptain", sameI(hc.Full, hc.FixedCaptain),
			sameF(hc.FullXP, hc.FixedCaptainXP),
			"the weekly armband against the day-one one"},
		{"Full vs NoCaptain", sameI(hc.Full, hc.NoCaptain),
			sameF(hc.FullXP, hc.NoCaptainXP), "doubling somebody against nobody"},
		{"FixedCaptain vs NoCaptain", sameI(hc.FixedCaptain, hc.NoCaptain),
			sameF(hc.FixedCaptainXP, hc.NoCaptainXP),
			"the pinned armband against nobody"},
	} {
		if m.pts != m.xp {
			t.Errorf("%s (%s): the points rungs are%s identical over the season and "+
				"the xPoints rungs are%s — one rung has been fed another's score",
				m.name, m.reason, notIf(m.pts), notIf(m.xp))
		}
		// A pair that is identical on BOTH metrics passes this check without
		// testing anything, which is how the Full-against-FixedCaptain swap
		// survived its first mutation run: at a 2024-25 GW21 entry the model
		// captains the same player every week, so the two rungs coincide and the
		// mirror is unfalsifiable. Said out loud rather than left to be
		// rediscovered.
		if m.pts && m.xp {
			t.Errorf("%s (%s): the two rungs coincide in every week of this cell, "+
				"so neither check above can fail — pick a cell where they diverge",
				m.name, m.reason)
		}
	}

	// 4. The instrument is not a rename. 2024-25 carries native xG, xA and xGC, so
	//    a total landing exactly on the points total means the residual is inert.
	holdXP := sumFloats(hc.FullXP)
	holdPts := float64(sumInts(hc.Full))
	if closeEnough(holdXP, holdPts) {
		t.Errorf("HOLD xPoints %v equals HOLD points %v on a season with native "+
			"underlying — the residual is inert", holdXP, holdPts)
	}
	if closeEnough(res.XPoints, float64(res.Points)) {
		t.Errorf("POLICY xPoints %v equals POLICY points %d", res.XPoints, res.Points)
	}
	t.Logf("GW%d entry, %d hits: HOLD %v points / %v xPoints, POLICY %d / %v",
		cfg.StartGW, res.Hits, holdPts, holdXP, res.Points, res.XPoints)
}
