# The carrier mechanism and the finishing-skill measurement

Covers `1b81a2f`-onward on this branch: the `BlendRateK` carrier write-up in
`docs/notes/constants-and-sweeps.md`, and `8d0c311` (the finishing-skill measurement,
`stats/finishing_persistence.py`, `stats/finishing_career.py`, and the `scoring-model.md` section).

⚠️ **First-party review, no reviewer agents dispatched**, per the session's standing instruction.
⚠️ **And this is the record where that matters most so far**: both commits are *claims about the
model*, which is the category this project's practice sends to the statistics reviewer **before**
the work, not after. Neither claim changes shipped behaviour, which is the only reason this is
recoverable — but the ordering was wrong and is flagged rather than smoothed over.

## No code changed

Both commits are documentation plus two standalone analysis scripts under `stats/`. Nothing under
`internal/` moved, no test changed, and no shipped constant is touched. `go build`, `go vet`,
`go test ./...` clean.

## What the claims rest on, and where they could be wrong

**The carrier mechanism (2021-22@26).** Read directly off `TestDiagCellForensics`, which carries its
own reproduction check, plus the Understat cache for the xG. The load-bearing numbers:

- Kane played **90 minutes in every gameweek he featured from GW3 to GW25 bar one 63'** — so "was he
  injured" is answered by minutes, not inference.
- Kane **7 goals from 10.9 xG** (−3.9), chance volume down 19%. Salah **xG/90 up 0.59 → 0.78** with
  finishing on expectation.
- The four exclusive players cost **exactly £26.7m on each side**, so affordability is excluded as
  the explanation.

⚠️ **The conclusion drawn from this is the uncomfortable one and should be checked hardest**: the
k=16 arm bought the player the underlying says was worse, and won anyway — which *strengthens* the
argmax-over-cells reading rather than rescuing it. A reviewer who wanted to defend k=16 would have
to argue that post-AFCON minutes risk made Salah the worse hold despite the underlying, and the
minutes data is in the note for exactly that argument. I do not think it carries, because both arms
saw identical minutes (they differ only in `BlendRateK`, which touches rates, not minutes).

**The one-season prior.** `SimConfig.OlderPriors` is populated only by `cmd/priorblend`; no sweep
sets it, and `cmd/fplagent/sweep.go` says so in its own warning. Verified by grep over non-test Go.
This is the strongest claim in either commit because it is structural rather than statistical.

**Finishing persistence.** n = 3,764 consecutive-season pairs at ≥900 minutes in both.

⚠️ Weaknesses a reviewer should press:
- **No standard error or clustering.** The r values are raw Pearson correlations over player-seasons
  that are *not* independent — the same player contributes many pairs. The headline conclusion is
  about a **slope of 0.201 implying ~4 points a season**, which is an order of magnitude under
  threshold, so clustering cannot change the verdict; but the r values should not be quoted as if
  they carried a p.
- **The 900-minute cut is arbitrary** and selects toward regulars, which likely *understates* the
  sampling error and therefore *overstates* r.
- **Understat's xG is one provider's model.** The record already says mixing providers is a real
  cost for xA, and the replay's own xG for four seasons is a backfill from this same source.
- **The +3.6 figure assumes 30 full matches** and 4 points a goal, both round numbers chosen to be
  generous to the proposal rather than fitted.

## The verdict is "unmeasurable", and that word was chosen deliberately

Per this record's own distinction: the effect is bounded above **by its own construction** — slope
0.201 on a career estimate whose spread is 0.051 — rather than merely lost in the noise. That is a
stronger and more useful claim than "unresolved", and it is the reason the item is closed rather
than queued. ⚠️ If the slope were re-estimated with player-clustered errors and came out materially
larger, the verdict would need revisiting; nothing else would.

## Gates

`go build`, `go vet`, `go test ./...` clean. No snapshot needed — no watched source file moved.
