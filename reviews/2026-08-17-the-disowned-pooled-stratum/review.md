# The record was quoting a stratum its own run disowns

## What was reviewed

Branch `stop-quoting-the-disowned-pooled-stratum`, off `origin/main` at `c589586`. Four files:

| file | why |
|---|---|
| `AGENTS.md` | the bullet on the clean-sheet factor and the defensive fixture ladder |
| `internal/snapshot/retracted_test.go` | a new `retractedFigures` entry for `3.30` |
| `internal/snapshot/notes_test.go` | the resident-record byte budget, 108 → 110 KB |
| `stats/findings/2026-08-15-clean-sheet-2x2.md` | two in-place corrections |

**The defect.** `AGENTS.md` carried, as a live verdict, *"the defensive fixture channel reads 1.5688
(SE 0.2253), **t 2.53 native and 3.30 pooled**, above 1 in **6 of 6 seasons**"*. Both `3.30` and
`6 of 6` are **pooled-stratum** quantities, and `stats/defensive_fixture_pointintime/fit.txt` heads
that stratum **"POOLED STRATUM -- CONTEXT ONLY, NO VERDICT"** and states **"THIS STRATUM CARRIES NO
VERDICT IN EITHER DIRECTION"**, because three of its six seasons carry reconstructed xGC so `w1` is
not one construct.

Written by different runs under different pre-registrations, so **nobody's error** — but the resident
record, loaded into every session, was leaning on evidence its own source says must not be quoted,
and nothing reconciled them.

**Nothing was measured.** No replay, no cell, no sweep. Every figure is arithmetic off standard errors
already banked in `fit.txt`, or a row count off a banked CSV. `internal/snapshot` is in
`ReviewWatchedPaths` but not `SnapshotWatchedPaths`, so no accuracy snapshot is owed — correctly, since
a documentation and test-constant change moves no measured figure.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | ✅ | triage: `AGENTS.md`/`docs/` — the record only |
| **fpl-stats-review** | ✅ | not owed by triage, dispatched anyway: the change re-attributes inference figures, and this reviewer produced them earlier in the session |
| fpl-code-review | ❌ | no scoring or agent code changed. The two Go edits are a test-data entry and a test constant |
| fpl-security-review | ❌ | no `internal/fpl`, `internal/agent` or config-persistence change |
| fpl-run-review | ❌ | no live run |
| fpl-season-maintenance | ❌ | no hand-maintained list touched |

⚠️ **A first version of this change passed both the byte budget and every guard, and was wrong.** It
is the reason this record exists in this shape: the reviewers found four claims stronger than their
sources, and the compression that made it fit inside 108 KB is what removed the hedges.

## Findings — applied

Ranked by how misleading the state was before the fix.

**1. The "recovered fourth cluster" is native in the archive column and 63.6% reconstructed in the
regressor.** *Applied.* This was the change's headline finding and it overstated. `w1` is the model's
`XGC90`, which `simulate.go` accumulates as `xgc += g.XGC` over every gameweek up to the cutoff, then
blends toward the prior season — 2021-22, reconstructed end to end. So a 2022-23 GW16-38 row carries
reconstructed data even though the archive's own `expected_goals_conceded` is native from GW16.
**Verified independently**: row-weighted `15/(gw−1)` over the 333 rows gives **0.6359** mean, 0.600
median, 1.000 at GW16, 0.4054 at GW38. And `csregressor_test.go` excludes the season for exactly this
stated reason, which the first version read past. Also added: the cluster contributes **15 of 333**
`def_moved` rows against 438 of 1566 native, so it buys a degree of freedom on the *level* and almost
nothing on the revision channel.

**2. The new guard comment was wrong about how many copies survive, and about scope.** *Applied.* It
claimed the only remaining copy was in `reviews/`, out of scope by design. In fact **two** survive and
only that one is out by design: `stats/findings/2026-08-15-clean-sheet-2x2.md` carries both withdrawn
readings and is scanned by **nothing**, because the guard globs `stats/*.md` **non-recursively** —
verified at `retracted_test.go:383-384`. `AGENTS.md` points a reader straight at that file two lines
above the edited bullet. Marked in place there. ⚠️ **Widening the glob is the real fix and is not done
here.** Separately, the comment said `stats/*.R` was in scope; it is not — `.R` belongs to
`TestNoLivePointerCitesTheRecordByPath`, which is the one difference between the two surfaces. The
advice was right, the stated reason wrong, and a future reader would have mis-scoped the next entry.

**3. A qualifier had become an assertion.** *Applied.* *"Implies `s ≈ 1.50`, so the excess sits on
`FPL_DEF_FIXTURE_SCALE`'s defensive half"* was unchanged from the old text, but its support was what
had just been removed — it now sat immediately after "does not resolve" and asserted a location off an
unresolved coefficient. `s` is also a **ratio**, `b2/b1`, whose standard error is neither
coefficient's. And the localisation is itself a contrast that has been divided: `t(b2−b1)` reads
**+2.11** native against the same `t_crit` of 4.303, banked in
`stats/findings/2026-08-15-clean-sheet-2x2.md`. Now: *points at*, better supported than the factor,
not established over it.

**4. The bullet's headline outlived its own body.** *Applied.* It read *"the defensive fixture ladder
is what runs hot"* — an establishment claim in bold, in a file read by grep one bullet at a time. A
first correction to *"reads high but does not resolve"* fixed the significance claim but flattened the
`f`-versus-`b2` asymmetry while the body still asserted the localisation. Final wording names both:
neither separates from 1 on the carrying stratum, and the ladder is the higher point estimate rather
than the established location.

**5. The resident file disagreed with the banked run in the fourth significant figure.** *Applied.*
The bar was written `0.5916` (recomputed at the rounded SE 0.1375); `fit.txt:104` banks **0.5918**
(unrounded 0.137543). One quantity, two implementations, agreeing on the day they were written — this
project's signature failure in miniature. Now quoting the banked value, with the MDE moved to 0.7377
for internal consistency. Zero net bytes.

**6. One word, two arms, thirteen lines apart.** *Applied.* The new text said *"not unmeasurable,
nothing being capped"* while the same bullet says *"Read this as unmeasurable"* about the hindsight
leak — and both runs print `VERDICT: C — UNMEASURABLE`, so a reader who greps the source finds the
word the bullet denies. Now states which arm each word belongs to.

**7. A season-agreement claim was deleted when an admissible one existed.** *Applied.* Removing "above
1 in 6 of 6 seasons" was right — it is a pooled claim. But the native per-season column is banked
too: 1.4814 / 1.4393 / 1.8872, so **3 of 3**. Restored on the admissible stratum and labelled
"agreement, not a magnitude", matching the source table's own header (*"shape, not a test — no
standard errors, so a sign flip in one season is not a result"*).

**8. The degrees of freedom were chosen, not resolved.** *Applied.* `t_crit(2)` → `t_crit(G−1 = 2)`.
A standing rule tells readers to take df from the comparison, citing a recorded miss where
Satterthwaite gave 1.72 against an assumed 2. `fit.txt:100` says the verdict uses the pre-registered
`G−1`, so the number is a choice and now says so.

**9. "Points-null across a fourfold width change" is a null the same file downgrades.** *Applied.*
`AGENTS.md` elsewhere calls that arm *"unmeasured on the current grid rather than a measured null"* —
3 cells at one entry point, no threshold, no banked cells, pre-dating a change worth −95 on one of its
three seasons. Pre-existing, but the fix made it load-bearing: with the fit's resolution downgraded, it
became the only thing holding the line closed. Now stated as unmeasured, with the closure resting on
that ground.

**10. A prediction was stated as fact.** *Applied.* *"2022-23 being the likeliest of the six to
disagree"* was grounded in a per-season table both runs forbid reading a shape off. Replaced with the
mechanism, which is checkable: it is the season whose `w1` is least comparable with the other three.

**11. A sign error in the evidence file.** *Applied, marked in place.*
`stats/findings/2026-08-15-clean-sheet-2x2.md` quoted the closed line as zeroing the defensive
response *"costs* 20 points"; `AGENTS.md` says ***gains***, and the recorded column settles it — 0 at
**2172** against shipped **2152**. Marked rather than rewritten, per that store's convention.

**12. The byte budget, raised 108 → 110 KB.** *Applied.* The corrections above are ~1.2 KB of
qualifiers. The constant's own comment forbids compressing hedges to fit and names raising it as the
remedy — *"compression and overstatement are the same operation past a certain point"* — and the
evidence is this change itself: the first version fitted inside 108 KB with 222 bytes free and was
wrong in exactly that direction. Now 807 free.

## Findings — declined, with reasons

**Carrying the CR0 rank deficiency into `AGENTS.md`.** *Declined.* The native season-clustered CR0
meat is rank 2 with 3 parameters, which is real. But `fit.txt:415-419` says in its own words that
**`df = 2` already encodes the real constraint**, and the bullet quotes df 2 explicitly — so the
constraint is already resident. Adding the mechanism would be evidence in a verdict-only file. Left in
the research note. This is a mechanism argument, not a preference.

**Adding `4.77` to `retractedFigures`.** *Declined.* One reviewer proposed removing the pooled `t 4.77`
or listing it. The figure appears in the sentence that **disowns** the stratum, which is the same
exemption already recorded for `0.89` — *"still quoted legitimately, in the sentence that does the
retracting"* — and an entry would fire on the correction itself, the guard eating its own fix. Instead
the sentence was rewritten to carry the referent without the number: *"is where it clears its own
bar"*. So `4.77` is gone from the file and needs no entry.

**Using `0.5917` rather than `0.5918`.** *Declined.* Both are banked — 0.5917 in the hindsight run,
0.5918 in the point-in-time run. The bullet cites the point-in-time run, so it quotes that run's value.

**Widening the `stats/*.md` glob to reach `stats/findings/`.** *Declined for this change, and it is the
largest thing left owed.* It is the real fix for finding 2, it is a guard-surface change that would
newly scan a directory of dated findings, and it belongs in its own commit with its own review — not
riding a documentation correction. Recorded in the guard comment so it is not lost.

**Re-measuring the fourfold-width points null on the current grid.** *Declined — out of scope.* Finding
9 only requires the two sections to stop disagreeing, which they now do. Whether the arm is worth
re-running is a sweep decision.

## What could not be checked on this harness

- **Nothing here was measured, and no figure is new.** Every number is arithmetic off banked standard
  errors or a count off a banked CSV. The claims about what a *refit* would return — the four-cluster
  bar of 0.4376 in particular — **assume the standard error unchanged**, which a refit would move. That
  is flagged in the text.
- **Whether a partial fourth cluster is admissible at all** is not settled here. The text now states
  the 63.6% figure and hands the question to a pre-registration.
- **`t(b2−b1) = 2.11` is quoted from `stats/findings/`, not re-derived.**
- **The full suite:** an earlier run failed on `TestTheProrationExposureCutIsNotTheEverPresentCut` with
  **HTTP 429** from `raw.githubusercontent.com`. Re-run in isolation it **passes**. That test treats
  rate-limiting as a failure rather than skipping the way an unreachable API does — a real gap, not
  this change's to fix, and recorded here rather than in the queue because it will recur.

⚠️ **Every finding above was verified against the tree before being applied**, not taken from the
reports: the accumulation mechanism in `simulate.go`, the non-recursive glob at `retracted_test.go:383`,
the banked 0.5918, the native per-season column, the two surviving copies, the `gains`/`costs` sign,
and the 0.6359 recomputed from the CSV. One reviewer figure was itself wrong and is corrected here —
a four-cluster bar of 0.4375, where `3.18245 × 0.1375` is **0.4376**.
