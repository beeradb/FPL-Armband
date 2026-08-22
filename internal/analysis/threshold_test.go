package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// TestThresholdPointsAreNotProrated is the fix's whole point.
//
// FPL pays appearance points and the clean sheet as a step at sixty minutes: a
// starter taken off at seventy banks both in full. The model multiplied the
// entire per-90 estimate by minutes reliability, crediting him about 0.73 of
// each — and those two terms are 61% of a defender's per-90 score.
func TestThresholdPointsAreNotProrated(t *testing.T) {
	if !thresholdSplit {
		t.Skip("FPL_NO_THRESHOLD_SPLIT is set")
	}
	// A player who averages 70 minutes reaches sixty far more often than his
	// minutes share suggests, which is exactly the gap that was being lost.
	share := math.Pow(70.0/90.0, DefaultWeights().MinutesWeight)
	if got := playsSixty(70); !(got > share) {
		t.Errorf("P(60+) at 70 mean minutes is %.3f, no better than the %.3f the "+
			"minutes rating credited", got, share)
	}
	// Anchors from the fit, which the archive measured at 0.470 / 0.716 / 0.949.
	for _, c := range []struct{ mins, want, tol float64 }{
		{45, 0.470, 0.05}, {65, 0.716, 0.06}, {85, 0.949, 0.06},
	} {
		if got := playsSixty(c.mins); math.Abs(got-c.want) > c.tol {
			t.Errorf("P(60+) at %.0f minutes is %.3f, measured %.3f", c.mins, got, c.want)
		}
	}
}

// TestPlaysSixtyIsAProbability — monotone, bounded, and zero at zero.
//
// The zero matters. A logistic alone reads 0.045 at a mean of one minute, so a
// player who has never played would collect a slice of appearance points every
// week forever, and the "no Premier League data scores 0.00" property that
// research_targets is built on would quietly stop holding.
func TestPlaysSixtyIsAProbability(t *testing.T) {
	if got := playsSixty(0); got != 0 {
		t.Errorf("a player with no minutes reaches sixty with probability %v", got)
	}
	prev := -1.0
	for m := 0.0; m <= 90.0; m += 1 {
		p := playsSixty(m)
		if p < 0 || p > 1 {
			t.Fatalf("P(60+) at %.0f minutes is %v", m, p)
		}
		if p < prev {
			t.Fatalf("P(60+) fell from %.4f to %.4f at %.0f minutes", prev, p, m)
		}
		if m <= 60 && p > m/60+1e-9 {
			t.Fatalf("P(60+) at %.0f minutes is %.4f, above the Markov bound %.4f",
				m, p, m/60)
		}
		prev = p
	}
}

// TestThresholdSplitLeavesAnEverPresentAlone — the fix must move part-timers,
// not everybody. An ever-present already banks both terms every week, so his
// score should be near identical either way; if the split moves him too, it is
// rescaling the model rather than correcting a shape.
func TestThresholdSplitLeavesAnEverPresentAlone(t *testing.T) {
	e := testEngine(t)
	var nailed, partial *fpl.Element
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		m := e.Metrics(el)
		if m.ExpectedMinutes >= 85 && nailed == nil {
			nailed = el
		}
		if m.ExpectedMinutes > 60 && m.ExpectedMinutes < 72 && partial == nil {
			partial = el
		}
	}
	if nailed == nil || partial == nil {
		t.Skip("no suitable players in the current pool")
	}
	ratio := func(el *fpl.Element) float64 {
		m := e.Metrics(el)
		thr := e.thresholdXP90(el, m, m.Fixtures)
		rate := m.FixtureAdjXP90 - thr
		split := rate*m.MinutesRating + thr*playsSixty(m.ExpectedMinutes)
		old := m.FixtureAdjXP90 * m.MinutesRating
		if old <= 0 {
			return 1
		}
		return split / old
	}
	rn, rp := ratio(nailed), ratio(partial)
	t.Logf("%s (%.0f mins) %.3f, %s (%.0f mins) %.3f",
		nailed.WebName, e.Metrics(nailed).ExpectedMinutes, rn,
		partial.WebName, e.Metrics(partial).ExpectedMinutes, rp)
	if math.Abs(rn-1) > 0.06 {
		t.Errorf("an ever-present moves by %.1f%%; the split should barely touch him",
			100*(rn-1))
	}
	if !(rp > rn) {
		t.Errorf("the substituted starter moves %.3f against the ever-present's %.3f; "+
			"the correction is supposed to favour him", rp, rn)
	}
}

// TestPlaysSixtyRespectsBothBounds — the fit is held between two exact bounds.
//
// A logistic is wrong at both ends. The upper bound is Markov; the lower comes
// from minutes never exceeding ninety, so m <= 60(1-p) + 90p. At a mean of
// ninety the lower bound forces 1, which is the only right answer for a player
// on the pitch for every minute — the bare logistic saturates near 0.94 and
// would dock him for it.
func TestPlaysSixtyRespectsBothBounds(t *testing.T) {
	if got := playsSixty(90); got != 1 {
		t.Errorf("a player who never leaves the pitch reaches sixty %v of the time", got)
	}
	for m := 0.0; m <= 90.0; m += 0.5 {
		p := playsSixty(m)
		if m <= 60 && p > m/60+1e-9 {
			t.Fatalf("P(60+) at %.1f is %.4f, above Markov's %.4f", m, p, m/60)
		}
		if m >= 60 && p < (m-60)/30-1e-9 {
			t.Fatalf("P(60+) at %.1f is %.4f, below the %.4f a ninety-minute cap forces",
				m, p, (m-60)/30)
		}
	}
}

// TestBonusScheduleTracksEvidence — the bonus weight must depend on whether the
// rate it scales is this season's or last season's.
//
// Measured on the held opening fifteen across four seasons, the term is
// monotone harmful from a GW1 start and monotone helpful from GW11 and GW21. A
// constant cannot be right in both, and the aggregate hides it by averaging two
// opposite trends.
func TestBonusScheduleTracksEvidence(t *testing.T) {
	e := testEngine(t)
	w := e.Weights
	w.BonusPriorWeight, w.BonusWeight = 0.5, 2.0
	e.Weights = w

	var el *fpl.Element
	for i := range e.Boot.Elements {
		if e.Boot.Elements[i].Minutes > 2000 {
			el = &e.Boot.Elements[i]
			break
		}
	}
	if el == nil {
		t.Skip("no established player in the pool")
	}

	// Pre-season the rate is entirely last season's, so the prior end applies.
	if !e.SeasonHasStarted() {
		if got := e.bonusWeightFor(el); got != 0.5 {
			t.Errorf("pre-season bonus weight %v, want the prior end 0.5 — the rate is "+
				"entirely last season's there", got)
		}
	}
	// Evidence is bounded and rises with minutes played.
	for _, mins := range []int{0, 900, 3000} {
		probe := *el
		probe.Minutes = mins
		if v := e.bonusEvidence(&probe); v < 0 || v > 1 {
			t.Errorf("evidence share %v at %d minutes is not a share", v, mins)
		}
	}
	// A negative prior end disables the schedule entirely, which is how the
	// pre-measurement behaviour is restored.
	w.BonusPriorWeight = -1
	e.Weights = w
	if got := e.bonusWeightFor(el); got != w.BonusWeight {
		t.Errorf("disabled schedule gives %v, want the flat %v", got, w.BonusWeight)
	}
}

// TestTheBonusScheduleReadsAPlayedMatchDuringTheGap is the liveness half of the
// fifth instance of this record's GameweeksPlayed()-vs-SeasonHasStarted()
// defect family, found the same day PR #44 fixed the fourth: bonusEvidence
// gated on GameweeksPlayed()==0, which stays 0 for the whole multi-day span
// between a gameweek's first kickoff and its last final whistle — pinning
// EVERY player's bonus weight to the pure-prior end regardless of how much
// football his own club had actually played. Unlike the minutes floor
// (Class B, PR #44), this needed no per-club signal: el.Minutes already IS
// the player's own evidence count, so the fix is the provenance gate alone
// (Class C, see inLiveGameweekGap's comment).
func TestTheBonusScheduleReadsAPlayedMatchDuringTheGap(t *testing.T) {
	e := floorWindowEngine(t)
	established := &fpl.Element{Minutes: 90}

	if got := e.bonusEvidence(established); got <= 0 {
		t.Errorf("bonusEvidence = %v for a player 90 minutes into his own club's "+
			"completed match; want > 0 — a real match already on the board must not "+
			"read as no evidence just because no gameweek has FINISHED yet", got)
	}
}

// TestTheBonusScheduleIsUnchangedOutsideTheGap is the confinement pair to the
// liveness test above. It passes with the fix and without it, on purpose — a
// confinement check on a path the change cannot reach can only ever fail, so
// it confirms nothing on its own; TestTheBonusScheduleReadsAPlayedMatchDuringTheGap
// is the one that must move, and does.
func TestTheBonusScheduleIsUnchangedOutsideTheGap(t *testing.T) {
	e := floorWindowEngine(t)
	established := &fpl.Element{Minutes: 90}

	// Pre-season: nothing has kicked off anywhere.
	for i := range e.Fixtures {
		e.Fixtures[i].Started = false
		e.Fixtures[i].Finished = false
		e.Fixtures[i].FinishedProvisional = false
	}
	if got := e.bonusEvidence(established); got != 0 {
		t.Errorf("pre-season bonusEvidence = %v, want 0 — the rate is entirely last "+
			"season's before a ball is kicked anywhere", got)
	}

	// A gameweek has finished: DataWindow is honest again for everybody, and the
	// gate must not still be forcing 0.
	for i := range e.Fixtures {
		e.Fixtures[i].Started = true
		e.Fixtures[i].Finished = true
		e.Fixtures[i].FinishedProvisional = true
	}
	e.Boot.Events[0].Finished = true
	if e.GameweeksPlayed() != 1 {
		t.Fatalf("setup: GameweeksPlayed = %d, want 1", e.GameweeksPlayed())
	}
	want := 1.0 / (1.0 + e.Weights.BlendRateK)
	if got := e.bonusEvidence(established); math.Abs(got-want) > 1e-9 {
		t.Errorf("post-gap bonusEvidence = %v, want the raw n90/(n90+k) share %v — "+
			"the fix must not keep gating once GameweeksPlayed is honest again", got, want)
	}
}

// TestTheBonusScheduleIsZeroForAnUnplayedClub confirms the fix works by
// arithmetic, not by a second gate: a player at a club that has not kicked off
// has el.Minutes == 0 (FPL zeroes the whole league's aggregates the moment the
// season starts, not per club), so n90 == 0 and the schedule lands on the
// prior end regardless of SeasonHasStarted being true.
func TestTheBonusScheduleIsZeroForAnUnplayedClub(t *testing.T) {
	e := floorWindowEngine(t)
	debutant := &fpl.Element{Minutes: 0}

	if got := e.bonusEvidence(debutant); got != 0 {
		t.Errorf("bonusEvidence = %v for a player with no minutes on the board yet, "+
			"want 0 — no per-club gate is needed here, only el.Minutes itself", got)
	}
}

// TestInLiveGameweekGap pins the shared predicate directly, independent of any
// one consumer, so a future change to it is caught at the source.
func TestInLiveGameweekGap(t *testing.T) {
	e := floorWindowEngine(t)
	if !e.inLiveGameweekGap() {
		t.Fatal("floorWindowEngine's own setup is SeasonHasStarted=true, " +
			"GameweeksPlayed=0 — inLiveGameweekGap must read true here")
	}

	for i := range e.Fixtures {
		e.Fixtures[i].Started = false
	}
	if e.inLiveGameweekGap() {
		t.Error("pre-season (nothing started) must not read as the gap")
	}

	for i := range e.Fixtures {
		e.Fixtures[i].Started = true
	}
	for i := range e.Boot.Events {
		e.Boot.Events[i].Finished = true
	}
	if e.inLiveGameweekGap() {
		t.Error("once a gameweek has finished, this must not still read as the gap")
	}
}

// TestDefconCouplesToTheCleanSheet — a defender's own defensive workload is
// evidence about his clean sheet, and the model used to treat them as
// independent.
//
// Measured on 2025-26 the model predicted the *same* clean-sheet value for
// every defcon group — 1.016, 0.987, 1.064 per 90 from the lowest third to the
// highest — while what they collected was 1.046, 1.059 and 0.825. A defender
// clearing ten times a match is on a side under pressure and his team's xGC
// does not fully know it.
func TestDefconCouplesToTheCleanSheet(t *testing.T) {
	e := testEngine(t)
	w := e.Weights
	w.DefConCleanCoupling = 0.3
	e.Weights = w

	busy := e.defconCleanFactor(2, 12.0)
	quiet := e.defconCleanFactor(2, 4.0)
	if !(busy > 1 && quiet < 1) {
		t.Errorf("busy defender factor %.3f, quiet %.3f; a busy one should raise the "+
			"expected goals conceded his clean sheet is computed from", busy, quiet)
	}
	// Only defenders: a midfielder's defcon is 59% recoveries, which happen all
	// over the pitch and carry no implication about his side being pinned back.
	if got := e.defconCleanFactor(3, 12.0); got != 1 {
		t.Errorf("midfielder factor %.3f, want 1", got)
	}
	// Clamped, because it is fitted on 87 defenders in the one season that has
	// the category and must not run away on an outlier.
	if v := e.defconCleanFactor(2, 100); v > 1.35001 {
		t.Errorf("an extreme rate produced %.3f, above the clamp", v)
	}
	// Disabled by default-zero.
	w.DefConCleanCoupling = 0
	e.Weights = w
	if got := e.defconCleanFactor(2, 12.0); got != 1 {
		t.Errorf("coupling disabled still returns %.3f", got)
	}
}

// TestSavesAreFixtureSensitive — a keeper facing a strong attack makes more
// saves, and the model used to price only the losing half of that trade.
//
// It already raised his expected goals conceded for a hard fixture, correctly
// costing him clean-sheet value, and carried his saves across untouched.
// Measured within-keeper, saves against a given opponent run 1.46 to 0.75 — a
// factor of 1.96, against the defensive ladder's own 0.70-to-1.40, which is 2.0.
// The same multiplier drives both, which is why they now share it.
func TestSavesAreFixtureSensitive(t *testing.T) {
	if !savesFixtureAdjust {
		t.Skip("FPL_NO_SAVES_FIXTURE is set")
	}
	e := testEngine(t)
	var gk *fpl.Element
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.ElementType == 1 && el.Minutes > 2000 && el.Saves > 50 {
			gk = el
			break
		}
	}
	if gk == nil {
		t.Skip("no established keeper in the pool")
	}
	m := e.Metrics(gk)
	if m.Saves90 <= 0 {
		t.Skip("no save rate")
	}
	// The saves term must move with the opponent, in the same direction as the
	// goals conceded it comes from.
	easy := poissonFloorDiv(savesBlock, m.Saves90*0.7)
	hard := poissonFloorDiv(savesBlock, m.Saves90*1.4)
	if !(hard > easy) {
		t.Errorf("saves against a strong attack score %.3f against %.3f for a weak one",
			hard, easy)
	}
	// And saves must sit inside the fixture-sensitive part, not in the remainder
	// carried across unchanged — otherwise they are counted once at the adjusted
	// rate and again at the flat one. Checked by zeroing the save rate and seeing
	// the whole save term disappear from the neutral-difficulty value.
	part := e.fixtureSensitiveAt(m, 1, 1, 1)
	if part <= 0 {
		t.Fatalf("fixture-sensitive part is %.3f", part)
	}
	noSaves := m
	noSaves.Saves90 = 0
	got := part - e.fixtureSensitiveAt(noSaves, 1, 1, 1)
	want := poissonFloorDiv(savesBlock, m.Saves90)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("the fixture-sensitive part carries %.4f of save points, want %.4f", got, want)
	}
}
