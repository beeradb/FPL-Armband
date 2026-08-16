---
name: review-gate
description: Run the applicable reviewers over a branch's changes and write the review record that TestReviewCoversTheCurrentCode requires. Use before calling a branch done, or when that test fails.
---

# The review gate

⚠️ **The reviewer agents this skill dispatches are configured per-machine and are NOT in this
repository.** The triage table below names them; whether they are installed here is a separate
question, and `/agents` answers it. **If the table names one you do not have, you cannot satisfy the
gate by pretending otherwise — record it as unavailable and say so in the review record.** Before
this skill they were invoked ad hoc, which
meant most changes went unreviewed — and the cost is documented: a constant survived long enough to
be cited as ground truth for a model that no longer existed, and the research record accumulated ten
competing figures for one quantity with none marked canonical.

**What this enforces is that review happened, not that it was good.** No test can judge a review. But
`TestReviewCoversTheCurrentCode` fails when scoring or harness code has moved since the newest review
record, which is the same trick `TestSnapshotCoversTheCurrentCode` uses and it is why the snapshot
discipline stopped rotting.

## Before anything else: is a reviewer the right tool here?

**Invariants beat reviewers for the failure modes this project actually has.** Look at what has
caught real defects: a byte-identity check on the held metric caught seam violations; reproducing
29,747 of 29,747 bonus awards caught ten duplicate archive rows; a reproducibility test caught a
map-iteration bug that had already corrupted a published figure; the environment-switch completeness
test rejected a build that was missing a registration.

Every one of those is free and runs every time. Reviewers catch *judgement* errors — an unsupported
verdict word, a mechanism argument that has gone stale — which are real but rarer and much more
expensive to find.

So **before dispatching anyone, ask: what quantity must this change NOT move?** Write that test
first. If the change is a refactor claiming bit-identical output, a differential test over thousands
of random inputs is stronger evidence than any reviewer reading the diff, and the reviewer should
then be asked to check the *test*, not the refactor.

## Triage — which reviewers, by what the change touches

Running all of them on everything is not affordable: a single review costs 10 to 25 minutes and
several hundred thousand tokens. So:

| the change touches | dispatch |
|---|---|
| `internal/analysis` — scoring | **fpl-code-review**, **fpl-stats-review**, **fpl-findings-audit** |
| `internal/backtest` or `stats/*.R` — harness and inference | **fpl-stats-review**, **fpl-findings-audit** |
| `AGENTS.md`, `docs/` — the record only | **fpl-findings-audit** |
| `internal/agent`, `internal/fpl`, config persistence | **fpl-code-review**, **fpl-security-review** |
| a refactor asserting byte-identical output | **fpl-code-review** — and point it at the differential test rather than the diff |
| output of a live run that wrote config | **fpl-run-review** |
| the four hand-maintained season lists | **fpl-season-maintenance** |

Where two rows apply, take the union. Where nothing applies, record that no review was owed and why —
a recorded "not applicable" is what stops the next pass re-asking.

## Dispatching

Send them **concurrently** where they do not conflict; they are read-only, so they generally do not.
Each brief must carry:

- **the commit range**, not "the recent changes" — an agent given a vague scope audits the whole file
  and returns a summary you already had;
- **what is already known to be wrong**, so it does not rediscover it. This project's record is long
  and partly retracted, and an agent that treats every recorded claim as settled will contradict
  today's findings;
- **an explicit instruction not to treat recorded nulls as settled.** Most were computed with the
  retired naive estimator, several at half the current cell count. Invariance results survive;
  significance-based nulls are unresolved, not refuted. Handing over a "do not revisit" list is the
  mistake to avoid. `AGENTS.md`'s "A null is a tie, not the refutation of one" is the standing rule,
  and it is the one to quote in a brief;
- **whether other agents are active**, which files they own, and whether a replay is running. Parallel
  replays get killed on this machine and a killed run is a silently partial result.

## The record

⚠️ **Changed 2026-08-15, and it is now ONE commit rather than two.** The gate used to read a commit
SHA out of the record's directory name and diff `sha..HEAD`. Two things follow from that being gone:
**the directory name is free text**, and **the record no longer has to be committed separately**.

Stage the change first, then:

```bash
git add -A
go run ./cmd/armband reviewkey -out reviews/2026-08-15-<what-it-was-about>
# write review.md beside the key.csv it just wrote
git add reviews/2026-08-15-<what-it-was-about> && git commit
```

`reviewkey` digests the **staged index** over the review watch list and writes `key.csv`, which is
what `TestReviewCoversTheCurrentCode` compares against. `reviews/` is not itself watched, so the
record can ride in the same commit as the change it reviews.

**Name the directory for what the review was about**, not for a commit. Nothing parses it, and a
name that says "the clean-sheet retraction" is worth more to the next reader than seven hex digits.

`reviews/<dir>/review.md` contains:

| field | why |
|---|---|
| what was reviewed, in prose, and the commit range if there is one | orientation for a reader; the guard uses `key.csv`, not this |
| which reviewers ran, and which were skipped with the triage reason | a skip is a decision and must be visible |
| findings, ranked by how misleading the current state is | |
| **what was applied** | |
| **what was declined, and why** | the important one. Without it the same finding is re-raised every pass, which is its own tax |
| what could not be checked on this harness | the project's standing distinction between unmeasured and unmeasurable |

`TestReviewCoversTheCurrentCode` then passes until watched content moves again — **and a rebase is
no longer watched content moving**, which is the point of the change. Thirteen commits in this
repository's history did nothing but re-key these records after a rebase, and one of them deleted
915 lines of banked figures to keep a directory name valid.

⚠️ **If you rewrote history and the gate still fires, real content moved.** Do not re-key anything;
there is nothing to re-key. Read what `key.csv` says moved and review it.

## Two failure modes to avoid

**Do not let a reviewer's report become the finding.** A report is a set of proposals; several will be
wrong, and this session has had a reviewer misattribute a movement to a commit that provably caused
none. Verify before applying, and say so in the record.

**Do not run the gate as theatre.** If every review returns "looks fine", either the triage is
dispatching the wrong reviewers or the briefs are too vague to be useful. A gate that never finds
anything is a gate nobody will keep running.
