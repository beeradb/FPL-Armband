package main

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"armband/internal/config"
)

// The guard is the reason this command exists, so it is the thing that must be
// tested. Its first version checked only whether the target gameweek was
// `finished`, which misses the window that actually leaks: after the deadline and
// before the gameweek ends, where `finished` is still false and the payload
// already carries post-deadline team news and price moves.
func TestAsOfRefusesACaptureTakenAfterTheDeadline(t *testing.T) {
	dir := t.TempDir()
	deadline := time.Date(2026, 8, 28, 17, 30, 0, 0, time.UTC)
	writeCapture(t, dir, 2, deadline, deadline.Add(5*time.Hour), false)

	err := cmdAsOf(t.Context(), testConfig(t), []string{dir})
	if err == nil {
		t.Fatal("a capture taken five hours AFTER the deadline was accepted; " +
			"it carries team news the manager did not have")
	}
	if !strings.Contains(err.Error(), "not strictly before") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

func TestAsOfAcceptsACaptureTakenBeforeTheDeadline(t *testing.T) {
	dir := t.TempDir()
	deadline := time.Date(2026, 8, 28, 17, 30, 0, 0, time.UTC)
	writeCapture(t, dir, 2, deadline, deadline.Add(-6*time.Hour), false)

	// It will fail later for want of a real bootstrap, but it must not fail on
	// the guard — that is what distinguishes "refused as hindsight" from
	// "refused as unreadable".
	err := cmdAsOf(t.Context(), testConfig(t), []string{dir})
	if err != nil && strings.Contains(err.Error(), "not usable as point-in-time evidence") {
		t.Fatalf("a capture six hours BEFORE the deadline was refused as hindsight: %v", err)
	}
}

// The recency refusal, which is the second of the two divergences from the live
// path and was the one nothing tested.
//
// A capture carries no per-player match history, so an as-of run cannot build the
// index cmd/armband builds from element-summary. Below two gameweeks played that
// costs nothing — internal/backtest asserts it is a bit-exact no-op on all six
// archived seasons — and at two it moves the starting XI. The command must run in
// the first regime and refuse in the second, or it silently scores a different
// model than the engine it exists to be compared against.
func TestAsOfRefusesACaptureWithTwoGameweeksPlayed(t *testing.T) {
	dir := t.TempDir()
	deadline := time.Date(2026, 9, 4, 17, 30, 0, 0, time.UTC)
	writeCaptureWithPlayed(t, dir, 3, deadline, deadline.Add(-6*time.Hour), 2)

	err := cmdAsOf(t.Context(), testConfig(t), []string{dir})
	if err == nil {
		t.Fatal("a capture with two gameweeks played was accepted; the live path " +
			"would have wired a recency index here and this cannot build one")
	}
	if !strings.Contains(err.Error(), "gameweeks played") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// The other side of the same cutoff: one gameweek played must NOT be refused, or
// the command refuses the captures it was built for. The GW2 tiebreak capture —
// the only thing this command has been used for — sits exactly here.
func TestAsOfAcceptsACaptureWithOneGameweekPlayed(t *testing.T) {
	dir := t.TempDir()
	deadline := time.Date(2026, 8, 28, 17, 30, 0, 0, time.UTC)
	writeCaptureWithPlayed(t, dir, 2, deadline, deadline.Add(-6*time.Hour), 1)

	err := cmdAsOf(t.Context(), testConfig(t), []string{dir})
	if err != nil && strings.Contains(err.Error(), "gameweeks played") {
		t.Fatalf("a capture with ONE gameweek played was refused, but recency is a "+
			"measured no-op there: %v", err)
	}
}

// prior_half_life is the first divergence, and it was equally untested. It ships
// at 0, so the two paths are equivalent with the deployed config; the refusal
// exists for whoever turns it on, which is precisely the case no one would notice
// breaking.
func TestAsOfRefusesWhenPriorHalfLifeWouldBlendMultipleSeasons(t *testing.T) {
	dir := t.TempDir()
	deadline := time.Date(2026, 8, 28, 17, 30, 0, 0, time.UTC)
	writeCaptureWithPlayed(t, dir, 2, deadline, deadline.Add(-6*time.Hour), 1)

	cfg := testConfig(t)
	cfg.Weights.PriorHalfLife = 1.5

	err := cmdAsOf(t.Context(), cfg, []string{dir})
	if err == nil {
		t.Fatal("prior_half_life was set and the run was accepted; the live path " +
			"would blend several prior seasons through a client this does not have")
	}
	if !strings.Contains(err.Error(), "prior_half_life") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// writeCaptureWithPlayed writes a capture whose payload has `played` gameweeks
// already finished before the target event, so GameweeksPlayed() reads what the
// recency cutoff is tested against.
func writeCaptureWithPlayed(t *testing.T, dir string, event int, deadline, at time.Time, played int) {
	t.Helper()
	var events []map[string]any
	for i := 1; i < event; i++ {
		events = append(events, map[string]any{
			"id":            i,
			"deadline_time": deadline.Add(time.Duration(i-event) * 7 * 24 * time.Hour).Format(time.RFC3339),
			"finished":      i <= played,
			"data_checked":  i <= played,
		})
	}
	events = append(events, map[string]any{
		"id": event, "deadline_time": deadline.Format(time.RFC3339),
		"finished": false, "is_next": true,
	})
	elements := []map[string]any{{"id": 1}}
	body, err := json.Marshal(map[string]any{"elements": elements, "events": events})
	if err != nil {
		t.Fatal(err)
	}
	writeGzForTest(t, filepath.Join(dir, "bootstrap-static.json.gz"), body)
	// A live-series capture, not a backfilled one: these tests exercise the path
	// past capture.Replay, which reads both payloads. Without a fixtures file each
	// would fail on the missing file and satisfy its own assertion for the wrong
	// reason — which is how the first draft of them passed.
	writeGzForTest(t, filepath.Join(dir, "fixtures.json.gz"), []byte("[]"))
	m := map[string]any{"captured_at": at.Format(time.RFC3339), "event": event,
		"files": []map[string]any{
			{"endpoint": "/bootstrap-static/", "name": "bootstrap-static.json.gz"},
			{"endpoint": "/fixtures/", "name": "fixtures.json.gz"},
		}}
	mb, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCapture(t *testing.T, dir string, event int, deadline, at time.Time, finished bool) {
	t.Helper()
	boot := map[string]any{"elements": []map[string]any{{"id": 1}}, "events": []map[string]any{
		{"id": event - 1, "deadline_time": deadline.Add(-7 * 24 * time.Hour).Format(time.RFC3339), "finished": true},
		{"id": event, "deadline_time": deadline.Format(time.RFC3339), "finished": finished, "is_next": !finished},
	}}
	body, err := json.Marshal(boot)
	if err != nil {
		t.Fatal(err)
	}
	writeGzForTest(t, filepath.Join(dir, "bootstrap-static.json.gz"), body)
	m := map[string]any{"captured_at": at.Format(time.RFC3339), "event": event,
		"files": []map[string]any{{"endpoint": "/bootstrap-static/", "name": "bootstrap-static.json.gz"}}}
	mb, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGzForTest(t *testing.T, path string, body []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := gzip.NewWriter(f)
	if _, err := zw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// testConfig is deliberately empty: the guard runs before anything reads weights,
// so a test of the guard must not depend on a config file existing.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{}
}
