# The rebase onto main, and a measurement corrected twice by review

Covers `e5fa796..be520ef` — the rebase onto main's A2 cells-reader refactor, the scoring-table
pin, and the per-move residual measurement.

**Dispatched, not first-party.** `fpl-code-review` and `fpl-stats-review`, concurrently, with
briefs naming where I thought my own work was weakest. Both found things I had not, and in both
cases the finding was larger than what I had. The statistics review also gated the *plan*
(`docs/decision-scoring-design.md`) before any of it was built, which is what stopped a
byte-identical-zero control from being built into a go/no-go.

## The headline: the measurement was wrong twice, in two different ways

`stats/xpoints_permove.py` asks whether scoring a transfer decision on realised underlying
removes the ~36% of variance that the per-appearance figure suggests.

| version | 5-gw share | sd cut | what was wrong |
|---|---|---|---|
| first | 11.1% | 5.0% | three figures from **uncommitted** shell commands; one wrong |
| second | 10.0% | 2.1% | right quantity, **wrong population** |
| **current** | **~21.6%** | **~16%** | inside the shipped pool floor, ex ante |

**Neither correction was arithmetic.** The first was reproducibility, the second was population.

### What the code review found

- **Three of four published figures did not come from the committed script.** The 35.9% reference,
  the 23.3% all-rows figure and the "differencing preserves it" step were computed in throwaway
  shell commands. ⚠️ **The one that could not be regenerated was the one that was wrong**: the
  "65.3% of rows have no minutes" used `residual == 0.0` as a proxy for not having played, which is
  false for a 1-59 minute cameo with no goals or xG. The true figure is **60.1%**.
- **A control that could not fire.** The `bothplayed` calibration arm gated on *having a row*
  rather than *having played*, so the arm whose entire job was to check the pipeline against 35.9%
  read 22.1%. In the file added to gate a design.
- **Cross-season pairs.** 2,642 of 4,000 in the operative cell paired two different seasons,
  matched on gameweek *number* rather than football, with the price filter comparing two seasons'
  price scales.
- **Windows ran past GW38**, padding both players with zeros and mixing short windows into the long
  rows — biasing the very decay being reported.
- Two `concentration_screen.R` defects: the `MIN_CELLS` drop report sat *after* the "no screenable
  arms" `stop()`, so the one case where it matters most died with the reason unprinted; and a dead
  `if (nrow(bl) == 0) next` concealed a **third**, unargued behaviour change from the migration.
- The scoring-table guard left `assistPoints` and the 60-minute threshold unpinned, one line from
  what it was written to close.

### What the statistics review found, which was larger

- ⚠️ **The population was wrong, and it was most of the answer.** The script sampled all 2,490
  player-seasons; the replay does not transfer within that field. Gating both players on
  `MinExpectedMinutes` — **ex ante**, over the prior six gameweeks — moves the five-week share
  10.0% → **~21.6%** and the sd cut 2.1% → **~16%**. Reproduced here independently: 21.6% against
  the reviewer's 21.4%.
  **The tell was already in the record**: 13% of genuinely sold players stop playing, against
  60.1% blank rows in the sample. The script's header called the ungated figure the *upper* end of
  its bracket; it is the lower one.
- ⚠️ **The sd cut is the one statistic the sampler cannot pin down.** Monte-Carlo mean ~3.7%, range
  [1.9, 5.3], reaching zero at eight weeks. **2.1% was a tail draw** — and the pre-fix and post-fix
  pipelines have the *same* MC mean, so my recorded "5.0% → 2.1%, from the three code fixes" was
  **reseeding described as a correction**.
- ⚠️ **Reseeding measures the wrong noise.** Between-season SE is 0.72pp ungated and 1.32pp gated
  against a MC sd of ~0.44pp, on **df 2, `t_crit` 4.303**. The uncertainty block I had just added
  pointed at the smaller source and omitted the unstable statistic.
- ⚠️ **The mechanism attribution inverts with the population.** Inside the pool, one gameweek is
  33.5% against 35.9% for appearances — **blanks cost ~2 points, not ~12**, and are mostly a
  property of players nobody would buy. The **horizon** is the real channel.
- ⚠️ **The two mechanisms do not decompose.** Multiplicative main effects predict 19.6% against
  10.2% observed: a residual interaction of **×0.52, larger than the horizon main effect**.
- ⚠️ **The comparison to the 4.03× N inflation is withdrawn entirely.** Different estimators of
  different quantities, and *opposite roles* — the 4.03× removes power that never existed, an sd
  cut adds real power. They compose. It was "a gap between two point estimates is not a result
  until it is divided by something", committed twice in one paragraph.

**The verdict moves with it.** "Smaller than the clustering correction" is withdrawn; ×1.42
effective N is modest and real, and ⚠️ **the go/no-go arm must not be declined on this.**

## What survived

- **The 35.9% genuinely does not transport to a transfer.** Both reviews confirm it; only the size
  moved.
- **Both named positive controls are byte-identical zero on a transfer metric** — verified in the
  code before either review reported: `decide` never reads the captain (`simulate.go:1771`) and
  `viceCaptainFallback` is scoring-time only (`replay.go:263`). This was the plan-gating catch.
- **`perfectGate` on `xNet` is the single go/no-go arm**, against its recorded ≈106 a season.
- **A per-decision mean is not a paired difference** — measured counter-example in the record:
  points per transfer is monotone in restraint (66 at 273 moves to 88 at 206) while season points
  are not.
- The paired-differencing frame, the per-fixture clean sheet, the Go-not-Python argument, the
  per-position guard prescription, and the `carriers` key fix (verified live: the screen prints
  both `seas 2` and `seas 3`, which the bug could not have produced).
- The A2 migration itself: `read_cells` is strictly stronger than the check it replaced, and
  `TestTheSharedCellQuantitiesHaveOneImplementation` passes.

## The guard that was off while all this happened

⚠️ **`TestReviewCoversTheCurrentCode` SKIPPED on the rebase** — it ran, printed PASS, and diffed
nothing, because every review record named a commit the rebase had replaced. Its sibling
`staleness_test.go` carries the identical check against snapshots and its comment records the
identical bug being fixed *there*. **The repair went to one of two guards and not the other**, and
the half left behind is the one deciding whether unreviewed code reaches main. Now fatal.

## Gates

`go build`, `go vet`, `go test ./...` clean at this commit. `concentration_screen.R --committed`
reproduces its motivating figure unchanged (`BlendRateK=16 minus BlendRateK=12`, n 36, +23.3,
17/36, surv 0.01, seas 3). `xpoints_bias.py` unmoved at +33.1%. Snapshot regenerated post-rebase
differs from its predecessor by one line, the commit stamp, with `constants.csv` byte-identical.
`CLAUDE.md` untouched — it has ~230 bytes of headroom and the compaction pass on
`worktree-points-spread-screen` is blocking; the three standing rules earned here are queued in
`TODO.md` for promotion once it lands.

**Nothing shipped changed.** No scoring constant, config field or objective term moved.
