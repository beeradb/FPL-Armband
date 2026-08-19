<img src="docs/images/fpl-armband-banner.png" alt="FPL Armband — a dark shield carrying circuit traces, crossed by a blue captain's armband marked with a C, beside the wordmark FPL Armband" width="520">

An AI Fantasy Premier League analyst that runs on your machine. The binary is `armband`.

One free command builds the best 15-man squad it can find under the real rules — £100m,
positional quotas, three per club. Another finds the best transfers for the squad you already
own. Neither one touches an AI model: the scoring is a deterministic quantitative engine built
from FPL's actual points rules, reading the public FPL API, and it costs nothing to run. When
you want a second opinion that has actually read the team news, an LLM agent reasons over the
same numbers, searches the web for what the model cannot see, and critiques the model's own
output before it recommends anything. That layer is the only part that costs money, and it is
optional.

*Named for the captaincy, which is the biggest single decision you make each week and the one
effect in this project's record that stands clear of the noise.*

This is the page `armband -html squad.html squad` writes — real output from a live run, not a
mockup. The whole fifteen under the real rules: the eleven and the bench in substitution order,
the captain, what was spent and what is left, and against every player the next five fixtures,
expected minutes, the model's score and the price. The same page serves live over HTTP —
`armband serve` hosts it on 127.0.0.1, where every player in the fifteen carries a lock and a
boot button that write standing overrides — into the browser session by default, back to
config.json only with `-persist` — and the watchlist gains filters, sorting and paging.

![The squad page for gameweek 1, two days before the deadline, from the 2026-08-19 run. A header shows the gameweek, the deadline and free transfers, above three tabs: The eleven, Why this eleven, and Watchlist. Below, the starting eleven in a 3-5-2 — 46.5 XI points per gameweek, Bruno Fernandes as captain, £100.0m spent, £0.0m left — with each row carrying five colour-coded fixture-difficulty pills, expected minutes, the model's score and the price. The four-man bench follows in substitution order.](docs/images/squad-eleven.png)

The shape of the system, and where the split falls: everything up to the agent is deterministic
and free, and the language model is judgement layered on top of numbers it never computes
itself.

```mermaid
flowchart LR
    api["public FPL API<br/>read-only, unauthenticated"]
    engine["quantitative scoring model<br/>deterministic, never calls the LLM"]
    freecmds["free commands<br/>squad · serve · transfers · fixtures · chips · brief"]
    agentloop["LLM agent<br/>review · advise · due"]
    search["web search<br/>team news, press conferences"]
    you["you make the transfers<br/>no authenticated write path back to FPL"]

    api --> engine
    engine --> freecmds
    engine -->|"the same numbers,<br/>plus judgement — API billed"| agentloop
    search --> agentloop
    freecmds --> you
    agentloop --> you

    classDef io fill:#F4E0E3,stroke:#A8404E,color:#141A21
    classDef pure fill:#E3EDF1,stroke:#1F5F73,color:#141A21
    classDef llm fill:#DFEDE6,stroke:#2F7A57,color:#141A21
    classDef muted fill:#F4F6F9,stroke:#7A8791,color:#141A21
    class api,search io
    class engine,freecmds pure
    class agentloop llm
    class you muted
```

## Quick start

```bash
go build -o armband ./cmd/armband

./armband squad          # best 15 it can find under the real rules — free, no AI
./armband transfers      # best transfers for the squad you own — free
./armband fixtures       # fixture difficulty table
./armband brief          # the whole deterministic picture, as Markdown
```

For the AI commands, set credentials once:

```bash
export ANTHROPIC_API_KEY=sk-ant-...     # platform.claude.com/settings/keys
./armband review                        # the weekly decision
```

First run writes a `config.json` you can edit, or copy `config.example.json` over it and set
`entry_id` to your own team. Requires **Go 1.26.5 or newer** (see `go.mod`). Nothing but the AI
commands needs a credential of any kind — the FPL endpoints this uses are public and
unauthenticated.

**Only `entry_id` is required.** Every other key has a shipped default, and a key you leave out
of the file keeps that default — `config.example.json` is deliberately the short version rather
than the full one. The long `config.json` this repository ships is the *model's* configuration:
around forty tunable constants, most of which you should not need to touch and none of which you
should change without reading what it does first.

## Two things to know before your first paid run

**It writes to your config, never to FPL.** There is no authenticated write path at all — FPL's
session cookie, the my-team endpoint and the `auth` command were removed outright, and
`TestTheClientHasNoAuthenticatedSurface` fails the build if one comes back. You make the
transfers.

That config write is worth understanding, because **a review run leaves something behind**. When
the agent establishes something the model cannot see — a player out for six weeks, one who has
lost his place — it records it with `set_player_status`, which persists to `config.json` and
binds every later run, the free commands included: an excluded player is not offered again by
`squad`, `transfers` or `suggest_transfers` until the override is cleared. It refuses to store
one without a reason, and every standing override is re-reported for review each run. See
[docs/configuration.md](docs/configuration.md).

The loop is worth seeing whole — one paid run writes a fact down, and every free run afterwards
is bound by it until it is cleared:

```mermaid
flowchart LR
    reviewrun["armband review<br/>the agent establishes something<br/>the model cannot see"]
    setstatus["set_player_status<br/>refuses to store an override<br/>without a reason"]
    cfg["config.json<br/>standing override"]
    freecmds["squad · transfers · suggest_transfers<br/>the player is not offered again"]
    audit["re-reported for review<br/>on every later run"]

    reviewrun --> setstatus --> cfg
    cfg --> freecmds
    cfg --> audit
    audit -.->|"until cleared"| cfg

    classDef llm fill:#DFEDE6,stroke:#2F7A57,color:#141A21
    classDef pure fill:#E3EDF1,stroke:#1F5F73,color:#141A21
    classDef test fill:#FBF2E3,stroke:#B9770E,color:#141A21
    classDef muted fill:#F4F6F9,stroke:#7A8791,color:#141A21
    class reviewrun,setstatus llm
    class cfg test
    class freecmds pure
    class audit muted
```

## It plays its own picks, in public

[FPL Armband](https://fantasy.premierleague.com/entry/2785902/history) is this project's own
team. Every gameweek it fields what the model says, and the table is there whether that goes
well or badly. A model that only ever scores itself against replays of seasons it was tuned on
is marking its own homework — so the picks go somewhere anyone can check them. It is also the
`entry_id` that `config.example.json` ships with, so change that to your own.

One season is a small sample, and a league table is a slow way to learn anything. The
measurement that *does* resolve is below, against thousands of player-gameweeks. Read the live
team as skin in the game.

## Does it actually work?

A transfer is a question about **order** — will this player out-score that one — not about
hitting a points total. So the number to judge a predictor by is how well it ranks players
within a gameweek.

The squad page's watchlist tab asks that question a hundred times over, and it is worth seeing
how: the best hundred players outside the fifteen form one list by default, filterable by name,
position and club — a filter searches the whole pool, not just the hundred — and sortable on
every column but the fixture strip (price first), with each candidate measured not against the
league but against the weakest starter you already own in that position — named in the legend
above the list — and the Δ column giving the gap, green marking what clears the free-transfer
gate. On this run's data, two of the whole pool do.

![The watchlist tab, from the same 2026-08-19 run: the best hundred players not in the fifteen by default, as one list. The legend above it names the weakest starter already owned in each position — Kinsky for goalkeepers, Thiaw for defenders, E.Le Fée for midfielders, Thiago for forwards — and every row shows the position, the club, the next five fixtures, expected minutes, ownership, the model's score, the gap to that starter, and the price. Two gaps are shown in green; the note above the table says two of the 575 players outside the fifteen clear the free-transfer gate of 0.40 points per gameweek.](docs/images/watchlist.png)

The baselines are not straw men; they are what an FPL manager actually reasons from today.
**Recent form** here is a player's mean score over his last five gameweeks — the same idea as
the form figure FPL itself shows next to every player, and as the rolling averages on every
FPL stats site. The **flat season average** is total points divided by games played. Those are
the alternatives a real person is choosing between when they look at a player and guess.

| predictor | ranks players within a gameweek | over-rates its own top 20 by |
|---|---|---|
| **this model** | **0.427** | **0.41 pts/gw** |
| recent form (last five gameweeks) | 0.330 | 2.57 |
| flat season average | 0.311 | 1.03 |

The first column is a rank correlation: 0 means the ordering is no better than random, 1 means
perfect, higher is better. The model orders players 29% better than recent form. Football
is noisy and a perfect predictor of it is impossible, so read the column as a comparison
between the real alternatives, not as marks out of one.

The second column is the one that costs you money. Every predictor flatters the players it
rates highest, and those are exactly the players you would captain or transfer in. This model
over-rates its own top twenty by 0.41 points a gameweek; recent form over-rates its own by
2.57 — enough to talk you into a captain it has flattered by two and a half points a week.

Drawn to scale, that second column is the whole pitch — the model's edge is how honest it is
about its own favourites:

```mermaid
%%{init: {"themeVariables": {"xyChart": {"plotColorPalette": "#B9762A, #1F5F73, #2F7A57"}}}}%%
xychart-beta
    title "how far each predictor over-rates its own top 20"
    x-axis ["this model", "last 5 gameweeks", "season average"]
    y-axis "points per gameweek" 0 --> 3
    bar [0.41, 2.57, 1.03]
```

That is not the whole story, and the whole story is deliberately frank: on some measures —
picking out the players who go on to haul — the model is only line-ball with the moving
average, and most of its internal constants cannot be shown to be optimal. The full tables,
the unflattering columns included, are in [docs/accuracy.md](docs/accuracy.md). It answers
whether the model is right about **football**; whether a change to it earns **points** is
[docs/replay.md](docs/replay.md)'s question, and it resolves far less often.

## Commands

Most of these cost nothing to run. `internal/analysis` never calls the LLM, so the
optimiser, the fixture model, the chip validator and the replay are all free.

| Command | What it does | API cost |
|---|---|---|
| `brief` | The whole deterministic picture as one Markdown document, for pasting into a chat (`-html` for a page) | — |
| `squad` | Best 15 it can find under the real constraints: £100m, positional quotas, three per club (`-plain`, `-html`) | — |
| `serve` | The squad page over HTTP, loopback only — lock/boot buttons update the page in place (rows slide in and out), the watchlist filters, sorts and pages. Open the printed URL: its token gates the write actions. Overrides live in a browser-session cookie by default; `-persist` writes them to config.json (`-addr`, `-persist` after the command) | — |
| `transfers` | Best transfers for the squad you own, as a team sheet (`-plain`, `-html`) | — |
| `fixtures` | Fixture difficulty per club, easiest run first | — |
| `chips` | Chip windows, plan validation, blanks and doubles | — |
| `congestion` | International breaks, turnarounds, European load | — |
| `nations` | FPL nationality codes with example players | — |
| `backtest <season> [gws]` | Replay a finished season — see [docs/replay.md](docs/replay.md) | — |
| `snapshot` | Model-and-harness accuracy record from a sweep's CSVs | — |
| `capture` | Archive today's live payload, dated and immutable | — |
| `priors` | Cache last season's totals before FPL overwrites them | — |
| `verify-competitions` | Record that competition status was checked, with nothing to correct | — |
| `schedule` | Print a crontab line for `due` | — |
| `review` | Weekly decision: competitions → chips → transfers → news → verdict | yes |
| `advise` | Full pre-deadline analysis, written to a report | yes |
| `due` | Run `advise`, but only if a deadline is near (for cron) — a one-shot prompt, not the `review` protocol | yes, when it fires |

**Flags go before the command** — `armband -html squad.html squad`, never
`armband squad -html squad.html`, which is rejected rather than silently ignored.
`armband` with no arguments prints every flag, with worked examples of the ones
whose ordering is easy to get wrong; that string is the only complete list. A few
commands parse their own flags and take them after the command instead — the
usage text names which, and the same before-the-positionals rule applies there.

**`capture` is the one worth putting on a timer.** It archives `bootstrap-static` and
`fixtures` into a dated directory, two requests, so that point-in-time questions become
answerable later. It yields nothing this season and a great deal next spring — and every
week missed is data that cannot be recovered. `armband capture -list` audits the series
for gaps.

## Two ways to run the review

`internal/analysis` never calls the LLM, so the only thing the API ever pays for is the
judgement on top. That split gives you a choice.

**`armband brief` — free.** Writes everything the model knows as one self-contained
Markdown document: your criteria and review policy, the numbered review protocol, competition
status, chip plan, squad and transfers, every flagged player, your standing overrides, research
targets, the best squad it found, the fixture table, and a "what this model cannot see" section. Paste
it into any chat assistant you already have a subscription for, and get the review there.
Roughly 5,000 words pre-season, and longer once you have a squad to describe.

**`armband review` — API billed.** The same protocol, but the agent calls tools
iteratively: it can notice something odd and go look, rather than reasoning from a single
snapshot. `advise` is the same agent loop on a one-shot prompt, and `due` runs *that* unattended
on a schedule; what it will never do is submit anything, because there is no authenticated write
path and its absence is what the client's guard test exists to keep.

Use `brief` while you are still vetting the model's judgement, since it costs nothing to run
twenty times. Use `review` when you want it to work on its own.

> A chat subscription **cannot** fund `review`. Consumer plans cover the chat products;
> `armband` calls the Messages API directly and bills an API organization. `brief` exists
> precisely so the subscription you have is usable here.

---

## What makes it more than a spreadsheet

**The score is auditable.** Expected goals become points at the real position-dependent
rates; clean-sheet probability comes from expected goals conceded via a Poisson model. No
black-box rating — you can trace any number to the term that caused it, and every term is
stored separately and reported to the agent rather than folded into one opaque figure.

**FPL's step functions are modelled as steps.** Appearance points and the clean sheet are
paid at sixty minutes and are all-or-nothing, so they scale by the *probability a player
reaches sixty*, not by the fraction of a match he plays. Those two terms are 61% of a
defender's per-90 score, so treating them as rates — which the model did once — is a large
and position-dependent error.

**Expected minutes are treated as a gate, not a tiebreaker.** Correlation between season
minutes and season points is **r = 0.929**. Rotation risk is penalised convexly, weighted
toward recent gameweeks, with a per-position relaxation for midfielders.

**Uncertainty is priced where it was measured, and switched off where it was not.** Two
discounts survive because the data supports them, and they deliberately act on different
channels. The summer-signing discount multiplies `Score` by 0.88: signings turn out to be
marginally better per 90 but about 12% less available, so the discount is calibrated against
that availability gap rather than applied to minutes directly. The post-tournament rest
discount multiplies expected **minutes** by 0.83, because minutes are where its effect was
measured. Applying a discount on the wrong channel is what retired the new-manager penalty —
§6 of [docs/model.md](docs/model.md) tells that story.

The penalties that did not survive measurement ship at 1.00, the neutral value. The
new-manager penalty and five of the eight congestion penalties — the three European ones and
both short-rest ones — measured as nothing on the channel they are applied to. The remaining
three, covering domestic cups, long-haul travel and the week after an international break, sit
at 1.00 on the weaker but sufficient argument that an unmeasured multiplier which moves a score
is not neutral and 1.00 is.
All nine are still reported to the agent so a human can weigh them; none changes a number.
§5 and §6 of [docs/model.md](docs/model.md) record what each was measured at, and which
were not.

**The chip plan feeds squad construction.** A wildcard at GW6 shortens the optimiser's
horizon; a bench boost makes the optimiser buy fifteen playable footballers instead of eleven
plus fodder.

**Claims are validated against replayed seasons, not asserted.** Six archived seasons entered
at six deadlines give 36 matched cells, and a change is judged on the paired difference within
each cell rather than on a season total. Most of what that harness has measured is a *negative*
result — five separate corrections of genuinely measured biases lost points. See
[docs/replay.md](docs/replay.md).

**The agent can see what the model cannot.** Web search runs server-side, so it reads actual
team news and press conferences rather than relying on FPL's terse `news` field.

**Every run reports its own cost.**

```
14 API calls · 952k prompt tokens (900k cached, 95% hit rate) · 25k output · 3 web searches
estimated cost: $0.540 at introductory rates through 2026-08-31; $0.810 after
without prompt caching this run would have cost about $5.39
```

---

## Cost

Prompt caching is on by default with two breakpoints — the system prompt plus tool
definitions, and the conversation tail. Cache reads bill at 0.1× input, so a healthy run
shows a hit rate above 0.8. **A hit rate near zero means something in the prefix is changing
between calls** — that is a bug, not a tuning problem.

Context editing clears stale tool results once more than four tool calls are behind.

Levers, roughly in order of impact:

| Lever | Change | Effect |
|---|---|---|
| `effort` | `high` → `medium` | Largest single saving |
| `model` | `claude-opus-5` → `claude-sonnet-5` | $5/$25 → $3/$15 per MTok |
| Free commands | `brief`, `squad`, `transfers`, `fixtures`, `chips` | Zero cost |
| `max_iterations` | Lower than the default 25 | Bounds a runaway, not typical spend |

Pricing is date-aware, so promotional rates are applied while valid and revert automatically.

---

## Pre-season behaviour

Between seasons the FPL API's aggregate stats still hold **last season's** totals. That is the
correct baseline for gameweek 1, and the agent is told so explicitly — along with the
caveats: players who changed clubs show their old club's output, new signings from abroad show
nothing, and set-piece hierarchies may have changed.

This matters more than it sounds. Those aggregates reset at GW1 and then accumulate, so
anything that divides by a fixed 38-game season reports an ever-present as a fringe player for
most of the autumn. The model uses `Engine.DataWindow()` for exactly that reason.

## What you must configure by hand

The FPL API does not publish these, so they are hand-maintained and **must be re-derived every
summer**. They fail *silently*: a stale list simply applies to the wrong clubs, or stops
applying at all.

- **European and domestic cup participation** — dated windows per club
- **Managerial changes** (`new_coach_clubs`) — ten clubs shipped for 2026/27
- **Nationality groups** for travel load — run `armband nations` to map the opaque codes
- **Post-tournament rest** (`rest_players`) — the 17 Premier League players who were **on the
  pitch for the majority of a 2026 World Cup semi-final**. That is a teamsheet test, not a
  squad list: unused substitutes are excluded on purpose, so an audit against published squads
  will "find" names missing and be wrong. Read the comment on `DefaultRestPlayers` before
  editing the JSON, which has no room for one.

Three of the four lists are display-only. All eight congestion penalties ship at 1.00, and the
new-manager penalty with them, so a stale competition window, nationality group or manager list can
no longer mis-*score* a player — only mis-inform the agent reading it. `armband congestion` prints
what was derived from the calendar against what is still missing, and how old it is.

**`rest_players` is the exception, and it is live.** It multiplies expected minutes inside
`blendFor`, so a misspelt name there is a mis-scored player, not a cosmetic error — and it bites
in GW1 and GW2, the two gameweeks straight after the summer maintenance that was supposed to
have checked it.

## Known limitations

Worth knowing before you trust a recommendation:

- **Free transfers are reconstructed**, not published by FPL. Close, but verify against the
  site if a decision turns on the exact number.
- **Selling prices are reconstructed too, and that one is checkable.** FPL pays you what you
  paid plus half of any rise, so a squad cannot be sold for its market value. The
  reconstruction is proved right when it sums to FPL's own published team value, and reported
  as an assumption when it does not.
- **The model cannot distinguish injury absence from being dropped.** Both look like low
  expected minutes. This is the largest measured gap in the system and the reason the agent
  layer exists.
- **The replay cannot test judgement.** Team news is worthless in hindsight, so a backtest
  measures the floor the model provides rather than the system's actual output.
- **Not every constant in the model is provably the best one.** Seasons, not gameweeks, are the
  scarce axis, and six seasons of football is a small sample for questions at that resolution.
  [docs/replay.md](docs/replay.md) is candid about which comparisons the harness can settle and
  which it can't.

The tool says this to your face rather than leaving it in the documentation. The squad page's
"Why this eleven" tab names where the model is structurally blind that week — sides whose
defensive numbers were earned with a player who is now out, arrivals it scores 0.00 because
there are no Premier League minutes to read — and, for each group, the question worth going
and asking.

![The "Why this eleven" tab. Under "Standing overrides": no standing overrides, this squad is the model's unaided answer. Under "Where the model says it is blind", two worked groups: "Defences whose numbers no longer describe the side" lists five injured or unavailable defenders whose clubs' expected goals conceded was earned with them in the team, with the question "how long is he out, who replaces him, and should every defensive asset at that club be marked down?"; "No Premier League data — scored 0.00 regardless of role" lists five players from promoted clubs or overseas, with the question "is he in the predicted XI? If so he is badly underrated and worth a squad slot."](docs/images/why-this-eleven.png)

## Documentation

Everything in `docs/` is reference — a description of the system as the code stands, nothing
speculative. Seven documents and an index:

| | |
|---|---|
| **[docs/accuracy.md](docs/accuracy.md)** | How well it actually predicts, measured against a five-game average and a season average. **Start here if you want to know whether it works.** |
| **[docs/model.md](docs/model.md)** | How players are scored: underlying stats, fixtures, expected minutes, congestion, role risk. The intellectual core. |
| **[docs/configuration.md](docs/configuration.md)** | Every config field, what it does, and what you must fill in by hand. |
| **[docs/workflow.md](docs/workflow.md)** | The weekly review protocol and chip strategy. |
| **[docs/architecture.md](docs/architecture.md)** | Code layout, data flow, SDK gotchas, how to extend it. |
| **[docs/replay.md](docs/replay.md)** | The backtest harness: how a scoring change is validated, and what it can and cannot resolve. |
| **[docs/backfill.md](docs/backfill.md)** | Recovering historical team news from the Internet Archive, and the one rule that must not be got wrong. |
| **[docs/README.md](docs/README.md)** | The map: what each document covers and how they fit together. |

## Testing

```bash
go build ./... && go vet ./... && go test ./...
```

Tests hit the live FPL API and skip cleanly when unreachable. They assert **invariants and
regressions**, not exact values, since the underlying data changes weekly — a test pinned to a
specific player or score rots within days.

Each regression test encodes a bug that was actually shipped; see
[docs/architecture.md](docs/architecture.md#testing) for a representative five and the
optimiser's three guards. The replay's own diagnostics are behind `DIAG=1` and do not run in
the ordinary suite — [docs/replay.md](docs/replay.md) covers those.
