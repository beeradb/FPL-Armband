#!/usr/bin/env python3
"""Backfill missing per-gameweek expected goals and assists from Understat.

    python3 stats/understat_xg_backfill.py --calibrate        # report the provider ratios
    python3 stats/understat_xg_backfill.py --season 2022-23   # harvest, calibrate, write
    python3 stats/understat_xg_backfill.py --season 2022-23 --check   # validate a written repair
    python3 stats/understat_xg_backfill.py --all              # every repairable season

# Two different defects, and they want different calibrations

**2022-23 has a hole in a series that exists.** `gws/merged_gw.csv` records
`expected_goals` as zero for gameweeks 1-15, with entirely normal minutes (~19,700 a
gameweek), and the series begins at GW16. The weekly rows sum to 731.5 against
`players_raw.csv`'s season aggregate of 1097.3 — exactly 0.6666, where 2023-24 sums to
1.000. FPL backfilled the season totals when it introduced the statistic in December
2022 and never backfilled the weekly history. The same GW16 boundary produces the
recorded `starts` defect: one event, two symptoms.

**2019-20, 2020-21 and 2021-22 have no series at all.** Checked against the headers
rather than assumed: `expected_goals`, `expected_assists` and
`expected_goals_conceded` are **absent as columns** from both `players_raw.csv` and
`gws/merged_gw.csv` in all three. FPL did not publish the statistic yet.

That difference decides the calibration and it is the main thing to understand before
reading any number this script prints:

- For **2022-23** the provider offset is fitted **within the season**, on GW16-38 where
  both sources exist, and the season aggregate — which the archive carries complete and
  the repair never sees — is an independent validation target. That is the strongest
  form of this repair and it is unchanged from the original version of this script.
- For the three older seasons **neither is available**. There is no overlap window to
  fit on and no aggregate to validate against, because the quantity does not exist in
  the archive in any form. The offset therefore has to be carried in from the seasons
  that do have both, which is a weaker claim and is stated as one.

# Why the archive cannot repair itself, which is the trap

The obvious repair for 2022-23 uses only archive data: the season aggregate is complete
and the GW16-38 weekly values are known, so GW1-15's total is aggregate minus their sum,
and it could be distributed across those weeks in proportion to minutes.

**That is a point-in-time leak and must not be done.** The season aggregate includes
matches after the cutoff, so a model built through GW5 would be reading a quantity
derived from the whole season. The replay's entire discipline is that nothing after the
cutoff is visible, and `TestPointInTimeHidesFutureResults` exists because a feature once
trained on unplayed matches. A repair that is *exactly* right in aggregate and leaky per
week is worse than the hole, because the leak is invisible and inflates every figure at
once.

Understat is the honest source precisely because it is **per match**: a match has a
date, so it can be truncated at any cutoff the replay chooses. Note the trap does not
even arise for the three older seasons — there is no aggregate to leak from.

# What is repaired and what is deliberately not

- **xG: repaired**, divided by the measured provider ratio so the repaired weeks sit on
  FPL's scale. Understat runs within a few per cent of Opta in level.
- **xA: repaired, with the provider offset removed.** Understat reads high — it credits
  xA on any key pass leading to a shot, where Opta's expected assists is calibrated to
  assists. The offset is definitional rather than sloppy, and it is **not constant
  between seasons**, which is why it is measured on every overlap season available
  rather than taken as a constant.
- **xGC: NOT repaired, and not by omission.** Understat publishes team xGA per match,
  not per player. The recorded finding is that the *per-player* figure carries the
  substitution channel a club rate would delete — worth +0.140/+0.067/+0.007 pts/gw
  across substitution terciles — so a prorated club rate would be a *different
  quantity* present only in the repaired weeks. That is a within-grid inconsistency,
  not merely extra noise. Defenders' clean-sheet inputs therefore stay unrepaired.

# The crosswalk

`id_dict.csv` is bundled with the archive for **2021-22 and 2022-23 only** — checked,
not assumed; 2019-20, 2020-21 and 2023-24 onward have none. So the crosswalk is built
instead from FPL's permanent `code`:

- `code` -> understat id comes from `ChrisMusson/FPL-ID-Map`'s `Master.csv`. One column
  from an unlicensed repository, and it is a mapping between public identifiers.
- `code` -> element id comes from **the archive's own `players_raw.csv`**, which carries
  both. So the season-specific half is re-derived here rather than depended on, and the
  external dependency is one column that does not change when a season is added.

It is validated three ways, because a hand-maintained mapping is exactly what this
project keeps getting bitten by. See `--calibrate`, which prints all three: agreement
with the bundled `id_dict` where it exists, coverage of the players who matter, and —
the one that does not select for easy cases — **Understat minutes and goals against
FPL's own**, since a wrong id produces neither.

# The goals anchor, and the two things adding 2018-19 taught it

Goals are the strongest check available: both sources count an exact integer, so a wrong
id or a date mapped to the wrong gameweek shows here and nowhere else. Two corrections
came out of extending the harvest back a season, and both make the check sharper:

- **Own-goal cells are excluded.** Understat credits an own goal to the attacker who
  forced it and FPL credits it to the defender, so the anchor was measuring a
  definitional difference. It read 1.0019 on 2018-19 for exactly that reason, on two
  cells where the join is provably right — the minutes match to the minute. Excluded, the
  season reads 1.0000 with no disagreeing cell. See `own_goal_cells`.
- **The disagreeing-cell count is reported beside the ratio, and it is the sharper
  number.** A ratio of exactly 1.0000 is *not* proof the join is right: 2019-20 reads
  1.0000 with two cells disagreeing, because Understat gives De Bruyne a goal FPL gives
  to David Silva. One cell over and one cell short cancel — and that is precisely the
  signature of a mis-mapped date, the thing the anchor exists to catch. See
  `count_goal_mismatches`.

# Politeness

Understat is a free site with no stated licence. This makes one request per mapped
player, once, with a delay between them, and caches every response so a re-run costs
nothing. A player's response carries every season he played, so adding a season mostly
re-reads the cache. Do not remove the delay.
"""

import argparse
import csv
import gzip
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.request
import zlib
from collections import defaultdict

ARCHIVE = "https://raw.githubusercontent.com/vaastav/Fantasy-Premier-League/master/data"
IDMAP = "https://raw.githubusercontent.com/ChrisMusson/FPL-ID-Map/main/Master.csv"

CACHE = os.path.expanduser("~/.cache/fplagent/understat")
# Inside the package that consumes it, because `go:embed` cannot reach outside its own
# directory tree — and embedding is what makes the repair travel with the binary
# instead of depending on a path that resolves differently per caller.
OUTDIR = os.path.join("internal", "backtest", "repairdata")

# One request per player, spaced.
DELAY_S = 0.35

# The seasons this script repairs, and how each one differs.
#
# Deliberately a table rather than "whatever season is asked for". A repair that applies
# to any season is a repair nobody can reason about later, and this project already
# gates the transfer bank and defensive contribution by season for the same reason:
# handing a season a rule it was not played under is how a replay measures something
# that never happened. Each entry had its own header checked.
#
#   ulabel      Understat's name for the season. It uses the opening year.
#   first, last the gameweek window repaired, in FPL's post-renumber numbering.
#   inseason    True when the season has an FPL xG series to fit the offset against.
#   renumber    True for 2019-20 only. See `renumber_gw`.
#   xwalk       which crosswalk to join through. See below — this is not a detail.
#
# `xwalk` is "bundled" for 2022-23 and "code" for the rest, and the reason is
# reproducibility rather than preference. 2022-23's committed repair was built through
# the archive's own `id_dict.csv` (620 players), and the code-based crosswalk reaches
# 772 — strictly better coverage, and therefore a *different* repair. Every 2022-23
# figure in the record rests on the file as committed, so regenerating it with the wider
# crosswalk is an improvement that changes a measured population, which is a decision
# wanting its own pass and not a side effect of adding seasons. `--compare-crosswalks`
# reports what the change would be worth without applying it.
#
# 2018-19 is here for a different purpose from the other three and it is worth knowing
# which. The archive publishes no `teams.csv` for it, so it cannot be *played* — the
# loader marks it prior-only — and it is repaired solely so that it can be the **prior**
# for 2019-20, which takes the playable set from six seasons to seven. A prior is read
# through the season *total*, so per-gameweek granularity is more than that job needs;
# it is written anyway because the harvest is per match either way, the loader rebuilds
# the total from the weeks, and a second output format would be a second thing to keep
# equal. Its window is therefore the whole season like the others.
SEASONS = {
    "2018-19": dict(ulabel="2018", first=1, last=38, inseason=False, renumber=False,
                    xwalk="code"),
    "2019-20": dict(ulabel="2019", first=1, last=38, inseason=False, renumber=True,
                    xwalk="code"),
    "2020-21": dict(ulabel="2020", first=1, last=38, inseason=False, renumber=False,
                    xwalk="code"),
    "2021-22": dict(ulabel="2021", first=1, last=38, inseason=False, renumber=False,
                    xwalk="code"),
    "2022-23": dict(ulabel="2022", first=1, last=15, inseason=True, renumber=False,
                    xwalk="bundled"),
}

# The seasons carrying an FPL xG series, used to measure the provider ratio. 2022-23 is
# included but only its GW16-38 half is usable, which `overlap_first` records.
CALIBRATION_SEASONS = {
    "2022-23": dict(ulabel="2022", overlap_first=16),
    "2023-24": dict(ulabel="2023", overlap_first=1),
    "2024-25": dict(ulabel="2024", overlap_first=1),
    "2025-26": dict(ulabel="2025", overlap_first=1),
}

# Only players with a real body of football, when measuring the ratio. A cameo
# contributes a rounding error to both sides and noise to neither usefully.
CALIBRATION_MIN_MINUTES = 900


def fetch(url, headers=None, tries=4):
    """One GET, with backoff on a transport failure.

    Retried rather than allowed to fail, because a harvest is several hundred requests
    and a single reset two thirds of the way through otherwise loses the run. Only
    transport errors are retried: an HTTP status is the server's answer and repeating
    the request will not change it, so `HTTPError` propagates.

    The backoff is deliberately long relative to `DELAY_S`. A reset from a free site is
    most likely it asking for less traffic, and the right response to that is to slow
    down rather than to hammer it.
    """
    ctx = ssl.create_default_context()
    for attempt in range(tries):
        req = urllib.request.Request(
            url, headers=headers or {"User-Agent": "Mozilla/5.0"})
        try:
            with urllib.request.urlopen(req, context=ctx, timeout=60) as r:
                raw = r.read()
            break
        except urllib.error.HTTPError:
            raise
        except (urllib.error.URLError, OSError) as e:
            if attempt == tries - 1:
                raise
            wait = 5 * (attempt + 1)
            print(f"    {type(e).__name__} on {url}; retrying in {wait}s "
                  f"({attempt + 1}/{tries - 1})", file=sys.stderr)
            time.sleep(wait)
    if raw[:2] == b"\x1f\x8b":
        try:
            return gzip.decompress(raw)
        except OSError:
            return zlib.decompress(raw, 16 + zlib.MAX_WBITS)
    return raw


def cached_csv(url, key):
    """Fetch and parse a CSV, cached on disk.

    utf-8-sig, because `Master.csv` carries a byte-order mark and its first column
    would otherwise be named "﻿code" — which reads as a missing column rather
    than as an encoding problem.
    """
    os.makedirs(CACHE, exist_ok=True)
    path = os.path.join(CACHE, key)
    if not os.path.exists(path):
        with open(path, "wb") as f:
            f.write(fetch(url))
    with open(path, encoding="utf-8-sig", errors="replace") as f:
        return list(csv.DictReader(f))


def archive_csv(season, name):
    return cached_csv(f"{ARCHIVE}/{season}/{name}",
                      season + "-" + name.replace("/", "_"))


def player_data(uid):
    """Understat per-match rows for one player, cached.

    One response carries every season he played, so the cache is shared across every
    season this script touches and adding a season is mostly free.
    """
    os.makedirs(CACHE, exist_ok=True)
    path = os.path.join(CACHE, f"player-{uid}.json")
    if os.path.exists(path):
        with open(path, encoding="utf-8") as f:
            return json.load(f)
    time.sleep(DELAY_S)
    body = fetch(
        f"https://understat.com/getPlayerData/{uid}",
        headers={
            "User-Agent": "Mozilla/5.0",
            "X-Requested-With": "XMLHttpRequest",
            "Referer": f"https://understat.com/player/{uid}",
        },
    )
    d = json.loads(body.decode("utf-8"))
    with open(path, "w", encoding="utf-8") as f:
        json.dump(d, f)
    return d


def renumber_gw(gw, on):
    """2019-20's gameweeks are numbered 1-29 then 39-47, and must become 1-38.

    COVID stopped the season after GW29; FPL numbered the restarted rounds 39-47 rather
    than reusing 30-38. Checked against `fixtures.csv`: 38 distinct events, 380
    fixtures, events 30-38 entirely absent, so the shift is exactly minus nine and it
    cannot collide with a real gameweek.

    This must agree with the loader's own renumber or the repair rows land in the wrong
    weeks — the same quantity written twice, which this project's rule says must be
    checked rather than watched. `TestNineteenTwentyIsRenumberedToThirtyEight` pins the
    Go side and reproduces this arithmetic.
    """
    if on and gw >= 39:
        return gw - 9
    return gw


def bundled_crosswalk(season):
    """element id -> understat id from the archive's own `id_dict.csv`.

    Present for **2021-22 and 2022-23 only** — checked against the repository, not
    assumed. Returns None where it is absent.
    """
    try:
        idd = archive_csv(season, "id_dict.csv")
    except urllib.error.HTTPError:
        return None
    out = {}
    for r in idd:
        # 2021-22's header carries leading spaces on three of its four columns
        # (" FPL_ID"), where 2022-23's does not. Stripping the keys is what makes one
        # reader work on both, and it is the kind of difference a header check finds
        # and an assumption does not.
        r = {(k or "").strip(): (v or "").strip() for k, v in r.items()}
        out[int(r["FPL_ID"])] = r["Understat_ID"]
    return out


def code_crosswalk(season):
    """element id -> understat id via FPL's permanent `code`.

    Two halves, deliberately from two places. `code` -> understat comes from
    `ChrisMusson/FPL-ID-Map`, which is one column from an unlicensed repository and a
    mapping between public identifiers. `code` -> element id is re-derived from **the
    archive's own `players_raw.csv`**, which carries both — so the season-specific half
    is not depended on, and adding a season needs nothing new from outside.
    """
    code2u = {}
    for r in cached_csv(IDMAP, "idmap-Master.csv"):
        c, u = (r.get("code") or "").strip(), (r.get("understat") or "").strip()
        if c and u:
            code2u[int(c)] = u
    out = {}
    for r in archive_csv(season, "players_raw.csv"):
        u = code2u.get(int(r["code"]))
        if u:
            out[int(r["id"])] = u
    return out


def crosswalk(season, which="code"):
    """element id -> understat id, plus the coverage figures.

    Coverage is returned rather than left to be assumed, because a crosswalk that
    silently reaches half the league is the failure mode this has to rule out.
    """
    el2u = bundled_crosswalk(season) if which == "bundled" else code_crosswalk(season)
    if el2u is None:
        raise SystemExit(f"{season}: no bundled id_dict.csv in the archive")
    el2min, el2name = {}, {}
    for r in archive_csv(season, "players_raw.csv"):
        el = int(r["id"])
        el2min[el] = float(r.get("minutes") or 0)
        el2name[el] = r.get("web_name") or ""
    # A bundled crosswalk can name an element this season does not have.
    el2u = {el: u for el, u in el2u.items() if el in el2min}
    total_min = sum(el2min.values()) or 1.0
    mapped_min = sum(m for el, m in el2min.items() if el in el2u)
    regulars = [el for el, m in el2min.items() if m >= 900]
    cov = dict(
        players=len(el2min), mapped=len(el2u),
        minutes_share=mapped_min / total_min,
        regulars=len(regulars),
        regulars_mapped=sum(1 for el in regulars if el in el2u),
    )
    return el2u, el2min, el2name, cov


def check_against_bundled(season, el2u):
    """Third-party agreement, where the archive bundles its own crosswalk.

    Two maps built by different methods — exact name matching by the archive's
    maintainer, FPL's own `code` field here — so an agreement is a genuine
    cross-check and a disagreement names a specific player to look at.
    """
    idd = bundled_crosswalk(season)
    if idd is None:
        return None
    agree, disagree, bad = 0, 0, []
    for el, u in idd.items():
        if el not in el2u:
            continue
        if el2u[el] == u:
            agree += 1
        else:
            disagree += 1
            bad.append((el, u, el2u[el]))
    return agree, disagree, bad


def gameweek_by_date(season, renumber):
    """Map a calendar date to an FPL gameweek, from the archive's own gameweek rows.

    Built from `merged_gw.csv` rather than `fixtures.csv`, because that is the file
    whose gameweek numbering the loader actually uses — deriving the mapping from a
    different file is how two views of one quantity drift apart.

    A date can carry two gameweeks when a round straddles midweek, so the mapping keeps
    the gameweek that the majority of that date's rows carry. Ambiguity is reported
    rather than silently resolved.
    """
    rows = archive_csv(season, "gws/merged_gw.csv")
    key = "GW" if "GW" in rows[0] else "round"
    counts = defaultdict(lambda: defaultdict(int))
    for r in rows:
        ko = (r.get("kickoff_time") or "")[:10]
        gw = r.get(key)
        if ko and gw:
            counts[ko][renumber_gw(int(float(gw)), renumber)] += 1
    out, ambiguous = {}, 0
    for date, gws in counts.items():
        if len(gws) > 1:
            ambiguous += 1
        out[date] = max(gws.items(), key=lambda kv: kv[1])[0]
    return out, ambiguous


def archive_cells(season, renumber):
    """Per (element, gameweek): FPL's own minutes, goals, assists, xG and xA.

    Minutes is what filters foreign matches out of the harvest; goals is the
    independent value-level check on the join; xG and xA are what the offset is fitted
    against where they exist.
    """
    minutes = defaultdict(float)
    goals = defaultdict(float)
    assists = defaultdict(float)
    xg = defaultdict(float)
    xa = defaultdict(float)
    rows = archive_csv(season, "gws/merged_gw.csv")
    key = "GW" if "GW" in rows[0] else "round"
    for r in rows:
        gw = renumber_gw(int(float(r[key])), renumber)
        el = int(r["element"])
        minutes[(el, gw)] += float(r.get("minutes") or 0)
        goals[(el, gw)] += float(r.get("goals_scored") or 0)
        assists[(el, gw)] += float(r.get("assists") or 0)
        xg[(el, gw)] += float(r.get("expected_goals") or 0)
        xa[(el, gw)] += float(r.get("expected_assists") or 0)
    return minutes, goals, assists, xg, xa


def own_goal_cells(season, renumber):
    """Cells whose club benefited from an opponent's own goal that gameweek.

    These are excluded from the goals anchor, and the reason is a definitional
    difference between the two sources rather than a defect in either.

    **Understat credits an own goal to the attacker who forced it; FPL credits it to the
    defender as an own goal.** Found by the anchor refusing to read 1.0000 on 2018-19,
    and verified against the archive's own `fixtures.csv` stats blob rather than
    inferred: Burnley's two goals against Fulham on 2019-01-12 are recorded by FPL as own
    goals by Denis Odoi and Joe Bryan, with two *assists* for Jeff Hendrick and eleven
    points that arithmetic confirms contain no goal — and Understat gives Hendrick a
    goal. Huddersfield's goal against Arsenal on 2019-02-09 is Sead Kolasinac's own goal
    for FPL and Karlan Grant's goal for Understat.

    So the anchor was measuring a real disagreement about *whose* goal it was, on cells
    where the join itself is provably right — the minutes match to the minute. Excluding
    the beneficiary club's cells for that gameweek takes 2018-19 from 1.001927 with two
    mismatched cells to **1.000000 with none**, and leaves the other four seasons at
    1.000000 where they already were.

    Derived from `merged_gw.csv`'s own `own_goals` and `opponent_team` columns, checked
    present in every season this script touches. An own goal by a player means his
    *opponent* benefited, so the cells to drop are that opponent's. Deliberately drops
    whole cells rather than adjusting a value: an exclusion cannot introduce an error, and
    a correction that guessed which attacker Understat credited could.

    It costs about 4-5% of the joined cells, which is a lot of cells for a handful of
    goals — own goals are rare and clubs field twenty-odd players — and none of it
    matters, because this set is used for the *anchor only* and never to decide which
    rows are written or what the offset is.
    """
    team = {int(r["id"]): int(r["team"])
            for r in archive_csv(season, "players_raw.csv")}
    rows = archive_csv(season, "gws/merged_gw.csv")
    key = "GW" if "GW" in rows[0] else "round"
    benefited = set()
    for r in rows:
        if float(r.get("own_goals") or 0) > 0:
            benefited.add((int(r["opponent_team"]),
                           renumber_gw(int(float(r[key])), renumber)))
    out = set()
    for r in rows:
        el = int(r["element"])
        gw = renumber_gw(int(float(r[key])), renumber)
        if (team.get(el), gw) in benefited:
            out.add((el, gw))
    return out


def goals_anchor_cells(season, renumber, u_min, minutes):
    """The cells the goals anchor is read on: joined, played, and no own goal involved."""
    og = own_goal_cells(season, renumber)
    return [k for k in u_min if minutes.get(k, 0) > 0 and k not in og]


def count_goal_mismatches(cells, u_goals, goals):
    """Cells where the two sources disagree on the goal count.

    Reported beside the ratio because **the ratio alone cancels compensating errors**,
    and that is not hypothetical. 2019-20's anchor reads exactly 1.000000 and has two
    mismatched cells: Understat gives Kevin De Bruyne a Manchester City goal in GW10
    that FPL gives to David Silva, so one cell is +1 and the other is -1 and the ratio
    is blind to both. Season totals agree — De Bruyne 14 against FPL's 13, David Silva 5
    against 6.

    A mis-mapped *date* has the same signature, one week over and one week short, which
    is precisely what the anchor is meant to catch. So a ratio of 1.0000 was never proof
    the join was right, and this count is the sharper of the two checks.
    """
    return sum(1 for k in cells if u_goals.get(k, 0) != goals.get(k, 0))


def harvest(season, ulabel, renumber, el2u, minutes, only=None, verbose=True):
    """Understat xG, xA, minutes and goals per (element, gameweek) for one season.

    `only` restricts the population, which the calibration pass uses to keep both sides
    of its ratio on the same players.
    """
    date_gw, ambiguous = gameweek_by_date(season, renumber)
    u_xg, u_xa, u_min, u_goals = (defaultdict(float) for _ in range(4))
    missing = failed = foreign = 0
    items = [(el, u) for el, u in sorted(el2u.items()) if only is None or el in only]
    for i, (el, uid) in enumerate(items):
        try:
            d = player_data(uid)
        except (urllib.error.HTTPError, urllib.error.URLError, OSError, ValueError) as e:
            failed += 1
            print(f"  element {el} (understat {uid}): {e}", file=sys.stderr)
            continue
        for mt in d.get("matches", []):
            if mt.get("season") != ulabel:
                continue
            gw = date_gw.get((mt.get("date") or "")[:10])
            if gw is None:
                missing += 1
                continue
            # Reject matches that are not Premier League.
            #
            # `getPlayerData` returns a player's matches across EVERY league, and
            # Understat labels every league's 2022-23 as "2022" — so a player who moved
            # carries Ligue 1, Bundesliga and Serie A rows in the same response.
            # Checked: 98 distinct teams appear in season-2022 rows, including Paris
            # Saint Germain, VfB Stuttgart and Sassuolo. Filtering on the date alone
            # silently admitted every foreign match whose date happened to coincide
            # with a Premier League gameweek, which is what put the first validation
            # 3.3% OVER the season aggregate.
            #
            # The filter is the archive's own minutes rather than a team-name list,
            # because Understat and FPL spell clubs differently ("Tottenham" against
            # "Spurs") and a hand-maintained mapping is the thing this project has been
            # bitten by repeatedly. If the archive records no minutes for this player in
            # this gameweek then he played no Premier League football in it, so whatever
            # Understat has for that date belongs to another league. A player cannot
            # play two matches in one day, so this cannot reject a real appearance.
            if minutes[(el, gw)] <= 0:
                foreign += 1
                continue
            u_xg[(el, gw)] += float(mt.get("xG") or 0)
            u_xa[(el, gw)] += float(mt.get("xA") or 0)
            u_min[(el, gw)] += float(mt.get("time") or 0)
            u_goals[(el, gw)] += float(mt.get("goals") or 0)
        if verbose and (i + 1) % 200 == 0:
            print(f"  {i + 1}/{len(items)} players")
    if verbose:
        print(f"  harvest: {len(items)} players, {failed} failed, {missing} matches "
              f"whose date mapped to no gameweek, {foreign} rejected as not Premier "
              f"League, {ambiguous} dates carried more than one gameweek")
    return u_xg, u_xa, u_min, u_goals


def ratio_over(cells, num, den):
    """A ratio of two measurements of the SAME cells.

    The two sums MUST cover the same (element, gameweek) cells. The first version of
    this script summed Understat over mapped players and FPL over EVERY player, which is
    a mismatched population: the crosswalk does not reach everyone, so the ratio came
    out too low and dividing by it under-corrected. A calibration is a ratio of two
    measurements of the same thing or it is not a calibration.
    """
    n = sum(num.get(k, 0.0) for k in cells)
    d = sum(den.get(k, 0.0) for k in cells)
    return ((n / d) if d else float("nan")), n, d


def offset_cells(u, fpl, after_gw):
    """The cells the provider offset is fitted on, for one metric.

    Conditioned on **this metric's own** FPL denominator being non-zero, and on the
    Understat side having harvested the cell at all. That is the definition the
    committed 2022-23 repair was built with, and it is kept exactly: the cell set for
    xG and the cell set for xA are therefore *different*, each being the cells where
    that quantity exists on both sides. Widening either — to cells where the other
    metric is present, say — moves the ratio and so moves a data file every recorded
    2022-23 figure rests on.
    """
    return [k for k in u if k[1] > after_gw and fpl.get(k, 0) > 0]


def measure_ratios(verbose=True):
    """The provider offset, on every season that carries both sources.

    Returns {season: {"xg": r, "xa": r, ...}}. This is the only place the offset is
    computed, and the three older seasons have to borrow from it because they carry no
    FPL xG at all.
    """
    out = {}
    for season, spec in CALIBRATION_SEASONS.items():
        el2u, el2min, _, cov = crosswalk(season)
        pop = {el for el, m in el2min.items() if m >= CALIBRATION_MIN_MINUTES}
        minutes, goals, assists, fxg, fxa = archive_cells(season, False)
        u_xg, u_xa, u_min, u_goals = harvest(
            season, spec["ulabel"], False, el2u, minutes, only=pop, verbose=False)
        after = spec["overlap_first"] - 1
        rxg, nxg, dxg = ratio_over(offset_cells(u_xg, fxg, after), u_xg, fxg)
        rxa, nxa, dxa = ratio_over(offset_cells(u_xa, fxa, after), u_xa, fxa)
        # Minutes and goals are the checks on the join rather than the offset, so they
        # run over every cell with football, not only those with an xG denominator.
        played = [k for k in u_min if k[1] > after and minutes.get(k, 0) > 0]
        rmin, _, _ = ratio_over(played, u_min, minutes)
        # One definition of the goals anchor, here as in repair(): own-goal cells
        # excluded, mismatched cells counted beside the ratio. Two definitions of one
        # quantity is the bug class this project keeps rediscovering, and a calibration
        # pass that measured the join differently from the harvest would be exactly that.
        gcells = [k for k in goals_anchor_cells(season, False, u_min, minutes)
                  if k[1] > after]
        rgoals, ng, dg = ratio_over(gcells, u_goals, goals)
        gbad = count_goal_mismatches(gcells, u_goals, goals)
        out[season] = dict(xg=rxg, xa=rxa, minutes=rmin, goals=rgoals,
                           goal_mismatch=gbad, cells=len(played), cov=cov)
        if verbose:
            print(f"  {season}: xG {rxg:.4f} ({nxg:.1f}/{dxg:.1f})   "
                  f"xA {rxa:.4f} ({nxa:.1f}/{dxa:.1f})   "
                  f"minutes {rmin:.4f}   goals {rgoals:.4f} ({ng:.0f}/{dg:.0f}, "
                  f"{gbad} disagree)   {len(played)} cells")
    return out


def borrowed_ratio(ratios, field):
    """The offset to use for a season that has no overlap window of its own.

    The **unweighted mean across every overlap season**, with the spread reported as
    what it is: the residual level uncertainty on the backfilled seasons. It is not
    rescaled to make any validation read 1.000 — scaling hides the cause, and the cause
    here is that two xG models genuinely disagree.

    # Why rescale at all

    2022-23's repair already divides by an in-season ratio, so its GW1-15 sits on FPL's
    scale. Leaving the older seasons on Understat's would put two scales inside one
    grid, and the prior for a replayed season is the season before it — so the seam
    would fall inside a cell rather than between cells. Dividing by the mean bounds the
    level error at about half the spread instead of guaranteeing the whole of it.

    # Why the plain mean, and why the residual does not need resolving to 1%

    The four ratios are not a flat series: xG reads 1.031, 1.091, 1.128, 1.104. Three
    estimators are defensible — plain mean 1.0885, band midpoint 1.0795, and the mean of
    the two seasons whose *Understat* level matches the targets 1.061 (see
    `goals_anchor`; the evidence there argues for the low end). They span 2.6%, and
    picking whichever a four-point series happens to favour is the argmax error this
    project has recorded five times. So the plain mean ships: no season is hand-picked
    and no regime model is fitted.

    What licenses leaving a 4-5% residual is the finding this project already has from
    the clean sheet and the bonus term: **a level error shared by every player in a
    season is invisible to an argmax**, because the optimiser consumes an ordering, and
    scaling every player's xG in a season by 1.05 reorders nobody. So the residual is
    the benign kind of error.

    The reason to rescale *at all* is the one place a level shift is not benign: the
    prior for a replayed season is the season before it, so an unrescaled 2021-22 as the
    prior for 2022-23 would put the seam between two scales *inside* a cell. Dividing by
    the mean shrinks that seam from up to 13% to about 5%.

    What rescaling cannot touch, and no rescaling can, is **per-player dispersion** —
    two xG models disagree shot by shot, and the record puts the p90 of the per-player
    ratio at 1.54. That is an ordering error, it is the one that matters, and it is why
    a backfilled season is a noisier cell rather than an equivalent one.

    `--offset-xg` / `--offset-xa` exist so a different choice can be re-measured rather
    than argued about.
    """
    vs = [r[field] for r in ratios.values()]
    mean = sum(vs) / len(vs)
    spread = (max(vs) - min(vs)) / mean
    return mean, spread, vs


def goals_anchor(verbose=True):
    """Understat's xG against actual goals, for every season including the targets.

    Not a calibration — a season's finishing over- or under-performance is real, and
    forcing xG to track goals would delete the quantity. It is the only *level*
    reference the backfilled seasons have, so it is what says whether Understat's own
    scale in 2019-21 is the same scale as in the seasons the offset is borrowed from.
    """
    out = {}
    allseasons = {s: SEASONS[s] for s in SEASONS}
    for s, spec in CALIBRATION_SEASONS.items():
        allseasons.setdefault(s, dict(ulabel=spec["ulabel"], renumber=False))
    for season in sorted(allseasons):
        spec = allseasons[season]
        el2u, el2min, _, _ = crosswalk(season, "code")
        pop = {el for el, m in el2min.items() if m >= CALIBRATION_MIN_MINUTES}
        minutes, goals, _, _, _ = archive_cells(season, spec.get("renumber", False))
        u_xg, _, u_min, _ = harvest(season, spec["ulabel"], spec.get("renumber", False),
                                    el2u, minutes, only=pop, verbose=False)
        cells = [k for k in u_min if minutes.get(k, 0) > 0]
        ux = sum(u_xg[k] for k in cells)
        g = sum(goals.get(k, 0) for k in cells) or 1.0
        out[season] = ux / g
        if verbose:
            print(f"  {season}: understat xG {ux:7.1f} against {g:5.0f} actual goals "
                  f"= {ux / g:.4f}")
    return out


def report_crosswalk(season, which="code"):
    el2u, el2min, el2name, cov = crosswalk(season, which)
    print(f"crosswalk {season} ({which}): {cov['mapped']} of {cov['players']} players "
          f"mapped, {cov['minutes_share']:.4f} of league minutes, "
          f"{cov['regulars_mapped']}/{cov['regulars']} players with 900+ minutes")
    if which == "code":
        bundled = check_against_bundled(season, el2u)
        if bundled:
            agree, disagree, bad = bundled
            print(f"  vs the archive's bundled id_dict: {agree} agree, {disagree} "
                  f"disagree" + (f" {bad[:5]}" if bad else ""))
        else:
            print("  the archive bundles no id_dict for this season")
    return el2u, el2min, el2name


def compare_crosswalks():
    """What the wider crosswalk would be worth, per season, without applying it.

    Reported rather than adopted for 2022-23: see the note on `xwalk` in SEASONS.
    """
    for season in sorted(SEASONS):
        bundled = bundled_crosswalk(season)
        code = code_crosswalk(season)
        el2min = {int(r["id"]): float(r.get("minutes") or 0)
                  for r in archive_csv(season, "players_raw.csv")}
        if bundled is None:
            print(f"  {season}: no bundled id_dict; code crosswalk reaches "
                  f"{len(code)} of {len(el2min)}")
            continue
        bundled = {el: u for el, u in bundled.items() if el in el2min}
        extra = set(code) - set(bundled)
        print(f"  {season}: bundled {len(bundled)}, code {len(code)}, "
              f"{len(extra)} players the code crosswalk adds, carrying "
              f"{sum(el2min[e] for e in extra):.0f} minutes "
              f"({sum(el2min[e] for e in extra) / sum(el2min.values()):.2%} of the "
              f"league)")


def repair(season, out_path, write=True, force_xg=None, force_xa=None):
    spec = SEASONS[season]
    print(f"\n=== {season}  GW{spec['first']}-{spec['last']}"
          f"{'  (gameweeks 39-47 renumbered to 30-38)' if spec['renumber'] else ''}")
    el2u, el2min, el2name = report_crosswalk(season, spec["xwalk"])
    minutes, goals, assists, fxg, fxa = archive_cells(season, spec["renumber"])
    weekly_xg, weekly_xa = sum(fxg.values()), sum(fxa.values())
    raw = archive_csv(season, "players_raw.csv")
    agg_xg = sum(float(r.get("expected_goals") or 0) for r in raw)
    agg_xa = sum(float(r.get("expected_assists") or 0) for r in raw)
    print(f"  archive has weekly xG {weekly_xg:.1f} / aggregate {agg_xg:.1f}, "
          f"weekly xA {weekly_xa:.1f} / aggregate {agg_xa:.1f}")

    u_xg, u_xa, u_min, u_goals = harvest(
        season, spec["ulabel"], spec["renumber"], el2u, minutes)

    # The join check that does not select for easy cases. A wrong id, or a date mapped
    # to the wrong gameweek, produces neither the right minutes nor the right goals.
    #
    # Minutes are read over every joined cell. Goals are read over the cells with no own
    # goal in them — see own_goal_cells — and reported with a *mismatched-cell count*
    # beside the ratio, because the ratio cancels compensating errors and
    # count_goal_mismatches is the sharper of the two.
    cells = [k for k in u_min if minutes.get(k, 0) > 0]
    rmin, nm, dm = ratio_over(cells, u_min, minutes)
    gcells = goals_anchor_cells(season, spec["renumber"], u_min, minutes)
    rg, ng, dg = ratio_over(gcells, u_goals, goals)
    gbad = count_goal_mismatches(gcells, u_goals, goals)
    print(f"  join check on {len(cells)} cells: understat/FPL minutes {rmin:.4f} "
          f"({nm:.0f}/{dm:.0f})")
    print(f"  goals anchor on {len(gcells)} cells with no own goal in them: "
          f"{rg:.4f} ({ng:.0f}/{dg:.0f}), {gbad} cell(s) disagree")
    if gbad:
        print("    A DISAGREEING CELL IS WORTH READING even when the ratio is 1.0000, "
              "because\n    one cell over and one cell short cancel — which is exactly "
              "what a mis-mapped\n    date looks like. Known and benign: 2019-20 GW10, "
              "where Understat gives\n    De Bruyne a goal FPL gives to David Silva.")

    if spec["inseason"]:
        cxg = offset_cells(u_xg, fxg, spec["last"])
        cxa = offset_cells(u_xa, fxa, spec["last"])
        rxg, nxg, dxg = ratio_over(cxg, u_xg, fxg)
        rxa, nxa, dxa = ratio_over(cxa, u_xa, fxa)
        print(f"  offset fitted IN SEASON on GW{spec['last'] + 1}-38: "
              f"xG {rxg:.4f} ({nxg:.1f}/{dxg:.1f}) on {len(cxg)} cells, "
              f"xA {rxa:.4f} ({nxa:.1f}/{dxa:.1f}) on {len(cxa)} cells")
        src = "in-season"
    else:
        print("  this season carries no FPL xG column at all, so the offset cannot be "
              "fitted in season and is borrowed:")
        ratios = measure_ratios(verbose=True)
        rxg, sxg, _ = borrowed_ratio(ratios, "xg")
        rxa, sxa, _ = borrowed_ratio(ratios, "xa")
        print(f"  borrowed offset: xG {rxg:.4f} (spread {sxg:.1%} across "
              f"{len(ratios)} seasons), xA {rxa:.4f} (spread {sxa:.1%})")
        print("  READ HALF THAT SPREAD AS THE RESIDUAL LEVEL ERROR on this season's xG "
              "and xA. It is not removable from this archive.")
        src = "borrowed"

    if force_xg is not None:
        print(f"  --offset-xg overrides {rxg:.4f} with {force_xg:.4f}")
        rxg = force_xg
    if force_xa is not None:
        print(f"  --offset-xa overrides {rxa:.4f} with {force_xa:.4f}")
        rxa = force_xa

    if not write:
        return 0
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    n = 0
    add_xg = add_xa = 0.0
    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["element", "GW", "expected_goals", "expected_assists"])
        for (el, gw) in sorted(set(u_xg) | set(u_xa)):
            if gw < spec["first"] or gw > spec["last"]:
                continue
            xg, xa = u_xg[(el, gw)] / rxg, u_xa[(el, gw)] / rxa
            if xg == 0 and xa == 0:
                continue
            w.writerow([el, gw, f"{xg:.6f}", f"{xa:.6f}"])
            n += 1
            add_xg += xg
            add_xa += xa
    print(f"  wrote {out_path}: {n} rows, +{add_xg:.1f} xG +{add_xa:.1f} xA "
          f"({src} offset)")
    write_meta(season, out_path, spec, src, rxg, rxa, n, add_xg, add_xa, rmin, rg,
               len(gcells), gbad)
    return validate(season, out_path)


def transport(season, out_path):
    """Emit Understat xG per (element, gameweek) for a season that HAS an FPL xG column.

    This is the input to the **transport test**, and the whole point of it is that this
    season does not need repairing. `xgcrepair.go`'s validation of the expected-goals-
    conceded chain — pooled 0.9994, ever-present 1.0088, partial minutes 0.9853 — is
    scored on seasons whose per-player xG is **FPL's own**, while the chain only ever
    *runs* on seasons whose xG is **Understat's, rescaled**. So the entire validation is
    off the population it licenses, which `xgcrepair.go:179` records as untested rather
    than untestable. Writing Understat xG for a season that also carries the real xGC
    column lets the same shipped chain be scored on both inputs against one truth.

    # Two decisions that make this the honest arm rather than a flattering one

    **The offset is borrowed leave-one-out.** A season with no FPL xG of its own takes
    the unweighted mean of the seasons that have both — see `borrowed_ratio` — and the
    target season is by construction not one of them. Including it here would fit the
    level on the season being scored, which is the in-sample failure this record already
    carries once in the coarse team-strength map. The four ratios span 1.031 to 1.128,
    so leaving one out is worth up to about 2% of level and is not a formality.

    **A player the crosswalk does not reach gets nothing, not his FPL figure.** That is
    what happens on the seasons the repair actually runs on: an unmapped player's xG is
    simply absent, so his club's gameweek total is short and the *opponent's*
    reconstructed xGC is biased low. Substituting FPL's value for him here would measure
    a chain nobody runs and hide the coverage loss, which is one of the two mechanisms
    `xgcrepair.go` names as genuinely threatening transport. The crosswalk coverage is
    printed beside the file for exactly that reason.
    """
    spec = CALIBRATION_SEASONS[season]
    print(f"\n=== {season}  (transport arm: Understat xG for a season that has FPL's)")
    el2u, el2min, el2name = report_crosswalk(season, "code")
    minutes, goals, assists, fxg, fxa = archive_cells(season, False)
    u_xg, u_xa, u_min, u_goals = harvest(season, spec["ulabel"], False, el2u, minutes)

    # The join check, on the same terms the repair reports it. A transport arm built on
    # a broken join would read as a transport failure, and the two must not be confused.
    cells = [k for k in u_min if minutes.get(k, 0) > 0]
    rmin, nm, dm = ratio_over(cells, u_min, minutes)
    gcells = goals_anchor_cells(season, False, u_min, minutes)
    rg, ng, dg = ratio_over(gcells, u_goals, goals)
    gbad = count_goal_mismatches(gcells, u_goals, goals)
    print(f"  join check on {len(cells)} cells: understat/FPL minutes {rmin:.4f}")
    print(f"  goals anchor on {len(gcells)} cells: {rg:.4f}, {gbad} cell(s) disagree")

    ratios = measure_ratios(verbose=False)
    loo = {k: v for k, v in ratios.items() if k != season}
    rxg_loo, sxg, _ = borrowed_ratio(loo, "xg")
    rxg_all, _, _ = borrowed_ratio(ratios, "xg")
    own = ratio_over(offset_cells(u_xg, fxg, spec["overlap_first"] - 1), u_xg, fxg)[0]
    print(f"  offset: borrowed LEAVE-ONE-OUT {rxg_loo:.4f} over {len(loo)} seasons "
          f"(all four {rxg_all:.4f}); this season's OWN in-season ratio is {own:.4f}, "
          f"which is what the borrow gets wrong by {100 * (rxg_loo / own - 1):+.1f}%")

    # The coverage number that decides how to read everything downstream: how much of
    # FPL's own xG mass the mapped players carry, over the window the truth exists on.
    win = [k for k in fxg if k[1] >= spec["overlap_first"]]
    have = sum(fxg[k] for k in win if k in u_xg)
    allm = sum(fxg[k] for k in win)
    print(f"  crosswalk carries {have:.1f} of {allm:.1f} FPL xG on GW"
          f"{spec['overlap_first']}-38 = {have / allm:.4f} of the mass")

    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    n, tot = 0, 0.0
    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["element", "GW", "expected_goals"])
        for (el, gw) in sorted(u_xg):
            xg = u_xg[(el, gw)] / rxg_loo
            if xg == 0:
                continue
            w.writerow([el, gw, f"{xg:.6f}"])
            n += 1
            tot += xg
    print(f"  wrote {out_path}: {n} rows, {tot:.1f} xG")
    return 0


def write_meta(season, out_path, spec, src, rxg, rxa, rows, add_xg, add_xa, rmin, rg,
               gcells, gbad):
    """Record what was divided by, beside the data it was divided into.

    The offset is the single most load-bearing assumption in a backfilled season and it
    would otherwise exist only in this script's output, which nothing keeps. Written as
    a sidecar the Go loader reads and reports, so a snapshot naming a backfilled season
    also names the offset that season's xG is on and whether it was fitted in season or
    borrowed.

    It also carries the window and the renumber flag, which the Go side has its own copy
    of. That is deliberate and it is *checked* rather than watched: `applyXGRepair`
    refuses a repair whose meta disagrees with its own table. A duplicate that is
    verified every load is a pipeline test; an unverified one is the bug this project
    keeps rediscovering.
    """
    meta = dict(
        season=season, first_gw=spec["first"], last_gw=spec["last"],
        renumbered=bool(spec["renumber"]), crosswalk=spec["xwalk"],
        offset_source=src, offset_xg=round(rxg, 6), offset_xa=round(rxa, 6),
        rows=rows, sum_xg=round(add_xg, 4), sum_xa=round(add_xa, 4),
        join_minutes_ratio=round(rmin, 6), join_goals_ratio=round(rg, 6),
        # The cell count the goals ratio was read on, and how many of those cells the
        # two sources disagree on. The count travels with the ratio because the ratio
        # cancels compensating errors — see count_goal_mismatches — so a reader given
        # only 1.0000 would be told less than this script knows.
        join_goal_cells=gcells, join_goal_mismatch=gbad,
    )
    path = out_path[:-len(".csv")] + ".meta.json" if out_path.endswith(".csv") \
        else out_path + ".meta.json"
    with open(path, "w", encoding="utf-8") as f:
        json.dump(meta, f, indent=1, sort_keys=True)
        f.write("\n")
    print(f"  wrote {path}")


def validate(season, path):
    """Validate a written repair against whatever this season can check it with.

    2022-23 has the strong check: the season aggregate, which the archive carries
    complete and the repair never saw. The three older seasons have no aggregate at all,
    so the only checks are the join check above and the plausibility of the level
    against actual goals — which is weaker, and is reported as weaker rather than
    dressed up.
    """
    spec = SEASONS[season]
    if not os.path.exists(path):
        print(f"  no repair at {path}", file=sys.stderr)
        return 1
    add_xg = add_xa = 0.0
    rows = 0
    with open(path, encoding="utf-8") as f:
        for r in csv.DictReader(f):
            add_xg += float(r["expected_goals"])
            add_xa += float(r["expected_assists"])
            rows += 1
    minutes, goals, assists, fxg, fxa = archive_cells(season, spec["renumber"])
    wx, wa = sum(fxg.values()), sum(fxa.values())
    raw = archive_csv(season, "players_raw.csv")
    agg_xg = sum(float(r.get("expected_goals") or 0) for r in raw)
    agg_xa = sum(float(r.get("expected_assists") or 0) for r in raw)
    print(f"\n  validation, {rows} repair rows:")
    if agg_xg > 0:
        print(f"    xG  {wx:.1f} + {add_xg:.1f} = {wx + add_xg:.1f} against aggregate "
              f"{agg_xg:.1f}  ratio {(wx + add_xg) / agg_xg:.4f}")
        print(f"    xA  {wa:.1f} + {add_xa:.1f} = {wa + add_xa:.1f} against aggregate "
              f"{agg_xa:.1f}  ratio {(wa + add_xa) / agg_xa:.4f}")
        print("    1.000 would be exact. Read a shortfall as coverage — a player")
        print("    absent from the crosswalk contributes nothing — and an excess as")
        print("    double counting or a mis-mapped date. Neither is fixed by scaling.")
    else:
        tg, ta = sum(goals.values()), sum(assists.values())
        rg, ra = add_xg / tg, add_xa / ta
        print("    THIS SEASON HAS NO FPL AGGREGATE TO CHECK AGAINST — the column is")
        print("    absent, which is why it needed backfilling. The weaker check is the")
        print("    level against outcomes, beside what FPL's OWN xG and xA produce in")
        print("    the seasons that have them:")
        print(f"    xG/goals   {rg:.4f}   ({add_xg:.1f} against {tg:.0f} goals)")
        print(f"    xA/assists {ra:.4f}   ({add_xa:.1f} against {ta:.0f} assists)")
        ref = fpl_level_reference()
        gs = [v[0] for v in ref.values()]
        as_ = [v[1] for v in ref.values()]
        for s, (a, b) in sorted(ref.items()):
            print(f"      FPL's own {s}: xG/goals {a:.4f}, xA/assists {b:.4f}")
        print(f"    FPL's own range: xG/goals [{min(gs):.4f}, {max(gs):.4f}], "
              f"xA/assists [{min(as_):.4f}, {max(as_):.4f}]")
        print(f"    this season sits {rg / (sum(gs) / len(gs)):.3f} of FPL's mean "
              f"xG/goals and {ra / (sum(as_) / len(as_)):.3f} of its mean xA/assists.")
        print("    Deliberately NOT a pass/fail. FPL's own range is 6 points wide")
        print("    because finishing performance is real football, so landing outside")
        print("    it is not a defect and landing inside it is not a validation — with")
        print("    three seasons unobserved there is no way to know what FPL would have")
        print("    said. Read it as: is the level in the right neighbourhood, and which")
        print("    way would a different offset move it. The check that DOES validate")
        print("    the join is the goals ratio above, where both sources count an exact")
        print("    integer and must agree.")
    return 0


def fpl_level_reference():
    """FPL's own xG/goals and xA/assists per season, from the archive alone.

    The comparison a backfilled season's level has to be read against. Pure archive
    arithmetic — no Understat, no crosswalk — so it costs nothing and cannot fail for a
    reason the repair is responsible for.
    """
    out = {}
    for season in sorted(CALIBRATION_SEASONS):
        raw = archive_csv(season, "players_raw.csv")
        xg = sum(float(r.get("expected_goals") or 0) for r in raw)
        xa = sum(float(r.get("expected_assists") or 0) for r in raw)
        g = sum(float(r.get("goals_scored") or 0) for r in raw) or 1.0
        a = sum(float(r.get("assists") or 0) for r in raw) or 1.0
        out[season] = (xg / g, xa / a)
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--season", choices=sorted(SEASONS))
    ap.add_argument("--all", action="store_true", help="every repairable season")
    ap.add_argument("--calibrate", action="store_true",
                    help="report the provider ratios and the crosswalk, write nothing")
    ap.add_argument("--compare-crosswalks", action="store_true",
                    help="what the wider crosswalk would add, without applying it")
    ap.add_argument("--check", action="store_true", help="validate an existing repair")
    ap.add_argument("--transport", action="store_true",
                    help="write Understat xG for the seasons that carry FPL's own, as "
                         "the arm-B input to the xGC transport test. Writes to "
                         "stats/out/, NEVER to the embedded repairdata: these seasons "
                         "need no repair and a file there would be loaded as one")
    ap.add_argument("--offset-xg", type=float, default=None,
                    help="override the provider offset for xG, to re-measure a "
                         "different choice of estimator without editing this script")
    ap.add_argument("--offset-xa", type=float, default=None)
    ap.add_argument("--out", default=None)
    args = ap.parse_args()

    if args.compare_crosswalks:
        print("bundled id_dict against the code-based crosswalk, per season:")
        compare_crosswalks()
        return 0

    if args.calibrate:
        print("provider offset, understat/FPL, on every season carrying both sources")
        print(f"(players with {CALIBRATION_MIN_MINUTES}+ minutes)")
        ratios = measure_ratios()
        for field in ("xg", "xa"):
            mean, spread, vs = borrowed_ratio(ratios, field)
            print(f"\n  {field}: mean {mean:.4f}, range "
                  f"{min(vs):.4f}-{max(vs):.4f}, spread {spread:.1%} of the mean")
        print("\nThe spread is why this is not a constant. A season with no FPL xG of")
        print("its own borrows the mean and carries half the spread as level")
        print("uncertainty.")
        print("\nunderstat xG against ACTUAL GOALS — the only level reference the")
        print("backfilled seasons have, since they carry no FPL xG at all. Not a")
        print("calibration: a season's finishing performance is real and moves it.")
        goals_anchor()
        print("\ncrosswalk coverage per season:")
        for s in sorted(set(SEASONS) | set(CALIBRATION_SEASONS)):
            report_crosswalk(s, "code")
        return 0

    if args.transport:
        rc = 0
        for s_ in sorted(CALIBRATION_SEASONS):
            out = os.path.join("stats", "out", f"transport-{s_}-xg.csv")
            rc |= transport(s_, out)
        return rc

    seasons = sorted(SEASONS) if args.all else ([args.season] if args.season else [])
    if not seasons:
        ap.error("give --season, --all, --calibrate or --transport")
    rc = 0
    for s in seasons:
        out = args.out or os.path.join(OUTDIR, f"{s}-xg.csv")
        if args.check:
            report_crosswalk(s, SEASONS[s]["xwalk"])
            rc |= validate(s, out)
        else:
            rc |= repair(s, out, force_xg=args.offset_xg, force_xa=args.offset_xa)
    return rc


if __name__ == "__main__":
    sys.exit(main() or 0)
