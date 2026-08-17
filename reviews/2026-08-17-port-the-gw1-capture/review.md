# Porting the GW1 capture and the competition check onto the live line

Branch `port-the-gw1-capture`, based on `caf0d41` (tip of `origin/main`). One commit, this
record riding with it.

## What was reviewed

A commit that writes `config.json` and adds a captured payload. No Go code changed.

The GW1 free-path cycle was run earlier against a checkout of the now-retired private
repository, whose history is disjoint from this one — `git merge-base` returns nothing between
them, verified in both directions. Two artefacts from that run had not reached this line:

1. `data/captures/2026-08-17T0307Z/` — the point-in-time payload taken 110 hours before the GW1
   deadline. The series on this line ran 2026-08-10, 2026-08-13 and then stopped, so this was the
   irrecoverable half.
2. The competition-status verification, **re-derived** here by running `verify-competitions`
   rather than copied, because the two configs descend from different histories.

Added during review: `last_checked` stamped to 2026-08-17 on all nine standing overrides, which
were re-checked against current reporting during the same run and all still hold.

## What must not move, and did not

The shipped scoring configuration. `config.Load`'s backfill wrote two values into the file, and
if either differed from the code's default this commit would have silently changed a scoring
constant.

Checked directly rather than argued. `BlankRunPenalty: 0.75` at `internal/analysis/metrics.go:415`
and `LeagueShrinkK: 8` at `:440` are the code's own defaults, so the written values are the
shipped ones. Then end-to-end: `armband -plain squad` is **byte-identical** across the config
change, run against `HEAD~1`'s config via `-config` and against the committed one. Stamping the
nine `last_checked` dates is byte-identical again on the same comparison.

`go build`, `go vet`, `gofmt -l` and the full suite are clean.

## Reviewers

| reviewer | ran | triage |
|---|---|---|
| **fpl-run-review** | yes | this is the output of a live run that wrote config — the triage table's own row |
| fpl-code-review | **skipped** | no Go code changed. `git diff --stat` is `config.json` plus three data files |
| fpl-stats-review | **skipped** | no constant, estimator or measurement is asserted. The one quantity at risk — whether the backfill moved a scoring constant — was settled by a byte-identity check, which is stronger than a reading |
| fpl-security-review | **skipped** | no code path changed. See the declined finding on `sanitiseReason`, which is a real hole this commit neither opens nor widens |
| fpl-docs-review | **skipped** | no documentation changed |
| fpl-findings-audit | **skipped** | `AGENTS.md` untouched, no recorded verdict relied on or contradicted |
| fpl-season-maintenance | **skipped** | the four hand-maintained lists are unchanged. The competition *windows* were verified but not edited |

## Findings applied

**1. The commit message asserted a provenance the artefact contradicts.** I wrote that both
artefacts "were made against the retired repository". The capture's own manifest records
`fplagent_version: c104c3dc11f8`, and I verified that commit **is** an ancestor of `origin/main`
and is **not** an ancestor of the retired branch. `buildRevision` reads `vcs.revision` from Go's
build info and has no fallback that could invent a hash, so the stamp is real and my sentence was
not. Message rewritten to state only what is checkable: the files were committed onto the retired
branch, the build stamp resolves onto this one, and porting reunites them. I cannot fully account
for how a binary built in that worktree carried this line's revision, and the message no longer
claims to.

**2. `last_checked` was left stale on all nine overrides.** Verified the consumer chain:
`LastChecked` → `checkAge` → `NeedsCheck`, read on the **paid** path at
`internal/agent/prompt.go` where it appends `<- CHECK` and instructs the agent to search the news
for each one. `needsCheck` fires at seven days; the dates were 10-12 days old, so all nine were
flagged and the next `review` or `advise` would have re-run by web search the nine checks already
done, with the results replayed into every later request in that run. Worse, a flag that is always
set stops discriminating, which is the failure the field exists to prevent. All nine stamped to
2026-08-17; `brief` now reports none due a check. Each was genuinely re-checked: Saliba, Timber and
the Arsenal factor that rests on both; Mosquera; Kinsky and Dubravka on the same Tottenham
reporting; van Ewijk; Thomas; Isak.

## Findings declined

The run reviewer returned several findings about the **content** of overrides written before this
branch existed. They are recorded here because a declined finding that is not written down gets
re-raised every pass — but acting on any of them means changing what a footballer scores, which is
not something to fold into a commit that ports a data directory.

**The ARS ×1.15 factor's magnitude is unsupported by its own reason.** This is the strongest
finding in the review and it should be actioned separately. The override is live on `Score`
(`applyTeamOverrides` writes `TeamXGCFactor`; `metrics.go` multiplies `XGC90` by it), it lands on
six Arsenal assets including the most-owned keeper and the most-owned premium defender, and the
reviewer sizes it at roughly a 10% relative cut in clean-sheet probability. The reason's only
number — Arsenal conceding 0.67 without Saliba against 0.71 with him — runs *opposite* to the
correction, and the escape is a population (Saliba **and** Timber both absent) that was never
measured. That is close to the Gabriel case this project keeps as a worked example, and it sits
under the standing rule that a measured bias does not imply a correction exists. The right next
step is to measure the both-absent cell, or set the factor to 1.0 and keep the entry for its
reasoning. **Owed, and it is a scoring change requiring its own arm.**

**Six of nine overrides never reach the agent's system prompt.** `Roster.Active` returns lock,
exclude and expired only, so the prompt's "standing overrides" block lists two of nine while the
six minutes corrections and the club factor silently shape every score the agent reasons over.
The free `brief` path renders all nine, so a human sees them. Real defect, cheap fix, and squarely
a code change rather than a data port.

**`sanitiseReason` is applied in `Roster.Set` but not in `config.Load`.** Four reasons in this
config exceed the 400-rune bound because they were hand-written. No control characters and nothing
malicious, so there is no live problem — but the bound is unenforced on the load path. Notably this
is also the reason re-deriving the stamp was right rather than copying the config across
repositories, which would have been exactly the bypass the guard exists for.

**Dubravka and Saliba would be better as `minutes: 0` than exclusions**, and Saliba's stated clear
condition ("named in a matchday squad") is strictly later than FPL clearing his flag, which opens a
window where the model believes he is available and a standing exclusion silently refuses him.
Both are judgement changes to live overrides.

**The Kinsky reason's ownership argument does not survive symmetry** — Dubravka is *more* owned
(22.1% against 19.9%) in the same payload. The role argument stands on its own; the ownership
sentence should go. Editing a reason string is a content change, not a port.

**There is no free command to record an override re-check.** `verify-competitions` exists for
exactly this problem on the congestion side and its own comment makes the argument. The equivalent
for overrides does not exist, and `Roster.confirm` does not iterate `roster.teams` at all, so the
Arsenal entry has no write path outside a hand edit — which is what this commit did. A
`verify-overrides` command is the direct fix and belongs in the queue.

**Two pre-existing non-determinism bugs**, carried forward from the previous review record:
`cmdNations` ranges a map then sorts non-stably on one key, and `ChipWindows` ranges a map then
sorts stably on `(Stop, Start)` where Bench Boost and Triple Captain tie on both. Both reproduce on
an unmodified binary. Still owed.

**Smaller items recorded and not acted on:** `buildRevision` reads `vcs.revision` without
`vcs.modified`, so a capture from a dirty tree stamps a clean hash; `brief`'s club-override row
emits six columns against a seven-column header, so the Arsenal row renders misaligned; the Isak
absence is described as a leg break in `roster.go` and as a lost pre-season in `config.json`, and
one of them is wrong; the van Ewijk reason says "43 of Coventry's 44 Championship games" against a
46-game season.

## What could not be checked on this harness

- **Every real-world fixture date**, including Brighton's play-off legs. The reviewer has no web
  access; I verified them by search during the run, which is why the stamp was written, but that
  evidence is not reproducible from a checkout. Internal coherence is strong — all nine European
  clubs enter the League Cup at round three and all eleven others at round two, with no exceptions,
  and every European date falls on a plausible UEFA weekday.
- **The 0.67/0.71 Arsenal split and the 0.72 xGC** behind the ×1.15 factor. This checkout carries
  `data/captures/` but no season archive, so the numbers in that reason cannot be confirmed or
  refuted here. Settling the finding above needs a checkout with the archive.
- **Whether the nine overrides' underlying situations still hold.** FPL's own flags corroborate
  Saliba and Timber as unavailable; the rest rest on reporting checked during the run.
- **How the capture's build stamp came to name a commit on this line.** Verified that it does; not
  explained. The commit message now claims nothing about it beyond what was checked.
