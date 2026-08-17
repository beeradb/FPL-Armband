package config

// ReviewPolicy is the standing brief for the weekly review. You set the
// thresholds and the rules; the agent decides within them.
//
// The point of writing these down is that they bind the agent in the weeks
// when a move feels tempting. "Only transfer for a real gain" is easy to agree
// with in the abstract and easy to abandon after a bad gameweek.
type ReviewPolicy struct {
	// MinGainForTransfer is the modelled points-per-gameweek improvement a
	// straight swap must deliver to be worth a free transfer. Below this,
	// bank the transfer instead.
	//
	// # It reads 0.4, it is a no-op at that value, and raising it did not help
	//
	// The gate is `gain >= MinGainForTransfer && gain x horizon + money >=
	// FreeTransferValue`. At the shipped horizon of 5 and a free-transfer charge
	// of 2, the second clause already demands gain >= 0.4 — so the first clause
	// bound nothing. Sweeping it confirmed that exactly: 0.0 and 0.4 produce
	// byte-identical seasons, the same 222 transfers and the same points, over
	// four seasons and three entry points. Two constants were expressing one
	// threshold and one of them was decorative.
	//
	// It was briefly raised to 0.7 and is **retracted**. The reasoning still
	// looks sound: TestDiagTransferError finds the model over-rates the player
	// it *buys* by about 0.53 pts/gw while the player it sells is well
	// calibrated, and correcting a flat buy-side bias is arithmetically the same
	// as demanding that much more gain, which puts the honest threshold near
	// 0.4 + 0.53. A totals sweep on three start points appeared to confirm it,
	// 18048 at 0.70 against 17911.
	//
	// Re-measured properly — six start points, 24 matched cells, paired
	// differences — the direction **reverses**: 0.4 reads +0.535 pts/gw over 0.7
	// (t = +1.63), where the old method had 0.7 ahead by 137 points. Neither
	// clears |t| = 2, so the honest statement is that this constant does not
	// resolve on points at any value, and a setting changed on evidence that has
	// since inverted goes back.
	//
	// Two candidate explanations for the reversal and this cannot separate them:
	// the earlier run had half the cells, and BlankRunPenalty shipped in between,
	// which moves the same players. That is the standing rule — a sweep is only
	// valid at the setting of every knob it shares a population with.
	//
	// So it sits at 0.4 knowing it is a **no-op**: the gate's second clause,
	// gain x horizon + money >= FreeTransferValue, already demands 0.4 at horizon
	// 5 and charge 2, and 0.0 and 0.4 return byte-identical seasons. The real
	// threshold lives in free_transfer_value. Do not raise this one again without
	// re-measuring that at the same time.
	MinGainForTransfer float64 `json:"min_gain_for_free_transfer"`

	// MinGainForHit is the net modelled gain, across the whole horizon and
	// after the 4-point cost, required to justify a hit.
	MinGainForHit float64 `json:"min_net_gain_for_minus_4"`

	// BankUpTo is how many free transfers may accumulate.
	//
	// FPL raised this from 2 to 5 for the 2024-25 season. A replay of an
	// earlier season must use the rule that was in force — see
	// backtest.BankLimitFor.
	//
	// Old doc, kept because it still describes the intent: how many free
	// transfers to accumulate before spending,
	// absent an injury or a genuinely large upgrade. FPL caps banking at 5.
	BankUpTo int `json:"bank_transfers_up_to"`

	// FreeTransferValue is what a free transfer is charged before it will be
	// spent, in points across the horizon.
	//
	// A transfer with no deduction is not a transfer with no cost. Judged only
	// on a small per-gameweek threshold, the replay churned: it sold Palmer and
	// bought him back two gameweeks later, cycled the same three forwards
	// across four transfers, and round-tripped Saliba — twelve such reversals
	// across three seasons.
	//
	// Charging for the transfer is really a confidence threshold rather than a
	// true opportunity cost. A banked transfer that is about to expire is
	// genuinely free in resource terms, and exempting it makes no difference to
	// the replay at all; what the charge actually filters is moves too small to
	// be distinguishable from noise.
	//
	// Four — the price of buying another — is too high: it cuts the replay from
	// 73 transfers to 39 and scores below charging nothing. Anything from 1 to
	// 2.5 beats zero on all three seasons; 2 is the middle of that range. The
	// exact value is not resolvable on three seasons, so treat it as a knob
	// rather than a calibrated constant.
	FreeTransferValue float64 `json:"free_transfer_value"`

	// BankTransfersLookahead lets the weekly decision decline a move because a
	// larger package is affordable next week, when one more free transfer is in
	// hand.
	//
	// # What it buys, and why it is off
	//
	// A premium upgrade usually needs more than one move: the money is locked in
	// a player of a different position, so buying him means selling a forward
	// *and* funding the gap. The paired search can express that and almost never
	// gets the chance, because the weekly decision is greedy — it spends a free
	// transfer the moment any move clears the gate. Traced cost: in 2025-26 the
	// model rated Haaland above Salah from GW7 and could not buy him until GW13,
	// because nothing ever compared "spend one now" against "bank two and buy the
	// premium".
	//
	// It is also how a chip gets prepared for. With a bench boost or triple
	// captain planned inside the horizon, the *later* arm of the comparison
	// carries that chip's credit one week nearer, so waiting is worth more
	// exactly when the squad is being assembled for a chip.
	//
	// **Ships off**, and that is a statement about evidence rather than about the
	// mechanism. The recorded verdict is that the policy never banks a transfer
	// and that banking is not the fix — reached, it turns out, with nothing
	// counting whether the rule ever fired, which is why the replay now records
	// the funnel from decision weeks down to banked weeks. Until a sweep carries
	// those columns there is no measured case for turning it on, and shipping a
	// default on an unmeasured claim is what this project has a rule against.
	// Turning it on is supported, explained in the transfer output, and yours to
	// judge.
	BankTransfersLookahead bool `json:"bank_transfers_lookahead"`

	// PrepareForChips lets the transfer search value a planned bench boost or
	// triple captain that falls inside its horizon.
	//
	// Without it a squad is assembled for the average week and the chip is played
	// on whatever fifteen happens to be owned. With it, the bench a boost will
	// pay for and the armband a triple captain will treble are worth something to
	// the search *before* the chip week arrives — which is the only channel by
	// which a chip can be prepared for at all.
	//
	// The credit is amortised over the same horizon the gate is about to multiply
	// it back by, so it prices what the chip actually pays once rather than once
	// per gameweek. A wildcard between now and the chip closes the window: that
	// squad does not survive to play it.
	//
	// Off by default for the reason above it: the mechanism is real and the
	// points question is unresolved. It does nothing at all unless a chip is
	// actually planned in `chip_plan`.
	PrepareForChips bool `json:"prepare_squad_for_chips"`

	// MaxHitsPerWeek caps points deliberately spent on extra transfers.
	// Zero means never take a hit.
	MaxHitsPerWeek int `json:"max_hits_per_week"`

	// AlwaysActOnInjury forces a move when a starter is ruled out, regardless
	// of the gain thresholds — an unavailable player scores zero, which no
	// threshold can outweigh.
	AlwaysActOnInjury bool `json:"always_act_on_ruled_out_starter"`

	// Rules are free-text policies applied verbatim during the review.
	Rules []string `json:"rules"`

	// LeadHours is how long before a deadline a scheduled run should fire.
	//
	// The point of running late is team news: press conferences land one to two
	// days out and confirmed line-ups only at the deadline itself. Too early and
	// the run reasons from stale availability; too late and there is no time to
	// act on it. Six hours is a compromise that clears most Friday pressers
	// while leaving the evening to decide.
	LeadHours float64 `json:"scheduled_run_lead_hours"`
}

func DefaultReviewPolicy() ReviewPolicy {
	return ReviewPolicy{
		MinGainForTransfer: 0.4,
		MinGainForHit:      3.0,
		FreeTransferValue:  2.0,
		BankUpTo:           5,
		LeadHours:          6,
		MaxHitsPerWeek:     1,
		AlwaysActOnInjury:  true,
		Rules: []string{
			"Do nothing is a valid and usually underrated answer. Only recommend a move when the case is affirmative.",
			"Never take a hit to chase last week's points. A player who blanked is not thereby a sell.",
			"Prefer fixing a problem (injury, lost starting place, terrible run) over chasing an upgrade.",
			"If a wildcard is planned within two gameweeks, do not spend transfers on problems the wildcard will fix anyway.",
			"Check team news before finalising: the model cannot see press conferences, and FPL's own news field lags them.",
		},
	}
}
