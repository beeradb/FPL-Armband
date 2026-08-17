# Retiring the `ask` and `chat` subcommands

Branch `retire-ask-and-chat`, based on `b213ca1` (tip of `origin/main`). One commit, this
record riding with it.

## What was reviewed

The deletion of two CLI subcommands and everything that existed only to serve them.

All four AI commands were one function, `cmdAgent`, differing in two arguments — the prompt and
an `interactive bool`. The 2×2 those axes describe had `review` and `advise` on canned prompts,
`ask` on free text, and `chat` on an empty prompt with the interactive flag set. `ask` was
therefore the only non-interactive free-text path, and `chat` the only interactive one.

Removed: both `case` arms; `cmdAgent`'s trailing `chat` parameter; the interactive stdin loop;
the multi-turn transcript accumulation with its `---` separator and `## Q:` headers; the
session-total usage line; `truncateTitle`; the `bufio` import; and `"ask"` from
`commandsThatParseTheirOwnFlags`, where it had been the only entry registering no `FlagSet` at
all. Documentation references removed from `README.md` (command table and the mermaid agent
node), `docs/configuration.md` (credential list) and the usage string.

**Why it was worth doing beyond the deletion itself.** `ask`'s presence in
`commandsThatParseTheirOwnFlags` was a standing exception in the guard that stops this project's
most-repeated failure shape — `armband squad -html out.html` printing a squad and silently
writing no file. That exception was load-bearing in three assertions in `usageflags_test.go` and
required a sentence of usage text that a test asserted was present. Retiring `ask` collapses
`TestTheUsageTextNamesExactlyTheSelfParsingCommands` to plain set equality: every exemption must
now be a command that really does parse its own flags.

## What must not move, and did not

Nothing on the scoring path changed, so every deterministic command must be byte-identical. This
was checked directly rather than argued from the diff, per the gate's own preference for
invariants over reviewers.

`squad`, `fixtures`, `congestion` and `transfers` are byte-identical between a binary built at
`b213ca1` and one built from this branch, against the same `config.json` and cache.

Three commands reported a difference and **none of them is this change**:

| command | difference | cause |
|---|---|---|
| `brief` | one line: `-fpl-briefing.md` vs `-fpl-briefing-2.md` | the harness ran it twice and the writer suffixes to avoid clobbering. The other 398 lines are identical |
| `nations` | rows with equal counts reordered | **pre-existing non-determinism**, reproduced on the unmodified `b213ca1` binary across three runs |
| `chips` | Bench Boost and Triple Captain swap | **pre-existing non-determinism**, reproduced on the unmodified binary across four runs |

Behaviour checked by hand on the built binary: `armband ask "..."` and `armband chat` both now
error with `unknown command`; the usage text contains zero occurrences of either word; `review`
with credentials cleared reaches the credential check, which proves it dispatches past the new
empty-prompt precondition rather than tripping on it.

`go build ./...`, `go vet ./...` and `go test ./...` are clean — 15 packages.

## Reviewers

| reviewer | ran | triage |
|---|---|---|
| **fpl-code-review** | yes | the diff is the agent command layer — `cmdAgent`, the dispatch switch, the flag guard |
| **fpl-docs-review** | yes | `README.md`, `docs/configuration.md` and the usage string all changed, and the user asked explicitly that every documentation reference go |
| fpl-stats-review | **skipped** | no constant, no estimator, no cell, no measurement. Nothing statistical is asserted anywhere in the diff |
| fpl-findings-audit | **skipped** | `AGENTS.md` is untouched and no recorded verdict is relied on or contradicted. A capability was removed; no claim about football or the model changed |
| fpl-security-review | **skipped** | `internal/fpl` and config persistence are untouched. The only surface change is the **removal** of a stdin input path that fed arbitrary user text into the agent loop, which cannot add exposure. `internal/agent` is touched, but only to delete a write-only counter |
| fpl-run-review | **skipped** | not the output of a live run |

## Findings applied

All were verified against the code before being applied; none was taken on the reviewer's word.

**1. `TotalUsage` was left write-only by this change.** `internal/agent/agent.go` accumulated it
in seven statements and the session-total line deleted here was its only reader in the tree.
Verified by grep: seven writes, zero reads, test code included. Removed the field and all seven
writes. This is residue the deletion created, so it belongs to it.

**2. The `Agent` doc comment claimed a capability that no longer exists** — "keeping history so
the user can ask follow-up questions". Nothing asks a follow-up now. Reworded to say why history
is still kept: the tool loop inside a single `Ask` is itself a conversation.

**3. `if writeReport && answer != ""` had an unreachable-false clause.** `Ask` returns an error
rather than an empty string when the model produces no text
(`internal/agent/agent.go`, `if text == "" { return "", ... }`), and `cmdAgent` returns on that
error, so `answer` is never empty at that point. Verified by reading both. Simplified to
`if writeReport {` with a comment naming the reason.

**4. `docs/configuration.md` documented a command that does not exist.** The sentence edited here
listed `auth` among the commands running without a credential; `auth` was removed outright some
time ago — `README.md` says so explicitly — and the same list omitted `backfill` and `reviewkey`.
Verified: no `case "auth"` in the tree, and the built binary answers `unknown command "auth"`.
Because the sentence makes a **closed-set** claim and this change edited it, the enumeration was
replaced with a description rather than completed. Nothing checks such a list, which is precisely
how `auth` survived its own removal.

**5. `docs/architecture.md`'s sequence diagram said "write transcript".** The transcript was the
`strings.Builder` deleted here; `cmdAgent` now writes a single answer. Changed to "write report".

**6. `cmd/armband/transfers_test.go`'s drift comment names `ask`.** The comment is past-tense and
accurately records a real bug, so it was kept — but it is now the only `ask` in `cmd/armband/`,
eight lines above a four-entry map, and a reader checking the retirement was complete has to read
a paragraph to clear it. Added four words marking the command as retired, without touching the
account.

**7. A new test replaces a runtime-only precondition.** `cmdAgent` now rejects an empty prompt.
The reviewer established that this guard is unreachable today — all three callers pass a
`Sprintf` of a non-empty literal — and, more usefully, that it fails in the worst place: `due`
calls `writeDueState` **before** `cmdAgent`, and `checkDue` then refuses to re-run for that
gameweek, so a builder returning `""` would consume a scheduled review permanently with no report
and no retry. Verified by reading `due.go`. `TestEveryAgentPromptIsNonEmpty` now asserts both
builders return a real instruction against an empty bootstrap, moving the failure from the cron
path to the build. The runtime guard was kept as well: it is four lines, it can never fire for a
current caller, and a precondition that throws is this project's stated preference over a silent
no-op.

## Findings declined

**`nations` and `chips` are not run-to-run deterministic. Not fixed here.** Both are the recorded
`teamBands` defect one file over — a map-ranged slice with a comparator that does not totally
order:

- `cmd/armband/main.go`, `cmdNations`: `byRegion` is a map, ranged into a slice, then sorted with
  non-stable `sort.Slice` on `count` alone, so equal-count rows keep arbitrary map order.
- `internal/analysis/chips.go`, `ChipWindows`: `earliest` is a map, ranged into a slice, then
  sorted with `sort.SliceStable` on `(Stop, Start)`. Bench Boost and Triple Captain are tied on
  **both** keys, and stability preserves a non-deterministic input order — so reaching for the
  stable sort did not help.

Both reproduce on the unmodified `b213ca1` binary, so neither is attributable to this change, and
folding an unrelated correctness fix into a deletion would muddy both the diff and the
differential above. `AGENTS.md` already records `teamBands` as unfixed with the note that
`Optimize` had the identical defect and is pinned by `TestSeedOrderIsDeterministic`; these two
are further instances and the fix in each case is a total order on a stable key. **Owed as
follow-up work, with a test, not a silent patch.**

**`due` calls itself "the review" in four places while running `advicePrompt`.** The usage line,
the doc comment, the stdout line and the report title all say review. `README.md`'s row is the
only correct one and now says so pointedly. Pre-existing, and renaming a user-visible report
title is a separate decision. Declined as scope.

**`Reset()` and the trailing `a.history` assignment in `internal/agent/agent.go`.** Both are
multi-turn machinery. `Reset()` had no callers at `b213ca1` either, so it is pre-existing dead
code rather than residue of this change; `history` remains genuinely live *within* a single
`Ask`, appended before the request and read into `Params.Messages`. Removing the persistence is a
redesign of the agent package, not cleanup of a deletion. Declined, deliberately drawing the line
at what this change fully orphaned.

**Documenting the free path for correcting a competition window.** I had believed, and said, that
retiring these commands made a full billed run the only way to correct a competition window. That
is **wrong**, and the docs reviewer caught it: `european_campaigns` and `domestic_cups` are
hand-editable config keys. What is true is a pairing — a hand edit changes the windows but does
not stamp `LastVerified`, and `verify-competitions` stamps without changing windows, so the free
correction is two steps. Worth documenting beside the `european_campaigns` row, but it is a new
documentation claim rather than a reference to a retired command, so it is declined here as scope
and recorded as owed.

**A guard binding `README.md`'s command table and the credential list to the dispatch switch.**
Both reviewers arrived at this independently, from opposite directions: the usage-block↔switch
mirror has no test on the *deletion* side, and the `auth` bug is exactly what such a guard would
have caught. `TestTheUsageTextNamesExactlyTheSelfParsingCommands` already does this one noun over.
Declined as scope — building a new guard inside a deletion is how a two-file change becomes a
six-file one — but it is the most valuable thing this review surfaced that is not being done.

**A note in `review-gate/SKILL.md` that a filed review record is a dated artefact.** The docs
reviewer had to derive the rule that `reviews/` is not kept current, and observes it is at least
the second time. Correct and worth writing down; not this change's job.

**`reviews/2026-08-15-cli-usage-examples/review.md` discusses `ask` in four places.** Deliberately
untouched. Those are dated records of a past review, `reviews/` is not on the watched-path list,
and the file already uses the binary name `fplagent`, which no longer exists anywhere — so a
currency edit for `ask` would owe one for the binary name across roughly forty records. Editing
them would falsify the record for no reader's benefit.

## What could not be checked on this harness

- **No live agent run.** `review`, `advise` and `due` were exercised only as far as the credential
  gate; no API call was made, so the report-writing path is read rather than observed. The change
  to that path is the removal of an always-true condition, which is why reading it was judged
  enough.
- **Mermaid rendering.** `TestMermaidBlocksAreWellFormed` is structural only and passes; no
  renderer exists on this machine, so the edited `README.md` node and `architecture.md` diagram
  are asserted well-formed, not seen.
- **How often the retired commands were actually used.** Nothing in the repository records
  invocation counts, so the cost of removing the non-interactive free-text path is a judgement
  the user made, not a measured one. Worth stating plainly: this removes a capability, and no
  measurement says whether it mattered.
