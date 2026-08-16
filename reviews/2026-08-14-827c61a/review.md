# Review — round 3, the documentation of the config override contract

Branch `b5-season-list-parity`. Rounds 1 and 2 are recorded at
`reviews/2026-08-14-eb1b546/` and `reviews/2026-08-14-f0f0c9a/`; this covers the `docs/` change
that followed them, and the snapshot correction found while satisfying the gates.

## Reviewer

**fpl-code-review**, scoped to code-vs-docs correspondence.

⚠️ **`fpl-docs-accuracy` is the right reviewer and could not be used.** It was added to main in
`53b0757`, after this session started, so its definition is not loadable here. Recorded rather than
silently substituted — the next session should re-run this file past it, since it is scoped
precisely to "what a document claims the system does is what the code does".

## The contract was confirmed by measurement, not by reading

All eight fields, every form, through `Load` on real temp files:

| | absent key | one entry | `{}` / `[]` | `null` |
|---|---|---|---|---|
| the eight duplicated lists | Go default | wins | empty | **empty** |
| `review_policy.rules` (exception) | 5 | 1 | **5** | **5** |
| `minutes_weight_by_position` (exception) | 4 | **4, merged** | **4** | **4** |

I had got this wrong once already this branch — claiming "one rule for all seven" while
`tournament_absences` still had a `== nil` backfill — so it was checked by running rather than by
reading the struct tags. This time the claim held.

## Findings applied

**1. `null` was undocumented and pinned for one field of eight.** The prose said "`{}` and `[]`
included" and never mentioned `null`. That matters twice: the common belief is that `null` is a
no-op for `encoding/json` and it is not — it zeroes a map or slice — and **that belief is exactly
what the deleted `TournamentAbsences` backfill encoded**. The guard checked `null` only for
`european_campaigns`, so re-adding a `== nil` backfill to any of the six slices would break the
documented contract with the suite still green. **A doc claim outrunning its guard is how this
branch's own bug survived.** Subtest now covers all eight under both empty forms.

**2. The block broke the antecedent of the paragraph after it.** "`blank_run_penalty` is NOT one of
them" resolved to the "zero is honoured for…" list two lines above; inserting sixteen lines left
"them" pointing at the eight lists. Moved to after the numeric discussion closes.

**3. The enumeration read as exhaustive and was not.** `criteria` and `rest_regions` follow the same
rule and appeared in neither list. The block now states the general rule and names only the two
exceptions.

**4. "fixed-arity structures" is false for `review_policy.rules`** — it takes any number of rules,
and only `[]`/`null` trip its `len == 0` backfill. The property is "empty is a fallback trigger".
Corrected in the doc and the code, and both exceptions are now **asserted as exceptions** so the
claim cannot go wrong in either direction.

**5. `docs/README.md` said "every one needs a backfill in `config.Load`".** Eight shipped fields now
take an explicit exception and three never had one, so it was already loose; this change made it
quotable against the code.

Plus: four comments said "seven duplicated lists" where the guard checks eight.

## A finding that came from satisfying the gates, not from a reviewer

⚠️ **Main's newest snapshot misreports a model figure.** `2026-08-14-84a0945` banks the clean-sheet
block at **n = 2955, error 0.0592**. The code at that same commit produces **n = 2870, error
0.0698** — verified by running `TestDiagCleanSheetPoisson` on this tree.

2955 is the **pre-guard** sample. The `Fixtures != 1` guard has been in the code since `7f33270` and
is still there, so **no snapshot since that merge re-ran this diagnostic**; the recent rotations
carried the block forward from a `model.csv` that predates it.

**That is the failure the staleness guard exists to prevent, and it slipped through because the
guard checks that a snapshot EXISTS at the current commit, not that its figures were RE-MEASURED
there.** A snapshot rotated rather than retaken passes it. This branch's snapshot corrects the
figures; **the guard gap is not fixed and is worth a follow-up.**

## Declined

- **Extending the parity guard to `criteria` and `rest_regions`.** They follow the same override
  rule but are not *duplicated* — they exist once. This guard is about two copies drifting; adding
  single-source fields would blur what it checks. The doc states the general rule instead.
- **Fixing the staleness guard's rotate-versus-retake hole.** Real, and out of scope for a config
  branch — it needs a decision about how a snapshot proves its figures were measured at its own
  commit, which is a design question rather than a patch.

## What could not be checked

- **`fpl-docs-accuracy`'s own judgement**, per above.
- **Whether other snapshots in the series carry stale blocks.** Only the clean-sheet block was
  checked, and only against the newest snapshot. The rotation practice that produced it is not
  bounded to one figure.

## A structural wrinkle in the gates themselves, found by satisfying them

**Every snapshot commit owes a review record**, because snapshots are written under
`stats/snapshots/` and `stats` is a review-watched path. So the sequence
*change → snapshot → review record* cannot terminate in two commits: the snapshot itself trips the
review gate, and the record has to be re-keyed through it.

That is not obviously wrong — a snapshot is a claim about what moved, and this branch's snapshots
carry operator notes making substantive attributions, which genuinely are reviewable. But `stats` is
watched for its **R inference scripts**, not for generated artefacts, and the churn falls entirely
on generated output. Worth a decision rather than a patch: either exclude `stats/snapshots/`, or
state that a snapshot's notes are reviewable and the coupling is deliberate. **Not changed here** —
altering a gate to make one's own branch pass is the wrong reflex, and this record is the honest
alternative.

## Process notes worth keeping

**Three rebases, three sets of orphaned records.** Both gates key on commits, so every rebase
invalidates every snapshot and review record on the branch. Reviews can be renamed; snapshots
cannot, because `stamp.commit` is baked into `figures.csv` — renaming would leave the stamp lying,
so each needs a remove-commit-retake cycle. Main advanced 44 commits between two of them.

⚠️ **A concurrent reviewer agent working in this same worktree created and deleted a scratch test
mid-run**, failing one `go test ./...` before vanishing. The review-gate skill says to record which
files an agent owns and whether another is active; running one against the worktree you are also
testing in is the hazard it warns about.
