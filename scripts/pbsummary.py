"""Print the prior-blend headline: levels with the derived spread, then the paired table.

Reads only what stats/prediction_inference.R writes and prints; computes nothing
that the R script does not already produce, except the pooled spread, which is
sqrt(rmse^2 - bias^2) and is the identity the bias-versus-variance rule is stated in.

    python3 scripts/pbsummary.py /tmp/pb.csv "injury shaped: " [points|minutes]
"""
import csv
import math
import re
import subprocess
import sys

csv_path, pop = sys.argv[1], sys.argv[2]
target = sys.argv[3] if len(sys.argv) > 3 else "points"

r = subprocess.run(
    ["Rscript", "stats/prediction_inference.R", "--out=/tmp/pbsummary",
     "--target=" + target, "--population=" + pop, csv_path],
    capture_output=True, text=True)
if r.returncode:
    sys.exit(r.stdout[-2000:] + r.stderr[-2000:])

print("===", pop, "|", target)
rows = [x for x in csv.DictReader(open("/tmp/pbsummary/prediction_levels.csv"))
        if x["category"] == "all categories"]
print(f'{"arm":<40}{"n":>8}{"mae":>9}{"rmse":>9}{"bias":>9}{"spread":>9}')
for x in sorted(rows, key=lambda x: x["variant"]):
    b, rm = float(x["bias"]), float(x["rmse"])
    sd = math.sqrt(max(rm * rm - b * b, 0.0))
    print(f'{x["variant"]:<40}{int(float(x["n"])):>8}'
          f'{float(x["mae"]):>9.4f}{rm:>9.4f}{b:>9.4f}{sd:>9.4f}')

# The paired tables, verbatim from R. print() wraps wide frames, so the rows and
# their numbers arrive in two blocks; they are printed as R emitted them.
out = r.stdout
for head, tail in [("EACH ARM AGAINST THE SHIPPED BASELINE", "ORDERING AND THE TAIL"),
                   ("THE SAME TWO SCALARS, PAIRED WITHIN THE GAMEWEEK", "wrote prediction_levels")]:
    if head not in out:
        continue
    blk = out.split(head)[1].split(tail)[0]
    keep = [l for l in blk.splitlines()
            if re.match(r"^\s*(\[|comparison|statistic|estimate|se_gw|p_holm|[-0-9. e+]+$)", l)
            and l.strip()]
    print("\n--", head)
    print("\n".join(keep))
