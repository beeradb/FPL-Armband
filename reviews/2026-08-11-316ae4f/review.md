# Review record — the rank-objective write-up and its scope block

**Commit range reviewed:** `origin/main..316ae4f` — four commits, `2f603e3`, `3e05205`, `05b0022`,
`316ae4f`, on branch `rank-objective-handoff-finish`.

**This is the first review record on this branch, not in the repository.** ⚠️ An earlier draft of
this file claimed the latter and it is wrong: `origin/main` already carries
`reviews/2026-08-10-0d7d31d`, `2026-08-10-149615c` and `2026-08-10-be65af1`. This branch forked at
`149615c`, before those landed, which is why `TestReviewCoversTheCurrentCode` *skipped* here with
"no review records yet" rather than passing — a branch artifact, not a statement about the repo.
What is true is that nothing in this range, including the original measurement at `2f603e3`, had
been through a gate before now.

## Which reviewers ran

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the diff is documentation, but its *substance* is inference: weight ratios, flip counts, a field model, a tail argument |
| **fpl-findings-audit** | yes | record-only changes, cross-references, house style — the triage table's `CLAUDE.md` / `docs/` row |
| fpl-code-review | **skipped** | no Go changed. `go build`, `go vet`, `go test ./...` all pass; only `stats/rank_robustness.R`'s printed text was edited, and nothing depends on the string |
| fpl-security-review | **skipped** | nothing touches `internal/agent`, `internal/fpl`, config persistence or the cache |
| fpl-run-review | **skipped** | no live run, no config written |
| fpl-season-maintenance | **skipped** | the four hand-maintained lists are untouched |

## The finding that mattered

**The scope block's alarm was an artifact of its own arithmetic, and the reviewer caught it.**

`316ae4f` reported that `flat (no recency)` — worth −90 points a season, one of the two findings
this record leans on hardest — flips sign under a computed rank reweighting at the pessimistic end
of the field-spread range. It multiplied each cell's points difference by a weight, `w_i × d_i`.
That is a **first-order expansion**: it evaluates the field's density at one point and treats it as
constant across the gap between the two arms' scores. The gap is 90 points, about 0.6 of a field
standard deviation, over which the density changes several-fold.

**Verified independently before acting**, by computing the quantity the expansion approximates —
the mean over cells of `F(x_variant) − F(x_base)`, which is what scoring on rank literally is:

| arm | `t` naive | flips, linearised | flips, exact |
|---|---:|---:|---:|
| `POLICY` `flat (no recency)` | −2.76 | 61.7% | **0.0%** |
| `HOLD` `flat (no recency)` | −0.83 | 26.7% | **0.0%** |
| `POLICY` `half-life 2` | −2.90 | 41.7% | 18.3% |
| `POLICY` `half-life 8` | −0.81 | 100.0% | 78.3% |
| `POLICY` `half-life 20` | −0.41 | 100.0% | 75.0% |

Across a 60-point sensitivity box, **no arm above `|t|` = 1 changes sign anywhere.** The committed
verdict survives the computed reweighting and survives it *better* than the linearised version
suggested. The alarm is retracted.

## Findings applied

1. **The linearisation retracted and replaced with the exact percentile calculation**, with the
   60-specification box table. Harness note, scope block.
2. **The fat-tail defence refuted and its follow-up closed before anyone spent a session on it.** A
   heavier tail forces a *smaller* spread (the spread is identified from the percentile anchors),
   pushing cells further out; best case is a 28% compression of a ~2,000x ratio, and at 3 degrees of
   freedom it is *worse* than Gaussian. The anchors argue against a fat tail anyway.
3. **The Spearman −0.977 marked as an algebraic identity, not a measurement.** For data-independent
   weights `P(flip)` has a closed form in `|t|` alone which reproduces every arm to 0.74 percentage
   points. Corrected in the harness note, `CLAUDE.md` (twice), the handoff and `stats/README.md`.
4. **"No mechanism correlates the weights with which arm won a cell" corrected** — the correlation
   with our own baseline score is −0.15 to −0.70, same sign in all eleven arms. The honest phrasing
   is "systematically shrinking, and not far enough to flip".
5. **`HOLD`'s near-immunity promoted to the load-bearing defence** (0.0-10% against 0-78% on
   `POLICY`), since scoring constants are judged on `HOLD` by this record's own convention.
6. **The `sigma38` = 147 / `p` = 0.5 pairing recorded as internally inconsistent** — the marginals
   bound the within-gameweek spread at ≤ 15.5 points, so independent weeks cap the season spread at
   ~96. Bracketed at `p` ≥ 0.62; the box spans 0.5 to 1.0 and the answer is stable across it.
7. **The captaincy/bench cancellation downgraded from "nearly cancel" to a ±100-point bracket.**
   Field average now quoted as roughly 1,900 to 2,100, never 1,957.
8. **Dangling citation fixed** — `docs/notes/archive-and-data.md:626` does not exist (the file is 517
   lines and has no ownership section; verified). Re-pointed at a section title, with the reason it
   rotted stated: the write-up lives in another session's uncommitted tree.
9. **Three of the eleven arms marked not reproducible on this branch** — `EXP=HITS` is not a block
   `TestDiagTransferPolicy` carries here (verified: "HITS" does not appear in the file).
10. **`P(flip)`'s Monte-Carlo order-dependence recorded** — the script seeds once and shares one
    stream, so the last digit depends on argument order. The reviewer's re-run gave 3.5/18.7/2.9/1.4
    against the recorded 3.4/18.6/2.7/1.3.
11. **The script's own stdout and `stats/README.md` corrected** — both still called `P(flip)` "the
    realistic figure", which is where a re-runner actually meets it.
12. **Two irreconcilable entry counts marked in place** (10.68m against 11.52m, same arithmetic,
    neither naming a gameweek), with the blank-gameweek mechanism that probably explains it.
13. **Layout fixes** so a reader meets the qualification before the 0.0% column, plus the handoff
    header, the `docs/README.md` row, and "1.5" → "1.6 points over a season".

## Findings declined, and why

- **Add the season-clustered `t` to the eleven-arm table** (stats-review finding 3). Correct and
  important — it shows `flat` on `POLICY` does not resolve at all under the record's preferred
  estimator (−1.23 against a 3.18 bar), while `half-life 2` on `HOLD` at −4.80 is the only arm that
  does. **Deferred, not rejected**: recorded as follow-up 4 in the handoff. It needs a pass over the
  cells that belongs in `rank_robustness.R`, not in prose.
- **Move the exact-percentile calculation into `stats/rank_robustness.R`** (stats-review, "on
  provenance"). Agreed in principle and recorded as follow-up 3. Declined *here* because writing new
  R and re-deriving the committed table in the same commit that retracts a finding would leave
  nothing stable to compare against. The Python that produced these figures reproduces the block's
  own 37x / 2,343x / 17x and its 1,957 exactly, so the inputs are verified even though the estimator
  needed replacing.
- **Demote `R_crit` to a footnote** (stats-review finding 11). Declined for now: it is exact, its
  `P/N` derivation is short, and someone will re-derive it if it disappears. Its heading has been
  left alone. Revisit when follow-up 3 lands.
- **Reconcile the two entry counts by re-deriving one** (audit finding 5). Marked in place rather
  than resolved. Re-deriving 10.68m needs its season and gameweek, which the original does not
  record, so the honest act is to flag both as unusable rather than guess which is right.
- **Incrementing the "archive does not have X" tally to seven** (audit finding 14). Declined on the
  audit's own reasoning: the *column* really is absent and what was missed is that it is derivable,
  which is adjacent to that standing rule rather than another instance of it. The "said twice" count
  was dropped rather than corrected.
- **Some remaining unglossed terms** (audit finding 13, partial). `phi`, `z` and `sigma_i` are now
  glossed at first use in the scope block; `Phi` and the coefficient of variation are glossed where
  introduced. Not every occurrence in older prose was swept — that is a standing debt on this file,
  not something this range introduced.

## What could not be checked on this harness

- **The field's tail shape beyond ~3.1 standard deviations** — the archive holds per-player
  marginals and no field distribution. Only externally published percentile points constrain it.
  Per the refutation above it also does not matter.
- **The exponent `p`** (how the field's spread grows with cell length) — needs manager-level
  histories the archive does not hold. Bracketed at 0.62 to 1.0.
- **The captaincy and bench corrections** — **unmeasured, not unmeasurable**. Checkable against
  `average_entry_score` on the live season only, which is follow-up 2.
- **Three of the eleven arms** — the `EXP=HITS` harness is not on this branch.

## One thing the gate got right that is worth keeping

Neither reviewer's headline finding was reachable by reading the diff. The audit's came from
*running* the script and from *checking the Go* behind cited identifiers; the stats review's came
from *re-deriving the computation and then computing the thing it approximated*. A gate that only
read prose would have passed all four commits.
