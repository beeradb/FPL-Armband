package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfirmedBackfillMatchesTheLiveConfigOverrideByOverride pins the
// migration in backfillOverrideConfidence against the exact shape of the real
// 2026-27 production config.json: several minutes overrides with no
// "confirmed" key at all, written before RosterOverride.Confirmed existed.
//
// Verified by hand against that file, override by override: Kinsky (88), van
// Ewijk (85) and Mosquera (85) read confidently in their own free text and
// clear the retired magnitude floor, so the backfill must keep them reading
// nailed — regressing them to "likely starter" the moment this field ships
// is exactly the failure this test exists to catch. Thomas (80), Robertson
// (80) and Tzolis (82) explicitly hedge in their own reason text despite
// clearing the same floor — this backfill does NOT unhedge them (doing so
// from the number alone would be the exact mechanism being retired); it only
// promises no override's status flips on the day this ships. Isak (75) sits
// below the floor and is unaffected either way.
func TestConfirmedBackfillMatchesTheLiveConfigOverrideByOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Shaped like the real file: no "confirmed" key anywhere, because the
	// field did not exist when these were written.
	old := `{
		"entry_id": 1,
		"roster": {
			"minutes": [
				{"player_code": 1, "player": "Kinsky", "reason": "Confirmed as first-choice goalkeeper on a new long-term contract. This is now settled for the season.", "set_on": "2026-08-15", "expected_minutes": 88},
				{"player_code": 2, "player": "van Ewijk", "reason": "Starts at right wing-back tonight — the consensus pick as the best 4.0m defender in the game.", "set_on": "2026-08-15", "expected_minutes": 85},
				{"player_code": 3, "player": "Mosquera", "reason": "Starts at centre-back tonight; Saliba is out and Timber is weeks away.", "set_on": "2026-08-15", "expected_minutes": 85},
				{"player_code": 4, "player": "Thomas", "reason": "Set to 80 rather than a nailed 85 as this is his first Premier League start.", "set_on": "2026-08-15", "expected_minutes": 80},
				{"player_code": 5, "player": "Tzolis", "reason": "Set to 82 rather than a nailed 85 as this is still only his second competitive appearance for the club.", "set_on": "2026-08-15", "expected_minutes": 82},
				{"player_code": 6, "player": "Robertson", "reason": "Set to 80 rather than a nailed 85 given it's a debut for a 32-year-old in a rebuilt back line.", "set_on": "2026-08-15", "expected_minutes": 80},
				{"player_code": 7, "player": "Isak", "reason": "Held at 75 rather than a nailed 85 pending match sharpness.", "set_on": "2026-08-15", "expected_minutes": 75}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("a pre-Confirmed config file must still load: %v", err)
	}
	want := map[string]bool{
		"Kinsky":    true,
		"van Ewijk": true,
		"Mosquera":  true,
		"Thomas":    true,
		"Tzolis":    true,
		"Robertson": true,
		"Isak":      false,
	}
	if len(cfg.Roster.Minutes) != len(want) {
		t.Fatalf("got %d minutes overrides, want %d", len(cfg.Roster.Minutes), len(want))
	}
	for _, o := range cfg.Roster.Minutes {
		w, ok := want[o.Name]
		if !ok {
			t.Fatalf("unexpected override for %s", o.Name)
		}
		if o.Confirmed != w {
			t.Errorf("%s: Confirmed = %v, want %v (value %.0f) — a status flip on an "+
				"already-shipped override", o.Name, o.Confirmed, w, *o.ExpectedMinutes)
		}
	}
}

// TestExplicitConfirmedFieldIsNeverOverwrittenByTheMigration pins the other
// half of the contract: once an entry carries its own "confirmed" key —
// written either by a human editing the file directly or by this program's
// own Save, which never omits the key — the migration must leave it alone,
// in both directions. Getting this wrong would mean the retired magnitude
// heuristic keeps deciding behaviour forever, for any override whose author
// happened to pick a value on the wrong side of 80, which is the exact
// permanent fallback this field exists to remove.
func TestExplicitConfirmedFieldIsNeverOverwrittenByTheMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Player A: value clears the old floor (90) but is explicitly marked
	// unconfirmed — an analyst who has since downgraded their own confidence,
	// or a fresh override the tool wrote with confirmed left off.
	// Player B: value is well below the old floor (50) but explicitly
	// confirmed — a bit-part player the analyst is nonetheless certain about.
	cur := `{
		"entry_id": 1,
		"roster": {
			"minutes": [
				{"player_code": 10, "player": "PlayerA", "reason": "no longer certain", "set_on": "2026-08-20", "expected_minutes": 90, "confirmed": false},
				{"player_code": 11, "player": "PlayerB", "reason": "certain backup", "set_on": "2026-08-20", "expected_minutes": 50, "confirmed": true}
			]
		}
	}`
	if err := os.WriteFile(path, []byte(cur), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, o := range cfg.Roster.Minutes {
		switch o.Name {
		case "PlayerA":
			if o.Confirmed {
				t.Error("PlayerA was explicitly written confirmed:false — the migration " +
					"must not resurrect the old value-based reading (90 >= 80) over an " +
					"explicit key")
			}
		case "PlayerB":
			if !o.Confirmed {
				t.Error("PlayerB was explicitly written confirmed:true — the migration " +
					"must not override an explicit key even though the value (50) is well " +
					"under the old floor")
			}
		}
	}
}

// TestConfirmedHasNoOmitemptyTag guards the property the migration's
// one-shot-ness depends on: Save must always write "confirmed" explicitly,
// true or false, so that any override this program has ever saved is
// permanently immune to the legacy backfill re-firing on it. An "omitempty"
// tag would make a fresh false-by-default override indistinguishable, on the
// next Load, from one written before the field existed — reintroducing the
// magnitude fallback as a permanent behaviour rather than a one-time
// transition.
func TestConfirmedHasNoOmitemptyTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Default()
	m := 60.0
	cfg.Roster.Minutes = []RosterOverride{
		{Code: 1, Name: "Someone", Reason: "x", SetOn: "2026-08-20", ExpectedMinutes: &m, Confirmed: false},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasKey(b, "roster") {
		t.Fatal("test setup: roster key missing from saved file")
	}
	// hasKey only walks objects, not array elements, so check the raw bytes
	// directly for the one thing that matters: the key text is present at all,
	// which it would not be under an "omitempty" tag with Confirmed false.
	if !strings.Contains(string(b), `"confirmed"`) {
		t.Error(`Save wrote a minutes override with no "confirmed" key at all — ` +
			"an omitempty tag would make this indistinguishable from a pre-migration file")
	}
}
