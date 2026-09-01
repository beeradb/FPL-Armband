package fpl

import "testing"

// An override written `until_gameweek: N` protects gameweek N's own fixtures. It
// must not lapse at N's deadline, which is when FPL advances `is_next` — while N's
// fixtures are still being played.
//
// Live on 2026-09-01: GW3's deadline was Friday and its fixtures ran to Sunday, so
// six overlapping overrides would have expired for the whole weekend they existed
// to cover. The same failure needed a manual patch for a Watkins correction on
// 2026-08-23.
func TestOverrideGameweekHoldsWhileFixturesAreStillBeingPlayed(t *testing.T) {
	b := &Bootstrap{Events: []Event{
		{ID: 1, Finished: true},
		{ID: 2, Finished: true},
		// The deadline has passed, so FPL has already moved is_next on — but the
		// football has not been played.
		{ID: 3, Finished: false, IsCurrent: true},
		{ID: 4, Finished: false, IsNext: true},
	}}
	if got := b.NextEvent(); got == nil || got.ID != 4 {
		t.Fatalf("NextEvent should still report 4; the accessor under test is the fix, "+
			"not a change to this one: got %v", got)
	}
	ev := b.OverrideGameweek()
	if ev == nil || ev.ID != 3 {
		t.Fatalf("OverrideGameweek = %v, want gameweek 3 — an override through GW3 must "+
			"survive GW3's own fixtures", ev)
	}
	o := RosterOverrideExpiryProbe{UntilGameweek: 3}
	if o.expired(ev.ID) {
		t.Fatal("an override through GW3 lapsed while GW3 was being played")
	}
}

// Once the football is played it must lapse, or a correction outlives its evidence.
//
// ⚠️ Keyed on Finished, which is the only signal fpl.Event carries — it does not
// model data_checked at all. That is the right bar here regardless: the question is
// whether the fixtures were played, not whether FPL has finished auditing them.
func TestOverrideGameweekAdvancesOnceFixturesFinish(t *testing.T) {
	b := &Bootstrap{Events: []Event{
		{ID: 1, Finished: true},
		{ID: 2, Finished: true},
		{ID: 3, Finished: true},
		{ID: 4, Finished: false, IsNext: true},
	}}
	ev := b.OverrideGameweek()
	if ev == nil || ev.ID != 4 {
		t.Fatalf("OverrideGameweek = %v, want 4: GW3's fixtures are finished, so an "+
			"override through GW3 has done its job", ev)
	}
	o := RosterOverrideExpiryProbe{UntilGameweek: 3}
	if !o.expired(ev.ID) {
		t.Fatal("an override through GW3 outlived GW3")
	}
}

// A season with nothing finished yet must not fall through to nil.
func TestOverrideGameweekBeforeAnyFootball(t *testing.T) {
	b := &Bootstrap{Events: []Event{{ID: 1, Finished: false, IsNext: true}}}
	if ev := b.OverrideGameweek(); ev == nil || ev.ID != 1 {
		t.Fatalf("OverrideGameweek = %v, want 1", ev)
	}
}

// RosterOverrideExpiryProbe mirrors config.RosterOverride.Expired's one line, which
// this package cannot import without a cycle.
type RosterOverrideExpiryProbe struct{ UntilGameweek int }

func (o RosterOverrideExpiryProbe) expired(gw int) bool {
	return o.UntilGameweek > 0 && gw > o.UntilGameweek
}
