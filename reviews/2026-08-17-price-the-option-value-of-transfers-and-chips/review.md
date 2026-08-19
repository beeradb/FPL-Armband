# Pricing the four held options on one decaying curve

Reviewed: `c589586..HEAD` on `price-the-option-value-of-transfers-and-chips`, five commits, against
the merge base with `origin/main`. The branch also merges three bases in: `origin/development`,
`score-a-blank-gameweek-as-a-blank` and `census-the-blanks-and-doubles-per-cell`.

## What was reviewed

Four levers built as one change, on the argument that a banked transfer, a wildcard, a bench boost
and a free hit are the same object — an **option whose value decays as the window it can be
exercised over shrinks** — and that all four were priced by constants.

Two Phase 0 repairs, both of which were losing comparisons silently:

1. **`analysis.MoveLimit` clamped the hit allowance to 1 unconditionally**, so an arm at
   `MaxHits: 2` was byte-identical to shipped and read as a null; the funded-pair branch carried the
   same clamp as the literal `hitsNeeded <= 1`. Both now take a configurable `HitCeiling`,
   defaulting to 1.
2. **A chip planner's output went into the FIRST set wholesale**, so in a two-set season a chip at
   or after `ChipResetGW` was refused and `runPolicySweep` logged the cell **infeasible while every
   printed number stayed plausible** — an anchored-chips arm silently lost all six 2025-26 cells.
   `SplitChipSets` routes each chip into the set its week draws from.

Then one shared curve in `internal/analysis/optionvalue.go`, four consumers, five mediators, a
four-column per-cell fixture dose, and a wildcard falsifier that **fired**.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-code-review** | yes | `internal/analysis` (scoring), `internal/backtest` (harness), `internal/config` and `cmd/armband`. Found the unconsumed config block and the dose off-by-one |
| **fpl-stats-review** | yes | the branch quotes measured figures and proposes an arm design; found the attributability defect and the under-determined mechanism claim |
| **fpl-findings-audit** | yes | `AGENTS.md` is edited and two of its claims are made false by this change |
| **fpl-docs-review** | yes | `docs/configuration.md` and a new `stats/findings/` entry |
| fpl-security-review | not applicable | no change to `internal/fpl`, credential handling or config persistence semantics; the config additions are read-only knobs |
| fpl-season-maintenance | not applicable | none of the four hand-maintained lists is touched |

Both reviewers independently confirmed the byte-identical invariant (8 cells, every shared column,
against the merge base), broke all three join proofs on purpose and saw them fail with the right
messages, found no third spelling of the hit ceiling, and found no point-in-time leak in any
trigger. The copied-expression sanction was checked and judged a real distinction.

## Must-fix findings, and their outcome

### 1. The entire `option_value` config block had no consumer — FIXED

Grepping `OptionValue` outside its own file returned the `Config` field, the `Default()` line, the
`Load` backfill and one test assertion. **Nothing mapped it into a `SimConfig` and nothing on the
live path read it**; `OptionValuePolicy.Any()` and `ChipTrigger.Enabled` had no callers at all. A
user writing `"option_value": {"wildcard": {"enabled": true}}` would have got a byte-identical null
from a setting that never arrived — which is the exact trap this block was built to price, and
which `cmd/armband/backtest.go`'s own comment about `BankLookahead` already describes.

Fixed in three places:

- `cmd/armband/optionvalue.go`'s `applyOptionValue` maps all four levers plus the pricing curve
  onto the replay's `SimConfig`, called from `runBacktest`. `HitCeiling` is wired there too — it had
  the same gap.
- The taper reaches its **three** live consumers: `adviseBanking`, the agent's `suggest_transfers`
  tool, and the prompt's stated charge. All go through `config.OptionValuePolicy.FreeTransferCharge`,
  which resolves the switch and delegates to `analysis.TransferHoldFactorFor` — the same call the
  replay's `decide` makes, so the live recommendation and the replayed policy cannot disagree.
- The three chip triggers have **no live decision surface** — there is no "should I play a chip this
  week" command — so `noteUnhonouredLevers` puts a line in the transfer board's notes naming which
  lever cannot be honoured. A limitation stated is a different failure from one hidden.

Pinned by `TestTheOptionValueBlockReachesTheReplay`, in two halves. The behavioural half runs the
mapping with distinguishable constants and checks each landed. The **source** half reads the field
names off `config.OptionValuePolicy` by AST and requires each to appear in `applyOptionValue`'s
body, so a tenth field added and forgotten fails rather than going quiet.
`TestAnUnhonourableLeverIsReportedNotHidden` pins the other half.

### 2. Two numbers in the wildcard verdict were wrong, in three places each — FIXED

(a) **"five to nine players" was the HIT count.** `repairCost` returns `4 × max(0, changes − free)`
and `free` is 2 at the firing week, so `changes = cost/4 + 2` — **7, 11, 8, 10** at a GW1 entry and
**5, 5, 5, 3** at GW16. The sentence was only self-consistent at `free = 0`, and it quoted the GW1
column as general when the mid-season column is roughly half the size. Both reviewers found this
independently.

(b) **"weighed exactly one week in seven of the eight"** — the run gives `WeighedWeeks = 1` in
**8 of 8** at bar 12, and no bar in the table gives 7 of 8.

Corrected at `chiptriggers.go`, `config/optionvalue.go`, `wildcardtrigger_diag_test.go`, and in the
new `AGENTS.md` entry.

### 3. The churn diagnosis was under-determined and recorded as fact — FIXED

The stated cause was "the cost is large in exactly the week the model knows least". But the **GW16
cells fire at GW17 with fifteen gameweeks of data** and costs 12/12/12/4, so information poverty is
not the operative mechanism for half the grid. The competing reading is a **standing gap** between
any held fifteen and a fresh unconstrained argmax over the whole pool — a *level*, non-zero at every
cutoff, rather than churn, which is a *rate*. Both predict firing in week two, and **this diagnostic
cannot separate them**: the rule fires on first consultation and stops, so the cost is never
observed as a series on a fixed squad. The only arm that sees it twice is bar-20, and it reads
**12 → 16 over four weeks and 12 → 24 over two** — flat-to-rising, the standing-gap signature.

The prescription already written ("a repair cost measured against a squad the model still endorses,
rather than a fresh argmax over the whole pool") *is* the level reading, so the prose and the remedy
had been disagreeing about what was measured. Relabelled as a hypothesis, both readings named, the
diagnostic's inability to discriminate stated, and the entry-point contrast recorded as the
identification. **The conclusion — closure stands, lever ships off — is unchanged.**

### 4. The taper could not be attributed as built — FIXED

`OptionDecay(r,h) = r/(r+h)` has **no setting reproducing the flat constant**: as `h → ∞` it tends
to 0, not 1. So the taper lowered the mean charge as well as giving it a schedule, and every rung of
a half-life ladder moved the level too. Measured over a season at `FreeCost` 2.0:

| half-life | mean charge |
|---:|---:|
| 3 | 1.561 |
| 8 | 1.241 |
| 16 | 0.957 |
| 30 | 0.693 |

A factor of **2.3 across the ladder, all of it level** — against a level that has never been varied
in any banked sweep, so it is the untested prior question and a taper arm would be confounded with
it.

**Not pushed back on: the reviewer is right and the confound is larger than I would have guessed.**
Fixed by dividing the decay by its own mean over the option's whole window
(`analysis.MeanOptionDecay`), which is the reviewer's second option. The first — anchoring at season
start — fixes the first week and **not** the mean, so it does not discharge the objection: a monotone
decreasing function starting at 1 necessarily averages below 1. After normalising, the mean charge
on a GW1 cell reads 1.99 / 1.98 / 1.98 / 1.97 across the same ladder.

Two consequences recorded rather than tidied away. The factor now **exceeds 1 early** — that is what
mean preservation means, and a curve that only ever fell would be a discount with a schedule
attached. And de-confounding is exact only over the whole window: a GW26 cell still averages below
flat (1.44 / 1.17 / 0.99 / 0.85 across the ladder), so an arm on this must be read against the entry
gameweek rather than pooled. `TestTheCurveIsMeanPreservingAcrossTheHalfLifeLadder` pins it.

### 5. `AGENTS.md` stated two things the code contradicts — FIXED

Both replaced per the file's replace-don't-narrate convention.

- The `MaxHits`-clamp bullet under *Things that have already bitten* was false in all four clauses.
  Replaced with the ceiling's own entry: what the knob is, that both expressions must move together,
  which tests pin it, and that `DefaultHitCeiling` = 1 is measured at one setting on absolute totals
  and is therefore below the file's own twelve-cell bar.
- The banking bullet's *"there is nowhere to vary it to… the allowance can only move down"* was the
  stated ground for *"there is no configuration on this code in which banking acts for a reason
  attributable to banking"*. **The reason is removed; the conclusion is narrowed rather than
  kept.** The measured inertness is at the **2→3+** boundary (94 of 94 weeks); a two-hit arm reaches
  **3→4**, which no run has touched. So banking now has one untested channel attributable to
  banking, beside the preparation credit already named. Both recorded as unrun.

Added under *Absolute point totals*: the chip-set contamination this branch **repairs** — an
anchored-chip arm losing all six 2025-26 cells while every printed number stayed plausible, with
the note that **no anchored-chip cells are banked anywhere in `stats/snapshots/*/cells/`**, so
affected figures can only be re-measured and never re-derived.

Added to the wildcard closed line: the falsifier result, worded narrowly, decision counts only, with
the entry-point contrast as its identification and no points figure.

`docs/configuration.md`'s *"Not an opportunity cost — a confidence threshold"* is marked as the
disputed classification it is. `max_hits_ceiling` and the whole `option_value` block are documented.

## Should-fix findings, and their outcome

| # | finding | outcome |
|---|---|---|
| 6 | `ConsultedWeeks == 0` does not mean "the lever was off" for the chip triggers — `eligible` refuses the whole season when a plan owns the chip, so a 2×2 crossing placement with a trigger reads all-zero in the planned corner | **Fixed with a counter, not a comment.** `OfferedWeeks` is incremented before every guard; three new cells columns (`*_trig_offered`). The block comment asserting one reading over all five mediators is corrected — it is `ConsultedWeeks` for the taper and the prep credit, `OfferedWeeks` for the three triggers |
| 7 | `PricedWeeks` equals `ConsultedWeeks` by construction and cannot separate "the curve is inert" | **Reworded, kept.** Relabelled a tripwire for the factor arriving as a literal 1 — which is what a deleted join looks like — and the doc now points at `ftv_mean_charge` as the column that can answer inertness |
| 8 | `dose_late_*` one gameweek too narrow; the test enshrined the error | **Fixed.** The opening engine's window is `[start, start+H-1]`, so the first unseen week is `start+H`. `doseOver` and both comments corrected, the test's expectation corrected, and a direct boundary assertion added so the sums cannot hide it again |
| 9 | `SplitChipSets` panics on a week above 38 under an argument naming only the lower bound | **Fixed.** Out-of-range weeks are dropped, with the reasoning that this function routes and `ValidateChipSets` judges. The defect was the argument, not the behaviour |
| 10 | `prep_bench_sum`/`prep_captain_sum` sum a dimensionless weight, not points | **Corrected in place.** The doc no longer claims `× horizon` recovers a point value; the columns are relabelled a *horizon audit* — they diverge from `CreditWeeks/horizon` exactly when `effectiveHorizon` or `anticipate` moves the window, which is a real fact about the arm |
| 11 | "six live shrinkage weights" against a list of seven | **Fixed**, and the `blend.go` note's internal "four times… all three" corrected |
| 12 | the "under 2%" bound above `shouldBank` is wrong | **Fixed.** The relative bound is 100% at `r = 1`, because next week's charge is zero. Replaced with the absolute one — largest one-week step **0.358 points** at GW37 — and the congestion half is recorded as **not bounded by that argument at all** |
| 13 | decay and congestion disagreed about whether this gameweek is in the window; for a chip, the double it is played for raised its own bar | **Fixed at the source.** `loadWindow` takes an `after` bound, `fixtureLoadAfter` exposes it, and `HoldingCongestion` reads `[gw+1, gw+horizon]`. Parameterised rather than copied, so there is still one density function |
| 14 | `DefaultFreeHitBar = 16` inherited across a different quantity | **Recorded, not changed.** The comment now says the two do not share a scale, that "keeping the level constant" holds for the bench boost and not for this, and that `fh_trig_weighed`/`fh_trig_value` answer it the first time the lever runs |
| 15 | the free-hit trigger pays a full `Optimize` every eligible week, ~37 a cell; and rebuilt indexes `pe` already held | **Fixed both.** Gated on `analysis.SquadHasABlank` — a *necessary condition*, since the chip's whole value is fielding players in a round the held squad does not — and the recency and team-form indexes are now shared with `pe`. The filter skips the rebuild, not the reading, so a filtered week is not counted as weighed |
| 16 | the wildcard diagnostic prints no grid and its figure needs two env vars | **Fixed.** `gridLabel` is stamped at the head and beside the result, and any season failing `TransferPathComparable` is called out by name |
| 17 | the dose reads `cur.Fixtures` in full — the same hindsight this record flags on `fixtureLoadFor` | **Recorded as a third trap.** Defensible as a *covariate*, since it is identical across a cell's arms and cannot flatter one; **not** defensible as "what a manager could have planned for", which is what a quoted slope would claim |
| 18 | `benchBoostTrigger` cannot fire in the entry gameweek, undocumented | **Documented as a choice**, with its reason (commensurability with the two levers that sit inside the decision block) and its cost (1 of 13 candidate weeks at a GW26 entry) |
| 19 | `prompt.go` and `brief.go` report `MaxHitsPerWeek` without the ceiling | **Fixed.** Both print `analysis.MoveLimit(0, MaxHitsPerWeek, 0, HitCeiling)`, the effective cap. Pre-existing, and the new knob makes the divergent configuration legitimate |
| 20 | the 78% census figure lives only in Go comments | **Fixed, and the figure was wrong.** It had been **asserted, never run**. Measured: **83% pooled** over the three chips the control places, and **83 / 92 / 67** by chip. `stats/findings/2026-08-17-chip-placement-census.md`, which also records the Phase 0b repair (0 of 24 refused, was 6 of 24) and recomputes the two-set claim on this grid |

Nothing was declined.

## What this change does NOT establish

- **No sweep was run and no points figure is claimed.** The only measured outputs are the wildcard's
  decision counts and the census's placement counts, both of which are arithmetic or decision
  counts rather than replayed points.
- **The mean-preserving normalisation makes a taper arm attributable; it does not make one
  favourable.** The level question — what `free_transfer_value` should be at all — remains untested
  at any value.
- **Every lever ships off**, and `TestTheOptionValueLeversAreOffByDefault` fails if that changes.
  The four switches are independent, with no master flag, because the likely end state is chip
  placement on and banking off — pinned by `TestTheOptionValueLeversAreIndependent`.

## Suite

`go build ./...` and `go vet ./...` clean; `gofmt -l ./cmd ./internal` empty. `go test ./...` gives
exactly two `^--- FAIL` lines, both process gates: `TestReviewCoversTheCurrentCode` (this record) and
`TestSnapshotCoversTheCurrentCode` (the accuracy snapshot, which the branch owner regenerates).
