# The defensive fixture hindsight gate

## What was reviewed

`origin/main..HEAD` on `defensive-fixture-coefficient-hindsight-gate` — a
pre-registered, archive-side calibration fit asking whether the banked defensive
fixture coefficient `b2 = 1.5688` (`stats/cs_calibration.R`'s two-channel
decomposition) is an artefact of post-cutoff information in `def`.

Three commits: the pre-registration and the frozen input (`55493c7`), the fit
(`b563b76`), and the corrections this review produced.

**No Go code changed.** `HOLD` and `POLICY` are byte-identical by construction,
no replay cells were spent, and no points figure is produced anywhere.

## Which reviewers ran, and which were skipped

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the triage row for `stats/*.R` — this is entirely an inference change |
| **fpl-findings-audit** | yes | same row; and the run produces a verdict that would otherwise be written into the record at the wrong strength |
| fpl-code-review | **skipped** | no `internal/` change at all. The triage row that would pull it in (`internal/analysis`) is not touched |
| fpl-security-review | **skipped** | no `internal/agent`, `internal/fpl`, config persistence or cache path touched |
| fpl-run-review | **skipped** | no live run, no config written |
| fpl-season-maintenance | **skipped** | none of the four hand-maintained lists touched |

## The invariant came first

Per the skill's opening section, the question asked before dispatching anybody was
*what quantity must this change not move*. Two answers, both now tested rather than
asserted:

1. **The refit must reproduce the banked fit.** `stats/defensive_fixture_hindsight.R`
   now `fail()`s unless `b2` and the season × team CR2 SE match the banked figures
   on both strata. **Proven to fire**: dropping 2023-24 from the input aborts the
   run with `CONTROL FAILED ... b2 = 1.6245 (want 1.5688)`.
2. **The frozen input must not move when the join's reporting grows.** The
   regenerated `joined_rows.csv` is byte-identical to the one committed with the
   pre-registration, checked by `diff`.

## Findings, ranked by how misleading the current state was

### 1. The canary's sizing was an approximation, and it decided the verdict — APPLIED

Both reviewers reached this independently and I verified it before applying.
Pre-registration §7 sizes a full artefact as `(b2−1)/q_w`, an **OLS
omitted-variable formula** applied to a log-link GLM without partialling `w1` or
using the IWLS weights. It reads **1.977**. The quantity that sentence *names* —
the `b3` which, when omitted, drives the two-channel `b2` to the banked 1.5688 —
is **1.685**, solved exactly by refitting the misspecified model to expectations
from the candidate DGP. The detection threshold is **1.702**.

`1.702 > 1.685`, so **the canary fires and the verdict is C — UNMEASURABLE**,
where the approximation gave D. The error ran in the direction that flatters the
instrument.

Verified here rather than taken on report: `b3 = 0` returns `b2 = 1.000000`
exactly (the self-consistency check that the construction is right), the root is
monotone, and `uniroot` gives 1.684807.

**A second, independent route reaches the same place.** `clubSandwich`'s
Satterthwaite df for `b3` on this fit is **1.72**, not the pre-registered `G−1 =
2`, giving a threshold of 1.997 — which fires even against the *erroneous* 1.977.
The pre-registered df is the more generous of the two, and the generosity was
load-bearing.

**Why changing the verdict after the fit is legitimate here**: it is the same
estimand computed without an approximation the pre-registration never defended,
not a new rule; and it moves the verdict toward quoting *less*. The
pre-registration's own bar is that no verdict may be **upgraded** after the fact.
The pre-registration was **not edited** — a pre-registration edited after its fit
is no longer one. All corrections print in `fit.txt` under a dated post-fit
heading.

### 2. `p.adjust` silently shrank the Holm family — APPLIED

`p.adjust(ps[!is.na(ps)], "holm")` runs at `n = 1` when an arm is unestimable, so
it applies **no correction at all** — the exact opposite of the sentence printed
one line below it, while `length(ps)` still prints `m = 2`. Verified:
`p.adjust(c(H1=0.166, H2=NA)[!is.na(...)], "holm")` returns 0.166 against 0.332
with `n = 2`. Latent here because both arms estimated; it would have bitten
precisely in the case the comment is about. Fixed to `p.adjust(ps, "holm", n =
length(ps))`.

### 3. The mechanism argument was broader than it needed to be — APPLIED

Both commits said the strength-to-difficulty step is unverifiable. The *direct*
test is indeed impossible — the captures hold no fixtures payload. But an indirect
one was already sitting in the committed join and is decisive. On rows where the
opponent's coarse strength moved, `def` tracks the **end-of-season** value far
better than the cutoff value: Spearman **0.872 against 0.421** native, 0.866
against 0.518 pooled. If FPL's difficulty were frozen at season start, `def` would
track the cutoff value. **The archive's difficulty column is end-stamped**, which
is what the replay reads.

Supporting: `def` is a per-club-per-venue constant for the whole season in every
season (0 of 240 cells vary), and re-deriving `def` independently from the
archive's own `fixtures.csv` agrees on **2974 of 2974 rows** — which
simultaneously establishes the join key, that `band_strength` was 0, that
`defFixtureScale` was 1, and §1's "pure function of `team_h_difficulty`".

⚠️ **Deliberately not over-claimed.** This shows the *archive's* column is
end-stamped. It does **not** show FPL's *live* difficulty moved in-season, which
still needs a fixtures payload. The residual alternative — difficulty set once
from information the coarse `strength` field only caught up to later — is named in
the output.

### 4. The stated liveness bar could not fire — APPLIED

§6 cited the `def` distribution reproducing the banked one. The join copies `def`
unchanged and `fail()`s on any unmatched row, so conditional on the row count that
is **bit-identical by construction**. Replaced in the reporting by the independent
re-derivation above, which can fire and passes.

### 5. H2 was labelled null where it is unmeasurable — APPLIED

H2's own threshold is **1.6516** against the **0.5688** excess being tested — it
could only ever have seen an effect 2.9× the one in question. Now labelled
unmeasurable-by-its-own-threshold, with its p explicitly not quotable. It keeps its
Holm slot because it was declared, and the output notes the Holm bar was not
binding (H1's raw p 0.166 fails at `m = 1` too).

### 6. `n` over-rated the primary arm sevenfold — APPLIED

`fit.txt` printed `n = 1566` on the same line as `b3`. Only **221** rows are both
revised and `def ≠ 1`; 675 rows have `def == 1`, where `w2 = w3 = 0` and only the
intercept and `b1` are informed. Now printed, with `cor(w2, w3) = 0.532`.

### 7. The signed specification was cited as counter-evidence — APPLIED

The commit listed `w3s = −0.3093` as one of two readings against the leak. It
carries `w3s` **instead of** `w3`, so it constrains the up- and down-revised
excesses to be equal and opposite — a shape that cannot express the unsigned
model's own point estimate of both groups elevated. The leak mechanism is
direction-agnostic. Now labelled uninformative rather than counter-evidence. This
cuts against the branch's own framing, which is why it was worth applying.

### 8. Two unnamed alternatives, and one omitted contrary observation — APPLIED

Timing (`revised_opp` proxies "early in the season": mean gw **18.08** revised
against **22.76** unrevised, verified), outcome conditioning (`revised_opp` is a
function of end-of-season strength, set from results including this row's own
match), and the banked `cs_calibration` line that runs *against* the hypothesis —
"two live captures three days apart show 0 of 380 difficulties changing" — which
the pre-registration quoted the other half of.

### 9. §9's "nothing here reaches the scored path" — APPLIED as a correction

Wrong as a mechanism claim. `FPL_DEF_FIXTURE_SCALE` runs through
`defenceMultiplier → fixtureMultipliersFor → fixtureSensitiveAt` and is live on
the scored path. What the record holds is that *moving* it does not resolve on
points, which is a tie and not a refutation. Corrected in `fit.txt`; §9 is to be
read as "nothing here licenses moving a scored constant".

### 10. Rank-deficiency was invisible — APPLIED

The augmented native CR2 matrix has **rank 3 of 4**: the meat is a sum over three
clusters and cannot span four parameters. Computable, and therefore not caught by
the pre-registered CR0 fallback, which guards non-computability. Both the rank and
the Satterthwaite df now print with every fit.

## What was declined

- **Re-labelling the verdict D on strict pre-registration literalism.** The number
  1.98 was declared in advance, so a literalist reading keeps D. Declined because
  §8's branch C is worded on the *canary firing*, not on the number 1.98, and the
  1.98 was a mis-evaluation of the quantity §7 defines. Both readings are printed
  so a future reader can take either.
- **Editing the pre-registration in place.** Every correction goes in `fit.txt`
  under a dated post-fit heading instead.
- **Adopting Satterthwaite df for the verdict.** `G−1` was pre-registered. The
  Satterthwaite df is printed as context and noted as the *less* generous choice,
  so the disclosure costs nothing and swapping it would be a post-hoc estimator
  change.
- **Building the point-in-time `def` refit** that both reviewers recommend as the
  test with real power. It is out of scope for a gate that was asked to settle one
  question, and it is a new design that deserves its own pre-registration. Recorded
  below as what would resolve it.
- **Any edit to `CLAUDE.md`.** Forbidden by the task. The reviewers' proposed record
  edits are reported to the dispatching agent instead.

## What could not be checked on this harness

- **Whether FPL's live `team_h_difficulty` moved in-season.** The captures hold
  `bootstrap-static.json.gz` only. This is *unmeasurable on this archive*, not
  unmeasured — and it becomes measurable the moment the weekly capture takes a
  fixtures payload.
- **The size of the leak.** That is the verdict: `b3` is identified off 221 rows
  across three seasons, and no estimator choice changes that. C is a fact about the
  instrument, not about the constant.
- **Whether the revisions are outcome-shaped.** The counts and the wave structure
  reproduce from `data/captures/`; "outcome-shaped" rests on a script that lives on
  another branch and is the motivating hypothesis rather than a checked fact.

## What would resolve it

Not more cells and not a wider grid. Two candidates, in order of cost:

1. **Refit `b2` on a `def` built from the opponent's cutoff strength**, using all
   1566 rows instead of an interaction identified off 221. Minutes to run, no
   replay cells. Needs its own pre-registration.
2. **Add the fixtures payload to the weekly capture.** That turns the mechanism
   argument into a measurement and makes `def` gateable by cutoff, at which point
   the question is answered by construction. Same shape as the recorded "the weekly
   capture yields nothing this season, and unblocks four questions by next spring".
