"""Does a CAREER finishing estimate predict next season better than one season?

A single season is a near-useless estimator of the trait. That is not the same as
"there is no trait": Son is positive in 8 of 9 Premier League seasons. The
question a feature would rest on is whether pooling a career recovers enough
signal to be worth having.

⚠️ **PREMIER LEAGUE ONLY, and the loader is shared with
`finishing_persistence.py`.** Both scripts previously carried their own copy of
"previous season -> next season" and had already diverged — one gated on
consecutive seasons, the other took the previous season *with data*, and they
printed r = 0.087 and r = 0.096 for the same-named quantity while the write-up
quoted one script's denominator against the other's r. That is this project's
signature failure and it was live in a published table. One loader now, in
`finishing_persistence.py`, and the consecutive-season rule below is stated
rather than implied.
"""
import math

from finishing_persistence import MIN_MIN, load_pl_seasons, pearson, rate
from understat_pl import EARLIEST


def clustered_slope(xs, ys, groups):
    """OLS slope with cluster-robust (CR0) SE, clustered on player.

    The same player contributes several observations, so an iid SE is wrong.
    Reported because the size verdict rests on the slope, not on r.
    """
    n = len(xs)
    mx, my = sum(xs) / n, sum(ys) / n
    sxx = sum((x - mx) ** 2 for x in xs)
    b = sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / sxx
    a = my - b * mx
    byg = {}
    for x, y, g in zip(xs, ys, groups):
        byg.setdefault(g, []).append((x - mx) * (y - a - b * x))
    meat = sum(sum(v) ** 2 for v in byg.values())
    se = math.sqrt(meat) / sxx
    return b, se, len(byg)


if __name__ == '__main__':
    agg, names = load_pl_seasons()
    prior1, priorC, nxt, who = [], [], [], []
    for pid, by in agg.items():
        seasons = sorted(by)
        for i, s1 in enumerate(seasons):
            if i == 0:
                continue
            # Consecutive only, matching finishing_persistence.py. The previous
            # version took seasons[i-1] whatever its year, so 4.3% of its
            # observations had a gap and it silently answered a different question.
            if int(seasons[i - 1]) != int(s1) - 1:
                continue
            a1, prev = by[s1], by[seasons[i - 1]]
            if a1[0] < MIN_MIN or prev[0] < MIN_MIN:
                continue
            past = seasons[:i]
            pm = sum(by[s][0] for s in past)
            if pm < MIN_MIN:
                continue
            pxg = sum(by[s][1] for s in past)
            pg = sum(by[s][2] for s in past)
            prior1.append(rate(prev))
            priorC.append((pg - pxg) / (pm / 90))
            nxt.append(rate(a1))
            who.append(pid)

    print(f"PREMIER LEAGUE ONLY, {EARLIEST}-17 onward. Observations: {len(nxt)} "
          f"({len(set(who))} distinct players)")
    print()
    print(f"  previous SEASON  -> next season   r = {pearson(prior1, nxt):.3f}")
    print(f"  CAREER to date   -> next season   r = {pearson(priorC, nxt):.3f}")
    print()

    b, se, g = clustered_slope(priorC, nxt, who)
    print(f"  regression slope of next season on the career estimate: {b:.3f}")
    print(f"  player-clustered SE {se:.3f} over {g} clusters -> t {b/se:+.2f}")
    print(f"  95% CI on the slope: {b-1.96*se:.3f} to {b+1.96*se:.3f}")
    print()
    print("Practical size, for a +0.15/90 career overperformer (above Son's own level):")
    for label, slope in (("point estimate", b), ("CI low", b - 1.96 * se),
                         ("CI high", b + 1.96 * se)):
        g90 = slope * 0.15
        print(f"  {label:<15} {g90:+.3f} goals/90 -> {g90*30:+.2f} goals over 30 x 90'"
              f" -> about {g90*30*4:+.1f} FPL points a season")
    print()
    print("⚠️ That bounds the PREDICTED-RETURN channel, not the SELECTION channel.")
    print("The replay scores what an argmax picks, and a small ordering shift between")
    print("two similarly-rated forwards flips a marginal pick whose realised gap is not")
    print("bounded by this figure. It also prices no bonus and no assists.")
