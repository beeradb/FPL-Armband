package main

import (
	"testing"

	"armband/internal/analysis"
	"armband/internal/backtest"
)

// TestTheChannelArmsAreNotSwapped drives `channelPriors` down both arms and checks
// which prior supplied the minutes.
//
// # Why this test exists
//
// The two arms are one function called twice, which is right — it makes omitting a
// field from one copy unspellable, and `DefCon` was missing from both hand-written
// copies until 2026-08-14 while their control carried it. What the fold cost is
// error-detectability. The literals it replaced named their operands inside a
// struct, so writing the mirror the wrong way round was nearly unspellable too; the
// call sites now differ by ARGUMENT ORDER alone:
//
//	p = graftRates(blended, shipped) // blended minutes, shipped rates
//	p = graftRates(shipped, blended) // shipped minutes, blended rates
//
// Swap them and `go build`, `go vet` and this package's other tests all pass. The
// arithmetic is unchanged and every total is plausible; only the LABELS on the two
// output populations are exchanged. This binary exists to split one setting into a
// minutes channel and a rates channel whose recorded readings differ in size and in
// significance — `+0.00018 (t 0.11)` against `+0.00234 (t 3.28)` in the arm table
// above `currentSeasons`. So a swap does not degrade the answer, it prints each
// population under the other's name, and the arm that carries the effect and the
// arm that does not would trade places.
//
// ⚠️ Not "opposite directions", which an earlier version of this comment said: both
// recorded channel readings are POSITIVE, and `priorFrom`'s note forbids the
// extension in as many words — "do not extend that to 'rates-only is the best arm
// on both' — no whole-field figure for the rates-only arm is published here". The
// opposite-signed pairs in this binary belong to other contrasts: whole-field
// against within-population, and the two halves of the treated population.
//
// # How it detects the swap
//
// `graftRates` takes `Minutes` from its first argument verbatim, so the returned
// minutes name the arm. One player, thin enough to clear the gate, with an older
// season carrying very different minutes and a different rate, so blended and
// shipped cannot coincide on either quantity.
//
// ⚠️ The assertions are bounds, not the blend's own outputs, and that is deliberate.
// A blend of two seasons must land strictly BETWEEN them on both quantities,
// whatever weighting `BlendPriors` uses — so `shipped < blended` separates the arms
// without this test re-deriving the weighting. Asserting the exact 1,600 and 1.4625
// would make a change to `BlendPriors` fail here with a message accusing a swap
// that had not happened, and this test's whole job is to name the swap precisely.
// The two `== shipped` assertions are exact because the shipped values are this
// test's own fixture inputs, not the blend's output.
//
// It is a statement about which prior each arm reads, and deliberately not about
// the blend arithmetic: that is `BlendPriors`' own business and is tested where it
// lives.
func TestTheChannelArmsAreNotSwapped(t *testing.T) {
	const code = 4242
	// 900 minutes clears the gate — non-zero and under the half-season bar — and
	// 0.9 xG per 90 is a round number to read a graft back off.
	const shippedMins, shippedRate = 900, 0.9
	prior := seasonOf(&backtest.Player{ID: 1, Code: code, Minutes: 900, Starts: 10, XG: 9})
	// Both quantities well above the shipped season — 3,000 minutes at 1.8 per 90 —
	// so a blend of the two must exceed shipped on each, and neither arm can land on
	// the shipped figure by coincidence.
	older := seasonOf(&backtest.Player{ID: 1, Code: code, Minutes: 3000, Starts: 33, XG: 60})

	minutesArm := priorFor(t, prior, older, true)
	ratesArm := priorFor(t, prior, older, false)

	if minutesArm.Minutes <= shippedMins {
		t.Errorf("the minutes channel returned %d minutes, want the BLENDED figure, "+
			"which must exceed the shipped %d. Getting the shipped value means the two "+
			"graftRates calls have been swapped: this arm would be reporting the rates "+
			"channel's population under the minutes channel's name.",
			minutesArm.Minutes, shippedMins)
	}
	if ratesArm.Minutes != shippedMins {
		t.Errorf("the rates channel returned %d minutes, want the SHIPPED %d exactly. "+
			"This arm must leave minutes untouched — moving them is the other channel.",
			ratesArm.Minutes, shippedMins)
	}
	// The mirror of the same fact: the minutes arm carries the shipped per-90 and
	// the rates arm the blended one. Compared as rates because graftRates rescales
	// the totals onto whichever minutes base it kept.
	if got := per90(minutesArm.XG, minutesArm.Minutes); !near(got, shippedRate) {
		t.Errorf("the minutes channel grafted an xG rate of %.4f per 90, want the "+
			"SHIPPED %.1f. The arms supply rates from the wrong prior.", got, shippedRate)
	}
	// Positive, not merely "different from shipped": a negative assertion also passes
	// when the rate is 0, which is what BlendPriors returns if the fixture ever stops
	// registering as xG-capable — a silently inert leg.
	if got := per90(ratesArm.XG, ratesArm.Minutes); got <= shippedRate {
		t.Errorf("the rates channel returned an xG rate of %.4f per 90, want the "+
			"BLENDED figure, which must exceed the shipped %.1f. At or below it the arm "+
			"is holding the wrong half fixed, and exactly 0 means the fixture stopped "+
			"carrying xG and this leg is measuring nothing.", got, shippedRate)
	}
}

// priorFor runs one arm of channelPriors and returns the single player's prior.
func priorFor(t *testing.T, prior, older *backtest.Season, minutesChannel bool) *analysis.PriorPlayer {
	t.Helper()
	idx := channelPriors(nil, prior, []*backtest.Season{older}, 1, minutesChannel)
	p, ok := idx.Get(4242)
	if !ok || p == nil {
		t.Fatalf("channelPriors returned no prior for the test player, so neither arm "+
			"ran and the swap this test guards would be invisible (minutesChannel=%v)",
			minutesChannel)
	}
	return p
}

// seasonOf wraps players in the minimum Season the prior path reads: the map keyed
// by element id, from which ByCode and the capability probes work.
func seasonOf(ps ...*backtest.Player) *backtest.Season {
	s := &backtest.Season{Players: map[int]*backtest.Player{}}
	for _, p := range ps {
		s.Players[p.ID] = p
	}
	return s
}

func per90(total float64, minutes int) float64 {
	if minutes <= 0 {
		return 0
	}
	return total / float64(minutes) * 90
}

func near(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }
