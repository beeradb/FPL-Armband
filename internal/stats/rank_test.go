package stats

import "testing"

func TestRanksAverageTies(t *testing.T) {
	// The case the diagnostics actually hit: small integers with heavy ties.
	got := Ranks([]float64{1, 1, 1, 5})
	want := []float64{1, 1, 1, 3} // three-way tie at positions 0,1,2 -> mean 1
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Ranks tie handling: got %v want %v", got, want)
		}
	}
	if r := Ranks([]float64{3, 1, 2}); r[0] != 2 || r[1] != 0 || r[2] != 1 {
		t.Errorf("Ranks without ties: %v", r)
	}
}

// ⚠️ The regression this package exists to prevent: arbitrary tie-breaking
// attenuates rho, and does so MORE the more tied the data are. A perfectly
// monotone relationship with ties must still read 1.0.
func TestSpearmanIsNotAttenuatedByTies(t *testing.T) {
	a := []float64{0, 0, 0, 0, 1, 1, 2, 3}
	b := []float64{0, 0, 0, 0, 2, 2, 4, 6} // strictly monotone in a
	if got := Spearman(a, b); got < 0.999 {
		t.Errorf("Spearman on a monotone relation with ties = %.4f, want 1.0. "+
			"Arbitrary tie-breaking is what drives this below 1, and it drives it "+
			"down further the more ties there are — which is how two populations "+
			"of different tie density come to be compared with two instruments.", got)
	}
	if got := Spearman([]float64{1, 2, 3}, []float64{3, 2, 1}); got > -0.999 {
		t.Errorf("perfect inversion = %.4f, want -1.0", got)
	}
	if got := Spearman([]float64{1, 1, 1}, []float64{1, 2, 3}); got != 0 {
		t.Errorf("constant input = %.4f, want 0 — no ordering to correlate", got)
	}
}
