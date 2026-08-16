# The velocity block — four Tier 0 items, three reviewers

Branch `velocity-block-2026-08-15`, off `origin/main` at `2ebd2ae`. Commits `e00b14f`,
`711a374`, `9bf1471`, `0ce5ec5`, `7ba91c4`.

Four items from the work queue's Tier 0 (velocity), taken because the queue's priority rule puts
that class third of four and because none of them needed a sweep. No replay was run for any of
this and none was owed: every claim here is settleable by reading code, git history and banked
snapshots.

| item | what it was |
|---|---|
| G2 | `TestSnapshotCoversTheCurrentCode`'s failure message mis-described what trips it |
| G1 | The retraction guard could not see the agent definitions or `README.md` |
| the no-floor label | `MinExpectedMinutes = 0` is the shipped floor, and three sweep blocks labelled that arm "no floor" |
| the document drift | Three documents described the four-season grid in the present tense |

The document-drift item is the one the queue tracked as **PARTIAL**, and **F1** — the `README.md`
degrees-of-freedom claim, filed separately — turned out to be the same work seen from the other
end. **Both are closed here, once.** The queue warned they must not be closed twice; they are not.

## Reviewers

| reviewer | why | when |
|---|---|---|
| **fpl-stats-review** | the drift item is a measurement claim, and the queue said explicitly that the replacement wording must not be guessed | on the **plan**, before the edit |
| **fpl-code-review** | the diff touches the backtest harness and the guard layer | on the result |
| **fpl-findings-audit** | the commits assert things about estimands, contamination and supersession | on the result |
| **fpl-docs-accuracy** | three user-facing documents changed | on the result |

**Skipped, with reasons.** `fpl-security-review` — nothing touches `internal/agent`, `internal/fpl`,
config persistence or the cache. `fpl-run-review` — no live run, nothing written to config.
`fpl-season-maintenance` — the four hand-maintained lists are untouched.

**The statistics review ran on the plan rather than the diff, deliberately.** The standing rule is
"review the plan, not just the output, for anything that will produce a number", and its recorded
evidence is that two of five commissioned measurements had briefs whose inference was invalid and
whose *code was correct* — a review of the finished work would have blessed both.

## Invariants first, and they earned it

Before any reviewer was dispatched: what must this change **not** move?

- **The guard widening must actually reach the new surfaces.** Both guards were green on arrival,
  exactly as the queue predicted — which proves nothing. Mutation-tested: a planted retracted
  `0.53` in `.claude/agents/fpl-stats-review.md` and in `README.md` fires the guard at both sites;
  a typoed surface name fires the new per-surface floor in both guards.
- **The floor label must name a value that rebuilds the arm.**
  `TestTheFloorLabelNamesAValueThatReproducesTheArm` parses the value back out of the label and
  resolves it the way the opening build does. Mutation-tested both ways it can fail: a bare
  `"no floor"` trips the parse arm, and `"floor=0 (no floor)"` trips the round-trip arm — the
  second being the defect the item was filed against.
- **The git-backed file lister must not change what is scanned.** `git ls-files` over
  `internal`/`cmd` is byte-identical to the `WalkDir` it replaces, 329 files either way.

Two of those caught nothing. The third is the one that mattered, and it is recorded below under
what the reviewers found, because the equality holds only on a clean tree and the commit message
originally said so without the qualifier.

## Findings applied

Ranked by how misleading the state was before the fix.

### 1. `273` and `232` are not one arm's two power notches — the claim crossed a data state too

`fpl-findings-audit`. My own commit message asserted that `docs/accuracy.md`'s `273` was the
80%-power MDE of the very arm whose p = 0.05 threshold is `232`, "one power notch up on the same
estimator and the same grid". **That is arithmetically impossible as stated.** Within one row of
`variance_components.R` the ratio `mde/sig` is fixed by df alone; at 3 df the self-test pins the
multiples at 3.1824 and 4.1609, ratio 1.30748. So 232's partner is **303**, not 273 — and
`internal/snapshot/inference_test.go:178` asserts exactly that, `harness.mde_season.coarsest ==
"303"`, "want the flat arm's 303".

Both figures do belong to the MINHL#1 flat-no-recency POLICY arm on the four-season clustered
estimator, but to **two different runs**: 232 from `stats/testdata/minutes_half_life_cells.csv`,
273 from the snapshot series at `2026-08-10-9af2315`. The page crossed a power notch **and** a data
state — "an estimator swap reads as a data change" arriving twice in one number.

**Applied**: the fix to `232` stands, the reasoning in the commit message is corrected at `7ba91c4`.
Verified independently before applying.

### 2. The estimator mixing was fixed in one document and left in two

`fpl-findings-audit`. `9bf1471` corrected `stats/README.md` to say the `3.9–232` span is pooled
across both estimators while the median 39 is season-clustered — and left `docs/accuracy.md`
carrying the unqualified span **and** attributing all of it to cell-consistency, which the same
commit establishes is false. `docs/replay.md` carried the identical sentence and is the document
the other two now point at.

`stats/regenerate_mde.sh:19-25` re-derives the population from committed cells: **n = 23, median
39.6, range 7.6–231.9**, with start-fixed printed separately at 3.8. So the clustered range is
**7.6 to 232** and the 3.9 end is start-fixed.

**Applied** to both documents.

### 3. `requireEverySurface` was vacuous on the two surfaces handed to it as literals

`fpl-code-review`, and the most serious defect I introduced. `{filepath.Join(root, "CLAUDE.md")}`
has length 1 whether or not the file exists, so the new floor could never fire on `CLAUDE.md` or
`README.md`. The failure that hides is not quiet: `".claude/*.md"` sorts before `"CLAUDE.md"`, so a
renamed or absent `CLAUDE.md` would scan the nine agent definitions, reach `checkRetracted`'s
`rel == "CLAUDE.md"` branch, and `Skipf` the **whole test** — `README.md`, `docs/`, `stats/` and all
329 Go files unscanned, `go test ./...` exiting 0, one SKIP line. That is "a guard that quietly
scans nothing reports PASS" surviving in the surface this commit had just hardcoded.

**Applied**: every surface is discovered via `trackedFiles`, so an absent file is an empty surface
and a loud failure.

### 4. `floorLabel` carried a second copy of the shipped floor, and mislabelled 0

`fpl-code-review`. Two defects in one function:

- It spelled `55` literally, three lines under `resolvedMinExpectedMinutes`' comment saying that a
  second copy of that switch is the hazard — "if the default moved to 60, a probe carrying its own
  switch would keep building the 55 arm, keep printing a clean result, and read as a perfect
  reproduction". It asks `resolvedMinExpectedMinutes` now.
- `floorLabel(0)` returned `"floor=0"`, which a human reads as *no floor* and which is the shipped
  floor — the exact inversion the function was written to remove. It labels by the **resolved**
  value now, so 0 and the shipped rung carry the same label, which is what they are.
- `%.0f` would round a legal fractional rung — 62.5 → `"floor=62"` — into a label that reproduces a
  different arm, invisibly. `%g`.

The guard's table was a hand copy of the two sweeps' rung literals, so it could go stale only on
values nobody listed. It now carries the three classes those rungs happen to miss: `0`, a
fractional rung, and a negative other than `-1`.

### 5. A test-only helper in a watched path was the whole of this branch's snapshot debt

`fpl-code-review`. `internal/backtest` is in `SnapshotWatchedPaths`, and `floorLabel` — used only by
tests — was the single reason `TestSnapshotCoversTheCurrentCode` fired. Moving it to
`floorpopulation_test.go` cleared that gate outright, so no snapshot regeneration was owed.

**The pointer comment I first left behind in `simulate.go` re-keyed the snapshot on its own**, which
is precisely the cost G2's rewording exists to name, so it went too. The reverse pointer lives in
`floorLabel`'s doc comment instead.

### 6. A diagnostic banked a wrong data state into its provenance field

`fpl-docs-accuracy`. `TestDiagTransferError` reads `sweepPairNames()` — six seasons — and hardcodes
`"4 seasons x 3 start points"` in two labels, one of which is emitted into the model CSV as the
row's `grid`. A wrong data state written into the field whose job is recording the data state; the
snapshot duly reads "480 moves judged, 4 seasons x 3 start points" for a six-season run. Both labels
derive from the grid now.

### 7. `README.md:140` — the front-door site I missed

`fpl-docs-accuracy`. "Four archived seasons entered at six deadlines give 24 matched cells", two
hundred lines above the site I did fix, in a file that is in **no** guard's reach. The README was
internally inconsistent for the length of one commit.

### 8. `harness_test.go`'s grid advice was superseded and unmarked

`fpl-findings-audit`. The comment tells sweepers that transfer settings want
`FPL_SWEEP_SEASONS=default`; `CLAUDE.md` superseded that on 2026-08-13 — ten more POLICY arms, four
of them `min_gain`, widening helping 10 of 11, median ratio 0.62. That comment is what a sweeper
reads before choosing a grid, so the two disagreeing silently was the cost. **Marked in place
rather than deleted**, since the sentence is the thing being retracted.

### 9. Smaller, all applied

- `docs/accuracy.md`'s cited snapshot `2026-08-10-9e5e1d1` was **deleted** at `3095fe5` as one of
  fourteen "pre-rigour" snapshots, 111 minutes after the page was written. The page now says its
  figures cannot be checked against anything in the tree rather than pointing at a missing
  directory. **Figures untouched** — see "declined" below.
  ⚠️ **Corrected after this record was first written: `stats/snapshots/2026-08-10-27740ba` is LIVE
  and carries the identical transfer-error rows** (321 moves, buy −0.609 / sell −0.201), because
  `3095fe5` kept it deliberately as "the only one taken after the autosub fix". So the older half of
  that comparison *is* checkable from a fresh checkout, and the right fix is to re-point the page's
  citation at the live snapshot — which is a different and much cheaper act than re-rendering it.
  ⚠️ **This also refutes part of the reasoning above**: the two states cannot differ by the autosub
  fix, since a post-fix snapshot reproduces the pre-fix rows exactly.
- The prediction benchmark inherits `sweepPairNames()` through `loadPairs`, so its population
  follows the grid. That answers the question I had left open at `stats/README.md:363` and corrects
  three further sites stating four-season observation counts in the present tense (40,000 → ~60,000
  observations, ~130 → ~200 clusters, "season clustering gives four" → six). The coupling itself is
  now stated, which is what stops it regrowing.
- The 85-seconds-an-arm timing rotted when the grid widened, as `docs/replay.md` already said
  outright. Budget from the per-cell rate.
- The last surviving "the archive carries expected goals from 2022-23 only, so there will never be
  many more seasons". The conclusion survives on a different mechanism, checked against the loader:
  2018-19 publishes no `teams.csv` and 2016-17/2017-18 no `fixtures.csv`, so a season that cannot be
  played cannot be a cell.
- `shape_inference.R`'s description was pinned to a grid the script resolves from its input.
- My own slip: `TestSeasonNeedsReproduceTheNamedGrids` enumerates **four** grids, not three.

## Findings declined

- **Re-render `docs/accuracy.md`'s figures.** The newest banked snapshot moves most of the page, and
  one claim — "it is more wrong about a player it buys than one it sells" — is now **the wrong way
  round**: buy −0.61 → −0.258, sell −0.20 → −0.305. ⚠️ **Both sides stay negative, so both stay
  over-predictions; what reverses is which is larger, not a sign.** The first version of this line
  said "reversed sign", which tells a reader one side flipped direction — a different and much
  larger result. Caught on review, after the wrong wording had already been copied outward.
  ⚠️ **Corrected in round two: this sentence said "under-predictions", which is backwards.**
  `transfererror_test.go:20` defines `buy error = actual − modelled`, so a negative median is the
  model rating a player **above** what he returned. The substance was right and the term was
  inverted, in the one line written to stop a direction being misread.
  That is a measurement question for `fpl-stats-review`, not a documentation edit, and the queue's
  own rule is not to silently re-point a page at a newer directory because the order statistics move
  a lot between snapshots. **Filed, with the transfer-error label fix as its prerequisite** — which
  is done here, so the re-render is now unblocked.
- **Add `.claude` to `ReviewWatchedPaths`.** Correct in principle — the same "two guards, one rule"
  pattern — but it makes every agent-definition edit owe a review record, which is a standing cost
  and an operator's call rather than a coding one. Filed.
- **Give `mermaid_test.go` the shared surface.** A third copy of the surface enumeration, with no
  floor at all. Low impact, and widening it blind risks firing on legitimate content. Filed.
- **Edit `CLAUDE.md`'s "worth roughly 20-26 a season on `HOLD`"**, which is ambiguous between a level
  and a gain and attaches a pooled endpoint (26) to `HOLD` (22). `CLAUDE.md` was outside this
  change's scope and a wording change there wants its own review. Filed with the proposed wording.
- **`stats/README.md:363`'s three sibling holes** — which diagnostics are four-season by capability
  versus which follow the grid, and `understat_starts_backfill.py` having no section. Filed.

## What did not survive checking

**A report is a set of proposals.** One claim from each of the three reviewers was wrong, and each
was checked before being acted on:

- **`fpl-stats-review`** said `scoringPairNames()` is named only by `TestDiagExtendedSeasons`
  outside the grid switch. Three diagnostics replay on it — `TestDiagExtendedSeasons`,
  `TestDiagXGAggregate`, `TestDiagXGCPoints` — and two more enumerate it. That matters: two of the
  three are the xGC arms, so their figures are seven-season `HOLD` figures and are not commensurable
  with a 36-cell sweep. The shipped prose says so. Confirmed independently by `fpl-docs-accuracy`.
- **`fpl-stats-review`** proposed treating `273` as a stale literal for `232`, which is the
  conclusion I had reached myself and which `fpl-findings-audit` then showed was right for the wrong
  reason. See finding 1.
- **My own correction to the reviewer** was itself checked rather than assumed: I first hypothesised
  that widening `TestNoLivePointerCitesTheRecordByPath` to `.claude/` would fire, because agent
  definitions are permitted to name things user-facing docs may not. **That was wrong** — that guard
  forbids specific stale repo-relative paths, and `.claude/` contains none. Both guards were widened.

## What could not be checked on this harness

- **Whether block J or block E is one of the seven ladders in the banked schedule screen.**
  `fpl-code-review` traced that `schedule_screen.R`'s label parse now constructs a **5-rung** ladder
  for block E where it previously built 4, because the baseline's prefix changed from `""` to
  `"floor="`. Strictly an improvement — the baseline stopped being dropped — but a fresh screen run
  over that block would build a different arm set than the banked one, changing its contrast count
  and therefore its Holm family. **Report the ladder width beside any re-run.** Nothing banked is
  invalidated: the cells CSV carries the reproducing value in `min_expected_minutes` separately, and
  no reader joins across runs on the label.
- **Whether `CLAUDE.md`'s 20-26 has a third source.** `mde_aggregate.py:164` cites the grid-width
  study; the study's banked artifact carries 12.4/8.4/0.677, and 22/26 appear only as arithmetic in
  `gridwidth_test.go`. The two citations point at each other. Unmeasured rather than unmeasurable —
  it is one question to `fpl-stats-review`.
- **Whether the model's top-band calibration really moved from 0.84 to 0.96.** That is the
  `docs/accuracy.md` re-render, declined above.

## A note on the git-backed lister

The commit message records the `WalkDir` → `git ls-files` swap as verified byte-identical, 329 files
either way. `fpl-code-review` re-verified it and added the qualifier that matters: **that equality is
a property of a clean tree**. An untracked new `.go` file is now invisible to the guard until it is
staged, where the walk saw it on save; and a tracked file deleted without `git rm` stays an index
entry. Neither is fatal and the trade is worth it — the alternative walk returns ~2,000 files from
eighteen sibling worktrees in the primary checkout — but the caveat is in the helper's comment now
rather than only here.

---

# Round two — `fpl-docs-accuracy` on the four later commits

The record above covers the branch through `7ba91c4`. Four commits landed after it — `c146ae0`,
`a08797c`, `03e7d34`, `39cbb34` — and all four edit *this record* or the documents it had just
changed, which is exactly the material no reviewer had seen. `fpl-docs-accuracy` was re-dispatched
over them and over the four documents as they now stand.

**Every figure below was re-derived here before it was acted on**, because a report is a set of
proposals: the snapshot key diff, the MDE medians, the diagnostic count and the sign convention were
all re-run rather than taken on trust. Two of the reviewer's proposals did not survive that.

## The one that mattered: the page said it could not be checked, and it can

`docs/accuracy.md` cited `2026-08-10-9e5e1d1`, which `3095fe5` deleted, and the ⚠️ under it told the
reader "the figures below cannot be checked against anything in the tree". **That is false**, and
the previous round had already half-caught it — finding 9 above records the correction that
`2026-08-10-27740ba` is live and carries the identical transfer-error rows, but the page itself was
never re-pointed. So the correction lived in the review record and the wrong claim stayed in the
document, which is the worse half of the pair to leave standing.

Re-pointing was **checked key by key rather than argued**. Recovering the deleted file with
`git show 3095fe5^:…` and diffing it against the live one:

| prefix | identical | differ | absent from the live file |
|---|---|---|---|
| `model.*` | **513** | **0** | 0 |
| `harness.*` | 15 | 48 | 0 |
| `sweep.*` | 5 | 4 | 0 |
| `stamp.*` | 0 | 2 | 0 |

Both files carry 588 keys. **Every model figure the page quotes resolves in the live snapshot,
unchanged**, so this is a citation repair and not a re-render — the declined re-render above stays
declined and stays owed.

Two things make it strictly better than a relocation. `9e5e1d1` carries `stamp.dirty,true`: it was
rendered from a dirty tree and was **never attributable to any committed code state**, where
`27740ba` is clean and is an ancestor of `main`. And the 48 `harness.*` rows that *do* differ reach
nothing on the page, because the earlier round had already routed harness thresholds to
`stats/out/<sweep>/mde.csv` — **that separation is what makes the swap safe**, and it is now stated
as the reason rather than left as a coincidence.

## Findings applied

Ranked by how misleading the state was before the fix.

### 1. The bolding in the RMSE table was the MAE ordering

The table is headed "Root-mean-square error … **Lower is better**", and in two of five columns it
bolded the higher number. Both are explained by one cause — all five bolds match the **MAE** argmin,
and the three columns where the two metrics agree were right by luck:

| column | model | naive5 | flat season | RMSE winner | was bolded |
|---|---|---|---|---|---|
| Tickers | 1.5740 | 1.8349 | **1.5598** | flat season | model |
| Haulers | **5.5970** | 5.6183 | 5.8052 | model | naive5 |

It reached the page's opening summary too: "**worse** than a moving average at picking out the
players who go on to haul" is false on the table's own metric — 5.5970 against 5.6183.

**Applied**: the bold marks the RMSE minimum, the summary says "line-ball", and a ⚠️ under the table
gives both MAE figures and says neither near-tied column supports a claim about either predictor.
Under 1% separates them and they flip between metrics; that is the honest reading, and it is also
why nobody noticed.

### 2. A section that reversed had its retraction 150 lines away

The top-of-page ⚠️ correctly said one section had moved, and a reader arriving at "It is more wrong
about a player it buys than one it sells" got nothing. Re-derived from
`2026-08-15-9e743cf`: buy −0.6086 → −0.2577, sell −0.2006 → −0.3046, **asymmetry −0.4080 → +0.0469**.
The heading and the argmax paragraph under it argue a direction that no longer holds.

**Applied**: a dated in-place marker under the heading, figures left as the record of what was run.
The availability sub-finding genuinely survives — `never_played_again` is unchanged at −2.4739 — and
the marker says so rather than casting doubt over the whole section.

### 3. Two figures on a page whose first rule is provenance are not banked

`docs/accuracy.md` cited the naive predictor realising "about 61% of what it promises" in its top
band "against this model's 87%". `reportPredictionCalibration` guards its sink with
`if name == "model"` (`prediction_test.go:1151`), so **the naive predictors' band ratios are printed
and never banked** — neither snapshot contains them. And the model's own banked figure is **0.8445**,
not 87%.

**Applied**: 87% corrected to 84.5% with its key and its two underlying numbers, and the naive pair
marked as read off a console run. Line 11 says every model figure comes from the snapshot; this was
the one sentence that silently made that false, which matters more than the 2.5 points of error.

### 4. `12% of the gain` is 12% of the threshold

`harness_test.go:106-112` prices the borrowed provider offset at "7.4 rather than 8.4 — which is ~12%
and a lower bound". That is 12% **of the six-season threshold**. As a fraction of the *gain* — the
12.4 → 8.4 improvement — the same point is nearer a fifth.

**Applied**, with the arithmetic spelled out, plus the caveat the reviewer flagged and I confirmed:
that comment block ends in a "superseded by the xGC repair at `7cb769e`" marker, so the figure was
measured pre-repair and must not be presented as post-repair. The reviewer's defence of the
neighbouring **0.677** also stands and is now on the page: it is the positive control, the arm
entitled to answer, the other two measured being near-degenerate.

### 5. The estimator was named on the range and not on the median

Both documents now say the 7.6–232 span is season-clustered and the 3.9 end start-fixed — and left
the **median 39** beside it unlabelled, which raises the identical question the ⚠️ was written to
answer. Re-derived with `stats/regenerate_mde.sh`: n = 23, clustered median **39.6**, start-fixed
**32.3**, pooled 3.8–231.9.

**Applied** at both sites, four words each. And `stats/README.md:316`'s "**35** on the start-fixed
one" is the one figure in four documents that re-derives *different* — it is **32**. Corrected, and
pointed at the script that prints both, since this one is seconds away from checkable.

### 6. `FPL_NO_STARTS_REPAIR` is a member of a class `docs/replay.md` enumerates

The ⚠️ box naming the switches that change the parsed season lists three. `applyStartsRepair` is
called from `repaired` on the `Load` path (`season.go:542`) and reads the variable at
`startsrepair.go:69`, so it is a fourth — and the comment at `season.go:535-541` records that
placing it in `fetch` instead already caused exactly the failure the box warns about.

**Applied** to the box and the switch table, with the caveat that makes it usable: at shipped config
both arms are byte-identical, so a null there says the harvest is unread on the scoring path rather
than that it does not matter. That is the byte-identical trap, and an entry without it would invite
the reading it exists to prevent.

### 7. Smaller, all applied

- **"Roughly sixty" `TestDiag*` tests is 99**, in `docs/replay.md` and `docs/architecture.md` — one
  quantity, two copies, both stale. Replaced with "around a hundred, and the current count is one
  `grep` away" rather than re-pinned, since a literal here will rot again.
- **`docs/README.md`'s "60 banked runs … eleven `cells/` directories"** re-derive as **71** and
  **14**; "twelve `FINDINGS.md`" is still right. The sentence's job is "the evidence layer is not
  empty", which neither count carries, so both moving numbers are gone.
- **`stats/regenerate_mde.sh` cited a literal that no longer exists** — `mde_aggregate.py:56`'s
  hand-maintained `SIX = {…}`, replaced by `grid_seasons()` deriving the width from the `seasons`
  column. The *warning* it introduced is still correct and is kept: the filtered six-season line
  resolves to two arms, both `vice6` — HOLD 8.40 and POLICY 16.11 — confirmed against
  `mde-all-arms.csv` rather than assumed from the label.

## What did not survive checking

- **The reviewer's proposed top-of-page wording reused "reversed sign"** for the buy/sell
  section — the exact phrasing this record had already caught and corrected as telling the reader
  that a side flipped direction. Not applied; the marker says the asymmetry reverses and both sides
  stay negative. **The wrong wording came back through a reviewer that had read the corrected
  record**, which is worth noting: the record stopped it the second time, but only because someone
  compared.
- **This record's own correction had the term backwards.** "Both sides remain under-predictions" is
  wrong — the convention is `actual − modelled`, so negative is an over-prediction. Fixed in place
  above. The sentence was written to stop a direction being misread and inverted a direction doing
  it.
- **`README.md:140` was reported as a possible fourth-to-six overwrite and is correct as it
  stands** — it describes the shipped grid (`extendedPairNames` × six entry gameweeks = 36), not a
  recorded four-season result. The two sites stating the record's own four-season population both
  still say four and both are right. **No recorded result was widened by this branch**, which was the
  thing most worth checking and the thing a reader of the diff would have worried about.

## Filed, not applied

- **Four more registered switches are documented nowhere** — `FPL_NO_LEGAL_AUTOSUBS`,
  `FPL_TEAM_FORM`, `FPL_TEAM_FORM_RAW`, `FPL_WC_IGNORES_BOOST`, all in
  `internal/snapshot/fingerprint.go`. Adding them is a documentation expansion rather than a
  correction, and `FPL_TEAM_FORM` in particular wants its shipped-off verdict beside it.
- **`docs/accuracy.md` says the model orders players 29% better; `CLAUDE.md` says 28%**, and the same
  ratio on `2026-08-15-9e743cf` is 31%. The 29% is correctly derived from the page's own snapshot.
  Which is canonical is a measurement question for `fpl-stats-review`, and `CLAUDE.md` remains
  outside this change's scope for the reason already given above.
- **The hedging on "roughly 26" is unevenly distributed across three documents**, and
  `docs/accuracy.md` ends by sending the reader to `docs/replay.md` for "the full treatment" — which
  is the *less* hedged of the two on that figure. The reviewer proposes moving the measured check and
  the backfill cost into `replay.md` and having `accuracy.md` defer. Sound, and an editorial
  restructure rather than a correction.
- **`stats/regenerate_mde.sh` reports `oraclechip` as `FAIL`.** Its arms are all-zero, so the
  variance decomposition prints `NaN%` and `lm` halts on `NA/NaN/Inf in 'y'`. Pre-existing and
  untouched here — it is the triple-captain arm `CLAUDE.md` already records as degenerate, 23 of 36
  cells placing the chip. But **"FAIL" is the wrong word for "unmeasurable"**, and a sweep that
  cannot run reading as a sweep that broke is the byte-identical trap wearing a new hat. Worth a
  distinct status; not a change to make inside a documentation review.
- **A provenance diagram for `docs/accuracy.md`.** The page now has two figure sources with
  different regeneration paths and different staleness stories, carried in prose. A small
  `flowchart LR` would say it in one glance. **Not drawn**: `TestMermaidBlocksAreWellFormed` is a
  well-formedness check and not a renderer, so a green suite would not tell anyone it draws.

## Key scope after the rebase — why this record's key is stale on purpose

This round was committed on `velocity-block-2026-08-15`, which **merged into `origin/main`
(`82fc8e0`) while the work was in progress**. A merged worktree branch is retired rather than
reusable, so the commit was moved to `docs-review-followup-2026-08-15`, branched off `origin/main`
and cherry-picked clean — none of the eight files it touches had moved on `main`.

`key.csv` in this directory was keyed against the **pre-rebase** base and is therefore stale on the
new one. It is also **inert**: `NewestKey` arbitrates on `recorded_at`, and
`2026-08-15-same-club-bonus-closed-line` (17:41) is later than this record (17:03), so that record
is the gate's arbiter and this key decides nothing.

⚠️ **It was deliberately not refreshed, and `TestReviewCoversTheCurrentCode` is consequently RED on
this branch.** Refreshing it would stamp a digest over the whole watch list at this commit and make
*this* record the arbiter — asserting that this review covers everything currently on `main`. It
does not. **The gate is already red on `origin/main` itself, before this branch existed**: the
newest record there is `same-club-bonus-closed-line` at digest `37123fbca8cc` against an actual
`03c0c69b63e8`, with four trees moved under merges that carry no covering record —

    internal/analysis   internal/agent   internal/config   stats

None of the four is this change's. Re-keying here would turn that red green without anyone having
read them, which is the "a guard that quietly scans nothing reports PASS" failure this same record
argues against in finding 3 above, arriving from the opposite direction: not a guard that scans
nothing, but a key that claims everything.

**What is owed, and by whom:** a review of those four trees, from whoever merged them. It is not
absorbed here, and this section exists so that it is not lost when the next branch keys over it.
The docs changes in this commit are covered by the round above.

## A correction to the reviewer's standing brief

`fpl-docs-accuracy` is briefed that the root `README.md` "is guarded by none of the three tests named
in this brief". **That stopped being true on this branch at `e00b14f`** —
`internal/snapshot/retracted_test.go:314-328` adds `README.md` as the retraction guard's "first
widening". The reviewer read the README by hand anyway and flagged its own brief, which is the
behaviour that catches this class. The brief lives in `.claude/`, outside both watch lists, so
nothing else would have.
