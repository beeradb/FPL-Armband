package fpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const baseURL = "https://fantasy.premierleague.com/api"

// Client talks to the public FPL API. Responses are cached on disk so a single
// agent run (which may call many tools) hits the network once per endpoint.
//
// cacheDir is the only place this Client ever writes: a live-fetched response
// always lands there, never in snapshotDir. snapshotDir, when set, is an
// optional read-only base consulted before cacheDir on every read — see get()
// for the read order and NewWithSnapshot for why it is resolved through
// symlinks once, at construction, rather than on every read.
//
// # Serving stale rather than failing — a deliberate exception
//
// get()'s read order ends with a stale fallback: if a live fetch fails and
// neither the overlay nor the snapshot has anything FRESH, it serves whatever
// is on disk however old, rather than erroring. That overrides two of this
// project's own standing rules — "no fallbacks, one correct path" and "throw
// errors, fail fast" — on purpose, and only here. The reasoning: `armband
// serve` calls Bootstrap()/Fixtures() once at process startup, so an error
// here is not "this one request degrades", it is the pod failing to start —
// CrashLoopBackOff, no service at all, for as long as FPL is unreachable. The
// alternative to stale-but-coherent data is not a wrong answer the user might
// act on; it is no answer whatsoever. Every other error path in this codebase
// still fails loudly — this one exists because failing loudly here has no
// safe landing.
//
// Serving stale data is only acceptable because it is never silent: every
// fallback logs, and the staleness state is exposed through StaleServing,
// StaleAgeSeconds and LiveFetchFailures for cmd/armband's /metrics route to
// publish, so an on-call human is paged rather than finding out from a user
// report.
//
// # Two TTLs, because two endpoint families have different freshness needs
//
// cacheTTL governs Bootstrap, Fixtures, Entry, Picks and History — every
// endpoint the served worldview is built from once, at process startup, and
// frozen for the process's life (see cmd/armband's engine construction).
// elementTTL governs ElementSummary alone: it is read per HTTP request by
// playerDetail, not memoised, so it has no frozen-worldview guarantee to
// protect and can be given a much shorter window — a viewed player's match
// history catches up to whatever FPL is currently serving between archive
// refreshes, rather than waiting for the next scheduled warm+publish+restart
// cycle. Both TTLs are gated by get()'s same overlay-then-snapshot-then-live
// order and the same stale-serving fallback described above.
type Client struct {
	http        *http.Client
	cacheDir    string
	snapshotDir string
	cacheTTL    time.Duration
	elementTTL  time.Duration

	mu        sync.Mutex
	bootstrap *Bootstrap
	fixtures  []Fixture

	staleness staleness
}

// staleness is the process-lifetime signal behind Client's Stale* accessors.
// It exists so the deliberate "serve stale rather than fail" exception above
// is alertable rather than merely logged — see the Client comment for why
// that trade was made only here.
//
// All three fields are updated from get(), which many goroutines may call
// concurrently (a `serve` process handles overlapping requests), so each is
// its own atomic rather than being guarded by Client.mu — that mutex is
// reserved for the Bootstrap/Fixtures memoization, and taking it here would
// serialise every cache read behind it for no reason.
type staleness struct {
	// serving is 1 while the MOST RECENT get() call had to fall back to data
	// older than cacheTTL because a live fetch failed, 0 once a call
	// completes via a fresh cache hit or a successful live fetch. It is a
	// snapshot of the last call, not a sticky "ever happened" flag — the
	// simplest thing that is still true, and a Prometheus rate() over it is
	// what turns a flickering series into "how much of the window was
	// degraded" if that is ever needed.
	serving atomic.Bool
	// ageSeconds is the age, in seconds, of the stale copy most recently
	// served. It is only ever written on a stale read, so a reader who missed
	// the moment `serving` flipped back to 0 can still see how old the last
	// bad one was.
	ageSeconds atomic.Int64
	// liveFetchFailures counts every live fetch to FPL that has failed, for
	// the life of the process. Monotonic and never reset, matching the
	// Prometheus counter type it feeds.
	liveFetchFailures atomic.Uint64
}

// StaleServing reports whether the most recent read had to fall back to data
// older than cacheTTL. See the Client and staleness comments for why that
// fallback exists and what "most recent" means across concurrent callers.
func (c *Client) StaleServing() bool { return c.staleness.serving.Load() }

// StaleAgeSeconds is the age, in seconds, of the stale copy most recently
// served — 0 if none has been served yet this process's lifetime.
func (c *Client) StaleAgeSeconds() int64 { return c.staleness.ageSeconds.Load() }

// LiveFetchFailures is the number of live fetches to FPL that have failed to
// produce a usable answer since this process started — a transport error, a
// non-200 status, AND a 200 whose body didn't decode (an HTML login page, or
// anything else that isn't the JSON expected). Not transport failures alone:
// an on-call human correlating this against FPL's own status page should read
// it as "FPL did not answer usefully", not "the network was down". It does
// NOT count the caller's own context ending mid-fetch (a browser tab
// navigating away from a player card) — that says nothing about FPL, and
// counting it would let ordinary page-request cancellation page on-call. See
// get()'s context.Canceled/DeadlineExceeded check.
func (c *Client) LiveFetchFailures() uint64 { return c.staleness.liveFetchFailures.Load() }

// NewWithSnapshot is New with an optional read-only base checked before
// cacheDir on every read. cacheDir remains the only place anything is ever
// written — see get()'s comment for the read order and why.
//
// snapshotDir, if non-empty, is resolved through any symlinks EXACTLY ONCE,
// here, not on every read. That is deliberate: the deployment's snapshotDir
// is `.../archive/current`, a symlink a generator process repoints to a new
// immutable directory periodically. Resolving it once and keeping the
// concrete target for the client's whole lifetime means a later repoint
// cannot change what THIS process reads mid-run — matching the existing
// architecture's "the engine is built once, only a restart unfreezes it"
// property (see cmd/armband/main.go and internal/fpl.Client.Bootstrap's own
// memoization) rather than fighting it.
//
// If the path cannot be resolved — most likely because it does not exist yet,
// e.g. the generator has not published a first snapshot — the snapshot is
// disabled for this process's whole lifetime (every read falls through to a
// live fetch) rather than used as given. Using the unresolved path would not
// actually fail here: os.Stat and os.ReadFile transparently re-resolve a
// symlink component on every call, so if the path later starts existing —
// entirely plausible on the exact cold-start race this comment is about — the
// client would silently start tracking every later repoint instead of staying
// frozen, which is the per-read behaviour this whole mechanism exists to
// avoid. Disabling on failure fails in the safe direction (more live fetches)
// instead of quietly reintroducing that inconsistency.
// elementTTL, like ttl, is forced to zero by -refresh (see cmd/armband/main.go)
// so the generator always refetches ElementSummary too, not just Bootstrap and
// Fixtures.
func NewWithSnapshot(cacheDir, snapshotDir string, ttl, elementTTL time.Duration) *Client {
	if snapshotDir != "" {
		resolved, err := filepath.EvalSymlinks(snapshotDir)
		if err != nil {
			snapshotDir = ""
		} else {
			snapshotDir = resolved
		}
	}
	return &Client{
		http:        &http.Client{Timeout: 30 * time.Second},
		cacheDir:    cacheDir,
		snapshotDir: snapshotDir,
		cacheTTL:    ttl,
		elementTTL:  elementTTL,
	}
}

// New is NewWithSnapshot with no snapshot base — every existing caller's
// behaviour, unchanged.
func New(cacheDir string, ttl, elementTTL time.Duration) *Client {
	return NewWithSnapshot(cacheDir, "", ttl, elementTTL)
}

func (c *Client) get(ctx context.Context, path string, out any, ttl time.Duration) error {
	key := strings.NewReplacer("/", "_", "?", "_", "&", "_").Replace(strings.Trim(path, "/"))
	overlayPath := filepath.Join(c.cacheDir, key+".json")

	// Every endpoint this client reaches is public and TTL-cached. There is no
	// authenticated endpoint and no credentialled response, so there is nothing
	// here that must be kept off disk — see the note on Client. ttl is the
	// caller's own freshness window (cacheTTL or elementTTL — see the Client
	// comment), not a fixed field, so Bootstrap/Fixtures and ElementSummary can
	// disagree about how old is too old.
	fresh := func(fi os.FileInfo) bool {
		return time.Since(fi.ModTime()) < ttl
	}

	// The overlay first: it holds anything a process sharing this cacheDir has
	// itself fetched, and it is where -refresh's ttl=0 refetches land. This
	// does not compare mtimes against the snapshot, so it is not "always the
	// freshest available" — an overlay entry a minute inside its own TTL wins
	// even if the snapshot underneath it was regenerated more recently. It is
	// "fresh enough by this process's own TTL", the same guarantee cacheTTL
	// gave before snapshotDir existed.
	if fi, err := os.Stat(overlayPath); err == nil && fresh(fi) {
		if b, err := os.ReadFile(overlayPath); err == nil {
			if err := json.Unmarshal(b, out); err == nil {
				c.staleness.serving.Store(false)
				return nil
			}
		}
	}

	// Then the read-only snapshot, if one is configured. Its mtime is the
	// moment the GENERATOR fetched it, not the moment this process started —
	// ttl is measuring the same thing it always measured (how long ago
	// this exact JSON was fetched from FPL), so a process that has fetched
	// nothing itself can still serve fresh data. A missing snapshot (not yet
	// generated, or this deployment doesn't use one) falls straight through to
	// the live fetch below — a snapshot miss must never look like an empty or
	// wrong answer, only a live one.
	var snapPath string
	if c.snapshotDir != "" {
		snapPath = filepath.Join(c.snapshotDir, key+".json")
		if fi, err := os.Stat(snapPath); err == nil && fresh(fi) {
			if b, err := os.ReadFile(snapPath); err == nil {
				if err := json.Unmarshal(b, out); err == nil {
					c.staleness.serving.Store(false)
					return nil
				}
			}
		}
	}

	body, fetchErr := c.fetch(ctx, path)
	if fetchErr != nil && (errors.Is(fetchErr, context.Canceled) || errors.Is(fetchErr, context.DeadlineExceeded)) {
		// The caller's own context ended, not FPL's fetch — e.g. a browser tab
		// navigating away mid-request cancels r.Context() on the player-card
		// route. That is routine and says nothing about FPL's health, so it
		// must not touch the staleness counters this deliberately-alertable
		// fallback exists to keep trustworthy, and there is no live body to
		// fall back from anyway.
		return fetchErr
	}
	if fetchErr == nil {
		if unmarshalErr := json.Unmarshal(body, out); unmarshalErr == nil {
			// Writes always go to the overlay (cacheDir), never to snapshotDir. The
			// snapshot is read-only from this process's side by construction: only the
			// deployment's generator writes there, on its own schedule. Do not "fix"
			// this into writing wherever a cache miss came from.
			if err := os.MkdirAll(c.cacheDir, 0o755); err == nil {
				_ = os.WriteFile(overlayPath, body, 0o644)
			}
			c.staleness.serving.Store(false)
			return nil
		} else if len(body) > 0 && body[0] == '<' {
			// A 200 carrying HTML is FPL serving a login page instead of the
			// resource, which is what an expired or missing session looks like from
			// here. Saying "invalid character '<'" sends people to look for a
			// parsing bug.
			fetchErr = fmt.Errorf("GET %s: FPL returned an HTML page, not data — "+
				"the session is missing or expired", path)
		} else {
			fetchErr = fmt.Errorf("GET %s: decoding: %w", path, unmarshalErr)
		}
	}

	// The live fetch failed — network error, non-200 status, or a body FPL
	// sent that did not decode. See the Client comment for why this
	// deliberately serves stale data rather than returning fetchErr here: at
	// startup (Bootstrap/Fixtures, called once per process) an error is not
	// "this read degrades", it is the pod failing to start.
	//
	// Stale snapshot before stale overlay: the snapshot is what a healthy
	// generator most recently published for every endpoint it covers, and is
	// more likely to be complete than an overlay this one process happened to
	// fetch piecemeal.
	c.staleness.liveFetchFailures.Add(1)
	if snapPath != "" && c.serveStale(snapPath, "snapshot", path, out) {
		return nil
	}
	if c.serveStale(overlayPath, "overlay", path, out) {
		return nil
	}

	// Nothing usable anywhere — the one case this deliberate fallback does not
	// cover, and the original live-fetch error is the right thing to report.
	return fetchErr
}

// serveStale is get()'s last resort: read whatever is at diskPath, however
// old, and unmarshal it into out. Called only after a live fetch has already
// failed — see the Client and staleness comments for why this override of
// "no fallbacks, fail fast" is deliberate and only here. Returns false, out
// untouched, if there is nothing usable at diskPath at all — the one case
// get() still returns an error for.
func (c *Client) serveStale(diskPath, source, requestPath string, out any) bool {
	fi, err := os.Stat(diskPath)
	if err != nil {
		return false
	}
	b, err := os.ReadFile(diskPath)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(b, out); err != nil {
		return false
	}
	age := time.Since(fi.ModTime())
	c.staleness.serving.Store(true)
	c.staleness.ageSeconds.Store(int64(age.Seconds()))
	log.Printf("fpl: live fetch to FPL failed; serving stale %s data for %s (age %s) — "+
		"see the armband_serving_stale_data Prometheus gauge", source, requestPath, age.Round(time.Second))
	return true
}

// fetch performs the request and returns the raw body, with no cache on either
// side of it.
//
// Split out of get so that Raw can have the bytes exactly as FPL sent them. That
// distinction is load-bearing for the weekly capture: re-serialising a parsed
// struct would silently drop every field the parser does not read, and this
// project has five recorded instances of concluding "the archive does not carry X"
// when X was present and simply unparsed.
func (c *Client) fetch(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	// The FPL API rejects requests without a browser-like User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) armband/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: reading body: %w", path, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		// Wrapped with %w rather than left as plain status text, so a caller that needs
		// to tell "this resource does not exist" apart from "FPL is unreachable" — the
		// import route does, for an entry id or a gameweek's picks a visitor typed —
		// can use errors.Is(err, ErrNotFound) instead of matching on this string, which
		// is exactly the trap ErrNotAnEntryID's own doc comment names.
		return nil, fmt.Errorf("GET %s: status %d: %s: %w",
			path, resp.StatusCode, truncate(string(body), 200), ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

// ErrNotFound is returned (wrapped) when FPL answers 404 to a live fetch.
//
// A sentinel so a caller can ask "does this resource exist" with errors.Is rather than
// parsing "status 404" out of an error string — the same reasoning ErrNotAnEntryID's own
// doc comment gives, and the two exist for the same route: PUT /api/import needs to answer
// a visitor with "no such team" (404) rather than "FPL is not answering" (503), and those
// two failures must not be told apart by matching on message text.
var ErrNotFound = errors.New("fpl: not found")

// Raw returns an endpoint's body exactly as FPL sent it, always over the network.
//
// It exists for the weekly capture, whose entire value is that it records a
// *moment*: a cached body would date the capture to whenever the cache was
// written, and a re-serialised struct would record only the fields this program
// happens to parse today. Neither is evidence about a moment.
func (c *Client) Raw(ctx context.Context, path string) ([]byte, error) {
	body, err := c.fetch(ctx, path)
	if err != nil {
		return nil, err
	}
	// A 200 carrying HTML is FPL serving a login or error page. Storing it would
	// put a file in the archive that looks like a capture and is not one.
	if len(body) > 0 && body[0] == '<' {
		return nil, fmt.Errorf("GET %s: FPL returned an HTML page, not data", path)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("GET %s: body is not valid JSON, refusing to archive it", path)
	}
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// BootstrapFetchedAt reports when the bootstrap payload now in hand was last
// pulled from FPL — the disk cache file's own modification time, since that
// write IS the fetch and this client keeps no separate record of it. The
// zero time means nothing has been cached yet.
//
// It exists for the News tab's freshness line ("FPL status last read 3
// minutes ago"), which must be server-formatted for the same reason every
// other staleness figure in the client contract is: the client has no
// honest way to compute "ago" against a clock it does not share with the
// server. See internal/viewmodel.State.News.
//
// ⚠️ For a long-running `armband serve` process this answers "when was the
// data now being served fetched", which is right — but Bootstrap memoizes
// the result in memory for the life of the process (see its own comment),
// so nothing in this client will fetch AGAIN until the process restarts,
// however old the disk cache gets. A caller pairing this with CacheTTL to
// print "next read at HH:MM" is describing the cache file's TTL contract,
// not a promise that THIS process will act on it — that would need a
// periodic refresh this client does not have.
func (c *Client) BootstrapFetchedAt() time.Time {
	fi, err := os.Stat(filepath.Join(c.cacheDir, "bootstrap-static.json"))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// CacheTTL is how long a cached response is served before a fresh request
// would refetch it — cache_minutes, as constructed by New. Exported so a
// caller can compute what BootstrapFetchedAt's own freshness window is,
// without a second copy of the number.
func (c *Client) CacheTTL() time.Duration {
	return c.cacheTTL
}

// Bootstrap returns players, teams and gameweeks, memoized for the process.
func (c *Client) Bootstrap(ctx context.Context) (*Bootstrap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bootstrap != nil {
		return c.bootstrap, nil
	}
	var b Bootstrap
	if err := c.get(ctx, "/bootstrap-static/", &b, c.cacheTTL); err != nil {
		return nil, err
	}
	c.bootstrap = &b
	return &b, nil
}

// Fixtures returns all 380 fixtures for the season.
func (c *Client) Fixtures(ctx context.Context) ([]Fixture, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fixtures != nil {
		return c.fixtures, nil
	}
	var f []Fixture
	if err := c.get(ctx, "/fixtures/", &f, c.cacheTTL); err != nil {
		return nil, err
	}
	c.fixtures = f
	return f, nil
}

// FixturesLive is Fixtures with no cache anywhere — always a live fetch.
//
// Client.Fixtures memoises in memory for the life of the process (see its own
// comment), which is right for planning a fixture list and wrong for asking
// "has this match kicked off yet" — a question the process's own long life
// makes stale the moment it is answered once. See Raw's doc comment for the
// same shape of problem on Entry/Picks, for an unrelated reason.
func (c *Client) FixturesLive(ctx context.Context) ([]Fixture, error) {
	body, err := c.Raw(ctx, "/fixtures/")
	if err != nil {
		return nil, err
	}
	var f []Fixture
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("GET /fixtures/: decoding: %w", err)
	}
	return f, nil
}

// Live returns one gameweek's per-player stats — live during play, final once FPL
// finishes scoring it. Always a fetch, never cached, for the same reason
// FixturesLive is: a cached "live" number is a contradiction.
func (c *Client) Live(ctx context.Context, event int) (*EventLive, error) {
	body, err := c.Raw(ctx, fmt.Sprintf("/event/%d/live/", event))
	if err != nil {
		return nil, err
	}
	var el EventLive
	if err := json.Unmarshal(body, &el); err != nil {
		return nil, fmt.Errorf("GET /event/%d/live/: decoding: %w", event, err)
	}
	return &el, nil
}

// ElementSummary returns per-player match history, past seasons and upcoming fixtures.
func (c *Client) ElementSummary(ctx context.Context, id int) (*ElementSummary, error) {
	var s ElementSummary
	if err := c.get(ctx, fmt.Sprintf("/element-summary/%d/", id), &s, c.elementTTL); err != nil {
		return nil, err
	}
	return &s, nil
}

// entryPath and picksPath are the one spelling of these two endpoints. Entry, Picks,
// EntryUncached and PicksUncached all build their request path through these rather than
// inlining the format string a second time — see this project's own "one quantity, two
// implementations" rule.
func entryPath(id int) string { return fmt.Sprintf("/entry/%d/", id) }
func picksPath(entryID, event int) string {
	return fmt.Sprintf("/entry/%d/event/%d/picks/", entryID, event)
}

// Entry returns a manager's overall record.
func (c *Client) Entry(ctx context.Context, id int) (*Entry, error) {
	var e Entry
	if err := c.get(ctx, entryPath(id), &e, c.cacheTTL); err != nil {
		return nil, err
	}
	return &e, nil
}

// Picks returns a manager's squad for a given gameweek. Only available once
// that gameweek's deadline has passed.
func (c *Client) Picks(ctx context.Context, entryID, event int) (*EntryPicks, error) {
	var p EntryPicks
	if err := c.get(ctx, picksPath(entryID, event), &p, c.cacheTTL); err != nil {
		return nil, err
	}
	return &p, nil
}

// EntryUncached and PicksUncached are Entry and Picks with no disk cache on either side —
// always a live fetch, exactly like Raw.
//
// # Why this route may not go through the disk cache
//
// get()'s cache writes one file per distinct URL it is asked for, keyed on the path. Every
// other caller of Entry/Picks in this codebase supplies an id the OPERATOR configured
// (config.EntryID) or one covered by the same trust boundary — a bounded, self-inflicted
// set of cache files. PUT /api/import is different: the id comes from an anonymous visitor
// typing into a public form, so a route that reached the ordinary cached path would turn
// "how many files sit in cacheDir" into an attacker-controlled quantity. This codebase's
// deployment shares that disk with the season archive, the proxy cache and Traefik's own
// acme.json — a filled disk there takes certificate renewal down with everything else on
// it, which is the failure signup.Clean's own length bound exists to prevent on the other
// public write path this server has.
//
// Raw already solves exactly this for the weekly capture, for an unrelated reason (a
// capture must record a moment, not a cache-write time) — these two functions are it,
// reused for a different reason, rather than a second no-cache mechanism.
func (c *Client) EntryUncached(ctx context.Context, id int) (*Entry, error) {
	path := entryPath(id)
	body, err := c.Raw(ctx, path)
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("GET %s: decoding: %w", path, err)
	}
	return &e, nil
}

// PicksUncached is Picks with no disk cache — see EntryUncached's doc comment for why.
func (c *Client) PicksUncached(ctx context.Context, entryID, event int) (*EntryPicks, error) {
	path := picksPath(entryID, event)
	body, err := c.Raw(ctx, path)
	if err != nil {
		return nil, err
	}
	var p EntryPicks
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("GET %s: decoding: %w", path, err)
	}
	return &p, nil
}

// --- Lookup helpers -------------------------------------------------------

func (b *Bootstrap) TeamByID(id int) *Team {
	for i := range b.Teams {
		if b.Teams[i].ID == id {
			return &b.Teams[i]
		}
	}
	return nil
}

func (b *Bootstrap) TeamByName(name string) *Team {
	n := strings.ToLower(strings.TrimSpace(name))
	for i := range b.Teams {
		t := &b.Teams[i]
		if strings.ToLower(t.ShortName) == n || strings.ToLower(t.Name) == n {
			return t
		}
	}
	for i := range b.Teams {
		if strings.Contains(strings.ToLower(b.Teams[i].Name), n) {
			return &b.Teams[i]
		}
	}
	return nil
}

func (b *Bootstrap) ElementByID(id int) *Element {
	for i := range b.Elements {
		if b.Elements[i].ID == id {
			return &b.Elements[i]
		}
	}
	return nil
}

// ElementByCode looks a player up by his PERMANENT code rather than his
// season-scoped element id. Every standing correction is keyed on the code
// for exactly this reason — element ids are reassigned every summer, so a
// caller holding only a code (a config override, for instance) needs this
// rather than reconstructing a code-to-id map of its own.
func (b *Bootstrap) ElementByCode(code int) *Element {
	if code == 0 {
		return nil
	}
	for i := range b.Elements {
		if b.Elements[i].Code == code {
			return &b.Elements[i]
		}
	}
	return nil
}

// FindPlayers does a fuzzy name match, best matches first.
func (b *Bootstrap) FindPlayers(query string) []*Element {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	// Tiers are ordered by how likely the match is to be the player a human
	// meant. WebName — FPL's own display name — must outrank a hit on someone
	// else's first name, or common given names swamp the intended player:
	// "Rodri" prefix-matches both Rodri (WebName "Rodrigo") and Rodrigo
	// Bentancur, and with those tied the points tiebreak handed back Bentancur.
	// The agent then looked up the wrong player entirely.
	const (
		exactWeb = iota
		exactFull
		prefixWeb
		prefixSurname
		prefixFull
		containsWeb
		containsFull
	)
	type scored struct {
		el    *Element
		score int
	}
	var out []scored
	for i := range b.Elements {
		e := &b.Elements[i]
		web := strings.ToLower(e.WebName)
		surname := strings.ToLower(e.SecondName)
		full := strings.ToLower(e.FirstName + " " + e.SecondName)
		switch {
		case web == q:
			out = append(out, scored{e, exactWeb})
		case full == q:
			out = append(out, scored{e, exactFull})
		case strings.HasPrefix(web, q):
			out = append(out, scored{e, prefixWeb})
		case strings.HasPrefix(surname, q):
			out = append(out, scored{e, prefixSurname})
		case strings.HasPrefix(full, q):
			out = append(out, scored{e, prefixFull})
		case strings.Contains(web, q):
			out = append(out, scored{e, containsWeb})
		case strings.Contains(full, q):
			out = append(out, scored{e, containsFull})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score < out[j].score
		}
		return out[i].el.TotalPoints > out[j].el.TotalPoints
	})
	res := make([]*Element, len(out))
	for i, s := range out {
		res[i] = s.el
	}
	return res
}

func (b *Bootstrap) PositionShort(elementType int) string {
	for _, t := range b.ElementTypes {
		if t.ID == elementType {
			return t.SingularNameShort
		}
	}
	return "?"
}

// NextEvent returns the upcoming gameweek, or the current one if none is flagged next.
func (b *Bootstrap) NextEvent() *Event {
	for i := range b.Events {
		if b.Events[i].IsNext {
			return &b.Events[i]
		}
	}
	for i := range b.Events {
		if b.Events[i].IsCurrent {
			return &b.Events[i]
		}
	}
	for i := range b.Events {
		if !b.Events[i].Finished {
			return &b.Events[i]
		}
	}
	return nil
}

func (b *Bootstrap) CurrentEvent() *Event {
	for i := range b.Events {
		if b.Events[i].IsCurrent {
			return &b.Events[i]
		}
	}
	return nil
}

// StatusLabel turns FPL's single-letter availability code into something readable.
func (e *Element) StatusLabel() string {
	switch e.Status {
	case "a":
		return "available"
	case "d":
		return "doubtful"
	case "i":
		return "injured"
	case "s":
		return "suspended"
	case "u":
		return "unavailable"
	case "n":
		return "not in squad"
	default:
		return e.Status
	}
}

// TenthsToMillions converts one of FPL's price fields -- always tenths of a million -- to
// the unit every surface in this codebase actually displays. Element.NowCost,
// PastSeason.StartCost/EndCost and HistoryEntry.Value are all in the same unit; this is the
// one implementation of that conversion, so a caller must not repeat "/ 10.0" itself.
func TenthsToMillions(tenths int) float64 { return float64(tenths) / 10.0 }

func (e *Element) PriceM() float64 { return TenthsToMillions(e.NowCost) }

// History returns a manager's gameweek-by-gameweek record and chips played.
func (c *Client) History(ctx context.Context, entryID int) (*EntryHistory, error) {
	var h EntryHistory
	if err := c.get(ctx, fmt.Sprintf("/entry/%d/history/", entryID), &h, c.cacheTTL); err != nil {
		return nil, err
	}
	return &h, nil
}

// MaxBankedTransfers is the FPL cap on saved free transfers.
const MaxBankedTransfers = 5

// UnlimitedTransfers is returned when no gameweek has been scored yet. Before
// the first deadline a manager is still assembling the initial squad and may
// make as many changes as they like, so any finite number is wrong — and "1" is
// wrong in the dangerous direction, since it invites the agent to advise
// hoarding a transfer that does not exist as a constraint.
const UnlimitedTransfers = -1

// FreeTransfers reconstructs how many free transfers a manager has available.
//
// FPL does not publish this directly, so it is rebuilt from the transfer history:
// one free transfer per gameweek, banked up to the cap, minus those spent. A
// wildcard or free hit week is skipped, since transfers made under a chip are
// unlimited and do not consume the allowance. Treat the result as a close
// reconstruction rather than an authoritative figure.
//
// An empty history means the season has not started, which is unambiguous:
// the manager has unlimited transfers, not one.
func FreeTransfers(h *EntryHistory) int {
	if h == nil || len(h.Current) == 0 {
		return UnlimitedTransfers
	}
	chipAt := map[int]string{}
	for _, c := range h.Chips {
		chipAt[c.Event] = c.Name
	}

	ft := 1
	for _, gw := range h.Current {
		switch chipAt[gw.Event] {
		case "wildcard", "freehit":
			// Chip weeks don't spend the allowance.
		default:
			ft -= gw.EventTransfers
			if ft < 0 {
				ft = 0
			}
		}
		// The following gameweek grants one more, capped.
		ft++
		if ft > MaxBankedTransfers {
			ft = MaxBankedTransfers
		}
	}
	if ft < 1 {
		ft = 1
	}
	return ft
}
