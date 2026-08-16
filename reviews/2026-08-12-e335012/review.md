# Review record — measuring what real team news is worth

**Commits reviewed:** `7647544..e335012` on `oracle-team-news2` (branched from
`flag-refit`). The reviewers themselves ran over `7647544..a29f0a7`; everything after that
commit is the corrections they asked for, plus the snapshot regeneration. Previous record:
[`2026-08-12-540658c`](../2026-08-12-540658c/review.md).

**What the change is.** Six seasons of FPL's own pre-deadline `bootstrap-static` captures
exported to two embedded tables and fed to the replay as an information oracle in place of
`statusAt`'s end-of-season availability reconstruction, in two nested arms — the real flag,
and the flag plus `chance_of_playing_next_round`. Plus the sweep diagnostic, a reach
diagnostic, the research-record entry, and two accuracy snapshots.

## Before the reviewers: what must this change NOT move?

Per the skill's first section, invariants first. What was written, and what each caught:

| invariant | outcome |
|---|---|
| the zero `Oracles` leaves availability untouched (`TestTeamNewsIsOffUnlessAsked`) | passes; the default path is the thing being pinned |
| a covered gameweek is authoritative **in both directions** | passes — this is the property that makes the arm a replacement rather than a patch |
| an uncovered gameweek falls back wholesale | passes |
| the join is on permanent player **code**, with ids crossed over so a wrong join silently flags the wrong man | passes |
| only the second arm sets the percentage (else the contrast is zero by construction) | passes |
| `Validate` refuses a sourceless arm, a chance-only arm, and composition with `OracleAvailability` | passes |
| the `TeamNews` dynamic type is comparable (`Oracles` is compared with `!=` per cell) | passes |
| the export carries `d` flags and percentages — the two things `statusAt` cannot produce | passes |
| every season the grid plays has coverage | passes |
| the staleness filter actually drops gameweeks | passes |
| a player with no permanent code is left alone | passes; **verified a no-op on this archive** — zero such players in all six seasons |
| Tier 1 field diff: the arms perturb `Status` and `ChanceOfPlayingNextRound` and nothing else | passes |
| accuracy snapshot: every model figure byte-identical, twice | passes |

**Three defects were caught by the repository's own guards rather than by a reviewer**,
which is the skill's point restated: `TestTheGridIsDeclaredOnce` caught the reach
diagnostic re-declaring the six entry points as its own slice;
`TestEnvSwitchListIsComplete` caught `FPL_TEAMNEWS_MAX_HOURS` being unfingerprinted, so a
run under it would have been stamped as shipped defaults; and
`TestSnapshotCoversTheCurrentCode` caught the snapshot twice. All three were free.

## Reviewers dispatched

Triage: the change touches `internal/backtest` and `CLAUDE.md`/`docs/`, so the union is
**fpl-stats-review** and **fpl-findings-audit**. Both ran concurrently, read-only, on the
scratch worktree.

**Skipped, with reasons.** `fpl-code-review` — nothing in `internal/analysis` moved and no
byte-identity refactor is claimed; the seam is 30 lines guarded by a Tier 1 field diff.
`fpl-security-review` — no credential, no network, no agent-facing surface; the captures
were already committed. `fpl-run-review` — no live run and no config written.
`fpl-season-maintenance` — no hand-maintained list touched.

## Findings, ranked by how misleading the state was

### Applied

1. **The framing was wrong: this is not hindsight** (stats 5). A capture taken hours before
   a deadline holds strictly less than a manager has at it, and `availabilityFactor` already
   reads both fields off live `bootstrap-static` — so the shipped model behaves like the
   second arm and it is the *replay* that was blind. Every replay figure in the record is
   measured under an availability model worse than what ships. Rewritten; the percentage
   contrast is now recorded as a live shipping candidate, explicitly unjustified.
2. **The held headline is carried by one season** (stats 1, audit 4). Median cell +0.216;
   drop 2023-24 and the other five average +0.401/gw ≈ +15 against the pooled +33. The
   dose-response corroboration dies with it — r +0.509 falls to +0.055 — and the highest-reach
   cell in the grid is the same cell. Both figures now travel together, 15 named as central.
3. **Ties were booked as evidence** (stats 2). The `POLICY` contrast is 22 negative, 10
   positive, 4 tied — not "26 of 36". This is the exact defect the record catalogues in
   `shape_inference.R`, arriving by hand in prose. Corrected in the note and in `CLAUDE.md`.
4. **The held contrast's 18 tied cells are "could not run", not "worth nothing"** (stats 3),
   and the reason is exact: doubtful players are precisely who `MinExpectedMinutes` removes
   from the pool. Reworded, and the null is now stated as *bounded* to ±3 a season.
5. **The arms differ on exactly one status** (stats 3, audit 8). Counted from the export:
   `chance` is determined by `status` everywhere except `d`; 7,163 doubtful rows, repriced up
   over down at 2:1. Mechanism counted rather than asserted.
6. **The record already measured the "if"** (audit 7): 0.545 / 0.256 / 0.085 of normal
   minutes at 75/50/25, on 51,598 paired observations — so the contrast's sign was predicted
   in advance. Added with its own caveat (a minutes ratio against a `Score` multiplier).
7. **Coverage overstated**: 228 captured, **224** fed the replay. The freshness tail is now
   reported (25 gameweeks over a day, two over a week) with the direction it biases, and
   2020-21's anomalous 783 "news better" is flagged as undiagnosed (audit 5).
8. **The estimator was chosen silently** (stats 6, audit 11). `variance_components.R` run:
   `POLICY`'s `%seas` is 1.9, so the fixed-block threshold of 28 is indicated against the
   quoted clustered 37. Both reported; verdict unchanged.
9. **The entry-point shape** (stats 4, audit 15): cited at six points not three, noted as
   partly mechanical since the windows are nested, and the mechanism usually offered is a
   `HOLD` argument while `HOLD` is flat.
10. **The pre-registered direction failed** (audit 9). The diagnostic predicted `POLICY`
    would carry it; `HOLD` is larger. Recorded as a failed prediction rather than quietly
    dropped, and the "which is why `HOLD` shifts so little" claim removed from both files.
11. **The dividing line contradicted a worked proof** (audit 2). It implied paired results
    break across a repricing; the vice-captain fix proves they do not. Rewritten, and it now
    names the constants that genuinely interact.
12. **The isolation sentence credited a guard that cannot see the change** (audit 1).
    `go:embed` puts 101,605 flag rows inside `internal/backtest`, so the import scan passes
    without visibility. The note now says the `Oracles` gate is what protects the record.
13. **`MinExpectedMinutes` is not unmeasured** (audit 10) — swept once, weakly, and against
    the direction the reach table would motivate. Corrected.
14. **"Floor" was the brief's word and is wrong** (audit 17). The record's word for ≈14 is
    *design average*; ≈32 conditional on firing. Fixed in `teamnews.go` and the note.
15. **Seasons-to-resolve** was asserted at thirteen; computed, it is **eleven** (stats 1,
    audit 12). Fixed before the reviews landed.
16. **Two competing superlatives** for one pattern (audit 6). Dropped; the season-average
    contrast keeps "sharpest" and this one claims only the input axis.
17. **The `CLAUDE.md` bullet** was the longest in its index and repeated the failed
    prediction (audit 14). Shortened, corrected, glosses added.

### Declined, with reasons

- **A 48-hour staleness arm** (audit 5). A 24-hour arm was already run and is in the note.
  It reproduces the contrast (−0.432 against −0.480) and attenuates the flag arms — but with
  coverage already complete a filter can only *remove* information, so attenuation is
  mechanical and says nothing about whether the dropped captures misled. A 48-hour arm would
  land between the two and answer the same non-question. What would answer it is two crawls
  of one deadline at different distances, which `Store.AllAt` supports and this backfill
  mostly does not carry. Recorded as the design, not run.
- **In-place marks 3b and 3d** (audit 3). The mark at the `PointInTime` build claim was
  applied, and the `CLAUDE.md` weekly-capture bullet was not. Both are true and both belong
  to the *capture* programme rather than to this measurement; `CLAUDE.md` is edited
  concurrently by other agents and a second bullet-level edit is conflict for little gain.
  Left for the capture owner, named here so it is not lost.
- **Cross-grid comparison of ≈14 against +33** (audit 13). The audit computed the
  four-season restriction (+30 held, +21 with transfers) and found the comparison survives.
  Not added: the note no longer leans on the comparison — it says the stand-in's *shape* is
  wrong, not its size — so the arithmetic would be supporting a claim that is no longer made.
- **Glosses on `PointInTimeWith`, `PreSeasonWith`, `EngineAt`** (audit 17). Applied to
  `availabilityFactor` and `MinExpectedMinutes`, which a reader needs to follow the argument.
  Declined for the three replay internals: they appear in a section about the replay, in a
  file whose reader is already inside it.
- **Re-running anything on the replay.** Both reviewers concluded the grid is sound and the
  prose was the problem. No sweep was re-run.

## What is left open

- The contrast wants **independent football**, which this archive does not contain. The
  honest entry is *awaiting 2026-27*; the 24-hour arm shares every cell and is not a second
  draw.
- Whether 2023-24 is signal or a draw is unresolved, and the note carries both readings.
- The oracle banner prints "upper bound" for every arm, which is wrong for this one. Fixing
  it per-oracle is a change to shared machinery this measurement did not need; noted in the
  research record instead.
