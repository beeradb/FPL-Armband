package backtest

// The unit half of the anti-residual arm: two checks that need no replay at all.
//
// Both exist because the failures they catch are invisible in a sweep. A fourth
// near-copy behind one switch can be wired to a sibling's hook, and a sign that did
// not flip produces a plausible table either way; a per-package log that drifts from
// the criteria it describes moves a *pre-registered null* rather than failing
// visibly. Neither is detectable from the figures the sweep prints, and both are
// decidable from the predicates alone in milliseconds.

import "testing"

// antiResidualFixture is a season whose forwards differ on conversion in both
// directions and by very different amounts, so an enumeration over its pairs meets
// positive, negative and exactly-zero residual gaps rather than one sign.
//
// Forwards throughout, for the reason TestPerfectGateResidualJudgesOnlyConversion
// gives: cleanSheetPoints is zero for them and concedeBlock has no entry, so the
// residual here is the goals channel alone and no assertion depends on the
// clean-sheet or concede reconstructions.
func antiResidualFixture() *Season {
	// Each player is described by his per-gameweek points, goals and xG over
	// GW10-13. 5 and 6 are deliberate duplicates, so the enumeration below contains
	// pairs whose residual gap is exactly zero — the one case where neither
	// criterion may accept a free package.
	rows := []struct {
		id            int
		points, goals int
		xg            float64
	}{
		{1, 6, 1, 0.0},  // scored from nothing: large positive residual
		{2, 6, 0, 1.0},  // missed everything: large negative residual
		{3, 4, 1, 0.5},  // mildly over-converting
		{4, 4, 0, 0.0},  // no shot and no goal: nothing on this channel
		{5, 2, 0, 0.25}, // a small under-converter...
		{6, 2, 0, 0.25}, // ...and his identical twin
	}
	s := &Season{Name: "2025-26", Players: map[int]*Player{}}
	for _, r := range rows {
		gws := map[int]GW{}
		for gw := 10; gw <= 13; gw++ {
			gws[gw] = GW{Points: r.points, Goals: r.goals, XG: r.xg,
				Minutes: 90, Fixtures: 1}
		}
		s.Players[r.id] = &Player{ID: r.id, Type: 4, GWs: gws}
	}
	// As repaired() does for every loaded season. The sample is far under
	// minCalibrationSample so the scale falls back to neutral, which is why the
	// twins above are twins.
	//
	// ⚠️ resolveInstrumentInputs, NOT its halves — which is what its own doc comment
	// asks of a hand-built season standing in for repaired(). It resolves the
	// season's points table onto each Player as well as the conversion scale.
	// Without the table every Player carries a zero-valued ScoringRules, `Prices`
	// answers false for a forward, and `XPointsResidual` refuses the whole fixture.
	// This test was written against a Season that needed only the conversion scale,
	// so it is now two steps behind Load rather than one — the shape repaired()'s
	// own header catalogues, and exactly what happened when this test and the
	// season-rules pin first met, each correct alone.
	s.resolveInstrumentInputs()
	return s
}

// antiResidualProposals enumerates the packages the two tests below are asserted
// over: every ordered pair of the fixture's forwards, at three horizons, with and
// without a hit.
func antiResidualProposals() []transferProposal {
	var out []transferProposal
	for in := 1; in <= 6; in++ {
		for outID := 1; outID <= 6; outID++ {
			if in == outID {
				continue
			}
			for _, horizon := range []float64{1, 2, 3} {
				for _, hits := range []int{0, 1} {
					out = append(out, transferProposal{
						Moves:   []Move{{OutID: outID, InID: in}},
						GW:      10,
						Horizon: horizon,
						Hits:    hits,
					})
				}
			}
		}
	}
	return out
}

// TestTheAntiResidualGateIsTheResidualGateWithOneSignFlipped is the cheapest
// available check on the new arm, and it catches the failure that would be silent.
//
// Three near-copies behind one switch is already the arrangement where an arm
// stamps itself and runs a sibling's criterion. A fourth that is the *negation* of
// one of them is worse: "wired to the sibling's hook" and "the sign did not flip"
// both produce a plausible sweep, and telling them apart from the output needs the
// very figure the sweep exists to produce.
//
// Two claims, and the second is the sharper one:
//
//   - **No package may be accepted by both.** `{ΔR > 4h}` and `{ΔR < −4h}` cannot
//     overlap at any h ≥ 0. An overlap means one arm is running the other's hook.
//   - **With no hit they must partition, except at ΔR exactly zero.** The dead band
//     is 8h wide, so it closes entirely for a free package. That is the construction
//     the whole contrast rests on — it is why the two accept masses are `p` and
//     `1 − p` rather than two unrelated numbers — and it is asserted here rather
//     than assumed in a write-up.
func TestTheAntiResidualGateIsTheResidualGateWithOneSignFlipped(t *testing.T) {
	s := antiResidualFixture()
	var sawRes, sawAnti bool
	for _, p := range antiResidualProposals() {
		res, anti := perfectGateResidual(s, p), perfectGateAntiResidual(s, p)
		if res && anti {
			t.Fatalf("horizon %.0f hits %d, %d for %d: accepted by BOTH the residual "+
				"gate and its negation, which is impossible for two criteria that "+
				"differ only in sign — one is running the other's hook",
				p.Horizon, p.Hits, p.Moves[0].InID, p.Moves[0].OutID)
		}
		sawRes = sawRes || res
		sawAnti = sawAnti || anti
		if p.Hits != 0 {
			continue
		}
		// The dead band has zero width with no hit to pay for, so the only package
		// both may refuse is one whose residual gap is exactly zero.
		if !res && !anti {
			if g := describeGatePackage(s, p, false); g.DR != 0 {
				t.Errorf("horizon %.0f: %d for %d is refused by both criteria with no "+
					"hit and a residual gap of %+.4f — with h = 0 the accept sets "+
					"partition the stream and only ΔR = 0 may fall between them",
					p.Horizon, p.Moves[0].InID, p.Moves[0].OutID, g.DR)
			}
		}
	}
	// An enumeration that never saw either criterion accept anything would pass
	// vacuously, which is the shape of failure this package keeps paying for.
	if !sawRes || !sawAnti {
		t.Fatalf("the fixture never exercised one of the criteria (residual accepted "+
			"something: %v, anti-residual: %v); the enumeration is vacuous",
			sawRes, sawAnti)
	}

}

// TestEveryGateAxisRoutesToItsOwnPredicate pins the switch, which the disjointness
// enumeration above is structurally blind to.
//
// ⚠️ **Two probe proposals are not enough, and a code review proved it rather than
// suspecting it.** The first version of this check asserted the anti axis against
// one package of positive residual and one of negative. On that fixture
// `perfectGateXPoints` answers **identically to `perfectGateAntiResidual` on both
// probes**, so `case AxisTransferGateAntiResidual: return perfectGateXPoints(s, p)`
// passed — and under that mis-route the sweep prints a six-column table in which
// the anti arm is a byte-copy of the underlying arm, and the pre-registered
// contrast is silently a different contrast. The accept-everything arm was weaker
// still: its single probe is accepted by *two* of the four siblings.
//
// So the check is now exhaustive rather than probed. Every axis is run through
// `acceptTransfer` over the whole enumeration and must agree with its own predicate
// on **every** package — which no mis-route can satisfy unless two predicates are
// identical functions, and the vacuity guard below refuses that too.
func TestEveryGateAxisRoutesToItsOwnPredicate(t *testing.T) {
	s := antiResidualFixture()
	ps := antiResidualProposals()

	axes := []struct {
		name string
		axis DecisionAxis
		want gateOracle
	}{
		{"transfergate", AxisTransferGate, perfectGate},
		{"transfergatexp", AxisTransferGateXPoints, perfectGateXPoints},
		{"transfergateres", AxisTransferGateResidual, perfectGateResidual},
		{"transfergateanti", AxisTransferGateAntiResidual, perfectGateAntiResidual},
		{"transfergateall", AxisTransferGateAcceptAll, gateAcceptEverything},
	}
	answers := make([][]bool, len(axes))
	for i, a := range axes {
		cfg := SimConfig{Oracles: Oracles{Decision: a.axis}}
		answers[i] = make([]bool, len(ps))
		for j, p := range ps {
			got := acceptTransfer(cfg, s, p)
			if want := a.want(s, p); got != want {
				t.Fatalf("axis %s, horizon %.0f hits %d, %d for %d: the switch answered "+
					"%v and its own predicate answers %v — the axis is wired to another "+
					"axis's hook", a.name, p.Horizon, p.Hits, p.Moves[0].InID,
					p.Moves[0].OutID, got, want)
			}
			answers[i][j] = got
		}
	}
	// The vacuity guard, and it is the half that makes the loop above mean anything:
	// if two predicates agreed everywhere on this fixture, routing one axis to the
	// other would satisfy every assertion. Each pair must differ somewhere.
	for i := range axes {
		for j := i + 1; j < len(axes); j++ {
			differs := false
			for k := range ps {
				if answers[i][k] != answers[j][k] {
					differs = true
					break
				}
			}
			if !differs {
				t.Errorf("%s and %s answer identically on all %d enumerated packages, "+
					"so this fixture cannot tell a mis-route between them from correct "+
					"wiring — widen antiResidualFixture", axes[i].name, axes[j].name,
					len(ps))
			}
		}
	}

	// And the shipped rule must differ from all five, or "falls through to
	// shippedAccept" would be undetectable — the accept-everything arm's own failure
	// mode, since it has no criterion to be checked against.
	shipped := SimConfig{MinGain: 0.4, MinGainHit: 2}
	sawShippedDiffer := map[string]bool{}
	for _, p := range ps {
		barred := p.withBar(shipped.MinGain)
		want := barred.shippedAccept(shipped)
		if got := acceptTransfer(shipped, s, barred); got != want {
			t.Fatalf("the un-oracled config does not run the shipped rule")
		}
		for i, a := range axes {
			if acceptTransfer(SimConfig{Oracles: Oracles{Decision: a.axis}}, s, barred) != want {
				sawShippedDiffer[axes[i].name] = true
			}
		}
	}
	for _, a := range axes {
		if !sawShippedDiffer[a.name] {
			t.Errorf("%s answers exactly as the shipped rule does on all %d enumerated "+
				"packages, so an axis falling through to shippedAccept would be "+
				"indistinguishable from it here", a.name, len(ps))
		}
	}
}

// TestTheGateLogChangesNoDecision is the claim SimConfig.gateLog makes about
// itself, asserted rather than traced.
//
// The hook is invisible to the oracle machinery — Tier 1 covers InfoOracle only and
// Tier 2 keys on Oracles — so nothing else in this package could notice a future
// version of it reaching a decision. It runs a real season twice, identical but for
// the log, and requires every collected outcome to be byte-identical.
//
// A real season rather than a fixture, because what is being checked is that the
// observer cannot reach the decision *path*, and the decision path is what a
// two-empty-season fixture does not have.
func TestTheGateLogChangesNoDecision(t *testing.T) {
	cur, prior, base := chipSim(t)

	quiet, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	logged := base
	var n int
	logged.gateLog = func(gatePackage) { n++ }
	noisy, err := Simulate(cur, prior, logged)
	if err != nil {
		t.Fatalf("logged: %v", err)
	}
	if quiet.Points != noisy.Points || quiet.Transfers != noisy.Transfers ||
		quiet.Hits != noisy.Hits || quiet.XPoints != noisy.XPoints ||
		squadHash(quiet.OpeningSquad) != squadHash(noisy.OpeningSquad) {
		t.Errorf("the gate log changed the season it observes: points %d against %d, "+
			"moves %d against %d, hits %d against %d, squads %s against %s",
			quiet.Points, noisy.Points, quiet.Transfers, noisy.Transfers,
			quiet.Hits, noisy.Hits, squadHash(quiet.OpeningSquad),
			squadHash(noisy.OpeningSquad))
	}
	// And it must have observed something, or the assertion above is a comparison of
	// a run with itself.
	if n == 0 {
		t.Error("the gate log recorded no packages at all, so the invariance above " +
			"holds vacuously")
	}
}

// TestTheGateLogAgreesWithTheCriteriaItDescribes pins the per-package log against
// the predicates it is meant to describe.
//
// The log re-derives the window from `p.Horizon` and re-walks the package, so it is
// a fourth expression of arithmetic three predicates already carry. That is the
// convention gate.go keeps — every one of them reads the same `p.Horizon`, which is
// what makes the copies safe — but a log is the worst place to be quietly wrong,
// because its whole purpose is to supply the offset a contrast's null is built from.
// A drift here would move a pre-registered null rather than fail visibly.
//
// So each channel is asserted through the criterion that consumes it: ΔX must
// reproduce the underlying gate's answer once charged, ΔR the residual gate's, and
// −ΔR the anti-residual gate's.
func TestTheGateLogAgreesWithTheCriteriaItDescribes(t *testing.T) {
	s := antiResidualFixture()
	for _, p := range antiResidualProposals() {
		g := describeGatePackage(s, p, false)
		charge := HitCost * float64(p.Hits)
		if got, want := g.DX-charge > 0, perfectGateXPoints(s, p); got != want {
			t.Errorf("horizon %.0f hits %d, %d for %d: the log's ΔX %+.4f says %v and "+
				"perfectGateXPoints says %v", p.Horizon, p.Hits, p.Moves[0].InID,
				p.Moves[0].OutID, g.DX, got, want)
		}
		if got, want := g.DR-charge > 0, perfectGateResidual(s, p); got != want {
			t.Errorf("horizon %.0f hits %d, %d for %d: the log's ΔR %+.4f says %v and "+
				"perfectGateResidual says %v", p.Horizon, p.Hits, p.Moves[0].InID,
				p.Moves[0].OutID, g.DR, got, want)
		}
		if got, want := -g.DR-charge > 0, perfectGateAntiResidual(s, p); got != want {
			t.Errorf("horizon %.0f hits %d, %d for %d: the log's −ΔR %+.4f says %v and "+
				"perfectGateAntiResidual says %v", p.Horizon, p.Hits, p.Moves[0].InID,
				p.Moves[0].OutID, -g.DR, got, want)
		}
		// The window is read rather than recomputed, the same claim every criterion
		// in gate.go makes about itself.
		if want := int(p.Horizon); g.Weeks != want || g.GW != p.GW || g.Hits != p.Hits {
			t.Errorf("the log recorded GW%d / %d weeks / %d hits for a package of "+
				"GW%d / %.0f / %d", g.GW, g.Weeks, g.Hits, p.GW, p.Horizon, p.Hits)
		}
	}

	// The log is written on every arm and it records what the ARM answered, not what
	// any one criterion would have answered. An arm-blind log would report the same
	// accept mass for every arm, which is exactly the quantity the contrast's null
	// needs to differ between them.
	var seen []gatePackage
	cfg := SimConfig{Oracles: Oracles{Decision: AxisTransferGateAcceptAll}}
	cfg.gateLog = func(g gatePackage) { seen = append(seen, g) }
	// Player 1 scored from nothing and player 2 missed everything, so this package
	// carries a large POSITIVE residual: the accept-everything arm takes it and the
	// anti-residual arm must not.
	p := transferProposal{Moves: []Move{{OutID: 2, InID: 1}}, GW: 10, Horizon: 3}
	if !acceptTransfer(cfg, s, p) {
		t.Fatal("the accept-everything arm refused a package")
	}
	if len(seen) != 1 || !seen[0].Accepted {
		t.Fatalf("the accept-everything arm logged %d package(s); the log must record "+
			"one entry carrying the arm's own answer", len(seen))
	}
	seen = nil
	cfg.Oracles.Decision = AxisTransferGateAntiResidual
	if acceptTransfer(cfg, s, p) {
		t.Fatal("the anti-residual arm accepted a package of positive residual")
	}
	if len(seen) != 1 || seen[0].Accepted {
		t.Errorf("the anti-residual arm logged %d package(s) and recorded accepted=%v; "+
			"the log is not reading the arm's own answer", len(seen),
			len(seen) == 1 && seen[0].Accepted)
	}
}
