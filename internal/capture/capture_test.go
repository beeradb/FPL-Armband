package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fake is a Fetcher that serves canned bodies and counts calls, so the whole
// package is testable with no network. Tests here must not skip when the API is
// unreachable: this is the one command whose input cannot be re-fetched later, so
// its guarantees have to hold on every machine.
type fake struct {
	bodies map[string][]byte
	fail   map[string]error
	calls  int
}

func (f *fake) Raw(_ context.Context, path string) ([]byte, error) {
	f.calls++
	if err, ok := f.fail[path]; ok {
		return nil, err
	}
	b, ok := f.bodies[path]
	if !ok {
		return nil, fmt.Errorf("no canned body for %s", path)
	}
	return b, nil
}

// bootstrapWith builds a minimal bootstrap-static carrying one unparsed field, so a
// test can prove the capture stored the body rather than a re-serialisation.
func bootstrapWith(deadline time.Time) []byte {
	return []byte(fmt.Sprintf(`{
	  "events": [
	    {"id": 1, "deadline_time": %q, "is_current": false, "is_next": false},
	    {"id": 2, "deadline_time": %q, "is_current": false, "is_next": true}
	  ],
	  "elements": [{"id": 7, "status": "d", "chance_of_playing_next_round": 75}],
	  "a_field_this_program_does_not_parse": {"nested": [1, 2, 3]}
	}`, deadline.Add(-7*24*time.Hour).Format(time.RFC3339), deadline.Format(time.RFC3339)))
}

func newFake(deadline time.Time) *fake {
	return &fake{bodies: map[string][]byte{
		"/bootstrap-static/": bootstrapWith(deadline),
		"/fixtures/":         []byte(`[{"id": 1, "kickoff_time": "2026-08-14T19:00:00Z"}]`),
	}}
}

// TestCaptureStoresTheBodyBitForBit is the property the whole feature rests on.
//
// The alternative implementation — marshal the parsed struct — is tempting because
// the program already holds a parsed Bootstrap. It is wrong, and this project has
// FIVE recorded instances of the same mistake in reverse: concluding "the archive
// does not carry X" when X was present and merely unparsed. The availability flags,
// the per-gameweek defensive contribution, the fixture kickoff times, FPL's club
// strength, and the per-gameweek ownership were all found that way.
//
// A capture whose fidelity is limited to today's parser would bake that failure in
// permanently, and it would be undetectable — the file would look complete.
func TestCaptureStoresTheBodyBitForBit(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
	f := newFake(at.Add(50 * time.Hour))

	dir, m, err := Take(context.Background(), f, root, at, "abc123")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}

	got, err := Read(dir, "/bootstrap-static/")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := f.bodies["/bootstrap-static/"]
	if string(got) != string(want) {
		t.Errorf("stored body is not byte-identical to what FPL sent.\n got %d bytes\nwant %d bytes",
			len(got), len(want))
	}
	if !strings.Contains(string(got), "a_field_this_program_does_not_parse") {
		t.Error("a field this program does not parse did not survive the capture — which is " +
			"exactly the failure mode: five times this project has concluded the archive " +
			"lacks something it carried, because nothing was reading it")
	}
	if len(m.Files) != len(Endpoints) {
		t.Errorf("manifest lists %d files, want %d", len(m.Files), len(Endpoints))
	}
	// The checksum is of the uncompressed body, so it can be checked against a
	// re-fetch or against another party's copy without agreeing on a gzip level.
	for _, fl := range m.Files {
		if fl.Note != "" {
			t.Errorf("%s failed unexpectedly: %s", fl.Endpoint, fl.Note)
		}
		if fl.SHA256 == "" || fl.Bytes == 0 {
			t.Errorf("%s: manifest is missing a checksum or a size", fl.Endpoint)
		}
	}
}

// TestACaptureIsNeverOverwritten pins immutability.
//
// A capture is the only copy of a moment. Overwriting one is not a lost cache entry,
// it is destroyed evidence — so a collision is an error rather than a silent
// replacement, the same call the R inference script makes when asked to write over
// another sweep's output.
func TestACaptureIsNeverOverwritten(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
	f := newFake(at.Add(24 * time.Hour))

	if _, _, err := Take(context.Background(), f, root, at, ""); err != nil {
		t.Fatalf("first Take: %v", err)
	}
	before, err := Read(filepath.Join(root, DirName(at)), "/bootstrap-static/")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// Same minute, different content — the case where silently overwriting would
	// actually lose something.
	f.bodies["/bootstrap-static/"] = []byte(`{"events": [], "elements": []}`)
	if _, _, err := Take(context.Background(), f, root, at, ""); err == nil {
		t.Fatal("Take overwrote an existing capture; it must refuse")
	}

	after, err := Read(filepath.Join(root, DirName(at)), "/bootstrap-static/")
	if err != nil {
		t.Fatalf("Read after: %v", err)
	}
	if string(before) != string(after) {
		t.Error("the existing capture's contents changed, so the refusal came too late")
	}
}

// TestAFailedEndpointIsFlaggedNotDropped is the sweep harness's rule applied here.
//
// "This endpoint could not be fetched" is a fact about the capture. Dropping the
// entry reads later as an endpoint nobody tried, which is a different and weaker
// claim that looks like a clean run — and the whole point of a manifest is that a
// reader in 2028 can tell the two apart.
func TestAFailedEndpointIsFlaggedNotDropped(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
	f := newFake(at.Add(24 * time.Hour))
	f.fail = map[string]error{"/fixtures/": fmt.Errorf("status 503")}

	dir, m, err := Take(context.Background(), f, root, at, "")
	if err != nil {
		t.Fatalf("Take should survive one failing endpoint: %v", err)
	}
	if len(m.Files) != len(Endpoints) {
		t.Fatalf("manifest lists %d files, want %d — a failure must be a flagged row, "+
			"never a missing one", len(m.Files), len(Endpoints))
	}
	var noted bool
	for _, fl := range m.Files {
		if fl.Endpoint == "/fixtures/" {
			noted = fl.Note != ""
		}
	}
	if !noted {
		t.Error("the failing endpoint carries no note saying it failed")
	}
	// The endpoint that worked is still evidence and must still be there.
	if _, err := Read(dir, "/bootstrap-static/"); err != nil {
		t.Errorf("the successful endpoint was not stored: %v", err)
	}
}

// TestTheManifestDatesItselfAndTheDeadline covers two things a path cannot carry.
//
// The timestamp is inside the file because a directory can be renamed, copied or
// hand-reconstructed, and then the path is not evidence of anything. The deadline
// context is there because a capture 200 hours out records a completely different
// information state from one taken six hours out, and only the second answers the
// question the capture exists for — what was known when the decision was taken.
func TestTheManifestDatesItselfAndTheDeadline(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
	deadline := at.Add(5 * time.Hour)
	f := newFake(deadline)

	dir, _, err := Take(context.Background(), f, root, at, "")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("no manifest: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("manifest does not parse: %v", err)
	}
	if !m.CapturedAt.Equal(at) {
		t.Errorf("manifest says %v, captured at %v", m.CapturedAt, at)
	}
	if m.Event != 2 {
		t.Errorf("event = %d, want 2 — is_next is the gameweek a capture informs", m.Event)
	}
	if m.HoursToDeadline == nil {
		t.Fatal("no hours-to-deadline recorded")
	}
	if got := *m.HoursToDeadline; got < 4.9 || got > 5.1 {
		t.Errorf("hours to deadline = %.2f, want ~5", got)
	}
}

// TestDirNamesSortChronologically pins the one thing the snapshot directories get
// wrong.
//
// Snapshot directories are date-then-commit, so a lexical sort LOOKS chronological
// and is wrong the moment two share a day — the tie falls to a hex string carrying
// no time information, and that defect shipped twice there. A capture series is a
// time series and will routinely have two entries in a day, so the name is a full
// UTC timestamp and List can sort it as text.
func TestDirNamesSortChronologically(t *testing.T) {
	root := t.TempDir()
	times := []time.Time{
		time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC),
		time.Date(2026, 8, 12, 17, 5, 0, 0, time.UTC), // same day, later
		time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC),
	}
	for _, at := range times {
		if err := os.MkdirAll(filepath.Join(root, DirName(at)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List returned %d, want 3: %v", len(got), got)
	}
	for i := range got {
		if want := DirName(times[i]); got[i] != want {
			t.Errorf("List[%d] = %s, want %s (lexical order must be chronological)", i, got[i], want)
		}
	}
}

// TestUTCNotLocal — a series spanning a clock change must stay monotonic.
//
// Two captures an hour apart across the autumn transition have the same local wall
// time, so a local-time name would collide and the second would be refused as a
// duplicate. The refusal is correct behaviour for a genuine collision and completely
// wrong here: these are different moments.
func TestUTCNotLocal(t *testing.T) {
	// 2026-10-25 is the UK clock change; 01:30 BST and 01:30 GMT are an hour apart.
	uk, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	a := time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC) // 01:30 BST
	b := time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC) // 01:30 GMT
	if a.In(uk).Format("1504") != b.In(uk).Format("1504") {
		t.Skip("this year's transition does not produce the ambiguous local hour")
	}
	if DirName(a) == DirName(b) {
		t.Errorf("two moments an hour apart share a capture name (%s); the name must be UTC",
			DirName(a))
	}
	if !(DirName(a) < DirName(b)) {
		t.Errorf("%s should sort before %s", DirName(a), DirName(b))
	}
}
