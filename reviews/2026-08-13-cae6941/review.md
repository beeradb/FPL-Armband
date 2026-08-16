# Review of `4d61058..cae6941` — the backfill runs, the mde recovery, the starts harvest

34 commits reviewed at `0a7a8ff`; the fixes carry it to `cae6941`. Three reviewers ran
concurrently, alongside two earlier reviews whose reports were captured but unapplied
(`reviews/2026-08-13-backfill-runs/review.md`).

**Terms.** `HOLD` is buy-and-hold with weekly re-picking, the metric that decides a scoring
question; `POLICY` adds the weekly transfer decision. A **cell** is one replayed season entered at
one deadline. **Season-clustered** treats each season as one independent unit; **start-fixed**
treats the six entry gameweeks as fixed. The **data states** are `shipped` (both expected-goals
backfills on), `xgcoff` (expected-goals repair on, expected-goals-conceded repair off) and
`xgoff` = `bothoff` (neither).

## Reviewers, and the skips

| reviewer | ran | scope |
|---|---|---|
| **fpl-code-review** | yes | `startsrepair.go`, the `season.go` call site, the harvest script, the tests, the env-switch registration |
| **fpl-stats-review** | yes | the validation, the identity, `MinutesWeight`, the mde recovery, Runs A and B |
| **fpl-findings-audit** | yes | `CLAUDE.md`, `TODO.md`, `docs/notes/`, both `FINDINGS.md` |
| fpl-security-review | **skipped** | no credential handling, agent-loop or config-persistence change. The harvest fetches public numeric CSV parsed with `strconv`; no secret reaches disk |
| fpl-run-review | **skipped** | no live run wrote config in this range |
| fpl-season-maintenance | **skipped** | the four hand-maintained season lists are untouched |

**An invariant was written before dispatching**, per the gate's own rule that invariants beat
reviewers here. The starts repair had no confinement check — nothing stopped a harvest file
reaching a season that records its own starts.
`TestTheStartsHarvestCannotReachARecordedSeason` (`0a7a8ff`) fails at build time on a bad harvest
rather than after a season has been replayed.

## Applied — findings that changed a conclusion

**1. The starts repair was inside `fetch`, so the harvest was inert.** (`c287862`) It ran before
`Load` writes the season cache — the placement `season.go` explicitly forbids, because a repair
applied before the write is baked into the cache and the escape hatch then reads a repaired cache.
Verified rather than inferred: the cached 2021-22 season held **7,005 starts, all
rank-reconstructed, none harvested**. Moved into `repaired()`, which changes the conflict
semantics — after the cache the reconstruction has already run, so `Starts > 0` means "the rank
rule guessed", and the repair now replaces an inferred start while refusing a recorded one.
`TestTheStartsSwitchWorksOnACacheHit` pins the placement and asserts the on-arm is capable of
differing, so it cannot pass on a corpse.

**2. The foreign-match leak, and an identity that could not see it.** (`7188ee5`) The inherited
filter is per **gameweek**, not per match, so a mid-season mover with Premier League minutes *and*
a foreign fixture inside one gameweek window had the foreign match admitted and counted as a
start. Two cells: Martial's Sevilla match and Weghorst's Leipzig match, both 2021-22 GW23.

The season-total identity could not detect it, and the test comment claiming otherwise was false
of the file it guarded: 2021-22 was wrong in four club-gameweeks by +1, +1, −1, −1 and summed to
exactly 8,360. Replaced by the direction that cannot cancel — no season above 8,360, no gameweek
above a double round — plus a per-club-gameweek audit in the harvest script, where the fixture
data is already loaded. Now **0 over-credited club-gameweeks** in every season.

**3. The headline comparison was not like-for-like.** (`7188ee5`) "0.000% against 2.36%" set a
two-sided statistic including doubles against a one-sided statistic excluding them. On the same
cells the rank rule scores **11.74%**. The harvest wins by five times more than claimed, and the
claim was still wrong.

**4. What the exact zero validates.** (`7188ee5`) Plumbing, not football — both sources descend
from the same official lineup feed — and the comparison is conditioned on the archive, so a
genuine disagreement where Understat has a start and the archive has no minutes is filtered out
before scoring. Also corrected: coverage is **99.8%** in 2023-24's 1-29 band, not 100% everywhere;
and the documented reconstruction fallback does not fire where the harvest reached a club-gameweek
and missed one player (3 rows, all 2021-22).

**5. `MinutesWeight` "inverts" was a movement nobody measured.** Every responding cell is 2022-23,
so three of four season means are identically zero, the clustered *t* is **pinned at 1.00 by
construction** — the `DefConCleanCoupling` degeneracy — and naive |t| ≤ 1.67. Half the recorded
sentence also survives: "wins none of the decidable cells" reads 0, 1, 2 of 24 against 4.8
expected by chance, so only the mean-rank clause moves. Corrected in `CLAUDE.md`, and
`constants-and-sweeps.md` — the note the index defers to, which carried none of this and asserted
the superseded verdict flatly in a heading — now carries the qualification with its body untouched.

**6. "Its price can never be measured here" is refuted by code.** `scoringPairNames` is a
seven-season `HOLD`-only grid that plays 2019-20, which the repair reaches — a **fourth cluster**,
taking df from 2 to 3. Found independently by the stats review and the findings audit.
Impossibility downgraded to unrun.

**7. The vice-captain ledger.** "Four fifths `c76c0d8`" is **129%** of the net drift once the
denominator moved; a reader taking four fifths of 0.0332 is out by 60%. Replaced with the chain,
and the last two terms marked single-season bookkeeping (5 and 4 cells, no clustered *t*) rather
than causes.

**8. "Each ladder's top order holds" is arithmetic.** 18 of 24 cells are byte-identical across
data states, so a stable ordering over all 24 confirms nothing; on the six that can move, both
orderings invert.

**9. The GW1 "gradient" does not hold, and it had displaced a correct caution.** The six
entry-point means are not monotone; the early-minus-late contrast is +2.03 pts/gw at
season-clustered *t* = **0.91** on df 2 and reverses in one of three seasons; GW1's own mean is
+0.36 at *t* = 0.17. Weaker than the season sign count the same document grades at p = 0.25. The
"one cell is not a reading" caution is restored.

**10. My own ledger inflation.** I recorded the starting elevens as an **eighth** failure of "the
archive does not have X is unverified until someone greps for X". It is not: the archive genuinely
lacks the column before 2022-23, and that was verified rather than asserted. It is the *widening*
of the rule, and "second instance after expected goals conceded" attached the punchline to the
wrong proposition — xGC was never fetched and discarded. Count restored to seven.

## Declined, with reasons

**The per-club-gameweek assertion is not in the Go test.** The bound is 11 × that gameweek's
club-fixtures, and a double gameweek legitimately exceeds 220 — 2018-19 GW32 carries 330, which is
11 × 30. The fixture count is not in the harvest CSV, so the sharper check lives in the harvest
script where `merged_gw.csv` is already loaded. The Go test keeps the bound that needs no join.

**The three uncovered 2021-22 rows stay as they are.** A player the harvest missed inside a
club-gameweek it otherwise reached keeps `Starts = 0` with the flag clear, because
`reconstructStarts` skips a club-gameweek once any start is recorded there. Fixing it properly
means teaching the reconstruction about partial coverage. Documented in both files instead: the
blast radius is 3 rows in five seasons and the direction is an under-credit, not a false start.

**`FINDINGS.md` §2's ordering bullets are annotated, not rewritten.** The document is a dated
record; annotating in place is this project's convention, and rewriting would destroy the sequence
in which the errors were found.

**The two-arm byte-identity check is not run.** Both reviewers independently concluded that at
shipped config **nothing in the scoring path reads `Starts`** — `reliabilityFrom` weights
`startShare` by zero *and* `blankRate` takes the unified-appearance branch — so the harvest should
leave every replayed cell byte-identical. Worth confirming; queued, not done. Note the
`fingerprint.go` comment still describes a squad-decision channel that is weighted zero twice
over, and wants the same correction.

**The remaining items from the two earlier reviews are still unapplied** and are recorded as such
in their own file. This pass took the ones that change a conclusion.

## What could not be checked on this harness

- **Whether the harvested starts are right where it matters.** The validation seasons are not the
  repair seasons, and the repair seasons have no recorded starts to score against — which is why
  they are being repaired. The per-club-gameweek audit (716/716, 272/272, 696 exact with 3 short)
  is the strongest available substitute, and it is a consistency check rather than an accuracy one.
- **Whether the two sources share an error.** Both descend from the same official feed, so
  agreement cannot rule that out.
- **The fourth cluster's size.** `scoringPairNames` makes it reachable; nobody has run it.
