# The anti-residual arm: run, and it does not discriminate

**Run** `TestDiagGateOracleOnXPoints`, **6 arms × 36 cells = 216 cells**, one `run_id`, six seasons
by six entry gameweeks. Shipped data state — no `FPL_NO_*` switch. `EXP=anti-residual-gate`, through
`scripts/replay`, exit 0, peak RSS 130 MB, 15 minutes.

**Why.** The residual gate — `perfectGateResidual`, accept iff `ΔR − 4h > 0` — reads **−0.828
pts/gw on `policy_xpoints`**, and its *sign* was being read as informative. That reading is sound
only if a criterion carrying no information about the underlying gain would read zero, and nobody
had measured that. This is the arm that was supposed to measure it.

Two new arms:

| arm | criterion |
|---|---|
| **`ANTI`** (`transfergateanti`) | accept iff **`−ΔR − 4h > 0`** — the same criterion negated, hit charge unchanged |
| **`ACCEPTALL`** (`transfergateall`) | accept everything. **Not an oracle**: the no-gate policy, which this project has never measured |

The plan was reviewed before a line was written, and that review changed it: the contrast's null is
**not zero**. The finished work was reviewed again on both code and inference, and both of those
changed what is written here. The code review found a mis-route that passed every check the change
originally carried; the inference review found that the null comparison had no standard error and
that, done properly, one corner of it sits 2% inside the bar.

## The verdict

> **`unresolved`. The pre-registered test was run and could not discriminate.** `ANTI − RES` on
> `policy_xpoints` is **−0.229 pts/gw (−8.7 a season), CR2 t −0.38, 18 of 36 cells positive, wild
> p 0.7035**, against this contrast's own threshold of **58.1 a season**. Leave-one-season-out
> **changes sign** (+4.5 to −22.0). ⚠️ **And the null it should have been tested against is not
> identified** — see below — so the honest reading is *the test could not discriminate*, never
> *the null survived a challenge*.

**Scope: this touches the SIGN reading only.** The residual gate's realised-points half is
byte-reproduced and still resolves (+2.255 pts/gw, t 4.64, wild 0.0111, threshold 47.5). What falls
is the recorded *"the sign is informative only if an X-uninformative criterion would read zero,
which nobody has measured — that is what the anti-residual arm tests"*. That test has now been run
at 36 cells and is uninformative, so the sign reading is **unsupported with a measured failure to
corroborate**, rather than unsupported with the test outstanding. **Do not write an unscoped
"suggestive → unresolved".**

**The ceiling on this run was `RES`'s negative sign becoming unsupported-and-bounded rather than
`suggestive`, and that is what happened.** It cannot and does not make `RES` `established`.

## Provenance, and the two checks that have power

**The three banked arms reproduce exactly**, against `stats/snapshots/2026-08-15-gatescaled/`:

| arm | realised POLICY | `policy_xpoints` |
|---|---:|---:|
| perfect on POINTS | **+3.482** | **+2.204** |
| perfect on UNDERLYING | **+2.229** | **+1.942** |
| perfect on the RESIDUAL | **+2.255** | **−0.828** |

⚠️ **Stronger than "every printed digit": the CR2 standard errors and the wild-bootstrap p's
reproduce too** — `RES` on `policy_xpoints` reads SE 0.406, t −2.04, wild 0.0598 in both runs.
Matching SEs means the whole 36-cell vector is unchanged, not merely its mean.

**That is the check with no power, so the liveness half is stated beside it.** Confinement is a
*code* fact and re-running it can only fail. What must MOVE, and does: all five oracled arms move
`policy_xpoints`, and the transfer counts are 1104 / 1146 / 795 / 878 / 1739 against shipped's 891.

**Confinement.** `hold_points`, `hold_fixedcap_points`, `hold_nocap_points` and `hold_xpoints` are
**+0.000 in all 36 cells for all five oracled arms**. The held metric buys the opening fifteen and
never transfers, so no gate reaches it whatever its criterion.

**Three sweeps, byte-identical.** The code review's fixes landed between a first run and a second,
and a third ran from a committed tree so the provenance would read `dirty false`. All 216 cells and
every column of all three are identical — which is the checkable form of `SimConfig.gateLog`'s claim
to be an observer that cannot reach a decision, and the reason the fixes can be said to have changed
no behaviour rather than assumed to have.

⚠️ **`cells/*.provenance.csv` records `commit 5ba4a76, dirty false`, and that SHA no longer exists:
the branch was rebased onto a moved `origin/main` after the run.** The rebased commit is the parent
of this snapshot's own. What was checked rather than assumed: the two commits `origin/main` added
under `internal/analysis` are **comment-only**, and its one change under `internal/backtest` is a
**test file**, so nothing on the scored path moved and these cells stand against the rebased tree.
Recorded here because a provenance line naming an unresolvable SHA is exactly the defect this
directory exists to prevent, and marking it is cheaper than pretending it did not happen.

## The table

Thresholds are each row's own: `t_crit(df) × SE_CR2 × 38`, df 5.0, `t_crit` 2.5706. `S_eff`
(movable seasons) is **6 of 6** on every row, so the wild-bootstrap floor is `6/6^6` = **0.000129**
and nothing quoted here is floor-bound.

`policy_xpoints`, paired against shipped:

| arm | mean | a season | SE_CR2 | t | threshold | p raw | p wild | seasons + |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| perfect on POINTS | +2.204 | +83.8 | 0.399 | 5.53 | 39.0 | 0.0027 | 0.0096 | 6/6 |
| perfect on UNDERLYING | +1.942 | +73.8 | 0.347 | 5.59 | 33.9 | 0.0025 | 0.0081 | 6/6 |
| perfect on the RESIDUAL | −0.828 | −31.5 | 0.406 | −2.04 | 39.7 | 0.0971 | 0.0598 | 1/6 |
| **ANTI-residual** | **−1.057** | **−40.2** | **0.512** | **−2.06** | **50.0** | **0.0940** | **0.1006** | **2/6** |
| **NO GATE** | **−1.976** | **−75.1** | **0.387** | **−5.11** | **37.8** | **0.0037** | **0.0081** | **0/6** |

⚠️ **The Holm family is 5 here and 3 in the bank, and that is bookkeeping, not evidence.** The
harness defines the family mechanically as the alternatives present in the sweep and metric, and two
arms answering a different question were co-run. `RES`'s **raw p is 0.0971 and its wild p 0.0598 in
both runs** — nothing about `RES` moved. Only `p_holm` did, 0.0971 → 0.1880. Say which family a
quoted adjusted p belongs to, or the record reads as though new evidence weakened `RES`.

⚠️ **`ANTI`'s realised-points level is a NEGATIVE control and liveness only.** It reads **−4.105
pts/gw** — clearly negative, so the arm wired and the sign flipped. `Points = X + R` identically and
it accepts on the sign of `−R`, so a negative level there is guaranteed by construction. The
realised-points contrast `ANTI − RES` is **−6.360, t −11.23, positive in 2 of 36**: it is the
largest and most quotable number in this bank and it **discriminates nothing**, being doubly
constructed. It is labelled here because it will otherwise be quoted by whoever reads the file next.

## The null is not identified, and that is this run's substantive finding about its own design

The plan review established the estimand. Per offered package, with `μ = E[ΔX]` and
`p = P(ΔR > 4h)`:

    ANTI − RES = −cov(ΔX, sign ΔR) + μ·(1 − 2p)

The design wants the first term; the second vanishes only at `p = ½` or `μ = 0`, and neither is
known. `ACCEPTALL` was added to identify it, via `C = (ANTI + RES) − ACCEPTALL` and
`T = ACCEPTALL − C`.

**It does not identify it. Three reasons, worst first.**

**1. `T` is not sign-identified, and both signs resolve.**

| | pts/gw | a season | SE_CR2 | t |
|---|---:|---:|---:|---:|
| `T`, net of the hit charge | **−2.066** | −78.5 | 0.752 | **−2.75** |
| `T`, gross of the hit charge | **+4.361** | +165.7 | 0.738 | **+5.91** |

Both clear `t_crit` 2.5706, in opposite directions, off the same cells. An estimator that returns a
decisively negative and a decisively positive answer depending on a definitional choice is not
estimating a parameter. This is an **identification failure**, not a precision problem, and
presenting it as a range would misdescribe it.

The definitional choice is the hit charge, and it is forced rather than chosen. `moveLimit` bounds a
week at `free + 1` moves with `max_hits_per_week` 1, so an arm that refuses nothing exhausts the
bound every week: `free` ends each week at zero and starts the next at one, and either the funded
pair fires needing a hit or the singles loop runs twice with the second paid for. Measured: **821
hits over 918 cell-gameweeks against shipped's 73**. `res.XPoints -= float64(res.HitCost)`, so the
charge sits inside the level. Hit channel (`4 × hits / weeks`, computed per cell — the levels are
per-cell means, so a pooled hit channel would be a second estimator): shipped **+0.318**, `RES`
+0.257, `ANTI` +0.418, `ACCEPTALL` **+3.577** pts/gw.

The strip is exact arithmetic and `ACCEPTALL`'s accept rule does not read the charge, so no
acceptance decision is being counterfactually undone. ⚠️ It is **not** "what a hit-free policy would
score": the move sequence, the budget and the squad are all downstream of the hits actually taken.

**2. The partition premise fails structurally, and no number of cells repairs it.** `T` as "the
value of the whole offered stream, split into two complementary accept sets" requires `RES` and
`ANTI` to face the *same* stream. They do not — 795 moves against 878 — so the squads diverge from
the first week they disagree, after which each arm is offered packages the other never sees. There
is no `p` that both arms face, which is why the three estimates of `p̂` disagree by 57%. This is
prior to everything else here and is the reason the pre-registered test was never performable on a
replay.

**3. `p̂` from the mediator is not an accept rate.** `moves(RES)/moves(ACCEPTALL)` = 795/1739 =
0.4572 is a ratio of **executed moves** across two divergent paths, one of them saturating
`moveLimit`; `p` is defined per **package**. A funded pair is one package and two to five moves, and
`decide`'s singles loop returns on its first rejection, so a refusing arm forfeits the rest of the
week's moves for a reason unrelated to how often its criterion says yes. The per-package log gives
the package mass directly: **0.2911** on `RES`'s own stream, **0.3636** on the shipped stream. The
move ratio is printed as a labelled proxy and should not be quoted as `p`.

### Tested properly, nothing rejects — and the closest corner is 2% inside the bar

The null must be tested as **one per-cell contrast**, `Z = (ANTI − RES) − (1 − 2p)·T`, which carries
the covariance between the contrast and its own null — both are built from the same arms on the same
cells. Comparing two point estimates is this record's own named failure, paid for once on
`BlendRateK`.

| | Z, pts/gw | a season | t | threshold |
|---|---:|---:|---:|---:|
| net of hits, p̂ 0.2911 | +0.634 | +24.1 | +0.92 | 67.6 |
| net of hits, p̂ 0.3636 | +0.335 | +12.7 | +0.52 | 62.8 |
| net of hits, p̂ 0.4572 | −0.052 | −2.0 | −0.09 | 58.9 |
| gross of hits, p̂ 0.2911 | −1.874 | −71.2 | **−2.51** | 73.0 |
| gross of hits, p̂ 0.3636 | −1.242 | −47.2 | −1.76 | 69.0 |
| gross of hits, p̂ 0.4572 | −0.426 | −16.2 | −0.63 | 66.0 |

**None rejects at 2.5706 — but the worst corner is t −2.51, sitting 2% inside the bar.** Done the
naive way (the contrast against a null of +1.844 using the contrast's own SE of 0.595) it would have
read **t −3.48 and rejected spuriously**. So the honest sentence is *the test did not discriminate*,
and **not** *the null survived a challenge*.

**One thing genuinely reassuring, and it is the only robustness here.** The hit correction moves the
two arms in **opposite** directions — `RES` −0.828 → −0.885 (t −2.39), `ANTI` −1.057 → −0.937
(t −1.77) — and the contrast shrinks toward zero either way: −0.229 (t −0.38) net, −0.052 (t −0.08)
gross. So the *contrast's* near-zero reading is not an artefact of the hit treatment, even though
`C`, `T` and the arms' ordering all are. ⚠️ Net of hits the ordering is
`ACCEPTALL < ANTI < RES < 0`; gross of hits it **inverts completely**, with `ACCEPTALL` best. **No
ordering claim across these three arms may be made without naming the hit treatment.**

### Where the per-package figures come from

`internal/backtest/gatestream_test.go`, printed by the diagnostic and reproduced in `console.txt`
beside these cells. They are **not** in `cells/` and are not recoverable from it, which is why the
console output is banked.

| arm | offered | took | free | p | anti | μ(ΔX) | mean ΔR | −cov | μ(1−2p) | ratio |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| shipped | 1928 | 0.345 | 0.550 | 0.3636 | 0.4590 | +7.9212 | −0.6063 | −0.6984 | +2.1611 | −0.32 |
| on pts | 1562 | 0.440 | 0.635 | 0.3278 | 0.5403 | +2.6465 | −2.0024 | −0.8576 | +0.9115 | −0.94 |
| on xP | 1519 | 0.474 | 0.604 | 0.4042 | 0.4398 | +1.3801 | −0.4308 | −0.9026 | +0.2644 | −3.41 |
| on resid | 1807 | 0.291 | 0.841 | 0.2911 | 0.6397 | +7.7375 | −3.4326 | −0.8494 | +3.2329 | −0.26 |
| anti | 1797 | 0.331 | 0.800 | 0.5826 | 0.3306 | +8.4773 | +2.5405 | −0.5388 | −1.4011 | +0.38 |
| accept all | 905 | 1.000 | **0.093** | 0.3116 | 0.3624 | +8.1126 | −0.4562 | −0.7476 | +3.0568 | −0.24 |

**The pre-registered STOP condition fired.** It was written as *"p far from ½ AND μ materially
positive AND the ratio near 1 or below"*: `p̂` is 0.29-0.36, `μ̂` is +7.7 to +7.9, and `|ratio|` is
0.26 to 0.32 — the accept-mass offset is three to four times the antisymmetric term the run is for.
It does not change the verdict, because the verdict is a null and the contrast is a fifth of its own
threshold; it is recorded because it is the pre-registered reason this design could not have
discriminated even had the contrast come out large.

Note `free` = **0.093** on the accept-all row against 0.80-0.84 on the two gated arms. The sentence
*"the dead band has zero width whenever h = 0, which is the large majority of packages"* is true of
the shipped stream and of `RES` and `ANTI`; it is **false by construction** on the one arm whose
level defines `T`.

## A second, separate result: the no-gate policy

A different claim from everything above, and it deserves not to be buried.

**Accepting every offered swap costs 82.4 points a season on realised POLICY** — −2.168 pts/gw,
CR2 t −6.05, threshold **35.0**, wild p **0.0051**, **6 of 6 season means negative**, 33 of 36 cells
negative, LOSO **−77.1 to −92.9**. On `policy_xpoints` it is **−75.1** a season, t −5.11, threshold
37.8, LOSO **−67.1 to −84.2**.

**Gross of the transfer charge it GAINS 40.9 a season** — +1.077 pts/gw, CR2 t **3.47**, threshold
**30.4**, wild 0.012, 6 of 6 season means positive, LOSO **+31.9 to +46.1**. On `policy_xpoints`,
+48.2 (t 3.74, threshold 33.2). This is a per-cell quantity in the CSV (`policy_points + 4 × hits`),
so it has a standard error and clears its own threshold — a measurement, not a mechanism claim.

⚠️ **It is NOT a lower bound on the gate-constant family**, and the temptation to read it as one is
why this paragraph exists. `ACCEPTALL` differs from shipped in **three** ways at once: no value
criterion; `moveLimit` saturating at 821 hits against 73; and `acceptTransfer` bypassing
`shippedAccept` entirely, so `min_gain` and `free_transfer_value` are off inside it. Folding three
levers into one arm measures their sum and none of them. What it bounds is **no gate at the shipped
move and hit budget** — and the forced hit is part of what that phrase means rather than
contamination of it.

⚠️ **Do not pair it with the perfect gate's 106 to form a span.** That figure is four-season and
this is six — the grid mismatch this record has already suspended the 0.89 bar over. The
perfect-gate arm's six-season figure is +132.3 on realised points, in this same bank.

⚠️ **Do not write "the gate's value is largely hit avoidance rather than move selection."** The
+40.9 is a decomposition of `ACCEPTALL`'s own outcome, not of the gate's value, and the three-lever
confound blocks that inference. The safe sentence is: *accepting every offered swap raises squad
points by about 41 a season before the transfer charge and loses about 82 after it* — a statement
about the valuation, with the gate's own decomposition left unclaimed. Separating them needs a
`MaxHits: 0` arm, which is unrun.

## Concentration

`stats/concentration_screen.R`, both metrics, every pairwise contrast.

- `--metric=policy_xpoints_per_gw`: **1 of 15 arms flagged, and it is `ANTI − RES`** — `MOST CELLS
  DISAGREE`, 18 of 36 positive, and dropping its three largest cells moves it from −8.7 to −28.5.
  That is what a null looks like, and it corroborates the reading.
- `ACCEPTALL` is unflagged on both metrics: 33 of 36 cells negative on `policy_xpoints`, 0.87 of the
  mean surviving the top-three drop.
- `--metric=policy_per_gw`: no arm flagged.

## What must not be quoted from this run

- **A `−33 to +70 a season` null span.** It presents an identification failure as imprecision. Quote
  the `Z` table and the two `T`'s with their t's.
- **`ANTI`'s realised-points level, or `ANTI − RES` on realised points**, as anything but liveness.
  Both are construction.
- **Any ordering of `RES`, `ANTI` and `ACCEPTALL`** without naming the hit treatment — it inverts.
- **A four-season figure paired with any six-season figure here.**
- **`p̂ = 0.4572`** as an accept mass. It is a move-weighted ratio across divergent paths.
- **An unscoped "suggestive → unresolved" for the residual gate.** Its realised-points half resolves
  and is untouched.

## What this leaves open

**The question the arm was built for is still unanswered.** ⚠️ `ANTI` is **anti-informative, not
X-uninformative** — `−ΔR` is exactly as informative about `ΔX` as `ΔR` is — so it never could answer
it alone; only the pair against an accept-everything reference could, and that reference does not
identify. Whether a criterion carrying no information about the underlying gain would read zero on
`policy_xpoints` remains **unmeasured**, and this run establishes that it is not measurable by this
design.

The selection-arithmetic alternative — that selecting in-players on high realised residual and
out-players on low residual depresses accumulated xPoints whenever `X` and `R` are negatively
correlated across the candidate population — is **neither confirmed nor excluded** here, and remains
measurable off the archive with no sweep.

**The general rule this run establishes**, which is the thing worth carrying forward: an
antisymmetric pair of criteria cancels the common level but **not** the accept-mass asymmetry, so
the contrast's null is `T·(1 − 2p)` rather than zero — and identifying it needs an accept-everything
arm whose own budget constraints do not contaminate the level it defines. Here they did.
