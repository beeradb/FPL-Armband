package backtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The measured per-match source is a PRIVATE cache and is not in this
// repository, so every test here skips without it. That is deliberate and it is
// the same shape as the archive-dependent tests around it: `go test ./...` must
// pass on a machine that has neither.
//
// ⚠️ A skip is not a pass. `TestExternalXGCIsOffByDefault` runs everywhere and is
// what holds the public default in place; the rest verify the arm when its data
// is present.
func externalDirOrSkip(t *testing.T) string {
	t.Helper()
	d := externalXGCDir()
	if d == "" {
		t.Skip("no external xGC directory configured")
	}
	if _, err := os.Stat(d); err != nil {
		t.Skipf("external xGC directory %s not readable: %v", d, err)
	}
	return d
}

// The public default, and the only test here that runs on every machine.
//
// It pins the thing a licensing constraint depends on: a clone with no
// configured directory reads the reconstruction and nothing else. If this fails,
// a public clone has started reading a source it does not have.
func TestExternalXGCIsOffByDefault(t *testing.T) {
	t.Setenv("FPL_XGC_EXTERNAL_DIR", "")
	SetXGCExternalDir("")
	if got := externalXGCDir(); got != "" {
		t.Fatalf("external xGC resolved to %q with nothing configured", got)
	}
	s := &Season{Name: "2021-22", Players: map[int]*Player{}}
	res, err := s.applyExternalXGC()
	if err != nil {
		t.Fatalf("applyExternalXGC with no source: %v", err)
	}
	if res.Dir != "" || res.Applied != 0 || res.Matches != 0 {
		t.Fatalf("external xGC did work with nothing configured: %+v", res)
	}
}

// ⚠️ Naming a directory that is not there must ERROR, not fall back.
//
// The failure this guards is the one this package has produced twice: a switch
// that degrades quietly leaves a run unable to say which arm it measured, and
// two incomparable sweeps then look like one.
func TestExternalXGCMissingDataIsAHardError(t *testing.T) {
	t.Setenv("FPL_XGC_EXTERNAL_DIR", filepath.Join(t.TempDir(), "absent"))
	s := &Season{Name: "2021-22", Players: map[int]*Player{}}
	if _, err := s.applyExternalXGC(); err == nil {
		t.Fatal("a configured directory that does not exist was accepted — " +
			"the external source must never fall back to the reconstruction silently")
	}

	// A season the source does not cover is a DECISION, not a failure: 2023-24
	// carries native xGC and needs nothing.
	native := &Season{Name: "2023-24", Players: map[int]*Player{}}
	if _, err := native.applyExternalXGC(); err != nil {
		t.Fatalf("an uncovered season errored: %v", err)
	}
}

// Every club the source names must resolve, in every season it covers.
//
// This is the guard the whole join rests on. The two naming schemes agree on
// fewer than half the league, so an unmapped club does not announce itself — it
// removes a team from the comparison and leaves the rest looking healthy.
func TestExternalXGCClubNamesAllResolve(t *testing.T) {
	dir := externalDirOrSkip(t)
	cfg := loadConfig(t)
	for season := range xgcExternalSeasons {
		if _, err := os.Stat(externalXGCPath(dir, season)); err != nil {
			t.Errorf("%s is declared covered but has no file: %v", season, err)
			continue
		}
		s := loadSeason(t, cfg, season)
		clubs, matches, err := externalClubXGC(s, dir)
		if err != nil {
			t.Errorf("%s: %v", season, err)
			continue
		}
		if matches != 380 {
			t.Errorf("%s: joined %d matches, want a full 380 — a short join is a "+
				"silently smaller comparison", season, matches)
		}
		// Twenty clubs, each with at least one gameweek.
		seen := map[int]bool{}
		for _, k := range sortedClubKeys(clubs) {
			seen[k[0]] = true
		}
		if len(seen) != 20 {
			t.Errorf("%s: %d clubs carry xGC, want 20", season, len(seen))
		}
	}
}

// The overlay must actually reach the rows, and the reconstruction must then
// stand down where it did.
//
// ⚠️ It checks BOTH halves. An overlay that wrote nothing and a reconstruction
// that filled everything would leave the season looking identical to the default
// arm, which is the null-that-is-really-a-no-op this package keeps catching.
func TestExternalXGCReplacesTheReconstruction(t *testing.T) {
	dir := externalDirOrSkip(t)
	cfg := loadConfig(t)
	const season = "2021-22"

	t.Setenv("FPL_XGC_EXTERNAL_DIR", "")
	base, err := Load(context.Background(), cfg.CacheDir, season)
	if err != nil {
		t.Skipf("archive unreachable: %v", err)
	}
	if base.XGCExternal.Applied != 0 {
		t.Fatalf("the default arm applied %d external rows", base.XGCExternal.Applied)
	}

	t.Setenv("FPL_XGC_EXTERNAL_DIR", dir)
	ext, err := Load(context.Background(), cfg.CacheDir, season)
	if err != nil {
		t.Fatalf("loading %s under the external source: %v", season, err)
	}
	if ext.XGCExternal.Applied == 0 {
		t.Fatal("the external arm applied no rows — the overlay is a no-op, and a " +
			"no-op that looks like a null result is this package's signature failure")
	}
	if ext.XGRepair.XGC.Applied >= base.XGRepair.XGC.Applied {
		t.Errorf("the reconstruction applied %d rows under the external source "+
			"against %d without it — writing measured values first must make it "+
			"stand down, not run alongside",
			ext.XGRepair.XGC.Applied, base.XGRepair.XGC.Applied)
	}

	// The two arms must differ on the actual quantity, or the swap is cosmetic.
	var sameRows, diffRows int
	for _, id := range sortedSeasonPlayerIDs(ext) {
		bp, ok := base.Players[id]
		if !ok {
			continue
		}
		for gw, g := range ext.Players[id].GWs {
			b, ok := bp.GWs[gw]
			if !ok || (g.XGC == 0 && b.XGC == 0) {
				continue
			}
			if g.XGC == b.XGC {
				sameRows++
			} else {
				diffRows++
			}
		}
	}
	if diffRows == 0 {
		t.Fatal("no player-gameweek's xGC differs between the two arms")
	}
	t.Logf("%s: external applied %d rows over %d matches (sum %.1f); "+
		"reconstruction fell from %d to %d rows; %d player-gameweeks differ, %d agree",
		season, ext.XGCExternal.Applied, ext.XGCExternal.Matches, ext.XGCExternal.SumXGC,
		base.XGRepair.XGC.Applied, ext.XGRepair.XGC.Applied, diffRows, sameRows)
}

// The switch must work on a CACHE HIT, which is the trap its two siblings each
// fell into: a repair applied inside `fetch` is baked into the cached bytes, and
// then the escape hatch reads a repaired cache and reports the two arms as
// identical. `repaired()` is on the right side of the cache write and this fails
// if the overlay moves.
func TestTheExternalXGCSwitchWorksOnACacheHit(t *testing.T) {
	dir := externalDirOrSkip(t)
	cfg := loadConfig(t)
	const season = "2021-22"

	// Warm the on-disk cache under the EXTERNAL arm first, so a baked-in overlay
	// would poison what the default arm then reads.
	t.Setenv("FPL_XGC_EXTERNAL_DIR", dir)
	if _, err := Load(context.Background(), cfg.CacheDir, season); err != nil {
		t.Skipf("archive unreachable: %v", err)
	}
	t.Setenv("FPL_XGC_EXTERNAL_DIR", "")
	back, err := Load(context.Background(), cfg.CacheDir, season)
	if err != nil {
		t.Fatalf("reloading %s: %v", season, err)
	}
	if back.XGCExternal.Applied != 0 {
		t.Fatalf("the default arm read %d external rows off a cache warmed under "+
			"the external arm — the overlay has moved inside fetch",
			back.XGCExternal.Applied)
	}
	if back.XGRepair.XGC.Applied == 0 {
		t.Fatal("the reconstruction applied nothing on the way back — the cached " +
			"bytes carry the overlay, which is exactly what repaired() exists to prevent")
	}
}
