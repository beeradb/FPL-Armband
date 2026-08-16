# Review — the correction convention

## What was reviewed

Two files, one change: a new `## Conventions` bullet in `CLAUDE.md` stating that a correction
**replaces** the claim rather than narrating the replacement, and a paragraph in
`internal/snapshot/notes_test.go` recording why that entry of the never-resident list is the one
people breach without noticing.

This rides on the `guard-the-wrong-regressor-class` branch but is **not** that branch's work — the
scan and this convention are independent, and the scan's own commit deliberately excluded these two
files rather than absorbing them.

## Why the convention is a restatement, not a new rule

`TestTheResidentIndexStaysSmall` has always carried *"Never resident, at any size: sweep tables,
derivations, worked examples, **the history of a retraction**."* The rule existed; practice had
drifted from it. What the change adds is the rule stated **positively**, in the file a writer is
editing, rather than only in the comment of the guard that fires once the bytes are already spent.

The drift has a cause worth naming, and the `notes_test.go` paragraph names it: writing *"X was Y,
now withdrawn, because Z"* **looks like diligence rather than growth**, so nobody recognises it as
the breach. It is also the correct behaviour in the other store — a correction is marked in place
where the evidence lives — which is why the two conventions being opposites is stated explicitly
as deliberate. Anyone finding the asymmetry will otherwise try to reconcile it.

## Which reviewers ran

- **`fpl-findings-audit`** — ran over this text as part of the concurrent scan work, since both were
  in the same worktree. Its finding is recorded in `reviews/2026-08-16-the-wrong-regressor-scan/`.
- **`fpl-code-review` — skipped, and the skip is a decision.** No behaviour changes: one bullet of
  prose and one comment. The only Go edit is a comment block inside an existing test's doc comment.
- **`fpl-stats-review` — skipped.** No figure, threshold, estimator or verdict moves. Nothing here
  is a measurement.

## Findings and what was applied

**Applied — the bullet violated its own rule.** Two of its five clauses narrated the bullet's own
provenance: a sentence explaining that the rule already existed in the size guard and that this was
it restated "where a writer meets it instead of where a reader hits a byte budget". That is the
history of how the text came to be, inside the bullet forbidding exactly that. Both clauses were cut.
The bullet is now the rule, the deliberate asymmetry with the other store, and one caveat.

**Kept deliberately: the `notes_test.go` paragraph DOES narrate, and that is correct there.** This
file's conventions ask comments to explain *why*, especially where a value was calibrated or a bug
was fixed, and the paragraph's whole content is the diagnosis of a recurring breach — including
that this branch's predecessor committed it and needed a budget raise to fit the narration. A test
comment is not the resident index; the never-resident list governs `CLAUDE.md`, not its guard.

## What was declined, and why

- **Adding an assertion that no `## Conventions` bullet narrates its own provenance.** Considered and
  declined: the rule is about *what a correction does to a claim*, which has no syntactic
  fingerprint. A scan keyed on "was" or "withdrawn" would fire on legitimate prose — including the
  bullet's own reference to a withdrawn reading — and a guard that fires on the correction it
  enforces is a recorded failure mode of this repository's guards.
- **Deleting the never-resident entry now that the rule is stated in `CLAUDE.md`.** Declined: one
  quantity, two implementations is this project's signature failure, but these are not two copies of
  one quantity. The list is what the guard checks against; the bullet is what a writer is told. They
  serve different readers at different moments, and the bullet points at no line number so neither
  rots when the other moves.
- **Raising the size budget.** Not needed. `CLAUDE.md` is well inside it after the edit.

## What could not be checked here

- **Whether the convention will hold.** It is prose, and prose is not a guard. The preceding
  paragraph in `notes_test.go` records one breach; nothing detects the next one, and the reason
  nothing does is recorded above under what was declined.
- **Whether the other store's opposite convention is stated as clearly at its own end.** That store
  is not readable from a fresh checkout of this repository, so this review cannot verify it, and the
  claim here is limited to what this repository's rule is.
