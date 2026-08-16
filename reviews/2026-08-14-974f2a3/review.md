# Review — `4ba97f8..ba9d5fc`, branch `worktree-cs-xgc-factor-rejudge`

Two queued items worked: `FPL_CS_XGC_FACTOR` (item 3) and the `prior_half_life` unrepaired
replication (item 2). **The review changed the outcome of both**, so this record is mostly about
what the reviewers caught rather than about what was shipped.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** | the branch makes two measurement claims and declines a queued sweep on one of them | **Refuted the headline claim** by finding a defect in the diagnostic it rested on |
| **fpl-findings-audit** | `CLAUDE.md`, three notes and `TODO.md` changed | 14 findings; 9 applied, 3 superseded by the stats review, 2 declined |

Skipped, with reasons: **fpl-code-review** — the only non-test Go change is a comment in
`cmd/priorblend`; the diagnostic and dump are `_test.go` and were reviewed by the stats pass as
instruments. **fpl-security-review**, **fpl-run-review**, **fpl-season-maintenance** — nothing in
`internal/agent`, `internal/fpl`, config persistence, no live run, no season lists.

## Was an invariant the better tool here?

Partly, and this is the honest part of the record. The defect below is exactly the class an
invariant catches and a reviewer catching it was luck of the draw. **The guard that should have
existed is "every diagnostic that reads a `GW` row states its position on double gameweeks"** —
five files carry `Fixtures != 1`, one did not, and nothing counted them. That guard is **not
written here** and is queued in TODO.md instead; it wants a decision about which diagnostics are
legitimately double-blind before it can be a test rather than a nuisance.

Two guards *did* fire during this work and both were load-bearing: `TestEnvSwitchListIsComplete`
caught `FPL_CS_ROWS` unregistered, and `TestTheSharedCellQuantitiesHaveOneImplementation` caught a
raw `read.csv` in the new R script. Neither was a near miss — both would have been real defects.

---

## Finding 1 — `TestDiagCleanSheetPoisson` was counting a double gameweek as one match

**Raised by** fpl-stats-review. **Verified independently** before applying: five sibling
diagnostics carry `if g.Fixtures != 1`; this file had **no reference to `Fixtures` at all**.

Since the doubles fix (`89fa973`, 2026-08-08) `loadGameweeks` accumulates, so a doubled row carries
xGC summed over both fixtures while `CleanSheets > 0` still reads "at least one". The diagnostic
compared **P(zero goals across both matches)** against **a clean sheet in either** — the
intersection against the union.

Measured on the 85 such rows: the model predicts **0.1255**, they realise **0.4235**, and
`1−(1−0.2383)² = 0.4199`. The union signature to two decimals; this is not ambiguous.

**Applied**: the guard, plus a comment naming the four siblings and the commit that gave them theirs.

**Consequence for the record**: this refutes the "wrong family" finding written the same day. See
finding 2. It also means **every banked `clean_sheet_calibration` figure from 2026-08-08 to
2026-08-14 is contaminated** — `error` 0.0592 and `points per match` 0.2368 against the corrected
0.0698 and 0.2791, n 2955 against 2870. Recorded in TODO.md and the note.

## Finding 2 — the "wrong family" claim is retracted, and the fit reverses

**Applied in full.** With the guard, and fitted per observation instead of on six bucket means:

| population | n | a | b | pure SCALE | pure OFFSET |
|---|---:|---:|---:|---|---|
| all rows | 2870 | +0.144 (0.049) | +1.120 (0.053) | f = 1.256, p = 0.0007 | a = +0.238, p = 0.024 |
| native only | 2598 | +0.100 (0.050) | **+1.173** (0.057) | f = 1.268, **p = 0.029** | a = +0.238, p = 0.0026 |

`b > 1` in **4 of 4 seasons**. Of the two one-parameter families the **multiplier is the
less-rejected one** — the reverse of what was claimed. Neither is adequate.

**Three errors were stacked, and the record now says so**: ~43% of the retracted intercept was the
85 contaminated rows, ~27% was the bucket-mean estimator (biased by `a +0.10, b −0.09` on the same
rows — now *measured* beside the MLE in `stats/cs_calibration.R` rather than asserted), and the
direction was pre-ordained regardless, because errors-in-variables in a regressive regressor
produces `a > 0, b < 1` mechanically.

⚠️ **The Jensen bias had been flagged in the original write-up.** Flagging it was not enough. The
lesson recorded is that a caveat which invalidates the estimator should stop the write-up, not
annotate it — the fix was one `glm()` call.

**Verified independently**: I reproduced every figure the reviewer quoted, including the
bucket-mean estimator on the same rows, before rewriting anything.

## Finding 3 — the provenance retraction stands, and gains a count that needs no inference

**Applied.** `f5fcc91` is 2026-08-07 01:52; the doubles fix is 2026-08-08 17:54 — checked from
`git log`. **Every arm of the ladder ran with doubles half-counted, ~106–115 a season**, which is
more than twice the 188-point MID gap the retraction previously leaned on. The ladder does run
*after* the zero-penalty guard and the selling-price fix; both checked so nobody repeats it.

**Also applied**: the MID = 1.0 claim is relabelled as **forensic inference**, not fact. The audit
found that `cleanSheetXGCFactor` is *added by* `f5fcc91`, so **no committed tree in this repository
can run that ladder** — both tables came from an uncommitted working tree, which is why no config
artefact survives and why its absence is not counter-evidence.

## Finding 4 — the shipped mechanism argument is narrower than recorded

**Applied**, and this is the most consequential surviving finding. `exp(-f·x)/exp(-x) =
exp(-(f−1)·x)` **depends on x**, so `FPL_CS_XGC_FACTOR` is a **within-position reprice**, not the
position-wide rescale that the "a bias shared by every player in a position is not an ordering
error" exemption covers. On the clean fit the non-shared component is the *larger* one.

The exemption still covers a **flat** clean-sheet rescale. It does not cover `f`. **Unmeasured** —
recorded as the open question rather than as a verdict.

The note's closing paragraph ("the bias does not vary between them") is now marked in place as
refuted by the table above it, and `TODO.md`'s "a flat multiplier cannot, by construction" is marked
as wrong about *this* knob.

## Finding 5 — item 2 re-ran the superseded population

**Applied, and re-measured.** The first pass re-ran only `popFieldSound`, which `cmd/priorblend`
marks SUPERSEDED, and then wrote "quote this from now on" five lines below the paragraph saying
never to quote this arm there. Both reviewers caught it independently.

| arm | population | rank_corr | t (season) | df | raw p | negative |
|---|---|---:|---:|---:|---:|---:|
| repaired | `popEveryone` | −0.000389 | −4.83 | 5 | 0.0048 | 6 of 6 |
| unrepaired | `popEveryone` | −0.000493 | −4.06 | 5 | 0.0097 | 6 of 6 |
| unrepaired | `popFieldSound` | −0.00051 | −4.66 | 5 | — | 6 of 6 |

The repaired arm **reproduces the recorded −0.00039 / t −4.82 / p 0.0048 to every digit**, so A3
left it untouched exactly as predicted and the **Holm-clearing 0.0385 stands**. `p = 0.031` stands:
six of six on `popEveryone` in both arms.

⚠️ **The unrepaired arm is not Holm-clearing on its own** (0.0097 × 8 = 0.078). It is the
replication, carried by the sign test. Recorded, because the first write-up would have implied
otherwise.

**The mediator check is the strongest single result on this branch**: between the two arms, 2023-24,
2024-25 and 2025-26 are **identical to the digit**, and only the three seasons the xG backfill
reaches move. "That moves the `FPL_NO_XG_REPAIR=1` arm and only that arm" was an assertion; it is
now a measurement.

## Findings 6–9, applied without argument

- **"Falls monotonically" was false on its own table**, in both data states — the clean `f_zero`
  column rises at two of five steps. In a record whose standard for a shape *is* monotonicity, the
  wrong word to get wrong. Corrected everywhere it appeared.
- **The "sharpening" is withdrawn.** On clean rows the residual is same-signed in **6 of 6**
  buckets, so "over-predicting at every level" was right as written — and bigger than recorded.
- **The retracted claim survived in the section heading, the opening sentence and a closed TODO
  entry.** All three now carry markers; the retraction had been written 25 lines below the heading
  asserting it.
- **`constants-and-sweeps.md` and the `cmd/priorblend` comment** — the note that owns the
  `prior_half_life` tables and the comment *inside the binary that produced them* still carried
  pre-A3 figures. Both marked. The code comment mattered most: this project's convention makes it
  the primary artefact.

## Declined

- **Regenerating the accuracy snapshot.** The corrected figures are measured and recorded, but a
  full snapshot needs eight model diagnostics **and** a harness sweep (`EXP=MINHL`, 180m timeout),
  and snapshots are dated point-in-time records rather than files to edit. The supersession is
  written into TODO.md and the note with both figures, so the next snapshot will move for a reason
  that is already explained. **This is a deferral, not a judgement that it does not matter.**
- **Adding the "every diagnostic states its double-gameweek position" guard.** Wanted, and queued —
  but it needs a decision about which diagnostics are legitimately double-blind, and inventing that
  taxonomy inside a retraction commit is how a guard ends up asserting something nobody checked.
- **`harness-and-inference.md`'s "arms this frees" list.** The audit proposed removing
  `FPL_CS_XGC_FACTOR` from it. **Declined and reversed**: the line is now *open*, so the pointer is
  correct as it stands. This is an example of an audit finding that the concurrent stats review
  invalidated.
- **Two of the audit's proposed edits to the "dominantly an offset" paragraph.** Superseded — the
  paragraph was deleted rather than hedged.

## What could not be checked on this harness

- **Whether the `f` arm's within-position reprice costs points.** It needs the 2x2, and neither
  `SetCleanSheetXGCFactor` nor the flat P(CS) knob exists. Unmeasured, not unmeasurable.
- **Whether the offset component is real at all.** Errors-in-variables in the regressor produces
  the offset signature mechanically, so this instrument cannot separate a genuine offset from the
  regressor's own noise. Separating them needs a smoother regressor — the model's own blended
  season rate — which is a different diagnostic.
- **`needsSweep` does not deliver a native population.** 2022-23 is flagged `nativeXG` at the
  *season* level while FPL added the fields mid-season, so its GW1-15 rows are reconstructed and a
  season-level flag cannot express a within-season hole. Those 272 rows lean offset-ward and are
  what flips the "all rows" line. Recorded; the native cut is the one quoted.

## The generalisable lesson

**A harness fix can break a diagnostic written before it.** The doubles fix changed what a *row
means*. Four consumers were updated the next day; a fifth was not, and 3% of its rows then carried
43% of a family verdict. **When a loader changes what a row means, grep every consumer of that row,
not only the ones in the commit that changes it.** Third instance of this shape in the record.

---

# Round two — the surface written *in response* to round one

Round one's fixes were themselves unreviewed: `stats/cs_calibration.R`, `rowdump_test.go`, the
`Fixtures != 1` guard and the rewritten `CLAUDE.md` bullet were all written after the reviewers
returned. The user's standing instruction — *build the fixture and then get a review, otherwise the
fixture will not be reviewed* — applies exactly here, and the repo agreed: after rebasing onto the
restructured `main`, `TestReviewCoversTheCurrentCode` fired, because **`stats/*.R` is a watched
path** on the new main and `cs_calibration.R` is a watched file.

**fpl-stats-review**, second pass. Every item below was independently reproduced before applying.

## The thing I most wanted checked: the GLM converges

I published six fits without checking convergence. A binomial GLM with a **log** link is not
guaranteed to converge — the linear predictor must stay ≤ 0 to be a probability — and a failed
`glm` returns with a *warning*, `$converged = FALSE`, and coefficients from wherever IWLS stopped.
`Rscript` defers warnings to the end of the run, **after every table has printed**. That is the
silent-failure shape exactly.

**All six converged cleanly**, interior, 3–4 iterations, no boundary, max fitted p ≤ 0.963. Six
different starting values all land on `a = +0.10030, b = +1.17310`. **Nothing published was junk.**

**The check is added anyway**, as `fail` rather than `note` — a non-converged fit that still prints
a coefficient is worse than no output. Applied to all three restrictions and to the per-season loop,
which was previously unguarded.

## The best finding: the hard-coded boundary was unnecessary

`cs_calibration.R` cut the native population with `!(season == "2022-23" & gw <= 15)`. That is a
**fourth copy** of a boundary `xgrepair.go` already owns (`"2022-23": {FirstGW: 1, LastGW: 15}`) —
this record's signature failure, committed in the middle of a branch about that very failure.

**And it was unnecessary.** Verified independently: the native subset of a repaired dump is
**byte-identical** to a dump taken under `FPL_NO_XGC_REPAIR=1` — 2598 of 2870 rows, all six
columns, `max |Δxgc| = 0`. So the cut is gone; the data state is chosen by the existing, tested,
already-fingerprinted switch, and the script fits whatever it is handed. **The R no longer knows
what year FPL added a column.**

## Overstatement in `CLAUDE.md`, corrected

The bullet claimed "the multiplier family fits *better* than the offset, not worse". Two problems,
both real:

- **The ranking reverses between populations**, and the script prints the all-rows verdict *first*.
  The native arm is defensible on mechanism — the reconstructed rows are a noisier regressor and
  dropping them moves the fit *away* from the offset, as errors-in-variables predicts — but that
  argument has to be *in the bullet* or it reads as picked.
- **Both restrictions are rejected on both populations.** `TODO.md` said so; `CLAUDE.md` did not,
  and `CLAUDE.md` is the only copy that loads. A reader would take it as licence to sweep `f`.

Now reads: native rows only, ranking reverses on all rows, `a = +0.100 (t 1.99)`,
`b = +1.173 (t 3.22)`, **both** restrictions rejected, neither family adequate.

## An error in the 2x2 I had proposed

`TODO.md` specified `f ∈ {1.0, 1.27}` crossed with a flat arm. **1.2678 is the `a = 0`-constrained
fit — it has already absorbed the level into the slope**, so crossing it with a flat multiplier
double-counts the level in the both-on corner. Corrected to the orthogonal decomposition of the
*joint* fit, `P = 0.9046 · exp(-1.1731·x)`: **`f ∈ {1.0, 1.173}` × flat `∈ {1.0, 0.905}`**, under
which the both-on corner reproduces the free fit exactly.

This also produced better sizing than the argument I had: `exp(-0.1731x)` spans **1.44x** across the
10th-to-90th xGC percentiles, where `f = 1.27` would span 1.75x — so **a ladder over `f` alone
over-corrects the ordering by ~40%**, which is a concrete reason for the 2x2 rather than an appeal
to a rule.

## Also applied

- **Cluster-robust SEs are now printed** (season × team, 80 clusters), because 2598 team-matches are
  not 2598 independent draws. ⚠️ The Wald and the LRT **disagree at the boundary**: `t(a = 0)` is
  1.99, which this record's `|t| < 2` convention does not reject, while the LRT p is 0.029, which
  does. Both are printed so the surviving family cannot depend on which test someone ran.
- **The row dump was not reproducible.** `for gw := range p.GWs` is a map range, so two identical
  runs produced files differing on ~3300 lines — in a file whose own comment explains why a
  diagnostic that moves between runs trains its reader to ignore the diff. Gameweeks are now sorted;
  verified byte-identical across runs.
- **The dump swallowed three errors.** A failed `os.Create` left the *previous* run's file on disk
  with the test still passing and `t.Logf` invisible without `-v`, so R would fit the wrong
  population and report it as the right one; a failed write or `Flush` truncates it. All three are
  now fatal — the repo's own "a killed sweep leaves a partial cells file" hazard.
- **A latent weights bug**: `aggregate` drops empty factor levels and `table` does not, so indexing
  counts positionally silently mismatches them the moment a bucket is empty. Inert today, fixed.
- **The `< 100` per-season skip** was arbitrary and silent, and cut on a population the header did
  not name. Replaced with a condition that means something, and any dropped season is now announced.
- **The bucket-mean gap was quoted from the wrong population** — "a +0.10, b −0.09" is the *native*
  gap while the script printed the all-rows one, about half that. Both are now named with their
  data state.
- **`cmd/priorblend`**: "reproduces … t −4.82 exactly" softened to "to the recorded precision" (it
  prints −4.83), and **"0.078 over the eight-arm family" relabelled Bonferroni, not Holm** — Holm
  would rank it second and give 0.068. Nothing turns on it, but a mislabelled correction gets copied.

## Declined in round two

- **Switching away from the log link.** Checked and it is right: `p = exp(-(a+bx))` makes
  `binomial(link="log")` the model exactly and the nesting exact. Quasi-binomial is unusable — a
  *Bernoulli* dispersion parameter is not identifiable, so it would rescale every SE by an arbitrary
  number. `nls` loses the LRT; fitting the complement is a different model.
- **Recording anything further in `CLAUDE.md`.** The remaining items are hygiene on the instrument,
  and the instrument is not what that file records.

## Still not done, and deliberately

The accuracy snapshot is still not regenerated — same reason as round one, and the banked
`clean_sheet_calibration` figures are still marked contaminated in `TODO.md` and the record. The
double-gameweek guard is still queued rather than written, for the taxonomy reason.
