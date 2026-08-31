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
