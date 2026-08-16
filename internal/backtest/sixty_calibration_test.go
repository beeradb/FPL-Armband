package backtest

// How often does a player reach 60 minutes?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagSixtyMinutes -v
//
// Two of FPL's scoring terms are step functions at 60 minutes and not rates.
// Appearance points are 1 below it and 2 at or above; the clean sheet pays 4 to
// a defender or keeper and 1 to a midfielder at 60+, and nothing below. A
// player taken off at 70 banks all of both.
//
// The model *used to* price them per 90 and multiply the whole per-90 figure by
// minutes reliability, so that player was credited about 0.73 of each. This
// diagnostic was written to justify replacing that with a fitted P(he reaches 60
// minutes), and `playsSixty` is the result: `thresholdXP90` now splits the two
// step terms out and scales them by it.
//
// Mean minutes per gameweek is the right predictor because it is exactly what
// blend.MinutesPerMatch carries — the same quantity, including the weeks he did
// not play, and already recency-weighted and blended against his prior.
//
// # What this got wrong for as long as it has existed, and the rule it breaks
//
// The column this printed as "model now" was `(minutes/90)^MinutesWeight` — the
// reliability proxy `playsSixty` **replaced** — and it kept printing it under that
// name after the replacement shipped. Its error was read into the accuracy
// snapshot as a property of the shipped model, and the snapshot's own commentary
// drew an ordering conclusion from it: that the model "mis-ranks a part-timer
// against an ever-present". That conclusion was about a curve the model had
// stopped using.
//
// The rule it breaks is one this project already names as its signature failure:
// **one quantity with two implementations, where the measured one is not the one
// that runs.** It has bitten in `DefaultBenchWeight` against `Weights.BenchWeight`,
// in the two estimators of P(appears), and here — this time inside a diagnostic,
// which is the worst place for it, because a diagnostic is what everything else is
// checked against.
//
// So the shipped curve is now read from the package through `analysis.PlaysSixty`
// rather than reimplemented, and the superseded proxy is still printed beside it —
// it is genuinely informative to see what the change bought — but labelled as
// superseded. **Do not reintroduce a local copy of a curve this file is checking.**
//
// Read the shipped column, and read it as a shape rather than a level: the error
// crosses zero around fifty minutes, so it is not a uniform bias that an argmax
// would ignore.

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagSixtyMinutes(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	type bucket struct {
		lo, hi float64
		n      float64
		sixty  float64
		mean   float64
		// shipped is what playsSixty credits, read from the package. superseded is
		// the minutes-reliability proxy it replaced, kept for the comparison and
		// named so nobody reads it as the model again.
		shipped    float64
		superseded float64
	}
	buckets := []*bucket{
		{lo: 0, hi: 10}, {lo: 10, hi: 20}, {lo: 20, hi: 30}, {lo: 30, hi: 40},
		{lo: 40, hi: 50}, {lo: 50, hi: 60}, {lo: 60, hi: 70}, {lo: 70, hi: 80},
		{lo: 80, hi: 91},
	}

	type obs struct{ mean, sixty float64 }
	var all []obs

	// Named so the snapshot's grid string below counts the population that ran.
	// needsSweep grows by a season every summer, and a literal in a string that
	// reaches the accuracy snapshot is the mislabel this package has already paid
	// for once.
	seasons := playedSeasons(needsSweep)

	for _, sn := range seasons {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		maxGW := 0
		for _, p := range s.Players {
			for gw := range p.GWs {
				if gw > maxGW {
					maxGW = gw
				}
			}
		}
		for _, p := range s.Players {
			var mins, sixty, n float64
			for gw := 1; gw <= maxGW; gw++ {
				g, ok := p.GWs[gw]
				if !ok {
					continue
				}
				n++
				mins += float64(g.Minutes)
				if g.Minutes >= 60 {
					sixty++
				}
			}
			if n < 20 {
				continue
			}
			mpg, rate := mins/n, sixty/n
			all = append(all, obs{mpg, rate})
			for _, b := range buckets {
				if mpg >= b.lo && mpg < b.hi {
					b.n++
					b.sixty += rate
					b.mean += mpg
					// What the model credits now, read from the package rather
					// than from a copy of it, so this line cannot go stale the
					// way the one below it did.
					b.shipped += analysis.PlaysSixty(mpg)
					// What it credited before playsSixty: the minutes share
					// raised to the convexity exponent.
					b.superseded += math.Pow(mpg/90, cfg.Weights.MinutesWeight)
				}
			}
		}
	}

	fmt.Printf("\nP(60+ minutes) against mean minutes per gameweek.\n")
	fmt.Printf("%s, players present for 20+ gameweeks.\n\n", seasonsLabel(len(seasons)))
	fmt.Printf("`shipped` is playsSixty, which is what the model credits. `superseded` is\n")
	fmt.Printf("the minutes-reliability proxy it replaced, kept for the comparison — it is\n")
	fmt.Printf("NOT the model and was printed as though it were for as long as this existed.\n\n")
	fmt.Printf("%-14s %7s %10s %10s %10s %9s %10s\n",
		"mean mins/gw", "n", "mean", "P(60+)", "shipped", "error", "superseded")
	for _, b := range buckets {
		if b.n == 0 {
			continue
		}
		p60, ship, old := b.sixty/b.n, b.shipped/b.n, b.superseded/b.n
		fmt.Printf("%2.0f - %-9.0f %7.0f %10.1f %10.3f %10.3f %+9.3f %10.3f\n",
			b.lo, b.hi, b.n, b.mean/b.n, p60, ship, ship-p60, old)
	}

	// A logistic in mean minutes is the natural one-parameter shape: it is
	// bounded, monotone, and the data is a proportion. Fitted by a coarse grid
	// on the two parameters, which is plenty for a curve this smooth.
	bestA, bestB, bestErr := 0.0, 0.0, math.Inf(1)
	for a := 0.04; a <= 0.30001; a += 0.005 {
		for b := 20.0; b <= 70.0; b += 0.5 {
			var e float64
			for _, o := range all {
				p := 1 / (1 + math.Exp(-a*(o.mean-b)))
				e += (p - o.sixty) * (p - o.sixty)
			}
			if e < bestErr {
				bestA, bestB, bestErr = a, b, e
			}
		}
	}
	fmt.Printf("\nlogistic P(60+) = 1 / (1 + exp(-%.3f x (mins - %.1f)))\n", bestA, bestB)
	fmt.Printf("rms %.4f over %d players\n", math.Sqrt(bestErr/float64(len(all))), len(all))

	fmt.Printf("\n%-14s %12s %12s %12s %12s\n",
		"mean mins/gw", "actual", "refit here", "shipped", "superseded")
	for _, b := range buckets {
		if b.n == 0 {
			continue
		}
		m := b.mean / b.n
		fmt.Printf("%2.0f - %-9.0f %12.3f %12.3f %12.3f %12.3f\n", b.lo, b.hi,
			b.sixty/b.n, 1/(1+math.Exp(-bestA*(m-bestB))), b.shipped/b.n,
			b.superseded/b.n)
		// One row per minutes band, because unlike the clean-sheet bias this error
		// is *not* uniform across players: appearance points and the clean sheet
		// are steps at sixty minutes, so mis-scaling them mis-ranks a part-timer
		// against an ever-present, which is an ordering error an argmax does see.
		//
		// The snapshot takes the SHIPPED column. It used to take the superseded one
		// under the name "model credits", which made a real ordering claim about a
		// curve the model no longer runs; the proxy is still emitted, under a name
		// that says what it is.
		sink.emitAll("sixty_minute_threshold",
			fmt.Sprintf("%d player-seasons, %s, 20+ gameweeks", len(all), seasonsLabel(len(seasons))),
			fmt.Sprintf("players averaging %.0f-%.0f minutes a gameweek", b.lo, b.hi),
			int(b.n),
			measure{"model credits", b.shipped / b.n},
			measure{"actually reached 60 minutes", b.sixty / b.n},
			measure{"error", b.shipped/b.n - b.sixty/b.n},
			measure{"the superseded minutes-reliability proxy credited", b.superseded / b.n})
	}
}
