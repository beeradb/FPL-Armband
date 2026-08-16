package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Store is a read-only view over a captures root, live and backfilled alike.
//
// # Why one reader rather than two
//
// A live capture and a recovered one differ in provenance and in nothing else that
// matters to a consumer: both are FPL's own bootstrap-static, both are dated, both
// are immutable. Anything reading them wants "what did the game say about this
// player at this deadline", and having to ask that question twice — once of a live
// series and once of an archive — is how the two drift into disagreeing. The
// provenance is carried on every answer instead, so a consumer that *does* care can
// still tell.
//
// Reads are lazy: opening a store parses manifests only, which are a few hundred
// bytes each, and a payload is decompressed when somebody asks for a gameweek. Six
// seasons of bodies is around 150 MB uncompressed and there is no reason to hold it.
type Store struct {
	root string

	// byGameweek indexes the manifests. The season key is "" for the live series,
	// which is not filed under a season because the live series does not know what
	// season it is in — it is whatever FPL was serving.
	byGameweek map[seasonGW]entry

	// counts is how many captures cover each gameweek, which byGameweek cannot say
	// because it holds only the one that reads closest to the deadline. A denser
	// pull stores several per gameweek and the count is what tells a re-run
	// whether it has already done that work.
	counts map[seasonGW]int

	// all is every indexed capture, in index order. byGameweek keeps only the one
	// that reads closest to each deadline, which is the right answer to "what was
	// FPL saying" and the wrong basis for anything that must walk the whole store —
	// the point-in-time proof most of all, since the captures it would skip are
	// exactly the ones no write-time check has looked at since.
	all []entry

	seasons []string
}

type seasonGW struct {
	Season string
	GW     int
}

type entry struct {
	dir string
	m   *Manifest
}

// Open indexes a captures root. A root that does not exist is an empty store rather
// than an error: a checkout with no captures is a normal state, and the caller's own
// "nothing found" message is more useful than a path error from here.
func Open(root string) (*Store, error) {
	s := &Store{root: root, byGameweek: map[seasonGW]entry{}, counts: map[seasonGW]int{}}
	seasons := map[string]bool{}

	// The live series sits directly under the root; backfilled seasons sit one
	// level down, one directory per season. Both are read the same way.
	if err := s.index(root, ""); err != nil {
		return nil, err
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		// A capture-timestamp directory is a live capture, already indexed.
		if _, err := time.Parse(dirLayout, d.Name()); err == nil {
			continue
		}
		if err := s.index(filepath.Join(root, d.Name()), d.Name()); err != nil {
			return nil, err
		}
		seasons[d.Name()] = true
	}
	for k := range seasons {
		s.seasons = append(s.seasons, k)
	}
	sort.Strings(s.seasons)
	return s, nil
}

// index reads every capture directly under dir.
//
// It looks for a manifest rather than for a directory name it recognises. The live
// series names directories by timestamp and the backfill names them by gameweek and
// timestamp, and a reader that had to know both would have to be taught a third the
// next time somebody adds one. The manifest is the authority on what a capture is —
// that is why `Take` writes one at all, rather than relying on the path — so
// "has a manifest" is the right membership test.
func (s *Store) index(dir, season string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		path := filepath.Join(dir, n)
		b, err := os.ReadFile(filepath.Join(path, "manifest.json"))
		if err != nil {
			// A directory with no manifest is a killed capture, which `Take`
			// documents as the deliberate shape of an interrupted write. Skipping
			// it is right; failing the whole store for it is not.
			continue
		}
		var m Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("manifest %s: %w", path, err)
		}
		if m.Event == 0 {
			continue // a capture that could not be tied to a gameweek answers no question here
		}
		key := seasonGW{Season: season, GW: m.Event}
		s.counts[key]++
		s.all = append(s.all, entry{dir: path, m: &m})

		// Where two captures cover one deadline, the later one wins: it is the
		// closer read of the information state the decision was taken in, which
		// is the whole quantity being recovered. Deterministic rather than
		// last-indexed, because the directory listing is sorted first and a tie
		// decided by filesystem order is a tie decided differently on two machines.
		if prev, ok := s.byGameweek[key]; ok && !m.CapturedAt.After(prev.m.CapturedAt) {
			continue
		}
		s.byGameweek[key] = entry{dir: path, m: &m}
	}
	return nil
}

// dirLayout is the capture directory name format. Named so the two places that
// parse it cannot drift apart; see DirName for why it is UTC.
const dirLayout = "2006-01-02T1504Z"

// Seasons lists the backfilled seasons present, oldest first.
func (s *Store) Seasons() []string { return append([]string(nil), s.seasons...) }

// Count is how many captures cover one gameweek. Zero is a gap.
func (s *Store) Count(season string, gw int) int {
	return s.counts[seasonGW{Season: season, GW: gw}]
}

// Dir is the directory of the capture that reads closest to a gameweek's deadline.
func (s *Store) Dir(season string, gw int) (string, bool) {
	e, ok := s.byGameweek[seasonGW{Season: season, GW: gw}]
	if !ok {
		return "", false
	}
	return e.dir, true
}

// PlayerStatus is what FPL was saying about one player at one moment.
//
// Keyed on `Code`, the permanent player identifier, everywhere it is used. Element
// ids are reassigned every summer, so an availability record keyed on one comes back
// next season attached to a different footballer — a trap this project has already
// paid for once, in the standing overrides.
type PlayerStatus struct {
	Code      int    `json:"code"`
	ElementID int    `json:"id"` // this season's id, for joining to the season archive
	WebName   string `json:"web_name"`
	Team      int    `json:"team"`
	Type      int    `json:"element_type"`

	// Status is FPL's one-letter availability flag: `a` available, `d` doubtful,
	// `i` injured, `s` suspended, `n` on loan or otherwise ineligible, `u`
	// unavailable (most often departed).
	Status string `json:"status"`

	// ChanceThis and ChanceNext are FPL's percentage chance of playing, this
	// gameweek and next, in steps of 25. **Nil is not zero**: nil means FPL
	// published no figure, which for a fit player is the normal state, while 0
	// means it published a definite "will not play". Collapsing the two would
	// turn every healthy player in the game into a certain absentee.
	ChanceThis *int `json:"chance_of_playing_this_round"`
	ChanceNext *int `json:"chance_of_playing_next_round"`

	// News is the free text FPL showed — "Calf injury - 75% chance of playing" —
	// and NewsAdded is when it was posted. The pair is what makes an absence
	// datable rather than merely present.
	News      string `json:"news"`
	NewsAdded string `json:"news_added"`
}

// Availability is every player's status at one gameweek's deadline.
type Availability struct {
	Season string
	Event  int

	// SnapshotAt is when the payload was served, and Deadline is the moment it is
	// evidence about.
	//
	// For a **backfilled** capture SnapshotAt is always strictly before Deadline —
	// nothing else can enter the store. For a **live** one it need not be: that
	// series runs on a timer rather than on the fixture list, so a capture taken
	// after a deadline is a legitimate record of a later moment and HoursBefore is
	// then negative. Check `Backfilled`, or check the sign, before treating a row
	// from this store as pre-decision evidence.
	SnapshotAt time.Time
	Deadline   time.Time

	// HoursBefore is the gap between them. A capture two hours before a deadline is
	// the team news; one nine days before it is last week's.
	HoursBefore float64

	// EventsBehind is how many deadlines had already passed when this was crawled:
	// 0 for a crawl inside this gameweek's own run-up, 1 when it also predates the
	// previous deadline, -1 when the payload could not say.
	//
	// Read this before HoursBefore. Nine days is fresh across an international break
	// and badly stale inside a normal week, and only this field knows which.
	EventsBehind int

	// Backfilled says where this came from: false for a capture this project took
	// itself, true for one recovered from the Internet Archive.
	Backfilled bool

	// Players is keyed by permanent player code.
	Players map[int]PlayerStatus
}

// At returns the availability recorded for one gameweek of one season.
//
// `season` is "" for the live series and e.g. "2020-21" for a backfilled one.
//
// A missing gameweek is an error naming what is missing, rather than an empty
// result. Coverage is genuinely patchy — the Internet Archive crawled FPL about
// two days in three in 2020-21 — and a caller that silently treats an absent
// gameweek as "nobody was injured" would produce clean-looking figures from no data
// at all. That is this package's recorded failure mode, twice: a repair that
// silently applies nothing.
func (s *Store) At(season string, gw int) (*Availability, error) {
	e, ok := s.byGameweek[seasonGW{Season: season, GW: gw}]
	if !ok {
		where := "the live capture series"
		if season != "" {
			where = season
		}
		return nil, fmt.Errorf("no capture covers GW%d of %s — this gameweek has no "+
			"recovered team news and must be reported as a gap, never filled", gw, where)
	}
	return s.readAt(season, e)
}

// readAt decompresses one capture and builds the answer.
func (s *Store) readAt(season string, e entry) (*Availability, error) {
	boot, err := Read(e.dir, BootstrapEndpoint)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", e.dir, err)
	}
	var payload struct {
		Elements []PlayerStatus `json:"elements"`
	}
	if err := json.Unmarshal(boot, &payload); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", e.dir, err)
	}

	a := &Availability{
		Season: season, Event: e.m.Event,
		SnapshotAt:   e.m.CapturedAt,
		Backfilled:   e.m.Backfill != nil,
		EventsBehind: -1,
		Players:      make(map[int]PlayerStatus, len(payload.Elements)),
	}
	if e.m.EventDeadline != nil {
		a.Deadline = *e.m.EventDeadline
	}
	if e.m.HoursToDeadline != nil {
		a.HoursBefore = *e.m.HoursToDeadline
	}

	// The staleness measure this package argues is the one to read first, computed
	// here rather than left to every consumer. An earlier version documented it at
	// length on Quality and then omitted it from the only type anybody reads, so the
	// first consumer of this data could not see that three "covered" gameweeks of
	// 2024-25 are byte-identical copies of one crawl.
	if q, err := VerifyPreDeadline(boot, e.m.Event, e.m.CapturedAt); err == nil {
		a.EventsBehind = q.EventsBehind
	}

	for _, p := range payload.Elements {
		if p.Code == 0 {
			continue // no permanent key, so nothing can safely be joined to it
		}
		a.Players[p.Code] = p
	}
	return a, nil
}

// Player answers the single question this whole backfill exists to answer: what was
// FPL saying about this footballer when this deadline came round.
func (s *Store) Player(season string, gw, code int) (PlayerStatus, bool, error) {
	a, err := s.At(season, gw)
	if err != nil {
		return PlayerStatus{}, false, err
	}
	p, ok := a.Players[code]
	return p, ok, nil
}

// There is deliberately no Coverage method here.
//
// One existed, returning a 38-row present-or-missing table, and `backfill.Rows` built
// the same table with two more columns. Two views of one quantity is this project's
// signature defect — the version somebody reads is reliably the one that knows less —
// so the richer builder is the only one, and it works from `Count`, `At` and the
// stored payloads, all of which are here.

// Manifests returns EVERY indexed manifest with the directory it came from, so a
// diagnostic can walk the whole store without re-implementing the index.
//
// Every one, not one per gameweek. An earlier version read `byGameweek` and so
// silently checked a fifth of the store at a denser cadence while printing a total
// that read as complete — and the captures it skipped were precisely those that no
// longer pass under the write-time check, since a hand-placed or later-corrupted file
// that is not the latest for its gameweek would never be re-verified by anything.
func (s *Store) Manifests() map[string]*Manifest {
	out := make(map[string]*Manifest, len(s.all))
	for _, e := range s.all {
		out[e.dir] = e.m
	}
	return out
}

// AllAt returns every capture covering one gameweek, closest to the deadline first.
//
// `At` answers "what was FPL saying at this deadline" and returns one answer, which is
// what almost every consumer wants. This is for the one that does not: a denser pull
// stores several crawls through a gameweek's run-up so the availability *trajectory*
// can be read, and without an enumerator those extra captures are unreachable — the
// cadence flag would spend requests on a third party's infrastructure to produce bytes
// no question could reach.
func (s *Store) AllAt(season string, gw int) []*Availability {
	var out []*Availability
	for _, e := range s.all {
		if e.m.Event != gw || s.seasonOf(e.dir) != season {
			continue
		}
		if a, err := s.readAt(season, e); err == nil {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SnapshotAt.After(out[j].SnapshotAt) })
	return out
}

// seasonOf recovers which season an indexed directory belongs to. The live series
// sits directly under the root and has no season; a backfilled one is one level down.
func (s *Store) seasonOf(dir string) string {
	rel, err := filepath.Rel(s.root, dir)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}
