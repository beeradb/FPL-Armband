package backtest

// Sweeping the transfer policy on the quiet metrics.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTransferPolicy -v -timeout 90m
//
// Every transfer-policy result in AGENTS.md was measured on a single path from
// GW1, which is the noisiest thing this replay produces: nudging a scoring
// constant by 2% moves four-season points by 67, and the transfer path is where
// nearly all of that sensitivity lives. The file's own remedy is to average over
// entry points as well as seasons, because seasons are scarce and paths are not.
//
// This runs the whole matrix in-process — four season pairs times three start
// points per variant — instead of shelling out to the CLI, so a sweep costs
// minutes rather than an afternoon and the archives are parsed once.
//
// # Which metric
//
// POLICY, not HOLD. AGENTS.md is explicit that hold is for scoring constants
// and the policy line for constants that are *about* transfers, where the path
// is the thing being measured.
//
// HOLD is still reported, as an invariance check rather than a result. It makes
// no transfers, so a knob that only touches the transfer decision must leave it
// **byte-identical**. If HOLD moves while sweeping DecisionHorizon, the knob is
// leaking into scoring and the experiment is measuring two things again — which
// is the exact bug that made the original Horizon sweep uninterpretable.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// sweepBankLimit is the free-transfer bank every sweep cell uses, overriding
// BankLimitFor.
//
// FPL raised the bank from two to five for 2024-25, so the four replayed seasons
// straddle two different rule sets: 2022-23 and 2023-24 could save two, the
// later pair five. BankLimitFor reproduces that faithfully, which is right when
// reporting what a season would actually have scored — AGENTS.md is explicit
// that it exists so 2023-24 is not simulated saving transfers nobody could save.
//
// It is wrong for a *sweep*. Comparing a setting across cells governed by
// different transfer rules adds a nuisance factor that has nothing to do with
// the setting, and it interacts with exactly the knobs being swept: how many
// moves are affordable in a week changes what a gain threshold or a search
// structure can express. Holding the modern rule everywhere makes the cells
// directly comparable at the cost of historical fidelity in two of them, which
// is the right trade when the output is a contrast rather than a score.
//
// Absolute season totals produced this way are therefore **not** comparable with
// figures elsewhere in AGENTS.md; only the paired differences are.
const sweepBankLimit = 5

type policyVariant struct {
	label string
	apply func(*SimConfig)
	// plan installs a full two-set chip schedule from the cell's season and
	// entry point — the shipped user-facing planner, for arms that measure the
	// machine a user actually watches rather than the no-chip sweep machine.
	// Nil for every arm that does not; where set, the schedule overrides
	// whatever apply installed (the planner path is skipped, so a plan and an
	// apply-installed ChipPlanner cannot fight over one quantity).
	plan func(*Season, int) analysis.ChipSchedule
	// oracles is the hindsight this arm runs under, and it is **not** read from
	// here: runPolicySweep overwrites it with what apply actually installs, then
	// checks the label against it. So a hand-written arm cannot claim an oracle it
	// does not install, and oracleVariant is the only comfortable way to write
	// one. Zero — no hindsight — for every ordinary arm.
	oracles Oracles

	// setting reads this arm's swept value back out of the config apply
	// installed, and is written to the CSV's `setting` column.
	//
	// A getter rather than a number, for the same reason oracles is not read from
	// its own field: a declared value is a second expression of one quantity and
	// this package's signature bug is two such expressions drifting apart. Run on
	// the applied SimConfig, the column cannot describe a setting the cell did not
	// have.
	//
	// **Nil is meaningful and is the right value for most arms.** It says the
	// family varies no single scalar — an on/off arm, a two-dimensional one like
	// BONUS's prior/evidence pair, or a set of unrelated configurations — and
	// stats/schedule_screen.R needs to be *told* that rather than guessing it from
	// whether the labels happen to contain distinct numbers. Only a family that is
	// genuinely an ordered ladder should set it.
	setting func(SimConfig) float64

	// observe is called once per cell with the season this arm just played, for
	// an arm whose result is not one of the collected metrics.
	//
	// The chip-week oracle is the case it exists for: its whole output is
	// SimResult.ChipOracle and every metric the sweep collects must be
	// byte-identical to the baseline, so there is nothing for a paired difference
	// to say. Nil for every ordinary arm, and it may not touch the simulation —
	// the season is already played by the time it runs.
	observe func(pair seasonPair, start int, res *SimResult)
}

func runPolicySweep(t *testing.T, variants []policyVariant, starts []int) {
	t.Helper()
	cfg := loadConfig(t)

	// The per-cell CSV, when FPL_CELLS is set. Nil otherwise, and every call on
	// a nil sink is a no-op, so the sweep runs exactly as before when it is not.
	sink, err := openCellSink(os.Getenv("FPL_CELLS"))
	if err != nil {
		t.Fatal(err)
	}
	defer sink.close()
	// Labelled from EXP when a single block is being run, from the test name
	// otherwise, plus an ordinal so several blocks in one session stay separate.
	label := os.Getenv("EXP")
	if label == "" {
		label = t.Name()
	}
	sweep := sink.sweepLabel(label)

	// Parsed up front and cached process-wide, so the first variant is not
	// charged for work every later arm reuses.
	pairs := loadPairs(t, cfg)

	// Provenance, written *before* the first cell rather than after the last.
	//
	// That ordering is the whole feature. A sweep killed under load — which this
	// machine does routinely, and which AGENTS.md records happening four times to
	// one block, at 1, 3, 3 and 4 arms of 6 — leaves a cells file containing
	// however many arms finished and nothing saying how many were asked for. So
	// three arms of six reads downstream as a complete three-arm sweep, and the
	// only reason the real case was ever noticed is that somebody counted by hand.
	// Declaring the arms up front makes the gap arithmetic instead.
	//
	// It also stamps the constants in force. The costly failure this project has
	// actually had is not a wrong number but an orphaned one: a whole section was
	// measured with the transfer gate's minimum-gain threshold at 0.7, the value
	// was retracted to 0.4 three commits later, and nothing recorded the link, so
	// a later audit cited the section as ground truth.
	//
	// It is written *before* the oracle probe below, not after, and that ordering
	// matters for one of the two stamps it takes. `snapshot.FingerprintOf` reads
	// the process environment, and three variant sets in this package mutate it
	// inside `apply` — FPL_NO_AVAILABILITY and FPL_UNREGISTERED_POOL — so probing
	// every arm first would leave the *last* arm's switch set and record it as the
	// run's ambient setting. Per-cell behaviour was never affected; the sidecar was.
	// Declaring the arms needs only their labels, which exist already.
	writeSweepProvenance(t, sweep, sink, cfg, variants, pairs, starts)

	// What hindsight each arm installs, read off a probe config rather than off
	// the arm's own field — see oraclesOf. Done before the first cell, because
	// every check it feeds is a reason not to run the sweep at all.
	for vi := range variants {
		variants[vi].oracles = oraclesOf(cfg, starts[0], variants[vi])
	}
	if err := validateOracleArms(variants); err != nil {
		t.Fatal(err)
	}
	printOracleBanner(variants)

	fmt.Printf("\n%-22s", "variant")
	for _, st := range starts {
		fmt.Printf(" %8s", fmt.Sprintf("gw%d", st))
	}
	fmt.Printf(" %9s | %9s %7s\n", "POLICY", "HOLD", "moves")

	// Per-cell results, so variants can be compared as *paired* differences
	// rather than as two sums. Each (season, start) is one matched pair: the
	// same football, the same opening conditions, one setting changed.
	cells := make([]map[string]float64, len(variants))
	// The same, for the held opening fifteen. Scoring and squad-selection
	// constants belong on this metric, and judging them on POLICY paired
	// differences would be the same category error as reading their totals off
	// the transfer line.
	holdCells := make([]map[string]float64, len(variants))
	// HOLD re-scored with the armband pinned to the day-one pick, and with nobody
	// doubled at all. Both are candidate lower-noise instruments rather than
	// metrics in their own right — see HoldCaptaincy — and they cost nothing here
	// because HoldCaptaincyWeekly computes all three rungs in the single weekly
	// pass HOLD already pays for.
	fixedCapCells := make([]map[string]float64, len(variants))
	noCapCells := make([]map[string]float64, len(variants))
	// The same two headline metrics on the accumulated-xPoints instrument. They
	// cost nothing — `weekScoreWithChip` accumulates both totals from one set of
	// decisions, so POLICY's comes back on `SimResult` and HOLD's on the weekly
	// pass `HoldCaptaincyWeekly` is already paying for.
	//
	// **They are not metrics to decide on.** xPoints is a residual on four channels
	// and is under-smoothed by the bonus leak; what it is *for* is the paired SE
	// ratio against the points arm of the same contrast, which is the pilot's own
	// kill criterion. Read them beside POLICY and HOLD, never instead.
	xpCells := make([]map[string]float64, len(variants))
	holdXPCells := make([]map[string]float64, len(variants))
	// Transfer count and hits per cell. Not metrics — nobody reports a paired
	// difference in moves — but they are the sharpest invariance columns this
	// sweep has, because they are integers counted without noise. The armband
	// oracle pins both: `decide` never reads the captain, so a hindsight armband
	// that changed a transfer count would be reaching an axis it declared it
	// cannot reach, and one changed transfer is visible where a fraction of a
	// point a gameweek is not.
	moveCells := make([]map[string]float64, len(variants))
	hitCells := make([]map[string]float64, len(variants))

	var baseHold int
	for vi, v := range variants {
		byStart := map[int]int{}
		total, holdTotal, moves := 0, 0, 0
		infeasible := 0
		cells[vi] = map[string]float64{}
		holdCells[vi] = map[string]float64{}
		fixedCapCells[vi] = map[string]float64{}
		noCapCells[vi] = map[string]float64{}
		moveCells[vi] = map[string]float64{}
		hitCells[vi] = map[string]float64{}
		xpCells[vi] = map[string]float64{}
		holdXPCells[vi] = map[string]float64{}
		for _, start := range starts {
			for _, pair := range pairs {
				prior, cur := pair.Prior, pair.Cur
				// weeklyXI false: a variant that wants the imminent-gameweek
				// eleven sets it in apply, which several blocks do. Changing the
				// default here would silently move every block that does not.
				sc := sweepConfig(cfg, start, false)
				v.apply(&sc)
				if v.plan != nil {
					sc.ChipPlanner = nil
					sch := v.plan(pair.Cur, start)
					sc.Chips, sc.Chips2 = sch.First, sch.Second
				}
				// The stamp is read back out of the config the cell will run
				// under, so it cannot describe an oracle the simulation did not
				// get. An arm whose hindsight varies by cell would make the
				// per-arm declaration — and every check built on it — a lie.
				if sc.Oracles != v.oracles {
					t.Fatalf("variant %q installs %s at %s@%d and %s at the probe: "+
						"an arm's hindsight must not depend on the cell",
						v.label, sc.Oracles.Stamp(), pair.Name, start, v.oracles.Stamp())
				}
				// Identity is filled before the run, so a failure still has a
				// row to report. BankUpTo is read off the applied config rather
				// than from sweepBankLimit, so a variant that changes the bank
				// says so in the CSV.
				// MinExpMinutes is the *resolved* floor, not the raw field: 0
				// means the shipped 55 and only a negative means none, so
				// writing the field would put a 0 in the column for the
				// baseline and read downstream as a floorless arm.
				row := cellRow{
					Sweep: sweep, RunID: sink.run(), Variant: v.label,
					VariantIndex: vi, IsBaseline: vi == 0,
					Season: pair.Name, PriorSeason: pair.PriorName,
					StartGW: start, BankUpTo: sc.BankUpTo,
					MinExpMinutes: sc.resolvedMinExpectedMinutes(),
				}.under(sc.Oracles)
				if v.setting != nil {
					row.HasSetting, row.Setting = true, v.setting(sc)
				}
				res, err := Simulate(cur, prior, sc)
				if err != nil {
					// A variant can be infeasible rather than wrong — a high
					// enough pool floor leaves too few players to field a legal
					// fifteen. That is a result about the variant, not a broken
					// harness, so record it and carry on.
					//
					// The row is still emitted, flagged: a dropped cell reads
					// downstream as a comparison on fewer cells rather than as a
					// variant that failed, which is a different claim.
					infeasible++
					sink.cell(row.asInfeasible())
					continue
				}
				// Per gameweek *played*, not per season. A GW1 entry banks 38
				// gameweeks and a GW21 entry 18, so raw differences are not
				// comparable across start points: pooling them silently weights
				// the earliest regime twice as heavily and inflates the spread.
				cells[vi][fmt.Sprintf("%s@%d", pair.Name, start)] =
					float64(res.Points) / float64(len(res.Weeks))
				moveCells[vi][fmt.Sprintf("%s@%d", pair.Name, start)] =
					float64(res.Transfers)
				hitCells[vi][fmt.Sprintf("%s@%d", pair.Name, start)] =
					float64(res.Hits)
				if v.observe != nil {
					v.observe(pair, start, res)
				}
				byStart[start] += res.Points
				total += res.Points
				moves += res.Transfers
				// All three captaincy rungs from one weekly pass. Hold() would
				// recompute the same loop for the Full rung alone, and this
				// package's standing rule is that two expressions of one quantity
				// end with the measured one not being the one that runs.
				hc := HoldCaptaincyWeekly(cur, prior, sc, res.OpeningSquad)
				h := sumInts(hc.Full)
				fixedCap, noCap := sumInts(hc.FixedCaptain), sumInts(hc.NoCaptain)
				// HOLD's xPoints comes off the SAME weekly pass as HOLD's points —
				// hc.FullXP is hc.Full's mirror week by week — so the two readings
				// cannot be of different elevens.
				holdXP := sumFloats(hc.FullXP)
				key := fmt.Sprintf("%s@%d", pair.Name, start)
				holdCells[vi][key] = float64(h) / float64(len(res.Weeks))
				fixedCapCells[vi][key] = float64(fixedCap) / float64(len(res.Weeks))
				noCapCells[vi][key] = float64(noCap) / float64(len(res.Weeks))
				xpCells[vi][key] = res.XPoints / float64(len(res.Weeks))
				holdXPCells[vi][key] = holdXP / float64(len(res.Weeks))
				holdTotal += h

				row.Weeks = len(res.Weeks)
				// The mediator, as a column. Every squad-selection arm's "the
				// fifteen did not move" reading was previously a proxy off the
				// points columns, which cannot see a swapped fifteenth man who
				// is never fielded.
				row.SquadHash = squadHash(res.OpeningSquad)
				row.PolicyPoints, row.HoldPoints = res.Points, h
				row.Moves, row.Hits = res.Transfers, res.Hits
				row.HasCaptainRungs = true
				row.HoldFixedCaptain, row.HoldNoCaptain = fixedCap, noCap
				row.HasXPoints = true
				row.PolicyXPoints, row.HoldXPoints = res.XPoints, holdXP
				// What each chip actually returned, from the week it was played
				// in. Free — the season is already simulated — and emitted by
				// every sweep for the same reason the captaincy rungs are: the
				// question it answers can only be asked of a sweep whose effect
				// is already known, so recording it always costs nothing and
				// recording it on demand means it is missing when wanted.
				// The transfer-banking mediator, on every arm rather than the
				// banking ones. It is what makes a banking arm's null readable
				// at all — see bankingOf — and it is free, since Simulate has
				// already counted it.
				row.BankingMediator, row.HasBanking = bankingOf(res)
				// And the fixture-run funnel beside it, on every arm and for the
				// same reason: it is what makes a fixture-run arm's null readable,
				// and Simulate has already counted it.
				row.FixtureRuns = fixtureRunsOf(res)
				// And the four option-value funnels beside them, on every arm and
				// for the identical reason: a lever that is wired and inert
				// reports a clean null, and each of these is what tells that apart
				// from a lever that ran and never fired. Free — Simulate has
				// already counted them — and per lever rather than pooled, because
				// the four switches are independent and a null on one has to be
				// readable without reference to the others.
				row.TransferHold = res.TransferHold
				row.WildcardTrig = res.Wildcard
				row.BenchBoostTrig = res.BenchBoost
				row.FreeHitTrig = res.FreeHit
				row.ChipPrep = res.ChipPrep
				row.GateFloor = res.GateFloor
				// The per-cell fixture dose. It is NOT a mediator — it is a
				// function of the season and the entry gameweek alone, identical
				// on every arm of a cell — and it is read at the horizon this arm
				// actually built its opening squad on, because an arm that varies
				// the horizon varies its own dose. See DoseFor.
				//
				// ⚠️ Emitted only. Nothing here fits a slope, and a dose-response
				// needs its own pre-registration against the two traps in doseCols.
				row.HasDose = true
				d := DoseFor(pair.Cur, start, sc.Weights.Horizon)
				row.ActDoubles, row.ActBlanks = d.ActDoubles, d.ActBlanks
				row.LateDoubles, row.LateBlanks = d.LateDoubles, d.LateBlanks
				row.HasChipWeeks = true
				populateChipWeekColumns(t, fmt.Sprintf("%s@%d", pair.Name, start),
					res.Weeks, &row)
				// And the chip-week oracle's three readings of each chip, when the
				// cell was granted one. A no-op otherwise — res.ChipOracle is nil
				// unless AxisChipWeek is on — so this is unconditional for the same
				// reason the block above is: the diagnostic that wants these cannot
				// add them afterwards, and its table was the only place they have
				// ever existed.
				row.chipReadings, row.HasChipOracle = chipReadingsOf(res)
				sink.cell(row)
			}
		}
		if vi == 0 {
			baseHold = holdTotal
		}
		flag := ""
		if holdTotal != baseHold {
			flag = "  <-- HOLD MOVED"
		}
		if infeasible > 0 {
			flag += fmt.Sprintf("  [%d/%d runs INFEASIBLE]", infeasible, len(starts)*len(pairs))
		}
		fmt.Printf("%-22s", v.label)
		for _, st := range starts {
			fmt.Printf(" %8d", byStart[st])
		}
		fmt.Printf(" %9d | %9d %7d%s\n", total, holdTotal, moves, flag)
	}

	// Tier 2, immediately after the grid and before any table is read. An oracle
	// that reaches a decision it declared it cannot reach is measuring something
	// other than what its label says, and that is cheap to detect here and
	// expensive to notice later.
	series := invarianceSeries(cells, holdCells, fixedCapCells, noCapCells,
		holdXPCells, xpCells, moveCells, hitCells)
	for _, v := range oracleInvarianceViolations(variants, series) {
		t.Error(v)
	}
	// And the mirror. Every other guarantee here is a refusal, so an arm that
	// reaches nothing at all passes them all and reports the clean null an inert
	// arm reports. See oracleLivenessViolations.
	for _, v := range oracleLivenessViolations(variants, series) {
		t.Error(v)
	}

	// POLICY is refused outright, rather than reported with a caveat, when the grid
	// plays a season whose transfer path is not a sample of the same process.
	//
	// The only such season is 2019-20 — FPL granted unlimited free transfers before
	// the GW30+ deadline after the COVID restart and froze prices for three months —
	// and it reaches a sweep only through the `scoring` grid. The reason this is a
	// refusal and not a footnote is that a POLICY mean pooled over it is a plausible
	// number of the right magnitude with nothing about it to say which population it
	// came from, which is this package's signature failure: an orphaned figure that a
	// later audit cites as ground truth. HOLD is unaffected and is what the grid is
	// for. `sweep_inference.R` drops the same rows for the same reason, because the
	// cells file outlives this printout.
	var notComparable []string
	for _, p := range pairs {
		if !TransferPathComparable(p.Name) {
			notComparable = append(notComparable, p.Name)
		}
	}
	if len(notComparable) > 0 {
		fmt.Printf("\n--- POLICY: NOT REPORTED ---\n")
		fmt.Printf("This grid plays %s, whose transfer path and wallet are not samples of\n",
			strings.Join(notComparable, ", "))
		fmt.Printf("the same process as the others'. A pooled POLICY mean over it would be\n")
		fmt.Printf("indistinguishable from a valid one. Use HOLD, which this grid exists for.\n")
	} else {
		fmt.Printf("\n--- POLICY (the weekly transfer decision) ---")
		reportPairedDifferences(variants, cells, starts, "policy", sweep, sink)
		// The same season on the accumulated-xPoints instrument, under the same
		// refusal: 2019-20's transfer path is not a sample of the same process
		// whichever metric it is scored on.
		fmt.Printf("\n--- POLICY on accumulated xPoints (instrument; read beside POLICY) ---")
		reportPairedDifferences(variants, xpCells, starts, "policy_xpoints", sweep, sink)
	}
	fmt.Printf("\n--- HOLD (the opening fifteen; use this for scoring constants) ---")
	reportPairedDifferences(variants, holdCells, starts, "hold", sweep, sink)
	// The pilot's own metric, and the one the paired SE ratio is taken on. It is
	// printed after HOLD and never instead of it: the record's captaincy precedent
	// is that a quieter instrument can be a deafer one, so a lower-variance reading
	// is only worth having next to the figure it claims to sharpen.
	fmt.Printf("\n--- HOLD on accumulated xPoints (instrument; read beside HOLD) ---")
	reportPairedDifferences(variants, holdXPCells, starts, "hold_xpoints", sweep, sink)
	// The two captaincy rungs, reported after HOLD and never instead of it. They
	// are candidate instruments: a lower-noise reading of the same nudge is only
	// worth having if it keeps the effect, so read them beside HOLD's figure
	// rather than in place of it.
	fmt.Printf("\n--- HOLD with the armband pinned to the day-one pick (diagnostic instrument) ---")
	reportPairedDifferences(variants, fixedCapCells, starts, "hold_fixedcap", sweep, sink)
	fmt.Printf("\n--- HOLD with nobody doubled (diagnostic instrument; not what FPL pays) ---")
	reportPairedDifferences(variants, noCapCells, starts, "hold_nocap", sweep, sink)

	if sink == nil {
		fmt.Printf("\nNo SE, t or verdict here by design — inference lives in stats/sweep_inference.R.\n")
		fmt.Printf("Re-run with FPL_CELLS=/tmp/cells.csv to emit the per-cell rows it reads.\n")
	} else {
		fmt.Printf("\nCells written. Now: Rscript stats/sweep_inference.R <the FPL_CELLS path>\n")
	}
}

// reportPairedDifferences turns each variant-versus-baseline comparison into a
// mean paired difference and a cell count. The standard error, the df, the
// p-value and the multiplicity adjustment come from stats/sweep_inference.R —
// see "What it deliberately no longer reports" below.
//
// # Why this and not the totals
//
// A sweep produces four seasons times three start points, and collapsing that to
// one sum throws away the fact that they are **matched pairs**: the same
// football, the same opening conditions, one setting changed. The 12 differences
// carry their own scale, so the question "is this bigger than the noise?" is
// answerable per comparison instead of against a single remembered constant.
//
// A folklore noise floor — "anything under 300 is nothing" — is both too strict
// for a comparison whose cells agree and too generous for one whose cells
// disagree wildly. Two variants differing by +40 in every one of twelve cells is
// a real effect; two differing by +900 and -860 are indistinguishable. The
// totals column cannot tell those apart and this can.
//
// # What it deliberately no longer reports
//
// It used to print a naive SE, a season-clustered SE, two t ratios and a verdict
// word. Those are gone, and the point is that they are gone rather than
// improved: they were a second implementation of a quantity now computed in
// stats/sweep_inference.R, and this project's own rule for that is "two defaults
// for one quantity means the measured one is not the one that runs". The
// clustered SE in particular averaged four seasons and took their spread, with
// no small-sample correction and no principled df — a noisy estimate of noise
// presented beside a precise-looking t.
//
// The **mean is deliberately still computed here**, and that duplication is the
// exception that proves the rule: it is written to the .means.csv file and R
// asserts its own recomputation against it. A duplicate that is checked every
// run is a pipeline test; an unchecked one is the bug class.
// phase is a group of adjacent start points sharing an information regime.
// It carries no variance estimate of its own. reportRegimeNoise used to live
// here and compared each regime's SE against a 1/sqrt(weeks) null — the third
// hand-rolled variance estimator in this package, and dead code besides. R
// reproduces it from the CSV's season and start_gw columns.
type phase struct {
	label  string
	starts []int
}

// phasesOf splits start points into early, middle and late thirds.
//
// The blend weight is n/(n+k), so entry gameweek *is* the information regime: a
// GW1 entrant decides on the prior alone, a late one almost entirely on this
// season. Pooling adjacent starts is the only way to get enough cells for a
// variance estimate worth reading, since seasons are the scarce axis and each
// start point contributes just one cell per season.
func phasesOf(starts []int) []phase {
	if len(starts) < 3 {
		return nil
	}
	n := len(starts) / 3
	mk := func(label string, ss []int) phase {
		return phase{label: label, starts: ss}
	}
	return []phase{
		mk("early (prior-led)", starts[:n]),
		mk("middle", starts[n:2*n]),
		mk("late (season-led)", starts[2*n:]),
	}
}

func phaseDiffs(base, got map[string]float64, ph phase) []float64 {
	var out []float64
	for _, st := range ph.starts {
		suffix := fmt.Sprintf("@%d", st)
		for key, b := range base {
			if !strings.HasSuffix(key, suffix) {
				continue
			}
			if g, ok := got[key]; ok {
				out = append(out, g-b)
			}
		}
	}
	return out
}

// The mean is the descriptive half of a comparison and the only statistic this
// file still computes. meanOf, sd, seOf, meanSE and median all live in
// stats_test.go — a second copy of any of them here would be the same mistake at a
// smaller scale.
func reportPairedDifferences(variants []policyVariant, cells []map[string]float64,
	starts []int, metric, sweep string, sink *cellSink) {
	if len(variants) < 2 {
		return
	}
	fmt.Printf("\nPaired against %q — same season, same start, one setting changed.\n",
		variants[0].label)
	fmt.Printf("%-22s %8s %8s   %s\n", "variant", "mean", "n", "(SE, t and p: see stats/sweep_inference.R)")

	for vi := 1; vi < len(variants); vi++ {
		var diffs []float64
		for key, base := range cells[0] {
			got, ok := cells[vi][key]
			if !ok {
				continue
			}
			diffs = append(diffs, got-base)
		}
		if len(diffs) < 2 {
			continue
		}
		mean := meanOf(diffs)
		fmt.Printf("%-22s %+8.3f %8d\n", variants[vi].label, mean, len(diffs))
		sink.mean(sweep, metric, variants[vi].label, variants[0].label, vi, mean,
			len(diffs), variants[vi].oracles.Stamp())

		// Grouped into phases rather than reported per start point. Adding
		// start points does not add cells *per* start — each still has one per
		// season — so a per-start figure is a single cell. Means only: whether a
		// regime differs from another is an inference question and belongs in R,
		// which has the season and start_gw of every cell.
		for _, ph := range phasesOf(starts) {
			d := phaseDiffs(cells[0], cells[vi], ph)
			if len(d) < 3 {
				continue
			}
			fmt.Printf("    %-18s %+8.3f %8d\n", ph.label, meanOf(d), len(d))
		}
	}
	reportOracleContrasts(variants, cells)
	fmt.Printf("\nmean is points **per gameweek played**, so cells with different\n")
	fmt.Printf("horizons are comparable. Multiply by ~38 for a full-season figure.\n")
}

// TestDiagRejudge re-tests every result this project settled by summing four
// seasons and eyeballing the total against a remembered noise floor.
//
//	DIAG=1 EXP=H go test ./internal/backtest -run TestDiagRejudge -v -timeout 3h
//
// The threshold sweep went from "303-point spread, non-monotone, noise" at 12
// cells to a monotone ladder with t = +3.36 at 24, so the old method was capable
// of missing a real effect worth ~37 points a season. Everything decided that
// way is therefore unverified, and four of those decisions **shipped**.
//
// Each block names its metric. A scoring or squad-selection constant belongs on
// HOLD; only constants that are *about* transfers belong on POLICY. Reading the
// wrong one is the same category error as reading the wrong total.
func TestDiagRejudge(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()
	picker := newBlockPicker()
	defer picker.check(t)
	want := picker.want

	// H. min_gain_for_free_transfer, raised 0.4 -> 0.7 this session.
	//
	// Recorded at the time as the strongest of the shipped group, because its
	// *direction* came from an independent measurement — a buy-side bias of
	// 0.53 pts/gw — and only the confirmation came from totals.
	//
	// Both halves are **retracted**. The 0.53 does not reproduce at shipped
	// config (−0.230 median, +0.079 mean, and the asymmetry reverses sign), and
	// the totals confirmation reversed under paired differences, which is why
	// min_gain went back to 0.4. This arm is kept because the *no-op* result
	// below is real and reproduces: 0.0 and 0.4 are byte-identical, since the
	// second clause of the gate already demands 0.4 at horizon 5 and charge 2.
	if want("H") {
		fmt.Printf("\n=== H. min_gain (shipped 0.7). Metric: POLICY.\n")
		var v []policyVariant
		for _, g := range []float64{0.7, 0.0, 0.4, 0.95, 1.3} {
			label := fmt.Sprintf("min_gain=%.2f", g)
			if g == 0.7 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label,
				apply: func(sc *SimConfig) { sc.MinGain = g }})
		}
		runPolicySweep(t, v, starts)
	}

	// I. BlankRunPenalty, shipped at 0.75 on mechanism.
	//
	// A scoring change, so HOLD is the metric. On totals it was +21 held,
	// non-monotone, with POLICY disagreeing — squarely inside noise, and never
	// given a t.
	if want("I") {
		fmt.Printf("\n=== I. blank-run penalty (shipped 0.75). Metric: HOLD.\n")
		var v []policyVariant
		for _, b := range []float64{0.75, 1.0, 0.85, 0.6} {
			label := fmt.Sprintf("penalty=%.2f", b)
			if b == 0.75 {
				label += " (ships)"
			}
			if b == 1.0 {
				label = "off"
			}
			v = append(v, policyVariant{label: label,
				apply: func(sc *SimConfig) { sc.Weights.BlankRunPenalty = b }})
		}
		runPolicySweep(t, v, starts)
	}

	// J. MinExpectedMinutes, re-judged.
	//
	// Squad selection, so HOLD. A wrong verdict here is a missed change rather
	// than a shipped mistake.
	//
	// The "122-point HOLD spread" this block was once justified by is **block E's**
	// output, not this one's — block J did not exist at 194c654, and E's six arms
	// {-1, 30, 45, 55, 65, 75} are the only ones that can produce a table with 30
	// and 75 in it. It is also a **12-cell total** worth 13.8 a season, not 122.
	//
	// Re-run 2026-08-14 as paired differences on the six-season grid: +0 on the
	// twelve cells E measured, 2 of 36 opening fifteens moved overall, and on
	// points the contrast is *unmeasurable* rather than unresolved — with 2 of 6
	// seasons non-zero the clustered |t| is capped at 1.58 by construction.
	// stats/findings/2026-08-14-minfloor.md.
	if want("J") {
		fmt.Printf("\n=== J. pool floor (shipped 55). Metric: HOLD.\n")
		var v []policyVariant
		for _, f := range []float64{55, -1, 45, 65} {
			v = append(v, policyVariant{label: floorLabel(f),
				apply: func(sc *SimConfig) { sc.MinExpectedMinutes = f }})
		}
		runPolicySweep(t, v, starts)
	}

	// The captain-term shrink.
	//
	// xiValue counts the squad's highest scorer twice, once in the sum and once
	// as the captain, and TestDiagObjectiveDivergence measured that as the
	// objective's only real blind spot: buying a player over £9.0m, the
	// objective claims +1.460 pts/gw over the cheaper alternative and delivers
	// −0.258. A premium acquisition becomes that highest scorer, so the buy-side
	// over-rating is doubled along with him.
	//
	// captainShrink pulls the armband's value toward the runner-up, hardest
	// exactly when the best is far clear of the field. 1.0 is the old behaviour.
	//
	// **Both metrics matter here, unlike every other block.** Squad selection
	// uses this term — it is what stops the optimiser being indifferent between
	// a flat eleven and one built around a star — so a fix that helps transfers
	// and wrecks the opening fifteen is no fix at all.
	if want("CAPTAIN") {
		fmt.Printf("\n=== captain-term shrink. Metrics: HOLD *and* POLICY.\n")
		fmt.Printf("1.00 = ship today (armband worth the full top score).\n")
		fmt.Printf("0.00 = armband worth exactly the runner-up.\n")
		var v []policyVariant
		for _, sh := range []float64{1.0, 0.85, 0.7, 0.5} {
			label := fmt.Sprintf("shrink=%.2f", sh)
			if sh == 1.0 {
				label += " (ships)"
			}
			v = append(v, policyVariant{
				label: label,
				apply: func(sc *SimConfig) { analysis.SetCaptainShrink(sh) },
			})
		}
		runPolicySweep(t, v, starts)
		analysis.SetCaptainShrink(1.0)
	}

	// Fixture difficulty: does the opening squad want it at all?
	//
	// The recorded verdict is that the ladders are unresolvable — turning either
	// off is as good as shipped. That was measured on the *net* of two consumers
	// which fixture load has since shown want opposite things. The hypothesis
	// here is the same shape: difficulty is worth more to a transfer, which
	// deliberately buys a run of fixtures, than to a fifteen picked before any
	// fixture is near.
	//
	// The replay already builds separate engines for the opening squad and the
	// weekly decision, so this needs no new plumbing — just a different
	// FixtureWeight for the first. The transfer side stays at the configured
	// 0.65 throughout, so any movement is squad selection alone, and HOLD is
	// where it should show.
	if want("LADDER") {
		fmt.Printf("\n=== fixture weight on the OPENING SQUAD only.\n")
		fmt.Printf("Transfers stay at the configured value. HOLD is the metric.\n")
		var v []policyVariant
		for _, w := range []float64{-1, 0, 0.3, 1.0} {
			label := fmt.Sprintf("squad fixture wt %.2f", w)
			if w < 0 {
				label = "same as transfers (ships)"
			}
			v = append(v, policyVariant{
				label: label,
				apply: func(sc *SimConfig) {
					sc.WeeklyXI = true
					if w >= 0 {
						sc.SquadFixtureWeight = w
						sc.SquadFixtureWeightSet = true
					}
				},
			})
		}
		runPolicySweep(t, v, starts)
	}

	// The captain shrink, moved to the transfer objective.
	//
	// Applied in xiValue it reached every caller and measured negative on both
	// metrics. But the defect was measured on transfers alone — two searches
	// asked from a common squad, the objective claiming +1.460 pts/gw on a
	// £9.0m+ buy and delivering −0.258 — while squad construction needs the
	// term at full strength, since it is what stops the optimiser being
	// indifferent between a flat eleven and one built around a star.
	//
	// HOLD must not move: that is the test that the seam held.
	if want("CAPSEAM") {
		fmt.Printf("\n=== captain shrink in the transfer objective only.\n")
		fmt.Printf("HOLD must not move.\n")
		var v []policyVariant
		for _, sh := range []float64{1.0, 0.85, 0.7, 0.5} {
			label := fmt.Sprintf("shrink=%.2f", sh)
			if sh == 1.0 {
				label = "off (ships)"
			}
			v = append(v, policyVariant{
				label: label,
				apply: func(sc *SimConfig) {
					sc.WeeklyXI = true
					analysis.SetCaptainShrink(sh)
				},
			})
		}
		runPolicySweep(t, v, starts)
		analysis.SetCaptainShrink(1.0)
	}

	// Fixture load routed to the transfer objective only.
	//
	// Scaling Score everywhere gains on transfers (+0.891/gw) and costs on the
	// held opening fifteen (-1.654/gw): the signal is real but it corrupts squad
	// building, which is done before a ball is kicked and months from any
	// double. XIValue is reached only by transfer decisions — RankSwaps,
	// RankPairs, BuildPlans, the unified search — while squad construction goes
	// through the unexported xiValue and the eleven through BestXI. So applying
	// the load there should keep the gain and drop the damage.
	//
	// HOLD is the test. It must be byte-identical to the baseline: if it moves,
	// the signal has leaked into squad selection and the separation failed.
	if want("LOADTR") {
		fmt.Printf("\n=== fixture load in the transfer objective only.\n")
		fmt.Printf("HOLD must not move — that is the whole claim.\n")
		runPolicySweep(t, []policyVariant{
			{label: "XI only (ships)", apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				analysis.SetFixtureLoadTransfers(false)
			}},
			{label: "+ transfers see it", apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				analysis.SetFixtureLoadTransfers(true)
			}},
			{label: "everywhere (for scale)", apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				analysis.SetFixtureLoadTransfers(false)
				analysis.SetFixtureLoadWeeklyOnly(false)
			}},
		}, starts)
		analysis.SetFixtureLoadTransfers(true)
		analysis.SetFixtureLoadWeeklyOnly(true)
	}

	// Banking transfers toward a package the policy cannot otherwise afford.
	//
	// TestDiagBanking shows the weekly decision is greedy: 74% of weeks have
	// zero or one transfer in hand, and five weeks in four seasons make three
	// moves or more, which is the shape a premium switch needs. shouldBank adds
	// the missing comparison — the best package with one more transfer, valued
	// over a horizon one shorter, against the best available now.
	//
	// The fourth arm tests an interaction rather than the term alone. Scaling
	// Score by fixture load across the *horizon* lost 12 points a season and
	// drove transfers up 29%, which is the signature of seeing an opportunity it
	// cannot properly act on. A double three weeks out wants a squad assembled
	// for it, and assembling one needs banked transfers. Neither may work alone
	// while both work together.
	if want("BANK") {
		fmt.Printf("\n=== banking lookahead, and its interaction with fixture load.\n")
		runPolicySweep(t, []policyVariant{
			{label: "greedy (ships)", apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
			}},
			{label: "+ bank lookahead", apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				sc.BankLookahead = true
			}},
			{label: "+ horizon load", apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				analysis.SetFixtureLoadWeeklyOnly(false)
			}},
			{label: "+ bank AND horizon load", apply: func(sc *SimConfig) {
				sc.WeeklyXI = true
				sc.BankLookahead = true
				analysis.SetFixtureLoadWeeklyOnly(false)
			}},
		}, starts)
		analysis.SetFixtureLoadWeeklyOnly(true)
	}

	// Weighting last season's closing gameweeks above its opening ones.
	//
	// Three recency knobs exist and none does this: RateHalfLife and
	// MinutesHalfLife weight within the *current* season, prior_half_life blends
	// across seasons. The prior season itself is a flat total, so a player who
	// lost his place in March counts the same as one who won it.
	//
	// That is the direct answer to why a heavier prior failed. Raising
	// BlendRateK lost monotonically and lost *most* late in a season, which said
	// the prior is stale rather than merely weak. Weighting it toward the run-in
	// attacks the staleness instead of paying more for it.
	//
	// Minutes and rates are separate knobs because this project's standing
	// finding is that they behave oppositely — minutes reward sharp recency
	// because it removes a bias, rates punish it because a short window chases
	// finishing variance. Prediction: the minutes half of this helps and the
	// rate half does not.
	if want("PRIOR") {
		fmt.Printf("\n=== recency *inside* the prior season. Half-lives in gameweeks.\n")
		fmt.Printf("Prediction: weighting minutes helps, weighting rates does not.\n")
		runPolicySweep(t, []policyVariant{
			{label: "flat prior (ships)", apply: func(sc *SimConfig) {}},
			{label: "minutes hl=8", apply: func(sc *SimConfig) { sc.PriorMinutesHalfLife = 8 }},
			{label: "minutes hl=15", apply: func(sc *SimConfig) { sc.PriorMinutesHalfLife = 15 }},
			{label: "rates hl=15", apply: func(sc *SimConfig) { sc.PriorRateHalfLife = 15 }},
			{label: "both hl=15", apply: func(sc *SimConfig) {
				sc.PriorMinutesHalfLife = 15
				sc.PriorRateHalfLife = 15
			}},
		}, starts)
	}

	// How fast the model stops trusting last season.
	//
	// BlendRateK is the prior's strength when mixing last season's output rates
	// into this one, in 90s played: current-season weight is n/(n+k), so k=8
	// takes it from 33% at four matches to 67% at sixteen.
	//
	// TestDiagCalibrationDrift says that transition is too fast. The
	// expected/actual ratio is 1.013 through GW12, 0.916 from GW16 to GW28 and
	// 1.004 by GW32 — and it is the *expected* column that moves while actuals
	// stay flat, so the model is getting more confident rather than worse. Since
	// the inflation is concentrated at the top of a noisier distribution, more
	// shrinkage pulls the tail down relative to the middle: an ordering change,
	// not a level shift, which is what every failed flat correction was.
	//
	// **The prediction is directional and stated before the run**: raising k
	// should help, and should help most in the middle regime. AGENTS.md records
	// that this constant could not be resolved on the replay before — that was
	// three seasons, one path, judged on totals. This is 24 paired cells with a
	// hypothesis attached.
	//
	// Note k=8 is an out-of-sample predictive fit (MAE on xG/90). If the
	// decision metric prefers a larger value that is not a contradiction: this
	// project's standing finding is that an argmax wants more shrinkage than a
	// predictor does.
	//
	// **RUN 2026-08-14, and the prediction above is answered — do not re-register
	// it.** stats/snapshots/2026-08-14-blend/: 4 arms x 36 cells on the
	// six-season grid, HOLD reads -11.6 / +11.6 / +12.6 a season for 12/16/24,
	// non-monotone, Holm 1.000, nothing resolving, and 8 ships unchanged. So
	// "raising k should help" half failed — k=12 came back negative — and
	// "most in the middle regime" is **unsupported on entry columns**, where the
	// middle is the worst part of the curve. The calendar reading of that clause
	// is untested and needs a within-season decomposition this harness does not
	// produce.
	//
	// The run also **reverses the 24-cell ladder** recorded in the
	// constants-and-sweeps note, on that table's own four seasons.
	//
	// ⚠️ The arms below start at the shipped value, which is a design defect: it
	// cannot distinguish an interior optimum from an unbounded one. A re-run
	// wants settings below 8.
	if want("BLEND") {
		fmt.Printf("\n=== BlendRateK: how fast last season stops counting.\n")
		fmt.Printf("Prediction: higher helps, most in the middle regime.\n")
		var v []policyVariant
		for _, k := range []float64{8, 12, 16, 24} {
			label := fmt.Sprintf("BlendRateK=%.0f", k)
			if k == 8 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label,
				apply:   func(sc *SimConfig) { sc.Weights.BlendRateK = k },
				setting: func(sc SimConfig) float64 { return sc.Weights.BlendRateK }})
		}
		runPolicySweep(t, v, starts)
	}

	// BLENDLO — the same ladder with the arms the banked run should have had.
	//
	// BLEND's arms are 8/12/16/24 and 8 is the shipped value, so the ladder starts
	// at its own baseline and **cannot distinguish an interior optimum from a
	// slope**. Its one clean feature — 12 worse than both its neighbours — is
	// unreadable for that reason: with nothing below 8 there is no left-hand
	// neighbour for 8 itself, and "12 is a trough" and "the surface is rough" have
	// the same evidence.
	//
	// This block re-runs the whole ladder in ONE comparison rather than adding two
	// arms to be read beside the banked four. Two runs cannot be stitched: R keys a
	// comparison on (run_id, sweep) precisely so that separate runs of one block do
	// not pool into an over-confident sample, and the banked run is at a different
	// commit. Six arms is ~16 minutes against the recorded four-arm 10m49s.
	//
	// It is also the first ladder to carry declared settings, so the screen's slope
	// is licensed by the design rather than by the arm labels.
	//
	// **Pre-registered before the run**, in
	// stats/snapshots/2026-08-14-blendlo/PREREGISTRATION.md:
	//
	//   - The primary question is SHAPE, not a winner. "The optimum is below 8"
	//     requires k=3 or k=5 to be positive against 8; the standing finding that
	//     an argmax wants MORE shrinkage than a predictor predicts the opposite
	//     sign, so a positive low arm is evidence against a documented principle
	//     rather than a free win.
	//   - **Expect no arm to resolve.** The banked arms ran CR2 SEs of 0.40-0.55
	//     pts/gw against a t_crit of 2.571 on df 5, so the detectable effect is
	//     ~39-54 points a season and the whole recorded ladder spans 24. This is
	//     run for the shape and to give `k=8` a left-hand neighbour, and a null is
	//     the expected result rather than a disappointment.
	//   - Monotonicity across all six rungs would be the one readable outcome, and
	//     it is not forced by the construction: nothing removes a line item here.
	//   - Judged on `HOLD`. `BlendRateK` is a scoring constant, so POLICY adds the
	//     transfer path's 303-point noise floor to a question about rates.
	if want("BLENDLO") {
		fmt.Printf("\n=== BlendRateK, the full ladder including arms below the shipped value.\n")
		fmt.Printf("Question: is 8 in a trough or on a slope? Shape, not a winner.\n")
		var v []policyVariant
		// 8 first: variants[0] is the baseline every paired difference is taken
		// against, and it must be the shipped value for the ladder to read as
		// "against what ships". The rest ascend.
		for _, k := range []float64{8, 3, 5, 12, 16, 24} {
			label := fmt.Sprintf("BlendRateK=%.0f", k)
			if k == 8 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label,
				apply:   func(sc *SimConfig) { sc.Weights.BlendRateK = k },
				setting: func(sc SimConfig) float64 { return sc.Weights.BlendRateK }})
		}
		runPolicySweep(t, v, starts)
	}

	// BLEND2 — the pre-split semantics, which is what the recorded ladder swept.
	//
	// This exists because BLEND's arms are **not the same intervention** they were
	// when the table in the constants-and-sweeps note was recorded, and that
	// was invisible for two rounds of review.
	//
	// At c261f32 — the commit that both records that table and introduces
	// want("BLEND") — `shrinkToLeague` read the same constant:
	//
	//	blend.go:407   k := e.Weights.BlendRateK        // c261f32
	//	blend.go:407   k := e.Weights.LeagueShrinkK     // today, since c509255
	//
	// c509255 ("Split LeagueShrinkK from BlendRateK", 2026-08-10) gave the league
	// anchor its own constant, and the BLEND block above still sets only
	// BlendRateK. So the recorded ladder moved **two** anchors together — the
	// personal prior's strength AND the pull of a priorless player's rates toward
	// his position's league-wide rates — while today's moves one.
	//
	// That is this record's own "folding two levers into one arm measures their
	// sum and neither", pointed at the recorded row. It also qualifies the
	// resident claim that splitting them "changed nothing": that is a
	// simple-effect null taken at 8/8, and every pre-split arm ABOVE 8 moved both.
	//
	// Setting both to k reproduces the old arm exactly, so this is the one-run
	// test of whether the reversal is a changed estimand rather than engine drift.
	if want("BLEND2") {
		fmt.Printf("\n=== BlendRateK AND LeagueShrinkK together: the pre-split arm.\n")
		fmt.Printf("Prediction: this reproduces the recorded ladder; BLEND alone does not.\n")
		var v []policyVariant
		for _, k := range []float64{8, 12, 16, 24} {
			label := fmt.Sprintf("both k=%.0f", k)
			if k == 8 {
				label += " (ships)"
			}
			// Both anchors move together, so the family is still an ordered
			// ladder in one number even though two fields change — which is
			// exactly the distinction label parsing could not draw. The getter
			// reads BlendRateK and the guard below checks the two agree, so an
			// arm that set only one would fail rather than report a slope in a
			// setting half its cells did not have.
			v = append(v, policyVariant{label: label,
				apply: func(sc *SimConfig) {
					sc.Weights.BlendRateK = k
					sc.Weights.LeagueShrinkK = k
				},
				setting: func(sc SimConfig) float64 {
					if sc.Weights.LeagueShrinkK != sc.Weights.BlendRateK {
						panic(fmt.Sprintf("BLEND2 arm has BlendRateK=%v and "+
							"LeagueShrinkK=%v: the pre-split arm moves both",
							sc.Weights.BlendRateK, sc.Weights.LeagueShrinkK))
					}
					return sc.Weights.BlendRateK
				}})
		}
		runPolicySweep(t, v, starts)
	}

	// Fixture load on the imminent gameweek rather than the horizon.
	//
	// Run with FPL_FIXTURE_LOAD=1, since the scaling is an env-gated term.
	// Scaling the horizon score lost 12 points a season with transfers and 53
	// held; the hypothesis is that the term is right and the *window* was wrong.
	// A double this Saturday genuinely doubles a player's return that week,
	// where a double in April should barely move who you buy today.
	if want("WEEKXI") {
		fmt.Printf("\n=== fixture load: horizon against imminent gameweek.\n")
		fmt.Printf("FPL_FIXTURE_LOAD is %v.\n", os.Getenv("FPL_FIXTURE_LOAD") != "")
		runPolicySweep(t, []policyVariant{
			{label: "load off (baseline)", apply: func(sc *SimConfig) {
				analysis.SetFixtureLoad(false)
				analysis.SetFixtureLoadWeeklyOnly(false)
			}},
			{label: "load on, XI horizon", apply: func(sc *SimConfig) {
				analysis.SetFixtureLoad(true)
				analysis.SetFixtureLoadWeeklyOnly(false)
			}},
			{label: "load on, XI this week", apply: func(sc *SimConfig) {
				analysis.SetFixtureLoad(true)
				analysis.SetFixtureLoadWeeklyOnly(false)
				sc.WeeklyXI = true
			}},
			{label: "load on XI only", apply: func(sc *SimConfig) {
				analysis.SetFixtureLoad(true)
				analysis.SetFixtureLoadWeeklyOnly(true)
				sc.WeeklyXI = true
			}},
		}, starts)
		analysis.SetFixtureLoad(false)
		analysis.SetFixtureLoadWeeklyOnly(true)
	}

	// The buy-side discount, conditioned on acquisition.
	//
	// TestDiagTransferError measures the player being bought as over-rated by
	// 0.53 pts/gw while the player sold is well calibrated, and
	// TestDiagObjectiveDivergence shows the damage concentrated in premiums,
	// where the captain term doubles the error.
	//
	// Unlike min_gain — which was raised to this figure and retracted — this
	// corrects the incoming player's *score*, so it flows through XIValue and
	// therefore through the captain term, doubling exactly where the error
	// doubles. Both metrics are reported: the discount only touches the transfer
	// search, so HOLD must not move.
	if want("BUY") {
		fmt.Printf("\n=== buy-side discount on the incoming player. Metric: POLICY.\n")
		fmt.Printf("HOLD must not move — this touches the transfer search only.\n")
		var v []policyVariant
		for _, d := range []float64{0, 0.25, 0.53, 0.8} {
			label := fmt.Sprintf("discount=%.2f", d)
			if d == 0 {
				label = "off (ships)"
			}
			v = append(v, policyVariant{
				label: label,
				apply: func(sc *SimConfig) { analysis.SetBuyDiscount(d) },
			})
		}
		runPolicySweep(t, v, starts)
		analysis.SetBuyDiscount(0)
	}

	// The PRICES block is gone. It ran the whole grid *once*, printing whichever
	// arm the process environment selected, and the measurement was completed by
	// running the test twice and comparing two totals by eye — because the oracle
	// was a package-level var read at init and could not be varied within a run.
	// It cannot be a paired comparison in that shape, so it produced a bare point
	// estimate with no cell count for a figure later quoted as a hard ceiling.
	//
	// With the oracle on SimConfig both arms run in one process and pair
	// properly, which is exactly what TestDiagPriceTimingSignificance does. Two
	// blocks measuring one quantity is this package's most-repeated bug, so the
	// weaker one is deleted rather than kept alongside. EXP=PRICES now fails
	// loudly through blockPicker.check, naming the blocks that do exist.

	// K. The unified search: three claims at once.
	//
	// The shipped per-move gain threshold, the 265-point cost of removing the
	// pool floor from it — the single number carrying the "argmax protection"
	// conclusion — and unified against bespoke at +59, which was called a tie.
	if want("K") {
		fmt.Printf("\n=== K. unified search. Metric: POLICY.\n")
		runPolicySweep(t, []policyVariant{
			{label: "bespoke (ships)", apply: func(sc *SimConfig) {}},
			{label: "uni, per-move gain", apply: func(sc *SimConfig) { sc.Unified = true }},
			{label: "uni, per-decision", apply: func(sc *SimConfig) {
				sc.Unified = true
				sc.UnifiedGainPerDecision = true
			}},
			{label: "uni, no pool floor", apply: func(sc *SimConfig) {
				sc.Unified = true
				sc.UnifiedPoolFloor = -1
			}},
		}, starts)
	}
}

// TestDiagProjection re-tunes the scoring constants at the current resolution.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagProjection -v -timeout 6h
//
// Every constant here was last measured on three entry points or fewer, judged
// on totals. That method has since been shown capable of both missing a real
// effect — the transfer threshold read as noise at 12 cells and t = +3.36 at 24
// — and manufacturing one, since min_gain reversed direction and was retracted.
//
// **HOLD is the metric throughout.** These are scoring and squad-selection
// constants, and this project's standing rule is that the held opening fifteen
// is the quieter line for anything that is not *about* transfers. POLICY is
// printed alongside but should not decide.
//
// Each block is paired against the shipped value, so the question asked is
// always "is anything better than what we run", not "which of these is largest".
func TestDiagProjection(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()
	picker := newBlockPicker()
	defer picker.check(t)
	want := picker.want

	base := func(sc *SimConfig) { sc.WeeklyXI = true }

	// Minutes recency. Flat against 4 was worth about 200 points on the old
	// method, and 2, 8 and 20 all beat 4 on a later re-sweep — the value was
	// never distinguished, only the presence of recency.
	if want("MINHL") {
		fmt.Printf("\n=== minutes half-life (ships 4). Metric: HOLD.\n")
		var v []policyVariant
		for _, x := range []float64{4, 0, 2, 8, 20} {
			label := fmt.Sprintf("half-life %.0f", x)
			if x == 4 {
				label += " (ships)"
			}
			if x == 0 {
				label = "flat (no recency)"
			}
			v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
				base(sc)
				sc.Weights.MinutesHalfLife = x
			}})
		}
		runPolicySweep(t, v, starts)
	}

	// The convexity exponent on minutes reliability. This is the constant whose
	// 2% nudge moved four-season points by 67 and whose 1.15 sat 300 below both
	// neighbours — the original evidence that the response surface is a step
	// function.
	// 1.15 is in the ladder deliberately, and it is not a filler value. This file
	// records it at 8007 against 8281 at 1.0 and 8304 at 1.25 — three hundred below
	// BOTH neighbours — and that single observation is the primary evidence for the
	// claim that the replay's response surface is a step function rather than a
	// gradient, which is where the ±150 noise floor governing every sweep here comes
	// from. It was measured on four-season totals down a single path, by the method
	// that was later retired for missing the transfer threshold and manufacturing
	// the min_gain result. Adding the arm re-tests it under paired differences at
	// almost no cost.
	//
	// Adding an arm is safe for the other comparisons: each paired difference is
	// arm-minus-baseline within a cell, so the existing four are untouched. It does
	// widen the family a Holm adjustment corrects over, which is a real and accepted
	// cost — the shape across five settings is worth more here than a marginally
	// stronger adjusted p-value on any one of them.
	if want("MINW") {
		fmt.Printf("\n=== minutes convexity exponent (ships 1.0). Metric: HOLD.\n")
		var v []policyVariant
		for _, x := range []float64{1.25, 1.0, 1.15, 1.5, 1.75} {
			label := fmt.Sprintf("exponent %.2f", x)
			if x == 1.0 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
				base(sc)
				sc.Weights.MinutesWeight = x
			}})
		}
		runPolicySweep(t, v, starts)
	}

	// The bonus schedule: how much of a player's own historical bonus rate to
	// believe, interpolated on how much of it is current-season evidence.
	if want("BONUS") {
		fmt.Printf("\n=== bonus schedule, prior/evidence (ships 0.5/1.5). Metric: HOLD.\n")
		type pair struct{ prior, evidence float64 }
		var v []policyVariant
		for _, x := range []pair{{0.5, 1.5}, {1.0, 1.0}, {0.5, 2.0}, {0, 2.0}, {1.5, 1.5}} {
			label := fmt.Sprintf("%.1f / %.1f", x.prior, x.evidence)
			if x.prior == 0.5 && x.evidence == 1.5 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
				base(sc)
				sc.Weights.BonusPriorWeight = x.prior
				sc.Weights.BonusWeight = x.evidence
			}})
		}
		runPolicySweep(t, v, starts)
	}

	// Couples the clean sheet to a defender's own defensive workload. Shipped at
	// 0.3 on the mechanism, measured on the single season that has defcon.
	if want("DCC") {
		fmt.Printf("\n=== defcon/clean-sheet coupling (ships 0.3). Metric: HOLD.\n")
		var v []policyVariant
		for _, x := range []float64{0.3, 0, 0.6, 1.0} {
			label := fmt.Sprintf("coupling %.1f", x)
			if x == 0.3 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
				base(sc)
				sc.Weights.DefConCleanCoupling = x
			}})
		}
		runPolicySweep(t, v, starts)
	}

	// What the opening fifteen credits a bench player at. Flagged PRIORITY: the
	// held metric inverted toward 0.02 and the two metrics disagreed.
	if want("BENCH") {
		fmt.Printf("\n=== opening bench weight (ships 0.10). Metric: HOLD.\n")
		var v []policyVariant
		for _, x := range []float64{0.10, 0.02, 0.05, 0.20} {
			label := fmt.Sprintf("bench weight %.2f", x)
			if x == 0.10 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
				base(sc)
				sc.BenchWeight = x
			}})
		}
		runPolicySweep(t, v, starts)
	}

	// How much of the fixture-adjusted rate is blended in, for both consumers at
	// once — the squad-only split was tested separately and found no seam.
	if want("FIXW") {
		fmt.Printf("\n=== fixture weight, both consumers (ships 0.65). Metric: HOLD.\n")
		var v []policyVariant
		for _, x := range []float64{0.65, 0, 0.35, 1.0} {
			label := fmt.Sprintf("fixture weight %.2f", x)
			if x == 0.65 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
				base(sc)
				sc.Weights.FixtureWeight = x
			}})
		}
		runPolicySweep(t, v, starts)
	}

	// The minutes prior's strength, in matches. Its companion BlendRateK was
	// re-judged today and confirmed at 8; this half never was.
	if want("MINK") {
		fmt.Printf("\n=== minutes prior strength (ships 5). Metric: HOLD.\n")
		var v []policyVariant
		for _, x := range []float64{5, 1, 3, 8, 12} {
			label := fmt.Sprintf("BlendMinutesK %.0f", x)
			if x == 5 {
				label += " (ships)"
			}
			v = append(v, policyVariant{label: label, apply: func(sc *SimConfig) {
				base(sc)
				sc.Weights.BlendMinutesK = x
			}})
		}
		runPolicySweep(t, v, starts)
	}
}

// TestDiagTransferPolicy separates the two jobs Horizon does at once.
//
// Horizon sets how far ahead the fixture average looks *and*, through
// `gain x horizon >= charge`, how demanding the transfer gate is. Sweeping it
// moves transfer counts from 55 to 109, which is the second effect leaking into
// the first. DecisionHorizon exists to pin them apart, and AGENTS.md records
// that the separation has never been re-run since the scoring fixes landed.
func TestDiagTransferPolicy(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()
	values := []int{2, 3, 4, 5, 6, 8}
	// Each block is ~20 minutes, so allow running one at a time.
	picker := newBlockPicker()
	defer picker.check(t)
	want := picker.want

	if want("A") {
		fmt.Printf("\n=== A. transfer threshold (DecisionHorizon), fixture window pinned at 5\n")
		fmt.Printf("HOLD must not move: it makes no transfers.\n")
		// Baselined on the shipped value, so the paired column answers the
		// question that matters — is anything better than what we run? —
		// rather than everything against the extreme.
		a := []policyVariant{{
			label: "threshold=5 (ships)",
			apply: func(sc *SimConfig) {
				sc.Weights.Horizon = 5
				sc.DecisionHorizon = 5
			},
		}}
		for _, v := range values {
			if v == 5 {
				continue
			}
			a = append(a, policyVariant{
				label: fmt.Sprintf("threshold=%d", v),
				apply: func(sc *SimConfig) {
					sc.Weights.Horizon = 5
					sc.DecisionHorizon = v
				},
			})
		}
		runPolicySweep(t, a, starts)
	}

	if want("B") {
		fmt.Printf("\n=== B. fixture window (Weights.Horizon), threshold pinned at 5\n")
		fmt.Printf("HOLD is expected to move here — the window changes who is fielded.\n")
		var b []policyVariant
		for _, v := range values {
			b = append(b, policyVariant{
				label: fmt.Sprintf("window=%d", v),
				apply: func(sc *SimConfig) {
					sc.Weights.Horizon = v
					sc.DecisionHorizon = 5
				},
			})
		}
		runPolicySweep(t, b, starts)
	}

	// C. The correction the error diagnostic implies.
	//
	// TestDiagTransferError puts the buy-side over-rating at roughly 0.5-0.6
	// pts/gw and — once the funding legs are separated out — flat in the
	// modelled gain rather than growing with it. A flat correction on the buy
	// side is arithmetically the same thing as demanding that much more gain
	// before moving, which is exactly what MinGain does. So the measured bias
	// implies a specific value for a shipped constant, and this tests it rather
	// than sweeping for an argmax.
	if want("C") {
		fmt.Printf("\n=== C. min_gain, the constant the buy-side bias implies\n")
		fmt.Printf("Shipped is 0.40; the measured buy-side error implies about 0.95.\n")
		fmt.Printf("HOLD must not move.\n")
		var c []policyVariant
		for _, v := range []float64{0.0, 0.4, 0.7, 0.95, 1.3} {
			c = append(c, policyVariant{
				label: fmt.Sprintf("min_gain=%.2f", v),
				apply: func(sc *SimConfig) {
					sc.Weights.Horizon = 5
					sc.DecisionHorizon = 5
					sc.MinGain = v
				},
			})
		}
		runPolicySweep(t, c, starts)
	}

	// D. The blank-run availability correction.
	//
	// Unlike A-C this is a *scoring* change, so HOLD is the metric that matters
	// and it is expected to move. The policy column is reported alongside, but
	// with a ~300-point noise band it can only corroborate, not decide.
	if want("D") {
		fmt.Printf("\n=== D. blank-run discount (availability)\n")
		fmt.Printf("A scoring change, so HOLD is the metric and SHOULD move.\n")
		var d []policyVariant
		for _, v := range []float64{1.0, 0.85, 0.75, 0.6} {
			label := fmt.Sprintf("penalty=%.2f", v)
			if v == 1.0 {
				label = "off (was shipped)"
			}
			d = append(d, policyVariant{
				label: label,
				apply: func(sc *SimConfig) { sc.Weights.BlankRunPenalty = v },
			})
		}
		runPolicySweep(t, d, starts)
	}

	// E. The rotation-risk cliff on the opening squad.
	//
	// A *cliff*, not a discount: below it a player is dropped from the pool
	// rather than scored lower. It has never been measured, and
	// TestDiagAvailability showed it removes every established player with two
	// trailing blanks — an enormous claim for an unmeasured constant.
	//
	// It filters the opening fifteen only; the weekly transfer search ranks over
	// the whole pool. So HOLD is the metric, and most of any movement should
	// appear at the GW1 start.
	//
	// Note the £4.5m fodder exemption in squad.go: cheap reserves are exempt
	// from this floor whatever it is set to, so raising it cannot empty the
	// bench, and lowering it mostly changes who is eligible for the eleven.
	if want("E") {
		fmt.Printf("\n=== E. MinExpectedMinutes, the opening squad's rotation cliff\n")
		fmt.Printf("Squad selection, so HOLD is the metric. Shipped is 55.\n")
		var ee []policyVariant
		for _, v := range []float64{-1, 30, 45, 55, 65, 75} {
			ee = append(ee, policyVariant{
				label: floorLabel(v),
				apply: func(sc *SimConfig) { sc.MinExpectedMinutes = v },
			})
		}
		runPolicySweep(t, ee, starts)
	}

	// F. Unifying the two searches, retried.
	//
	// Optimize builds a squad; RankSwaps/RankPairs revise one. They are the same
	// problem under different constraints, and OptimizeRequest.MaxChanges can
	// express both — "the best squad within k changes of the one I own", which
	// also reaches 2-up/2-down, something the bespoke structure cannot.
	//
	// It was tried and lost 39 points. That verdict is worth re-testing for two
	// reasons. It was measured on a single GW1 path, which is the noisiest
	// metric here — the same harness now puts ~300 points of jitter on the
	// transfer path alone. And it predates minutes-only reliability, the
	// threshold split, defcon visibility, the bonus schedule, availability
	// reconstruction and the min_gain fix, any of which could have been what it
	// was really measuring.
	//
	// There is also a confound in the original: the unified arm filters
	// candidates at MinExpectedMinutes 55 while the bespoke search ranks over
	// the whole pool with no floor. So it is run both ways, because otherwise
	// this measures the filter rather than the search.
	if want("F") {
		fmt.Printf("\n=== F. bespoke against unified bounded revision\n")
		fmt.Printf("HOLD must not move: the opening squad is built the same way in both.\n")
		runPolicySweep(t, []policyVariant{
			{label: "bespoke (shipped)", apply: func(sc *SimConfig) {}},
			{label: "unified, floor 55", apply: func(sc *SimConfig) { sc.Unified = true }},
			{label: "unified, no floor", apply: func(sc *SimConfig) {
				sc.Unified = true
				sc.UnifiedPoolFloor = -1
			}},
		}, starts)
	}

	// G. Stopping net value from choosing k.
	//
	// The unified search loops k = 1..limit and keeps whichever k scores best on
	// net value — an argmax over the same noisy estimate that produced each
	// candidate. TestDiagTransferError measures each player bought as over-rated
	// by ~0.53 pts/gw, so a k-move revision inflates its own gain by about
	// k x 0.53 and the argmax preferentially picks the most inflated option.
	//
	// Two ways to stop it, one derived and one swept:
	//
	//   per-move gain   the threshold scales with k, which is what the bespoke
	//                   search already does and what the measured per-player
	//                   optimism implies. No new constant.
	//   surcharge       an escalating charge, k(k-1)/2. A free parameter.
	if want("G") {
		fmt.Printf("\n=== G. how k is chosen in the unified search\n")
		fmt.Printf("HOLD must not move: this is the weekly decision only.\n")
		g := []policyVariant{
			{label: "bespoke (shipped)", apply: func(sc *SimConfig) {}},
			{label: "uni, gain per decision", apply: func(sc *SimConfig) {
				sc.Unified = true
				sc.UnifiedGainPerDecision = true
			}},
			{label: "uni, gain per move", apply: func(sc *SimConfig) { sc.Unified = true }},
		}
		for _, v := range []float64{0.5, 1.0, 2.0} {
			g = append(g, policyVariant{
				label: fmt.Sprintf("uni, per-dec +surch %.1f", v),
				apply: func(sc *SimConfig) {
					sc.Unified = true
					sc.UnifiedGainPerDecision = true
					sc.UnifiedSurcharge = v
				},
			})
		}
		runPolicySweep(t, g, starts)
	}
}

// TestDiagPriceTimingSignificance puts a standard error on "perfect price
// timing is worth about 16 points a season".
//
//	DIAG=1 go test ./internal/backtest -run TestDiagPriceTimingSignificance -v -timeout 60m
//
// The predecessor of this test measured the price oracle by running the whole
// sweep twice — once per process, since the oracle was read from
// FPL_ORACLE_PRICES at package init — and comparing the two totals by eye. That
// gives a bare point estimate with no SE or t, unlike every other 24-cell result
// in AGENTS.md, even though the figure is used as a hard ceiling ("if a bound
// this generous is small, nothing tighter can be large").
//
// The oracle is now a bit on SimConfig.Oracles, so the two arms are paired cells
// within one process and reportPairedDifferences applies without any new
// statistics. It also drops a mutated package global and the deferred restore
// that only held as long as nothing panicked mid-sweep.
//
// # The invariance is the interesting half
//
// A price advantage cannot reach squad building: the opening fifteen is bought
// before any transfer is made, and this oracle perturbs the *transaction* seam
// only — the optimiser is still quoted the real NowCost. So all three held rungs
// must come back byte-identical to the baseline arm's, in every cell.
// OracleTransactPrice declares exactly that and oracleInvarianceViolations enforces it
// after the grid, where it used to be checked by reading a column by hand.
func TestDiagPriceTimingSignificance(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()
	fmt.Printf("\n=== perfect price timing, full grid: %s.\n",
		gridLabel(len(sweepPairNames()), len(starts)))
	fmt.Printf("Metric: POLICY. This is a hindsight upper bound, not a shippable setting.\n")
	runPolicySweep(t, []policyVariant{
		{label: "real prices (ships)", apply: func(sc *SimConfig) {}},
		oracleVariant(Oracles{Info: OracleTransactPrice}, "perfect timing", nil),
	}, starts)
}
