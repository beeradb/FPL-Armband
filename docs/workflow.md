# Weekly workflow and chip strategy

This is the operating manual: how the weekly review actually runs, step by step, and how the
chip plan shapes everything around it. [architecture.md](architecture.md) says how the pieces
fit together; this document says what to do with them once a week.

## Two ways to run the review

The protocol below is the same either way; only who does the reasoning changes.

**`armband brief`** writes the whole deterministic picture as one self-contained Markdown
document and costs nothing: your criteria and review policy, the six steps, competition
status, chip plan, squad and transfers, every flagged player, standing overrides, research
targets, the optimal squad, the fixture table, and a "what this model cannot see" section.
Hand it to a Claude Code session and work the steps there.

⚠️ **Two of those come from your team file, so pass `-team`.** Your criteria and your chip plan
live in `team.json` rather than `config.json` — see
[configuration.md](configuration.md#two-files-configjson-and-teamjson) for why — and a brief run
without the flag describes a manager with the shipped criteria and no chips planned, without
saying so.

**`armband review`** runs the same protocol through the API, where the agent calls tools
iteratively and can chase something it notices mid-analysis rather than reasoning from one
snapshot. This is the path intended to eventually run unattended just before a deadline.

The brief's one real limitation is that it is a snapshot: it cannot decide halfway through
that a particular player needs a closer look. It also omits the API path's **Step 4b**, the
price check — that step calls the optimiser twice, once against tomorrow's prices, which a
static document cannot do.

---

## The weekly review

Both paths run the same numbered deliberation, **Step 0 through Step 5**, with the API path
adding a price check as Step 4b. The order matters — each step constrains the next, and
jumping straight to "which transfer" is how managers burn transfers on problems a planned
wildcard would have fixed for free. The diagram shows the spine; the sections after it take
each step in turn.

```mermaid
flowchart TB
    s0["Step 0 — Competition status<br/>who is still in Europe and the cups,<br/>before scoring anyone"]
    s1["Step 1 — Chip strategy<br/>re-derived weekly from what is known now"]
    near{"wildcard or free hit<br/>within 2 gameweeks?"}
    s2["Step 2 — Transfers and budget<br/>free transfers reconstructed, not published"]
    s3["Step 3 — Availability<br/>FPL flags plus web search<br/>manager quote beats aggregator beats rumour"]
    s4["Step 4 — Research targets<br/>look where the model is structurally blind"]
    s4b["Step 4b — Prices<br/>API path only: does a rise change the TIMING?"]
    s5["Step 5 — Decide<br/>measured against review_policy"]
    out(["verdict: the specific move, or an explicit<br/>'no move this week' — plus the one thing<br/>that would change the agent's mind"])
    note(["flagged, and it changes every later step:<br/>do not pay now for problems<br/>the chip will fix for free"])

    s0 --> s1
    s1 --> near
    near -->|"yes"| note
    near --> s2
    s2 --> s3
    s3 --> s4
    s4 --> s4b
    s4b --> s5
    note -.-> s5
    s5 --> out

    classDef first fill:#F4E0E3,stroke:#A8404E,color:#141A21
    classDef q fill:#FBF2E3,stroke:#B9770E,color:#141A21
    classDef done fill:#DFEDE6,stroke:#2F7A57,color:#141A21
    classDef api fill:#F4F6F9,stroke:#7A8791,color:#141A21
    class s0 first
    class near q
    class out done
    class s4b,note api
```

Step 0 is red because it is the one that invalidates everything after it: score players
against stale competition status and the earlier work has to be thrown away rather than
adjusted.

**An imminent chip does not skip the later steps.** The prompt asks the agent to *say so
immediately* because it changes how every later step is weighed — a transfer to paper over a
problem the wildcard will fix is not worth making. Availability still gets checked; a player
who is out is still out.

### Step 0 — Competition status

**Before scoring anyone.** Which clubs are still in Europe and the domestic cups, over what
dates, and how long ago that was verified.

Participation is not in the FPL API, so the configuration is only as good as the last check —
and it goes stale every time a club is eliminated. The agent verifies against current results
by web search and corrects anything wrong with `update_competition_status`, which re-scores
every player immediately and persists the change to `config.json`.

This is first because it changes the numbers everything else rests on. A club knocked out of
Europe should stop carrying a rotation penalty; one that wins a play-off should start
carrying it. Scoring players against stale competition status produces confidently wrong
recommendations — and if you score first and discover the error afterwards, the earlier work
has to be thrown away.

### Step 1 — Chip strategy, re-derived weekly

**Chip timing is delegated to the agent.** The gameweeks in `config.json` are a starting
hypothesis, not a commitment: each week the agent asks what the best gameweek for every
unused chip is *given what is known now*, and says whether that differs from the plan on
file. Fixture runs, injuries, price changes and how the squad actually developed all move
the answer.

Two things force a rethink: a chip whose window is closing unused, and a wildcard now sitting
so far away that the squad cannot survive until it.

If a wildcard or free hit is within two gameweeks, the agent says so immediately, because it
re-weighs everything downstream — problems the chip will fix for free should not be paid for
now.

Moving a chip is cheap; drifting without a plan is not. The step always ends with a specific
gameweek for every unused chip, even a provisional one, plus the single thing that would
move it.

### Step 2 — Transfers and budget

Free transfers and money in the bank. The transfer count is **reconstructed** from history
rather than published, so it is close but not authoritative; the agent flags when a decision
turns on it.

### Step 3 — Availability, including news the model cannot see

FPL's official status flags, **plus a web search**. FPL's `news` field is terse and lags press
conferences by days, so the agent searches for injury updates, expected return dates and
rotation talk on any flagged player and any player under consideration.

Source weighting is explicit: a manager's direct quote outranks an aggregator, which outranks
a rumour account. The agent states which it relied on and labels a rumour as a rumour.

The step has a second half that is easy to skip and is the one that keeps the system honest.
New findings are written back with `set_player_status`, so the *next* run starts from them.
And **every standing override is re-checked against this week's news**, not just the ones
about to lapse — a hand-maintained list that outlives its situation and keeps applying
silently is a failure mode this project has hit more than once.

### Step 4 — Research targets, where the model is structurally blind

The model cannot see a role change, a set-piece hierarchy that moved over the summer, or a
player whose statistics came from a different club. `research_targets` names those players,
and the instruction is to search them **even though you were not considering them** — that is
the entire point of the step, and it is the one an agent optimising for a tidy answer will
quietly drop.

### Step 4b — Prices, API path only

The optimiser is run twice, the second time against tomorrow's expected prices. A price
change is a **timing** input, not a decision input: it can say to make a move today rather
than Friday, and it should not decide which move. `armband brief` has no equivalent, because
a static document cannot re-run the optimiser.

### Step 5 — Decide, including deciding nothing

Measured against `review_policy`. **Doing nothing and banking the transfer is a first-class
outcome and usually the right one.**

A move is recommended only on an affirmative case: an injury or suspension costing real
points, a player who has lost his starting place, a fixture swing worth acting on, or an
upgrade clearing the threshold. Never churn to look busy after a bad week.

The output ends with a clear verdict — the specific move, or an explicit "no move this week"
— plus the one thing that would change the agent's mind before the deadline.

---

## Correct the numbers, do not override the decision

When the review turns up a fact the model cannot see — an injury the flags have not caught
up with, a player who has quietly lost his place — there are two ways to act on it, and they
are not equivalent. This is the single most important habit in operating the system.

**Correct the numbers, do not override the decision.** The analysis layer knows facts the model
cannot see; it does not know better than the optimiser what a player is worth. `minutes` mode
supplies the fact — what he actually plays — and lets everything downstream recompute. `lock`
and `start` assert a conclusion instead, and are unfalsifiable: the optimiser cannot decline.

Isak is the case. He scored 0.49 pts/gw because a leg break held him to 694 minutes, so he was
locked in — and the optimiser satisfied that by parking £9.0m on the bench at 0.53. Correcting
his minutes to 85 puts him at **3.57**, and the optimiser then declines him at £9.0m in favour
of Thiago at 4.66 for £8.0m. That refusal *is the answer*, and a lock destroys it. A lock can
only ever reproduce your own conclusion back at you; a corrected number lets the optimiser
tell you something you did not already believe.

Traced as two paths from the same fact, the difference is the whole lesson — the red path
can only echo you, the green one can disagree with you:

```mermaid
flowchart TB
    fact["the fact the model cannot see:<br/>Isak reads 0.49 pts/gw off 694 minutes,<br/>because of a leg break that is over"]

    lockit["override the decision:<br/>lock him in"]
    forced["the optimiser cannot decline —<br/>£9.0m parked on the bench at 0.53"]
    mirror["your own conclusion,<br/>reproduced back at you"]

    fixnum["correct the number:<br/>minutes mode, 85 per match"]
    rescore["everything downstream recomputes —<br/>his score goes 0.49 to 3.57"]
    answer["the optimiser is still free to decline,<br/>and does: Thiago at 4.66 for £8.0m —<br/>something you did not already believe"]

    fact --> lockit --> forced --> mirror
    fact --> fixnum --> rescore --> answer

    classDef muted fill:#F4F6F9,stroke:#7A8791,color:#141A21
    classDef bad fill:#F4E0E3,stroke:#A8404E,color:#141A21
    classDef pure fill:#E3EDF1,stroke:#1F5F73,color:#141A21
    classDef good fill:#DFEDE6,stroke:#2F7A57,color:#141A21
    class fact muted
    class lockit,forced,mirror bad
    class fixnum,rescore pure
    class answer good
```

Setting minutes to 0 also subsumes most exclusions: the score collapses and the player is never
picked, without the layer having to be certain.

**Check the claim before you exclude on it.** The first live run excluded Gabriel because
Arsenal's 0.72 xGC "was earned with Saliba alongside him". The archive says otherwise — in
2025-26 Arsenal conceded **0.67** a game without Saliba against 0.71 with him, over nine games,
with near-identical xGC. The reasoning was plausible, the press coverage of the injury was real,
and the inference was never checked against thirty seconds of data. The 4th-best player in the
game was excluded on it.

---

## Chip strategy

Chips expire and only one may be played per gameweek, so four first-half chips need four
distinct gameweeks inside a fixed window. Deciding week by week reliably ends with chips
burned in the last fortnight or lost outright.

⚠️ **The plan lives in `team.json`, loaded with `-team`, not in `config.json`.** It is one
manager's strategy for one entry, and `config.json` is also what a deployed server mounts for
anyone who visits — a chip plan reaching a stranger truncates *his* optimiser horizon to a
wildcard he never planned. Without `-team`, `armband chips` reports every chip as unplanned,
which is indistinguishable from having planned none.

### Windows

FPL issues two sets per season, and `ChipWindows()` **reads them from the bootstrap payload**
rather than assuming them — `start_event` and `stop_event` per chip. The table below is what the
payload currently says, transcribed here for orientation; the code is the authority and nothing
pins this table, so treat a disagreement as the table being stale.

| Chip | First half | Second half |
|---|---|---|
| Bench Boost | **GW1**-19 | GW20-38 |
| Triple Captain | **GW1**-19 | GW20-38 |
| Wildcard | GW2-19 | GW20-38 |
| Free Hit | GW2-19 | GW20-38 |

Bench Boost and Triple Captain are playable in GW1; wildcard and free hit are not.

Laid out on the calendar, the squeeze is easier to see than in the table: four first-half
chips need four distinct gameweeks inside the bars on the left, the wildcard and free hit
open a week later than the other two, and everything left unplayed at GW19 is gone.

```mermaid
gantt
    title Chip windows across the 38 gameweeks
    dateFormat X
    axisFormat %s
    todayMarker off

    section Bench Boost
        first half GW1-19    :bb1, 1, 19
        second half GW20-38  :bb2, 20, 38
    section Triple Captain
        first half GW1-19    :tc1, 1, 19
        second half GW20-38  :tc2, 20, 38
    section Wildcard
        first half GW2-19    :wc1, 2, 19
        second half GW20-38  :wc2, 20, 38
    section Free Hit
        first half GW2-19    :fh1, 2, 19
        second half GW20-38  :fh2, 20, 38
```

`armband chips` does **not** print both columns. It keeps only the half covering the upcoming
gameweek, so from GW20 onward every row reads `GW20-38` — which is the useful view when planning
and a surprise if you are expecting the table above.

### How the plan changes squad construction

This is the part most managers miss, and it is wired into the optimiser. A chip plan is not
just a note in the calendar; it changes what the optimiser builds this week.

**A wildcard at GW N** makes the current squad disposable, so the horizon shortens to the span
from the upcoming gameweek to N-1. There is no value optimising for fixtures the squad will
never serve.

**A free hit does not shorten the horizon.** The chip fields a separate temporary fifteen for
one gameweek and hands the permanent squad straight back, so the squad still has to be good
afterwards. What a planned free hit removes is that one gameweek from scoring, which
`ApplyFreeHitToScoring` does.

**A planned bench boost raises the bench weight**, and the effect is large and easy to see for
yourself: set a plan, run `armband chips` to confirm the raised weight, and run `armband squad`
with and without it. Expect a bench of genuine starters instead of two near-ghosts, paid for by
a weaker eleven in the weeks before the boost.

Three chips, three different levers — the diagram below is worth a moment precisely because
readers conflate them: one shortens the horizon, one removes a gameweek from scoring without
shortening anything, and one changes a weight.

```mermaid
flowchart LR
    wc["planned wildcard<br/>at GW N"]
    fh["planned free hit<br/>at GW N"]
    bb["planned bench boost<br/>at GW N"]

    hz["horizon shortens to<br/>upcoming GW .. N-1"]
    sc["GW N removed from scoring<br/>via ApplyFreeHitToScoring —<br/>the horizon does not shorten"]
    bw["bench weight raised"]

    wcwhy["the current squad is disposable:<br/>no value optimising for fixtures<br/>it will never serve"]
    fhwhy["a temporary fifteen plays GW N;<br/>the permanent squad comes straight<br/>back and must still be good"]
    bbwhy["a bench of genuine starters,<br/>paid for by a weaker eleven<br/>in the weeks before the boost"]

    wc --> hz --> wcwhy
    fh --> sc --> fhwhy
    bb --> bw --> bbwhy

    classDef chip fill:#F4F6F9,stroke:#7A8791,color:#141A21
    classDef lever fill:#E3EDF1,stroke:#1F5F73,color:#141A21
    classDef effect fill:#FBF2E3,stroke:#B9770E,color:#141A21
    class wc,fh,bb chip
    class hz,sc,bw lever
    class wcwhy,fhwhy,bbwhy effect
```

### Timing principles

**Wildcard.** Late enough that roles have resolved, early enough to fix a bad start. The
biggest weakness of any pre-season squad is unproven roles — new signings, new managers,
players whose stats came from another club — and four or five gameweeks of real data settles
almost all of it.

Prefer the gameweek **after** a long international break over the one before it. Wildcarding
before a break commits the squad, then leaves it idle for three weeks while players get
injured on international duty, with only a single free transfer to react.

**Bench Boost.** Must follow the wildcard, so the squad can be built for it, and should avoid
post-international-break gameweeks where rotation is least predictable.

**Triple Captain.** A nailed premium with a soft home fixture. Without double gameweeks
there is no obvious spike, so this is a judgement call best made close to the week itself.

**Free Hit.** The awkward one. Its natural use is a blank gameweek — and blanks come from
FA Cup postponements in January, which fall in the *second* half. **The first-half Free Hit
may have no natural target at all.** Treat it as a chip to spend opportunistically on the
week your squad's fixtures are worst, and set a placeholder gameweek so it does not expire
unnoticed.

### Validation

`armband chips` checks the plan against the windows the API publishes, and reports five
distinct problems: a gameweek outside a chip's range, a chip planned for the wrong half of the
season, two chips in one gameweek, a gameweek that has already passed, and a chip left to
expire unplanned. A clean plan says so explicitly.

It then reports the effect on squad building — the shortened horizon and the raised bench weight
— so you can see what the plan is doing before running the optimiser.

**Blanks and doubles are printed separately and are not part of plan validation.** They are
computed from the fixture list whether or not a plan exists, which is the right shape: a blank
is a fact about the calendar, not a mistake in your plan.

---

## A note on blanks and doubles

In a freshly published fixture list, **every club plays exactly once in every gameweek**.
Blanks and doubles emerge later, once cup progress forces postponements.

This means a Free Hit held for a blank is a bet that one materialises before the chip
expires — and in the first half of a season, that bet is usually a bad one.
