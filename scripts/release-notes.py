#!/usr/bin/env python3
"""Write a release body in which every number was generated, not typed.

    python3 scripts/release-notes.py --figures <dir>/figures.csv --key <dir>/key.csv

# Why this exists

A snapshot Release already carries `figures.csv`, `constants.csv` and `key.csv` —
567 generated figures and a provenance stamp — and its body was one static
sentence. Everything a reader needs was attached and nothing said what any of it
meant, or what had changed.

⚠️ **The rule this follows is the registry's: a number in prose carries its
command, or it is not a number.** Every figure below is read out of the attached
CSV at generation time. Nothing is transcribed, so nothing can drift from what
shipped. Where a figure cannot be read, the note says so rather than omitting the
line — an absent row and a zero must not look alike.

# What it deliberately does NOT do

⚠️ It does not read the vault, and it could not: the vault's only remote is a
path on one machine, so CI cannot reach it. A claim that lives only there cannot
be re-derived by a reader of this repository and therefore may not appear in a
public release note. That is not a filing preference — it is the same test that
decides whether a claim is checkable at all. See `stats/claims.yaml`'s header.

⚠️ It does not judge whether a movement is GOOD. It reports what moved and by how
much. `figures-moved-check.py` is the gate; this is the record.
"""
import argparse, csv, os, subprocess, sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)


def read_csv(path):
    """figure -> raw string value. Strings are kept: not every figure is numeric."""
    out = {}
    if not os.path.exists(path):
        return out
    with open(path) as f:
        for r in csv.DictReader(f):
            fig, val = r.get("figure"), r.get("value")
            if fig and val not in (None, ""):
                out[fig] = val
    return out


def as_float(v):
    try:
        return float(v)
    except (TypeError, ValueError):
        return None


def previous_figures(tmp):
    r = subprocess.run(["gh", "release", "list", "--limit", "1", "--json", "tagName",
                        "-q", ".[0].tagName"], capture_output=True, text=True)
    tag = r.stdout.strip()
    if r.returncode or not tag:
        return {}, None
    dest = os.path.join(tmp, "prev-figures.csv")
    d = subprocess.run(["gh", "release", "download", tag, "-p", "figures.csv",
                        "-O", dest, "--clobber"], capture_output=True, text=True)
    if d.returncode or not os.path.exists(dest):
        return {}, tag
    return read_csv(dest), tag


def claims_block():
    """Re-derive the claims registry, so the note carries live values not stored ones."""
    p = subprocess.run([sys.executable, os.path.join(HERE, "claims.py"), "--emit"],
                       capture_output=True, text=True, cwd=os.path.dirname(HERE))
    if p.returncode != 0:
        return ["⚠️ The claims registry did not evaluate cleanly, so its figures are "
                "omitted rather than reported stale:", "", "```", p.stderr.strip()[:400], "```"]
    return ["```", p.stdout.strip(), "```"]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--figures", required=True)
    ap.add_argument("--key", required=True)
    ap.add_argument("--tmp", default=".")
    ap.add_argument("--threshold", type=float, default=0.005)
    a = ap.parse_args()

    cur, key = read_csv(a.figures), read_csv(a.key)
    prev, prev_tag = previous_figures(a.tmp)

    L = ["The model figures and the constants in force at "
         f"`{key.get('commit', 'unknown')[:12]}`.", ""]

    # --- what moved
    L.append("## What moved")
    L.append("")
    if not prev:
        L.append(f"⚠️ No previous snapshot to compare against (newest release: "
                 f"`{prev_tag or 'none'}`), so nothing here is a delta.")
    else:
        moved, gone, arrived = [], [], []
        for fig, v in sorted(cur.items()):
            if fig not in prev:
                arrived.append(fig); continue
            a_, b_ = as_float(prev[fig]), as_float(v)
            if a_ is None or b_ is None:
                if prev[fig] != v:
                    moved.append((fig, prev[fig], v, None))
                continue
            if abs(b_ - a_) > a.threshold:
                moved.append((fig, f"{a_:g}", f"{b_:g}", b_ - a_))
        gone = sorted(set(prev) - set(cur))

        if not moved and not gone and not arrived:
            L.append(f"**Nothing.** No figure moved by more than `{a.threshold}` against "
                     f"`{prev_tag}`, across {len(cur)} figures.")
        else:
            L.append(f"Against `{prev_tag}`, across {len(cur)} figures:")
            L.append("")
            if moved:
                L.append("| figure | before | after | delta |")
                L.append("|---|---:|---:|---:|")
                for fig, b, c, d in moved[:40]:
                    L.append(f"| `{fig}` | {b} | {c} | {f'{d:+g}' if d is not None else '—'} |")
                if len(moved) > 40:
                    L.append(f"| … | | | **{len(moved)-40} more not shown** |")
                L.append("")
            # ⚠️ Named, never silently dropped: an absent figure and an unchanged one
            # must not read alike, and a vanished figure is the more alarming of the two.
            if gone:
                L.append(f"⚠️ **{len(gone)} figure(s) present before and ABSENT now**: "
                         + ", ".join(f"`{g}`" for g in gone[:10])
                         + (" …" if len(gone) > 10 else ""))
                L.append("")
            if arrived:
                L.append(f"{len(arrived)} new figure(s): "
                         + ", ".join(f"`{g}`" for g in arrived[:10])
                         + (" …" if len(arrived) > 10 else ""))
                L.append("")
    L.append("")

    # --- the registry, re-derived rather than quoted
    L.append("## Load-bearing constants, re-derived at build time")
    L.append("")
    L.append("Not transcribed — `scripts/claims.py` ran each claim's command against this "
             "commit. See `stats/claims.yaml` for what admits a figure here.")
    L.append("")
    L.extend(claims_block())
    L.append("")

    # --- provenance
    L.append("## Provenance")
    L.append("")
    L.append("| | |")
    L.append("|---|---|")
    for label, k in (("commit", "commit"), ("recorded at", "recorded_at"),
                     ("watched digest", "watched_digest")):
        v = key.get(k)
        L.append(f"| {label} | {'`'+v+'`' if v else '⚠️ **absent from key.csv**'} |")
    dirty = cur.get("stamp.dirty")
    if dirty and dirty.lower() != "false":
        L.append("| ⚠️ tree | **DIRTY — these figures cannot be reproduced from any commit** |")
    L.append("")
    L.append("Figures: `figures.csv` · constants: `constants.csv` · stamp: `key.csv`, all "
             "attached to this release. Every number above was read from them at build time.")

    print("\n".join(L))
    return 0


if __name__ == "__main__":
    sys.exit(main())
