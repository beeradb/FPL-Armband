# Moving the agent definitions out of the repository

## What was reviewed

`git rm` of eight agent definitions from `.claude/agents/`, plus the pointer edits the removal
falsifies. Branch `worktree-move-the-agent-definitions`. Done on the user's instruction and tagged
by them as a v1 release blocker: these describe the project's internal review process and would ship
with any public push.

Live copies were placed in `~/.claude/agents/` **before** the removal, so nothing stopped working at
any point, and a versioned copy was mirrored to a private store.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| **fpl-code-review** | ran — **and its running is the test.** Its own definition was one of the eight removed, so a successful dispatch after the deletion was staged is the observation that the private load path works. It reported byte-identical copies for all eight, diffed against `HEAD` |
| others | ⚠️ **Formally none was owed, and that is itself a finding.** `.claude` is not in `ReviewWatchedPaths` and no triage row matches a `.claude/` change, so **the change that deletes the review apparatus owes no review by the repository's own rules.** Recorded rather than left implicit |

## Findings

**1. APPLIED — `merge-gate`'s frontmatter named a removed agent.** `SKILL.md:3` bound the gate on
"the default agent and fpl-feature". That line loads into every session's skill index, so it is a
*binding* claim rather than a pointer. Now names the role, not the file.

**2. APPLIED — `reviewgate_test.go` asserted the count and path the skill was edited to drop.**
`:18` and `:101` both said *"eight reviewer agents exist in `.claude/agents/`"* — and `:101` is the
message a **failing gate prints**, so it would have sent whoever must act on it to a directory that
no longer exists. One quantity, two implementations, one updated: the desynchronised-mirror class,
created by this change. `internal/snapshot` is watched, so this was guarded material left stale.

**3. APPLIED — the first version of the new `review-gate` paragraph broke its own rule.** It wrote
"Do not write their number or their location here" and then wrote both, one sentence later, and
reintroduced the only tracked non-`reviews/` reference to `.claude/agents/`. Rewritten to say what a
reader actually needs: the agents are per-machine, the triage table names them, `/agents` says
whether you have them, and **an unavailable reviewer is recorded as unavailable rather than skipped
silently.**

**4. NOT FIXED, and it is the largest cost of this change — the retraction guard loses reach.**
`retracted_test.go` and `notes_test.go` scan a `.claude/*.md` surface discovered via `git ls-files`.
That surface goes from 10 files to 2. `requireEverySurface` fails only on **zero**, so a surface
that lost 8 of 10 members reports PASS — ⚠️ **the per-surface floor built to catch exactly this
cannot fire.** This is the third recorded time that guard has lost scope without noticing.

⚠️ **The cost is concrete, not theoretical.** `retracted_test.go` records the motivating instance:
`fpl-stats-review.md` once shipped a **withdrawn** figure as its own worked example, on every
dispatch, including the dispatches that found it. That brief now lives where **no guard in any
checkout can reach it**, so the next retracted figure that lands in a reviewer brief ships silently
and permanently. **Not fixable inside this repository by construction** — the material left its
reach, which was the instruction. Recorded in the work note as an accepted cost with no mitigation.

**5. NOT FIXED — a tracked, enabled, fail-closed Stop hook names an agent a fresh clone will not
have.** `.claude/hooks/docs-accuracy-trigger.sh` emits `decision: "block"` telling the session to run
`fpl-docs-accuracy`. It resolves here. On a fresh clone that edits `docs/` or `README.md`, the turn
is **blocked** with an instruction naming a subagent that does not exist, and the only exits are the
second-stop brake or the model claiming it already ran it. ⚠️ **Every other dangle in this change is
prose; this one is executable.** Left for a decision because making the hook tolerate a missing agent
is a behaviour change beyond this branch's scope.

**6. APPLIED — staging was inconsistent.** The deletions were staged while the skill correction was
not, so a bare `git commit` would have shipped the falsified sentence in the same commit that
deleted the directory it named.

## What was declined, and why

- **Widening to `.claude/skills/` and `.claude/hooks/`.** They are also tracked and also describe
  internal process, so the same publication question applies — but the ask named agent definitions.
  Recorded in the work note rather than silently included.
- **Adding `.claude` to `ReviewWatchedPaths`** so a future change here would owe a review. Arguably
  right, and **declined as out of scope**: it changes what every future change is gated on, which is
  not a decision to make as a side effect of a file move.

## What could not be checked on this harness

- **Whether the two copies stay in step.** No generator and no equality check exists between
  `~/.claude/agents/` and the mirror; the drift is expected and unguarded. Byte-identity was verified
  **today only**.
- **Anything about the briefs' content once they leave.** That is finding 4, and it is unmeasurable
  from inside the repository by construction rather than by omission.
- **No detection threshold applies** — everything here is a file count or a string match.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` (empty) and `go test ./internal/snapshot/...`
all pass. `fpl-code-review` dispatched successfully **after** the deletion was staged, resolving from
`~/.claude/agents/`, and confirmed all eight copies byte-identical to `HEAD`.
