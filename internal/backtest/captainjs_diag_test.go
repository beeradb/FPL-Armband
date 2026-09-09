package backtest

// TestDiagCaptainJamesStein — DO NOT RUN. VOID by algebra, not a live diagnostic.
//
// Common-mean shrink Score' = (1−λ)Score + λμ is strictly order-preserving for
// λ ∈ [0,1), so CaptainAndVice cannot move. The 2026-09-08 prereg was amended
// before any cell; this test Skips even with DIAG=1. The pin is
// TestCaptainOverlayHalfShrinkPreservesArgmax. Heteroscedastic B is a different
// experiment. Do not delete the Skip to "get numbers" — that mints a
// byte-identical CSV.

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"armband/internal/analysis"
)

type captainJSCell struct {
	season, squadHash string
	start, horizon    int
	lambda            float64
	holdPoints        int
	weeks, differs    int
	ungatedCaptainID  int // last week's shipped captain (summary, not per-week)
	gatedCaptainID    int // last week's arm captain
}

func TestDiagCaptainJamesStein(t *testing.T) {
	// ⚠️ AMENDED 2026-09-08, before any cell was scored. Common-mean shrink
	// Score' = (1−λ)Score + λμ is strictly order-preserving for λ ∈ [0,1):
	// Score'_i − Score'_j = (1−λ)(Score_i − Score_j). The 36-cell overlay would
	// be byte-identical to A0 — VOID by algebra, not a null. Do not spend the
	// grid. Heteroscedastic B (a different shrink per player) is a different
	// experiment and needs its own prereg. Pin: TestCaptainOverlayHalfShrinkPreservesArgmax.
	t.Skip("VOID by algebra: common-mean shrink cannot change CaptainAndVice for λ∈[0,1)")
	requireDiag(t)
	cfg := loadConfig(t)
	starts := sweepStarts()
	pairs := loadPairsOrSkip(t, cfg)

	lambdas := []float64{0, 0.25, 0.50, 0.75}
	var cells []captainJSCell

	type seasonDelta struct {
		delta   map[float64]int
		differs map[float64]int
		weeks   map[float64]int
	}
	bySeason := map[string]*seasonDelta{}
	var seasonOrder []string

	fmt.Printf("\n=== captain James–Stein linear shrink (HOLD overlay)\n")
	fmt.Printf("Score' = (1−λ)·Score + λ·mean(XI); captain/vice = CaptainAndVice(Score').\n")
	fmt.Printf("λ=0 identity against HoldCaptaincyWeekly.Captain is a Fatal, not a warning.\n")
	fmt.Printf("%s × 4 λ. Go prints season deltas only; R owns inference.\n\n",
		gridLabel(len(pairs), len(starts)))

	for _, pair := range pairs {
		for _, start := range starts {
			sc := seasonConfig(cfg, pair.Name, start, false)
			res, err := Simulate(pair.Cur, pair.Prior, sc)
			if err != nil {
				t.Fatalf("%s@%d: Simulate: %v", pair.Name, start, err)
			}
			held := res.OpeningSquad
			hc := HoldCaptaincyWeekly(pair.Cur, pair.Prior, sc, held)
			shipTotal := sumInts(hc.Full)
			hash := squadHash(held)

			armPts := map[float64]int{}
			armDiffers := map[float64]int{}
			lastUngated, lastGated := map[float64]int{}, map[float64]int{}

			for i, gw := range hc.GW {
				e := holdWeekEngine(pair.Cur, pair.Prior, sc, gw)
				xiM := xiMetricsFromIDs(e, hc.XI[i])
				if len(xiM) != 11 {
					t.Fatalf("%s@%d GW%d: xi metrics length %d, want 11",
						pair.Name, start, gw, len(xiM))
				}
				xiP := idsToPlayers(pair.Cur, hc.XI[i])
				benchP := idsToPlayers(pair.Cur, hc.Bench[i])

				for _, lam := range lambdas {
					shrunk := shrinkXIScores(xiM, lam)
					cap, vice := analysis.CaptainAndVice(shrunk)
					if lam == 0 && cap.ID != hc.Captain[i] {
						t.Fatalf("%s@%d GW%d: λ=0 captain %d != HoldCaptaincyWeekly's %d "+
							"— shrink/CaptainAndVice disagrees with the shipped walk",
							pair.Name, start, gw, cap.ID, hc.Captain[i])
					}
					pts := weekScore(xiP, benchP, gw, cap.ID, vice.ID).Points
					armPts[lam] += pts
					if cap.ID != hc.Captain[i] {
						armDiffers[lam]++
					}
					lastUngated[lam] = hc.Captain[i]
					lastGated[lam] = cap.ID
				}
			}

			agg, ok := bySeason[pair.Name]
			if !ok {
				agg = &seasonDelta{
					delta:   map[float64]int{},
					differs: map[float64]int{},
					weeks:   map[float64]int{},
				}
				bySeason[pair.Name] = agg
				seasonOrder = append(seasonOrder, pair.Name)
			}

			for _, lam := range lambdas {
				pts := armPts[lam]
				if lam == 0 && pts != shipTotal {
					t.Fatalf("%s@%d: λ=0 hold_points %d != sum(hc.Full) %d",
						pair.Name, start, pts, shipTotal)
				}
				cells = append(cells, captainJSCell{
					season: pair.Name, start: start, horizon: len(hc.GW),
					lambda: lam, holdPoints: pts,
					weeks: len(hc.GW), differs: armDiffers[lam],
					ungatedCaptainID: lastUngated[lam],
					gatedCaptainID:   lastGated[lam],
					squadHash:        hash,
				})
				agg.delta[lam] += pts - shipTotal
				agg.differs[lam] += armDiffers[lam]
				agg.weeks[lam] += len(hc.GW)
			}

			fmt.Printf("%-9s start=%02d  A0=%4d", pair.Name, start, shipTotal)
			for _, lam := range lambdas[1:] {
				fmt.Printf("  λ=%.2f Δ=%+4d differs=%d/%d",
					lam, armPts[lam]-shipTotal, armDiffers[lam], len(hc.GW))
			}
			fmt.Printf("\n")
		}
	}

	fmt.Printf("\n--- per-season summed deltas (arm − A0) ---\n")
	fmt.Printf("%-9s", "season")
	for _, lam := range lambdas[1:] {
		fmt.Printf("  %8s %10s", fmt.Sprintf("Δλ=%.2f", lam), "differs")
	}
	fmt.Printf("\n")
	for _, name := range seasonOrder {
		agg := bySeason[name]
		fmt.Printf("%-9s", name)
		for _, lam := range lambdas[1:] {
			fmt.Printf("  %+8d %5d/%-4d",
				agg.delta[lam], agg.differs[lam], agg.weeks[lam])
			if agg.differs[lam] == 0 {
				fmt.Printf("  [VOID: λ=%.2f never differed in this season]", lam)
			}
		}
		fmt.Printf("\n")
	}
	fmt.Printf("(VOID printed when weeks_captain_differs is 0 for λ>0 across a season;\n")
	fmt.Printf(" that is not called a null. No SE/t/verdict from Go — see stats/captain_js.R)\n")

	if path := os.Getenv("FPL_CELLS"); path != "" {
		if err := writeCaptainJSCells(path, cells); err != nil {
			t.Fatalf("writing FPL_CELLS: %v", err)
		}
		fmt.Printf("wrote %d rows to %s\n", len(cells), path)
	}
}

func writeCaptainJSCells(path string, cells []captainJSCell) error {
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
		"season", "start", "horizon_weeks", "lambda",
		"hold_points", "weeks_total", "weeks_captain_differs",
		"ungated_captain_id", "gated_captain_id", "squad_hash",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, c := range cells {
		if err := w.Write([]string{
			c.season,
			strconv.Itoa(c.start),
			strconv.Itoa(c.horizon),
			strconv.FormatFloat(c.lambda, 'f', 2, 64),
			strconv.Itoa(c.holdPoints),
			strconv.Itoa(c.weeks),
			strconv.Itoa(c.differs),
			strconv.Itoa(c.ungatedCaptainID),
			strconv.Itoa(c.gatedCaptainID),
			c.squadHash,
		}); err != nil {
			return err
		}
	}
	return w.Error()
}
