"""How much of the replay's weekly noise is conversion variance?

Scoring the replay on expected points from REALISED underlying — xG instead of
goals, xA instead of assists, a per-fixture exp(-xGC) instead of the binary clean
sheet — is a lower-noise instrument only if the variance it removes is a large
share of the variance there is.

⚠️ **This measures a PER-PLAYER-GAMEWEEK variance share and nothing downstream of
it.** The first version computed a squad sd cut and a detection-threshold move
from this number; both steps were wrong and are removed rather than patched:

  - **The sd cut assumed additivity.** Var(points - residual) is measured
    directly below and is NOT Var(points) - Var(residual): the residual covaries
    positively with what remains, because high-xG players carry more conversion
    variance.
  - **The threshold move was never derived.** A detection threshold depends on
    the SE of the PAIRED difference across cells. Where two arms hold the same
    player, that player's conversion residual cancels EXACTLY — same football,
    same week — so the residual enters a paired difference only through the
    symmetric difference of the two elevens. Nobody has decomposed that, and the
    share there is not this share.
  - **Players are not independent within a gameweek**, so an XI's variance is not
    11x a player's. A club's clean sheet is one event shared by every defender
    owned. See `xpoints_guard.py`, where clustering raises the SE 1.8x.
"""
from xpoints_common import NATIVE_XG_SEASONS, appearances, unscaled_residuals


def var(xs):
    n = len(xs)
    m = sum(xs) / n
    return sum((x - m) ** 2 for x in xs) / (n - 1)


if __name__ == '__main__':
    pts, r_att, r_all, xpts = [], [], [], []
    for et, v, _ in appearances():
        a, c = unscaled_residuals(v, et)
        pts.append(v['points'])
        r_att.append(a)
        r_all.append(a + c)
        xpts.append(v['points'] - (a + c))

    vt, va, vall, vx = var(pts), var(r_att), var(r_all), var(xpts)
    n = len(pts)
    print(f"player-gameweeks with minutes, seasons {', '.join(NATIVE_XG_SEASONS)}: {n}")
    print()
    print(f"  variance of realised FPL points per player-gameweek : {vt:7.3f}  (sd {vt**0.5:.2f})")
    print(f"  variance of the goals/assists conversion residual   : {va:7.3f}  "
          f"({100*va/vt:4.1f}% of total)")
    print(f"  ... plus the clean-sheet residual                   : {vall:7.3f}  "
          f"({100*vall/vt:4.1f}% of total)")
    print()
    print(f"  variance of xPoints itself (points - residual)      : {vx:7.3f}")
    print(f"  additive prediction, Var(points) - Var(residual)    : {vt-vall:7.3f}")
    print(f"  implied covariance                                  : {(vt-vall-vx)/2:+7.3f}")
    print(f"  ACTUAL sd reduction from scoring on xPoints         : "
          f"{100*(1-(vx/vt)**0.5):4.1f}%   (additivity would say "
          f"{100*(1-(1-vall/vt)**0.5):.1f}%)")
    print()
    print("⚠️ That percentage is a PER-APPEARANCE figure. It does NOT transport to a")
    print("squad sd, to a paired difference, or to a detection threshold — see this")
    print("file's header for why each step fails. Quote the variance share; quote")
    print("nothing downstream of it until the paired decomposition is measured.")
