package backtest

import "testing"

// TestAnticipateChipsChangesAReplayedSeason is the liveness half of wiring
// AnticipateChips/AnticipateGate up to cmd/armband: a config a manager can
// actually set has to reach a path that is capable of changing the score, or
// turning it on is a no-op indistinguishable from the setting never having
// arrived at all — the byte-identical-null trap this project treats as a
// distinct, worse bug than a wrong effect size, because it looks exactly like
// a mechanism that was tried and found not to matter.
//
// A wildcard is planned mid-season so `anticipate` has a chip ahead of every
// decision from entry to GW10 to shorten the horizon for, and to make the
// free-hit-scoring exclusion reachable in principle. The two runs are
// otherwise identical, on the same two real archived seasons every other test
// in this file replays, so an exact tie in total points across a whole season
// would say the mediator never fired rather than that it fired and cancelled
// out (see chipplay_test.go's TestChipsAreOffByDefault for the same style of
// comparison used to pin the opposite fact, that an EMPTY plan must be a
// byte-identical tie).
func TestAnticipateChipsChangesAReplayedSeason(t *testing.T) {
	cur, prior, base := chipSim(t)
	base.Chips.Wildcard = 10

	off, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}

	on := base
	on.AnticipateChips = true
	on.AnticipateGate = true
	onRes, err := Simulate(cur, prior, on)
	if err != nil {
		t.Fatal(err)
	}

	if onRes.Points == off.Points {
		t.Errorf("AnticipateChips/AnticipateGate did not change a single point across the "+
			"season (%d both ways) with a wildcard planned at GW10 — the setting does not "+
			"appear to reach the scored path", onRes.Points)
	}
}
