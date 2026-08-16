"""Audit the xPoints channels against what FPL actually pays.

Two rulebook audits — one on Opus, one on Fable, run blind to each other — each
found one thing the other missed. Both are checked here, because a finding from a
single pass is a hypothesis and this file is what makes it a measurement.

# 1. ⚠️ RETRACTED — the clean sheet is NOT compared against the wrong event

**The premise was false and FPL's own accounting refutes it.** This section first
claimed that `expected_goals_conceded` is a while-on-pitch quantity while the clean
sheet FPL pays is a TEAM-MATCH event, so the two were different events and the
comparison a category error.

FPL pays a clean sheet for **not conceding while on the pitch**, having played sixty
minutes. That is exactly what `exp(-xgc)` models. Two independent adjudications
settled it from the archive, which is FPL applying its own rule tens of thousands of
times:

  - Over six seasons and 22,605 GK/DEF rows at 60+ minutes, `clean_sheets == 1`
    **iff** the player's own `goals_conceded == 0`, with ZERO exceptions.
  - **89 rows carry a clean sheet in a match their club conceded in** — impossible
    under a team-match rule — and 88 of the 89 reconstruct `total_points` with the
    four points included, so FPL did not merely record them, it paid for them.
  - Per fixture, **77 of 77** within-club disagreements run "the substituted man is
    credited, the ninety-minute man is not". The reverse direction has no instances;
    the four that appear on ACCUMULATED rows are double gameweeks, i.e. arithmetic.

⚠️ **The band table below is real and its interpretation was not.** Two thirds of the
gap is club selection: matched within a single-fixture club-gameweek the effect falls
from −0.034/−0.059/−0.088 to **−0.0196 clean sheets**, which is **season-clustered
t −2.11 against t_crit 4.303 at df 2 — it does not resolve**. It is 3-4% of the
clean-sheet channel, not "the bigger half" of anything.

The band split is kept because it is the evidence for the retraction, and because it
demonstrates the rule this cost: **a band split does not identify a mechanism.**
Splitting a population on the variable a story is about will show the story's sign
whenever the groups differ in anything else. The matched comparison is the one that
identifies; this one only describes.

⚠️ The real mechanism is elsewhere and is a LEVEL error: `exp(-Σx)` exceeds
`Π(1-xᵢ)`, a Jensen gap. See `stats/xg_provider_scale.py`.

# 2. The missing-data gate is applied to one channel and not the others (Fable)

`XPointsResidual` gates the clean sheet on `XGC > 0`, with the argument — correct,
and stated at length in xpoints.go — that a zero xGC is MISSING DATA rather than a
guaranteed clean sheet.

The goals and assists channels carry no such gate. A row with a realised goal and
`xg == 0` is, in a season that has xG at all, a row whose xG is missing: a goal
implies a shot and a shot has positive xG. Ungated, the whole attacking return is
stripped as "conversion luck".

⚠️ This script reads the cache, which is the PRE-REPAIR state — `season.go`'s
Understat backfill is applied in memory and is not visible here. So what this
measures is the exposure the repair has to cover, not what survives it. The
post-repair residual needs Go and is stated as unmeasured rather than guessed at.
"""
import collections
import json
import math

import xpoints_common as xc

BANDS = [(60, 90, '60-89'), (90, 10 ** 9, '90+')]


def rows(seasons):
    for s in seasons:
        d = json.load(open(f'.cache/fpl/backtest-v8-{s}.json'))
        ps = d['players']
        for p in (list(ps.values()) if isinstance(ps, dict) else ps):
            et = p.get('element_type') or p.get('type')
            if et not in xc.GOAL:
                continue
            for gw, v in (p.get('gws') or {}).items():
                yield s, et, v


def _ols1(x, y):
    """Slope of y on x."""
    n = len(x)
    mx, my = sum(x) / n, sum(y) / n
    sxy = sum((a - mx) * (b - my) for a, b in zip(x, y))
    sxx = sum((a - mx) ** 2 for a in x)
    return sxy / sxx


def _ols2(x1, x2, y):
    """Slopes of y on (x1, x2) — two-regressor OLS by the normal equations."""
    n = len(y)
    m1, m2, my = sum(x1) / n, sum(x2) / n, sum(y) / n
    s11 = sum((a - m1) ** 2 for a in x1)
    s22 = sum((b - m2) ** 2 for b in x2)
    s12 = sum((a - m1) * (b - m2) for a, b in zip(x1, x2))
    s1y = sum((a - m1) * (c - my) for a, c in zip(x1, y))
    s2y = sum((b - m2) * (c - my) for b, c in zip(x2, y))
    det = s11 * s22 - s12 * s12
    return (s22 * s1y - s12 * s2y) / det, (s11 * s2y - s12 * s1y) / det


def go_expected_clean_sheets(v):
    """The expectation the Go instrument actually computes.

    ⚠️ `xc.expected_clean_sheets` is the PYTHON copy and it has drifted: the Go side
    caps the expectation at `eligible = min(Fixtures, Minutes/60)`, because the club's
    fixture count is not the number of clean sheets ON OFFER to a player who only
    played one leg of a double. The cap landed in Go and the mirror was never updated.

    A first version of this file published the Python form's numbers as the
    instrument's, which is the desynchronised-mirror class — and the drift is largest
    in the cell that most flattered the finding. `TestTheXPointsScriptsShareTheScoringTable`
    cannot catch it: it pins the CONSTANTS and says nothing about the FORM.
    """
    f = max(int(v.get('fixtures') or 1), 1)
    eligible = min(f, int(v['minutes']) // 60)
    return eligible * math.exp(-float(v['xgc']) / f)


def clean_sheet_exposure(seasons):
    """Over-prediction of the clean sheet, split by minutes band."""
    print("1. ⚠️ RETRACTED — THE CLEAN SHEET IS *NOT* SCORED AGAINST THE WRONG EVENT")
    print()
    print("   FPL pays it for not conceding WHILE ON THE PITCH at 60+ minutes, which")
    print("   is what exp(-xgc) models. Table kept as the evidence, not the finding:")
    print("   matched within club-gameweek the gap is a third this size and does not")
    print("   resolve (t -2.11 vs t_crit 4.303). A band split cannot identify a cause.")
    print()
    print(f"   {'season':<9} {'band':<7} {'n':>6} {'predicted':>10} {'real':>8} "
          f"{'over':>8}")
    for s in seasons:
        for lo, hi, label in BANDS:
            pred = real = 0.0
            n = 0
            for _, et, v in rows([s]):
                if xc.CS[et] <= 0 or et not in (1, 2):
                    continue
                if not (lo <= v['minutes'] < hi):
                    continue
                if float(v['xgc']) <= 0:
                    continue
                n += 1
                pred += go_expected_clean_sheets(v)
                real += v['clean_sheets']
            if n == 0:
                continue
            over = (pred - real) / real if real else float('nan')
            print(f"   {s:<9} {label:<7} {n:>6} {pred:>10.1f} {real:>8.0f} "
                  f"{over:>7.1%}")
    print()
    print()
    print("   ⚠️ The 13-36pp spread across seasons is the 90+ BASELINE arm moving")
    print("   (realised 90+ clean sheets rose 21.7 -> 27.4%), not the effect varying.")
    print("   The 60-89 column is nearly constant. Quote neither as a range.")


def missing_xg_exposure(seasons):
    """Attacking returns that would be stripped as luck because xG is absent."""
    print()
    print("2. THE MISSING-DATA GATE COVERS THE CLEAN SHEET AND NOT THE ASSISTS")
    print()
    print("   ⚠️ Counted PER CHANNEL. A first version used a joint `xg > 0 or xa > 0`")
    print("   gate, which skips a scorer whose ASSIST xA is missing — undercounting by")
    print("   ~2.3x and, worse, hiding WHICH channel is exposed. The goal channel has")
    print("   ZERO rows on a native season: a goal implies a shot and a shot has xG.")
    print("   The entire exposure is assists.")
    print()
    print(f"   {'season':<9} {'goal n':>7} {'goal pts':>9}   {'assist n':>9} "
          f"{'assist pts':>11}   (pre-repair cache)")
    for s in seasons:
        n_g = n_a = 0
        pts_g = pts_a = 0.0
        for _, et, v in rows([s]):
            if v['minutes'] <= 0:
                continue
            if v['goals'] > 0 and float(v['xg']) == 0:
                n_g += 1
                pts_g += v['goals'] * xc.GOAL[et]
            if v['assists'] > 0 and float(v['xa']) == 0:
                n_a += 1
                pts_a += v['assists'] * xc.ASSIST
        print(f"   {s:<9} {n_g:>7} {pts_g:>9.0f}   {n_a:>9} {pts_a:>11.0f}")
    print()
    print("   ⚠️ The cache is PRE-REPAIR. season.go backfills xG from Understat in")
    print("   memory, so most of this is repaired before xPoints ever sees it. What")
    print("   survives the harvest is UNMEASURED here and needs Go, not a guess.")
    print("   Two things hold regardless: the native-season rows are mostly LEGITIMATE")
    print("   (a won-penalty or deflected assist genuinely carries ~0 xA, so a naive")
    print("   `xg+xa > 0` gate would be wrong), and under FPL_NO_XG_REPAIR=1 the")
    print("   2022-23 figure is live — so the arm's result carries a data state.")


def bonus_leakage(seasons):
    """How much of the stripped conversion residual re-enters through bonus.

    Both audits found this independently and by different estimators, which is why
    it is the one item here that needed no adjudication. Reproduced once more so the
    number this record quotes comes from committed code.
    """
    print()
    print("3. BONUS IS CAUSALLY DOWNSTREAM OF THE CHANNELS BEING REPLACED")
    print()
    print("   BPS pays goals, assists and clean sheets, so the conversion luck the")
    print("   residual strips re-enters through the bonus column, which xPoints keeps.")
    print()
    print(f"   {'population':<24} {'n':>7} {'slope':>7} {'b_R':>7} {'b_E':>7}")
    groups = {'attackers (MID+FWD, 60+)': (3, 4), 'defenders (GKP+DEF, 60+)': (1, 2)}
    for label, ets in groups.items():
        xs, ys, rs, es = [], [], [], []
        for _, et, v in rows(seasons):
            if et not in ets or v['minutes'] < 60:
                continue
            att, cs = xc.unscaled_residuals(v, et)
            xs.append(att + cs)
            ys.append(float(v['bonus']))
            # R = what he actually did on the replaced channels; E = what he was
            # expected to do. The residual is R - E, so regressing bonus on BOTH is
            # what discriminates a real leak from an artefact of the shared E.
            r = v['goals'] * xc.GOAL[et] + v['assists'] * xc.ASSIST
            e = (float(v['xg']) * xc.GOAL[et] + float(v['xa']) * xc.ASSIST)
            if v['minutes'] >= xc.CS_MINUTES and xc.CS[et] > 0 and float(v['xgc']) > 0:
                r += v['clean_sheets'] * xc.CS[et]
                e += go_expected_clean_sheets(v) * xc.CS[et]
            rs.append(r)
            es.append(e)
        n = len(xs)
        slope = _ols1(xs, ys)
        b_r, b_e = _ols2(rs, es, ys)
        print(f"   {label:<24} {n:>7} {slope:>7.3f} {b_r:>7.3f} {b_e:>7.3f}")
    print()
    print("   ⚠️ b_E ~ 0 is the evidence, and the correlation is NOT quoted here.")
    print("   Conditional on what a player DID, what he was EXPECTED to do adds")
    print("   nothing to his bonus — BPS is a function of realised events only. That")
    print("   is why the univariate slope is a real leak and not an artefact of the")
    print("   residual and bonus sharing a term. A correlation would be large under")
    print("   any dependence and cannot size a leak.")
    print()
    print("   ⚠️ The leak SATURATES: bonus caps at 3 and is a within-match rank, so a")
    print("   hat-trick does not return 3.9 bonus. An argmax lives in the tail, so the")
    print("   effective leak on the decisions that matter is smaller than the slope.")
    print("   ⚠️ And n is not n independent draws — bonus is a rank within a match.")
    print()
    print("   The slope is the leak: for every point of conversion luck xPoints")
    print("   removes, that many points of the same luck stay inside via bonus.")
    print("   ⚠️ It biases the xPoints arm TOWARD the points arm, so part of the")
    print("   recorded recovered fraction is leakage rather than information in the")
    print("   underlying. Do not read the two channels as independent.")


def main():
    seasons = xc.NATIVE_XG_SEASONS
    print(f"Channel audit over {', '.join(seasons)}.\n")
    clean_sheet_exposure(seasons)
    missing_xg_exposure(seasons)
    bonus_leakage(seasons)


if __name__ == '__main__':
    main()
