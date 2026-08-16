package stats

import (
	"os/exec"
	gopath "path"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestMedianIsTheOrdinaryDefinition pins the convention itself.
//
// The even-count cases are the point: they are the only inputs on which this
// repo's two former conventions disagreed, which is why eleven expressions of one
// quantity could coexist for months without anything catching it. {2,1} -> 1.5 and
// {4,1,3,2} -> 2.5 would both have read 2 and 3 under the upper median.
func TestMedianIsTheOrdinaryDefinition(t *testing.T) {
	for _, c := range []struct {
		xs   []float64
		want float64
	}{
		{[]float64{}, 0},
		{[]float64{7}, 7},
		{[]float64{2, 1}, 1.5},
		{[]float64{3, 1, 2}, 2},
		{[]float64{4, 1, 3, 2}, 2.5},
		{[]float64{5, 4, 3, 2, 1}, 3},
		{[]float64{1.5, 1.5, 1.5, 1.5}, 1.5},
		{[]float64{-2, -1, 1, 2}, 0},
		{[]float64{0, 0, 0, 10}, 0},
	} {
		if got := Median(c.xs); got != c.want {
			t.Errorf("Median(%v) = %v, want %v", c.xs, got, c.want)
		}
	}
}

// TestMedianOfAnIntSeriesKeepsTheHalf is the case the collapse was designed
// around, and the reason [Median] returns float64 for every input type.
//
// The chip diagnostic takes the median of a whole number of points per gameweek
// and averages it across 36 cells. An `int`-in, `int`-out signature truncates
// every one of them, which biases the row the chips note quotes downward by
// up to half a point — comparable to the effect the row reports.
func TestMedianOfAnIntSeriesKeepsTheHalf(t *testing.T) {
	if got := Median([]int{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("Median([1 2 3 4]) = %v, want 2.5 — an int series still has a "+
			"half-integer median, and truncating it here is a downward bias on "+
			"every figure averaged from it", got)
	}
}

// TestMedianDoesNotSortTheCaller pins the copy.
//
// Callers pass slices whose order carries meaning — a per-gameweek series, a
// per-capture series — and a helper that sorted in place would corrupt them with
// nothing failing. That is the failure mode this project's record is largely a
// catalogue of, and it costs one allocation to make impossible.
func TestMedianDoesNotSortTheCaller(t *testing.T) {
	xs := []float64{3, 1, 2}
	Median(xs)
	if xs[0] != 3 || xs[1] != 1 || xs[2] != 2 {
		t.Errorf("Median reordered its argument to %v", xs)
	}
}

// TestTheMiddleValueHasOneImplementation.
//
// # What this guards, and why a source scan
//
// "The middle value of a slice" had **eleven expressions in nine files across four packages**, under
// two conventions and three names, and two of them had already diverged: an even
// count read one way in `internal/backfill` and another in
// `internal/backtest/teamnewsfile.go`. Nothing failed, because each was correct in
// isolation and no test compared them.
//
// The failure this stops is not a rewrite. It is somebody needing a median in a
// fifth package and writing `sort.Float64s(s); s[len(s)/2]` inline, because four
// lines felt cheaper than an import. Copies agree on the day they are written —
// which is why a runtime check cannot catch it and a source scan does. It is the
// same guard, one language over, as
// `TestTheSharedCellQuantitiesHaveOneImplementation` in `internal/snapshot` —
// except that one scans **names**, which is the difference this one exists for.
//
// ⚠️ **A new quantity to guard is a ROW in
// [TestTheCopiedExpressionsHaveOneImplementation], not a copy of this test.** This
// one keeps its own function because the argument below is long enough to bury a
// table, and because it is cited by name in two review records; that table is
// where the next quantity and every one after it goes. Both share [goSources], so
// the two scans cannot drift apart about which files they reach.
//
// **It matches the idiom, not the name**, and that is the whole reason it works.
// Nine of the eleven copies had no name at all — they were four lines inside a
// larger function — so a scan for the word "median" would have found two.
//
// ⚠️ **It is keyed on one spelling of the idiom, and that is a known limit.**
// `mid := len(s)/2` then `s[mid]`, `s[(len(s)-1)/2]`, `s[len(s)>>1]` and
// `s[len(m.vals)/2]` all escape it. None exists in the tree today — checked — so
// the guard is currently complete, but it is a tripwire rather than a proof.
//
// # The permitted wrapper
//
// `internal/backtest` keeps `median` as a one-line wrapper on [Median], so its
// diagnostics read as they always have. A wrapper is not a second implementation:
// it forwards, and adds no arithmetic.
//
// # What it found when it was written, and the one exemption
//
// Written to stop the *next* copy, it failed immediately on **seven** existing
// ones across four more packages — the same way
// `TestTheSharedCellQuantitiesHaveOneImplementation` failed on three R scripts the
// day it was added.
//
// The one exemption below is not a median of a sample. Every exemption here must
// carry that argument, and the list must **shrink**: an entry whose expression has
// gone is a failure too, so nobody can migrate one and leave the debt recorded as
// outstanding.
func TestTheMiddleValueHasOneImplementation(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skip(err)
	}

	// backfill.midpoint answers "which season is this crawl from", by picking a
	// representative kickoff. It is not summarising a spread and its result is
	// never reported as a median — the name says midpoint and the value is used as
	// a date. It also takes `[]time.Time`, which is not a Number, so routing it
	// through Median would mean a comparator callback for a single caller.
	//
	// Recorded rather than silently skipped: if it ever starts being quoted as a
	// median, it stops being exempt.
	//
	// Keyed on the expression rather than a line number, so an edit anywhere above
	// it does not silently move the exemption onto a different statement.
	exempt := map[string]string{
		"internal/backfill/run.go": "return ts[len(ts)/2]",
	}
	seenExempt := map[string]bool{}

	// The shape of every copy this audit found: sort a float slice, then index at
	// or beside len/2. Matched on the indexing rather than the sort, because
	// sorting is ordinary and indexing the midpoint is not.
	midpoint := regexp.MustCompile(`\[\s*len\(\s*\w+\s*\)\s*/\s*2\s*(-\s*1\s*)?\]`)

	srcs, err := goSources(root)
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, f := range srcs {
		// This package is where the one implementation lives.
		if gopath.Dir(f.rel) == "internal/stats" {
			continue
		}
		for i, line := range strings.Split(f.body, "\n") {
			if !midpoint.MatchString(line) {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if exempt[f.rel] == trimmed {
				seenExempt[f.rel] = true
				continue
			}
			offenders = append(offenders, f.rel+":"+strconv.Itoa(i+1)+"  "+trimmed)
		}
	}
	for path := range exempt {
		if !seenExempt[path] {
			t.Errorf("%s no longer carries the exempted middle-value expression, "+
				"but is still listed as an exemption. Delete it from `exempt` above. "+
				"A debt list that overstates the debt stops being read.", path)
		}
	}
	if len(offenders) > 0 {
		t.Errorf("a middle-value expression outside internal/stats:\n  %s\n\n"+
			"Call stats.Median. There is one median here and it is the ordinary "+
			"definition. Writing the four-line idiom inline is how this quantity "+
			"came to have eleven expressions in nine files across four packages under two "+
			"conventions, two of which had already diverged.",
			strings.Join(offenders, "\n  "))
	}
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
