package backtest

// The per-package log's accumulator, and the table it prints.
//
// # What it is for, and why a transfer count is not it
//
// The ANTI-minus-RESIDUAL contrast has a null the design already knows is not
// zero. The two criteria are `{ΔR > 4h}` and `{ΔR < −4h}`, disjoint with a dead
// band of width 8h — and h is zero for the large majority of offered packages, so
// over most of the stream the two arms *partition* it. Writing the effect on
// accumulated xPoints per offered package, with μ the mean underlying gain and p
// the residual arm's accept mass:
//
//	ANTI − RES = −cov(ΔX, sign ΔR) + μ·(1 − 2p)
//
// Only the first term is the quantity the run is for. The second is a nuisance that
// vanishes at p = ½ or μ = 0 and at no other configuration, and it enters
// **additively, in the run's own units**. Reporting `moves` does not repair it,
// because nothing says what to do when the counts differ — and a level bias shows up
// as a *high* `pos`, so neither the concentration screen nor the wild bootstrap can
// see it. It has to be measured.
//
// None of the three quantities is recoverable from a transfer count. Equal counts
// are not the same packages, and this diagnostic has recorded owing exactly this
// statistic since its first re-run, when counts were reported in its place.
//
// # It is an observer and never a decision
//
// `acceptTransfer` calls it after the answer is fixed and nothing branches on what
// it returns, so an arm with the log on plays the identical season to one without.
// That is what lets it run inside the scoring sweep rather than needing a second
// grid — a per-arm replay of its own would be another full sweep for a statistic
// that costs an addition per package.

import (
	"fmt"
	"math"
	"sync"
)

// gateStream accumulates one arm's offered-package statistics.
//
// Sums rather than the packages themselves: the three statistics are all first and
// second moments, a sweep offers of order 10^4 packages an arm, and keeping the rows
// would trade a fixed few words for a slice whose size is a property of the
// experiment.
//
// The mutex is not currently load-bearing — runPolicySweep walks arms and cells
// serially — and it is here because the thing it guards is a plain accumulator
// reached from the simulation's own hot path, which is the shape this project has a
// standing rule about. It costs an uncontended lock per package.
type gateStream struct {
	mu sync.Mutex
	// n is every package offered to the gate, and accepted how many it took.
	n, accepted int
	// free is how many carried no hit, which is where the dead band closes and the
	// two criteria partition the stream.
	free int
	// resSet and antiSet are how many packages meet each criterion, computed here
	// rather than read off `Accepted` on purpose: `Accepted` is what THIS arm
	// answered, and p is a property of the stream rather than of the arm judging it.
	resSet, antiSet int
	// The moments. sign is sign(ΔR), zero at exactly zero.
	sumDX, sumDR, sumSign, sumDXSign float64
}

func (g *gateStream) add(p gatePackage) {
	sign := 0.0
	switch {
	case p.DR > 0:
		sign = 1
	case p.DR < 0:
		sign = -1
	}
	charge := HitCost * float64(p.Hits)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	if p.Accepted {
		g.accepted++
	}
	if p.Hits == 0 {
		g.free++
	}
	if p.DR-charge > 0 {
		g.resSet++
	}
	if -p.DR-charge > 0 {
		g.antiSet++
	}
	g.sumDX += p.DX
	g.sumDR += p.DR
	g.sumSign += sign
	g.sumDXSign += p.DX * sign
}

// packageMass is p, the residual criterion's accept mass over this arm's own
// offered stream — the quantity the contrast's null is built from.
//
// It is a PACKAGE fraction. The mediator's moves(RES)/moves(ACCEPTALL) estimates
// the same thing and is move-weighted, which is not the same number: a funded pair
// is one package and two to five moves, and `decide`'s singles loop returns on its
// first rejection, so a refusing arm forfeits the rest of the week's moves for a
// reason that has nothing to do with how often its criterion says yes.
func (g *gateStream) packageMass() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.n == 0 {
		return 0
	}
	return float64(g.resSet) / float64(g.n)
}

// namedStream is one arm's accumulator with the name it prints under.
//
// A slice built where the arms are declared, rather than a map plus a separate
// hand-written order list. ⚠️ The first version had both, joined by string equality,
// and the consumer skipped a name it could not find — so renaming an arm on one side
// would have printed five rows with no error and no total, in the one output that
// supplies the contrast's offset. That is this package's signature failure in the
// place it can least afford it, and the repair is to have one list rather than to
// remember to update two.
type namedStream struct {
	name   string
	stream *gateStream
}

// reportGateStreams prints the offered-stream statistics and, from them, the two
// terms of the contrast's decomposition.
//
// **The comparison to read is the last two columns.** They are the antisymmetric
// term the run is for and the accept-mass nuisance it is confounded with, in the
// same per-package units, so their ratio says directly whether the offset is
// negligible — without needing the sweep's own scale, and before its means are read.
//
// ⚠️ Per OFFERED PACKAGE, not per gameweek and not per season. The sweep's means are
// points per gameweek played and these are not on that scale; what transports
// between them is the RATIO of the two terms, which is scale-free.
func reportGateStreams(streams []namedStream) {
	fmt.Printf("\nPER-PACKAGE LOG — the offered proposal stream, per arm.\n")
	fmt.Printf("Every package handed to the gate, whatever the gate then said.\n")
	fmt.Printf("`p` is P(dR > 4h) on THIS arm's own stream; `anti` is P(dR < -4h).\n")
	fmt.Printf("They sum to 1 exactly where h = 0 and leave the dead band elsewhere,\n")
	fmt.Printf("so READ THE `free` COLUMN BEFORE ANYTHING ELSE: the partition the\n")
	fmt.Printf("whole decomposition rests on holds only where h = 0. *** The no-gate\n")
	fmt.Printf("arm's `free` is LOW BY CONSTRUCTION *** — moveLimit bounds a week at\n")
	fmt.Printf("free+1 moves and an arm that refuses nothing exhausts it every week,\n")
	fmt.Printf("so it takes a -4 almost every week and its level carries a hit charge\n")
	fmt.Printf("no gated arm pays. Correct T for it with the hit channel, 4*hits/wks.\n\n")
	fmt.Printf("%-11s %8s %7s %7s %7s %7s %9s %9s | %11s %11s %8s\n",
		"arm", "offered", "took", "free", "p", "anti", "mu(dX)", "mean dR",
		"-cov", "mu(1-2p)", "ratio")
	for _, ns := range streams {
		g := ns.stream
		name := ns.name
		if g == nil || g.n == 0 {
			continue
		}
		n := float64(g.n)
		p := float64(g.resSet) / n
		anti := float64(g.antiSet) / n
		mu := g.sumDX / n
		meanSign := g.sumSign / n
		// cov(dX, sign dR) = E[dX * sign dR] - E[dX] * E[sign dR].
		cov := g.sumDXSign/n - mu*meanSign
		info := -cov
		offset := mu * (1 - 2*p)
		ratio := math.NaN()
		if offset != 0 {
			ratio = info / offset
		}
		fmt.Printf("%-11s %8d %7.3f %7.3f %7.4f %7.4f %+9.4f %+9.4f | %+11.4f %+11.4f %8.2f\n",
			name, g.n, float64(g.accepted)/n, float64(g.free)/n, p, anti, mu,
			g.sumDR/n, info, offset, ratio)
	}
	fmt.Printf("\nHOW TO READ THE LAST THREE COLUMNS.\n")
	fmt.Printf("  ANTI - RES = -cov(dX, sign dR) + mu*(1-2p), per offered package.\n")
	fmt.Printf("  The first term is what the run is for; the second is the nuisance\n")
	fmt.Printf("  the accept-all arm exists to identify. `ratio` is the first over\n")
	fmt.Printf("  the second: near zero means the contrast is almost entirely the\n")
	fmt.Printf("  offset and cannot discriminate, and large means the offset is\n")
	fmt.Printf("  negligible and the contrast may be read against ~zero.\n")
	fmt.Printf("  *** STOP AND REPORT, do not read the contrast, if p is far from\n")
	fmt.Printf("  0.5 AND mu is materially positive AND the ratio is near 1 or\n")
	fmt.Printf("  below. *** That is the configuration in which mass asymmetry\n")
	fmt.Printf("  alone manufactures the signature that is supposed to mean dR\n")
	fmt.Printf("  carries information about dX.\n")
	fmt.Printf("  These are PER PACKAGE. The sweep's means are per gameweek played,\n")
	fmt.Printf("  so do not add them together; what transports is the ratio.\n")
	fmt.Printf("  The arms do not share a stream — paths diverge from week one — so\n")
	fmt.Printf("  read the shipped row as the reference and the spread across rows\n")
	fmt.Printf("  as how much the divergence moves the quantities themselves.\n")
	fmt.Printf("  *** `p` HERE IS THE PACKAGE MASS AND IS THE ONE TO QUOTE. *** The\n")
	fmt.Printf("  mediator's moves(RESID)/moves(NOGATE) is a MOVE-weighted proxy for\n")
	fmt.Printf("  it — a funded pair is one package and two to five moves, and the\n")
	fmt.Printf("  singles loop ends a week on its first rejection, so a refusing arm\n")
	fmt.Printf("  loses the rest of the week's moves for reasons unrelated to its\n")
	fmt.Printf("  accept mass. Report both; the null is built from this one.\n")
}
