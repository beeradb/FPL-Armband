# Review — making the planner editable, and the controls that did not change the model

**Range reviewed:** `origin/main..HEAD` on `make-the-planner-editable` — `236c7c4` through the
commit this record rides in.

⚠️ **CORRECTION, 2026-08-19.** This said "six commits". It was seven at the time of writing
and nine as merged; the branch merged at `99ae9f2`, and `2d6caf1` is the last commit this
record covers. A reader checking the range against `git log` would have found a different
number, which is exactly the sort of small wrongness that makes a record stop being trusted.

**What it is.** The planner shipped read-only in the previous branch. Four bugs came back
from use — a remove button that did nothing, a swap panel that offered nobody, a stale
opening squad, and no persistence through reloads — and they turned out to be one bug and
three consequences of it. The one bug is persistence. The remove button *looked* broken
because a dismissal filtered a JavaScript array: the row vanished, the model went on applying
the correction, and the squad did not move. Nothing was saved because nothing could be.

This branch adds the session store (`cmd/armband/session.go`), a replacement picker built by
a visual designer against the shipped design assets, a seeded opening squad with an Optimize
button, and a layout-golden suite rebuilt on committed `viewmodel.State` fixtures instead of
model output.

**Three rounds of review ran, and the third is why this record is long.** The first two
covered the feature. The third asked one question — *does this control change the model, or
only the page?* — and the answer was no often enough that it became the theme.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| `fpl-code-review` | yes, three rounds | the branch's whole claim is that the controls now bind, which is the class this repository has paid for most often |
| `fpl-security-review` | yes | a browser-driven write path over the planner's state, a cookie store, and a `-persist` mode that edits `config.json` |
| `fpl-docs-review` | yes | a design note was written to the research store, and it carried a figure that had already been withdrawn once |
| `fpl-stats-review` | yes, round two | a points figure was quoted for the variety mechanism. It was refuted; see finding 12 |
| `fpl-findings-audit` | no | no verdict in `AGENTS.md` was leaned on. Two entries were ADDED (below), which is a different act — they are process lessons pinned by tests in this branch, not measurements |
| `fpl-run-review` | no | no live run wrote config |
| `fpl-season-maintenance` | no | none of the four hand-maintained lists moved |

**A reviewer's report is a set of proposals.** Every finding below was reproduced before it
was acted on. Two were reproduced and found narrower than reported; those are recorded as
applied-with-a-correction rather than accepted wholesale. One is declined.

## Findings, ranked by how misleading the state was

### 1. `serve -persist` discarded the entire session while the page said it had saved to `config.json` — APPLIED

Found independently by `fpl-security-review` (its finding 1) and `fpl-code-review` (its
finding 1), which is worth noting: two reviewers with different briefs converged on it.

`effectiveCfgFrom` returned `*s.cfg` under `-persist`, and `applyTo` is the only thing that
turns the session's locks, blocks, dismissals and chip placements into config the build
reads. Meanwhile `saveSession` never wrote a file — the only `config.Save` on the serve path
belongs to the form-POST route, and nothing in the shipped client posts to it.

So under the mode the README advertises as the durable one: the badge lit, the document
reported `store: "config"`, the player stayed in the squad, and `config.json` was untouched.
The security reviewer measured it:

```
persist=false  store="session"  blocked=[489639]  still in the fifteen: false  cfg.Exclude=1
persist=true   store="config"   blocked=[489639]  still in the fifteen: true   cfg.Exclude=1
```

This is the branch's own defect in the mode with the strongest claim to be durable, and it
was mine — I wrote the early return, in the previous round, while fixing a *different*
`-persist` bug.

**Applied**, and the mode's promise made true rather than the claim removed: the session is
applied over the config in both modes, and under `-persist` `persistCorrections` writes lock
and exclude through to `config.json` and then clears them from the session, so exactly one
store holds each. A dismissal under `-persist` is a deletion from the file rather than a
suppression of it — otherwise `-persist` would be a mode where a reader can add a correction
and never remove one.

**Tests:** `TestUnderPersistABlockReachesTheFileAndTheSquad` asserts both halves (it binds the
build AND it is in the file), `TestUnderPersistTheCorrectionLivesInExactlyOneStore`,
`TestUnderPersistADismissalRemovesTheOverrideFromTheFile`.

### 2. `GET /api/state` was unauthenticated, cross-origin reachable, and mutating — APPLIED

The route minted a seed and stored it. Any page open in the reader's browser could `fetch`
it; because `SameSite=Strict` withholds the real cookie, the server saw an empty session,
minted a fresh seed and answered `Set-Cookie` — replacing the reader's fifteen, arrangement,
armband, corrections and chip placements with nothing, in an HttpOnly cookie the page cannot
read back to warn them. The attacker learns nothing and destroys everything.

**Applied**: only an authed request may mint. An unauthed caller still gets a document, built
on a zero seed — the straight optimum, which is a fine answer to "what does the model think"
and is not a store.

**Tests:** `TestAnUnauthedReadDoesNotOverwriteTheReadersSession`, and
`TestAnAuthedReadStillMintsASeed` — without the second, the first passes on a server that
never mints, which would silently disable the varied opening squad.

### 3. The transfer gate was decided twice, and the equivalence test pinned the constant rather than the rule — APPLIED

Two findings, in sequence, and the second is the more instructive.

**First:** Go compared raw floats (`gate > 0 && d >= gate`) while the client compared at the
precision the row prints. A delta of 0.3999 against a 0.40 gate printed `+0.40`, took a
below-the-gate badge from the server, and was simultaneously called worth making by the
picker. The client's rounding was right and was in the wrong layer. Moved to
`present.ClearsGate`; the client keeps one named mirror, because the picker's delta is
against the player being replaced and the server sends deltas against the weakest starter.
Pinned by `TestTheGateIsDecidedTheSameWayInBothLanguages`, which runs one table through both
languages in a real browser against the shipped `app.js`.

**Second, from `fpl-code-review`:** that pin was passing by luck. `math.Round(v*100)/100` and
`toFixed(2)` are not the same function — `toFixed` is specified on the exact value of the
double, while `math.Round` rounds the float *product*, which can land on a `.5` boundary the
value itself is below. The reviewer enumerated it: they disagree about 219,300 times over the
tie-adjacent doubles, always in the same direction. At the shipped gate of 0.40 they agree,
which is why 14 probes found nothing. Set `min_gain` to 0.30 and
`ClearsGate(0.295, 0.30)` is `true` in Go and `false` in the client, with no test failing.

**Applied**: the client computes `Math.round(d*100)/100`, which is bit-identical to Go for
every value that can reach a verdict (they differ only on negative halves, and a gate of zero
or less clears nothing). Fourteen separating cases added to the table. I verified the
enlarged table fails on `toFixed` before trusting it — it names ten disagreements.

**Recorded because it generalises**: an equivalence test whose table contains no separating
case pins the shipped constant, not the rule. `AGENTS.md`'s "one quantity, two
implementations" entry now says so, along with the fact that both existing scans are Go-only
and cannot see a copy in the client at all.

### 4. Chip placement was stored, echoed, and never applied — APPLIED

`engine.Chips` is assigned once at process start. A placement reached the cookie and the
document and stopped there, beside copy promising the projection would re-run under that
chip's rules. Now `e.Chips` comes from the request's config, and `session.chipsInto` maps
gameweek→key into the schedule. `TestPlacingAChipChangesTheProjection`.

Three defects were found underneath it:

- **A planned wildcard truncated the shared engine's horizon permanently.** `ApplyChipPlan`
  shortens `Weights.Horizon` — right for the squad being built, and permanent on a server
  holding one engine for every reader. A wildcard at GW3 dropped the engine to two gameweeks
  and left it there, for that reader after they removed the chip and for everyone else in the
  process. Latent while the plan came only from config; the feature that let a reader place a
  chip made it reachable. Restored by `defer`.
  `TestAPlannedWildcardDoesNotShortenTheNextReadersHorizon`.
- **Placements only ever reached the FIRST set**, while FPL grants a second from GW20 and this
  repo models both. `containingSet`'s own comment already named this failure as distinct from
  an impossible week. Now routed by `ChipSchedule.Place`. `TestAPlacementGoesInTheSetWhoseWindowHoldsIt`.
- **`chipsInto` ranged a map and assigned**, so two placements of one chip resolved by Go's
  iteration order — the plan flipped between them on every request and the reader was served a
  different fifteen on each reload with nothing having changed. Now ranged in gameweek order.

### 5. The lock and block icons on a card never saved — APPLIED

They mutated a JavaScript Set and called `renderAll()`. The player sheet's versions of the
same two actions saved correctly, so the behaviour depended on which surface the reader used.
This is the original "remove button does nothing" bug, still present on the control nearest
the reader. One implementation now (`toggleCorrection`), two callers.

**Test:** `TestEveryControlOnACardReachesTheServer` — a browser test that asserts a **request**,
not a repaint, because a control that repaints and sends nothing is exactly what is being
pinned and a test satisfied by the badge passes on the broken code. Verified failing on the
old JavaScript.

⚠️ The docs review is right that "only a browser could catch this" is too strong: a source
scan asserting the icon handler calls the save path would have caught it, and this repo's
standing remedy for "the code does not reach the thing" is a scan
(`TestTheHitCeilingIsReadByTheFundedPairBranch`). The true claim is narrower — no Go test that
exercises the *server* could catch it. Corrected in the design note.

### 6. `validateSession` guarded writes only — APPLIED

Cookies are not scoped by port, so anything the browser loads from another service on
`127.0.0.1` can set this one's cookie. A fifteen the write path refuses — the same player
fifteen times — was rebuilt on every read, producing an empty eleven and a zero captain that
the client throws on, in an HttpOnly cookie the page cannot clear. The reader's only escape
was devtools. `readValidSession` now applies the write path's rule on the way out and
discards rather than repairs. `TestAStoredSessionThatWouldBeRefusedIsDiscarded`.

Thin exploit path under this threat model, as the reviewer said. Applied because it is cheap
and collapses a class rather than an instance.

### 7. A chip could be placed where the game refuses it — APPLIED

Found by me, not by a reviewer, while checking the reviewer's second-chip-set concern. The
rail only draws what `PlayableChips` returns, so the page cannot produce an illegal
placement; the endpoint could, and the result was silent and durable. `validateSession` now
checks each placement against the same function the rail is drawn from.
`TestAChipCannotBePlacedWhereTheGameRefusesIt`.

### 8. `squadFromCodes` was a second implementation of the money — APPLIED

`Optimize` counts in integer tenths; the reload path summed fifteen float prices, leaving
~1e-14 of dust. The client compares affordability **raw**, so with a £0.5m bank, a £4.5m
player out and a £5.0m player in, the integer path gives exactly 0 and the float path gives
−8.9e-16 — the same target reachable from a fresh build and silently missing after a reload.
The reload is the *common* path, because a saved fifteen skips the optimiser. Now counted in
tenths, and `TotalCost` with it. (The first version of this fix double-counted `TotalCost`
and was caught by `TestTheBankIsNotZeroOnAReload`.)

### 9. `violatesRoster` checked `StartIDs` against the fifteen — APPLIED

A must-start lock means the player is in the **XI**, which is why `Optimize` threads
`mustStart` into `bestXIWith`. Checking membership of the fifteen let a must-start player sit
on the reader's bench with the override silently not applied. Reachable only under `-persist`,
so it compounded with finding 1.

### 10. `applyAction` added to `Lock` without pruning `Dismissed` — APPLIED

`settled()` strips `Lock`/`Exclude` of anything dismissed, and nothing else prunes
`Dismissed`. So a lock set through the form path was stored and then deleted on the next
read, forever, with no error anywhere. The client's `toggleCorrection` already handled this;
the form path did not. Dormant today — nothing posts to `/action` — and it becomes live the
moment the JavaScript-free fallback is wired up, which is named as the next piece of work.

### 11. `save()` dropped clicks, then could poison its own chain — APPLIED

`if(saving) return` discarded the mutation as well as the request. Replaced with a promise
chain, which the reviewer then checked properly and found one gap: a **synchronous throw
inside a mutate** rejects the chain, and every later save on the page is skipped for the
lifetime of the document — silently, with nothing but an unhandled rejection. `saveArrangement`
calls `syncArrangement()` inside its mutate, so this is not purely theoretical. A `.catch()`
on the chain makes it structurally impossible.

The reviewer's other three answers were that the chain does not grow unboundedly, does not
lose a mutation, and does not deadlock on a rejected fetch. Confirmed and unchanged.

### 12. The variety figure was refuted, and then the replacement was wrong too — APPLIED, TWICE

**Round two:** the note and a commit message claimed "0.88 pts/gw mean, 1.02 worst, over five
seeds", extrapolated to "33 to 39 a season", and compared that against the smallest effect the
harness can detect. `fpl-stats-review` refuted all three. `buildVariedSquad` draws two
exclusions from twelve candidates, so the sample space is C(12,2) = 66 and enumerable in
about 75 seconds. The census: min −0.097, mean 0.526, median 0.506, max 1.448. **Five seeds
over-stated the typical cost and under-stated the ceiling at once**, and produced only two
distinct values, so the effective sample was about two rather than five.

**Round three:** the corrected note said `median 0.513`. `fpl-docs-review` checked it against
the test's own output — it is 0.506. 0.513309 is the 34th of 66 sorted values, the upper of
the two middle ones, read off a sorted list. I re-ran the census to confirm. Every other
figure reproduced exactly.

Also applied from that review: the data state is now named (capture `2026-08-19T1348Z`, GW1,
horizon **5** — while `serve` defaults to `-horizon 4`, so the census bounds the mechanism at
one gameweek longer than the page it is about); the negative minimum is glossed as the local
search's own slack rather than a benefit of excluding anyone; "an optimum" is now "a reference
squad", because `Optimize` is a heuristic and those four pairs are the measure of how
heuristic; the general lesson gained its two load-bearing clauses ("enumerate", and
"deterministic given the draw", and "over the draw"); and the argmax hypothesis now says why
*mild* is doing real work — the draw never touches the captain, who carries the largest
selection inflation in the fifteen, and the constrained squad is itself an argmax.

⚠️ **The withdrawn figure is published in commit `3d97270`'s message and cannot be corrected
there.** This paragraph is the retraction. It may carry the census numbers because
`TestTheVarietyCostIsBounded` computes them.

**It also called a projection "measured".** `3d97270`'s message reads "Measured rather than
asserted: variety costs 0.88 pts/gw on average…". That word is the part most likely to be
repeated, and it was wrong independently of the arithmetic: the figure is the model's own
projected difference at one instant, on one capture, at one gameweek, under one horizon.
Nothing was measured. A projected gap and a measured points cost are different quantities,
and this record is the only place in the repository where that sentence is contradicted.

### 13. `PlayableChips` was thin-feed fragile, and offered chips it could not play — APPLIED

A chip with a start and no stop was dropped from every gameweek (the symptom is a chip row
that is simply not there); a stop of zero now reads as an open window. And an unknown chip was
*offered*, then hit a `switch` with no matching case and did nothing — the branch's own defect
class. Unknown chips are no longer offered.

### 14. `tokenOK("")` was true when the server had no token — APPLIED

Added 2026-08-19: the commit message listed this and the record did not, which is an omission
rather than a policy — findings I found myself are recorded elsewhere here.
`subtle.ConstantTimeCompare` on two zero-length slices returns 1, so an empty token opened
every write route, while `authed` guarded the same case explicitly. The two disagreed about
what an unconfigured server means. Unreachable today — `cmdServe` returns on a token error and
never builds one — which is precisely why it is pinned rather than relied upon.
`TestAServerWithNoTokenGrantsNothing` asserts both halves and that `authed` and `tokenOK` now
agree.

### 15. One `Dismissed` entry means two things — DECLINED, with the reason

`Dismissed[C]` suppresses every config override on code C — exclude, lock and minutes — and
also deletes the reader's own session lock or block on C. A reader with a config exclusion and
their own session lock on the same player, pressing ✕ on the lock row, clears both.

**Declined for this branch.** Fixing it means keying dismissals by `(kind, code)`, which
changes the cookie's shape and therefore `sessionVersion`, and the state it protects against
is one the UI does not currently produce. It is recorded here rather than in the code, because
a comment describing a limitation nobody can reach reads as a warning about something that
happens.

## Corrections to the reviewers

Two reports were narrower than they read, and saying so is the point of this section.

**`S.blocks` is not reachable, after finding 1.** `fpl-code-review` argued that three of the
four client sites reading `S.blocks` *are* reachable, and that `-persist` is what reaches them
— correct against the code as it stood, because `-persist` discarded the session and left a
blocked player in the squad. With finding 1 applied, `-persist` writes the exclusion to config
and clears it from the session, so `Session.Blocked` is empty and the player is out of the
squad. The sites are unreachable in both modes. Left in place.

**"An empty rail beats offering a chip that would be refused" needs no stderr line.** The
reviewer suggested warning when `boot.Chips` is empty. Declined: finding 13 makes the only
realistic thin-feed case (an open-ended window) work correctly, and an entirely empty `chips`
array is a payload this code has never seen.

## What could not be checked on this harness

- **The process claims.** "Each new test was watched failing on the defect before it passed"
  is true and has no artefact. `fpl-docs-review` flagged it as asserted-not-verified and is
  right to. Where it mattered most — the two browser tests and the gate table — the failure
  output is quoted in the commit messages, which is the closest thing to evidence available.
- **`-persist` end to end against a real `config.json` on a live feed.** The new tests use a
  temp-dir config and the committed capture. The write path is `config.Save`, which is
  covered elsewhere, but the combination is not exercised against live data.
- **Whether the picker's client-side delta matters in practice.** The gate *rule* is now
  pinned across both languages; the delta itself (`c.xp - o.xp`) is still computed in the
  client, because the server sends deltas against the weakest starter and there is no endpoint
  answering "candidates against THIS outgoing player". Filed, not measured.
- **The census at the horizon the planner actually serves.** It runs at the configured 5;
  `serve` defaults to 4. Whether the bound differs at 4 is unmeasured.

## Additions to `AGENTS.md`

Two, both process lessons pinned by tests in this branch rather than measurements:

- **"A per-request mutation of a shared engine outlives the request"**, in *Things that have
  already bitten*, with `TestAPlannedWildcardDoesNotShortenTheNextReadersHorizon`. It is the
  one defect here whose class recurs the moment anything else is served over HTTP, and a CLI
  that exits cannot find it.
- **A clause on the "one quantity, two implementations" rule**: both existing scans are
  Go-only and cannot see a copy in the client, and where deleting the copy is not available
  the pin needs the separating cases or it fixes the constant rather than the rule.

The file is now 44,982 bytes against a 44 KB budget with 74 bytes free. Nothing was dropped to
fit.

## The accuracy snapshot

`stats/snapshots/2026-08-19-7256e67`, regenerated from the documented recipe after the last
code change. **Only `stamp.commit` moved.** That is the expected result rather than a missing
one: `internal/analysis` did change — `PlayableChips` reads the feed's windows and
`ChipSchedule` gained `Place` — but neither is on the scoring path, and a snapshot records
WHEN it was taken rather than that the model behaved differently. The R inference ran; no
cells file was supplied, so the replay half is the previous series' and is unchanged.

## What is owed

- `carry-the-selling-price-into-the-planner` — tier 1, filed in the research store at the
  user's request. The planner prices a sale at the LISTED price; FPL pays purchase price plus
  half of any rise. The error is one-directional and only ever over-states the budget.

  ⚠️ **CORRECTION, 2026-08-19, after this record was committed.** This paragraph originally
  read "The rule is already implemented in `analysis.SquadState.sellPrice` and the planner
  simply does not reach it." **That is false**, and it is corrected here rather than deleted
  because it is the kind of claim a later reader would build on. `SquadState.sellPrice`
  (`internal/analysis/swaps.go:287`) is a map lookup into `SquadState.Sell` with a fallback to
  market price — not the rule. The rule is implemented **twice**, at
  `internal/fpl/squadprices.go:118` and `internal/backtest/wallet.go:77`, both spelling
  `paid + (market-paid)/2`, and nothing pins them to each other. `SquadState.Sell` is filled
  from `fpl.SquadPrices` only when `entry_id` is set, so without one the CLI transfer search
  has the same defect this item is filed about. The accurate framing is not "the planner fails
  to reach a capability" but "the capability needs entry data the planner does not have".
  Found by `fpl-docs-review`; I had asserted it from a name rather than from the body.
- The transfer confirm flow is stubbed: no hit cost, no deadline check.
- Three client surfaces still compute model quantities — the formations rail, the two captain
  fallbacks, and the per-pound ratio — named in `internal/webui`'s package comment.
- A server endpoint for per-outgoing-player deltas, which would remove the picker's last
  computation.

**A green gate is not a correct change.** Every condition in `merge-gate` is a process fact.
What this branch rests on is that fourteen defects were found across three rounds, each has a
test, and every new test was watched failing on its defect first.
