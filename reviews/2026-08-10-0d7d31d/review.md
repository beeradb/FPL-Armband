# Review — the club-form blend, replayed; and what "form" is made of

**Commit range reviewed:** `ec608b8..0d7d31d`. Two substantive commits: `774a2b4` builds the blend
and measures it, `0d7d31d` is the snapshot showing it moves nothing when off.

## Reviewers dispatched

**None, and this is the second consecutive record saying so — which is itself worth flagging.**

The triage table sends `internal/analysis` changes to fpl-code-review, fpl-stats-review and
fpl-findings-audit. The argument for skipping is that the change is inert by construction and the
inertness is *measured*, not asserted: the regenerated snapshot moves exactly one figure, the commit
stamp. What a code reviewer would be checking — that the shipped path does not move — is answered
more strongly by that diff than by reading the patch.

The argument against skipping is that this is the pass where a **between-club ordering change** was
built, and the record's own rule is that such a change reorders the entire pool because nothing
forces you to own players from a particular club. That it currently ships off does not make the
code correct; it makes the code unexercised.

**So: the null result is well evidenced and the machinery is not independently reviewed.** If the
blend is ever switched on, it owes a full review before that happens, and this paragraph is the
reason. Two consecutive self-reviewed records is the point at which the gate stops being a gate.

## Findings

### 1. The blend does not resolve, on either metric. RECORDED

Per gameweek played, 24 cells, season-clustered by `stats/sweep_inference.R`:

| weight | `HOLD` | SE (CR2) | t | Holm p | `POLICY` |
|---|---|---|---|---|---|
| 0.25 | −0.216 | 0.208 | −1.04 | 1.00 | −0.546 |
| 0.50 | +0.620 | 0.905 | 0.69 | 1.00 | −0.115 |
| 0.75 | +0.540 | 0.745 | 0.72 | 1.00 | −0.559 |

Best case +24 a season on `HOLD` against an SE of 34. `POLICY` flat to negative throughout, which is
the channel a between-club shift should move hardest — that is the more informative half of the
table and it points the wrong way.

**The ordering agrees across two instruments**, prediction and replay both ranking 0.50, 0.75, 0.25.
On this harness an ordering is cheaper to establish than a gap. ⚠️ But the pre-registration in
`teamformsweep_test.go` **mis-transcribed the prediction table**, stating an ordering it does not
contain. Corrected in place, and the honest reading is that 0.25-is-worst was pre-registered while
the 0.50-against-0.75 pair was stated correctly only afterwards.

### 2. "The model under-reacts to fixtures at club level" — RETRACTED

It rested on a +0.334 correlation between the club ratio and fixture difficulty. The club-level
modelled figure is `XG90 x XGScale x ExpectedMinutes / 90` and the attacking multiplier is applied
**downstream** in `fixtureSensitiveAt`, so the modelled side carries no fixture term while the
realised side was earned against real opponents. The correlation is expected before any model error
exists.

The Gap 3 *spread* survives — 0.248 against 0.243 with both sides neutralised — so only the fixture
reading was wrong, not the headline. Recorded in CLAUDE.md with the retraction rather than quietly
dropped, because it was stated to a reader as a finding.

### 3. Float determinism, caught by the point-in-time test. FIXED

`newTeamFormIndex` summed expected goals by ranging a map, so club totals differed in their last bits
between runs. The comment justifying it said "addition commutes" — true in arithmetic, false in
binary floating point, and CLAUDE.md attaches that exact caveat to that exact claim. On a surface
where a 2% nudge moves four-season points by 67, a last-bit difference can flip a squad comparison
and make a replay non-reproducible.

Found because `TestTeamFormIsPointInTime` reported two figures that printed identically to six
decimals and compared unequal. Worth noting the test was written for a *different* hazard and caught
this one.

### 4. The same test then failed for a second, real reason. FIXED

Once the fixture adjustment landed, `Season.Fixtures` became a second input to the index, and the
truncated season the leak test builds did not carry it. Fixed by truncating fixtures too. Difficulty
ratings are published before a season starts, so reading a future fixture's rating is not hindsight
the way reading its scoreline is — but "not hindsight" is a judgement and "identical without the
future" is a fact, so it is checked.

### 5. Form is about half fixtures, and that half is anti-signal. RECORDED

Form correlates with the ease of the fixtures producing it at +0.407 (partly mechanical, see finding
2). Neutralising both windows improves the blend's prediction from 0.2390 to 0.2339. The mechanism is
sharper than the "fixtures revert" claim first made: a club's fixture ease in one window correlates
with its ease in the next at **−0.519**, because the schedule is a fixed budget.

Stated carefully because the obvious objection is correct: relative team strength *is* knowable and
predictive, and upcoming fixtures *are* published. The narrow claim is that last window's opponents
say nothing useful about next window's, and the model already prices the actual upcoming ones.

## Declined

- **Switching the blend on.** Nothing resolves; `POLICY` is negative.
- **Re-running the sweep on the fixture-adjusted variant.** The sweep launched before that finding, so
  it measured the raw blend. The adjusted version is the better-motivated arm and is unmeasured on the
  replay. Not run because the raw arm's `POLICY` result gives no reason to spend four more hours, and
  because the prediction gap between the two is 0.2390 against 0.2339 — far inside what 24 cells
  resolve. **Named as the specific unmeasured thing** rather than left implicit.
- **Wiring the blend live.** `TeamForm` is nil outside the replay by design. There is no case for
  paying for a live feed of a correction that has not earned points.

## What could not be checked on this harness

- **Whether the blend helps in the seasons it is active in.** It is structurally inert before nine
  gameweeks, so early-entry cells contribute exactly +0.000 and a third of the grid carries no signal
  either way. The effective sample is smaller than 24 cells and the SE does not know that.
- **Whether the `POLICY` sign is real.** −0.115 at t = −0.16 is not a negative result, it is no
  result, and the transfer path's own noise band is about 300 points a season.
- **The fixture-adjusted arm on the replay**, as above.
