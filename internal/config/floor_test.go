package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTheShippedFloorIsTheMeasuredSchedule pins the shipped early floor to the
// measured configuration: {1.0, 0.2} through GW8, shipped 2026-08-18 on the
// user's ruling over stats/findings/2026-08-18-scheduled-floor.md. The guard's
// job is to make a silent revert of the schedule a visible change, and to keep
// the shipped config.json and the Go default from drifting into two values.
func TestTheShippedFloorIsTheMeasuredSchedule(t *testing.T) {
	def := Default()
	want := EarlyFloor{FreeTransferValue: 1.0, MinGainForTransfer: 0.2, UntilGameweek: 8}
	if def.Review.EarlyFloor != want {
		t.Errorf("config.Default() ships the floor %+v, want %+v — the shipped "+
			"schedule is a measured configuration, not a placeholder",
			def.Review.EarlyFloor, want)
	}
	if !def.Review.BankTransfersLookahead {
		t.Errorf("config.Default() ships bank_transfers_lookahead off; the " +
			"override-mode corner that resolves includes it")
	}
	// The shipped file carries the same numbers: one quantity, one value.
	file := shippedConfig(t)
	if file.Review.EarlyFloor != want {
		t.Errorf("the shipped config.json floor is %+v, want %+v", file.Review.EarlyFloor, want)
	}
}

// TestAnExplicitOffFloorSurvivesLoad pins the on/off round-trip: a written zero
// schedule is a deliberate off, and the key-probe backfill must not re-arm it
// with the shipped default. Without this, "configurable on/off" would be a
// claim the loader silently voids.
func TestAnExplicitOffFloorSurvivesLoad(t *testing.T) {
	b := []byte(`{"review_policy": {"early_floor": {"until_gameweek": 0}}}`)
	cfg := loadBytes(t, b)
	if cfg.Review.EarlyFloor.UntilGameweek != 0 {
		t.Errorf("an explicit zero schedule was re-armed to %+v — the key-probe "+
			"backfill overwrote a deliberate off", cfg.Review.EarlyFloor)
	}
	charge, minGain := cfg.Review.EffectiveFloor(3)
	if charge != cfg.Review.FreeTransferValue || minGain != cfg.Review.MinGainForTransfer {
		t.Errorf("a zero schedule reads %v/%v at GW3, want the flat constants",
			charge, minGain)
	}
}

// TestEffectiveFloorAppliesTheSchedule pins the schedule's arithmetic: the
// early values through GW8 inclusive, the flat constants after, and the flat
// constants everywhere when the schedule is off.
func TestEffectiveFloorAppliesTheSchedule(t *testing.T) {
	r := Default().Review
	for _, c := range []struct {
		gw      int
		charge  float64
		minGain float64
	}{
		{1, 1.0, 0.2},
		{8, 1.0, 0.2},
		{9, 2.0, 0.4},
		{38, 2.0, 0.4},
	} {
		charge, minGain := r.EffectiveFloor(c.gw)
		if charge != c.charge || minGain != c.minGain {
			t.Errorf("GW%d reads %v/%v, want %v/%v",
				c.gw, charge, minGain, c.charge, c.minGain)
		}
	}
}

// loadBytes parses and backfills a config from the given JSON, the way Load
// does for a file on disk. The backfill lives inside Load, which takes a path —
// writing to a temp file is the honest way through it, because the key probe
// reads the raw bytes and a hand-rolled subset here would test a different
// backfill than the one that ships.
func loadBytes(t *testing.T, b []byte) Config {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
