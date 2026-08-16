"""How strong is the realised-points sanity check on an xPoints instrument?

The design is: tune on expected points from realised underlying, where variance
per event is the problem, and check the AGGREGATE against realised points, where
enough events have accumulated that the conversion residual should wash out.

⚠️ **The first version of this script concluded "the guard is sharper than the
thing it guards". That conclusion is WITHDRAWN.** It rested on an iid standard
error and an inflated N, and it compared an absolute-level SE against a
paired-difference threshold. Corrected below, the guard's power straddles the
threshold it was claimed to beat.
"""
import math

from xpoints_common import (CS, GOAL, NATIVE_XG_SEASONS, appearances,
                            clustered_se_of_mean, unscaled_residuals)

# Entry gameweeks 1/6/11/16/21/26 over six seasons.
WEEKS = [38, 33, 28, 23, 18, 13]
SEASONS = 6
T_CRIT = 2.571   # t_crit(5), the season-clustered df a six-season grid gives

if __name__ == '__main__':
    resid, keys, pts = [], [], []
    for et, v, key in appearances():
        a, c = unscaled_residuals(v, et)
        resid.append(a + c)
        keys.append(key)
        pts.append(v['points'])

    n = len(resid)
    mean_r = sum(resid) / n
    sd_r = (sum((x - mean_r) ** 2 for x in resid) / (n - 1)) ** 0.5
    mean_p = sum(pts) / n
    iid_se = sd_r / math.sqrt(n)
    cl_se, n_clusters = clustered_se_of_mean(resid, keys)

    print(f"player-gameweeks: {n}   mean realised points per appearance: {mean_p:.3f}")
    print(f"  mean conversion residual : {mean_r:+.4f}  "
          f"({100*mean_r/mean_p:+.2f}% of mean points)")
    print(f"  sd per appearance        : {sd_r:.3f}")
    print(f"  iid SE {iid_se:.5f}   club-gameweek clustered SE {cl_se:.5f} "
          f"over {n_clusters} clusters   -> design effect {(cl_se/iid_se)**2:.2f}x")
    print()

    gw_cells = SEASONS * sum(WEEKS)
    N = gw_cells * 11
    distinct = SEASONS * 38
    print(f"On the shipped grid: {gw_cells} gameweek-cells x11 = {N} player-gameweeks per arm.")
    print(f"⚠️ Those {gw_cells} gameweek-cells contain only {distinct} DISTINCT")
    print(f"   season-gameweeks — the six entry points replay the SAME football, so N is")
    print(f"   inflated {gw_cells/distinct:.2f}x before any squad overlap is counted.")
    print()

    # Scale the per-appearance clustered SE to the grid, then apply both inflations.
    base = cl_se * math.sqrt(n / N)
    for label, extra in (("club clustering only", 1.0),
                         ("+ entry-point nesting", math.sqrt(gw_cells / distinct))):
        se = base * extra
        det = T_CRIT * se
        print(f"  {label:<24} SE {se:.4f}  detectable bias {det:.4f} pts/appearance"
              f"  = {100*det/mean_p:4.2f}%  = {det*11*38:5.1f} pts a season")
    print()
    print("⚠️ VERDICT: the guard is NOT sharper than the thing it guards.")
    print("Constants here are worth 11-34 a season against a HOLD threshold of 33")
    print("(the pooled four-season median of 39 is the wrong baseline for a scoring")
    print("metric). The guard's power straddles that. It is worth having as a")
    print("not-vacuous aggregate check; it cannot be quoted as bounding circularity.")
    print()
    print("⚠️ And a bias CONSTANT across arms cancels in a paired difference, which is")
    print("the comparison that matters. What this guard can actually catch is a bias")
    print("that differs BETWEEN arms — one arm selecting a population whose xG converts")
    print("differently — which is not the quantity measured above.")
