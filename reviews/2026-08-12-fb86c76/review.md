# Review at `fb86c76` — `prior_half_life` on the repaired archive

**Range reviewed:** `fe8bdf6..9d5774a`, plus the corrections applied in `fb86c76`.
Only `9d5774a` touched watched paths in that range (`CLAUDE.md`,
`docs/notes/constants-and-sweeps.md`).

**What the branch does.** Re-measures `prior_half_life` after the xGC repair landed,
reads a statistic the command had been emitting and nobody had read, and closes the
item as unresolved. Nothing shipped changes: no constant, no scoring path. The whole
executable diff is one `fmt.Fprintf` to stderr plus a new test file.

## The invariant came first, and it is the strongest thing here

Per the skill's opening section, before dispatching anyone: **what must this change
not move?** Nothing, and that is checkable rather than assertable.
`cmd/priorblend/main.go` is comment-only — 90 lines added, zero executable, verified
by diffing with comment lines stripped. The only behavioural change in the branch is
a warning that fires when `FPL_WEIGHT` names `prior_half_life`. That is why no
replay was re-run to accept this branch, and it is worth more than any of the
reviews below.

## Reviewers run, and why

| reviewer | why |
|---|---|
| **fpl-findings-audit** | `CLAUDE.md` and `docs/` changed — the record-only row of the triage table |
| **fpl-stats-review** | the substance is statistical claims about a measurement |
| **fpl-code-review** | one executable change plus two new guard tests |

**Skipped:** `fpl-security-review` (nothing in `internal/agent`, `internal/fpl` or
config persistence), `fpl-run-review` (no live run, no config written),
`fpl-season-maintenance` (the four hand-maintained lists are untouched).

## Findings, ranked by how misleading the state was

**1. Holm was applied only to the arms that could have supported shipping.** The
seven affirmative arms were corrected and the negative one was handed through with
the word "refutes". `t = −4.34` at df 5 is a raw p of 0.0074, which the same family
takes to 0.052. *Applied* — and it turned out to matter in both directions, see 2.

**2. The strongest negative finding was quoted off a population the file itself
retires.** `popFieldSound` carries a SUPERSEDED marker in `cmd/priorblend`. On
`popEveryone` the same arm reads `t = −4.82`, raw p 0.0048, **Holm over eight arms
0.0385** — it clears the bar the affirmative arms fail. *Applied.* Found only
because the statistics reviewer re-ran the arms rather than reading the tables.

**3. "The deciding statistic" was refuted by the experiment's own minutes-only
arm.** A uniform uplift to a selected subgroup is not nothing: it relocates that
subgroup within the whole-field ordering, which is what the optimiser consumes. The
"a bias shared by every player in a position is not an ordering error" precedent
does not transfer, because it rests on FPL's positional quotas and there is no quota
on thin-season players. Minutes-only is the proof — within-population +0.00017 at
t 0.11, whole field −0.00040 at t −4.33. *Applied*: two statistics, information and
level, and the verdict now stands on both.

**4. The centrepiece correction was itself wrong for half the grid.** "Only the
blend arm reaches back into the pre-2022-23 seasons" fails for current seasons
2020-21 through 2022-23, where the *immediate* prior is also pre-2022-23, so the
shipped baseline moved too: +0.00492 / +0.00419 / −0.00202, and exactly 0.00000 in
the last three. The asymmetry is real only for 2023-24 and 2024-25. *Applied.* Note
the shape of this: a correction of an unchecked cancellation claim, itself asserting
an unchecked mechanism.

**5. "Unmeasurable on this harness" was not earned.** Its stated reason — "the live
column is six GW1 cells" — is false; `mix()` folds the prior in at every gameweek at
`n90/(n90+BlendRateK)`, and the treated population's current-season evidence
accumulates *slower* than average, so the prior stays heavier for exactly the players
the setting touches. The general form of the argument ("effect and noise share the
discrete-squad channel") is true of every constant this file has swept and would
retire the sweep programme. It also sat fifteen lines below a retraction of the same
claim about the same feature. *Applied* — now "unmeasured and expected to stay
unresolved". **I adopted this phrase from the earlier plan review without checking
it**, which is the failure the record names repeatedly.

**6. The treated-population tail resolves the favourable way and was omitted.**
Signed error over the top twenty inside the treated group: +0.0011 / +0.0283 /
+0.0837 / +0.1467 / +0.1954 across the doses at t up to 11.78, unanimous across
seasons, Holm-surviving from 0.25 down to 0.0006. Monotone, unanimous,
multiplicity-clearing — the three things this record accepts in place of an argmax.
*Applied*, with the reading that a signed error over a distribution's top is a
**level** statistic and therefore exactly what the rescaling the ordering statistic
was chosen to discount produces. It is not independent evidence of information. A
verdict of "the affirmative case does not exist" cannot silently omit it.

**7. The test written to catch a silent no-op was itself one.** It scanned
`internal/analysis`, where the wiring will not happen: that package consumes a
`PriorSeason` interface, and the seam that would ship the feature is the
`backtest.SimConfig` literal in `cmd/fplagent/backtest.go`. Code review demonstrated
by mutation that adding `PriorHalfLife: cfg.Weights.PriorHalfLife` there left both
tests green. *Applied* — widened to the trees that build the replay's config, with
named expected sites, and re-mutation-tested at the real seam (now fails correctly).

**8. Twelve of twelve is six independent seasons.** The two archive loads share
five-sixths of their data and the repair is exactly zero in 2025-26, so that season
contributes two identical observations. Sign evidence is p = 0.031, not the 0.0005
twelve unanimous draws would imply. *Applied.*

**9. The repair-effect readings are unpaired differences of two noisy estimates.**
Backing SEs out of the published t values puts half-life 2's sign flip at t ≈ 1.0 and
the rates-channel movement at t ≈ 1.06, and the arm that flipped is one of seven —
an argmax. No paired difference and no SE on the difference was computed anywhere,
against this file's own rule. *Applied*: both downgraded from "supported" to
"readings to test", and the bold headline removed. Also recorded that the
diff-in-diff is carried by 2023-24 and that 2025-26 is structurally inert.

**Smaller, all applied:** the gofmt-alignment fragility in the declaration skip;
"populated nowhere outside cmd/priorblend and one test" (no test populates it —
`variance_test.go` reads it and always sees empty); "rates-only is the best arm on
both" (no whole-field figure for that arm is published); the family is five doses
plus a channel split, not seven independent settings; every table above the new
section in `cmd/priorblend` is pre-repair and is now marked as such; the post-hoc
label was present in two of four copies and is now in all four, with the
forecast-versus-record qualifier the mechanism needs.

## Declined

- **Widening the whole-field ladder re-run to the repaired archive at every
  half-life.** Named as a gap and marked in the file instead. It is a ten-second run,
  but it would add a fourth table to a section whose verdict does not turn on it.
  Recorded as unrun rather than quietly left.
- **Publishing the bias column for the two channel arms**, which would decide whether
  the minutes channel's fixed-size shift is a falsehood or merely mis-sized. Named in
  the record as the measurement that would settle it. Out of scope for a branch whose
  verdict is "stays off".
- **Fixing the `bench` twin defect.** `FPL_WEIGHT=bench=N` is inert on the backtest
  path for the same class of reason — `Optimize` reads `Weights.BenchWeight` only when
  the caller passes zero, and every replay call site passes non-zero. Real,
  pre-existing, and a separate change; queued rather than folded in, because it needs
  its own decision about whether to warn or to wire.
- **The stderr pipe's unclosed read end and the shared `FPL_WEIGHT: ` prefix.** One fd
  per test run and a cosmetic grep ambiguity.

## What could not be checked on this harness

- **Whether the setting is worth points.** Unmeasured, and expected to stay
  unresolved rather than unmeasurable — see finding 5 for the distinction and why the
  stronger word was withdrawn.
- **Whether the ordering gain is real.** Nothing survives multiplicity in either
  direction on the affirmative arms. Resolving it needs a single pre-committed arm
  attached to the next archive change, not another seven-arm sweep.
- **Whether the post-hoc minutes mechanism is right.** It is labelled as a hypothesis
  in all four places it appears.

## Notes for the next pass

**`CLAUDE.md` is at 163,837 bytes against a 163,840 budget — three bytes of slack.**
The next addition must displace something. This is not a warning about this branch;
it is the state the branch leaves behind.

**`go test ./...` cannot be trusted to check the review gate.** Go's test cache
served a stale PASS for `internal/snapshot` recorded before the commit existed,
hiding the gate failure completely — which is how this branch was reported as green
while failing. `TestReviewCoversTheCurrentCode` reads git state, which the cache
cannot see. **Use `-count=1` for this package.** Worth a guard of its own.

**Two of three reviewers found errors in the corrections, not only in the original.**
Findings 4 and 5 are both cases of a correction asserting a new unchecked mechanism.
The reviewers also produced claims that did not survive checking — the earlier plan
review's "inert twice over on a sweep" (`FPL_WEIGHT` reaches no sweep for any key)
and "the CSVs are already on disk" (they were in `/tmp` and gone). Verify before
applying, in both directions.
