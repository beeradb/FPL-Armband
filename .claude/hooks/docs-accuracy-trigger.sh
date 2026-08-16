#!/usr/bin/env bash
# Stop hook: if user-facing documentation is unreviewed at the end of a turn, ask for
# a documentation-accuracy pass before the turn ends.
#
# Scope is docs/ AND the root README.md. The README is the only genuinely user-facing
# document in the project and the first line of the agent's own scope table, and the
# first version of this hook could never fire for it.
#
# # Why Stop rather than PostToolUse on Write|Edit
#
# A documentation change is usually several files. Reviewing each intermediate save
# reviews half-written prose and costs a subagent run per keystroke. Stop sees the
# finished state, once.
#
# # What "changed" means here, and why it is not just the working tree
#
# A Stop hook cannot see what the turn did; it can only see what the tree looks like
# now. So this fires on UNREVIEWED rather than on CHANGED, and tracks reviewed-ness
# with a per-session content hash.
#
# The change set is the union of the working tree and any in-scope commit not yet
# pushed. The commit half is not an edge case: the standing habit in this project is
# to write the finding into the docs and commit in the same turn, so a working-tree
# check alone is blind to the most careful turns — silently, with a clean exit 0.
#
# # The hash is over CONTENT, and that is the whole design
#
# The first version hashed `git status --porcelain` output, which is ` M docs/model.md`
# no matter what changed inside the file. It therefore fired once per session and was
# inert for every edit after — including the edits made in response to its own block,
# which is the one state it most needs to see. That is this codebase's signature bug
# class: it works the first time and measures nothing thereafter.
#
# Hashing content also fixes a false fire for free: `git add` with no content change
# moved the status code, and so moved the old hash.
#
# # Two independent brakes on the loop
#
# A blocked Stop re-prompts the model, and the docs are still dirty when it stops
# again. `stop_hook_active` is the harness's own signal that this stop follows a
# blocked one, and it is the primary brake. The content hash is the second: once a
# state has been asked about, it is not asked about again. Both are needed — the hash
# alone would re-fire the moment the model applied a proposal.
set -uo pipefail

payload=$(cat)

need() { command -v "$1" >/dev/null 2>&1 || { echo "docs-accuracy-trigger: $1 not found" >&2; exit 0; }; }
need jq
need git

# Anchor on the repository root, not on the caller's cwd. A pathspec is resolved
# relative to cwd, so running from a subdirectory matched nothing and exited 0 — and
# this project works in worktrees, where CLAUDE_PROJECT_DIR may name a different tree
# entirely. Both were silent no-ops.
root=$(git -C "${CLAUDE_PROJECT_DIR:-$PWD}" rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$root" 2>/dev/null || exit 0

# The harness sets this when the stop is itself a continuation of a blocked stop.
# Primary loop brake.
if [ "$(printf '%s' "$payload" | jq -r '.stop_hook_active // false' 2>/dev/null)" = "true" ]; then
  exit 0
fi

# ':/x' is root-relative regardless of cwd. Excluding a docs/TODO.md by pathspec rather
# than by grep: the old `grep -v 'TODO\.md'` matched the whole status line, so it
# dropped docs/TODO.md-notes.md outright and would drop a rename whose destination was
# a real doc.
#
# ⚠️ The exclude is INERT, and it always was — `docs/TODO.md` has never existed in this
# repository (`git log --all -- docs/TODO.md` is empty). The work queue lived at the repo
# ROOT and left on 2026-08-15, but this scope is ':/docs' and never reached it either.
# So do not read this as a pathspec that used to bind and stopped: it never bound.
# It is KEPT because the lesson below is about pathspec spelling, not about that file.
#
# ⚠️ The exclude is spelled `:(exclude,top)docs/TODO.md`, with `top` and NO leading
# slash. `:(exclude)/docs/TODO.md` is not a root-relative exclude — git reads the
# slash as an absolute path and the whole command dies with
# "fatal: Invalid path '/docs'". Written that way once, and because the error was
# routed to /dev/null and this script does not set -e, every invocation exited 0 with
# empty arrays. A hook that is installed, runs, and never fires.
scope=(':/docs' ':/README.md' ':(exclude,top)docs/TODO.md')

# A pathspec error must not read as "nothing changed", so validate the pathspec in a
# call whose exit status we can actually see. It cannot be the same call as the real
# one: command substitution strips NUL bytes, so capturing `-z` output in a variable
# silently concatenates every filename into one string. Two cheap calls, each doing
# the thing it can do correctly.
if ! probe=$(git status --porcelain -- "${scope[@]}" 2>&1 >/dev/null); then
  printf 'docs-accuracy-trigger: git status failed, not firing: %s\n' "$probe" >&2
  exit 0
fi
mapfile -d '' -t worktree < <(git status --porcelain -uall -z -- "${scope[@]}" |
  awk 'BEGIN{RS="\0"; ORS="\0"} NF {print substr($0,4)}')

# Committed but not yet pushed. No upstream (fresh branch, detached HEAD) contributes
# nothing rather than failing.
mapfile -d '' -t committed < <(git diff --name-only -z '@{upstream}...HEAD' -- "${scope[@]}" 2>/dev/null)

changed=("${worktree[@]}" "${committed[@]}")
[ ${#changed[@]} -eq 0 ] && exit 0

files=$(printf '%s\n' "${changed[@]}" | grep -v '^$' | sort -u | paste -sd' ' -)
[ -z "$files" ] && exit 0

# Content of every in-scope file as it stands on disk, tracked and untracked.
state=$( {
  git ls-files -z -- "${scope[@]}" 2>/dev/null | xargs -0 -r git hash-object 2>/dev/null
  git ls-files -o --exclude-standard -z -- "${scope[@]}" 2>/dev/null | xargs -0 -r git hash-object 2>/dev/null
  printf '%s\n' "${committed[@]}"
} | git hash-object --stdin 2>/dev/null) || exit 0

session=$(printf '%s' "$payload" | jq -r '.session_id // empty' 2>/dev/null)
[ -z "$session" ] && session="nosession"
dir="$(git rev-parse --git-dir)/fpl-docs-accuracy"
mkdir -p "$dir" 2>/dev/null || exit 0
find "$dir" -type f -mtime +7 -delete 2>/dev/null
mark="$dir/$session"

if [ -f "$mark" ] && [ "$(cat "$mark" 2>/dev/null)" = "$state" ]; then
  exit 0
fi

# Build the message BEFORE recording that we asked. The first version wrote the mark
# first, so a jq failure or the hook timeout between the two left the state marked
# "already asked" with nothing ever emitted — unrecoverable inside that session.
out=$(jq -cn --arg files "$files" '{
  decision: "block",
  reason: (
    "This turn left user-facing documentation unreviewed: " + $files + "\n\n" +
    "Run the fpl-docs-accuracy subagent over those files before finishing. It checks " +
    "that what a document claims the system does is what the code does, that no " +
    "scoring term, config field or package is undocumented, and whether a diagram " +
    "would carry something the prose cannot.\n\n" +
    "It proposes; it does not rewrite. Apply what survives your own judgement, say " +
    "what you rejected and why, and do not silently drop a finding. Note that docs/ " +
    "is in reviewWatchedPaths, so an applied edit owes a review record.\n\n" +
    "If you have already run it over these exact files this turn, say so and finish."
  )
}') || exit 0

printf '%s' "$state" >"$mark" 2>/dev/null
printf '%s\n' "$out"
