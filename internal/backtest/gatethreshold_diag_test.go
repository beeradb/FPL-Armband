package backtest

import (
	"fmt"
	"os"
	"testing"
)

// `min_gain` and the decision horizon are one threshold written twice. This was
// launched to ask what the second copy is worth once it stops being decorative —
// ⚠️ **and "decorative" is the word this diagnostic disproved before it ran**; see
// the correction below. What it actually measures is the horizon.
//
//	DIAG=1 EXP=GATE2X2 FPL_CELLS=/tmp/gate/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagGateThreshold$' -v -timeout 6h
//
// # ⚠️ The premise this was built on is false, and the reporting is inverted
//
// It was launched to answer "what is the floor worth once it binds", on the
// belief that `min_gain` cannot act at the shipped horizon and had only ever been
// swept there. **Both halves are wrong.** The floor is inert at the shipped
// horizon only at or below 0.4 — above `charge/horizon` it is the binding clause,
// no horizon change required — and it was swept there, paired at 24 cells.
// Re-baselined on the shipped 0.4, the constants-and-sweeps note reads
// **0, −0.535, −0.589, −0.866 pts/gw at floors of 0.4, 0.7, 0.95 and 1.30**:
// monotone harmful across four consecutive settings, which is one of the three
// shapes this record accepts in place of a point estimate. The question is
// answered at the configuration that ships, over a wider binding range than this
// grid probes.
//
// So **the primary here is the horizon, not the floor**:
//
//   - **Primary — the horizon main effect**, `(H8, 0.4) − (H5, 0.4)`. A 36-cell,
//     six-season replication of a recorded **−0.503 pts/gw, CR2 t −2.57, Holm
//     0.33** at 24 cells, against a threshold of **≈16 a season** anchored on that
//     arm's own SE of 0.196. This is the half explicitly *outside* the closed
//     "stop sweeping the transfer gate" bound: `perfectGate` **reads** `p.Horizon`
//     rather than choosing it, so the 106-point ceiling says nothing about
//     changing it.
//   - **Secondary — the floor at H8**, pre-registered **positive** (a lower floor
//     better), against **≈26-55 a season**. It sits *inside* the closed gate family
//     at a horizon nobody ships, so a favourable result is not a case for anything.
//
// # The gate arithmetic, which is still what makes the grid legible
//
// `internal/config/policy.go`: a free transfer needs
//
//	gain >= MinGainForTransfer   AND   gain x horizon + money >= FreeTransferValue
//
// so the second clause alone demands `gain >= charge / horizon` — exactly 0.4 at
// the shipped 2 and 5, which is why 0.0 and 0.4 are byte-identical. At horizon 8
// it demands only 0.25 and the floor governs. Horizon 6 gives a band of width
// 0.067 against 8's 0.15, so 8 is the only useful second level.
//
// # Two things that would make the H8 column unreadable if ignored
//
// **The hit branch carries no gain bar at all.** `shippedAccept` checks
// `p.Gain < p.GainBar` first and `asHit()` sets `GainBar = noGainBar`, so a hit is
// gated on `MinGainHit` alone. Horizon 8 makes hits far easier to justify, so its
// population has a larger hit share that `min_gain` cannot touch — raising the
// horizon makes the floor binding on free moves *and* shrinks the fraction of
// moves it can reach, two effects in opposite directions. **Read the per-cell
// `moves` and `hits` columns as mediators**, or the contrast is uninterpretable at
// either sign.
//
// **`DecisionHorizon` is set, not `Weights.Horizon`.** That is deliberate and it
// is the one thing that would make the horizon effect uninterpretable if it were
// wrong: the fixture-averaging window stays put and `gateHorizon` is the only
// consumer, so this moves the gate and not the scoring.
//
// # The arms
//
//	                  | min_gain 0.4 (shipped) | min_gain 0.0
//	horizon 5 shipped | baseline               | slack, predicted identical
//	horizon 8         | min_gain binds         | charge clause alone
//
// ⚠️ **Arm 2 is a regression check on the gate, not a wiring check**, and the first
// version of this comment called it one. It returns byte-identical whether
// `MinGain` is read on the scored path or not read at all, so it cannot
// distinguish wired-and-inert from unwired — the failure AGENTS.md's standing rule
// names. The genuine wiring check is arm 4 against arm 3. Its value is as a
// regression on the gate given five contamination events, and its proper home is a
// property test over a grid of gains in microseconds rather than 36 replayed
// cells.
//
// # Pre-registered
//
//   - **`H5, 0.0` byte-identical to the baseline on POLICY.** ⚠️ If it comes back
//     non-zero in one or two cells, check the float band before concluding the gate
//     changed: `gain >= 0.4` and `gain*5 >= 2` are equivalent over the reals and not
//     in float64, since 0.4 is not representable. The recorded null held over 12
//     cells and 222 transfers; this is 36 cells and more moves.
//   - **`H8, 0.0` must differ from `H8, 0.4`.** Mechanism-certain; a null there is a
//     wiring failure, not a finding.
//   - **The floor contrast is predicted POSITIVE** — a lower floor better — on the
//     monotone ladder, on the measured mechanism that an over-charged gate refuses
//     real improvements, and on the perfect gate transacting *more* (595 moves to
//     716, 57 hits to 144).
//   - **HOLD byte-identical in all four arms**; both settings are transfer-only.
//   - Report factorial main effects beside simple effects — but note the `min_gain`
//     factorial main effect is exactly **half** the H8 simple effect by
//     construction, since the H5 half is zero. Quote it as arithmetic, not as a
//     second estimate.
//   - **A favourable H8 result is not a case for raising `min_gain` at horizon 5.**
//     The monotone ladder says the opposite and that configuration is not in this
//     design.
func TestDiagGateThreshold(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	fmt.Printf("\n=== the gate horizon, with the min_gain floor as the secondary\n")
	fmt.Printf("The gate is `gain >= min_gain AND gain x horizon >= charge`, so the\n")
	fmt.Printf("second clause alone demands gain >= charge/horizon — exactly 0.4 at\n")
	fmt.Printf("the shipped 2 and 5, which is why 0.0 and 0.4 are identical. At\n")
	fmt.Printf("horizon 8 it demands 0.25 and the floor governs.\n\n")
	fmt.Printf("**PRIMARY is the HORIZON**: arm 3 against arm 1, a %s replication\n",
		gridLabel(len(sweepPairNames()), len(starts)))
	fmt.Printf("of a recorded -0.503 pts/gw (CR2 t -2.57) against a threshold near 16 a\n")
	fmt.Printf("season. It is the half OUTSIDE the closed transfer-gate bound, because\n")
	fmt.Printf("perfectGate READS the horizon rather than choosing it.\n")
	fmt.Printf("SECONDARY is the floor at H8, arm 4 against arm 3, PRE-REGISTERED\n")
	fmt.Printf("POSITIVE and sitting inside that closed family at a horizon nobody\n")
	fmt.Printf("ships — a favourable result there is not a case for anything.\n")
	fmt.Printf("Arm 2 is a REGRESSION check on the gate, not a wiring check: it is\n")
	fmt.Printf("byte-identical whether min_gain is read or not read at all.\n")
	fmt.Printf("**HOLD must not move** — both settings are transfer-only.\n")
	fmt.Printf("Read `moves` and `hits` per cell as mediators: horizon 8 enlarges the\n")
	fmt.Printf("hit share, and hits carry no gain bar for min_gain to touch.\n\n")

	arms := []policyVariant{
		{
			label: "shipped (h5, min_gain 0.4)",
			apply: func(sc *SimConfig) { sc.DecisionHorizon = 5 },
		},
		{
			label: "h5, min_gain 0.0",
			apply: func(sc *SimConfig) { sc.DecisionHorizon = 5; sc.MinGain = 0 },
		},
		{
			label: "h8, min_gain 0.4",
			apply: func(sc *SimConfig) { sc.DecisionHorizon = 8 },
		},
		{
			label: "h8, min_gain 0.0",
			apply: func(sc *SimConfig) { sc.DecisionHorizon = 8; sc.MinGain = 0 },
		},
	}

	runPolicySweep(t, arms, starts)

	fmt.Printf("\nArm 2 must read +0.000; if it does not, check the float band before\n")
	fmt.Printf("concluding the gate changed — gain>=0.4 and gain*5>=2 are equivalent\n")
	fmt.Printf("over the reals and not in float64.\n")
	fmt.Printf("The min_gain factorial main effect is exactly HALF the H8 simple\n")
	fmt.Printf("effect by construction, since the H5 half is zero. Quote it as\n")
	fmt.Printf("arithmetic, not as a second estimate.\n")
}
