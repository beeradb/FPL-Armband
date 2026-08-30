package backtest

import (
	"encoding/csv"
	"os"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// Does preferring the higher-ceiling player inside the band buy UPSIDE?
//
// PRE-REGISTERED — see the vault note "prereg does a haul-potential tiebreak buy
// upside". ⚠️ The primary direction here is the OPPOSITE of the ownership sweep's
// in #147: that proposal claimed tail SAFETY and predicted dispersion would fall,
// this one claims ceiling and predicts upside will rise. Read side by side they
// are easy to confuse.
//
// #148 already showed haul rate does not predict MEAN next-gameweek points inside
// the band. A mean cannot answer this question: Optimize maximises expected points
// and is indifferent to skew by construction, so two players it rates equally are
// interchangeable to it however differently their returns are distributed.
func TestDiagHaulTiebreakSweep(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	band := cfg.Review.MinSeparableGain
	if band <= 0 {
		t.Fatal("MinSeparableGain is zero, so every arm is the baseline")
	}

	weeks := [][]string{{"season", "arm", "gw", "gross", "net"}}
	seasons := [][]string{{"season", "arm", "points", "opening_slots_vs_baseline", "haul_table_size"}}

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)

		// Ceiling from the PRIOR season: entirely before the replay, so no
		// cutoff arithmetic and no leakage. Keyed by element id because that is
		// what PlayerMetrics carries; ids are stable within the archive's pairs.
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

		arms := []struct {
			name string
			tb   analysis.Tiebreak
		}{
			{"baseline", analysis.TiebreakOff},
			{"haul", analysis.Tiebreak{Signal: analysis.TiebreakHaul, Band: band, HaulRate: haul}},
			{"price", analysis.Tiebreak{Signal: analysis.TiebreakPrice, Band: band}},
		}

		var baseOpening []int
		for _, arm := range arms {
			sc := sweepConfig(cfg, 1, false)
			sc.Tiebreak = arm.tb
			res, err := Simulate(cur, prior, sc)
			if err != nil {
				t.Fatalf("%s %s: %v", season, arm.name, err)
			}
			if arm.name == "baseline" {
				baseOpening = append([]int(nil), res.OpeningSquad...)
			}
			diff := openingSlotsDifferent(baseOpening, res.OpeningSquad)
			for _, w := range res.Weeks {
				weeks = append(weeks, []string{season, arm.name,
					strconv.Itoa(w.GW), strconv.Itoa(w.Gross), strconv.Itoa(w.Net)})
			}
			seasons = append(seasons, []string{season, arm.name,
				strconv.Itoa(res.Points), strconv.Itoa(diff), strconv.Itoa(len(haul))})
			t.Logf("%-8s %-9s points=%4d opening-slots-changed=%2d/15 haul-table=%d",
				season, arm.name, res.Points, diff, len(haul))
		}
	}
	writeHaulCSV(t, "weeks.csv", weeks)
	writeHaulCSV(t, "seasons.csv", seasons)
}

func writeHaulCSV(t *testing.T, name string, rows [][]string) {
	t.Helper()
	dir := "/work/drop/haultiebreak-2026-08-30"
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
