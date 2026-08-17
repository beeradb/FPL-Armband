package analysis

import "testing"

// TestPackageValueFloorsAtDoingNothing pins the arithmetic both callers share.
//
// The floor is what makes the two arms of the banking comparison commensurable:
// a package that fails the gain floor, or whose gain does not cover its own
// transfer charge, is worth exactly what declining it is worth. Without it a
// negative value could win a max() against another negative and the rule would
// compare two things nobody would do.
func TestPackageValueFloorsAtDoingNothing(t *testing.T) {
	for _, c := range []struct {
		name              string
		p                 TransferPackage
		horizon, freeCost float64
		minGain           float64
		want              float64
	}{
		{"clears everything", TransferPackage{Gain: 1.0, Moves: 1}, 5, 2, 0.4, 3},
		{"two moves pay twice", TransferPackage{Gain: 2.0, Moves: 2}, 5, 2, 0.4, 6},
		{"below the gain floor", TransferPackage{Gain: 0.3, Moves: 1}, 5, 2, 0.4, 0},
		{"gain does not cover the charge", TransferPackage{Gain: 0.5, Moves: 1}, 2, 2, 0.4, 0},
		{"no moves is not a package", TransferPackage{Gain: 9, Moves: 0}, 5, 2, 0.4, 0},
	} {
		if got := PackageValue(c.p, c.horizon, c.freeCost, c.minGain); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// The best of several, and never below doing nothing. Gains are chosen to be
	// exact in binary — main has just landed the lesson that replayed floats must
	// not be compared for equality, and a pinning test should not need a
	// tolerance to state an arithmetic fact.
	best := BestPackageValue([]TransferPackage{
		{Gain: 0.5, Moves: 1}, {Gain: 1.25, Moves: 2}, {Gain: 0.25, Moves: 1},
	}, 5, 2, 0.4)
	if best != 2.25 {
		t.Errorf("best package is %v, want 2.25", best)
	}
	if got := BestPackageValue(nil, 5, 2, 0.4); got != 0 {
		t.Errorf("no packages is worth %v, want 0 — doing nothing is always available", got)
	}
}

// TestBankingActsOnATie pins the direction a tie goes.
//
// Waiting costs a gameweek of whatever was on the table, so a rule that banked on
// equal value would trade a certain week for an estimate. It is one line of code
// and it is the kind of line two implementations disagree about silently, which
// is why it is a function and why this test exists.
func TestBankingActsOnATie(t *testing.T) {
	if PreferWaiting(3.0, 3.0) {
		t.Error("a tie must act, not wait")
	}
	if !PreferWaiting(3.0, 3.0001) {
		t.Error("a larger later value must wait")
	}
	if PreferWaiting(3.0, 2.9999) {
		t.Error("a smaller later value must act")
	}
}

// TestBankAdviceSeparatesItsThreeRefusals is why the advice carries a reason.
//
// A policy that declines a transfer and says nothing is indistinguishable from
// one that found nothing worth doing, and those are opposite recommendations. The
// three refusals are a guard on the allowance, a guard on the season ending, and
// the comparison genuinely coming out against waiting — and one of those has a
// degenerate case, where neither arm was worth anything, that must not be
// counted as the rule preferring to act.
func TestBankAdviceSeparatesItsThreeRefusals(t *testing.T) {
	// At the ceiling: waiting buys no extra move.
	a := AdviseBank(5, 5, 5, 10, 99)
	if a.Bank || a.Guard != BankGuardCeiling || a.Weighed() {
		t.Errorf("at the ceiling: %+v — must refuse without comparing, however "+
			"large the later arm looks", a)
	}
	// One gameweek left: nothing to spend a banked transfer into.
	a = AdviseBank(1, 5, 1, 10, 99)
	if a.Bank || a.Guard != BankGuardHorizon || a.Weighed() {
		t.Errorf("at the season's end: %+v", a)
	}
	// Nothing on either side: a refusal by a rule that had nothing to weigh.
	a = AdviseBank(1, 5, 5, 0, 0)
	if a.Bank || a.Guard != BankGuardNone || a.Weighed() {
		t.Errorf("nothing to weigh: %+v — this must not read as the rule "+
			"preferring to act now", a)
	}
	// A real comparison, both ways round.
	a = AdviseBank(1, 5, 5, 4, 9)
	if !a.Bank || !a.Weighed() {
		t.Errorf("waiting is worth more: %+v", a)
	}
	a = AdviseBank(1, 5, 5, 9, 4)
	if a.Bank || !a.Weighed() {
		t.Errorf("acting is worth more: %+v", a)
	}
	// Every state says something. A blank line is how a declined move becomes
	// indistinguishable from a policy that was never asked.
	for _, c := range []BankAdvice{
		{Bank: true}, {Guard: BankGuardCeiling}, {Guard: BankGuardHorizon},
		{}, {NowValue: 9, LaterValue: 4},
	} {
		if c.Explain() == "" {
			t.Errorf("no explanation for %+v", c)
		}
	}
}

// TestTheChipWindowIsWalledByAWildcardAndNotByAFreeHit pins the two boundaries
// that decide whether a transfer made now is preparing for anything.
//
// A wildcard replaces all fifteen, so a credit reaching past one would spend this
// week's transfers buying a bench for a squad about to be torn up. A free hit
// lends a fifteen for one week and hands the permanent squad straight back, so a
// chip beyond it is still played by the squad being valued now.
func TestTheChipWindowIsWalledByAWildcardAndNotByAFreeHit(t *testing.T) {
	boost := ChipSchedule{First: ChipPlan{BenchBoost: 22}}
	// Inside the horizon, nothing in the way.
	if cr := ChipCreditAt(boost, true, true, 20, 5); cr.Bench <= 0 {
		t.Errorf("a boost two weeks out must be credited: %+v", cr)
	}
	// Outside the horizon.
	if cr := ChipCreditAt(boost, true, true, 10, 5); cr.Bench != 0 {
		t.Errorf("a boost twelve weeks out must not be: %+v", cr)
	}
	// Switched off is a hard zero, whatever is planned.
	if cr := ChipCreditAt(boost, false, false, 20, 5); cr.Bench != 0 || cr.Captain != 0 {
		t.Errorf("preparation off must credit nothing: %+v", cr)
	}
	// A wildcard in between walls it off.
	walled := ChipSchedule{First: ChipPlan{BenchBoost: 22, Wildcard: 21}}
	if cr := ChipCreditAt(walled, true, true, 20, 5); cr.Bench != 0 {
		t.Errorf("a wildcard before the chip must end the window: %+v", cr)
	}
	// A free hit in between does not.
	lent := ChipSchedule{First: ChipPlan{BenchBoost: 22, FreeHit: 21}}
	if cr := ChipCreditAt(lent, true, true, 20, 5); cr.Bench <= 0 {
		t.Errorf("a free hit hands the squad back, so it must not wall: %+v", cr)
	}
	// Amortised over the horizon the gate is about to multiply it back by, so the
	// two cancel to what the chip actually pays once.
	if cr := ChipCreditAt(boost, true, true, 20, 4); cr.Bench != 1.0/4 {
		t.Errorf("credit is %v, want 1/horizon", cr.Bench)
	}
	// Both sets are consulted. A second-set chip is invisible to a reader that
	// asks a single field, which is how a two-set plan reverts to single-set
	// behaviour with nothing failing.
	second := ChipSchedule{Second: ChipPlan{BenchBoost: 22}}
	if cr := ChipCreditAt(second, true, true, 20, 5); cr.Bench <= 0 {
		t.Errorf("a second-set boost must be credited: %+v", cr)
	}
}

// TestMoveLimitClampsToOneHit pins the arithmetic the banking comparison is
// built on — the whole question is what one more move would buy.
func TestMoveLimitClampsToOneHit(t *testing.T) {
	if got := MoveLimit(2, 1, 0); got != 3 {
		t.Errorf("two free plus a hit is %d, want 3", got)
	}
	if got := MoveLimit(2, 3, 0); got != 3 {
		t.Errorf("more than one hit must clamp: got %d, want 3", got)
	}
	if got := MoveLimit(2, 1, 2); got != 2 {
		t.Errorf("an explicit ceiling must bind: got %d, want 2", got)
	}
	if got := MoveLimit(0, 0, 0); got != 0 {
		t.Errorf("no transfers and no hits is %d, want 0", got)
	}
}
