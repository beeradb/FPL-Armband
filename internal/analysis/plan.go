package analysis

import "sort"

// A transfer plan: a complete answer rather than a menu.
//
// RankSwaps and RankPairs return moves, and leaving them as moves pushes real
// work onto the reader — assembling the resulting fifteen, working out who would
// then start, noticing that an upgrade is only worth anything if the man arriving
// displaces someone. A plan carries all of that already resolved.
//
// # Two different elevens, on purpose
//
// GainPerGW is measured on the eleven the *scoring* model picks, which reads a
// five-gameweek fixture average. That is the number every replayed result in
// this project was produced under and it is not changed here.
//
// XI is the eleven you would actually field this week, picked on the imminent
// fixture. Those differ — a player with four easy games and one hard one is
// started in all five by the horizon view and benched for the hard one by the
// weekly view. Closing that gap in the *decision* was measured as worth nothing
// (2149 against 2152), which is why the decision still uses the horizon. But
// showing a recommendation alongside an eleven nobody would field is simply
// wrong, so the display uses the week.
type Plan struct {
	// Moves is what changes, out-for-in.
	Moves []Swap
	// Transfers is how many the plan costs.
	Transfers int
	// Squad is the resulting fifteen.
	Squad []PlayerMetrics
	// XI, Bench, Formation and Captain are for the imminent gameweek. Bench is
	// in substitution order: the keeper, then outfielders best first.
	XI        []PlayerMetrics
	Bench     []PlayerMetrics
	Formation string
	Captain   PlayerMetrics
	// GainPerGW is the change in XIValue against the current squad, on the
	// scoring horizon. This is the decision number.
	GainPerGW float64
	// Spend is the net cost in tenths; negative frees money.
	Spend int

	// DependsOn is the player whose unavailability would hurt this plan most,
	// and GainIfOut is what the plan would then be worth. Together they answer
	// "is there anything left to learn before acting?" — which is the question
	// that decides whether to move tonight or wait for team news.
	DependsOn PlayerMetrics
	GainIfOut float64
	// SurvivesLoss is whether the plan still beats doing nothing without him.
	// When false, acting before that player's status is known is a gamble on
	// the one fact that decides it.
	SurvivesLoss bool
}

// applyTo returns the squad that results from the moves.
func applyMoves(squad []PlayerMetrics, moves []Swap) []PlayerMetrics {
	out := append([]PlayerMetrics(nil), squad...)
	for _, m := range moves {
		for i := range out {
			if out[i].ID == m.Out.ID {
				out[i] = m.In
				break
			}
		}
	}
	return out
}

// BuildPlans turns the transfer search's output into complete, ranked plans.
//
// week is an engine whose horizon is the imminent gameweek, used only to pick
// the eleven each plan would actually field. Pass nil to fall back to the
// scoring horizon, which is what the decision uses anyway.
func BuildPlans(s SquadState, candidates []PlayerMetrics, week *Engine,
	bank, maxMoves, limit int) []Plan {

	base := s.value(s.Players)
	var plans []Plan

	add := func(moves []Swap) {
		if len(moves) == 0 || len(moves) > maxMoves {
			return
		}
		squad := applyMoves(s.Players, moves)
		spend := 0
		for _, m := range moves {
			spend += tenths(m.In.Price) - s.sellPrice(m.Out)
		}
		p := Plan{
			Moves:     moves,
			Transfers: len(moves),
			Squad:     squad,
			GainPerGW: s.value(squad) - base,
			Spend:     spend,
		}
		p.XI, p.Bench, p.Formation, p.Captain = fieldedXI(squad, week)
		p.DependsOn, p.GainIfOut = weakestLink(s, squad)
		p.SurvivesLoss = p.GainIfOut > 0
		plans = append(plans, p)
	}

	for _, sw := range RankSwaps(s, candidates, bank) {
		add([]Swap{sw})
	}
	if maxMoves >= 2 {
		for _, pr := range RankPairs(s, candidates, bank, maxMoves-1, limit) {
			moves := append([]Swap{pr.Up}, pr.Downs...)
			add(moves)
		}
	}

	sort.SliceStable(plans, func(i, j int) bool { return plans[i].GainPerGW > plans[j].GainPerGW })
	if len(plans) > limit {
		plans = plans[:limit]
	}
	return plans
}

// fieldedXI picks the eleven a squad would actually start this week.
//
// The squad's players carry horizon-averaged scores. Re-scoring them on the
// imminent fixture changes who starts but must not change the plan's reported
// gain, so the returned players carry the week's scores and the plan's GainPerGW
// is computed separately.
func fieldedXI(squad []PlayerMetrics, week *Engine) (xi, bench []PlayerMetrics, formation string, captain PlayerMetrics) {
	pick := squad
	if week != nil {
		if rescored := week.rescore(squad); len(rescored) == len(squad) {
			pick = rescored
		}
	}
	xi, bench, formation = bestXI(pick)
	for _, p := range xi {
		if p.Score > captain.Score {
			captain = p
		}
	}
	// Substitution order: the reserve keeper is not interchangeable with an
	// outfielder, and the first outfield sub is the one that actually comes on.
	sort.SliceStable(bench, func(i, j int) bool {
		if (bench[i].Position == "GKP") != (bench[j].Position == "GKP") {
			return bench[i].Position == "GKP"
		}
		return bench[i].Score > bench[j].Score
	})
	return xi, bench, formation, captain
}

// rescore returns the same players scored on this engine's horizon.
func (e *Engine) rescore(squad []PlayerMetrics) []PlayerMetrics {
	byID := map[int]PlayerMetrics{}
	for _, m := range e.AllMetrics() {
		byID[m.ID] = m
	}
	out := make([]PlayerMetrics, 0, len(squad))
	for _, p := range squad {
		if m, ok := byID[p.ID]; ok {
			out = append(out, m)
			continue
		}
		out = append(out, p)
	}
	return out
}

// WeekEngine returns a view of this engine whose fixture horizon is the imminent
// gameweek, for deciding who would actually be fielded.
//
// It is built lazily and once: the calibration passes in NewEngineFull are not
// free, and a fresh one per plan would multiply that by however many plans are
// on offer. Under a sync.Once because the tool runner drives searches from
// several goroutines at once.
//
// # Every carried field, not a chosen subset
//
// The constructor takes the bootstrap, the fixtures, the weights and the two
// risk models; **everything else a caller may set on an Engine has to be copied
// here by hand**, and `TeamForm` was missed when it arrived. It was inert only
// because `teamFormWeight` ships at zero.
//
// **The exposure was live, not in the replay.** `internal/backtest` never calls
// this — it builds each engine directly and sets `TeamForm` on all six — so no
// replayed figure could have moved and no sweep of `FPL_TEAM_FORM` was ever
// unsafe. What was wrong is the agent's own output: `Plan.GainPerGW` carried the
// club-form view and `Plan.XI`, picked through this engine, did not, so a
// recommendation was scored on one view of the league and displayed beside an
// eleven chosen on another. `WeekViews` had the same split through `engineAt`.
// See TestDerivedEnginesCarryEverySource, which derives the required list from
// the struct rather than trusting this comment.
//
// The budget fields go across too. Nothing reads them off a derived engine
// today, which is exactly why they were absent; copying them costs nothing and
// removes the trap where a later `wk.AssemblyBudget()` quietly answers from an
// empty wallet.
func (e *Engine) WeekEngine() *Engine {
	e.weekOnce.Do(func() {
		w := e.Weights
		w.Horizon = 1
		wk := NewEngineFull(e.Boot, e.Fixtures, w, e.Cong, e.Role)
		wk.Priors = e.Priors
		wk.Recent = e.Recent
		// Carried, or the eleven is fielded under one selection policy and the
		// transfer decided under another — the split
		// TestDerivedEnginesCarryEverySource records for TeamForm, and inert in
		// exactly the same way until somebody switches the flag on.
		wk.Tiebreak = e.Tiebreak
		// Read together under overrideMu, the same rule minutesOverrideFor's
		// doc comment states: three unguarded field reads here could each
		// observe a different SetMinutesOverride write (set_player_status runs
		// concurrently with other tool calls in the same turn), pairing one
		// write's minutes with another's expiry or confirmed flag.
		e.overrideMu.RLock()
		wk.MinutesOverride = e.MinutesOverride
		wk.MinutesOverrideUntil = e.MinutesOverrideUntil
		wk.MinutesOverrideConfirmed = e.MinutesOverrideConfirmed
		e.overrideMu.RUnlock()
		wk.TeamXGCFactor = e.TeamXGCFactor
		wk.TeamForm = e.TeamForm
		wk.SellPrices = e.SellPrices
		wk.Chips = e.Chips
		wk.Entry = e.Entry
		wk.SquadValue = e.SquadValue
		wk.HypotheticalBudget = e.HypotheticalBudget
		wk.Bank = e.Bank
		wk.Budget = e.Budget
		// Deliberately not copied: skipped gameweeks. A free hit removes a week
		// from the *scoring* horizon because the squad will not play it; asking
		// who to field this week is a different question, and if this week is the
		// free hit there is no permanent eleven to pick anyway.
		e.weekEngine = wk
	})
	return e.weekEngine
}

// weakestLink finds the player whose absence would cost this plan the most, and
// what the plan would then be worth.
//
// # Why both squads are zeroed
//
// The naive version zeroes the player only in the plan's squad and watches the
// gain collapse. That is wrong whenever he is someone you already own: if he is
// unavailable he is unavailable in the do-nothing squad too, so the *margin*
// between them barely moves and the plan is no worse than it was. Ranking on the
// naive number would flag your best existing player every time, which is true
// and useless — you were never going to learn something that changes it.
//
// Zeroing him on both sides isolates the question that matters: does this plan
// still beat doing nothing if he does not play. That is only really at risk for
// the player being *bought*, which is exactly the failure mode of acting early.
// It is judged on the same objective the plan was chosen under — s.value, not
// XIValue — so that a plan bought for a chip week is stress-tested against the
// chip week. Reading the plain eleven here would report the dependency of a plan
// nobody made.
func weakestLink(s SquadState, planned []PlayerMetrics) (PlayerMetrics, float64) {
	current := s.Players
	base := s.value(current)
	planGain := s.value(planned) - base

	worst := planGain
	var who PlayerMetrics
	for _, p := range planned {
		out := s.value(zeroOut(planned, p.ID)) - s.value(zeroOut(current, p.ID))
		if who.ID == 0 || out < worst {
			worst, who = out, p
		}
	}
	return who, worst
}

// zeroOut returns the squad with one player scoring nothing, which is what an
// unavailable player is worth. He stays in the fifteen because he is still
// occupying the slot — that is the point.
func zeroOut(squad []PlayerMetrics, id int) []PlayerMetrics {
	out := make([]PlayerMetrics, len(squad))
	copy(out, squad)
	for i := range out {
		if out[i].ID == id {
			out[i].Score = 0
		}
	}
	return out
}
