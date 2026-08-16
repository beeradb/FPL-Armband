# The third pass over `docs/replay.md`, and the code comments that keep re-infecting it

## What was reviewed

`docs/replay.md` after it had absorbed **three** independent correction passes from sessions that
did not know about each other, plus two merge resolutions. Plus `stats/README.md`'s schema table,
which this page routes readers to.

## The two findings that change what a reader believes about a measurement

**`FPL_NO_FIXTURE_LOAD` is `HOLD`-inert by construction, and the row said nothing about it.**
`Engine.FixtureLoadInScore()` gates on `fixtureLoadWeeklyOnly` (ships `true`) and a horizon-1
engine. The replay builds one in exactly two places, both requiring `SimConfig.WeeklyXI` or a free
hit; `HoldCaptaincyWeekly` builds every weekly engine at the configured horizon, and shipped
`fixture_horizon` is 5. **So at shipped config the switch is byte-identical on `HOLD` always**, and
on `POLICY` too for any arm leaving `WeeklyXI` false — which `sweepConfig` does unless the block
sets it. This page's own closing section is about a plausible number that measured nothing, and
this row was a route into it: set the switch, get a clean tight null, conclude "doubles do not
matter". The row now names the mediator. The old wording also invited a second misreading — that
the replay stops *paying* the second fixture. It does not; this is a term on `Score`.

**The `hold_xpoints` row stated "|t| goes down" flatly, and its own cited file disagrees.** On arm
levels the pilot splits **two down, two up** (flat −0.08 → −0.15, half-life 2 −1.37 → −1.84 both
*rise*). Where |t| falls six of six is the **between-arm contrast** block. The verdict survives —
never quote a `hold_xpoints` threshold as if power improved — but the population had to be named,
because a reader checking a single arm would find a counter-example and discard the whole warning.
⚠️ **A prior review record had this qualifier and it did not reach the page** — a true, unique fact
lost between a review and the document it reviewed, which is the failure mode this pass was briefed
to hunt.

## Applied

- Both of the above.
- **`TestDiagGateXPoints` does not exist** — it is `TestDiagGateOracleOnXPoints`. A reader
  following the citation finds nothing.
- **`pick.want` does not exist either** — every call site uses a local alias, so
  `grep "pick.want"` returns empty, which on this page reads as "no blocks". ⚠️ **I wrote that
  line**, in the previous pass, while restoring a verification method. Restoring a method and
  getting the identifier wrong is worse than leaving it out. Now `want("…")`, and the reviewer
  re-ran the enumeration: the block table is correct at this commit.
- **`FPL_WEIGHT` has an inert key on the command it documents.** `prior_half_life` is a no-op for
  `fplagent backtest`, warns on stderr, and is pinned by
  `TestPriorHalfLifeSaysItCannotReachTheReplay`. A byte-identical run from a knob you set.
- **An unsourced multiplier retired.** "Understated a real concurrent run by about sevenfold" has
  no pairing that produces seven on this page's own numbers; the only 7 available is the
  driver-to-binary ratio stated two paragraphs earlier, which looks like the number that got
  reused. Replaced with the checkable comparison and an instruction to quote the band, not a ratio.
- **Nine of fifty cells columns were documented nowhere** — including `hold_xpoints`/`policy_xpoints`,
  which the page's own new section exists to explain, and `squad_hash`, the mediator `CLAUDE.md`'s
  confinement/liveness rule is quoted on. Added to `stats/README.md`'s schema table.

## Applied outside the docs, deliberately

**Three code comments still described a chip sequence FPL does not permit** — a wildcard "in the
same week as" a bench boost, where `playWildcard` gates on `gw+1`. `internal/backtest/replay.go`,
`internal/backtest/simulate.go` (eleven lines above a *correct* comment saying the opposite), and
`internal/snapshot/fingerprint.go`.

⚠️ **I fixed these rather than handing them on, and the reason is the point.** The doc was corrected
last round; these were not. **That leaves the documentation right and the code comments wrong**,
which is the inverse of the usual direction — so the next pass that checks the page against the
code finds two comments contradicting it and "corrects" the page *back to the wrong thing*. That is
exactly how this error survived two doc passes and a merge already. Leaving them would have been
choosing to be re-infected.

## Declined

- **The switch-lifetime diagram**, for the third time and on the same ground: it is **not
  rendered**, and `TestMermaidBlocksAreWellFormed` is not a renderer, so a green suite is no
  evidence the block draws. What changed is that it is now **fully drafted and its four classes
  traced against source**, so this is a render-and-land job rather than a design one. ⚠️ Recorded
  rather than dropped — I am declining the *risk*, not the idea, and I have now declined it three
  times, which is itself a signal that someone should just render it.
- **Adjudicating which population should headline the `hold_xpoints` claim.** The doc edit names
  the population, which is the documentation fix; whether the contrast block is the right one to
  lead with is `fpl-stats-review`'s call.
- **`scripts/replay:65-67`'s claim that a stray package path "is rejected with a usage dump"** — the
  reviewer built a throwaway test binary and disproved it (exit 0, flags silently discarded).
  `docs/replay.md` is right and the script comment is wrong. Same inversion as the chip comments,
  but a `scripts/` change rather than a comment on the path this pass touched. Handed on.
- **Two stale counts in code comments** — `oracle.go` says "the three oracles" over a `case` listing
  six; `sweep.go` quotes a bench tuple that is not the shipped one. No doc quotes either, so no
  reader is currently misled. Handed on.

## What could not be checked

- **Whether the diagram renders.** Not drafted into the file.
- **The statistical adjudication** above.
- **`FPL_TEAMNEWS_MAX_HOURS` still has no row**, and it changes the *data* an arm replays (228 of
  228 gameweeks unset against 199 at 24 hours). That is a switch class this page treats as
  dangerous enough for a blockquote elsewhere. Left for the next pass and named here so it is not
  rediscovered as novel.
