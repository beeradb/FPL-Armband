package backtest

// Fit P(a player records at least one minute) against his mean minutes.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagPlaysAtAll -v
//
// # Why this was needed
//
// FPL pays ONE appearance point for playing at all and TWO at sixty minutes or
// more, and the model used to have only the upper branch: it scaled the whole
// appearance term by `playsSixty(ExpectedMinutes)`, so a player who appeared
// without reaching the hour was credited zero where FPL pays him one. The correct
// expectation is
//
//	appearance = P(appears at all) + P(reaches 60)
//
// so the shortfall was exactly the probability of appearing but finishing under the
// hour, and the missing ingredient was a fitted P(appears at all) — the sibling of
// `playsSixty`. This is the fit behind it. Both now ship; see
// internal/analysis/appearance.go.
//
// # Why the blank-rate machinery could not supply it, and what happened next
//
// `blankRate` looked like the right quantity and was not. It was
// `blankFromNotStarting x (1 - StartShare)` with the constant fitted through the
// origin over start share 0.70+, the regime a starting eleven occupies. At a start
// share of zero it returns 0.624, so `1 - blankRate` would have credited a player
// who has never played with 0.376 of an appearance point in perpetuity — retiring
// the property that no Premier League data scores 0.00, which `research_targets`
// depends on.
//
// So this fitted its own curve, held below the Markov bound P(X >= 1) <= mean
// minutes, which forces zero at zero minutes.
//
// **The sequel is that having two curves for one quantity was itself the bug.**
// `blankRate` was not merely unsuitable for the appearance point; it was a SECOND
// estimator of P(appears), consumed by the derived bench slot weights and by
// `defconPerGameweek`, disagreeing with this one by up to 0.376 and biased upward by
// about eight points of probability. It now reads this curve instead, and
// TestDiagStartShare below carries that measurement along with the reason the
// start-aware replacement for both curves was rejected.
//
// # Measurement
//
// One row per player-season. Restricted to single-fixture gameweeks so minutes are
// per-match rather than per-double, and the denominator is the gameweeks his club
// actually played — taken from the fixture list, because a blank gameweek has no
// row at all and counting rows would mistake a club blank for a player blank.

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// appearanceFitSeasons is the population both appearance fits in this file read,
// and it is a literal rather than the shared grid on purpose: the fits are
// per-player-season regressions, not paired replay cells, and a backfilled season
// carries a borrowed provider offset that a fitted coefficient would absorb.
//
// It is named so the two headers that report it can count it. Both used to say
// "four seasons" in prose beside a computed sample size, which is the shape that
// let a grid label outlive the grid it described.
var appearanceFitSeasons = []string{"2022-23", "2023-24", "2024-25", "2025-26"}

func TestDiagPlaysAtAll(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type row struct {
		meanMins float64 // over gameweeks his club played
		appeared float64 // share of those with at least a minute
		sixty    float64 // share of those reaching the hour
		startSh  float64 // share of those he started
		n        int
	}
	var rows []row

	for _, name := range appearanceFitSeasons {
		cur := loadSeason(t, cfg, name)

		// Single-fixture gameweeks per club, from the fixture list.
		played := map[int]map[int]bool{} // team -> gw -> true
		count := map[[2]int]int{}        // (team, gw) -> fixtures
		for _, f := range cur.Fixtures {
			if f.Event == nil {
				continue
			}
			count[[2]int{f.TeamH, *f.Event}]++
			count[[2]int{f.TeamA, *f.Event}]++
		}
		for k, c := range count {
			if c != 1 {
				continue // a double: minutes are a two-match total
			}
			if played[k[0]] == nil {
				played[k[0]] = map[int]bool{}
			}
			played[k[0]][k[1]] = true
		}

		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			gws := played[p.Team]
			if len(gws) < 20 {
				continue
			}
			var mins, apps, sixties, starts, n float64
			for gw := range gws {
				n++
				g, ok := p.GWs[gw]
				if !ok {
					continue // club played, he did not: a genuine blank
				}
				mins += float64(g.Minutes)
				if g.Minutes >= 1 {
					apps++
				}
				if g.Minutes >= 60 {
					sixties++
				}
				starts += float64(g.Starts)
			}
			if n < 20 {
				continue
			}
			// Drop players who never featured at all: their appearance rate is
			// trivially zero and they would dominate the low band without
			// informing the curve, which is about players who DO play sometimes.
			if apps == 0 {
				continue
			}
			rows = append(rows, row{
				meanMins: mins / n, appeared: apps / n, sixty: sixties / n,
				startSh: starts / n, n: int(n),
			})
		}
	}

	if len(rows) < 200 {
		t.Skipf("only %d player-seasons", len(rows))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].meanMins < rows[j].meanMins })

	// The shipped sixty-minute curve, for comparison and as the shape to beat.
	//
	// Read from the package. It used to be a local copy of the logistic — with the
	// coefficients inlined and **both exact bounds missing**, so it diverged from
	// what ships by +0.028 at one mean minute and −0.061 at ninety, where the
	// (m-60)/30 floor binds and the bare logistic saturates at 0.939 instead of 1.
	// That fed the "current against correct" appearance table below, whose figures
	// were then quoted verbatim into appearanceFactor's comment, and it made the
	// m=0 line print 0.0423 where the shipped model returns exactly 0 — reporting a
	// failure of the property research_targets is built on that the model does not
	// have.
	//
	// Same rule as TestDiagSixtyMinutes, which had the identical defect: a
	// diagnostic must never carry its own copy of the curve it is checking.
	playsSixtyFit := analysis.PlaysSixty

	// Grid-search a logistic for P(appears), Markov-capped at mean minutes so it
	// is exactly zero at zero. Same functional form as the sixty-minute fit, so
	// the two are directly comparable and no new shape is introduced.
	best := struct{ k, m0, rms float64 }{rms: math.Inf(1)}
	for k := 0.02; k <= 0.30; k += 0.005 {
		for m0 := -10.0; m0 <= 40.0; m0 += 0.5 {
			var ss float64
			for _, r := range rows {
				p := 1 / (1 + math.Exp(-k*(r.meanMins-m0)))
				p = math.Min(p, r.meanMins) // Markov: P(X>=1) <= E[X]
				d := p - r.appeared
				ss += d * d
			}
			if rms := math.Sqrt(ss / float64(len(rows))); rms < best.rms {
				best = struct{ k, m0, rms float64 }{k, m0, rms}
			}
		}
	}

	fmt.Printf("\n=== P(records at least one minute), fitted against mean minutes\n")
	fmt.Printf("%d player-seasons, %s, single-fixture gameweeks only.\n",
		len(rows), seasonsLabel(len(appearanceFitSeasons)))
	fmt.Printf("Fit: 1/(1+exp(-%.3f(m-%.1f))), capped at min(fit, m). rms %.4f\n\n",
		best.k, best.m0, best.rms)

	fit := func(m float64) float64 {
		return math.Min(1/(1+math.Exp(-best.k*(m-best.m0))), m)
	}

	fmt.Printf("%-14s %6s %9s %9s %9s %9s %9s\n",
		"mean minutes", "n", "measured", "fitted", "P(60+)meas", "P(60+)fit", "gap")
	bands := []float64{5, 15, 25, 35, 45, 55, 65, 75, 85, 91}
	lo := 0.0
	for _, hi := range bands {
		var mm, ap, sx, n float64
		for _, r := range rows {
			if r.meanMins >= lo && r.meanMins < hi {
				mm += r.meanMins
				ap += r.appeared
				sx += r.sixty
				n++
			}
		}
		if n >= 5 {
			mm /= n
			fmt.Printf("%5.0f - %-6.0f %6.0f %9.3f %9.3f %9.3f %9.3f %9.3f\n",
				lo, hi, n, ap/n, fit(mm), sx/n, playsSixtyFit(mm), ap/n-sx/n)
		}
		lo = hi
	}
	// ---------------------------------------------------------------------
	// A better-behaved parameterisation, from an identity rather than a curve.
	//
	// E[minutes] = P(appears) x E[minutes | appears], so
	//
	//	P(appears) = mean minutes / E[minutes when he appears]
	//
	// exactly. That is worth preferring for three reasons: it is an identity
	// rather than a fitted shape, the conditional mean is a smoother quantity to
	// fit than a probability bounded at 0 and 1, and it forces P -> 0 as mean
	// minutes -> 0 by construction rather than by a Markov cap bolted on top.
	fmt.Printf("\n=== The conditional mean, which is what the identity needs\n")
	fmt.Printf("E[minutes when he appears], by mean minutes. If this were constant the\n")
	fmt.Printf("appearance curve would be a straight line through the origin.\n\n")
	fmt.Printf("%-14s %6s %11s %11s\n", "mean minutes", "n", "condMean", "impliedP")
	lo2 := 0.0
	type cm struct{ m, cond float64 }
	var cms []cm
	for _, hi := range bands {
		var mm, cond, n float64
		for _, r := range rows {
			if r.meanMins >= lo2 && r.meanMins < hi && r.appeared > 0 {
				mm += r.meanMins
				cond += r.meanMins / r.appeared
				n++
			}
		}
		if n >= 5 {
			mm /= n
			cond /= n
			fmt.Printf("%5.0f - %-6.0f %6.0f %11.1f %11.3f\n", lo2, hi, n, cond, mm/cond)
			cms = append(cms, cm{mm, cond})
		}
		lo2 = hi
	}

	// Fit the conditional mean linearly in mean minutes, then derive P.
	var sx, sy, sxx, sxy, sn float64
	for _, r := range rows {
		if r.appeared <= 0 {
			continue
		}
		c := r.meanMins / r.appeared
		if c > 90 {
			c = 90 // cannot exceed a full match
		}
		sx += r.meanMins
		sy += c
		sxx += r.meanMins * r.meanMins
		sxy += r.meanMins * c
		sn++
	}
	slope := (sn*sxy - sx*sy) / (sn*sxx - sx*sx)
	icept := (sy - slope*sx) / sn
	identP := func(m float64) float64 {
		c := icept + slope*m
		if c < 1 {
			c = 1
		}
		if c > 90 {
			c = 90
		}
		return math.Min(m/c, 1)
	}
	var ss float64
	for _, r := range rows {
		d := identP(r.meanMins) - r.appeared
		ss += d * d
	}
	fmt.Printf("\nE[mins|appears] ~ %.2f + %.4f x m   (least squares)\n", icept, slope)
	fmt.Printf("Implied P(appears) = m / that, rms %.4f against the logistic's %.4f\n",
		math.Sqrt(ss/float64(len(rows))), best.rms)
	fmt.Printf("At m=0 it is exactly %.4f by construction.\n\n", identP(0))
	fmt.Printf("%-14s %6s %9s %9s %9s\n", "mean minutes", "n", "measured", "logistic", "identity")
	lo3 := 0.0
	for _, hi := range bands {
		var mm, ap, n float64
		for _, r := range rows {
			if r.meanMins >= lo3 && r.meanMins < hi {
				mm += r.meanMins
				ap += r.appeared
				n++
			}
		}
		if n >= 5 {
			mm /= n
			fmt.Printf("%5.0f - %-6.0f %6.0f %9.3f %9.3f %9.3f\n", lo3, hi, n, ap/n, fit(mm), identP(mm))
		}
		lo3 = hi
	}

	fmt.Printf("\nThe last column is the population FPL pays one point to and the model\n")
	fmt.Printf("pays nothing: appeared, did not reach the hour.\n")

	// What the correction is worth, in appearance points per gameweek.
	fmt.Printf("\n=== What the missing point is worth, appearance points per gameweek\n")
	fmt.Printf("current = 2 x P(60+ fitted). correct = P(appears fitted) + P(60+ fitted).\n\n")
	fmt.Printf("%-14s %6s %9s %9s %9s\n", "mean minutes", "n", "current", "correct", "diff")
	lo = 0
	var totalDiff, totalN float64
	for _, hi := range bands {
		var mm, n float64
		for _, r := range rows {
			if r.meanMins >= lo && r.meanMins < hi {
				mm += r.meanMins
				n++
			}
		}
		if n >= 5 {
			mm /= n
			cur := 2 * playsSixtyFit(mm)
			corr := fit(mm) + playsSixtyFit(mm)
			fmt.Printf("%5.0f - %-6.0f %6.0f %9.3f %9.3f %+9.3f\n", lo, hi, n, cur, corr, corr-cur)
			totalDiff += (corr - cur) * n
			totalN += n
		}
		lo = hi
	}
	// What one statistic cannot say — and how much of that is really nothing.
	//
	// The obvious test is to hold mean minutes in a band and split by start share:
	// the realised sixty-minute rate then rises by about 0.11 across the terciles.
	// **That is not on its own an ordering error, and reading it as one is the trap.**
	// Start share and mean minutes stay correlated INSIDE a band, so the higher
	// tercile also has higher mean minutes and the fit rises too. What the fit cannot
	// express is only the part left over, which is the `resid` column: measured spread
	// minus fitted spread across the three terciles.
	//
	// That residual is small and inconsistent in sign — in one band the fit moves MORE
	// than the outcome does. TestDiagStartShare below asks whether a start-aware fit
	// predicts better out of sample; the answer is 0.14%, and this is why.
	fmt.Printf("\n=== what mean minutes alone cannot separate — and how little that is\n")
	fmt.Printf("Both fits read mean minutes alone, but mean minutes still rises across the\n")
	fmt.Printf("terciles, so the fitted columns move too. Only `resid` is unexplained.\n\n")
	fmt.Printf("%-14s %-14s %6s %9s %9s %9s %9s\n",
		"mean minutes", "start share", "n", "app meas", "app fit", "60 meas", "60 fit")
	for _, mb := range [][2]float64{{10, 25}, {25, 40}, {40, 55}, {55, 70}, {70, 85}} {
		var shares []float64
		for _, r := range rows {
			if r.meanMins >= mb[0] && r.meanMins < mb[1] {
				shares = append(shares, r.startSh)
			}
		}
		if len(shares) < 30 {
			continue
		}
		sort.Float64s(shares)
		q1, q3 := shares[len(shares)/3], shares[2*len(shares)/3]
		var firstM, firstF, lastM, lastF float64
		var seen int
		for _, sb := range [][2]float64{{-1, q1}, {q1, q3}, {q3, 2}} {
			var mmm, am, af, xm, xf, cnt float64
			for _, r := range rows {
				if r.meanMins >= mb[0] && r.meanMins < mb[1] &&
					r.startSh >= sb[0] && r.startSh < sb[1] {
					mmm += r.meanMins
					am += r.appeared
					xm += r.sixty
					cnt++
				}
			}
			if cnt >= 10 {
				af, xf = fit(mmm/cnt), playsSixtyFit(mmm/cnt)
				fmt.Printf("%5.0f - %-6.0f %.2f - %-8.2f %6.0f %9.3f %9.3f %9.3f %9.3f\n",
					mb[0], mb[1], math.Max(0, sb[0]), math.Min(1, sb[1]), cnt,
					am/cnt, af, xm/cnt, xf)
				if seen == 0 {
					firstM, firstF = xm/cnt, xf
				}
				lastM, lastF = xm/cnt, xf
				seen++
			}
		}
		if seen >= 2 {
			fmt.Printf("%-14s %-14s %6s %9s %9s %9s %+9.3f  <- resid on P(60+)\n",
				"", "  spread", "", "", "", "", (lastM-firstM)-(lastF-firstF))
		}
	}
	fmt.Printf("\nRead the resid column, not the measured one. The measured sixty-minute rate\n")
	fmt.Printf("rises about 0.11 across the terciles in every band — but the FIT rises too,\n")
	fmt.Printf("because start share and mean minutes stay correlated inside a band. What is\n")
	fmt.Printf("left unexplained is a few hundredths, it is not monotone across the bands, and\n")
	fmt.Printf("in one of them the fit over-reacts. So the ordering error mean minutes cannot\n")
	fmt.Printf("express is much smaller than the raw split suggests, which is why the\n")
	fmt.Printf("start-aware fit in TestDiagStartShare buys 0.14%%.\n")

	fmt.Printf("\nPooled mean change: %+.3f appearance points per gameweek.\n", totalDiff/totalN)
	fmt.Printf("A player at zero mean minutes must read exactly 0.000 on the correct\n")
	fmt.Printf("column, or the 'no Premier League data scores 0.00' property is gone:\n")
	fmt.Printf("  at m=0.0  correct = %.4f\n", fit(0)+playsSixtyFit(0))
	fmt.Printf("  at m=0.5  correct = %.4f\n", fit(0.5)+playsSixtyFit(0.5))
}

// ---------------------------------------------------------------------------
// The start-share axis: is mean minutes really one sufficient statistic short?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagStartShare -v
//
// # The question
//
// playsSixty and playsAtAll both read mean minutes alone, and mean minutes
// conflates two different footballers: one who plays twenty minutes every week and
// one who plays ninety once a month. The archive carries per-gameweek `starts`
// beside minutes, so a start / substitute / unused split is available. The comment
// on playsAtAll called the term provisional pending exactly that, and the
// expected-points review's Gap 7 lists it.
//
// # The answer, in one line
//
// Start share is not a second statistic in practice. It correlates with mean
// minutes at 0.9934 in the population the model faces, and against the same
// functional form refitted WITHOUT it, adding it buys 0.1% on P(60+) and 0.4% on
// P(appears) out of sample. Refitting the constants of what already ships is worth
// 3.9% and 2.0% — four to twenty times more, and both are small.
//
// What is real is a descriptive ordering error: hold mean minutes fixed and the
// realised sixty-minute rate rises monotonically with start share in every band,
// spanning about 0.11, while the estimator returns one number. That error is genuine
// and it is not predictable, because at the granularity the model estimates from,
// within-bin start share is mostly noise. Descriptive and actionable disagree — the
// same shape as recency on rates, which predicted better and policied worse.
//
// # Why the fit window is 2023-24 onward and not four seasons
//
// The per-gameweek `starts` column is zero for all of 2021-22 and for 2022-23
// through GW15, and reconstructStarts repairs it BY RANK FROM MINUTES. Fitting a
// model whose entire premise is that starts carry information beyond minutes, on
// starts derived from minutes, is circular. 2022-23 is held out instead, and section
// 4 confirms it behaves like the recorded seasons.
// ---------------------------------------------------------------------------

// shippedSixty and shippedAtAll mirror internal/analysis. They are duplicated here
// deliberately: this file measures whether the shipped curves should change, so it
// has to be able to state what they are today independently of what they become.
// TestTheDiagnosticBaselineMatchesTheShippedFit binds the two local mirrors below
// to the package they mirror.
//
// shippedSixty and shippedAtAll are a DELIBERATE copy: they are the pre-change
// baseline every column in this file is compared against, and they cannot call
// analysis.PlaysSixty because that reads the live variables, which an arm may have
// overridden. That is the one legitimate reason to duplicate a curve — and it is
// still a duplicate, so without this test changing a shipped constant leaves the
// "shipped" column here quietly measuring last year's model with nothing failing.
//
// Everything else in this package that wants the shipped curve calls the package.
func TestTheDiagnosticBaselineMatchesTheShippedFit(t *testing.T) {
	slope, mid, intercept, condSlope := analysis.ShippedAppearanceFit()
	if slope != 0.065 || mid != 48 || intercept != 28.15 || condSlope != 0.779 {
		t.Fatalf("the shipped fit is now %v/%v/%v/%v; shippedSixty and shippedAtAll in "+
			"this file still hardcode 0.065/48/28.15/0.779, so every \"shipped\" column "+
			"here is measuring the old curve. Update both together.",
			slope, mid, intercept, condSlope)
	}
	// The constants agreeing is necessary and not sufficient — the mirrors also
	// carry the two exact bounds and the coupling, and those are what a careless
	// edit drops.
	for _, m := range []float64{0, 1, 15, 45, 65, 85, 90} {
		if got, want := shippedSixty(m), analysis.PlaysSixty(m); math.Abs(got-want) > 1e-12 {
			t.Errorf("shippedSixty(%v) = %.12f, package says %.12f", m, got, want)
		}
		if got, want := shippedAtAll(m), analysis.PlaysAtAll(m); math.Abs(got-want) > 1e-12 {
			t.Errorf("shippedAtAll(%v) = %.12f, package says %.12f", m, got, want)
		}
	}
}

func shippedSixty(m float64) float64 {
	if m <= 0 {
		return 0
	}
	p := 1 / (1 + math.Exp(-0.065*(m-48)))
	if hi := m / 60; hi < p {
		p = hi
	}
	if lo := (m - 60) / 30; lo > p {
		p = lo
	}
	return clampDiag(p, 0, 1)
}

func shippedAtAll(m float64) float64 {
	if m <= 0 {
		return 0
	}
	c := clampDiag(28.15+0.779*m, 1, 90)
	p := clampDiag(m/c, 0, 1)
	if x := shippedSixty(m); x > p {
		p = x
	}
	return p
}

// shippedBlank is the SECOND estimator of P(appears) that used to run beside
// playsAtAll: 1 - 0.624 x (1 - start share), reached through blankRate. Kept so the
// disagreement that motivated unifying them stays measurable rather than remembered.
func shippedBlank(s float64) float64 {
	return clampDiag(0.624*(1-clampDiag(s, 0, 1)), 0, 1)
}

func clampDiag(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// winRow is one windowed prediction: estimate mean minutes and start share over the
// season to a cutoff, then predict what happens over the next five gameweeks his
// club plays. That is the task the model actually performs, and a season-aggregate
// fit cannot stand in for it — the aggregate has 36 gameweeks of evidence behind
// each statistic where the model often has six.
type winRow struct {
	season string
	m, s   float64
	app    float64
	sixty  float64
	recon  bool
}

func winRowsFor(t *testing.T, cutoffs []int, ahead int) []winRow {
	t.Helper()
	cfg := loadConfig(t)
	var out []winRow
	for _, name := range appearanceFitSeasons {
		cur := loadSeason(t, cfg, name)
		// Single-fixture gameweeks only, so minutes are per match rather than per
		// double — the same restriction TestDiagPlaysAtAll uses above. clubGameweeks
		// is the package's one implementation of "how many matches did this club play
		// that gameweek"; sorted here because a window is a prefix in time and map
		// order is not one.
		single := map[int][]int{}
		for team, gws := range clubGameweeks(cur) {
			for gw, fixtures := range gws {
				if fixtures == 1 {
					single[team] = append(single[team], gw)
				}
			}
		}
		for tm := range single {
			sort.Ints(single[tm])
		}
		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			gws := single[p.Team]
			for _, c := range cutoffs {
				var mins, starts, hist float64
				var recon bool
				for _, gw := range gws {
					if gw > c {
						continue
					}
					hist++
					g, ok := p.GWs[gw]
					if !ok {
						continue // his club played, he did not: a genuine blank
					}
					mins += float64(g.Minutes)
					starts += float64(g.Starts)
					if g.StartsReconstructed {
						recon = true
					}
				}
				if hist < 5 {
					continue
				}
				var app, sixty, n float64
				for _, gw := range gws {
					if gw <= c || n >= float64(ahead) {
						continue
					}
					n++
					g, ok := p.GWs[gw]
					if !ok {
						continue
					}
					if g.Minutes >= 1 {
						app++
					}
					if g.Minutes >= 60 {
						sixty++
					}
				}
				if n < float64(ahead) {
					continue
				}
				out = append(out, winRow{season: name, m: mins / hist, s: starts / hist,
					app: app / n, sixty: sixty / n, recon: recon})
			}
		}
	}
	return out
}

// expectedMinutesRowsFor is winRowsFor with one change: the predictor is the
// model's own `ExpectedMinutes` rather than a windowed mean computed from raw
// archive rows.
//
// That change is the entire content of section 6, and it is not cosmetic.
// `ExpectedMinutes` is blended against a multi-season prior, recency-weighted at
// `MinutesHalfLife` and rested, so it carries materially less error than a bare
// six-gameweek mean — and a least-squares fit is a function of how noisy its
// predictor is, not only of the relationship being fitted. Fitting on the noisier
// axis and evaluating on the quieter one is the mismatch appearance.go names as its
// reason for leaving the recorded refit alone.
//
// The targets are identical to winRowsFor's, deliberately: same single-fixture
// restriction, same five-gameweek forward window, same "his club played and he did
// not is a genuine blank" rule. Only the x-axis moves, so the two sections are
// comparable.
func expectedMinutesRowsFor(t *testing.T, cutoffs []int, ahead int) []winRow {
	t.Helper()
	cfg := loadConfig(t)
	var out []winRow

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)

		single := map[int][]int{}
		for team, gws := range clubGameweeks(cur) {
			for gw, fixtures := range gws {
				if fixtures == 1 {
					single[team] = append(single[team], gw)
				}
			}
		}
		for tm := range single {
			sort.Ints(single[tm])
		}

		for _, c := range cutoffs {
			boot, fx := PointInTime(cur, prior, c)
			// Horizon 1, for the same reason the prediction benchmark uses it: the
			// quantity being fitted is a single gameweek's appearance, and Score's
			// fixture average over five would be answering a different question.
			// ExpectedMinutes itself is horizon-independent, but building the engine
			// the way the benchmark does keeps the two instruments on one object.
			w := cfg.Weights
			w.Horizon = 1
			e := analysis.NewEngineFull(boot, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = idx
			e.Recent = newRecentIndexWith(cur, c, w.MinutesHalfLife, w.RateHalfLife)

			for i := range boot.Elements {
				el := &boot.Elements[i]
				p := cur.Players[el.ID]
				if p == nil {
					continue
				}
				gws := single[p.Team]
				// The same five-gameweek history bar winRowsFor applies, so the two
				// populations differ in their predictor and not in their membership.
				var hist float64
				for _, gw := range gws {
					if gw <= c {
						hist++
					}
				}
				if hist < 5 {
					continue
				}
				var app, sixty, n float64
				for _, gw := range gws {
					if gw <= c || n >= float64(ahead) {
						continue
					}
					n++
					g, ok := p.GWs[gw]
					if !ok {
						continue
					}
					if g.Minutes >= 1 {
						app++
					}
					if g.Minutes >= 60 {
						sixty++
					}
				}
				if n < float64(ahead) {
					continue
				}
				m := e.Metrics(el)
				out = append(out, winRow{season: cur.Name, m: m.ExpectedMinutes,
					s: m.StartShare, app: app / n, sixty: sixty / n})
			}
		}
	}
	return out
}

// fitNoStarts fits the two shipped forms with free constants and no start-share
// term, over whatever rows it is given. It is the same search fitAll runs for its
// `refit` column, lifted out so section 6 can run it on a different x-axis without
// duplicating the boxes — two copies of a fitter is two fitters that can disagree.
func fitNoStarts(fr []winRow) (pApp, pSix [3]float64) {
	pApp = refineDiag([3]float64{1, -0.5, 0}, [3]float64{60, 1.5, 0}, 24, 4,
		func(p [3]float64) float64 {
			f, ss := identity(p[0], p[1], 0), 0.0
			for _, r := range fr {
				d := f(r.m, r.s) - r.app
				ss += d * d
			}
			return ss
		})
	pSix = refineDiag([3]float64{0.01, 10, 0}, [3]float64{0.40, 90, 0}, 24, 4,
		func(p [3]float64) float64 {
			f, ss := boundedLogit(p[0], p[1], 0), 0.0
			for _, r := range fr {
				d := f(r.m, r.s) - r.sixty
				ss += d * d
			}
			return ss
		})
	return pApp, pSix
}

// reportExpectedMinutesFit scores three curves on the axis the model actually uses:
// the shipped one, the refit fitted on the windowed proxy, and a refit fitted on
// `ExpectedMinutes` itself.
//
// The middle column is the one to read. If the windowed refit beats shipped here
// too, the recorded 2.0%/3.9% carries to the real consumer and the case for running
// it through the replay is straightforward. If it loses here while winning on its
// own axis, that is the over-hedging appearance.go predicts, measured — and the
// third column says how much of the gain survives once the fit is done properly.
func reportExpectedMinutesFit(rows []winRow, pAppWin, pSixWin [3]float64) {
	fmt.Printf("\n=== 6. the same forms fitted against ExpectedMinutes, the predictor the model uses\n")
	fmt.Printf("%d rows. Leave-one-season-out, so the ExpectedMinutes column is not flattered\n",
		len(rows))
	fmt.Printf("by being scored on its own fit.\n\n")

	seasons := map[string]bool{}
	for _, r := range rows {
		seasons[r.season] = true
	}
	var names []string
	for s := range seasons {
		names = append(names, s)
	}
	sort.Strings(names)

	shipSlope, shipMid, shipIntercept, shipCondSlope := analysis.ShippedAppearanceFit()
	shipApp := identity(shipIntercept, shipCondSlope, 0)
	shipSix := boundedLogit(shipSlope, shipMid, 0)
	winApp := identity(pAppWin[0], pAppWin[1], 0)
	winSix := boundedLogit(pSixWin[0], pSixWin[1], 0)

	fmt.Printf("%-12s %7s | %-27s | %-27s\n", "holdout", "n", "P(appears) rms", "P(60+) rms")
	fmt.Printf("%-12s %7s | %8s %8s %8s | %8s %8s %8s\n",
		"", "", "shipped", "windowed", "on E[m]", "shipped", "windowed", "on E[m]")

	var A0, A1, A2, S0, S1, S2, N float64
	for _, h := range names {
		var fr, ho []winRow
		for _, r := range rows {
			if r.season == h {
				ho = append(ho, r)
			} else {
				fr = append(fr, r)
			}
		}
		pApp, pSix := fitNoStarts(fr)
		emApp := identity(pApp[0], pApp[1], 0)
		emSix := boundedLogit(pSix[0], pSix[1], 0)

		var a0, a1, a2, s0, s1, s2, n float64
		for _, r := range ho {
			d := shipApp(r.m, r.s) - r.app
			a0 += d * d
			d = winApp(r.m, r.s) - r.app
			a1 += d * d
			d = emApp(r.m, r.s) - r.app
			a2 += d * d
			d = shipSix(r.m, r.s) - r.sixty
			s0 += d * d
			d = winSix(r.m, r.s) - r.sixty
			s1 += d * d
			d = emSix(r.m, r.s) - r.sixty
			s2 += d * d
			n++
		}
		q := func(v float64) float64 { return math.Sqrt(v / n) }
		fmt.Printf("%-12s %7.0f | %8.4f %8.4f %8.4f | %8.4f %8.4f %8.4f\n",
			h, n, q(a0), q(a1), q(a2), q(s0), q(s1), q(s2))
		A0, A1, A2 = A0+a0, A1+a1, A2+a2
		S0, S1, S2 = S0+s0, S1+s1, S2+s2
		N += n
	}
	q := func(v float64) float64 { return math.Sqrt(v / N) }
	fmt.Printf("%-12s %7.0f | %8.4f %8.4f %8.4f | %8.4f %8.4f %8.4f\n",
		"POOLED", N, q(A0), q(A1), q(A2), q(S0), q(S1), q(S2))
	fmt.Printf("\nagainst shipped, on this axis: windowed refit appears %+.1f%%, sixty %+.1f%%\n",
		100*(1-q(A1)/q(A0)), 100*(1-q(S1)/q(S0)))
	fmt.Printf("                               refit on E[m]  appears %+.1f%%, sixty %+.1f%%\n",
		100*(1-q(A2)/q(A0)), 100*(1-q(S2)/q(S0)))

	// The constants to run, fitted on everything. In-sample, like section 5, and for
	// the same reason: the table above is the evidence, this is the thing to paste.
	pApp, pSix := fitNoStarts(rows)
	fmt.Printf("\nFPL_APPEARANCE_FIT=\"%.4f,%.2f,%.2f,%.4f\"   (fitted on ExpectedMinutes)\n",
		pSix[0], pSix[1], pApp[0], pApp[1])
	fmt.Printf("\nNeither refit is a verdict. Lower prediction error is a candidate worth\n")
	fmt.Printf("spending replay time on, and this project has a recorded case where 2%% lower\n")
	fmt.Printf("out-of-sample error cost about 49 points a season.\n")
}

// refineDiag is a coarse-to-fine search over a three-parameter box: `passes` passes,
// each quartering the box around the current best. A full fine grid over three
// parameters costs tens of minutes here; this costs seconds, and on a smooth
// least-squares surface the two agree. A zero-width dimension is held fixed, which
// is how the no-start-share control is fitted by the same code as the candidate.
func refineDiag(lo, hi [3]float64, steps, passes int, ss func([3]float64) float64) [3]float64 {
	var best [3]float64
	for i := range best {
		best[i] = (lo[i] + hi[i]) / 2
	}
	bestV := math.Inf(1)
	for pass := 0; pass < passes; pass++ {
		var step [3]float64
		for i := range step {
			step[i] = (hi[i] - lo[i]) / float64(steps)
		}
		for a := 0; a <= steps; a++ {
			for b := 0; b <= steps; b++ {
				for c := 0; c <= steps; c++ {
					p := [3]float64{
						lo[0] + step[0]*float64(a),
						lo[1] + step[1]*float64(b),
						lo[2] + step[2]*float64(c),
					}
					if v := ss(p); v < bestV {
						bestV, best = v, p
					}
					if step[2] == 0 {
						break
					}
				}
				if step[1] == 0 {
					break
				}
			}
			if step[0] == 0 {
				break
			}
		}
		for i := range lo {
			w := (hi[i] - lo[i]) / 4
			lo[i], hi[i] = best[i]-w, best[i]+w
		}
	}
	return best
}

// boundedLogit generalises the shipped playsSixty shape: a logistic in
// (mean minutes + c x start share), held between the same two exact bounds. c = 0 is
// the shipped form with free constants, which is the control that separates "a
// better curve" from "a better predictor".
func boundedLogit(k, m0, c float64) func(m, s float64) float64 {
	return func(m, s float64) float64 {
		if m <= 0 {
			return 0
		}
		p := 1 / (1 + math.Exp(-k*(m+c*s-m0)))
		if hi := m / 60; hi < p {
			p = hi
		}
		if lo := (m - 60) / 30; lo > p {
			p = lo
		}
		return clampDiag(p, 0, 1)
	}
}

// identity generalises the shipped playsAtAll shape: P = m / E[m | appears], with
// the conditional mean linear in mean minutes and start share. It is exactly zero at
// zero mean minutes for ANY coefficients, which is the property research_targets
// depends on and the reason this form is preferred to a logistic for appearance.
func identity(a, b, c float64) func(m, s float64) float64 {
	return func(m, s float64) float64 {
		if m <= 0 {
			return 0
		}
		return clampDiag(m/clampDiag(a+b*m+c*s, 1, 90), 0, 1)
	}
}

func TestDiagStartShare(t *testing.T) {
	requireDiag(t)
	cutoffs := []int{6, 10, 14, 18, 22, 26, 30}
	const ahead = 5
	rows := winRowsFor(t, cutoffs, ahead)
	if len(rows) < 5000 {
		t.Skipf("only %d windowed rows", len(rows))
	}

	// -----------------------------------------------------------------
	// 1. The two estimators of P(appears) that used to run side by side.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== 1. the two estimators of P(appears), before they were unified\n")
	fmt.Printf("Both claimed to be P(records at least one minute). Nothing required them to agree.\n\n")
	fmt.Printf("%-44s %12s %12s %8s\n", "player", "1-blankRate", "playsAtAll", "gap")
	for _, c := range []struct {
		label string
		m, s  float64
	}{
		{"ever-present, 90 minutes, start share 1.0", 90, 1.0},
		{"nailed starter withdrawn at 85, share 1.0", 85, 1.0},
		{"starts half the matches, plays 90", 45, 0.5},
		{"never starts, always on for 45", 45, 0.0},
		{"fringe: 2 mean minutes, start share 0.02", 2, 0.02},
	} {
		a, b := 1-shippedBlank(c.s), shippedAtAll(c.m)
		fmt.Printf("%-44s %12.3f %12.3f %8.3f\n", c.label, a, b, math.Abs(a-b))
	}
	var gap, gmax, bb, ab, brms, arms, n float64
	for _, r := range rows {
		g := math.Abs((1 - shippedBlank(r.s)) - shippedAtAll(r.m))
		gap += g
		if g > gmax {
			gmax = g
		}
		d := (1 - shippedBlank(r.s)) - r.app
		bb += d
		brms += d * d
		d = shippedAtAll(r.m) - r.app
		ab += d
		arms += d * d
		n++
	}
	fmt.Printf("\nover %.0f windowed rows: mean |gap| %.4f, worst %.4f\n", n, gap/n, gmax)
	fmt.Printf("against the realised appearance rate:\n")
	fmt.Printf("  1 - blankRate  bias %+.4f  rms %.4f\n", bb/n, math.Sqrt(brms/n))
	fmt.Printf("  playsAtAll     bias %+.4f  rms %.4f\n", ab/n, math.Sqrt(arms/n))
	fmt.Printf("The start-share estimator is the worse one on both counts, and its constant\n")
	fmt.Printf("(0.624) was fitted only over start share 0.70+. That is why the unified\n")
	fmt.Printf("estimator is the minutes one rather than a compromise between the two.\n")

	// -----------------------------------------------------------------
	// 2. The cheap kill: hold mean minutes fixed, split by start share.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== 2. hold mean minutes fixed, split by start-share tercile\n")
	fmt.Printf("The trap: this looks like it isolates ordering error and does not. Start share\n")
	fmt.Printf("and mean minutes stay correlated INSIDE a band, so the predicted columns move\n")
	fmt.Printf("too. Compare the two spreads down each block, not the measured one alone.\n\n")
	fmt.Printf("%-14s %-16s %6s %9s %9s %9s %9s\n",
		"mean minutes", "start share", "n", "app meas", "app pred", "60 meas", "60 pred")
	for _, mb := range [][2]float64{{10, 25}, {25, 40}, {40, 55}, {55, 70}, {70, 85}} {
		var ss []float64
		for _, r := range rows {
			if r.m >= mb[0] && r.m < mb[1] {
				ss = append(ss, r.s)
			}
		}
		if len(ss) < 60 {
			continue
		}
		sort.Float64s(ss)
		q1, q3 := ss[len(ss)/3], ss[2*len(ss)/3]
		for _, sb := range [][2]float64{{-1, q1}, {q1, q3}, {q3, 2}} {
			var am, ap, xm, xp, k float64
			for _, r := range rows {
				if r.m >= mb[0] && r.m < mb[1] && r.s >= sb[0] && r.s < sb[1] {
					am += r.app
					ap += shippedAtAll(r.m)
					xm += r.sixty
					xp += shippedSixty(r.m)
					k++
				}
			}
			if k >= 20 {
				fmt.Printf("%5.0f - %-6.0f %.2f - %-10.2f %6.0f %9.3f %9.3f %9.3f %9.3f\n",
					mb[0], mb[1], math.Max(0, sb[0]), math.Min(1, sb[1]), k,
					am/k, ap/k, xm/k, xp/k)
			}
		}
	}
	var sm, sq, mm, mq, ms, k float64
	for _, r := range rows {
		sm += r.s
		sq += r.s * r.s
		mm += r.m
		mq += r.m * r.m
		ms += r.m * r.s
		k++
	}
	corr := (k*ms - mm*sm) / math.Sqrt((k*mq-mm*mm)*(k*sq-sm*sm))
	fmt.Printf("\ncorr(mean minutes, start share) = %.4f\n", corr)
	fmt.Printf("That is the mechanism for everything below, and for why the table above is not\n")
	fmt.Printf("the clean split it looks like: the two statistics are very nearly the same\n")
	fmt.Printf("variable in the population the model faces.\n")

	// -----------------------------------------------------------------
	// 3. Does it predict better? Leave one season out.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== 3. leave-one-season-out on the windowed task\n")
	fmt.Printf("Fitted on the other two recorded-starts seasons; 2022-23 is excluded from every\n")
	fmt.Printf("fit because its starts are reconstructed FROM minutes, which would be circular.\n")
	fmt.Printf("`refit` is the shipped functional form with free constants and NO start share.\n\n")
	fmt.Printf("%-12s %6s | %-26s | %-26s\n", "holdout", "n", "P(appears) rms", "P(60+) rms")
	fmt.Printf("%-12s %6s | %8s %8s %8s | %8s %8s %8s\n",
		"", "", "shipped", "refit", "+starts", "shipped", "refit", "+starts")

	seasons := []string{"2023-24", "2024-25", "2025-26"}
	pick := func(names ...string) []winRow {
		var o []winRow
		for _, r := range rows {
			for _, nm := range names {
				if r.season == nm {
					o = append(o, r)
				}
			}
		}
		return o
	}
	// pApp and pSix are the fitted parameters of the no-start-share refits, in the
	// forms' own argument order: identity(intercept, slope, _) and
	// boundedLogit(slope, midpoint, _). They are returned because a refit nobody can
	// read the constants of is a refit nobody can run — section 5 below prints them
	// in FPL_APPEARANCE_FIT's order, which is the whole point of the exercise.
	fitAll := func(fr []winRow) (a1, a2, s1, s2 func(m, s float64) float64,
		cApp, cSix float64, pApp, pSix [3]float64) {
		i1 := refineDiag([3]float64{1, -0.5, 0}, [3]float64{60, 1.5, 0}, 24, 4, func(p [3]float64) float64 {
			f, ss := identity(p[0], p[1], 0), 0.0
			for _, r := range fr {
				d := f(r.m, r.s) - r.app
				ss += d * d
			}
			return ss
		})
		i2 := refineDiag([3]float64{1, -0.5, -60}, [3]float64{60, 1.5, 60}, 16, 4, func(p [3]float64) float64 {
			f, ss := identity(p[0], p[1], p[2]), 0.0
			for _, r := range fr {
				d := f(r.m, r.s) - r.app
				ss += d * d
			}
			return ss
		})
		l1 := refineDiag([3]float64{0.01, 10, 0}, [3]float64{0.40, 90, 0}, 24, 4, func(p [3]float64) float64 {
			f, ss := boundedLogit(p[0], p[1], 0), 0.0
			for _, r := range fr {
				d := f(r.m, r.s) - r.sixty
				ss += d * d
			}
			return ss
		})
		l2 := refineDiag([3]float64{0.01, 10, -80}, [3]float64{0.40, 90, 80}, 16, 4, func(p [3]float64) float64 {
			f, ss := boundedLogit(p[0], p[1], p[2]), 0.0
			for _, r := range fr {
				d := f(r.m, r.s) - r.sixty
				ss += d * d
			}
			return ss
		})
		return identity(i1[0], i1[1], 0), identity(i2[0], i2[1], i2[2]),
			boundedLogit(l1[0], l1[1], 0), boundedLogit(l2[0], l2[1], l2[2]),
			i2[2], l2[2], i1, l1
	}

	var A0, A1, A2, S0, S1, S2, N float64
	for _, h := range seasons {
		var fr []winRow
		for _, o := range seasons {
			if o != h {
				fr = append(fr, pick(o)...)
			}
		}
		fa1, fa2, fs1, fs2, cA, cS, _, _ := fitAll(fr)
		var a0, a1, a2, s0, s1, s2, m float64
		for _, r := range pick(h) {
			d := shippedAtAll(r.m) - r.app
			a0 += d * d
			d = fa1(r.m, r.s) - r.app
			a1 += d * d
			d = fa2(r.m, r.s) - r.app
			a2 += d * d
			d = shippedSixty(r.m) - r.sixty
			s0 += d * d
			d = fs1(r.m, r.s) - r.sixty
			s1 += d * d
			d = fs2(r.m, r.s) - r.sixty
			s2 += d * d
			m++
		}
		q := func(v float64) float64 { return math.Sqrt(v / m) }
		fmt.Printf("%-12s %6.0f | %8.4f %8.4f %8.4f | %8.4f %8.4f %8.4f   (start-share coeffs %+.0f, %+.0f)\n",
			h, m, q(a0), q(a1), q(a2), q(s0), q(s1), q(s2), cA, cS)
		A0 += a0
		A1 += a1
		A2 += a2
		S0 += s0
		S1 += s1
		S2 += s2
		N += m
	}
	q := func(v float64) float64 { return math.Sqrt(v / N) }
	fmt.Printf("%-12s %6.0f | %8.4f %8.4f %8.4f | %8.4f %8.4f %8.4f\n",
		"POOLED", N, q(A0), q(A1), q(A2), q(S0), q(S1), q(S2))
	appGain := 100 * (1 - q(A2)/q(A1))
	sixGain := 100 * (1 - q(S2)/q(S1))
	fmt.Printf("\nrefitting the shipped forms, no new input: appears %+.1f%%, sixty %+.1f%%\n",
		100*(1-q(A1)/q(A0)), 100*(1-q(S1)/q(S0)))
	fmt.Printf("adding start share on top of that:        appears %+.2f%%, sixty %+.2f%%\n",
		appGain, sixGain)
	fmt.Printf("\nNote the sign on P(appears): refitting a LOGISTIC there loses, because the\n")
	fmt.Printf("shipped form is an identity and a logistic cannot reproduce its exact zero.\n")
	fmt.Printf("`refit` above is therefore the identity with free constants, not a logistic.\n")

	// -----------------------------------------------------------------
	// 4. Held out on the reconstructed season.
	// -----------------------------------------------------------------
	fa1, fa2, fs1, fs2, _, _, _, _ := fitAll(pick(seasons...))
	var r0, r1, r2, t0, t1, t2, rn, nrec float64
	for _, r := range pick("2022-23") {
		if r.recon {
			nrec++
		}
		d := shippedAtAll(r.m) - r.app
		r0 += d * d
		d = fa1(r.m, r.s) - r.app
		r1 += d * d
		d = fa2(r.m, r.s) - r.app
		r2 += d * d
		d = shippedSixty(r.m) - r.sixty
		t0 += d * d
		d = fs1(r.m, r.s) - r.sixty
		t1 += d * d
		d = fs2(r.m, r.s) - r.sixty
		t2 += d * d
		rn++
	}
	rq := func(v float64) float64 { return math.Sqrt(v / rn) }
	fmt.Printf("\n=== 4. held out on 2022-23, whose starts are reconstructed by rank from minutes\n")
	fmt.Printf("%.0f rows, %.0f of them containing a reconstructed start\n", rn, nrec)
	fmt.Printf("appears shipped %.4f refit %.4f +starts %.4f | sixty shipped %.4f refit %.4f +starts %.4f\n",
		rq(r0), rq(r1), rq(r2), rq(t0), rq(t1), rq(t2))
	fmt.Printf("The reconstructed season behaves like the recorded ones, so the repair is not\n")
	fmt.Printf("distorting the comparison — it is simply not fit on.\n")

	// -----------------------------------------------------------------
	// 5. The refit as four numbers you can actually run.
	// -----------------------------------------------------------------
	//
	// Sections 3 and 4 establish that refitting the shipped forms is worth several
	// times what the rejected start-share input was worth, and then throw the
	// constants away. That is the orphaned-measurement shape this project keeps
	// hitting: a figure in prose with no code that can recompute it, and in this case
	// no way to run the thing the figure is about.
	//
	// **All four seasons are fitted here, where section 3 excludes 2022-23.** That
	// exclusion is about start share — 2022-23's starts are reconstructed from
	// minutes, so fitting a start-share coefficient on them is circular. This fit has
	// no start-share coefficient, reads only mean minutes and realised appearance,
	// and the circularity therefore does not arise. Using the season is the right
	// call and it is not the same call section 3 made, so it is stated rather than
	// left to be noticed.
	fmt.Printf("\n=== 5. the refit as constants, fitted on all %s\n",
		seasonsLabel(len(appearanceFitSeasons)))
	_, _, _, _, _, _, pApp, pSix := fitAll(rows)
	// Read from the package rather than restated, so this line cannot claim a
	// shipped value the model does not hold — the two-defaults-for-one-quantity
	// failure, which this project has shipped once already.
	shipSlope, shipMid, shipIntercept, shipCondSlope := analysis.ShippedAppearanceFit()
	fmt.Printf("shipped:  sixty_slope %.4f  sixty_midpoint %.2f  cond_intercept %.2f  cond_slope %.4f\n",
		shipSlope, shipMid, shipIntercept, shipCondSlope)
	fmt.Printf("refit:    sixty_slope %.4f  sixty_midpoint %.2f  cond_intercept %.2f  cond_slope %.4f\n",
		pSix[0], pSix[1], pApp[0], pApp[1])
	fmt.Printf("\nFPL_APPEARANCE_FIT=\"%.4f,%.2f,%.2f,%.4f\"\n", pSix[0], pSix[1], pApp[0], pApp[1])
	fmt.Printf("\nThat is an in-sample fit and is NOT the evidence — sections 3 and 4 are, and\n")
	fmt.Printf("they are leave-one-season-out. This exists so the winning form can be run\n")
	fmt.Printf("through the prediction benchmark, which scores it against the predictor the\n")
	fmt.Printf("model actually feeds in rather than against the windowed proxy fitted here.\n")

	// -----------------------------------------------------------------
	// 6. The same refit against the predictor the model actually uses.
	// -----------------------------------------------------------------
	//
	// This is the section that answers appearance.go's stated reason for not acting
	// on section 3. Every fit above is against a *windowed proxy*: mean minutes over
	// the season to a cutoff, computed from raw archive rows. What playsSixty and
	// playsAtAll are evaluated at in production is `ExpectedMinutes` — blended
	// against a multi-season prior, recency-weighted at MinutesHalfLife, and rested.
	//
	// The distinction is not pedantic and its direction is predictable. A curve
	// fitted against a noisier predictor than the one it will be evaluated at is
	// over-hedged: least squares flattens it, because a flatter curve is the right
	// answer to a predictor carrying more error. So the windowed refit is expected to
	// be too flat for this consumer, and the honest way to find out is to fit against
	// the real thing.
	//
	// Same targets, same forms, same fitter. The only change is the x-axis.
	emRows := expectedMinutesRowsFor(t, cutoffs, ahead)
	if len(emRows) < 2000 {
		fmt.Printf("\n=== 6. skipped: only %d ExpectedMinutes rows\n", len(emRows))
	} else {
		reportExpectedMinutesFit(emRows, pApp, pSix)
	}

	// A null nobody can re-check is prose. Fail if start share ever starts paying, so
	// the finding stays a live claim rather than a frozen comment.
	if appGain > 1.0 || sixGain > 1.0 {
		t.Errorf("start share now buys %+.2f%% on P(appears) and %+.2f%% on P(60+) over the "+
			"same forms refitted without it. The recorded finding is that it buys under 1%% "+
			"on both, which is why the start/substitute/unused multinomial was measured and "+
			"NOT shipped. If this fires, re-read the finding rather than the test.",
			appGain, sixGain)
	}
}
