# The explicit-zero prior guard

Covers `9bba522..HEAD` on `prior-zero-guard` — stopping FPL's `"0.00"` for
statistics it never measured from entering a prior as a measured observation.

## ⚠️ Read this first: this work was committed to `main` unreviewed

`9dbeb39` was committed directly to `main`. That was wrong, it is the rule this
repository does not bend, and it has been undone: `main` is reset to
`origin/main` and the work moved here. The commit was never pushed.

**The review then found a defect worse than the one that commit fixed** —
`internal/agent/tools.go` handing unmeasured zeroes to the model on a path that is
*live today*, where the fixed one is inert until a setting is turned on. So this
is not a procedural note. Merging `9dbeb39` on its own would have shipped a fix
for the smaller half of the bug and left the larger half in place, and the only
thing that surfaced it was running the review.

## ⚠️ No reviewer agent was dispatched, and that is a second deviation

The session had no `Task` tool, so none of the seven agents in `.claude/agents/`
could be invoked. The applicable briefs were read and applied by the session that
wrote the code. That is weaker than an independent agent and it shares the
author's blind spots.

One mitigation is real and one is not. This session's own brief **is**
`fpl-stats-review`, verbatim, so that review is the reviewer's. `fpl-code-review`,
`fpl-findings-audit` and `fpl-security-review` were done out of role.

**Dispatch those three properly before merging.** The findings below are what a
self-review found; they are not evidence that a real one would find nothing — and
the headline finding here is precisely that the first pass missed the worse bug.

## Triage

| reviewer | owed | why |
|---|---|---|
| **fpl-code-review** | **yes** | `internal/analysis` scoring, `internal/fpl`, `internal/agent` |
| **fpl-stats-review** | **yes** | `internal/analysis` — changes what a prior believes |
| **fpl-findings-audit** | **yes** | two source comments asserted things that are false |
| **fpl-security-review** | **yes** | `internal/fpl` and the agent tool layer |
| fpl-run-review | no | no live run, nothing written to `config.json` |
| fpl-season-maintenance | no | none of the four hand-maintained lists touched |

## Invariants first, per the skill

**What must this NOT move? Every shipped figure.** Checked, not asserted: the
snapshot at the tip is **byte-identical on every model figure** to
`2026-08-14-3c2af85`. That is the expected result and it is the point — the fix is
inert at shipped config, because `cmd/fplagent` gates `LoadPriors` on
`prior_half_life > 0` and the archive paths set no flags. **That makes it a
simple-effect null, not a proof of harmlessness**: it says nothing about the
configuration the feature exists for.

**Each guard was verified to fail with its half of the fix reverted**, because a
test that passes because the feature is inert is worse than no test:

| revert | failure |
|---|---|
| wiring in `internal/recent` | `blended xGC of 1.0879 per 90 is below what the measured seasons support` |
| flags ignored in `BlendPriors` | `the unflagged blend loses only 0.0% of the rate` |

## Findings

**1. The agent was being handed the unmeasured zeroes, on a live path.** CONFIRMED,
fixed at `1b7a27b`. `tools.go` returned `sum.HistoryPast` unaltered, so a
centre-half with 3,151 minutes in 2018/19 arrived carrying
`expected_goals_conceded: "0.00"`. Live on every `seasons: true` lookup, and tool
output is replayed on every subsequent API call, so a wrong number is paid for
repeatedly and can anchor a conversation. `pastSeasonsForTool` now **omits** those
keys — a missing key is the one encoding an LLM cannot misread as a measurement,
where a zero or a flag are both still numbers.

**2. `simulate.go` named a consumer that does not exist.** CONFIRMED by grep, fixed
in place. It claimed `internal/priors/adapter.go` reads `history_past`; it does
not and never has, it adapts `internal/priors`' own archive-backed store. "Naming
the consumer is the check" is the standing rule and this named the wrong one.

**3. The same comment claimed "the live path does not do this".** CONFIRMED false
when written — `internal/recent/priors.go` did exactly this. Corrected in place
rather than rewritten, because a confidently wrong comment is the finding.

**4. The snapshot recipe appends rather than truncates.** CONFIRMED, **not fixed
here, and it affects a snapshot already on `main`**. The diagnostics append to
`FPL_MODEL_CSV`, so running the command twice into one path silently doubles the
file. `8c1ba70`'s snapshot was rendered from a 1101-row file containing two runs.
Its model figures were verified byte-identical to its predecessor at the time, so
no figure was corrupted — but that was luck. `staleness_test.go` warns about a
*stale* `/tmp` file and not about an *appended* one. Owed a fix.

**5. The defcon boundary differs between sources.** CONFIRMED, recorded not
reconciled. FPL's API carries `defensive_contribution` from **2024/25**; the
archive's weekly column starts at **2025-26**. CLAUDE.md's season table states the
archive's boundary as though it were the game's. Two sources, two boundaries, and
the API's is wider — which also means the live path can see a season the replay
cannot.

## What was declined, and why

- **Keying on the value rather than the season.** Pope recorded `expected_goals`
  of exactly `0.00` in 2022/23, a season the data fully covers, so a zero test
  would silently discard a real observation for every goalkeeper.
- **Dropping the affected seasons from the blend.** Their minutes, starts, bonus,
  saves and cards are real football. The blended minutes are asserted identical
  between the flagged and unflagged arms for exactly this reason.
- **Flagging the archive paths.** They reach those seasons through the
  expected-goals repair and genuinely do have the figures. The zero value of both
  flags is "this season has everything", so they are untouched by construction.
- **Fixing the replay's defcon prior.** `simulate.go` names it as a defect and
  says the two halves need separate fixes. Out of scope here; the comment now
  says so accurately.

## What could not be checked on this harness

- **Whether the corrected prior is better on points.** It is inert at shipped
  config, and the configuration it acts in — `prior_half_life > 0` — is a setting
  the record says is unresolved and not favourable. Unmeasured, not unmeasurable.
- **Whether the agent reasoned wrongly from the zeroes in practice.** No transcript
  archive; the defect is established by reading the payload, not by observing harm.
