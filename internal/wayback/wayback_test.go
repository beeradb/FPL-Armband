package wayback

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestGunzippedSurvivesEitherShape pins the trap that costs an hour.
//
// The Archive replays the origin's bytes and FPL served `bootstrap-static` gzipped, so
// the response carries `Content-Encoding: gzip` whether or not the request asked for
// it. Go's transport only decompresses transparently when it added the
// `Accept-Encoding` header itself — a condition easy to change by accident three
// refactors from now — so the sniff must be on the magic number, and it must be a
// no-op on bytes that are already plain.
func TestGunzippedSurvivesEitherShape(t *testing.T) {
	plain := []byte(`{"events":[],"elements":[]}`)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if got := Gunzipped(buf.Bytes()); !bytes.Equal(got, plain) {
		t.Errorf("gzipped input came back as %q", got)
	}
	if got := Gunzipped(plain); !bytes.Equal(got, plain) {
		t.Errorf("plain input was mangled into %q", got)
	}
	// Something that starts with the magic number but is not valid gzip must come
	// back untouched rather than empty. Returning nothing here would read downstream
	// as an endpoint that served an empty body, which is a different and wrong fact.
	broken := []byte{0x1f, 0x8b, 0x00, 0x01, 0x02}
	if got := Gunzipped(broken); !bytes.Equal(got, broken) {
		t.Errorf("undecompressable input came back as %q, want it unchanged", got)
	}
	if len(Gunzipped(nil)) != 0 {
		t.Error("nil input produced output")
	}
}

// TestFetchDecompressesWhatTheArchiveActuallySends drives the real client against a
// server that behaves the way the Archive does.
func TestFetchDecompressesWhatTheArchiveActuallySends(t *testing.T) {
	plain := []byte(`{"events":[{"id":1}],"elements":[]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exactly the Archive's shape: the header is set by hand and the body is
		// compressed regardless of what the client asked for.
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		zw := gzip.NewWriter(w)
		defer zw.Close()
		_, _ = zw.Write(plain)
	}))
	defer srv.Close()

	c := New(t.TempDir())
	c.MinInterval = 0
	body, err := c.fetchURL(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, plain) {
		t.Errorf("got %q, want the decompressed original — a naive read fails here on byte 0x8b", body)
	}
}

// TestRetriesAreBoundedAndPolite pins the behaviour owed to a charity's servers: back
// off rather than hammer, and give up rather than loop.
func TestRetriesAreBoundedAndPolite(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(t.TempDir())
	c.MinInterval = 0
	c.MaxAttempts = 5
	c.BackoffBase = time.Millisecond
	body, err := c.get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("a request that succeeded on the third attempt returned %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("got %q", body)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("made %d requests, want 3", got)
	}
}

// TestAnEmptyBodyIsAnErrorNotAnAnswer pins a silent-failure guard.
//
// The Archive returns 200 with nothing under load. Caching that would poison the run
// in the worst possible way: the caller would read "this gameweek has no team news"
// rather than "the fetch failed", and a gap that is really an outage is indistinguishable
// from one that is really a gap.
func TestAnEmptyBodyIsAnErrorNotAnAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := New(dir)
	c.MinInterval = 0
	c.MaxAttempts = 2
	c.BackoffBase = time.Millisecond
	if _, err := c.cachedGet(context.Background(), srv.URL, "raw", false); err == nil {
		t.Fatal("an empty 200 was accepted as a body")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("an empty 200 left %d cache entries behind; a poisoned cache turns a "+
			"transient outage into a permanent hole", len(entries))
	}
}

// TestTheCacheIsUsedOnASecondRequest pins that a finished season is fetched once.
func TestTheCacheIsUsedOnASecondRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`[["timestamp","original","digest"]]`))
	}))
	defer srv.Close()

	c := New(t.TempDir())
	c.MinInterval = 0
	for i := 0; i < 3; i++ {
		if _, err := c.cachedGet(context.Background(), srv.URL, "cdx", false); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("made %d requests for one immutable answer, want 1", got)
	}
}

// TestRawURLAsksForTheUnrewrittenOriginal pins the `id_` marker.
//
// Without it the Archive injects its own toolbar and rewrites links, which for a JSON
// endpoint means a body that no longer parses — and the failure would look like a
// corrupt payload rather than a wrong URL.
func TestRawURLAsksForTheUnrewrittenOriginal(t *testing.T) {
	s := Snapshot{
		At:       time.Date(2021, 2, 19, 16, 39, 25, 0, time.UTC),
		Original: "https://fantasy.premierleague.com/api/bootstrap-static/",
	}
	want := "https://web.archive.org/web/20210219163925id_/" +
		"https://fantasy.premierleague.com/api/bootstrap-static/"
	if got := s.RawURL(); got != want {
		t.Errorf("RawURL() = %q, want %q", got, want)
	}

	// The timestamp must be rendered in UTC. A local-time render would name a
	// different crawl — one that may not exist, or worse, one that does.
	local := Snapshot{At: s.At.In(time.FixedZone("BST", 3600)), Original: s.Original}
	if got := local.RawURL(); got != want {
		t.Errorf("a snapshot in a non-UTC location rendered as %q", got)
	}
}

// TestIndexReadsColumnsByName pins the parse against a CDX response whose column set
// is not the default one.
//
// Reading by position is the natural thing to write and it shifts silently when the
// `fl` parameter changes: a status code lands in the digest field and the index reads
// as a plausible list of nothing.
func TestIndexReadsColumnsByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately reordered, with an extra leading column.
		_, _ = w.Write([]byte(`[
			["urlkey","digest","timestamp","original","length"],
			["com,x)/","ABCDEF","20210219163925","https://x/","1234"],
			["com,x)/","GHIJKL","not-a-timestamp","https://x/","1234"],
			["com,x)/","MNOPQR","20210220104652","https://x/","99"]
		]`))
	}))
	defer srv.Close()

	c := New(t.TempDir())
	c.MinInterval = 0
	snaps, err := c.indexFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("parsed %d snapshots, want 2 — the unparseable timestamp must be dropped, "+
			"not guessed at", len(snaps))
	}
	if snaps[0].Digest != "ABCDEF" || snaps[0].Length != 1234 {
		t.Errorf("columns were read by position: %+v", snaps[0])
	}
	if !snaps[0].At.Equal(time.Date(2021, 2, 19, 16, 39, 25, 0, time.UTC)) {
		t.Errorf("timestamp parsed as %v, want UTC", snaps[0].At)
	}
	if snaps[0].At.Location() != time.UTC {
		t.Errorf("timestamp is in %v, not UTC", snaps[0].At.Location())
	}
}

// TestDiagWaybackIsReachable is the live check, DIAG-gated like every other
// network-dependent check in this project, and it skips rather than fails when the
// Archive is unreachable.
//
//	DIAG=1 go test ./internal/wayback -run TestDiagWayback -v
//
// It asserts invariants rather than values. The Archive's crawl history for a finished
// season is immutable in principle, but a test pinned to a specific crawl would still
// rot the day indexing changes, and this project's convention is that a test naming a
// particular row is a test that will be deleted rather than fixed.
func TestDiagWaybackIsReachable(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	c := New(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	from := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2021, 1, 31, 0, 0, 0, 0, time.UTC)
	snaps, err := c.Index(ctx, "fantasy.premierleague.com/api/bootstrap-static/", from, to, false)
	if err != nil {
		t.Skipf("the Internet Archive is unreachable: %v", err)
	}
	if len(snaps) == 0 {
		t.Fatal("the index returned no crawls for January 2021, which contradicts the " +
			"coverage this backfill is built on")
	}
	for _, s := range snaps {
		if s.At.Before(from) || s.At.After(to) {
			t.Errorf("crawl at %s is outside the requested window", s.At)
		}
	}
	t.Logf("%d crawls in January 2021", len(snaps))

	body, err := c.Fetch(ctx, snaps[0])
	if err != nil {
		t.Skipf("fetching %s: %v", snaps[0].RawURL(), err)
	}
	// The one thing worth asserting about the payload: it is JSON, not gzip, by the
	// time it reaches a caller.
	if len(body) < 2 || body[0] != '{' {
		t.Fatalf("the payload does not start with '{' — it begins %q, which is what an "+
			"undecompressed gzip body looks like", body[:min(8, len(body))])
	}
	if !strings.Contains(string(body[:min(4096, len(body))]), `"events"`) {
		t.Error("the payload carries no events[], so it cannot be dated from the inside")
	}
	t.Logf("fetched %d bytes from %s", len(body), snaps[0].RawURL())
}
