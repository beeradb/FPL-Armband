package backtest

// Which metric can each setting actually reach?
//
//	DIAG=1 EXP=reach FPL_SWEEP_SEASONS=default FPL_SWEEP_STARTS=1,16 \
//	  scripts/replay -run TestDiagMetricReach -v -timeout 3h
//
// # The gap this closes
//
// The rule that a scoring constant belongs on `HOLD` and a transfer constant on
// `POLICY` exists only as prose and as scattered per-constant comments. Nothing
// enumerates it and nothing checks it — while the *oracle* arms have exactly this
// machinery: each declares `MustNotMove()` and `MustMove()`, and
// `oracleInvarianceViolations` / `oracleLivenessViolations` enforce both on every
// grid. Ordinary settings have no equivalent, and the gap has a measured cost.
//
// It catches three things prose cannot. A knob reaching a metric it should not,
// which is a leak. A knob reaching **nothing at all**, which is the
// byte-identical null this record calls its signature failure. And it tells a
// sweeper in advance which metric is even capable of answering their question —
// the wasteful direction being the expensive one, since `POLICY` is `HOLD` plus a
// transfer path whose own noise is 303 points of spread.
//
// # What a negative cell means here, stated before anyone reads one
//
// **"Moved" is proof of reach. "Did not move" is NOT proof of no reach.** The
// response surface is a step function — a 2% nudge to one exponent has moved
// four-season points by 67 — so a knob can reach a metric and still be
// byte-identical at one particular setting. Every arm below is therefore probed
// at an **extreme of its legal domain** rather than at a nudge, which is the
// strongest available perturbational evidence, and a zero is still reported as
// *no reach observed at this setting* rather than as *cannot reach*.
//
// ⚠️ The strongest form of the negative is **static**, not perturbational: name
// the consumer on that path, per the standing rule that naming a consumer is the
// check and naming a package is not. A purely perturbational map reproduces, in
// its negative cells, the very failure it exists to fix. The mechanical version
// is a poison value that panics if read, turning "did not move" into "was not
// read"; it is not built here and is recorded as owed.
//
// # Conditional knobs, which are how this becomes a trap
//
// Some knobs only act where a condition holds — `PrepareTripleCaptain` only where
// the chip is placed, which is 23 of 36 cells on the full grid; `BankUpTo` at the
// wildcard and free-hit accrual. A cell chosen without regard to that returns "no
// reach" for a knob that is merely idle there. So each arm below carries a
// `fires` note, and the report prints the **cell count** each column moved in
// rather than a bare boolean, so an idle arm and an inert arm are distinguishable.
//
// # Cost
//
// The `SimConfig` knobs are perturbable in process, one arm each. Two of the
// `internal/analysis` package vars have exported setters —
// `analysis.SetUnifiedAppearance` and `analysis.SetDerivedBenchSlots` — so they
// ride in the same process; only `FPL_RELIABILITY_SPLIT` and `FPL_MINUTES_WEIGHT`
// genuinely need an exec each, and they are out of scope for this pass.
//
// Run it on the four-season grid with two entry points: this is a reachability
// question, not an effect-size one, so cells buy nothing beyond giving each
// conditional knob somewhere to fire.
//
// ⚠️ **Every arm here is a simple-effect null, because the baseline is not the
// shipped configuration.** `base` sets `WeeklyXI = true` while `runPolicySweep`
// calls `sweepConfig` with `false`. That is deliberate — several blocks in this
// package do the same, and changing it would silently move them — but it means a
// zero below is "no reach observed at this setting, from this baseline". For
// knobs whose consumers are disjoint from `WeeklyXI`'s single one
// (`simulate.go:1110`, the weekly fielding engine) it is harmless; nothing here
// establishes that for the table as a whole.
//
// ⚠️ **And the cells file cannot self-verify an arm's applied value.**
// `runPolicySweep` writes the applied `BankUpTo` into `cellRow` precisely so a
// variant that changes the bank says so in the CSV; there is no equivalent column
// for any knob below. Until there is, "the arm arrived" rests on a code reading
// plus a sibling arm that moves — which is what the 89 arm is for.

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// reachArm is one knob at one extreme of its legal domain.
type reachArm struct {
	label string
	// fires records the condition under which the knob can act at all, so a
	// zero-reach row can be read as "idle here" rather than "inert".
	fires string
	apply func(sc *SimConfig)
}

func TestDiagMetricReach(t *testing.T) {
	requireDiag(t)

	// The map reads its own cells back, so it needs the sink on whether or not
	// the operator set it. An explicit FPL_CELLS wins, because a run whose cells
	// nobody keeps is a run nobody can re-derive.
	cellPath := os.Getenv("FPL_CELLS")
	if cellPath == "" {
		cellPath = filepath.Join(t.TempDir(), "reach.csv")
		t.Setenv("FPL_CELLS", cellPath)
	}

	base := func(sc *SimConfig) { sc.WeeklyXI = true }
	with := func(f func(sc *SimConfig)) func(*SimConfig) {
		return func(sc *SimConfig) { base(sc); f(sc) }
	}

	arms := []reachArm{
		// Transfer-decision knobs. The record's claim is that these are confined
		// to decide(), which HOLD never calls — a theorem for two of them and a
		// contingent fact for the others, which is the distinction this map
		// exists to compute rather than assert.
		{"min_gain = 3.0 (far above the binding 0.4)", "every transfer week",
			with(func(sc *SimConfig) { sc.MinGain = 3.0 })},
		{"free_transfer_value = 0", "every transfer week",
			with(func(sc *SimConfig) { sc.MinGainHit = 0 })},
		{"bank_up_to = 1", "wildcard and free-hit accrual",
			with(func(sc *SimConfig) { sc.BankUpTo = 1 })},
		{"max_hits = 0", "any week the search wants a hit",
			with(func(sc *SimConfig) { sc.MaxHits = 0 })},
		{"decision_horizon = 1", "every transfer week; also oracleWindow's fallback",
			with(func(sc *SimConfig) { sc.DecisionHorizon = 1 })},
		{"bank_lookahead on", "weeks with a transfer banked",
			with(func(sc *SimConfig) { sc.BankLookahead = true })},

		// Squad-selection and scoring knobs. These should reach HOLD.
		// -1, not 0. This label is what a reader copies to reproduce the arm, and
		// 0 is the shipped 55 — see floorLabel. Not folded into floorLabel: that
		// one names a rung of a floor ladder, this names a knob in a table of
		// knobs, and the two formats are read side by side with different columns.
		{"min_expected_minutes = -1 (no floor; 0 is the shipped 55)", "the opening build, every cell",
			with(func(sc *SimConfig) { sc.MinExpectedMinutes = -1 })},
		{"min_expected_minutes = 89 (near-total floor)", "the opening build, every cell",
			with(func(sc *SimConfig) { sc.MinExpectedMinutes = 89 })},
		{"bench_weight = 0", "the opening build, every cell",
			with(func(sc *SimConfig) { sc.BenchWeight = 0.0001 })},
		{"bench_weight = 0.30 (past the recorded cliff)", "the opening build, every cell",
			with(func(sc *SimConfig) { sc.BenchWeight = 0.30 })},
		{"minutes_half_life = 1", "every scored gameweek",
			with(func(sc *SimConfig) { sc.MinutesHalfLife = 1 })},
		{"minutes_half_life = 20", "every scored gameweek",
			with(func(sc *SimConfig) { sc.MinutesHalfLife = 20 })},
		{"weekly_xi off", "every scored gameweek",
			func(sc *SimConfig) { sc.WeeklyXI = false }},

		// Chip-preparation knobs. Conditional by construction, and the reason
		// this report prints cell counts rather than booleans.
		{"prepare_bench_boost on", "only cells that place a bench boost",
			with(func(sc *SimConfig) { sc.PrepareBenchBoost = true })},
		{"prepare_triple_captain on", "only cells that place a triple captain",
			with(func(sc *SimConfig) { sc.PrepareTripleCaptain = true })},

		// The two analysis-package knobs that have in-process setters. They are
		// restored by the defer below, not by a trailing statement.
		{"unified appearance off (legacy 0.624 rule)", "every scored gameweek",
			func(sc *SimConfig) { base(sc); analysis.SetUnifiedAppearance(false) }},
		{"fixed bench tuple (derived slots off)", "the opening build, every cell",
			func(sc *SimConfig) { base(sc); analysis.SetDerivedBenchSlots(false) }},
	}

	priorOut, priorGK, priorDerived := analysis.BenchSlotState()
	defer func() {
		analysis.SetUnifiedAppearance(true)
		analysis.SetDerivedBenchSlots(priorDerived)
		analysis.SetBenchSlots(priorOut, priorGK)
	}()

	// Every arm must restore the package state the previous arm may have
	// changed, because apply runs per cell and the arms are not independent
	// otherwise. Wrapping here rather than in each arm is what stops the next
	// person forgetting.
	variants := []policyVariant{{label: "shipped (baseline)", apply: func(sc *SimConfig) {
		analysis.SetUnifiedAppearance(true)
		analysis.SetDerivedBenchSlots(true)
		base(sc)
	}}}
	for _, a := range arms {
		f := a.apply
		variants = append(variants, policyVariant{label: a.label, apply: func(sc *SimConfig) {
			analysis.SetUnifiedAppearance(true)
			analysis.SetDerivedBenchSlots(true)
			f(sc)
		}})
	}

	runPolicySweep(t, variants, sweepStarts())
	reportReach(t, cellPath, arms)
}

// outcomeColumns are the cell columns a reachability claim can be made about.
// Deliberately not every column: season, start and weeks are identity, and
// bank_up_to is provenance.
var outcomeColumns = []string{
	"policy_points", "hold_points", "moves", "hits",
	"hold_fixedcap_points", "hold_nocap_points",
	"frozen_points", "frozen_captain_points", "weekly_points",
	"bench_boost_pts", "triple_captain_pts",
}

func reportReach(t *testing.T, path string, arms []reachArm) {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading back the cells this sweep wrote: %v", err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil || len(rows) < 2 {
		t.Fatalf("cells file at %s is unreadable or empty: %v", path, err)
	}

	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	for _, want := range append([]string{"variant", "is_baseline", "season", "start_gw"}, outcomeColumns...) {
		if _, ok := col[want]; !ok {
			t.Fatalf("cells file has no %q column: the schema moved and this map "+
				"would silently report no reach for it", want)
		}
	}

	type cellKey struct{ season, start string }
	baseline := map[cellKey][]string{}
	byArm := map[string]map[cellKey][]string{}
	for _, r := range rows[1:] {
		k := cellKey{r[col["season"]], r[col["start_gw"]]}
		if r[col["is_baseline"]] == "true" {
			baseline[k] = r
			continue
		}
		v := r[col["variant"]]
		if byArm[v] == nil {
			byArm[v] = map[cellKey][]string{}
		}
		byArm[v][k] = r
	}
	if len(baseline) == 0 {
		t.Fatal("no baseline cells: every reach verdict below would be vacuous")
	}

	// ⚠️ A column this grid does not populate reports a clean zero for every arm,
	// which reads exactly like "nothing reaches this metric". The first run of
	// this map printed five such columns — frozen_points, frozen_captain_points
	// and weekly_points are empty strings unless the frozen rungs are collected,
	// and both chip columns are 0 in every cell of a grid that places no chip —
	// so eleven columns of verdict were really six. That is the
	// assertion-instead-of-computation failure this map exists to fix, arriving
	// inside the map. They are separated out rather than dropped, because
	// "unpopulated" and "unreached" are the distinction the whole file is about.
	var live, dead []string
	for _, c := range outcomeColumns {
		populated := false
		for _, r := range baseline {
			if v := r[col[c]]; v != "" && v != "0" {
				populated = true
				break
			}
		}
		if populated {
			live = append(live, c)
			continue
		}
		dead = append(dead, c)
	}

	fmt.Printf("\n--- metric reachability: cells (of %d) in which each column differs from the baseline ---\n",
		len(baseline))
	fmt.Printf("A zero is NO REACH OBSERVED AT THIS SETTING, never 'cannot reach'.\n")
	fmt.Printf("Read the `fires` column before believing a zero: an idle knob and an inert one look identical here.\n")
	if len(dead) > 0 {
		fmt.Printf("\n⚠️ %d column(s) are NOT POPULATED by this grid and carry no verdict about any arm:\n  %v\n",
			len(dead), dead)
		fmt.Printf("   Their zeros would otherwise read as 'nothing reaches this metric'. Excluded below.\n")
	}
	fmt.Printf("\n")

	fmt.Printf("%-46s", "arm")
	for _, c := range live {
		fmt.Printf(" %10s", short(c))
	}
	fmt.Printf("\n")

	var inert []string
	for _, a := range arms {
		cells := byArm[a.label]
		fmt.Printf("%-46s", trunc(a.label, 46))
		total := 0
		for _, c := range live {
			n := 0
			for k, row := range cells {
				b, ok := baseline[k]
				if !ok {
					continue
				}
				if !sameNumber(row[col[c]], b[col[c]]) {
					n++
				}
			}
			total += n
			fmt.Printf(" %10d", n)
		}
		fmt.Printf("\n")
		if total == 0 {
			inert = append(inert, a.label+"  (fires: "+a.fires+")")
		}
	}

	fmt.Printf("\nReached nothing at this setting — check the `fires` condition before\n")
	fmt.Printf("concluding the knob is inert, and prefer a named consumer to this table:\n")
	if len(inert) == 0 {
		fmt.Printf("  (none)\n")
	}
	sort.Strings(inert)
	for _, s := range inert {
		fmt.Printf("  - %s\n", s)
	}
	fmt.Printf("\n")
}

// sameNumber compares two cell values numerically where both parse, and as
// strings otherwise. Byte equality alone would report a reach for "1" against
// "1.0", which the writer can produce for an integer-valued float column.
func sameNumber(a, b string) bool {
	if a == b {
		return true
	}
	x, errA := strconv.ParseFloat(a, 64)
	y, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil {
		return false
	}
	return x == y
}

func short(c string) string {
	if len(c) <= 10 {
		return c
	}
	return c[:10]
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
