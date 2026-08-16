# The README's write boundary, and a ratio I got wrong correcting a ratio

## What was reviewed

`README.md` and `docs/replay.md`, the latter narrowly — only the six applications from
`reviews/2026-08-16-the-third-replay-doc-pass/`.

⚠️ **A constraint I gave the reviewer was wrong, and it changes what this record covers.**
`README.md` is **not** in `ReviewWatchedPaths` (`internal/snapshot/watched.go`): the list is
`internal/analysis`, `internal/backtest`, `internal/agent`, `internal/fpl`, `internal/config`,
`internal/snapshot`, `stats`, `docs`, `config.json`, `CLAUDE.md`. So the README findings owe no
review record and only the `docs/replay.md` correction does.

⚠️ **CORRECTION, 2026-08-16, same day.** This record originally added that the README is therefore
"the one user-facing file with no mechanical floor at all — no retraction guard". **That is false,
and I verified it against source after a later review caught it.**
`TestRetractedFiguresAreNotQuotedAsCurrent` builds its surfaces at
`internal/snapshot/retracted_test.go:352-360` and they include **the root `README.md`** and
`.claude/*.md`, added 2026-08-15. So the README is outside the *review gate* and inside the
*retraction guard*. The claim came from the reviewer's report, I repeated it without checking, and
`docs/README.md` carried the same understatement — which is how a wrong claim about a guard's reach
propagates: the document says it, a reviewer reads the document, and the next writer quotes the
reviewer. This record exists anyway, because that is the reason to write one, not the reason not
to.

## The finding that prompted the pass

**"Advisory only" was true about FPL and wrong about your machine.** The second sentence — "it never
touches your FPL account" — is true in the strongest available sense, and the reviewer confirmed it
against source. But "advisory" is not: `set_player_status` writes `config.json` through
`Toolbox.updateConfig` → `config.Save`, and **both `reviewPrompt` and `advicePrompt` instruct the
agent to call it**. It is the normal path, not an edge case.

**And the writes bind the *free* commands afterwards.** `applyRoster` pushes stored locks and
exclusions onto every `squad`/`transfers`/`brief` optimise — its own comment says "so the free
deterministic commands solve the same problem the agent does" — and `squadBrief` raises "*N
hand-set overrides bind this squad; it is not the model's unaided answer.*" So one `review` run can
durably change what `fplagent squad` prints next week, and the README never said so.

Replaced with **"It writes to your config, never to FPL"**, plus a paragraph on what a review run
leaves behind. That is both more accurate and more reassuring, because the reassuring half is
**checkable**: a named test fails the build. "Advisory only" is a vibe; the boundary is a fact.

## Applied

1. The write boundary, above.
2. **A safety claim that was false for the one live list.** "Because all eight congestion penalties
   ship at 1.00, a stale list can no longer mis-score a player" reads as covering all four
   maintained lists — including `rest_players`, which multiplies expected minutes inside `blendFor`
   and is **live at GW1 and GW2**, the two gameweeks straight after the maintenance meant to catch
   it. The README was contradicting `CLAUDE.md`'s own season-maintenance verdict.
3. **A multiplier's channel, stated backwards.** The summer-signing discount ships at 0.88 as a
   multiplier on **`Score`**, calibrated *against* a minutes gap — not "on minutes". The README then
   contrasted it with the rest discount "rather than on the score", implying both were on minutes.
   ⚠️ This is `CLAUDE.md`'s *"check what a multiplier multiplies before calibrating it"* verdict
   violated inside the README — and getting exactly this backwards is what retired the new-manager
   penalty.
4. **"All eight congestion penalties measured as nothing" overstated the evidence.** Six measured
   null *on the channel they are applied to*; the domestic-cup and long-haul penalties were **never
   measured** and ship at 1.00 because that is the neutral value. Long-haul was in fact live at 0.86
   on Brazilians and Argentines while a comment claimed it inert.
5. **A roadmap promise for a surface that was deliberately deleted.** "Will eventually run
   unattended just before a deadline, once FPL authentication is added" — `due` already runs
   unattended, and the authentication clause describes a removed surface as pending work, quietly
   softening the line the README opens with.
6. **`due` runs `advise`, not `review`** — different prompts, so a real cost and behaviour
   difference.
7. **The `review` chain dropped the step its own prompt puts first** — "competition status comes
   first because it changes the numbers everything else rests on."
8. **The docs map named a category removed on 2026-08-14** ("which describe intentions"). All eight
   docs are reference now.
9. **39 points quoted as a harness constant.** It is the season-clustered *median of 23
   comparisons*; the same set reads 32 start-fixed and individual comparisons run 12.7 to 232. The
   README's own limitations section already got this right, so the two paragraphs disagreed.

## The one I got wrong myself

⚠️ **My replacement for the retired "sevenfold" claim was itself overstated.** I wrote that 70 MB is
"low by a third to a half" against the banked 89-142 MB band. It is **a fifth to a half**:
(89−70)/89 = 21%, (142−70)/142 = 51%. Corrected.

**That is the same defect class the pass was hunting** — a ratio written beside the numbers that
refute it. Retiring an unsourced multiplier and replacing it with an unchecked one is not an
improvement, and it is worth recording that the correction needed correcting.

## Declined

- **The write-surface diagram.** The reviewer drafted it and it is the best-argued of the three
  diagrams proposed today — it shows an *asymmetry* prose cannot, and a **loop** (the agent writes
  config, which feeds back into the free commands) that needs three clauses to say. ⚠️ Declined on
  the same ground as the previous two: **not rendered**, and `TestMermaidBlocksAreWellFormed` is not
  a renderer. The reviewer flags the specific risk — a dotted edge to "your FPL team" labelled *no
  write path exists* may read as a connection rather than a barrier, which would invert the meaning
  on the one claim that most needs to be unambiguous. **Recorded, not dropped.** This is the third
  diagram declined for want of a render step; that is now a pattern worth fixing rather than
  repeating.
- **Listing all thirteen write paths in the README.** The reviewer enumerated them; most are
  ordinary (`reports/`, `.cache/fpl/`, `data/captures/`). Only the roster override changes future
  output, and that is what was added.
- **Adding `backfill` and `reviewkey` to the command table**, the intro-pricing sample caveat, and
  the `accuracy.md` row. All correct, none load-bearing; left for a pass that is about the command
  table.
- **`main.go`'s usage string** carries the same `due`/`review` error as the table. A code change,
  handed on.

## What could not be checked

- **Whether the diagram renders.**
- **The 39-point median itself** — the reviewer reported, correctly, that adjudicating it is
  `fpl-stats-review`'s. What was fixed here is that the README quoted a per-comparison median as a
  harness constant.
