#!/usr/bin/env python3
"""Report FPL's mid-season revisions to team strength, per season.

Why this exists: `fixtures.csv` carries ONE `team_h_difficulty` per fixture — the
end-of-season value — and `playedFixtures` strips the scoreline and the `Finished`
flag but NOT the difficulty. So if FPL revises its ratings mid-season, `def` carries
post-cutoff information into any fit that reads it, and the bias runs upward on a
fixture-interaction term.

The archived captures under `data/captures/<season>/GW*/` are the only point-in-time
record of what the ratings were AT each cutoff. This script reads them and reports
which clubs' ratings moved, and where.

⚠️ What this measures is TEAM STRENGTH, not difficulty. The captures carry no fixtures
payload, so whether `team_h_difficulty` itself moved cannot be answered from this
archive. The step from one to the other is a mechanism argument.

⚠️ "Never moved on the coarse `strength`" is necessary but NOT sufficient to call a row
clean: the fine `strength_*` fields move for every club in every season, and FDR may key
on those.

Run from the repository root:

    python3 stats/team_strength_revisions.py
"""
import glob
import gzip
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CAPTURES = os.path.join(ROOT, "data", "captures")

# The coarse 1-5 rating, and the fine per-venue ratings FDR may key on instead.
FINE = (
    "strength_overall_home", "strength_overall_away",
    "strength_attack_home", "strength_attack_away",
    "strength_defence_home", "strength_defence_away",
)


def seasons():
    return sorted(
        d for d in os.listdir(CAPTURES)
        if re.fullmatch(r"\d{4}-\d{2}", d) and os.path.isdir(os.path.join(CAPTURES, d))
    )


def read(season):
    """Coarse and fine ratings per gameweek, keyed by the club's short name."""
    coarse, fine = {}, {}
    for cap in sorted(glob.glob(f"{CAPTURES}/{season}/GW*/bootstrap-static.json.gz")):
        gw = int(re.search(r"GW(\d+)", cap).group(1))
        with gzip.open(cap) as fh:
            teams = json.load(fh)["teams"]
        coarse[gw] = {t["short_name"]: t["strength"] for t in teams}
        fine[gw] = {t["short_name"]: tuple(t.get(k) for k in FINE) for t in teams}
    return coarse, fine


def moved(series):
    """Clubs whose value changed between consecutive captures, and the waves."""
    gws = sorted(series)
    changed, waves = set(), []
    for a, b in zip(gws, gws[1:]):
        delta = {
            k: (series[a][k], series[b][k])
            for k in series[a]
            if k in series[b] and series[a][k] != series[b][k]
        }
        if delta:
            waves.append((a, b, delta))
            changed |= set(delta)
    return changed, waves


def main():
    if not os.path.isdir(CAPTURES):
        sys.exit(f"no captures directory at {CAPTURES}")

    for season in seasons():
        coarse, fine = read(season)
        if not coarse:
            print(f"=== {season}: no captures ===\n")
            continue

        gws = sorted(coarse)
        n = len(coarse[gws[0]])
        cmoved, waves = moved(coarse)
        fmoved, _ = moved(fine)

        print(f"=== {season}: {len(gws)} captures, GW{gws[0]}..GW{gws[-1]} ===")
        print(f"  coarse `strength` moved for {len(cmoved)}/{n} clubs")
        print(f"  fine `strength_*` moved for  {len(fmoved)}/{n} clubs")
        for a, b, delta in waves:
            body = ", ".join(f"{k} {v[0]}->{v[1]}" for k, v in sorted(delta.items()))
            print(f"    GW{a}->GW{b}: {body}")
        print()


if __name__ == "__main__":
    main()
