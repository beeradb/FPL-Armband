# The forced-move census: breaks are rarer in doubles, and the appearance shortfall is where the congestion mass lives

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
decomposition arm could be justified. **It refutes the prevalence half, and it
finds the mass in the two channels the prevalence hides: the cost of a break that
does land on a double, and the appearance shortfall — the user's reading of what a
double is FOR, full appearance points in every leg.**

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
difference (−0.0161) is over 3 SE of a simple binomial reading, which is stated as
descriptive because club-gameweeks are not independent and no p belongs to a
census. The doubling-week exposure is thin (1,975 of 41,567 regular-weeks) and the
doubling-week break counts are small (2-27 per season); the ratio's season spread
(0.33-1.14) reflects that. ⚠️ Part of the lowness is definitional, and the user
named it: a regular who plays one leg of a double has "appeared" by this proxy's
test, where the whole point of a double is full appearance points in both legs.
The shortfall section below measures that channel directly.

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
ratio — 1.34 is conservative. The doublers' lower points-per-match is a
contributor to the 1.34 sitting below the naive 0.73 × 2 (the 10.0+ bracket
breaks zero times in doubling weeks); it is not a decomposition.

## The appearance-shortfall channel: the double is FOR the full appearance points

The user's second note, 2026-08-18: a regular who plays one leg of two has
"appeared" by the break proxy, *"but that's not really the goal of a double. You
want full appearance points."* The shortfall is a regular-week where the player
records fewer 60-minute legs than the club plays fixtures. The missed appearance
points are the exact FPL valuation — a leg never entered costs both points (2), a
leg of 1-59 minutes has banked the first and misses the second (1). The break
above is the all-legs extreme of the same channel, so shortfall counts are a
superset of it.

| | regular-weeks | shortfalls | rate | expected missed appearance pts / regular-week | ratio |
|---|---:|---:|---:|---:|---:|
| ordinary week | 39,592 | 11,763 | 0.2971 | 0.4209 | — |
| doubling week | 1,975 | 827 | **0.4187** | **0.8805** | **2.09** |

**Regulars miss an appearance point in 42% of doubling weeks against 30% of
ordinary ones, and the expected missed appearance points per regular-week are
2.09× in doubles.** This is the largest of the three channels, it is the one the
user predicted, and it is the one the asserted `CongestionSensitivity` 1.0 —
factor 2.0 at a full double — lands closest to. The independent sibling
measurement (`TestDiagNailednessInDoubles`) agrees at the per-player level:
nailed players record 60+ minutes in both legs in 0.72-0.82 of doubling weeks
across the six census seasons.

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

Premiums almost never break in doubling weeks — zero in six seasons. ⚠️ The table
carries counts, not rates: the per-bracket exposure is not printed, and under a
uniform rate the expected 10.0+ doubling-week count is ≈1.25, so zero is
consistent with the premium bracket being absent from doubling breaks rather than
proof of it.

## What this settles, and what it does not

**Settles, on the prevalence half**: regulars' availability does NOT break more
often in doubling weeks — 0.73× the rate, pooled, 5 of 6 seasons. Any reading of
the congestion multiplier as pricing *more frequent* forced moves is refuted.

**Settles, on the cost half**: the cost-weighted channel has mass — **1.34×** the
expected missed points per regular-week in doubling weeks.

**Settles, on the appearance half**: **2.09×** the expected missed appearance
points per regular-week in doubling weeks — regulars miss a 60-minute leg in 42%
of doubles. This is the channel with the mass, it is the one the user predicted
(*"you want full appearance points"*), and the asserted `CongestionSensitivity`
1.0 (factor 2.0 at a full double) lands close to it. The asserted constant is
therefore **defensible on the appearance channel and wrong on the forced-move
channel** — the mechanism it was written to price is not the mechanism that
delivers.

**Does not settle**: the net value — the burden is a bound on the caught-short
cost, not a valuation, and a manager holding a free transfer pays the replacement
difference, not the full missed points; whether a *state-dependent* insurance
reading on a different calendar quantity would differ; and whether the proxy's
misses (injury-then-sale, GW1-GW3) hide a channel the archive rows cannot
express.
