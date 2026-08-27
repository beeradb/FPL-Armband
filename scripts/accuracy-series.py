#!/usr/bin/env python3
"""Assemble the published accuracy snapshots into a series, and say what moved.

    scripts/accuracy-series.py [--out DIR] [--limit N] [--threshold 0.005]

Every push to main publishes an accuracy snapshot as a GitHub Release --
`.github/workflows/snapshot.yml` has done so since this repository's first public
commit. Each carries `figures.csv`, including the model's calibration against
realised points: predicted, actual, their ratio (1.000 is perfect), the bias, and
the mean absolute error, for a model built through each of GW4..GW32.

WHY THIS EXISTS

The snapshots were never the problem. **Nothing joined them into a series.** A
release per push is not a record anyone can read: it cannot answer "when did
accuracy change, and what landed that week", which is the only question the
series is for. Assembled, ten days of history contained exactly one step over
0.01 -- and it named its own commit.

⚠️ WHAT THE SERIES CANNOT SEE

CI publishes from a GitHub runner, which has no access to any local measured xGC
cache: the workflow sets no source switch and this repository carries no such
data. So every published snapshot is on the RECONSTRUCTED input, and the series
is blind to the source local development may run against. A regression that
appears only on a measured input would never show up here.

⚠️ A DROP AFTER AN INPUT CHANGE IS NOT EVIDENCE THE INPUT IS WORSE. Every scoring
constant was fitted against the reconstruction, so swapping the input leaves them
stale by construction, and a degradation is ambiguous between "worse data" and
"better data, mistuned constants". Separating them needs a re-tune on the new
input and a tuned-against-tuned comparison.

⚠️ RATIO IS A CALIBRATION LEVEL, NOT ACCURACY. It says whether predictions run
high on average. A model can be perfectly calibrated and rank players badly, and
an optimiser consumes the ordering. MAE travels alongside; neither measures rank.
"""
import argparse, csv, json, os, subprocess, sys, tempfile

COHORTS = (4, 8, 12, 16, 20, 24, 28, 32)
METRICS = ("predicted", "actual", "ratio", "bias", "mean_absolute_error")


def gh(*args):
    return subprocess.run(["gh", *args], capture_output=True, text=True)


def releases(limit):
    r = gh("release", "list", "--limit", str(limit),
           "--json", "tagName,createdAt", "-q", '.[] | [.tagName, .createdAt] | @tsv')
    if r.returncode:
        sys.exit(f"gh release list failed: {r.stderr.strip()}")
    out = []
    for line in r.stdout.splitlines():
        if "\t" in line:
            out.append(tuple(line.split("\t")))
    return out


def subjects():
    """Commit subjects, so a step names what landed rather than only when."""
    log = subprocess.run(["git", "log", "--format=%h\t%s", "--all"],
                         capture_output=True, text=True).stdout
    return {l.split("\t", 1)[0][:7]: l.split("\t", 1)[1]
            for l in log.splitlines() if "\t" in l}


REASON_TRAILER = "Figures-moved:"
BACKFILL = "stats/figures-moved.csv"


def backfilled():
    """Reasons RECONSTRUCTED after the fact, for commits predating the trailer.

    ⚠️ **A reconstruction is not a declaration** and the two are kept apart
    everywhere downstream. A trailer is the author saying why at the time; a row
    here is a later reader's account, produced by someone who already knew the
    figures had moved and was therefore looking for a cause. That search finds
    one more often than it should.
    """
    out = {}
    if not os.path.exists(BACKFILL):
        return out
    for row in csv.reader(open(BACKFILL)):
        if not row or row[0].lstrip().startswith("#") or len(row) < 3:
            continue
        out[row[0].strip()[:7]] = {"expected": row[1].strip().lower() == "yes",
                                   "why": row[2].strip()}
    return out


def reasons():
    """The declared REASON a commit moved the published figures.

    A subject line says what a commit did. It does not say why the numbers
    moved, and those are different facts: `5b97033` reads "Stop squad
    feasibility from depending on score order", which is true and gives no hint
    that it would trade early-season calibration for late-season calibration
    three to one.

    So a commit that moves the figures declares it, in a `Figures-moved:`
    trailer, and that text is what the timeline overlays. ⚠️ **Enforced in CI by
    scripts/figures-moved-check.py, which fails a build whose snapshot differs
    from its predecessor with no trailer.** Without the check the convention
    decays to whichever authors remember it, and the annotations become a record
    of diligence rather than of change.
    """
    log = subprocess.run(
        ["git", "log", "--format=%h%x01%B%x02", "--all"],
        capture_output=True, text=True).stdout
    out = {}
    for entry in log.split("\x02"):
        if "\x01" not in entry:
            continue
        sha, body = entry.split("\x01", 1)
        sha = sha.strip()[:7]
        for line in body.splitlines():
            if line.startswith(REASON_TRAILER):
                out[sha] = line[len(REASON_TRAILER):].strip()
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="accuracy-series")
    ap.add_argument("--limit", type=int, default=300)
    ap.add_argument("--threshold", type=float, default=0.005,
                    help="report a cohort move larger than this")
    a = ap.parse_args()
    os.makedirs(f"{a.out}/rel", exist_ok=True)

    rels, subj, why, back = releases(a.limit), subjects(), reasons(), backfilled()
    print(f"{len(rels)} releases", file=sys.stderr)
    for tag, _ in rels:
        dest = f"{a.out}/rel/{tag}.csv"
        if not os.path.exists(dest):
            gh("release", "download", tag, "-p", "figures.csv", "-O", dest, "--clobber")

    rows = []
    for tag, created in rels:
        path = f"{a.out}/rel/{tag}.csv"
        if not os.path.exists(path):
            continue
        fig = {r["figure"]: r["value"] for r in csv.DictReader(open(path))
               if r.get("figure")}
        c = {}
        for gw in COHORTS:
            pre = f"model.calibration_drift.model_built_through_gw{gw}."
            d = {}
            for m in METRICS:
                v = fig.get(pre + m)
                if v:
                    try:
                        d[m] = float(v)
                    except ValueError:
                        pass
            if "ratio" in d:
                c[str(gw)] = d
        if c:
            sha = tag.replace("snapshot-", "")
            rows.append({"s": sha, "d": created[:10], "t": created[11:16],
                         "m": subj.get(sha, "")[:90],
                         "why": why.get(sha, "") or back.get(sha, {}).get("why", ""),
                         "src": ("declared" if sha in why
                                 else "reconstructed" if sha in back else ""),
                         "expected": back.get(sha, {}).get("expected"),
                         "c": c})
    rows.sort(key=lambda r: (r["d"], r["t"]))

    moves = []
    for gw in COHORTS:
        g, prev = str(gw), None
        for i, r in enumerate(rows):
            if g not in r["c"]:
                continue
            v = r["c"][g]["ratio"]
            if prev is not None and abs(v - prev) > a.threshold:
                moves.append({"i": i, "g": gw, "from": prev, "to": v,
                              "s": r["s"], "d": r["d"], "m": r["m"],
                              "why": r.get("why", ""), "src": r.get("src", ""),
                              "expected": r.get("expected")})
            prev = v
    moves.sort(key=lambda m: -abs(m["to"] - m["from"]))

    json.dump({"rows": rows, "moves": moves, "cohorts": list(COHORTS)},
              open(f"{a.out}/series.json", "w"), separators=(",", ":"))

    print(f"\n{len(rows)} snapshots carrying calibration figures, "
          f"{rows[0]['d']} to {rows[-1]['d']}")
    print(f"{len(moves)} cohort moves over {a.threshold}\n")
    print(f"{'cohort':>7} {'from':>8} {'to':>8} {'delta':>9}  {'date':<11} {'commit':<9} what landed")
    undeclared = set()
    for m in moves:
        print(f"  gw{m['g']:<4} {m['from']:>8.4f} {m['to']:>8.4f} "
              f"{m['to']-m['from']:>+9.4f}  {m['d']:<11} {m['s']:<9} {m['m'][:52]}")
        if m["why"]:
            tag = "declared" if m["src"] == "declared" else "RECONSTRUCTED"
            exp = "" if m["expected"] is None else (
                "  [expected]" if m["expected"] else "  [UNEXPECTED]")
            print(f"           {tag}{exp}: {m['why'][:70]}")
        else:
            undeclared.add((m["s"], m["m"][:52]))
    if undeclared:
        print(f"\n⚠️ {len(undeclared)} commit(s) moved figures with no {REASON_TRAILER} trailer,")
        print("   so the timeline can say WHEN they moved and not WHY:")
        for sha, subj_ in sorted(undeclared):
            print(f"     {sha}  {subj_}")
    if not moves:
        print("  (nothing moved -- which is a result, not an empty run)")


if __name__ == "__main__":
    main()
