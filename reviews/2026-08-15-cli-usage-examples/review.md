# Review — usage examples for the flag-ordering rule, and the README's stale flag list

Branch `cli-usage-examples`, off `origin/main` at `1787246`. Range: `1787246..HEAD`.

## What was reviewed

A CLI-surface and documentation change. `rejectFlagsAfterCommand` has always rejected
`fplagent squad -html out.html`, and its comment argued the ordering rule was "worth a guard
rather than a line in the usage text". The operator decided it should be both. So: worked
examples in the `usage` const, the guard kept, its comment corrected, and `README.md`'s
hand-written flag list — five names when the CLI has eight — deleted rather than completed.

**No review was strictly owed.** `ReviewWatchedPaths` is `internal/analysis`, `internal/backtest`,
`internal/agent`, `internal/fpl`, `internal/config`, `internal/snapshot`, `stats`, `docs`,
`config.json`, `CLAUDE.md`. This change touches `README.md` and `cmd/fplagent/`, so
`TestReviewCoversTheCurrentCode` does not fire. The record is written anyway, because two
reviewers found a defect that would have shipped and because the declined findings below are
worth not re-raising.

⚠️ **A `TODO.md` correction was made and then backed out.** `098d74e` added a title line for D2 and
corrected the false sentence "Only C1 is open, under *Velocity* above" — D2 has been open since the
review that raised it, and `FindPrevious` still parses a commit out of a snapshot directory name
and falls back to lexical max. The correction was right and is now **moot**: the operator has
retired the repo's work queue, and `retire-todo-md` deletes the file. Editing a file another branch
deletes buys only a modify/delete conflict, so this branch no longer touches it. **The content is
not lost** — D2's body, and the fact that the repo copy asserted otherwise, are recorded in the
queue that replaces it.

## Reviewers

| agent | ran | why |
|---|---|---|
| **fpl-code-review** | yes | new test code, and the change is about a silent-no-op guard |
| **fpl-docs-accuracy** | yes | the root README and the binary's own help text |
| fpl-stats-review | no | no scoring constant, no measurement, no replay |
| fpl-findings-audit | no | no verdict or figure in `CLAUDE.md` is touched |
| fpl-security-review | no | no credential path, no config persistence, no agent-loop input |
| fpl-run-review | no | no live run wrote config |
| fpl-season-maintenance | no | the four hand-maintained lists are untouched |

## Findings, worst first

**1. The worked example I added was itself the bug the change exists to prevent.** Both reviewers
found it independently. `fplagent backfill 2023-24 -coverage` does not enable `-coverage`:
`cmdBackfill` builds its own `FlagSet` and calls `fs.Parse(args)`, which stops at the first
non-flag argument exactly as `flag.Parse` does. So `-coverage` was read as a *second season name*,
and the documented invocation would have entered the several-hundred-request Internet Archive
crawl its own caption promised to avoid, then printed `FAILED` for the phantom season and exited
0. `docs/backfill.md:240` already had the correct form. **Applied**: the example is now
`fplagent backfill -coverage all`, and the Examples block states the sub-rule explicitly.

**2. The first version of the guard could not catch finding 1, and I had already watched the
earlier version fail to catch something else.** `rejectFlagsAfterCommand` returns `nil` on sight
of a command in `commandsThatParseTheirOwnFlags`, so the test asserted *nothing* about the five
commands whose ordering is subtle enough to need an example. **Applied**: for those commands the
test now asserts no flag appears after a positional argument, `ask` excepted. Mutation-tested —
restoring the broken example produces a failure naming the flag, the positional and the mechanism.

**3. Both guards were hand-models of things Go already owns.** The flag names came from a
`go/ast` scan blind to the entire `XxxVar` family — converting one flag to `flag.IntVar` would
have removed it from the test's view silently — and the argument splitter carried its own
hardcoded list of which flags are boolean, which `CLAUDE.md` names directly: *"a diagnostic must
never carry its own copy of the thing it is checking"*. **Applied**: `registerGlobalFlags(fs
*flag.FlagSet)` extracted from `run()`, which passes `flag.CommandLine` so behaviour is unchanged.
The test builds the same set on a throwaway `FlagSet` and uses `VisitAll` for names and `Parse`
for splitting. Both hand-models deleted.

**4. The check was one-directional.** Registered → documented only, so a documented-but-deleted
flag passed. Having removed the README's list, the usage text is the only place left for that lie
to hide. **Applied**: set equality both ways.

**5. `ask` does not parse its own flags.** My prose said five commands do. `ask` joins
`flag.Args()[1:]` into free text and parses nothing; it is in the exemption map because a question
may legitimately begin with a dash. A reader following the sentence would have had `-refresh`
silently absorbed into their question. **Applied** in the usage text, the README and the
`globalFlags` doc comment.

**6. `-html` is not squad-only.** It is passed to `cmdBrief`, `cmdSquad` and `cmdTransfers`, and
`cmdBrief` writes a briefing page. The flag's own description said "the squad". **Applied**: the
description names all three, and the README's command table carries `-plain`/`-html` on those rows.

**7. "prints every flag and a worked example of each" was false when written** — three of eight
flags have no example. The same shape as the defect being fixed. **Applied**: weakened to "worked
examples of the ones whose ordering is easy to get wrong".

**8. My replacement introduced a fresh unguarded copy.** The new README sentence enumerated the
self-parsing commands, which is a second implementation of `commandsThatParseTheirOwnFlags` with
no equality check — the same mechanism that made the flag list stale. **Applied**: the enumeration
is gone; the README points at the usage text, which is one sentence away.

**9. The correction comment led with the retracted text.** House convention marks the standing
statement first and the ⚠️ correction after, with the commit that caused it. **Applied**, naming
`dbeee98`, `7d346f9` and `4546732`.

## Declined, and why

- **`README.md:224`, "Four seasons is three degrees of freedom."** True when written, stale since
  the grid widened to six; `CLAUDE.md` puts season-clustered df at 5. Declined **here** because it
  is a measurement claim and this branch has no statistics review; the reviewer that raised it said
  the same. It is a real defect and wants `fpl-stats-review` on the wording, not my guess. Filed
  rather than fixed.
- **`internal/present` is absent from `docs/architecture.md`'s package table.** Pre-existing, and
  it is why `-html` had nowhere in `docs/` to be documented. Out of scope; touching `docs/` would
  also pull this change into `ReviewWatchedPaths`.
- **A mermaid diagram of `cmd/fplagent` → `internal/present` → {pitch, list, page}.** Declined: it
  belongs with the `docs/architecture.md` fix above, not with a usage-text change.
- **Re-framing `fplagent brief` in `docs/workflow.md`.** Declined for the same reason; the
  reviewer recommended the README-only version.
- **An equality check tying the README's command list to the exemption map.** Moot for the README —
  the list is deleted rather than guarded, which is the stronger fix. ⚠️ **Not moot for the usage
  text, and declining it here was wrong**: the same enumeration survives there. See finding 10 in
  the second round below, where it is implemented.

## What could not be checked here

- **Whether a reader who will not build the binary is now under-served.** The argument for deleting
  the README's flag list is that an unguarded copy drifts, demonstrably, twice. The argument
  against is that `README.md:40-59` duplicates the whole command table anyway, so the document's
  practice is *duplicate and accept the risk*, and this change makes it inconsistent. That is a
  judgement about the audience, not a fact about the code, and it is recorded here unresolved. The
  compromise taken — flags named on the three command rows where they change the output — keeps
  the "usage text is the only complete list" claim true without an enumeration that can go stale as
  a set.
- **Nothing was measured and no figure moved.** No scoring or harness path is touched; `HOLD` and
  `POLICY` are not in question. A GW1 squad run before and after the base change was byte-identical,
  but that was a check on the *base*, not on this diff.

## Second round — a private-store audit re-read the change after it was committed

**10. The fix traded one unguarded enumeration for another, and nothing tied it to the map.**
Deleting `README.md`'s flag list was right, but the replacement text — in the usage string — names
`capture, backfill, snapshot and reviewkey`, which is a hand-maintained copy of
`commandsThatParseTheirOwnFlags` with no equality check. That is this item's own defect class one
noun over, against the map whose hand-listed copy has already drifted once: the `reviewkey`
omission that made every documented invocation of a new command error while `go build`, `go vet`
and `go test` all stayed clean. **Applied**: `TestTheUsageTextNamesExactlyTheSelfParsingCommands`
parses the sentence and asserts set equality with the map, `ask` excluded and asserted separately
because it registers no `FlagSet`. Mutation-tested by dropping `reviewkey` from the sentence —
the exact historical drift — and it fails naming both sets.

The same audit reported `README.md` still enumerating five commands including `ask`, and still
claiming "a worked example of each". **Both were already fixed** in the first round; the audit read
a working tree that had moved under it, and says so. No action.

## Redaction note — 2026-08-16

One section heading above was edited after this record was filed. It named a private store
this repository may not name; it now reads **a private-store audit**. The section and its
findings are unchanged — that audit ran, and found what it is recorded as finding.

⚠️ **Cleaned rather than exempted.** The standing exemption for already-committed
disclosures is a grandfather clause over an enumerated set; this was found afterwards. The
cost — amending a dated attestation — is acknowledged, which is why this note exists rather
than the edit being silent. **No finding was altered.**
