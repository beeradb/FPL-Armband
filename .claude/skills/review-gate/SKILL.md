---
name: review-gate
description: Run the applicable reviewers over a branch's changes and write the review record that TestReviewCoversTheCurrentCode requires. Use before calling a branch done, or when that test fails.
---

# The review gate

## First: is a reviewer the right tool?

Before dispatching anyone, ask **what quantity this change must not move**, and write that test.
An invariant runs every time and costs nothing; a review costs 10-25 minutes and several hundred
thousand tokens. For a refactor claiming identical output, a differential check over many inputs
is stronger than any reading of the diff — and the reviewer should then be pointed at the test.

Reviewers are for judgement errors: an unsupported verdict, a stale mechanism argument.

## Triage

| the change touches | dispatch |
|---|---|
| `internal/analysis` — scoring | fpl-code-review, fpl-stats-review, fpl-findings-audit |
| `internal/backtest` or `stats/*.R` | fpl-stats-review, fpl-findings-audit |
| `AGENTS.md`, `docs/` — the record only | fpl-findings-audit |
| `internal/agent`, `internal/fpl`, config persistence | fpl-code-review, fpl-security-review |
| a refactor asserting identical output | fpl-code-review, pointed at the differential test |
| output of a live run that wrote config | fpl-run-review |
| the four hand-maintained season lists | fpl-season-maintenance |

Where two rows apply, take the union. Where none applies, record that no review was owed and why.

⚠️ These agents are configured per-machine and are not in this repository; `/agents` lists what is
installed here. If the table names one that is not installed, record it as unavailable — you
cannot satisfy the gate by pretending otherwise.

## Dispatching

Send them concurrently where they do not conflict; they are read-only, so they generally do not.
Every brief must carry:

- **the commit range**, not "the recent changes";
- **what is already known to be wrong**, so it is not rediscovered;
- **an instruction not to treat recorded nulls as settled** — quote AGENTS.md's "A null is a tie,
  not the refutation of one";
- **whether other agents are active**, which files they own, and whether a replay is running.

## The record

Stage the change first, then:

```bash
git add -A
go run ./cmd/armband reviewkey -out reviews/<date>-<what-it-was-about>
# write review.md beside the key.csv it just wrote
git add reviews/<date>-<what-it-was-about> && git commit
```

`reviewkey` digests the **staged index**, so the record commits with the change it reviews. Name
the directory for what the review was about — nothing parses it.

`review.md` contains:

| field | |
|---|---|
| what was reviewed, in prose, and the commit range | |
| which reviewers ran, and which were skipped with the triage reason | a skip is a decision and must be visible |
| findings, ranked by how misleading the current state is | |
| **what was applied** | |
| **what was declined, and why** | otherwise the same finding is re-raised every pass |
| what could not be checked on this harness | unmeasured and unmeasurable are different |

A filed record is a dated artefact, not a live document. Do not edit one to keep it current when
the code moves; a record naming something later renamed is correct about the moment it describes.

If the gate fires after a rebase, content moved: the key is over content, not history. Read what
`key.csv` names and review it — **do not re-key.**

## Two failure modes

**A reviewer's report is a set of proposals, not findings.** Several will be wrong. Verify before
applying, and say in the record that you did.

**A gate that never finds anything is not being run properly.** If every review returns "looks
fine", the triage is dispatching the wrong reviewers or the briefs are too vague.
