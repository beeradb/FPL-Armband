package backtest

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"testing"

	"armband/internal/config"
)

// TestDiagUnregisteredPool is the evidence behind pool.go, re-derivable rather
// than frozen prose.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagUnregisteredPool -v
//
// It reports three things and asserts two of them:
//
//   - **How many players the leak admitted**, per season, and how many of them
//     were priced below the season's own minimum GW1 price — a price FPL never
//     charged, and the canary that found the bug.
//   - **The 2025-26 opening squad**, with the unregistered members named, in both
//     arms. Named rather than counted: "four unregistered players" is a number
//     anybody can dismiss, and "Mané, who did not enter the game until GW11,
//     bought in GW1 at £4.2m when he never cost less than £4.5m" is not.
//   - **The `now_cost - cost_change_start` assertion.** This reproduces the GW1
//     price exactly for registered players and is the obvious fallback. It is
//     asserted here and used nowhere, because for an *unregistered* player it
//     returns a legal-looking price measured from his own entry — which would
//     have repaired every symptom and hidden the bug. Asserting a thing and
//     depending on it are different, and this is the line between them.
//
// It needs the archive's raw CSV for the third item, and skips when unreachable.
func TestDiagUnregisteredPool(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	fmt.Printf("\n%-9s %7s %13s %8s %10s %8s\n",
		"season", "players", "unregistered", "share", "min GW1", "below it")
	for _, pair := range sweepPairNames() {
		cur := loadSeason(t, cfg, pair[1])
		reg := registeredBy(cur, 0)
		minGW1 := 0
		for _, p := range cur.Players {
			if v := priceAt(p, 1); v > 0 && (minGW1 == 0 || v < minGW1) {
				minGW1 = v
			}
		}
		var unreg, cheap int
		for _, p := range cur.Players {
			if reg.has(p.ID) {
				continue
			}
			unreg++
			if p.NowCost < minGW1 {
				cheap++
			}
		}
		fmt.Printf("%-9s %7d %13d %7.0f%% %10.1f %8d\n", pair[1], len(cur.Players),
			unreg, 100*float64(unreg)/float64(len(cur.Players)),
			float64(minGW1)/10, cheap)
		if len(reg.blind) > 0 {
			t.Errorf("%s has clubs with no GW1 rows %v; the blind-club guard is "+
				"holding the leak open for them and the count above understates it",
				pair[1], reg.blind)
		}
		assertPoolIsHonest(t, pair, cfg, minGW1)
	}

	// The opening squad, both arms. Run twice per arm: the optimiser is not
	// deterministic (TestDiagOptimizerIsNotDeterministic), so a single fifteen is
	// not evidence of anything on its own.
	var cur, prior *Season
	for _, p := range loadPairs(t, cfg) {
		if p.Name == "2025-26" {
			cur, prior = p.Cur, p.Prior
		}
	}
	// Taken once, with the leak off, and reused by both arms. Computing it inside
	// the leak-restored arm is a trap this diagnostic fell into on its first run:
	// registeredBy answers "everyone" under the hatch, so the column that counts
	// the leak's victims read zero in the very arm that has them.
	os.Unsetenv("FPL_UNREGISTERED_POOL")
	truth := registeredBy(cur, 0)

	fmt.Printf("\n2025-26 opening squad, GW1 entry\n")
	defer os.Unsetenv("FPL_UNREGISTERED_POOL")
	for _, arm := range []struct {
		label string
		leak  bool
	}{{"leak restored", true}, {"corrected (ships)", false}} {
		if arm.leak {
			os.Setenv("FPL_UNREGISTERED_POOL", "1")
		} else {
			os.Unsetenv("FPL_UNREGISTERED_POOL")
		}
		for run := 0; run < 2; run++ {
			res, err := Simulate(cur, prior, seasonConfig(cfg, "2025-26", 1, false))
			if err != nil {
				t.Fatal(err)
			}
			reg := truth
			var bad []string
			for _, id := range res.OpeningSquad {
				p := cur.Players[id]
				if reg.has(id) {
					continue
				}
				first := 0
				for gw := 1; gw <= 38; gw++ {
					if _, ok := p.GWs[gw]; ok {
						first = gw
						break
					}
				}
				bad = append(bad, fmt.Sprintf("%s (entered GW%d at %.1f, bought at %.1f, %d pts)",
					p.WebName, first, float64(p.GWs[first].Value)/10,
					float64(p.NowCost)/10, p.TotalPoints))
			}
			sort.Strings(bad)
			// Bench cover is the mechanism worth counting. A player with no
			// gameweek row records no minutes, so he cannot be autosubbed in —
			// he is not a weak substitute, he is an absent one. Count how many
			// of the fifteen could record a minute in the opening weeks, which
			// is exactly when a squad needs cover and has no transfers to fix it.
			cover := func(gw int) int {
				n := 0
				for _, id := range res.OpeningSquad {
					if g, ok := cur.Players[id].GWs[gw]; ok && g.Minutes > 0 {
						n++
					}
				}
				return n
			}
			dead := 0
			for _, id := range res.OpeningSquad {
				if cur.Players[id].Minutes == 0 {
					dead++
				}
			}
			fmt.Printf("  %-18s run %d: points %d, value %.1f, played GW1 %d/15, "+
				"GW1-5 mean %.1f/15, never played %d, unregistered %d %v\n",
				arm.label, run, res.Points, float64(res.StartValue)/10, cover(1),
				float64(cover(1)+cover(2)+cover(3)+cover(4)+cover(5))/5, dead,
				len(bad), bad)
		}
	}
	os.Unsetenv("FPL_UNREGISTERED_POOL")

	// The assertion, on the fallback that must never be a fallback.
	assertCostChangeStart(t, "2025-26", cur, truth)
}

// assertPoolIsHonest checks the three invariants the fix is really about, on the
// real archive and at several cutoffs: at every cutoff that no pool member lacks a
// gameweek row at or before it and that none is priced at or below zero, and at GW1
// that none is priced below the season's own minimum GW1 price. The zero-price
// check is what pins the blind-club forward walk in registration.price.
//
// **These are the assertions to trust, and a replayed points total is not one.**
// `Optimize` is not deterministic (TestDiagOptimizerIsNotDeterministic), so a
// squad or a season total that reproduces may have reproduced by luck, and the
// 2025-26 GW1 cell moved by 20 points between two processes running identical
// code. Pool membership and pool pricing are exact set operations over a frozen
// archive: they reproduce or the fix is wrong, with no noise floor in between.
func assertPoolIsHonest(t *testing.T, pair [2]string, cfg config.Config, minGW1 int) {
	t.Helper()
	prior := loadSeason(t, cfg, pair[0])
	cur := loadSeason(t, cfg, pair[1])
	for _, through := range []int{0, 1, 6, 11, 26, 38} {
		boot, _ := PointInTime(cur, prior, through)
		cutoff := through
		if cutoff < 1 {
			cutoff = 1
		}
		var late, free, cheap int
		for _, el := range boot.Elements {
			p := cur.Players[el.ID]
			seen := false
			for gw := 1; gw <= cutoff; gw++ {
				if _, ok := p.GWs[gw]; ok {
					seen = true
					break
				}
			}
			if !seen {
				late++
			}
			if el.NowCost <= 0 {
				free++
			}
			// FPL has never charged less than the season's own opening floor, so
			// a pool member below it is a price that did not exist. This is the
			// check that caught the bug: £3.9m players in the GW1 squad.
			if cutoff == 1 && el.NowCost < minGW1 {
				cheap++
			}
		}
		if late > 0 {
			t.Errorf("%s at GW%d: %d pool members have no gameweek row at or before "+
				"GW%d — they were not in the game and could not be bought",
				pair[1], through, late, cutoff)
		}
		if free > 0 {
			t.Errorf("%s at GW%d: %d pool members are priced at zero or less",
				pair[1], through, free)
		}
		if cheap > 0 {
			t.Errorf("%s at GW%d: %d pool members are priced below the season's own "+
				"minimum GW1 price of %.1f", pair[1], through, cheap, float64(minGW1)/10)
		}
	}
}

// assertCostChangeStart checks that now_cost - cost_change_start reproduces the
// GW1 price for every registered player, and reports what it does for the
// unregistered ones.
//
// The first half is why it looks like the right fallback. The second half is why
// it is not: it measures from the player's *own* entry to the game, so it returns
// a 0.5m-aligned, above-minimum, entirely plausible price for somebody who could
// not be bought at all. Using it would have made every squad look legal while the
// replay went on picking players who did not exist.
func assertCostChangeStart(t *testing.T, season string, cur *Season, reg registration) {
	t.Helper()
	r, c, col, err := rows(context.Background(), season, "players_raw.csv")
	if err != nil {
		t.Skipf("players_raw.csv unreachable, skipping the assertion: %v", err)
	}
	defer c.Close()
	if _, ok := col["cost_change_start"]; !ok {
		t.Skipf("%s players_raw.csv carries no cost_change_start column", season)
	}

	var agree, disagree, unregSensible, unregTotal int
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("players_raw.csv: %v", err)
		}
		id := ival(rec, col, "id")
		p := cur.Players[id]
		if p == nil {
			continue
		}
		entry := ival(rec, col, "now_cost") - ival(rec, col, "cost_change_start")
		if reg.has(id) {
			if entry == priceAt(p, 1) {
				agree++
			} else {
				disagree++
			}
			continue
		}
		unregTotal++
		// "Sensible" is the trap, stated as a predicate: at or above the £4.0m
		// floor — so it passes the check that caught the real bug — and equal to
		// his price on the day he actually entered the game, which is what makes
		// it look like a considered answer rather than a guess.
		if entry >= 40 && entry == priceAt(p, firstRow(p)) {
			unregSensible++
		}
	}
	if disagree > 0 {
		t.Errorf("now_cost - cost_change_start disagrees with the GW1 price for %d of "+
			"%d registered players; the identity this assertion rests on has changed",
			disagree, agree+disagree)
	}
	fmt.Printf("\nnow_cost - cost_change_start, %s:\n"+
		"  registered:   %d of %d reproduce the GW1 price exactly\n"+
		"  unregistered: %d of %d return their own *entry* price — legal-looking,\n"+
		"                above the floor, and not a price anyone could have paid in GW1.\n"+
		"                This is why it is an assertion and never a fallback.\n",
		season, agree, agree+disagree, unregSensible, unregTotal)
}

// firstRow is the first gameweek the archive shows a player in, or 0.
func firstRow(p *Player) int {
	for gw := 1; gw <= 38; gw++ {
		if _, ok := p.GWs[gw]; ok {
			return gw
		}
	}
	return 0
}

// TestDiagUnregisteredPoolImpact is what the leak was worth, paired across the
// full grid.
//
//	DIAG=1 FPL_CELLS=/tmp/pool/cells.csv \
//	  go test ./internal/backtest -run TestDiagUnregisteredPoolImpact -count=1 -v -timeout 180m
//
// A single cell said the corrected replay scored *more*, which is the opposite of
// what the mechanism predicts — the leak granted free budget and free hindsight,
// so removing it should cost points on paper. One cell of a non-deterministic
// replay is not evidence either way, and this is the measurement that can say.
//
// Read HOLD as primary. This is a change to the *pool*, so it is a squad-selection
// change and belongs on the metric that excludes the transfer path; POLICY is
// reported because the pool also feeds the weekly transfer search, so unlike a
// pure scoring knob it is not expected to be byte-identical.
func TestDiagUnregisteredPoolImpact(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	runPolicySweep(t, []policyVariant{
		{label: "corrected (ships)", apply: func(sc *SimConfig) {
			os.Unsetenv("FPL_UNREGISTERED_POOL")
			sc.WeeklyXI = true
		}},
		{label: "leak restored", apply: func(sc *SimConfig) {
			os.Setenv("FPL_UNREGISTERED_POOL", "1")
			sc.WeeklyXI = true
		}},
	}, sweepStarts())
	os.Unsetenv("FPL_UNREGISTERED_POOL")
}
