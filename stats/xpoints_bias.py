"""Is the xPoints residual unbiased, and where does the bias live?

An aggregate realised-points guard is only meaningful if xPoints measures the
same thing as points. It does not: the net is two large opposing biases that
partly cancel, which is worse than one small one, because they cancel at
whatever attack/defence ratio the squad happens to hold.

⚠️ Clustered on club-gameweek. A club's clean sheet is ONE event shared by every
defender owned, so an iid t on that channel is inflated by roughly the number of
qualifying defenders per club-match.
"""
import math

from xpoints_common import (CS, CS_MINUTES, NATIVE_XG_SEASONS, appearances,
                            clustered_se_of_mean, expected_clean_sheets,
                            unscaled_residuals)


def report(vals, keys, label):
    n = len(vals)
    m = sum(vals) / n
    sd = (sum((x - m) ** 2 for x in vals) / (n - 1)) ** 0.5
    iid = sd / math.sqrt(n)
    cl, g = clustered_se_of_mean(vals, keys)
    print(f"  {label:<30} n {n:6d}  mean {m:+7.4f}")
    print(f"  {'':<30} iid SE {iid:.4f} (t {m/iid:+6.2f})   "
          f"clustered SE {cl:.4f} (t {m/cl:+6.2f}) over {g} club-gameweeks")
    return m


if __name__ == '__main__':
    att, att_k, cs, cs_k = [], [], [], []
    real = xp = 0.0
    for et, v, key in appearances():
        # (!) Both channels used to be recomputed here, inline, which contradicted
        # xpoints_common's own docstring -- "One implementation, because the three
        # scripts previously carried their own copy of the residual" -- in one of the
        # three scripts that sentence is about. The condition below still decides
        # MEMBERSHIP of the clean-sheet list, because a non-qualifying appearance must
        # not contribute a zero to a mean over qualifying ones; but the VALUES now
        # have one home.
        a, c = unscaled_residuals(v, et)
        att.append(a)
        att_k.append(key)
        if v['minutes'] >= CS_MINUTES and CS[et] > 0:
            cs.append(c)
            cs_k.append(key)
        if et in (1, 2) and v['minutes'] >= CS_MINUTES:
            real += v['clean_sheets']
            xp += expected_clean_sheets(v)

    print(f"Seasons {', '.join(NATIVE_XG_SEASONS)} — PER-FIXTURE clean sheet expectation.")
    print("Decomposing the aggregate residual (points per appearance):")
    report(att, att_k, "goals/assists conversion")
    report(cs, cs_k, "clean sheet vs exp(-xGC/f)")
    print()
    print("Clean sheets, the readable version (GK and DEF, 60+ minutes):")
    print(f"  {real:.0f} real, {xp:.0f} predicted by the per-fixture expectation")
    print(f"  over-predicts by {100*(xp-real)/real:+.1f}%")
    print()
    print("⚠️ This is the SAME estimator the record already applies per team-match,")
    print("on a different unit — NOT an independent method. Roughly five GK/DEF appear")
    print("per team-match, so the two aggregations are near-uniform reweightings of one")
    print("computation and were always going to agree.")
