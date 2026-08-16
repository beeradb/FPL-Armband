# Retiring the repo's work queue, and what three reviewers found in the doing

Covers `1787246..HEAD` on `retire-todo-md` — deleting `TODO.md` on the user's instruction so the
queue has one home, plus the review round that followed.

**Dispatched, not first-party.** `fpl-code-review` and `fpl-findings-audit` reviewed `780e8ea`;
a third agent audited the two out-of-repo queue files this change wrote. I wrote the change and
reviewed none of my own work except by running it. Two of the three reviewers independently found
the same two defects, and disagreed on a third — recorded below, because the disagreement is the
useful part.

**The reviews paid for themselves in one finding.** `.claude/agents/fpl-stats-review.md` was
quoting a *retracted* figure as its own worked example — in the reviewer whose job is deciding
whether evidence supports a claim, loaded verbatim on every dispatch, including the dispatches that
produced this record.

## What changed

**`780e8ea`** deletes `TODO.md` and rewrites every reference to it to stand alone — `CLAUDE.md`
(3 sites), 13 Go files, `docs/`, `stats/xpoints_common.py`, and the two tracked `.claude/` files.

**It is deleted rather than stubbed, and that is forced rather than chosen.** A stub would have to
say where the queue went; that is a location this repository may not name. There is no forwarding
address by construction, so the honest artefact is absence.

**`5a935cf`** applies the reviews. **`758efab`** re-keys the accuracy snapshot.

## The migration was lossy, and the checking method is the finding

**One open item existed only in `TODO.md`**: *"(PRIORITY) Re-run the ChipCredit bench channel with
the xPoints columns"*. Found by diffing all 58 open checkboxes against the destination
mechanically — not by reading either list — and migrated before the delete.

It had survived the 2026-08-14 migration because **two adjacent items look exactly like it and
neither is it**: one asks to wire the chip credit into the live agent, the other rules out
*building* `ChipCredit`. Neither is "re-measure the bench channel on a new instrument".

⚠️ **A migration that loses one item in 58 reads as complete.** The 2026-08-14 pass reported no
losses and had one. So: **a migration is verified by diffing the two sets, never by re-reading the
destination** — the destination always looks complete, that being what a destination is.

⚠️ **The count needs its own command.** `TODO.md` mixed 37 `- [ ]` with 21 numbered `N. [ ]`, so
the destination's own `^- \[ \]` recount returns 37 and reads as 21 lost items. Two counts of the
same set, two commands.

## What the reviewers found

### A live retracted figure, in the worst available place

`.claude/agents/fpl-stats-review.md` carried "the buy side is over-rated by ~0.53 pts/gw". That is
`retractedFigures[0]`; it does not reproduce at shipped config (−0.230 median, +0.079 mean). Marked
in place — the **mechanism** survives, the **size** does not.

⚠️ **The guard cannot see it, and this is a real scope gap rather than an oversight.**
`TestRetractedFiguresAreNotQuotedAsCurrent` does not scan `.claude/agents/*.md`. Eight tracked agent
definitions are loaded verbatim into a subagent on every dispatch — "read without choosing" in
exactly the sense that guard's own comment uses to justify its scope. The reviewer checked the rest
of the tree: one further match is already excluded by an `unless` list and one is the documented
bare-literal false positive, so widening would cost no noise. **Filed, not fixed** — widening a
guard is its own change and owes its own review.

### Four claims my own rewrites made false or uncheckable

Replacing a pointer with prose converts a signpost into an assertion. An assertion can be wrong,
and four were:

- **`simulate.go`** said three callers "still do not" set `NoDefCon`. All four do, since `d27c5c9`
  — and `PriorStatsFrom`'s doc comment twenty lines below already recorded the fix. One file
  asserting a defect open and closed at once.
- **`startsrepair.go`** says the harvest "must run **before**" `reconstructStarts`. ⚠️ **It runs
  after.** `c287862` moved it out of `fetch` into `repaired` the same day the paragraph was
  written. Verified directly rather than taken from the review: `reconstructStarts` at the end of
  `fetch`, `applyStartsRepair` in `repaired`, `Replaced` counting the overwrite. **The invariant is
  PRECEDENCE, not sequence**, which is exactly why the error survived a year of reading — recorded
  beats inferred either way, so the outcome never contradicted the sentence. Corrected here, in its
  test twin (which repeated it inside the file documenting the correction), and in `CLAUDE.md`.
- **`CLAUDE.md`'s "Only C1 of the audit is open"** was a bare index key into a document that has
  left the repository. Restated by what it means.
- **`inference_test.go`** said an item was "Queued" two lines above "✅ Empty since 2026-08-14".

### A floor on the retraction guard

Added, matching the one its sibling has carried all along. That guard has now lost **two** scope
entries — the evidence glob, then the queue — and `checkRetracted` returns silently on an
unreadable path, so shrinkage is invisible by construction rather than merely easy to miss. It has
no legitimate absence to declare, so a count is the right shape and fails in the right direction.

### Where two reviewers disagreed, and who was right

The findings audit passed the hook's new comment as "correctly kept and correctly marked". The code
review called it a false causal claim. **The code review is right**: I wrote that the excluded path
"no longer exists", implying it once bound. `git log --all -- docs/TODO.md` is empty — `docs/TODO.md`
has *never* existed here, and the scope is `:/docs`, which never reached the repo-root file either.
Inert from the day it was written, for a different reason than the one I recorded. Corrected.

Worth stating plainly: **a reviewer passing something is not evidence it is sound**, and two
reviewers agreeing is not either. What settled it was `git log`.

## What this cost, stated rather than buried

**`TestRetractedFiguresAreNotQuotedAsCurrent` no longer scans the queue at all.** Its own comment
argued the queue was the *harder and more load-bearing* case: a retracted figure survives there as
a **task premise**, setting what gets built next rather than merely misinforming a reader. That
coverage is now zero, and nothing in a checkout can reach the queue.

The append was **removed** rather than left pointing at a deleted path. `checkRetracted` is silent
on an unreadable file, so a dangling entry would have passed forever while checking nothing — the
failure this package exists to catch, arriving through the door it was built to watch.

⚠️ **The queue files now hold live figures with nothing checking them** — `+7.28`, `Holm ≈0.066`,
`30-60%` — in exactly the task-premise position the deleted guard was pointed at. None is currently
retracted. That is a fact about today, not a property of the arrangement.

## Owed, and deliberately not done here

- **Widen the retraction guard to `.claude/agents/*.md`.** The gap above. Verified to cost no noise.
- **`TestSnapshotCoversTheCurrentCode` mis-describes itself.** Its failure text says "adding a
  diagnostic does not trip this; changing what the model computes does". The second half is false:
  the exclusion is by filename suffix, so three **comment-only** rewrites forced a snapshot re-key
  on a change that computes nothing. The re-key was done; the false sentence was not touched.
- **A `branch:` dependency with no guard on either side.** The queue files record this deletion as
  contingent on this branch landing. No repo test can see them and nothing watches this branch, so
  that sentence is the entire mechanism.

## The merge with `main`, and why it owes no fresh review

`origin/main` moved to `5e7ce5f` while this branch ran, so it was merged in before landing. **The
merge owes no new review**, and the reason is checkable rather than asserted: the three incoming
commits touch `README.md`, `cmd/fplagent/main.go`, `cmd/fplagent/usageflags_test.go` and a review
record — and **none of those paths is in `ReviewWatchedPaths` or `SnapshotWatchedPaths`**. No
conflicts; `TODO.md` stayed deleted; the accuracy snapshot key is untouched.

⚠️ **One thing did bite, and it is a property of two branches rather than of either.**
`TestReviewCoversTheCurrentCode` failed after the merge against a digest that was **not this
record's** — `NewestKey` orders by `recorded_at`, not by directory name, and the incoming record was
stamped 14:07:05 against this one's 14:02:07. So the newest key on the merged tree was one recorded
**before** any of this branch's changes existed, and it reported five watched trees as moved.

Nothing was wrong with either branch. **Two records, each correct on its own branch, compose into a
tree whose newest key describes neither** — and the failure names the *other* record's digest, which
reads at first glance as this change having broken something it never touched. Re-keyed here, which
is the documented remedy. **The guard behaved correctly**; what is worth knowing is that a
concurrent branch can make your record stale by being five minutes later, with no content overlap at
all.

## Verification

`go build ./...`, `go vet ./...` and `go test ./... -count=1` pass, including the network-hitting
analysis and backtest suites. The two gate guards fail until their own commits land, by design —
the snapshot at `758efab`, this record last.

**Invariants before reviewers, per the skill's own first section.** The quantity this change must
not move is *behaviour*: it edits comments, prose and one test's file list. `go vet` plus the full
suite is that check, and the accuracy snapshot's 560 figure rows diffing clean against
`2026-08-15-e056821` is the stronger form of it — nothing the model computes moved.
