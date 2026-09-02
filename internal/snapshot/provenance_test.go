package snapshot

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestAKilledArmIsVisible is the reason the provenance file exists.
//
// A sweep killed under load leaves a cells file containing however many arms
// finished. Nothing in the cells file says how many were asked for, so three arms
// of six reads downstream as a complete three-arm sweep — and that has happened:
// one block was killed four times, at 1, 3, 3 and 4 arms of 6, and the gap was
// noticed only because somebody counted by hand.
//
// The declaration is written before the first cell precisely so that a kill leaves
// it behind. A test that only checked a completed sweep would pass while the
// feature was useless.
func TestAKilledArmIsVisible(t *testing.T) {
	dir := t.TempDir()
	cells := filepath.Join(dir, "cells.csv")

	if err := WriteProvenance(ProvenancePath(cells), Provenance{
		Sweep: "SWEEP#1", RunID: "r1", Commit: "abc123def456", Digest: "deadbeef1234",
		Seasons: []string{"2024-25<-2023-24"}, StartGWs: []int{1, 11},
		BankUpTo:     5,
		DeclaredArms: []string{"shipped", "alternative A", "alternative B"},
	}); err != nil {
		t.Fatal(err)
	}

	// Only two of the three arms emitted cells: arm B died.
	writeCellsFixture(t, cells, [][]string{
		{"SWEEP#1", "r1", "shipped", "0", "true", "2024-25", "2023-24", "1", "38", "5", "false", "1900", "1800", "40", "4"},
		{"SWEEP#1", "r1", "shipped", "0", "false", "2024-25", "2023-24", "11", "28", "5", "false", "1400", "1300", "30", "2"},
		{"SWEEP#1", "r1", "alternative A", "1", "false", "2024-25", "2023-24", "1", "38", "5", "false", "1910", "1800", "41", "4"},
		{"SWEEP#1", "r1", "alternative A", "1", "false", "2024-25", "2023-24", "11", "28", "5", "false", "1410", "1300", "31", "2"},
	})

	rows, err := ReadCells(cells)
	if err != nil {
		t.Fatal(err)
	}
	prov, err := ReadProvenance(ProvenancePath(cells))
	if err != nil {
		t.Fatal(err)
	}
	sweeps := GroupSweeps(rows, prov)
	if len(sweeps) != 1 {
		t.Fatalf("expected one sweep, got %d", len(sweeps))
	}
	s := sweeps[0]
	if !s.HasProv {
		t.Fatal("provenance was written and not joined; the key must be (sweep, run_id)")
	}
	if len(s.Arms) != 3 {
		t.Fatalf("expected the killed arm to still appear, got %d arms", len(s.Arms))
	}
	killed := s.Arms[2]
	if killed.Label != "alternative B" {
		t.Errorf("declaration order was lost: third arm is %q, so a killed arm in the "+
			"middle of a ladder would move rather than stay in place", killed.Label)
	}
	if killed.Ran {
		t.Error("an arm that emitted no cells is reported as having run")
	}

	// And it must reach the reader. A structural field nobody prints is not a fix.
	md, _ := Render(Inputs{Sweeps: sweeps})
	if !strings.Contains(md, "KILLED") {
		t.Error("the snapshot does not say an arm was killed; silence reading as " +
			"success is exactly the failure this is for")
	}
	if !strings.Contains(md, "incomplete") {
		t.Error("the snapshot does not warn that the grid is incomplete")
	}
}

// TestCellsWithoutProvenanceSayTheyAreUnattributable.
//
// The frozen fixture in stats/testdata is such a file, and so is every cells file
// produced before the stamping existed. Reading one is fine; reading one *without
// being told* is how an orphaned measurement becomes a citation.
func TestCellsWithoutProvenanceSayTheyAreUnattributable(t *testing.T) {
	dir := t.TempDir()
	cells := filepath.Join(dir, "cells.csv")
	writeCellsFixture(t, cells, [][]string{
		{"OLD#1", "r0", "shipped", "0", "true", "2024-25", "2023-24", "1", "38", "5", "false", "1900", "1800", "40", "4"},
		{"OLD#1", "r0", "other", "1", "false", "2024-25", "2023-24", "1", "38", "5", "false", "1910", "1800", "41", "4"},
	})
	rows, err := ReadCells(cells)
	if err != nil {
		t.Fatal(err)
	}
	sweeps := GroupSweeps(rows, map[string]Provenance{})
	if sweeps[0].HasProv {
		t.Fatal("claimed provenance where none exists")
	}
	md, _ := Render(Inputs{Sweeps: sweeps})
	for _, want := range []string{"No provenance sidecar", "unattributable",
		"cannot be checked"} {
		if !strings.Contains(md, want) {
			t.Errorf("the snapshot does not contain %q, so a reader would not know the "+
				"figures have no stamp", want)
		}
	}
}

// TestProvenanceAppendsRatherThanOverwriting.
//
// Several sweeps run in one session and losing the earlier ones is the failure
// mode — the same reason the cells file appends. Two runs of the same block must
// also stay two samples: pooling them would shrink a standard error while adding no
// information, which is manufactured confidence.
func TestProvenanceAppendsRatherThanOverwriting(t *testing.T) {
	dir := t.TempDir()
	path := ProvenancePath(filepath.Join(dir, "cells.csv"))
	for _, p := range []Provenance{
		{Sweep: "A#1", RunID: "r1", Digest: "aaa", DeclaredArms: []string{"x"}},
		{Sweep: "B#2", RunID: "r1", Digest: "bbb", DeclaredArms: []string{"y"}},
		{Sweep: "A#1", RunID: "r2", Digest: "ccc", DeclaredArms: []string{"z"}},
	} {
		if err := WriteProvenance(path, p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ReadProvenance(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 distinct (sweep, run_id) records, got %d: two runs of one "+
			"block must not merge", len(got))
	}
	if got["A#1\x00r1"].Digest != "aaa" || got["A#1\x00r2"].Digest != "ccc" {
		t.Error("two runs of the same sweep label were confused; run_id must be part " +
			"of the key")
	}
}

// TestProvenancePathIsDerivedOnce.
//
// The .means.csv path was once derived by two different rules — Go trimmed a ".csv"
// suffix and R substituted an anchored regex — which agree only when the path ends
// in ".csv". Given a path that did not, R resolved the cells file itself and the run
// died *after* the replay had been paid for. Nothing outside Go reads provenance, so
// there is one rule; this pins it against the same input that broke the other one.
func TestProvenancePathIsDerivedOnce(t *testing.T) {
	for in, want := range map[string]string{
		"/tmp/cells.csv":   "/tmp/cells" + provenanceSuffix,
		"/tmp/cells":       "/tmp/cells" + provenanceSuffix,
		"/tmp/a.csv/b.csv": "/tmp/a.csv/b" + provenanceSuffix,
	} {
		if got := ProvenancePath(in); got != want {
			t.Errorf("ProvenancePath(%q) = %q, want %q", in, got, want)
		}
		if got := ProvenancePath(in); got == in {
			t.Errorf("ProvenancePath(%q) returned the cells path itself, which is how "+
				"the .means.csv mismatch destroyed a finished run", in)
		}
	}
}

// TestProvenanceCarriesTheWatchedDigest pins the mechanism
// stats/mde_aggregate.py's staleness check depends on: WriteProvenance records
// the same digest WatchedDigest computes for the commit it stamps, the value
// round-trips exactly (a real mismatch must read as a mismatch, not get
// normalised away), and a sidecar written before this field existed reads back
// as absent rather than as an accidental match.
//
// Uses the gitRepo fixture from watched_test.go rather than this checkout, so
// the digest that "should" round-trip is fixed and known rather than whatever
// origin/main happens to be today.
func TestProvenanceCarriesTheWatchedDigest(t *testing.T) {
	repo := newGitRepo(t)
	repo.seedWatched()
	repo.commitAll("base")

	digest, perPath, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	var watched []Constant
	for _, p := range SnapshotWatchedPaths {
		watched = append(watched, Constant{Path: p, Value: perPath[p]})
	}

	dir := t.TempDir()
	cells := filepath.Join(dir, "cells.csv")
	if err := WriteProvenance(ProvenancePath(cells), Provenance{
		Sweep: "SWEEP#1", RunID: "r1", Commit: "deadbeef",
		WatchedDigest: digest, WatchedPaths: watched,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := ReadProvenance(ProvenancePath(cells))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := got["SWEEP#1\x00r1"]
	if !ok {
		t.Fatal("no record for the sweep just written")
	}
	if p.WatchedDigest != digest {
		t.Errorf("watched digest round trip: got %s want %s", p.WatchedDigest, digest)
	}
	for _, c := range watched {
		var found bool
		for _, g := range p.WatchedPaths {
			if g.Path != c.Path {
				continue
			}
			found = true
			if g.Value != c.Value {
				t.Errorf("per-path digest for %s: got %s want %s", c.Path, g.Value, c.Value)
			}
		}
		if !found {
			t.Errorf("per-path digest for %s did not round trip", c.Path)
		}
	}

	// A banked digest that no longer matches HEAD is exactly what a code change
	// produces. It must stay distinguishable from a match rather than get
	// coerced into one.
	repo.write("internal/analysis/score.go", "package analysis\n\nconst K = 24\n")
	repo.commitAll("move a constant")
	nowDigest, _, err := WatchedDigest(repo.dir, "HEAD", SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if nowDigest == p.WatchedDigest {
		t.Fatal("the fixture did not move the digest, so the mismatch case below proves nothing")
	}
	if p.WatchedDigest == nowDigest {
		t.Error("a stale banked digest read back as matching the current one")
	}

	// A sidecar written before this column existed — every provenance file
	// banked in this repository today — must read back as absent, not as a
	// zero-value that a careless comparison could mistake for a match.
	dir2 := t.TempDir()
	cells2 := filepath.Join(dir2, "cells.csv")
	if err := WriteProvenance(ProvenancePath(cells2), Provenance{
		Sweep: "OLD#1", RunID: "r1", Commit: "deadbeef", Digest: "cccccccccccc",
	}); err != nil {
		t.Fatal(err)
	}
	old, err := ReadProvenance(ProvenancePath(cells2))
	if err != nil {
		t.Fatal(err)
	}
	op, ok := old["OLD#1\x00r1"]
	if !ok {
		t.Fatal("no record for the legacy-shaped sweep just written")
	}
	if op.WatchedDigest != "" {
		t.Errorf("a sidecar with no watched_digest column read back as %q, want absent", op.WatchedDigest)
	}
	if len(op.WatchedPaths) != 0 {
		t.Errorf("a sidecar with no watched_path rows produced %d, want 0", len(op.WatchedPaths))
	}
}

// fullSizeProvenance builds a Provenance whose rendered block is comfortably
// bigger than bufio's 4096-byte default buffer — real banked blocks measure
// 16,265-24,789 bytes across every stats/cells/**/*.provenance.csv, so 400
// constants (roughly matching that population) is the realistic case, not a
// stress fixture.
func fullSizeProvenance(sweep, runID string) Provenance {
	p := Provenance{
		Sweep: sweep, RunID: runID, Commit: "abc123def456789", Digest: "deadbeef1234",
		Seasons: []string{"2024-25<-2023-24"}, StartGWs: []int{1, 11},
		BankUpTo: 5,
	}
	for i := 0; i < 20; i++ {
		p.DeclaredArms = append(p.DeclaredArms, fmt.Sprintf("arm-%d", i))
	}
	for i := 0; i < 400; i++ {
		p.Constants = append(p.Constants, Constant{
			Path:  fmt.Sprintf("internal.analysis.SomeLongConstantPathName%d", i),
			Value: fmt.Sprintf("value-%d", i),
		})
	}
	return p
}

// countingWriter counts how many times Write is called on it, so a test can
// assert a caller issued exactly one syscall-shaped write rather than several.
type countingWriter struct {
	io.Writer
	calls int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.calls++
	return c.Writer.Write(p)
}

// TestAProvenanceBlockExceedingTheBufferStaysOneWrite.
//
// A csv.Writer built directly over an *os.File goes through bufio's default
// 4096-byte buffer and flushes across several independent write(2) calls once a
// block exceeds it — which every real provenance block does (16-25 KB measured).
// Under the replay wrapper's parallel-sweep queueing (see AGENTS.md) that is a
// torn row waiting to happen. The fix builds
// the whole block in memory first and hands it to the underlying writer once;
// this pins that "once" against a block realistically sized, not against a
// fixture too small to have caught the original bug.
func TestAProvenanceBlockExceedingTheBufferStaysOneWrite(t *testing.T) {
	p := fullSizeProvenance("SWEEP#1", "r1")
	blk := provenanceBlock(p, true)
	if len(blk) <= 4096 {
		t.Fatalf("fixture block is %d bytes, want > 4096 to actually exercise the "+
			"bufio default this test is about", len(blk))
	}

	var buf strings.Builder
	cw := &countingWriter{Writer: &buf}
	if err := writeProvenanceTo(cw, p, true); err != nil {
		t.Fatal(err)
	}
	if cw.calls != 1 {
		t.Errorf("writeProvenanceTo issued %d Write calls for a %d-byte block, want "+
			"exactly 1 — a concurrent writer sharing the same file can interleave "+
			"between multiple calls but not within one", cw.calls, len(blk))
	}
}

// TestConcurrentSweepsNeverTearARow.
//
// On Linux, write(2) to a regular file holds the inode's rwsem for its duration
// and O_APPEND makes the offset-bump-and-write atomic against other O_APPEND
// writers on the same file, so one write(2) per sweep cannot interleave with
// another. Before the fix, WriteProvenance issued several independent writes per
// call (bufio flushing a >4KB block), so two sweeps racing on one sidecar could
// tear a row in half. This fails before the fix and passes after; run with
// `-race -count=10` to lean on it.
func TestConcurrentSweepsNeverTearARow(t *testing.T) {
	dir := t.TempDir()
	path := ProvenancePath(filepath.Join(dir, "cells.csv"))

	const goroutines = 8
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			p := fullSizeProvenance("SWEEP#1", fmt.Sprintf("r%d", g))
			if err := WriteProvenance(path, p); err != nil {
				t.Errorf("goroutine %d: WriteProvenance: %v", g, err)
			}
		}(g)
	}
	wg.Wait()

	got, err := ReadProvenance(path)
	if err != nil {
		t.Fatalf("ReadProvenance failed on a concurrently-written sidecar: %v", err)
	}
	if len(got) != goroutines {
		t.Fatalf("expected %d distinct run records, got %d — a torn row merged or "+
			"dropped one", goroutines, len(got))
	}
	want := fullSizeProvenance("", "")
	for g := 0; g < goroutines; g++ {
		key := fmt.Sprintf("SWEEP#1\x00r%d", g)
		p, ok := got[key]
		if !ok {
			t.Errorf("run r%d is missing entirely", g)
			continue
		}
		if len(p.DeclaredArms) != len(want.DeclaredArms) {
			t.Errorf("run r%d: got %d declared arms, want %d — a torn row", g,
				len(p.DeclaredArms), len(want.DeclaredArms))
		}
		if len(p.Constants) != len(want.Constants) {
			t.Errorf("run r%d: got %d constants, want %d — a torn row", g,
				len(p.Constants), len(want.Constants))
		}
		if p.Commit != want.Commit {
			t.Errorf("run r%d: commit %q, want %q — a torn row", g, p.Commit, want.Commit)
		}
	}
}

// TestProvenanceIsNotWrittenThroughABufferedFileWriter is a tripwire in this
// repo's own idiom (source scan, e.g. chipGameweekLiteral in
// internal/analysis): the next refactor of this file must not reintroduce a
// csv.Writer built directly over the *os.File, which is what made torn rows
// possible in the first place.
func TestProvenanceIsNotWrittenThroughABufferedFileWriter(t *testing.T) {
	b, err := os.ReadFile("provenance.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "csv.NewWriter(f)") {
		t.Fatal("provenance.go builds a csv.Writer directly over the *os.File again. " +
			"A real provenance block is 16-25 KB, 4-7x bufio's 4096-byte default " +
			"buffer, so this flushes across several independent write(2) calls and " +
			"a concurrent sweep can tear a row. Build the block with provenanceBlock " +
			"(a bytes.Buffer csv.Writer) and hand the whole thing to the file in one " +
			"Write call instead — see writeProvenanceTo.")
	}
}

// TestADuplicateHeaderRowDoesNotCreateAPhantomEntry.
//
// Two sweeps can both open a fresh sidecar, both see Size()==0, and both emit the
// header row. The duplicate parses as sweep="sweep", run_id="run_id", key="key",
// value="value" — matching no case in the switch below — but without an explicit
// skip it still creates a phantom out["sweep\x00run_id"] entry with every field
// zero, which is exactly the shape ReadProvenance's other callers use for "this
// file has no provenance".
func TestADuplicateHeaderRowDoesNotCreateAPhantomEntry(t *testing.T) {
	dir := t.TempDir()
	path := ProvenancePath(filepath.Join(dir, "cells.csv"))

	// The real header, plus a second one appended by hand — reproducing what two
	// racing writers each seeing Size()==0 would produce, without depending on
	// goroutine scheduling to land the race.
	raw := "sweep,run_id,key,value\n" +
		"SWEEP#1,r1,commit,abc123\n" +
		"sweep,run_id,key,value\n" +
		"SWEEP#1,r1,constants_digest,deadbeef\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadProvenance(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, phantom := got["sweep\x00run_id"]; phantom {
		t.Error("a duplicate header row created a phantom sweep=\"sweep\" entry")
	}
	p, ok := got["SWEEP#1\x00r1"]
	if !ok {
		t.Fatal("the real record went missing alongside the duplicate header")
	}
	if p.Commit != "abc123" || p.Digest != "deadbeef" {
		t.Errorf("real record corrupted: got Commit=%q Digest=%q", p.Commit, p.Digest)
	}
	if len(got) != 1 {
		t.Errorf("expected exactly 1 record, got %d", len(got))
	}
}

// writeCellsFixture writes a cells CSV in the *current* schema, with the layer and
// captaincy-rung columns blank.
//
// Blank rather than zero, deliberately: an unmeasured layer and a layer measured at
// zero are different facts, and only one of them is a number anything should
// average.
func writeCellsFixture(t *testing.T, path string, rows [][]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("sweep,run_id,variant,variant_index,is_baseline,season,prior_season," +
		"start_gw,weeks,bank_up_to,infeasible,policy_points,hold_points,moves,hits," +
		"policy_per_gw,hold_per_gw,frozen_points,frozen_per_gw,frozen_captain_points," +
		"frozen_captain_per_gw,weekly_points,weekly_per_gw,hold_fixedcap_points," +
		"hold_fixedcap_per_gw,hold_nocap_points,hold_nocap_per_gw\n")
	for _, r := range rows {
		b.WriteString(strings.Join(r, ","))
		b.WriteString(",,,,,,,,,,,,\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
