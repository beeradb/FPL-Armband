# Review: the option-decay 2×2 (taper under the exit levers)

**What was reviewed:** the pre-registered measurement of `TaperFreeTransferValue` under the
exit levers — `TestDiagOptionDecayUnderExitLevers`, `stats/taperx_contrasts.R`, the finding
`stats/findings/2026-08-18-option-decay-2x2.md`, and the banked cells at
`stats/snapshots/2026-08-18-option-decay-2x2/cells/`. Commit range `48ebc71..0daabed` on
branch `re-judge-prior-reactivity-and-option-decay` (tracks `origin/development`).

**Reviewers:**

- **fpl-stats-review — ran twice.** First on the PLAN (before any cell ran): widened the
  design from a single ON-corner contrast to the mechanism-named 2×2, supplied the canary
  argument (the flat level ladder as the envelope), and demanded the column roles, liveness
  floors, expected charge schedule, and registered limitations that went into the
  pre-registration. Then on the OUTPUT (post-run): recomputed every figure against the banked
  cells and reported the five findings below.
- **fpl-findings-audit — skipped, by triage.** The change does not edit `AGENTS.md`/`CLAUDE.md`;
  per the user's standing direction, results live in the research record's notes and the
  finding, not in the resident record. Nothing new in its scope.
- **fpl-code-review — skipped, by triage.** The diff is a diagnostic test, an R inference
  script, a finding and banked cells; the taper wiring it scores was reviewed when the
  option-value block was built. No scoring path is changed (the lever ships off; the shipped
  tree is byte-identical — the flat-off baseline reproduces the ladder's banked 2.0 rung
  36/36, and the shared 12 ON/OFF cells reproduce the prior 2×2 byte-identically).

**Findings, ranked by how misleading the current state was:**

1. "~15× below its threshold" was an arithmetic slip — 2.7 against a threshold of 17.9 is 15%
   of the threshold, ≈6.6× below it. Present in both the finding and the post-run correction
   in the doc comment.
2. The byproduct compared the k8-only levers figure (+97.4) against the prior 2×2's k-mean B
   (+73.0) — like against unlike; the same-configuration prior figure is the k8-only +63.3.
   The finding also omitted the strongest corroboration available: the shared 12 cells
   reproduce the prior run byte-identically.
3. "Six season values across a 340-point spread" did not reproduce from either banked cell
   set.
4. Start-fixed thresholds and the wild-bootstrap floor were not quoted beside the clustered
   ones, per the standing rule.
5. "Sharpest POLICY thresholds this record has produced" was loose in the plural — true of the
   ON contrast (10.7), not of OFF/AxB (16.7/17.9).

**What was applied:** all five, verified against the banked cells before editing.

1. "~15×" → "about 15% of its threshold (≈6.6× below it)" in the finding and the doc comment.
2. Byproduct rewritten: the byte-identical 12-cell reproduction is stated, the comparison is
   made against the prior k8-only +63.3, and the entry-point schedule (GW1 +38.2 … GW26
   +180.3) is shown.
3. The unsourced spread figure is replaced with the record's actual GW1-noisiest-column
   referent (`stats/findings/2026-08-13-benchshape.md`).
4. Start-fixed thresholds added to the contrast table; the wild-bootstrap floor (6/6⁶ =
   0.000129) is quoted beside the S_eff values.
5. Plural scoped to the ON contrast, with the vice-captain fix's 12.7 as the reference point.

**What was declined:** nothing. The reviewer verified the pre-registration's post-run sign
correction is genuine and honestly handled (a real internal contradiction, marked in place,
dated, disclosed in the finding) — no further action.

**What could not be checked on this harness:** the runtime details (13m41s, peak RSS 120 MB,
exit 0) are not carried in the provenance and are plausible rather than checkable; the
live-cell "first chip at entry+7" detail is printed by the diagnostic, not banked. Neither is
load-bearing. The two competing readings (churn noise vs churn signal) remain unresolved by
this run — that is the pre-registered expected outcome, not a defect.
