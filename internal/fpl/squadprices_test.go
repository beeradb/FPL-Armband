package fpl

import "testing"

// Exact is the whole point: reconstructed prices are only usable as fact when
// they reproduce FPL's own team value. A near miss is still an assumption,
// because a squad £0.3m poorer than the model thinks refuses a transfer at the
// deadline just as firmly as one £3m poorer.
func TestSquadPricesChecksAgainstReportedValue(t *testing.T) {
	exact := &SquadPrices{Value: 1003, Reconstructed: 1003}
	if !exact.Exact() {
		t.Error("a reconstruction matching team value is not reported as exact")
	}
	if exact.Drift() != 0 {
		t.Errorf("drift %d on an exact match", exact.Drift())
	}

	over := &SquadPrices{Value: 1003, Reconstructed: 1008}
	if over.Exact() {
		t.Error("a reconstruction 0.5m adrift is reported as exact")
	}
	if over.Drift() != 5 {
		t.Errorf("drift %d, want +5 — positive means the model thinks it is richer", over.Drift())
	}

	under := &SquadPrices{Value: 1003, Reconstructed: 998}
	if under.Drift() != -5 {
		t.Errorf("drift %d, want -5", under.Drift())
	}
}

// A season that has not started has nothing to reconstruct, and saying so beats
// returning an empty map that reads as "everything sells at market".
func TestSquadPricesRefusesBeforeAnyGameweek(t *testing.T) {
	c := New(t.TempDir(), 0, 0)
	if _, err := c.SquadPrices(t.Context(), 1, 0); err == nil {
		t.Error("reconstructing prices before GW1 succeeded")
	}
	if _, err := c.SquadPrices(t.Context(), 0, 5); err == nil {
		t.Error("reconstructing prices without an entry id succeeded")
	}
}
