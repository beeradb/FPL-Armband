package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

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

	// Before anything else, and before the network. cmdSquad refuses on the same
	// footing: answering after the search has run makes the reader wait for work whose
	// output does not exist. Checking it at the END, as this did, also missed the two
	// outcomes that return early -- so a week where the policy banked or found nothing
	// exited 0 with no file and no message.
	if htmlPath != "" {
		return errPageRetired
	}
	if cfg.EntryID == 0 {
		return fmt.Errorf("no entry_id in config.json, so there is no squad to improve.\n" +
			"Set one, or use `armband squad` to build a fifteen from scratch")
	}
	// FPL only exposes picks once a deadline has passed. Before GW1 that is
	// expected rather than an error, and saying so is the difference between a
	// user thinking the tool is broken and knowing the season has not started.
	board, why := ownedTransferBoard(ctx, cfg, client, e)
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
	// The verdict in words, before the arithmetic that produced it. A reader deciding
	// whether to open FPL at all needs "act or hold" first; the two package values
	// underneath are what make it checkable rather than assertable.
	outcome := board.outcome()
	if outcome == outcomeBank || outcome == outcomeNothing {
		fmt.Printf("\n  %s\n", "We'd recommend a hold.")
	}
	// The banking rule's own reasoning — but not when the hold above came from an empty
	// board and the rule reached its comparison with nothing in either arm. The rule's
	// "not banking" means it did not fire; printed under "we'd recommend a hold" it
	// reads as a flat contradiction, because the two use the word in different senses.
	//
	// ⚠️ Narrowed to BankGuardNone deliberately. A guard means the comparison was never
	// made for a structural reason — the allowance is already at its ceiling, say — and
	// that text tells the reader banking is not available to him at all, which is not
	// the contradiction this suppresses and is worth keeping. An earlier version tested
	// only !Weighed(), which is ALWAYS true under outcomeNothing (an empty board gives
	// a zero now-arm, and any positive later-arm would have made this outcomeBank), so
	// it silently swallowed both guard messages as well.
	if board.Consulted &&
		!(outcome == outcomeNothing && board.Advice.Guard == analysis.BankGuardNone &&
			!board.Advice.Weighed()) {
		fmt.Printf("\n  %s\n", bankLine(board.Advice))
		// ⚠️ The chip plan is ALREADY in the two numbers above — liveHorizon calls
		// EffectiveHorizon and has always discarded its reason — but nothing said
		// so, and the effect is large enough that silence misleads. Measured
		// 2026-08-24 on the house squad at GW2: moving the planned wildcard from
		// GW6 to GW4 took the banking arms from "now 3.74 · waiting 2.31" to
		// "now 0.87 · waiting 0.00" while the recommended transfers, their gain
		// and their hinge stayed byte-identical. A reader comparing the two runs
		// could see the arithmetic move and had nothing telling him why.
		//
		// `armband chips` has always printed this; `transfers` reads the same
		// plan through the same function and did not. One fact, one wording, both
		// commands.
		if _, why := e.EffectiveHorizon(cfg.Chips); why != "" {
			fmt.Printf("    %s\n", dim(why+" — a transfer made now earns over fewer weeks"))
		}
	}

	// Both renderers switch on transferBoard.outcome and nothing else, which is
	// what stops this command and the page disagreeing about the same board.
	//
	// A banked week names what it declined — "wait" is only actionable if you know
	// what you are waiting to afford — but as a declined option rather than as a
	// recommendation, and no team sheet is drawn for a move nobody is making.
	switch outcome {
	case outcomeBank:
		// A declined week still names what it declined, because "wait" is only
		// actionable if you know what you are waiting to afford. Offered as the
		// alternative to the recommendation rather than as the recommendation.
		if len(plans) > 0 {
			fmt.Printf("\n  %s\n", dim("But if you must make a move:"))
			printPlanLines(os.Stdout, topPlans(equivalentTo(plans, cfg.Review.MinSeparableGain), 3))
		}
		fmt.Println()
		return nil
	case outcomeNothing:
		// Distinct from the case above and the difference matters: there the rule
		// weighed a real move and preferred waiting, here there is no move to weigh.
		// Collapsing them would offer a reader alternatives that do not exist.
		fmt.Printf("  %s\n", dim(emptyBoardReason))
		fmt.Printf("  %s\n\n", dim(bankingIsFirstClass))
		return nil
	}

	// The best plan in full, then the runners-up as one-liners. Printing five
	// complete team sheets buries the decision.
	best := plans[0]
	present.Moves(os.Stdout, best, theme)
	present.Squad(os.Stdout, planSquad(best), theme,
		fmt.Sprintf("The eleven this would field in GW%d", board.GW))

	printSeparableBand(os.Stdout, plans, cfg.Review.MinSeparableGain)

	return nil
}

// emptyBoardReason and bankingIsFirstClass are the two halves of what an empty board
// says, in one place because THREE surfaces now render the same transferBoard and must
// not describe the same state three ways — which is the reason transferBoard exists at
// all. The CLI, `bestPlanForOwnedSquad` (which the page renders as NoPlan) and
// `apitransfers.go`'s document for the builder all read these.
//
// ⚠️ The third arrived while this was being written, carrying its own copy of the same
// wrong sentence. That is the mirror this constant exists to close, and it will happen
// again: a fourth surface will be written by someone who has not read this comment. If
// the wording needs to differ per surface, give boardOutcome a method rather than
// letting a literal back in.
//
// ⚠️ The wording these replaced was "No move clears the threshold this week", and it
// described a threshold that is not applied: BuildPlans keeps every move with a
// positive gain and imposes no floor of its own. An empty board means nothing raises
// the eleven at all, which is a stronger and different statement.
const (
	emptyBoardReason = "Nothing on the board would improve this squad, so there is no " +
		"move to offer even if you wanted one."
	bankingIsFirstClass = "Banking the transfer is a first-class outcome and usually " +
		"the right one."
)

// bestPlanForOwnedSquad returns the best transfer plan for the squad you own,
// or a plain-language reason there is none.
//
// A banking decision outranks the plan and is returned as the reason: when the
// rule says wait, the best plan is one the policy has decided not to make, and
// printing it as a recommendation is the opposite of what was decided.
func bestPlanForOwnedSquad(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine) (*analysis.Plan, string) {

	board, why := ownedTransferBoard(ctx, cfg, client, e)
	if board == nil {
		return nil, why
	}
	switch board.outcome() {
	case outcomeBank:
		return nil, board.Advice.Explain()
	case outcomeNothing:
		return nil, emptyBoardReason + " " + bankingIsFirstClass
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

// boardOutcome is what the week's decision comes to, as one value both renderers
// switch on.
type boardOutcome int

const (
	// outcomeRecommend: make the best plan.
	outcomeRecommend boardOutcome = iota
	// outcomeBank: the banking rule says hold this week's transfer.
	outcomeBank
	// outcomeNothing: no plan on offer at all.
	outcomeNothing
)

// outcome is the single decision, and it exists because sharing the board was not
// sharing the decision.
//
// The command and the page each read the same `transferBoard` and each decided
// for themselves what it meant: the page returned no plan when the rule said
// wait, while the command printed the advice and then rendered the moves and a
// team sheet anyway. Same config, same squad, opposite recommendations — the
// exact failure the shared board was introduced to prevent, one layer up from
// where it was prevented.
//
// Banking outranks a plan. When the rule says wait, the best plan is one the
// policy has decided not to make, and presenting it as a recommendation is the
// opposite of what was decided.
func (b *transferBoard) outcome() boardOutcome {
	switch {
	case b.Consulted && b.Advice.Bank:
		return outcomeBank
	case len(b.Plans) == 0:
		return outcomeNothing
	default:
		return outcomeRecommend
	}
}

// ownedTransferBoard assembles the weekly decision for `cfg.EntryID`'s OWN squad — the CLI's
// and `bestPlanForOwnedSquad`'s caller, neither of which has a session to read a fifteen
// from. It fetches the entry's actual picks fresh, converts them to permanent codes (the
// keyspace buildTransferBoard's `squad` parameter takes — see that function's own comment),
// and prices the search at the entry's own sell prices.
//
// `client.Entry`/`client.Picks` are fetched again inside buildTransferBoard, for the bank,
// squad value and current event — a second call to the same cached endpoint, which is a
// disk read within the process's cacheTTL rather than a second round trip to FPL. That
// redundancy buys buildTransferBoard one signature for both callers, which is the point:
// see that function's own comment on why `squad` is a parameter rather than something it
// derives itself.
func ownedTransferBoard(ctx context.Context, cfg config.Config, client *fpl.Client,
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
	codes := make([]int, 0, len(picks.Picks))
	for _, p := range picks.Picks {
		if el := e.Boot.ElementByID(p.Element); el != nil {
			codes = append(codes, el.Code)
		}
	}
	if len(codes) != 15 {
		return nil, fmt.Sprintf("Read %d of 15 picks, so no transfers were computed.", len(codes))
	}
	return buildTransferBoard(ctx, cfg, client, e, cfg.EntryID, codes, e.SellPrices)
}

// buildTransferBoard assembles the weekly decision for one entry's squad, or a
// plain-language reason there is none.
//
// The reason is returned rather than swallowed on purpose. "No transfers shown"
// and "you have no squad yet" and "the model wants no move this week" look
// identical as an absent section, and only the last of those is a
// recommendation — this project's review policy says doing nothing is a
// first-class outcome and usually the right one, which is worth saying out loud
// rather than leaving as a gap in a page.
//
// Three parameters carry every difference between the two callers, and this function has
// no branch on which one reached it — the same rule internal/viewmodel/build.go states for
// buildResults, applied here:
//
//   - entryID: cfg.EntryID for the CLI (through ownedTransferBoard), a reader's own
//     sess.Entry for the page.
//   - squad: permanent player CODES. The CLI's own owned fifteen, fetched fresh from FPL's
//     picks (see ownedTransferBoard). The page must pass the reader's CURRENT pitch —
//     sess.Squad — and NOT re-derive it from picks: the reader has been editing, and a
//     suggestion computed from a fifteen that is no longer on screen would offer to sell
//     players who are not there.
//   - sell: purchase-price-aware selling prices, keyed by element id, or nil for
//     sell-at-market. The CLI passes e.SellPrices, which is cfg.EntryID's own purchase
//     history. A page caller MUST pass nil — engine.SellPrices belongs to the site's own
//     squad, and handing it to a visitor's search would apply the house team's purchase
//     history to a stranger's players, keyed on element ids that overlap by coincidence.
//     client.SquadPrices, the honest per-visitor alternative, is not affordable at this
//     traffic shape — see the design note this implements. The panel says "priced at
//     market" instead of pretending otherwise.
//
// The entry and picks fetches stay regardless of caller, but only for EntryHistory.Bank,
// EntryHistory.Value and entry.CurrentEvent — never for the squad, which is always the
// `squad` parameter above.
func buildTransferBoard(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine, entryID int, squad []int, sell map[int]int) (*transferBoard, string) {

	if entryID == 0 {
		return nil, "No entry_id is configured, so there is no squad to improve — " +
			"this page is a fifteen built from scratch."
	}
	entry, err := client.Entry(ctx, entryID)
	if err != nil {
		return nil, "Could not read your entry, so no transfers were computed: " + err.Error()
	}
	if entry.CurrentEvent == nil {
		return nil, "FPL exposes picks only after a deadline has passed, so there is no " +
			"squad to improve yet. Before GW1 this page is the opening fifteen."
	}
	picks, err := client.Picks(ctx, entryID, *entry.CurrentEvent)
	if err != nil {
		return nil, "Could not read your picks, so no transfers were computed: " + err.Error()
	}

	var squadMetrics []analysis.PlayerMetrics
	for _, code := range squad {
		if el := e.Boot.ElementByCode(code); el != nil {
			squadMetrics = append(squadMetrics, e.Metrics(el))
		}
	}
	if len(squadMetrics) != 15 {
		return nil, fmt.Sprintf("Read %d of 15 players in your squad, so no transfers were computed.", len(squadMetrics))
	}

	state := analysis.NewSquadState(squadMetrics)
	// FPL pays what you paid plus half of any rise, never the market price. Nil
	// means sell-at-market, which overstates the budget.
	state.Sell = sell

	// Standing overrides bind here for the reason recorded in the research
	// record: excluding a player from squad builds while the transfer search
	// still offers to buy him is worse than not excluding him at all.
	req := analysis.OptimizeRequest{}
	notes := applyRoster(cfg, e, &req)
	excluded := map[int]bool{}
	for _, id := range req.ExcludeIDs {
		excluded[id] = true
	}
	// ⚠️ The floor is a SEASON TOTAL and must be scaled to the window FPL's aggregates
	// actually cover. A bare `c.Minutes < 600` is the same defect ScaledMinMinutesFor
	// was written for, and its own doc comment describes the symptom exactly: "the
	// floor stayed at its unscaled 600, and every player in the game failed it against
	// a fresh-season minutes count."
	//
	// That was fixed for Optimize and NOT here, so the two searches disagreed about who
	// is even a candidate. Measured on the live GW1 data that exposed it: 0 of 609
	// players had 600+ minutes because the aggregates reset when GW1 completed, and the
	// most anyone had played was 90. The pool was empty, BuildPlans had nothing to rank,
	// and every transfer search — `armband transfers`, the agent's suggest_transfers and
	// the page's Suggest-transfers button alike — answered "nothing would improve this
	// squad" for every user. Not a wrong answer a reader could argue with: a silent one,
	// for roughly the first seven gameweeks of every season, until players accumulate
	// enough minutes to clear a bar meant for a full season.
	//
	// Scaled, the same floor reads 600*1/38 = 15 after one gameweek, which a 90-minute
	// starter clears and a 10-minute substitute does not — which is what it was always
	// for.
	//
	// Memoised per club exactly as Optimize's own floorFor does: inLiveGameweekGap the
	// window is that club's own finished matches, so it genuinely differs between teams
	// mid-gameweek.
	scaledFloor := map[int]int{}
	floorFor := func(teamID int) int {
		if v, ok := scaledFloor[teamID]; ok {
			return v
		}
		v := e.ScaledMinMinutesFor(teamID, transferMinSeasonMinutes)
		scaledFloor[teamID] = v
		return v
	}
	var pool []analysis.PlayerMetrics
	for _, c := range e.AllMetrics() {
		if excluded[c.ID] || c.Minutes < floorFor(c.TeamID) {
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
	// reconstruction rather than an authority. A failed read leaves it unknown,
	// and unknown is UnlimitedTransfers rather than a guess of 1: guessing low
	// would have the rule advise hoarding a transfer that may not exist, which is
	// the expensive direction.
	//
	// ⚠️ **And it must say so.** The fallback silently disables banking — the rule
	// declines on an unlimited allowance — which on a transient network failure is
	// indistinguishable from the switch being off. A capability that vanishes
	// quietly is the failure this whole change exists to stop, so the reason is
	// carried and printed.
	free := fpl.UnlimitedTransfers
	var notes2 []string
	if h, err := client.History(ctx, entryID); err == nil {
		free = fpl.FreeTransfers(h)
	} else if cfg.Review.BankTransfersLookahead {
		notes2 = append(notes2, "Could not read your transfer history, so the free-transfer "+
			"allowance is unknown and the banking rule was not run: "+err.Error())
	}

	// Priced before `build` so the closure can charge over it. The banking
	// comparison re-prices its later arm over `horizon-1` itself.
	horizon := liveHorizon(cfg, e, gw)

	// One plan builder, shared by the banking comparison and the recommendation,
	// so the rule cannot weigh a package space the command does not offer.
	//
	// ⚠️ Asks for `planCandidatePool`, not the five printed, because BuildPlans
	// truncates on GROSS gain. See rankPlansByNetValue.
	build := func(st analysis.SquadState, limit int) []analysis.Plan {
		plans := analysis.BuildPlans(st, pool, e.WeekEngine(), bank, limit, planCandidatePool)
		return rankPlansByNetValue(cfg, plans, free, gw, horizon)
	}

	// A lever this command cannot honour says so rather than going quiet. See
	// noteUnhonouredLevers: the taper reaches every live consumer, the three chip
	// state rules reach none, and a setting that silently means nothing is
	// indistinguishable from one that means what it says.
	if n := noteUnhonouredLevers(cfg.OptionValue); n != "" {
		notes2 = append(notes2, n)
	}
	b := &transferBoard{
		Bank: bank, GW: gw, Free: free,
		Value: picks.EntryHistory.Value, Notes: append(notes, notes2...),
	}
	b.Advice, b.Consulted = adviseBanking(cfg, e, state, build, free, gw, horizon)

	// The plans themselves carry the chip credit too, so a recommendation made
	// with a chip in view is the one displayed. Without this the banking advice
	// would price a planned chip and the moves printed beside it would not.
	state.Chip = chipCreditNow(cfg, e, gw, horizon)
	b.Plans = build(state, liveMoveLimit(cfg, free))
	return b, ""
}

// chipCreditNow is what a planned chip is worth to a transfer made in gameweek
// `gw`, amortised over `horizon`.
//
// Zero unless `prepare_squad_for_chips` is on AND a chip is actually planned
// inside the horizon, so every other configuration is unaffected. This is the
// one channel by which a chip can be prepared for at all — without it the search
// assembles a squad for the average week and the chip is played on whatever
// fifteen happens to be owned.
//
// ⚠️ **The horizon is a parameter and must be the one the arm is valued over.**
// It read the engine's horizon unconditionally, so the banking rule's later arm
// got a credit of `1/h` and then multiplied it by `h-1`, losing a fifth of the
// chip's value at the shipped horizon of 5 — on the waiting arm only, which
// biased the rule against banking in exactly the weeks a chip was planned, which
// is the case the feature exists for. The replay never had this: `shouldBank`
// passes the shortened horizon, so `1/(h-1) x (h-1)` cancels to the whole chip.
// The window bound moves with it, so `[gw+1, gw+1+(h-1))` matches the replay
// rather than reaching a gameweek further.
func chipCreditNow(cfg config.Config, e *analysis.Engine, gw int, horizon float64) analysis.ChipCredit {
	if !cfg.Review.PrepareForChips {
		return analysis.ChipCredit{}
	}
	cr := analysis.ChipCreditAt(cfg.Chips, true, true, gw, horizon)
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

// planFn builds the ranked plans for one squad state at one move limit.
//
// A parameter rather than a direct call so the banking decision can be exercised
// without a bootstrap, a squad and a network. That is not a convenience: the
// previous shape could only be tested through its two refusal arms, both of which
// return before the engine is touched, so the suite pinned that the switch turns
// the rule OFF and nothing pinned that it turns it ON — the identical hole a
// review had just proved for the sweep's own wiring, one commit later.
type planFn func(st analysis.SquadState, limit int) []analysis.Plan

// liveHorizon is how many gameweeks a transfer made now can still earn over.
//
// Two clamps, and the second was missing. `EffectiveHorizon` shortens for a
// planned wildcard — the squad does not survive one — and it does NOT clamp at
// the end of the season, because it answers "how long must this squad serve"
// rather than "how many gameweeks are left". The replay clamps separately, in
// `effectiveHorizon(configured, gw)`.
//
// Without the second clamp the banking rule's horizon guard is unreachable live,
// and a manager at GW38 is advised to hold a transfer into a gameweek that does
// not exist.
func liveHorizon(cfg config.Config, e *analysis.Engine, gw int) float64 {
	h, _ := e.EffectiveHorizon(cfg.Chips)
	return liveHorizonFor(gw, h)
}

// liveHorizonFor is the season-end clamp on its own, so the arithmetic can be
// checked without a bootstrap. `38 - gw + 1` counts gw itself, because a transfer
// made now plays in it.
func liveHorizonFor(gw, horizon int) float64 {
	if left := 38 - gw + 1; left < horizon {
		horizon = left
	}
	if horizon < 0 {
		horizon = 0
	}
	return float64(horizon)
}

// adviseBanking runs the banking rule over the squad you own.
//
// The two arms are priced exactly as the replay prices them — the best package
// available now over the horizon, against the best package one more transfer
// would buy over a horizon one shorter — through analysis.BestPackageValue, and
// the decision itself is analysis.AdviseBank. So the live recommendation and the
// replayed policy cannot disagree about what a package is worth, about which way
// a tie goes, or about either guard.
//
// The later arm is priced at NEXT week's decision over a horizon one shorter, and
// its chip credit is amortised over that same shorter horizon: a boost one week
// nearer is one week less to spread it across, which is the whole quantity this
// comparison turns on when a chip is planned.
//
// Unlimited free transfers — before the first deadline — is not a bankable
// state: there is no allowance to accumulate, so the rule is not consulted.
// The horizon is a parameter rather than derived here, so the rule can be
// exercised without a bootstrap — see TestTheLiveBankingRuleDecidesBothWays. The
// caller gets it from liveHorizon, which is the only place the two clamps live.
func adviseBanking(cfg config.Config, e *analysis.Engine, state analysis.SquadState,
	build planFn, free, gw int, horizon float64) (analysis.BankAdvice, bool) {

	if !cfg.Review.BankTransfersLookahead || free == fpl.UnlimitedTransfers {
		return analysis.BankAdvice{}, false
	}
	value := func(limit, atGW int, over float64) float64 {
		if limit < 1 || over < 1 {
			return 0
		}
		st := state
		st.Chip = chipCreditNow(cfg, e, atGW, over)
		var packages []analysis.TransferPackage
		for _, p := range build(st, limit) {
			packages = append(packages,
				analysis.TransferPackage{Gain: p.GainPerGW, Moves: p.Transfers})
		}
		// The charge, tapered when the lever is on. Through
		// config.OptionValuePolicy.FreeTransferCharge, which resolves the switch
		// and delegates to analysis.TransferHoldFactorFor — the same call the
		// replay's `decide` makes — so the live recommendation and the replayed
		// policy cannot disagree about what a transfer costs, exactly as they
		// already cannot disagree about what a package is worth.
		//
		// The scheduled early floor applies to the BASE first — schedule first,
		// curve second — the same composition the replay's `decide` and the swap
		// tool use, so the shipped schedule reaches the banking comparison.
		//
		// ⚠️ Priced at `atGW`, so the later arm gets NEXT week's charge rather
		// than this week's. That is the asymmetry the replay's shouldBank names
		// and does not correct; here the arms are already valued at their own
		// gameweeks, so charging them at their own gameweeks costs nothing.
		baseCharge, minGain := cfg.Review.EffectiveFloor(atGW)
		charge := cfg.OptionValue.FreeTransferCharge(
			baseCharge, e, heldIDs(state), atGW)
		return analysis.BestPackageValue(packages, over,
			charge, minGain)
	}
	now := value(liveMoveLimit(cfg, free), gw, horizon)
	later := value(liveMoveLimit(cfg, free+1), gw+1, horizon-1)
	return analysis.AdviseBank(free, liveBankCeiling(cfg), horizon, now, later), true
}

// planCandidatePool is how many plans BuildPlans is asked for before they are
// re-ranked on net value, and it must exceed the five printed.
//
// BuildPlans sorts on GainPerGW and then truncates (`plan.go`), so asking it for
// five returns the five highest-GROSS plans. At a two-move limit on 2026-09-01
// all five were two-move packages gaining +0.76/gw, and the one-move package
// gaining +0.67 — worth MORE once its own transfer was paid for, because it took
// no hit — was evicted before anything priced it. Both the printed advice and the
// banking rule's `now` arm read the worse package as the best available.
const planCandidatePool = 25

// rankPlansByNetValue orders plans on what they are worth after paying for the
// transfers they spend, and drops any that take a hit without clearing
// `min_net_gain_for_minus_4`.
//
// # Why this is not analysis.PackageValue
//
// `analysis.TransferPackage` is `{Gain, Moves}` — it has no hits field, so
// `PackageValue` charges every move the same `free_transfer_value` and CANNOT
// price a -4. `fpl.HitCost` appears nowhere in `internal/analysis`. The replay
// prices hits in its own gate (`transferProposal.value`), unexported and in
// another package, so the live command had no hit-aware valuation at all: it
// ranked on gross gain and printed the top of that list.
//
// ⚠️ The replay already records the phenomenon this fixes, as a property rather
// than a defect: "a wider limit can substitute a higher-gain, costlier package
// that is worth less once charged". True, and harmless where it is counted by a
// diagnostic. Not harmless where it becomes the recommendation.
//
// ⚠️ `min_net_gain_for_minus_4` was inert before this. It is written into the
// config, printed to the reader in `brief` as "Min net gain across the horizon to
// justify a -4", and passed to the agent prompt, the page and the backtest — and
// read by neither this command nor internal/analysis. A gate that is displayed and
// not applied is worse than one that is absent.
//
// Pre-deadline (`UnlimitedTransfers`) no hit can exist, so the order is left alone.
func rankPlansByNetValue(cfg config.Config, plans []analysis.Plan,
	free, gw int, horizon float64) []analysis.Plan {

	if free == fpl.UnlimitedTransfers || len(plans) == 0 {
		return plans
	}
	charge, _ := cfg.Review.EffectiveFloor(gw)
	net := func(p analysis.Plan) float64 {
		hits := p.Transfers - free
		if hits < 0 {
			hits = 0
		}
		gross := p.GainPerGW * horizon
		// The gate is on what the hit itself has to clear, so it is measured
		// after the -4 and before the per-move charge — the quantity the config
		// key names and the brief prints.
		if hits > 0 && gross-float64(fpl.HitCost*hits) < cfg.Review.MinGainForHit {
			return math.Inf(-1)
		}
		return gross - float64(fpl.HitCost*hits) - charge*float64(p.Transfers-hits)
	}
	kept := make([]analysis.Plan, 0, len(plans))
	for _, p := range plans {
		if !math.IsInf(net(p), -1) {
			kept = append(kept, p)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool { return net(kept[i]) > net(kept[j]) })
	if len(kept) > 5 {
		kept = kept[:5]
	}
	return kept
}

// liveMoveLimit is how many transfers are actually available, and is what BOTH
// the banking comparison's now-arm and the printed recommendation are built from.
//
// They were two different numbers: the rule asked what `MoveLimit(free, ...)`
// buys while the command offered plans of up to three moves regardless of the
// allowance. At one free transfer the rule weighed at most two moves against a
// recommendation of up to three, so it could advise banking about the very
// package printed beside it — and the extra plans were not executable anyway.
func liveMoveLimit(cfg config.Config, free int) int {
	if free == fpl.UnlimitedTransfers {
		// Before the first deadline the squad can be rebuilt freely, so the
		// allowance is not the constraint. Three is what the command has always
		// offered and there is no reason to narrow it here.
		return 3
	}
	return analysis.MoveLimit(free, cfg.Review.MaxHitsPerWeek, 0, cfg.Review.HitCeiling)
}

// liveBankCeiling is the allowance the ceiling guard is measured against.
//
// Clamped to what FPL actually grants, because `fpl.FreeTransfers` reconstructs
// the allowance under the same cap: a config saying `bank_transfers_up_to: 8`
// would otherwise put the guard permanently out of reach, since the reconstructed
// figure can never reach 8. The replay is unaffected — it takes its ceiling from
// `BankLimitFor(season)`, the rule that was actually in force.
func liveBankCeiling(cfg config.Config) int {
	if n := fpl.MaxBankedTransfers; cfg.Review.BankUpTo > n {
		return n
	}
	return cfg.Review.BankUpTo
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

// heldIDs is the element ids of a squad state, for the congestion read.
//
// `SquadState` carries `Players` and an `Owned` set, and neither is a slice of
// ids — the search never needed one. Ranging `Owned` would be a map walk, which
// this package has already made an optimiser non-deterministic once; `Players` is
// ordered, so it is the side to read.
func heldIDs(st analysis.SquadState) []int {
	out := make([]int, 0, len(st.Players))
	for _, p := range st.Players {
		out = append(out, p.ID)
	}
	return out
}

// topPlans returns at most n plans, already ranked by BuildPlans.
func topPlans(plans []analysis.Plan, n int) []analysis.Plan {
	if len(plans) < n {
		return plans
	}
	return plans[:n]
}

// printPlanLines renders plans as one scannable line each: what it is worth, then
// what it does. The gain leads because it is the column a reader compares on.
func printPlanLines(w io.Writer, plans []analysis.Plan) {
	for _, p := range plans {
		fmt.Fprintf(w, "    %s  %s\n",
			dim(fmt.Sprintf("%+.2f pts/gw", p.GainPerGW)), movesLine(p))
	}
}

// bandEpsilon absorbs float representation error at the edge of the band.
//
// Without it, which side of the band a move falls on is decided by arithmetic noise:
// 1.00-0.59 evaluates to 0.41000000000000003, so a plan sitting EXACTLY 0.41 behind the
// leader is reported as separable from it while an identical gap computed from different
// operands is not. That is the arbitrariness this whole band exists to remove, so it
// cannot be reintroduced by the comparison that implements it.
//
// 1e-9 is representation slack, not policy slack: gains carry two decimals of meaning and
// this is six orders of magnitude below that, so it can never widen the band in a way a
// reader could notice.
const bandEpsilon = 1e-9

// transferMinSeasonMinutes is the sample-size floor for a transfer candidate, written
// as a SEASON TOTAL and scaled per club before use. Too little football behind a player
// and his per-90 rates are not believable.
//
// It matches the 600 Optimize's OptimizeRequest.MinMinutes ships with, deliberately: a
// player the optimiser would not buy should not be offered as a transfer either, and
// two different bars would make the squad page and the transfer page disagree about who
// exists.
const transferMinSeasonMinutes = 600

// equivalentTo returns the leading run of plans this model cannot tell apart from the
// best one: every plan whose gain is within band of plans[0]'s.
//
// plans must already be ranked by gain, which BuildPlans guarantees, so the run is a
// prefix and the scan can stop at the first plan that falls outside it.
func equivalentTo(plans []analysis.Plan, band float64) []analysis.Plan {
	if len(plans) == 0 || band <= 0 {
		return plans
	}
	top := plans[0].GainPerGW
	for i, p := range plans {
		if top-p.GainPerGW > band+bandEpsilon {
			return plans[:i]
		}
	}
	return plans
}

// printSeparableBand shows the moves that are the same answer as the recommended one,
// rather than a ranking of runners-up.
//
// # Why this is not "also considered"
//
// It used to be, and that was the winner's curse rendered as a list. AGENTS.md's argmax
// box: "take six noisy estimates and keep the biggest: the winner is usually not the
// best option, it is the option whose estimate got the most flattering noise... That is
// why this record wants a shape rather than a winner, and why the transfer search -- an
// argmax over players -- reaches for whichever player the model most over-rates."
//
// Printing #2 and #3 UNDER the recommendation says the model ranked them and they lost.
// It did not: within MinSeparableGain it has no opinion, and presenting one of them as
// the answer hands the reader a pick made by noise. So the band is stated as a set, and
// the reader is told what to break the tie on -- something the model cannot see.
//
// A single-member band prints nothing. There is no tie to report, and a heading over one
// row would imply the others were beaten rather than absent.
func printSeparableBand(w io.Writer, plans []analysis.Plan, band float64) {
	rest := equivalentTo(plans, band)[1:]
	if len(rest) == 0 {
		return
	}
	fmt.Fprintf(w, "\n  %s\n", fmt.Sprintf("%d more moves are the same answer as that one.", len(rest)))
	printPlanLines(w, rest)
	fmt.Fprintf(w, "  %s\n\n", dim(fmt.Sprintf(
		"All within %.2f pts/gw of the pick above, which is what this model over-rates "+
			"its own top picks by. It cannot separate them — break the tie on something "+
			"it cannot see.", band)))
}

// defaultSeparableGain is the shipped band, read from config's own defaults so the test
// that pins it against docs/accuracy.md cannot drift from what actually ships.
func defaultSeparableGain() float64 { return config.Default().Review.MinSeparableGain }
