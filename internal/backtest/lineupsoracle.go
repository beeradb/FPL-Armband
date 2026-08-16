package backtest

// The lineups oracle: OracleLineups tells the model who is picked, and nothing
// about how long he stays on.
//
// # Why the minutes oracle had to be split, and what each half bounds
//
// `OracleMinutes` grants two facts at once, and only one of them is a capability
// anybody could build:
//
//   - **Selection.** Did he start, come off the bench, or not feature. This is
//     the half a real system can partly acquire — press conferences, injury
//     reporting, rotation patterns, a manager's pre-match noises. It is the same
//     quantity the judgement layer exists to supply, and the same quantity
//     `FPL_ORACLE_AVAILABILITY` reaches a corner of.
//   - **Quantity given selection.** The 62nd-minute hook, the 20-minute cameo, the
//     full ninety. **Nobody knows this at the deadline even in principle.** A
//     manager does not know it; the manager making the substitution does not know
//     it before the match.
//
// So `OracleLineups` grants the first and prices the second at its conditional
// average, and `OracleMinutes` grants both. **The subtraction is the point.**
// Lineups is the reachable part; minutes-minus-lineups is the irreducible
// residual nobody could ever have bought. Reporting one number for the pair
// invites reading the whole of it as headroom — which is precisely the error the
// armband oracle's decomposition corrected, where +210 turned out to be mostly an
// unreachable order statistic rather than a target.
//
// # Both arms travel down the minutes channel, and that is a finding about the model
//
// The obvious implementation of a lineups oracle is to rewrite `StartShare` and
// leave `MinutesPerMatch` alone. **It would be a silent no-op.** Nothing in the
// shipped scoring model consumes start information at all:
//
//   - `reliabilityFrom` (internal/analysis/metrics.go) reads `StartShare` and
//     weights it by `reliabilityMinutesShare`, which ships at **1.0**
//     (internal/analysis/sweep.go). The expression is
//     `w*minutesShare + (1-w)*startShare`, so the start term is multiplied by
//     exactly zero. Live-looking dead code.
//   - `appearanceOdds` (internal/analysis/appearance.go) reads `StartShare` only
//     in its legacy branch. `unifiedAppearance` defaults on and the live path is
//     `playsAtAll(m.ExpectedMinutes)` — minutes again.
//   - `StartShare` survives as a *reported* field the agent reads, and in blend
//     plumbing. Nothing scores off it.
//
// An arm expressed through `StartShare` would therefore stamp itself, change
// nothing, and report a clean null — the exact failure the oracle harness's
// liveness tier exists to catch. Both arms consequently travel down
// `MinutesPerMatch`, at different **resolutions**, which preserves the
// information distinction exactly: lineups knows the state and prices it at the
// conditional mean, minutes knows the realised value.
//
// It is worth recording what that dead channel means on its own, because nothing
// in the model states it: **the model cannot distinguish a starter hooked at 70
// every week from an ever-present who misses one game in four.** Identical
// minutes per match, completely different in FPL terms — the first banks two
// appearance points and clears the sixty-minute clean-sheet threshold every week
// and never triggers an autosub; the second banks nothing in a quarter of his
// gameweeks and is the reason a bench exists.
//
// # Where the two arms should separate, as a prediction rather than an assumption
//
// Not on the rate terms. Goals, assists, defensive contribution, bonus and saves
// all genuinely scale with time on the pitch, so pricing a start at its
// conditional average instead of its realised value moves them by the residual
// and no more.
//
// On the **threshold** terms. Appearance points are two separate one-point
// awards, at the first minute and at sixty, and the clean sheet keys off the same
// sixty-minute mark. So "did he reach sixty" is a single binary that flips both
// at once, and it is exactly the thing a conditional average smooths away: a
// player who alternates 90 and 40 has the same mean as one who plays 65 every
// week and collects the sixty-minute award half as often.
//
// **If the two arms do not separate, that is a finding about the model rather
// than a failed build** — it would say the threshold split is not where the
// remaining value is.
//
// # The conditional averages are measured, and their provenance is recorded
//
// Not symmetric, not invented. Over the archive, a start is 83 to 87 minutes and
// a substitute appearance is **18**, with position-level means of about 89 (GKP),
// 85 (DEF), 80 (MID) and 79 (FWD) for starts and 17 to 19 for outfield
// substitutes. A substitute who regularly plays 70 minutes is not a real player,
// and a conditional average implying one is a bug; `TestConditionalMinutesAreRealistic`
// pins the bands.
//
// They come from the player's own record where the sample supports it and fall
// back to his position and then to the league, and `conditionalMinutes.source`
// says which — because a per-player mean on two appearances is a worse estimate
// than the position mean on ten thousand, and the resolution is not visible from
// the number.
//
// # One archive caveat, which lands squarely on this oracle
//
// `merged_gw.csv` records no starts for 2022-23 through GW15, and
// `reconstructStarts` infers them by ranking minutes within a club-gameweek. Its
// recorded boundary two is *"never for fitting a start / substitute / unused
// multinomial"*, and boundary three is *"never as evidence about an individual
// rotation or returning player"* — which is precisely what this oracle consumes.
// The error is 2.36% of starter slots and is biased by role: a nailed starter
// withdrawn at half time and a fringe player eased back in at half time both play
// 45 and tie.
//
// Two responses, both deliberate. The conditional averages are accumulated from
// **non-reconstructed rows only** where those exist, so the prices themselves are
// clean. The *classification* of a future gameweek still uses the reconstructed
// flag, because the alternative — a minutes threshold — measures three times
// worse. The diagnostic counts the exposure per cell rather than hiding it: it is
// confined to one of four seasons and to the first fifteen gameweeks of it.

import "armband/internal/analysis"

// minConditionalSample is how many appearances in a state a player needs before
// his own conditional average is used instead of his position's.
//
// **Asserted, not measured.** Three is a floor on "this is not one afternoon",
// and the estimate it guards is a mean of a tight distribution — starts are
// 83-87 minutes across every season and position, so the per-player refinement is
// small either way and the cost of getting the threshold wrong is correspondingly
// small. It is reported by source rather than assumed away: the diagnostic prints
// how many players resolve at each level.
const minConditionalSample = 3

// conditionalMinutes is what a player plays given that he is picked, in each of
// the two states the archive can distinguish.
//
// The two states resolve **independently**, and that is not a detail. A nailed
// starter has thirty starts and no substitute appearances at all, so requiring
// one level to supply both would push every such player onto the position mean
// for the state he has most evidence about — which is the wrong way round, and
// was the first version's behaviour.
type conditionalMinutes struct {
	start float64
	sub   float64
	// startSource and subSource are "player", "position" or "league": which
	// sample each number came from. Carried rather than inferred, because a
	// per-player mean on three appearances and a league mean on thirty thousand
	// are different estimates and nothing about the value says which one it is.
	startSource string
	subSource   string
}

// meanSample is a running mean.
type meanSample struct {
	sum float64
	n   int
}

func (s *meanSample) add(v float64) { s.sum += v; s.n++ }

// resolve returns the clean sample's mean where it has support, the full
// sample's where it does not, and false when neither does.
//
// Two pools rather than one because 2022-23's reconstructed starts are usable in
// aggregate and explicitly unusable as individual evidence — so the *prices* are
// built from rows the archive actually recorded, and the reconstructed rows are
// only a fallback for a season that has nothing else.
func resolve(clean, all meanSample) (float64, bool) {
	if clean.n >= minConditionalSample {
		return clean.sum / float64(clean.n), true
	}
	if all.n >= minConditionalSample {
		return all.sum / float64(all.n), true
	}
	return 0, false
}

// statePools is the raw evidence for one player, one position or the league.
type statePools struct{ startClean, startAll, subClean, subAll meanSample }

// conditionalTable is every level of conditional average a season supports.
type conditionalTable struct {
	byCode map[int]*statePools
	byPos  map[int]*statePools
	league statePools
}

// newConditionalTable measures what a start and a substitute appearance are
// worth, per player, per position and league-wide.
//
// **Single-fixture gameweeks only.** The archive records starts and minutes per
// *gameweek*, not per match, so in a double there is no way to split 130 minutes
// between a start and a substitute appearance. Doubles are 10 to 42 team
// gameweeks a season against about 380 single ones, so dropping them costs
// nothing and keeps the two conditional means uncontaminated by each other.
func newConditionalTable(s *Season) conditionalTable {
	out := conditionalTable{byCode: map[int]*statePools{}, byPos: map[int]*statePools{}}
	byCode, byPos := out.byCode, out.byPos

	add := func(p *statePools, start bool, mins float64, clean bool) {
		switch {
		case start && clean:
			p.startClean.add(mins)
			p.startAll.add(mins)
		case start:
			p.startAll.add(mins)
		case clean:
			p.subClean.add(mins)
			p.subAll.add(mins)
		default:
			p.subAll.add(mins)
		}
	}

	for _, pl := range s.Players {
		if pl.Code == 0 {
			continue
		}
		if byCode[pl.Code] == nil {
			byCode[pl.Code] = &statePools{}
		}
		if byPos[pl.Type] == nil {
			byPos[pl.Type] = &statePools{}
		}
		for gw := 1; gw <= 38; gw++ {
			g, ok := pl.GWs[gw]
			if !ok || g.Fixtures != 1 || g.Minutes <= 0 {
				continue
			}
			start := g.Starts > 0
			clean := !g.StartsReconstructed
			mins := float64(g.Minutes)
			add(byCode[pl.Code], start, mins, clean)
			add(byPos[pl.Type], start, mins, clean)
			add(&out.league, start, mins, clean)
		}
	}
	return out
}

// forPlayer is the most specific conditional average this season supports, each
// state resolved on its own evidence: the player's own where he has enough of it,
// else his position's, else the league's.
func (c conditionalTable) forPlayer(code, pos int) (conditionalMinutes, bool) {
	levels := []struct {
		name string
		p    *statePools
	}{
		{"player", c.byCode[code]},
		{"position", c.byPos[pos]},
		{"league", &c.league},
	}
	var out conditionalMinutes
	for _, lv := range levels {
		if lv.p == nil {
			continue
		}
		if out.startSource == "" {
			if v, ok := resolve(lv.p.startClean, lv.p.startAll); ok {
				out.start, out.startSource = v, lv.name
			}
		}
		if out.subSource == "" {
			if v, ok := resolve(lv.p.subClean, lv.p.subAll); ok {
				out.sub, out.subSource = v, lv.name
			}
		}
	}
	return out, out.startSource != "" && out.subSource != ""
}

// leagueMinutes is what a start and a substitute appearance are worth across the
// whole season, ignoring who it was. The coarsest rung, and the one the
// diagnostic prints so the prices are auditable rather than merely applied.
//
// It is also the only "does this season support any prices at all" predicate, and
// deliberately the only one: a season with no football produces no league sample,
// `forPlayer` returns false for everybody, and the arm emits nothing — which the
// sweep's liveness check reports as an inert arm rather than as a clean null. An
// earlier version carried a separate `leagueKnown` flag that no production path
// consulted, which is two expressions of one predicate with one of them dead.
func (c conditionalTable) leagueMinutes() (conditionalMinutes, bool) {
	// No player carries code or position -1, so both specific rungs miss and the
	// walk lands on the league sample.
	return c.forPlayer(-1, -1)
}

// lineupCoverage restricts which players an oracle arm is allowed to know about.
// nil means every player, which is the shipped behaviour.
//
// It exists so a *restricted* arm can be measured without a second copy of the
// oracle's construction. The record's standing rule is that a diagnostic must
// never carry its own copy of the thing it is checking, and this family has
// already paid for that twice.
//
// Why restriction is worth measuring at all: an arm that touches every player
// every week is a DENSE arm, and dense arms on this harness need 135-239 points a
// season to resolve. A restricted arm is sparse, and the sparse availability arm
// needed 14. So a question posed as "where does the value come from" is about an
// order of magnitude easier to answer here than the same question posed as "how
// much is it worth in total".
type lineupCoverage func(p *Player, statusAtCutoff string) bool

// newOracleLineupsCovering builds the hindsight *selection* view for a decision
// taken with data through gameweek `through`, optionally restricted to the players
// `covers` admits. A nil `covers` is the unrestricted arm.
//
// It shares `selectionOver` with the minutes oracle, which is deliberate: the two
// arms must classify the same window the same way or their difference measures
// the classifier rather than the residual. The only thing that differs is what a
// classified fixture is priced at.
func newOracleLineupsCovering(s *Season, through, window int, base analysis.RecentForm,
	covers lineupCoverage) analysis.RecentForm {
	cal := fixtureCalendar(s)
	cond := newConditionalTable(s)
	// Hoisted: gameweekStart scans every fixture in the season, and this loop runs
	// over every player, for every engine, of every gameweek, of every cell.
	cutoff := gameweekStart(s, through+1)
	future := map[int]analysis.RecentPlayer{}
	for _, p := range s.Players {
		if p.Code == 0 {
			continue
		}
		sel := selectionOver(p, cal[p.Team], through+1, through+window)
		if sel.fixtures == 0 {
			continue
		}
		cm, ok := cond.forPlayer(p.Code, p.Type)
		if !ok {
			continue
		}
		// The restriction is applied on the honest reconstruction, with no oracle
		// of its own: a restricted arm must be defined by what the model could
		// have known at the cutoff, not by hindsight about who to be right about.
		//
		// **The gameweek here is load-bearing and a refactor can break it in
		// silence.** `through+1` is the same gameweek `PointInTimeWith` evaluates
		// `statusAt` at when it builds the bootstrap, so the coverage test and the
		// status the model actually sees agree. Test it against any other week of
		// the window and the two restricted arms stop partitioning — a player
		// flagged at the cutoff but not at `through+3` would be admitted by both
		// arms, or by neither, and their sum would no longer be the unrestricted
		// arm. That the sum reproduces the unrestricted arm digit for digit is the
		// check that this still holds.
		if covers != nil && !covers(p, statusAt(p, through+1, cutoff, Oracles{})) {
			continue
		}
		// `BlankRun` is deliberately left at its zero value, and that is a second
		// intervention rather than an omission: `known.BlankRun = f.BlankRun` in
		// minutesoracle.go copies it across, so every covered player has his blank
		// run erased and `blankRunFactor` — which ships ON — stops applying to him.
		//
		// It is the right behaviour for an arm that knows the selection outcome: a
		// blank run is a *proxy* for "he may have quietly stopped playing", and an
		// oracle that has been told whether he plays does not need the proxy and
		// would be double-counting if it kept it. But it means the arm is
		// "selection knowledge AND no blank-run penalty" rather than selection
		// knowledge alone, and a reader comparing it against the shipped model is
		// comparing two changes. Stated here because it is invisible at the call
		// site.
		future[p.Code] = analysis.RecentPlayer{
			MinutesPerMatch: (sel.starts*cm.start + sel.subs*cm.sub) / sel.fixtures,
			StartShare:      sel.starts / sel.fixtures,
			Matches:         int(sel.fixtures),
		}
	}
	return oracleFutureIndex{base: base, future: future}
}
