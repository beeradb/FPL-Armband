# Result: re-pricing the two oracles on harvested starts

**Run 2026-08-13**, two arms of `TestDiagMinutesOracle` on the shipped six-season grid differing
only in `FPL_NO_STARTS_REPAIR`. 5 arms × 36 cells each, `exit=0`, 13m39s per run, peak RSS 130 MB.
Cells in `stats/snapshots/2026-08-13-oracle-starts/`. Pre-registration in
`oracle-reprice-prereg.md`, committed at `4936e5b` **before** the run.

## The three predictions

| # | prediction | outcome |
|---|---|---|
| 1 | the baseline arm must not move in any of 36 cells | **PASS** — 0/36 |
| 2 | movement confined to live cells; the 12 replaying 2024-25 and 2025-26 cannot move | **PASS** — 0 dead cells moved in any arm |
| 3 | `recon` falls to ≈0 with the harvest on and is high with it off | **PASS** — 0.0% in all six seasons on; **31.9% / 29.7% / 10.7%** for 2020-21 / 2021-22 / 2022-23 off |

No direction was predicted for the prices, deliberately.

## Only one of the two oracles was ever exposed

| arm | cells moved |
|---|---|
| `real (ships)` | **0/36** |
| `availability` | **0/36** |
| **`lineups`** | **18/36** — 2020-21, 2021-22, 2022-23, all six start points |
| `minutes` | **0/36** |
| `minutes`, season-average window | **0/36** |

**`OracleMinutes` does not move at all, so the recorded ≈47 stands unchanged**, and the mechanism is
the same weighted-zero fact that makes the shipped path immune: the minutes oracle accumulates
`sel.starts` and writes it out as `StartShare` (`minutesoracle.go:395`), which `reliabilityFrom`
multiplies by zero and `appearanceOdds` does not read under unified appearance. It writes a field
nothing consumes.

`OracleLineups` moves because it uses `Starts` differently — `lineupsoracle.go:221` takes
`g.Starts > 0` to **classify** a club-gameweek as a start, then prices that state in **minutes**,
which everything reads. Classification, not a start share.

Note the 18 live cells are the three seasons whose **own** starts the harvest reaches. **2023-24 did
not move**, so the prior-season channel is inert — consistent with prior `StartShare` also being
weighted zero. The pre-registration allowed up to 24 live cells and 18 responded; that is consistent
with prediction 2 rather than a sharpening of it, and it is the one place the pre-registration was
not tight.

## What the lineups bound is now worth

| | over all 36 cells | on the 18 live cells |
|---|---|---|
| harvest **off** (rank-reconstructed starts) | **72.6** a season | — |
| harvest **on** (recorded starts, ships) | **92.9** a season | — |
| paired difference | **+20.3** a season | **+40.6** a season |

**The recorded ≈73 reproduces at 72.6 with the harvest off.** That is the useful confirmation: the
figure was indeed measured against rank-reconstructed ground truth, exactly as the independent audit
inferred, and it is not stale for any other reason.

⚠️ **The difference does not resolve.** On the 18 live cells it is +1.0682 pts/gw, naive SE 0.7593
(t +1.41, df 17), season-clustered SE 0.7244 (**t +1.47 on df 2**, against a critical value of
4.30). Only three seasons carry any signal, and they disagree sharply — **2020-21 −6.5, 2021-22
+88.9, 2022-23 +39.4** a season, 12 of 18 cells positive and 5 negative. One season is most of it.

**The two figures are one effect on two denominators** — 40.6 × 18/36 = 20.3 — and the 18 dead cells
are excluded from the paired statistics rather than pooled as ties, per the standing rule that a
cell where the intervention could not run is not a tie.

## Verdict

**`suggestive`, and the direction is mechanically sensible.** Perfect selection is worth *more* when
the oracle is told the truth about who was selected: with reconstructed starts it was handed a
ground truth wrong in 2.36% of starter slots, biased 3:1 toward flattering the least certain player,
so it was a degraded oracle. Removing that raises the bound.

**What should be recorded is the level, not the difference.** `OracleLineups` ≈73 was measured on a
data state that no longer ships; on recorded starts the same bound reads **≈93**. The *increase* is
not established, so quote the new level with its data state and do not quote +20 or +41 as a
measured effect.

**`OracleMinutes` ≈47 needs no change and no caveat** — it was never exposed.
