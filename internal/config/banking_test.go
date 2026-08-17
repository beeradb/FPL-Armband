package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTheBankingSettingsSurviveAnOlderConfigFile is the backfill contract for
// two fields that deliberately have no backfill.
//
// The standing rule is that a new field gets one in Load, so an existing file
// stays valid. These two are the narrow case where the *absence* of a backfill
// is the correct implementation — both are booleans defaulting to off, so a
// missing key already unmarshals to the shipped behaviour — and that is a claim
// worth pinning rather than asserting in a comment.
//
// The failure it guards is the one this project has shipped before in the other
// direction: a value-check migration that never fires because `cfg` starts from
// Default(). If either default ever moves to true, this test fails, and the fix
// is a `hasKey` probe rather than a value check — at that point a deliberate
// `false` and an absent key stop meaning the same thing.
func TestTheBankingSettingsSurviveAnOlderConfigFile(t *testing.T) {
	if d := DefaultReviewPolicy(); d.BankTransfersLookahead || d.PrepareForChips {
		t.Fatal("both settings ship off; a default of true makes the absent key and " +
			"a deliberate false different facts, which needs a hasKey migration " +
			"rather than the nothing this test is pinning")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// A file written before either field existed: the review policy present, the
	// two keys absent.
	old := `{"entry_id":1,"review_policy":{"min_gain_for_free_transfer":0.55,` +
		`"bank_transfers_up_to":3,"max_hits_per_week":2}}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("an older config file must still load: %v", err)
	}
	if cfg.Review.BankTransfersLookahead {
		t.Error("an absent bank_transfers_lookahead must load as off — a file " +
			"written before the field existed had the greedy policy and must keep it")
	}
	if cfg.Review.PrepareForChips {
		t.Error("an absent prepare_squad_for_chips must load as off")
	}
	// And the settings the file DID carry survive, so this is not passing because
	// the whole policy was replaced by defaults.
	if cfg.Review.MinGainForTransfer != 0.55 || cfg.Review.BankUpTo != 3 ||
		cfg.Review.MaxHitsPerWeek != 2 {
		t.Errorf("the file's own review policy was not preserved: %+v", cfg.Review)
	}

	// An explicit true round-trips, which is what makes the setting reachable at
	// all — a field that loads as false whatever the file says is a knob that
	// does not exist.
	on := `{"entry_id":1,"review_policy":{"bank_transfers_lookahead":true,` +
		`"prepare_squad_for_chips":true}}`
	if err := os.WriteFile(path, []byte(on), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Review.BankTransfersLookahead || !cfg.Review.PrepareForChips {
		t.Errorf("an explicit true did not survive Load: %+v", cfg.Review)
	}
}
