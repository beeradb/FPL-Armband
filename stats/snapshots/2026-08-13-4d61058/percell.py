#!/usr/bin/env python3
"""Print a paired cells file cell by cell, and its season means.

Plumbing beside `pair_cells.py`: it prints what the arms did in each cell so a
reader can see the shape a mean is hiding. It computes **no** standard error, no
t and no verdict word — those are `sweep_inference.R`'s and this repository does
not keep two implementations of them.

The per-gameweek convention is enforced here rather than assumed: every
difference is divided by that cell's own `weeks`, because the six entry points
bank 38/33/28/23/18/13 gameweeks and a raw total weights the GW1 regime nearly
three times as heavily as the GW26 one. A season figure is per-gameweek times
38, never a pooled total divided by the cell count.

    percell.py paired.csv [--metric hold|policy]
"""

import argparse
import collections
import csv


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("path")
    args = ap.parse_args()

    rows = list(csv.DictReader(open(args.path)))
    base = {(r["season"], r["start_gw"]): r for r in rows if r["is_baseline"] == "true"}
    arm = {(r["season"], r["start_gw"]): r for r in rows if r["is_baseline"] == "false"}
    if set(base) != set(arm) or not base:
        raise SystemExit("the file does not hold one baseline and one arm per cell")

    print("season   start weeks |  HOLD off     on      d   per gw |"
          "  POL off     on      d   per gw | moves  hits")
    bys = collections.defaultdict(list)
    for k in sorted(base, key=lambda x: (x[0], int(x[1]))):
        b, a = base[k], arm[k]
        w = int(b["weeks"])
        dh = int(a["hold_points"]) - int(b["hold_points"])
        dp = int(a["policy_points"]) - int(b["policy_points"])
        print("%-8s %4s %5d | %7d %6d %+6d %+8.3f | %7d %6d %+6d %+8.3f | %3s/%-3s %2s/%-2s"
              % (k[0], k[1], w, int(b["hold_points"]), int(a["hold_points"]), dh, dh / w,
                 int(b["policy_points"]), int(a["policy_points"]), dp, dp / w,
                 b["moves"], a["moves"], b["hits"], a["hits"]))
        bys[k[0]].append((dh / w, dp / w, dh, dp))

    print("\nseason means, per gameweek played (x38 for a season):")
    for s in sorted(bys):
        v = bys[s]
        h = sum(x[0] for x in v) / len(v)
        p = sum(x[1] for x in v) / len(v)
        nh = sum(1 for x in v if x[2] != 0)
        np_ = sum(1 for x in v if x[3] != 0)
        print("  %-8s HOLD %+7.4f (%+6.1f/season, %d/%d cells moved)   "
              "POLICY %+7.4f (%+6.1f/season, %d/%d moved)"
              % (s, h, h * 38, nh, len(v), p, p * 38, np_, len(v)))

    allc = [x for v in bys.values() for x in v]
    h = sum(x[0] for x in allc) / len(allc)
    p = sum(x[1] for x in allc) / len(allc)
    print("\n  pooled over %d cells: HOLD %+7.4f (%+6.1f/season)   POLICY %+7.4f (%+6.1f/season)"
          % (len(allc), h, h * 38, p, p * 38))
    print("  cells with HOLD positive / negative / zero: %d / %d / %d"
          % (sum(1 for x in allc if x[2] > 0), sum(1 for x in allc if x[2] < 0),
             sum(1 for x in allc if x[2] == 0)))
    print("  cells with POLICY positive / negative / zero: %d / %d / %d"
          % (sum(1 for x in allc if x[3] > 0), sum(1 for x in allc if x[3] < 0),
             sum(1 for x in allc if x[3] == 0)))


if __name__ == "__main__":
    main()
