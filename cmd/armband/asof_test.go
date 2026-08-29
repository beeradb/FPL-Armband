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
