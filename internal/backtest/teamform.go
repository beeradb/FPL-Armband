package backtest

import (
	"os"
	"sort"

	"armband/internal/analysis"
)

// The replay's supplier for the club-form blend. See internal/analysis/teamform.go
// for what the blend is, what it measured, and why it is off by default.
//
// # Point-in-time, and the one way this could leak
//
// Everything here reads `p.GWs[gw]` for gw at or before the cutoff, which is the
// rule `PointInTime` applies to every other input. The hazard worth naming is the
// *window*: a trailing window ending at the cutoff is safe, and one centred on it
// would not be. `TestTeamFormIsPointInTime` pins that a source built at gameweek G
// is identical whether or not the season's later rows exist.
//
// # Matches, not gameweeks
//
// A double gameweek is two matches of expected goals in one row and a blank is no
// row at all, so dividing by gameweeks would read a club with a double as having
// found form. That is the units error this project has already shipped once, when a
// 22-minute cameo read as 8.18 bonus a gameweek. The denominator is club matches,
// taken from `clubGameweeks`, which is this package's one implementation of "how
// many matches did this club play that gameweek".

// teamFormIndex answers analysis.TeamFormSource from the archive.
type teamFormIndex struct {
	recent        map[int]float64
	season        map[int]float64
	recentMatches map[int]int
	seasonMatches map[int]int
}

// TeamForm is the club's expected goals per match over the trailing window and over
// the season to date, along with the match counts in each window.
func (t teamFormIndex) TeamForm(teamID int) (recent, season float64, recentMatches, seasonMatches int, ok bool) {
	r, okR := t.recent[teamID]
	s, okS := t.season[teamID]
	rm, okRM := t.recentMatches[teamID]
	sm, okSM := t.seasonMatches[teamID]
	if !okR || !okS || !okRM || !okSM || r <= 0 || s <= 0 {
		return 0, 0, 0, 0, false
	}
	return r, s, rm, sm, true
}

// teamFormWindow is how many gameweeks the trailing window covers.
//
// Nine, because that is the window the prediction result was measured over — the
// diagnostic fits at GW19 and uses GW20-28 to predict GW29-38. Choosing a different
// one here would mean the replay was arbitrating something the prediction work
// never measured, and the temptation to tune it is exactly the argmax this project
// keeps warning about.
const teamFormWindow = 9

// minTeamFormMatches is how many matches a club needs in each window before its
// ratio is used. Four is half the window: below it the denominator is a handful of
// games and the ratio is noise about noise.
const minTeamFormMatches = 4

// newTeamFormIndex builds the source as of `through`, over a trailing window of
// teamFormWindow gameweeks.
//
// Returns nil when the blend is off, so the engine keeps the shipped path exactly
// rather than taking a neutral-but-different one. A nil source is the same
// statement the Recent hook makes when it cannot be built, and `teamFormFactor`
// short circuits on it.
func newTeamFormIndex(s *Season, through int) analysis.TeamFormSource {
	if analysis.TeamFormWeight() <= 0 || through < 1 {
		return nil
	}
	lo := through - teamFormWindow + 1
	if lo < 1 {
		lo = 1
	}

	// Expected goals summed per club, and the club's match count per gameweek.
	//
	// `GW.Fixtures` is how many matches the player's club played that gameweek, so
	// any one of a club's rows carries it and the maximum across them is the club's
	// count — robust to a player who has no row because he did not feature.
	//
	// **Walked in sorted order, and the first version of this was not.** The
	// reasoning for skipping it was that this only accumulates and addition
	// commutes — true in arithmetic and false in binary floating point, which is
	// the caveat AGENTS.md attaches to exactly this claim. Ranging a map summed the
	// same expected goals in a different order every run and the totals differed in
	// the last bits. `TestTeamFormIsPointInTime` caught it by reporting two figures
	// that printed identically to six decimal places and compared unequal.
	//
	// On a response surface where a 2% nudge to one exponent moves four-season
	// points by 67, a last-bit difference is not cosmetic: it can flip a squad
	// comparison and make a replay non-reproducible, which is the property the
	// whole harness rests on.
	ids := make([]int, 0, len(s.Players))
	for id := range s.Players {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	rx, sx := map[int]float64{}, map[int]float64{}
	played := map[int]map[int]float64{}
	for _, id := range ids {
		p := s.Players[id]
		// Gameweeks in order too, for the same reason: p.GWs is a map, and the
		// expected-goals sum is the quantity whose last bits matter. The fixture
		// counts below are small integers and exact in float64 at any order, so
		// only this sum needed it — but both are walked in order rather than
		// leaving the next reader to work out which half was safe.
		for gw := 1; gw <= through; gw++ {
			g, ok := p.GWs[gw]
			if !ok {
				continue
			}
			sx[p.Team] += g.XG
			if gw >= lo {
				rx[p.Team] += g.XG
			}
			fx := float64(g.Fixtures)
			if fx < 1 {
				fx = 1
			}
			if played[p.Team] == nil {
				played[p.Team] = map[int]float64{}
			}
			if fx > played[p.Team][gw] {
				played[p.Team][gw] = fx
			}
		}
	}

	rm, sm := map[int]float64{}, map[int]float64{}
	for team, gws := range played {
		for gw, fixtures := range gws {
			sm[team] += fixtures
			if gw >= lo {
				rm[team] += fixtures
			}
		}
	}

	// Mean attacking ease over each window, on the shipped ladder, so the two
	// figures can be compared at neutral difficulty.
	//
	// **This is the difference between measuring form and measuring the fixture
	// list.** A club's recent output correlates with the ease of the fixtures that
	// produced it at +0.407, so a raw recent-against-season ratio is substantially
	// a statement about who it happened to play — and fixtures revert by
	// construction, because everyone plays everyone, so that component predicts
	// nothing. Worse, the model already applies a per-fixture multiplier
	// downstream, so feeding the fixture component in here would count it twice.
	//
	// Measured: dividing each window by its own ease improves the blend's
	// prediction of the next stretch from 0.2390 to 0.2339. Both windows are put on
	// neutral terms and the model's own ladder then handles the fixtures ahead, as
	// it already does.
	re, se := map[int]float64{}, map[int]float64{}
	rn, sn := map[int]float64{}, map[int]float64{}
	for _, f := range s.Fixtures {
		if f.Event == nil || *f.Event < 1 || *f.Event > through {
			continue
		}
		for team, d := range map[int]int{f.TeamH: f.TeamHDifficulty, f.TeamA: f.TeamADifficulty} {
			e := analysis.AttackMultiplier(d)
			se[team] += e
			sn[team]++
			if *f.Event >= lo {
				re[team] += e
				rn[team]++
			}
		}
	}
	ease := func(sum, n map[int]float64, team int) float64 {
		if !teamFormAdjusted || n[team] == 0 || sum[team] <= 0 {
			return 1
		}
		return sum[team] / n[team]
	}

	idx := teamFormIndex{
		recent:        map[int]float64{},
		season:        map[int]float64{},
		recentMatches: map[int]int{},
		seasonMatches: map[int]int{},
	}
	for team, m := range sm {
		if m >= minTeamFormMatches {
			idx.season[team] = sx[team] / m / ease(se, sn, team)
			idx.seasonMatches[team] = int(m)
		}
	}
	for team, m := range rm {
		if m >= minTeamFormMatches {
			idx.recent[team] = rx[team] / m / ease(re, rn, team)
			idx.recentMatches[team] = int(m)
		}
	}
	return idx
}

// teamFormAdjusted divides each window by the ease of the fixtures it contained,
// so the ratio the engine takes is form at neutral difficulty rather than form
// plus the fixture list.
//
// On by default when the blend is on, because it measured better. FPL_TEAM_FORM_RAW=1
// restores the unadjusted version, which is worth keeping armable: the two differ by
// exactly the fixture component, so running both is what separates "tracking the club
// pays" from "the fixture ladder is mis-scaled" — and this project has five recorded
// cases of the second dressed as the first.
var teamFormAdjusted = os.Getenv("FPL_TEAM_FORM_RAW") == ""
