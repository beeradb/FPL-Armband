package analysis

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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

// ⚠️ liveCaptureDir is HARDCODED rather than read from capture.LiveCapture, and
// that is deliberate.
//
// internal/backfill's TestTheScoringPathCannotSeeRecoveredTeamNews forbids
// internal/analysis and internal/backtest from importing armband/internal/capture
// at all — the packages that recover historical availability must not be reachable
// from the scoring path, because every figure in the record was measured with
// backtest.statusAt as it stands and wiring real availability in reprices all of
// it. The guard is source-scanning and deliberately blunt: "the failure is a new
// import in a new file". A test import is exactly how that boundary would erode,
// so this reads the committed bytes directly instead of importing the reader.
//
// The cost is a hardcoded directory name that could drift from capture.LiveCapture.
// TestTheAnalysisFixtureNamesTheLiveCapture in internal/capture pins the two
// against each other, which is this repository's standing answer to a
// cross-package duplicate it cannot delete.
const liveCaptureDir = "../../data/captures/2026-08-19T1348Z"

// readCapturedBootstrap decodes a committed capture's bootstrap without importing
// the capture package. Eight lines of gzip and JSON rather than a boundary
// violation; it duplicates a decode, not a quantity.
func readCapturedBootstrap(t *testing.T, dir string) *fpl.Bootstrap {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "bootstrap-static.json.gz"))
	if err != nil {
		t.Fatalf("opening the committed capture %s: %v\n\nIt is in the repository; "+
			"an unreadable one is a broken checkout, not a missing dependency, and "+
			"skipping would turn every assertion below into a silent pass.", dir, err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("decompressing %s: %v", dir, err)
	}
	defer zr.Close()
	var boot fpl.Bootstrap
	if err := json.NewDecoder(zr).Decode(&boot); err != nil {
		t.Fatalf("decoding %s: %v", dir, err)
	}
	return &boot
}

func readCapturedFixtures(t *testing.T, dir string) []fpl.Fixture {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "fixtures.json.gz"))
	if err != nil {
		t.Fatalf("opening the committed fixtures at %s: %v", dir, err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("decompressing fixtures at %s: %v", dir, err)
	}
	defer zr.Close()
	var fx []fpl.Fixture
	if err := json.NewDecoder(zr).Decode(&fx); err != nil {
		t.Fatalf("decoding fixtures at %s: %v", dir, err)
	}
	return fx
}

// captureEngine is the default: complete data — bootstrap and fixtures — from the
// committed live-series capture, before a ball is kicked.
func captureEngine(t *testing.T) *Engine {
	t.Helper()
	boot, fx := captureBootAndFixtures(t)
	return NewEngine(boot, fx, DefaultWeights())
}

// captureBootAndFixtures is the one place this package reads the committed
// capture, so its engine helpers cannot drift onto different data.
func captureBootAndFixtures(t *testing.T) (*fpl.Bootstrap, []fpl.Fixture) {
	t.Helper()
	return readCapturedBootstrap(t, liveCaptureDir), readCapturedFixtures(t, liveCaptureDir)
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
	boot := readCapturedBootstrap(t, midSeasonCapture)
	e := NewEngine(boot, nil, DefaultWeights())
	// ⚠️ Nine, not "at least two". The doc above promises nine gameweeks of
	// accumulated evidence and an earlier version of this guard only checked for
	// two, which would have let the pin drift anywhere in between while the
	// comment kept claiming nine.
	if got := e.GameweeksPlayed(); got != 9 {
		t.Fatalf("the mid-season capture reports %d gameweeks played, want 9; "+
			"the pin has moved and every caller's premise with it", got)
	}
	return e
}
