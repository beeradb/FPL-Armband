# The schedule screen: are any shipped constants schedules in disguise?

**Run 2026-08-14** at `2f0610e`, `stats/schedule_screen.R --committed`. **No replay** — this reads
cells already committed under `stats/snapshots/`. Output in `screen.txt`, per-arm table in
`screen.csv`.

The question, from TODO.md: `BonusWeight` looked structureless in aggregate because two opposite
trends averaged out — harmful from GW1, helpful from GW11 — and the answer was **a schedule, not a
constant**. So print each arm's mean paired difference **per entry gameweek** on **HOLD**, and flag
any constant whose *ordering across settings* reverses between the early and late columns.

⚠️ **Revised after statistics review and findings audit, 2026-08-14. Six of the claims below
changed and two were wrong.** Each correction is marked in place rather than quietly applied.

## The two headlines

**1. `BlendRateK` — the item's own named "strongest untried candidate" — has no committed cells, so
the free screen cannot reach it.** The sweep block exists (`want("BLEND")`,
`internal/backtest/transferpolicy_test.go:805-818`, arms 8/12/16/24) and was never banked: zero
rows carrying `BlendRateK` anywhere under `stats/snapshots/*/cells/`, and no `BLEND#` sweep label
in the archive. Answering the question the item was written to ask is a **4-arm × 36-cell replay**,
not an R script.

⚠️ **`MINK` is not `BlendRateK`.** Its arms are labelled `BlendMinutesK`, the *minutes* prior
strength; `BlendRateK` is the *rates* prior strength, and it is the one the over-confidence
argument is about. Separate fields, separate sweep blocks. A reader skimming the archive for "the
blend k cells" finds MINK and would screen the wrong constant.

**This is the third instance of a class TODO.md already names** — an item scheduled as free whose
data does not exist. `BandStrength` was the second, caught the same way: a constant having been
*swept* does not mean its cells were *banked*. The rule that item states — read the code before
scheduling — needs one word added: read the **archive** too.

**2. Nothing resolves, and the design guaranteed that before the data was read.** This is the
finding the first version of this document buried under a p-value table.

## The power, which must be read before any p value

| ladder | GW1→GW26 swing over its own span | **MDE** | df |
|---|---:|---:|---:|
| `FIXW` | −78.6 | **152.4** | 5 |
| `BENCH` | −87.2 | **234.1** | 3 |
| `MINHL` | +49.8 | **154.4** | 5 |
| `TEAMFORM` | +55.8 | **228.5** | 3 |
| `MINK` | +11.6 | **267.3** | 3 |
| `MINW` | −4.4 | **349.2** | 3 |

Season points, `t_crit(df) × SE × entry-span × setting-range × 38`, computed by the script rather
than by hand. Per-arm late-minus-early has a median MDE of **124**, range 29-289.

**Against CLAUDE.md's own median detectable of 39 points a season, this screen is 3 to 9 times
blunter than an ordinary sweep verdict** — on a question whose founding case is worth tens of
points. "Holm 1.000" therefore carries almost no information: it was close to pre-determined.

⚠️ **Do not import this record's prior that an interaction is the *cheap* contrast.** That is
recorded at CR2 SE 0.216 against 0.599 and it holds for a difference of differences **within one
cell**, which cancels the path divergence a single difference carries. This interaction is formed
**between cells**, across entry points that are different squads scoring different football, so
none of that cancellation applies. **This is the expensive kind of interaction.**

## What was screened

11 committed sweeps. Three are **structural theorems** and are excluded from the family rather
than reported as nulls:

- `H#1` (`min_gain`) and `HITS#1` are confined to `decide()`, which HOLD never calls.
- ⚠️ **`ANCHORED#1` is a theorem for a different reason, and the first version of this document
  named the wrong consumer.** `HoldCaptaincyWeekly` (`internal/backtest/simulate.go:2609`) never
  touches `Chips`, `Chips2` or `ChipPlanner`, and `cfg.ChipPlanner` is resolved inside `Simulate`:
  **HOLD plays no chips at all.** Right verdict, wrong mechanism — which is exactly the rule
  ("naming the consumer is the check") that this record states.

That leaves **8 measurable sweeps, 28 arm contrasts, and 6 ordered ladders.**

⚠️ **A consequence of screening HOLD only: no transfer constant is screened at all.** For those the
schedule question is **unasked, not answered**. It is only askable on `POLICY`, which this screen
refuses by design.

⚠️ **The family is not one data state, and one block has none.** The screen reads whatever was
banked: `MINHL`, `FIXW` and `benchshape` on six seasons (`b400ec3`, `68b6380`); `BONUS`, `MINW` and
`BENCH` on four at `d9b83c5` under `FPL_SWEEP_SEASONS=default`; `MINK` on four at `1a08d43`; and
**`TEAMFORM` with its provenance file absent entirely**. Each contrast is internally consistent —
one block, one state, printed per block — but the **Holm family is not one data state**, and
`MINHL` and `FIXW` are the two ladders the 2026-08-13 pass showed *move* across data states,
`MINHL`'s arms by up to 0.53 pts/gw. Read the per-block verdicts, not the table as one measurement.

## Nothing resolves, on either statistic

| statistic | family | best raw p | Holm |
|---|---:|---:|---:|
| schedule test (does the ladder's slope trend with entry?) | 6 ladders | 0.242 (`FIXW`) | 1.000 |
| per-arm late-minus-early contrast | 28 arms | 0.0585 (`benchshape`) | 1.000 |

**Read this as a tie, not a refutation.** A non-resolving comparison shows two settings cannot be
separated by this instrument; it does not show they are equal. Given the MDE table above, what it
refutes is only that one of these six is a schedule worth **150 to 350 points a season** — which is
far larger than a `BonusWeight`-sized schedule, so the founding case's own effect is **below this
screen's floor**.

⚠️ **These should arguably be one family of 34.** Two statistics on the same data answering the same
question, with the minimum reported across both; Holm is valid under arbitrary dependence, so
pooling is legitimate and is the honest family for an exploratory screen. Immaterial here — every
value is 1.000 — but the next run should pick one statistic in advance or Holm the union.

⚠️ **The "factor from resolving" column of the first version is withdrawn.** It printed
`t_crit(df)/|t|`, which is arithmetically exact and the wrong quantity: `1/|t|` has undefined
expectation, so under the null it is large simply because the estimate landed near zero. `MINW`'s
"79.6" said nothing about `MINW`; it said `MINW`'s noise draw was small. Ranking six ladders by it
ranks them by which drew nearest zero — the argmax problem run backwards — and it invites the
reading this record forbids, that a large factor means "definitively nothing". The MDE is a
statement about the *instrument*, which is stable.

## The positive control is only half a control, and the reason it fails is not the one first given

`BONUS#1` is the prior/evidence schedule sweep — the constant whose entry-point reversal founded
this hypothesis. The schedule test **refuses it**, so the instrument cannot test the case that
motivated it.

⚠️ **The stated reason was wrong.** The first version said the screen refuses it "because the family
is two-dimensional". The code implements no such test: `ladder_of()` parses the first number out of
each label and refuses on duplicates, and `BONUS` is refused because `0.5 / 1.5` and `0.5 / 2.0`
both parse to `0.5`. **A two-dimensional family whose first coordinates happen to be distinct —
`0.0/2.0, 0.5/1.5, 1.0/1.0, 1.5/0.5` — would be accepted silently and a slope with no referent
computed.** The guard looks principled and rests on a coincidence of label text, which is the
miniature of this record's own "naming the consumer is the check". The screen now prints *which*
rule fired; the real fix is for the sweep to emit a numeric `setting` column.

Two related consequences of parsing settings out of labels:

- **`TEAMFORM`'s baseline ("shipped, no club-form blend") and `MINHL`'s "flat (no recency)" arm drop
  out of their ladders** for carrying no number. `TEAMFORM`'s schedule test therefore runs
  **unanchored on three positive doses and cannot see the on/off step** — which is the contrast the
  recorded `TEAMFORM` finding (+24 a season against SE 34) is actually about. It is answering a
  different question for that ladder.
- A future sweep that relabels an arm silently changes the estimand.

What the descriptive columns show, on the *flat* sub-ladder `1.0/1.0` against `1.5/1.5`:

| entry | `1.0/1.0` | `1.5/1.5` | more flat weight is |
|---|---:|---:|---|
| GW1 | −0.579 | −1.368 | worse |
| GW6 | −0.152 | +0.970 | better |
| GW11 | −0.143 | +0.955 | better |
| GW16 | −1.054 | −0.000 | better |
| GW21 | −0.597 | +0.125 | better |
| GW26 | +2.173 | +0.135 | worse |

So "harmful from GW1, helpful from GW11" reproduces **directionally**, with a flip back at GW26.
⚠️ **Do not read the flip's location off this.** It is 2 arms on 4 seasons with no test attached;
the recorded boundary is GW11 and this suggests GW6, and nothing here can separate them. The
`regime_of` criticism below survives under either, which is the point.

## Two method findings, both against the item as written

**1. The flag the item specifies is unusable as a test, and fires hardest under the null.** "Flag
any constant whose ordering across settings reverses" — with four arms and six entry columns, each
column's rank correlation is a noisy draw about the constant's true slope, so a sign change is close
to certain whenever that slope is near zero. Under a true slope of zero,
`P(at least one sign change) ≈ 1 − 2·2⁻⁶ ≈ 0.97`, falling as the true slope grows: **the fire rate
is maximised at no effect.** Five of six ladders trip it, including `MINW` at t = −0.04, exactly as
predicted. It conflates "this constant is inert" with "this constant wants to be a schedule".

The replacement is the interaction (`schedule_test`), which has a calibrated null. Its weakness is
power against a **step**, not arbitrariness. The sign-change flag is kept and labelled DESCRIPTIVE.

**2. The repo's 2/2/2 `regime_of` split straddles the founding case's only boundary.** "Early" is
{GW1, GW6}, and the `BonusWeight` reversal is between GW1 and GW6-GW11 — inside that bucket.
Pooling averages away exactly the reversal being screened for, which is the same failure
("averaging reads as no structure") the schedule hypothesis exists to name. All six columns are
printed for that reason.

## The estimator, named honestly

`schedule_test` routes through `se_cr` so there is one implementation, but ⚠️ **it is a one-sample t
over seasons, not a CR2 fit, and should be called that.** With one observation per cluster the CR2
adjustment is `(1−1/G)^(−1/2)` for every cluster, the sandwich collapses to `s²/G`, and the
Satterthwaite df is exactly `G−1` — bit-identical to `t.test` at G = 4 and G = 6. `cells_common.R`'s
stated virtue for CR2 is that "the df it resolves is *reported* rather than asserted"; here the df
is **asserted by the design**, and a reader comparing "df 5" here with "df 5" from a genuine CR2 fit
would think comparable information had been resolved.

Two properties in its favour, neither obvious. **The ladder slope is invariant to the baseline's own
noise** — every point is `arm − baseline` including the baseline's own 0, so the common baseline is
an additive shift and cancels from a slope; that is a real reason to prefer it over the 28 per-arm
contrasts, which all share the baseline's noise and are strongly correlated with each other. And
**the unweighted second stage costs ~12%**, not a factor: the weights that would down-weight the
noisy GW26 column also destroy design leverage.

## Three limitations that constrain every reading above

**The entry columns are nested suffixes of one season.** ⚠️ **This argument is conditional on the
calendar reading and the first version stated it unconditionally.** Under a single per-week effect
`e(w)`, `D(g) = mean over [g,38] of e`, so `D(26) − D(1)` is an exact **0.66** shrink of the true
early/late gap — sign-preserving, proportional, and therefore *correctable*, so "read the sign,
never the size" was over-cautious by a factor of 1.5. Under an **evidence** reading — the one the
bench-shape mechanism below assumes — `e` is indexed by entry point, the squads differ, and there is
no attenuation at all. ⚠️ And non-flatness is **necessary, not sufficient**: any `e` whose mean over
[26,38] equals its mean over [1,38] gives exactly zero while being wildly non-flat. Read "only if",
not "exactly when".

**Entry point confounds evidence-at-entry with the window scored.** A flag says "this constant's
best value depends on entry point", consistent with both an evidence schedule and a calendar one.
For a blend weight the two coincide, which is why `BlendRateK` is the case this confound hurts
least.

**The late column is the noisiest.** Per-entry SD of the paired differences across all screened arms
runs 1.548 / 1.525 / 1.537 / 2.218 / 2.573 / **3.357** for GW1 through GW26, so ranking by
late-minus-early ranks partly by which arm drew the noisiest column: `TEAMFORM` +3.154, `MINW`
+2.731 and −2.542, `BONUS` +2.173 all rest on GW26.

## The one shape worth a pre-registered follow-up

Against the `flat 1/1/1/1` baseline, every non-flat bench arm is worth about **+2.2 to +2.5 pts/gw
at a GW1 entry** and roughly nothing after. Measured as GW1-against-the-rest, which is the shape's
own contrast:

| arm | GW1 | GW6 | GW11 | GW16 | GW21 | GW26 | **GW1 − rest** | t | df | raw p |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| fixed 2.4/1.0/0.4/0.2 | +2.167 | −0.081 | −0.702 | −0.478 | −0.148 | +0.038 | **+2.441** | 2.97 | 5 | 0.031 |
| fixed 1.9/1.2/0.6/0.3 | +2.114 | −0.141 | −0.940 | −0.500 | −0.250 | +0.064 | **+2.468** | 3.35 | 5 | 0.020 |
| derived (ships) | +1.689 | −0.525 | +0.077 | −0.428 | +0.556 | −2.449 | **+2.242** | 1.65 | 5 | 0.159 |

⚠️ **Three corrections to the first version of this section.**

- It reported the **late-minus-early** contrast (raw p 0.058 and 0.077) immediately after quoting
  the +2.1 GW1 level, which reads as testing the +2.1. **It does not** — nothing tested the +2.1.
  And that contrast comes from the 2/2/2 split this same document criticises, which pools the GW1
  step with a ~0 GW6 column and roughly halves it.
- It said "about zero from GW6 onward". The columns show **mildly negative from GW6 to GW21,
  deepest at GW11 (−0.70 and −0.94), returning to zero at GW26.**
- It named "the two fixed tuples", an argmax over two. **All three non-flat arms agree at +2.2 to
  +2.5, including the shipping derived one** — so the claim is the coherent single statement "any
  non-flat bench weighting beats flat at a GW1 entry and nowhere else".

⚠️ **Still post hoc, and Holm still kills it.** But this candidate is the one contrast in the family
that **escapes the late-column caveat**: it rests on GW1, which sits in the *quiet* group — cell-level
SD 1.548, against GW26's 3.357. ⚠️ Not "the quietest column", as a first draft had it: GW6 (1.525)
and GW11 (1.537) are marginally lower and the three are effectively tied, with the step up coming at
GW16. The point stands on the group, not on GW1 being the minimum. That materially strengthens the
case for pre-registering it.

⚠️ **It is not the bench-shape question already answered as a tie**, which is among the *non-flat*
arms; this baseline is `flat 1/1/1/1`, which nothing ships.

**A mechanism, as a hypothesis and not a finding:** a GW1 build has no current-season evidence, so
the bench weighting is a large share of what separates candidate fifteens, while by GW26 the
optimiser has 25 gameweeks of separation and picks a similar squad whatever the weights say. ⚠️ **It
predicts decay to zero and the columns show a middle dip**, so it does not fit the shape it was
written for.

## What this buys

Nothing ships changed. For the queue:

1. **`BlendRateK` needs a 4-arm × 36-cell replay** to be screened at all.
2. **`BONUS` needs a one-dimensional flat-weight ladder** if the founding case is to be tested
   rather than quoted, and **the sweep should emit a numeric `setting` column** so `ladder_of` stops
   inferring the design from label text.
3. **Pre-register `benchshape` as GW1-against-rest across all three non-flat arms**, not
   late-minus-early on two.
