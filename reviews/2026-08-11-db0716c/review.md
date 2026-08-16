# Review record — the vice-captain supersession and its bisected attribution

**Commit range reviewed:** `0104d9d..02e815f` — `fe5f87b` (marks the two research notes with the
attribution) and `02e815f` (carries it into `CLAUDE.md`, the index every session loads). Previous
record: [`2026-08-11-497c20d`](../2026-08-11-497c20d/review.md).

## Reviewers dispatched, and why

Triage: a change touching `CLAUDE.md` and `docs/` owes **`fpl-findings-audit`**. It ran.

Skipped with reason: `fpl-stats-review` — the change contains no new statistical claim, only a
re-measurement of an existing comparison and an attribution; the audit was asked to flag anything
needing a statistician and flagged exactly one item, recorded below as deferred.
`fpl-code-review` — no source changed. `fpl-security-review`, `fpl-run-review`,
`fpl-season-maintenance` — nothing in scope.

**This was the second review owed in a row on documentation changes.** The first was recorded as a
reasoned skip. Taking the skip twice running is how a gate stops meaning anything, which is why
this one was dispatched even though the change looked safe. It was not safe: the audit found an
error in the index bullet that made the record *worse* than before the change.

## The headline: the change I made to `CLAUDE.md` was wrong, and the audit caught it

`+0.4102` is a **four-season** figure. The shipped default grid is **six** seasons, where the same
test reads **+0.4590** on `HOLD` and **+0.4313** on `POLICY`. All five bisect probes were 48-cell
runs — two arms over 24 cells — so none of them touched the shipped grid.

I verified this myself from the committed cells before applying: `vice4.csv` gives +0.4102 / +0.2967
over 24 cells and 4 seasons, `vice6.csv` gives +0.4590 / +0.4313 over 36 cells and 6 seasons.

**The index bullet was correct before I edited it.** It read `+0.46`, which rounds correctly for the
shipped grid. I replaced it with a four-season number labelled as the shipped one — walking into the
exact coincidence that `CLAUDE.md` already warns about in the six-season block, written the same
day: *+0.4590 rounds to the recorded +0.46, so a fresh default-grid run reads as a perfect
reproduction while the drift sits on the four.* There are two true sentences here and I merged them.

## Second: the stated mechanism did not exist in the code

The change claimed the mechanism was "exact" — that the fallback fires on a modelled probability and
`c76c0d8` changed the model's belief about a captain blanking. Verified against source, neither
holds. The rule fires on a **realised** fact (`replay.go:242` reads `g.Minutes == 0`), and `c76c0d8`
did not touch `playsAtAll` or `appearanceFactor` — it changed `blankRate`, whose consumers are the
bench-slot weights and `defconPerGameweek`. The channel is **squad selection**: the estimator that
prices the bench moved which fifteen was bought, hence who wore the armband.

That is still a direct mechanism. It is not the one written down, and the word "exact" implied a
directness the code does not support — the failure class this audit was dispatched to find.

## Findings applied

1. Grid mislabel, in both files and the index — the most misleading item.
2. `CLAUDE.md` asserting both that the drift is attributed and that it is not, ~400 lines apart.
3. Mechanism rewritten from "belief" to "selection", with the separability test named (defcon
   scores in 2025-26 alone, bench weights bite every season, so the per-season shape of the −0.043
   distinguishes them at no replay cost).
4. The **POLICY** twin marked: it was measured all along and nobody looked. +0.2967 on four seasons,
   +0.4313 on six; and "clears significance on both standard errors" is **marginal on the historical
   grid** (t = 2.50 against the 3.182 that three degrees of freedom demand) while holding on the
   shipped one. `HOLD` moves the other way and is *stronger* under CR2 than the retired estimator
   said.
5. The unmarked re-measurement at `harness-and-inference.md:421` stamped — genuine, taken before
   `c76c0d8`, and its **invariance conclusion is untouched**; only the levels are stale.
6. Config-hazard fix credited to the wrong commit (`7baf5b4`, which touches no config loading);
   corrected to `7a4645b` + `b6c6aa2`, two commits closing two different halves.
7. "Figures recorded after it are unaffected" narrowed — unaffected *by this estimator*; the
   backfill and the grid widening both land later and both move figures.
8. The dated line replaced with a mechanical one: `git merge-base --is-ancestor c76c0d8 <commit>`
   plus the enumerable affected set. A date cannot separate — `c76c0d8` is 01:03 on 10 August.
9. "Nothing else in the range touches that channel" marked as **not verified**, with the six files
   outside my commit filter named — `reconstructstarts.go` especially, since it feeds `StartShare`,
   the input to the *old* `blankRate` rule. What licenses ignoring them is the +0.002 bracket, not
   the filter.
10. "About 81%" softened to "about four fifths" with its ±2-point rounding stated; "nothing further"
    corrected to "+0.002, inside the rounding".
11. Config harmlessness scoped to the **read set** rather than the two files, which differ on
    `roster` and on review rules.
12. Paragraph break restoring the `MinutesWeight` sentence to its own paragraph.

## Declined, or deferred, and why

- **Deleting any superseded figure. Declined**, per the marking convention: superseded text is kept
  and stamped so the idea is not rebuilt.
- **Twelve further sites quoting the superseded figures** (the `12.7` family in the canonical block,
  four `harness-and-inference.md` sites, two `TODO.md` lines, one test comment). **Deferred with a
  TODO entry.** They are real and each wants a pointer rather than a rewrite; the canonical block's
  clause is the load-bearing one because everything defers to it.
- **The `armband pinned` row (+1.233) and the thresholds derived from it.** **Unmeasured, not
  unmeasurable** — the only item here needing a run, and a small one (same two-arm sweep, different
  metric rung). This is the item the audit referred to a statistician; deferred rather than guessed.
- **The pre-registration/verdict positive-control sign convention** (`gridwidth_test.go:60-62`
  describes a different arm ordering from the one the published verdict used). Deferred: it is a
  note on a pre-registration, and pre-registrations are annotated rather than edited — it belongs in
  the same postscript that already exists there.

## What could not be checked by reading

Only the armband row above. Everything else in this record was settled against files, source, or
the committed cells, and the two decisive numbers were recomputed independently rather than taken
from the audit's report.
