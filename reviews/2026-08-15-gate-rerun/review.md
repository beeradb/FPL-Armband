# The gate oracle re-run on the scaled xPoints instrument

**What was reviewed.** The working tree of `xpoints-scaled-gate-rerun` against `main` `82fc8e0` —
a re-run of `TestDiagGateOracleOnXPoints` on the per-(season, position) conversion scale that
merged at `82fc8e0`, plus the record updates the re-run owed. Five files edited, three added
(`stats/gate_additivity.R`, `internal/backtest/conversionscale_diag_test.go`,
`stats/snapshots/2026-08-15-gatescaled/`).

**No production code changed.** Verified twice, and once independently: every edit to
`internal/analysis/xpoints.go` and `internal/backtest/gatexpoints_diag_test.go` is comment-only.
`fpl-code-review` confirmed it by parsing both revisions with `go/parser`, dropping comment nodes
and diffing the printed ASTs — identical. The only executable changes are the budget constant in
`internal/snapshot/notes_test.go`, a new DIAG-gated `_test.go` diagnostic, and two files under
`stats/`.

## Reviewers

| reviewer | when | why |
|---|---|---|
| **fpl-stats-review** | on the **PLAN**, before the run | standing rule: review the plan, not just the output, for anything that will produce a number |
| **fpl-stats-review** | on the **OUTPUT** | `internal/backtest` + `stats/*.R` |
| **fpl-code-review** | on the diff | `internal/analysis` touched (comments), two new scripts, one new diagnostic |
| **fpl-findings-audit** | on the diff | `CLAUDE.md` and the record edits |
| **the private record/placement auditor** | on the research-record writes | placement arbitration, raised by the user mid-session |

Skipped: **fpl-security-review** (no `internal/agent`, `internal/fpl` or config-persistence
change), **fpl-run-review** (no live run wrote config), **fpl-season-maintenance** (the four
hand-maintained lists are untouched).

**The plan review earned its place and this is the evidence.** Two of its findings changed what was
measured and how it was read, and neither would have been visible in a diff of the finished work.

## Findings, ranked by how misleading the current state was

### 1. The headline statistic could not discriminate — verdict downgraded

The first write-up led with `RESIDUAL − UNDERLYING` on `policy_xpoints`: −2.770 pts/gw, CR2
t −11.38, wild p 0.0033, positive in 1 of 36. Decisive by every column, and worthless as evidence.
The contrast is exactly `level_R − level_X`; about 70% of its magnitude is the X leg, which is the
underlying arm's **positive control** on that very metric; and the diagnostic's own pre-registration
(`gatexpoints_diag_test.go:51-55`) declares its null false in advance — "expected to be positive and
materially smaller… never against zero". A large t against a null nobody held is not a finding.

**Applied.** Verdict downgraded from `established` to `suggestive`. What carries the result is now
stated as the residual arm's own level (−0.828 pts/gw, CR2 t −2.04, p 0.0971, **does not resolve**),
informative through its **sign** because the pre-registered bonus confound expected it positive.

⚠️ **The instructive part**: the same write-up had already withdrawn "the only arm that improves both
metrics" as half-mechanical — the identical defect one level down — and then re-committed it at the
contrast. The plan review caught it at the level; only the output review caught the migration.
**Both reviews were necessary and neither was sufficient.**

### 2. The confinement check confirmed nothing, and the check with power was missing

Framing the points arm's byte-identity as proof the change was confined is invalid: confinement is
a *code* fact (`acceptTransfer` → `perfectGate` → `pointsOver`, and nothing in `weekScoreWithChip`
branches on an xPoints quantity), so the check can only fail, and its candidate causes — data state,
stale cache, intervening commits, a dirty banked tree — never include the change under test. The
banked reference is itself `a359de4, dirty true` against a diff that *adds* `perfectGateResidual`,
so a failure would not have been attributable anyway.

**Applied.** Reframed as a **provenance** check, and the liveness check it lacked was added and run:
`hold_xpoints` **must move** (144/144), `hold_points` and `squad_hash` must not (0/144), the
baseline's `policy_points` must not (0/36).

### 3. "0 of 144" was wrong, and the true number is the better result

The record said `hold_points`, `squad_hash` and the baseline's `policy_points` all moved in "0 of
144". The baseline has only **36** cells, and across all arms `policy_points` moved in **47 of 144**
— 0/36 baseline, 0/36 points arm, **20/36 underlying, 27/36 residual**. Verified independently.

**Applied.** Denominators corrected, and the 20/27 promoted: it shows the changed criterion moved
actual gate **decisions**, which is arrival on the scored path rather than merely in a column — a
stronger liveness statement than `hold_xpoints`, which is a HOLD-path quantity.

### 4. Three demonstrated defects in the two `stats/` scripts

All three were shown with working reproductions, not asserted.

- **`gate_recovered_fraction.py` keyed cells without `run_id`/`sweep`.** Safe only by accident while
  the path was hardcoded; making it an argument — the repair in this same diff — turned it live.
  Concatenating the two banks printed the **superseded 0.6450 and [0.426, 0.835]** with no warning.
  **Applied**: a one-block contract check that refuses and names both blocks.
- **`tc = 2.571` hardcoded beside a computed, unused `n`.** On a four-season subset the script
  printed "H0 theta=0.89: **REJECT** at df 5" where df is 3. **Applied**: `t_crit` derived from the
  seasons present, from a table, failing loudly outside it. The same subset now reads "cannot reject
  at df 3".
- **`gate_additivity.R`'s arm guard checked presence, not identity**, under a comment claiming it
  would fail loudly on a re-ordered sweep. Swapping `variant_index` 1↔2 produced **+133.3 a season,
  t 7.10, p 0.0009** — pure mislabelling, no complaint. `μ_X + μ_R − μ_P` is asymmetric in P.
  **Applied**: arms resolved by their `oracle` criterion string. Swapped indices now yield the
  correct figure rather than a refusal.

### 5. An estimator swap written up as a data change — third instance in this record

The write-up reported non-additivity as "now unresolved on both estimators, where +43.6 rejected on
start-fixed". That transition never happened. `gate_additivity.R` prints `se_cr2_start` — CR2
clustered on the **entry point**, which `cells_common.R:327` calls "a robustness check rather than a
rival estimate" — and it was labelled "start-fixed", the record's name for `se_fixed` in
`sweep_inference.R:499`, a fixed-effects estimator on different df.

**Applied.** The script now prints it as `CR2 entrypt` and states outright that it is not the
record's start-fixed. The claimed transition is **withdrawn**: on the primary season-clustered
estimator the contrast was unresolved in both runs (t 1.92 → 1.77).

**Declined:** moving `se_fixed`/`season_share` into `cells_common.R` so the script could compute the
licensed estimator. `sweep_inference.R:530` keeps `season_share` local deliberately so it runs
standalone, and a third copy is forbidden outright. **Filed as an open item instead**, with the
consequence recorded: no like-for-like fixed-effects comparison across the two runs exists, and none
may be quoted until one is computed.

### 6. Superseded figures still presented as current

Found by audit, all applied: two stale markers *above* the new result block (including "NO RE-RUN
HAS BEEN DONE" and "no verdict may be quoted off it today"); the bullet headline still reading
"re-run owed"; `+85.3`, the threshold of 46 with `SE 0.471`, and the **Holm triple
0.0047/0.0101/0.0109** — which was missing from the supersession list entirely and is now
0.0047/0.0130/0.0113; and a live `fmt.Printf` emitting a superseded xppilot SE figure on every run,
one file away from the script being repaired for exactly that.

⚠️ **`54.8` was NOT superseded** and listing it as such was my error: the points arm's `SE_CR2` is
0.560 in both runs, so its threshold is 54.71 in both. 54.8 is a mis-rounding. Listing it beside two
that did move invites the conclusion that the points arm moved — contradicting the provenance check
three lines above.

⚠️ **"Discharged" was overstated.** The supersession list also covers xppilot's SE figures, which
belong to a different sweep and were not re-measured. Corrected to "discharged for the gate arms",
in `CLAUDE.md` and in the budget comment that was argued on the discharge.

### 7. The budget raise did not bind

Raised 80 KB → 88 KB, leaving 6,920 bytes of slack where every prior step left about 1 KB. Both
reviewers flagged it independently, and the objection is not tidiness: the watch written into that
very comment could not fire until 6.9 KB of unremarked growth had landed — the `min_gain` pattern,
a threshold set where it cannot act. **Applied**: 84 KB, ~1.4 KB of slack.

⚠️ It went to 82 in between, and 82 was wrong for a reason worth recording: the reviewers sized the
raise against the pre-review draft, and acting on their strongest finding *grew* the file. **A
budget sized against a pre-review draft is sized against the wrong file.**

### 8. The banked scale did not name its own data state

`conversion_scales.txt` exists solely to satisfy "name the data state or do not quote a recorded
level", and printed no data-state marker — indistinguishable from one produced under
`FPL_NO_XG_REPAIR=1`. **Applied**: the diagnostic prints every `FPL_`-prefixed variable actually
set. Deliberately not a list of the switches that matter — a list here would be a second copy of the
fingerprint enumeration and would go stale the day a switch is added; "what is set" cannot.

### 9. The record-placement audit found a self-contradiction I had introduced

Trimming the banked `FINDINGS.md` to "provenance only" was the **wrong correction**, applied on the
wrong axis. All twelve tracked precedents are narrative verdict documents, and a pre-registration
must never be separated from its own findings — so this run's own verdict belongs there. The result
was a file declaring "this is NOT the research record" seventeen lines above a section carrying the
verdict, with a near-verbatim copy of the same paragraphs in the other store.

**Applied.** The axis is now **this-run versus across-runs**: this run's numbers, provenance, checks
*and verdict* here; what it means for everything else, elsewhere. The self-contradicting paragraph
is corrected in place and says what it got wrong.

⚠️ **A leak of my own, caught by the same audit and fixed before pushing**: the reviewer table in
this record named a private agent by its registry name. The exemption for such disclosures is bounded
to what was already banked, so a new one is not covered. Renamed to its function. **The forward rule
is that the reviewer table names functions, not registry entries.**

⚠️ **Two pointers I added resolved to nothing** — `→ inference-and-instruments` and a second
`→ xppilot`. Nine notes exist and neither is among them. Both repointed to `transfer-policy`, which
exists and is where the 0.89 bar lives. **A third, pre-existing `→ xppilot` at `CLAUDE.md:570` is
also dangling and was left alone** — it is not this change's, and it wants its own one-line fix.

### 10. The follow-up this run opened was refuted before it was built

The re-run left one gap: whether the residual arm's negative `policy_xpoints` is partly selection
arithmetic. The obvious check — a cross-sectional `corr(X, R)` over candidates, off the archive with
no sweep — was reviewed as a plan and **refuted on mechanism**, verified against the code rather
than accepted:

- **The gate is a VETO on one candidate, not a selector.** `bestSwap` returns `swaps[0]`
  (`simulate.go:2200-2216`) and `decide`'s `default:` branch **returns** on rejection rather than
  trying the next (`:1591`). There is no argmax over the residual, no candidate population, no tail
   — so any statistic over candidates describes a different operator.
- **`corr(X, R) = 0` is a CALIBRATION null.** `Points = X + R` identically, so
  `cov(X,R) = cov(X,P) − var(X)` and `corr(X,R) < 0 ⟺ β(P|X) < 1`. It measures the instrument's
  attenuation slope, not football — and X carries an in-sample conversion scale. **The same defect
  as the contrast it was meant to check**: a null nobody holds, arithmetically tied to the scored
  quantity. That is three invalid inferences in one session, all of the same family, and all three
  were caught by reviewing a *plan* rather than an output.

**Closed at no run cost by the same review**, off banked cells and verified independently: the
hit-charge rival is dead. The residual arm makes 2.67 fewer moves and 0.39 fewer hits a cell, so the
hit channel is **+0.06 pts/gw and helps**, against an observed −0.828. Effectively all of it is
squad composition.

**What replaces it**: an **anti-residual** arm (accept iff `−ΔR − 4h > 0`), which is antisymmetric
and needs no calibration, plus a **conjunction** arm that does not bypass the value bar. Queued with
pre-registered readings, including the one that would drop the recorded verdict to *indistinguishable
from noise*.

⚠️ **And an unnamed alternative that may be larger than the one being chased**: `acceptTransfer`
replaces `shippedAccept` entirely (`gate.go:148-159`), so `min_gain` and `free_transfer_value` do
not apply inside an oracle arm. Substituting an uninformative criterion for the value bar could
produce the negative with no X/R correlation at all. The footprint fits — the two informative arms
make 24-29% *more* moves than shipped while the residual arm makes 10.8% *fewer*.

## What was declined

- **Moving `se_fixed` into `cells_common.R`** — see finding 5. Filed as an open item.
- **Collapsing the superseded figure list in `CLAUDE.md`** to a pointer, which would have made this
  a near-zero-byte raise. Declined because it means editing a marked-in-place correction written by
  earlier work; the growth was absorbed by tightening this session's own prose instead.
- **A per-package decision log** to deliver the disagreement rate the diagnostic has owed since
  2026-08-14. Real, but it is a new measurement rather than this re-run, and it wants its own change.
- **Adding a leave-one-season-out scale arm** to this run — it would take the Holm family from 3 to
  4 while an unresolved 2-versus-3 dispute is already recorded. It belongs in its own block.
- **Re-deriving `xpoints_variance.py`, `xpoints_permove.py` and `xpoints_guard.py`.** Owed, and out
  of scope for the re-run; the queue item stays open.
- **Shrinking the duplicated section in the research record, and ticking the closed queue item's
  checkbox.** Both are correct and both were declined for the same mechanical reason: that store's
  write surface offers append, prepend and replace-whole-note only, and re-emitting a 40-54 KB file
  that concurrent sessions have open, from a paged read, risks a truncation that is not recoverable
  the way a stale paragraph is. Both are **recorded in place** with an instruction for whoever next
  opens those files for a targeted edit. ⚠️ **This is a limitation of the interface, not a judgement
  that the duplication is acceptable** — and it is the one finding from that audit I could not act
  on.
- **Moving the budget-raise retrospective out of `notes_test.go`.** Proposed on the ground that a
  retrospective about a review process is not repo material. Declined: the sizing argument and its
  history are what stop the next raise repeating the error, and they are read at the point the
  constant is edited. The general lesson — *a quantity sized against a pre-review draft is sized
  against the wrong artefact* — was filed in the research record instead, so the durable half is not
  repo-only.

## What could not be checked on this harness

- **Whether the residual gate's negative underlying gain is real or selection arithmetic.** If
  underlying and conversion residual correlate negatively across the candidate population over the
  decision window, the sign is partly mechanical. Measurable off the archive with no sweep, and
  **unrun** — which is why the verdict is `suggestive` and not `established`.
- **Whether the 50% recovery bar could ever resolve here.** It straddled twice and the interval got
  wider; six season clusters cannot deliver the ~45% SE cut it would need. The bar is retired as a
  decision rule rather than re-run — *unresolvable on this instrument*, not answered.
- **Whether the conversion scale improves replayed points.** It is instrumentation and cannot move
  them; the question is not posed by this run.
- **Any fixed-effects comparison across the two runs** — see finding 5.

## Note on the record's two stores

The banked `FINDINGS.md` under `stats/snapshots/2026-08-15-gatescaled/` is **run provenance**: what
ran, at what data state, what came out, and the mechanical checks. It is in the repository because
figures quoted in repo files must be checkable on this host. **The verdict and the reasoning are not
there** and are not reproduced in this record either — a conclusion in two stores is the drift
failure this project has already paid for once.
