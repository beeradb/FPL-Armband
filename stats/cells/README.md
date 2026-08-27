# `stats/cells/` — banked sweep cells: screen INPUTS, and the evidence behind cited figures

Two kinds of thing live here, and telling them apart is the point of this file.

- **Inputs.** Per-cell CSVs that `stats/schedule_screen.R` and `stats/concentration_screen.R` **read
  as data**. Two committed scripts fail without them.
- **Evidence.** Banked cells behind a figure that `AGENTS.md`, or a `stats/findings/` narrative,
  states as settled record. No script reads them; a reader consults them to check a number the
  record asserts. In practice every citation here is currently from `AGENTS.md` — no
  `stats/findings/` entry postdates 2026-08-18 — but both count, and the rule below says so.

⚠️ **CORRECTED 2026-08-26: this file used to say the directory held inputs and *only* inputs, and
that the mechanical test for adding one was whether a `stats/*.R` screen grepped for its path.**
That was true when it was written and had stopped being true within a fortnight: ten directories
dated 2026-08-25 and 2026-08-26 are evidence, no R screen reads any of them, and `AGENTS.md` cites
eight of them by name. The rule and the practice had diverged, and a rule nobody follows stops
protecting anything. The rule below is widened to cover what the directory is actually for — **not
relaxed**, because an unbounded version of it is exactly the failure the original was written
against.

## Why they are not under `stats/snapshots/`

Everything under `stats/snapshots/` is *evidence for a verdict* — a record a reader might follow —
and the accuracy series that used to sit beside it now publishes as a GitHub Release rather than a
tracked directory.

**Cells are the exception, for two different reasons depending on which kind they are.** An input
cannot be replaced by an inlined figure: a screen that reads a cells file needs the file. And a
banked-evidence figure the record quotes as resolved needs somewhere checkable to have come from,
because the resident index carries verdicts and pointers rather than the cells behind them.

## What travels with a cells file

Each `<name>.csv` is accompanied by the sidecars that were beside it:

- **`<name>.provenance.csv` — the one that matters.** It records the data state the cells were
  measured at. ⚠️ **A cells file without it is a measurement of unknown provenance**, which this
  project already has a recorded failure about.
- `<name>.means.csv`, and `<name>.log` where one existed.

**Two exceptions, and they are the only two.** ⚠️ **CORRECTED 2026-08-26: this section used to name
`teamform.csv` as "the one file here whose data state is not recorded". There are thirteen.**

- **`2026-08-12-4d61058/teamform.csv`** has no provenance sidecar and never had one. It is kept
  because `concentration_screen.R` names it, but a result leaning on it carries that caveat.
- **`xgc-transport/`** holds twelve CSVs and **no provenance sidecar at all**, with four `.txt` logs
  standing in for them (three `*-console.txt` and a `tercile-inference.txt`). Read those for its
  data state; there is nothing machine-readable to grep.

  ⚠️ **The twelve do not all earn their place the same way, and an earlier draft of this section
  exempted the whole directory in prose — which is precisely the softening this rule is supposed to
  prevent.** They split:

  - **Four inputs.** `transport-*.csv`, reached through the documented `cp` at
    `stats/xgc_tercile_transport.R:7` rather than a hardcoded path. That is still test (1).
  - **Eight orphans.** `xgc-tercile-<season>.csv` and `xgc-tercile-<season>-buckets.csv` are
    `TestDiagXGCTransport`'s own **output** (`internal/backtest/xgctransport_test.go:347,349`), read
    by no committed script and cited by neither surface. They fail both tests exactly as the four
    named at the end of this file do, and belong on that list until someone cites or removes them.

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

**A new file must earn its place one of two ways, and the test stays mechanical.**

1. **As an input** — a committed `stats/*.R` script reads it. Test: `grep` the screens for the path.
2. **As evidence** — `AGENTS.md` or a `stats/findings/` narrative cites it *by name*, and it carries
   its provenance sidecar. Test: `grep` those surfaces for the directory name.

**Anything satisfying neither is an orphan, and an orphan is what this rule exists to keep out.** The
moment this becomes a place to put interesting cells, it is the snapshot series again under a new
name and the same removal will be due a second time. Note that (2) is a real constraint and not a
formality: it means the *figure lands first*, in the record, and the cells follow to back it up —
never cells banked speculatively against a claim nobody has made.

⚠️ **Four entries and eight files fail both tests today**, recorded here rather than quietly
deleted, because deciding which of the two kinds each is a judgement about the work that produced
it:

- `2026-08-25-decomp-rerun/` — cited by no admission surface. Its only reference anywhere is a
  cells-internal cross-link from `2026-08-25-f7d2be1b/README.md`, which is not one of the two.
- `2026-08-13-chipprep3.csv` and `2026-08-13-chipseq2.csv` — named only in a `sweep_inference.R`
  comment recording that the screen **dropped** them, which is a mention, not a read.
- The eight `xgc-tercile-*` files in `xgc-transport/`, per the split above.
- `2026-08-26-wildcard-noanchor/` — ⚠️ **not uncited, and the citations are the problem.** Two Go
  comments name it by exact path: `internal/backtest/xidrift.go:263` and
  `internal/backtest/wildcardreservation_diag_test.go:57`. Neither is an admission surface, and
  **both assert as settled fact the figure this directory's own README retracts** — that the
  shipped wildcard trigger leaves the policy taking *more* hits, **+0.58**. That arm's bar was
  **zero** and it was not the shipped rule; at the real bar it reads **−0.03**. The retraction is
  dated the same day as the comments and both files are still uncorrected on `main`. Fixing those
  two comments is what settles this entry; it is not this file's to do.

Either give each one a citation or remove it; do not let the list grow.
