"""Which Understat (player, season) pairs are Premier League football?

`getPlayerData` returns a player's matches across EVERY league, and Understat
labels every league's 2022-23 as "2022" — so a player who moved carries Ligue 1,
Bundesliga and Serie A rows in the same response. Measured on this cache:
**66,777 of 182,465 matches (36.6%) have neither team in the Premier League**,
touching 821 of 1,733 players. Salah's Understat 2014/2015/2016 are Fiorentina
and Roma; Kane's 2025 is Bayern; Son's 2014 is Leverkusen.

⚠️ **This is not a hypothetical.** Two scripts here published a finishing-skill
measurement over the unfiltered pool before a code review caught it, including a
"Son is positive in 9 of 11 seasons" line where one of the eleven is Bundesliga.

# Why the filter is an id map rather than a team-name list

`understat_xg_backfill.py` already faced this and rejected name matching in as
many words: "Understat and FPL spell clubs differently ('Tottenham' against
'Spurs') and a hand-maintained mapping is the thing this project has been bitten
by repeatedly." Its own filter is the archive's minutes, which needs a
date-to-gameweek map this question does not otherwise require.

`idmap-Master.csv` answers it directly and at the right granularity: one row per
player, an `understat` id, and one column per season holding the FPL element id
for the seasons he was **in the Premier League**. Non-empty means he was in it.
"""
import csv
import os

CACHE = os.path.expanduser('~/.cache/fplagent/understat')

# Understat labels a season by its opening year; the id map uses FPL's "16-17".
SEASON_COLS = {
    '2016': '16-17', '2017': '17-18', '2018': '18-19', '2019': '19-20',
    '2020': '20-21', '2021': '21-22', '2022': '22-23', '2023': '23-24',
    '2024': '24-25', '2025': '25-26',
}


def pl_seasons_by_understat_id(path=None):
    """understat id (str) -> set of Understat season labels played in the PL."""
    path = path or os.path.join(CACHE, 'idmap-Master.csv')
    out = {}
    with open(path, encoding='utf-8-sig') as f:
        for row in csv.DictReader(f):
            uid = (row.get('understat') or '').strip()
            if not uid:
                continue
            seasons = {s for s, col in SEASON_COLS.items()
                       if (row.get(col) or '').strip()}
            if seasons:
                out.setdefault(uid, set()).update(seasons)
    return out


# ⚠️ Understat seasons before 2016 have no column in the id map, so they cannot
# be confirmed as Premier League and are dropped. That is the conservative
# direction — it discards real PL football from 2014-15 and 2015-16 rather than
# admitting foreign football — and it is why any figure from these scripts is
# quoted as "2016-17 onward" rather than "career".
EARLIEST = '2016'
