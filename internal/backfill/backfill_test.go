package backfill

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"armband/internal/wayback"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func snaps(times ...string) []wayback.Snapshot {
	out := make([]wayback.Snapshot, 0, len(times))
	for _, s := range times {
		out = append(out, wayback.Snapshot{At: at(s), Original: "https://x/"})
	}
	return out
}

// TestSelectPreDeadlineTakesTheLastBeforeAndNotTheNearest is the test for the mistake
// this whole design is arranged around.
//
// Nearest is what you write if you are thinking about accuracy, and it is wrong in a
// way nothing downstream can see. FPL updates availability continuously, and hardest
// in the hour after a deadline as team sheets land — so a crawl forty minutes late is
// both nearer to the deadline and a strictly better forecast of that gameweek than
// anything a manager could have had. The resulting figures would be excellent and
// would be measuring hindsight.
func TestSelectPreDeadlineTakesTheLastBeforeAndNotTheNearest(t *testing.T) {
	deadline := at("2020-10-03T10:00:00Z")

	// The crawl 40 minutes AFTER the deadline is much nearer than the one 22 hours
	// before it. The rule must take the earlier one anyway.
	got, ok := SelectPreDeadline(snaps(
		"2020-10-02T12:00:00Z",
		"2020-10-03T10:40:00Z",
		"2020-10-01T08:00:00Z",
	), deadline)
	if !ok {
		t.Fatal("no snapshot selected")
	}
	if !got.At.Equal(at("2020-10-02T12:00:00Z")) {
		t.Fatalf("selected %s; the rule is the LAST crawl strictly before the deadline, "+
			"never the nearest", got.At.Format(time.RFC3339))
	}
}

// TestSelectPreDeadlineIsStrict pins the boundary. A crawl at the deadline instant is
// not before it, and `<=` here would be undetectable downstream.
func TestSelectPreDeadlineIsStrict(t *testing.T) {
	deadline := at("2020-10-03T10:00:00Z")
	if _, ok := SelectPreDeadline(snaps("2020-10-03T10:00:00Z"), deadline); ok {
		t.Error("a crawl at exactly the deadline was selected")
	}
	if got, ok := SelectPreDeadline(snaps("2020-10-03T09:59:59Z"), deadline); !ok ||
		!got.At.Equal(at("2020-10-03T09:59:59Z")) {
		t.Error("a crawl one second before the deadline should be selected")
	}
}

// TestSelectPreDeadlineReportsAGapRatherThanReachingForward pins the loud path.
//
// When every crawl is after the deadline the answer is "nothing", and the caller must
// report a gap. Reaching for the nearest crawl after the deadline "just for the weeks
// with no coverage" reintroduces the whole problem exactly where coverage is worst,
// which is not a random sample of weeks.
func TestSelectPreDeadlineReportsAGapRatherThanReachingForward(t *testing.T) {
	deadline := at("2020-10-03T10:00:00Z")
	if s, ok := SelectPreDeadline(snaps(
		"2020-10-03T10:00:01Z", "2020-10-04T09:00:00Z",
	), deadline); ok {
		t.Fatalf("selected %s from crawls that are all after the deadline", s.At)
	}
	if _, ok := SelectPreDeadline(nil, deadline); ok {
		t.Fatal("selected something from an empty index")
	}
}

// TestSelectPreDeadlineNeverLeaks is the property, over random inputs.
//
// The two tests above pin the cases somebody thought of. This one asserts the
// invariant itself across ten thousand shuffled indexes: whatever comes back is
// strictly earlier than the deadline, and it is the latest such crawl. Order of the
// input must not matter — the CDX index arrives sorted and a rule that quietly
// depends on that would break the day it does not.
func TestSelectPreDeadlineNeverLeaks(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	deadline := at("2020-10-03T10:00:00Z")

	for trial := 0; trial < 10000; trial++ {
		n := 1 + rng.Intn(12)
		in := make([]wayback.Snapshot, 0, n)
		for i := 0; i < n; i++ {
			// Spread across a fortnight either side, so roughly half the draws
			// land after the deadline and the boundary is hit often.
			offset := time.Duration(rng.Intn(14*24*60)-7*24*60) * time.Minute
			in = append(in, wayback.Snapshot{At: deadline.Add(offset)})
		}
		got, ok := SelectPreDeadline(in, deadline)

		var want time.Time
		for _, s := range in {
			if s.At.Before(deadline) && (want.IsZero() || s.At.After(want)) {
				want = s.At
			}
		}
		if want.IsZero() {
			if ok {
				t.Fatalf("trial %d: selected %s when nothing precedes the deadline", trial, got.At)
			}
			continue
		}
		if !ok {
			t.Fatalf("trial %d: reported a gap when %s precedes the deadline", trial, want)
		}
		if !got.At.Before(deadline) {
			t.Fatalf("trial %d: LEAK — selected %s, at or after the deadline %s",
				trial, got.At.Format(time.RFC3339), deadline.Format(time.RFC3339))
		}
		if !got.At.Equal(want) {
			t.Fatalf("trial %d: selected %s, want the latest preceding crawl %s",
				trial, got.At, want)
		}
	}
}

// TestSelectWindowAssignsEachCrawlToOneGameweek covers the denser cadence.
//
// At one per gameweek the rule has no lower bound, so a sparse stretch still yields
// honest — if stale — evidence. A denser pull must bound the window below at the
// previous deadline, or one crawl gets filed under two gameweeks and the second copy
// reads later as independent evidence when it is the same bytes.
func TestSelectWindowAssignsEachCrawlToOneGameweek(t *testing.T) {
	prev := at("2020-09-26T10:00:00Z")
	deadline := at("2020-10-03T10:00:00Z")
	in := snaps(
		"2020-09-20T09:00:00Z", // before the previous deadline: belongs to GW3, not GW4
		"2020-09-28T09:00:00Z",
		"2020-10-01T09:00:00Z",
		"2020-10-02T09:00:00Z",
		"2020-10-03T11:00:00Z", // after the deadline
	)
	got := SelectWindow(in, prev, deadline, 3)
	if len(got) != 3 {
		t.Fatalf("selected %d crawls, want 3", len(got))
	}
	// Latest first, and none from outside the run-up.
	want := []string{"2020-10-02T09:00:00Z", "2020-10-01T09:00:00Z", "2020-09-28T09:00:00Z"}
	for i, w := range want {
		if !got[i].At.Equal(at(w)) {
			t.Errorf("position %d is %s, want %s", i, got[i].At, w)
		}
	}
	for _, s := range got {
		if !s.At.Before(deadline) {
			t.Errorf("LEAK — %s is at or after the deadline", s.At)
		}
	}

	// With no lower bound and n = 1 it must agree exactly with SelectPreDeadline,
	// because that is the shipped cadence and two rules for one quantity is the
	// shape this project keeps paying for.
	one := SelectWindow(in, time.Time{}, deadline, 1)
	last, ok := SelectPreDeadline(in, deadline)
	if !ok || len(one) != 1 || !one[0].At.Equal(last.At) {
		t.Errorf("SelectWindow(n=1) gave %v, SelectPreDeadline gave %v", one, last)
	}
}

// season builds an aligned calendar: 38 weekly kickoffs with FPL's ninety-minute
// deadline before each.
func season() (deadlines, kickoffs map[int]time.Time) {
	deadlines, kickoffs = map[int]time.Time{}, map[int]time.Time{}
	base := at("2020-09-12T11:30:00Z")
	for gw := 1; gw <= 38; gw++ {
		k := base.Add(time.Duration(gw-1) * 7 * 24 * time.Hour)
		kickoffs[gw] = k
		deadlines[gw] = k.Add(-90 * time.Minute)
	}
	return deadlines, kickoffs
}

// TestCompareCatchesTheWrongSeason is the cross-check's reason for existing.
//
// Wayback windows are calendar dates and seasons overlap them: 2019-20 ran into July
// 2020 because of the pandemic, so a crawl fetched while looking for 2020-21 can carry
// the previous season's `events[]`. Every deadline would then be weeks out, every
// selection would find a plausible-looking crawl, and the season would be quietly
// mis-dated with nothing failing.
func TestCompareCatchesTheWrongSeason(t *testing.T) {
	deadlines, kickoffs := season()

	ok := Compare(deadlines, kickoffs)
	if err := ok.Err(); err != nil {
		t.Fatalf("a correctly aligned season failed the check: %v", err)
	}
	if ok.MedianGapHours < 1.4 || ok.MedianGapHours > 1.6 {
		t.Errorf("median gap %.2f h, want FPL's 1.5", ok.MedianGapHours)
	}

	// A previous season's calendar, out by a fortnight in either direction. Both
	// move the median by weeks, which is the signature of the wrong season.
	for _, shift := range []time.Duration{-14 * 24 * time.Hour, 3 * 24 * time.Hour} {
		moved := map[int]time.Time{}
		for gw, d := range deadlines {
			moved[gw] = d.Add(shift)
		}
		if err := Compare(moved, kickoffs).Err(); err == nil {
			t.Errorf("a calendar %v out of step passed the cross-check", shift)
		}
	}

	// And a check that compared almost nothing must not pass by default. An
	// unverified calendar mis-dates everything below it.
	thin := map[int]time.Time{1: kickoffs[1], 2: kickoffs[2]}
	if err := Compare(deadlines, thin).Err(); err == nil {
		t.Error("a cross-check over two gameweeks passed")
	}
}

// TestCompareToleratesProvisionalDeadlines is the correction to how this backfill was
// first specified, and it is a fact about FPL rather than about this code.
//
// A mid-season crawl carries **provisional** deadlines for gameweeks it has not
// reached, and they move. Measured on 2020-21 against a crawl from 27 January: GW1-24
// sit at FPL's rule of exactly +1.50 h, GW25-35 read −1.0 to −17.5 h because those
// fixtures were later moved to Friday and Saturday evening slots, and GW36-37 read
// +74 to +77 h because they were rescheduled into a different week. Ten of 38
// gameweeks look "impossible" and none of them is an error.
//
// So the check must pass a season like that — an earlier version failed it outright,
// which would have rejected every real season — while still reporting which gameweeks
// are provisional, because those are exactly the ones whose deadline has to be re-read
// before anything is selected against it.
func TestCompareToleratesProvisionalDeadlines(t *testing.T) {
	deadlines, kickoffs := season()

	// The 2020-21 shape: the back third of the season sits later than the truth.
	for gw := 25; gw <= 35; gw++ {
		deadlines[gw] = deadlines[gw].Add(17*time.Hour + 30*time.Minute)
	}
	// And two gameweeks rescheduled into a different week entirely.
	for _, gw := range []int{36, 37} {
		deadlines[gw] = deadlines[gw].Add(-75 * time.Hour)
	}

	c := Compare(deadlines, kickoffs)
	if err := c.Err(); err != nil {
		t.Fatalf("a real season with provisional deadlines was rejected: %v", err)
	}
	if c.MedianGapHours < 1.4 || c.MedianGapHours > 1.6 {
		t.Errorf("median gap %.2f h; the median must be untouched by tail drift", c.MedianGapHours)
	}
	if c.Inverted != 11 {
		t.Errorf("counted %d inverted gameweeks, want 11 (GW25-35)", c.Inverted)
	}
	if got := len(c.Provisional()); got != 13 {
		t.Errorf("Provisional() named %d gameweeks, want 13 — the 11 that drifted later "+
			"plus the 2 rescheduled", got)
	}
	// The set is what tells the resolver which deadlines it must not trust.
	for _, gw := range c.Provisional() {
		if gw < 25 {
			t.Errorf("GW%d is at FPL's rule but was named provisional", gw)
		}
	}
}

// TestSeasonWindowContainsTheSeasonEvenWhenTheCalendarMoved pins the query window.
//
// It only has to contain the deadlines — the cross-check is what establishes that
// what came back is the right season — so it is deliberately wide. Both recent
// exceptions are checked: 2020-21 started in September, and 2019-20 finished in July.
func TestSeasonWindowContainsTheSeasonEvenWhenTheCalendarMoved(t *testing.T) {
	from, to, err := SeasonWindow("2020-21")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"2020-09-12T10:00:00Z", "2021-05-23T15:00:00Z"} {
		if x := at(d); x.Before(from) || x.After(to) {
			t.Errorf("%s falls outside the query window %s..%s", d, from, to)
		}
	}
	if _, _, err := SeasonWindow("2020"); err == nil {
		t.Error("a malformed season name was accepted")
	}
	if _, _, err := SeasonWindow("twenty-21"); err == nil {
		t.Error("a malformed season name was accepted")
	}
}

// TestDeadlinesRoundTrip pins that a recovered calendar survives being written and
// read, including the cross-check finding — which is the part no refetch reproduces
// and therefore the reason the file exists at all rather than an HTTP cache entry.
func TestDeadlinesRoundTrip(t *testing.T) {
	root := t.TempDir()
	in := &Deadlines{
		Season: "2020-21",
		By:     map[int]time.Time{1: at("2020-09-12T10:00:00Z"), 38: at("2021-05-23T14:00:00Z")},
		Source: "https://web.archive.org/web/20210102000000id_/x",
		Check:  CrossCheck{Compared: 38, MedianGapHours: 1.5},
	}
	if err := SaveDeadlines(root, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadDeadlines(root, "2020-21")
	if err != nil {
		t.Fatal(err)
	}
	if !out.By[1].Equal(in.By[1]) || out.Check.Compared != 38 || out.Check.MedianGapHours != 1.5 {
		t.Fatalf("round trip lost something: %+v", out)
	}
	if _, err := LoadDeadlines(root, "2021-22"); err == nil {
		t.Error("a season with no calendar on disk loaded anyway")
	}
}

// TestRunRefusesToWorkWithoutTheCrossCheckInput pins that the archive witness is
// mandatory rather than nice to have. Without it a mis-dated season passes silently,
// which is the failure the cross-check exists to prevent.
func TestRunRefusesToWorkWithoutTheCrossCheckInput(t *testing.T) {
	_, err := Run(t.Context(), wayback.New(t.TempDir()), Options{
		Root: t.TempDir(), Season: "2020-21",
	})
	if err == nil {
		t.Fatal("a backfill ran with no kickoff times to check the calendar against")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Errorf("the refusal should say so plainly; got %v", err)
	}
}
