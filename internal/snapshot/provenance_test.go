package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAKilledArmIsVisible is the reason the provenance file exists.
//
// A sweep killed under load leaves a cells file containing however many arms
// finished. Nothing in the cells file says how many were asked for, so three arms
// of six reads downstream as a complete three-arm sweep — and that has happened:
// one block was killed four times, at 1, 3, 3 and 4 arms of 6, and the gap was
// noticed only because somebody counted by hand.
//
// The declaration is written before the first cell precisely so that a kill leaves
// it behind. A test that only checked a completed sweep would pass while the
// feature was useless.
func TestAKilledArmIsVisible(t *testing.T) {
	dir := t.TempDir()
	cells := filepath.Join(dir, "cells.csv")

	if err := WriteProvenance(ProvenancePath(cells), Provenance{
		Sweep: "SWEEP#1", RunID: "r1", Commit: "abc123def456", Digest: "deadbeef1234",
		Seasons: []string{"2024-25<-2023-24"}, StartGWs: []int{1, 11},
		BankUpTo:     5,
		DeclaredArms: []string{"shipped", "alternative A", "alternative B"},
	}); err != nil {
		t.Fatal(err)
	}

	// Only two of the three arms emitted cells: arm B died.
	writeCellsFixture(t, cells, [][]string{
		{"SWEEP#1", "r1", "shipped", "0", "true", "2024-25", "2023-24", "1", "38", "5", "false", "1900", "1800", "40", "4"},
		{"SWEEP#1", "r1", "shipped", "0", "false", "2024-25", "2023-24", "11", "28", "5", "false", "1400", "1300", "30", "2"},
		{"SWEEP#1", "r1", "alternative A", "1", "false", "2024-25", "2023-24", "1", "38", "5", "false", "1910", "1800", "41", "4"},
		{"SWEEP#1", "r1", "alternative A", "1", "false", "2024-25", "2023-24", "11", "28", "5", "false", "1410", "1300", "31", "2"},
	})

	rows, err := ReadCells(cells)
	if err != nil {
		t.Fatal(err)
	}
	prov, err := ReadProvenance(ProvenancePath(cells))
	if err != nil {
		t.Fatal(err)
	}
	sweeps := GroupSweeps(rows, prov)
	if len(sweeps) != 1 {
		t.Fatalf("expected one sweep, got %d", len(sweeps))
	}
	s := sweeps[0]
	if !s.HasProv {
		t.Fatal("provenance was written and not joined; the key must be (sweep, run_id)")
	}
	if len(s.Arms) != 3 {
		t.Fatalf("expected the killed arm to still appear, got %d arms", len(s.Arms))
	}
	killed := s.Arms[2]
	if killed.Label != "alternative B" {
		t.Errorf("declaration order was lost: third arm is %q, so a killed arm in the "+
			"middle of a ladder would move rather than stay in place", killed.Label)
	}
	if killed.Ran {
		t.Error("an arm that emitted no cells is reported as having run")
	}

	// And it must reach the reader. A structural field nobody prints is not a fix.
	md, _ := Render(Inputs{Sweeps: sweeps})
	if !strings.Contains(md, "KILLED") {
		t.Error("the snapshot does not say an arm was killed; silence reading as " +
			"success is exactly the failure this is for")
	}
	if !strings.Contains(md, "incomplete") {
		t.Error("the snapshot does not warn that the grid is incomplete")
	}
}

// TestCellsWithoutProvenanceSayTheyAreUnattributable.
//
// The frozen fixture in stats/testdata is such a file, and so is every cells file
// produced before the stamping existed. Reading one is fine; reading one *without
// being told* is how an orphaned measurement becomes a citation.
func TestCellsWithoutProvenanceSayTheyAreUnattributable(t *testing.T) {
	dir := t.TempDir()
	cells := filepath.Join(dir, "cells.csv")
	writeCellsFixture(t, cells, [][]string{
		{"OLD#1", "r0", "shipped", "0", "true", "2024-25", "2023-24", "1", "38", "5", "false", "1900", "1800", "40", "4"},
		{"OLD#1", "r0", "other", "1", "false", "2024-25", "2023-24", "1", "38", "5", "false", "1910", "1800", "41", "4"},
	})
	rows, err := ReadCells(cells)
	if err != nil {
		t.Fatal(err)
	}
	sweeps := GroupSweeps(rows, map[string]Provenance{})
	if sweeps[0].HasProv {
		t.Fatal("claimed provenance where none exists")
	}
	md, _ := Render(Inputs{Sweeps: sweeps})
	for _, want := range []string{"No provenance sidecar", "unattributable",
		"cannot be checked"} {
		if !strings.Contains(md, want) {
			t.Errorf("the snapshot does not contain %q, so a reader would not know the "+
				"figures have no stamp", want)
		}
	}
}

// TestProvenanceAppendsRatherThanOverwriting.
//
// Several sweeps run in one session and losing the earlier ones is the failure
// mode — the same reason the cells file appends. Two runs of the same block must
// also stay two samples: pooling them would shrink a standard error while adding no
// information, which is manufactured confidence.
func TestProvenanceAppendsRatherThanOverwriting(t *testing.T) {
	dir := t.TempDir()
	path := ProvenancePath(filepath.Join(dir, "cells.csv"))
	for _, p := range []Provenance{
		{Sweep: "A#1", RunID: "r1", Digest: "aaa", DeclaredArms: []string{"x"}},
		{Sweep: "B#2", RunID: "r1", Digest: "bbb", DeclaredArms: []string{"y"}},
		{Sweep: "A#1", RunID: "r2", Digest: "ccc", DeclaredArms: []string{"z"}},
	} {
		if err := WriteProvenance(path, p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ReadProvenance(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 distinct (sweep, run_id) records, got %d: two runs of one "+
			"block must not merge", len(got))
	}
	if got["A#1\x00r1"].Digest != "aaa" || got["A#1\x00r2"].Digest != "ccc" {
		t.Error("two runs of the same sweep label were confused; run_id must be part " +
			"of the key")
	}
}

// TestProvenancePathIsDerivedOnce.
//
// The .means.csv path was once derived by two different rules — Go trimmed a ".csv"
// suffix and R substituted an anchored regex — which agree only when the path ends
// in ".csv". Given a path that did not, R resolved the cells file itself and the run
// died *after* the replay had been paid for. Nothing outside Go reads provenance, so
// there is one rule; this pins it against the same input that broke the other one.
func TestProvenancePathIsDerivedOnce(t *testing.T) {
	for in, want := range map[string]string{
		"/tmp/cells.csv":   "/tmp/cells" + provenanceSuffix,
		"/tmp/cells":       "/tmp/cells" + provenanceSuffix,
		"/tmp/a.csv/b.csv": "/tmp/a.csv/b" + provenanceSuffix,
	} {
		if got := ProvenancePath(in); got != want {
			t.Errorf("ProvenancePath(%q) = %q, want %q", in, got, want)
		}
		if got := ProvenancePath(in); got == in {
			t.Errorf("ProvenancePath(%q) returned the cells path itself, which is how "+
				"the .means.csv mismatch destroyed a finished run", in)
		}
	}
}

// writeCellsFixture writes a cells CSV in the *current* schema, with the layer and
// captaincy-rung columns blank.
//
// Blank rather than zero, deliberately: an unmeasured layer and a layer measured at
// zero are different facts, and only one of them is a number anything should
// average.
func writeCellsFixture(t *testing.T, path string, rows [][]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("sweep,run_id,variant,variant_index,is_baseline,season,prior_season," +
		"start_gw,weeks,bank_up_to,infeasible,policy_points,hold_points,moves,hits," +
		"policy_per_gw,hold_per_gw,frozen_points,frozen_per_gw,frozen_captain_points," +
		"frozen_captain_per_gw,weekly_points,weekly_per_gw,hold_fixedcap_points," +
		"hold_fixedcap_per_gw,hold_nocap_points,hold_nocap_per_gw\n")
	for _, r := range rows {
		b.WriteString(strings.Join(r, ","))
		b.WriteString(",,,,,,,,,,,,\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
