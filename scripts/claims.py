#!/usr/bin/env python3
"""Run every claim in stats/claims.yaml. A stored number that cannot be re-derived is a bug.

    python3 scripts/claims.py             # check
    python3 scripts/claims.py --emit      # print what each claim currently evaluates to
    python3 scripts/claims.py --selftest  # prove this checker can FAIL

# Why this exists

The recurring error in this project is not a wrong idea, it is a NUMBER PRODUCED
BY AN AD-HOC COMMAND, stated as fact, and never re-run. They are cheap to check
and go unchecked, because prose does not carry its command.

⚠️ **A copy is safe exactly when a checker can reach both sides and runs on every
change.** A store retired earlier in this project failed that test — its source
lived where no guard could read it. This registry is the reachable case: entry and
source in one checkout, compared by CI. `internal/backtest/cellcsv_regression_test.go`
puts the same idea repo-natively: "A wrong source is the failure mode, not a wrong
constant."

A `derived` claim stores the command AND the expectation, and this script re-runs
the command and compares. ⚠️ What the retired store lacked was not the value — it
was the generator and the equality check. Both are here and both run every CI run.
⚠️ This detects unintended CHANGE, not WRONGNESS: a deliberate change is fixed by
editing `expect`, and only review catches a deliberate change that should not have
happened. A claim that cannot be re-run cheaply is `measured` instead and stores
what INVALIDATES it, so the checker reports staleness rather than truth.

# What it does NOT do

⚠️ It does not read prose for MEANING. It does check that each `asserted_in` file
exists and contains the expected value — that is admission criterion 2 turned from
prose into a gate — but a sentence that quotes the right number while drawing the
wrong conclusion is invisible to it.

⚠️ A green run is not evidence this checker works. That is the OTHER recurring
failure here, so `--selftest` exists and CI runs it: it mutates each expectation
and asserts the checker goes red. A guard that has never been shown to fail is
not a guard.
"""
import argparse, os, subprocess, sys, yaml

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REG = os.path.join(ROOT, "stats", "claims.yaml")


def run(cmd):
    p = subprocess.run(["sh", "-c", cmd], cwd=ROOT, capture_output=True, text=True, timeout=120)
    return p.returncode, p.stdout.strip(), p.stderr.strip()


def check(claims, emit=False):
    problems = []
    for c in claims:
        cid, kind = c["id"], c["kind"]
        if kind == "derived":
            rc, out, err = run(c["command"])
            if rc != 0:
                problems.append(f"{cid}: command failed rc={rc}: {err[:160]}")
                continue
            if emit:
                print(f"  {cid}\n      -> {out}")
                continue
            exp = c.get("expect")
            if exp is None:
                print(f"  {cid:32} = {out}   (no pinned expectation; derived fresh each run)")
            elif out != str(exp):
                problems.append(
                    f"{cid}: expected {exp!r}, command produced {out!r}. "
                    f"⚠️ Fix the CODE or the expectation deliberately — do not edit this to go green.")
            else:
                print(f"  {cid:32} = {out}   ok")
            # admission criterion 2, checked rather than asserted
            for rel in c.get("asserted_in", []):
                fp = os.path.join(ROOT, rel)
                if not os.path.exists(fp):
                    problems.append(f"{cid}: asserted_in {rel!r} does not exist")
                elif exp is not None and str(exp) not in open(fp, errors="ignore").read():
                    problems.append(
                        f"{cid}: asserted_in {rel!r} does not contain {exp!r} — either the "
                        f"quote moved, or this claim is not load-bearing by criterion 2")
        elif kind == "measured":
            if emit:
                print(f"  {cid}\n      -> {c['value']}  [{c['measured_at']}]")
                continue
            missing = [k for k in ("value", "measured_at", "threshold") if not c.get(k)]
            if missing:
                problems.append(
                    f"{cid}: measured claim missing {missing}. ⚠️ A result below the detection "
                    f"threshold of its own comparison is not a result — but note this asserts the "
                    f"threshold is PRESENT, not that the value clears it. Free-prose thresholds "
                    f"cannot be compared mechanically; that stays a review question.")
            else:
                print(f"  {cid:32} = {c['value']}")
                print(f"  {'':32}   measured at: {c['measured_at']}")
                for inv in c.get("invalidated_by", []):
                    print(f"  {'':32}   ⚠️ invalidated by: {inv}")
        elif kind == "judgement":
            problems.append(f"{cid}: a judgement is not a statistic — verdicts live in AGENTS.md")
        else:
            problems.append(f"{cid}: unknown kind={kind!r} (expected derived/measured)")
    return problems


def selftest():
    """Prove the checker bites. A green null proves nothing; only a bite does."""
    reg = yaml.safe_load(open(REG))
    derived = [c for c in reg["claims"] if c["kind"] == "derived"]
    failures = 0
    for c in derived:
        # ⚠️ A claim with no `expect` CANNOT FAIL in check(), so it is invisible to
        # both halves of this tool: it reports a successful derivation whatever the
        # command returns, including a broken one. An earlier version skipped these
        # and still printed "0 failures", which is this project's rule 2 violated by
        # the tool that states it. Unfalsifiable is now RED.
        if c.get("expect") is None:
            print(f"  ✗ {c['id']}: no expectation, so this claim can never fail — "
                  f"pin one, or drop it under the admission test")
            failures += 1
            continue
        # the unmutated claim must be green first, or a "caught" is just a broken command
        if check([c]):
            print(f"  ✗ {c['id']}: fails BEFORE mutation — a broken command cannot prove a bite")
            failures += 1
            continue
        mutated = dict(c)
        mutated["expect"] = str(c["expect"]) + "_MUTATED"
        if not check([mutated]):
            print(f"  ✗ {c['id']}: mutating the expectation did NOT fail the check")
            failures += 1
        else:
            print(f"  ✓ {c['id']}: mutation caught")
    # and a measured claim stripped of its threshold must fail
    m = [c for c in reg["claims"] if c["kind"] == "measured"]
    if m:
        stripped = {k: v for k, v in m[0].items() if k != "threshold"}
        if not check([stripped]):
            print(f"  ✗ {m[0]['id']}: removing the detection threshold did NOT fail the check")
            failures += 1
        else:
            print(f"  ✓ {m[0]['id']}: missing-threshold caught")
    print(f"\n  selftest: {failures} failure(s)")
    return 1 if failures else 0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--emit", action="store_true")
    ap.add_argument("--selftest", action="store_true")
    a = ap.parse_args()
    reg = yaml.safe_load(open(REG))

    if a.selftest:
        print("CLAIMS CHECKER SELFTEST — mutating each expectation, expecting red\n")
        return selftest()

    print("CLAIMS REGISTRY" + ("  (emit)" if a.emit else ""))
    problems = check(reg["claims"], emit=a.emit)
    if a.emit:
        return 0
    if problems:
        print("\n  PROBLEMS:")
        for p in problems:
            print(f"    ✗ {p}")
        return 1
    print(f"\n  {len(reg['claims'])} claim(s) check out")
    return 0


if __name__ == "__main__":
    sys.exit(main())
