# The fixture-scale silent fallback

## What was reviewed

Branch `fail-loudly-on-an-unreadable-fixture-scale`, one commit off `origin/main` (`cf4379a`).

`internal/analysis/sweep.go` carried a lenient parser, `envScale`, that returned the shipped `1` on
any value `strconv.ParseFloat` could not read and on any negative. Its only two callers were
`atkFixtureScale` and `defFixtureScale`, reading `FPL_ATK_FIXTURE_SCALE` and
`FPL_DEF_FIXTURE_SCALE`. Both names are stamped into the run provenance by
`internal/snapshot/fingerprint.go`. So `FPL_DEF_FIXTURE_SCALE=1,5` — a comma for a decimal point —
scored every cell at the shipped 1.0 while the fingerprint recorded the run as configured at 1.5:
this record's byte-identical-null trap wearing a provenance stamp, and a null indistinguishable
from "the ladder does nothing".

`envScaleStrict` already existed in the same file, written for `FPL_CS_SCALE` and carrying the
rationale in its own comment. The change converges both siblings on it and deletes `envScale`
rather than leaving a lenient helper for the next knob. Plus a regression test and a `docs/replay.md`
row.

## Reviewers

| reviewer | why |
|---|---|
| **fpl-code-review** | touches `internal/analysis`; and the change asserts byte-identical output, which the triage table sends here pointed at the differential rather than the diff |
| **fpl-stats-review** | asked one question only: does the defect cast doubt on any banked figure measured by setting these two variables |
| **fpl-findings-audit** | asked whether any `CLAUDE.md` verdict rests on these knobs having been set, and whether an edit is owed |

Skipped: **fpl-security-review** (no credential, no persistence, no agent-facing surface),
**fpl-run-review** (no live run), **fpl-season-maintenance** (no hand-maintained list),
**fpl-docs-accuracy** (the one documentation edit is a single table row describing the behaviour
this change introduces, and `fpl-findings-audit` read it).

## The load-bearing claim: byte-identical at every valid configuration

This is the finding that matters, because a change here that moved a scored value would contaminate
every banked figure.

Verified twice, independently. I ran a throwaway in-package differential — the removed lenient
parser against the real `envScaleStrict`, compared on `math.Float64bits` — over 200,034 accepted
inputs, plus 700,000 draws through `ladder()` at seven rungs. `fpl-code-review` did not take that on
trust and re-ran its own at 160,059 inputs. **Zero divergences on any input the old parser did not
already silently default.** Specifically identical: `NaN` (`NaN < 0` is false in both), `±Inf`, `-0`
(bits `8000000000000000`, since `-0.0 < 0` is false under IEEE), `1e-400` → `0` with `err == nil`,
hex-float `0x1.8p1`, `".5"`, `"5."`, `"+1.5"`.

The durable half of the argument is checkable from a checkout without the differential, and is the
better evidence: **`v < 0` is character-for-character the same expression in both functions**, and
`strconv.ParseFloat` never accepts surrounding whitespace — so `TrimSpace(s) != s` implies the old
parser always fell back on `s`. The complete set of newly-refused inputs is therefore: unparseable,
negative, `-Inf`, and overflow (`1e400`, which ParseFloat returns as `+Inf` **with** `ErrRange`, so
the old parser defaulted). All four were previously silent 1s. The one input that moves in the
*accepting* direction is whitespace-padded (`" 1.5"` was 1, is now 1.5), which is inside the same
set.

The differential was **deliberately not committed**. `fpl-code-review` agreed and strengthened the
reason: a committed copy of the removed lenient parser would pin the wrong contract permanently —
it asserts "the strict parser must agree with the lenient one on everything ParseFloat accepts",
which would make refusing `NaN` a test failure, and it would resurrect in the test tree the exact
one-quantity-two-implementations failure the diff removes from the source tree.

## Did the defect ever fire?

**No, and this is settled rather than assumed.** Both reviewers reached it by different evidence.

A fallback arm is not merely close to shipped: `ladder()` short-circuits on `scale == 1` returning
`base`, so a fallback arm is the shipped scoring path instruction-for-instruction. It can only
produce zero difference from shipped. The fingerprint of the defect having fired is therefore an arm
reading *exactly* the shipped arm.

The clean re-run behind the current wording (`337f83d`, 2026-08-07) reads, and I confirmed this
directly with `git show 337f83d:CLAUDE.md`:

| scale | defence | attack |
|---|---|---|
| 0 | 2172 | 2127 |
| 0.5 | 2158 | 2129 |
| **1.0 (ships)** | **2152** | **2152** |
| 1.5 | 2178 | 2164 |
| 2.0 | 2117 | 2122 |

Eight of eight non-default arms differ from shipped, and no two share a value. The earlier run
(`46e4342`, 2026-08-06 — the commit that introduced both the vars and the lenient parser, so the
first possible exposure) shows the same, with a monotone five-point attacking ladder that a silent
fallback cannot generate. The mediator agrees: `metrics.go` reads both vars at four rungs each
through `ladder`.

`fpl-stats-review` confirmed **nothing is banked for either knob**: no `stats/snapshots/*/cells/`
arm names either variable, and none could — sweep provenance stamping arrives at `11cabfc`
(2026-08-09), two days after both ladder runs. The near-miss to disarm is `FIXW#1`, which is the
config field `fixture_weight` clamped to [0,1], not these vars, and never touches this parser.

One honest caveat, from `fpl-findings-audit`: `Optimize` was not run-to-run deterministic until
`9e5e1d1` (2026-08-10), so strictly each arm at that date was an independent draw rather than a
guaranteed reproduction. Eight independent draws of one configuration do not land on eight distinct
values 20-60 points apart. Read the conclusion as overwhelming rather than airtight.

## Findings applied

1. **The new test did not pin the DEFAULT, and the change had just doubled the surface for getting
   it wrong.** `fpl-code-review`'s strongest finding, and demonstrated rather than supposed: it
   mutated only the attacking default to `envScaleStrict("FPL_ATK_FIXTURE_SCALE", 0)` — flattening
   1.30/1.15/0.85/0.72 to a flat 1.00 in every command and every replayed cell, forever — and the
   whole `internal/analysis` package stayed green, including the new test. The shipped `1` used to
   be a single `return 1` shared by both knobs; it is now a per-call-site argument written twice.
   The defensive ladder is covered by `TestConcededPenaltyScalesWithFixtures`; the attacking one had
   no liveness guard anywhere. **Applied**: an unset arm asserting `atk=1 def=1`, verified to fail
   under exactly that mutation.
2. **My own comment claimed `envScaleStrict` was "the only scale parser in this file", which is
   false.** Raised by `fpl-stats-review`. `envDefaultAbove`, `envDefault`, `envOpt` and
   `envScaleMap` are all still lenient and all serve fingerprinted knobs. **Applied**: the comment
   now says it is the only *strict* parser, enumerates the lenient ones, and names the two that are
   worse than the defect just fixed because they silently refuse a *meaningful* setting.
3. **The test could pass vacuously on an ambient marker.** `runChild` scrubbed the two
   `FPL_*_FIXTURE_SCALE` entries but not the child-selection variable, so an operator with
   `ANALYSIS_FIXTURE_SCALE_CHILD` exported would have put the *parent* into the child branch, where
   it prints and returns having asserted nothing. **Applied, and more than patched**: child
   selection moved out of the environment entirely to a command-line argument. A test whose subject
   is an environment variable should not take its own control signal from the environment.
   `fpl-code-review` separately verified there was no fork bomb — `os/exec` keeps the last duplicate
   key in `cmd.Env`, so the appended marker always beat an ambient one.
4. **A substring assertion that accepted the wrong value.** `strings.Contains(out, "def=0")` also
   matches `def=0.5`. Weak rather than wrong, in a test whose whole subject is a value silently
   becoming something else. **Applied**: `hasScales` matches the reported line exactly.
5. **`docs/replay.md:398` was silent about the refusal**, where the `FPL_SWEEP_STARTS` row three
   pages up already carries the sentence this change earns. **Applied**, in that row's own voice.

## Findings declined, with reasons

1. **Refuse non-finite input (`NaN`, `±Inf`) in `envScaleStrict`.** `fpl-code-review` is right that
   `err != nil || v < 0` does not reject them, that `FPL_ATK_FIXTURE_SCALE=NaN` propagates `NaN`
   through `attackMultiplier`, and that `normaliseBenchSlots` 260 lines above already refuses
   non-finite components with an argument that transfers verbatim. **Declined here** on three
   grounds. It is inherited, not introduced — the lenient parser accepted `NaN` and `Inf`
   identically, so refusing them would break the byte-identity claim this change rests on. It is
   provenance-*honest*, which is a materially different defect: the fingerprint stamps `"NaN"`, so
   nothing lies about what ran, and the trap this change closes is specifically the lying one. And
   `envScaleStrict` also serves `FPL_CS_SCALE`, so the one-line fix silently widens the change to a
   third knob. It is a good next change and should say so in its own commit message. Recorded in
   the comment on `envScaleStrict` so it is not rediscovered.
2. **Converge the other lenient parsers.** `envDefaultAbove` (`FPL_CS_XGC_FACTOR`,
   `FPL_CAPTAIN_SHRINK`, `FPL_BUY_DISCOUNT`, `FPL_BLANK_RUN_PENALTY`, `FPL_BLANK_RUN_MAX`,
   `FPL_MAGNITUDE_ALPHA`), `envDefault` (`FPL_RELIABILITY_SPLIT`), `envOpt`
   (`FPL_MINUTES_WEIGHT`), `envScaleMap` (`FPL_POS_MINUTES_SCALE`), `benchSlotWeights` and
   `appearanceFit` all carry the identical trap on fingerprinted knobs. **Declined**: one mechanism,
   one test, one commit — and `fpl-code-review` agreed with the boundary. Documented in the comment
   rather than left to be rediscovered. Two deserve priority over the rest, because they refuse a
   setting a sweep would actually want: `envDefaultAbove` defaults on `v <= 0`, so
   `FPL_CS_XGC_FACTOR=0` — no clean-sheet exponent at all — reads as shipped; and `envScaleMap`
   drops an unparseable entry while applying its siblings, so `MID=0.75,FWD:0.5` silently applies
   half a position map.
3. **Two `CLAUDE.md` edits proposed by `fpl-findings-audit`, both genuine, both out of scope for
   this branch.** Recorded here so they are not lost, and neither is a consequence of the defect:
   - **A sign inversion inside a closed line.** `CLAUDE.md` says twice that "zeroing the defensive
     response entirely **costs** 20 points". The clean re-run table has scale 0 at **2172** against
     shipped **2152** — zeroing *gains* 20 on that table. The word is inherited from the pre-fix
     run (`46e4342`, where zeroing lost 6) and survived the re-run that reversed it. It is
     load-bearing because it sits inside the gloss "never won any, at any setting tried". I
     confirmed this myself with `git show 337f83d:CLAUDE.md`; it is settled by reading and needs no
     run.
   - **"Both span less than the noise" is not what the source says.** The source paragraph states
     the defensive ladder "spans 61 points across a fourfold change in width" and that both are
     "inside the ±50 noise" — 61 > 50, and the compaction resolved the contradiction in the
     direction the numbers do not support. The figure also carries no cell count: it is three
     seasons at one entry point on absolute totals, five values per ladder, below the twelve-cell
     bar this file sets and against its own rule to judge on paired differences. `fpl-stats-review`
     reached the same verdict independently — the ladders are *unmeasured on the current grid*
     rather than a measured null, and the `FPL_DEF_FIXTURE_SCALE` points arm stays closed on the
     clean-sheet canary (~4× below detection, banked at
     `stats/snapshots/2026-08-15-clean-sheet-2x2/`) rather than on that table.

     **Declined here** because a code-defect branch is the wrong vehicle for editing the research
     record, and because the second one changes what a closed line rests on. Both are owed their own
     change.
4. **Any `CLAUDE.md` edit on account of the defect itself.** Both `fpl-findings-audit` and
   `fpl-stats-review` reached "no edit owed", independently, and I agree. The defect never fired;
   recording a contamination that did not occur is worse than silence, because the next reader
   discounts a verdict that is clean. The generic lesson is already resident twice — "A
   byte-identical result is not a tie" and "Check that a setting is read on the path you are about
   to score". The prospective warning belongs in the code comment beside the test, which is where it
   is.
5. **A "Things that have already bitten" entry.** Proposed and rejected by `fpl-findings-audit` for
   the same reason, and I concur: that section is bugs that fired and cost recorded points.

## The snapshot this record also covers

`stats/snapshots/2026-08-16-01ef5b7` is inside this key, because `stats` is on the review watch
list as well as the snapshot one. It was **not** separately reviewed and does not need to be: it is
mechanically generated output, and no reviewer judgement went into it.

It is worth recording what it says, because it confirms this change's central claim from a
direction the change itself could not reach. **All 560 figures are identical to the preceding
snapshot except `stamp.commit`, and `constants.csv` is byte-identical.** The 555 model rows were
freshly computed — the CSV was deleted first, written to a path nobody else uses, passed to both
halves, and run under `-count=1`, which is the complete set of traps the note on
`TestSnapshotCoversTheCurrentCode` warns about, all of which have produced a falsely-identical
snapshot here before. So "nothing moved" is a measurement rather than an artefact, and it is
independent of the parser differential: that compared two parsers on 200,034 inputs, this recomputes
the model's own accuracy figures end to end and finds no digit changed.

## What could not be checked on this harness

- **Whether the ladders have a shape.** Nothing on disk can distinguish "no shape" from "too small
  a sweep to see one". The arms demonstrably *ran*; the resolution question is separate and open.
  No re-run is proposed here.
- **How far a `NaN` scale propagates through `Optimize`.** `fpl-code-review` marked this PLAUSIBLE
  and explicitly did not trace it. It bears on declined finding 1 and should be traced by whoever
  takes it.
- **The 200,034-input differential is not reproducible from this checkout**, by design (see above).
  The `TrimSpace`-implies-old-fallback argument is the half that is, and it is recorded in the
  commit message and in the comment on `envScaleStrict`.
- One claim in `internal/backtest/harness_test.go:229` — that the defensive half of
  `FPL_DEF_FIXTURE_SCALE` was swept under the pre-repair xGC dilution — looks too strong, since the
  sweep ran on three seasons that all carry native xGC. Noticed by `fpl-findings-audit`, not acted
  on, not part of this change.
