package backtest

// Where exactly does the model over-predict a high-defcon defender?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDefconWhere -v
//
// The bias decomposition says 81% of a defender's defensive-contribution term
// is already priced elsewhere. The write-up blamed the negative correlation
// between defensive actions and clean sheets — a side without the ball concedes
// more *and* clears more — but that was reasoning, not measurement, and there
// is a rival explanation that fits the same evidence: defcon rates regress, so
// a defender with a high first-half rate simply does not sustain it and the
// model is wrong about the term itself rather than about its interaction.
//
// The two are separable, because both halves of the prediction can be compared
// against what actually happened. A defender's defensive-contribution points
// are 2 whenever he records ten actions in a match, and his clean-sheet points
// are 4 whenever his side keeps one — both directly countable from the archive.
// So the question becomes arithmetic: of the over-prediction, how much sits in
// the defcon component and how much in the clean sheet?

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagDefconWhere(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	cur, err := Load(ctx, cfg.CacheDir, "2025-26")
	if err != nil {
		t.Fatal(err)
	}
	prior, err := Load(ctx, cfg.CacheDir, "2024-25")
	if err != nil {
		t.Fatal(err)
	}

	const cut = 19
	e, boot := EngineAt(cur, prior, cut, SimConfig{Weights: cfg.Weights})

	type row struct {
		dc90               float64 // what the model believed going in
		dcTerm, csTerm     float64 // predicted, per 90
		dcActual, csActual float64 // realised, per 90
		dcRateAfter        float64 // his actual defcon rate in the second half
		xgc90              float64
	}
	var rows []row

	for i := range boot.Elements {
		el := &boot.Elements[i]
		if el.ElementType != 2 || el.Minutes < 600 {
			continue // defenders only: the effect is theirs
		}
		m := e.Metrics(el)
		p := cur.Players[el.ID]
		if p == nil {
			continue
		}
		var mins, dcPts, csPts, dcActions float64
		for gw := cut + 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.Minutes == 0 {
				continue
			}
			mins += float64(g.Minutes)
			dcActions += float64(g.DefCon)
			if g.DefCon >= 10 {
				dcPts += 2
			}
			if g.CleanSheets > 0 && g.Minutes >= 60 {
				csPts += 4
			}
		}
		if mins < 450 {
			continue
		}
		per90 := 90 / mins
		rows = append(rows, row{
			dc90:        m.DefCon90,
			dcTerm:      analysis.DefconTermFor(2, m.DefCon90),
			csTerm:      analysis.CleanSheetTermFor(2, m.XGC90),
			dcActual:    dcPts * per90,
			csActual:    csPts * per90,
			dcRateAfter: dcActions * per90,
			xgc90:       m.XGC90,
		})
	}
	if len(rows) < 30 {
		t.Skipf("only %d defenders clear the filters", len(rows))
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].dc90 < rows[j].dc90 })
	fmt.Printf("\nWhere the over-prediction of a high-defcon defender actually sits.\n")
	fmt.Printf("2025-26, model through GW%d, realised GW%d-38, %d defenders.\n\n",
		cut, cut+1, len(rows))
	fmt.Printf("%-14s %4s %7s %8s %8s %8s %8s %8s %8s %8s\n",
		"dc/90 group", "n", "dc/90", "dc pred", "dc real", "dc miss",
		"cs pred", "cs real", "cs miss", "dc after")

	q := len(rows) / 3
	for b := 0; b < 3; b++ {
		lo, hi := b*q, (b+1)*q
		if b == 2 {
			hi = len(rows)
		}
		var dc, dcT, dcA, csT, csA, after float64
		for _, r := range rows[lo:hi] {
			dc += r.dc90
			dcT += r.dcTerm
			dcA += r.dcActual
			csT += r.csTerm
			csA += r.csActual
			after += r.dcRateAfter
		}
		n := float64(hi - lo)
		fmt.Printf("%-14s %4.0f %7.2f %8.3f %8.3f %+8.3f %8.3f %8.3f %+8.3f %8.2f\n",
			[]string{"lowest", "middle", "highest"}[b], n, dc/n,
			dcT/n, dcA/n, (dcA-dcT)/n, csT/n, csA/n, (csA-csT)/n, after/n)
	}

	fmt.Printf("\n'miss' is realised minus predicted, so negative is over-prediction.\n")
	fmt.Printf("If the correlation story is right the damage is in the clean-sheet column.\n")
	fmt.Printf("If defcon rates simply regress, it is in the defcon column and 'dc after'\n")
	fmt.Printf("falls short of the 'dc/90' the model went in believing.\n")
}
