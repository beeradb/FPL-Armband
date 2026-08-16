# The calibration-accessor guard, and the residual coverage measurement

## What was reviewed

`integrate-the-accessor-guard`, merging `guard-the-calibration-accessors-and-size-the-residual-exposure`
onto `main`, plus two edits made during integration.

Two independent pieces, kept in separate commits on the source branch:

1. **A guard**: `TestTheCalibrationAccessorsHaveNoProductionCallers`, a row in the existing AST scan
   in `cmd/priorblend/gatecallers_test.go`, asserting the exported calibration accessors have no
   non-test callers.
2. **A measurement**: `internal/backtest/residualcoverage_diag_test.go`, sizing the post-repair
   exposure of `XPointsResidual`'s ungated goals and assists channel. ~2s, no cells.

## Reviewers

The source branch ran `fpl-code-review`, `fpl-stats-review` and `private-store-audit` before handing
over. This record covers the integration and the two edits I made on top.

## Findings

### Applied during integration

1. **The second scanner had the identical reach hole, and its own doc claim made that worse.**
   `internal/stats/copies_test.go`'s `goSources` skipped only `.git`, `node_modules`, `data` and
   `stats`, so it descended into every sibling worktree under `.claude/worktrees/`. Measured from
   the main checkout, **9,101 of the 9,450 Go files it reached were another branch's.** That breaks
   the guard in **both** directions: it can report a copy that exists only on a branch nobody is
   merging, and `sanctioned` is keyed on repo-relative paths that cannot match a sibling's, so a
   real exemption silently stops applying.
   ⚠️ **It is invisible from inside a worktree**, where `.claude/worktrees` is empty — the scan
   reaches 349 files here and 9,450 from the checkout where it actually runs. Fixed with the same
   nested-checkout detection the accessor guard uses: a `.git` *entry*, by presence rather than
   name, so it holds for a worktree (a `.git` file), a clone (a `.git` directory) and a vendored
   repository alike.
   ⚠️ **My first version of that fix was wrong** and skipped the entire tree, because the root is
   itself a checkout and I tested for `.git` before exempting `rel == "."`. **The scan's own
   "no Go sources found; this guard is scanning the wrong tree" assertion caught it** — which is
   the argument for that assertion existing, and worth recording rather than quietly fixing.
2. **`CLAUDE.md` gained the measurement's line**, which the source branch was instructed not to
   write. Recorded as counts of archive rows with **no points claim**, since nothing was replayed
   and no threshold applies.

### Accepted from the source branch, with the reasons it gave

3. **The guard covers five accessors, not the three I briefed.** `DefconTermFor` and `SavesTermFor`
   sit in the same file, one is named in another row's own doc comment, and both have test-only
   callers today. Leaving them out of a table whose comment says "the next one is a row" is exactly
   the hand-maintained-list failure this project keeps paying for. **Accepted** — the scope addition
   was reported rather than slipped in, which is the behaviour I want.
4. **A correction the agent's own code review forced, and it matters.** Its first comment claimed
   all three accessors were one-line delegations with "nothing for a runtime equivalence test to
   catch". That is **false** for `CleanSheetTermFor` and `SavesTermFor`, which delegate to nothing —
   each *re-expresses* its term. Left standing, that comment would have justified deleting the real
   property check in `internal/analysis/cleansheetprob_test.go`. The comment now says the opposite.
5. **The measurement's first verdict was refuted by its own statistics review.** It read "the
   harvest left essentially nothing" after splitting exposure into `covered`/`blank`, declaring
   `covered` benign **by assumption**, and testing only `blank`. **All the elevation was in the
   untested bucket.** Corrected, with the retraction recorded. This is the third time this session a
   subagent's headline was wrong in the direction that flattered its own instrument.

## Declined

- **Dropping the guard back to three accessors.** Offered by the agent; declined for the reason in
  finding 3.
- **Implementing a gate on the attacking channel.** Out of scope by instruction and correct to
  leave: most of the exposure is legitimate — a won penalty or deflected assist genuinely carries
  ~0 xA — so a naive `xg+xa > 0` gate would be wrong. The right shape, if one is ever wanted, is a
  season/gameweek capability gate like `DefconScoredIn`. Recorded in `CLAUDE.md` so the next reader
  does not reach for the naive one.
- **Reading the ~0.3% residual mass share as a bound on decision leverage.** It is a *lower* bound
  there, not an upper one, because every exposed row carries a realised return — which is precisely
  what a positive-residual gate fires on. Recorded rather than dropped.

## What could not be checked

- **The instrument-change hypothesis** for the assist elevation (Understat's key-pass xA against
  FPL's `expected_assists`, which also pays for won penalties and deflections). The count is
  consistent with it and does not test it. Labelled a hypothesis in `CLAUDE.md`.
- **Any points consequence.** Nothing was replayed. No detection threshold applies to these levels
  and none of this sizes a gate arm.
- **Whether the guard is a proof.** It is not, and its comment says so: these scans match an idiom
  keyed on one spelling of it, so they are tripwires.

## Redaction note — 2026-08-16

One reviewer name in the paragraph above was edited after this record was filed. The name identified
a private store this repository may not name; it now reads **private-store-audit**. The sentence is
otherwise unchanged and no finding was touched — that reviewer ran, and on the scope stated.

⚠️ **The instance was found by the bare repository's `pre-receive` hook, which rejected a push naming
it.** It is cleaned rather than exempted: the standing exemption for already-committed disclosures is
a grandfather clause over an enumerated set, and this was found afterwards. The cost — amending a
dated attestation — is acknowledged, which is why this note exists rather than the edit being silent.
