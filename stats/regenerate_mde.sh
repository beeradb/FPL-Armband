#!/bin/bash
#
# Rebuild every per-sweep variance table, and the per-arm threshold aggregate the
# record's "median 39 points a season" figure is computed from.
#
#     stats/regenerate_mde.sh
#
# Why this script exists. `stats/out/` is gitignored, so the tables
# `variance_components.R` writes are deleted by any clean checkout and were in fact
# lost — the queue carried "regenerate the MDE tables" as a priority item for
# exactly that reason, and the 23 per-arm thresholds behind the canonical median
# could not be quoted or re-derived while it was missing.
#
# ⚠️ That item quoted the path as `stats/out/mde.csv`, which **nothing writes** —
# see the note below on the per-sweep `--out`. The real outputs are
# `stats/out/<label>/mde.csv` per sweep plus the aggregate
# `stats/out/mde-all-arms.csv`. Corrected 2026-08-15, along with the same wrong path
# in `stats/README.md`; the wrong one had been copied between three places.
#
# ✅ **Run 2026-08-15 and it reproduces the record's canonical population.** 14 of 15
# sweeps produced tables, 264 arm rows, and the four-season settings population reads
# **n = 23, median 39.6, range 7.6-231.9** against the recorded "23 comparisons,
# median 39, spanning 3.9 to 232" — the low end of the recorded range comes from the
# start-fixed estimator, which the run prints separately at 3.8. So the canonical
# median is re-derivable from committed cells by one command, which is what this
# script was for.
#
# ⚠️ **`oraclechip` fails, and that is the arm rather than the script.** Its four
# metrics all read sd 0.000 — the arms are byte-identical, so there is no variance to
# fit and `lm` halts on NA. The `run` helper below already predicts exactly this. Do
# not "fix" it.
#
# ⚠️ **Clean `stats/out/` of foreign directories before trusting the aggregate.**
# `mde_aggregate.py` globs `stats/out/*/mde.csv`, so any per-sweep directory left
# behind by an unrelated run is absorbed into the table the canonical median is
# computed from. Three such directories were present on 2026-08-15 and were moved
# aside before this run.
#
# It is **not silent** — the aggregate prints the sweep labels it used, so a foreign
# directory does appear in that list. But it appears as one more name among fourteen,
# which is caught only by a reader who checks the list against what they ran. Read it.
#
# ⚠️ **The six-season settings figure is controls, not settings sweeps.** This note used
# to say `mde_aggregate.py:56` hardcodes `SIX = {"noise6", "vice6", "teamnews"}`. **That
# literal is gone** — `grid_seasons()` now derives the width from each row's own
# `seasons` column, so the population is computed rather than listed. What has not
# changed is the reason not to quote it: the filtered six-season line resolves to **two
# arms, both controls** (the vice-captain arm), which is why the script prints that
# warning under it. So the "six-season grids" line is not the six-season version of the
# four-season settings population beside it, and the two medians are **not
# comparable**. Read the six-season number as a statement about those three sweeps
# only. A hand-maintained membership list with no guard is the class this project has
# watched rot four times; it wants a rule, not a literal.
#
# The fix is not to un-ignore `stats/out/`: those tables are derived, and a derived
# artefact in git rots against the script that makes it. The fix is that **every
# cells file this reads is committed**, so the tables are always one command away.
# Cells are the expensive thing (a sweep arm is minutes of replay); the tables are
# seconds of R.
#
# Note `variance_components.R` writes to one directory per sweep and never to bare
# `stats/out` — see "Where the tables go" in stats/README.md. A twelve-cell demo run
# once wrote to the bare default and its figures were read as current by the accuracy
# snapshot for weeks, because the output named no source. Do not "simplify" the
# per-sweep --out away.

set -u
cd "$(dirname "$0")/.." || exit 1

OUT=stats/out
SNAP=stats/snapshots
CELLS=stats/cells
mkdir -p "$OUT"

fail=0

run() {
  local label="$1" cells="$2"
  if [ ! -f "$cells" ]; then
    # A missing input is a rotted path, not an empty result — silently returning
    # here once cost the canonical median three of its fourteen sweeps (11/184/n=10
    # instead of 14/264/n=23) and nothing downstream noticed, because the aggregate
    # still looked like a valid one. That is a different failure than the
    # oraclechip case below: there the file is present and R has a real reason to
    # refuse it. Here the file just isn't where this script says it is, so stop
    # the whole run rather than hand mde_aggregate.py a quieter population.
    echo "ERROR $label — input file missing from disk: $cells" >&2
    exit 1
  fi
  if Rscript stats/variance_components.R --out="$OUT/$label" "$cells" \
      > "$OUT/$label.log" 2>&1; then
    echo "  ok    $label"
  else
    # A sweep whose arms are byte-identical on a metric has zero variance there and
    # R cannot fit it. That is a real property of the arm, not a failure of this
    # script, so it is reported and does not abort the run.
    echo "  FAIL  $label (see $OUT/$label.log)"
    fail=$((fail + 1))
  fi
}

echo "Per-sweep variance tables:"

# The grid-width study: the positive control and the noise floor, at four and six
# seasons. Committed at 6acc5ad.
run noise4        "$SNAP/2026-08-11-6acc5ad/noise4.csv"
run noise6        "$SNAP/2026-08-11-6acc5ad/noise6.csv"
run vice4         "$SNAP/2026-08-11-6acc5ad/vice4.csv"
run vice6         "$SNAP/2026-08-11-6acc5ad/vice6.csv"

# The sweeps rescued from /tmp on 2026-08-12. Before that they existed on one
# disk, in a directory the operating system is entitled to empty.
# ⚠️ The HITS block no longer exists in the source (checked 2026-08-13: no
# want("HITS") in transferpolicy_test.go). These cells are still valid evidence of
# what was measured, and they are why rescuing them mattered — but they cannot be
# regenerated, so this arm's thresholds are unreproducible from code. Do not treat a
# committed cells file as proof its sweep still exists.
#
# hits, teamform, and anchored moved out of $SNAP/.../cells/ into $CELLS/... at
# c6fb465 ("Relocate the banked cells the R screens read as data"); the other
# seven inputs below did not move. This script kept pointing at the old path for
# all three, which the missing-file check above used to swallow as a silent SKIP.
run hits          "$CELLS/2026-08-12-4d61058/hits.csv"
run teamform      "$CELLS/2026-08-12-4d61058/teamform.csv"
run oracleminutes "$SNAP/2026-08-12-4d61058/cells/oracleminutes.csv"
run anchored      "$CELLS/2026-08-12-4d61058/anchored.csv"
run oraclearmband "$SNAP/2026-08-12-4d61058/cells/oraclearmband.csv"
run oraclegate    "$SNAP/2026-08-12-4d61058/cells/oraclegate.csv"
run oracleprices  "$SNAP/2026-08-12-4d61058/cells/oracleprices.csv"
run oraclechip    "$SNAP/2026-08-12-4d61058/cells/oraclechip.csv"
run pricetiming   "$SNAP/2026-08-12-4d61058/cells/pricetiming.csv"
run teamnews      "$SNAP/2026-08-12-4d61058/cells/teamnews.csv"

# The frozen minutes-half-life fixture. It is the worked example
# `variance_components.R --selftest` pins, so its per-arm thresholds are
# regression-tested independently of this script. ⚠️ Its header has 23 columns where
# a current sweep writes 27 and it has no provenance sidecar: it is here because the
# record's per-arm table was computed from this sweep, NOT because a constant may be
# re-derived from it. See "Which cells files can be used" in stats/README.md.
run minhl_frozen  stats/testdata/minutes_half_life_cells.csv

echo
python3 stats/mde_aggregate.py

if [ "$fail" -gt 0 ]; then
  echo
  echo "$fail sweep(s) produced no tables — see the logs named above."
fi
