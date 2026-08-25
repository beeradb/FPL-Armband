package backtest

// Per-gameweek best-captain projection: what a manager means by "play the
// triple captain when your best player has his best fixture".
//
// # Why this exists
//
// `anchored_diag_test.go` places the triple captain on the best remaining
// DOUBLE gameweek. That is real FPL practice for the second half of a season,
// and it is only half the rule: in the first half there are usually no doubles
// at all, so the rule finds nothing and the chip goes unplayed. The other half,
// and the dominant one early, is opponent quality — captain your best attacker
// against the weakest side available.
//
// # ⚠️ It is YOUR best player, not the league's, and the first version got this wrong
//
// The first version took the maximum over `AllMetrics()` — every player in the
// league — and so chose the week that suited whoever the league's top projected
// scorer was that week. A manager does not own that player. The chip is applied
// to the captain of the fielded eleven, drawn from fifteen owned players, so the
// week was being selected on a signal the squad could not cash in. That dilutes
// the effect toward zero, and the measurement showed it: +4.6 points a season
// against roughly +7.8 implied by the projections the rule was reading.
//
// The fix is `topN`: rank players once on the SEASON view, keep the few a
// manager would plausibly own and captain, and time the chip on those. It is a
// proxy for the squad rather than the squad itself — see the caveat below — but
// it is a proxy for the right quantity, which the league-wide maximum was not.
//
// # Why not the actual opening squad
//
// `Simulate` builds the opening fifteen at its own line, AFTER the chip plan is
// resolved, and the plan feeds squad construction (a planned bench boost changes
// the bench weight). Reading the real squad here would mean either reordering
// that — a circular dependency — or rebuilding it, which is one quantity with
// two implementations, this project's signature failure.
//
// `topN` avoids both. It is defensible for this specific quantity because the
// triple captain goes on a premium, premiums are near-universally owned, and a
// premium is the most stable holding in a squad — the player least likely to
// have been transferred out by the week the chip is played.
//
// ⚠️ It is still a PROXY. It does not verify the player was owned in that week,
// and a rule that picked a premium this squad never bought would overstate the
// achievable effect.
//
// # The scoring is the engine's own, not a second implementation
//
// Scoring "player p in gameweek g" by reaching for `fixtureSensitiveAt` and
// reassembling rates, minutes and multipliers by hand would drift from the
// scored path the moment either side changed. Instead this uses the seam the
// engine already has: `SetSkipGameweeks`, built for the free hit to take a week
// OUT of scoring, inverted to skip every week except `g`. `TeamFixtures` then
// returns only `g`'s fixtures, so `Metrics().Score` is that single gameweek's
// projection, assembled by exactly the code that scores every other number here.
//
// # ⚠️ Horizon 1 is load-bearing, not a tuning choice
//
// `Engine.FixtureLoadInScore` is true only at horizon 1. At the shipped horizon
// of 5 the score is a five-week average and does NOT carry `FixtureLoad`, so a
// club playing twice scores exactly like a club playing once and the whole
// second half of the strategy is invisible. Measured: at horizon 5 the best
// projected score took FOUR distinct values across a season with no double
// anywhere; at horizon 1 it takes fourteen, doubles (12.0, 13.6, 14.2) standing
// clear of ordinary weeks (~5-7) and blanks (4.4, 5.0) below.
//
// # Point-in-time
//
// Engines are built by `EngineAt` at the planning cutoff, so every rate, prior
// and minutes estimate comes from data on or before that gameweek. The FIXTURE
// LIST is read ahead, which the `ChipPlanner` doc comment calls "the cheap kind
// of knowledge" — the schedule is public in advance. No realised points and no
// final minutes are read; that would be an Oracle and would have to be stamped.
//
// ⚠️ What this does NOT model is a manager re-deciding as the season unfolds.
// Projections are made once, at the cutoff, so a week 30 chosen from week 1
// information rests on week 1's view of form.

import (
	"sort"

	"armband/internal/analysis"
)

// captainCandidates is how many of the top-ranked players the timing rule may
// captain. Three rather than one: a manager owns more than one premium, and
// taking only the single best would make the rule hostage to one player's
// fixture run. Not swept — it is a structural choice about what "your best
// player" means, and sweeping it would be fitting the definition to the answer.
const captainCandidates = 3

// TopCaptainCandidates ranks players on the SEASON view and returns the element
// ids of the few a manager would plausibly own and captain.
//
// Ranked on the season engine — the shipped horizon — deliberately: "who is my
// best player" is a question about the whole season, not about one week. Timing
// is the per-week question, and it is asked separately.
func TopCaptainCandidates(e *analysis.Engine, n int) []int {
	if e == nil || n < 1 {
		return nil
	}
	all := e.AllMetrics()
	sort.SliceStable(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	if n > len(all) {
		n = len(all)
	}
	out := make([]int, 0, n)
	for _, m := range all[:n] {
		out = append(out, m.ID)
	}
	return out
}

// BestCaptainXPByGameweek returns, for each gameweek in (after, 38], the
// highest score any of `candidates` projects in that gameweek.
//
// The engine is mutated (its skip set is rewritten) and left with an empty skip
// set on return, so pass an engine built for this purpose — not one a
// simulation is scoring against. It should be a HORIZON 1 engine; see above.
func BestCaptainXPByGameweek(e *analysis.Engine, after int, candidates []int) map[int]float64 {
	out := map[int]float64{}
	if e == nil || len(candidates) == 0 {
		return out
	}
	want := make(map[int]bool, len(candidates))
	for _, id := range candidates {
		want[id] = true
	}
	defer e.SetSkipGameweeks(nil)

	for gw := after + 1; gw <= 38; gw++ {
		skip := make([]int, 0, 37)
		for other := 1; other <= 38; other++ {
			if other != gw {
				skip = append(skip, other)
			}
		}
		e.SetSkipGameweeks(skip)

		best := 0.0
		for _, m := range e.AllMetrics() {
			if want[m.ID] && m.Score > best {
				best = m.Score
			}
		}
		// A gameweek in which none of the candidates plays scores nothing and is
		// not a candidate week; leaving it out of the map is what makes "no
		// entry" mean "nobody to captain" rather than "zero points available".
		if best > 0 {
			out[gw] = best
		}
	}
	return out
}
