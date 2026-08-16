# Building the xPoints instrument, and being wrong about it four times

Covers `6137b22..cfe257f` — the xPoints scorer, the `AxisTransferGateXPoints` arm, the run, the
corrections and the re-run.

**Dispatched, not first-party.** `fpl-code-review` and `fpl-stats-review` concurrently, twice: once
on the plan before anything was built, and once on the instrument and its result. Both found things
larger than what I had, both times.

## The result, after correction

**36 cells, six seasons, `POLICY`, CR2 clustered on season (df 5).** Both oracle arms are scored on
realised points; only what the oracle *knows* differs.

| oracle criterion | pts/gw | CR2 SE | t | Holm | per season |
|---|---|---|---|---|---|
| perfect on realised POINTS | +3.482 | 0.560 | 6.21 | 0.0032 | 132 |
| **perfect on realised UNDERLYING** | **+2.246** | **0.471** | **4.76** | **0.0050** | **+85.3** |
| contrast | −1.236 | 0.296 | −4.18 | — | −47 |

**The level is the finding: +85.3 a season against this comparison's own threshold of 46.0**, 29 of
36 cells, 6 of 6 season means. That licenses xPoints as a per-decision instrument and needs no
ratio.

The recovered fraction is **0.645, Fieller 95% CI [0.426, 0.835]**
(`stats/gate_recovered_fraction.py`). It **rejects equivalence** (t −4.18) and 0.89 (t −3.19) and
**cannot reject** the pre-registered 50% (t +1.83).

## What the code review found, and it was mutation-proved

⚠️ **The test I called load-bearing did not test what it claimed.** Replacing
`ExpectedCleanSheets(g.XGC, g.Fixtures)` with `(g.XGC, 1)` — which for a double *is* the summed form
the file exists to prevent — left the whole suite green. The test exercised the helper in isolation
and no test ever constructed a row with `Fixtures != 1`. A second test was outright vacuous: deleting
the guard it pinned changed nothing, because every other term in its row was already zero.

⚠️ **My first two replacements also passed under the mutation.** The first varied minutes and
fixtures together; the second used a defender, whose concede channel I had just made
fixture-dependent, so the rows differed through that channel instead. The working version varies the
fixture count alone, on a midfielder where the clean sheet is the only live channel — verified
failing under the mutation and passing without it. **Three attempts to write one honest test.**

**Four scorer defects**, all of which the result rested on:

1. The clean sheet was priced over the **club's** fixtures rather than the player's *eligible*
   appearances — a player who kept one leg of a double was charged an expectation of 2 against a
   realisable maximum of 1. **320 rows, 505 points** of inflation.
2. The concede deduction was gated on 60 minutes, which is the clean sheet's rule and not FPL's.
3. The concede realised side for a double was `floor((a+b)/2)` where `Points` already contains
   `floor(a/2)+floor(b/2)` — a phantom +1 on ~a fifth of GK/DEF doubles. Now left **realised** on
   doubles: a channel this file cannot get right is one it must not replace.
4. ⚠️ **The axis declared no invariance.** `mustNotMoveForAxis` fell through to nil, so Tier 2
   checked nothing, while the banner announced the arm as resting on "the input diff" — a guarantee
   that structurally cannot cover a decision axis. Nothing caught it, because the guard that
   validates declarations passes vacuously on an empty one.

Plus three guards the diagnostic had dropped from its predecessor, and a missing `xgc > 0` gate on
the Python residual.

⚠️ **One defect NOT fixed, and it is the deepest.** `baseXP90` applies `scaleFor(pos)` before pricing
xG and xA; this scorer uses raw values. A defender comes out **+0.29 xPoints per appearance** above
his realised points and a forward **−0.14** — about a two-point head start for a defender-in /
forward-out package over the gate's window. Documented with its measured size and queued.
**The "a bias shared within a position is not an ordering error" rule does not cover it: this gate
compares across positions.**

## What the statistics review found

- ⚠️ **The recovered fraction was quoted bare.** "A gap between two point estimates is not a result
  until it is divided by something" — and I had read the same estimate as "clears 50%" and "well
  short of 89%" in one paragraph. Divided, the pre-correction interval contained both bars.
- ⚠️ **"A gate that has never seen a goal" was FALSE.** `xPoints` is `points − residual` on four
  channels, so it retains realised **minutes** — the channel the record calls the sell side's
  *entire* error — and bonus. Therefore **not a lower bound** either: two biases run opposite ways.
- ⚠️ **The "~89% to tune a gate constant" arithmetic is withdrawn.** It divides one comparison's
  threshold by its own estimate; the same arithmetic on this run gives 41%. The gate-tuning closure
  rests on the older argument — constants worth 11-34 against thresholds of ~39.
- ⚠️ **132 is not an update of the recorded ≈106.** On the same four seasons this run gives 110.
  **All the movement is the two added seasons**, on a different data state.
- ⚠️ **The pre-registered mediator was not delivered** — transfer counts were reported instead of
  the disagreement rate. Stated rather than quietly substituted.
- `stats/concentration_screen.R` **cannot screen a POLICY-only arm** — it reads `hold_per_gw` and
  every oracle-on-transfers arm is byte-identical on `HOLD` by construction. Queued.

## The one genuinely satisfying part

**The four scorer fixes moved the estimate by 0.15 a season and cut its SE by 11%** (0.528 → 0.471,
t 4.27 → 4.76). The defects were adding **variance, not bias** — which is not what I would have
guessed, and is why the re-run was worth doing rather than assuming the direction. The points arm
came back byte-identical at +3.482, which is the check that the fixes were confined to the xPoints
path.

## Gates

`go build`, `go vet`, `go test ./...` clean at this commit. Both snapshots regenerated after the
instrument landed and after the corrections: **figures.csv differs by the commit stamp alone,
`constants.csv` byte-identical**, both times — the instrument does not reach the shipped scoring
path. `CLAUDE.md` untouched (~230 bytes of headroom; the compaction pass on
`worktree-points-spread-screen` is blocking, and the standing rules earned here are queued in
`TODO.md` for promotion once it lands).

**Nothing shipped changed.** No scoring constant, config field or objective term moved.
