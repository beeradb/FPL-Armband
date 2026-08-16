package backtest

// What is scoring-chip *timing* worth?
//
//	DIAG=1 EXP=ORACLECHIP FPL_CELLS=/tmp/oraclechip/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagChipWeekOracle$' -v -timeout 3h
//
// # What changed, and why the old number should not be quoted
//
// This was TestDiagChipCeiling: a bespoke loop over four seasons at one entry
// point, with the argmax written inline and no flag, no stamp and no invariance.
// The oracle-design document calls it the cautionary case among the three oracles that
// existed before the shared harness — a quarter of the 24 cells this project
// treats as the floor for a verdict, and a "do not cite these numbers" caveat
// carried in the research record rather than a fix.
//
// It is now an arm of the standard sweep under AxisChipWeek. Same argmax, same
// per-week gains, six entry points instead of one, a stamp in every cell it
// writes, and an invariance the harness checks rather than an operator.
//
// # This oracle changes nothing, which is the point of building it first
//
// Week.BenchBoostGain and Week.TripleCaptainGain are recorded every gameweek
// against the *unchipped* week, and no chip is ever played. So the oracle reads a
// slice that already exists, and AxisChipWeek declares that **every** collected
// metric must be byte-identical to the un-oracled baseline: POLICY, all three held
// rungs, transfer count and hits. That makes this the sharpest available test of
// the harness itself — any movement anywhere means an argmax over a finished
// season has somehow reached a decision.
//
// # Three readings per chip, and only the first is an oracle
//
//   - ORACLE: the best gameweek in hindsight. Unreachable, and the ceiling.
//   - MEDIAN: a gameweek picked at random, which is what a badly-timed chip is
//     worth.
//   - THRESHOLD: the first week clearing a bar, which is the shape any honest
//     policy must take — you cannot see the rest of the season, so you take the
//     first good one and accept it may not be the best.
//
// **Timing is ORACLE minus THRESHOLD.** ORACLE minus MEDIAN is the value of
// playing the chip at all rather than wasting it, and conflating the two is how a
// chip-timing policy gets justified by a number that measures something else.
// ⚠️ This said "a different and larger quantity" and the second half is not
// forced: `oracle − median` exceeds `oracle − threshold` exactly when the
// threshold reading exceeds the median, and firstClearing falls back to the final
// week when nothing clears the bar — which can land below it. Different question,
// ordering unmeasured.
//
// # All three readings are banked per cell
//
// They were printed and never written to the cells file. ⚠️ **Not "printed as an
// aggregate"** — the grid below has always carried one line per cell, and this
// repository banks sweep stdout as well as cells, so the dispersion was not
// unrecorded in principle. What was missing is that nothing in `stats/` reads
// stdout, and this sweep's own banked cells —
// stats/snapshots/2026-08-12-4d61058/cells/oraclechip.csv, 24 cells over four
// seasons — carry none of the readings, because the schema had no columns for
// them. So both differences below were quotable only as means over the grid,
// which is why the conclusion drawn from them has no detection threshold of its
// own.
//
// runPolicySweep now writes the per-chip oracle, median and threshold onto every
// cell row it emits, through chipReadingsOf — the one derivation — so each is a
// per-cell quantity keyed by `(run_id, sweep, season, start_gw)` and read one
// `variant` at a time, like every other column. The bars the threshold rule uses
// are banked beside them rather than left to the build; see chipBarBenchBoost.
//
// ⚠️ **Nothing has been re-measured.** The columns make a standard error
// obtainable and do not supply one: no sweep has run under this schema, no
// banked file carries the block, and stats/sweep_inference.R does not read these
// columns. Forming either difference is a run plus a reader away.
//
// # What it does not bound
//
// Chip *preparation*. Bench boost pays all fifteen and this measures it on the
// squad the ordinary objective builds — one that credits the bench at almost
// nothing and converges on eleven good players and four who cannot cover. No
// argmax over these gains can see that gap; it needs a squad built for the chip,
// which is what OptimizeRequest.BenchBoost and the wildcard exist for.
// See the chips note.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/stats"
)

// chipBar is the gain a threshold policy waits for, per chip.
//
// Roughly twice a typical week — the level a manager would recognise as "worth
// spending the chip on". Asserted, not measured: it stands in for a policy nobody
// has built, and the diagnostic's output is oracle *minus* this, so a bar set too
// low flatters chip timing and a bar set too high flatters the threshold rule.
// Both bars are held fixed across every cell so the comparison is between entry
// points rather than between bars.
const (
	chipBarBenchBoost    = 16
	chipBarTripleCaptain = 12
)

// chipReadingsOf derives the chip-week oracle's three readings of each chip from
// a season that has already been played, and is the ONE place they are computed.
//
// runPolicySweep calls it on every cell, so the readings are banked, and this
// diagnostic calls it to build the table it prints — so the printed table and the
// CSV cannot disagree about a level. That is the point: the table used to be
// computed here and written nowhere, which is how six means came to be quoted
// with no dispersion behind them.
//
// The bool is false unless AxisChipWeek granted the cell a placement.
// SimResult.ChipOracle is nil otherwise, and a zero-valued block would read as
// "the oracle ran and found nothing worth playing".
//
// The median and the threshold are deliberately NOT computed on the engine side:
// ChipWeek's doc comment says why — they are ordinary readings of
// Week.BenchBoostGain rather than oracles, and computing them beside the argmax
// would put a baseline inside the hindsight arm where a later reader could quote
// it as one.
//
// ⚠️ That separation is weaker here than it is there, and the difference is worth
// stating: the banked block puts all three on the *oracled arm's own row*. What
// keeps them apart in the file is the column names — only `*_oracle_*` is
// hindsight, and `*_median_pts` and `*_threshold_pts` are ordinary readings of
// the same played season that must never be quoted as oracle output.
//
// # ⚠️ The two differences these support are BOUNDED BELOW BY ZERO
//
// `*_oracle_pts` is `max(gains)` — `bestChipWeek` is an argmax over exactly the
// slice rebuilt here. `firstClearing` returns an element of that same slice, and
// `stats.Median` of it cannot exceed its maximum. So `oracle − threshold` and
// `oracle − median` are **≥ 0 in every cell, always, by construction**.
// reportChipCells errors if `oracle < median`, which is HALF that fact written as
// a guard; the threshold half is unguarded and rests on `firstClearing` returning
// an element of the same slice.
//
// A clustered standard error on either is the right instrument — for **an interval
// on a bound**. It is NOT a test against zero: `lm(diff ~ 1)` would return a t
// against a null the arithmetic already refuted, and one not commensurable with any
// other t in this record. That is the status AGENTS.md assigns the perfect armband,
// whose "t of 20.4 is mechanical".
//
// ⚠️ The sign is guaranteed; the MAGNITUDE is not, and reading a small |t| here as
// evidence about the bound is the trap. A difference that is exactly 0 in several
// cells — the threshold rule catching the best week — caps the clustered |t| by
// construction, the same degeneracy AGENTS.md records for the minutes floor
// ("with 2 of 6 seasons non-zero the clustered |t| is capped at 1.58"). Zero in
// every cell and `cells_common.R`'s `degenerate` refuses it outright.
//
// So quote these as "perfect timing is worth at most X **over the threshold rule**"
// — naming the comparator, because `oracle − median` bounds it over an UNTIMED chip
// instead — and per start point, since the window a cell played sets how many weeks
// the maximum ranges over. stats/README.md carries the reader's half of this.
func chipReadingsOf(res *SimResult) (chipReadings, bool) {
	if res == nil || res.ChipOracle == nil {
		return chipReadings{}, false
	}
	bb := make([]int, 0, len(res.Weeks))
	tc := make([]int, 0, len(res.Weeks))
	for _, w := range res.Weeks {
		bb = append(bb, w.BenchBoostGain)
		tc = append(tc, w.TripleCaptainGain)
	}
	return chipReadings{
		BenchBoostOracleGW:     res.ChipOracle.BenchBoost.GW,
		BenchBoostOraclePts:    res.ChipOracle.BenchBoost.Gain,
		BenchBoostMedianPts:    stats.Median(bb),
		BenchBoostThresholdPts: firstClearing(bb, chipBarBenchBoost),
		BenchBoostBarPts:       chipBarBenchBoost,
		TripleCapOracleGW:      res.ChipOracle.TripleCaptain.GW,
		TripleCapOraclePts:     res.ChipOracle.TripleCaptain.Gain,
		TripleCapMedianPts:     stats.Median(tc),
		TripleCapThresholdPts:  firstClearing(tc, chipBarTripleCaptain),
		TripleCapBarPts:        chipBarTripleCaptain,
	}, true
}

// chipCell is one (season, entry point) reading of both chips, for the printed
// table. The readings themselves are the banked struct, not a copy of it.
type chipCell struct {
	season string
	start  int
	weeks  int
	chipReadings
}

func TestDiagChipWeekOracle(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== the chip-week oracle, full grid.\n")
	fmt.Printf("The oracled arm must reproduce the baseline in EVERY collected\n")
	fmt.Printf("metric — POLICY, all three held rungs, moves and hits — because it\n")
	fmt.Printf("reads per-week gains the replay already recorded and plays no chip.\n")
	fmt.Printf("Its output is the table below, not a paired difference.\n")

	// The table is printed from the same readings the sweep banks, taken from the
	// same function. A second derivation here is how the printed levels and the
	// file's levels get to disagree, and it is what made this table
	// unreproducible from anything committed.
	var cells []chipCell
	collect := func(pair seasonPair, start int, res *SimResult) {
		got, ok := chipReadingsOf(res)
		if !ok {
			t.Errorf("%s@%d ran under %s and came back with no chip placement — "+
				"the axis is stamped and inert, which reports as a clean null",
				pair.Name, start, (Oracles{Decision: AxisChipWeek}).Stamp())
			return
		}
		cells = append(cells, chipCell{
			season: pair.Name, start: start, weeks: len(res.Weeks),
			chipReadings: got,
		})
	}

	oracle := oracleVariant(Oracles{Decision: AxisChipWeek}, "perfect chip week", nil)
	oracle.observe = collect
	runPolicySweep(t, []policyVariant{
		{label: "real (ships)", apply: func(sc *SimConfig) {}},
		oracle,
	}, starts)

	reportChipCells(t, cells)
}

// firstClearing is the threshold rule: the first week whose gain clears the bar,
// falling back to the last week if none ever does.
//
// The fallback is the rule and not a convenience. A chip unplayed at the final
// whistle is worth exactly nothing, so a policy that never sees its bar must
// still spend it, and crediting zero there would make the threshold rule look
// worse than it is by exactly the seasons where it is hardest.
func firstClearing(gains []int, bar int) int {
	for _, g := range gains {
		if g >= bar {
			return g
		}
	}
	if len(gains) == 0 {
		return 0
	}
	return gains[len(gains)-1]
}

// reportChipCells prints the grid and the two differences that matter.
//
// Every column it prints is a banked column, so the table is a rendering of the
// cells file rather than a second calculation of it. **It is not the place to
// give either difference a standard error** — that is the inference layer's, off
// the file, on the per-cell values this now writes.
func reportChipCells(t *testing.T, cells []chipCell) {
	t.Helper()
	if len(cells) == 0 {
		t.Fatal("the oracled arm observed no cells, so nothing below was measured")
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].start != cells[j].start {
			return cells[i].start < cells[j].start
		}
		return cells[i].season < cells[j].season
	})

	fmt.Printf("\n%-9s %6s %6s | %6s %6s %6s %5s | %6s %6s %6s %5s\n",
		"season", "start", "weeks",
		"BB orc", "BB med", "BB thr", "gw", "TC orc", "TC med", "TC thr", "gw")
	var bo, bm, bt, to, tm, tt float64
	// The mediator: an argmax that always picked the same week would not be
	// selecting, and the figure would be a fixed-week policy wearing an oracle's
	// label.
	weeksChosen := map[int]bool{}
	for _, c := range cells {
		fmt.Printf("%-9s %6d %6d | %6d %6.1f %6d %5d | %6d %6.1f %6d %5d\n",
			c.season, c.start, c.weeks,
			c.BenchBoostOraclePts, c.BenchBoostMedianPts, c.BenchBoostThresholdPts,
			c.BenchBoostOracleGW,
			c.TripleCapOraclePts, c.TripleCapMedianPts, c.TripleCapThresholdPts,
			c.TripleCapOracleGW)
		bo += float64(c.BenchBoostOraclePts)
		bm += c.BenchBoostMedianPts
		bt += float64(c.BenchBoostThresholdPts)
		to += float64(c.TripleCapOraclePts)
		tm += c.TripleCapMedianPts
		tt += float64(c.TripleCapThresholdPts)
		weeksChosen[c.BenchBoostOracleGW] = true
		if float64(c.BenchBoostOraclePts) < c.BenchBoostMedianPts ||
			float64(c.TripleCapOraclePts) < c.TripleCapMedianPts {
			t.Errorf("%s@%d: the oracle week is worse than the median week "+
				"(%d<%.1f, %d<%.1f) — the argmax is not an argmax",
				c.season, c.start, c.BenchBoostOraclePts, c.BenchBoostMedianPts,
				c.TripleCapOraclePts, c.TripleCapMedianPts)
		}
	}
	n := float64(len(cells))
	fmt.Printf("%-9s %6s %6s | %6.1f %6.1f %6.1f %5s | %6.1f %6.1f %6.1f %5s\n",
		"mean", "", "", bo/n, bm/n, bt/n, "", to/n, tm/n, tt/n, "")

	fmt.Printf("\nTIMING is oracle minus threshold: bench boost %+.1f, triple captain %+.1f.\n",
		(bo-bt)/n, (to-tt)/n)
	fmt.Printf("PLAYING IT AT ALL is oracle minus median: %+.1f and %+.1f. These are\n",
		(bo-bm)/n, (to-tm)/n)
	fmt.Printf("different questions and only the first is what a timing policy buys.\n")
	fmt.Printf("Both are points for ONE gameweek in a season, so they are already at\n")
	fmt.Printf("season scale: do not divide them by gameweeks played, do not multiply\n")
	fmt.Printf("by 38, and do not read them as a paired difference against the\n")
	fmt.Printf("baseline arm.\n")
	// ⚠️ This used to end "so compare them against this harness's season-scale
	// detection threshold directly", which is the borrowing the record retracted:
	// a threshold belongs to a comparison, and these two have none of their own.
	fmt.Printf("Both are means over %d cells and neither carries its dispersion. The\n", len(cells))
	fmt.Printf("per-cell readings behind them are banked in the cells file under\n")
	fmt.Printf("bench_boost_oracle_pts and its siblings, which is where a standard\n")
	fmt.Printf("error for either difference has to come from — a mean printed here has\n")
	fmt.Printf("no threshold of its own and must not borrow one.\n")
	fmt.Printf("\nNeither bounds chip PREPARATION: bench boost is measured on a squad\n")
	fmt.Printf("built by an objective that credits the bench at almost nothing.\n")

	if len(weeksChosen) < 2 {
		t.Errorf("the bench-boost oracle chose the same gameweek in all %d cells, "+
			"so it is a fixed-week policy under an oracle label rather than an "+
			"argmax", len(cells))
	}
}
