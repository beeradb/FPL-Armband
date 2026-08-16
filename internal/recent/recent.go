// Package recent builds a recency-weighted view of the current season from
// per-player match history.
//
// # Why this exists
//
// FPL's bootstrap publishes season totals and nothing else. That is fine for
// rates, which are a statement about a player's quality and are stable, and
// wrong for minutes, which are a statement about whether he is in the team right
// now. A player who lost his place six weeks ago still reads as an ever-present
// on a season average.
//
// Measured across three replayed seasons on 8,374 out-of-sample predictions,
// weighting minutes with a two-gameweek half-life predicts the next five
// gameweeks 8.9% better than the flat total (MAE 18.98 against 20.83), and
// wiring it into the replay won all three seasons. The same test says the
// opposite for rates — "last 3 games" is 19% *worse* than the season average at
// predicting both points and underlying — so only minutes are weighted here.
//
// # Cost
//
// The history lives behind /element-summary/{id}/, one request per player.
// Measured against the live API: 0.25s each sequentially, and 24 requests in
// 0.94s at concurrency 6, so the whole pool costs roughly half a minute once per
// cache window rather than once per command. Only players who have actually
// played are fetched — a player with no minutes has no history to weight, and
// the flat fallback already reports him correctly — which is 400-500 requests
// rather than 700.
//
// Responses are 13-32KB each depending on how far into the season it is, and the
// client caches raw bodies, so expect this to roughly double the cache directory
// by May. None of it reaches the LLM: this changes a number the tools already
// report, not the shape of what they report.
package recent

import (
	"context"
	"fmt"
	"math"
	"sync"

	"golang.org/x/sync/errgroup"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// DefaultConcurrency is how many history requests run at once. Six was measured
// as comfortably fast without hammering a public API.
const DefaultConcurrency = 6

// Form is a recency-weighted view of the season so far, keyed by FPL's
// permanent player code.
type Form struct {
	HalfLife float64
	// Fetched and Failed report coverage, so a caller can say how much of the
	// pool actually got the treatment.
	Fetched, Failed int

	byCode map[int]analysis.RecentPlayer
}

// Get returns a player's recency-weighted minutes, and whether he was fetched.
func (f *Form) Get(code int) (analysis.RecentPlayer, bool) {
	if f == nil {
		return analysis.RecentPlayer{}, false
	}
	p, ok := f.byCode[code]
	return p, ok
}

// Load fetches match history for every player who has played this season and
// weights it by recency.
//
// Individual failures are tolerated: a player whose history could not be
// fetched simply falls back to his season average, which is what the model did
// before this existed. A caller should check Failed rather than assume full
// coverage.
func Load(ctx context.Context, c *fpl.Client, boot *fpl.Bootstrap, halfLife float64, concurrency int) (*Form, error) {
	if halfLife <= 0 {
		return nil, fmt.Errorf("half-life must be positive, got %v", halfLife)
	}
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	// The most recent completed gameweek is what recency counts back from.
	through := 0
	for _, ev := range boot.Events {
		if ev.Finished && ev.ID > through {
			through = ev.ID
		}
	}
	if through == 0 {
		return nil, fmt.Errorf("no completed gameweeks; there is nothing to weight")
	}

	// A player with no minutes has no history to weight and the flat fallback
	// already reports him correctly, so he is not worth a request.
	type job struct{ id, code int }
	var jobs []job
	for i := range boot.Elements {
		el := &boot.Elements[i]
		if el.Minutes > 0 && el.Code > 0 {
			jobs = append(jobs, job{id: el.ID, code: el.Code})
		}
	}

	f := &Form{HalfLife: halfLife, byCode: make(map[int]analysis.RecentPlayer, len(jobs))}
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)
	for _, j := range jobs {
		g.Go(func() error {
			s, err := c.ElementSummary(ctx, j.id)
			if err != nil {
				mu.Lock()
				f.Failed++
				mu.Unlock()
				return nil // degrade to the season average for this player
			}
			p, ok := weigh(s.History, through, halfLife)
			mu.Lock()
			if ok {
				f.byCode[j.code] = p
				f.Fetched++
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return f, nil
}

// weigh collapses a match history into recency-weighted minutes.
//
// Weighting is per match rather than per gameweek, so a double counts twice and
// a blank not at all — which is what "minutes when he plays" means.
func weigh(history []fpl.HistoryEntry, through int, halfLife float64) (analysis.RecentPlayer, bool) {
	var mins, starts, den float64
	var n int
	for _, h := range history {
		if h.Round < 1 || h.Round > through {
			continue
		}
		w := math.Pow(0.5, float64(through-h.Round)/halfLife)
		mins += float64(h.Minutes) * w
		starts += float64(h.Starts) * w
		den += w
		n++
	}
	if den == 0 || n == 0 {
		return analysis.RecentPlayer{}, false
	}
	return analysis.RecentPlayer{
		MinutesPerMatch: mins / den,
		StartShare:      starts / den,
		Matches:         n,
		BlankRun:        blankRun(history, through),
	}, true
}

// blankRun counts consecutive gameweeks, ending at the most recent one his club
// played, in which he recorded no minutes. See analysis.blankRunFactor.
//
// Minutes are summed per round before counting, so a double gameweek is one
// opportunity rather than two — a player who blanked one leg of a double has not
// been dropped. And only rounds his club actually played are considered, because
// a club's blank gameweek produces no history entry at all and must not read as
// an absence.
func blankRun(history []fpl.HistoryEntry, through int) int {
	byRound := map[int]int{}
	for _, h := range history {
		if h.Round < 1 || h.Round > through {
			continue
		}
		byRound[h.Round] += h.Minutes
	}
	run := 0
	for gw := through; gw >= 1; gw-- {
		mins, ok := byRound[gw]
		if !ok {
			continue
		}
		if mins > 0 {
			break
		}
		run++
	}
	return run
}
