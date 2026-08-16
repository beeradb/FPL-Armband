# Review — `7f33270..4b4a498`, branch `b5-season-list-parity`

Audit item **B5** (the hand-maintained lists duplicated between Go defaults and `config.json`), and
**B6**, which the review of B5 uncovered. Two review rounds, both **fpl-code-review**.

## Round 1 — the guard

Recorded in full at `reviews/2026-08-14-833cddc/review.md`. In short: the guard's scope was four
lists where **seven are duplicated and three already disagree**, and its mechanism test called
`json.Unmarshal` directly, so it pinned a property of the standard library — proved blind by a
probe. Both applied.

## Round 2 — the semantics change

The user then stated there is **exactly one `config.json` and no other users**, which removed the
only reason I had declined to fix the merge/replace asymmetry. So it was fixed, and re-reviewed.

**The change itself was confirmed correct and could not be broken.** Every substantive finding was
about the record and the guards around it.

### Finding 1 — `internal/config` was watched by NEITHER gate

**Verified**: `watchedPaths` was `internal/analysis`, `internal/backtest`, `config.json`;
`reviewWatchedPaths` added the agent, client, `stats`, `docs`, `CLAUDE.md`. Neither listed
`internal/config`.

`config.Load` decides the **effective** value of every field a config file omits — the second half
of "the shipped constants", which `config.json` alone does not capture. A commit deleting only
`if cfg.Weights.BlendRateK <= 0 { ... }` moves every scoring figure for any file omitting that key,
touches no watched path, and ships with **no snapshot and no review**.

Not hypothetical in shape: this branch's own backfill deletions tripped both gates only
*incidentally*, via a type change in `internal/analysis` that rode along.

`internal/snapshot` was unwatched by the review gate too — the **provenance machinery itself**,
where the `-note` discard fixed in `763e5e5` lived and owed no review.

**Applied**: both added. The gate immediately began naming `internal/config/config.go` and
`internal/snapshot/render.go`. Recorded as **B6**.

### Finding 2 — "one rule for all seven lists" was false

**Verified by measurement through `Load`**, not by reading: `tournament_absences: null` → **6**
(the default, resurrected, via a `== nil` backfill); `review_policy.rules: []` → **5**;
`minutes_weight_by_position` **still merges**. My blanket `CLAUDE.md` sentence — "the rule does not
extend to a LIST" — had live counter-examples one field away in the same function.

**Applied**: `tournament_absences`' backfill deleted (it *is* a hand-maintained enumeration, and
`null` resurrecting while `[]` emptied was two answers to one question). `Review.Rules` and
`MinutesWeightByPosition` deliberately left — fixed-arity structures where empty is meaningless
rather than a statement. The claim is narrowed everywhere it appears.

### Finding 3 — the guard's failure message taught the mechanism it no longer has

The map branch still read "CANNOT shorten the list by any route" — text a maintainer reads at the
exact moment of a summer re-derivation, and false in the **costly** direction: the maps have just
joined the class the slice branch describes ("a re-derivation done only in Go has NO EFFECT"). The
conclusion survives; the reason inverts. **Applied**, along with the file-level doc block and
`shippedConfig`'s rationale.

### Finding 4 — a missing guard for the scenario the change exists for

The strongest justification is not hand-editing. `update_competition_status` with `remove: true`
drops a club's last window and persists it — and the old `Load` merged the Go defaults back in,
**silently resurrecting the club the agent had just removed**. A correction that undoes itself one
run later is worse than one that never applied.

**Applied**: `TestRemovingAClubSurvivesASaveLoadCycle` pins the whole round trip including `Save`.
The record now leads with this rather than the weaker hand-editing case, and notes that removing a
club by editing **both** copies always worked — my commit message had over-claimed.

### Finding 5 — the snapshot was stamped `dirty`

So its commit stamp did not identify the code that produced its figures. My own fault: I `rm -rf`'d
a committed snapshot directory before retaking. **Applied**: the superseded snapshot is removed in
its own commit, and the retake happens on a clean tree — `stamp.dirty: false`.

More importantly, the operator note **presented the empirical null as the demonstration**. It is
not. That this branch moves no model figure rests on **mechanism**:
`TestDiagCleanSheetPoisson` builds no `Engine` at all; every other model diagnostic constructs an
**empty** `analysis.Congestion`, so the campaign maps never reach the replay path; and all eight
congestion penalties ship at 1.00. The null agrees but **could not have detected a difference**.
The note now says so.

## The attribution, checked three ways

Seven figures moved, all `clean_sheet_calibration`. Confirmed as the doubles guard already on main:

1. Diffed against the immediate predecessor `92318eb` — exactly those seven plus the stamps.
   (Diffing against `3c2af85` shows three extra `transfer_error` medians that moved earlier in the
   day and are not this branch's.)
2. **Pre-registered**: `3b6a698`'s commit message named 0.0698 and 0.2791 before this snapshot
   existed. A prediction matched, not a post-hoc story.
3. Mechanism, as above — the change cannot reach a model figure by construction.

## Declined

- **An explicit `"..._replace": true` flag.** A second way to do one thing; the flag is itself a new
  field needing a backfill whose default re-poses the same question; and the agent persistence path
  cannot set it, so the resurrection bug survives.
- **Keeping merge and adding a removal list.** Two mechanisms with an ordering question between
  them, no way to say "none", and the removal list is a third hand-maintained list that goes stale
  the same way. It also makes `Save`/`Load` non-idempotent.
- **Reconciling the three whitelisted divergences.** They are deliberate; the guard fixes their
  *status*, and the whitelist is itself checked.

## The cost being accepted, stated because it is real

**A Go-only re-derivation of the two campaign maps is now inert.** Add a club to
`DefaultEuropeanCampaigns`, forget `config.json`, and the club does not exist at runtime — merge
used to rescue that. The only thing between it and a silent mis-score is
`TestTheShippedConfigsHandMaintainedListsMatchTheGoDefaults`, which therefore carries more weight
than it did. Acceptable: the guard exists, it is falsified, and the maps are display-only today at
1.00 penalties.

## What could not be checked

- **Whether the three divergences are correct as football.** `DefaultCongestion`'s own comment says
  `long_haul_regions` contradicts its documentation — nine nations the description covers are
  absent. Unmeasured.
- **Whether duplicated pairs exist outside `Weights`, `Congestion` and `RoleRisk`.** The sweep
  covered those three structs; seven is a floor.
- **Whether other guards stop one directory short**, as both `watchedPaths` and the B5 scope did.
  Two instances of that shape in one branch suggests looking, and nobody has.

## Falsification

Every guard on this branch was falsified before being trusted: five perturbations for the parity
check (dropped player, deleted club, staled date, reconciled whitelist entry, `Load` probe), the
`-note` regression test run against the unfixed renderer, and the mechanism test against a
reinstated merge.
