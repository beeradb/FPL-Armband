package backtest

import (
	"fmt"
	"math/rand"
	"os"
	"sort"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// viceCaptainFallback gates the armband passing to the vice-captain when the
// captain blanks. Set FPL_NO_VICE_CAPTAIN=1 to restore the old behaviour,
// which simply forfeited the bonus, for re-measurement.
var viceCaptainFallback = os.Getenv("FPL_NO_VICE_CAPTAIN") == ""

// wildcardBuildsForBoost decides whether a wildcard played the week BEFORE
// a bench boost optimises the fifteen for the chip (all fifteen counted) or
// builds an ordinary squad. It exists to separate the cost of the *wildcard*
// from the cost of *building for the boost*, which the first sequence
// measurement conflated. Set FPL_WC_IGNORES_BOOST=1 to build ordinarily.
var wildcardBuildsForBoost = os.Getenv("FPL_WC_IGNORES_BOOST") == ""

// legalAutosubs gates checking formation legality before making a substitution.
// Set FPL_NO_LEGAL_AUTOSUBS=1 to restore the old behaviour, which substituted
// in bench order regardless — including an outfielder for a blanking keeper.
var legalAutosubs = os.Getenv("FPL_NO_LEGAL_AUTOSUBS") == ""

// PreSeason reconstructs what FPL showed before a season kicked off: this
// season's squads and opening prices, carrying last season's statistics.
//
// That combination is the whole point. Pre-season, FPL's bootstrap holds the
// previous season's totals against the new season's rosters and prices, and
// that is exactly the evidence a manager has when picking a GW1 squad. A replay
// built from end-of-season numbers would be reading the answers.
//
// Players with no prior — promoted clubs, arrivals from abroad — carry zeros,
// which is also what FPL shows and what the model has to cope with.
// `cur` must be playable and `prior` need not be: this reads only season aggregates
// from the prior, which is exactly why a season with no teams.csv can still be one.
//
// The inert wrapper, as PointInTime is to PointInTimeWith: no hindsight at all.
func PreSeason(cur, prior *Season) (*fpl.Bootstrap, []fpl.Fixture) {
	return PreSeasonWith(cur, prior, Oracles{})
}

// PreSeasonWith is PreSeason with an oracle state. It is the second half of the
// information seam — see PointInTimeWith — and reaches the pre-season view, which
// is the one an opening-fifteen decision is made from.
func PreSeasonWith(cur, prior *Season, o Oracles) (*fpl.Bootstrap, []fpl.Fixture) {
	mustBePlayable(cur)
	priorByCode := prior.ByCode()

	b := &fpl.Bootstrap{
		// See the identical line in PointInTimeWith: the season is what pins the
		// engine's scoring rules, and a pre-season bootstrap needs it as much as a
		// mid-season one — the opening fifteen is picked through the same terms.
		Season: cur.Name,
		Teams:  cur.Teams,
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: fmt.Sprintf("Gameweek %d", i)})
	}

	// Only players who were in the game at GW1. A January signing appears in
	// players_raw.csv with a full season record and in no gameweek row until
	// January, so iterating the first without this gate put him in the pre-season
	// pool priced at what he was worth in May — see pool.go.
	reg := registeredBy(cur, 0)
	for _, p := range cur.Players {
		if !reg.has(p.ID) {
			continue
		}
		// Opening price, not the closing one: prices move all season and using
		// the final figure would let the replay buy a riser at its old cost.
		el := fpl.Element{
			ID: p.ID, Code: p.Code, WebName: p.WebName, ElementType: p.Type,
			Team: p.Team, NowCost: reg.price(p, 1), Status: statusAt(p, 1, gameweekStart(cur, 1), o),
		}
		if q := priorByCode[p.Code]; q != nil {
			el.Minutes, el.Starts = q.Minutes, q.Starts
			el.TotalPoints, el.Bonus = q.TotalPoints, q.Bonus
			el.Saves, el.CleanSheets, el.GoalsConceded = q.Saves, q.CleanSheets, q.GoalsConceded
			el.GoalsScored, el.Assists = q.Goals, q.Assists
			el.YellowCards, el.RedCards = q.Yellow, q.Red
			el.ExpectedGoals = fpl.Num(q.XG)
			el.ExpectedAssists = fpl.Num(q.XA)
			el.ExpectedGoalsConceded = fpl.Num(q.XGC)
			if q.Minutes > 0 {
				per90 := 90 / float64(q.Minutes)
				el.ExpectedGoalsPer90 = fpl.Num(q.XG * per90)
				el.ExpectedAssistsPer90 = fpl.Num(q.XA * per90)
				el.ExpectedGCPer90 = fpl.Num(q.XGC * per90)
				// Pre-season carries the prior season's record, and defensive
				// contribution was not recorded before 2025-26 — so a 2025-26
				// replay opens blind to it and accumulates from GW1, which is
				// roughly the position a real manager was in that August.
				if DefconScoredIn(cur.Name) {
					// q.DefCon rather than a loop over q.GWs: the season total is
					// derived once in `repaired()` now, so this is the same number
					// with one implementation instead of two. It was an inline
					// map-order sum, which was correct only because DefCon is an
					// integer.
					el.DefensiveContribution = q.DefCon
					el.DefensiveContributionPer90 = fpl.Num(float64(q.DefCon) * per90)
				}
			}
		}
		// Gameweek 1, because pre-season *is* the run-up to the first deadline. The
		// in-season half of the seam passes `through+1` for the same reason, and
		// writing the two as one expression would need a `through` this function does
		// not have.
		applyTeamNews(&el, cur.Name, 1, o)
		b.Elements = append(b.Elements, el)
	}
	sort.Slice(b.Elements, func(i, j int) bool { return b.Elements[i].ID < b.Elements[j].ID })
	if o.Has(OracleOmniscient) {
		// through = 0: pre-season, the whole season is still to come, so this is
		// the view the control is strongest at. Same call as PointInTimeWith's, so
		// the two halves of the seam cannot disagree about which football counts as
		// hindsight.
		applyOmniscience(b, cur, 0)
	}
	return b, cur.Fixtures
}

// Result is how a squad actually did.
type Result struct {
	Label string
	// SquadPoints is every one of the fifteen players' actual returns. It
	// measures player selection alone, with no lineup decisions in it.
	SquadPoints int
	// XIPoints holds the chosen eleven all season with the captain doubled and
	// autosubs applied — a realistic floor, since a real manager would change
	// the eleven weekly.
	XIPoints int
	// ValueChange is what the fifteen were worth at the end minus the start, in
	// tenths. The model has no concept of price movement, so this measures a
	// blind spot rather than a skill.
	ValueChange int
	Squad       []int // element ids
}

// Score replays a squad across the season.
func Score(label string, s *Season, squad []analysis.PlayerMetrics, xi []analysis.PlayerMetrics, captain, vice int) Result {
	r := Result{Label: label}
	inXI := map[int]bool{}
	for _, p := range xi {
		inXI[p.ID] = true
	}

	var startVal, endVal int
	for _, m := range squad {
		p := s.Players[m.ID]
		if p == nil {
			continue
		}
		r.Squad = append(r.Squad, m.ID)
		for _, g := range p.GWs {
			r.SquadPoints += g.Points
		}
		startVal += priceAt(p, 1)
		endVal += lastValue(p)
	}
	r.ValueChange = endVal - startVal

	// Weekly: field the chosen eleven, substitute anyone who did not play,
	// double the captain.
	for gw := 1; gw <= 38; gw++ {
		var played, benched []*Player
		for _, m := range squad {
			p := s.Players[m.ID]
			if p == nil {
				continue
			}
			if inXI[m.ID] {
				played = append(played, p)
			} else {
				benched = append(benched, p)
			}
		}
		r.XIPoints += weekPoints(played, benched, gw, captain, vice)
	}
	return r
}

// weekPoints scores one gameweek, applying autosubs and the captaincy.
// chipKind is which chip is played in a gameweek, if any.
//
// Only the two that change *scoring* are here. A wildcard changes what you may
// transfer and a free hit changes which squad is fielded, so neither belongs in
// weekPoints — the free hit is already handled by excluding its gameweek from
// scoring (Engine.ApplyFreeHitToScoring).
type chipKind int

const (
	chipNone chipKind = iota
	chipBenchBoost
	chipTripleCaptain
)

func weekPoints(xi, bench []*Player, gw, captain, vice int) int {
	return weekPointsWithChip(xi, bench, gw, captain, vice, chipNone)
}

// weekTotals is one gameweek scored on both metrics from ONE set of decisions.
//
// # Why a struct rather than a second scoring function
//
// The accumulated-xPoints instrument needs "the season this squad scored, with
// the four conversion channels replaced" — the same eleven, the same autosubs,
// the same armband outcome, the same chip, with `analysis.XPoints` substituted
// per player-gameweek. A second function computing that would be a second
// implementation of the *selection* logic, which is this package's signature
// bug: `weekPointsWithChip` is where the vice-captain fallback, the legal-autosub
// formation check and the bench-boost special case actually live, and a mirror of
// it would drift on the first change to any of the three.
//
// So the decisions are made exactly once, in `weekScoreWithChip`, and both
// running totals are fed from the same branches. `weekPointsWithChip` is now a
// projection of that — `.Points` — and is byte-for-byte the same number it always
// returned, which is what keeps every existing call site and every recorded
// figure unmoved.
//
// **XPoints is float64 and Points is int, deliberately**, on `xPointsOver`'s
// reasoning: an expectation is not a score, and rounding it would quantise away
// most of what the instrument exists to measure.
type weekTotals struct {
	Points  int
	XPoints float64
}

func (t *weekTotals) add(s weekTotals) {
	t.Points += s.Points
	t.XPoints += s.XPoints
}

// addMult adds n further copies of one player's return — the armband, where n is
// 1 for an ordinary captain and 2 for a triple captain.
func (t *weekTotals) addMult(n int, s weekTotals) {
	t.Points += n * s.Points
	t.XPoints += float64(n) * s.XPoints
}

// playerWeek is one player-gameweek on both metrics.
func playerWeek(p *Player, g GW) weekTotals {
	return weekTotals{Points: g.Points, XPoints: xPointsOf(p, g)}
}

// weekScore is weekPoints on both metrics.
func weekScore(xi, bench []*Player, gw, captain, vice int) weekTotals {
	return weekScoreWithChip(xi, bench, gw, captain, vice, chipNone)
}

// weekPointsWithChip scores one gameweek under a chip.
//
// Bench boost pays all fifteen, so there is no autosub step at all: a blanking
// starter is not replaced, he simply scores nothing, and every bench player
// scores whether anyone blanked or not. Getting that wrong by keeping the
// autosub loop would double-count the substitute.
//
// Triple captain multiplies the armband by three rather than two, so the *gain*
// over an ordinary week is exactly one further copy of the captain's return —
// and nothing if he blanks, which is the risk the chip actually carries.
//
// # The vice-captain
//
// FPL passes the armband to the vice-captain whenever the captain records no
// minutes — injury, suspension, a rotation call, or simply no fixture that
// week — and this had no fallback at all: a blanking captain forfeited the
// bonus outright. `ViceCaptainWeight` (0.08) already estimates this inside the
// squad-selection *objective* ("a nailed captain blanks... perhaps one week in
// twelve"), but nothing carried it into the replay's actual scoring.
// TestDiagBlankHandling measured the model's own captain choice blanking 9.6%
// of weeks — close to that estimate — and forfeiting 261 points of vice-bonus
// across 612 gameweek-decisions, about 16 points a season. If both the
// captain and the vice blank, nobody is doubled, which is the real rule too.
func weekPointsWithChip(xi, bench []*Player, gw, captain, vice int, c chipKind) int {
	return weekScoreWithChip(xi, bench, gw, captain, vice, c).Points
}

// weekScoreWithChip is the implementation weekPointsWithChip projects. Every
// decision below — who is in the eleven, who blanked, who is legally substituted
// in, whose armband it ends up being — is made once and feeds both running
// totals. See weekTotals for why there is no second copy of this function.
func weekScoreWithChip(xi, bench []*Player, gw, captain, vice int, c chipKind) weekTotals {
	capMult := 1 // extra copies on top of the player's own return
	if c == chipTripleCaptain {
		capMult = 2
	}

	var total weekTotals
	var captainScore, viceScore weekTotals
	captainPlayed, vicePlayed := false, false
	// The eleven's position counts, blanking starters included: a player who
	// records nothing still occupies his slot until somebody legally replaces
	// him, which is what makes the formation check below meaningful.
	counts := map[string]int{}
	var blanked []string // positions of the starters who recorded no minutes
	for _, p := range xi {
		pos := analysis.PositionForElementType(p.Type)
		counts[pos]++
		g := p.GWs[gw]
		if g.Minutes == 0 {
			blanked = append(blanked, pos)
			continue
		}
		s := playerWeek(p, g)
		total.add(s)
		switch p.ID {
		case captain:
			captainScore, captainPlayed = s, true
		case vice:
			viceScore, vicePlayed = s, true
		}
	}
	switch {
	case captainPlayed:
		total.addMult(capMult, captainScore)
	case vicePlayed && viceCaptainFallback:
		total.addMult(capMult, viceScore)
	}

	if c == chipBenchBoost {
		// Every bench player scores, and nobody is substituted in.
		for _, p := range bench {
			if g := p.GWs[gw]; g.Minutes > 0 {
				total.add(playerWeek(p, g))
			}
		}
		return total
	}

	// Autosubs, in bench order, and only where FPL would actually make one.
	//
	// This used to ignore formation legality, justified as "close enough for a
	// floor, and it cannot flatter the model since every squad gets it". Both
	// halves are wrong. `bestXIWith` orders the bench with the reserve keeper
	// *last*, so a blanking goalkeeper was replaced by the highest-scoring
	// bench outfielder — a substitution FPL never makes, since the eleven must
	// contain exactly one keeper. And a blanking defender in a three-at-the-back
	// side was replaced even when every bench outfielder would take it to two,
	// where FPL makes no substitution at all. That second case is a pure
	// over-credit rather than a mis-assignment, so it does not wash out.
	if legalAutosubs {
		for _, p := range bench {
			if len(blanked) == 0 {
				break
			}
			g := p.GWs[gw]
			if g.Minutes == 0 {
				continue
			}
			in := analysis.PositionForElementType(p.Type)
			for i, out := range blanked {
				counts[out]--
				counts[in]++
				if !analysis.LegalFormation(counts) {
					counts[out]++
					counts[in]--
					continue
				}
				total.add(playerWeek(p, g))
				blanked = append(blanked[:i], blanked[i+1:]...)
				break
			}
		}
		return total
	}
	blanks := len(blanked)
	for _, p := range bench {
		if blanks == 0 {
			break
		}
		if g := p.GWs[gw]; g.Minutes > 0 {
			total.add(playerWeek(p, g))
			blanks--
		}
	}
	return total
}

// lastValue is what a player was worth at the *end* of the season, for the
// value-change column.
//
// It is the one deliberate exception to pool.go's rule that nothing falls back to
// `Player.NowCost`. Everywhere else that fallback is the leak — the closing price
// standing in for a price at some earlier gameweek. Here the closing price is the
// quantity being asked for, so `NowCost` is not a fallback at all but the exact
// answer, used when a player has no priced gameweek rows to walk.
func lastValue(p *Player) int {
	best, v := 0, p.NowCost
	for gw, g := range p.GWs {
		if gw >= best && g.Value > 0 {
			best, v = gw, g.Value
		}
	}
	return v
}

// RandomSquads returns the distribution of legal squads drawn at random from
// players who actually featured, so the model's result has something to be
// judged against. A squad picked by dartboard is the honest baseline: beating
// it is the minimum bar, not an achievement.
func RandomSquads(s *Season, budget, n int, seed int64) []int {
	quota := map[int]int{1: 2, 2: 5, 3: 5, 4: 3}
	var pool []*Player
	reg := registeredBy(s, 0)
	for _, p := range s.Players {
		// Registered at GW1 as well as having played: the baseline a squad is
		// judged against has to be a squad that could have been bought.
		if p.Minutes > 0 && reg.has(p.ID) {
			pool = append(pool, p)
		}
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID < pool[j].ID })

	rng := rand.New(rand.NewSource(seed))
	var out []int
	for attempt := 0; attempt < n*400 && len(out) < n; attempt++ {
		have := map[int]int{}
		club := map[int]int{}
		spend, pts, picked := 0, 0, 0
		rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		for _, p := range pool {
			if have[p.Type] >= quota[p.Type] || club[p.Team] >= 3 {
				continue
			}
			cost := priceAt(p, 1)
			if spend+cost > budget {
				continue
			}
			have[p.Type]++
			club[p.Team]++
			spend += cost
			picked++
			pts += p.TotalPoints
			if picked == 15 {
				break
			}
		}
		if picked == 15 {
			out = append(out, pts)
		}
	}
	sort.Ints(out)
	return out
}

// Percentile reports where a score falls in a sorted distribution.
func Percentile(sorted []int, v int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := sort.SearchInts(sorted, v)
	return float64(i) / float64(len(sorted)) * 100
}

// Dud is a squad pick that returned almost nothing, and why.
//
// The distinction matters more than the total. A player who never took the
// pitch was not a modelling error — he had left the league, or was injured all
// season, and the model has no way to know either. That is the judgement
// layer's job. A player who played and simply was not good enough is a genuine
// miss.
type Dud struct {
	Name    string
	Price   float64
	Points  int
	Minutes int
	// Reason is why he failed, in plain language.
	Reason string
	// NeverPlayed marks the picks a single web search would have avoided.
	NeverPlayed bool
}

// Duds reports expensive picks that returned under the threshold.
func Duds(s *Season, squad []analysis.PlayerMetrics, minPrice float64, maxPoints int) ([]Dud, float64) {
	var out []Dud
	var wasted float64
	for _, m := range squad {
		p := s.Players[m.ID]
		if p == nil || p.TotalPoints >= maxPoints || m.Price < minPrice {
			continue
		}
		mins := 0
		for _, g := range p.GWs {
			mins += g.Minutes
		}
		d := Dud{Name: p.WebName, Price: m.Price, Points: p.TotalPoints, Minutes: mins}
		switch {
		case mins == 0:
			d.Reason = "never played — left the league, or out all season"
			d.NeverPlayed = true
		case mins < 900:
			d.Reason = fmt.Sprintf("only %d minutes — injury or suspension", mins)
		default:
			d.Reason = "played, underperformed"
		}
		out = append(out, d)
		wasted += m.Price
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Price > out[j].Price })
	return out, wasted
}

// Ceiling is the best legal squad by actual points at opening prices — perfect
// hindsight, and therefore unreachable. It bounds the scale rather than setting
// a target: no manager knows a player's final total in August.
func Ceiling(s *Season, budget int) int {
	type row struct {
		p    *Player
		cost int
	}
	var all []row
	reg := registeredBy(s, 0)
	for _, p := range s.Players {
		// A hindsight bound still has to be a bound on *legal* squads, and a
		// player who joined in January could not be in a GW1 fifteen.
		if !reg.has(p.ID) {
			continue
		}
		all = append(all, row{p, priceAt(p, 1)})
	}
	sort.Slice(all, func(i, j int) bool {
		vi := float64(all[i].p.TotalPoints) / float64(max1(all[i].cost))
		vj := float64(all[j].p.TotalPoints) / float64(max1(all[j].cost))
		if vi != vj {
			return vi > vj
		}
		return all[i].p.ID < all[j].p.ID
	})
	quota := map[int]int{1: 2, 2: 5, 3: 5, 4: 3}
	have, club := map[int]int{}, map[int]int{}
	spend, pts, n := 0, 0, 0
	for _, r := range all {
		if have[r.p.Type] >= quota[r.p.Type] || club[r.p.Team] >= 3 || spend+r.cost > budget {
			continue
		}
		have[r.p.Type]++
		club[r.p.Team]++
		spend += r.cost
		pts += r.p.TotalPoints
		if n++; n == 15 {
			break
		}
	}
	return pts
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// PriorSeasonName returns the season before the one given, for names in FPL's
// "2023-24" form.
func PriorSeasonName(season string) (string, error) {
	var a, b int
	if _, err := fmt.Sscanf(season, "%d-%d", &a, &b); err != nil || a < 2000 {
		return "", fmt.Errorf("season %q is not in 2023-24 form", season)
	}
	return fmt.Sprintf("%d-%02d", a-1, (a)%100), nil
}
