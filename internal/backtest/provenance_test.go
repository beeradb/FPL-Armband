package backtest

// Stamping a sweep with what produced it, as a side effect of emitting cells.
//
// There is deliberately no "remember to record the settings" step. This project's
// history is a list of hand-maintained records that rotted silently — the four
// season lists that go stale every summer, an override list that outlived its
// situation and kept applying, a cache version bump that a stale file defeated —
// and a convention saying "also write down what you ran" would rot the same way.
// So the declaration is written by the same function that opens the cells file,
// from values it already holds.

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"armband/internal/config"
	"armband/internal/snapshot"
)

// writeSweepProvenance records one sweep's identity beside its cells.
//
// It never fails the test. A sweep arm is fifteen minutes of replay and a
// provenance file that could not be opened is not a reason to throw that away —
// but it is a reason to say so loudly, because a snapshot built from cells with
// no stamp is precisely the unattributable measurement this exists to prevent.
// t.Errorf rather than t.Logf for that reason: the sweep completes and prints its
// table, and the test still fails so the gap cannot be missed.
func writeSweepProvenance(t *testing.T, sweep string, sink *cellSink,
	cfg config.Config, variants []policyVariant, pairs []seasonPair, starts []int) {
	t.Helper()
	cells := os.Getenv("FPL_CELLS")
	if cells == "" || sink == nil {
		return
	}

	// FingerprintView, not cfg: `chip_plan` moved to the team file and is
	// `json:"-"` on Config, so marshalling the config itself would record the
	// subtree as ABSENT and move constants_digest over an unchanged model. See
	// config.Config.FingerprintView.
	fp, err := snapshot.FingerprintOf(cfg.FingerprintView())
	if err != nil {
		t.Errorf("cells written without a constants fingerprint: %v", err)
		return
	}
	sha, dirty := snapshot.GitState(".")

	// commit and constants_digest both fail as staleness detectors on this
	// project's own banked cells — see Provenance.WatchedDigest's comment for
	// why. WatchedDigest is computed the same way `armband snapshot` computes
	// it for an accuracy snapshot: root the paths at the repo root, digest HEAD.
	root, err := snapshot.RepoRoot(".")
	if err != nil {
		t.Errorf("cells written without a watched digest: %v", err)
		return
	}
	watchedDigest, perPath, err := snapshot.WatchedDigest(root, "HEAD", snapshot.SnapshotWatchedPaths)
	if err != nil {
		t.Errorf("cells written without a watched digest: %v", err)
		return
	}
	paths := make([]string, 0, len(perPath))
	for p := range perPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var watchedPaths []snapshot.Constant
	for _, p := range paths {
		watchedPaths = append(watchedPaths, snapshot.Constant{Path: p, Value: perPath[p]})
	}

	seasons := make([]string, 0, len(pairs))
	for _, p := range pairs {
		// "played<-priors from", because a replay's model is built from the prior
		// season and a bare season name hides half of what produced the numbers.
		seasons = append(seasons, p.Name+"<-"+p.PriorName)
	}
	arms := make([]string, 0, len(variants))
	for _, v := range variants {
		arms = append(arms, v.label)
	}

	// BankUpTo is read off a constructed cell config rather than from
	// sweepBankLimit directly, so a variant that changes the bank cannot make this
	// stamp a lie. sweepConfig pins the modern five-transfer rule for every cell,
	// which is historically wrong for 2022-23 and 2023-24 — a caveat that has
	// governed roughly half of AGENTS.md's evidence while living only in a code
	// comment. Carrying it here means a snapshot cannot omit it.
	bank := sweepConfig(cfg, starts[0], false).BankUpTo

	err = snapshot.WriteProvenance(snapshot.ProvenancePath(cells), snapshot.Provenance{
		Sweep: sweep, RunID: sink.run(),
		Commit: sha, Dirty: dirty, Digest: fp.Digest,
		Seasons: seasons, StartGWs: starts, BankUpTo: bank,
		DeclaredArms: arms, Constants: fp.Constants, Env: fp.Env,
		WatchedDigest: watchedDigest, WatchedPaths: watchedPaths,
	})
	if err != nil {
		t.Errorf("cells written without provenance: %v", err)
	}
}

// writeDiagProvenance is writeSweepProvenance's counterpart for a diagnostic
// that is not a policy sweep but still banks its own CSV for a reader to
// difference against a later run.
//
// # Why this exists, out of 129 diagnostics that print to stdout and nothing else
//
// requireDiag already stamps commit, dirty and the fingerprinted env on every
// DIAG-gated diagnostic's stdout — TestEveryDiagGateGoesThroughRequireDiag
// enforces that structurally, so it needs no further work here. What it does
// NOT do is put that stamp anywhere check_shared_code_state
// (stats/cells_common.R) can read it, and check_shared_code_state is what
// actually catches two runs at different commits being differenced — a stamp a
// human has to eyeball is the exact mechanism that missed the FPL_XGC_EXTERNAL_DIR
// mismatch diagstamp_test.go's own doc comment records.
//
// That gap is real only where a diagnostic ALSO writes a standalone CSV
// alongside its stdout table — a file that can be carried off, read by R
// against a LATER run's file, and quoted, exactly as sweep cells are. Scanned
// by hand
// (2026-09) against every diagnostic outside runPolicySweep: 7 test functions
// do this, each gated on an operator-chosen env var that names the output path
// (FPL_CELLS in bandcalibration_diag_test.go, FPL_TAPER_CSV, FPL_RANKS_CSV and
// FPL_LEVELS_CSV, FPL_INSEASON_CSV, FPL_XGCDUMP, FPL_XGC_TERCILE_CSV,
// FPL_CELLS_DIR) — all now call this. 7 more (haulchannel_diag_test.go,
// variancefrontier_diag_test.go, trajectory_diag_test.go,
// haultiebreak_diag_test.go, breakoutdiscrimination_diag_test.go,
// enablervalue_diag_test.go, tiebreaksweep_diag_test.go) write a CSV to a
// HARDCODED path — a dated /work/drop/*-2026-08-30 directory or a testdata
// fixture — never an env var, which reads as a one-off investigation dump
// already tied to a specific past run rather than a diagnostic meant to be
// re-run and compared going forward. They are named here rather than silently
// dropped, and are NOT wired to this call: whether each is closed history or
// still wants provenance is an open question this pass did not settle.
//
// # Why no source scan enforces this, unlike requireDiag's
//
// TestEveryDiagGateGoesThroughRequireDiag works because there is exactly one
// way to gate on DIAG: read the env var by name. There is no one way to "write
// a CSV a reader might quote" — the 7 wired here alone split between
// encoding/csv and hand-joined fmt.Fprintf rows, and one of them keeps its
// os.Create in a helper function a text scan over the *_test.go body would
// have to follow across a call boundary to find. A scan built to catch that
// reliably would need to reason about control flow, which is exactly the false
// confidence this project's own scans are documented as NOT providing ("a scan
// passing is not 'there are no copies'"). So this list is hand-maintained, and
// it is exactly the kind of list this project's own history says rots — noted
// here rather than pretended away.
func writeDiagProvenance(t *testing.T, path string, cfg config.Config) {
	t.Helper()
	if path == "" {
		return
	}

	fp, err := snapshot.FingerprintOf(cfg.FingerprintView())
	if err != nil {
		t.Errorf("diagnostic CSV written without a constants fingerprint: %v", err)
		return
	}
	sha, dirty := snapshot.GitState(".")

	root, err := snapshot.RepoRoot(".")
	if err != nil {
		t.Errorf("diagnostic CSV written without a watched digest: %v", err)
		return
	}
	watchedDigest, perPath, err := snapshot.WatchedDigest(root, "HEAD", snapshot.SnapshotWatchedPaths)
	if err != nil {
		t.Errorf("diagnostic CSV written without a watched digest: %v", err)
		return
	}
	paths := make([]string, 0, len(perPath))
	for p := range perPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var watchedPaths []snapshot.Constant
	for _, p := range paths {
		watchedPaths = append(watchedPaths, snapshot.Constant{Path: p, Value: perPath[p]})
	}

	err = snapshot.WriteProvenance(snapshot.ProvenancePath(path), snapshot.Provenance{
		Sweep: t.Name(), RunID: runIDForProcess(),
		Commit: sha, Dirty: dirty, Digest: fp.Digest,
		Constants: fp.Constants, Env: fp.Env,
		WatchedDigest: watchedDigest, WatchedPaths: watchedPaths,
	})
	if err != nil {
		t.Errorf("diagnostic CSV written without provenance: %v", err)
	}
}

// TestWriteSweepProvenanceRecordsTheWatchedDigest pins that a live sweep's
// sidecar carries the same watched digest WatchedDigest computes directly for
// HEAD — the value stats/mde_aggregate.py's staleness check reads.
//
// Runs against this checkout rather than a synthetic fixture, and skips if it
// is not a git checkout, matching TestEverySnapshotCandidateCarriesAKey and
// TestAKeyDescribesTheCommitItNames in internal/snapshot/watched_test.go.
func TestWriteSweepProvenanceRecordsTheWatchedDigest(t *testing.T) {
	if _, err := snapshot.RepoRoot("."); err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	dir := t.TempDir()
	cells := filepath.Join(dir, "cells.csv")
	t.Setenv("FPL_CELLS", cells)

	sink, err := openCellSink(cells)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.close()

	cfg := config.Config{}
	pairs := []seasonPair{{PriorName: "2023-24", Name: "2024-25"}}
	starts := []int{1}
	variants := []policyVariant{{label: "shipped"}}

	writeSweepProvenance(t, "WATCHEDTEST#1", sink, cfg, variants, pairs, starts)
	if t.Failed() {
		t.Fatal("writeSweepProvenance reported a failure before this test could check anything")
	}

	prov, err := snapshot.ReadProvenance(snapshot.ProvenancePath(cells))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := prov["WATCHEDTEST#1\x00"+sink.run()]
	if !ok {
		t.Fatal("no provenance record for the sweep just written")
	}
	if p.WatchedDigest == "" {
		t.Fatal("WatchedDigest was not recorded")
	}

	root, err := snapshot.RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := snapshot.WatchedDigest(root, "HEAD", snapshot.SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if p.WatchedDigest != want {
		t.Errorf("recorded watched digest %s does not match WatchedDigest(HEAD) %s",
			p.WatchedDigest, want)
	}
}

// TestWriteDiagProvenanceRecordsAStandaloneCSVsSidecar pins writeDiagProvenance's
// two observable promises: a no-op on an unset path (a diagnostic run with its
// CSV output off must not create a sidecar nobody asked for), and a sidecar
// carrying the same commit, dirty flag and watched digest a sweep's does, keyed
// by this test's own name so a reader who calls it twice does not silently
// overwrite the first sidecar record.
func TestWriteDiagProvenanceRecordsAStandaloneCSVsSidecar(t *testing.T) {
	if _, err := snapshot.RepoRoot("."); err != nil {
		t.Skipf("not a git checkout: %v", err)
	}

	dir := t.TempDir()

	// Unset path: no-op, no sidecar, no CSV.
	writeDiagProvenance(t, "", config.Config{})
	if t.Failed() {
		t.Fatal("writeDiagProvenance failed on an unset path, which must be a silent no-op")
	}
	if entries, err := os.ReadDir(dir); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("an unset path created files: %v", entries)
	}

	csvPath := filepath.Join(dir, "diag.csv")
	cfg := config.Config{}
	writeDiagProvenance(t, csvPath, cfg)
	if t.Failed() {
		t.Fatal("writeDiagProvenance reported a failure before this test could check anything")
	}

	prov, err := snapshot.ReadProvenance(snapshot.ProvenancePath(csvPath))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := prov[t.Name()+"\x00"+runIDForProcess()]
	if !ok {
		t.Fatal("no provenance record for the diagnostic run just written")
	}
	if p.WatchedDigest == "" {
		t.Fatal("WatchedDigest was not recorded")
	}

	root, err := snapshot.RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	sha, dirty := snapshot.GitState(".")
	if p.Commit != sha {
		t.Errorf("recorded commit %s does not match GitState %s", p.Commit, sha)
	}
	if p.Dirty != dirty {
		t.Errorf("recorded dirty %v does not match GitState %v", p.Dirty, dirty)
	}
	want, _, err := snapshot.WatchedDigest(root, "HEAD", snapshot.SnapshotWatchedPaths)
	if err != nil {
		t.Fatal(err)
	}
	if p.WatchedDigest != want {
		t.Errorf("recorded watched digest %s does not match WatchedDigest(HEAD) %s",
			p.WatchedDigest, want)
	}
}
