package snapshot

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestThisPackageDoesNotComputeInference is the companion to
// TestInferenceLivesInOnePlace in internal/backtest, which scans only its own
// directory and so cannot see this one.
//
// The rule is that every standard error, degree of freedom and minimum detectable
// effect comes from stats/*.R and nowhere else. The temptation here is specific and
// strong: the MDE is two lines of arithmetic given the components, and this package
// already reads the components. Yielding to it would be the bug class behind
// DefaultBenchWeight against Weights.BenchWeight — one quantity with two
// implementations, where the measured one turned out not to be the one that ran.
//
// Source-scanning rather than a runtime assertion, for the same reason the existing
// guards are: the failure is a *new* file quietly adding a copy, and the copy agrees
// with the original on the day it is written.
func TestThisPackageDoesNotComputeInference(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Assembled from fragments so this file does not match its own scanner.
	patterns := map[string]*regexp.Regexp{
		"a t-distribution quantile": regexp.MustCompile(`\bqt\s*\(`),
		"a critical value":          regexp.MustCompile(`t` + `Crit\s*[:=]`),
		"a standard error":          regexp.MustCompile(`\b(se|SE)` + `Of\s*\(|standardError`),
		"a p-value":                 regexp.MustCompile(`pValue|p` + `Value\s*[:=]`),
		"a variance from residuals": regexp.MustCompile(`sumSquares|meanSquare`),
	}
	self := "inference_test.go"
	var offenders []string
	for _, f := range files {
		if filepath.Base(f) == self {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for what, re := range patterns {
			if re.Match(b) {
				offenders = append(offenders, filepath.Base(f)+" appears to compute "+what)
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("inference belongs in stats/variance_components.R and this package "+
			"only reads what it publishes; found:\n  %s\n\n"+
			"If a new figure is wanted, add it to the R script's output tables and read "+
			"it here. Two implementations of one quantity is how the measured value "+
			"stops being the one that runs.", strings.Join(offenders, "\n  "))
	}
}

// TestThePrimaryEstimatorIsRead, not chosen here.
//
// Which estimator is licensed depends on whether the season component exists, which
// is an F test — so R decides and marks the row. If Go picked instead, the snapshot
// could report three degrees of freedom where fifteen were licensed, which is the
// difference between a 62-point and a 42-point threshold on the held metric.
func TestThePrimaryEstimatorIsRead(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, filepath.Join(dir, "mde.csv"),
		"estimator,se,df,t_crit,sig_gw,sig_season,mde_gw,mde_season,metric,is_primary,p_season_pooled,power,alpha,season_gws",
		`"season-clustered (primary)",0.515,3,3.182,1.639,62.3,2.143,81.4,"hold",FALSE,0.80,0.8,0.05,38`,
		`"start fixed, no season effect",0.515,15,2.131,1.098,41.7,1.544,58.7,"hold",TRUE,0.80,0.8,0.05,38`,
	)
	in, err := ReadInference(dir)
	if err != nil {
		t.Fatal(err)
	}
	row, ok := in.PrimaryMDE("hold")
	if !ok {
		t.Fatal("no primary row found; R marks exactly one and it must be honoured")
	}
	if row.DF != 15 {
		t.Errorf("primary df is %d, want 15 — the row R licensed, not the first one",
			row.DF)
	}
	if row.MDESeason != 58.7 {
		t.Errorf("primary MDE is %v, want 58.7", row.MDESeason)
	}
}

// TestTheThresholdIsReadPerArmAndNotOnlyPooled.
//
// The defect this guards against was live for weeks. R pooled the variance
// components across a sweep's arms with a plain mean and reported one threshold per
// metric, and on the minutes-half-life grid the `flat (no recency)` arm — the one
// that turns the feature off — contributed 72% of the pooled season variance on the
// transfer metric. So the widely-quoted 147 points a season was that one arm's 232
// applied to three arms whose own figures are 80, 90 and 133.
//
// The fixture below is those real numbers, trimmed to two arms. What is asserted is
// that each arm keeps its own figure, that both ends of the bracket survive, and
// that the pooled row is still readable but no longer the only thing on the page.
func TestTheThresholdIsReadPerArmAndNotOnlyPooled(t *testing.T) {
	dir := t.TempDir()
	head := "scope,block,metric,variant,estimator,se,df,t_crit,sig_gw,sig_season," +
		"mde_gw,mde_season,f_season,p_season,f_power,is_primary,p_season_pooled," +
		"power,alpha,season_gws"
	writeCSV(t, filepath.Join(dir, "mde.csv"), head,
		`arm,B#1,policy,flat (no recency),"season-clustered (primary)",1.918,3,3.182,6.103,231.9,7.980,303.2,9.637,0.00086,0.2215,FALSE,NA,0.8,0.05,38`,
		`arm,B#1,policy,flat (no recency),"start fixed, no season effect",0.618,15,2.131,1.317,50.0,1.852,70.4,9.637,0.00086,0.2215,FALSE,NA,0.8,0.05,38`,
		`arm,B#1,policy,half-life 2,"season-clustered (primary)",0.661,3,3.182,2.105,80.0,2.752,104.6,1.561,0.2402,0.2215,FALSE,NA,0.8,0.05,38`,
		`arm,B#1,policy,half-life 2,"start fixed, no season effect",0.529,15,2.131,1.128,42.9,1.585,60.2,1.561,0.2402,0.2215,FALSE,NA,0.8,0.05,38`,
		`pooled,B#1,policy,NA,"season-clustered (primary)",1.212,3,3.182,3.857,146.6,5.043,191.6,NA,NA,0.2215,TRUE,0.00086,0.8,0.05,38`,
	)
	inf, err := ReadInference(dir)
	if err != nil {
		t.Fatal(err)
	}

	arms := inf.Brackets("policy")
	if len(arms) != 2 {
		t.Fatalf("got %d per-arm brackets, want 2 — the arms were pooled away again",
			len(arms))
	}
	if arms[0].Variant != "flat (no recency)" || arms[1].Variant != "half-life 2" {
		t.Errorf("arms out of order or unnamed: %q, %q", arms[0].Variant, arms[1].Variant)
	}
	if arms[0].Clustered.SigSeason != 231.9 || arms[1].Clustered.SigSeason != 80.0 {
		t.Errorf("each arm must keep its own threshold; got %.1f and %.1f",
			arms[0].Clustered.SigSeason, arms[1].Clustered.SigSeason)
	}
	// Both ends present. Collapsing to one would re-introduce a pre-test that has
	// about 22% power against the case that would change the answer.
	if arms[0].StartFixed.SigSeason != 50.0 || arms[0].StartFixed.DF != 15 {
		t.Errorf("the start-fixed end of the bracket is missing: %.1f at %d df",
			arms[0].StartFixed.SigSeason, arms[0].StartFixed.DF)
	}
	if arms[0].Clustered.DF != 3 {
		t.Errorf("the clustered end should carry 3 df, got %d", arms[0].Clustered.DF)
	}
	if arms[0].FPower <= 0 || arms[0].FPower > 0.5 {
		t.Errorf("the season F test's power is %v; it is read from R and should be "+
			"the ~0.22 that makes a pre-test inadmissible", arms[0].FPower)
	}
	// An arm row must never be mistaken for the licensed pooled row.
	pooled, ok := inf.PrimaryMDE("policy")
	if !ok {
		t.Fatal("the pooled row disappeared; every threshold in AGENTS.md is one")
	}
	if pooled.SigSeason != 146.6 || pooled.Scope != "pooled" {
		t.Errorf("PrimaryMDE returned %v at scope %q, want the pooled 146.6",
			pooled.SigSeason, pooled.Scope)
	}

	inf.Source = "B#1"
	md, v := Render(Inputs{Inference: []Inference{inf}})
	for _, want := range []string{"flat (no recency)", "half-life 2",
		"pooled over the sweep's arms"} {
		if !strings.Contains(md, want) {
			t.Errorf("the headline does not mention %q", want)
		}
	}
	// The finest and coarsest on the page are the arms' figures, not the pooled
	// one — otherwise a reader still takes 192 as the harness's resolution.
	if v.Value["harness.b_1.mde_season.policy.half_life_2.clustered"] != "105" {
		t.Errorf("per-arm figures are not in the machine-readable record: %q",
			v.Value["harness.b_1.mde_season.policy.half_life_2.clustered"])
	}
	// The conservative end of each bracket, so the headline never rests on the
	// estimator that is only valid where the season component is zero.
	if got := v.Value["harness.mde_season.finest"]; got != "105" {
		t.Errorf("finest comparison on the page is %q, want the 105 of the "+
			"half-life 2 arm's clustered end", got)
	}
	if got := v.Value["harness.mde_season.coarsest"]; got != "303" {
		t.Errorf("coarsest comparison is %q, want the flat arm's 303", got)
	}
}

// TestAMissingTableIsNamedRatherThanIgnored.
// TestTablesFromAnotherSweepAreDetected reproduces a provenance failure that has
// now happened twice, from two different directions.
//
// `armband snapshot` defaults its cells path to a location a throwaway demo has
// written to, and its --out directory holds whatever the previous run's inference
// left there. So the two halves of a harness section can come from different
// sweeps: the numbers from R's stale tables, and the grid, labels and provenance
// from whatever cells file happened to be at the default path.
//
// The first occurrence went unnoticed for weeks — a twelve-cell demo reporting a
// mean paired difference of 6.1 points a gameweek was read as a replayed season,
// because the output named no source. The second was caught only because a label
// happened to change visibly in a diff, which is luck rather than a guard.
//
// The block column and the cells file's sweep label are the same fact from two
// places, so this project's rule applies: a duplicated quantity is a pipeline test
// when it is checked and the bug when it is merely watched.
func TestTablesFromAnotherSweepAreDetected(t *testing.T) {
	dir := t.TempDir()
	head := "scope,block,metric,variant,estimator,se,df,t_crit,sig_gw,sig_season," +
		"mde_gw,mde_season,f_season,p_season,f_power,is_primary,p_season_pooled," +
		"power,alpha,season_gws"
	writeCSV(t, filepath.Join(dir, "mde.csv"), head,
		`arm,1786303273-51573 / VICECAP#1,policy,vice on (ships),"season-clustered (primary)",0.105,3,3.182,0.333,12.7,0.436,16.6,4.81,0.015,0.2215,FALSE,NA,0.8,0.05,38`,
	)
	inf, err := ReadInference(dir)
	if err != nil {
		t.Fatal(err)
	}

	// R records the block as "<run id> / <LABEL>", so the label is a substring.
	if !inf.MatchesCells([]string{"VICECAP#1"}) {
		t.Error("the tables came from VICECAP#1 and the cells file says VICECAP#1; " +
			"that must not be reported as a mismatch")
	}
	// The actual failure: the cells file at the default path held a twelve-cell
	// demo labelled T#1 while these tables came from the real sweep.
	if inf.MatchesCells([]string{"T#1"}) {
		t.Error("tables computed from VICECAP#1 were accepted as belonging to a " +
			"cells file containing only T#1 — this is the demo-file provenance bug, " +
			"and every harness figure would be attributed to the wrong measurement")
	}
	// A label that is a substring of the run id must not match by accident. "5157"
	// appears inside the run id 1786303273-51573 and is not a sweep label.
	if inf.MatchesCells([]string{"MINHL#1"}) {
		t.Error("an unrelated sweep label matched")
	}

	// Tables predating the block column cannot be checked, and an unverifiable
	// claim must not be reported as a refuted one.
	old := Inference{MDE: []MDE{{Metric: "policy"}}}
	if !old.MatchesCells([]string{"VICECAP#1"}) {
		t.Error("tables with no block column report a mismatch; they should report " +
			"nothing, since there is nothing to compare")
	}
	if !inf.MatchesCells(nil) {
		t.Error("no cells labels at all should not read as a mismatch")
	}
}

func TestAMissingTableIsNamedRatherThanIgnored(t *testing.T) {
	in, err := ReadInference(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Missing) != 2 {
		t.Fatalf("expected both absent tables to be named, got %v", in.Missing)
	}
	joined := strings.Join(in.Missing, " ")
	if !strings.Contains(joined, "mde.csv") {
		t.Errorf("the missing MDE table is not named: %v", in.Missing)
	}
}

// TestANegativeVarianceComponentIsNotSquareRooted.
//
// A method-of-moments variance component can come out negative, and that is
// informative rather than an error: it is a component saying it is smaller than the
// noise around it, which REML's 0.000 hides. It must not become a NaN halfway down a
// table.
func TestANegativeVarianceComponentIsNotSquareRooted(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, filepath.Join(dir, "variance_pooled.csv"),
		"metric,arms,v_season,v_start,v_resid,S,G,v_season_used,v_start_used",
		"hold,4,-0.76,0.84,6.36,4,6,0,0.84",
	)
	in, err := ReadInference(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Components) != 1 {
		t.Fatalf("got %d components", len(in.Components))
	}
	c := in.Components[0]
	if c.SDSeason != 0 {
		t.Errorf("a floored-to-zero season component came out as %v", c.SDSeason)
	}
	if c.SharePath <= 99 {
		t.Errorf("with no season component the whole spread is path noise; got %.1f%%",
			c.SharePath)
	}
	in.Source = "S#1"
	md, _ := Render(Inputs{Inference: []Inference{in}})
	if strings.Contains(md, "NaN") {
		t.Error("the snapshot contains NaN")
	}
	// A zero point estimate is not a fact at four seasons, and the snapshot has to
	// say so or a reader treats it as one.
	if !strings.Contains(md, "not proof of zero") {
		t.Error("a zero season component is presented without its caveat")
	}
}

func writeCSV(t *testing.T, path, header string, rows ...string) {
	t.Helper()
	body := header + "\n" + strings.Join(rows, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAnExactInvarianceIsNotInfinitePrecision.
//
// When every paired difference is exactly zero, R writes a standard error of 0 and
// NA for the season F test. Rendered naively that becomes "detects 0 points a
// season" beside "p = 0.000" — the finest possible resolution and the most
// significant possible season component, for a metric that did not move at all.
//
// The state is legitimate and common: it is what an invariance check *passing* looks
// like, and it is exactly what the nobody-doubled rung produces under the
// vice-captain fix, since a metric that doubles nobody is structurally blind to a
// rule about who gets doubled. This is the blank-versus-zero distinction that has
// already cost this project two seasons of replays, arriving somewhere new.
func TestAnExactInvarianceIsNotInfinitePrecision(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, filepath.Join(dir, "mde.csv"),
		"estimator,se,df,t_crit,sig_gw,sig_season,mde_gw,mde_season,metric,is_primary,p_season_pooled,power,alpha,season_gws",
		`"season-clustered (primary)",0,3,3.182,0,0,0,0,"hold_nocap",TRUE,NA,0.8,0.05,38`,
		`"start fixed, no season effect",0.515,15,2.131,1.098,41.7,1.544,58.7,"hold",TRUE,0.80,0.8,0.05,38`,
	)
	writeCSV(t, filepath.Join(dir, "variance_pooled.csv"),
		"metric,arms,v_season,v_start,v_resid,S,G,v_season_used,v_start_used",
		"hold_nocap,1,0,0,0,4,6,0,0",
		"hold,1,0,0.84,6.36,4,6,0,0.84",
	)
	inf, err := ReadInference(dir)
	if err != nil {
		t.Fatal(err)
	}
	row, _ := inf.PrimaryMDE("hold_nocap")
	if !row.Degenerate {
		t.Error("a zero standard error across the grid was not recognised as an exact " +
			"invariance")
	}
	if row.HasPSeason {
		t.Error("R's NA for the season F test was parsed as a real p-value; 0.000 is " +
			"the most significant value there is and this metric did not move")
	}

	inf.Source = "S#1"
	md, v := Render(Inputs{Inference: []Inference{inf}})
	if !strings.Contains(md, "cannot move") {
		t.Error("the degenerate metric is not described as unable to move")
	}
	if strings.Contains(md, "**0 pts/season**") {
		t.Error("a zero resolution is printed as though the metric could detect an " +
			"arbitrarily small effect")
	}
	// The degenerate metric must get no numeric row at all in the components table.
	// Checked on its own line rather than across the whole document, because a
	// variance component of exactly 0.000 is legitimate elsewhere — the shipped grid
	// has one on HOLD — and a document-wide search for "0.000" would fail on that.
	for _, line := range strings.Split(md, "\n") {
		if !strings.Contains(line, "nobody doubled") || !strings.HasPrefix(line, "|") {
			continue
		}
		if !strings.Contains(line, "exact invariance") &&
			!strings.Contains(line, "cannot move") {
			t.Errorf("the degenerate metric got a numeric row: %s", line)
		}
		if strings.Contains(line, "0.000") {
			t.Errorf("an undefined F test or variance is printed as 0.000: %s", line)
		}
	}
	// And it must not become the "finest comparison" the headline quotes, which
	// would report the harness as resolving zero points a season.
	if v.Value["harness.mde_season.finest"] == "0" {
		t.Error("the degenerate row became the finest resolution in the summary")
	}
	if v.Value["harness.s_1.invariant.hold_nocap"] != "true" {
		t.Error("the invariance is not recorded in the machine-readable figures")
	}
}

// TestTheSharedCellQuantitiesHaveOneImplementation.
//
// The paired difference, the CR2 standard errors and the early/middle/late regime
// split are computed by more than one R script now: sweep_inference.R produces the
// per-arm verdicts and schedule_screen.R the per-entry-gameweek columns. They were
// factored into stats/cells_common.R for exactly the reason AGENTS.md calls this
// record's signature failure — one quantity with two implementations, four recorded
// instances, each found only after the two copies had already disagreed in print.
//
// The failure mode this guards is not a rewrite. It is somebody adding a third
// script and pasting `se_cr` into it because sourcing felt like a dependency, or
// re-adding a local `diffs_for` to sweep_inference.R during a merge. Both copies
// agree on the day they are written, which is why a runtime check would not catch
// it and a source scan does.
//
// Scanning source rather than running R is also forced: `go build`, `go vet` and
// `go test` must all pass on a machine with no R installed, so nothing here may
// invoke Rscript.
func TestTheSharedCellQuantitiesHaveOneImplementation(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skip(err)
	}
	const home = "cells_common.R"
	// Every .R under stats/, RECURSIVELY. ⚠️ This globbed `stats/*.R` until
	// 2026-08-14 and therefore never saw `stats/snapshots/*/sensitivity.R` — two
	// more readers of a cells file, one of them reading it raw with its own flag
	// coercion and the narrow `season@start_gw` key. A2 consolidated seven readers
	// and reported them as all of them; there were nine.
	//
	// A snapshot directory is exactly where a copy goes to be forgotten, and those
	// scripts are not archives: they are callers of this shared library, and
	// `min_cells` becoming required broke one of them outright.
	var files []string
	err = filepath.WalkDir(filepath.Join(root, "stats"),
		func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(path, ".R") {
				files = append(files, path)
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no R scripts found under stats/; this guard is scanning the wrong tree")
	}

	// Assembled so the definition in cells_common.R is matched but a *call* is not:
	// R defines with `name <- function(`, and every other occurrence is a use.
	// ⚠️ The READER names are on this list, and they were the omission that
	// mattered. A2's whole thesis is that reading the file is where the family's
	// divergences lived, so `read_cells` is the name most worth pinning — and
	// without it a re-added local
	//
	//	read_cells <- function(p) { d <- read_sidecar(p); d$cell <- paste(...); d }
	//
	// passes the raw-read scan below AND the name scan here, reinstating the exact
	// defect this item removed.
	shared := []string{"diffs_for", "se_cr", "se_cr2", "se_cr2_start",
		"degenerate", "regime_of", "as_flag",
		"read_cells", "read_cells_all", "read_sidecar",
		// Added 2026-08-15 with the wild cluster bootstrap. `cr2_t_fast` is the
		// copy-worthy one: it is a hot inner loop, and the place it would get
		// pasted is `concentration_screen.R`, which already computes its own means
		// over the same cells and is the file most likely to want a t next.
		"cr2_t_fast", "wild_cluster_p", "wild_cluster_p_season", "wcb_label",
		// Added 2026-08-16 with stats/blank_run_position.R. `sig_and_mde` is the
		// `t_crit(df) * SE` / 80%-power pair this record quotes a threshold from
		// in nearly every verdict, and it was two lines inside
		// `variance_components.R`'s `mde_row` — small enough to retype, which is
		// exactly how the eleven medians happened. `mde_row` now forwards to it
		// and supplies the cells family's x38 season scale, which a caller
		// measuring a unitless ratio must not inherit.
		"sig_and_mde"}
	def := func(name string) *regexp.Regexp {
		return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*<-\s*function`)
	}

	// Debt this guard FOUND when it was written, on 2026-08-14, recorded rather
	// than exempted. Three scripts predating cells_common.R carry their own
	// copies, and they have ALREADY DIVERGED — so AGENTS.md's "four instances"
	// of one-quantity-two-implementations is an undercount:
	//
	//   - the minimum cells an arm needs to survive differs by copy: cells_common
	//     and grid_width drop below 2, variance_components below 4. An arm can
	//     therefore appear in one script's table and be absent from another's,
	//     with no error either time.
	//   - grid_width's copy drops such an arm SILENTLY, which is the exact defect
	//     the note above cells_common's diffs_for says it exists to prevent.
	//   - grid_width's `degenerate` takes a vector where cells_common's takes a
	//     data frame: same name, different signature, so sourcing both would be
	//     resolved by file order.
	//   - grid_width hardcodes "_per_gw" rather than taking the scale.
	//
	// `as_flag` is textually identical everywhere and is duplication without
	// divergence, which is why it is listed but is the least urgent.
	//
	// Migrating these changes what they print, so it is queued rather than done
	// here. The list must SHRINK: an entry whose copy has gone is a failure too,
	// so nobody can migrate a script and leave the debt recorded as outstanding.
	// Recorded as "Three more copies of the paired difference", and closed 2026-08-14.
	// ✅ **Empty since 2026-08-14.** The three scripts listed here all sourced
	// cells_common.R as part of A2, along with the four that were never listed —
	// `entry_density.R`, `rank_robustness.R`, `prediction_inference.R` and
	// `schedule_screen.R`'s reader. `variance_components.R` still wraps
	// `diffs_for`, but a wrapper that forwards is not a second implementation: it
	// supplies this estimator's own cell floor of 4, which is `mom`'s precondition
	// and deliberately not the shared 2.
	//
	// The list must stay empty or shrink. It is kept rather than deleted because
	// the stale-entry branch below is what stops a migration being recorded as
	// outstanding after it has landed.
	knownCopies := map[string][]string{}

	var homeSeen bool
	var offenders, stale []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		base := filepath.Base(f)
		if base == home {
			homeSeen = true
			for _, name := range shared {
				if !def(name).MatchString(src) {
					t.Errorf("%s no longer defines %s; the shared home has lost a "+
						"quantity and every caller is now free to grow its own",
						home, name)
				}
			}
			continue
		}
		known := knownCopies[base]
		for _, name := range shared {
			if def(name).MatchString(src) {
				if !contains(known, name) {
					offenders = append(offenders, base+" defines "+name)
				}
				continue
			}
			if contains(known, name) {
				stale = append(stale, base+" no longer defines "+name)
			}
		}
	}
	if !homeSeen {
		t.Fatalf("stats/%s is missing; it is where these quantities live", home)
	}
	if len(offenders) > 0 {
		t.Errorf("a NEW second implementation of a shared cell quantity:\n  %s\n\n"+
			"Source stats/%s instead. One quantity computed two ways is this "+
			"record's signature failure — the copies agree when written and the "+
			"disagreement is found in print, months later.",
			strings.Join(offenders, "\n  "), home)
	}
	// The name scan above cannot see a reader that defines nothing, and the two
	// worst ones did exactly that: `entry_density.R` coerced its flags with an
	// inline `%in% c(TRUE, "true")` and `rank_robustness.R` with `== "true"`,
	// neither of them a definition. So the family's actual entry point — reading
	// the file at all — is scanned separately, and by SHAPE rather than by name,
	// which is the lesson `TestTheMiddleValueHasOneImplementation` records.
	//
	// `read_sidecar` exists in cells_common.R so this ban has an alternative: a
	// blanket refusal with nowhere to go gets routed around rather than obeyed.
	// Every way R reads a delimited file, not just read.csv — the ban reads as
	// broad and was narrow. `scan` and `readLines` are deliberately absent: they
	// are not table readers and refusing them would catch unrelated code.
	rawRead := regexp.MustCompile(
		`(^|[^.\w])((utils|readr|data\.table)::)?` +
			`(read\.csv2?|read\.table|read\.delim2?|read_csv|read_delim|fread)\s*\(`)
	var rawReaders []string
	for _, f := range files {
		base := filepath.Base(f)
		if base == home {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") || !rawRead.MatchString(line) {
				continue
			}
			rawReaders = append(rawReaders, base+":"+strconv.Itoa(i+1)+"  "+trimmed)
		}
	}
	// The reader's own self-test must exist. It is the only check with power over
	// what this consolidation fixed: the banked cells exercise none of the paths the
	// readers differed on — 0 infeasible rows in 7,320, 0 empty character fields,
	// `is_baseline` character in all 72 files — so a before/after on them is a
	// regression check and nothing more. `stats/cells_reader_selftest.R` supplies the
	// multi-block, flag-spelling, empty-character and infeasible cases the bank
	// cannot.
	//
	// Asserted by existence rather than by running it, because `go test` must pass on
	// a machine with no R installed.
	selftest := filepath.Join(root, "stats", "cells_reader_selftest.R")
	if _, err := os.Stat(selftest); err != nil {
		t.Errorf("stats/cells_reader_selftest.R is missing (%v).\n\n"+
			"It is the only test with power over the reader consolidation — the "+
			"banked cells agree under every coercion the readers ever used, so they "+
			"cannot separate them. Deleting it leaves the before/after regression "+
			"check, which answers a different question.", err)
	}

	if len(rawReaders) > 0 {
		t.Errorf("a raw read.csv outside stats/%s:\n  %s\n\n"+
			"Call read_cells (a cells file: coercion, keys and the contract checks) "+
			"or read_sidecar (a .means.csv or .provenance.csv). Reading the file is "+
			"where this family's divergences lived — one script keyed cells without "+
			"the sweep and cross-paired arms against other sweeps' baselines, "+
			"silently, on committed data.",
			home, strings.Join(rawReaders, "\n  "))
	}

	if len(stale) > 0 {
		t.Errorf("recorded duplication has been removed but is still listed as "+
			"outstanding:\n  %s\n\nDelete it from knownCopies above, and from the "+
			"work queue if it is still listed there. A debt list that overstates "+
			"the debt stops being read.",
			strings.Join(stale, "\n  "))
	}
}

// TestBothCellReadersAgreeOnTheScale.
//
// diffs_for takes the metric column's suffix as a REQUIRED argument, with no
// default, because per_gw and per_path are different estimands rather than two
// units for one number — stats/sweep_inference.R's own header says a table headed
// "per gameweek" over totals is the failure that option exists to make impossible.
//
// A default would restore exactly that hazard: a caller that had not decided would
// silently get one, and the resulting table would be indistinguishable from a
// correct one. So the absence of a default is the contract, and it is asserted.
func TestBothCellReadersAgreeOnTheScale(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skip(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "stats", "cells_common.R"))
	if err != nil {
		t.Fatal(err)
	}
	sig := regexp.MustCompile(`diffs_for\s*<-\s*function\s*\(([^)]*)\)`)
	m := sig.FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("cannot find diffs_for's signature in cells_common.R")
	}
	args := strings.Split(m[1], ",")
	var suffix string
	for _, a := range args {
		if strings.Contains(a, "suffix") {
			suffix = strings.TrimSpace(a)
		}
	}
	if suffix == "" {
		t.Fatal("diffs_for no longer takes a suffix; the scale would come from " +
			"somewhere implicit")
	}
	if strings.Contains(suffix, "=") {
		t.Errorf("diffs_for's suffix argument has a default (%q). per_gw and "+
			"per_path are different estimands, so a caller that has not chosen "+
			"must not be handed one silently.", suffix)
	}
}
