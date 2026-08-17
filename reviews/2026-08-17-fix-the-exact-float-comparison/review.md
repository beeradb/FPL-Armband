# Exact float comparison, and what it revealed about the record

Branch `fix-the-exact-float-comparison`, based on `23c2950`. One commit, this record riding
with it.

## What was reviewed

Two unit tests compared `float64` for exact equality on a recency-weighted figure. On GitHub's
runner they read `90.00000000000001`, so **CI had been red on every commit for at least eight
consecutive commits** while passing on the machines the work is done on.

The fix is a tolerance. What it turned up on the way is worth more than the fix.

## The diagnosis, and the two hypotheses that were wrong

`newRecentIndexWith` computes `w = math.Pow(0.5, (through-gw)/halfLife)`, then `mins += minutes*w`
and `den += w*fx`. For the toy player — 90 minutes in each of two gameweeks, one fixture each,
`through` 2, `halfLife` 4 — that is `(90w + 90)/(w + 1)`, which is 90 mathematically.

Two hypotheses were formed and **both refuted by execution**:

1. **FMA contraction.** `mins += minutes*w` is a fusable multiply-add and Go fuses on arm64 but
   not amd64. Computed both fused (`math.FMA`) and force-unfused (an explicit `float64`
   conversion, which the spec says prevents fusion): **both give exactly 90.** The code reviewer
   went further and showed the refutation is a theorem here, not an observation — the first
   iteration fuses an addend of exactly zero and the second multiplies by exact operands, so
   given the same `w` both architectures *must* agree.
2. **Accumulation order.** Refuted by reading: `simulate.go` walks an ordered integer loop with
   map lookups by key, and each player writes a distinct entry.

What does reproduce it: perturbing `math.Pow(0.5, 0.25)` by minus one ulp. This machine is arm64
and returns `0x3feae89f995ad3ae`; one ulp lower is the *correctly rounded* 2^-0.25, and feeding
that weight through the expression yields `0x4056800000000001` = 90.000000000000014211 — bit for
bit what CI reports. Only that weight reproduces those bits within ±4 ulps.

So: **Go's `math` is not bit-identical across machines**, dev here is arm64 and CI is amd64. The
cause is structural — `Exp` has per-architecture assembly, `Log` has it on amd64 only, and amd64's
`Exp` branches at run time on `cpu.X86.HasAVX && cpu.X86.HasFMA`, so two amd64 CPUs can disagree
from one binary.

⚠️ **The amd64 half is inferred from CI's value, not executed.** There is no amd64 host here.

## What must not move, and did not

The production code. This change is test-only plus two record edits; `git diff` touches no
non-test Go file. Verified by mutation rather than by reading:

- **The tolerance accepts the failure**: |90.00000000000001 − 90| = 1.4e-14 against 1e-9.
- **The tolerance still catches a real error**: scaling `mins` by 1.000001 — a 9e-5 error on a 90,
  five orders below one minute — makes **both** relaxed assertions fail. So they were not
  neutered.
- **A structural mutation is still caught**: changing `den += w*fx` to `den += w` is not caught by
  the three tests touched here, but is caught by `TestASecondSetChipActuallyPlays`. Recorded as a
  pre-existing coverage observation, not fixed.

`go build`, `go vet`, `gofmt` and the full suite are clean but for
`TestSnapshotCoversTheCurrentCode`, which fires on `config.json` — untouched here — and is the
CI-owned condition described below.

## Reviewers

| reviewer | ran | triage |
|---|---|---|
| **fpl-code-review** | yes | the diff changes a test guard; asked to attack the diagnosis rather than accept it |
| **fpl-stats-review** | yes | not for the tests — for whether architecture-dependent floating point undermines the reproducibility of banked cells |
| **fpl-findings-audit** | yes | `AGENTS.md` was edited, which is this reviewer's row |
| fpl-security-review | **skipped** | no production code, no credential path, no input surface |
| fpl-docs-review | **skipped** | `README.md` and `docs/` untouched; the record edit went to findings-audit, whose row it is |
| fpl-run-review | **skipped** | not the output of a live run |

## Findings applied

Every one was verified against the code or the banked cells before being applied.

**1. `sameMinutes` duplicated `closeEnough`, which already exists in the same package** with the
identical 1e-9 constant (`xpointsweek_test.go`). That is this project's named signature failure,
one quantity with two implementations, and the two scans that exist for it are idiom tripwires
that do not reach this shape. Now delegates, so the constant lives once.

**2. The class fix missed the mirrored spelling, and it was the direction that fails silently.**
`if a.MinutesPerMatch == b.MinutesPerMatch` is a *must-differ* assertion, so a last-bit difference
makes it **pass**. Harmless today (0 against 90); the day the window regresses toward a season
average it evaporates. Now tolerant, with a comment saying why. **The change was under-reach, not
over-reach.**

**3. The same landmine exists one package over**, `internal/recent/recent_test.go`, surviving only
because `weigh(h, 1, 2)` has `through == Round` so the weight is `math.Pow(0.5, 0)` and hits the
exact `y == 0` special case. Fixed, since leaving a known landmine to be stepped on later is worse
than a one-line edit.

**4. Three errors in my own `AGENTS.md` entry**, all caught by the audit and all confirmed at the
source before correcting:
- It named **priors and team strength** as exposed on `Score`. Both are refuted: `prior_half_life`
  ships at 0, so `BlendPriors` sets `halfLife = 1` and the exponent is an integer, and Go's `Pow`
  takes integer exponents through exact repeated squaring without ever calling `Exp(yf*Log(x))`.
  Both team-strength sites ship off behind `FPL_MAGNITUDE`/`FPL_TEAM_FORM`. **This is precisely
  the trap the `recent_test.go` comment written in the same commit warns about**, reproduced two
  files away.
- It **omitted** the Poisson saves and concede blocks and `defconCleanFactor`, which are exposed,
  and failed to note that `reliabilityFrom` is exposed for **midfielders only** — the other three
  positions ship an exponent of exactly 1.
- It **contradicted itself**: "swapping the transcendental moves every banked cell" against "the
  points columns reproduce unless a decision flips" two sentences later. True only of the float64
  `xpoints` columns.

**5. The `teamBands` correction narrated the replacement and smuggled a magnitude.** `AGENTS.md`'s
own Conventions section forbids annotating a withdrawal in this file, and quoting "about 0.7 a
season … understates fourfold" invites the reader to compute 2.8 — a per-season figure this
review cannot support, because cell counts do not transport to magnitudes across metrics with
different dispersion. Rewritten to carry the counts and no magnitude. Also: "understates"
mis-framed a scope difference as an error — 3 of 36 was correct *for `hold_points`*.

**6. `squad_hash` is not "integer-quantised"** — it is a digest of the fifteen, which is
*insensitive* to any float perturbation short of a membership change. A stronger guarantee than
quantisation, and worth stating as the thing it is.

**7. The CI claim would have looked falsified on merge.** Fixing these two assertions does **not**
turn CI green: `TestSnapshotCoversTheCurrentCode` is red alongside them for an unrelated reason.
Now said explicitly, because a true finding that appears refuted the moment it lands is the worst
way to lose one.

**8. "unmeasured, not unmeasurable" is right, and the route is nameable** — CI runs amd64, so one
sweep there diffed against a banked arm measures the decision-flip rate. Named, so the label is
actionable rather than a shrug.

**9. The budget was raised from 106 KB to 108 KB, in this commit, naming the claim** — which is
what `TestTheResidentIndexStaysSmall`'s own failure message instructs, and its rule is that
deleting a qualifier to fit is the failure the constant exists to prevent. The qualifiers *are*
the entry here: unhedged, it would record a mechanism argument as a measurement.

## Verified independently, against the banked cells

The stats reviewer asserted `AGENTS.md` understates `teamBands`. Rather than take it, the two
banked runs in `stats/snapshots/2026-08-16-band-strength/cells/` — same commit, same constants
digest, different `run_id`, so a diff between them is pure non-determinism — were recounted here:

| arm | squad_hash | hold_points | policy_points | policy_xpoints | hold_xpoints | moves | hits |
|---|---|---|---|---|---|---|---|
| `band_strength 0` (ships) | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| `band_strength 1` | 1 | 3 | 12 | 13 | 6 | 7 | 7 |

All of 36 cells. So the recorded "3 of 36" is exactly the `hold_points` count, `POLICY` moves 12,
and `squad_hash` moves in 1 cell against `hold_points`'s 3 — **two cells re-scored an unchanged
fifteen.** The shipped arm is inert in every column, and inertness is additionally a *code* fact:
`attackBandAdj` returns 1 before `teamBands` is ever called, verified by reading.

## Findings declined

**The decision-flip rate across architectures is not measured here.** The stats review's judgement
is that on any recorded effect size it is indistinguishable from noise by twelve or more orders of
magnitude — but that is an order-of-magnitude argument from an unmeasured flip rate, explicitly
not offered as settled. The mechanism is live and unblocked: `byScoreDesc` compares raw float64
with strict `>`, `cutByExpectedMinutes` is a cliff, and the transfer search is an argmax. **The
record now says unmeasured and quotes no magnitude.** Measuring it means one two-arm 36-cell sweep
on the amd64 runner diffed against the arm64 bank — roughly six minutes of runner time. Owed.

**Recording `GOARCH` in sweep provenance.** Proposed and not done. It is bookkeeping, it costs
nothing, and without it every future cross-machine question is retrospectively unanswerable —
including for cells already banked, which are now permanently unlabelled. Owed, and it should
happen regardless of how the measurement above comes out.

**A scan for exact float comparison in test files.** This project's own preferred shape for a
class of defect is a scan, not a runtime test — `TestNonlinearTransformsScoreTheModelsOwnRegressor`
and `TestTheCopiedExpressionsHaveOneImplementation` are the precedents. Such a scan would make the
new `AGENTS.md` entry honour its section's contract ("each now covered by a regression test"),
which it currently does not; the entry says so in its own words. Declined as scope — building a
new scan inside a two-test fix is how a small change becomes a large one.

**`snapshot.yml` runs the accuracy diagnostics on amd64** and publishes as a release asset, so
there is now an amd64 series of the same figures alongside the arm64 in-repo ones, and nothing
labels either. `AGENTS.md` says "Take the figure from the newest snapshot, not from here" without
saying which. Real, out of scope, owed.

**The second CI failure.** `TestSnapshotCoversTheCurrentCode` fires because `config.json` moved
past the newest *committed* snapshot, and `snapshot.yml` now publishes to releases rather than
committing — so the committed series can never advance again and that test cannot pass in CI by
construction. Fixing it means either setting `FPL_SNAPSHOTS_EXTERNAL=1` and removing the 98
committed snapshot directories, or reversing the publish design. **That is a decision about
deleting banked figures and is not this branch's to take.**

## What could not be checked on this harness

- **An amd64 execution of anything.** No amd64 host, no qemu. The amd64 half of the diagnosis is
  inferred from CI's reported value plus an enumeration showing only the correctly rounded weight
  reproduces those bits within ±4 ulps. Strong, but inferred.
- **Whether the two amd64 `Exp` paths actually produce different bits.** The runtime branch on
  `useFMA` is verified to exist in the Go source; that the branches differ numerically is
  inferred. "Can disagree" is the right modal and is what the record says.
- **The retired "about 0.7 a season" `teamBands` figure.** Not re-derivable — it is not in the
  snapshot, and both provenance files carry `dirty: true`, so the commit hash does not pin the
  code those runs used. The replacement quotes no per-season figure rather than carrying one
  forward that cannot be checked.
- **Whether the snapshot-staleness failure was present on all eight red commits.** Verified on the
  most recent; the earlier ones were confirmed only for the float assertions.
