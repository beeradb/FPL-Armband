package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTheSnapshotLeadsWithWhatItCanDetect.
//
// The requirement is not decorative. Almost every constant this project argues over
// is worth 11 to 34 points a season and the minimum detectable effect is larger than
// that on both metrics, so "unresolved" is the *expected* result for a real effect —
// and a snapshot that buried that would let the same reader draw the same wrong
// conclusion the project has already drawn in both directions.
func TestTheSnapshotLeadsWithWhatItCanDetect(t *testing.T) {
	in := Inputs{Inference: []Inference{{
		Source: "MINHL#1", Dir: "stats/out",
		MDE: []MDE{
			{Metric: "policy", Estimator: "season-clustered (primary)", DF: 3,
				SigSeason: 147, MDESeason: 192, Primary: true, PSeason: 0.001},
			{Metric: "hold", Estimator: "start fixed, no season effect", DF: 15,
				SigSeason: 42, MDESeason: 59, Primary: true, PSeason: 0.63},
		},
	}}}
	md, values := Render(in)

	headline := strings.Index(md, "what this harness can detect")
	if headline < 0 {
		t.Fatal("no detectability section at all")
	}
	for _, later := range []string{"## Provenance", "## Model accuracy",
		"## Change since"} {
		if i := strings.Index(md, later); i >= 0 && i < headline {
			t.Errorf("%q comes before the detectability section; it has to lead", later)
		}
	}
	if !strings.Contains(md, "192") || !strings.Contains(md, "59") {
		t.Error("the minimum detectable effect is not printed")
	}
	// The interpretation must travel with the number, or a reader takes an
	// unresolved verdict for a refutation.
	if !strings.Contains(md, "unresolved") {
		t.Error("nothing tells the reader that unresolved is the expected outcome " +
			"rather than a refutation")
	}
	if values.Value["harness.minhl_1.mde_season.policy"] != "192" {
		t.Errorf("the MDE is not in the machine-readable figures, so the next "+
			"snapshot cannot diff it: got %q",
			values.Value["harness.minhl_1.mde_season.policy"])
	}
	// The figure is per comparison, and a snapshot that let it read as a property of
	// the harness would invite quoting one sweep's resolution as a project constant.
	if !strings.Contains(md, "per comparison, not per harness") {
		t.Error("the snapshot does not warn that the MDE belongs to the comparison " +
			"rather than to the harness")
	}
	if !strings.Contains(md, "source sweep") {
		t.Error("the MDE table does not name its source sweep")
	}
}

// TestAnAbsentHalfIsReportedAsAbsent.
//
// A section that quietly omitted its numbers looks much like a section that had
// nothing to say, and this project has already recorded a null result that was
// really a measurement that never ran — the fixture-results gate that silently
// returned the baseline exactly.
func TestAnAbsentHalfIsReportedAsAbsent(t *testing.T) {
	md, values := Render(Inputs{
		Inference: []Inference{{Source: "S#1", Dir: "stats/out",
			Missing: []string{"mde.csv (the minimum detectable effect)"}}},
	})
	if !strings.Contains(md, "Not measured in this snapshot") {
		t.Error("a missing harness half is not announced")
	}
	if !strings.Contains(md, "absent rather than empty") {
		t.Error("the absent-versus-empty distinction is not drawn, which is the whole " +
			"point of reporting it")
	}
	if values.Value["harness.mde.present"] != "false" {
		t.Error("absence is not recorded in the figures, so a diff could not show a " +
			"half disappearing between snapshots")
	}
	if values.Value["model.present"] != "false" {
		t.Error("an absent model half is not recorded either")
	}
}

// TestOperatorNotesSurviveASnapshotWithNoCells pins a silent discard.
//
// `-note` is the only way a human can attach "this movement is a harness fix,
// not the model" to a snapshot, and the seven-figure clean-sheet movement of
// 2026-08-14 is exactly that case. The notes block used to sit after
// renderStamp's `len(in.Sweeps) == 0` early return, so a MODEL-ONLY snapshot —
// which is what the staleness guard's own instructions tell you to produce, since
// its recipe passes `-model` and no `-cells` — dropped every note on the floor
// with no warning and no error.
//
// Silence reading as success, in the artefact whose entire purpose is provenance.
// The note the operator typed is precisely the caveat that would otherwise have to
// be passed on by word of mouth, which means eventually not passed on.
func TestOperatorNotesSurviveASnapshotWithNoCells(t *testing.T) {
	const note = "the movement is a harness fix, not the model"
	md, _ := Render(Inputs{Notes: []string{note}})
	if !strings.Contains(md, note) {
		t.Error("an operator note was discarded by a snapshot with no cells file. " +
			"That is the shape this flag exists for — a model-only snapshot is what " +
			"the staleness guard's recipe produces — and dropping it silently is " +
			"worse than rejecting the flag would be.")
	}
	if !strings.Contains(md, "Operator notes") {
		t.Error("the notes section heading is missing, so the note reads as body text")
	}

	// And still present in the case that always worked, so the fix did not simply
	// move the hole.
	md2, _ := Render(Inputs{
		Notes:  []string{note},
		Sweeps: []Sweep{{Label: "X"}},
	})
	if !strings.Contains(md2, note) {
		t.Error("an operator note was discarded by a snapshot WITH cells")
	}
}

// TestTheDiffSeparatesMovedAddedAndRemoved.
//
// Removal is the case that matters most and is easiest to omit: a figure present
// last time and absent now means a diagnostic did not run, which must never read
// like a diagnostic that found nothing.
func TestTheDiffSeparatesMovedAddedAndRemoved(t *testing.T) {
	prev := newValues()
	prev.set("model.calibration_drift.through_gw16.ratio", "0.8990")
	prev.set("model.gone_diagnostic.x.y", "1.0000")
	prev.set("harness.s_1.mde_season.hold", "59")

	in := Inputs{
		Previous: prev, PrevName: "2026-08-01-abc1234",
		Inference: []Inference{{Source: "S#1", Dir: "stats/out", MDE: []MDE{
			{Metric: "hold", Estimator: "start fixed, no season effect", DF: 15,
				SigSeason: 42, MDESeason: 61, Primary: true},
		}}},
		Model: []Diagnostic{{
			Slug: "calibration_drift", Title: "drift", Grid: "g",
			Groups: []string{"through GW16"}, Measures: []string{"ratio"},
			N:      map[string]int{"through GW16": 652},
			Values: map[string]map[string]float64{"through GW16": {"ratio": 0.925}},
		}},
	}
	md, _ := Render(in)

	if !strings.Contains(md, "Figures that moved") {
		t.Error("no moved section")
	}
	if !strings.Contains(md, "0.8990") || !strings.Contains(md, "0.9250") {
		t.Error("the before and after values are not both shown")
	}
	if !strings.Contains(md, "No longer measured") {
		t.Error("a figure that vanished is not reported; that is the case a reader " +
			"most needs told, since it means a diagnostic did not run")
	}
	if !strings.Contains(md, "gone_diagnostic") {
		t.Error("the vanished figure is not named")
	}
	// The attribution instruction is the reason the diff exists at all: it has to
	// separate "the model changed" from "the harness changed".
	if !strings.Contains(md, "fingerprint") {
		t.Error("the diff does not tell the reader to attribute a movement via the " +
			"constants fingerprint")
	}
}

// TestAnUnchangedSnapshotReportsNothingMoved.
//
// The specific hazard is a figure that differs *by construction* between any two
// snapshots — `diff.baseline` is true for the first and false for every one after —
// which would head every future diff with a spurious row. A diff whose first entry
// is always noise is one a reader learns to skip, which is the same failure as a
// non-reproducible diagnostic.
func TestAnUnchangedSnapshotReportsNothingMoved(t *testing.T) {
	build := func(prev *Values, name string) (string, *Values) {
		return Render(Inputs{
			Previous: prev, PrevName: name,
			Inference: []Inference{{Source: "S#1", Dir: "d", MDE: []MDE{
				{Metric: "hold", Estimator: "start fixed, no season effect", DF: 15,
					SigSeason: 42, MDESeason: 59, Primary: true},
			}}},
			Model: []Diagnostic{{
				Slug: "d", Title: "t", Grid: "g", Groups: []string{"one"},
				Measures: []string{"ratio"}, N: map[string]int{"one": 5},
				Values: map[string]map[string]float64{"one": {"ratio": 1.0}},
			}},
		})
	}
	_, first := build(nil, "")
	md, _ := build(first, "2026-08-01-aaaaaaa")
	if !strings.Contains(md, "Nothing moved") {
		t.Errorf("two identical snapshots did not report an unchanged result; the diff "+
			"section reads:\n%s", md[strings.Index(md, "## Change since"):])
	}
}

// TestTheFirstSnapshotSaysItIsTheFirst, rather than printing an empty diff that
// reads as "nothing moved".
func TestTheFirstSnapshotSaysItIsTheFirst(t *testing.T) {
	md, v := Render(Inputs{})
	if !strings.Contains(md, "first snapshot") {
		t.Error("a snapshot with no predecessor does not say so")
	}
	if strings.Contains(md, "Nothing moved") {
		t.Error("a first snapshot claims nothing moved, which is indistinguishable " +
			"from a real no-change result")
	}
	if v.Value["diff.baseline"] != "true" {
		t.Error("baseline status is not recorded")
	}
}

// TestTheBankCaveatTravelsWithTheData.
//
// Sweeps pin the modern five-transfer bank for every cell, which is historically
// wrong for 2022-23 and 2023-24 — a caveat that has governed roughly half of this
// project's evidence while living only in a code comment. If it is not in the
// snapshot it is not in the record.
func TestTheBankCaveatTravelsWithTheData(t *testing.T) {
	s := Sweep{Label: "S#1", Banks: []int{5},
		Seasons:  []string{"2022-23<-2021-22", "2023-24<-2022-23"},
		StartGWs: []int{1}, HasProv: true,
		Prov: Provenance{Sweep: "S#1", DeclaredArms: []string{"a"}, BankUpTo: 5},
		Arms: []Arm{{Label: "a", IsBaseline: true, Cells: 2, Declared: true, Ran: true}},
	}
	md, _ := Render(Inputs{Sweeps: []Sweep{s}})
	for _, want := range []string{"2022-23", "historically wrong",
		"only the paired differences"} {
		if !strings.Contains(md, want) {
			t.Errorf("the bank caveat is incomplete: %q is missing", want)
		}
	}
}

// TestValuesRoundTrip, since the diff is worthless if the companion file cannot be
// read back exactly.
func TestValuesRoundTrip(t *testing.T) {
	v := newValues()
	v.set("a", "1")
	v.set("b,with,commas", "hello, world")
	v.setf("c", 0.8990123, 4)
	path := filepath.Join(t.TempDir(), ValuesFile)
	if err := WriteValues(path, v); err != nil {
		t.Fatal(err)
	}
	back, err := ReadValues(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range v.Keys {
		if back.Value[k] != v.Value[k] {
			t.Errorf("%q round-tripped as %q, want %q", k, back.Value[k], v.Value[k])
		}
	}
	if back.Value["c"] != "0.8990" {
		t.Errorf("float formatting is unstable: got %q", back.Value["c"])
	}
}

// TestFindPreviousPicksByRecordedTimeNotByNameOrModificationTime.
//
// The predecessor is chosen from the `recorded_at` in each candidate's own key, and
// every other ordering anyone would reach for is wrong. Modification time is
// rewritten by a checkout, a copy or a rebase. The directory name looks
// chronological and stops being so the moment two snapshots share a day, when the
// tie falls to a hex commit carrying no time at all — the failure that once reported
// a detection threshold as having moved 186 points when nothing had moved.
//
// # Three candidates, with the newest in the lexical MIDDLE
//
// Two cannot do this job, and the sibling test in watched_test.go records why: with
// two, the wanted answer is either the first entry read or the last, so the test
// passes for some wrong implementation by coincidence. Here every wrong rule lands
// somewhere else:
//
//	first-wins / lexical-min / no ordering → 2026-08-01-aaaaaaa (oldest)
//	lexical-max / name-date / readdir-last → 2026-08-05-zzzzzzz (middling)
//	modification time                      → 2026-08-01-aaaaaaa (touched last)
//	recorded_at                            → 2026-08-03-mmmmmmm (newest)  ✓
//
// # No git repository, and that is the subject rather than a shortcut
//
// The predecessor of this test built a real repository, because the rule under test
// resolved each directory's short commit with `git show`. Nothing here does: the
// three names carry `aaaaaaa`, `mmmmmmm` and `zzzzzzz`, which are not commits and
// are not even hex, so under the old rule ALL THREE dangle and the whole series is
// unorderable. It orders anyway. That is the property the change bought — a rebase
// preserves content while rewriting history, so it used to break a ranking that
// depends on nothing it touches.
func TestFindPreviousPicksByRecordedTimeNotByNameOrModificationTime(t *testing.T) {
	root := t.TempDir()
	at := func(h int) time.Time { return time.Date(2026, 8, 15, h, 0, 0, 0, time.UTC) }
	const (
		oldest   = "2026-08-01-aaaaaaa"
		newest   = "2026-08-03-mmmmmmm"
		middling = "2026-08-05-zzzzzzz"
	)
	for name, when := range map[string]time.Time{
		oldest: at(9), newest: at(17), middling: at(12),
	} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		v := newValues()
		v.set("which", name)
		if err := WriteValues(filepath.Join(dir, ValuesFile), v); err != nil {
			t.Fatal(err)
		}
		if err := WriteKey(dir, Key{Digest: "d-" + name, RecordedAt: when}); err != nil {
			t.Fatal(err)
		}
	}
	if !(oldest < newest && newest < middling) {
		t.Fatalf("the fixture no longer puts the newest record in the lexical middle")
	}
	// Touch the oldest record last, so mtime order disagrees too.
	stale := filepath.Join(root, oldest, ValuesFile)
	if err := os.Chtimes(stale, timeNow(), timeNow()); err != nil {
		t.Fatal(err)
	}

	name, v, err := FindPrevious(root, "2026-08-09-ccccccc")
	if err != nil {
		t.Fatal(err)
	}
	if name != newest {
		t.Errorf("picked %q, want %q; a name- or mtime-ordered search picks a "+
			"different record and reports the difference between them as a change",
			name, newest)
	}
	if v.Value["which"] != newest {
		t.Errorf("read %q's figures, want %q's", v.Value["which"], newest)
	}

	// Excluding the directory being written now, or a re-run diffs against itself
	// and always reports nothing moved. A same-day re-run at the same commit reuses
	// the directory name AND finds a key already sitting in it, so this exclusion
	// does more work under the content key than it did under the commit rule.
	if n, _, _ := FindPrevious(root, newest); n != middling {
		t.Errorf("the excluded directory was not excluded: got %q, want %q", n, middling)
	}
}

// A candidate that carries no key is not orderable, so it must not be able to win
// the ranking — and the one place where "no key" is still the right answer is a
// series of exactly one, where nothing is being ordered at all.
//
// This is the residue of the coverage blocker that kept the content key out of
// FindPrevious. `runSnapshot` writes figures and key together and the 51 historical
// candidates were backfilled, so the mixed state should not arise; if it ever does,
// silently ranking an unkeyed directory last is the behaviour, and diffing against
// the wrong baseline is what that prevents.
func TestAnUnkeyedCandidateCannotWinTheRanking(t *testing.T) {
	root := t.TempDir()
	write := func(name string, key bool, when time.Time) {
		t.Helper()
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		v := newValues()
		v.set("which", name)
		if err := WriteValues(filepath.Join(dir, ValuesFile), v); err != nil {
			t.Fatal(err)
		}
		if key {
			if err := WriteKey(dir, Key{Digest: "d-" + name, RecordedAt: when}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// The unkeyed one sorts last by name, so a rule that fell back to lexical order
	// would pick it.
	write("2026-08-01-keyed", true, time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC))
	write("2026-08-09-unkeyed", false, time.Time{})

	if n, _, err := FindPrevious(root, ""); err != nil || n != "2026-08-01-keyed" {
		t.Errorf("picked (%q, %v), want 2026-08-01-keyed; an unkeyed directory cannot "+
			"be placed in the series and must not win it", n, err)
	}
}

// TestTheThreeBaselineOutcomesStayDistinct.
//
// Three states that a single "" answer would conflate, and conflating them is how the
// fallback this replaces got written:
//
//   - an empty series, which legitimately has no baseline;
//   - a series of exactly one, which has a baseline and needs no ordering to find it,
//     so it is returned whether or not it carries a key;
//   - several candidates none of which carries a key, where a baseline EXISTS and
//     cannot be identified. That is an error, because every way of guessing which one
//     reduces to the lexical rule, which is what produced the 186-point phantom
//     movement.
//
// The middle case is the one that is easy to lose. Refusing it would fail every
// snapshot root written before the key existed, where nothing is ambiguous.
//
// A reinstated quiet guess is caught by the error check on the last case, whose
// failure message prints the directory that was picked — so the specific wrong answer
// is named in the output rather than merely refused.
func TestTheThreeBaselineOutcomesStayDistinct(t *testing.T) {
	base := t.TempDir()

	write := func(root, name string) {
		t.Helper()
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		v := newValues()
		v.set("which", name)
		if err := WriteValues(filepath.Join(dir, ValuesFile), v); err != nil {
			t.Fatal(err)
		}
	}

	// A root that does not exist yet: the first snapshot of all.
	missing := filepath.Join(base, "nothing-here")
	if n, v, err := FindPrevious(missing, ""); err != nil || n != "" || v != nil {
		t.Errorf("an absent root gave (%q, %v, %v); the first snapshot in a series has "+
			"no baseline and that is not an error", n, v, err)
	}

	// A root holding directories that are not snapshots. The series is still empty,
	// so this must read as "no predecessor" rather than as an unidentifiable one —
	// stats/snapshots really does hold banked-cell directories with no figures file.
	root := filepath.Join(base, "snapshots")
	if err := os.MkdirAll(filepath.Join(root, "2026-08-02-cells", "cells"), 0o755); err != nil {
		t.Fatal(err)
	}
	if n, _, err := FindPrevious(root, ""); err != nil || n != "" {
		t.Errorf("a root with no figures file gave (%q, %v), want (\"\", nil)", n, err)
	}

	// One candidate, carrying no key. Unambiguous, so it is the answer: there is
	// nothing to order it against.
	write(root, "2026-08-01-aaaaaaa")
	if n, v, err := FindPrevious(root, ""); err != nil || n != "2026-08-01-aaaaaaa" || v == nil {
		t.Errorf("a lone candidate gave (%q, %v); it is the predecessor by "+
			"construction, and refusing it fails every snapshot root written before "+
			"the key existed", n, err)
	}

	// A second candidate, also unkeyed. Now there IS an ordering to get wrong.
	write(root, "2026-08-05-fffffff")
	n, v, err := FindPrevious(root, "")
	if err == nil {
		t.Fatalf("picked %q with no error; neither candidate carries a key, so the "+
			"baseline is unknown and choosing one silently is the 186-point bug", n)
	}
	if n != "" || v != nil {
		t.Errorf("returned (%q, %v) beside the error; a failed lookup must return nothing", n, v)
	}
	// The message has to tell an operator what happened and what to do about it.
	// Asserted structurally rather than on the 186 itself: this project corrects
	// figures in place, and a test pinning the number makes the error harder to
	// correct rather than easier.
	for _, want := range []string{"-previous", "cannot be identified"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
}

// timeNow is a local helper so the test above does not import time solely to build
// one timestamp.
func timeNow() time.Time { return time.Now().Add(time.Hour) }
