# Keying the snapshot baseline on content instead of on a commit

Branch `snapshot-baseline-content-keyed`, cut from `origin/main` at `76a4f3f`. One commit.

## What was reviewed

`snapshot.FindPrevious` (`internal/snapshot/values.go`) chooses the baseline that a new accuracy
snapshot diffs its figures against. It parsed a short sha out of each snapshot directory's name
and resolved it to a commit date with `git show -s --format=%ct`. A sibling change removed its
lexical-max fallback and left the keying alone, recording two blockers in the code. Both are
closed here and the keying is switched.

Rebase preserves content while rewriting history, so the commit key broke on exactly the
operation that changes nothing this function cares about — and it broke *asymmetrically*: an
unresolvable candidate was skipped while others resolved, and a rebase orphans the **newest**
shas, which is precisely the baseline that matters. The failure is invisible downstream, because
only *whether* a baseline existed is banked, never which one.

Four parts:

1. **Backfill.** The 51 candidates that carried no `key.csv` were given one, computed from that
   directory's own commit.
2. **A reconstruction marker.** `KeyReconstructed` / `Key.Reconstructed`, written only when
   non-empty.
3. **The ranking moves into a shared `newestKeyed`**, called by `NewestKey` and by
   `FindPrevious`, which select candidates on different rules. ⚠️ This was specified as "give
   `NewestKey` an `exclude`" and that is **not** what shipped — see finding 1.
4. **`FindPrevious` ranks on the key.** `newestByCommitDate` and `snapshotCommit` are deleted.

Files: `internal/snapshot/values.go`, `internal/snapshot/watched.go`, and the three test files;
plus 51 added `stats/snapshots/*/key.csv`.

`internal/snapshot` and `stats` are on `ReviewWatchedPaths` but neither is on
`SnapshotWatchedPaths`, so this owes a review record and does **not** owe a fresh accuracy
snapshot.

## The recount, done first and from what the code considers

Every number in the brief that motivated this work was re-derived rather than reused, because the
denominator is easy to get wrong: `FindPrevious` filters candidates on holding a `figures.csv`,
so the 19 directories under `stats/snapshots/` that hold only banked cells are **not** candidates
and owe no key. Counted at `76a4f3f`:

| quantity | count |
|---|---|
| directories under `stats/snapshots/` | 78 |
| **candidates** — those holding `figures.csv` | **59** |
| candidates carrying a `key.csv` before this change | 8 |
| candidates whose recorded sha resolves in this checkout | **59 of 59** |
| candidates that could not be backfilled | **0** |

The premise the backfill rests on holds: nothing dangles today. ⚠️ **It is perishable and this
is the only reason the work was possible now.** Each rebase orphans more of those commits, and a
candidate whose commit has gone cannot be given a key after the fact at all — the digest is
computed *from the tree at that commit*.

**One candidate demonstrates both halves of that, and it is checkable.** `13cded0` is **not an
ancestor of `origin/main`** (`git merge-base --is-ancestor 13cded0 origin/main` exits 1); it
resolves only because the object still sits in this checkout's store. `27740ba`, for contrast,
exits 0. So:

- its key could be reconstructed **here and nowhere else** — a fresh clone never could have;
- and under the old rule a fresh clone **silently skipped it** in the ranking, since `git show`
  would fail. Under the new rule the key is committed, so every clone orders the full series.

That is the gain and the deadline in one directory.

Note that the 19 non-candidates are also the directories with free-text names
(`2026-08-13-benchshape`, `2026-08-14-blend-datastate`), whose last dash-separated segment is not
a sha at all. None of them was ever orderable and none was ever ranked.

## No figure moves, and the argument is structural rather than a spot check

Nothing under `figures.csv`, `constants.csv`, `snapshot.md`, `FINDINGS.md` or any existing
`key.csv` is modified: the diff is 6 Go files plus 51 **added** files. `key.csv` is read by
exactly three callers — the two staleness guards and now `FindPrevious` — and none of them writes
a figure.

The one path by which a backfilled key *could* have changed a recorded quantity is the staleness
guard, which reads only the **newest** key and compares its digest against HEAD's. A backfilled
key that ranked newest would change what that guard compares. None can: the newest key in the
series is `2026-08-15-ecebfcc` at `2026-08-16T00:01:20Z`, written live, and every backfilled
`recorded_at` is a commit date at or before `2026-08-15T03:07:35Z`.

## Baseline neutrality, over the whole series

The deliverable. `FindPrevious` picks the maximum of the series excluding one directory, so the
comparison is: for **every** snapshot in the series, taken in turn as the one being written, does
the baseline move?

Driven through the real `FindPrevious` — compiled once against the commit-date rule and once
against the content key, on the real `stats/snapshots` — over all 59 candidates plus a
hypothetical new directory:

**60 of 60 identical. Nothing moved.**

That table alone is weak evidence, and saying so is part of the result: because the rule returns
a global maximum, all 60 answers are either `2026-08-15-ecebfcc` or (for `ecebfcc` itself)
`2026-08-15-0485f4e`, so it only ever exercises the top two of a 59-deep ordering. So the **full
ordering** was extracted too, from the same real code, by copying the series to a scratch root and
repeatedly asking for the newest and removing it — 59 ranks, under both rules.

### One rank moves, and it is worth stating rather than rounding away

| rank (0 = newest) | commit-date rule | content key |
|---|---|---|
| 7 | `2026-08-15-9e743cf` | `2026-08-14-862c86a` |
| 8 | `2026-08-14-862c86a` | `2026-08-14-5a70915` |
| 9 | `2026-08-14-5a70915` | `2026-08-14-84a0945` |
| 10 | `2026-08-14-84a0945` | `2026-08-15-9e743cf` |

`2026-08-15-9e743cf` moves from rank 7 to rank 10 — 8th-newest to 11th-newest — displacing three
directories up by one each. Ranks 0-6 and 11-58 are identical, so the movement never reaches a
baseline under any exclusion, which is why the 60-row table above shows nothing.

**Why**, and it is not a defect in this change. That directory's key is the only one of the eight
pre-existing keys that was seeded by hand, in `dacf549`, the commit that introduced the key
mechanism. It carries `recorded_at 2026-08-15T00:38:00Z` against a commit date of
`2026-08-15T04:42:28Z` — **four hours and four minutes before the commit it names**. A live
record cannot do that: the snapshot runs at a checkout of that commit, so it is written at or
after it. The other seven were written 7 to 388 seconds *after* their commits.

It is left byte-identical. It is somebody's attestation, no figure depends on it, and the
arithmetic is recorded here instead. ⚠️ **Follow-up for the operator to decide:** that key is a
reconstruction with no marker, which is the state the `reconstructed` row added here exists to
prevent. Whether to annotate it is a call about someone else's record, not a plumbing fix.

## What is reconstructed, and how a reader can tell

The two halves of a key reconstruct differently, and conflating them would be the dishonest
version of this change.

**`watched_digest` reconstructs exactly.** It is a pure function of the git tree at the record's
own commit, and `runSnapshot` digests that same commit (`HEAD`) when it writes a key live. This
is *verified*, not argued: recomputing each of the 8 pre-existing digests from its own sha
reproduced the recorded string, **8 of 8**, including `9e743cf`.

**`recorded_at` does not.** It is wall-clock time at the moment of writing and nothing in the tree
remembers it. The backfill substitutes the commit's own date, which is a lower bound — by the 7-to-388
seconds measured on the live keys. It is chosen because it makes the content-keyed ordering
reproduce the commit-date ordering it replaces, not because it is when anything happened.

So a backfilled key carries a `reconstructed` row naming the substitution:

```
reconstructed,"backfilled 2026-08-15 when key.csv became the baseline key; watched_digest is
exact, recomputed from commit 27740ba; recorded_at is that commit's date standing in for the
unrecorded write time"
```

Live keys have no such row, and `TestAReconstructedKeySaysSo` asserts **both** halves — a marker
written unconditionally would appear on every key and distinguish nothing, which is the same
defect wearing the opposite costume. The row is deliberately a sentence rather than a boolean: a
bare `true` would record that something was substituted without recording what, and the
substitution is the whole content of the caveat. No guard reads it, and it must not become an
ordering input.

The backfill itself was a throwaway program, not a committed subcommand: it can only ever run
once, and only while the commits resolve. A permanent command for it would be dead code that
looks live.

## Regression tests

`internal/snapshot` is provenance machinery, where the recorded failure modes are a silently
wrong answer shaped like a right one, and one quantity with two implementations.

| test | what it stops |
|---|---|
| `TestEverySnapshotCandidateCarriesAKey` | the coverage blocker returning. A candidate with no key is *skipped*, so a snapshot silently diffs against an older baseline than its predecessor. Candidate-level, not directory-level: banked-cell directories owe no key |
| `TestFindPreviousPicksByRecordedTimeNotByNameOrModificationTime` | name order, mtime order, first-wins and lexical-max, with three candidates and the newest in the lexical middle. **Uses no git repository at all** — the three names carry `aaaaaaa`/`mmmmmmm`/`zzzzzzz`, which are not even hex, so under the old rule the whole series was unorderable. That is the property the change bought |
| `TestAKeyDescribesTheCommitItNames` | a key whose digest was computed from the wrong tree. Presence is not correctness: a backfill that digested `HEAD` fifty-one times would be well-formed, pass every other test, and silently reorder the series. Verifies against the paths **the key itself records**, so a later watch-list edit is not a false alarm; skips any commit this checkout cannot reach |
| `TestAReconstructedKeySaysSo` | a reconstructed attestation reading as a contemporaneous one, in both directions |
| `TestAnUnkeyedCandidateCannotWinTheRanking` | an unkeyed directory winning by a lexical fallback |
| `TestTheThreeBaselineOutcomesStayDistinct` (updated) | conflating "no baseline", "one candidate, no ordering needed" and "several candidates, none identifiable" |

Mutation-checked rather than assumed: deleting the `RecordedAt.After` comparison from
`newestKeyed` fails three tests; deleting one `key.csv` from the real series fails the coverage
guard and names the directory.

### The two-implementations risk, addressed by construction

`FindPrevious` and the two guards select candidates on **different** rules — holding a
`figures.csv` against being any directory — but rank them the same way. So the selection is
per-caller and the ranking is one function, `newestKeyed`. Having `FindPrevious` filter and then
rank for itself would have re-committed the divergence these two guards have already committed
once, on the `merge-base --is-ancestor` check.

## What is deliberately *not* fixed

A **partial** absence of keys. A candidate with no key is skipped while others have one, so it
loses to an older keyed candidate — the same shape as the dangling-commit skip it replaces, one
property better: a key cannot be orphaned by a rebase, so nothing that happens to *history* can
put a candidate into that state.

A run still can, and the first draft of this record claimed otherwise on both sides of one page.
`runSnapshot` writes `figures.csv` (`cmd/fplagent/snapshot.go:282`) and `key.csv` (`:302`) with
three error returns between them, one of which — `WatchedDigest` — is deliberately fatal when a
watched path matches nothing. The write **order** is what makes that fail safe rather than
silently: the half-written directory carries no key, is therefore never *ranked* as anyone's
baseline, and trips the coverage test loudly once committed. Writing the key first would invert
both properties.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-code-review** | yes | provenance machinery carrying a no-figure-moves claim, plus a bulk write into banked directories. The triage row is "a refactor asserting byte-identical output" |
| fpl-stats-review | no | no measurement was produced and no figure, constant or inference is claimed to move. The change alters which baseline a *future* snapshot diffs against |
| fpl-docs-review | no | `docs/`, `README.md` and `CLAUDE.md` are untouched, and none of them describes baseline selection |
| fpl-findings-audit | no | `CLAUDE.md` is untouched; no recorded verdict rests on baseline selection |
| fpl-security-review | no | no credential, network, cache or agent surface |
| fpl-season-maintenance, fpl-run-review | no | not applicable |

Self-review does not count, which is why the neutrality proof is a before/after run of the real
code over the real series rather than an argument about the diff.

`fpl-code-review` reproduced the whole result independently — 59 candidates, 59/59 resolving,
**all 59** digests and all 236 `path.*` rows recomputing from their own commits (a wider check
than the 8 I had run), 60/60 baseline neutrality, and the single `9e743cf` displacement. It also
confirmed the ordering suite discriminates against rank-by-digest, rank-ascending, ignore-exclude,
lexical-max, mtime and first-wins.

## Findings

Five, all confirmed and all applied. None was in the "measures nothing" class; four were comments
asserting mechanisms that do not exist, which in this package is the failure that matters, because
a provenance comment is what the next person reasons from.

### Applied

1. **The `exclude` parameter had no caller, and its comment described a design the code does not
   use.** The brief specified "give `NewestKey` an `exclude`", and I implemented it — then
   `FindPrevious` did not use it, because it *cannot*: it ranks only directories holding a
   `figures.csv` while `NewestKey` ranks every directory. It shares the ranking (`newestKeyed`)
   and applies its own exclusion when selecting its own candidates. So the parameter had zero
   non-test callers, its own test was the only thing exercising it, and it put the exclusion
   predicate in two places — the exact shape of the defect its comment claimed to avoid.

   **The parked blocker dissolved rather than being fixed.** The self-diff failure is real and is
   prevented, by `FindPrevious`'s own filter; "`NewestKey` takes no `exclude`" was a true
   observation that did not imply the repair it looked like it implied. `NewestKey(root)` is back
   to one argument, the test is deleted, and the surviving coverage is the
   exclude-the-newest-of-three assertion inside
   `TestFindPreviousPicksByRecordedTimeNotByNameOrModificationTime`, which the reviewer confirms
   also catches a short-circuit widened to `len(names) <= 2`. **This is a deliberate deviation
   from the brief and is flagged as one.**

2. **"the eight live keys were written 7 to 388 seconds after the commit they name" was wrong for
   two of the eight**, and one of them refuted the "lower bound" framing the sentence rested on.
   Recomputed: `+7, +10, +100, +179, +309, +388, 0` and **`−14,668`**. `ecebfcc` is exactly 0, and
   `9e743cf` precedes its own commit by four hours. The comment also counted `9e743cf` among the
   live keys while `values.go` argued twenty lines away that it is hand-seeded and that a live
   record cannot do that — two statements that could not both stand. Now: seven of eight, 0 to 388
   seconds, with `9e743cf` named as the exception rather than averaged into the range.

3. **`sort.Strings(names)` was inert, and its comment gave a reason that cannot be true.** It
   claimed to fix which name wins a tie in `newestKeyed` — but that tie-break is `n > dir`, a
   maximum, which is invariant to the order names arrive in. The sort was load-bearing under the
   deleted commit-date rule, whose strict comparison kept the *first* name seen. Removed; the
   re-run confirms no answer moved, which is the finding restated as a measurement. An inert line
   with a live-sounding justification is worse than no line, because the next reader budgets for
   it.

4. **"which `runSnapshot` cannot do, since it writes both" is false.** Three error returns sit
   between the two writes, one of them deliberately fatal. See "What is deliberately not fixed"
   above, which now carries the honest version in both places rather than one.

5. **Nothing re-checked that a key describes the commit it names, and the check decays.** The
   coverage test sees presence and parseability only. A backfill that digested `HEAD` fifty-one
   times would have been well-formed, passed the whole package, and silently reordered the series
   — the same class of silent wrong answer this change exists to remove, arriving through the
   repair. `TestAKeyDescribesTheCommitItNames` closes it: 59 verified, 0 unreachable today, and it
   fires naming the directory when a digest is corrupted. ⚠️ It is written to **decay gracefully**
   — unreachable commits are counted and skipped, because `13cded0` already cannot be verified
   from a fresh clone and more will follow. The verifiable fraction only falls, which is an
   argument for adding it now rather than for adding it loosely.

### Not applied

None. The reviewer raised nothing that survived as a disagreement.


## Appended after merging `origin/main` — the resident-record compaction

`origin/main` moved again while this branch was in flight, landing a compaction of the resident
record. **That work carries its own record and is not re-reviewed here.** The merge was clean — no
conflicts — and this section covers only what it means for this change.

**Nothing in the compaction touches this change's subject.** It edited `CLAUDE.md`, two reference
documents and one test's prose; this branch edits the snapshot baseline's keying and adds 51
reconstructed `key.csv` files. The two do not overlap in any file.

**The size budget was left at the value this line of work raised it to**, and the compaction now
sits well under it. That is deliberately not lowered here: the constant's own rule is *raise it and
name the claim*, and it says nothing about reclaiming headroom — lowering it as a side effect of an
unrelated merge would be a decision taken by whoever merged last rather than by whoever owns the
record.

**The backfill is unaffected by the merge.** Its correctness rests on the commits the keys name
still resolving, which is a property of the object store rather than of `main`'s content, and it was
verified after the merge as well as before: build, vet and the full suite are green at the merge
commit.
