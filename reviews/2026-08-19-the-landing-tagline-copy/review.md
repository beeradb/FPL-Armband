# The landing page's hero tagline, and the `-update` trap it uncovered

**What was reviewed.** The working tree of `fix-the-hero-tagline-copy` against `origin/main`
`1010e51`, one commit — three user-facing strings in
`internal/webui/assets/pages/landing.html`, two regenerated goldens, and a doc-string change to
`internal/webui/visual_test.go`'s `-update` flag. The review ran over `1010e51..a53892f`; the
commit was subsequently amended to `b55e6a8` to carry the review's corrections, since it had not
been pushed and a wrong justification is better removed than annotated.

**What the change is.** The user reported the hero tagline "see the working" as broken. It was
first read as a *rendering* fault and briefed as one; the user corrected the reading — the
**wording** was the item — and chose the replacement. So the item changed shape mid-flight, and
both the commit message and the review below have to represent that rather than tidy it away.

| line | before | after |
|---|---|---|
| 197 | `See <span class="u">the working.</span>` | `See <span class="u">why.</span>` |
| 7 | `…own rules, with the working shown. Set your eleven…` | `…own rules. See why. Set your eleven…` |
| 324 | `set your eleven with the working in front of you.` | `…with the reasons in front of you.` |

Line 324 — the closing CTA — is the one nobody counted; the user named two instances and there
were three.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| `fpl-docs-review` | **yes** | the change is user-facing written copy, and the paired work note is written record. The reviewer it owns. |
| `fpl-code-review` | **skipped** | its triage row is scoring, the backtest harness, config, or the agent tool layer. This diff touches none: three HTML strings, two PNGs, and a test flag's doc string. |
| `fpl-stats-review` | **skipped** | no measurement, no constant, no claim about the model. |
| `fpl-security-review` | **skipped** | nothing under `internal/fpl`, `internal/agent` or config persistence; no new input reaches the agent loop. |
| `fpl-findings-audit` | **skipped** | `AGENTS.md` and `docs/` unchanged. |

⚠️ **A skip is a decision, so all four are named rather than omitted.** The one that is closest to
owed is `fpl-code-review`: `visual_test.go` is Go, and a Go change with no code reviewer is the
kind of gap this table exists to make visible. It was skipped because the change is a string
constant in a flag description and a comment — no control flow, no expression, nothing the
recorded bug classes can reach.

## Findings, ranked by how misleading the current state was

### 1. The span boundary's justification was measured on the wrong quantity — APPLIED

The commit message and the work note both justified wrapping `why.` alone rather than `See why.`
on a measurement: line 2's `S` at x=38 against line 1's `P` at x=36, called *"a permanent 2px
ragged left edge"*. The note went further and said the finding *"does not need re-deriving"*.

**The figures are right and the conclusion drawn from them is backwards.** x=36/x=38 are **box and
glyph-origin** coordinates. "Ragged left edge" names the **visible ink** edge, and the ink runs the
other way: `S` carries ~1.7px less left side bearing than `P` at 62px, so the 2px padding
**cancels** the bearing instead of adding to it. Measured off rendered PNGs under the harness's own
flags:

| viewport | type | shipped `See <span>why.</span>` (S − P) | rejected `<span>See why.</span>` (S − P) |
|---|---|---|---|
| 1280 | 62px | **−1.74px** (S overhangs) | **+0.26px** (flush) |
| 390 | 35.1px | −0.84px | +1.09px |

`P`'s ink edge is identical in both variants; only `S` moves, by exactly the 2.00px of padding. The
**rejected** candidate was the flush one.

**Applied.** The markup is unchanged — the choice stands on the surviving argument, that the mark
belongs on the payoff word rather than the verb, and the visible difference is under 2px either
way. What changed is the record: the commit message now states the geometric argument **and that
review refuted it**, rather than dropping it. ⚠️ **The generalisation is the part worth keeping: a
glyph-origin displacement is not a visible rag, and a typographic claim measured on box coordinates
has not been measured on what a reader sees.**

### 2. "0px indent at both ends of the clamp" named a state that cannot occur — APPLIED

`.hero h1` is `clamp(38px,5.4vw,62px)`, but `@media (max-width:720px)` overrides it to
`clamp(32px,9vw,42px)`. Above 720px, `5.4vw > 38.88px` — **the 38px floor is never reached in the
shipped page.** The low end was not measured because it could not be. Per the standing rule that a
comparison which could not run is not a null, the wording is now "a 0px span-box indent" with no
claim about clamp ends.

### 3. The `-update` trap was a repo fact living only in a commit message — APPLIED, and it is the finding with the longest reach

`visual_test.go`'s `*update` branch writes each golden and **returns before any comparison runs**,
so `-update` rewrites **every** golden rather than the failing ones. Combined with
`browsertest.NoiseFloor = 32`, a rewrite can bank a render difference the comparison would have
forgiven. Observed during this work: `phone-picker.png` came back modified on a run that changed
only the landing page, and reverting it left the suite green.

This is checkable from a fresh checkout, so by the routing rule it belongs in the repository — but
the flag's doc string said only *"rewrite the layout goldens from what the application currently
renders"*, which warns nobody. **Applied**: the warning is now on the flag's own doc string and in
a comment above it, where somebody running the flag will meet it.

### 4. The verdict "the highlight geometry is measurably wrong" was stronger than the numbers — APPLIED

The geometry was measured and is not in dispute: the `.u` background box is **78.00px** at the 62px
ceiling against a **63.24px** line box, because an inline element's background paints the **content
area**, sized from the font's ascent+descent — `line-height` does not size it. The `62%` stop lands
**15.64px** above the baseline, **46%** up a 34px x-height, with the baseline at **82.05%** of the
box.

**That says where the fill lands, not that it is wrong.** "Wrong" needs an intended target and
nothing in the tree records one: `.hero h1 .u` carries no comment, the idiom appears nowhere else,
and `62%` is the same stop this file's two `.lbg` gradients use. A highlighter-pen effect marks
*through* text by construction. Softened accordingly.

⚠️ **This also refutes the hypothesis that opened the whole item.** The original brief proposed
that `line-height:1.02` made the stop land wrong. It does not — `line-height` does not size the
background box at all.

### 5. The commit subject said "anywhere" and meant the product copy — APPLIED

`docs/configuration.md:169` still reads "Full working in [model.md]". It is reference prose rather
than product voice and is deliberately left; the subject now says "in the product copy" and the
message names the docs instance.

### 6. Two elements are both called "the tagline" — APPLIED as one clause

`landing.html:187` and `:320` carry `<span class="tagline">Your man on the pitch.</span>`, and the
CSS comment says it *"sits with the mark, never with the H1 — they do different jobs"*. The user,
and therefore both records, call the **h1** the tagline. No change needed; one clause added so the
next reader is not confused about which element the repo means.

## Declined

**Nothing was declined.** Every finding was applied, which is itself worth flagging — this gate's
own rule is that a review finding nothing is not being run properly, and the inverse deserves the
same suspicion. The honest reading is that findings 1, 2 and 4 are all the same mistake (a number
that was real but did not measure the claimed quantity) surfacing in three places, and that one
mistake was mine.

## What could not be checked on this harness

- **The `phone-picker.png` churn narrative.** The *behaviour* is verified in `visual_test.go`; the
  specific run, the revert and the green re-run happened in a scratch state that no longer exists.
  Unreproducible here, so it is recorded as observed rather than as measured.
- **"The old meta description sat on the truncation edge" at 155 characters.** A judgement about
  search-result rendering, not a measurement, and stated as a judgement.
- **The leak scan's first two channels could not run the shipped scanner.** `LEAKSCAN_PATTERN` is a
  CI secret and is unset locally, and `scripts/leakscan` correctly says so and refuses to report a
  pass. Both channels were scanned by hand instead; both are clean, as is the branch name. ⚠️
  **A hand scan is weaker than the real one** — it checks the spellings the operator thought of.

## Owed, and not blocking this merge

The repository still has **no contrast test of any kind**. `internal/webui` guards type *sizes* —
`TestNothingRendersBelowTheTypeFloor` and `TestMobileTypeIsNeverSmallerThanDesktopType` — and both
read `assets/static/armband.css` alone, so neither can see `landing.html`'s own `<style>` block.
That block ships `.mini .arm` at **7px**, two below the floor the system states as absolute. The
floor is broken on the public landing page today and no scan can see it. `visual_test.go`'s own
header volunteers the gap: *"A rendered check is stronger and is owed for CONTRAST, where
compositing and inheritance mean a stylesheet cannot answer the question."*

Not this change's to fix — it moved no colour pairing and no type size — but it is the second
review in two days to arrive at the same absence.
