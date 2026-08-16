# Review record — the availability-flag re-fit

**Commit range reviewed:** `a61941d..HEAD` on `flag-refit` — `cmd/flagfit`, `backtest.EngineAt`,
and section 3 of `stats/snapshots/2026-08-11-0104d9d/FINDINGS.md`. Previous record:
[`2026-08-11-5c17e7e`](../2026-08-11-5c17e7e/review.md).

## Reviewers dispatched

**`fpl-code-review`**, **`fpl-stats-review`** and **`fpl-findings-audit`**, concurrently, read-only.

Code review is not what the triage table demands for a diagnostic, and it was dispatched anyway
because **here the code is the measurement**: an off-by-one in the point-in-time cutoff, or a unit
error in the numerator, produces a wrong number that looks entirely reasonable. That judgement was
right — the two defects that invalidated the first result were both code, and neither would have
been visible in the output.

Skipped with reason: `fpl-security-review`, `fpl-run-review`, `fpl-season-maintenance` — nothing in
scope.

**All three found real errors.** That is now five reviews for five today.

## The headline: the re-fit was wrong twice and is now a weaker claim than it started as

The program reported an exponent of **1.59** and called it "the denominator that actually gets
multiplied". Both halves were wrong.

**It measured an unwired engine.** `flagfit` called `analysis.NewEngineFull` and attached neither
`Engine.Recent` nor `Engine.Priors`, which every engine inside `Simulate` attaches on the next two
lines. With `Recent` nil there is no recency-weighted minutes index and `blankRunFactor` never
fires; with `Priors` nil the prior-season blend returns early. So it read the right *field* carrying
its *fallback* value — a flat season-to-date mean — and the mechanism its own write-up named, the
recency index carrying old absences, **was not switched on in the run**.

**And it divided a two-match total by a per-match expectation.** A double gameweek posts 180
minutes. Doubles are 10% of the 25% rung against 2% of the unflagged rung, so it did not cancel in
the normalisation. The two defects moved the answer in **opposite** directions, so neither could be
ignored and they did not offset.

**Then the statistics review removed the corrected headline too.** The corrected write-up led on a
season trend — the exponent rising 1.03 to 1.76, with "face value was right in 2020-21" as its
hook. That is confounded with archive quality: Wayback coverage improves over time, early seasons
carry staler captures, and a stale flag has had time to be revised, which drags the rung toward the
unflagged population and pushes the exponent down. **Verified independently before applying**: with
a 24-hour filter the slope goes from **+0.132 per season (t = 4.35) to +0.060 (t = 0.69)**. The
trend is gone.

## What survives, stated at the strength it has earned

- **FPL's flags are optimistic and the model's face-value reading is refuted** under every
  specification tried, by a wide margin. This is the finding.
- **A 75% flag is worth about 0.65 to 0.70 of expectation** — 2,088 observations, replicating in
  every season. This is the durable number.
- **An explicit 100% flag carries real information**, about 9% against the unflagged group — but
  see below.

## What was withdrawn

- **"The exponent is 1.59."** Two code defects.
- **"Three constructions agree."** They shared one numerator, one flag series and one population,
  differing only in a denominator that two of them got wrong *in the same way* — so their agreement
  concealed the defect rather than testing for it.
- **"`flag²` is refuted."** No interval was ever computed. Clustered by season, the best-defended
  specification gives 1.59 with a 95% interval of **[1.16, 2.03]**, which contains 2.00.
- **"The trend is the finding."** Confounded, above.
- **"The model under-predicts healthy players by 9%."** A pooled reference inflated by doubles;
  corrected it is ~2%, and the residual is a *slope* in expected minutes rather than a level.
- **"A 100% flag is a buy signal."** FPL's 100 is **sticky** — only ~10% of those rows are fresh and
  no player ever returns to the unflagged group — so the comparison is "has had news this season"
  against "never has". Corrected for calendar position, doubles and denominator it is **+6.6%**
  overall and **+2.4%** among near-nailed players, decaying to +0.4% after ten weeks. Not
  actionable.

## Applied

Both code defects fixed and verified; the trend downgraded with its filter and figures; the interval
recorded; the sticky-flag correction; the rung-values-not-an-exponent instruction, with the reasons
(three points cannot identify a form, the anchor sits 6.7% off 1.0, the 0% rung has a floor); the
77%-weight-on-one-rung caveat; and the standing rule that any future flag measurement must report
and filter on `hours_to_deadline`, which every manifest carried and nothing read.

Two structural fixes rather than one-off corrections:

- **`backtest.EngineAt`** now exists, so the replay's wiring cannot be omitted by a caller who does
  not know to look for it. The rule *"every engine that scores players needs the index"* held; its
  guard reads `simulate.go` only, and an engine in `cmd/` is invisible to it. **The guard should be
  widened to glob `cmd/`** — recorded, not done.
- **`flagfit -maxlead`**, so the staleness confound is testable rather than argued about.

Incidentally: reading each capture once per gameweek rather than once per player took the run from
**over 25 minutes to 7 seconds**. That is why the floor sweep and the staleness filter were checked
at all — at 25 minutes a run, nothing was.

## Declined, or deferred, and why

- **Acting on the 100% finding. Declined**, on the reviewer's own arithmetic: +2.4% where it would
  be applied, against a harness that cannot see it.
- **Wiring the percentage into the replay. Deferred deliberately** to a medium-priority `TODO.md`
  entry, scheduled *after* the GW1 deadline, because it changes availability behaviour ten days
  before a real squad is picked. The entry names the seam (the oracle mechanism, already
  fingerprinted), the two arms it must be split into, and the expectation that it will not resolve.
- **A `Score`-space measurement** of a decaying, minutes-conditioned correction — the thing that
  would make the 100% finding actionable. Named by the reviewer, not attempted.
- **Widening `TestEveryScoringEngineGetsRecency` to `cmd/`.** Deferred; it is a guard change and
  belongs with the replay wiring above.

## Process failures worth recording

**I swept a reviewer's scratch directory into a commit** with `git add -A`. Removed in `048b91b`
and `.gitignore`d. A blanket add in a worktree three agents are working in is not safe.

**Six API connection failures across five agents** interrupted this session. The mitigation that
worked was reordering: commit the expensive artefact before analysing it, and for read-only
reviewers, write each finding out as it is confirmed rather than composing one report at the end.
Both agents that were cut off after that instruction lost nothing.

## What could not be checked

`PointInTime` carries no percentage, so **the value of any correction remains unmeasurable on this
harness as wired** — unchanged by everything above, and the single most important sentence in the
section. Whether the season drift is real or an artefact of the 25% rung's ~500 observations split
six ways is **unmeasured**; the project's own forward capture answers it in a couple of seasons.
