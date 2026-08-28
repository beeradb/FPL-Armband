// Is BonusWeight's shipped 1.5 defensible, and where does the schedule land on
// the player bands?
//
//	DIAG=1 EXP=bonusweight FPL_CELLS=/tmp/bonusweight.csv \
//	  scripts/replay -run TestDiagBonusWeight -v -timeout 2h
//	DIAG=1 go test ./internal/backtest/ -run TestDiagBonusWeightByBand -v
//
// # ⚠️ The founding table is a RETIRED REGIME and cannot be compared to today
//
// `BonusWeight`'s doc comment carries a five-value table peaking at 1.0. That
// table was swept in `a0bb150` under the FLAT regime, **before
// `BonusPriorWeight` existed**. Today `BonusWeight` is the EVIDENCE end of a
// schedule: `bonusWeightFor` returns `lo + (hi-lo)*bonusEvidence`, with `lo` the
// prior end (0.5) and `hi` this field (1.5). The flat regime is now reachable
// only as `bonus_prior_weight: -1`, which short-circuits the interpolation.
//
// So "the curve peaks at 1.0" is a statement about a knob that no longer exists
// in that form, and **1.5 — the value that actually ships — was never in it.**
// The same comment records that this has already been misread once, as
// "BonusWeight ships at 1.0". This run does not repeat the comparison; it
// measures the shipped value against its own neighbours on the current archive.
//
// The record is harsher still on the old table's provenance: three GW1 cells on
// `POLICY`, on absolute totals, an argmax over five values, from the 2026-08-05
// data state — inside the zero-penalty window and before the doubles,
// substitution and selling-price fixes. Four of the five contamination events
// bear on it, and 66% of the associated 67 points is 2024-25, the season the
// zero-penalty bug was worth 113.
//
// # Pre-registration
//
//   - **Baseline is the SHIPPED 1.5**, not the old table's 1.0. The question is
//     "is what we ship defensible", not "which of five values wins" — the second
//     is the argmax that produced the figure being replaced, and repeating it
//     would commit the same error with fresher data.
//   - **Expected outcome: UNRESOLVED.** The record states that a
//     `BonusWeight`-sized effect sits below this harness's floor, and that the
//     schedule screen's own p=0.05 threshold is 152-349 a season per ladder
//     against a grid median of 39. This run is very unlikely to separate
//     anything, and that is not a reason to skip it: the shipped value currently
//     rests on a table from a regime it never belonged to.
//   - **The arms are not equally spaced in APPLIED weight**, and must not be read
//     as a ladder in it. `hi` is a ceiling the schedule approaches and never
//     reaches: at `blend_rate_k` 8 an ever-present reaches evidence 0.826, so
//     `hi=1.5` applies ~1.33 and `hi=0.5` applies a flat 0.5 (where `hi==lo`).
//     Arm 0.5 is therefore a REGIME CHANGE as well as a level change.
//   - **P0, the mediator.** `bonusEvidence` returns 0 before the season starts,
//     so at a GW1 cutoff every arm applies exactly `lo` and the knob is INERT by
//     construction. GW1 cells must be byte-identical; movement elsewhere is the
//     liveness half. This is the BandStrength lesson applied in advance.
package backtest

import (
	"fmt"
	"sort"
	"testing"
)

func TestDiagBonusWeight(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	bw := func(w float64) func(sc *SimConfig) {
		return func(sc *SimConfig) { sc.Weights.BonusWeight = w }
	}
	settingOf := func(sc SimConfig) float64 { return sc.Weights.BonusWeight }

	runPolicySweep(t, []policyVariant{
		{
			label:   "bonus_weight 1.5 — the evidence end of the schedule (SHIPS)",
			apply:   bw(1.5),
			setting: settingOf,
		},
		{
			// hi == lo, so the schedule collapses to a flat 0.5. A regime change,
			// not a point on a ladder — see the pre-registration.
			label:   "bonus_weight 0.5 — collapses the schedule to flat prior",
			apply:   bw(0.5),
			setting: settingOf,
		},
		{
			label:   "bonus_weight 1.0 — the old flat table's peak, under the schedule",
			apply:   bw(1.0),
			setting: settingOf,
		},
		{
			label:   "bonus_weight 2.0 — leaning further into a circular term",
			apply:   bw(2.0),
			setting: settingOf,
		},
	}, starts)
}

// TestDiagBonusWeightByBand answers the other half: the applied bonus weight is
// ALREADY a function of the player band, because `bonusEvidence` is the player's
// own minutes and `rotationLabel` cuts on his expected minutes. This reports
// where the schedule actually lands per band, and what the bonus term is worth
// there, so "sweep it at our bands" can be read against what the schedule
// already does rather than proposed blind.
func TestDiagBonusWeightByBand(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type cell struct {
		n                   int
		evidence, applied   float64
		bonusPer90, minutes float64
	}
	agg := map[string]*cell{}
	lo, hi := 0.5, 1.5 // the shipped schedule ends; see bonusWeightFor

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		sc := sweepConfig(cfg, 1, false)
		k := sc.Weights.BlendRateK

		for gw := 2; gw <= 38; gw++ {
			e, _ := EngineAt(cur, prior, gw-1, sc)
			if e == nil {
				continue
			}
			for _, m := range e.AllMetrics() {
				el := e.Boot.ElementByID(m.ID)
				if el == nil {
					continue
				}
				n90 := float64(el.Minutes) / 90
				ev := 0.0
				if k > 0 {
					ev = n90 / (n90 + k)
					if ev > 1 {
						ev = 1
					}
				} else {
					ev = 1
				}
				c := agg[m.RotationRisk]
				if c == nil {
					c = &cell{}
					agg[m.RotationRisk] = c
				}
				c.n++
				c.evidence += ev
				c.applied += lo + (hi-lo)*ev
				c.minutes += float64(el.Minutes)
				if el.Minutes > 0 {
					c.bonusPer90 += float64(el.Bonus) / (float64(el.Minutes) / 90)
				}
			}
		}
	}

	fmt.Println("\nWHERE THE BONUS SCHEDULE LANDS ON EACH PLAYER BAND")
	fmt.Printf("Applied weight is lo + (hi-lo)*evidence, lo=%.1f hi=%.1f, evidence = n90/(n90+k).\n", lo, hi)
	fmt.Println("So the weight is ALREADY band-scheduled: it is a function of the same")
	fmt.Println("minutes the band is cut on. This is what a per-band sweep would be changing.")
	fmt.Printf("\n  %-15s %10s %10s %12s %12s\n", "band", "n", "evidence", "applied wt", "bonus/90")
	for _, b := range []string{"nailed", "likely starter", "rotation risk", "squad player", "fringe"} {
		c := agg[b]
		if c == nil || c.n == 0 {
			continue
		}
		f := float64(c.n)
		fmt.Printf("  %-15s %10d %10.3f %12.3f %12.3f\n",
			b, c.n, c.evidence/f, c.applied/f, c.bonusPer90/f)
	}
	var keys []string
	for b := range agg {
		keys = append(keys, b)
	}
	sort.Strings(keys)
	fmt.Printf("\n  ⚠️ The ceiling of 1.5 is never applied to anyone: the highest band averages\n")
	fmt.Printf("  well below it, because evidence saturates at n90/(n90+k) < 1.\n")
}
