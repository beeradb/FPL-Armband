# The xGC transport test

Covers `6031ef2..8c1ba70` on `xgc-transport-test` — refuting the substitution-correction
replay arm on its own mediator, then building and running the transport test that
replaces it.

## ⚠️ No reviewer agent was dispatched, and that is a real deviation

**The session had no `Task` tool, so none of the seven agents in `.claude/agents/` could
be invoked.** Instead the applicable briefs were read and applied by the same session
that wrote the code. That is weaker than an independent agent in exactly the way it
sounds — a self-review shares the author's blind spots — and it is recorded here rather
than glossed, because the whole point of the gate is that an unreviewed change cannot
hide behind a record covering something else.

One mitigation is real and worth stating: this session's own system brief **is**
`fpl-stats-review`, verbatim. So the statistical review is the reviewer's, not an
imitation of it. The code review and the findings audit are the ones done out of role.

**Whoever picks this up next should dispatch `fpl-code-review` and `fpl-findings-audit`
properly on this range.** The two findings below are what a self-review found; they are
not evidence that a real one would find nothing.

## Triage

| reviewer | owed? | why |
|---|---|---|
| **fpl-stats-review** | **yes** | `internal/backtest` and `stats/` — a new measurement and a verdict on it |
| **fpl-findings-audit** | **yes** | `docs/notes/archive-and-data.md` and the record edits |
| **fpl-code-review** | **yes, by judgement** | not in the triage table for `stats/`, but the entire finding rests on ~90 lines of new Python and a new Go diagnostic that nothing else checks. A bug in `transport()` manufactures the result |
| fpl-security-review | no | no agent layer, no client, no config persistence |
| fpl-run-review | no | no live run, nothing written to `config.json` |
| fpl-season-maintenance | no | none of the four hand-maintained lists touched |

## Invariants first, per the skill

**What must this change NOT move? Everything shipped.** Checked rather than asserted:

    git diff --cached -- '*.go' ':!*_test.go' | grep '^[+-]' | grep -v '^[+-]//'

returns **nothing** — every non-test Go change is a comment. `repairdata/` is untouched,
and the new `--transport` mode writes only to `stats/out/`, deliberately, because a file
under `repairdata/` for a season needing no repair would be loaded *as* a repair by
everything in the package. Full suite green on `internal/backtest` (102s).

Two guards were added to the diagnostic before it was committed, because the skill is
right that invariants beat reviewers here:

- **a positive control** — arm A must land on the recorded validation (ever-presents
  1.0088 at 3.9% MAE), or the run is measuring a code change and the input-change reading
  is invalid;
- **a liveness check** — if the Understat xG failed to arrive, arm B collapses onto arm A
  and the run reports a *perfect* transport. That is this package's signature failure and
  it is now a `t.Fatalf`.

## Findings

Ranked by how misleading the state was.

**1. "The transport assumption does not hold" invited "the repair is bad".** CONFIRMED,
and applied at `82f06f5`. The measurement compares the chain on Understat input against
the chain on FPL input. What the *replay* runs is the chain against **nothing** —
`baseXP90` gates both the clean sheet and the goals-conceded deduction on `XGC90 > 0`, so
with the repair off every defender and keeper in those seasons is scored with neither
term. A reconstruction carrying 16-20% per-match error can still be a large improvement
on a term switched off entirely, and nothing here measures that. Corrected in all three
places the result is recorded, so no reader meets one without the other.

**2. The 2022-23 ordering population is selected differently from the other three.**
CONFIRMED, applied at `82f06f5`. The 900-minute filter runs over the window where a real
xGC figure exists — GW16-38 in 2022-23, the whole season elsewhere — hence n = 120 against
143-155. Defensible as "a real body of football inside the scored window", but not the
same population, so it is now marked read-beside rather than pooled.

**3. The accuracy verdict carries no standard error.** ACKNOWLEDGED, not fixed. "3.0-5.2%
→ 16.0-20.2%" is called `established` on effect size and on replicating in four seasons,
not on a computed t. By this project's own standard that is an assertion about sampling
noise rather than a measurement of it. It is left as it stands because the fourfold change
at n = 5,184 cells is not plausibly sampling noise and four independent seasons all move
the same way — but a paired per-club-gameweek contrast with a real SE is the honest
version, and it is queued in TODO.md as what would close the *ordering* half too.

**4. The liveness guard compares two floats for exact equality.** ACKNOWLEDGED, not
fixed. It catches a total delivery failure and would not catch a *partial* one — a
crosswalk that reached half the players would still produce differing arms. The harvest's
own coverage report (0.998-1.000 of FPL's xG mass, minutes join 1.004, goals anchor
1.0000) is what covers that today, and it is printed beside the file rather than asserted
in the test. Worth converting to an assertion if this is ever re-run on a season whose
crosswalk is thinner.

## What was applied

Findings 1 and 2, in `docs/notes/archive-and-data.md`, `internal/backtest/xgcrepair.go`
and `TODO.md`.

## What was declined, and why

- **Fitting a level correction from the transport residual.** The level moves by
  0.929/0.998/1.047/1.021 and is explained to the third decimal by what the borrowed
  offset gets wrong per season. It is not a defect to correct: a level shared by every
  club is invisible to an argmax, which is the same argument that pins `xgcScale` at 1,
  and fitting it on the FPL-fed seasons to apply on the Understat-fed ones is the
  transport failure this very test just measured.
- **Shipping the substitution correction.** Refuted before it was run — normalised to 1.0
  at 90 minutes it moves season xGC by 0.3-0.6% with a 0.9-2.2% spread on the population
  that gets bought, about 0.02 pts/90 of re-ordering against a model with ~0.5 pts/90 of
  its own resolution. Its own decision rule was a trap: "the sign survives" was the
  near-certain output of an inert intervention.
- **Re-running the 18-cell `FPL_NO_XGC_REPAIR` sweep.** Nothing in this work changes what
  the replay computes, so the recorded −34 is not stale. It is still a draw.

## What could not be checked on this harness

- **Whether the transport error explains the −34.** This measures the chain's fidelity,
  not the replay's points. The second reading of that sweep now has a *measured premise*
  where it had none; it is not thereby the explanation, and the record says so.
- **Whether the ordering loss is material to squad choice.** xGC feeds the clean sheet and
  the goals-conceded deduction, 26-45% of a defender's score, so the Spearman loss reaching
  `Score` is smaller than the 0.019-0.052 measured on xGC90 — by how much is unmeasured.
- **A held-out season.** The chain can only be scored where a real xGC column exists, which
  is the four seasons it never runs on. There is no fifth.

## Sections checked and found sound

- The five contamination events do not reach this result: it is a fresh non-replay
  diagnostic, and both arms read the same archive rows.
- The cell-count rule does not apply — this is not a cell-based comparison, so "twelve
  cells or fewer" has no purchase on it.
- `spearman` is `stats_test.go`'s, not a second implementation. Both arms call the shipped
  `reconstructedXGC`. `withTransportXG` copies rather than mutates, so the process-global
  season handed to arm A is not touched by arm B.
- No oracle flag is read, set or defaulted anywhere in the diff.

## Found while satisfying the snapshot guard, after the first record was written

**5. The documented snapshot recipe omits `TestDiagTransferError`.** CONFIRMED, fixed at
`8c1ba70`. Following `staleness_test.go`'s command as written produces a snapshot with the
eight `model.transfer_error.*` rows **absent** — the buy/sell asymmetry and the
sold-player-played-again split, which is the evidence behind "the sell side is calibrated;
its error is entirely availability". Nothing fails: the renderer emits what it is given, so
a short model CSV makes a short snapshot, and the guard only asks that a snapshot *exists*.
It is worse than the stale-`/tmp` trap it sits beside, because the missing rows read as "not
measured at this commit" rather than as "the operator ran the wrong command". Caught only by
diffing against the previous snapshot, which is not a step the recipe asks for.

**6. The harness half of the snapshot series is unreproducible.** CONFIRMED, not fixable
here. Those figures come from a per-cell sweep CSV; `b2a3d9c`'s lived at `/tmp/cells.csv`
and is gone. `stats/cells/` holds two chip files, so the convention of committing them
exists and MINHL's was not followed. This snapshot therefore carries the model half only,
which is a **visible degradation of the series** and is recorded as one. Passing the old
cells forward under a new stamp was the alternative and is worse — it would attach one
sweep's provenance to a commit it never ran at, which is the shape of error this whole
package exists to prevent.

**The invariant this branch owed is now on the record rather than asserted.** Every model
figure in `2026-08-13-a4632d1` is byte-identical to `b2a3d9c`'s, which is what a
comment-only change to `xgcrepair.go` must produce.

## Unrelated, pre-existing, and not fixed here

`TestEnvSwitchListIsComplete` in `internal/snapshot` fails on `FPL_CHIP_PLAN`, "first seen
in `../../.claude/worktrees/prior-half-life-on-repaired-xgc/...`". Reproduced with this
branch's changes stashed, so it predates them. Note *why* it fires: the scan is reaching
into a worktree outside the repository. That is the "a green scan of the wrong tree"
hazard CLAUDE.md already carries a standing rule about, arriving from the opposite
direction — a red scan of the wrong tree. It will keep crying wolf while any worktree
exists.
