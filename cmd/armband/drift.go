package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"armband/internal/analysis"
	"armband/internal/backtest"
	"armband/internal/config"
	"armband/internal/fpl"
)

// cmdDrift reports how far the squad you actually fielded has fallen behind the
// squad you could have built, gameweek by gameweek.
//
// # What the number is
//
// For each gameweek: take the fifteen you owned, take the best fifteen the
// optimiser can build for the same money, and subtract what their best legal
// elevens score. It is the gap in projected points a week, not realised points —
// a statement about the squad you were holding, not about whether it happened to
// haul.
//
// # Why it is computed at gameweek MINUS ONE, and this is the whole honesty of it
//
// `EngineAt(cur, prior, gw-1, ...)` builds the model's view from data through
// `gw-1`, which is what was knowable when the squad for `gw` was picked. That is
// the same seam the replay uses, and it is exported rather than reimplemented
// here for the usual reason.
//
// ⚠️ **Reading the drift for a past gameweek with TODAY's engine would be a leak,
// and a self-flattering one.** Today's ratings know how the season went, so the
// "best squad you could have built" would be built with hindsight and would beat
// your real squad by more the further back you looked — manufacturing a rising
// trend out of nothing, in exactly the shape a rising trend is expected. This
// command therefore rebuilds the engine per gameweek instead of scoring old
// squads with a current one.
//
// # Two things it cannot do
//
//   - **It stops where the archive stops.** The per-gameweek season data is
//     published during the season and lags live by some amount, so the most
//     recent gameweek or two will usually be missing. That is a short series, not
//     a wrong one.
//   - **It reads the squad you FIELDED, from FPL, and the budget you had at the
//     time** (`entry_history.value + entry_history.bank`). It does not know what
//     you could have afforded after selling at each player's own selling price,
//     so the comparison is against a squad built at market prices. That flatters
//     the fresh squad slightly and is stated rather than corrected.
func cmdDrift(ctx context.Context, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	season := fs.String("season", "", "season to read, e.g. 2026-27 (default: the "+
		"season the archive holds most recently)")
	from := fs.Int("from", 1, "first gameweek to report")
	through := fs.Int("through", 38, "last gameweek to report")
	out := fs.String("out", "", "also write the series as CSV to this path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cfg.EntryID == 0 {
		return fmt.Errorf("drift needs your FPL manager id: set entry_id in the config " +
			"file. Without it there is no squad to measure, only an optimum")
	}
	if *season == "" {
		return fmt.Errorf("drift needs -season, e.g. `armband drift -season 2026-27`")
	}

	// ⚠️ **THE SEASON MUST BE THE ONE BEING PLAYED, and this is the only place
	// that can tell.** FPL's picks endpoint serves the CURRENT season only —
	// there is no way to ask it for an earlier one — while `-season` chooses
	// which archive the engine is built from. **Element ids are reassigned every
	// summer**, so a mismatch does not fail: the ids still resolve, because
	// they are small integers that exist in both seasons. They simply name
	// DIFFERENT PLAYERS, who mostly have no data, and the command reports a held
	// eleven scoring 0.00 against a fresh eleven scoring 47.98 — a drift made
	// entirely of absences, wearing the shape of a catastrophically decayed
	// squad.
	//
	// That is what the first version of this command printed, and no
	// id-resolution check can catch it, which is why the test is the season
	// label rather than the squad.
	if live := liveArchiveSeason(time.Now()); *season != live {
		return fmt.Errorf("-season is %s but the season being played is %s. FPL serves "+
			"picks for the current season ONLY, and element ids are reassigned every "+
			"summer, so this pairing would score your squad against other people's "+
			"ids and report their absence as your decline", *season, live)
	}

	cur, err := backtest.Load(ctx, cfg.CacheDir, *season)
	if err != nil {
		return fmt.Errorf("loading %s: %w", *season, err)
	}
	// The prior season feeds the multi-season priors. A missing one is not fatal:
	// the engine falls back to league rates, which is what a first archived
	// season gets anyway.
	prior, err := backtest.Load(ctx, cfg.CacheDir, priorArchiveSeason(*season))
	if err != nil {
		prior = nil
	}

	sc := backtest.SimConfig{Weights: cfg.Weights, StartGW: 1}
	client := fpl.New(cfg.CacheDir, 24*time.Hour, 24*time.Hour)

	var rows []driftRow
	var skipped, freeHit []int

	for gw := *from; gw <= *through; gw++ {
		picks, err := client.Picks(ctx, cfg.EntryID, gw)
		if err != nil || len(picks.Picks) == 0 {
			skipped = append(skipped, gw)
			continue
		}
		// ⚠️ **A free hit's fifteen is not the squad you were holding**, and
		// reporting it here would flatter the series exactly where it matters.
		// FPL returns the one-week free-hit team for that gameweek; the
		// persistent squad — the one whose decay this command exists to show —
		// is handed straight back afterwards and never appears. A free-hit team
		// is built for that week's fixtures, so it reads as LOW drift while the
		// real squad's gap goes unmeasured.
		//
		// Skipped rather than flagged: a number that does not mean what its
		// column says is worse than an absence, because only one of the two gets
		// noticed.
		if picks.ActiveChip != nil && *picks.ActiveChip == "freehit" {
			freeHit = append(freeHit, gw)
			continue
		}

		held := make([]int, 0, len(picks.Picks))
		for _, p := range picks.Picks {
			held = append(held, p.Element)
		}

		budget := picks.EntryHistory.Value + picks.EntryHistory.Bank
		if budget <= 0 {
			skipped = append(skipped, gw)
			continue
		}

		e, _ := backtest.EngineAt(cur, prior, gw-1, sc)

		// ⚠️ **THE SQUAD AND THE ENGINE MUST DESCRIBE THE SAME SEASON, and nothing
		// else checks it.** FPL's picks endpoint serves only the season being
		// played — there is no way to ask it for 2024-25 — while `-season` chooses
		// which archive the engine is built from. **Element ids are reassigned
		// every summer**, so pairing this season's picks with another season's
		// archive resolves almost nobody and reports a drift made entirely of
		// absences. That looks exactly like a catastrophically decayed squad,
		// which is the most misleading shape this number could take, and it is
		// what the first version of this command printed: a held eleven scoring
		// 0.00 against a fresh eleven scoring 47.98.
		//
		// Refusing beats reporting. A squad whose players cannot be found is not
		// a squad with a large drift; it is a reading that did not happen.
		resolved := 0
		for _, id := range held {
			if e.Boot.ElementByID(id) != nil {
				resolved++
			}
		}
		// A squad that cannot field eleven is not a squad with a big drift, so
		// eleven — not fifteen — is the bar. A player genuinely removed from the
		// game mid-season is a real and rare thing, and it should not fail a run.
		if resolved < driftMinResolved {
			return fmt.Errorf("GW%d: only %d of your %d players exist in the %s archive. "+
				"FPL serves picks for the CURRENT season only, so -season must name the "+
				"season being played — element ids are reassigned every summer and "+
				"pairing two seasons reports a drift made of absences rather than decay",
				gw, resolved, len(held), *season)
		}

		// ⚠️ **The floors are not optional and their zero value is not a default.**
		// Omitting them builds the comparison squad with NO minutes floor, so it
		// can be filled with rotation risks and injured returnees that
		// `armband squad`, `armband transfers` and the agent's own optimiser
		// would all refuse to offer. Drift would then be measured against a
		// laxer optimiser than any surface the user can act through, and would
		// read systematically too high. The first version of this command did
		// exactly that.
		sq, err := e.Optimize(analysis.OptimizeRequest{
			Budget:             budget,
			MinMinutes:         analysis.PoolMinMinutes,
			MinExpectedMinutes: analysis.PoolMinExpectedMinutes,
		})
		if err != nil || len(sq.Players) == 0 {
			skipped = append(skipped, gw)
			continue
		}
		fresh := make([]int, 0, len(sq.Players))
		for _, p := range sq.Players {
			fresh = append(fresh, p.ID)
		}

		h, f := analysis.XIPoints(e, held), analysis.XIPoints(e, fresh)
		rows = append(rows, driftRow{
			gw: gw, held: h, fresh: f, drift: f - h,
			changes:      countChanged(held, fresh),
			budgetTenths: budget,
		})
	}

	if len(rows) == 0 {
		return fmt.Errorf("no gameweek in %d-%d could be read: the archive may not carry "+
			"%s yet, or entry %d has no picks there", *from, *through, *season, cfg.EntryID)
	}

	fmt.Printf("\nSQUAD DRIFT, entry %d, %s\n", cfg.EntryID, *season)
	// ⚠️ "the eleven you fielded" would overstate this. What is scored is the
	// model's own best legal eleven from the fifteen you OWNED — not your actual
	// starting eleven, not your captain, and not autosubs. The distinction is
	// small in points and total in meaning: this is a claim about the squad, not
	// about how you set it up.
	fmt.Printf("How far the best eleven from the fifteen you OWNED fell behind the best\n")
	fmt.Printf("eleven buildable for the same money, in projected points a gameweek.\n")
	fmt.Printf("⚠️ Each row is scored on what was knowable BEFORE that gameweek, not on\n")
	fmt.Printf("what happened in it — so this is not hindsight and not realised points.\n")
	fmt.Printf("⚠️ Scored at horizon %d, so it is a multi-week average and cannot see one\n",
		cfg.Weights.Horizon)
	fmt.Printf("week's double or blank. Fixture load reaches Score at horizon 1 only.\n\n")
	fmt.Printf("  %3s %9s %9s %8s %9s %9s\n", "gw", "yours", "best", "drift", "changes", "budget")
	for _, r := range rows {
		fmt.Printf("  %3d %9.2f %9.2f %8.2f %9d %9.1f\n",
			r.gw, r.held, r.fresh, r.drift, r.changes, float64(r.budgetTenths)/10)
	}
	if len(skipped) > 0 {
		fmt.Printf("\n  not read: %v — no picks, or the archive does not reach that week yet\n",
			skipped)
	}
	if len(freeHit) > 0 {
		fmt.Printf("  free hit: %v — skipped. FPL returns the one-week team for those, not\n",
			freeHit)
		fmt.Printf("            the squad you keep, so the gap there is not this series'.\n")
	}
	fmt.Printf("\n⚠️ `changes` counts how many of your fifteen the fresh squad replaced. It\n")
	fmt.Printf("is a move count, not a price: swapping a bench player nobody would spend a\n")
	fmt.Printf("transfer on counts the same as replacing your captain. Read `drift`.\n")

	if *out != "" {
		if err := writeDriftCSV(*out, cfg.EntryID, *season, rows); err != nil {
			return err
		}
		fmt.Printf("\nwrote %s\n", *out)
	}
	return nil
}

// liveArchiveSeason is the season being played at time t, in the archive's own
// two-digit form.
//
// The boundary is July, matching `priorSeasonName`'s: January to June still
// belongs to the season that began the previous calendar year. The two are kept
// consistent deliberately — a project with two different opinions about when a
// season turns over would place the same date in different seasons depending on
// which function was asked.
func liveArchiveSeason(t time.Time) string {
	y := t.Year()
	if t.Month() < time.July {
		y--
	}
	return fmt.Sprintf("%04d-%02d", y, (y+1)%100)
}

// priorArchiveSeason turns "2026-27" into "2025-26".
//
// ⚠️ **Deliberately NOT `priorSeasonName`, which already exists in this package
// and answers a different question.** That one derives the prior season from the
// live engine's next deadline and formats it for `internal/priors`; this one
// maps one ARCHIVE season label to the one before it, in the archive's own
// two-digit form, with no engine and no clock involved. Same words, different
// input, different store, different format — so they are named apart rather than
// unified.
//
// Returns the input unchanged when it is not in that shape, so a caller gets a
// failed load naming something it can recognise rather than a silent empty prior.
func priorArchiveSeason(s string) string {
	if len(s) != 7 || s[4] != '-' {
		return s
	}
	start, err := strconv.Atoi(s[:4])
	if err != nil {
		return s
	}
	return fmt.Sprintf("%04d-%02d", start-1, (start)%100)
}

// countChanged is how many of `held` the fresh squad does not contain.
func countChanged(held, fresh []int) int {
	in := make(map[int]bool, len(fresh))
	for _, id := range fresh {
		in[id] = true
	}
	n := 0
	for _, id := range held {
		if !in[id] {
			n++
		}
	}
	return n
}

// driftMinResolved is how many of a held fifteen must exist in the archive
// before a gameweek is reported at all.
//
// Eleven, because that is what a legal starting eleven needs: below it the
// comparison cannot be made and the honest answer is an error, not a number.
// Deliberately not fifteen — a player removed from the game mid-season is real
// and rare, and should not fail an otherwise sound run.
const driftMinResolved = 11

// driftRow is one gameweek's reading.
type driftRow struct {
	gw           int
	held, fresh  float64
	drift        float64
	changes      int
	budgetTenths int
}

func writeDriftCSV(path string, entry int, season string, rows []driftRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"entry", "season", "gw", "held_xi", "fresh_xi",
		"drift", "changes", "budget_tenths"}); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{
			strconv.Itoa(entry), season, strconv.Itoa(r.gw),
			strconv.FormatFloat(r.held, 'f', 4, 64),
			strconv.FormatFloat(r.fresh, 'f', 4, 64),
			strconv.FormatFloat(r.drift, 'f', 4, 64),
			strconv.Itoa(r.changes),
			strconv.Itoa(r.budgetTenths),
		}); err != nil {
			return err
		}
	}
	return nil
}
