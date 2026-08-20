package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSnapshotDirSurvivesAnOlderConfigFile pins SnapshotDir's deliberate lack
// of a Load backfill: Default() already leaves it "", so a config.json written
// before the field existed unmarshals onto that default and stays "" — the
// correct disabled state, not an omission needing correction. And a file that
// does set it gets the value back verbatim, with no path manipulation or
// default substitution.
func TestSnapshotDirSurvivesAnOlderConfigFile(t *testing.T) {
	if Default().SnapshotDir != "" {
		t.Fatal("Default() must leave SnapshotDir empty — the disabled state")
	}

	dir := t.TempDir()

	oldPath := filepath.Join(dir, "old-config.json")
	old := `{"entry_id":1,"cache_dir":".cache/fpl"}`
	if err := os.WriteFile(oldPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(oldPath)
	if err != nil {
		t.Fatalf("an older config file predating snapshot_dir must still load: %v", err)
	}
	if cfg.SnapshotDir != "" {
		t.Errorf("an absent snapshot_dir must load as empty (disabled), got %q", cfg.SnapshotDir)
	}

	newPath := filepath.Join(dir, "new-config.json")
	withSnapshot := `{"entry_id":1,"cache_dir":".cache/fpl","snapshot_dir":"/archive/current"}`
	if err := os.WriteFile(newPath, []byte(withSnapshot), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(newPath)
	if err != nil {
		t.Fatalf("a config file setting snapshot_dir must load: %v", err)
	}
	if cfg.SnapshotDir != "/archive/current" {
		t.Errorf("snapshot_dir must round-trip verbatim, got %q", cfg.SnapshotDir)
	}
}
