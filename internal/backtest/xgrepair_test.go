package backtest

import (
	"encoding/json"
	"testing"

	"armband/internal/fpl"
)

// aRepairedSeason is the season whose repair is fitted in season and validated against
// an aggregate the repair never sees. It is the strongest of the four and therefore the
// one a single-season property test should use.
const aRepairedSeason = "2022-23"

// seasonWithAHole builds a minimal season with the archive's actual defect shape: a
// player with real minutes and the given xG in the repaired window.
func seasonWithAHole(name string, el, gw int, xg float64) *Season {
	return &Season{Name: name, Players: map[int]*Player{
		el: {ID: el, Code: 461358, GWs: map[int]GW{
			gw: {Minutes: 90, Fixtures: 1, XG: xg},
		}},
	}}
}

// TestXGRepairOnlyFillsAHole pins idempotence.
//
// The repair must never overwrite a value the archive already carries. Two reasons,
// and the second is the one that matters in a year: running it twice must not double
// anything, and if FPL ever backfills its own weekly history the repair has to degrade
// to a no-op rather than fight the real data. A repair that competes with the source
// is worse than no repair, because the disagreement is invisible.
func TestXGRepairOnlyFillsAHole(t *testing.T) {
	t.Setenv("FPL_NO_XG_REPAIR", "")

	// Two properties, in every repaired season rather than only the first one someone
	// thought of. Note the window is the whole season for three of the four, so the
	// repair legitimately creates gameweeks the synthetic season does not have — which
	// is why the assertions are about the pre-filled cell and about the second pass,
	// not about how many rows were applied.
	filled := 0
	for _, name := range repairedSeasons() {
		s := seasonWithAHole(name, 1, 5, 0.42)
		before := s.Players[1].GWs[5].XG
		first, err := s.applyXGRepair()
		if err != nil {
			t.Fatalf("%s: applyXGRepair: %v", name, err)
		}
		// One: a cell the archive already filled is untouched.
		if got := s.Players[1].GWs[5].XG; got != before {
			t.Errorf("%s: xG changed from %v to %v on a gameweek that already had a "+
				"value; the repair must only ever fill a hole", name, before, got)
		}

		// Two: idempotence, asserted by running it again on its own output. This is
		// the property that matters in a year — if FPL ever backfills its weekly
		// history the repair meets real data everywhere and has to become a no-op.
		snapshot := map[int]GW{}
		for gw, g := range s.Players[1].GWs {
			snapshot[gw] = g
		}
		aggXG, aggXA := s.Players[1].XG, s.Players[1].XA
		second, err := s.applyXGRepair()
		if err != nil {
			t.Fatalf("%s: second applyXGRepair: %v", name, err)
		}
		if second.Applied != 0 {
			t.Errorf("%s: a second pass applied %d more rows; the repair is not "+
				"idempotent and running it twice doubles the xG it added",
				name, second.Applied)
		}
		if second.Skipped < first.Applied {
			t.Errorf("%s: the first pass filled %d cells and the second skipped only "+
				"%d, so a cell the repair itself wrote was not recognised as full",
				name, first.Applied, second.Skipped)
		}
		for gw, want := range snapshot {
			if got := s.Players[1].GWs[gw]; got != want {
				t.Errorf("%s: GW%d moved from %+v to %+v on a second pass",
					name, gw, want, got)
			}
		}
		// The aggregate half has to be idempotent too, and it fails differently: the
		// weekly rows are already full on a second pass so nothing is added to them,
		// but a rebuild that summed into the total rather than replacing an empty one
		// would double the season every time it ran.
		if s.Players[1].XG != aggXG || s.Players[1].XA != aggXA {
			t.Errorf("%s: the season aggregate moved from %.6f/%.6f to %.6f/%.6f on a "+
				"second pass", name, aggXG, aggXA, s.Players[1].XG, s.Players[1].XA)
		}
		if second.AggFilled != 0 {
			t.Errorf("%s: a second pass rebuilt %d players' season totals; the "+
				"aggregate rebuild must only ever fill an empty one",
				name, second.AggFilled)
		}
		filled += first.Applied
	}
	// The fixture is one player, so a season whose repair has no row for him
	// legitimately applies nothing. What must not happen is *every* season applying
	// nothing, because then the hole-filling path was never entered and this test
	// would pass on an empty repair — the "a test that passes because there is no
	// data is not a test" failure this package keeps writing down.
	if filled == 0 {
		t.Error("no season filled a single cell for the fixture player, so the " +
			"hole-filling path was never exercised. Either the shipped repair data " +
			"is empty (run stats/understat_xg_backfill.py) or the fixture's element " +
			"id appears in none of it and should be changed.")
	}
}

// TestRepairedAggregateMatchesTheWeeklyRows pins the aggregate half of the repair.
//
// The weekly half writes `g.XG` on the per-gameweek rows; `PreSeason` and the prior
// index read the season total `p.XG`. Before this, the second was never written, so a
// season used as a prior contributed zero expected goals however well its weeks were
// repaired — see rebuildXGAggregates for the three cells that hit.
//
// The invariant is the one that cannot drift: a rebuilt total must equal the sum of the
// rows it was rebuilt from. Asserted per player rather than in aggregate, since a
// season-wide sum would pass while two players' totals were swapped.
func TestRepairedAggregateMatchesTheWeeklyRows(t *testing.T) {
	t.Setenv("FPL_NO_XG_REPAIR", "")
	rebuilt := 0
	for _, name := range repairedSeasons() {
		s := seasonWithAHole(name, 1, 5, 0)
		res, err := s.applyXGRepair()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !xgRepairs[name].NoAggregate {
			// 2022-23's own aggregate is complete in the archive, so the repair must
			// leave it alone entirely. Adding the GW1-15 backfill to a total that
			// already contains those weeks would inflate the season by half again,
			// which is the failure this branch exists to prevent.
			if res.AggFilled != 0 || res.AggXG != 0 {
				t.Errorf("%s carries a complete FPL aggregate and the repair rebuilt "+
					"%d players' totals (+%.1f xG) anyway; that double-counts GW%d-%d",
					name, res.AggFilled, res.AggXG,
					xgRepairs[name].FirstGW, xgRepairs[name].LastGW)
			}
			continue
		}
		// A season the table says has no aggregate must not turn out to have one. If
		// it does, the table is wrong about the archive and the "only fill a hole"
		// guard is the only thing standing between that and a doubled season.
		if res.AggKept != 0 {
			t.Errorf("%s is marked NoAggregate and %d players already carried a "+
				"season total; the table and the archive disagree", name, res.AggKept)
		}
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			// Through the same helper the rebuild uses, and that is load-bearing rather
			// than tidy: summing the same gameweek map in two different orders gives
			// values that differ in the last bits, so a test with its own loop failed
			// here while printing both numbers identically. One implementation of one
			// quantity, which is this package's standing rule.
			xg, xa := weeklyXGTotals(p)
			if xg == 0 && xa == 0 {
				continue
			}
			rebuilt++
			if p.XG != xg || p.XA != xa {
				t.Errorf("%s element %d: aggregate is %.6f/%.6f, weekly rows sum to "+
					"%.6f/%.6f — a prior season is read through the aggregate, so a "+
					"mismatch is what a consumer actually sees",
					name, id, p.XG, p.XA, xg, xa)
			}
		}
	}
	if rebuilt == 0 {
		t.Error("no player's aggregate was rebuilt in any season, so this asserted " +
			"nothing. Either the shipped repair data is empty (run " +
			"stats/understat_xg_backfill.py) or no NoAggregate season is listed.")
	}
}

// TestWeeklyXGTotalsSumsInGameweekOrder pins the ordering, which is a reproducibility
// property rather than a correctness one.
//
// Floating-point addition is not associative, so summing a player's gameweek map in map
// order gives a value differing in the last bits from run to run. That is enough to
// matter: the aggregate feeds a prior, the prior feeds a score, and the optimiser returns
// a discrete fifteen, so 1e-16 can flip a slot and change a replayed season. It arrived
// as a test failure whose two numbers printed identically.
//
// The values are chosen so the order is *observable*, and that choice is checked rather
// than assumed — the first version of this test used 0.1 x gw, whose 38 terms sum
// identically in either direction, so it asserted nothing about ordering while appearing
// to. The self-check at the bottom is what caught that, and it is the reason the fixture
// looks arbitrary.
func TestWeeklyXGTotalsSumsInGameweekOrder(t *testing.T) {
	// Precomputed into slices, and that is not incidental. Go is permitted to fuse
	// `acc + a*b` into a single FMA instruction, and on arm64 it does — so a reference
	// loop that recomputed `0.13*gw*gw` inline produced a different total from one
	// adding the same values already stored as float64, and the first version of this
	// test failed on that rather than on any ordering. Both sides now add identical
	// stored floats, so the only thing under test is the order.
	var xgs, xas [39]float64
	for gw := 1; gw <= 38; gw++ {
		xgs[gw] = 0.13 * float64(gw) * float64(gw)
		xas[gw] = 0.7 / float64(gw)
	}

	p := &Player{ID: 1, GWs: map[int]GW{}}
	for gw := 1; gw <= 38; gw++ {
		p.GWs[gw] = GW{XG: xgs[gw], XA: xas[gw]}
	}
	var wantXG, wantXA float64
	for gw := 1; gw <= 38; gw++ {
		wantXG += xgs[gw]
		wantXA += xas[gw]
	}
	gotXG, gotXA := weeklyXGTotals(p)
	if gotXG != wantXG || gotXA != wantXA {
		t.Errorf("weeklyXGTotals gave %.20g/%.20g, ascending-gameweek order gives "+
			"%.20g/%.20g. The two differ in the last bits, which is what a map-order "+
			"sum looks like, and the rebuilt aggregate has to be reproducible",
			gotXG, gotXA, wantXG, wantXA)
	}

	// The self-check: both columns must actually be order-sensitive, or the assertion
	// above would pass for an implementation that summed in any order at all.
	var descXG, descXA float64
	for gw := 38; gw >= 1; gw-- {
		descXG += xgs[gw]
		descXA += xas[gw]
	}
	if descXG == wantXG || descXA == wantXA {
		t.Fatalf("this fixture is order-insensitive (xG %v, xA %v), so the assertion "+
			"above cannot detect an ordering change — pick values whose magnitudes "+
			"differ enough for reassociation to show",
			descXG == wantXG, descXA == wantXA)
	}
}

// TestRepairedXGCAggregateMatchesTheWeeklyRows is the xGC counterpart to
// TestRepairedAggregateMatchesTheWeeklyRows, and it did not exist.
//
// # The gap, which is not the one first recorded
//
// `weeklyXGTotals` was extracted so the xG rebuild and the test checking it could
// not drift apart, and `TestRepairedAggregateMatchesTheWeeklyRows` is that check:
// it asserts `p.XG` equals the sum of the rows it was rebuilt from, per player.
// `rebuildXGCAggregates` carried its own copy of the same 1..38 walk, so the
// argument was made once and implemented twice.
//
// ⚠️ The first write-up of that finding claimed the **xGC test** carried a copy
// too, citing "a diagnostic must never carry its own copy of the thing it is
// checking". That was wrong twice over. The walk it pointed at filters for
// ever-presents to derive club xGA — a different job sharing a bound — and the
// real defect was the opposite: **there was no xGC aggregate test at all.** A
// first attempt to close it added an order test over `weeklyTotal` in isolation,
// which never runs the rebuild and so cannot catch the regression it names.
//
// Concretely, what escapes an isolated order test: someone re-inlines this rebuild
// as `for gw, g := range p.GWs { xgc += g.XGC }`. Map order, irreproducible totals,
// and every test still green. This one runs the rebuild and compares its output
// against the shared walk, so that regression fails here.
//
// Per player rather than in aggregate, for the reason the xG version gives: a
// season-wide sum passes while two players' totals are swapped.
func TestRepairedXGCAggregateMatchesTheWeeklyRows(t *testing.T) {
	// Weekly rows present, season totals absent — the archive shape the rebuild
	// exists for. The magnitudes are deliberately uneven so that a sum taken in a
	// different order would land a ULP away rather than cancelling.
	s := &Season{Name: "2021-22", Players: map[int]*Player{}}
	for id := 1; id <= 4; id++ {
		p := &Player{ID: id, Team: 1, GWs: map[int]GW{}}
		for gw := 1; gw <= 38; gw++ {
			if gw%(id+1) == 0 {
				continue // a different, ragged set of gameweeks per player
			}
			p.GWs[gw] = GW{Minutes: 90, Fixtures: 1,
				XGC: 0.13*float64(gw)*float64(gw) + float64(id)/7}
		}
		s.Players[id] = p
	}
	// One player already carries a total, which the rebuild must not touch.
	s.Players[4].XGC = 123.5

	filled, kept, _ := s.rebuildXGCAggregates()
	if filled != 3 || kept != 1 {
		t.Fatalf("rebuilt %d totals and kept %d, want 3 and 1 — the fixture is not "+
			"exercising both branches", filled, kept)
	}
	for id := 1; id <= 3; id++ {
		p := s.Players[id]
		want := weeklyTotal(p, func(g GW) float64 { return g.XGC })
		if p.XGC != want {
			t.Errorf("element %d: rebuilt season xGC %.20g, but its own weekly rows "+
				"sum to %.20g. A difference in the last bits is what a map-order sum "+
				"looks like, and this aggregate feeds a prior, which feeds a score, "+
				"which an optimiser turns into a discrete fifteen", id, p.XGC, want)
		}
	}
	if s.Players[4].XGC != 123.5 {
		t.Errorf("a player whose season total was already present had it rewritten "+
			"to %.6f; the rebuild must only ever fill a zero", s.Players[4].XGC)
	}

	// The self-check: the fixture must be order-SENSITIVE, or the assertion above
	// would pass for a rebuild summing in any order at all. Paid for on the xG
	// side, where the first version used values that summed identically in either
	// direction and asserted nothing while appearing to.
	p := s.Players[1]
	var asc, desc float64
	for gw := 1; gw <= 38; gw++ {
		if g, ok := p.GWs[gw]; ok {
			asc += g.XGC
		}
	}
	for gw := 38; gw >= 1; gw-- {
		if g, ok := p.GWs[gw]; ok {
			desc += g.XGC
		}
	}
	if asc == desc {
		t.Fatal("this fixture is order-insensitive, so the per-player assertion " +
			"cannot detect an ordering change — pick values whose magnitudes differ " +
			"enough for reassociation to show")
	}
}

// TestRepairedAggregateDoesNotLeakIntoAPlayedSeason is the half of the pin that matters.
//
// Rebuilding a season total from all 38 gameweeks is the shape of a point-in-time leak,
// and this package refuses that repair elsewhere on exactly those grounds. What makes it
// safe here is that the aggregate is only ever read for a **prior** season, while the
// season being *played* is read through `PointInTime`, which accumulates weeks 1..through
// and never touches `p.XG`.
//
// That is a claim about behaviour, so it is asserted behaviourally rather than by trusting
// the comment: a view at GW5 must see the first five gameweeks and strictly less than the
// season, even though the aggregate now holds the whole of it.
func TestRepairedAggregateDoesNotLeakIntoAPlayedSeason(t *testing.T) {
	t.Setenv("FPL_NO_XG_REPAIR", "")

	// A season with the archive's actual shape: no aggregate, xG spread over the year.
	// Built locally rather than loaded, so this runs without the network.
	const el = 1
	cur := &Season{Name: "2020-21", Teams: []fpl.Team{{ID: 1}},
		Players: map[int]*Player{el: {ID: el, Code: 461358, Team: 1, Type: 4,
			Minutes: 38 * 90, NowCost: 100, Status: "a", GWs: map[int]GW{}}}}
	for gw := 1; gw <= 38; gw++ {
		cur.Players[el].GWs[gw] = GW{Minutes: 90, Fixtures: 1, XG: 0.25, XA: 0.10}
	}
	filled, _, _, _ := cur.rebuildXGAggregates()
	if filled != 1 {
		t.Fatalf("rebuildXGAggregates filled %d players, want 1", filled)
	}
	const wantSeason = 38 * 0.25
	if d := cur.Players[el].XG - wantSeason; d > 1e-9 || d < -1e-9 {
		t.Fatalf("aggregate is %.6f, want %.6f", cur.Players[el].XG, wantSeason)
	}

	// The prior is irrelevant here; PointInTime reads `cur` for a played gameweek.
	prior := &Season{Name: "2019-20", Players: map[int]*Player{}}
	boot, _ := PointInTime(cur, prior, 5)
	var seen float64
	for _, e := range boot.Elements {
		if e.ID == el {
			seen = float64(e.ExpectedGoals)
		}
	}
	const wantThrough5 = 5 * 0.25
	if d := seen - wantThrough5; d > 1e-9 || d < -1e-9 {
		t.Errorf("a view through GW5 reports %.6f expected goals, want %.6f. The "+
			"rebuilt season total is %.6f, and a played season reading it would be the "+
			"leak this repair is only safe without",
			seen, wantThrough5, cur.Players[el].XG)
	}
	if seen >= cur.Players[el].XG {
		t.Errorf("a view through GW5 sees %.6f against a season total of %.6f; the "+
			"cutoff is not truncating anything", seen, cur.Players[el].XG)
	}
}

// TestXGRepairIsOffWhenAsked pins the escape hatch at the level it is read.
func TestXGRepairIsOffWhenAsked(t *testing.T) {
	t.Setenv("FPL_NO_XG_REPAIR", "1")
	for _, name := range repairedSeasons() {
		s := seasonWithAHole(name, 1, 5, 0)
		res, err := s.applyXGRepair()
		if err != nil {
			t.Fatalf("%s: applyXGRepair: %v", name, err)
		}
		if res.Rows != 0 || res.Applied != 0 {
			t.Errorf("%s: FPL_NO_XG_REPAIR set and the repair still read %d rows and "+
				"applied %d", name, res.Rows, res.Applied)
		}
		if got := s.Players[1].GWs[5].XG; got != 0 {
			t.Errorf("%s: xG became %v with the repair switched off", name, got)
		}
	}
}

// TestXGRepairLeavesUnlistedSeasonsAlone — the repair is a table of seasons with a
// measured defect, and a mechanism that fires on "whatever season has a file" is one
// nobody can reason about later. This project gates the transfer bank and defensive
// contribution by season for the same reason: handing a season a rule it was not played
// under makes the replay measure something that never happened.
//
// The seasons named here are the ones with a *complete* FPL expected-goals series, so a
// repair reaching them would be overwriting real data rather than filling a hole.
func TestXGRepairLeavesUnlistedSeasonsAlone(t *testing.T) {
	t.Setenv("FPL_NO_XG_REPAIR", "")
	for _, name := range []string{"2023-24", "2024-25", "2025-26"} {
		if _, listed := xgRepairs[name]; listed {
			t.Fatalf("%s is in xgRepairs but this test asserts it is not repaired — "+
				"one of the two is wrong, and it is not this test's job to guess "+
				"which", name)
		}
		s := seasonWithAHole(name, 1, 5, 0)
		res, err := s.applyXGRepair()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Rows != 0 {
			t.Errorf("%s: the repair read %d rows; only %v are repaired",
				name, res.Rows, repairedSeasons())
		}
	}
}

// TestXGRepairSwitchWorksOnACacheHit is the important one, and it guards a hazard that
// would otherwise be silent.
//
// `Load` caches the fully-parsed `Season` as JSON. If the repair were applied inside
// `fetch` — the obvious place, beside the other loaders — it would be baked into the
// cached bytes. Then `FPL_NO_XG_REPAIR=1` would read a repaired cache and report the
// unrepaired arm as identical to the repaired one: an escape hatch that is a no-op
// while looking like a null result.
//
// That is not hypothetical. This file already records a cache-version bump that hit
// stale files and "reported no congestion anywhere — the null result looked exactly
// like a real one", and an appearance-estimator hatch that reached one consumer of two
// and was caught only as an unexplained zero in the prediction benchmark.
//
// So the repair lives in `repaired()`, outside the cache. This test simulates the
// round trip: marshal a season the way `Load` does, unmarshal it, and check that the
// switch still decides the outcome.
func TestXGRepairSwitchWorksOnACacheHit(t *testing.T) {
	orig := seasonWithAHole(aRepairedSeason, 1, 5, 0)
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// What is cached must be the UNREPAIRED archive. If XGRepair were serialised, or
	// the repair applied before the write, this is where it would show.
	var cached Season
	if err := json.Unmarshal(b, &cached); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cached.XGRepair.Applied != 0 || cached.XGRepair.Rows != 0 {
		t.Errorf("the cached season carries a repair report (%+v); XGRepair must be "+
			"json:\"-\" — it is a statement about a load, not data about a season",
			cached.XGRepair)
	}

	// Now the two arms, both starting from the same cached bytes.
	load := func(off bool) *Season {
		var s Season
		if err := json.Unmarshal(b, &s); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if off {
			t.Setenv("FPL_NO_XG_REPAIR", "1")
		} else {
			t.Setenv("FPL_NO_XG_REPAIR", "")
		}
		out, err := repaired(&s)
		if err != nil {
			t.Fatalf("repaired: %v", err)
		}
		return out
	}

	on, off := load(false), load(true)
	if off.XGRepair.Rows != 0 {
		t.Errorf("with the switch off the repair still read %d rows from a cache hit",
			off.XGRepair.Rows)
	}
	// The arms must be *capable* of differing. With header-only shipped data both are
	// zero, which is legitimate — so assert the mechanism rather than a value, and say
	// so, since a test that passes because there is no data is not a test.
	if on.XGRepair.Rows == 0 {
		t.Log("note: no repair rows are shipped, so this asserted the plumbing and " +
			"not a difference. Run stats/understat_xg_backfill.py to populate it.")
	}
}

// TestXGRepairRowsAreInsideTheWindow makes the shipped data and the table agree.
//
// A row outside a season's window means the file and `xgRepairs` disagree, and the right
// response is to fail rather than to skip the row: one of the two is wrong, and skipping
// would hide which. Same reasoning as the loader refusing a gameweek number it cannot
// place instead of dropping it.
func TestXGRepairRowsAreInsideTheWindow(t *testing.T) {
	t.Setenv("FPL_NO_XG_REPAIR", "")
	for _, name := range repairedSeasons() {
		// An empty season: every row will be "unknown", which exercises the window
		// check on every row without needing the real player set.
		s := &Season{Name: name, Players: map[int]*Player{}}
		res, err := s.applyXGRepair()
		if err != nil {
			t.Fatalf("%s: the shipped repair data disagrees with its window: %v",
				name, err)
		}
		if res.Rows > 0 && res.Unknown != res.Rows {
			t.Errorf("%s: with no players, all %d rows should be unknown, got %d",
				name, res.Rows, res.Unknown)
		}
	}
}

// TestEveryRepairShipsItsOffset pins the caveat to the data.
//
// A backfilled season's largest single assumption is the provider offset its xG was
// divided by, and for the three seasons with no FPL expected-goals column at all that
// offset is *borrowed* from other seasons rather than fitted. If that fact lives only in
// a script's stdout then nothing downstream can report it, and a snapshot would name a
// backfilled season without naming what its xG is on.
//
// So every shipped repair must carry a sidecar, the sidecar must agree with the loader's
// own table about the window and the renumber, and it must say which kind of offset it
// used. `readXGRepairMeta` enforces the first two on every load; this asserts the third
// and that the enforcement is actually reached.
func TestEveryRepairShipsItsOffset(t *testing.T) {
	t.Setenv("FPL_NO_XG_REPAIR", "")
	for _, name := range repairedSeasons() {
		s := &Season{Name: name, Players: map[int]*Player{}}
		res, err := s.applyXGRepair()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.Rows == 0 {
			t.Logf("%s: no repair rows shipped; run stats/understat_xg_backfill.py", name)
			continue
		}
		m := res.Meta
		switch {
		case m.OffsetSource != "in-season" && m.OffsetSource != "borrowed":
			t.Errorf("%s: offset_source is %q, want in-season or borrowed", name,
				m.OffsetSource)
		case m.OffsetXG < 0.5 || m.OffsetXG > 2:
			t.Errorf("%s: offset_xg %v is not a plausible provider ratio", name, m.OffsetXG)
		case m.OffsetXA < 0.5 || m.OffsetXA > 2:
			t.Errorf("%s: offset_xa %v is not a plausible provider ratio", name, m.OffsetXA)
		}
		// The join check, which is the one that validates the crosswalk rather than
		// the offset: Understat goals against FPL goals over the joined cells. Both
		// sources count an exact integer, so a wrong player id or a date mapped to
		// the wrong gameweek shows here and nowhere else. It reads 1.0000 on every
		// repaired season, and a tenth of a per cent of slack is generous.
		if m.JoinGoalsRatio < 0.999 || m.JoinGoalsRatio > 1.001 {
			t.Errorf("%s: the harvest's goals ratio is %.4f, not 1.0000 — the "+
				"crosswalk or the date-to-gameweek mapping is wrong, and the xG it "+
				"produced is attached to the wrong players or the wrong weeks",
				name, m.JoinGoalsRatio)
		}
		// The sharper half of the same check, and the ratio above is not sufficient
		// without it. A ratio of exactly 1.0000 cancels compensating errors: 2019-20
		// reads 1.0000 with two disagreeing cells, because Understat gives De Bruyne a
		// goal FPL gives to David Silva. A mis-mapped *date* has that same signature —
		// one week over, one week short — so the count is what would catch it.
		//
		// The bound is deliberately a small absolute number rather than a share. The
		// population is five committed seasons whose counts are known (0/2/0/0/0), so
		// the guard's job is to notice a *change*, and a percentage of ten thousand
		// cells would let fifty new disagreements through in silence.
		const knownMismatches = 2
		if m.JoinGoalCells == 0 {
			t.Errorf("%s: the sidecar records no goal-anchor cell count, so its ratio "+
				"cannot be read as a check — re-run stats/understat_xg_backfill.py",
				name)
		} else if m.JoinGoalMismatch > knownMismatches {
			t.Errorf("%s: %d of %d goal-anchor cells disagree, above the %d this "+
				"archive is known to carry (2019-20's De Bruyne/David Silva pair). The "+
				"ratio cancels a cell that is one week early against one that is one "+
				"week late, so a growing count is how a date-mapping error looks here",
				name, m.JoinGoalMismatch, m.JoinGoalCells, knownMismatches)
		}
		if m.Rows != res.Rows {
			t.Errorf("%s: sidecar claims %d rows, the CSV has %d", name, m.Rows, res.Rows)
		}
	}
}

// TestNineteenTwentyIsRenumberedToThirtyEight pins the one season whose gameweeks are
// not numbered 1-38.
//
// COVID stopped 2019-20 after GW29 and FPL numbered the restarted rounds **39-47**
// rather than reusing 30-38. `loadGameweeks` drops anything outside 1..38, so without
// the renumber the replay silently loses all nine restart rounds — a quarter of the
// season, present in the archive, reading as a season that stopped in March. Every
// figure computed from it would be plausible and a quarter short, which is the
// doubles-counting failure again.
//
// The arithmetic is duplicated in `stats/understat_xg_backfill.py`, because the repair
// rows have to land in the same weeks the loader puts the football in. That is the
// duplicate this test exists to hold equal: it reproduces the mapping explicitly rather
// than calling the function on both sides of an assertion, so a change to the Go rule
// fails here instead of agreeing with itself.
func TestNineteenTwentyIsRenumberedToThirtyEight(t *testing.T) {
	// Every label the archive uses, and the gameweek it must become. 38 in, 38 out,
	// contiguous 1..38 — which is the property that makes the shift safe: events 30-38
	// are entirely absent from 2019-20's fixtures.csv, so nothing can collide.
	seen := map[int]bool{}
	for gw := 1; gw <= 29; gw++ {
		got := renumberGW("2019-20", gw)
		if got != gw {
			t.Errorf("2019-20 GW%d renumbered to %d; the pre-COVID rounds keep their "+
				"numbers", gw, got)
		}
		seen[got] = true
	}
	for gw := 39; gw <= 47; gw++ {
		want := gw - 9
		got := renumberGW("2019-20", gw)
		if got != want {
			t.Errorf("2019-20 GW%d renumbered to %d, want %d", gw, got, want)
		}
		seen[got] = true
	}
	for gw := 1; gw <= 38; gw++ {
		if !seen[gw] {
			t.Errorf("gameweek %d is not produced by any archive label, so the "+
				"renumbered season has a hole in it", gw)
		}
	}
	if len(seen) != 38 {
		t.Errorf("the renumber produced %d distinct gameweeks, want 38", len(seen))
	}

	// And it must reach only that season, over the whole label range rather than a
	// handful of spot checks. A renumber applied to a season numbered 1-38 would shift
	// its last nine rounds into 30-38 on top of the real ones — and since every figure
	// in the record is measured on the four seasons below, that identity is what says
	// this change is inert for all of them.
	for _, name := range []string{
		"2020-21", "2021-22", "2022-23", "2023-24", "2024-25", "2025-26",
	} {
		for gw := 1; gw <= 47; gw++ {
			if got := renumberGW(name, gw); got != gw {
				t.Errorf("%s GW%d renumbered to %d; only 2019-20 is renumbered",
					name, gw, got)
			}
		}
	}
}

// TestRestartGameweeksAreASchemaCheck pins the cache guard for the renumber.
//
// A 2019-20 cache written before `renumberGW` existed holds 29 gameweeks, and reads as a
// season that stopped in March. A version bump alone would not catch it — this package
// records v2, v3 and v4 archives sitting beside the current ones, so leftovers
// demonstrably accumulate — and bumping would invalidate the four seasons this change
// does not touch. So the check is targeted at the season that can be wrong.
func TestRestartGameweeksAreASchemaCheck(t *testing.T) {
	truncated := &Season{Name: "2019-20", Players: map[int]*Player{
		1: {ID: 1, GWs: map[int]GW{29: {Minutes: 90, Fixtures: 1}}},
	}}
	if truncated.hasRestartGameweeks() {
		t.Error("a 2019-20 season with nothing after GW29 was accepted; that is the " +
			"pre-renumber cache, and accepting it replays three quarters of a season " +
			"as if it were all of it")
	}
	whole := &Season{Name: "2019-20", Players: map[int]*Player{
		1: {ID: 1, GWs: map[int]GW{29: {Minutes: 90, Fixtures: 1},
			38: {Minutes: 90, Fixtures: 1}}},
	}}
	if !whole.hasRestartGameweeks() {
		t.Error("a renumbered 2019-20 was rejected")
	}
	// Every other season must pass unconditionally, or the check would reject a
	// perfectly good 2022-23 for the crime of not being 2019-20.
	for _, name := range []string{"2020-21", "2022-23", "2025-26"} {
		s := &Season{Name: name, Players: map[int]*Player{
			1: {ID: 1, GWs: map[int]GW{1: {Minutes: 90, Fixtures: 1}}},
		}}
		if !s.hasRestartGameweeks() {
			t.Errorf("%s was rejected by a check that is 2019-20's alone", name)
		}
	}
}
