# The four surviving `docs/replay.md` accuracy findings

## What was reviewed

Applying the four findings that survived a prior accuracy pass over `docs/replay.md`. The other
five were discharged by a concurrent branch that merged at `b452d44`; this branch is the remainder.

Branch `worktree-apply-the-replay-doc-findings`, cut from `main`. Files changed: `docs/replay.md`,
`scripts/replay`, `CLAUDE.md`, `stats/README.md`. **No executable line changed anywhere** —
`scripts/replay` is comment-only, verified below.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| **fpl-findings-audit** | ran — triage row *"`CLAUDE.md`, `docs/` — the record only"* |
| **fpl-code-review** | ran — the `scripts/replay` comment defects, which the source note assigned to it explicitly |
| fpl-stats-review | **skipped, and it is the one to re-dispatch.** Two findings below turn on figures whose adjudication is a measurement, not a reading: whether the superseded xppilot SE percentages can be re-derived on the shipped instrument, and whether "five of six" or "six of six" is right. Neither is settled here; both are marked in the page as unsettled rather than resolved |
| fpl-security-review, fpl-run-review, fpl-season-maintenance | not applicable — no credential surface, no live run, no season lists |

## Findings, ranked by how misleading the current state was

**1. The first draft quoted SUPERSEDED figures and upgraded them. APPLIED.** The 20-25% and 30-60%
standard-error cuts, and the "five of six contrasts" count, are **pre-conversion-scale xppilot
figures, superseded 2026-08-15 and never re-run**. Three places say so — `internal/analysis/xpoints.go`,
`internal/backtest/gatexpoints_diag_test.go` (which prints *"take them from a re-run, not from
here"*), and `internal/snapshot/notes_test.go`. The draft added *"the record is emphatic about it"*,
which upgraded them further. **The percentages are removed from the page; the direction and the
closed-line verdict are kept, because they rest on *removing variance removes signal*, which a
rescaling does not reach.** ⚠️ **Root cause is not in this diff: `CLAUDE.md` carries these numbers
unmarked** — `grep -n supersed CLAUDE.md` returns nothing. Recorded, not fixed here.

**2. "five of six contrasts" does not reproduce. APPLIED as a marker, NOT resolved.** The committed
`stats/snapshots/2026-08-15-xppilot/inference.txt` shows **six of six** on both estimators; on arm
levels `|t|` *rises* on two of four. The count's population is stated nowhere. The page now says to
read the direction, not the count, and names the contradiction. **Adjudicating it is
`fpl-stats-review`'s and is not done.**

**3. `hits` was glossed as points. It is a COUNT. APPLIED.** `internal/backtest/gate.go`: `Hits` is
how many moves are paid for with a −4; the points charged are `4 × hits`. A factor-of-four error in
a gloss written for a non-specialist — the worst kind, because the audience cannot catch it. Also
corrected: the **column order** (the xPoints pair precedes `moves`/`hits`), and that both are
`POLICY`-side only.

**4. The described mechanism was not the mechanism. APPLIED.** xPoints is a **residual**, not a
re-scoring: `xPoints = points − residual` over **four** channels, with every other channel left
realised — which is what lets it span three bonus regimes. And `ConversionScale` has two fields,
`Goals` and `Assists`, so the scale prices the attacking half only; the clean-sheet half is replaced
but unscaled. The draft attached the in-sample caveat to a scale the reader believed covered
everything. The **BPS leak** is now stated too: about a quarter of the removed conversion luck
returns through realised bonus for an attacker (corr 0.606, slope 0.252, n 12,104).

**5. The fix was partial — `CLAUDE.md` and `stats/README.md` still said "execs the binary". APPLIED.**
Both reviewers flagged this independently as most severe: correcting the script and the detail page
while the *authoritative* file said the opposite converts one wrong statement into a contradiction
that a reader resolves in favour of `CLAUDE.md`. The desynchronised-mirror class, created by the fix.
Both corrected in this commit.

**6. "three of the guard rails depend on not exec-ing" is wrong. APPLIED.** Traced one at a time: the
slot semaphore **survives** an exec (an `flock` on an inherited fd, released at exit), and the memory
cap is unaffected (`--scope` binds either way). What is lost is the printed peak RSS entirely, plus
the diagnosis half of the exit status — and a leaked timer file. The count also collided with a
*different* "three guard rails" in `CLAUDE.md`. **The page now refuses to give a count and says to
read the script**, which is that page's own standing rule about counts.

**7. `exec` in the script would confuse a reader told it does not exec. APPLIED.** Six `exec` calls
exist, all `exec {fd}>file` with no command word — file-descriptor redirection, not process
replacement. Both the page and the script now say so explicitly.

**8. The `elapsed=` citation and the OOM string. APPLIED, and independently confirmed.**
`grep -rn 'elapsed='` over `2026-08-14-blend/` and `-blendlo/` returns nothing; the timings are
prose in each `FINDINGS.md`, and `elapsed=` lines *do* exist in other snapshot directories, which is
what made the citation look checkable. The wrapper prints `KILLED BY SIGNAL 9 — this is an
out-of-memory kill, not a test failure` and never the string `OOM`; the page now says to grep for
`SIGNAL`.

**9. Two pre-existing `scripts/replay` defects in the block being edited. APPLIED.** Not part of the
four, surfaced by the review, and fixed because both are the same class and one is consequential:

- *"the measured peak, which is 0.9-1.0 GB per run"* was contradicted 55 lines below — those five
  numbers were measured while the wrapper still shelled out to `go test`, and that gigabyte **was**
  the driver, now gone. Real peak is 89-142 MB. **The stale figure made the 2G soft cap read as ~2x
  headroom when it is nearer 20x**, so nobody tuning it would suspect it of being decorative.
- *"Arguments are passed through to `go test`"* is false — flag *spellings* are translated for a
  compiled binary; anything outside the list reaches it raw and is rejected. `CLAUDE.md` repeated it
  verbatim; both corrected.

## What was declined, and why

- **`stats/README.md`'s CSV-schema table has no xPoints row.** Proposed by the audit and **declined
  for scope**: this branch is the `docs/replay.md` findings, and the schema table is a second
  document's structure. ⚠️ **Recorded so it is not re-raised blind** — it is a real gap, and the two
  files now each read as a schema authority.
- **`CLAUDE.md`'s unmarked superseded percentages.** Declined here: marking them is an edit to the
  research record's own verdict text, which is `fpl-findings-audit`'s to propose against `CLAUDE.md`
  directly, not a side effect of a docs branch. The page now carries the marker instead.
- **Re-deriving the xppilot SE cuts on the shipped instrument.** Declined — it is a **run**, not a
  reading. Left as a marked open question rather than silently dropped.
- **`"understated a real concurrent run sevenfold"`** (`scripts/replay`, echoed in `docs/replay.md`).
  Against the file's own table 1031/70 is **14.7x**; the only 7 in the file is 1031/145 = 7.1, which
  looks like the number that got reused. **Declined** because the right value depends on which
  baseline the sentence meant, and guessing would replace one unsourced figure with another.
- **Several cosmetic reflow and cross-reference nits.** Declined as noise.

## What could not be checked on this harness

- **Whether the superseded percentages would reproduce on the shipped conversion scale.** Not
  unmeasured by accident — the code states it cannot be re-measured by any sweep since 2026-08-15,
  so this needs a fresh pilot, not a re-read.
- **Whether "five of six" or "six of six" is correct.** The banked file says six; the source of five
  is untraceable to a population. Only a re-run with a stated estimator settles it.
- **No detection threshold applies to anything in this record** — every figure here is a file count,
  a byte count or a transcription check, not a measurement of football.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` (empty), `bash -n scripts/replay`, and
`go test ./internal/snapshot/...` all pass. `scripts/replay` verified comment-only two ways by the
reviewer: zero non-`#` lines in `git diff -U0`, and comment-stripped files byte-identical either side.
