package backtest

// Wiring for ValidateChipSpend (chipsets.go) into the sweep harness.
//
// # Why this is scoped to policyVariant.plan, and nothing wider
//
// A chip can reach a cell through three different mechanisms: a literal
// `SimConfig.Chips`/`Chips2` an `apply` hook assigns directly, a `ChipPlanner`
// that resolves one chip at a time inside Simulate, or `policyVariant.plan` —
// the field whose own doc comment says it installs "a full two-set chip
// schedule ... the shipped user-facing planner". Only the third makes any
// claim to spending a full grant. This package's diagnostics use the other
// two constantly and on purpose for narrow, single-lever measurements that
// were never meant to place all four chips — `anchored_diag_test.go`'s own
// `laggedPlan` never places a wildcard at all, and `tworegime_diag_test.go`
// runs an arm labelled "FIRST SET WASTED" in so many words. Making every
// literal or `ChipPlanner`-installed arm justify a full spend would refuse a
// large body of already-measured, already-cited research for behaving
// exactly as designed.
//
// `policyVariant.plan` carries no such population: it has exactly two callers
// in this package (`hittuning_diag_test.go`, `unplayedchips_diag_test.go`),
// both `FullAnchoredPlan` — the same full-grant machine `cmd/armband/backtest.go`
// runs live. Guarding it costs nothing today and catches exactly the bug class
// this file exists for: a full-plan mechanism silently discarding a set, which
// is what actually happened once, in `anchoredPlan`'s first version, and cost a
// diagnostic roughly a season's worth of chips before anyone noticed (see
// SplitChipSets's doc comment in chipsets.go). A future `.plan` arm that is
// deliberately partial can still declare `ChipLapse`, exactly as
// `tworegime_diag_test.go` would need to if it used this field instead of
// `ChipPlanner`.

import (
	"fmt"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// lapseOf reads back the lapse an arm's apply hook actually installs, the same
// way oraclesOf reads back its oracle — a probe on a throwaway config, never
// the arm's own literal field, so a hand-written arm cannot claim a
// declaration it does not install.
func lapseOf(cfg config.Config, start int, v policyVariant) ChipSetLapse {
	sc := sweepConfig(cfg, start, false)
	v.apply(&sc)
	return sc.ChipLapse
}

// chipPlansFromRow reconstructs the two sets' worth of ACTUALLY PLAYED chips
// from a cellRow's chip-week columns, bucketing each recorded play into the
// set whose window it falls inside — GW < ChipResetGW is the first set,
// GW >= ChipResetGW the second, the same boundary splitChipSets routes on.
//
// Built from what the row recorded rather than from the config's planned
// schedule, because a plan is only ever an intention: a chip placed in a
// legal week can still fail to be fielded (see the free-hit eligibility
// notes on Week.FreeHit), and the row's own columns are what Simulate
// actually did, populated straight off res.Weeks by populateChipWeekColumns.
//
// ⚠️ `sets` matters and must be splitChipSets's own short-circuit: on a
// ONE-SET season every play belongs in `first` regardless of its week
// number — there is no second set to route a late play into. Bucketing
// blindly on the GW20 threshold instead misread every one-set season whose
// chips landed after GW20 (which is most of them; GW20-38 is most of the
// calendar) as a mostly-empty first set plus an ungranted, and therefore
// silently skipped, second one — the exact false positive this guard exists
// to never produce. Caught by TestChipPlansFromRowRoutesAOneSetSeasonEntirely
// ToTheFirstSet after it fired on a real sweep (2023-24 read 1/4 first-set
// chips spent with the other three misrouted into a set the season never
// granted).
func chipPlansFromRow(row cellRow, sets int) (first, second analysis.ChipPlan) {
	assign := func(gw int, set func(*analysis.ChipPlan, int)) {
		if gw <= 0 {
			return
		}
		if sets < 2 || gw < ChipResetGW {
			set(&first, gw)
		} else {
			set(&second, gw)
		}
	}
	assign(row.BenchBoostGW, func(p *analysis.ChipPlan, gw int) { p.BenchBoost = gw })
	assign(row.BenchBoostGW2, func(p *analysis.ChipPlan, gw int) { p.BenchBoost = gw })
	assign(row.TripleCapGW, func(p *analysis.ChipPlan, gw int) { p.TripleCaptain = gw })
	assign(row.TripleCapGW2, func(p *analysis.ChipPlan, gw int) { p.TripleCaptain = gw })
	assign(row.FreeHitGW, func(p *analysis.ChipPlan, gw int) { p.FreeHit = gw })
	assign(row.FreeHitGW2, func(p *analysis.ChipPlan, gw int) { p.FreeHit = gw })
	assign(row.WildcardGW, func(p *analysis.ChipPlan, gw int) { p.Wildcard = gw })
	assign(row.WildcardGW2, func(p *analysis.ChipPlan, gw int) { p.Wildcard = gw })
	return first, second
}

// TestChipPlansFromRowRoutesAOneSetSeasonEntirelyToTheFirstSet is the
// regression this bug earned: chipPlansFromRow used to bucket purely on the
// GW20 threshold, so a one-set season whose chips (correctly) landed after
// GW20 — which is most of the calendar, and includes FullAnchoredPlan's own
// fallback weeks — read as a mostly-empty first set plus a handful of chips
// misrouted into a second set the season never granted, and the guard read
// that second bucket as "not owed" and stayed silent about it while flagging
// the first as short. Found on a real sweep: 2023-24 read 1/4 first-set
// chips spent with the other three silently misclassified away.
func TestChipPlansFromRowRoutesAOneSetSeasonEntirelyToTheFirstSet(t *testing.T) {
	row := cellRow{
		WildcardGW: 4, FreeHitGW: 29, BenchBoostGW: 34, TripleCapGW: 37,
	}
	first, second := chipPlansFromRow(row, 1)
	if got := PlacedChips(first); got != 4 {
		t.Errorf("a one-set season's chips placed %d/4 in the first set, want 4: %+v",
			got, first)
	}
	if got := PlacedChips(second); got != 0 {
		t.Errorf("a one-set season routed %d chips into a second set it never "+
			"granted: %+v", got, second)
	}
	// The two-set case still routes on the GW20 boundary, which is the whole
	// point of the guard: a two-set season that plays everything before GW20
	// really has left its second set unspent, and that must still be visible.
	twoSet := cellRow{
		WildcardGW: 6, BenchBoostGW: 8, TripleCapGW: 9, FreeHitGW: 16,
		WildcardGW2: 28, BenchBoostGW2: 33, TripleCapGW2: 35, FreeHitGW2: 31,
	}
	first2, second2 := chipPlansFromRow(twoSet, 2)
	if got := PlacedChips(first2); got != 4 {
		t.Errorf("a two-set season's first-half chips placed %d/4: %+v", got, first2)
	}
	if got := PlacedChips(second2); got != 4 {
		t.Errorf("a two-set season's second-half chips placed %d/4: %+v", got, second2)
	}
}

// validateChipSpendArms resolves every `.plan`-carrying arm's chip schedule
// across the whole grid BEFORE the first cell runs, and refuses a plan that
// would silently discard a granted, reachable set — see the file doc comment
// for why this is scoped to `.plan` and not to every chip-installing arm.
//
// It runs each arm's `apply` on a probe config the same way `oraclesOf` does,
// so `sc.ChipSets` reflects whatever that arm actually declares rather than
// the season's default — an arm replaying under ChipSetsForced must be
// checked against the count it declares, not the one history granted.
func validateChipSpendArms(cfg config.Config, variants []policyVariant, pairs []seasonPair, starts []int) error {
	for _, v := range variants {
		if v.plan == nil {
			continue
		}
		for _, start := range starts {
			for _, pair := range pairs {
				sc := sweepConfig(cfg, start, false)
				v.apply(&sc)
				sch := v.plan(pair.Cur, start)
				if err := ValidateChipSpendWith(pair.Cur.Name, sc.ChipSets, start,
					v.lapse, sch.First, sch.Second); err != nil {
					return fmt.Errorf("variant %q at %s@%d: %w", v.label, pair.Name, start, err)
				}
			}
		}
	}
	return nil
}
