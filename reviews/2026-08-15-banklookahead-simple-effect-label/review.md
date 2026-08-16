# Re-label the `BankLookahead` null as a simple-effect null

**Reviewed:** `origin/main..HEAD` on `banklookahead-simple-effect-label`, one commit. A single
`CLAUDE.md` bullet in "The weekly transfer decision". No code changed; no figure moved; nothing ran.

**What it closes.** A queued item open since 2026-08-14 asked to re-label the two verdicts most at
risk from the simple-effect reading — *"neither is wrong; both are narrower than recorded"* — with
the explicit instruction to **mark them rather than re-run them**. The bench-slot half was
discharged earlier by a re-measurement. The `BankLookahead` half was not, and the bullet still read
"`BankLookahead` changes nothing and ships off" with no qualifier.

## Reviewers

| reviewer | verdict |
|---|---|
| **fpl-findings-audit** | ran; **seven findings, all applied** — see below |
| fpl-code-review | **skipped**: no code changed. The diff is one prose bullet in the resident index |
| fpl-stats-review | **skipped**: no measurement, no arm, no cell. The change *narrows the scope of* a recorded null and asserts no new quantity |
| fpl-security-review | **skipped**: no credential handling, no cache, no agent tool layer, no write path |
| fpl-run-review, fpl-season-maintenance | not applicable |

## Findings, ranked by how misleading the original draft was

**1. The mechanism claim was invented, and it was backwards.** The first draft said banking is worth
something "only when a *future* week is worth more than this one" and ranked **team news** as the
sharpest of the four conditions. `shouldBank` (`internal/backtest/simulate.go`) takes **one**
engine and prices both arms on today's board; its own doc comment says it "does not predict the
future" and that banking "has to win on the *size* of what it unlocks rather than on optimism about
timing". So the draft described a channel the code was deliberately written not to use. The
team-news ranking was also mine rather than the ask's — the ask lists four conditions and ranks
none — and it cut against the record's own team-news bullet (+15 a season against a threshold of
51, and −18 on `POLICY` for granular repricing). **Cut entirely.**

**2. "Taken at the shipped floor, charge and gate" was false.** Every arm of the `BANK` block sets
`WeeklyXI = true`, including the baseline labelled `"greedy (ships)"`, and so does the reach map's
base — while `runPolicySweep` passes `false`, with a comment saying changing that default would
silently move every block that does not set it. The label is what misleads, not the runs.
`stats/snapshots/2026-08-13-reach/FINDINGS.md` already self-flags this. **Corrected to name the
identifiers and the `WeeklyXI` departure.**

**3. The mediator is unchecked, and that is the qualifier that matters most.** Nothing counts how
often `shouldBank` fires. The reach map's "weeks with a transfer banked" is a **declared** firing
condition — a string literal — not a measured column, and `reach.csv` has no such column. The
standing rule is explicit that a byte-identical null may be a comparison that did not run at all,
and that the mediator must be checked before applying either reading. **Added**, and the
"never fired" reading is deliberately left as a labelled hypothesis rather than asserted.

**4. "Floor, charge and gate" recomposed what a closed line forbids composing**, and omitted
`BankUpTo` — the guard `shouldBank` reads *first*. The closed line says the gate's floor and its
horizon are one threshold written twice and must not be composed into one ladder. **Corrected to
name `MinGain`, `FreeCost`, `BankUpTo` and the horizon, without implying independence.**

**5. "The 2x2 against team news is a separate, unrun item" was broader than this checkout can
support.** No cross is banked here — verified — but "unrun" is an absence claim about a record that
is not in this repository. **Hedged to "No cross against team news is banked here."**

**6. The draft restated the standing rule instead of citing it.** The file already says
"true of the shipped configuration and silent about any other" and "becomes *untested* elsewhere,
not false". Saying it again in different words in the same file is one-quantity-two-implementations
in prose. **Replaced with a citation.**

**7. Placement and length.** The insert orphaned the bullet's unrelated second claim about the
late-season quiet, and ran ~740 bytes — roughly 5x its neighbours, in a file resident in every
request. **Moved to the end of the bullet and cut to ~310 bytes.**

## What was declined

**Nothing was re-run, and nothing should be.** The ask's last sentence is "mark both as
simple-effect nulls rather than re-running them", and re-labelling narrows the scope of a claim
without creating a new one.

**The "`shouldBank` never fires" hypothesis was not resolved.** It is consistent with the
byte-identical result but unmeasured, and resolving it needs a **counter, not a sweep**. Left
labelled as a hypothesis rather than promoted or dropped.

**The partial-resolution gap in the baseline picker was left alone** — a sibling change on `main`
documents it with its detection route; it is not this change's business.

## What could not be checked on this harness

**Nothing here is a measurement, so no detection threshold applies**, and this harness's scale
(median 39 points a season; 33 on `HOLD`, 70 on `POLICY`) must not be quoted against it. The edit
changes what is claimed *about* a recorded null — its scope and its provenance — not its value.

**Whether the cross has ever been run outside this repository cannot be checked from here**, which
is why finding 5 is hedged to what is banked in-tree.

**One trap found while writing this record, worth knowing.** `TestReviewCoversTheCurrentCode`
digests **HEAD**, while `reviewkey` digests the **staged index**. So a staged-but-uncommitted change
to a watched path leaves the gate passing: a green suite before committing is *not* evidence the
record covers your change. The two agree once the change is committed, so this is a property of
when the gate can see work, not a false pass in committed state.
