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

	fp, err := snapshot.FingerprintOf(cfg)
	if err != nil {
		t.Errorf("cells written without a constants fingerprint: %v", err)
		return
	}
	sha, dirty := snapshot.GitState(".")

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
	})
	if err != nil {
		t.Errorf("cells written without provenance: %v", err)
	}
}
