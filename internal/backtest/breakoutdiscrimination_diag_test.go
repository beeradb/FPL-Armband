// Does a cheap, in-form player go on to keep returning, or does the hot spell
// stop the moment it is noticed?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBreakoutDiscrimination -v
//
// # What "breakout candidate" means here
//
// A player at or under £6.0m who has scored 15+ points across his last three
// gameweeks after scoring under 10 across the three before that — a visible
// step up from a cold platform, on a price a manager can actually still
// afford. Three gameweeks either side is the shortest window that survives a
// single haul: one 12-point blank-and-brace can clear the hot threshold alone,
// but sustaining 15 across three needs at least two productive matches, and
// the report below (`hot_start_share`) says whether those matches were starts
// or a substitute cameo that happened to convert everything.
//
// This is DESCRIPTIVE, not a policy arm. It emits one CSV row per qualifying
// (season, player, gameweek) and nothing more — no threshold is chosen here,
// no verdict is reached in Go. Inference — does the hot window predict the
// next ten gameweeks, and does the engine's own Score already price it in —
// happens in R against the emitted rows. See the standing rule: Go prints no
// p-value and no verdict word.
//
// # Point-in-time is the whole point
//
// A "breakout candidate at gameweek g" must be a call a manager filling his
// team on the Friday before gameweek g+1 could actually have made. Every
// predictor column here — price, the hot window, the cold window, minutes,
// starts, the engine's Score — is built from EngineAt/PointInTime cut off at
// g, which by construction cannot see gameweek g+1 or later (PointInTime's own
// doc: "Nothing after `through` is visible"). This package has a recorded bug
// class for exactly the opposite mistake — an unwired or hindsight-fed engine
// reading like a point-in-time one — so the discipline here is the same as
// EngineAt's own: never read a GW row above g when deciding whether g
// qualifies.
//
// The outcome is the one place the future is deliberately allowed in:
// out_points_10 and out_minutes_10 sum gameweeks g+1..g+10, because "did the
// breakout continue" is a question only the future can answer. Mixing that
// window into any predictor column would be the leak; keeping it out of every
// column but these two is what point-in-time means in practice.
//
// # Why g runs 5..28
//
// g=5 is the earliest gameweek with a full three-gameweek cold window behind
// it (g-5 down to g-3 needs g>=5 to stay off gameweek 0, which does not
// exist). g=28 is the latest that leaves a full ten-gameweek outcome window
// inside a 38-gameweek season (28+10=38): a shorter outcome window on the
// tail gameweeks would silently mix players with 10 games of runway in with
// players who had 2, which is exactly the kind of unstated censoring this
// package's void checks exist to catch instead of hide.
//
// # The engine is built once per (season, g)
//
// EngineAt reconstructs the whole bootstrap and reruns every scoring term; it
// is not free, and nothing about its answer depends on which player is asking.
// Rebuilding it per player would multiply the cost by the size of the pool for
// no different a number.
//
// # hot_pts_minus_xgi, not "expected points"
//
// The brief for this diagnostic asked for the hot window's points minus the
// engine's own expected points for that window, if that were cheaply
// available. It is not: the engine's xPoints machinery
// (`analysis.XPointsResidual`) is priced per gameweek through a
// position-and-season conversion scale that has no per-player, per-window
// entry point here, and this file is not the place to build one. What is
// cheap and unambiguous is FPL's own flat scoring — 4 for a goal, 3 for an
// assist — applied to the hot window's own xG and xA, which says the same
// thing the requested column would: how much of the hot window's points came
// from finishing above (or below) the underlying chances, rather than from
// the chances themselves. Named for what it is rather than for what it is not.
package backtest

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// sumPointsOver, sumMinutesOver and friends all answer "what happened between
// two gameweeks, inclusive" from ONLY the rows the caller hands them —
// nothing here reaches past the range it is given, which is what lets every
// caller below stay honest about which side of gameweek g it is reading.
func sumGWRange(p *Player, from, to int) (points, minutes, starts, rows int, xg, xa float64) {
	for gw := from; gw <= to; gw++ {
		g, ok := p.GWs[gw]
		if !ok {
			continue
		}
		points += g.Points
		minutes += g.Minutes
		starts += g.Starts
		xg += g.XG
		xa += g.XA
		rows++
	}
	return
}

// breakoutRow is one qualifying (season, player, gameweek) observation. Field
// order matches the CSV column order exactly, so the struct is the header.
type breakoutRow struct {
	season         string
	gw             int
	playerID       int
	webName        string
	position       int
	priceTenths    int
	hotPoints      int
	hotXGXAper90   float64
	hotPtsMinusXGI float64
	hotStartShare  float64
	hotMinutes     int
	prevMinutes    int
	minutesTrend   int
	engineScore    float64
	teamID         int
	outPoints10    int
	outMinutes10   int
}

var breakoutCSVHeader = []string{
	"season", "gw", "player_id", "web_name", "position", "price_tenths",
	"hot_points", "hot_xgxa_per90", "hot_pts_minus_xgi", "hot_start_share",
	"hot_minutes", "prev_minutes", "minutes_trend", "engine_score", "team_id",
	"out_points_10", "out_minutes_10",
}

func (r breakoutRow) toCSV() []string {
	return []string{
		r.season,
		strconv.Itoa(r.gw),
		strconv.Itoa(r.playerID),
		r.webName,
		strconv.Itoa(r.position),
		strconv.Itoa(r.priceTenths),
		strconv.Itoa(r.hotPoints),
		strconv.FormatFloat(r.hotXGXAper90, 'f', 4, 64),
		strconv.FormatFloat(r.hotPtsMinusXGI, 'f', 4, 64),
		strconv.FormatFloat(r.hotStartShare, 'f', 4, 64),
		strconv.Itoa(r.hotMinutes),
		strconv.Itoa(r.prevMinutes),
		strconv.Itoa(r.minutesTrend),
		strconv.FormatFloat(r.engineScore, 'f', 4, 64),
		strconv.Itoa(r.teamID),
		strconv.Itoa(r.outPoints10),
		strconv.Itoa(r.outMinutes10),
	}
}

func TestDiagBreakoutDiscrimination(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	const (
		priceCeil  = 60 // tenths of a million: £6.0m
		hotFloor   = 15
		coldCeil   = 10
		gStart     = 5
		gEnd       = 28
		outHorizon = 10
	)

	outDir := os.Getenv("FPL_BREAKOUT_OUT")
	if outDir == "" {
		outDir = "/work/drop/breakout-2026-08-30"
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", outDir, err)
	}
	outPath := filepath.Join(outDir, "candidates.csv")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("creating %s: %v", outPath, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write(breakoutCSVHeader); err != nil {
		t.Fatalf("writing header: %v", err)
	}

	type namedFace struct {
		season, webName        string
		gw                     int
		priceTenths, hotPoints int
		outPoints10            int
	}
	var faces []namedFace
	faceNeedles := []string{"palmer", "semenyo", "rogers", "wilson", "anderson", "anthony"}

	perSeasonCount := map[string]int{}
	var total int

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)
		sc := sweepConfig(cfg, 1, false)

		for g := gStart; g <= gEnd; g++ {
			// Built once per (season, g) and reused for every player at this
			// cutoff — see the doc comment above on why that is correct and
			// not merely faster. EngineAt itself is the point-in-time seam:
			// it can see gameweeks 1..g and nothing past g.
			eng, _ := EngineAt(cur, prior, g, sc)
			if eng == nil {
				continue
			}
			score := map[int]float64{}
			for _, m := range eng.AllMetrics() {
				score[m.ID] = m.Score
			}

			for _, p := range cur.Players {
				// price at g, or the most recent priced row at or before g —
				// the same fallback PointInTime's own registration.price uses,
				// so a candidate here is priced exactly as the engine sees him.
				price := priceAt(p, g)
				if price <= 0 || price > priceCeil {
					continue
				}

				hotPts, hotMins, hotStarts, hotRows, hotXG, hotXA := sumGWRange(p, g-2, g)
				if hotRows == 0 {
					continue
				}
				if hotPts < hotFloor {
					continue
				}
				coldPts, prevMins, _, _, _, _ := sumGWRange(p, g-5, g-3)
				if coldPts >= coldCeil {
					continue
				}

				var xgxaPer90 float64
				if hotMins > 0 {
					xgxaPer90 = (hotXG + hotXA) / float64(hotMins) * 90
				}
				startShare := float64(hotStarts) / float64(hotRows)
				ptsMinusXGI := float64(hotPts) - 4*hotXG - 3*hotXA

				// The outcome. Deliberately the one place this loop reads past
				// g: a missing row in g+1..g+10 means the player did not
				// appear and scores nothing, not that the row is unknown, so
				// it is left uncounted rather than imputed.
				outPts, outMins, _, _, _, _ := sumGWRange(p, g+1, g+outHorizon)

				row := breakoutRow{
					season:         season,
					gw:             g,
					playerID:       p.ID,
					webName:        p.WebName,
					position:       p.Type,
					priceTenths:    price,
					hotPoints:      hotPts,
					hotXGXAper90:   xgxaPer90,
					hotPtsMinusXGI: ptsMinusXGI,
					hotStartShare:  startShare,
					hotMinutes:     hotMins,
					prevMinutes:    prevMins,
					minutesTrend:   hotMins - prevMins,
					engineScore:    score[p.ID],
					teamID:         p.Team,
					outPoints10:    outPts,
					outMinutes10:   outMins,
				}
				if err := w.Write(row.toCSV()); err != nil {
					t.Fatalf("writing row: %v", err)
				}
				perSeasonCount[season]++
				total++

				lname := strings.ToLower(p.WebName)
				for _, needle := range faceNeedles {
					if strings.Contains(lname, needle) {
						faces = append(faces, namedFace{
							season: season, webName: p.WebName, gw: g,
							priceTenths: price, hotPoints: hotPts, outPoints10: outPts,
						})
						break
					}
				}
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("flushing %s: %v", outPath, err)
	}

	// Pre-registered void check. A season yielding a handful of candidates out
	// of a full replay is not "the effect is small" — it is a filter that
	// silently narrowed to almost nothing, and the two look identical unless
	// this is printed every run.
	t.Log("=== candidate counts per season (pre-registered void check: expect >~20) ===")
	seasons := make([]string, 0, len(perSeasonCount))
	for s := range perSeasonCount {
		seasons = append(seasons, s)
	}
	sort.Strings(seasons)
	for _, s := range seasons {
		n := perSeasonCount[s]
		t.Logf("  %-8s %5d candidates%s", s, n, func() string {
			if n < 20 {
				return "  <-- BELOW THE ~20 VOID-CHECK FLOOR"
			}
			return ""
		}())
	}
	t.Logf("  TOTAL    %5d candidates across %d seasons", total, len(seasons))

	t.Log("=== named face-validity cases (palmer, semenyo, rogers, wilson, anderson, anthony) ===")
	sort.Slice(faces, func(i, j int) bool {
		if faces[i].season != faces[j].season {
			return faces[i].season < faces[j].season
		}
		return faces[i].gw < faces[j].gw
	})
	for _, f := range faces {
		t.Logf("  %-8s gw%-2d %-16s price=%d hot_points=%d out_points_10=%d",
			f.season, f.gw, f.webName, f.priceTenths, f.hotPoints, f.outPoints10)
	}
	if len(faces) == 0 {
		t.Log("  (none found)")
	}

	t.Logf("wrote %d candidate rows to %s", total, outPath)
}
