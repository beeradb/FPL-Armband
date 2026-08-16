package recent

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// Priors is a multi-season prior built from FPL's own per-player history.
//
// # Why not the CSV archive
//
// internal/priors reads a third-party mirror that publishes one season. Blending
// needs several, and the mirror 404s for anything older, so a blend configured
// against it silently degrades to the single season it was meant to improve on.
//
// FPL's element-summary carries history_past: four completed seasons per player
// with minutes, starts, expected goals and assists, defensive contributions,
// saves and cards — everything the model reasons from. It is authoritative, and
// internal/recent already fetches that endpoint for match history, so the data
// costs nothing extra.
type Priors struct {
	Fetched, Failed int
	byCode          map[int]*analysis.PriorPlayer
}

// Get returns a player's blended prior, and whether he has one.
func (p *Priors) Get(code int) (*analysis.PriorPlayer, bool) {
	if p == nil {
		return nil, false
	}
	q, ok := p.byCode[code]
	return q, ok
}

// LoadPriors fetches each player's past seasons and blends them.
//
// halfLife is in seasons. Zero or one available season returns the most recent
// alone, so this is a strict generalisation of the single-season prior.
func LoadPriors(ctx context.Context, c *fpl.Client, boot *fpl.Bootstrap,
	halfLife float64, concurrency int) (*Priors, error) {

	if boot == nil || len(boot.Elements) == 0 {
		return nil, fmt.Errorf("no players to load priors for")
	}
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	out := &Priors{byCode: map[int]*analysis.PriorPlayer{}}
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)
	for i := range boot.Elements {
		el := boot.Elements[i]
		if el.Code == 0 {
			continue
		}
		g.Go(func() error {
			s, err := c.ElementSummary(ctx, el.ID)
			if err != nil {
				mu.Lock()
				out.Failed++
				mu.Unlock()
				return nil // this player falls back to whatever else is available
			}
			p, ok := blendPast(s.HistoryPast, halfLife)
			if !ok {
				return nil
			}
			mu.Lock()
			out.byCode[el.Code] = &p
			out.Fetched++
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if out.Fetched == 0 {
		return nil, fmt.Errorf("no past seasons found for any player")
	}
	return out, nil
}

// blendPast collapses a player's past seasons into one prior.
//
// FPL returns history_past oldest first, so the list is walked backwards to make
// SeasonsAgo count from the most recent.
func blendPast(past []fpl.PastSeason, halfLife float64) (analysis.PriorPlayer, bool) {
	if len(past) == 0 {
		return analysis.PriorPlayer{}, false
	}
	// lastSeasonMinutes is what he recorded in the most recent season ON RECORD,
	// zero-minute row included. The blend gate is asked about that, not about the
	// most recent season that survives the filter below — a player who sat out
	// last season entirely must not be blended on the strength of the one before
	// it. FPL returns history_past oldest first, so the last element is it.
	//
	// ⚠️ One case this path cannot see and the two archive paths can. history_past
	// carries a row per season the player was IN THE GAME, so "registered and never
	// played" arrives as a row of zero minutes and is gated correctly, while
	// "playing abroad last season" arrives as no row at all — and then this reads
	// an OLDER season's minutes and gates on those. He is treated as though that
	// season were last season. It is unchanged from before the gate and the
	// shipped model does the same thing (`priors.Load` has no row for him either,
	// so `Get` returns false and he meets the league rate) — but it does mean the
	// three implementations agree about a footballer only when the source records
	// him. Untested, because there is nothing here to assert against: the
	// benchmark runs on the archive, where the row always exists.
	lastSeasonMinutes := past[len(past)-1].Minutes

	var hist []analysis.PriorSeasonStats
	for i := len(past) - 1; i >= 0; i-- {
		s := past[i]
		if s.Minutes == 0 {
			continue
		}
		hist = append(hist, analysis.PriorSeasonStats{
			PriorPlayer: analysis.PriorPlayer{
				Minutes: s.Minutes, Starts: s.Starts,
				XG:     s.ExpectedGoals.Float(),
				XA:     s.ExpectedAssists.Float(),
				XGC:    s.ExpectedGoalsConceded.Float(),
				DefCon: s.DefensiveContribution,
				Bonus:  s.Bonus, Saves: s.Saves,
				Yellow: s.YellowCards, Red: s.RedCards,
			},
			SeasonsAgo: len(hist),
			// The values above are copied whatever the season, because the
			// flags are what tell the blend which of them were measured. Before
			// 2022/23 FPL returns "0.00" for all three expected statistics
			// beside three thousand real minutes, and before 2024/25 the same
			// for defensive contribution — an absence arriving as data. Without
			// these two lines a long-serving defender blends five genuine-looking
			// zeroes into his expected goals conceded.
			// NoXG and NoXGC take the same predicate here, and that is correct for
			// this source rather than an oversight: FPL's `history_past` publishes
			// all three expected statistics together from 2022/23, so there is no
			// state in which it has one and not the other. The flags are separate
			// because the *archive* has such a state under FPL_NO_XGC_REPAIR — see
			// analysis.PriorSeasonStats.
			NoXG:     !s.HasExpected(),
			NoXGC:    !s.HasExpected(),
			NoDefCon: !s.HasDefCon(),
		})
	}
	// No minutes last season is "no usable history", and it must come back that
	// way HERE rather than as the last season he played.
	//
	// This is the gate's other half and it is not symmetrical with the archive
	// paths, because this one throws the evidence away before the gate can see
	// it. `hist` has had zero-minute rows dropped, so for a man who sat out last
	// season `hist[0]` is a season at least two years old, carrying minutes —
	// and `blendRates` gates on `!ok || p.Minutes == 0`, so a prior with minutes
	// never reaches `shrinkToLeague`. Returning it would rate him on stale rates,
	// and pre-season it would REPLACE the bootstrap outright (see the
	// `played == 0` branch in blend.go).
	//
	// What the shipped model does with him is the bar: at prior_half_life 0 this
	// function is not called at all — `cmd/armband` skips `LoadPriors` and takes
	// `priors.Load`, one season, which carries `Minutes 0` and so does reach
	// `shrinkToLeague`. Reporting no prior reaches the same place.
	//
	// Getting this wrong would have been the exact failure the gate was built to
	// avoid: a rule applied on two paths and defeated on the third, and the third
	// is the only one a live command runs.
	if len(hist) == 0 || lastSeasonMinutes == 0 {
		return analysis.PriorPlayer{}, false
	}
	// One season, or blending switched off: the most recent, unchanged.
	if len(hist) == 1 || halfLife <= 0 {
		return hist[0].PriorPlayer, true
	}
	// Only reach back when the most recent season is thin and he actually played
	// some of it — analysis.ShouldBlendPrior carries both halves and the
	// reasoning. By here the zero half has already returned above, so this line
	// only ever declines a FULL season, and declining it returns last season
	// unchanged: what the shipped model believes about him, which is the property
	// that lets prior_half_life be turned on without moving anyone the feature is
	// not for.
	if !analysis.ShouldBlendPrior(lastSeasonMinutes) {
		return hist[0].PriorPlayer, true
	}
	return analysis.BlendPriors(hist, halfLife), true
}

// ThinSeason is the minutes below which the most recent season stops being
// trusted on its own. Half a season.
//
// An alias, not a second declaration — see analysis.ThinSeason.
const ThinSeason = analysis.ThinSeason
