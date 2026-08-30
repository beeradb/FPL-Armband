package backtest

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

// The six-season tiebreak sweep. PRE-REGISTERED before the run — see the vault
// note "prereg the tiebreak inside the noise band on six seasons" — including
// the finding that the season-points arm CANNOT resolve and is reported only so
// that a null there is not mistaken for evidence of no cost.
//
// Go prints no standard error, no t and no verdict word: that is AGENTS.md's
// division of labour, and the inference runs in R over the CSV this writes.
//
// The void condition is checked first and reported whatever else happens. An arm
// that changed no squad has not tested a policy, and its null means nothing.
func TestDiagTiebreakSweep(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	band := cfg.Review.MinSeparableGain
	if band <= 0 {
		t.Fatalf("MinSeparableGain is %v, so every arm is the baseline and the "+
			"sweep tests nothing", band)
	}

	arms := []struct {
		name string
		tb   analysis.Tiebreak
	}{
		{"baseline", analysis.TiebreakOff},
		{"ownership", analysis.Tiebreak{Signal: analysis.TiebreakOwnership, Band: band}},
		{"price", analysis.Tiebreak{Signal: analysis.TiebreakPrice, Band: band}},
	}

	weekRows := [][]string{{"season", "arm", "gw", "gross", "net"}}
	seasonRows := [][]string{{"season", "arm", "points", "xpoints", "transfers",
		"hits", "opening_slots_vs_baseline"}}

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)

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
				weekRows = append(weekRows, []string{season, arm.name,
					strconv.Itoa(w.GW), strconv.Itoa(w.Gross), strconv.Itoa(w.Net)})
			}
			seasonRows = append(seasonRows, []string{season, arm.name,
				strconv.Itoa(res.Points), fmt.Sprintf("%.2f", res.XPoints),
				strconv.Itoa(res.Transfers), strconv.Itoa(res.Hits), strconv.Itoa(diff)})

			t.Logf("%-8s %-9s points=%4d xp=%7.1f transfers=%3d hits=%2d "+
				"opening-slots-changed=%2d/15",
				season, arm.name, res.Points, res.XPoints, res.Transfers,
				res.Hits, diff)
		}
	}

	writeSweepCSV(t, "tiebreak-weeks.csv", weekRows)
	writeSweepCSV(t, "tiebreak-seasons.csv", seasonRows)
}

// openingSlotsDifferent is outcome 4, the void check: how many of the fifteen
// this arm bought that the baseline did not. Zero across the board means the
// policy never bit and every other number in the run is uninformative.
func openingSlotsDifferent(base, arm []int) int {
	if len(base) == 0 {
		return 0
	}
	in := map[int]bool{}
	for _, id := range base {
		in[id] = true
	}
	n := 0
	for _, id := range arm {
		if !in[id] {
			n++
		}
	}
	return n
}

func writeSweepCSV(t *testing.T, name string, rows [][]string) {
	t.Helper()
	dir := "testdata/tiebreak"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/" + name
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	sort.SliceStable(rows[1:], func(i, j int) bool { return false })
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	t.Logf("wrote %s (%d rows)", path, len(rows)-1)
}
