# The rulebook-audit arc: two audits, two adjudications, one retraction, one acquisition

Covers the span from the previous record (`cc9aa2d`-era) to `1e283b7`: the provider-scale
acquisition (`006dfbb`), the retraction-and-fixes commit (`b46a0f3`), and the third rebase onto
main with its snapshot rotation.

**The review layers this record summarises were dispatched, not first-party**, and they stacked
five deep — which is the arc's actual story:

1. Two **rulebook audits** (Opus and Fable, run blind to each other) mapped every paid FPL scoring
   key against the xPoints instrument. Full channel map: nothing unrepresented; three real gaps.
2. Two **reviews of the audit follow-up** (`ec0b6bb`) found nine defects in my application of the
   audits — including that a mechanism I recorded as verified was false.
3. Two **independent adjudications** (one Opus, one Fable, blind to each other and to which claim
   was mine) settled the contested mechanism from FPL's own accounting.
4. A **Fable transport review** answered whether the provider difference is an adjustable schedule.
5. A **statistics review of the accumulated-xPoints protocol** gated the next programme, demanding
   six amendments — and the user's own question then removed a seventh defect the reviewer and I
   had both accepted.

## The retraction, which is the headline

⚠️ **"The clean sheet is compared against the wrong event" was recorded as a finding and is
false.** FPL pays the clean sheet for not conceding *while on the pitch* at 60+ minutes — exactly
what `exp(−xgc)` models. Settled by 22,605 rows with zero exceptions, 89 clean sheets paid in
matches the club conceded in, 77-of-77 within-club disagreements favouring the substituted man —
and by `docs/model.md` §4e, which already stated the rule in a paragraph that opens *"because a
change was once queued on getting it backwards"*. The queued "fix" (fetch club match-level xGC)
was the exact proposal that paragraph forbids.

What survives: −0.0196 clean sheets matched within club-gameweek, season-clustered t −2.11 against
t_crit 4.303 — does not resolve; 3-4% of the channel. The durable output is the rule: **a band
split does not identify a mechanism; the matched comparison does** — and its corollary, that
"verified independently" covered the band *table*, not the *premise*.

## The acquisition

Per-shot Opta is commercially licensed and fbref challenges automated clients, so `006dfbb`
acquired the question's *answer* instead: StatsBomb's complete, openly licensed 2015/16 PL season
(9,908 shots, 380/380 matches) against Understat over the same fixtures. **The Jensen scale agrees
across providers to 0.9%** (1.2664 vs 1.2778 on 175 shared fixtures) while the providers differ 11%
on `E[x²]/E[x]` — the scale is a slowly-varying functional, which is why it transports where the
level offsets cannot reach it. Season variation beats provider variation ~6:1: name the season,
not the feed. Measurement only; `FPL_CS_XGC_FACTOR` stays closed.

## The nine defects fixed at `b46a0f3`, in one line each

One-season decomposition passed off as three-season (the "30-44%" → 35.8% pooled, a ceiling);
Python/Go clean-sheet form drift published as the instrument's numbers; missing-xG counted with a
joint gate hiding that the exposure is **all assists** (goal channel: zero rows); bonus leakage now
evidenced by the discriminating `bonus ~ R + E` regression (b_R 0.254, b_E −0.003) instead of a
correlation; 2022-23's offset mislabelled borrowed when its sidecar says in-season; the collinear
backfill/era group means deleted; the closed-set guard now fails rather than skips off-network;
`special_multiplier` and the doubles carve-out added to the left-realised list; three retractions
returned to the item they belong to.

## The protocol gate

The accumulated-xPoints protocol (private store: `2026-08-14-accumulated-xpoints-protocol.md`) was gated
by statistics review: **do not build as written; buildable after amendments**. The load-bearing
find: "multiplicity is cheap on the proxy" reasserts a derivation the record had already withdrawn.
Amended: measured sharpening gate, pre-registered shapes, xG-lean constants excluded from the
bundle, bundle capped at 2-3, three-outcome final rule, pilots reordered. ⚠️ **Amendment 7 came
from the user**: the vice-captain contrast is the control, not the ruler — it has no squad
divergence, so it reports the best-case SE ratio even when every real ladder's is ≈1. The gate now
measures per-arm ratios on the pilot's own ladder contrasts.

## Gates

`go build`, `go vet`, `go test ./...` clean at this commit. Snapshot regenerated after the third
rebase; figures moved by the commit stamp alone, `constants.csv` byte-identical — main's eight new
commits (including its own clean-sheet diagnostic fix, the same doubles-accumulation class caught
independently) moved no model figure here. **Nothing shipped changed across the whole arc.**

## Redaction note — 2026-08-16

One parenthetical above was edited after this record was filed. It named a private store
this repository may not name; it now reads **private store**. The protocol's gate verdict
and the finding under it are unchanged.

⚠️ **Cleaned rather than exempted.** The standing exemption for already-committed
disclosures is a grandfather clause over an enumerated set; this was found afterwards. The
cost — amending a dated attestation — is acknowledged, which is why this note exists rather
than the edit being silent. **No finding was altered.**
