# Compacting the skill definitions, and the snapshot that made the suite green

Branch `fix-the-exact-float-comparison`, second commit, based on `c1f211b`.

## What was reviewed

Two things that arrived together because the second was unlocked by reviewing the first.

**The skills.** `.claude/skills/merge-gate/SKILL.md` and `review-gate/SKILL.md`, compacted on the
instruction "we won't need reasons, just rules — it exposes way too much internal info". 244 lines
to 157. Removed: incident narratives and their figures, justification paragraphs, machine
specifics, and all three references to the private store by name.

**The snapshot.** `stats/snapshots/2026-08-17-c1f211b`, regenerated because the review of the
skills established that regenerating is the correct action today — the opposite of what the
compaction had just asserted.

## The correction that mattered

My compacted condition 4 said: *"Do not regenerate and commit a snapshot directory to satisfy this
locally"*, on the reasoning that `snapshot.yml` had taken the job over.

**That was wrong, and it made the gate unsatisfiable by construction.** Verified: `snapshot.yml`
publishes with `gh release create` and never commits, while **77 keyed snapshots are still
committed** — so `NewestKey` finds one, `FPL_SNAPSHOTS_EXTERNAL=1` does not skip, the committed
series never advances, and the only remaining route to condition 4 was the one I had forbidden.
The test's own failure message says "Regenerate it". The base text had handled this correctly with
a conditional — *once* the series leaves the repository — and my compaction converted a future
state into a present-tense instruction.

Corrected to describe today's state. Then acted on: the diagnostics were run and the snapshot
banked, which is why the **whole suite passes for the first time in this session**.

Recipe traps observed rather than trusted: `-count=1` was passed, the model CSV was written to a
path nobody else uses and given to both halves, and it was checked non-empty (555 rows) and
holding exactly one run before rendering. `figures.csv` carries `model.present,true` — the silent
failure this recipe is annotated for produces `false`.

## Reviewers

| reviewer | ran | triage |
|---|---|---|
| **fpl-docs-review** | yes | the change is written record; asked specifically whether the compaction dropped a **rule** rather than a **reason** |
| fpl-code-review | **skipped** | no Go changed. The snapshot is generated output, and its generator is unmodified |
| fpl-stats-review | **skipped** | the snapshot banks figures, it does not assert a claim about them. No constant, estimator or arm |
| fpl-findings-audit | **skipped** | `AGENTS.md` and `docs/` untouched in this commit |
| fpl-security-review, fpl-run-review, fpl-season-maintenance | **skipped** | no production path, no live run, no hand-maintained list |

## Findings applied

**1. Condition 4 was inverted.** Above. The most consequential thing in the review.

**2. `/agents` was cut, and it was the only means behind a rule that survived.** The compaction
kept "record a reviewer as unavailable" while dropping how to determine installation. Restored —
and it also un-falsifies a filed record that cites the pointer as part of the argument for keeping
these files tracked.

**3. The re-key rule moved to the file that lacks the trigger.** `TestReviewCoversTheCurrentCode`'s
failure message says "Invoke the `review-gate` skill", so the session standing in front of a fired
gate reads `review-gate` — which no longer said "do not re-key". Restored there; the `merge-gate`
copy stays.

**4. The branch-name channel named a closed half and missed the live one.** "Reaches generated
headers" is false at HEAD — `render.go` deliberately carries no branch field and a test holds it
absent. What is live is **merge commit subjects**, where the name lands with nobody typing it.
Both versions had this wrong; now it names the referent that exists.

**5. Two clauses restored**: "concurrently **where they do not conflict**" (the conditional was the
rule and I had kept only its reason), and "a branch can pass every line and still leave work owed
— record it rather than blocking the merge".

**6. Condition 12's referent was unresolvable.** "The paired branch" with an empty check column
told a reader nothing about what pairs with what. Reworded without naming anything.

**7. Disclosure nit taken**: the grep placeholder went from "private store name" back to "the
withheld name" — a shade less disclosing, for no loss.

## Findings declined

**Genericising the reviewer agent names in the triage table.** They are `fpl-` prefixed
descriptions of function, already tracked at base, and a recorded decision keeps these two files
tracked. Genericising would destroy the table's only purpose, which is naming which agent to
dispatch.

**Adding an `internal/snapshot` row to the triage table.** It is in `ReviewWatchedPaths` and
appears in no triage row, and `watched.go` argues it is the highest-stakes tree in the list. Real
gap, pre-existing, and adding a row is a change to the review policy rather than a compaction.
Owed.

**Row 3's wording.** "A review record **exists** and covers current content" overstates:
`TestReviewCoversTheCurrentCode` *skips* when no keyed record exists, so it does not establish
existence. Pre-existing, untouched by the compaction, left alone rather than fixed in passing.

**The `--allow-unrelated-histories` prohibition is new, and is kept deliberately.** The reviewer is
right that it appears nowhere else in the repo and was invented during a compaction, which is not
what "keep the rules" authorises. It is kept because it was nearly stepped on earlier today: the
retired repository's `development` shares no merge base with this line, and merging it would have
collided across ~198 files. A rule that would have prevented a real mistake this session is worth
its line, but it is flagged here as an addition rather than a restoration.

**The claimed disclosure win is smaller than it looks.** Every figure and narrative removed from
these two files is still published elsewhere in the repository — `staleness_test.go`,
`snapshot.yml` and `AGENTS.md` between them carry the timings, the counts and the incident
histories. What the compaction actually achieves is less repetition and two fewer mentions of the
private store, not the removal of information from the public repo. Recorded so the next reader
does not over-read it.

## What could not be checked

- **The `pre-receive` hook.** Asserted in both versions, on a repository not reachable from this
  checkout. Unchanged by the compaction and unverified here.
- **Whether the named reviewer agents are installed on this machine**, which is the question
  `/agents` answers and which is why finding 2 mattered.
- **Whether the banked snapshot's figures would reproduce on another machine.** They would not, at
  full float64 precision — that is the finding recorded in `AGENTS.md` in the previous commit, and
  this snapshot is an arm64 one carrying no `GOARCH`. The provenance gap is owed work, not
  something this commit closes.
