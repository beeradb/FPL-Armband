"""Does the held-out season's verdict move under the data repairs?

The record's held-out season for several constant decisions is 2022-23. Run A found
the backfills move 6 of 24 four-season cells -- ALL of them 2022-23. So the held-out
season is precisely and only the season whose data changed.

Task B ran BENCH (the opening bench weight) across three data states. That is exactly
the constant whose verdict rests on the held-out season disagreeing, so its 2022-23
column can be read directly.
"""
import csv, os, statistics as st

# Task B's cells are in the SIBLING snapshot directory, so locate them from this file
# rather than from a checkout path. The absolute path that used to be here named a host
# account and a worktree that no longer exists, so it published a private path AND
# could not run from any other clone.
D = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                 os.pardir, "2026-08-13-4d61058", "cells")

for blk in ("bench", "minw", "bonus", "mink"):
    print(f"\n=== {blk.upper()} — 2022-23 only, paired vs the shipped arm, HOLD pts/gw")
    per_state = {}
    for state in ("shipped", "xgcoff", "norepair"):
        p = os.path.join(D, f"runD-{blk}-{state}.csv")
        if not os.path.exists(p):
            continue
        rows = list(csv.DictReader(open(p)))
        base = next(r["variant"] for r in rows if r["is_baseline"] == "true")
        b = {(r["season"], r["start_gw"]): float(r["hold_per_gw"])
             for r in rows if r["variant"] == base}
        for a in dict.fromkeys(r["variant"] for r in rows):
            if a == base:
                continue
            d = [float(r["hold_per_gw"]) - b[(r["season"], r["start_gw"])]
                 for r in rows
                 if r["variant"] == a and r["season"] == "2022-23"]
            if d:
                per_state.setdefault(a, {})[state] = st.mean(d)
    if not per_state:
        continue
    print(f"  {'arm':26} {'shipped':>9} {'xgcoff':>9} {'norepair':>9} {'swing':>8}")
    for a, m in per_state.items():
        vals = [m.get(s) for s in ("shipped", "xgcoff", "norepair")]
        sw = max(v for v in vals if v is not None) - min(v for v in vals if v is not None)
        print(f"  {a[:26]:26} " + " ".join(
            f"{v:+9.3f}" if v is not None else "        -" for v in vals) +
            f" {sw:8.3f}")
    # does the winner change?
    winners = {}
    for s in ("shipped", "xgcoff", "norepair"):
        cand = {a: m[s] for a, m in per_state.items() if s in m}
        if cand:
            winners[s] = max(cand, key=cand.get)
    print(f"  best arm on 2022-23 by state: " +
          ", ".join(f"{s}={w}" for s, w in winners.items()))
    print("  -> " + ("VERDICT MOVES" if len(set(winners.values())) > 1
                     else "verdict stable"))
