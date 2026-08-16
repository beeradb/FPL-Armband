# The two staleness guards are keyed on content, not on a commit

Covers the change on `guard-content-keying`, branched from `origin/main` at `0ecef97`. This is the
first record written under the new one-commit flow, so it reviews the mechanism it is itself the
first user of.

**Dispatched, not first-party**: `fpl-code-review` reviewed the staged diff. I wrote the change, so
I did not review it — the finding that mattered most was one I would not have found, because it was
invisible to `go build`, `go vet` and `go test ./...` all being clean.

## What changed and why

`TestReviewCoversTheCurrentCode` and `TestSnapshotCoversTheCurrentCode` identified their record by a
commit SHA in a directory name and diffed `sha..HEAD`. A commit SHA is a **history** pointer and both
guards ask a **content** question, so rebase — which preserves content and rewrites history — broke
the key while changing nothing either guard cares about.

The bill on this history: **13 of 905 commits** are pure re-key churn, 4 of them in the last 25.
`0ecef97` is a directory rename with **zero insertions and zero deletions**, the entire commit.
`5a70915` **deleted `figures.csv`, `constants.csv` and `snapshot.md` — 915 lines of banked figures**
— because the naming convention could not survive a rebase. A provenance guard that destroys
provenance to keep its filenames tidy has inverted its purpose, and that is the argument for the
change rather than the commit count.

Both guards now compare a sha256 over `<mode> <blob>\t<path>` for every non-test file under their
watch list, read from `key.csv` in the record's own directory. `merge-base --is-ancestor` is deleted
from both: it detected a dangling key, and a content digest cannot dangle.

Two consequences beyond the churn. **A review record is now one commit, not two** — `reviews/` is not
a watched path, so `fplagent reviewkey` digests the staged index and the record rides in the same
commit as the change it reviews. And **the directory name is free text**, because nothing parses it.

## Findings, and what was done about each

Ten findings. **Nine applied, one declined.** Ranked by how misleading the state was.

### 1. `fplagent reviewkey` could not be invoked at all — applied

The new command was absent from `commandsThatParseTheirOwnFlags`, so `rejectFlagsAfterCommand`
rejected every flag after the command name. `reviewkey -out x` errored with "flags must come before
the command"; `-out x reviewkey` errored with "flag provided but not defined". **There was no third
invocation.** Every documented usage — in the command's own doc comment and in `SKILL.md` — was the
one that errored.

⚠️ **`go build`, `go vet` and `go test ./...` were all clean**, because the exemption map had no
completeness check: `transfers_test.go` hand-listed three of its four entries and had already drifted
by one (`backfill`). This is the repository's signature failure — one quantity, two implementations —
sitting inside the guard against a silent no-op.

Applied: `reviewkey` added to the map; the test now **iterates the map** instead of a copy of it, and
a second test pins the other half of the pairing (every exempt name is a real command, every
self-parsing command is exempt).

### 2. The review gate was therefore a permanent skip — applied

Consequence of 1. No review record carried a key, `NewestKey` ignores unkeyed directories by design,
and the only producer of a key was the dead command. The gate reported PASS while covering nothing —
**the exact state the deleted ancestor check's comment warned about** ("a skip here is
indistinguishable from reviewed and clean"), reached through a different door. Fixed by 1, and this
record is the first key.

The reviewer independently reproduced the snapshot digest with a shell pipeline and confirmed
`9ec6c445…` byte for byte, so the snapshot side was genuinely passing on content, not by accident.
(That digest has since changed to `036250…` — see finding 5, which altered the algorithm.)

### 3. The ordering test could not fail — applied

`TestTheNewestKeyIsTheMostRecentlyRecordedOne` used two records with the wanted answer at the
lexically-first name. `os.ReadDir` returns sorted entries, so the answer was also the first one read:
the test passed for first-wins, for lexical-min, and **for no ordering at all**. Deleting the
`RecordedAt.After` comparison outright left the package green. It discriminated against exactly one
wrong implementation.

Ordering is the single property `NewestKey` exists for, and the deleted `newestSnapshot` blames a real
186-point mis-report on getting it wrong.

Applied, and **beyond what the reviewer proposed**: two records cannot separate first-wins from
lexical-max, whichever way round you put them. The fixture now uses **three**, with the newest in the
lexical middle, so first-wins, lexical-min, lexical-max and readdir-last each pick a different wrong
directory.

### 4. `DigestDiff` could report an empty list beside a fired gate — applied

It iterated the current watch list, and `WatchedDigest` populates an entry for every path it is given
— so **removing** an entry moves the composite while nothing shows as moved. The guard would fire and
print a blank bullet, which is the failure the per-path breakdown exists to prevent. Also reachable
via a trailing slash in a watch entry, which made a tree silently unreportable forever.

Applied: `DigestDiff` iterates the union and labels paths no longer on the list; both guards route
through `describeMoved`, which says so explicitly when the list is empty rather than printing nothing.

### 5. The digest was not a pure function of content — applied

`ls-tree` and `ls-files` C-quote non-ASCII paths **by default**, so the same file digests differently
depending on the reader's `core.quotePath`. Two machines would disagree on identical content and the
gate would fire permanently — the false-failure mode this code's own comments say gets guards deleted.
Latent today (no watched tree holds such a path) but `docs/` and `stats/` are watched and this
project's data is full of accented names.

Applied: `-z` on both commands, splitting on NUL. This changed every digest, so the migrated snapshot
key was regenerated.

### 6. Mode-only changes were invisible — applied

`chmod -x stats/regenerate_mde.sh` leaves the blob id identical, so the digest passed it while the old
`git diff --name-only` gate caught it. Two files in a watched tree are executable today and both are
scripts the documented workflow runs. Applied: the mode is now part of the digest line.

### 7. A watch entry matching nothing contributed silently zero — applied

`git ls-tree -- internal/analyis` exits 0 with no output, so a typo left a whole tree unguarded with
no error. Amplified by both writers passing `"."`: run from a subdirectory, every pathspec resolves to
nothing and `reviewkey` printed `digest e3b0c442…` — **the sha256 of the empty string, dressed as a
measurement, with a zero exit**.

Applied: `WatchedDigest` errors when any entry matches nothing, and both writers resolve the
repository root through a new shared `RepoRoot`. The test fixtures now seed every watch entry, driven
off the lists rather than a hand-written copy.

### 8. The snapshot guard downgraded a documented Fatal to a Skip — applied

The predecessor made a git failure at that point `t.Fatalf` with an explicit comment that it was not
environmental. I had made it `t.Skipf`. `repoRoot` has already established that git works, so what
remains is a watch-list typo or a damaged repository. Applied: both guards `Fatalf`, which also
restores the one duty of the deleted ancestor check that does not disappear with it.

### 9. `recorded_at` tie-breaking was meaningless — applied

`RFC3339` truncates sub-second precision and `After` is strict, so two records written in the same
second tied and the winner was whatever `os.ReadDir` yielded first. The tie was **moved** from day
granularity to second granularity, not closed. Applied: `RFC3339Nano`, plus a documented
lexical tie-break so an exact tie is deterministic rather than filesystem-dependent.

### 10. Two implementations of "which record is newest" — **DECLINED, and it is owed**

`runSnapshot` still picks its **diff baseline** with `FindPrevious` (`values.go:131`), which parses the
short sha out of a directory name, resolves it with `git show -s --format=%ct`, and falls back to
lexical max when none resolves. That is precisely the mechanism this change retires, and after the
rebases that motivated the change those shas dangle, every lookup fails, and the lexical fallback
picks the baseline. The recorded bug for that fallback is **"reported a detection threshold as having
moved 186 points when nothing had moved at all"**.

So the staleness question is now content-keyed while the moved-figures table is still commit-keyed.
The reviewer is right that this is the signature failure re-arriving inside the fix for the previous
instance.

**Declined for now, on migration reality rather than on principle.** `key.csv` exists on exactly one
snapshot; the other ~80 have none. Switching `FindPrevious` to `NewestKey` today would make it ignore
every historical snapshot and silently change which baseline the next snapshot diffs against — a
figure-moving change smuggled inside a plumbing fix, which is the shape `internal/config`'s watch-list
hole was added for. It wants its own change, after enough snapshots carry keys, and with the
before/after discipline the `sweep_inference.R` migration got.

⚠️ **Filed, not forgotten.** Without a follow-up this is a knowingly-left second implementation, which
is worse than the one the change removed, because this one is documented.

## What could not be checked on this harness

- **Nothing here is a measurement**, so none of the project's inference machinery applies. No cells, no
  standard errors, no thresholds. The claims are about mechanism and are checkable by reading and by
  the tests added.
- The **13-commit churn count** is this repository's history only, and the four-in-the-last-twenty-five
  concentration is a small sample. It motivates the change; it is not evidence about future rates.
- **`core.quotePath`** (finding 5) is confirmed as a mechanism and latent as a trigger — no watched
  tree currently holds a non-ASCII path.

## Gates

`go build ./...`, `go vet ./...`, `go test ./...` clean, including the full `internal/backtest` suite
(150s). Snapshot guard passes against the migrated key. Review gate passes against this record.

Mutation-tested rather than assumed, on the two checks most at risk of being vacuous: removing the
`p+"/"` separator fails `TestAWatchedPathDoesNotClaimItsPrefixSibling`; the reviewer separately
confirmed the ordering test was vacuous before finding 3 rebuilt its fixture.

**No measured figure moves.** `SnapshotWatchedPaths` is untouched by this change, so the snapshot
digest at `9e743cf` equals the digest at this tip — verified during migration, printed as `equal:
true`.

---

## Follow-up, same day: an audit of the queue notes, and what it found in the repo

Ran after the merge, over the two task-queue notes this change produced. Three of its findings land
in **this repository** rather than in the queue, and all three are applied here.

### A wikilink leak that this change shipped to main — applied, and now guarded

`.claude/skills/review-gate/SKILL.md:66` carried `[[old-nulls-are-unresolved-not-refuted]]`, a
reference to a document that is not in this repository and cannot be resolved by anyone reading this
checkout. A docs review had already flagged the class, and a later branch had already written a guard
for it — **on a branch that was never merged**. The commit above then edited that same file, 33
lines, and shipped the reference to `main` untouched, because nothing in the tree checked.

Replaced with the standing rule in `CLAUDE.md` that carries the same point ("A null is a tie, not the
refutation of one"), which a reader can actually follow.

⚠️ **The audit's own brief asserted a guard named `TestNoTrackedMarkdownCitesAWikilink` exists and
instructed "run the test; do not re-invent the grep". It does not exist** — `go test -run` on a name
that matches nothing exits 0, so following that instruction returns a green result that checks
nothing. That is how the leak survived. **The test now exists**
(`internal/snapshot/wikilink_test.go`), scanning every tracked `.md` including `.claude/`.

Its first run **caught a false positive and the design was wrong, not the file**: it flagged
`blocks[[b]]` — R list indexing, in backticks, in `reviews/2026-08-14-dbe8251/review.md:35`. The
first version reasoned that code blocks should not be exempt because "a fenced example teaches the
syntax as acceptable". Backwards: `[[…]]` is ordinary syntax in a language this project writes, so
scanning code manufactures false positives rather than catching more leaks. Code spans and fenced
blocks are now blanked, and a second rule requires the brackets not to follow an identifier. Verified
by reintroducing the real leak and watching it fail, then removing it again.

### "Thirteen of 905 commits" paired two populations — applied

The numerator was counted on the linear history; the denominator was `git rev-list --count --all`,
which reaches ~30 refs including abandoned branches and `backup/pre-rebase-2026-08-14` — a ref
created *for* the very rebases this change is about. Verified: **807** on the linear history against
**906** across all refs.

So the honest ratio is **13 of 807, 1.6%** — the original **understated its own case**. Corrected in
`internal/snapshot/watched.go` with the counting rule stated, since the next person will otherwise
reach for `--all` again. The definition is now explicit too: eleven commits with zero insertions and
zero deletions, plus two deletion-only re-keys.

⚠️ **The figure is also in this commit's parent's commit message, which is on `main` and cannot be
corrected.** A reader who finds "905" there and "807" in the code should trust the code.

⚠️ **`5a70915` was not the only one.** `9191772` performed the identical 915-line deletion against a
different snapshot directory, so the destruction happened **at least twice**. Recorded in
`watched.go`.

### `values.go:131` named the wrong function — fixed in the queue note, recorded here

The D2 write-up attributed the diff-baseline behaviour to `FindPrevious` at `values.go:131`.
`FindPrevious` is at **:95**; :131 is `newestByCommitDate`. The sentence spans three sites — `:95`,
the lexical fallback at `:116-117`, and the commit resolution at `:131`. No repository file carries
the wrong reference, so nothing is applied here; it is fixed in the queue note.

### What the audit proposed and I declined

- **Move the D1 closure from `tasks/` to `findings/`.** The A1/A5 precedent is real and the argument
  is sound, but the C-section already exists in both files, so following the pattern would duplicate
  a second time — against the standing rule that a mirrored copy drifts. Linked instead of copied.
- **Two stale citations into `reviewgate_test.go`** (`:100-117`, `:105-110`) were exactly right at
  `0ecef97` and dangle at `dacf549`, because this change deleted the block they pointed at. Marked in
  place in the queue note rather than silently repointed: the original citation was correct when
  written, and that is worth preserving over a tidy edit.

### Not applied, and it is a real open question

`tasks-index.md` states 58 open items while its by-area table sums to 63. **Pre-existing** — the
prior version said 57 against a table summing to 62 — but it is one quantity with two implementations
in a file I rewrote whole. I have not reconciled it because I do not know which number is wrong, and
inventing one is how a wrong count becomes authoritative. Flagged, not fixed.
