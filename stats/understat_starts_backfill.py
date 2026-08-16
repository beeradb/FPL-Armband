"""Recover the real starting elevens from Understat, for the seasons the archive lost.

    python3 stats/understat_starts_backfill.py --seasons 2019-20,2020-21,2021-22,2022-23
    python3 stats/understat_starts_backfill.py --validate          # score against truth

# Why this exists

`merged_gw.csv` carries `starts` as zero for all of 2021-22 and for 2022-23 through
GW15, and does not carry the column at all before 2022-23. `reconstructStarts` fills
that by rank — within a club-gameweek the eleven with the most minutes started — which
is exact in *count* and misclassifies **2.36% of starter slots**, with a documented
**3:1 bias** toward crediting a returning player with a start he did not make.

The truth was recoverable the whole time. Understat's `getPlayerData` returns a
`matches` array whose rows carry `position`, and **`Sub` is an explicit value**. A start
is exactly `position != "Sub"`.

**This is not a new source and not a new fetch path.** `understat_xg_backfill.py`
already requests that endpoint per player, already caches the whole JSON under
`~/.cache/fplagent/understat`, and already reads `time` out of the very rows that carry
`position` — it simply never wrote it out. This script imports that module and reuses
its cache, its element-to-Understat crosswalk, its date-to-gameweek map and its foreign
-match filter, so the only new thing here is which field gets read.

# What it fixes that the rank rule cannot

**The role bias.** The rank rule's error is systematic, not symmetric: a nailed starter
withdrawn at half time and a fringe player eased back on at half time both record 45
minutes and tie, and the tie-break promotes the wrong one. Recorded starts have no tie.

**Doubles.** `reconstructStarts` deliberately declines a club-gameweek with two
fixtures, because a gameweek total cannot say whether one start or two were made, and
leaves the recorded zero in place. Understat is *per match*, so two starts in one
gameweek are counted directly. 2022-23 alone has 42 double team-gameweeks.

# The check, which is free

Seasons from 2023-24 record real starts, and 2022-23 records them from GW16, so the
derived starts can be scored against recorded truth. On 22,088 starter slots the
harvest misclassifies **0.000%**.

⚠️ **Do not compare that with the 2.36% recorded against `reconstructStarts`: they are
different statistics on different populations.** The 2.36% is one-sided and excludes
double club-gameweeks, which the rank rule declines to touch. Scored on these same
cells with the same two-sided statistic `validate()` computes, the rank rule
misclassifies **11.74%** — so the harvest wins by about five times more than the naive
comparison suggests, and the naive comparison was still wrong. Quote 11.74% when
comparing the two methods and 2.36% only as the rank rule's own recorded figure.

# What the exact zero is, and is not, evidence for

It validates the **plumbing, not the football**. FPL's `starts` and Understat's
`position` both descend from official Premier League lineup data, so agreement is what
two views of one feed should produce; what the check rules out is a broken crosswalk, a
mis-mapped date, a leaked foreign fixture or a mishandled double. It cannot rule out an
error the two sources share.

It is also **conditioned on the archive**: `harvest_starts` drops any Understat row
where the archive records no minutes, so the one class of genuine disagreement —
Understat says he started, the archive has no minutes — is filtered out before the
comparison rather than counted as an error.

**The better evidence is about the seasons being repaired**, and `club_gameweek_audit`
prints it on every run: each club-gameweek against 11 x its fixtures. Currently 716/716
exact in 2020-21, 272/272 in 2022-23's window, and 696 exact with 3 short in 2021-22 —
**zero over-credited anywhere**.

# Two honest limits

**Coverage is not uniform and must not be pooled.** Understat carries only players who
appear in its match rosters. Fringe players with few minutes are both the likeliest gap
*and* the population the rank rule gets wrong, so a coverage figure averaged over
everyone would hide exactly the failure that matters. `--validate` reports coverage by
minutes band for that reason — and it is not uniformly complete: 2023-24's 1-29 band is
2,454 of 2,460.

⚠️ **A missing row is a gap, and the flag on it is only honest where the harvest missed
the WHOLE club-gameweek.** `reconstructStarts` skips a club-gameweek once any start is
recorded there, so a player the harvest missed inside a club-gameweek it otherwise
reached keeps `Starts = 0` with `StartsReconstructed` false — indistinguishable from a
recorded substitute. Measured: 3 rows in 2021-22, 0 in the other four seasons. An
earlier version of this docstring claimed the fallback covered that case; it does not.
"""

import argparse
import csv
import os
import sys
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import understat_xg_backfill as ux  # noqa: E402

# Seasons whose starts are missing or partial, with the gameweek window to write.
# 2022-23 records its own starts from GW16, so only GW1-15 is filled; the others have
# no usable start data at all. The windows are deliberately narrower than "everything
# Understat has" so this can never overwrite a season's own recorded truth.
TARGETS = {
    "2018-19": dict(ulabel="2018", first=1, last=38, xwalk="code"),
    "2019-20": dict(ulabel="2019", first=1, last=38, xwalk="code", renumber=True),
    "2020-21": dict(ulabel="2020", first=1, last=38, xwalk="code"),
    "2021-22": dict(ulabel="2021", first=1, last=38, xwalk="code"),
    "2022-23": dict(ulabel="2022", first=1, last=15, xwalk="bundled"),
}

# Seasons that record real starts, used only to score this method against truth.
VALIDATE = {
    "2022-23": dict(ulabel="2022", first=16, last=38, xwalk="bundled"),
    "2023-24": dict(ulabel="2023", first=1, last=38, xwalk="code"),
    "2024-25": dict(ulabel="2024", first=1, last=38, xwalk="code"),
}


def archive_starts(season, renumber):
    """Per (element, gameweek): recorded starts, minutes, and matches played.

    `played` counts the archive's own rows with minutes — one per fixture, so it is
    the number of Premier League matches this player actually appeared in that
    gameweek. It is what bounds the harvest: see the per-match leak in
    `harvest_starts`.
    """
    starts, minutes = defaultdict(float), defaultdict(float)
    played = defaultdict(int)
    rows = ux.archive_csv(season, "gws/merged_gw.csv")
    key = "GW" if "GW" in rows[0] else "round"
    for r in rows:
        try:
            el = int(r["element"])
            gw = ux.renumber_gw(int(r[key]), renumber)
        except (KeyError, ValueError):
            continue
        if gw is None:
            continue
        m = float(r.get("minutes") or 0)
        minutes[(el, gw)] += m
        if m > 0:
            played[(el, gw)] += 1
        if r.get("starts") not in (None, ""):
            starts[(el, gw)] += float(r.get("starts") or 0)
    return starts, minutes, played


def harvest_starts(season, spec, verbose=True):
    """Per (element, gameweek): starts counted from Understat's own position field."""
    renumber = spec.get("renumber", False)
    date_gw, _ = ux.gameweek_by_date(season, renumber)
    el2u, _, _, cov = ux.crosswalk(season, spec.get("xwalk", "code"))
    if verbose:
        print(f"  crosswalk: {cov['mapped']}/{cov['players']} players, "
              f"{cov['regulars_mapped']}/{cov['regulars']} regulars, "
              f"{100*cov['minutes_share']:.1f}% of minutes")
    _, minutes, played = archive_starts(season, renumber)

    started = defaultdict(int)      # (el, gw) -> matches begun
    appeared = defaultdict(int)     # (el, gw) -> matches with any Understat row
    failed = foreign = unmapped = 0

    items = sorted(el2u.items())
    for i, (el, uid) in enumerate(items):
        try:
            d = ux.player_data(uid)
        except Exception as e:                                   # noqa: BLE001
            failed += 1
            print(f"  element {el} (understat {uid}): {e}", file=sys.stderr)
            continue
        for mt in d.get("matches", []):
            if mt.get("season") != spec["ulabel"]:
                continue
            gw = date_gw.get((mt.get("date") or "")[:10])
            if gw is None:
                unmapped += 1
                continue
            # The same foreign-match filter the xG harvest uses: if the archive
            # records no minutes for this player in this gameweek then he played no
            # Premier League football in it, so whatever Understat has for that date
            # belongs to another league. Understat labels every league's 2022-23 as
            # "2022", so this filter is load-bearing rather than defensive.
            if minutes[(el, gw)] <= 0:
                foreign += 1
                continue
            appeared[(el, gw)] += 1
            if (mt.get("position") or "") != "Sub":
                started[(el, gw)] += 1
        if verbose and (i + 1) % 200 == 0:
            print(f"  {i + 1}/{len(items)} players")
    # The inherited filter is per GAMEWEEK, not per match, and that leaks.
    #
    # It rejects an Understat row when the archive records no minutes for that
    # (element, gameweek). A player who moved mid-season can have Premier League
    # minutes *and* a foreign fixture inside the same gameweek window, and his
    # foreign match is then admitted — counted as a start whenever `position` is not
    # `Sub`. Measured on the committed harvest before this guard: Anthony Martial's
    # Osasuna-Sevilla match and Wout Weghorst's Leipzig-Wolfsburg match both landed
    # in 2021-22 GW23, crediting Man Utd with a twelfth starter and Weghorst with two
    # starts in a gameweek he played once.
    #
    # The archive says how many matches he actually played, so the leak is detectable
    # exactly: more Understat rows than archive appearances. It is NOT correctable
    # here — with two same-window matches there is nothing in this data to say which
    # is the domestic one — so the cell is dropped rather than guessed at, and
    # `reconstructStarts` fills it downstream. An honest gap beats a confident guess,
    # which is the lesson the doubles-counting bug taught on this same season.
    leaked = 0
    for k in list(appeared):
        if appeared[k] > played[k]:
            leaked += 1
            del appeared[k]
            started.pop(k, None)
    if verbose:
        print(f"  harvest: {len(items)} players, {failed} failed, {unmapped} unmapped "
              f"dates, {foreign} rejected as not Premier League, {leaked} dropped as "
              f"ambiguous (more Understat matches than archive appearances)")
    return started, appeared


def club_gameweek_audit(season, spec, started):
    """Check each club-gameweek against 11 x its fixtures — the assignment check.

    A season total is a **net-leakage** check and cannot see an assignment error: an
    over-credit in one club-gameweek and an under-credit in another cancel in it
    exactly. That is not hypothetical. Before the per-match guard below existed, the
    2021-22 harvest was wrong in four club-gameweeks by +1, +1, -1, -1 — Burnley and
    Man Utd over-credited in GW23 by two foreign fixtures, Watford under-credited in
    GW11 and GW12 by a crosswalk miss — and summed to exactly 8,360, which is why an
    equality test on the season total passed while the harvest was wrong.

    Reported rather than raised: an under-credit is a gap `reconstructStarts` fills
    downstream and is expected wherever the crosswalk misses a player. An
    **over**-credit is the harvest asserting a start nobody made, and is the number to
    watch.
    """
    renumber = spec.get("renumber", False)
    rows = ux.archive_csv(season, "gws/merged_gw.csv")
    key = "GW" if "GW" in rows[0] else "round"
    team_of, fixtures = {}, defaultdict(set)
    for r in rows:
        try:
            el, gw = int(r["element"]), ux.renumber_gw(int(r[key]), renumber)
        except (KeyError, ValueError):
            continue
        if gw is None:
            continue
        t = (r.get("team") or r.get("team_x") or "").strip()
        if not t:
            continue
        team_of[(el, gw)] = t
        if r.get("fixture"):
            fixtures[(t, gw)].add(r["fixture"])

    got = defaultdict(int)
    for (el, gw), n in started.items():
        t = team_of.get((el, gw))
        if t:
            got[(t, gw)] += n

    over = under = ok = 0
    for (t, gw), fx in sorted(fixtures.items()):
        if gw < spec["first"] or gw > spec["last"]:
            continue
        want = 11 * len(fx)
        have = got.get((t, gw), 0)
        if have == want:
            ok += 1
        elif have > want:
            over += 1
            print(f"    OVER  {t} GW{gw}: {have} against {want}")
        else:
            under += 1
    print(f"  club-gameweeks: {ok} exact, {under} short (gaps the reconstruction "
          f"fills), {over} OVER-credited")
    return over


def write(season, spec, started, appeared, out_path):
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    n = tot = 0
    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["element", "GW", "starts", "understat_matches"])
        for (el, gw) in sorted(appeared):
            if gw < spec["first"] or gw > spec["last"]:
                continue
            w.writerow([el, gw, started[(el, gw)], appeared[(el, gw)]])
            n += 1
            tot += started[(el, gw)]
    print(f"  wrote {out_path}: {n} rows, {tot:.0f} starts")
    return n


def validate(season, spec):
    """Score derived starts against the archive's recorded ones.

    Two statistics, because they answer different questions. **Slot error** is the one
    comparable with `reconstructStarts`' recorded 2.36%: of all recorded starter slots,
    how many does this method miss. **Coverage by minutes band** is the one that decides
    whether the method is safe for the population it is meant to fix -- a high pooled
    coverage carried by ever-presents would be no use at all.
    """
    renumber = spec.get("renumber", False)
    truth, minutes, _ = archive_starts(season, renumber)
    started, appeared = harvest_starts(season, spec, verbose=False)

    lo, hi = spec["first"], spec["last"]
    cells = [(k, v) for k, v in truth.items() if lo <= k[1] <= hi and minutes[k] > 0]
    slots = sum(v for _, v in cells)
    if slots == 0:
        print(f"  {season}: no recorded starts in GW{lo}-{hi}")
        return

    # `started` is a defaultdict, so `.get(k)` returns None for a player who appeared
    # and began no match. Comparing that to a recorded 0.0 counts a disagreement that
    # contributes nothing to the error — which is how an earlier version of this
    # printed "slot error 0.00%" and "2919 cells disagreeing" on the same run, two
    # numbers that cannot both be true. Default to 0 and compare numerically.
    missed = wrong = covered = 0
    over = under = 0
    bands = defaultdict(lambda: [0, 0])
    for k, t in cells:
        m = minutes[k]
        b = "1-29" if m < 30 else "30-59" if m < 60 else "60-89" if m < 90 else "90+"
        bands[b][1] += 1
        if k in appeared:
            covered += 1
            bands[b][0] += 1
            got = float(started.get(k, 0))
            if got != float(t):
                wrong += 1
                missed += abs(got - float(t))
                if got > t:
                    over += 1
                else:
                    under += 1
        else:
            missed += t
            under += 1

    print(f"\n{season} GW{lo}-{hi}: {len(cells)} player-gameweeks, {slots:.0f} "
          f"recorded starter slots")
    print(f"  coverage           {covered}/{len(cells)} "
          f"({100.0*covered/len(cells):.1f}%)")
    print(f"  slot error         {100.0*missed/slots:.3f}%  ({missed:.0f} of "
          f"{slots:.0f} slots)   (reconstructStarts: 2.36%)")
    print(f"  cells disagreeing  {wrong} of {covered} covered "
          f"({over} over-credited, {under} under-credited)")
    print("  coverage by minutes band:")
    for b in ("1-29", "30-59", "60-89", "90+"):
        c, t = bands[b]
        if t:
            print(f"    {b:6} {c:6}/{t:<6} ({100.0*c/t:5.1f}%)")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--seasons", default=",".join(TARGETS))
    ap.add_argument("--validate", action="store_true")
    ap.add_argument("--out", default=None)
    a = ap.parse_args()

    if a.validate:
        for s, spec in VALIDATE.items():
            validate(s, spec)
        return 0

    for s in [x.strip() for x in a.seasons.split(",") if x.strip()]:
        spec = TARGETS.get(s)
        if not spec:
            print(f"{s}: not a target season", file=sys.stderr)
            continue
        print(f"{s}:")
        started, appeared = harvest_starts(s, spec)
        club_gameweek_audit(s, spec, started)
        out = a.out or os.path.join(ux.OUTDIR, f"{s}-starts.csv")
        write(s, spec, started, appeared, out)
    return 0


if __name__ == "__main__":
    sys.exit(main() or 0)
