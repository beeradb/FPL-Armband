# The anti-residual and no-gate transfer arms

**What was reviewed.** The branch `transfer-gate-bar-and-anti-residual`, rebased on `origin/main`:
two new transfer-gate decision axes on the backtest oracle machinery, a per-package observation log,
the diagnostic grown from four arms to six, one banked sweep, and this record.

**What it produces.** A 216-cell replay and a verdict. So the plan was reviewed before a line was
written, and the finished work was reviewed on both code and inference.

## Reviewers

| reviewer | when | why |
|---|---|---|
| **fpl-stats-review** | on the PLAN, before any code | the standing rule for anything that will produce a number. It found the blocking defect below |
| **fpl-code-review** | on the finished working tree | the diff touches the backtest harness, adds an axis to a switch that already had three near-copies, and adds a field to `SimConfig` |
| **fpl-stats-review** | on the RESULT, after the sweep | the run produces a verdict against a pre-registered null |

**Skipped, with reasons.** **fpl-security-review** — no `internal/agent`, `internal/fpl`,
config-persistence or cache change; the new `SimConfig` field is unexported and nil on every shipped
path. **fpl-run-review** — no live run. **fpl-season-maintenance** — the hand-maintained lists are
untouched. **The documentation reviewer** — no `docs/` or `README` change; `CLAUDE.md` takes one
verdict line, reviewed as part of the inference review's own recommendation.

## The plan review changed the design, and it is the reason this run is interpretable at all

The task as originally specified pre-registered `ANTI − RES` against a null of **zero**. That is
wrong, and the reason is not statistical fastidiousness. The two accept sets are `{ΔR > 4h}` and
`{ΔR < −4h}`, disjoint with a dead band of width `8h` — and `h` is zero for the large majority of
offered packages, so over most of the stream the two arms **partition** it. Per offered package:

    ANTI − RES = −cov(ΔX, sign ΔR) + μ·(1 − 2p)

The design wants the first term. The second vanishes only at `p = ½` or `μ = 0`, and this record's
own priors say neither holds. **So a large antisymmetric term is manufacturable with
`cov(ΔX, sign ΔR)` exactly zero** — a configuration in which both pre-registered readings fit the
data, which is the discrimination failure a plan review exists to catch. Neither the concentration
screen nor the wild bootstrap could see it: a level bias shows as a *high* `pos`, which reads as
reassurance.

The fix was a sixth arm that accepts everything, to identify the offset in the run's own units.
**That arm was built, and it did not identify it** — see the run's own `FINDINGS.md`. The plan
review was right that the null is not zero and right that identifying it needed another arm; what
neither it nor the build anticipated is that the accept-everything arm's own budget constraints
contaminate the level it defines.

## Code review: five findings, all applied

### 1. A mis-route of the new axis to a sibling's hook passed every check — BLOCKING

The unit test's enumeration called the predicates **directly**, so it was structurally blind to the
switch, and the only switch-level assertions were two probe proposals. On that fixture
`perfectGateXPoints` answers **identically to `perfectGateAntiResidual` on both probes**, so
`case AxisTransferGateAntiResidual: return perfectGateXPoints(s, p)` satisfied both. The reviewer
demonstrated this by evaluating all four predicates rather than asserting it.

Under that mis-route the sweep prints a six-column table whose "anti" column is a byte-copy of
"on xP", and the pre-registered contrast is silently a different contrast. The accept-everything arm
was weaker still: its single probe is accepted by **two** of the four siblings.

**Applied.** `TestEveryGateAxisRoutesToItsOwnPredicate` runs every axis through `acceptTransfer` over
the whole 180-package enumeration and requires agreement with its own predicate on **every** one,
plus a pairwise-distinguishability guard so the loop cannot pass vacuously, plus a shipped-rule
guard so "falls through to `shippedAccept`" is detectable. No mis-route survives it unless two
predicates are identical functions, which the vacuity guard refuses.

### 2. The accept-everything arm had no equality liveness check anywhere — BLOCKING

Its diagnostic checks were inequalities (`allm <= bm`, `allm < rm || …`), which a mis-route can
satisfy. This matters more than finding 1 because it is not a reported arm in its own right: `C` and
`p̂` are both read off it, so a duplicated no-gate arm produces a pre-registered null computed from
an arm that is not the no-gate policy.

**Applied.** An equality check against all five other arms, with the reason stated at the assertion.

### 3. The no-gate arm takes a −4 in nearly every gameweek, which falsifies the partition premise for the one arm that identifies the nuisance

`moveLimit` bounds a week at `free + 1` moves with `max_hits_per_week` 1, and an arm that refuses
nothing exhausts it every week. Traced through `simulate.go` rather than inferred. So its offered
stream is 50-100% hit-carrying, where the sentence *"the dead band has zero width whenever `h = 0`,
which is the large majority of packages"* appeared in four places.

**This is the finding that changed the result**, not merely the code. Measured on the run: 821 hits
over 918 cell-gameweeks against shipped's 73. **Applied** as documentation at all four sites, a
`free` column read-me-first instruction in the printed guidance, the hit channel printed beside the
mediator, and — after the inference review — as the reason `T` is not sign-identified.

### 4. `p̂` printed as the null's input was a ratio of moves, where `p` is a per-package accept mass

A funded pair is one package and two to five moves; `decide`'s singles loop returns on its first
rejection, so a refusing arm forfeits the rest of the week's moves for a reason unrelated to its
accept mass. One quantity, two implementations, and the biased one was wired into the sentence that
states the null.

**Applied.** `gateStream.packageMass` is the primary figure, printed on two streams, with the move
ratio kept and labelled as a proxy.

### 5. `mustMoveForAxis` returned nil for every axis, and its comment said "all three are empty" while the catalogue had five

The harness-level liveness check runs after **every** grid; a diagnostic's own assertions run only
when someone runs that diagnostic. **Applied**: both new axes declare `moves`, which is guaranteed
by construction for the accept-everything arm, and the comment is rephrased as a rule rather than a
count.

### 6. Two hand-written lists joined by string equality — latent

`gateStreamOrder` against the `logging()` keys. They matched, and the consumer skipped a name it
could not find, so a rename on one side would print five rows with no error in the one output that
supplies the contrast's offset. **Applied**: one list, built where the arms are declared.

### Also applied from the review's "nothing found" sections

`TestTheGateLogChangesNoDecision` — the reviewer traced that the observer cannot reach a decision and
noted there was no test asserting it. There is now: a real season replayed twice, identical but for
the log, every outcome required byte-identical.

### Confirmed clean by the code review

The sign flip (`net = -net` above the hit subtraction, so the criterion is `−ΔR − 4h`, not
`−(ΔR − 4h)`); the window read from `p.Horizon` in all four copies; registration complete at every
point with the invariance lists **written out per axis** rather than shared; `SimConfig`
comparability (nothing compares whole `SimConfig`s; one `reflect.DeepEqual` in a regression test
still passes because two nil funcs compare equal); both halves of the positional zip extended; no
point-in-time leak.

## Inference review of the result: twelve findings, all applied

The three that changed what is written:

### 1. The null comparison had no standard error, and the null is built from the arms in the contrast

Comparing `ANTI − RES` to six point-estimate nulls is this record's own named failure. The correct
object is one per-cell contrast `Z = D − (1−2p)·T`, which carries the covariance for free. Done that
way nothing rejects — **but the worst corner is t −2.51 against 2.5706**, and done the naive way it
would have read **t −3.48 and rejected spuriously**. **Applied**: the `Z` table is the headline
inference, and the wording is *the test could not discriminate*, never *the null survived*.

### 2. `T` is not sign-identified, and both signs resolve

−2.066 (t −2.75) net of the hit charge, +4.361 (t +5.91) gross of it. Both clear the bar, in
opposite directions, off the same cells. **Applied**: this is written as an identification failure,
not as a range, and the `−33 to +70` span the brief was heading toward is listed under "what must
not be quoted".

### 3. The +41 gross-of-hits figure is a measurement, not a mechanism claim — and its gloss was wrong

It has a standard error and clears its own threshold. But it is a decomposition of the no-gate arm's
own outcome, not of the gate's value, and that arm moves **three** levers at once. **Applied**: the
figure is reported with its inference, and the inference "the gate's value is largely hit avoidance"
is explicitly forbidden.

Also applied: a per-cell rather than pooled hit channel (five figures moved); the leave-one-season-out
range corrected on the realised arm (−77.1 to −92.9, not the `policy_xpoints` −67 to −84); the
realised `ANTI` level and the realised `ANTI − RES` labelled as construction at the point of
quotation; Holm 3 → 5 named as bookkeeping with `RES`'s unchanged raw and wild p's beside it; the
downgrade scoped so the resolving realised-points half is not retracted with it; the per-package
log's source named and its console output banked, since none of those figures is in `cells/`.

## What ran, and the provenance

`EXP=anti-residual-gate`, `scripts/replay -run TestDiagGateOracleOnXPoints`, six seasons × six entry
gameweeks × six arms, shipped data state, exit 0, peak RSS 130 MB.

**Three sweeps were run and only the third is banked.** The first produced the numbers; the code
review's fixes then landed and the second re-ran them — **all 216 cells and every column identical**,
which is the checkable form of the observer claim. The third ran from a **committed** tree so the
banked provenance reads `dirty false`, because this record has twice complained that a snapshot from
a dirty tree cannot be reproduced from any commit.

**The reproduction guard passed exactly**, and more strongly than "every printed digit": the CR2
standard errors and wild-bootstrap p's reproduce as well, so the whole cell vector is unchanged.

⚠️ **`origin/main` moved again after the run and the branch was rebased onto it, so the SHA in the
banked provenance no longer resolves.** Checked rather than waved through: main's two
`internal/analysis` commits are comment-only and its one `internal/backtest` change is a test file,
so nothing on the scored path moved and the cells stand. The review key below is therefore keyed
after the rebase, which is why it is re-derived rather than the one written before it.

## What this change does not do

**No shipped behaviour changed.** Both new axes are `DecisionAxis` values reachable only from a
diagnostic; `SimConfig.gateLog` is unexported and nil everywhere else; the `acceptTransfer` split is
a wrapper around the same switch. `go build`, `go vet` and `go test ./...` pass.

**It does not answer the question the arm was built for.** `ANTI` is anti-informative, not
X-uninformative, so it never could alone; only the pair against an accept-everything reference could,
and that reference does not identify. That is stated in the code, in the banked findings, and in the
one line this change adds to `CLAUDE.md`.
