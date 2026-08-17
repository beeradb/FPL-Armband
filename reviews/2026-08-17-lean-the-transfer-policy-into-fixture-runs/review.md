# Leaning the transfer policy into fixture runs

Reviewed: the working tree of `lean-the-transfer-policy-into-fixture-runs` against `b4e2b61`
(the banking merge on `development`), which is this branch's base.

## What was reviewed

Three things, which are one piece of work:

1. **A determinism fix in `teamBands`.** The 3/14/3 band assignment was built by ranging a map
   into a slice and ordering it with the non-stable `sort.Slice`, so a tie on a band boundary
   resolved by Go's randomised map order. Fixed by building the slice in club-id order and
   breaking both sort ties on club id.
2. **A fixture-run mediator**, five new cells-CSV columns beside the transfer-banking funnel:
   `band_ready_weeks`, `band_moves`, `band_run_moves`, `band_worse_moves`, `band_exposure`.
3. **The tandem made expressible and provable** — banking, chip preparation and the fixture
   bands in one arm, each checked at its own mediator rather than at its config field.

**No points claim is made anywhere in this change, and no default moves.** `band_strength` still
ships at 0.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the change touches `internal/analysis` (scoring) and `internal/backtest` (harness), and its whole output is an instrument whose columns will be read by someone who was not here |
| fpl-code-review | **skipped** | triage lists it for `internal/analysis`. Declined deliberately: the two failure modes it exists to catch here are covered by invariants that run every time — the determinism fix has a positive control that fails without it, and the mediator join has a source scan that fails when the line is deleted. The gate's own preamble says invariants beat reviewers for exactly these failure modes. Recorded as a decision, not an omission |
| fpl-findings-audit | **skipped** | triage lists it for `AGENTS.md` edits. The `AGENTS.md` change is a single verdict correction (`teamBands` "Unfixed" → fixed) plus one stale enumeration, both of which the statistics reviewer checked directly and corrected. No new finding was recorded and no figure was added |
| fpl-security-review | not applicable | no change to `internal/agent`, `internal/fpl`, or config persistence |

## Findings, applied

Ranked by how misleading the state would have been.

1. **`band_run_moves` had no null to be read against.** With only the favourable count,
   `band_moves - band_run_moves` pooled "traded the run away" with "the bands had nothing to say
   about this move", and those license opposite conclusions. **Applied: added
   `band_worse_moves`.** The data vindicates the finding outright — on one 2025-26 cell the arm
   reads **11 better, 10 worse, 10 tied**, so the original single count would have been reported
   as "a third of transfers improve the run" when the honest reading is a coin flip.
2. **Nothing warned that a between-arm difference in these columns is a `POLICY` difference.**
   The first thing a tandem sweep invites is subtracting them across arms, which carries the full
   transfer-path divergence with no pairing and ~31 observations a cell. Applied, in the struct
   comment and in `stats/README.md`, with the recorded 303-point transfer-path floor named.
3. **`stats/README.md` was not updated at all**, and it is where an analyst actually looks —
   it already carries the "counts, not rates" warning for the banking block. Applied: a full
   section, including that `band_moves` is **not** the `moves` column (36 against 31 on one cell)
   and that `band_ready_weeks` is close to a calendar fact rather than an arm property.
4. **`FixtureRun.Fixtures`' doc comment was factually wrong** — it said blanks and doubles change
   the count. They do not: `TeamFixtures` takes the next *n fixtures*, so a double changes which
   gameweeks the window spans, not how many matches are in it. Wrong reason, and it is the reason
   a reader would use to decide whether doubles inflate `band_exposure`. Applied.
5. **"That split is the same one `attackBandAdj` and `defenceBandAdj` make" overstated.** True of
   *which* band each position reads, false of *how much* it is worth: the model's attacking
   coefficients are deliberately asymmetric (0.23 target against 0.15 avoid) and the clean sheet
   enters through `exp(-x)`. `Net` weighs them equally, so it is a count and never a proxy for
   the adjustment's size. Applied.
6. **`TestTheFixtureRunFunnelNests` asserted a property four of six sweep start points
   legitimately violate** — from about GW7 every decision week is band-ready. It passed only
   because the fixture pins `StartGW: 1`. Applied: gated on an early start point, with the gate's
   reason stated, since the sibling comment in the same file warns this is how a test gets deleted
   instead of read.
7. **`FixtureRunMediator.Moves`' comment and the code disagreed** about moves whose players do not
   resolve. Applied: the comment now says "resolvable".
8. **The `144` reachable-assignments figure could not be checked from the text.** The number is
   right, but the quoted tie sizes multiply to 96 — it is a product of `C(tie, places inside)`.
   Applied: the derivation is now carried in full in `banddeterminism_test.go` and the shortened
   copy in `bands.go` points at it rather than inviting wrong arithmetic.
9. **The `AGENTS.md` jitter warning was over-stated by omission.** "Cannot be re-derived from its
   own cells" standing alone reads as an indictment of the `BandStrength` result two bullets
   above. The size is known and small — the arm's mean moved +0.339 → +0.357 pts/gw, about a tenth
   of that contrast's own CR2 standard error, concentrated away from the cells carrying the
   estimate. Applied, with the referent the file's own rule requires.
10. **`docs/configuration.md` used `0.25` in a worked example with no explanation**, which reads
    as a recommendation. It is precisely the arm the record calls deciding and unrun. Applied.
11. **A stale enumeration in `AGENTS.md`** — the `mustNotMoveForAxis` bullet listed the column
    families required to differ by arm and did not include the new block. Applied.
    (`cellMetricColumns` is unchanged at eight, so no invariance falsely covers the new columns.)

## What was declined

**Nothing from the review was declined.** Two things the reviewer suggested for *later* are
recorded here rather than done:

- **A canary arm** — a `band_strength` large enough that the mediator must move — before any
  tandem sweep is banked, so a flat result can be told from a mediator that cannot respond. Agreed
  and correct; it belongs to whoever runs the sweep, not to this change, which runs none.
- **A per-move "band component of the chosen pair's score delta"** was considered and refused on
  mechanism, not cost: it describes the pair the argmax already chose, so it inherits that
  selection and would read as attribution while being a property of the winner. The refusal is
  written into `stats/README.md` so it is not built later.

## What could not be checked on this harness

- **Whether the tandem is worth anything.** Nothing here measures it, and nothing here claims to.
  The change makes the arm expressible and its null readable; the sweep is a separate act.
- **The magnitude the determinism defect cost, per season.** One repeat gives an occurrence count,
  not a variance estimate. The recorded 0.7 points a season is a single draw and is quoted as one.
- **Attribution — did the bands cause a move.** Unavailable at any affordable cost, not deferred:
  it needs a second full transfer search every week. This is a property of the instrument and is
  stated as such in both the code and `stats/README.md`.

## Proofs run, rather than reasoned about

Two silent-no-op holes were closed and both were demonstrated, because a guard that has never been
seen to fire is not evidence:

- **The determinism fix**: with `bands.go` reverted, `TestBandAssignmentIsDeterministic` reports
  **3 distinct band assignments from 16 byte-identical calls** and fails. With the fix, 1 and
  passes. The test constructs its own boundary tie, so it carries its own positive control rather
  than depending on whichever ties a season happens to hold.
- **The mediator join**: deleting `row.FixtureRuns = fixtureRunsOf(res)` from `runPolicySweep`
  was verified to leave **every other test in the package passing** — exactly the hole a review
  found in the banking block one commit earlier. `TestTheSweepAssignsBothMediators` fails on it,
  and watches the older banking join too, which had the same gap.

`go build ./... && go vet ./... && go test ./...` — all packages ok.
