# Review record — the archive data repairs

**Commits reviewed:** `728b7da..fe8bdf6` on `worktree-archive-data-repairs`, off `main`. Twelve
commits of new work. Named for the snapshot commit rather than the last work commit, so that
nothing the staleness guard watches changes after the record — the same reason two earlier
records here are named "past the merge" and "past its own snapshot". Previous record:
[`2026-08-12-2ab3f63`](../2026-08-12-2ab3f63/review.md).

**What the change is.** The three data issues the 2026-08-12 column audit ranked medium or
higher: reconstructing `expected_goals_conceded` for the four seasons that carry none, and two
row-level guards at load. Plus the audit's record-keeping items, which were reported and never
written down.

It is a **data** change rather than a scoring one — no constant moved and no term was added —
but it changes what the model is given in 18 of 36 cells, so it was triaged as a harness change.

## Triage

| reviewer | ran | why |
|---|---|---|
| **fpl-code-review** | yes | the diff changes `loadGameweeks`, the cache seam and the repair pipeline in `internal/backtest` |
| **fpl-stats-review** | yes | the reconstruction makes a calibration claim and a validation claim, and ships a constant |
| **fpl-findings-audit** | yes, on the second attempt | `CLAUDE.md`, two notes and the accuracy snapshot's reading text all changed, and three standing claims were retracted. **It found the most serious defect in the branch** — see finding 7 |
| fpl-security-review | no | nothing touches `internal/agent`, `internal/fpl`, config persistence or the disk cache's contents beyond a parsed field |
| fpl-run-review | no | no live run, no config written |
| fpl-season-maintenance | no | none of the four hand-maintained lists is involved |

The first `fpl-findings-audit` attempt died on an API error having produced nothing, and was
re-run. Recorded because a reviewer that fails silently is indistinguishable from one that
found nothing.

## What was written BEFORE any reviewer was dispatched

The review-gate skill's first section says invariants beat reviewers for this project's failure
modes, and that the first question is what quantity the change must not move. Two came out of
that and both earned their place:

- **`TestDiagWeeklyRowsReconcileWithSeasonTotals`.** `merged_gw.csv` and `players_raw.csv` are
  two views of one season, so a player's gameweek rows must sum to his season totals. That is a
  free detector for *any* row-level defect rather than the two that happen to be known — it is
  how the 2025-26 duplicates were found in the first place. Six of eight seasons now reconcile
  exactly; 2018-19 (3 minutes) and 2024-25 (17 minutes, 1 point) are pinned by season and to the
  exact figure rather than admitted by a tolerance, because a tolerance wide enough for 17
  minutes is wide enough to hide a duplicated appearance.
- **`TestTheRowGuardsDropPhantomMatchesAndKeepRealDoubles`.** A real double gameweek has the
  *identical shape* to the duplicate defect — same player, same gameweek, two rows, two
  `Fixtures`. A guard keyed on (element, gameweek) rather than (element, fixture) would have
  re-introduced the +115 pts/season doubles bug while fixing two much smaller ones. The stub puts
  one of each in the same week.

## Findings, ranked by how misleading the current state was

### 1. The arm this change queued for measurement could not have measured anything

**Found by:** fpl-code-review. **Verified independently** at `transferpolicy_test.go:101`.
**Applied.**

`FPL_NO_XGC_REPAIR` is re-read on every `Load`, and the comment I wrote claimed that made it
usable as a sweep arm. It does not: the *season* is memoised in a process-global cache, and
`runPolicySweep` calls `loadPairs` **once, before the variant loop**. An arm written the way
`FPL_NO_AVAILABILITY` and `FPL_UNREGISTERED_POOL` are — mutating the environment inside
`apply`, which is legitimate for those because they act per cell — would have replayed the
identical already-repaired season in both arms, come out byte-identical in every cell, and
reported a **clean, tight null on exactly the change it was built to measure.**

This is the package's signature failure aimed at its own follow-up, and the TODO item queuing
that sweep is a PRIORITY. Fixed at the switch, in `docs/replay.md`'s hatch table with a general
warning covering all three parse-time switches, and in the TODO item itself.

### 2. The repair reaches 18 of 36 cells; the instruction I wrote said 12

**Found by:** fpl-stats-review. **Verified independently** against `extendedPairNames`.
**Applied.**

The six-season grid plays 2020-21 through 2025-26. The hole is total in 2020-21 and 2021-22 and
partial in 2022-23 — and `PointInTime` accumulates from GW1, so **all six** 2022-23 entries see
the repaired GW1-15 rows rather than only the early ones. The inherited "12 of 36" counts fully
blind seasons and does not survive the repair reaching a partial hole.

My sweep instruction told the next person not to pool over 36 because 24 were structural zeroes.
It would have discarded six live cells including the largest mover in its own table. The same
instruction also named the wrong grid: `FPL_SWEEP_SEASONS=default` is the 24-cell four-season
grid, not the 36-cell one.

### 3. The pooled 0.9994 is a cancellation, not a null

**Found by:** fpl-stats-review as arithmetic. **Measured** by adding the split. **Applied.**

| population | n | reconstructed/actual | correlation | MAE |
|---|---:|---:|---:|---:|
| ever-present, single fixture | 18,531 | **1.0088** | **0.977** | **3.9%** |
| partial minutes | 20,442 | **0.9853** | — | **29.7%** |
| pooled | 38,973 | 0.9994 | 0.937 | 14.2% |

Two biases of opposite sign meeting in the middle. **A pooled null built this way would survive
both halves growing**, so the split is now what the diagnostic reports and what the note quotes.
The direction is what linear minutes-prorating predicts against a late-loaded danger profile,
which is *consistent with* the recorded goal-timing finding rather than a second measurement of
it. Also: the pooled correlation of 0.937 is largely a **minutes** correlation, since both sides
scale with time on the pitch — 0.977 is the figure that speaks to per-match fidelity.

### 4. "Refuted" was too strong, and the sample-size match was not evidence

**Found by:** fpl-stats-review. **Applied.**

What dies in the audit's "1.02-1.04 overshoot" is the **size, not the sign**: on its own
population the ratio of totals is above 1 in all four seasons, so ~1% survives and only 2-4%
goes. And the diagnosis is **inference from a value match rather than a re-run** — the audit's
prototype no longer exists, its recorded MAE triple (5.1/3.4/5.0) does not reproduce under any
assignment of seasons, and the "714-740" I cited as a matching sample size belongs to a
*different* check in the audit's own text. The n coincidence is decent circumstantial evidence
the populations match; it is not proof the implementations did.

### 5. "A player withdrawn at 60 gets 2/3 of the club figure, which is right on average"

**Found by:** fpl-code-review, **with a measurement**. **Applied.**

Actual xGC over the minutes-prorated share:

| minutes | 2023-24 | 2024-25 | 2025-26 |
|---|---:|---:|---:|
| 1-29 | 1.525 | 1.434 | 1.637 |
| 30-59 | 1.065 | 1.015 | 1.007 |
| 60-89 | 0.950 | 0.957 | 0.924 |

So the proration is not linear in truth: a late substitute faces ~1.5x his prorated share and a
withdrawn starter ~0.95x. At player-*season* level the two errors cancel to within ±1.7% for
defenders and keepers with 900+ minutes, which is why this is a caveat and not a defect — but
"right on average" was wrong and is now the table above.

### 6. Five smaller code defects, all applied

- **Guard B was gated behind the fixture list and does not need one.** "A player cannot appear
  twice in one fixture" is self-contradictory on its own, so duplicate detection was switched
  off for 2016-17 and 2017-18 for a reason that applies only to the calendar guard. The test is
  now `TestOnlyTheCalendarGuardIsInertWithoutAFixtureList` and asserts the two degrade
  differently.
- **`NoClubMatch` was uncounted**, so `Applied + Skipped + Empty` did not reconcile with
  appearances. The degenerate case is the point: a repaired season loading with no fixture list
  would have read `Applied 0, Skipped 0, Empty 0` — byte-identical to "there was nothing to
  repair". Now counted and asserted to reconcile.
- **The nil-pointer cache gate was one generation deep.** `RowGuards` now carries a `Guards`
  schema count, because the next guard added would otherwise read every existing cache as
  current and report its own count as a plausible zero.
- **`TestDiagArchiveRowGuards` read the disk cache**, so it compared its expected counts against
  an *earlier* parse and would have passed on stale numbers through exactly the archive
  re-publication it claims to catch. It now parses into a `t.TempDir()`; runtime went 0.57s to
  ~2s, which is the difference between reading a cache and reading the archive.
- **An ungated cache-seam test**, which the xG hatch had and this one lacked, so `go test ./...`
  exercised one and not the other. Plus a comment naming a test that does not exist.

### 7. THE WORST FINDING IS ONE THIS BRANCH INTRODUCED: the `d`/2020-21 annotation

**Found by:** fpl-findings-audit. **Verified independently** — the source, the code, and a count
of the capture payloads. **Applied: withdrawn, and the TODO item raised on it deleted.**

Working from the column audit, I annotated CLAUDE.md's team-news finding to say the granularity
arm is **structurally inert in six 2020-21 cells** because that season carries no `d` status, and
raised a medium-priority TODO to re-read the arm with those cells excluded.

Both halves are wrong:

- **Wrong file.** The audit counted `status` in `players_raw.csv`, which is the end-of-season
  snapshot behind `statusAt` — the **baseline** arm. `OracleTeamNews` and `OracleTeamNewsChance`
  read `data/captures/<season>/GW*/bootstrap-static.json.gz`. Counted there, **2020-21 carries
  1,271 `d` player-gameweeks across its 38 captures, 19 to 55 a round**, against 1,049-1,304 in
  every other season. Nothing is inert; the recorded 18 / 22 / 10 / 4 counts stand untouched.
- **Wrong code.** I wrote that `statusAt` consults `NewsAdded` only for `u` or `i` "so a `d`
  player is returned as `d` from GW1". The code is `if p.Status != "u" && p.Status != "i" {
  return "a" }` — a `d` player is returned as **available**, deliberately, because `d` is
  transient and its May value says nothing about September.

This is the archive-versus-payload confusion in its purest form, and it is worse than the
defects the branch was fixing: those were in data, this was a **false claim inserted into the
research record**, in the section a reader consults before quoting the team-news result. Both
the annotation and the deleted TODO item are kept as marked entries so the idea is not rebuilt.

The general lesson, and it belongs in the standing rule: **a status count is only about the file
it was counted in.** "The archive says X" needs to name which file, because this project now has
end-of-season snapshots, weekly rows and crawled payloads that disagree with each other by
construction.

### 8. The snapshot silently stopped measuring fifteen figures

**Found by:** fpl-findings-audit, with a *hypothesis* about the cause. **Measured, and the
hypothesis is confirmed.** Applied.

`2026-08-12-30d8b06` carries a "No longer measured" section listing fifteen
`model.defcon_bias.forwards_*` figures, and the note documenting that snapshot did not mention
them. The audit proposed the duplicate-row guard as the cause and labelled it unverified.

Confirmed by counting the raw CSV both ways: element 100 (Kroupi, a forward) read **652 minutes
through GW19 on the duplicated rows and 489 on the deduplicated ones**, so the guard moved him
across `TestDiagDefconBias`'s 600-minute gate, the forwards group went from 24 to 23, and 23
trips the `len(in) < 24` skip. The diagnostic prints `forwards: only 23 players, skipped`.

Two consequences, both recorded: the forwards defcon figures are **absent rather than
unchanged** and must not be quoted from an older snapshot as current; and the group was **one
player from the floor all along**, which is a fragility in the diagnostic rather than in the
guard.

### 9. Four smaller record defects, all applied

- **The 12-of-36 correction reached the header comment and not four other places**, including
  the sweep instruction — whose dilution factor was also wrong, since the untouched remainder is
  18 rather than 24, so it halves the estimate rather than thirding it.
- **"Both defects halve a rate rather than moving a total"** is false for one of the two. True
  of 2019-20, whose rows are empty — which is why they survived. The 2025-26 duplicates moved
  +163 minutes and +27 points as well.
- **"`Optimize` is not run-to-run deterministic"** was quoted as the reason the per-cell table is
  not a verdict. It was fixed at `9e5e1d1` and this very note says so 200 lines above. The real
  reason is that a *deterministic* discrete search is still a sensitive one.
- **"A uniform xGC scaling cannot reorder within a position"** overreaches: it preserves the
  ordering of the clean-sheet *term*, but `Score` also carries the goals-conceded deduction and
  `exp(-xGC)` is convex, so two defenders differing elsewhere can swap. The **size** is what
  makes it safe, and the additive-bias precedent does not apply to a multiplicative scaling.
- Arithmetic: mean absolute error falls at **seven** of eight cutoffs, not six; the snapshot
  moves **249** model figures, not 251; `metrics.go`'s gates are at `:1895` and `:1902`.

### 10. The composition mediator, added on review, refutes half its own prediction

**Suggested by:** fpl-stats-review. **Applied, and it found something.**

The pre-registered sign is about composition, and the test reported only points. Adding the
mediator shows the **DEF+GKP count is 7 in every cell of both arms** — and could not have been
anything else, because FPL forces two goalkeepers and five defenders in a fifteen. "The model
buys more defenders" was never an available outcome, so half the prediction was unfalsifiable
when it was written. The column is kept precisely because a structurally inert mediator reads
identically to a null.

Money, which is free to move, does: 33.5 → 41.0, 36.5 → 30.5, 35.5 → 39.0, 34.0 → 39.5 across
the four affected cells, and unchanged to the tenth of a million in all three unaffected ones.

## What was declined, and why

- **Fitting `xgcScale` to 1.0088, the ever-present figure.** Declined, and the reviewer agreed
  with the conclusion while improving the argument. It is not transportable — every estimate
  comes from the FPL-fed seasons and it is applied only to the Understat-fed ones — and the
  backfill's own provider offset carries a **4-5% residual level error** that the reconstruction
  inherits linearly. Arguing over 0.06% or 0.9% inside ±4-5% is not calibration. A uniform xGC
  scaling is also monotone in the clean sheet and cannot reorder within a position.
- **Fixing the mid-season transfer attribution.** Measured at 0.32-1.17% of appearances and 7-15
  players a season. Declined for now and recorded as a TODO option, on the specific ground that
  the validation seasons carry the same defect at the same rate, so it is already inside the
  reported error rather than sitting on top of it. Fixing it needs a new `GW` field and a cache
  change, and would improve the validated figures as well as the repaired ones.
- **Chasing the unexplained 0.9%.** Left recorded with the alternatives eliminated —
  misattribution is zero-sum across clubs and cannot produce a level shift; the ten duplicate
  rows are 0.15% of a season's xG; two-decimal truncation biases down — leaving a definitional
  difference as the surviving candidate. That is a hypothesis with alternatives eliminated, and
  it is labelled as one rather than asserted.

## What could not be checked on this harness

- **What the repair is worth in points.** The per-cell table is one entry point per cell and is
  explicitly not a verdict; the six-season sweep is queued in TODO.md and must be run as **two
  separate processes** per finding 1. Unmeasured, not unmeasurable.
- **The transport assumption**, previously recorded as untestable and now corrected to
  **untested**: `CALIBRATION_SEASONS` already harvests Understat per-player-match xG for exactly
  the four seasons carrying a real xGC column, so the chain can be run end to end from Understat
  and scored against the truth. One harvest, no replay. Queued.
- **A second xGC-carrying season for the four repaired ones.** Genuinely unmeasurable: the
  archive has no `expected_goals_conceded` column before 2022-23 and never will.

## Was the gate worth running

Yes, and not marginally. The three reviewers produced **nineteen confirmed defects** between
them, of which the branch's own testing had found none. Ranked by what they would have cost:

1. a queued PRIORITY measurement that **could not have measured anything** and would have
   reported a clean null (code review);
2. a **false claim inserted into the research record**, in the section a reader consults before
   quoting the team-news result (findings audit — and the claim was mine, added in this branch);
3. a pooled figure that read as a null and is a **cancellation** (statistics review);
4. a sweep instruction that would have **discarded six live cells** including the largest mover
   in its own table (statistics review).

Two of those are the failure mode this project names as its signature — a null that is really an
inert instrument — arriving in the *work built to avoid it*. That is the argument for the gate
in one line.

**Where the invariants beat the reviewers**, per the skill's own claim: the reconciliation test
and the doubles regression were both written before any reviewer was dispatched, both are free,
both run every time, and the reconciliation test is how the 2025-26 duplicates were found in the
first place. The reviewers caught *judgement* errors — over-claims, a wrong denominator, a
mis-sourced count — which is exactly the split the skill predicts.

**Where a reviewer was wrong, and it was checked rather than applied.** The findings audit
proposed the duplicate-row guard as the cause of the fifteen vanished forwards figures and
labelled it a hypothesis. My first test of it looked like a refutation — Kroupi sits at 489
minutes, well below the gate — because I counted the deduplicated archive rather than the
duplicated one. Counting both ways confirmed it. The lesson cuts the same way as finding 7: the
number is only about the population you counted.
