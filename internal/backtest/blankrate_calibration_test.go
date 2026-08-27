package backtest

// What is the chance a player records no minutes at all this week?
//
// The bench exists for that event and nothing in the model estimates it. The
// slot weights are a fixed tuple standing in for "how often is this slot
// reached", which is a property of the eleven in front of it — a bench behind
// eleven ever-presents is worth much less than the same bench behind a fragile
// side, and one constant cannot say so.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBlankRate -v
//
// The model has StartShare — starts over matches available — and no appearance
// count, so the question is what StartShare implies about blanking. Not
// starting is not the same as not playing: a squad player who comes off the
// bench records minutes and cannot be autosubbed for. This measures the gap
// rather than assuming it.

import (
	"context"
	"fmt"
	"math"
	"testing"
)

func TestDiagBlankRate(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	// Buckets of start share, since the relationship need not be linear.
	type bucket struct {
		lo, hi          float64
		n               float64
		blank, notStart float64
		subbedOn        float64
	}
	buckets := []*bucket{
		{lo: 0.00, hi: 0.20}, {lo: 0.20, hi: 0.40}, {lo: 0.40, hi: 0.60},
		{lo: 0.60, hi: 0.75}, {lo: 0.75, hi: 0.85}, {lo: 0.85, hi: 0.95},
		{lo: 0.95, hi: 1.01},
	}

	// Named so the header below counts this list rather than restating its
	// length — the two halves of a printed label must not be able to disagree.
	seasons := []string{"2022-23", "2023-24", "2024-25", "2025-26"}

	var xs, ys []float64
	for _, sn := range seasons {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		// How many gameweeks the season actually ran, so a player who joined in
		// January is not counted as having blanked the autumn.
		maxGW := 0
		for _, p := range s.Players {
			for gw := range p.GWs {
				if gw > maxGW {
					maxGW = gw
				}
			}
		}
		for _, p := range s.Players {
			if len(p.GWs) < maxGW/2 {
				continue // partial season: not in the league throughout
			}
			var blanks, starts, played, n float64
			for gw := 1; gw <= maxGW; gw++ {
				g, ok := p.GWs[gw]
				if !ok {
					continue
				}
				n++
				if g.Minutes == 0 {
					blanks++
					continue
				}
				played++
				if g.Starts > 0 {
					starts++
				}
			}
			if n < 20 {
				continue
			}
			ss, br := starts/n, blanks/n
			xs = append(xs, 1-ss)
			ys = append(ys, br)
			for _, b := range buckets {
				if ss >= b.lo && ss < b.hi {
					b.n++
					b.blank += br
					b.notStart += 1 - ss
					b.subbedOn += (played - starts) / n
				}
			}
		}
	}

	fmt.Printf("\nBlank rate against start share, %s, players present\n", seasonsLabel(len(seasons)))
	fmt.Printf("for at least half the season and 20+ gameweeks.\n\n")
	fmt.Printf("%-14s %6s %10s %10s %10s %8s\n",
		"start share", "n", "not-start", "blank", "sub apps", "blank/ns")
	for _, b := range buckets {
		if b.n == 0 {
			continue
		}
		ns, bl := b.notStart/b.n, b.blank/b.n
		ratio := 0.0
		if ns > 0 {
			ratio = bl / ns
		}
		fmt.Printf("%.2f - %.2f    %6.0f %10.3f %10.3f %10.3f %8.3f\n",
			b.lo, b.hi, b.n, ns, bl, b.subbedOn/b.n, ratio)
	}

	// A single ratio through the origin is the form the model would use:
	// blank = c * (1 - startShare). Fitted by least squares with no intercept,
	// because a player who starts everything blanks nothing by construction.
	var num, den float64
	for i := range xs {
		num += xs[i] * ys[i]
		den += xs[i] * xs[i]
	}
	c := num / den
	var ssRes, ssTot, meanY float64
	for _, y := range ys {
		meanY += y
	}
	meanY /= float64(len(ys))
	for i := range xs {
		r := ys[i] - c*xs[i]
		ssRes += r * r
		ssTot += (ys[i] - meanY) * (ys[i] - meanY)
	}
	fmt.Printf("\nblank = c x (1 - start share):  c = %.3f   R2 = %.3f   n = %d\n",
		c, 1-ssRes/ssTot, len(xs))
	fmt.Printf("residual sd %.3f\n", math.Sqrt(ssRes/float64(len(xs))))

	// The regime that matters is the eleven, which is made of near-starters.
	// A fit dominated by fringe players would be calibrated where it is never
	// used, so this reports the same constant on that subset alone.
	num, den = 0, 0
	nHigh := 0
	for i := range xs {
		if 1-xs[i] < 0.70 {
			continue
		}
		num += xs[i] * ys[i]
		den += xs[i] * xs[i]
		nHigh++
	}
	if den > 0 {
		fmt.Printf("start share >= 0.70 only:       c = %.3f              n = %d\n",
			num/den, nHigh)
	}
}
