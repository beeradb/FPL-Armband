# Review — nine accuracy findings applied to `docs/replay.md`, and a count the program got wrong

**Branch:** `replay-doc-accuracy`, cut from `origin/main` at `9f7ce6a`.
**Files:** `docs/replay.md` (nine findings), `CLAUDE.md` (one duplicated figure, kept consistent),
`cmd/fplagent/backtest.go` (one string, and the duplicate behind it removed).

A documentation-accuracy review of `docs/replay.md` returned nine findings and two proposals. All
nine are applied; both proposals are declined, one on merit and one for want of a draft.

## The invariant came first

**What must this change not move?** Any scoring figure, any replayed cell, any recorded constant.
`docs/replay.md` and `CLAUDE.md` are prose and cannot. The one Go edit is a `fmt.Printf` in
`cmd/fplagent/backtest.go` — terminal output of the human-readable replay, downstream of every
number it prints — and it changes a word and derives a count from a slice already in scope. No
`SimConfig`, no weight, no gate, no cell.

`go build ./...`, `go vet ./...` and `go test ./...` all pass, including the 156 s
`internal/backtest` run. `gofmt -l ./internal ./cmd` is empty.

## Every finding was independently re-checked before being applied

The review arrived stating that each finding had already been verified against the code. It was
re-checked anyway, per the standing rule that a review is a set of proposals. **Nine of nine
defects held. Two of nine carried a *supporting* claim that did not**, and both would have been
pasted into the document verbatim — see *What the review got wrong* below.

| # | finding | how it was verified |
|---|---|---|
| 1 | the primary sweep recipe used bare `go test`, twenty-four lines above the paragraph telling you to use `scripts/replay` | read `scripts/replay`: it exports `DIAG=1` and translates `-run`, `-v`, `-timeout`, `-count` (and more) into `-test.*` in an explicit `case` |
| 2 | the memory cap and printed peak RSS are conditional, stated as properties | the caps are `-p MemoryHigh=` / `-p MemoryMax=` inside the `systemctl --user is-active` branch only; the `else` prints `no user systemd manager; running without a memory cap` and runs `nice -n`. `timer=()` when `/usr/bin/time` is not executable, and the `awk` then yields `peak_rss=unknown`. Defaults read off the script: 2G, 4G, nice 10 |
| 3 | `97 MB` is one block measured once | traced to commit `89618ec`, dated **2026-08-11**. Counted every banked figure under `stats/snapshots/` |
| 4 | ten switches exist and are documented nowhere | enumerated every `Getenv` in `internal/` and `cmd/`, then read each switch's declaration and call sites |
| 5 | the drift warning is now false | `grep 'want("'` over `internal/backtest/transferpolicy_test.go` against the three diagnostics' line ranges: the doc's three rows are set-equal to the code's blocks |
| 6 | the slot semaphore is per checkout | `lockdir=…/fpl-replay-$(echo "$repo" | md5sum | cut -c1-12)` — keyed by repository path, with a comment saying so |
| 7 | "all five R scripts" is stale | `ls stats/*.R` → **13**; `stats/README.md` names **8** |
| 8 | the oracle machinery was described by its confinement half only | `Oracles.MustNotMove()` at `oracle.go:1095` and `MustMove()` at `:1290`; `oracleInvarianceViolations` and `oracleLivenessViolations` in `oracleharness_test.go`; read `TestLivenessDeclarationsAreCoherent` in full |
| 9 | the canonical 39 is missing its grid on the sentence people quote | the grid is named two paragraphs up; the estimator was already named on the sentence itself |
| — | `cmd/fplagent/backtest.go` says "four transfer policies" | the `policies` slice holds **three**, with a comment explaining the fourth's removal. **The document was right and the program was wrong** |

## What the review got wrong

Recorded because "verified" was claimed for these, and it did not hold.

1. ⚠️ **"`FPL_MULTI_SURCHARGE` and `FPL_BUDGET_WEIGHT` act only under `FPL_UNIFIED_TRANSFERS`" is
   false of `FPL_BUDGET_WEIGHT`, and false in the expensive direction.** True of the surcharge:
   `surchargeFor` has exactly one call site, inside `unifiedDecide`. But `budgetWeight` is only
   *declared* in `unified.go` — it is *consumed* by `moneyPts` in `decide`
   (`simulate.go`), at both the funded-pair and the single-move sites, which sit **after** the
   `if unifiedTransfers || cfg.Unified { … return }` early return. So it acts on the **shipped
   bespoke search**. Pasted as written, the document would have told a sweeper that a knob live on
   the shipped path was inert. The applied row says so explicitly, with a ⚠️ contrasting it against
   the surcharge above it. This is *check that a setting is read on the path you are about to
   score* and *naming the consumer is the check; naming a package is not* — a declaration site
   read as a consumption site.
2. ⚠️ **"`stats/README.md` already routes its step 1 through the wrapper, so the two documents
   currently disagree" is backwards.** `stats/README.md`'s *Running it* step 1 and both sweep steps
   of its snapshot recipe use bare `go test`; it names `scripts/replay` only in a comment. There
   was no disagreement to resolve. The sentence was **left out of the applied edit** rather than
   softened — the recipe change stands on the wrapper's own guard rails. `stats/README.md` was out
   of this change's scope and is left as it is; the inconsistency is filed.
3. The proposed banked peak-RSS range of **94 to 130 MB** was wrong twice over — the low end
   misses 89 and 91 in `stats/snapshots/2026-08-11-0104d9d/`, and the high end stops below the
   142 MB in `2026-08-13-4d61058/`. The corrected span is **89 to 142**; the working is under
   *The applied edits were themselves reviewed* below, because the first correction of it was
   also wrong.

One provenance fact turned up while checking finding 3, and it is in the document: **three of the
five directories the review cited record the figure in `FINDINGS.md` prose, not in a `peak_rss=`
line** (`-blend` 111, `-blendlo` 119, `-blend-datastate` 94). A grep for the wrapper's own output
format finds none of them.

## The applied edits were themselves reviewed, and six were wrong

The documentation reviewer was run over the working tree before this was committed, and it
returned six must-fix defects **in the corrections above**. All six were re-verified here and all
six are fixed. They are recorded rather than quietly folded in, because five of the six are the
same failure the pass was correcting — a claim written from the shape of the thing rather than
from the thing.

1. ⚠️ **"93 to 130 MB" was false, and the same sentence refuted it** by naming a 142 MB run
   outside the range it had just asserted. It also missed 89 and 91 in
   `stats/snapshots/2026-08-11-0104d9d/` — two 47-minute sweep blocks. Full sweep blocks span
   **89 to 142 MB**; the document now says that, and notes a static probe goes as low as 37.
2. ⚠️ **"117 to 130 in `2026-08-16-anti-residual-gate/`" laundered a transcription contradiction
   into a spread.** That directory holds exactly one run — one `replay: exit` line, `peak_rss=117`
   — and its `FINDINGS.md` says 130 for that same run. Quoting it as a range implied two
   measurements where there is one and a disagreement. It is now stated as the disagreement it is.
3. ⚠️ **"fifteen to thirty times above that range" does not divide.** Against 89-142 MB the soft
   cap is 14-23× and the hard cap 29-46×. Both are now given, separately.
4. ⚠️ **"The tables below cover every switch that changes what the replay *does*" is false in
   three places.** `FPL_CONFIG` — which `config.json` a diagnostic loads, so every constant at once
   — plus `FPL_TEAMNEWS_MAX_HOURS` and `FPL_FIXTURE_LOAD` are registered in `envSwitches` and
   documented in no file in this repository. Checked by diffing the registry against `docs/`,
   `stats/README.md`, `CLAUDE.md` and `README.md`. The section now says it is **not** complete and
   names the three. It also scopes the `envSwitches` authority claim, which reaches Go source only
   and not the wrapper's own shell-side `FPL_REPLAY_*`.
5. ⚠️ **"a recipe recorded anywhere in `docs/` pastes into it unchanged" was false in the silent
   direction, and this is the sharpest of the six.** Swapping `go test` for `scripts/replay` while
   leaving the package path gives the test binary a leading positional argument, and Go's flag
   parsing stops at the first non-flag — so every `-test.*` flag after it is ignored, the binary
   runs the **whole package** under `DIAG=1`, selects nothing, and **exits 0**. Reproduced
   directly in this tree: `/tmp/stats.test ./internal/stats -test.run TestNoSuchTestAtAll -test.v`
   runs the package and prints `PASS`, where the same command without the path prints
   `testing: warning: no tests to run`. A sentence written to make recipes safe to paste would
   have produced exactly the sweep-that-measured-nothing this document is about. The caveat is now
   ⚠️-marked and says to drop the prefix, package path included.
6. ⚠️ **"`AxisChipWeek` declaring none is the deliberate exception" over-claimed**, implying every
   other axis is liveness-checked. `mustMoveForAxis` returns `moves` for the anti-residual and
   accept-all gate arms and nil for the other five. Rewritten — and written as a **rule rather
   than a count**, because `mustMoveForAxis`'s own comment records a count that went stale beside
   it twice.

One optional correction was taken too: the **145 MB and 97 MB are different statistics** — the
1 Hz sampler's peak for the binary against GNU `time`'s for the wrapped run — and the document now
says so rather than leaving two numbers for one block unreconciled. The reviewer also noted, and
this record accepts, that the 1 Hz attribution *of the 97 itself* is asserted rather than
corroborated anywhere in this tree; the sentence no longer claims it.

Three adjacent in-tree defects were reported and **not** acted on, being outside this change:
`internal/snapshot/fingerprint.go` lists `"FPL_CONFIG"` twice; its header says the list is
"generated" where the test says it is hand-maintained and counted (the test is right); and
`FPL_FIXTURE_LOAD` is only ever *printed* by the `WEEKXI` block, which sets the behaviour through
`analysis.SetFixtureLoad` regardless — so the printed line reports a variable that does not control
the arm. That last one is the byte-identical-null shape and is worth someone's attention.

## What was applied

**1. The recipe.** Now `EXP=MINHL FPL_CELLS=… scripts/replay -run TestDiagProjection -v -timeout
180m`, with `DIAG=1` dropped because the wrapper exports it. A sentence after the block says the
flag spellings are `go test`'s and the wrapper translates them, which is *why* a recorded recipe
pastes unchanged rather than an accident that it does.

**2. The caps.** `FPL_REPLAY_MEM_HIGH` (2G, soft) and `FPL_REPLAY_MEM_MAX` (4G, hard) are named,
with the ⚠️ that **both exist only inside `systemd-run --user --scope`** and the exact line the
wrapper prints when they do not. `FPL_REPLAY_NICE` (10) gets its own bullet. The peak-RSS bullet
says it comes from GNU `/usr/bin/time` and degrades to `peak_rss=unknown`.

**3. The 97 MB.** Dated 2026-08-11, named as a before/after pair rather than a budget, distinguished
from the 145 MB beside it (different statistics on the same block), and set against the **89 to
142 MB** a full sweep block has actually banked, with each directory named. The earlier of the
figure's two appearances now says "one block measured both ways on 2026-08-11" and points forward.

⚠️ **`CLAUDE.md` carried the identical 97 MB and defers here — both were changed, in this commit.**
Fixing only the document would have left the resident file, which every session reads, carrying the
undated number and pointing at a page contradicting it: one quantity, two implementations, in the
two files most likely to be quoted from memory. `CLAUDE.md` keeps the dated pair, the range and the
pointer; the derivation stays in one place. Its memory-cap bullet gained the systemd clause for the
same reason.

**4. Ten switch rows**, each written from the declaration and its call sites:

- `FPL_NO_LEGAL_AUTOSUBS` and `FPL_WC_IGNORES_BOOST` under *Turning a shipped behaviour off*,
  placed **immediately after `FPL_NO_VICE_CAPTAIN`** to match `replay.go`'s own order and, more to
  the point, to avoid inserting rows between the three xG switches and `FPL_NO_STARTS_REPAIR`,
  whose cell says "the three above". `FPL_NO_LEGAL_AUTOSUBS` carries a ⚠️ naming it as a
  contamination-event reproducer, worth 7-14 points a season, rather than a knob worth sweeping.
- `FPL_TEAM_FORM`, `FPL_TEAM_FORM_RAW` and `FPL_CHIP_PLAN` under *Turning something on that does
  not ship*. The chip row records that **the replay is chipless unless a plan is given**, and that
  a sweep arm sets `SimConfig.Chips` instead — `FPL_CHIP_PLAN` is read in `cmd/fplagent` and
  nowhere in `internal/backtest`, which `sweepConfig` confirms by setting no chips at all.
- `FPL_APPEARANCE_FIT`, `FPL_MULTI_SURCHARGE`, `FPL_BUDGET_WEIGHT` under *Varying a constant*.
- `FPL_REPLAY_HTML` and `FPL_REPLAY_GWS` under *Selecting the grid and the output*, which is where
  they belong: `internal/snapshot`'s skip list excludes both from the fingerprint on the ground
  that they are about the **page**, not the replay.

The section opener said "Every one of these defaults to shipped behaviour", which reads as a
complete catalogue. It now scopes itself to switches that change what the replay *does*, and
**names `internal/snapshot`'s `envSwitches` as the authority** — `TestEnvSwitchListIsComplete`
holds it against a scan of the tree, so that list is the one a reader can trust to be complete and
this table is the one that explains them.

**5. The drift instance retired, the rule kept.** The table agrees with the code; the paragraph
says so, says that is a fact with a short shelf life rather than a property, and states that
`blockPicker.check`'s generated list is the authority and a disagreement is the table being wrong.

**6. Per checkout, not per machine** — appended to the semaphore bullet.

**7.** "all five R scripts" → "the R scripts that read it". No count.

**8. The liveness half**, in three parts: that `Oracles` declares both sets and the sweep enforces
both, each treating "the sweep does not collect that column" as a failure; that **liveness is the
half with power**, because confinement is a code fact whose re-run can only fail while `MustMove`
is what separates an arm reaching the scored path from an inert one; and the three properties
`TestLivenessDeclarationsAreCoherent` pins, with `AxisChipWeek`'s deliberate exemption.

**9.** "on four seasons × six entry gameweeks" added to the 39 sentence, with a ⚠️ to quote the
grid with it. The estimator was already named there and is untouched.

**The program.** `simulating 38 gameweeks under four transfer policies…` now reads *three* — and
reads it from `len(policies)`. The slice moved above the print with its explanatory comment, so
the count has **one implementation** and nothing can disagree with it. That is the fix rather than
a regression test: the string said "four" for as long as the fourth policy had been gone, which is
this project's signature failure in its cheapest form, and a test asserting a printed number
against a slice length would be a third copy of the same quantity.

## What was declined

- **A flowchart of `scripts/replay`.** Declined **on merit**, agreeing with the reviewer's own
  judgement. It would restate the five numbered guard rails in the order the prose already gives
  them, and the two things it could genuinely carry — the systemd-conditional cap and the
  per-checkout lock — are now two clauses in finding 2.
- **A mermaid diagram of when each class of switch is read.** ⚠️ **Recorded as a candidate worth
  someone's time, not as a rejected idea.** It is the document's most error-prone content: getting
  read-once, read-per-call and read-during-`Load` confused produces a byte-identical null, which is
  the failure class the whole document is aimed at. It is declined here only because the reviewer
  explicitly did not draft or render one, and `TestMermaidBlocksAreWellFormed` is a
  well-formedness check rather than a renderer — a green suite would say nothing about whether it
  draws. It is filed as an open item rather than lost in this record.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **documentation review** | yes | `docs/` and `CLAUDE.md` both changed; this is a documentation-accuracy change end to end |
| code review | no | one `fmt.Printf` and a slice hoisted twelve lines, in a CLI presentation path. No scoring, config, agent or harness code |
| statistics review | no | nothing measured, nothing re-judged. The one number added — the banked RSS range — is memory, not points, and carries its own provenance |
| security review | no | no change to `internal/fpl`, `internal/agent`, config persistence or the cache |
| run review, season maintenance | no | no live run; the four hand-maintained season lists are untouched |
