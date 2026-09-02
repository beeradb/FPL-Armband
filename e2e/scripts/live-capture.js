// The one JS spelling of internal/capture.LiveCapture.
//
// This project's signature failure is one quantity with two implementations, and this file
// IS the second one -- a JS harness cannot import a Go constant. So this name is pinned
// against the Go source by internal/capture/analysisfixture_test.go
// (TestTheAnalysisFixtureNamesTheLiveCapture, extended to scan this file too, not only
// internal/analysis/capturetestengine_test.go). Update both together, or a capture rename
// on the Go side silently leaves this suite priming a directory that no longer exists.
//
// GW1 of 2026-27, captured 2026-08-19T13:48Z: the engine has no prior-season blend and no
// recency index at this point in the season, so what this suite drives is what production
// actually serves at GW1 -- not an approximation of a later gameweek. See
// internal/capture/replay.go's own comment on LiveCapture for the full reasoning, including
// why the import card and the transfer planner are unreachable under it (e2e/README.md).
const LIVE_CAPTURE = '2026-08-19T1348Z';

module.exports = { LIVE_CAPTURE };
