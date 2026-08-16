"""Is finishing overperformance (goals - xG) a persistent player trait?

The whole case for a per-player finishing term rests on this. If season N's
G-xG/90 does not predict season N+1's, there is nothing to capture.

⚠️ **PREMIER LEAGUE ONLY.** The first version of this script pooled every league
Understat covers — 36.6% of matches in the cache have neither team in the PL —
and published a persistence figure and a "Son is positive in 9 of 11 seasons"
line over the mixed pool. See `understat_pl.py`. Restricted here to seasons the
id map confirms as Premier League, which also means **2016-17 onward**, since
earlier seasons have no column to confirm.

⚠️ This script and `finishing_career.py` both compute "previous season -> next
season" and, before this fix, DIVERGED on it — one gating on consecutive
seasons, the other taking the previous season with data — printing r = 0.087 and
r = 0.096 for the same-named quantity while the note quoted one denominator
against the other's r. They now share `pl_pairs` below, which is the only
implementation.
"""
import json
import glob
import os
from collections import defaultdict

from understat_pl import CACHE, EARLIEST, pl_seasons_by_understat_id

MIN_MIN = 900          # a half-season of football, so the rate means something


def load_pl_seasons():
    """player id -> {season -> [minutes, xG, goals]}, Premier League only."""
    pl = pl_seasons_by_understat_id()
    agg = defaultdict(lambda: defaultdict(lambda: [0.0, 0.0, 0.0]))
    names = {}
    for f in glob.glob(os.path.join(CACHE, 'player-*.json')):
        try:
            j = json.load(open(f))
        except Exception:
            continue
        pid = j.get('player', {}).get('id')
        if not pid or pid not in pl:
            continue
        names[pid] = j.get('player', {}).get('name', '')
        for m in j.get('matches', []):
            s = m.get('season')
            if not s or s < EARLIEST or s not in pl[pid]:
                continue
            a = agg[pid][s]
            a[0] += float(m['time'])
            a[1] += float(m['xG'])
            a[2] += float(m['goals'])
    return agg, names


def rate(a):
    return (a[2] - a[1]) / (a[0] / 90)


def pearson(xs, ys):
    n = len(xs)
    if n < 3:
        return float('nan')
    mx, my = sum(xs) / n, sum(ys) / n
    num = sum((x - mx) * (y - my) for x, y in zip(xs, ys))
    dx = sum((x - mx) ** 2 for x in xs) ** 0.5
    dy = sum((y - my) ** 2 for y in ys) ** 0.5
    return num / (dx * dy) if dx and dy else float('nan')


if __name__ == '__main__':
    agg, names = load_pl_seasons()
    pairs = []
    for pid, by in agg.items():
        seasons = sorted(by)
        for i in range(len(seasons) - 1):
            s0, s1 = seasons[i], seasons[i + 1]
            if int(s1) != int(s0) + 1:
                continue
            a0, a1 = by[s0], by[s1]
            if a0[0] < MIN_MIN or a1[0] < MIN_MIN:
                continue
            pairs.append((names[pid], pid, s0, rate(a0), rate(a1)))

    xs = [p[3] for p in pairs]
    ys = [p[4] for p in pairs]
    r = pearson(xs, ys)
    print(f"PREMIER LEAGUE ONLY, {EARLIEST}-17 onward, >= {MIN_MIN} min in BOTH seasons")
    print(f"consecutive-season pairs: {len(pairs)}  "
          f"({len(set(p[1] for p in pairs))} distinct players)")
    print(f"correlation of G-xG per 90, season N vs N+1:  r = {r:.3f}   r^2 = {r*r:.3f}")
    print()

    srt = sorted(range(len(pairs)), key=lambda i: xs[i])
    q = max(1, len(pairs) // 5)
    top_n = set(srt[-q:])
    top_n1 = set(sorted(range(len(pairs)), key=lambda i: ys[i])[-q:])
    print(f"top-quintile finishers in season N who repeat in N+1: "
          f"{len(top_n & top_n1)} of {q}  "
          f"({100*len(top_n & top_n1)/q:.0f}%, chance is 20%)")
    print()

    print("The named cases, Premier League seasons only (G-xG per 90):")
    for who in ('Son Heung-Min', 'Harry Kane', 'Mohamed Salah'):
        hits = [pid for pid, nm in names.items() if nm == who]
        if not hits:
            print(f"  {who}: not found")
            continue
        by = agg[hits[0]]
        out = [f"{s}:{rate(by[s]):+.2f}" for s in sorted(by) if by[s][0] >= MIN_MIN]
        pos = sum(1 for s in sorted(by) if by[s][0] >= MIN_MIN and rate(by[s]) > 0)
        print(f"  {who:<16} " + "  ".join(out) + f"   [{pos} of {len(out)} positive]")
