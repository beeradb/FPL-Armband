package backtest

// Regression coverage for the header-claiming fix in openAppendCSV.
//
// The bug: two processes racing to open the SAME not-yet-existing CSV path
// (two sweeps sharing one FPL_CELLS target) both O_CREATE it and can both
// observe Stat().Size() == 0 before either has written a byte, so a
// size-based "am I the creator" check has BOTH of them write a header — the
// loser's lands mid-file as a bogus data row. openAppendCSV replaces the size
// check with O_EXCL, which the kernel guarantees exactly one opener wins.

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

// TestOpenAppendCSVClaimsCreationExactlyOnceSequentially is the deterministic
// half: no goroutines, no scheduler luck, just two calls in a row against the
// same fresh path. The first must claim creation; the second must not, since
// by the time it runs the file already exists — this is the part of the fix
// that needs no contention to verify.
func TestOpenAppendCSVClaimsCreationExactlyOnceSequentially(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")

	f1, created1, err := openAppendCSV(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	f1.Close()
	if !created1 {
		t.Fatal("first open of a fresh path must claim creation")
	}

	f2, created2, err := openAppendCSV(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	f2.Close()
	if created2 {
		t.Fatal("second open of an already-existing path must not claim creation again")
	}
}

// TestConcurrentCSVOpensWriteExactlyOneHeader is the racy half. N goroutines,
// released together, each race openAppendCSV on the SAME not-yet-existing
// path and each write one row — the creator writing its header first. Run
// with `-race -count=50` to lean on actually triggering the O_CREATE race
// this fixes; a build predating the fix (Stat().Size()==0 as the creation
// test) fails this reliably under load, since every goroutine observes size
// 0 before any of them has written a byte.
func TestConcurrentCSVOpensWriteExactlyOneHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	header := []string{"h1", "h2"}

	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			f, created, err := openAppendCSV(path)
			if err != nil {
				errs <- err
				return
			}
			defer f.Close()
			w := csv.NewWriter(f)
			if created {
				if err := w.Write(header); err != nil {
					errs <- err
					return
				}
			}
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
		if len(r) == len(header) && r[0] == header[0] && r[1] == header[1] {
			headerCount++
		}
	}
	if headerCount != 1 {
		t.Fatalf("header %v appears %d times in the file, want exactly 1 — "+
			"a size-based creation check lets more than one goroutine believe "+
			"it created the file", header, headerCount)
	}
}
