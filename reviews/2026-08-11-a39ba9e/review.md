# Review record — the historical availability backfill

**Commit reviewed:** `a39ba9e` on branch `wayback-backfill`, based on `origin/main` at
`e555c88`. Previous record: [`2026-08-11-6d65e04`](../2026-08-11-6d65e04/review.md).

**On the rename.** The review ran against an uncommitted working tree, because the agent that
did the work could not commit — its worktree pin moved mid-task. This record was therefore
first written as `2026-08-11-e555c88`, naming the *base* rather than the work. It was renamed
to the commit that actually contains the reviewed bytes once that commit existed, and the
contents are otherwise unchanged. The reviewed tree and `a39ba9e` are the same bytes: the
commit was made from exactly the tree the reviewers read.

**What the change is.** Three new packages and a CLI subcommand that recover
point-in-time FPL team news for finished seasons from Internet Archive crawls of
`bootstrap-static`, plus 33 MB of recovered data for 2020-21 through 2025-26. See
[docs/backfill.md](../../docs/backfill.md). Nothing in the scoring path reads it.

## Triage

| touched | dispatched | why |
|---|---|---|
| `docs/` (new `backfill.md`, edits to `architecture.md`, `README.md`) | **fpl-findings-audit** | the triage table's `docs/` row |
| new network I/O, third-party bytes committed to the repo, a new CLI writer | **fpl-security-review** | nearest to the `internal/fpl` / config-persistence row |
| new packages with a leakage-shaped safety property | **fpl-code-review** | correctness of a point-in-time guarantee |

**Skipped:** `fpl-stats-review` — nothing here measures anything or moves a constant,
and the change is enforced not to reach the scoring path.
`fpl-run-review`, `fpl-season-maintenance` — no live run, no season lists.

## The invariant came first, per the skill

**What must this change NOT move? Every replayed figure in the research record.** The
backfill recovers something the replay would obviously like — the injuries that
resolve, which `statusAt` cannot reconstruct at all — and wiring it in is a one-line
import that would silently make half the record incomparable with the other half.

`backfill.TestTheScoringPathCannotSeeRecoveredTeamNews` source-scans
`internal/analysis`, `internal/backtest` and `internal/agent` for imports of
`internal/{backfill,wayback,capture}`. Verified non-vacuous: it scans 176 files and the
same matching finds real imports. `internal/agent` was added on the security review's
recommendation and is the most important of the three — the recovered payloads carry
FPL's free-text `news` field, so the agent loop is the one surface where third-party
text could act rather than merely be wrong.

`TestEveryStoredCaptureIsPointInTimeHonest` re-derives the safety property from the
stored bytes for all 229 captures, in the ordinary suite rather than behind DIAG.

## Findings applied

**From my own re-reading, before any reviewer reported:**

1. **The brief's design leaks, and the cross-check caught it.** Reading all 38
   deadlines from one mid-season crawl is unsafe because FPL's calendar moves within a
   season: on 2020-21 the discovery crawl's deadlines for GW25 onward are up to 17.5 h
   later than the truth. Six of 38 gameweeks would have *selected* a post-deadline
   crawl. Fixed by `resolve`, which re-reads each gameweek's deadline from a crawl near
   it. Regression: `TestAProvisionalCalendarDoesNotLeak`, which also asserts the naive
   rule still reaches a post-deadline crawl on its fixture, so the test keeps meaning
   something if someone later calls the resolver over-engineering.
2. **A hole in the widen branch.** `resolve` widens its bound to whatever deadline a
   verified crawl reports, and a crawl can report a deadline FPL had not yet corrected.
   Capped at `hardBound` = first kickoff − 90 min, from the season archive, which is
   the one estimate that cannot be provisional.
3. **Two coverage-table builders.** `capture.Store.Coverage` and `backfill.Rows` built
   the same table, and the offline `-coverage` report used the poorer one — so the
   staleness column printed `—` in the mode people actually run. Removed the weaker;
   `backfill.Rows` is now the only builder. **This reappeared inside a change whose own
   documentation warns about it**, roughly an hour after I wrote that warning.
4. **A 51-second capture displayed as "0.0 h before the deadline"** — the one value the
   store refuses to hold. Now two decimals.

**From fpl-code-review (12 findings, all confirmed by the reviewer against the data):**

5. `Store.Manifests()` iterated `byGameweek`, so the headline honesty test walked one
   capture per gameweek, not all of them — exact at today's cadence, one-in-five at a
   denser one, while printing a total that read as complete. Now iterates every entry.
6. `-per-gameweek > 1` stored captures no read API could reach. Added `Store.AllAt`.
7. `Rows` swallowed the verification error, so a stored capture whose bytes no longer
   prove their own timing reported as ordinary coverage. Now a loud `Row.Err` and
   **not** counted as covered.
8. `Availability` omitted `EventsBehind` — the field this package argues at length is
   the one to read first. Now computed in `readAt`.
9. `Availability`'s "SnapshotAt is always strictly before Deadline" is false for *live*
   captures. Comment corrected to scope the guarantee to backfilled ones.
10. A killed run could leave a truncated cache entry that reads as a permanent Archive
    gap. Cache writes are now temp-file-plus-rename.
11. `wayback.Client` documented serialisation it did not enforce. `last` is now
    mutex-guarded, so the politeness floor is a property of the type.
12. Dead fields removed (`Row.Stored`, discarded `resolve` return); `Result.Crawls` now
    printed; the duplicated season list collapsed to `backfill.Seasons`.
13. Minute-resolution directory names silently dropped a colliding second crawl. Now
    distinguishes "already have this crawl" from a collision, and says so.

**From fpl-security-review:**

14. **The CDX index was fetched over plaintext HTTP** and the `original` URL it names
    was interpolated into the payload fetch unchecked. Since save-page-now is open to
    anyone, a rewritten index chooses what gets archived into this repo as evidence.
    Now HTTPS, and every row must name the requested resource (`sameResource`).
15. No size cap on the network read or the gunzip. Both now `io.LimitReader` at 64 MB.
16. The season string reached `filepath.Join` unvalidated — `startYear` constrains only
    the first field, so `2020-21/../../elsewhere` parsed. Added `ValidSeason`.
17. **The `Digest` field's documented purpose was false.** It claimed to be the handle
    for checking a refetch; the reviewer measured 14 of 228 matching, because CDX
    digests the archived record while we store a re-compressed decoded body. Comment
    corrected to name `File.SHA256`, which does hold — all 230 files verified.

**From fpl-findings-audit:**

18. **"Would have *stored* a leak" was wrong in three code comments** — the doc had it
    right and the comments did not. `VerifyPreDeadline` refuses those bodies, so the
    naive design loses six gameweeks as visible **gaps**. Corrected everywhere; the
    distinction is the whole reason the second mechanism exists.
19. **The discovery crawl is 26 January 2021, not 27**, in five files. I had quoted the
    crawl I probed by hand rather than the one the code selected; `deadlines.json`
    records `source_at: 2021-01-26T10:05:35Z`.
20. **The worst inversion is not −17.5 h.** That holds for 2020-21 to 2023-24; 2024-25
    reads −42.0 and 2025-26 −41.5. The doc quoted the season that matched.
21. **`MaxMedianGapHours` was labelled weaker than its evidence and conflated two
    claims.** The 1.50 h median is measured on **all six seasons** (227 gameweeks); the
    bound of 6 is asserted. Now says so separately.
22. **The drift mechanism is inferred.** Eight of ten moved to Friday 19:00/20:00 and
    two to a Saturday lunchtime slot; none to a Saturday evening, which the comment
    claimed. And GW26 sat inside the quoted "GW25-35" band without having moved.
23. **The coverage result was absent from the doc**, so the 100% had nowhere to be
    qualified. Added "What was actually recovered", including the finding that only
    **4 of 228** rows are stale on `EventsBehind` against 10 on hours, and that **7
    gameweeks are served by 3 crawls** — so anything treating gameweeks as independent
    draws must pool or exclude them.
24. `docs/backfill.md` claimed `VerifyPreDeadline` would refuse via `is_current`, which
    is not one of the three things it reads. Corrected to name the deadline and
    `is_next` tests, and to say plainly that the crawl was fetched by hand to check.

## Declined, with reasons

- **Changing `BackfillDirName` to second resolution.** It would rename all 228 existing
  directories, and a re-run would refetch rather than recognise them — spending a
  charity's bandwidth to fix a collision reachable only at a cadence nobody has run.
  Handled by making the collision loud instead (finding 13).
- **Verifying the `/drf/` pre-2020 sparsity claim.** Inherited from the task brief, not
  checked. One CDX query would settle it; it is now labelled unverified in both places
  rather than silently trusted. Declined because the floor of 2020-21 is independently
  justified by six seasons that do work.
- **Marking `internal/capture`'s "it yields nothing this season" as superseded.** The
  audit is right that it should be, and I have not done it — see "left undone".
- **Enforcing `Deadlines.Confirmed`'s documented contract.** Its stated rule ("treat an
  unconfirmed deadline as an upper bound") binds no reader. All six seasons on disk are
  fully confirmed, so it is latent; recorded rather than fixed.
- **Reporting "38 of 38" for 2022-23.** GW7 was cancelled outright after the Queen's
  death and never played. The denominator is a constant 38 in `CoveragePct`. Left
  alone: the research record already documents that gameweek, and a special case in the
  coverage denominator is a worse trap than a footnote.

## What could not be checked on this harness

- **Whether the recovered flags improve anything.** Out of scope by instruction and by
  the isolation test. This change delivers data and proves it honest; measuring it is a
  separate job with its own pass.
- **The claim that FPL updates availability hardest in the hour *after* a deadline**,
  which is the stated motivation for the strict-before rule. It is arithmetically
  plausible and **structurally unmeasurable from this store by design**, since nothing
  after a deadline is ever stored. The rule is correct either way, so this is a
  labelling matter.
- **Whether broadcast selection is what moves the fixtures.** Not checkable from the
  archive. What is measured is that the kickoff moved and the deadline moved with it.

## Left undone

Named here rather than quietly: the `internal/capture` doc comment still says the four
blocked questions wait until next spring, and three of them no longer do. It wants a
superseded marker pointing at `docs/backfill.md`.
