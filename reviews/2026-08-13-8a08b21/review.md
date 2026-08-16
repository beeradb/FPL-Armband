# The page rebuild, and a gate that was already stale before this merge

Covers `9317f70..8a08b21` on `worktree-prior-half-life-on-repaired-xgc` — the HTML
generator work — plus the merge of `origin/main`.

## ⚠️ What this record does NOT cover

**Four of main's commits changed watched files after main's own review record and were
not reviewed by me.** `reviews/2026-08-13-51161e5/review.md` is named for the oldest of
main's five new commits; the four after it — `0455de2`, `e25c841`, `4936e5b`,
`f62fa2c` — touched `CLAUDE.md`, `docs/notes/archive-and-data.md`,
`docs/notes/harness-and-inference.md`, `docs/rank-objective-handoff.md` and the
`2026-08-13-oracle-starts` cells and logs.

**`TestReviewCoversTheCurrentCode` was therefore already failing on main before this
branch merged it**, and my side touched none of those files — verified with
`git diff --name-only 9317f70 HEAD~1`. This record exists so the merged branch has one,
not to imply I audited the oracle re-pricing. Whoever owns that work owns its review.

The distinction matters because the guard's whole purpose is that an unreviewed change
cannot hide behind a record covering something else.

## The one real risk in the merge, checked

Main's tip re-prices the oracles **on recorded starts** — the harvest this branch added.
Two branches working one seam is where a semantic conflict lives, and `git merge-tree`
finding no textual conflict says nothing about that.

Checked rather than assumed: the merge is conflict-free, and **the full suite is green
on the merged tree**, `internal/backtest` included. Build and vet clean.

## What was reviewed here

No reviewer agent was dispatched. This is presentation code — it renders numbers the
model already computed and cannot change one — so the triage table's rows for scoring,
harness and config do not apply. Recorded as a decision rather than left implicit.

**The pitch was removed.** It was inherited rather than chosen, and I had defended
keeping it with an argument I would have made for any layout I found in place. It shows
a formation well and does everything else these pages need badly: comparing a player
with his own backup, reading a column of scores, scanning ten weeks for a change. The
squad is now a position-ordered table with the bench in the same table under its own
header.

**The palette was replaced and audited.** All three theme states carry the full token
set, and no rule anywhere holds a literal colour — checked by stripping the `:root`
blocks and grepping the remainder. The previous palette had shipped with only the
`prefers-color-scheme` block, so a reader who explicitly chose dark on a light OS got
the light palette.

**The page carries the judgement, read from a file.** `internal/analysis` is pure
computation and this binary does not judge; a page that composed its own verdict would
be a second opinion competing with the one in the report. An absent file renders
nothing, which is honest rather than empty.

## Defects found by the user in the output, all fixed

These are recorded because all three were visible on the page and invisible to my tests,
which is the pattern worth remembering rather than the individual bugs.

| defect | why the tests missed it |
|---|---|
| Every replay card read "no fixture" — `Opponent` was never populated, and that branch means *blank gameweek*, not "unknown" | The tests checked labels, moves and self-containment. None looked at a card |
| The bench sat below ten gameweeks of tabs, though it is part of the same fifteen | No test asserts reading order, and none reasonably could |
| The squad page carried no deadline, chip plan or override count | The page was never asked what it was for |

The opponent test is deliberately two-sided: asserting only that opponents appear would
pass on a page that also cried blank everywhere, and asserting only that the blank
marker is absent would pass on a page that had stopped reporting real blanks.

## Declined

**`FPL_ANALYSIS_MD` and `FPL_REPLAY_*` are in the fingerprint skip list, not
`envSwitches`.** All three are about the page rather than the model: prose about a
decision and an output path cannot move a measured number. The completeness guard
caught each one and was right to.

**The brief is not embedded in the squad page.** It is a source document, not a
conclusion; the page carries the verdict instead. Stated because it was asked for, then
narrowed, and the narrowing is the better answer.

## What could not be checked here

Whether main's oracle re-pricing is correct. It builds on this branch's starts harvest,
the suite is green against it, and that is a compatibility check rather than a
correctness one.
