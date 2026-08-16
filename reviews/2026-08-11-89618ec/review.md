# Review record — the before/after figure for the replay wrapper

**Commit reviewed:** `89618ec`. Range `c54917d..89618ec`, following on from
[`2026-08-11-c54917d`](../2026-08-11-c54917d/review.md), which holds the substance.

**This is a recorded "no further review owed", in the form the `review-gate` skill asks for**, so
the next pass does not re-ask.

## What changed

Two prose lines, in `CLAUDE.md` and `docs/notes/harness-and-inference.md`, adding one measurement
that was still running when the reviewed commit was written: the same sweep block executed both
ways is **1031 MB through `go test` against 97 MB through `scripts/replay`**, and no slower.

## Why no reviewer is owed

**Nothing was claimed that was not measured, and nothing was retracted.** The figure confirms the
mechanism the previous record already reviewed — that the `go test` driver, not the replay, carried
the memory cost — rather than introducing a new claim. It moves the "removes about seven eighths"
estimate to a directly measured tenth.

**No Go source changed**, so the free invariant that governed the previous record still holds
verbatim:

```
git diff --name-only c54917d..89618ec -- '*.go'   ->   0 files
```

`go build ./... && go vet ./... && go test ./...` is green.

## The one caveat carried forward

**The "no slower" half is observed, not controlled.** 6:17 through the wrapper against 8:39
through `go test`, but the two runs saw different machine load — the second overlapped a full
`go test ./...` — so the honest reading is that removing the driver did not cost time, not that it
saved 2:22. The memory figure is the claim; the timing is an observation beside it. Anyone wanting
the speed result should run the two arms alternately on a quiet host.
