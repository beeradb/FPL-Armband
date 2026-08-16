# The concentration screen and the cell forensics probe

Covers `832c42b` (`stats/concentration_screen.R` and the record updates) and `24acdc3` (the
`HoldCaptaincy` per-week fields and `TestDiagCellForensics`).

⚠️ **First-party review again — no reviewer agents were dispatched**, per the session's standing
instruction. Stated plainly rather than dressed up: this branch is owed an independent pass. What
partly offsets it is that every defect found on it was found by *running* the code, which this repo
holds to outperform reading.

## The invariant question first

**Nothing about scoring may move.** `internal/backtest/simulate.go` is the only production file
touched, and the change is additive: `HoldCaptaincy` gains `GW`, `XI`, `Bench`, `Captain`, `Vice`,
all populated from values the loop already computed, and nothing on the scoring path reads them.

Checked: the accuracy snapshot regenerated at `832c42b` differs from `5a683b0` in **the commit stamp
and nothing else** — two differing lines in `figures.csv`, both the stamp. `go build`, `go vet`,
`go test ./...` clean.

## Three defects found, all by running

**1. A process-global leaked between cells and changed every number after it.** `TeamFormWeight` is
a package variable, not a `SimConfig` field. The probe restored it with a `defer`, which fires once
at the end of the test — so the two TEAMFORM cells left it at 0.50 and **both `BlendRateK` cells ran
with club form switched on**, a configuration neither arm ever swept. Output was entirely plausible:
sensible squads, sensible captains, a total in the right range. Nothing would have flagged it.

⚠️ The file already carried a comment warning about exactly this, written for the *within-cell* case.
The bug was one level up, between cells. Fixed by resetting to the shipped value before each side of
each cell, so an arm that does not mention a global gets the shipped one.

**2. The baseline was being reconstructed instead of copied.** A sweep's baseline is its own
`variants[0]`. TEAMFORM's arms *both* set `WeeklyXI = true` and differ only in the club-form weight,
so building the baseline as a bare `sweepConfig` would have compared the weight and the
imminent-fixture eleven at once — "folding two levers into one arm measures their sum and neither".
Both sides are now copied from the sweep's own arm list.

**3. An `awk` column check was wrong on a quoted CSV.** `"club form, weight 0.50"` contains a comma,
so naive splitting shifted every column after `variant`. Caught because the season column printed
`true`/`false`. The R screens use `read.csv` and were never affected, but the lesson is that ad-hoc
`awk` over these files is unsafe wherever a label may contain a comma.

## The guard that catches this class

Each forensic cell now declares the paired difference it must reproduce, and the probe **fails** if
it does not. That is what surfaced defect 1 — without it the leaked configuration produced numbers
that looked fine. A forensic probe that cannot reproduce the figure it is explaining is explaining a
different figure.

⚠️ **And the guard immediately found something it was not looking for.** TEAMFORM does not reproduce
its bank: `+10.231 / +7.333` committed against `+7.154 / +7.111` here. That is a **data-state**
difference — its cells are from `2026-08-12-4d61058`, before the archive repairs — and not a leak.
The discriminator is that the `BlendRateK` cells, banked on the current tree, reproduce to three
decimals. Recorded in the struct rather than reconciled away.

## Judgement calls a reviewer should second-guess

- **The screen's thresholds are arbitrary** (`MIN_SEASON` 8, `SURVIVE` 0.35, `POS_NEAR` 0.60) and
  flag **21 of 36** arms. That rate is either the right answer — the bank really is lumpy, which is
  consistent with almost nothing in it resolving — or the thresholds are too loose. It prints counts
  rather than verdicts for that reason, but a reviewer may want them tightened or justified.
- **The framing was corrected mid-flight and the correction is load-bearing.** The first version
  read a concentrated mean as evidence of an artefact. It is not: a setting whose mechanism only
  fires sometimes ought to be lumpy. The screen now says a flag means "this mean is a few events,
  find the right denominator". ⚠️ If a later reader restores the stronger reading, the screen becomes
  a machine for retracting real intermittent effects.
- **`seas` inverts the naive reading** and that deserves scrutiny: carriers spread over three seasons
  are *harder* to dismiss than two in one season, even though the three-season case is the one that
  defeats leave-one-season-out. That is an argument, not a measurement.
- **Four cells is a small sample of the 21 flagged.** They were chosen to contrast one nested case
  against one spread case, before their contents were read, but they are not representative.

## Gates

`go build`, `go vet`, `go test ./...` clean; `gofmt` clean; accuracy snapshot regenerated and moves
only its stamp. ⚠️ **CLAUDE.md is at exactly 69,632 of 69,632 bytes — zero headroom**, paid for here
by compressing the BPS bullet, whose cut figures were verified present in `scoring-model.md` first.
