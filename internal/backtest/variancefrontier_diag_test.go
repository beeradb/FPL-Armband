package backtest

import (
	"encoding/csv"
	"os"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// Is there a variance frontier at equal expected points?
//
// PRE-REGISTERED — see the vault note "prereg is there a variance frontier at
// equal expected points". This is the cheap go/no-go on whether a RANK objective
// could differ from the shipped points-maximising one.
//
// Rank is P(score > a percentile of the field), which depends on a squad's score
// DISTRIBUTION and not its mean. That can only differ from a mean-maximiser if,
// among squads the engine rates as equal, realised variance materially differs.
// If it does not, a rank objective IS a points objective and a four-piece build
// is not justified.
//
// ⚠️ This measures no rank and models no field: Season carries no
// average_entry_score and the archive has no population scores. It measures only
// whether the raw material — dispersion that varies at fixed mean — exists.
//
// The arms are the tiebreak signals merged in #147 and #153, used here not as
// candidate policies (all three were ruled out on their own terms) but as a
// near-EV-equal SQUAD GENERATOR. The 2x band deliberately exceeds the model's own
// stated resolution so the frontier is traced rather than sampled at one point.
//
// RESULT (six seasons, deployed weights). The frontier exists and OWNERSHIP is
// the lever: all three ownership bands move weekly SD past their own season-
// clustered threshold, at +1.076 to +1.320 against a baseline SD of 17.47 — call
// it 6-8%. Those three are the quotable numbers because they are the three arms
// individually tested.
//
// ⚠️ Do NOT quote the full -0.355..+1.320 spread as "9.6% of weekly SD". The
// squash commit that merged this file (#157) says "~10%", and that is the figure
// being corrected here: it is a range statistic over a mix of resolved and
// unresolved deltas, it has no standard error of its own, and it is carried
// entirely by its two extremes — drop the highest arm and the lowest (haul-1x,
// t = -0.72 and plausibly pure noise) and the spread halves to 0.895.
//
// ⚠️ The lever is NOT free. "EV-indistinguishable" here means "not detected at a
// loose threshold", not "equal": 8 of 9 arms drift negative, mean -21.4 points a
// season, and price and haul are negative at every dose. Thresholds run ±69 to
// ±122 points a season, two to four times this project's own ~30-35 point
// detection floor, so no individual arm is powered to see a cost that size. Read
// it as "cheap at a resolution this design cannot see past".
//
// ⚠️ The pre-registered decision rule had two clauses and the SECOND FAILED:
// baseline wins outright at p90 and p95, ownership-0.5x wins only at p50 and p75.
// The clause computed P(>T) from six season totals per arm — between-season
// difficulty, not the strategy variance the first clause measures — so it was the
// wrong test, but it failed as written and the record says so.
func TestDiagVarianceFrontier(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	base := cfg.Review.MinSeparableGain
	if base <= 0 {
		t.Fatal("MinSeparableGain is zero, so every arm is the baseline")
	}

	weeks := [][]string{{"season", "arm", "gw", "gross", "net"}}
	seasons := [][]string{{"season", "arm", "signal", "band", "points", "slots_vs_baseline"}}

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)

		haul := map[int]float64{}
		for _, p := range prior.Players {
			var played, hauls float64
			for _, g := range p.GWs {
				played++
				if g.Points >= 9 {
					hauls++
				}
			}
			if played > 0 {
				haul[p.ID] = hauls / played
			}
		}

		type arm struct {
			name, signal string
			band         float64
			tb           analysis.Tiebreak
		}
		arms := []arm{{name: "baseline", signal: "none", band: 0, tb: analysis.TiebreakOff}}
		for _, mult := range []float64{0.5, 1.0, 2.0} {
			b := base * mult
			for _, sg := range []string{analysis.TiebreakOwnership, analysis.TiebreakPrice, analysis.TiebreakHaul} {
				tb := analysis.Tiebreak{Signal: sg, Band: b}
				if sg == analysis.TiebreakHaul {
					tb.HaulRate = haul
				}
				arms = append(arms, arm{
					name:   sg + "-" + strconv.FormatFloat(mult, 'g', -1, 64) + "x",
					signal: sg, band: b, tb: tb,
				})
			}
		}

		var baseOpening []int
		for _, a := range arms {
			sc := sweepConfig(cfg, 1, false)
			sc.Tiebreak = a.tb
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("%s %s: %v", season, a.name, err)
			}
			if a.name == "baseline" {
				baseOpening = append([]int(nil), res.OpeningSquad...)
			}
			diff := openingSlotsDifferent(baseOpening, res.OpeningSquad)
			for _, w := range res.Weeks {
				weeks = append(weeks, []string{season, a.name,
					strconv.Itoa(w.GW), strconv.Itoa(w.Gross), strconv.Itoa(w.Net)})
			}
			seasons = append(seasons, []string{season, a.name, a.signal,
				strconv.FormatFloat(a.band, 'f', 3, 64),
				strconv.Itoa(res.Points), strconv.Itoa(diff)})
			t.Logf("%-8s %-14s band=%.3f points=%4d slots=%2d/15",
				season, a.name, a.band, res.Points, diff)
		}
	}
	writeFrontierCSV(t, "weeks.csv", weeks)
	writeFrontierCSV(t, "seasons.csv", seasons)
}

func writeFrontierCSV(t *testing.T, name string, rows [][]string) {
	t.Helper()
	dir := "/work/drop/variance-frontier-2026-08-30"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(dir + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	t.Logf("wrote %s/%s (%d rows)", dir, name, len(rows)-1)
}
