// What is an "enabler" actually worth — the cheap player bought to fund a
// premium elsewhere, who is expected to rise in price and get sold on later?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagEnablerValue -v
//
// # Why PointsPerTenth is reported first
//
// An enabler's whole case rests on one conversion: a price rise turned into
// cash, and cash turned into points via PointsPerTenth (analysis/budgetvalue.go).
// enabler_points below is nothing but sell_side_tenths * points_per_tenth_at_g —
// a rise the manager never gets to keep in full (see the halving note),
// multiplied by whatever a tenth of a million is worth in points that week. If
// that second factor is tiny, no rise, however real, converts into anything
// worth discussing — the whole enabler thesis is bounded above by a number this
// package already knows how to measure and has not, until now, printed next to
// the players it is meant to be worth. Part 1 exists to put that bound on the
// record BEFORE a single candidate row is read, so nobody reads Part 2's
// numbers as bigger than the conversion rate allows.
//
// # This reuses breakoutdiscrimination_diag_test.go's candidate population
//
// "Cheap, in-form, was cold before that" is one population, not two — a
// breakout candidate and an enabler candidate are the same player asked two
// different questions (does he keep returning himself; what is he worth when
// sold). Re-deriving the filter here on slightly different thresholds would
// make the two studies silently incomparable while looking like the same
// study, so the constants and the qualifying logic below are copied from
// TestDiagBreakoutDiscrimination's, unchanged. sumGWRange is shared outright
// (defined once, in that file) rather than re-implemented — a second version
// of "what happened between two gameweeks" is exactly the class of bug this
// project keeps a name for.
//
// # Point-in-time
//
// Every predictor column — price at g, the hot and cold windows, the engine's
// PointsPerTenth at g — is read no later than gameweek g, through the same
// EngineAt/priceAt/sumGWRange seam the breakout diagnostic uses, for the same
// reason: a manager filling his team on the Friday before gameweek g+1 could
// not have seen gameweek g+1. price_at_g_plus_10 and own_points_10 are the
// deliberate exceptions — an enabler's sale price and his own return are both
// questions only the future answers — and they are the only two columns that
// read past g.
//
// # What is NOT in this CSV, and why
//
// The brief asked for transfers_in_hot, transfers_out_hot and net_transfers_hot
// — gross transfer activity in the hot window. This package's archive parser
// (season.go's loadGameweeks) does not read those columns off the source CSV
// at all: Player and GW carry Selected, a per-gameweek ownership LEVEL (a
// count of managers holding the player that week), and nothing for the
// transfer EVENT counts the source archive separately publishes. Selected's
// own doc warns it is not comparable across gameweeks without normalising for
// the growing manager pool, and a week-over-week delta in it is a different,
// confounded quantity from a real transfers-in/transfers-out count, not a
// stand-in for one. Reporting that delta under the requested column names
// would be exactly this project's recorded failure mode — a plausible number
// measuring something other than what it claims — so it is left out rather
// than faked. Adding the real columns means teaching season.go's loadGameweeks
// to parse them, which is a season.go change and out of scope for a diagnostic
// test file that must not touch existing files. See the REPORT for this run
// for the explicit callout.
package backtest

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// enablerRow is one qualifying (season, player, gameweek) observation. Field
// order matches the CSV column order exactly, so the struct is the header.
type enablerRow struct {
	season         string
	gw             int
	playerID       int
	webName        string
	priceTenths    int
	priceAtGPlus10 int
	rawRiseTenths  int
	sellSideTenths int
	pointsPerTenth float64
	enablerPoints  float64
	ownPoints10    int
}

var enablerCSVHeader = []string{
	"season", "gw", "player_id", "web_name", "price_tenths",
	"price_at_g_plus_10", "raw_rise_tenths", "sell_side_tenths",
	"points_per_tenth_at_g", "enabler_points", "own_points_10",
}

func (r enablerRow) toCSV() []string {
	return []string{
		r.season,
		strconv.Itoa(r.gw),
		strconv.Itoa(r.playerID),
		r.webName,
		strconv.Itoa(r.priceTenths),
		strconv.Itoa(r.priceAtGPlus10),
		strconv.Itoa(r.rawRiseTenths),
		strconv.Itoa(r.sellSideTenths),
		strconv.FormatFloat(r.pointsPerTenth, 'f', 6, 64),
		strconv.FormatFloat(r.enablerPoints, 'f', 4, 64),
		strconv.Itoa(r.ownPoints10),
	}
}

// sellSideOf is FPL's own rule — sell a player and you get what you paid plus
// HALF of any rise since, rounded DOWN to the nearest tenth; a fall is taken in
// full — applied here as a delta rather than an absolute price, because that is
// the shape enabler_points wants: how much of the rise (or fall) the manager
// actually gets to bank, not what the player would fetch outright.
//
// This is not a second implementation of the rule. wallet.sellPrice
// (wallet.go) already IS it — the exact halve-and-truncate arithmetic is
// pinned there and tested in wallet_test.go — so this wraps a throwaway wallet
// seeded with `price` as the recorded purchase and hands the reused answer
// back as a delta: sellSideOf = sellPrice(priceAtGPlus10) - price. A second
// hand-written halving here, next to that one, is exactly the "one quantity,
// two implementations" failure this codebase's own comments call out by name
// — and the one place in this brief where being off by 2x is the easy way to
// be wrong.
func sellSideOf(price, priceAtGPlus10 int) int {
	w := &wallet{bought: map[int]int{0: price}}
	return w.sellPrice(0, priceAtGPlus10) - price
}

func TestDiagEnablerValue(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	// Unchanged from TestDiagBreakoutDiscrimination: same population, same
	// reasons for each bound. See that file's doc comment for gStart/gEnd and
	// the price ceiling; repeating the derivation here instead of pointing at
	// it would be the same "silently incomparable population" risk the top of
	// this file's doc comment warns about.
	const (
		priceCeil  = 60 // tenths of a million: £6.0m
		hotFloor   = 15
		coldCeil   = 10
		gStart     = 5
		gEnd       = 28
		outHorizon = 10
	)

	// One hardcoded path, not an env override — a second way to steer output
	// would be an undeclared FPL_* switch and TestEnvSwitchListIsComplete greps
	// the tree for exactly that.
	outDir := "/work/drop/enabler-2026-08-30"
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
	if err := w.Write(enablerCSVHeader); err != nil {
		t.Fatalf("writing header: %v", err)
	}

	// --- Part 1: PointsPerTenth's magnitude, reported before anything it bounds. ---
	t.Log("=== Part 1: PointsPerTenth by season and gameweek (points per tenth of a million) ===")
	type ppTRow struct {
		season string
		gw     int
		value  float64
	}
	var ppt []ppTRow
	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)
		sc := sweepConfig(cfg, 1, false)
		for _, g := range []int{5, 10, 15, 20, 25, 30} {
			eng, _ := EngineAt(cur, prior, g, sc)
			if eng == nil {
				continue
			}
			ppt = append(ppt, ppTRow{season: season, gw: g, value: eng.PointsPerTenth()})
		}
	}
	sort.Slice(ppt, func(i, j int) bool {
		if ppt[i].season != ppt[j].season {
			return ppt[i].season < ppt[j].season
		}
		return ppt[i].gw < ppt[j].gw
	})
	var pptSum float64
	for _, r := range ppt {
		t.Logf("  %-8s gw%-2d  points_per_tenth=%.6f", r.season, r.gw, r.value)
		pptSum += r.value
	}
	var pptMean float64
	if len(ppt) > 0 {
		pptMean = pptSum / float64(len(ppt))
	}
	t.Logf("  POOLED MEAN points_per_tenth = %.6f (n=%d season/gw cells)", pptMean, len(ppt))
	if pptMean < 0.01 {
		t.Logf("  *** TINY: at %.6f points per tenth, even a full £1.0m (10 tenths) rise "+
			"caps out around %.2f points once it is converted — read every enabler_points "+
			"figure below against that ceiling, not against the raw price rise. ***",
			pptMean, pptMean*10)
	}

	// --- Part 2: the per-candidate CSV. ---
	perSeasonCount := map[string]int{}
	var total int
	var enablerSum, ownSum float64
	var ratioN int

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)
		sc := sweepConfig(cfg, 1, false)

		for g := gStart; g <= gEnd; g++ {
			// Built once per (season, g) and reused for every candidate at this
			// cutoff — EngineAt is not free, and PointsPerTenth does not depend
			// on which player is asking for it. See EngineAt's own doc for why
			// this is the seam that must be used rather than a bare
			// NewEngineFull here.
			eng, _ := EngineAt(cur, prior, g, sc)
			if eng == nil {
				continue
			}
			pointsPerTenthAtG := eng.PointsPerTenth()

			for _, p := range cur.Players {
				// Same candidate definition as TestDiagBreakoutDiscrimination,
				// copied rather than re-derived — see this file's doc comment.
				price := priceAt(p, g)
				if price <= 0 || price > priceCeil {
					continue
				}
				hotPts, _, _, hotRows, _, _ := sumGWRange(p, g-2, g)
				if hotRows == 0 {
					continue
				}
				if hotPts < hotFloor {
					continue
				}
				coldPts, _, _, _, _, _ := sumGWRange(p, g-5, g-3)
				if coldPts >= coldCeil {
					continue
				}

				// price_at_g_plus_10: GW.Value at g+10, or the most recent
				// priced row at or before g+10 — priceAt already IS that walk
				// (see its own doc), so this reuses it rather than
				// re-implementing "most recent priced row" a second time.
				priceAtGPlus10 := priceAt(p, g+outHorizon)
				rawRise := priceAtGPlus10 - price

				// ⚠️ The single most important number in this file. FPL does
				// not hand back a rise in full on sale — it shares half of it,
				// rounded DOWN to the nearest tenth, and takes a fall in full.
				// Pricing an enabler at his raw rise rather than what he
				// actually raises overstates every one of his numbers by
				// roughly 2x, which is exactly the failure mode this
				// diagnostic exists to avoid. sellSideOf wraps wallet.sellPrice
				// (wallet.go) — the one place that arithmetic is implemented in
				// this codebase — rather than repeating it here.
				sellSide := sellSideOf(price, priceAtGPlus10)

				outPts, _, _, _, _, _ := sumGWRange(p, g+1, g+outHorizon)

				enablerPoints := float64(sellSide) * pointsPerTenthAtG

				row := enablerRow{
					season:         season,
					gw:             g,
					playerID:       p.ID,
					webName:        p.WebName,
					priceTenths:    price,
					priceAtGPlus10: priceAtGPlus10,
					rawRiseTenths:  rawRise,
					sellSideTenths: sellSide,
					pointsPerTenth: pointsPerTenthAtG,
					enablerPoints:  enablerPoints,
					ownPoints10:    outPts,
				}
				if err := w.Write(row.toCSV()); err != nil {
					t.Fatalf("writing row: %v", err)
				}
				perSeasonCount[season]++
				total++
				enablerSum += enablerPoints
				ownSum += float64(outPts)
				ratioN++
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("flushing %s: %v", outPath, err)
	}

	// --- Part 3: logs. ---
	// Pre-registered void check, same floor as the breakout diagnostic uses for
	// the same population: a season yielding a handful of candidates out of a
	// full replay is a filter that silently narrowed to nothing, not "the
	// effect is small" — and the two look identical unless this prints every run.
	t.Log("=== candidate counts per season (void check: expect >~20) ===")
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

	t.Log("=== pooled enabler_points vs own_points_10 ===")
	if ratioN > 0 {
		enablerMean := enablerSum / float64(ratioN)
		ownMean := ownSum / float64(ratioN)
		t.Logf("  pooled mean enabler_points = %.4f", enablerMean)
		t.Logf("  pooled mean own_points_10  = %.4f", ownMean)
		if ownMean != 0 {
			t.Logf("  ratio enabler_points/own_points_10 (of means) = %.4f", enablerMean/ownMean)
		} else {
			t.Log("  own_points_10 pooled mean is 0; ratio is undefined")
		}
	} else {
		t.Log("  (no candidates; nothing to pool)")
	}

	t.Logf("wrote %d candidate rows to %s", total, outPath)
}
