# Merging main, and the index that two branches both grew

Covers the merge `84e49b2..7787f04` — eighteen commits of main's scope-priority follow-up
work joined to this branch's chip-set and page work.

## ⚠️ What this record does NOT cover

**Main's eighteen commits were reviewed on their own branch and not by me.** They carry
their own records — `2026-08-13-d4f028b` is the newest of them, and `1e06ba9` exists
specifically to name it for the tip it covers. The changes they bring to
`internal/analysis/squad.go`, `internal/analysis/sweep.go`, `docs/model.md`,
`docs/configuration.md` and three notes are theirs.

This record exists so the merged branch has one covering **the merge**, not to imply I
audited a refuted headline, a confinement violation, or the bench-shape re-measurement.
Whoever owns that work owns its review.

## The merge itself

**Conflict-free, and checked with `git merge-tree` before merging rather than after.** Both
sides moved since `6031ef2`: main took the scope-priority follow-ups, this branch took the
second chip set, the page rebuild and the hindsight retraction.

**The real risk was the record, not the code**, and it is the one the budget guard's own
comment names: two branches editing one index is how a claim gets paid for twice. Audited —
**130 bullets, zero repeated openings**, and no duplicated sentence over 90 characters. The
growth on both sides is genuine content.

**Full suite green on the merged tree**, `internal/backtest` and `internal/analysis`
included. Build and vet clean.

## What I changed in the merged file, and why

`CLAUDE.md` came out of the merge **95 bytes over budget**, both sides having added
legitimately. I did not raise the budget — the guard's own comment calls that a ratchet, and
it was already raised once this session. Three edits instead:

| edit | reason |
|---|---|
| The `fullSight` retraction moved from the two-set bullet onto the **anchoring** bullet | It corrects that claim, not the two-set one. Removes the duplicated context as a side effect |
| `OracleLineups` bullet said "nothing reads `StartShare`" **twice** | A literal self-repeat inside one bullet. This is main's content and I touched it only to delete the repetition |
| The starting-elevens bullet was reflowed and lost "First instance of the wider rule" | Meta-commentary about the rule rather than the rule |

My own additions were cut to bare verdicts, with the evidence left in
`docs/notes/chips.md` — which is what the guard asks for.

## What could not be checked

**Whether main's eighteen commits are correct.** The suite passes against them and this
branch's tests pass beside them; that is a compatibility check, not a correctness one.

**Whether the two branches' findings interact.** Main re-measured bench shape and the metric
reach; this branch changed chip expressibility, which is off by default. No mechanism
connects them that I can see, and I did not sweep to confirm it.
