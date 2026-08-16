// Package stats holds descriptive statistics shared by more than one package.
//
// # Why it exists
//
// "The middle value of a slice" had **eleven expressions in nine files across
// four packages** — `analysis`, `backfill`, `backtest` and `cmd/armband` — under
// two conventions and three names. Two were named: `medianOf` in
// `internal/backtest/teamnewsfile.go`, which averaged the two middle values, and
// `median` in `internal/backtest/stats_test.go`, which took the upper. They had
// already diverged on every even-length input. The rest were the same four-line
// idiom written inline.
//
// (An earlier version of this comment said "ten expressions across six packages".
// Six was the number of *files* the guard newly failed on, and the package count
// was never checked — `git grep -E '\[len\([a-z]+\)/2\]'` on the pre-change tree
// is what settles it.)
//
// `stats_test.go` had already consolidated six copies of `mean` inside its own
// package for exactly this reason, and its header names the class: "this
// package's recorded history is a list of quantities that had two
// implementations and then stopped agreeing". That argument stopped at the
// package boundary, and **a repeated four-line idiom has no name to collide
// with**, so neither a name scan nor a reviewer reading one file could see the
// rest. The guard therefore matches the *shape* — `[len(x)/2]` — rather than the
// word.
//
// # There is one median, and the upper variant was withdrawn
//
// A first version of this package shipped two estimators, `Median` and
// `UpperMedian`, on the reasoning that both had a figure in the record standing
// on them and picking either would silently move the other. That was measured
// and true, and it was still the wrong answer.
//
// The reason is that the upper median was **an accident at every single site**.
// Nine of the ten copies took it, not because anyone chose an estimator, but
// because `sort.Float64s(s); s[len(s)/2]` is what you write when you want a
// median and are not thinking about ties. Checked one at a time: `PointsPerTenth`
// is a slope estimate, where averaging is strictly better; the two display sites
// and the per-position threshold in `research.go` have no reason whatever;
// `backfill.Compare` recovers FPL's ninety-minute rule, where an observed value
// sounds principled but every gap sits at 1.50 so it makes no difference. Only
// the chip diagnostic had an argument — its median is compared against an
// observed maximum — and it is a weak one.
//
// So keeping both was preserving an artefact and calling it a design. One
// quantity, one implementation, the ordinary definition: what R's `median()`,
// Python's `statistics.median` and every table in `stats/*.R` mean by the word.
//
// # What that cost, paid rather than avoided
//
// Collapsing moved one recorded figure, and it was re-measured rather than
// estimated: the chips note's "playing it at all" row, through the chip
// diagnostic's median week. The sweep was run twice on one tree — once under each
// estimator — so the delta is attributable to the median and not to the grid
// having widened from 24 cells to 36 since that table was written.
//
// ⚠️ **Two figures moved, not one, and the second was expected not to.** Three
// `model.transfer_error.*.median` rows moved by −0.0005 to −0.0018. The reasoning
// that they could not was that their populations are odd-sized, where the two
// conventions cannot differ — but those counts described one run at one setting,
// and parity was assumed rather than checked. The accuracy snapshot is what
// settled it, and the *means* on the same rows are byte-identical, which is the
// proof that the population did not change and the estimator did.
//
// Nothing else moved. `MedianHours` and `docs/backfill.md`'s 4.7-8.5 already
// averaged; the BPS and determinism diagnostics compute a median the record does
// not quote; two sites are display only. One reproducibility break is named
// rather than hidden: `PointsPerTenth` is reached from the replay's transfer path
// only when `FPL_BUDGET_WEIGHT` is set, so the shipped replay is byte-identical
// but that experimental arm is not.
package stats

import "slices"

// Number is the numeric types this repo actually takes a median of. Deliberately
// not every numeric type in Go: the constraint is a list of what exists, so
// widening it is a decision somebody makes rather than something that happens.
type Number interface {
	~int | ~int64 | ~float64
}

// Median is the middle value, averaging the two middle values when the count is
// even — the ordinary definition.
//
// It returns float64 for every input type, because the median of an even-length
// []int genuinely can be a half-integer and returning T would silently truncate
// it. That is not hypothetical: the chip diagnostic averages a median weekly gain
// in points across 36 cells, and truncating each one is a downward bias on the
// figure the record quotes.
//
// It returns 0 for an empty slice rather than failing. Every implementation this
// replaced did the same, five of the six `mean` copies before them did too, and
// the case is degenerate at every call site — no treated players, no gameweeks
// compared. A diagnostic printing 0 for "nothing to average" is easier to read
// than one printing NaN.
//
// It sorts a copy. Several callers pass a slice whose order carries meaning — a
// per-gameweek series, a per-capture series — and sorting in place would corrupt
// the caller's data silently.
func Median[T Number](xs []T) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return float64(s[mid])
	}
	return (float64(s[mid-1]) + float64(s[mid])) / 2
}
