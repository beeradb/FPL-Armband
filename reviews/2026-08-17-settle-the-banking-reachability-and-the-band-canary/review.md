# Settling the banking reachability and the band canary

Reviewed: the working tree of `settle-the-banking-reachability-and-the-band-canary` against
`d8a3eb9` (the fixture-run merge on `development`), which is this branch's base.

## What was reviewed

Two **liveness diagnostics** and the instrumentation they needed. Both were run before a tandem
sweep crossing banking × fixture runs × chip preparation is designed, because both decide whether
that experiment is well-formed at all.

1. **Question A — can the banking rule's waiting arm fire at shipped config?** A `bankProbe`
   recorded per consulted week, and `TestDiagBankingReachability` over 8 cells.
2. **Question B — the band canary.** A dose-response of the fixture mediator against
   `band_strength`, and the dose at which a flat tandem result becomes readable.
3. **The instrumentation.** An unexported `SimConfig.bankLog` hook in the `gateLog` mould;
   `bestPackageValue` split into `transferPackages` plus `analysis.BestPackage`.

**No points claim is made anywhere in this change, no threshold or p-value is quoted, and no
default moves.** `bank_transfers_lookahead` and `band_strength` both still ship off.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the deliverable is two claims about instruments that a later sweep will be read through, and this project's memory says a plan producing a number is reviewed before the number is |
| **fpl-code-review** | yes | the diff touches `internal/analysis` (scoring package) and `internal/backtest` (the harness), including a refactor of the function the shipped banking decision is made from |
| fpl-findings-audit | **skipped** | it lists `AGENTS.md` edits in its triage, and this change makes two. Both were written *from* the statistics review's corrections and were re-read by it after the fixes landed — the reviewer that would have audited them is the one that dictated them. Recorded as a deliberate skip rather than an omission |
| fpl-security-review | not applicable | no change to `internal/agent`, `internal/fpl`, the client, or config persistence. No new user-settable field |
| fpl-docs-review | not applicable | no change to `README.md` or `docs/` |

**Self-review was not performed and is forbidden.** Everything below came from the two reviewers.

## The finding that changed the deliverable

**The statistics review refused Claim A as originally written, and it was right.**

The first draft claimed *"the waiting arm CANNOT fire at shipped config"*. The reviewer's
objection was that the run establishes a **measured non-firing with an unidentified mechanism**,
not an impossibility — and that the two generalise differently. An extra-move channel reading
zero has two causes:

- the wider move limit enumerated **nothing new** (structural — `RankPairs` builds a
  multi-downgrade set only for upgrades no single funding sale can reach), or
- it enumerated something that topped the **gain** ranking `bestPair` sorts on, and then lost on
  **value** once `PackageValue` charged per move (football, and grid-dependent).

The original diagnostic could not tell them apart. **Applied**: `bankProbe.SamePackages` now
records whether the two arms produced the identical candidate list, and the re-run answers it —
**224 of 226** weeks identical, with `no_haircut < now` in **0 of 226** excluding the rival
reading directly. The mechanism is structural, so the claim is now *stronger* than the version
that was refused, and it is stated as a measured non-firing plus an identified cause rather than
as an impossibility.

**The second finding was the sharper one: the positive control was at the wrong boundary.**

`MaxHits: 0` banks 30 times and flips 72, which the first draft offered as proof that the move
limit reaches the enumeration. The reviewer pointed out that all of it sits at the **1→2**
boundary — `limit_now < 2` in 132 of 226 weeks, where `transferPackages` skips `bestPair`
outright — and that shipped `MaxHits` can **never** reach that boundary, because the weekly
accrual forces `free ≥ 1` and `MoveLimit = free + hits`. So the control was measuring a
neighbouring path, not the one under test. **Applied**: the diagnostic now prints a block
restricted to `limit_now ≥ 2`, and the same `MaxHits: 0` arm there reads **94 weeks, 94 identical
candidate lists, 0 flips**. Across both arms the extra move is inert at the shipped boundary in
**318 of 320** weeks. The control survives as a control — the lever demonstrably reaches the
enumeration — but it no longer carries weight it was not entitled to.

**And the band canary's justification was post-hoc.** The reviewer established that **dose 1 also
moves the mediator in 8 of 8 cells**, which is the diagnostic's own printed selection rule, and
that the two criteria picking 2 over it — reaching the opening fifteen in 4 of 8 rather than 2 of
8, and exposure outside the whole 0-to-1 range — were chosen after seeing the ladder. **Applied**:
`AGENTS.md` now records that both qualify, that 2 is taken as a **margin** rather than as the
bar, and that the extra criteria are post-hoc. Dose 1 is not recorded as having failed anything.

## The code review's verification

**The enumeration split is behaviour-preserving, verified empirically rather than argued.** The
reviewer replayed both the shipped arm and `bankingArm` against `origin/development` and confirmed
the seasons are byte-identical: `bestPackageValue`'s two call sites became `transferPackages`
twice plus `analysis.BestPackage` twice, with `gw+1`/`horizon-1` on the later arm and the chip
credit unchanged inside the enumeration. `analysis.BestPackageValue` now delegates to
`BestPackage`, so the value returned is the same selection by construction.

**The `bankLog` hook cannot change a decision.** Nothing branches on it; it is called after
`AdviseBank` has returned, and on the guard path after the refusal is fixed.
`TestTheBankLogChangesNoDecision` asserts it on **both** the shipped arm and `bankingArm`, the
latter because at shipped config the rule banks zero times and a season that never takes the
banked branch cannot show a change in it.

## Findings applied

| # | reviewer | finding | outcome |
|---|---|---|---|
| 1 | stats | the extra-move channel's zero is not attributable — structural inertness and a value loss are pooled | **applied**: `bankProbe.SamePackages`, re-run, 224/226 identical |
| 2 | stats | the `MaxHits: 0` control is at the 1→2 boundary, which shipped config never visits | **applied**: restricted block, 94/94/0 |
| 3 | stats | `limit_now ≥ 2` in 226/226 is **forced** by the accrual, not observed; same for `limit_later > limit_now` with `max_moves` unset | **applied**: labelled as forced in the output and in `AGENTS.md`, so it is not read as evidence about football |
| 4 | both | denominator mismatch — the allowance sums ran over 236 consulted weeks while every count ran over 226 unguarded ones | **applied**: all sums moved past the guard. The mean move limit corrected **2.627 → 2.558**, which is the reviewer's point made concrete: the guarded weeks are the high-allowance ones |
| 5 | stats | the two guards were pooled, hiding which constraint bound | **applied**: split, 3 ceiling / 7 horizon shipped, 2 / 8 at `MaxHits: 0` |
| 6 | both | `NoHaircut` is **not** sign-constrained against `Now` — `bestPair` ranks on gain while `PackageValue` charges per move, so a wider search can return something worth less | **applied**: counted (`0 of 226` in both arms), `min` added to every quantile line, and no assertion of that sign anywhere in the guards |
| 7 | stats | a `%.4f` print of `0.0000` is not an exact zero | **applied**: the output says it bounds the channel below 1e-4 and names `SamePackages` as the exact statement |
| 8 | stats | no provenance in the banking diagnostic's header | **applied**: prints `FPL_SWEEP_SEASONS`, both repair switches and the resolved oracle struct |
| 9 | code | the guard-path doc said "every field below is zero", but `Free`, `Limit` and `Horizon` are populated — and the diagnostic depends on that | **applied**: comment corrected, with the dependence named so the literal is not trimmed to match it |
| 10 | code | the chip-credit caveat was attached to `NoHaircut` only; it applies to `NoExtraMove` equally | **applied** |
| 11 | code | the probe guard hand-rolled `p.Later > p.Now` instead of calling `analysis.PreferWaiting` | **applied**: a copy would fail with "the probe has drifted" on the day someone legitimately moved the tie direction |
| 12 | code | `quantiles`' doc said upper quartile and computed p90 | **applied**, and `min` added for finding 6 |
| 13 | code | `gate.go`'s "what is deliberately NOT routed through here" still named `bestPackageValue` after the rename | **applied**: names `analysis.BestPackage` over `transferPackages`' candidates |
| 14 | both | `sweepPairNames` returns the **six**-season grid unless `FPL_SWEEP_SEASONS=default`; neither diagnostic recorded which it ran on | **applied**: printed in both headers, and `AGENTS.md` records the canary as a four-season figure that does not transport to 12 cells |
| 15 | stats | the band bullet called the exposure ladder monotone while quoting +95 at 0.5 against +93 at 1 | **applied**: the *direction* is forced by construction, the ladder is not monotone, and `better` falls 11 → 9 across the dose on the gate test's single season |
| 16 | stats | the canary licenses a null at doses ≥ 2 only | **applied**: at 0.25 — the deciding arm this record still calls unrun — the mediator moves in 6 of 8 and the fifteen in 1 of 8, so a flat result there stays unreadable in 2 cells |

One extra guard was added while fixing finding 1, not requested by either reviewer: with the
preparation switches off the two arms differ only by their move limit, so an identical candidate
list forces `NoHaircut == Now`. `TestTheBankProbeReportsTheArmsShouldBankCompared` asserts it.

## Findings declined, with reasons

- **`moveKeyOf` duplicates the local closure in
  `TestTheFixtureRunLeverReachesTheTransferDecision`** (code review). **Declined and recorded in
  place.** The two want different keys — this one uses element ids, which survive a mid-season
  name change, and the other uses names because a human reads its failure message. Each is only
  ever compared with itself, so a divergence cannot produce a wrong answer today. The comment says
  so, and says that neither copied-expression scan matches this shape, so the exposure is recorded
  rather than hidden.
- **`TestTheBandDoseCanaryDiscriminates` degrades to a skip under `FPL_MAGNITUDE`** rather than
  refusing outright the way `TestDiagBandCanary` does (code review). **Accepted as is.** Under
  that switch `BandChannelLive` is false, so `ReadyWeeks` is 0 and the existing skip fires — a
  silent skip rather than a false pass. The diagnostic, which produces the number a human quotes,
  is the one that `t.Fatalf`s.

## Not done, and named as such

- **No `internal/analysis` unit test proving `RankPairs`' multi-downgrade branch is reachable in
  principle.** The statistics review suggested constructing a squad where no single sale funds a
  premium, so that the branch is shown to exist independently of any replay. `SamePackages`
  answers the same question empirically and on the path that matters, and the `MaxHits: 0` arm
  exercises the branch 53 times — but *reachability in principle* stays unpinned by a test, and
  finding 1's conclusion rests on the empirical count alone.
- **No research-store note.** This branch and its evidence are one unit of work, and only half of
  it exists. Left to the coordinator rather than created here, because a note keyed to a branch
  name that may be renamed or folded is worse than none.
- **The accuracy snapshot is deliberately not regenerated.** `TestSnapshotCoversTheCurrentCode`
  is expected to fail on this branch; it is the coordinator's to regenerate immediately before
  merging, because any further commit invalidates it.

## A process failure worth recording

The first hand-back reported `go build ./... && go vet ./... && go test ./...` as green. **It was
not.** The suite had been run, and had passed — but *before* the final commit, and both
`TestReviewCoversTheCurrentCode` and `TestSnapshotCoversTheCurrentCode` are keyed to committed
content, so committing invalidated the result that was then reported. The claim was a stale
observation presented as a current one.

It is the same shape as the defect these two diagnostics were built to expose: **a claim of "no
movement" that was never checked against the thing that moves.** Recorded here rather than only
fixed, because the remedy is procedural — re-run the suite in full after the last commit, and
grep `^--- FAIL` explicitly rather than reading a tail, since a long diagnostic block pushes
failures out of view.
