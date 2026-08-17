package analysis

// Banking a free transfer: declining this week's move because a larger package
// next week is worth more.
//
// # Why the rule lives here and not in the replay
//
// It was written in `internal/backtest`, where it could only ever be a sweep
// switch — the live commands never saw it, so a user could not act on it and the
// agent could not explain it. The decision itself is pure arithmetic over a squad
// and a candidate pool, which is what this package is for, so the rule, the
// package valuation and the chip credit's window all sit here and the replay
// calls them. That is the standing rule against one quantity with two
// implementations, applied before the second one gets written rather than after.
//
// # What it does and does not assume
//
// It does not predict the future. It asks something answerable entirely from
// today's board: **what is the best package I could afford with one more
// transfer, and is it worth more than the best I can afford now, even after
// losing a gameweek of it?**
//
// Waiting is charged honestly. Next week's package earns over a horizon one
// shorter, because a week of the gain is gone, while this week's move is credited
// in full. So banking has to win on the *size* of what it unlocks rather than on
// optimism about timing. Both arms are priced on today's board, which is the
// approximation — next week's will differ — and it applies to both sides equally.

// TransferPackage is one candidate set of moves, reduced to what the banking
// comparison needs: what it gains per gameweek and how many transfers it spends.
type TransferPackage struct {
	Gain  float64
	Moves int
}

// PackageValue is what one package is worth over a horizon, after paying for the
// transfers it spends, or zero when it does not clear the gain floor.
//
// Zero is "do nothing", which is always available — so a package that fails the
// floor, or whose gain does not cover its own transfer charge, is worth exactly
// as much as declining it. That floor is what makes the two arms of the banking
// comparison commensurable: both are measured against doing nothing.
func PackageValue(p TransferPackage, horizon, freeCost, minGain float64) float64 {
	if p.Moves <= 0 || p.Gain < minGain {
		return 0
	}
	v := p.Gain*horizon - freeCost*float64(p.Moves)
	if v < 0 {
		return 0
	}
	return v
}

// BestPackageValue is the most valuable of several packages, floored at zero.
func BestPackageValue(packages []TransferPackage, horizon, freeCost, minGain float64) float64 {
	best := 0.0
	for _, p := range packages {
		if v := PackageValue(p, horizon, freeCost, minGain); v > best {
			best = v
		}
	}
	return best
}

// MoveLimit is how many transfers may be made in one gameweek: every free
// transfer plus one hit, unless capped.
//
// The "plus one" is a single hit, and the clamp is deliberate. Two hits in a week
// is an edge case — it needs a specific reason, usually an injury, and that is
// the judgement layer's job rather than something a scoring model should go
// looking for. Allowing it mostly widened the search space so the policy could
// find expensive ways to chase noise: on three replayed seasons the two-hit
// policy never won.
//
// Here rather than in the replay because the banking comparison is built on it —
// the whole question is what one more move would buy — and the live command has
// to reach the same answer the replay does.
func MoveLimit(free, maxHits, maxMoves int) int {
	if maxHits > 1 {
		maxHits = 1
	}
	limit := free + maxHits
	if maxMoves > 0 && maxMoves < limit {
		limit = maxMoves
	}
	return limit
}

// PreferWaiting is the banking comparison itself: is next week's larger
// allowance worth more than acting now?
//
// A strict `>`, so a tie acts. That is deliberate and is the conservative
// direction: waiting costs a gameweek of whatever was on the table, and a rule
// that banked on equal value would trade a certain week for an estimate.
//
// One line, and it is a function because it is the quantity two callers share.
// The replay and the live transfer command must not be able to disagree about
// which way a tie goes.
func PreferWaiting(nowValue, laterValue float64) bool { return laterValue > nowValue }

// BankGuard is why the banking rule declined without ever making its comparison.
type BankGuard int

const (
	// BankGuardNone means the rule reached its comparison.
	BankGuardNone BankGuard = iota
	// BankGuardCeiling means the allowance already sits at the bank limit, so
	// waiting buys no extra move and simply loses a week.
	BankGuardCeiling
	// BankGuardHorizon means there is at most one gameweek left to spend into.
	BankGuardHorizon
)

// BankAdvice is the banking decision and everything needed to explain it.
//
// # Why the reasons are returned rather than logged
//
// A policy that declines a transfer and says nothing is indistinguishable from a
// policy that found nothing worth doing, and those are opposite recommendations:
// one says "act next week with two transfers", the other says "your squad is
// fine". The replay does not care — it only needs the boolean — but a user does,
// and the agent cannot explain a number it was never given. So the decision
// carries its own reason, and `Explain` renders it.
type BankAdvice struct {
	// Bank is the decision: hold this week's transfer.
	Bank bool
	// Guard is the reason the comparison was never made, or BankGuardNone.
	Guard BankGuard
	// NowValue and LaterValue are the two arms, in points across their own
	// horizons. Both zero means nothing cleared the gain floor in either week,
	// which is a refusal by a rule that had nothing to weigh — a different fact
	// from one that weighed a real choice and preferred acting now.
	NowValue, LaterValue float64
	// Free and BankUpTo are the allowance the comparison ran with and the
	// ceiling it was measured against.
	Free, BankUpTo int
	// Horizon is the gameweeks the now-arm was valued over; the later arm is
	// valued over one fewer.
	Horizon float64
}

// Weighed reports whether the rule made a real comparison: past both guards, and
// with at least one arm worth something.
//
// This is the denominator a "how often did banking fire" rate belongs over.
// Without it a zero pools "the rule weighed a real choice and preferred acting
// now" with "there was nothing to weigh", and those license opposite
// conclusions.
func (a BankAdvice) Weighed() bool {
	return a.Guard == BankGuardNone && (a.NowValue > 0 || a.LaterValue > 0)
}

// BankGuardFor is the pair of refusals that need no package valuation.
//
// Split out so both callers reach them without pricing anything: valuing the two
// arms means two full transfer searches, and the whole point of a guard is to
// decline before paying for that. The replay calls this first and returns; the
// live path calls it through AdviseBank.
//
// ⚠️ **Horizon must already be clamped to the gameweeks that remain.** This
// cannot do it — it is handed a horizon rather than a gameweek — and the second
// guard is the one that stops a manager being advised at GW38 to hold a transfer
// into a gameweek that does not exist. The replay clamps in `effectiveHorizon`
// and the live path in `adviseBanking`.
func BankGuardFor(free, bankUpTo int, horizon float64) BankGuard {
	// Nothing to bank toward: the allowance is already at its ceiling, so waiting
	// buys no extra move and simply loses a week.
	if free >= bankUpTo {
		return BankGuardCeiling
	}
	// A week is only worth waiting through if there is more than one left.
	if horizon <= 1 {
		return BankGuardHorizon
	}
	return BankGuardNone
}

// AdviseBank runs the banking rule over two already-valued arms.
//
// The caller supplies the package values because the two callers enumerate
// packages differently — the replay prices them through a wallet that knows what
// every player was bought for, the live command through the ranked plans it is
// about to print — and forcing one enumeration on both would make the cheaper
// caller wrong rather than making them agree. What must not differ, and does not,
// is the rule applied to those values: the guards through BankGuardFor, the
// comparison through PreferWaiting, and what counts as having weighed anything
// through Weighed.
func AdviseBank(free, bankUpTo int, horizon, nowValue, laterValue float64) BankAdvice {
	a := BankAdvice{
		Free: free, BankUpTo: bankUpTo, Horizon: horizon,
		NowValue: nowValue, LaterValue: laterValue,
	}
	if a.Guard = BankGuardFor(free, bankUpTo, horizon); a.Guard != BankGuardNone {
		// The arms are zeroed rather than reported, because a guard refuses
		// *before* the comparison and reporting values it never consulted would
		// invite a reader to check the arithmetic of a decision made another way.
		a.NowValue, a.LaterValue = 0, 0
		return a
	}
	a.Bank = PreferWaiting(nowValue, laterValue)
	return a
}

// Explain is the decision in one sentence, for a user rather than a sweep.
//
// It never returns the empty string: "the rule did not fire" is a statement, and
// a blank line is how a declined move becomes indistinguishable from a policy
// that was never asked.
func (a BankAdvice) Explain() string {
	switch {
	case a.Bank:
		return "Bank this week's transfer: with one more in hand next week the best " +
			"package is worth more than anything available now, even after losing a " +
			"gameweek of it."
	case a.Guard == BankGuardCeiling:
		return "Not banking: the transfer allowance is already at its limit, so " +
			"waiting buys no extra move and only loses a gameweek."
	case a.Guard == BankGuardHorizon:
		return "Not banking: there is no run of gameweeks left to spend a banked " +
			"transfer into."
	case !a.Weighed():
		// Scoped to the banking comparison on purpose. It is NOT a claim that no
		// transfer is worth making: the printed recommendation comes from
		// BuildPlans, which applies neither the gain floor nor the transfer
		// charge, so a plan can legitimately be offered directly beneath this
		// line. What has nothing on either side is the *waiting* question.
		return "Not banking: neither this week's nor next week's best package " +
			"cleared the transfer charge, so the waiting comparison had nothing " +
			"to weigh on either side."
	default:
		return "Not banking: acting now is worth more than waiting for a larger " +
			"allowance next week."
	}
}

// ChipCreditAt prices a chip planned inside a decision's horizon, per gameweek.
//
// # It is amortised over the gate's horizon, and that is not a free choice
//
// The gate charges `gain x horizon`, so a gain expressed per gameweek is worth
// its horizon multiple by the time it is compared with a transfer's cost. A chip
// pays *once*: the bench scores in one week, the armband triples in one week. So
// the per-gameweek credit is the one-off premium divided by the same horizon the
// gate is about to multiply it back by, and the two cancel to what the chip
// actually pays. Reading a different horizon here would credit the chip a
// multiple of its own value.
//
// The window is closed at the near end and open at the far one: a chip in *this*
// gameweek still counts, because a transfer made now plays in it.
//
// # A wildcard between here and the chip ends the window
//
// The squad being valued does not survive a wildcard — it replaces all fifteen.
// So a credit that reached past one would spend this week's free transfers buying
// a bench for a fifteen that is about to be torn up, and the preparation it paid
// for would arrive in the rebuild for nothing. That is not hypothetical: it is
// the shape of the wildcard-into-boost sequence, which is the play the chip
// actually lives in.
//
// The free hit is deliberately *not* a barrier. It fields a temporary fifteen for
// one week and hands the permanent squad straight back, so a chip beyond it is
// still played by the squad being valued now.
//
// # Both sets, always
//
// It asks the schedule for the NEXT chip of each kind at or after `gw`, which
// with two sets is not the same as "the wildcard": reading a single field returns
// whichever set happens to hold it, so a second-half decision could be walled by
// a wildcard played in September. That is how a two-set plan silently reverts to
// single-set behaviour.
func ChipCreditAt(s ChipSchedule, prepBench, prepCaptain bool, gw int, horizon float64) ChipCredit {
	if horizon < 1 {
		horizon = 1
	}
	var cr ChipCredit
	wall := 39
	if wc := s.Next(SlotWildcard, gw); wc > 0 && wc < wall {
		wall = wc
	}
	inHorizon := func(week int) bool {
		return week >= gw && week < wall && float64(week) < float64(gw)+horizon
	}
	if bb := s.Next(SlotBenchBoost, gw); prepBench && bb > 0 && inHorizon(bb) {
		cr.Bench = 1 / horizon
	}
	if tc := s.Next(SlotTripleCaptain, gw); prepCaptain && tc > 0 && inHorizon(tc) {
		cr.Captain = 1 / horizon
	}
	return cr
}
