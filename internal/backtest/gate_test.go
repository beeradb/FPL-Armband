package backtest

// The gate was four expressions of one rule, and this is the test that it is now
// one expression of the same rule.
//
// Factoring an accept predicate is the most dangerous kind of refactor this
// package admits: every arm of every transfer sweep ever run went through those
// four expressions, so a factoring that changed one of them by a comparison
// operator would silently invalidate the whole transfer half of the record while
// every test still passed and every number still looked plausible.
//
// So this restates the four *original* expressions verbatim and asserts the
// factored predicate agrees with each over a grid of inputs that straddles every
// boundary in them. Restating them is the point: a test written against the new
// code would only prove the new code is self-consistent.

import (
	"math"
	"testing"
)

// TestTheFactoredGateIsTheShippedRule compares transferProposal.shippedAccept
// against the four expressions it replaced, term for term.
func TestTheFactoredGateIsTheShippedRule(t *testing.T) {
	cfg := SimConfig{MinGain: 0.4, MinGainHit: 2.0, MaxHits: 1}
	const horizon, freeCost = 5.0, 2.0

	// Straddling values around every threshold in the rule, including exact ties,
	// which is where the two comparison operators differ.
	gains := []float64{-1, 0, 0.3999, 0.4, 0.4001, 1, 2.5}
	moneys := []float64{-1, 0, 0.5}
	alts := []float64{0, 1.5, 3}

	for _, gain := range gains {
		for _, money := range moneys {
			// The free single: `!useHit && gain >= MinGain && gain*h+money >= freeCost`.
			want := gain >= cfg.MinGain && gain*horizon+money >= freeCost
			p := transferProposal{
				Moves: []Move{{}}, Gain: gain, Money: money,
				Horizon: horizon, FreeCost: freeCost,
			}.withBar(cfg.MinGain)
			if got := p.shippedAccept(cfg); got != want {
				t.Errorf("free single gain=%v money=%v: got %v want %v", gain, money, got, want)
			}

			// The hit single: `hits < MaxHits && gain*h+money-HitCost >= MinGainHit`,
			// with **no** minimum-gain bar. That asymmetry is the one most likely to
			// be tidied away by accident.
			wantHit := gain*horizon+money-HitCost >= cfg.MinGainHit
			ph := transferProposal{
				Moves: []Move{{}}, Gain: gain, Money: money,
				Horizon: horizon, FreeCost: freeCost,
			}.asHit()
			if got := ph.shippedAccept(cfg); got != wantHit {
				t.Errorf("hit single gain=%v money=%v: got %v want %v", gain, money, got, wantHit)
			}

			for _, alt := range alts {
				for _, hitsNeeded := range []int{0, 1} {
					// The funded pair, two moves:
					//   pairValue = gain*h + money - HitCost*hits - freeCost*(n-hits)
					//   ok = gain >= MinGain && pairValue > soloValue
					//   if hits > 0 { ok = ok && pairValue-soloValue >= MinGainHit }
					const n = 2
					pairValue := gain*horizon + money -
						HitCost*float64(hitsNeeded) - freeCost*float64(n-hitsNeeded)
					wantPair := gain >= cfg.MinGain && pairValue > alt
					if wantPair && hitsNeeded > 0 {
						wantPair = pairValue-alt >= cfg.MinGainHit
					}
					pp := transferProposal{
						Moves: []Move{{}, {}}, Gain: gain, Money: money,
						Hits: hitsNeeded, Alternative: alt, Strict: true,
						GainBar: cfg.MinGain, Horizon: horizon, FreeCost: freeCost,
					}
					if got := pp.shippedAccept(cfg); got != wantPair {
						t.Errorf("pair gain=%v money=%v alt=%v hits=%d: got %v want %v",
							gain, money, alt, hitsNeeded, got, wantPair)
					}

					// The unified search:
					//   net = gain*h - HitCost*hits - freeCost*(n-hits) - surcharge
					//   if hits > 0 && net < MinGainHit { reject }
					//   if net <= 0 { reject }
					// with no money term and no gain bar at this point (the bar is
					// applied separately because it scales with the move count).
					for _, sur := range []float64{0, 1} {
						net := gain*horizon -
							HitCost*float64(hitsNeeded) - freeCost*float64(n-hitsNeeded) - sur
						wantU := net > 0
						if wantU && hitsNeeded > 0 {
							wantU = net >= cfg.MinGainHit
						}
						up := transferProposal{
							Moves: []Move{{}, {}}, Gain: gain, Hits: hitsNeeded,
							Surcharge: sur, Strict: true, GainBar: noGainBar,
							Horizon: horizon, FreeCost: freeCost,
						}
						if got := up.shippedAccept(cfg); got != wantU {
							t.Errorf("unified gain=%v alt=0 hits=%d sur=%v: got %v want %v",
								gain, hitsNeeded, sur, got, wantU)
						}
					}
				}
			}
		}
	}
}

// TestTheGateAsymmetriesArePreserved names the two oddities in prose so that
// somebody tidying them up has to delete a test that says why they exist.
func TestTheGateAsymmetriesArePreserved(t *testing.T) {
	cfg := SimConfig{MinGain: 0.4, MinGainHit: 2.0}

	// A hit has no minimum-gain bar. A package with a gain below MinGain that
	// still clears MinGainHit across the horizon is accepted as a hit and refused
	// as a free move — which is the shipped behaviour, however odd it reads.
	low := transferProposal{
		Moves: []Move{{}}, Gain: 0.3, Horizon: 30, FreeCost: 2,
	}
	if low.withBar(cfg.MinGain).shippedAccept(cfg) {
		t.Error("a gain below MinGain was accepted as a free move")
	}
	if !low.asHit().shippedAccept(cfg) {
		t.Error("a gain below MinGain was refused as a hit; the shipped hit branch " +
			"has no minimum-gain bar and squaring that up is a behaviour change " +
			"with a measurement attached, not a refactor")
	}
	if !math.IsInf(low.asHit().GainBar, -1) {
		t.Error("asHit no longer clears the gain bar")
	}

	// The funded pair must beat its alternative strictly; a single swap need only
	// tie zero. Exactly at the boundary the two disagree.
	tie := transferProposal{
		Moves: []Move{{}}, Gain: 0.4, Horizon: 5, FreeCost: 2.0,
	}
	// gain*h = 2.0, freeCost 2.0, so value is exactly 0.
	if v := tie.value(); v != 0 {
		t.Fatalf("the boundary case is not on the boundary: value %v", v)
	}
	if !tie.withBar(cfg.MinGain).shippedAccept(cfg) {
		t.Error("a single swap worth exactly the charge was refused; the shipped " +
			"single branch compares with >=")
	}
	strict := tie
	strict.Strict = true
	strict.GainBar = cfg.MinGain
	if strict.shippedAccept(cfg) {
		t.Error("a package worth exactly its alternative was accepted; the shipped " +
			"pair branch compares with >")
	}
}

// TestPerfectGateJudgesRealisedPointsOverTheDecisionWindow pins the oracle.
func TestPerfectGateJudgesRealisedPointsOverTheDecisionWindow(t *testing.T) {
	s := &Season{Name: "2025-26", Players: map[int]*Player{
		// In: 3 points a week from GW10.
		1: {ID: 1, GWs: map[int]GW{10: {Points: 3}, 11: {Points: 3}, 12: {Points: 3}, 13: {Points: 99}}},
		// Out: 2 a week.
		2: {ID: 2, GWs: map[int]GW{10: {Points: 2}, 11: {Points: 2}, 12: {Points: 2}, 13: {Points: 0}}},
	}}
	mv := Move{OutID: 2, InID: 1}
	p := transferProposal{Moves: []Move{mv}, GW: 10, Horizon: 3}

	// +3 over the window the decision was justified on, so accepted.
	if !perfectGate(s, p) {
		t.Error("a move worth +3 over its own horizon was refused")
	}
	// The window is read, not recomputed. GW13's 99 points sit outside a
	// three-gameweek horizon and must not rescue or condemn anything — a second
	// expression of the decision window is the drift this package keeps paying for.
	short := p
	short.Horizon = 1
	if !perfectGate(s, short) {
		t.Error("a move worth +1 over one gameweek was refused")
	}
	// A hit costs its real four points, so the same +3 move refuses when it needs one.
	hit := p
	hit.Hits = 1
	if perfectGate(s, hit) {
		t.Error("a +3 move that costs a -4 was accepted; the oracle must charge " +
			"hits at what FPL charges them")
	}
	// And the shipped path must not consult the season at all. Passing nil is the
	// strongest available statement of that: a predicate that read results would
	// be the point-in-time leak this package guards hardest.
	cfg := SimConfig{MinGain: 0.4, MinGainHit: 2}
	if acceptTransfer(cfg, nil, p.withBar(cfg.MinGain)) !=
		p.withBar(cfg.MinGain).shippedAccept(cfg) {
		t.Error("the shipped gate is not what acceptTransfer runs without an oracle")
	}
}

// TestPerfectGateResidualJudgesOnlyConversion pins the third criterion against
// the two it was built to sit between.
//
// The construction is the whole test: two forwards whose realised points are
// **identical** over the window, one of whom scored from nothing and one of whom
// missed everything. Realised points cannot separate them, expected points from
// underlying separates them the WRONG way, and only the conversion residual says
// take the finisher. So all three criteria answer differently on one proposal, and
// an implementation that quietly judged points or xPoints would fail rather than
// look plausible — which is the failure mode this package keeps paying for, since
// a gate oracle's output is a bounded-looking number either way.
//
// Forwards, because cleanSheetPoints is zero for them and concedeBlock has no
// entry, so the residual here is the goals channel alone and the assertion does not
// depend on the clean-sheet or concede reconstructions.
func TestPerfectGateResidualJudgesOnlyConversion(t *testing.T) {
	// 6 points a week each, from GW10. The finisher's six are a goal (4) plus his
	// appearance; the profligate's six are the same appearance plus four points
	// from channels xPoints does not replace, so only the underlying differs.
	fin := map[int]GW{}
	prof := map[int]GW{}
	for gw := 10; gw <= 12; gw++ {
		fin[gw] = GW{Points: 6, Goals: 1, XG: 0, Minutes: 90, Fixtures: 1}
		prof[gw] = GW{Points: 6, Goals: 0, XG: 1, Minutes: 90, Fixtures: 1}
	}
	// GW13 reverses the whole thing, and sits outside a three-gameweek horizon.
	fin[13] = GW{Points: 0, Goals: 0, XG: 5, Minutes: 90, Fixtures: 1}
	prof[13] = GW{Points: 20, Goals: 5, XG: 0, Minutes: 90, Fixtures: 1}

	s := &Season{Name: "2025-26", Players: map[int]*Player{
		1: {ID: 1, Type: 4, GWs: fin},
		2: {ID: 2, Type: 4, GWs: prof},
		// A smaller pair for the hit charge: +2 of conversion over one gameweek.
		3: {ID: 3, Type: 4, GWs: map[int]GW{
			10: {Points: 4, Goals: 1, XG: 0.5, Minutes: 90, Fixtures: 1}}},
		4: {ID: 4, Type: 4, GWs: map[int]GW{
			10: {Points: 4, Goals: 0, XG: 0, Minutes: 90, Fixtures: 1}}},
	}}
	// As `repaired()` does for every loaded season. The forwards here carry 8.5
	// expected goals between them, well under minCalibrationSample, so the scale
	// falls back to neutral 1.0 and every figure the comment above reasons about —
	// the level realised points, the 24 of underlying, the +24 of residual — is
	// unchanged. The call is here so the fixture is built the way a season is,
	// not to alter its arithmetic. It also resolves the season's points table,
	// which the instrument refuses a row without.
	s.resolveInstrumentInputs()

	p := transferProposal{Moves: []Move{{OutID: 2, InID: 1}}, GW: 10, Horizon: 3}

	// Realised points are exactly level, so the points gate refuses: it needs a
	// strictly positive net and has none.
	if perfectGate(s, p) {
		t.Error("the points gate accepted a package worth exactly zero realised " +
			"points; the construction is not the one this test reasons about")
	}
	// Underlying refuses it the other way round, and hard: the incoming player's
	// expected points are 24 lower over the window.
	if perfectGateXPoints(s, p) {
		t.Error("the underlying gate accepted the player with three fewer expected " +
			"goals; the criterion is not reading xPoints")
	}
	// Only conversion says yes. +24 of residual, and nothing else in the package.
	if !perfectGateResidual(s, p) {
		t.Error("the residual gate refused +24 of pure conversion; it is not " +
			"judging realised minus expected")
	}

	// The window is read from p.Horizon, not recomputed. GW13 would reverse the
	// sign by 40 if it leaked in, which is the same drift guard the sibling carries.
	long := p
	long.Horizon = 4
	if perfectGateResidual(s, long) {
		t.Error("widening the horizon to include GW13 did not reverse the residual " +
			"gate, so it is not reading the window it was handed")
	}

	// A hit costs its real four points on this criterion too, deliberately: the arm
	// is scored on realised points, where a hit costs four of them. +2 of conversion
	// does not pay for one.
	small := transferProposal{Moves: []Move{{OutID: 4, InID: 3}}, GW: 10, Horizon: 1}
	if !perfectGateResidual(s, small) {
		t.Error("a package worth +2 of conversion was refused with no hit to pay for")
	}
	hit := small
	hit.Hits = 1
	if perfectGateResidual(s, hit) {
		t.Error("a +2 conversion package that costs a -4 was accepted; the residual " +
			"oracle must charge hits at what FPL charges them, not at their own " +
			"conversion residual, which is zero")
	}

	// Wired, and to its own hook. Three near-copies behind one switch is exactly
	// the arrangement where an arm stamps itself and runs the sibling's criterion —
	// the four-expressions bug this file was written to end, one level up.
	res := SimConfig{Oracles: Oracles{Decision: AxisTransferGateResidual}}
	if !acceptTransfer(res, s, p) {
		t.Error("AxisTransferGateResidual does not route to perfectGateResidual")
	}
	xp := SimConfig{Oracles: Oracles{Decision: AxisTransferGateXPoints}}
	if acceptTransfer(xp, s, p) {
		t.Error("AxisTransferGateXPoints answers what the residual axis answers; " +
			"the two arms are running one criterion")
	}
	pt := SimConfig{Oracles: Oracles{Decision: AxisTransferGate}}
	if acceptTransfer(pt, s, p) {
		t.Error("AxisTransferGate answers what the residual axis answers; the two " +
			"arms are running one criterion")
	}
}
