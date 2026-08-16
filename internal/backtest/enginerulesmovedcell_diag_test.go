package backtest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"armband/internal/analysis"
)

// Why did ONE cell of the confinement grid move, and is the new number the right
// one?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTheMovedCell -v -timeout 30m
//	DIAG=1 FPL_NO_SEASON_SCORING_RULES=1 \
//	    go test ./internal/backtest -run TestDiagTheMovedCell -v -timeout 30m
//
// Pinning the engine's scoring rules per season left `hold_points` and the
// opening fifteen byte-identical across the whole grid and moved `policy_points`
// in exactly one cell — **2023-24 entry GW11, 1730 pinned against 1732
// unpinned**. A two-point move in one cell of thirty-six is far below anything
// this harness can resolve, and that is *not* the question. The question is
// whether the two points are a defect being corrected or a defect being
// introduced, and "it is too small to matter" answers neither.
//
// Three things have to be established, and only the first is cheap:
//
//  1. **which rule differs** between today's constants and 2023-24's table, and
//     through which channel — printed below, from the tables themselves;
//  2. **which population mediates it**, because a rule that differs and reaches
//     nobody cannot move a cell;
//  3. **what actually changed in the replayed season** — the moves, week by week,
//     differenced between the arms. That is the only one that distinguishes "the
//     rule acted" from "the transfer argmax diverged".
//
// ⚠️ **`-count=1` is not optional here and its absence produced a wrong answer
// once already.** Both arms are the same package at the same commit and differ
// only in an environment variable read at package initialisation, so Go's test
// cache served the pinned run's output for the unpinned one — the two arms
// printed identical move lists and identical points, which reads exactly like
// "the hatch is inert" and is the strongest possible wrong answer, since it is
// what a confinement result is supposed to look like. `staleness_test.go` records
// the same trap for the snapshot recipe.
//
// # THE ANSWER, run 2026-08-16
//
// The rule is `Goal[1]`: **6 pinned against 10 unpinned**, and it is the only
// channel that differs — all four positions' goal, clean-sheet and concede values
// and the assist are enumerated above and match otherwise.
//
// The mediator is **expected** goals, not realised ones. 2023-24 holds **0**
// goalkeeper goals, and **12 of 85** goalkeepers in the GW11 pool are still
// re-priced on `Score`, because `baseXP90` prices `XG90 x scale x Goal[pos]`.
//
// What changed in the season is **timing, not selection**. Both arms make the
// same 27 transfers and take the same 1 hit, and the move lists differ only in
// that GW29 and GW30 swap contents:
//
//	pinned    GW29 Raya(GKP)->Pickford(GKP), Bowen->Odegaard   GW30 Saliba->Gabriel
//	unpinned  GW29 Saliba->Gabriel                             GW30 Raya->Pickford, Bowen->Odegaard
//
// The player whose price moved is the goalkeeper, and it is the goalkeeper's move
// whose week changed. So the rule did act — but the two points measure **when
// three transfers were made**, which is a draw from the transfer path's own
// 303-point spread, not the value of the rule.
//
// # ⚠️ AND THE BOUNDARY, NOT THE PIN, IS WHAT MOVES IT
//
// `keeperGoalRuleChangeSeason` is **bounded, not measured**: the change happened
// somewhere in 2021-22..2024-25 and the constant takes the LATEST end the
// evidence permits, which applies the pre-modern 6 to 2021-22, 2022-23 and
// 2023-24 on no direct evidence at all. Re-run with the constant temporarily set
// to the EARLY end, `"2021-22"`:
//
//	2023-24 element_type 1: goal 10/10 — 0 of 85 goalkeepers re-priced
//	CELL 2023-24 start=11 policy_points=1732   (the pre-pin value)
//
// Repeated over the whole 72-cell confinement grid rather than this one cell, the
// early boundary reproduces the **unpinned** arm byte for byte in **72 of 72** —
// all six 2022-23 cells and the 2023-24 one included.
//
// So **the pin moves nothing and the constant moves everything**. Pinning the
// engine's rules per season is byte-identical on replayed points; every moved cell
// belongs to `keeperGoalRuleChangeSeason` taking the late end of a range this
// repository cannot resolve. That is not an argument against pinning — replaying a
// finished season under a rule nobody played under is wrong however small it is —
// but it is the reason nobody may read those points as evidence that the new
// number is the correct one, and it is why a constant that was instrumentation-only
// is points-load-bearing from here. The constant was restored immediately; the run
// is recorded because it is what makes the direction question answerable rather
// than a matter of opinion.
func TestDiagTheMovedCell(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cc := loadConfig(t)

	// # 1. The rule
	//
	// There is exactly one amendment in this table, so this is a complete
	// enumeration rather than a search: anything else differing would be a defect
	// in `ScoringRulesFor` and is asserted against below.
	const season = "2023-24"
	pinned, live := analysis.ScoringRulesFor(season), analysis.ScoringRulesFor("")
	t.Logf("RULE: %s against the live table", season)
	for pos := 1; pos <= 4; pos++ {
		g1, g2 := pinned.Goal[pos], live.Goal[pos]
		c1, c2 := pinned.CleanSheet[pos], live.CleanSheet[pos]
		b1, ok1 := pinned.ConcedeBlock[pos]
		b2, ok2 := live.ConcedeBlock[pos]
		mark := "  "
		if g1 != g2 || c1 != c2 || b1 != b2 || ok1 != ok2 {
			mark = "**"
		}
		t.Logf("%s element_type %d: goal %v/%v  clean sheet %v/%v  concede block %v(%v)/%v(%v)",
			mark, pos, g1, g2, c1, c2, b1, ok1, b2, ok2)
	}
	t.Logf("   assist %v/%v", pinned.Assist, live.Assist)

	// # 2. The mediating population
	//
	// ⚠️ **The realised-goals reading is the trap, and it is the one the
	// instrument's own author fell into and had to retract.** The archive holds
	// exactly ONE goalkeeper goal in ten seasons (Alisson, 2020-21 GW36), so
	// "a goalkeeper's goal is worth 6 rather than 10" looks like it can reach
	// nothing in 2023-24. It reaches plenty, because `baseXP90` prices
	// `XG90 x scale x Goal[pos]` — the EXPECTED half — and every goalkeeper with a
	// non-zero blended expected-goals rate is re-priced whether he ever shoots or
	// not. Counted here rather than argued.
	prior, err := Load(context.Background(), cc.CacheDir, "2022-23")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	cur, err := Load(context.Background(), cc.CacheDir, season)
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	realisedKeeperGoals := 0
	for _, p := range cur.Players {
		if p.Type != 1 {
			continue
		}
		for gw := 1; gw <= 38; gw++ {
			if g, ok := p.GWs[gw]; ok {
				realisedKeeperGoals += g.Goals
			}
		}
	}

	// ⚠️ **Both engines get the SAME fixtures, and a first version of this passed
	// `nil` to the second.** `Score` carries `FixtureAdjXP90`, so an engine with no
	// fixture list scores every position differently and the assertion below fired
	// on 165 defenders, 222 midfielders and 57 forwards — a re-pricing that had
	// nothing to do with the table. It read exactly like `ScoringRulesFor` having
	// drifted on a channel nobody amended, which is why the assertion is an
	// assertion: the wrong answer here was loud rather than plausible.
	sc := sweepConfig(cc, 11, true)
	boot, fx := PointInTime(cur, prior, 10)
	e := analysis.NewEngineFull(boot, fx, cc.Weights, analysis.Congestion{}, analysis.RoleRisk{})
	e.Priors = sc.priors(cur, prior)
	e.Recent = sc.recentIndex(cur, 10)
	e.TeamForm = newTeamFormIndex(cur, 10)

	flat := *boot
	flat.Season = ""
	un := analysis.NewEngineFull(&flat, fx, cc.Weights, analysis.Congestion{}, analysis.RoleRisk{})
	un.Priors = e.Priors
	un.Recent = e.Recent
	un.TeamForm = e.TeamForm

	keepers, withRate, byPos := 0, 0, map[int]int{}
	for i := range boot.Elements {
		el := &boot.Elements[i]
		if el.ElementType == 1 {
			keepers++
		}
		pm := e.Metrics(el)
		um := un.Metrics(&flat.Elements[i])
		if pm.Score != um.Score {
			byPos[el.ElementType]++
			if el.ElementType == 1 {
				withRate++
			}
		}
	}
	t.Logf("MEDIATOR at the GW11 entry cutoff: %d realised goalkeeper goals in all of %s; "+
		"%d goalkeepers in the pool, %d of whom are re-priced on SCORE",
		realisedKeeperGoals, season, keepers, withRate)
	for pos := 1; pos <= 4; pos++ {
		if byPos[pos] > 0 && pos != 1 {
			t.Errorf("element_type %d is re-priced by a table whose only amendment is "+
				"the goalkeeper's goal — %d players. Either ScoringRulesFor has "+
				"drifted on another channel or the two engines differ in something "+
				"other than the table", pos, byPos[pos])
		}
	}

	// # 3. What changed in the replayed season
	//
	// The moves, in order, with the gameweek and the points the cell finished on.
	// Run the two arms in separate processes and diff these lines: the escape
	// hatch is read at package initialisation, so one process cannot hold both.
	res, err := Simulate(cur, prior, sc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("CELL 2023-24 start=11 policy_points=%d transfers=%d hits=%d",
		res.Points, res.Transfers, res.Hits)
	for _, m := range res.Moves {
		t.Logf("MOVE gw=%02d out=%d in=%d", m.GW, m.OutID, m.InID)
	}
	// The names too, so a diff is readable without a second lookup.
	name := map[int]string{}
	for _, p := range cur.Players {
		name[p.ID] = fmt.Sprintf("%s(%s)", p.WebName, analysis.PositionForElementType(p.Type))
	}
	for _, m := range res.Moves {
		t.Logf("NAMED gw=%02d out=%s in=%s", m.GW, name[m.OutID], name[m.InID])
	}
}
