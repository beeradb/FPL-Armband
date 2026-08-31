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
