# Review: transfer-hit tuning — the measurement, its correction, and the verdict

**What was reviewed**: the transfer-hit tuning measurement, branch `transfer-hit-tuning`,
commits `a07f3f4..4ad659c` (`origin/development..HEAD`): the pre-registration in
`TestDiagHitTuning`, the verdict sidecar, `stats/hittune_verdicts.R`, the finding
`stats/findings/2026-08-18-transfer-hit-tuning.md`, the banked snapshot, the AGENTS.md
verdict line, and the accuracy snapshot regeneration.

**Reviewers that ran**:

- **fpl-stats-review — the plan review** (before the first cell): found the original design
  (a per-gameweek gain bar on the hit branch) **cannot bind** — the hit branch's implied bar
  is already (MinGainHit + 4)/H = 7/H = 1.4 pts/gw at the shipped horizon, so every rung sat
  below the existing bar. **Applied**: the design was replaced with the MinGainHit ladder
  before the pre-registration was committed.
- **fpl-stats-review — the output review** (first bank): five findings, all applied:
  1. *Mixed population* — the sidecar computed the `hit` flag and discarded it, so the
     headline "641 hit packages, 34.3%" folded free transfers (gated at `min_gain` 0.4) in
     with hits (gated at `MinGainHit` 3.0). **Applied**: flag written (`a0877c2`), the
     216-cell sweep re-run at that commit, all rates filtered to hit packages, the bank
     replaced by the single clean run. The paired cells statistics were unchanged (verified:
     no-hits −10.0, SE 16.0, t −0.63, p 0.558 identical across banks; cells differ only in
     run_id).
  2. *"~half negative"* — wrong count for the loss-rate deltas. **Applied**: recomputed
     (later superseded by the adjusted deltas below).
  3. *Banked cells had 5 duplicate partial-run rows* (two run_ids). **Applied**: re-banked
     from the single clean run_id; means and provenance filtered to the same run.
  4. *Veto paragraph quoted the grid-wide 26-39* — **applied**: now quotes this comparison's
     own thresholds 13.5-18.8.
  5. *Holm family 5 in the inference vs 3 registered* — **applied**: noted in the finding;
     no p moves under either.
- **fpl-stats-review — the re-verification** (corrected bank): four findings, all applied:
  1. *workedOut matched same-gameweek purchases* — 32 of 66 matches were lag-0, where the
     in-side is identical by construction and the comparison degenerates to who was sold.
     The pre-registration says "bought **later**, free". **Applied**: the R match window is
     now gw+1..gw+4 with the earliest later purchase (deterministic — the old first-match
     pick rode Go map iteration order); workedOut reads 24 of 47 (51%), mean +5.7. The
     finding's "clear majority" claim was removed — it is a coin flip at this n.
  2. *The deltas used the raw rate; rule (a) registers the availability-adjusted rate* —
     **applied**: deltas recomputed adjusted: +0.003/−0.024/+0.032 (t 0.07/−0.41/0.42,
     25/24/21 cells).
  3. *Same-week `n_moves > 2` rows merge a funded pair with free singles* (5 of 98; 2 below
     the bar) — **applied**: fidelity ⚠️ in the finding (24.5% → 23.7% if excluded).
  4. *Small accuracy items* — sidecar filename, the measured hits-per-cell range (2.72 mean,
     max 8), the doc comment's column enumeration (in_ids) — all applied.
- **fpl-findings-audit** (two passes): six findings, all applied: the "clear majority"
  overstatement (rewritten with the binomial caveat, then superseded by the window fix);
  the correction narrative absent from the repo record (⚠️ added to the finding, full
  narrative here); the threshold source question (handed to fpl-stats-review — see below);
  the wrong sidecar filename; shipping-rule clause (d) unevidenced (verified from the cells:
  hit reductions 1.08/1.36/1.69 per cell against `moves − hits` rises +0.08/+0.03/+0.11,
  and added); the paired research-store note's frontmatter commit pin (set to the merge
  commit). Second pass: the adjusted n misattached in the AGENTS.md line and the paired
  note's stale State deltas — both fixed.

**Skipped with the triage reason**: fpl-code-review — the triage table assigns
`internal/backtest` and `stats/*.R` to fpl-stats-review + fpl-findings-audit (both ran);
the only Go change is the sidecar `hit` write inside a diagnostic test, touching no scoring,
config, or agent path. fpl-security-review — no authenticated surface or credential path
touched.

**The suite's own guards, after the reviews** — two fired and were fixed:
`TestTheSharedCellQuantitiesHaveOneImplementation` caught a raw `read.csv` in
`stats/hittune_verdicts.R`; the script now reads through `read_sidecar` (the sanctioned
home), output verified bit-identical. `TestEnvSwitchListIsComplete` caught `FPL_HITS_CSV`
unregistered; it names an output path, so it joined the skip map beside `FPL_CELLS` with
the same comment discipline. Both guards fire on idiom, not on harm — the fixes are
registration, not re-measurement, and no number moved.

**What was declined**: nothing. Every finding above was applied or resolved.

**What could not be checked on this harness**: the wildcard-week-after split — vacuous by
design (the measured corner plays no wildcard; zero rows in the sidecar). The workedOut
pairs are clustered by cell and n 47 — quoted as descriptive with no SE. The finding's
threshold column (16.3/13.5/18.8) is the run-time inference print; recomputing
`t_crit × CR2 SE × 38` from the rounded printed SEs reproduces it to ±0.1-0.2 (16.2/13.4/
18.6), so the values differ only by print rounding. The sidecar carries no `run_id` column,
so a future mixed sidecar could not be detected from the file itself — noted, no fix
(provenance pins the cells run).

**The verdict recorded**: MinGainHit 3.0 stands; nothing ships. 24.5% of hit packages below
the gate's own bar (29.7% adjusted) against a ~50% truncation null; ladder resolves nothing;
workedOut 51% of later-matched hits (descriptive); no-hits −10 a season (unresolved).
