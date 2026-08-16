package backtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkLateSignee adds a player who joins the game partway through the season: he
// has a full players_raw record and gameweek rows only from `from` onward, which
// is exactly the shape a January signing has in the archive.
//
// He is put on an *existing* club deliberately. A club with no rows at all by the
// cutoff triggers the blind-club guard in registeredBy and is kept, so a late
// signee given his own club would silently pass every test below while the leak
// stayed open — which is the failure TestABlindClubIsKept pins from the other side.
func mkLateSignee(s *Season, id, from, entryPrice, closingPrice int) *Player {
	p := &Player{ID: id, Code: 1000 + id, Type: 3, Team: 1, WebName: "Late",
		NowCost: closingPrice, GWs: map[int]GW{}}
	price := entryPrice
	for gw := from; gw <= 38; gw++ {
		if gw == 38 {
			price = closingPrice
		}
		p.GWs[gw] = GW{Points: 5, Minutes: 90, Value: price}
		p.TotalPoints += 5
		p.Minutes += 90
	}
	s.Players[id] = p
	return p
}

// TestUnregisteredPlayersAreNotInThePool is the point-in-time test for *who could
// be bought*, alongside TestPointInTimeCannotSeeTheFuture for *what was known*.
//
// The replay iterated players_raw.csv, which lists everyone registered at any
// point in the season, and priced anyone with no gameweek row at the closing
// price. So a January signing sat in the GW1 pool at what he was worth in May.
// Measured on 2025-26: 151 of 841 players, four of them in the replayed opening
// squad, two returning 168 points between them.
func TestUnregisteredPlayersAreNotInThePool(t *testing.T) {
	cur, prior := mkSeason(), mkSeason()
	mkLateSignee(cur, 99, 11, 45, 42)

	inPool := func(through int) bool {
		boot, _ := PointInTime(cur, prior, through)
		for _, el := range boot.Elements {
			if el.ID == 99 {
				return true
			}
		}
		return false
	}
	for _, through := range []int{0, 1, 5, 10} {
		if inPool(through) {
			t.Errorf("a player whose first gameweek row is GW11 is in the pool at "+
				"GW%d; he was not in the game and had no price", through)
		}
	}
	for _, through := range []int{11, 20, 38} {
		if !inPool(through) {
			t.Errorf("a player whose first gameweek row is GW11 is missing from the "+
				"pool at GW%d, where he really was buyable", through)
		}
	}
}

// TestThePoolIsNeverPricedFromTheFuture pins the two halves of the leak together:
// membership and price. Either alone would have looked repaired.
func TestThePoolIsNeverPricedFromTheFuture(t *testing.T) {
	cur, prior := mkSeason(), mkSeason()
	// A riser: opens at 45, ends at 60. Buying him at 45 in GW20 would be the
	// price half of the same bug.
	r := cur.Players[8]
	for gw := 1; gw <= 38; gw++ {
		v := 45
		if gw >= 20 {
			v = 60
		}
		g := r.GWs[gw]
		g.Value = v
		r.GWs[gw] = g
	}
	r.NowCost = 60

	for _, through := range []int{0, 1, 19, 20, 38} {
		boot, _ := PointInTime(cur, prior, through)
		for _, el := range boot.Elements {
			if el.ID != 8 {
				continue
			}
			want := 45
			if through >= 20 {
				want = 60
			}
			if el.NowCost != want {
				t.Errorf("at GW%d the pool prices him at %d, want %d", through, el.NowCost, want)
			}
		}
	}
}

// TestPriceAtNeverFallsBackToTheClosingPrice pins the single most important
// property of the one price implementation.
//
// Zero is the honest answer for a player with no row at or before the cutoff, and
// it is what makes the leak *visible*: a squad containing an impossible player
// fails loudly instead of quietly costing a tenth. The tempting alternative,
// `now_cost - cost_change_start`, returns a plausible legal-looking price and is
// the subject of TestDiagUnregisteredPool's assertion — never of a fallback.
func TestPriceAtNeverFallsBackToTheClosingPrice(t *testing.T) {
	p := &Player{NowCost: 42, GWs: map[int]GW{
		11: {Value: 45}, 12: {Value: 44}, 38: {Value: 42},
	}}
	for _, through := range []int{0, 1, 5, 10} {
		if got := priceAt(p, through); got != 0 {
			t.Errorf("priceAt(GW%d) = %d before his first row, want 0; a non-zero "+
				"answer here is the closing price leaking backwards", through, got)
		}
	}
	for through, want := range map[int]int{11: 45, 12: 44, 20: 44, 38: 42} {
		if got := priceAt(p, through); got != want {
			t.Errorf("priceAt(GW%d) = %d, want %d", through, got, want)
		}
	}
}

// TestLastValueKeepsItsClosingPriceFallback pins the deliberate exception.
//
// pool.go's rule is that nothing falls back to Player.NowCost, because everywhere
// else that is the closing price standing in for an earlier one. lastValue asks
// for the closing price, so NowCost is the exact answer rather than a fallback. A
// well-meaning sweep of "remove the NowCost fallbacks" would break the value-change
// column and nothing would say so.
func TestLastValueKeepsItsClosingPriceFallback(t *testing.T) {
	if got := lastValue(&Player{NowCost: 55, GWs: map[int]GW{}}); got != 55 {
		t.Errorf("lastValue with no rows = %d, want the closing price 55", got)
	}
	p := &Player{NowCost: 55, GWs: map[int]GW{1: {Value: 50}, 20: {Value: 53}}}
	if got := lastValue(p); got != 53 {
		t.Errorf("lastValue = %d, want the latest priced row 53", got)
	}
}

// TestABlindClubIsKept pins the guard that stops the fix becoming a much larger
// distortion than the leak.
//
// A club with no rows at all by the cutoff has said nothing about its players, and
// dropping its whole squad would remove five defenders and a keeper from the pool.
// It does not arise in the shipped grid — all 20 clubs have GW1 rows in all four
// seasons — but 2022-23 GW7 was cancelled outright and has no rows for anybody, so
// the case is real rather than hypothetical.
func TestABlindClubIsKept(t *testing.T) {
	s := mkSeason()
	// A whole club that blanks the opening gameweek: rows only from GW2.
	for id := 90; id < 93; id++ {
		p := &Player{ID: id, Code: 1000 + id, Type: 2, Team: 99,
			WebName: "Blanked", NowCost: 45, GWs: map[int]GW{}}
		for gw := 2; gw <= 38; gw++ {
			p.GWs[gw] = GW{Points: 3, Minutes: 90, Value: 45}
		}
		s.Players[id] = p
	}
	reg := registeredBy(s, 0)
	for id := 90; id < 93; id++ {
		if !reg.has(id) {
			t.Errorf("player %d is excluded although his whole club has no GW1 rows; "+
				"the archive said nothing about him and the guard should keep him", id)
		}
	}
	if len(reg.blind) != 1 || reg.blind[0] != 99 {
		t.Errorf("blind clubs = %v, want [99] so the condition is reported rather "+
			"than silently re-opening the leak for that club", reg.blind)
	}
	// And he must be *priced*. priceAt honestly returns 0 for him — he has no row
	// at or before the cutoff — and a pool member costing nothing is a worse bug
	// than the leak, because the optimiser would buy fifteen of him.
	for id := 90; id < 93; id++ {
		if got := reg.price(s.Players[id], 0); got != 45 {
			t.Errorf("player %d from a blind club is priced at %d, want his earliest "+
				"row's 45; zero would put a free player in the pool", id, got)
		}
	}
	// And the guard must not fire when the club simply has a late signing — nor
	// price him, which is the whole point of keeping the two answers together.
	late := mkSeasonWithLateSignee()
	lateReg := registeredBy(late, 0)
	if len(lateReg.blind) != 0 {
		t.Errorf("blind clubs = %v on a season whose clubs all played GW1, want none",
			lateReg.blind)
	}
	if got := lateReg.price(late.Players[99], 0); got != 0 {
		t.Errorf("a late signee on a club that did play GW1 is priced at %d, want 0; "+
			"the forward walk is for blind clubs only and must not rescue him", got)
	}
}

func mkSeasonWithLateSignee() *Season {
	s := mkSeason()
	mkLateSignee(s, 99, 11, 45, 42)
	return s
}

// TestOpeningPriceIsDeclaredOnce counts the copies of "what did this player cost",
// the way TestTheGridIsDeclaredOnce counts copies of the replay grid.
//
// There were five — openingPrice, priceAt, and inline blocks in PreSeason, Score
// and RandomSquads — and all five ended in the same fallback to the closing price.
// Four of them were correct-looking three-liners; the divergence was in the one
// nobody diffed. Source-scanning for the same reason the other structural guards
// do: the failure is a *new* call site pasting the idiom back in, and it agrees
// with the original on the day it is written.
func TestOpeningPriceIsDeclaredOnce(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// Fragments, so this file does not match its own scanner.
	inlineLookup := "GWs[1]; ok" + " && g.Value"
	closingFallback := "return p." + "NowCost"
	home := "pool.go"

	var offenders []string
	for _, f := range files {
		base := filepath.Base(f)
		if base == home || base == "pool_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if strings.Contains(src, inlineLookup) {
			offenders = append(offenders, base+" re-derives the opening price inline")
		}
		if strings.Contains(src, closingFallback) {
			offenders = append(offenders, base+" falls back to the closing price")
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("priceAt in %s is the only implementation of what a player cost; "+
			"found %v.\nA price at gameweek N must come from a row at or before N, "+
			"and zero when there is none — falling back to Player.NowCost is the "+
			"leak that put 151 unregistered players in the 2025-26 GW1 pool.",
			home, offenders)
	}
}
