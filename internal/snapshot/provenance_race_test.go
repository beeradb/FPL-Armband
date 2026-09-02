package snapshot

// Regression coverage for WriteProvenance's flock-guarded header claim — the
// snapshot package's twin of the identical fix in internal/backtest's
// openAppendCSV. See that package's cellcsv_race_test.go for the shared
// rationale: an unguarded size check lets two processes racing to open the
// SAME not-yet-existing sidecar both believe they created it and both write
// a header row, and the loser's lands mid-file as a bogus data row that
// TestADuplicateHeaderRowDoesNotCreateAPhantomEntry above shows
// ReadProvenance had to defend against on the read side. It also covers the
// sharper failure an O_EXCL-only fix would have introduced: a sweep killed
// after creating the sidecar but before writing its header would leave every
// later writer believing the header was already spoken for, so it would
// never get written at all. The flock closes both: every writer decides
// fresh, under the same lock, whether the file is still empty.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const provenanceHeaderLine = "sweep,run_id,key,value\n"

// TestWriteProvenanceClaimsTheHeaderExactlyOnceSequentially is the
// deterministic half: two calls in a row against a fresh path, no
// contention involved. The first call must write the header; the second
// must not, since by the time it runs the file already exists — this is the
// part of the fix that needs no goroutine scheduling luck to verify.
func TestWriteProvenanceClaimsTheHeaderExactlyOnceSequentially(t *testing.T) {
	path := ProvenancePath(filepath.Join(t.TempDir(), "cells.csv"))

	if err := WriteProvenance(path, fullSizeProvenance("SWEEP#1", "seq1")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	afterFirst, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(afterFirst), provenanceHeaderLine); got != 1 {
		t.Fatalf("after the first write the header appears %d times, want 1", got)
	}

	if err := WriteProvenance(path, fullSizeProvenance("SWEEP#1", "seq2")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	afterSecond, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(afterSecond), provenanceHeaderLine); got != 1 {
		t.Fatalf("after the second write the header appears %d times, want 1", got)
	}
}

// TestAnAbandonedProvenanceFileIsHealed is the scenario an O_EXCL-only fix
// would have missed: a sweep that creates the sidecar and vanishes (killed,
// or simply never gets around to writing) before its header lands. A later
// writer must not read "the file already exists" as "the file already has
// its header" — that would leave the sidecar headerless forever, silently,
// which ReadProvenance would then read by treating the first real data row
// as the header.
func TestAnAbandonedProvenanceFileIsHealed(t *testing.T) {
	path := ProvenancePath(filepath.Join(t.TempDir(), "cells.csv"))

	// Simulate the abandoned creator directly: create the sidecar exactly as
	// WriteProvenance's OpenFile would, then vanish without writing or even
	// taking the flock — the worst case, not just an early return.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := WriteProvenance(path, fullSizeProvenance("SWEEP#1", "healer")); err != nil {
		t.Fatalf("write after an abandoned creator: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), provenanceHeaderLine); got != 1 {
		t.Fatalf("after healing an abandoned sidecar the header appears %d "+
			"times, want exactly 1 — a writer that defers to a creator which "+
			"never wrote anything leaves the file headerless forever", got)
	}
	got, err := ReadProvenance(path)
	if err != nil {
		t.Fatalf("ReadProvenance failed on the healed sidecar: %v", err)
	}
	if _, ok := got["SWEEP#1\x00healer"]; !ok {
		t.Fatalf("the healing write's own record is missing: %v", got)
	}
}

// TestConcurrentWriteProvenanceWritesExactlyOneHeader is the racy half. N
// goroutines, released together via a start channel, each call
// WriteProvenance with a distinct RunID against the SAME not-yet-existing
// sidecar. Before this fix, every goroutine could observe an unguarded
// Stat().Size()==0 before any of them had written a byte, so every one of
// them would write its own header row. Run with `-race -count=50` to lean
// on actually triggering that contention rather than trusting a single pass.
//
// It also checks that claiming the header cost no sweep its own row: all N
// records must still be present and distinguishable by run_id.
func TestConcurrentWriteProvenanceWritesExactlyOneHeader(t *testing.T) {
	path := ProvenancePath(filepath.Join(t.TempDir(), "cells.csv"))

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			runID := fmt.Sprintf("r%d", i)
			if err := WriteProvenance(path, fullSizeProvenance("SWEEP#1", runID)); err != nil {
				errs <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), provenanceHeaderLine); got != 1 {
		t.Fatalf("header appears %d times in the concurrently-written sidecar, "+
			"want exactly 1 — a size-based creation check lets more than one "+
			"goroutine believe it created the file", got)
	}

	got, err := ReadProvenance(path)
	if err != nil {
		t.Fatalf("ReadProvenance failed on a concurrently-written sidecar: %v", err)
	}
	if len(got) != n {
		t.Fatalf("got %d distinct run records, want %d — claiming the header "+
			"must not have cost a sweep its own row", len(got), n)
	}
}
