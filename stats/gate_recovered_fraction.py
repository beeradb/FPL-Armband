"""Fieller and delta-method interval for the recovered fraction, season-clustered.

The cells file is an ARGUMENT. It used to be a hardcoded path to the 2026-08-14
snapshot, which is how this script came to print a figure from an instrument that
had changed under it, with nothing in its output saying so -- the warning that
occupied this docstring for two commits. That failure is now structural rather than
remembered: there is no default, and the input path is printed with the result.

(!) WHAT THIS COMPUTES AND WHAT IT CANNOT. The recovered fraction is arm 2 (perfect
on realised UNDERLYING) over arm 1 (perfect on POINTS). It is defined for that pair
ONLY. The `for arm in (1, 2)` loop is therefore not a limitation to be generalised
away: arm 3, the RESIDUAL gate, is a COMPONENT rather than a criterion approximating
another, so a ratio against arm 1 is mechanically computable and semantically wrong,
and a reader would add the two fractions and get nonsense. The residual arm's
contrasts belong in `sweep_inference.R` (pairwise, with CR2) and in
`gate_additivity.R` (mu_X + mu_R - mu_P). Both are committed; the adaptation that
originally produced them was not.

(!) THREE UNSIZED BIASES RUN THROUGH THIS FRACTION, so it is not a lower bound and
not an upper one:
  - xPoints retains realised bonus, saves, cards, minutes and defcon, and bonus is
    paid largely for the goals and assists the residual replaces -- INFLATES it;
  - arm 1 optimises the very quantity both arms are scored on -- DEFLATES it;
  - the conversion scale is fitted IN SAMPLE and season-global, so arm 2's criterion
    enjoys a fit no deployable criterion has -- INFLATES it, and this one is new with
    the scale. The fraction is OPTIMISTIC for anything shippable.

(!) The 50% bar this prints t's against has now failed TWICE to be a decision rule:
Fieller [0.426, 0.835] on 2026-08-14 and [0.325, 0.813] on the scaled instrument,
both straddling 0.50 while rejecting 0.89 and 1.00. The design is unchanged between
them and the interval got WIDER, so read a straddle as `unresolved` and not as
`nearly resolved`.
"""
import csv
import math
import sys
from collections import defaultdict

# The cells file is an ARGUMENT and has no default, deliberately. It used to be a
# hardcoded path to one banked snapshot, which is how this script came to print a
# figure from a superseded instrument with nothing in its output saying so. A
# default would put that failure back: the caller who forgets the argument is
# exactly the caller who does not know which bank they are reading.
if len(sys.argv) != 2:
    sys.exit("usage: gate_recovered_fraction.py <cells.csv>\n"
             "  e.g. stats/snapshots/2026-08-15-gatescaled/cells/gatescaled.csv")
rows = list(csv.DictReader(open(sys.argv[1])))
if not rows:
    sys.exit("no rows")

# (!) EXACTLY ONE (run_id, sweep), asserted before anything is keyed.
#
# `cells_common.R:132` keys a cell on `run_id + sweep + season + start_gw`, and
# AGENTS.md records why: "the divergence that bit was the cell KEY". This script
# keyed on (variant_index, season, start_gw) alone, which was safe only by
# accident while the input path was hardcoded to one bank. Making the path an
# argument -- the repair directly above -- turned that accident into a live
# defect: concatenate two banks and the later rows silently overwrite the earlier
# ones, and the script prints a blend of two instruments as one figure. Verified
# by review: gatescaled + gateresidual concatenated printed the SUPERSEDED 0.6450
# and [0.426, 0.835] with no warning.
blocks = {(r['run_id'], r['sweep']) for r in rows}
if len(blocks) != 1:
    sys.exit(f"CONTRACT VIOLATION: {len(blocks)} (run_id, sweep) blocks in one file:\n"
             + "\n".join(f"  {rid} / {sw}" for rid, sw in sorted(blocks))
             + "\nThis script computes a ratio of two arms WITHIN one block. Cross-run\n"
               "pairing against another block's baseline is the recorded cell-KEY\n"
               "divergence. Split the file, or read the two banks separately.")

by = {}
for r in rows:
    by[(r['variant_index'], r['season'], r['start_gw'])] = float(r['policy_per_gw'])

seasons = sorted({r['season'] for r in rows})
starts = sorted({r['start_gw'] for r in rows}, key=int)

# per-season mean paired difference against the baseline, for each oracle arm
sm = {1: [], 2: []}
for s in seasons:
    for arm in (1, 2):
        d = [by[(str(arm), s, g)] - by[('0', s, g)] for g in starts]
        sm[arm].append(sum(d) / len(d))

n = len(seasons)


def mean(x):
    return sum(x) / len(x)


mx, my = mean(sm[1]), mean(sm[2])          # 1 = points arm, 2 = underlying arm
vx = sum((a - mx) ** 2 for a in sm[1]) / (n - 1) / n
vy = sum((a - my) ** 2 for a in sm[2]) / (n - 1) / n
cxy = sum((a - mx) * (b - my) for a, b in zip(sm[1], sm[2])) / (n - 1) / n

theta = my / mx
# delta method on a ratio
se = abs(theta) * math.sqrt(vy / my**2 + vx / mx**2 - 2 * cxy / (mx * my))
# t_crit is DERIVED from the seasons actually present, not pinned at df 5.
#
# (!) This was `tc = 2.571  # t_crit at df 5` beside a computed `n` that was never
# used, and it was correct only because the input path was hardcoded to a
# six-season bank. The argv repair above removed that guarantee: on a four-season
# subset the script printed "H0 theta=0.89: REJECT at df 5" where df is 3 and
# t_crit is 3.182, so the rejection was wrong and both intervals were too narrow.
# A small table rather than scipy, which is not a dependency here.
T_CRIT_975 = {1: 12.706, 2: 4.303, 3: 3.182, 4: 2.776, 5: 2.571,
              6: 2.447, 7: 2.365, 8: 2.306, 9: 2.262, 10: 2.228}
df = n - 1
if df not in T_CRIT_975:
    sys.exit(f"CONTRACT VIOLATION: {n} seasons gives df {df}, outside the table "
             f"({min(T_CRIT_975)}-{max(T_CRIT_975)}). Add the value rather than "
             f"falling back to a constant.")
tc = T_CRIT_975[df]

# The input travels with the figure. A recovered fraction is meaningless without
# the bank it came from -- this script printed one for two commits after the
# instrument under it changed.
print(f"cells: {sys.argv[1]}")
# `rows` and `cells` are different counts and the record's thresholds are all on
# cells. Printing rows under the name "cells" would also hide the one symptom that
# distinguishes a cross-block concatenation from a clean run.
n_arms = len({r['variant_index'] for r in rows})
cells = len(seasons) * len(starts)
if len(rows) != n_arms * cells:
    sys.exit(f"CONTRACT VIOLATION: {len(rows)} rows is not {n_arms} arms x "
             f"{cells} cells ({len(seasons)} seasons x {len(starts)} starts).")
print(f"seasons: {n}   starts: {len(starts)}   arms: {n_arms}   "
      f"cells: {cells}   rows: {len(rows)}   df: {df}   t_crit: {tc}")
print()
print(f"points arm   mean {mx:+.4f} pts/gw   season means {[round(v,3) for v in sm[1]]}")
print(f"underlying   mean {my:+.4f} pts/gw   season means {[round(v,3) for v in sm[2]]}")
print(f"correlation between arms across seasons: "
      f"{cxy/math.sqrt(vx*vy):+.3f}")
print()
print(f"recovered fraction  {theta:.4f}")
print(f"delta-method SE     {se:.4f}")
print(f"delta 95% CI        [{theta-tc*se:.3f}, {theta+tc*se:.3f}]")

# Fieller — the correct interval for a ratio
a = mx**2 - tc**2 * vx
b = -2 * (mx * my - tc**2 * cxy)
c = my**2 - tc**2 * vy
disc = b * b - 4 * a * c
if disc > 0 and a > 0:
    lo = (-b - math.sqrt(disc)) / (2 * a)
    hi = (-b + math.sqrt(disc)) / (2 * a)
    print(f"Fieller 95% CI      [{lo:.3f}, {hi:.3f}]")
else:
    print("Fieller: unbounded (denominator not significantly non-zero)")

print()
for bar in (0.50, 0.89, 1.00):
    t = (my - bar * mx) / math.sqrt(vy + bar**2 * vx - 2 * bar * cxy)
    print(f"H0 theta={bar:.2f}: t={t:+.2f}  "
          f"{'REJECT' if abs(t) > tc else 'cannot reject'} at df {df}")

print()
print("PER SEASON — read this before building on the pooled level.")
print()
# ⚠️ Which seasons feed xPoints' four replaced channels from a backfill rather than
# from FPL's own columns. `xgRepairs` covers 2020-21 and 2021-22 in full and 2022-23
# for GW1-15, all under ONE borrowed offset fitted on 2022-23's overlap window; the
# xGC there is reconstructed rather than harvested, which the record prices at a
# 16-20% ever-present error against 3.0-5.2% FPL-fed.
#
# This is not a claim that those cells are wrong. It is the record's own rule that a
# defect costing unevenly across seasons invents shapes rather than adding noise, so
# an instrument whose QUALITY differs by season must be read per season at least once.
BACKFILLED = {'2020-21': 'full, borrowed offset',
              '2021-22': 'full, borrowed offset',
              # ⚠️ NOT "borrowed". 2022-23's sidecar says `in-season` — it is the
              # season the borrowed offset was FITTED FROM, not a consumer of one.
              '2022-23': 'GW1-15, in-season offset'}
print(f"  {'season':<9} {'points':>8} {'underlying':>11} {'ratio':>7}   underlying source")
for s, a, b in zip(seasons, sm[1], sm[2]):
    src = BACKFILLED.get(s)
    note = f"⚠️ backfilled ({src})" if src else "FPL-fed"
    print(f"  {s:<9} {a*38:>8.1f} {b*38:>11.1f} {b/a:>7.2f}   {note}")

print()
# ⚠️ The two group MEANS this block used to print are gone, and the reason is not
# that they were underpowered.
#
# The three backfilled seasons are exactly the three OLDEST and the three FPL-fed
# exactly the three NEWEST, so "backfill" is perfectly collinear with era — the 2-vs-5
# transfer bank, defcon arriving in 2025-26, three bonus regimes, 2020-21's schedule.
# That is not a power problem and more cells do not fix it: the grouping cannot
# identify the backfill at ANY sample size. Saying "df 2" implied otherwise.
#
# They were also printed on the wrong quantity. This section's subject is the RATIO,
# and the group means were levels; on the ratio the groups overlap completely
# (on the scaled instrument, backfilled 0.65/0.81/0.46 against FPL-fed
# 0.51/0.33/0.80 -- the pre-scale run read 0.71/0.62/0.53 and 0.86/0.34/0.83, and
# the overlap is the point in both).
print("  ⚠️ Do NOT read the backfill annotation as a contrast. The three backfilled")
print("  seasons are the three OLDEST and the three FPL-fed the three NEWEST, so the")
print("  split is collinear with era — transfer bank, defcon, three bonus regimes,")
print("  2020-21's schedule. No sample size identifies the backfill from this.")
print("  The contrast that CAN is FPL_NO_XG_REPAIR=1 on these same cells, where era")
print("  cancels within season. Until then read the column as the spread the pooled")
print("  figure averages over, which is the estimand. The spread is printed above")
print("  rather than quoted here: a literal range in this string went stale the")
print("  first time the instrument moved, and said 0.34-0.86 where the scaled")
print("  instrument gives 0.33-0.81.")
print()
print("  ⚠️ The 'FPL-fed' label is the cell's OWN season only: a 2023-24 cell's priors")
print("  read 2022-23, which is backfilled for GW1-15.")
