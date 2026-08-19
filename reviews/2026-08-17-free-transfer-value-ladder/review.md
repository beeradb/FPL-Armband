# Review: the flat free_transfer_value ladder

## What was reviewed

The first banked variation of `free_transfer_value`: the five-rung ladder (1.0/1.5/2.0/3.0/4.0)
run twice on the six-season grid (36 cells per arm, `POLICY`), its banked snapshot at
`stats/snapshots/2026-08-17-free-transfer-value-ladder/`, the finding at
`stats/findings/2026-08-17-free-transfer-value-ladder.md`, the AGENTS.md replacement of the
withdrawn "never been varied" claim, the kink clause added to the `min_gain` bullet, and the four
new guard tests in `internal/backtest/freetransfer_test.go`. Commit range
`origin/development..1dc5d1d/c4de5ce` plus the staged write-up that was reviewed before commit.

## Reviewers

- **fpl-stats-review** — ran. Reproduced every figure in the finding from the banked cells:
  point estimates, season means, per-arm thresholds on both estimators, Holm on the clustered p,
  wild-bootstrap floors at S_eff 6, the kink arithmetic against `simulate.go`, the GW35 boundary,
  the census residuals, confinement/liveness cell by cell, concentration and LOSO. **No figure
  wrong.**
- **fpl-code-review** — ran. Verified the four new tests do what the finding claims: the kink
  test refuses to run under the three switches that falsify the identity, carries a positive
  control, and pins GW35; the source-scan test hard-codes `Alternative` zero and `Strict` false.
  No one-quantity-two-implementations violations; no new config fields, so no backfill owed.
  **Clean.**
- **fpl-docs-review** — ran. One finding, applied (below).
- **fpl-findings-audit** — ran. Two findings, both applied (below). Record coherence verified
  otherwise: the six-ties arithmetic, kink-identity consistency across bullets, hypothesis
  labelling, per-gw×38 consistency, nulls read as ties.
- Skipped: **fpl-security-review** (no surface it owns was touched — no client, agent, fpl-API or
  config-persistence change). No replay was running during the reviews.

## Findings and dispositions

1. **Docs — applied.** The test file's title comment still said "the level this constant has
   never been swept at"; now reads "measured and unresolved".
2. **Audit — applied.** The transfer-gate closure's count moved from "one invariance and two
   ties" to "one invariance and six ties", orphaning the trailing "not four supports" denial,
   which dates to v1 and has no live referent. Removed, per the file's convention that the
   current truth replaces the narrated correction.
3. **Audit — applied.** The xPoints instrument's reading at the 1.0 rung is −0.001 pts/gw; the
   "+0.0" quoted in AGENTS.md and the finding carried a wrong sign. Now "≈0.0" in both.

Nothing was declined.

## What could not be checked on this harness

Cross-architecture reproduction: the run is arm64, CI is amd64, and the banked `xpoints` columns
are full float64, so they are not expected to reproduce there bit-for-bit. The points columns are
integers and `squad_hash` a digest, so a flip would show as a decision change, not a rounding.
The same-machine reproduction (two runs, byte-identical on all 180 cells) is recorded in the
finding.
