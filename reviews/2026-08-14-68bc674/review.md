# The accuracy snapshot at `68bc674`, and why it owes no fresh review

Covers `68bc674` only: three generated artefacts under `stats/snapshots/2026-08-14-9272432/`.
No source changed.

## What this is

The staleness guard correctly noticed that `internal/backtest/simulate.go` had moved since the
previous snapshot's commit. The move is `24acdc3`, reviewed at `reviews/2026-08-14-24acdc3/`, and
this snapshot is that review's own verification step arriving one commit later — the earlier
snapshot was generated *before* `24acdc3` was committed, so it stamped the wrong parent.

## The one thing worth checking, and it was checked

The `HoldCaptaincy` per-week fields must move no measured figure. Against
`2026-08-14-832c42b`, `figures.csv` differs on **four lines, all stamp**:

    stamp.commit  832c42bb5747 -> 9272432c6b30
    stamp.dirty   true         -> false

Zero model figures move, which is the invariant `24acdc3` claimed and this is the evidence for it.

`stamp.dirty` going true to false is not a finding: this is the first snapshot on the branch taken
on a clean tree. Worth naming because a dirty stamp means the commit alone does not identify the
code that ran, so the *previous* snapshot is the weaker artefact of the two.

## Gates

`go build`, `go vet`, `go test ./...` clean at this tip.
