#!/usr/bin/env python3
"""Fail a pull request whose runtime or memory regressed, against a bar this run measured for itself.

    scripts/perf-gate.py --base base.txt --control control.txt --head head.txt \
        [--message "$(git log -1 --format=%B)"]

# Why there are THREE arms and not two

The obvious gate runs the benchmarks on `base` and on `head` and fails when
benchstat calls a difference significant. Measured on this repository before this
script was written, that gate **fails on a no-op change**. Comparing the same code
to itself, at benchstat's default alpha of 0.05:

    GreedyFill/pool200   +4.82%  (p=0.004)
    GreedyFill/pool600   +5.68%  (p=0.002)
    ObjectiveScratch     -6.08%  (p=0.041)
    geomean             +13.05%

Three of five benchmarks "significant", on identical code. The first cause was
that all six base samples ran before all six head samples, so **run order was
confounded with the arm** and every thermal or scheduling drift loaded onto head.
Interleaving the runs fixes the aggregate — geomean went to -0.25%, which is the
right answer for identical code — and does NOT fix the per-benchmark verdicts:

    GreedyFill/pool200   -3.33%  (p=0.015)
    GreedyFill/pool600   -4.14%  (p=0.015)
    ObjectiveWith       +14.63%  (p=0.009)

Still three of five, still on identical code. The samples are not i.i.d.: each
`go test` invocation carries its own warm-up and GC state, so repeated
invocations differ systematically in a way the significance test does not model.

⚠️ **So a bar asserted in advance is the wrong instrument, and a bar of "p < 0.05"
is a bar of zero.** This project's own rule is that a result below the detection
threshold of its own comparison is not a result — and nothing had ever measured
what that threshold IS for these benchmarks on the machine actually running them.

`control` is a second run of the base commit. `base` vs `control` is a comparison
whose true answer is known to be zero, so **every difference it reports is that
runner's noise, measured on that runner, in that job, under that load**. The bar
for `head` is set above it. A gate that has not shown it can tell "no change" from
a change has not earned the right to block a merge.

# What is gated hard, and what is not

Memory is deterministic and runtime is not — measured, not assumed. Across three
local runs of the same code, `B/op` and `allocs/op` were identical to the byte
while `sec/op` moved 21%. So:

  - **B/op and allocs/op**: any significant regression fails. These do not drift.
    ⚠️ If the NULL shows a significant memory difference, that is not noise to be
    absorbed into a bar — it means the benchmark itself is nondeterministic, and
    this script says so rather than quietly widening the bar to cover it.
  - **sec/op**: fails only when significant AND larger than the measured null bar.

# The override

A `Perf-moved:` trailer in the commit message, mirroring the `Figures-moved:`
trailer the accuracy gate already uses. One trailer convention, not two.

    Perf-moved: the DP seed pass now allocates per-tier instead of once; +18%
    allocs is the price of dropping the O(n^2) rescan, and it is intended.
"""

import argparse
import csv
import io
import re
import subprocess
import sys

# Units benchstat emits, and how this gate treats each.
MEMORY_UNITS = ("B/op", "allocs/op")
TIME_UNITS = ("sec/op",)

# ⚠️ A floor under the measured bar, not a substitute for it. The null run is
# itself a sample and a quiet runner can report a suspiciously small bar; this
# stops a lucky null from making the gate hair-trigger. It is NOT the bar when the
# null measures something larger.
MIN_TIME_BAR_PCT = 5.0

# ⚠️ Memory gets a measured bar too, and an earlier version of this file did not
# give it one. It hard-failed on ANY significant memory move under the comment
# "memory is deterministic, so this is real". That premise was measured on some of
# these benchmarks and generalised to all of them, and it is false:
# `BenchmarkDPSeeds/pool600/tight` moves memory between two runs of ONE commit,
# on every draw taken. Over 20 runs in three draws: B/op 10726546..10727168, a
# spread of ~620 bytes in 10.7MB (~0.006%), and allocs/op 24247..24250.
#
# ⚠️ Those are OBSERVED extremes over 20 runs, not a bound. Each of the three
# draws escaped the previous draw's range at one end or both, which is the point:
# the quantity is not deterministic, so no number of samples establishes a
# maximum. The load-bearing claim is the ORDER of the jitter — ~0.006% against a
# 1.0% tolerance — and that is stable across every draw.
#
# The gate therefore failed on main's own null on the first pull request after it
# merged — it was not measuring the thing it assumed.
#
# The fix is the one the runtime bar already uses: let the null say what this
# runner and these benchmarks actually do, PER BENCHMARK. A deterministic
# benchmark earns a 0% bar and keeps the strict rule, which is most of them; one
# that jitters earns exactly its own jitter and no more.
#
# ⚠️ The ceiling is what stops that becoming an excuse. A null memory move above
# it is not jitter to be tolerated — that benchmark cannot gate memory at all, and
# it is reported loudly rather than silently given a wider bar. ~0.006% against
# 1.0% is more than two orders of magnitude of headroom, and a real memory
# regression is far larger still, so the bite is intact.
#
# ⚠️ **Do NOT read "over the ceiling" as "there is a bug in the benchmark."** An
# earlier version printed "Fix the benchmark", which sends a reader looking for a
# line of code that does not exist. Memory drifts across PROCESSES with no project
# code involved. A standalone module containing nothing but `map[int]int` taking
# 20,000 fixed keys, 24 separate processes, go1.26.5, `-benchtime=1x`:
#
#     18 runs   1182584 B/op   144 allocs/op
#      6 runs   1182600 B/op   145 allocs/op
#
# One extra allocation about a quarter of the time, on code with no logic in it.
# Any benchmark that builds maps inherits this.
#
# ⚠️ **`-benchtime=1x` IS THE MEASUREMENT, not a detail.** Under the default,
# Go picks `b.N` per process (1488..1662 across five runs here) and reports
# total/N — so the SAME experiment shows a spurious ~10-byte B/op wobble from
# integer division while hiding the real 144/145 split entirely. Two separate
# attempts at this measurement, taken with the default, drew opposite and equally
# wrong conclusions before the third pinned `b.N`. If you re-measure this, pin it.
#
# ⚠️ Version-pinned deliberately: this concerns runtime internals, so an unversioned
# claim about it rots at the next toolchain bump. Measured on go1.26.5/arm64; CI
# runs amd64, untested here.
#
# So a ceiling breach reads "this benchmark's memory is too unstable to gate with"
# — NOT "someone introduced a defect".
MEM_NULL_TOLERANCE_PCT = 1.0


def run_benchstat(a, b):
    """A/B two benchmark files and return rows as (unit, name, delta_pct, p, significant)."""
    out = subprocess.run(
        ["benchstat", "-format", "csv", a, b],
        capture_output=True, text=True,
    )
    if out.returncode != 0:
        sys.exit(f"benchstat failed: {out.stderr.strip()}")

    rows, unit = [], None
    for rec in csv.reader(io.StringIO(out.stdout)):
        if not rec or not any(rec):
            continue
        # A unit header looks like: ,sec/op,CI,sec/op,CI,vs base,P
        if rec[0] == "" and len(rec) > 1 and rec[1]:
            unit = rec[1]
            continue
        if unit is None or rec[0].startswith(("goos", "goarch", "pkg", "cpu")):
            continue
        if rec[0] == "geomean":
            name = "geomean"
        else:
            name = rec[0]
        if len(rec) < 7:
            continue
        delta_s, p_s = rec[5].strip(), rec[6].strip()
        pm = re.search(r"p=([0-9.]+)", p_s)
        p = float(pm.group(1)) if pm else None

        # "~" means benchstat found no significant difference.
        if delta_s in ("", "~"):
            rows.append((unit, name, 0.0, p, False))
            continue

        m = re.match(r"([+-]?[0-9.]+)%", delta_s)
        if m:
            rows.append((unit, name, float(m.group(1)), p, True))
            continue

        # ⚠️ "?" means SIGNIFICANT with an undefined percentage, and it is the
        # commonest shape a memory regression takes: benchstat cannot divide by a
        # zero baseline, so a benchmark going from 0 to 2048 B/op reports
        # `?  p=0.008` rather than a number.
        #
        # The first version of this parser skipped any row whose delta did not
        # match a percentage, so it silently dropped exactly that row — and the
        # gate passed a change that added an allocation to a zero-allocation hot
        # path, which is the single thing it was built to stop. Caught by
        # injecting a real regression and watching the gate say "no regression".
        # A gate is not proved by its null passing; it is proved by its bite.
        base_v, head_v = to_float(rec[1]), to_float(rec[3])
        if base_v is None or head_v is None:
            rows.append((unit, name, 0.0, p, False))
            continue
        if base_v == 0:
            # Undefined as a ratio, unambiguous as a direction.
            delta = float("inf") if head_v > 0 else 0.0
        else:
            delta = (head_v - base_v) / base_v * 100.0
        rows.append((unit, name, delta, p, head_v != base_v))
    return rows


def to_float(x):
    try:
        return float(x)
    except (TypeError, ValueError):
        return None


def bar_from_null(null_rows):
    """What this runner earned: a runtime bar, per-benchmark memory bars, and the
    benchmarks whose memory is too unstable to gate with at all."""
    worst = 0.0
    for unit, name, delta, _p, sig in null_rows:
        if unit in TIME_UNITS and sig and name != "geomean":
            worst = max(worst, abs(delta))

    mem_bars, broken = {}, []
    for unit, name, delta, _p, sig in null_rows:
        if unit not in MEMORY_UNITS or not sig or name == "geomean":
            continue
        # inf is "moved off a zero baseline" — never jitter, always a real change,
        # so it can never earn a bar.
        if delta == float("inf") or abs(delta) > MEM_NULL_TOLERANCE_PCT:
            broken.append((unit, name, delta))
            continue
        mem_bars[(unit, name)] = abs(delta)

    return max(worst, MIN_TIME_BAR_PCT), mem_bars, broken


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--control", required=True, help="a SECOND run of base: the null")
    ap.add_argument("--head", required=True)
    ap.add_argument("--message", default="", help="commit message, read for a Perf-moved: trailer")
    args = ap.parse_args()

    null_rows = run_benchstat(args.base, args.control)
    cand_rows = run_benchstat(args.base, args.head)

    bar, mem_bars, broken = bar_from_null(null_rows)

    print("PERFORMANCE GATE")
    print(f"  runtime bar measured from this runner's own null: {bar:.1f}%"
          f"{'  (floor)' if bar == MIN_TIME_BAR_PCT else ''}")
    if mem_bars:
        print(f"  memory bars measured from the same null, per benchmark "
              f"(tolerance {MEM_NULL_TOLERANCE_PCT:.1f}%):")
        for (unit, name), b in sorted(mem_bars.items()):
            print(f"       {name}  {unit}  {b:.3f}%")
    else:
        print("  memory: the null moved none of it, so every memory bar is 0%")

    if broken:
        print("\n  ⚠️ THE NULL MOVED MEMORY BY MORE THAN THE TOLERANCE. This benchmark")
        print("     cannot gate memory, and widening its bar to fit would hide whatever is")
        print("     doing it. ⚠️ Not necessarily a defect: memory drifts across processes")
        print("     even with no project code — see MEM_NULL_TOLERANCE_PCT. Investigate")
        print("     before assuming there is a line of code to fix:")
        for unit, name, delta in broken:
            how = "off a zero baseline" if delta == float("inf") else f"{delta:+.2f}%"
            print(f"       {name}  {unit}  {how} between two runs of ONE commit")

    failures = []
    for unit, name, delta, p, sig in cand_rows:
        if name == "geomean" or not sig or delta <= 0:
            continue
        if unit in MEMORY_UNITS:
            mb = mem_bars.get((unit, name), 0.0)
            if delta <= mb:
                continue
            how = "from zero" if delta == float("inf") else f"{delta:+.2f}%"
            why = (f"above this benchmark's {mb:.3f}% null bar" if mb
                   else "this benchmark's memory did not move in the null, so this is real")
            failures.append(f"{name}  {unit}  {how} (p={p})  — {why}")
        elif unit in TIME_UNITS and delta > bar:
            failures.append(f"{name}  {unit}  {delta:+.2f}% (p={p})  — above this runner's {bar:.1f}% null bar")

    # Regressions still count when the null itself is broken; the note above is a
    # separate report, not an excuse to pass.
    if not failures and not broken:
        print("\n  no regression above the measured bar")
        return 0
    if not failures:
        print("\n  no regression above the measured bar, but see the null warning above")
        return 1

    print("\n  REGRESSIONS:")
    for f in failures:
        print(f"    ✗ {f}")

    if re.search(r"^Perf-moved:", args.message, re.MULTILINE):
        print("\n  Overridden by a Perf-moved: trailer. The regression is recorded, not denied.")
        return 0

    print("\n  Add a Perf-moved: trailer saying WHY this is the right trade, or fix it.")
    print("  The trailer is one line and should answer the question a later reader has:")
    print("  not what the commit did, but why the cost moved and whether that was intended.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
