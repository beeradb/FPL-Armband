package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// populatedTeam is a TeamConfig with every field set to something that is not
// its zero value, so an assertion about "was it carried" cannot pass vacuously.
//
// Built by hand rather than by reflection because it has to be READABLE: the
// point of these tests is that a person can see what a reader would and would
// not inherit. TestPopulatedTeamHasEveryFieldSet keeps it honest as fields are
// added.
func populatedTeam() TeamConfig {
	mins := 90.0
	return TeamConfig{
		Chips: analysis.ChipSchedule{
			First:  analysis.ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9},
			Second: analysis.ChipPlan{Wildcard: 20, FreeHit: 36, BenchBoost: 38, TripleCaptain: 37},
		},
		HypotheticalBudget: 103.5,
		Criteria:           []string{"Never own more than one Spurs player."},
		Lock: []RosterOverride{{
			Code: 118748, Name: "Salah", Reason: "the squad is built around him",
			SetOn: "2026-08-31", LastChecked: "2026-08-31", ExpectedMinutes: &mins,
		}},
		LeadHours: 11,
	}
}

// TestPopulatedTeamHasEveryFieldSet is the guard on the fixture above, not on
// the code under test.
//
// A field added to TeamConfig and not added to populatedTeam would sit at its
// zero value there, and every test below would go on passing while checking
// nothing about it — the shape of vacuous pass this project has been caught by
// before. Asserted on the whole struct so it needs no maintenance of its own.
func TestPopulatedTeamHasEveryFieldSet(t *testing.T) {
	v := reflect.ValueOf(populatedTeam())
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Errorf("populatedTeam leaves %s at its zero value, so every test "+
				"using it asserts nothing about that field", v.Type().Field(i).Name)
		}
	}
}

// TestTheTeamHalfSurvivesARoundTripThroughBothFiles pins that a team file
// written out and read back is the same file, and that the CONFIG half is
// unharmed by carrying it in memory.
//
// Asserted on the whole TeamConfig rather than field by field: a field added
// later that ApplyTo, Team or SaveTeam forgot would otherwise round-trip to its
// zero value with nothing failing, which is exactly how a moved setting stops
// applying in silence.
func TestTheTeamHalfSurvivesARoundTripThroughBothFiles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	teamPath := filepath.Join(dir, "team.json")

	want := populatedTeam()
	merged := want.ApplyTo(Default())

	if err := Save(cfgPath, merged); err != nil {
		t.Fatal(err)
	}
	if err := SaveTeam(teamPath, merged.Team()); err != nil {
		t.Fatal(err)
	}

	// ⚠️ Load must NOT error. If Save wrote any of the moved keys back into
	// config.json, checkNoTeamKeys refuses the file it just produced — a writer
	// that bricks its own output, which is the trap the `json:"-"` tags exist
	// to close.
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("the config Save just wrote does not load back: %v\n\n"+
			"Save wrote a key that moved to the team file. Config's moved fields "+
			"must all be json:\"-\".", err)
	}
	got, err := LoadTeam(teamPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the team file did not round trip\n  got:  %+v\n  want: %+v", got, want)
	}
	if !reflect.DeepEqual(got.ApplyTo(cfg).Team(), want) {
		t.Errorf("re-applying the team file to the reloaded config lost something")
	}
}

// TestSaveWritesNoTeamKeyIntoTheConfigFile is the same guard read from the
// other end, and it is the one that would catch a field whose tag was left
// alone by mistake.
func TestSaveWritesNoTeamKeyIntoTheConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, populatedTeam().ApplyTo(Default())); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range movedTeamKeys {
		if hasKey(raw, k.path...) {
			t.Errorf("Save wrote %q into config.json; it belongs in the team file, "+
				"and the next Load refuses a config that carries it",
				strings.Join(k.path, "."))
		}
	}
}

// TestLoadRefusesAConfigThatStillCarriesAMovedKey is the LOUD half of the
// migration.
//
// `internal/config` does not use DisallowUnknownFields, so a key left behind
// parses, is ignored, and the setting simply stops applying — with nothing
// anywhere saying so. A chip plan that reads "no chips planned" is
// indistinguishable from a manager who planned none, and that is the
// byte-identical null this project keeps paying for.
//
// The table is driven by movedTeamKeys itself rather than by a hand-written
// list, so a key added to the migration is covered the day it is added.
func TestLoadRefusesAConfigThatStillCarriesAMovedKey(t *testing.T) {
	for _, k := range movedTeamKeys {
		joined := strings.Join(k.path, ".")
		t.Run(joined, func(t *testing.T) {
			// Built by nesting, so a nested key lands where the real file has it
			// rather than at the top level under a dotted name nothing reads.
			var v any = json.RawMessage(`[]`)
			for i := len(k.path) - 1; i >= 0; i-- {
				v = map[string]any{k.path[i]: v}
			}
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, b, 0o644); err != nil {
				t.Fatal(err)
			}

			_, err = Load(path)
			if err == nil {
				t.Fatalf("a config.json still carrying %q loaded without complaint. "+
					"It would parse, be ignored, and the setting would silently stop "+
					"applying", joined)
			}
			// The message has to name the key. "Something moved" sends the reader
			// diffing two files by hand.
			if !strings.Contains(err.Error(), joined) {
				t.Errorf("the error does not name %q: %v", joined, err)
			}
			if !strings.Contains(err.Error(), "team file") {
				t.Errorf("the error does not say where the key went: %v", err)
			}
		})
	}
}

// TestReviewRulesIsNotTreatedAsAMovedKey pins the correction that a docs review
// caught before this shipped.
//
// `review_policy.rules` reads exactly like `criteria` — free text in the
// owner's own words — and the first pass at this split moved it for that
// reason. It is a PUBLISHED SURFACE: page.go copies it into present.Policy, the
// viewmodel carries it into the state, and the template gates the whole "The
// rules it is deciding under" section on it being non-empty. Moving it would
// have blanked a section of the live site, and refusing a config that carries
// it would have refused every config that exists.
func TestReviewRulesIsNotTreatedAsAMovedKey(t *testing.T) {
	for _, k := range movedTeamKeys {
		if strings.Join(k.path, ".") == "review_policy.rules" {
			t.Fatal("review_policy.rules is listed as a moved key. It is rendered " +
				"on every page under \"The rules it is deciding under\" and must " +
				"stay in config.json — see ReviewPolicy.Rules.")
		}
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(
		`{"review_policy":{"rules":["Do nothing is a valid answer."]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("a config carrying review_policy.rules must still load: %v", err)
	}
	if len(cfg.Review.Rules) != 1 || cfg.Review.Rules[0] != "Do nothing is a valid answer." {
		t.Errorf("review_policy.rules did not survive the load: %+v", cfg.Review.Rules)
	}
}

// TestTheShippedFilesLoad checks the repository's own pair against the loader
// that now refuses a stale config.
//
// The tracked config.json is read by every backtest diagnostic through
// harness_test's loadConfig, which t.Fatalf's on a load error — so a migration
// that shipped ahead of the file would fail every diagnostic at once, and a
// fresh clone of `armband` with it. This is the cheapest place to notice.
func TestTheShippedFilesLoad(t *testing.T) {
	cfgPath := filepath.Join("..", "..", "config.json")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skipf("no config.json at the repository root: %v", err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("the tracked config.json does not load: %v\n\n"+
			"Migrate it in the same commit as the loader change: every backtest "+
			"diagnostic loads this file and fails hard when it will not load.", err)
	}
	team, err := LoadTeam(filepath.Join("..", "..", "team.json"))
	if err != nil {
		t.Fatalf("the tracked team.json does not load: %v", err)
	}
	// The merged pair must be a whole config: the shipped chip plan is what the
	// replay plays, and reading it as empty is the failure the split risks.
	if merged := team.ApplyTo(cfg); merged.Chips == (analysis.ChipSchedule{}) {
		t.Error("the shipped team.json carries no chip plan at all, so every " +
			"diagnostic reading it replays a chipless season")
	}
}

// TestLoadTeamRefusesAPathThatDoesNotExist. -team is an explicit statement that
// a team file exists. Answering a typo with an empty chip plan is precisely the
// silent null the loud migration above exists to abolish, so it must not be the
// answer here either. Load creates config.json when absent; this deliberately
// does not.
func TestLoadTeamRefusesAPathThatDoesNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.json")
	if _, err := LoadTeam(path); err == nil {
		t.Fatal("a -team path that does not exist loaded as an empty team file")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("LoadTeam created the file it was asked to read; -team must not " +
			"write one, or a typo silently becomes a new empty plan")
	}
}

// TestLoadTeamKeepsTheShippedDefaultsForKeysItDoesNotCarry. A team file silent
// on criteria or on the lead time has not repealed them: it gets what a
// config.json omitting those keys always got.
func TestLoadTeamKeepsTheShippedDefaultsForKeysItDoesNotCarry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team.json")
	if err := os.WriteFile(path, []byte(`{"chip_plan":{"wildcard_gameweek":6}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTeam(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Criteria, defaultCriteria()) {
		t.Errorf("a team file silent on criteria lost the shipped list: %+v", got.Criteria)
	}
	if got.LeadHours != defaultLeadHours {
		t.Errorf("lead hours = %v, want the shipped %v", got.LeadHours, defaultLeadHours)
	}
	if got.Chips.First.Wildcard != 6 {
		t.Errorf("the flat single-set chip plan did not read as the first set: %+v", got.Chips)
	}
}

// TestSavePairRefusesATeamChangeWithNowhereToPutIt.
//
// `-persist` and the agent's set_player_status both persist a LOCK, and a lock
// lives in the team file now. A writer holding only a config path has two bad
// options — write `roster.lock` back into config.json, where the next Load
// hard-errors on it, or drop it and report success — so it refuses instead. The
// config file must be untouched by the refusal: half a change is worse than
// none, and the caller advances its in-memory config on the strength of this
// returning nil.
func TestSavePairRefusesATeamChangeWithNowhereToPutIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	before := Default()
	if err := Save(path, before); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	after := before
	after.Roster.Lock = populatedTeam().Lock

	err = SavePair(path, "", before, after)
	if err == nil {
		t.Fatal("a lock was persisted with no team file to persist it to")
	}
	if !strings.Contains(err.Error(), "lock") {
		t.Errorf("the refusal does not name the setting it is about: %v", err)
	}
	if !strings.Contains(err.Error(), "-team") {
		t.Errorf("the refusal does not say how to fix it: %v", err)
	}
	if now, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if !now.ModTime().Equal(stat.ModTime()) {
		t.Error("the refused change still rewrote config.json; a refusal must " +
			"leave both files exactly as they were")
	}
}

// TestSavePairWritesEachHalfToItsOwnFile, and writes only the half that moved.
func TestSavePairWritesEachHalfToItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	teamPath := filepath.Join(dir, "team.json")

	before := Default()
	after := before
	after.Roster.Lock = populatedTeam().Lock
	after.Congestion.UCLPenalty = 0.77 // a config-half change in the same write

	if err := SavePair(cfgPath, teamPath, before, after); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Congestion.UCLPenalty != 0.77 {
		t.Errorf("the config half did not land: %v", cfg.Congestion.UCLPenalty)
	}
	if len(cfg.Roster.Lock) != 0 {
		t.Error("the lock came back out of config.json, which the loader refuses")
	}
	team, err := LoadTeam(teamPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(team.Lock) != 1 || team.Lock[0].Code != 118748 {
		t.Errorf("the lock did not reach the team file: %+v", team.Lock)
	}
}

// TestSavePairLeavesTheTeamFileAloneWhenNothingTeamSideMoved.
//
// Most owner-side writes — a competition window, a minutes override — touch
// only the config half. Rewriting team.json on every one of them would churn a
// tracked file for no reason and make its history useless for answering "when
// did the chip plan last change".
func TestSavePairLeavesTheTeamFileAloneWhenNothingTeamSideMoved(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	teamPath := filepath.Join(dir, "team.json")

	before := populatedTeam().ApplyTo(Default())
	if err := Save(cfgPath, before); err != nil {
		t.Fatal(err)
	}
	if err := SaveTeam(teamPath, before.Team()); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(teamPath)
	if err != nil {
		t.Fatal(err)
	}

	after := before
	after.Congestion.UCLPenalty = 0.66
	if err := SavePair(cfgPath, teamPath, before, after); err != nil {
		t.Fatal(err)
	}
	now, err := os.Stat(teamPath)
	if err != nil {
		t.Fatal(err)
	}
	if !now.ModTime().Equal(stat.ModTime()) {
		t.Error("a config-only change rewrote the team file")
	}
}

// TestFingerprintViewCarriesTheChipPlan.
//
// internal/snapshot's modelSubtrees names `chip_plan`, and a subtree it expects
// and cannot find is recorded as "ABSENT FROM CONFIG" — a changed
// constants_digest over a byte-identical model, and a value the replay actually
// reads going unfingerprinted. Both are the "changed stamp over unchanged
// cells" failure that file's own comments name. Asserted on the marshalled
// shape, because that is what the fingerprint walks.
func TestFingerprintViewCarriesTheChipPlan(t *testing.T) {
	cfg := populatedTeam().ApplyTo(Default())

	bare, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if hasKey(bare, "chip_plan") {
		t.Fatal("config.Config now marshals chip_plan; this test and " +
			"FingerprintView are both stale")
	}

	b, err := json.Marshal(cfg.FingerprintView())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Chips analysis.ChipSchedule `json:"chip_plan"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Chips != cfg.Chips {
		t.Errorf("the fingerprint view lost the chip plan\n  got:  %+v\n  want: %+v",
			got.Chips, cfg.Chips)
	}
	// The rest of the config has to still be there, or the digest covers less
	// than it claims for every other subtree.
	for _, sub := range []string{"weights", "congestion", "role_risk", "review_policy"} {
		if !hasKey(b, sub) {
			t.Errorf("the fingerprint view dropped the %q subtree", sub)
		}
	}
}
