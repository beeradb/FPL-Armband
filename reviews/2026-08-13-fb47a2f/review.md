# Review: the two expected-goals backfill measurement runs

**Commit range**: `4d61058..fb47a2f` — fourteen commits, from the pre-registration through both
runs' cells, their analysis, and the record edits. **No behaviour changed**: the only non-comment
Go edit in the range is one test constant.

**What was done.** Two measurement runs commissioned in `TODO.md` as PRIORITY items. Run A is a
2x2 (really a 1x3 — see below) over `FPL_NO_XG_REPAIR` and `FPL_NO_XGC_REPAIR` on the four-season
grid; Run B is `FPL_NO_XGC_REPAIR` on the six-season grid. Twelve replay processes, 1,512 cells,
every one exiting 0 with the arm count its provenance sidecar declared. Full write-up in
`stats/snapshots/2026-08-13-4d61058/`, named for the commit the **cells were computed at** rather
than for the commit this review covers — the two differ deliberately, because the snapshot's
identity is its data and this record's identity is the diff.

## Reviewers

| reviewer | ran | outcome |
|---|---|---|
| **fpl-stats-review** (Run B + the switch couplings) | **yes** | returned; **eleven findings applied**, three of which changed a claim rather than softening it |
| **fpl-stats-review** (Run A) | dispatched, **no report** | completed twice without its report reaching this session. **Not discharged** — see below |
| **fpl-findings-audit** (the record edits) | dispatched, **no report** | same. **Not discharged** |
| fpl-code-review | **skipped** | no behaviour changed; the Go diff is two comment blocks and one test constant |
| fpl-security-review | **skipped** | nothing in `internal/agent`, `internal/fpl` or config persistence moved |
| fpl-run-review | **skipped** | no live run; nothing wrote config |
| fpl-season-maintenance | **skipped** | the four hand-maintained lists are untouched |

⚠️ **Two of the three owed reviews did not land, and that is recorded rather than papered over.**
Both agents were dispatched, both completed, and neither report surfaced; both were resumed with a
request for a compact summary and neither produced one. This task has now lost five subagents to
the same class of failure. **What that leaves unverified is named at the end of this file.**

## The invariants ran first, and they are most of the evidence

The gate's own rule is to ask what quantity the change must **not** move before dispatching
anyone. Four such checks were free, and three carry more weight than any point estimate in the
write-up:

1. **18 of 36 cells byte-identical** in Run B — exactly the cells the xGC repair provably cannot
   reach, named by season in the pre-registration before the run. One differing cell would have
   refuted the confinement outright.
2. **192 cells byte-identical** between the `FPL_NO_XG_REPAIR=1` and both-switches corners, with
   `.means.csv` agreeing to 17 significant digits on every arm and metric. This is the
   pre-registered test of P1, and it is why the 2x2 is reported as having three live corners.
3. **24 cells byte-identical** between the same configuration run in two *different processes* —
   `TestDiagBaseline` in the `MINHL` process against the `FIXW` process's own baseline arm. This
   is the control that licenses cross-process pairing at all, which is the design's one real
   assumption.
4. **A bit-exact reproduction of an independently produced snapshot.** The `FPL_NO_XGC_REPAIR=1`
   and `FPL_NO_XG_REPAIR=1` corners reproduce `stats/snapshots/2026-08-11-0104d9d/`'s two arms to
   all 17 significant digits on both metrics, across a day and a code change.

## Findings applied, from the review that did land

All eleven are marked ⚠️ in place in `FINDINGS.md`. The three that changed a claim rather than
softening it:

**1. A sentence was factually wrong about its own data.** The write-up said two seasons were
"carried by a minority of large negative cells". 2021-22 and 2022-23 are each **4 of 6** negative —
a majority. The season that is a near-tie is 2020-21, at 3 of 6. Corrected to "two negative seasons
and one undecided". Verified against `cells/runB-paired.csv` before applying.

**2. The season-clustered *t* is the least trustworthy of the three, not the most.** The first
draft leant on CR2's −2.71 (df 2). `variance_components.txt` reports `F_seas = 0.14, p = 0.871` —
an F *below one*: the three season means landed **closer together than within-season noise alone
predicts**, which is a low chi-square draw rather than seasons agreeing, and it makes the clustered
SE anticonservatively small. The harness's own printed rule prefers `t fixd` (−1.02, df 10) where
`%seas` is near zero, and it is exactly 0.0 here. All three are now reported as a range, and the
threshold is quoted as **55 to 150 a season** rather than as a single 54.

**3. The sign of the headline flips across defensible estimands** — −34 a season equal-weighted,
−25 inverse-variance, **+14 on GW1 entries alone**, −17 over all 36 cells. The review also
surfaced the axis that explains it: early entries average +0.11 pts/gw and late ones −1.92. This
is now the decisive fact in the write-up, and it demoted the headline from a measured price to a
point estimate whose interval spans zero.

The other eight, more briefly: the 18-cell restriction needed the *fabricated degrees of freedom*
argument (the 36-cell fit reads df 5 because three season means are constants) rather than only
the dilution one; the 36-cell figure is now reported alongside; the threshold is the *t* restated
and not a second witness; the external-validity confound is severe (the three reachable seasons are
the three worst-instrumented, and a held-out season is unavailable **by construction**); 45% of the
effect is captaincy and the rungs are now printed; the provenance paragraph was rewritten to the
verifiable pairwise form; the mediator carries P6 while the `HOLD` census only *is consistent with*
it; and the "established / not free" claim was downgraded, since `established` is this project's
strongest verdict word and no estimator here earns it.

**One finding was applied without a reviewer, from re-deriving the arithmetic**: the three level
contrasts in Run A are a **sequential** decomposition and exactly additive, so "plus any
interaction, inseparably" was the wrong caveat. What is unavailable is the interaction itself,
because the fourth corner does not exist. Corrected at `fb47a2f`.

## What was declined

**Nothing from the review that landed was declined.** All eleven were verified against the
artefacts and applied.

**One thing the runs suggested and this change declines to do**: give the xGC repair its own gate
so that "xG off, xGC on" becomes reachable. It is a small code change and it would make the 2x2 a
real 2x2. Declined here because this branch was commissioned as measurement and a model change
inside it would make every figure above harder to attribute. Queued in `TODO.md` instead.

**`DCC` was declined as a Run A block**, and on an argument rather than on cost. `defconCleanFactor`
returns 1 whenever `dc90 <= 0` or the player is not a defender; defensive contribution scores in
2025-26 alone; the two backfills reach 2022-23 alone on the four-season grid. The live sets are
disjoint, so a `DCC`-by-data-state interaction is **identically zero by construction** and running
it would have produced a tight null on a comparison that could not have moved — this package's
signature failure. Checked by reading `teamstrength.go:304-310` rather than by running it.

## What could not be checked on this harness

- **The xGC repair's points effect cannot be resolved, and not for want of cells.** It lives in
  three seasons, so df 2 and a critical value of 4.303; a season with native xGC is a season where
  the repair is inert, so no widening of the grid adds a cluster. A Rademacher wild cluster
  bootstrap floors at p = 0.250 on three clusters.
- **The 2x2's fourth corner does not exist**, so the interaction between the two backfills is zero
  by construction rather than by measurement.
- **The `.means.csv` end-to-end check is unavailable for any cross-process pairing**, because Go
  never computed the pairing. `pair_cells.py` deliberately refuses to hand-write one — supplying a
  check with its own answer turns a pipeline test into a tautology.
- **Whether the xGC repair's negative sign is the reconstruction's own substitution bias** rather
  than "a better-specified objective makes a worse policy". Both are live; the experiment that
  separates them is queued.

## What is NOT discharged by this record

Because two of the three owed reviews did not report, the following are **unreviewed** and should
be treated as such by whoever picks this up:

1. **Run A's section 2 in `FINDINGS.md`** — in particular the single-season level decomposition
   (−53.8 / +82.7 / −3.7 / +136.5 a season, all on 2022-23 alone with no clustering available),
   the "the weekly fill carries essentially all of the effect" conclusion drawn from it, and
   whether "the top of each ladder's order is stable across four data states" is a real shape claim
   or one observation counted four times.
2. **The record edits** — `CLAUDE.md`, `TODO.md`, `docs/notes/archive-and-data.md`,
   `stats/README.md`, and the two retracted-in-place Go comments — for the specific failure the
   first review caught three times in the first draft: a claim stated more strongly than its
   source. The CLAUDE.md bullets were compressed hard against a byte budget, and compression is
   how hedges get lost in this file.
3. **The `TestEveryNoteIsIndexed` budget raise from 52 to 54 KB.** The constant's own comment
   permits raising it for a claim that needs its hedge back, and names the three hedges here — but
   nobody independent has judged whether that is what happened, or whether evidence that belongs in
   a note was left resident.

## One thing to know before reading the commit log

Commit `29aa7aa`'s subject says *"the xGC repair costs 34 points a season on HOLD and does not
resolve"*. The first clause is stronger than the review left the finding. **The commit subject is
not the verdict**; `stats/snapshots/2026-08-13-4d61058/FINDINGS.md` is. Recorded because this
repository has been bitten before by a figure being quoted from wherever it was convenient rather
than from where it was qualified.
