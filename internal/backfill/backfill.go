// Package backfill recovers point-in-time FPL team news for finished seasons.
//
// # What it is for
//
// The season archive this project replays is an end-of-season photograph. A player
// injured from September to November finishes the season fit, and looks fit in every
// weekly row of the record. So the replay cannot see who was actually unavailable
// when a decision was taken: `backtest.statusAt` reconstructs a guess from the final
// status plus the date its news was posted, which can carry a season-ending absence
// backwards and can say nothing whatever about one that resolved.
//
// The absences that resolve are the population every rotation-risk constant needs.
// `internal/capture` was built to record this going forward and says plainly in its
// own doc comment that it "yields nothing this season". This package retires that
// sentence by recovering the same quantity from the Internet Archive's crawls of
// `bootstrap-static`, which carried `status`, `chance_of_playing_next_round`, `news`
// and `news_added` all along.
//
// # What it deliberately does not do
//
// It does not touch the replay's scoring path and it does not change `statusAt`.
// Every measured figure in the research record was produced with the reconstruction
// as it stands, and switching the input underneath them would make the record
// incomparable with itself while looking like nothing had happened. Delivering the
// data and proving it honest is this job; using it is the next one, with its own
// measurement pass.
//
// # The one rule that must not be got wrong
//
// A snapshot is taken from **strictly before** the deadline, never the nearest one.
// A crawl from two hours after a deadline contains team news the manager did not
// have, and every figure derived from it would look entirely plausible. See
// `SelectPreDeadline`, and `capture.VerifyPreDeadline` for the check that refuses to
// store a body that cannot prove its own timing from the inside.
package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"armband/internal/capture"
	"armband/internal/stats"
	"armband/internal/wayback"
)

// Target is the URL whose crawls carry the availability block.
//
// Bare, with no scheme: that is the form the CDX index matches on.
const Target = "fantasy.premierleague.com/api/bootstrap-static/"

// Seasons are the seasons the Internet Archive can support, and the single list of
// them — the CLI had a second copy, which is two sources of truth for one quantity.
//
// The floor of 2020-21 is **asserted from the brief this work was given and only
// partly verified here.** What was checked directly is that 2020-21 through 2025-26
// all yield a crawl before every one of their 38 deadlines. What was NOT checked is
// the stated reason for stopping there — that FPL's pre-2020 `/drf/` endpoints hold
// of the order of ten captures a year, too sparse to place against 38 deadlines. That
// figure is inherited, not measured, and anyone wanting an earlier season should
// query the index for it rather than trust this comment.
var Seasons = []string{"2020-21", "2021-22", "2022-23", "2023-24", "2024-25", "2025-26"}

// seasonName is the archive's season form, and the only shape allowed to become a
// path component. Checked because `startYear` constrains only the first field, so
// "2020-21/../../elsewhere" parses happily and would otherwise reach filepath.Join.
var seasonName = regexp.MustCompile(`^\d{4}-\d{2}$`)

// ValidSeason reports whether a season name is safe to use as a directory.
func ValidSeason(season string) bool { return seasonName.MatchString(season) }

// SelectPreDeadline returns the last snapshot strictly before a deadline.
//
// # Why last-before and not nearest
//
// Nearest is the natural thing to write and it is wrong in a way nothing downstream
// could detect. FPL updates `status` and `chance_of_playing_next_round` continuously,
// including in the hours after a deadline when the team sheets land — so a crawl
// forty minutes *after* a deadline is both the nearest one and a strictly better
// prediction of that gameweek than anything a manager could have had. Figures built
// on it would be excellent, plausible, and measuring hindsight.
//
// The comparison is strict. A crawl at exactly the deadline instant is not before
// it, and the cost of excluding it is one snapshot in a hundred thousand against a
// rule with no edge case in it.
//
// `ok` is false when nothing qualifies, and the caller must report that gameweek as
// a gap. Reaching for the nearest snapshot after the deadline "just for that one
// week" reintroduces the whole problem for the weeks where coverage is worst, which
// are not a random sample of weeks.
func SelectPreDeadline(snaps []wayback.Snapshot, deadline time.Time) (wayback.Snapshot, bool) {
	var best wayback.Snapshot
	found := false
	for _, s := range snaps {
		if !s.At.Before(deadline) {
			continue
		}
		if !found || s.At.After(best.At) {
			best, found = s, true
		}
	}
	return best, found
}

// SelectWindow returns up to n snapshots strictly before deadline and strictly after
// after, latest first.
//
// The lower bound exists so a denser pull does not assign one crawl to two
// gameweeks: with `after` set to the previous deadline, every snapshot belongs to
// exactly one gameweek's run-up. At n = 1 and a zero `after` this is exactly
// `SelectPreDeadline`, which is the required cadence and the only one run so far.
func SelectWindow(snaps []wayback.Snapshot, after, deadline time.Time, n int) []wayback.Snapshot {
	var in []wayback.Snapshot
	for _, s := range snaps {
		if !s.At.Before(deadline) {
			continue
		}
		if !after.IsZero() && !s.At.After(after) {
			continue
		}
		in = append(in, s)
	}
	sort.Slice(in, func(i, j int) bool { return in[i].At.After(in[j].At) })
	if n > 0 && len(in) > n {
		in = in[:n]
	}
	return in
}

// Deadlines is a season's gameweek calendar, as FPL published it.
type Deadlines struct {
	Season string `json:"season"`

	// By maps gameweek to deadline. A map rather than a slice because a season
	// whose calendar could not be fully recovered must be missing entries rather
	// than carrying zero values, which read as 1 January year zero and would sort
	// before every snapshot.
	By map[int]time.Time `json:"deadlines"`

	// Confirmed marks a gameweek whose deadline has been re-read from a crawl taken
	// inside its own run-up, and is therefore final rather than provisional.
	//
	// The distinction is not cosmetic. A deadline read from a distant crawl can be
	// **later** than the true one — up to 17.5 hours on 2020-21 and 42 hours on
	// 2024-25 — and selecting "the last crawl before a deadline that is too late" is
	// how a crawl from after the real deadline gets *selected*, and then refused at
	// the store boundary and lost as a gap.
	//
	// It can also be **earlier**: 2020-21's GW36 and GW37 moved 74 and 77 hours
	// later than the provisional figure, because they were rescheduled into a
	// different week. So `resolve` widens as well as tightens, and an unconfirmed
	// deadline is an estimate in both directions rather than an upper bound.
	Confirmed map[int]bool `json:"confirmed,omitempty"`

	// Source records which crawl the calendar was first read out of, so the claim
	// is checkable later. A calendar is the input to every selection below it, and
	// a wrong one would mis-date the entire season without failing anything.
	Source     string    `json:"source"`
	SourceAt   time.Time `json:"source_at"`
	RecordedAt time.Time `json:"recorded_at"`

	// Check is what the season archive said about the calendar as first read. It is
	// not refreshed as deadlines are confirmed, because its job is to record what
	// the discovery crawl claimed and how far that turned out to be off.
	Check CrossCheck `json:"cross_check"`
}

// Sorted returns the gameweeks present, ascending.
func (d Deadlines) Sorted() []int {
	out := make([]int, 0, len(d.By))
	for gw := range d.By {
		out = append(out, gw)
	}
	sort.Ints(out)
	return out
}

// CrossCheck compares FPL's published deadlines against the season archive's kickoff
// times, which is the only independent witness this project has.
//
// # What is being checked, and what is authoritative
//
// These are not two measurements of one quantity with a winner to pick. FPL's
// `deadline_time` is the authority on when a decision had to be taken and nothing
// else here carries it; the archive's kickoff times are a different quantity, ninety
// minutes later, from a different source. What the comparison establishes is
// **alignment**: that the calendar just downloaded describes the same 38 gameweeks
// the replay will score, from the same season.
//
// It matters because the failure it catches is otherwise silent. Wayback windows are
// calendar dates and seasons overlap them — 2019-20 ran into July 2020 because of the
// pandemic, so a crawl fetched while looking for 2020-21 can carry the *previous*
// season's events[]. Every deadline would then be weeks out, every selection would
// find a plausible-looking crawl, and the whole season would be quietly mis-dated.
//
// # The statistic is the MEDIAN gap, and the tails are real
//
// The obvious rule is that every gap must be positive and about ninety minutes, since
// a match cannot kick off before its own deadline. **That rule is wrong, and finding
// out why is the most useful thing this check has done.** Measured on 2020-21 against
// a crawl from 26 January 2021:
//
//	GW1-24   gap = +1.50 h exactly — FPL's published rule, on gameweeks already
//	         played or imminent when the crawl was taken
//	GW25-35  gap = −1.0 to −17.5 h — the crawl's deadlines for FUTURE gameweeks are
//	         PROVISIONAL, and moved earlier when broadcasters later picked those
//	         fixtures for Friday and Saturday evening slots
//	GW36-37  gap = +74 to +77 h — those gameweeks were subsequently rescheduled into
//	         a different week entirely
//
// So ten of 38 gameweeks read as "impossible" and none of them is an error: FPL's
// gameweek calendar is **not static within a season**, and a mid-season crawl is only
// authoritative about the gameweeks up to itself. That is a correction to how this
// backfill was originally specified — see `Run`, which resolves each gameweek's
// deadline from a crawl taken near it instead of trusting one calendar for all 38.
//
// The median is untouched by any of it and reads **1.50 h exactly**, which is FPL's
// own rule recovered from data. That makes it the right alignment statistic: a
// wrong-season calendar is out by a week or more and moves the median by that much,
// while provisional drift and rescheduling move only the tails.
type CrossCheck struct {
	Rows     []CrossCheckRow `json:"rows"`
	Compared int             `json:"compared"`

	// MedianGapHours is the alignment statistic. FPL's rule puts it at 1.50.
	MedianGapHours float64 `json:"median_gap_hours"`

	// Inverted counts gameweeks whose first kickoff precedes the deadline recorded
	// for them, and WorstInvertedHours is the largest such gap. Both are reported
	// rather than fatal: they measure how provisional this calendar is, which is
	// exactly what tells `Run` which gameweeks need their deadline re-read.
	Inverted           int     `json:"inverted"`
	WorstInvertedHours float64 `json:"worst_inverted_hours"`

	// Drifted counts gameweeks more than an hour away from the median gap — the
	// provisional and rescheduled ones together.
	Drifted int `json:"drifted"`
}

// CrossCheckRow is one gameweek's comparison.
type CrossCheckRow struct {
	Event        int       `json:"event"`
	Deadline     time.Time `json:"deadline"`
	FirstKickoff time.Time `json:"first_kickoff"`
	GapHours     float64   `json:"gap_hours"`
}

// Compare builds the cross-check. `kickoffs` maps gameweek to the earliest kickoff
// the season archive records for it; gameweeks the archive does not cover are simply
// not compared, and the count of what *was* compared is reported so a season that
// checked almost nothing cannot pass by default.
func Compare(deadlines map[int]time.Time, kickoffs map[int]time.Time) CrossCheck {
	var c CrossCheck
	var gaps []float64
	for gw := 1; gw <= 38; gw++ {
		d, okD := deadlines[gw]
		k, okK := kickoffs[gw]
		if !okD || !okK || d.IsZero() || k.IsZero() {
			continue
		}
		gap := k.Sub(d).Hours()
		c.Rows = append(c.Rows, CrossCheckRow{Event: gw, Deadline: d, FirstKickoff: k, GapHours: gap})
		c.Compared++
		if gap <= 0 {
			c.Inverted++
			if gap < c.WorstInvertedHours {
				c.WorstInvertedHours = gap
			}
		}
		gaps = append(gaps, gap)
	}
	if len(gaps) > 0 {
		// This recovers FPL's published ninety-minute rule from data rather than
		// summarising a spread, so an *observed* gap sounded like the right thing
		// to report and this was an inline upper median. It makes no difference:
		// the rule puts every gap at 1.50 h, so both middle values are 1.50 and
		// both estimators return it. The recorded "1.50 h exactly in all six
		// seasons" is unchanged, and MaxMedianGapHours bounds it at 6.
		c.MedianGapHours = stats.Median(gaps)
		for _, g := range gaps {
			if g-c.MedianGapHours > 1 || c.MedianGapHours-g > 1 {
				c.Drifted++
			}
		}
	}
	return c
}

// MinCompared is how many gameweeks must line up before the check means anything.
//
// **Asserted.** Twenty of 38 is a bit over half, chosen so that a season whose
// archive coverage is partial can still be validated while one that compared three
// gameweeks cannot pass by accident. Nothing was swept to pick it; it is a
// this-is-obviously-too-few line, not a calibrated one.
const MinCompared = 20

// MaxMedianGapHours bounds the plausible median distance from a deadline to the first
// kickoff of its gameweek.
//
// **The median is measured on six seasons; the bound of 6 is asserted.** FPL's
// published rule is ninety minutes, and the median deadline-to-first-kickoff gap reads
// **1.50 h exactly in every one of 2020-21 through 2025-26** — 227 gameweeks compared,
// one short of 228 because 2022-23's GW7 was cancelled outright and the archive
// carries no fixture for it. Six independent seasons recovering the published rule to
// the minute is about as good as support gets here.
//
// What is *not* swept is the width of the band around it. Six hours leaves room for a
// season dragged by rescheduled openers without admitting anything near the week-scale
// error a wrong season produces, and the thing it has to separate — 1.5 h from 168 h —
// is not close. The margin is measured too: the median survives tail drift only while
// fewer than half the gameweeks are provisional, and the worst season on disk is
// 2020-21 with 12 of 38.
const MaxMedianGapHours = 6

// Err reports a calendar that does not line up with the football.
//
// Note what is deliberately *not* fatal: individual gameweeks whose kickoff precedes
// their recorded deadline. That reads as impossible and is not — see CrossCheck for
// the measurement — because a mid-season crawl carries provisional deadlines for
// gameweeks it has not reached. Failing on it would reject every real season. `Run`
// handles those by re-reading each gameweek's deadline from a crawl near it.
func (c CrossCheck) Err() error {
	if c.Compared < MinCompared {
		return fmt.Errorf("only %d gameweeks could be compared against the season archive "+
			"(need %d): the calendar is unverified, and an unverified calendar mis-dates "+
			"every snapshot below it", c.Compared, MinCompared)
	}
	if c.MedianGapHours <= 0 || c.MedianGapHours > MaxMedianGapHours {
		return fmt.Errorf("the median gap from deadline to first kickoff is %.2f h against "+
			"FPL's published rule of 1.5 h (accepted range 0 to %.0f h) over %d gameweeks; "+
			"this calendar does not describe this season",
			c.MedianGapHours, float64(MaxMedianGapHours), c.Compared)
	}
	return nil
}

// Provisional lists the gameweeks whose recorded deadline disagrees with the archive
// by more than an hour, which is the set whose deadline must be re-read from a crawl
// taken nearer to them before anything is selected against it.
func (c CrossCheck) Provisional() []int {
	var out []int
	for _, r := range c.Rows {
		if r.GapHours-c.MedianGapHours > 1 || c.MedianGapHours-r.GapHours > 1 {
			out = append(out, r.Event)
		}
	}
	return out
}

// SeasonWindow is the calendar range to ask the Archive about for a season.
//
// It opens on 1 July of the starting year and closes on 1 August of the next. That is
// wider than any season and deliberately so: the window only has to *contain* the
// deadlines, and the cross-check is what establishes that what came back is the right
// season. A tight window would trade a check that works for one that has to be right
// about a calendar that has moved twice in recent memory — 2020-21 started in
// September and 2019-20 finished in July.
func SeasonWindow(season string) (from, to time.Time, err error) {
	y, err := startYear(season)
	if err != nil {
		return from, to, err
	}
	from = time.Date(y, time.July, 1, 0, 0, 0, 0, time.UTC)
	to = time.Date(y+1, time.August, 1, 0, 0, 0, 0, time.UTC)
	return from, to, nil
}

// startYear parses "2020-21" into 2020.
func startYear(season string) (int, error) {
	if !ValidSeason(season) {
		return 0, fmt.Errorf("season %q is not in the archive's YYYY-YY form, e.g. 2020-21", season)
	}
	parts := strings.Split(season, "-")
	if len(parts) != 2 || len(parts[0]) != 4 {
		return 0, fmt.Errorf("season %q is not in the archive's YYYY-YY form, e.g. 2020-21", season)
	}
	y, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("season %q is not in the archive's YYYY-YY form, e.g. 2020-21", season)
	}
	return y, nil
}

// deadlinesPath is where a season's recovered calendar is cached.
//
// Beside the captures rather than in the HTTP cache, because it is a finding about
// the season and not a copy of a response: it carries the cross-check result, which
// no refetch would reproduce. Keeping it here is also what lets a re-run and a
// coverage report work with no network at all.
func deadlinesPath(root, season string) string {
	return filepath.Join(capture.SeasonDir(root, season), "deadlines.json")
}

// LoadDeadlines reads a season's cached calendar. Missing is not an error; the caller
// discovers it instead.
func LoadDeadlines(root, season string) (*Deadlines, error) {
	b, err := os.ReadFile(deadlinesPath(root, season))
	if err != nil {
		return nil, err
	}
	var d Deadlines
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	if len(d.By) == 0 {
		return nil, fmt.Errorf("%s carries no deadlines", deadlinesPath(root, season))
	}
	return &d, nil
}

// SaveDeadlines writes a season's calendar.
func SaveDeadlines(root string, d *Deadlines) error {
	dir := capture.SeasonDir(root, d.Season)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(deadlinesPath(root, d.Season), append(b, '\n'), 0o644)
}

// DiscoverDeadlines is pass one: fetch one snapshot from the middle of the season and
// read all 38 deadlines out of its `events[]`.
//
// One fetch, not 38. FPL publishes the whole season's calendar in every payload, so
// asking each gameweek separately would be 38 requests to somebody else's free
// infrastructure for 38 copies of one answer.
//
// The candidate is chosen nearest the middle of the football rather than the middle
// of the calendar window, because the window is deliberately wider than the season
// and its midpoint can land in a summer with the previous season's payload still
// being served. `midpoint` comes from the season archive's own kickoff times, so the
// snapshot is picked using the season we are actually trying to describe.
func DiscoverDeadlines(ctx context.Context, c Source, season string,
	snaps []wayback.Snapshot, midpoint time.Time, kickoffs map[int]time.Time) (*Deadlines, error) {

	if len(snaps) == 0 {
		return nil, fmt.Errorf("the Internet Archive has no 200-status crawls of %s in "+
			"%s's window at all, so this season cannot be backfilled from it", Target, season)
	}
	ordered := append([]wayback.Snapshot(nil), snaps...)
	sort.Slice(ordered, func(i, j int) bool {
		di := absDuration(ordered[i].At.Sub(midpoint))
		dj := absDuration(ordered[j].At.Sub(midpoint))
		if di == dj {
			return ordered[i].At.Before(ordered[j].At)
		}
		return di < dj
	})

	// Try a few candidates. A single crawl can be a truncated body or, at a season
	// boundary, the previous season's calendar — both are recoverable by asking a
	// different one, and neither is worth failing the whole season over. What is
	// not recoverable is accepting a wrong calendar, which is why every candidate
	// must pass the archive cross-check before it is believed.
	const candidates = 5
	var lastErr error
	for i := 0; i < candidates && i < len(ordered); i++ {
		s := ordered[i]
		body, err := c.Fetch(ctx, s)
		if err != nil {
			lastErr = err
			continue
		}
		events, err := capture.ParseEvents(body)
		if err != nil {
			lastErr = err
			continue
		}
		by := map[int]time.Time{}
		for _, e := range events {
			if e.ID >= 1 && e.ID <= 38 && !e.DeadlineTime.IsZero() {
				by[e.ID] = e.DeadlineTime.UTC()
			}
		}
		if len(by) == 0 {
			lastErr = fmt.Errorf("the crawl at %s carries no usable deadlines", s.At)
			continue
		}
		check := Compare(by, kickoffs)
		if err := check.Err(); err != nil {
			lastErr = fmt.Errorf("the crawl at %s does not describe %s: %w", s.At, season, err)
			continue
		}
		return &Deadlines{
			Season: season, By: by,
			Source: s.RawURL(), SourceAt: s.At, RecordedAt: time.Now().UTC(),
			Check: check,
		}, nil
	}
	return nil, fmt.Errorf("no crawl near the middle of %s yielded a calendar that agrees "+
		"with the season archive after %d attempts: %w", season, candidates, lastErr)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
