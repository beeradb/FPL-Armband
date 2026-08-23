package backtest

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// TestPointInTimeCannotSeeTheFuture is the load-bearing test of the whole
// simulation. If the reconstructed view leaks even one gameweek of hindsight
// every result becomes meaningless while still looking entirely plausible.
func TestPointInTimeCannotSeeTheFuture(t *testing.T) {
	cur, prior := mkSeason(), mkSeason()
	// Make the future unmistakable: from GW11 everyone explodes.
	for _, p := range cur.Players {
		for gw := 11; gw <= 38; gw++ {
			p.GWs[gw] = GW{Points: 1000, Minutes: 90, Value: 999, Goals: 50, XG: 50}
		}
	}

	boot, _ := PointInTime(cur, prior, 10)
	for _, el := range boot.Elements {
		if el.GoalsScored >= 50 {
			t.Fatalf("%s carries %d goals at GW10; the future has leaked in",
				el.WebName, el.GoalsScored)
		}
		if el.NowCost == 999 {
			t.Fatalf("%s is priced from a later gameweek", el.WebName)
		}
		if float64(el.ExpectedGoals) >= 50 {
			t.Fatalf("%s carries future xG", el.WebName)
		}
	}
}

func TestPointInTimeAccumulatesExactly(t *testing.T) {
	cur, prior := mkSeason(), mkSeason()
	boot, _ := PointInTime(cur, prior, 10)

	byID := map[int]int{}
	for _, el := range boot.Elements {
		byID[el.ID] = el.Minutes
	}
	for id, p := range cur.Players {
		want := 0
		for gw := 1; gw <= 10; gw++ {
			want += p.GWs[gw].Minutes
		}
		if got := byID[id]; got != want {
			t.Errorf("player %d has %d minutes through GW10, want %d", id, got, want)
		}
	}
}

// TestPointInTimeMarksGameweeksFinished — the data window depends on it. If the
// events are not marked, every player divides by 38 and lands in the "fringe"
// band, which is the bug the DataWindow fix addressed.
func TestPointInTimeMarksGameweeksFinished(t *testing.T) {
	cur, prior := mkSeason(), mkSeason()
	boot, _ := PointInTime(cur, prior, 7)
	finished := 0
	for _, ev := range boot.Events {
		if ev.Finished {
			finished++
		}
	}
	if finished != 7 {
		t.Errorf("%d gameweeks marked finished, want 7", finished)
	}

	e := analysis.NewEngineFull(boot, cur.Fixtures, analysis.DefaultWeights(),
		analysis.Congestion{}, analysis.RoleRisk{})
	if got := e.DataWindow(); got != 7 {
		t.Errorf("engine data window %d, want 7", got)
	}
	// An ever-present should read ~90 minutes a gameweek, not 90*7/38.
	el := boot.Elements[0]
	if m := e.Metrics(&el); m.ExpectedMinutes < 80 || m.ExpectedMinutes > 95 {
		t.Errorf("expected minutes %.1f at GW7, want ~90", m.ExpectedMinutes)
	}
}

func TestPointInTimeZeroIsPreSeason(t *testing.T) {
	cur, prior := mkSeason(), mkSeason()
	for _, p := range prior.Players {
		p.Minutes = 1234
	}
	boot, _ := PointInTime(cur, prior, 0)
	for _, el := range boot.Elements {
		if el.Minutes != 1234 && el.Minutes != 0 {
			t.Fatalf("%s has %d minutes pre-season, want last season's 1234", el.WebName, el.Minutes)
		}
	}
}

func TestPriceAtTracksTheWeek(t *testing.T) {
	p := &Player{NowCost: 50, GWs: map[int]GW{
		1: {Value: 50}, 2: {Value: 51}, 3: {Value: 53},
	}}
	for gw, want := range map[int]int{1: 50, 2: 51, 3: 53, 10: 53} {
		if got := priceAt(p, gw); got != want {
			t.Errorf("priceAt(GW%d) = %d, want %d", gw, got, want)
		}
	}
}

// TestPriorIndexResolvesByCode — element ids are reassigned every summer, so a
// replay that joined on them would silently pair unrelated players.
func TestPriorIndexResolvesByCode(t *testing.T) {
	prior := mkSeason()
	for _, p := range prior.Players {
		p.Minutes = 2000
		p.ID += 500 // ids differ across seasons; codes do not
	}
	idx := newPriorIndex(prior)
	if q, ok := idx.Get(1001); !ok || q.Minutes != 2000 {
		t.Errorf("code lookup failed: %+v ok=%v", q, ok)
	}
	if _, ok := idx.Get(999999); ok {
		t.Error("unknown code resolved to a player")
	}
}

// TestCappingToOneTransferMakesHitsUnreachable records why the cap exists and
// what it costs.
//
// The original policy made at most one transfer a week while free transfers
// accrued at one a week, so the bank never emptied, the hit branch was dead
// code, and all three replayed seasons took zero hits. That looked like evidence
// that hits are not worth taking. It was not evidence of anything.
func TestCappingToOneTransferMakesHitsUnreachable(t *testing.T) {
	free, bankUpTo := 1, 2
	for week := 0; week < 38; week++ {
		if free < bankUpTo {
			free++
		}
		// Capped at one move a week, the limit is always spent on a free
		// transfer and never reaches the hit allowance.
		if moveLimit(free, 1, 1, 0) != 1 || free == 0 {
			t.Fatalf("week %d: free=%d — the cap should keep the bank topped up", week, free)
		}
		free--
	}
	// Uncapped, the same accrual empties the bank and a hit becomes available.
	free = 2
	if moveLimit(free, 1, 0, 0) != 3 {
		t.Error("uncapped, two free transfers plus a hit allowance should permit three moves")
	}
}

// TestEffectiveHorizonShrinksLateOn — a -4 taken in May has almost no season
// left to repay it, and a policy using a fixed five-gameweek horizon would keep
// chasing form to the final whistle.
func TestEffectiveHorizonShrinksLateOn(t *testing.T) {
	for _, c := range []struct {
		gw   int
		want float64
	}{{1, 5}, {20, 5}, {34, 5}, {35, 4}, {36, 3}, {37, 2}, {38, 1}} {
		if got := effectiveHorizon(5, c.gw); got != c.want {
			t.Errorf("effectiveHorizon(5, GW%d) = %.0f, want %.0f", c.gw, got, c.want)
		}
	}
	if got := effectiveHorizon(5, 45); got != 1 {
		t.Errorf("past the end of the season the horizon is %.0f, want 1", got)
	}
}

// TestMoveLimitAllowsHitsOnlyBeyondTheFreeTransfers — the previous policy made
// at most one transfer a week while free transfers accrued at one a week, so the
// bank never emptied and the hit branch was dead code.
func TestMoveLimitAllowsHitsOnlyBeyondTheFreeTransfers(t *testing.T) {
	for _, c := range []struct {
		free, hits, cap, want int
	}{
		{2, 1, 0, 3}, // two free plus one hit
		{2, 0, 0, 2}, // hits disabled
		{1, 2, 0, 2}, // a second hit is never offered, whatever the config says
		{2, 1, 1, 1}, // capped at one, the old behaviour
		{0, 1, 0, 1}, // no free transfers left: a hit is still reachable
	} {
		if got := moveLimit(c.free, c.hits, c.cap, 0); got != c.want {
			t.Errorf("moveLimit(free=%d hits=%d cap=%d) = %d, want %d",
				c.free, c.hits, c.cap, got, c.want)
		}
	}
	if moveLimit(0, 0, 0, 0) != 0 {
		t.Error("with no free transfers and no hit allowance the policy must do nothing")
	}
	// The limit is free+1 however many transfers are banked, so the modern
	// five-transfer bank offers six moves and never a second hit.
	if got := moveLimit(5, 2, 0, 0); got != 6 {
		t.Errorf("moveLimit(free=5) = %d, want 6 — five free transfers plus a single hit", got)
	}
}

// TestHitPaysForItselfOverTheHorizon pins the arithmetic a -4 has to clear. A
// hit is four points now against a gain spread over the weeks remaining, so a
// gain that justifies one in September need not justify one in May.
func TestHitPaysForItselfOverTheHorizon(t *testing.T) {
	const minGainHit = 3.0
	worth := func(gain float64, gw int) bool {
		return gain*effectiveHorizon(5, gw)-4 >= minGainHit
	}
	if !worth(1.5, 10) {
		t.Error("1.5/gw over five gameweeks is +3.5 net and should justify a hit")
	}
	if worth(1.0, 10) {
		t.Error("1.0/gw over five gameweeks is +1.0 net and should not justify a hit")
	}
	if worth(1.5, 37) {
		t.Error("the same 1.5/gw gain at GW37 is -1.0 net and must not justify a hit")
	}
}

// TestJudgeScoresOverTheDecisionHorizon — a move made at GW8 is judged on the
// five gameweeks it was justified on, not the rest of the season. The policy
// re-decides every week, so crediting a transfer with points scored in May
// would attribute thirty later decisions to one.
func TestJudgeScoresOverTheDecisionHorizon(t *testing.T) {
	s := mkSeason()
	// Player 8 scores 5 a week, player 13 scores 6. A swap of one for the other
	// is worth exactly one point a gameweek.
	v := Judge(s, []Move{{GW: 8, OutID: 8, InID: 13}}, 5)
	if len(v) != 1 {
		t.Fatalf("judged %d moves, want 1", len(v))
	}
	if v[0].Weeks != 5 {
		t.Errorf("horizon %d gameweeks at GW8, want 5", v[0].Weeks)
	}
	if v[0].InPoints != 30 || v[0].OutPoints != 25 {
		t.Errorf("in %d out %d over five gameweeks, want 30 and 25",
			v[0].InPoints, v[0].OutPoints)
	}
	if v[0].Net() != 5 {
		t.Errorf("net %d, want 5", v[0].Net())
	}
}

// TestJudgeChargesTheHit — a -4 has to clear the four points before it counts as
// a gain, or every hit looks free in the report.
func TestJudgeChargesTheHit(t *testing.T) {
	s := mkSeason()
	v := Judge(s, []Move{{GW: 8, OutID: 8, InID: 13, Hit: true}}, 5)
	if v[0].Net() != 1 {
		t.Errorf("net %d for a +5 swap taken on a hit, want 1", v[0].Net())
	}
}

// TestJudgeTruncatesAtTheEndOfTheSeason — a transfer at GW37 has two gameweeks
// left, and reading past GW38 would silently score it as zero.
func TestJudgeTruncatesAtTheEndOfTheSeason(t *testing.T) {
	s := mkSeason()
	v := Judge(s, []Move{{GW: 37, OutID: 8, InID: 13}}, 5)
	if v[0].Weeks != 2 {
		t.Errorf("horizon %d at GW37, want 2", v[0].Weeks)
	}
	if v[0].InPoints != 12 {
		t.Errorf("in %d points over GW37-38, want 12", v[0].InPoints)
	}
}

// TestJudgeToleratesAMissingPlayer — an unknown id must score zero rather than
// panic, since a move can name a player the season data does not carry.
func TestJudgeToleratesAMissingPlayer(t *testing.T) {
	s := mkSeason()
	v := Judge(s, []Move{{GW: 1, OutID: 99999, InID: 13}}, 5)
	if v[0].OutPoints != 0 {
		t.Errorf("unknown player scored %d, want 0", v[0].OutPoints)
	}
}

// TestJudgeFlagsASoldPlayerWhoNeverAppears guards the honesty of the report.
//
// When the sold player does not play again, Net() credits the transfer with his
// zero as though the eleven would have carried it. It would not — an autosub
// covers him for free. Across three replayed seasons that is 19% of transfers,
// and it is most of the apparent gap between modelled and actual gains, so the
// flag has to survive.
func TestJudgeFlagsASoldPlayerWhoNeverAppears(t *testing.T) {
	s := mkSeason()
	// Player 8 plays every week; blank player 2 is the reserve keeper who never
	// appears at all.
	v := Judge(s, []Move{{GW: 8, OutID: 8, InID: 13}}, 5)
	if !v[0].OutPlayed {
		t.Error("an ever-present is marked as never appearing")
	}
	v = Judge(s, []Move{{GW: 8, OutID: 2, InID: 13}}, 5)
	if v[0].OutPlayed {
		t.Error("a player with no minutes is marked as having appeared")
	}
	// An unknown id must not read as having played.
	v = Judge(s, []Move{{GW: 8, OutID: 99999, InID: 13}}, 5)
	if v[0].OutPlayed {
		t.Error("an unknown player is marked as having appeared")
	}
}

// TestFreeTransfersAreNotFree pins the gate that stopped the replay churning.
//
// A transfer with no points deducted still has to clear FreeCost across the
// horizon. Without it the policy sold Palmer at GW6 and bought him back at GW8,
// cycled the same three forwards across four transfers, and produced twelve
// round-trips across three replayed seasons.
func TestFreeTransfersAreNotFree(t *testing.T) {
	const horizon, cost = 5.0, 2.0
	worth := func(gain float64) bool { return gain*horizon >= cost }

	if worth(0.39) {
		t.Error("0.39/gw is +1.95 over the horizon and must not spend a free transfer")
	}
	if !worth(0.41) {
		t.Error("0.41/gw is +2.05 over the horizon and should")
	}
	// The old gate was MinGain alone, which a marginal move clears easily.
	if 0.39 < 0.4 == false {
		t.Fatal("fixture assumes MinGain 0.4")
	}
	// A hit still has to clear four on top of its own threshold, so the bar for
	// a paid transfer stays strictly higher than for a free one.
	if cost >= HitCost {
		t.Errorf("a free transfer is charged %.1f against a hit's %.1f; it must be cheaper, "+
			"or there is never a reason to prefer the free one", cost, float64(HitCost))
	}
}

// TestChargingFourForAFreeTransferIsTooMuch records the measurement, because
// four is the intuitive answer and it is wrong.
//
// Four is what an extra transfer costs, so it looks like the price of the one
// in hand. Charged that much the replay made 39 transfers instead of 73 and
// scored below charging nothing at all — the gate stops filtering noise and
// starts refusing real improvements. Everything from 1 to 2.5 beat zero on all
// three seasons.
func TestChargingFourForAFreeTransferIsTooMuch(t *testing.T) {
	if config.Default().Review.FreeTransferValue >= HitCost {
		t.Errorf("free transfer valued at %.1f, at or above a hit's %.1f — measured at 4 the "+
			"replay scored 2073 against 2091 for charging nothing",
			config.Default().Review.FreeTransferValue, float64(HitCost))
	}
	if v := config.Default().Review.FreeTransferValue; v <= 0 {
		t.Errorf("free transfer valued at %.1f; zero reproduces the churning policy", v)
	}
}

// TestBankLimitFollowsTheRulesOfTheSeason — FPL raised the transfer bank from 2
// to 5 for 2024-25. Replaying an earlier season under the modern rule lets the
// policy save transfers it could not have saved, and every conclusion drawn
// from that season is then about a game nobody played.
func TestBankLimitFollowsTheRulesOfTheSeason(t *testing.T) {
	for season, want := range map[string]int{
		"2021-22": 2, "2022-23": 2, "2023-24": 2,
		"2024-25": 5, "2025-26": 5, "2026-27": 5,
	} {
		if got := BankLimitFor(season); got != want {
			t.Errorf("BankLimitFor(%s) = %d, want %d", season, got, want)
		}
	}
	if config.Default().Review.BankUpTo != 5 {
		t.Errorf("config default banks %d transfers; the current rule is 5",
			config.Default().Review.BankUpTo)
	}
}

// TestTheTransferChargeDoesNotTaper records a change that was made and undone.
//
// The charge was tapered to zero over the closing gameweeks, on the argument
// that a transfer banked through the final whistle is worth nothing. That is
// true of *option value* and false of what this charge actually does — it is a
// confidence threshold, and a marginal move in May is as likely to be noise as
// one in September with less time to come good.
//
// Measured across three seasons, tapering scored 2199 against 2208 for the flat
// charge, and left 20 transfers after GW33 returning +72 where the flat charge
// makes 8 returning +89. Fewer, better late moves.
func TestTheTransferChargeDoesNotTaper(t *testing.T) {
	cfg := SimConfig{FreeCost: 2.0}
	cfg.Weights.Horizon = 5
	// The charge is read straight from config with no gameweek term. If a taper
	// is reintroduced it has to beat 2208 first.
	if cfg.FreeCost != 2.0 {
		t.Fatalf("fixture broken: %v", cfg.FreeCost)
	}
}

// TestLateSeasonQuietIsNotThePrice records why the taper is worth so little.
//
// The replay makes almost no transfers after GW28, and the obvious explanation
// is that the shrinking horizon stops paying for them. Measured on 2025-26 it is
// not: the best available swap from GW29 onward gains 0.00, 0.00, 0.00, 0.13,
// 0.00, 0.02, 0.09, 0.00, 0.00 and 0.02 points a gameweek. The squad has
// converged and there is nothing left to buy, so lowering the price changes
// nothing — tapering to zero is correct but bought about a point a season.
//
// The test is the arithmetic that shows the charge cannot be what binds.
func TestLateSeasonQuietIsNotThePrice(t *testing.T) {
	cfg := SimConfig{FreeCost: 2.0, MinGain: 0.4}
	cfg.Weights.Horizon = 5

	// The best gains actually observed from GW29 to GW38 in the 2025-26 replay.
	observed := []float64{0.00, 0.00, 0.00, 0.13, 0.00, 0.02, 0.09, 0.00, 0.00, 0.02}
	for i, gain := range observed {
		gw := 29 + i
		// MinGain is per gameweek, and every one of these falls short of it —
		// so the move is refused even if the transfer were handed over free.
		if gain >= cfg.MinGain {
			t.Errorf("GW%d gain %.2f clears MinGain; the fixture no longer shows a "+
				"converged squad", gw, gain)
		}
	}
	// Even handed over free, the best move on the final gameweek is worth 0.02
	// a gameweek, so the price was never what bound.
	if observed[len(observed)-1] >= cfg.MinGain {
		t.Error("GW38 fixture no longer shows a converged squad")
	}
}

// TestEveryScoringEngineGetsRecency guards a bug that hid for a whole round of
// measurement.
//
// Simulate builds three engines: one to decide transfers, one to pick the
// eleven, and one inside Hold. A patch wired recency into two of them and
// silently missed the transfer decision, so transfers were still being chosen on
// flat season minutes while the reported gain came entirely from better
// captaincy. The scores looked plausible and the season totals moved, so nothing
// failed — it only surfaced when the sell-side error refused to budge.
//
// Wiring the third one in took the error on players being sold from a median
// -0.70/gw to -0.19 and the mean across three seasons from 2152 to 2199.
func TestEveryScoringEngineGetsRecency(t *testing.T) {
	src, err := os.ReadFile("simulate.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	engines := strings.Count(body, "analysis.NewEngineFull(")
	// Any assignment counts, not just a fresh newRecentIndex call. The chip
	// code builds a free-hit engine for a gameweek an engine already exists
	// for and reuses that index (`he.Recent = ve.Recent`) — correct, and
	// cheaper than building the same thing twice. A prefix match on
	// "newRecentIndex" called that unwired, which is the guard being literal
	// rather than the code being wrong.
	// De-quoted on BOTH sides of the balance. Subtracting quoted engine
	// constructions and not quoted recency assignments is asymmetric, and the
	// asymmetry makes the guard less sensitive rather than more: this very file
	// contains four occurrences of ".Recent = " and not one is an assignment,
	// so it scored itself 1 engine / 4 wired and exempted its own construction.
	// Found by audit, in the commit that fixed the mirror of the same hole.
	wired := strings.Count(body, ".Recent = ") -
		strings.Count(body, `".Recent = `) - strings.Count(body, "`he.Recent = ")
	// But the builder must still be reached, or a refactor that renamed it
	// would leave every engine assigned from a nil neighbour and this count
	// would still balance. That is the failure the prefix match was guarding
	// and it is kept as its own assertion.
	//
	// It now takes two hops, because the four near-identical newRecentIndexWith
	// calls were collapsed into SimConfig.recentIndex so the minutes oracle has
	// one seam to perturb rather than four. That is the right direction — four
	// copies of one expression is the shape this package's most-repeated bug
	// takes — but it moves the builder call into another file, so following the
	// seam is the guard's job now. Checking only the assignment here would pass
	// against a constructor that had quietly stopped calling the builder.
	if !strings.Contains(body, ".Recent = cfg.recentIndex") {
		t.Error("no engine builds a recency index through cfg.recentIndex; if the " +
			"constructor was renamed, update this test — if it was dropped, every " +
			"engine is now falling back to flat season minutes")
	}
	// The second hop, parsed rather than grepped.
	//
	// It used to read minutesoracle.go and look for the substring
	// "newRecentIndexWith(" anywhere in it — a file whose first sixty-odd lines
	// are prose *about* that constructor, so a comment mentioning it with a paren
	// would have silenced the guard entirely. It also hardcoded the filename for a
	// constructor that is not oracle-specific and would move the moment
	// SimConfig.recentIndex did.
	//
	// Asking the question directly — does recentIndex's *body* call the builder —
	// is location-independent and cannot be satisfied by a comment.
	if !funcBodyCalls(t, "recentIndex", "newRecentIndexWith") {
		t.Error("SimConfig.recentIndex's body no longer calls newRecentIndexWith, so " +
			"every engine is assigned an index that was never built from the season")
	}
	// All of them, now including the one that builds the opening squad. That
	// used to be exempt on the grounds that there is no season to be recent
	// about before GW1 — true only while every replay started there. With
	// StartGW a mid-season entry picks its squad from ten weeks of form, and
	// newRecentIndex already returns nil when `through` is zero, so the
	// exemption bought nothing and cost the case it was wrong about.
	if wired != engines {
		t.Errorf("%d engines built, %d given a recency index. An engine that scores "+
			"players without one silently falls back to flat season minutes.",
			engines, wired)
	}

	// And everywhere else in the repository, which is the seam this guard had
	// until 2026-08-12.
	//
	// It read simulate.go and nothing else, so an engine built anywhere else was
	// invisible to it. `cmd/flagfit` built one with NewEngineFull and set neither
	// Recent nor Priors, measured 48,803 observations against the resulting
	// fallback value, and published an exponent that had to be withdrawn — while
	// this test passed, because the rule it enforces was scoped to one file.
	//
	// The rule was right and its reach was wrong, which is the more expensive of
	// the two failures: a guard that cannot see the mistake reads exactly like a
	// guard that found nothing.
	//
	// The check is the same balance as above — an engine per recency assignment —
	// rather than "must call EngineAt". EngineAt takes a Season and a SimConfig,
	// so the live path in cmd/armband cannot use it, and several diagnostics
	// wire the index correctly by hand. Demanding one constructor would flag
	// correct code, and a guard that cries wolf is turned off.
	for _, f := range enginesMissingRecency(t) {
		if why, known := unwiredBaseline[filepath.Base(f.path)]; known {
			t.Logf("known unwired engine: %s — %s", f.path, why)
			continue
		}
		t.Errorf("%s builds %d engine(s) and assigns %d recency index(es). An engine "+
			"that scores players without one silently falls back to flat season "+
			"minutes: ExpectedMinutes reads the season mean and blankRunFactor never "+
			"fires. The field is populated and the value is wrong, which is why this "+
			"is a guard and not a comment. In internal/backtest, backtest.EngineAt "+
			"attaches Priors, Recent and TeamForm the way Simulate does.",
			f.path, f.engines, f.wired)
	}
}

type engineSite struct {
	path    string
	engines int
	wired   int
}

// enginesMissingRecency returns every Go file in the repository that constructs
// more engines than it gives a recency index to.
//
// Source-scanning rather than a runtime check, for the reason the neighbouring
// guards give: the failure is a new call in a new file, and it agrees with
// everything else on the day it is written.
//
// Counting per file is coarse — a file could build two engines and wire the
// same one twice — but it is the same trade the in-file check above accepts, and
// it catches the failure that actually happened: a file that builds one and
// wires none.
func enginesMissingRecency(t *testing.T) []engineSite {
	t.Helper()
	var out []engineSite
	err := filepath.Walk("../..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// .git holds no Go the compiler sees; .claude holds other branches'
			// entire checkouts, which would be scanned as though they were this one.
			if info.Name() == ".git" || info.Name() == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// simulate.go is checked engine by engine above.
		if filepath.Base(path) == "simulate.go" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := string(b)
		// Subtract quoted occurrences. The sibling guards count this very string
		// in their own source, so a scanner that cannot tell a call from a string
		// literal flags the guards that do the same job for TeamForm and Priors.
		// The neighbouring guard's history records the mirror of this: a comment
		// mentioning the constructor with a paren once silenced it entirely.
		engines := strings.Count(body, "analysis.NewEngineFull(") -
			strings.Count(body, `"analysis.NewEngineFull(`)
		if engines == 0 {
			return nil
		}
		// Only a recency assignment counts. Calling EngineAt does NOT offset a
		// bare NewEngineFull in the same file — EngineAt constructs its engine in
		// engineat.go, not here, so counting it would let one correct call mask an
		// incorrect one. Caught by mutation-testing this guard: adding a bare
		// construction to a file that already called EngineAt did not trip it.
		wired := strings.Count(body, ".Recent = ") -
			strings.Count(body, `".Recent = `) - strings.Count(body, "`he.Recent = ")
		if wired < engines {
			out = append(out, engineSite{path: path, engines: engines, wired: wired})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	return out
}

// funcBodyCalls reports whether the named package-level function or method calls
// the named function anywhere in its body.
//
// It exists so a wiring guard can follow a seam across a rename or a file move
// without hardcoding either, and so that prose describing the seam cannot satisfy
// the guard that watches it. Non-test files only: the question is about shipped
// wiring.
func funcBodyCalls(t *testing.T, fn, callee string) bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found, called := false, false
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name.Name != fn || fd.Body == nil {
				continue
			}
			found = true
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == callee {
					called = true
				}
				return true
			})
		}
	}
	if !found {
		t.Errorf("no function named %q in this package — the guard is following a "+
			"seam that has been renamed or removed, so update it deliberately "+
			"rather than letting it pass vacuously", fn)
	}
	return called
}

// TestEveryScoringEngineGetsPriors is the same count for the third information
// channel, which had none.
//
// The omniscience control found out the hard way that PointInTime is not the
// single seam into "what the model believes about a player": blendRates has a
// pre-season branch that, at played == 0, overwrites the entire blend from
// Engine.Priors. So Priors is a channel in its own right, exactly as Recent is,
// and it is assigned at every one of the same six sites.
//
// It has already diverged once. HoldCaptaincyWeekly built its prior with
// newPriorIndexMulti unconditionally while Simulate honoured PriorMinutesHalfLife
// and PriorRateHalfLife, so a diagnostic that set either gave the *squad* a
// recency-weighted prior and the weekly re-pick that scores it a flat one — one
// quantity, two expressions, silently disagreeing. SimConfig.priors closed that,
// and this is what stops it reopening.
func TestEveryScoringEngineGetsPriors(t *testing.T) {
	src, err := os.ReadFile("simulate.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	engines := strings.Count(body, "analysis.NewEngineFull(")
	wired := strings.Count(body, ".Priors = ")
	if wired != engines {
		t.Errorf("%d engines built, %d given a prior index. An engine without one "+
			"scores every player with no prior season on his own current-season "+
			"sample, which is the debut-explosion bug shrinkToLeague exists to fix.",
			engines, wired)
	}
	// The same two-hop check the recency guard makes, for the same reason: the
	// assignments could all balance while every one of them came from a builder
	// that had stopped being called.
	if !strings.Contains(body, ".Priors = idx") {
		t.Error("no engine takes its prior from a shared index; if the variable was " +
			"renamed, update this test")
	}
	if !funcBodyCalls(t, "priors", "newPriorIndexMulti") {
		t.Error("SimConfig.priors' body no longer calls newPriorIndexMulti, so the " +
			"multi-season prior blend is unreachable and every engine falls back to " +
			"last season alone")
	}
	// The run-in half of the same constructor. It is the branch a diagnostic
	// reaches by setting PriorMinutesHalfLife or PriorRateHalfLife, and the one
	// whose absence from HoldCaptaincyWeekly made the run-in table's HOLD column
	// a hybrid — see the constants-and-sweeps note.
	if !funcBodyCalls(t, "priors", "newPriorIndexRecent") {
		t.Error("SimConfig.priors' body no longer calls newPriorIndexRecent, so " +
			"prior_half_life's run-in weighting is unreachable and setting it is a " +
			"silent no-op")
	}
}

// TestOnlyTheTransferEngineAnticipatesChips is the counting guard for the fourth
// per-engine channel, and it counts *down* rather than up.
//
// Recency, priors and club form must reach every engine, and their guards fail
// when one is missed. `SimConfig.anticipate` is the opposite: it must reach
// exactly the engine the weekly transfer decision uses and no other, because the
// arm's whole claim rests on it. `AnticipateChips` was measured with `HOLD` at
// +0.000 in all 24 cells and that was read as a leak check — it is a
// *construction guarantee*, and it is only a guarantee while this count is one.
//
// # What that costs the arm, stated because the arm's own write-up did not
//
// The opening-squad engine is deliberately left blind. So the arm cannot see the
// fifteen you buy three weeks before a wildcard tears it up: `SuggestBenchWeight`
// reaches squad construction through `OptimizeRequest.BenchWeight` and nothing on
// this path calls it. The write-up named the bench boost as the mechanism the arm
// does not carry, and this is the second one.
//
// Wiring the opening squad would be a different and larger experiment — it would
// move `HOLD`, so the invariance that currently proves no leak would stop being
// available as a check.
func TestOnlyTheTransferEngineAnticipatesChips(t *testing.T) {
	src, err := os.ReadFile("simulate.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	if n := strings.Count(body, "cfg.anticipate("); n != 1 {
		t.Errorf("cfg.anticipate is called at %d engine site(s), not 1. Reaching a "+
			"second engine would move HOLD, and HOLD being byte-identical is the "+
			"only evidence the anticipation arm has that it touched the transfer "+
			"decision and nothing else. Reaching none makes the arm inert while "+
			"still reporting a difference for the chips it plays.", n)
	}
	// And it must be the engine `decide` is handed. `pe` is built, anticipated,
	// then passed to decide in the same block; a call sited on `ve` or `he` would
	// still count as one.
	if !strings.Contains(body, "cfg.anticipate(pe, gw)") {
		t.Error("cfg.anticipate is no longer called on `pe`, the engine the weekly " +
			"transfer decision is run against. If the variable was renamed, update " +
			"this test — if the call moved to the eleven's engine or the free-hit " +
			"engine, the arm is measuring something other than what it claims")
	}
}

// TestDecisionHorizonDefaultsToTheFixtureWindow — separating them must not
// change behaviour for anyone who does not set it.
func TestDecisionHorizonDefaultsToTheFixtureWindow(t *testing.T) {
	var cfg SimConfig
	cfg.Weights.Horizon = 5
	if got := effectiveHorizon(cfg.Weights.Horizon, 10); got != 5 {
		t.Fatalf("fixture window %v", got)
	}
	// decide() falls back to Weights.Horizon when DecisionHorizon is unset; the
	// two knobs exist because Horizon was doing both jobs at once, and sweeping
	// it measured a fixture effect and a threshold effect mixed together.
	if cfg.DecisionHorizon != 0 {
		t.Error("DecisionHorizon should default to zero, meaning 'use the window'")
	}
}

// TestPointInTimeHidesFutureResults guards the replay's most dangerous failure
// mode. The archive carries every scoreline for the whole season, so anything
// reading fixture results — the attack/defence bands most obviously — would
// happily rate clubs on matches that had not been played yet, and the replay
// would report a score no manager could have achieved.
//
// The scores are needed, so they cannot simply be dropped at load: they are
// gated by gameweek instead, and this is what checks the gate.
func TestPointInTimeHidesFutureResults(t *testing.T) {
	cur, prior := mkSeason(), mkSeason()
	// One fixture per gameweek, every one with a result.
	for gw := 1; gw <= 38; gw++ {
		e, h, a := gw, gw%5, gw%7
		cur.Fixtures = append(cur.Fixtures, fpl.Fixture{
			ID: gw, Event: &e, TeamH: 1, TeamA: 2,
			TeamHScore: &h, TeamAScore: &a,
		})
	}

	for _, through := range []int{1, 5, 12, 20, 38} {
		_, fx := PointInTime(cur, prior, through)
		var visible, leaked int
		for _, f := range fx {
			if f.Event == nil {
				continue
			}
			has := f.TeamHScore != nil || f.TeamAScore != nil
			switch {
			case *f.Event <= through && has:
				visible++
			case *f.Event > through && (has || f.Finished):
				leaked++
			}
		}
		if leaked > 0 {
			t.Errorf("through GW%d: %d fixtures after the cutoff still carry a result",
				through, leaked)
		}
		if visible != through {
			t.Errorf("through GW%d: %d results visible, want %d — too few and the "+
				"bands can never form", through, visible, through)
		}
	}
}

// TestDefconIsScoredOnlyFrom2025_26 pins the rule and the wiring together.
//
// FPL introduced defensive contribution for 2025-26. The replay was blind to it
// in every season — PointInTime never populated the field — so the one season
// that has the category was scored by a model that could not see it, worth
// about 95 points. Earlier seasons must stay blind, and not by accident: the
// archive happens to carry no defensive_contribution before 2025-26, but the
// underlying actions were always recorded and a backfill would silently hand
// three replayed seasons a rule nobody played under.
func TestDefconIsScoredOnlyFrom2025_26(t *testing.T) {
	for season, want := range map[string]bool{
		"2021-22": false, "2022-23": false, "2023-24": false,
		"2024-25": false, "2025-26": true, "2026-27": true,
	} {
		if got := DefconScoredIn(season); got != want {
			t.Errorf("DefconScoredIn(%s) = %v, want %v", season, got, want)
		}
	}
}

// TestStartGWWindowsBothPolicyAndBaseline — a mid-season entry must be scored
// over the weeks it played, and so must the baseline it is compared against.
//
// Both halves have been wrong. The policy charged the squad `start`-week prices
// for a squad the optimiser had quoted at `start-1`, opening with a negative
// bank; the transfer search then accepted only moves that freed money, and a
// GW11 entry made twenty-four forced downgrades and finished 476 points below
// simply holding. And the baseline was built from a fresh SimConfig carrying
// only Weights, so it ignored StartGW entirely and scored the whole season —
// 23 weeks of football compared against 38.
func TestStartGWWindowsBothPolicyAndBaseline(t *testing.T) {
	ctx := context.Background()
	cc := loadConfig(t)
	cur, err := Load(ctx, cc.CacheDir, "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(ctx, cc.CacheDir, "2022-23")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	cfg := SimConfig{
		Weights: config.Default().Weights, Budget: analysis.DefaultBudget,
		BankUpTo: BankLimitFor("2023-24"), FreeCost: 2, StartGW: 16,
	}
	sim, err := Simulate(cur, prior, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sim.Weeks); got != 23 {
		t.Errorf("a GW16 entry played %d gameweeks, want 23", got)
	}
	if len(HoldWeekly(cur, prior, cfg, sim.OpeningSquad)) != 23 {
		t.Error("the hold baseline is not windowed to the same gameweeks")
	}
	// The wallet must not open in the red: that is the negative-bank bug, and
	// its symptom is a policy that trails the baseline it should beat.
	hold := Hold(cur, prior, cfg, sim.OpeningSquad)
	if sim.Points < hold {
		t.Errorf("policy scored %d against %d for holding; a mid-season entry that "+
			"cannot afford to transfer is the negative-bank bug", sim.Points, hold)
	}
	t.Logf("GW16 entry: hold %d, policy %d over %d weeks", hold, sim.Points, len(sim.Weeks))
}

// unwiredBaseline is the set of files that built an engine without a recency
// index before the guard above could see them, keyed by base name.
//
// It is a baseline and not an exemption list. The guard's job from here is to
// catch the *next* one; these are debt, and each entry says what is known about
// it rather than asserting it is fine. Two of the three classes below are
// genuinely correct code and one is the same defect the guard was widened for,
// which is exactly why a blanket "these are all fine" would have been the wrong
// shape.
//
// **Auditing the "not yet checked" entries is queued.** Removing an
// entry from this map is the deliberate act that says someone looked.
var unwiredBaseline = map[string]string{
	// ---- Correct: nothing to be recent about ----
	//
	// A pre-season or synthetic engine has no current season behind it, and
	// newRecentIndex returns nil at a zero cutoff anyway. These are not debt.
	"backtest.go": "OK on recency (cutoff 0) — but builds with no Priors, unlike " +
		"Simulate at StartGW 1, so the printed GW1 squad is not the replay's. " +
		"See AGENTS.md 'priors must load regardless of season state'.",
	"simulate_test.go": "OK — the engine at DataWindow's test reads only the " +
		"gameweek count off a synthetic season; no rate or minute enters the verdict. " +
		"Only visible since the wired count was de-quoted.",
	"fixtureload_test.go": "OK — synthetic empty bootstrap, no season at all",
	// The served application's fixture, built from the committed GW1 capture. At GW1
	// GameweeksPlayed() is 0, and cmd/armband/main.go wires BOTH the recency index and
	// the priors behind `GameweeksPlayed() > 0` — so the live binary builds an engine
	// with neither at this point in the season. Attaching one here would make the
	// fixture LESS faithful to production, not more, and would make every visual
	// golden a picture of a squad the binary never produces.
	// TestTheFixtureMatchesWhatProductionBuildsAtGW1 asserts the premise rather than
	// leaving it as a claim in a comment.
	"webroutes_test.go":          "OK — GW1 capture, GameweeksPlayed()==0, and the live path wires recency only above 0",
	"determinism_test.go":        "OK — PointInTime cutoff 0",
	"determinismfactors_test.go": "OK — PointInTime cutoff 0",

	// ---- Fixed 2026-08-12 ----
	//
	// The seven mid-season sites below all built unwired and were rewired onto
	// EngineAt. They are kept here, empty, as the record of what was wrong: the
	// map is a baseline and an entry leaving it is the deliberate act that says
	// someone looked. Re-add one only with a reason.

	// ---- Mismatch with production, milder ----
	//
	// Builds a LIVE engine from the API the way cmd/armband does — except the
	// live path sets .Recent and this does not, so the tool layer is exercised
	// against a different engine from the one that ships.
	"transfers_test.go": "MISMATCH — live engine, production wires recency and this does not",
}
