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


def trailer_search_body():
    """Every commit message the trailer could legitimately be written in.

    ⚠️ **`git log -1` alone made this gate UNSATISFIABLE on a pull request, and
    it was, for as long as the gate existed.** On a `pull_request` event
    actions/checkout does not check out the branch — it checks out a synthetic
    merge commit GitHub generates for the run, whose entire message is
    "Merge <head-sha> into <base-sha>". No trailer an author writes can ever
    appear there, so the gate failed, printed instructions to add a trailer, and
    then failed again with the trailer added.

    That went unnoticed because the gate only speaks when the figures MOVE, and
    most pull requests do not move them. The first one that did could not be
    merged by following the message it printed — which is the worst shape a
    guard can take: correct in what it detects, impossible to answer.

    A synthetic merge commit has two parents, and the first is the base branch
    tip, so `HEAD^1..HEAD` is exactly the pull request's own commits. That range
    is where the author's trailer actually lives. On a push, HEAD has one parent
    and this is `git log -1` as before.

    ⚠️ Deliberately ANY commit in the range, not just the tip. An author who
    explains the movement in the commit that caused it has said the thing this
    gate exists to make them say; demanding it be repeated on the last commit
    would be asking for a ritual rather than an explanation.
    """
    parents = subprocess.run(["git", "rev-list", "--parents", "-n", "1", "HEAD"],
                             capture_output=True, text=True).stdout.split()
    rev = "HEAD^1..HEAD" if len(parents) > 2 else "HEAD~1..HEAD"
    out = subprocess.run(["git", "log", "--format=%B", rev],
                         capture_output=True, text=True)
    # A shallow clone, or the very first commit, has no parent to range from.
    # Fall back rather than crash: the old behaviour is still correct there.
    if out.returncode != 0:
        return subprocess.run(["git", "log", "-1", "--format=%B"],
                              capture_output=True, text=True).stdout
    return out.stdout


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


def selftest():
    """Build both commit shapes in a throwaway repo and check the trailer is found.

    The defect this covers is not a wrong answer, it is an UNREACHABLE one: the
    gate demanded a trailer it then looked for in the wrong commit, so on a pull
    request no author could satisfy it. Nothing failed loudly — the gate simply
    stayed red while its own instructions were followed.

    Two shapes, and the merge one is the case that was broken:

      push          HEAD has one parent, the trailer is on HEAD itself
      pull request  HEAD is GitHub's synthetic merge; parent 1 is the base tip,
                    parent 2 is the branch, and the trailer is on a commit
                    INSIDE the branch rather than on HEAD

    Plus the negative case, because a check that only ever answers "found" would
    pass while matching nothing at all.
    """
    import tempfile
    def git(cwd, *a):
        return subprocess.run(["git", "-C", cwd, *a], capture_output=True, text=True,
                              check=True).stdout
    ok = True
    with tempfile.TemporaryDirectory() as d:
        git(d, "init", "-q", "-b", "main")
        git(d, "config", "user.email", "t@t"); git(d, "config", "user.name", "t")
        open(os.path.join(d, "f"), "w").write("1")
        git(d, "add", "."); git(d, "commit", "-q", "-m", "base")
        git(d, "checkout", "-q", "-b", "feature")
        open(os.path.join(d, "f"), "w").write("2")
        git(d, "commit", "-qam", "the change\n\nFigures-moved: because X")

        cwd = os.getcwd()
        try:
            os.chdir(d)
            # Shape 1: a push, HEAD is the commit carrying the trailer.
            if TRAILER not in trailer_search_body():
                print("SELFTEST FAIL: trailer not found on a single-parent HEAD"); ok = False
            # Shape 2: a pull request, HEAD is a merge and the trailer is behind it.
            os.chdir(cwd); git(d, "checkout", "-q", "main")
            git(d, "merge", "-q", "--no-ff", "feature", "-m", "Merge feature into main")
            os.chdir(d)
            if TRAILER not in trailer_search_body():
                print("SELFTEST FAIL: trailer not found behind a merge commit — this is "
                      "the shape every pull request has, and the shape that was broken")
                ok = False
            # Negative: a branch that declares nothing must still read as silent.
            os.chdir(cwd); git(d, "checkout", "-q", "-b", "quiet", "main")
            open(os.path.join(d, "f"), "w").write("3")
            git(d, "commit", "-qam", "no trailer here")
            os.chdir(d)
            if TRAILER in trailer_search_body():
                print("SELFTEST FAIL: found a trailer where none was written"); ok = False
        finally:
            os.chdir(cwd)
    print("selftest: ok" if ok else "selftest: FAILED")
    return 0 if ok else 1


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--current")
    ap.add_argument("--threshold", type=float, default=0.005)
    ap.add_argument("--tmp", default=".")
    ap.add_argument("--selftest", action="store_true",
                    help="check the trailer is findable in both commit shapes")
    a = ap.parse_args()
    if a.selftest:
        return selftest()
    if not a.current:
        ap.error("--current is required")

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

    declared = next((l[len(TRAILER):].strip() for l in trailer_search_body().splitlines()
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
