package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// TestOptimizeEscapesAClubConstrainedDeadEnd is the reported case as a
// deterministic fixture: no archive, no live API, built on scaleEngine
// (enginescale_test.go) so it runs in milliseconds and cannot rot with the
// season.
//
// The shape mirrors the real failing trace from 2021-22 GW18 (see
// freehitblank_test.go and this fix's package doc on minCostToFill/
// fillBound): forwards are scarce and concentrated in a handful of clubs,
// and those same clubs' other positions are the cheapest, most attractive
// picks in the whole pool. A club-blind bound lets the greedy spend those
// clubs' ENTIRE 3-player cap on non-forwards before it ever looks at a
// forward — sealing off the only clubs that HAVE one, even though a legal
// fifteen exists (it just has to leave room). Eight clubs, nine forwards
// (three clubs supplying three each), ~130 players, £82.0m — the same
// budget and the same "found none" error text the real trace produced.
//
// Model note: roster shape follows benchPool/benchSquad
// (optimizerbench_test.go), which already builds a club-cap-binding
// synthetic pool; this fixture goes further and makes the CLUB cap the
// thing standing between "legal" and "the search gives up", rather than
// merely exercising it in passing.
func TestOptimizeEscapesAClubConstrainedDeadEnd(t *testing.T) {
	const numClubs = 8
	// Clubs 1-3 are the trap: they hold every forward in the pool, three
	// each, and their own GKP/DEF/MID are priced far below anyone else's —
	// the cheapest, highest-value-score picks in the whole pool.
	trapClubs := map[int]bool{1: true, 2: true, 3: true}

	var els []fpl.Element
	id := 1
	next := func() int { id++; return id - 1 }

	addPlayer := func(team, elementType int, price int, goals, assists int, xg, xa float64) {
		els = append(els, fpl.Element{
			ID: next(), Team: team, ElementType: elementType, NowCost: price,
			Minutes: 3000, Starts: 33, Status: "a",
			GoalsScored: goals, ExpectedGoals: fpl.Num(xg),
			Assists: assists, ExpectedAssists: fpl.Num(xa),
			CleanSheets: 8, GoalsConceded: 40,
		})
	}

	for club := 1; club <= numClubs; club++ {
		if trapClubs[club] {
			// Cheap, uniformly decent GKP/DEF/MID — the bait.
			addPlayer(club, 1, 39, 0, 0, 0, 0) // GKP £3.9m
			addPlayer(club, 1, 40, 0, 0, 0, 0) // GKP £4.0m
			for i := 0; i < 5; i++ {
				addPlayer(club, 2, 40+i, 1, 2, 2, 3) // DEF £4.0-4.4m
			}
			for i := 0; i < 5; i++ {
				addPlayer(club, 3, 41+i, 3, 4, 4, 5) // MID £4.1-4.5m
			}
			// The only forwards in the pool: three per trap club, priced
			// well above the club's own bait but still affordable — the
			// point is that they are the WORST value in the pool, not that
			// they are unaffordable, so a value-ordered greedy reaches for
			// everything else first.
			for i := 0; i < 3; i++ {
				addPlayer(club, 4, 55+i*5, 8, 3, 9, 4) // FWD £5.5/6.0/6.5m
			}
			continue
		}
		// The other five clubs: legal fallback candidates, priced and scored
		// so they are the SECOND choice by value, never the first — enough
		// of them, at every non-forward position, to complete a fifteen once
		// the trap clubs are correctly rationed.
		addPlayer(club, 1, 45, 0, 0, 0, 0)
		addPlayer(club, 1, 47, 0, 0, 0, 0)
		for i := 0; i < 5; i++ {
			addPlayer(club, 2, 50+i, 1, 1, 1, 2)
		}
		for i := 0; i < 5; i++ {
			addPlayer(club, 3, 55+i, 2, 2, 3, 3)
		}
	}
	t.Logf("fixture: %d players, %d clubs, 9 forwards (all in clubs 1-3)", len(els), numClubs)

	e := scaleEngine(t, els...)
	sq, err := e.Optimize(OptimizeRequest{Budget: 820, MinMinutes: 0, MinExpectedMinutes: 0})
	if err != nil {
		t.Fatalf("Optimize could not fill a legal fifteen at £82.0m: %v — a legal completion "+
			"exists (it just requires not spending every trap club's whole cap on "+
			"non-forwards), so this is the club-blind bound dead-end the fix removes", err)
	}

	if len(sq.Players) != SquadSize {
		t.Fatalf("squad has %d players, want %d", len(sq.Players), SquadSize)
	}
	pos := map[string]int{}
	for _, p := range sq.Players {
		pos[p.Position]++
	}
	for want, n := range squadQuota {
		if pos[want] != n {
			t.Errorf("position %s = %d, want %d", want, pos[want], n)
		}
	}
	for club, n := range sq.ClubCounts {
		if n > MaxPerClub {
			t.Errorf("club %s has %d players, limit is %d", club, n, MaxPerClub)
		}
	}
	if sq.TotalCost > 82.0 {
		t.Errorf("total cost £%.1fm exceeds the £82.0m budget", sq.TotalCost)
	}
	if pos["FWD"] != 3 {
		t.Fatalf("squad has %d forwards, want 3 — the only clubs holding a forward are "+
			"1-3, so this number is the direct check that the trap did not seal them off",
			pos["FWD"])
	}
	t.Logf("formation %s, cost £%.1fm, club counts %v", sq.Formation, sq.TotalCost, sq.ClubCounts)
}
