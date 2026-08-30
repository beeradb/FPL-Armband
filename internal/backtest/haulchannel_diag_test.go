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

// Does haul propensity predict inside the noise band, where ownership did not?
//
// PRE-REGISTERED — see the vault note "prereg does haul propensity predict
// inside the band". Population, signals, outcome and decision rule were fixed
// before this ran; nothing was added or dropped afterwards.
//
// A CHANNEL test, not a policy test. `Score` is a mean, and two players with the
// same expected points can carry very different odds of a big return — nothing on
// the forward projection path carries a distribution, so the optimiser cannot see
// the difference. This asks whether there is anything there to see, before anyone
// builds a model that emits one.
//
// Pair-level on purpose. The six-season policy sweep this follows had df 5 and
// could barely resolve anything; AGENTS.md says to sharpen the comparison rather
// than add cells, and pairs inside the band are the sharper comparison.
func TestDiagHaulChannel(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	band := cfg.Review.MinSeparableGain
	if band <= 0 {
		t.Fatal("MinSeparableGain is zero, so there is no band to pair inside")
	}

	const (
		firstCut = 6  // six gameweeks of history behind the proxy
		lastCut  = 37 // predict through+1, so 38 is the last outcome
		topN     = 30 // per position: the pool the optimiser actually chooses among
	)

	rows := [][]string{{"season", "gw", "pos", "id_hi", "id_lo",
		"d_haulrate", "d_expmins", "d_own", "d_price", "d_points"}}

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)
		sc := sweepConfig(cfg, 1, false)

		byID := map[int]*Player{}
		for _, p := range cur.Players {
			byID[p.ID] = p
		}

		for through := firstCut; through <= lastCut; through++ {
			e, _ := EngineAt(cur, prior, through, sc)
			if e == nil {
				continue
			}

			// Haul rate strictly from gameweeks the model can see.
			haul := map[int]float64{}
			for id, p := range byID {
				var played, hauls float64
				for gw := 1; gw <= through; gw++ {
					g, ok := p.GWs[gw]
					if !ok {
						continue
					}
					played++
					if g.Points >= 9 {
						hauls++
					}
				}
				if played > 0 {
					haul[id] = hauls / played
				}
			}

			byPos := map[string][]analysis.PlayerMetrics{}
			for _, m := range e.AllMetrics() {
				byPos[m.Position] = append(byPos[m.Position], m)
			}

			for pos, ms := range byPos {
				sort.SliceStable(ms, func(i, j int) bool { return ms[i].Score > ms[j].Score })
				if len(ms) > topN {
					ms = ms[:topN]
				}
				for i := 0; i < len(ms); i++ {
					for j := i + 1; j < len(ms); j++ {
						a, b := ms[i], ms[j]
						if a.Score-b.Score > band {
							// Sorted descending, so every later j is further away.
							break
						}
						ha, oka := haul[a.ID]
						hb, okb := haul[b.ID]
						if !oka || !okb {
							continue
						}
						// Orient the pair on the signal under test — haul rate —
						// and record every other signal's difference with the SAME
						// orientation, so all four are measured on one pair set.
						hi, lo := a, b
						dh := ha - hb
						if dh < 0 {
							hi, lo = b, a
							dh = -dh
						} else if dh == 0 {
							continue // the signal has no opinion about this pair
						}
						rows = append(rows, []string{
							season, strconv.Itoa(through + 1), pos,
							strconv.Itoa(hi.ID), strconv.Itoa(lo.ID),
							f(dh),
							f(hi.ExpectedMinutes - lo.ExpectedMinutes),
							f(hi.Ownership - lo.Ownership),
							f(hi.Price - lo.Price),
							f(float64(realised(byID, hi.ID, through+1) -
								realised(byID, lo.ID, through+1))),
						})
					}
				}
			}
		}
		t.Logf("%s: %d pairs cumulative", season, len(rows)-1)
	}

	// Banked outside the repo: 93,990 rows is 6.3MB, which is not testdata. The
	// path is named in the finding's measured_at, the way
	// work/landed/attribute-the-override-mode-effect.md names its cells.
	// One path, not an env override. A second way to set an output location is a
	// fallback, and it would also be an undeclared FPL_* switch --
	// TestEnvSwitchListIsComplete greps the tree for those and cannot tell an
	// output path from one that reshapes a measurement.
	dir := "/work/drop/haul-channel-2026-08-30"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/pairs.csv"
	fh, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	w := csv.NewWriter(fh)
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	t.Logf("wrote %s (%d pairs)", path, len(rows)-1)
}

// realised is what a manager who started this player actually received. A player
// with no row in the gameweek scores zero, which is what a blank or an omission
// pays — not a missing value to be dropped, because dropping them would select
// against exactly the failure a tiebreak is supposed to avoid.
func realised(byID map[int]*Player, id, gw int) int {
	p, ok := byID[id]
	if !ok {
		return 0
	}
	g, ok := p.GWs[gw]
	if !ok {
		return 0
	}
	return g.Points
}

func f(v float64) string { return fmt.Sprintf("%.6f", v) }
