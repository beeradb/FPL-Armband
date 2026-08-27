package backtest

// The accumulated-xPoints pilot: one ladder, both metrics, one kill criterion.
//
//	EXP=XPPILOT FPL_CELLS=/tmp/xp.csv scripts/replay -run TestDiagXPointsPilot -v -timeout 4h
//	Rscript stats/sweep_inference.R /tmp/xp.csv
//
// # What the programme is
//
// Tune constants on **forward-accumulated xPoints** — every replayed gameweek
// scored from the same eleven, the same autosubs and the same armband with
// `analysis.XPoints` substituted per player-gameweek — and spend the realised-points
// noise budget once, on a single pre-registered bundle. The premise is that the
// proxy is quieter, so effects worth 11-34 points a season stop being invisible.
//
// # ⚠️ THE KILL CRITERION, and it is the whole point of this file
//
// The premise above is **unmeasured**, and the record has already withdrawn it once
// in derivation form: a player both arms hold cancels exactly in a paired
// difference, so the residual survives only on the players the two arms disagree
// about. The programme's sharpening factor is therefore the **per-arm paired
// standard-error ratio**
//
//	SE(hold_xpoints) / SE(hold_points)
//
// taken on **this ladder's own arm contrasts**, arm by arm, out of
// `stats/sweep_inference.R` on the cells this test writes.
//
//   - **Ratios clustering near 1 close the programme.** The dominant noise is then
//     squad-flip sensitivity, which both metrics share and neither smooths, and no
//     amount of re-running ladders on the proxy will help.
//   - Ratios around 0.7-0.85 are the best case the measured bounds allow, and even
//     there thresholds move only from ~33 to ~23-28 a season. Multiplicity is about
//     30% cheaper on the proxy, not cheap.
//
// **⚠️ The ratio must NOT be read off the vice-captain control below.** That
// contrast is structurally guaranteed to flatter the metric: `decide` never reads
// the captain, so both arms hold *identical squads* and the contrast is one extra
// copy of one player's score in the two-to-four weeks a season the fallback fires.
// The path divergence that dominates a real ladder is absent from it entirely, so
// it would report ~0.75-0.85 even in the world where every real ladder reads 1.00 —
// the gate would pass exactly when it should fail. The control's job is narrower
// and is stated at its own function.
//
// # Why MinutesHalfLife is pilot 1
//
// A mechanism-agreed, minutes-side ladder. Minutes are a **realised** channel that
// xPoints does not replace, so the circularity that makes an xG-lean constant
// untrustworthy on this metric — the model predicts from xG and the metric scores
// on xG — is second-order here, and any divergence between the two metrics'
// verdicts is attributable to noise reduction rather than to flattery. Its points
// shape is recorded as flat from 3 to 8, so a clean shape on xPoints is the
// programme's best case and a repeat of the flatness is its cheap close.
// `BlendRateK` is pilot 2 and is explicitly adversarial; it runs only if this one
// shows shapes AND the ratios come in under 1.
//
// The arms are the recorded MINHL block verbatim — shipped 4 as the baseline, then
// flat, 2, 8, 20 — so the points column is directly comparable with the banked
// MINHL cells rather than being a fresh ladder that has to establish its own.

import (
	"fmt"
	"testing"
)

func TestDiagXPointsPilot(t *testing.T) {
	requireDiag(t)

	fmt.Printf("\n=== PILOT 1: minutes half-life (ships 4), scored on BOTH metrics.\n")
	fmt.Printf("The verdict is the per-arm SE ratio SE(hold_xpoints)/SE(hold_points)\n")
	fmt.Printf("on these ladder contrasts, from stats/sweep_inference.R. Near 1 closes\n")
	fmt.Printf("the programme. Do NOT take the ratio off the vice-captain control.\n")

	// The MINHL block's arms, in its order: shipped first, because
	// runPolicySweep pairs everything against variants[0].
	//
	// `viceCaptainFallback` is set explicitly in `base` rather than left alone,
	// on gridwidth_test.go's reasoning: it is a package-level var, the control
	// below mutates it, and arms run in sequence — so an arm that inherited the
	// control's `false` would be measuring two changes at once with nothing in
	// the output to say so.
	base := func(sc *SimConfig) {
		sc.WeeklyXI = true
		viceCaptainFallback = true
	}
	var v []policyVariant
	for _, x := range []float64{4, 0, 2, 8, 20} {
		label := fmt.Sprintf("half-life %.0f", x)
		if x == 4 {
			label += " (ships)"
		}
		if x == 0 {
			label = "flat (no recency)"
		}
		v = append(v, policyVariant{
			label: label,
			apply: func(sc *SimConfig) {
				base(sc)
				sc.Weights.MinutesHalfLife = x
			},
			// A genuine ordered ladder, so it declares its own swept value rather
			// than leaving stats/schedule_screen.R to parse it out of the label —
			// which is what dropped the "flat" arm from the screen last time, it
			// carrying no number.
			setting: func(sc SimConfig) float64 { return sc.minutesHalfLife() },
		})
	}
	runPolicySweep(t, v, sweepStarts())

	// Restored for any test sharing this process, on the same reasoning as the
	// explicit set in `base`.
	viceCaptainFallback = true

	reportViceCaptainMetricControl(t)
}

// reportViceCaptainMetricControl is the SIGN-AND-BAND CONTROL, and it is not the
// ruler.
//
// # What it can say
//
// One thing: **does the metric preserve a mechanism-certain effect?** The
// vice-captain fallback is the sharpest positive control this harness has — its
// mechanism is certain, it lands almost identically in every cell, and it is one of
// very few findings here that clears significance on both standard errors. If
// turning it off does not cost xPoints with the same sign and roughly the same
// magnitude as it costs points, then the substitution has removed signal along with
// noise and nothing downstream is worth running. That is the captaincy precedent
// stated as a test: removing 45% of `HOLD`'s residual variance removed 47% of its
// signal, so a quieter instrument has to be shown to still hear.
//
// Pre-registered before the run: same sign, and the xPoints magnitude inside
// [0.5x, 1.5x] of the **same-run** points contrast. Never against the recorded
// +0.4590, which carries a data-state caveat of its own.
//
// # ⚠️ What it CANNOT say, and why this paragraph exists
//
// It cannot measure the programme's sharpening factor. `decide` never reads the
// captain, so **both arms hold identical squads in every cell** — the contrast is
// one extra copy of one player's score in the weeks the fallback fires, and the
// squad-flip sensitivity that dominates a real ladder contrast is absent from it by
// construction. Its SE ratio is therefore the best case available to the metric
// (pure per-appearance smoothing) whatever the ladders do, and reading the gate off
// it would pass the programme in exactly the world where it should fail. The ratio
// is taken arm by arm on the ladder above. Firing is also a **minutes** event —
// the captain blanks or he does not — which is identical on both metrics, so that
// variance component is shared and untouchable.
//
// # Why HOLD, and why direct Simulate
//
// HOLD, because that is the metric the recorded vice-captain figure is on and the
// metric the ladder is judged on. Direct `Simulate` calls rather than two more
// sweep arms, because the two arms must be **paired within a cell** on a squad that
// is provably the same — which this checks rather than assumes, by comparing the
// opening fifteens.
//
// No p, no t and no verdict word: this package prints the descriptive half and the
// inference lives in `stats/sweep_inference.R`. The mean and its naive standard
// error are printed because a band check needs a band, and the naive SE is the
// wrong one for a verdict — it ignores the season clustering — which is precisely
// why nothing here calls this a result.
func reportViceCaptainMetricControl(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== CONTROL: the vice-captain fallback on BOTH metrics.\n")
	fmt.Printf("Sign-and-band only. Pre-registered: same sign, and the xPoints\n")
	fmt.Printf("contrast within [0.5x, 1.5x] of THIS RUN's points contrast.\n")
	fmt.Printf("⚠️ Its SE ratio is NOT the programme's sharpening factor — both arms\n")
	fmt.Printf("hold identical squads, so it cannot exhibit the noise the gate is about.\n\n")
	fmt.Printf("%-18s %9s %9s | %9s %9s\n",
		"cell", "d points", "d xpoints", "on points", "on xpoints")

	// Sign: "vice off minus vice on", so a real benefit reads NEGATIVE — the same
	// convention TestDiagGridWidth's positive control uses.
	//
	// The global is restored by defer as well as inline. The inline restores keep
	// each iteration honest; the defer is what survives a panic inside Simulate or
	// a t.Fatal added later inside the window — the leak would otherwise turn the
	// fallback off for every subsequent test in the process, silently. Code review
	// caught the asymmetry: xpointsweek_test.go already did this and this file did
	// not.
	defer func() { viceCaptainFallback = true }()
	var dPts, dXP []float64
	scored := 0
	for _, start := range starts {
		for _, pair := range pairs {
			sc := sweepConfig(cfg, start, false)
			sc.WeeklyXI = true

			viceCaptainFallback = true
			onRes, err := Simulate(pair.Cur, pair.Prior, sc)
			if err != nil {
				fmt.Printf("%-18s infeasible: %v\n",
					fmt.Sprintf("%s@%d", pair.Name, start), err)
				continue
			}
			onHold := HoldCaptaincyWeekly(pair.Cur, pair.Prior, sc, onRes.OpeningSquad)

			viceCaptainFallback = false
			offRes, err := Simulate(pair.Cur, pair.Prior, sc)
			if err != nil {
				fmt.Printf("%-18s infeasible: %v\n",
					fmt.Sprintf("%s@%d", pair.Name, start), err)
				viceCaptainFallback = true
				continue
			}
			offHold := HoldCaptaincyWeekly(pair.Cur, pair.Prior, sc, offRes.OpeningSquad)
			viceCaptainFallback = true

			// The mediator, checked rather than assumed. `decide` never reads the
			// captain, so the two arms must open on the same fifteen; if they do
			// not, this contrast is measuring squad selection and the paragraph
			// above about it having no squad divergence is false.
			if a, b := squadHash(onRes.OpeningSquad), squadHash(offRes.OpeningSquad); a != b {
				t.Errorf("%s@%d: the two vice arms opened on different fifteens "+
					"(%s against %s) — this contrast is not the squad-identical "+
					"one its reading depends on", pair.Name, start, a, b)
			}

			w := float64(len(onRes.Weeks))
			pd := float64(sumInts(offHold.Full)-sumInts(onHold.Full)) / w
			xd := (sumFloats(offHold.FullXP) - sumFloats(onHold.FullXP)) / w
			dPts = append(dPts, pd)
			dXP = append(dXP, xd)
			scored++
			fmt.Printf("%-18s %9.4f %9.4f | %9.1f %9.1f\n",
				fmt.Sprintf("%s@%d", pair.Name, start), pd, xd,
				float64(sumInts(onHold.Full))/w, sumFloats(onHold.FullXP)/w)
		}
	}

	mp, sp := meanSE(dPts)
	mx, sx := meanSE(dXP)
	fmt.Printf("\n%-18s %9.4f %9.4f   (mean paired difference, pts/gw)\n", "MEAN", mp, mx)
	fmt.Printf("%-18s %9.4f %9.4f   (naive SE — NOT the inference; see R)\n", "SE (naive)", sp, sx)
	fmt.Printf("%-18s %9d\n", "cells", scored)
	// The pre-registered control criteria are ASSERTED, not printed for a reader
	// to notice. They are mechanical checks like the squad-identity mediator, not
	// inference — the no-verdict-in-Go convention is about standard errors and
	// significance words, and neither applies to "the sign matches and the ratio
	// sits in a band written down before the run". Code review flagged that the
	// first version printed them, which is how a failed control becomes a line in
	// a log nobody reads.
	if scored > 0 {
		if (mp < 0) != (mx < 0) {
			t.Errorf("vice-captain control: the two metrics DISAGREE ON SIGN "+
				"(points %+.4f, xpoints %+.4f) — the metric does not preserve a "+
				"mechanism-certain effect, and the pre-registered control fails",
				mp, mx)
		}
		if mp != 0 {
			r := mx / mp
			if r < 0.5 || r > 1.5 {
				t.Errorf("vice-captain control: xpoints/points ratio %.3f is "+
					"outside the pre-registered [0.5, 1.5] band", r)
			}
			fmt.Printf("%-18s %9.3f   (pre-registered band 0.5 to 1.5 — asserted)\n",
				"xpoints / points", r)
		}
	}
	if sp != 0 {
		fmt.Printf("%-18s %9.3f   ⚠️ NOT the programme's sharpening factor\n",
			"SE ratio", sx/sp)
	}
	fmt.Printf("\nThe gate is the per-arm SE ratio on the LADDER above, from R.\n")
}
