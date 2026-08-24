"""Collect every per-sweep `mde.csv` into one per-arm table, and report the
population medians the record quotes.

Run it through `stats/regenerate_mde.sh`, which writes the per-sweep tables first.

# What a row is, and why the aggregate is not itself an instrument property

One row is one **comparison**: one alternative against its baseline, on one metric,
under one estimator. A detection threshold belongs to a comparison and not to the
harness — that is the whole finding this table exists to support, and the range
across these rows is sixtyfold. Do not average the column and call the result "the
harness's resolution"; that is the pooling defect the `scope` column was added to
undo.

# Two filters, both load-bearing

**Structurally inert arms are dropped.** A transfer setting cannot move `HOLD`: every
cell comes out byte-identical, the variance is exactly zero, and the threshold reads
0.0. That is not a comparison that resolves one point a season, it is a comparison
that was never able to run. Counting it would drag every median toward zero. This
project's rule is that a season producing identical output under an intervention is
not a tie, and the same holds for an arm.

**Only `hold` and `policy` count toward the quoted medians.** The captaincy rungs
(`hold_fixedcap`, `hold_nocap`) are instrument diagnostics rather than settings
anybody ships, and they postdate the recorded figure. They stay in the written CSV
and out of the summary.
"""

import csv
import glob
import os
import statistics
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "stats", "out")
AGG = os.path.join(OUT, "mde-all-arms.csv")

CLUSTERED = "season-clustered (primary)"
START_FIXED = "start fixed, no season effect"

# The near-null 1-2% nudges of TestDiagNoiseFloor. They measure the floor rather
# than a threshold — arms built not to move — so they belong in the table and not in
# a median that is meant to describe an ordinary comparison.
NOISE = {"noise4", "noise6"}

# Hindsight arms. They are real comparisons on the same grid and their thresholds are
# honest, but they bound what information is worth rather than what a setting is
# worth, and they were not in the population the record's figure was computed over.
ORACLE = {"oracleminutes", "oraclearmband", "oraclegate", "oracleprices",
          "oraclechip", "pricetiming", "teamnews"}

# Grids. Six-season sweeps resolve finer and must not be pooled with four-season
# ones: the whole point of the widening was that the threshold moved.
#
# ⚠️ This was the hand-maintained literal `SIX = {"noise6", "vice6", "teamnews"}`
# until 2026-08-15. It is now DERIVED from the `seasons` column that
# `variance_components.R` already writes into every row — which reproduces those
# three exactly, so nothing moves, and a six-season sweep added tomorrow is no longer
# silently pooled into the four-season population that the record's canonical median
# is computed over.
#
# The standing rule this follows is that the stale-item class wants **a rule and not
# a count**. Two of the three names in that literal were `noise6` and `teamnews` — a
# noise-floor arm and an oracle — both of which the four-season side excludes by
# name, so the literal was also the mechanism behind an asymmetric filter. See
# `six_settings` below.
def grid_seasons(row):
    """The season count this arm was measured on, from the row itself.

    Errors rather than defaulting. A row with no `seasons` would otherwise be
    silently treated as four-season and pooled into the record's population, which
    is the exact failure the derivation is here to remove — a missing value must not
    be indistinguishable from a four.
    """
    raw = (row.get("seasons") or "").strip()
    if not raw:
        raise SystemExit(
            f"arm row from sweep {row.get('_sweep')!r} carries no `seasons` value, "
            "so its grid width is unknown and it cannot be placed in either "
            "population. Regenerate its table with a current variance_components.R."
        )
    return int(raw)


def load():
    rows, cols = [], None
    for path in sorted(glob.glob(os.path.join(OUT, "*", "mde.csv"))):
        sweep = os.path.basename(os.path.dirname(path))
        with open(path, newline="") as fh:
            for r in csv.DictReader(fh):
                if cols is None:
                    cols = list(r.keys())
                r["_sweep"] = sweep
                rows.append(r)
    return rows, cols


def sig(r):
    try:
        return float(r["sig_season"])
    except (KeyError, TypeError, ValueError):
        return None


def summarise(label, rows, estimator):
    vals = [sig(r) for r in rows if r["estimator"] == estimator]
    vals = [v for v in vals if v is not None and v > 0]
    if not vals:
        print(f"  {label:44} n=0")
        return
    print(f"  {label:44} n={len(vals):3}  median {statistics.median(vals):6.1f}"
          f"  range {min(vals):5.1f}-{max(vals):6.1f}")


# --- staleness: is a banked sweep's provenance still true of the code at HEAD? -----
#
# `commit` and `constants_digest` — the two fields `stats/*.provenance.csv` carried
# before this — both fail at this job. `commit` is a history pointer, and this
# repository's history was squashed at 61bf00a ("FPL Armband v1", 2026-08-16); a
# commit banked before that root is not an ancestor of anything on origin/main any
# more, `git merge-base --is-ancestor` says so forever, and that is a property of
# the pointer rather than of the content. `constants_digest` hashes config.json's
# modelSubtrees and env switches (see internal/snapshot/fingerprint.go) and nothing
# else, so it cannot see a change to internal/analysis or internal/backtest by
# construction — and it moves on congestion.status_last_verified, a documentary date
# nothing reads, which is a false positive in the other direction.
#
# `watched_digest` (internal/snapshot/watched.go's `WatchedDigest`, over
# `SnapshotWatchedPaths`) is content over the code and the shipped constants
# together, and it is what `armband snapshot` already keys an accuracy snapshot's
# staleness on. This wires the same digest to sweep provenance.
#
# The comparison is one-directional and cannot be reimplemented here: the digest's
# definition lives in exactly one place, `internal/snapshot/watched.go`, and a second
# walk in Python duplicating it is precisely the "one quantity, two implementations"
# failure this project's own comments name repeatedly. So the current digest is
# asked of the Go binary that already computes it, once per run, and every sweep
# below is compared against that one answer.


def head_watched_digest():
    """WatchedDigest's composite over SnapshotWatchedPaths at HEAD, via
    `armband snapshot -watched-digest` (cmd/armband/snapshot.go).

    Returns (digest, None) on success or (None, reason) on failure. Failure is
    not fatal to the run — see the caller — because a banked cells file is
    useful evidence even on a machine that cannot build the Go binary; it just
    means every sweep reports "unknown" instead of a real comparison.
    """
    try:
        out = subprocess.run(
            ["go", "run", "./cmd/armband", "snapshot", "-watched-digest"],
            cwd=ROOT, capture_output=True, text=True, timeout=180)
    except (OSError, subprocess.TimeoutExpired) as e:
        return None, f"could not run armband snapshot -watched-digest: {e}"
    if out.returncode != 0:
        return None, f"armband snapshot -watched-digest failed: {out.stderr.strip()}"
    digest = out.stdout.strip()
    if not digest:
        return None, "armband snapshot -watched-digest printed nothing"
    return digest, None


def provenance_path(cells):
    """Mirrors snapshot.ProvenancePath (internal/snapshot/provenance.go): trim
    a trailing .csv, append .provenance.csv. Nothing outside Go read provenance
    until this script did, so this is the second implementation of that rule —
    kept to the exact same trim-and-append Go uses, not the anchored-regex form
    that once let this rule and R's disagree.
    """
    base = cells[:-4] if cells.endswith(".csv") else cells
    return base + ".provenance.csv"


def source_cells(label):
    """The cells path `stats/regenerate_mde.sh` ran for this sweep label.

    Written by that script's `run()` into `$OUT/<label>/source_cells.txt`,
    because this script only ever sees `$OUT/<label>/mde.csv` and has no other
    way to find the sidecar beside the cells file that produced it.
    """
    p = os.path.join(OUT, label, "source_cells.txt")
    if not os.path.isfile(p):
        return None
    with open(p) as fh:
        return fh.read().strip()


def banked_watched_digest(cells):
    """The most recently written `watched_digest` row in cells' provenance
    sidecar, or None if the sidecar is missing or predates this column —
    every sidecar banked before this change.
    """
    if cells is None:
        return None
    path = provenance_path(cells)
    if not os.path.isfile(path):
        return None
    digest = None
    with open(path, newline="") as fh:
        for row in csv.DictReader(fh):
            if row.get("key") == "watched_digest" and row.get("value"):
                digest = row["value"]
    return digest


def report_staleness(sweeps):
    """Print, per sweep, whether its banked watched_digest matches HEAD's —
    and an unmissable summary. Never raises and never exits non-zero: a banked
    cells file predating this change is expected to mismatch, that is a fact
    about when it was measured rather than a bug in this script, and the tool
    stays useful on every cells file already on disk.
    """
    print("\nProvenance vs the code at HEAD (watched_digest, internal/snapshot/watched.go):")
    head_digest, head_err = head_watched_digest()
    if head_err:
        print(f"  (!) could not compute HEAD's watched digest: {head_err}")
        print("      every sweep below is reported unknown as a result.")

    stale = matched = unknown = 0
    for label in sweeps:
        cells = source_cells(label)
        banked = banked_watched_digest(cells)
        if head_digest is None or banked is None:
            why = "no source_cells.txt (rerun via regenerate_mde.sh)" if cells is None \
                else ("no provenance sidecar" if not os.path.isfile(provenance_path(cells))
                      else "sidecar predates the watched_digest column")
            print(f"  {label:20} unknown   ({why})")
            unknown += 1
        elif banked == head_digest:
            print(f"  {label:20} match")
            matched += 1
        else:
            print(f"  {label:20} *** STALE ***   banked {banked} != HEAD {head_digest}")
            stale += 1

    print(f"  -> {stale} STALE, {matched} match, {unknown} unknown, of {len(sweeps)} sweeps")
    if stale:
        print("     STALE means the code or shipped constants moved since that sweep was "
              "run — the thresholds below may not describe the code at HEAD.")


def main():
    rows, cols = load()
    if not rows:
        sys.exit("no per-sweep mde.csv found under stats/out — "
                 "run stats/regenerate_mde.sh, which writes them first")

    arms = [r for r in rows if r.get("scope") == "arm"]
    os.makedirs(OUT, exist_ok=True)
    with open(AGG, "w", newline="") as fh:
        w = csv.writer(fh)
        w.writerow(["sweep"] + cols)
        for r in sorted(arms, key=lambda r: (r["_sweep"], r.get("metric", ""),
                                             r.get("variant", ""))):
            w.writerow([r["_sweep"]] + [r.get(c, "") for c in cols])

    sweeps = sorted({r["_sweep"] for r in rows})
    print(f"wrote {os.path.relpath(AGG, ROOT)} — {len(arms)} arm rows "
          f"from {len(sweeps)} sweeps")
    print(f"  {', '.join(sweeps)}")

    report_staleness(sweeps)

    metric = [r for r in arms if r.get("metric") in ("hold", "policy")]
    four = [r for r in metric if grid_seasons(r) == 4]
    settings = [r for r in four
                if r["_sweep"] not in NOISE and r["_sweep"] not in ORACLE]

    print("\nPer-arm significance thresholds, points a season "
          "(HOLD and POLICY, inert arms excluded):")
    summarise("everything on disk", metric, CLUSTERED)
    summarise("four-season grids", four, CLUSTERED)
    summarise("four-season settings sweeps  [the record's population]",
              settings, CLUSTERED)
    summarise("  the same, start-fixed estimator", settings, START_FIXED)

    # ⚠️ The six-season line is filtered THE SAME WAY as the four-season settings
    # population, and until 2026-08-15 it was not. That asymmetry printed a median of
    # 8.4 directly beneath the canonical 39 and invited the reading that six seasons
    # cut the threshold fivefold. It does not. The unfiltered thirteen rows were
    # 7 `noise6` floor arms (0.9 to 8.4 — arms built not to move, excluded by name
    # from the four-season side), 4 `teamnews` oracle rows, and 2 `vice6` control
    # rows. The median of that is the top of the noise block, and it is a statement
    # about the noise floor rather than about any setting.
    #
    # Filtered symmetrically there is **no six-season settings population at all** —
    # `vice6` alone, n=2, and that is the mechanism-certain positive control. So this
    # grid has never measured a six-season *setting* threshold, and the honest output
    # is to say so rather than to print a median of controls.
    #
    # The six-season power figure is the grid-width study's 20-26 a season on HOLD,
    # not anything derivable here. Do not quote a median off this line.
    six_all = [r for r in metric if grid_seasons(r) == 6]
    six_settings = [r for r in six_all
                    if r["_sweep"] not in NOISE and r["_sweep"] not in ORACLE]
    summarise("six-season settings sweeps   [same filter as above]",
              six_settings, CLUSTERED)
    print("    ^ controls only — see the note in this script. NOT comparable with")
    print("      the four-season median above, and not a six-season power figure.")
    summarise("six-season grids, UNFILTERED [noise + oracle included]",
              six_all, CLUSTERED)

    # Any grid width that is neither, named rather than dropped. Unlike the check
    # this replaced — which subtracted the settings population from its own
    # definition and was therefore empty by construction, a vacuous guard of exactly
    # the kind this repository keeps paying for — a five-season or eight-season sweep
    # really would land here and really is unhandled by the two summaries above.
    other = sorted({(r["_sweep"], grid_seasons(r)) for r in metric
                    if grid_seasons(r) not in (4, 6)})
    if other:
        print(f"\n  (!) {len(other)} sweep(s) on neither a four- nor a six-season "
              "grid, counted in NEITHER line above: "
              + ", ".join(f"{s} ({n} seasons)" for s, n in other))

    both = [v for v in (sig(r) for r in settings) if v is not None and v > 0]
    if both:
        print(f"\n  across both estimators pooled: min {min(both):.1f}, "
              f"max {max(both):.1f}")
        print("  recorded in harness-and-inference.md: "
              "23 comparisons, median 39, spanning 3.9 to 232")


if __name__ == "__main__":
    main()
