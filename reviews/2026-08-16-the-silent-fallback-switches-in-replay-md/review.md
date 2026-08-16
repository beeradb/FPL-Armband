# The silent-fallback switches in `docs/replay.md`

## What was reviewed

`docs/replay.md` at `main` = `0248f23`, after the tier-1 batch merged. The batch's
`fail-loudly-on-an-unreadable-fixture-scale` branch added a row to this file, and the row landed
without a documentation review — caught by the `Stop` hook, not by me.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-docs-accuracy** | ✅ | `docs/` changed; this is its triage row |
| others | ❌ skipped | no code changed on this branch — the diff is `docs/replay.md` only |

## The finding that mattered, and it was not the row I merged

The new row was **broadly accurate**. The live defect was its **neighbours**: the same table
documents ten knobs that still fall back silently on a malformed value, with no warning anywhere on
the page — and the fingerprint stamps the string you *set*, not the value that ran. **This page's
own closing section is about exactly that failure class** ("a plausible number that measured
nothing"), and the table above it was a route straight into it.

Two are worse than a typo because they refuse a setting a reader would *deliberately choose*:

- **`FPL_CS_XGC_FACTOR=0`** — "no clean-sheet exponent at all", the natural extreme arm — reads as
  shipped, because `envDefaultAbove` defaults on anything `<= 0`.
- **`FPL_POS_MINUTES_SCALE`** drops an entry it cannot parse **while applying its siblings**, so
  half a position map arrives looking whole.

## Applied

1. **A warning paragraph under the constants table** naming which three switches refuse a bad value
   and which do not, with the two deliberate-setting cases called out. ⚠️ It carries its own scope
   caveat: **"three" counts the parsers in `internal/analysis/sweep.go` only**, and at least one
   more lenient parser lives outside it (`envFloat` in `internal/backtest/unified.go`, behind
   `FPL_MULTI_SURCHARGE` and `FPL_BUDGET_WEIGHT`). Without that clause the paragraph would have
   asserted a completeness it cannot support — the reviewer flagged this and it is the single most
   important edit in the set.
2. **Per-row caveats** on `FPL_CS_XGC_FACTOR` and `FPL_POS_MINUTES_SCALE`.
3. **Corrected the new fixture-scale row.** It said the parser "panics on anything it cannot read",
   which is both too strong and too weak: it *also* panics on a **negative** (a value it can read
   perfectly well — and inverting the ladder is a plausible arm to reach for), and it does **not**
   panic on `NaN` or `Inf`, which parse and are deliberately allowed so the fingerprint stamps what
   ran. Also noted `FPL_CS_SCALE` reads the same strict parser, since it sat silent directly under a
   row that talked about parsing and read as though only the ladders were strict.
4. **Retired a stale drift warning.** The page warned that `BLENDLO` and `BLEND2` were in
   `TestDiagRejudge` and not in the table. They are in the table; the reviewer enumerated every
   `pick.want` against its enclosing function and all three block tables agree with source. Kept the
   retraction in place rather than deleting it — this file's own convention, and the *opposite* of
   `CLAUDE.md`'s replace-don't-narrate rule, which is deliberate and documented.
5. **Two undocumented shipped-behaviour switches**, declared within twelve lines of one this table
   already documents: `FPL_NO_LEGAL_AUTOSUBS` and `FPL_WC_IGNORES_BOOST`. The first matters most —
   this page defines `HOLD` as "with autosubs and the vice-captain fallback applied", named both
   mechanisms, then offered a switch for only one of them.
6. **Annotated the 97 MB figure.** Correct as measured, but six runs banked since span **94 to
   130 MB**, the 130 from this batch. Annotated rather than replaced, per the page's own note that
   the figure "holds only until some arm makes it false".
7. **The memory cap can silently not exist.** Without a user systemd manager the wrapper falls
   through to plain `nice` and prints "running without a memory cap" to stderr. A guard rail a
   reader may believe they have and not have — the highest-value item in that section.
8. **One row for the whole `FPL_*_ROWS` class** of diagnostic output paths, five of which were
   undocumented, with the reader-facing point that they are skipped by the fingerprint and so cannot
   move a measured number. One row naming the class rather than five rows that would rot.

## Declined, with the reason

- **The mermaid diagram** of the malformed-value fan-out. It is a good idea and I am not taking it:
  the reviewer flagged it is **unrendered**, that `TestMermaidBlocksAreWellFormed` is not a renderer
  and catches only four plausible breakages, and that its colour coding reads deliberately backwards
  (the panic is the *green* outcome). The prose paragraph in item 1 covers the same ground, and this
  page already carries two diagrams. Adding an unverified third to a file that just gained a
  correctness warning is the wrong trade. **Recorded rather than dropped** — if someone renders it,
  it is worth revisiting.
- **Rows for the eight other undocumented switches** the reviewer listed as "holes"
  (`FPL_APPEARANCE_FIT`, `FPL_TEAM_FORM`, `FPL_MULTI_SURCHARGE`, `FPL_BUDGET_WEIGHT`,
  `FPL_TEAMNEWS_MAX_HOURS`, `FPL_CHIP_PLAN`, …). Real, and out of scope for a branch whose remit was
  the row that just merged. `FPL_MULTI_SURCHARGE`/`FPL_BUDGET_WEIGHT` are named in the new paragraph
  because they carry the scope caveat; the rest belong in their own pass.
- **`FPL_FIXTURE_LOAD`.** The reviewer could find **no consumer** — only a `printf` reporting it.
  That is either a dead switch or a reach failure, and it is a code question rather than a
  documentation one. **No doc should acquire a row for it until someone adjudicates**, which is the
  reviewer's own position and I agree.

## What could not be checked

- The diagram was not rendered, so its correctness is unverified.
- The full suite hits the live FPL API; I ran `internal/snapshot` (which holds the doc and staleness
  guards) and the mermaid check. No sweep was run, and nothing here produces a number.
- Whether `FPL_FIXTURE_LOAD` is dead — see above.
