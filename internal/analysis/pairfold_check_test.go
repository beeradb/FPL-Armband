package analysis

import (
	"math/rand"
	"testing"
)

// TestThePairFoldMatchesSequentialPlay pins the equivalence that
// bestFormation's constant-time fold rests on: a sequence's (captain, vice)
// record, folded through foldPair into any prior state, equals replaying the
// sequence's players one at a time. The ties are the point — the sequential
// update is not a pure top-2 selection, equal scores do not promote, so this
// is checked with heavy ties rather than random floats.
func TestThePairFoldMatchesSequentialPlay(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	vals := func() []float64 {
		n := 2 + rng.Intn(7)
		vs := make([]float64, n)
		for i := range vs {
			vs[i] = float64(rng.Intn(11)) / 2 // 0..5 in halves — plenty of ties
		}
		return vs
	}
	seqFold := func(c, v float64, vs []float64) (float64, float64) {
		for _, x := range vs {
			if x > c {
				c, v = x, c
			} else if x > v {
				v = x
			}
		}
		return c, v
	}
	for trial := 0; trial < 200000; trial++ {
		vs := vals()
		p, q := seqFold(0, 0, vs)
		if p < q {
			t.Fatalf("record not ordered: %v -> (%v,%v)", vs, p, q)
		}
		c := float64(rng.Intn(11)) / 2
		v := float64(rng.Intn(11)) / 2
		if v > c {
			c, v = v, c
		}
		gotC, gotV := foldPair(c, v, p, q)
		wantC, wantV := seqFold(c, v, vs)
		if gotC != wantC || gotV != wantV {
			t.Fatalf("pair fold diverges: seq %v prior (%v,%v): pair (%v,%v) vs sequential (%v,%v)",
				vs, c, v, gotC, gotV, wantC, wantV)
		}
	}
}
