package backtest

// Unit tests for the oracle types and the sweep rules built on them.
//
// None of this needs the archive or the network: it is about what an oracle
// *declares*, which is the half of the design that has to be right before any
// replay is worth running.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"armband/internal/config"
)

// TestOracleStampIsCanonicalAndNeverBlank pins the join key.
//
// The stamp is written into the per-cell CSV, the means file and every arm label,
// and R joins on it. A stamp that varied with construction order, or that came
// back blank, would silently split one arm's rows into two groups or file them
// under "not measured".
func TestOracleStampIsCanonicalAndNeverBlank(t *testing.T) {
	if got := (Oracles{}).Stamp(); got != "-" {
		t.Errorf("inert stamp is %q, want %q — blank means 'not measured' "+
			"everywhere else in this schema", got, "-")
	}
	if got := (Oracles{}).Kind(); got != "none" {
		t.Errorf("inert kind is %q, want %q", got, "none")
	}
	both := Oracles{Info: OracleAvailability | OracleTransactPrice}
	reversed := Oracles{Info: OracleTransactPrice | OracleAvailability}
	if both.Stamp() != reversed.Stamp() {
		t.Errorf("stamp depends on construction order: %q against %q",
			both.Stamp(), reversed.Stamp())
	}
	for _, c := range []struct {
		o    Oracles
		want string
		kind string
	}{
		{Oracles{Info: OracleAvailability}, "info:availability", "info"},
		{Oracles{Info: OracleTransactPrice}, "info:prices", "info"},
		{both, "info:availability+prices", "info"},
		{Oracles{Decision: AxisChipWeek}, "decision:chipweek", "decision"},
		{Oracles{Info: OracleAvailability, Decision: AxisChipWeek, Composite: true},
			"info:availability|decision:chipweek", "composite"},
	} {
		if got := c.o.Stamp(); got != c.want {
			t.Errorf("stamp %q, want %q", got, c.want)
		}
		if got := c.o.Kind(); got != c.kind {
			t.Errorf("kind %q, want %q", got, c.kind)
		}
	}
}

// TestOracleValidateRefusesWhatItCannotMeasure is the anti-no-op guard on the
// type itself.
//
// The failure it prevents is the one this project ships most often: a switch that
// is wired, stamped, reported and inert. An arm carrying an oracle bit with no
// hook behind it would produce a full table of plausible numbers, a mean, a
// standard error and a verdict — all of them measuring the baseline against
// itself. That is indistinguishable from a real null, which is why it must be an
// error rather than a comment.
func TestOracleValidateRefusesWhatItCannotMeasure(t *testing.T) {
	for _, o := range []Oracles{
		{},
		{Info: OracleAvailability},
		{Info: OracleTransactPrice},
		{Info: OracleAvailability | OracleTransactPrice},
		{Decision: AxisChipWeek},
		{Info: OracleAvailability, Decision: AxisChipWeek, Composite: true},
	} {
		if err := o.Validate(); err != nil {
			t.Errorf("%s must be valid: %v", o.Stamp(), err)
		}
	}

	// A bit with no implementation. Written as a raw cast because there is no
	// constant for it — which is the point: the catalogue's remaining oracles do
	// not exist as constants until their hooks do.
	unimplemented := Oracles{Info: InfoOracle(1) << 20}
	if err := unimplemented.Validate(); err == nil {
		t.Error("an information bit with no hook must be refused; a stamped arm " +
			"that measures nothing reports a clean null")
	}
	// And it still stamps honestly rather than silently omitting the bit, because
	// Stamp is called from reporting paths that do not validate.
	if !strings.Contains(unimplemented.Stamp(), "unknown") {
		t.Errorf("an unimplemented bit stamps as %q, hiding itself in the one "+
			"column that exists to prevent that", unimplemented.Stamp())
	}

	// An axis with no hook must fail loudly rather than run the baseline under a
	// "decision:" label. A raw cast for the same reason as the bit above: the
	// catalogue's remaining axes do not exist as constants until their hooks do.
	if err := (Oracles{Decision: DecisionAxis(1 << 20)}).Validate(); err == nil {
		t.Error("a decision axis with no hook must be refused")
	}
	// Composite is refused with nothing to compose...
	if err := (Oracles{Info: OracleAvailability, Composite: true}).Validate(); err == nil {
		t.Error("Composite with no decision oracle must be refused")
	}
	if err := (Oracles{Decision: AxisChipWeek, Composite: true}).Validate(); err == nil {
		t.Error("Composite with no information oracle must be refused")
	}
	// ...and it is *required* in the other direction, which only became
	// expressible once an axis existed. An arm mixing better data with better
	// judgement bounds neither, so it must be deliberate rather than accidental.
	if err := (Oracles{Info: OracleAvailability, Decision: AxisChipWeek}).Validate(); err == nil {
		t.Error("information plus decision without Composite must be refused: such " +
			"a figure bounds neither better data nor better judgement")
	}
}

// TestNoInertDecisionAxes is TestNoInertOracleConstants for the other half of the
// type: every declared axis must be implemented and must have a stamp name.
//
// An axis constant with no hook is the worse of the two silent no-ops, because a
// DecisionAxis is a plain enum — `Oracles{Decision: AxisStartingXI}` would compile,
// validate, stamp an arm "decision:startingxi" and reproduce the baseline exactly.
func TestNoInertDecisionAxes(t *testing.T) {
	for axis := AxisNone + 1; axis < AxisNone+64; axis++ {
		_, named := axisName[axis]
		if implementedAxes[axis] != named {
			t.Errorf("axis %d: implemented=%v named=%v — an implemented axis with "+
				"no name stamps as a number, and a named axis with no hook stamps "+
				"an arm that measures nothing", int(axis), implementedAxes[axis], named)
		}
	}
	if len(implementedAxes) == 0 {
		t.Error("no decision axis is implemented; this test is then vacuous and " +
			"should be deleted along with the type")
	}
}

// TestSimulateRefusesAnUnimplementedOracle carries the same guard to the entry
// point, because a value that never reaches Validate is not validated.
func TestSimulateRefusesAnUnimplementedOracle(t *testing.T) {
	cur, prior := twoSeasonsForOracleTest()
	cfg := SimConfig{Budget: 1000, BankUpTo: 1, Oracles: Oracles{Info: InfoOracle(1) << 20}}
	if _, err := Simulate(cur, prior, cfg); err == nil {
		t.Fatal("Simulate must refuse an oracle bit with no hook, the same way it " +
			"refuses an illegal chip plan")
	}
}

// TestNoInertOracleConstants counts the declared constants against the
// implemented set.
//
// The oracle-design document lists five more oracles, and the temptation is to declare
// their constants now so the catalogue reads completely. That is exactly the
// silent no-op: `Oracles{Info: OracleMinutes}` would compile, stamp, sweep and
// change nothing. So every declared bit must be implemented, have a name, and
// have a place in the canonical order.
func TestNoInertOracleConstants(t *testing.T) {
	for _, bit := range infoOrder {
		if implementedInfo&bit == 0 {
			t.Errorf("%s is in the stamp order and not in the implemented set", infoName[bit])
		}
		if infoName[bit] == "" {
			t.Errorf("bit %#b has no stamp name", uint(bit))
		}
	}
	// Every implemented bit is in the order, or its name would never appear in a
	// stamp and a composite arm would be described as a lesser one.
	var covered InfoOracle
	for _, bit := range infoOrder {
		covered |= bit
	}
	if covered != implementedInfo {
		t.Errorf("implemented set %#b, stamp order covers %#b — a bit missing "+
			"from the order stamps as nothing", uint(implementedInfo), uint(covered))
	}
	if len(infoName) != len(infoOrder) {
		t.Errorf("%d names for %d bits", len(infoName), len(infoOrder))
	}
}

// TestEveryOracleSaysWhatItBounds is the coverage check for the banner.
//
// It is not cosmetic. The banner is the one place a reader is told what class of
// bound the table below is, and oracleBounds falls through to "bounds nothing
// declared" for anything it has no sentence for — so an oracle shipped without one
// prints a *confident denial* above its own figure. OracleMinutes did exactly that
// for the length of one commit: implemented, stamped, live, and announced as
// bounding nothing.
//
// The same class as the constants above, one layer out: there the silent no-op is
// an arm that measures nothing, here it is an arm that measures something and says
// it does not.
func TestEveryOracleSaysWhatItBounds(t *testing.T) {
	const fallback = "bounds nothing declared"
	for _, bit := range infoOrder {
		if got := oracleBounds(Oracles{Info: bit}); got == fallback {
			t.Errorf("info oracle %q has no sentence in oracleBounds, so its own "+
				"sweep banner denies that it bounds anything", infoName[bit])
		}
	}
	for axis := range implementedAxes {
		if got := oracleBounds(Oracles{Decision: axis}); got == fallback {
			t.Errorf("decision axis %q has no sentence in oracleBounds", axisName[axis])
		}
	}
	// And the fallback still fires for an inert value, or the check above would
	// pass against an oracleBounds that had stopped having a fallback at all.
	if got := oracleBounds(Oracles{}); got != fallback {
		t.Errorf("an inert Oracles reads %q, want %q", got, fallback)
	}
}

// TestEveryInformationOracleHasAnInputDiff is the coverage check for Tier 1.
//
// Tier 1 is the cheap guarantee — seconds, no replay — so it is the one an author
// under time pressure skips. A bit with no case there ships with no statement at
// all about what it may perturb, and the first thing anyone would notice is a
// figure that bounds the wrong quantity.
func TestEveryInformationOracleHasAnInputDiff(t *testing.T) {
	covered := map[InfoOracle]bool{}
	for _, c := range tierOneCases {
		covered[c.oracle] = true
	}
	for _, bit := range infoOrder {
		if !covered[bit] {
			t.Errorf("%s has no case in tierOneCases, so nothing says which "+
				"fields it may perturb", infoName[bit])
		}
	}
}

// TestMustNotMoveIsIntersectedNotUnioned pins the one piece of arithmetic in the
// declaration that is easy to get backwards.
//
// Each oracle declares what *it* cannot move. With availability and prices both
// on, the held rungs move legitimately, because availability changes which
// fifteen is bought — so a union of the two declarations would fail a correct run
// and, worse, teach whoever hit it to delete the check. The intersection is the
// honest combined claim, and it says the combined arm has no cell invariance at
// all, which is worth knowing before reading one.
func TestMustNotMoveIsIntersectedNotUnioned(t *testing.T) {
	price := Oracles{Info: OracleTransactPrice}.MustNotMove()
	if len(price) == 0 {
		t.Fatal("the price oracle must declare the held rungs; that declaration " +
			"is the check which proved it reaches only the transfer path")
	}
	if got := (Oracles{Info: OracleAvailability}).MustNotMove(); len(got) != 0 {
		t.Errorf("the availability oracle declares %v and legitimately moves "+
			"every metric — a false invariance fails correct runs", got)
	}
	both := Oracles{Info: OracleAvailability | OracleTransactPrice}.MustNotMove()
	if len(both) != 0 {
		t.Errorf("availability+prices declares %v; the combination must be the "+
			"intersection of the two, not the union", both)
	}
}

// TestMustNotMoveNamesRealColumns is the mirror check between three expressions
// of one quantity: the declaration, the CSV header, and the series the sweep
// actually collects.
//
// This package's most-repeated bug is two expressions of one thing drifting
// apart — DefaultBenchWeight against Weights.BenchWeight, fixtureSensitivePart
// against baseXP90 — and here the drift would be silent in the worst direction: a
// renamed column would leave the invariance unchecked while every sweep still
// printed a full table.
func TestMustNotMoveNamesRealColumns(t *testing.T) {
	inHeader := map[string]bool{}
	for _, h := range cellHeader {
		inHeader[h] = true
	}
	series := invarianceSeries(nil, nil, nil, nil, nil, nil, nil, nil)
	check := func(who string, cols []string) {
		for _, col := range cols {
			if !inHeader[col] {
				t.Errorf("%s declares %q, which is not a column in the per-cell "+
					"CSV — the declaration and the schema have drifted", who, col)
			}
			if _, ok := series[col]; !ok {
				t.Errorf("%s declares %q, which runPolicySweep does not collect, "+
					"so the invariance is declared and never checked", who, col)
			}
		}
	}
	for _, bit := range infoOrder {
		check(infoName[bit], (Oracles{Info: bit}).MustNotMove())
	}
	for axis := range implementedAxes {
		check(axisName[axis], (Oracles{Decision: axis}).MustNotMove())
	}
}

// TestEveryDecisionAxisDeclaresAnInvariance closes the hole that let one axis
// ship with no Tier 2 check at all.
//
// AxisTransferGateXPoints was added to implementedAxes, to axisName, to
// oracleBounds and to the gate's switch — and its case in mustNotMoveForAxis was
// missed, so it fell through to the default's nil. Every guard passed:
// TestMustNotMoveNamesRealColumns validates the names of whatever is declared and
// an empty list has no names to fail on, oracleInvarianceViolations loops over an
// empty set and reports nothing, and the console banner announced the arm as
// resting on the input diff — which covers InfoOracle only and structurally cannot
// cover a decision axis. An arm announced as checked by a check that does not exist
// for it is worse than one announced as unchecked.
//
// A blanket rule is right here where it is wrong for MustMove: the chip-week axis
// forced MustMove to allow empty, because its whole output is a recorded slice and
// it declares that *everything* must stay put. There is no counterpart on the
// must-not side. Every axis in the catalogue changes some decision or reads some
// hindsight, so every one of them can name at least one metric it cannot reach —
// and an axis that genuinely could reach every collected column would be an
// omniscient arm, which Reportable already refuses.
func TestEveryDecisionAxisDeclaresAnInvariance(t *testing.T) {
	for axis := range implementedAxes {
		if len((Oracles{Decision: axis}).MustNotMove()) == 0 {
			t.Errorf("decision axis %q declares no invariance, so Tier 2 checks "+
				"nothing for any arm that runs it and passes vacuously. Add its "+
				"case to mustNotMoveForAxis: an unchecked arm reports the same "+
				"clean table as a checked one", axisName[axis])
		}
	}
}

// TestEveryDeclarableColumnIsCollected pins the two expressions of "what an
// oracle may pin" against each other.
//
// cellMetricColumns lives in oracle.go, beside the declarations; invarianceSeries
// lives in the sweep that builds the series. A column in one and not the other is
// silent in both directions and worse in one: a column declarable but uncollected
// makes MustNotMove name an invariance nobody checks, and the sweep still prints a
// complete table.
func TestEveryDeclarableColumnIsCollected(t *testing.T) {
	series := invarianceSeries(nil, nil, nil, nil, nil, nil, nil, nil)
	if len(series) != len(cellMetricColumns) {
		t.Errorf("runPolicySweep collects %d series and cellMetricColumns names %d",
			len(series), len(cellMetricColumns))
	}
	for _, col := range cellMetricColumns {
		if _, ok := series[col]; !ok {
			t.Errorf("cellMetricColumns names %q and runPolicySweep does not "+
				"collect it, so an oracle could declare an invariance that is "+
				"never checked", col)
		}
	}
	for col := range series {
		found := false
		for _, c := range cellMetricColumns {
			if c == col {
				found = true
			}
		}
		if !found {
			t.Errorf("runPolicySweep collects %q and cellMetricColumns does not "+
				"name it, so no oracle can pin it", col)
		}
	}
}

// TestLivenessDeclarationsAreCoherent pins the three properties MustMove has to
// have to be worth checking at all.
//
// It may only name a column the sweep collects, or it declares a liveness claim
// nobody checks — the same hole TestEveryDeclarableColumnIsCollected closes for
// MustNotMove. It may never name a column the same oracle pinned, or the arm is
// required to move something it is required to leave alone and no run can pass.
// And the price oracle must actually declare one, because it is the arm the
// mechanism exists for: no bootstrap fields, a must-not set, a diagnostic that
// asserts nothing, and a headline that is a null.
func TestLivenessDeclarationsAreCoherent(t *testing.T) {
	series := invarianceSeries(nil, nil, nil, nil, nil, nil, nil, nil)
	all := []Oracles{
		{Info: OracleAvailability}, {Info: OracleTransactPrice}, {Info: OracleMinutes},
		{Decision: AxisChipWeek}, {Decision: AxisArmband}, {Decision: AxisTransferGate},
		{Info: OracleAvailability | OracleTransactPrice},
	}
	for _, o := range all {
		pinned := map[string]bool{}
		for _, c := range o.MustNotMove() {
			pinned[c] = true
		}
		for _, c := range o.MustMove() {
			if _, ok := series[c]; !ok {
				t.Errorf("%s declares %q must move and no sweep collects it",
					o.Stamp(), c)
			}
			if pinned[c] {
				t.Errorf("%s declares %q must move AND must not move — no run can "+
					"pass, and the contradiction is invisible until an arm is built",
					o.Stamp(), c)
			}
		}
	}
	if len(Oracles{Info: OracleTransactPrice}.MustMove()) == 0 {
		t.Error("the price oracle has no liveness declaration. It is the one " +
			"implemented oracle with no other evidence it is doing anything: no " +
			"bootstrapFields for Tier 1, a must-NOT set for Tier 2, a diagnostic " +
			"that asserts nothing, and a null for a headline — which is exactly " +
			"what an inert arm reports")
	}
	// The chip-week axis pins every column, so a blanket "something must move"
	// rule would fail the one arm whose invariance is total. Its liveness lives in
	// its observe hook instead, and this records that the omission is deliberate.
	if got := (Oracles{Decision: AxisChipWeek}).MustMove(); len(got) != 0 {
		t.Errorf("AxisChipWeek declares %v must move, but it declares every column "+
			"must NOT move — it changes no decision by construction", got)
	}
}

// TestAnInertOracleArmIsReported drives the liveness checker directly, because a
// guard whose only exercise is the path it guards is a guard nobody has seen fire.
func TestAnInertOracleArmIsReported(t *testing.T) {
	arms := []policyVariant{
		{label: "real (ships)"},
		oracleVariant(Oracles{Info: OracleTransactPrice}, "perfect timing", nil),
	}
	same := []map[string]float64{
		{"a@1": 10, "b@1": 12},
		{"a@1": 10, "b@1": 12},
	}
	series := invarianceSeries(nil, nil, nil, nil, nil, nil, same, nil)
	if got := oracleLivenessViolations(arms, series); len(got) != 1 {
		t.Errorf("an oracle arm that changed no transfer count in any cell must be "+
			"reported as inert; got %v", got)
	}
	moved := []map[string]float64{
		{"a@1": 10, "b@1": 12},
		{"a@1": 10, "b@1": 13},
	}
	if got := oracleLivenessViolations(arms, invarianceSeries(nil, nil, nil, nil, nil, nil, moved, nil)); len(got) != 0 {
		t.Errorf("one differing cell is liveness; got %v", got)
	}
	// A declared column nobody collects is a failure, not a skip — the same rule
	// the invariance side follows, for the same reason.
	if got := oracleLivenessViolations(arms, map[string][]map[string]float64{}); len(got) != 1 {
		t.Errorf("a liveness claim on an uncollected column must fail loudly; got %v", got)
	}
}

// TestOracleColumnsAreLastAndCounted is the schema check a version bump alone
// would not catch.
//
// The cells file is append-only and its header is compared on open, so the shape
// of the schema *is* the compatibility contract. oracleCols exists so the
// stale-header regression test can synthesise this build's predecessor by
// stripping exactly the block that was appended — which is only true while the
// block really is last and really is that long.
func TestOracleColumnsAreLastAndCounted(t *testing.T) {
	want := []string{"oracle", "oracle_kind"}
	if oracleCols != len(want) {
		t.Fatalf("oracleCols is %d and the block is %d columns", oracleCols, len(want))
	}
	got := cellHeader[len(cellHeader)-oracleCols:]
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the last %d columns are %v, want %v — stripping oracleCols "+
				"no longer yields the predecessor header", oracleCols, got, want)
		}
	}
	if meansHeader[len(meansHeader)-1] != "oracle" {
		t.Errorf("the means file does not end with the stamp: %v", meansHeader)
	}
}

// TestUnstampedCellRowWritesTheInertPair pins "never blank".
//
// Blank means "not measured" in this schema — that is what the layer and rung
// columns use it for — and every row does know its oracle state. A blank here
// would read downstream as a cell whose provenance was unrecorded, which is a
// different and more alarming claim than "no hindsight".
func TestUnstampedCellRowWritesTheInertPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cells.csv")
	sink, err := openCellSink(path)
	if err != nil {
		t.Fatal(err)
	}
	sink.cell(sampleRow(sink.sweepLabel("T"), sink.run(), "shipped", 0, "2025-26", 1))
	sink.close()

	_, rows := readCells(t, path)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0]["oracle"] != "-" || rows[0]["oracle_kind"] != "none" {
		t.Fatalf("unstamped row carries %q/%q, want %q/%q",
			rows[0]["oracle"], rows[0]["oracle_kind"], "-", "none")
	}
	// And an infeasible cell keeps its stamp: it describes the arm, not the
	// measurement, so an oracled arm's failed cell is still an oracled cell.
	inf := sampleRow("T#1", "r", "v", 1, "2025-26", 1).
		under(Oracles{Info: OracleAvailability}).asInfeasible()
	if o, k := inf.stamp(); o != "info:availability" || k != "info" {
		t.Fatalf("an infeasible cell lost its stamp: %q/%q", o, k)
	}
}

// TestEveryCellRowIsStamped scans source rather than behaviour, the way
// TestInferenceLivesInOnePlace does, because the failure mode is a *new*
// diagnostic quietly emitting cells with no provenance.
//
// A row built without .under() still writes "-", which is correct for an
// ordinary sweep and a **lie** for one whose config came from sweepConfig with
// FPL_ORACLE_PRICES set in the environment. No runtime assertion can see that: the
// row is well-formed and the number is plausible.
func TestEveryCellRowIsStamped(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// The file that defines cellRow legitimately constructs and manipulates one
	// without stamping, and this file constructs unstamped rows on purpose.
	exempt := map[string]bool{
		"cellcsv_test.go":            true,
		"cellcsv_regression_test.go": true,
		"oracle_test.go":             true,
	}
	for _, f := range files {
		if exempt[filepath.Base(f)] {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if !strings.Contains(src, "cellRow{") {
			continue
		}
		if !strings.Contains(src, ".under(") {
			t.Errorf("%s builds a cellRow and never stamps it with .under(sc.Oracles) — "+
				"its cells would claim no hindsight whatever the config carried", f)
		}
	}
}

// TestNothingInAnalysisKnowsAboutOracles enforces the design's hard boundary as a
// mechanism rather than as a comment.
//
// The oracle-design document and `oracle.go`'s file comment both state it: **oracles
// live in internal/backtest and nowhere else**, because `internal/analysis` is the
// shipped scoring engine and a hindsight hook in it would be a hook the live agent
// runs. The design explicitly asks for confinement to be a mechanism, and it was
// true today only by grep — which is the same standing this project's other
// hand-maintained invariants had immediately before they rotted.
//
// It has already been pushed on twice. The design's claim that `PointInTime` is
// the single information seam failed against the shipped code for `Engine.Recent`
// and again for `Engine.Priors`, and both times the pressure was toward hooking
// `internal/analysis` directly. Both were resolved on this side of the boundary
// instead — `newRecentIndexWith` and `newPriorIndexMulti` are manufactured here —
// and that is the outcome this test exists to keep.
//
// Test files there are excluded on purpose: "oracle" has a second, standard
// meaning in testing (a frozen reference implementation), and
// `optimizerdiff_test.go` uses it correctly in that sense. Firing on that would
// get the guard deleted, and then it guards nothing.
func TestNothingInAnalysisKnowsAboutOracles(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "analysis", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no files found in internal/analysis — this guard is looking at " +
			"the wrong place and would pass vacuously")
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if strings.Contains(strings.ToLower(src), "oracle") {
			t.Errorf("%s mentions an oracle. internal/analysis is the SHIPPED scoring "+
				"engine: a hindsight hook there is one the live agent runs, and it "+
				"would make every oracle figure a bound on a model nobody plays. An "+
				"information oracle must be expressible as a perturbation of what "+
				"this package manufactures — the bootstrap, the recency index or the "+
				"prior index — which is where the three that exist all live",
				filepath.Base(f))
		}
		if strings.Contains(src, "armband/internal/backtest") {
			t.Errorf("%s imports internal/backtest, which inverts the dependency the "+
				"whole confinement rests on", filepath.Base(f))
		}
	}
}

// TestEveryMeansRowIsStampedFromWhatRan is the same scan one file along, and it
// exists because the cells half of the guard passed while the means half did not.
//
// TestDiagVarianceDecomposition stamped its *cells* from sc.Oracles — which folds
// OraclesFromEnv — and its *means* from a hardcoded Oracles{}.Stamp(). So an
// oracled run wrote a cells file saying "info:prices" and a means file saying "-"
// for the same numbers, in the one column that exists to stop a hindsight bound
// being read as a score. No runtime assertion can see it: both rows are
// well-formed and neither number is implausible.
//
// The rule is the same one the whole Oracles-on-SimConfig design buys: a stamp is
// *derived* from the value the simulation consumed, never typed beside it. So a
// literal in the stamp position is the defect, wherever it appears.
func TestEveryMeansRowIsStampedFromWhatRan(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// The two files that test the sink itself construct rows by hand on purpose,
	// including inert ones — that is what they are for.
	exempt := map[string]bool{
		"cellcsv_test.go":            true,
		"cellcsv_regression_test.go": true,
	}
	// Parsed rather than grepped. A substring search matches the *prose* above,
	// which describes the defect in order to explain the guard — and a check a
	// comment can trip is a check somebody eventually deletes.
	fset := token.NewFileSet()
	for _, f := range files {
		if exempt[filepath.Base(f)] {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "mean" || len(call.Args) == 0 {
				return true
			}
			stamp := call.Args[len(call.Args)-1]
			ast.Inspect(stamp, func(m ast.Node) bool {
				lit, ok := m.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if id, ok := lit.Type.(*ast.Ident); ok && id.Name == "Oracles" {
					t.Errorf("%s:%d stamps a means row from a literal Oracles{} — its "+
						"cells are stamped from the config that ran, so an oracled run "+
						"would disagree with itself across two files. Derive the stamp "+
						"from the arm's own Oracles, as runPolicySweep does",
						filepath.Base(f), fset.Position(m.Pos()).Line)
				}
				return true
			})
			return true
		})
	}
}

// TestOracleArmRulesAreEnforced exercises the three refusals a sweep makes before
// it spends an hour replaying anything.
func TestOracleArmRulesAreEnforced(t *testing.T) {
	avail := Oracles{Info: OracleAvailability}
	ok := []policyVariant{
		{label: "real (ships)"},
		oracleVariant(avail, "perfect team news", nil),
	}
	// oracleVariant's own label is what the rule wants, so the shipped shape
	// passes without anyone typing a prefix.
	if err := validateOracleArms(ok); err != nil {
		t.Fatalf("the ordinary two-arm oracle sweep must be accepted: %v", err)
	}

	oracledBaseline := []policyVariant{
		oracleVariant(avail, "perfect team news", nil),
		{label: "real"},
	}
	oracledBaseline[0].oracles = avail
	if err := validateOracleArms(oracledBaseline); err == nil {
		t.Error("an oracled baseline must be refused — every difference this " +
			"harness prints is paired against variants[0], so the sign of one " +
			"measured against another oracle means nothing")
	}

	unlabelled := []policyVariant{
		{label: "real"},
		{label: "hindsight", oracles: avail},
	}
	if err := validateOracleArms(unlabelled); err == nil {
		t.Error("an oracled arm whose label does not say so must be refused")
	}

	claiming := []policyVariant{
		{label: "real"},
		{label: "ORACLE[info:availability] but not really"},
	}
	if err := validateOracleArms(claiming); err == nil {
		t.Error("an arm that claims an oracle it does not install must be refused")
	}
}

// TestAStrayEnvironmentOracleIsRefusedRatherThanInherited pins what happens when
// somebody runs an ordinary sweep with FPL_ORACLE_PRICES still exported from the
// last one.
//
// Two behaviours were available and only one of them is safe *and* loud.
// Silently clearing the seed inside the sweep would protect the numbers and turn
// a documented switch into a mystery — the reader would see it in
// docs/replay.md, set it, and get the baseline with nothing saying so. Honouring
// it oracles every arm including the baseline, which the baseline rule then
// refuses by name. So an oracle can only enter a sweep as a declared arm, and
// trying to smuggle one in through the environment stops the run.
func TestAStrayEnvironmentOracleIsRefusedRatherThanInherited(t *testing.T) {
	t.Setenv("FPL_ORACLE_PRICES", "1")
	base := policyVariant{label: "real (ships)", apply: func(sc *SimConfig) {}}
	got := oraclesOf(config.Default(), 1, base)
	if !got.Has(OracleTransactPrice) {
		t.Fatal("FPL_ORACLE_PRICES no longer reaches a sweep cell config, so the " +
			"switch is inert on the diagnostics while still documented")
	}
	base.oracles = got
	if err := validateOracleArms([]policyVariant{base}); err == nil {
		t.Fatal("a sweep whose baseline is oracled by the environment must be " +
			"refused — every paired difference it printed would be hindsight " +
			"measured against hindsight")
	}
}

// TestOracleVariantStampsWhatItInstalls pins the property the whole config-field
// design exists to buy.
//
// With an environment variable, the label and the behaviour are two mechanisms
// that have to be kept in step by hand. Here they come from one value, and the
// sweep re-reads that value out of the config every cell.
func TestOracleVariantStampsWhatItInstalls(t *testing.T) {
	o := Oracles{Info: OracleTransactPrice}
	v := oracleVariant(o, "perfect timing", func(sc *SimConfig) { sc.MinGain = 1.5 })
	if !strings.HasPrefix(v.label, "ORACLE[info:prices] ") {
		t.Errorf("label %q does not carry the stamp", v.label)
	}
	var sc SimConfig
	v.apply(&sc)
	if sc.Oracles != o {
		t.Errorf("apply installed %s, label says %s", sc.Oracles.Stamp(), o.Stamp())
	}
	if sc.MinGain != 1.5 {
		t.Error("the caller's own apply did not run")
	}
}

// TestInvarianceViolationsAreReported drives the Tier 2 checker over synthetic
// cells, because the real thing costs a 24-cell sweep and the *checker* is what
// needs testing.
func TestInvarianceViolationsAreReported(t *testing.T) {
	variants := []policyVariant{
		{label: "real (ships)"},
		oracleVariant(Oracles{Info: OracleTransactPrice}, "perfect timing", nil),
	}
	variants[1].oracles = Oracles{Info: OracleTransactPrice}

	clean := map[string]float64{"2024-25@1": 50.0, "2025-26@1": 48.5}
	hold := []map[string]float64{clean, {"2024-25@1": 50.0, "2025-26@1": 48.5}}
	policy := []map[string]float64{clean, {"2024-25@1": 55.0, "2025-26@1": 52.0}}
	// `hold_xpoints` is collected here because OracleTransactPrice declares it —
	// the held fifteen is out of a price oracle's reach on either reading of it.
	// Leaving it out is not a smaller test, it is the "declared but uncollected"
	// failure below, which is why this map has to grow whenever a declaration does.
	series := map[string][]map[string]float64{
		"policy_points":        policy,
		"hold_points":          hold,
		"hold_fixedcap_points": hold,
		"hold_nocap_points":    hold,
		"hold_xpoints":         hold,
	}
	if v := oracleInvarianceViolations(variants, series); len(v) != 0 {
		t.Fatalf("a clean run reported violations: %v", v)
	}

	// One rung moves by a tenth of a point per gameweek — far below anything a
	// sweep could resolve, and a categorical failure rather than a small effect.
	leaked := map[string][]map[string]float64{
		"policy_points":        policy,
		"hold_points":          {clean, {"2024-25@1": 50.1, "2025-26@1": 48.5}},
		"hold_fixedcap_points": hold,
		"hold_nocap_points":    hold,
		"hold_xpoints":         hold,
	}
	v := oracleInvarianceViolations(variants, leaked)
	if len(v) != 1 || !strings.Contains(v[0], "hold_points") {
		t.Fatalf("a leaking oracle must be named: %v", v)
	}

	// A declaration this sweep does not collect is a failure, not a skip: an
	// unchecked invariance looks exactly like a passing one.
	missing := map[string][]map[string]float64{"policy_points": policy}
	if v := oracleInvarianceViolations(variants, missing); len(v) == 0 {
		t.Fatal("a declared column with no collected series must fail")
	}
}

// TestChipWeekOracleReadsAndDoesNotDecide pins the two claims AxisChipWeek makes:
// that it picks the best week, and that picking it cannot reach anything else.
//
// The second half is the load-bearing one. This axis declares that *every*
// collected metric must be byte-identical to the baseline, which is the strongest
// invariance in the catalogue — and an invariance is only worth declaring if the
// thing it constrains is structurally incapable of violating it. placeChips takes
// a finished []Week and returns a value; it is handed no squad, no wallet and no
// engine, so there is nothing for it to change.
func TestChipWeekOracleReadsAndDoesNotDecide(t *testing.T) {
	weeks := []Week{
		{GW: 5, BenchBoostGain: 3, TripleCaptainGain: 11},
		{GW: 6, BenchBoostGain: 17, TripleCaptainGain: 2},
		{GW: 7, BenchBoostGain: 9, TripleCaptainGain: 11},
	}
	before := append([]Week(nil), weeks...)
	got := placeChips(weeks, bestChipWeek)
	if got.BenchBoost != (ChipWeek{GW: 6, Gain: 17}) {
		t.Errorf("bench boost placed at %+v, want GW6 for 17", got.BenchBoost)
	}
	// First week on a tie, so a chip is never credited to a later week than it had
	// to be played in — the honest reading when two weeks are worth the same.
	if got.TripleCaptain != (ChipWeek{GW: 5, Gain: 11}) {
		t.Errorf("triple captain placed at %+v, want GW5 for 11 (first on a tie)",
			got.TripleCaptain)
	}
	if !reflect.DeepEqual(weeks, before) {
		t.Fatalf("placeChips mutated the season it was handed:\n%+v\n%+v", before, weeks)
	}
	// An empty season places nothing rather than claiming GW0 for zero points,
	// which would read downstream as a chip that was worth nothing.
	if e := placeChips(nil, bestChipWeek); e.BenchBoost.GW != 0 || e.BenchBoost.Gain != 0 {
		t.Errorf("an empty season placed %+v", e.BenchBoost)
	}
}

// TestChipWeekOracleIsOffUnlessAsked pins the default in code.
//
// SimResult.ChipOracle is a pointer precisely so this is checkable: a zero-valued
// struct would read identically to "the oracle ran and found no week worth
// playing", which is the silent-no-op shape. Nil says the axis was never asked
// for.
func TestChipWeekOracleIsOffUnlessAsked(t *testing.T) {
	if (Oracles{}).Decision != AxisNone {
		t.Fatal("the zero Oracles requests a decision axis")
	}
	cols := (Oracles{Decision: AxisChipWeek}).MustNotMove()
	if len(cols) != len(cellMetricColumns) {
		t.Errorf("the chip-week axis pins %d columns and the sweep collects %d — "+
			"this axis changes no decision, so it must pin everything", len(cols),
			len(cellMetricColumns))
	}
}

// TestArmbandOracleCaptainsSomeoneWhoPlayed pins the three claims AxisArmband
// makes about who it names.
//
// The middle one is the load-bearing one for the *report*: an oracle captain
// necessarily played, so FPL's vice-captain fallback never fires under this
// oracle and the figure bounds captain and vice jointly. If that stopped being
// true the figure would silently change what it bounds.
func TestArmbandOracleCaptainsSomeoneWhoPlayed(t *testing.T) {
	cur := &Season{Name: "2025-26", Players: map[int]*Player{}}
	add := func(id, mins, pts int) {
		cur.Players[id] = &Player{ID: id, GWs: map[int]GW{7: {Minutes: mins, Points: pts}}}
	}
	// The highest scorer in the squad is not in the eleven, so naming him would be
	// reaching past the axis: the oracle may only choose among the eleven the model
	// itself picked.
	add(1, 90, 4)
	add(2, 90, 12)
	add(3, 0, 0)  // blanked: high-scoring on the season, nothing this week
	add(4, 20, 2) // a cameo, and still ahead of anyone who did not play
	add(99, 90, 30)
	xi := []int{1, 2, 3, 4}

	capt, vice := bestArmband(cur, xi, 7)
	if capt != 2 {
		t.Errorf("captain %d, want 2 — the highest realised scorer in the eleven", capt)
	}
	if vice != 1 {
		t.Errorf("vice %d, want 1 — the second highest who played", vice)
	}
	for _, id := range []int{capt, vice} {
		if cur.Players[id].GWs[7].Minutes == 0 {
			t.Errorf("%d recorded no minutes; an oracle captain must always have "+
				"played, or the figure stops bounding captain and vice jointly", id)
		}
		in := false
		for _, x := range xi {
			if x == id {
				in = true
			}
		}
		if !in {
			t.Errorf("%d is not in the eleven the model picked — the oracle has "+
				"reached past its axis", id)
		}
	}

	// Nobody played: nothing can be doubled whoever is named, and naming 0 would
	// quietly score the week under the *no-captain* rung, which is a third rule
	// rather than a captain who blanked.
	blank := &Season{Name: "x", Players: map[int]*Player{
		5: {ID: 5, GWs: map[int]GW{7: {}}},
		6: {ID: 6, GWs: map[int]GW{7: {}}},
	}}
	if c, v := bestArmband(blank, []int{5, 6}, 7); c == 0 || v == 0 {
		t.Errorf("an all-blank eleven named %d/%d; id 0 is 'no captain', which is a "+
			"different scoring rule", c, v)
	}

	// Ties break on the lower id, deterministically. Cells run concurrently in this
	// package and a map-iteration tiebreak has already made Optimize
	// non-deterministic once.
	tie := &Season{Name: "x", Players: map[int]*Player{
		8: {ID: 8, GWs: map[int]GW{7: {Minutes: 90, Points: 6}}},
		9: {ID: 9, GWs: map[int]GW{7: {Minutes: 90, Points: 6}}},
	}}
	for i := 0; i < 20; i++ {
		if c, _ := bestArmband(tie, []int{9, 8}, 7); c != 8 {
			t.Fatalf("tie broke to %d on iteration %d; the choice must be stable", c, i)
		}
	}
}

// TestArmbandOracleDeclaresTheFreeInvariances pins what the axis promises not to
// touch, because those four columns are the whole falsification budget for it.
func TestArmbandOracleDeclaresTheFreeInvariances(t *testing.T) {
	got := map[string]bool{}
	for _, c := range (Oracles{Decision: AxisArmband}).MustNotMove() {
		got[c] = true
	}
	for _, want := range []string{"moves", "hits", "hold_nocap_points", "hold_fixedcap_points"} {
		if !got[want] {
			t.Errorf("the armband axis does not pin %q — decide() never reads the "+
				"captain, so that column is a free check on an integer counted "+
				"without noise", want)
		}
	}
	// And it must NOT pin the two it legitimately moves, or a correct run fails and
	// whoever hits it learns to delete the check.
	for _, wrong := range []string{"policy_points", "hold_points"} {
		if got[wrong] {
			t.Errorf("the armband axis pins %q, which it legitimately moves", wrong)
		}
	}
}

// twoSeasonsForOracleTest builds the smallest pair Simulate will accept, so the
// entry-point guard can be tested without the archive. It is deliberately not
// playable to completion — the oracle refusal happens before any football.
func twoSeasonsForOracleTest() (cur, prior *Season) {
	cur = &Season{Name: "2025-26", Players: map[int]*Player{}}
	prior = &Season{Name: "2024-25", Players: map[int]*Player{}}
	return cur, prior
}
