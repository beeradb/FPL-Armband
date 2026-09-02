package backtest

import (
	"context"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// TestBandStrengthArrivesOnTheScoredPath is the arrival check that has to run
// *before* the points sweep, not after it.
//
// `BandStrength` has the exact signature this record warns about. It is read in
// one place — `fixtureMultipliersFor` in metrics.go — and every gate between the
// config field and a player's `Score` fails **closed and silently**:
//
//   - `magnitudeDifficulty` (FPL_MAGNITUDE) bypasses the band call entirely;
//   - `teamBands` returns `ready: false` until `bandMinMatches` fixtures have
//     finished, and returns the baseline for every club when it does;
//   - `playedFixtures` strips `Finished` and the scoreline past the cutoff, and
//     the recorded bug here is precisely that an earlier `loadFixtures` dropped
//     scores wholesale, so `teamBands` saw no completed matches and *every* band
//     returned 1. That produced a clean-looking null that measured nothing.
//
// So a `BandStrength` sweep that comes back flat has two readings — "the bands
// are worth nothing" and "the bands never ran" — and only this test separates
// them. It asserts the strong form: at a mid-season cutoff, on **the engine the
// replay itself would use**, turning the knob on must move a real player's
// `Score`.
//
// It goes through `EngineAt` rather than building its own `NewEngineFull`, which
// is the difference between "an engine responds to the knob" and "the engine this
// harness scores with responds to it". `EngineAt` attaches the prior blend, the
// recency index and the team-form source the way `Simulate` does; an engine
// missing them returns the field the model reads carrying its fallback value,
// which has already produced one withdrawn figure in this project.
//
// The remaining half of the chain is read rather than executed here, and is named
// so the next reader does not take this test for more than it proves:
// `policyVariant.apply` sets `SimConfig.Weights.BandStrength`; `Simulate` hands
// `cfg.Weights` to the opening-squad, weekly-XI and free-hit engines; and
// `HoldCaptaincyWeekly` — where the HOLD metric is computed — hands it to the
// frozen-captain and per-gameweek held engines. There is no environment variable
// and no second process, so both arms toggle inside one `run_id`.
func TestBandStrengthArrivesOnTheScoredPath(t *testing.T) {
	cfg := loadConfig(t)
	cur, err := Load(context.Background(), cfg.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(context.Background(), cfg.CacheDir, "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	// Comfortably past bandMinMatches, so "not ready" cannot explain a null, and
	// still mid-season, so the cutoff is doing real work.
	const cutoff = 19
	at := func(bs float64) []struct {
		id    int
		score float64
	} {
		sc := sweepConfig(cfg, cutoff+1, false)
		sc.Weights.BandStrength = bs
		// A fresh engine per arm: teamBands is sync.Once-guarded per engine, so the
		// band cache must not be shared between the two settings.
		e, _ := EngineAt(cur, prior, cutoff, sc)
		var out []struct {
			id    int
			score float64
		}
		for _, m := range e.AllMetrics() {
			out = append(out, struct {
				id    int
				score float64
			}{m.ID, m.Score})
		}
		return out
	}

	off, on := at(0), at(1)
	if len(off) != len(on) {
		t.Fatalf("pool sizes differ: %d against %d", len(off), len(on))
	}
	moved, scored := 0, 0
	for i := range off {
		if off[i].id != on[i].id {
			t.Fatalf("pool order differs at %d: %d against %d", i, off[i].id, on[i].id)
		}
		if off[i].score == 0 && on[i].score == 0 {
			continue
		}
		scored++
		if off[i].score != on[i].score {
			moved++
		}
	}
	if scored == 0 {
		t.Fatal("no player scored above zero at the cutoff — the point-in-time " +
			"bootstrap is empty, so this test cannot see the knob either way")
	}
	if moved == 0 {
		t.Fatalf("BandStrength 0 against 1 moved 0 of %d scored players at GW%d. "+
			"The knob does not arrive on the scored path, so any points sweep of it "+
			"would return a byte-identical null that reads as 'the bands are worth "+
			"nothing'. Check FPL_MAGNITUDE, teamBands readiness, and whether "+
			"playedFixtures is still handing over finished scorelines.", scored, cutoff)
	}
	// ⚠️ Logged, never asserted, and not to be copied into a write-up. The count
	// alternates between 469 and 494 run to run: at this cutoff the third-best
	// attack is a two-way tie, so the band assignment is itself non-deterministic.
	// See banddeterminism_test.go. Only `moved > 0` is stable.
	t.Logf("BandStrength 0 -> 1 moves %d of %d scored players at GW%d "+
		"(count varies run to run; see banddeterminism_test.go)", moved, scored, cutoff)
}

// TestBandStrengthCannotActBeforeAnyFixtureIsPlayed is the confinement half of
// the arrival check, and it is the one with teeth.
//
// A confinement check on a path that cannot carry the effect confirms nothing, so
// it is deliberately paired with the liveness check above: that one MUST move,
// this one MUST NOT. `bandMinMatches` is 5 and the bands are built from finished
// fixtures only, so at a pre-season cutoff there is no rating and both adjustments
// return exactly 1 — which is the property that lets the opening fifteen of a GW1
// replay cell be byte-identical between arms, as it was in 6 of 6 GW1 cells on
// both arms of the banked sweep.
//
// **If this ever fails, the bands are reading matches that had not been played.**
// That is the recorded `loadFixtures` bug arriving from the opposite direction,
// and it is the one place a fixture-reading feature can silently train on the
// future — the archive holds every score for the whole season.
//
// `teamBands` is protected by its own `Finished` check, and that guard is
// unchanged by this fix — moving it would move band membership and therefore
// `Score`, which needs its own measurement (see the doc comment on teamBands).
//
// `buildTeamRates` used to have no such protection: it tested only that the
// scores were non-nil, under a comment asserting `playedFixtures` had already
// stripped them. `PreSeasonWith` returns `cur.Fixtures` unfiltered — `playedFixtures`
// is never called when `through <= 0` — and every archived season carries
// `finished: false` *and* `finished_provisional: false` on all 380 fixtures, all of
// them scored, so at cutoff 0 `buildTeamRates` held the whole season's results.
// That reached scoring only through `magnitudeAttack`/`magnitudeDefence` behind
// `FPL_MAGNITUDE`, so it was opt-in rather than shipped — but any `FPL_MAGNITUDE`
// figure including a GW1 entry cell was contaminated by it.
//
// Fixed by gating `buildTeamRates` on `FinishedProvisional`, the same flag
// `TeamMatchesFinished` and `blend.go` already use for "this match's own numbers
// are locked in" — see the doc comment on `fpl.Fixture`. `TestPointInTimeHidesFutureResults`
// sweeps `through` over {1, 5, 12, 20, 38} and never tests 0, which is why this hole
// was invisible to it; `TestPreSeasonTeamRatesCannotSeeTheSeasonsResults` below is
// the direct pin, on the pre-season path where the bug lived.
func TestBandStrengthCannotActBeforeAnyFixtureIsPlayed(t *testing.T) {
	cfg := loadConfig(t)
	cur, err := Load(context.Background(), cfg.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(context.Background(), cfg.CacheDir, "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	// Cutoff 0 is what a StartGW=1 cell builds its opening squad at.
	at := func(bs float64) []float64 {
		sc := sweepConfig(cfg, 1, false)
		sc.Weights.BandStrength = bs
		e, _ := EngineAt(cur, prior, 0, sc)
		var out []float64
		for _, m := range e.AllMetrics() {
			out = append(out, m.Score)
		}
		return out
	}

	off, on := at(0), at(1)
	if len(off) != len(on) {
		t.Fatalf("pool sizes differ: %d against %d", len(off), len(on))
	}
	moved := 0
	for i := range off {
		if off[i] != on[i] {
			moved++
		}
	}
	if moved != 0 {
		t.Fatalf("BandStrength moved %d of %d scores at a PRE-SEASON cutoff, where no "+
			"fixture has been played and teamBands must not be ready. The bands are "+
			"rating clubs on matches that had not happened — check that playedFixtures "+
			"is still stripping Finished and the scoreline past the cutoff.",
			moved, len(off))
	}
}

// TestPreSeasonTeamRatesCannotSeeTheSeasonsResults is the direct pin for the bug
// recorded above: at a pre-season cutoff, `buildTeamRates` must read only the
// prior, never the archived season's actual scorelines.
//
// `PreSeasonWith` — reached through `EngineAt(cur, prior, 0, cfg)`, exactly as a
// StartGW=1 replay cell builds its opening squad — returns the archive's fixtures
// unfiltered: all 380 of them carry a final scoreline and all 380 read
// `finished_provisional: false`, because the archive build never touches either
// flag (season.go). Before the fix, `buildTeamRates` counted a fixture the moment
// both scores were non-nil, so a GW1 engine accumulated the whole season it had
// not seen yet. This asserts every club reads `Played == 0` at that cutoff, and
// that its rate matches a fixture-less engine's — the pure prior.
func TestPreSeasonTeamRatesCannotSeeTheSeasonsResults(t *testing.T) {
	cfg := loadConfig(t)
	cur, err := Load(context.Background(), cfg.CacheDir, "2024-25")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}
	prior, err := Load(context.Background(), cfg.CacheDir, "2023-24")
	if err != nil {
		t.Skipf("archive unavailable: %v", err)
	}

	sc := sweepConfig(cfg, 1, false)
	e, boot := EngineAt(cur, prior, 0, sc)

	// A plain struct literal, not analysis.NewEngineFull — this engine never
	// scores a player (only TeamRatesFor, below, which reads Boot.Teams and
	// Fixtures alone), so it correctly carries no recency index and must stay
	// invisible to TestEveryScoringEngineGetsRecency rather than added to its
	// exemption list under a false justification.
	fixtureless := &analysis.Engine{
		Boot:    &fpl.Bootstrap{Season: boot.Season, Teams: boot.Teams},
		Weights: sc.Weights,
	}

	if len(boot.Teams) == 0 {
		t.Fatal("no teams on the pre-season bootstrap — cannot check anything")
	}
	for _, team := range boot.Teams {
		got := e.TeamRatesFor(team.ID)
		if got.Played != 0 {
			t.Errorf("%s: Played = %d at a pre-season cutoff, want 0 — buildTeamRates "+
				"is reading finished-looking scorelines from an archive that has not "+
				"revealed this season yet", team.ShortName, got.Played)
		}
		want := fixtureless.TeamRatesFor(team.ID)
		if got != want {
			t.Errorf("%s: rate %+v at pre-season differs from the pure prior %+v — "+
				"the archive's unrevealed scorelines are still moving it", team.ShortName, got, want)
		}
	}
}

// TestBandStrengthIsShippedOff pins the baseline the measurement is against.
//
// The banked sweep's baseline arm is "the shipped setting" and is written as the
// zero it happens to be rather than read from config. If the shipped value ever
// moves, that sweep would compare the new shipped value against itself on one arm
// and against 1.0 on the other, and the label on the baseline row would be wrong
// rather than the arithmetic. It is also the premise of
// TestBandStrengthIsDeterministicAtTheShippedSetting, which argues from the bands
// never being built.
func TestBandStrengthIsShippedOff(t *testing.T) {
	cfg := loadConfig(t)
	if cfg.Weights.BandStrength != 0 {
		t.Fatalf("band_strength ships at %v, not 0. TestDiagBandStrength's baseline "+
			"arm is labelled as the shipped setting and hard-codes 0; update both "+
			"together or the snapshot records a comparison nobody made.",
			cfg.Weights.BandStrength)
	}
}
