package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"armband/internal/analysis"
	"armband/internal/capture"
	"armband/internal/config"
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
func cmdAsOf(cfg config.Config, args []string) error {
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

	boot, fixtures, err := capture.Replay(dir)
	if err != nil {
		return fmt.Errorf("replaying the capture: %w", err)
	}

	// ⚠️ Refuse hindsight. The manifest names the gameweek this capture is
	// evidence for; if the payload already shows it played, the capture is from
	// after the deadline and the answer would be contaminated.
	event, err := captureEvent(dir)
	if err != nil {
		return err
	}
	if event > 0 {
		for _, e := range boot.Events {
			if e.ID == event && e.Finished {
				return fmt.Errorf("capture %s is evidence for GW%d, but its own payload "+
					"already shows GW%d finished — it is from after the deadline and "+
					"running the optimiser on it would be hindsight, not a forecast",
					filepath.Base(dir), event, event)
			}
		}
	}

	engine := analysis.NewEngineFull(boot, fixtures, cfg.Weights, cfg.Congestion, cfg.RoleRisk)
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

// captureEvent reads the gameweek a capture is evidence for from its manifest.
// A capture without one is not refused — the live series has carried manifests
// without an event since before the field existed — but it cannot be guarded.
func captureEvent(dir string) (int, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return 0, fmt.Errorf("reading the capture manifest: %w", err)
	}
	var m struct {
		Event int `json:"event"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, fmt.Errorf("parsing the capture manifest: %w", err)
	}
	return m.Event, nil
}
