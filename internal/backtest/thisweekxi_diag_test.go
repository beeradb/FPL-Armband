package backtest

// TestDiagThisWeekXIOnHold — does picking the held fifteen's XI and captain on
// this week's Score harvest the per-match fixture effect?
//
// Pre-registered (vault, committed before this file existed as a points run):
// memory/2026-09-08-prereg-does-this-week-xi-on-hold-harvest-the-per-match-fixture-effect.md
//
//	DIAG=1 EXP=FIXH FPL_SWEEP_SEASONS=extended FPL_CELLS=<path> \
//	  scripts/replay -run TestDiagThisWeekXIOnHold -v -timeout 2h
//
// HOLD only. Does not flip WeeklyXI and call Hold() — that path never reads the
// flag. A0 is shipped HoldCaptaincyWeekly. A1 isolates this-GW FDR (skip other
// gameweeks, load off, Score=0 on blanks, both double legs averaged). A2 is
// horizon 1 with load on (WeekEngine / WeeklyXI fielding).
//
// Go prints no t, SE, or verdict word. Inference is stats/sweep_inference.R.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagThisWeekXIOnHold(t *testing.T) {
	requireDiag(t)
	picker := newBlockPicker()
	defer picker.check(t)
	if !picker.want("FIXH") {
		return
	}

	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)
	starts := sweepStarts()

	sink, err := openCellSink(os.Getenv("FPL_CELLS"))
	if err != nil {
		t.Fatal(err)
	}
	defer sink.close()
	sweep := sink.sweepLabel("FIXH")

	type armDef struct {
		label string
		field HoldFielding
		ship  bool // A0: call HoldCaptaincyWeekly, not the override
	}
	arms := []armDef{
		{label: "A0_horizon5_shipped", ship: true},
		// Load is already out of Score at shipped horizon 5
		// (FixtureLoadInScore is false unless Horizon==1). DisableLoad would
		// be a no-op here; Isolate + ZeroBlank are the live treatments.
		{label: "A1_thisgw_difficulty", field: HoldFielding{
			Isolate: true, ZeroBlank: true,
		}},
		{label: "A2_horizon1_load", field: HoldFielding{Horizon: 1}},
	}
	variants := make([]policyVariant, len(arms))
	for i, a := range arms {
		a := a
		variants[i] = policyVariant{label: a.label, apply: func(*SimConfig) {}}
	}
	writeSweepProvenance(t, sweep, sink, cfg, variants, pairs, starts)

	fmt.Printf("\nTHIS-WEEK XI ON HOLD — %s, chips off, no transfers\n",
		gridLabel(len(pairs), len(starts)))
	fmt.Printf("A0 = shipped HoldCaptaincyWeekly (horizon %d). A1 = this-GW FDR, load off, blanks zeroed.\n",
		cfg.Weights.Horizon)
	fmt.Printf("A2 = horizon 1, load on. Mediator: XI-diff weeks, captain-diff weeks, blank-club XI player-GWs.\n\n")

	type med struct{ xi, cap, blank, weeks int }
	medians := map[string]med{}

	for _, pr := range pairs {
		for _, start := range starts {
			sc := sweepConfig(cfg, start, false)
			held, err := openingHeld(pr.Cur, pr.Prior, sc)
			if err != nil {
				t.Fatalf("%s@%d opening: %v", pr.Name, start, err)
			}
			ship := HoldCaptaincyWeekly(pr.Cur, pr.Prior, sc, held)

			for vi, a := range arms {
				var hc HoldCaptaincy
				if a.ship {
					hc = ship
				} else {
					hc = HoldCaptaincyWithFielding(pr.Cur, pr.Prior, sc, held, a.field)
				}
				if a.ship {
					// Copy-check: the A0 path is shipped HOLD, not a second loop.
					alt := HoldCaptaincyWithFielding(pr.Cur, pr.Prior, sc, held, HoldFielding{})
					if sumInts(alt.Full) != sumInts(ship.Full) {
						t.Fatalf("%s@%d A0 WithFielding(zero)=%d shipped=%d — the override default drifted",
							pr.Name, start, sumInts(alt.Full), sumInts(ship.Full))
					}
				}
				xiDiff, capDiff, blank := fieldingMediator(pr.Cur, ship, hc)
				m := medians[a.label]
				m.xi += xiDiff
				m.cap += capDiff
				m.blank += blank
				m.weeks += len(hc.Full)
				medians[a.label] = m

				weeks := len(hc.Full)
				hold := sumInts(hc.Full)
				row := cellRow{
					Sweep:        sweep,
					RunID:        sink.run(),
					Variant:      a.label,
					VariantIndex: vi,
					IsBaseline:   vi == 0,
					Season:       pr.Name,
					PriorSeason:  pr.PriorName,
					StartGW:      start,
					Weeks:        weeks,
					BankUpTo:     sc.BankUpTo,
					PolicyPoints: hold, // unused; HOLD is the metric. Required column.
					HoldPoints:   hold,
					Moves:        0,
					Hits:         0,
				}.under(sc.Oracles)
				sink.cell(row)
				fmt.Printf("%s@%d %-22s hold=%5d weeks=%2d xi_diff=%2d cap_diff=%2d blank_xi=%2d\n",
					pr.Name, start, a.label, hold, weeks, xiDiff, capDiff, blank)
			}
		}
	}

	fmt.Printf("\nMEDIATOR TOTALS (void an arm with xi_diff=0 AND cap_diff=0)\n")
	void := false
	for _, a := range arms {
		m := medians[a.label]
		fmt.Printf("  %-22s xi_diff_weeks=%d cap_diff_weeks=%d blank_xi_pgw=%d over %d scored weeks\n",
			a.label, m.xi, m.cap, m.blank, m.weeks)
		if a.ship {
			continue
		}
		if m.xi == 0 && m.cap == 0 {
			fmt.Printf("  VOID %s: comparison never ran\n", a.label)
			void = true
		}
	}
	if void {
		t.Fatal("an arm differed in 0 XI weeks and 0 captain weeks — void, not null; do not interpret points")
	}
	fmt.Printf("\nRe-run inference:\n  Rscript stats/sweep_inference.R %s\n", os.Getenv("FPL_CELLS"))
}

func openingHeld(cur, prior *Season, sc SimConfig) ([]int, error) {
	e, _ := EngineAt(cur, prior, sc.startGW()-1, sc)
	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: sc.Budget, MinMinutes: 600,
		MinExpectedMinutes: sc.resolvedMinExpectedMinutes(),
		BenchWeight:        sc.openingBenchWeight(),
	})
	if err != nil {
		return nil, err
	}
	held := make([]int, 0, 15)
	for _, p := range sq.Players {
		held = append(held, p.ID)
	}
	if len(held) != 15 {
		return nil, fmt.Errorf("opening fifteen has %d players", len(held))
	}
	return held, nil
}

func fieldingMediator(cur *Season, base, arm HoldCaptaincy) (xiDiff, capDiff, blankXI int) {
	n := len(base.GW)
	if len(arm.GW) < n {
		n = len(arm.GW)
	}
	for i := 0; i < n; i++ {
		if !sameIDSet(base.XI[i], arm.XI[i]) {
			xiDiff++
		}
		if base.Captain[i] != arm.Captain[i] {
			capDiff++
		}
		gw := arm.GW[i]
		for _, id := range arm.XI[i] {
			p := cur.Players[id]
			if p == nil {
				continue
			}
			if !clubPlaysGW(cur, p.Team, gw) {
				blankXI++
			}
		}
	}
	return
}

func sameIDSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]int(nil), a...)
	bs := append([]int(nil), b...)
	sort.Ints(as)
	sort.Ints(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}
