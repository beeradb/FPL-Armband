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

// And how far is the RECONSTRUCTION from the same source, in the seasons where
// only it exists? If the source is accurate where a truth exists, then in the
// repaired seasons the gap between the two is the RECONSTRUCTION's error.
//
//	DIAG=1 FPL_XGC_EXTERNAL_DIR=<dir> go test ./internal/backtest \
//	    -run TestDiagReconstructionAgainstExternal -v -timeout 20m
func TestDiagReconstructionAgainstExternal(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	dir := externalXGCDir()
	if dir == "" {
		t.Skip("no external xGC directory configured")
	}
	cfg := loadConfig(t)

	fmt.Printf("\n=== the RECONSTRUCTION against the same external source,\n")
	fmt.Printf("in the repaired seasons where no native truth exists.\n")
	fmt.Printf("Read beside the native comparison: same units, same proration.\n\n")
	fmt.Printf("%-9s %8s %8s %8s %8s %8s %8s\n",
		"season", "rows", "recon", "external", "bias", "MAE", "corr")

	var allR, allE []float64
	for _, name := range []string{"2020-21", "2021-22"} {
		s := loadSeason(t, cfg, name)
		// The reconstruction, computed the way applyXGCRepair computes it.
		rec, _ := reconstructedXGC(s, xgcScale)
		club, matches, err := externalClubXGC(s, dir)
		if err != nil || matches == 0 {
			t.Errorf("%s: %v", name, err)
			continue
		}
		ext, _ := proratedClubXGC(s, club)

		var r, e []float64
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for _, gw := range sortedGameweeks(ext[id]) {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes <= 0 || rec[id][gw] <= 0 || ext[id][gw] <= 0 {
					continue
				}
				r, e = append(r, rec[id][gw]), append(e, ext[id][gw])
			}
		}
		if len(r) < 2 {
			continue
		}
		allR, allE = append(allR, r...), append(allE, e...)
		fmt.Printf("%-9s %8d %8.3f %8.3f %+8.3f %8.3f %8.3f\n",
			name, len(r), meanOf(r), meanOf(e), meanOf(e)-meanOf(r), maeOf(e, r), corrOf(e, r))
	}
	if len(allR) < 2 {
		t.Skip("no comparable rows")
	}
	fmt.Printf("\n%-9s %8d %8.3f %8.3f %+8.3f %8.3f %8.3f\n",
		"ALL", len(allR), meanOf(allR), meanOf(allE),
		meanOf(allE)-meanOf(allR), maeOf(allE, allR), corrOf(allE, allR))
	fmt.Printf("\nMAE as a share of the reconstruction mean: %.1f%%\n",
		100*maeOf(allE, allR)/meanOf(allR))
	fmt.Printf("\n⚠️ DO NOT read this gap as the reconstruction's error. A claim that\n")
	fmt.Printf("it was — retracted 2026-08-25 — died on the three-way below: run in the\n")
	fmt.Printf("NATIVE seasons, where both can be checked, the reconstruction is as\n")
	fmt.Printf("accurate as the source (14.0%% MAE against 13.1%%) and LESS biased\n")
	fmt.Printf("(-0.1%% against -0.4%%), and the two agree to 5.5%%. The gap here is the\n")
	fmt.Printf("xG PROVIDER — the repaired seasons feed the chain Understat with a\n")
	fmt.Printf("borrowed offset, whose 4-5%% envelope xgcrepair.go:177-186 records —\n")
	fmt.Printf("and not the estimator. Use TestDiagThreeWayXGC.\n")
}

// All three quantities on the SAME rows, in the seasons where a truth exists.
//
//	DIAG=1 FPL_XGC_EXTERNAL_DIR=<dir> go test ./internal/backtest \
//	    -run TestDiagThreeWayXGC -v -timeout 20m
//
// ⚠️ **This is the comparison the other two should have been.** Measuring the
// source against native in one set of seasons and the reconstruction against the
// source in a DIFFERENT set produced an apparent 13.1%-against-17.4% gap that is
// really two confounds: different seasons, and — the larger one — a different xG
// PROVIDER, since the repaired seasons feed the chain Understat rather than Opta.
// A finding built on that gap was published and retracted the same day.
//
// Run on one population it says the opposite: the two estimators are a dead heat.
func TestDiagThreeWayXGC(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	dir := externalXGCDir()
	if dir == "" {
		t.Skip("no external xGC directory configured")
	}
	cfg := loadConfig(t)

	names := make([]string, 0, len(nativeXGCSeasons))
	for n := range nativeXGCSeasons {
		names = append(names, n)
	}
	sort.Strings(names)

	var nat, ext, rec []float64
	for _, name := range names {
		s := loadSeason(t, cfg, name)
		club, matches, err := externalClubXGC(s, dir)
		if err != nil || matches == 0 {
			t.Errorf("%s: %v", name, err)
			continue
		}
		e, _ := proratedClubXGC(s, club)
		// reconstructedXGC is NOT windowed by xgRepairs — only applyXGCRepair
		// consults that table — so it can be computed for a native season and
		// compared against the truth it would have replaced.
		r, _ := reconstructedXGC(s, xgcScale)
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			for _, gw := range sortedGameweeks(e[id]) {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes <= 0 || g.XGC <= 0 || e[id][gw] <= 0 || r[id][gw] <= 0 {
					continue
				}
				nat, ext, rec = append(nat, g.XGC), append(ext, e[id][gw]), append(rec, r[id][gw])
			}
		}
	}
	if len(nat) < 2 {
		t.Skip("no comparable rows")
	}
	fmt.Printf("\n=== all three xGC quantities on the SAME %d rows, native seasons\n\n", len(nat))
	show := func(est, ref []float64, lab string) {
		fmt.Printf("  %-18s bias %+6.1f%%   MAE %5.1f%%   corr %.3f\n", lab,
			100*(meanOf(est)-meanOf(ref))/meanOf(ref),
			100*maeOf(est, ref)/meanOf(ref), corrOf(est, ref))
	}
	show(ext, nat, "source vs native")
	show(rec, nat, "reconstruction vs native")
	show(rec, ext, "reconstruction vs source")

	fmt.Printf("\n⚠️ The two estimators are a DEAD HEAT against FPL's own xGC, and the\n")
	fmt.Printf("reconstruction is the LESS biased of the two. Neither is shown better.\n")
	fmt.Printf("⚠️ So nothing here explains the standard-error doubling the chip arms\n")
	fmt.Printf("show under the source, and nothing here distinguishes 'the\n")
	fmt.Printf("reconstruction borrows strength' from 'the source injects variance'.\n")
	fmt.Printf("An ACCURACY test cannot: borrowing strength is a claim about\n")
	fmt.Printf("CORRELATION between the estimate and the scored path. That is open.\n")
}
