package main

import (
	"context"
	"fmt"
	"os"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
)

// cmdTransfers runs the deterministic transfer search over the squad you
// actually own and prints the best plans. No LLM, so it costs nothing.
//
// It is the same search `suggest_transfers` gives the agent — analysis.BuildPlans
// over analysis.NewSquadState — deliberately, and not a second implementation of
// the same decision. This project's standing rule is that anything proposing
// transfers goes through internal/analysis/swaps.go, because the two mistakes it
// exists to prevent (ranking on a player's own score rather than what the swap
// does to the ELEVEN, and only ever moving one player at a time) were each made
// twice independently before it existed.
//
// What this adds over the tool is a human view: what goes out, what comes in,
// the gain, and the player the plan hinges on.
func cmdTransfers(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine, plain bool, htmlPath string) error {

	if cfg.EntryID == 0 {
		return fmt.Errorf("no entry_id in config.json, so there is no squad to improve.\n" +
			"Set one, or use `armband squad` to build a fifteen from scratch")
	}
	entry, err := client.Entry(ctx, cfg.EntryID)
	if err != nil {
		return fmt.Errorf("reading entry %d: %w", cfg.EntryID, err)
	}
	// FPL only exposes picks once a deadline has passed. Before GW1 that is
	// expected rather than an error, and saying so is the difference between a
	// user thinking the tool is broken and knowing the season has not started.
	if entry.CurrentEvent == nil {
		return fmt.Errorf("FPL has no squad for entry %d yet — picks only become "+
			"visible after a deadline has passed. Before GW1 use `armband squad`",
			cfg.EntryID)
	}
	picks, err := client.Picks(ctx, cfg.EntryID, *entry.CurrentEvent)
	if err != nil {
		return fmt.Errorf("reading picks: %w", err)
	}

	var squad []analysis.PlayerMetrics
	for _, p := range picks.Picks {
		el := e.Boot.ElementByID(p.Element)
		if el == nil {
			return fmt.Errorf("FPL returned player id %d, which is not in the "+
				"bootstrap; the cached data may be from a previous season", p.Element)
		}
		squad = append(squad, e.Metrics(el))
	}
	if len(squad) != 15 {
		return fmt.Errorf("expected 15 picks, got %d", len(squad))
	}

	state := analysis.NewSquadState(squad)
	// FPL pays what you paid plus half of any rise, never the market price. Nil
	// means sell-at-market, which overstates the budget.
	state.Sell = e.SellPrices

	// Standing overrides bind here for the reason recorded in the research
	// record: excluding a player from squad builds while the transfer search
	// still offers to buy him is worse than not excluding him at all.
	req := analysis.OptimizeRequest{}
	notes := applyRoster(cfg, e, &req)
	excluded := map[int]bool{}
	for _, id := range req.ExcludeIDs {
		excluded[id] = true
	}

	var pool []analysis.PlayerMetrics
	for _, c := range e.AllMetrics() {
		if excluded[c.ID] || c.Minutes < 600 {
			continue
		}
		if c.Status != "available" && c.Status != "doubtful" {
			continue
		}
		pool = append(pool, c)
	}

	bank := picks.EntryHistory.Bank
	plans := analysis.BuildPlans(state, pool, e.WeekEngine(), bank, 3, 5)

	theme := present.TerminalTheme()
	if plain {
		theme = present.PlainTheme()
	}

	for _, n := range notes {
		fmt.Printf("\n%s\n", dim(n))
	}
	fmt.Printf("\n%s\n", dim(fmt.Sprintf(
		"entry %d after GW%d · bank £%.1fm · squad value £%.1fm · %s",
		cfg.EntryID, *entry.CurrentEvent, float64(bank)/10,
		float64(picks.EntryHistory.Value)/10, e.Budget.Label())))

	if len(plans) == 0 {
		present.Moves(os.Stdout, analysis.Plan{}, theme)
		return nil
	}

	// The best plan in full, then the runners-up as one-liners. Printing five
	// complete team sheets buries the decision.
	best := plans[0]
	present.Moves(os.Stdout, best, theme)
	present.Squad(os.Stdout, planSquad(best), theme,
		fmt.Sprintf("The eleven this would field in GW%d", *entry.CurrentEvent+1))

	if len(plans) > 1 {
		fmt.Printf("\n  %s\n", dim("also considered"))
		for _, p := range plans[1:] {
			fmt.Printf("    %s  %s\n",
				dim(fmt.Sprintf("%+.2f pts/gw", p.GainPerGW)), movesLine(p))
		}
		fmt.Println()
	}

	if htmlPath != "" {
		f, err := os.Create(htmlPath)
		if err != nil {
			return fmt.Errorf("write transfers HTML: %w", err)
		}
		defer f.Close()
		sq := planSquad(best)
		if err := present.HTML(f, sq, &best,
			fmt.Sprintf("Transfers for GW%d", *entry.CurrentEvent+1),
			fmt.Sprintf("entry %d · bank £%.1fm", cfg.EntryID, float64(bank)/10)); err != nil {
			return fmt.Errorf("render transfers HTML: %w", err)
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Printf("  %s\n\n", dim("wrote "+htmlPath))
	}
	return nil
}

// bestPlanForOwnedSquad returns the best transfer plan for the squad you own,
// or a plain-language reason there is none.
//
// The reason is returned rather than swallowed on purpose. "No transfers shown"
// and "you have no squad yet" and "the model wants no move this week" look
// identical as an absent section, and only the last of those is a
// recommendation — this project's review policy says doing nothing is a
// first-class outcome and usually the right one, which is worth saying out loud
// rather than leaving as a gap in a page.
func bestPlanForOwnedSquad(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine) (*analysis.Plan, string) {

	if cfg.EntryID == 0 {
		return nil, "No entry_id is configured, so there is no squad to improve — " +
			"this page is a fifteen built from scratch."
	}
	entry, err := client.Entry(ctx, cfg.EntryID)
	if err != nil {
		return nil, "Could not read your entry, so no transfers were computed: " + err.Error()
	}
	if entry.CurrentEvent == nil {
		return nil, "FPL exposes picks only after a deadline has passed, so there is no " +
			"squad to improve yet. Before GW1 this page is the opening fifteen."
	}
	picks, err := client.Picks(ctx, cfg.EntryID, *entry.CurrentEvent)
	if err != nil {
		return nil, "Could not read your picks, so no transfers were computed: " + err.Error()
	}

	var squad []analysis.PlayerMetrics
	for _, p := range picks.Picks {
		if el := e.Boot.ElementByID(p.Element); el != nil {
			squad = append(squad, e.Metrics(el))
		}
	}
	if len(squad) != 15 {
		return nil, fmt.Sprintf("Read %d of 15 picks, so no transfers were computed.", len(squad))
	}

	state := analysis.NewSquadState(squad)
	state.Sell = e.SellPrices

	req := analysis.OptimizeRequest{}
	applyRoster(cfg, e, &req)
	excluded := map[int]bool{}
	for _, id := range req.ExcludeIDs {
		excluded[id] = true
	}
	var pool []analysis.PlayerMetrics
	for _, c := range e.AllMetrics() {
		if excluded[c.ID] || c.Minutes < 600 {
			continue
		}
		if c.Status != "available" && c.Status != "doubtful" {
			continue
		}
		pool = append(pool, c)
	}

	plans := analysis.BuildPlans(state, pool, e.WeekEngine(), picks.EntryHistory.Bank, 3, 5)
	if len(plans) == 0 {
		return nil, "No move clears the threshold this week. Banking the transfer is a " +
			"first-class outcome and usually the right one."
	}
	return &plans[0], ""
}

// planSquad adapts a Plan's imminent-week eleven into the shape the pitch
// renderer draws.
//
// The XI and bench here are the ones that would actually be FIELDED next
// gameweek, not the horizon-averaged eleven the decision was made on. Those are
// two different elevens on purpose — presenting a recommendation next to a team
// nobody would field is simply wrong — so this is the right one to draw and the
// wrong one to re-derive a gain from.
func planSquad(p analysis.Plan) analysis.Squad {
	sq := analysis.Squad{
		Players:    p.Squad,
		StartingXI: p.XI,
		Bench:      p.Bench,
		Formation:  p.Formation,
		Captain:    p.Captain,
	}
	for _, x := range p.XI {
		if x.ID != p.Captain.ID && x.Score > sq.ViceCaptain.Score {
			sq.ViceCaptain = x
		}
		sq.XIScore += x.Score
		sq.TotalCost += x.Price
	}
	for _, b := range p.Bench {
		sq.TotalCost += b.Price
	}
	sq.ExpectedPoints = sq.XIScore + p.Captain.Score
	return sq
}

func movesLine(p analysis.Plan) string {
	s := ""
	for i, m := range p.Moves {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%s → %s", m.Out.Name, m.In.Name)
	}
	return s
}
