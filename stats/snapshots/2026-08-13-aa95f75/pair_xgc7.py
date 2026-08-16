"""The fourth cluster for the expected-goals-conceded repair.

    python3 stats/snapshots/2026-08-13-aa95f75/pair_xgc7.py

The six-season grid reaches three played seasons with no native xGC column, which is
df 2 and a critical value of 4.303 — the reason the record called the repair's price
unmeasurable. `scoringPairNames` is a seven-season `HOLD`-only grid that also plays
**2019-20**, which has no native column either and which the repair does reach. That is
a fourth cluster: df 3, critical value 3.182.

`HOLD`-only does not bite, because `HOLD` is the metric that decides a scoring question.
`POLICY` is not reported for this grid at all — it plays 2019-20, whose transfer path is
not a sample of the same process (FPL granted unlimited free transfers before the GW30+
deadline and froze prices for three months).

**Pool only over the cells the repair can reach.** The other three played seasons carry a
native column and are structurally inert; including them would halve the estimate and
report the dilution as a smaller effect.
"""

import csv
import os
import statistics as st

HERE = os.path.dirname(os.path.abspath(__file__))
CELLS = os.path.join(HERE, "cells")
T_CRIT = {1: 12.706, 2: 4.303, 3: 3.182, 4: 2.776, 5: 2.571}


def load(name):
    with open(os.path.join(CELLS, name), newline="") as fh:
        return {(r["season"], r["start_gw"]): r for r in csv.DictReader(fh)}


def main():
    on, off = load("xgc7-on.csv"), load("xgc7-off.csv")
    keys = sorted(set(on) & set(off))

    diffs = {k: float(on[k]["hold_per_gw"]) - float(off[k]["hold_per_gw"]) for k in keys}
    live = {k: v for k, v in diffs.items() if abs(v) > 1e-12}
    seasons = sorted({k[0] for k in keys})
    live_seasons = sorted({k[0] for k in live})

    print(f"cells paired: {len(keys)} over {len(seasons)} seasons")
    print(f"cells that move: {len(live)} over {len(live_seasons)} seasons "
          f"({', '.join(live_seasons)})")
    print(f"structurally inert: {len(keys) - len(live)}\n")

    # Pool over the reachable seasons only.
    pool = {k: v for k, v in diffs.items() if k[0] in live_seasons}
    by_season = {}
    for (s, _), d in pool.items():
        by_season.setdefault(s, []).append(d)

    means = {s: st.mean(v) for s, v in sorted(by_season.items())}
    print("per season, HOLD pts/gw (x38 for the season scale):")
    for s, m in means.items():
        print(f"  {s}  {m:+.4f}   ({m*38:+.1f} a season, {len(by_season[s])} cells)")

    vals = list(means.values())
    S = len(vals)
    mean = st.mean(vals)
    se = st.stdev(vals) / (S ** 0.5) if S > 1 else float("nan")
    df = S - 1
    t = mean / se if se else float("nan")
    crit = T_CRIT.get(df, 2.0)

    print(f"\npooled over {S} clusters (df {df}, critical value {crit}):")
    print(f"  mean      {mean:+.4f} pts/gw   ({mean*38:+.1f} a season)")
    print(f"  clustered SE {se:.4f}          threshold {crit*se*38:.0f} a season")
    print(f"  t         {t:+.2f}   -> {'RESOLVES' if abs(t) > crit else 'does not resolve'}")
    print(f"\n  seasons agreeing in sign with the pooled mean: "
          f"{sum(1 for v in vals if (v > 0) == (mean > 0))} of {S}")


if __name__ == "__main__":
    main()
