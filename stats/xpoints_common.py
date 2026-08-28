"""Shared scoring for the xPoints instrument's three probes.

One implementation, because the three scripts previously carried their own copy
of the residual and a review found the same defect in all three at once.

# ⚠️ The clean sheet must be computed PER FIXTURE

`season.go` accumulates a gameweek's rows, so a double gameweek arrives as ONE
entry with `fixtures = 2`, `xgc` summed over both matches and `clean_sheets`
counting up to 2. `exp(-xgc)` on that is `exp(-(x1+x2))`, where the expectation
over two matches is `exp(-x1) + exp(-x2)`. Through a non-linear function the
accumulated total is not the same quantity at all, and it under-predicts badly:
248 double rows of 11,049 carry 109 real clean sheets against 37 predicted.

That is this record's own doubles class — "a double gameweek is two archive
rows" — arriving somewhere new. It cost a published figure: the first version of
these scripts reported `exp(-xGC)` over-predicting clean sheets by **28.3%**, and
the corrected per-fixture figure is about **33%**. The 28.3% was then written up
as "independently reproducing" the record's recorded over-prediction, which made
a bug look like a corroboration.

`f * exp(-xgc/f)` splits the accumulated xGC evenly across the fixtures. That is
exact for a single fixture and an approximation for a double — the two matches
had different opponents — but it is the right functional form, where the summed
version is not.
"""
import json
import math

# FPL's per-position points, keyed by element_type. These are a SECOND copy of
# `goalPoints` and `cleanSheetPoints` in internal/analysis/metrics.go, which is the
# failure this project has recorded more than any other -- so they are pinned to
# the Go originals by TestTheXPointsScriptsShareTheScoringTable, and the Go side is
# in turn asserted against FPL's own published game_config by
# TestScoringConstantsMatchFPL. Do not edit either copy alone.
#
# (!) GOAL[1] was 6 here against the engine's 10. FPL pays a keeper 10 for a goal
# and publishes it; 6 was this file's own invention. Corrected 2026-08-14, and no
# published figure moves: there are ZERO keeper goals in all 33,878 appearances
# across the three native-xG seasons, so the divergence had never once been
# exercised. That is what makes it worth pinning rather than merely fixing -- a
# second implementation that agrees on every row of the data you have is exactly
# the one nobody notices going wrong.
#
# (!) AMENDED 2026-08-16: "6 was this file's own invention" is WRONG, and the
# correction stands anyway. 6 is a value FPL really paid -- what a keeper's goal
# was worth in 2020-21, decoded from the archive (Alisson GW36, 90 minutes, 1
# goal, 0 clean sheets, 1 conceded, 2 saves, 2 bonus, 0 cards, 0 own goals, 0
# penalties, total_points 10, so the goal was paid exactly 6).
#
# What is retracted is "invention". Whether THIS file's 6 descended from that rule
# or was simply typed is unknown and unrecoverable, and the correction does not
# need it: 10 is right for the seasons these scripts actually read.
#
# The Go instrument now prices each season under its own table
# (internal/analysis/scoringrules.go, the BankLimitFor of the instrument, which
# owns the boundary). THIS FILE DOES NOT: it carries one table, today's, for every
# season it is pointed at.
#
# (!) SIZE THE DIVERGENCE, do not assume it is zero. GOAL[1] multiplies xg as well
# as goals -- `(goals - xg) * GOAL[et]` below -- so every keeper row with xg > 0 is
# affected, not only the ones with a goal. Of NATIVE_XG_SEASONS, 2024-25 and
# 2025-26 are priced correctly (FPL's own game_config publishes GKP 10 from
# 2024-25 GW16); only **2023-24** falls in the unresolved span, and it holds one
# affected row -- Pope GW3, xg 0.02 -- worth 0.08 residual points. That is the
# whole cost at the default seasons.
#
# It is NOT zero anywhere else, and `appearances()` takes a `seasons=` argument:
# 2018-19, 2019-20 and 2022-23 carry three, three and two more such rows, and
# 2020-21 carries Alisson's goal at 3.6 xPoints. Fixing it properly means
# threading the season into `unscaled_residuals`, which changes five scripts and
# supersedes their banked figures -- owed, not done. Until then, do not point
# these scripts at a season outside NATIVE_XG_SEASONS and read a keeper number.
GOAL = {1: 10, 2: 6, 3: 5, 4: 4}
CS = {1: 4, 2: 4, 3: 1, 4: 0}
# Named rather than inline for the same reason, and pinned by the same test. The
# assist points were a bare `* 3` inside unscaled_residuals() and in xpoints_bias.py -- the
# identical shape as the keeper goal, one line away from it, and left unpinned by the
# first version of the guard. The 2026/27 rule discussion is live, so this is a
# constant that can actually move.
ASSIST = 3.0
# FPL pays the clean sheet only from sixty minutes. Same rule as `appearancePoints`
# being the long_play value; pinned so it cannot drift from the engine's threshold.
CS_MINUTES = 60
NATIVE_XG_SEASONS = ['2023-24', '2024-25', '2025-26']


def expected_clean_sheets(v):
    """Expected clean sheets for one gameweek entry, per fixture."""
    f = max(int(v.get('fixtures') or 1), 1)
    return f * math.exp(-float(v['xgc']) / f)


def appearances(seasons=None, require_cache=True):
    """Yield (element_type, gameweek entry, cluster key) per appearance with minutes.

    The cluster key is (season, gameweek, club) — the club-gameweek. A club's
    clean sheet is ONE event shared by every defender owned, so appearances
    within it are perfectly correlated on that channel and an iid SE is wrong.
    ⚠️ The gameweek number is the `gws` dict KEY and the club is on the PLAYER,
    not on the gameweek entry; a first attempt read both off the entry, got
    `None` for each, and silently collapsed to a single cluster.

    ⚠️ Reads `.cache/fpl/backtest-v9-*.json` DIRECTLY, so `season.go`'s ungated
    defect repairs — the phantom-match and duplicate-row drops — are NOT applied.
    Small for these seasons (10 duplicate rows in 2025-26, no phantoms) and
    stated because the rule is to state it.

    ⚠️ Raises on a missing cache file rather than skipping. Two of these scripts
    used to `continue`, so a missing season yielded a two-season figure printed
    under a three-season header.
    """
    seasons = seasons or NATIVE_XG_SEASONS
    for s in seasons:
        path = f'.cache/fpl/backtest-v9-{s}.json'
        try:
            d = json.load(open(path))
        except FileNotFoundError:
            if require_cache:
                raise SystemExit(
                    f"missing {path} — run from the repo root. Refusing to print a "
                    f"figure over fewer seasons than its header claims.")
            continue
        ps = d['players']
        for p in (list(ps.values()) if isinstance(ps, dict) else ps):
            # element_type 5 is the assistant managers FPL ran for 2024-25 —
            # 322 archive rows accumulating to 312 player-gameweeks and carrying
            # 1,861 points, and none in any other season — correctly excluded,
            # since they score on none of these channels.
            #
            # (!) This key-presence skip is what the GO side lacked until
            # 2026-08-16: `XPointsResidual` read `goalPoints[position]` as a bare
            # map index, so an unpriced position had its goals channel silently
            # priced at ZERO and still returned a plausible number. The mirror was
            # the safer of the two implementations. It now refuses, in
            # `ScoringRules.Prices`.
            et = p.get('element_type') or p.get('type')
            if et not in GOAL:
                continue
            for gw, v in (p.get('gws') or {}).items():
                if v['minutes'] <= 0:
                    continue
                yield et, v, (s, int(gw), p.get('team'))


def unscaled_residuals(v, et):
    """(attacking, clean-sheet) conversion residual for one appearance, UNSCALED.

    (!) This is DELIBERATELY no longer the Go instrument's arithmetic, and the
    divergence is declared here rather than left latent.

    `internal/analysis/xpoints.go` prices xG and xA through a per-season,
    per-position ConversionScale, because a raw xG is not an FPL goal and a raw xA
    is emphatically not an FPL assist -- a forward's assists/xA runs at ~2.1. This
    function keeps the raw difference.

    That is correct for ONE consumer and a caveat for the others:

      - xpoints_bias.py MUST stay unscaled. Measuring the cross-position bias is
        its whole subject, and the Go instrument's in-sample fit now drives that
        bias to zero by construction, so this is the only remaining way to observe
        the quantity at all.

      - xpoints_channel_audit.py sizes the bonus leak against the removed
        residual, which is a property of the raw channels. Unaffected.

      - (!) xpoints_variance.py and xpoints_permove.py report properties of the
        INSTRUMENT'S residual -- the variance share it removes, and a transfer's
        share of it. Scaling changes that residual's variance, materially on the
        forward assists channel where `A - xA` becomes `A - 2.1*xA`. Their banked
        figures are therefore figures on the superseded instrument, exactly as the
        gate arm's are. Re-deriving them is queued; do not read them as current.

      - (!) xpoints_guard.py sizes the aggregate realised-points guard against the
        residual's SD, which scaling moves, so its printed "detectable bias" figures
        are on the superseded instrument too. Its VERDICT -- that the guard is not
        sharper than the thing it guards -- is a comparison against a threshold and
        is not obviously overturned by a rescaling, but it has not been re-derived
        either. It is the FIFTH caller and a first version of this list omitted it.

    `TestTheXPointsScriptsShareTheScoringTable` pins the CONSTANTS across the two
    sides and says nothing about the form, which is what let the xGC gate drift
    once already. The name is the guard here: a caller reaching for "the residual"
    has to type the word `unscaled` and is thereby told.
    """
    att = (v['goals'] - v['xg']) * GOAL[et] + (v['assists'] - v['xa']) * ASSIST
    cs = 0.0
    # (!) Gated on xgc > 0, matching internal/analysis/xpoints.go and baseXP90.
    #
    # A zero xGC is MISSING DATA, not a guaranteed clean sheet: exp(-0) = 1, so
    # without this every player in a season carrying no xGC is handed a certain
    # clean sheet and then charged the full four points for not keeping it.
    #
    # Benign at this file's default population -- 1, 0 and 2 qualifying rows across
    # the three native-xG seasons -- and NOT benign at any other, since
    # `appearances()` takes a `seasons=` argument and four archived seasons have no
    # native xGC at all. Added after code review found the Go side gated and this
    # side not: TestTheXPointsScriptsShareTheScoringTable pins the CONSTANTS and
    # nothing about the FORM, so the two residuals can otherwise drift apart
    # silently. Queued.
    if v['xgc'] > 0 and v['minutes'] >= CS_MINUTES and CS[et] > 0:
        cs = (v['clean_sheets'] - expected_clean_sheets(v)) * CS[et]
    return att, cs


def clustered_se_of_mean(vals, keys):
    """CR0 standard error of a mean, clustered on keys."""
    import collections
    n = len(vals)
    m = sum(vals) / n
    byg = collections.defaultdict(float)
    for v, k in zip(vals, keys):
        byg[k] += (v - m)
    return math.sqrt(sum(s * s for s in byg.values())) / n, len(byg)
