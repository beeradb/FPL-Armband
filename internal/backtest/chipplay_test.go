package backtest

import (
	"context"
	"testing"

	"armband/internal/analysis"
)

// The wildcard and free hit change *which* squad is fielded, which is a
// different kind of change from the two scoring chips and has two failure modes
// that a points total will not reveal.
//
// A wildcard that does not settle the wallet player by player leaves the
// purchase prices describing a squad the manager no longer owns, so every
// subsequent selling price — and therefore every subsequent budget — is wrong
// for the rest of the season, silently.
//
// A free hit that writes its temporary fifteen back into the held squad turns a
// one-week loan into a permanent free rebuild, which would be the single most
// flattering bug it is possible to introduce here.

func chipSim(t *testing.T) (*Season, *Season, SimConfig) {
	t.Helper()
	cfg := loadConfig(t)
	ctx := context.Background()
	prior, err := Load(ctx, cfg.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unreachable: %v", err)
	}
	cur, err := Load(ctx, cfg.CacheDir, "2025-26")
	if err != nil {
		t.Skipf("archive unreachable: %v", err)
	}
	return cur, prior, SimConfig{
		Weights: cfg.Weights, MinGain: cfg.Review.MinGainForTransfer,
		MinGainHit: cfg.Review.MinGainForHit, BankUpTo: sweepBankLimit,
		MaxHits: cfg.Review.MaxHitsPerWeek, Budget: 1000,
		FreeCost: cfg.Review.FreeTransferValue, StartGW: 1, WeeklyXI: true,
	}
}

// TestFreeHitFieldsABorrowedSquad — the chip must actually field a different
// fifteen, and none of it may survive into the following week.
//
// Both halves matter and they fail in opposite directions. If the temporary
// squad is never built, the chip is inert and scores the ordinary team. If it
// is written back into the permanent squad, a one-week loan becomes a free
// permanent rebuild, which is the most flattering bug available here.
//
// Compared within a single run rather than against a chipless one: a free hit
// week correctly makes no permanent transfer, so the two runs diverge from that
// week onward for a legitimate reason.
func TestFreeHitFieldsABorrowedSquad(t *testing.T) {
	cur, prior, base := chipSim(t)
	// Second SET, not merely a later week: chipSim replays 2025-26, where FPL
	// resets the chips at GW19, so a first-set chip at GW20 is one nobody could
	// have played. ValidateChipSets refuses it — which is the guard working, and
	// is why these three tests changed when it was moved into Simulate.
	base.Chips2 = analysis.ChipPlan{FreeHit: 20}

	got, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	var before, during, after Week
	for _, wk := range got.Weeks {
		switch wk.GW {
		case 19:
			before = wk
		case 20:
			during = wk
		case 21:
			after = wk
		}
	}
	if !during.FreeHit {
		t.Fatal("GW20 not marked as a free hit; the chip never fired")
	}

	permanent := map[int]bool{}
	for _, id := range during.Squad {
		permanent[id] = true
	}
	borrowed := 0
	for _, id := range during.XI {
		if !permanent[id] {
			borrowed++
		}
	}
	if borrowed == 0 {
		t.Error("the free hit fielded only players from the permanent squad; " +
			"the temporary fifteen was never used")
	}

	// The permanent squad is untouched ACROSS THE CHIP WEEK ITSELF. Deliberately
	// not checked into the following gameweek: a free hit hands the squad back
	// and ordinary transfers resume immediately, so a move at GW21 is the rules
	// working, not the chip leaking.
	//
	// It was checked both ways originally and passed, because that branch's
	// policy happened to make no move at GW21. On this branch it makes two —
	// the autosub legality fix, the unregistered-pool fix and the determinism
	// fix all move the scoring — and the test failed for a behaviour that is
	// correct. A test that asserts more than the rule it names will eventually
	// fail on something legal; the invariant that matters is the chip week
	// making no permanent transfer, which TestFreeHitMakesNoPermanentTransfer
	// pins directly.
	_ = after
	for _, pair := range [][2]Week{{before, during}} {
		have := map[int]bool{}
		for _, id := range pair[0].Squad {
			have[id] = true
		}
		for _, id := range pair[1].Squad {
			if !have[id] {
				t.Errorf("player %d entered the permanent squad across GW%d->GW%d "+
					"of a free hit", id, pair[0].GW, pair[1].GW)
			}
		}
	}
}

// TestWildcardSettlesTheWallet — a wildcard must leave the squad affordable at
// the money the manager actually had, not at market value.
func TestWildcardSettlesTheWallet(t *testing.T) {
	cur, prior, base := chipSim(t)
	// Second SET, not merely a later week: chipSim replays 2025-26, where FPL
	// resets the chips at GW19, so a first-set chip at GW20 is one nobody could
	// have played. ValidateChipSets refuses it — which is the guard working, and
	// is why these three tests changed when it was moved into Simulate.
	base.Chips2 = analysis.ChipPlan{Wildcard: 20}

	got, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}

	var wc Week
	for _, wk := range got.Weeks {
		if wk.GW == 20 {
			wc = wk
		}
	}
	if !wc.Wildcard {
		t.Fatal("GW20 not marked as a wildcard; the chip never fired")
	}
	if wc.Transfers == 0 {
		t.Error("a wildcard that changed nobody is not exercising the rebuild")
	}
	if wc.HitCost != 0 {
		t.Errorf("wildcard week charged %d points of hits; it is meant to be free", wc.HitCost)
	}
	// The bank may not go negative at any point after the rebuild, which is the
	// signature of having been quoted market value rather than selling value.
	for _, wk := range got.Weeks {
		if wk.GW >= 20 && wk.Bank < 0 {
			t.Errorf("GW%d bank is %d — the wildcard spent money the manager did not have",
				wk.GW, wk.Bank)
		}
	}
}

// TestOneChipPerGameweek — FPL allows one chip per gameweek, and a replay that
// does not enforce it will silently measure an illegal sequence. The first
// version of TestDiagChipSequence played a wildcard and a bench boost in the
// same week and produced a confident, meaningless number.
func TestOneChipPerGameweek(t *testing.T) {
	for _, p := range []analysis.ChipPlan{
		{Wildcard: 20, BenchBoost: 20},
		{FreeHit: 30, TripleCaptain: 30},
		{Wildcard: 8, FreeHit: 8},
	} {
		if err := ValidateChipSets("2022-23", p, analysis.ChipPlan{}); err == nil {
			t.Errorf("two chips in one gameweek accepted: %+v", p)
		}
	}
	// The legal sequence — wildcard, then boost the week after — must pass.
	for _, p := range []analysis.ChipPlan{
		{Wildcard: 20, BenchBoost: 21},
		{Wildcard: 8, FreeHit: 30, BenchBoost: 9, TripleCaptain: 34},
		{},
	} {
		if err := ValidateChipSets("2022-23", p, analysis.ChipPlan{}); err != nil {
			t.Errorf("legal plan rejected: %+v — %v", p, err)
		}
	}
}

// TestFreeHitMakesNoPermanentTransfer — everything a free hit does reverts, so
// the permanent squad must be identical either side of it. If the ordinary
// transfer decision still runs that week, the manager gets handed back a squad
// he never chose.
func TestFreeHitMakesNoPermanentTransfer(t *testing.T) {
	cur, prior, base := chipSim(t)
	// Second SET, not merely a later week: chipSim replays 2025-26, where FPL
	// resets the chips at GW19, so a first-set chip at GW20 is one nobody could
	// have played. ValidateChipSets refuses it — which is the guard working, and
	// is why these three tests changed when it was moved into Simulate.
	base.Chips2 = analysis.ChipPlan{FreeHit: 20}

	got, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	var before, after Week
	for _, wk := range got.Weeks {
		switch wk.GW {
		case 19:
			before = wk
		case 20:
			after = wk
		}
	}
	if !after.FreeHit {
		t.Fatal("GW20 not marked as a free hit")
	}
	if after.Transfers != 0 {
		t.Errorf("free hit week made %d permanent transfers; it may make none", after.Transfers)
	}
	held := map[int]bool{}
	for _, id := range before.Squad {
		held[id] = true
	}
	for _, id := range after.Squad {
		if !held[id] {
			t.Errorf("player %d entered the permanent squad during a free hit week", id)
		}
	}
}

// TestAPlacedFreeHitAlwaysFires — a free hit that is placed must either be
// played or fail loudly, and never quietly not happen.
//
// The call site discarded `freeHitSquad`'s error until 2026-08-13, so a build
// that failed left `fielded` as the permanent squad and `week.FreeHit` false,
// and the cell scored as an ordinary week. Nothing anywhere recorded that the
// chip had not fired. That is the failure this record keeps meeting from the
// other direction — a null that means "the intervention could not run" read as
// a null that means "the intervention is worth nothing" — and it is worse here
// than in the unwired-knob cases, because the wildcard four lines above always
// propagated, so the two chips disagreed about what a failure means.
//
// Placed at four weeks rather than one, across both chip sets: the single GW20
// case the other tests use passes whenever that particular build succeeds, which
// says nothing about a cell entered elsewhere in a sweep.
func TestAPlacedFreeHitAlwaysFires(t *testing.T) {
	cur, prior, base := chipSim(t)

	// chipSim replays 2025-26, which grants two sets — first expires after
	// GW19, second runs GW20-38. A week must be placed in the set that could
	// actually have played it or ValidateChipSets refuses the plan.
	for _, c := range []struct {
		gw     int
		second bool
	}{{8, false}, {15, false}, {22, true}, {31, true}} {
		b := base
		if c.second {
			b.Chips2 = analysis.ChipPlan{FreeHit: c.gw}
		} else {
			b.Chips = analysis.ChipPlan{FreeHit: c.gw}
		}
		got, err := Simulate(cur, prior, b)
		if err != nil {
			t.Fatalf("free hit at GW%d: %v", c.gw, err)
		}
		fired := false
		for _, wk := range got.Weeks {
			if wk.GW == c.gw {
				fired = wk.FreeHit
			}
		}
		if !fired {
			t.Errorf("a free hit placed at GW%d was not played, and the season "+
				"returned no error saying so", c.gw)
		}
	}
}

// TestAFreeHitThatCannotBuildIsAnError — the failure the call site used to
// discard has to exist before propagating it means anything.
//
// TestAPlacedFreeHitAlwaysFires above cannot reach this: at every placement it
// tries, the optimiser builds a fifteen quite happily, so it would have passed
// against the swallowed error too. What it pins is the invariant, not the fix.
// This pins the other half — that `freeHitSquad` reports a failed build rather
// than returning a short squad — and the two together are what close the gap.
func TestAFreeHitThatCannotBuildIsAnError(t *testing.T) {
	cur, prior, base := chipSim(t)

	// Built through EngineAt rather than by hand: an engine assembled here would
	// score players with no recency index, which TestEveryScoringEngineGetsRecency
	// exists to refuse — and it caught this test on its first run.
	hz := base
	hz.Weights.Horizon = 1
	e, _ := EngineAt(cur, prior, 19, hz)

	// Starved of money, at £10.0m. Two earlier drafts of this line were wrong in
	// ways worth recording, because both look like they should work:
	//
	// An expected-minutes floor no footballer can clear does NOT empty the pool
	// — bench fodder under £4.5m is deliberately exempt from that floor
	// (squad.go:456-459), so the optimiser built a legal fifteen of reserves.
	//
	// And a budget of *zero* does not starve it either: `Optimize` reads 0 as
	// "unset" and substitutes the default £100.0m, so the squad came back fully
	// priced. Only a budget that is small and non-zero actually binds.
	if _, err := freeHitSquad(e, cur, newWallet(100), nil, 20, 55, base); err == nil {
		t.Error("freeHitSquad built a fifteen from an empty pool; a failed build " +
			"must be an error, or propagating it at the call site is worth nothing")
	}
}

// TestChipsAreOffByDefault — every figure recorded before chips existed is a
// chipless season, so an unset plan must reproduce it exactly.
func TestChipsAreOffByDefault(t *testing.T) {
	cur, prior, base := chipSim(t)

	a, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	b := base
	b.Chips = analysis.ChipPlan{} // explicit zero
	c, err := Simulate(cur, prior, b)
	if err != nil {
		t.Fatal(err)
	}
	if a.Points != c.Points {
		t.Errorf("an empty chip plan changed the season: %d against %d", c.Points, a.Points)
	}
}
