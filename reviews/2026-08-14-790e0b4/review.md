# Reviewing a migration that took the research record out of the repository

Covers `4ba97f8..790e0b4` — four commits that removed `docs/notes/` (nine notes, 887 KiB) and
five design documents, stripped CLAUDE.md's 34 links into them, rewrote ~190 references across
Go, R, `TODO.md` and the remaining docs, and repaired the guards that lost their subject.

`docs/` is now eight files, all reference. The record's evidence half is no longer held here;
its verdict half is resident in CLAUDE.md, unchanged.

## Reviewers dispatched, and why

Four agents across two rounds, none of them me — self-review is not a review.

**On the plan, before any file moved:** a claim-verification pass told to prefer refuting, and
`fpl-findings-audit` asked for judgement rather than facts. **On the executed diff:**
`fpl-findings-audit` again, on whether any verdict changed meaning, and `fpl-code-review` on
the guards.

Not dispatched: `fpl-stats-review` — no measurement is implicated by this change, and the two
audits independently confirmed nothing here needs a run. `fpl-security-review` — no credential,
cache or agent-loop surface is touched.

## The thing most at risk came through clean

All ~130 verdict bullets in CLAUDE.md are **text-identical** to `4ba97f8` after normalising
away the stripped links and bold. No qualifier lost, no hedge dropped, no ⚠️ removed, no
"suggestive" promoted to "established", every `t`, `p`, threshold and cell count intact. The 12
`TODO.md` rewrites and ~20 Go/R comment rewrites are faithful path→name substitutions.

That is the failure this project has recorded most often — "compression and overstatement are
the same operation past a certain point" — and this migration did not commit it. Everything
below is pointers and guard scope.

## The guard was the defect, and it failed in the way it was written to catch

`TestNothingCitesTheRecordByPath` matched **one literal** and passed green over **nine live
dangling pointers**, on the day it was written. Two blind spots it could not see:

- the reference docs cite the directory **relatively**, as a markdown link from inside `docs/`,
  so the repo-root prefix never appears;
- Go's idiomatic citation is a `filepath.Join` over separate quoted segments, containing **no
  slash at all**.

`mermaid_test.go` still held a dead glob in exactly that `Join` form — the same silence that
`retracted_test.go`'s sibling deletion was reasoned about at length, in the same commit,
applied to one of the two files that had it. So the guard could not see the one instance its
own package had just created.

Widened to enumerate the forms, extended to the five design documents (**nothing was watching
those at all**), given a floor on the file count so a reorganisation cannot silently reduce the
scan to `CLAUDE.md` alone and still report PASS, and renamed to
`TestNoLivePointerCitesTheRecordByPath` for what it actually covers. **22 offenders across 18
files, up from 0.**

The self-match avoidance is load-bearing rather than defensive: `goSources` walks `_test.go`,
so the test scans its own source and an unsplit literal would self-fail. The reviewer verified
the split works by planting three probe files and confirming each was caught.

## Three claims of mine were stronger than the diff supported

- **`docs/README.md` said the guard catches a returning path.** It did not, at the commit that
  wrote the claim. Corrected, and the correction names the failure rather than quietly fixing it.
- **CLAUDE.md told a reader not to re-measure and treat the new number as arbitration.** That
  forbids the operation by which this record corrects itself — `min_gain` reversed at twice the
  sample; the minutes floor's argmax protection re-measured at −40. Re-measuring is still how a
  verdict falls. What changed is that a new number now sits *beside* the recorded one instead of
  replacing a reading of the same cells, so the instruction is to say which you have.
- **"The evidence is not held in this repository" is too absolute.** `stats/snapshots/` holds 60
  banked runs, twelve `FINDINGS.md` and eleven `cells/` directories, and CLAUDE.md cites them by
  name. That is now the *only* checkable evidence layer and neither new section named it.

## What the migration lost, stated as costs rather than details

- **The completeness direction of the seam is gone, and it was missing from the cost list.**
  `TestEveryNoteIsIndexed` asserted both directions; only "every link resolves" still has a
  subject. **Nothing can now tell that a finding has silently left the record** — which the
  original test called worse than never having written it. Unenforceable from a checkout.
- **The retraction guard no longer reaches the evidence** — 9 files, 908,682 bytes, roughly twice
  the remaining prose surface. The reviewer checked this was a live population, not a notional
  one: grepping the distinctive retracted literals at `4ba97f8` hits **7 of the 9**. Deleting the
  glob rather than repointing it is right, and the reviewer looked for an alternative that keeps
  coverage inside the checkout and found none.
- **Edits to the moved material no longer owe a review.** Dropped, not relocated.
- **`TODO.md` is a known gap.** It is a work queue read forward, so a stale pointer there is a
  task *premise* — which `retracted_test.go` argues, about that very file, is worse than
  misinforming a reader. Its open items were swept by hand; nothing stops the next one.

## The budget went to 72 KB, and the reason is not that the file grew

The move **freed** 675 bytes: the notes were never resident, so stripping 34 link targets was the
only size effect. My first draft of the plan argued the opposite — that the budget needed raising
*because* of the move — and had the mechanism backwards.

What actually changed is that **there is nowhere to move evidence to.** The old failure message
said "move it into the notes and leave a verdict behind"; that is now unfollowable, and the
destination must not be named here. So the budget stopped being a ceiling with an overflow valve
and became a hard one — and a hard ceiling held at the old number makes deleting a hedge the
cheapest available edit, which is precisely what that comment documents four instances of. The
review of this commit then spent the 675 bytes and 146 more restoring qualifiers, which is the
same pattern arriving on schedule.

## One thing the record nearly lost outright

The rank-objective handoff was **not** a closed document — it carried three follow-ups after
review, the live one being the captaincy correction against `average_entry_score`. The `docs/`
index row that said so was deleted in the same commit as the document. So the closed half stayed
resident under a heading reading "do not rebuild these", and the open half was nowhere at all.
`TODO.md` now carries the three; CLAUDE.md's closed line is marked ⚠️ against it.

This is the migration's only substantive loss of *live* content, and no test could have caught
it: the retraction guard checks figures, the review gate checks commits, the notes guard checks
paths. It was found by reading.

## Not done, deliberately

- **The performance history stays.** `snapshot.md` is generated by `fplagent snapshot` from the
  same `Render` call as the CSVs, so moving the 46 existing files would be undone silently by the
  next run, in the directory just emptied — and pointing the renderer elsewhere would name a
  location this repo may not name. The 12 hand-written `FINDINGS.md` were the only movable part,
  against three Go comments that would dangle. **A pre-registration must never be separated from
  its own findings**, and five sit in those directories.
- **`docs/architecture.md` and `docs/model.md` stay.** A scoring constant and its description in
  `model.md` must be *simultaneously* true; a note and the code it describes need only be
  *consistently* true. That distinction is the whole of what separates them from the notes, and
  if it does not hold the notes should have stayed too.
- **`docs/accuracy.md` stays**, derived from `stats/snapshots/`.

## What could not be checked here

- **Whether a cloud reviewer loses as much as claimed.** No cloud configuration is in this
  checkout, so the reach of that cost is inferred from the agent briefs rather than observed.
- **Whether the batch autocommit can be scoped.** It runs on the host; this VM cannot edit host
  launchd agents.
- **Anything about points.** No measurement is implicated. A snapshot was regenerated because
  four *comment-only* edits landed under watched trees — `TestSnapshotCoversTheCurrentCode`
  cannot tell a comment from a constant, and a guard that fails for reasons unrelated to what it
  guards gets switched off, so the snapshot was regenerated rather than the guard loosened.
