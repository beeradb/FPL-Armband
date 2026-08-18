package backtest

// Option decay under the exit levers: the 2x2 the user's option-value half asks
// for, and the twin of TestDiagPriorReactivityUnderExitLevers.
//
//	DIAG=1 EXP=TAPERX FPL_CELLS=/tmp/taperx.csv \
//	  scripts/replay -run TestDiagOptionDecayUnderExitLevers -v -timeout 2h
//
// # The question
//
// The user's option-decay hypothesis, 2026-08-17: a free transfer and a chip
// are options whose value decays as their window shrinks — "the earlier they
// are, the more valuable... our weighting may also be wrong on that." The lever
// (`TaperFreeTransferValue`) is BUILT and ships off: it reprices what a free
// transfer is CHARGED, from the flat 2.0 to a mean-preserving schedule —
// `analysis.OptionValueAt` = Decay x Congestion, ~1.3x early, ~0 late, the same
// mean over a full season. The flat LEVEL ladder has run (1.0/1.5/3.0/4.0
// against 2.0, 36 cells,
// stats/findings/2026-08-17-free-transfer-value-ladder.md) and nothing resolves
// (+8.8/+6.5/-23.0/-10.5 against per-arm thresholds of 15 to 34). The taper
// SHAPE has never been scored. The sibling half of the same user message — the
// prior-reactivity 2x2 — measured the priors under the exit levers and found
// the levers resolve (+73.0 a season, threshold 36.2) while the prior does not
// (-20.4 against 107.7).
//
// Factor A, option decay: the taper off (the shipped flat 2.0) against on at
// the default curve (`DefaultOptionHalfLife` 8, `CongestionSensitivity` 1.0,
// horizon 5 — all three asserted, none swept). Factor B, exit levers: OFF
// (shipped) against ON — the override mode the user directed and the prior 2x2
// vetted: `anchoredPlan` chips set at the analysis layer, `AnticipateChips`,
// `BankLookahead`, `WeeklyXI` true. Four arms, one grid: extended six seasons x
// six entry points = 36 cells per arm, 144 cells, POLICY.
//
// # Registered contrasts (three under Holm — not a search over seven)
//
//	ON simple   (taper - flat) | levers on.  PRIMARY: the user's question —
//	              is the taper worth anything on the machine that has
//	              something to wait for.
//	OFF simple  (taper - flat) | levers off. The flat ladder's own registered
//	              comparator (its baseline is the flat 2.0 rung at levers off),
//	              re-run in this process rather than paired across commits —
//	              the scored path has moved since the ladder's commit, and
//	              cross-commit pairing needs a byte-identical path.
//	Interaction ON simple - OFF simple. Predicted >= 0, i.e. the taper MORE
//	              harmful (less helpful) under ON: optionvalue.go's own
//	              mechanism comment names the pair — the congestion spike and
//	              the doubles arrive in the SAME weeks, so the taper's
//	              congestion half raises the price of spending a transfer in
//	              precisely the weeks a doubles-chasing plan most wants to
//	              spend one (a doubling week in a long window reads ~5.2
//	              against the flat 2.0). The ON corner IS the doubles-chasing
//	              configuration. A reversal is information, not a failure.
//
// The factorial main (taper mean over both corners) is printed beside the two
// simples, labelled as which it is, per the standing rule on simple-effect
// nulls. Thresholds per contrast from stats/variance_components.R on the banked
// cells; Holm over the three registered.
//
// # The canary argument — why 144 cells are being spent, stated up front
//
// The flat level ladder is the canary envelope and it is already banked. The
// mean normalisation compresses the family's amplitude: the early-week factor
// is 1.185 at h=3, 1.325 at h=8, 1.594 at h=30 and approaches 2.0 as h -> inf
// (a charge of 4.0 — the ladder's top rung), so for ANY half-life the taper's
// charge sits pointwise inside the ladder's measured [1.0, 4.0] envelope except
// in the final weeks below 1.0 — the ladder's owed-but-unrun 0.0 rung region,
// which is singles-inert by the kink and acts only through funded pairs. The
// ladder's per-point responses bound the taper's effect to a season-weighted
// integral of those unresolved responses — a FIRST-ORDER bound, deep inside
// unresolved. "Unresolved with a threshold and an MDE" is therefore the
// EXPECTED reading, and it is a complete answer: the deliverables are the
// mediator profile, the entry-point decomposition and the banked cells.
//
// What a RESOLVING default would mean is registered now: it reads as "the
// default's shape is worth X", a single point of an unmeasured family, and owes
// an extreme-half-life arm before any shape claim — the record's shape rule
// refuses to generalise a single setting. The shipping rule: the taper would
// only ship on if the ON simple effect resolves positive at its own threshold.
// Nothing here is decided by which arm scores highest.
//
// # The two competing readings, and which sign supports which
//
// The worldview rewrite (TestDiagWorldviewRewrite) measured the fresh optimum
// turning over 4-5 players a week, all season — ⚠️ a recorded diagnostic print
// in the research record, NOT banked cells, so it is quoted as a reading and
// not re-derived here. The taper raises the charge exactly where that churn
// lives (early weeks, long windows). If the churn is argmax noise, the taper
// HELPS by filtering moves the model would regret; if it is information the
// model must act on, the taper HURTS by suppressing real adaptation. The signs
// of the contrasts decide which reading the data supports, at point-estimate
// size, whether or not either resolves.
//
// # Column roles — the entry-point decomposition, committed before the run
//
// Mean preservation is exact over a full [1, 38] window only, so the taper
// carries a level cut at late entries (the normalised curve averages below 1
// over a [27, 38] decision window). The six entry points are read in committed
// roles: GW1/GW6 are the SHAPE-CLEAN columns (mean charge ~2.0); GW11/GW16
// transitional; GW21/GW26 are LEVEL-CUT columns (mean charge ~1.3-1.5), whose
// comparable bound is the ladder's 1.0 rung (+8.8, unresolved). No pooled
// figure is quoted across the roles.
//
// # Liveness — floors, not hopes, so a null cannot hide a confinement
//
//   - ftv_flips: gate answers that flip under the taper's counterfactual
//     re-pricing. The SHAPE-CLEAN columns (GW1/GW6) must show flips in >= 4 of
//     6 seasons, or the shape half is INSUFFICIENT DATA rather than a tie; the
//     level-cut columns are expected to act through funded pairs only and are
//     reported per role with the same floor applied. ⚠️ The counter is a
//     partial counterfactual: a funded pair's Alternative is priced at the
//     tapered charge, so pair flips are approximate — a tripwire, not an exact
//     counterfactual decision count.
//   - moves must differ from baseline in a per-cell census. The ladder's 3.0
//     rung (bar 0.6, crossing the kink) moved moves in 30 of 36 cells, and the
//     taper's early bar is ~0.53, so a comparable share is expected in the
//     shape columns. A flat zero is a finding about the gate, not a tie.
//   - ftv_mean_charge must show the pre-registered schedule: GW1 cells
//     ~2.0-2.1 (congestion slightly above 1), GW26 cells ~1.3-1.5. A flat ~2.0
//     column means the schedule never arrived. ⚠️ The load is read from the
//     clubs the squad ACTUALLY HOLDS, so it is a mediator column of the arm's
//     own path rather than a dose column — read the schedule off the taper
//     arm's column, never off a baseline comparison.
//   - HOLD is byte-identical across the taper arms BY CONSTRUCTION (the taper
//     is read only inside the weekly transfer decision and HOLD makes none).
//     It is a code fact, not a result; the checks with power are the columns
//     above. Both corners' HOLD rows are built from the same variant-applied
//     config, so there is no fresh-config trap to rediscover.
//   - banked_weeks is reported: the ON corner's banking was inert in the prior
//     2x2 (0 in 48 cells). If it is > 0 here, banking is a live third member
//     of the compound corner and the writeup says so.
//
// # Registered limitations, so the verdicts cannot over-read
//
//   - The ON corner is the compound configuration the prior 2x2 vetted (chips
//     + anticipation + WeeklyXI fielding); the taper is measured inside it.
//     The OFF simple effect is the ladder's own configuration and reproduces
//     its baseline rung in-process.
//   - One lever, two channels: at the default curve the taper varies decay AND
//     congestion together. No arm here separates them; a resolving effect owes
//     that decomposition rather than assuming either half.
//   - The MinGain kink makes the taper asymmetric by construction. A free
//     single must clear solo.Gain*horizon >= freeCost AND solo.Gain >= MinGain;
//     at the shipped MinGain 0.4 and horizon 5 the flat 2.0 sits exactly on
//     the kink, so the taper can only RAISE the singles bar early (2.65/5 ~
//     0.53) and the late cheapening is inert at the singles gate — it acts
//     only through funded-pair value() and banking. ⚠️ That late-half
//     inertness is arithmetic from the kink identity;
//     TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink pins it for
//     constant charges only and refuses to run under the taper.
//   - Mean preservation covers the DECAY half only; congestion is not
//     mean-preserving, so even GW1 cells carry a small level shift (doubles
//     outweigh blanks, slightly above 2.0).
//   - anchoredPlan is full sight: the ON corner's +73.0 is an upper bound on a
//     real manager's figure, and the taper is measured inside that corner.
//   - WeeklyXI is constant across the taper arms within each corner, so the
//     taper contrasts do NOT carry it (unlike the prior 2x2's cross-config
//     comparison).
//
// # The live-cell moderator (ON corner only), registered before the run
//
// Per the user's criterion from the prior run: a cell can improve only if its
// window from entry contains at least one double or blank AND the plan's first
// chip does not land immediately after entry. Both halves are reported; the
// prediction is that the ON simple effect concentrates in the live half. The
// prior run's GW1/GW16 grid was all-live; the full six-entry grid's late
// columns are where an inert half gets teeth. The OFF corner has no chips to
// plan and is exempt from the split.
//
// Multiplicity: Holm over the three registered contrasts. Each gets its own
// threshold from stats/variance_components.R on the banked cells.

import (
	"fmt"
	"os"
	"testing"
)

func TestDiagOptionDecayUnderExitLevers(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== option decay under the exit levers: a registered 2x2\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)
	fmt.Printf("Arms: taper off/on x levers off/on. ON = anchoredPlan chips (override\n")
	fmt.Printf("mode, set not optimised) + AnticipateChips + BankLookahead + WeeklyXI.\n\n")

	// The live-cell split for the ON corner, printed before any cell runs:
	// calendar and plan are deterministic in (season, entry), upstream of every
	// result.
	fmt.Printf("%-9s %-5s %-4s %-4s %-4s %-4s %-6s %s\n",
		"season", "start", "WC", "BB", "FH", "TC", "live", "reason")
	live := 0
	liveByStart := map[int]int{}
	for _, p := range pairs {
		for _, start := range starts {
			plan := anchoredPlan(p.Cur, start)
			census := censusOf(p.Cur)
			doubles, blanks := 0, 0
			for _, w := range census {
				if w.gw < start || !w.played {
					continue
				}
				if w.doubling > 0 {
					doubles++
				}
				if w.blanking > 0 {
					blanks++
				}
			}
			first := firstChipWeek(plan)
			isLive := (doubles > 0 || blanks > 0) && (first == 0 || first >= start+2)
			reason := "no double or blank in window"
			if doubles > 0 || blanks > 0 {
				if first == start+1 {
					reason = "first chip immediate (entry+1)"
				} else {
					reason = fmt.Sprintf("%dd %db, first chip GW%d", doubles, blanks, first)
				}
			}
			if isLive {
				live++
				liveByStart[start]++
			}
			fmt.Printf("%-9s GW%-4d %3d %3d %3d %3d  %-5v %s\n",
				p.Name, start, plan.Wildcard, plan.BenchBoost, plan.FreeHit,
				plan.TripleCaptain, isLive, reason)
		}
	}
	fmt.Printf("\n%d of %d cells live. Both halves are reported; the prediction is\n",
		live, len(pairs)*len(starts))
	fmt.Printf("that the ON simple effect concentrates in the live half.\n")
	for _, start := range starts {
		n := liveByStart[start]
		verdict := fmt.Sprintf("%d of %d live", n, len(pairs))
		if n < 4 {
			verdict += " — INSUFFICIENT DATA for this half (floor is 4)"
		}
		fmt.Printf("  entry GW%-3d %s\n", start, verdict)
	}
	fmt.Println()

	leversOn := func(sc *SimConfig) {
		sc.ChipPlanner = anchoredPlan
		sc.AnticipateChips = true
		sc.BankLookahead = true
		// Registered before the run: the ON corner fields its chip weeks on
		// the imminent gameweek, exactly as the prior 2x2 defined it. The OFF
		// arms stay at the shipped false.
		sc.WeeklyXI = true
	}
	taperOn := func(sc *SimConfig) {
		sc.TaperFreeTransferValue = true
		// OptionPricing left zero: all defaults (HalfLife 8, congestion
		// sensitivity 1.0, horizon 5), which is the asserted shipped curve.
	}
	arms := []policyVariant{
		{label: "flat, levers off", apply: func(sc *SimConfig) {}},
		{label: "taper, levers off", apply: taperOn},
		{label: "flat, levers on", apply: leversOn},
		{label: "taper, levers on", apply: func(sc *SimConfig) {
			leversOn(sc)
			taperOn(sc)
		}},
	}
	runPolicySweep(t, arms, starts)
}
