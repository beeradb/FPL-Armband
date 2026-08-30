package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"armband/internal/analysis"
	"armband/internal/capture"
	"armband/internal/config"
	"armband/internal/priors"
)

// cmdAsOf runs the optimiser against a CAPTURED moment instead of the live API.
//
// # What this is for
//
// Comparing our squad against managers who rebuilt at a given deadline is only
// honest if the model sees what they saw. Run against live data it would price
// players on results published after the deadline — point-in-time leakage, which
// this repository has a bug class for. A capture is the fix: immutable bytes from
// before the deadline, replayed.
//
// # Why it is dispatched early, with capture, backfill and drift
//
// For drift's reason exactly: it builds its own engine from bytes on disk and
// never talks to FPL, so fetching a live bootstrap first would be a network round
// trip and a scoring pass in service of a command that reads neither. It would
// also make an as-of run fail on a live outage, which is precisely the
// dependency the capture exists to remove.
//
// # ⚠️ The guard is the point
//
// A capture whose target gameweek has already finished is not evidence about the
// decision a manager faced, it is hindsight. This refuses to run on one rather
// than printing a squad that would look good for the wrong reason. The manifest's
// `event` names the gameweek; the payload's own events[] says whether it has been
// played.
func cmdAsOf(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("asof", flag.ContinueOnError)
	budget := fs.Int("budget", 1000, "squad budget in tenths of a million (1000 = £100.0m)")
	asJSON := fs.Bool("json", false, "emit the squad as JSON rather than a table")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := fs.Arg(0)
	if dir == "" {
		return fmt.Errorf("asof needs a capture directory, e.g. " +
			"`armband asof data/captures/2026-08-28T1100Z`")
	}

	// ⚠️ Refuse hindsight, using the repository's own verifier rather than a
	// second copy of it.
	//
	// The first version of this checked only whether the target gameweek was
	// `finished`, and that misses the window that actually leaks: AFTER the
	// deadline and BEFORE the gameweek ends, where `finished` is still false but
	// the payload carries post-deadline team news, price moves and partial
	// results. VerifyPreDeadline's own comment says why its three tests are not
	// redundant — `is_next` catches aiming at the wrong gameweek, the deadline
	// comparison catches being late for the right one, and `finished` catches a
	// body from long after the fact. Only the third was implemented here.
	event, capturedAt, err := captureMoment(dir)
	if err != nil {
		return err
	}
	if event > 0 {
		raw, err := capture.Read(dir, capture.BootstrapEndpoint)
		if err != nil {
			return fmt.Errorf("re-reading the captured bootstrap to verify it: %w", err)
		}
		q, err := capture.VerifyPreDeadline(raw, event, capturedAt)
		if err != nil {
			return fmt.Errorf("capture %s is not usable as point-in-time evidence for "+
				"GW%d: %w", filepath.Base(dir), event, err)
		}
		fmt.Fprintf(os.Stderr, "verified pre-deadline: %.1fh before the GW%d deadline\n",
			q.HoursBefore, event)
	}

	boot, fixtures, err := capture.Replay(dir)
	if err != nil {
		return fmt.Errorf("replaying the capture: %w", err)
	}

	engine := analysis.NewEngineFull(boot, fixtures, cfg.Weights, cfg.Congestion, cfg.RoleRisk)

	// ⚠️ THE PRIOR IS NOT OPTIONAL, and omitting it is not a small difference.
	//
	// blend.go returns before both the prior mix and shrinkToLeague when Priors is
	// nil, so an engine without it scores raw current-season per-90 rates with no
	// shrinkage at all. At one match played that is the GW1 scoreboard, and
	// shrinkToLeague's own comment records the incident: a promoted-club debutant
	// who played ninety minutes "concentrated the wildcard/free-hit builder on
	// exactly these players — up to 5 of 15 squad slots".
	//
	// The first version of this command omitted it and produced exactly that: a
	// 5-4-1 of cheap GW1 point-scorers, £6.2m unspent, a £4.6m defender captained
	// at a nonsense 14.57 expected points per gameweek.
	//
	// Last season's priors are pre-deadline by construction — the season ended
	// before this one began — so loading them introduces no leakage. The gate is
	// the live path's: only once the season has started, because before then FPL's
	// own aggregates already ARE last season's totals.
	// ⚠️ The live path has TWO prior sources and this has one. When
	// prior_half_life is set it first tries recent.LoadPriors, a multi-season
	// blend that needs a live client — which an as-of run does not have and must
	// not acquire. It ships at 0, so with the deployed config the two paths are
	// exactly equivalent; if anyone turns it on, this would silently score a
	// different model than the live engine while still claiming parity. Refuse
	// rather than diverge quietly.
	if cfg.Weights.PriorHalfLife > 0 {
		return fmt.Errorf("prior_half_life is %v, so the live path would blend "+
			"multiple prior seasons through a live client that an as-of run does "+
			"not have; this command would score a different model than the engine "+
			"it is being compared against", cfg.Weights.PriorHalfLife)
	}

	// ⚠️ The recency index is the live path's SECOND unbuildable input, and it is
	// refused on the same principle as prior_half_life above rather than a
	// different one.
	//
	// cmd/armband wires engine.Recent from recent.Load — per-player match history,
	// one element-summary request per player who has played. A capture holds the
	// bootstrap and the fixtures and nothing else, so an as-of run cannot build
	// that index, and blend.go's recency branch is then never reached:
	// MinutesPerMatch stays at the flat season-to-date mean and blankRunFactor
	// never fires.
	//
	// Where the cutoff comes from, measured rather than assumed
	// (TestRecencyIsANoOpAtOneGameweekPlayed, internal/backtest):
	//
	//   0 or 1 gameweeks played — the index is a BIT-EXACT no-op. Across all six
	//     archived seasons, 0 of 500-690 players differ on ExpectedMinutes. At one
	//     gameweek the recency-weighted mean over a single gameweek and the flat
	//     season-to-date mean are arithmetically the same number.
	//   2 gameweeks played — 194-246 players move, up to 3 of the starting XI
	//     changes and expected points shift by up to 0.20.
	//
	// So this runs where the two paths are provably one model and refuses where
	// they are not. The live path's own gate is GameweeksPlayed() > 0; ours is > 1,
	// and the extra gameweek is bought by the measurement above, not by assumption.
	if engine.GameweeksPlayed() > 1 {
		return fmt.Errorf("this capture has %d gameweeks played, and from two "+
			"onward the live path's recency index moves the squad (194-246 players, "+
			"up to 3 of the XI); a capture carries no per-player match history to "+
			"build it from, so an as-of run here would score a different model than "+
			"the engine it is being compared against", engine.GameweeksPlayed())
	}

	if engine.SeasonHasStarted() {
		s, err := priors.Load(ctx, cfg.CacheDir, priorSeasonName(engine))
		if err != nil {
			return fmt.Errorf("loading the prior season: %w — run `armband priors`; "+
				"an as-of run without it scores the unshrunk one-match model", err)
		}
		engine.Priors = priors.Adapter{S: s}
	}

	squad, err := engine.Optimize(analysis.OptimizeRequest{Budget: *budget})
	if err != nil {
		return fmt.Errorf("optimising: %w", err)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(squad)
	}

	fmt.Printf("as of %s (GW%d, budget £%.1fm)\n\n", filepath.Base(dir), event, float64(*budget)/10)
	fmt.Printf("  %-18s %-5s %-6s %6s  %s\n", "PLAYER", "CLUB", "POS", "PRICE", "xP")
	for _, p := range squad.StartingXI {
		fmt.Printf("  %-18s %-5s %-6s %6.1f  %5.2f\n", p.Name, p.Team, p.Position, p.Price, p.Score)
	}
	fmt.Printf("  %s\n", "-- bench --")
	for _, p := range squad.Bench {
		fmt.Printf("  %-18s %-5s %-6s %6.1f  %5.2f\n", p.Name, p.Team, p.Position, p.Price, p.Score)
	}
	fmt.Printf("\n  formation %s   captain %s   cost £%.1fm   xP %.2f\n",
		squad.Formation, squad.Captain.Name, squad.TotalCost, squad.ExpectedPoints)
	return nil
}

// captureMoment reads the gameweek a capture is evidence for and when it was
// taken. A capture without an event is not refused — the live series carried
// manifests without the field before it existed — but it cannot be guarded, and
// the caller skips verification rather than inventing a target.
func captureMoment(dir string) (int, time.Time, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("reading the capture manifest: %w", err)
	}
	var m struct {
		Event      int       `json:"event"`
		CapturedAt time.Time `json:"captured_at"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, time.Time{}, fmt.Errorf("parsing the capture manifest: %w", err)
	}
	return m.Event, m.CapturedAt, nil
}
