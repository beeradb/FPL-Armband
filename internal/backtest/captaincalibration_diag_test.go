package backtest

// TestDiagCaptainCalibration — A0 / C / F sizing rungs of the 2026-09-01 prereg
// memory/2026-09-01-prereg-does-an-ownership-censored-captain-rule-beat-argmax-xp.md
// (run ONLY A0, C, F; ownership-censor A1–A4 are not implemented here).
//
//	DIAG=1 EXP=CALIB FPL_CELLS=/tmp/captain-cal/cells.csv FPL_SWEEP_SEASONS=extended \
//	  scripts/replay -run '^TestDiagCaptainCalibration$' -v -timeout 3h
//	Rscript stats/captain_rule.R /tmp/captain-cal/cells.csv
//
// HOLD overlay: one Simulate for the opening fifteen, one HoldCaptaincyWeekly
// for A0 (Full) and F (FixedCaptain) in the same pass, then a cheap C rescore
// with bestArmband on that week's XI. Go prints per-season totals and the six
// season means of (F−A0) and (C−A0) only — no SE, t, threshold or verdict word.
// Inference is stats/captain_rule.R (per_path estimand).
//
// Banked cells at stats/snapshots/2026-09-08-captain-cal/legacy/ were measured
// at 4f5aa59c, before #205 shipped HOLD's weekly pick at horizon 1. Re-running
// this diagnostic now measures shipped HOLD, not those cells.

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type captainCalCell struct {
	season                    string
	start, horizon            int
	a0, f, c                  int
	weeks, weeksOracleDiffers int
	squadHash                 string
}

func TestDiagCaptainCalibration(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	starts := sweepStarts()
	pairs := loadPairsOrSkip(t, cfg)

	var cells []captainCalCell
	type seasonAgg struct {
		a0, f, c int
		n        int
		fMinus   []float64
		cMinus   []float64
	}
	bySeason := map[string]*seasonAgg{}
	var seasonOrder []string

	fmt.Printf("\n=== captain calibration A0/C/F (HOLD overlay, chips off)\n")
	fmt.Printf("A0 = HoldCaptaincyWeekly.Full; F = FixedCaptain (same pass);\n")
	fmt.Printf("C = bestArmband on that week's XI. Identity: pickXI captain == hc.Captain.\n")
	fmt.Printf("%s. Go prints totals only; R owns inference.\n\n",
		gridLabel(len(pairs), len(starts)))

	for _, pair := range pairs {
		for _, start := range starts {
			sc := seasonConfig(cfg, pair.Name, start, false) // WeeklyXI false; chips empty
			res, err := Simulate(pair.Cur, pair.Prior, sc)
			if err != nil {
				t.Fatalf("%s@%d: Simulate: %v", pair.Name, start, err)
			}
			held := res.OpeningSquad
			hc := HoldCaptaincyWeekly(pair.Cur, pair.Prior, sc, held)

			a0 := sumInts(hc.Full)
			fPts := sumInts(hc.FixedCaptain)

			var cPts, oracleDiffers int
			for i, gw := range hc.GW {
				e := holdWeekEngine(pair.Cur, pair.Prior, sc, gw)
				_, _, shipCap, _ := pickXIAt(e, held, gw, false)
				if shipCap != hc.Captain[i] {
					t.Fatalf("%s@%d GW%d: pickXIAt captain %d != HoldCaptaincyWeekly's %d "+
						"— the duplicated loop has drifted from the reference",
						pair.Name, start, gw, shipCap, hc.Captain[i])
				}
				ocap, ovice := bestArmband(pair.Cur, hc.XI[i], gw)
				if ocap != hc.Captain[i] {
					oracleDiffers++
				}
				xiP := idsToPlayers(pair.Cur, hc.XI[i])
				benchP := idsToPlayers(pair.Cur, hc.Bench[i])
				cPts += weekScore(xiP, benchP, gw, ocap, ovice).Points
			}
			if oracleDiffers == 0 {
				t.Errorf("%s@%d: C never differed from A0 across %d weeks — "+
					"wiring dead, not a null", pair.Name, start, len(hc.GW))
			}

			row := captainCalCell{
				season: pair.Name, start: start, horizon: len(hc.GW),
				a0: a0, f: fPts, c: cPts,
				weeks: len(hc.GW), weeksOracleDiffers: oracleDiffers,
				squadHash: squadHash(held),
			}
			cells = append(cells, row)

			agg, ok := bySeason[pair.Name]
			if !ok {
				agg = &seasonAgg{}
				bySeason[pair.Name] = agg
				seasonOrder = append(seasonOrder, pair.Name)
			}
			agg.a0 += a0
			agg.f += fPts
			agg.c += cPts
			agg.n++
			agg.fMinus = append(agg.fMinus, float64(fPts-a0))
			agg.cMinus = append(agg.cMinus, float64(cPts-a0))

			fmt.Printf("%-9s start=%02d  A0=%4d  F=%4d  C=%4d  (F-A0=%+5d C-A0=%+5d)  "+
				"oracle_differs=%d/%d\n",
				pair.Name, start, a0, fPts, cPts, fPts-a0, cPts-a0,
				oracleDiffers, len(hc.GW))
		}
	}

	fmt.Printf("\n--- per-season summed totals (across that season's entry points) ---\n")
	fmt.Printf("%-9s %5s %6s %6s %6s  mean(F-A0) mean(C-A0)\n",
		"season", "n", "A0", "F", "C")
	var fMeans, cMeans []float64
	for _, name := range seasonOrder {
		agg := bySeason[name]
		fMean := meanFloats(agg.fMinus)
		cMean := meanFloats(agg.cMinus)
		fMeans = append(fMeans, fMean)
		cMeans = append(cMeans, cMean)
		fmt.Printf("%-9s %5d %6d %6d %6d  %+10.2f %+10.2f\n",
			name, agg.n, agg.a0, agg.f, agg.c, fMean, cMean)
	}
	fmt.Printf("\n%s means of (F−A0):", seasonsLabel(len(fMeans)))
	for _, m := range fMeans {
		fmt.Printf(" %+.2f", m)
	}
	fmt.Printf("\n%s means of (C−A0):", seasonsLabel(len(cMeans)))
	for _, m := range cMeans {
		fmt.Printf(" %+.2f", m)
	}
	fmt.Printf("\n(No SE/t/threshold/verdict from Go — see stats/captain_rule.R)\n")

	if path := os.Getenv("FPL_CELLS"); path != "" {
		if err := writeCaptainCalCells(path, cells); err != nil {
			t.Fatalf("writing FPL_CELLS: %v", err)
		}
		fmt.Printf("wrote %d cells to %s\n", len(cells), path)
	}
}

func meanFloats(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func writeCaptainCalCells(path string, cells []captainCalCell) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{
		"season", "start", "horizon_weeks",
		"hold_points", "hold_fixedcap_points", "hold_oracle_points",
		"weeks_total", "weeks_oracle_differs", "squad_hash",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, c := range cells {
		if err := w.Write([]string{
			c.season,
			strconv.Itoa(c.start),
			strconv.Itoa(c.horizon),
			strconv.Itoa(c.a0),
			strconv.Itoa(c.f),
			strconv.Itoa(c.c),
			strconv.Itoa(c.weeks),
			strconv.Itoa(c.weeksOracleDiffers),
			c.squadHash,
		}); err != nil {
			return err
		}
	}
	return w.Error()
}
