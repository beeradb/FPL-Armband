package backtest

// Descriptive statistics for the diagnostics, in one place.
//
// # Why this file exists
//
// "The mean of a []float64" had **six** implementations in this package: `meanOf`
// and `mean` at package level in two different files, plus byte-identical local
// closures in `calibration_test.go`, `teamblend_prior_test.go`,
// `teamblend_calibration_test.go` and `variance_test.go`. `meanSE` computed a
// seventh inline. `sd` and `median` were each defined in one file and reached for
// from others.
//
// None of that was causing a wrong answer, which is exactly why it was worth
// fixing before it did. This package's recorded history is a list of quantities
// that had two implementations and then stopped agreeing — `DefaultBenchWeight`
// against `Weights.BenchWeight`, `fixtureSensitivePart` against `baseXP90`, two
// copies of the season-clustered standard error, two derivations of the
// `.means.csv` path, two expressions of the per-gameweek denominator. Every one
// was harmless when written.
//
// # The one semantic difference, and how it was resolved
//
// Five of the six mean implementations returned 0 for an empty slice; the closure
// in `calibration_test.go` had no guard and returned NaN. On every non-empty input
// they are identical, so no measurement moves — and the empty case is degenerate
// there (it would mean no treated players at all). The guarded version wins
// because a diagnostic that prints 0 for "nothing to average" is easier to read
// than one that prints NaN, and because five call sites already assumed it.
//
// # What is deliberately still separate
//
// `promoted_test.go` keeps its own mean: it takes a projection function over a
// slice of structs rather than a `[]float64`, which is a different signature doing
// a different job, and flattening it would mean allocating a slice at every call
// site to satisfy a shared helper. That is the standing rule for this package —
// one implementation per *quantity*, not one per superficial shape.
//
// # The median escaped this file twice, and was consolidated a package up
//
// This header's argument stopped at the package boundary, and the middle value
// had three implementations under two conventions across two packages:
// `teamnewsfile.go`'s `medianOf` averaged the two middle values, while `median`
// here and an inline copy in `internal/backfill` took the upper of the two. Two of
// the three were production code, so no amount of tidying inside `_test.go` files
// could have reached them.
//
// There is now one, [stats.Median], the ordinary definition. A first attempt kept
// two named estimators, on the reasoning that both had a figure in the record
// standing on them — true, and still wrong, because the upper median turned out to
// be an accident at every site rather than anyone's choice. The one figure that
// moved was re-measured. `median` below is a one-line wrapper kept for the call
// sites; the full argument and the accounting are in `internal/stats`.

import (
	"fmt"
	"math"
	"sort"

	"armband/internal/stats"
)

// meanOf is the arithmetic mean, 0 for an empty slice.
func meanOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var t float64
	for _, x := range xs {
		t += x
	}
	return t / float64(len(xs))
}

// sd is the sample standard deviation, on n-1 degrees of freedom.
//
// NaN for fewer than two values, deliberately: there is no spread to report from
// one observation, and a diagnostic that printed 0 would be claiming certainty it
// does not have.
func sd(xs []float64) float64 {
	if len(xs) < 2 {
		return math.NaN()
	}
	m := meanOf(xs)
	var v float64
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return math.Sqrt(v / float64(len(xs)-1))
}

// tCrit95 is the two-sided 5% critical value of Student's t at df degrees of
// freedom.
//
// A diagnostic printing a *computed* df beside a *hardcoded* critical value is
// the grid-label defect in another costume: two halves of one statement that can
// disagree, and nothing checks that they still do not. Two in teamshare_test.go
// did. Both printed 3.182 — the value at df 3, which is what four seasons give —
// on lines that computed and printed a df of 5 once the default grid widened to
// six seasons.
//
// A table rather than a series expansion because the range that matters here is
// a season count minus one, and it panics outside it for the reason
// parseSweepStarts panics on an unreadable entry-point grid: a plausible critical
// value nobody chose is worse than a stop.
//
// # This is one quantity in two languages, and that is the deliberate part
//
// `stats/` computes the same `qt(0.975, df)` from R's own distribution function
// in six scripts. Collapsing to one implementation is not available: a Go
// diagnostic that prints a t beside a computed df needs the number in Go, and
// this package has printed one since before either scan existed, while every
// *verdict* still comes from the R path. So the rule this project applies to a
// second copy — extend an existing scan rather than add a runtime equivalence
// test — is applied to the THIRD: TestInferenceLivesInOnePlace walks the AST for
// a tabulated critical value in code outside this file, which its marker strings
// could never see, since a table is ten float literals and no idiom.
func tCrit95(df int) float64 {
	table := map[int]float64{
		1: 12.706, 2: 4.303, 3: 3.182, 4: 2.776, 5: 2.571,
		6: 2.447, 7: 2.365, 8: 2.306, 9: 2.262, 10: 2.228,
	}
	v, ok := table[df]
	if !ok {
		panic(fmt.Sprintf("tCrit95: df %d is outside the tabulated 1..10. The df here "+
			"is a season count minus one, so this means the grid moved further than "+
			"this table did — extend it rather than approximating.", df))
	}
	return v
}

// seOf is the standard error of the mean.
func seOf(xs []float64) float64 {
	if len(xs) < 2 {
		return math.NaN()
	}
	return sd(xs) / math.Sqrt(float64(len(xs)))
}

// meanSE returns both, and returns (0, 0) rather than NaN below two values
// because its callers print it in tables where a zero reads as "not measured".
//
// It is expressed through meanOf and seOf rather than recomputing either, which is
// the whole point of this file: there was a period when this function held the
// package's seventh mean and third variance formula.
func meanSE(xs []float64) (mean, se float64) {
	if len(xs) < 2 {
		return 0, 0
	}
	return meanOf(xs), seOf(xs)
}

// median is the middle value, averaging the two middle values for an even count.
//
// A one-line wrapper on [stats.Median] so the diagnostics in this package read as
// they always have. It used to be a second implementation of a word that meant
// something else one file away — `teamnewsfile.go`'s `medianOf` averaged where
// this took the upper — and eight more copies of the same idiom were inlined
// across four other packages. There is now one.
//
// ⚠️ **This changed estimator, which is a data change on anything quoting it.**
// Two consumers quote a median into the record: the chip diagnostic, re-measured
// by running its sweep under both estimators on one tree, and `TestDiagTransferError`,
// whose four medians appear in `docs/accuracy.md`.
//
// The transfer-error medians were expected not to move — this file's own header
// describes a run whose three populations are 327, 284 and 43, all odd, and the
// conventions cannot differ on an odd count. **That expectation is not evidence**:
// those counts describe one run at one setting, and `docs/accuracy.md` quotes
// different figures (−0.61/−0.20/−0.05/−2.47) than that header does, so it is a
// different population whose parity nobody has checked. The accuracy snapshot
// regenerated alongside this change is what settles it, because it re-runs the
// diagnostic and the rows either move or they do not.
//
// See internal/stats for the full accounting.
func median(xs []float64) float64 { return stats.Median(xs) }

// spearman is Spearman's rank correlation: the ordinary (Pearson) correlation
// computed on ranks rather than on values.
//
// +1 means the two orderings agree exactly, 0 means one carries no information
// about the other, −1 means they are exactly reversed. It answers a different
// question from an error figure, and it is the question this project's optimiser
// actually asks: a squad search consumes an *ordering* of players and never a
// level, which is why the bonus term is kept despite being badly calibrated.
//
// Ties share the average of the ranks they span, which is the standard treatment
// and matters here rather than being a technicality: a gameweek's realised points
// are full of ties — dozens of players on exactly 2 — and assigning them
// arbitrary distinct ranks would inject an ordering the data does not contain.
//
// Reported as (rho, ok). It returns ok=false when there is nothing to correlate:
// fewer than two observations, or one of the two inputs constant, in which case
// no ordering exists to agree with rather than a correlation of zero.
func spearman(x, y []float64) (float64, bool) {
	if len(x) < 2 || len(x) != len(y) {
		return 0, false
	}
	rx, ry := tiedRanks(x), tiedRanks(y)
	mx, my := meanOf(rx), meanOf(ry)
	var num, dx, dy float64
	for i := range rx {
		a, b := rx[i]-mx, ry[i]-my
		num += a * b
		dx += a * a
		dy += b * b
	}
	if dx == 0 || dy == 0 {
		return 0, false
	}
	return num / math.Sqrt(dx*dy), true
}

// tiedRanks ranks ascending from 1, giving tied values the average of the ranks
// they span.
func tiedRanks(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	out := make([]float64, len(v))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		// Ranks i+1 .. j+1 inclusive, averaged.
		avg := float64(i+1+j+1) / 2
		for k := i; k <= j; k++ {
			out[idx[k]] = avg
		}
		i = j + 1
	}
	return out
}

// medianInt is deleted, and its absence is the point.
//
// It existed to give the chip diagnostic an `int` median of an `int` series, by
// widening to float64, taking the median and truncating back. That round-tripped
// exactly under the upper median, because the upper median of a slice is one of
// its own elements — and it stopped being exact the moment the estimator became
// the ordinary one, where an even count lands on a half-integer. Truncating each
// of 36 cells before averaging them is a downward bias on the figure the chips
// note quotes.
//
// So the caller takes float64 from [stats.Median] and never rounds. The lesson is
// that the `int`-in, `int`-out signature was the defect: it looked like a
// convenience and was really a lossy conversion nobody had priced.
