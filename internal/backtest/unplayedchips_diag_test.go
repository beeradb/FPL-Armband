package backtest

// What are the chips the replay never plays worth?
//
//	DIAG=1 FPL_CELLS=/work/drop/unplayed-chips-2026-08-31/cells/unplayedchips.csv \
//	  go test ./internal/backtest -run TestDiagUnplayedChipValue -count=1 -v -timeout 6h
//
// Pre-registered: memory/2026-08-30-prereg-what-are-the-unplayed-chips-worth-and-do-they-
// explain-the-2025-26-collapse.md, committed before any number here existed. The design
// below is that prereg's, not this file's, and nothing in it may be changed to suit a
// result.
//
// # The question
//
// `sweepConfig` installs no chip plan, and `TestDiagCensus` part 4 asserts that it never
// will. So every constant sweep, every transfer-policy sweep and every placement figure
// in this project's record was measured by a manager who played **zero chips** while
// every real manager in the same field played theirs. `ChipSetsFor` returns 1 before
// 2025-26 and 2 from it, and `analysis.ChipPlan` carries four chips, so the handicap is
// four chips a season through 2024-25 and **eight** in 2025-26 — and 2025-26 is the
// season the engine's placement collapsed from ≈379k to ≈1.70M. This measures the
// handicap. It does not, and cannot, attribute the collapse to it.
//
// # Arms — identical but for the chip allowance
//
//   - **A** — no chips. Bit-for-bit the machine every banked sweep cell ran on.
//   - **B** — `FullAnchoredPlan`, the shipped user-facing planner, under the allowance
//     the season **actually granted**: one set for 2020-21..2024-25, two for 2025-26.
//
// ⚠️ `ChipSets` is left at zero in both arms, which is "ask the season" — so
// `SplitChipSetsWith` and `ValidateChipSetsWith` both route through `ChipSetsFor`.
// **`ChipSetsForced` is deliberately NOT used.** It replays an older season under
// today's rules, which is a different question, and since the whole hypothesis here is
// that the ALLOWANCE differed by season it would destroy the contrast rather than
// sharpen it. `assertGrantedAllowance` pins that: five seasons must come back with an
// empty second set and 2025-26 with a full one, so a later edit to the planner or to
// `ChipSetsFor` cannot silently turn this into the forced arm.
//
// # Why arm B installs a schedule and not a `ChipPlanner`
//
// `FullAnchoredPlan` returns both sets at once, and a `ChipPlan` holds one slot per
// chip — so a single-plan planner routed through `SplitChipSets` can only ever fill one
// of the two sets, and 2025-26's second set would simply not exist. `policyVariant.plan`
// is the field for this and it nils `ChipPlanner` on the way through, so the two cannot
// fight over one quantity. This is the same wiring `cmd/armband/backtest.go` uses for
// `FPL_CHIP_PLAN=anchored`, which is the point: the arm is the machine a user watches.
//
// # Entry point and horizon
//
// One start point, GW1, 38 weeks. Not the shipped six-start grid, and the reason is not
// economy. Entry points are strictly nested — `SimConfig` carries no `EndGW` — so a GW21
// entry shares 18 of its weeks with a GW1 entry, and a chip set is a **season-shaped**
// object: the first set expires at GW19, so a GW21 entrant is granted only half of what
// a GW1 entrant is. Pooling the two would average a four-chip season with a two-chip one
// and report the mean as "what chips are worth". At one start per season the six cells
// are one per cluster, and the clustered SE the prereg names is exactly the SE of the
// six season means.
//
// # The reproduction gate, and why it fires before arm B
//
// Arm A must return the six banked totals below. They are in
// `stats/snapshots/2026-08-28-bonusweight/cells/bonusweight.csv` and in
// `/work/drop/variance-frontier-2026-08-30/seasons.csv`, byte-identical across two runs
// on two different diagnostics. A drift there means this process is not the machine those
// figures were measured on, and every delta computed against it would be a comparison
// between two harnesses wearing the clothes of one. `runPolicySweep` runs variants in
// order, so arm A completes first and the gate `Fatal`s before arm B is paid for.
//
// ⚠️ **A pinned figure in an ordinary test would rot.** This one is gated behind `DIAG=1`
// and skipped by `go test ./...` for that reason: it is a correctness precondition for a
// measurement, not an invariant of the engine.
//
// # The void condition this file counts rather than assumes
//
// `chipsets.go` warns that across six archived first halves there are 15 doubling
// club-gameweeks out of 189 and **11 of the 15 are one COVID-rescheduled 2020-21 round**.
// A chip is worth something different in a season with nothing to spend it on, so the
// per-season doubling census is computed here and written beside every delta. It is not
// an adjustment and nothing is divided by it — it is the context without which a
// per-season number cannot be read.
//
// # What is decision-bearing and what is not
//
// Decision-bearing: the pooled paired delta B−A in points per gameweek, season-clustered,
// df 5, t_crit 2.571, against a threshold of 2.571×SE. The inference itself is
// `stats/sweep_inference.R`'s, from the cells file — not recomputed here, for the reason
// `reportPairedDifferences` gives.
//
// Not decision-bearing, and printed as **shape**: which chip carries the value, whether
// the second set lands on doubles, and 2025-26's delta against the five one-set seasons.
// That last is one season against five; it cannot resolve at df 5 and the prereg
// pre-declares it as shape. Confirming it needs 2026-27, the second two-set season.

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// unplayedChipsRunDir is where this run's cells and its summary land. Named for the
// run and hard-coded rather than taken from the environment, the way
// TestDiagVarianceFrontier's is: the directory is part of the record the prereg points
// at, and a run that silently wrote somewhere else would be a result nobody can find.
const unplayedChipsRunDir = "/work/drop/unplayed-chips-2026-08-31"

// bankedNoChipPolicyPoints is arm A's expected POLICY total per season, from a GW1
// entry at the sweep bank.
//
// **Measured, not asserted** — and measured twice, on two diagnostics that share no code
// beyond `sweepConfig`: `stats/snapshots/2026-08-28-bonusweight/cells/bonusweight.csv`
// (the shipping bonus-weight arm) and `/work/drop/variance-frontier-2026-08-30/seasons.csv`
// (the frontier baseline). They agree byte for byte, which is what makes them usable as a
// gate rather than as a memory.
var bankedNoChipPolicyPoints = map[string]int{
	"2020-21": 1963,
	"2021-22": 2336,
	"2022-23": 2086,
	"2023-24": 2301,
	"2024-25": 2433,
	"2025-26": 2137,
}

// doublingCensus is one season's blank-and-double calendar, counted from the fixture
// list alone.
//
// ⚠️ **Both halves are read, and that is the point of the type rather than a detail.**
// The prereg voids on *"a season whose fixture archive cannot express the BLANKS AND
// doubles the chips are for"*, and an earlier version of this file computed `blankWeeks`
// and printed only the doubles — so the free hit, which is the blank chip and which
// `FullAnchoredPlan` anchors on *"the window's biggest blank"*, had no column anywhere
// saying whether it ever landed on one. Half a void condition checked reads exactly like
// a whole one.
//
// Club-gameweeks, not gameweeks: a round in which three clubs play twice is three, which
// is the unit `chipsets.go`'s 15-of-189 figure is quoted in. Split at `ChipResetGW`
// because that is where the chip sets split, so the second-set number is the one that
// says whether 2025-26's extra four chips had anything to aim at.
type doublingCensus struct {
	season      string
	firstHalf   int // doubling club-gameweeks in GW1..ChipResetGW-1
	secondHalf  int // and in ChipResetGW..38
	doubleWeeks map[int]int
	blankWeeks  map[int]int // clubs idle in a round the league otherwise played
	firstBlank  int         // blanking club-gameweeks in GW1..ChipResetGW-1
	secondBlank int         // and in ChipResetGW..38
}

func (d doublingCensus) total() int      { return d.firstHalf + d.secondHalf }
func (d doublingCensus) blankTotal() int { return d.firstBlank + d.secondBlank }

// doublingCensusOf reads a season's calendar into the two halves the chip sets split at.
//
// ⚠️ It delegates to `censusOf`, and that is load-bearing rather than tidy. Classifying
// a round — zero is a blank, two or more is a double — is already spelled four times in
// this package, `censusOf`'s own comment says nothing will catch a fifth, and the trap it
// documents is specific: `count[gw]` is nil for a round with no fixtures at all, so the
// natural loop reads every club as blanking and reports 2022-23 GW7 as a twenty-club
// blank. `censusOf` skips unplayed rounds before reading the field. Re-spelling it here
// would have inherited exactly that misreading into the number this prereg voids on.
//
// It reads the FIXTURE LIST and nothing else — no scoreline, no `Finished` flag, no
// player row — so it needs no gameweek gate. That is the same information
// `FullAnchoredPlan` plans on and the same a real manager has; the point-in-time hazard
// in this package lives in `playedFixtures`, which this never touches.
func doublingCensusOf(cur *Season) doublingCensus {
	d := doublingCensus{
		season:      cur.Name,
		doubleWeeks: map[int]int{},
		blankWeeks:  map[int]int{},
	}
	for _, w := range censusOf(cur) {
		if !w.played {
			continue
		}
		if w.doubling > 0 {
			d.doubleWeeks[w.gw] = w.doubling
			if w.gw < ChipResetGW {
				d.firstHalf += w.doubling
			} else {
				d.secondHalf += w.doubling
			}
		}
		if w.blanking > 0 {
			d.blankWeeks[w.gw] = w.blanking
			if w.gw < ChipResetGW {
				d.firstBlank += w.blanking
			} else {
				d.secondBlank += w.blanking
			}
		}
	}
	return d
}

// chipPlay is one chip, the week it was played in, whether that week doubled, and
// what the simulator recorded the chip returning in that week.
//
// ⚠️ `gain` exists for the bench boost and the triple captain only, and is a WEEK-LOCAL
// quantity: the bench's points, and the captain's extra copy. There is no equivalent for
// the wildcard or the free hit, because neither has an effect confined to its own week —
// a wildcard's value is the squad it leaves behind. `hasGain` says which is which, so a
// missing figure reads as "not measurable this way" rather than as a zero.
//
// ⚠️ It is collected HERE rather than read off the cells file, and the reason is a real
// defect in the shared column. `runPolicySweep` writes `bench_boost_gw`/`_pts` by
// looping over the weeks and assigning on every hit, so a TWO-SET season — which plays
// two bench boosts and two triple captains — keeps only the LAST of each and silently
// drops the first-set chip. That is exactly 2025-26, the one season the whole two-set
// hypothesis is about.
type chipPlay struct {
	name     string
	gw       int
	doubling int // doubling clubs in that gameweek; 0 is a plain week
	blanking int // and clubs idle in it; 0 means every club played
	gain     int
	hasGain  bool
}

// armCell is one (season, arm) row of the summary CSV.
type armCell struct {
	season  string
	arm     string
	points  int
	weeks   int
	plays   []chipPlay
	census  doublingCensus
	firstN  int // chips placed in the first set
	secondN int // and in the second
}

func (c armCell) chipsPlayed() int { return len(c.plays) }

func TestDiagUnplayedChipValue(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)

	// ---- Part 0: the void conditions, before any simulation ----
	//
	// Every one of these is cheap and every one of them is a reason not to run the
	// sweep at all. They come first so a wiring fault costs seconds rather than hours.
	assertOneStartPoint(t, len(pairs))
	assertGrantedAllowance(t, pairs)

	census := map[string]doublingCensus{}
	for _, p := range pairs {
		census[p.Name] = doublingCensusOf(p.Cur)
	}
	printCensus(pairs, census)

	// ---- Part 1: the two arms ----
	//
	// Collected through `observe`, which runs after the cell is scored and may not
	// touch the simulation. `runPolicySweep` iterates variants, starts and pairs on
	// one goroutine, so these appends need no lock — and if that ever changes, the
	// completeness check below is what notices.
	var rows []armCell
	// `plan` is the arm's OWN planner, nil for the arm that plays none. It is
	// threaded through rather than looked up, so the "chips placed" columns describe
	// what THIS arm was given: reading FullAnchoredPlan unconditionally put "4
	// placed" beside "0 played" on every no-chip row, which is a row that contradicts
	// itself and the kind a reader trusts because it is arithmetic.
	collect := func(arm string, plan func(*Season, int) analysis.ChipSchedule) func(seasonPair, int, *SimResult) {
		return func(pair seasonPair, start int, res *SimResult) {
			c := armCell{
				season: pair.Name, arm: arm,
				points: res.Points, weeks: len(res.Weeks),
				census: census[pair.Name],
			}
			for _, w := range res.Weeks {
				for _, k := range []struct {
					on      bool
					name    string
					gain    int
					hasGain bool
				}{
					{w.Wildcard, "wildcard", 0, false},
					{w.FreeHit, "free_hit", 0, false},
					{w.BenchBoost, "bench_boost", w.BenchBoostGain, true},
					{w.TripleCaptain, "triple_captain", w.TripleCaptainGain, true},
				} {
					if k.on {
						c.plays = append(c.plays, chipPlay{
							name: k.name, gw: w.GW,
							doubling: c.census.doubleWeeks[w.GW],
							blanking: c.census.blankWeeks[w.GW],
							gain:     k.gain, hasGain: k.hasGain,
						})
					}
				}
			}
			if plan != nil {
				sch := plan(pair.Cur, start)
				c.firstN, c.secondN = PlacedChips(sch.First), PlacedChips(sch.Second)
			}
			rows = append(rows, c)
		}
	}

	const armA, armB = "no chips (the machine every banked cell ran on)",
		"chips as the season granted them"

	arms := []policyVariant{
		{
			label: armA,
			apply: func(sc *SimConfig) {},
			observe: func(pair seasonPair, start int, res *SimResult) {
				// The reproduction gate. Fatal rather than Error, and NOT
				// because arm B is expensive — the whole twelve-cell run takes
				// about thirty seconds, and an earlier version of this comment
				// claimed hours. It is Fatal because a delta taken against a
				// baseline that is not the banked one is a comparison between
				// two harnesses wearing the clothes of one, and continuing would
				// produce a complete-looking table nobody can source.
				if want, ok := bankedNoChipPolicyPoints[pair.Name]; ok && res.Points != want {
					t.Fatalf("arm A did NOT reproduce the banked no-chip baseline for "+
						"%s from GW%d: got %d, banked %d.\n"+
						"Those six totals are byte-identical across two independent "+
						"diagnostics, so a drift here means this process is not the "+
						"machine they were measured on — and every chip delta computed "+
						"against it would be comparing two harnesses. Stopping before "+
						"arm B rather than reporting a number nobody can source.",
						pair.Name, start, res.Points, want)
				}
				collect(armA, nil)(pair, start, res)
			},
		},
		{
			label:   armB,
			apply:   func(sc *SimConfig) {},
			plan:    FullAnchoredPlan,
			observe: collect(armB, FullAnchoredPlan),
		},
	}

	// Checked on the arms themselves, after they exist, rather than on a
	// hand-written copy of them: a guard that inspects a second expression of the
	// arms is a second thing to keep in sync, which is the drift this package
	// records over and over.
	assertArmsDifferOnlyInChips(t, cfg, arms)

	fmt.Printf("\n=== what are the chips the replay never plays worth?\n")
	fmt.Printf("Arm A plays none — the machine every banked sweep cell ran on.\n")
	fmt.Printf("Arm B plays FullAnchoredPlan under ChipSetsFor: one set through\n")
	fmt.Printf("2024-25, two in 2025-26. Metric: POLICY. Entry GW1, 38 weeks.\n")
	fmt.Printf("Primary: the pooled paired delta, season-clustered, df 5,\n")
	fmt.Printf("t_crit 2.571 — from stats/sweep_inference.R on the cells file.\n")

	runPolicySweep(t, arms, []int{1})

	// ---- Part 2: completeness, before anything is read ----
	//
	// An INFEASIBLE cell is skipped by `runPolicySweep` before `observe`, so a
	// refused chip plan would leave this summary short a row and every table below
	// would quietly describe a smaller population. That is not hypothetical: an
	// anchored-chips arm once lost all six 2025-26 cells to `ValidateChipSets` while
	// every printed number stayed plausible. Counted, not assumed.
	want := 2 * len(pairs)
	if len(rows) != want {
		t.Fatalf("collected %d (season, arm) rows, want %d: a cell was refused or "+
			"skipped, and every figure below would describe a population smaller "+
			"than the one the sweep declared", len(rows), want)
	}

	writeUnplayedChipsCSV(t, rows)
	reportDeltas(t, rows, census)
}

// assertOneStartPoint says in the output that the entry grid is pinned, and why.
//
// The grid is a pre-registered part of the design and `runPolicySweep` is handed
// `[]int{1}` as a literal, so nothing an operator sets can move it. What this guards is
// the READER: FPL_SWEEP_STARTS is the switch every other diagnostic in this package
// honours, and a leftover value in a shell would otherwise make the output look like it
// had been obeyed.
//
// ⚠️ **The reason is narrower than "entry points are nested", and the first draft of this
// comment got it wrong.** Checked against `FullAnchoredPlan` and `ValidateChipSets` at all
// six standard starts, on all six seasons:
//
//	GW1, GW6, GW11 — first=4 second=4 everywhere. FULL granted allowance.
//	GW16           — 2025-26 places 3 of 4 first-set chips. Short.
//	GW21, GW26     — 2025-26 places 0 first-set chips. The first set is already expired.
//
// So the allowance argument rules out GW16 and later, and **does not rule out GW6 and
// GW11**. Those two are available at the full allowance and this measurement did not use
// them; the pre-registered grid is one entry point and a wider one is a different
// measurement needing its own pre-registration. Recorded here rather than left implied,
// because "there is no wider grid" is a much stronger claim than the design supports and
// it is the claim a reader will infer from a bare `[]int{1}`.
//
// ⚠️ It reads the environment string rather than calling `sweepStarts`, which **panics**
// on a one-element grid — "need at least two entry points". That refusal is right for a
// sweep whose variance comes from the grid and wrong here, where the six clusters are the
// six seasons and the entry point is held fixed on purpose.
func assertOneStartPoint(t *testing.T, seasons int) {
	t.Helper()
	if s := strings.TrimSpace(os.Getenv("FPL_SWEEP_STARTS")); s != "" {
		t.Logf("FPL_SWEEP_STARTS=%q is set and is IGNORED here: the pre-registered "+
			"design is one entry point. GW16 and later shorten a two-set season's "+
			"first set (GW21 and GW26 place none of it at all), so they are not "+
			"pooled with GW1; GW6 and GW11 do grant the full allowance and are "+
			"simply outside this measurement's design.", s)
	}
	// Derived from the grid that ran, never spelled. `sweepPairNames` returns six
	// pairs today and FPL_SWEEP_SEASONS moves that inside a run, so a literal count
	// here would describe a population the sweep did not have — which is what
	// TestPrintedGridLabelsAreDerived refuses, and it refused this file's first
	// draft.
	fmt.Printf("\nEntry grid: %s, GW1 only, 38 weeks — one cell per cluster, so the\n",
		gridLabel(seasons, 1))
	fmt.Printf("clustered SE is exactly the SE of the %d season means (df %d).\n",
		seasons, seasons-1)
	fmt.Printf("Every cell is 38 weeks, so per_gw and per_path differ by a constant and\n")
	fmt.Printf("give an identical t — which is why the per_gw default is safe HERE and\n")
	fmt.Printf("is not a general licence. Read the cells either way.\n")
}

// assertArmsDifferOnlyInChips is the prereg's first void condition, checked rather than
// asserted in prose.
//
// Both arms' `apply` hooks are empty, so the configs they hand to `Simulate` must be
// EQUAL — bank, oracles, start gameweek, weights, horizon, gate thresholds, the xGC
// source, everything. `reflect.DeepEqual` over the whole struct is what makes that a
// claim about the config rather than about the fields somebody remembered to list; it is
// the right instrument here precisely because every func field is nil on both sides, so
// a planner installed by either `apply` would fail it.
//
// The chip allowance is then installed by `runPolicySweep`'s `plan` path, which sets
// `Chips` and `Chips2` and nils `ChipPlanner`, and touches nothing else.
func assertArmsDifferOnlyInChips(t *testing.T, cfg config.Config, arms []policyVariant) {
	t.Helper()
	if len(arms) != 2 {
		t.Fatalf("this design has exactly two arms; got %d", len(arms))
	}
	// Each arm's own apply hook, run on the base the sweep will run it on. Neither
	// installs anything, and this asserts that rather than trusting it — an edit
	// that slips a setting into one of them would otherwise be indistinguishable
	// from a chip effect.
	applied := make([]SimConfig, len(arms))
	for i, a := range arms {
		applied[i] = sweepConfig(cfg, 1, false)
		a.apply(&applied[i])
	}
	if !reflect.DeepEqual(applied[0], applied[1]) {
		t.Fatalf("the two arms hand Simulate different configs before any chip is "+
			"installed:\nA = %+v\nB = %+v\n"+
			"The prereg voids on any difference other than the chip allowance",
			applied[0], applied[1])
	}
	// reflect.DeepEqual over a struct carrying func fields is true only when each
	// pair is nil, which is the property wanted here: an apply-installed planner on
	// either side fails it. What it cannot see is a planner installed through
	// `plan`, which is the one difference this design intends, so that is counted
	// separately.
	planned := 0
	for _, a := range arms {
		if a.plan != nil {
			planned++
		}
	}
	if planned != 1 {
		t.Fatalf("%d of %d arms install a chip schedule; exactly one must — a "+
			"second would make the contrast chips-against-chips and a zeroth "+
			"would make it chips-against-itself", planned, len(arms))
	}
	base := applied[0]
	if base.ChipSets != 0 {
		t.Fatalf("ChipSets = %d on the sweep base: a nonzero value replays every "+
			"season under a declared allowance instead of the one it granted, "+
			"which is the ChipSetsForced counterfactual the prereg voids on",
			base.ChipSets)
	}
	if base.Chips != (analysis.ChipPlan{}) || base.Chips2 != (analysis.ChipPlan{}) ||
		base.ChipPlanner != nil || base.ChipScheduleP != nil || base.ChipPlannerXP != nil {
		t.Fatalf("the sweep base already carries a chip plan (%v/%v, planner=%v, "+
			"schedule=%v, xp=%v); arm A would not be the no-chip machine the banked "+
			"baseline was measured on", base.Chips, base.Chips2,
			base.ChipPlanner != nil, base.ChipScheduleP != nil, base.ChipPlannerXP != nil)
	}
	if (base.Oracles != Oracles{}) {
		t.Fatalf("the sweep base carries hindsight (%s); the prereg runs oracles off",
			base.Oracles.Stamp())
	}
	// And the things a reader would want named, read off the config rather than
	// recited: DeepEqual says the arms agree, and these say what they agree ON — so
	// a base that changed underneath both arms is still visible in the output.
	fmt.Printf("\n--- the arms differ ONLY in the chip allowance (checked, not asserted)\n")
	fmt.Printf("Both arms' apply hooks are empty and the configs they produce are\n")
	fmt.Printf("reflect.DeepEqual over the whole SimConfig — every field, including the\n")
	fmt.Printf("nil func fields, so a planner installed by either apply would fail it.\n")
	fmt.Printf("They agree on: bank_up_to=%d start_gw=%d weekly_xi=%v horizon=%d\n",
		base.BankUpTo, base.startGW(), base.WeeklyXI, base.Weights.Horizon)
	fmt.Printf("               oracles=%s chip_sets=%d (0 = ask the season)\n",
		base.Oracles.Stamp(), base.ChipSets)
	fmt.Printf("               min_gain=%.2f min_gain_hit=%.2f max_hits=%d budget=%d\n",
		base.MinGain, base.MinGainHit, base.MaxHits, base.Budget)
	fmt.Printf("               xgc source: %s\n", xgcSourceLabel())
	fmt.Printf("Exactly one arm sets `plan`. runPolicySweep's plan path then sets Chips\n")
	fmt.Printf("and Chips2 and nils ChipPlanner, and touches nothing else.\n")
}

// xgcSourceLabel names the xGC input both arms read, so the one process-global that
// could differ between two runs of this file is in the output rather than in an
// operator's shell. Both arms share it by construction — loadConfig sets it once, at
// process level — so it cannot differ BETWEEN the arms; it can differ between RUNS, and
// that is what this prints.
func xgcSourceLabel() string {
	d := externalXGCDir()
	if d == "" {
		return "FPL's own aggregates (no external directory)"
	}
	return d
}

// printCensus writes the per-season doubling and blanking calendar.
func printCensus(pairs []seasonPair, census map[string]doublingCensus) {
	fmt.Printf("\n--- the doubling census (the prereg's void condition, counted)\n")
	fmt.Printf("Doubling CLUB-gameweeks, from the fixture list alone. Split at the\n")
	fmt.Printf("chip reset (GW%d), because that is where the sets split.\n\n", ChipResetGW)
	fmt.Printf("%-9s %10s %10s %8s   %s\n",
		"season", "GW1-19", "GW20-38", "total", "doubling weeks (clubs)")
	firstAll, secondAll := 0, 0
	for _, p := range pairs {
		d := census[p.Name]
		firstAll += d.firstHalf
		secondAll += d.secondHalf
		var weeks []int
		for gw := range d.doubleWeeks {
			weeks = append(weeks, gw)
		}
		sort.Ints(weeks)
		s := ""
		for _, gw := range weeks {
			s += fmt.Sprintf(" GW%d(%d)", gw, d.doubleWeeks[gw])
		}
		if s == "" {
			s = " none"
		}
		fmt.Printf("%-9s %10d %10d %8d  %s\n",
			p.Name, d.firstHalf, d.secondHalf, d.total(), s)
	}
	fmt.Printf("%-9s %10d %10d %8d\n", "ALL", firstAll, secondAll, firstAll+secondAll)

	// The other half of the void condition. The free hit is the blank chip and
	// FullAnchoredPlan anchors it on the window's biggest blank, so a season with no
	// blank to aim at constrains it exactly as a season with no double constrains
	// the bench boost.
	fmt.Printf("\nBlanking club-gameweeks — clubs idle in a round the league otherwise\n")
	fmt.Printf("played. Counted only in played rounds, so an absent gameweek is not read\n")
	fmt.Printf("as a twenty-club blank. This is the FREE HIT's supply.\n\n")
	fmt.Printf("%-9s %10s %10s %8s   %s\n",
		"season", "GW1-19", "GW20-38", "total", "blank weeks (clubs)")
	fbAll, sbAll := 0, 0
	for _, p := range pairs {
		d := census[p.Name]
		fbAll += d.firstBlank
		sbAll += d.secondBlank
		var weeks []int
		for gw := range d.blankWeeks {
			weeks = append(weeks, gw)
		}
		sort.Ints(weeks)
		s := ""
		for _, gw := range weeks {
			s += fmt.Sprintf(" GW%d(%d)", gw, d.blankWeeks[gw])
		}
		if s == "" {
			s = " none"
		}
		fmt.Printf("%-9s %10d %10d %8d  %s\n",
			p.Name, d.firstBlank, d.secondBlank, d.blankTotal(), s)
	}
	fmt.Printf("%-9s %10d %10d %8d\n", "ALL", fbAll, sbAll, fbAll+sbAll)

	fmt.Printf("\nchipsets.go records 15 first-half doubling club-gameweeks of 189 across\n")
	fmt.Printf("six archived first halves, 11 of them one COVID-rescheduled 2020-21 round.\n")
	fmt.Printf("Read every per-season delta against this table: a chip is worth something\n")
	fmt.Printf("different in a season with nothing to spend it on.\n")
}

// assertGrantedAllowance pins that arm B is the granted allowance and not the forced one.
//
// The failure it exists for is silent by construction: `ChipSetsForced` and
// `ChipSetsFor` are both integers, both plausible, and a season replayed under the wrong
// one produces a complete-looking table with eight chips where four were available. So
// the shape is checked directly on the plan — five seasons must come back with an EMPTY
// second set, and 2025-26 with a full one — rather than by reading which constant the
// code names.
func assertGrantedAllowance(t *testing.T, pairs []seasonPair) {
	t.Helper()
	fmt.Printf("\n--- arm B's allowance, read off the plan rather than off the constant\n")
	for _, p := range pairs {
		sets := ChipSetsFor(p.Name)
		sch := FullAnchoredPlan(p.Cur, 1)
		first, second := PlacedChips(sch.First), PlacedChips(sch.Second)
		fmt.Printf("%-9s ChipSetsFor=%d  placed first=%d second=%d\n",
			p.Name, sets, first, second)
		if sets == 1 && second != 0 {
			t.Fatalf("%s granted one set but the plan places %d chips in a second: "+
				"this is the ChipSetsForced counterfactual, which replays a season "+
				"under rules nobody played under and destroys the contrast this "+
				"measurement is about", p.Name, second)
		}
		if sets == 2 && second == 0 {
			t.Fatalf("%s granted two sets but the plan places none in the second: "+
				"arm B is measuring four chips where the season allowed eight, "+
				"which is the whole hypothesis unmeasured", p.Name)
		}
		if first != 4 {
			t.Fatalf("%s: the first set places %d chips, not 4 — a chip the plan "+
				"never places is a chip the arm never plays, and the delta would "+
				"understate the allowance without saying so", p.Name, first)
		}
		if err := ValidateChipSets(p.Name, sch.First, sch.Second); err != nil {
			t.Fatalf("%s: arm B's plan is refused by the season's own rules (%v); "+
				"Simulate would record the cell INFEASIBLE and the comparison "+
				"would quietly run on fewer seasons than it declares", p.Name, err)
		}
	}
}

// writeUnplayedChipsCSV emits the summary the prereg asks for: one row per
// (season, arm), carrying the points, the chips actually played and the doubling census
// the delta has to be read against.
//
// It sits beside the ordinary cells file rather than replacing it. The cells file is what
// `stats/sweep_inference.R` reads and is where the inference happens; this is the
// human-readable join of the two things a reader needs in one place, and nothing computes
// a verdict from it.
func writeUnplayedChipsCSV(t *testing.T, rows []armCell) {
	t.Helper()
	dir := unplayedChipsRunDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := [][]string{{
		"season", "arm", "points", "weeks", "per_gw", "chips_played",
		"chips_placed_first_set", "chips_placed_second_set",
		"doubling_cgw_total", "doubling_cgw_gw1_19", "doubling_cgw_gw20_38",
		"blanking_cgw_total", "blanking_cgw_gw1_19", "blanking_cgw_gw20_38",
		"chip_weeks", "chips_on_a_double", "chips_on_a_blank",
		"free_hits_on_a_blank", "week_local_gain_bb_tc",
	}}
	for _, r := range rows {
		onDouble, onBlank, fhOnBlank, localGain := 0, 0, 0, 0
		played := ""
		for _, p := range r.plays {
			if p.doubling > 0 {
				onDouble++
			}
			if p.blanking > 0 {
				onBlank++
				if p.name == "free_hit" {
					fhOnBlank++
				}
			}
			if p.hasGain {
				localGain += p.gain
			}
			if played != "" {
				played += " "
			}
			if p.hasGain {
				played += fmt.Sprintf("%s@GW%d(%dx,%db,+%d)",
					p.name, p.gw, p.doubling, p.blanking, p.gain)
			} else {
				played += fmt.Sprintf("%s@GW%d(%dx,%db)",
					p.name, p.gw, p.doubling, p.blanking)
			}
		}
		out = append(out, []string{
			r.season, r.arm,
			strconv.Itoa(r.points), strconv.Itoa(r.weeks),
			strconv.FormatFloat(float64(r.points)/float64(r.weeks), 'f', 4, 64),
			strconv.Itoa(r.chipsPlayed()),
			strconv.Itoa(r.firstN), strconv.Itoa(r.secondN),
			strconv.Itoa(r.census.total()),
			strconv.Itoa(r.census.firstHalf), strconv.Itoa(r.census.secondHalf),
			strconv.Itoa(r.census.blankTotal()),
			strconv.Itoa(r.census.firstBlank), strconv.Itoa(r.census.secondBlank),
			played, strconv.Itoa(onDouble), strconv.Itoa(onBlank),
			strconv.Itoa(fhOnBlank), strconv.Itoa(localGain),
		})
	}
	path := filepath.Join(dir, "seasons.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.WriteAll(out); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	fmt.Printf("\nwrote %s (%d rows)\n", path, len(out)-1)
}

// reportDeltas prints the paired per-season table and the shape questions.
//
// ⚠️ **No SE, no t and no verdict here, by design.** The clustered standard error this
// prereg's decision rule needs is computed in `stats/sweep_inference.R` from the cells
// file, and re-deriving it in Go would be a second implementation of a quantity this
// package already had drift on once — `reportPairedDifferences` says exactly this, and
// this function obeys the same rule. What is printed here is the paired differences
// themselves, which nothing else joins to the census.
func reportDeltas(t *testing.T, rows []armCell, census map[string]doublingCensus) {
	t.Helper()
	byKey := map[string]armCell{}
	seen := map[string]bool{}
	var seasons []string
	for _, r := range rows {
		byKey[r.season+"|"+r.arm] = r
		if !seen[r.season] {
			seen[r.season] = true
			seasons = append(seasons, r.season)
		}
	}
	sort.Strings(seasons)
	armAName, armBName := rows[0].arm, ""
	for _, r := range rows {
		if r.arm != armAName {
			armBName = r.arm
			break
		}
	}

	fmt.Printf("\n--- paired per-season delta, chips minus no chips (POLICY)\n")
	fmt.Printf("%-9s %8s %8s %8s %9s %7s %8s   %s\n",
		"season", "A", "B", "delta", "per_gw", "chips", "dbl_cgw", "chip weeks")
	for _, s := range seasons {
		a, okA := byKey[s+"|"+armAName]
		b, okB := byKey[s+"|"+armBName]
		if !okA || !okB {
			t.Fatalf("%s is missing an arm (A=%v B=%v); the completeness check "+
				"above should have caught this", s, okA, okB)
		}
		d := b.points - a.points
		weeks := ""
		for _, p := range b.plays {
			if weeks != "" {
				weeks += " "
			}
			if p.hasGain {
				weeks += fmt.Sprintf("%s@%d/%dx/%db/+%d",
					shortChip(p.name), p.gw, p.doubling, p.blanking, p.gain)
			} else {
				weeks += fmt.Sprintf("%s@%d/%dx/%db",
					shortChip(p.name), p.gw, p.doubling, p.blanking)
			}
		}
		fmt.Printf("%-9s %8d %8d %8d %9.3f %7d %8d   %s\n",
			s, a.points, b.points, d,
			float64(d)/float64(a.weeks), b.chipsPlayed(),
			census[s].total(), weeks)
	}

	fmt.Printf("\n--- SHAPE, not a result: which chip, and where it landed\n")
	fmt.Printf("⚠️ Not decision-bearing. The figures below are the WEEK-LOCAL gains the\n")
	fmt.Printf("simulator already records — the bench's points on a boost week, the\n")
	fmt.Printf("captain's extra copy on a triple week. They do NOT sum to the season\n")
	fmt.Printf("delta and are not meant to: a chip also changes the squad and the\n")
	fmt.Printf("transfers around it, and the wildcard and the free hit have no\n")
	fmt.Printf("week-local gain AT ALL, because neither has an effect confined to its\n")
	fmt.Printf("own week. Read these as attribution of direction, never of size.\n\n")
	fmt.Printf("%-9s %14s %14s %10s %12s   %-18s %s\n",
		"season", "bench boost", "triple capt", "sum BB+TC", "season delta",
		"chips on a double", "free hits on a blank")
	var bbAll, tcAll, dAll int
	for _, s := range seasons {
		a, b := byKey[s+"|"+armAName], byKey[s+"|"+armBName]
		bb, tc, onDouble, fh, fhOnBlank := 0, 0, 0, 0, 0
		for _, p := range b.plays {
			switch p.name {
			case "bench_boost":
				bb += p.gain
			case "triple_captain":
				tc += p.gain
			case "free_hit":
				fh++
				if p.blanking > 0 {
					fhOnBlank++
				}
			}
			if p.doubling > 0 {
				onDouble++
			}
		}
		d := b.points - a.points
		bbAll, tcAll, dAll = bbAll+bb, tcAll+tc, dAll+d
		fmt.Printf("%-9s %14d %14d %10d %12d   %-18s %d of %d\n",
			s, bb, tc, bb+tc, d,
			fmt.Sprintf("%d of %d", onDouble, b.chipsPlayed()), fhOnBlank, fh)
	}
	fmt.Printf("%-9s %14d %14d %10d %12d\n", "ALL", bbAll, tcAll, bbAll+tcAll, dAll)
	fmt.Printf("\nThe gap between (BB+TC) and the season delta is everything the two\n")
	fmt.Printf("localisable chips cannot account for: the wildcard, the free hit, and\n")
	fmt.Printf("the whole transfer path a chip week reshapes around itself. It is a\n")
	fmt.Printf("residual, not a fifth chip.\n")

	// The two-set contrast, printed as shape and labelled as such in the same
	// breath. It is the owner's hypothesis and it is the one thing here most likely
	// to be quoted out of context, so the caveat is not left to the reader.
	fmt.Printf("\n--- SHAPE: the two-set season against the five one-set seasons\n")
	var one []float64
	var two float64
	for _, s := range seasons {
		a, b := byKey[s+"|"+armAName], byKey[s+"|"+armBName]
		v := float64(b.points-a.points) / float64(a.weeks)
		if ChipSetsFor(s) >= 2 {
			two = v
		} else {
			one = append(one, v)
		}
	}
	var sum float64
	lo, hi := one[0], one[0]
	for _, v := range one {
		sum += v
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	// Mean and range, and deliberately no standard deviation and no z-score.
	// Five seasons is too few for a spread to be worth a number, and a "the
	// two-set season sits N sd above" line is precisely the over-reading the
	// prereg pre-declares this contrast cannot support. The range says the same
	// thing without inviting it.
	fmt.Printf("one-set seasons (n=%d): mean %+.3f per gw, range %+.3f..%+.3f\n",
		len(one), sum/float64(len(one)), lo, hi)
	fmt.Printf("two-set season   (n=1): %+.3f per gw\n", two)
	fmt.Printf("\n⚠️ ONE season against five. It cannot resolve at df 5 as its own\n")
	fmt.Printf("contrast and the prereg pre-declares it as SHAPE. The thing to read it\n")
	fmt.Printf("against is the SPREAD across the one-set seasons above: if the best\n")
	fmt.Printf("one-set season reaches what the two-set season reached, the allowance\n")
	fmt.Printf("is not what separates them. Confirming this needs 2026-27, the second\n")
	fmt.Printf("two-set season.\n")
}

func shortChip(n string) string {
	switch n {
	case "bench_boost":
		return "BB"
	case "triple_captain":
		return "TC"
	case "free_hit":
		return "FH"
	case "wildcard":
		return "WC"
	}
	return n
}
