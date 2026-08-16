# Merge close-out — the data-repair branch against main

No new review dispatched. The substantive record is
[`2026-08-13-cae6941`](../2026-08-13-cae6941/review.md) (three reviewers over
`4d61058..0a7a8ff`), continued in [`a6626ff`](../2026-08-13-a6626ff/review.md) and
[`d8d2450`](../2026-08-13-d8d2450/review.md). This covers the merge with main and the two runs
made after that close-out.

## The merge

Main had moved **33 commits** past the merge base while this branch went 47 — chips, the free-hit
horizon, and wildcard truncation. Two conflicts, both in Go comments, **both resolved main's way on
merit**:

- **`harness_test.go`** — both sides recorded that the xGC repair supersedes the pre-repair block.
  Main's version keeps the original text as the pre-repair baseline and marks it superseded above,
  which is this record's convention; ours had replaced it. Main's also carries the 12 → 18 cell
  correction.
- **`notes_test.go`** — main raised the CLAUDE.md byte budget to 60 KB with a better argument than
  ours: pinning the constant to the current size is a **ratchet**, because *"when adding a rule
  costs a commit and deleting a hedge costs nothing, hedges go"*, and it says explicitly not to
  re-compress qualifiers.

⚠️ **This branch is that comment's case study, and the merge is where it was noticed.** Working
under the then-current 54 KB, it compressed a qualifier to fit on **four separate occasions**: the
`MinutesWeight` non-inversion argument, the fourth-cluster figures, the vice drift chain, and the
`FIXW` margins that a review had *just* restored after an earlier compression removed them. Each
was defensible alone; the aggregate is exactly the ratchet. Raised to **64 KB** naming what needs
the room, after auditing both halves for duplication first — 128 bullets, zero repeated openings,
since two branches editing one index is the obvious way to pay twice for one claim.

**No functional test failed on the merged tree.** Main's chips and free-hit work and this branch's
data repairs coexist without interaction, which was the real risk.

## The two runs since the close-out

**Block `H` (`min_gain`) on both grids** — the transfer setting the grid question actually needed.
Widening helps **4 of 4 arms**, taking the combined count to **10 of 11** and retiring the
four-season exception on the population it was written for. A free corroboration: `min_gain` 0.00
and 0.40 have identical standard errors to four decimals on both grids, independently reproducing
the recorded "byte-identical seasons" finding.

**The held-out season, and this is the largest thing the session turned up.** Several constants are
recorded as unvalidated because **2022-23 disagreed** — and 2022-23 is precisely and only the
season the backfills changed (6 of 24 four-season cells, all of it). On 2022-23 alone the best arm
moves with the data state in **4 of 4 blocks**. So those out-of-sample justifications were reached
on data that no longer exists. Queued in TODO as a sweep programme rather than annotated away,
with the distinction preserved: **what is void is the stated justification, not necessarily the
choice** — most of these ship on *nothing beats it*.

## The accuracy snapshot

Refreshed at `2026-08-13-1ed7ff6`. Its staleness guard was blocking: the newest snapshot predated
the starts data, the starts repair and `season.go`'s repair ordering. Eight diagnostics plus the
render, all exit 0, diffed against `5edf13c`. The harness half reuses the six-season `MINHL` cells
from this session rather than re-sweeping. The two invariance rows reporting movement are `MINHL`'s
own, and `MINHL` is a scoring knob — the snapshot's text says `HOLD` should move for one of those,
so they are a description rather than a failure.

## State at this commit

| gate | status |
|---|---|
| `go build ./...`, `go vet ./...` | green |
| full `go test ./...` | green |
| `TestReviewCoversTheCurrentCode` | this record |
| `TestSnapshotCoversTheCurrentCode` | `2026-08-13-1ed7ff6` |
| fast-forward from main | **available** — main is an ancestor of this commit |

## Still open, deliberately

- **Re-deriving the constants whose verdict rests on 2022-23** — a sweep programme, queued in
  TODO with its priority order (bench slot weights, then `BandStrength`, then the reliability mix).
- **The wording half of `reviews/2026-08-13-backfill-runs`** — applied where a conclusion changed,
  left where it is emphasis.
- **The `MinutesWeight` tie-break** — restricting the contrast to ever-presents, where the
  reconstruction's own error is 3.9% rather than 29.7%, would break the non-independence. Cheap and
  named.
- **A held-out check on native data is no longer possible** for anything: every season outside
  2023-24 / 2024-25 / 2025-26 carries backfilled xG, xGC or starts.
