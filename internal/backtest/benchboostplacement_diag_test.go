package backtest

// Is bench-boost PLACEMENT measurable at all, and does the state rule beat a
// fixed offset?
//
//	DIAG=1 EXP=BBCEILING FPL_CELLS=/tmp/bbceiling/cells.csv \
//	    FPL_SWEEP_SEASONS=extended scripts/replay \
//	    -run '^TestDiagBenchBoostPlacement$' -v -timeout 3h
//
//	DIAG=1 EXP=BBRULE FPL_CELLS=/tmp/bbrule/cells.csv \
//	    FPL_SWEEP_SEASONS=extended scripts/replay \
//	    -run '^TestDiagBenchBoostPlacement$' -v -timeout 3h
//
// Two blocks, and the second is **gated on the first**. `BBCEILING` is the canary:
// perfect placement against the fixed-offset control, which is the largest number
// any placement rule could return, because a rule's pick is one element of the
// same slice the argmax maximises over. If that ceiling is not separable from the
// cell-to-cell dispersion of this comparison, nothing smaller is, and `BBRULE`
// should not be spent. The pre-registration is in
// `stats/findings/2026-08-17-bench-boost-placement-PREREGISTRATION.md`.
//
// # Why this comparison is cheap when every other transfer-side one is not
//
// **A bench boost is path-invariant on this code, and the whole design rests on
// it.** Three facts, each checkable in `Simulate`:
//
//   - the trigger is consulted AFTER `pickXI`, so every transfer for the week is
//     already made, and `consult` mutates only `chipTriggers`;
//   - playing the chip sets `chip = chipBenchBoost` and `week.BenchBoost`, which
//     reach `weekScoreWithChip` and nothing else — not `free`, not the wallet, not
//     `held`;
//   - `week.BenchBoostGain` is recorded in EVERY week against the *unchipped*
//     week, so it does not go quiet in the week a chip was actually used.
//
// So a placement arm and a control arm hold the identical fifteen every week, make
// the identical transfers, and differ only in which week the chip is scored. The
// paired difference is exactly `gain(A) - gain(B)`, with no transfer-path
// divergence, which is why the 303-point transfer-path noise floor and the
// `POLICY` median of ~70 are the wrong bars for it.
//
// ⚠️ **That is an argument, and this file CHECKS it rather than relying on it.**
// Three checks, in increasing strength:
//
//   - `squad_hash`, `moves` and `hits` identical to the no-chip baseline;
//   - the whole per-week `BenchBoostGain` VECTOR identical across every arm of the
//     block — which is what makes the oracle arm's argmax and the control arm's
//     pick two readings of one slice rather than of two seasons;
//   - `policy_points(arm) - policy_points(baseline) == bench_boost_pts(arm)`,
//     exactly, in integers. `weekPointsWithChip` is `weekScoreWithChip(...).Points`
//     and `BenchBoostGain` is the difference of exactly those two calls, so this
//     identity holds if and only if every other week scored the same.
//
// If any of the three fails the premise is false and no figure from this file
// means anything; each is a `t.Error`, not a printed warning.
//
// # The estimand is PER SEASON-PATH and it is not negotiable
//
// A chip pays in one gameweek. The cell total IS the whole effect, so
// `--scale=per_path` is the reading and `per_gw x 38` is a different quantity —
// this record measures the two disagreeing by 32% on exactly this channel. Neither
// difference below may be divided by weeks played or multiplied by 38.
//
// # What no arm here plays, and what that costs
//
// **No wildcard, no free hit, no triple captain.** The wildcard is excluded because
// pinning it to a common week put the bench boost immediately after the rebuild in
// 30 of 30 cells for one arm and 3-5 of 30 for the others — a worse confound than
// the one it removes. The free hit is excluded because it replaces `fielded` for a
// round, so `BenchBoostGain` in that week would describe a borrowed bench; the
// triple captain because it occupies a week the boost could otherwise use.
//
// The consequence is stated rather than hidden: every arm boosts a squad the
// ordinary objective built, and that objective credits the bench at almost
// nothing. So the LEVEL of every gain here is a floor. For a placement contrast the
// floor cancels, because both arms carry it — but a flat bench flattens the gain
// profile across weeks, so **the ceiling measured here is itself a floor on the
// ceiling**, and a null is correspondingly weaker evidence than it looks.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// congestionOff is the smallest expressible "no congestion damping", and it is a
// number rather than a zero for a reason worth stating loudly.
//
// ⚠️ **`OptionPricing.CongestionSensitivity = 0` does NOT switch congestion off.**
// `analysis.CongestionFactor` reads `if sensitivity <= 0 { sensitivity =
// DefaultCongestionSensitivity }`, so a zero means the package default of 1.0 —
// the *strongest* setting, not the absent one. That is the documented
// unset-is-a-no-op convention the whole `OptionPricing` struct follows, and it is
// correct there; it just means an arm that wants the channel dead has to say so
// with a value, and an arm that wrote 0 believing it had turned congestion off
// would be a comparison that never ran wearing the clothes of one that did.
//
// At 1e-12 the factor is `1 + 1e-12 x (load - 1)` with `load` in [0, 2], so it is
// within 1e-12 of exactly 1 — numerically the off state, at every load this
// archive can produce. `TestCongestionSensitivityZeroIsTheDefaultNotOff` pins both
// halves of this comment.
const congestionOff = 1e-12

// benchBoostControlPlan is the fixed-offset control: the bench boost at
// `start + controlOffsets.benchBoost` and no other chip.
//
// It delegates to `controlWeeks` rather than re-spelling the offset rule, which
// also gives it the overrun-walks-backwards behaviour for free. Only the bench
// boost slot is read: `controlWeeks` places the boost first from an empty `taken`
// map, so its week is unaffected by the two chips this drops.
func benchBoostControlPlan(cur *Season, start int) analysis.ChipPlan {
	return analysis.ChipPlan{BenchBoost: controlWeeks(cur, start).BenchBoost}
}

// bbArm is one arm's reading of one cell, collected through `policyVariant.observe`
// so the console output is self-contained rather than only reconstructible from
// the CSV.
type bbArm struct {
	squad           string
	moves, hits     int
	points          int
	gains           []int // Week.BenchBoostGain, one per played gameweek, in order
	playedGW        int   // the gameweek a chip was actually scored in, 0 for none
	playedPts       int   // that week's gain, 0 when none was played
	oracleGW        int   // the argmax week, only when AxisChipWeek granted one
	oraclePts       int
	hasOracle       bool
	med             ChipTriggerMediator
	weeks           int
	firstGW, lastGW int
}

// bbCollector accumulates one bbArm per (arm, cell).
type bbCollector struct {
	arms  []string
	byArm []map[string]*bbArm
}

func newBBCollector(n int) *bbCollector {
	c := &bbCollector{byArm: make([]map[string]*bbArm, n)}
	for i := range c.byArm {
		c.byArm[i] = map[string]*bbArm{}
	}
	return c
}

func (c *bbCollector) observe(i int, name string) func(seasonPair, int, *SimResult) {
	if i < len(c.arms) {
		c.arms[i] = name
	} else {
		for len(c.arms) <= i {
			c.arms = append(c.arms, "")
		}
		c.arms[i] = name
	}
	return func(pair seasonPair, start int, res *SimResult) {
		a := &bbArm{
			squad: squadHash(res.OpeningSquad),
			moves: res.Transfers, hits: res.Hits, points: res.Points,
			med: res.BenchBoost, weeks: len(res.Weeks),
		}
		for _, w := range res.Weeks {
			a.gains = append(a.gains, w.BenchBoostGain)
			if w.BenchBoost {
				a.playedGW, a.playedPts = w.GW, w.BenchBoostGain
			}
		}
		if len(res.Weeks) > 0 {
			a.firstGW = res.Weeks[0].GW
			a.lastGW = res.Weeks[len(res.Weeks)-1].GW
		}
		// The banked columns' own derivation, never a second argmax here: a
		// diagnostic must not carry its own copy of the thing it is checking.
		if r, ok := chipReadingsOf(res); ok {
			a.hasOracle = true
			a.oracleGW, a.oraclePts = r.BenchBoostOracleGW, r.BenchBoostOraclePts
		}
		c.byArm[i][fmt.Sprintf("%s@%d", pair.Name, start)] = a
	}
}

func (c *bbCollector) keys() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range c.byArm {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// verifyPathInvariance is the premise the whole design rests on, executed rather
// than assumed. Arm 0 must be the no-chip baseline.
//
// It returns the number of cells checked, so a silent zero — every cell infeasible
// in one arm, say — cannot read as a clean pass.
func verifyPathInvariance(t *testing.T, c *bbCollector) int {
	t.Helper()
	checked := 0
	for _, key := range c.keys() {
		base := c.byArm[0][key]
		if base == nil {
			continue
		}
		if base.playedGW != 0 {
			t.Errorf("%s: the baseline arm played a bench boost at GW%d; arm 0 must "+
				"be the un-chipped reference or every identity below is against the "+
				"wrong season", key, base.playedGW)
			continue
		}
		for i := 1; i < len(c.byArm); i++ {
			a := c.byArm[i][key]
			if a == nil {
				continue
			}
			checked++
			if a.squad != base.squad || a.moves != base.moves || a.hits != base.hits {
				t.Errorf("%s arm %d (%s): squad/moves/hits moved (%s/%d/%d against "+
					"%s/%d/%d). A bench boost is consulted after pickXI and reaches "+
					"only weekScoreWithChip, so this cannot happen unless the arm is "+
					"changing something else — and if it does, this file's whole "+
					"premise is false", key, i, c.arms[i],
					a.squad, a.moves, a.hits, base.squad, base.moves, base.hits)
			}
			if len(a.gains) != len(base.gains) {
				t.Errorf("%s arm %d (%s): %d weeks against the baseline's %d",
					key, i, c.arms[i], len(a.gains), len(base.gains))
				continue
			}
			same := true
			for j := range a.gains {
				if a.gains[j] != base.gains[j] {
					same = false
					break
				}
			}
			if !same {
				t.Errorf("%s arm %d (%s): the per-week BenchBoostGain vector differs "+
					"from the baseline's. The oracle's argmax and this arm's pick are "+
					"then readings of two different seasons and the ceiling is not a "+
					"ceiling", key, i, c.arms[i])
			}
			// The sharpest of the three, and an exact integer identity:
			// weekPointsWithChip IS weekScoreWithChip(...).Points and
			// BenchBoostGain is the difference of exactly those two calls.
			if got, want := a.points-base.points, a.playedPts; got != want {
				t.Errorf("%s arm %d (%s): policy_points moved by %d and the chip it "+
					"played was worth %d. Those are equal on an identical path, so "+
					"the arm changed a week it did not play a chip in",
					key, i, c.arms[i], got, want)
			}
		}
	}
	if checked == 0 {
		t.Error("path invariance was checked on ZERO cells, which passes every " +
			"assertion above and proves nothing")
	}
	return checked
}

func TestDiagBenchBoostPlacement(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	starts := sweepStarts()
	pairs := loadPairs(t, cfg)

	blocks := newBlockPicker()
	defer blocks.check(t)

	fmt.Printf("\n=== bench-boost placement: the canary first, the rule only if it clears\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("Every figure below is POINTS PER SEASON-PATH. A chip pays in one\n")
	fmt.Printf("gameweek, so the cell total is the whole effect: do NOT divide by\n")
	fmt.Printf("weeks played and do NOT multiply by 38.\n")
	fmt.Printf("The control plays the bench boost at entry+%d and nothing else.\n",
		controlOffsets.benchBoost)
	fmt.Printf("No arm plays a wildcard, a free hit or a triple captain.\n")

	// The placement census, before either sweep: a cell whose control chip has no
	// week is a comparison that cannot run, and that has to be counted rather than
	// discovered as a zero afterwards.
	placeable := 0
	fmt.Printf("\n--- where the control puts the boost\n")
	fmt.Printf("%-9s %-6s %6s\n", "season", "entry", "bb gw")
	for _, p := range pairs {
		for _, start := range starts {
			gw := benchBoostControlPlan(p.Cur, start).BenchBoost
			if gw > 0 {
				placeable++
			}
			fmt.Printf("%-9s GW%-4d %6d\n", p.Name, start, gw)
		}
	}
	fmt.Printf("cells the control can place a bench boost in: %d of %d\n",
		placeable, len(pairs)*len(starts))
	if placeable == 0 {
		t.Fatal("the control places no bench boost anywhere, so both blocks below " +
			"would compare a chip against itself")
	}

	baseline := policyVariant{
		label: "no chip (reference)",
		apply: func(sc *SimConfig) {},
	}
	control := policyVariant{
		label: fmt.Sprintf("bench boost at entry+%d", controlOffsets.benchBoost),
		apply: func(sc *SimConfig) { sc.ChipPlanner = benchBoostControlPlan },
	}

	if blocks.want("BBCEILING") {
		fmt.Printf("\n--- BLOCK BBCEILING: the canary\n")
		fmt.Printf("Perfect placement is bench_boost_oracle_pts on the oracle arm; the\n")
		fmt.Printf("control's own pick is bench_boost_pts on arm 1. The ceiling is their\n")
		fmt.Printf("difference and is >= 0 in every cell BY CONSTRUCTION, because the\n")
		fmt.Printf("oracle's argmax ranges over the slice the control's pick is drawn\n")
		fmt.Printf("from. A t against zero is therefore mechanical: read this as an\n")
		fmt.Printf("interval on a bound, and as a SIZING of the comparison rather than\n")
		fmt.Printf("as a test of anything.\n")

		c := newBBCollector(3)
		b, ctl := baseline, control
		b.observe = c.observe(0, b.label)
		ctl.observe = c.observe(1, ctl.label)
		oracle := oracleVariant(Oracles{Decision: AxisChipWeek},
			"perfect bench-boost week", nil)
		oracle.observe = c.observe(2, oracle.label)

		runPolicySweep(t, []policyVariant{b, ctl, oracle}, starts)

		n := verifyPathInvariance(t, c)
		fmt.Printf("\npath invariance checked on %d arm-cells: squad/moves/hits, the\n", n)
		fmt.Printf("whole per-week gain vector, and the exact integer identity\n")
		fmt.Printf("policy_points(arm) - policy_points(baseline) == bench_boost_pts.\n")
		reportBBCeiling(t, c)
	}

	if blocks.want("BBRULE") {
		fmt.Printf("\n--- BLOCK BBRULE: the state rule against the control\n")
		fmt.Printf("BenchBoostTrigger at bar %.1f, with congestion damping OFF.\n",
			config.DefaultBenchBoostBar)
		fmt.Printf("⚠️  CongestionSensitivity = 0 means the DEFAULT of %.1f, not off, so\n",
			analysis.DefaultCongestionSensitivity)
		fmt.Printf("this arm sets %g instead. A single on/off contrast at the default\n",
			congestionOff)
		fmt.Printf("would confound the decay shape with congestion damping, and the\n")
		fmt.Printf("congestion spike lands in the same weeks as the doubles.\n")

		c := newBBCollector(3)
		b, ctl := baseline, control
		b.observe = c.observe(0, b.label)
		ctl.observe = c.observe(1, ctl.label)
		rule := policyVariant{
			label: "bench boost by state rule",
			apply: func(sc *SimConfig) {
				sc.BenchBoostTrigger = true
				sc.BenchBoostBar = config.DefaultBenchBoostBar
				sc.OptionPricing.CongestionSensitivity = congestionOff
			},
		}
		rule.observe = c.observe(2, rule.label)

		runPolicySweep(t, []policyVariant{b, ctl, rule}, starts)

		n := verifyPathInvariance(t, c)
		fmt.Printf("\npath invariance checked on %d arm-cells.\n", n)
		reportBBRule(t, c)
	}
}

// reportBBCeiling prints the canary's per-cell table. It computes no standard
// error: that is the inference layer's, off the banked cells.
func reportBBCeiling(t *testing.T, c *bbCollector) {
	t.Helper()
	fmt.Printf("\n%-9s %-6s %6s | %7s %7s | %7s %7s | %8s\n",
		"season", "entry", "weeks", "ctl gw", "ctl pts", "orc gw", "orc pts", "ceiling")
	var sum float64
	n, tied := 0, 0
	weeks := map[int]bool{}
	for _, key := range c.keys() {
		ctl, orc := c.byArm[1][key], c.byArm[2][key]
		if ctl == nil || orc == nil {
			fmt.Printf("%-20s   INFEASIBLE in one arm — a comparison that could not run\n", key)
			continue
		}
		if !orc.hasOracle {
			t.Errorf("%s ran under %s and came back with no placement — the axis is "+
				"stamped and inert, which reports as a clean null",
				key, (Oracles{Decision: AxisChipWeek}).Stamp())
			continue
		}
		d := orc.oraclePts - ctl.playedPts
		if d < 0 {
			t.Errorf("%s: the oracle week (%d) is worth less than the control's (%d). "+
				"The argmax ranges over the slice the control's pick is in, so this "+
				"is only possible if the two arms scored different seasons",
				key, orc.oraclePts, ctl.playedPts)
		}
		if d == 0 {
			tied++
		}
		weeks[orc.oracleGW] = true
		sum += float64(d)
		n++
		fmt.Printf("%-20s %6d | %7d %7d | %7d %7d | %8d\n",
			key, ctl.weeks, ctl.playedGW, ctl.playedPts, orc.oracleGW, orc.oraclePts, d)
	}
	if n == 0 {
		t.Fatal("no cell carried both arms, so the canary measured nothing")
	}
	fmt.Printf("\nmean ceiling over %d cells: %+.2f points per season-path\n", n, sum/float64(n))
	fmt.Printf("cells where the control already caught the best week: %d of %d\n", tied, n)
	fmt.Printf("distinct oracle weeks: %d\n", len(weeks))
	if len(weeks) < 2 {
		t.Errorf("the oracle chose the same gameweek in all %d cells, so it is a "+
			"fixed-week policy under an oracle label rather than an argmax", n)
	}
	fmt.Printf("\nThis mean carries NO dispersion. The per-cell values are banked as\n")
	fmt.Printf("bench_boost_oracle_pts (arm 2) and bench_boost_pts (arm 1); the\n")
	fmt.Printf("standard error, the threshold and the wild bootstrap come from\n")
	fmt.Printf("stats/bench_boost_placement.R and from nowhere else.\n")
}

// reportBBRule prints the rule arm's per-cell table and its liveness funnel.
func reportBBRule(t *testing.T, c *bbCollector) {
	t.Helper()
	fmt.Printf("\n%-9s %-6s | %7s %7s | %7s %7s | %7s | %5s %5s %5s\n",
		"season", "entry", "ctl gw", "ctl pts", "rule gw", "rule pts", "rule-ctl",
		"offer", "cons", "weigh")
	var sum float64
	n, fired, moved := 0, 0, 0
	for _, key := range c.keys() {
		ctl, r := c.byArm[1][key], c.byArm[2][key]
		if ctl == nil || r == nil {
			fmt.Printf("%-20s   INFEASIBLE in one arm — a comparison that could not run\n", key)
			continue
		}
		d := r.playedPts - ctl.playedPts
		if r.playedGW != 0 {
			fired++
		}
		if r.playedGW != ctl.playedGW {
			moved++
		}
		sum += float64(d)
		n++
		fmt.Printf("%-20s | %7d %7d | %7d %7d | %7d | %5d %5d %5d\n",
			key, ctl.playedGW, ctl.playedPts, r.playedGW, r.playedPts, d,
			r.med.OfferedWeeks, r.med.ConsultedWeeks, r.med.WeighedWeeks)
	}
	if n == 0 {
		t.Fatal("no cell carried both arms, so the rule arm measured nothing")
	}
	fmt.Printf("\nmean difference over %d cells: %+.2f points per season-path\n", n, sum/float64(n))
	fmt.Printf("LIVENESS — cells the rule played a bench boost in: %d of %d\n", fired, n)
	fmt.Printf("LIVENESS — cells whose bench-boost gameweek DIFFERS from the control: %d of %d\n",
		moved, n)
	if moved == 0 {
		fmt.Printf("\n⚠️  The rule never moved the chip's week. The deliverable is that\n")
		fmt.Printf("count, not a null: the arm did not act, so there is nothing for a\n")
		fmt.Printf("paired difference to be about.\n")
	}
	fmt.Printf("\nAgain: no dispersion here. stats/bench_boost_placement.R owns it.\n")
}

// TestCongestionSensitivityZeroIsTheDefaultNotOff pins the trap `congestionOff`
// exists for, and it is a regression test rather than a restatement.
//
// The failure it guards is silent in every column: an arm meaning to hold the
// congestion channel still, writing 0, and getting the strongest setting the
// package has. Nothing downstream would say so, and the arm would report a
// confounded contrast as a clean one. Both halves are asserted — that 0 IS the
// default, and that the epsilon IS off — because a test of only the second would
// pass on a build where the first had quietly changed.
func TestCongestionSensitivityZeroIsTheDefaultNotOff(t *testing.T) {
	// `load` is matches per club per gameweek: 0 is a total blank, 2 is every
	// club doubling, which spans everything the archive can produce.
	for _, load := range []float64{0, 0.5, 1, 1.5, 2} {
		if got, want := analysis.CongestionFactor(load, 0),
			analysis.CongestionFactor(load, analysis.DefaultCongestionSensitivity); got != want {
			t.Errorf("load %.1f: sensitivity 0 gives %v and the default gives %v. "+
				"If those have stopped agreeing, congestionOff's comment is wrong "+
				"and every arm that trusted it is describing a different contrast",
				load, got, want)
		}
		if got := analysis.CongestionFactor(load, congestionOff); got < 1-1e-9 || got > 1+1e-9 {
			t.Errorf("load %.1f: at congestionOff the factor is %v, not 1. The arm "+
				"that claims the channel is dead would be damping after all", load, got)
		}
	}
	// And the two must differ somewhere, or the epsilon buys nothing.
	if analysis.CongestionFactor(2, 0) == analysis.CongestionFactor(2, congestionOff) {
		t.Error("a doubling week prices identically at sensitivity 0 and at " +
			"congestionOff, so this constant is not expressing anything")
	}
}

// TestTheBenchBoostControlPlanPlacesOnlyTheBoost checks the control arm is what
// its label says, on every cell of the shipped grid.
//
// Two failures it would catch, both of which produce a plausible table: a plan
// that also places a free hit — which replaces `fielded` for a round and would
// make `BenchBoostGain` describe a borrowed bench — and a cell where the offset
// leaves no week at all, which is a comparison that cannot run rather than a zero.
func TestTheBenchBoostControlPlanPlacesOnlyTheBoost(t *testing.T) {
	cfg := loadConfig(t)
	for _, pair := range loadPairsOrSkip(t, cfg) {
		for _, start := range sweepStarts() {
			p := benchBoostControlPlan(pair.Cur, start)
			if p.Wildcard != 0 || p.FreeHit != 0 || p.TripleCaptain != 0 {
				t.Errorf("%s@%d: the control plan places WC%d FH%d TC%d beside the "+
					"boost; only the bench boost may vary in a placement contrast",
					pair.Name, start, p.Wildcard, p.FreeHit, p.TripleCaptain)
			}
			if p.BenchBoost <= start || p.BenchBoost > 38 {
				t.Errorf("%s@%d: the control bench boost is GW%d, outside (%d, 38] — "+
					"a chip at or before entry is decided with no information the "+
					"opening squad did not also have", pair.Name, start, p.BenchBoost, start)
			}
			// It must be the offset rule and not something else that happens to be
			// in range. `controlWeeks` places the boost first from an empty map, so
			// the only departure from entry+offset is the overrun walk-back.
			if want := start + controlOffsets.benchBoost; want <= 38 && p.BenchBoost != want {
				t.Errorf("%s@%d: the control bench boost is GW%d, want GW%d",
					pair.Name, start, p.BenchBoost, want)
			}
		}
	}
}
