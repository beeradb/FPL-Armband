# The copied-expression guard, and the two copies it found

**What was reviewed.** The working tree of `tier-0-queue-sweep` against its own tip `0ae92e4` —
queue item C1, "extend the guards rather than write a new one each time". Eight files edited, one
added (`internal/stats/copies_test.go`).

**No computed figure moves.** The only executable edits are two deduplications, both traced to
identity rather than asserted: `priors.priorFrom` is field-identical to the two composite literals
it replaces, and `cmd/priorblend`'s `graftRates` is **bit**-identical to the two mirror-image
literals it replaces — same operands, same order, same `int(x*k + 0.5)` rounding, no
reassociation available to the compiler. Everything else is a guard, a comment, or the resident
index.

## Reviewers

| reviewer | when | why |
|---|---|---|
| **fpl-code-review** | on the diff | two deduplications on a prior-construction path, one of them reached by the live engine; a new source scan |
| **fpl-docs-review** | on the diff | every new doc comment asserts a fact about this codebase's history; `CLAUDE.md` edited |

Skipped: **fpl-stats-review** (no measurement, no constant, no claim about points),
**fpl-security-review** (no `internal/agent`, `internal/fpl` or config-persistence change),
**fpl-run-review** (no live run), **fpl-season-maintenance** (the hand-maintained lists are
untouched). Self-review was not substituted for any of these.

## The re-scoping, which is half the item

The item named three targets. **Two of the three had no duplicate to collapse**, and saying so is
the deliverable rather than a shortfall — the failure class this whole line of work exists to
prevent is scheduling work without reading the code.

| target | verdict |
|---|---|
| `PriorSeasonName` | **already single.** `backtest.PriorSeasonName` holds the arithmetic; both `seasonBefore` copies delegate to it (closed at `d27c5c9`). Nothing to collapse. |
| `xiValue` | **already single.** `xiValueShrunk` is the implementation; `xiValue` and `xiValueForTransfer` delegate. `xiValueOfParts` is a documented deliberate copy, re-checked and still load-bearing, and `refXIValue` is a frozen differential oracle whose own comment forbids sharing code. Nothing to collapse. |
| `priorFrom` per source | **one real duplicate, collapsed.** Two field lists projected `priors.Player` into `analysis.PriorPlayer` — `Adapter.Get` and `LoadBlended` — now one `priorFrom`. A second pair, `cmd/priorblend`'s two channel arms, was folded into `graftRates` on the way past. |

⚠️ **The dated check attached to this item was itself stale on one point and right on two.** It
said only one `priorFromStrength` exists — true, and `PriorFromStrength` is its one-line exported
wrapper, so that is not the target either. It said the R half had already landed: confirmed,
`read_cells`, `read_cells_all` and `read_sidecar` are on the shared-name list and the walk is
recursive. It said `xiValueOfParts` must not be folded in: confirmed, and the reason still holds —
`bestFormation` still calls it inside the formation loop.

## The guard

`TestTheCopiedExpressionsHaveOneImplementation` is a table, not a test per quantity. It shares
`goSources` with `TestTheMiddleValueHasOneImplementation`, so the two scans cannot drift apart
about which files they reach, and the next quantity is a row.

It matches on the **idiom**, never the name — the lesson the median guard paid for, where nine of
eleven copies had no name at all. Demonstrated rather than claimed: a scratch copy named
`scratchOffender` with variables `best`/`second` is caught by row 1, a `Sprintf` season decrement
by row 2, and a `PriorPlayer` literal by row 3, none of them reusing an identifier.

Both failure directions were exercised. A new copy is reported; and deleting a **sanctioned** copy
reports the debt list as overstating the debt, which is what stops a migration being recorded as
outstanding after it has landed. The R guard was checked the same way: renaming `read_cells` in
`cells_common.R` fires "the shared home has lost a quantity".

## Findings, and what each changed

The two reviewers agreed on four defects and each found two more. All were applied.

### 1. The guard's own history was wrong about who diverged — applied

The doc said `cmd/fplagent` and `cmd/priorblend` derived the previous season from *different*
fields. They agreed with each other; both derived the end year from the season's **END** while
`backtest.PriorSeasonName` derives it from its **START**. So `"2024-30"` gave "2023-29" from the
two copies and "2023-24" from the one implementation. The row's own failure message had it right,
so the file contradicted itself.

### 2. Two live near-misses were absent from a paragraph that read as "the blind spot is empty" — applied

`backtest.captainAndVice` is the same running top-two over the same `Score` — its own doc says so —
and escapes row 1 because its tuple targets are paired. `cmd/fplagent`'s `priorSeasonName` is the
same year decrement as row 2 and escapes it by emitting the four-digit form. Both are outside their
row's stated `quantity`, so neither is an offender; both are now named in the limits, because they
are the templates the next copy gets written from. **Neither is collapsed**: `captainAndVice` sits
on the replay's scoring path where tie order is load-bearing, so folding it in would be a behaviour
change rather than a deduplication, and this change is required to move no figure.

### 3. A stated exclusion the regex did not implement — applied

The comment said map-literal element types were excluded. What the regex excluded was the
*empty-braces spelling*; every such literal in the tree happens to be written that way, so the test
passed by luck. A non-empty `map[int]analysis.PriorPlayer{` would have been reported as a second
projection with advice that does not apply. `priorContainer` now excludes container literals
explicitly, and the trade — their elements go unscanned — is recorded.

### 4. A named consumer that does not exist — applied

`Season.blend` was cited as the second caller of `priorFrom`. There is no such method; it is
`LoadBlended`. Naming the consumer *is* the check, so a pointer that does not resolve is the defect
the row's own comment forbids.

### 5. `render.go`'s deletion had a checkably false reason — applied

The first version said the grid "is decided by `FPL_SWEEP_SEASONS` and the sweep's own provenance,
neither of which this package can see". It can: `Sweep.Seasons` comes off the provenance and the
sweep table already prints "seasons replayed" and the cell count from it. The deletion is still
right, for two better reasons — `leadParagraph` takes no arguments and runs before any sweep is
read, and a model-only snapshot has no sweep at all; and the **default** lives in `sweepPairNames`
in `internal/backtest`'s test files, which no shipped code can reach. This was the comment a reader
was most likely to test.

### 6. Three overstated or miscounted claims — applied

A causal claim that the eight materialised elevens "was the allocation" that made the optimiser
GC-bound (the recorded profile is 176 KB and 61 blocks per evaluation across eight buffers, so it
was one of them); "eight copies" and "four bespoke guards", both of which a reader who counts gets
differently — replaced by the rule, per the standing preference for a rule over a tally; and a
"forty lines" that is fifty-one.

### 7. Comment lines false-positived, found by testing rather than by review — applied

A comment quoting `"%d-%02d"` or `PriorPlayer{` fired the scan, which would have made documenting
the rule break it. Line comments are now skipped, as the R guard already does with `#`.

## Two things the item asserted that the code refutes

- **"`internal/snapshot` is watched by both the review digest and the accuracy-snapshot digest."**
  It is in `ReviewWatchedPaths` only. `SnapshotWatchedPaths` is `internal/analysis`,
  `internal/backtest`, `internal/config`, `config.json`. So the `render.go` edit re-keys the review
  digest and **not** the accuracy snapshot, and no snapshot is owed. The same mistake is in
  `0ae92e4`'s message and in the comment that flagged the phrase; that comment is corrected here.
- **"Both debt lists must shrink."** Neither *could*. The R guard's `knownCopies` has been empty
  since 2026-08-14, and the median guard's one exemption is still live and still correct. What
  shrank is the number of copies in the tree — two folded away. Reporting the lists as unchanged is
  the honest answer; both were checked, and both still fail when a listed copy is gone.

## Known reach limits, recorded rather than assumed

Sanctions are counted per **file**, not pinned to an occurrence — the two sanctioned copies in
`squad.go` are the same 27 characters, so there is no text to tell them apart. That is weaker than
the median guard's key-on-the-expression, and it is now said so in the file. Counting **lines** is a
second seam. Both are visible in a diff of the file; what neither the key nor the count can see is a
copy in a file nobody diffed, which is the case the scan does catch.

## Not fixed here, and why

`internal/priors` and `cmd/priorblend` are on **neither** watch list, while `priors.Adapter` is
wired into the live engine. So a semantic change to the live prior projection can ship owing neither
a review nor a snapshot — the same shape as the `internal/config` hole `watched.go` records finding
by accident. Raised by the docs review, out of scope for a guards change, and it wants a decision
about the watch list rather than an edit to this diff.

## Build

`go build ./... && go vet ./... && go test ./...` — clean. Live-API tests skip where unreachable.
