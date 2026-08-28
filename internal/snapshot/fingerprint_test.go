package snapshot

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"armband/internal/config"
)

// TestFingerprintMovesWithAConstantAndNotOtherwise pins the two properties the
// whole stamp rests on.
//
// If the digest does not move when a scoring constant moves, two sweeps measured
// under different models are reported as comparable — which is the failure that
// let a body of evidence measured at a minimum-gain threshold of 0.7 be cited as
// ground truth after the value was retracted to 0.4. And if it moves when nothing
// relevant changed, every snapshot diff reads as "the model changed" and the
// signal is gone.
func TestFingerprintMovesWithAConstantAndNotOtherwise(t *testing.T) {
	cfg := config.Default()
	base, err := FingerprintOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if base.Digest == "" {
		t.Fatal("empty digest")
	}

	again, err := FingerprintOf(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	if again.Digest != base.Digest {
		t.Errorf("the same config fingerprinted twice gave %s then %s; the walk is "+
			"not deterministic, so every snapshot diff will claim the model changed",
			base.Digest, again.Digest)
	}

	moved := config.Default()
	moved.Weights.MinutesHalfLife += 1
	mfp, err := FingerprintOf(moved)
	if err != nil {
		t.Fatal(err)
	}
	if mfp.Digest == base.Digest {
		t.Error("changing minutes_half_life did not change the fingerprint; a sweep " +
			"run under a different scoring model would be reported as comparable")
	}

	// Irrelevant fields must not move it, or the diff cries wolf. The entry id and
	// the cache window cannot change a single replayed point.
	noise := config.Default()
	noise.EntryID = 1234567
	noise.CacheMinutes = 999
	noise.ReportDir = "somewhere-else"
	nfp, err := FingerprintOf(noise)
	if err != nil {
		t.Fatal(err)
	}
	if nfp.Digest != base.Digest {
		t.Error("the entry id, cache window or report directory moved the " +
			"fingerprint; none of them can change a replayed point, so a diff would " +
			"attribute a coincidence to the model")
	}
}

// TestFingerprintCoversEnvSwitches is separate because it is the half that is easy
// to forget.
//
// A switch like FPL_NO_VICE_CAPTAIN changes what game is being scored, so a sweep
// run with it set is not comparable with one run without it — and unlike a config
// value it leaves no trace anywhere on disk. A fingerprint blind to the
// environment would call the two runs identical.
func TestFingerprintCoversEnvSwitches(t *testing.T) {
	cfg := config.Default()
	base, err := FingerprintOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FPL_NO_VICE_CAPTAIN", "1")
	with, err := FingerprintOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if with.Digest == base.Digest {
		t.Error("FPL_NO_VICE_CAPTAIN did not move the fingerprint; a run that scored " +
			"a different game would be reported as comparable with one that did not")
	}
	found := false
	for _, e := range with.Env {
		if e.Path == "FPL_NO_VICE_CAPTAIN" {
			found = true
		}
	}
	if !found {
		t.Error("the switch is not listed in Env, so the constants file would not " +
			"name it and a diff could not explain the movement")
	}

	// An empty value still counts as set. Several switches are tested for key
	// presence rather than value — the same trap as BonusPriorWeight, whose
	// disabled state is -1 and whose migration therefore has to probe for the key.
	t.Setenv("FPL_NO_VICE_CAPTAIN", "")
	empty, err := FingerprintOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Digest == base.Digest {
		t.Error("a switch set to the empty string was treated as unset; presence is " +
			"what several of these mean, so an empty value must still show")
	}
}

// TestEnvSwitchListIsComplete counts the FPL_* switches in the source against the
// list this package carries.
//
// The list is hand-maintained and this project's hand-maintained lists rot: the
// four season lists go stale every summer, an override list outlived its situation
// and kept applying, and both failures were silent. So the only safe kind of list
// here is one a test counts — the same reason TestEveryScoringEngineGetsRecency
// counts scoring engines rather than trusting a comment.
//
// A new switch failing this test is the point. It means somebody added a knob that
// changes what the model does and every snapshot taken since would have called two
// incomparable runs comparable.
func TestEnvSwitchListIsComplete(t *testing.T) {
	root := filepath.Join("..", "..")
	re := regexp.MustCompile(`FPL_[A-Z0-9_]+`)
	// Switches that are not model settings and must stay out of a committed
	// artefact or would be meaningless in one.
	skip := map[string]bool{
		"FPL_SESSION": true, // a credential; a snapshot is committed
		"FPL_CELLS":   true, // where to write, not what to compute
		// Forces the layout goldens to compare in CI, where they are otherwise
		// skipped because GitHub's runners render every shot 2/255 off the
		// committed PNGs. It gates a TEST, not a model input: nothing on the
		// scoring or replay path reads it, so a sweep run with it set computes
		// exactly what the shipped defaults compute. ⚠️ Delete this entry when
		// the goldens defect is fixed and the skip in visual_test.go goes with
		// it — an exemption outliving its switch is how this map rots.
		"FPL_LAYOUT_GOLDENS": true,
		// Where the hit-verdict sidecar is written, for
		// stats/hittune_verdicts.R. An output path: the package rows it carries
		// are the same rows already aggregated into the cells file, so setting
		// it cannot move a measured number.
		"FPL_HITS_CSV": true,
		// Where the CLI backtest writes its clickable replay, and how many
		// gameweeks it renders. Both are about the PAGE, not the replay: the
		// simulation runs identically whether or not a page is asked for, and
		// truncating the render cannot reach a decision, because the policy is
		// causal and GW3 never depended on GW11.
		"FPL_REPLAY_HTML": true,
		"FPL_REPLAY_GWS":  true,
		// Where the reasoning layer's verdict is read from, for rendering onto the
		// squad page. Prose about a decision, not an input to one: the model cannot
		// read it and no number moves whether it is set or not.
		"FPL_ANALYSIS_MD": true,
		"FPL_MODEL_CSV":   true, // the same
		// Where the prediction benchmark writes its per-gameweek sufficient
		// statistics. An output path, so it cannot change a measured number.
		"FPL_PREDICTION_CSV": true,
		// Where the unknown-prior ordering diagnostic writes its per-season rhos
		// and its per-position levels, for stats/unknown_prior_ranks.R. Both are
		// output paths on a DIAG-only test that runs no sweep and banks no cell,
		// so neither can move a measured number.
		"FPL_RANKS_CSV":    true,
		"FPL_LEVELS_CSV":   true,
		"FPL_INSEASON_CSV": true,
		// Where the xGC-reconstruction dump writes, and which seasons it covers.
		// Both are read ONLY by TestDiagXGCDump, which reads
		// `clubXGAPerGameweek` and writes a CSV. It computes nothing on the
		// scoring or replay path and mutates no season, so a sweep run with
		// either set computes exactly what the shipped defaults compute — the
		// same argument as FPL_REPLAY_HTML/FPL_REPLAY_GWS above, where narrowing
		// what is rendered cannot reach a decision.
		//
		// ⚠️ _SEASONS narrows the dump, not the replay. If this switch ever comes
		// to select seasons for something that SCORES, it belongs in envSwitches
		// instead — the exemption is about where it is read, not about its name.
		"FPL_XGCDUMP":         true,
		"FPL_XGCDUMP_SEASONS": true,
		// Where the clean-sheet diagnostic dumps one row per team-match, for
		// stats/cs_calibration.R to fit per observation rather than on the six
		// bucket means it prints. An output path: the rows written are the same
		// rows already aggregated into the printed table and the model CSV, so
		// setting it cannot move a measured number. It exists because a family
		// verdict was once read off the bucket-mean fit and came out backwards.
		"FPL_CS_ROWS": true,
		// Where the clean-sheet REGRESSOR diagnostic dumps one row per
		// team-gameweek, for the same R script to fit. An output path on the same
		// grounds as FPL_CS_ROWS above — and the pair is deliberate: that dump
		// carries realised single-match xGC, this one carries the model's own
		// XGC90, and the difference between the two fits is the finding. Same
		// schema, different quantity, so name which file a figure came from.
		"FPL_CSREG_ROWS": true,
		// The same diagnostic's second dump, whose `xgc` column is the PRODUCT
		// XGC90 x def — the fixture path's exponent. A third output path, and a
		// third file that shares one schema and means a third thing, which is why
		// each names its regressor in its own header.
		"FPL_CSREG_DEF_ROWS": true,
		// Where the per-position blank-run calibration dumps one row per
		// player-cutoff, for stats/blank_run_position.R. An output path on the
		// same grounds as the three above. ⚠️ The switch that DOES change what
		// that diagnostic measures is `FPL_NO_BLANK_RUN`, which is already
		// fingerprinted below — and it must be, because `ExpectedMinutes` carries
		// `blankRunFactor`, so the calibration is fitted to the term's own output
		// without it. The diagnostic refuses to run unless it is set.
		"FPL_BLANKRUN_ROWS": true,
		// Where the xGC transport diagnostic dumps one row per 900+ minute keeper
		// or defender season — both arms' reconstructed XGC90 against the same
		// truth, with the substitution-exposure tercile already assigned — for
		// stats/xgc_tercile_transport.R. A fifth output path, on the same grounds:
		// the rows are the same rows already aggregated into the printed tercile
		// table, and the CUT travels in the file rather than being re-derived in R,
		// so setting this cannot move a measured number.
		"FPL_XGC_TERCILE_CSV": true,
		"FPL_SWEEP":           true, // output formatting only
		"FPL_TRACE":           true, // output formatting only
		// The squad pipeline's stage timings, printed to stderr. Output only, on
		// the same grounds as the two above: it prints how long a stage took and
		// changes nothing a stage does, so a run with it set is the same run.
		// It is the instrument the optimiser's speedups were measured on and is
		// kept for the next one — see buildSquadPage.
		"FPL_SERVE_TIMINGS": true,
		// Where the same pipeline writes a pprof CPU profile. An output path, on
		// the same grounds as the CSV dumps above.
		"FPL_CPU_PROFILE": true,
		"FPL_SEASON":      true, // which season a CLI backtest replays
		"FPL_START_GW":    true, // which gameweek a CLI backtest enters at
		// Declares that the snapshot series is published by CI rather than
		// committed, so its absence is expected. It gates a TEST precondition and
		// cannot reach the model, which is why it is skipped rather than
		// fingerprinted — a snapshot recording it would be recording something
		// about the machine that took it, not about what was measured.
		"FPL_SNAPSHOTS_EXTERNAL": true,
	}
	have := map[string]bool{}
	for _, k := range envSwitches {
		have[k] = true
	}

	found := map[string]string{} // switch -> first file seen in
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // an unreadable directory is not this test's business
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", ".cache", "node_modules", "out", "snapshots":
				return filepath.SkipDir
			// `.claude/worktrees` holds full checkouts of OTHER commits, so
			// scanning it makes this test assert a property of code that is not
			// in this build. It fired for real: FPL_CHIP_PLAN was reported as
			// unregistered, "first seen in
			// ../../.claude/worktrees/prior-half-life-on-repaired-xgc/...", on a
			// tree where no such switch exists — a red scan of the wrong tree,
			// which is the inverse of the hazard AGENTS.md already carries a
			// standing rule about. It cries wolf while any worktree exists, and a
			// guard that fails for reasons unrelated to what it guards gets
			// disabled, after which it guards nothing.
			//
			// Nothing is lost by skipping the whole directory: the agent and skill
			// briefs under it are markdown, and this walk reads only .go files.
			case ".claude":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// This file names the switches it excludes, which would otherwise read as
		// uses of them.
		if strings.HasSuffix(path, "fingerprint_test.go") ||
			strings.HasSuffix(path, "fingerprint.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, m := range re.FindAllString(string(b), -1) {
			if _, seen := found[m]; !seen {
				found[m] = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var missing []string
	for sw, where := range found {
		if skip[sw] || have[sw] {
			continue
		}
		missing = append(missing, sw+" (first seen in "+where+")")
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these FPL_* switches are read by the code and are not in "+
			"envSwitches, so a sweep run with any of them set would be fingerprinted "+
			"as though the shipped defaults ran:\n  %s\n\n"+
			"Add each to envSwitches, or to this test's skip map if it names an output "+
			"path rather than a model setting.", strings.Join(missing, "\n  "))
	}

	// The other direction: a switch deleted from the code but left in the list
	// makes the digest depend on a variable nothing reads.
	var stale []string
	for _, sw := range envSwitches {
		if _, ok := found[sw]; !ok {
			stale = append(stale, sw)
		}
	}
	if len(stale) > 0 {
		t.Errorf("envSwitches names switches no Go file reads: %s. Either the knob "+
			"was deleted and this entry should go, or it was renamed and the "+
			"fingerprint is now blind to it.", strings.Join(stale, ", "))
	}
}

// TestFingerprintNamesAnAbsentSubtreeRatherThanSkippingIt.
//
// If a config subtree is renamed, fingerprinting the remainder would produce a
// digest that looks perfectly healthy and covers less than it claims — the same
// shape as a cache version bump that a stale file defeats. It has to be loud.
func TestFingerprintNamesAnAbsentSubtreeRatherThanSkippingIt(t *testing.T) {
	// A stand-in config with no weights at all, which is what a rename looks like
	// from here.
	type bare struct {
		Congestion map[string]float64 `json:"congestion"`
	}
	fp, err := FingerprintOf(bare{})
	if err != nil {
		t.Fatal(err)
	}
	var sawAbsent int
	for _, c := range fp.Constants {
		if strings.Contains(c.Value, "ABSENT") {
			sawAbsent++
		}
	}
	if sawAbsent == 0 {
		t.Error("a config with no weights subtree fingerprinted silently; a rename " +
			"would then produce a healthy-looking digest covering less than it claims")
	}
}

// TestSliceFingerprintIgnoresOrderButNotContent.
//
// The hand-maintained season lists are sets, so reordering the clubs in one is the
// same model and must not read as a change — a diff that cried wolf on a reorder
// would train its reader to ignore it. Editing a name is a real change and must.
func TestSliceFingerprintIgnoresOrderButNotContent(t *testing.T) {
	a := flatten("x", []any{"ARS", "LIV", "MCI"})
	b := flatten("x", []any{"MCI", "ARS", "LIV"})
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected one summarised leaf each, got %d and %d", len(a), len(b))
	}
	if a[0].Value != b[0].Value {
		t.Errorf("reordering a club list changed its fingerprint (%q against %q); the "+
			"replay cannot see an ordering here, so the diff would report a "+
			"difference that does not exist", a[0].Value, b[0].Value)
	}
	c := flatten("x", []any{"ARS", "LIV", "NEW"})
	if a[0].Value == c[0].Value {
		t.Error("changing a club in the list did not change its fingerprint; a summer " +
			"maintenance edit would then be invisible in every snapshot diff")
	}
}

// The chip plan's fingerprint PATHS must not move for a single-set plan.
//
// `chip_plan` is a model subtree, so `flatten` walks it and every key becomes a
// constant path. When the plan became a two-set type, the obvious marshalling
// turns `chip_plan.wildcard_gameweek` into `chip_plan.first.wildcard_gameweek`
// plus four new `chip_plan.second.*` zero rows — so every constant digest
// recorded before that becomes non-comparable to one after, on a change that
// moves no cell. `fingerprint.go` names this exact hazard: "a changed stamp over
// byte-identical cells, which is the byte-identical null inverted".
//
// `ChipSchedule.MarshalJSON` keeps the flat form when there is no second set,
// which is what holds the paths still. This pins that, because the property is
// a consequence of the marshaller rather than of anything local. Found by review.
func TestASingleSetChipPlanDoesNotMoveTheFingerprintPaths(t *testing.T) {
	cfg := config.Default()
	cfg.Chips.First.Wildcard = 6
	cfg.Chips.First.BenchBoost = 8

	fp, err := FingerprintOf(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var chip []string
	for _, c := range fp.Constants {
		if strings.HasPrefix(c.Path, "chip_plan") {
			chip = append(chip, c.Path)
		}
	}
	sort.Strings(chip)

	want := []string{
		"chip_plan.bench_boost_gameweek",
		"chip_plan.free_hit_gameweek",
		"chip_plan.triple_captain_gameweek",
		"chip_plan.wildcard_gameweek",
	}
	if strings.Join(chip, ",") != strings.Join(want, ",") {
		t.Errorf("chip_plan fingerprint paths are now\n  %v\nwant\n  %v\n\n"+
			"Changed paths make every digest recorded before this commit "+
			"non-comparable to one after, on a change that moves no cell.", chip, want)
	}
}

// TestEnvDigestSeparatesSetFromUnset is the bite test for the fourth recorded
// comparability failure, reproduced as its own signature.
//
// `FPL_XGC_EXTERNAL_DIR` was SET for one run and UNSET for the other. Nothing was
// dirty. No commit differed. Both runs were individually correct. The two were
// differenced anyway and a published verdict flipped sides — the ceiling read
// -0.693 against a 0.450 threshold one way and -0.331 against 0.554 the other, so
// the only cell that cleared its threshold was the one that was not robust.
//
// ⚠️ What makes this fire is that the digest covers the ENVIRONMENT at all, and
// writes each switch's name into it. An earlier draft of this test claimed the
// load-bearing property was writing an explicit "(unset)" line for absent
// switches. ⚠️ **That claim was false and the test did not hold it**: removing
// the line left this test green, because the names of the set switches already
// separate the arms. The claim was withdrawn only because the removal was
// injected and the test was watched for a failure that never came.
func TestEnvDigestSeparatesSetFromUnset(t *testing.T) {
	for _, k := range envSwitches {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}

	unset := CurrentEnv()
	if len(unset.Set) != 0 {
		t.Fatalf("a switch survived unsetting: %v", unset.Set)
	}

	t.Setenv("FPL_XGC_EXTERNAL_DIR", t.TempDir())
	set := CurrentEnv()

	if set.Digest == unset.Digest {
		t.Fatal("the env digest is equal with FPL_XGC_EXTERNAL_DIR set and unset — " +
			"this is the fourth comparability failure exactly, and the two arms it " +
			"describes flipped a published verdict")
	}
	if len(set.Set) != 1 {
		t.Fatalf("expected exactly the one switch set, got %v", set.Set)
	}
	// ⚠️ And it must still be digested on the way out: this value reaches a
	// committed sidecar and a pasted table, and the directory it names is an
	// unlicensed source.
	if !strings.HasPrefix(set.Set[0].Value, "data:") {
		t.Fatalf("a path-valued switch reached the record undigested: %q", set.Set[0].Value)
	}

	// Two runs whose xGC source holds DIFFERENT DATA must differ too. This is the
	// same failure one layer up: equal digests here would let sweep_inference.R
	// call two arms comparable on data one of them never read.
	loaded := t.TempDir()
	if err := os.WriteFile(filepath.Join(loaded, "x.json"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FPL_XGC_EXTERNAL_DIR", loaded)
	if other := CurrentEnv(); other.Digest == set.Digest {
		t.Fatal("two xGC sources holding different data produced one env digest")
	}

	// ⚠️ And the converse, which is a DELIBERATE semantic change and not an
	// oversight. Two different paths holding IDENTICAL contents now digest the
	// same, where the old path-string hash told them apart. That is the right way
	// round: the question a comparability guard asks is "did the data differ",
	// not "did the string differ", and a cache copied to a second location is the
	// same data. The cost is that a run cannot be attributed to a location from
	// the sidecar — which was already true and deliberate, because the location
	// is a host path that may not be published.
	twin := t.TempDir()
	if err := os.WriteFile(filepath.Join(twin, "x.json"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FPL_XGC_EXTERNAL_DIR", twin)
	twinEnv := CurrentEnv()
	t.Setenv("FPL_XGC_EXTERNAL_DIR", loaded)
	if twinEnv.Digest != CurrentEnv().Digest {
		t.Fatal("two paths holding identical data produced different digests — the digest is " +
			"reading the path again rather than the contents")
	}
}
