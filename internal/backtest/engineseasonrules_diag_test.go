package backtest

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
)

// Confinement and liveness for pinning the ENGINE's scoring rules per season and
// refusing a position it has no rules for.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagEngineSeasonRules -v -timeout 2h
//	DIAG=1 FPL_NO_SEASON_SCORING_RULES=1 \
//	    go test ./internal/backtest -run TestDiagEngineSeasonRules -v -timeout 2h
//	DIAG=1 FPL_NO_UNPRICED_POSITION_GUARD=1 \
//	    go test ./internal/backtest -run TestDiagEngineSeasonRules -v -timeout 2h
//
// One hatch per process, never both at once: they are separate defects and an arm
// that moved both could not attribute what it found. The confinement runs both
// `WeeklyXI` settings within each process, so no third switch is needed.
//
// Two processes rather than two arms in one, because both switches are read at
// package initialisation in `internal/analysis`. That is deliberate: a mutable
// global on the scoring path is what "a global left in the state the previous arm
// put it in" means, and this package's recorded failures are mostly that shape.
// The cost is that the two outputs are diffed by eye or by `diff`, and the format
// below is stable line-by-line so that works.
//
// # Why both halves, and why the confinement half alone would prove nothing
//
// This record's standing rule: *a confinement check on a path that cannot carry
// the effect confirms nothing — pair it with a liveness check that must move.*
// The sibling change to the xPoints instrument came back byte-identical on
// `hold_points` and `policy_points` in 30 of 30 cells, and read narrowly that was
// nearly worthless, because `Player.Rules` had exactly one reader and it was
// inside an instrumentation column.
//
// The engine's half is the opposite case — it writes `Score` — so a byte-identical
// points result here is a real confinement claim, but only if something is shown
// to have arrived. Two liveness arms are reported below, and both MUST be
// non-zero:
//
//   - **the season pin**: player-cutoffs whose `BaseXP90` differs between the
//     season's table and today's, reported beside the count whose **`Score`**
//     differs and the largest `|ΔScore|` anywhere on the grid. ⚠️ **Quote both.**
//     `BaseXP90` is the channel the rule enters; `Score` is what the optimiser
//     orders on, and a keeper with no minutes has the first move while the second
//     stays identically zero — so a `BaseXP90` count is an **upper bound on the
//     mediating population**, and the confinement below would otherwise be a
//     confinement of a quantity this arm never measured;
//   - **the unpriced-position guard**: `BaseXP90` for FPL's 2024-25 assistant
//     managers, which was exactly `appearancePoints` and must now be 0.
//
// ⚠️ **The liveness arm asserts in the SHIPPED process only.** With either hatch
// set it is guaranteed to find nothing and to see an unpriced position still
// scoring — that IS the pre-change behaviour, not a defect — and a diagnostic
// that exits non-zero on its own control arm makes the exit status unreadable for
// the confinement half, which is the half the hatch process exists to produce.
// `docs/replay.md` sells an exit status you can trust. With a hatch set the same
// loop LOGS instead, which is what turns "Score was already zero for this
// population by another route" from an assertion into a measurement.
func TestDiagEngineSeasonRules(t *testing.T) {
	requireDiag(t)
	cc := loadConfig(t)

	t.Logf("data state: FPL_NO_SEASON_SCORING_RULES=%q FPL_NO_UNPRICED_POSITION_GUARD=%q",
		os.Getenv("FPL_NO_SEASON_SCORING_RULES"), os.Getenv("FPL_NO_UNPRICED_POSITION_GUARD"))

	t.Run("liveness", func(t *testing.T) { engineRulesLiveness(t, cc) })
	t.Run("confinement", func(t *testing.T) { engineRulesConfinement(t, cc) })
}

// engineRulesHatched reports whether either escape hatch is set, which is what
// turns the liveness arm from an assertion into a log. Read from the environment
// rather than from `internal/analysis`, whose vars are unexported.
func engineRulesHatched() bool {
	return os.Getenv("FPL_NO_SEASON_SCORING_RULES") != "" ||
		os.Getenv("FPL_NO_UNPRICED_POSITION_GUARD") != ""
}

// engineRulesLiveness counts what the two changes reach on the engine's own
// output column, before any transfer decision is taken.
//
// It is deliberately cheap — it builds engines and scores players, and replays
// nothing — because a liveness arm answers "did it arrive", not "what is it
// worth". The confinement arm below is the expensive one.
func engineRulesLiveness(t *testing.T, cc config.Config) {
	pairs := sweepPairNames()
	// The grid's own entry points, taken from `sweepStarts` rather than written out
	// — `TestTheGridIsDeclaredOnce` is what stops a diagnostic quietly measuring a
	// different population from the sweep it is quoted beside — plus two cutoffs
	// this arm needs and the grid has no reason to carry.
	//
	// **0** is pre-season, where `PointInTime` hands over to `PreSeason` and the
	// engine is built entirely on the prior season's aggregates.
	//
	// **37** is kept because a first pass found FPL's assistant managers only
	// there. ⚠️ **That was a sampling artefact and the sampled figure is 26**: the
	// first pass probed 1, 5, 10, 20 and 37 and missed the entry point in between.
	// On this grid it is 0 at 1, 6, 11, 16 and 21, and 20 at both 26 and 37.
	//
	// ⚠️ **26 is not the onset either, and a second version of this comment
	// claimed it was.** 26 is the earliest point on the SHIPPED GRID that carries
	// them, which is the reachability claim that matters — the replay really picks
	// a squad there. The onset is `through` **23**, read off the archive rather
	// than off a probe: their first `merged_gw.csv` rows are GW23, and
	// `registeredBy` admits a player from his first row. No grid entry point
	// samples 23, which is why probing kept returning whichever of {26, 37} it
	// checked. Dropping a cutoff that once mattered is how a population goes
	// quiet, so 37 stays.
	cutoffs := append([]int{0}, sweepStarts()...)
	cutoffs = append(cutoffs, 37)

	pinMoved, guardMoved := 0, 0
	guardBySeason := map[string]int{}
	pinBySeason := map[string]int{}
	// The mediator that matters. `BaseXP90` is where the rule enters; `Score` is
	// what the optimiser orders on, and the two are not the same population — a
	// keeper whose minutes reliability and appearance factor are ~0 has a moved
	// base and a `Score` that is identically zero in both arms, so no ordering
	// change is possible for him. Counting only the first would report an upper
	// bound as if it were the population.
	scoreMoved := 0
	var maxBase, maxScore float64
	var maxBaseWho, maxScoreWho string
	for _, p := range pairs {
		prior, err := Load(context.Background(), cc.CacheDir, p[0])
		if err != nil {
			t.Skipf("archive unavailable: %v", err)
		}
		cur, err := Load(context.Background(), cc.CacheDir, p[1])
		if err != nil {
			t.Skipf("archive unavailable: %v", err)
		}
		for _, through := range cutoffs {
			sc := sweepConfig(cc, 1, true)
			boot, fx := PointInTime(cur, prior, through)
			// ⚠️ **Wired the way `Simulate` wires it, and this was wrong first.**
			// The first version built both engines with `NewEngineFull` and nothing
			// else, and `TestEveryScoringEngineGetsRecency` caught it. That is not a
			// tidiness failure here: `Engine.Priors` moves every per-90 rate through
			// `blendRates`, and the rate this arm measures the pin against IS one of
			// them — a keeper whose season xG is zero can carry a non-zero blended
			// rate from his prior season, and an unwired engine would have counted a
			// different population from the one the replay scores. **The first
			// version counted 14 player-cutoffs; correctly wired it is 552** — a 40x
			// understatement, in the direction that flattered the confinement result.
			pinned := analysis.NewEngineFull(boot, fx, cc.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			pinned.Priors = sc.priors(cur, prior)
			pinned.Recent = sc.recentIndex(cur, through)
			pinned.TeamForm = newTeamFormIndex(cur, through)

			// The same bootstrap with its season removed, which is exactly what the
			// engine saw before this change: today's table for every season. Wired
			// from the pinned engine's own sources rather than rebuilt, so the ONLY
			// difference between the two is the table.
			flat := *boot
			flat.Season = ""
			unpinned := analysis.NewEngineFull(&flat, fx, cc.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			unpinned.Priors = pinned.Priors
			unpinned.Recent = pinned.Recent
			unpinned.TeamForm = pinned.TeamForm

			for i := range boot.Elements {
				el := &boot.Elements[i]
				if !pinned.ScoringRules().Prices(el.ElementType) {
					// The guard's own population. `Prices` is false, so `Metrics`
					// refuses the priced half — and what it refused was
					// `appearancePoints`, not nothing.
					m := pinned.Metrics(el)
					switch {
					case engineRulesHatched():
						// The hatch process MEASURES the pre-change behaviour rather
						// than failing on it, which is also the only place the
						// "Score was already zero by another route" claim stops being
						// something reached by reading code.
						if guardMoved < 3 {
							t.Logf("  PRE-CHANGE unpriced element_type %d %s %s @%d: "+
								"BaseXP90 %v Score %v minutes %d",
								el.ElementType, p[1], m.Name, through, m.BaseXP90,
								m.Score, m.Minutes)
						}
					case m.BaseXP90 != 0 || m.Score != 0:
						t.Errorf("%s @%d: %s is an unpriced element_type %d and still "+
							"scored BaseXP90 %v / Score %v", p[1], through, m.Name,
							el.ElementType, m.BaseXP90, m.Score)
					}
					guardMoved++
					guardBySeason[fmt.Sprintf("%s@%d", p[1], through)]++
					continue
				}
				pm := pinned.Metrics(el)
				um := unpinned.Metrics(&flat.Elements[i])
				if d := math.Abs(pm.BaseXP90 - um.BaseXP90); d > 0 {
					pinMoved++
					pinBySeason[p[1]]++
					if d > maxBase {
						maxBase, maxBaseWho = d, fmt.Sprintf("%s %s @GW%d", p[1], pm.Name, through)
					}
					if pinMoved <= 8 {
						t.Logf("  pin moves %s %s @GW%d element_type %d: base %.6f against %.6f",
							p[1], pm.Name, through, el.ElementType, pm.BaseXP90, um.BaseXP90)
					}
				}
				if d := math.Abs(pm.Score - um.Score); d > 0 {
					scoreMoved++
					if d > maxScore {
						maxScore, maxScoreWho = d, fmt.Sprintf("%s %s @GW%d", p[1], pm.Name, through)
					}
				}
			}
		}
	}

	for _, k := range sortedCountKeys(pinBySeason) {
		t.Logf("  pin, by played season: %s %d", k, pinBySeason[k])
	}
	for _, k := range sortedCountKeys(guardBySeason) {
		t.Logf("  guard, by played season and cutoff: %s %d", k, guardBySeason[k])
	}
	t.Logf("LIVENESS season pin:   %d player-cutoffs whose BaseXP90 differs from today's table, "+
		"max |dBaseXP90| %.6f (%s)", pinMoved, maxBase, maxBaseWho)
	t.Logf("LIVENESS season pin:   %d player-cutoffs whose SCORE differs — the mediator the "+
		"optimiser orders on — max |dScore| %.6f (%s)", scoreMoved, maxScore, maxScoreWho)
	t.Logf("LIVENESS unpriced guard: %d player-cutoffs refused, each of which scored "+
		"BaseXP90 = appearancePoints before", guardMoved)

	// The hatch process is a control arm and must not fail. See this file's header.
	if engineRulesHatched() {
		t.Log("a hatch is set: the assertions below are skipped, because finding " +
			"nothing IS the pre-change behaviour and a control arm that exits " +
			"non-zero makes the confinement half's exit status unreadable")
		return
	}

	// ⚠️ FAILURES, not logs. A liveness arm that finds nothing reads exactly like a
	// change that does nothing, and the confinement result below is worthless
	// without it — which is the whole reason this record demands the pair.
	if pinMoved == 0 {
		t.Errorf("the season pin changed no player's base expected points anywhere "+
			"on %s. Either the pin is not reaching the engine, or the only rule it "+
			"amends has no mediator in this archive — check ScoringRulesFor and "+
			"Boot.Season before reading the confinement result below as confinement",
			gridLabel(len(pairs), len(cutoffs)))
	}
	if guardMoved == 0 {
		t.Error("no unpriced element_type was refused anywhere on the grid. FPL's " +
			"2024-25 assistant managers are element_type 5 and enter the replay's " +
			"bootstrap from through 26 — if the loader or PointInTimeWith now filters " +
			"them, say so there rather than letting this go quiet")
	}
}

// engineRulesConfinement replays the six-season grid and prints the three
// quantities that must NOT move: what the policy scored, what the held opening
// fifteen scored, and which fifteen that was.
//
// One line per cell, sorted, so the two processes' outputs diff cleanly.
//
// ⚠️ **`WeeklyXI` is run BOTH ways, in one process, and that is not decoration.**
// `runPolicySweep` — which produced the `POLICY` cells banked under
// `stats/snapshots/` — calls `sweepConfig(cfg, start, false)`, while a diagnostic
// that wants the eleven a manager would actually field runs `true`. The two pick
// a different eleven every week, so a null at one is a **simple-effect null** and
// says nothing about the other, exactly as `CLAUDE.md` records for the banking
// arm. Running both here rather than behind an environment switch keeps the
// answer in one output and adds no global to fingerprint.
func engineRulesConfinement(t *testing.T, cc config.Config) {
	var lines []string
	// ⚠️ `sweepPairNames`, not `extendedPairNames`, and the same function the
	// liveness arm uses. They are identical at the default grid and diverge the
	// moment anyone sets FPL_SWEEP_SEASONS — at which point the two halves of this
	// diagnostic would describe different populations while printing one label
	// each. One quantity, two implementations, inside the diagnostic built to
	// prove a wiring claim.
	for _, p := range sweepPairNames() {
		prior, err := Load(context.Background(), cc.CacheDir, p[0])
		if err != nil {
			t.Skipf("archive unavailable: %v", err)
		}
		cur, err := Load(context.Background(), cc.CacheDir, p[1])
		if err != nil {
			t.Skipf("archive unavailable: %v", err)
		}
		for _, start := range sweepStarts() {
			for _, weeklyXI := range []bool{true, false} {
				sc := sweepConfig(cc, start, weeklyXI)
				res, err := Simulate(cur, prior, sc)
				if err != nil {
					t.Fatalf("%s @%d weeklyxi=%v: %v", p[1], start, weeklyXI, err)
				}
				held := Hold(cur, prior, sc, res.OpeningSquad)
				lines = append(lines, fmt.Sprintf(
					"CELL %s start=%02d weeklyxi=%-5v policy_points=%d hold_points=%d squad_hash=%s",
					p[1], start, weeklyXI, res.Points, held, squadHash(res.OpeningSquad)))
			}
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		t.Log(l)
	}
	t.Logf("CONFINEMENT: %s, each run at WeeklyXI true and false",
		gridLabel(len(sweepPairNames()), len(sweepStarts())))
}

// sortedCountKeys is a stable iteration order for the per-season breakdowns, so
// two processes' logs diff. Named apart from this package's existing sortedKeys,
// which is over a different map type.
func sortedCountKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
