package backtest

// Does a keeper's save rate say something about his clean sheet that his team's
// expected goals conceded does not?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagSavesClean -v
//
// The same question just answered for defenders, where the answer was yes: the
// model predicted the same clean-sheet value for every defcon group while what
// they collected differed by 27%. A keeper making six saves a game is under
// pressure in exactly the way a defender making ten clearances is, and the model
// prices his saves from his own rate and his clean sheet from his team's xGC
// with nothing linking them.
//
// This one can be measured properly. Defensive contribution exists in a single
// season; saves exist in all of them, so this runs over four and does not rest
// on twenty-two keepers.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

func TestDiagSavesClean(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	ctx := context.Background()

	type row struct {
		saves90            float64
		svTerm, csTerm     float64
		svActual, csActual float64
	}
	var rows []row

	pairs := xgPairNames()
	for _, pair := range pairs {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatal(err)
		}
		const cut = 19
		e, boot := EngineAt(cur, prior, cut, SimConfig{Weights: cfg.Weights})

		for i := range boot.Elements {
			el := &boot.Elements[i]
			if el.ElementType != 1 || el.Minutes < 600 {
				continue
			}
			m := e.Metrics(el)
			p := cur.Players[el.ID]
			if p == nil {
				continue
			}
			var mins, svPts, csPts float64
			for gw := cut + 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes == 0 {
					continue
				}
				mins += float64(g.Minutes)
				svPts += float64(g.Saves / 3) // FPL pays a point per three saves
				if g.CleanSheets > 0 && g.Minutes >= 60 {
					csPts += 4
				}
			}
			if mins < 450 {
				continue
			}
			per90 := 90 / mins
			rows = append(rows, row{
				saves90:  m.Saves90,
				svTerm:   analysis.SavesTermFor(m.Saves90),
				csTerm:   analysis.CleanSheetTermFor(1, m.XGC90),
				svActual: svPts * per90,
				csActual: csPts * per90,
			})
		}
	}
	if len(rows) < 30 {
		t.Skipf("only %d keepers clear the filters", len(rows))
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].saves90 < rows[j].saves90 })
	fmt.Printf("\nKeepers: does the save rate predict the clean sheet beyond team xGC?\n")
	// Season PAIRS, not seasons: this fits on the prior season and scores on the
	// played one, so three pairs span four distinct seasons and seasonsLabel
	// would name the wrong unit.
	fmt.Printf("%d season pairs, model through GW19, realised GW20-38, %d keeper-seasons.\n\n",
		len(pairs), len(rows))
	fmt.Printf("%-10s %5s %8s %9s %9s %8s %9s %9s %8s %9s\n",
		"saves/90", "n", "sv/90", "sv pred", "sv real", "sv miss", "cs pred", "cs real",
		"cs miss", "total")

	q := len(rows) / 3
	for b := 0; b < 3; b++ {
		lo, hi := b*q, (b+1)*q
		if b == 2 {
			hi = len(rows)
		}
		var sv, svT, svA, csT, csA float64
		for _, r := range rows[lo:hi] {
			sv += r.saves90
			svT += r.svTerm
			svA += r.svActual
			csT += r.csTerm
			csA += r.csActual
		}
		n := float64(hi - lo)
		fmt.Printf("%-10s %5.0f %8.2f %9.3f %9.3f %+8.3f %9.3f %9.3f %+8.3f %+9.3f\n",
			[]string{"lowest", "middle", "highest"}[b], n, sv/n,
			svT/n, svA/n, (svA-svT)/n, csT/n, csA/n, (csA-csT)/n,
			((svA-svT)+(csA-csT))/n)
	}
	fmt.Printf("\n'miss' is realised minus predicted. If the save rate carries clean-sheet\n")
	fmt.Printf("information the team rate does not, the cs miss falls as saves rise.\n")
}
