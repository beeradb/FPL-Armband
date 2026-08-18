# The forced-move census: congestion has no mass in the archive

**Run 2026-08-18**, `TestDiagForcedMoveCensus`, archive rows only — no `Simulate`
call, no cells file, no points claim, so no detection threshold applies. Every
figure is a count or a rate over `gws/merged_gw.csv` walked through the loader's
own `gameweekRows` (the phantom-match and duplicate-row guards are the loader's,
not a second implementation), with the calendar read from `fixtures.csv`.

It exists because the taper's congestion half — `CongestionSensitivity` 1.0,
`CongestionHorizon` 5 — prices a mechanism that was **asserted, never counted**:
P(forced move soon) rises where the fixtures pile up. The option-decay 2×2
(`stats/findings/2026-08-18-option-decay-2x2.md`) then read the taper's
interaction as a point-estimate reversal of exactly that channel's prediction.
This count was the cheapest-first item owed before any decay-versus-congestion
decomposition arm could be justified — and it decides against one.

## The proxy

An **availability break**: a player who appeared in each of his club's previous
three played gameweeks (the regularity bar) and then records zero minutes in the
next two (the persistence bar — a one-week rest is not a move a manager must
answer). A club-gameweek is the unit: a double's legs are summed into one week, a
blank week is skipped rather than read as an absence, and the before/after
sequences run over played club-gameweeks only. The club is the fixture's, via
`was_home`, so a January transfer starts a new sequence. The bracket is the
player's price the week before the break.

⚠️ The proxy misses injury-then-sale (the player leaves the weekly rows), cannot
see GW1-GW3 (nobody yet has three appearances), and attributes a break to the week
it STARTS — a regular rested the week *before* a double is a single-week break, so
the bias runs toward inflating the ordinary-week rate and manufacturing the
reversal below. Read the direction as "no spike", never as "negative mass".

## The count, over the replay's six seasons

| season | regular-weeks, single | regular-weeks, doubling | breaks, single | breaks, doubling | rate/S | rate/D | ratio |
|---|---:|---:|---:|---:|---:|---:|---:|
| 2020-21 | 5,753 | 405 | 363 | 16 | 0.0631 | 0.0395 | 0.63 |
| 2021-22 | 5,384 | 609 | 351 | 27 | 0.0652 | 0.0443 | 0.68 |
| 2022-23 | 6,675 | 468 | 385 | 18 | 0.0577 | 0.0385 | 0.67 |
| 2023-24 | 6,895 | 267 | 409 | 18 | 0.0593 | 0.0674 | 1.14 |
| 2024-25 | 7,432 | 117 | 417 | 4 | 0.0561 | 0.0342 | 0.61 |
| 2025-26 | 7,453 | 109 | 416 | 2 | 0.0558 | 0.0183 | 0.33 |
| **pooled** | **39,592** | **1,975** | **2,341** | **85** | **0.0591** | **0.0430** | **0.73** |

**The prevalence hypothesis — the break rate is HIGHER in doubling weeks — is
refuted on the archive.** Pooled ratio 0.73, below 1 in 5 of 6 seasons; the pooled
difference (−0.0161) is about 3 SE of a simple binomial reading, which is stated as
descriptive because club-gameweeks are not independent and no p belongs to a
census. The doubling-week exposure is thin (1,975 of 41,567 regular-weeks) and the
doubling-week break counts are small (2-27 per season); the ratio's season spread
(0.33-1.14) reflects that.

## The cost half: the insurance identity has two factors

The insurance value is P(break) × the points a caught-short manager misses, and
the second factor is not constant. The burden is the broken player's own points
per match over his three preceding appearances, times the legs his club plays in
the break's start week — one match in an ordinary week, two in a double. Expected
burden per regular club-gameweek:

| | expected missed points / regular-week | ratio |
|---|---:|---:|
| ordinary week | 0.1400 | — |
| doubling week | 0.1883 | **1.34** |

⚠️ **Added on the user's note, 2026-08-18: *"it may not be as prevalent in
doubles, but it can be twice as costly when it happens."* Measured: the
cost-weighted channel DOES concentrate in doubling weeks — 1.34× the expected
missed points per regular-week, against a prevalence ratio of 0.73.** The burden
is an upper bound on the caught-short cost (the player scores his own average; a
manager with a free transfer pays only the replacement difference), and the
start-week attribution bias inflates the ordinary-week rate and so deflates this
ratio — 1.34 is conservative. The doublers' lower points-per-match (premiums break
in doubles zero times in six seasons) is why 1.34 sits below the naive
0.73 × 2.

## The premium bracket

The insurance reading names premiums specifically — they play more, so congestion
exposure concentrates on them:

| bracket | breaks, single | breaks, doubling |
|---|---:|---:|
| <5.0 | 1,157 | 46 |
| 5.0-6.4 | 901 | 32 |
| 6.5-7.9 | 195 | 4 |
| 8.0-9.9 | 63 | 3 |
| 10.0+ | **25** | **0** |

Premiums almost never break in doubling weeks — zero in six seasons. The forced
demand the congestion multiplier insures against is not where the multiplier
prices it.

## What this settles, and what it does not

**Settles, on the prevalence half**: regulars' availability does NOT break more
often in doubling weeks — 0.73× the rate, pooled, 5 of 6 seasons. Any reading of
the congestion multiplier as pricing *more frequent* forced moves is refuted.

**Settles, on the cost half**: the cost-weighted channel has mass — **1.34×** the
expected missed points per regular-week in doubling weeks. The asserted
`CongestionSensitivity` 1.0 makes a doubling week double the holding value (factor
2.0); the archive's own cost-weighted concentration is 1.34, so the asserted
slope overshoots the measured channel by about a half while the P-only channel
runs the other way. That is a count against the constant's *size*, not a
refutation of its sign.

**Does not settle**: the net value — the burden is a bound on the caught-short
cost, not a valuation, and a manager holding a free transfer pays the replacement
difference, not the full missed points; whether a *state-dependent* insurance
reading on a different calendar quantity would differ; and whether the proxy's
misses (injury-then-sale, GW1-GW3) hide a channel the archive rows cannot
express.
