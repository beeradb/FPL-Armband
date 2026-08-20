package fpl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
type Client struct {
	http        *http.Client
	cacheDir    string
	snapshotDir string
	cacheTTL    time.Duration

	mu        sync.Mutex
	bootstrap *Bootstrap
	fixtures  []Fixture
}

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
func NewWithSnapshot(cacheDir, snapshotDir string, ttl time.Duration) *Client {
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
	}
}

// New is NewWithSnapshot with no snapshot base — every existing caller's
// behaviour, unchanged.
func New(cacheDir string, ttl time.Duration) *Client {
	return NewWithSnapshot(cacheDir, "", ttl)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	key := strings.NewReplacer("/", "_", "?", "_", "&", "_").Replace(strings.Trim(path, "/"))
	overlayPath := filepath.Join(c.cacheDir, key+".json")

	// Every endpoint this client reaches is public and TTL-cached. There is no
	// authenticated endpoint and no credentialled response, so there is nothing
	// here that must be kept off disk — see the note on Client.
	fresh := func(fi os.FileInfo) bool {
		return time.Since(fi.ModTime()) < c.cacheTTL
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
				return nil
			}
		}
	}

	// Then the read-only snapshot, if one is configured. Its mtime is the
	// moment the GENERATOR fetched it, not the moment this process started —
	// cacheTTL is measuring the same thing it always measured (how long ago
	// this exact JSON was fetched from FPL), so a process that has fetched
	// nothing itself can still serve fresh data. A missing snapshot (not yet
	// generated, or this deployment doesn't use one) falls straight through to
	// the live fetch below — a snapshot miss must never look like an empty or
	// wrong answer, only a live one.
	if c.snapshotDir != "" {
		snapPath := filepath.Join(c.snapshotDir, key+".json")
		if fi, err := os.Stat(snapPath); err == nil && fresh(fi) {
			if b, err := os.ReadFile(snapPath); err == nil {
				if err := json.Unmarshal(b, out); err == nil {
					return nil
				}
			}
		}
	}

	body, err := c.fetch(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		// A 200 carrying HTML is FPL serving a login page instead of the
		// resource, which is what an expired or missing session looks like from
		// here. Saying "invalid character '<'" sends people to look for a
		// parsing bug.
		if len(body) > 0 && body[0] == '<' {
			return fmt.Errorf("GET %s: FPL returned an HTML page, not data — "+
				"the session is missing or expired", path)
		}
		return fmt.Errorf("GET %s: decoding: %w", path, err)
	}

	// Writes always go to the overlay (cacheDir), never to snapshotDir. The
	// snapshot is read-only from this process's side by construction: only the
	// deployment's generator writes there, on its own schedule. Do not "fix"
	// this into writing wherever a cache miss came from.
	if err := os.MkdirAll(c.cacheDir, 0o755); err == nil {
		_ = os.WriteFile(overlayPath, body, 0o644)
	}
	return nil
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

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

// Bootstrap returns players, teams and gameweeks, memoized for the process.
func (c *Client) Bootstrap(ctx context.Context) (*Bootstrap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bootstrap != nil {
		return c.bootstrap, nil
	}
	var b Bootstrap
	if err := c.get(ctx, "/bootstrap-static/", &b); err != nil {
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
	if err := c.get(ctx, "/fixtures/", &f); err != nil {
		return nil, err
	}
	c.fixtures = f
	return f, nil
}

// ElementSummary returns per-player match history, past seasons and upcoming fixtures.
func (c *Client) ElementSummary(ctx context.Context, id int) (*ElementSummary, error) {
	var s ElementSummary
	if err := c.get(ctx, fmt.Sprintf("/element-summary/%d/", id), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Entry returns a manager's overall record.
func (c *Client) Entry(ctx context.Context, id int) (*Entry, error) {
	var e Entry
	if err := c.get(ctx, fmt.Sprintf("/entry/%d/", id), &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// Picks returns a manager's squad for a given gameweek. Only available once
// that gameweek's deadline has passed.
func (c *Client) Picks(ctx context.Context, entryID, event int) (*EntryPicks, error) {
	var p EntryPicks
	if err := c.get(ctx, fmt.Sprintf("/entry/%d/event/%d/picks/", entryID, event), &p); err != nil {
		return nil, err
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
	if err := c.get(ctx, fmt.Sprintf("/entry/%d/history/", entryID), &h); err != nil {
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
