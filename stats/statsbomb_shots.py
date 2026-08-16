"""Acquire per-shot xG for the 2015/16 Premier League from StatsBomb's open data.

# Why a third provider, when the question was about Opta

The clean-sheet channel prices P(no goal) as `exp(-Σx)` where the truth is
`Π(1-xᵢ)`. The gap between them is a scale `c = -E[ln(1-x)]/E[x]` that depends on
the DISTRIBUTION of individual shot sizes. Measured on Understat it is 1.32-1.34 in
every Premier League season 2016-2025.

The open question was whether that number is a property of *football* or of
*Understat's scale*, because FPL's `expected_goals_conceded` is Opta and the archive
carries only per-gameweek aggregates. **Per-shot Opta is not publicly available** —
it is licensed commercially, and fbref, the usual proxy, serves an active Cloudflare
challenge to automated clients, which is a request not to scrape it.

So this acquires the question's *answer* rather than its named input. StatsBomb
publish the complete 2015/16 Premier League season with per-shot xG under a
**third, independent** model, free and openly licensed. Understat covers the same
season. That is 380 matches of identical football scored by two different xG models,
which measures the thing the Opta caveat was standing in for: **how much does `c`
move between providers?**

⚠️ It does not give the Opta value and cannot. What it gives is the *size of
provider disagreement* on this specific functional, which is what bounds the
uncertainty. If two independent models agree on `c` over the same 380 matches, a
third differing wildly becomes implausible; if they disagree, the disagreement is
the bound and the constant is not transportable at any precision.

# What it stores, and what it throws away

Each StatsBomb events file is ~3.2 MB and carries every touch in the match; the
shots are ~33 of ~3,700 events. Downloading 380 of them is ~1.2 GB to keep a few
hundred kilobytes, so this streams each match, extracts the shots, and discards the
rest. The artefact is one small CSV that can be committed and re-read for free.

Re-running is cheap: the CSV is the cache, and a complete one short-circuits the
whole download.

Source: https://github.com/statsbomb/open-data — free, and released under a licence
that permits this use. Competition 2 (Premier League), season 27 (2015/2016).
"""
import csv
import json
import os
import sys
import time
import urllib.request

RAW = "https://raw.githubusercontent.com/statsbomb/open-data/master/data"
COMP, SEASON = 2, 27
OUT = "stats/statsbomb_pl1516_shots.csv"
# Polite: this is somebody's free public repository, not an API we pay for.
DELAY_S = 0.15
FIELDS = ["match_id", "match_date", "team", "minute", "xg", "shot_type", "outcome"]


def get(url):
    req = urllib.request.Request(url, headers={"User-Agent": "fplagent-research/1.0"})
    with urllib.request.urlopen(req, timeout=90) as r:
        return json.load(r)


def main():
    if os.path.exists(OUT):
        with open(OUT) as f:
            n = sum(1 for _ in f) - 1
        print(f"{OUT} already holds {n} shots; delete it to re-acquire.")
        return

    matches = get(f"{RAW}/matches/{COMP}/{SEASON}.json")
    print(f"{len(matches)} matches in competition {COMP} season {SEASON}")

    rows = []
    for i, m in enumerate(sorted(matches, key=lambda x: x["match_id"]), 1):
        mid = m["match_id"]
        try:
            events = get(f"{RAW}/events/{mid}.json")
        except Exception as e:            # noqa: BLE001 — one bad match must not
            print(f"  !! {mid}: {e}")     # lose the other 379; the count is printed
            continue                       # at the end so a short run is visible
        for e in events:
            if e.get("type", {}).get("name") != "Shot":
                continue
            s = e.get("shot") or {}
            xg = s.get("statsbomb_xg")
            if xg is None:
                continue
            rows.append({
                "match_id": mid,
                "match_date": m.get("match_date", ""),
                "team": (e.get("team") or {}).get("name", ""),
                "minute": e.get("minute", ""),
                "xg": f"{float(xg):.6f}",
                "shot_type": (s.get("type") or {}).get("name", ""),
                "outcome": (s.get("outcome") or {}).get("name", ""),
            })
        if i % 25 == 0 or i == len(matches):
            print(f"  {i}/{len(matches)} matches, {len(rows)} shots", flush=True)
        time.sleep(DELAY_S)

    if not rows:
        sys.exit("no shots extracted — refusing to write an empty cache")

    with open(OUT, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=FIELDS)
        w.writeheader()
        w.writerows(rows)

    seen = len({r["match_id"] for r in rows})
    print(f"wrote {OUT}: {len(rows)} shots over {seen} matches")
    # ⚠️ Said out loud rather than left to a reader who assumes 380. A partial
    # acquisition is a smaller season, and a smaller season is a different
    # population -- the same trap as a killed sweep reading as a complete one.
    if seen != len(matches):
        print(f"  !! {len(matches) - seen} matches missing — this is a PARTIAL "
              f"season and must be described as one")


if __name__ == "__main__":
    main()
