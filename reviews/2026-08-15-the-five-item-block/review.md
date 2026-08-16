# Five queued items, and three defects found by doing them

Covers `c47b86c..HEAD` on `list-block-2026-08-15` — the five items picked off the queue after the
staleness-guard work, plus what turned up while doing them.

**Dispatched, not first-party.** `fpl-feature` built items 1, 2 and 3; `fpl-stats-review` reviewed
the *plans* for 2 and 3 before either drew a conclusion; a general agent did item 5; a further agent
checked the queue notes. I wrote items 4's follow-ups and the provenance guard, and I reviewed none
of my own work except by running it.

**The reviews earned their cost twice over.** The statistics review invalidated my own brief for
item 3 before the run, and the queue-note audit found a live scoring path documented as inert six
days before GW1. Neither was in scope for the item that surfaced it.

## The five items

**1. `stats/concentration_screen.R --metric`** (`a359de4`). Default HOLD output byte-identical,
diff-verified. POLICY screens 109 contrasts against HOLD's 85, of which exactly 2 are
byte-identical and both are recorded theorems. Three further defects fixed: the parser read
`--metric=x` as a *filename*; a `_points` column would have been silently rescaled by 38; and
`no screenable arms found` was one string for three situations, only one of which is a result —
the ambiguity that sent the gate arm to a hand pass.

⚠️ **And one the agent surfaced and deliberately left, which I took.** 26 contrasts were dropped at
`all(diff == 0)` in total silence, so "53 of 85 arms flagged" was really 85 of 111 considered. They
are exactly the transfer families, byte-identical on HOLD *by theorem*. Now counted, and the count
doubles as the pointer to `--metric=policy_per_gw`. This changes the default output by one line;
the byte-identity requirement was mine and existed to prove no regression, not to protect a silent
drop.

**2. Wild cluster bootstrap** (`8509837`). **Rademacher was the wrong scheme and the file I pointed
the agent at was an argument against the statistic, not a helper** — at four clusters it cannot
reject at 5% however large the effect, so it would have returned "not significant" for the perfect
armband at naive t 20.43. Webb 6-point weights instead, **enumerated exactly** rather than sampled:
46,656 draws cost 0.054 s, so there is no seed, no Monte-Carlo error, and no RNG state — which
removed a real defect where the first call in a fresh session installed `.Random.seed`.

The agent then caught its own error: **the floor is `6/6^S_eff`, not `2/6^S_eff`**, because six
draws tie `|t|`, not two. The wrong constant told 22 banked arms at `S_eff = 3` that their
instrument reached 0.0093 when it reaches 0.0278.

**No clean retraction exists in the bank.** Of 34 CR2-significant rows, 29 agree and 5 are
withdrawn — four of them three-season arms, the fifth `oraclegate` POLICY at 0.054. The rule
adopted is asymmetric: it may withdraw support from a CR2 rejection and may never grant one.

**3. The residual-only gate arm** (`8509837`). Verdict: **decision quality dominates.** Realised
POLICY +90.5 against its own threshold of 59.0 — the positive control firing — and a **reversal on
the discriminator**, `policy_xpoints` −22.1, t −1.06, positive in 10 of 36 cells against 34 of 36
for underlying. `RESIDUAL − UNDERLYING` on `policy_xpoints` is −94.8 a season, CR2 t −6.96.

**4. `stats/regenerate_mde.sh`** (`a359de4`, `178b80d`). Reproduces the canonical population:
n = 23, median **39.6**, range 7.6–231.9 against the recorded "23 comparisons, median 39, spanning
3.9 to 232". The median is one command from committed cells again.

**5. `docs/replay.md`** (`a359de4`). All four load-bearing corrections plus five more, and
`docs/architecture.md:211`, which carried the same defect in a worse form — a *prescription* to
measure at 24 cells. A stale figure about a past measurement stays true as a four-season figure; a
stale instruction is simply wrong.

## Findings the items did not ask for

### My brief for item 3 was invalid, and the review caught it before the run

`XPoints = Points − XPointsResidual` (`xpoints.go:339`), so the three gate criteria are an exact
additive decomposition and the "new" arm is the other half of the arm already run. All three are
scored on realised points, so an oracle foreseeing any additive component raises that metric **by
construction**: "the residual arm gains" was the expected outcome under my hypothesis *and* its
negation, and could not discriminate.

Rebriefed to read four arms on two metrics with `policy_xpoints` as discriminator — which costs
nothing, the column was already banked. ⚠️ **The pre-registered confound predicted
positive-but-smaller** on the discriminator, because xPoints retains realised bonus and bonus is
awarded largely for the goals and assists the residual replaces. **It came out negative**, so the
result sits on the far side of its own confound.

### A model CSV mixes runs across branches, and that shipped a wrong snapshot to main

Regenerating a snapshot for a **comment-only** change moved seven clean-sheet figures. It was not
the comment.

`FPL_MODEL_CSV` **appends**, its path is outside the repository, and the renderer keeps the newest
row per figure. On a machine where several branches share a scratch path, "newest" means newest in
wall-clock time, not produced by this code. The file I reused held **eight runs going back days**.

⚠️ **`stats/snapshots/2026-08-15-9e743cf`, on `main`, reports the clean-sheet rate over 2955
team-matches where its own commit produces 2870.** The 85 difference is exactly the doubled rows
`3b6a698` removed — and `3b6a698` is an **ancestor** of `9e743cf`. The snapshot's commit had the
fix; its figures did not.

`assemble`'s comment says a model measurement is deterministic, so the later run is the one that
ships. **True within a checkout, silently false across them.** `snapshot.ModelRunIDs` now reports
the mixture and the snapshot command stamps it as a problem.

✅ **The correction moves toward the record, not away.** The over-prediction reads **29.3%**
post-fix against CLAUDE.md's recorded "~30%"; the contaminated snapshot read 24.3%. The doubled
rows were understating a bias the record had priced correctly. Nothing in CLAUDE.md needed changing
— the snapshot was the wrong one. Retaken as `2026-08-15-8509837` on a clean tree from a single-run
CSV.

### A live scoring path documented as inert, in four places, six days before GW1

`b9da35c`. **`DefaultRestPlayers` is not display-only.** `blendFor` applies `restFactor` at
`blend.go:165`, scaling `MinutesPerMatch` and `StartShare` by `rest_minutes_factor` 0.83 — a
`Weights` field, not one of the eight `Congestion` penalties — at **GW1 and GW2 only**, which is
exactly the two gameweeks after the summer maintenance meant to have checked the list.

Wrong in `CLAUDE.md`, `docs/architecture.md`, `docs/model.md`, and the doc comment of
`TestTheShippedCongestionBlockIsInert` — the test the other three cite. Three things kept it alive
and each will recur: **two unrelated mechanisms answer to "rest"**; **the obvious check refutes
itself**, since `restFactor`'s other call site is labelled "Reporting only"; and **the count was
right for the wrong reason**, because `DefaultNewCoachClubs` is display-only via a *ninth* penalty
outside the block.

### The commit that guarded one leak form shipped another

`c47b86c`, titled "Guard the wikilink leak this change shipped", added four word-form references to
a store outside this repository in its own review record, and a fifth in its own commit message.
The tree ones are removed in `b9da35c`; the message is on `main` and cannot be. **A word-form guard
is owed and is not written** — six more live in older review records and two generated
`snapshot.md` branch fields, which are banked provenance and are left alone deliberately.

`TestNoTrackedMarkdownCitesAWikilink`'s own first run caught a false positive — R's `blocks[[b]]` —
so "code blocks are not exempt, a fenced example teaches the syntax" was backwards and code spans
are excluded.

### The MDE six-season line was not comparable with the canonical one

`178b80d`. The four-season side is filtered to a settings population; the six-season side was not.
Seven of its 13 rows were `noise6` floor arms *built not to move*. Filtered symmetrically there is
**no six-season settings population** — `vice6` alone, n=2, itself a control. Anyone quoting
"median 8.4" as a six-season threshold would have been badly wrong.

Fixed better than patched: the `seasons` column is already in every row, so the hand-maintained
`SIX = {noise6, vice6, teamnews}` is **derived** now. Reproduces those three exactly, no figure
moves, and it removes the mechanism behind the asymmetry — two of the three names were themselves a
noise arm and an oracle.

## What was declined, and why

- **Moving the D1 closure between queue files.** The audit argued moving is not copying and I think
  it is right; I left it because the section it would join is already duplicated across two files
  and I was not going to make a third pattern at this hour. Recorded, not forgotten.
- **A word-form leak guard.** It would fail immediately on six pre-existing tracked references and
  two generated branch fields. The scope decision — redact banked provenance, or exempt it — is
  not mine to take unilaterally.
- **Repointing two `reviewgate_test.go` citations** that were exact when written and dangle now
  because this branch deleted the code they named. Marked in place instead.
- **A commit column in the model CSV.** It is the right fix and would make the mixture a hard
  failure rather than a warning. It changes a schema `modelcsv_test.go` pins, with four agents in
  one worktree. Filed.

## Corrections to my own earlier reporting

- I told the item-5 agent `docs/replay.md:377` cited `stats/out/mde.csv`. It cited a bare
  `mde.csv`. The agent checked and said so.
- I said `mde_aggregate.py` *silently* absorbs foreign directories. It names them. Reading the code
  instead of repeating the warning is what turned up the real defect.
- `a359de4`'s commit message says `SIX = {noise6, vice6, teamform}`; the literal was `teamnews`.
- I flagged the queue's 58-vs-63 count as unreconcilable rather than counting. The audit counted:
  the table's 22 should be 17, and 58 is right. The principle was sound and the situation was not
  one it applied to.

## What could not be checked

- **The word-form leak inventory is a grep, not a guard**, so it is complete only as of today.
- **`stats/gate_recovered_fraction.py` hardcodes the 2026-08-14 path and arm indices 1–2.** Item 3's
  contrasts ran from an adaptation outside the repo; the committed script does not reproduce them.
- **The bootstrap's 300-design verification is not checked in** — the enumeration identity is
  asserted in the R code but not pinned by `cells_reader_selftest.R`.
- Nothing here re-measures a shipped constant, so no threshold in the record moves.

## Gates

`go build ./...`, `go vet ./...`, `go test ./...` clean, including `internal/backtest` at 152 s.
Accuracy snapshot retaken on a clean tree at `2026-08-15-8509837`. CLAUDE.md budget raised to 76 KB
rather than compressed, as that test now instructs, with the claim that needed the room named in
its comment.

---

## Second audit round, same day: three counting errors and a half-fixed generator

A second pass over the queue notes found two things that land in this repository.

### `render.go` was recorded as fixed and was fixed once of four times

`a3233c9` removed one falsified sentence from the generated snapshot prose. **Three more sites in
the same file survived** — `:187`, `:310`, `:634` — still generating "the four seasons … only three
degrees of freedom … 3.18, not the familiar 2". That is *verbatim* the defect corrected in
`docs/replay.md` and, as a prescription, in `docs/architecture.md:211`; six seasons give df 5 and
`t_crit` 2.571, and the df belongs to the **comparison** rather than the grid, so a fixed number is
wrong even at a fixed grid. **Eight banked snapshots carry the old sentence.**

⚠️ **It was missed twice for the same reason: nobody greps generated prose.** Which is exactly why
the queue item calls this file the worst case — a reader meets the claim as *this run's own output*,
not as documentation that might be old. Fixed at `e056821`, which states no df and no `t_crit` and
points at the computed per-arm table. Snapshot re-rendered; figures byte-identical, verified by
diffing every non-stamp row.

### Two standing rules promoted out of the queue notes and into `CLAUDE.md`

Both had **three verbatim copies** in the notes with no generator — the drift shape those same notes
name. A rule only works if it is resident where the next brief gets written.

- **"Review the plan, not just the output, for anything that will produce a number."**
- **"A snapshot's figures are not guaranteed to have come from its own commit."** — the "check which
  *file* a number came from" rule one level up: right file, wrong *checkout*.

The budget was raised to 78 KB rather than compressed, per that test's own instruction, and both
claims are named in its comment. ⚠️ **Second consecutive raise.** Both are rules rather than
evidence, and the comment now says to watch whether that stays true — three raises for evidence
would mean the boundary has moved.

### Three counting errors in one day, each a different way of not counting

Recorded because the third was committed *inside the paragraph correcting the second*.

1. **Flagged instead of counted** — a mismatch marked "unreconciled" when the recount was one
   command, which the flag itself said to run.
2. **Derived instead of counted** — "six closed and four opened, so 56". Wrong twice: one item was
   already ticked and another was never a checkbox item at all.
3. **Counted, but stamped with a derived date** — "58 at the start of the day" was today's correct
   58 projected backwards over a same-day addition. It was 57.

**A count is counted at a stated commit or it is not counted.** The principle that started this —
don't pick a number to make the arithmetic work — was right; what it missed is that *deriving* and
*back-dating* are the same failure in different clothes.

### Also corrected in the notes, all verified against committed objects

The MDE range was quoted across two estimators without saying so (7.6 is the season-clustered
minimum; the recorded 3.9 is start-fixed) — a direct hit on the standing rule that an estimator swap
reads as a data change. The document-drift item named five documents where the write-up said six and
listed seven. Three different closure counts for one day's work. And the five contamination
magnitudes had been copied out of `CLAUDE.md` one sentence after declaring they were not.

### What the audit verified and found sound

Roughly forty figures, line numbers and commit facts. Every cell of the residual-arm table
recomputed from banked cells, including the three thresholds, which are not in the source and had to
be re-derived. The bootstrap arithmetic, the 26 dropped contrasts, the six-season filtering, the
contamination claim in all four parts, and every `file:line`. Nothing was found wrong that was a
wrong *number* — the failures were correct numbers carrying wrong qualifiers.
