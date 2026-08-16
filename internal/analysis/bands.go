package analysis

import "sort"

// Rating opponents separately for attack and defence.
//
// FPL's FDR blends both into one number, so a side that scores freely and
// concedes freely reads as mid-table to everyone. Split into two ratings and
// banded 3/14/3 — worst three, an undifferentiated middle, best three — the
// underlying effects are large, and the defensive side is roughly twice the
// attacking one, which is exactly what a blended rating cannot express:
//
//	within-player, vs the middle 14 | target band  | avoid band
//	attackers vs opponent defence   | +25% / +13%  | -11% / -14%
//	defenders vs opponent attack    | +41% / +21%  | -23% / -27%
//
// The descriptive finding is real. Acting on it previously measured 2195 at
// full strength against 2208 for leaving FPL's blended FDR alone, and it is
// re-tested here against a corrected optimiser and a bench that is no longer
// priced at zero.
//
// # Detection carries no hindsight
//
// Bands come from goals scored and conceded in *finished* fixtures only, so at
// any gameweek the rating uses exactly what a manager could have known. Before
// bandMinMatches have been played there is no rating and no adjustment, which
// means the opening weeks run on FPL's FDR alone.
const bandMinMatches = 5

// bandSize is how many clubs sit in each tail.
const bandSize = 3

// teamBand is where a club sits on one of the two ratings.
type teamBand int

const (
	bandMiddle teamBand = iota
	bandWorst           // bottom three: the band to target
	bandBest            // top three: the band to avoid
)

// bands holds both ratings for every club.
type bands struct {
	attack  map[int]teamBand // how good this club is at scoring
	defence map[int]teamBand // how good this club is at not conceding
	ready   bool
}

// teamBands computes the 3/14/3 split from finished fixtures. Guarded by a
// sync.Once because the tool runner drives Metrics over the whole pool from
// several goroutines at once, and two of them racing to build the same map is a
// fatal "concurrent map writes" rather than a recoverable panic.
func (e *Engine) teamBands() bands {
	e.bandOnce.Do(func() {
		type rec struct{ for_, against, played float64 }
		by := map[int]*rec{}
		get := func(id int) *rec {
			if by[id] == nil {
				by[id] = &rec{}
			}
			return by[id]
		}
		for _, f := range e.Fixtures {
			if !f.Finished || f.TeamHScore == nil || f.TeamAScore == nil {
				continue
			}
			h, a := get(f.TeamH), get(f.TeamA)
			h.for_ += float64(*f.TeamHScore)
			h.against += float64(*f.TeamAScore)
			h.played++
			a.for_ += float64(*f.TeamAScore)
			a.against += float64(*f.TeamHScore)
			a.played++
		}

		type rated struct {
			id       int
			scored   float64
			conceded float64
		}
		var rs []rated
		for id, r := range by {
			if r.played < bandMinMatches {
				continue
			}
			rs = append(rs, rated{id, r.for_ / r.played, r.against / r.played})
		}
		if len(rs) < 2*bandSize+1 {
			e.bandCache = bands{}
			return
		}

		b := bands{attack: map[int]teamBand{}, defence: map[int]teamBand{}, ready: true}

		// Attack: fewest goals scored is the worst attack, and the band an
		// opposing defender wants to face.
		sort.Slice(rs, func(i, j int) bool { return rs[i].scored < rs[j].scored })
		for i := 0; i < bandSize; i++ {
			b.attack[rs[i].id] = bandWorst
			b.attack[rs[len(rs)-1-i].id] = bandBest
		}
		// Defence: most goals conceded is the worst defence, and the band an
		// opposing attacker wants to face.
		sort.Slice(rs, func(i, j int) bool { return rs[i].conceded > rs[j].conceded })
		for i := 0; i < bandSize; i++ {
			b.defence[rs[i].id] = bandWorst
			b.defence[rs[len(rs)-1-i].id] = bandBest
		}
		e.bandCache = b
	})
	return e.bandCache
}

// Band adjustments, applied on top of the FDR-derived multipliers.
//
// These are stated as effects on the quantity each one multiplies, not as
// effects on points, because that is the distinction that decided the
// new-manager calibration. attackBandAdj scales expected goals and assists;
// defenceBandAdj scales expected goals conceded.
//
//   - The three leakiest defences concede 31-70% more than the median club, and
//     attackers return about 23% above their own average against them.
//   - The bottom three attacks score materially less than the median, so a
//     defender facing one should be expected to concede less, which the clean
//     sheet term then converts into points on its own.
//
// The avoid side is deliberately smaller than the target side on attack, since
// the measured penalty for facing a top-three defence (-11% / -14%) is milder
// than the reward for facing a bottom-three one.
const (
	attackBandTarget  = 0.23
	attackBandAvoid   = 0.15
	defenceBandTarget = 0.25
	defenceBandAvoid  = 0.25
)

// attackBandAdj scales attacking returns by the opponent's defensive band.
func (e *Engine) attackBandAdj(opponentID int, strength float64) float64 {
	if strength <= 0 {
		return 1
	}
	b := e.teamBands()
	if !b.ready {
		return 1
	}
	switch b.defence[opponentID] {
	case bandWorst:
		return 1 + attackBandTarget*strength
	case bandBest:
		return 1 - attackBandAvoid*strength
	}
	return 1
}

// defenceBandAdj scales expected goals conceded by the opponent's attacking
// band. Facing a blunt attack means conceding less, so the target band returns
// a factor below one.
func (e *Engine) defenceBandAdj(opponentID int, strength float64) float64 {
	if strength <= 0 {
		return 1
	}
	b := e.teamBands()
	if !b.ready {
		return 1
	}
	switch b.attack[opponentID] {
	case bandWorst:
		return 1 - defenceBandTarget*strength
	case bandBest:
		return 1 + defenceBandAvoid*strength
	}
	return 1
}
