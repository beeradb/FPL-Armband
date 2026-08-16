package backtest

import (
	"os"
	"testing"
)

// TestDiagXGCPoints sizes what the expected-goals-conceded reconstruction does to a
// replayed season, per cell.
//
//	DIAG=1 EXP=1 go test ./internal/backtest -run TestDiagXGCPoints -v -timeout 120m
//
// # Why this is per cell and carries no verdict
//
// The reconstruction is structurally confined: it fills a hole that exists in
// 2018-19, 2019-20, 2020-21, 2021-22 and the first fifteen gameweeks of 2022-23, and
// nowhere else. So the pairs whose *played* season already carries real xGC must come
// out **byte-identical**, and that invariance is a sharper check than any of the moved
// figures — it is the only assertion here.
//
// The moved cells are reported and **not** pooled into a verdict. One entry point on
// one cell is the noisiest way this harness can be read; the record's own detection
// threshold is a median of 39 points a season, and `Optimize` is separately not
// run-to-run deterministic, so a cell can move without the repair having moved it.
// Turning these rows into a number is a sweep — the full six-season grid on
// `FPL_NO_XGC_REPAIR` — and that is recorded in the work queue rather than done here.
//
// # What the sign should be, stated in advance
//
// Pre-registering this because the temptation afterwards is to explain whichever way
// it went. With the repair off, `baseXP90` gates both the clean sheet and the
// goals-conceded deduction on `XGC90 > 0`, so **every defender and keeper in those
// seasons is scored with neither term**. The clean sheet is the larger of the two and
// it is worth 26-45% of their score, so the repair should make defenders and keepers
// substantially more attractive relative to attackers, and the opening fifteen should
// change in most affected cells. Whether that is worth *points* is a different
// question and this cannot answer it: a better-specified objective making a worse
// policy is the single most repeated finding in this project's record.
//
// ⚠️ **Half of that prediction was unfalsifiable and the mediator is what showed it.**
// The DEF+GKP *count* reads **7 in every cell of both arms**, and it could not have
// read anything else: FPL forces two goalkeepers and five defenders in a fifteen, so
// the quota is a constraint rather than a choice and "the model buys more defenders"
// was never a thing that could happen. The count column is kept precisely because a
// structurally inert mediator looks identical to a null, and this file's rule is that
// a season producing identical output under an intervention is not a tie.
//
// The composition claim therefore has to be about **money**, which is free to move:
// which defenders and which keeper, not how many. That column does move — and it is
// the same class of error as "check what a multiplier multiplies before calibrating
// it", arriving in a mediator instead of a constant.
func TestDiagXGCPoints(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	// Both arms parsed under this test's control. loadSeason's process-global cache
	// would hand back whichever arm won the race, which is the hazard that makes a
	// paired comparison silently measure one arm twice.
	load := func(name string, repair bool) *Season {
		if repair {
			t.Setenv("FPL_NO_XGC_REPAIR", "")
		} else {
			t.Setenv("FPL_NO_XGC_REPAIR", "1")
		}
		s, err := Load(t.Context(), cfg.CacheDir, name)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	pairs := scoringPairNames()

	// The mechanism half is cheap and it is what establishes the defect: how many of
	// the played season's appearances carry any expected goals conceded at all.
	t.Log("prior      season     appearances with xGC, repair off    on")
	affected := map[string]bool{}
	for _, p := range pairs {
		var off, on int
		for _, repair := range []bool{false, true} {
			s := load(p[1], repair)
			n := 0
			for _, id := range sortedSeasonPlayerIDs(s) {
				for _, g := range s.Players[id].GWs {
					if g.Minutes > 0 && g.XGC > 0 {
						n++
					}
				}
			}
			if repair {
				on = n
			} else {
				off = n
			}
		}
		t.Logf("%-10s %-10s %31d    %d", p[0], p[1], off, on)
		if on != off {
			affected[p[1]] = true
		}
	}
	if len(affected) == 0 {
		t.Fatal("no played season's xGC coverage moved, so this measured nothing — " +
			"either the repair is not reaching Load or the seasons list is wrong")
	}

	if os.Getenv("EXP") == "" {
		t.Log("set EXP=1 for the paired points comparison (two replays per cell, slow)")
		return
	}

	// The MEDIATOR, and it is not decoration.
	//
	// The sign pre-registered above is about **composition** — the repair restores
	// the clean sheet and the goals-conceded deduction, so defenders and keepers
	// should become relatively more attractive and the opening fifteen should shift
	// money toward them. The points columns cannot test that. By this project's own
	// standard a mechanism claim without its mediator is an assertion: the horizon
	// ladder is judged on transfer count, the perfect gate on moves and hits, and
	// this belongs in the same category.
	//
	// It is also the half that is NOT swamped by path noise. A points difference on
	// one cell competes with the optimiser's own nondeterminism; "how much of the
	// budget went to defenders and keepers" is a property of the squad that was
	// bought, and it either moved or it did not.
	type squadShape struct {
		defGkp int // players in the fifteen at element type 1 or 2
		spend  int // tenths of a million spent on them
	}
	shapeOf := func(s *Season, ids []int) squadShape {
		var out squadShape
		for _, id := range ids {
			pl := s.Players[id]
			if pl == nil || (pl.Type != 1 && pl.Type != 2) {
				continue
			}
			out.defGkp++
			out.spend += priceAt(pl, 1)
		}
		return out
	}

	t.Log("prior      season     HOLD off   HOLD on    delta    POLICY off  POLICY on   " +
		"delta   DEF+GKP off   on      spend off  on")
	for _, p := range pairs {
		var holdOff, holdOn, polOff, polOn int
		var shapeOff, shapeOn squadShape
		for _, repair := range []bool{false, true} {
			prior, cur := load(p[0], repair), load(p[1], repair)
			sc := sweepConfig(cfg, 1, false)
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("%s: %v", p[1], err)
			}
			if repair {
				shapeOn = shapeOf(cur, res.OpeningSquad)
			} else {
				shapeOff = shapeOf(cur, res.OpeningSquad)
			}
			// The Full rung is HOLD, taken from the weekly pass rather than
			// Hold() for the reason the sweep harness gives: one pass produces
			// all three rungs, and two expressions of one quantity end with the
			// measured one not being the one that runs.
			h := sumInts(HoldCaptaincyWeekly(cur, prior, sc, res.OpeningSquad).Full)
			if repair {
				holdOn, polOn = h, res.Points
			} else {
				holdOff, polOff = h, res.Points
			}
		}
		note := ""
		if !TransferPathComparable(p[1]) {
			note = "  (HOLD only)"
		}
		if !affected[p[1]] && (holdOn != holdOff || polOn != polOff) {
			// Not an error, and the aggregate diagnostic had to establish that
			// separately. This season's xGC is provably untouched — the coverage
			// table above is exact — so a points difference cannot be the repair.
			// It is Optimize returning a different fifteen from identical inputs.
			note += "  <- xGC unchanged; this is optimiser nondeterminism"
		}
		t.Logf("%-10s %-10s %8d   %8d   %+6d   %9d   %9d   %+6d   %6d %6d   %7.1f %6.1f%s",
			p[0], p[1], holdOff, holdOn, holdOn-holdOff,
			polOff, polOn, polOn-polOff,
			shapeOff.defGkp, shapeOn.defGkp,
			float64(shapeOff.spend)/10, float64(shapeOn.spend)/10, note)
	}
}
