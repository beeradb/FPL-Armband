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


def bars_from_null(null_rows):
    """The two bars this runner earned, plus what the null moved on memory.

    ⚠️ `geomean` is excluded from both bars. benchstat prints a percentage for it
    even when every individual row is `~`, because it is a summary rather than a
    tested comparison, and letting it set a bar would let one summary row widen
    the gate for every real one.
    """
    time_worst, mem_worst = 0.0, 0.0
    moved = []
    for unit, name, delta, _p, sig in null_rows:
        if name == "geomean" or not sig:
            continue
        if unit in TIME_UNITS:
            time_worst = max(time_worst, abs(delta))
        elif unit in MEMORY_UNITS:
            mem_worst = max(mem_worst, abs(delta))
            moved.append((unit, name, delta))
    return (max(time_worst, MIN_TIME_BAR_PCT),
            max(mem_worst, MIN_MEM_BAR_PCT),
            moved)


def regressions(cand_rows, time_bar, mem_bar):
    """Every head row that fails, given the two bars the null earned.

    Only positive deltas can fail: a benchmark that got faster or allocated less
    is not a regression, and `sig` is benchstat's own judgement rather than this
    script's.
    """
    out = []
    for unit, name, delta, p, sig in cand_rows:
        if name == "geomean" or not sig or delta <= 0:
            continue
        if unit in MEMORY_UNITS:
            if delta == float("inf"):
                # A direction, not a ratio: no bar covers going from zero.
                out.append(f"{name}  {unit}  from zero (p={p})  — an allocation on a path that had none")
            elif delta > mem_bar:
                out.append(f"{name}  {unit}  {delta:+.2f}% (p={p})  — above this runner's {mem_bar:.2f}% memory null bar")
        elif unit in TIME_UNITS and delta > time_bar:
            out.append(f"{name}  {unit}  {delta:+.2f}% (p={p})  — above this runner's {time_bar:.1f}% null bar")
    return out


def selftest():
    """Prove the gate still bites, and that it stops biting the thing it should not.

    ⚠️ The predecessor of this function was a manual act — a real regression was
    injected by hand and the gate watched to see whether it said "no regression",
    which is how the parser bug that dropped `?` rows was caught. Nothing kept
    doing it. A gate is not proved by its null passing; it is proved by its bite,
    so the bite is now asserted every time this file is run with --selftest.
    """
    ok = True

    def check(label, cond, got=None):
        nonlocal ok
        if cond:
            print(f"ok:   {label}")
        else:
            ok = False
            print(f"FAIL: {label}" + (f"  (got {got!r})" if got is not None else ""))

    # A quiet null earns the floors, not zero.
    t, m, moved = bars_from_null([])
    check("an empty null falls back to both floors", (t, m) == (MIN_TIME_BAR_PCT, MIN_MEM_BAR_PCT), (t, m))
    check("a quiet null reports nothing moved", moved == [], moved)

    # The regression this change exists to stop failing: map-seed jitter in the
    # null, and a head that did nothing.
    jitter = [("allocs/op", "Optimize/pool600", 0.004, "0.008", True)]
    t, m, moved = bars_from_null(jitter)
    check("map-seed jitter does not lift the memory bar off its floor", m == MIN_MEM_BAR_PCT, m)
    check("the jitter is still reported so the bar's origin is visible", len(moved) == 1, moved)
    check("a null that moved memory is no longer a failure by itself",
          regressions([], t, m) == [])

    # ...and the bite it must keep. The docstring's own override example is +18%.
    real = [("allocs/op", "Optimize/pool600", 18.0, "0.008", True)]
    check("a real allocation regression still fails", len(regressions(real, t, m)) == 1)

    # A benchmark going from zero allocations to some is a direction, not a ratio.
    zero = [("allocs/op", "Polish/pool200", float("inf"), "0.008", True)]
    check("an allocation appearing on a zero-allocation path still fails",
          len(regressions(zero, t, m)) == 1)

    # Below the floor is tolerated; just above it is not. Ordering matters more
    # than either number.
    check("a memory move under the bar passes",
          regressions([("B/op", "X", MIN_MEM_BAR_PCT - 0.1, "0.01", True)], t, m) == [])
    check("a memory move over the bar fails",
          len(regressions([("B/op", "X", MIN_MEM_BAR_PCT + 0.1, "0.01", True)], t, m)) == 1)

    # A loud null raises the bar above the floor, which is the whole point of
    # measuring it rather than asserting it.
    loud = [("allocs/op", "Y", 3.0, "0.01", True), ("sec/op", "Y", 21.0, "0.01", True)]
    t2, m2, _ = bars_from_null(loud)
    check("a loud null raises the memory bar above its floor", m2 == 3.0, m2)
    check("a loud null raises the runtime bar above its floor", t2 == 21.0, t2)
    check("a 2% memory move passes under a 3% measured bar",
          regressions([("allocs/op", "Y", 2.0, "0.01", True)], t2, m2) == [])

    # geomean is a summary benchstat prints even when every real row is `~`.
    gm = [("allocs/op", "geomean", 40.0, None, True)]
    _, m3, moved3 = bars_from_null(gm)
    check("geomean cannot widen the memory bar", m3 == MIN_MEM_BAR_PCT, m3)
    check("geomean is not reported as the null moving", moved3 == [], moved3)
    check("geomean cannot fail the gate", regressions(gm, t, m) == [])

    # An improvement is not a regression.
    check("a faster benchmark is not a failure",
          regressions([("sec/op", "Z", -30.0, "0.01", True)], t, m) == [])
    check("an insignificant move is not a failure",
          regressions([("allocs/op", "Z", 99.0, "0.9", False)], t, m) == [])

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

    time_bar, mem_bar, moved = bars_from_null(null_rows)

    print("PERFORMANCE GATE")
    print(f"  runtime bar measured from this runner's own null: {time_bar:.1f}%"
          f"{'  (floor)' if time_bar == MIN_TIME_BAR_PCT else ''}")
    print(f"  memory  bar measured from this runner's own null: {mem_bar:.2f}%"
          f"{'  (floor)' if mem_bar == MIN_MEM_BAR_PCT else ''}")

    if moved:
        # Reported, not failed. Go randomises map hash seeds per process, so a
        # benchmark that builds maps moves memory between two runs of one commit
        # and there is nothing to fix. What it IS good for is showing the reader
        # where the memory bar above came from.
        print("\n  the null moved memory, which is what the memory bar is measured from:")
        for unit, name, delta in moved:
            print(f"    {name}  {unit}  {delta:+.2f}% between two runs of ONE commit")

    failures = regressions(cand_rows, time_bar, mem_bar)

    if not failures:
        print("\n  no regression above the measured bars")
        return 0

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
