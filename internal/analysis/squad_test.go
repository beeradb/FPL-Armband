package analysis

import (
	"strings"
	"testing"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	// Built from a COMMITTED CAPTURE, not the live API. See
	// capturetestengine_test.go for why: the live fetch made this suite
	// irreproducible, cost a cold 1.6MB round trip per call site, and -- worst --
	// skipped when FPL was unreachable, so the whole package went green while
	// testing nothing.
	return captureEngine(t)
}

func TestOptimizeProducesLegalSquad(t *testing.T) {
	e := testEngine(t)
	skipDuringLiveGW1Gap(t, e)

	sq, err := e.Optimize(OptimizeRequest{MinMinutes: 500})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	if len(sq.Players) != SquadSize {
		t.Errorf("squad size = %d, want %d", len(sq.Players), SquadSize)
	}
	if len(sq.StartingXI) != 11 {
		t.Errorf("XI size = %d, want 11", len(sq.StartingXI))
	}
	if len(sq.Bench) != 4 {
		t.Errorf("bench size = %d, want 4", len(sq.Bench))
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

	if sq.TotalCost > 100.0 {
		t.Errorf("total cost £%.1fm exceeds £100.0m budget", sq.TotalCost)
	}
	if sq.Remaining < 0 {
		t.Errorf("negative remaining budget: %.1f", sq.Remaining)
	}

	// Every starter should be someone we'd actually field.
	for _, p := range sq.StartingXI {
		if p.Score <= 0 {
			t.Errorf("starter %s has non-positive score %.2f", p.Name, p.Score)
		}
	}

	t.Logf("formation %s, cost £%.1fm, XI score %.1f", sq.Formation, sq.TotalCost, sq.XIScore)
	for _, p := range sq.StartingXI {
		t.Logf("  XI  %-3s %-16s %-4s £%.1fm  score %.2f  fdr %.1f", p.Position, p.Name, p.Team, p.Price, p.Score, p.AvgDifficulty)
	}
	for _, p := range sq.Bench {
		t.Logf("  SUB %-3s %-16s %-4s £%.1fm  score %.2f", p.Position, p.Name, p.Team, p.Price, p.Score)
	}
}

func TestOptimizeRespectsLocksAndExclusions(t *testing.T) {
	e := testEngine(t)

	base, err := e.Optimize(OptimizeRequest{MinMinutes: 500})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Exclude the top scorer and lock a player who was not selected.
	excluded := base.StartingXI[0].ID
	var lockID int
	inBase := map[int]bool{}
	for _, p := range base.Players {
		inBase[p.ID] = true
	}
	for _, m := range e.AllMetrics() {
		if !inBase[m.ID] && m.Position == "MID" && m.Minutes > 1500 && m.Price < 7.0 {
			lockID = m.ID
			break
		}
	}
	if lockID == 0 {
		t.Skip("no suitable lock candidate found")
	}

	sq, err := e.Optimize(OptimizeRequest{
		MinMinutes: 500,
		LockIDs:    []int{lockID},
		ExcludeIDs: []int{excluded},
	})
	if err != nil {
		t.Fatalf("Optimize with constraints: %v", err)
	}

	found := false
	for _, p := range sq.Players {
		if p.ID == excluded {
			t.Errorf("excluded player %d appears in squad", excluded)
		}
		if p.ID == lockID {
			found = true
		}
	}
	if !found {
		t.Errorf("locked player %d missing from squad", lockID)
	}
	if sq.TotalCost > 100.0 {
		t.Errorf("total cost £%.1fm exceeds budget", sq.TotalCost)
	}
}

func TestBestXIFormationIsLegal(t *testing.T) {
	e := testEngine(t)
	sq, err := e.Optimize(OptimizeRequest{MinMinutes: 500})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	pos := map[string]int{}
	for _, p := range sq.StartingXI {
		pos[p.Position]++
	}
	if pos["GKP"] != 1 {
		t.Errorf("XI has %d keepers, want 1", pos["GKP"])
	}
	for _, k := range []string{"DEF", "MID", "FWD"} {
		if pos[k] < xiMin[k] || pos[k] > xiMax[k] {
			t.Errorf("XI has %d %s, want %d-%d", pos[k], k, xiMin[k], xiMax[k])
		}
	}
}

// TestMinutesReliabilityTracksExpectedMinutes guards against the regression
// where minutes reliability was derived from FPL's starts_per_90 field. That
// field measures "when this player appears, does he start", which is ~1.0 for
// nearly every player, so a 25-minute-per-week rotation option scored the same
// as an ever-present.
func TestMinutesReliabilityTracksExpectedMinutes(t *testing.T) {
	e := testEngine(t)

	var nailed, fringe []PlayerMetrics
	for _, m := range e.AllMetrics() {
		switch {
		case m.ExpectedMinutes >= 78:
			nailed = append(nailed, m)
		case m.ExpectedMinutes > 0 && m.ExpectedMinutes <= 30:
			fringe = append(fringe, m)
		}
	}
	if len(nailed) == 0 || len(fringe) == 0 {
		t.Skip("dataset lacks both nailed and fringe players")
	}

	var worstNailed = 1.0
	for _, m := range nailed {
		if m.MinutesRating < worstNailed {
			worstNailed = m.MinutesRating
		}
	}
	var bestFringe float64
	for _, m := range fringe {
		if m.MinutesRating > bestFringe {
			bestFringe = m.MinutesRating
		}
	}

	if bestFringe >= worstNailed {
		t.Errorf("fringe players rate as high as nailed ones: best fringe %.3f >= worst nailed %.3f",
			bestFringe, worstNailed)
	}
	if bestFringe > 0.5 {
		t.Errorf("a player averaging <=30 min/gw rated %.3f; rotation risk is not being penalised", bestFringe)
	}
	t.Logf("worst nailed %.3f, best fringe %.3f", worstNailed, bestFringe)
}

func TestOptimizeRespectsExpectedMinutesFloor(t *testing.T) {
	e := testEngine(t)
	// ⚠️ Two, not the GW1 gap alone. skipDuringLiveGW1Gap closes only the window
	// where some clubs have played and some have not; it says nothing about how
	// much evidence a minutes floor needs to be satisfiable. With ONE gameweek
	// played the league's expected-minutes estimates are drawn from a single
	// match, and the optimiser can legitimately find no eleven that all clear 60
	// — the assertion then fails on a data state rather than on a defect.
	//
	// Observed 2026-08-30, one gameweek in and GW2 stuck unfinished for two days:
	// this passed at 08:17 and failed at 10:05 on unchanged code, which is the
	// signature of a live-data coupling rather than a regression. Two mirrors
	// corroboratingMatches, this package's own bar for trustworthy minutes, and
	// the same literal internal/agent's optimize_test.go already uses.
	skipUntilLiveEvidence(t, e, 2)
	sq, err := e.Optimize(OptimizeRequest{MinExpectedMinutes: 60, BenchWeight: 0.02})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	for _, p := range sq.StartingXI {
		// Cheap fodder is exempt from the floor, but must not reach the XI.
		if p.ExpectedMinutes < 60 {
			t.Errorf("XI contains %s at %.1f expected minutes, below the 60 floor",
				p.Name, p.ExpectedMinutes)
		}
	}
}

// firstIDsAt returns the element ids of the first n players at pos.
//
// No price or club screen, deliberately. A rejection on the squad rules is
// decided before the budget or the three-per-club cap is consulted, so a test
// of one must not hand over a request those would have refused anyway: it could
// then pass without the check it is written for ever running, which is why each
// rejection test asserts the error text as well. The one acceptance test that
// reads from here forces a single keeper, which any budget seats.
func firstIDsAt(t *testing.T, e *Engine, pos string, n int) []int {
	t.Helper()
	var out []int
	for _, m := range e.AllMetrics() {
		if m.Position == pos {
			out = append(out, m.ID)
			if len(out) == n {
				return out
			}
		}
	}
	t.Skipf("pool holds %d players at %s, need %d", len(out), pos, n)
	return nil
}

// cheapIDsAt returns n ids at pos, each from a different club and priced at the
// bench-fodder end of the market.
//
// For the tests that must reach the optimiser rather than be turned back at the
// door. Neither the three-per-club cap nor the budget is what a forced-start
// test is about, and both are checked AFTER the seating check, so a request
// either of them would refuse cannot demonstrate what the seating check does —
// disabling the check under test would then produce a different error rather
// than the silent collapse, and the test would report a pass it had not earned.
// That is not hypothetical: ten forced starters taken in pool order are ten
// Arsenal players.
func cheapIDsAt(t *testing.T, e *Engine, pos string, n int) []int {
	t.Helper()
	var out []int
	seenClub := map[string]bool{}
	for _, m := range e.AllMetrics() {
		if m.Position != pos || m.Price > 4.5 || seenClub[m.Team] {
			continue
		}
		seenClub[m.Team] = true
		out = append(out, m.ID)
		if len(out) == n {
			return out
		}
	}
	t.Skipf("only %d cheap %s from distinct clubs, need %d", len(out), pos, n)
	return nil
}

// TestTwoForcedStartingGoalkeepersAreRejected pins the collapse that forcing
// both keepers to start used to produce.
//
// GKP is the only position where squadQuota (2) exceeds xiMax (1), so two
// forced keepers cannot be seated by any formation. bestFormation skipped every
// candidate on exactly that test, returned ok=false, and materialise left the
// pick empty — so ON THE PRE-FIX CODE THIS REQUEST RETURNED err == nil AND A
// SQUAD WITH A ZERO-LENGTH XI, all fifteen benched, Formation == "", a
// zero-value captain, and an objective quietly degenerated to benchValue over
// the whole squad. That is what the failure message below reports, so the test
// fails on the old code by naming the collapse rather than by a bare
// "wanted an error".
//
// No live-gap skip on purpose. The rejection is a statement about the squad
// rules, not about any football, so it must hold in every data state.
func TestTwoForcedStartingGoalkeepersAreRejected(t *testing.T) {
	e := testEngine(t)
	ids := firstIDsAt(t, e, "GKP", 2)

	squad, err := e.Optimize(OptimizeRequest{MinMinutes: 500, StartIDs: ids})
	if err == nil {
		// The nil check comes first: reporting the collapse means reading the
		// squad, and a nil squad beside a nil error is the same defect one
		// stage further along.
		if squad == nil {
			t.Fatal("two forced keepers were accepted, returning no squad and no error")
		}
		t.Fatalf("two forced keepers were accepted; XI %d, bench %d, formation %q",
			len(squad.StartingXI), len(squad.Bench), squad.Formation)
	}
	if !strings.Contains(err.Error(), "can start at GKP") {
		t.Errorf("error does not name the position that overflowed: %v", err)
	}
	if squad != nil {
		t.Errorf("an error came back with a squad attached: %+v", squad)
	}
}

// TestForcedStartersOverTheOutfieldBudgetAreRejected is the AGGREGATE half of
// the seating check, and the case a per-position bound cannot see.
//
// Five defenders is exactly xiMax["DEF"] and five midfielders exactly
// xiMax["MID"], so every position is individually within what an eleven can
// seat. No formation seats them together: a legal eleven fields at least one
// forward, so d>=5, m>=5, f>=1 is eleven outfield players for the ten places
// beside the keeper. xiMax sums to 13 outfield, not 10, which is why bounding
// each position on its own leaves this reachable — and on such a check this
// request returned err == nil with a zero-length XI and formation "", the same
// silent collapse two forced keepers produced, arrived at through counts each
// of which is legal by itself.
//
// No live-gap skip, for the reason on TestTwoForcedStartingGoalkeepersAreRejected:
// the rejection is a statement about the squad rules, not about any football.
// The forced ten ARE screened on price and club, though — see cheapIDsAt — so
// that the lock quota, the club cap and the budget, all of which are checked
// after the seating check, cannot be what turns this request back.
func TestForcedStartersOverTheOutfieldBudgetAreRejected(t *testing.T) {
	e := testEngine(t)
	ids := cheapIDsAt(t, e, "DEF", xiMax["DEF"])
	ids = append(ids, cheapIDsAt(t, e, "MID", xiMax["MID"])...)

	squad, err := e.Optimize(OptimizeRequest{MinMinutes: 500, StartIDs: ids})
	if err == nil {
		if squad == nil {
			t.Fatal("eleven outfield forced starters were accepted, returning no squad and no error")
		}
		t.Fatalf("%d forced starters for %d outfield places were accepted; XI %d, bench %d, formation %q",
			len(ids), XISize-1, len(squad.StartingXI), len(squad.Bench), squad.Formation)
	}
	if !strings.Contains(err.Error(), "cannot fit one legal formation") {
		t.Errorf("rejected, but not by the seating check: %v", err)
	}
	if squad != nil {
		t.Errorf("an error came back with a squad attached: %+v", squad)
	}
}

// TestOneForcedStartingGoalkeeperStillBuilds is the liveness half: the check
// above must reject the infeasible request without rejecting the legal one that
// sits immediately below it. One forced keeper is exactly xiMax["GKP"].
func TestOneForcedStartingGoalkeeperStillBuilds(t *testing.T) {
	e := testEngine(t)
	skipDuringLiveGW1Gap(t, e)
	ids := firstIDsAt(t, e, "GKP", 1)

	sq, err := e.Optimize(OptimizeRequest{MinMinutes: 500, StartIDs: ids})
	if err != nil {
		t.Fatalf("one forced keeper was rejected: %v", err)
	}
	if len(sq.Players) != SquadSize {
		t.Errorf("squad size = %d, want %d", len(sq.Players), SquadSize)
	}
	started := false
	for _, p := range sq.StartingXI {
		if p.ID == ids[0] {
			started = true
		}
	}
	if !started {
		t.Errorf("the forced keeper is not in the XI; formation %q", sq.Formation)
	}
	byPos := map[string]int{}
	for _, p := range sq.StartingXI {
		byPos[p.Position]++
	}
	if !LegalFormation(byPos) {
		t.Errorf("illegal formation %q from %v", sq.Formation, byPos)
	}
}

// TestForcedStartersUpToTheOutfieldMaximumAreAccepted guards the new check
// against over-rejecting.
//
// It is written over an outfield position rather than GKP because the check is
// general: five defenders is xiMax["DEF"] and also squadQuota["DEF"], so it is
// the tightest legal outfield request there is, and an off-by-one in the bound
// would refuse it.
func TestForcedStartersUpToTheOutfieldMaximumAreAccepted(t *testing.T) {
	e := testEngine(t)
	skipDuringLiveGW1Gap(t, e)

	ids := cheapIDsAt(t, e, "DEF", xiMax["DEF"])

	sq, err := e.Optimize(OptimizeRequest{MinMinutes: 500, StartIDs: ids})
	if err != nil {
		t.Fatalf("%d forced defenders is xiMax and was rejected: %v", len(ids), err)
	}
	forced := map[int]bool{}
	for _, id := range ids {
		forced[id] = true
	}
	seated := 0
	for _, p := range sq.StartingXI {
		if forced[p.ID] {
			seated++
		}
	}
	if seated != len(ids) {
		t.Errorf("%d of %d forced defenders started, formation %q", seated, len(ids), sq.Formation)
	}
	byPos := map[string]int{}
	for _, p := range sq.StartingXI {
		byPos[p.Position]++
	}
	if !LegalFormation(byPos) {
		t.Errorf("illegal formation %q from %v", sq.Formation, byPos)
	}
}

// TestTheSeatingCheckAgreesWithTheFormationSearch is the differential test the
// joint condition rests on: over every forced-start set a legal fifteen can
// hold, the check's verdict has to be the SEARCH's verdict.
//
// checkLocksAndForcedStarts states the seating rule a second time, in closed
// form, ahead of the search that really decides it. One quantity with two
// implementations is the failure this repository knows best, and the closed
// form got it wrong in exactly that way the first time it was written: bounding
// each position by its own xiMax accepts five forced defenders beside five
// forced midfielders, which no formation seats. So the check is not tested
// against a restatement of the rule here — it is tested against the real
// xiScratch.bestFormation, run over a full synthetic fifteen, on all 432 forced
// sets the squad quotas permit (3 x 6 x 6 x 4).
//
// Needs neither the API nor an Engine: the squad is built by hand and the only
// inputs are the formation tables.
func TestTheSeatingCheckAgreesWithTheFormationSearch(t *testing.T) {
	// A full legal fifteen, scores descending with the id. Every position is
	// filled to its squadQuota, so nDEF, nMID and nFWD are each that position's
	// xiMax and bestFormation's availability test can never be what rejects a
	// formation — leaving `forced` as the only binding constraint.
	var squad []PlayerMetrics
	byID := map[int]PlayerMetrics{}
	idsAt := map[string][]int{}
	next := 1
	for _, pos := range posNames {
		for i := 0; i < squadQuota[pos]; i++ {
			p := PlayerMetrics{ID: next, Position: pos, Price: 4.0, Score: float64(100 - next)}
			squad = append(squad, p)
			byID[p.ID] = p
			idsAt[pos] = append(idsAt[pos], p.ID)
			next++
		}
	}

	for g := 0; g <= squadQuota["GKP"]; g++ {
		for d := 0; d <= squadQuota["DEF"]; d++ {
			for m := 0; m <= squadQuota["MID"]; m++ {
				for f := 0; f <= squadQuota["FWD"]; f++ {
					want := [4]int{g, d, m, f}
					mustStart := map[int]bool{}
					var req OptimizeRequest
					for i, pos := range posNames {
						for _, id := range idsAt[pos][:want[i]] {
							mustStart[id] = true
							req.StartIDs = append(req.StartIDs, id)
							req.LockIDs = append(req.LockIDs, id)
						}
					}

					// The counts come from split rather than from `want`
					// because split is what sorts forced players to the front
					// of each position, which is what makes bestFormation's
					// `forced[i] > d` test the right one.
					var sc xiScratch
					_, _, _, _, seatable := sc.bestFormation(sc.split(squad, mustStart))

					err := checkLocksAndForcedStarts(req, mustStart, byID)
					if seatable && err != nil {
						t.Errorf("%v is seated by a legal formation and the check refused it: %v", want, err)
					}
					if !seatable && err == nil {
						t.Errorf("%v is seated by no formation and the check accepted it, "+
							"which is the silent collapse", want)
					}
				}
			}
		}
	}
}
