package backtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtureRunsOf is the fixture-run mediator as the cells file records it.
//
// One function rather than a few lines at each construction site, on the rule
// this package is named for: two expressions of one quantity end with the
// measured one not being the one that runs. bankingOf and chipReadingsOf are the
// same shape for the same reason.
//
// It returns no `ok`, unlike bankingOf, because it shares HasBanking's gate — the
// two funnels are counted on the same weeks and are recorded together or not at
// all. See cellRow.FixtureRuns.
func fixtureRunsOf(res *SimResult) FixtureRunMediator {
	if res == nil {
		return FixtureRunMediator{}
	}
	return res.FixtureRuns
}

// TestTheFixtureRunBlockSitsBetweenBankingAndTheChips pins the schema position,
// in the mould of TestTheBankingBlockIsBeforeTheChipBlockAndCounted.
//
// Every block in this header gets its own position test because a column dropped
// between two counted blocks is invisible to a test that indexes from either end.
// This one also pins the seam it created: the banking block used to touch the
// chip block, and five columns went in between them.
func TestTheFixtureRunBlockSitsBetweenBankingAndTheChips(t *testing.T) {
	want := []string{
		"band_ready_weeks", "band_moves", "band_run_moves", "band_worse_moves",
		"band_exposure",
	}
	if fixtureRunCols != len(want) {
		t.Fatalf("fixtureRunCols is %d and the block is %d columns",
			fixtureRunCols, len(want))
	}
	at := fixtureRunBlockAt()
	got := cellHeader[at : at+fixtureRunCols]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the %d columns before the chip block are %v, want %v",
			fixtureRunCols, got, want)
	}
	if before := cellHeader[at-1]; before != "free_at_decision" {
		t.Fatalf("the column before the fixture-run block is %q, want the banking "+
			"block's last column — the two mediators are one region", before)
	}
	// ⚠️ Its right-hand neighbour is no longer the chip block: the four
	// option-value funnels landed between them, so the decision-mediator region
	// is now three funnels wide — banking, fixture runs, option value — with the
	// dose pair after it and the chips after that.
	if after := cellHeader[at+fixtureRunCols]; after != "ftv_weeks" {
		t.Fatalf("the column after the fixture-run block is %q, want the "+
			"option-value block's first column", after)
	}
}

// TestTheFixtureRunFunnelNests pins the inequalities every reading of the block
// rests on, in the mould of TestTheBankingFunnelNests.
//
// ⚠️ **It is two nestings, not one chain.** `decision_weeks >= band_ready_weeks`
// counts weeks; `band_moves >= band_run_moves` counts MOVES, and a week can carry
// several. Asserting one single chain all the way down would be wrong on any cell that made two
// transfers in a ready week, and would fail for a reason that is not a defect —
// which is how a test gets deleted instead of read.
func TestTheFixtureRunFunnelNests(t *testing.T) {
	cur, prior, base := chipSim(t)

	for _, c := range []struct {
		name     string
		strength float64
	}{
		{"bands off", 0},
		{"bands on", 1},
	} {
		sc := base
		sc.Weights.BandStrength = c.strength
		res, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		f, b := res.FixtureRuns, res.Banking

		if f.ReadyWeeks > b.DecisionWeeks {
			t.Errorf("%s: band_ready_weeks %d exceeds decision_weeks %d — the bands "+
				"were counted in a week that never reached the transfer decision",
				c.name, f.ReadyWeeks, b.DecisionWeeks)
		}
		if f.RunMoves+f.WorseMoves > f.Moves {
			t.Errorf("%s: band_run_moves %d plus band_worse_moves %d exceeds "+
				"band_moves %d — the two directions are disjoint subsets of the "+
				"moves counted, and the remainder is the ties",
				c.name, f.RunMoves, f.WorseMoves, f.Moves)
		}
		// A zero exposure sum with moves in both directions is fine; a non-zero one
		// with neither is not, and would mean the sign classification and the
		// magnitude are being computed from different quantities.
		if f.Exposure != 0 && f.RunMoves == 0 && f.WorseMoves == 0 {
			t.Errorf("%s: band_exposure is %+d with no move classified in either "+
				"direction, so the sum and the counts are not reading the same delta",
				c.name, f.Exposure)
		}
		if f.Moves > 0 && f.ReadyWeeks == 0 {
			t.Errorf("%s: %d moves counted with the bands never ready, which the "+
				"accumulation guard makes impossible", c.name, f.Moves)
		}
		// The bands need five matches played, so a season entered EARLY cannot
		// report every decision week ready. A cell that does has lost the readiness
		// guard, which would make band_ready_weeks a restatement of decision_weeks
		// and the first step of the funnel worthless.
		//
		// ⚠️ **Gated on an early start point, and that gate is not decoration.**
		// From about GW7 onward every decision week IS ready, so this assertion is
		// legitimately false at four of the sweep's six entry points. Ungated it
		// would fail for a reason that is not a defect the moment someone changed
		// the fixture's start — which the comment above already names as how a test
		// gets deleted instead of read.
		if base.StartGW <= 3 && b.DecisionWeeks > 0 && f.ReadyWeeks == b.DecisionWeeks {
			t.Errorf("%s: all %d decision weeks report the bands ready, but a season "+
				"entered at GW%d must run its opening weeks with too few matches "+
				"played for any rating to exist", c.name, f.ReadyWeeks, base.StartGW)
		}
		t.Logf("%s: %d decision weeks, %d band-ready, %d moves — %d better, %d "+
			"worse, %d tied — exposure %+d", c.name, b.DecisionWeeks, f.ReadyWeeks,
			f.Moves, f.RunMoves, f.WorseMoves, f.Moves-f.RunMoves-f.WorseMoves,
			f.Exposure)
	}
}

// TestTheSweepWritesTheFixtureRunBlock closes the join nothing else covers, and
// it exists because the banking work found the identical hole one commit earlier:
// deleting the single line that put that mediator into the CSV left the whole
// package green, and both columns would have been blank in every cell of every
// sweep for ever.
//
// The brief for this change said to assume the same hole and prove it is not
// there. This is that proof — it fails if `row.FixtureRuns = fixtureRunsOf(res)`
// is removed from runPolicySweep.
//
// It asserts the JOIN and deliberately not the VALUES. Whether the policy ever
// moves toward a better run is the finding the columns exist to report, and a
// test pinned to what the data did rots within days.
func TestTheSweepWritesTheFixtureRunBlock(t *testing.T) {
	cur, prior, base := chipSim(t)
	base.Weights.BandStrength = 1
	res, err := Simulate(cur, prior, base)
	if err != nil {
		t.Fatal(err)
	}
	if res.FixtureRuns.ReadyWeeks == 0 {
		t.Skip("the bands were never ready in this season, so a populated block " +
			"cannot be told from an empty one")
	}

	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	row := cellRow{
		Sweep: sink.sweepLabel("T"), RunID: sink.run(), Variant: "band_strength 1",
		Season: "2025-26", PriorSeason: "2024-25", StartGW: 1, Weeks: len(res.Weeks),
	}.under(base.Oracles)
	row.BankingMediator, row.HasBanking = bankingOf(res)
	row.FixtureRuns = fixtureRunsOf(res)
	sink.cell(row)
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	for _, col := range []string{
		"band_ready_weeks", "band_moves", "band_run_moves", "band_worse_moves",
		"band_exposure",
	} {
		if r[col] == "" {
			t.Errorf("%s is blank on a season whose bands were ready in %d weeks — "+
				"the sweep's cellRow is not being filled from the SimResult.\n\n"+
				"Look for `row.FixtureRuns = fixtureRunsOf(res)` in runPolicySweep. "+
				"That join is one line, nothing else in the package executes it, and "+
				"without it every cell of every sweep carries five empty columns "+
				"while the sweep prints and banks normally. This is the failure the "+
				"banking block found the hard way.", col, res.FixtureRuns.ReadyWeeks)
		}
	}
	if got := atoiOrFail(t, r["band_ready_weeks"]); got != res.FixtureRuns.ReadyWeeks {
		t.Errorf("band_ready_weeks is %d and the season counted %d",
			got, res.FixtureRuns.ReadyWeeks)
	}
	if got := atoiOrFail(t, r["band_run_moves"]); got != res.FixtureRuns.RunMoves {
		t.Errorf("band_run_moves is %d and the season counted %d",
			got, res.FixtureRuns.RunMoves)
	}
}

// TestTheFixtureRunColumnsAreBlankWithoutBands is the block's half of the file's
// standing rule: **blank is a gap and zero is a measurement**.
//
// band_ready_weeks is written on every arm that decided anything, so a zero there
// says the bands never existed — a real reading, and the true one for an
// early-entry cell. The three move columns go blank in that case, because with no
// rating there is nothing a move's exposure could have been measured against, and
// a 0 would assert a measurement that was never taken.
func TestTheFixtureRunColumnsAreBlankWithoutBands(t *testing.T) {
	cols := []string{
		"band_ready_weeks", "band_moves", "band_run_moves", "band_worse_moves",
		"band_exposure",
	}

	render := func(m FixtureRunMediator) []string {
		path := filepath.Join(t.TempDir(), "cells.csv")
		sink, err := openCellSink(path)
		if err != nil {
			t.Fatal(err)
		}
		row := cellRow{
			Sweep: sink.sweepLabel("T"), RunID: sink.run(), Variant: "v",
			Season: "2025-26", StartGW: 1, Weeks: 38,
			HasBanking:      true,
			BankingMediator: BankingMediator{DecisionWeeks: 30, FreeHeld: 30},
			FixtureRuns:     m,
		}
		sink.cell(row)
		sink.close()
		_, rows := readCells(t, path)
		if len(rows) != 1 {
			t.Fatalf("want 1 row, got %d", len(rows))
		}
		out := make([]string, 0, len(cols))
		for _, c := range cols {
			out = append(out, rows[0][c])
		}
		return out
	}

	got := render(FixtureRunMediator{})
	if want := []string{"0", "", "", "", ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("with the bands never ready the block rendered %v, want %v — "+
			"band_ready_weeks is a measurement and the other three are gaps",
			got, want)
	}
	got = render(FixtureRunMediator{
		ReadyWeeks: 24, Moves: 9, RunMoves: 4, WorseMoves: 2, Exposure: -3,
	})
	if want := []string{"24", "9", "4", "2", "-3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("a populated block rendered %v, want %v", got, want)
	}
}

// TestTheFixtureRunLeverReachesTheTransferDecision is the arrival check, and it
// is the one this whole change turns on.
//
// # Why a scoring-path arrival test is not enough
//
// TestBandStrengthArrivesOnTheScoredPath already establishes that BandStrength
// moves what a player is worth. That is a different claim from "it changes what
// the policy buys", and this project has shipped the gap between them: the
// original BandStrength refutation was measured on an arm where the hold engine
// was built from a fresh SimConfig that never saw the setting, so the hold
// baseline was byte-identical across that whole sweep BY CONSTRUCTION and only
// POLICY rows could move. A byte-identical result that was a comparison which
// never ran.
//
// So this asserts on the transfer decision's own output — the moves made and the
// points they produced — rather than on any player's score.
//
// # What a failure means
//
// Not "the bands are worthless". It means the setting does not arrive at
// `decide`, which would make every tandem arm a null by construction. The brief
// for this change said to stop and report exactly that rather than paper over it.
func TestTheFixtureRunLeverReachesTheTransferDecision(t *testing.T) {
	cur, prior, base := chipSim(t)

	run := func(strength float64) *SimResult {
		sc := base
		sc.Weights.BandStrength = strength
		res, err := Simulate(cur, prior, sc)
		if err != nil {
			t.Fatalf("simulate at band_strength %v: %v", strength, err)
		}
		return res
	}
	off, on := run(0), run(1)

	// The lever has to have had something to act on, or a difference in the moves
	// would be evidence of something else and an absence of one would be
	// unreadable. This is the guard TestBandStrengthArrivesOnTheScoredPath keeps
	// for the same reason.
	if off.FixtureRuns.ReadyWeeks == 0 {
		t.Fatal("the bands were never ready in this season, so this test cannot " +
			"tell a lever that does not arrive from one that had nothing to act on")
	}

	moveKey := func(res *SimResult) string {
		s := ""
		for _, mv := range res.Moves {
			s += fmt.Sprintf("%d:%s>%s;", mv.GW, mv.Out, mv.In)
		}
		return s
	}
	if moveKey(off) == moveKey(on) && off.Points == on.Points {
		t.Fatalf("band_strength 0 and 1 produced the same %d transfers and the same "+
			"%d points over %d decision weeks, %d of them with the bands ready.\n\n"+
			"The fixture-run lever is not reaching the transfer decision. It is read "+
			"in exactly one place — Engine.fixtureMultipliersFor, through "+
			"attackBandAdj and defenceBandAdj — and it gets there on "+
			"SimConfig.Weights, which Simulate hands to the transfer engine `pe`. "+
			"Check that chain before reading anything into any fixture-run "+
			"measurement: a setting that never arrives returns a byte-identical null "+
			"that looks exactly like a null meaning the knob does nothing.",
			len(off.Moves), off.Points, off.Banking.DecisionWeeks,
			off.FixtureRuns.ReadyWeeks)
	}
	t.Logf("band_strength 0 -> 1 over %d decision weeks (%d band-ready): "+
		"%d moves -> %d, %d points -> %d, exposure %+d -> %+d",
		off.Banking.DecisionWeeks, off.FixtureRuns.ReadyWeeks,
		len(off.Moves), len(on.Moves), off.Points, on.Points,
		off.FixtureRuns.Exposure, on.FixtureRuns.Exposure)
}

// TestTheTandemLeversAreExpressibleTogether is the point of the whole exercise.
//
// The reopened question is not "does the fixture lever work" — every recorded arm
// in that family says it does not resolve one at a time. It is whether banking, a
// planned chip and the fixture bands do something together that none does alone.
// This project's standing rule is that a one-at-a-time null is a *simple-effect*
// null, true of the shipped configuration and silent about any other, so the
// tandem is untested rather than refuted.
//
// This asserts the tandem can be *expressed*: one SimConfig carrying all three
// levers, each arriving at its own consumer, simulating to completion. It makes
// no points claim, and asserts nothing about what the tandem is worth.
//
// ⚠️ It checks each lever's MEDIATOR rather than its config field, which is the
// difference between "the setting was written" and "the setting ran".
func TestTheTandemLeversAreExpressibleTogether(t *testing.T) {
	cur, prior, base := chipSim(t)

	// All three levers in one arm. Each ships off, and each is settable by a user:
	// bank_transfers_lookahead, prepare_squad_for_chips and weights.band_strength.
	sc := base
	sc.BankLookahead = true
	sc.PrepareBenchBoost = true
	sc.PrepareTripleCaptain = true
	sc.Weights.BandStrength = 1
	// Both chips inside the FIRST set's window. 2025-26 is the only two-set
	// season and its first set expires after GW19, so a plan straddling that
	// boundary is rejected outright — which is a real rule and not a reason to
	// widen anything here. The tandem only needs a chip inside the horizon for
	// the preparation credit to be reachable at all.
	sc.Chips.BenchBoost = 12
	sc.Chips.TripleCaptain = 17

	res, err := Simulate(cur, prior, sc)
	if err != nil {
		t.Fatalf("the tandem arm did not simulate: %v", err)
	}

	// Banking arrived: the rule was actually asked.
	if res.Banking.ConsultedWeeks == 0 {
		t.Errorf("the banking lever was set and the rule was consulted in 0 of %d "+
			"decision weeks, so the tandem arm is not running the lever it names",
			res.Banking.DecisionWeeks)
	}
	// The fixture lever arrived: the bands existed to be acted on.
	if res.FixtureRuns.ReadyWeeks == 0 {
		t.Errorf("the fixture-run lever was set and the bands were ready in 0 of %d "+
			"decision weeks", res.Banking.DecisionWeeks)
	}
	// The chip lever had chips to prepare for. Without them chipCreditFor is a
	// no-op and this is a two-lever arm wearing a three-lever label.
	played := 0
	for _, w := range res.Weeks {
		if w.BenchBoost || w.TripleCaptain {
			played++
		}
	}
	if played != 2 {
		t.Errorf("the tandem arm planned two scoring chips and played %d, so the "+
			"chip-preparation lever had nothing to credit", played)
	}

	t.Logf("tandem: %d decision weeks, %d consulted, %d banked, %d band-ready, "+
		"%d moves (%d better, %d worse, exposure %+d), %d chips played",
		res.Banking.DecisionWeeks, res.Banking.ConsultedWeeks, res.Banking.BankedWeeks,
		res.FixtureRuns.ReadyWeeks, res.FixtureRuns.Moves, res.FixtureRuns.RunMoves,
		res.FixtureRuns.WorseMoves, res.FixtureRuns.Exposure, played)
}

// TestTheFixtureRunMediatorReadsTheEngineHorizon pins the one unit error this
// mediator can make.
//
// `bandExposureDelta` must read `e.Weights.Horizon` — the window `Metrics` scored
// the candidates over — and never a horizon of its own. `ApplyChipPlan` shortens
// the engine's horizon in the weeks before a chip, so a mediator carrying a second
// definition would report on a run nobody was scored against, in exactly the weeks
// the chip-preparation lever is active. That is the tandem arm, which is the arm
// this whole change exists to make measurable.
//
// Checked by construction rather than by replay: two engines at one cutoff
// differing only in Horizon must see different fixture windows.
func TestTheFixtureRunMediatorReadsTheEngineHorizon(t *testing.T) {
	cfg := loadConfig(t)
	cur, prior, _ := chipSim(t)

	short := sweepConfig(cfg, 12, false)
	short.Weights.Horizon = 1
	long := sweepConfig(cfg, 12, false)
	long.Weights.Horizon = 8

	se, _ := EngineAt(cur, prior, 11, short)
	le, _ := EngineAt(cur, prior, 11, long)
	if !se.BandChannelLive() || !le.BandChannelLive() {
		t.Skip("the bands are not ready at this cutoff, so there is nothing to count")
	}

	widened := 0
	for team := 1; team <= 20; team++ {
		// Position 3 (midfielder); the claim is about the window, not the band side.
		s := se.FixtureRunFor(team, se.Weights.Horizon, 3)
		l := le.FixtureRunFor(team, le.Weights.Horizon, 3)
		if l.Fixtures > s.Fixtures {
			widened++
		}
	}
	if widened == 0 {
		t.Fatalf("no club's fixture window widened between horizon %v and horizon "+
			"%v, so FixtureRunFor is not reading the horizon it is handed and "+
			"bandExposureDelta would report on a run the engine never scored",
			se.Weights.Horizon, le.Weights.Horizon)
	}
}

// TestTheSweepAssignsBothMediators closes the join at the level the CSV test
// above cannot reach.
//
// # Why a second test, and why it reads source
//
// TestTheSweepWritesTheFixtureRunBlock puts a real SimResult through
// `fixtureRunsOf` and the sink, which is what the sweep does with it — but it
// performs that composition itself. So deleting the sweep's own assignment leaves
// it passing, and every cell of every sweep would carry five blanks while the
// package stayed green. That is precisely the hole a review found in the banking
// block one commit earlier, and TestTheSweepWritesTheBankingBlock has the same
// shape and therefore the same gap.
//
// `runPolicySweep` lives in a _test.go file, so it is unreachable from a normal
// gate test and `funcBodyCalls` skips test files by design. A source scan is the
// remaining instrument, and it is this package's established one —
// TestEveryScoringEngineGetsRecency counts engines by reading simulate.go as text
// for the same reason.
//
// ⚠️ **A scan is a tripwire, not a proof.** It matches the seam by name, so a
// rename satisfies it only if the name is updated deliberately, and an assignment
// spelled another way would slip past. That is the standing caveat on every scan
// here: it stops the deletion that has actually happened rather than every
// deletion imaginable.
func TestTheSweepAssignsBothMediators(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	body := ""
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name.Name != "runPolicySweep" || fd.Body == nil {
				continue
			}
			// Sliced from the raw source by the body's own offsets, so comments
			// elsewhere in the file cannot satisfy the guard that watches the
			// wiring — the failure mode funcBodyCalls' doc comment names.
			body = string(src[fset.Position(fd.Body.Pos()).Offset:fset.Position(fd.Body.End()).Offset])
		}
	}
	if body == "" {
		t.Fatal("no runPolicySweep in this package — the guard is following a seam " +
			"that has been renamed or removed, so update it deliberately rather " +
			"than letting it pass vacuously")
	}

	// Both mediators, asserted together. They are one region of the cells file and
	// they fail the same way, so a guard watching only the newer one would leave
	// the older join exactly as exposed as it was before.
	// The two original mediators, plus the five that landed with the option-value
	// levers and the dose. Every one of them fails the same way — blank columns,
	// a sweep that prints and banks normally, no other test executing the line —
	// so they are watched together. ⚠️ The five new ones are named INDIVIDUALLY
	// rather than as one block assignment, because the four levers are
	// independently switchable and a guard that accepted any one of them would
	// let the other three go silently missing.
	for _, want := range []string{
		"bankingOf(res)", "fixtureRunsOf(res)",
		"res.TransferHold", "res.Wildcard", "res.BenchBoost", "res.FreeHit",
		"res.ChipPrep", "DoseFor(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("runPolicySweep does not call %s.\n\n"+
				"The mediator is not reaching the cellRow, so its columns will be "+
				"blank in every cell of every sweep while the sweep prints and banks "+
				"normally — and no other test in this package executes that line. A "+
				"mediator that is silently absent is worse than none: it turns every "+
				"arm's null from readable back into ambiguous, which is the entire "+
				"reason both blocks exist.", want)
		}
	}
}
