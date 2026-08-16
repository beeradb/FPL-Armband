# The point-in-time defensive fixture refit

## What was reviewed

`origin/main..HEAD` on `refit-the-defensive-fixture-coefficient-at-the-cutoff` —
a pre-registered, archive-side calibration refit asking whether the banked
defensive fixture coefficient `b2 = 1.5688` (`stats/cs_calibration.R`'s
two-channel decomposition) is inflated by the archive's end-of-season difficulty
column.

The predecessor arm (`defensive-fixture-coefficient-hindsight-gate`, three
commits, unmerged) tested this with a binary interaction identified off 221 of
1566 rows and returned UNMEASURABLE. It named what would resolve it and did not
build it. This branch builds it: `def` rebuilt from the opponent's strength as it
stood at each row's own cutoff, refit on all 1566 rows.

Three commits: the pre-registration and its frozen input (`8f86a01`), the fit
(`20deec1`), and the corrections this review produced.

**No Go code changed.** `HOLD` and `POLICY` are byte-identical by construction, no
replay cells were spent, no shipped constant moved, and no points figure is
produced anywhere. `CLAUDE.md` is not edited.

## The invariant came first

Per the skill's opening section, the question asked before dispatching anybody was
*what quantity must this change not move*. Four answers, all `fail()` rather than
prose, and all proven to fire:

1. **The refit must reproduce the banked fit.** Model E refitted here must return
   `b2_end` 1.5688, `b1` 1.0476 and the season × team CR2 SE 0.2253. **Proven to
   fire**: dropping 2023-24 aborts with `CONTROL FAILED ... b2_end = 1.6245`.
2. **The map must reproduce every archived difficulty**, 4560/4560, or the join
   aborts. The gate is residual-identically-zero, never a good fit.
3. **The banked `def` must equal `defenceMultiplier(end-of-season difficulty)`**
   on 2974/2974 rows, which pins `band_strength = 0` and `defFixtureScale = 1` and
   so guarantees `def_pit` and `def_end` sit on the same ladder.
4. **The frozen input must not move when the join's reporting grows.** The
   regenerated `joined_rows.csv` is byte-identical to the one committed with the
   pre-registration, checked by `diff`.

## Which reviewers ran, and which were skipped

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the triage row for `stats/*.R` — this is entirely an inference change |
| **fpl-findings-audit** | yes | same row; and the run produces a verdict that would otherwise enter the record at the wrong strength |
| fpl-code-review | **skipped** | no `internal/` change at all |
| fpl-security-review | **skipped** | no `internal/agent`, `internal/fpl`, config persistence or cache path touched |
| fpl-run-review | **skipped** | no live run, no config written |
| fpl-season-maintenance | **skipped** | none of the four hand-maintained lists touched |

⚠️ **A process failure worth recording: the statistics reviewer should have gated
the pre-registration before the fit ran, and did not.** The fit costs seconds, the
pre-registration was committed unedited before any fitting code, and every
correction below lands in `fit.txt` under a dated post-fit heading rather than in
the pre-registration — so nothing is unrecoverable. But the ordering was wrong and
the next arm of this kind should dispatch the reviewer on the plan.

**Every reviewer number below was reproduced independently before being applied.**
Two throwaway verification scripts re-solved the alternative poles, the paired
jackknife, the first-capture map and the live-payload denominators; all agreed to
six decimals and were then deleted.

## Findings, ranked by how misleading the current state was

### 1. The join claimed a check it did not implement — APPLIED, by implementing it

The docstring said it "checks that convention against the capture directory's own
timestamp and the archive's kickoff times rather than taking it on trust". It read
`GW(\d+)` out of the directory name and nothing else. `grep` for
`kickoff|timestamp|deadline` returned only the sentence. This is the failure mode
`CLAUDE.md` records against `restFactor`'s reporting-only call site: prose that
reads like confirmation.

The convention was in fact correct — the guarantee lives in
`internal/capture/backfilled.go`'s `VerifyPreDeadline` — so the substance survived
and the sentence did not. **Implemented rather than deleted**: the join now parses
the crawl timestamp out of each directory name, reads first kickoffs from the
archive, and **aborts** if any capture postdates the gameweek it names. It reports
**0 postdating in all six seasons**, and surfaces something previously unstated —
**six captures are more than eight days stale** (2020-21 GW3/5/7, 2024-25
GW8/9/10) because those directories share a crawl with an earlier gameweek.

### 2. The verdict letter is pole-specification dependent — APPLIED as a correction

§7 defines pole H on the **statistic** ("Model P returns exactly 1"). Defining it
on the **truth** instead — the true point-in-time slope is the shipped 1, with
`c_rev` solved so Model E still reproduces 1.568809 — is equally coherent. I
solved it: `c_rev` = 3.139961, Model P → **0.661538**, Model D contrast →
**+2.139961**. Separations become 0.773381 and 2.139961, and against the realised
thresholds **H1 would not fire** (1.9806 < 2.1400), so the decision rule falls
through to **D**.

Two things follow and the first is the reassuring one: **the pre-registered pole
is the conservative member of the pair on both statistics**, so the arm picked the
harder alternative and landed on the more cautious label. But "C" without that
caveat over-states, and `fit.txt` now says the verdict should be read as **"C or
D"**. Both forbid quoting, so nothing downstream moves.

Within the pre-registered definition the sizing is stable: the reviewer perturbed
`a` ± 0.15 and `b1` ± 0.2 and the contrast moved only over 1.157–1.222.

### 3. There is a third pole, and §4 denied it exists — APPLIED as a correction

§4 says "a negative contrast would say the cutoff column tracks outcomes better
than the end-stamped one, which no mechanism here predicts". **Pole P is that
mechanism**: FPL's contemporaneous rating tracking the opponent's strength in
*that* match better than a season-end aggregate does. Solved at `c_pit` =
2.305783, its Model D contrast is **−2.305783**.

So H1's null is not "no artefact" — it is "no artefact **and** `def_end` is the
true regressor", with two zero-hindsight poles at 0 and −2.31. **The canary is
unaffected**: the binding pairwise separation on the level is still N–H at 0.4349
(N–P is 0.8709, P–H is 1.3058), so naming pole P would not have flattered the
instrument. Had the contrast come out at −1.5 and rejected, the decision rule
would have called it "the framing is wrong" when it was pole P.

### 4. The paired estimator was rejected without being computed — APPLIED

I had rejected a paired `b2_end − b2_pit` on the grounds that there is no CR2 for
a stacked GLM system. The reviewer computed it. It has **exactly H2's pole
separation** (both poles pin Model E at 1.568809) and the two coefficients are
0.970 correlated across season clusters, so it is far better determined than
either level: leave-one-season-out jackknife SE **0.1770**, threshold **0.7618**
against 0.434919.

**It also fires**, which is the only reason it is quotable: a post-hoc estimator
that reaches the same verdict as the pre-registered arms cannot have been chosen
for its answer. It now prints in `fit.txt`, labelled post-hoc, and is the
estimator to use if this is ever revisited. The jackknife also cross-validates the
CR2 figures (`b2_end` 0.1421 against 0.1375; `b2_pit` 0.3128 against 0.3019).

### 5. §2 understates its own control — APPLIED

§2 calls "FPL re-published the difficulty when it revised a strength" unchecked.
The map check substantially checks it, and the sharpest form was never printed: a
column frozen at season start would have to be reproduced by the **first
capture's** strength, and that reproduces **2755 of 4560** against the
end-of-season strength's **4560 of 4560**. The frozen-column alternative is
**refuted**, not merely unlikely. The join now prints both.

What survives is narrower and is now stated as such: whether FPL *published* the
intermediate values. **And if it did not, the correct point-in-time column is the
season-start one — a third column this arm does not fit and S1's lag-1 arm does
not reach.** That is the honest residual.

### 6. Two denominators were inflated — APPLIED

Both headline "measurements" quoted a row count as if it were an evidence count:

- **4560/4560** is **93 distinct `(field, strength, difficulty)` constraints over
  120 club-seasons** — a club-season's 38 fixture-sides repeat one constraint,
  because `def` is one value per club-venue all season.
- **760/760 venue-matched against 456/760** discriminates on only **304 of 760**
  sides, from **8 of 20 clubs**: the rest face a club whose home and away ratings
  are equal, where the swapped map cannot fail. And the two capture rows printed
  were **one observation twice** — no team field differs between them.

Both are now printed with their real denominators, and the report now says that
the archive-side field screen forces the same venue pairing independently, which
is the cheaper and stronger support and was never claimed as such.

### 7. §6's "sharpest row" cannot fail — APPLIED

"`def_end` constant in 120 of 120 club-venue-season cells" is an arithmetic
consequence of the map check that runs earlier in the same script and is required
to pass. Monotonicity the construction forces is not evidence. The join now says
so in place, and names the two rows that *can* fail: the 438/1566 move rate and
`def_pit` varying in 59 of 120.

### 8. Three smaller leaks in the C verdict's own discipline — APPLIED

- **The `nearer pole N` branch-condition line was a reading.** `verdictv`
  short-circuits on C before the A/B branches consume it, so it was computed,
  printed, and never used — while being the line most readable as "the estimate
  favours no artefact". It is now suppressed under C with the reason given.
- **The pooled block quoted a native-stratum `H0`** (1.434919, an IWLS projection
  over the native rows and native control coefficients) and carried no canary, so
  a reader with a calculator could derive a "B-shaped" reading on the stratum the
  file declares carries nothing. That inference is now explicitly blocked.
- **The "why it does not resolve" block diagnosed from S3**, the table the same
  block says carries nothing. It now points instead at the season × team SE, which
  goes **0.2253 → 0.2336 (+3.7%)** against **+119.5%** on the season-clustered
  one. That rules out "the reconstructed regressor is simply weaker" without
  reading S3 at all. The block is legitimately outside the C prohibition — a
  season-clustered CR2 SE *is* the season-disagreement statistic — but it now says
  so rather than appearing to be a second reading.

### 9. The disclosure sentence is false as written — APPLIED as a correction

The pre-registration's preamble says "Not computed: ... any standard error, t, p
or threshold whatsoever", and §7 then quotes `SE(b2_end) = 0.1375`. That figure
was not computed for this arm — it is the predecessor's banked control SE — but it
**was known** when the canary was sized, which is exactly why §7 says H2 firing
was the expected outcome. `fit.txt` now records that the disclosure must be read
as "no standard error was computed *here*".

Both reviewers were asked whether a pre-registration that looked at its own
control is still one. Both said yes, on three checkable grounds: the control is
*required* to reproduce a figure the record already banks to four decimals; the
poles are functions of design plus that control and touch no outcome on `w2_pit`;
and §8 named **D as the modal expectation** in advance and got **C**, so it did
not name what it got.

### 10. The rank warning was selective — APPLIED

It printed on Model D alone, making Models E and P look better founded. With
G = 3 the cluster scores sum to zero, so the CR0 cluster meat is rank 2 in **all
three** models; CR2's adjustment lifts E and P to nominal full rank. `df = 2`
already encodes the real constraint. The line now states the CR0 rank for every
model.

### 11. A false claim printed in two other files — APPLIED

`stats/cs_calibration.R` and `stats/snapshots/2026-08-15-clean-sheet-2x2/FINDINGS.md`
both print "two live captures three days apart show 0 of 380 difficulties
changing, so revision looks rare". **Between those same two captures 0 of 20 clubs
changed any team field**, so the antecedent never fired and the observation cannot
distinguish "difficulty does not track strength" from "nothing moved". Both are
corrected **in place and dated**, marked as withdrawn rather than rewritten, and
both now point at the measurement that replaces the caveat.

⚠️ The findings audit checked and **`CLAUDE.md` does not carry this claim** — its
line is already conditional ("*if* difficulties ever moved mid-season") and needs
no amendment on this point. My §2 correctly named `cs_calibration.R` rather than
`CLAUDE.md`.

## What was declined, and why

- **Sizing the pooled poles and running the paired arm on six seasons.** Both
  reviewers name this as the one reachable measurement — pooled paired jackknife SE
  0.0826, so `t_crit(5) × SE` = 0.2123 against a separation of 0.4349, clearing by
  a factor of two. **Declined here.** It cannot be pre-registered blind: `fit.txt`
  already banks the pooled `b2_end` 1.5654 and `b2_pit` 1.4546, and the review has
  now additionally exposed the paired SE. Any pooled arm is a precision-motivated
  re-analysis of banked data and must be labelled as one. The separate objection
  also stands: `w1` is a different construct in three of those six seasons
  (reconstructed xGC), which is why the pre-registration refused a verdict there,
  and that objection does not vanish because the estimator got sharper. **Recorded
  as available and unrun**, with its three preconditions.
- **"De-contaminating a regressor is not free" as a standing claim.** Declined. It
  is n = 1; the ratio is between two variance estimates each carrying 2 df from the
  *same* three clusters, so it is not itself a resolved quantity; and the same
  ratio on the pooled stratum is only about 1.5. `fit.txt` now records the three SE
  columns and explicitly refuses the aphorism.
- **Editing the pre-registration.** Forbidden by its own §8. Every correction
  above prints in `fit.txt` under a dated post-fit heading, and every one of them
  moves the record toward quoting *less*.
- **Editing `CLAUDE.md`.** Out of scope by instruction. The findings audit's
  proposed entry is recorded below for whoever lands it.
- **Re-running the fit under the alternative pole H′ as a second arm.** Declined:
  it would be choosing a pole after seeing which verdict each gives, which is
  exactly what §8 forbids. It is recorded as a correction to the *label*, not as a
  second result.

## What could not be checked on this harness

- **Whether FPL published intermediate difficulty values mid-season.** The
  per-gameweek captures hold `bootstrap-static.json.gz` and no fixtures payload, so
  no archived capture can show a difficulty. The frozen-at-season-start
  alternative is now refuted (finding 5), but "computed live and never published"
  is not reachable. The sibling queue item to add a fixtures payload to the weekly
  capture is partly already answered by finding 5 and would settle the rest.
- **H2 at df 2, ever.** The native stratum cannot grow: `data/captures/` starts at
  2020-21 and earlier seasons carry no native xGC. Three seasons is three seasons.
- **The replay's actual exposure to the leak.** The fitted population runs GW7–38,
  and the reconstruction disagrees with the archived difficulty most in GW1–6 —
  22%/60%/40% of fixture-sides in 2023-24/2024-25/2025-26 against 0%/15%/8% in
  GW31–38. The estimand is defined on the fitted population and is unaffected; any
  statement about what the *replay* faces understates it.

## What this suggests the record should eventually say — not applied here

One entry, and it is a **harness fact rather than a calibration one**, so it is
untouched by the C verdict:

> **The replay's fixture difficulty is not point-in-time.** `playedFixtures`
> strips the scoreline and `Finished` and not `team_h_difficulty`, so
> `defenceMultiplier` scores every gameweek off the opponent's end-of-season
> strength. FPL's difficulty is an exact step function of the opponent's fine
> `strength_overall_*` rating (4560/4560 archived fixture-sides; 2755/4560 from
> the season-start value, so the column is end-stamped and a frozen column is
> refuted). Reconstructed at each row's own cutoff, the level differs on **438 of
> 1566** scored native team-gameweeks, and worse earlier in the season.
> **The size of the consequence is unmeasured**: the refit is
> **C — UNMEASURABLE** on three season clusters with both canaries firing, and
> the reconstruction more than doubles the season-clustered SE
> (`stats/defensive_fixture_pointintime/fit.txt`).

And one clause for the predecessor's canary lesson, wherever it lands: **size a
canary against the separation between two candidate DGPs, never between one DGP
and the observed estimate.** The naive figure over-states by 30.8% here. That is
the same error class one level up from the predecessor's OLS omitted-variable
formula, and this arm caught it before the fit — though here it changed the margin
and not the answer.

Finally, `internal/analysis/metrics.go`'s `FPL_MAGNITUDE` path is the code-side
answer to this leak and is never cross-referenced: it builds team rates from
finished fixtures only, which in a replay are already cutoff-stripped, so it
removes the end-stamp on the scored path with no reconstruction and no assumption
about what FPL published. It ships off. Anyone reading the C verdict and asking
"so what do we do about the leak?" should be pointed there, and at the record's
existing finding that the defensive ladder is points-null across a fourfold width
change, before opening anything new.
