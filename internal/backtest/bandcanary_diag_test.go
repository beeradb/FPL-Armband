package backtest

// A canary for the fixture-run mediator: which dose of `band_strength` makes it
// provably respond?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBandCanary -v -timeout 2h
//
// # Why a canary is needed before a tandem sweep, not after
//
// A tandem arm crossing banking, fixture runs and chip preparation could come
// back flat for two reasons that license opposite conclusions: the policy saw the
// band distinction and declined to act, or the mediator could not respond at any
// setting the sweep used. This project's signature failure is reading the second
// as the first. `band_ready_weeks` is not enough on its own, because it counts
// weeks the bands EXISTED and is invariant in `band_strength` by construction —
// `BandChannelLive` reads `teamBands().ready` and the magnitude switch, neither
// of which the dose touches.
//
// So the canary is a dose at which the three MOVE columns and `band_exposure`
// must differ from the shipped dose of 0. If they do not move there, a zero
// anywhere below it is unreadable.
//
// # What this is NOT
//
// The canary is not a recommended setting and must not be read as one. It is the
// dose that makes a null interpretable, chosen for the size of the mediator's
// response and explicitly NOT for what it scored — picking a dose because it
// scored well is the argmax trap this record names as its most load-bearing idea.
// No points figure, threshold or verdict is reported here.

import (
	"fmt"
	"os"
	"testing"
)

// bandDose is one arm of the ladder: what the fixture mediator did at one
// `band_strength`, pooled over the cells, plus how many cells changed decisions.
type bandDose struct {
	strength float64
	cells    int
	// The five mediator columns, summed over cells. ReadyWeeks is summed for
	// completeness and is expected to be identical at every dose.
	ready, moves, run, worse, exposure int
	// decisionWeeks is the funnel's shared denominator, from the banking block.
	decisionWeeks int
	// Cells whose opening fifteen, whose transfer list, or whose mediator differs
	// from the same cell at dose 0. Counted per cell rather than pooled, because a
	// pooled sum can cancel across cells and a count cannot.
	squadMoved, movesMoved, mediatorMoved int
}

// bandCell identifies one replayed cell, so a dose can be compared against dose 0
// within the same football rather than against a pooled total.
type bandCell struct {
	season string
	start  int
}

// TestDiagBandCanary is the dose-response of the fixture mediator against
// `band_strength`.
//
// The grid is whatever `sweepPairNames` returns, entered at GW1 and GW16, with
// WeeklyXI true, BankUpTo pinned at sweepBankLimit, no chips and no banking — the
// fixture lever alone, so nothing else can be the cause of a move.
//
// ⚠️ **That is the SIX-season extended grid unless `FPL_SWEEP_SEASONS=default` is
// set**, which is why the header prints the variable rather than a season count
// taken on trust. The recorded canary was measured on the four-season grid at 8
// cells, so a "8 of 8" reading does not transport to a 12-cell run and the dose
// has to be re-read there.
//
// The ladder is 0, 0.25, 0.5, 1, 2, 4. The first four are the settings this
// record already argues about — 0 ships, 0.25 is the arm the original refutation
// turned on and is still unrun, and 1 and 2 are the two banked arms — so the
// ladder answers the canary question and locates it relative to the settings a
// sweep would plausibly use in the same run. 4 is above every value ever tried
// and is included as the upper rung: a canary that has to go there tells a
// different story from one that fires at 1.
func TestDiagBandCanary(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := sweepPairNames()
	starts := []int{1, 16}
	doses := []float64{0, 0.25, 0.5, 1, 2, 4}

	// The gate the whole reading depends on. Under FPL_MAGNITUDE,
	// fixtureMultipliersFor returns before consulting the bands at all, so every
	// dose would be a byte-identical null off a lever that was bypassed — the
	// exact inversion BandChannelLive was written to prevent.
	magnitude := os.Getenv("FPL_MAGNITUDE")
	if magnitude != "" {
		t.Fatalf("FPL_MAGNITUDE is set to %q. The band channel is bypassed one "+
			"function above attackBandAdj under that switch, so every dose below "+
			"would report a clean zero for a reason that is not the dose. Unset it "+
			"and re-run.", magnitude)
	}

	type key struct {
		cell bandCell
		dose float64
	}
	fingerprint := map[key]struct {
		squad, moves string
		med          FixtureRunMediator
	}{}

	out := make([]*bandDose, len(doses))
	for i, d := range doses {
		out[i] = &bandDose{strength: d}
	}

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		for _, start := range starts {
			c := bandCell{pair[1], start}
			for i, d := range doses {
				sc := sweepConfig(cfg, start, true)
				sc.Weights.BandStrength = d
				res, err := Simulate(cur, prior, sc)
				if err != nil {
					t.Fatalf("band_strength %v, %s from GW%d: %v", d, pair[1], start, err)
				}
				f := res.FixtureRuns
				a := out[i]
				a.cells++
				a.ready += f.ReadyWeeks
				a.moves += f.Moves
				a.run += f.RunMoves
				a.worse += f.WorseMoves
				a.exposure += f.Exposure
				a.decisionWeeks += res.Banking.DecisionWeeks

				fp := struct {
					squad, moves string
					med          FixtureRunMediator
				}{squadHash(res.OpeningSquad), moveKeyOf(res), f}
				fingerprint[key{c, d}] = fp
				if base, ok := fingerprint[key{c, 0}]; ok && d != 0 {
					if base.squad != fp.squad {
						a.squadMoved++
					}
					if base.moves != fp.moves {
						a.movesMoved++
					}
					if base.med != fp.med {
						a.mediatorMoved++
					}
				}
			}
		}
	}

	fmt.Printf("\nBAND CANARY — %s from GW1 and GW16, WeeklyXI=true, bank_up_to %d,\n",
		seasonsLabel(len(pairs)), sweepBankLimit)
	fmt.Printf("no chips, no chip preparation, bank_transfers_lookahead off.\n")
	fmt.Printf("FPL_MAGNITUDE unset, so the band channel reaches " +
		"fixtureMultipliersFor.\n")
	fmt.Printf("FPL_SWEEP_SEASONS=%q, FPL_NO_XG_REPAIR=%q, FPL_NO_XGC_REPAIR=%q.\n",
		os.Getenv("FPL_SWEEP_SEASONS"), os.Getenv("FPL_NO_XG_REPAIR"),
		os.Getenv("FPL_NO_XGC_REPAIR"))
	fmt.Printf("Counts only: no points column, no threshold, no verdict.\n\n")

	fmt.Printf("%-6s %6s %7s %7s %7s %7s %9s | %s\n", "dose", "cells", "decision",
		"ready", "moves", "better", "worse", "exposure")
	for _, a := range out {
		fmt.Printf("%-6.2f %6d %7d %7d %7d %7d %9d | %+d\n", a.strength, a.cells,
			a.decisionWeeks, a.ready, a.moves, a.run, a.worse, a.exposure)
	}

	fmt.Printf("\nCells differing from the SAME cell at band_strength 0 (of %d):\n\n",
		out[0].cells)
	fmt.Printf("%-6s %10s %10s %10s\n", "dose", "squad", "transfers", "mediator")
	for _, a := range out {
		if a.strength == 0 {
			continue
		}
		fmt.Printf("%-6.2f %10d %10d %10d\n", a.strength,
			a.squadMoved, a.movesMoved, a.mediatorMoved)
	}

	// The reading, stated in the output so it cannot drift from the numbers.
	fmt.Printf("\nHow to read it.\n")
	fmt.Printf("`ready` is expected to be IDENTICAL at every dose: BandChannelLive\n")
	fmt.Printf("reads teamBands().ready and the magnitude switch, neither of which the\n")
	fmt.Printf("dose touches. It is the funnel's first step and it is NOT a canary — a\n")
	fmt.Printf("dose that moved it would mean the readiness guard had picked up a\n")
	fmt.Printf("dependence on the lever.\n\n")
	fmt.Printf("The canary is the lowest dose whose `mediator` column above is the full\n")
	fmt.Printf("cell count, or close to it: at that dose a zero reading elsewhere means\n")
	fmt.Printf("the policy declined, rather than that the instrument could not respond.\n")
	fmt.Printf("It is a readability threshold and NOT a recommended setting; nothing\n")
	fmt.Printf("here scores any dose and none should be chosen on points.\n")
}

// moveKeyOf is a season's transfer list as one string, for asking whether two
// arms made the same decisions.
//
// ⚠️ It is a second fingerprint beside the local closure in
// TestTheFixtureRunLeverReachesTheTransferDecision, which keys on player NAMES
// where this keys on ids. Each is only ever compared with itself, so a
// divergence cannot produce a wrong answer today — but nothing guards that, and
// neither copied-expression scan matches this shape. Recorded rather than merged
// because the two want different keys: an id survives a name change mid-season,
// and the other one's failure message is read by a human.
func moveKeyOf(res *SimResult) string {
	s := ""
	for _, mv := range res.Moves {
		s += fmt.Sprintf("%d:%d>%d;", mv.GW, mv.OutID, mv.InID)
	}
	return s
}

// TestTheBandDoseCanaryDiscriminates is the canary's own liveness guard, and it
// is the half that makes a zero elsewhere mean something.
//
// A canary is a claim that an effect of a given size WOULD move the arm. Sizing
// one wrongly is how an arm passes a check it should have failed, so the claim is
// asserted here on the mediator directly: at a dose the diagnostic above nominates
// as the canary, the fixture mediator must differ from the same cell at dose 0.
//
// It asserts a difference and deliberately not a direction or a magnitude. Which
// way exposure moves is the finding a tandem sweep exists to report, and a test
// pinned to what one season did rots within days.
func TestTheBandDoseCanaryDiscriminates(t *testing.T) {
	cur, prior, base := chipSim(t)

	run := func(strength float64) *SimResult {
		sc := base
		sc.Weights.BandStrength = strength
		res, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatalf("band_strength %v: %v", strength, err)
		}
		return res
	}
	off := run(0)
	if off.FixtureRuns.ReadyWeeks == 0 {
		t.Skip("the bands were never ready in this season, so a mediator that " +
			"cannot respond is indistinguishable from one that did not")
	}

	// bandCanaryDose is the dose TestDiagBandCanary nominates. Pinned here so the
	// claim "at this dose the mediator must respond" is executed by the gate suite
	// rather than resting on a diagnostic nobody re-runs.
	const bandCanaryDose = 2.0
	on := run(bandCanaryDose)

	if off.FixtureRuns.ReadyWeeks != on.FixtureRuns.ReadyWeeks {
		t.Errorf("band_ready_weeks moved from %d to %d across the dose. It counts "+
			"weeks the bands EXISTED and BandChannelLive reads no dose, so this is "+
			"the readiness guard having acquired a dependence on the lever — which "+
			"would make the funnel's first step a restatement of its second",
			off.FixtureRuns.ReadyWeeks, on.FixtureRuns.ReadyWeeks)
	}
	if off.FixtureRuns == on.FixtureRuns {
		t.Fatalf("the fixture mediator is identical at band_strength 0 and %v: "+
			"%+v.\n\nThe canary does not discriminate, so a flat tandem result at or "+
			"below this dose would be unreadable — it could not be told apart from a "+
			"mediator that cannot respond. Do not bank a tandem sweep until this "+
			"passes: raise the dose, or find where the lever stops arriving.",
			bandCanaryDose, off.FixtureRuns)
	}
	t.Logf("band_strength 0 -> %v over %d decision weeks (%d band-ready): "+
		"moves %d -> %d, better %d -> %d, worse %d -> %d, exposure %+d -> %+d",
		bandCanaryDose, off.Banking.DecisionWeeks, off.FixtureRuns.ReadyWeeks,
		off.FixtureRuns.Moves, on.FixtureRuns.Moves,
		off.FixtureRuns.RunMoves, on.FixtureRuns.RunMoves,
		off.FixtureRuns.WorseMoves, on.FixtureRuns.WorseMoves,
		off.FixtureRuns.Exposure, on.FixtureRuns.Exposure)
}
