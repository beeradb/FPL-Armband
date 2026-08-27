package backtest

// Who gets substituted, by position?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagSubstitution -v
//
// MinutesWeightByPosition relaxes the rotation penalty for midfielders on the
// argument that their returns accrue in the minutes they play, while a
// defender's clean sheet is all or nothing. That argument, if it holds, is
// about *being substituted* — a starter taken off at 65 minutes — rather than
// about being dropped, and it should apply to forwards too.
//
// It is worth checking before tuning a forward knob on it, because the same
// argument was accepted for midfielders without anyone measuring the premise.

import (
	"context"
	"fmt"
	"testing"
)

func TestDiagSubstitutionByPosition(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	names := map[int]string{1: "GKP", 2: "DEF", 3: "MID", 4: "FWD"}
	type acc struct {
		starts, minutes, full, sixty float64
		cameo, cameoMins             float64
	}
	by := map[int]*acc{1: {}, 2: {}, 3: {}, 4: {}}

	// Named so the header below counts this list rather than restating it.
	seasons := []string{"2022-23", "2023-24", "2024-25", "2025-26"}

	for _, sn := range seasons {
		s, err := Load(ctx, cfg.CacheDir, sn)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range s.Players {
			a := by[p.Type]
			if a == nil {
				continue
			}
			for _, g := range p.GWs {
				switch {
				case g.Starts > 0:
					a.starts++
					a.minutes += float64(g.Minutes)
					if g.Minutes >= 90 {
						a.full++
					}
					if g.Minutes >= 60 {
						a.sixty++
					}
				case g.Minutes > 0:
					a.cameo++
					a.cameoMins += float64(g.Minutes)
				}
			}
		}
	}

	fmt.Printf("\nWhat happens when a player starts, by position. %s.\n\n", seasonsLabel(len(seasons)))
	fmt.Printf("%-6s %9s %11s %11s %11s %10s %11s\n",
		"pos", "starts", "mins/start", "played 90", "played 60+", "sub apps", "mins/sub")
	for _, pos := range []int{1, 2, 3, 4} {
		a := by[pos]
		if a.starts == 0 {
			continue
		}
		fmt.Printf("%-6s %9.0f %11.1f %10.1f%% %10.1f%% %10.0f %11.1f\n",
			names[pos], a.starts, a.minutes/a.starts,
			100*a.full/a.starts, 100*a.sixty/a.starts,
			a.cameo, a.cameoMins/atLeastOne(a.cameo))
	}

	fmt.Printf("\nThe relaxation is justified by starters being taken off, so the\n")
	fmt.Printf("column that matters is mins/start — and the gap against defenders\n")
	fmt.Printf("is what a per-position scale would be pricing:\n\n")
	def := by[2].minutes / by[2].starts
	for _, pos := range []int{1, 3, 4} {
		a := by[pos]
		if a.starts == 0 {
			continue
		}
		m := a.minutes / a.starts
		fmt.Printf("  %-4s %.1f against a defender's %.1f — %+.1f minutes (%+.1f%%)\n",
			names[pos], m, def, m-def, 100*(m/def-1))
	}
}

func atLeastOne(v float64) float64 {
	if v < 1 {
		return 1
	}
	return v
}
