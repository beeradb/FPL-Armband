#!/usr/bin/env bash
# SessionStart + Stop hook: name the terminal tab after the branch this session is on.
#
# Several fpl sessions run at once, and from outside they are indistinguishable — the
# tab bar shows the same thing for all of them. This puts the branch in the title, so
# a glance at the tab bar says which piece of work each window is doing.
#
# # Why the branch, and not a ticket id
#
# Nothing tells a session which queue item it is on, so a ticket id would have to be
# set by hand and would go stale the moment the session moved on — this project has
# paid repeatedly for hand-maintained state with no generator. A branch name has a
# generator (git), updates itself when the work moves, and is ALREADY required to be
# descriptive: the naming rule forbids ordinals and cryptic contractions, and the
# merge gate checks branch names as one of its three leak channels. So the branch is
# the closest thing to a ticket name that maintains itself.
#
# # Why this cannot rename the session in Claude Code's own listings
#
# It cannot, and no hook can — session metadata is not writable from a hook, and
# `/rename` is a user command. This hook moves the TERMINAL EMULATOR's tab title
# only. If the listing is what you needed, this is not it.
#
# # Why SessionStart AND Stop
#
# SessionStart sets the title once, at launch. Stop re-runs it after every turn, so a
# session that switches branch mid-session — which happens constantly here, since the
# merge gate has each integration on its own branch — retitles itself without anyone
# remembering to. There is no "branch changed" event to hook, and PostToolUse on Bash
# would fire on every command to catch the rare checkout.
#
# # Degradation is deliberate at every step
#
# A hook that breaks a turn to fix a cosmetic tab title is a bad trade. So:
#   - no git, detached HEAD, or a git that errors  -> fall back to the directory name
#   - `jq` absent                                  -> hand-rolled JSON, no dependency
#   - `terminalSequence` unsupported by the host   -> the JSON is ignored, nothing breaks
# and the hook exits 0 unconditionally. It has no opinion about anything and must
# never be the reason a turn fails.

set -uo pipefail

cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || exit 0

# A worktree reports its own branch, which is what we want — each integration branch
# gets its own tab. `--show-current` is empty on a detached HEAD rather than erroring.
label="$(git branch --show-current 2>/dev/null)"

if [ -z "$label" ]; then
	# Detached HEAD still has a short SHA worth showing; if even that fails we are
	# probably not in a repository at all, so use the directory.
	label="$(git rev-parse --short HEAD 2>/dev/null)"
fi
if [ -z "$label" ]; then
	label="$(basename "$PWD")"
fi

# Tab bars truncate, and they truncate from the RIGHT — which would eat the
# distinguishing tail of a long branch name and leave every tab reading
# "integrate-the-...". So keep the tail and drop the head, marked so it is visibly
# elided rather than looking like a real name.
#
# ASCII only, deliberately. A title goes out as a raw byte sequence to whatever
# terminal is attached, and the set of emulators that mishandle a multi-byte
# character there is not one worth discovering from a garbled tab. There is no
# typographic gain to weigh against it.
max=32
if [ "${#label}" -gt "$max" ]; then
	label="...${label: -$((max - 3))}"
fi

title="fpl: ${label}"

# OSC 0 sets icon name and window title together; BEL terminates it. Escaping is
# printf's rather than a literal control character, so this file stays greppable and
# diffable as plain text.
seq="$(printf '\033]0;%s\007' "$title")"

# Hand-rolled JSON so the hook has no dependency on jq. Only two characters can
# appear in a branch name that need escaping here — a backslash and a double quote —
# and git forbids both in a ref name, so the substitution below is belt-and-braces
# rather than load-bearing.
esc="${seq//\\/\\\\}"
esc="${esc//\"/\\\"}"
esc="${esc//$'\033'/\\u001b}"
esc="${esc//$'\007'/\\u0007}"

printf '{"terminalSequence":"%s"}\n' "$esc"
exit 0
