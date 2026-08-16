# Review record — running replay sweeps in parallel

**Commit reviewed:** `c54917d`, on `worktree-replay-concurrency`. Range `9fe14b5..c54917d`,
where `9fe14b5` is the commit the previous newest record was taken at.

**Why this record exists.** `TestReviewCoversTheCurrentCode` failed, correctly, naming two
watched files: `CLAUDE.md` and `docs/notes/harness-and-inference.md`. The third file in the
change, `scripts/replay`, is new and unwatched.

## What must not move, and why that is free here

The `review-gate` skill's first instruction is to ask what quantity the change must not move
before dispatching anybody. Here the answer is *every* replayed number, and the check costs
nothing:

```
git diff --name-only 9fe14b5..c54917d -- '*.go'   ->   0 files
```

**No Go source changed at all.** The change adds a shell wrapper and edits two prose files, so
no scoring constant, no harness path and no inference script moved. `go build ./... && go vet
./... && go test ./...` was run and is green. This is the invariant the skill prefers to a
reviewer, and it is dispositive: a diff that touches no compiled code cannot move a measured
figure.

## Reviewers dispatched

**None — and this is a substitution, not a skip.** Triage puts a `CLAUDE.md` + `docs/` change in
the **fpl-findings-audit** row. That agent was not dispatched because this session operates
under an instruction not to spawn subagents unless explicitly asked. The audit was therefore
performed directly, over the same scope an agent would have been given, and its findings are
below. Recording the substitution rather than writing "not applicable" is deliberate: the next
pass should know a single reader did this, not a dedicated auditor.

The other six reviewers are skipped on triage. Nothing touches `internal/analysis`,
`internal/agent`, `internal/fpl`, config persistence, the season lists, or a live run's output.

## Findings

Ranked by how misleading the state was, all found during the change and all applied before the
commit. Two are corrections to claims this change itself had already written down, which is the
substantive content of this record.

**1. "A sweep holds about 70 MB, flat" was wrong by roughly sevenfold, and was already committed
to the working tree before it was caught.** The figure is the steady-state RSS of the sweep
*binary*, sampled on a pre-built binary — a measurement that excludes the `go test` driver
entirely. A real concurrent run is the driver plus the binary. Had it stood, it would have
justified a concurrency setting that reproduced the original failure, since it implied twenty
sweeps would fit where three actually do. **Applied:** the claim is replaced everywhere by the
full per-process table, and the note records the error in place as the reason the table exists.

**2. The mechanism first offered for the ~1 GB peak was refuted by the next measurement.** The
first explanation was that `GOMEMLIMIT` made Go's scavenger retain freed pages up to the limit.
It was written into the script as settled and then falsified: removing `GOMEMLIMIT` raised the
peak slightly (1031, 1032) rather than lowering it. **Applied:** the comment now states the
measured spread on both arms and says the knob is moot, since the process it applied to is no
longer run.

**3. The GOMEMLIMIT arms were mis-attributed in the first commit message.** Two runs were
labelled "with" and four "without"; in fact five were with (899, 962, 978, 1003, 1004) and two
without (1031, 1032). The conclusion is unchanged — the effect is smaller than the block-to-block
spread — but the arms were wrong. **Applied:** commit amended, script comment corrected.

**4. The default soft memory cap was set below the measured peak.** `MemoryHigh=1G` against a
0.9-1.0 GB peak would have throttled every ordinary sweep, making the wrapper a tax rather than
a guard rail. **Applied:** defaults raised to 2G/4G, explicitly above the peak, with the reason
stated at the assignment.

**5. A malformed `GOMEMLIMIT` killed a run before a single test executed.** `${mem_high%G}GiB`
turns `32M` into `32MGiB`, which Go rejects as a fatal startup error. Caught by the wrapper's own
error path. **Applied:** moot after finding 2 removed the assignment, but the conversion bug is
worth recording as the reason the passthrough is not reconstructed.

## What was declined

**Protecting sweeps from the OOM killer via `oom_score_adj`.** Considered and not built. Lowering
a sweep's score only redirects the kill to whatever else is on the host, which on this
infrastructure is the interactive session; trading a re-runnable batch job for someone's editor is
not obviously the right trade, and with the driver's gigabyte removed the pressure that motivated
it is largely gone.

**A regression test for the wrapper.** `scripts/replay` is shell, and the properties worth pinning
(a semaphore that queues, a cap that binds, an exit status that propagates) were each verified by
hand and are recorded here. A Go test that shells out to it would be slow and would mostly test
`flock`. Recorded as a deliberate gap rather than an oversight.

## What could not be checked here

**Why the `go test` driver holds ~1 GB for the life of a run.** The size is measured and
reproducible across five blocks; the *mechanism* is not established. Output buffering was
considered and is not sufficient — a block emits well under a megabyte. This is **unmeasured
rather than unmeasurable**: it wants a heap profile of the `go` process, which nothing here
needed once the driver was removed from the path.

**Whether the peak table generalises beyond `TestDiagTransferPolicy`.** The per-process table is
one block sampled at 1 Hz; the 0.9-1.0 GB peak is confirmed across five blocks of the same test,
but no other diagnostic was profiled. The wrapper prints peak RSS on every run precisely so this
is answered by accumulation rather than by assumption.
