"""Task B: the four projection blocks that Run A could not reach, across three data states.

    python3 stats/snapshots/2026-08-13-4d61058/taskb_analysis.py

# What this answers, and the two traps it is built around

The question is not "what is each constant worth" -- that is recorded and mostly
unresolved. It is **whether the recorded verdict and the recorded ordering survive the
data state**, now that the two expected-goals backfills have changed what the replay is
scoring on.

**Trap one: counting one observation three times.** On the previous run 18 of the 24
cells were byte-identical across data states, so an ordering that "holds in all three
corners" holds in three copies of the same 18 cells plus six that move. This script
therefore reports the **responding-cell count** for every block, and re-runs the
ordering restricted to those cells alone. A stable ordering over non-responding cells
is arithmetic, not evidence.

**Trap two: a level quoted without its own noise.** Every ladder entry here carries a
standard error, because on the previous run every one of 28 entries had |t| < 2.2 and
25 of 28 below 1.6 -- levels moving by less than one standard error of themselves.

# Estimators

`se_naive` treats all 24 cells as independent, which is optimistic and correct only for
asking whether cells agree. `se_clustered` is the spread of the four season means over
root-S; this record has verified it equals clubSandwich's CR2 on these grids to four
decimals, and it is the conservative reading at three degrees of freedom (critical value
3.182). Both are printed because this project's rule is to report the range rather than
pick an end.

Figures are paired differences **per gameweek**; multiply by 38 for the season scale, which
is done in the `/season` column. Never divide a pooled total by the cell count.
"""

import csv
import glob
import os
import statistics as st

HERE = os.path.dirname(os.path.abspath(__file__))
CELLS = os.path.join(HERE, "cells")

# The three data states. `norepair` is FPL_NO_XG_REPAIR=1, which -- because
# applyXGCRepair sits inside applyXGRepair's early return -- turns off BOTH backfills.
STATES = ["shipped", "xgcoff", "norepair"]
BLOCKS = ["minw", "bonus", "bench", "mink"]
BLOCK_NAME = {
    "minw": "MINW  minutes convexity exponent (ships 1.25)",
    "bonus": "BONUS bonus prior/evidence schedule (ships 0.5/1.5)",
    "bench": "BENCH opening bench weight (ships 0.10)",
    "mink": "MINK  minutes prior strength (ships 5)",
}
T_CRIT_3DF = 3.182


def load(block, state):
    p = os.path.join(CELLS, f"runD-{block}-{state}.csv")
    rows = list(csv.DictReader(open(p)))
    for r in rows:
        r["_key"] = (r["season"], r["start_gw"])
        r["_hold"] = float(r["hold_per_gw"])
        r["_policy"] = float(r["policy_per_gw"])
    return rows


def baseline_of(rows):
    for r in rows:
        if r["is_baseline"] == "true":
            return r["variant"]
    raise SystemExit("no baseline arm")


def paired(rows, arm, base, metric):
    """Per-cell paired difference, arm minus baseline, keyed on (season, start)."""
    b = {r["_key"]: r[metric] for r in rows if r["variant"] == base}
    out = {}
    for r in rows:
        if r["variant"] == arm:
            out[r["_key"]] = r[metric] - b[r["_key"]]
    return out


def stats_of(diffs):
    """Mean, naive SE, season-clustered SE. Clustered = sd(season means)/sqrt(S)."""
    v = list(diffs.values())
    n = len(v)
    mean = st.mean(v)
    se_naive = st.stdev(v) / (n ** 0.5) if n > 1 and st.pstdev(v) > 0 else 0.0
    by_season = {}
    for (season, _), d in diffs.items():
        by_season.setdefault(season, []).append(d)
    means = [st.mean(x) for x in by_season.values()]
    S = len(means)
    se_cl = st.stdev(means) / (S ** 0.5) if S > 1 and st.pstdev(means) > 0 else 0.0
    return mean, se_naive, se_cl, S


def responding_cells(block):
    """Cells whose baseline arm differs across the three data states.

    Everything else is the same football scored the same way, so an arm-ordering
    computed over it is identical in all three states by construction.
    """
    base_by_state = {}
    for s in STATES:
        rows = load(block, s)
        base = baseline_of(rows)
        base_by_state[s] = {r["_key"]: r["_hold"] for r in rows if r["variant"] == base}
    keys = sorted(base_by_state[STATES[0]])
    resp = [k for k in keys
            if len({round(base_by_state[s][k], 10) for s in STATES}) > 1]
    return keys, resp


def main():
    print("Task B -- four projection blocks across three data states, four-season grid")
    print("Paired differences per gameweek on HOLD; /season is x38.\n")

    for block in BLOCKS:
        keys, resp = responding_cells(block)
        print("=" * 78)
        print(BLOCK_NAME[block])
        print(f"  responding cells: {len(resp)} of {len(keys)}"
              f"   ({', '.join(sorted({k[0] for k in resp})) or 'none'})")
        print("=" * 78)

        for state in STATES:
            rows = load(block, state)
            base = baseline_of(rows)
            arms = [a for a in dict.fromkeys(r["variant"] for r in rows) if a != base]
            print(f"\n  [{state}]  baseline: {base}")
            print(f"  {'arm':28} {'pts/gw':>8} {'/season':>8} "
                  f"{'se_naive':>9} {'se_clust':>9} {'t_clust':>8}")
            for a in arms:
                d = paired(rows, a, base, "_hold")
                m, sn, sc, S = stats_of(d)
                t = m / sc if sc else float("nan")
                print(f"  {a:28} {m:8.3f} {m*38:8.1f} {sn:9.3f} {sc:9.3f} {t:8.2f}")

        # Ordering, over all cells and over the responding ones only.
        print("\n  ordering of arms by mean HOLD difference (best first):")
        for label, subset in (("all cells", keys), ("responding only", resp)):
            if not subset:
                print(f"    {label:18} -- no cells")
                continue
            line = []
            for state in STATES:
                rows = load(block, state)
                base = baseline_of(rows)
                arms = [a for a in dict.fromkeys(r["variant"] for r in rows)
                        if a != base]
                scored = []
                for a in arms:
                    d = paired(rows, a, base, "_hold")
                    sub = {k: v for k, v in d.items() if k in set(subset)}
                    scored.append((st.mean(sub.values()) if sub else 0.0, a))
                scored.sort(reverse=True)
                line.append(f"{state}: " + " > ".join(a for _, a in scored))
            print(f"    {label}")
            for l in line:
                print(f"      {l}")
        print()


if __name__ == "__main__":
    main()
