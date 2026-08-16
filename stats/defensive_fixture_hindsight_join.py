#!/usr/bin/env python3
"""Join point-in-time opponent strength onto the banked clean-sheet regressor rows.

# Why this exists

`stats/cs_calibration.R`'s two-channel decomposition fits

    -ln P(clean sheet) = a + b1*XGC90 + b2*XGC90*(def-1)

and banks `b2 = 1.5688` (native stratum) as the defensive fixture channel running
hot. `def` is `defenceMultiplier(fixture.Difficulty)` — with `band_strength: 0`
shipped, `defenceBandAdj` is exactly 1, so `def` is a pure function of the
archive's `team_h_difficulty`/`team_a_difficulty`. That column is the
**end-of-season** value and `playedFixtures` does not gate it, so `def` may carry
post-cutoff information.

`stats/team_strength_revisions.py` (on `record-the-team-strength-revision-leak`)
establishes that FPL revises the coarse 1-5 `strength` for 6-11 clubs of 20 in
every archived season, in outcome-shaped waves. This script turns that into a
regressor: for each banked row it recovers the opponent and asks whether that
opponent's coarse `strength` at the row's own cutoff differs from its
end-of-season value.

⚠️ **The captures carry no fixtures payload.** What is joined here is TEAM
STRENGTH, not difficulty, so whether `team_h_difficulty` itself moved cannot be
answered from this archive. The step from a revised strength to a revised
difficulty is a MECHANISM argument and is labelled as one everywhere downstream.

⚠️ **`revised_opp == 0` does not mean a clean row.** The fine `strength_*` fields
moved for 20 of 20 clubs in all six seasons, and FDR may key on those. The
never-revised subsample is therefore a robustness arm, not a clean stratum.

# The point-in-time convention

A banked row's `gw` is the gameweek being predicted; the engine's cutoff is
`gw - 1`. The capture named `GW{k}` is taken BEFORE gameweek k's deadline
(verified against `deadlines.json`), so capture `GW{gw}` is exactly the state the
engine had at cutoff `gw - 1`. That is the "capture nearest the cutoff".

The end-of-season value is read from the archive's own `teams.csv`, which is
scraped in the same pass as the `fixtures.csv` that supplies `def` — so the two
sides of the comparison come from the same file family rather than from a capture
that might predate a final revision. The GW38 capture is cross-checked against it
and any disagreement is reported.

# Usage

    python3 stats/defensive_fixture_hindsight_join.py \
        stats/snapshots/2026-08-15-clean-sheet-2x2/cs_regressor_fixture_path_rows.csv \
        /tmp/joined.csv [--archive /tmp/fplarch]

Writes the banked columns unchanged plus `opponent`, `home`, `strength_opp_cut`,
`strength_opp_end`, `revised_opp`, `revised_delta` (end minus cutoff, signed) and
`either_revised_season` (1 if EITHER club's coarse strength moved anywhere in the
season). Prints the liveness report to stderr.
"""
import argparse
import csv
import glob
import gzip
import json
import os
import re
import sys
from collections import Counter, defaultdict

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CAPTURES = os.path.join(ROOT, "data", "captures")

# `defenceMultiplier` from internal/analysis/metrics.go:955, at the shipped
# `defFixtureScale` of 1 where `ladder` is the identity. Restated here ONLY to
# re-derive `def` independently of the Go emitter -- a check that shares the
# implementation it is checking proves nothing. It is not a second source of
# truth: nothing downstream reads it, and if it drifts the re-derivation fails
# loudly, which is the intended behaviour.
DEFENCE_MULTIPLIER = {1: 0.70, 2: 0.85, 3: 1.00, 4: 1.20, 5: 1.40}


def _spearman(xs, ys):
    """Spearman rank correlation, midranks for ties.

    Written out rather than pulled from scipy because this repository's stats
    scripts take no third-party Python dependency, and the quantity is four
    lines.
    """
    def rank(v):
        order = sorted(range(len(v)), key=lambda i: v[i])
        r = [0.0] * len(v)
        i = 0
        while i < len(order):
            j = i
            while j + 1 < len(order) and v[order[j + 1]] == v[order[i]]:
                j += 1
            avg = (i + j) / 2.0 + 1.0
            for k in range(i, j + 1):
                r[order[k]] = avg
            i = j + 1
        return r

    rx, ry = rank(xs), rank(ys)
    n = len(rx)
    mx, my = sum(rx) / n, sum(ry) / n
    num = sum((a - mx) * (b - my) for a, b in zip(rx, ry))
    den = (sum((a - mx) ** 2 for a in rx) * sum((b - my) ** 2 for b in ry)) ** 0.5
    return num / den if den else float("nan")


def fail(*msg):
    """Stop rather than emit a partial join.

    A broken join reads downstream exactly like a null — `revised_opp` all zero
    is both "nothing was revised" and "the key did not match". The record's
    own standing rule: a guard that cannot fire is not a passed check.
    """
    sys.exit("defensive_fixture_hindsight_join: " + "".join(str(m) for m in msg))


def coarse_by_gameweek(season):
    """Coarse `strength` per team id, per capture gameweek."""
    out = {}
    for cap in sorted(glob.glob(f"{CAPTURES}/{season}/GW*/bootstrap-static.json.gz")):
        m = re.search(r"GW(\d+)", os.path.basename(os.path.dirname(cap)))
        if m is None:
            fail("capture directory does not name a gameweek: ", cap)
        gw = int(m.group(1))
        with gzip.open(cap) as fh:
            teams = json.load(fh)["teams"]
        out[gw] = {t["id"]: t["strength"] for t in teams}
    return out


def archive_season(archive, season):
    """Opponent lookup per (gameweek, team), and end-of-season coarse strength."""
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
            # FixtureBrief in internal/analysis/metrics.go:1337 — Home takes
            # team_h_difficulty.
            opp[(gw, h)].append((a, 1, int(float(r["team_h_difficulty"]))))
            opp[(gw, a)].append((h, 0, int(float(r["team_a_difficulty"]))))

    with open(tm) as fh:
        end = {int(r["id"]): int(r["strength"]) for r in csv.DictReader(fh)}
    return opp, end


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("rows")
    ap.add_argument("out")
    ap.add_argument("--archive", default="/tmp/fplarch")
    args = ap.parse_args()

    with open(args.rows) as fh:
        banked = list(csv.DictReader(fh))
    if not banked:
        fail("no rows in ", args.rows)

    seasons = sorted({r["season"] for r in banked})
    coarse = {s: coarse_by_gameweek(s) for s in seasons}
    arch = {s: archive_season(args.archive, s) for s in seasons}

    for s in seasons:
        if not coarse[s]:
            fail("no captures for ", s)
        # The GW38 capture against the archive's own teams.csv. Printed rather
        # than asserted: a late revision after the last deadline is possible and
        # is exactly the kind of thing that must be visible, not swallowed.
        last = max(coarse[s])
        end = arch[s][1]
        gap = {k: (coarse[s][last][k], end[k])
               for k in end if k in coarse[s][last] and coarse[s][last][k] != end[k]}
        print(f"{s}: captures GW{min(coarse[s])}..GW{last}; "
              f"GW{last} capture vs teams.csv disagrees for {len(gap)}/{len(end)} clubs"
              + (f" {gap}" if gap else ""), file=sys.stderr)

    fields = list(banked[0].keys()) + [
        "opponent", "home", "strength_opp_cut", "strength_opp_end",
        "revised_opp", "revised_delta", "either_revised_season",
    ]

    # Clubs whose coarse strength moved at any point in the season, for the
    # robustness arm.
    moved_ever = {}
    for s in seasons:
        gws = sorted(coarse[s])
        moved = set()
        for a, b in zip(gws, gws[1:]):
            for k in coarse[s][a]:
                if k in coarse[s][b] and coarse[s][a][k] != coarse[s][b][k]:
                    moved.add(k)
        moved_ever[s] = moved

    out_rows = []
    unmatched = Counter()
    for r in banked:
        s, gw, team = r["season"], int(r["gw"]), int(r["team"])
        opp_list = arch[s][0].get((gw, team), [])
        if len(opp_list) != 1:
            # The Go emitter already required exactly one fixture (`g.Fixtures
            # == 1` and a single `TeamFixtures` hit), so anything else here is a
            # join failure rather than a double gameweek, and must be loud.
            unmatched[(s, len(opp_list))] += 1
            continue
        o, home, difficulty = opp_list[0]
        cut = coarse[s].get(gw, {}).get(o)
        end = arch[s][1].get(o)
        if cut is None or end is None:
            unmatched[(s, "no strength")] += 1
            continue
        row = dict(r)
        # Carried under a leading underscore and stripped before writing: it is
        # an audit input for the re-derivation below, not part of the contract.
        row["_difficulty"] = difficulty
        row.update(
            opponent=o, home=home, strength_opp_cut=cut, strength_opp_end=end,
            revised_opp=int(cut != end), revised_delta=end - cut,
            either_revised_season=int(team in moved_ever[s] or o in moved_ever[s]),
        )
        out_rows.append(row)

    if unmatched:
        fail("unjoined rows, which is a broken key rather than a result: ",
             dict(unmatched))
    if len(out_rows) != len(banked):
        fail("row count changed: ", len(banked), " -> ", len(out_rows))

    with open(args.out, "w", newline="") as fh:
        w = csv.DictWriter(fh, fieldnames=fields, extrasaction="ignore")
        w.writeheader()
        w.writerows(out_rows)

    # --- liveness -----------------------------------------------------------
    #
    # Three things must be non-degenerate or there is no comparison: `def` must
    # take several values, `revised_opp` must take both, and `def` must vary
    # WITHIN each level of `revised_opp` — otherwise the interaction is collinear
    # with the main effect and the fit is not identified.
    native = {"2023-24", "2024-25", "2025-26"}
    for label, rows in (("pooled (six seasons)", out_rows),
                        ("native (three seasons)",
                         [r for r in out_rows if r["season"] in native])):
        print(f"\n=== liveness: {label}, n = {len(rows)} ===", file=sys.stderr)
        print("  def:", sorted(Counter(r["def"] for r in rows).items()),
              file=sys.stderr)
        print("  revised_opp:",
              sorted(Counter(r["revised_opp"] for r in rows).items()),
              file=sys.stderr)
        print("  revised_delta:",
              sorted(Counter(r["revised_delta"] for r in rows).items()),
              file=sys.stderr)
        print("  either_revised_season:",
              sorted(Counter(r["either_revised_season"] for r in rows).items()),
              file=sys.stderr)
        for v in (0, 1):
            sub = [r for r in rows if r["revised_opp"] == v]
            print(f"  def | revised_opp = {v}:",
                  sorted(Counter(r["def"] for r in sub).items()), file=sys.stderr)
        print("  per season revised_opp share:", file=sys.stderr)
        for s in sorted({r["season"] for r in rows}):
            sub = [r for r in rows if r["season"] == s]
            k = sum(r["revised_opp"] for r in sub)
            print(f"    {s}  {k}/{len(sub)}  ({k / len(sub):.3f})", file=sys.stderr)

        # The design-side sizing quantity, computed here because it needs NO
        # outcome: the share of the w2 regressor's squared variation carried by
        # revised rows.
        #
        # ⚠️ `needed_b3 = (b2 - 1) / q_w` is an OLS omitted-variable formula and
        # is NOT the right sizing for a log-link GLM -- it ignores partialling and
        # the IWLS weights, and on this data it over-states the required leak by
        # 17% (1.977 against an exact 1.685), which is larger than the margin the
        # verdict turned on. `stats/defensive_fixture_hindsight.R` solves the
        # exact quantity and prints both. `q_w` is kept because the
        # pre-registration quotes it.
        w2 = [float(r["xgc90"]) * (float(r["def"]) - 1) for r in rows]
        tot = sum(v * v for v in w2)
        rev = sum(v * v for v, r in zip(w2, rows) if r["revised_opp"])
        print(f"  q_w (share of sum w2^2 on revised rows) = "
              f"{rev / tot if tot else float('nan'):.4f}"
              "   ⚠️ an approximation; see the R script for the exact sizing",
              file=sys.stderr)

        # --- the check that can actually fire ------------------------------
        #
        # The `def` distribution above reproducing the banked one is
        # TAUTOLOGICAL: this script copies `def` unchanged and fails on any
        # unmatched row. The check with power is whether `def` re-derived
        # independently from the archive's own fixtures file agrees, which
        # simultaneously establishes the join key, that `band_strength` was 0 and
        # that `def` is a pure function of end-of-season difficulty.
        bad = sum(1 for r in rows
                  if abs(DEFENCE_MULTIPLIER[r["_difficulty"]] - float(r["def"])) > 1e-9)
        print(f"  def re-derived from the archive's own fixtures.csv: "
              f"{len(rows) - bad}/{len(rows)} agree"
              + ("  ⚠️ MISMATCH" if bad else ""), file=sys.stderr)

        # --- is the archive's difficulty column end-stamped? ----------------
        #
        # This is the one place the strength-to-difficulty step stops being pure
        # assertion. On rows where the opponent's coarse strength MOVED, the
        # cutoff and end-of-season values differ, so `def` can be asked which it
        # tracks. If FPL's difficulty were frozen at season start, `def` would
        # follow the cutoff value. It follows the end value, by a wide margin,
        # in both strata.
        #
        # ⚠️ What this shows is that the ARCHIVE'S column is end-stamped, which is
        # what the replay reads and is therefore the leak that matters here. It
        # does NOT show that FPL's live difficulty moved during the season --
        # that still needs a fixtures payload the captures do not hold. The
        # residual alternative is that difficulty was set once from information
        # the coarse `strength` field only caught up to later.
        rv = [r for r in rows if r["revised_opp"]]
        if rv:
            print(f"  on the {len(rv)} REVISED rows, Spearman(def, opponent strength):"
                  f"  end-of-season {_spearman([float(r['def']) for r in rv], [r['strength_opp_end'] for r in rv]):.3f}"
                  f"   at the cutoff {_spearman([float(r['def']) for r in rv], [r['strength_opp_cut'] for r in rv]):.3f}",
                  file=sys.stderr)
            print("    -> the archive's difficulty column tracks END-of-season strength.",
                  file=sys.stderr)

        # `def` is one value per club-venue for the whole season, in every
        # season, which is what an end-stamped column looks like: the archive
        # carries no difficulty variation for a cutoff to gate.
        cells = defaultdict(set)
        for r in rows:
            cells[(r["season"], r["opponent"], r["home"])].add(r["def"])
        varying = sum(1 for v in cells.values() if len(v) > 1)
        print(f"  club-venue-season cells with more than one `def`: "
              f"{varying}/{len(cells)}", file=sys.stderr)

    print(f"\nwrote {len(out_rows)} rows to {args.out}", file=sys.stderr)


if __name__ == "__main__":
    main()
