# Removing the lexical-max fallback from the snapshot baseline lookup

Branch `snapshot-baseline-fallback`, cut from `origin/main` at `63eff6f`. One commit.

## What was reviewed

`snapshot.FindPrevious` (`internal/snapshot/values.go`) chooses the baseline that a new
accuracy snapshot diffs its figures against. It parses a short sha out of each snapshot
directory name and resolves it to a commit date with `git show -s --format=%ct`. When **no**
candidate's sha resolved it silently fell back to picking the lexically-largest directory
name — the rule whose recorded failure is "reported a detection threshold as having moved 186
points when nothing had moved at all". The fallback is removed.

The staleness guards were moved off commit keying to content keying earlier; this function was
the last commit-keyed copy. **Only the fallback is addressed here. The keying is not**, and the
reason is recorded in the code: at `63eff6f` only 8 of the 59 candidate directories carry a
`key.csv`, so switching to `NewestKey` today would ignore the other 51 and silently change which
baseline the next snapshot diffs against — a moved figure smuggled inside a plumbing fix.

Files: `internal/snapshot/values.go`, `internal/snapshot/render_test.go`,
`internal/snapshot/watched_test.go`, `cmd/fplagent/snapshot.go`.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-code-review** | yes | closest triage row is "a refactor asserting byte-identical output"; the change carries a no-figure-moves claim that needed checking against the control flow |
| **fpl-docs-review** | yes | the doc comment was substantially rewritten and makes counted, checkable claims |
| fpl-stats-review | no | nothing statistical is claimed and no measurement was produced. The change alters which baseline a *future* snapshot diffs against, not any figure, constant or inference |
| fpl-findings-audit | no | `CLAUDE.md` and `docs/` are untouched, and neither describes baseline selection (verified by both reviewers independently) |
| fpl-security-review | no | no credential, network, cache or agent surface |
| fpl-season-maintenance, fpl-run-review | no | not applicable |

`internal/snapshot` is on `ReviewWatchedPaths` but **not** `SnapshotWatchedPaths`, so this owes a
review record and does not owe a fresh accuracy snapshot.

## The invariant, checked before the reviewers

The skill's rule is that an invariant beats a reviewer. The quantity that must not move is the
baseline the real series picks. A probe calling `FindPrevious("../../stats/snapshots", "")`
returned `PICK="2026-08-15-ecebfcc" ERR=<nil> NKEYS=559` at `origin/main`, and returns the same
after the change. The probe was temporary and is not committed; it is reproducible in four lines.

The structural argument is stronger than the probe and does not depend on it: the old and new
code differ only where `newestByCommitDate` returns `""`, and at `63eff6f` **all 59 candidate
directories resolve** — so the fallback was already dormant. Removing dormant code cannot move a
figure.

## Findings

Ranked by how misleading the state was. Both reviewers independently caught #1; I had caught it
myself before either reported, which is corroboration rather than a third finding.

### Applied

1. **The `4d61058` tie example was refuted by the code eight lines above it.** The comment cited
   `stats/snapshots/2026-08-12-4d61058` and `2026-08-13-4d61058` as a live instance of two
   directories naming one commit. Neither carries a `figures.csv`, so `values.go` filters both
   out before the candidate list is built and neither can ever reach the ordering. Recounted:
   among the 59 candidates there are **no duplicate shas and no duplicate commit seconds**, so no
   tie is reachable at all. Rewritten to state the tie rule as determinism rather than as a fix
   for an observed collision. This was the one citation in the change that looked checkable, and
   it was wrong — exactly the "check which *file* a number came from" failure.

2. **A single unresolvable candidate now errored where the old code was unambiguously right** —
   a genuine regression, and the only place the "changes behaviour only where it is already
   wrong" claim was false. With one candidate there is no ordering to get wrong; it is the
   predecessor by construction. The old code returned it, the new code refused. It bites any
   snapshot root outside a git checkout, where no sha can ever resolve. Fixed by picking directly
   when `len(names) == 1` and only consulting git above that. The recorded 186-point failure
   needed three same-day candidates. The outcome table in the doc comment is now three states,
   not two.

3. **The comment said `NewestKey` orders by the content key. It orders by a recorded
   `recorded_at` timestamp**; the digest answers "has it moved" and has no order at all. Since
   `FindPrevious`'s whole job is ordering, a reader following that instruction would look for
   order in a digest and not find it. Corrected, and "the staleness guard" corrected to both
   guards.

4. **The migration recipe named coverage as the only blocker, and it is not.** `NewestKey` takes
   no `exclude` parameter and `FindPrevious`'s does real work: a same-day re-run at the same
   commit reuses the directory name, and that directory already holds a key by the time anything
   reads it — so a naive switch would diff a snapshot against itself and report that nothing
   moved. Recorded as a second blocker.

5. **The error's recovery instruction did not work as written.** It said `-previous <dir>` while
   the flag is consumed as `filepath.Join(*prevDir, ValuesFile)` — a path relative to cwd, not a
   name relative to `-out`. An operator pasting the bare directory name would have got a
   file-not-found. The message now prints the full path form and mentions `-no-r`, which reuses
   the inference the aborted run already wrote. This is the one sentence the error exists to
   deliver.

6. **The error's last sentence contradicted itself** ("If this snapshot really is starting a
   fresh series, the series is not empty"). Rewritten.

7. **The "not fixed here" section argued against a strawman and gave no detection route.** It
   presented error-versus-silence as exhaustive when a warning is the middle option, and never
   said how the residual presents. Both are now stated, and the argument is measured rather than
   plausible: **58 of the 59 candidates are ancestors of `origin/main`; `2026-08-10-13cded0` is
   not**, resolving only because the object still sits in this checkout's store. So "error on any
   dangling candidate" would refuse to run in every fresh clone today. Also corrected "silently"
   — the chosen baseline *is* printed and stamped in the markdown; what it is not is
   machine-checkable, because only whether a baseline existed is banked, never which one.

8. **The recount instruction had no generator**, which is the failure it was warning about. The
   two `ls | wc -l` commands are now in the comment. Independently recounted by the record
   reviewer: 59, 8, and all 8 keyed directories also carry figures.

9. **Test comment overclaimed its own coverage.** It credited an `n == "<lexical max>"` assertion
   that both reviewers showed is unreachable — the `err == nil` fatal above it already covers the
   only surviving case. The dead branch is removed and the comment now credits the check that
   actually does the work, whose failure message interpolates the picked directory and so names
   the wrong answer in the output.

10. **"All three orderings disagree" mis-described the fixture.** Name order and mtime order
    agree with each other and both select the older commit; commit order stands alone. That is
    the *stronger* fixture and the comment undersold it.

11. **`olderDir` / `newerDir` inverted their own contents** — the variable tracked the commit's
    age while the string tracked the opposite date, so the fixture assertion read as a typo.
    Renamed to `olderCommitDir` / `newerCommitDir`.

12. **`commitAt` rebuilt the `exec.Command` plumbing** instead of going through `r.git`, so later
    hardening of the shared helper would miss it. Both now delegate to one `gitEnv`.

13. **`cmd/fplagent/snapshot.go`'s help text was falsified by this change.** It promised "a
    missing input is reported in the snapshot rather than being fatal". The baseline refusal is
    now a genuine exception and is stated as one. This was outside the reviewed diff; the change
    is what falsified it, so it is repaired here rather than routed onward.

### Declined

- **Render the snapshot with `Previous = nil` and a stamped problem, instead of failing.**
  (fpl-code-review.) It would avoid discarding the R inference at the point of maximum sunk cost.
  Declined: it reintroduces the shape the change exists to remove — a written snapshot whose
  moved-figures table is absent for a reason the artefact has to explain in prose, sitting next
  to a "this is the first snapshot" block that means something different. The sunk-cost objection
  is answered directly by naming `-no-r` in the error, which makes the corrected re-run cheap.
  Recorded because it is a reasonable design and will be proposed again.

- **Assert the literal `"186"` in the error message.** The two reviewers split: the code reviewer
  would keep it as a cheap deliberate pin, the record reviewer called it a drift hazard because
  this project corrects figures in place and a test pinning the number makes the error harder to
  correct. Took the record reviewer's side; the assertion is now structural (`-previous`,
  `cannot be identified`). The number stays in the error itself, where an operator can read it.

- **Delete `sort.Strings(names)` as inert.** `os.ReadDir` is documented to return entries sorted
  by filename, so the line changes nothing observable. Kept, with the comment corrected to say
  so: it makes the tie rule a property of this function rather than one inherited from another
  package's documentation. Removing it is a behaviour-adjacent edit to a pre-existing line and
  was not in scope.

- **Harmonise the tie direction with `NewestKey`** (this breaks ties toward the smaller name,
  `NewestKey` toward the larger). Provably a no-op today, since no tie exists on either sha or
  commit second — but it is a line the keying migration will delete, and changing an untested
  ordering rule for tidiness is how a baseline moves by accident. The real risk is that whoever
  lands that migration ports the tie rule and calls it "no change" without checking; that is
  named in the code comment.

- **Add `-c commit.gpgsign=false` to the test git helper.** A developer with global signing
  enabled would see these fixtures fail. Pre-existing, affects the existing `commitAll` equally,
  and out of scope. `gitEnv` now gives it one place to land.

- **Fix `prevName = *prevDir` stamping a path where `FindPrevious` yields a bare name.**
  Pre-existing inconsistency in the rendered markdown, outside the diff. Noted because this
  change promotes `-previous` from a convenience to the required recovery, so it will land in the
  banked record more often.

## What could not be checked here

- **The 186-point incident itself is inherited, not re-derived.** The three commits named in the
  original comment exist, share the date 2026-08-09, and stand in the stated
  lexical-versus-chronological relationship — so the mechanism is verified. The figure is not: no
  `2026-08-09-*` directory survives in `stats/snapshots` (the series begins 2026-08-10), so the
  snapshot pair that produced it is gone from the tree. That is an argument for keeping exactly
  one full statement of it and referring to it elsewhere, which is how the four mentions are now
  arranged.
- **"A rebase orphans the newest shas"** is a mechanism claim, labelled as such in the comment
  and not demonstrated by anything in the change.
- **A rebase also rewrites `%ct`**, so a rebased old snapshot could become "newest" by this rule
  while being a stale record — a wrong baseline with no dangling sha involved. Mechanism only; it
  does not occur in the series today, since commit-date order and directory-date order agree
  across all 59 at day granularity. It is a further argument for the keying migration.
- **Neither the accuracy page nor `stats/README.md` tells an operator that the baseline is chosen
  for them, or how to check which one was chosen.** The selection rule that produced a 186-point
  phantom movement is documented only in a Go doc comment. Recorded as an absence; no edit is
  owed by this change.
