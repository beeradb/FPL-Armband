import json, gzip, glob, collections, statistics, os

# Paths come from the environment, with a repo-relative default for repo content and a
# home-relative one for build products. They used to be absolute paths into one
# machine's home directory, which made this script unrunnable by anybody else and put a
# private filesystem layout into a public repository.
#
# The captures default to the ones this repo already tracks. The original pointed into a
# throwaway worktree named prior-blend-experiment, which held the same data.
CAP = os.environ.get('FLAGCAL_CAPTURES', 'data/captures').rstrip('/') + '/'
# The backtest cache is repo-relative, NOT a home cache. %s is the season.
#
# ⚠️ This defaulted to ~/.cache/fpl/ for one commit and that was wrong — it broke the
# script. The path it replaced was `/home/<user>/fpl/.cache/fpl/...`, where `fpl` is the
# CHECKOUT DIRECTORY, so `.cache/` sits inside the working tree and never in $HOME.
# Reading a home directory out of an absolute path is easy to get backwards; the giveaway
# is that the segment after the user is a repo name.
CACHE = os.environ.get('FLAGCAL_CACHE', '.cache/fpl/backtest-v8-%s.json')
SEASONS = ['2021-22', '2022-23', '2023-24', '2024-25', '2025-26']  # cached archive seasons

# 1. flags: season -> gw -> code -> (chance, status)
flags = {}
dup = collections.defaultdict(list)  # byte-identical repeat crawls
for s in SEASONS:
    flags[s] = {}
    for d in sorted(glob.glob(CAP + s + '/GW*')):
        man = json.load(open(d + '/manifest.json'))
        gw = man['event']
        dup[(s, man['backfill']['wayback_timestamp'])].append(gw)
        body = json.load(gzip.open(d + '/bootstrap-static.json.gz'))
        flags[s][gw] = {int(e['code']): (e['chance_of_playing_next_round'], e['status'])
                        for e in body['elements']}
repeat = {s: set() for s in SEASONS}
for (s, ts), gws in dup.items():
    if len(gws) > 1:
        for g in gws:
            repeat[s].add(g)

# 2. outcomes
mins = {}
pos = {}
for s in SEASONS:
    d = json.load(open(CACHE % s))
    mins[s] = {}
    pos[s] = {}
    for p in d['players'].values():
        c = int(p['code'])
        pos[s][c] = p['element_type']
        mins[s][c] = {int(g): v['minutes'] for g, v in p.get('gws', {}).items()}

# 3. pair. Baseline = mean minutes over PRIOR unflagged gameweeks, rolling, this season.
POSN = {1: 'GKP', 2: 'DEF', 3: 'MID', 4: 'FWD'}
rows = []
for s in SEASONS:
    for c in mins[s]:
        hist = []
        for gw in sorted(flags[s]):
            f = flags[s][gw].get(c)
            if f is None:
                continue
            chance, status = f
            m = mins[s][c].get(gw)
            unflagged = (status == 'a' and (chance is None or chance == 100))
            if m is not None and len(hist) >= 3:
                base = sum(hist) / len(hist)
                if base >= 30:
                    rows.append(dict(season=s, gw=gw, code=c, chance=chance, status=status,
                                     mins=m, base=base, ratio=m / base,
                                     pos=POSN[pos[s][c]], repeat=gw in repeat[s]))
            if unflagged and m is not None:
                hist.append(m)
                if len(hist) > 8:
                    hist.pop(0)

nrep = sum(1 for r in rows if r['repeat'])
print('paired observations: %d   (dropping %d from repeat-crawl gameweeks)' % (len(rows), nrep))
rows = [r for r in rows if not r['repeat']]
print()


def key(c):
    return 'null' if c is None else str(int(c))


g = collections.defaultdict(list)
for r in rows:
    g[key(r['chance'])].append(r)
print('%-6s %8s %9s %9s %11s' % ('flag', 'n', 'mean', 'median', 'blanked'))
for k in ['0', '25', '50', '75', '100', 'null']:
    v = g.get(k, [])
    if not v:
        continue
    rs = [x['ratio'] for x in v]
    print('%-6s %8d %9.3f %9.3f %10.1f%%' % (k, len(v), sum(rs) / len(rs),
          statistics.median(rs), 100 * sum(1 for x in v if x['mins'] == 0) / len(v)))
