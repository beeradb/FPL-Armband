#!/usr/bin/env python3
"""Rebuild `def` from the opponent's strength AS IT STOOD AT EACH CUTOFF.

# Why this exists

`stats/cs_calibration.R`'s two-channel decomposition fits

    -ln P(clean sheet) = a + b1*XGC90 + b2*XGC90*(def-1)

and banks `b2 = 1.5688` on the native stratum. `def` is
`defenceMultiplier(fixture.Difficulty) * defenceBandAdj(...)`, and with the
shipped `band_strength: 0` the band adjustment is exactly 1, so `def` is a pure
function of the archive's `team_h_difficulty`/`team_a_difficulty`. That column is
the **end-of-season** value: `internal/backtest/season.go` reads one difficulty
per fixture, and `playedFixtures` strips the scoreline and the `Finished` flag
but not the difficulty. So `def` may carry post-cutoff information into `b2`.

The predecessor arm (`defensive-fixture-coefficient-hindsight-gate`) tested this
with an interaction identified off 221 of 1566 rows and came back UNMEASURABLE.
This script does the thing that arm identified as what would actually resolve it:
reconstruct the difficulty each row's engine *would have seen at its own cutoff*,
and hand the whole 1566 rows to a refit.

# The map, which is DECODED rather than assumed

FPL's fixture difficulty is not a free column. Across all six archived seasons
and both venues, 4560 of 4560 fixture-sides, it is an exact, monotone, global
step function of the opponent's **fine** `strength_overall_*` rating:

    the difficulty a club faces AT HOME  = step(opponent's strength_overall_home)
    the difficulty a club faces AWAY     = step(opponent's strength_overall_away)

    step(s) = 1 if s < 1000, 2 if s < 1105, 3 if s < 1220, 4 if s < 1350, else 5

Read `strength_overall_home` as "this club's strength *as faced by a home side*"
and `strength_overall_away` as "as faced by an away side" — Arsenal 2025-26 sits
at 1305 and 1355, so visiting Arsenal rates 5 and hosting them rates 4, which is
the right way round.

⚠️ **The coarse 1-5 `strength` field does NOT determine difficulty** and was
checked first: it is neither deterministic nor monotone at either venue. Nor is
any of `strength_attack_*` or `strength_defence_*`. The two `strength_overall_*`
fields are the only ones that work, and each works only at its own venue. That is
why this arm keys on the fine fields where the predecessor keyed on the coarse
one.

The thresholds are identified only up to the gaps the end-of-season data leaves
empty — (975, 1000), (1100, 1105), (1215, 1220), (1340, 1350). Three of the four
are 5 or 10 rating points wide. Any *cutoff* strength value landing inside a gap
is counted and reported, because there the reconstruction is a guess.

⚠️ **This is a MECHANISM step, not a measurement.** The per-gameweek captures
carry `bootstrap-static.json.gz` and no fixtures payload, so no archived capture
can show a difficulty. Three of the four things the step needs *are* checkable
and are checked below; the fourth is not:

1. the map reproduces every archived difficulty exactly — 4560 of 4560 — or this
   script aborts;
2. the venue pairing is verified against a **live** payload that carries
   `fixtures.json.gz` beside `bootstrap-static.json.gz`
   (`data/captures/2026-*`), where FPL now publishes `strength_overall_*`
   already on the 1-5 scale and the difficulty is that field *identically*,
   760/760 venue-matched against 456/760 venue-swapped;
3. the GW38 capture agrees with the archive's own `teams.csv` for 20 of 20 clubs
   in all six seasons, so the reconstruction is continuous into the banked
   column at the end of the season rather than a differently-built column;
4. **not checkable here**: that FPL re-published the difficulty when it revised a
   strength mid-season, rather than freezing the difficulty column at season
   start. If it froze it, this reconstruction is wrong.

Nothing downstream may claim to have recovered the true point-in-time difficulty.

# The point-in-time convention

A banked row's `gw` is the gameweek being predicted, so the engine's cutoff is
`gw - 1`. The capture named `GW{k}` is taken before gameweek k's deadline, so
capture `GW{gw}` is the state the engine had at cutoff `gw - 1`.

This script checks that convention against the capture directory's own crawl
timestamp and the archive's kickoff times, and **aborts** if any capture postdates
the first kickoff of the gameweek it is named for. ⚠️ An earlier draft of this
docstring claimed the check and did not implement it — the same failure mode
`AGENTS.md` records against `restFactor`'s reporting-only call site, prose that
reads like confirmation. The guarantee also lives in
`internal/capture/backfilled.go`'s `VerifyPreDeadline`; this is a second,
independent reading of it from the filenames, which is the point.

⚠️ Some capture directories share a crawl timestamp, so a gameweek's "cutoff"
state can be up to two weeks stale: 2020-21 GW02/GW03 and GW06/GW07, and 2024-25
GW08/GW09/GW10. Reported, and S1's lagged arm is the sensitivity for it.

# Usage

    python3 stats/defensive_fixture_pointintime_join.py \
        stats/snapshots/2026-08-15-clean-sheet-2x2/cs_regressor_fixture_path_rows.csv \
        stats/defensive_fixture_pointintime/joined_rows.csv [--archive /tmp/fplarch]

Writes the banked columns unchanged plus `opponent`, `home`, `strength_cut`,
`strength_end`, `difficulty_end`, `difficulty_pit`, `def_pit`, `def_moved` and
`ambiguous`. Prints the liveness report to stderr.

**This script touches no outcome.** `clean_sheet` is carried through unread; every
quantity it computes or prints is a design quantity.
"""
import argparse
import csv
import datetime
import glob
import gzip
import json
import os
import re
import sys
from collections import Counter, defaultdict

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CAPTURES = os.path.join(ROOT, "data", "captures")

# `defenceMultiplier` from internal/analysis/metrics.go, at the shipped
# `defFixtureScale` of 1 where `ladder` is the identity. Restated here ONLY so
# that `def` can be re-derived independently of the Go emitter — a check that
# shares the implementation it is checking proves nothing. It is not a second
# source of truth: if it drifts, the re-derivation below fails loudly.
DEFENCE_MULTIPLIER = {1: 0.70, 2: 0.85, 3: 1.00, 4: 1.20, 5: 1.40}

# The decoded thresholds. Lower bound of each difficulty level, in
# `strength_overall_*` units.
CUTS = (1000, 1105, 1220, 1350)

# The empty intervals the decoding leaves. A cutoff strength inside one of these
# is a value the end-of-season data never took, so the level is a guess.
GAPS = ((975, 1000), (1100, 1105), (1215, 1220), (1340, 1350))


def fail(*msg):
    """Stop rather than emit a partial join.

    A broken join reads downstream exactly like a null — `def_moved` all zero is
    both "the reconstruction changes nothing" and "the key did not match".
    """
    sys.exit("defensive_fixture_pointintime_join: " + "".join(str(m) for m in msg))


def difficulty_from(strength):
    d = 1
    for c in CUTS:
        if strength >= c:
            d += 1
    return d


def captures(season):
    """Fine per-venue strength per team id, per capture gameweek, plus crawl times."""
    out, crawled = {}, {}
    for cap in sorted(glob.glob(f"{CAPTURES}/{season}/GW*/bootstrap-static.json.gz")):
        base = os.path.basename(os.path.dirname(cap))
        m = re.match(r"GW(\d+)-(\d{4}-\d{2}-\d{2}T\d{4}Z)$", base)
        if m is None:
            fail("capture directory does not name a gameweek and a crawl time: ", cap)
        gw = int(m.group(1))
        crawled[gw] = datetime.datetime.strptime(
            m.group(2), "%Y-%m-%dT%H%MZ").replace(tzinfo=datetime.timezone.utc)
        with gzip.open(cap) as fh:
            teams = json.load(fh)["teams"]
        out[gw] = {
            t["id"]: (t["strength_overall_home"], t["strength_overall_away"])
            for t in teams
        }
    if not out:
        fail("no captures for ", season)
    return out, crawled


def check_capture_timing(season, crawled, archive):
    """Every capture must predate the first kickoff of the gameweek it names.

    Stated in the docstring above and, until this was written, not implemented.
    A capture taken after kick-off is not a cutoff state, and reading one would
    put the leak back in by a different door — silently, because the strength
    fields would still parse.
    """
    first = {}
    with open(os.path.join(archive, season, "fixtures.csv")) as fh:
        for r in csv.DictReader(fh):
            if not r["event"] or not r["kickoff_time"]:
                continue
            gw = int(float(r["event"]))
            ko = datetime.datetime.strptime(
                r["kickoff_time"], "%Y-%m-%dT%H:%M:%SZ").replace(
                    tzinfo=datetime.timezone.utc)
            if gw not in first or ko < first[gw]:
                first[gw] = ko
    late = {gw: (crawled[gw].isoformat(), first[gw].isoformat())
            for gw in sorted(crawled) if gw in first and crawled[gw] > first[gw]}
    stale = {gw: (crawled[gw].isoformat(), first[gw].isoformat())
             for gw in sorted(crawled)
             if gw in first and (first[gw] - crawled[gw]).days > 8}
    return late, stale


def archive_season(archive, season):
    """Opponent and end-of-season difficulty per (gameweek, team), plus teams.csv."""
    fx = os.path.join(archive, season, "fixtures.csv")
    tm = os.path.join(archive, season, "teams.csv")
    for p in (fx, tm):
        if not os.path.exists(p):
            fail("missing ", p)

    opp = defaultdict(list)
    with open(fx) as fh:
        for r in csv.DictReader(fh):
            if not r["event"]:
                continue
            gw = int(float(r["event"]))
            h, a = int(r["team_h"]), int(r["team_a"])
            # The difficulty carried is the one the SCORING club faces, matching
            # FixtureBrief in internal/analysis/metrics.go — Home takes
            # team_h_difficulty.
            opp[(gw, h)].append((a, 1, int(float(r["team_h_difficulty"]))))
            opp[(gw, a)].append((h, 0, int(float(r["team_a_difficulty"]))))

    with open(tm) as fh:
        teams = {int(r["id"]): r for r in csv.DictReader(fh)}
    return opp, teams


CANDIDATE_FIELDS = (
    "strength", "strength_overall_home", "strength_overall_away",
    "strength_attack_home", "strength_attack_away",
    "strength_defence_home", "strength_defence_away",
)


def field_screen(archive, seasons):
    """Which strength field can carry a map at all — reported, not assumed.

    A field qualifies only if, pooled over every season, difficulty is a
    DETERMINISTIC and MONOTONE function of it at that venue. Printing the whole
    screen is the point: the fine field is not a choice among several that would
    have worked, it is the only one that works, and the coarse 1-5 `strength`
    the predecessor arm keyed on is not among them.
    """
    obs = {("H", f): set() for f in CANDIDATE_FIELDS}
    obs.update({("A", f): set() for f in CANDIDATE_FIELDS})
    for s in seasons:
        opp, teams = archive_season(archive, s)
        for (_gw, _team), lst in opp.items():
            for o, home, difficulty in lst:
                for f in CANDIDATE_FIELDS:
                    obs[("H" if home else "A", f)].add((int(teams[o][f]), difficulty))
    out = {}
    for key, pairs in obs.items():
        ps = sorted(pairs)
        det = len({v for v, _ in ps}) == len(ps)
        mono = all(ps[i][1] <= ps[i + 1][1] for i in range(len(ps) - 1))
        out[key] = (det, mono, len({v for v, _ in ps}))
    return out


def live_venue_check():
    """The venue pairing, against the only payloads holding fixtures AND teams.

    `data/captures/2026-*` are live captures with `fixtures.json.gz` beside
    `bootstrap-static.json.gz`. In 2026/27 FPL publishes `strength_overall_*`
    already on the 1-5 difficulty scale, so the map degenerates to the identity
    and what remains testable is exactly the part that matters here: WHICH field
    pairs with WHICH venue. ⚠️ The decoded THRESHOLDS do not transport to these
    payloads — the units changed — so this is a check on structure only.
    """
    rows = []
    for cap in sorted(glob.glob(os.path.join(CAPTURES, "2026-*"))):
        bs = os.path.join(cap, "bootstrap-static.json.gz")
        fx = os.path.join(cap, "fixtures.json.gz")
        if not (os.path.exists(bs) and os.path.exists(fx)):
            continue
        with gzip.open(bs) as fh:
            teams = {t["id"]: t for t in json.load(fh)["teams"]}
        with gzip.open(fx) as fh:
            fixtures = json.load(fh)
        # A club whose home and away ratings are equal cannot separate the two
        # pairings, so it is not evidence. Counted, because the raw 760 is not
        # the denominator the check earns.
        splits = {i for i, t in teams.items()
                  if t["strength_overall_home"] != t["strength_overall_away"]}
        n = matched = swapped = disc = 0
        for f in fixtures:
            if f.get("team_h_difficulty") is None:
                continue
            h, a = f["team_h"], f["team_a"]
            n += 2
            disc += (a in splits) + (h in splits)
            matched += teams[a]["strength_overall_home"] == f["team_h_difficulty"]
            matched += teams[h]["strength_overall_away"] == f["team_a_difficulty"]
            swapped += teams[a]["strength_overall_away"] == f["team_h_difficulty"]
            swapped += teams[h]["strength_overall_home"] == f["team_a_difficulty"]
        rows.append((os.path.basename(cap), n, matched, swapped, disc,
                     len(splits), len(teams)))
    return rows


def check_map_globally(archive, seasons):
    """The map must reproduce EVERY archived difficulty exactly, or it is not a map.

    The gate is "residual identically zero", never "a good fit" — this record's
    own standard, set when a mis-coded sibling feature returned plausible
    coefficients at an 88%-exact fit.
    """
    total = bad = 0
    per_season = {}
    for s in seasons:
        opp, teams = archive_season(archive, s)
        n = w = 0
        for (_gw, team), lst in opp.items():
            for o, home, difficulty in lst:
                fld = "strength_overall_home" if home else "strength_overall_away"
                n += 1
                if difficulty_from(int(teams[o][fld])) != difficulty:
                    w += 1
        per_season[s] = (n - w, n)
        total += n
        bad += w
    return per_season, total, bad


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("rows")
    ap.add_argument("out")
    ap.add_argument("--archive", default="/tmp/fplarch")
    # Pre-registration S1: rebuild from a staler capture, to show the reading is
    # not knife-edge on the cutoff convention. `--cutoff-lag 1` reads GW{gw-1}.
    ap.add_argument("--cutoff-lag", type=int, default=0)
    args = ap.parse_args()
    if args.cutoff_lag < 0:
        fail("--cutoff-lag must be >= 0; a negative lag reads a capture taken "
             "AFTER the gameweek being predicted, which is the leak this arm exists "
             "to remove")

    with open(args.rows) as fh:
        banked = list(csv.DictReader(fh))
    if not banked:
        fail("no rows in ", args.rows)
    for c in ("season", "gw", "team", "def", "xgc90"):
        if c not in banked[0]:
            fail("the rows file is missing ", c)

    seasons = sorted({r["season"] for r in banked})
    caps = {s: captures(s) for s in seasons}
    cap = {s: caps[s][0] for s in seasons}
    crawled = {s: caps[s][1] for s in seasons}
    arch = {s: archive_season(args.archive, s) for s in seasons}

    # --- the map, checked on every archived fixture-side before anything else --
    per_season, total, bad = check_map_globally(args.archive, seasons)
    print("=== the decoded map, checked on every archived fixture-side ===",
          file=sys.stderr)
    for s in seasons:
        ok, n = per_season[s]
        print(f"  {s}: {ok}/{n} difficulties reproduced exactly"
              + ("  ⚠️ MISMATCH" if ok != n else ""), file=sys.stderr)
    print(f"  pooled: {total - bad}/{total}", file=sys.stderr)
    if bad:
        fail("the map does not reproduce the archive's own difficulty column on ",
             bad, " of ", total, " fixture-sides. It is not a map, and nothing "
             "downstream may use it.")

    # ⚠️ The denominator. 4560 fixture-sides is the ROW count, not the evidence
    # count: a club-season's 38 sides repeat one constraint, because `def` is one
    # value per club-venue for the whole season. Quote both.
    constraints = set()
    cells = set()
    for s in seasons:
        opp, teams = arch[s]
        for (_gw, _team), lst in opp.items():
            for o, home, difficulty in lst:
                fld = "strength_overall_home" if home else "strength_overall_away"
                constraints.add((fld, int(teams[o][fld]), difficulty))
                cells.add((s, o, fld))
    print(f"  ⚠️ DENOMINATOR: those {total} fixture-sides are {len(constraints)} "
          f"distinct (field, strength, difficulty)\n     constraints over "
          f"{len(cells) // 2} club-seasons — a club-season's 38 sides repeat one\n"
          f"     constraint. 4560 is the row count; {len(constraints)} is the evidence.",
          file=sys.stderr)

    # The sharpest thing the decoding buys, and it is a MEASUREMENT rather than
    # the correlational inference the predecessor arm carried: if the archive's
    # difficulty column had been frozen at season start, it would have to be
    # reproduced by the FIRST capture's strength. It is not, by a wide margin.
    end_ok = first_ok = n = 0
    for s in seasons:
        opp, teams = arch[s]
        first = cap[s][min(cap[s])]
        for (_gw, _team), lst in opp.items():
            for o, home, difficulty in lst:
                fld = "strength_overall_home" if home else "strength_overall_away"
                n += 1
                end_ok += difficulty_from(int(teams[o][fld])) == difficulty
                first_ok += difficulty_from(first[o][0 if home else 1]) == difficulty
    print("\n=== end-stamped, or frozen at season start? ===", file=sys.stderr)
    print(f"  archive difficulty == step(END-of-season strength):   {end_ok}/{n}",
          file=sys.stderr)
    print(f"  archive difficulty == step(FIRST capture's strength): {first_ok}/{n}",
          file=sys.stderr)
    print("  The archive's column is the END-of-season one, and a column frozen at\n"
          "  season start is REFUTED here rather than inferred — it would have to\n"
          "  reproduce every side and reproduces 60%. ⚠️ What is still ASSUMED is\n"
          "  whether FPL PUBLISHED the intermediate values. If it froze them, the\n"
          "  correct point-in-time column is the season-start one, a third column\n"
          "  this arm does not fit.", file=sys.stderr)

    print("\n=== which strength field can carry a map at all ===", file=sys.stderr)
    screen = field_screen(args.archive, seasons)
    for f in CANDIDATE_FIELDS:
        cells = " ".join(
            f"{v}: deterministic={screen[(v, f)][0]!s:5s} monotone={screen[(v, f)][1]!s:5s}"
            for v in ("H", "A"))
        print(f"  {f:24s} {cells}", file=sys.stderr)
    print("  -> only strength_overall_home (at home) and strength_overall_away "
          "(away) qualify.\n     The coarse 1-5 `strength` does NOT.", file=sys.stderr)

    print("\n=== the venue pairing, on a LIVE payload carrying fixtures ===",
          file=sys.stderr)
    live = live_venue_check()
    for name, n, matched, swapped, disc, clubs, ntm in live:
        print(f"  {name}: {n} fixture-sides, venue-matched identity {matched}, "
              f"venue-swapped {swapped}", file=sys.stderr)
        print(f"    ⚠️ only {disc} of those {n} sides can discriminate at all — the "
              f"rest face a club\n       whose home and away ratings are equal. "
              f"{clubs} of {ntm} clubs carry the check.", file=sys.stderr)
    if len(live) > 1 and live[0][1:] == live[1][1:]:
        print("  ⚠️ The two rows above are ONE observation printed twice: no team "
              "field\n     differs between the two captures.", file=sys.stderr)
    print("  ⚠️ structure only — 2026/27 publishes strength_overall_* already on "
          "the 1-5\n     scale, so the decoded THRESHOLDS do not transport. And the "
          "archive-side\n     field screen below forces the same pairing "
          "independently, which is the\n     cheaper and stronger support.",
          file=sys.stderr)

    print("\n=== capture timing: every capture must predate its gameweek's first "
          "kickoff ===", file=sys.stderr)
    for s in seasons:
        late, stale = check_capture_timing(s, crawled[s], args.archive)
        print(f"  {s}: {len(late)} captures postdate a kickoff; "
              f"{len(stale)} are more than 8 days stale "
              + (f"{sorted(stale)}" if stale else ""), file=sys.stderr)
        if late:
            fail("capture(s) taken AFTER the gameweek they name, which is not a "
                 "cutoff state: ", late)

    print("\n=== the terminal anchor: GW38 capture vs the archive's teams.csv ===",
          file=sys.stderr)
    for s in seasons:
        teams = arch[s][1]
        last = max(cap[s])
        gap = {k for k in teams if k in cap[s][last]
               and (int(teams[k]["strength_overall_home"]),
                    int(teams[k]["strength_overall_away"])) != cap[s][last][k]}
        print(f"  {s}: GW{last} capture disagrees for {len(gap)}/{len(teams)} clubs",
              file=sys.stderr)

    out_rows = []
    unmatched = Counter()
    for r in banked:
        s, gw, team = r["season"], int(r["gw"]), int(r["team"])
        lst = arch[s][0].get((gw, team), [])
        if len(lst) != 1:
            # The Go emitter already required exactly one fixture, so anything
            # else here is a join failure rather than a double gameweek.
            unmatched[(s, len(lst))] += 1
            continue
        o, home, difficulty_end = lst[0]
        fld = "strength_overall_home" if home else "strength_overall_away"
        strength_end = int(arch[s][1][o][fld])
        # GW1 has no earlier capture, so a lagged run clamps to GW1 rather than
        # dropping rows: changing the POPULATION between the primary and its own
        # sensitivity would make the two incomparable, which is the more
        # misleading of the two failures.
        cap_gw = max(1, gw - args.cutoff_lag)
        pit = cap[s].get(cap_gw, {}).get(o)
        if pit is None:
            unmatched[(s, "no capture")] += 1
            continue
        strength_cut = pit[0] if home else pit[1]
        difficulty_pit = difficulty_from(strength_cut)
        ambiguous = any(lo < strength_cut < hi for lo, hi in GAPS)
        row = dict(r)
        row.update(
            opponent=o, home=home,
            strength_cut=strength_cut, strength_end=strength_end,
            difficulty_end=difficulty_end, difficulty_pit=difficulty_pit,
            def_pit=DEFENCE_MULTIPLIER[difficulty_pit],
            def_moved=int(abs(DEFENCE_MULTIPLIER[difficulty_pit] - float(r["def"])) > 1e-9),
            ambiguous=int(ambiguous),
        )
        out_rows.append(row)

    if unmatched:
        fail("unjoined rows, which is a broken key rather than a result: ",
             dict(unmatched))
    if len(out_rows) != len(banked):
        fail("row count changed: ", len(banked), " -> ", len(out_rows))

    # The banked `def` re-derived from the archive's own difficulty column. This
    # pins three things at once: the join key, that `band_strength` was 0 so
    # `defenceBandAdj` is exactly 1, and that `defFixtureScale` was 1 so `ladder`
    # is the identity. Without it, `def_pit` and `def` could be on two different
    # ladders and the comparison would be meaningless.
    off = sum(1 for r in out_rows
              if abs(DEFENCE_MULTIPLIER[r["difficulty_end"]] - float(r["def"])) > 1e-9)
    print(f"\n`def` re-derived from the archive's difficulty column: "
          f"{len(out_rows) - off}/{len(out_rows)} agree", file=sys.stderr)
    if off:
        fail("the banked `def` is not defenceMultiplier(end-of-season difficulty) "
             "on ", off, " rows, so def_pit and def are not on the same ladder")

    fields = list(banked[0].keys()) + [
        "opponent", "home", "strength_cut", "strength_end",
        "difficulty_end", "difficulty_pit", "def_pit", "def_moved", "ambiguous",
    ]
    os.makedirs(os.path.dirname(os.path.abspath(args.out)), exist_ok=True)
    with open(args.out, "w", newline="") as fh:
        w = csv.DictWriter(fh, fieldnames=fields, extrasaction="ignore")
        w.writeheader()
        w.writerows(out_rows)

    # --- liveness ------------------------------------------------------------
    #
    # THE bar this arm lives or dies on. The predecessor's liveness check could
    # not fire, because its join copied `def` through unchanged. This one can:
    # if the reconstruction returns the banked column, the same column has been
    # rebuilt and the arm is void. The fraction is reported, never asserted.
    native = {"2023-24", "2024-25", "2025-26"}
    for label, rows in (("native (three seasons, carries the verdict)",
                         [r for r in out_rows if r["season"] in native]),
                        ("pooled (six seasons, context only)", out_rows)):
        n = len(rows)
        moved = [r for r in rows if r["def_moved"]]
        print(f"\n=== liveness: {label}, n = {n} ===", file=sys.stderr)
        print(f"  def_pit != def_end on {len(moved)}/{n} = {len(moved) / n:.4f}"
              + ("   ⚠️ VOID: the reconstruction IS the banked column"
                 if not moved else ""), file=sys.stderr)
        print("  def_end:", sorted(Counter(r["def"] for r in rows).items()),
              file=sys.stderr)
        print("  def_pit:", sorted(Counter(r["def_pit"] for r in rows).items()),
              file=sys.stderr)
        print("  difficulty_pit - difficulty_end:",
              sorted(Counter(r["difficulty_pit"] - r["difficulty_end"]
                             for r in rows).items()), file=sys.stderr)
        print("  rows whose cutoff strength lands in a decoded GAP:",
              sum(r["ambiguous"] for r in rows), file=sys.stderr)
        for sn in sorted({r["season"] for r in rows}):
            sub = [r for r in rows if r["season"] == sn]
            k = sum(r["def_moved"] for r in sub)
            print(f"    {sn}  moved {k}/{len(sub)} = {k / len(sub):.4f}",
                  file=sys.stderr)

        # `def_pit` must also VARY, and vary within each level of `def_moved`,
        # or the refit's second channel is collinear with the first.
        for v in (0, 1):
            sub = [r for r in rows if r["def_moved"] == v]
            print(f"  def_pit | def_moved = {v}:",
                  sorted(Counter(r["def_pit"] for r in sub).items()),
                  file=sys.stderr)

        # The design-side share of the banked regressor's squared variation that
        # sits on rows the reconstruction moves. No outcome enters it.
        w2 = [float(r["xgc90"]) * (float(r["def"]) - 1) for r in rows]
        tot = sum(v * v for v in w2)
        mv = sum(v * v for v, r in zip(w2, rows) if r["def_moved"])
        print(f"  share of sum w2_end^2 carried by moved rows = "
              f"{mv / tot if tot else float('nan'):.4f}", file=sys.stderr)

        # `def` is one value per club-venue for the whole season in the ARCHIVE,
        # which is what an end-stamped column looks like. The reconstruction must
        # break that, or it has not moved anything in time.
        #
        # ⚠️ Only the SECOND line below is evidence. `def_end` reading 0/N is an
        # arithmetic consequence of the map check that ran earlier and had to
        # pass — `def_end = DEFENCE_MULTIPLIER[step(strength_end[opponent][venue])]`,
        # and both are fixed within the cell. Monotonicity the construction forces
        # is not evidence, and this row was briefly quoted as the sharpest in the
        # table when it cannot fail. The liveness that CAN fail is the move rate
        # above and `def_pit`'s count here.
        for col, name in (("def", "def_end"), ("def_pit", "def_pit")):
            cells = defaultdict(set)
            for r in rows:
                cells[(r["season"], r["opponent"], r["home"])].add(r[col])
            varying = sum(1 for v in cells.values() if len(v) > 1)
            print(f"  club-venue-season cells with more than one {name}: "
                  f"{varying}/{len(cells)}", file=sys.stderr)

    print(f"\nwrote {len(out_rows)} rows to {args.out}", file=sys.stderr)


if __name__ == "__main__":
    main()
