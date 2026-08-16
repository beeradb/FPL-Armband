package backtest

import (
	"context"
	"math"
	"testing"

	"armband/internal/analysis"
)

// The two silent-failure traps in the xPoints instrument, pinned on the archive
// rather than on a fixture, because both are about data this project already
// holds and neither had ever been exercised.
//
// `internal/analysis/xpointsrules_test.go` pins the arithmetic. These pin the
// **wiring**: that `repaired()` resolves a table onto every player, that the
// table is the season's own, and that the position the instrument cannot price
// really is in the archive and really cannot reach a squad.

// TestEveryLoadedPlayerCarriesHisOwnSeasonsScoringRules.
//
// `Player.Rules` is `json:"-"` and set in `repaired()`, which runs on both the
// cache-hit and the fetch path. A field like that is exactly what this file's
// header catalogues going wrong — `DefCon` and `Conversion` both had to be moved
// to this side of the cache write after being written into it as zero — so the
// property is worth asserting rather than inferring from the absence of a panic
// somewhere else.
//
// Two seasons, on opposite sides of the one rule change this repository can
// demonstrate, so the test also shows the resolution is per season and not one
// table shared by the process.
func TestEveryLoadedPlayerCarriesHisOwnSeasonsScoringRules(t *testing.T) {
	cc := loadConfig(t)
	// Season NAMES, not a copy of a rule: the boundary itself lives in
	// `analysis.ScoringRulesFor` and is read from it below.
	for _, name := range []string{"2020-21", "2025-26"} {
		s, err := Load(context.Background(), cc.CacheDir, name)
		if err != nil {
			t.Skipf("archive unavailable: %v", err)
		}
		if len(s.Players) == 0 {
			t.Fatalf("%s loaded no players", name)
		}
		want := analysis.ScoringRulesFor(name)
		for _, p := range s.Players {
			if p.Rules.Season != name {
				t.Fatalf("%s: player %d carries rules for season %q — repaired() is "+
					"not resolving the season's own points table onto every player, "+
					"and the instrument will refuse or misprice his rows",
					name, p.ID, p.Rules.Season)
			}
			if p.Rules.Goal[1] != want.Goal[1] {
				t.Fatalf("%s: player %d prices a goalkeeper's goal at %v, want %v",
					name, p.ID, p.Rules.Goal[1], want.Goal[1])
			}
		}
	}
}

// TestTheArchivesOnlyGoalkeeperGoalIsPricedUnderItsOwnSeasonsRules is the
// regression test for the per-season pin, taken end to end on real rows.
//
// # The trap
//
// The instrument used to read `analysis.goalPoints` directly, and that table is
// asserted against FPL's **current** published `game_config` by
// `TestScoringConstantsMatchFPL` — which is what keeps it honest and also what
// would have carried the next rule change backwards over the whole archive. That
// is the failure `BankLimitFor` and `DefconScoredIn` exist to prevent for the
// transfer bank and for defensive contribution.
//
// # Why this row and no other
//
// A goalkeeper's goal is the change that has already happened: 10 today, 6 in
// 2020-21. **Alisson, 2020-21 GW36 is the only goalkeeper goal in the archive**,
// and it is what makes the 6 a measurement rather than an assertion — his row
// reconstructs to exactly 6 with every other channel accounted for (90 minutes,
// no clean sheet, one conceded, two saves, two bonus, `total_points` 10).
//
// So this asserts the goal channel through the *replay's own* mapping,
// `xPointsOf`, differenced against the same row with the goal removed. Everything
// else — the appearance, the bonus, the conversion scale, the clean-sheet and
// concede channels — is common to the two and cancels exactly, so what is left is
// the price of the goal and nothing else.
//
// It fails if the per-season pin is removed, if `repaired()` stops resolving the
// table, or if the boundary is moved to the far side of 2020-21.
func TestTheArchivesOnlyGoalkeeperGoalIsPricedUnderItsOwnSeasonsRules(t *testing.T) {
	modern := analysis.ScoringRulesFor("2025-26").Goal[1]
	if modern == analysis.ScoringRulesFor("2020-21").Goal[1] {
		t.Fatalf("2020-21 and 2025-26 price a goalkeeper's goal identically (%v), "+
			"so this test cannot distinguish a per-season table from today's one",
			modern)
	}

	cc := loadConfig(t)
	s, err := Load(context.Background(), cc.CacheDir, "2020-21")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	found := 0
	for _, p := range s.Players {
		if p.Type != 1 {
			continue
		}
		for gw := 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.Goals == 0 {
				continue
			}
			found++
			bare := g
			bare.Goals = 0
			// xPoints is Points minus the residual and Points is identical in the
			// two rows, so this difference is exactly Goals * the goal's price.
			priced := (xPointsOf(p, bare) - xPointsOf(p, g)) / float64(g.Goals)
			if math.Abs(priced-6) > 1e-9 {
				t.Errorf("%s GW%d: the instrument prices his goal at %v. FPL paid 6 "+
					"in 2020-21 — decoded from this very row — and %v is today's "+
					"value, so the archive is being re-scored under a rule nobody "+
					"played under", p.WebName, gw, priced, modern)
			}
		}
	}
	// ⚠️ The fixture has to exist. If the archive ever stops carrying this row the
	// test would pass by measuring nothing, which is the vacuous-pass shape this
	// package keeps paying for.
	if found == 0 {
		t.Fatal("2020-21 carries no goalkeeper goal, so nothing above was checked. " +
			"Alisson GW36 is the archive's only one and it is what makes the 6 a " +
			"measurement — find where it went before relaxing this")
	}
}

// TestTheArchiveHoldsAPositionTheInstrumentCannotPrice is the liveness half of
// the unknown-position guard, and it is the half with power.
//
// The guard itself is a **code** fact: `XPointsResidual` refuses an element_type
// its rules have no entry for, and `internal/analysis` pins that on a
// constructed row. Re-asserting it here could only pass. What is worth checking
// is the claim the guard rests on — that the population is REAL and that its
// unreachability is a property of a caller rather than of the archive.
//
// Both halves are checked, because they are the two ways this becomes wrong:
//
//   - the archive really does load players the instrument has no table for. FPL
//     ran assistant managers as `element_type` 5 for 2024-25 — **322 archive
//     rows accumulating to 312 player-gameweeks and carrying 1,861 points**,
//     loaded into `Season.Players` like anyone else, and every one of them
//     recording zero minutes. Read through a bare map index their goals channel
//     prices at zero.
//   - and nothing can put one in a squad, because `PointInTime` publishes
//     element_types 1-4 only, so a manager resolves to an unknown position short
//     name, and no squad quota answers to that. If that ever changes the guard
//     stops being a tripwire and starts being a crash, which is a decision
//     somebody should take deliberately.
func TestTheArchiveHoldsAPositionTheInstrumentCannotPrice(t *testing.T) {
	cc := loadConfig(t)
	// 2024-25 is the season FPL ran managers in, and the only one.
	s, err := Load(context.Background(), cc.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	unpriceable := 0
	for _, p := range s.Players {
		if !p.Rules.Prices(p.Type) {
			unpriceable++
		}
	}
	// ⚠️ A FAILURE, not a skip. The whole justification for placing the guard
	// before the `Minutes <= 0` return is that this population exists and records
	// no minutes. A skip here would take that evidence away while the suite went
	// green — "a guard that skips when it cannot find its subject is a guard
	// nobody notices losing", which this repository's own xPoints script guard
	// says in as many words.
	if unpriceable == 0 {
		t.Fatal("2024-25 loaded no player of an unpriced element_type. That is the " +
			"population the unknown-position guard exists for and the reason it " +
			"sits before the blank-gameweek return — if the loader now filters " +
			"element_type 5, say so there rather than letting this go quiet")
	}
	t.Logf("2024-25 loads %d players the instrument has no scoring table for", unpriceable)

	// The unreachability, which is what makes the guard a tripwire rather than a
	// live crash. A position the bootstrap does not publish has no short name, and
	// squadQuota answers zero for it, so no legal squad can hold one.
	boot, fixtures := PointInTime(s, &Season{Name: "2023-24", Players: map[int]*Player{}}, 5)
	if boot == nil {
		t.Fatalf("PointInTime returned no bootstrap (%d fixtures); the position "+
			"mapping this test reads is unavailable", len(fixtures))
	}
	playable := map[string]bool{"GKP": true, "DEF": true, "MID": true, "FWD": true}
	for _, p := range s.Players {
		if p.Rules.Prices(p.Type) {
			continue
		}
		if short := boot.PositionShort(p.Type); playable[short] {
			t.Fatalf("element_type %d resolves to position %q, which the optimiser "+
				"holds a squad quota for. An unpriced position can now reach a "+
				"squad, so XPointsResidual's refusal is a live crash rather than a "+
				"tripwire — give it a scoring table or keep it out of the pool",
				p.Type, short)
		}
	}
	// And the mirror, so the check above is not passing because the bootstrap
	// publishes nothing at all: the four real positions must still resolve.
	for _, et := range []int{1, 2, 3, 4} {
		if short := boot.PositionShort(et); !playable[short] {
			t.Fatalf("element_type %d resolves to %q; the bootstrap this test reads "+
				"positions from is not publishing the real ones, so the assertion "+
				"above is vacuous", et, short)
		}
	}
}
