# `stats/cells/` — banked sweep cells that are INPUTS, not evidence

These are per-cell CSVs that `stats/schedule_screen.R` and `stats/concentration_screen.R` **read as
data**. They are not here for a reader to consult; they are here because two committed scripts fail
without them.

## Why they are not under `stats/snapshots/`

⚠️ **The snapshot series is being removed from this repository and from its history.** Everything in
it is *evidence for a verdict* — a record a reader might follow — and the plan is that such records
live outside the repository, with each figure carried inline wherever it is cited.

**These files are the exception, and the distinction is the whole reason this directory exists: an
input cannot be replaced by an inlined figure.** A screen that reads a cells file needs the file. So
they moved here rather than out, and the rest of the series can go.

## What travels with a cells file

Each `<name>.csv` is accompanied by the sidecars that were beside it:

- **`<name>.provenance.csv` — the one that matters.** It records the data state the cells were
  measured at. ⚠️ **A cells file without it is a measurement of unknown provenance**, which this
  project already has a recorded failure about.
- `<name>.means.csv`, and `<name>.log` where one existed.

⚠️ **`2026-08-12-4d61058/teamform.csv` has NO provenance sidecar.** It never had one. It is kept
because `concentration_screen.R` names it, but **it is the one file here whose data state is not
recorded**, and a result that leans on it carries that caveat.

## The directory names are labels, not pointers

The `<date>-<sha>` subdirectories are the snapshot directories these came from, kept so a reader can
tell which sweep produced which cells. ⚠️ **CORRECTED 2026-08-22: this was written in the future
tense about a rewrite that had already happened.** The 2026-08-16 history reset (one root commit,
`61bf00a`) means most of these SHAs already name commits absent from the published repository —
recount before quoting one; some predate the reset entirely and never resolve, others survived it as
directory names carried forward with the data. Either way: **do not try to resolve one, and do not
renumber them to match a new history**, which would assert a provenance that was never true. The same
rule applies to `reviews/`'s 79 SHA-keyed directories — see `reviews/README.md` for the dated note
recording why.

## Adding to this directory

**Only add a file that a committed script actually reads.** The moment this becomes a place to put
interesting cells, it is the snapshot series again under a new name, and the same removal will be due
a second time. The test is mechanical: `grep` the `stats/*.R` screens for the path.
