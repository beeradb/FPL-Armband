# Three dispatched reviews, and what they found

Covers `a70f7af`, which applies them. **This is the first real review record on the second half of
this branch.** Every earlier record on it was first-party, written because a session instruction was
read as forbidding subagents; that reading was wrong, the user overruled it, and the records it
produced are superseded by this one wherever they disagree.

Reviewers: `fpl-stats-review`, `fpl-code-review`, `fpl-findings-audit`, dispatched concurrently
against the branch with briefs naming where I thought my own work was weakest.

## What they found that I did not

**Two data-validity bugs, both invalidating published figures.**

1. **The finishing measurement pooled non-Premier-League football** (code review). 36.6% of matches
   in the Understat cache have neither team in the PL — `getPlayerData` returns every league — and
   the published "Son is positive in 9 of 11 seasons" counted a Bundesliga year while Salah's early
   positives were Serie A. ⚠️ **The repo already documents this trap**, in
   `understat_xg_backfill.py`'s own comment, and the new scripts walked into it anyway.
2. **`exp(−xGC)` was applied to a double gameweek's accumulated xGC** (code review). `season.go`
   sums the rows, so `exp(−(x₁+x₂))` stood in for `exp(−x₁)+exp(−x₂)`. 248 double rows carry 109
   real clean sheets against 37 predicted. ⚠️ **This is the recorded doubles class arriving through
   a non-linear function** — a shape the existing guard, which keys on `(element, fixture)`, does
   not cover.

**Three headline claims refuted** (statistics review):

- **"39 → about 31"** — 39 is the *pooled four-season* median where a scoring metric belongs against
  the `HOLD` median of 33; the 19.9% sd figure assumed additivity where the covariance is **+0.63**;
  and the transport was never derived, because **a shared player's residual cancels exactly** in a
  paired difference.
- **"The guard is sharper than the thing it guards"** — iid SE, retired `2 × SE`, and an N inflated
  **4.03×** because the grid's 918 gameweek-cells hold only 228 distinct season-gameweeks. Corrected:
  34.7 to 69.7 a season against a threshold of 33.
- **"Independently reproduces the recorded clean-sheet over-prediction"** — same estimator, same
  archive column, different aggregation, and with ~5 GK/DEF per team-match the two were always going
  to agree. ⚠️ I had called this the strongest result on the branch; it was the weakest-supported.

**And the framing of the whole concentration finding** (statistics review): leave-one-season-out was
never a concentration test — this record says so — so calling it a blind spot dressed a known
limitation as a discovery. The mechanism is heavy tails (pooled excess kurtosis **+3.46**), and a
wild cluster bootstrap puts the motivating contrast at **p = 0.0625**, which does not clear 0.05.
`seas=3` is the **modal outcome of noise** (P = 0.605), so the argument built on it inverted the one
direction the data cannot support.

## The finding I would have least expected

`concentration_screen.R` **could not compute the contrast in its own opening paragraph**. It looped
only over arms against the baseline, while `BlendRateK=16 minus BlendRateK=12` is a between-arm
contrast. It quoted "+23.3 a season, 17 of 36 cells, three cells carry 99%" and could produce none
of them.

That is precisely the defect this record holds against `schedule_screen.R` — "it cannot test its own
motivating example" — reproduced inside the file written to complain about it. Now fixed: it screens
every pairwise contrast, 85 of them, and reproduces its own example exactly.

## What survived

- **The `HoldCaptaincy` addition is clean** — code review grepped every consumer and confirmed
  nothing reads the new fields, and the slices are defensively copied. The snapshot agrees.
- **The forensic probe's baseline reconstruction is right**, verified arm-by-arm against
  `teamformsweep_test.go` and `transferpolicy_test.go`.
- **The three-season restriction in the xpoints scripts is correct**, verified against
  `xgrepair.go`'s season scoping.
- **The variance share (35.9%) and the two-opposing-biases decomposition** both stand, with clustered
  t values substituted for iid ones.
- **The prescription "run any realised-points guard per position group"** — statistics review calls
  it the most useful thing in the commit, and it is unchanged.
- **The BPS compression check I claimed to have done** was independently re-verified, along with
  **eight other figures** cut from CLAUDE.md today. All carried.

## Declined, with reasons

- **`fpl-stats-review` proposed adding a wild cluster bootstrap to `sweep_inference.R`.** Right, and
  not done here — it changes the inference layer every banked figure is read through, and belongs in
  its own change with its own review rather than bundled into a correction commit. Queued.
- **It also proposed replacing `seas` with a precision-weighted carrier ranking.** The column is kept
  because `seas=1` is genuinely informative (P ≈ 0.015) and because the length-bias warning now has
  to live somewhere. Reweighting is the better fix and is queued rather than improvised.
- **`fpl-findings-audit` proposed exact CLAUDE.md replacement text for several bullets.** Applied in
  substance, not verbatim — the byte budget had 4 bytes of headroom and its suggestions were net
  longer. Funded instead by cutting the retraction-history bullet it correctly identified as a
  never-resident class.

## Gates

`go build`, `go vet`, `go test ./...` clean. `TestDiagCellForensics` re-run and all four cells still
reproduce. `CLAUDE.md` 69,456 of 69,632. The three corrected scripts re-run and their figures are the
ones now in the notes.
