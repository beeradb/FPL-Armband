package main

import (
	"fmt"
	"math/rand"
	"sort"

	"armband/internal/analysis"
)

// How the fifteen on the page is chosen.
//
// Three cases, and keeping them apart is what lets the page be fast, stable and still
// honest about what the model would pick:
//
//   - the reader has a team: rebuild it, no search at all;
//   - the reader pressed Optimize: the true optimum;
//   - a new session: a varied squad, seeded so it is stable across reloads.

// arrangement is the reader's own lineup, by code. Empty fields fall back to the model's
// choice, which is what a fresh team has.
type arrangement struct {
	XI      []int
	Bench   []int
	Captain int
	Vice    int
}

// squadFromCodes rebuilds a stored fifteen without running the optimiser.
//
// This is the reload path, and it is arithmetic rather than a search: the fifteen is known,
// so only the arrangement has to be settled. A reload therefore costs nothing like the
// second the optimiser takes.
//
// It returns nil if any player has gone — transferred out of the game, or a code the
// bootstrap no longer carries. A partial fifteen is not a team, and silently drawing
// fourteen players would be worse than starting again.
func squadFromCodes(e *analysis.Engine, pool []analysis.PlayerMetrics, codes []int, arr arrangement) *analysis.Squad {
	if len(codes) != 15 {
		return nil
	}
	byCode := map[int]analysis.PlayerMetrics{}
	for _, m := range pool {
		if code, ok := elementCode(e, m.ID); ok {
			byCode[code] = m
		}
	}
	fifteen := make([]analysis.PlayerMetrics, 0, 15)
	for _, c := range codes {
		m, ok := byCode[c]
		if !ok {
			return nil
		}
		fifteen = append(fifteen, m)
	}

	// The reader's arrangement wins where they have one.
	//
	// BestXI is the model's answer to "who should start", and re-deriving it here would
	// silently undo every drag the reader made -- they would move a player to the bench,
	// reload, and find him back in the eleven with nothing to explain it. The model's
	// answer is the DEFAULT, not the correction.
	xi, bench, formation := analysis.BestXI(fifteen)
	if picked, benched, ok := arrangeFrom(e, fifteen, arr); ok {
		xi, bench = picked, benched
		formation = formationOf(xi)
	}
	sq := &analysis.Squad{
		Players:    fifteen,
		StartingXI: xi,
		Bench:      bench,
		Formation:  formation,
		ClubCounts: map[string]int{},
	}
	// Remaining is what the reader can still spend, and Optimize sets it on its own path.
	// This one built the squad by hand and left it at zero, so every reload reported an
	// empty bank -- and the replacement picker spends exactly this figure, so it refused
	// funded transfers with the arithmetic shown on screen.
	// The money is counted in integer TENTHS, as Optimize counts it.
	//
	// Summing fifteen float prices and subtracting leaves about 1e-14 of dust, and the
	// client compares affordability raw: `bank + sold - price >= 0`. With a £0.5m bank, a
	// £4.5m player out and a £5.0m player in, the integer path gives exactly 0 and the float
	// path gives −8.9e-16, so the same target is reachable from a fresh build and silently
	// missing after a reload. The reload is the common path, because a saved fifteen skips
	// the optimiser.
	spent := 0
	for _, m := range fifteen {
		spent += int(m.Price*10 + 0.5)
		sq.ClubCounts[m.Team]++
	}
	sq.TotalCost = float64(spent) / 10
	if budget, _, err := e.AssemblyBudget(); err == nil {
		sq.Remaining = float64(budget-spent) / 10
	}
	for _, m := range xi {
		sq.XIScore += m.Score
	}
	// The armband goes to the two highest scorers in the eleven, which is the rule the
	// replay scores, so the page and the objective never disagree about who wears it.
	ranked := append([]analysis.PlayerMetrics(nil), xi...)
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
	if len(ranked) > 0 {
		sq.Captain = ranked[0]
		sq.ExpectedPoints = sq.XIScore + ranked[0].Score
	}
	if len(ranked) > 1 {
		sq.ViceCaptain = ranked[1]
	}
	return sq
}

// violatesRoster reports whether a stored fifteen breaks the standing corrections.
//
// Excluded players must not be in it, locked players must be. Anything else -- budget, the
// club limit, the position quotas -- is checked where the squad enters, in validateSession,
// because those are facts about a legal squad rather than about the reader's corrections.
func violatesRoster(sq *analysis.Squad, req analysis.OptimizeRequest) bool {
	if sq == nil {
		return false
	}
	in := map[int]bool{}
	for _, p := range sq.Players {
		in[p.ID] = true
	}
	for _, id := range req.ExcludeIDs {
		if in[id] {
			return true
		}
	}
	for _, id := range req.LockIDs {
		if !in[id] {
			return true
		}
	}
	// StartIDs is checked against the ELEVEN, not the fifteen. A must-start lock says the
	// player is picked, not merely owned — which is why Optimize threads mustStart all the
	// way into bestXIWith. Checking membership of the fifteen let a must-start player sit on
	// the reader's bench with the override silently not applied.
	starting := map[int]bool{}
	for _, p := range sq.StartingXI {
		starting[p.ID] = true
	}
	for _, id := range req.StartIDs {
		if !starting[id] {
			return true
		}
	}
	return false
}

// elementCode maps an element id to the permanent code.
func elementCode(e *analysis.Engine, id int) (int, bool) {
	for i := range e.Boot.Elements {
		if e.Boot.Elements[i].ID == id {
			return e.Boot.Elements[i].Code, true
		}
	}
	return 0, false
}

// varietySeed is how many players a varied squad is asked to do without.
//
// Two. One is often invisible — the optimiser frequently has a near-identical second
// choice for a single slot — and three starts costing real points, because each exclusion
// removes the best remaining answer to a question the budget has already been spent on.
const varietyExclusions = 2

// buildVariedSquad returns a good squad that is not the same good squad every time.
//
// # Why the page needs this at all
//
// The optimiser is deterministic, so every reader who opens the planner for the first time
// sees an identical fifteen, and the same reader sees it again tomorrow. That is correct
// and it is stale: the point of the tool is to argue with a suggestion, and an argument
// needs somewhere to start that is not always the same place.
//
// # Why the optimiser itself is not touched
//
// Determinism there is load-bearing and pinned — TestSeedOrderIsDeterministic,
// TestBandAssignmentIsDeterministic, TestOptimizerHotPathIsBitExact. Randomness inside
// Optimize would make the replay non-reproducible, which is the instrument every scoring
// claim in this project rests on.
//
// So the variety is entirely in the REQUEST: a seeded choice of two players to do without,
// and then the ordinary optimiser answering the ordinary question. Every squad this
// produces is a real optimum — of a slightly different question. Nothing here scores a
// player, ranks a squad, or decides what "good" means.
//
// # Why the excluded pair is drawn from the optimum rather than the pool
//
// Excluding two random players from six hundred almost always excludes two nobody was
// going to buy, and the answer does not move. Drawing from the squad the model actually
// wanted guarantees the question is different. It costs one extra optimiser run, once per
// session, because the result is then stored.
//
// The cost is bounded by construction: the replacements are the best remaining players at
// those prices, so the squad is worse than optimal by the gap between a first and second
// choice, twice.
func buildVariedSquad(e *analysis.Engine, req analysis.OptimizeRequest, seed int64) (*analysis.Squad, error) {
	best, err := e.Optimize(req)
	if err != nil {
		return nil, err
	}
	if seed == 0 || len(best.Players) == 0 {
		return best, nil
	}

	// Seeded, so the same session gets the same squad on every reload. A reader who
	// refreshes has not asked for a different team.
	rng := rand.New(rand.NewSource(seed))

	// Never the captain, and never a goalkeeper. Losing the best player in the squad is
	// not variety, it is a worse team; and with two keepers and one bench slot for them,
	// excluding one mostly just shuffles which cheap keeper sits out.
	var candidates []int
	for _, m := range best.Players {
		if m.ID == best.Captain.ID || m.Position == "GKP" {
			continue
		}
		candidates = append(candidates, m.ID)
	}
	// Sorted before shuffling: map and slice order upstream is not guaranteed stable
	// across runs, and an unsorted input would make the seed mean different things.
	sort.Ints(candidates)
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	n := varietyExclusions
	if len(candidates) < n {
		n = len(candidates)
	}
	if n == 0 {
		return best, nil
	}

	varied := req
	varied.ExcludeIDs = append(append([]int(nil), req.ExcludeIDs...), candidates[:n]...)
	out, err := e.Optimize(varied)
	if err != nil {
		// A varied squad is a nicety; the optimum is the product. If the constrained
		// question has no answer, answer the unconstrained one rather than failing.
		return best, nil
	}
	return out, nil
}

// newSeed draws a seed for a session that has none.
//
// math/rand rather than crypto/rand deliberately: this picks which of several good squads a
// reader is shown, and nothing about it is a secret. Using a cryptographic source would
// imply it was.
func newSeed() int64 {
	// Never zero — zero means "no variety" everywhere else in this file.
	for {
		if n := rand.Int63(); n != 0 {
			return n
		}
	}
}

// arrangeFrom resolves the reader's stored lineup against the fifteen.
//
// It returns ok=false and leaves the model's choice standing if the arrangement does not
// describe a legal eleven of exactly these players. That is not defensiveness: a stored
// lineup can go stale in ordinary use — a transfer replaces a player, and the eleven that
// mentioned him no longer covers the squad — and drawing ten players would be worse than
// drawing the model's eleven.
func arrangeFrom(e *analysis.Engine, fifteen []analysis.PlayerMetrics, arr arrangement) (xi, bench []analysis.PlayerMetrics, ok bool) {
	if len(arr.XI) != 11 {
		return nil, nil, false
	}
	byCode := map[int]analysis.PlayerMetrics{}
	for _, m := range fifteen {
		if code, found := elementCode(e, m.ID); found {
			byCode[code] = m
		}
	}
	seen := map[int]bool{}
	for _, code := range arr.XI {
		m, found := byCode[code]
		if !found || seen[code] {
			return nil, nil, false
		}
		seen[code] = true
		xi = append(xi, m)
	}
	// The bench is whatever is left, in the reader's order where they gave one.
	for _, code := range arr.Bench {
		if m, found := byCode[code]; found && !seen[code] {
			seen[code] = true
			bench = append(bench, m)
		}
	}
	for _, m := range fifteen {
		code, _ := elementCode(e, m.ID)
		if !seen[code] {
			bench = append(bench, m)
		}
	}
	if len(xi)+len(bench) != len(fifteen) {
		return nil, nil, false
	}
	if !legalShape(xi) {
		return nil, nil, false
	}
	return xi, bench, true
}

// legalShape is FPL's rule: one keeper, three to five defenders, two to five midfielders,
// one to three forwards.
//
// Checked because a stored eleven is data from a browser, and an illegal one would render
// as a pitch with four players in a row that has three slots.
func legalShape(xi []analysis.PlayerMetrics) bool {
	n := map[string]int{}
	for _, m := range xi {
		n[m.Position]++
	}
	return len(xi) == 11 && n["GKP"] == 1 &&
		n["DEF"] >= 3 && n["DEF"] <= 5 &&
		n["MID"] >= 2 && n["MID"] <= 5 &&
		n["FWD"] >= 1 && n["FWD"] <= 3
}

func formationOf(xi []analysis.PlayerMetrics) string {
	n := map[string]int{}
	for _, m := range xi {
		n[m.Position]++
	}
	return fmt.Sprintf("%d-%d-%d", n["DEF"], n["MID"], n["FWD"])
}

// totalCost is the price of a fifteen, in millions.
func totalCost(squad []analysis.PlayerMetrics) float64 {
	var t float64
	for _, m := range squad {
		t += m.Price
	}
	return t
}
