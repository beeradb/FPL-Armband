package analysis

import (
	"fmt"
	"math"
	"testing"
)

// TestSquadXIValueIgnoresTheBench — the objective a transfer is judged on must
// count only the eleven that scores points, or the policy spends its transfers
// improving reserves. Replacing the worst bench player with someone still worse
// than the eleven must move nothing.
func TestSquadXIValueIgnoresTheBench(t *testing.T) {
	squad := make([]PlayerMetrics, 0, 15)
	add := func(pos string, score float64) {
		squad = append(squad, PlayerMetrics{
			ID: len(squad) + 1, Position: pos, Score: score,
		})
	}
	add("GKP", 5)
	add("GKP", 1)
	for i := 0; i < 5; i++ {
		add("DEF", float64(5-i))
	}
	for i := 0; i < 5; i++ {
		add("MID", float64(6-i))
	}
	for i := 0; i < 3; i++ {
		add("FWD", float64(6-i))
	}
	base := XIValue(squad)

	// The reserve keeper goes from 1.0 to 2.0 — still far below the first
	// choice, so the eleven is unchanged.
	squad[1].Score = 2
	if got := XIValue(squad); got != base {
		t.Errorf("upgrading a reserve moved the objective from %.2f to %.2f", base, got)
	}

	// Upgrading the best player moves it by the gain twice, since the captain
	// is counted a second time.
	squad[1].Score = 1
	best := 0
	for i, p := range squad {
		if p.Score > squad[best].Score {
			best = i
		}
	}
	squad[best].Score += 1
	if got := XIValue(squad); got != base+2 {
		t.Errorf("upgrading the captain by 1 moved the objective to %.2f, want %.2f",
			got, base+2)
	}
}

// TestPriceFrontierKeepsOnlyTheReachableBest — a player outscored by someone at
// or below his price can never be the right answer, and keeping him makes the
// paired search intractable.
func TestPriceFrontierKeepsOnlyTheReachableBest(t *testing.T) {
	// The frontier must be ascending in both price and score.
	ms := []PlayerMetrics{
		{ID: 1, Position: "MID", Price: 4.0, Score: 2.0},
		{ID: 2, Position: "MID", Price: 5.0, Score: 1.0}, // dominated
		{ID: 3, Position: "MID", Price: 6.0, Score: 3.0},
		{ID: 4, Position: "MID", Price: 7.0, Score: 2.5}, // dominated
		{ID: 5, Position: "MID", Price: 8.0, Score: 4.0},
	}
	got := frontierOf(ms)
	want := []int{1, 3, 5}
	if len(got) != len(want) {
		t.Fatalf("frontier has %d players, want %d: %+v", len(got), len(want), got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("frontier[%d] is player %d, want %d", i, got[i].ID, id)
		}
	}
}

// mkSquad builds a legal fifteen where every player scores `base` for his
// position, so a test can perturb exactly one thing.
func mkSwapSquad() []PlayerMetrics {
	var sq []PlayerMetrics
	add := func(pos string, n int, price, score float64) {
		for i := 0; i < n; i++ {
			sq = append(sq, PlayerMetrics{
				ID: len(sq) + 1, Name: fmt.Sprintf("%s%d", pos, i),
				Position: pos, Team: fmt.Sprintf("T%d", len(sq)+1),
				Price: price, Score: score,
			})
		}
	}
	add("GKP", 2, 5.0, 3.0)
	add("DEF", 5, 5.0, 3.0)
	add("MID", 5, 7.0, 4.0)
	add("FWD", 3, 7.0, 4.0)
	return sq
}

// TestSearchPairsFundsAPremiumNoSingleSwapCanReach is the point of the whole
// function.
//
// A squad with no spare money cannot buy an expensive player one swap at a time:
// the downgrade that funds him lowers the eleven on its own and is rejected
// before the upgrade is ever considered. Replaying 2025-26, that is why Haaland
// — the season's highest scorer — sat between £3.4m and £7.4m out of reach in
// every one of the 37 weeks the policy ran.
func TestSearchPairsFundsAPremiumNoSingleSwapCanReach(t *testing.T) {
	sq := mkSwapSquad()
	// A £9.0m striker worth far more than the £7.0m he replaces, funded by
	// dropping a £5.0m defender to £3.0m. The striker is £2.0m out of reach on
	// his own with an empty bank, and the defender is a loss on his own, so
	// only the pair works.
	pool := []PlayerMetrics{
		{ID: 100, Name: "Premium", Position: "FWD", Team: "TX", Price: 9.0, Score: 9.0},
		{ID: 101, Name: "Cheap", Position: "DEF", Team: "TY", Price: 3.0, Score: 2.6},
	}

	got, ok := firstPair(sq, pool, 0, 1)
	if !ok {
		t.Fatal("no pair found; the premium is unreachable and should have been funded")
	}
	if got.Up.In.Name != "Premium" {
		t.Errorf("upgrade bought %q, want Premium", got.Up.In.Name)
	}
	if got.Downs[0].In.Name != "Cheap" {
		t.Errorf("funding leg bought %q, want Cheap", got.Downs[0].In.Name)
	}
	// +5.0 on the striker, counted twice because he becomes captain. The
	// downgraded defender drops out of the eleven, so he costs nothing.
	if got.Gain != 10 {
		t.Errorf("combined gain %.2f, want 10", got.Gain)
	}
}

// TestSearchPairsRespectsTheBudget — the pair must be affordable together, not
// merely individually.
func TestSearchPairsRespectsTheBudget(t *testing.T) {
	sq := mkSwapSquad()
	// The funding leg frees only £1.0m against a £5.0m shortfall.
	pool := []PlayerMetrics{
		{ID: 100, Name: "Premium", Position: "FWD", Team: "TX", Price: 12.0, Score: 9.0},
		{ID: 101, Name: "Cheap", Position: "DEF", Team: "TY", Price: 4.0, Score: 2.9},
	}
	if _, ok := firstPair(sq, pool, 0, 1); ok {
		t.Error("funded a premium the squad could not afford")
	}
}

// TestSearchPairsHoldsTheClubLimit — three per club, counted across both legs.
func TestSearchPairsHoldsTheClubLimit(t *testing.T) {
	sq := mkSwapSquad()
	// Three of the squad already play for the premium's club. They are the two
	// keepers and a midfielder, so neither leg of the pair can sell one of them
	// — selling a TX player to buy a TX player would keep the count legal, and
	// this test is about the case where it does not.
	sq[0].Team, sq[1].Team, sq[7].Team = "TX", "TX", "TX"
	pool := []PlayerMetrics{
		{ID: 100, Name: "Premium", Position: "FWD", Team: "TX", Price: 9.0, Score: 9.0},
		{ID: 101, Name: "Cheap", Position: "DEF", Team: "TY", Price: 3.0, Score: 2.6},
	}
	got, ok := firstPair(sq, pool, 0, 1)
	if ok && got.Up.In.Name == "Premium" {
		t.Error("bought a fourth player from one club")
	}
}

// firstPair is the single best funded pair, or none.
func firstPair(squad, pool []PlayerMetrics, bank, maxDowns int) (Pair, bool) {
	ps := RankPairs(NewSquadState(squad), pool, bank, maxDowns, 1)
	if len(ps) == 0 {
		return Pair{}, false
	}
	return ps[0], true
}

// TestRankSwapsAndPairsAgainstTheRealPool exercises both searches against live
// data, where the synthetic fixtures above cannot catch an ordering or legality
// bug that only appears at scale.
func TestRankSwapsAndPairsAgainstTheRealPool(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	sq, err := e.Optimize(OptimizeRequest{MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02})
	if err != nil {
		t.Skipf("optimiser unavailable: %v", err)
	}
	pool := e.AllMetrics()

	// An optimal squad with an empty bank has no improving swap, so testing
	// against it proves nothing. Degrade it first: replace each player with
	// someone at almost exactly his price who scores less. That leaves the
	// budget spent and the bank empty — the conditions a funded pair needs —
	// while giving both searches real work to do.
	squad := append([]PlayerMetrics(nil), sq.Players...)
	clubs := map[string]int{}
	for _, p := range squad {
		clubs[p.Team]++
	}
	for i, cur := range squad {
		for _, c := range pool {
			if c.Position != cur.Position || c.Score >= cur.Score {
				continue
			}
			if tenths(c.Price) != tenths(cur.Price) {
				continue
			}
			dup := false
			for j, q := range squad {
				if j != i && q.ID == c.ID {
					dup = true
				}
			}
			if dup {
				continue
			}
			// The replacement has to keep the squad legal, or the searches are
			// being asked about a fifteen that could never exist.
			if c.Team != cur.Team && clubs[c.Team] >= MaxPerClub {
				continue
			}
			clubs[cur.Team]--
			clubs[c.Team]++
			squad[i] = c
			break
		}
	}

	state := NewSquadState(squad)
	swaps := RankSwaps(state, pool, 0)
	base := XIValue(squad)
	trial := make([]PlayerMetrics, len(squad))
	for i, sw := range swaps {
		if state.Owned[sw.In.ID] {
			t.Fatalf("swap %d buys %s, who is already owned", i, sw.In.Name)
		}
		if sw.In.Position != sw.Out.Position {
			t.Fatalf("swap %d trades a %s for a %s", i, sw.Out.Position, sw.In.Position)
		}
		if i > 0 && sw.Gain > swaps[i-1].Gain {
			t.Fatalf("swaps are not ordered: %.3f after %.3f", sw.Gain, swaps[i-1].Gain)
		}
		if sw.Gain <= 0 {
			t.Fatalf("swap %d has gain %.3f; non-improving swaps must be dropped", i, sw.Gain)
		}
	}
	if len(swaps) == 0 {
		t.Fatal("no swaps found from a deliberately degraded squad")
	}
	{
		// The reported gain must be the actual change in the eleven.
		copy(trial, squad)
		for i, p := range trial {
			if p.ID == swaps[0].Out.ID {
				trial[i] = swaps[0].In
			}
		}
		if got := XIValue(trial) - base; math.Abs(got-swaps[0].Gain) > 1e-9 {
			t.Errorf("best swap reports %.4f but moves the eleven by %.4f", swaps[0].Gain, got)
		}
	}

	// Every pair must be a premium no single swap could have reached, and must
	// hold the three-per-club limit across both legs.
	pairs := RankPairs(state, pool, 0, 4, 5)
	t.Logf("%d single swaps, %d funded pairs", len(swaps), len(pairs))
	for _, pr := range pairs {
		if pr.Moves() < 2 {
			t.Errorf("a funded move must have at least one funding sale, got %d transfers",
				pr.Moves())
		}
		if tenths(pr.Up.In.Price) <= tenths(pr.Up.Out.Price) {
			t.Errorf("pair buys %s at £%.1fm to replace %s at £%.1fm — that is affordable alone",
				pr.Up.In.Name, pr.Up.In.Price, pr.Up.Out.Name, pr.Up.Out.Price)
		}
		if state.Owned[pr.Up.In.ID] {
			t.Error("pair buys a player already owned")
		}
		spend := tenths(pr.Up.In.Price) - tenths(pr.Up.Out.Price)
		for _, d := range pr.Downs {
			if state.Owned[d.In.ID] {
				t.Error("funding leg buys a player already owned")
			}
			spend += tenths(d.In.Price) - tenths(d.Out.Price)
		}
		if spend > 0 {
			t.Errorf("pair overspends by %d tenths with an empty bank", spend)
		}
		counts := map[string]int{}
		for _, p := range squad {
			counts[p.Team]++
		}
		counts[pr.Up.Out.Team]--
		counts[pr.Up.In.Team]++
		for _, d := range pr.Downs {
			counts[d.Out.Team]--
			counts[d.In.Team]++
		}
		for club, n := range counts {
			if n > MaxPerClub {
				t.Errorf("pair leaves %d players from %s", n, club)
			}
		}
	}
}

// TestSearchPairsFundsFromSeveralSalesWhenOneIsNotEnough is the point of
// generalising beyond a single funding sale.
//
// FPL raised the transfer bank from 2 to 5 for 2024-25, so a manager really can
// sell three players to buy one. Capped at a single funding sale, the same
// premiums stay unreachable that the one-for-one search could not reach — just
// at a higher price point.
func TestSearchPairsFundsFromSeveralSalesWhenOneIsNotEnough(t *testing.T) {
	sq := mkSwapSquad()
	// A £12.0m striker replacing a £7.0m one needs £5.0m, and no single
	// defender sale frees more than £2.0m.
	pool := []PlayerMetrics{
		{ID: 100, Name: "Premium", Position: "FWD", Team: "TX", Price: 12.0, Score: 9.0},
		{ID: 101, Name: "CheapA", Position: "DEF", Team: "TY", Price: 3.0, Score: 2.8},
		{ID: 102, Name: "CheapB", Position: "DEF", Team: "TZ", Price: 3.0, Score: 2.8},
		{ID: 103, Name: "CheapC", Position: "DEF", Team: "TW", Price: 3.0, Score: 2.8},
	}
	if _, ok := firstPair(sq, pool, 0, 1); ok {
		t.Error("one funding sale frees £2.0m against a £5.0m shortfall and must not suffice")
	}
	got, ok := firstPair(sq, pool, 0, 3)
	if !ok {
		t.Fatal("three funding sales free £6.0m and should reach the premium")
	}
	if got.Up.In.Name != "Premium" {
		t.Errorf("bought %q, want Premium", got.Up.In.Name)
	}
	if n := len(got.Downs); n < 3 {
		t.Errorf("funded from %d sales; £5.0m needs three at £2.0m each", n)
	}
	if got.Moves() != 1+len(got.Downs) {
		t.Errorf("Moves() = %d against 1 upgrade and %d sales", got.Moves(), len(got.Downs))
	}
	// Each funding sale must be from a different squad slot — selling the same
	// player twice would raise imaginary money.
	seen := map[int]bool{}
	for _, d := range got.Downs {
		if seen[d.Out.ID] {
			t.Errorf("player %s sold twice in one move", d.Out.Name)
		}
		seen[d.Out.ID] = true
	}
}

// TestSearchPairsPrefersFewerTransfers — two routes to the same eleven are not
// equally good, because transfers are the scarce resource.
func TestSearchPairsPrefersFewerTransfers(t *testing.T) {
	sq := mkSwapSquad()
	pool := []PlayerMetrics{
		{ID: 100, Name: "Premium", Position: "FWD", Team: "TX", Price: 9.0, Score: 9.0},
		// One sale covers the whole £2.0m shortfall; the others are redundant.
		{ID: 101, Name: "Cheap", Position: "DEF", Team: "TY", Price: 3.0, Score: 2.9},
	}
	got, ok := firstPair(sq, pool, 0, 4)
	if !ok {
		t.Fatal("no funded move found")
	}
	if got.Moves() != 2 {
		t.Errorf("used %d transfers where 2 suffice", got.Moves())
	}
}

// TestForcedStarterIsSelectedNotBenched is the distinction that makes a lock
// worth having.
//
// Locking guarantees squad membership, and the optimiser will satisfy that the
// cheapest way it can. Locking Isak put £9.0m on the bench at 0.53 pts/gw and
// built the eleven around it — the opposite of "the squad is built around him".
func TestForcedStarterIsSelectedNotBenched(t *testing.T) {
	squad := mkSwapSquad()
	// The worst forward in the squad, who a plain bestXI would never start.
	worst := -1
	for i, p := range squad {
		if p.Position != "FWD" {
			continue
		}
		if worst < 0 || p.Score < squad[worst].Score {
			worst = i
		}
	}
	squad[worst].Score = 0.5 // clearly the last player anyone would pick

	xi, _, _ := bestXI(squad)
	for _, p := range xi {
		if p.ID == squad[worst].ID {
			t.Fatal("fixture broken: the weakest forward starts unforced")
		}
	}

	xi, bench, formation := bestXIWith(squad, map[int]bool{squad[worst].ID: true})
	started := false
	for _, p := range xi {
		if p.ID == squad[worst].ID {
			started = true
		}
	}
	if !started {
		t.Errorf("a forced starter was benched; formation %s, bench %d", formation, len(bench))
	}
	if len(xi) != 11 {
		t.Errorf("forcing a starter produced %d players, not 11", len(xi))
	}
}

// TestForcingMorePlayersThanAFormationSeats — three forced forwards cannot be
// seated by a 3-5-2, so the search must pick a formation that fits them rather
// than silently dropping one.
func TestForcingMorePlayersThanAFormationSeats(t *testing.T) {
	squad := mkSwapSquad()
	must := map[int]bool{}
	for i := range squad {
		if squad[i].Position == "FWD" {
			must[squad[i].ID] = true
		}
	}
	if len(must) != 3 {
		t.Fatalf("fixture has %d forwards, want 3", len(must))
	}
	xi, _, formation := bestXIWith(squad, must)
	seated := 0
	for _, p := range xi {
		if must[p.ID] {
			seated++
		}
	}
	if seated != 3 {
		t.Errorf("%d of 3 forced forwards started in a %s", seated, formation)
	}
}

// TestStartIDsSurviveTheWholeOptimiser guards the bug the feature shipped with.
//
// StartIDs began as a list parallel to LockIDs, and four places in Optimize read
// LockIDs directly. The greedy start pre-placed the forced player, the DP seeds
// did not, holdsLocks then rejected every seed, and the answer came back without
// him at all — strictly worse than the plain lock it was meant to strengthen.
//
// StartIDs is now folded into LockIDs once at the top, so a forced starter is a
// lock everywhere downstream by construction rather than by each call site
// remembering.
func TestStartIDsSurviveTheWholeOptimiser(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())

	// A player the optimiser would never pick: expensive and barely playing.
	var target PlayerMetrics
	for _, m := range e.AllMetrics() {
		if m.Position == "FWD" && m.ExpectedMinutes < 30 && m.Price >= 8.0 {
			target = m
			break
		}
	}
	if target.ID == 0 {
		t.Skip("no fringe premium forward in the pool")
	}

	base := OptimizeRequest{MinMinutes: 600, MinExpectedMinutes: 55, BenchWeight: 0.02}
	unforced, err := e.Optimize(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range unforced.Players {
		if p.ID == target.ID {
			t.Skipf("%s is picked unforced; the test proves nothing", target.Name)
		}
	}

	req := base
	req.StartIDs = []int{target.ID}
	got, err := e.Optimize(req)
	if err != nil {
		t.Fatalf("optimising with a forced starter: %v", err)
	}
	inSquad, inXI := false, false
	for _, p := range got.Players {
		if p.ID == target.ID {
			inSquad = true
		}
	}
	for _, p := range got.StartingXI {
		if p.ID == target.ID {
			inXI = true
		}
	}
	if !inSquad {
		t.Fatalf("%s was forced to start and is not even in the squad", target.Name)
	}
	if !inXI {
		t.Errorf("%s is in the squad but benched; StartIDs did not reach selection", target.Name)
	}
	if len(got.StartingXI) != 11 || len(got.Players) != SquadSize {
		t.Errorf("squad is %d with an XI of %d", len(got.Players), len(got.StartingXI))
	}
}
