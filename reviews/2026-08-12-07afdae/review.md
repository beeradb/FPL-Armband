# Review: three rules about constants

**Commit range**: `bc2a15b..07afdae`, plus the corrections applied on top in response to this
review.

**What it does**: records three rules that survived a review of every constant in the project —
an epistemics rule and a wiring rule into CLAUDE.md's standing rules, a triage rule at the head of
TODO.md's "Calibration debt" — and raises the CLAUDE.md budget from 52 to 56 KB while changing the
principle it is set on.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | yes | `CLAUDE.md` and `TODO.md` only |
| fpl-stats-review | **skipped, but now owed** — see below | nothing was measured here. The triage rule's *consequences* turn on threshold comparisons, and two reclassifications need that judgement before the items are struck |
| fpl-code-review | **skipped** | the only Go change is a test constant and its comment |
| others | **skipped** | nothing in their triage rows moved |

## The headline: the triage rule does more than I claimed

I wrote in TODO.md that applying the rule makes the list "shorter rather than longer". That was an
assertion I had not enumerated, and I asked for it to be tested. It is right in direction and
**wrong in degree**: enumerated across all eight open items under Calibration debt, **not one is
debt under the rule as stated** — every one has either its value or its consequence measured.

That is a finding about the section's own preamble as much as about the rule. It read *"the list
below is only the part with no evidence"*, three lines above the new rule, and that has been false
of every item beneath it for some time. **Corrected**, and stated as "nearly empties" rather than
"gets shorter", with the note that two of the reclassifications turn on a threshold comparison and
want statistics review before the items are actually struck.

The reclassifications, recorded so the next pass does not re-derive them: the defcon opponent
effect (value measured at 1.069-0.901, consequence ~±2 a season) and the bench-slot formation
legality item (already answered in `optimiser-and-squad.md`, 0 of 24 uncovered) are **done, not
owed**. The congestion item is **done as shipped** — all eight constants at 1.0, consequence
pinned at zero — and what remains of it is a *build* proposal, which is a different question.
Two items are **not constants at all** (neighbourhood averaging, the archive team-strength
snapshot date) and belong in other sections. Two are **gap cases** — value measured, consequence
not — which the rule as written does not classify: `prior_half_life` and the busy-keeper
over-rating.

## Findings applied

**1. The wiring rule named the wrong package, and the repo already knew.** I recorded the rule as
"check a setting reaches `internal/analysis`". `cmd/fplagent/priorhalflife_test.go:66` records
that an earlier version of that guard *scanned exactly that tree*, that code review caught it by
mutation, and that **the route which would make the warning false does not pass through
`internal/analysis` at all** — `internal/analysis` consumes a `PriorSeason` index built in
`internal/backtest`, and the seam that would ship the feature is the `backtest.SimConfig` literal
in `cmd/fplagent/backtest.go`. So the rule as recorded would have reintroduced into the resident
file the precise error its own guard was corrected for, and a reader following it would get a
green scan while the warning went on telling every sweeper a wired knob was inert.

**Corrected** to "read on the path you are about to score", with the package trap recorded
explicitly: **naming the consumer is the check; naming a package is not.** This is the most
valuable thing in the commit and it is the opposite of what the commit originally said.

Note the shape: I had already caught myself asserting an unverified mechanism here and corrected
it before committing — and the corrected version was *still* wrong, in a way only the source could
settle. Checking one layer is not checking.

**2. The triage rule dropped the clause that made its own precedent valid.** I wrote "bounded is
done, not owed". The gate closure it generalises has three ingredients, not one: the bound (106),
*the comparison's own threshold* (94), and the resulting 89%-of-perfect-hindsight requirement. As
written the rule closes anything with a bound, and the record supplies the counter-example
immediately: **the armband is bounded at 210 and captaincy is emphatically not closed**, because
the span of captaincy *rules* inside that bound is ~28 a season and 28 is what a change competes
for. **Corrected** to "bounded below what the comparison that would test it can resolve", with the
armband named as the reason a large bound closes nothing by itself.

**3. The item the rule will be applied to first was held open by a stale comparison.** The
"filters and durations" item ends *"It remains larger than the transfer gate (bounded at 106 and
closed), which is the comparison that justifies spending effort here rather than there."* The
family is bounded at ≈73 held. 73 < 106, so it is *smaller* — and the comparison is `HOLD` against
`POLICY`, whose thresholds differ (33 against 70), so it does not stand even corrected. The
sentence is a survivor of the `+183` retraction two lines above it: the number was fixed and the
inference it supported was not. **Corrected**, and the item marked done-not-owed subject to the
statistics review.

With a carve-out that must not be lost: the item's premise, "every one of these is a guess about
who will play", does not cover all seven constants it bundles. `BudgetValue`'s £1.0m clamp is a
money-valuation slope and `MinMinutes` 600 is a pool filter; **neither is bounded by a lineups
oracle**, so both stay debt when the rest of the family closes.

**4. "Two open questions" was a count with nothing behind it.** I could not name the two and
deliberately did not guess — but the committed text did not say so, and read as though someone had
counted. **Marked** as carried from the proposal rather than verified.

**5. Rules 1 and 2 gave opposite instructions using the same word.** Rule 1: an unresolvable
question is still *owed* an answer. Rule 2: a bounded constant is *not owed* work. They live in
different files with nothing reconciling them. **Corrected** with a clause saying rule 1 is about
the question and rule 2 about the queue.

**6. Rule 1's scope was broader than its subject.** "Only the second describes anything here" —
but the record does contain something with no truth value in the required sense: the choice of
objective. Whether to maximise points or percentile has no right answer independent of what the
manager is chasing. **Scoped to constants**, with the objective carve-out stated.

**7. Rule 1 grazed the mechanism closures.** A reader combining "still owed an answer if the
instrument improves" with "closed on mechanism, unresolved on points" could conclude the
closed-lines block reopens whenever the grid widens — and it just widened. **Clause added**: a
line closed on mechanism is answered, and a wider grid re-prices a bound rather than licensing a
re-run of a mechanism argument.

**8. Two TODO items were stale against the code.** `DomesticCupPenalty` 0.97 and `LongHaulPenalty`
0.86 are quoted as shipping values; both are 1.0, pinned. This is **the same stale pair `f62f8bb`
corrected in CLAUDE.md** — the fix did not chase it into TODO.md, which is the ordinary way a
correction goes half-applied. And the "six of eight ship at 1.0" count is now eight of eight.
Both **corrected**.

**9. An open experiment was justified by an invented shape.** The fixture item proposes work on
the grounds that the attacking ladder "has reversed — now monotone decreasing, best at zero".
CLAUDE.md's contamination table records that the zero-penalty bug **invented that monotone
decline**. **Retracted in place**, leaving the standing verdict that the apparatus is unresolvable
at current scoring — a closed line, not an experiment.

**10. The budget comment's discriminator was wrong, and measurably so.** It claimed the guard
catches category regrowth because that arrives at "2-5 KB a paste" against "tens of bytes" for
honest edits. Measured from git, the file went 96 KB → 161 KB across ~50 commits, a bullet at a
time, at ordinary edit sizes — the scoring-model index alone reached 29 KB for 20 bullets. A
reader applying the size test would have waved through every commit that produced the 65 KB.
**Corrected**: the discriminator is *where the text lands*, never how much arrives at once. A
third category was also missing — retraction narrative, 12,952 bytes of it, which is neither index
duplication nor a sweep table and is exactly what this branch ruled out of the file.

## Declined

**Relocating the two non-constants** (neighbourhood averaging → "Cutting the replay's noise"; the
archive team-strength snapshot date → "Archive defects found by audit"). Correct, but moving items
between TODO sections in a commit about recording rules would obscure both. Flagged for the next
pass on that list.

**Two `[x]` items missing their `~~strikethrough~~`.** Cosmetic. Real, and it makes two done items
scan as open in a list about to be triaged, but not worth widening this commit.

**Striking the reclassified items.** The rule now says they are done, not owed; actually removing
them is a decision that turns on threshold comparisons and belongs to `fpl-stats-review`. The
preamble says so rather than pre-empting it.

## What could not be checked on this harness

Nothing here is measurable, and nothing here needs a run. Every finding is settled by reading the
source or the record. The two gap cases the triage rule does not classify — value measured,
consequence not — are a genuine hole in the rule rather than an oversight in applying it, and
whether they are debt is a judgement the rule as given does not make.

## The pattern, third instance

All three reviews on this branch found the same thing: **the transport was clean and the summary
was not.** Here the "source" was three rules given in one line each, and expanding them introduced
a wrong package name, a dropped threshold clause, an unsourced count, a scope that overreached and
a discriminator that measurement refutes.

The mechanism is the same one `notes_test.go` now records: what gets lost in compression is the
qualifying clause, because it costs bytes and carries no figure. What this round adds is that the
same thing happens in *expansion* — filling out a one-line rule invents the qualifications it does
not have, and they are wrong at exactly the rate you would expect of something nobody checked.
Both directions need the source read beside the result.
