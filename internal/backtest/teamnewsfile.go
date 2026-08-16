package backtest

// The reader half of the recovered-team-news seam.
//
// `cmd/teamnewsexport` turns `data/captures` into two small tables and this reads
// them back. The tables are embedded rather than read from a path, for the reason
// `repairData` is: the data then travels with the binary and cannot resolve
// differently depending on which directory a test was invoked from. `go:embed`
// reaches only inside its own package tree, which is why they live here.
//
// This is the *only* implementation of TeamNews that ships. There is deliberately
// not a second one reading the capture store live: two expressions of one quantity,
// with the measured one not being the one that runs, is this project's signature
// defect, and it has arrived through a "convenience" reader more than once.

import (
	"embed"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"

	"armband/internal/stats"
)

//go:embed teamnewsdata/*.csv
var teamNewsData embed.FS

// TeamNewsFilter selects which recovered captures a source is allowed to use.
//
// # Why staleness is a filter and not a footnote
//
// A capture read two hours before a deadline is the team news. One read nine days
// before it is last week's, and it is wrong in a *direction*: a player flagged after
// the crawl reads as available, so a stale gameweek attenuates toward the unflagged
// population and drags the arm toward the baseline. That is a confound rather than
// noise — it biases the estimate toward zero — so the headline needs to say which
// captures produced it, and a run at a tighter threshold is the check.
//
// Read EventsBehind before HoursBefore. Nine days is fresh across an international
// break and badly stale inside a normal week, and only that field knows which: it
// counts how many deadlines had already passed when the payload was crawled, so 1
// means the capture predates the *previous* gameweek and cannot be about this one.
type TeamNewsFilter struct {
	// MaxHoursBefore drops a gameweek whose closest capture was read longer than
	// this before its deadline. Zero means no limit.
	MaxHoursBefore float64

	// MaxEventsBehind drops a gameweek whose capture predates more deadlines than
	// this. Zero keeps only captures taken inside the gameweek's own run-up.
	// Negative means no limit.
	//
	// The zero value is therefore the *strict* setting, which is the right default
	// for a field whose lax setting silently admits last week's news.
	MaxEventsBehind int
}

// TeamNewsTable is the embedded export, filtered.
//
// A pointer type because TeamNews requires a comparable dynamic type: Oracles is
// compared with != once per cell to prove an arm's hindsight does not vary by cell.
type TeamNewsTable struct {
	filter TeamNewsFilter

	// covered is the gameweeks that survived the filter, and it is the authority.
	// A gameweek missing here falls back to the reconstruction wholesale.
	covered map[seasonWeek]bool

	// flags is what FPL was publishing about the players it was saying anything
	// about. Everyone else in a covered gameweek is available with no percentage.
	flags map[seasonWeek]map[int]newsFlag

	// meta is every capture the export carried, filtered or not, so a diagnostic
	// can report what was dropped rather than only what survived.
	meta []newsMeta
}

type seasonWeek struct {
	Season string
	GW     int
}

type newsFlag struct {
	Status string
	Chance *int
}

// newsMeta is one gameweek's provenance, kept for reporting.
type newsMeta struct {
	Season       string
	GW           int
	HoursBefore  float64
	EventsBehind int
	Backfilled   bool
	Players      int
	Flagged      int
	Kept         bool
}

// LoadTeamNews reads the embedded export under a filter.
//
// It fails rather than returning an empty table when the export is missing or
// unreadable. An empty source would make every arm built on it inert, and an inert
// oracle reports exactly the same clean null as a real one — which is the failure
// this package has catalogued more often than any other.
func LoadTeamNews(f TeamNewsFilter) (*TeamNewsTable, error) {
	t := &TeamNewsTable{
		filter:  f,
		covered: map[seasonWeek]bool{},
		flags:   map[seasonWeek]map[int]newsFlag{},
	}
	if err := t.readCoverage(); err != nil {
		return nil, err
	}
	if len(t.covered) == 0 {
		return nil, fmt.Errorf("team news: no gameweek survives the filter "+
			"(max %.0fh before the deadline, at most %d deadlines behind) — an "+
			"empty source produces an oracle that changes nothing and reports it "+
			"as a null result", f.MaxHoursBefore, f.MaxEventsBehind)
	}
	if err := t.readFlags(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *TeamNewsTable) readCoverage() error {
	rows, err := readEmbeddedCSV("teamnewsdata/coverage.csv", 8)
	if err != nil {
		return err
	}
	for _, r := range rows {
		gw, err := strconv.Atoi(r[1])
		if err != nil {
			return fmt.Errorf("team news coverage: gameweek %q: %w", r[1], err)
		}
		hours, err := strconv.ParseFloat(r[3], 64)
		if err != nil {
			return fmt.Errorf("team news coverage: hours %q: %w", r[3], err)
		}
		behind, err := strconv.Atoi(r[4])
		if err != nil {
			return fmt.Errorf("team news coverage: events behind %q: %w", r[4], err)
		}
		players, _ := strconv.Atoi(r[6])
		flagged, _ := strconv.Atoi(r[7])

		m := newsMeta{
			Season: r[0], GW: gw, HoursBefore: hours, EventsBehind: behind,
			Backfilled: r[5] == "true", Players: players, Flagged: flagged,
		}
		m.Kept = t.keep(m)
		t.meta = append(t.meta, m)
		if m.Kept {
			t.covered[seasonWeek{r[0], gw}] = true
		}
	}
	return nil
}

// keep applies the filter to one capture.
//
// EventsBehind of -1 means the payload could not say, which is treated as failing
// any events-behind limit rather than passing it: an unknown staleness is not a
// verified-fresh one, and this is the direction that keeps a stale flag out of a
// headline rather than into it.
func (t *TeamNewsTable) keep(m newsMeta) bool {
	if t.filter.MaxHoursBefore > 0 && m.HoursBefore > t.filter.MaxHoursBefore {
		return false
	}
	if t.filter.MaxEventsBehind >= 0 {
		if m.EventsBehind < 0 || m.EventsBehind > t.filter.MaxEventsBehind {
			return false
		}
	}
	return true
}

func (t *TeamNewsTable) readFlags() error {
	rows, err := readEmbeddedCSV("teamnewsdata/flags.csv", 5)
	if err != nil {
		return err
	}
	for _, r := range rows {
		gw, err := strconv.Atoi(r[1])
		if err != nil {
			return fmt.Errorf("team news flags: gameweek %q: %w", r[1], err)
		}
		key := seasonWeek{r[0], gw}
		if !t.covered[key] {
			continue
		}
		code, err := strconv.Atoi(r[2])
		if err != nil {
			return fmt.Errorf("team news flags: code %q: %w", r[2], err)
		}
		f := newsFlag{Status: r[3]}
		if r[4] != "" {
			v, err := strconv.Atoi(r[4])
			if err != nil {
				return fmt.Errorf("team news flags: chance %q: %w", r[4], err)
			}
			// A fresh pointer per row. Nil is not zero here: nil means FPL
			// published no figure, which for a fit player is the normal state,
			// while 0 means it published a definite "will not play".
			f.Chance = &v
		}
		if t.flags[key] == nil {
			t.flags[key] = map[int]newsFlag{}
		}
		t.flags[key][code] = f
	}
	return nil
}

func readEmbeddedCSV(name string, fields int) ([][]string, error) {
	f, err := teamNewsData.Open(name)
	if err != nil {
		return nil, fmt.Errorf("team news: %s is not embedded — regenerate it with "+
			"cmd/teamnewsexport: %w", name, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = fields
	var out [][]string
	// The header is read and discarded rather than skipped by index, so a file
	// whose columns have been reordered fails on the field count instead of being
	// read into the wrong variables.
	if _, err := r.Read(); err != nil {
		return nil, fmt.Errorf("team news: %s has no header: %w", name, err)
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("team news: %s: %w", name, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

// Covers implements TeamNews.
func (t *TeamNewsTable) Covers(season string, gw int) bool {
	return t.covered[seasonWeek{season, gw}]
}

// FlagAt implements TeamNews.
//
// An unflagged player in a covered gameweek is available with no published
// percentage, which is the normal state of a fit footballer and is what the export
// omits to keep itself to a hundred thousand rows rather than a million.
func (t *TeamNewsTable) FlagAt(season string, gw, code int) (string, *int, bool) {
	key := seasonWeek{season, gw}
	if !t.covered[key] {
		return "", nil, false
	}
	if f, ok := t.flags[key][code]; ok {
		return f.Status, f.Chance, true
	}
	return "a", nil, true
}

// Summary is what a diagnostic prints before it reports a number: which seasons the
// source covers, how fresh the captures were, and how much was dropped.
//
// It is here rather than in the diagnostic because the answer depends on the filter,
// and a coverage figure computed anywhere other than beside the filter that produced
// it is the shape of claim this project keeps finding falsified.
func (t *TeamNewsTable) Summary() []TeamNewsSeasonSummary {
	bySeason := map[string]*TeamNewsSeasonSummary{}
	var order []string
	for _, m := range t.meta {
		s, ok := bySeason[m.Season]
		if !ok {
			s = &TeamNewsSeasonSummary{Season: m.Season}
			bySeason[m.Season] = s
			order = append(order, m.Season)
		}
		s.Captured++
		if !m.Kept {
			s.Dropped++
			continue
		}
		s.Kept++
		s.Flagged += m.Flagged
		s.hours = append(s.hours, m.HoursBefore)
		if m.EventsBehind > 0 {
			s.StaleByEvent++
		}
	}
	sort.Strings(order)
	out := make([]TeamNewsSeasonSummary, 0, len(order))
	for _, name := range order {
		s := bySeason[name]
		// The ordinary median, which is the estimator the recorded per-season range
		// of 4.7 to 8.5 hours was measured under. Taking the upper of the two middle
		// values instead — as nine of this repo's other ten copies once did — would
		// move that range to 6.0 to 8.6 with no data changing.
		s.MedianHours = stats.Median(s.hours)
		out = append(out, *s)
	}
	return out
}

// TeamNewsSeasonSummary is one season's coverage under the filter in force.
type TeamNewsSeasonSummary struct {
	Season string
	// Captured is gameweeks with any capture at all; Kept is those that survived
	// the filter and Dropped the rest. Reported separately because "we have no
	// crawl" and "we have a crawl and refused it" are different facts.
	Captured, Kept, Dropped int
	// Flagged is player-gameweeks FPL was saying something about, across the kept
	// gameweeks. It is the mediator: an arm whose flagged count is near zero
	// cannot move anything, whatever its headline says.
	Flagged int
	// StaleByEvent counts kept gameweeks whose capture predates an earlier
	// deadline, which is only reachable with MaxEventsBehind relaxed.
	StaleByEvent int
	MedianHours  float64

	hours []float64
}
