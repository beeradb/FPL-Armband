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

**Both units get a bar measured from this runner's own null.** Runtime's is wide;
memory's is very narrow. Neither is asserted in advance, which is the whole idea
above applied consistently rather than to one unit.

  - **sec/op**: fails when significant AND larger than the measured null bar.
  - **B/op and allocs/op**: the same, against their own — much smaller — bar. A
    benchmark going from zero allocations to some still fails outright, because
    that is a direction rather than a ratio and no bar covers it.

⚠️ **This REPLACES the claim that memory does not drift, which was false.** The
docstring used to say `B/op` and `allocs/op` were "identical to the byte" across
three local runs and gate any significant memory difference on that basis. **Go
seeds every map's hash randomly per process**, so the number of overflow buckets
allocated for one fixed key set differs between runs. Measured on a standalone
module containing nothing but a `map[int]int` taking 20,000 fixed keys, four
separate processes gave 145 / 144 / 144 / 145 allocs/op and B/op spanning 67
bytes. Any benchmark that builds a map inherits this.

The consequence was not theoretical: `BenchmarkOptimize/pool600` builds maps and
drifts by about 0.01%, and with five interleaved rounds per arm benchstat has
enough samples to call that significant — so the NULL arm failed, and the gate
returned 1 on pull requests whose diff did not touch the package at all. There
was no override for it either: a `Perf-moved:` trailer covers a regression in the
head, and this was the base disagreeing with itself.

⚠️ **The old text called that "a defect in the benchmark".** It is not. There is
no line of code to attribute it to and no version of `Optimize` that would not
drift; the mistake was generalising "these particular benchmarks did not move" to
"memory does not drift".

# The override

A `Perf-moved:` trailer in the commit message, mirroring the `Figures-moved:`
trailer the accuracy gate already uses. One trailer convention, not two.

    Perf-moved: the DP seed pass now allocates per-tier instead of once; +18%
    allocs is the price of dropping the O(n^2) rescan, and it is intended.
"""

import argparse
import csv
import io
import math
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

# The same floor, for memory, and it is deliberately two orders of magnitude
# tighter than the runtime one.
#
# ⚠️ Sized against what it must NOT hide, not against what it must absorb. The
# drift it exists to tolerate is map-seed jitter, measured at roughly 0.01-0.02%
# on a benchmark allocating ~63,900 times. The regression this gate was built to
# catch is the shape recorded in the override example below: +18% allocs. A floor
# of 0.5% is far above the jitter and far below anything that has ever been called
# a regression here, so it buys reproducibility without buying blindness.
#
# ⚠️ It is a FLOOR and not the bar. When the null moves memory by more than this,
# the null's own figure is used instead — same as runtime.
MIN_MEM_BAR_PCT = 0.5

# The aggregate floor, and it is tighter than the per-row one on purpose.
#
# ⚠️ A per-row bar can be WALKED UNDER: ~0.4% of extra allocations added to every
# benchmark in the package is a set of individually-under-bar rows, each real and
# significant, that passes while costing a multi-percent aggregate. The per-row
# check cannot see that, because it never looks at more than one row.
#
# geomean is far quieter than any single row — a measured null moved its memory
# geomean by +0.00% while an injected 18% regression moved it by +18.00% — so it
# can carry a much tighter floor, which is what makes the walk unprofitable.
MIN_MEM_GEOMEAN_BAR_PCT = 0.25


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


def floor_for(unit):
    return MIN_TIME_BAR_PCT if unit in TIME_UNITS else MIN_MEM_BAR_PCT


def bars_from_null(null_rows):
    """One bar PER UNIT that this runner earned, plus what the null moved.

    ⚠️ **A bar is per unit, not per family.** `B/op` and `allocs/op` do not move
    proportionally — the measurement behind this file's docstring has allocs/op
    moving by one while B/op moves by 67 bytes — so a single shared memory bar
    would let a loud null on one unit widen the tolerance for the other.

    ⚠️ **`geomean` sets no bar.** benchstat prints a percentage for it even when
    every individual row is `~`, because it is a summary rather than a tested
    comparison, and letting it set a bar would let one summary row widen the gate
    for every real one. It is *checked* separately — see geomean_bars.

    ⚠️ **A non-finite delta sets no bar either, and that is a hole this had.** A
    zero-baseline row reports `inf`, and `max(anything, inf)` is `inf`, so one
    from-zero row in the NULL silently raised the memory bar to infinity and
    disabled the magnitude gate for every other benchmark in the suite. It is
    reported instead, where it reads as what it is: the null doing something a
    ratio cannot describe.
    """
    worst, moved, weird = {}, [], []
    for unit, name, delta, _p, sig in null_rows:
        if name == "geomean" or not sig:
            continue
        if unit in MEMORY_UNITS:
            moved.append((unit, name, delta))
        if not math.isfinite(delta):
            weird.append((unit, name))
            continue
        worst[unit] = max(worst.get(unit, 0.0), abs(delta))
    bars = {u: max(worst.get(u, 0.0), floor_for(u)) for u in TIME_UNITS + MEMORY_UNITS}
    return bars, moved, weird


def geomean_bars(null_rows):
    """The aggregate bar per memory unit, measured from the null's own geomean.

    ⚠️ **This exists because a per-benchmark bar can be walked under.** Once
    memory has a 0.5% floor, a change adding ~0.4% of allocations to every
    benchmark in the package produces a set of individually-under-bar rows, each
    real and significant, and passes — while costing a multi-percent aggregate.
    The per-row bar cannot see that by construction, because it never looks at
    more than one row.

    The aggregate is much quieter than any single row — a measured null moved its
    memory geomean by +0.00% while an 18% injected regression moved it by
    +18.00% — so it can carry a far tighter floor, which is what makes the
    split-regression walk unprofitable rather than merely harder.
    """
    worst = {}
    for unit, name, delta, _p, sig in null_rows:
        if name != "geomean" or unit not in MEMORY_UNITS or not sig:
            continue
        if not math.isfinite(delta):
            continue
        worst[unit] = max(worst.get(unit, 0.0), abs(delta))
    return {u: max(worst.get(u, 0.0), MIN_MEM_GEOMEAN_BAR_PCT) for u in MEMORY_UNITS}


def regressions(cand_rows, bars, geo_bars):
    """Every head row that fails, given the bars the null earned.

    Only positive deltas can fail: a benchmark that got faster or allocated less
    is not a regression, and `sig` is benchstat's own judgement rather than this
    script's.
    """
    out = []
    for unit, name, delta, p, sig in cand_rows:
        if not sig or delta <= 0:
            continue
        if name == "geomean":
            if unit in MEMORY_UNITS and delta > geo_bars[unit]:
                out.append(f"{name}  {unit}  {delta:+.2f}%  — above the {geo_bars[unit]:.2f}% "
                           f"aggregate bar; a cost spread thinly across benchmarks is still a cost")
            continue
        if unit in MEMORY_UNITS:
            if not math.isfinite(delta):
                # A direction, not a ratio: no bar covers going from zero.
                out.append(f"{name}  {unit}  from zero (p={p})  — an allocation on a path that had none")
            elif delta > bars[unit]:
                out.append(f"{name}  {unit}  {delta:+.2f}% (p={p})  — above this runner's {bars[unit]:.2f}% memory null bar")
        elif unit in TIME_UNITS and delta > bars[unit]:
            out.append(f"{name}  {unit}  {delta:+.2f}% (p={p})  — above this runner's {bars[unit]:.1f}% null bar")
    return out


def decide(null_rows, cand_rows, message):
    """The whole verdict: exit code plus the lines explaining it.

    ⚠️ **Separated from main() so the selftest can drive the REAL control flow.**
    The first version of the selftest asserted "a null that moved memory is no
    longer a failure by itself" by calling regressions() with an empty candidate
    list, which returns [] whatever the bars are — the check passed for a reason
    unrelated to what it claimed, and would have kept passing if main() regressed
    to the old behaviour. The property is about the verdict, so the verdict is
    what a test has to be able to call.
    """
    lines = []
    bars, moved, weird = bars_from_null(null_rows)
    geo = geomean_bars(null_rows)

    lines.append("PERFORMANCE GATE")
    for u in TIME_UNITS:
        lines.append(f"  runtime bar measured from this runner's own null: {bars[u]:.1f}%"
                     f"{'  (floor)' if bars[u] == floor_for(u) else ''}")
    for u in MEMORY_UNITS:
        lines.append(f"  {u:<9} bar measured from this runner's own null: {bars[u]:.2f}%"
                     f"{'  (floor)' if bars[u] == floor_for(u) else ''}"
                     f"   aggregate {geo[u]:.2f}%")

    if moved:
        # Reported, not failed. Go randomises map hash seeds per process, so a
        # benchmark that builds maps moves memory between two runs of one commit
        # and there is nothing to fix. What it IS good for is showing the reader
        # where the memory bars above came from.
        lines.append("")
        lines.append("  the null moved memory, which is what the memory bars are measured from:")
        for unit, name, delta in moved:
            shown = "from zero" if not math.isfinite(delta) else f"{delta:+.2f}%"
            lines.append(f"    {name}  {unit}  {shown} between two runs of ONE commit")
    if weird:
        lines.append("")
        lines.append("  ⚠️ the null produced a zero-baseline row, which no ratio describes. It is")
        lines.append("     excluded from the bars rather than setting them to infinity:")
        for unit, name in weird:
            lines.append(f"    {name}  {unit}")

    failures = regressions(cand_rows, bars, geo)
    if not failures:
        lines.append("")
        lines.append("  no regression above the measured bars")
        return 0, lines

    lines.append("")
    lines.append("  REGRESSIONS:")
    for f in failures:
        lines.append(f"    ✗ {f}")

    if re.search(r"^Perf-moved:", message, re.MULTILINE):
        lines.append("")
        lines.append("  Overridden by a Perf-moved: trailer. The regression is recorded, not denied.")
        return 0, lines

    lines.append("")
    lines.append("  Add a Perf-moved: trailer saying WHY this is the right trade, or fix it.")
    lines.append("  The trailer is one line and should answer the question a later reader has:")
    lines.append("  not what the commit did, but why the cost moved and whether that was intended.")
    return 1, lines


def selftest():
    """Prove the gate still bites, and that it stops biting the thing it should not.

    ⚠️ The predecessor of this function was a manual act — a real regression was
    injected by hand and the gate watched to see whether it said "no regression",
    which is how the parser bug that dropped `?` rows was caught. Nothing kept
    doing it. A gate is not proved by its null passing; it is proved by its bite,
    so the bite is now asserted every time this file is run with --selftest.

    ⚠️ **Every assertion about the VERDICT goes through decide().** An earlier
    version of this function checked "a null that moved memory is no longer a
    failure" by calling regressions() with an empty candidate list, which returns
    [] whatever the bars are. It passed for a reason unrelated to its label and
    would have kept passing if the verdict regressed to the old behaviour. A test
    that cannot fail is worse than no test, because it reads like coverage.
    """
    ok = True

    def check(label, cond, got=None):
        nonlocal ok
        if cond:
            print(f"ok:   {label}")
        else:
            ok = False
            print(f"FAIL: {label}" + (f"  (got {got!r})" if got is not None else ""))

    def row(unit, name, delta, p="0.008", sig=True):
        return (unit, name, delta, p, sig)

    QUIET = []
    CLEAN_HEAD = [row("allocs/op", "X", -1.0), row("sec/op", "X", -2.0)]

    # --- the bars ---------------------------------------------------------
    bars, moved, weird = bars_from_null(QUIET)
    check("a quiet null falls back to every floor",
          bars["sec/op"] == MIN_TIME_BAR_PCT and bars["allocs/op"] == MIN_MEM_BAR_PCT, bars)
    check("a quiet null reports nothing moved", moved == [] and weird == [], (moved, weird))

    # Bars are per UNIT: a loud allocs/op null must not widen the B/op bar.
    split = [row("allocs/op", "Y", 3.0)]
    bars2, _, _ = bars_from_null(split)
    check("a loud allocs/op null raises its own bar", bars2["allocs/op"] == 3.0, bars2["allocs/op"])
    check("a loud allocs/op null does NOT widen the B/op bar",
          bars2["B/op"] == MIN_MEM_BAR_PCT, bars2["B/op"])

    # geomean sets no bar.
    gm = [row("allocs/op", "geomean", 40.0, p=None)]
    bars3, moved3, _ = bars_from_null(gm)
    check("geomean cannot widen a per-row bar", bars3["allocs/op"] == MIN_MEM_BAR_PCT, bars3["allocs/op"])
    check("geomean is not reported as the null moving", moved3 == [], moved3)

    # --- the verdict, driven end to end -----------------------------------
    # THE regression this commit exists to fix: the null moved memory, the head
    # did nothing. Must pass.
    jitter_null = [row("allocs/op", "Optimize/pool600", 0.004, p="0.008")]
    code, _ = decide(jitter_null, CLEAN_HEAD, "")
    check("a null that moved memory does NOT fail a clean head", code == 0, code)

    _, moved4, _ = bars_from_null(jitter_null)
    check("...and the movement is still reported, so the bar's origin is visible",
          len(moved4) == 1, moved4)

    # The bite it must keep. The docstring's own override example is +18%.
    real = [row("allocs/op", "Optimize/pool600", 18.0)]
    code, _ = decide(QUIET, real, "")
    check("a real allocation regression still fails", code == 1, code)
    code, _ = decide(QUIET, real, "subject\n\nPerf-moved: intended, buys the rescan")
    check("a Perf-moved: trailer still overrides it", code == 0, code)

    # A from-zero allocation is a direction, not a ratio.
    code, _ = decide(QUIET, [row("allocs/op", "Polish", float("inf"))], "")
    check("an allocation appearing on a zero-allocation path still fails", code == 1, code)

    # ⚠️ A from-zero row in the NULL must not raise the bar to infinity and
    # disable the magnitude gate for every other benchmark in the suite.
    inf_null = [row("allocs/op", "WeirdZeroAlloc", float("inf"))]
    bars5, _, weird5 = bars_from_null(inf_null)
    check("a from-zero NULL row does not set an infinite bar",
          math.isfinite(bars5["allocs/op"]), bars5["allocs/op"])
    check("...and is reported as unratioable rather than silently dropped",
          len(weird5) == 1, weird5)
    code, _ = decide(inf_null, [row("allocs/op", "Elsewhere", 50.0)], "")
    check("a +50% regression still fails despite a from-zero null row", code == 1, code)

    # ⚠️ A cost spread thinly across many benchmarks stays under every per-row
    # bar. The aggregate is what catches it.
    thin = [row("allocs/op", f"B{i}", 0.4) for i in range(8)]
    code, _ = decide(QUIET, thin, "")
    check("a thin per-row cost alone is under the per-row bar", code == 0, code)
    code, _ = decide(QUIET, thin + [row("allocs/op", "geomean", 0.4, p=None)], "")
    check("...but the same cost fails on the aggregate bar", code == 1, code)
    code, _ = decide(QUIET, [row("allocs/op", "geomean", 0.1, p=None)], "")
    check("an aggregate move under the aggregate bar passes", code == 0, code)

    # Ordering around the per-row bar.
    code, _ = decide(QUIET, [row("B/op", "X", MIN_MEM_BAR_PCT - 0.1)], "")
    check("a memory move under the bar passes", code == 0, code)
    code, _ = decide(QUIET, [row("B/op", "X", MIN_MEM_BAR_PCT + 0.1)], "")
    check("a memory move over the bar fails", code == 1, code)

    # A loud null legitimately buys tolerance.
    loud = [row("allocs/op", "Y", 3.0), row("sec/op", "Y", 21.0)]
    code, _ = decide(loud, [row("allocs/op", "Y", 2.0)], "")
    check("a 2% memory move passes under a 3% measured bar", code == 0, code)
    code, _ = decide(loud, [row("sec/op", "Y", 30.0)], "")
    check("a 30% runtime move still fails a 21% measured bar", code == 1, code)

    # Non-regressions.
    code, _ = decide(QUIET, [row("sec/op", "Z", -30.0)], "")
    check("a faster benchmark is not a failure", code == 0, code)
    code, _ = decide(QUIET, [row("allocs/op", "Z", 99.0, p="0.9", sig=False)], "")
    check("an insignificant move is not a failure", code == 0, code)

    print("\n" + ("SELFTEST PASSED" if ok else "SELFTEST FAILED"))
    return 0 if ok else 1


def main():
    if "--selftest" in sys.argv:
        return selftest()
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--control", required=True, help="a SECOND run of base: the null")
    ap.add_argument("--head", required=True)
    ap.add_argument("--message", default="", help="commit message, read for a Perf-moved: trailer")
    args = ap.parse_args()

    null_rows = run_benchstat(args.base, args.control)
    cand_rows = run_benchstat(args.base, args.head)

    code, lines = decide(null_rows, cand_rows, args.message)
    for line in lines:
        print(line)
    return code


if __name__ == "__main__":
    sys.exit(main())
