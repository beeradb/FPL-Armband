# Review: scoping the next work, where two of the three fatal findings were my own

**Commit range reviewed:** `9317f70..4fd232c`. Fixes applied at `0d59a8f`, which this record is
named for.

**What changed.** A new `TODO.md` section — "The next work, scoped" — turning the three open
`PRIORITY` items left by the archive-repair arc and the chip/gate arc into six named tests with
pre-registrations, plus a correction ticking the Understat starts item that had shipped a day
earlier with its checkbox unticked.

## Reviewers run, and the triage

| reviewer | why |
|---|---|
| **fpl-stats-review** | six experiment designs, each with a pre-registered prediction and a threshold |

Skipped: **fpl-code-review** — no source change, `TODO.md` only. **fpl-findings-audit** — the
section makes no new points claim; every figure it quotes is cited to an existing record, and the
statistics reviewer checked those citations as part of its remit. **fpl-security-review**,
**fpl-run-review**, **fpl-season-maintenance** — nothing in scope.

**Invariants first.** `go build`, `go vet` and `internal/snapshot` clean before and after, including
`TestReviewCoversTheCurrentCode` and `TestRetractedFiguresAreNotQuotedAsCurrent`. None of that caught
anything, which is the expected result for a doc-only change and is the reason the reviewer was the
whole of the check here.

## Findings, ranked

### 1. The bench 2x2's arms did not run the contrast its own P1 named — APPLIED

`FPL_FIXED_BENCH_SLOTS=1` clears `derivedBenchSlots` (`internal/analysis/benchslots.go:138`) and
restores the **shaped tuple 2.4 / 1.0 / 0.4 / 0.2**, not a flat bench. So the 2x2 as written measured
derived-against-shaped — **6 points across four seasons**, ~1.5 a season, an order of magnitude below
the best-resolved comparison this record has produced — while pre-registering the **77-point**
flat-against-shaped gap. Flat needs `FPL_BENCH_SLOTS=1,1,1,1`.

Re-specified as three shape levels by two floor levels, six arms. Two further corrections came with
it: the second floor level moves from 30 to **no floor**, because bench fodder at ≤ £4.5m is exempt
from the floor entirely (`internal/analysis/squad.go:377-380`, `456-459`) so 30 is an interior point
on a step function; and P1 is re-registered as **expected unresolved**, since 77 over four GW1 cells
is 0.507 pts/gw, about t 1.1.

**And the target figure is internally contradicted by the repository.**
`docs/notes/optimiser-and-squad.md:493-497` has the held-out 2022-23 column as a **tie**, 1666
against 1666; `internal/analysis/squad.go:1139-1141` says flat is **79 better** there and that
"shaped beats flat is not established by anything". Both cannot be current. Recorded as owed rather
than settled — it is a paper question, not a sweep.

### 2. Item 4's withdrawal was right and its stated reason was refutable — APPLIED

The draft withdrew the xGC substitution discriminator on the ground that "a difference between two
unresolved estimates is less resolved than either". False on this harness, and CLAUDE.md's own rule
says so: the two arms share a baseline and 18 identical cells, so their difference is the within-cell
second difference recorded at **season-clustered SE 0.216 against 0.599** for the noisier main
effect, with no df penalty. Publishing it would have handed someone a correct argument for
reinstating the experiment.

Replaced with the three reasons that hold: the sign is a function of the estimand (equal-weighted
−34, inverse-variance −25, **GW1 entries alone +14**); the comparison's own recorded threshold of
**55-150 a season** already exceeds the 34, so no run is needed; and the estimator is degenerate in
the triple-captain-preparation shape, bounding a flip rate and nothing about a flip's value.

**The mediator replacement was withdrawn too**, and this is the finding I would have been least
likely to reach alone. Reading whether the corrected column moves the defender ordering is a wiring
check, not a discriminator: "a better-specified objective makes a worse policy" is *consistent with*
the ordering improving — that is what the phrase means — and `prior_half_life` is the standing
precedent of an arm that improves ordering and stays off. The unit was wrong as well; a season-level
sign test on three seasons caps at **p = 0.125**.

The near-null control survives on its own account and **not as a gate**: a large floor would stop the
arm, a small floor would not license it, so the gate had one outcome that changed anything.

### 3. Item 6 was mis-priced — APPLIED

`BandStrength` was listed among "the two free ones". There is no `BANDS` block in
`TestDiagProjection` and `band_strength` appears under `stats/snapshots/*/cells/` only inside
`*.provenance.csv` config dumps, never as an arm label — **no committed cells to re-judge**. It is a
2-arm × 36-cell replay. The section assumed cells existed because a constant had been swept, which is
a version of this record's own rule about checking which *file* a number came from.

### 4. A retracted figure was re-quoted as an acceptance bar — APPLIED

The draft set "beat 2.36% of starter slots misclassified" as the acceptance threshold for the starts
harvest. That figure was retracted the same day:
`stats/snapshots/2026-08-13-aa95f75/FINDINGS.md:148` records the like-for-like number as **11.74%**
for the rank rule against the harvest's 0.000%, and `reviews/2026-08-13-cae6941/review.md:54` logs
it. Deleted from the new section and from the stale item's own text.

### 5. Two of three sentences about the oracles were wrong — APPLIED

The draft said the lineups and minutes oracles **gate cells** on `!g.StartsReconstructed`, that four
seasons were excluded, and that the harvest makes them "live cells". None holds.
`internal/backtest/lineupsoracle.go:222` routes rows into a *clean* pool and `resolve` (`:159-161`)
**falls back to the full sample** where clean support is thin; `minutesoracle.go:344-352` is an
exposure counter and gates nothing.

What actually moves is the ground truth, and the honest price is smaller than claimed: the recorded
bound is 24 cells across four seasons, of which only 2022-23's six carry reconstructed starts and
only through GW15. **The gain is the six-season grid**, which is the only route that touches the
threshold of 177 — not the re-run.

### 6. The reachability map would have been a trap in three ways — APPLIED

Conditional knobs return false no-reach from a cell where they simply do not fire
(`PrepareTripleCaptain` places a chip in 23 of 36 cells). Small nudges are the regime where a step
function hides a live knob, so probe at the extremes of each knob's legal domain. And a purely
perturbational map reproduces, in its negative cells, the assertion-instead-of-computation failure it
exists to fix — so each observed no-reach wants a named consumer, cheapest as a poison value that
panics if read, converting "did not move" into "was not read".

Also: two of the four `internal/analysis` knobs have exported in-process setters
(`analysis.SetUnifiedAppearance`, `analysis.SetDerivedBenchSlots` at `sweep.go:356`, `:363`), so the
draft's exec budget was overstated — which also removes item 3's "the two shape levels are two
execs" and restores a process-equivalence control the xGC run had to establish separately.

### 7. A third `Starts` consumer, and a null read wider than it licenses — APPLIED

`tournamentAbsence` reads `el.Starts` **directly** at `internal/analysis/metrics.go:1829`. It is
inert in every replayed cell for a reason that had not been named: every replay engine is constructed
with an empty congestion block (`simulate.go:991`, `:1049`, `:1113`, `:1130`, `:2573`, `:2589`,
`engineat.go:56`), so `absenceByID` is empty. The **agent** builds `Congestion` from config. Exposure
today is nil because all eight congestion penalties ship at 1.00, but the 36-cell byte-identical run
licenses "no replayed cell moves", never "nothing reads starts". Scoped in place.

### 8. Two smaller applied items

- **A falsifiable boundary that was false.** The draft said the xGC "native xG, missing xGC" corner
  exists "only 2023-24 onward, and those already carry native xGC". `xgRepairs`
  (`internal/backtest/xgrepair.go:71-83`) repairs 2022-23 for GW1-15 only, so its second half is
  exactly that window at 60% xGC coverage. Does not change the conclusion; corrected rather than
  softened.
- **A conflation inherited from the queued item.** The `BENCH` projection block sweeps
  `sc.BenchWeight` — the bench *level* — while the 2x2 varies the slot *shape*, and the two are
  deliberately orthogonal (`benchslots.go:41-46`, `103-133`). The consolidation claim drops from six
  items to four, and the held-out item, which makes the same conflation, now says so.

## Declined, with reasons

Nothing was declined. Every finding was applied, which is unusual here and is explained by the
subject: a plan has no sunk measurement to defend, so the cost of applying a finding is a paragraph
rather than a re-run. That is the argument for reviewing designs before running them and it is worth
recording as the reason rather than as a coincidence.

## What could not be checked on this harness

- **The flat-against-shaped contradiction** between `optimiser-and-squad.md` and `squad.go` cannot be
  settled by reading; one of the two was measured under a data state or an appearance rule the other
  was not, and neither carries its provenance. It is queued as owed.
- **Whether the reachability map's negative cells are real** is exactly what the map is for, so the
  poison-value check is a design commitment rather than something this review could verify.
- **Nothing here was run.** Every figure quoted is from an existing record, and the review checked
  citations rather than re-deriving them. Six designs remain unrun.
