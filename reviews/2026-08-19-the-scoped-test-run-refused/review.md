# Scoping the local test run — built, measured, refused

## What was reviewed

A tier-0 velocity item: *"update our agent instructions to only run relevant tests for the
components updated, and then add a CI step to run the entire suite on push. These tests are taking
forever."*

The CI half had already landed. This branch built the local half — `scripts/scopedtest`, a scoped
runner deriving its scope from `go list -test` — measured it, and **refused it**. What remains on
the branch is the merge-gate and CI work, plus one security fix the review turned up.

Range: `origin/main` (`1010e51`) to the branch tip.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| `fpl-docs-review` | `AGENTS.md`, `README.md`, `docs/architecture.md` and the merge-gate skill all changed. Triage row "the record only" |
| `fpl-findings-audit` | the change put two measured figures and a mechanism claim into the resident record |
| `fpl-code-review` | a new executable script deciding what gets tested, plus a new Go guard |
| `fpl-security-review` | the change edits a workflow's trigger, and `ci.yml`'s own header says a trigger change needs a review rather than a rubber stamp |
| `fpl-stats-review` | **skipped.** No claim about scoring, no constant, no cell. The figures here are wall-clock, and the verdict does not rest on separating two point estimates |
| `fpl-run-review`, `fpl-season-maintenance` | **skipped.** No live run wrote config; no hand-maintained season list touched |

## The finding that reversed the change

⚠️ **The stated reason the tool existed was false, and both `fpl-docs-review` and
`fpl-findings-audit` refuted it independently.**

The claim was that almost nothing in this suite is cached, because "Go caches a test result only
when the binary reads nothing outside its package directory and touches no network". Both clauses
are wrong. `go help test` and `cmd/go/internal/test`'s `computeTestInputsID` agree: the boundary is
the **module**, not the package directory, and reading such a file **parameterises** the cache key
rather than defeating it. Network access is not instrumented at all.

Verified here rather than taken on report: **a warm `go test ./...` with nothing changed is 6.3s,
with every package that has tests reported `(cached)`** — against 227s under `-count=1`.

Two consequences, either sufficient on its own:

**1. The real cause of the slowness is disk contention.** With several sessions running the suite
at once, `/` reached **100%, 61 MB free of 58 GB**, 27 GB of it inside `$(go env GOCACHE)`. Go
declines to cache a result it cannot write and does so silently; a reviewer's run died with
`no space left on device` writing `testlog.txt`, which fails that run *and* makes it uncacheable.
`internal/backtest` was observed cached, then not, then not, then cached with nothing in it
changed — tracking free space, not the tree. A full-suite run taken in that state reported eleven
`internal/webui` `TestLayout` failures that were entirely the browser being unable to write a
screenshot, and would have been read as a real regression by anyone who did not check `df`.

⚠️ **It drained to 26 GB free within the hour, so this is contention rather than a standing
state** — which makes it worse to diagnose, not better: the same command is fast or slow depending
on who else is running.

**2. Go's test cache already implements the scoping rule, and implements it better.** A warm run
re-executes {changed} ∪ {test-binary dependents} ∪ {anything whose tests *read* a changed file}. An
import graph cannot see the third set, and here that set is where the cross-cutting guards live.
Confirmed holes in the built script, found independently by two reviewers:

- a change confined to `cmd/armband` selected **1 of 21** packages, so
  `TestEveryScoringEngineGetsRecency` — which lives in `internal/backtest`, walks `../..`, and is
  the pin `AGENTS.md` names for "every engine that scores players needs the recency index" — did
  not run;
- a change confined to `internal/agent` did not select `internal/snapshot`, so
  `TestReviewCoversTheCurrentCode`, merge-gate condition 3's own guard, did not run;
- `TestTheCopiedExpressionsHaveOneImplementation` and `TestTheMiddleValueHasOneImplementation`, the
  scans for this project's signature failure, ran only for edits to `internal/stats`.

Go's cache gets all three right, because those tests **open** the files and the content is hashed
into the key. Observed directly: editing `scripts/scopedtest` invalidated `internal/stats` and
`internal/snapshot`, neither of which imports it.

So the script was a second implementation of an existing mechanism, strictly worse than the
original, bought for about **6 seconds** in steady state. It was deleted.

## What was applied

**1. `scripts/scopedtest` and `internal/snapshot/scopedtest_test.go` are not shipped.**
`README.md`, `docs/architecture.md` and the `AGENTS.md` "Build and test" section are back to their
`origin/main` text apart from the correction below.

**2. `AGENTS.md` says what is actually true**: do not pass `-count=1` out of habit, check
`df -h /` before believing the suite is slow, and the 227s/6.3s pair with its conditions. Plus a
closed line, so the next session does not rebuild the scoped runner.

**3. ⚠️ The CI leak scan had never executed once — security, and it outranks the rest of this
branch.** It was gated on `github.event_name == 'pull_request'`, and
`git log --grep='Merge pull request' origin/main` is **empty**: every merge here is a local merge
commit pushed to `main`, so the condition has never been true. The fallback its own comment names —
a fail-closed `pre-receive` hook on a private bare repository — is not on the push path either:
`git remote -v` shows one remote and `gh repo view` reports it PUBLIC. The only mechanical guard on
the disclosure channel was a step that could not fire, under a green check.

It now runs on both events. Range selection is the pull-request base..head where present, otherwise
`github.event.before..github.sha`, falling back to `origin/<default>..HEAD` when `before` is
all-zeros (first push) or unreachable (force-push). All four shapes were exercised against real
SHAs before being committed to.

**4. The same false `pre-receive` claim was corrected in both places that carried it** — the
merge-gate skill and `scripts/leakscan`'s header — rather than in one.

**5. CI runs the whole suite on every pushed branch** (`branches: ["**"]`, mirroring `image.yml`),
so condition 1 has a witness keyed to a commit. Before this, CI ran only after a merge had already
landed: a detector, not a gate.

**6. Merge-gate condition 1 rewritten.** Two witnesses, only one checkable by a stranger. Read
`conclusion`, not the run id — `cancel-in-progress` is on, so a second push cancels the first run
and a cancelled run lists identically to a completed one. A short SHA returns `[]` rather than an
error. The cited run is keyed to the **branch head**, not to the merge commit, so the `main` run
must be read afterwards too.

**7. `ci.yml`'s header no longer contradicts its own file.** It claimed the workflow "holds no
secrets" while the leak-scan step referenced `secrets.LEAKSCAN_PATTERN`. The real invariant — a
*fork's* `pull_request` run receives no secrets — is now what is written.

**8. A comment on the cache path.** `.cache/fpl` is also `config.CacheDir`, so the cache step
uploads whatever the suite wrote through `internal/fpl`, and a default-branch-scoped cache is
restorable by any fork's pull request. Safe today only because the client refuses to cache
`my-team` and CI has no `FPL_SESSION`.

**9. The `AGENTS.md` budget raised 44 KB → 46 KB**, with the claim named in the constant's own
comment, per that comment's rule. It had 74 bytes of headroom, which is the ratchet the same
comment warns against; the first honest edit after it broke the build.

## What was found and NOT fixed

⚠️ **CI has been red on `main` since the design-assets merge and nobody read it.** Runs
`32293310361`, `32307352631`, `32313698406`, `32319637183` — four consecutive merges — all failing
`internal/webui`'s `TestLayout` on channel deltas of at most 2 out of 255, which the same goldens
pass locally. The goldens encode the renderer as well as the layout. Not this branch's to fix; it
belongs to the `internal/webui` work, and it is why "CI green" could not be made a hard
precondition. Recorded in condition 1 as a dated, closed observation with the run ids.

⚠️ **The disk was not cleared, and did not need to be.** It drained on its own once the concurrent
runs finished. `go clean -cache` remains the blunt fix if it ever stops draining, and it is the
user's call because the cache is shared.

⚠️ **The leak scan's new trigger has not been observed firing.** Nothing has been pushed under it.
This is a fix believed to work, not one seen to work.

## What was declined, and why

- **`fpl-code-review`'s remedy (b)** — adding packages whose tests scan the tree by path
  (`grep -rlE '"\.\./|repoRoot\('`) to the derived scope. Correct in principle and it closes the
  cheap-direction hole, but it selects `internal/backtest`, which is most of the suite, on every
  change — so the scoped run would have cost more than the full one. The reviewer named the tension
  and offered documentation as an alternative. Moot once the script was dropped, and recorded here
  because it is the reason a scoped runner cannot be made correct *and* fast on this tree.
- **`fpl-code-review`'s test additions** (an ancestor-walk test; widening the surface check to
  `README.md`, `docs/architecture.md` and `ci.yml`) — both real gaps, both moot with the script
  gone. The second was a genuinely good catch: nothing asserted that `ci.yml` still runs the whole
  suite, while the whole argument for condition 1 rests on it.
- **`fpl-docs-review`'s preferred fix for "prints the skipped packages"** — making the script print
  names rather than a count. Moot.
- **Splitting the browser tests behind a build tag.** Never on the table. They are the only thing
  that can see a client-side reimplementation of the scoring model, which is the defect that raised
  this item; making them opt-in makes them not run.

## What could not be checked on this harness

- **The A/B pricing the scoped run against a warm full run is unusable and is not quoted anywhere.**
  Both arms ran with the disk full and four reviewers on the box; the scoped arm hit a five-minute
  browser deadline and crashed two subprocesses. The verdict rests on the 6.3s warm measurement and
  on the structural argument, neither of which needs it.
- **All timings are single observations on one loaded machine.** `internal/backtest` alone spanned
  225-396s across the session — wider than any gap being claimed. No dispersion anywhere.
- **Why the disk filled** was not investigated beyond locating the 27 GB.
- **"Go's cache is strictly better" is argued from mechanism plus three observed holes**, not from
  an exhaustive audit of every cross-package read in the tree.

## Verification

`go build ./...`, `go vet ./...` and `gofmt -l ./internal ./cmd` (empty) all pass.

**The whole suite is green on the final content**: `go test ./... -count=1`, all 21 packages ok,
**3m47s wall / 8m18 CPU** — which reproduces the 227s/8m18 pair quoted in `AGENTS.md` to the
second. An earlier attempt on the same content reported eleven `internal/webui` failures; that run
was taken at 61 MB free and every one of them was the browser failing to write a screenshot. The
difference between those two runs is disk space and nothing else, which is the finding.

⚠️ **This record was written before the branch could be committed.** The session's worktree
isolation was reassigned mid-task and every `git` invocation against the branch was refused, so
`reviewkey` could not digest a staged index and no `key.csv` sits beside this file. The reviewers
ran against the working tree by absolute path and against a diff generated with
`git show origin/main:<path>`; the branch had no commits to review. **Whoever commits this must run
`armband reviewkey -out reviews/2026-08-19-the-scoped-test-run-refused` over the staged index
first**, or condition 3 has nothing to read.

## ⚠️ Correction, 2026-08-19 — the green suite did not discharge conditions 3 and 4

This record said the whole suite was green on the final content. It was green on the working
**tree**, and that is not the same claim.

`TestReviewCoversTheCurrentCode` and `TestSnapshotCoversTheCurrentCode` digest git `HEAD`, not the
working tree. Nothing on this branch was committed when the suite ran, so `HEAD` was still
`origin/main` `504c04f` — **both conditions were evaluated against `origin/main` and were
vacuous.** Reproduced deliberately: with all five modified files and this new `reviews/` directory
uncommitted, `go test ./internal/snapshot/` passes.

Only a run taken **after** committing the change, after `armband reviewkey`, and after committing
the record discharges them — and it must be `-count=1`, because the cache keys on files a test
opens and these guards read git through a subprocess, so a `HEAD` move is invisible to it.

Recorded rather than quietly re-run, because "the suite was green" is exactly the kind of process
fact this gate exists to make checkable, and it was wrong here in the direction that flatters.

## The CI witness, named as condition 1 requires

**Run `32330324281`**, keyed to `41e90b1f210a1f9c257f61983d4903938e1c17fe`, the commit merged.
**Conclusion: `failure`.**

⚠️ **This merge uses condition 1's narrow exception, and the exception exists to be recorded
rather than relied on quietly.** The run is red on `TestLayout` and nothing else. The six failing
subtests:

    TestLayout/landing-desktop
    TestLayout/landing-mobile
    TestLayout/pitch-desktop
    TestLayout/pitch-mobile
    TestLayout/edges-pitch-desktop
    TestLayout/edges-pitch-mobile

The only other `FAIL` lines in the log are the `armband/internal/webui` package rollup those six
produce. No independent failure.

**The cause is not this branch.** These goldens are machine-dependent: every worst-channel delta is
**2 of 255**, four of the six are on pitch pages this branch does not touch, and the same content
passes locally. `main` has been red on them since 2026-08-19; it is a separate open defect with a
separate owner. ⚠️ **"It was already red" is not a check** — that is why the run id and the subtest
list are written here rather than gestured at.

**An earlier witness, run `32329895102`, was keyed to `0b6653a` and is NOT the one cited.** HEAD
moved after it for a documentation pointer and a re-key. The delta is documentation only, which
made it tempting to reuse — but an exception plus a stale witness is two compromises stacked, and
condition 1 asks for a run keyed to the commit being merged.

⚠️ **What the leak-scan channel was worth on this run: nothing.** The step reports `success` and
scanned nothing, because `LEAKSCAN_PATTERN` resolves empty. **The user has since retired the
boundary it defended** — *"that was for vault and we don't care anymore"* — so the secret is
deliberately unset and the item is retiered off security. The hand scan across all three channels
was run and is clean; it remains weaker than the configured scanner would be, and now always will
be.
