// Package wayback reads archived HTTP responses out of the Internet Archive.
//
// # Why this exists
//
// The season archive this project replays is an end-of-season photograph: a player
// injured from September to November finishes the season fit and looks healthy in
// every weekly row. `backtest.statusAt` reconstructs what it can from the final
// status plus its timestamp, which is one-directional — it can carry a
// season-ending absence backwards and can say nothing at all about one that
// resolved. The absences that resolve are the population every rotation-risk
// constant needs.
//
// FPL published the answer at the time and nobody kept it. The Internet Archive
// did, in the form of crawls of `bootstrap-static`, whose `status`,
// `chance_of_playing_next_round`, `news` and `news_added` fields are exactly the
// team news a manager saw. This package is the narrow client that gets them back.
//
// # Why it is its own package, and why it does not touch internal/fpl
//
// `internal/fpl` is the live client and its cache is a *cache*: entries expire and
// a stale one is a bug. What comes back from here is the opposite — an immutable
// record of a moment that has already passed, which can be cached forever and must
// never be served to the live path. Sharing a cache directory shape between the two
// would invite exactly that confusion, so the two do not share code.
//
// # Politeness
//
// This is someone else's free infrastructure and the CDX index in particular is
// slow — a season-wide query has taken anywhere from 8 to 40 seconds. Every request
// made here is serialised behind a minimum interval, retried with exponential
// backoff, and cached aggressively enough that a second run of the same backfill
// makes no requests at all.
package wayback

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TimestampLayout is how the Wayback Machine writes an instant: UTC, to the second,
// no separators. Both the CDX index and the replay URL use it.
const TimestampLayout = "20060102150405"

// cdxEndpoint is the index. HTTPS, not HTTP.
//
// The plaintext form works and was what this used first. It is the wrong choice for a
// reason specific to this package: the index names the URL that every payload fetch
// then goes to, so anyone able to rewrite the index in flight chooses which pages get
// archived into this repository as evidence. The payload leg being HTTPS does not
// help — the Archive faithfully serves whatever page it was pointed at, and its
// save-page-now endpoint is open to anyone, so a crafted index turns a genuine TLS
// connection into a delivery mechanism for authored data.
const cdxEndpoint = "https://web.archive.org/cdx/search/cdx"

// maxBody caps a single response.
//
// **Asserted, at roughly forty times the largest real payload** — bootstrap-static
// runs about 1.5 MB uncompressed and the largest body stored here is under 2 MB. It is
// not a tuned value; it exists because both reads below are `io.ReadAll` into memory,
// and without it a compromised or rewritten response is bounded only by the machine.
const maxBody = 64 << 20

// Snapshot is one archived crawl of one URL.
//
// `At` is when the Archive's crawler fetched the page, which is the field the whole
// point-in-time argument rests on. Content can be slightly *older* than that — the
// origin sat behind a CDN with a five-minute cache — but it can never be newer, so
// a snapshot taken before a deadline cannot contain team news from after it. The
// error is one-directional and in the safe direction.
type Snapshot struct {
	At       time.Time `json:"at"`
	Original string    `json:"original"` // the URL as crawled
	Digest   string    `json:"digest"`   // the Archive's content hash, for dedup
	Length   int       `json:"length"`   // compressed bytes in the WARC, not the body
}

// RawURL is where the unrewritten original body lives.
//
// The `id_` suffix is load-bearing: without it the Archive serves the page with its
// own toolbar and rewritten links injected, which for a JSON endpoint means a body
// that no longer parses. With it the bytes are what the origin sent.
func (s Snapshot) RawURL() string {
	return fmt.Sprintf("https://web.archive.org/web/%sid_/%s",
		s.At.UTC().Format(TimestampLayout), s.Original)
}

// Client fetches from the Internet Archive, politely.
type Client struct {
	http     *http.Client
	cacheDir string

	// MinInterval is the floor on the gap between two requests. Requests are
	// serialised through `last` rather than rate-limited by a token bucket,
	// because there is no burst worth allowing here: the work is a few hundred
	// fetches that nobody is waiting on.
	MinInterval time.Duration

	// MaxAttempts counts the first try. The Archive returns 429 and 503 under
	// load often enough that one retry is not enough and rare enough that five
	// attempts have always been plenty.
	MaxAttempts int

	// BackoffBase is the first retry delay; each further attempt doubles it. A
	// field rather than a constant so a test can exercise the retry path without
	// spending twelve seconds asleep — the behaviour under test is "how many
	// requests", not "how long between them", and the two are separable.
	BackoffBase time.Duration

	// mu guards last, so the politeness floor is a property of the TYPE rather than
	// of there happening to be one caller today. The obvious next change here is to
	// fan a few hundred fetches through an errgroup, which without this is both a
	// data race and a silent removal of the rate limit — and this project records an
	// unguarded map build as a *fatal* concurrent write that took down a live run.
	mu   sync.Mutex
	last time.Time
}

// New builds a client caching into cacheDir.
//
// The timeout is generous because the CDX index genuinely is slow — a season-wide
// query returning a few hundred rows has taken 40 seconds — and a timeout that
// fires on a healthy-but-slow index would look exactly like an empty season, which
// is the failure this package least wants to be capable of.
func New(cacheDir string) *Client {
	return &Client{
		http:        &http.Client{Timeout: 5 * time.Minute},
		cacheDir:    cacheDir,
		MinInterval: 1500 * time.Millisecond,
		MaxAttempts: 5,
		BackoffBase: 2 * time.Second,
	}
}

// Index lists every archived crawl of target between from and to.
//
// `target` is a bare URL with no scheme — `fantasy.premierleague.com/api/bootstrap-static/`
// — because that is the shape CDX matches on.
//
// Two query parameters are doing real work. `filter=statuscode:200` drops the
// redirects and error pages, which are archived too and whose bodies are not the
// payload. `collapse=digest` drops a crawl whose content is byte-identical to the
// one before it; on this endpoint that has never yet fired, because
// `selected_by_percent` and the transfer counters move continuously, but it costs
// nothing and it means a duplicate can never be presented as independent evidence.
//
// The result is cached forever rather than for a TTL. A finished season's crawl
// history is immutable — the Archive cannot retroactively crawl 2020 — so a TTL
// here would only buy repeated slow requests for an answer that cannot change.
// `refresh` exists for the case where that reasoning is wrong.
func (c *Client) Index(ctx context.Context, target string, from, to time.Time, refresh bool) ([]Snapshot, error) {
	q := url.Values{}
	q.Set("url", target)
	q.Set("from", from.UTC().Format("20060102"))
	q.Set("to", to.UTC().Format("20060102"))
	q.Set("output", "json")
	q.Set("filter", "statuscode:200")
	q.Set("collapse", "digest")
	snaps, err := c.indexFromFor(ctx, cdxEndpoint+"?"+q.Encode(), target, refresh)
	if err != nil {
		return nil, fmt.Errorf("querying the CDX index for %s: %w", target, err)
	}
	return snaps, nil
}

// indexFrom fetches and parses one CDX response.
//
// Split from Index so the parse can be driven against a stub serving a column layout
// the live endpoint does not currently use. That is worth the extra function: the
// failure this guards against is a *silent* one, and it cannot be provoked against the
// real index at all.
func (c *Client) indexFrom(ctx context.Context, endpoint string, refresh ...bool) ([]Snapshot, error) {
	return c.indexFromFor(ctx, endpoint, "", refresh...)
}

// indexFromFor is indexFrom plus the resource every row is required to name.
func (c *Client) indexFromFor(ctx context.Context, endpoint, want string, refresh ...bool) ([]Snapshot, error) {
	var again bool
	if len(refresh) > 0 {
		again = refresh[0]
	}
	body, err := c.cachedGet(ctx, endpoint, "cdx", again)
	if err != nil {
		return nil, err
	}

	// CDX with output=json returns an array of arrays whose FIRST row names the
	// columns. Reading by name rather than by position is not fussiness: the
	// column set changes with the `fl` parameter, and a positional reader that
	// silently shifts by one would put a status code in the digest field and
	// report a plausible-looking index of nothing.
	var rows [][]string
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("the CDX index returned something that is not its "+
			"usual array-of-arrays: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil // no captures at all in the window; the caller decides if that is fatal
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	need := []string{"timestamp", "original", "digest"}
	for _, n := range need {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("the CDX index has no %q column; it returned %v", n, rows[0])
		}
	}

	out := make([]Snapshot, 0, len(rows)-1)
	for _, r := range rows[1:] {
		if len(r) <= col["digest"] {
			continue
		}
		at, err := time.ParseInLocation(TimestampLayout, r[col["timestamp"]], time.UTC)
		if err != nil {
			// A row whose timestamp does not parse is not a row whose timestamp
			// can be guessed at, and this package's entire safety property is
			// that the timestamp is trustworthy. Drop it rather than repair it.
			continue
		}
		s := Snapshot{At: at, Original: r[col["original"]], Digest: r[col["digest"]]}
		if want != "" && !sameResource(s.Original, want) {
			// The index names the URL the payload fetch will go to, so a row
			// pointing somewhere else is either a malformed query or somebody
			// choosing what lands in this repository. Neither is worth following.
			continue
		}
		if i, ok := col["length"]; ok && len(r) > i {
			s.Length, _ = strconv.Atoi(r[i])
		}
		out = append(out, s)
	}
	return out, nil
}

// Fetch returns the original body of one snapshot, decompressed.
//
// # The gzip trap
//
// The Archive replays the origin's bytes, and FPL served `bootstrap-static`
// gzipped. So the response carries `Content-Encoding: gzip` whether or not the
// request asked for it, and whether or not the HTTP client would normally
// transparently decode. Go's transport only auto-decompresses when it added the
// `Accept-Encoding` header itself, which is a condition that is easy to change by
// accident three refactors from now.
//
// So this sniffs the gzip magic number instead of trusting either the header or the
// transport. That is the version that is correct under every combination, and the
// symptom it prevents — a JSON parse failing on byte 0x8b — is obscure enough to
// cost an hour.
func (c *Client) Fetch(ctx context.Context, s Snapshot) ([]byte, error) {
	return c.fetchURL(ctx, s.RawURL())
}

// fetchURL is Fetch without the Wayback URL composition, so a test can drive the
// decompression path against a server that behaves the way the Archive does.
func (c *Client) fetchURL(ctx context.Context, url string) ([]byte, error) {
	body, err := c.cachedGet(ctx, url, "raw", false)
	if err != nil {
		return nil, err
	}
	return Gunzipped(body), nil
}

// sameResource compares a URL the index returned against the one asked for, ignoring
// scheme and a trailing slash — CDX normalises neither consistently.
func sameResource(got, want string) bool {
	norm := func(u string) string {
		u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
		return strings.TrimSuffix(u, "/")
	}
	return norm(got) == norm(want)
}

// Gunzipped decompresses body if it is gzipped and returns it unchanged if not.
//
// Exported because the same sniff is needed by anything reading bytes this package
// cached, and a second copy of it is exactly the "one quantity, two
// implementations" shape this project keeps being bitten by.
func Gunzipped(body []byte) []byte {
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return body
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return body
	}
	defer zr.Close()
	// Capped for the same reason as the network read, and more sharply needed here:
	// a small compressed body can decompress without bound.
	out, err := io.ReadAll(io.LimitReader(zr, maxBody))
	if err != nil {
		return body
	}
	return out
}

// cachedGet fetches a URL, returning a cached body when there is one.
//
// The cache key is a hash of the URL rather than the URL itself, because a CDX
// query is several hundred characters of query string and would not survive being a
// filename. The `kind` prefix is there so a human can see at a glance whether a
// cache directory is full of index queries or payloads.
func (c *Client) cachedGet(ctx context.Context, endpoint, kind string, refresh bool) ([]byte, error) {
	sum := sha256.Sum256([]byte(endpoint))
	path := filepath.Join(c.cacheDir, "wayback-"+kind+"-"+hex.EncodeToString(sum[:])[:20])
	if !refresh {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return b, nil
		}
	}
	body, err := c.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(c.cacheDir, 0o755); err == nil {
		// Written to a temporary file and renamed, so a killed run cannot leave a
		// truncated entry behind. That failure is worse here than a missing cache: a
		// half-written body fails to gunzip, fails to parse, and is then reported as
		// "the Internet Archive holds no crawl before this deadline" — a permanent
		// gap, immune to re-running, blamed on a third party. The empty-200 guard
		// above exists for exactly this shape; this is its other half.
		if tmp, err := os.CreateTemp(c.cacheDir, "tmp-*"); err == nil {
			_, werr := tmp.Write(body)
			cerr := tmp.Close()
			if werr == nil && cerr == nil {
				_ = os.Rename(tmp.Name(), path)
			} else {
				_ = os.Remove(tmp.Name())
			}
		}
	}
	return body, nil
}

// get performs one polite, retrying HTTP GET.
func (c *Client) get(ctx context.Context, endpoint string) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= c.MaxAttempts; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		body, retryAfter, err := c.once(ctx, endpoint)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if attempt == c.MaxAttempts {
			break
		}
		// Exponential backoff, but the server's own Retry-After wins when it
		// sends one. Backing off less than asked is how a polite client becomes
		// an impolite one under exactly the load that made it ask.
		delay := time.Duration(1<<uint(attempt-1)) * c.BackoffBase
		if retryAfter > delay {
			delay = retryAfter
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", c.MaxAttempts, lastErr)
}

// once is a single request. It returns the server's requested delay separately,
// because a 429 with a Retry-After is a different instruction from a 500.
func (c *Client) once(ctx context.Context, endpoint string) ([]byte, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	// Identify the client. The Archive asks for this and it is the minimum owed
	// to infrastructure being used for free.
	req.Header.Set("User-Agent", "armband/1.0 (historical availability backfill; contact via repository)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))

	if resp.StatusCode != http.StatusOK {
		var after time.Duration
		if v := resp.Header.Get("Retry-After"); v != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				after = time.Duration(n) * time.Second
			}
		}
		return nil, after, fmt.Errorf("%s returned %s", endpoint, resp.Status)
	}
	if readErr != nil {
		return nil, 0, readErr
	}
	if len(body) == 0 {
		// An empty 200 is the Archive under load. Treated as an error so it is
		// retried, because caching it would poison the run silently — the
		// caller would see "no snapshot" rather than "the fetch failed".
		return nil, 0, fmt.Errorf("%s returned 200 with an empty body", endpoint)
	}
	return body, 0, nil
}

// wait enforces MinInterval between requests.
func (c *Client) wait(ctx context.Context) error {
	if c.MinInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.last.IsZero() {
		if d := c.MinInterval - time.Since(c.last); d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
	}
	c.last = time.Now()
	return nil
}
