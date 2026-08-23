package config

import "armband/internal/analysis"

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
	//
	// Re-measured 2026-08-17 as a flat ladder — 1.0/1.5/3.0/4.0 against 2.0, 36
	// cells on the six-season grid, POLICY: +8.8/+6.5/-23.0/-10.5 a season against
	// per-arm thresholds of 15 to 34, Holm >= 0.56 on the clustered p, and no
	// shape (non-monotone, 3.0 worse than 4.0). So "not resolvable on three
	// seasons" is not a grid-width problem — it does not resolve on six either.
	// ⚠️ 0.0 was NOT a rung, so the "beats zero" comparison above is still the
	// only record of that comparator and has not been re-run.
	// ⚠️ A ladder spanning 2.0 crosses a kink: a free single's bar is
	// max(MinGainForTransfer, FreeTransferValue/H), and at the shipped horizon 5
	// this constant's 2.0 IS MinGainForTransfer's 0.4 exactly, so below 2.0 the
	// singles channel cannot move. See
	// stats/findings/2026-08-17-free-transfer-value-ladder.md.
	FreeTransferValue float64 `json:"free_transfer_value"`

	// EarlyFloor is the scheduled gate floor the "react faster early"
	// measurement runs: the charge and gain bar applied up to and including
	// UntilGameweek, with the shipped constants after. The zero value is off,
	// which is the whole backfill — a config file without the key gets the
	// shipped behaviour, and an empty schedule is a statement the way the
	// campaign maps are. See stats/findings/2026-08-18-gate-floor-2x2.md for
	// the measurement that licenses the schedule shape.
	EarlyFloor EarlyFloor `json:"early_floor"`

	// BankTransfersLookahead lets the weekly decision decline a move because a
	// larger package is affordable next week, when one more free transfer is in
	// hand.
	//
	// # What it buys, and why it ships on
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

	// AnticipateChips lets the weekly transfer decision know a chip is coming,
	// via `SimConfig.AnticipateChips`/`anticipate` in internal/backtest: a
	// planned wildcard means the held squad only has to serve until it is
	// replaced, so a short fixture run stops being a bet the policy has to
	// unwind through the gate, and a planned free hit's week is excluded from
	// scoring entirely. It reaches only the horizon and the free-hit week; it
	// cannot carry a bench boost or triple captain (see PrepareForChips), and
	// it does nothing unless a chip is actually planned in `chip_plan`.
	//
	// This is the same on/off pair `SimConfig.AnticipateChips` and
	// `SimConfig.AnticipateGate` are in the replay, collapsed to one setting
	// here on purpose. The replay's own comment on `AnticipateGate` records
	// that the two are opposite levers — scoring a move on a shortened horizon
	// while still charging it over the full one over-credits near-term fixture
	// spikes by construction — and that a mismatched pair was measured at -17
	// points a season. The replay exposes both because a sweep needs to
	// isolate them one at a time; a config a manager edits by hand has no such
	// need and every reason not to reproduce a measured loss, so this field
	// drives both `AnticipateChips` and `AnticipateGate` together rather than
	// exposing a second knob that could be left off by omission.
	//
	// Off by default: it was measured coherent in isolation (about +2.5 a
	// season) and as part of a larger, unresolved corner alongside a
	// calendar-anchored chip plan and banking lookahead (+73/+97 a season,
	// not attributable to this lever alone) — real mechanism, unresolved
	// points, the same standing this project holds PrepareForChips to.
	AnticipateChips bool `json:"anticipate_chips"`

	// MaxHitsPerWeek caps points deliberately spent on extra transfers.
	// Zero means never take a hit.
	MaxHitsPerWeek int `json:"max_hits_per_week"`

	// HitCeiling is the largest value MaxHitsPerWeek can MEAN. Zero takes
	// analysis.DefaultHitCeiling, which is 1 and is what ships.
	//
	// # Why there are two knobs and not one
	//
	// `analysis.MoveLimit` clamped the hit allowance to 1 unconditionally, so
	// setting `max_hits_per_week: 2` here changed nothing at all and said nothing
	// about it. The clamp has an argument behind it — see
	// `analysis.DefaultHitCeiling` — but an unreachable clamp makes the routine
	// two-hit week unexpressible, and this record's rule is that a knob which
	// silently means something else is worse than one that is absent.
	//
	// Raise this to raise the ceiling; raise `max_hits_per_week` to spend into it.
	// A ceiling below `max_hits_per_week` wins, which is what a ceiling is.
	HitCeiling int `json:"max_hits_ceiling"`

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

// EarlyFloor is the scheduled gate floor the "react faster early" measurement
// runs: FreeTransferValue and MinGainForTransfer applied up to and including
// UntilGameweek, the shipped constants after. A zero UntilGameweek is off — an
// explicit statement, so Load probes for the KEY rather than the value when
// backfilling: a config file without the key gets the shipped schedule, one
// that writes a zero schedule keeps it.
type EarlyFloor struct {
	FreeTransferValue  float64 `json:"free_transfer_value"`
	MinGainForTransfer float64 `json:"min_gain_for_free_transfer"`
	UntilGameweek      int     `json:"until_gameweek"`
}

// EffectiveFloor is the week's charge and gain bar, honouring the scheduled
// early floor. The shipped schedule — {1.0, 0.2} through GW8, measured by
// stats/findings/2026-08-18-scheduled-floor.md and shipped on the user's
// ruling — applies only to weeks at or before UntilGameweek; every other week
// reads the flat constants.
func (r ReviewPolicy) EffectiveFloor(gw int) (charge, minGain float64) {
	charge, minGain = r.FreeTransferValue, r.MinGainForTransfer
	if r.EarlyFloor.UntilGameweek > 0 && gw > 0 && gw <= r.EarlyFloor.UntilGameweek {
		charge = r.EarlyFloor.FreeTransferValue
		minGain = r.EarlyFloor.MinGainForTransfer
	}
	return
}

func DefaultReviewPolicy() ReviewPolicy {
	return ReviewPolicy{
		MinGainForTransfer: 0.4,
		MinGainForHit:      3.0,
		FreeTransferValue:  2.0,
		BankUpTo:           5,
		LeadHours:          6,
		MaxHitsPerWeek:     1,
		HitCeiling:         analysis.DefaultHitCeiling,
		AlwaysActOnInjury:  true,
		// Shipped 2026-08-18 on the user's ruling, after the measurements: the
		// banking lookahead that was measured inert as a solo lever is part of
		// the override-mode corner that resolves (+73/+97 a season), and the
		// early floor ships on the user's info-density reading at its measured
		// schedule — +6.7/+4.0 at the live entries, unresolved, accepted on
		// mechanism rather than resolution. See
		// stats/findings/2026-08-18-scheduled-floor.md.
		BankTransfersLookahead: true,
		EarlyFloor: EarlyFloor{
			FreeTransferValue:  1.0,
			MinGainForTransfer: 0.2,
			UntilGameweek:      8,
		},
		Rules: []string{
			"Do nothing is a valid and usually underrated answer. Only recommend a move when the case is affirmative.",
			"Never take a hit to chase last week's points. A player who blanked is not thereby a sell.",
			"Prefer fixing a problem (injury, lost starting place, terrible run) over chasing an upgrade.",
			"If a wildcard is planned within two gameweeks, do not spend transfers on problems the wildcard will fix anyway.",
			"Check team news before finalising: the model cannot see press conferences, and FPL's own news field lags them.",
		},
	}
}
