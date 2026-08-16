# Bounding the expected-points instrument

Covers `621c226`: `stats/xpoints_variance.py`, the harness-note section, and the TODO item.

⚠️ **First-party review, no reviewer agents dispatched.** ⚠️ **And this is a proposal about the
metric the whole record is measured on**, which is squarely the statistics reviewer's territory and
should go to them before anything is built. Nothing is built, which is deliberate — the commit
bounds the idea and stops.

## No code changed

Documentation and one standalone script. Nothing under `internal/`, no test, no shipped constant.
`go build`, `go vet`, `go test ./...` clean.

## The measurement, and what it does and does not support

**What it supports.** 33,878 player-gameweeks with minutes, on the three seasons carrying native xG,
so no backfill enters. The conversion residual is 25.6% of per-player-gameweek points variance and
36.0% including the clean sheet. Those are simple sample variances of quantities read straight from
the archive, and the arithmetic is checkable by re-running the script.

**What it does NOT support, and the note says so.**

- ⚠️ **The 20% sd claim assumes players are independent within a gameweek.** They are not — squad
  members share fixtures, and clean sheets in particular are perfectly correlated within a club.
  The *ratio* is more robust than the level, which is why the note quotes the ratio, but a squad
  with three players from one defence would see less benefit than 20%. **This is the weakest step
  in the chain and a reviewer should attack it first.**
- ⚠️ **"Threshold 39 → 31" is a linear extrapolation** from an sd reduction to a detection
  threshold. It holds only if the residual is independent of the paired contrast, which is
  plausible and untested.
- ⚠️ **`exp(−xGC)` as P(clean sheet)** is a Poisson assumption. The repo has
  `TestDiagCleanSheetPoisson`, so this is checkable rather than asserted, but it was not checked
  here.
- ⚠️ **Bonus is left realised**, and bonus is driven by BPS which is driven by goals. So a scored
  goal still pays bonus while its xG replacement does not remove it — the instrument is a hybrid,
  not a clean expected-points model. Defensible, and stated, but it means the 36% is not the whole
  conversion channel.

## The judgement call, and it is the important part

The commit's central claim is **not** that the instrument is good. It is that the decisive question
is "does it remove less signal than variance", that the captaincy work already answered exactly this
shape of question **negatively**, and that two positive controls exist to answer it here. That is
the right framing and it is the one thing I would defend.

⚠️ The circularity risk is stated but not quantified: the model predicts from xG and the metric would
score on xG. I claim realised points must stay the arbiter where the two diverge. A reviewer could
reasonably ask for a demonstration — e.g. an arm known to over-weight xG, scored both ways — before
the instrument is trusted for tuning at all.

## Gates

`go build`, `go vet`, `go test ./...` clean. No snapshot needed — no watched source file moved.
