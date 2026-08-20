package fpl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// snapshotKey mirrors get()'s cache-key derivation for a simple path, so tests
// can write directly into an overlay or snapshot directory without going
// through the network.
const snapshotTestPath = "/bootstrap-static/"

func snapshotKeyFile(dir string) string {
	return filepath.Join(dir, "bootstrap-static.json")
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// dialErrorTransport fails any request rather than dialling out, so a test can
// prove "no network call happened" by asserting get() still succeeded.
type dialErrorTransport struct{}

func (dialErrorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("no network call should have been made")
}

// TestOverlayPreferredOverSnapshot pins the read order: when both the overlay
// (cacheDir) and the read-only snapshot have fresh content for the same key,
// the overlay wins. The overlay is where a process's own -refresh writes and
// any live fetch land, so it must always be at least as fresh as the snapshot.
func TestOverlayPreferredOverSnapshot(t *testing.T) {
	overlayDir, snapDir := t.TempDir(), t.TempDir()
	mustWrite(t, snapshotKeyFile(overlayDir), `{"events":[],"elements":[],"from":"overlay"}`)
	mustWrite(t, snapshotKeyFile(snapDir), `{"events":[],"elements":[],"from":"snapshot"}`)

	c := &Client{
		http:        &http.Client{Transport: dialErrorTransport{}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Hour,
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "overlay" {
		t.Fatalf("expected the overlay's content, got %q", out.From)
	}
}

// TestSnapshotUsedWhenOverlayEmpty pins that a read-only snapshot is actually
// consulted, and that consulting it means no live fetch happens — a snapshot
// hit must save the network call, not merely agree with it.
func TestSnapshotUsedWhenOverlayEmpty(t *testing.T) {
	overlayDir, snapDir := t.TempDir(), t.TempDir()
	mustWrite(t, snapshotKeyFile(snapDir), `{"events":[],"elements":[],"from":"snapshot"}`)

	c := &Client{
		// A transport that panics on any dial: if the client fell through to a
		// live fetch instead of reading the snapshot, this test would fail loudly
		// rather than silently accepting whatever a real network call returned.
		http:        &http.Client{Transport: dialErrorTransport{}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Hour,
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "snapshot" {
		t.Fatalf("expected the snapshot's content, got %q", out.From)
	}
}

// TestASnapshotMissFallsThroughToALiveFetch is the "fallback must not fail in
// the dangerous direction" regression: a snapshot that has nothing for this key
// (not yet generated, or a deployment that doesn't use one) must degrade to a
// live fetch, never to a wrong or empty answer served as if it were real data.
func TestASnapshotMissFallsThroughToALiveFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"elements":[],"from":"live"}`))
	}))
	t.Cleanup(srv.Close)

	overlayDir, snapDir := t.TempDir(), t.TempDir()
	// snapDir exists but has no file for this key — the miss case.

	c := &Client{
		http:        &http.Client{Transport: rewrite{to: srv.URL}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Hour,
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "live" {
		t.Fatalf("a snapshot miss must fall through to a live fetch, got %q", out.From)
	}
}

// TestWritesAlwaysLandInTheOverlay pins that a live fetch is only ever cached
// into cacheDir, never into snapshotDir — the snapshot is read-only from this
// process's side by construction; only the deployment's generator writes there.
func TestWritesAlwaysLandInTheOverlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"elements":[]}`))
	}))
	t.Cleanup(srv.Close)

	overlayDir, snapDir := t.TempDir(), t.TempDir()

	c := &Client{
		http:        &http.Client{Transport: rewrite{to: srv.URL}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Hour,
	}

	var out map[string]any
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}

	if _, err := os.Stat(snapshotKeyFile(overlayDir)); err != nil {
		t.Errorf("expected a cache file in the overlay: %v", err)
	}
	if _, err := os.Stat(snapshotKeyFile(snapDir)); err == nil {
		t.Errorf("a fetch must never write into snapshotDir, but a file appeared there")
	}
}

// TestZeroTTLBypassesBothOverlayAndSnapshot is the direct regression test for
// -refresh (ttl=0): even when both the overlay and the snapshot have files with
// a fresh mtime and valid content, a zero TTL must still live-fetch, and the
// freshly-fetched body must still land in the overlay, the only writable
// location.
func TestZeroTTLBypassesBothOverlayAndSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"elements":[],"from":"live"}`))
	}))
	t.Cleanup(srv.Close)

	overlayDir, snapDir := t.TempDir(), t.TempDir()
	mustWrite(t, snapshotKeyFile(overlayDir), `{"events":[],"elements":[],"from":"overlay"}`)
	mustWrite(t, snapshotKeyFile(snapDir), `{"events":[],"elements":[],"from":"snapshot"}`)

	c := &Client{
		http:        &http.Client{Transport: rewrite{to: srv.URL}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    0, // what -refresh produces
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "live" {
		t.Fatalf("a zero TTL must bypass both overlay and snapshot and live-fetch, got %q", out.From)
	}

	b, err := os.ReadFile(snapshotKeyFile(overlayDir))
	if err != nil {
		t.Fatalf("the freshly-fetched body must be written to the overlay: %v", err)
	}
	if !strings.Contains(string(b), "live") {
		t.Errorf("the overlay file was not overwritten with the live-fetched body: %s", b)
	}
	// The snapshot must remain untouched.
	b, err = os.ReadFile(snapshotKeyFile(snapDir))
	if err != nil {
		t.Fatalf("the snapshot file should still exist: %v", err)
	}
	if !strings.Contains(string(b), "snapshot") {
		t.Errorf("the snapshot file must never be rewritten, got: %s", b)
	}
}

// TestNewWithSnapshotResolvesTheSymlinkOnceNotPerRead pins the mechanism the
// deployment depends on: snapshotDir is typically `.../archive/current`, a
// symlink a generator process repoints to a new immutable directory
// periodically. NewWithSnapshot must resolve it to a concrete target ONCE, at
// construction — so a live process cannot see a later generator run's output
// mid-life, matching "the engine is built once, only a restart unfreezes it".
func TestNewWithSnapshotResolvesTheSymlinkOnceNotPerRead(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, "a")
	dirB := filepath.Join(root, "b")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, snapshotKeyFile(dirA), `{"events":[],"elements":[],"from":"A"}`)
	mustWrite(t, snapshotKeyFile(dirB), `{"events":[],"elements":[],"from":"B"}`)

	current := filepath.Join(root, "current")
	if err := os.Symlink(dirA, current); err != nil {
		t.Fatal(err)
	}

	c := NewWithSnapshot(t.TempDir(), current, time.Hour)
	// Prevent any accidental live fetch from masking a wrong read.
	c.http = &http.Client{Transport: dialErrorTransport{}}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "A" {
		t.Fatalf("expected A's content on first read, got %q", out.From)
	}

	// Repoint the symlink to B without reconstructing the client.
	if err := os.Remove(current); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dirB, current); err != nil {
		t.Fatal(err)
	}

	out.From = ""
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get (after repoint): %v", err)
	}
	if out.From != "A" {
		t.Fatalf("expected the client to keep reading A after a later repoint (resolved once at "+
			"construction), got %q", out.From)
	}
}

// TestASnapshotSymlinkThatDoesNotExistYetIsDisabledForTheProcessLifetime is the
// regression test for a bug review caught in this same change: NewWithSnapshot
// used to fall back to the LITERAL, unresolved path when EvalSymlinks failed —
// the exact case named in its own doc comment, "does not exist yet". That
// looked harmless (every snapshot read already tolerates a missing path, so it
// just fell through to a live fetch) but was not: os.Stat and os.ReadFile
// re-resolve a symlink component on every call, so once the path later starts
// existing — the plausible cold-start race where a reader pod starts before
// the deployment's generator has published its first "current" symlink — the
// client would silently start tracking every later repoint instead of staying
// frozen to one generation, defeating the entire point of resolving once.
//
// The fix disables the snapshot for the process's whole remaining lifetime
// when it cannot be resolved at construction, rather than reintroducing
// per-read resolution. This test constructs against a path that does not
// exist yet, confirms the miss falls through to a live fetch as before, THEN
// creates the symlink pointing at real content and confirms the client still
// does not pick it up — proving the snapshot stayed disabled rather than
// starting to resolve per read.
func TestASnapshotSymlinkThatDoesNotExistYetIsDisabledForTheProcessLifetime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"elements":[],"from":"live"}`))
	}))
	t.Cleanup(srv.Close)

	root := t.TempDir()
	current := filepath.Join(root, "current") // does not exist at construction

	c := NewWithSnapshot(t.TempDir(), current, time.Hour)
	c.http = &http.Client{Transport: rewrite{to: srv.URL}}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "live" {
		t.Fatalf("a snapshot path that doesn't exist yet must fall through to a live fetch, got %q", out.From)
	}

	// Now the generator "publishes" its first snapshot: the symlink starts
	// existing, pointing at real, fresh content.
	dirA := filepath.Join(root, "a")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, snapshotKeyFile(dirA), `{"events":[],"elements":[],"from":"snapshot"}`)
	if err := os.Symlink(dirA, current); err != nil {
		t.Fatal(err)
	}

	// If the client had reintroduced per-read resolution, it would now read the
	// snapshot's "from":"snapshot" body off disk instead of fetching live — the
	// two outcomes are observably different regardless of transport, since a
	// snapshot read never dials out at all.
	out.From = ""
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get (after the symlink appeared): %v", err)
	}
	if out.From != "live" {
		t.Fatalf("the snapshot must stay disabled for this process's lifetime once construction "+
			"failed to resolve it — got %q, which means a later-appearing symlink was picked up "+
			"mid-run", out.From)
	}
}

// failingTransport fails every request without dialling out, so a test can
// force get()'s live-fetch branch to fail deterministically.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("simulated FPL outage")
}

// TestALiveFetchFailureFallsBackToTheStaleSnapshot is the direct regression
// test for the defect this change fixes: previously, a live-fetch failure
// with nothing FRESH cached returned an error outright, which is what made
// `armband serve` CrashLoop during an FPL outage once its cache aged out.
func TestALiveFetchFailureFallsBackToTheStaleSnapshot(t *testing.T) {
	overlayDir, snapDir := t.TempDir(), t.TempDir()
	mustWrite(t, snapshotKeyFile(snapDir), `{"from":"stale-snapshot"}`)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(snapshotKeyFile(snapDir), old, old); err != nil {
		t.Fatal(err)
	}

	c := &Client{
		http:        &http.Client{Transport: failingTransport{}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Minute,
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v, want the stale fallback to succeed", err)
	}
	if out.From != "stale-snapshot" {
		t.Fatalf("expected the stale snapshot's content, got %q", out.From)
	}
	if !c.StaleServing() {
		t.Error("StaleServing() should report true after a stale fallback")
	}
	if c.StaleAgeSeconds() < 3600 {
		t.Errorf("StaleAgeSeconds() = %d, want at least the ~2h backdate", c.StaleAgeSeconds())
	}
	if c.LiveFetchFailures() != 1 {
		t.Errorf("LiveFetchFailures() = %d, want 1", c.LiveFetchFailures())
	}
}

// TestALiveFetchFailureFallsBackToTheStaleOverlayWhenNoSnapshotIsConfigured
// covers the local, non-deployment shape of this client: no snapshotDir at
// all, only its own overlay.
func TestALiveFetchFailureFallsBackToTheStaleOverlayWhenNoSnapshotIsConfigured(t *testing.T) {
	overlayDir := t.TempDir()
	mustWrite(t, snapshotKeyFile(overlayDir), `{"from":"stale-overlay"}`)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(snapshotKeyFile(overlayDir), old, old); err != nil {
		t.Fatal(err)
	}

	c := &Client{
		http:     &http.Client{Transport: failingTransport{}},
		cacheDir: overlayDir,
		cacheTTL: time.Minute,
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "stale-overlay" {
		t.Fatalf("expected the stale overlay's content, got %q", out.From)
	}
	if !c.StaleServing() {
		t.Error("StaleServing() should report true")
	}
}

// TestStaleSnapshotIsPreferredOverStaleOverlay pins the fallback ORDER: the
// snapshot is what a healthy generator most recently published, and is tried
// before this process's own overlay.
func TestStaleSnapshotIsPreferredOverStaleOverlay(t *testing.T) {
	overlayDir, snapDir := t.TempDir(), t.TempDir()
	mustWrite(t, snapshotKeyFile(overlayDir), `{"from":"stale-overlay"}`)
	mustWrite(t, snapshotKeyFile(snapDir), `{"from":"stale-snapshot"}`)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(snapshotKeyFile(overlayDir), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(snapshotKeyFile(snapDir), old, old); err != nil {
		t.Fatal(err)
	}

	c := &Client{
		http:        &http.Client{Transport: failingTransport{}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Minute,
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.From != "stale-snapshot" {
		t.Fatalf("expected the stale SNAPSHOT to win over the stale overlay, got %q", out.From)
	}
}

// TestNoFallbackAvailableAnywhereStillReturnsTheLiveFetchError is the "must
// not fail in the dangerous direction" bound on the other side: a totally
// cold cache (nothing on disk at all, e.g. a fresh archive plus an FPL
// outage) must still error rather than serve empty or wrong data.
func TestNoFallbackAvailableAnywhereStillReturnsTheLiveFetchError(t *testing.T) {
	overlayDir, snapDir := t.TempDir(), t.TempDir()
	// Nothing written anywhere.

	c := &Client{
		http:        &http.Client{Transport: failingTransport{}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Minute,
	}

	var out struct {
		From string `json:"from"`
	}
	err := c.get(context.Background(), snapshotTestPath, &out)
	if err == nil {
		t.Fatal("expected an error when nothing exists anywhere to fall back to")
	}
	if c.StaleServing() {
		t.Error("StaleServing() should stay false — nothing was actually served")
	}
	if c.LiveFetchFailures() != 1 {
		t.Errorf("LiveFetchFailures() = %d, want 1", c.LiveFetchFailures())
	}
}

// TestAFreshReadClearsTheStaleGaugeAfterFPLRecovers checks that the "most
// recent call" gauge is not sticky: once a later call succeeds via a live
// fetch, StaleServing must go back to false.
func TestAFreshReadClearsTheStaleGaugeAfterFPLRecovers(t *testing.T) {
	overlayDir, snapDir := t.TempDir(), t.TempDir()
	mustWrite(t, snapshotKeyFile(snapDir), `{"from":"stale"}`)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(snapshotKeyFile(snapDir), old, old); err != nil {
		t.Fatal(err)
	}

	c := &Client{
		http:        &http.Client{Transport: failingTransport{}},
		cacheDir:    overlayDir,
		snapshotDir: snapDir,
		cacheTTL:    time.Minute,
	}

	var out struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), snapshotTestPath, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !c.StaleServing() {
		t.Fatal("expected StaleServing() to be true after the simulated outage")
	}

	// FPL "recovers": swap in a working transport and read a DIFFERENT key, so
	// the fresh-checks for the FIRST key cannot short-circuit the live fetch
	// and mask the gauge reset.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"from":"live"}`))
	}))
	t.Cleanup(srv.Close)
	c.http = &http.Client{Transport: rewrite{to: srv.URL}}

	var out2 struct {
		From string `json:"from"`
	}
	if err := c.get(context.Background(), "/fixtures/", &out2); err != nil {
		t.Fatalf("get (recovery): %v", err)
	}
	if c.StaleServing() {
		t.Error("expected StaleServing() to be false again once a live fetch succeeds")
	}
}
