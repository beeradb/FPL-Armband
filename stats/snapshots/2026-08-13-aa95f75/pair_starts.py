import csv, os

D = ("/home/bbowman.guest/fpl/.claude/worktrees/prior-half-life-on-repaired-xgc/"
     "stats/snapshots/2026-08-13-aa95f75/cells")

def load(name):
    rows = list(csv.DictReader(open(os.path.join(D, name))))
    return {(r["season"], r["start_gw"]): r for r in rows}

on, off = load("starts-on.csv"), load("starts-off.csv")
keys = sorted(set(on) | set(off))
print(f"cells: on={len(on)} off={len(off)} paired={len(keys)}")

# Every populated outcome column, not just the two headline metrics: a repair that
# moved transfer count while leaving points alone would still be a data state.
cols = [c for c in on[keys[0]]
        if c not in ("sweep", "run_id", "variant", "variant_index", "is_baseline",
                     "season", "prior_season", "start_gw", "weeks", "bank_up_to",
                     "infeasible", "oracle", "oracle_kind")]
cols = [c for c in cols if any(on[k].get(c, "") != "" for k in keys)]

diff = {}
for k in keys:
    for c in cols:
        a, b = on[k].get(c, ""), off[k].get(c, "")
        if a != b:
            diff.setdefault(c, []).append((k, a, b))

print(f"outcome columns compared: {len(cols)}")
if not diff:
    print("\nRESULT: byte-identical in all 36 cells on every populated outcome column.")
    print("The prediction holds: at shipped config nothing in the scoring path reads Starts.")
else:
    print(f"\nRESULT: {len(diff)} column(s) differ — a data state has arrived.")
    for c, items in sorted(diff.items()):
        print(f"  {c}: {len(items)} cells differ")
        for k, a, b in items[:4]:
            print(f"     {k}  on={a}  off={b}")
