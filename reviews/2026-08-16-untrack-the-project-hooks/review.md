# Untracking the project hooks and settings

## What was reviewed

Removing `.claude/settings.json` and both `.claude/hooks/*.sh` from the tracked tree, keeping them on
disk as `.claude/settings.local.json` and `.claude/hooks/`, both gitignored. `.claude/skills/` stays
tracked.

## Which reviewers ran

| reviewer | why |
|---|---|
| none | no code changed; the judgement is which of two categories each file belongs to, argued below. Verified by the full suite and by `git check-ignore` |

## ⚠️ The defect this fixes, which is not a preference

`docs-accuracy-trigger.sh` is a **fail-closed Stop hook**. It emits `decision: "block"` with *"Run
the `fpl-docs-accuracy` subagent over those files before finishing."* **That agent left the tracked
tree earlier the same day**, when the agent definitions were moved to `~/.claude/agents/`.

**So a stranger who cloned the published repository, edited `docs/`, and finished a turn would have
been blocked with an instruction naming a subagent they do not have** — with no exit but the
second-stop brake, or asserting they had already run it, which is the model lying to clear a block.

⚠️ **This was recorded as a known consequence when the agents moved and left unfixed.** Every other
dangling reference that move created was prose. **This one executed**, and publication is what made
it reach strangers.

**Second, `settings.json` imposed unrelated configuration**: it enabled a `frontend-design` plugin
and wired a hook that renames the terminal tab. Neither is a cloner's business.

## The split, and why the skills stay

| | disposition |
|---|---|
| `hooks/*.sh`, `settings.json` | **untracked** — they EXECUTE on someone else's machine, and one fails closed |
| `skills/merge-gate`, `skills/review-gate` | **kept** — they execute nothing and are invoked deliberately |

**The skills are among the most useful things here for a reader**: they state how the project decides
something is ready to merge. Both were already made honest about the agents not being in the tree —
`review-gate` says they are per-machine, points at `/agents`, and requires an unavailable reviewer to
be **recorded as unavailable** rather than silently skipped.

## Why `settings.local.json` and not `~/.claude/`

The agent definitions went to `~/.claude/agents/` because that is a documented load path and they are
wanted everywhere. **These are project-scoped**: the hooks resolve through `$CLAUDE_PROJECT_DIR`, and
putting them in the user directory would fire this project's docs hook **in every unrelated
project**.

`.claude/settings.local.json` was already gitignored at `.gitignore:3`, so it needed no new rule;
`.claude/hooks/` did and got one. Both verified with `git check-ignore -v`.

## What was declined

- **Deleting the hooks outright.** They work and they are wanted here; the problem is publication, not
  the automation.
- **Repairing the fail-closed hook to tolerate a missing agent.** That is a behaviour change to
  working automation, and untracking removes the exposure without touching what runs. ⚠️ **The hook is
  still fail-closed on this machine and still names an agent that resolves only from `~/.claude/`.**

## What could not be checked

- **That no cloner already hit the blocked-turn case.** The repository has been public for under an
  hour, but this is unknowable from here.
- **No detection threshold applies.**

## Verification

`go build ./...`, `go vet ./...`, full `go test ./...` pass. `git ls-files .claude` returns the two
skills only. All three files remain on disk and both paths are confirmed ignored.
