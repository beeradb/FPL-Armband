package backtest

// The gate floor under the exit levers: charge x min_gain, the 2x2 the user's
// "react faster early" hypothesis names.
//
//	DIAG=1 EXP=FLOORX FPL_CELLS=/tmp/floorx.csv \
//	  scripts/replay -run TestDiagGateFloorUnderExitLevers -v -timeout 2h
//
// # The question
//
// The user, 2026-08-18: "early season it may pay to react faster because there's
// a ton of new info. New managers, new players, new roles, etc. By 6-10 games in
// things are typically more Settled I think?" The transfer gate accepts a free
// single iff solo.Gain*horizon >= freeCost AND solo.Gain >= MinGain, so its
// effective floor is max(MinGain, charge/horizon) — 0.4 at the shipped 0.4 / 2.0
// / horizon 5, where both terms sit exactly on the kink. EITHER lever lowered
// alone is inert at the singles gate by that identity — measured: the min_gain
// ladder is byte-identical at or below 0.4, and the flat free_transfer_value
// ladder's low rungs (1.0/1.5) read +8.8/+6.5, unresolved. Only lowering BOTH
// drops the floor. That interaction is unmeasured, and a mechanism that names
// one max() of two terms is exactly what the 2x2 rule exists for.
//
// # The design — the 2x2 at the ON corner, plus the OFF floor-drop simple
//
// Factor A, charge: FreeCost 2.0 (shipped) against 1.0. Factor B, gain bar:
// MinGain 0.4 (shipped) against 0.2. All four corners run under the ON corner
// (anchoredPlan + AnticipateChips + BankLookahead + WeeklyXI true — the
// override mode the prior 2x2 vetted at +73 and the option-decay run
// corroborated at +97.4 a season on 36 cells). The ON corner is the future
// configuration if the levers ship, which is the stated reason for running the
// 2x2 there; the OFF corner's floor-drop simple ({1.0,0.2} against {2.0,0.4}
// at shipped config) rides along as the shipping-relevant confirmation, and its
// baseline is the ladder's 2.0 rung, which the option-decay run proved
// reproduces byte-identically in-process.
//
// Grid: extended six seasons x six entry points, 36 cells per arm, 216 cells,
// POLICY. WeeklyXI is constant across every arm of this run (true in the ON
// arms, shipped-false in the OFF arms), so no contrast carries it.
//
// # Registered contrasts (Holm over FOUR — S is a member, see below)
//
//	A    charge main (1.0 vs 2.0), mean over MinGain.
//	B    MinGain main (0.2 vs 0.4), mean over charge.
//	AxB  the interaction.
//	S    the floor-drop simple {1.0,0.2} minus {2.0,0.4}. NOT independent:
//	     S = A + B exactly (direct expansion of the cell means), so a
//	     combination can reject when neither component does and it belongs in
//	     the Holm family rather than beside it. It is the user's arm: the
//	     only corner whose singles floor actually drops.
//	OFF  {1.0,0.2} minus {2.0,0.4} at shipped config — one contrast, its own
//	     threshold, outside the family.
//
// Contrast weights, stated so a resolving main cannot be misread: with the
// singles-floor effect F living only in the {1.0,0.2} corner, F enters A, B and
// AxB with weights 1/2, 1/2 and 1. A resolving main is F/2 plus the pair
// channel, never a pure lever effect.
//
// # The kink arithmetic, per horizon regime — do not re-derive it from "0.4"
//
// The effective horizon shortens at the end: effectiveHorizon(5, gw) =
// min(5, 39-gw). At H=5 (GW1-34) the floor is max(MinGain, charge/5):
// {2.0,0.4} = 0.4; {1.0,0.4} = 0.4 (singles-INERT via the charge clause —
// MinGain binds); {2.0,0.2} = 0.4 (singles-INERT via MinGain — the charge
// clause binds); {1.0,0.2} = 0.2 (the floor DROPS). From GW35 (H=4..1) every
// arm's floor moves: baseline 0.5/0.667/1.0/2.0 against {1.0,0.4}'s
// 0.4/0.4/0.5/1.0, {2.0,0.2}'s 0.4/0.4/0.5/1.0, and {1.0,0.2}'s
// 0.25/0.333/0.5/1.0 — four end-of-season weeks per cell where all three
// non-baseline arms are live on singles. The two "inert" corners also differ
// in their pair channel: {1.0,0.4} moves the pair through value()'s per-move
// charge AND through soloValue (the alternative rises too); {2.0,0.2} moves it
// through the pair's GainBar alone. Banking reads both levers (shouldBank
// prices BestPackage at freeCost and MinGain), and hits move by path
// divergence — the ladder's census put 83% of the 1.0 rung's multi-move
// movement in the hit channel. So "the mains are pair-channel-only" is FALSE;
// they are pair-plus-banking-plus-hits channels.
//
// # The canary commitment — what a null here does and does NOT license
//
// The user selected both in sequence: this level 2x2 first, a scheduled floor
// second. The schedule arm is pre-registered NOW: {FreeCost 1.0, MinGain 0.2}
// through GW8 (the midpoint of the user's "6-10 games"), the shipped {2.0,
// 0.4} after, under the ON corner, one arm against the ON baseline. It is
// licensed by S resolving in EITHER sign — a resolving negative S is the
// mirror schedule (raise the early floor) and is registered as such.
//
// ⚠️ The canary is CONDITIONAL, not "null level => no schedule". Level =
// schedule + late-drop (path divergence second-order). If the late-half drop
// is INERT — the record's late-season quiet says nothing clears the gain
// threshold at any price after GW28 — then level ≈ schedule and a level null
// binds the schedule. If the late-half drop is LIVE AND NEGATIVE, the level
// can be null while the schedule helps, and the user's own hypothesis is
// exactly the case where the settled season is where a low floor does no good.
// The discriminator is the pre-registered flip split: floor_flips_gt28 below
// the floor (see Liveness) reads "late half inert, canary sound"; at or above
// it reads "late half live, a level null does not bind the schedule".
//
// # Liveness — the counter, the floors, and why it exists
//
// No existing column could count "the floor admitted smaller moves": the moves
// census mixes the singles-floor channel with the pair channel (the ladder's
// 1.0 rung moved moves in 32/36 cells through the pair channel alone), and the
// gate diagnostic stream is not wired into the sweep. So `decide`'s accept
// closure now counts a counterfactual on EVERY arm and every proposal: would
// the SHIPPED gate ({FreeCost 2.0, MinGain 0.4}, hit branch's noGainBar
// preserved) have answered differently? Split at quietBoundaryGW (28).
//
//   - floor_flips_le28 >= 4 of 6 seasons in the floor-drop arm, or the early
//     half is INSUFFICIENT DATA rather than a null.
//   - floor_flips_gt28 reported with the same floor, AS the canary
//     discriminator above.
//   - moves differ from baseline in >= 24 of 36 cells in the floor-drop arm.
//   - free_at_decision lower in the floor-drop arm in >= 4 of 6 seasons (more
//     spending shows there).
//   - HOLD byte-identical is a code fact (the floor is a transfer-decision
//     quantity), not a result.
//
// # Column roles, re-derived for THIS question
//
// The taper run's roles (shape-clean vs level-cut) belong to its mean
// preservation and are NOT inherited. Here the partition is EARLY-SEASON
// EXPOSURE: only GW1/GW6/GW11 cells contain gameweeks 1-10 (the information
// window the user names); GW16 is transitional; GW21/GW26 are late. Every cell
// contains the GW28+ quiet, which is what makes the flip split the canary. No
// pooled figure across roles.
//
// # Registered limitations
//
//   - Level, not schedule: the floor drop applies all season; the schedule arm
//     is the follow-up, its shape committed above.
//   - The ON corner is the compound configuration; WeeklyXI constant across
//     arms (no cross-config carry). No cross-config comparisons against the
//     ladder's OFF baseline are made here — the OFF arms of THIS run are that
//     comparison, in-process.
//   - BankUpTo 5 in seasons that had a 2-transfer bank: the floor governs
//     spending, and the pinned bank overstates the spending capacity of the
//     2020-21..2023-24 cells. Paired differences within a cell survive.
//   - "The model churns 4-5 players a week early" is an unbanked diagnostic
//     print, quoted as a reading (the worldview rewrite's own caveat).
//   - Thresholds per contrast from stats/variance_components.R, t_crit 2.571
//     at df 5; Holm over the four ON-family contrasts.
//
// Multiplicity: Holm over A, B, AxB and S. Each gets its own threshold.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagGateFloorUnderExitLevers(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== the gate floor under the exit levers: charge x min_gain\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)
	fmt.Printf("Arms: {FreeCost, MinGain} = {2.0,0.4} {1.0,0.4} {2.0,0.2} {1.0,0.2}\n")
	fmt.Printf("under the ON corner, plus the OFF floor-drop simple {1.0,0.2} vs\n")
	fmt.Printf("{2.0,0.4} at shipped config. Flip columns split at GW28.\n\n")

	leversOn := func(sc *SimConfig) {
		sc.ChipPlanner = anchoredPlan
		sc.AnticipateChips = true
		sc.BankLookahead = true
		sc.WeeklyXI = true
	}
	// floorDrop is the user's arm: both levers down, so the singles floor
	// actually moves below 0.4.
	floorDrop := func(sc *SimConfig) {
		sc.FreeCost = 1.0
		sc.MinGain = 0.2
	}
	arms := []policyVariant{
		{label: "{2.0,0.4} levers on (baseline)", apply: leversOn},
		{label: "{1.0,0.4} levers on", apply: func(sc *SimConfig) {
			leversOn(sc)
			sc.FreeCost = 1.0
		}},
		{label: "{2.0,0.2} levers on", apply: func(sc *SimConfig) {
			leversOn(sc)
			sc.MinGain = 0.2
		}},
		{label: "{1.0,0.2} levers on", apply: func(sc *SimConfig) {
			leversOn(sc)
			floorDrop(sc)
		}},
		{label: "{2.0,0.4} levers off (shipped)", apply: func(sc *SimConfig) {}},
		{label: "{1.0,0.2} levers off", apply: floorDrop},
	}
	runPolicySweep(t, arms, starts)
}

// TestDiagScheduledFloor is the schedule arm pre-registered in the 2x2's doc
// comment above: {FreeCost 1.0, MinGain 0.2} through GW8 — the midpoint of the
// user's "6-10 games" — the shipped {2.0, 0.4} after, under the ON corner, one
// arm against the ON baseline re-run in the same process.
//
//	DIAG=1 EXP=SCHEDFLOOR FPL_CELLS=/tmp/schedfloor.csv \
//	  scripts/replay -run TestDiagScheduledFloor -v -timeout 2h
//
// # The license, restated from the 2x2's canary
//
// The level 2x2 read S = -2.5 a season against a threshold of 32.3 (nothing
// resolves) and its entry-point columns pointed the user's predicted way
// (+22.7/+26.5 early, -8.4/-41.4 late, six cells each). The pre-registered
// canary discriminator fired: floor_flips_gt28 is live in 6 of 6 seasons, so
// the late half is LIVE and the level null does not bind the schedule — the
// committed conditional licenses this arm even though the committed license
// (S resolving) did not fire. Both readings are stated rather than conflated.
//
// # Registered liveness for the schedule
//
//   - floor_flips_le28 >= 4 of 6 seasons (the schedule's early weeks admit
//     something the shipped floor refuses) or the arm is INSUFFICIENT DATA.
//   - floor_flips_gt28 = 0 by construction: after GW8 the arm's gate IS the
//     shipped gate, and the counterfactual re-prices against the shipped
//     constants — the restore is a confinement, checked rather than assumed.
//   - moves differ from the ON baseline in the early-exposure columns; the
//     census is reported per entry point.
//   - HOLD byte-identical (code fact).
//
// One contrast, its own threshold from stats/variance_components.R.
func TestDiagScheduledFloor(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== the scheduled floor: {1.0,0.2} through GW8, shipped after, levers on\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)

	leversOn := func(sc *SimConfig) {
		sc.ChipPlanner = anchoredPlan
		sc.AnticipateChips = true
		sc.BankLookahead = true
		sc.WeeklyXI = true
	}
	arms := []policyVariant{
		{label: "ON baseline", apply: leversOn},
		{label: "ON + schedule GW8", apply: func(sc *SimConfig) {
			leversOn(sc)
			sc.EarlyFloor.FreeTransferValue = 1.0
			sc.EarlyFloor.MinGainForTransfer = 0.2
			sc.EarlyFloor.UntilGameweek = 8
		}},
	}
	runPolicySweep(t, arms, starts)
}
