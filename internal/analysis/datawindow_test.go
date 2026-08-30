package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// skipDuringLiveGW1Gap skips when the engine was built from the LIVE API (not a
// simulated one — see playGameweeks) and the season is caught in the one window
// this whole suite cannot reason about honestly: SeasonHasStarted true,
// GameweeksPlayed still 0.
//
// # Why this specific window, and not "any live season"
//
// Once GameweeksPlayed() > 0, every club has played its first fixture, so every
// player has SOME current-season evidence — thin, but real, and el.Minutes means
// what the doc comments say it means again. Before SeasonHasStarted, el.Minutes
// is still last season's carryover, also honest. Only the gap between the two —
// GW1 itself, from the first kickoff of the season to the last final whistle of
// it, a span of days — has SOME clubs on fresh-season zeros and others still on
// last season's totals, in the SAME live fetch.
//
// cmd/armband's live server closes this gap with internal/priors: once
// SeasonHasStarted, it loads last season as a fallback so a fresh-season zero
// reads as "no evidence yet" rather than "scores zero". These tests cannot do
// the same — internal/priors imports internal/analysis, so a test file in
// package analysis importing internal/priors back is a cycle, not a call.
// internal/backtest carries the same wiring cmd/armband does, because it is a
// separate package one level up; that is where this scenario is exercised for
// real, not here.
//
// So a test built on a live engine in exactly this window sees a mix no doc
// comment in this package promises to handle, and asserting through it proves
// nothing about the model — only about which clubs happened to have fixtures
// early in the alphabet this particular gameweek. Skip rather than fail; it
// passes again within days, on its own, without a code change.
func skipDuringLiveGW1Gap(t *testing.T, e *Engine) {
	t.Helper()
	if e.SeasonHasStarted() && e.GameweeksPlayed() == 0 {
		t.Skip("mid-GW1: some clubs have played this season and some have not, " +
			"in the same live fetch, and this package's test engines do not load " +
			"a prior season the way cmd/armband's live server does — see this " +
			"function's own comment. Passes again once GW1 finishes.")
	}
}

// skipUntilLiveEvidence skips while the live season has played fewer than
// `matches` gameweeks, for a test whose subject needs that much accumulated
// evidence to exist at all.
//
// # Why the gap guard above is not enough, and what that cost
//
// `skipDuringLiveGW1Gap` closes the mid-GW1 window — the days when some clubs
// have played and some have not — and its reasoning about that DATA MIX is
// right. What it is not is a statement about how much evidence a test needs,
// and on 2026-08-25 the difference took **twelve tests** down across this
// package and `internal/agent`, the morning GW1 finished, on code nobody had
// touched. Measured that day, one gameweek in: 612 players, **307 with a
// non-zero score**, `DataWindow() == 1`. Nothing was broken. There was one match
// of football, and a rested player reads zero rather than "no evidence yet".
//
// So the guard closed a window one week wide in front of tests needing anywhere
// from two matches to ten, and they began failing the moment it stopped.
//
// # The threshold belongs to the CALLER
//
// It differs by an order of magnitude between them, so one shared number would
// be wrong for most:
//
//   - `corroboratingMatches` (2) is this package's own bar for "this player's
//     minutes are trustworthy" — the floor for anything reading minutes.
//   - the set-piece duty test wants 900 minutes, i.e. ten full matches.
//
// ⚠️ **Prefer making a test season-independent over skipping it.** `chips_test.go`
// now expresses its gameweeks relative to `upcomingGW` and is asserted all year;
// that is strictly better than going quiet for ten months. A skip is the
// fallback for tests whose subject genuinely IS accumulated evidence.
func skipUntilLiveEvidence(t *testing.T, e *Engine, matches int) {
	t.Helper()
	skipDuringLiveGW1Gap(t, e)
	if played := e.GameweeksPlayed(); e.SeasonHasStarted() && played < matches {
		t.Skipf("live season has played %d gameweek(s); this test needs %d before "+
			"its subject exists in the data. Not a defect — see "+
			"skipUntilLiveEvidence. Asserts again from GW%d.", played, matches, matches)
	}
}

// playGameweeks marks the first n gameweeks finished, so the engine believes
// that much of the season has been played.
//
// It drives BOTH signals the engine reads for this, and must: GameweeksPlayed reads
// Boot.Events[].Finished, but matchesAvailable — the one that actually turns a
// player's raw Minutes into a rate — reads TeamMatchesStarted, which is computed
// from e.Fixtures[].Started, not from Events at all. Both engines here load live
// fixtures from the real FPL API (see roleEngine), so whatever the real season's
// current matches happen to show would otherwise leak into a test that is supposed
// to control every gameweek explicitly. Setting both keeps the simulation the only
// thing either signal can see.
func playGameweeks(t *testing.T, e *Engine, n int) {
	t.Helper()
	for i := range e.Boot.Events {
		e.Boot.Events[i].Finished = i < n
	}
	for i := range e.Fixtures {
		ev := e.Fixtures[i].Event
		played := ev != nil && *ev <= n
		e.Fixtures[i].Started = played
		e.Fixtures[i].Finished = played
		// ⚠️ FinishedProvisional too, and it is the one that was missing.
		//
		// There are THREE signals here, not the two this comment used to claim.
		// TeamMatchesFinished -- the denominator the current/prior MINUTES blend
		// divides by, blend.go's `n := float64(e.TeamMatchesFinished(el.Team))`
		// -- is gated on FinishedProvisional rather than Finished, deliberately:
		// Finished lags full time by many hours on live data, and using it would
		// reintroduce the staleness that gate exists to remove.
		//
		// Leaving it unset made every simulated season read as ZERO completed
		// matches, so `mix` gave the prior full weight and MinutesPerMatch pinned
		// to priorMinutes/38 no matter what the caller set el.Minutes to. A
		// player dropped to ten minutes a week still reported 86.84 expected
		// minutes, which is exactly what TestBlendYieldsToTheSeason complains
		// about -- and it was this helper's fault, not the engine's.
		e.Fixtures[i].FinishedProvisional = played
	}
	if got := e.GameweeksPlayed(); got != n {
		t.Fatalf("set up %d finished gameweeks, engine sees %d", n, got)
	}
	// ⚠️ Assert the OTHER two signals as well, or this helper cannot detect its
	// own no-op. The check above reads Boot.Events and would have passed
	// unchanged through the entire defect described above.
	if n > 0 {
		if !e.SeasonHasStarted() {
			t.Fatalf("set up %d gameweeks but no fixture reads Started, so the "+
				"engine still believes it is pre-season", n)
		}
		var anyFinished bool
		for _, tm := range e.Boot.Teams {
			if e.TeamMatchesFinished(tm.ID) > 0 {
				anyFinished = true
				break
			}
		}
		if !anyFinished {
			t.Fatalf("set up %d gameweeks but TeamMatchesFinished is 0 for every "+
				"club, so the minutes blend will see no evidence and anchor on the "+
				"prior", n)
		}
	}
}

// findEverPresent returns a player who started nearly every match last season,
// to stand in for an ever-present during the simulated season.
func findEverPresent(t *testing.T, e *Engine) *fpl.Element {
	t.Helper()
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Starts >= 35 && el.Minutes >= 3000 && el.ElementType == 3 {
			// Skip anyone on the post-tournament rest list. That term scales
			// expected minutes, so a flagged player legitimately reports below 90
			// and would fail callers asserting about something else entirely —
			// the data window, rate shrinkage, the recency hook.
			if _, f := e.restFactor(el); f < 1 {
				continue
			}
			// And skip anyone unavailable. Every caller compares scores before
			// and after some change, and an injured player scores zero both
			// times, so the comparison silently proves nothing.
			if availabilityFactor(el) == 0 {
				continue
			}
			return el
		}
	}
	t.Skip("no unrested ever-present midfielder in the dataset")
	return nil
}

// TestDataWindowTracksTheSeason is the guard on the bug that made the model
// unusable the moment a ball was kicked.
//
// FPL's `minutes` field carries last season's total until GW1 completes, then
// resets and accumulates. The denominator has to follow it. Dividing one
// gameweek's 90 minutes by a fixed 38 reports an ever-present as 2.4 minutes per
// gameweek — every player in the game lands in the "fringe" band, scored at
// about 1% of true value, and nothing recovers until roughly GW29.
func TestDataWindowTracksTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	// ⚠️ PRE-SEASON ONLY, and it used to assert unconditionally.
	//
	// The window is a full season precisely because FPL still holds last
	// season's totals — which stops being true the moment GW1 completes, when
	// `DataWindow()` correctly drops to 1. Asserting 38 in-season demands the
	// bug this test exists to prevent. It failed every branch's CI from the
	// morning GW1 finished, on behaviour that was right.
	//
	// The simulated sweep below is unaffected and is the real regression guard:
	// `playGameweeks` drives the window explicitly, so it holds in any week.
	if !e.SeasonHasStarted() {
		if got := e.DataWindow(); got != GameweeksPerSeason {
			t.Errorf("pre-season data window is %d, want %d — FPL still holds last "+
				"season's totals, so the window is a full season", got, GameweeksPerSeason)
		}
	}

	el := findEverPresent(t, e)
	trueMins, trueStarts := el.Minutes, el.Starts
	defer func() { el.Minutes, el.Starts = trueMins, trueStarts }()

	for _, gw := range []int{1, 2, 3, 5, 10, 19, 28, 38} {
		playGameweeks(t, e, gw)
		if got := e.DataWindow(); got != gw {
			t.Fatalf("after GW%d the data window is %d, want %d", gw, got, gw)
		}

		// An ever-present: he has played every minute of every match so far.
		el.Minutes, el.Starts = gw*90, gw
		m := e.Metrics(el)

		if m.ExpectedMinutes < 85 || m.ExpectedMinutes > 95 {
			t.Errorf("after GW%d an ever-present shows %.1f expected minutes, want ~90",
				gw, m.ExpectedMinutes)
		}
		// ⚠️ From GW2, not GW1. `corroboratingMatches` is 2 -- this package's own
		// bar for "this player's minutes are trustworthy" -- so ONE match of
		// ninety minutes is deliberately not enough to call anyone nailed, and
		// the band correctly reads "likely starter" there.
		//
		// ⚠️ CORRECTED 2026-08-30 on review. An earlier version of this comment
		// blamed playGameweeks not setting Fixture.FinishedProvisional. That is
		// wrong and was checked: e.Priors is never set in this file, so
		// blendRatesCode returns before it reaches TeamMatchesFinished at all,
		// and minutesCorroborated -- the function that actually gates "nailed" --
		// reads el.Minutes/90 against corroboratingMatches directly. Reverting
		// only the FinishedProvisional line still fails here identically.
		//
		// The real history: corroboratingMatches landed in d0fcd865 on
		// 2026-08-22, after this file was last touched, so the unconditional GW1
		// assertion had ALREADY been broken by that commit. It survived because
		// the live data this test ran on did not reach the branch. The gw > 1
		// guard is the right fix; the mechanism previously written here was not
		// the one at work.
		if gw > 1 && m.RotationRisk != "nailed" {
			t.Errorf("after GW%d an ever-present is banded %q, want nailed",
				gw, m.RotationRisk)
		}
		if m.MinutesRating < 0.9 {
			t.Errorf("after GW%d an ever-present has reliability %.3f, want ~1.0",
				gw, m.MinutesRating)
		}
	}
}

// TestMinutesFloorScalesWithTheSeason guards the second half of the same bug. A
// 600-minute floor is written as a season total; applied unscaled it excludes
// every player in the game until about GW26, and the optimiser fails outright
// rather than returning a bad squad.
func TestMinutesFloorScalesWithTheSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	defer playGameweeks(t, e, 0)

	for _, gw := range []int{1, 3, 8} {
		playGameweeks(t, e, gw)
		sq, err := e.Optimize(OptimizeRequest{
			Budget: DefaultBudget, MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02,
		})
		if err != nil {
			t.Fatalf("after GW%d the optimiser failed with a season-scale minutes floor: %v", gw, err)
		}
		if !squadIsLegal(sq.Players, DefaultBudget) {
			t.Errorf("after GW%d the optimiser returned an illegal squad", gw)
		}
	}
}

// TestTournamentAbsencesStopApplyingInSeason — the absence list describes the
// season the aggregates came from. Once gameweeks are played FPL has overwritten
// those aggregates, so last summer's list describes data that is no longer in
// hand and must not shrink the denominator.
func TestTournamentAbsencesStopApplyingInSeason(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	defer playGameweeks(t, e, 0)

	var listed *fpl.Element
	for i := range e.Boot.Elements {
		if e.tournamentAbsence(&e.Boot.Elements[i]).Matches > 0 {
			listed = &e.Boot.Elements[i]
			break
		}
	}
	if listed == nil {
		t.Skip("no player carries a tournament absence")
	}
	if e.matchesAvailable(listed) >= GameweeksPerSeason {
		t.Error("pre-season, a listed player's denominator should be reduced")
	}

	playGameweeks(t, e, 5)
	if a := e.tournamentAbsence(listed); a.Matches != 0 {
		t.Errorf("%s still carries a %d-match absence from last season's list "+
			"after 5 gameweeks of this one", listed.WebName, a.Matches)
	}
	if got := e.matchesAvailable(listed); got != 5 {
		t.Errorf("%s has a denominator of %d after 5 gameweeks, want 5", listed.WebName, got)
	}
}
