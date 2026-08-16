#!/usr/bin/env python3
"""Pair two cells files produced by two separate replay PROCESSES.

`FPL_NO_XG_REPAIR`, `FPL_NO_XG_AGGREGATE` and `FPL_NO_XGC_REPAIR` act during
`Load`, and `runPolicySweep` calls `loadPairs` once before the variant loop into
a process-global season cache. So they cannot be sweep arms: an arm setting one
inside its `apply` replays the same already-parsed season in both arms and the
sweep reports a tight null on exactly the thing it was built to measure. The two
data states therefore have to be two processes, and the pairing has to happen
afterwards — which is what this does.

This is **plumbing, not statistics**. It rewrites two cells files into one so
that `stats/sweep_inference.R` can pair them the way it pairs any other sweep,
keyed on (run_id, sweep) as the block and (season, start_gw) as the cell. No
standard error, no t and no verdict is computed here; that separation is the
project's rule and it is not being bent.

Usage:

    pair_cells.py --baseline off.csv --baseline-label "..." \\
                  --arm on.csv --arm-label "..." \\
                  --sweep NAME --out paired.csv [--only-sweep SUBSTR]

Two properties are checked rather than assumed, because a pairing that silently
drops or duplicates a cell is exactly the "plausible number that measured
nothing" failure this package is a catalogue of:

  * both files must carry the identical set of (sweep-ordinal, variant_index,
    season, start_gw) keys — a missing cell means one process was killed;
  * the output must contain exactly twice as many rows as either input.

It also prints the per-cell disagreement census, which is the confinement check:
how many cells differ at all, and in which seasons.

**No `.means.csv` is written.** `sweep_inference.R` re-derives the means from the
cells and, when a means file is present, asserts its own arithmetic against Go's
by `stop()`. Hand-writing one here would supply the check's own answer to it and
turn a pipeline test into a tautology, so the script is deliberately left with
nothing to check against and says so in its output.
"""

import argparse
import csv
import sys
from collections import Counter, defaultdict

# The columns that carry a per-cell outcome. A difference in any of these is a
# difference in the cell; the rest are identity or provenance.
OUTCOME = [
    "policy_points", "hold_points", "moves", "hits",
    "policy_per_gw", "hold_per_gw",
    "frozen_points", "frozen_per_gw",
    "frozen_captain_points", "frozen_captain_per_gw",
    "weekly_points", "weekly_per_gw",
    "hold_fixedcap_points", "hold_fixedcap_per_gw",
    "hold_nocap_points", "hold_nocap_per_gw",
]


def read(path, only_sweep=None):
    with open(path, newline="") as f:
        rows = list(csv.DictReader(f))
    if only_sweep:
        rows = [r for r in rows if only_sweep in r["sweep"]]
    return rows


def key(r):
    # Keyed on the sweep's ORDINAL rather than its label. `sweepLabel` is
    # "<EXP or test name>#<ordinal>", and the two processes may have been given
    # different EXP labels — they were, so that each run's own provenance sidecar
    # names its data state. What has to match is the *position*: the same blocks
    # in the same order, which is what the ordinal counts. variant_index
    # distinguishes the arms inside a block.
    #
    # This is checked, not trusted: main() refuses to pair two files whose key
    # sets differ, so a mismatch in block order is a hard error rather than a
    # silently short pairing.
    return (r["sweep"].rsplit("#", 1)[-1], r["variant_index"],
            r["season"], r["start_gw"])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--baseline", required=True)
    ap.add_argument("--arm", required=True)
    ap.add_argument("--baseline-label", required=True)
    ap.add_argument("--arm-label", required=True)
    ap.add_argument("--sweep", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--only-sweep", default=None,
                    help="keep only rows whose sweep label contains this")
    ap.add_argument("--only-variant-index", default=None,
                    help="keep only rows at this variant_index")
    ap.add_argument("--seasons", default=None,
                    help="comma-separated played seasons to keep. Use this to "
                         "restrict to the cells an intervention can reach: "
                         "pooling over cells where it is structurally inert "
                         "halves the estimate and reports the dilution as a "
                         "smaller effect")
    args = ap.parse_args()

    base = read(args.baseline, args.only_sweep)
    arm = read(args.arm, args.only_sweep)
    if args.only_variant_index is not None:
        base = [r for r in base if r["variant_index"] == args.only_variant_index]
        arm = [r for r in arm if r["variant_index"] == args.only_variant_index]
    if args.seasons:
        keep = set(s.strip() for s in args.seasons.split(","))
        base = [r for r in base if r["season"] in keep]
        arm = [r for r in arm if r["season"] in keep]

    bk = {key(r): r for r in base}
    ak = {key(r): r for r in arm}
    if len(bk) != len(base) or len(ak) != len(arm):
        sys.exit("duplicate keys: %d/%d baseline, %d/%d arm"
                 % (len(bk), len(base), len(ak), len(arm)))
    if set(bk) != set(ak):
        missing = sorted(set(bk) ^ set(ak))[:10]
        sys.exit("the two files do not cover the same cells; %d differ, e.g. %s"
                 % (len(set(bk) ^ set(ak)), missing))

    # --- the confinement census ------------------------------------------
    differ = defaultdict(set)
    same = defaultdict(set)
    for k in bk:
        b, a = bk[k], ak[k]
        moved = any(b[c] != a[c] for c in OUTCOME if c in b and b[c] != "")
        (differ if moved else same)[k[0]].add((k[2], k[3]))
    # Which arms moved, named. A census by season answers "where", and this
    # answers "in which comparison" — the two together are the confinement check.
    per_block_moved = {k: len(v) for k, v in differ.items()}
    seasons_moved = Counter()
    seasons_still = Counter()
    for s, cells in differ.items():
        for season, _ in cells:
            seasons_moved[season] += 1
    for s, cells in same.items():
        for season, _ in cells:
            seasons_still[season] += 1

    print("per-cell census over %d rows a side (%d blocks)"
          % (len(bk), len(set(k[0] for k in bk))))
    print("  cells that DIFFER, by season:")
    for season in sorted(set(seasons_moved) | set(seasons_still)):
        print("    %-10s differ %4d   identical %4d"
              % (season, seasons_moved.get(season, 0), seasons_still.get(season, 0)))
    print("  totals: differ %d, identical %d"
          % (sum(seasons_moved.values()), sum(seasons_still.values())))
    if len(per_block_moved) > 1 or len(bk) > 40:
        print("  cells that differ, by block ordinal / variant index:")
        for k in sorted(per_block_moved):
            print("    ordinal %-4s %3d" % (k, per_block_moved[k]))

    # --- write the paired file -------------------------------------------
    fields = list(base[0].keys())
    out = []
    for src, label, idx, is_base in ((base, args.baseline_label, "0", "true"),
                                     (arm, args.arm_label, "1", "false")):
        for r in src:
            q = dict(r)
            # One block, so R pairs across the two processes. The original
            # per-block ordinal is folded into the sweep label so several blocks
            # in one file stay separate rather than pooling.
            # The block name must be derived from the ORDINAL, not from the two
            # files' own sweep labels: those differ whenever the two processes
            # were given different EXP labels, and R blocks on (run_id, sweep),
            # so keeping either label would put the two arms in different blocks
            # and silently pair nothing. That is exactly the "clean tight null on
            # the thing it was built to measure" failure this whole script exists
            # to avoid, so it is worth a comment rather than a clever expression.
            q["sweep"] = "%s#%s:v%s" % (args.sweep, r["sweep"].rsplit("#", 1)[-1],
                                        r["variant_index"])
            q["run_id"] = "paired"
            q["variant"] = label
            q["variant_index"] = idx
            q["is_baseline"] = is_base
            out.append(q)

    if len(out) != 2 * len(base):
        sys.exit("row count check failed: %d out, %d expected" % (len(out), 2 * len(base)))

    with open(args.out, "w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        w.writerows(out)
    print("wrote %s: %d rows, %d blocks" % (args.out, len(out),
                                            len(set(r["sweep"] for r in out))))
    print("no .means.csv is written; sweep_inference.R will say it cannot check "
          "Go's means, which is correct — Go never computed this pairing.")


if __name__ == "__main__":
    main()
