# The `AGENTS.md` rename's fallout across five documents

## What was reviewed

`docs/README.md`, `docs/architecture.md`, `docs/configuration.md`, `docs/model.md`,
`docs/workflow.md` — scoped deliberately to one question: **what did the `CLAUDE.md` → `AGENTS.md`
rename break?**

Another session moved the resident index; `CLAUDE.md` is now a 424-byte file whose whole content is
`@AGENTS.md` plus a comment explaining that Claude Code reads `CLAUDE.md` and not `AGENTS.md`. That
rename swept through these five.

The reviewer was told explicitly that **"it came through clean, nothing to fix" was a good outcome**
and not to manufacture findings. It did not come through clean.

## Applied

1. **A stale number the rename walked straight past.** `docs/README.md`'s map node said "68 KB
   budget"; `notes_test.go:622` reads `const budget = 106 * 1024`, and `AGENTS.md` is 104,927 bytes.
   ⚠️ **The direction of the error is what matters**: a reader concludes the resident index is 34 KB
   *over* budget and the guard must be failing, when there are 3,617 bytes free. The standing
   instruction is "raise the budget rather than compressing a qualifier", which that reader would
   then get exactly backwards. **The number is removed rather than refreshed** — it has been raised
   at least seven times and will go stale again; the file that holds it is named instead.
2. **Three dated attestations rewritten to name a file that did not exist on the date they cite.**
   "Moved from AGENTS.md 2026-08-12" — but the resident index was `CLAUDE.md` until 2026-08-16. A
   reader tracing provenance searches `AGENTS.md`'s history before the rename and finds an empty
   file. Now "the resident index (then `CLAUDE.md`)".
   ⚠️ **This is the rename commit's own doctrine applied one file short.** That commit deliberately
   excluded `reviews/` and `stats/snapshots/` because "the pointer moves but the record of the
   pointer does not". A "moved from X on DATE" sentence is the same kind of object, and was
   substituted anyway.
3. **Both guard-scope lists understated what the guards cover.** They said
   `TestRetractedFiguresAreNotQuotedAsCurrent` scans `AGENTS.md`, `docs/*.md`, `stats/*.md` and Go
   sources. `retracted_test.go:352-360` also builds **the root `README.md`** and **`.claude/*.md`**,
   added 2026-08-15. And `stats/*.md` is three files, not one.
4. **The map never mentioned the `CLAUDE.md` bridge.** After the rename `docs/` described a one-file
   arrangement while the root holds two, so the obvious reading of a stray `CLAUDE.md` is that it is
   a leftover to delete — which would **silently stop the project's instructions loading in Claude
   Code**, the exact failure the rename was written to avoid. Folded into the finding-1 wording.

## The correction I owe on my own record

⚠️ **Finding 3 is a claim I repeated without checking, and it is now corrected in two places.**
`reviews/2026-08-16-the-readme-write-boundary/review.md` said the root README is "the one
user-facing file with no mechanical floor at all — no retraction guard". **False.** The retraction
guard has scanned it since 2026-08-15. I verified that against `retracted_test.go` after this
reviewer flagged it, and amended that record in place with the correction dated.

**The propagation path is worth naming, because it is the reusable part.** `docs/README.md` carried
the understatement → a reviewer read the document and repeated it in a report → I quoted the report
into a review record → and I then briefed *this* reviewer with the same wrong claim, which is how it
was finally caught. **A wrong claim about a guard's reach is self-propagating**, because every
downstream writer treats the document as the authority on what the guard does. The fix has to be at
the document, which is what finding 3 does.

## Declined

- **Renaming the mermaid node id `claude`** in `docs/README.md`. It is never rendered and the id is
  not user-visible; changing it is churn. Recorded so it is not re-raised.
- **A second diagram box for the 424-byte `CLAUDE.md` import.** The reviewer's reasoning is right —
  a box restating one clause is the failure mode a diagram is supposed to avoid. One clause in the
  existing node instead.

## What the rename got right, recorded so it is not re-checked

- **No stray `CLAUDE` remains** in the five files.
- **Nothing was renamed that should not have been.** `docs/workflow.md`'s "Hand it to a Claude Code
  session" is about the *tool* and was left alone — that was the main hazard.
- **Every cited `AGENTS.md` section name resolves**, the one markdown link resolves, and
  `internal/snapshot/watched.go` was updated, so the review gate did not lose its subject.
- **Grammar survived** all six rewritten lines — no "the AGENTS.md file" artefacts.
