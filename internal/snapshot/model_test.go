package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAnUnregisteredDiagnosticStillAppears.
//
// A lookup table invites the failure where a new diagnostic's rows are silently
// dropped because nobody added its prose. That would be a diagnostic that ran and
// produced nothing visible, which is the one state this artefact exists to make
// impossible.
func TestAnUnregisteredDiagnosticStillAppears(t *testing.T) {
	path := writeModelFixture(t, [][]string{
		{"brand_new_thing", "r1", "some grid", "a group", "12", "ratio", "0.75"},
	})
	got, err := ReadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected the unregistered diagnostic to survive, got %d tables", len(got))
	}
	if !strings.Contains(got[0].Title, "no description registered") {
		t.Errorf("title %q does not flag the missing prose; a reader would take the "+
			"slug for a considered heading", got[0].Title)
	}
	md, _ := Render(Inputs{Model: got})
	if !strings.Contains(md, "brand_new_thing") || !strings.Contains(md, "0.75") {
		t.Error("the unregistered diagnostic's numbers are not in the snapshot")
	}
}

// TestAMissingMeasureIsADashNotAZero.
//
// Zero is a value. A blank is an absence. This project priced that distinction once
// already, when an unset penalty multiplied a score by zero instead of being a
// no-op and zeroed every congested player for two seasons of replays.
func TestAMissingMeasureIsADashNotAZero(t *testing.T) {
	path := writeModelFixture(t, [][]string{
		{"d", "r1", "g", "group one", "10", "ratio", "0.9"},
		{"d", "r1", "g", "group one", "10", "bias", "-0.1"},
		{"d", "r1", "g", "group two", "10", "ratio", "1.1"},
		// group two has no bias row at all.
	})
	got, err := ReadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	md, values := Render(Inputs{Model: got})
	if !strings.Contains(md, "—") {
		t.Error("a measure absent for one group did not render as a dash; a zero there " +
			"would read as a measured absence of bias")
	}
	if _, ok := values.Value["model.d.group_two.bias"]; ok {
		t.Error("an unmeasured cell was written to the figures file, where a diff " +
			"would treat it as a real value that later changed")
	}
}

// TestAStaleModelHeaderIsRefused.
//
// Appending rows under a header from a different build produces a file that is
// ragged rather than obviously broken, and the likely reader is a half-remembered
// path from last week. This is the same lesson as "a cache version bump is not a
// schema check": the check has to be on the shape, at open time.
func TestAStaleModelHeaderIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.csv")
	if err := os.WriteFile(path,
		[]byte("diagnostic,group,value\nd,g,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadModel(path); err == nil {
		t.Error("a file missing the measure column was accepted as a model CSV")
	} else if !strings.Contains(err.Error(), "measure") {
		t.Errorf("the error does not name the missing column: %v", err)
	}
}

// TestALaterRunOverwritesAnEarlierOne, and the reason is the opposite of the cells
// file's.
//
// A cells row is a *sample*, so two runs must stay two samples or pooling them
// manufactures confidence. A model measurement is a deterministic function of a
// fixed archive, so two runs of it are the same number unless the model changed — in
// which case the later one is the one that ships.
func TestALaterRunOverwritesAnEarlierOne(t *testing.T) {
	path := writeModelFixture(t, [][]string{
		{"d", "old-run", "g", "group", "10", "ratio", "0.500"},
		{"d", "new-run", "g", "group", "10", "ratio", "0.900"},
	})
	got, err := ReadModel(path)
	if err != nil {
		t.Fatal(err)
	}
	if v := got[0].Values["group"]["ratio"]; v != 0.9 {
		t.Errorf("got %v, want the later run's 0.9", v)
	}
}

// TestReadingNotesTravelWithEveryRegisteredTable.
//
// A table of bare numbers is unreadable six months later: "ratio 0.899" means
// nothing to somebody who has to guess whether high is good. Every registered
// diagnostic must say what it measures and which direction is better.
func TestReadingNotesTravelWithEveryRegisteredTable(t *testing.T) {
	for slug, meta := range registry {
		if strings.TrimSpace(meta.title) == "" {
			t.Errorf("%s has no title", slug)
		}
		if strings.TrimSpace(meta.what) == "" {
			t.Errorf("%s does not say what it measures", slug)
		}
		if strings.TrimSpace(meta.reading) == "" {
			t.Errorf("%s does not say which direction is better", slug)
		}
		// Naming the good direction is the specific thing that has to be present,
		// because it is the thing a reader cannot infer.
		low := strings.ToLower(meta.reading)
		if !strings.Contains(low, "better") && !strings.Contains(low, "over-pred") &&
			!strings.Contains(low, "over-rated") && !strings.Contains(low, "over-credit") &&
			!strings.Contains(low, "perfect") {
			t.Errorf("%s's reading note does not say which direction is good", slug)
		}
	}
}

func writeModelFixture(t *testing.T, rows [][]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.csv")
	var b strings.Builder
	b.WriteString(strings.Join(
		[]string{"diagnostic", "run_id", "grid", "group", "n", "measure", "value"}, ","))
	b.WriteString("\n")
	for _, r := range rows {
		b.WriteString(strings.Join(r, ","))
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
