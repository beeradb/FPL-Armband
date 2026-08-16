# Review — `7f33270..833cddc`, branch `b5-season-list-parity`

Audit item **B5** from TODO.md's "One quantity, two implementations — the repo-wide audit": the
hand-maintained season lists exist as Go defaults *and* as literal copies in the shipped
`config.json`. Two commits: the guard, then the review's corrections.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-code-review** | config persistence semantics, and the four season lists | **Found a scope hole and a blind test.** Both confirmed independently and fixed |

Skipped, with reasons:

- **fpl-security-review** — the triage table pairs it with code-review for config persistence, but
  this change *reads* `config.json` in a test and alters no persistence path, no credential
  handling and no agent input. Recorded as not applicable rather than silently omitted.
- **fpl-season-maintenance** — it *re-derives* the four lists, which is a research task. B5 is a
  guard against the two copies drifting, not a summer re-derivation, and the lists' staleness is a
  separate standing item.
- **fpl-stats-review**, **fpl-findings-audit** — no measurement, no claim about the model, and
  nothing added to `CLAUDE.md`.

## Was an invariant the better tool?

This *is* the invariant — B5's whole deliverable is a guard. The question that mattered instead was
whether the guard's own scope was right, and it was not.

## Finding 1 — the scope was four lists; seven are duplicated and three already disagree

**Verified independently** by reflecting over every slice and map field of `Weights`, `Congestion`
and `RoleRisk` before accepting it:

| list | config.json | Go | |
|---|---|---|---|
| `weights.rest_players` | 17 | 17 | agree |
| `weights.tournament_absences` | 6 | 6 | agree — **unguarded until now** |
| `weights.minutes_weight_by_position` | 4 | 4 | agree |
| `congestion.european_campaigns` | 9 | 9 | agree |
| `congestion.domestic_cup_campaigns` | 20 | 20 | agree |
| `role_risk.new_coach_clubs` | 10 | 10 | agree |
| **`congestion.long_haul_regions`** | **[30, 10]** | **[]** | **differ** |
| **`congestion.regular_international_regions`** | **5 codes** | **[]** | **differ** |
| **`role_risk.confirmed_starters`** | **4 names** | **[]** | **differ** |

⚠️ **This class of divergence has already shipped a live effect**, and the repo already knows:
`DefaultCongestion`'s comment records that `long_haul_regions` is empty in Go, `[30, 10]` in the
file, has **no backfill**, and so ran a **0.86 multiplier on every Brazilian and Argentine** while
a comment claimed the term was inert. `confirmed_starters` exempts four players from
`NewSigningPenalty`, live at **0.88**.

**Applied**: scope widened to seven, the three divergent ones **whitelisted with their reasons**,
and the whitelist itself checked — reconciling a divergence fails the test and asks for the entry
to be deleted, so an exception cannot outlive the reason it was granted.

## Finding 2 — the mechanism test did not exercise `Load`

The reviewer proved it with a probe rather than by argument, and **I reproduced the probe**:
inserting `cfg.Congestion.European = nil` into `Load` immediately before its unmarshal — the exact
refactor the test's docstring claims to protect against — left the test **passing**. It was pinning
a property of `encoding/json`, not of this repository.

**Applied**: it now writes a real file and calls `Load`. The probe fails it. Going through `Load`
also covers `config.go:229-232`, which is the real reason the campaign maps cannot be emptied.

**A test that never exercises the function it names is testing the language.**

## Finding 3 — folding map keys was misinformation, not just leniency

Map lookups downstream are exact (`cg.European[team.ShortName]`). Folding matched `ars` to `ARS`,
then indexed the JSON map with the **Go** spelling and reported `config.json: (none)` for a club
whose window the file does carry. **Applied**: exact for maps, fold for slices only.

The slices keep the fold, with a corrected justification: `RestPlayers` does **not** go through
`containsFold` — it goes through `Boot.FindPlayers`, which lowercases and then matches
exact/prefix/contains, so it is case-insensitive and fuzzier. My assumption was right for the wrong
reason.

## Finding 4 — a correction to my own framing

**`rest_players` is not display-only.** It sits under `Weights`, and `RestMinutesFactor` ships at
**0.83** against expected minutes. Only the two congestion *maps* are neutralised by the 1.00
penalties. I had over-generalised that in the brief.

## Also applied

`t.Skipf` → `t.Fatalf` on a missing `config.json`. It is committed at the repo root, so its absence
is itself a defect, and a check that cannot run is indistinguishable in the output from one that
passed.

## Declined

- **Fixing the asymmetry** (pointerising the slices, or clearing the maps before unmarshal). The
  reviewer and I agree: it changes what every existing `config.json` means, against the standing
  rule that config fields need a backfill so existing files stay valid. Recorded as a **known gap
  in the config format** — the two campaign maps cannot be shortened by any route — and pinned by
  its own sub-case so closing it has to be deliberate.
- **Guarding duplicates within a list** (`["A","A"]` vs `["A"]` compares equal). Both consumers are
  membership tests, so it is harmless; noted rather than guarded.

## What could not be checked on this harness

- **Whether the three divergences are correct** as football. The guard fixes their *status* as
  deliberate; whether `[30, 10]` is the right set of long-haul regions is a measurement nobody has
  made, and `DefaultCongestion`'s comment already says the list is inconsistent with its own
  documentation — nine other nations are absent.
- **Whether other duplicated pairs exist outside these three structs.** The sweep covered
  `Weights`, `Congestion` and `RoleRisk`. Other config groups are unexamined.

## Falsification

Five perturbations, each restored and verified clean afterwards: a dropped rest player, a deleted
club, a staled `start_date`, a reconciled whitelist entry, and the `Load` probe. **A guard that has
only ever passed is the failure printing as the pass.**
