# Review record — gating the prior blend on a thin but non-zero last season

**Commits reviewed:** `f13ec84..888a2e6` on `prior-blend-gate`, restricted to the fourteen that
are this task's. The branch shares a working directory with another agent, so the range is not
the whole of it — `c1291d7`, `9b2ecb6`, `782b89d`, `3a3cb71`, `905fb40`, `afac8ac` and `ff342c8`
are theirs and were excluded from both briefs by name.

**What the change does.** `prior_half_life` blends the season before last into a player's prior
when last season was thin (under `ThinSeason`, 1,710 minutes). It shipped off and still does.
A benchmark on the parent branch found it was two features under one name — helpful for a thin
but non-zero last season, harmful for zero minutes — so this adds `analysis.ShouldBlendPrior`
and routes all three implementations of the rule through it, then re-runs the benchmark.

## Reviewers dispatched

| reviewer | why |
|---|---|
| **fpl-code-review** | the diff touches `internal/analysis` and `internal/backtest`, which the triage table sends here, and the specific risk is a rule applied on some paths and not others |
| **fpl-stats-review** | same two trees, plus the change re-runs a measurement and rewrites what it claims |

**`fpl-findings-audit` was skipped deliberately.** The triage table asks for it on
`internal/analysis`, and one was already reading the parent branch `prior-blend-benchmark`
concurrently. Dispatching a second onto an overlapping range would have duplicated its work on a
moving target, which is the thing the task brief asked to avoid. Nothing in this range edits
`CLAUDE.md` or `docs/`. The two reviewers that did run were both told to report to me rather than
propose record edits.

**Invariants first, per the skill's opening section.** Before dispatching anyone the question was
what this change must not move. Three answers, all now tested rather than argued:

- at `prior_half_life 0` nothing may move — checked by diffing all 561 model diagnostic rows
  between the base commit and the branch (50 differ, every one at the fifteenth significant
  digit, i.e. summation order);
- the two excluded populations must land where the shipped model lands — one
  `TestTheBlendGateIsThinAndNonZero` per package, each deriving its expectation from
  `ShouldBlendPrior` rather than restating it, and each verified to fail when its own gate is
  reverted;
- the gate wired into the model must reproduce the hand-built by-population arm — `reportGateWiring`,
  666 to 865 priors per season pair, no disagreement.

Those three found nothing the reviewers then found, which is the honest reading: the reviewers
earned their cost on **claims**, not on code.

## Findings, ranked by how misleading the state was

### 1. The gate was applied on two paths and silently defeated on the third — the only live one. APPLIED (`39c3e8a`)

From **fpl-code-review**, and it is the most valuable finding of the pass because it is the exact
failure the gate was built to prevent.

`recent.blendPast` drops zero-minute seasons from the history *before* anything else, so for a
player who sat out last season `hist[0]` is a season at least two years old and **carries
minutes**. `blendRates` gates on `!ok || p.Minutes == 0`, so a prior with minutes never reaches
`shrinkToLeague`: he was rated on stale rates, and pre-season the stale season replaced the
bootstrap outright. `cmd/fplagent` reaches this path and no other when `prior_half_life > 0`.

The reviewer also showed the *baseline* was wrong: `prior_half_life 0` does not take
`blendPast`'s `halfLife <= 0` branch, it stops `recent.LoadPriors` being called at all and the
prior comes from `priors.Load` — one season, carrying `Minutes 0`, which does reach
`shrinkToLeague`. Verified before applying, by writing the failing test first.

Fixed by returning "no usable prior" when the last season on record is zero minutes. The test
now reconstructs the shipped answer instead of comparing against a branch production never
executes.

### 2. The gated ordering ladder is not monotone, and "removes ninety per cent" is the wrong shape. APPLIED (`41e7038`)

From **fpl-stats-review**. Half-life 0 is a rung of the ladder and its value is exactly
`0.000000` by definition; leaving the anchor out is what let the text call the gated ladder
"still monotone", when the gate is precisely what destroyed that property.

Verified independently rather than accepted: differencing the two ladders shows the gate's effect
is **+0.00204 to within 0.5% across a fourfold change in half-life**, and the residual increments
are identical gated and ungated. The gate removes a **level**; the **slope** is untouched and is
negative, so more of this feature still orders the field worse.

The reviewer named a cheap decisive test — add half-lives 0.125 and 0.25, since a real interior
optimum returns smoothly to zero through a run of positive values and a noise draw does not. It
was run. The curve reads `0 → +0.000142 → +0.000431 → +0.000332 → −0.000204 → −0.000752`, which
is a rise to an interior maximum near 0.25 and then a monotone decline through zero, and it
survives dropping 2022-23 at about half strength. That is better evidence than the text had, and
it only exists because the reviewer asked for the extra rungs.

### 3. 99% of the highlighted positive rung is one anomalous season. APPLIED (`41e7038`)

From **fpl-stats-review**, verified by leave-one-out. The `+0.00033` at half-life 0.5 collapses
to `+0.000032` without 2022-23, a season sitting ≈ +0.002 above the other five in every arm of
both runs, whose top-twenty signed error is +1.92 over GW16-38 against ±0.25 elsewhere. Recorded
with the caveat that dropping a season chosen for being anomalous is itself a selection.

**Not resolved, and recorded as such:** whether 2022-23's step at GW17 is the World Cup restart
or the archive's native xG seam at GW16. Both sit there and this instrument cannot separate them.
The reviewer's suggested check — re-run with `FPL_NO_XG_REPAIR`, or excluding GW17+ — was not run.
That is **unmeasured, not unmeasurable**.

### 4. The tail sentence volunteered the outlier's direction. APPLIED (`41e7038`)

Pooled, the top-twenty signed error moves toward zero. Five of six seasons move **away** from it,
and 2022-23 supplies more than the whole pooled change. The inherited sentence was not false — it
carried "does not resolve" — but it read as reassurance the data does not give.

### 5. The gameweek-clustered t is not evidence for this comparison. APPLIED (`41e7038`)

The treatment is constant within a player-season, so 227 gameweek clusters are near-replicates of
six season-level quantities; clustering must be at the level of assignment, and the design effect
is 2.5x. The Holm p of 0.058 I had quoted for the positive rung is now stated as not evidence,
alongside the point that the arm clearing 0.05 on that same clustering is half-life 2 **in the
negative direction**.

### 6. `popFieldSound` acquired a second definition between the two runs being compared. APPLIED (`39c3e8a`)

From **fpl-stats-review**. Hoisting `r.zeroPrior` above the control exit dropped 9,705
observations from a population whose entire purpose is comparability across the two runs. Nothing
quoted depended on it. Restored to the ungated definition and verified back to 148,428.

### 7. Two remaining hand-copies of half the bar. APPLIED (`41e7038`)

`classify` and `injuryOnlyPriors` still read `q.Minutes >= priors.ThinSeason` while their doc
claimed the predicate "is called rather than restated". Equivalent today, and equivalent only
while the bar's *comparison* matches as well as its value. Both now read the predicate.

### 8. `reportGateWiring`'s denominator was inflated. APPLIED (`41e7038`)

It unioned the older seasons' codes in, and both indexes only emit codes present in the prior
season — so codes found only in an older season counted as agreement without either index being
asked anything. 860–1,346 becomes **666–865**, still with no disagreement.

### 9. The population-nesting contract is now false. APPLIED (`41e7038`)

`popAbsent` is inside both `popCase` and `popControl` under the gate, so `popCase` is no longer a
subset of `popReached`. Harmless downstream — `emitGameweek` writes one row per population and no
name is a substring of another, which the code reviewer confirmed — but stated where the
R-selector contract is stated.

## Declined, with reasons

**`gatecallers_test.go`'s three holes: not closed.** It matches on selector name only, so a local
shim passes; it matches a call whose result is discarded, which is the silent-no-op shape it
exists to catch; and it names three files by hand. Declined because the behavioural tests in each
package are the real evidence and would catch all three, and a source check that tries to prove
more than "the call is present" becomes a second implementation of the thing it checks. The holes
are now named in the test rather than left implied.

**Commit `3754d76` is red and history was not rewritten.** The code reviewer showed, statically,
that it carries another agent's half-finished guard edit — swept in by my `git add -A` — and that
`TestEveryScoringEngineGetsRecency` fails at that commit. It is green from `b9a81b8` onward.
Declined to fix by rebase: the branch is pushed, another agent is committing into the same
working directory, and rewriting under them is a worse failure than a red commit mid-branch.
Recorded here so a bisect knows. The same accident ran in both directions — `ff342c8` is the
other agent's commit carrying my uncommitted `cmd/priorblend/main.go` edits.

**The interior optimum's location: not pinned.** Three positive rungs is a shape; which of 0.125,
0.25 and 0.5 is best is an argmax over four noisy points and is not claimed.

**`FPL_NO_XG_REPAIR` / GW17 sensitivity: not run.** Named above as unmeasured.

## What could not be checked on this harness

**Whether the gate is worth points.** The prediction benchmark ranks predictors and cannot price
them, and this project's hardest-won result is that a better predictor can make a worse policy —
arriving, twice, from this exact family. No replay was run and none can be until
`SimConfig.OlderPriors` is populated outside `cmd/priorblend`, which it is not.

**Whether the gated setting should be turned on.** Unmeasured rather than unmeasurable, but the
design is demanding and is recorded in the branch report: `HOLD`, 24 cells, one pre-registered
half-life, and a **mediator reported first** — how many opening-squad slots and fielded
player-gameweeks are attributable to a blended player. If that is near zero the arm is inert and
its null says nothing, exactly as the availability oracle's "inert in 13 of 24 cells" does. A
null with a dead mediator is unmeasurable, not refuted.

**Whether 2022-23's anomaly is the World Cup or the xG seam.** Both fall at the same gameweek.

## One thing the reviewers agreed on that is worth carrying

The gated re-run produced exactly **two genuinely new statistics** — the whole-field ordering at
half-lives other than 1. Everything else is identical to the ungated run by construction, or an
n-weighted combination of two sub-populations already reported. The load-bearing result of these
commits is therefore the **wiring check**, not the re-run: the gate in the model and the
hand-built by-population arm are the same index, so a measurement already made transfers to the
shipped code with no inferential step.

And the gated run is **in-sample with respect to the gate's own construction** — same six
seasons, and the population split that motivated the gate was introduced by the commit that first
ran it. The case for the gate rests on mechanism, which would stand if the numbers had come out
differently: `shrinkToLeague` is a designed answer for a player with no usable history, and a
two-year-old season is not.
