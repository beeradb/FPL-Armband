package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRosterSetIsMutuallyExclusive — a player locked and excluded at once makes
// the solver unsatisfiable, and the failure would surface as "no legal squad"
// rather than as the contradiction it is.
func TestRosterSetIsMutuallyExclusive(t *testing.T) {
	var r Roster
	if err := r.Set("lock", RosterOverride{Code: 1, Name: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Set("exclude", RosterOverride{Code: 1, Name: "A"}); err != nil {
		t.Fatal(err)
	}
	if len(r.Lock) != 0 {
		t.Errorf("excluding a locked player left him locked: %+v", r.Lock)
	}
	if len(r.Exclude) != 1 {
		t.Errorf("exclude list is %+v, want one entry", r.Exclude)
	}
	if err := r.Set("clear", RosterOverride{Code: 1}); err != nil {
		t.Fatal(err)
	}
	if len(r.Lock) != 0 || len(r.Exclude) != 0 {
		t.Error("clear left the player on a list")
	}
	if err := r.Set("nonsense", RosterOverride{Code: 1}); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

// TestRosterSetReplacesRatherThanDuplicates — setting the same player twice
// must update him, or the lists grow without bound across a season.
func TestRosterSetReplacesRatherThanDuplicates(t *testing.T) {
	var r Roster
	_ = r.Set("exclude", RosterOverride{Code: 7, Name: "Saliba", Reason: "back"})
	_ = r.Set("exclude", RosterOverride{Code: 7, Name: "Saliba", Reason: "back, longer than thought"})
	if len(r.Exclude) != 1 {
		t.Fatalf("%d entries for one player, want 1", len(r.Exclude))
	}
	if r.Exclude[0].Reason != "back, longer than thought" {
		t.Errorf("reason is %q; the update did not replace", r.Exclude[0].Reason)
	}
}

// TestRosterOverridesExpire guards the failure mode that has bitten this
// codebase before: a hand-maintained list that outlives the situation it
// described and keeps applying silently.
func TestRosterOverridesExpire(t *testing.T) {
	r := Roster{
		Exclude: []RosterOverride{
			{Code: 1, Name: "Injured", UntilGameweek: 6},
			{Code: 2, Name: "Forever", UntilGameweek: 0},
		},
		Lock: []RosterOverride{{Code: 3, Name: "Essential", UntilGameweek: 4}},
	}
	lock, exclude, expired := r.Active(5)
	if len(lock) != 0 {
		t.Errorf("a lock through GW4 is still active at GW5: %+v", lock)
	}
	if len(exclude) != 2 {
		t.Errorf("%d exclusions active at GW5, want 2", len(exclude))
	}
	if len(expired) != 1 || expired[0].Name != "Essential" {
		t.Errorf("expired is %+v, want the GW4 lock", expired)
	}
	// An indefinite override never expires — it is reported for review instead.
	_, exclude, _ = r.Active(38)
	found := false
	for _, o := range exclude {
		if o.Name == "Forever" {
			found = true
		}
	}
	if !found {
		t.Error("an indefinite exclusion lapsed on its own")
	}
}

// TestConfirmRefreshesWithoutMovingLists — re-verifying an override must not
// change which list a player is on, or a routine weekly check silently flips a
// lock into an exclusion.
func TestConfirmRefreshesWithoutMovingLists(t *testing.T) {
	r := Roster{Exclude: []RosterOverride{
		{Code: 9, Name: "Saliba", Reason: "back", SetOn: "2026-08-05",
			LastChecked: "2026-08-05", UntilGameweek: 8},
	}}
	if err := r.Set("confirm", RosterOverride{
		Code: 9, Name: "Saliba", Reason: "back, setback in training",
		LastChecked: "2026-09-01", UntilGameweek: 12,
	}); err != nil {
		t.Fatal(err)
	}
	if len(r.Exclude) != 1 || len(r.Lock) != 0 {
		t.Fatalf("confirm moved him between lists: lock=%+v exclude=%+v", r.Lock, r.Exclude)
	}
	got := r.Exclude[0]
	if got.LastChecked != "2026-09-01" {
		t.Errorf("last checked %q, want the new date", got.LastChecked)
	}
	if got.UntilGameweek != 12 {
		t.Errorf("until GW%d; a setback should push the date out", got.UntilGameweek)
	}
	if got.SetOn != "2026-08-05" {
		t.Errorf("set-on moved to %q; it records when the override began", got.SetOn)
	}
	// Confirming someone with no override is a mistake, not a way to create one.
	if err := r.Set("confirm", RosterOverride{Code: 99, Name: "Nobody"}); err == nil {
		t.Error("confirmed a player who has no standing override")
	}
}

// TestNeedsCheckCatchesBothFailureModes — an override goes stale two ways: nobody
// has looked in a while, or its expiry is imminent and the date itself is now the
// decision.
func TestNeedsCheckCatchesBothFailureModes(t *testing.T) {
	today := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	fresh := RosterOverride{SetOn: "2026-08-30", LastChecked: "2026-08-30", UntilGameweek: 20}
	if fresh.NeedsCheck(today, 5) {
		t.Error("an override checked two days ago is flagged")
	}
	stale := RosterOverride{SetOn: "2026-08-01", LastChecked: "2026-08-01", UntilGameweek: 20}
	if !stale.NeedsCheck(today, 5) {
		t.Error("an override unchecked for a month is not flagged")
	}
	// Recently checked, but about to lapse: the return date is this week's call.
	expiring := RosterOverride{SetOn: "2026-08-30", LastChecked: "2026-08-30", UntilGameweek: 6}
	if !expiring.NeedsCheck(today, 5) {
		t.Error("an override lapsing next gameweek is not flagged")
	}
	// Never checked at all.
	never := RosterOverride{SetOn: "not-a-date"}
	if !never.NeedsCheck(today, 5) {
		t.Error("an unparseable date is not flagged for a look")
	}
}

// TestTermWeightsAreNotBackfilled — zero is a setting for a term weight, not an
// omission.
//
// SetPieceWeight ships at 0.0 because measurement showed it double-counted
// penalties, and BonusWeight documents the sweep that a reader might want to
// re-run by zeroing it. A `<= 0` backfill of the kind used for BenchWeight and
// MinutesWeight would silently overwrite both, so term weights are deliberately
// left out of that list.
func TestTermWeightsAreNotBackfilled(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"

	// Explicitly zeroed term weights must survive a round trip.
	cfg := Default()
	cfg.Weights.BonusWeight = 0
	cfg.Weights.FixtureWeight = 0
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Weights.BonusWeight != 0 {
		t.Errorf("bonus_weight 0 was backfilled to %v; disabling the term is impossible",
			got.Weights.BonusWeight)
	}
	if got.Weights.FixtureWeight != 0 {
		t.Errorf("fixture_weight 0 was backfilled to %v", got.Weights.FixtureWeight)
	}
	if got.Weights.SetPieceWeight != 0 {
		t.Errorf("set_piece_weight is %v; it ships at 0 for a measured reason",
			got.Weights.SetPieceWeight)
	}

	// A key absent from the file keeps its default, because Unmarshal leaves
	// fields it does not see alone and cfg starts from Default().
	if err := os.WriteFile(path, []byte(`{"entry_id": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Weights.BonusWeight != Default().Weights.BonusWeight {
		t.Errorf("an absent bonus_weight loaded as %v, want the default %v",
			got.Weights.BonusWeight, Default().Weights.BonusWeight)
	}
}

// The post-tournament term was renamed from "rest_discount" (a Score
// multiplier) to "rest_minutes_factor" (a minutes multiplier). An unknown JSON
// key unmarshals silently, so without a migration an existing config would lose
// the term entirely and nothing would say so.
//
// The trap this pins is that presence cannot be tested by value: Load starts
// from Default(), so an absent rest_minutes_factor already reads as 0.83 and a
// zero-check migration never fires.
func TestLegacyRestDiscountMigrates(t *testing.T) {
	write := func(body string) Config {
		t.Helper()
		p := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	// An old file keeps its effective behaviour: 0.75 on Score is the same
	// discount as 0.75^(1/1.25) on minutes, run back through the exponent.
	old := write(`{"weights":{"rest_discount":0.75,"minutes_weight":1.25}}`)
	want := math.Pow(0.75, 1/1.25)
	if math.Abs(old.Weights.RestMinutesFactor-want) > 1e-9 {
		t.Errorf("legacy rest_discount 0.75 migrated to %.4f, want %.4f",
			old.Weights.RestMinutesFactor, want)
	}
	if eff := math.Pow(old.Weights.RestMinutesFactor, 1.25); math.Abs(eff-0.75) > 1e-9 {
		t.Errorf("migrated factor has Score effect %.4f, want the original 0.75", eff)
	}

	// A file that says nothing gets the calibrated default.
	if got := write(`{"weights":{}}`).Weights.RestMinutesFactor; got != Default().Weights.RestMinutesFactor {
		t.Errorf("absent key gave %.4f, want the default %.4f",
			got, Default().Weights.RestMinutesFactor)
	}

	// A new file wins outright, and is not overwritten by a stale legacy key.
	both := write(`{"weights":{"rest_minutes_factor":0.90,"rest_discount":0.75}}`)
	if both.Weights.RestMinutesFactor != 0.90 {
		t.Errorf("explicit rest_minutes_factor gave %.4f, want 0.90", both.Weights.RestMinutesFactor)
	}

	// The legacy field must never survive into the running config.
	if old.Weights.LegacyRestDiscount != 0 {
		t.Errorf("legacy field leaked into the config as %.4f", old.Weights.LegacyRestDiscount)
	}
}
