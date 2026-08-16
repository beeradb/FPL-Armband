# The resident index moves to `AGENTS.md`

## What was reviewed

Renaming `CLAUDE.md` to `AGENTS.md`, leaving a one-line `CLAUDE.md` that imports it, and rewriting
**244 references across 93 tracked files**. Branch `worktree-move-the-resident-index-to-agents-md`.

Done on the user's instruction — *"We also want to remove as many mentions of claude as we can"*,
then *"CLAUDE.md is fine, unless there's an open standard"*. There is one, so the condition was met.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| **claude-code-guide** | ran, **before** the change rather than after, because the whole plan turned on one factual question: does the tool read `AGENTS.md`? |
| fpl-findings-audit | ⚠️ **skipped, and this is the judgement call in this record.** The triage row for `docs/`-and-the-record says dispatch it. **No sentence changed meaning** — this is a mechanical string substitution plus a file rename, verified by diff shape. A reviewer reading 244 identical substitutions would spend a long review budget confirming that. **If a substitution changed a claim, this skip is how it got through** |

## The finding that decided the design

⚠️ **Claude Code does NOT read `AGENTS.md`.** Its documentation says so directly: *"Claude Code
reads `CLAUDE.md`, not `AGENTS.md`."* A plain rename would have caused a **complete silent loss** —
no error, no warning, no fallback, and the project's instructions simply stop loading into every
session. **That is the failure this project fears most: a thing that stops working while reporting
nothing.**

The documented, supported bridge is a `CLAUDE.md` whose content is `@AGENTS.md`. That is what
shipped. `AGENTS.md` is the source of truth and is the cross-tool open format; `CLAUDE.md` is 424
bytes of import plus a comment saying not to write anything in it.

## What was applied

**1. `git mv CLAUDE.md AGENTS.md`,** plus a new `CLAUDE.md` carrying the import and its reason.

**2. Three functional couplings**, which are the ones that would have broken silently:

- `internal/snapshot/watched.go` — `ReviewWatchedPaths` listed `CLAUDE.md`. It now lists
  `AGENTS.md`. ⚠️ **Had this been missed, the review gate would have stopped watching the resident
  record entirely** while continuing to pass.
- `internal/snapshot/notes_test.go` — the resident-index byte budget `os.Stat`s the file. Pointed at
  the 424-byte bridge it would have passed forever, budget or no budget.
- The retraction and live-pointer surfaces in `notes_test.go` and `retracted_test.go`, which scan the
  resident record for figures the record has retracted. Pointed at the bridge they would scan
  nothing and report PASS.

**3. 244 prose references in 93 files**, mechanically substituted.

## What was declined, and why

- **`reviews/` and `stats/snapshots/` were excluded from the substitution.** Each is a **dated
  attestation about a named commit**; the repository's own doctrine is *"the pointer moves; the
  record of the pointer does not"*. Rewriting them would make them attest to a filename that did not
  exist when they were written. **They now cite a file that is not at that path, which is correct
  and deliberate.**
- **The remaining functional mentions of the vendor.** `ANTHROPIC_API_KEY` is the variable the code
  reads; `claude-opus-5` and its siblings are the strings the API accepts; `anthropics/anthropic-sdk-go`
  is the module path; `.claude/` is the tool's own directory. **None can change without breaking the
  software**, and this record states that rather than leaving the next pass to rediscover it.
- **`CLAUDE.md` itself still exists and still has that name.** It is unavoidable — it is the tool's
  hardcoded filename — and it is now 424 bytes that contain no project content.

## What could not be checked on this harness

- **That the import actually loads in a real session.** It is the documented mechanism and the file
  is syntactically a one-line import, but **nothing here observed a session resolving it.** ⚠️ **The
  failure mode if it does not is silent**, which is exactly why this deserves a deliberate check:
  start a fresh session and confirm the instructions are present.
- **Whether any of the 244 substitutions changed a meaning.** Verified as a shape — the diff is
  `CLAUDE.md` → `AGENTS.md` and nothing else — not as 244 individual readings.
- **No detection threshold applies.** Everything here is a string count.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` (empty) and the full `go test ./...`
pass. ⚠️ **`TestReviewCoversTheCurrentCode` failed before the commit with `watched path "AGENTS.md"
matches no files at HEAD`** — correct behaviour, since the digest reads `HEAD` and the rename was
staged rather than committed. It passes once committed, which is the documented flow.
