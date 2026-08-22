import csv, collections, sys, os

# The gated per-player CSV this reads is a RUN ARTEFACT, not repo content, so there is no
# sensible default and the caller must say where it is:
#
#     python3 scripts/pbdecay.py <pb-gated.csv>          # or PBDECAY_CSV=<path>
#
# ⚠️ It used to be an absolute path into an agent job's temp directory
# (~/.claude/jobs/<id>/tmp/pb-gated.csv). That directory is ephemeral and is long gone, so
# the script had been DEAD as well as machine-specific. Failing loudly with usage beats
# defaulting to a path that silently does not exist.
path = sys.argv[1] if len(sys.argv) > 1 else os.environ.get('PBDECAY_CSV')
if not path:
    sys.exit('usage: pbdecay.py <pb-gated.csv>   (or set PBDECAY_CSV)')
POP = 'the case, injury shaped: he played some of last season'
SHIPPED = 'shipped: prior_half_life 0'
ARM = 'prior_half_life 1'

# (arm, gw) -> [n, sum_pred, sum_act, sum_abs_err]
agg = collections.defaultdict(lambda: [0.0, 0.0, 0.0, 0.0])
for r in csv.DictReader(open(path)):
    if r['population'] != POP or r['target'] != 'points' or r['predictor'] != 'model':
        continue
    if r['category'] != 'all categories':
        continue
    k = (r['variant'], int(r['gw']))
    a = agg[k]
    a[0] += float(r['n'])
    a[1] += float(r['sum_pred'])
    a[2] += float(r['sum_act'])
    a[3] += float(r['sum_abs_err'])

buckets = [(1, 3), (4, 6), (7, 10), (11, 15), (16, 22), (23, 38)]


def stats(arm, lo, hi):
    n = p = a = e = 0.0
    for gw in range(lo, hi + 1):
        v = agg.get((arm, gw))
        if not v:
            continue
        n += v[0]; p += v[1]; a += v[2]; e += v[3]
    if n == 0:
        return None
    return n, (p - a) / n, e / n


print('Does the advantage decay as current-season football accumulates?')
print('Population: %s\n' % POP)
print('%-10s %9s %11s %11s %11s %11s' %
      ('gameweeks', 'n', 'bias ship', 'bias blend', 'bias gain', 'mae gain'))
for lo, hi in buckets:
    s = stats(SHIPPED, lo, hi)
    b = stats(ARM, lo, hi)
    if not s or not b:
        continue
    label = '%d-%d' % (lo, hi)
    # bias is predicted minus actual; negative is under-rating. "gain" is how much
    # of that under-rating the blend removes, so positive means it helped.
    gain = abs(s[1]) - abs(b[1])
    print('%-10s %9.0f %11.4f %11.4f %11.4f %11.4f'
          % (label, s[0], s[1], b[1], gain, s[2] - b[2]))
