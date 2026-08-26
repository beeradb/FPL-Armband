package backtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"armband/internal/analysis"
)

// The join proofs: for every wire this change adds, delete the line and this
// fails.
//
// # Why these are separate from the mediator tests
//
// A mediator says what a lever DID. It cannot say whether the lever reached the
// decision, because a counter incremented beside an unread value counts perfectly
// well — which is the shape three branches in this repository have shipped with
// the whole suite green. So each test here pairs a **liveness** check that must
// move with a **confinement** check that must not, on the record's own rule that a
// confinement check alone confirms nothing.
//
// ⚠️ Every one of them is `POLICY`-side. `Hold` buys the opening fifteen and never
// transfers, so no lever here can reach it — and that is a code fact rather than an
// empirical null, which is why it is asserted as byte-identity rather than tested
// for a difference.

// TestTheTaperReachesTheTransferDecision is the free-transfer taper's join proof.
//
// Liveness: the taper repricing the charge every week — from roughly 0.8x at GW1
// to exactly 0x at GW38 — must change the transfer path. Confinement: it must not
// reach the held metric, because `Hold` makes no transfers.
//
// ⚠️ **Deleting `freeCost = cfg.FreeCost * wh.Factor` leaves the mediator intact**
// — `wh.Factor` is still computed, so `ftv_priced_weeks` would still be non-zero —
// and only this test notices. That asymmetry is the reason it exists.
func TestTheTaperReachesTheTransferDecision(t *testing.T) {
	cur, prior, base := chipSim(t)

	shipped, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	tapered := base
	tapered.TaperFreeTransferValue = true
	got, err := Simulate(cur, prior, tapered)
	if err != nil {
		t.Fatal(err)
	}

	// The mediator ran. This is necessary and NOT sufficient — see the doc.
	m := got.TransferHold
	if m.ConsultedWeeks == 0 {
		t.Fatalf("the taper was never consulted over %d weeks", len(got.Weeks))
	}
	if m.PricedWeeks != m.ConsultedWeeks {
		t.Errorf("the taper moved the charge in %d of %d consulted weeks; the "+
			"decay is below 1 in every gameweek of a season, so every consulted "+
			"week should be priced", m.PricedWeeks, m.ConsultedWeeks)
	}
	if m.GateCalls == 0 {
		t.Errorf("the taper counted %d gate calls; the counting wrapper is not on "+
			"the path the decision takes", m.GateCalls)
	}
	// ⚠️ **The mean charge must come out CLOSE to the flat one, not below it.**
	// The curve is normalised to average 1 over the option's whole window — see
	// analysis.MeanOptionDecay — precisely so that a taper arm is a shape contrast
	// rather than a level cut, so a mean well under `FreeCost` would mean the
	// normaliser is missing and every half-life rung is moving the level too.
	//
	// The band is wide because this cell is one entry point and the congestion
	// factor is not exactly mean-1 on any particular squad; what it refuses is the
	// un-normalised case, which reads about 0.62 x FreeCost.
	if c := m.MeanCharge(); c < 0.8*base.FreeCost || c > 1.4*base.FreeCost {
		t.Errorf("the mean applied charge is %v against a shipped %v. The curve is "+
			"mean-preserving, so these should be close; a mean near %v is the "+
			"un-normalised curve, which confounds every taper arm with a level cut.",
			c, base.FreeCost, 0.62*base.FreeCost)
	}
	// And the shipped arm did NOT run it, which is what makes the comparison a
	// comparison rather than two readings of one policy.
	if shipped.TransferHold.ConsultedWeeks != 0 {
		t.Errorf("the shipped arm consulted the taper %d times; it must be off "+
			"by default", shipped.TransferHold.ConsultedWeeks)
	}

	// Liveness. The charge enters `PackageValue` and every accept expression, and
	// it is roughly 20% lower at GW1 and zero at GW38, so a season that made any
	// transfer at all under a decision this close to its bar must differ.
	if got.Transfers == shipped.Transfers && got.Points == shipped.Points &&
		got.Hits == shipped.Hits {
		t.Errorf("the tapered arm replayed identically to shipped: %d transfers, "+
			"%d points, %d hits.\n\n"+
			"The charge is the thing the gate subtracts, so a charge that is a "+
			"fifth lower in August and zero in May cannot leave every decision "+
			"where it was. Check that `freeCost` is assigned from the factor and "+
			"not merely computed beside it — deleting that one line leaves every "+
			"mediator column populated and this is the only test that notices.",
			got.Transfers, got.Points, got.Hits)
	}

	// Confinement, and it is a code fact rather than an empirical null: `Hold`
	// buys the opening fifteen and never transfers, so no transfer charge can
	// reach it. A movement here means the lever is changing squad SELECTION,
	// which would make any figure from this arm bound something else.
	if squadHash(got.OpeningSquad) != squadHash(shipped.OpeningSquad) {
		t.Errorf("the taper moved the opening fifteen; it is a charge on a " +
			"transfer and the opening squad makes none")
	}
}

// TestTheChipTriggersReachTheSimulation is the three chip rules' join proof.
//
// Liveness: at a bar of zero each rule fires on the first week its reading is
// positive, and the week it names must be the week the simulation actually plays
// the chip in. Confinement: a rule that is off must not fire, and no two chips may
// land in one gameweek.
//
// ⚠️ **`FiredGW` alone proves nothing.** The mediator is written by `consult`,
// which does not touch the scoring switch — so deleting `trig.plays` from either
// switch leaves `FiredGW` set and the chip unplayed, and only the cross-check
// against `Week.BenchBoost` / `Week.FreeHit` / `Week.Wildcard` catches it.
func TestTheChipTriggersReachTheSimulation(t *testing.T) {
	cur, prior, base := chipSim(t)

	for _, c := range []struct {
		name  string
		apply func(*SimConfig)
		med   func(*SimResult) ChipTriggerMediator
		// played reports whether this week's record says the chip was played.
		played func(Week) bool
	}{
		{"bench boost",
			func(sc *SimConfig) { sc.BenchBoostTrigger, sc.BenchBoostBar = true, 0 },
			func(r *SimResult) ChipTriggerMediator { return r.BenchBoost },
			func(w Week) bool { return w.BenchBoost }},
		{"free hit",
			func(sc *SimConfig) { sc.FreeHitTrigger, sc.FreeHitBar = true, 0 },
			func(r *SimResult) ChipTriggerMediator { return r.FreeHit },
			func(w Week) bool { return w.FreeHit }},
		{"wildcard",
			// A reservation of zero fires as soon as the repair cost is positive,
			// which is the first week the allowance cannot reach the optimum. That
			// is deliberately the most eager setting available: this test is about
			// the WIRE, and a bar tuned to fire rarely would make it flaky.
			func(sc *SimConfig) { sc.WildcardTrigger, sc.WildcardReservation = true, 0 },
			func(r *SimResult) ChipTriggerMediator { return r.Wildcard },
			func(w Week) bool { return w.Wildcard }},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := base
			c.apply(&cfg)
			got, err := Simulate(cur, prior, cfg)
			if err != nil {
				t.Fatal(err)
			}
			m := c.med(got)
			if m.ConsultedWeeks == 0 {
				t.Fatalf("the %s rule was never consulted over %d weeks — its "+
					"eligibility test is refusing every gameweek, or the rule is "+
					"not on the path at all", c.name, len(got.Weeks))
			}
			if m.FiredGW == 0 {
				t.Fatalf("the %s rule never fired at a bar of zero, over %d "+
					"weighed weeks of %d consulted. At that bar it fires on the "+
					"first positive reading, so a zero here is the reading never "+
					"being formed rather than the bar refusing it",
					c.name, m.WeighedWeeks, m.ConsultedWeeks)
			}
			// The join itself: the weeks the mediator names are the weeks the
			// simulation played the chip in.
			//
			// ⚠️ **A chip is spent once PER SET, not once per season.** Seasons
			// from 2025-26 grant each chip twice, one set either side of
			// ChipResetGW, so a rule at a zero bar fires in both halves and two
			// plays is correct. Asserting a single play here hid that: the
			// mediator's scalar FiredGW was being overwritten by the second
			// firing, so this join compared the first play against the last
			// firing and the two disagreed.
			var playedIn []int
			for _, w := range got.Weeks {
				if c.played(w) {
					playedIn = append(playedIn, w.GW)
				}
			}
			if len(playedIn) == 0 {
				t.Fatalf("the %s rule fired at GW%d and the simulation played it "+
					"in no gameweek at all.\n\n"+
					"`consult` writes the mediator and does NOT touch the scoring "+
					"switch, so deleting `trig.plays` from that switch leaves the "+
					"mediator populated and the chip unplayed — a lever that reads "+
					"as live in every column and changes nothing. This is the only "+
					"test that separates them.", c.name, m.FiredGW)
			}
			if !slices.Equal(playedIn, m.FiredGWs) {
				t.Errorf("the %s rule fired in %v and the simulation played it in "+
					"%v; every firing must be a play and every play a firing.\n\n"+
					"`consult` writes the mediator and does NOT touch the scoring "+
					"switch, so deleting `trig.plays` from that switch leaves the "+
					"mediator populated and the chip unplayed. This is the only "+
					"test that separates them.", c.name, m.FiredGWs, playedIn)
			}
			if m.FiredGW != playedIn[0] {
				t.Errorf("the %s mediator's scalar FiredGW is GW%d but the first "+
					"play was GW%d. That field is documented as the FIRST firing "+
					"precisely so a two-set season cannot change its meaning "+
					"silently; overwriting it made it the last.",
					c.name, m.FiredGW, playedIn[0])
			}
			// One firing per set, which is what makes two plays correct rather
			// than a double-spend.
			seen := map[bool]bool{}
			for _, gw := range playedIn {
				if half := gw < ChipResetGW; seen[half] {
					t.Errorf("%s played twice in the same chip set: %v, with the "+
						"set boundary at GW%d. Two plays are only legal either "+
						"side of it.", c.name, playedIn, ChipResetGW)
				} else {
					seen[half] = true
				}
			}
			// Confinement: the other two rules stay silent, which is what makes
			// the four switches independent rather than merely separate fields.
			for _, other := range []struct {
				name string
				med  ChipTriggerMediator
			}{
				{"bench boost", got.BenchBoost}, {"free hit", got.FreeHit},
				{"wildcard", got.Wildcard},
			} {
				if other.name == c.name {
					continue
				}
				if other.med.ConsultedWeeks != 0 || other.med.FiredGW != 0 {
					t.Errorf("with only the %s lever on, the %s rule ran (%+v) — "+
						"one lever is implying another", c.name, other.name, other.med)
				}
			}
		})
	}
}

// TestATriggeredChipDoesNotCollideWithAPlannedOne is the eligibility rule's join
// proof, and it guards the one way a state rule can produce a season FPL would
// have refused.
//
// `ValidateChipSets` runs once, before the first gameweek, so it cannot see a rule
// firing later. The rule therefore enforces FPL's constraints itself, and this is
// what executes that.
func TestATriggeredChipDoesNotCollideWithAPlannedOne(t *testing.T) {
	cur, prior, base := chipSim(t)

	cfg := base
	// A bench boost the PLAN already places. The rule must decline for that chip
	// entirely rather than adding a second one.
	cfg.Chips2 = analysis.ChipPlan{BenchBoost: 30}
	cfg.BenchBoostTrigger, cfg.BenchBoostBar = true, 0
	got, err := Simulate(cur, prior, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.BenchBoost.ConsultedWeeks != 0 {
		t.Errorf("the bench boost rule was consulted %d times with the chip "+
			"already planned for GW30; a rule that fires for a planned chip gives "+
			"the season two of them, and ValidateChipSets cannot see it",
			got.BenchBoost.ConsultedWeeks)
	}
	boosts := 0
	for _, w := range got.Weeks {
		if w.BenchBoost {
			boosts++
		}
	}
	if boosts != 1 {
		t.Errorf("the season played %d bench boosts, want exactly the planned one",
			boosts)
	}
}

// TestTheHitCeilingIsReadByTheFundedPairBranch is the ceiling's join proof, and it
// is a source scan rather than a replay for a reason worth stating.
//
// The defect being guarded is a LITERAL: `hitsNeeded <= 1` beside a configurable
// `MoveLimit`. Lifting the limit and leaving the literal widens the search while
// the funded pair refuses anything that uses the extra move — which is not a
// crash, not a wrong number, and not visible in any points column, because the
// wider arm simply reproduces the narrower one. A replay cannot separate "the
// ceiling did not arrive" from "the season never wanted two hits"; the source can.
//
// ⚠️ It is a tripwire keyed on one spelling, exactly as
// `TestTheCopiedExpressionsHaveOneImplementation`'s rows are, and it is honest
// about that: a branch rewritten as `hitsNeeded < 2` escapes it. What it catches is
// somebody re-introducing the clamp by reaching for the obvious.
func TestTheHitCeilingIsReadByTheFundedPairBranch(t *testing.T) {
	src, err := os.ReadFile("simulate.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "simulate.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "decide" || fd.Body == nil {
			continue
		}
		body = string(src[fset.Position(fd.Body.Pos()).Offset:fset.Position(fd.Body.End()).Offset])
	}
	if body == "" {
		t.Fatal("no decide in simulate.go — this guard is following a seam that " +
			"has been renamed, so update it deliberately rather than letting it " +
			"pass vacuously")
	}
	// Comment lines are dropped before the negative check below, for the reason
	// the copied-expression scan drops them: this function's own comment explains
	// the clamp it replaced, and without the strip documenting the rule breaks it.
	var code []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		code = append(code, line)
	}
	body = strings.Join(code, "\n")

	if !strings.Contains(body, "hitsNeeded <= cfg.hitCeiling()") {
		t.Errorf("the funded-pair branch does not gate on cfg.hitCeiling().\n\n" +
			"`analysis.MoveLimit` clamped the hit allowance to 1 unconditionally " +
			"and this branch carried the same clamp as a literal. Lifting one " +
			"without the other widens the move limit while the pair refuses " +
			"anything that spends the extra move, so an arm at MaxHits 2 " +
			"reproduces shipped and reads as a null. No points column can see it.")
	}
	if strings.Contains(body, "hitsNeeded <= 1") {
		t.Errorf("the funded-pair branch carries the literal `hitsNeeded <= 1` " +
			"again; that is the clamp the ceiling replaced")
	}
}
