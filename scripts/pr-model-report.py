#!/usr/bin/env python3
"""Render the model's accuracy figures as a pull request comment, with charts.

    scripts/pr-model-report.py --figures <figures.csv> --tmp <dir> --out report.md

# Why a report here when a gate already exists

`figures-moved-check.py` fails a build whose figures moved without a
`Figures-moved:` trailer. That gate is the right shape and it stays — but it runs
on `main`, after the change has landed, and it speaks in exit codes and log
lines. This renders the same comparison **before** the merge and **as a picture**,
for the reviewer who has to decide whether a scoring change was a good idea.

⚠️ **It is a report and NOT a gate.** It never fails a build. If it ever grows a
pass/fail opinion, that opinion belongs in `figures-moved-check.py`, which
already has one and is already wired to block.

# One implementation of the comparison, not two

The baseline — the newest published snapshot's `figures.csv` — is fetched by
importing `figures-moved-check.py` and calling its own `read()` and `previous()`.
It is loaded by path because the filename has hyphens and is not a legal module
name. **Do not reimplement the download or the CSV parse here.** Two copies of
"which figures did we compare against" is exactly the divergence this repository
keeps paying for, and the gate's answer is the one that must win.

# Reading the audience

Written for a reviewer who is not a subject-matter expert on this model: the big
picture first, then the chart, then a reference table for every term used. A
figure name like `model.prediction_calibration.predicted_6_0_and_above.ratio`
means nothing on its own, so it is never printed without its plain-English gloss.
"""
import argparse
import importlib.util
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
MARKER = "<!-- pr-model-report -->"

# Calibration bands, in the order the model orders them, with the label a person
# reads rather than the slug the CSV carries.
BANDS = [
    ("predicted_under_1_0", "under 1"),
    ("predicted_1_0_to_2_0", "1–2"),
    ("predicted_2_0_to_3_0", "2–3"),
    ("predicted_3_0_to_4_0", "3–4"),
    ("predicted_4_0_to_5_0", "4–5"),
    ("predicted_5_0_to_6_0", "5–6"),
    ("predicted_6_0_and_above", "6+"),
]

GLOSSARY = [
    ("calibration ratio",
     "What players actually scored divided by what the model predicted they would. "
     "**1.00 is perfect.** Below 1.00 the model over-predicts that group; above, it under-predicts."),
    ("predicted band",
     "Players grouped by what the model *said* they would score this week — not by what they "
     "went on to score. This is the grouping a decision is actually made on."),
    ("the 6+ band",
     "The handful of players the model rates highest each week. **The transfer search picks from "
     "here**, so its calibration matters more than the overall average."),
    ("error sd",
     "How spread out the errors are within a band, once the average bias is taken out. "
     "A big number means individual predictions vary a lot even when the average is right."),
    ("bias",
     "Predicted minus actual, averaged. Positive means the model over-predicts."),
    ("MAE",
     "Mean absolute error — the average size of a miss, ignoring direction, in points."),
]


def bail(out, headline, detail):
    """Write a report saying what could not be measured, and exit 0 regardless.

    Loud on the pull request, silent to the build. The distinction matters: a
    reviewer needs to know the check did not happen, and a red X on a reporting
    job trains people to ignore red Xs.
    """
    with open(out, "w") as f:
        f.write("\n".join([
            MARKER,
            "## Model accuracy on this pull request",
            "",
            f"⚠️ **Not checked — {headline}.**",
            "",
            detail,
            "",
            "This is a reporting job and does not gate anything, so the build is unaffected. "
            "But treat this pull request as **unmeasured, not clean.**",
        ]) + "\n")
    print(f"wrote {out} (not checked: {headline})", file=sys.stderr)
    return 0


def load_gate():
    """Import figures-moved-check.py by path; its filename is not a module name."""
    path = os.path.join(HERE, "figures-moved-check.py")
    spec = importlib.util.spec_from_file_location("figures_moved_check", path)
    if spec is None or spec.loader is None:
        return None
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def bar(delta, span, width=18):
    """A diverging bar around a centre column, so sign reads before magnitude."""
    if span <= 0:
        return " " * width + "│" + " " * width
    n = min(width, int(round(abs(delta) / span * width)))
    if delta >= 0:
        return " " * width + "│" + "█" * n + " " * (width - n)
    return " " * (width - n) + "█" * n + "│" + " " * width


def chart(cur, prev):
    """A mermaid bar chart of calibration by predicted band, against the 1.00 line.

    GitHub renders mermaid natively in a comment, so this needs no image hosting
    and no external service — which also means it cannot silently break when one
    of those goes away.
    """
    labels, vals = [], []
    for slug, label in BANDS:
        v = cur.get(f"model.prediction_calibration.{slug}.ratio")
        if v is not None:
            labels.append(f'"{label}"')
            vals.append(round(v, 4))
    if not vals:
        return ""
    lo = min(0.95, min(vals) - 0.03)
    hi = max(1.05, max(vals) + 0.03)
    ones = ", ".join("1" for _ in vals)
    out = [
        "```mermaid",
        "xychart-beta",
        '    title "Calibration by predicted band — 1.00 is perfect"',
        f"    x-axis [{', '.join(labels)}]",
        f'    y-axis "actual ÷ predicted" {lo:.2f} --> {hi:.2f}',
        f"    bar [{', '.join(str(v) for v in vals)}]",
        f"    line [{ones}]",
        "```",
        "",
        "The flat line is 1.00, where the model would be exactly right. Bars below it are "
        "groups the model over-rates; bars above it, groups it under-rates.",
    ]
    if prev:
        moved = [
            (label, cur[f"model.prediction_calibration.{slug}.ratio"]
             - prev[f"model.prediction_calibration.{slug}.ratio"])
            for slug, label in BANDS
            if f"model.prediction_calibration.{slug}.ratio" in cur
            and f"model.prediction_calibration.{slug}.ratio" in prev
        ]
        worst = max(moved, key=lambda m: abs(m[1]), default=None)
        if worst and abs(worst[1]) > 1e-9:
            out.append(f"\nLargest calibration move in this pull request: the **{worst[0]}** band, "
                       f"`{worst[1]:+.4f}`.")
    return "\n".join(out)


def movement(cur, prev, tag, threshold):
    # ⚠️ Truthiness, NOT `is None`. `previous()` returns a parsed dict, and a
    # published figures.csv that is empty or header-only parses to `{}` with no
    # error — so an identity check let an EMPTY baseline fall through to the
    # comparison, match nothing, and render as "Nothing moved". An unchecked
    # comparison reading as a clean one is the exact failure the gate this
    # borrows from warns about in its own comment. Caught in review; both
    # branches now agree, and `main()` normalises `{}` to `None` besides.
    if not prev:
        return (f"### What moved\n\n"
                f"⚠️ **Unchecked, not clean.** There is no published snapshot to compare against "
                f"(newest release: `{tag or 'none'}`), so nothing here says whether this pull "
                f"request moved a figure.")
    moved = sorted(
        ((f, prev[f], v) for f, v in cur.items()
         if f in prev and abs(v - prev[f]) > threshold),
        key=lambda m: abs(m[2] - m[1]), reverse=True)
    if not moved:
        return (f"### What moved\n\n"
                f"**Nothing.** No figure moved by more than `{threshold}` against `{tag}`, "
                f"across {len(cur)} figures. This change did not alter the model's measured "
                f"accuracy.")
    span = max(abs(n - o) for _, o, n in moved)
    rows = [f"### What moved\n",
            f"**{len(moved)} of {len(cur)} figures** moved by more than `{threshold}` "
            f"against `{tag}`. Longest bar is the biggest move; left of the centre line is "
            f"down, right is up.\n",
            "```text",
            f"{'figure':<52} {'was':>9} {'now':>9}   {'':<18}0{'':<18}",
            "-" * 110]
    for f, o, n in moved[:15]:
        short = f[6:] if f.startswith("model.") else f
        rows.append(f"{short[:52]:<52} {o:>9.4f} {n:>9.4f}   {bar(n - o, span)}  {n - o:+.4f}")
    rows.append("```")
    if len(moved) > 15:
        rows.append(f"\n…and {len(moved) - 15} more.")
    rows.append("\n⚠️ **Movement is not failure.** A scoring change *should* move these. What "
                "needs explaining is movement nobody predicted — and a `Figures-moved:` trailer "
                "in the commit is how that gets recorded.")
    return "\n".join(rows)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--figures", required=True)
    ap.add_argument("--tmp", default=".")
    ap.add_argument("--out", required=True)
    ap.add_argument("--threshold", type=float, default=0.005)
    a = ap.parse_args()

    # ⚠️ EVERY path below writes a report and returns 0. This script must never
    # fail a build — that is its stated contract, and it was broken: two early
    # `return 1` paths meant an unreadable render turned into a red check with no
    # comment and no run summary, because the workflow's later steps do not run
    # after a failed step. A report that says "I could not measure this" is
    # useful; a missing report that also reds the build is not, and the gating
    # opinion belongs to `figures-moved-check.py` alone.
    gate = load_gate()
    if gate is None:
        return bail(a.out, "the comparison machinery could not be loaded",
                    "`figures-moved-check.py` could not be imported, so there is no way to read "
                    "this branch's figures or fetch a baseline.")

    try:
        cur = gate.read(a.figures)
    except Exception as e:
        return bail(a.out, "the figures could not be read", f"Reading `{a.figures}` raised `{e}`.")
    if not cur:
        return bail(a.out, "no figures were produced",
                    f"`{a.figures}` parsed to zero figures. The diagnostics reported success, so "
                    "this points at the render rather than the run — and it means **nothing on "
                    "this pull request has been checked.**")

    try:
        prev, tag = gate.previous(a.tmp)
    except Exception as e:                      # a missing baseline is not a crash
        prev, tag = None, None
        print(f"could not fetch a baseline: {e}", file=sys.stderr)
    # An empty baseline is no baseline. See movement()'s comment.
    if not prev:
        prev = None

    parts = [
        MARKER,
        "## Model accuracy on this pull request",
        "",
        "Every week this model predicts how many points each footballer will score. These "
        "figures ask whether those predictions are any good — and whether **this change** made "
        "them better or worse. They are measured on six seasons of real matches, replayed as if "
        "the model had not seen what happened next.",
        "",
        chart(cur, prev),
        "",
        movement(cur, prev, tag, a.threshold),
        "",
        "### Reference",
        "",
        "| term | what it means |",
        "|---|---|",
    ]
    parts += [f"| **{t}** | {d} |" for t, d in GLOSSARY]
    parts += [
        "",
        f"<sub>{len(cur)} figures from this branch's own run. Report only — this never fails a "
        "build; `figures-moved-check.py` is the gate that does.</sub>",
    ]
    with open(a.out, "w") as f:
        f.write("\n".join(parts) + "\n")
    print(f"wrote {a.out} ({len(cur)} figures, baseline {tag or 'none'})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
