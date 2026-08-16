package backtest

// The sweep-side half of the oracle harness: how an oracled arm is declared, how
// it is stamped, and what it must not move.
//
// oracle.go holds the types and the declarations; this holds the four things a
// *sweep* has to do with them — construct an arm, refuse an arm that would
// produce an uninterpretable figure, say so loudly on the console, and check the
// invariance the arm declared. See the oracle-design document.
//
// # Why provenance is layered rather than recorded once
//
// The failure being designed against is not a wrong number. It is an **orphaned**
// one: a whole section measured with the transfer gate's minimum-gain threshold
// at 0.7, retracted to 0.4 eleven commits later, and then cited as ground truth
// by an audit that had no way to see which setting produced it. An oracle figure
// is far more dangerous in that role, because it is a hindsight upper bound that
// looks exactly like a score. So the stamp is attached at every place a number
// can be copied from — the console table, the per-cell CSV, the means file, the
// provenance sidecar's declared arms — and all four come from one edit, because a
// convention saying "remember to write down what you ran" rots the same way every
// other hand-maintained record in this project has.

import (
	"fmt"
	"sort"
	"strings"

	"armband/internal/config"
)

// oraclePrefix is the label prefix an oracled arm carries.
//
// It travels into the console table, the CSV variant column, the R block headers
// and the means rows, because all four read the label. Constructed here rather
// than typed by an author, so it cannot be forgotten on the one arm that most
// needs it.
func oraclePrefix(o Oracles) string {
	return fmt.Sprintf("ORACLE[%s] ", o.Stamp())
}

// oracleVariant builds a sweep arm that runs under hindsight.
//
// It does two things a hand-written policyVariant cannot be trusted to do
// together: it stamps the label, and it installs the oracle on the SimConfig the
// cell actually runs. Because both come from the same value, the stamp cannot
// disagree with what ran — which is the property the whole config-field design
// exists to buy, and which an environment variable structurally cannot have.
//
// The extra apply is for the rest of the arm's settings and runs *after* the
// oracle is installed. It may not touch Oracles: runPolicySweep re-reads the
// value out of the constructed config for every cell and fails if it has moved.
func oracleVariant(o Oracles, label string, apply func(*SimConfig)) policyVariant {
	return policyVariant{
		label:   oraclePrefix(o) + label,
		oracles: o,
		apply: func(sc *SimConfig) {
			sc.Oracles = o
			if apply != nil {
				apply(sc)
			}
		},
	}
}

// oraclesOf reports the hindsight an arm actually installs, by applying it to a
// probe config.
//
// Read off the applied config rather than off the variant's own field, the same
// trick writeSweepProvenance uses for BankUpTo: an arm that sets Oracles in a
// hand-written apply is described correctly, and an arm whose label claims an
// oracle its apply does not install is caught by validateOracleArms rather than
// quietly stamping a lie.
// The environment seed is deliberately **not** cleared first. sweepConfig folds
// FPL_ORACLE_AVAILABILITY and FPL_ORACLE_PRICES into every cell config, so a
// stray one oracles the baseline arm too — and validateOracleArms then refuses
// the whole sweep rather than quietly measuring hindsight against hindsight.
// Clearing it here would be safe and silent, which is the wrong half of that
// trade: a switch documented in docs/replay.md that turns out to be inert on the
// diagnostics is exactly the class of surprise this package keeps paying for.
func oraclesOf(cfg config.Config, start int, v policyVariant) Oracles {
	sc := sweepConfig(cfg, start, false)
	v.apply(&sc)
	return sc.Oracles
}

// validateOracleArms refuses a sweep whose oracle arrangement would make its own
// output uninterpretable. Its caller makes this fatal, not a warning: every one
// of these produces a plausible table with a meaningless sign.
//
// It returns an error rather than taking a *testing.T so that the rules can be
// tested directly. A guard whose only exercise is the code path it guards is a
// guard nobody has ever seen fire.
func validateOracleArms(variants []policyVariant) error {
	anyActive := false
	for _, v := range variants {
		if err := v.oracles.Validate(); err != nil {
			return fmt.Errorf("variant %q: %w", v.label, err)
		}
		// The unreportable arm is refused here rather than at the means row,
		// because a sweep writes a number in four places and the means row is only
		// the most convenient of them. See Oracles.Reportable and omniscience.go.
		if !v.oracles.Reportable() {
			return fmt.Errorf("variant %q runs under %s, which is a test fixture and "+
				"never a report: it bounds no capability, it is the positive control "+
				"for the apparatus. A sweep would write its figure to the cells file, "+
				"the means file, the console table and the provenance sidecar, and "+
				"stats/sweep_inference.R would give it a standard error. Drive "+
				"Simulate directly", v.label, v.oracles.Stamp())
		}
		if v.oracles.Active() {
			anyActive = true
		}
	}
	// An arm may not *claim* hindsight it does not install, whether or not
	// anything else in the sweep is oracled: a label is the only thing most
	// readers of a console table ever see.
	for _, v := range variants {
		if !v.oracles.Active() && strings.Contains(v.label, "ORACLE[") {
			return fmt.Errorf("variant %q is labelled as an oracle and installs "+
				"none — a label is the only thing most readers see", v.label)
		}
	}
	if !anyActive {
		return nil
	}
	// The baseline must be un-oracled, enforced rather than conventional. Every
	// comparison this harness prints is paired against variants[0], so a paired
	// difference between two oracled arms bounds nothing and marking an oracled
	// arm as the baseline makes every downstream sign meaningless.
	if variants[0].oracles.Active() {
		return fmt.Errorf("the baseline arm %q is oracled (%s) — a paired "+
			"difference between two hindsight arms bounds neither better data nor "+
			"better judgement, and reportPairedDifferences pairs everything "+
			"against variants[0]", variants[0].label, variants[0].oracles.Stamp())
	}
	for _, v := range variants {
		if v.oracles.Active() && !strings.HasPrefix(v.label, oraclePrefix(v.oracles)) {
			return fmt.Errorf("variant %q runs under %s and does not say so in "+
				"its label — use oracleVariant, which stamps it from the same "+
				"value it installs", v.label, v.oracles.Stamp())
		}
	}
	return nil
}

// printOracleBanner says what the table below is and is not, before anyone reads
// a number off it.
func printOracleBanner(variants []policyVariant) {
	var oracled []policyVariant
	for _, v := range variants {
		if v.oracles.Active() {
			oracled = append(oracled, v)
		}
	}
	if len(oracled) == 0 {
		return
	}
	fmt.Printf("\n*** ORACLE SWEEP — these arms are given hindsight ***\n")
	for _, v := range oracled {
		fmt.Printf("  %-28s %-9s %s\n", v.oracles.Stamp(), v.oracles.Kind(), oracleBounds(v.oracles))
		if cols := v.oracles.MustNotMove(); len(cols) > 0 {
			fmt.Printf("  %-28s must not move: %s\n", "", strings.Join(cols, ", "))
		} else {
			fmt.Printf("  %-28s declares no cell invariance; it rests on the input diff\n", "")
		}
		if cols := v.oracles.MustMove(); len(cols) > 0 {
			fmt.Printf("  %-28s must move (liveness): %s\n", "", strings.Join(cols, ", "))
		}
	}
	fmt.Printf("Every oracled figure is an **upper bound** on what a capability could be\n")
	fmt.Printf("worth, never a score. The totals below are NOT comparable with any figure\n")
	fmt.Printf("in AGENTS.md, all of which were measured with no hindsight at all — and\n")
	fmt.Printf("no oracle may become the default, or the whole record stops being\n")
	fmt.Printf("comparable with itself at a stroke.\n")
}

// oracleBounds is the one-line statement of what each oracle bounds, so the
// banner says what the number means rather than only which flag was on.
func oracleBounds(o Oracles) string {
	var out []string
	if o.Has(OracleAvailability) {
		out = append(out, "bounds perfect team news at the coarsest resolution: the degenerate "+
			"case of lineups, firing on a season TOTAL of zero minutes, so it sees only players "+
			"who never appear all year and no injury that resolves")
	}
	if o.Has(OracleTransactPrice) {
		out = append(out, "bounds perfect price timing, and with it the whole economic case for acting fast")
	}
	if o.Has(OracleMinutes) {
		out = append(out, "bounds the WHOLE rotation-risk family: how much football each player "+
			"is about to play over the decision window, of which perfect team news is the "+
			"degenerate case")
	}
	if o.Has(OracleLineups) {
		out = append(out, "bounds the REACHABLE half of that: who is picked over the decision "+
			"window, priced at conditional averages. Minutes minus lineups is the residual "+
			"nobody could have bought")
	}
	if o.Has(OracleFeatures) {
		s := "bounds perfect team news at the resolution the QUESTION is asked at: " +
			"whether each player features in the gameweek about to be played, which " +
			"is the one availability channel the lineups oracle cannot reach, because " +
			"availabilityFactor zeroes a flagged player's Score before " +
			"ExpectedMinutes is consulted"
		// The caveat is true of an ungated arm and FALSE of a gated one, and
		// printing it regardless would misdescribe the figure below it — which is
		// the failure this banner exists to prevent, arriving in the banner.
		if o.FeaturesFrom == 0 {
			s += ". NOT a clean bound on a squad-changing arm: a player missing the " +
				"entry gameweek reads unavailable and is removed from the opening " +
				"pool for the season"
		} else {
			s += ". Gated from gameweek " + fmt.Sprint(o.FeaturesFrom) + ", so the " +
				"opening squad is built without it and the figure is the value of " +
				"ACTING on the news rather than of owning a different fifteen"
		}
		out = append(out, s)
	}
	if o.Has(OracleOmniscient) {
		out = append(out, "bounds NOTHING — it is the positive control for the apparatus, "+
			"a fixture that must never be reported")
	}
	if o.Has(OracleTeamNews) {
		out = append(out, "bounds knowing the REAL team news at each deadline: FPL's own "+
			"availability flag as published, including the doubtful cases and every "+
			"absence that later resolved, which the end-of-season reconstruction cannot "+
			"see at all")
	}
	if o.Has(OracleTeamNewsChance) {
		out = append(out, "adds FPL's published percentage chance of playing, which the "+
			"replay has never seen; against the flag-only arm it bounds the GRANULARITY "+
			"alone, not team news")
	}
	switch o.Decision {
	case AxisChipWeek:
		out = append(out, "bounds scoring-chip TIMING on the squad actually held; "+
			"not chip preparation, which needs a squad built for the chip")
	case AxisArmband:
		out = append(out, "bounds better judgement given the same data, on the armband; "+
			"captain AND vice jointly, since an oracle captain always played")
	case AxisTransferGate:
		out = append(out, "bounds the WHOLE transfer-gate constant family at once: "+
			"acceptance of the model's own proposals, not the search that made them")
	case AxisTransferGateXPoints:
		out = append(out, "bounds NOT a capability but a CRITERION: what a gate that "+
			"has only ever seen underlying — xG, xA, xGC, never a goal — is worth "+
			"against one that knows the realised points. Read it as a fraction of "+
			"the transfergate arm on the same cells, never on its own")
	case AxisTransferGateResidual:
		out = append(out, "bounds NOTHING on realised points: it is a POSITIVE "+
			"CONTROL. Points = xPoints + residual identically, so an oracle on the "+
			"residual's sign raises the scored metric by construction and a gain "+
			"there is expected whatever is true. Read policy_xpoints instead, as a "+
			"ratio against the transfergatexp arm — and never as a share of the "+
			"transfergate arm's gain, which is not what a decomposition of the "+
			"CRITERIA licenses about the GAINS")
	case AxisTransferGateAntiResidual:
		out = append(out, "bounds NOTHING on realised points either, and this one "+
			"is a NEGATIVE control: it accepts on the sign of MINUS the residual, "+
			"so points = xPoints + residual makes its realised level negative by "+
			"construction and a level that is not clearly negative means the arm "+
			"did not wire. Its purpose is the CONTRAST against transfergateres on "+
			"policy_xpoints, whose null is NOT zero — the two accept sets partition "+
			"the free-transfer stream, so accept-mass asymmetry enters additively "+
			"and transfergateall is what identifies it")
	case AxisTransferGateAcceptAll:
		out = append(out, "is NOT an oracle: it accepts every package the search "+
			"proposes and reads no hindsight at all. Two uses — it is the NO-GATE "+
			"policy, bounding the gate-constant family from below where a perfect "+
			"gate bounds it from above, and it identifies the accept-mass offset in "+
			"the transfergateanti-minus-transfergateres contrast, whose null is "+
			"T*(1-2p) rather than zero. The week's allowance and the one-hit limit "+
			"still bind: this removes the VALUE bar, not the transfer budget")
	}
	if len(out) == 0 {
		return "bounds nothing declared"
	}
	return strings.Join(out, "; ")
}

// oracleInvarianceViolations is Tier 2: every metric an oracle declared it must not
// move is compared against the baseline arm, cell by cell.
//
// Falsification is roughly two orders of magnitude cheaper than confirmation on
// this harness — a violated invariance shows up in one cell, where confirming an
// effect on the transfer metric needs an effect of about 147 points a season. So
// this runs in Go, immediately after the grid, rather than waiting for someone to
// remember an R flag. Today, forgetting the flag means the check silently does
// not happen, which is the same class of failure as the oracle itself.
//
// A declared column with no collected series is a **failure**, not a skip. That
// is the anti-no-op guard: an oracle that names a metric this sweep does not
// measure has declared an invariance nobody is checking, and silence there is
// indistinguishable from a clean pass.
func oracleInvarianceViolations(variants []policyVariant,
	series map[string][]map[string]float64) []string {

	var out []string
	for vi := 1; vi < len(variants); vi++ {
		v := variants[vi]
		if !v.oracles.Active() {
			continue
		}
		for _, col := range v.oracles.MustNotMove() {
			got, ok := series[col]
			if !ok || len(got) <= vi {
				out = append(out, fmt.Sprintf("%s declares %s must not move and "+
					"this sweep does not collect it — an unchecked invariance is "+
					"worse than none", v.oracles.Stamp(), col))
				continue
			}
			var worst string
			var worstDiff float64
			moved := 0
			for _, key := range sortedKeys(got[0]) {
				base := got[0][key]
				cell, ok := got[vi][key]
				if !ok {
					continue // infeasible in one arm; the row records that itself
				}
				if cell == base {
					continue
				}
				moved++
				if d := cell - base; abs(d) > abs(worstDiff) {
					worst, worstDiff = key, d
				}
			}
			if moved > 0 {
				out = append(out, fmt.Sprintf("INVARIANCE VIOLATED: %s moved %s in "+
					"%d cell(s); worst %s by %+.4f per gameweek. The oracle is "+
					"reaching a decision it declared it cannot reach, so its "+
					"headline figure is measuring something other than what it "+
					"claims", v.label, col, moved, worst, worstDiff))
			}
		}
	}
	return out
}

// oracleLivenessViolations is the mirror of the invariance check: every metric an
// oracle declared it *must* move is compared against the baseline arm, and an
// oracle that moved none of them is reported as inert.
//
// Every other guarantee in this package is a refusal, and an arm wired so badly
// that it reaches nothing at all passes all of them. That is not hypothetical for
// OracleTransactPrice — it has no bootstrapFields for Tier 1 to observe, a
// must-*not* set for Tier 2, a diagnostic that asserts nothing, and a headline
// that is a null. See Oracles.MustMove for why `moves` is the column and why the
// headline metric is not.
//
// A declared column with no collected series is a failure here for the same
// reason it is over there: an unchecked declaration is worse than none.
func oracleLivenessViolations(variants []policyVariant,
	series map[string][]map[string]float64) []string {

	var out []string
	for vi := 1; vi < len(variants); vi++ {
		v := variants[vi]
		if !v.oracles.Active() {
			continue
		}
		for _, col := range v.oracles.MustMove() {
			got, ok := series[col]
			if !ok || len(got) <= vi {
				out = append(out, fmt.Sprintf("%s declares %s must move and this "+
					"sweep does not collect it — an unchecked liveness claim is worse "+
					"than none", v.oracles.Stamp(), col))
				continue
			}
			moved := 0
			for _, key := range sortedKeys(got[0]) {
				cell, ok := got[vi][key]
				if !ok {
					continue // infeasible in one arm; the row records that itself
				}
				if cell != got[0][key] {
					moved++
				}
			}
			if moved == 0 {
				out = append(out, fmt.Sprintf("INERT ORACLE: %s left %s identical to "+
					"the baseline in every one of %d cells. The arm is not reaching a "+
					"decision at all, and an arm that reaches nothing reports the same "+
					"clean null as a real one — which is why this is checked rather "+
					"than inferred from the headline",
					v.label, col, len(got[0])))
			}
		}
	}
	return out
}

// invarianceSeries names each collected per-cell metric by the CSV column it is
// written to, which is the same name MustNotMove declares.
//
// One map rather than positional arguments so that a declaration naming a column
// this sweep does not collect is *detectable*: with positional arguments the
// checker could only compare what it was handed and would have no way to notice a
// declaration going unchecked.
//
// The keys are asserted against cellMetricColumns by
// TestEveryDeclarableColumnIsCollected, because the two lists express one thing —
// "what an oracle may pin" — and this package's most-repeated bug is one quantity
// with two expressions. A column added here and not there is undeclarable; one
// added there and not here is undeclared *and* unchecked, which is worse.
func invarianceSeries(policy, hold, fixedCap, noCap, holdXP, policyXP, moves, hits []map[string]float64) map[string][]map[string]float64 {
	return map[string][]map[string]float64{
		"policy_points":        policy,
		"hold_points":          hold,
		"hold_fixedcap_points": fixedCap,
		"hold_nocap_points":    noCap,
		"hold_xpoints":         holdXP,
		"policy_xpoints":       policyXP,
		"moves":                moves,
		"hits":                 hits,
	}
}

// reportOracleContrasts prints the paired difference between each pair of
// *oracled* arms, which is the one comparison the baseline-paired table cannot
// show and the only one that separates a reachable bound from an unreachable one.
//
// # Why this exists at all
//
// Two oracles over the same quantity at different resolutions do not measure two
// capabilities. Their **difference** is the third quantity, and it is usually the
// one worth having: perfect lineups bounds what better team news could buy, and
// perfect minutes minus perfect lineups is the residual nobody could have bought
// even in principle, because the minute a substitution is made is not known at the
// deadline by anybody including the manager. Reporting one number for the pair
// invites reading the whole of it as headroom — the error the armband oracle's
// decomposition corrected, where the bulk of +210 turned out to be an order
// statistic rather than a target.
//
// # Mean only — and that is a tooling gap, not a statistical fact
//
// The same rule reportPairedDifferences follows: the mean is the descriptive half
// and every standard error in this project is computed in stats/sweep_inference.R.
// R pairs each arm against the un-oracled baseline and has no arm-contrast
// estimator, so no SE is printed here.
//
// **Do not read that as "this contrast has no standard error".** Both arms sit on
// the same cells with the same baseline, so the baseline cancels and the contrast
// is a clean paired difference with exactly the inference any other paired
// difference has. It is simply not computed yet, and computing it in Go would be a
// second implementation of the estimator this project deliberately moved into R.
// Until the R side grows an arm contrast, quote a contrast's *mean* and say the
// inference is outstanding — never that it is unavailable in principle.
//
// # One contrast in three is not like-for-like, and the output does not say so
//
// The arms are nested in *information* and not in *reach*. `OracleAvailability`
// perturbs the bootstrap, so it moves the pre-season squad build; the minutes and
// lineups arms perturb `Engine.Recent`, which `blendRates` refuses to consult at
// `played == 0`. So at a GW1 entry availability reaches a decision the other two
// provably cannot, and a contrast against it subtracts two arms that acted at
// different seams rather than two resolutions of one fact. Only the
// minutes-versus-lineups contrast shares a seam and a classifier.
//
// Silent unless a sweep runs two or more oracled arms, which is the only
// arrangement where the contrast means anything.
func reportOracleContrasts(variants []policyVariant, cells []map[string]float64) {
	var idx []int
	for vi, v := range variants {
		if v.oracles.Active() {
			idx = append(idx, vi)
		}
	}
	if len(idx) < 2 {
		return
	}
	fmt.Printf("\n  contrasts between oracled arms (mean only; the SE is outstanding,\n")
	fmt.Printf("  not unavailable — these are paired differences on shared cells)\n")
	for a := 0; a < len(idx); a++ {
		for b := a + 1; b < len(idx); b++ {
			lo, hi := idx[a], idx[b]
			var diffs []float64
			for key, l := range cells[lo] {
				h, ok := cells[hi][key]
				if !ok {
					continue
				}
				diffs = append(diffs, h-l)
			}
			if len(diffs) < 2 {
				continue
			}
			fmt.Printf("    %-40s %+8.3f %8d\n",
				fmt.Sprintf("%s minus %s", variants[hi].oracles.Stamp(),
					variants[lo].oracles.Stamp()),
				meanOf(diffs), len(diffs))
		}
	}
}

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
