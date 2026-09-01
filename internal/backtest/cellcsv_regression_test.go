package backtest

// Regression tests for the cell CSV contract.
//
// Everything here is a failure that would be *silent*: the sweep still runs, the
// file still parses, and R still reports a mean — just of the wrong cells. These
// need no FPL API and no archive, because the contract is about the shape of the
// file rather than about football.

import (
	"encoding/csv"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"armband/internal/config"
)

// openCellSink is called with a path here rather than through FPL_CELLS, so these
// tests never depend on process environment and can run in parallel with
// anything else.

func readCells(t *testing.T, path string) (header []string, rows []map[string]string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) == 0 {
		t.Fatal("no records at all, not even a header")
	}
	header = recs[0]
	for _, r := range recs[1:] {
		m := map[string]string{}
		for i, h := range header {
			if i < len(r) {
				m[h] = r[i]
			}
		}
		rows = append(rows, m)
	}
	return header, rows
}

// sampleRow gives every same-typed column a *distinguishable* value.
//
// That is deliberate rather than tidy. csv.Reader catches a change in the number
// of columns, but a permutation of two int columns — season with prior_season,
// moves with hits — is only catchable if their values differ, and this is the
// only place a mirror between cellHeader and cellSink.cell can be pinned.
func atoiOrFail(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("not an integer: %q", s)
	}
	return n
}

func sampleRow(sweep, runID, variant string, vi int, season string, start int) cellRow {
	return cellRow{
		Sweep: sweep, RunID: runID, Variant: variant,
		VariantIndex: vi, IsBaseline: vi == 0,
		Season: season, PriorSeason: "2019-20", StartGW: start,
		Weeks: 38 - start + 1, BankUpTo: sweepBankLimit,
		PolicyPoints: 2000 + start, HoldPoints: 1900 + start,
		Moves: 41, Hits: 7,
	}
}

// TestCellCSVIsReproducibleFromWeeks pins the thing the whole contract exists
// for: that the per-gameweek figure can be re-derived downstream.
//
// The harness divides by len(res.Weeks) before anything else sees the number, and
// a GW1 entry banks 38 gameweeks against a GW26 entry's 13. If only the
// pre-divided figure were emitted, R could neither check the denominator nor
// re-weight the cells, and this project has already shipped one bug from exactly
// that — pooling start points weighted the earliest regime twice as heavily.
func TestCellCSVIsReproducibleFromWeeks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")
	for _, start := range []int{1, 26} {
		sink.cell(sampleRow(sweep, sink.run(), "shipped", 0, "2025-26", start))
	}
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		// Every column lands under its own name. cellHeader and cellSink.cell
		// are positional mirrors of each other, so a permutation of two
		// same-typed columns is silent unless something asserts the pairing.
		for name, want := range map[string]string{
			"season": "2025-26", "prior_season": "2019-20",
			"moves": "41", "hits": "7", "bank_up_to": "5",
		} {
			if r[name] != want {
				t.Fatalf("column %q carries %q, want %q — cellHeader and "+
					"cellSink.cell have desynchronised", name, r[name], want)
			}
		}
		weeks, err := strconv.Atoi(r["weeks"])
		if err != nil || weeks <= 0 {
			t.Fatalf("weeks not usable as a denominator: %q", r["weeks"])
		}
		if weeks != 38-atoiOrFail(t, r["start_gw"])+1 {
			t.Fatalf("weeks %d does not match start_gw %q", weeks, r["start_gw"])
		}
		for _, m := range []string{"policy", "hold"} {
			pts, err := strconv.Atoi(r[m+"_points"])
			if err != nil {
				t.Fatalf("%s_points unparseable: %q", m, r[m+"_points"])
			}
			got, err := strconv.ParseFloat(r[m+"_per_gw"], 64)
			if err != nil {
				t.Fatalf("%s_per_gw unparseable: %q", m, r[m+"_per_gw"])
			}
			// Exact, not approximate: the column is written at full float64
			// precision precisely so a downstream check compares arithmetic
			// rather than formatting.
			if want := float64(pts) / float64(weeks); got != want {
				t.Fatalf("%s_per_gw %v is not %d/%d = %v", m, got, pts, weeks, want)
			}
		}
	}
}

// TestInfeasibleCellStillEmitsARow guards the hole in the grid.
//
// A variant that cannot field a legal fifteen is a result about the variant. If
// the cell is dropped instead of flagged, the comparison downstream reads as one
// made on fewer cells, which is a different and much weaker claim — and it looks
// exactly like a clean run.
func TestInfeasibleCellStillEmitsARow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")
	sink.cell(sampleRow(sweep, sink.run(), "shipped", 0, "2025-26", 1))
	sink.cell(sampleRow(sweep, sink.run(), "floor=75", 1, "2025-26", 1).asInfeasible())
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 2 {
		t.Fatalf("an infeasible cell must still be a row: want 2, got %d", len(rows))
	}
	var found bool
	for _, r := range rows {
		if r["infeasible"] != "true" {
			continue
		}
		found = true
		if r["variant"] != "floor=75" {
			t.Fatalf("wrong row flagged: %q", r["variant"])
		}
		// Empty rather than zero. A zero in a column R averages is a plausible
		// number; an empty cell is a missing one, and only one of those can be
		// mistaken for a score.
		if r["policy_per_gw"] != "" || r["hold_per_gw"] != "" {
			t.Fatalf("infeasible cell must have no per-gameweek figure, got %q/%q",
				r["policy_per_gw"], r["hold_per_gw"])
		}
		if r["weeks"] != "0" {
			t.Fatalf("infeasible cell played no gameweeks, got weeks=%q", r["weeks"])
		}
	}
	if !found {
		t.Fatal("no row carries infeasible=true")
	}
}

// TestAppendingTwoSweepsKeepsBoth pins the failure mode that loses work.
//
// Several sweep blocks run in one session, and a writer that truncated would
// leave only the last — after hours of replay. The header must also appear once,
// or the second sweep's header parses as a data row whose season is the string
// "season".
func TestAppendingTwoSweepsKeepsBoth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")

	// **The same base label on both**, which is the case that actually bites:
	// runPolicySweep opens a fresh sink per block, and with EXP unset every block
	// in a test derives its label from the same t.Name(). If the ordinal lived on
	// the sink it would restart at 1 each time and both blocks would be written
	// as "SAME#1" — and since R keys a comparison on (run_id, sweep), that
	// silently pools two unrelated experiments. Passing distinct labels here
	// would make this test pass for the wrong reason.
	const sameLabel = "SAME"

	first, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	swA := first.sweepLabel(sameLabel)
	first.cell(sampleRow(swA, first.run(), "shipped", 0, "2025-26", 1))
	first.close()

	// A second block in the same process, on the same path.
	second, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	swB := second.sweepLabel(sameLabel)
	second.cell(sampleRow(swB, second.run(), "shipped", 0, "2025-26", 1))
	second.close()

	// And the run id must be stable across sinks in one process, or the two
	// blocks look like two separate runs and the sweep ordinal stops being what
	// distinguishes them.
	if first.run() != second.run() {
		t.Fatalf("run_id must be process-global, got %q and %q",
			first.run(), second.run())
	}

	header, rows := readCells(t, path)
	if len(rows) != 2 {
		t.Fatalf("appending must preserve both sweeps: want 2 rows, got %d", len(rows))
	}
	if header[0] != "sweep" {
		t.Fatalf("header lost: %v", header)
	}
	for _, r := range rows {
		if r["season"] == "season" {
			t.Fatal("a second header was written as a data row")
		}
	}
	if rows[0]["sweep"] == rows[1]["sweep"] {
		t.Fatalf("two sweeps share a label (%q); R would pair variants across "+
			"unrelated experiments", rows[0]["sweep"])
	}
	if swA == swB {
		t.Fatalf("sweepLabel is not distinguishing sweeps: %q and %q", swA, swB)
	}
}

// TestExactlyOneBaselinePerSweep pins the reference arm.
//
// reportPairedDifferences pairs everything against variants[0]. If the flag is
// absent, R has to guess the reference; if two arms carry it, R silently pairs
// against whichever it sorts first, and the sign of every reported difference is
// then unexplained rather than wrong-looking.
func TestExactlyOneBaselinePerSweep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	// Two sweeps, three arms each, two start points — the smallest grid with
	// every way of getting this wrong available.
	for _, block := range []string{"A", "B"} {
		sweep := sink.sweepLabel(block)
		for vi, name := range []string{"shipped", "alt1", "alt2"} {
			for _, start := range []int{1, 21} {
				sink.cell(sampleRow(sweep, sink.run(), name, vi, "2025-26", start))
			}
		}
	}
	sink.close()

	_, rows := readCells(t, path)
	baselines := map[string]map[string]bool{} // sweep -> variant set
	for _, r := range rows {
		if (r["variant_index"] == "0") != (r["is_baseline"] == "true") {
			t.Fatalf("is_baseline disagrees with variant_index on %v", r)
		}
		if r["is_baseline"] != "true" {
			continue
		}
		if baselines[r["sweep"]] == nil {
			baselines[r["sweep"]] = map[string]bool{}
		}
		baselines[r["sweep"]][r["variant"]] = true
	}
	if len(baselines) != 2 {
		t.Fatalf("want a baseline in each of 2 sweeps, got %d", len(baselines))
	}
	for sweep, vs := range baselines {
		if len(vs) != 1 {
			t.Fatalf("sweep %s has %d baseline variants, want exactly 1: %v", sweep, len(vs), vs)
		}
	}
}

// TestMeansFileAccompaniesTheCells pins the pipeline's own checksum.
//
// The mean is the one quantity computed in both Go and R on purpose. That is only
// defensible while R can actually check it, which requires Go to write it down —
// so the means file existing, and carrying the baseline it was paired against, is
// part of the contract rather than a convenience.
func TestMeansFileAccompaniesTheCells(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")
	sink.mean(sweep, "hold", "alt1", "shipped (ships)", 1, -0.7171234, 24, "")
	sink.close()

	_, rows := readCells(t, filepath.Join(dir, "cells.means.csv"))
	if len(rows) != 1 {
		t.Fatalf("want 1 mean row, got %d", len(rows))
	}
	r := rows[0]
	if r["metric"] != "hold" || r["variant"] != "alt1" {
		t.Fatalf("mean row mislabelled: %v", r)
	}
	if r["baseline_variant"] != "shipped (ships)" {
		t.Fatalf("mean row does not say what it was paired against: %v", r)
	}
	got, err := strconv.ParseFloat(r["mean_per_gw"], 64)
	if err != nil {
		t.Fatal(err)
	}
	// Full precision, so R's comparison can be tight enough to catch a real
	// disagreement rather than loose enough to hide one.
	if got != -0.7171234 {
		t.Fatalf("mean did not round-trip: %v", got)
	}
	if r["n_cells"] != "24" {
		t.Fatalf("n_cells lost: %q", r["n_cells"])
	}
	// An un-oracled mean says so rather than leaving the column blank. Blank
	// means "not measured" everywhere else in this schema, and a means row is the
	// one a reader copies a number out of.
	if r["oracle"] != "-" {
		t.Fatalf("mean row carries oracle %q, want %q for an ordinary sweep",
			r["oracle"], "-")
	}
}

// TestAppendingUnderAWrongSchemaIsRefused pins the loud failure.
//
// Appending this build's rows under an older build's header produces a ragged
// file rather than a broken one, and the likely reader is a half-remembered
// Rscript on last week's path. This project has already been burned by treating a
// version bump as a schema check.
func TestAppendingUnderAWrongSchemaIsRefused(t *testing.T) {
	// The chain of predecessors, each one block older than the last.
	//
	// ⚠️ **Truncation is only a valid synthesis for a block that was appended
	// after the current LAST block, and nothing has been since the oracle pair
	// landed.** Every build after it carries `oracle, oracle_kind` at the end, so
	// truncating past the arm block strips them too and produces a header no
	// build has ever written — a 31-column one ending at `triple_captain_pts`.
	// That is what the two entries below the first two used to be, and the trap
	// is the one this test's own comments record springing twice already: every
	// synthesised header differs from the current one and is refused whatever it
	// contains, so a mislabelled entry passes and still lies about which build
	// wrote it. Checked against `git show <commit>:internal/backtest/cellcsv_test.go`
	// rather than reasoned about — the real headers are 50, 40, 36, 33, 29, 27 and
	// 23 columns, and only the last two end anywhere but `oracle_kind`.
	//
	// So every block that is not the trailing pair is removed from the MIDDLE.
	// The final two entries truncate, and legitimately: the oracle pair was last
	// when it was added, and the captaincy rungs were last before that.
	//
	// ⚠️ The banking block is the newest and sits EARLIER in the schema than any
	// of them — before the chip block — so it is removed first and every later
	// step is relative to the shorter header it leaves. Removing it is what makes
	// the first entry below the 50-column header `origin/main` really wrote,
	// checked with `git show`.
	// ⚠️ The fixture-run block is newer than the banking block and sits
	// immediately AFTER it, so it comes off first and the whole chain shifts one
	// name down again. Because it sits after the banking block, removing it does
	// not move where the banking block starts — which is why the next line can
	// keep using `bankingBlockAt()`, an offset into the full header, on the
	// already-shortened one.
	// ⚠️ The floor block is the newest of all and sits between the dose block and
	// the chip block, so it comes off first and the whole chain shifts one name
	// down again. Because it sits after the dose block, removing it does not move
	// where the dose block starts — which is why the next line can keep using an
	// offset into the full header on an already-shortened one.
	// ⚠️ The second-play chip columns are newer still — bench_boost_gw2/pts2 and
	// triple_captain_gw2/pts2, recording a two-set season's second play of each
	// chip rather than silently overwriting the first — and they widen the chip
	// block itself rather than adding a neighbouring one, sitting one pair after
	// each chip's first-play pair. So they head the whole chain and come off
	// with TWO headerWithout calls rather than one: the triple-captain pair
	// first, since removing it does not shift the bench-boost pair's offset and
	// the reverse order would.
	chipWeekBlockAt := floorBlockAt() + floorCols
	noTripleSecondPlay := headerWithout(cellHeader, chipWeekBlockAt+6, 2)
	noSecondPlay := headerWithout(noTripleSecondPlay, chipWeekBlockAt+2, 2)
	noFloor := headerWithout(noSecondPlay, floorBlockAt(), floorCols)
	// ⚠️ The dose block and the option-value block are newer still, and both sit
	// AFTER the fixture-run block, so they come off first and in that order —
	// dose outermost. Because both sit after the fixture-run block, removing them
	// does not move where that block starts, which is why the next line can keep
	// using an offset into the full header on an already-shortened one.
	noDose := headerWithout(noFloor, doseBlockAt(), doseCols)
	noOption := headerWithout(noDose, optionBlockAt(), optionCols)
	noFixtureRuns := headerWithout(noOption, fixtureRunBlockAt(), fixtureRunCols)
	noBanking := headerWithout(noFixtureRuns, bankingBlockAt(), bankingCols)
	noChipOracle := headerWithout(noBanking, chipOracleBlockAtIn(noBanking), chipOracleCols)
	noXPoints := headerWithout(noChipOracle, xPointsBlockAtIn(noChipOracle), xPointsCols)
	noArm := headerWithout(noXPoints, len(noXPoints)-oracleCols-armCols, armCols)
	// historicalChipWeekCols is the chip block's width BEFORE this build's
	// second-play columns — 4, not the current chipWeekCols (8) — because by
	// this point in the chain the second-play columns are already gone
	// (stripped by noSecondPlay, above), so this step has to remove exactly the
	// block that remains here, not the block's current width.
	const historicalChipWeekCols = 4
	noChipWeek := headerWithout(noArm,
		len(noArm)-oracleCols-historicalChipWeekCols, historicalChipWeekCols)
	noOracle := noChipWeek[:len(noChipWeek)-oracleCols]

	// Two plausible older schemas, both synthesised by stripping the trailing
	// blocks this build appended rather than by pasting a literal that would rot.
	// The oracle columns are last, so stripping oracleCols is *literally* the
	// header this build's predecessor wrote, and stripping the captaincy rungs as
	// well is the one before that. A file at either shape must be refused rather
	// than appended to, since a ragged CSV misaligns columns instead of failing.
	stale := map[string][]string{
		// The blocks are stripped in the order they were added, innermost last.
		// ⚠️ The chip block went in *before* the oracle pair, so stripping
		// oracleCols alone stops at `triple_captain_pts` — a header no build ever
		// wrote. Both entries were mislabelled until chipWeekCols was threaded
		// through, and the test still passed, because every synthesised header
		// differs from the current one and is refused whatever it contains.
		// ⚠️ The arm block went in *after* the chip block and *before* the oracle
		// pair, so the true predecessor strips oracleCols and armCols and stops at
		// `triple_captain_pts`. Same trap the chip block sprang: every synthesised
		// header differs from the current one and is refused whatever it contains,
		// so a mislabelled entry here still passes and still lies about which build
		// wrote it.
		// ⚠️ The xPoints block went in *after* the arm block in time but sits
		// *before* it in the schema, because the oracle pair must stay last and the
		// arm block must stay immediately before it. So no stripped suffix of this
		// header is the shipped predecessor at all: the real predecessor is this
		// header with the four xPoints columns removed from the MIDDLE. Synthesised
		// that way rather than by truncation, so the entry's name stays true — the
		// same trap the chip and arm blocks each sprang, where a mislabelled entry
		// passed because every synthesised header differs from the current one and
		// is refused whatever it contains.
		// ⚠️ The chip-oracle block is the second to go in mid-header, one position
		// earlier again. The chain is now built by removing one block at a time
		// from wherever it actually sits, and each entry was checked against the
		// header its named build really wrote — see the note above.
		// ⚠️ The banking block is the third mid-header block and the earliest yet,
		// which is why it heads the chain rather than tailing it. Everything below
		// it shifted down one name; the widths did not move, because they are
		// facts about commits.
		// ⚠️ The fixture-run block is the fourth mid-header block and the newest,
		// sitting immediately after the banking one. It heads the chain and every
		// entry below it shifted down one name again; the widths did not move,
		// because they are facts about commits.
		// ⚠️ Two more mid-header blocks landed together, both AFTER the
		// fixture-run one: the option-value funnels and the dose pair. The dose
		// block is the outermost of the two, so it heads the chain and every
		// entry below it shifted down two names. The widths did not move,
		// because they are facts about commits.
		// ⚠️ The floor block is the newest mid-header block, sitting between the
		// dose block and the chip block. It heads the chain and every entry below
		// it shifted down one name again; the widths did not move, because they
		// are facts about commits.
		// ⚠️ The second-play chip columns are newer still — they widen the chip
		// block itself rather than adding a neighbouring one, so they head the
		// chain and every entry below it shifted down one name and one "builds
		// back" level again; the widths did not move, because they are facts
		// about commits.
		"predecessor (no second-play chip columns)":      noSecondPlay,
		"two builds back (no floor-flip columns either)": noFloor,
		"three builds back (no dose columns either)":     noDose,
		"four builds back (no option funnels either)":    noOption,
		"five builds back (no fixture-run either)":       noFixtureRuns,
		"six builds back (no banking either)":            noBanking,
		"seven builds back (no chip-oracle either)":      noChipOracle,
		"eight builds back (no xpoints either)":          noXPoints,
		"nine builds back (no arm columns either)":       noArm,
		"ten builds back (no chip columns either)":       noChipWeek,
		"eleven builds back (no oracle pair either)":     noOracle,
		"twelve builds back (no captaincy rungs either)": noOracle[:len(noOracle)-captainRungCols],
	}
	// The property these entries have now failed three times to have: a
	// synthesised header must be the one the build it names really wrote.
	// Nothing in this test can notice otherwise, because every wrong header is
	// refused just as readily as a right one — so the check has to be stated
	// separately, against history.
	//
	// The widths and final columns are read off
	// `git show <commit>:internal/backtest/cellcsv_test.go` and are facts about
	// commits that cannot move, which is why they are literals. What rots is the
	// synthesis above, and that is what this pins. A new block added anywhere
	// makes the first entry stop being 50 columns, which is the failure: the
	// entry would then name a build it is no longer describing.
	for _, w := range []struct {
		name string
		cols int
		last string
	}{
		{"predecessor (no second-play chip columns)", 94, "oracle_kind"},
		{"two builds back (no floor-flip columns either)", 92, "oracle_kind"},
		{"three builds back (no dose columns either)", 88, "oracle_kind"},
		{"four builds back (no option funnels either)", 60, "oracle_kind"},
		{"five builds back (no fixture-run either)", 55, "oracle_kind"},
		{"six builds back (no banking either)", 50, "oracle_kind"},
		{"seven builds back (no chip-oracle either)", 40, "oracle_kind"},
		{"eight builds back (no xpoints either)", 36, "oracle_kind"},
		{"nine builds back (no arm columns either)", 33, "oracle_kind"},
		{"ten builds back (no chip columns either)", 29, "oracle_kind"},
		{"eleven builds back (no oracle pair either)", 27, "hold_nocap_per_gw"},
		{"twelve builds back (no captaincy rungs either)", 23, "weekly_per_gw"},
	} {
		got, ok := stale[w.name]
		if !ok {
			t.Fatalf("no synthesised header named %q", w.name)
		}
		if len(got) != w.cols || got[len(got)-1] != w.last {
			t.Errorf("the %q header is %d columns ending %q; the build it names "+
				"wrote %d ending %q — the entry is mislabelled, which is silent "+
				"here because every wrong header is refused too",
				w.name, len(got), got[len(got)-1], w.cols, w.last)
		}
	}

	for name, old := range stale {
		path := filepath.Join(t.TempDir(), "cells.csv")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		w := csv.NewWriter(f)
		if err := w.Write(old); err != nil {
			t.Fatal(err)
		}
		w.Flush()
		f.Close()

		if _, err := openCellSink(path); err == nil {
			t.Fatalf("appending under the %s header must be refused, not "+
				"silently ragged", name)
		}
	}

	// And the same header is accepted, so the check is not simply rejecting
	// every existing file.
	fresh := filepath.Join(t.TempDir(), "cells.csv")
	s1, err := openCellSink(fresh)
	if err != nil {
		t.Fatal(err)
	}
	s1.cell(sampleRow(s1.sweepLabel("A"), s1.run(), "shipped", 0, "2025-26", 1))
	s1.close()
	s2, err := openCellSink(fresh)
	if err != nil {
		t.Fatalf("reopening a file this build wrote must succeed: %v", err)
	}
	s2.close()
}

// TestUnmeasuredLayersAreEmptyNotZero separates "not measured" from "measured as
// nothing".
//
// Only the variance decomposition measures the intermediate layers; an ordinary
// sweep leaves them blank. A zero would read downstream as "the frozen eleven
// scored nothing per gameweek", which is a number R would happily average, and
// the layer marginals are differences between adjacent layers — so one spurious
// zero corrupts two of them.
func TestUnmeasuredLayersAreEmptyNotZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")
	plain := sampleRow(sweep, sink.run(), "shipped", 0, "2025-26", 1)
	sink.cell(plain)

	withLayers := plain
	withLayers.Variant = "nudged"
	withLayers.VariantIndex, withLayers.IsBaseline = 1, false
	withLayers.HasLayers = true
	withLayers.Frozen, withLayers.FrozenCaptain, withLayers.Weekly = 1600, 1780, 1860
	sink.cell(withLayers)
	sink.close()

	layerCols := []string{"frozen", "frozen_captain", "weekly"}
	_, rows := readCells(t, path)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		measured := r["variant"] == "nudged"
		for _, c := range layerCols {
			pts, per := r[c+"_points"], r[c+"_per_gw"]
			if !measured {
				if pts != "" || per != "" {
					t.Fatalf("%s must be blank when unmeasured, got %q/%q", c, pts, per)
				}
				continue
			}
			if pts == "" || per == "" {
				t.Fatalf("%s must be populated when measured, got %q/%q", c, pts, per)
			}
			p, err := strconv.Atoi(pts)
			if err != nil {
				t.Fatal(err)
			}
			weeks, err := strconv.Atoi(r["weeks"])
			if err != nil {
				t.Fatal(err)
			}
			got, err := strconv.ParseFloat(per, 64)
			if err != nil {
				t.Fatal(err)
			}
			if want := float64(p) / float64(weeks); got != want {
				t.Fatalf("%s_per_gw %v is not %d/%d", c, got, p, weeks)
			}
		}
	}

	// And an infeasible cell must not claim layer measurements either.
	inf := withLayers.asInfeasible()
	if inf.HasLayers {
		t.Fatal("an infeasible cell must not claim to carry layer measurements")
	}
}

// TestCaptainRungsAreEmptyNotZero is the same separation for the captaincy rungs
// as TestUnmeasuredLayersAreEmptyNotZero is for the frozen ladder, and it needs
// its own test because the two blocks are gated independently: an ordinary sweep
// measures the rungs and not the ladder, and the variance decomposition measures
// the ladder and not the rungs.
//
// The zero is genuinely dangerous on this block in a way it is not on the others.
// `hold_nocap` is HOLD with nobody doubled, so a zero in that column is a
// *plausible* season total for a metric whose whole point is that a term has been
// removed — nothing about it looks wrong, and R would average it happily.
func TestCaptainRungsAreEmptyNotZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")

	// A variance-decomposition-shaped row: the frozen ladder measured, the
	// captaincy rungs not.
	ladderOnly := sampleRow(sweep, sink.run(), "shipped", 0, "2025-26", 1)
	ladderOnly.HasLayers = true
	ladderOnly.Frozen, ladderOnly.FrozenCaptain, ladderOnly.Weekly = 1600, 1780, 1860
	sink.cell(ladderOnly)

	// An ordinary-sweep-shaped row: the rungs measured, the ladder not. The two
	// rung values differ from each other and from hold_points, so a permutation
	// between the three columns is catchable.
	rungsOnly := sampleRow(sweep, sink.run(), "nudged", 1, "2025-26", 1)
	rungsOnly.HasCaptainRungs = true
	rungsOnly.HoldFixedCaptain, rungsOnly.HoldNoCaptain = 1830, 1700
	sink.cell(rungsOnly)
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		measured := r["variant"] == "nudged"
		for col, want := range map[string]int{
			"hold_fixedcap": 1830, "hold_nocap": 1700,
		} {
			pts, per := r[col+"_points"], r[col+"_per_gw"]
			if !measured {
				if pts != "" || per != "" {
					t.Fatalf("%s must be blank when unmeasured, got %q/%q — a zero "+
						"here is a plausible season total, not an obvious gap",
						col, pts, per)
				}
				continue
			}
			if atoiOrFail(t, pts) != want {
				t.Fatalf("%s_points carries %q, want %d — cellHeader and "+
					"cellSink.cell have desynchronised", col, pts, want)
			}
			weeks := atoiOrFail(t, r["weeks"])
			got, err := strconv.ParseFloat(per, 64)
			if err != nil {
				t.Fatal(err)
			}
			if wantPer := float64(want) / float64(weeks); got != wantPer {
				t.Fatalf("%s_per_gw %v is not %d/%d", col, got, want, weeks)
			}
		}
		// The frozen ladder must not have leaked into the rung row or vice versa;
		// they are separate flags and a single HasLayers-style gate covering both
		// would make one of the two sweep shapes emit six phantom columns.
		if measured && r["frozen_points"] != "" {
			t.Fatalf("a rung-only row must not carry ladder columns: %q", r["frozen_points"])
		}
		if !measured && r["hold_nocap_points"] != "" {
			t.Fatalf("a ladder-only row must not carry rung columns: %q", r["hold_nocap_points"])
		}
	}

	inf := rungsOnly.asInfeasible()
	if inf.HasCaptainRungs {
		t.Fatal("an infeasible cell must not claim to carry captaincy rungs")
	}
}

// TestTheBankingColumnsSeparateOffFromNeverFired pins the whole reason the block
// exists.
//
// The recorded verdict that the policy never banks a transfer was reached with no
// column recording whether the rule ever fired, so it could not be told apart from
// an arm that was wired and never reached — which is this project's signature
// failure and the case its "a byte-identical result is not a tie" rule names. The
// separation is spelled in the file's own idiom, blank for a gap and zero for a
// measurement, and it only works if the two columns are gated *independently*:
// banked_weeks on the rule having been consulted, free_at_decision on a decision
// week having happened at all.
//
// So there are four readings and this pins each of them. A single gate covering
// both columns would collapse the middle two — "banking was off" and "banking was
// on and never fired" — back into one blank, which is exactly the state that
// cannot be read.
//
// It also pins the denominator. free_at_decision is a mean over DECISION weeks,
// not over gameweeks played: a wildcard or free-hit week plays football and makes
// no decision, so dividing by `weeks` would understate the allowance by however
// many chips the arm played. The sample rows below carry different values for the
// two, so a swapped denominator fails rather than agreeing by coincidence.
func TestTheBankingColumnsSeparateOffFromNeverFired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")

	// FreeHeld is chosen so the mean is exact in binary and so that dividing by
	// `weeks` (38, from sampleRow's start of 1) would give a different number.
	// Every count differs from every other, so a permutation of two same-typed
	// columns fails rather than agreeing by coincidence.
	rowsIn := map[string]BankingMediator{
		"banking-off": {DecisionWeeks: 30, FreeHeld: 45},
		"banking-on-never-fired": {
			DecisionWeeks: 30, ConsultedWeeks: 29, WeighedWeeks: 18, FreeHeld: 60,
		},
		"banking-on-fired": {
			DecisionWeeks: 30, ConsultedWeeks: 28, WeighedWeeks: 19,
			BankedWeeks: 7, FreeHeld: 90,
		},
	}
	sink.cell(sampleRow(sweep, sink.run(), "block-not-recorded", 0, "2025-26", 1))
	for _, v := range []string{"banking-off", "banking-on-never-fired", "banking-on-fired"} {
		r := sampleRow(sweep, sink.run(), v, 0, "2025-26", 1)
		r.HasBanking, r.BankingMediator = true, rowsIn[v]
		sink.cell(r)
	}
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 4 {
		t.Fatalf("want 4 rows, got %d", len(rows))
	}
	// decision, consulted, weighed, banked, free_at_decision.
	want := map[string][5]string{
		"block-not-recorded":     {"", "", "", "", ""},
		"banking-off":            {"30", "0", "", "", "1.5"},
		"banking-on-never-fired": {"30", "29", "18", "0", "2"},
		"banking-on-fired":       {"30", "28", "19", "7", "3"},
	}
	cols := [5]string{
		"decision_weeks", "consulted_weeks", "weighed_weeks",
		"banked_weeks", "free_at_decision",
	}
	for _, r := range rows {
		w, ok := want[r["variant"]]
		if !ok {
			t.Fatalf("unexpected variant %q", r["variant"])
		}
		for i, col := range cols {
			if got := r[col]; got != w[i] {
				t.Fatalf("%s: %s is %q, want %q — the two columns the rule owns go "+
					"blank when it was never consulted, the two the decision loop "+
					"owns are written whenever it ran, and free_at_decision is a "+
					"mean over decision weeks rather than over `weeks`",
					r["variant"], col, got, w[i])
			}
		}
	}

	// And an infeasible cell carries neither. The block is a count of what the
	// decision loop did, and a cell that could not field a fifteen ran none.
	inf := cellRow{HasBanking: true, BankingMediator: BankingMediator{
		DecisionWeeks: 30, ConsultedWeeks: 30, BankedWeeks: 7, FreeHeld: 90,
	}}.asInfeasible()
	if inf.HasBanking || inf.BankingMediator != (BankingMediator{}) {
		t.Fatalf("an infeasible cell must not claim decision weeks: %+v", inf.BankingMediator)
	}
}

// TestTheBankingBlockIsBeforeTheChipBlockAndCounted is this block's position
// assertion, on the reasoning every block after it already states: a column
// dropped in between two counted blocks is invisible to a test that indexes from
// either end, so every block gets its own.
//
// Both neighbours are asserted by name. The banking pair is the only block that
// went in ahead of the chip columns, so nothing else pins that seam, and the
// stale-header chain removes it first — an entry that stopped being where this
// says would silently re-label every build in that chain.
func TestTheBankingBlockIsBeforeTheChipBlockAndCounted(t *testing.T) {
	want := []string{
		"decision_weeks", "consulted_weeks", "weighed_weeks",
		"banked_weeks", "free_at_decision",
	}
	if bankingCols != len(want) {
		t.Fatalf("bankingCols is %d and the block is %d columns", bankingCols, len(want))
	}
	at := bankingBlockAt()
	got := cellHeader[at : at+bankingCols]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the %d columns before the chip block are %v, want %v",
			bankingCols, got, want)
	}
	if before := cellHeader[at-1]; before != "hold_nocap_per_gw" {
		t.Fatalf("the column before the banking block is %q, want the captaincy "+
			"rungs' last column", before)
	}
	// The banking block's right-hand neighbour is the fixture-run block, not the
	// chip block: the two mediators sit together in one decision-mediator region,
	// and the chip columns moved five to the right when the second funnel landed.
	if after := cellHeader[at+bankingCols]; after != "band_ready_weeks" {
		t.Fatalf("the column after the banking block is %q, want the fixture-run "+
			"block's first column", after)
	}
	// And the synthesised predecessor really is this header minus exactly those
	// two, which is the property the stale-header chain's first entry rests on.
	if n := len(withoutBankingBlock()); n != len(cellHeader)-bankingCols {
		t.Fatalf("withoutBankingBlock has %d columns, want %d",
			n, len(cellHeader)-bankingCols)
	}
	for _, c := range withoutBankingBlock() {
		for _, w := range want {
			if c == w {
				t.Fatalf("withoutBankingBlock still carries %q", c)
			}
		}
	}
}

// TestTheXPointsColumnsAreEmptyNotZeroAndDivideByWeeks is the accumulated-xPoints
// block's half of the two rules every other block here already keeps.
//
// The blank rule matters more on this block than on any other. `hold_xpoints` is a
// season total on a metric whose entire claim is that it is a *smoother reading of
// the same season*, so a 0.0 in it is not merely plausible, it is the shape a
// reader would least question — and the variance decomposition builds its own
// cellRow and does not measure these, so an ungated block would put that zero in a
// real file.
//
// The denominator rule matters because these are the only float columns in the
// schema that carry a per-gameweek twin, so they are the only ones where a
// hand-written `perGW` variant could divide by something other than `weeks`. A
// fixed 38 is the specific error: it is right for exactly the GW1 cells and wrong
// by a factor of nearly three at GW26, which is the pooling bug this project has
// already shipped once, arriving through a new column.
func TestTheXPointsColumnsAreEmptyNotZeroAndDivideByWeeks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")

	// Two entry points, so a denominator that ignores `weeks` cannot agree with
	// both — a single GW1 row would pass against a fixed 38.
	want := map[int][2]float64{}
	for _, start := range []int{1, 26} {
		r := sampleRow(sweep, sink.run(), "measured", 0, "2025-26", start)
		r.VariantIndex, r.IsBaseline = 0, true
		r.HasXPoints = true
		// Distinguishable from each other and from the points columns, so a
		// permutation between any two of the four is catchable.
		r.HoldXPoints = float64(r.HoldPoints) + 0.25
		r.PolicyXPoints = float64(r.PolicyPoints) + 0.75
		want[start] = [2]float64{r.HoldXPoints, r.PolicyXPoints}
		sink.cell(r)
	}
	// And one row from a sweep that does not measure them at all.
	unmeasured := sampleRow(sweep, sink.run(), "not measured", 1, "2025-26", 1)
	sink.cell(unmeasured)
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	for _, r := range rows {
		cols := []string{"hold_xpoints", "hold_xpoints_per_gw",
			"policy_xpoints", "policy_xpoints_per_gw"}
		if r["variant"] == "not measured" {
			for _, c := range cols {
				if r[c] != "" {
					t.Errorf("%s must be blank when unmeasured, got %q — a 0.0 "+
						"season total on this metric is the least questionable "+
						"wrong number in the schema", c, r[c])
				}
			}
			continue
		}
		start := atoiOrFail(t, r["start_gw"])
		weeks := atoiOrFail(t, r["weeks"])
		for i, pair := range [][2]string{
			{"hold_xpoints", "hold_xpoints_per_gw"},
			{"policy_xpoints", "policy_xpoints_per_gw"},
		} {
			tot, err := strconv.ParseFloat(r[pair[0]], 64)
			if err != nil {
				t.Fatalf("%s unparseable: %q", pair[0], r[pair[0]])
			}
			if tot != want[start][i] {
				t.Fatalf("%s carries %v, want %v — cellHeader and cellSink.cell "+
					"have desynchronised", pair[0], tot, want[start][i])
			}
			per, err := strconv.ParseFloat(r[pair[1]], 64)
			if err != nil {
				t.Fatalf("%s unparseable: %q", pair[1], r[pair[1]])
			}
			// Exact, like the points columns: both are written at full float64
			// precision so R's own re-derivation compares arithmetic rather than
			// formatting, and a loosened tolerance here is how a wrong
			// denominator gets waved through.
			if wantPer := tot / float64(weeks); per != wantPer {
				t.Errorf("%s is %v, want %v/%d = %v — the denominator is not the "+
					"gameweeks this cell played", pair[1], per, tot, weeks, wantPer)
			}
		}
	}

	// An infeasible cell claims none of it: no fifteen was fielded, so there is no
	// season on either metric.
	measured := sampleRow(sweep, sink.run(), "measured", 0, "2025-26", 1)
	measured.HasXPoints = true
	measured.HoldXPoints, measured.PolicyXPoints = 1900.5, 2000.5
	inf := measured.asInfeasible()
	if inf.HasXPoints || inf.HoldXPoints != 0 || inf.PolicyXPoints != 0 {
		t.Fatal("an infeasible cell must not claim xPoints totals")
	}
}

// TestCellSinkIsConcurrencySafe follows the standing rule in this package rather
// than the current call pattern.
//
// runPolicySweep writes cells from a single goroutine today, so nothing here is
// racing yet. But this project's rule is that anything mutable on a shared object
// is lock-guarded, and it earned that rule the expensive way — a map built under a
// plain nil check is a *fatal* concurrent map write, and one took down a live run.
// A sweep that ever parallelises its cells would otherwise interleave CSV records
// and hand two blocks the same label. Cheap to pin now; the same argument as
// TestConcurrentOverridesAllPersist.
// The oracle stamp is written concurrently too, and it is the column where an
// interleave would do the most damage: a row that lands under another arm's
// stamp is a hindsight figure filed as an ordinary one, which is precisely the
// mistake the whole provenance layer exists to prevent. So half the goroutines
// write an oracled row and the assertion is that every row's stamp still matches
// its own variant.
func TestCellSinkIsConcurrencySafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	oracled := Oracles{Info: OracleAvailability}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sw := sink.sweepLabel("P")
			row := sampleRow(sw, sink.run(), "v", i%2, "2025-26", 1)
			o := Oracles{}
			if i%2 == 1 {
				o = oracled
				row.Variant = "oracle"
			}
			row.Oracle, row.OracleKind = o.Stamp(), o.Kind()
			sink.cell(row)
			sink.mean(sw, "hold", row.Variant, "base", 1, float64(i), 24, o.Stamp())
		}(i)
	}
	wg.Wait()
	sink.close()

	// Every record intact — an interleaved write would produce a ragged row that
	// the CSV reader rejects outright.
	_, rows := readCells(t, path)
	if len(rows) != 20 {
		t.Fatalf("want 20 rows, got %d", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r["sweep"]] {
			t.Fatalf("duplicate sweep label under concurrency: %q", r["sweep"])
		}
		seen[r["sweep"]] = true
		want, wantKind := Oracles{}.Stamp(), Oracles{}.Kind()
		if r["variant"] == "oracle" {
			want, wantKind = oracled.Stamp(), oracled.Kind()
		}
		if r["oracle"] != want || r["oracle_kind"] != wantKind {
			t.Fatalf("variant %q carries stamp %q/%q, want %q/%q — a row filed "+
				"under another arm's hindsight",
				r["variant"], r["oracle"], r["oracle_kind"], want, wantKind)
		}
	}

	_, means := readCells(t, strings.TrimSuffix(path, ".csv")+".means.csv")
	if len(means) != 20 {
		t.Fatalf("want 20 mean rows, got %d", len(means))
	}
	for _, m := range means {
		want := Oracles{}.Stamp()
		if m["variant"] == "oracle" {
			want = oracled.Stamp()
		}
		if m["oracle"] != want {
			t.Fatalf("mean row for %q carries stamp %q, want %q — an averaged "+
				"number is the one that gets pasted into a document",
				m["variant"], m["oracle"], want)
		}
	}
}

// TestInferenceLivesInOnePlace counts the hand-rolled variance estimators in this
// package, the way TestEveryScoringEngineGetsRecency counts scoring engines.
//
// The claim in AGENTS.md is that SEs, degrees of freedom and p-values are computed
// in stats/sweep_inference.R and nowhere else. That claim was **false when first
// written**: vicecaptain_test.go carried a line-for-line copy of the naive and
// season-clustered estimators that had just been deleted from
// transferpolicy_test.go, so the retired estimator was still producing a figure
// recorded in AGENTS.md. Two implementations of one quantity is this project's
// most-repeated bug, and the reason it survives is that nothing counts them.
//
// This scans source rather than behaviour, which is unusual here and is the point:
// the failure mode is a *new* file quietly growing a third copy, and no runtime
// assertion can see that.
//
// **Scope matters, or this test is worthless.** It flags only the
// season-clustering idiom — collapsing seasons to one number each and taking their
// spread — which is specific to paired sweep cells. It deliberately does not flag
// the per-player, per-team-match and per-move standard errors in the calibration
// diagnostics: those measure a different unit, the cell is not their unit of
// replication, and routing them through a per-cell CSV would be a category error.
//
// # The critical-value table is the second thing counted here
//
// `tCrit95` in stats_test.go tabulates `qt(0.975, df)`, which `stats/` computes
// from R's own distribution function in six scripts. That is one quantity in two
// languages, and it cannot be collapsed: a diagnostic that prints a t beside a
// computed df — this package has done so since before either scan existed — needs
// the number in Go, while every *verdict* still comes from the R path. What can
// be prevented is a THIRD copy, which is the standing remedy: extend the scan
// that already exists rather than add a runtime equivalence test per copy.
//
// The marker-string half above cannot see it, because a table is ten float
// literals and no idiom. So this half walks the AST for a tabulated critical
// value appearing in code outside its home. Literals only, so the many comments
// quoting 3.182 and 2.571 as *recorded* thresholds stay legal — the same line
// TestPrintedGridLabelsAreDerived draws. Verified when written: across `internal`
// and `cmd`, the only code-position occurrences of any of the ten are the table
// itself.
func TestInferenceLivesInOnePlace(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// The season-clustered SE, in the shape both copies had it: build a
	// per-season mean, then take the spread of those. Assembled from fragments so
	// this file does not contain the markers it scans for — the first version did,
	// and flagged itself.
	markers := []string{"season" + "Means", "cluster" + "SE", "bySeason" + "Sum"}
	self := "cellcsv_regression_test.go"
	// tCrit95's home. The values are Student's t at 0.975 for df 1..10, and they
	// are assembled from fragments for the same reason the markers above are: a
	// scanner that contains what it scans for flags itself.
	tCritHome := "stats_test.go"
	tCritValues := map[string]bool{
		"12.7" + "06": true, "4.3" + "03": true, "3.1" + "82": true, "2.7" + "76": true,
		"2.5" + "71": true, "2.4" + "47": true, "2.3" + "65": true, "2.3" + "06": true,
		"2.2" + "62": true, "2.2" + "28": true,
	}
	fset := token.NewFileSet()
	var offenders []string
	for _, f := range files {
		base := filepath.Base(f)
		if base == self {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, m := range markers {
			if strings.Contains(src, m) {
				offenders = append(offenders, f+" contains "+m)
			}
		}
		if base == tCritHome {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if ok && lit.Kind == token.FLOAT && tCritValues[lit.Value] {
				offenders = append(offenders, base+":"+
					strconv.Itoa(fset.Position(lit.Pos()).Line)+
					" hardcodes the critical value "+lit.Value)
			}
			return true
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("season-clustered SEs are computed in stats/sweep_inference.R and "+
			"nowhere else, and the critical values they are read against live in "+
			"tCrit95 in %s; found %v.\nIf this is a genuine new sweep harness, emit "+
			"cells via runPolicySweep and let R do the inference. If it measures a "+
			"different unit (per player, per match, per move), it is not a sweep and "+
			"should not be clustering seasons at all. If it is a critical value being "+
			"printed beside a computed df, call tCrit95 — a second table is how the "+
			"two 3.182s survived the grid widening to df 5.", tCritHome, offenders)
	}
}

// TestTheGridIsDeclaredOnce counts the copies of the replay grid.
//
// The four season pairs were written out verbatim in eleven files and the
// six-entry-point grid in five. That is not a tidiness complaint: adding a fifth
// season pair meant eleven edits, and a diagnostic that missed one would silently
// measure a different population from the sweep it was being compared against —
// while still printing a plausible number.
//
// Source-scanning for the same reason as TestInferenceLivesInOnePlace: the failure
// is a *new* file pasting the literal back in, and no runtime assertion sees that.
func TestTheGridIsDeclaredOnce(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Fragments, so this file does not match its own scanner.
	pairLit := `{"2021-22", ` + `"2022-23"}`
	startLit := "1, 6, 11, " + "16, 21, 26"
	home := "harness_test.go"

	var offenders []string
	for _, f := range files {
		base := filepath.Base(f)
		if base == home || base == "cellcsv_regression_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if strings.Contains(src, pairLit) {
			offenders = append(offenders, base+" re-declares the season pairs")
		}
		if strings.Contains(src, startLit) {
			offenders = append(offenders, base+" re-declares the start grid")
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("the replay grid lives in %s (sweepPairNames, sweepStarts) and "+
			"nowhere else; found %v", home, offenders)
	}
}

// gridLabelInProse matches a grid size written out in a printed string: a count
// as a word or a numeral, attached to the unit a diagnostic measures in.
//
// "24 cells", "four seasons", "4 seasons x 6 starts", "six-season", "four
// clusters". A derived label reads "%d cells" or "%s", so a format verb in the
// count's place is what passing looks like.
//
// Two deliberate holes, both because the alternative is a scanner nobody can
// keep passing. **"one" is not a count here**: every occurrence in this package
// is `leave-one-season-out`, "one cell of 36" or "one start and one absence" —
// descriptions of a unit, never a grid, since no grid is one season wide. And
// the leading character class excludes a hyphen, a digit and a decimal point,
// because `\b` treats all three as word boundaries: without it "2019-20 season"
// matches on its own second half, and "the 0.95 start-share band" matches on
// "95 start". Both were observed, the second outside this package.
// Group 2 is the label; group 1 is the character consumed to look behind.
var gridLabelInProse = regexp.MustCompile(
	`(?i)(^|[^-.0-9a-z])((two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[0-9]+)[ -](season|cell|cluster|start|entry point)s?)\b`)

// allowedGridLabels are the printed labels that must NOT be derived, each with
// the reason it is an exception.
//
// The list is short and per-line on purpose. A category exemption — "this file
// quotes history" — would re-admit the failure through the file it exempts, so
// an exemption names the fragment it permits and dies when that fragment is
// edited.
//
// **How many there are is reported by the test and asserted in no prose,
// including here.** The first write-up of this scan said seven when there were
// eight, which is the same defect one level up: an uncounted quantity gets
// written several different ways, and the exemption count is the only reviewable
// summary of the escape hatch.
var allowedGridLabels = []struct{ file, fragment, why string }{
	{
		"variance_test.go", "measured at 24 cells, the four-season grid",
		"a recorded figure: -0.717 pts/gw was measured when the default grid was " +
			"four seasons, and deriving its label would relabel a past measurement " +
			"with the grid running now",
	},
	{
		"extendedseasons_test.go", "the four seasons FPL publishes it for run 1068 to 1198",
		"an archive fact, not a grid: FPL publishes expected goals for four seasons " +
			"and the 1068-1198 band is the range measured on exactly those",
	},
	{
		"anchored_diag_test.go", "30 cells against 3-5 for the lag arms",
		"a recorded confound, measured before this arm dropped the wildcard: pinning " +
			"it to a common week put the boost after the rebuild in 30 of 30 cells for " +
			"one arm and 3-5 for the others. The counts belong to that run, not to this one",
	},
	{
		"tworegime_diag_test.go", "unplaced in 13 of 36 cells",
		"a recorded result belonging to a DIFFERENT test's grid: it is how often " +
			"TestDiagAnchoredChips leaves the triple captain unplayed, counted as " +
			"triple_captain_gw == 0 in stats/cells/2026-08-25-f7d2be1b/anchored.csv " +
			"and identical across all five arms there. Printed here as the reason " +
			"these two arms are not comparable. Deriving it from THIS grid would " +
			"relabel that measurement with a run it did not come from — " +
			"FPL_SWEEP_SEASONS moves this grid and not that recorded 13 of 36. " +
			"⚠️ It first said SIXTEEN, and the exemption was granted before the count " +
			"was checked against the cells; the file it came from is named above so " +
			"the next reader can check rather than trust",
	},
	{
		"harness_test.go", "need at least two entry points",
		"a minimum a parser enforces, not a description of any grid: parseSweepStarts " +
			"rejects FPL_SWEEP_STARTS with fewer than two, whatever the default is",
	},
	{
		"teamshare_test.go", "four-season run the two positive seasons were the same pair",
		"a recorded reading: WHICH two seasons carried the pooled gain was read off " +
			"the four-season run, while the sign count and the leave-one-out beside " +
			"it are computed, so only the identification stays historical",
	},
	{
		"residualcoverage_diag_test.go", "on the six-season default grid the share was flat",
		"a recorded reading, and the only grid it can be asserted on: the six-season " +
			"default is the one carrying both backfilled and FPL-fed seasons, so a " +
			"derived count would re-assert 'backfilled and FPL-fed alike' over a " +
			"four-season grid that has no backfilled season in it",
	},
	{
		"cellforensics_test.go", "three cells carry 99% of it",
		"a recorded concentration, and the point of the row: it names which cells " +
			"carried a past contrast so they can be replayed one at a time",
	},
	{
		"seasonneedsguard_test.go", "one of only two seasons the backfill adds",
		"a fact about the Understat backfill — it makes 2019-20 and 2020-21 playable — " +
			"and not a count of the grid this test runs",
	},
}

// TestPrintedGridLabelsAreDerived reads the OUTPUT for a stale grid label, which
// TestTheGridIsDeclaredOnce above cannot.
//
// That test scans for the grid's *source* literals — the season pairs and the
// entry points — so it catches a diagnostic that pastes the grid back in. It
// cannot see a diagnostic that reads the shared grid correctly and then prints
// "24 cells (4 seasons x 6 starts)" over it, because a format string is prose and
// prose is what nothing was checking. Twenty-nine such labels were stale when this
// was written, and the split is the reason to state the method beside the count:
// **8 came from the two text greps recorded in `8c05c69`'s message and the other
// 21 from this AST scan.** ⚠️ An earlier version of this sentence said the greps
// found "29 candidates", which is the TOTAL written into the wrong slot and made
// the arithmetic unreconcilable — 29 = 29 + 21 is not a split.
//
// Only the 29 and the 21 are measured. The 29 is checkable here: that commit adds
// exactly 29 `gridLabel`/`seasonsLabel` call sites outside the file defining them
// and this one. The 21 is that commit's own statement. **The 8 is arithmetic on
// those two, not a separate count of what each grep found** — worth saying,
// because the commit also records that each hit was "judged individually, not
// substituted", so a grep hit could have been judged not to need deriving at all.
// The greps' raw hit counts were 108 and 118 **at `8c05c69^`** and are not
// candidate counts: both are dominated by comments, which is what makes a text
// search the weaker half here and why the scan found more than it did.
//
// ⚠️ **No snapshot population string was ever wrong**, and a first write-up said
// one was. The three `sink.emitAll` grid strings that changed were all accurate:
// two read `playedSeasons(needsSweep)`, which `FPL_SWEEP_SEASONS` does not move,
// and the third matches `tackledSeasons`. Deriving them is future-proofing
// against `seasonCapabilities` gaining next summer's season, not a repair — but
// it is the highest-stakes place a label can go stale, because the snapshot
// renders it as the population a figure was measured on.
//
// It scans string literals only, via the AST, and that is the useful half of the
// design: a doc comment recording "measured at 24 cells" is a statement about
// history and stays legal, while the same words in something that gets printed
// have to be derived or exempted by name.
//
// # The package boundary is deliberate, and was checked
//
// The glob is this package's own `*.go`, because the grid is a backtest concept
// and `sweepPairNames` is a test function no other package can reach. The rest of
// the tree was scanned with the same pattern when this landed: what it finds
// outside here is recorded figures quoted to the agent (the twelve round-trips
// over three seasons, in `internal/agent`), the retraction records in
// `internal/snapshot`, and test fixtures describing their own local populations —
// all category (b), none derivable. ✅ The one live phrase this left standing —
// `internal/snapshot/render.go` printing "six season pairs by default" into every
// snapshot, one clause under a comment telling the reader not to restate a grid
// width there — was **deleted 2026-08-15**, which was the only available fix: the
// grid is chosen by `FPL_SWEEP_SEASONS` and recorded in the sweep's provenance,
// and `internal/snapshot` can see neither, so there was nothing there to derive it
// from. ⚠️ `internal/snapshot` is a watched path for the REVIEW digest only —
// `SnapshotWatchedPaths` is `internal/analysis`, `internal/backtest`,
// `internal/config` and `config.json` — so that edit re-keyed one digest and not
// two. Reading "watched" as one list is the mistake; there are two, with different
// questions behind them.
//
// Four things it still cannot see, stated because a guard whose reach is assumed
// is worse than one whose reach is known. **A label assembled from pieces** — a
// fragment per Printf — reads as several innocent literals; one such split
// existed in csbias_test.go and was joined rather than exempted, which is the
// remedy. **A count passed as an argument**, `Printf("%d cells", 24)`, is a
// literal in the wrong node. **A number-word past twelve**, since the
// alternation stops there. And it checks only that a printed count is
// *computed*, never that it is computed from the right thing: a label deriving
// its season count from the wrong slice passes here, which is what
// `gridLabel(len(pairs), len(starts))` at the call site is for.
func TestPrintedGridLabelsAreDerived(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var offenders []string
	used := make([]bool, len(allowedGridLabels))
	for _, f := range files {
		base := filepath.Base(f)
		// This file carries the pattern and the exemptions, so it would match
		// itself — the same reason the two scanners above skip their own homes.
		if base == "cellcsv_regression_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			// Exemption is per LABEL, not per literal. Matching the whole string
			// would let a second, stale label ride into an exempted sentence — the
			// allowlist would then be granting cover it was never read as granting.
			var exempt [][2]int
			for i, a := range allowedGridLabels {
				if a.file != base {
					continue
				}
				if j := strings.Index(s, a.fragment); j >= 0 {
					used[i] = true
					exempt = append(exempt, [2]int{j, j + len(a.fragment)})
				}
			}
		next:
			for _, loc := range gridLabelInProse.FindAllStringSubmatchIndex(s, -1) {
				lo, hi := loc[4], loc[5] // group 2, the label itself
				for _, e := range exempt {
					if lo >= e[0] && hi <= e[1] {
						continue next
					}
				}
				offenders = append(offenders, base+":"+
					strconv.Itoa(fset.Position(lit.Pos()).Line)+" prints "+strconv.Quote(s[lo:hi]))
			}
			return true
		})
	}
	// An exemption that matches nothing is a rename, a retyped quote or a deleted
	// line, and all three read downstream as "the allowlist is still describing
	// the code" — the same argument WatchedDigest makes about a watch-list entry
	// matching no files.
	for i, a := range allowedGridLabels {
		if !used[i] {
			offenders = append(offenders, "allowedGridLabels entry for "+a.file+
				" matches nothing: "+strconv.Quote(a.fragment))
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a printed grid size must be derived from the grid that ran — "+
			"gridLabel(len(pairs), len(starts)) or seasonsLabel(len(seasons)) in "+
			"harness_test.go — because sweepPairNames returns %d pairs by default "+
			"and FPL_SWEEP_SEASONS/FPL_SWEEP_STARTS move both counts within a run.\n"+
			"found %v\n"+
			"If the label describes a RECORDED result rather than the run in front "+
			"of you, it stays a literal and is added to allowedGridLabels with its "+
			"reason — there are %d such exemptions today, and that count is reported "+
			"here rather than written down anywhere.",
			len(sweepPairNames()), offenders, len(allowedGridLabels))
	}
}

// TestDescriptiveStatsAreDeclaredOnce counts the copies of "the mean of a
// []float64".
//
// There were six: two package-level functions in different files and four
// byte-identical local closures, plus a seventh computed inline inside meanSE.
// None was producing a wrong answer, which is the point — every desynchronised
// mirror in this package's history was harmless when it was written.
//
// A local closure is the easy way to regrow this, so that is what the scan looks
// for. The one exception is deliberate and documented in stats_test.go:
// promoted_test.go takes a projection function over a slice of structs, which is a
// different signature doing a different job.
func TestDescriptiveStatsAreDeclaredOnce(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Fragments again, so this file does not match itself.
	patterns := []string{
		"mean := " + "func(xs []float64)", "mean := " + "func(x []float64)",
		"sd := " + "func(", "seOf := " + "func(", "median := " + "func(",
	}
	home := "stats_test.go"
	var offenders []string
	for _, f := range files {
		base := filepath.Base(f)
		if base == home || base == "cellcsv_regression_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, p := range patterns {
			if strings.Contains(src, p) {
				offenders = append(offenders, base+" declares a local `"+p+"`")
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("meanOf, sd, seOf, meanSE and median live in %s; found %v.\n"+
			"If a call site needs different semantics, say so at the shared "+
			"definition rather than forking it locally.", home, offenders)
	}
}

// TestTheSweepConfigCarriesTheShippedSettings pins what the eight unified
// SimConfig literals actually carried.
//
// **The first version of this test was a tautology and a review caught it.** It
// normalised the bank and then compared the two configs with reflect.DeepEqual —
// but seasonConfig is *derived* from sweepConfig by overwriting one field, so that
// compared a value against a copy of itself. Every field except the bank was
// unguarded. Changing `FreeCost: cfg.Review.FreeTransferValue` to `FreeCost: 0`,
// or `Budget: 1000` to `Budget: 100`, or sourcing MinGain from MinGainForHit,
// would have passed: both arms carry the error identically, the whole non-DIAG
// suite stays green, and every replayed transfer decision changes while every
// POLICY figure in AGENTS.md silently stops reproducing.
//
// So the fields are asserted against `cfg` directly. That is the only thing
// standing between a typo here and eleven diagnostics quietly measuring something
// else — a comparison between the two constructors can never provide it, because
// they share the code being checked.
//
// It uses config.Default() rather than the shipped file on purpose: this test is
// not DIAG-gated, and config.Load *creates* the file when absent, so reading an
// absolute path here would have a plain `go test ./...` write to an unrelated
// location on any machine that is not this one. The structural claim does not
// depend on the values being the real ones.
func TestTheSweepConfigCarriesTheShippedSettings(t *testing.T) {
	cfg := config.Default()
	for _, start := range []int{0, 1, 21} {
		for _, weekly := range []bool{false, true} {
			sw := sweepConfig(cfg, start, weekly)

			// Every field the old literals set, against the source they read it
			// from. A wrong source is the failure mode, not a wrong constant.
			if sw.MinGain != cfg.Review.MinGainForTransfer {
				t.Fatalf("MinGain reads %v, want cfg.Review.MinGainForTransfer %v",
					sw.MinGain, cfg.Review.MinGainForTransfer)
			}
			if sw.MinGainHit != cfg.Review.MinGainForHit {
				t.Fatalf("MinGainHit reads %v, want cfg.Review.MinGainForHit %v",
					sw.MinGainHit, cfg.Review.MinGainForHit)
			}
			if sw.MaxHits != cfg.Review.MaxHitsPerWeek {
				t.Fatalf("MaxHits reads %v, want cfg.Review.MaxHitsPerWeek %v",
					sw.MaxHits, cfg.Review.MaxHitsPerWeek)
			}
			if sw.FreeCost != cfg.Review.FreeTransferValue {
				t.Fatalf("FreeCost reads %v, want cfg.Review.FreeTransferValue %v",
					sw.FreeCost, cfg.Review.FreeTransferValue)
			}
			if !reflect.DeepEqual(sw.Weights, cfg.Weights) {
				t.Fatal("Weights is not the config's Weights")
			}
			// £100.0m in tenths, which is what every literal carried. Not
			// DefaultBudget: the replay states it explicitly, and this project has
			// already been bitten by two defaults for one quantity.
			if sw.Budget != 1000 {
				t.Fatalf("Budget reads %d, want 1000", sw.Budget)
			}
			if sw.BankUpTo != sweepBankLimit {
				t.Fatalf("sweepConfig must pin sweepBankLimit, got %d", sw.BankUpTo)
			}
			if sw.StartGW != start || sw.WeeklyXI != weekly {
				t.Fatalf("a parameter did not reach the struct: start=%d weekly=%v got %+v",
					start, weekly, sw)
			}

			// Nothing the literals left alone may acquire a value here. These are
			// the knobs each block sets for itself, and a default sneaking in would
			// change every diagnostic at once.
			var zero SimConfig
			if sw.BenchWeight != zero.BenchWeight || sw.MaxMoves != zero.MaxMoves ||
				sw.DecisionHorizon != zero.DecisionHorizon ||
				sw.MinutesHalfLife != zero.MinutesHalfLife ||
				sw.MinExpectedMinutes != zero.MinExpectedMinutes ||
				sw.Unified != zero.Unified || sw.BankLookahead != zero.BankLookahead ||
				sw.MaxFundingSales != zero.MaxFundingSales ||
				sw.PriorHalfLife != zero.PriorHalfLife {
				t.Fatalf("sweepConfig sets a knob the literals left zero: %+v", sw)
			}

			// And no hindsight, which is the one field where a default sneaking in
			// would not merely change a diagnostic but invalidate the whole
			// record: every figure in AGENTS.md was measured with none. The
			// environment is unset here explicitly rather than assumed, because
			// this test would otherwise pass or fail according to the shell it
			// was run from.
			t.Setenv("FPL_ORACLE_AVAILABILITY", "")
			t.Setenv("FPL_ORACLE_PRICES", "")
			if o := sweepConfig(cfg, start, weekly).Oracles; o.Active() {
				t.Fatalf("sweepConfig grants %s with nothing asking for it", o.Stamp())
			}

			// And the two constructors differ in exactly one field. 2023-24
			// predates the five-transfer bank, so they must disagree here or the
			// comparison is vacuous.
			se := seasonConfig(cfg, "2023-24", start, weekly)
			if se.BankUpTo != BankLimitFor("2023-24") {
				t.Fatalf("seasonConfig must use BankLimitFor, got %d", se.BankUpTo)
			}
			if se.BankUpTo == sw.BankUpTo {
				t.Fatalf("the two configs agree on the bank (%d) for a season that "+
					"predates the rule change; one is not doing its job", sw.BankUpTo)
			}
			se.BankUpTo = sw.BankUpTo
			if !reflect.DeepEqual(se, sw) {
				t.Fatalf("the two configs differ in more than the bank:\n sweep  %+v\n season %+v",
					sw, se)
			}
		}
	}
}

// TestNilCellSinkIsANoOp keeps the env var checked in one place.
//
// Every call site is unconditional, so if a nil sink panicked, a sweep run
// without FPL_CELLS would die — which is the normal way to run it.
func TestNilCellSinkIsANoOp(t *testing.T) {
	sink, err := openCellSink("")
	if err != nil {
		t.Fatal(err)
	}
	if sink != nil {
		t.Fatal("an unset FPL_CELLS must produce no sink")
	}
	// None of these may panic or write anywhere.
	if got := sink.sweepLabel("T"); got != "" {
		t.Fatalf("want empty sweep label from a nil sink, got %q", got)
	}
	if got := sink.run(); got != "" {
		t.Fatalf("want empty run id from a nil sink, got %q", got)
	}
	sink.cell(sampleRow("T#1", "r", "shipped", 0, "2025-26", 1))
	sink.mean("T#1", "hold", "alt", "shipped", 1, 0.5, 24, Oracles{}.Stamp())
	sink.close()
}

// TestSweepStartsIsTheSixUnlessAskedOtherwise pins the opt-in.
//
// Two failures are guarded and both are silent. If the default ever becomes the
// dense grid, every figure in AGENTS.md becomes incomparable with every figure
// measured afterwards while both still print. And if a malformed FPL_SWEEP_STARTS
// fell back to the six — which is what the FPL_BENCH_SLOTS pattern does — a run
// believed to be dense would quietly measure the shipped grid, and a
// correlation-versus-spacing table computed from it would contain no short
// spacings at all.
//
// It lives in this file rather than beside TestDiagEntryDensity because it has to
// name the shipped grid to pin it, and TestTheGridIsDeclaredOnce forbids that
// everywhere except here and harness_test.go. A guard that cannot state the value it
// guards is no guard, and the two structural tests would otherwise fight.
func TestSweepStartsIsTheSixUnlessAskedOtherwise(t *testing.T) {
	shipped := []int{1, 6, 11, 16, 21, 26}

	// Unset and empty are the same request, and both mean the shipped grid.
	for _, unset := range []string{"", " "} {
		t.Setenv("FPL_SWEEP_STARTS", unset)
		if got := sweepStarts(); !reflect.DeepEqual(got, shipped) {
			t.Fatalf("FPL_SWEEP_STARTS=%q gave %v, want the shipped %v — every figure "+
				"in AGENTS.md is measured at the six, and changing the default makes "+
				"the record incomparable with itself", unset, got, shipped)
		}
	}

	t.Setenv("FPL_SWEEP_STARTS", " 4,2 , 1 ")
	if got, want := sweepStarts(), []int{1, 2, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("want whitespace tolerated and the grid sorted, %v; got %v", want, got)
	}

	// A grid that cannot be read must fail loudly rather than fall back.
	for _, bad := range []string{"1", "1,x,3", "0,5", "1,39", "1,1,6", "1,-2"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("FPL_SWEEP_STARTS=%q was accepted; a grid that cannot be "+
						"read must panic, because a run believed to be dense that "+
						"silently measures the six produces a complete-looking result "+
						"on a population containing no short spacing", bad)
				}
			}()
			t.Setenv("FPL_SWEEP_STARTS", bad)
			_ = sweepStarts()
		}()
	}
}

// TestSweepSeasonsIsTheSixUnlessAskedOtherwise pins the grid, for the same reason
// TestSweepStartsIsTheSixUnlessAskedOtherwise pins the entry points.
//
// **This test used to assert the opposite, and the change is deliberate.** It was
// TestSweepSeasonsIsTheFourUnlessAskedOtherwise, on the argument that every figure
// in the record is measured on the four and widening the default would make the
// record incomparable with itself at a stroke, silently, while both old and new
// figures still print.
//
// That argument was wrong on a point that turned out to be checkable. The four are
// a strict subset of the six, and the cells they produce inside a six-season run are
// byte-identical to an independently run four-season sweep — 48 of 48 overlapping
// cells on the positive control, 192 across all arms. Nothing is invalidated; the
// old figures are simply four-season figures. See gridwidth_test.go for the
// pre-registration this was decided against, and stats/snapshots/2026-08-11-6acc5ad
// for the cells.
//
// So the opt-in reverses rather than disappears: unset gives the six that now ship,
// and `default` names the historical four, which is how a recorded figure gets
// reproduced on the grid it was measured on. Both directions are pinned here,
// because the failure this guards against is symmetric — a run believed to be six
// seasons that measured four reports a threshold about a third too optimistic, and
// a run believed to be four that measured six is not comparable with the record it
// is about to be written into.
//
// The malformed case panics rather than falling back, for the same reason.
func TestSweepSeasonsIsTheSixUnlessAskedOtherwise(t *testing.T) {
	historicalFour := [][2]string{
		{"2021-22", "2022-23"}, {"2022-23", "2023-24"},
		{"2023-24", "2024-25"}, {"2024-25", "2025-26"},
	}

	// Pinned to a literal rather than to extendedPairNames(), which is the function
	// under test. Review caught the tautology: comparing the two meant editing
	// extendedPairNames to return four pairs — the exact regression this guards
	// against — left the test green.
	shippedSix := [][2]string{
		{"2019-20", "2020-21"}, {"2020-21", "2021-22"},
		{"2021-22", "2022-23"}, {"2022-23", "2023-24"},
		{"2023-24", "2024-25"}, {"2024-25", "2025-26"},
	}

	for _, v := range []string{"", "extended", "  "} {
		t.Setenv("FPL_SWEEP_SEASONS", v)
		got := sweepPairNames()
		want := shippedSix
		if len(got) != len(want) {
			t.Fatalf("FPL_SWEEP_SEASONS=%q gave %d pairs, want the shipped %d",
				v, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("FPL_SWEEP_SEASONS=%q pair %d = %v, want %v", v, i, got[i], want[i])
			}
		}
	}

	// The escape hatch back to the grid every recorded figure was measured on.
	t.Setenv("FPL_SWEEP_SEASONS", "default")
	got := sweepPairNames()
	if len(got) != len(historicalFour) {
		t.Fatalf("default gave %d pairs, want the historical %d", len(got), len(historicalFour))
	}

	// "scoring" was unpinned entirely until review noticed. It is HOLD-only, so a
	// silent change to it would put 2019-20's transfer path into a POLICY figure —
	// the one season whose wallet is not a sample of the same process.
	t.Setenv("FPL_SWEEP_SEASONS", "scoring")
	if seven := sweepPairNames(); len(seven) != 7 {
		t.Errorf("scoring gave %d pairs, want 7", len(seven))
	} else if seven[0] != [2]string{"2018-19", "2019-20"} {
		t.Errorf("scoring leads with %v, want the 2018-19 prior pair", seven[0])
	}
	for i := range historicalFour {
		if got[i] != historicalFour[i] {
			t.Errorf("default pair %d = %v, want %v", i, got[i], historicalFour[i])
		}
	}

	// Unrecognised must not read as success.
	t.Setenv("FPL_SWEEP_SEASONS", "six")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("an unrecognised FPL_SWEEP_SEASONS returned a grid instead of " +
					"panicking; a run believed to be six seasons would measure four and " +
					"report a threshold a third too optimistic")
			}
		}()
		_ = sweepPairNames()
	}()
}

// TestTheArmBlockIsBeforeTheOracleBlockAndCounted is the positional half of the
// schema contract for the columns added beside squad_hash.
//
// armCols exists for the same reason oracleCols does — the stale-header test
// synthesises older schemas by stripping trailing blocks — and it is only
// correct while the block really is where it says and really is that long. The
// oracle pair must stay last: TestOracleColumnsAreLastAndCounted asserts that
// from the other end, and this test is what stops a later column being dropped
// in *between* the two blocks, where neither test would see it.
func TestTheArmBlockIsBeforeTheOracleBlockAndCounted(t *testing.T) {
	want := []string{"setting", "min_expected_minutes", "squad_hash"}
	if armCols != len(want) {
		t.Fatalf("armCols is %d and the block is %d columns", armCols, len(want))
	}
	got := cellHeader[len(cellHeader)-oracleCols-armCols : len(cellHeader)-oracleCols]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the %d columns before the oracle pair are %v, want %v",
			armCols, got, want)
	}
}

// xPointsBlockAtIn is where the accumulated-xPoints columns start in a header:
// immediately before the arm block, which is immediately before the oracle pair.
//
// Parameterised by the header rather than reading cellHeader, because the
// stale-header chain removes an earlier block first and the offsets it needs are
// then relative to a shorter header. Anchored from the END, so it stays correct
// whatever is added in front of it.
func xPointsBlockAtIn(h []string) int {
	return len(h) - oracleCols - armCols - xPointsCols
}

func xPointsBlockAt() int { return xPointsBlockAtIn(cellHeader) }

// chipOracleBlockAtIn is where the chip-week oracle's columns start: immediately
// before the xPoints block, and immediately after the chip block.
//
// Parameterised by the header for the reason xPointsBlockAtIn is: the
// stale-header chain removes the banking block first, and the offsets it needs
// are then relative to a shorter header.
func chipOracleBlockAtIn(h []string) int { return xPointsBlockAtIn(h) - chipOracleCols }

func chipOracleBlockAt() int { return chipOracleBlockAtIn(cellHeader) }

// bankingBlockAtIn is where the transfer-banking columns start: immediately
// before the chip block, which is immediately before the chip-oracle block.
//
// Anchored from the END like every other block offset here, so it stays correct
// whatever is added in front of it.
func bankingBlockAtIn(h []string) int {
	return fixtureRunBlockAtIn(h) - bankingCols
}

func bankingBlockAt() int { return bankingBlockAtIn(cellHeader) }

// fixtureRunBlockAtIn is where the fixture-run columns start: immediately after
// the banking block and immediately before the chip block.
//
// Anchored from the END for the same reason every offset here is. Note that
// bankingBlockAtIn now chains through this rather than through chipWeekCols
// directly — the banking block stopped being the chip block's neighbour when this
// one went in between them, and an offset that still subtracted only chipWeekCols
// would have silently re-labelled banking columns as fixture-run ones.
func fixtureRunBlockAtIn(h []string) int {
	return optionBlockAtIn(h) - fixtureRunCols
}

// optionBlockAtIn is where the four option-value funnels start: immediately after
// the fixture-run block and immediately before the dose block.
//
// Anchored from the END like every other offset here, and chained THROUGH the
// dose block rather than subtracting chipWeekCols directly — the same correction
// fixtureRunBlockAtIn already carries in its own comment. An offset that skipped a
// neighbour would silently re-label one block's columns as another's, which is the
// failure every position assertion in this file exists to catch.
func optionBlockAtIn(h []string) int { return doseBlockAtIn(h) - optionCols }

func optionBlockAt() int { return optionBlockAtIn(cellHeader) }

// floorBlockAtIn is where the gate-floor counterfactual starts: immediately
// after the dose block and immediately before the chip block.
func floorBlockAtIn(h []string) int {
	return chipOracleBlockAtIn(h) - chipWeekCols - floorCols
}

// floorBlockAt is the floor block's offset in the shipped header.
func floorBlockAt() int { return floorBlockAtIn(cellHeader) }

// doseBlockAtIn is where the per-cell fixture dose starts: immediately after the
// option-value funnels and immediately before the floor block.
func doseBlockAtIn(h []string) int {
	return floorBlockAtIn(h) - doseCols
}

func doseBlockAt() int { return doseBlockAtIn(cellHeader) }

// withoutOptionBlock and withoutDoseBlock synthesise the two headers this build's
// predecessors wrote. The dose block is the NEWEST and the outermost of the pair,
// so the stale-header chain removes it first and the option block second.
func withoutOptionBlock() []string {
	return headerWithout(cellHeader, optionBlockAt(), optionCols)
}

func withoutDoseBlock() []string {
	return headerWithout(cellHeader, doseBlockAt(), doseCols)
}

func fixtureRunBlockAt() int { return fixtureRunBlockAtIn(cellHeader) }

// headerWithout removes n columns starting at at, which is how an older header
// is synthesised once a block has been added anywhere but the end.
func headerWithout(h []string, at, n int) []string {
	out := append([]string(nil), h[:at]...)
	return append(out, h[at+n:]...)
}

// withoutXPointsBlock is this build's header with the xPoints columns removed
// from the middle.
//
// It exists because the stale-header test's "strip the trailing block" trick
// stops working the moment a block is added anywhere but the end, and the oracle
// pair and the arm block both have to stay where they are. Synthesising the older
// header instead of truncating keeps that test's entries honest about which build
// they name, which is the one property it has twice failed to have.
// ⚠️ It is no longer the *predecessor* — the chip-oracle block went in after it,
// one position earlier in the schema — so the stale-header chain removes that
// one first. This function is still what the xPoints block's position assertion
// is written against.
func withoutXPointsBlock() []string {
	return headerWithout(cellHeader, xPointsBlockAt(), xPointsCols)
}

// withoutChipOracleBlock is this build's header with the chip-week oracle's
// columns removed from the middle.
//
// ⚠️ It is **no longer the predecessor** — the banking block went in after it and
// earlier in the schema — so the stale-header chain removes that one first. Same
// demotion the xPoints helper above already carries, and recorded for the same
// reason: this test's own comments say its entries have three times failed to
// name the build they describe, and an uncorrected "literally the predecessor"
// is how the fourth happens. This function is still what the chip-oracle block's
// position assertion is written against.
func withoutChipOracleBlock() []string {
	return headerWithout(cellHeader, chipOracleBlockAt(), chipOracleCols)
}

// withoutBankingBlock is this build's header with the five transfer-banking
// columns removed from the middle.
//
// ⚠️ It is **no longer the predecessor** — the fixture-run block went in after it
// and immediately after it in the schema — so the stale-header chain removes that
// one first. The same demotion the xPoints and chip-oracle helpers above already
// carry, recorded for the same reason: this test's own comments say its entries
// have three times failed to name the build they describe, and an uncorrected
// "literally the predecessor" is how the fourth happens. This function is still
// what the banking block's position assertion is written against.
func withoutBankingBlock() []string {
	return headerWithout(cellHeader, bankingBlockAt(), bankingCols)
}

// withoutFixtureRunBlock is this build's header with the five fixture-run columns
// removed from the middle — literally the header the predecessor build wrote,
// since the fixture-run block is the newest and it too went in mid-header.
func withoutFixtureRunBlock() []string {
	return headerWithout(cellHeader, fixtureRunBlockAt(), fixtureRunCols)
}

// TestTheXPointsBlockIsBeforeTheArmBlockAndCounted is the positional half of the
// schema contract for the accumulated-xPoints columns, on exactly the reasoning
// TestTheArmBlockIsBeforeTheOracleBlockAndCounted states for the block after it.
//
// A column dropped in between two counted blocks is invisible to a test that
// indexes from either end, so every block gets its own position assertion.
func TestTheXPointsBlockIsBeforeTheArmBlockAndCounted(t *testing.T) {
	want := []string{
		"hold_xpoints", "hold_xpoints_per_gw",
		"policy_xpoints", "policy_xpoints_per_gw",
	}
	if xPointsCols != len(want) {
		t.Fatalf("xPointsCols is %d and the block is %d columns", xPointsCols, len(want))
	}
	at := xPointsBlockAt()
	got := cellHeader[at : at+xPointsCols]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the %d columns before the arm block are %v, want %v",
			xPointsCols, got, want)
	}
	// And the synthesised predecessor really is this header minus exactly those
	// four, or the stale-header test's entries are mislabelled again.
	if n := len(withoutXPointsBlock()); n != len(cellHeader)-xPointsCols {
		t.Fatalf("withoutXPointsBlock has %d columns, want %d",
			n, len(cellHeader)-xPointsCols)
	}
	for _, c := range withoutXPointsBlock() {
		for _, w := range want {
			if c == w {
				t.Fatalf("withoutXPointsBlock still carries %q", c)
			}
		}
	}
}

// TestTheChipOracleBlockIsBeforeTheXPointsBlockAndCounted is this block's
// position assertion, on the reasoning the two blocks after it already state: a
// column dropped in between two counted blocks is invisible to a test that
// indexes from either end, so every block gets its own.
func TestTheChipOracleBlockIsBeforeTheXPointsBlockAndCounted(t *testing.T) {
	want := []string{
		"bench_boost_oracle_gw", "bench_boost_oracle_pts",
		"bench_boost_median_pts", "bench_boost_threshold_pts", "bench_boost_bar_pts",
		"triple_captain_oracle_gw", "triple_captain_oracle_pts",
		"triple_captain_median_pts", "triple_captain_threshold_pts",
		"triple_captain_bar_pts",
	}
	if chipOracleCols != len(want) {
		t.Fatalf("chipOracleCols is %d and the block is %d columns",
			chipOracleCols, len(want))
	}
	at := chipOracleBlockAt()
	got := cellHeader[at : at+chipOracleCols]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the %d columns before the xPoints block are %v, want %v",
			chipOracleCols, got, want)
	}
	// It sits immediately after the chip block, which is what makes "the chip
	// columns" one readable region of the file rather than two separated by an
	// unrelated instrument.
	if before := cellHeader[at-1]; before != "triple_captain_pts2" {
		t.Fatalf("the column before the chip-oracle block is %q, want the chip "+
			"block's last column", before)
	}
	// And the synthesised predecessor really is this header minus exactly those
	// ten, or the stale-header test's entries are mislabelled again — which is
	// the failure this file has now recorded three times. ("eight" stood here and
	// was simply wrong: the block above is ten columns and chipOracleCols is 10.)
	if n := len(withoutChipOracleBlock()); n != len(cellHeader)-chipOracleCols {
		t.Fatalf("withoutChipOracleBlock has %d columns, want %d",
			n, len(cellHeader)-chipOracleCols)
	}
	for _, c := range withoutChipOracleBlock() {
		for _, w := range want {
			if c == w {
				t.Fatalf("withoutChipOracleBlock still carries %q", c)
			}
		}
	}
}

// TestTheChipOracleReadingsAreBankedPerCell is why the chip-timing table can be
// given a standard error, and it fails if any of the readings stops being banked.
//
// The table those readings feed was printed and written nowhere: the six levels —
// oracle, median and threshold for each of the two scoring chips — were summed
// across the grid inside the diagnostic and reported as means, so the comparison
// had no dispersion and its conclusion had no detection threshold of its own.
// Banking the run would not have fixed it, because the schema had no columns to
// put them in. That is the specific failure this test exists to keep fixed, and
// it is why the assertion is **per cell**: an aggregate written to a file is the
// same six means in a new location.
//
// Two entry points, so a block that collapsed to one value per sweep — the shape
// the old table had — cannot pass.
func TestTheChipOracleReadingsAreBankedPerCell(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")

	// Every value distinguishable from every other, so a permutation between two
	// same-typed columns is catchable — the same reason sampleRow gives each
	// column a different number. A median on a half-integer as well, because the
	// series is integer gains over an even number of weeks and truncating it is
	// the one arithmetic error this block can make.
	want := map[int]map[string]string{}
	for i, start := range []int{1, 26} {
		r := sampleRow(sweep, sink.run(), "perfect chip week", 0, "2025-26", start)
		r.HasChipOracle = true
		r.BenchBoostOracleGW, r.BenchBoostOraclePts = 30+i, 41+i
		r.BenchBoostMedianPts, r.BenchBoostThresholdPts = 6.5+float64(i), 18+i
		r.BenchBoostBarPts = 22 + i
		r.TripleCapOracleGW, r.TripleCapOraclePts = 12+i, 27+i
		r.TripleCapMedianPts, r.TripleCapThresholdPts = 4.5+float64(i), 14+i
		r.TripleCapBarPts = 9 + i
		want[start] = map[string]string{
			"bench_boost_oracle_gw":        strconv.Itoa(r.BenchBoostOracleGW),
			"bench_boost_oracle_pts":       strconv.Itoa(r.BenchBoostOraclePts),
			"bench_boost_median_pts":       strconv.FormatFloat(r.BenchBoostMedianPts, 'g', -1, 64),
			"bench_boost_threshold_pts":    strconv.Itoa(r.BenchBoostThresholdPts),
			"bench_boost_bar_pts":          strconv.Itoa(r.BenchBoostBarPts),
			"triple_captain_oracle_gw":     strconv.Itoa(r.TripleCapOracleGW),
			"triple_captain_oracle_pts":    strconv.Itoa(r.TripleCapOraclePts),
			"triple_captain_median_pts":    strconv.FormatFloat(r.TripleCapMedianPts, 'g', -1, 64),
			"triple_captain_threshold_pts": strconv.Itoa(r.TripleCapThresholdPts),
			"triple_captain_bar_pts":       strconv.Itoa(r.TripleCapBarPts),
		}
		sink.cell(r)
	}
	// The un-oracled arm of the same sweep, which measures none of it.
	sink.cell(sampleRow(sweep, sink.run(), "real (ships)", 1, "2025-26", 1))
	sink.close()

	header, rows := readCells(t, path)
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	// The cell key has to travel with the readings or a reader cannot pair or
	// cluster them, which is how one past reader cross-paired arms against other
	// blocks' baselines.
	for _, k := range []string{"run_id", "sweep", "season", "start_gw"} {
		if !slices.Contains(header, k) {
			t.Fatalf("the header carries no %q column, so a reader cannot key "+
				"these readings to a cell", k)
		}
	}
	seen := map[int]bool{}
	for _, r := range rows {
		if r["variant"] == "real (ships)" {
			for c := range want[1] {
				if r[c] != "" {
					t.Errorf("%s must be blank on an arm that measured no chip "+
						"oracle, got %q — a zero here reads as a chip worth "+
						"nothing in its best week, which is a number", c, r[c])
				}
			}
			continue
		}
		start := atoiOrFail(t, r["start_gw"])
		seen[start] = true
		for c, w := range want[start] {
			if r[c] != w {
				t.Errorf("%s@%d: %s carries %q, want %q — cellHeader and "+
					"cellSink.cell have desynchronised", r["season"], start, c, r[c], w)
			}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("want a banked reading for each of 2 entry points, got %d — a "+
			"block that carries one value per sweep is the aggregate this "+
			"replaced, not a per-cell quantity", len(seen))
	}

	// An infeasible cell claims none of it: no fifteen was fielded, so there was
	// no season for an argmax to run over.
	measured := sampleRow(sweep, sink.run(), "perfect chip week", 0, "2025-26", 1)
	measured.HasChipOracle = true
	measured.BenchBoostOraclePts, measured.BenchBoostMedianPts = 41, 6.5
	inf := measured.asInfeasible()
	if inf.HasChipOracle || inf.BenchBoostOraclePts != 0 || inf.BenchBoostMedianPts != 0 {
		t.Fatal("an infeasible cell must not claim a chip-oracle reading")
	}
}

// TestTheChipOracleReadingsHaveOneImplementation pins the derivation to
// chipReadingsOf, which both the banked columns and the printed table come from.
//
// The failure it guards is the one that produced the item: the readings were
// computed inside the diagnostic that prints them, so nothing else could have
// them. Recomputing them at the sink — or in a second diagnostic — would put this
// project's signature bug back, with the added property that the printed table
// and the file would disagree silently.
//
// It checks arithmetic rather than source, because there is a single entry point
// to check: the median must not be truncated, and the threshold rule must fall
// back to the last week rather than to zero.
func TestTheChipOracleReadingsHaveOneImplementation(t *testing.T) {
	// Four weeks, so the bench-boost median lands on a half-integer. The
	// triple-captain series never clears its bar, which is the fallback case.
	res := &SimResult{
		Weeks: []Week{
			{GW: 5, BenchBoostGain: 2, TripleCaptainGain: 1},
			{GW: 6, BenchBoostGain: 30, TripleCaptainGain: 3},
			{GW: 7, BenchBoostGain: 5, TripleCaptainGain: 2},
			{GW: 8, BenchBoostGain: 9, TripleCaptainGain: 4},
		},
		ChipOracle: &ChipOracle{
			BenchBoost:    ChipWeek{GW: 6, Gain: 30},
			TripleCaptain: ChipWeek{GW: 8, Gain: 4},
		},
	}
	got, ok := chipReadingsOf(res)
	if !ok {
		t.Fatal("a cell with a placement must carry the block")
	}
	// (2, 5, 9, 30) sorted: the median is (5+9)/2. An int here would report 7.
	if got.BenchBoostMedianPts != 7 {
		t.Errorf("bench-boost median is %v, want 7", got.BenchBoostMedianPts)
	}
	if got.TripleCapMedianPts != 2.5 {
		t.Errorf("triple-captain median is %v, want 2.5 — truncating each cell "+
			"biases the pooled reading down by up to half a point",
			got.TripleCapMedianPts)
	}
	// The first week clearing 16, which is GW6's 30.
	if got.BenchBoostThresholdPts != 30 {
		t.Errorf("bench-boost threshold is %d, want the first week clearing the bar",
			got.BenchBoostThresholdPts)
	}
	// Nothing clears 12, so the chip is spent in the final week for 4. Zero would
	// make the threshold rule look worse than it is in exactly the seasons where
	// it is hardest.
	if got.TripleCapThresholdPts != 4 {
		t.Errorf("triple-captain threshold is %d, want the last week's gain as the "+
			"fallback", got.TripleCapThresholdPts)
	}
	if got.BenchBoostOracleGW != 6 || got.TripleCapOracleGW != 8 {
		t.Errorf("the chosen weeks are %d and %d, want 6 and 8",
			got.BenchBoostOracleGW, got.TripleCapOracleGW)
	}
	// The bars travel with the readings, and they are the bars the readings were
	// actually taken against rather than a declared copy — the same rule the arm
	// block keeps. Without them `*_threshold_pts` is a mixture of "a week cleared"
	// and "the season ran out" with no way to tell which cell is which.
	if got.BenchBoostBarPts != chipBarBenchBoost || got.TripleCapBarPts != chipBarTripleCaptain {
		t.Errorf("the banked bars are %d and %d, want the constants the threshold "+
			"rule ran against (%d and %d)",
			got.BenchBoostBarPts, got.TripleCapBarPts,
			chipBarBenchBoost, chipBarTripleCaptain)
	}
	// And the partition the bar exists to make recoverable: bench boost cleared,
	// triple captain did not.
	if !(got.BenchBoostThresholdPts >= got.BenchBoostBarPts) {
		t.Error("the bench-boost cell cleared its bar and the banked columns do not say so")
	}
	if got.TripleCapThresholdPts >= got.TripleCapBarPts {
		t.Error("the triple-captain cell never cleared its bar and the banked " +
			"columns read as though it did")
	}

	// And no placement means no block, rather than a zeroed one that reads as an
	// oracle that ran and found nothing.
	if _, ok := chipReadingsOf(&SimResult{Weeks: res.Weeks}); ok {
		t.Error("a cell with no placement must not claim a chip-oracle reading")
	}
}

// TestTheArmColumnsSayWhatRanRatherThanWhatWasLabelled pins the three columns'
// distinct blank rules, which are the whole reason they can be trusted.
//
// A gap and a zero mean different things in every other block of this schema and
// they mean different things here too: no declared setting is a fact about the
// family that stats/schedule_screen.R must be *told*, where `setting=0` is a real
// rung on several ladders here. The floor is never blank, because every cell has
// one.
func TestTheArmColumnsSayWhatRanRatherThanWhatWasLabelled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")

	ladder := sampleRow(sweep, sink.run(), "k=24", 1, "2025-26", 1)
	ladder.HasSetting, ladder.Setting = true, 24
	ladder.MinExpMinutes, ladder.SquadHash = 55, squadHash([]int{3, 1, 2})
	sink.cell(ladder)

	// An arm that varies no single scalar, which is most of them.
	onoff := sampleRow(sweep, sink.run(), "team form on", 2, "2025-26", 1)
	onoff.MinExpMinutes, onoff.SquadHash = 55, squadHash([]int{9})
	sink.cell(onoff)

	// A zero setting is a rung, not an absence.
	zero := sampleRow(sweep, sink.run(), "k=0", 3, "2025-26", 1)
	zero.HasSetting, zero.Setting = true, 0
	zero.MinExpMinutes, zero.SquadHash = 55, squadHash([]int{4})
	sink.cell(zero)

	// Infeasible: the arm columns survive, the measurement does not.
	dead := sampleRow(sweep, sink.run(), "floor=89", 4, "2025-26", 1)
	dead.HasSetting, dead.Setting = true, 89
	dead.MinExpMinutes, dead.SquadHash = 89, squadHash([]int{7})
	sink.cell(dead.asInfeasible())
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 4 {
		t.Fatalf("want 4 rows, got %d", len(rows))
	}
	byVariant := map[string]map[string]string{}
	for _, r := range rows {
		byVariant[r["variant"]] = r
	}
	for _, c := range []struct{ variant, setting, floor string }{
		{"k=24", "24", "55"},
		{"team form on", "", "55"},
		{"k=0", "0", "55"},
		{"floor=89", "89", "89"},
	} {
		r := byVariant[c.variant]
		if r["setting"] != c.setting {
			t.Errorf("%s: setting is %q, want %q", c.variant, r["setting"], c.setting)
		}
		if r["min_expected_minutes"] != c.floor {
			t.Errorf("%s: min_expected_minutes is %q, want %q",
				c.variant, r["min_expected_minutes"], c.floor)
		}
	}
	if h := byVariant["k=24"]["squad_hash"]; h == "" {
		t.Error("a feasible cell must carry its opening-fifteen hash")
	}
	// The mediator column is a measurement, so an infeasible cell has a gap
	// where a hash of nothing would be a value.
	if h := byVariant["floor=89"]["squad_hash"]; h != "" {
		t.Errorf("an infeasible cell built no fifteen; squad_hash is %q, want blank", h)
	}
}

// TestTheSquadHashIsSetIdentity is why the hash may be compared across cells.
//
// The optimiser's output order was once not run-to-run stable at all
// (TestSeedOrderIsDeterministic), so a hash that moved with it would report a
// squad change on about one landscape in seventy-two, and the mediator column
// would be worse than no column.
func TestTheSquadHashIsSetIdentity(t *testing.T) {
	fifteen := []int{101, 7, 42, 9, 300, 12, 88, 5, 61, 2, 77, 30, 14, 55, 23}
	shuffled := []int{23, 55, 14, 30, 77, 2, 61, 5, 88, 12, 300, 9, 42, 7, 101}
	if a, b := squadHash(fifteen), squadHash(shuffled); a != b {
		t.Errorf("the same fifteen in two orders hashes to %s and %s", a, b)
	}
	// One man swapped must move it, which is the case the column exists for:
	// HOLD cannot see a fifteenth man who is never fielded.
	swapped := append([]int(nil), fifteen...)
	swapped[14] = 999
	if squadHash(fifteen) == squadHash(swapped) {
		t.Error("swapping one player left the hash unchanged")
	}
	if squadHash(nil) != "" {
		t.Error("no squad must hash to a gap, not to a value")
	}
}

// TestTheChipWritersRecordBothPlaysOfATwoSetCell is the regression test for the
// bug fixed alongside it: a two-set season (2025-26 onward, and any ChipSets:2
// sweep arm) plays bench boost and triple captain twice, once per set, and the
// writer used to track a single value OVERWRITTEN on every play — so a cell's
// bench_boost_gw/pts and triple_captain_gw/pts recorded only the LAST play and
// silently dropped the first set's points. Confirmed damage exists in
// stats/cells/2026-08-25-tworegime/{legacy,measured-xgc}.csv, which are left as
// historical artifacts under the old schema — see their own README caveat
// rather than this test, which exists to stop it happening again.
//
// It exercises the actual writer code path (populateChipWeekColumns, called
// from runPolicySweep's own row-building loop) rather than a hand-rolled copy
// of the collection logic, and it writes through the real cellSink so the CSV
// round trip — including the four new columns' position in cellHeader — is
// covered too.
func TestTheChipWritersRecordBothPlaysOfATwoSetCell(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")

	// A two-set cell: bench boost at GW3 and GW30, triple captain at GW5 and
	// GW32. GWs and gains all differ from each other so a mixed-up column
	// or a mixed-up chip reads back wrong rather than agreeing by accident.
	twoSet := sampleRow(sweep, sink.run(), "two-set", 0, "2025-26", 1)
	twoSet.HasChipWeeks = true
	populateChipWeekColumns(t, "two-set@1", []Week{
		{GW: 3, BenchBoost: true, BenchBoostGain: 11},
		{GW: 5, TripleCaptain: true, TripleCaptainGain: 22},
		{GW: 30, BenchBoost: true, BenchBoostGain: 33},
		{GW: 32, TripleCaptain: true, TripleCaptainGain: 44},
	}, &twoSet)
	sink.cell(twoSet)

	// A one-play cell: only the first set's bench boost fires and the triple
	// captain never plays at all.
	oneSet := sampleRow(sweep, sink.run(), "one-set", 1, "2025-26", 1)
	oneSet.HasChipWeeks = true
	populateChipWeekColumns(t, "one-set@1", []Week{
		{GW: 7, BenchBoost: true, BenchBoostGain: 55},
	}, &oneSet)
	sink.cell(oneSet)
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	byVariant := map[string]map[string]string{}
	for _, r := range rows {
		byVariant[r["variant"]] = r
	}

	// (a) Both plays are recoverable, with the FIRST chronological play in the
	// _gw/_pts columns and the second in _gw2/_pts2.
	two := byVariant["two-set"]
	want := map[string]string{
		"bench_boost_gw": "3", "bench_boost_pts": "11",
		"bench_boost_gw2": "30", "bench_boost_pts2": "33",
		"triple_captain_gw": "5", "triple_captain_pts": "22",
		"triple_captain_gw2": "32", "triple_captain_pts2": "44",
	}
	for col, w := range want {
		if got := two[col]; got != w {
			t.Errorf("two-set cell column %q = %q, want %q", col, got, w)
		}
	}

	// (b) A one-play cell leaves _gw2/_pts2 at zero.
	one := byVariant["one-set"]
	wantOne := map[string]string{
		"bench_boost_gw": "7", "bench_boost_pts": "55",
		"bench_boost_gw2": "0", "bench_boost_pts2": "0",
		"triple_captain_gw": "0", "triple_captain_pts": "0",
		"triple_captain_gw2": "0", "triple_captain_pts2": "0",
	}
	for col, w := range wantOne {
		if got := one[col]; got != w {
			t.Errorf("one-set cell column %q = %q, want %q", col, got, w)
		}
	}
}

// TestAThirdChipPlayFatals is (c): a three-play cell must fatal rather than
// silently drop a play, which is what a schema recording only two plays would
// otherwise do — the same silent-drop failure mode this whole fix exists to
// close, one play further along. It targets the collection logic directly
// (populateChipWeekColumns) rather than the full sweep, since no rule this
// codebase ships can construct a three-play SimResult and the point is to pin
// what the writer does if one ever existed — a bug in a future chip-set rule,
// say — rather than to reproduce one under today's rules.
func TestAThirdChipPlayFatals(t *testing.T) {
	t.Run("bench boost", func(t *testing.T) {
		ct := &collectingT{}
		row := cellRow{}
		populateChipWeekColumns(ct, "three-play@1", []Week{
			{GW: 3, BenchBoost: true, BenchBoostGain: 1},
			{GW: 10, BenchBoost: true, BenchBoostGain: 2},
			{GW: 20, BenchBoost: true, BenchBoostGain: 3},
		}, &row)
		if !ct.fataled {
			t.Fatal("a third bench-boost play must fatal, and did not")
		}
	})
	t.Run("triple captain", func(t *testing.T) {
		ct := &collectingT{}
		row := cellRow{}
		populateChipWeekColumns(ct, "three-play@1", []Week{
			{GW: 3, TripleCaptain: true, TripleCaptainGain: 1},
			{GW: 10, TripleCaptain: true, TripleCaptainGain: 2},
			{GW: 20, TripleCaptain: true, TripleCaptainGain: 3},
		}, &row)
		if !ct.fataled {
			t.Fatal("a third triple-captain play must fatal, and did not")
		}
	})
}

// TestBenchBoostGWAgreesWithTheTriggerMediator is (d): bench_boost_gw must
// equal bb_trig_gw where both fire, pinning the chip block's "first play"
// convention against ChipTriggerMediator.FiredGW's own "first firing"
// convention — see that field's doc comment for why "first" is load-bearing
// once chips come in two sets. A cell's bench-boost trigger mediator and its
// chip-week block are populated from the same res.Weeks by two different
// pieces of code (runPolicySweep's row-building loop and Simulate's trigger
// bookkeeping), so the two columns agreeing is not guaranteed by the type
// system — only by both having adopted "first" rather than "last".
func TestBenchBoostGWAgreesWithTheTriggerMediator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sweep := sink.sweepLabel("T")

	row := sampleRow(sweep, sink.run(), "two-set", 0, "2025-26", 1)
	row.HasChipWeeks = true
	weeks := []Week{
		{GW: 3, BenchBoost: true, BenchBoostGain: 11},
		{GW: 30, BenchBoost: true, BenchBoostGain: 33},
	}
	populateChipWeekColumns(t, "two-set@1", weeks, &row)

	// The trigger mediator, built the same way ChipTriggerMediator's own doc
	// comment describes: FiredGW set on the FIRST firing only, later firings
	// appended to FiredGWs but not overwriting the scalar field.
	row.HasBanking = true
	row.BenchBoostTrig = ChipTriggerMediator{
		FiredGW: weeks[0].GW, FiredGWs: []int{weeks[0].GW, weeks[1].GW},
	}
	sink.cell(row)
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got["bench_boost_gw"] != got["bb_trig_gw"] {
		t.Fatalf("bench_boost_gw %q disagrees with bb_trig_gw %q — the chip "+
			"block and the trigger mediator no longer share the same "+
			"first-play convention", got["bench_boost_gw"], got["bb_trig_gw"])
	}
	if got["bench_boost_gw"] != "3" {
		t.Fatalf("bench_boost_gw is %q, want the FIRST play's gameweek (3)",
			got["bench_boost_gw"])
	}
}

// collectingT satisfies fataler by RECORDING a Fatalf call rather than
// aborting the goroutine the way a real *testing.T does — the point of
// TestAThirdChipPlayFatals is to observe that the guard fired, which a real
// Fatalf's runtime.Goexit would make impossible to inspect afterwards.
// populateChipWeekColumns keeps running its own code after the recorded
// call — a real Fatalf never would — so the two calls in
// TestAThirdChipPlayFatals only read `fataled`, never a field the function
// might still overwrite past the third-play check.
type collectingT struct {
	fataled bool
}

func (c *collectingT) Helper() {}

func (c *collectingT) Fatalf(format string, args ...any) {
	c.fataled = true
}
