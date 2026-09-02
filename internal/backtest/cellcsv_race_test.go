package backtest

// Regression coverage for the header-claiming fix in openAppendCSV.
//
// The bug: two processes racing to open the SAME not-yet-existing CSV path
// (two sweeps sharing one FPL_CELLS target) both O_CREATE it and can both
// observe Stat().Size() == 0 before either has written a byte, so a
// size-based "am I the creator" check has BOTH of them write a header — the
// loser's lands mid-file as a bogus data row.
//
// openAppendCSV closes that race with an exclusive flock held across the
// check-and-decide step, not with O_EXCL: an O_EXCL-only fix (tried first,
// caught in review) swaps the duplicate-header bug for a worse one — a
// creator killed between winning O_EXCL and flushing its header (this
// project's own sweeps get OOM-killed and run out of disk under load, see
// AGENTS.md) leaves a file every later opener sees as "already created", so
// the header is never written at all. The flock makes "is this file still
// empty" a question every opener answers fresh and one at a time, so an
// abandoned empty file gets healed by the next opener instead of staying
// headerless forever.
import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

var raceTestHeader = []string{"h1", "h2"}

// TestOpenAppendCSVClaimsCreationExactlyOnceSequentially is the deterministic
// half: no goroutines, no scheduler luck, just two calls in a row against the
// same fresh path. The first must claim creation and write the header; the
// second must not, since by the time it runs the file already holds one —
// this is the part of the fix that needs no contention to verify.
func TestOpenAppendCSVClaimsCreationExactlyOnceSequentially(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")

	f1, _, created1, err := openAppendCSV(path, raceTestHeader)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	f1.Close()
	if !created1 {
		t.Fatal("first open of a fresh path must claim creation")
	}

	f2, _, created2, err := openAppendCSV(path, raceTestHeader)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	f2.Close()
	if created2 {
		t.Fatal("second open of an already-headered path must not claim creation again")
	}
}

// TestAnAbandonedEmptyFileIsHealed is the scenario the O_EXCL-only design
// missed: a "creator" that opens the file and is killed (or simply never
// gets around to writing) before its header lands. A later opener must not
// read "the file already exists" as "the file already has its header" —
// that would leave the file headerless forever, silently, which is worse
// than the duplicate-header bug this whole fix exists to prevent, because a
// duplicate is at least visible as an extra row.
func TestAnAbandonedEmptyFileIsHealed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")

	// Simulate the abandoned creator directly: create the file exactly as
	// openAppendCSV's first opener would, then vanish without writing or
	// even taking the flock — the worst case, not just an early return.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f2, w2, created2, err := openAppendCSV(path, raceTestHeader)
	if err != nil {
		t.Fatalf("open after an abandoned creator: %v", err)
	}
	defer f2.Close()
	if !created2 {
		t.Fatal("an opener finding the file still empty must heal it by " +
			"claiming the header, not defer to a creator that never wrote one")
	}
	if err := w2.Write([]string{"row", "0"}); err != nil {
		t.Fatal(err)
	}
	w2.Flush()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0][0] != raceTestHeader[0] || recs[0][1] != raceTestHeader[1] {
		t.Fatalf("got records %v, want a header row followed by one data row", recs)
	}
}

// TestConcurrentCSVOpensWriteExactlyOneHeader is the racy half. N goroutines,
// released together, each race openAppendCSV on the SAME not-yet-existing
// path and each write one row. Run with `-race -count=50` to lean on
// actually triggering the O_CREATE race this fixes; a build predating the
// fix (Stat().Size()==0 as the creation test, checked outside any lock)
// fails this reliably under load, since every goroutine observes size 0
// before any of them has written a byte.
func TestConcurrentCSVOpensWriteExactlyOneHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			f, w, created, err := openAppendCSV(path, raceTestHeader)
			if err != nil {
				errs <- err
				return
			}
			defer f.Close()
			_ = created // the header, if any, is already written by openAppendCSV
			if err := w.Write([]string{"row", strconv.Itoa(i)}); err != nil {
				errs <- err
				return
			}
			w.Flush()
			if err := w.Error(); err != nil {
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

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != n+1 {
		t.Fatalf("got %d records, want %d (one header plus %d rows) — a "+
			"duplicated header would show up as an extra record here",
			len(recs), n+1, n)
	}
	headerCount := 0
	for _, r := range recs {
		if len(r) == len(raceTestHeader) && r[0] == raceTestHeader[0] && r[1] == raceTestHeader[1] {
			headerCount++
		}
	}
	if headerCount != 1 {
		t.Fatalf("header %v appears %d times in the file, want exactly 1 — "+
			"a size-based creation check lets more than one goroutine believe "+
			"it created the file", raceTestHeader, headerCount)
	}
}
