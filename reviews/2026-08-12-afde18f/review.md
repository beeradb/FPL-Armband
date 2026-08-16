# Review: merging main into the CLAUDE.md reorganisation

**Commit range**: `afde18f` — the merge of `origin/main` (three commits: `9d5774a`, `fb86c76`,
`e44f65f`) into the reorganisation branch — plus the corrections applied on top in response to
this review.

**Why the merge was not mechanical.** main added a 31-line block to CLAUDE.md recording that the
`prior_half_life` experiment has been run, annotating the very block this branch was moving out to
`docs/notes/`. Git could not align the two: the conflict spanned the whole tail of the file, this
branch's compressed version against main's uncompressed one. Resolved by hand — structure from
this branch, every line main *added* carried forward.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | yes | `CLAUDE.md` and `docs/` only |
| fpl-stats-review | **skipped** | no measurement made here. main's run is already reviewed under `reviews/2026-08-12-fb86c76/`; the question in *this* commit is whether the merge represents it faithfully, which is an audit question |
| fpl-code-review | **skipped** | main's Go arrives unmodified; this branch's only Go edit is a comment and a test constant |
| others | **skipped** | nothing in their triage rows moved |

## The invariant ran first, and it cleared the merge

The quantity a merge must not move is **main's content**. Tested mechanically:

- all 26 non-blank lines main added to CLAUDE.md are present in `docs/notes/recency-and-priors.md`
  — 25 byte-identical, 1 deliberately repathed (the `constants-and-sweeps` link, from a
  CLAUDE.md-relative path to a notes-relative one);
- `git diff origin/main HEAD` over `TODO.md`, `docs/notes/constants-and-sweeps.md`, `cmd/` and
  `reviews/` shows the only deletion anywhere is this branch's own canonical-title repointing.

**That check passed, and it was not the interesting part.** Every defect below is in the *summary*
of main's finding, not in its transport — which is the same split as the previous review on this
branch, and is becoming this reorganisation's characteristic failure.

## Findings applied

**1. The index bullet asserted the exact reading main retracted.** It said "neither survives
Holm". Main's block says the affirmative arms do not, and then two sentences later that the
**negative** arm does: `popEveryone` at **0.0385**, and that quoting 0.052 means quoting the
superseded `popFieldSound`. `constants-and-sweeps.md:1496` is blunter — *"the run's strongest
negative finding was being quoted off a retired population."* Verified by reading both. The
resident file was therefore telling every session that nothing resolved in either direction.
**Corrected**, with the population named and the instruction to never quote the retired one.

Worse, and the reason this ranked first: `TODO.md:373` carried the pre-correction reading too, so
after the merge **two of the three places stated the retracted version and only the notes carried
the correction** — the correction was the minority reading in the repo. `TODO.md` is now
annotated in place, marking that this is the same error one layer down: an item that already
says "an earlier version corrected the arms it wanted to disbelieve and exempted this one" was
itself exempting it again.

**2. "Unresolved, not favourable" omitted the one family that resolves favourably.** The signed
error over the twenty highest-predicted players inside the treated population is monotone,
unanimous from dose 0.25 up and Holm-clearing. `constants-and-sweeps.md:1511` records it
explicitly *"because a verdict of 'the affirmative case does not exist' cannot silently omit the
one family that resolves the other way in the same CSV."* The bullet performed exactly that
omission. **Corrected**, carrying the level-statistic qualification that is why it does not change
the verdict.

**3. A dangling referent that resolved to the wrong precedent.** The bullet kept "the precedent it
argues against was set on replayed points" but dropped the clause that names it. Its nearest
antecedent had become the position-quota precedent, which was not set on replayed points and is
not the one at issue. **Corrected** by naming it inline — that removing a bias is safe for an
argmax — and the "six independent seasons, not twelve draws" hedge was restored at the same time.

**4. The bias/variance standing rule now has a measured counter-instance.** The minutes channel of
`prior_half_life` is precisely the bias-removal candidate that rule would have picked, and it is
the arm least supported for shipping. **Marked in place**, with the reason it is not a refutation:
the rule was established on replayed points and the counter-instance is a prediction-ordering
statistic, so it declines to extend the rule rather than overturning it.

**5. Two adjacent bullets read as contradicting each other.** The thin-prior bullet says the
benefit is unmeasured; the new one says it has been measured. Both true — the −7 is replayed
points, the run is prediction ordering — and the whole force of the new finding rests on that
distinction. **Corrected** with one clause naming the metric.

**6. The new note's framing paragraph asserted a rule its own content contradicts.** This branch
wrote "minutes need recency and rates do not" as governing everything in
`recency-and-priors.md`; two hundred lines below, minutes-only is the negative arm and rates-only
the best-supported. The block's own reconciliation — *which season to trust* versus *when within a
season to weight* — is marked post-hoc by main, so it cannot be leaned on. **Marked**, with the
rule scoped to within-season recency and stated as untested across seasons.

**7. "Both channels are non-zero" is false under the experiment's own names.** main wrote "both
are non-zero" of two *statistics*; the bullet relabelled them "channels", which is the literal name
of two arms, one of which is `+0.00018` at t 0.11. **Corrected to "statistics"** — one word.

**8. A Go comment main wrote against the old structure.** `priorhalflife_test.go:25` cites "five
sections of CLAUDE.md carry a ⚠️". **Repointed** at the standing rule that now holds that material.

**9. Markdown and headroom.** After the corrections CLAUDE.md sat 53 bytes under the 52 KB budget,
which is not headroom. Two of the three new bullets were tightened without dropping a hedge,
landing at 53,059.

## Declined

**`TODO.md:434` and `cmd/priorblend/main.go`'s package comment** still carry the "only the blend
arm reaches back into the pre-2022-23 seasons — asymmetric by construction" claim that
`constants-and-sweeps.md:1543` retracts as *"wrong for half the grid"*, and
`priorhalflife_test.go:21` still says `OlderPriors` is populated in "one test" where the record
says no test populates it. All three are **main's**, pre-existing, and unrelated to this merge —
`fb86c76` corrected the notes and left the queue and the Go comments behind. Fixing them here
would widen a merge commit into someone else's cleanup and would obscure what this commit did.
Flagged for the next pass on that work.

## The pattern worth naming

Both reviews on this branch found the same shape: **the transport was clean and the summary was
not.** The mechanical check — enumerate, assert a destination, grep for a distinctive string — has
now twice confirmed no content was lost, and twice missed that a compressed bullet had become
stronger than its source. Six such bullets in the first review, four in this one.

The mechanism is specific and worth stating as a rule, because it will recur every time this file
is compressed: **the hedge is the cheapest thing to cut, and cutting it is invisible to every
check except reading the source beside the summary.** A hedge costs bytes and carries no figure,
so it looks like padding to a compression pass and looks like nothing at all to a diff. That is
why `notes_test.go`'s budget comment now records the six from last time by name, and why raising
that budget to give a claim its hedge back is the only legitimate reason to raise it.

## What could not be checked on this harness

Nothing here is measurable. main's underlying run is reviewed under `reviews/2026-08-12-fb86c76/`;
this review takes its figures as given and checks only that they are represented faithfully. The
points question for `prior_half_life` remains unmeasured and, per the record, expected to stay
unresolved — the arm is one assignment away from wired, which is a note for whoever wires it, not
a task this commit creates.
