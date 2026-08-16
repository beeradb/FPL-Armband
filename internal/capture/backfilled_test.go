package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// boot builds a bootstrap-static body good enough to be interrogated about its own
// timing: 38 weekly gameweeks with `target`'s deadline set exactly, `next` marked as
// the gameweek FPL considers upcoming, and everything before it finished.
//
// Built rather than fixtured because the properties under test are relations between
// fields — deadline against crawl time, `is_next` against the gameweek claimed — and
// a frozen fixture would pin one combination where the interesting thing is the grid.
func boot(t *testing.T, target int, deadline string, next int) []byte {
	t.Helper()
	base, err := time.Parse(time.RFC3339, deadline)
	if err != nil {
		t.Fatal(err)
	}
	type element struct {
		Code       int    `json:"code"`
		ID         int    `json:"id"`
		WebName    string `json:"web_name"`
		Status     string `json:"status"`
		ChanceNext *int   `json:"chance_of_playing_next_round"`
		News       string `json:"news"`
		NewsAdded  string `json:"news_added"`
	}
	seventyFive := 75
	payload := struct {
		Events   []Event   `json:"events"`
		Elements []element `json:"elements"`
	}{
		Elements: []element{
			{Code: 118748, ID: 4, WebName: "Fit", Status: "a"},
			{Code: 154043, ID: 7, WebName: "Doubtful", Status: "d", ChanceNext: &seventyFive,
				News: "Calf injury - 75% chance of playing", NewsAdded: "2020-09-11T11:00:08.600094Z"},
		},
	}
	for gw := 1; gw <= 38; gw++ {
		payload.Events = append(payload.Events, Event{
			ID:           gw,
			DeadlineTime: base.Add(time.Duration(gw-target) * 7 * 24 * time.Hour),
			Finished:     next > 0 && gw < next,
			IsNext:       gw == next,
			IsCurrent:    next > 1 && gw == next-1,
		})
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestVerifyPreDeadlineRefusesACrawlThatIsNotStrictlyEarlier is the boundary the whole
// backfill turns on.
//
// A crawl one second before a deadline is evidence. One at the deadline instant, or a
// second after it, is not — FPL updates `status` and `chance_of_playing_next_round`
// continuously, including in the minutes after a deadline when team sheets firm up.
// The comparison is therefore strict, and the equality case is tested explicitly
// because it is the one a `<=` typo would let through and nothing downstream could
// detect.
func TestVerifyPreDeadlineRefusesACrawlThatIsNotStrictlyEarlier(t *testing.T) {
	deadline := time.Date(2020, 10, 3, 10, 0, 0, 0, time.UTC)
	body := boot(t, 4, "2020-10-03T10:00:00Z", 4)

	for _, tc := range []struct {
		name string
		at   time.Time
		want bool // want an error
	}{
		{"a day before", deadline.Add(-24 * time.Hour), false},
		{"a second before", deadline.Add(-time.Second), false},
		{"exactly at the deadline", deadline, true},
		{"a second after", deadline.Add(time.Second), true},
		{"an hour after, when the team sheets are in", deadline.Add(time.Hour), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := VerifyPreDeadline(body, 4, tc.at)
			if tc.want && err == nil {
				t.Fatalf("a crawl at %s was accepted as evidence for a deadline at %s",
					tc.at.Format(time.RFC3339), deadline.Format(time.RFC3339))
			}
			if !tc.want && err != nil {
				t.Fatalf("a crawl at %s was refused: %v", tc.at.Format(time.RFC3339), err)
			}
		})
	}
}

// TestVerifyPreDeadlineRefusesAPayloadThatHasMovedOn checks the payload-internal half
// of the proof, which needs no clock at all.
//
// If a body were served after GW4's deadline, FPL's own bookkeeping would say so
// twice over — `is_next` would have advanced past 4, and GW4 would eventually read as
// finished. Both are caught here, and they matter because they do not depend on
// trusting the Internet Archive's crawler clock, which is the one external number the
// rest of the argument rests on.
func TestVerifyPreDeadlineRefusesAPayloadThatHasMovedOn(t *testing.T) {
	at := time.Date(2020, 10, 2, 9, 0, 0, 0, time.UTC)

	// The payload's own is_next has advanced to GW5 while we claim it predates
	// GW4's deadline. That is a body from after the fact, whatever its timestamp.
	if _, err := VerifyPreDeadline(boot(t, 4, "2020-10-03T10:00:00Z", 5), 4, at); err == nil {
		t.Error("a payload whose is_next is GW5 was accepted as evidence for GW4's deadline")
	}

	// GW4 already finished.
	body := boot(t, 4, "2020-10-03T10:00:00Z", 6)
	if _, err := VerifyPreDeadline(body, 4, at); err == nil {
		t.Error("a payload reporting GW4 as finished was accepted as evidence for its deadline")
	}

	// A body about a different season entirely: no such gameweek.
	if _, err := VerifyPreDeadline([]byte(`{"events":[{"id":1,"deadline_time":"2020-09-12T10:00:00Z"}]}`),
		4, at); err == nil {
		t.Error("a payload with no GW4 was accepted as evidence about GW4")
	}

	// And a body with no calendar at all cannot be dated from the inside, so it is
	// not usable however plausible its filename.
	if _, err := VerifyPreDeadline([]byte(`{"elements":[]}`), 4, at); err == nil {
		t.Error("a payload with no events[] was accepted as point-in-time evidence")
	}
}

// TestVerifyPreDeadlineCountsTheDeadlinesAlreadyPassed pins the staleness measure.
//
// Hours before a deadline is the obvious quality field and it is the weaker one: nine
// days before a deadline is fresh across an international break and badly stale inside
// a normal week. Counting how many deadlines had already gone by answers that
// directly, out of FPL's own bookkeeping, with no calendar arithmetic — and -1 has to
// stay distinguishable from 0, because a zero that quietly means "could not tell"
// reads as the freshest possible row.
func TestVerifyPreDeadlineCountsTheDeadlinesAlreadyPassed(t *testing.T) {
	at := time.Date(2020, 10, 2, 9, 0, 0, 0, time.UTC)

	q, err := VerifyPreDeadline(boot(t, 4, "2020-10-03T10:00:00Z", 4), 4, at)
	if err != nil {
		t.Fatal(err)
	}
	if q.EventsBehind != 0 {
		t.Errorf("a crawl inside GW4's own run-up reads as %d deadlines behind, want 0", q.EventsBehind)
	}

	// The same crawl offered as evidence for GW6: honest, because it predates GW6's
	// deadline, but two deadlines stale and it must say so.
	q, err = VerifyPreDeadline(boot(t, 4, "2020-10-03T10:00:00Z", 4), 6, at)
	if err != nil {
		t.Fatal(err)
	}
	if q.EventsBehind != 2 {
		t.Errorf("a GW4-era crawl offered for GW6 reads as %d deadlines behind, want 2", q.EventsBehind)
	}

	// No is_next anywhere: unknown, which must not read as fresh.
	q, err = VerifyPreDeadline(boot(t, 4, "2020-10-03T10:00:00Z", 0), 4, at)
	if err != nil {
		t.Fatal(err)
	}
	if q.EventsBehind != -1 {
		t.Errorf("a payload naming no next gameweek reads as %d, want -1 for unknown", q.EventsBehind)
	}
}

// TestWriteRefusesToStoreALeakyCapture pins that the check happens where bytes enter
// the store, not where they are consumed.
//
// A quarantine would be worse than a refusal: this package has twice shipped a repair
// that silently applied nothing, and a directory of rejected-but-present captures is
// the same shape — something later reads it as data.
func TestWriteRefusesToStoreALeakyCapture(t *testing.T) {
	root := t.TempDir()
	deadline := time.Date(2020, 10, 3, 10, 0, 0, 0, time.UTC)

	_, _, err := Backfilled{
		Root: root, Season: "2020-21", Event: 4,
		Body:   boot(t, 4, "2020-10-03T10:00:00Z", 5),
		At:     deadline.Add(time.Hour),
		Source: "https://web.archive.org/web/20201003110000id_/x", Stamp: "20201003110000",
	}.Write()
	if err == nil {
		t.Fatal("a crawl from after the deadline was stored")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Errorf("the refusal should say so plainly; got %v", err)
	}

	// And nothing was left behind for something else to find later.
	entries, _ := os.ReadDir(SeasonDir(root, "2020-21"))
	if len(entries) != 0 {
		t.Errorf("a refused capture left %d entries on disk", len(entries))
	}
}

// TestBackfilledRoundTripsKeyedByPermanentCode is the read API's contract.
//
// Keyed by `code`, never by element id. FPL reassigns element ids every summer, so an
// availability record keyed on one comes back next season attached to a different
// footballer — a trap this project has already paid for in the standing overrides, and
// one that is much worse here because the whole point of six seasons of this data is
// to join it across seasons.
func TestBackfilledRoundTripsKeyedByPermanentCode(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2020, 10, 2, 9, 0, 0, 0, time.UTC)

	dir, m, err := Backfilled{
		Root: root, Season: "2020-21", Event: 4,
		Body: boot(t, 4, "2020-10-03T10:00:00Z", 4), At: at,
		Source: "https://web.archive.org/web/20201002090000id_/x",
		Stamp:  "20201002090000", Digest: "ABCDEF", Version: "deadbeef",
	}.Write()
	if err != nil {
		t.Fatal(err)
	}
	if m.Backfill == nil || m.Backfill.Season != "2020-21" {
		t.Fatalf("provenance was not recorded: %+v", m.Backfill)
	}
	if m.HoursToDeadline == nil || *m.HoursToDeadline <= 0 {
		t.Fatalf("a stored capture must record a positive distance to its deadline, got %v",
			m.HoursToDeadline)
	}
	if !strings.Contains(filepath.Base(dir), "GW04") {
		t.Errorf("directory %s does not name its gameweek, so a hole is invisible in a listing",
			filepath.Base(dir))
	}

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	p, ok, err := store.Player("2020-21", 4, 154043)
	if err != nil || !ok {
		t.Fatalf("looking up a player by permanent code: ok=%v err=%v", ok, err)
	}
	if p.Status != "d" || p.ChanceNext == nil || *p.ChanceNext != 75 {
		t.Errorf("recovered %+v, want status d at a 75%% chance", p)
	}
	if p.News == "" || p.NewsAdded == "" {
		t.Error("the news text and its timestamp are what make an absence datable; both must survive")
	}

	// A fit player must come back with a nil chance, not a zero one. Collapsing
	// "FPL published no figure" into "0% chance of playing" would turn every
	// healthy player in the game into a certain absentee.
	fit, ok, err := store.Player("2020-21", 4, 118748)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if fit.ChanceNext != nil {
		t.Errorf("a fit player came back with a chance of %d; nil and zero are different facts",
			*fit.ChanceNext)
	}
}

// TestAskingForAGapIsAnErrorNotAnEmptyGameweek pins the loudness requirement.
//
// Coverage is genuinely patchy and that is fine. What is not fine is a caller reading
// a missing gameweek as one where nobody was injured, which is what an empty result
// would mean. The recorded failure mode of this package is a repair that silently
// applies nothing.
func TestAskingForAGapIsAnErrorNotAnEmptyGameweek(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2020, 10, 2, 9, 0, 0, 0, time.UTC)
	if _, _, err := (Backfilled{
		Root: root, Season: "2020-21", Event: 4,
		Body: boot(t, 4, "2020-10-03T10:00:00Z", 4), At: at,
		Source: "x", Stamp: "20201002090000",
	}).Write(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.At("2020-21", 12)
	if err == nil {
		t.Fatalf("GW12 has no capture but At() returned %d players", len(a.Players))
	}
	if !strings.Contains(err.Error(), "gap") {
		t.Errorf("the error should name the thing as a gap; got %v", err)
	}
	// `Count` is the primitive a coverage table is built from, and it must
	// distinguish stored from absent without anyone opening a payload.
	if got := store.Count("2020-21", 4); got != 1 {
		t.Errorf("Count for the stored GW4 is %d, want 1", got)
	}
	if got := store.Count("2020-21", 12); got != 0 {
		t.Errorf("Count for the absent GW12 is %d, want 0", got)
	}
	if _, ok := store.Dir("2020-21", 12); ok {
		t.Error("Dir returned a directory for a gameweek with no capture")
	}
}

// TestABackfilledCaptureIsNeverOverwritten carries the live series' rule into the
// backfill. A capture is evidence about a moment, and overwriting one destroys the
// only copy.
func TestABackfilledCaptureIsNeverOverwritten(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2020, 10, 2, 9, 0, 0, 0, time.UTC)
	b := Backfilled{
		Root: root, Season: "2020-21", Event: 4,
		Body: boot(t, 4, "2020-10-03T10:00:00Z", 4), At: at, Source: "x", Stamp: "20201002090000",
	}
	if _, _, err := b.Write(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Write(); err == nil {
		t.Fatal("a second write to the same crawl overwrote the first")
	}
}

// TestParseEventsHasOneImplementation pins that the live capture path and the backfill
// read the calendar the same way.
//
// This project's signature failure is one quantity with two implementations where the
// measured one is not the one that runs — it has shipped inside a diagnostic, inside
// the appearance model, and inside the bench weights. The deadline parse is now used
// by `annotateDeadline`, by `VerifyPreDeadline` and by the calendar discovery, and a
// private copy reappearing beside it is the thing to catch.
func TestParseEventsHasOneImplementation(t *testing.T) {
	body := boot(t, 4, "2020-10-03T10:00:00Z", 4)
	events, err := ParseEvents(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 38 {
		t.Fatalf("parsed %d events, want 38", len(events))
	}

	// The live path's annotation and the backfill's verification must agree about
	// the same body, since they are the two consumers.
	var m Manifest
	at := time.Date(2020, 10, 2, 9, 0, 0, 0, time.UTC)
	annotateDeadline(&m, body, at)
	q, err := VerifyPreDeadline(body, m.Event, at)
	if err != nil {
		t.Fatal(err)
	}
	if m.EventDeadline == nil || !m.EventDeadline.Equal(q.Deadline) {
		t.Fatalf("the live path dates this body to %v and the backfill to %v",
			m.EventDeadline, q.Deadline)
	}
	if m.HoursToDeadline == nil || fmt.Sprintf("%.6f", *m.HoursToDeadline) !=
		fmt.Sprintf("%.6f", q.HoursBefore) {
		t.Fatalf("the two paths disagree about the distance to the deadline: %v vs %v",
			m.HoursToDeadline, q.HoursBefore)
	}
}
