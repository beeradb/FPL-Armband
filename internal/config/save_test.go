package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Saving the repository's own config.json must not rewrite its chip plan.
//
// The chip plan became a two-set type, and the obvious implementation writes the
// new `{"first":…,"second":…}` form unconditionally. That would rewrite every
// existing config.json on the first save — the agent persists roster overrides,
// so this happens in ordinary use — turning up as an unexplained diff on a
// tracked file. Worse, it is a one-way door: anything still typed
// `analysis.ChipPlan` reads the two-set object as all zeros **with no error**,
// which is the byte-identical null this project keeps being caught by.
//
// Checked against the real file rather than a fixture, because a fixture would be
// a second copy of the thing being checked and would not notice the day the real
// one changes shape. Found by the security review of the two-set change.
func TestSavingDoesNotRewriteTheCheckedInChipPlan(t *testing.T) {
	path := filepath.Join("..", "..", "config.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no config.json at the repository root: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("the checked-in config no longer loads: %v", err)
	}

	out := filepath.Join(t.TempDir(), "config.json")
	if err := Save(out, cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	chipPlan := func(b []byte, what string) json.RawMessage {
		t.Helper()
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s is not an object: %v", what, err)
		}
		return m["chip_plan"]
	}

	// Compared as re-encoded values rather than as raw bytes: Save indents from
	// scratch, so whitespace legitimately differs and only the content matters.
	norm := func(r json.RawMessage) string {
		t.Helper()
		var v any
		if err := json.Unmarshal(r, &v); err != nil {
			t.Fatalf("chip_plan is not valid JSON: %v", err)
		}
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	before := norm(chipPlan(raw, "config.json"))
	after := norm(chipPlan(saved, "the saved config"))
	if before != after {
		t.Errorf("saving rewrote the chip plan\n  before: %s\n   after: %s\n\n"+
			"A single-set plan must round-trip in the flat form. Rewriting it "+
			"churns a tracked file and makes the plan unreadable to anything still "+
			"typed ChipPlan, which loses it silently rather than erroring.",
			before, after)
	}
}

// Save must never leave a partial file where a whole one used to be.
//
// It writes a sibling and renames, so a reader sees the old file or the new one.
// This pins the observable half: the target is never absent, and what lands is
// always loadable.
func TestSaveReplacesTheFileWholeAndLeavesNoLitter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := Default()
	cfg.Chips.First.Wildcard = 6
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("saving over an existing config: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("the saved config does not load back: %v", err)
	}
	if got.Chips.First.Wildcard != 6 {
		t.Errorf("round trip lost the wildcard: %+v", got.Chips)
	}

	// The temporary sibling must not survive. A directory filling with
	// .config-*.json is how a "harmless" temp file becomes an operational
	// problem nobody attributes to the writer.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("save left litter behind: %v", names)
	}
}
