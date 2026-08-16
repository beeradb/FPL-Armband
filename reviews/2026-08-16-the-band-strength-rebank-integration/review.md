# The `BandStrength` re-bank, integrated

## What was reviewed

`integrate-the-band-strength-rebank`, merging `rebank-band-strength-zero-on-repaired-archive` onto
`main` (`460d3ce`), plus the `CLAUDE.md` entries I wrote on top. The source branch carries its own
record at `reviews/2026-08-16-band-strength-remeasure/`, including its reviewers' findings.

## The item's actual purpose was achieved, and its stated purpose was not

**Achieved: the cells are banked.** `stats/snapshots/2026-08-16-band-strength/cells/` now holds
`bandstrength.csv`, its `.means.csv` and a `.provenance.csv`, plus a repeat run. `BandStrength`
having **no row anywhere under `stats/snapshots/*/cells/`** was the reason this item stayed open —
I verified that absence myself at the start of the session, and it is now closed.

⚠️ **Not achieved, and I briefed it wrongly: this did not re-test the recorded verdict.** The
constant was decided by an arm at **s = 0.25**. The run swept 0 / 1 / 2. **s = 0.25 remains unrun.**
The originating commit records the real verdict as *"the best of six swept values against a ±20
noise floor"* which *"loses 12 out of sample on 2022-23"* — an **argmax whose winner sits inside its
own stated noise floor**, which is the single most load-bearing failure mode in this record. My
brief said "re-measure `BandStrength`", the private-store note said the same, and neither named the arm.
**The agent caught this itself and withdrew its own headline.**

## Findings

### Applied

1. **The result, with both its thresholds.** `HOLD`, s=1 against 0: +0.357 pts/gw (+13.6 a season),
   CR2 SE 0.184, df 5, t 1.94, threshold **18**, MDE **24**. **Unresolved, below its own MDE.**
   ⚠️ Recorded as *unresolved* and explicitly **not** as unmeasurable — the agent claimed
   unmeasurable first and its statistics review refuted it: the s=2 canary reads −10.6 against 27
   and came back **smaller and opposite**, so it sizes nothing. "Unresolved twice."
2. **That the original refutation was `POLICY`, not `HOLD`** — so no `HOLD` figure speaks to it, and
   the agent's own "2022-23 reverses sign" claim was **metric-crossed and withdrawn**. On `POLICY`,
   2022-23 reads −16.3 at s=1, the same sign as the original −12.
3. **Why that is now established rather than inferred, which is the best thing in this batch.** At
   the originating commit, `FPL_BAND_STRENGTH` reached only the `base` SimConfig handed to the
   transfer policies; the hold row was built from a fresh `SimConfig` that never saw it. **So the
   hold baseline was byte-identical across that entire sweep by construction, and only `POLICY` rows
   could move.** That is a textbook instance of this record's own trap — a byte-identical result
   that was a comparison which never ran — and it is now a documented case rather than a warning.
4. **Two defects the run turned up, recorded and NOT fixed:**
   - **`teamBands` is not run-to-run deterministic** (map-ranged slice, non-stable sort), moving 3 of
     36 cells on its own deciding column, ~0.7 a season. ⚠️ Latent in the replay but **reachable by
     a user**: `FPL_WEIGHT=band=1` reaches a live `fplagent review`. `Optimize` had the identical
     defect and is pinned by `TestSeedOrderIsDeterministic`; this path has no equivalent.
   - **A pre-season point-in-time leak.** `PreSeasonWith` returns fixtures **unfiltered**, and
     `buildTeamRates` gates on a scoreline being non-nil rather than on `Finished` — under a comment
     asserting the opposite. `TestPointInTimeHidesFutureResults` has **never tested `through = 0`**,
     so the guard pinning the in-season path never reached this one. Behind `FPL_MAGNITUDE`, so
     latent.
5. **`FPL_NO_XGC_REPAIR` bears on 18 of 36 cells**, not the 6 an earlier note claimed.

### Declined

- **Fixing either defect in this branch.** Both are real; neither is what this branch is for, and
  the determinism fix in particular is a live-path change owed its own confinement and liveness
  evidence. Logged rather than absorbed.
- **Running `s = 0.25` here.** It is the obvious follow-up and it is a different arm with a
  different pre-registration. Recorded as unrun in `CLAUDE.md` in the same breath as the result, so
  the result cannot be read as having closed the question.
- **Marking the retraction in `bands.go` and `docs/configuration.md`.** The agent reports writing it
  and having it reverted outside its session. Not re-applied here — a doc edit needs the docs
  reviewer, and `docs/` is in the watch list. Left for that pass rather than slipped in.

## A concurrent session corrected the same document, and one of its corrections is wrong

`origin/main` moved three commits mid-gate, all of them correcting `docs/replay.md` — the same file
I had corrected an hour earlier. Four conflicting hunks. **I took their side in every one**, because
their pass was deeper on each overlap:

- they identified `FPL_NO_LEGAL_AUTOSUBS` as restoring a **named contamination event worth 7-14
  points a season**, where mine only described the behaviour;
- on memory they found the band is **89-142 MB**, not the 94-130 I recorded, that **97 and 145 are
  different statistics** (a wrapped-run cost against the 1 Hz sampler's peak), and that
  `2026-08-16-anti-residual-gate/` **disagrees with itself**, 117 in `console.txt` against 130 in
  `FINDINGS.md`;
- they found the slot semaphore is keyed by a hash of the **repository path**, so the limit is per
  checkout rather than per machine — a second worktree gets its own three;
- and they documented three switches I had declined as out of scope.

My two unique additions auto-merged and are kept: the silent-fallback paragraph, and the
`FPL_*_ROWS` class row.

⚠️ **But their new `FPL_APPEARANCE_FIT` row said a partial override is "refused rather than
half-applied", and that is wrong in the direction that matters.** `appearanceFit`
(`internal/analysis/sweep.go:566-582`) returns the four **shipped** values on wrong arity or on one
unparseable field — **silently, with no error**. "Refused" reads as a loud rejection. Left as
merged, the page would have contradicted my own paragraph six lines below it, which lists that
switch among the silent ones. **Corrected here, verified against the source.**

That is the concrete case for merging early: two sessions corrected one document within an hour,
neither knew, and the merge is the only place the disagreement surfaced.

## What could not be checked

- **Whether `s = 0.25` reproduces the original +18.** Unrun.
- **The size of either defect on a live `fplagent review`.** Only the replay-side 3-of-36 count
  exists; nothing measures the user-facing path.
- **No `FINDINGS.md`** in the snapshot directory — the agent's harness refuses report files. The
  verdict lives in the commit bodies and the source branch's review record. Every other named
  snapshot has one, so this is a gap in the series rather than in the evidence.

## Redaction note — 2026-08-16

One phrase above was edited after this record was filed. It named a private store this
repository may not name; it now reads **the private-store note**. The finding is unchanged —
the point stands that the brief and that note agreed and neither named the arm.

⚠️ **Cleaned rather than exempted.** The standing exemption for already-committed
disclosures is a grandfather clause over an enumerated set; this was found afterwards. The
cost — amending a dated attestation — is acknowledged, which is why this note exists rather
than the edit being silent. **No finding was altered.**
