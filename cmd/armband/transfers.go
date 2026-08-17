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
	// FPL only exposes picks once a deadline has passed. Before GW1 that is
	// expected rather than an error, and saying so is the difference between a
	// user thinking the tool is broken and knowing the season has not started.
	board, why := buildTransferBoard(ctx, cfg, client, e)
	if board == nil {
		return fmt.Errorf("%s", why)
	}
	plans, bank := board.Plans, board.Bank

	theme := present.TerminalTheme()
	if plain {
		theme = present.PlainTheme()
	}

	for _, n := range board.Notes {
		fmt.Printf("\n%s\n", dim(n))
	}
	fmt.Printf("\n%s\n", dim(fmt.Sprintf(
		"entry %d after GW%d · bank £%.1fm · squad value £%.1fm · %s",
		cfg.EntryID, board.GW-1, float64(bank)/10,
		float64(board.Value)/10, e.Budget.Label())))

	// The banking decision, before the moves rather than after them. A policy
	// that declines a transfer and says nothing is indistinguishable from one
	// that found nothing worth doing, and those are opposite recommendations:
	// one says "act next week with two transfers", the other says "your squad is
	// fine". Printed only when the rule was actually consulted, so a user who has
	// not switched it on is not told about a decision nobody made.
	if board.Consulted {
		fmt.Printf("\n  %s\n", bankLine(board.Advice))
	}

	if len(plans) == 0 {
		present.Moves(os.Stdout, analysis.Plan{}, theme)
		return nil
	}

	// The best plan in full, then the runners-up as one-liners. Printing five
	// complete team sheets buries the decision.
	best := plans[0]
	present.Moves(os.Stdout, best, theme)
	present.Squad(os.Stdout, planSquad(best), theme,
		fmt.Sprintf("The eleven this would field in GW%d", board.GW))

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
			fmt.Sprintf("Transfers for GW%d", board.GW),
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
// A banking decision outranks the plan and is returned as the reason: when the
// rule says wait, the best plan is one the policy has decided not to make, and
// printing it as a recommendation is the opposite of what was decided.
func bestPlanForOwnedSquad(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine) (*analysis.Plan, string) {

	board, why := buildTransferBoard(ctx, cfg, client, e)
	if board == nil {
		return nil, why
	}
	if board.Consulted && board.Advice.Bank {
		return nil, board.Advice.Explain()
	}
	if len(board.Plans) == 0 {
		return nil, "No move clears the threshold this week. Banking the transfer is a " +
			"first-class outcome and usually the right one."
	}
	return &board.Plans[0], ""
}

// bankLine renders the banking decision for a terminal.
//
// The decision is stated first and the arithmetic behind it second, because the
// number a reader needs is "act or wait" and the two package values are what
// make that checkable rather than assertable.
func bankLine(a analysis.BankAdvice) string {
	s := a.Explain()
	if a.Guard != analysis.BankGuardNone {
		return s
	}
	return s + dim(fmt.Sprintf("\n    now %.2f · waiting %.2f · %d free of %d",
		a.NowValue, a.LaterValue, a.Free, a.BankUpTo))
}

// transferBoard is everything the weekly transfer decision is made from, for the
// squad you actually own.
//
// It exists so `armband transfers` and the page's brief cannot answer the same
// question two ways. Both used to assemble the state, the pool and the plans
// inline, in near-identical blocks; adding the banking decision to only one of
// them would have produced a page that recommends acting and a command that
// recommends waiting, off the same config and the same squad.
type transferBoard struct {
	Plans []analysis.Plan
	Bank  int
	// GW is the gameweek being decided FOR — the one after the last that
	// finished, which is the week a transfer made now would first play in.
	GW int
	// Free is the reconstructed free-transfer allowance, or
	// fpl.UnlimitedTransfers before the first deadline.
	Free int
	// Advice is the banking decision. Zero-valued and Consulted false when
	// banking is switched off, which is not the same as a decision to act.
	Advice    analysis.BankAdvice
	Consulted bool
	// Value is the squad's value in tenths, and Notes are the standing-override
	// lines the command prints above its header.
	Value int
	Notes []string
}

// buildTransferBoard assembles the weekly decision for the squad you own, or a
// plain-language reason there is none.
//
// The reason is returned rather than swallowed on purpose. "No transfers shown"
// and "you have no squad yet" and "the model wants no move this week" look
// identical as an absent section, and only the last of those is a
// recommendation — this project's review policy says doing nothing is a
// first-class outcome and usually the right one, which is worth saying out loud
// rather than leaving as a gap in a page.
func buildTransferBoard(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine) (*transferBoard, string) {

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

	// The gameweek a transfer made now would first play in.
	gw := *entry.CurrentEvent + 1
	bank := picks.EntryHistory.Bank
	// The allowance is reconstructed from the transfer history, which the picks
	// payload does not carry — FPL publishes it nowhere, so this is a close
	// reconstruction rather than an authority, and the banking rule says so when
	// it declines to run on it. A failed history read leaves the allowance
	// unknown, and unknown is UnlimitedTransfers here rather than a guess of 1:
	// guessing low would have the rule advise hoarding a transfer that may not
	// exist, which is the expensive direction.
	free := fpl.UnlimitedTransfers
	if h, err := client.History(ctx, cfg.EntryID); err == nil {
		free = fpl.FreeTransfers(h)
	}

	b := &transferBoard{
		Bank: bank, GW: gw, Free: free,
		Value: picks.EntryHistory.Value, Notes: notes,
	}
	b.Advice, b.Consulted = adviseBanking(cfg, e, state, pool, bank, free, gw)

	// The plans themselves carry the chip credit too, so a recommendation made
	// with a chip in view is the one displayed. Without this the banking advice
	// would price a planned chip and the moves printed beside it would not.
	state.Chip = chipCreditNow(cfg, e, gw)
	b.Plans = analysis.BuildPlans(state, pool, e.WeekEngine(), bank, 3, 5)
	return b, ""
}

// chipCreditNow is what a planned chip is worth to a transfer made this week.
//
// Zero unless `prepare_squad_for_chips` is on AND a chip is actually planned
// inside the horizon, so every other configuration is unaffected. This is the
// one channel by which a chip can be prepared for at all — without it the search
// assembles a squad for the average week and the chip is played on whatever
// fifteen happens to be owned.
func chipCreditNow(cfg config.Config, e *analysis.Engine, gw int) analysis.ChipCredit {
	if !cfg.Review.PrepareForChips {
		return analysis.ChipCredit{}
	}
	horizon, _ := e.EffectiveHorizon(cfg.Chips)
	cr := analysis.ChipCreditAt(cfg.Chips, true, true, gw, float64(horizon))
	if cr.Bench > 0 {
		// The bench boost values fifteen players in ONE gameweek, so it needs
		// that week's per-club fixture counts: a club playing twice is worth
		// double there, and the horizon-averaged FixtureLoad a player carries
		// reads 1.2 rather than 2. Without this the credit buys a good bench
		// instead of a doubling one.
		cr.WeekLoad = e.FixtureCountsIn(cfg.Chips.Next(analysis.SlotBenchBoost, gw))
	}
	return cr
}

// adviseBanking runs the banking rule over the squad you own.
//
// The two arms are priced exactly as the replay prices them — the best package
// available now over the horizon, against the best package one more transfer
// would buy over a horizon one shorter — through analysis.BestPackageValue, so
// the live recommendation and the replayed policy cannot disagree about what a
// package is worth or which way a tie goes.
//
// The later arm is priced at NEXT week's decision, so its chip credit is too: a
// boost one week nearer is one week less to amortise it over, which is the whole
// quantity this comparison turns on when a chip is planned.
//
// Unlimited free transfers — before the first deadline — is not a bankable
// state: there is no allowance to accumulate, so the rule is not consulted.
func adviseBanking(cfg config.Config, e *analysis.Engine, state analysis.SquadState,
	pool []analysis.PlayerMetrics, bank, free, gw int) (analysis.BankAdvice, bool) {

	if !cfg.Review.BankTransfersLookahead || free == fpl.UnlimitedTransfers {
		return analysis.BankAdvice{}, false
	}
	horizon, _ := e.EffectiveHorizon(cfg.Chips)
	value := func(limit, atGW int, over float64) float64 {
		if limit < 1 || over < 1 {
			return 0
		}
		st := state
		st.Chip = chipCreditNow(cfg, e, atGW)
		var packages []analysis.TransferPackage
		for _, p := range analysis.BuildPlans(st, pool, e.WeekEngine(), bank, limit, 5) {
			packages = append(packages,
				analysis.TransferPackage{Gain: p.GainPerGW, Moves: p.Transfers})
		}
		return analysis.BestPackageValue(packages, over,
			cfg.Review.FreeTransferValue, cfg.Review.MinGainForTransfer)
	}
	now := value(analysis.MoveLimit(free, cfg.Review.MaxHitsPerWeek, 0),
		gw, float64(horizon))
	later := value(analysis.MoveLimit(free+1, cfg.Review.MaxHitsPerWeek, 0),
		gw+1, float64(horizon-1))
	return analysis.AdviseBank(free, cfg.Review.BankUpTo, float64(horizon), now, later), true
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
