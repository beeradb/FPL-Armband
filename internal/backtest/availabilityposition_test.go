// Is the minutes over-prediction that `blankRunFactor` divides out really
// "position-wide"?
//
//	FPL_NO_BLANK_RUN=1 FPL_BLANKRUN_ROWS=/tmp/blankrun.csv DIAG=1 \
//	  go test ./internal/backtest -run TestDiagAvailabilityByPosition -v -count=1 -timeout 60m
//	Rscript stats/blank_run_position.R /tmp/blankrun.csv
//
// # The claim under test
//
// `analysis.blankRunFactor`'s comment derives the shipped 0.75 by taking each
// trailing-blank row's actual/expected ratio **relative to the run-0 row**,
// which it justifies as stripping out "the model's general, and harmlessly
// position-wide, tendency to over-predict minutes". On the table recorded at
// that site the run-0 ratio is ~0.935, so the plateau reads 0.752/0.737/0.765
// de-levelled and 0.704/0.689/0.716 raw — a 7% move in a live scoring constant.
// ⚠️ That table is pre-doubles-fix. At today's data state the divisor is 0.960
// on six seasons and 0.968 on four, so the de-levelling is worth **3-4%**. See
// the dated note in `analysis.blankRunFactor`: the size changed, the question
// did not.
//
// The word doing the work is **position-wide**. AGENTS.md's exemption is "a
// bias shared by every player in a position is not an ordering error; a
// within-position bias is" — so a bias that is genuinely common to all four
// positions is a level the optimiser cannot act on, and removing it from a
// calibration is free. A bias that differs by position is not.
//
// **The table that comment rests on carries no position split.** It was pooled
// over GKP/DEF/MID/FWD and the claim was never measured. This re-runs the same
// calibration — the same population filters, the same window, the same
// estimator — cut by position, so the claim can be checked rather than asserted.
//
// # Two things that would otherwise make this measure the wrong thing
//
// **`ExpectedMinutes` now carries `blankRunFactor` itself.** `Metrics` sets it
// from `blendRates`, which applies the discount, so re-running the calibration
// at shipped config measures the *residual after* the term rather than the
// signal the term was fitted to. `analysis` reads FPL_NO_BLANK_RUN once at
// package init, so the switch cannot be set from inside a test — this refuses
// to run without it rather than quietly reporting a circular fit.
//
// **The calibration population is the unflagged one.** The recorded table says
// so in its header ("Restricted to players FPL had *not* flagged"), and the
// flagged group is where availabilityFactor already has the channel. Both are
// printed here; only the unflagged one bears on the claim.
//
// # Why Go prints ratios and not verdicts
//
// The standing division is Go for the engine, R for the inference, CSV as the
// contract. A ratio of means computed over four positions invites exactly the
// argmax this record warns about — pick the position furthest from the pool and
// call it a difference — so the uncertainty and the multiplicity correction are
// stats/blank_run_position.R's, over the per-observation dump this writes.
package backtest

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

func TestDiagAvailabilityByPosition(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	// Not a Skip. An operator who set DIAG and forgot this switch would get a
	// complete-looking table measuring the residual after the very term being
	// calibrated, and nothing in the output would say which of the two it was.
	if os.Getenv("FPL_NO_BLANK_RUN") == "" {
		t.Fatal("set FPL_NO_BLANK_RUN=1 as well: ExpectedMinutes carries " +
			"blankRunFactor, so at shipped config this calibration is fitted to " +
			"its own output. analysis reads the switch at package init, so it " +
			"has to be in the process environment.")
	}
	cfg := loadConfig(t)
	pairs := sweepPairNames()

	// Collected BEFORE the dump is opened. newRowDump truncates its target, so
	// opening first would leave a header-only CSV where a complete one had been
	// if the run then skipped — the "a killed sweep leaves a partial file that
	// reads downstream as a complete one" hazard, in miniature.
	all := collectAvailabilityObs(t, cfg)
	if len(all) < 200 {
		t.Skipf("only %d observations", len(all))
	}

	rows := newRowDump(os.Getenv("FPL_BLANKRUN_ROWS"), t.Fatalf,
		"season", "cutoff", "code", "position", "run", "run_capped",
		"flagged", "expected", "actual")
	defer rows.close()

	// The recorded table's own binning: runs past three are one "4 or more"
	// row, because that is where the exponential average has caught up.
	const runCap = 4
	bucket := func(run int) int {
		if run > runCap {
			return runCap
		}
		return run
	}
	runLabel := func(k int) string {
		if k == runCap {
			return "4 or more"
		}
		return strconv.Itoa(k)
	}

	// The raw run AND the capped bucket. An earlier version dumped only the
	// bucket, which collapsed 4-and-above irreversibly — the row that carries
	// the plateau's upper cliff would then have existed in the console log and
	// nowhere re-analysable.
	for _, r := range all {
		rows.write(r.season, strconv.Itoa(r.cutoff), strconv.Itoa(r.code),
			analysis.PositionForElementType(r.elemType),
			strconv.Itoa(r.run), strconv.Itoa(bucket(r.run)),
			strconv.FormatBool(r.flagged),
			strconv.FormatFloat(r.expected, 'f', 4, 64),
			strconv.FormatFloat(r.actual, 'f', 4, 64))
	}

	fmt.Printf("\n%d player-cutoffs, %s, established players only\n",
		len(all), seasonsLabel(len(pairs)))
	fmt.Printf("(5+ appearances averaging 60+ minutes at the cutoff).\n")
	fmt.Printf("Predicting mean minutes per gameweek over the next %d.\n",
		availabilityWindow)
	fmt.Printf("FPL_NO_BLANK_RUN=1, so ExpectedMinutes carries no blank-run discount.\n")
	// Provenance the rows file cannot carry, printed beside it. Both feed
	// ExpectedMinutes through blendRates, so a figure quoted from this dump
	// belongs to these settings and to no others. rowDump has no schema and no
	// run id by design — it is scratch — so the console log is where this lives.
	fmt.Printf("minutes_half_life=%g blend_minutes_k=%g horizon=%d\n",
		cfg.Weights.MinutesHalfLife, cfg.Weights.BlendMinutesK, cfg.Weights.Horizon)

	// ratio is the recorded table's own estimator: mean(actual)/mean(expected)
	// over a cell, which on a common n is the ratio of the sums. Keeping it
	// identical matters — AGENTS.md, "an estimator swap reads as a data change" —
	// because the pooled figures this is compared against were computed that way.
	ratio := func(rows []availObs) (n int, exp, act, r float64) {
		for _, o := range rows {
			exp += o.expected
			act += o.actual
		}
		n = len(rows)
		if n == 0 || exp == 0 {
			return n, 0, 0, 0
		}
		return n, exp / float64(n), act / float64(n), act / exp
	}

	positions := []string{"GKP", "DEF", "MID", "FWD"}
	// Element types in the same order, so the label and the filter cannot drift.
	elemFor := map[string]int{"GKP": 1, "DEF": 2, "MID": 3, "FWD": 4}

	pick := func(pos string, k int, flagged bool) []availObs {
		var out []availObs
		for _, o := range all {
			if o.flagged != flagged {
				continue
			}
			if pos != "" && o.elemType != elemFor[pos] {
				continue
			}
			if k >= 0 && bucket(o.run) != k {
				continue
			}
			out = append(out, o)
		}
		return out
	}

	report := func(head string, flagged bool) {
		fmt.Printf("\n=== %s\n\n", head)
		fmt.Printf("%-10s %-11s %6s %9s %9s %8s %8s\n",
			"position", "blanks", "n", "expected", "actual", "bias", "ratio")
		for _, pos := range append([]string{""}, positions...) {
			label := pos
			if label == "" {
				label = "all"
			}
			for k := 0; k <= runCap; k++ {
				n, exp, act, r := ratio(pick(pos, k, flagged))
				if n == 0 {
					continue
				}
				fmt.Printf("%-10s %-11s %6d %9.1f %9.1f %+8.1f %8.4f\n",
					label, runLabel(k), n, exp, act, act-exp, r)
			}
			fmt.Printf("\n")
		}
	}
	report("UNFLAGGED — the population the recorded calibration used", false)
	report("FLAGGED — availabilityFactor already has this channel", true)

	// The de-levelling itself, which is the thing the claim licenses. The
	// plateau is runs 1-3 pooled, because that is what one constant covers;
	// the per-run rows are printed above for anyone reading the shape.
	fmt.Printf("\n=== the de-levelling, unflagged only\n\n")
	fmt.Printf("run-0 ratio is the divisor the comment calls position-wide.\n")
	fmt.Printf("raw = plateau ratio; delevelled = raw / run-0 ratio.\n\n")
	fmt.Printf("%-10s %6s %10s %6s %8s %11s\n",
		"position", "n(0)", "run-0", "n(1-3)", "raw", "delevelled")
	for _, pos := range append([]string{""}, positions...) {
		label := pos
		if label == "" {
			label = "all"
		}
		n0, _, _, r0 := ratio(pick(pos, 0, false))
		var plateau []availObs
		for k := 1; k <= 3; k++ {
			plateau = append(plateau, pick(pos, k, false)...)
		}
		nP, _, _, rP := ratio(plateau)
		if n0 == 0 || nP == 0 || r0 == 0 {
			continue
		}
		fmt.Printf("%-10s %6d %10.4f %6d %8.4f %11.4f\n",
			label, n0, r0, nP, rP, rP/r0)
	}

	fmt.Printf("\nNo standard error is printed here on purpose. Four positions " +
		"invite an argmax\nover the one furthest from the pool; the uncertainty, " +
		"the season clustering and\nthe multiplicity correction are in " +
		"stats/blank_run_position.R.\n")
}
