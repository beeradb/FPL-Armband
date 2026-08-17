# Measuring the appearance floor, and whether nailed players survive a double

Reviewed: `measure-the-appearance-floor-and-nailedness-in-doubles` against `e88ece9` on
`development`, which is this branch's base.

## What was reviewed

Two **descriptive archive measurements** and the production seams they needed. Ten archived
seasons, 253,509 priced player-matches, 183,428 eligible player-weeks.

1. `analysis.DecomposeMatch` — one realised match split into the eleven channels FPL pays. The
   four channels `ScoringRulesFor` carries come from the season's own table; the rest read
   package constants that are not season-pinned, which is the exposure `ScoringRules`' own
   docstring already declares. Nothing widens that table.
2. `backtest.(*Season).gameweekRows` — `loadGameweeks`'s walk, extracted so a per-match reader
   gets the phantom-match and duplicate-row guards without a second copy of them.
3. Two DIAG-gated diagnostics, plus `mirrorArchive`, a test-only byte cache.
4. Regression tests: the iterator yields one callback per surviving match and applies the same
   guards `Load` records; the decomposition reconciles against `total_points`; the
   card-without-minutes ordering is pinned; three constants added to
   `TestScoringConstantsMatchFPL`.

**No points claim is made anywhere, no threshold or p-value is quoted, no default moves, and no
constant is proposed for change.** These are counts, rates and distributions.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the deliverable is six claims read off archive tables, and the question that decides them is whether any population is selected on the outcome it then measures |
| **fpl-code-review** | yes | the diff touches `internal/analysis` and `internal/backtest`, including the walk the shipped doubles guard lives in |
| fpl-findings-audit | **skipped** | it triages on `AGENTS.md` edits and this change makes none. Recorded as a deliberate skip |
| fpl-security-review | not applicable | no change to `internal/agent`, `internal/fpl`, the client, or config persistence; no new user-settable field |
| fpl-docs-review | not applicable | no change to `README.md` or `docs/` |

**Self-review was not performed and is forbidden.** Everything below came from the two reviewers.

## The findings that changed a result

### The club-depth split was computed from the cell it split, and fixing it reversed the answer

`clubRotation` measured churn in the hour-clearing eleven across consecutive fixtures, and a
double's two fixtures are consecutive. So a club that rotated in its double scored higher churn,
was more likely labelled "rotating", and then read a lower double rate in the split it had
caused. The statistics review named this as selection on the outcome *inside the cell*, and the
defence in the comment — "used only to split, never as a predictor" — did not cover it.

**Applied**: doubled gameweeks are excluded from the measure entirely. The nailed row reverses,
from rotating −0.0277 against settled −0.0195 to rotating −0.0067 against settled −0.0389. That
reversal is the contamination made visible; neither ordering is now claimed, because neither
cell separates from the other.

### The within-player contrast did not match on when, and the sign flipped

A player's doubles fall late in a season and his singles span all of it, so the paired difference
still carried the calendar — cumulative load, dead rubbers, title-race squad management.

**Applied**: the single arm is restricted to within three gameweeks of one of his own doubles.
The mostly-nailed cell goes from −0.0175 to **+0.0162**, and every median is 0.0000. The earlier
negative was the calendar, not the double.

### The diversification test was read against a reference that cannot move

The per-gameweek table compared the nailed bucket against the fringe one, which clears sixty
minutes in 5% of single weeks — close enough to a floor that a second lottery cannot help it. So
that contrast moved almost entirely with the top bucket, and it read the gap as *widening* under
a line of prose calling the number a narrowing.

**Applied**: both references are printed, the word is chosen from the sign, and an
independent-legs benchmark sits beside each rate. Against that benchmark every bucket undershoots
on "at least once" and overshoots on "both", so a double's two legs are **positively correlated**
— which is a better finding than the one the table was written to test, and it is derivable from
the same counts.

### A premise of the diagnostic's own header was false

The header justified enumerating (player, gameweek) pairs from the calendar on the ground that
`merged_gw.csv` lists only matchday squads, so a rotated player would have no row at all.
**Measured: 0 of 183,428 eligible player-weeks have no row.** The archive files a row for every
registered player every week and records the rotated ones at zero minutes.

The design is right for the reason that survives — conditioning on having appeared drops the
rotated player — and it was never right for the reason first given. The header now says so.

### `startedLegs` had a real defect that was worth nothing, and the record says which

A loop over a player's legs finds nothing to object to when there are none, so a week with no row
fell through to "known, zero starts" — in every season, including the six that publish no
`starts` column. Inside a non-recording gameweek the numerator could then only be zero while the
denominator still counted him. The code reviewer sized the consequence by re-basing the printed
columns and put the nailed bucket's `d(start)` near +0.03 against a printed +0.0007.

**That sizing does not hold, and the reason is the finding above**: the contaminated population
is empty. The fix moved **no printed number**. It ships anyway, because a gate that depends on a
coincidence in the current archive is not a gate, and the comment records both halves — the
defect and its zero size — rather than claiming a correction that did not correct anything.

## Findings applied without changing a result

- **The post-double follow-up was per week while the arms differ in matches per week.** A
  follow-up week can itself be a double, and the treated arm meets more of those by construction.
  The 60+ and minutes columns are now per match; the five-minute gap this had produced on the
  matched control is gone, leaving a one-week dip at gw+1 and nothing at gw+2 or gw+3.
- **The post-double control matched load but not composition.** Both arms are now restricted to
  clubs that doubled that season. It still does not match the point in the season, and the note
  says so.
- **"after a full single" was a residual, not a category** — the two-full-singles clause claims
  first, so what was left was a full single preceded by an absence, a bench week or an early
  substitution. Its absence rate is the highest of the three arms and differencing against it
  reverses the comparison. Relabelled.
- **The top-N-by-points selector is the appearance share's own denominator.** Stronger than
  "end-stamped": selecting the top thirty by season points directly selects a low appearance
  share. Both share tables and the headline now say so, and the 10.0m+ price bracket is named as
  the prior-information counterpart.
- **The pooled doubles tables are season-confounded** — roughly half of all double club-gameweeks
  fall in 2020-21 and 2021-22 against a singles column drawn from eight seasons. Stated once,
  above every table it applies to.
- **Cluster counts are printed beside every headline cell**: the distinct (season, club,
  gameweek) doubles behind it, which for the nailed elite cell is 125 against 222 player-weeks.
- **The appearance share now has a per-season column**, because ten seasons span three bonus
  regimes, the defcon introduction and the keeper-goal change — all of which move the share's
  denominator and none of which move its numerator. It is stable (`everyone` 0.561 to 0.620,
  top-30 0.368 to 0.402), so the pooled figure is not a mixture artefact. Measured, not assumed.
- **The byte mirror writes atomically and refuses to cache an empty 200.** A short CSV parses
  cleanly into a season with fewer matches, which reads downstream as football rather than as a
  fault, and a cached empty body would permanently turn a season into "no calendar".
- Smaller: `seasonKey` refuses a zero rather than merging cluster keys; `quartiles` returns NaN
  where there is no distribution; `DecomposeMatch` honours `FPL_NO_UNPRICED_POSITION_GUARD` like
  the rest of the package; a dead `share.PerPlayer` field is gone; a CSV parse failure in the
  reconciliation test fails instead of reporting "archive unreachable"; the look-back window's
  use of the current club's calendar for a mid-season mover is recorded.

## What the reviewers confirmed rather than corrected

- **The `gameweekRows` extraction is behaviour-preserving.** Same guard order (misfiled before
  duplicate, so the counts stay disjoint), same `(element, fixture)` key, same renumbering and
  bound, callback at exactly the old accumulate site. Returning the report rather than assigning
  it is safe: the one former difference — a partial report attached after a mid-walk CSV error —
  is unreachable, because `Load` returns before caching or repairing.
- **`archiveURL` cannot leak between tests.** Nothing in `internal/backtest` runs in parallel,
  the stub tests load a fixture season into a `t.TempDir()`, and the parsed-season cache is keyed
  on the cache directory as well as the name.
- **Nailedness is prior-only.** `priorMinutesShare` reads strictly earlier gameweeks, and the
  tenure floor cannot leak because a week with no history yields zero matches and is dropped.
- **The decomposition reconciles with an identically zero residual on 253,509 rows in all ten
  seasons**, re-run by the reviewer. That is also the evidence that FPL's appearance rule did not
  change across the archive, and it clears the one thing worth checking on the newest channel:
  `defconThreshold` returns 12 for goalkeepers, who are not paid defensive contribution, and no
  2025-26 row reaches it.

## Claims recorded, and claims refused

The statistics review was asked to judge six readings. Two are supported and are what this branch
reports; four are not, and are reported as unresolved or unmeasurable rather than as findings:

| reading | verdict |
|---|---|
| the appearance floor is a minority of an elite player's return | **supported**, with the level relabelled — it is a *majority* for everyone (0.594) and the gradient is the fact |
| a nailed elite player does not bank four appearance points in a double | **supported**, and the selection runs against the conclusion, so it is a bound |
| the per-match 60+ rate falls in doubles for nailed and elite players | **not supported** — the only confound-free estimator reads +0.0015 with a median of 0.0000 |
| per gameweek, diversification favours rotation risks | **replaced** — the gain from a second draw is `p(1−p)`, so the shape is arithmetic; what the counts do show is that the legs are positively correlated |
| post-double absence does not rise | **not identified** rather than null — the control matches club and load but not the calendar |
| the club splits are ordered the way congestion predicts | **refused** — the ordering reversed on a method fix and no cell separates from another |

Nothing here is written into `AGENTS.md` by this branch.
