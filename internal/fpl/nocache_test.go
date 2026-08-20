package fpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serve stands in for the FPL API, returning one canned body for any path.
func serve(t *testing.T, body string) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	c := &Client{
		http:     srv.Client(),
		cacheDir: dir,
		cacheTTL: time.Hour,
	}
	// baseURL is a package constant, so the request has to be pointed at the test
	// server by overriding the transport rather than the URL. Rewriting the host on
	// the way out keeps the code under test — including its path handling and its
	// cache-key derivation — completely unmodified.
	c.http = &http.Client{Transport: rewrite{to: srv.URL}}
	return c, dir
}

type rewrite struct{ to string }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := req.URL.Parse(r.to)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme, req.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(req)
}

func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// TestUnpricedResponsesStillCache pins that the client actually caches.
//
// It was written as the counter-test to a no-cache guard that has since been
// deleted along with the authenticated endpoint it protected. It is kept on its
// own merit: without it, a future "do not cache X" rule could be satisfied by
// never caching anything, which would pass while quietly turning one network
// call per endpoint per run into one per tool call — and the tool runner is
// concurrent.
func TestUnpricedResponsesStillCache(t *testing.T) {
	c, dir := serve(t, `{"events": [], "elements": []}`)

	var out map[string]any
	if err := c.get(context.Background(), "/bootstrap-static/", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	files := cacheFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("expected exactly one cache file, got %v", files)
	}
	if !strings.Contains(files[0], "bootstrap-static") {
		t.Errorf("cache file is named %q, expected it to name the endpoint", files[0])
	}
	if b, err := os.ReadFile(filepath.Join(dir, files[0])); err != nil || len(b) == 0 {
		t.Errorf("cache file is empty or unreadable: %v", err)
	}
}

// TestRawRefusesAnHTMLPage stops a login page being archived as if it were data.
//
// FPL serves HTML with a 200 rather than a 401, so this is the normal shape of an
// expired session from here. Stored, it would put a file in the archive that looks
// like a capture, carries a checksum, and is not one — and the manifest would say it
// succeeded. Discovering that in 2028, mid-analysis, is the expensive version.
func TestRawRefusesAnHTMLPage(t *testing.T) {
	c, _ := serve(t, `<!DOCTYPE html><html><body>Sign in</body></html>`)
	if _, err := c.Raw(context.Background(), "/bootstrap-static/"); err == nil {
		t.Fatal("Raw accepted an HTML page")
	} else if !strings.Contains(err.Error(), "HTML") {
		t.Errorf("the error should name HTML so nobody hunts for a decoder bug, got: %v", err)
	}
}

// TestRawIsAlwaysFresh — a capture records a moment, so it must not be served from a
// cache written at some other moment.
//
// This is the distinction that makes the capture safe where the rejected external
// mirror was not: our own capture of our own live fetch, never read by the live path.
// If `Raw` could return a cached body, a capture would date itself to whenever the
// cache was written and the series would silently be a series of cache-write times.
func TestRawIsAlwaysFresh(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"n": 1}`))
	}))
	defer srv.Close()

	c := &Client{
		http:     &http.Client{Transport: rewrite{to: srv.URL}},
		cacheDir: t.TempDir(),
		cacheTTL: time.Hour,
	}

	// Warm the cache through the ordinary path, then confirm Raw ignores it.
	var out map[string]any
	if err := c.get(context.Background(), "/bootstrap-static/", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	after := hits
	if _, err := c.Raw(context.Background(), "/bootstrap-static/"); err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if hits != after+1 {
		t.Errorf("Raw served from cache (%d hits before, %d after); a capture must record "+
			"the moment it was taken, not the moment the cache was written", after, hits)
	}
}

// TestZeroTTLRefetchesEvenAFreshFile pins the mechanism the `-refresh` flag depends
// on, and by extension the mechanism the snapshot generator (the `warm` job) needs so
// that raising the SERVING ttl (cache_minutes) does not also freeze the generator.
//
// The generator and the server run the same binary against the same on-disk cache and
// the same config. If the server's cache_minutes is raised above the generator's
// refresh interval — the fix this test exists to guard — a generator that reused that
// same TTL would find its own just-written file "fresh" on its next run and stop
// fetching forever. `-refresh` (cacheTTL: 0 on the Client) is how the generator opts
// out of whatever TTL the shared config carries: a zero TTL must treat every file as
// stale regardless of how recently it was written, not merely regardless of how old it
// looks under an unrelated TTL.
//
// The three-client shape is deliberate: the SAME on-disk file is read by a normal-TTL
// client first, to prove the file really is fresh (a stale-file test would not
// distinguish "always refetches" from "TTL happened to have elapsed").
func TestZeroTTLRefetchesEvenAFreshFile(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"events": [], "elements": []}`))
	}))
	defer srv.Close()
	httpClient := &http.Client{Transport: rewrite{to: srv.URL}}
	dir := t.TempDir()

	seed := &Client{http: httpClient, cacheDir: dir, cacheTTL: time.Hour}
	var out map[string]any
	if err := seed.get(context.Background(), "/bootstrap-static/", &out); err != nil {
		t.Fatalf("seeding the cache: %v", err)
	}
	if hits != 1 {
		t.Fatalf("seeding the cache took %d network hits, want 1", hits)
	}

	// Control: an ordinary client with a generous TTL must serve the file we just
	// wrote from cache, without a second network round trip. This is what confirms
	// the file is genuinely "fresh" — the case that must NOT fool a zero-TTL client.
	fresh := &Client{http: httpClient, cacheDir: dir, cacheTTL: time.Hour}
	if err := fresh.get(context.Background(), "/bootstrap-static/", &out); err != nil {
		t.Fatalf("fresh-TTL client: %v", err)
	}
	if hits != 1 {
		t.Fatalf("fresh-TTL client hit the network (%d total hits, want 1) reading a file it "+
			"just wrote; the test setup is broken, not the client under test", hits)
	}

	// The generator's client: same directory, same file, zero TTL. It must refetch
	// regardless of the file's age.
	refresh := &Client{http: httpClient, cacheDir: dir, cacheTTL: 0}
	if err := refresh.get(context.Background(), "/bootstrap-static/", &out); err != nil {
		t.Fatalf("zero-TTL client: %v", err)
	}
	if hits != 2 {
		t.Errorf("zero-TTL client served a fresh file from cache (%d total network hits, want "+
			"2); a generator run with -refresh would freeze the shared archive exactly like "+
			"an ordinary reader once the serving cache_minutes is raised above its own "+
			"refresh interval", hits)
	}
}

// TestTheClientHasNoAuthenticatedSurface pins a deletion rather than a feature.
//
// The FPL session cookie, the my-team endpoint and the `armband auth` command
// were removed outright. They existed for one reason, stated in the file that is
// now gone: a future write path — setting a lineup, making transfers, playing a
// chip — which are POSTs with no public equivalent. Selling prices never needed
// them; those are reconstructed from public data and checked against FPL's own
// team value, which is both credential-free and verifiable.
//
// Deleting it removed three standing security findings at once: a cache guard
// that failed OPEN for any authenticated endpoint someone might add later, a
// cookie attached to every request rather than the one that needed it, and the
// pre-conditions owed before any POST could ship. None of those can regress
// while there is no credential to mishandle.
//
// So this test guards an ABSENCE, which is the only way an absence stays true.
// If a write path is ever wanted it should be built from scratch against a
// threat model, not by reviving this — and the first thing to do is delete this
// test deliberately, which is the point.
func TestTheClientHasNoAuthenticatedSurface(t *testing.T) {
	src, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatalf("read client.go: %v", err)
	}
	for _, banned := range []string{
		"Session",     // the cookie field
		"authCookie",  // attaching it
		"my-team",     // the only endpoint that needed it
		"pl_profile",  // the cookie name
		"csrftoken",   // what a write path would have needed next
		"X-CSRFToken", //
		"http.MethodPost",
	} {
		if strings.Contains(string(src), banned) {
			t.Errorf("client.go mentions %q. The authenticated surface was removed "+
				"deliberately; re-adding one is a decision that needs a threat model "+
				"and a fresh review, not a quiet reintroduction.", banned)
		}
	}

	// And nothing else in the package should carry it either.
	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range entries {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), "http.MethodPost") {
			t.Errorf("%s issues a POST. This client is read-only against a public API; "+
				"a write path is out of scope until it is designed with one.", f)
		}
	}
}
