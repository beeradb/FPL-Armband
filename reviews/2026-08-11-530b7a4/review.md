# Review record — the second merge, and a findings audit applied

**Commit range reviewed:** `f05b391..530b7a4`. Four things are in scope and they are separable:

1. **the merge `a84b6a3`**, which brought origin/main's rank-objective work onto this branch and
   moved six watched files without a review record;
2. **the findings audit applied at `310093a` and `b9c7d46`**, which is most of this record's
   substance;
3. **the snapshot regenerations at `4eb3cd2` and `8956190`**, which are invariant checks rather
   than changes;
4. **`530b7a4`**, which applies the two HIGH findings of the statistical review of (1) — including
   one defect this session introduced while fixing another.

`058890f` and `a601800` touch only `reviews/` and move no watched file.

## The branch was red, and the merge commit said it was green

`a84b6a3`'s message ends "`go build`, `go vet`, `gofmt -l` and `go test ./...` green on the merged
tree." **That was false when written.** `TestReviewCoversTheCurrentCode` fails at `a84b6a3`, and it
fails for the ordinary reason: six watched files changed with the merge and no record covered them.

**The cause is an ordering trap worth naming, because it is repeatable and it caught this session
twice.** The suite was run *before* the merge was committed. At that moment HEAD was still the
pre-merge commit, so the guard's `git diff sha..HEAD` was empty and the gate passed. Committing the
merge is what created the failing state, and nothing re-ran after it.

Both guards in this package diff **committed** history — `sha..HEAD` — so neither can see working-tree
changes. A suite run before the final commit therefore tests a tree that does not yet exist. The
same trap fired again inside this very session, in the opposite direction and entirely
independently: a full `go test ./...` over my uncommitted edits reported only the review gate
failing, and `TestSnapshotCoversTheCurrentCode` failed the instant the identical changes were
committed, because `swaps.go` and `unified.go` are in a watched tree.

The remedy is ordering, not tooling: **run the suite after the final commit.** A guard that reads
committed history cannot be made to see anything else, and it should not be — the alternative is a
gate that passes on work nobody can retrieve.

`a84b6a3` is deliberately **not** amended. Renaming it would change its sha and therefore the
directory name of the record covering it, which is circular. The claim is corrected here instead,
which is where a reader looking for the state of the branch will be.

## The audit was wrong about the guard, and right about everything else

The audit that produced this work gave a contributing cause for B1: that `newestReview` picks the
newest record by **directory name**, so the incoming line's `2026-08-11-9fe14b5` sorts before this
branch's `2026-08-11-f05b391` despite being the later commit, and that it will mis-order again on
the next merge.

**Checked, and it is not true — in three separate ways.**

`newestReview` (`internal/snapshot/reviewgate_test.go:143-173`) calls `newestByCommitDate`, and
falls back to lexical ordering only when git can resolve *no* record at all. So the selection is not
by directory name.

The premise is also inverted: by commit date `f05b391` is `2026-08-11T13:09:57` and `9fe14b5` is
`13:04:11`, so `f05b391` **is** the later commit, not the earlier one the audit assumed.

And the claimed mechanism could not have produced the observed failure even if it were the
implementation. Lexically `f` > `9`, so `2026-08-11-f05b391` is the maximum of the whole `reviews/`
listing — the fallback would have selected exactly the same record. Both orderings agree here.

There is therefore no mis-ordering, no latent defect, and nothing to fix in the guard. The failure
was simply that nobody wrote a record for the merge.

This is recorded at length for two reasons. Writing a false mechanism into a permanent artefact is
worse than the stale figures this record exists to remove — a wrong mechanism invites a "fix" to
code that is correct, and this package's own history shows how long such a thing survives once
written down. And **an audit treated as infallible is the same failure as a review record nobody
re-reads**: the audit was specific, well-evidenced and right about all five of its other items, and
that is exactly what makes a single wrong diagnosis in it dangerous.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** | the merge introduced a new inference script (`stats/rank_robustness.R`), a new verdict in `CLAUDE.md` and 346 lines of `harness-and-inference.md`, none read on this branch | core argument verified sound; **2 HIGH findings applied**, 5 recorded and declined — see the addendum |
| **fpl-findings-audit** | `CLAUDE.md`, `TODO.md` and `docs/` changed | ran ahead of this record; its findings are the H/M items below |

**Not dispatched, with reasons.** `fpl-code-review`: the merge added no Go code, and this branch's
own Go changes are comments plus one test file — the snapshot below proves no computed figure moved.
`fpl-security-review`: nothing touches `internal/fpl`, `internal/agent`, config persistence or the
cache. `fpl-run-review`: no live run. `fpl-season-maintenance`: no hand-maintained list moved.

## The invariant came first, and it is the strongest evidence here

Following this package's own rule — *what quantity must this change not move, and is that tested?* —
the answer is that **a comment-only change must move no measured figure at all**. That is checkable
rather than arguable.

`4eb3cd2` and `8956190` regenerate the accuracy snapshot, against `a0fdddd` and `310093a`
respectively. In **both**, the entire "figures that moved" table is one row — `stamp.commit` — and
`constants.csv` is byte-identical. Every calibration-drift, clean-sheet, transfer-error and
prediction-benchmark figure reproduces exactly.

Note the staleness guard demanded these regenerations for files whose diffs are **pure comment**.
That is correct behaviour and should not be "improved": the guard cannot distinguish a comment from
a constant, and "this edit was only prose" is precisely the judgement call this repository has
watched rot four separate times. The cost is about thirty seconds each.

The workflow lesson is the cheap one: **batch prose edits to `internal/analysis` and
`internal/backtest` into a single commit.** The guard is per-commit, so splitting them across two
cost a regeneration each — which is why there are two near-identical snapshots in the series rather
than one.

## Findings applied

Every one was reproduced independently before it was acted on.

### H1 — the merge created a live retracted figure, in a file no audit had read

`stats/README.md` closed its `rank_robustness.R` section by deriving the verdict from the Spearman
**−0.977** — a figure the *incoming* line had itself retracted as an algebraic identity recovered by
simulation. The ⚠️ was added to the `P(flip)` bullet ~25 lines above and this closing sentence was
missed, so the file simultaneously warned about the figure and concluded from it.

This is the defect the merge created, and the mechanism is worth keeping: the pre-merge grep covered
only *this* branch's four retractions (`+322`, `+183`, `+5.6`, `+10.8`) and would never have caught
one of the incoming line's. **A textual auto-merge needs both lines' retraction lists, not one.**

The closing sentence now states the verdict from what actually supports it — no arm with `|t| >= 1.72`
flips under a random reweighting to 5x — and carries an explicit ⚠️ explaining why the Spearman is
not evidence, pointing at the computed reweighting (−0.315 across 60 specifications) as the stronger
test that the verdict does survive.

### H2 — `+183` was live in four Go sites, and the guard could not see any of them

The correct position: `+183` measures a **season-average window**. The arithmetic is right and the
*label* was wrong — it never bounded knowing a player's trajectory. The bounded arms are **≈73**
(lineups, CR2 t = 1.32) and **≈47** (minutes, t = 0.62), and **neither resolves**.

| site | what it said |
|---|---|
| `internal/snapshot/retracted_test.go` | the `now:` field for `322` taught `+183` as "the quantity that does resolve" |
| `internal/capture/capture.go` | package doc, "the only information oracle that resolves" |
| `internal/backtest/availabilityoracle_test.go` | file header |
| `internal/backtest/prediction_test.go` | **printed** it in diagnostic output |

The first is a repeat offence and its own comment predicted it: a stale `now:` makes the guard teach
the position it exists to retire. The general point is now written beside it — **a `now:` field is
prose inside a test, so nothing checks it and it rots exactly like the record it polices.**

Also corrected while there: the availability oracle's figure is **≈14** against a common baseline,
not the `+16` three of these sites carried, which came from the earlier baseline (585 moves against
595).

**Guard entries added** for `183`, `+5.6`, `+10.8` and `0.977`, each with mandatory `context` terms
and, where a live quantity shares the literal, an `unless`. The `183` entry excludes lines saying
"season-average" or "window", because the record legitimately *tabulates* that arm — what is
withdrawn is the claim it bounds the trajectory, not the measurement. One existing passage in
`docs/notes/transfer-policy.md` was reworded rather than exempted, so the marker word sits on the
line carrying the figure.

### H2a — the guard now reads Go source, which is the gap that let this recur

Not in the audit's brief, and the strongest thing in this change. All four H2 sites were invisible
to `TestRetractedFiguresAreNotQuotedAsCurrent`, which read only `CLAUDE.md`, `TODO.md` and
`docs/`. **A comment justifying shipped behaviour is a stronger claim than a line in the research
record, not a weaker one** — `swaps.go` explained why a correction exists using three figures that
had all been withdrawn.

The scan now covers `internal/` and `cmd/` (and `stats/*.md`). It found **four further live
retracted figures with zero false positives**:

| site | figure | note |
|---|---|---|
| `internal/backtest/unified.go:207` | `0.53` | **shipped code** — justified the per-move threshold with a retracted per-player over-rating |
| `internal/backtest/transferpolicy_test.go:468` | `0.53` | called it "the strongest of the shipped group" on evidence that has since reversed |
| `internal/backtest/availability_test.go:295` | `+273` | correctly-argued prose with no marker word |
| `internal/analysis/appearance_test.go:55` | `0.1112` | same shape |

The mandatory context term is what keeps it quiet — `swaps.go` contains a `322` that is a transfer
count, and it does not fire. The last two were already *reasoning* correctly and simply lacked a
marker; they were reworded rather than exempted, on the principle that the fix for correctly-annotated
prose failing a guard is to annotate it in the words the guard knows.

`unified.go` is the one that matters: its rule is **kept**, because the consistency argument (two
searches must not apply one constant at different granularities) never needed a size, and because
the rule is a documented no-op at shipped settings. Only the retracted justification is gone. **No
behaviour changed.**

### H3 — `TODO.md` carried the figure as a task premise

Three sites, at ~23, ~490 and ~588. The middle one instructed the reader to "**Cite that**"; the
last used it to size the open "Filters and durations, none measured" item. A superseded figure
surviving as a *premise* is worse than one misinforming a reader, because it sets what gets built
next — which is the argument the retraction test's own comment makes for having `TODO.md` in scope
at all.

All three now carry the bounded arms with their t values and an explicit ⚠️ against reinstating
`+183`. The third is rewritten to say what the family actually supports: not "the prize is sized"
but an **ordering** — lineups above minutes, so selection matters more than minute-level precision —
while still noting it exceeds the transfer gate (bounded at 106 and closed), which is the comparison
that justifies spending effort there.

### M1 — `swaps.go` stated three retracted figures as current measurements

In shipped library code, justifying `discountIncoming`. Corrected to the shipped-config values:
buy side **−0.230** median / **+0.079** mean, sell side **−0.282** median, buy-minus-sell **+0.051**.

**The correction is more than a number swap, and the file now says so.** The winner's-curse
asymmetry this code was built on **is not present at shipped config** — the incoming player is
marginally *better* calibrated than the outgoing one, the opposite of the predicted sign. What does
survive is much larger and is a different mechanism: a sold player who keeps playing is −0.100 a
gameweek against **−2.223** for the 13% who stop. The sell-side error is about who *disappears*.

The `1.72` premium over-valuation is now **+1.242 with SE 1.019 at t = +1.22** on 23 rows. The
captain-doubling mechanism is arithmetically real and is kept; what is gone is any measured size for
it. `buyDiscount` already ships at zero and the later half of the file already recorded that
correctly — only the opening justification was stale, which is the characteristic shape here: a file
gets a correction appended and its premise left alone.

### M2 — the chip-timing arithmetic credited a rule with hindsight

`CLAUDE.md` ~479 and `docs/notes/chips.md` ~474 credited a **threshold rule** with **+30** a season
for playing the two scoring chips. The table twelve lines above gives oracle − median = **30.7** and
threshold − median = **20.8**. The rule was being credited with the ~10 points of *timing* that the
same paragraph calls unreachable.

Re-derived from the table's own rows (bench boost 19.8 / 14.8 / 4.2, triple captain 20.6 / 15.7 /
5.5): the rule's row is 10.6 + 10.2 = **20.8**, and it captures **68%** of the oracle, not the
"three quarters" claimed. Both files now quote the rule's row and say explicitly why the oracle's is
not reachable.

## The incoming rank-objective work

A statistical review was dispatched over `stats/rank_robustness.R`, the `harness-and-inference.md`
section it produced, and `docs/rank-objective-handoff.md`. **Its findings are recorded in the
addendum at the foot of this file**; what follows is my own reading, arrived at independently while
applying H1, and it should be read as the reviewer's *brief* rather than its conclusion.

The headline — *scoring the replay on rank reorders only what was already unresolved* — arrives
having already survived two self-retractions on its own line: the Spearman `−0.977` (an identity
recovered by simulation) and a scope-block alarm (a first-order-expansion artifact). What remains
supporting it is the computed reweighting from ownership marginals, 60 specifications, with `HOLD` —
the metric this project uses for scoring constants — flipping in 0.0-10% against 0-78% on `POLICY`.

⚠️ My draft of this paragraph also said "no arm above `|t| = 1` changes sign", which the addendum
shows is false. It is left corrected rather than deleted because the mistake is the finding: I read
the section, wrote H1 against it, and reproduced its wrong summary twice without ever checking it
against the table directly above it.

The structural argument is the part to trust and it is not statistical: percentile is a monotone
function of points and the field does not react to our policy, so **no individual paired cell can
change sign** and rank-scoring is *exactly* a reweighting. That is why the verdict does not depend
on the retracted correlation.

**The reading to guard against**, and the reason this section is here rather than accepted silently:
"reorders only what was already unresolved" is a statement about *arms below `|t| = 0.6`*, which are
the ones that do flip. It is not a licence to treat those arms as settled. See below.

## Do not treat this record's nulls as settled

Stated explicitly because several figures in this change are nulls, and a null written down
confidently is how the last round of over-reading happened.

**Invariance results survive and are real**: byte-identical figures under a comment-only change, the
`0.0`/`0.4` min_gain no-op, `HOLD` at exactly `0.000` where a seam held. Those are arithmetic.

**Significance-based nulls are unresolved, not refuted.** Specifically:

- the **availability arm at ≈14 against its own threshold of 14** — that is p = 0.0497 restated, not
  a second witness; it fails Holm at 0.149 and is inert in 13 of 24 held cells, so ≈14 is a design
  average against ≈32 conditional on firing;
- the **two bounded minutes arms** (≈73 at t 1.32, ≈47 at t 0.62), neither of which resolves in
  either direction;
- the **anchored-chip arms**, which additionally sit on **provenance-dirty cells** — so "anchoring is
  worth nothing" is not established, and only the `+10.8` measurement is refuted.

The wording applied throughout prefers "does not resolve" over "is worth nothing", and the new
`+10.8` guard entry says so in its `now:` field, so the next reader to grep for it is told the
distinction rather than the verdict.

## Verification

Run **after** the final commit, which is the whole point of this record's first section:

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

The accuracy snapshots at `stats/snapshots/2026-08-11-310093a/` and
`stats/snapshots/2026-08-11-b9c7d46/` are the evidence that none of it changed a computed figure.
`530b7a4` touches only `stats/` and `docs/`, which are review-watched but not snapshot-watched, so
it needs no third regeneration.

---

# Addendum — statistical review of the incoming rank-objective work

`fpl-stats-review` read `stats/rank_robustness.R`, both write-ups, the `CLAUDE.md` bullets and the
handoff, and ran short R checks against the checked-in fixture. **Its verdict on the core argument
is that it is sound**, and it verified the exact parts rather than taking them: `R_crit = P/N`
matches a brute-force adversarial optimiser to four significant figures on all eight arms; the
coefficient-of-variation figures (0.199 at R=2, 0.455 at R=5) reproduce; `Phi(−|t|/s_e)` matches the
20,000-draw simulation to 0.73pp, confirming the Spearman retraction *algebraically* as well as
numerically; and the sqrt-wk sign invariance holds in 8 of 8.

**Two HIGH findings, both applied at `530b7a4`.**

### The summary sentence contradicted the table nine lines above it

`docs/notes/harness-and-inference.md` asserted "**No arm above `|t|` = 1 changes sign anywhere in the
box**". Its own table gives `POLICY` `half-life 2` at `|t|` = **2.90** with an exact-percentile flip
share of **18.3%**. The next sentence compounded it: "the three arms that flip" — the table shows
**six** with a non-zero share (78.3, 75.0, 53.3, 18.3, 10.0, 1.7). Three flip in a *majority* of the
box, which is presumably what was meant.

This is the load-bearing claim, not a wording slip. The Spearman was withdrawn as an identity, so
the computed-reweighting box is what the headline now rests on; and `POLICY half-life 2` is named
elsewhere in the same section as one of "the two arms this record actually leans on", worth −59
points a season.

**I propagated it myself.** Fixing H1 above, I copied that sentence into `stats/README.md` as the
"stronger test" the verdict survives on. So the session's own correction reintroduced the defect it
was correcting, one paragraph away — which is the sharpest available demonstration of why the
retraction guard exists and why a figure's *summary* needs checking against its table, not just its
provenance.

Corrected in all four live sites (`harness-and-inference.md`, `CLAUDE.md` twice,
`stats/README.md`, `docs/rank-objective-handoff.md`) to the statement that actually holds: **no
`HOLD` arm above `|t|` = 1 flips**, with `HOLD` at 0.0-10% against 0-78% on `POLICY`. The defence is
structural in the **metric**, which matters because `HOLD` is what this project judges scoring
constants on — and the section's own closing paragraph already said so.

`reviews/2026-08-11-316ae4f/review.md` carries the sentence too and is **deliberately left alone**: a
review record is a record of what was believed at that commit, and editing one retroactively is a
worse failure than the stale sentence.

### `rank_robustness.R` blocked on `sweep` alone

Every other script in `stats/` blocks on `(run_id, sweep)` — `sweep_inference.R:248`,
`shape_inference.R:572`, `variance_components.R:527`, `entry_density.R:501` — and
`internal/snapshot/cells.go:152` documents that key explicitly. This one used the sweep label only,
and keyed cells on `(season, start_gw)`.

The cells file is **append-mode by design**, so one file can legitimately hold two runs of a label.
Pooled, each arm reads twice the rows, `n` doubles, the SE shrinks by √2 and every `t` inflates by
41% — with cells paired against the wrong run's baseline, since `bm[a$cell]` takes the first match.
Silent, and it produces a plausible table. That is precisely what
`internal/backtest/cellcsv_regression_test.go` was written to prevent.

Fixed, with a guard for an absent `run_id`. The fixture reproduces its recorded table unchanged —
means, `t`, `R_crit` and sqrt-wk all identical, with only the documented Monte-Carlo drift in the
last printed digit of `P(flip)`.

## Findings recorded and not applied

Judgement calls, listed so the next reader inherits them rather than rediscovering them.

- **The headline generalises from 11 arms of two sweeps to "no verdict in the record".** The section
  is honest about this; the `CLAUDE.md` bullets state it unqualified. The gap is concrete: the
  fixture-load transfer seam (POLICY t = +2.64) and the unified-search verdict (POLICY t = −2.52)
  are POLICY-judged shipping decisions in the same band as the one POLICY arm that flipped. Not
  rewritten here because narrowing the headline properly is a claim about scope that wants its own
  pass, and this record's change is already large.
- **Percentile is the mildest rank objective.** Under a threshold payoff — beat a rival, top-10k —
  the monotone argument still holds but the reweighting is `{0, ∞}`, and `R_crit` runs 1.14 to 6.06,
  so an arbitrary threshold objective could flip every arm. "Closed for evaluation" should read
  "closed for percentile scoring".
- **The sqrt-wk column is called "exact" and is not** — it assumes `p = 0.5` where the same document
  brackets `p >= 0.62`. The reviewer swept the family `weeks^(1−p)` for `p` in [0.5, 1] and **no arm
  changes sign at any exponent**, so the conclusion is safe and only the word is wrong. Corrected in
  the script header; the two prose sites are not.
- **No `--selftest`, and the defence for omitting it is now weaker than it was.** `stats/README.md`
  argues the only exact quantity is `R_crit`. The note since derived a second, `P(flip) =
  Phi(−|t|/s_e)` — and a selftest pinning the simulation against it **is the check that would have
  caught the Spearman claim before it reached the record**. Three smaller things it would cover:
  `flipprob` reports 100% when the mean is exactly zero, since `sign(0)` matches no non-zero sign;
  the byte-identical `next` prints its "invariance check passing" line only when *every* arm is
  identical, so a mixed sweep drops those arms silently; and there is no required-column check.
- **"Exactly a reweighting" and "weights independent of the data" cannot both be leaned on.** By the
  mean value theorem the exact weight is `f(ξ)` with `ξ` *between* the two arms' scores, so the
  weights are data-dependent by construction. Only the computed-percentile calculation is free of
  the tension — a further argument for moving it into the script.

## One more reason not to read the nulls as settled

The reviewer makes a point that sharpens the section above, and it is the most useful thing in the
report. "Reorders only what was already unresolved" is being heard as "rank scoring is safe". For an
arm recorded as *indistinguishable* a sign change overturns nothing — no direction was claimed. But
**this project routinely acts in the `|t|` ∈ [0.5, 2) band on mechanism or sign-consistency
grounds**: `min_gain` returned to 0.4 at t = 1.63, `DecisionHorizon` 3 is queued at t = 1.56, the
early wildcard ships as a prior at +14 to +23. Those are exactly the arms rank-scoring reorders.

**"Unresolved" is not the same as "nothing was built on it."**
