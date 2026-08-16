# A retraction, and the reviewer finding that caused it

Covers `d93d476..84e49b2`. No reviewer was dispatched for this range — it corrects a claim
the previous record introduced, and the correction came from the user rather than from an
agent. Recording that as the provenance rather than implying an audit.

## What was retracted

The stats review said picking chip weeks from a season's doubles is hindsight. I applied it
to `CLAUDE.md`, `docs/notes/chips.md` and the previous review record. It is wrong twice.

**First, finding a double is not hindsight.** Reschedulings are rumoured for months and
confirmed weeks ahead. The package under review already said so: `sightLags = {2, 4, 6}`
exists precisely because the reveal is early, not zero.

**Second — and this is the deeper one — knowing which double is *biggest* is not hindsight
either.** I retreated to that position and it did not survive. The manager reports knowing
the season's biggest double in advance in **3 years of 3**, because the cup and European
calendar makes it predictable long before a fixture is formally moved.

## What that does to the record

`sightedWeeks` states as fact: *the constraint a short lag imposes is not that you cannot
find a double, it is that you cannot tell a double from the biggest double of the season.*
**That is an assumption, it was never measured, and the only direct evidence contradicts
it.** It is the premise the entire sight-lag ladder rests on.

Flagged in place rather than deleted — the arms are still worth running, and **no measured
figure moves**. What changes is which row answers the question:

| before | after |
|---|---|
| `fullSight` is the optimistic arm; the lag arms are the realistic ones | **`fullSight` is the realistic arm** — it is what a manager actually knows |
| its −5.4 is an upper bound on calendar anchoring | its **−5.4, clustered t −0.66**, is the *estimate* of what calendar anchoring is worth |
| the lag arms correct for realism | the lag arms bound **pessimism**, pricing a manager with less information than anyone has |

The data limitation that survives is narrower than the retracted claim: the archive keeps
only the **final** `event` assignment, so the replay cannot model a manager who knows
*less*. Which the evidence above says is the case nobody needs.

## The process failure, stated plainly

The review-gate skill says a reviewer's report is a set of proposals and several will be
wrong. I verified the review's two code findings — both real, both fixed — and did not
verify this one, even though **the file it contradicted was open in the same change**.

The pattern is specific enough to name: the finding arrived with a mechanism attached ("the
published fixture list has no doubles"), the mechanism was true, and I let a true mechanism
carry a false conclusion. Recorded in memory as `doubles-are-known-in-advance`.

## Declined

**The lag arms were not deleted.** They are the honest way to ask what a manager with a
short view would do, and the retraction is about which arm to *quote*, not about whether the
ladder is worth having.

**No re-measurement was run.** Nothing in the tables changes — the arms already exist and
their numbers stand. This is a reinterpretation of an existing measurement, which costs a
comment and not a sweep.
