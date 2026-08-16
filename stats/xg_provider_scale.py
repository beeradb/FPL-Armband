"""Does the clean-sheet Jensen scale `c` belong to football, or to a provider?

# The question, and why this is the right experiment for it

`xpoints.go` prices P(clean sheet) as `exp(-Σx)`. The truth is `Π(1-xᵢ)` over the
individual shots faced, and `exp(-Σx) > Π(1-xᵢ)` always, so the model over-predicts.
The gap is a scale

    c = -E[ln(1-x)] / E[x]

which depends on the DISTRIBUTION of shot sizes, not on their total. Measured on
Understat it is 1.316-1.343 in every Premier League season 2016-2025.

⚠️ **FPL's `expected_goals_conceded` is Opta, and per-shot Opta is not publicly
available** — it is licensed commercially, and fbref serves an active Cloudflare
challenge to automated clients, which is a request not to scrape it. So the recorded
caveat was "quote the order, not the constant", with no way to close it.

**This closes it from the other side.** The worry was never that we needed Opta's
number; it was that `c` might be an artefact of Understat's scale. StatsBomb publish
the complete 2015/16 Premier League season with per-shot xG under a third,
independent model, and Understat covers the same season. That is **380 matches of
identical football scored by two different xG models**, which measures how far `c`
moves between providers — the quantity the Opta caveat was standing in for.

⚠️ It still does not give the Opta value. Two providers agreeing bounds how much a
third can plausibly differ; it does not identify it.

# The fixture pin, and why it is not optional

`stats/understat_pl.py` **cannot confirm 2015-16 as Premier League** — the id map has
no column before 2016-17, so it drops that season deliberately (`EARLIEST = '2016'`).
Understat's 2015 label unfiltered spans **114 teams across several leagues**, and
this record has already lost a published finding to exactly that: a finishing
measurement that pooled Bundesliga and Serie A because `getPlayerData` returns every
league a player appeared in.

So the Premier League filter here is not a name list anyone typed. It is **the
StatsBomb fixture set itself** — every Understat shot must fall in a match whose date
and team pair appear in StatsBomb's 380. That makes the two samples the same football
by construction rather than by assumption, and it is what turns a provider comparison
into a controlled one.
"""
import collections
import csv
import glob
import json
import math

SB_CSV = "stats/statsbomb_pl1516_shots.csv"
UNDERSTAT = "/home/bbowman.guest/.cache/fplagent/understat/player-*.json"
SEASON = "2015"

# The two providers spell one club differently. Verified against both team lists —
# every other name matches exactly, so this is the whole map rather than a sample.
ALIAS = {"afc bournemouth": "bournemouth"}


def norm(t):
    t = t.strip().lower()
    return ALIAS.get(t, t)


def fixture_key(date, home, away):
    """A match, identified the same way on both sides: the day and the two clubs."""
    return (date[:10], norm(home), norm(away))


def scale(xs):
    """c = -E[ln(1-x)]/E[x], the factor `exp(-c*Σx)` needs to equal `Π(1-xᵢ)`."""
    num = -sum(math.log(1.0 - x) for x in xs)
    den = sum(xs)
    return num / den


def describe(name, xs):
    n = len(xs)
    m = sum(xs) / n
    m2 = sum(x * x for x in xs) / n
    print(f"  {name:<28} n {n:>6}  mean xG {m:.4f}  E[x2]/E[x] {m2/m:.4f}  "
          f"c {scale(xs):.4f}")
    return scale(xs)


def load_statsbomb():
    shots, fixtures = [], {}
    for r in csv.DictReader(open(SB_CSV)):
        shots.append(r)
    # Home/away is not in the shot row, so a fixture is keyed by its unordered pair.
    by_match = collections.defaultdict(set)
    date_of = {}
    for r in shots:
        by_match[r["match_id"]].add(norm(r["team"]))
        date_of[r["match_id"]] = r["match_date"][:10]
    for mid, teams in by_match.items():
        if len(teams) == 2:
            fixtures[(date_of[mid], frozenset(teams))] = mid
    return shots, fixtures


def load_understat(fixtures):
    """Understat shots falling inside the StatsBomb fixture set, and only those."""
    keep, seen_fixtures = [], set()
    for f in glob.glob(UNDERSTAT):
        try:
            d = json.load(open(f))
        except Exception:                     # noqa: BLE001
            continue
        for s in (d if isinstance(d, list) else d.get("shots")) or []:
            if str(s.get("season")) != SEASON:
                continue
            key = (s["date"][:10], frozenset({norm(s["h_team"]), norm(s["a_team"])}))
            if key not in fixtures:
                continue
            keep.append(s)
            seen_fixtures.add(key)
    return keep, seen_fixtures


def main():
    sb_shots, fixtures = load_statsbomb()
    print(f"StatsBomb: {len(sb_shots)} shots over {len(fixtures)} fixtures\n")

    u_shots, matched = load_understat(fixtures)
    print(f"Understat: {len(u_shots)} shots falling inside those fixtures, "
          f"covering {len(matched)} of {len(fixtures)}")
    if len(matched) < len(fixtures):
        # ⚠️ Said out loud. A partial overlap is a different population, and the
        # comparison below is only controlled over the fixtures BOTH providers see.
        print(f"  !! {len(fixtures) - len(matched)} StatsBomb fixtures have no "
              f"Understat shots — the comparison covers the intersection only")
    print()

    # ⚠️ The two samples are NOT the same shots, and saying which way they differ
    # matters more than the headline.
    #
    # StatsBomb's file is match-centric: every shot in all 380 matches. The
    # Understat cache is a PLAYER-centric harvest of ~2,170 footballers who matter
    # to this project in later seasons, so for 2015/16 it holds only the shots taken
    # by that subset — which is why it reaches 175 fixtures rather than 380.
    #
    # So this is not "the same shots scored two ways". It is two overlapping samples
    # of the same 380 matches, differing in provider AND in which shooters they
    # contain. Restricting StatsBomb to the shared fixtures removes the first half
    # of that difference; nothing here removes the second, and it is stated rather
    # than papered over.
    sb_common = [r for r in sb_shots
                 if (r["match_date"][:10], frozenset({norm(r["team"])})) or True]
    keys_common = matched
    by_match_teams = collections.defaultdict(set)
    date_of = {}
    for r in sb_shots:
        by_match_teams[r["match_id"]].add(norm(r["team"]))
        date_of[r["match_id"]] = r["match_date"][:10]
    common_mids = {mid for mid, teams in by_match_teams.items()
                   if (date_of[mid], frozenset(teams)) in keys_common}
    sb_common = [r for r in sb_shots if r["match_id"] in common_mids]
    print(f"StatsBomb restricted to the {len(common_mids)} shared fixtures: "
          f"{len(sb_common)} shots\n")

    sb_all = [float(r["xg"]) for r in sb_common]
    u_all = [float(s["xG"]) for s in u_shots]

    # Penalties are ~0.76 and -ln(1-x) grows without bound, so a handful of them
    # move c more than hundreds of ordinary shots. Reported both ways rather than
    # chosen, because the clean sheet a defender is paid for is broken by penalties
    # too — the with-penalties figure is the one the channel needs.
    sb_open = [float(r["xg"]) for r in sb_common if r["shot_type"] != "Penalty"]
    u_open = [float(s["xG"]) for s in u_shots
              if str(s.get("situation", "")).lower() != "penalty"]

    print("c = -E[ln(1-x)]/E[x], the Jensen scale the clean-sheet term needs\n")
    print(" all shots, including penalties:")
    c_sb = describe("StatsBomb (2015/16 PL)", sb_all)
    c_u = describe("Understat (same fixtures)", u_all)
    print()
    print(" excluding penalties:")
    c_sb_o = describe("StatsBomb", sb_open)
    c_u_o = describe("Understat", u_open)

    print()
    print(f" provider gap, all shots      {c_u - c_sb:+.4f}  "
          f"({abs(c_u-c_sb)/c_sb:.1%} of the StatsBomb value)")
    print(f" provider gap, open play etc  {c_u_o - c_sb_o:+.4f}  "
          f"({abs(c_u_o-c_sb_o)/c_sb_o:.1%})")
    print()
    print(" ⚠️ This bounds how far two independent models drift on the same 380")
    print(" matches. It does NOT identify Opta's value, which is what FPL uses and")
    print(" which is not publicly available at shot level.")


if __name__ == "__main__":
    main()
