"""Does widening the grid help or hurt POLICY? Eight more arms against the one on record.

    python3 stats/snapshots/2026-08-13-aa95f75/grid_policy.py

# The question

The six-season grid ships as the default. It was adopted on a `HOLD` result — the
positive control's threshold fell from 12.4 to 8.4 points a season — and on `POLICY`
the same control moved the *wrong* way, 14.4 to 16.1, because 2021-22 arrived returning
+1.16 pts/gw against a native range of 0.000 to 0.581 and nearly doubled the
between-season spread.

That is **one arm**, and the record says so: it prescribes `FPL_SWEEP_SEASONS=default`
for transfer settings "until more POLICY arms with real effects are run on both grids".
This is that comparison. `MINHL` and `FIXW` both carry real `POLICY` effects and both
had four-season cells already committed, so only the six-season half needed running.

# What is being compared, and what it is not

The **clustered standard error**, four seasons against six, per arm and per metric.
That is the quantity the grid choice is about: more clusters buy degrees of freedom
(critical value 3.182 → 2.571) and cost whatever variance the added seasons bring.

⚠️ This is **not** a comparison of effect sizes. The two grids play different football,
so an arm's mean is expected to move; what matters here is whether the *precision*
improves. And a ratio below 1 is not automatically a win — the critical value falls
too, so the honest summary is the implied threshold, `t_crit x SE x 38`.
"""

import csv
import os
import statistics as st

HERE = os.path.dirname(os.path.abspath(__file__))
R = os.path.dirname(os.path.dirname(os.path.dirname(HERE)))
T_CRIT = {3: 3.182, 5: 2.571}


def arm_stats(path, metric):
    with open(path, newline="") as fh:
        rows = list(csv.DictReader(fh))
    base = next((r["variant"] for r in rows if r["is_baseline"] == "true"), None)
    if base is None:
        return {}
    b = {(r["season"], r["start_gw"]): float(r[metric])
         for r in rows if r["variant"] == base}
    out = {}
    for a in dict.fromkeys(r["variant"] for r in rows):
        if a == base:
            continue
        by = {}
        for r in rows:
            if r["variant"] != a:
                continue
            k = (r["season"], r["start_gw"])
            by.setdefault(k[0], []).append(float(r[metric]) - b[k])
        means = [st.mean(v) for v in by.values()]
        if len(means) < 2:
            continue
        S = len(means)
        m = st.mean(means)
        se = st.stdev(means) / S ** 0.5
        out[a] = (m, se, S)
    return out


PAIRS = [
    ("MINHL", "stats/snapshots/2026-08-13-4d61058/cells/runA-minhl-shipped.csv",
     "stats/snapshots/2026-08-13-aa95f75/cells/6s-minhl.csv"),
    ("FIXW", "stats/snapshots/2026-08-13-4d61058/cells/runA-fixw-shipped.csv",
     "stats/snapshots/2026-08-13-aa95f75/cells/6s-fixw.csv"),
    # H is min_gain -- the transfer gate's minimum-gain threshold, and the only block
    # here that is a TRANSFER setting rather than a scoring constant. It is the
    # population the four-season exception was written for, so it carries more weight
    # on that question than the two above put together.
    ("H", "stats/snapshots/2026-08-13-aa95f75/cells/h-four.csv",
     "stats/snapshots/2026-08-13-aa95f75/cells/h-six.csv"),
]


def main():
    for metric in ("policy_per_gw", "hold_per_gw"):
        print(f"\n{'='*74}\n{metric}\n{'='*74}")
        print(f"{'block':7} {'arm':22} {'SE4':>7} {'SE6':>7} {'ratio':>6} "
              f"{'thr4':>6} {'thr6':>6}  verdict")
        ratios = []
        for blk, p4, p6 in PAIRS:
            f4, f6 = os.path.join(R, p4), os.path.join(R, p6)
            if not (os.path.exists(f4) and os.path.exists(f6)):
                print(f"{blk:7} -- cells not on disk, skipped")
                continue
            a4 = arm_stats(f4, metric)
            a6 = arm_stats(f6, metric)
            for a in a4:
                if a not in a6:
                    continue
                _, se4, S4 = a4[a]
                _, se6, S6 = a6[a]
                if se4 == 0 or se6 == 0:
                    continue
                t4, t6 = T_CRIT.get(S4 - 1, 2.6), T_CRIT.get(S6 - 1, 2.6)
                thr4, thr6 = t4 * se4 * 38, t6 * se6 * 38
                ratios.append(thr6 / thr4)
                v = "better" if thr6 < thr4 else "WORSE"
                print(f"{blk:7} {a[:22]:22} {se4:7.4f} {se6:7.4f} "
                      f"{se6/se4:6.2f} {thr4:6.0f} {thr6:6.0f}  {v}")
        if ratios:
            better = sum(1 for r in ratios if r < 1)
            print(f"\n  threshold ratio six/four: median {st.median(ratios):.2f}, "
                  f"range {min(ratios):.2f}-{max(ratios):.2f}")
            print(f"  arms where widening HELPS: {better} of {len(ratios)}")


if __name__ == "__main__":
    main()
