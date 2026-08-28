"""Does scoring on underlying remove ~36% of the variance of a TRANSFER, or of an APPEARANCE?

# The question, and why it is step zero

The proposal to score individual transfer decisions on realised underlying rests on
one number: that doing so removes about 36% of the variance of the thing being
scored. That number -- 25.6% attacking, 35.9% with the clean sheet -- is measured
**per player-gameweek** by xpoints_variance.py, and the note that produced it
retracted exactly this transport: "that is a per-appearance figure and it transports
to nothing else".

A transfer is not an appearance. It is a DIFFERENCE of TWO players SUMMED over a
window of w gameweeks. Both operations change the share, and they change it in
opposite directions:

  - Differencing two players ADDS two independent conversion residuals while the
    systematic quality gap -- the thing a transfer is actually trying to buy -- is a
    difference of means that can be small. That RAISES the residual share.
  - Summing over w weeks grows the systematic gap like w and the week-to-week noise
    like sqrt(w). That LOWERS the residual share, and it lowers it faster the longer
    the horizon.

Neither effect is estimable from the per-appearance figure, and they do not cancel
by any argument anyone has made. So the share is measured here directly, on the
same three native-xG seasons, before any Go is written.

# What is computed

For an ordered pair of players (in, out) and a window of w gameweeks starting at g:

    Net  = sum over the window of (points_in  - points_out)
    R    = sum over the window of (residual_in - residual_out)
    xNet = Net - R

R is the conversion residual of the DIFFERENCE: goals against xG and clean sheets
against the per-fixture exp(-xGC/f), priced at FPL's own per-position points. The
five channels the design leaves realised -- appearance, bonus, saves, cards, defcon
-- cancel out of R exactly, because they are identical in Net and xNet.

Reported: var(R)/var(Net), the share of per-move variance the substitution removes,
and 1 - sd(xNet)/sd(Net), the sd cut, which is what a detection threshold scales
with.

# The population matters, and it dominates everything else here

Real transfers are not random pairs. Populations reported, each adding one
restriction:

  any        any two players, any positions
  pos        same position
  posprice   same position, within 0.5m
  poolprice  ... and both clearing the shipped pool floor, EX ANTE
  bothplayed both players played every week of the window (calibration only)

⚠️ **`poolprice` is the operative row, and the first version of this header named
`posprice` and called it the UPPER end when it is the lower one.** The optimiser will
not buy a player below `MinExpectedMinutes`, so the whole field is not a transfer
population -- 60.1% of its rows are blank, against the **13%** of genuinely sold
players who stop playing, which the record measures on the sell side. Gating on the
same floor the code under test applies moves the five-week share from **10% to
~21.6%** and the sd cut from 2.1% to ~16%. That is not a refinement, it is most of
the answer.

⚠️ Still a bracket rather than a measurement. The gate is symmetric, and a real sale
often targets a player whose minutes are collapsing and who would fail it, so this
over-corrects the sell side. **The estimand is the replay's own move list**, which
`TestDiagTransferError` already judges 222-327 of; emitting `(in_id, out_id, gw,
season, fundingLeg)` from it and feeding those triples here would replace both ends
of the bracket with the thing itself.

⚠️ Reads the cache directly, so season.go's ungated defect repairs are not applied --
same caveat as every other script in this family, stated because the rule is to
state it.
"""
import collections
import json
import math
import random

import xpoints_common as xc

WINDOWS = [1, 3, 5, 8]
PAIRS_PER_CELL = 4000
SEED = 20260814          # fixed: this is a measurement, and it must reproduce
DIFF_DRAWS = 200000      # for the independent-difference reference
PRICE_BAND = 5           # tenths of a million, i.e. 0.5m
LAST_GW = 38             # windows must fit inside the season; see `measure`
RESEEDS = 6              # sampling-spread replicates; see `reseed`

# The shipped pool floor, applied EX ANTE. `MinExpectedMinutes` is 55 minutes a
# gameweek and the replay will not buy below it, so a stand-in population that
# ignores it is not a transfer population -- it is the whole field, 60.1% of whose
# rows are blank, against the 13% of REAL sold players who stop playing. Averaged
# over the prior POOL_LOOKBACK gameweeks and never over the window itself, so no
# hindsight enters; starts are pushed past the lookback for the same reason.
POOL_MIN_MINUTES = 55
POOL_LOOKBACK = 6
POPULATIONS = ('any', 'pos', 'posprice', 'poolprice', 'bothplayed')


def load():
    """Per season: {player_id: (element_type, {gw: (points, residual, value)})}."""
    out = {}
    for s in xc.NATIVE_XG_SEASONS:
        path = f'.cache/fpl/backtest-v9-{s}.json'
        try:
            d = json.load(open(path))
        except FileNotFoundError:
            raise SystemExit(
                f"missing {path} — run from the repo root. Refusing to print a "
                f"figure over fewer seasons than its header claims.")
        ps = d['players']
        players = {}
        for p in (list(ps.values()) if isinstance(ps, dict) else ps):
            et = p.get('element_type') or p.get('type')
            if et not in xc.GOAL:
                continue          # element_type 5 is managers
            weeks = {}
            for gw, v in (p.get('gws') or {}).items():
                played = v['minutes'] > 0
                att, cs = xc.unscaled_residuals(v, et) if played else (0.0, 0.0)
                # `played` is carried explicitly. A first version inferred it from
                # `residual == 0.0`, which is WRONG and published a wrong figure:
                # a player with 1-59 minutes, no goals, no assists and no xG has a
                # residual of exactly 0.0 while having played. That proxy read the
                # zero-minute share as 65.3% where the direct count is 60.1%.
                weeks[int(gw)] = (float(v['points']), att + cs,
                                  float(v.get('value') or 0), played,
                                  float(v['minutes']))
            if weeks:
                players[p['id']] = (et, weeks)
        out[s] = players
    return out


def reference_shares(data):
    """The two single-appearance figures every per-move number is read against.

    These were computed in throwaway shell commands when this measurement was
    first written, and three published figures therefore came from code that was
    never committed -- caught by code review. They are computed here so that every
    number this script's conclusions rest on is emitted by the script.

    Returns (appearance_share, diff_share, allrow_share, zero_minute_fraction).
    """
    pts, res = [], []
    all_pts, all_res, zero = [], [], 0
    for players in data.values():
        for et, weeks in players.values():
            for _, (p, r, _v, played, _m) in weeks.items():
                all_pts.append(p)
                all_res.append(r)
                if played:
                    pts.append(p)
                    res.append(r)
                else:
                    zero += 1
    va, _ = variance(pts)
    vr, _ = variance(res)
    vaa, _ = variance(all_pts)
    vra, _ = variance(all_res)

    # "Differencing is innocent": two INDEPENDENT appearances differenced preserve
    # the share exactly, because var of a difference of iid draws is 2*var on both
    # numerator and denominator. Computed rather than asserted, since it is the step
    # that licenses attributing the whole decay to the window and the blanks.
    rng = random.Random(SEED)
    n = len(pts)
    d, dr = [], []
    for _ in range(DIFF_DRAWS):
        i, j = rng.randrange(n), rng.randrange(n)
        d.append(pts[i] - pts[j])
        dr.append(res[i] - res[j])
    vd, _ = variance(d)
    vdr, _ = variance(dr)

    return (vr / va, vdr / vd, vra / vaa, zero / len(all_pts))


def in_pool(weeks, start):
    """Would the optimiser have this player in its pool at `start`?

    Mean minutes over the POOL_LOOKBACK gameweeks BEFORE `start`, against the
    shipped `MinExpectedMinutes`. Strictly ex ante -- it reads no gameweek inside
    the window being scored, so no hindsight enters the population definition.

    ⚠️ This is applied to BOTH sides. A real sale often targets a player whose
    minutes are collapsing, who would fail this gate, so a symmetric floor
    over-corrects on the sell side. It brackets rather than measures; the estimand
    is the replay's own move list, which this script cannot see.
    """
    tot = 0.0
    for g in range(start - POOL_LOOKBACK, start):
        e = weeks.get(g)
        if e:
            tot += e[4]
    return tot / POOL_LOOKBACK >= POOL_MIN_MINUTES


def window_sums(weeks, start, w):
    """(points, residual) summed over [start, start+w).

    A gameweek the player has no row for contributes zero, which is the right
    reading for a transfer: an unowned or unplayed week returns nothing.
    """
    pts = res = 0.0
    for g in range(start, start + w):
        e = weeks.get(g)
        if e:
            pts += e[0]
            res += e[1]
    return pts, res


def variance(xs):
    n = len(xs)
    m = sum(xs) / n
    return sum((x - m) ** 2 for x in xs) / (n - 1), m


def measure(players, w, pop, rng):
    """Sample ordered pairs and return (share, sd_cut, n, mean_net)."""
    # (!) Pairs are drawn WITHIN a season. The first version pooled all three
    # seasons into one id space keyed by (season, player_id), so `a != b` admitted
    # the same footballer in two different seasons and 2,642 of 4,000 pairs in the
    # operative cell were cross-season -- matched on gameweek NUMBER rather than on
    # football, with the price filter comparing two seasons' price scales. It moved
    # the headline little (11.1% to 10.4%) and it is a population nobody could
    # transfer within, which is the whole claim `posprice` makes for itself.
    by_season = collections.defaultdict(list)
    for (s, pid), v in players.items():
        by_season[s].append((pid, v))

    nets, xnets, rs = [], [], []
    tries = 0
    seasons = sorted(by_season)
    while len(nets) < PAIRS_PER_CELL and tries < PAIRS_PER_CELL * 60:
        tries += 1
        s = rng.choice(seasons)
        pool = by_season[s]
        if pop == 'any':
            ia, ib = rng.randrange(len(pool)), rng.randrange(len(pool))
        else:
            et = rng.choice([1, 2, 3, 4])
            same = [i for i, (_, (e, _)) in enumerate(pool) if e == et]
            if len(same) < 2:
                continue
            ia, ib = rng.choice(same), rng.choice(same)
        if ia == ib:
            continue
        wa, wb = pool[ia][1][1], pool[ib][1][1]
        common = set(wa) & set(wb)
        if not common:
            continue
        # Only start points where the FULL window fits inside the season, so the
        # w column is the effective window and not a nominal one. Sampling
        # uniformly over all shared gameweeks let ~7 of 38 starts at w=8 run off
        # the end, padding both players with guaranteed zeros and quietly mixing
        # shorter windows into the long rows -- which biases exactly the decay this
        # script reports.
        lo = POOL_LOOKBACK + 1 if pop == 'poolprice' else 1
        starts = [g for g in sorted(common) if g + w - 1 <= LAST_GW and g >= lo]
        if not starts:
            continue
        start = rng.choice(starts)
        if pop == 'poolprice':
            if not (in_pool(wa, start) and in_pool(wb, start)):
                continue
        if pop in ('posprice', 'poolprice'):
            va = wa[start][2]
            vb = wb[start][2]
            if va <= 0 or vb <= 0 or abs(va - vb) > PRICE_BAND:
                continue
        if pop == 'bothplayed':
            # Calibration population: both players PLAYED in every window week.
            #
            # (!) This gate first read `g not in wa`, i.e. "has a row", which is not
            # the same thing at all -- 60.1% of rows are zero-minute. The arm that
            # exists to check this pipeline against the recorded 35.9% therefore
            # read 22.1% and could not have flagged a broken pipeline. Gated on
            # minutes it reads ~34%, which is what a control is for. Found by code
            # review, and it is this record's own "a control must be able to fire".
            if any(g not in wa or g not in wb or
                   not wa[g][3] or not wb[g][3]
                   for g in range(start, start + w)):
                continue
        pa, ra = window_sums(wa, start, w)
        pb, rb = window_sums(wb, start, w)
        net = pa - pb
        r = ra - rb
        nets.append(net)
        rs.append(r)
        xnets.append(net - r)

    if len(nets) < 100:
        return None
    vn, mn = variance(nets)
    vr, _ = variance(rs)
    vx, _ = variance(xnets)
    if vn <= 0:
        return None
    return vr / vn, 1 - math.sqrt(vx / vn), len(nets), mn


def main():
    data = load()
    allp = {}
    for s, players in data.items():
        for pid, v in players.items():
            allp[(s, pid)] = v

    print(__doc__.split('\n')[0])
    print()
    print(f"Three native-xG seasons: {', '.join(xc.NATIVE_XG_SEASONS)}. "
          f"{len(allp)} player-seasons. {PAIRS_PER_CELL} sampled pairs per cell, "
          f"seed {SEED}. Pairs are drawn WITHIN a season.")
    print()

    app, diff, allrow, zerofrac = reference_shares(data)
    print("Reference — the single-appearance figures every row below is read against.")
    print(f"  appearances only (minutes > 0)          {app:>7.1%}   "
          f"<- the recorded 35.9%")
    print(f"  two INDEPENDENT appearances differenced {diff:>7.1%}   "
          f"<- differencing alone preserves it")
    print(f"  every row, incl. the {zerofrac:.1%} with no minutes {allrow:>7.1%}")
    print()
    print("share = var(conversion residual of the difference) / var(Net)")
    print("sd cut = 1 - sd(xNet)/sd(Net), which is what a threshold scales with")
    print()
    print(f"{'window':>7}  {'population':<11} {'share':>8} {'sd cut':>8} "
          f"{'n':>6} {'mean Net':>9}")

    baseline = None
    for w in WINDOWS:
        for pop in POPULATIONS:
            # Injective in (w, pop). `SEED + w*10 + len(pop)` collided on 'any' and
            # 'pos', which are both length 3 — harmless here but a label that does
            # not identify its stream is one nobody can reproduce a single cell from.
            rng = random.Random(SEED + w * 100 + POPULATIONS.index(pop))
            got = measure(allp, w, pop, rng)
            if got is None:
                print(f"{w:>7}  {pop:<11} {'-- too few pairs --':>24}")
                continue
            share, sdcut, n, mn = got
            if w == 1 and pop == 'any':
                baseline = share
            print(f"{w:>7}  {pop:<11} {share:>7.1%} {sdcut:>8.1%} "
                  f"{n:>6} {mn:>9.2f}")
    print()
    print("⚠️ Read `poolprice` as the operative row, NOT `posprice`. The optimiser "
          "will not buy below MinExpectedMinutes, so the whole field is not a "
          "transfer population: it is 60.1% blank rows against the 13% of real sold "
          "players who stop playing. `posprice` is the LOWER end of the bracket.")
    if baseline is not None:
        print(f"Per APPEARANCE the share is {app:.1%}; inside the pool at one "
              f"gameweek it is already close to that, and what takes it down is the "
              f"WINDOW, not the blanks.")
    print()
    reseed(allp)
    per_season(data)


def reseed(allp):
    """Re-run the operative column under several seeds.

    There is no closed-form SE here -- the estimand is a ratio of two variances
    over a sampled population -- so the honest substitute is to show the sampling
    spread directly and let the reader see which digits are real.

    ⚠️ This lives in the script for a reason. Three figures in the first write-up of
    this measurement came from throwaway shell commands, and the one that could not
    be regenerated was the one that turned out to be wrong. An uncertainty statement
    quoted from a command nobody can re-run would be the same mistake again.
    """
    print(f"Sampling spread on the operative column ({RESEEDS} seeds, poolprice):")
    print(f"{'seed':>12}  " + "  ".join(f"{'w=' + str(w):>9}" for w in WINDOWS))
    rows = {w: [] for w in WINDOWS}
    cuts = {w: [] for w in WINDOWS}
    for k in range(RESEEDS):
        seed = SEED + k * 7919
        cells = []
        for w in WINDOWS:
            rng = random.Random(seed + w * 100 + POPULATIONS.index('poolprice'))
            got = measure(allp, w, 'poolprice', rng)
            cells.append(got[0])
            rows[w].append(got[0])
            cuts[w].append(got[1])
        print(f"{seed:>12}  " + "  ".join(f"{c:>9.1%}" for c in cells))

    print()
    for w in WINDOWS:
        v, c = rows[w], cuts[w]
        print(f"  w={w}: share {min(v):.1%} to {max(v):.1%}   "
              f"sd cut {min(c):.1%} to {max(c):.1%}")

    mono = all(
        all(rows[WINDOWS[i]][k] > rows[WINDOWS[i + 1]][k]
            for i in range(len(WINDOWS) - 1))
        for k in range(RESEEDS))
    print()
    print(f"⚠️ Monotone decreasing in every one of {RESEEDS} seeds: {mono}. **The "
          f"ordering is what this measurement supports**, not any single cell.")
    print()
    print("⚠️ The sd cut is the UNSTABLE statistic and must be quoted with its range, "
          "never as a point. It is a difference of two nearly equal square roots, so "
          "it carries far more sampling error than the share it derives from.")


def per_season(data):
    """The noise that matters is BETWEEN SEASONS, and reseeding cannot see it.

    Three seasons is df 2, `t_crit` 4.303. A Monte-Carlo spread over seeds measures
    how well 4,000 pairs pin down one number on the data held; it says nothing about
    whether another season would give that number. The record's own rule is that the
    df comes from the comparison, so the season column is the one to read.
    """
    print()
    print("Between-season spread at w=5 — the noise a reseed cannot measure:")
    for pop in ('posprice', 'poolprice'):
        shares = []
        for s in xc.NATIVE_XG_SEASONS:
            one = {(s, pid): v for (ss, pid), v in data_flat(data).items() if ss == s}
            rng = random.Random(SEED + POPULATIONS.index(pop))
            got = measure(one, 5, pop, rng)
            shares.append(got[0] if got else float('nan'))
        m = sum(shares) / len(shares)
        sd = math.sqrt(sum((x - m) ** 2 for x in shares) / (len(shares) - 1))
        print(f"  {pop:<10} " +
              "  ".join(f"{s} {v:.1%}" for s, v in
                        zip(xc.NATIVE_XG_SEASONS, shares)) +
              f"   mean {m:.1%}, between-season SE {sd / math.sqrt(len(shares)):.2%}")
    print("  ⚠️ df 2, t_crit 4.303. Read this beside the seed spread, not instead of it.")


def data_flat(data):
    out = {}
    for s, players in data.items():
        for pid, v in players.items():
            out[(s, pid)] = v
    return out


if __name__ == '__main__':
    main()
