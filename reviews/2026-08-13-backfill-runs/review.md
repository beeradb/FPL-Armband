# Two reviews of the backfill runs — captured, NOT applied, 2026-08-13

**Status: these findings are unapplied.** The runs and their write-up are on branch
`worktree-agent-abb7d9e9dfa570f3b` at `fb47a2f`, snapshot
`stats/snapshots/2026-08-13-4d61058/`. The agent that produced them dispatched a statistics
review and a findings audit, then **terminated on a session limit while waiting for them**, so
neither was folded in. They are recorded here because a reviewer's report lives only in an agent
transcript, and this project's most expensive failures are orphaned evidence rather than wrong
numbers.

⚠️ **This record discharges two of the three gaps that branch's own review record declares
outstanding.** `reviews/2026-08-13-fb47a2f/review.md` on `worktree-agent-abb7d9e9dfa570f3b` lists
the Run A statistics review and the record findings-audit as *"dispatched, no report — completed
twice without its report reaching this session. **Not discharged**"*, and marks the single-season
decomposition and the record edits as unreviewed on that basis. **Both reviews did complete and
did report — to the coordinating session rather than to the agent that asked for them**, which is
why that branch could not see them. Their content is below. The third gap it names, the
`TestEveryNoteIsIndexed` budget raise from 52 to 54 KB, **is** genuinely unjudged by an
independent reviewer; the findings audit reached it only as its own item 13 and is the same
agent, not a second opinion.

**Read the two records together, and prefer this one where they disagree about whether something
was reviewed.** Neither the write-up nor its figures changed between `fb47a2f` and that branch's
tip `64531fa` — the final commit adds only its review record — so every line reference below is
still valid at the tip.

Line references are into `stats/snapshots/2026-08-13-4d61058/FINDINGS.md` on that branch unless
stated otherwise.

**Terms.** `HOLD` is buy-and-hold with weekly re-picking, the metric that decides a scoring
question; `POLICY` adds the weekly transfer decision. A **cell** is one replayed season entered at
one deadline. **Season-clustered** treats each season as one independent unit; **start-fixed**
treats the six entry gameweeks as fixed. **`MINHL`** is the minutes half-life sweep block — how
sharply the minutes model discounts older gameweeks. **`FIXW`** is the fixture-weight block.
**`DCC`** is the defcon/clean-sheet coupling block. The four **data-state corners** are `shipped`
(both backfills on), `xgcoff` (expected-goals repair on, expected-goals-conceded repair off),
`xgoff`/`bothoff` (neither), and `aggoff` (weekly fills on, season aggregates not rebuilt).

---

## A. Statistics review — the corrections that change a stated conclusion

**1. The `+0.0200` expected-goals-conceded attribution in the drift ledger is stated as a cause
and is not one** (`:432-436`). Only **4 of 24 cells move**, not 6 — GW11 and GW16 are ties. The
four are +0.1316 / −0.1818 / +0.2222 / +0.3077. Naive *t* over 24 cells is **1.09**, and the
season-clustered *t* is **exactly 1.00 by construction** because three season means are
identically zero. The 2026-08-11 snapshot attached all of these warnings to its own −0.0108 and
said "do not quote a clustered t for it"; the new term has the same pathology and carries none.

**2. "Of which about four fifths is `c76c0d8`" has a stale denominator** (`:434`). In the
2026-08-11 write-up four fifths was of the 0.4634 → 0.4102 drift; in the new ledger the total is
0.4634 → 0.4302 = 0.0332, of which that commit is **128%**. A reader taking four fifths of 0.0332
is wrong by 60%. State the chain, not the fraction: +0.4634 recorded → −0.0424 model change,
mostly `c76c0d8` → **+0.4210** (`xgoff`) → −0.0108 expected-goals backfill → +0.4102 → **+0.0200**
expected-goals-conceded reconstruction → **+0.4302**.

**3. "The weekly fill carries essentially all of the effect" must be dropped** (`:405-406`,
mirrored at `:504` and `:404`). The standard error on the aggregate term is **1.481 pts/gw ≈ 56
points a season, |t| = 0.07** — it cannot distinguish "worth nothing" from "worth the whole
+82.7". And it is a **cancellation, not a null**: the six live cells are +3.89 / −1.70 / +0.25 /
−5.78 / +3.67 / −0.92 pts/gw, mean absolute 2.70 ≈ 103 points a season per cell, range −220 to
+148.

**4. "The ordering of the top two is stable in all four corners" is one observation counted four
times** (`:450-452`, `:471-474`). **18 of the 24 cells are literally identical in all four
corners**; all variation lives in 6. Restricted to those six the ordering differs in every corner
and inverts — `MINHL` 2022-23 makes *flat* best under `shipped` and *half-life 2* best under
`xgoff`, and `FIXW` 2022-23 makes 1.00 the best alternative in two of four corners, the opposite
of the pooled bullet.

**5. "The pipeline is validated across two days and a code change" overstates what the check
covers** (`:422-431`). The paired means match — on all four rungs, both corners, more than was
claimed. But the **cells do not**: 2025-26 GW26 moved in both arms and both corners (`policy`
713→754, `hold` 727→748, `moves` 11→9, `hits` 1→0), almost certainly the archive row guards at
`7cb769e`. The means survived by **paired-difference invariance**, which is exactly what makes the
check blind to what changed. It also licenses nothing about the repair-**on** arm, since `xgcoff`
skips the reconstruction entirely.

**6. "No *t* is quoted because none can be" is wrong** (`:388-389`). The entry-point axis gives
n = 6 and the predecessor snapshot quoted a standard error and *t* for this exact contrast. On the
six live cells: xGC −1.415 ± 1.285 (t −1.10); both +2.177 ± 1.854 (1.17); aggregate −0.0985 ±
1.481 (−0.07); weekly +2.275 ± 1.374 (1.66); implied xG/xA +3.592 ± 1.536 (**2.34**). None clears
2.571 at df 5. **Quote them with their standard errors** and label the contrast *unmeasurable on
this grid*, not "descriptive".

**7. The throughput figure that justified cutting scope appears nowhere in the logs**
(`:353-356`). `MINHL` ran 8 arms in 16:08-16:54 per process (~127 s/arm per process, ~25 s/arm
aggregate over five concurrent); `FIXW` 4 arms in ~9:30. The "four and a half hours" becomes
**~1.2 hours wall**. Either state the measured rate or drop the justification and call the cut a
judgement call.

**8. Smaller corrections.** The "implied" xG/xA row (`:396`) is a direct contrast between two
corners actually run — compute it directly and drop "implied"; what it hides is a *conditioning*
(expected-goals-conceded off in both arms, a state that no longer ships), not an interaction.
"P1 now holds on three independent sets of arms" (`:486`) is two sets, and not independent —
`xgoff` **is** `bothoff` by construction, so each repetition tests process determinism.
"Reproduces the 2026-08-11 6-of-24 result" (`:379`) should say *reproduces it for the expected-goals
contrast and extends it to two more*. The span "0.10 to 0.71 pts/gw" (`:480`) reconciles with no
reading except the table's global max−min. **Add standard errors to the ladders**: naive 24-cell
SEs run 0.31-0.56 pts/gw and **every one of the 28 ladder entries has |t| < 2.2, 25 of 28 below
1.6** — so "the levels move by up to 0.53 pts/gw" is a movement smaller than one standard error of
the quantity it moves.

**9. Two claims are understated.** The cross-process determinism control holds in **all five
corners** on all six outcome columns, not one; grid-invariance holds on **all 24 shared cells of
both arms**, not just the mean.

**10. An unclaimed reproduction stronger than the one claimed.** On the `vice off` arm,
`xgcoff − xgoff` restricted to 2022-23 gives **+3.6353 pts/gw, +138.1 a season, SE 1.540,
t = +2.36**, against the 2026-08-11 snapshot's recorded +3.635 / +138 / 1.540 / +2.36. Four
numbers, four matches. A **level** reproduction is strictly stronger than a paired one because it
is not protected by pairing. Lead with it.

**11. The cheapest unrun thing, which the write-up does not name.** The expected-goals repair
covers 2018-19 through 2022-23, so on the **six-season** grid the backfill and the aggregate arm
each reach **three played seasons and 18 live cells** instead of one and six, taking the clustered
degrees of freedom from degenerate to 2. Two of the four corners already exist. It needs
`TestDiagBaseline` at six seasons under each of the two switches: **two arms, ~3.5 minutes each**.
That is the one measurement that moves the aggregate-half question off "cannot be made".

## B. Findings audit — the record edits

**1. "Its price can never be measured here" is refuted by code** (`CLAUDE.md:509-513`).
`scoringPairNames` is a seven-season `HOLD`-only grid that **plays 2019-20**, which has no native
expected-goals-conceded column either, and the archive note's own table records that cell moving
+59 on `HOLD`. A fourth cluster exists, and the `HOLD`-only limit does not bite because `HOLD`
decides here. Structurally hard, not impossible.

**2. The six-season vice-captain figures are pre-repair** (`CLAUDE.md:415`). +0.4590 / +0.4313
come from a 2026-08-11 cells file taken before the reconstruction, which moves **18 of that grid's
36 cells** — and the same bullet corrects the four-season figure for exactly that cause. The
current six-season value is unmeasured; a 72-cell rerun settles it.

**3. Two switches or three.** `CLAUDE.md` says two (correct); `TODO.md` says three, twice. The
expected-goals-conceded repair is nested inside the expected-goals repair after its early return,
so the two are never both needed. What grew is **data states**, not switches.

**4. Stale counts and grids.** `TODO.md:1916-1917` still says "inert in 12 of 36 cells" where the
same commit fixed it to **18**. `harness-and-inference.md:3215-3216` still says the freed arms
"belong on `FPL_SWEEP_SEASONS=default`", where two thirds of the freed cells are off that grid —
they belong on the six-season grid. The same file still calls the 96-cell run unmeasured; it was
run.

**5. A shape the data does not have** (`archive-and-data.md`). "GW1 is genuinely the positive end
of a two-point-per-gameweek gradient" — the six means are +0.36 / −0.76 / +0.73 / −2.48 / −1.63 /
−1.64, which is not monotone, three cells each, no standard error, and a post-hoc axis. It also
retracts a still-true reason, and "three seasons of four" imports a season this grid never plays.

**6. A dropped hedge** (`CLAUDE.md:96`). "`FIXW`'s verdict against its shipped value changes sign"
omits the source's own qualifier — margins of **1 to 8 points a season, far inside the noise; a
warning about quoting a swept verdict, not a case for moving the constant**.

**7. Present tense for a repaired state** (`CLAUDE.md:148`). "Four of its six cells build the
opening fifteen with no expected goals at all" describes the pre-repair archive; the repair fills
it, which is why all six cells move.

**8. Checked and sound — do not re-audit.** The 18-of-36 count everywhere it appears; both switch
couplings; the `F_seas = 0.14` clustered-*t* correction carried into all three documents; the
`DCC` exclusion; the retained substitution-tercile figures; and the retractions in
`stats/README.md`, where nothing false was kept.

---

## What to do with this

Apply A1-A6 and B1-B4 before any of these figures is quoted elsewhere — they change stated
conclusions rather than wording. A10 and A11 are additions rather than corrections and are the
cheapest wins on the list.
