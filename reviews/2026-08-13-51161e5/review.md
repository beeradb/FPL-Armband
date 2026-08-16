# Review: the starts harvest as an invariance, and the rank closure's field-shape scope

**Commit reviewed:** `51161e5`, on `starts-invariance-and-rank-scope` off `9317f70`. Fixes
applied in the follow-up commit this record is named beside.

**What changed.** Documentation only — no Go, no sweeps, no new measurement. `TODO.md` closes the
starts-harvest item; `CLAUDE.md`'s season table gains the harvest's invariance; a new section in
`docs/notes/archive-and-data.md` records the harvest; `docs/notes/harness-and-inference.md` and
`docs/rank-objective-handoff.md` gain the field-shape scope on the rank-objective closure.

## Reviewers run, and the triage

| reviewer | why |
|---|---|
| **fpl-stats-review** | the session ran as this reviewer; two claims of a null and a distributional scope argument |
| **fpl-findings-audit** | `CLAUDE.md`, two notes and `TODO.md` all gained claims |

Skipped: **fpl-code-review** — nothing in `internal/` or `cmd/` changed; `go build` and `go vet`
pass and the diff is markdown. **fpl-security-review**, **fpl-run-review**,
**fpl-season-maintenance** — nothing in scope.

⚠️ **This is a self-review and should be read as the weaker instrument.** The session had no
`Task` tool, so `fpl-findings-audit` could not be *dispatched* as an independent reviewer; its
criteria were applied by the same agent that wrote the text under review. That is the arrangement
this record elsewhere calls out — a diagnostic must not carry its own copy of the thing it is
checking — and it applies here. **An independent dispatch of `fpl-findings-audit` over this commit
is still owed.**

## Verification performed

Every quoted figure was checked against source rather than against the document it was copied from,
**except where the source column below says otherwise**. ⚠️ **That exception is where this review
failed** — see the independent pass below.

| claim | source | result |
|---|---|---|
| 36,429 starts across 44,512 rows | summed `internal/backtest/repairdata/*-starts.csv` | **exact**: 10245+10543+9905+9783+4036 = 44,512 rows, 8360+8360+8360+8357+2992 = 36,429 starts |
| the commit message's "44,514 recorded starts" is wrong | same | **confirmed** — it is the row count, off by two, and neither number is the other |
| byte-identical across the two arms | `cut -d, -f3-` on `starts-on.csv` / `starts-off.csv` | **identical**, 36 cells |
| the two arms were genuinely different runs | `run_id` column | **distinct**: `1786636994-582197` against `1786637150-582899` |
| the off arm really had the switch set | `starts-off.provenance.csv` | **confirmed**: `env,FPL_NO_STARTS_REPAIR,1` |
| "10 populated outcome columns" | column census of `starts-on.csv` | **correct**, and now scoped — see finding 1 |
| `reliabilityMinutesShare` ships at 1.0 | `internal/analysis/sweep.go:122` | `envDefault("FPL_RELIABILITY_SPLIT", 1.0)` |
| `unifiedAppearance` defaults on | `internal/analysis/appearance.go:393` | `os.Getenv("FPL_NO_UNIFIED_APPEARANCE") == ""` |
| 2.36% / 24.5% / 7.7% | `internal/backtest/startsrepair.go:19-25` | verbatim |
| `HOLD` 7157 against `POLICY` 8550 are the **blind** columns | harness note's omniscience-control table | ⚠️ **checked against the document, not against cells.** The arithmetic is right (1789.25 / 2137.5) and the data state was never checked: the current cells give **7012 / 8713**. Withdrawn — see independent finding 1 |
| `no hits ever` = +0.041 pts/gw, `|t|` 0.22 | harness note line 2882 | verbatim |
| σ_F anchors, 60 specifications, −66 to +564, 2,106/1,525/3,564, 1,900-2,100 | harness note lines 3003-3170 | all verbatim |

## Findings, ranked

### 1. "Byte-identical on all 10 outcome columns" was true and unscoped — FIXED

The census returns 27 columns: 10 identifier/config, 10 populated outcomes, and **7 outcome columns
empty or `-` in both arms** (`frozen_points`, `frozen_captain_*`, `weekly_*`, `oracle`). So the
measured invariance covers what `TestDiagBaseline` populates, and the captaincy rungs are covered by
the *structural* argument only. That distinction matters here more than usual, because the whole
value of this null is that it is structural rather than statistical — quoting it as though the
measurement swept every column would overstate the one property being claimed for it. Scoped in
`docs/notes/archive-and-data.md`. `CLAUDE.md`'s one-line version is left as-is: "10 outcome
columns" is an accurate count and the note carries the caveat.

### 2. `HOLD` is an upper bound on a dormant entry, and the text read as a placement — FIXED

The dormancy bullet put the dropout mass "a couple of hundred points below the field average" on
the strength of `HOLD` at ~1,789 against a field bracketed 1,900-2,100. But `HOLD`'s fifteen is
**optimiser-built and re-picks its captain every week**, where a real quitter froze a hand-picked
squad and never touches the armband. A genuine dormant entry therefore sits *below* `HOLD` by an
unmeasured amount, so the recorded gap is a **floor on the gap**, not a location for the mass. The
argument it supports — that the mass is a shoulder rather than a spike at zero — survives, because
a floor is the direction that argument needs; but the number must not be read as a placement.
Qualified in `docs/notes/harness-and-inference.md`.

### 3. The session's own first recommendation was wrong, and the record says so — NO ACTION

Before checking the tree, this session proposed adding `FPL_NO_STARTS_REPAIR` to `CLAUDE.md`'s
data-state reproduction block as a third switch. The measurement refutes it: the repair is
byte-identical at shipped config, so it is not a data state, and documenting it as one would have
told every future sweeper to control for something inert. The retraction is recorded in the commit
message and the CLAUDE.md row now says "**do not** add it beside the xG switches above". Recorded
here because the failure mode — proposing a reproduction switch from reading a harvest's size
rather than its consumers — is the one this project keeps meeting.

### 4. A stale PRIORITY item nearly bought a second harvest — FIXED, and worth generalising

`TODO.md:1959` carried the harvest as open, PRIORITY, with "the cache is currently empty so this is
a full re-fetch, rate-limited" — while `startsrepair.go`, five `repairdata/*-starts.csv` files, six
tests and three commits were already in the tree. The item is closed with that noted. The general
form belongs beside the standing rule about greping before claiming absence: **check the tree before
pricing an open item on this list**, because a queue entry ages against work it does not observe.

## What is NOT established by any of this

- Nothing here re-derives a constant, and **nothing on the re-derivation queue is downstream of the
  harvest**. TODO 1914 was never blocked by it and still stands.
- The harvest's *value* is untested. Its channels — the lineups and minutes oracles, the
  diagnostics, the agent-facing field, the start/substitute/unused multinomial — are all unmeasured
  here, and the note says so.
- The skew hypothesis on the rank closure is **queued, not answered**. Item 5 of "What is still
  left" in both documents states the pre-registered predictions.

## Independent review, dispatched 2026-08-13

The ⚠️ above said an independent dispatch was owed. It was run: `fpl-findings-audit` and
`fpl-stats-review` were each dispatched **as separate `claude -p --agent` processes** with fresh
context and read-only tools, pointed at `9317f70..HEAD`. That is a genuine independent pass, not a
second reading by the author.

**They found twelve and three findings respectively, converging on the same root cause, and two of
them land on text this self-review had certified.** The corrections are applied in the commit this
section is committed with.

### What the independent pass caught that this one did not

| # | finding | disposition |
|---|---|---|
| 1 | **`HOLD` 7157 / `POLICY` 8550 are a superseded data state.** This branch's own snapshot gives **7012 / 8713** on the shipped archive — moving in *opposite* directions, each by more than the gap the passage argued over. `xgc7-off.csv` reproduces the 7157 | **numbers cut entirely**; the dormancy claim now stands on mechanism |
| 2 | **"No recorded figure moves" is false.** `lineupsoracle.go:221` and `minutesoracle.go:306` read `Starts` directly; both arms ran `oracle=none`, so the measurement provably did not cover that path. `OracleLineups` ≈73 and `OracleMinutes` ≈47 are **unmeasured, not unchanged** | corrected, and named as the one thing genuinely downstream of the harvest |
| 3 | **"All 36 cells" overstates the live sample by half.** `repairdata/` stops at 2022-23, so the 12 cells replaying 2024-25 and 2025-26 could not have moved. The CLAUDE.md edit landed *inside the table* whose header is "a byte-identical season under an intervention is not a tie" | **read the 36 as 24**, in both places |
| 4 | The `z_F` bound was arithmetically impossible (`3.84 > 3.1`), asserted a split the same file records as *not recorded*, and covered the interval where a mixture **cannot** act while staying silent on the one where it can | **withdrawn**, with the mixture-inside-the-box bound beside it |
| 5 | `11.52m` quoted bare and "stable", which the same file settled as forbidden — it is a series | quoted with season and gameweek |
| 6-12 | the handoff left disagreeing with the note it points at; "pre-registered … two reviewers independently" contradicting this record's own self-review admission; the captaincy caveat *stronger* than the facts (`hold_fixedcap`/`hold_nocap` are populated and identical); 2018-19 missing from the after-load identity; "nothing reads `Starts`" true of the replay but not the live agent path; the run being `5f8c29d` + `dirty`, 29 columns against 33 | all applied |

### The root cause, and it belongs in the template

Both reviewers named the same thing independently: **a figure was verified verbatim and the clause
built on it was not.** This review's table checked that 7157 was correctly copied and never asked
what data state produced it; it checked that `z_F` = −0.45 to +3.84 was correctly copied and never
asked whether "all but the lowest are inside 1.4-3.1" follows from it. It does not.

> **For every quoted figure, check the sentence it is used in against the sentence it came from,
> not just the digits.** A verification table structured to catch transcription will catch
> transcription and nothing else.

### What the independent pass confirmed

The byte-identity itself, re-derived a third time and sound — and `fpl-stats-review` added an
arrival check this review missed: the `constants_digest` **differs between the arms**
(`745eab0ee9dc` against `67455aaa8835`), because `FPL_NO_STARTS_REPAIR` is in `fingerprint.go`'s env
list, which proves the variable reached the process independently of the provenance line. The
harvest arithmetic (44,512 rows, 36,429 starts), the 8,360 = 38 × 10 × 22 identity and its external
anchor, the Weghorst residual, all three mechanism claims against source, the skew argument's
distribution-free core and its pre-registration, and the variance-aversion inversion. Also the
"44,514" retraction, which both flagged as retraction history to keep verbatim.

**One handoff to a future run**, from `fpl-findings-audit`: re-pricing `OracleLineups` and
`OracleMinutes` on harvested starts. Everything else was settled by reading.
