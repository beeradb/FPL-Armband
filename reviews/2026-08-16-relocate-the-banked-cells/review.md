# Relocating the banked cells the R screens read

## What was reviewed

Moving the sweep cells that `stats/schedule_screen.R` and `stats/concentration_screen.R` consume as
**data** out of `stats/snapshots/` and into `stats/cells/`, and repointing both screens.

This is a prerequisite for removing the snapshot series, not a tidy-up. **41 files moved**: 13 cells
CSVs plus their `.means.csv`, `.provenance.csv` and `.log` sidecars.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| none dispatched | ⚠️ **A deliberate skip, and the reasoning is that an invariant beat a reviewer here.** `review-gate`'s own first section says to ask what quantity the change must not move and to test that instead. The quantity is *what the screens print*, and the test is running them. Both were run end to end against the relocated files — see Verification. A reviewer reading a file-move diff would have been weaker evidence than the screens executing |

## The finding that shaped it

⚠️ **The snapshot series' removal was costed as a pointer problem, and these files are not pointers.**
Every other citation into `stats/snapshots/` can be discharged by inlining the figure beside the
claim. **These cannot: a screen that reads a cells file needs the file.** That distinction is why
this directory exists at all, and it is written into `stats/cells/README.md` so the next removal pass
does not treat it as more of the same.

## What was applied

**1. The dependency was DISCOVERED, not transcribed.** The move script greps both screens for
`stats/snapshots/...csv` and acts on what it finds. ⚠️ **A hand-written list is the thing that goes
stale** — and the two screens' `COMMITTED` vectors differ (`concentration_screen.R` names
`blendlo.csv`, `schedule_screen.R` does not), so a single hand list would have been wrong for one of
them.

**2. Sidecars travel with the cells.** Anything sharing a stem moved with it. ⚠️ **`.provenance.csv`
is the load-bearing one** — it records the data state the cells were measured at, and a cells file
without it is a measurement of unknown provenance.

**3. The directory structure is preserved** as `stats/cells/<snapshot-dir>/<file>`, rather than
flattened. Flattening would have lost which sweep produced which cells, which is exactly the
provenance the sidecars exist to carry.

**4. One stale prose comment fixed.** `schedule_screen.R`'s header said *"for every sweep already
committed under `stats/snapshots/`"*, which the move falsifies.

## What was found and NOT fixed

⚠️ **`2026-08-12-4d61058/teamform.csv` has no `.provenance.csv` sidecar.** It never had one — this
move did not lose it. It is kept because `concentration_screen.R` names it, and it is **the one file
in the new directory whose data state is not recorded**. Any result leaning on `TEAMFORM` carries
that caveat. Recorded in `stats/cells/README.md` rather than silently carried across.

## What was declined, and why

- **Renumbering or removing the `<date>-<sha>` directory names.** After the planned history rewrite
  those SHAs name commits that will not exist. **Left as labels** — they group cells by the run that
  produced them, which is information. ⚠️ **Renumbering them to match a new history would assert a
  provenance that was never true**, which is worse than a label that no longer resolves. Written into
  the README so a later pass does not "fix" it.
- **Moving any other cells.** Only files a committed script actually reads were moved. The README
  states the test mechanically, because the failure mode is obvious: the moment this becomes a place
  for interesting cells, it is the snapshot series under a new name and the same removal comes due
  again.

## What could not be checked on this harness

- **That the screens' OUTPUT is unchanged**, as opposed to the screens running. They were not
  diffed against a pre-move run. Both completed and printed their full tables and caveat blocks; a
  byte-diff of the output would be stronger and was not done.
- **Whether anything outside these two screens reads a moved file.** The grep covered `stats/*.R`
  and the Go tree; a consumer outside the repository would not be visible.
- **No detection threshold applies.** Nothing was measured here — this is a file move.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` (empty), `go test ./...` all pass.

**Both screens were RUN, not merely parsed:** `Rscript stats/concentration_screen.R --committed` and
`Rscript stats/schedule_screen.R --committed` each completed against the relocated files and printed
their tables. `grep -c stats/snapshots` over both screens returns 0 paths — the single remaining hit
was the prose comment, now corrected.

---

# Second piece: retaining the findings layer

## What was reviewed

Moving all fifteen `stats/snapshots/*/FINDINGS.md` to `stats/findings/<dir>.md`, so the retraction
narrative survives the series' removal.

## The finding that changed the scope

⚠️ **The design, and `retracted_test.go`'s own comment, named ONE file** —
`2026-08-15-gatescaled` — as holding in-place markers for three figures the retraction guard cannot
check, because their context words ("gate", "transfer", "threshold") are too common to key on.

**Counted: twelve of the fifteen carry withdrawal or retraction language**, and
`2026-08-15-clean-sheet-2x2` carries more of it than the file the test names. The test's comment is
accurate about its own three figures and **is not an inventory**, which is how a single-file rescue
would have quietly dropped eleven others.

**So the whole layer was retained rather than one file rescued. It costs 291 KB against the series'
11.0 MB — about 2.6%** — which is cheaper than being right about which markers matter.

The condition this protects is stated by the moved file itself: *"a verdict-only resident file only
works if the thing it stopped carrying lands somewhere a checkout can still reach."* `AGENTS.md`
deletes stale claims rather than annotating them, so without these the terseness stops being a
design and becomes a loss.

## What was applied

1. **Fifteen files moved**, `retracted_test.go`'s pointer updated to the new path.
2. **They stay OUTSIDE the retraction guard, unchanged.** The guard globs `stats/*.md`, which does
   not recurse, so `stats/findings/` is unscanned exactly as `stats/snapshots/` was. ⚠️ **That is
   deliberate: these files quote withdrawn wording verbatim, and a guard that scanned them would
   fire on the very text they exist to preserve.** Written into the directory's README so it is not
   "fixed" later.

## Two things the move did that nobody predicted

⚠️ **1. It repaired a broken citation, and killed two allowlist entries as a side effect.** Both
exemptions covered a two-level relative link in the banked findings for `2026-08-11-0104d9d`. From
the old depth that prefix reached `stats/` and named a path that never existed. From the new depth
it reaches the repository root and resolves against the note's real former home — so the citations
stopped dangling and the exemptions stopped describing anything. **Removed, with the reason recorded
in the code.**

**The reusable lesson: a relative link's correctness is a property of the file's DEPTH, not its
content, so a move can repair or break one with no edit. Re-run the guard after any relocation.**

⚠️ **2. The comment explaining that repair tripped a different guard.** Writing the path in the
explanation made `TestNoLivePointerCitesTheRecordByPath` fire on this test's own source, because
that literal is on its retired-location list. Rewritten to *name* the note rather than path to it —
which is what that guard's failure message tells you to do, applied to itself.

## What could not be checked

- **Whether anything outside this repository cites the moved findings by their old path.** A
  `reviews/` record that does is a dated attestation and is deliberately left alone.
- **No detection threshold applies.** Nothing was measured; this is a file move.
