# The tier-0 queue sweep

**What was reviewed.** `origin/main..HEAD` on `tier-0-queue-sweep` — nine commits, of which eight
were already written when this record was opened and the ninth carries it. The branch works a queue
of small items, none of them a scoring change, and its centre of gravity is **claims**: most of the
diff is comments, guard tests and the resident index, and most of the findings below are about
something being asserted more confidently than it had been checked.

**No computed figure moves, and that is checkable rather than argued.** Nothing on the scoring path
is touched: `internal/analysis` is byte-identical across the range, and the two executable edits in
the earlier commits (`priors.priorFrom`, `cmd/priorblend`'s `graftRates`) were traced to field and
bit identity respectively and are covered by their own record. The final commit's edits are
`CLAUDE.md`, one test comment and constant, six agent definitions, and two prose lines in older
review records. `go build`, `go vet`, `go test ./...` and `Rscript stats/cells_reader_selftest.R`
all pass.

## Reviewers

| reviewer | when | why |
|---|---|---|
| **fpl-code-review** | twice — on the derived-grid-label commit, then on the whole branch | guard tests, a `ReviewWatchedPaths` tree, and a source scan whose failure mode is a false pass |
| **fpl-stats-review** | on the branch | the chip block hands columns to an inference layer; the question of what a clustered SE means on a bound is a claim about evidence |
| **fpl-findings-audit** | twice — with the earlier commits, then on the final `CLAUDE.md` rewrite | the resident index is edited in three places, and the audit is the only thing that reads it against the code |
| **fpl-docs-review** | on the branch | `stats/README.md` and a long doc-comment record |
| per-item reviews | dispatched by the implementing agents | each item's own diff |

**Skipped, with reasons — a skip is a decision:**

- **fpl-security-review.** The branch touches **no credential handling** (there is none; its absence
  is guarded), **no cache**, **no agent tool layer**, and **no write path**. `internal/fpl` and
  `internal/agent` are byte-identical across the range — the key above records their digests
  unchanged from the previous record. The one edit that *looks* adjacent is the six agent
  definitions, and it only **removes** names from an allowlist, so it cannot widen a surface.
- **fpl-run-review** — no live run wrote config.
- **fpl-season-maintenance** — the four hand-maintained lists are untouched.

Self-review was not substituted for any of these. The final commit's own `CLAUDE.md` rewrite was
audited by a fresh **fpl-findings-audit** instance before it was committed, and that audit is the
source of five of the corrections in §1 below.

## Findings, ranked by how misleading the state was

### 1. A declared invariance was recorded as a null result about chip timing (applied)

The worst item on the branch, because it inverts what a number means rather than getting one wrong.
`CLAUDE.md` read *"Scoring-chip timing moved nothing — `+0.000` on `POLICY` and all three held
rungs, 36 cells."* But `mustNotMoveForAxis(AxisChipWeek)` **declares** those columns must not move
and the harness checks them cell by cell on every run; `AxisChipWeek` reads a finished season's
per-week gains through a `func(gains []int) int` and plays no chip. Byte-identical output is what
the axis is *required* to produce. Reading it as evidence about timing is reading a passing
assertion as a measurement, and it made the cell count look like it carried information.

**The deeper defect is that the cell count was standing in for a data state.** An earlier edit had
deleted the re-measurement caveat on the ground that the figures "carry their own data state in the
sentence (the cell count is in it)", which was already wrong when written — `cff7260`'s own message
names the **data state** as the mover.

Applied: the bullet now says the invariance is an invariance, gives the levels their own data state
(`d249d8a`'s six-season tree, cells never banked) separately from the one *banked* run
(`stats/snapshots/2026-08-12-4d61058/cells/oraclechip.*` — 24 cells, four seasons × six starts, at
`b6c6aa2` with `dirty,true`, an ancestor of the snapshot's own key that does not contain the
archive-repair commit `7cb769e`), and states that the levels are unbanked sums over the two scoring
chips and functions of two **asserted** bars.

⚠️ **The first draft of this correction was itself wrong in five places**, all overclaiming, and all
caught by the audit and re-verified against the source before applying:

| draft claim | what the code says |
|---|---|
| "returns every collected column" | `cellMetricColumns` is **eight**; the ten chip-reading columns and `oracle_kind` are collected and *required* to differ by arm |
| "the 36 cells is unsupported by anything in this repository" | **unbanked, not unsourced** — `d249d8a`'s message records `+0.000` in all 36 across two runs on one tree |
| "the output is oracle minus the bar" | minus the **first week clearing** the bar (`firstClearing`, falling back to the final week) |
| "pooled" glossed as over chips | the source means pooled over **entry points**, which is why the GW1 column is quoted apart |
| a flat "nothing has been re-measured" | true only **under the banked schema**; a flat version erases the levels' own data state |

The audit also positively settled the one thing the printer alone could not: `d249d8a` records
timing at `+3.8/+4.5`, which sums to the `+8.3`, so "sum over the two chips" is confirmed rather
than inferred. That is now in the entry, with "never halve it".

⚠️ **No standard error was added, and the entry now says why one must not be.** A dispersion *was*
recorded once (SE 1.25, t 6.6) on an unbanked run. Both differences are `≥ 0` in every cell **by
construction** — the oracle is `max(gains)` and both baselines are elements of the same slice — so a
t against zero is mechanical, the status this record already gives the perfect armband's 20.4. The
entry records the figure *and* its disqualification, which is the opposite of an upgrade.

### 2. A guard's blind spot was described as the reverse of what the guard says (applied)

The branch added *"Both Go guards match an idiom, never a name, and each records by name the live
near-misses it cannot see."* Two defects. **"Each" is false**: `copies_test.go` names two live
near-misses, while `median_test.go` names four *escaping spellings* and then records that none
exists in the tree today, so its blind spot is **empty** — the opposite claim. And **"both Go
guards" had no safe referent**: the only guards named in that bullet are
`TestTheSharedCellQuantitiesHaveOneImplementation` and `TestTheCopiedExpressionsHaveOneImplementation`,
and the first is a Go test that scans **R**. The median guard,
`TestTheMiddleValueHasOneImplementation`, was never named at all.

Applied, and then corrected again by the audit: **both** scans carry an *empty* escaping-spelling
list, and what only the copies scan carries is a second and different category — two **live**
near-misses it matches and excuses. The shared-cell guard is also **not name-only**: it scans R
function names *and* the raw table-read idiom by shape, because the two worst readers defined
nothing at all.

### 3. Two adjacent chip bullets both quoted 13.3 for unrelated quantities (applied)

Not an error, but one bullet apart. The timing bullet's is the GW1 column of `oracle − threshold`;
the preparation bullet's is the bench channel's season figure. The second is now disambiguated —
and, on the audit's finding, **named with its estimand**: it is `per_gw × 38`, where `per_path`
reads `+9.0`, a 32% disagreement the record's own "name the estimator" rule wants stated.

### 4. The resident index budget had no generator, and its headroom note was arithmetically impossible (applied)

The comment read *"~220 bytes, down from ~730 on 2026-08-15: closing the guard item took ~310"*, and
`730 − 310 ≠ 220`. Each end was right when written and the sentence was wrong anyway: `~730` was the
headroom at `891378b` (93,479 bytes), `6021401` then took the file to 93,676 without touching the
comment, and `~220`/`~310` are measured from that unrecorded intermediate.

**A difference is false the moment either end moves and nothing notices.** The comment now quotes a
**size with the commit it was taken at**, which `git show <commit>:CLAUDE.md | wc -c` can check.

And the figure now has a generator: `fi.Size()` was printed **only on failure**, which is how the
paragraphs above came to reason from sizes nobody had recorded. A `t.Logf` before the check emits it
on every run.

**Budget raised 92 KB → 97 KB**, per the test's own instruction to raise rather than trim. The claim
named as needing the room is finding 1 — the chip bullet's invariance/level separation, plus the
audit's five corrections to it.

### 5. Two disclosures redacted from dated review records (applied — a user decision)

Two committed records named a private store this repository may not name: a **reviewer-roster row**
in `reviews/2026-08-15-xpoints-conversion-scale/review.md` and a **commit-summary row** in
`reviews/2026-08-15-the-squad-page-rebuild/review.md`.

The standing exemption for already-committed disclosures was ruled to be a **grandfather clause over
an enumerated set only**; these two were found afterwards, so they are cleaned at the acknowledged
cost of amending a dated attestation. Both rows are **kept** — the reviewer still ran, the commit
still did what it did — and only the name is gone, replaced by a label that reads as nothing to an
outsider. Each file carries a **dated redaction note** so the amendment to a dated record is itself
attested rather than silent. No finding was altered.

`TestNoTrackedMarkdownCitesAWikilink` carries per-file exemptions keyed by line and hash; neither
file is in that list, and the full suite was re-run after the edit in case a line shift reached
anything.

### 6. Six agent definitions carried dead MCP tool names (applied — a user decision)

Each of the six tracked `.claude/agents/fpl-*.md` files listed three `mcp__*` tools on its `tools:`
frontmatter line and nowhere else. That server has been removed from this machine entirely and those
tools no longer exist in any session; the store is now reached through ordinary file tools. Every
one of the six already carries `Read, Grep, Glob, Bash`, which is what that needs.

Removing the names removes **dead references, not capability** — and it can only narrow an
allowlist, never widen one, which is why this does not owe a security review. All six frontmatters
were re-checked: delimiters intact, `tools:` well-formed, nothing else touched.

## What was declined, and why

Without this section the same findings are re-raised every pass.

- **Blank the chip block on POLICY-incomparable seasons at the Go level.** Declined. 2019-20 is
  upstream of every chip reading (unlimited free transfers), but the blanking belongs in the R
  blanklist, where the other instruments already do it. Doing it in Go would put the same exclusion
  in two implementations, which is this record's signature failure.
- **Strengthen the argmax mediator.** Declined. `reportChipCells` already errors if the bench-boost
  oracle picks one week in every cell. A stronger mediator was proposed and is not obviously
  correct; it wants its own thinking, not a drive-by.
- **Fold `xiValueOfParts` and `captainAndVice` into their neighbours.** Declined **deliberately**,
  and this is the one most likely to be re-proposed. `xiValueOfParts` is a documented copy whose
  load-bearing role was re-checked and still holds. `captainAndVice` sits on the replay's scoring
  path where **tie order is load-bearing**, so folding it is a behaviour change wearing a
  deduplication's clothes. Both are now *named* in the guard's comment as matched-and-excused, which
  is the right outcome: recorded, not hidden, not silently collapsed.
- **Two of three named guard targets.** Removed from the item for having **no duplicate**.
  `PriorSeasonName` and `xiValue` are already single, each with one implementation and delegating
  wrappers. Saying so is the deliverable, not a shortfall.
- **`FindPrevious` re-keying (item D2).** Left **blocked**, not done. Only **7 of 76** snapshots
  carry a key, so switching the lookup now would silently change which baseline the next snapshot
  diffs against — a change of comparator disguised as a refactor. It unblocks when coverage is
  broad enough that the two keyings agree.

## What could not be checked on this harness

- **Nothing here was measured on points, and nothing should have been.** No constant moved, no arm
  was run, and no cell was banked. Every figure quoted in the `CLAUDE.md` edits is a figure that was
  already in the record; the edits change what is claimed *about* them — their data state, their
  aggregation, their bars — not their values. **`HOLD` and `POLICY` are both untouched by
  construction**, since `internal/analysis` and `internal/backtest`'s non-test source are
  byte-identical across the range.
- **The chip levels cannot be re-derived from this checkout.** No banked file carries the oracle
  block, the schema that would carry it has had no sweep run under it, and
  `stats/sweep_inference.R` does not read the new columns. **A re-sweep is owed**, and it should be
  expected to produce *different* numbers rather than confirm these — the tree has moved since
  2026-08-14. That is a prediction, and the record now labels it as one.
- **The six agent definitions are not exercised by any test**, here or anywhere. Their frontmatter
  was verified by reading. A malformed allowlist would fail silently at dispatch time, which is why
  the edit was kept to deleting list entries rather than rewriting the line.

## Appended after merging `origin/main` — what the merge changed, and what it did not review

`origin/main` moved by sixteen commits while this branch was in flight, landing the clean-sheet
regressor refit. **That work carries its own record at `reviews/2026-08-15-the-clean-sheet-regressor-refit/`
and is not re-reviewed here.** What follows covers only what the merge itself decided.

**One conflict, in `internal/snapshot/notes_test.go`: both branches raised the resident-index budget
concurrently.** Neither figure survived — 97 KB was measured against a file without the clean-sheet
entry, 100 KB against one without the chip entry, so both were correct and both were obsolete before
either landed. Resolved by keeping **both entry blocks in full** and raising to 106 KB for the merged
105,655 bytes. The rule that constant enforces is *raise it and name the claim, never cut a qualifier
to fit*, and a concurrent raise is the first case where honouring it required keeping two claims
rather than one. Neither entry is a summary of the other, so neither was compressed.

The predecessor sentence "Headroom is ~730 bytes. The two blocks above still stand." was **removed
deliberately, not lost**: its first half is the difference-quoting form this branch retracts by name
and was already stale when written, and its second half is restated in the merge entry.

**One real defect the merge surfaced, and it is the label guard working as designed.**
`internal/backtest/csregressor_test.go` printed `all six seasons` as a literal while iterating
`sweepPairNames()`, which `FPL_SWEEP_SEASONS` moves — so a four-season run would have printed a
six-season population. That file landed on `main` independently and never passed through
`TestPrintedGridLabelsAreDerived`, which this branch adds. It now derives from the bound slice.
**This is the first instance the guard caught that was not planted as a test of it**, and it is
category (a) — a live label, not a recorded result — so it was derived rather than exempted.

**What the merge did NOT do.** It did not re-render the accuracy snapshot: `internal/analysis` and
the non-test source of `internal/backtest` are byte-identical to `origin/main` across this merge, so
`2026-08-15-ecebfcc` still covers the model, and `TestSnapshotCoversTheCurrentCode` passes without a
regeneration. It did not re-run any sweep, and it moved no figure on either side.
