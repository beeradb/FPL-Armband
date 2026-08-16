# The circularity guard measurement

Covers `9f95373`: `stats/xpoints_guard.py`, `stats/xpoints_bias.py`, the harness-note subsection and
the TODO addition.

⚠️ **First-party review, no reviewer agents dispatched.** As with the previous two commits, this is
a claim about the metric the record is measured on and belongs with the statistics reviewer before
anything is built on it. Nothing is built.

## No code changed

Documentation and two standalone scripts. `go build`, `go vet`, `go test ./...` clean.

## The strongest result, and why I believe it

**`exp(−xGC)` over-predicts clean sheets by 28.3%** — 3,459 predicted against 2,695 real, keepers
and defenders at 60+ minutes over three seasons. This is worth more than the headline because it is
an **independent reproduction of a recorded finding by a different route**: the record already has
the clean-sheet over-prediction shipping uncorrected "at about a quarter", measured inside the
model, and this is a raw-archive Poisson computation agreeing to a few points.

Two independent methods landing on the same number is the strongest form of evidence available here
and it did not have to happen. It also raises confidence in the rest of the script, since the same
loop produced both.

## What a reviewer should attack

- ⚠️ **The guard's power figure assumes independent player-gameweeks and they are not.** Squad-mates
  share fixtures; a club's clean sheet is one event counted across up to five owned players. The
  true SE is larger and the detectable bias worse than 1.26%. The note says so, but the headline
  number is still the optimistic one and would read as fact if the caveat were dropped. **This is
  the same weakness the previous commit's review flagged and it has not been fixed, only carried
  forward** — a proper cluster-robust version is owed before the figure is quoted anywhere else.
- ⚠️ **`exp(−xGC)` is a Poisson assumption**, and the 28.3% is partly a statement about that
  assumption rather than purely about xGC. `TestDiagCleanSheetPoisson` exists and would separate
  them; it was not run here. The agreement with the recorded "about a quarter" suggests the
  assumption is not doing much damage, but that is corroboration, not a check.
- ⚠️ **The attacking +0.060 has no explanation offered.** The likely cause is penalties — the record
  notes FPL's `expected_goals` excludes them — but I did not test that, and an unexplained +7σ bias
  in a proposed metric is not a detail. It is stated as a measurement, not diagnosed.
- **Three seasons only**, the natively-xG ones, so nothing here speaks to the four backfilled
  seasons where the instrument would be weakest.

## The judgement I would defend

The useful output is not "the guard has power 1.26%". It is **"run the guard per position group,
never only in total"**, which follows from the decomposition and would not have been visible from
the net figure. A −1.65% net reads as a small calibration offset and is in fact a +7σ and a −21σ
cancelling at whatever ratio the shipped squad happens to hold. Any arm that shifts the
attack/defence balance moves them differently.

## Gates

`go build`, `go vet`, `go test ./...` clean. No snapshot needed — no watched source file moved.
