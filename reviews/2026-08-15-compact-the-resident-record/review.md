# Compacting the resident record

## What was reviewed

A requested compaction of `CLAUDE.md`, plus three pointer updates forced by a section rename.
Uncommitted working-tree change against `63eff6f`; no commit range.

The instruction was to remove retractions and historical accounting and "only list what is", and
to pitch the prose at an intelligent college student rather than a subject-matter expert.

**The editorial rule actually applied**, because the instruction needs an interpretation and the
interpretation is the reviewable part:

- **Cut**: dates, commit SHAs, "this used to say X", "corrected on <date>", retraction chains,
  tallies of how many times a class of mistake has recurred, and the archaeology of which figure
  superseded which.
- **Kept**: current epistemic status — "unresolved, not a measured loss", "suggestive, not
  established", "a bound, not a measurement", threshold-versus-effect comparisons, cell counts,
  data-state caveats, and concentration warnings ("73% of it is 2025-26 alone"). These read like
  hedges but they are the current status of a live claim, not a record of a past correction.

That line is the whole of the judgement here, and `TestTheResidentIndexStaysSmall`'s own comment
predicts the failure mode: compression and overstatement are the same operation past a point, and
the qualifier is the first thing to go because it costs bytes and looks like padding.

Sizes: 105,655 → 69,108 bytes after the first pass, then → 78,518 after applying review findings.
956 lines against 1,219. The 106 KB budget was **not** changed; it does not bind, and pinning it
just above the new size is the ratchet its own comment forbids.

## Which reviewers ran

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | yes | owed by triage (`CLAUDE.md`/`docs/`). Briefed to read the NEW file standalone, without the diff, so its lens was independent |
| **fpl-docs-review** | yes | not strictly owed, but this change is 100% prose and the before/after comparison is the only way to see a lost qualifier. Briefed on the diff |
| fpl-stats-review | no | no measurement was run, no figure was computed, and no constant moved. Every number here is transcribed from the file it replaces |
| fpl-code-review, fpl-security-review | no | the only non-prose change is one string literal inside a test's failure message |
| fpl-run-review, fpl-season-maintenance | no | no live run; the four season lists are untouched (the `DefaultRestPlayers` change is to how the record *describes* it, not to the list) |

Both reviewers were dispatched concurrently and given deliberately different lenses so the second
was not a re-run of the first. No replay was running.

## What was applied

Both reports converged on four majors, all applied:

1. **A null was rewritten as a confirmation.** The clean-sheet bullet's headline became "The
   clean-sheet factor `f` is calibrated", where the body gives `f` = 1.0476, t 0.30 — a failure to
   separate. This is exactly what the file's own standing rule "A null is a tie, not the refutation
   of one" forbids, and the neighbouring MDE (0.424 against a candidate of 0.173) says the fit could
   not have seen the candidate either way. Retitled to "does not separate from 1", with the power
   caveat restored inline.
2. **The canonical 39-point threshold lost its grid and its estimator.** Restored to
   "39 on the four-season grid, season-clustered (start-fixed: 32; six-season arithmetic ≈ 26)",
   in the glossary and in "What the harness can resolve". Corroborated against `docs/accuracy.md`,
   which carries the same decomposition. This is the file's most-quoted bar and the compaction had
   left it reading as a current-grid figure.
3. **The 67-point bonus figure was stripped bare.** Restored both caveats: it is an absolute total
   from a contaminated era with 66% in one season, and the "leaning further in is worse" half of
   the closed line, which had vanished entirely — leaving a reader free to re-open the weight sweep.
4. **The contamination events.** Restored as three bullets rather than the original table: the
   doubles direction (+115, so earlier totals *understate*), the two over-stating fixes (7-14 and
   ~31 a season), the zero-penalty bug's unevenness (28 on 2023-24, 113 on 2024-25) and what it
   manufactured, and the defcon-visibility confound (−95 on 2025-26, one of four changes). Two
   surviving bullets depend on these — "73% of it is 2025-26" and "two seasons carry the swing" —
   and without them the paragraph asserted "it invents shapes" with no instance behind it.

Also applied, each a clause or two: the minutes floor retitled (it asserted inertness and then said
inertness is unmeasurable, and "the point estimate" had no antecedent); "resolves 12.7" corrected to
"resolves **at a threshold of** 12.7, on an effect of 0.436 pts/gw"; "two switches" corrected to
three; the 89% gate bar reconciled with the bullet 55 lines later that calls it grid-dependent;
`min_gain`'s ladder given back its cell count, its metric and its 0.4 baseline label; the
`FPL_NO_STARTS_REPAIR` null labelled a simple-effect null with its two conditioning switches named;
`OracleLineups`' ≈93 given its data state; the vice-captain bullet reordered so the caveat governs
the verdict; the residual arm's LOSO stability labelled arithmetic rather than evidence; the
schedule screen's inability to test its own motivating example (`BONUS`) restored; "do not re-run
at the refitted constants" restored; the `XGC90 × def` fit re-labelled a bound that cannot localise;
Fieller glossed; `sig_season/perfect` expanded; two different `+13.3`s disambiguated; "11 of the 15
are one COVID-rescheduled round" restored; and the "absence is not evidence" caveat restored.

**One finding is a real bug in the record, inherited rather than introduced.** Both the old and new
files said `restFactor` multiplies by `rest_minutes_factor` (0.83). It does not: `metrics.go`
prorates across the horizon, so at a 5-gameweek horizon with a 2-gameweek window the applied figure
is **0.93 at GW1**, 0.97 at GW2, 1.00 from GW3 — and the code comment records that the unprorated
version already cost a flagged player minutes across two months of fixtures. Corrected, with the
proration spelled out. This sits in the one part of "Season maintenance" that is live on the scoring
path, which is why it matters.

## What was verified, and how

The skill's rule — do not let a reviewer's report become the finding — was applied:

- **Read in source**: the `restFactor` proration and its call sites; `cellMetricColumns` has exactly
  eight entries; the VICECAP row (`sig_season` 12.7, mean 0.436, `mde_season` 16.6, df 3, metric
  `policy`), which is what makes "resolves 12.7" a threshold quoted as a result; three backfill
  switches, not two; "11 of the 15 are one COVID-rescheduled 2020-21 round"; `docs/accuracy.md`'s
  estimator decomposition behind the 39.
- **Checked against `HEAD:CLAUDE.md`**: every figure re-inserted from the reviewers' quotations of
  the prior record — −4.7/−0.9, 96% of `both` is 2021-22, thresholds 23/16/20, 66% of the 67,
  "two cells of one season", 2.36% of starter slots, "one season carrying 68%", −0.583. All nine
  present in the original. **Transcribed, not re-measured.**
- **Guards**: `go build`, `go vet`, `gofmt -l`, and `go test ./...` all clean. The retraction
  scanner passes, and no retracted literal survives the removal of the ⚠️ markers that used to
  cover them.

## What was declined

- **Lowering the 106 KB budget** to bind on the new size. `TestTheResidentIndexStaysSmall`'s comment
  argues at length that a budget pinned to the current size is a ratchet that makes deleting a hedge
  free and adding a rule expensive. The headroom is now large, and that is the intended direction.
- **Renaming the section back.** "Where the research record lives" became "What has been measured",
  which is what the section is. Three live pointers named the old title — a test failure message and
  two `docs/` files — and all three were updated instead, because a pointer should follow its target.
- **The audit's proposal to restore the contamination events as a four-row table.** Three bullets
  carry the same four magnitudes and the same directional warnings in less space, and the brief was
  to compact. The information is not reduced; the presentation is.
- **`fpl-stats-review` on the restored figures.** Both reviewers suggested handing several numbers
  to it for confirmation. Declined because it would be answering a question this change does not
  ask: no figure here is new, and each was checked to be a faithful transcription of the file being
  replaced. If any of them is wrong, it was wrong before this change and is owed a re-measurement,
  not a review.

## The merge, and why this record was re-keyed rather than a new one written

`origin/main` moved while this branch was in review, gaining `95ce989` — the snapshot baseline
fallback refusal. Merging it in moved `internal/snapshot`, which this record's key did not cover,
so `TestReviewCoversTheCurrentCode` fired on the merge. The key below is regenerated over the
merged tree.

**No new review was owed, and this is the reasoning rather than a shortcut.** The two changes
touch **zero files in common** — checked, not assumed: `git diff --name-only` over each side
intersects empty. Mine is `CLAUDE.md`, two `docs/` files and one test string; the incoming side is
`cmd/fplagent/snapshot.go`, `internal/snapshot/values.go` and two of its tests. The incoming
commit carries its own review record at `reviews/2026-08-15-snapshot-baseline-fallback/`, so both
halves of the merged tree are reviewed; what was unreviewed was only their *combination*, and with
an empty intersection there is no combination to review.

⚠️ **That argument is about file overlap, not about semantics**, and it would not hold if either
side had changed a shared quantity through different files. It does not here: nothing in the
incoming commit reads `CLAUDE.md`, and nothing in this change reaches `FindPrevious` or the
snapshot baseline. The merge is textually and semantically a union.

Full suite re-run green on the merged tree.

**A second merge followed, and this one did conflict in `CLAUDE.md`.** `76a4f3f` added a
seven-line qualifier to the `BankLookahead` bullet — that the null is a simple-effect null, that
it was not taken at shipped config because `BANK` and the reach map set `WeeklyXI` where
`runPolicySweep` does not, and that nothing counts how often `shouldBank` fires, so the zero may
be a comparison that did not run.

**Resolved by keeping the incoming text whole**, on the same rule this whole change was written
under: that qualifier is *current epistemic status*, which is the category this compaction
preserves, and it is the strongest form of it — a mediator warning against reading a zero as a
tie. It was not compressed to match the surrounding register. Nothing of the incoming commit was
dropped, and the bullet it attaches to survived the compaction intact, so it landed where its
author meant it to.

## What could not be checked on this harness

- **Every quantitative claim in the file.** The evidence sits outside this repository, so nothing
  here re-derives a threshold, a t, or a points figure. Both reviewers state the same limit. What
  was checked is internal consistency, agreement with the code the file names, and faithfulness to
  the text being replaced — never whether the underlying run supports the number.
- **Whether any deleted caveat existed only here.** Material cut as historical accounting is
  irrecoverable from this checkout apart from git history, which does hold it. Anything a future
  reader needs was restored above; the rest is deliberately gone, which is what was asked for.
- **The register goal.** "Reads like it was written for an intelligent college student" is not
  testable. The glossary of CR2, Holm, MDE and argmax, and the inline glosses for `defcon`,
  xPoints and `S_eff`, are the concrete part of it; both reviewers called the glossary the single
  best change in the diff and neither wanted it traded away for space.
