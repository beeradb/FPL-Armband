package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// fakePriors stands in for a loaded season so the blend can be tested without
// the network.
type fakePriors map[int]*PriorPlayer

func (f fakePriors) Get(code int) (*PriorPlayer, bool) { p, ok := f[code]; return p, ok }

// TestBlendIsANoOpPreSeason — before GW1, FPL's aggregates *are* last season, so
// there is nothing to shrink toward and blending would double-count it.
func TestBlendIsANoOpPreSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	e.Priors = fakePriors{}

	var checked int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Minutes < 900 {
			continue
		}
		checked++
		if m := e.Metrics(el); m.PriorWeight != 1 {
			t.Fatalf("%s has prior weight %.3f pre-season, want 1", el.WebName, m.PriorWeight)
		}
	}
	if checked == 0 {
		t.Skip("no players with minutes")
	}
}

// TestBlendHoldsTheLineEarlyInTheSeason is the point of the exercise.
//
// After one gameweek FPL knows one match. An established starter who is rested
// for it must not collapse to fringe, and a squad player who happens to start
// must not be promoted to nailed. Last season is the only thing standing between
// the model and that noise.
func TestBlendHoldsTheLineEarlyInTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	// Last season he was an ever-present.
	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}

	playGameweeks(t, e, 2)
	el.Minutes, el.Starts = 30, 0 // two gameweeks, one brief cameo

	withPrior := e.Metrics(el)
	e.Priors = nil
	without := e.Metrics(el)

	t.Logf("after 2 GWs with 30 minutes: expected minutes %.1f with a prior, %.1f without",
		withPrior.ExpectedMinutes, without.ExpectedMinutes)

	if withPrior.ExpectedMinutes <= without.ExpectedMinutes {
		t.Error("the prior did not lift an established starter after a quiet start")
	}
	if withPrior.ExpectedMinutes < 40 {
		t.Errorf("an ever-present is down to %.1f expected minutes after two gameweeks; "+
			"the prior is not holding", withPrior.ExpectedMinutes)
	}
	if withPrior.PriorWeight >= 0.5 {
		t.Errorf("current-season weight is %.2f after two gameweeks, expected well under half",
			withPrior.PriorWeight)
	}
}

// TestBlendYieldsToTheSeason — the prior is a starting point, not an anchor. By
// the end of a season it must be almost entirely displaced, or a player who has
// genuinely lost his place would never be marked down.
func TestBlendYieldsToTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()
	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}

	var last float64
	for _, gw := range []int{2, 5, 10, 20, 30} {
		playGameweeks(t, e, gw)
		el.Minutes, el.Starts = gw*10, 0 // dropped: ten minutes a week all season
		m := e.Metrics(el)
		t.Logf("GW%-3d weight on this season %.2f, expected minutes %.1f", gw, m.PriorWeight, m.ExpectedMinutes)
		if m.PriorWeight < last {
			t.Errorf("current-season weight fell from %.2f to %.2f at GW%d", last, m.PriorWeight, gw)
		}
		last = m.PriorWeight
	}
	playGameweeks(t, e, 30)
	el.Minutes, el.Starts = 300, 0
	if m := e.Metrics(el); m.ExpectedMinutes > 30 {
		t.Errorf("after 30 gameweeks at 10 minutes a week he still shows %.1f expected minutes; "+
			"the prior is anchoring rather than informing", m.ExpectedMinutes)
	}
}

// TestPriorlessPlayersShrinkToTheLeague — a promoted club's player or an arrival
// from abroad has no prior of his own, and scoring him on his own first few
// matches is how the replay ended up paying a four-point hit for a debutant.
//
// His rates shrink toward his position's league-wide rates. His minutes do not:
// ninety minutes in one appearance really does mean ninety minutes when he
// plays, and the minutes-reliability term already prices whether he plays again.
func TestPriorlessPlayersShrinkToTheLeague(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts, bonus := el.Minutes, el.Starts, el.Bonus
	defer func() { el.Minutes, el.Starts, el.Bonus = mins, starts, bonus }()

	e.Priors = fakePriors{} // empty: nobody has a prior
	playGameweeks(t, e, 1)
	el.Minutes, el.Starts, el.Bonus = 90, 1, 3 // a debut, and three bonus points

	m := e.Metrics(el)
	if m.PriorWeight >= 0.5 {
		t.Errorf("one match carries weight %.2f; a debut is not half a season of evidence",
			m.PriorWeight)
	}
	// Raw, three bonus in ninety minutes reads as three bonus every gameweek.
	if m.Bonus90 > 1.0 {
		t.Errorf("bonus/90 is %.2f after a single three-bonus debut; the league rate "+
			"should have pulled it well under 1", m.Bonus90)
	}
	// Minutes are deliberately untouched by the shrinkage.
	if math.Abs(m.ExpectedMinutes-90) > 1 {
		t.Errorf("expected minutes %.1f, want ~90 — shrinkage must not touch minutes",
			m.ExpectedMinutes)
	}
}

// TestCountingStatsGoThroughTheBlend guards a bug that survived the first blend.
//
// Bonus, saves and cards were read straight off the element as count*90/minutes
// while xG, xA and defensive contributions were blended. A player with 22
// minutes and two bonus points therefore scored at 8.18 bonus a gameweek, and
// the replay's early transfers were driven almost entirely by that: modelled
// gains of +5.59 and +8.50 a gameweek, into players with one substitute
// appearance.
//
// Any new counting term must be blended the same way, or it reproduces this.
func TestCountingStatsGoThroughTheBlend(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	bonus, saves, yel, red := el.Bonus, el.Saves, el.YellowCards, el.RedCards
	defer func() {
		el.Minutes, el.Starts = mins, starts
		el.Bonus, el.Saves, el.YellowCards, el.RedCards = bonus, saves, yel, red
	}()

	// Last season: an ever-present with entirely ordinary counting stats.
	e.Priors = fakePriors{el.Code: {
		Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300,
		Bonus: 11, Saves: 0, Yellow: 4, Red: 0,
	}}
	playGameweeks(t, e, 2)
	el.Minutes, el.Starts = 22, 0 // one brief cameo across two gameweeks
	el.Bonus, el.Saves, el.YellowCards, el.RedCards = 2, 3, 1, 1

	m := e.Metrics(el)
	// 22 minutes is a quarter of a match, so unblended every one of these is
	// multiplied by roughly four.
	for _, c := range []struct {
		name string
		got  float64
		raw  float64
	}{
		{"bonus", m.Bonus90, 2 * 90.0 / 22},
		{"saves", m.Saves90, 3 * 90.0 / 22},
		{"yellow", m.Yellow90, 1 * 90.0 / 22},
		{"red", m.Red90, 1 * 90.0 / 22},
	} {
		if c.got > c.raw/2 {
			t.Errorf("%s/90 is %.2f against a raw %.2f; it is not going through the blend",
				c.name, c.got, c.raw)
		}
	}
	// And the effect that matters: the score must not explode off one cameo.
	if m.Score > 8 {
		t.Errorf("score %.2f off 22 minutes; the counting stats are still raw", m.Score)
	}
}

// TestCalibratedBlendConstants records where the defaults came from, so a change
// has to be a deliberate recalibration rather than a nudge.
func TestCalibratedBlendConstants(t *testing.T) {
	w := DefaultWeights()
	if w.BlendMinutesK != 5 {
		t.Errorf("BlendMinutesK is %v; 5 was measured against 2025-26 (MAE 18.74 mins/match, "+
			"15%% better than either extreme)", w.BlendMinutesK)
	}
	if w.BlendRateK != 8 {
		t.Errorf("BlendRateK is %v; 8 was measured with 2024-25 as prior and 2025-26 as outcome "+
			"across 218 players (MAE 0.0511 xG/90, 16%% better than ignoring last season)", w.BlendRateK)
	}
	if w.LeagueShrinkK != 8 {
		t.Errorf("LeagueShrinkK is %v; the out-of-sample MAE optimum (K=2) costs POLICY "+
			"-0.843/gw (t=-1.94) on the replay — a predictive win that loses on the argmax, "+
			"the same failure mode as rate recency. Stays at the shared 8.", w.LeagueShrinkK)
	}
}

// fakeRecent stands in for per-gameweek history.
type fakeRecent map[int]RecentPlayer

func (f fakeRecent) Get(code int) (RecentPlayer, bool) { p, ok := f[code]; return p, ok }

// TestRecentMinutesDisplaceTheSeasonAverage is the point of the Recent hook.
//
// Minutes are a statement about the present. A player who lost his place six
// weeks ago still reads as a starter on a season average, and a season average
// is all FPL's bootstrap publishes. Predicting the next five gameweeks' minutes
// across three replayed seasons, a half-life of 2 is 8.9% better than the flat
// total; wiring it into the replay won all three seasons, +31 on the mean.
func TestRecentMinutesDisplaceTheSeasonAverage(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}
	playGameweeks(t, e, 20)
	// Season to date says an ever-present: 20 full matches.
	el.Minutes, el.Starts = 1800, 20

	seasonAvg := e.Metrics(el)

	// Recently, though, he has stopped playing.
	e.Recent = fakeRecent{el.Code: {MinutesPerMatch: 5, StartShare: 0, Matches: 20}}
	recent := e.Metrics(el)

	t.Logf("expected minutes: %.1f on the season average, %.1f on recent form",
		seasonAvg.ExpectedMinutes, recent.ExpectedMinutes)
	if recent.ExpectedMinutes >= seasonAvg.ExpectedMinutes {
		t.Errorf("a player who has stopped playing reads %.1f minutes against %.1f on the "+
			"season average; the recency hook is not applied",
			recent.ExpectedMinutes, seasonAvg.ExpectedMinutes)
	}
	if recent.Score >= seasonAvg.Score {
		t.Error("the score did not follow the minutes down")
	}
}

// TestRecentIsIgnoredPreSeason — before a ball is kicked there is no current
// season to be recent about, and FPL's totals are still last season's.
func TestRecentIsIgnoredPreSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	before := e.Metrics(el)
	e.Recent = fakeRecent{el.Code: {MinutesPerMatch: 0, StartShare: 0, Matches: 20}}
	if after := e.Metrics(el); after.ExpectedMinutes != before.ExpectedMinutes {
		t.Errorf("pre-season minutes moved from %.1f to %.1f; recency must not apply",
			before.ExpectedMinutes, after.ExpectedMinutes)
	}
}

// TestRecencyAppliesToMinutesOnly records the measurement that split them.
//
// The same out-of-sample test run on points and on xG+xA says sharp recency is
// actively worse — "last 3 games" is 19% worse than the season average on both,
// because underlying quality is stable and a short window chases finishing
// variance. Only minutes are weighted, and a future term must be measured
// before it joins them.
func TestRecencyAppliesToMinutesOnly(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	e.Priors = fakePriors{el.Code: {Minutes: 3300, Starts: 37, XG: 8, XA: 6, XGC: 40, DefCon: 300}}
	playGameweeks(t, e, 20)
	el.Minutes, el.Starts = 1800, 20

	base := e.Metrics(el)
	// Recent form that leaves minutes untouched must leave the rates untouched
	// too — the hook carries no rate information at all.
	e.Recent = fakeRecent{el.Code: {
		MinutesPerMatch: base.ExpectedMinutes, StartShare: 1, Matches: 20,
	}}
	got := e.Metrics(el)
	for _, c := range []struct {
		name     string
		was, now float64
	}{
		{"xG/90", base.XG90, got.XG90},
		{"xA/90", base.XA90, got.XA90},
		{"bonus/90", base.Bonus90, got.Bonus90},
		{"defcon/90", base.DefCon90, got.DefCon90},
	} {
		if math.Abs(c.was-c.now) > 1e-9 {
			t.Errorf("%s moved from %.4f to %.4f; recency must touch minutes only",
				c.name, c.was, c.now)
		}
	}
}

// TestMinutesOverrideBeatsTheData is the mechanism that should be reached for
// before any lock.
//
// Isak scored 0.49 pts/gw because a leg break held him to 694 minutes — the
// number is an artefact, not a judgement about his role. Correcting it lets the
// model recompute and answer the question it is good at, which is whether that
// is worth his price. A lock forces him in and never asks.
func TestMinutesOverrideBeatsTheData(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	mins, starts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = mins, starts }()

	playGameweeks(t, e, 10)
	el.Minutes, el.Starts = 90, 1 // one appearance in ten gameweeks

	fringe := e.Metrics(el)
	if fringe.ExpectedMinutes > 30 {
		t.Skipf("fixture gives %.1f expected minutes; too high to test", fringe.ExpectedMinutes)
	}

	e.MinutesOverride = map[int]float64{el.Code: 90}
	fixed := e.Metrics(el)

	t.Logf("%s: %.2f pts/gw on %.1f mins -> %.2f on %.1f after the correction",
		el.WebName, fringe.Score, fringe.ExpectedMinutes, fixed.Score, fixed.ExpectedMinutes)

	if fixed.ExpectedMinutes < 85 {
		t.Errorf("expected minutes %.1f after an override of 90", fixed.ExpectedMinutes)
	}
	if fixed.Score <= fringe.Score {
		t.Errorf("score did not rise with the minutes: %.2f -> %.2f", fringe.Score, fixed.Score)
	}
	// The correction must not touch his rates — it says how much he plays, not
	// how good he is.
	if math.Abs(fixed.XG90-fringe.XG90) > 1e-9 || math.Abs(fixed.XA90-fringe.XA90) > 1e-9 {
		t.Error("a minutes correction changed the per-90 rates")
	}
}

// TestMinutesOverrideToZeroSuppresses — setting a player to zero is how the
// analysis layer says "he is out", and it should make him unpickable without
// needing the certainty a hard exclusion implies.
func TestMinutesOverrideToZeroSuppresses(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	el := findEverPresent(t, e)
	playGameweeks(t, e, 10)

	before := e.Metrics(el)
	e.MinutesOverride = map[int]float64{el.Code: 0}
	after := e.Metrics(el)

	t.Logf("%s: %.2f -> %.2f pts/gw when set to zero minutes", el.WebName, before.Score, after.Score)
	if after.Score >= before.Score*0.25 {
		t.Errorf("a player set to zero minutes still scores %.2f against %.2f; he would "+
			"still be picked", after.Score, before.Score)
	}
}

// TestMinutesOverrideIsProratedByReturnDate — "out until GW12" is a claim about
// particular gameweeks, not about every week the horizon averages over.
//
// Applied flat, a short override on a five-gameweek horizon says the absence
// lasts as long as the model happens to be looking. Prorated, a player back
// inside the window carries his own minutes for the weeks he is available,
// which is what makes an expected return date usable through the mechanism that
// already works instead of through an exclusion.
func TestMinutesOverrideIsProratedByReturnDate(t *testing.T) {
	e := testEngine(t)
	next := e.Boot.NextEvent()
	if next == nil {
		t.Skip("no next gameweek")
	}
	// An established player, so the natural estimate is well clear of zero and
	// the proration has something to show.
	var el *fpl.Element
	for i := range e.Boot.Elements {
		if e.Boot.Elements[i].Minutes > 2500 {
			el = &e.Boot.Elements[i]
			break
		}
	}
	if el == nil {
		t.Skip("no established player in the pool")
	}
	natural := e.Metrics(el).ExpectedMinutes
	if natural <= 10 {
		t.Skipf("natural minutes %.1f too low to test", natural)
	}

	e.Weights.Horizon = 5
	e.MinutesOverride = map[int]float64{el.Code: 0}

	// Indefinite: applies flat, so the player is worth nothing across the board.
	e.MinutesOverrideUntil = nil
	if got := e.prorateOverride(el.Code, 0, natural); got != 0 {
		t.Errorf("indefinite override prorated to %.2f, want 0", got)
	}

	// Out for the whole horizon: still flat.
	e.MinutesOverrideUntil = map[int]int{el.Code: next.ID + 4}
	if got := e.prorateOverride(el.Code, 0, natural); got != 0 {
		t.Errorf("override covering the horizon gave %.2f, want 0", got)
	}

	// Back after two of the five: three weeks at his own minutes.
	e.MinutesOverrideUntil = map[int]int{el.Code: next.ID + 1}
	got := e.prorateOverride(el.Code, 0, natural)
	want := 3 * natural / 5
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("prorated to %.3f, want %.3f (natural %.1f)", got, want, natural)
	}
	if !(got > 0 && got < natural) {
		t.Errorf("prorated %.3f should sit strictly between 0 and %.3f", got, natural)
	}

	// The imminent-week view has a horizon of one, so a player out this week is
	// out, with no averaging to soften it.
	e.Weights.Horizon = 1
	if got := e.prorateOverride(el.Code, 0, natural); got != 0 {
		t.Errorf("horizon-1 gave %.2f for a player out this week, want 0", got)
	}
}
