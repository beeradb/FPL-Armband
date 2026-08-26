# The decomposition, re-run CLEAN at the commit its dirty sidecar stamped

`TestDiagAnchoredChipDecomposition` at **`3b8bf1ab`**, working tree verified
clean, all four blocks recording `dirty=false`.

## Why this exists

`stats/cells/2026-08-25-f7d2be1b/decomposition.csv` records **`dirty=true` for
blocks 2, 3 and 4** — bench boost, free hit and triple captain, which is three of
the four per-chip figures the 2026-08-25 record quotes. Only block 1 (wildcard,
at `a0f20a01`) was clean.

The uncommitted delta was never committed, so **what** it contained is
unrecoverable. **Whether it mattered is not** — check out the stamped commit
clean, re-run, compare. That is this directory.

⚠️ It was nearly not possible. `3b8bf1ab` is on no branch reachable from
`origin/main`: PR #83 was squash-merged and its commits are orphaned. The object
survived in one local clone. `002de39` was in exactly that state before a
`git gc --prune=now` destroyed it permanently, which this record already carries
as a closed loss.

## Result: all 288 cells identical

| | |
|---|---|
| cells matched | **288 of 288** |
| POLICY values differing | **0** |
| HOLD values differing | **0** |
| max absolute delta | **0** |

Per block, POLICY differing: #1 0 of 72, #2 0 of 72, #3 0 of 72, #4 0 of 72.

**So +5.6, +14.5, −0.75 and +6.4 stand.** The uncommitted delta touched nothing
the scored path reads, which is consistent with `a0f20a01..3b8bf1ab` being one
commit against `captainweekskill_diag_test.go` alone.

⚠️ **Block 1 reproducing is the load-bearing half of this check.** It was banked at
`a0f20a01` and re-run here at `3b8bf1ab`; if it had moved, the problem would be
determinism, not a dirty tree — a worse finding and a different one.

## ⚠️ What this does and does not change

**Does**: the three figures are **confirmed by re-run**.

**Does NOT**: make the original sidecar clean. `f7d2be1b/decomposition.provenance.csv`
still records `dirty=true` and always will. The correct form is *"confirmed by
re-run at `3b8bf1ab`"* — never *"clean"*.

⚠️ **It also does not license reading `dirty=true` as harmless.** This delta was
off the scored path; the next one need not be. What the flag costs is a re-run
instead of a lookup, and here that re-run was one `git gc` away from impossible.

⚠️ **These cells do NOT replace `f7d2be1b`'s**, which remain the record of what
PR #83 measured. Banked separately for that reason.
