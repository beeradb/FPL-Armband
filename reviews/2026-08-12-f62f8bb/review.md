# Review: the CLAUDE.md reorganisation

**Commit range**: `5eb5b6c..f62f8bb`, plus the corrections applied on top of `f62f8bb` in
response to this review.

**What changed**: CLAUDE.md 161,382 → 50,816 bytes. Retraction chains replaced by a "Closed
lines" block and a "Standing rules" block; failed experiments moved verbatim into `docs/notes/`
with two new notes created; the index compressed to verdict lines; two new resident blocks
added; three factual errors fixed. No Go logic changed — the one Go edit is the budget constant
in `internal/snapshot/notes_test.go`.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | yes | the change touches `CLAUDE.md` and `docs/` only, which is exactly the row this maps to in the triage table |
| fpl-code-review | **skipped** | no Go logic changed. The single Go edit is a test constant, and its effect — the guard firing — was observed directly rather than reviewed |
| fpl-stats-review | **skipped** | no measurement was made, no constant moved, no sweep run. Every claim moved is a claim that already existed; the question was whether it survived the move, which is an audit question |
| fpl-security-review | **skipped** | nothing under `internal/fpl`, `internal/agent`, config persistence, or the cache |
| fpl-run-review, fpl-season-maintenance | **skipped** | no live run, no season lists touched |

## The invariant came first, and it beat the reviewer to two of the defects

The skill's opening instruction is to ask what quantity the change must not move, and to test it
rather than dispatch someone. Here that quantity is **information content**: every substantive
claim in the old file must still exist somewhere in the tree.

The test: enumerate the old file by line range, assert a destination for every range, then grep
the new tree for a distinctive string from each. It found two blocks with no destination — the
three-bonus-regime table with its unresolved penalty-save contradiction (old 996-1058) and the
Isak/Haaland lock cases (old 1170-1188) — both fixed inside `f62f8bb` before review.

The audit then redid the check independently and far more thoroughly: numeric-token diffing,
word-token diffing, 6-gram shingle coverage per paragraph with everything under 45% read by
hand, and every bolded lead-in probed on its first nine normalised words. **It confirmed no
substantive claim vanished.** One worked example survives only in `reviews/` — see "declined".

That is the ordering the skill argues for, and it held: the cheap mechanical check caught the
two outright losses, and the expensive judgement check caught what it is actually good at —
claims that survived but got *stronger*.

## Findings applied

**1. The season table closed an arm the record holds open.** "What each season can and cannot
run" listed "the joint CBI-plus-tackled arm" as unmeasurable on all archived seasons.
`scoring-model.md:356` explicitly retracts precisely that claim, and `TODO.md` lists the arm as a
run to make. Verified by reading both. The resident file — the one loaded into every session —
stated the negation of a note correction written to retract it, and it forecloses the one
measurement that would sign the keeper result. **Corrected**: the row now says the *full
five-change figure* is unmeasurable (which is the record's actual claim, and the reason is that
no season carries both the modern saves baseline and a `tackled` column) and states that the
joint arm is measurable on 2016-19 and unrun.

**2. A closed line closed something that is open.** "Do not correct the clean-sheet
over-prediction" — but that sweep predates the xGC repair, and both `harness-and-inference.md`
and `TODO.md` name `FPL_CS_XGC_FACTOR` as an arm the repair un-blocked. **Corrected**: restated
as "ships uncorrected" with the mechanism, and marked explicitly as not fully closed.

**3. The 2019-20 row was incomplete in the direction that matters.** It carried only the `POLICY`
exclusion, omitting that the season has no native xG and reconstructed `starts`. A reader
building the xG-blind set from that table would have pooled 2019-20 as though it voted — the
exact error the block exists to prevent. **Corrected**.

**4. Six compressed bullets read more strongly than their sources.** This is the finding that
justifies the whole review. Compression dropped, in each case, the qualifier:

| bullet | restored |
|---|---|
| vice-captain | the clustered t is the retired estimator's output, re-derivable rather than current |
| team news | "suggestive, not established"; one raw p outside the Holm family; nothing ships changed |
| BPS decode | `pen_saved = 15` settles the pre-2024-25 leg only; the later contradiction is unresolved |
| bench slots | every figure in that tie was measured under the legacy 0.624 blank rate |
| xGC | the ~1% overshoot survives; only the predicted 2-4% died |
| noise split | "100% path noise" is a point estimate; the upper bound is unresolved |

**5. A count was inflated.** "A better predictor can make a worse policy — **five** numbered
arrivals." The record enumerates four and names them (`scoring-model.md:1990`). A count is how
that rule gets its force. **Corrected to four.**

**6. The contamination block.** Heading said four over a five-row table; the selling-price row
gave no direction (the record is unambiguous: figures predating the fix are *inflated*); and the
defcon-visible row attributed to visibility alone what the record attributes to four changes
jointly. **All three corrected.** The audit verified every figure in the table independently —
the zero-penalty 28/113, the doubles +115/+106, the substitution 7-14 and the defcon −95 (2136 →
2041 at `constants-and-sweeps.md:789`) all check out exactly.

**7. Dangling references.** `TODO.md` still pointed at the old canonical-block title — the
commit repointed ten references in the notes and missed this one. Two notes carried "the
zero-penalty bug **below**" with nothing below, a dangler that predates this work but whose
target now exists; both repointed at the contamination table.

**8. `recency-and-priors.md` lacked the contamination warning its sibling got.** Two notes
created in the same commit, both carrying totals-era three-season tables, only one warned. The
asymmetry reads as deliberate when it was an oversight. **Warning added.**

**9. Markdown.** Four appended headings had lost their preceding blank line. Found by a sweep
over every file this commit appended to; all fixed, all files now clean.

**10. The restored qualifiers cost 1.7 KB and broke the 48 KB budget.** Rather than
re-compressing accurate text back into the failure that had just been caught, the budget was
raised to 52 KB and the constant's comment now records *why*, naming all six bullets. The lesson
it encodes: **compression and overstatement are the same operation past a certain point, and the
qualifier is the first thing to go.** Raise it only to give a claim its hedge back, never to make
room for new evidence.

## Declined

**The bench-oracle worked example** (+0.1005 per blank → +7.0, not the +4.7 that made it
coincide with another arm's +3.9 and get written up as two instruments agreeing) now survives
only in `reviews/2026-08-12-2ab3f63/review.md`. The audit is right that this is the one case
where "keep the closed line, drop the history" cost something real — the anecdote *is* the
argument for the rule. Declined for now because the ×38 rule and its arithmetic are both in
CLAUDE.md and the example is one grep away in a durable location, and because re-importing
worked examples is how the file grew to 157 KB. **If it returns, it belongs in
`harness-and-inference.md`, not in CLAUDE.md.**

**`constants-and-sweeps.md:1059`** still states the old congestion penalty values unmarked. Real,
but it predates this change and the resident file now corrects it. Left for the next audit rather
than widened scope here.

**Go comments** naming moved material (`teamshare_test.go:549`, `reach_test.go:60`). Both are
soft references that still make sense to a reader; repointing them touches test files this change
otherwise does not.

**`docs/notes/scoring-model.md:256`**'s now-circular self-reference to the penalty-save
contradiction. The contradiction is now flagged in CLAUDE.md as part of finding 4, which restores
what the reference was for; the sentence itself is stale rather than wrong.

## What could not be checked on this harness

Nothing in this change is measurable — no figure moved, no sweep ran, and every claim is a claim
that already existed in the record. The distinction that matters here is a different one: the
audit's findings are all settled **by reading**, and none needs a run. Two of them (findings 1 and
2) are re-openings of closed lines rather than re-measurements, so they were edits rather than
work for `fpl-stats-review`.

The open runs this change *moved* out of CLAUDE.md and into `TODO.md` remain unrun and are
unaffected by it: the `TestDiagDefconWhere` re-run, the 81→60 defcon isolation, the bench-slot
tie at the current appearance rule, the 96-cell backfill check, the joint CBI-plus-tackled arm,
and regenerating `mde.csv`.

## Note for the next audit

The audit recorded a list of things it checked and found sound, so they need not be redone:
every verbatim move carries a dated provenance line naming what CLAUDE.md kept and why; every
sampled retraction chain is intact in its note with its ⚠️ and causing commit; the
"What the harness can resolve" compression is faithful, and its three-switch reproduction warning
is stated more clearly than the original; "Things that have already bitten" is now 16 entries
that are all genuinely shipped bugs with named regression tests, which is what the heading
promised and the old one did not deliver.
