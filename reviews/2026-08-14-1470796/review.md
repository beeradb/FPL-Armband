# The schedule screen, and two ways I lost information while trying to save space

Covers `2f0610e..bb9936b` — `stats/schedule_screen.R`, the `stats/cells_common.R`
refactor, the two new source-scan guards, and the record changes that came with them.

## What was built

TODO.md queued a "genuinely free" R-only screen: print each arm's mean paired difference
per entry gameweek on `HOLD` over cells already banked, and flag any constant whose
ordering across settings reverses between the early and late columns. It exists now as
`stats/schedule_screen.R --committed`, with results in
`stats/snapshots/2026-08-14-schedule-screen/`.

**The screen was free; the constant it was written for was not.** `BlendRateK` has no
banked cells — the `want("BLEND")` block exists and was never run to the archive — so the
headline question needs a 4-arm × 36-cell replay. Third instance of the class TODO.md
already names, after `BandStrength`.

**Nothing resolves, and the design guaranteed that.** Per-ladder MDE is 152 to 349 season
points against this grid's own median detectable of 39.

## Reviewers dispatched, and why

The change touches `stats/*.R` (harness and inference), `CLAUDE.md` and `docs/`, and it
asserts a byte-identical refactor. The triage table's union is **fpl-stats-review,
fpl-findings-audit and fpl-code-review**, the last pointed at the differential evidence
rather than at the diff, as the table directs. All three ran concurrently on committed
state.

**Not owed:** fpl-security-review (no credential, cache, agent or config-persistence path
is touched — this is R and markdown), fpl-run-review (no live run wrote config),
fpl-season-maintenance (no hand-maintained list touched, though a correction below lands
on a stale *pointer* to one).

⚠️ **fpl-code-review ran while the tree moved under it**, from `e8200df` to `c78737a` with
`schedule_screen.R` dirty. It handled that correctly — extracting `e8200df` and reviewing
that exactly — and its Part 1 evidence stands because `sweep_inference.R` and
`cells_common.R` were untouched between the two. Recorded because a reviewer given a moving
target usually produces stale findings, and this one did not: its finding 2 is the
duplication `c78737a` had just recorded, arrived at independently.

## The byte-identity claim, verified far wider than I verified it

I ran `sweep_inference.R` before and after on two cell files at the default scale with
plots off, and called that adequate. **fpl-code-review re-ran it on all 58 committed cells
CSVs at both `--scale=per_gw` and `--scale=per_path`, plus three files with plots on** —
99 emitted `inference*.csv` and every PNG byte-identical by md5, the only stdout difference
being the `wrote <path>` echo and the clubSandwich banner. The 17 runs that fail fail
identically in both, with the same messages and exit codes.

**My coverage had two real holes.** The third `diffs_for` call site (`:1144`) is inside
`if (opt_plots)`, which my `--no-plots` habit never reached; and `--scale=per_path` was
untested by me and is the option whose whole purpose is that it is a *different estimand*.
Both are now exercised. It also corrected me on a hazard I invented: `SCALE_SUFFIX` is
defined at `:95`, **before** the `source()` at `:113`, not after — and would have been safe
either way, since `diffs_for_scale` resolves it at call time.

A cross-check worth keeping: the two scripts now agree to 15 decimal places on the same
quantity — `sweep_inference.R`'s HOLD mean and `schedule_screen.R`'s `mean_all` for
`6s-minhl.csv` match exactly on all four arms. That is the refactor's purpose, measured.

## The invariant came first, and it paid immediately

The skill's own first rule is that invariants beat reviewers. The quantity this change
must not move is `sweep_inference.R`'s output, since the refactor moves its paired
difference and CR2 estimators into a shared file. That was checked by running the script
before and after on two committed cell files — one of which exercises the degenerate
(byte-identical HOLD) path — and requiring `inference.csv` to match. It did, and still
does after every subsequent edit.

Then `TestTheSharedCellQuantitiesHaveOneImplementation` was written to stop a *future*
second copy, **and failed on the first run against three existing ones**. `grid_width.R`,
`shape_inference.R` and `variance_components.R` carry their own `diffs_for`, `se_cr2`,
`degenerate` and `as_flag`, and they have already diverged:

- the minimum cells an arm needs to survive is **2** in two copies and **4** in the third,
  so an arm can appear in one script's table and be absent from another's with no error;
- `grid_width.R` drops such an arm **silently**, which is the defect the comment above
  `cells_common.R`'s `diffs_for` says that function exists to prevent;
- `grid_width.R`'s `degenerate` takes a vector where `cells_common.R`'s takes a data
  frame — same name, different signature;
- `grid_width.R` hardcodes `_per_gw` and cannot express `per_path` at all.

So CLAUDE.md's "four instances" of one-quantity-two-implementations was an undercount.
Migrating them changes what they print, so the guard carries an explicit `knownCopies`
list that must **shrink** — and fails if an entry's copy is gone while the debt is still
listed, so a migration cannot leave the record overstating what is owed.

**No reviewer found this.** It cost one source scan.

## Findings applied

Ranked by how misleading the state was.

### 1. I re-compressed two qualifiers the budget test's own comment names as prior offences

CLAUDE.md was **already 213 bytes over budget at `2f0610e`** — the test fails on `main`
— so this branch inherited a red test and treated clearing it as its own job. Four blocks
were moved out properly and verified against their destinations. Still over, I then
compressed the **vice-captain drift chain** and the **fourth-cluster figures**: two of the
four instances `notes_test.go`'s comment already records from the data-repair branch, in a
comment whose whole subject is that ratchet.

What the vice compression cost, to **no file at all**: `+0.4302` (today's four-season
shipped value), the correction that `c76c0d8` alone is **129%** of the net drift, and
"the two backfill terms are single-season bookkeeping rather than causes". And the
compressed bullet pointed the reader at `scoring-model.md`, whose ledger still said
*"about four fifths of the drift"* — the phrasing the deleted clause existed to retract.

**My check was a grep I misread.** I ran `grep '0.4302\|0.4210\|c76c0d8\|129%'`, got three
hits, and read that as confirmation; all three were `c76c0d8`, a commit hash that appears
throughout the note for other reasons.

Applied: budget raised to 68 KB with the binding recorded and the claims named, per what
that comment says to do at a second binding; the hedges restored; and
`scoring-model.md`'s ledger corrected — including two rows labelled "repair off" and
"repair on" that made the `FPL_NO_XGC_REPAIR=1` corner read as the shipped value.

**The transferable lesson is not "compress more carefully".** It is that "check the
destination carries it" must be a grep for the **figure**, not for the block.

### 2. There was no power figure, so "Holm 1.000" said almost nothing

The screen reported p values and a "factor from resolving" and never said what the null
excluded. Computing the MDE — which the script now does rather than my doing it by hand —
gives **152 / 234 / 154 / 229 / 267 / 349** season points per ladder against this grid's
own median detectable of **39**. The tie was close to pre-determined by the design, and a
`BonusWeight`-sized schedule is *below the screen's floor*, which is the honest headline.

Also applied: this record's prior that an interaction is the **cheap** contrast (CR2 SE
0.216 against 0.599) does **not** transfer. That holds for a difference of differences
*within* one cell, which cancels path divergence; this one is formed *between* cells,
across entry points that are different squads scoring different football.

### 3. "Factor from resolving" was an inversion — withdrawn

I introduced a `t_crit(df)/|t|` column. It is arithmetically exact and the wrong quantity:
`1/|t|` has undefined expectation, so under the null it is large precisely because the
estimate landed near zero. `MINW`'s "79.6" said nothing about `MINW`. Ranking six ladders
by it ranks them by which drew nearest zero, and it invites the reading this record
forbids — that a big number means "definitively nothing". Replaced by the MDE, which is a
statement about the instrument.

### 4. The `BONUS` refusal is a coincidence of label text, not a principled test

Both write-ups said the screen refuses `BONUS` "because the family is two-dimensional".
The code implements no such test: `ladder_of()` parses the first number from each label
and refuses on duplicates, and `BONUS` falls out only because `0.5 / 1.5` and `0.5 / 2.0`
both parse to `0.5`. **A 2-D family whose first coordinates happened to be distinct would
be accepted silently and a slope with no referent computed.** That is this record's own
*naming the consumer is the check*, turned on its author.

Applied: the screen now prints *which* rule fired, the stated reason is corrected
everywhere, and the real fix — a numeric `setting` column from the sweep — is queued. Same
cause, second casualty: `TEAMFORM`'s baseline carries no number, so its ladder runs
unanchored on three positive doses and cannot see the on/off step its recorded result is
about.

### 5. The strongest candidate was measured through the split the screen criticises

`benchshape` was reported as "+2.1 pts/gw at GW1, about zero from GW6 on, raw p 0.058 and
0.077". Three things wrong: the p values are the **2/2/2 late-minus-early contrast**, a
different quantity from the +2.1 they sat beside and drawn from the very split the same
document criticises for straddling the `BonusWeight` boundary; "about zero from GW6"
ignores a middle dip (−0.70 and −0.94 at GW11); and "the two fixed tuples" is an argmax
over two.

Re-measured as GW1-against-rest, which is the shape's own contrast: **+2.441, +2.468 and
+2.242** on 5 df — **all three non-flat arms agree, the shipping derived one included** —
so it is a single coherent statement rather than a pick. Still post hoc, Holm still 1.000.

### 6. `ANCHORED` is a theorem for a different reason than I gave

I wrote that all three excluded sweeps are "confined to `decide()`, which HOLD never
calls". True for `min_gain` and `HITS`. For the chip calendar the mechanism is that
**HOLD plays no chips at all** — `HoldCaptaincyWeekly` never touches `Chips`, `Chips2` or
`ChipPlanner`. Right verdict, wrong consumer named, two paragraphs from where this record
states that rule. Verified directly before applying.

### 7. Four defects in the new script, all reproduced before fixing

From fpl-code-review. **None changed a single reported number** — re-banking after the
fixes produced a byte-identical `screen.csv` — which is why they are dangerous rather than
loud.

- **`state_of` read the provenance file per *file*, not per *sweep block*.** The schema is
  `sweep,run_id,key,value` and a cells file can hold several blocks with different commits:
  `runA-minhl-shipped.csv` holds three (`59f0830`, `c6190c3`, `9daefda`). The old code
  filtered on `key` alone and took `commit[1]`, printing **MINHL#1's commit under MINHL#2's
  numbers** and repeating the env string once per block — quoting the wrong data state while
  citing the rule that says not to, with no error. The committed set is all single-block, so
  the banked run was correct by luck. Fixed and verified: the three blocks now print their
  three commits.
- **Every committed provenance file records `dirty,true`, and `state_of` never read it.**
  The screen was printing `commit b400ec3` as the data state for a run made from a dirty
  tree. Now flagged as `DIRTY TREE (commit does not identify the code)`.
- **A block with no usable arm vanished from the report while the header still counted
  it.** `if (length(arms) == 0) next` fired before anything was written, with `quiet = TRUE`
  suppressing the note `cells_common.R` prints for exactly this. Reproduced on
  `runA-minhl-shipped.csv`, whose `MINHL#1` carries only a baseline: the header said "3
  sweep(s)" and two appeared. Now reported as `NO USABLE ARM`.
- **None of `sweep_inference.R`'s contract checks were present.** With a second arm flagged
  `is_baseline`, `diffs_for`'s merge pairs every arm against a doubled baseline set and the
  screen printed a full, plausible table of *wrong* numbers — `fixture weight 0.35` at GW1
  reading `0.686` instead of `0.000` — before dying on an opaque R message, or **exiting 0
  with the wrong table** where no ladder is built. Added: exactly-one-baseline per block,
  `is_baseline == (variant_index == 0)`, and `hold_per_gw == hold_points / weeks`.
  Reproduced with a doctored file and verified it is now refused before any output.

Two smaller ones from the same review: the "ladder slope" label covers a **Spearman rho**
while the schedule test regresses on the **raw setting scale** — different estimands that
can disagree in sign, printed adjacently, with the flag using one and the test the other;
and the Holm family size was printed as `nrow(res)` where `p.adjust` drops NA rows, so a
partially-degenerate sweep would have overstated it. Both now stated and computed.

### 8. Smaller corrections applied

- The estimator is a **one-sample t over seasons**, not a CR2 fit: with one observation
  per cluster the sandwich collapses to `s²/G` and the df is exactly `G−1`. Calling it CR2
  is misleading *here specifically*, because CR2's stated virtue in `cells_common.R` is
  that its df is reported rather than asserted, and this df is asserted by the design.
- The nesting argument is **conditional on the calendar reading** (an exact 0.66 shrink,
  so the size *is* readable after correction) and **vacuous under the evidence reading**,
  where the squads differ by entry. I had stated it unconditionally.
- Non-flatness is **necessary, not sufficient** — "only if", not "exactly when".
- **No transfer constant was screened at all**; for those the question is unasked.
- The family mixes data states and `TEAMFORM` has no provenance file; `MINHL` and `FIXW`
  are exactly the two ladders shown to move across data states.
- `metrics.go:470` had already drifted into an unrelated data literal; the rest-list rule
  is on `DefaultRestPlayers`. Now named by identifier in both files.
- Two blocks inserted mid-paragraph in `archive-and-data.md`; spacing fixed.

## Declined, or applied differently than proposed

- **Pooling the two Holm families into one of 34.** Recorded in `FINDINGS.md` as what the
  next run should do, not retro-applied. Every value is 1.000 either way, and silently
  re-scoping a family after seeing the results is the move this record objects to; the
  right time to choose is before.
- **Migrating the three diverged R copies now.** Declined deliberately. It changes what
  those scripts print — `grid_width.R` would start reporting arms it currently swallows,
  and `variance_components.R`'s 4-cell floor may be a deliberate choice or a typo, and
  nobody knows which. Recorded as debt with a guard that forces the list to shrink.
- **Restoring the four blocks that were moved out properly.** The audit cleared all four
  as lossless against their destinations; only the two *compressions* were restored.
- **Rewriting `ladder_of` to test one-dimensionality properly.** The right fix is a numeric
  `setting` column emitted by `runPolicySweep`, which is a Go change on a branch that is
  otherwise R and markdown. Queued instead, with the incidental nature of the guard stated
  wherever its verdict is quoted. The review audited all 58 archived label sets and found
  **no sweep currently produces a wrong ladder**, so this is a latent hazard, not a live
  error — but two near-misses are recorded: `reach.csv`'s 18 unrelated knobs are refused
  only by a modal-prefix tie resolving to 2 < 3, and `runA-minhl-*` labels its baseline
  `shipped (baseline)` where `6s-minhl` says `half-life 4 (ships)`, so the same constant
  gets a `{2,8,20}` ladder with no anchor on one bank and `{4,2,8,20}` on the other. Those
  two `d(slope)/d(entry gw)` figures are **not comparable**, which the `over N arms at
  settings …` line shows but does not say.

## One sharpening left unapplied, and named so it is not lost

fpl-code-review notes that `FINDINGS.md`'s `BONUS` reproduction rests on the `1.5/1.5` arm:
`1.0/1.0` is negative at every column except GW26 and never flips, so "the flip reads at
GW6" is one arm's behaviour rather than the sub-ladder's. The section already says the
observation establishes nothing and now says explicitly not to re-site the boundary on it,
so this sharpens rather than corrects. Not applied because it is a claims judgement the
statistics review did not adjudicate; recorded here so the next pass has it.

## One reviewer figure did not survive recomputation

The statistics review reported GW1 as "the *quietest* column on the grid", cell-level SD
1.548 against GW26's 3.357. The SDs reproduce exactly (n = 132 per column), **but GW1 is
not the minimum**: GW6 is 1.525 and GW11 is 1.537, the first three are effectively tied,
and the step comes at GW16. The argument survives on the *group* — the candidate rests
outside the noisy columns — and the wording was corrected in all four places rather than
repeated. Recorded because "verify before applying" is the rule and this is what it caught.

## What could not be checked on this harness

- **Whether any of the six ladders is genuinely a schedule.** The MDE is 3-9× the grid's
  own median detectable, so this is *unresolved*, not refuted — and specifically not
  evidence that these constants are flat.
- **`BlendRateK`**, at all: no banked cells. Unmeasured, not unmeasurable — one replay.
- **`BONUS`**, the founding case: the sweep's arms are not an ordered family, so the
  instrument cannot test the example that motivated it. Unmeasured pending a 1-D ladder.
- **Whether the mixed data states change any verdict.** Each block is internally one
  state; the Holm family is not. Nothing resolves under any of them, so it does not bite
  today.
