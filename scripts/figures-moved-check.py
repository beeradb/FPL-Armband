#!/usr/bin/env python3
"""Fail a build whose accuracy figures moved without saying why.

    scripts/figures-moved-check.py --current figures.csv [--threshold 0.005]

# Why this is a gate and not a report

The accuracy snapshots have been published on every push since this repository's
first public commit, and for ten days nothing compared one to the next. In that
window exactly two commits moved the figures. One of them — a squad-feasibility
refactor — traded early-season calibration for late-season calibration roughly
three to one, and **nobody knew until the series was assembled a day later**.

A report would not have caught it, because nobody was reading. A gate stops the
build and makes the author say what happened, at the moment they still know.

# What it asks for

A `Figures-moved:` trailer in the commit message, whose text becomes the
annotation on the accuracy timeline. One line, and it should answer the question
a later reader actually has: not what the commit did — the subject already says
that — but **why the numbers moved**, and whether that was intended.

    Figures-moved: MinutesWeight 1.0 shifts every cohort's calibration down
    together; intended, and the size was not predicted.

⚠️ **Movement is not failure.** A scoring-constant change SHOULD move the
figures, and this gate is satisfied by declaring it. What it refuses is silent
movement, which is the only kind nobody can review.

⚠️ **It cannot tell intended from unintended movement** — only whether anyone
said. That distinction lives in the words, which is why the trailer is prose and
not a boolean.
"""
import argparse, csv, os, re, subprocess, sys

TRAILER = "Figures-moved:"


def read(path):
    out = {}
    with open(path) as f:
        for r in csv.DictReader(f):
            fig, val = r.get("figure"), r.get("value")
            if not fig or val in (None, ""):
                continue
            try:
                out[fig] = float(val)
            except ValueError:
                pass
    return out


def previous(tmp):
    """The newest published snapshot's figures, or None when there is none."""
    r = subprocess.run(["gh", "release", "list", "--limit", "1",
                        "--json", "tagName", "-q", ".[0].tagName"],
                       capture_output=True, text=True)
    tag = r.stdout.strip()
    if r.returncode or not tag:
        return None, None
    dest = os.path.join(tmp, "prev.csv")
    d = subprocess.run(["gh", "release", "download", tag, "-p", "figures.csv",
                        "-O", dest, "--clobber"], capture_output=True, text=True)
    if d.returncode or not os.path.exists(dest):
        return None, tag
    return read(dest), tag


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--current", required=True)
    ap.add_argument("--threshold", type=float, default=0.005)
    ap.add_argument("--tmp", default=".")
    a = ap.parse_args()

    cur = read(a.current)
    prev, tag = previous(a.tmp)
    if prev is None:
        # ⚠️ No predecessor is NOT a pass. It is the one case this gate cannot
        # judge, and saying so is the difference between "checked and clean" and
        # "did not check" — which this repository has already confused once, in a
        # guard that skipped when it found nothing and read as green.
        print(f"no previous snapshot to compare against (newest release: {tag or 'none'});"
              " this build is UNCHECKED, not clean")
        return 0

    moved = []
    for fig, v in sorted(cur.items()):
        if fig in prev and abs(v - prev[fig]) > a.threshold:
            moved.append((fig, prev[fig], v))
    if not moved:
        print(f"no figure moved more than {a.threshold} against {tag}")
        return 0

    body = subprocess.run(["git", "log", "-1", "--format=%B"],
                          capture_output=True, text=True).stdout
    declared = next((l[len(TRAILER):].strip() for l in body.splitlines()
                     if l.startswith(TRAILER)), "")

    print(f"{len(moved)} figure(s) moved more than {a.threshold} against {tag}:")
    for fig, o, n in moved[:20]:
        print(f"  {n-o:+9.4f}  {o:8.4f} -> {n:8.4f}  {fig}")
    if len(moved) > 20:
        print(f"  ... and {len(moved)-20} more")

    if declared:
        print(f"\ndeclared: {declared}")
        return 0
    print(f"""
FAIL: the published figures moved and the commit does not say why.

Add a `{TRAILER}` trailer to the commit message. It becomes the annotation on the
accuracy timeline, so write the reason a later reader needs — not what the commit
did, which the subject already says, but why these numbers moved and whether that
was intended.

Movement is not a failure. Silent movement is: it is the only kind nobody can
review, and it is how a squad-feasibility refactor came to trade early-season
calibration for late-season calibration with nobody noticing for a day.""")
    return 1


if __name__ == "__main__":
    sys.exit(main())
