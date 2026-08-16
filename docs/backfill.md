# Historical availability backfill

Read this before using anything under `data/captures/<season>/`, and before changing
`internal/backfill`, `internal/wayback` or the reader in `internal/capture`.

## What problem this solves

The season archive this project replays — [vaastav/Fantasy-Premier-League](https://github.com/vaastav/Fantasy-Premier-League) —
is an **end-of-season photograph**. It records what every player finished the season
with. A player injured from September to November finishes it fit, and looks fit in
every weekly row of the record.

So the replay cannot see who was actually unavailable at the moment a decision was
taken. `backtest.statusAt` reconstructs what it can from the final status plus the
date its news was posted, and that reconstruction is **one-directional by
construction**: it can carry a season-ending absence backwards, and it can say nothing
at all about an absence that resolved.

The absences that resolve are exactly the population every rotation-risk constant
needs. `internal/capture` was built to record this going forward and admits in its own
opening comment that it "yields nothing this season". This backfill recovers the same
quantity for six finished seasons.

**Four questions the research record marks as unanswerable are blocked on this data**,
and they are listed in `internal/capture`'s doc comment: whether a published "75%
chance of playing" really corresponds to about 75% of a fit player's minutes; the
injuries that *resolve*; the availability trajectory; and penalty duty changing
mid-season.

## Where the data comes from

FPL published the answer at the time, in `bootstrap-static`, and nobody kept it. The
Internet Archive did. Each archived crawl carries, per player:

| field | what it is |
|---|---|
| `status` | `a` available, `d` doubtful, `i` injured, `s` suspended, `n` ineligible, `u` unavailable |
| `chance_of_playing_next_round` | FPL's percentage, in steps of 25. **`null` is not `0`** — null means no figure was published, which for a fit player is the normal state |
| `chance_of_playing_this_round` | the same for the current gameweek |
| `news` / `news_added` | the free text FPL showed, and when it was posted. The pair is what makes an absence *datable* |
| `code` | the **permanent** player identifier. Everything here is keyed on it |

`code` and not `id`. FPL reassigns element ids every summer, so a record keyed on one
comes back next season attached to a different footballer — a trap this project has
already paid for in the standing overrides, and a worse one here, because the point of
six seasons of this data is to join it across seasons.

## The rule that must not be got wrong

> **Take the last crawl STRICTLY BEFORE the deadline. Never the nearest.**

Nearest is the natural thing to write and it is wrong in a way nothing downstream can
detect. FPL updates availability continuously, and hardest in the hour *after* a
deadline as team sheets land. So a crawl forty minutes late is both the nearest one
and a strictly better forecast of that gameweek than anything a manager could have
had. Figures built on it would be excellent, plausible, and measuring hindsight.

Two mechanisms enforce it, and they are deliberately independent.

**`SelectPreDeadline`** takes the latest crawl with `crawl < deadline`, strictly. It
returns "nothing" rather than reaching forward, and the caller reports a gap.

**`capture.VerifyPreDeadline`** then proves the timing **from the payload alone**,
using no external clock, and `Backfilled.Write` refuses to store a body that fails it:

- the target gameweek's `deadline_time`, as this very payload reports it, is after the
  crawl;
- that gameweek is not already `finished`;
- FPL's own `is_next` has not advanced past it.

The second mechanism exists because the first trusts the Internet Archive's crawler
clock, which is the single external number the whole argument rests on. A body served
after a deadline contradicts itself three ways, and none of them needs us to be right
about what time it was.

## What went wrong the first time: FPL's calendar moves within a season

The obvious design — and the one this task was specified with — reads all 38 deadlines
out of a single mid-season crawl, because FPL publishes the whole calendar in every
payload. **That is unsafe, and the cross-check against the season archive caught it.**

Measured on 2020-21, against the crawl of 26 January 2021:

| gameweeks | gap from deadline to that gameweek's first kickoff |
|---|---|
| GW1-24 | **+1.50 h exactly** — FPL's published rule, on gameweeks already played or imminent |
| GW25 and GW27-35 | **−1.0 to −17.5 h** — the crawl's deadlines for *future* gameweeks are provisional, and the first kickoff later moved earlier: to Friday 19:00/20:00 in eight cases (−17.5 h) and to a Saturday lunchtime slot in two (−1.0 h). GW26 did not move and reads +1.50 h |
| GW36-37 | **+73.8 and +76.5 h** — rescheduled into a different week entirely |

Ten of 38 gameweeks read as "a match kicked off before its own deadline", which is
impossible, and **none of them is an error**. The *mechanism* — broadcast fixture
selection — is inferred and not measured; what is measured is that the kickoff moved
and the deadline moved with it.

It is not a 2020-21 quirk. Across all six seasons the discovery crawl carries 7 to 12
provisional gameweeks, and the worst inversion runs −17.5 h in 2020-21 through 2023-24
but **−42.0 h in 2024-25 and −41.5 h in 2025-26** — a gameweek moved into a different
day, not a slot moved within one. Every figure is in that season's `deadlines.json`
under `cross_check`.

The consequence is concrete. GW25's deadline was published in January as 20 February
11:00 and the true one was 19 February 18:30. Two crawls sit between them, so
selecting against the January calendar reaches the crawl of **20 February 10:46 —
sixteen hours after the real deadline, with Friday night's match already played**. It
happens in **6 of 38 gameweeks in 2020-21** — GW25, 28, 29, 31, 32 and 35, all after
GW24.

`VerifyPreDeadline` would have refused every one of them, so the naive design loses
those six as **gaps** rather than leaking them: the payload-internal check is what
converts a silent leak into visible missing data. Recovering them is what needs the
resolver. (That crawl was fetched by hand to check: it reports GW25's corrected
deadline of 19 February 18:30, `is_current` on GW25 and `is_next` on GW26. The refusal
comes from the deadline and `is_next` tests — `is_current` is not one of the three
things the check reads.)

**`resolve` therefore treats the deadline as an estimate to be tightened**, not a
fact:

1. start from the earlier of the provisional deadline and ninety minutes before the
   archive's first kickoff for that gameweek — FPL's own rule, from a source that
   cannot be provisional;
2. select the last crawl before that bound, and fetch it;
3. ask **that** crawl what the deadline was; a crawl from inside a gameweek's own
   run-up carries the final figure;
4. if it says we are late, step back; if it says we could have gone later, widen. The
   bound moves monotonically, so it terminates.

In the common case step 3 confirms the bound and it is one fetch. Resolved deadlines
are written back to `deadlines.json` and marked `confirmed`, so a re-run does not
repeat the work or the mistake.

## The cross-check, and what it is for

Every season's recovered calendar is compared against the season archive's kickoff
times before anything is selected against it. These are **not two measurements of one
quantity with a winner to pick** — FPL's `deadline_time` is the authority and nothing
else in this project carries it. What the comparison establishes is *alignment*: that
the calendar describes the same 38 gameweeks the replay will score, from the same
season.

It matters because Wayback windows are calendar dates and seasons overlap them —
2019-20 ran into July 2020 — so a crawl fetched while looking for 2020-21 can carry
the previous season's `events[]`, and every deadline would be weeks out with nothing
failing.

**The statistic is the median gap, and it reads 1.50 h exactly in all six seasons —
FPL's published rule, recovered from data**, over 227 compared gameweeks. (227 and not
228: 2022-23's GW7 was cancelled outright after the Queen's death, and the archive
carries no fixture for it.) Provisional drift and rescheduling move only the tails; a
wrong season moves the median by a week or more. An earlier version failed the season
on any individual inversion, which would have rejected every real season.

## What was actually recovered

All six seasons, **228 of 228 gameweeks**, every one from a crawl whose own payload
proves it predates its deadline. That number is true and it flatters the data, so it
does not travel alone:

- median gap to the deadline **6.5 h**, per season 4.7 to 8.5;
- **10 of 228** rows are more than three days old, the worst 562.8 h — 23 days —
  at 2024-25 GW10;
- but on the measure this code argues is the better one, `EventsBehind`, only **4 of
  228** were crawled outside their own gameweek's run-up: 2020-21 GW3 and GW7, 2024-25
  GW9 and GW10. Six of the ten "stale by hours" rows span an international break and
  are the freshest team news that ever existed for that gameweek. **Read `EventsBehind`
  before `HoursBefore`.**
- **7 gameweeks are served by a crawl shared with a neighbour**, across three crawls.
  2024-25 GW8, GW9 and GW10 are byte-identical copies of the single crawl of 10 October
  2024. Three covered gameweeks, one observation — anything treating gameweeks as
  independent draws must pool or exclude them.

Coverage is not evidence. A row is evidence.

The store is 33 MB for 228 captures: about 92 KB each gzipped, from roughly 1.5 MB of
JSON apiece.

## Storage

```
data/captures/
  2026-08-10T0528Z/              <- the live series: timestamp-named, current season
    bootstrap-static.json.gz
    fixtures.json.gz
    manifest.json
  2020-21/
    deadlines.json               <- the recovered calendar, with its cross-check
    GW01-2020-09-11T0125Z/
      bootstrap-static.json.gz
      manifest.json
    GW02-.../
```

Backfilled directories lead with a zero-padded gameweek, where the live series is
timestamp-only. Two reasons, both of which bite. Where coverage is thin the same crawl
can be the last one before two consecutive deadlines and is honest evidence for both —
under timestamp-only naming the second write collides with the first and a gameweek
silently loses its row. And `ls` then sorts by gameweek, so a hole is visible as a
hole.

A backfilled manifest carries a `backfill` block that a live one does not: the season,
the exact URL fetched, the Archive's crawl identifier and its content digest. Live
captures written before this existed read back with it absent, which is the truth
about them.

Only `bootstrap-static` is recovered. The fixture list for a finished season is
already in the season archive, complete and with results, so fetching an archived copy
would spend someone else's bandwidth on a worse version of something already on disk.

## Reading it

```go
store, err := capture.Open("data/captures")
p, ok, err := store.Player("2020-21", 25, 118748)  // season, gameweek, permanent code
a, err := store.At("2020-21", 25)                  // every player at that deadline
n := store.Count("2020-21", 25)                    // captures covering it; 0 is a gap

dl, _ := backfill.LoadDeadlines("data/captures", "2020-21")
rows := backfill.Rows(store, "2020-21", dl)        // 38 rows, present or not
```

`At` on a gameweek with no capture is an **error naming the gap**, never an empty
result. Coverage is genuinely patchy and that is fine; a caller reading a missing
gameweek as "nobody was injured" is not. A repair that silently applies nothing is
this area's recorded failure mode, twice over.

`backfill.Rows` returns a row for all 38 gameweeks whether or not one was found, for
the same reason the sweep harness prints infeasible cells rather than dropping them.
It is the **only** coverage-table builder: `capture.Store` briefly had its own, with
two fewer columns, and the offline `-coverage` report used the poorer one — so the
staleness column that is the whole point of the table printed "—" in the mode people
actually run. One quantity with two implementations, where the one you read knows
less, is this project's signature defect and it took about an hour to reappear inside
a change whose own documentation warns about it.

## Commands

```bash
armband backfill 2020-21          # one season
armband backfill all              # 2020-21 through 2025-26
armband backfill -coverage all    # report what is on disk, fetch nothing
armband backfill -show 25 2020-21 # one gameweek's recovered team news
armband backfill -per-gameweek 5 2020-21   # denser, for the trajectory question
```

A re-run over a complete season makes **no network requests at all** — the calendar is
cached beside the captures and the captures are their own record of what has been
fetched. That is politeness rather than convenience: the CDX index is slow and it is a
charity's infrastructure.

`-per-gameweek` is the cadence knob. One is the decision-relevant unit and the only
one run so far. Higher stores several crawls per deadline, each verified against that
same deadline, so raising it cannot make the store less honest — only larger. It
exists for the availability-trajectory question, which will want several reads inside
a gameweek.

## Politeness

Requests are serialised behind a minimum interval (default 1.5 s), retried with
exponential backoff that honours `Retry-After`, and cached on disk indefinitely — a
finished season's crawl history is immutable, so a TTL would buy only repeated slow
requests for an answer that cannot change. `-refresh-index` exists for the case where
that reasoning is wrong.

**The raw payload arrives gzipped whether or not the request asked for it**, because
the Archive replays the origin's bytes and FPL served it compressed. `wayback.Gunzipped`
sniffs the magic number rather than trusting the header or the transport, which is
correct under every combination; the symptom it prevents is a JSON parse failing on
byte `0x8b`.

## What this deliberately does not do

**It does not touch the replay's scoring path and it does not change `statusAt`.**
Every measured figure in the research record was produced with the reconstruction as
it stands. Switching the input underneath them would make the record incomparable with
itself while looking like nothing had happened. Delivering the data and proving it
honest is one job; using it is the next one, with its own measurement pass.

**It re-derives no constant.** Not `BlendMinutesK`, not `availabilityFactor`, not
`MinutesHalfLife`.

## The test that matters most

`TestEveryStoredCaptureIsPointInTimeHonest` walks every capture on disk and re-derives
the honesty from the stored bytes rather than trusting the manifest beside them, then
checks the two against each other. It runs in the ordinary suite and is **not**
DIAG-gated: it reads only the filesystem, and gating it would mean the property most
worth guarding is the one least often checked.

`TestAProvisionalCalendarDoesNotLeak` pins the calendar-drift bug above, against a
synthetic archive, and asserts up front that the naive rule still reaches a
post-deadline crawl on that fixture — so the test keeps meaning something if somebody
later decides the resolver is over-engineering.
