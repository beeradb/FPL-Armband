# This script is stage 2: it reuses stage 1's bindings (SEASONS, g, ...) rather than
# recomputing them, which is why it execs the file instead of importing it.
#
# ⚠️ It used to exec an absolute path into an agent job's temp directory
# (~/.claude/jobs/<id>/tmp/flagcal.py). That directory is ephemeral and is long gone, so
# this script had been DEAD as well as machine-specific — it could not run for its own
# author either. It now execs its sibling, which is the same file, tracked.
import os
exec(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), 'flagcal.py')).read())

print('\n--- does it replicate? mean realised ratio by season ---')
print('%-6s %s' % ('flag', ''.join('%10s' % s for s in SEASONS)))
for k in ['0', '25', '50', '75', '100', 'null']:
    line = '%-6s' % k
    for s in SEASONS:
        v = [x['ratio'] for x in g.get(k, []) if x['season'] == s]
        line += '%10s' % ('%.3f' % (sum(v) / len(v)) if len(v) >= 20 else '-')
    print(line)

print('\n--- is the flag simply over-optimistic by a power law? ---')
print('%-6s %9s %9s %9s' % ('flag', 'measured', 'model now', 'flag^2'))
for k in ['25', '50', '75', '100']:
    v = [x['ratio'] for x in g.get(k, [])]
    p = int(k) / 100
    print('%-6s %9.3f %9.2f %9.3f' % (k, sum(v) / len(v), p, p * p))

print('\n--- both tails ---')
z = g.get('0', [])
print('flagged 0%% who played at all: %d of %d (%.1f%%), mean %.0f minutes when they did'
      % (sum(1 for x in z if x['mins'] > 0), len(z),
         100 * sum(1 for x in z if x['mins'] > 0) / len(z),
         statistics.mean([x['mins'] for x in z if x['mins'] > 0])))
h = g.get('100', [])
print('flagged 100%% who blanked:     %d of %d (%.1f%%)'
      % (sum(1 for x in h if x['mins'] == 0), len(h),
         100 * sum(1 for x in h if x['mins'] == 0) / len(h)))

print('\n--- by position (mean realised ratio) ---')
print('%-6s %s' % ('flag', ''.join('%8s' % p for p in ['GKP', 'DEF', 'MID', 'FWD'])))
for k in ['25', '50', '75', '100']:
    line = '%-6s' % k
    for p in ['GKP', 'DEF', 'MID', 'FWD']:
        v = [x['ratio'] for x in g.get(k, []) if x['pos'] == p]
        line += '%8s' % ('%.3f' % (sum(v) / len(v)) if len(v) >= 20 else '-')
    print(line)

print('\n--- status codes at flag 0, which the percentage cannot distinguish ---')
st = collections.defaultdict(list)
for x in g.get('0', []):
    st[x['status']].append(x)
for k, v in sorted(st.items(), key=lambda kv: -len(kv[1])):
    if len(v) < 20:
        continue
    print('  status %s: n=%5d  mean ratio %.3f  blanked %.1f%%'
          % (k, len(v), sum(y['ratio'] for y in v) / len(v),
             100 * sum(1 for y in v if y['mins'] == 0) / len(v)))
