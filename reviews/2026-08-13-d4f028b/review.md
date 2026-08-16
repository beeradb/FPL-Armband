# Review: the reachability map, and a contradiction I published without noticing

**Commit range reviewed:** `0b87796..b3c81a7`. Fixes applied at `6e3386a`; this record is named for
`d4f028b`, the tip it covers — the merge with main at `ff97eda` and the accuracy snapshot after it,
whose contents are the three items in "After the review" below.

**What changed.** The metric-reachability map (`TestDiagMetricReach`) and its 8-cell run; item 3
dropped; item 5 closed with a new confinement test; a new `MinExpectedMinutes` claim in `CLAUDE.md`;
two new standing rules; the bench-shape retractions from the previous record.

## Reviewers run, and the triage

| reviewer | why |
|---|---|
| **fpl-stats-review** | a byte-identical null published into `CLAUDE.md` as a verdict |
| **fpl-findings-audit** | four files gained or lost claims in one day, several correcting the branch's own earlier ones |

Skipped: **fpl-code-review** — the only source changes are test files and a comment, and the
diagnostic's correctness was checked by the statistics reviewer against the committed cells.
**fpl-security-review**, **fpl-run-review**, **fpl-season-maintenance** — nothing in scope.

**Invariants first.** `go build`, `go vet`, `internal/analysis`, `internal/backtest` and
`internal/snapshot` green throughout. **None of them caught anything below.** The one mechanical
guard that *should* have caught finding 2 — `TestRetractedFiguresAreNotQuotedAsCurrent` — could not,
for a reason recorded in finding 8.

## Findings, ranked

### 1. The claim I published contradicts the note it links to — MARKED, and the reconciliation queued

`docs/notes/transfer-policy.md`'s floor sweep puts **"no floor" 112 `HOLD` points below** the
shipped 55. `CLAUDE.md` now said removing the floor is **byte-identical**. Both cannot describe the
same tree.

The arms are wired identically — both set `MinExpectedMinutes = -1`, which `simulate.go:996-1001`
maps to no floor — so this is not an arrival failure on either side. Block J is **four-season totals
down a single path**, this record's noisiest method, from `194c654` on 2026-08-08: before the xG/xGC
backfills and before the appearance unification.

**I edited that file to add the reachability finding and inserted it eight lines above the
contradicting table without noticing.** Both claims are now marked unverified, in both files, and
re-running block J paired at 36 cells is queued as `PRIORITY`. Neither figure may be quoted until it
does.

### 2. The 51/79 retraction was applied to one file of five — APPLIED

It landed in `internal/analysis/squad.go` and nowhere else. It survived in
`docs/notes/optimiser-and-squad.md` (the *first* table, outside every marked block), in
`docs/model.md`, and twice in `TODO.md` — **once as the ordering premise of an open `PRIORITY`
item**, which is a withdrawn number scheduling work. All four marked in place.

The same note also still carried **"shaping the bench does help"** as its settled conclusion, which
is exactly what the 36-cell tie does not say. Withdrawn; "which shape barely matters" kept.

### 3. A third four-cell bench record — APPLIED

8126 against 8128, in a checked-off `TODO.md` item, matching neither of the other two. That is the
evidence they are three separate runs rather than three readings, and it means "the two four-cell
tables" undercounted. Now three, in `CLAUDE.md` and in place.

### 4. The mechanism attached to the null was wrong — APPLIED

The floor does **not** cut fringe players. It removes **96 to 126 a build**, topped by Salah
(£14.1m, 54.1 settled minutes), Saka, Isak, Palmer and Eze — 55 min/gw is 2090 minutes over 38,
which a heavily-substituted starter in a good side misses. "Expensive players who rarely start" is
false.

**The real mechanism is redundancy with the objective and it is the better argument**, because it
generalises: `Score` is already minutes-scaled, so the cut population's best score is 2.9-3.9
against a kept-pool best of 5.6-8.1.

### 5. Four scope errors on the same claim — APPLIED

- **"Removing the floor entirely" is wrong** — `MinMinutes` 600 is untouched in every arm.
- **The effective sample is 8 opening *builds***, not 8 cells over six columns: `minExp` reaches
  three call sites and two are the wildcard and free-hit squads, which never fire because
  `sweepConfig` places no chip, and the transfer search is floorless.
- **"At or below 55" is right but not from two probes** — it follows from the removed set being
  monotone in the floor, so the pools nest. A theorem for an exact optimiser, a near-theorem for a
  DP seed plus local search.
- **`BankUpTo` was mis-sorted** as a measurement; every consumer is on the `POLICY` path, so it is a
  theorem like `min_gain`. Three theorems and one measurement — and mis-sorting it *inside the
  section making that distinction* is the fourth time this branch committed the error it was
  documenting.

### 6. The credit claim was retro-fitted — WITHDRAWN

"The map cost eleven minutes and the 2x2 it invalidated was six arms over 36 cells" is contradicted
by the commit order: item 3 was dropped at `c4447a1`, the map ran at `1996ee7`. The map supplied a
**second, independent** reason to drop something already dropped — and the sentence was being used
to justify a standing "run the map first" policy, so the inflation mattered more than usual.

**The better reason the map does support**: item 3's mechanism needs a floor level making the eleven
*more fragile*; lowering cannot and raising makes it more nailed, so **no setting of the knob
produces the case the hypothesis is about**.

### 7. `weekly_xi` — NARROWED

`cfg.WeeklyXI` has exactly one consumer, `simulate.go:1110`, the weekly fielding engine. So its
`HOLD` zero **and** its moves/hits zeros are compile-time confined — theorems, by the same section's
own standard. My stated reason ("`HOLD` re-picks the eleven weekly by definition") does not follow:
re-picking weekly is not re-picking on a one-week horizon. The real content is the pairing — it
moves `POLICY` in 8 of 8 while changing no move and no hit.

### 8. The guard cannot hold these three figures — ATTEMPTED, REVERSED, RECORDED

The audit asked for `77`, `51` and `79` in `retractedFigures`. Added; they fired on **seven
legitimate lines** — `77 min/start`, `| 0 (flat) | 8060 |`, `| flat (no recency) | 28 | 51 |`, a
season-clustered t of −5.51. Bare two-digit integers keyed on words as common as "flat" and "bench"
cannot be guarded, and the guard's own rule is that one which fires on correct prose gets deleted.
Reversed, with the attempt recorded where the entries would have gone.

**This is why finding 2 needed an audit to find**, and it is worth knowing that the mechanical half
of the retraction discipline has a class of figure it structurally cannot cover.

### 9. Smaller applied items

- The last uncorrected `2 × SE × 38`, in `TestDiagNoiseFloor`'s own header.
- The `t_crit` rule read as if df 5 were a harness constant; it is resolved per comparison.
- "A null is a tie" said a null *confirms* two settings cannot be separated — one word from the
  error it forbids — and did not distinguish itself from the byte-identical rule two sections up.
- An unflagged simple-effect caveat on all 17 map arms: the baseline sets `WeeklyXI = true`, which
  is not the sweep default.
- `CLAUDE.md` was at 21 bytes of headroom; the ten-thresholds classification table moved to
  `harness-and-inference.md`, taking it to 662.

## Declined, with reasons

- **Converting the six stale queued items** the audit found. Queued rather than done: it is
  mechanical, it touches items across the whole file, and doing it inside a commit that is already
  correcting four files would bury it.
- **The remaining four `CLAUDE.md` moves** (~2 KB). One was taken to buy headroom; the rest are
  queued with byte estimates, because the file is not currently failing.
- **Re-running block J.** It is the right fix for finding 1 and it is 4 arms × 36 cells; queued as
  `PRIORITY` rather than launched.

## After the review: three things the reviewers did not see

Recorded here rather than in a new record, because none is a reviewed change and each is one
mechanical step.

**1. A confinement violation I shipped three commits earlier, caught by an invariant rather than by
me.** `TestNothingInAnalysisKnowsAboutOracles` scans `internal/analysis` for any mention of an
oracle, because a hindsight hook in the shipped scoring engine would make every oracle figure a
bound on a model nobody plays. My `SetBenchSlots` comment cited "the oracle stamp" as the precedent
for keeping the environment in step, which trips it on the word alone — correctly, since the rule
depends on the grep being unambiguous.

⚠️ **It had been failing since `c3f6548`.** I verified that commit with
`go test ./internal/analysis/ ./internal/snapshot/` and not `./internal/backtest`, which is where
the guard lives. The guard did its job; I did not give it the chance. **Run the full suite before
calling a branch clean** — three of this branch's commits claimed verification on a subset.

**2. `origin/main` moved five commits mid-review**, including the oracle re-pricing on recorded
starts that this branch had queued as owed. Merged rather than rebased, per this repository's
pattern. One conflict, in the Understat starts entry, where both sides had ticked the same item:
main's side wins outright — it carries the authoritative harvest figures and commit refs and already
states the byte-identical null — with two things carried over from mine that it lacks, the
`tournamentAbsence` third consumer and the stale-item count.

**3. Three of the audit's queued `CLAUDE.md` moves were taken**, because the merge combined both
sides' additions and put the file back over budget. The `prior_half_life` worked case and the
"no truth value" provenance went to `harness-and-inference.md`; the rest-list teamsheet rule went to
`configuration.md`. Each leaves the rule resident and moves the working. 219 bytes of headroom
remain; the fourth queued move is worth about 350 more.

## What could not be checked on this harness

- **Which of the two floor results is right.** Finding 1 is a contradiction, not a verdict; nothing
  here adjudicates it, which is why both are marked rather than one.
- **Whether the 8-build null generalises past the opening build.** No chip fires in this grid, so
  `playWildcard` and `freeHitSquad` are untested for this knob.
- **The map's own negative cells**, beyond the one arm that got a named consumer. That is the static
  half, still owed.
