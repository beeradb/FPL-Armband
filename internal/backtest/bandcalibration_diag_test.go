// Is the rotation_risk band label TRUE?
//
//	DIAG=1 go test ./internal/backtest/ -run TestDiagBandCalibration -v -timeout 60m
//
// # Why this is a calibration and not a sweep
//
// `rotationLabel` cuts ExpectedMinutes at 75/60/40/20 into nailed, likely
// starter, rotation risk, squad player and fringe. **Nothing on the scoring or
// optimiser path reads the result.** Every consumer is a reader-facing surface
// (`present/`, `brief.go`), the research prompt (`research.go`, "in the squad but
// not nailed"), or the agent's own tool payload (`agent/tools.go`).
//
// So a replay sweep of these cutoffs would return an exact zero on every arm —
// a structural null, not a result. The lever is inert on points BY CONSTRUCTION,
// which is the mediator lesson from the BandStrength run arriving one layer
// earlier. What the bands can be wrong about is their own claim: the label says
// a player is nailed, and either he starts or he does not.
//
// That claim is load-bearing even though it scores nothing. The record already
// says ExpectedMinutes and this band are "reported to the agent, which is told to
// treat them as a first-class filter", and that "a player reading 44 expected
// minutes who will play 30 is misinforming the only component that can act on
// it."
//
// # Point-in-time, for the usual reason
//
// The band is read from `EngineAt(cur, prior, gw-1)` — the model's view before
// the gameweek — and compared against what happened IN it. Banding with today's
// engine would be scoring the label with hindsight, and it would flatter every
// band by exactly the amount the season went on to reveal.
//
// # ⚠️ What is excluded, and why it is not a silent filter
//
// A player with no archive row for that gameweek is excluded and COUNTED. The
// missing rows are ~6.5% and are a blank gameweek or a player not yet on the
// books; a non-appearance by someone who could have played carries a real
// zero-minute row, of which 2024-25 alone has 8,529. Treating a missing row as a
// zero would charge players for gameweeks their club did not play, and dropping
// zero rows would flatter every band enormously. Both counts are printed.
package backtest

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
)

func TestDiagBandCalibration(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	type cell struct {
		n, played60, played0, minutes int
	}
	overall := map[string]*cell{}
	perSeason := map[string]map[string]*cell{}
	var noRow, total int

	for _, pair := range sweepPairNames() {
		season := pair[1]
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, season)
		perSeason[season] = map[string]*cell{}
		sc := sweepConfig(cfg, 1, false)

		// From GW2: the band at gw is read from the engine at gw-1, and a cutoff
		// of 0 has no data behind it at all.
		for gw := 2; gw <= 38; gw++ {
			e, _ := EngineAt(cur, prior, gw-1, sc)
			if e == nil {
				continue
			}
			for _, m := range e.AllMetrics() {
				p, ok := cur.Players[m.ID]
				if !ok {
					continue
				}
				total++
				g, ok := p.GWs[gw]
				if !ok {
					noRow++
					continue
				}
				band := m.RotationRisk
				for _, dst := range []map[string]*cell{overall, perSeason[season]} {
					c := dst[band]
					if c == nil {
						c = &cell{}
						dst[band] = c
					}
					c.n++
					c.minutes += g.Minutes
					if g.Minutes >= 60 {
						c.played60++
					}
					if g.Minutes == 0 {
						c.played0++
					}
				}
			}
		}
	}

	// FPL's own order, not alphabetical and not by frequency.
	order := []string{"nailed", "likely starter", "rotation risk", "squad player", "fringe"}
	claim := map[string]string{
		"nailed":         "75+ mins, corroborated",
		"likely starter": "60-75, or 75+ uncorroborated",
		"rotation risk":  "40-60",
		"squad player":   "20-40",
		"fringe":         "under 20",
	}

	fmt.Println("\nDOES THE ROTATION_RISK BAND TELL THE TRUTH?")
	fmt.Println("Band read from the engine at gw-1; minutes are what happened in gw.")
	fmt.Printf("\n  %-15s %-30s %8s %9s %9s %9s\n",
		"band", "what the label claims", "n", "mean min", "played 60+", "played 0")
	for _, b := range order {
		c := overall[b]
		if c == nil || c.n == 0 {
			fmt.Printf("  %-15s %-30s %8s %9s %9s %9s\n", b, claim[b], "-", "-", "-", "-")
			continue
		}
		fmt.Printf("  %-15s %-30s %8d %9.1f %8.1f%% %8.1f%%\n", b, claim[b], c.n,
			float64(c.minutes)/float64(c.n),
			100*float64(c.played60)/float64(c.n),
			100*float64(c.played0)/float64(c.n))
	}

	fmt.Printf("\n  played 60+ by season:\n")
	var seasons []string
	for s := range perSeason {
		seasons = append(seasons, s)
	}
	sort.Strings(seasons)
	fmt.Printf("    %-15s", "band")
	for _, s := range seasons {
		fmt.Printf(" %8s", s)
	}
	fmt.Println()
	for _, b := range order {
		fmt.Printf("    %-15s", b)
		for _, s := range seasons {
			c := perSeason[s][b]
			if c == nil || c.n == 0 {
				fmt.Printf(" %8s", "-")
				continue
			}
			fmt.Printf(" %7.1f%%", 100*float64(c.played60)/float64(c.n))
		}
		fmt.Println()
	}

	// Banked so the table above is re-derivable rather than only re-measurable.
	// The BandStrength run one PR earlier set this precedent; a calibration with
	// no row anywhere is exactly the state that made its own predecessor
	// impossible to check.
	if path := os.Getenv("FPL_CELLS"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		w := csv.NewWriter(f)
		if err := w.Write([]string{"season", "band", "n", "sum_minutes", "played60", "played0"}); err != nil {
			t.Fatal(err)
		}
		for _, s := range seasons {
			for _, b := range order {
				c := perSeason[s][b]
				if c == nil {
					continue
				}
				if err := w.Write([]string{s, b, strconv.Itoa(c.n), strconv.Itoa(c.minutes),
					strconv.Itoa(c.played60), strconv.Itoa(c.played0)}); err != nil {
					t.Fatal(err)
				}
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("\n  cells written to %s\n", path)
	}

	fmt.Printf("\n  %d player-gameweeks banded; %d (%.1f%%) had no archive row and were EXCLUDED,\n",
		total, noRow, 100*float64(noRow)/float64(total))
	fmt.Println("  which is a blank gameweek or a player not yet on the books. A genuine")
	fmt.Println("  non-appearance carries a zero-minute row and IS counted, in `played 0`.")
}
