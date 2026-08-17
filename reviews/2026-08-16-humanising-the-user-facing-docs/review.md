# Humanising the user-facing documentation

## What was reviewed

A rewrite of every user-facing document in the repository, from `c104c3d` (the public
`FPL Armband v1` tip) to the staged tree: the root `README.md`, all eight files under `docs/`, one
line of `AGENTS.md`, and one line of `internal/snapshot/mermaid_test.go`.

**This record was re-keyed once, and the reason is not a rebase.** Public `main` moved to `852bf77`
while the branch was in review, so it was merged in and the gate re-run, per the merge gate's
condition 7. That commit removes a branch-name row from two banked snapshots under `stats/`, which
is a review-watched tree, so the watched digest moved for reasons that have nothing to do with this
change. It touches no file this rewrite touches and the merge was clean. The re-key covers the
merged content; the review below is unchanged by it, and nothing in `852bf77` was reviewed here —
it arrived already on `main`.

The brief was tone and currency, not substance. The documentation had accumulated **91 warning
symbols and 42 dated lines** — retractions of figures, corrections of what a previous version of the
document had claimed, and provenance bookkeeping about deleted snapshot directories. Several
passages argued with themselves across three or four paragraphs, stating a figure and then
withdrawing it. The instruction was to remove that layer, state what is true now once, and write for
a reader who is numerate but has no knowledge of this project's internal history.

Three rules governed it, and they matter for reading the diff:

- **A withdrawn number was deleted along with its retraction note.** Where removing a retraction
  would have left a claim unsupported, the claim went too rather than being propped up.
- **No number was invented.** Nothing in the diff asserts a quantity that was not already in the
  tree, with one exception, corrected below, where a documented figure was found to be wrong and was
  replaced with one derived from the shipped code.
- **Pointers to documents outside this repository were removed.** A reader cannot follow them, and
  the verdicts they carried are resident in `AGENTS.md`.

Diagram count went from 7 to 36. All mermaid styling was then normalised onto a palette derived from
`internal/present/css.go`, the application's own HTML output — crimson `#A8404E`, teal `#1F5F73`,
green `#2F7A57`, amber `#B9770E`, grey `#7A8791`, ink `#141A21`, with accent `#B9762A` for charts.
There is no logo asset in the repository; the owner elected colours only.

## Which reviewers ran

| reviewer | status |
|---|---|
| **fpl-docs-review** | **ran**, on the default model. The single documentation reviewer, and the one whose remit is `docs/` and the root `README.md`. |
| fpl-findings-audit | **skipped** — see below. |
| everything else | **not owed.** The triage table maps this change to the record-only row. No scoring path, harness, agent, client or config-persistence code was touched. |

`fpl-findings-audit` was skipped on an explicit instruction to use a single reviewer for the
documentation. The triage table maps `AGENTS.md` and `docs/` to it, so this is a real deviation
rather than a not-applicable. Two things bound the risk, and neither makes the skip free:

- the change edits exactly one line of `AGENTS.md`, repairing a pointer to a `docs/README.md`
  section this rewrite deleted. It adds, removes and restates no verdict there;
- `fpl-docs-review`'s remit covers the same failure — a retracted figure quoted as current — over
  the surface that actually moved, and it found one.

What the skip leaves unchecked is whether any *verdict* in `AGENTS.md` has drifted from the evidence
behind it. That question is untouched by this change and remains open, exactly as it was before it.

## Findings

Ranked by how misleading the state was before the fix.

### 1. The bench slot weights were figures the shipped code cannot produce — **applied**

`docs/model.md` documented derived bench slot weights of **2.55 / 0.98 / 0.24** outfield and **0.38**
for the reserve keeper. Those are legacy values, fitted under the retired start-share blank rate. The
paragraph that said so was deleted during the rewrite, and the verb was changed to "the derivation
produces", which asserts they are what the code emits.

They are not, and the error is checkable without running anything: `benchSlotScale` is defined as
`4 / (gk + out[0] + out[1] + out[2])` at the reference eleven, so **the four weights sum to 4.0000 by
construction**. The documented tuple sums to 4.15, which is impossible for the quantity the sentence
names.

**Verified rather than taken on the reviewer's word.** A scratch probe against the shipped
`benchSlotWeightsFor`, with the reference eleven built as `benchSlotScale` builds it — eleven members
at `referenceBlankRate`, `ref[0].Position = "GKP"` — returns:

```
outfield=2.4920/0.9201/0.2206  gk=0.3673  sum=4.0000
```

The first attempt at the probe omitted the goalkeeper and returned a keeper weight of 0 and a sum of
3.98, which is what a wrong reference eleven looks like; the corrected probe reproduces the
reviewer's figures exactly. The probe was deleted after use.

The document now reads 2.49 / 0.92 / 0.22 and 0.37, and states that they sum to four by
construction. The correction also strengthens the surrounding argument: the passage claims the
derived weights are "near-identical on the two slots that matter" to a hand-swept 2.4 / 1.0, and the
true 2.49 / 0.92 supports that better than the figures it replaces.

`TestRetractedFiguresAreNotQuotedAsCurrent` could not have caught this. It keys on enumerated
literals, `2.55` is not among them, and adding it would collide with an unrelated live figure in
`internal/analysis/blend.go`.

### 2. The congestion measured-versus-argued split went vague in two files and stayed wrong in a third — **applied**

`TestTheShippedCongestionBlockIsInert` is the authority and its own comment states the split: **five**
of the eight penalties reached 1.00 by measurement — the three European penalties and both
short-rest penalties — and **three** by the argument that an unmeasured multiplier which moves a
score is not neutral while 1.00 is.

The rewrite replaced the explicit split with "several… the rest…" in both `docs/model.md` and
`docs/configuration.md`, while the root `README.md` continued to claim **six of the eight** were
measured. The net effect was that the confidently wrong number survived in the most-read file and the
two documents able to qualify it had stopped doing so. All three now state five and three, and name
which are which.

### 3. The post-break penalty's classification is genuinely unsettled, and saying so was deleted — **applied, as a restored caveat**

The rewrite removed a note recording that the record disagrees with itself about
`post_break_penalty`. The disagreement is real and survives in the current tree: §5 of
`docs/model.md` reports the week after an international break as showing nothing on either channel,
which reads as a measurement, while the test counts it among the three neutralised by argument.

This was **not** resolved, because nothing in the checkout resolves it. Both `docs/model.md` and
`docs/configuration.md` now state the five-and-three split and then say plainly that this one term's
classification is unsettled, that nothing downstream turns on it because the shipped value is 1.00
either way, and that it should not be cited as measured on the strength of either page. Recording an
open disagreement is not the same as the dated retraction bookkeeping this rewrite removed.

### 4. `docs/configuration.md`'s "four hand-maintained season lists" collided with the "three of the four" used everywhere else — **applied**

Both counts are correct over different sets. Inside the congestion block there are four lists — two
campaign maps, two nationality lists — and all four are display-only. Across the whole configuration
the season lists are the two campaign maps, `new_coach_clubs` and `rest_players`, and there it is
three of four, with `rest_players` live on expected minutes.

A diagram added during this change sat directly beneath the sentence and had four nodes in its
neutral group, inviting a reader to map one four onto the other. This is precisely the claim the
project record notes stood false in several places until it was corrected. The blockquote now scopes
its count to the block and gives the whole-config count beside it, and the diagram's lead-in notes
that `new_coach_clubs` is a `role_risk` field appearing there for the same display-only reason.

### 5. `docs/backfill.md` was unreachable from the root README — **applied**

The documentation table listed six documents and the index under the heading "Seven documents and an
index", omitting `docs/backfill.md` entirely. A reader arriving at the root had no path to it. Row
added; the count was already correct.

### 6. The accuracy page's provenance header was contradicted by a section on the same page — **applied**

The header asserted that *every* model figure comes from `stats/snapshots/2026-08-10-27740ba`. The
clean-sheet section quotes figures from `2026-08-15-clean-sheet-2x2/`, which it cites itself. The
pre-rewrite page carried a marked correction covering this; the rewrite removed the marker and the
sentence noting that several figures had moved. The header now says *most* figures come from the
older snapshot and that a re-measured section cites its own.

### 7. A composition of two measured shifts read as a joint measurement — **applied**

`docs/accuracy.md` presented the clean-sheet over-prediction rising from ~3% to ~3.7% without noting
that the second step composes two separately measured shifts rather than measuring them jointly.
`AGENTS.md` is explicit on the distinction. Wording corrected.

### 8. The optimiser ladder diagram's reading instruction pointed the wrong way — **applied**

The lead-in told the reader that each red failure box is one "the layer below it" could not escape;
the boxes sit below the technique whose failure they describe, so it should read "above". The diagram
also branched the exact seeds and funded restructures in parallel where `docs/model.md` numbers them
as successive layers. Both fixed.

### 9. Two claims in `docs/replay.md` kept their wording and lost their support — **applied**

The 4.5-seconds-per-cell budget lost the two banked runs that source it; both directories are
committed and the citation is restored. The strict-parser inventory was stated without its scope —
it is exhaustive for `internal/analysis/sweep.go` only, and `envFloat` in
`internal/backtest/unified.go` is a lenient parser outside it. Scope and the second parser both
restored.

### 10. The mermaid validator did not cover the root README — **applied, with a code change**

`TestMermaidBlocksAreWellFormed` scanned `AGENTS.md`, `docs/*.md` and `stats/*.md`. The root
`README.md` gained three diagrams in this change and none of them was being checked. The file list
now includes it, and the validator reports **36 blocks checked** against 33 before. This is the only
non-documentation edit in the diff, and it is in a `_test.go` file, so it is excluded from the
watched digest by construction.

## What was declined

- **The 29%-versus-28% discrepancy in the ordering improvement.** `docs/accuracy.md` says the model
  orders players 29% better than a five-game average; `AGENTS.md` says 28%. The documented ratio is
  0.427 / 0.330 = 1.294, so 29% is the correct arithmetic and the page is right. Pre-existing, not
  touched by this change, and correcting `AGENTS.md` is a findings-audit question rather than a
  documentation one. Recorded rather than fixed.

- **Restoring the note that `TestEveryNoteIsIndexed` no longer checks completeness in one
  direction.** The reviewer called this a judgement call rather than a defect. It is a standing
  hazard about a store that is not in this repository, `AGENTS.md` already carries the general form
  of it ("a claim that is absent here is not thereby unmeasured"), and `docs/README.md` is reference
  for what the system *is*. Adding it back would reintroduce exactly the class of internal
  archaeology this change existed to remove.

- **A table of contents and a renumbering for `docs/model.md`.** The reviewer is right that a
  1,350-line document with sections numbered 1, 2, 3, 3b, 4, 4b, 4c, 4d, 4e, 5, 6, 7 plus three
  unnumbered trailing sections is hard to navigate. Renumbering would break nine anchors that other
  documents link to, so it is a larger change than this one and wants its own pass.

- **Splitting the two passages the reviewer named as still hardest to read** — the blank-rate
  paragraph in `docs/model.md`'s bench section, and the degrees-of-freedom material in
  `docs/replay.md`'s "What it cannot do". Both are genuine and both are improvements on what they
  replaced. Deferred rather than attempted at the end of a large diff, where a restructuring edit is
  least likely to be reviewed properly.

## What could not be checked on this harness

- **The diagrams have not been rendered.** There is no mermaid renderer on this machine. All 36
  blocks pass the repository's structural validator — balanced subgraphs, balanced quotes, no
  forbidden markup, no reserved node ids — and the ten `xychart-beta` series were checked
  figure-by-figure against the tables beside them. Whether the `gantt` chart in `docs/workflow.md`
  renders as intended, and whether `xychart-beta` is supported by every viewer, is **asserted rather
  than verified**. The `%%{init: …}%%` theme directives on the charts are likewise unrendered.

- **Prose quality is a judgement, not a test.** `TestReviewCoversTheCurrentCode` establishes that a
  review happened over this content and nothing more. The reviewer's assessment was that the rewrite
  achieved its goal, that `docs/backfill.md` and the closing section of `docs/accuracy.md` came out
  best, and that two passages remain hard; the last of those is recorded as declined above.

- **Whether the deleted material included anything load-bearing** was checked by reading the diff,
  not by any mechanism. The reviewer found nothing beyond the items above, and specifically confirmed
  that no other withdrawn figure survived anywhere in the tree — `0.624`, the buy/sell asymmetry
  pair, `0.300`/`0.240`, `1.281`, `9e5e1d1`, the 97 MB and 70 MB memory figures and the 85-second
  per-arm budget were all grepped for on the live surfaces and are gone.
