package backtest

// The xGC reconstruction is gated TWICE, and only one gate has a test.
//
// The work queue carried this as "give the xGC repair its own gate, two lines to fix",
// naming `FPL_NO_XG_REPAIR=1` as the coupling. Two corrections, and the second is
// why this file exists.
//
// **The switch half is already done.** `TestTheXGCRepairHasAWorkingEscapeHatch`
// in `xgccoverage_test.go` pins both hatches on 2021-22, loading through `Load`
// rather than `loadSeason` so the process-global season cache cannot hand one arm
// the other's parse. Nothing there needs writing.
//
// **The second gate is not the switch.** `applyXGCRepair` is called at
// `xgrepair.go:367`, inside `applyXGRepair` and past its `if !ok || noXGRepair()`
// early return — and `!ok` is a season being **absent from `xgRepairs`**. So a
// season with no xG repair spec silently gets no xGC reconstruction either. That
// is harmless today, because every season outside the map carries native xGC, and
// it is exactly the shape this project calls its signature failure: a knob that
// does not arrive returns a byte-identical null, indistinguishable from a knob
// that does nothing.
//
// # And severing the coupling would not buy a fourth corner
//
// Worth recording here so nobody "fixes" it into something worse. The ordering is
// forced by the method: a club's expected goals conceded *is* its opponents'
// expected goals, so the reconstruction reads the repaired xG that
// `applyXGRepair` has just written. With the xG repair off, the blind seasons
// carry no xG at all, so an "xG off, xGC on" corner would reconstruct from zeros.
//
// The corner is meaningful only where native xG exists: 2023-24 onward, which
// already carry native xGC and where the repair is inert — plus one genuine but
// small window, **2022-23 from GW16**, whose xG repair covers GW1-15 only against
// a recorded 60% native xGC coverage. One partial half-season cannot carry a
// fourth corner.
//
// **So the 2x2 has three distinct corners for a structural reason rather than a
// wiring one**, and the right fix is to make the dependency explicit and reported,
// which is what this file does.

import "testing"

func TestTheXGCRepairIsConfinedToTheXGRepairSeasons(t *testing.T) {
	// Deliberately a hardcoded expectation rather than anything derived from
	// xgRepairs: a test that computes its expectation from the thing it is
	// checking asserts nothing, which this package has shipped twice.
	want := map[string]bool{
		"2018-19": true, "2019-20": true, "2020-21": true,
		"2021-22": true, "2022-23": true,
	}
	for name := range xgRepairs {
		if !want[name] {
			t.Errorf("%s gained an xG repair spec, so it silently gained the xGC "+
				"reconstruction too — applyXGCRepair is called inside applyXGRepair, "+
				"past its early return. Confirm that is intended, and that the season "+
				"has xG for the reconstruction to read.", name)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("%s lost its xG repair spec, which silently removes its xGC "+
			"reconstruction as well. The two are not separately gated, so this is "+
			"two changes rather than one.", name)
	}
}
