# The merge of `origin/main` at `6fee48c`, and why it owes no fresh review

Covers the merge commit `b397c93` only. Everything this branch contributes was reviewed at
`d6a61e8` (`reviews/2026-08-14-d6a61e8/`); this record exists because the staleness guard correctly
noticed seven watched files moved, and all seven arrived from the other side.

## What came in, and where it was reviewed

Four commits: `9dbeb39` (FPL's explicit zeroes no longer enter a prior as measured observations),
`1b7a27b` and `ea9dae6` (its review and snapshot), `6fee48c` (the review record). The incoming work
carries its own record at `reviews/2026-08-14-ea9dae6/`, which states that the review caught a live
bug the committed fix had missed. **Re-reviewing it here would be re-reviewing a reviewed branch.**

The watched files it moved — `internal/analysis/priorblend.go`, `internal/fpl/types.go`,
`internal/backtest/simulate.go`, `internal/agent/tools.go` and the snapshot artefacts — are all
theirs. This branch touches none of them.

## The one thing worth checking, and it was checked

`9dbeb39` changes how a multi-season prior is built, and this branch's whole subject is
`BlendRateK`, the constant that weights that prior. So the question is whether the incoming fix
invalidates the cells banked here.

**It does not, and the reason is a path distinction rather than a judgement.** `blendPast` — the
function the fix changes — is called from exactly one place, `internal/recent/priors.go:73`, which
is `recent.LoadPriors` on the **live agent path**. The replay builds its priors through
`newPriorIndexMulti` from `Season` structs and never reads `HistoryPast`. So the fix cannot reach
`stats/snapshots/2026-08-14-blend*/cells/`, and the three runs banked on this branch stand as
measured.

⚠️ Checked by naming the consumer rather than by assuming the package boundary — which is this
record's own rule, and the one that has failed here before.

## No conflicts, and the suite is clean

The merge applied without conflict. `go build`, `go vet` and `go test ./...` all pass on the merged
tree, and `CLAUDE.md` is 69,591 bytes against the 69,632 budget.

## What this merge is for

`main` moved from `9bba522` to `6fee48c` while this branch's last run was in flight, which made a
fast-forward impossible. Merging it back in restores that: `origin/main` is now an ancestor of this
tip, so `git merge --ff-only` will take it.
