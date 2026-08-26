package backtest

// Is the external xGC source ACCURATE, measured against FPL's own published xGC?
//
//	DIAG=1 FPL_XGC_EXTERNAL_DIR=<dir> go test ./internal/backtest \
//	    -run TestDiagExternalXGCAgainstNative -v -timeout 20m
//
// # The question, and why nothing has answered it
//
// `xgcexternal.go` replaces a RECONSTRUCTION with a MEASUREMENT for the seasons
// FPL never backfilled. Every argument for doing that rests on the measurement
// being better, and the record's own assessment of the source establishes only
// that it is *"not worse than what ships"* — never that it is more accurate.
//
// Measured on the chip arms, the swap does not move the effect and roughly
// DOUBLES the standard error, which flips two headline findings from resolving to
// not. Two opposite readings fit that and nothing distinguishes them:
//
//   - the reconstruction BORROWS STRENGTH — it derives xGC from opponents'
//     expected goals inside the same archive the squad is scored on, so it is
//     mechanically tied to the scoring inputs and may understate real variance; or
//   - the external source INJECTS variance those seasons do not have.
//
// Under the first, switching is correct and painful. Under the second, the
// instrument was degraded for nothing.
//
// # What makes this decidable
//
// **The external source covers seasons FPL DOES publish xGC for.** 2023-24,
// 2024-25 and 2025-26 carry native `expected_goals_conceded` and are also in the
// external bank — so in those seasons there is a ground truth to check against,
// and the reconstruction is not involved at all.
//
// If the external source reproduces native xGC closely, it is accurate, the extra
// variance in the repaired seasons is real, and the findings that stop resolving
// under it were never as resolved as they looked. If it does not, the extra
// variance is the source's own.
//
// ⚠️ **This is NOT the comparison the 2026-08-24 assessment already ran.** That
// one compared the source's opponent-xG against FPL's xGC at TEAM-gameweek level
// and found an 18.3% disagreement it called definitional. This runs the quantity
// the ENGINE actually consumes — per player-gameweek, after minute proration,
// through `proratedClubXGC`, the same function the overlay uses — because a
// definitional gap at team level can be large and still not move a squad.
//
// ⚠️ **A close agreement here does NOT make the source more accurate than the
// reconstruction.** It makes it accurate where FPL publishes a truth to check
// against. The repaired seasons have no such truth, which is the whole reason
// they are repaired, and no measurement can close that gap from inside.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
)

// The seasons compared are `nativeXGCSeasons` (csregressor_test.go) — FPL's own
// weekly expected_goals_conceded rather than a reconstruction, and all three are
// in the external bank, so both quantities exist for the same player-gameweek.
//
// ⚠️ **Reused rather than re-declared.** A second enumeration of "which seasons
// are native" is one quantity with two implementations, and its exclusion of
// 2022-23 — native only from GW16, so mixing its halves would compare a repaired
// window against a native one and call the difference provider error — is exactly
// the reasoning this comparison needs.

func TestDiagExternalXGCAgainstNative(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	dir := externalXGCDir()
	if dir == "" {
		t.Skip("no external xGC directory configured")
	}
	cfg := loadConfig(t)

	fmt.Printf("\n=== is the external xGC source accurate where FPL publishes a truth?\n")
	fmt.Printf("Per player-gameweek, after minute proration, against native\n")
	fmt.Printf("expected_goals_conceded. The reconstruction is not involved.\n\n")
	fmt.Printf("%-9s %8s %8s %8s %8s %8s %8s\n",
		"season", "rows", "native", "external", "bias", "MAE", "corr")

	var allNative, allExt []float64
	names := make([]string, 0, len(nativeXGCSeasons))
	for n := range nativeXGCSeasons {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		s := loadSeason(t, cfg, name)
		club, matches, err := externalClubXGC(s, dir)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if matches == 0 {
			continue
		}
		ext, _ := proratedClubXGC(s, club)

		var nat, got []float64
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for _, gw := range sortedGameweeks(ext[id]) {
				g, ok := p.GWs[gw]
				// Native rows only: a zero is FPL not publishing, not a real zero,
				// and including them would compare a number against an absence.
				if !ok || g.Minutes <= 0 || g.XGC <= 0 {
					continue
				}
				if ext[id][gw] <= 0 {
					continue
				}
				nat, got = append(nat, g.XGC), append(got, ext[id][gw])
			}
		}
		if len(nat) < 2 {
			continue
		}
		allNative, allExt = append(allNative, nat...), append(allExt, got...)
		fmt.Printf("%-9s %8d %8.3f %8.3f %+8.3f %8.3f %8.3f\n",
			name, len(nat), meanOf(nat), meanOf(got),
			meanOf(got)-meanOf(nat), maeOf(got, nat), corrOf(got, nat))
	}

	if len(allNative) < 2 {
		t.Skip("no comparable rows")
	}
	bias := meanOf(allExt) - meanOf(allNative)
	fmt.Printf("\n%-9s %8d %8.3f %8.3f %+8.3f %8.3f %8.3f\n",
		"ALL", len(allNative), meanOf(allNative), meanOf(allExt),
		bias, maeOf(allExt, allNative), corrOf(allExt, allNative))
	fmt.Printf("\nbias as a share of the native mean: %+.1f%%\n",
		100*bias/meanOf(allNative))
	fmt.Printf("MAE as a share of the native mean:  %.1f%%\n",
		100*maeOf(allExt, allNative)/meanOf(allNative))

	fmt.Printf("\n⚠️ Read this as accuracy WHERE A TRUTH EXISTS. The repaired seasons\n")
	fmt.Printf("have none — that is why they are repaired — so a close agreement here\n")
	fmt.Printf("licenses trusting the source, NOT a claim that it beats the\n")
	fmt.Printf("reconstruction on seasons neither can be checked against.\n")
	fmt.Printf("⚠️ NO INFERENCE HERE: season-clustered SEs live in sweep_inference.R.\n")
}

// maeOf is the mean absolute difference. Local, descriptive, and deliberately not
// an estimator — see the closing note above and TestInferenceLivesInOnePlace.
func maeOf(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var s float64
	for i := range a {
		s += math.Abs(a[i] - b[i])
	}
	return s / float64(len(a))
}
