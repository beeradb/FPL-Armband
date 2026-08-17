package backtest

// The transfer gate: the one predicate that decides whether a proposed set of
// moves is made.
//
// # Why this is one function
//
// It used to be four expressions — the funded pair's acceptance in decide, the
// free single, the hit single, and the unified search's own — all computing the
// same shape of arithmetic from the same constants and none of them named. That is
// this package's most-repeated bug at its most dangerous: four expressions of one
// rule, three of which would eventually stop agreeing with the fourth, with
// nothing to notice. `fixtureSensitivePart` against `baseXP90` is the same failure
// and it was silently wrong for every defender from one commit onward.
//
// It also made AxisTransferGate impossible to build honestly. An oracle hooked
// into four places is four hooks, and one of them will drift.
//
// # What was preserved exactly, including two asymmetries
//
// This is a refactor and not a change, so two oddities of the shipped rule are
// kept rather than tidied:
//
//   - **The funded pair must beat its alternative strictly; a single swap need
//     only tie zero.** `>` against `>=`. It matters only when a package is worth
//     precisely what declining it is worth, which is rare and not never.
//   - **The hit branch has no minimum-gain bar at all.** A free single must clear
//     MinGain; a hit is gated on MinGainHit and nothing else. Squaring that up
//     would be a behaviour change with a measurement attached, not a refactor, so
//     the bar is expressed as noGainBar and the asymmetry stays visible.
//
// # What is deliberately NOT routed through here
//
// `analysis.BestPackage`, which prices what banking a transfer would buy over the
// candidates `transferPackages` enumerated. It is a *valuation* rather than an
// accept — it answers "what is the best package worth" and never says yes or no —
// and folding it in would make the oracle able to change what shouldBank
// compares, which is a second axis. It ships off in any case.

import "math"

// noGainBar is the gain bar for a proposal the shipped rule does not gate on gain.
//
// Negative infinity rather than zero, because zero is a real bar: it would refuse
// a package with a negative modelled gain, and the shipped hit branch does not.
// Expressing "no bar" as a value the comparison cannot fail is what keeps this a
// refactor.
var noGainBar = math.Inf(-1)

// transferProposal is one thing a transfer search has proposed, described in the
// terms the gate reads and no others.
//
// A *package*, not a move: `decide` accepts a funded pair as a single decision and
// zeroes the funding legs' reported gains precisely because none of them stands up
// alone. So the unit the gate judges is a set of moves, and the oracle's unit has
// to be the same one.
type transferProposal struct {
	// Moves is what would change hands. Its length is the number of transfers
	// spent, which is what the free-transfer charge is levied per.
	Moves []Move
	// Gain is the package's modelled gain in points per gameweek, measured as what
	// it does to the *eleven* rather than to the players. See analysis.XIValue.
	Gain float64
	// Money is what the money the package frees is worth in points, already
	// discounted for how many gameweeks remain to spend it in. Zero when
	// budgetWeight is off, which is what ships.
	Money float64
	// Hits is how many of the moves are paid for with a -4.
	Hits int
	// Surcharge is any extra charge for doing several things at once, already
	// totalled. Only the unified search levies one, and it ships at zero.
	Surcharge float64
	// Alternative is what the gate would otherwise do, in the same units as
	// value(). The alternative to a funded pair is never "do nothing": it is to
	// spend the free transfer on the best single move and keep the four points.
	Alternative float64
	// Strict makes the package beat Alternative rather than tie it.
	Strict bool
	// GainBar is the minimum modelled gain, or noGainBar where the shipped rule
	// applies none.
	GainBar float64
	// Horizon is how many gameweeks the package has to repay itself, already
	// shortened for the end of the season.
	Horizon float64
	// FreeCost is what spending a free transfer is charged. A confidence
	// threshold rather than an opportunity cost — see SimConfig.FreeCost.
	FreeCost float64
	// GW is the gameweek the decision is taken for.
	GW int
}

// value is what the package is worth across the horizon, after paying for every
// move it spends.
func (p transferProposal) value() float64 {
	n := float64(len(p.Moves))
	h := float64(p.Hits)
	return p.Gain*p.Horizon + p.Money -
		HitCost*h - p.FreeCost*(n-h) - p.Surcharge
}

// withBar returns the proposal with a minimum-gain bar applied.
func (p transferProposal) withBar(bar float64) transferProposal {
	p.GainBar = bar
	return p
}

// asHit returns the proposal paid for with a -4 rather than a free transfer.
//
// It carries **no** gain bar, which is the shipped asymmetry documented at the
// top of this file: a free single must clear MinGain and a hit is gated on
// MinGainHit alone. Written as a named constructor so that asymmetry is visible
// at both call sites instead of being an absent field somebody assumes is a
// mistake.
func (p transferProposal) asHit() transferProposal {
	p.Hits = 1
	p.GainBar = noGainBar
	return p
}

// shippedAccept is the rule as it has always been, in one place.
func (p transferProposal) shippedAccept(cfg SimConfig) bool {
	if p.Gain < p.GainBar {
		return false
	}
	v := p.value()
	if p.Hits > 0 {
		// A hit has to clear a higher bar than merely being worth more than the
		// alternative, because four points is a certain cost against an uncertain
		// gain.
		if p.Strict && v <= p.Alternative {
			return false
		}
		return v-p.Alternative >= cfg.MinGainHit
	}
	if p.Strict {
		return v > p.Alternative
	}
	return v >= p.Alternative
}

// acceptTransfer is THE gate. Every accept expression in decide and in the
// unified search routes through it, which is what makes AxisTransferGate one hook
// rather than four.
//
// It is a two-line wrapper around gateDecision, and the split buys exactly one
// thing: the per-package log below is written **once**, at the single point every
// accept passes through, rather than at each of the switch's returns. Adding it at
// the returns would be a seventh expression of "what did the gate just decide" and
// the class of bug this file exists to end.
func acceptTransfer(cfg SimConfig, s *Season, p transferProposal) bool {
	ok := gateDecision(cfg, s, p)
	if cfg.gateLog != nil && s != nil {
		cfg.gateLog(describeGatePackage(s, p, ok))
	}
	return ok
}

// gateDecision is the rule itself: the shipped predicate, or the one oracle the
// arm was granted.
//
// The season is passed only so the oracle can read what actually happened. The
// shipped path ignores it, and must: a predicate that could see results would be
// the point-in-time leak this package guards hardest.
func gateDecision(cfg SimConfig, s *Season, p transferProposal) bool {
	switch cfg.Oracles.Decision {
	case AxisTransferGate:
		var oracle gateOracle = perfectGate
		return oracle(s, p)
	case AxisTransferGateXPoints:
		var oracle gateOracle = perfectGateXPoints
		return oracle(s, p)
	case AxisTransferGateResidual:
		var oracle gateOracle = perfectGateResidual
		return oracle(s, p)
	case AxisTransferGateAntiResidual:
		var oracle gateOracle = perfectGateAntiResidual
		return oracle(s, p)
	case AxisTransferGateAcceptAll:
		var oracle gateOracle = gateAcceptEverything
		return oracle(s, p)
	}
	return p.shippedAccept(cfg)
}

// gatePackage is one package offered to the gate, described in the two quantities
// the residual family's criteria are built from and nothing else.
//
// It exists because the arithmetic that reads this family's contrast needs three
// numbers off the *offered* stream — the accept mass p, the mean underlying gain
// mu, and the covariance between the underlying gain and the sign of the residual
// — and none of them is recoverable from a transfer count. The gate diagnostic has
// admitted owing this since the first re-run; counts were reported instead, and
// equal counts are not the same packages.
type gatePackage struct {
	// GW is the gameweek the decision was taken for, and Weeks the window the
	// criteria judged over, already clamped exactly as they clamp it.
	GW, Weeks int
	// Hits is how many of the moves are paid for with a -4.
	Hits int
	// DX is the package's realised UNDERLYING gain over the window: expected
	// points from realised underlying, in minus out. What perfectGateXPoints
	// judges, before its hit charge.
	DX float64
	// DR is the package's CONVERSION RESIDUAL gain over the same window, realised
	// minus expected. What perfectGateResidual judges, before its hit charge, and
	// what perfectGateAntiResidual judges the negation of.
	DR float64
	// Accepted is what the arm's own gate answered. Which criterion that was is
	// the arm's business; the log is written on every arm alike.
	Accepted bool
}

// describeGatePackage measures one offered package on both channels.
//
// It re-derives the window from p.Horizon the same way the criteria do rather than
// being handed it, which is the convention this file already keeps for its three
// near-copies: the window is identical across all of them **by construction**,
// because every one of them reads the same p.Horizon. It is an observer and never
// a decision — nothing branches on what it returns.
//
// TestTheGateLogAgreesWithTheCriteriaItDescribes pins DX and DR against the
// predicates, so a drift here shows up as a failed unit test rather than as a
// plausible offset in a contrast's null.
func describeGatePackage(s *Season, p transferProposal, accepted bool) gatePackage {
	weeks := int(p.Horizon)
	if weeks < 1 {
		weeks = 1
	}
	g := gatePackage{GW: p.GW, Weeks: weeks, Hits: p.Hits, Accepted: accepted}
	for _, mv := range p.Moves {
		inX := xPointsOver(s, mv.InID, p.GW, weeks)
		outX := xPointsOver(s, mv.OutID, p.GW, weeks)
		g.DX += inX - outX
		g.DR += (float64(pointsOver(s, mv.InID, p.GW, weeks)) - inX) -
			(float64(pointsOver(s, mv.OutID, p.GW, weeks)) - outX)
	}
	return g
}

// perfectGateAntiResidual is perfectGateResidual with the criterion's sign flipped
// and nothing else: accept iff −ΔR − 4h > 0.
//
// # What it is for
//
// The residual arm reads NEGATIVE on `policy_xpoints`, and the question that
// leaves open is whether that sign carries information or arithmetic. Two arms
// whose criteria are exact negations of each other share their whole common veto
// cost, so their contrast cancels it and leaves the antisymmetric part — which is
// the only part that could be information about ΔX.
//
// # The sign flips BEFORE the charge, and that is the whole of the construction
//
// `net = -net` sits above the hit subtraction, so the criterion is `−ΔR − 4h` and
// **not** `−(ΔR − 4h)`. A hit still costs four real points on an arm still scored
// on realised points, exactly as it does on all three siblings; flipping the charge
// with the criterion would make this arm *paid* to take hits and would be two
// changes in one arm. The consequence is worth stating because it is the reason the
// two accept sets do not tile the stream: `{ΔR > 4h}` and `{ΔR < −4h}` leave a dead
// band of width 8h, which is **zero** for a free package and 8 for a hit.
//
// # ⚠️ On realised points this is a NEGATIVE control by construction
//
// The exact mirror of what perfectGateResidual documents. `Points = X + R`
// identically, and this accepts on the sign of `−R`, so `E[X+R | R<0] < E[X]` with
// no decision quality anywhere in the mechanism. Its realised-points level is
// guaranteed negative and is a **liveness check and nothing else** — a level that
// is not clearly negative means the arm did not wire or the sign did not flip. The
// contrast against the residual arm on realised points is doubly constructed and
// must never be quoted at all.
//
// # ⚠️ It is ANTI-informative, not X-uninformative
//
// `−ΔR` is exactly as informative about ΔX as `ΔR` is, so this arm on its own does
// not measure what an X-uninformative criterion would read. Only the pair does, and
// only against an accept-everything reference — see gateAcceptEverything, which is
// what identifies the contrast's null.
func perfectGateAntiResidual(s *Season, p transferProposal) bool {
	weeks := int(p.Horizon)
	if weeks < 1 {
		weeks = 1
	}
	net := 0.0
	for _, mv := range p.Moves {
		in := float64(pointsOver(s, mv.InID, p.GW, weeks)) -
			xPointsOver(s, mv.InID, p.GW, weeks)
		out := float64(pointsOver(s, mv.OutID, p.GW, weeks)) -
			xPointsOver(s, mv.OutID, p.GW, weeks)
		net += in - out
	}
	// The one line that differs from perfectGateResidual, and it is deliberately
	// above the charge rather than folded into the return.
	net = -net
	net -= HitCost * float64(p.Hits)
	return net > 0
}

// gateAcceptEverything takes every package the search proposes. It is the no-gate
// policy, and it is not a decoration on the antisymmetric pair — it is what makes
// that pair's contrast answer its own question.
//
// # Why the contrast needs it
//
// The residual arm accepts `{ΔR > 4h}` and the anti arm `{ΔR < −4h}`. The dead band
// between them has **zero width whenever h = 0**, which is the large majority of
// offered packages, so for most of the stream the two arms *partition* it: the anti
// arm takes precisely what the residual arm refuses. Writing the effect on
// accumulated xPoints per offered package, with μ the mean underlying gain, p the
// residual arm's accept mass, T the accept-everything value of the whole stream and
// C the veto cost both arms share against the baseline:
//
//	ANTI − RES = −cov(ΔX, sign ΔR) + μ·(1 − 2p)
//
// The design wants the first term. The second vanishes only if the accept masses
// are equal or the mean gain is zero, and **neither is known** — the search proposes
// what it rates highest, so μ > 0 is the expectation, and realised-minus-expected
// differences are right-skewed, so p < ½ is the expectation. So the contrast's null
// is `T·(1 − 2p)` and **not zero**, and testing against zero would let mass
// asymmetry alone manufacture the signature that is supposed to mean ΔR carries
// information.
//
// This arm identifies both nuisance quantities in the run's own units, with no
// algebra and no unit conversion:
//
//	ACCEPTALL level = C + T
//	ANTI + RES     ≈ 2C + T,  so  C = (ANTI + RES) − ACCEPTALL  and  T = ACCEPTALL − C
//	p̂              = the residual criterion's PACKAGE accept mass on the offered
//	                 stream, from the per-package log. ⚠️ moves(RES)/moves(ACCEPTALL)
//	                 is a move-weighted proxy for it and a different number: a funded
//	                 pair is one package and several moves, and `decide`'s singles
//	                 loop returns on its first rejection, so a refusing arm forfeits
//	                 the rest of the week's moves for a reason unrelated to its mass.
//
// # It bounds the family from the other side
//
// Every gate oracle in this file bounds the constant family from ABOVE, by being
// right every time. This bounds it from BELOW, by never refusing anything, and that
// is a policy nothing in this project has measured. The structural half of the rule
// still binds — `decide` checks the week's allowance and the one-hit limit outside
// the gate — so this is "no VALUE bar", not "unlimited transfers".
//
// # ⚠️ It pays a hit charge no gated arm pays, and T̂ must be corrected for it
//
// Found by code review before the first reading rather than by a run. `moveLimit`
// bounds a week at `free + 1` moves with `MaxHitsPerWeek` 1, and an arm that refuses
// nothing exhausts that bound every week — so `free` ends every week at zero and
// starts the next at one, and either the funded pair fires needing one hit or the
// singles loop runs twice with the second paid for. **This arm takes a −4 in nearly
// every gameweek**, where the gated arms bank transfers because `decide` returns on
// a rejection.
//
// Two consequences. Its LEVEL is `C + T` minus a hit charge of its own, so a `T̂`
// read off it is biased low and the null built from it carries that bias; the
// correction is the hit channel, `4 × hits / weeks`, which every sweep already
// collects per cell. And *"the dead band has zero width whenever h = 0, which is the
// large majority of packages"* is a statement about the shipped stream and about the
// two gated arms — it is **false by construction on this one**, most of whose
// offered packages carry a hit. Read the per-package log's `free` column before
// using `T`.
//
// The season is ignored, which is the one honest thing to say about it: this is the
// only member of the family that is not an oracle at all. It is granted hindsight it
// declines to read, and it is filed as a decision axis because it replaces the same
// predicate through the same hook.
func gateAcceptEverything(s *Season, p transferProposal) bool { return true }

// perfectGateResidual is perfectGate with the criterion cut down to the
// CONVERSION RESIDUAL — realised points minus expected points from realised
// underlying, and nothing else.
//
// # Why the third near-copy and not a parameterisation
//
// The same argument perfectGateXPoints makes below, and it gets stronger with a
// third arm rather than weaker. The three differ on one expression each; a shared
// helper taking a scoring function would put the hit charge, the window clamp and
// the package loop one indirection away from all three, and the risk this file
// guards is a second expression of the DECISION WINDOW. The window is identical in
// all three by construction, because all three read the same p.Horizon.
//
// # What it is for, and what it cannot be read as
//
// analysis.XPoints is defined as Points minus XPointsResidual, so realised =
// xPoints + residual exactly, per player-gameweek. This arm and perfectGateXPoints
// therefore partition what perfectGate knows: one sees only the chances a decision
// bought, the other only whether they went in.
//
// ⚠️ **On realised points this arm is a POSITIVE CONTROL, not a measurement.** All
// three arms are scored on realised points, and an oracle accepting on the sign of
// an additive component of the metric it is scored on raises that metric by
// construction. A gain here is the expected outcome whatever is true of the gate,
// so it discriminates nothing; what discriminates is `policy_xpoints`, and a
// *small* figure on realised points is a wiring fault rather than a finding. The
// full argument is on AxisTransferGateResidual.
//
// ⚠️ The two halves' GAINS do not sum to the whole's, even though their criteria
// do. A gate is a threshold on a sum rather than a sum; the arms hold different
// squads from week one; and each component gate charges a full four points for a
// hit the composite charges once. No ratio of this arm to perfectGate is "the
// share of the points arm that is luck".
//
// ⚠️ Hits are charged at their real four points, the same as both siblings. There
// is a real argument the other way — a hit is deterministic, so its own conversion
// residual is exactly zero and a residual-only criterion arguably should not see
// it. It is not taken, for two reasons. The arm is SCORED on realised points, where
// a hit costs four of them, so an arm blind to the charge would take hits it loses
// on for reasons that have nothing to do with conversion — which would understate
// this arm and flatter the conclusion that the points arm is decision quality. And
// changing the charge as well as the criterion would be two changes in one arm.
func perfectGateResidual(s *Season, p transferProposal) bool {
	weeks := int(p.Horizon)
	if weeks < 1 {
		weeks = 1
	}
	net := 0.0
	for _, mv := range p.Moves {
		in := float64(pointsOver(s, mv.InID, p.GW, weeks)) -
			xPointsOver(s, mv.InID, p.GW, weeks)
		out := float64(pointsOver(s, mv.OutID, p.GW, weeks)) -
			xPointsOver(s, mv.OutID, p.GW, weeks)
		net += in - out
	}
	net -= HitCost * float64(p.Hits)
	return net > 0
}

// perfectGateXPoints is perfectGate with the criterion swapped, and nothing else.
//
// Deliberately a near-copy of perfectGate rather than a parameterisation of it: the
// two differ on one expression, and a shared helper taking a scoring function would
// put the hit charge, the window clamp and the package loop one indirection away
// from both. The risk this file guards is a second expression of the DECISION
// WINDOW, and the window is identical here by construction because it is read from
// the same p.Horizon.
//
// ⚠️ Hits are charged at their real four points, on the same scale, because a hit
// costs four actual points whatever the gate is reasoning about. Converting the
// charge to "expected points" would be a second, unmeasured change.
func perfectGateXPoints(s *Season, p transferProposal) bool {
	weeks := int(p.Horizon)
	if weeks < 1 {
		weeks = 1
	}
	net := 0.0
	for _, mv := range p.Moves {
		net += xPointsOver(s, mv.InID, p.GW, weeks) -
			xPointsOver(s, mv.OutID, p.GW, weeks)
	}
	net -= HitCost * float64(p.Hits)
	return net > 0
}

// gateOracle may only answer yes or no to a package the model proposed.
//
// It cannot propose one, which is what isolates the gate from the search. A bool
// is the whole of its expressive power.
//
// **This differs from the oracle-design document, which writes the signature as taking
// one Move.** A single move cannot express the unit the shipped gate actually
// judges: `decide` accepts a funded pair as one decision and zeroes the funding
// legs' reported gains precisely because none of them stands up alone, so a
// per-move yes/no would have to invent an acceptance rule the shipped code does
// not have. The package is the honest unit.
type gateOracle func(s *Season, p transferProposal) bool

// perfectGate is the transfer-gate oracle: accept exactly the packages that
// turned out to be worth making.
//
// # What it does and does not bound
//
// The model still *proposes*. The oracle only says yes or no, so this separates
// the gate from the search, which no other measurement in this package does. It
// bounds the entire minimum-gain / free-transfer-charge / hit-threshold family at
// once: no constant in that family can be worth more than a gate that is right
// every time.
//
// It cannot bound the search. A package the model never proposes is invisible
// here, and the standing finding is that the search's problem is *valuation*
// rather than reach.
//
// # Its figure is a FLOOR on perfect-gate performance, not a clean ceiling
//
// Three reasons, all implementation rather than statistics, and worth stating
// where the code is rather than only in the write-up:
//
//   - **It judges realised raw player points, not realised XI value.** The
//     modelled quantity it stands in for is a change in analysis.XIValue, and this
//     project's record is explicit that the sum of the players' own deltas is a
//     *biased* proxy for what a swap does to the eleven — "the proxy may filter; it
//     must never decide". Here it decides. Judging on XI value would need the
//     realised eleven each week, which is a second scoring pass.
//   - **The singles loop breaks on first rejection.** decide's `default:` arm
//     returns as soon as a proposal is refused, so this oracle cannot decline the
//     best-modelled single move and then take a good second-best one. It is a
//     perfect gate *in the model's proposal order*, which is weaker.
//   - **The judging window is the shipped DecisionHorizon.** p.Horizon is read
//     rather than chosen, deliberately — a second expression of the decision window
//     is the drift this file exists to prevent — but it means the figure bounds a
//     perfect gate at the shipped horizon and bounds nothing about changing it.
//
// And it does not bound min_gain for a *unified* policy: unified.go applies
// `gain < cfg.MinGain*n` before it ever reaches acceptTransfer, so that filter is
// outside the hook.
//
// None of this weakens the conclusion drawn from it, which rests on the ratio
// rather than on the level: the constants in this family measure 11-34 a season
// against a significance threshold of 94, so one would have to capture ~89% of
// perfect hindsight to resolve. See the transfer-policy note.
//
// ⚠️ **The ~89% is DEMOTED, 2026-08-15, and the sentence above is its only stated
// support — so the CONCLUSION survives while its DERIVATION does not.** It is
// `sig_season/perfect` on one bank — four seasons, `0102d0d`, dirty, before the
// 2026-08-12 archive repairs — and it does not transport: 0.696 on those same four
// seasons out of the later bank (`82fc8e0`, clean) — a different run, not a
// data-state effect ALONE; the channels are not separated — and 0.414 on six. It also
// charges a hypothetical constant the *perfect* arm's threshold, which is the
// cross-arm substitution AGENTS.md's "a detection threshold belongs to a comparison"
// forbids. And the correction runs AGAINST the closure: a perfect gate replaces the
// squad far harder than a `min_gain` nudge, so 94 over-states a constant's own
// threshold and the real bar is lower — gate constants are MORE resolvable than the
// withdrawn reason claimed, at the edge of this instrument rather than out of reach.
//
// What holds the closure up is the swept family judged on its own rows, and that
// ground is weaker than the ratio it replaces: **one invariance and two ties.** The
// invariance is `min_gain` at or below 0.4 reading byte-identical, at 12 cells and
// again at 36 — a fact about the code, and the strongest row. The two ties are the
// arms with thresholds of their own, which FAIL TO REJECT: the floor at horizon 8,
// −15.8 against 34, and the horizon arm, −8.4 against 21.7 — under "a null is a tie"
// they support the closure only by not refuting it. The fourth row, monotone harmful
// at the three values swept above it (0.7/0.95/1.30), is 24 cells on four seasons
// with no threshold recorded and its cells never banked — a GAP rather than a null,
// its bottom rung being −0.866 pts/gw = −32.9 a season. So the honest scope is
// "nothing swept in this family is RECORDED as having resolved", which is NARROWER
// than "nothing in this family can resolve".
//
// ⚠️ **That also re-grounds the three FLOOR caveats above.** They were dismissed as
// not weakening the conclusion because it "rests on the ratio rather than on the
// level" — and the ratio is now gone, so that reason is gone with it. They are
// harmless for a better reason: the re-grounded closure reads the swept family's own
// rows and never reads this arm's level at all.
//
// # Judged over the window the decision was justified on
//
// Same convention as Judge: a transfer made at GW8 is not a commitment to hold
// until May, because the policy re-decides every week. The horizon handed in has
// already been shortened by effectiveHorizon, so it is read rather than
// recomputed — a second expression of the decision window is exactly the drift
// this file exists to prevent.
//
// Hits are charged at their real four points, so a package that gains three over
// the window and costs a hit is correctly refused.
func perfectGate(s *Season, p transferProposal) bool {
	weeks := int(p.Horizon)
	if weeks < 1 {
		weeks = 1
	}
	net := 0
	for _, mv := range p.Moves {
		net += pointsOver(s, mv.InID, p.GW, weeks) - pointsOver(s, mv.OutID, p.GW, weeks)
	}
	net -= int(HitCost) * p.Hits
	return net > 0
}
