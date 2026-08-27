package stats

import "sort"

// Ranks are the positions of a slice's values in sorted order, **averaging the
// positions of tied values** — the midrank convention.
//
// # Why ties are the whole point
//
// Assigning tied values distinct positions, by whatever order a sort happens to
// leave them in, injects noise into the rank vector and **attenuates any
// correlation computed from it**. The attenuation scales with how tied the data
// are, which makes a rank correlation incomparable between populations of
// different tie density.
//
// ⚠️ **That is not hypothetical here, and it invalidated a table.** FPL points
// are small integers, so a population of every priced player carries hundreds of
// tied zeros, ones and twos in a gameweek, while a population of realistic picks
// carries far fewer. A diagnostic comparing rank correlation on the two was
// therefore comparing **two different instruments**, with the tie-dense side
// attenuated harder — and the gap it reported was partly the instrument. Midranks
// remove that.
//
// The average of the tied positions is the standard convention and is what
// Spearman's rho is defined on; a rho computed from arbitrary tie-breaking is not
// Spearman's rho for tied data.
func Ranks[T Number](xs []T) []float64 {
	n := len(xs)
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return xs[idx[a]] < xs[idx[b]] })
	for i := 0; i < n; {
		j := i
		for j+1 < n && xs[idx[j+1]] == xs[idx[i]] {
			j++
		}
		// Positions i..j are tied: every one of them takes their mean.
		mid := float64(i+j) / 2
		for k := i; k <= j; k++ {
			out[idx[k]] = mid
		}
		i = j + 1
	}
	return out
}

// Spearman is the rank correlation of two equal-length slices, on midranks.
//
// Returns 0 for fewer than two pairs, mismatched lengths, or a constant input —
// a constant has no ordering to correlate with, and 0 says "no ordering
// information" rather than manufacturing one.
//
// ⚠️ **A rank correlation is not comparable across populations whose tie density
// or size differ**, even computed correctly. Restricting to a high-scoring subset
// removes the easy part of the ordering problem, so rho falls with no change in
// skill at all — a fall must be read against a null that reproduces the same
// selection, never against the unrestricted figure. This function computes the
// statistic; it cannot supply the threshold.
func Spearman[T Number](a, b []T) float64 {
	if len(a) != len(b) || len(a) < 2 {
		return 0
	}
	ra, rb := Ranks(a), Ranks(b)
	var ma, mb float64
	for i := range ra {
		ma += ra[i]
		mb += rb[i]
	}
	n := float64(len(ra))
	ma, mb = ma/n, mb/n
	var num, da, db float64
	for i := range ra {
		x, y := ra[i]-ma, rb[i]-mb
		num += x * y
		da += x * x
		db += y * y
	}
	if da <= 0 || db <= 0 {
		return 0
	}
	return num / sqrt(da*db)
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	g := x
	for i := 0; i < 60; i++ {
		g = 0.5 * (g + x/g)
	}
	return g
}
