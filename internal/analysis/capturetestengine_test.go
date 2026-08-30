package analysis

import (
	"encoding/json"
	"testing"

	"armband/internal/capture"
	"armband/internal/fpl"
)

// Engines for tests, built from COMMITTED CAPTURES rather than the live API.
//
// # Why this replaced a live fetch
//
// Every engine helper in this package used to call `fpl.New(t.TempDir(), …)` and
// fetch the real Fantasy Premier League API, then `t.Skipf` when it could not
// reach it. Three things were wrong with that, and the third is the worst:
//
//   - **It was not reproducible.** Two runs of the same commit disagreed.
//     TestOptimizeRespectsExpectedMinutesFloor passed at 08:17 and failed at
//     10:05 on 2026-08-30 with no code change between them, because GW1 had
//     completed and GW2 had not, so the league carried one gameweek of evidence.
//     A test suite that is not a function of the commit is not a regression test.
//   - **It was slow and it was somebody else's server.** `t.TempDir()` gives each
//     helper an empty cache directory, so nothing was ever reused: ~124 call
//     sites across this package and internal/agent each paid a cold fetch of a
//     1.6MB bootstrap plus fixtures, from FPL's production service, on every run,
//     from a public repository's CI.
//   - ⚠️ **`t.Skipf` on an unreachable API meant the suite went GREEN when FPL was
//     down.** Every assertion below became a silent pass. That is not a flaky
//     test, it is a suite that cannot tell you it did not run.
//
// `cmd/armband/webroutes_test.go` already had the right convention and this
// follows it: a committed capture that cannot be read is a broken repository,
// not an absent dependency, so it is `t.Fatalf` and never a skip.
//
// # Two captures, because no single one carries both halves
//
// The live series (`capture.LiveCapture`) has a bootstrap AND fixtures but is
// pinned before kickoff, so `GameweeksPlayed()` is 0. The backfilled per-gameweek
// series has real accumulated minutes but, by construction, **no fixtures** —
// `internal/capture/backfilled.go` says why: a finished season's fixture list is
// already in the season archive and re-crawling it would spend somebody else's
// bandwidth. `internal/analysis` cannot reach that archive, because
// `internal/backtest` imports this package and the dependency cannot run both
// ways.
//
// So a test picks the one its subject needs, and says which.
const midSeasonCapture = "../../data/captures/2025-26/GW10-2025-11-01T1000Z"

// captureEngine is the default: complete data — bootstrap and fixtures — from the
// committed live-series capture, before a ball is kicked.
func captureEngine(t *testing.T) *Engine {
	t.Helper()
	boot, fx, err := capture.Replay("../../data/captures/" + capture.LiveCapture)
	if err != nil {
		t.Fatalf("reading the committed capture %s: %v\n\nThis capture is in the "+
			"repository. Failing to read it is a broken checkout, not a missing "+
			"dependency, and skipping would turn every assertion below into a "+
			"silent pass.", capture.LiveCapture, err)
	}
	return NewEngine(boot, fx, DefaultWeights())
}

// midSeasonEngine carries nine completed gameweeks of real accumulated minutes,
// for the tests whose subject IS accumulated evidence.
//
// ⚠️ **It has NO FIXTURES**, so `SeasonHasStarted()` reads false and anything
// depending on fixture difficulty, congestion or opponent strength is not
// meaningful on it. Use captureEngine for those. A test that needs both real
// minutes and real fixtures has no committed source today and should say so
// rather than reaching for the live API again.
func midSeasonEngine(t *testing.T) *Engine {
	t.Helper()
	raw, err := capture.Read(midSeasonCapture, capture.BootstrapEndpoint)
	if err != nil {
		t.Fatalf("reading the committed mid-season capture: %v", err)
	}
	var boot fpl.Bootstrap
	if err := json.Unmarshal(raw, &boot); err != nil {
		t.Fatalf("decoding the committed mid-season capture: %v", err)
	}
	e := NewEngine(&boot, nil, DefaultWeights())
	if e.GameweeksPlayed() < 2 {
		t.Fatalf("the mid-season capture reports %d gameweeks played; it is "+
			"supposed to carry nine, so the pin is wrong", e.GameweeksPlayed())
	}
	return e
}
