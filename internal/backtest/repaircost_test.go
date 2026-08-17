package backtest

import (
	"reflect"
	"testing"
)

// TestTheRepairSeriesChangesNoDecision is the observer's confinement, and it is
// the whole safety argument for wiring a repair cost into the weekly loop at all.
//
// "Do not build a state trigger for the wildcard" is a closed line, and the reason
// it is closed is that a repair cost fires immediately whatever it is measuring.
// `SimConfig.RecordRepairCost` computes the same quantity every gameweek and lets
// nothing act on it — so the failure that would matter is a patch that wires it
// into a branch, which is invisible in a points column because a rule that fires
// once produces a plausible season either way.
//
// ⚠️ **A confinement check on a path that cannot carry the effect confirms
// nothing.** So this asserts BOTH halves, and the second is the one with power:
//
//   - the season must be identical with the observer on and off — every point,
//     every transfer, every weekly fifteen;
//   - the series must be there and must be non-degenerate, because an observer
//     that silently produced nothing would pass the first half trivially and read
//     downstream as a diagnostic with nothing to say.
func TestTheRepairSeriesChangesNoDecision(t *testing.T) {
	cur, prior, base := chipSim(t)
	// Late enough to keep this to a handful of gameweeks: the observer costs three
	// `Optimize` calls a week — the expensive call in this package, about 3.5 s
	// each — and this is a guard rather than a measurement. Five gameweeks is
	// enough for every assertion below and costs roughly a minute; a GW1 entry
	// would cost six.
	base.StartGW = 33

	off, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	on := base
	on.RecordRepairCost = true
	got, err := Simulate(cur, prior, on)
	if err != nil {
		t.Fatal(err)
	}

	if off.RepairSeries != nil {
		t.Errorf("the observer filled %d weeks with RecordRepairCost unset; nil is "+
			"what says the switch was off, and an empty-but-present series reads "+
			"identically to a season nobody could price", len(off.RepairSeries))
	}
	if got.Points != off.Points || got.Transfers != off.Transfers ||
		got.Hits != off.Hits || got.HitCost != off.HitCost {
		t.Errorf("recording the repair cost moved the season: points %d against %d, "+
			"transfers %d against %d, hits %d against %d — it is an observation and "+
			"must decide nothing",
			got.Points, off.Points, got.Transfers, off.Transfers, got.Hits, off.Hits)
	}
	if !reflect.DeepEqual(got.OpeningSquad, off.OpeningSquad) {
		t.Errorf("recording the repair cost changed the opening fifteen:\n%v\n%v",
			got.OpeningSquad, off.OpeningSquad)
	}
	if len(got.Weeks) != len(off.Weeks) {
		t.Fatalf("recording the repair cost changed the number of weeks played: "+
			"%d against %d", len(got.Weeks), len(off.Weeks))
	}
	for i := range got.Weeks {
		if !reflect.DeepEqual(got.Weeks[i].Squad, off.Weeks[i].Squad) {
			t.Fatalf("GW%d holds a different fifteen with the observer on:\n%v\n%v",
				got.Weeks[i].GW, got.Weeks[i].Squad, off.Weeks[i].Squad)
		}
		if got.Weeks[i].Net != off.Weeks[i].Net {
			t.Fatalf("GW%d scored %d with the observer on and %d with it off",
				got.Weeks[i].GW, got.Weeks[i].Net, off.Weeks[i].Net)
		}
	}

	// The liveness half. The observer runs on every week that reaches a decision,
	// which is every week after the entry.
	want := len(off.Weeks) - 1
	if len(got.RepairSeries) != want {
		t.Fatalf("the series holds %d weeks and %d weeks reached a decision",
			len(got.RepairSeries), want)
	}
	priced := 0
	for _, r := range got.RepairSeries {
		if r.OK && r.FrozenOK && r.FrozenGrossOK {
			priced++
		}
		if r.OK && r.Cost != repairCostOf(r.Changes, r.Free) {
			t.Errorf("GW%d prices %d changes against %d free at %.1f, want %.1f — "+
				"the cost must stay a function of the two columns beside it",
				r.GW, r.Changes, r.Free, r.Cost, repairCostOf(r.Changes, r.Free))
		}
	}
	if priced == 0 {
		t.Fatalf("no week of %d produced a reading on all three squads; an observer "+
			"that is wired and silent is indistinguishable from one that found "+
			"nothing", len(got.RepairSeries))
	}

	// The first observed week is the week after the entry, and no transfer has
	// been made yet — so the evolving squad IS the opening one and the two arms
	// must agree exactly, on the budget as well as the count. This is the join
	// proof: a frozen arm that had quietly been handed the live squad or the live
	// wallet would agree here too, but one reading the WRONG squad would not.
	first := got.RepairSeries[0]
	if first.Changes != first.FrozenChanges || first.Budget != first.FrozenBudget {
		t.Errorf("at GW%d, before any transfer, the evolving arm reads %d changes "+
			"at a budget of %d and the frozen arm %d at %d — the two squads are the "+
			"same fifteen in that week and must price identically",
			first.GW, first.Changes, first.Budget,
			first.FrozenChanges, first.FrozenBudget)
	}
}

// TestTheFrozenArmIsTheOpeningFifteenAtTheOpeningWallet pins the two snapshots the
// frozen series rests on, because both fail silently.
//
// A frozen arm that quietly read the EVOLVING squad would produce a series
// identical to the policy one and be read as "both arms agree", which is the
// conclusion the whole diagnostic exists to distinguish from a real one. A frozen
// arm sharing the LIVE wallet would have its budget move with transfers it never
// made, so its rising series would be the policy's spending rather than its own
// squad's decay.
func TestTheFrozenArmIsTheOpeningFifteenAtTheOpeningWallet(t *testing.T) {
	w := newWallet(50)
	w.bought[1], w.bought[2] = 100, 60
	frozen := w.clone()

	w.sellAt(1, 120)
	w.buy(3, 150)
	if frozen.bank != 50 {
		t.Errorf("the frozen wallet's bank moved to %d when the live one traded",
			frozen.bank)
	}
	if paid, ok := frozen.bought[1]; !ok || paid != 100 {
		t.Errorf("the frozen wallet lost the sold player's book price: %d, %v",
			paid, ok)
	}
	if _, ok := frozen.bought[3]; ok {
		t.Errorf("the frozen wallet received a purchase the live one made")
	}
}
