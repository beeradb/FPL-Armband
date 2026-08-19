# Review: the AGENTS.md compaction onto the research vault

## What was reviewed

Commit `a36b1b3` plus the review-fix commit that follows it: AGENTS.md fell from 149,561 to
~78,200 bytes; derivation narratives moved to the vault notes the entries point at; the budget
test's ledger and ceiling were rewritten (150 KB → 80 KB). The old file is recoverable as
`git show origin/development:AGENTS.md`.

## Reviewers

- **fpl-findings-audit** — ran, briefed on the five severity-ordered checks (no verdict lost,
  no number altered, no qualifier dropped where the vault does not carry it, standing rules
  intact, operational sections intact). Verdicts: all 24 closures and every measured bullet
  accounted for; every checked number identical between old and new. Five findings, dispositions
  below.
- **fpl-docs-review** — ran. Found the dangling `xppilot` pointer (inherited from the old
  file), the header pointer-format slip, and seven missing glosses. All applied.
- Skipped: **fpl-stats-review** (no measurement claim added — the compaction moves text and
  changes no figure; every number was verified identical by the audit), **fpl-code-review**
  (the only code change is the budget constant and its comment), **fpl-security-review** (no
  client/agent/config surface touched).

## Findings and dispositions

1. **Audit, applied**: the `OptionPricing.CongestionSensitivity = 0` trap bullet was absent —
   restored in full with its test name.
2. **Audit, applied**: the squad-hash-is-weaker-evidence observation was absent from both files
   — restored into the teamBands bullet with its counts (squad_hash moved in 1 cell against
   hold_points's 3).
3. **Audit, applied**: `TestBandTiesBreakTowardTheLowerClubID` restored beside the other
   pinning test.
4. **Audit, applied**: the three fixture-load pinning test names restored (vault carried them;
   the resident file names all five again).
5. **Audit, left in vault by design**: the 2.36% oracle degradation figure — a detail of the
   perfect-minutes entry, carried by the vault notes; the verdict's qualifiers survive in the
   resident text.
6. **Docs, applied**: created `notes/xppilot.md` in the vault carrying the xPoints pilot record
   (the tuning closure, the two conversion scales, the 0.64/Fieller gate result, and the gate
   arms), and repointed the conversion-scale and underlying-criterion bullets at it. The
   dangling pointer was inherited from the old file but is more visible now that the header
   describes the pointer mechanism.
7. **Docs, applied**: header pointer format corrected to `→ **name**`; seven Go identifiers
   glossed on first use (`Bonus90`, `xiValueShrunk`, `DefConCleanCoupling`, `ChipResetGW`,
   `ChipCredit`, `BenchBoostGain`, `BenchBoostTrigger`).

Nothing was declined.

## What could not be checked on this harness

The vault notes that received the moved text are reviewed by the vault's own audit process, not
by this record; the audit verified that what was cut is *present* in the notes, and the
compaction's own budget comment names the residual failure form to watch — a qualifier moved
somewhere the reader will not follow.
