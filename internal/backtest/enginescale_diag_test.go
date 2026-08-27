package backtest

// What the exposed returns do to the conversion scale that is LIVE ON `Score`.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagEngineScaleExposure -v
//
// # The quantity, and why nothing already banked transports to it
//
// There are two conversion scales in this repository, built through the same
// `analysis.CalibrationRatio`, with the same thin-sample floor and the same
// `[0.5, 3.0]` clamp. `reviews/2026-08-16-the-conversion-fit-integration/` sized
// one of them and named the other as unsized. This file sizes the other one.
//
//   - `Season.calibrateConversion` (`backtest/season.go`) fits `Player.Conversion`
//     over **per-gameweek archive rows**, gated on `underlyingCoverage`, and is
//     read at exactly one non-test site — `xPointsOf`. It is instrumentation and
//     cannot move replayed points. `conversionfit_diag_test.go` measured it:
//     assists shift −2.7% to −14.0% across the 18 fitted DEF/MID/FWD cells.
//   - `Engine.calibrateExpectedStats` (`analysis/metrics.go`) fits `e.xScale` over
//     **per-element season-to-date aggregates** off `e.Boot.Elements`, with **no
//     coverage gate and no exposure gate**, and `scaleFor` is read by `baseXP90`
//     — which multiplies `m.XA90 * sc.Assists * assistPoints` — and by
//     `fixtureSensitiveAt`. That is `Score`.
//
// ⚠️ **Do not carry a number from the first to the second.** The populations are
// different in a way that runs the whole length of this file: an exposed row there
// is a player-gameweek whose `XA` prints 0.00; an exposed **element** here is a
// player whose *entire season to date* xA is zero while he has a realised assist.
// The element aggregate is a sum of up to 38 two-decimal values, so it is zero only
// when every one of them is, which makes the population strictly smaller.
//
// ⚠️ **It does NOT follow that the element criterion reaches a smaller share of its
// own phenomenon, and an earlier draft of this comment said it did.** The near-zero
// *denominator* shrinks for exactly the same reason the numerator does. Measured at
// the sibling's own band edge, **pooled over the six seasons the point estimate runs
// the OTHER WAY** — 31.7% at `thr=6` and 23.5% at `thr=38`, against the sibling's
// row-level 15.0-17.4%. Still not established (different populations, different
// denominators, clustered single-digit counts), so the question is **unsettled** —
// but unsettled with a direction, not symmetrically open.
//
// # The three things that bound the shift, in the order they bind
//
// `CalibrationRatio` is `clamp(actual/expected, 0.5, 3.0)` behind a thin-sample
// guard at `minCalibrationSample`, and both bounds are **live on this fit**. So a
// cell is one of three kinds, and only the third can carry a measurable shift:
//
//   - **FLOORED** — the position's season-to-date expected total has not reached
//     the guard, so the scale is exactly the neutral 1.0 and the exposure is
//     absorbed **completely**, whatever its size. This is not a rare corner: the
//     aggregate starts at zero every August and climbs, so every position is
//     floored for the opening weeks of every season, which is exactly the window
//     in which a zero season-to-date xA is common.
//   - **CLAMPED** — the ratio left `[0.5, 3.0]` and the exposure is absorbed
//     **partially**, by an amount the clamp fixes rather than the football.
//   - **FITTED** — the plain ratio survived, and the exposure moves the scale.
//
// The floor and the clamp are read back from the **shipped exported function**
// rather than restated (see `calibrationFloored` and `calibrationClamped`), because
// `minCalibrationSample` is unexported and a second copy of 20.0 in a diagnostic is
// this record's signature failure.
//
// # The criterion, and what fraction of its own phenomenon it reaches
//
// The primary criterion is `el.ExpectedAssists == 0 && el.Assists > 0`, which is
// the population the exposure mechanism is stated over: a realised return in the
// numerator with nothing behind it in the denominator.
//
// ⚠️ **It is a two-decimal DISPLAY threshold, not a semantic zero**, and the sibling
// file establishes that at row level it captures only 15.0-17.4% of near-zero
// -expectation assists in FPL-fed seasons. `reportElementKnifeEdge` measures the
// element-level equivalent **at the sibling's own band edge**, which is the only
// comparison that means anything — a wider denominator lowers a share mechanically.
// The shift is reported under two wider band criteria beside the exact-zero one so
// that a reader can see the answer move with the definition. The band edges are
// **ASSERTED**; the argument needs the buckets above zero to be populated, not the
// edges to sit anywhere in particular.
//
// # Data state
//
// `FPL_NO_XG_REPAIR=1` **bears on this fit, and harder than on the sibling.** The
// engine applies no coverage gate, so a gameweek the season records no xA for still
// contributes its realised assists to the numerator and nothing to the denominator
// — which is the exposure mechanism operating on a whole window rather than on
// scattered rows. Under that switch 2020-21 and 2021-22 record no xA at all and
// 2022-23 records none before GW16, so those windows are pure numerator. Whether
// the floor or the clamp catches that is a question with an arithmetic answer, and
// the table below prints it. Run both states before quoting a level.
//
// # What is NOT claimed here
//
// Nothing is replayed, so **no points figure is produced and no detection threshold
// applies**. A shift in `sc.Assists` is a position-wide multiplier on one additive
// term of `baseXP90`, which by AGENTS.md's own qualification reorders **within**
// position wherever two players differ in `XA90` — so it is not disposed of by "a
// bias shared by a position is not an ordering error".
//
// The translation into `Score` **is** measured, by `reportScoreReach`, in expected
// points per 90 — and without writing `assistPoints` down, which would be a second
// copy of a scoring constant. It is *recovered* from the model's own output instead
// and asserted to be one value everywhere. An earlier version of this file declared
// the translation unmeasurable on the strength of that constant being unexported;
// that was too quick, and the measurement is cheap.
//
// # ⚠️ The counterfactual conditions on the outcome, so it is not a candidate repair
//
// `dropExposed` deletes elements selected on `Assists > 0`, and the matched
// population — zero xA and zero assists — contributes nothing to either sum. So the
// arm can only ever remove numerator, and **its sign is the definition of the arm
// rather than a finding about football**. What this file measures is a
// **decomposition** — what share of a position's realised assists carry no recorded
// expectation — and `droppedScale` is an accounting identity, not an estimator of
// anything. No repair is proposed here, and this counterfactual could not be one.
//
// For the same reason, prefer neutral language. The words "leak" and "exposure" are
// used below for continuity with the sibling file, and they **pre-judge**: this
// term exists precisely to price assists that xA does not count, so an assist with
// no xA behind it is the population the term is *for*.
//
// # ⚠️ One estimand mismatch, named and out of scope
//
// The fit's denominator is the **raw season-to-date aggregate** `ExpectedAssists`,
// while what `sc.Assists` multiplies in `baseXP90` is `m.XA90` — blended toward a
// prior season, shrunk, per 90. The ratio is fitted on one construction and applied
// to another. It does not disturb the *translation* of a shift, which is linear, but
// it is a larger threat to the fit's correctness than anything sized here.
// **Unmeasured, and out of this file's scope.**
//
// # It does not mutate the loaded season
//
// The parsed-season cache is process-global and contractually read-only. Nothing
// here writes to a `*Season`; the counterfactual arms are computed from the
// bootstrap `PointInTime` returns, which is built fresh on every call.

import (
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/stats"
)

// engineScaleCutoffs are the deadlines the table prints: the six replay entry
// points as the engine sees them (`start-1`, so GW1 is `through = 0` and reads
// last season's totals), plus the opening weeks where the floor is doing the work
// and the completed season as the far end.
//
// The summary table sweeps every cutoff 0..38 regardless; this list only governs
// what is printed row by row.
func engineScaleCutoffs() []int {
	return []int{0, 1, 2, 3, 4, 5, 6, 8, 10, 12, 15, 20, 25, 30, 38}
}

// engineScaleEntries are the six deadlines the replay grid actually enters at, as
// the engine sees them (`start-1`). They are reported separately from the maximum
// over all 39 cutoffs, because a maximum over 39 is an ARGMAX and this record's own
// warning applies to it: the winner is the cutoff whose noise was most flattering.
// These six are fixed in advance by the grid and are not selected on the outcome.
func engineScaleEntries() []int { return []int{0, 5, 10, 15, 20, 25} }

// elementBands are the "near-zero expectation" edges the exact-zero criterion is
// measured against. ⚠️ ASSERTED, not calibrated. 0.05 is the sibling file's
// `knifeEdgeBand`, five times the archive's own quantum of 0.01; 0.25 is a
// deliberately loose second edge, because an element aggregate sums many rows and
// an edge chosen for a single row has no claim on it.
var elementBands = []float64{0, knifeEdgeBand, 0.25}

// calibrationFloored reports whether CalibrationRatio's thin-sample guard fired at
// this expected total.
//
// It ASKS the shipped exported function rather than repeating `minCalibrationSample`,
// which is unexported: above the guard, `actual = 2*expected` returns exactly 2, and
// at or below it returns the neutral 1. At `expected == 0` the guard fires before the
// division, so this reports true rather than dividing by zero — which is the answer
// wanted, since a position with no expected events has no fit.
func calibrationFloored(expected float64) bool {
	return analysis.CalibrationRatio(2*expected, expected) != 2
}

// ⚠️ **`conversionfit_diag_test.go` answers the same question a second way**, and
// this pair makes that one redundant. It recognises a floored cell by the sentinel
// `ship[pos] != neutral`, which its own comment admits cannot see a CLAMPED cell —
// and which would also misread a genuine fitted ratio of exactly 1.0 as floored.
// Both files are `package backtest`, so it could call these with no import.
//
// **Not folded in here, deliberately.** `Season.conversionFit` returns scales rather
// than the sums these helpers need, so the change is a signature change to the
// sibling's fit plus a rewrite of the assertions that hang off `fitted` — a
// different measurement's correctness, in a branch that is only supposed to be
// counting. Recorded as owed rather than done quietly.
//
// calibrationClamped reports whether the [0.5, 3.0] clamp moved the ratio, with the
// floor taking precedence — a floored cell is not a clamped one, and reporting it as
// both would double-count the absorption.
//
// `clamp` returns its argument unchanged inside the range, so the equality is exact
// rather than tolerant, and a tolerance here would misreport a ratio sitting on 3.0
// exactly.
func calibrationClamped(actual, expected float64) bool {
	if calibrationFloored(expected) {
		return false
	}
	return analysis.CalibrationRatio(actual, expected) != actual/expected
}

// fitStatus is the three-way classification, in the order the bounds bind.
func fitStatus(actual, expected float64) string {
	switch {
	case calibrationFloored(expected):
		return "flr"
	case calibrationClamped(actual, expected):
		return "clp"
	}
	return "fit"
}

// elementTotals is one position's season-to-date fit input, exactly as
// calibrateExpectedStats accumulates it off Boot.Elements.
type elementTotals struct {
	goals, xG, assists, xA float64
	// exGoals/exAssists are the realised returns sitting on exposed elements at a
	// given band, and exG/exA are how many elements those are.
	exGoals, exAssists float64
	exG, exA           int
	// bandXG/bandXA are the expectation those same elements carry, which is zero at
	// the exact-zero criterion and non-zero at a wider band. Accumulated here rather
	// than recomputed so the drop arm removes exactly the elements the tally counted —
	// which is also why `droppedScale` takes no band of its own. The criterion enters
	// this file at exactly one place, `engineFitInputs`, and a second copy of it would
	// be a chance to refit a population the tally never counted.
	bandXG, bandXA float64
}

// engineFitInputs sums the bootstrap the way calibrateExpectedStats does, and
// tallies the exposed elements at `band` alongside.
//
// ⚠️ This is a second traversal of `Boot.Elements`, and the guard against it
// diverging from the shipped one is not inspection: `checkAgainstEngine` asserts the
// ratios it derives from these sums against what the engine itself reports through
// `PlayerMetrics.XGScale/XAScale`, at **every one of the 39 cutoffs** rather than at
// the printed subset — the census and the three maxima are taken over all 39, so
// guarding only what is printed would leave the widest-reaching figures unchecked.
// The RATIO is never recomputed — `analysis.CalibrationRatio` is exported and is
// called directly, so the floor and the clamp are the shipped ones.
//
// An element is exposed in a channel when it carries a realised return and its
// season-to-date expectation is at or below `band`. At `band == 0` that is the
// exact-zero criterion, whose standing as a display threshold the header states.
func engineFitInputs(boot *fpl.Bootstrap, band float64) map[int]*elementTotals {
	out := map[int]*elementTotals{}
	for i := range boot.Elements {
		el := &boot.Elements[i]
		tot := out[el.ElementType]
		if tot == nil {
			tot = &elementTotals{}
			out[el.ElementType] = tot
		}
		g, xg := float64(el.GoalsScored), el.ExpectedGoals.Float()
		a, xa := float64(el.Assists), el.ExpectedAssists.Float()
		tot.goals += g
		tot.xG += xg
		tot.assists += a
		tot.xA += xa
		if g > 0 && xg <= band {
			tot.exGoals += g
			tot.bandXG += xg
			tot.exG++
		}
		if a > 0 && xa <= band {
			tot.exAssists += a
			tot.bandXA += xa
			tot.exA++
		}
	}
	return out
}

// shippedScale is the fit as it runs, and droppedScale is the same fit with the
// exposed elements removed from BOTH sums — which at `band == 0` is identical to
// removing them from the numerator alone, since an exposed element contributes
// nothing to the denominator by definition.
func shippedScale(tot *elementTotals) analysis.ConversionScale {
	return analysis.ConversionScale{
		Goals:   analysis.CalibrationRatio(tot.goals, tot.xG),
		Assists: analysis.CalibrationRatio(tot.assists, tot.xA),
	}
}

// The excluded elements take their own expectation with them, which is what
// "refit with the exposed elements excluded" means. At the exact-zero criterion
// that expectation is zero by definition, so the two readings coincide there and
// the arm reduces to the sibling file's `S' = (A − A_b)/X`.
func droppedScale(tot *elementTotals) analysis.ConversionScale {
	return analysis.ConversionScale{
		Goals:   analysis.CalibrationRatio(tot.goals-tot.exGoals, tot.xG-tot.bandXG),
		Assists: analysis.CalibrationRatio(tot.assists-tot.exAssists, tot.xA-tot.bandXA),
	}
}

func TestDiagEngineScaleExposure(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)

	state := "POST-repair (default)"
	if noXGRepair() {
		state = "PRE-repair (FPL_NO_XG_REPAIR=1, xGC reconstruction also off)"
	}
	fmt.Printf("\n=== the conversion scale that is LIVE ON Score, and what its exposed elements do to it\n")
	fmt.Printf("data state: %s\n\n", state)
	fmt.Printf("Engine.calibrateExpectedStats fits e.xScale over PER-ELEMENT season-to-date\n")
	fmt.Printf("aggregates off Boot.Elements, with no coverage gate and no exposure gate,\n")
	fmt.Printf("and scaleFor is read by baseXP90 — which is Score. An EXPOSED element is one\n")
	fmt.Printf("carrying a realised return whose whole season-to-date expectation is zero.\n")
	fmt.Printf("That is a STRICTLY SMALLER population than the sibling file's exposed ROWS,\n")
	fmt.Printf("and no figure from that file transports here.\n\n")
	fmt.Printf("`thr` is the cutoff the engine is built at: `through`, so the six replay\n")
	fmt.Printf("entry points are 0/5/10/15/20/25. At thr=0 the bootstrap carries LAST\n")
	fmt.Printf("season's totals, which is what a GW1 squad is picked on.\n\n")
	fmt.Printf("`Gst` and `Ast` are which of the three bounds is binding: flr = below the thin-sample\n")
	fmt.Printf("floor, so the scale is exactly 1.0 and the exposure is absorbed completely;\n")
	fmt.Printf("clp = the [0.5, 3.0] clamp moved it, so the exposure is absorbed in part by\n")
	fmt.Printf("the clamp rather than by the football; fit = the plain ratio survived and\n")
	fmt.Printf("the exposure moves the scale. Read `sh%%` ONLY on a `fit` cell.\n\n")

	exposureTable(t, pairs)
	gkpCensus(t, pairs)
	reportAggregateMediator(pairs)
	reportElementKnifeEdge(pairs)
	reportScoreReach(t, pairs, cfg)
	engineScaleSummary(t, pairs, cfg)

	fmt.Printf("\n⚠️ Nothing here is a points figure. No cell was replayed, so no detection\n")
	fmt.Printf("threshold applies and none of this sizes an arm. A shift in sc.Assists is a\n")
	fmt.Printf("position-wide multiplier on ONE additive term of baseXP90, which reorders\n")
	fmt.Printf("within position wherever two players differ in XA90 — the translation into\n")
	fmt.Printf("Score needs assistPoints, is unexported, and is left unmeasured here.\n")
	fmt.Printf("\n⚠️ No repair is proposed. The exact-zero criterion is a display threshold\n")
	fmt.Printf("and the knife-edge table above says what share of its own phenomenon it\n")
	fmt.Printf("reaches; a repair defined on it would act on that share.\n")
}

// exposureTable prints both channels at each printed cutoff. The two are one table
// rather than two because the interesting comparison is BETWEEN them: the sibling
// file's goal column is zero and that zero is FORCED there by underlyingCoverage
// excluding an uncovered gameweek from the fit. calibrateExpectedStats has no such
// gate, so this goal column is a measurement rather than an inherited null, and
// putting it beside the assist column is what stops it being read as the sibling's.
//
// Goalkeepers are omitted from the rows and asserted instead — see gkpCensus. They
// are floored at every cutoff of every season, which is one fact rather than 90 rows
// of it, and an assertion states it in a way a table cannot.
func exposureTable(t *testing.T, pairs []seasonPair) {
	fmt.Printf("\n=== exposed elements and the scale they move, both channels, outfield\n\n")
	fmt.Printf("`ex` counts EXPOSED ELEMENTS at the exact-zero criterion and `b` is the\n")
	fmt.Printf("realised returns sitting on them. `x` is the position's season-to-date\n")
	fmt.Printf("expected total, which is what the floor is tested against, and `S` is the\n")
	fmt.Printf("shipped fitted scale. `sh%%` is what the scale would be under a refit with\n")
	fmt.Printf("those elements excluded — READ IT ONLY ON A `fit` CELL.\n\n")
	fmt.Printf("%-14s %4s | %4s %5s %4s %7s %7s %8s | %4s %5s %4s %7s %7s %8s\n",
		"season pos", "thr",
		"Gex", "G_b", "Gst", "xG", "S_G", "shG%",
		"Aex", "A_b", "Ast", "xA", "S_A", "shA%")
	for _, pair := range pairs {
		for _, thr := range engineScaleCutoffs() {
			boot, _ := PointInTime(pair.Cur, pair.Prior, thr)
			sums := engineFitInputs(boot, 0)
			for pos := 2; pos <= 4; pos++ {
				tt := sums[pos]
				if tt == nil {
					continue
				}
				ship := shippedScale(tt)
				drop := droppedScale(tt)
				fmt.Printf("%-14s %4d | %4d %5.0f %4s %7.1f %7.3f %7.2f%% | %4d %5.0f %4s %7.1f %7.3f %7.2f%%\n",
					pair.Name+" "+posShort(pos), thr,
					tt.exG, tt.exGoals, fitStatus(tt.goals, tt.xG), tt.xG,
					ship.Goals, pctShift(ship.Goals, drop.Goals),
					tt.exA, tt.exAssists, fitStatus(tt.assists, tt.xA), tt.xA,
					ship.Assists, pctShift(ship.Assists, drop.Assists))
			}
		}
	}
}

// gkpCensus asserts the keeper case rather than printing it, because it is one fact
// and it is the cleanest instance of "bounded by construction" in this file: a
// keeper population's season xA never approaches the thin-sample floor, so the scale
// is exactly the neutral 1.0 at every cutoff and NO amount of keeper exposure can
// move it.
//
// It is an assertion and not a remark because it is the half of the verdict that
// could stop being true — a rules change crediting keepers with assists, or a floor
// lowered for some other reason, and the absorption quietly ends.
func gkpCensus(t *testing.T, pairs []seasonPair) {
	t.Helper()
	var exposedCells int
	for _, pair := range pairs {
		for thr := 0; thr <= 38; thr++ {
			boot, _ := PointInTime(pair.Cur, pair.Prior, thr)
			tt := engineFitInputs(boot, 0)[1]
			if tt == nil {
				continue
			}
			if tt.exA > 0 || tt.exG > 0 {
				exposedCells++
			}
			if st := fitStatus(tt.assists, tt.xA); st != "flr" {
				t.Errorf("%s thr=%d: the keeper assist channel is %q, not floored, at "+
					"xA %.3f. The keeper absorption reported here is not bounded by "+
					"construction after all", pair.Name, thr, st, tt.xA)
			}
			if st := fitStatus(tt.goals, tt.xG); st != "flr" {
				t.Errorf("%s thr=%d: the keeper goal channel is %q, not floored, at "+
					"xG %.3f", pair.Name, thr, st, tt.xG)
			}
		}
	}
	fmt.Printf("\n=== goalkeepers: absorbed entirely, at every cutoff of every season\n\n")
	fmt.Printf("Asserted rather than tabulated. A keeper population's season-to-date xG and\n")
	fmt.Printf("xA never reach the thin-sample floor, so scaleFor(1) is exactly the neutral\n")
	fmt.Printf("1.0 at all 39 cutoffs of all %d seasons and no keeper exposure can move it.\n",
		len(pairs))
	fmt.Printf("%d of those %d position-cutoff cells carry an exposed keeper anyway, which is\n",
		exposedCells, 39*len(pairs))
	fmt.Printf("what makes the absorption a fact about the floor rather than about the data.\n")
}

// reportAggregateMediator is the check that stops `thr=0 is byte-identical across
// the two data states` being read as a tie.
//
// The two halves of the information seam read DIFFERENT fields. `PointInTimeWith`
// accumulates `p.GWs[1..through].XA`, which `applyXGRepair` writes; `PreSeasonWith`
// reads the prior season's aggregate `p.XA`, which comes off `players_raw.csv` and
// which `rebuildXGAggregates` only ever fills where it is already zero. So a season
// whose own aggregate is complete has a pre-season cell the repair switch **cannot
// reach**, and its `thr=0` row is byte-identical for a reason that is a code fact
// rather than a measurement.
//
// This prints both quantities so the asymmetry is measured rather than inferred, and
// it is the same table that says which seasons the switch does bear on.
func reportAggregateMediator(pairs []seasonPair) {
	fmt.Printf("\n=== the two halves of the seam read different fields, so name which cutoff\n\n")
	fmt.Printf("`agg` is the season's own aggregate xA summed over players — what a LATER\n")
	fmt.Printf("season's pre-season cell (thr=0) reads through PreSeasonWith. `rows` is the\n")
	fmt.Printf("same players' per-gameweek xA summed — what every in-season cutoff reads\n")
	fmt.Printf("through PointInTimeWith. The REPAIR writes the weekly rows (applyXGRepair)\n")
	fmt.Printf("and fills the aggregate ONLY where it is already zero (rebuildXGAggregates);\n")
	fmt.Printf("FPL_NO_XG_REPAIR=1 DISABLES both. ⚠️ An earlier draft of this sentence said\n")
	fmt.Printf("the switch writes the rows, which tells a reader it ADDS data.\n\n")
	fmt.Printf("So where `agg` and `rows` disagree, the pre-season cell and the in-season\n")
	fmt.Printf("cells are fitting on different data, and a byte-identical thr=0 across the\n")
	fmt.Printf("two states is a comparison that never ran rather than a tie.\n\n")

	seen := map[string]bool{}
	fmt.Printf("%-9s | %10s %10s | %10s %10s\n", "season", "agg xG", "rows xG", "agg xA", "rows xA")
	for _, pair := range pairs {
		for _, s := range []*Season{pair.Prior, pair.Cur} {
			if s == nil || seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			var aggG, aggA, rowG, rowA float64
			for _, id := range sortedSeasonPlayerIDs(s) {
				pl := s.Players[id]
				aggG += pl.XG
				aggA += pl.XA
				for gw := 1; gw <= 38; gw++ {
					if g, ok := pl.GWs[gw]; ok {
						rowG += g.XG
						rowA += g.XA
					}
				}
			}
			fmt.Printf("%-9s | %10.1f %10.1f | %10.1f %10.1f\n", s.Name, aggG, rowG, aggA, rowA)
		}
	}
}

// reportScoreReach answers the question a percentage on a scale cannot: **what
// does this do to `Score`**, in the model's own units, with nothing replayed.
//
// # The seam, and why it is exact
//
// `el.Assists` is read at exactly two places in `internal/analysis`: the fit
// (`metrics.go:1270`) and a reporting field on `PlayerMetrics` (`:1483`) that
// `baseXP90` does not read. So zeroing the exposed elements' realised assists in a
// **copy** of the bootstrap moves the fitted scale and nothing else, and the
// per-player difference in `BaseXP90` is the scale's whole reach into `Score`.
//
// # It recovers the points constant rather than writing it down
//
// `assistPoints` is unexported, and a copy of a scoring constant in a diagnostic is
// this project's signature failure — so the earlier version of this file declared
// the translation unmeasurable and left it. That was too quick. Since
// `ΔBaseXP90 = XA90 · assistPoints · ΔS` exactly, the constant is **recoverable**
// from the model's own output wherever `XA90` and `ΔS` are non-zero, and it must
// come out the same for every player of every position — which is asserted here,
// and is a stronger check than a written-down number could be.
//
// # What it reports
//
// `dxp` is the largest per-player `|ΔBaseXP90|` at the cutoff, in expected points
// per 90. `share` is the assist term's share of `BaseXP90` for the median player of
// the position — the quantification `AGENTS.md`'s "a position-wide multiplier on one
// additive term" rule asks for, and which this file previously left absent. `inv` is
// the number of within-position ordering inversions the shift causes: since `ΔS` is a
// pure multiplier on one additive term, the re-ranking is deterministic in each
// player's `XA90` share, so inversions are counted rather than estimated.
//
// ⚠️ **`inv` counts ordering changes over the whole registered pool, not decisions.**
// `Optimize` is a knapsack against a budget, so an inversion is necessary for a
// different fifteen and nowhere near sufficient. Nothing here is a points figure.
func reportScoreReach(t *testing.T, pairs []seasonPair, cfg config.Config) {
	t.Helper()
	fmt.Printf("\n=== what the shift reaches on Score, in the model's own units\n\n")
	fmt.Printf("Both arms are engines wired exactly as Simulate wires them; the only\n")
	fmt.Printf("difference is that the second is built over a COPY of the bootstrap with the\n")
	fmt.Printf("exposed elements' realised assists zeroed. el.Assists is read at exactly two\n")
	fmt.Printf("places in internal/analysis — the fit, and a PlayerMetrics field baseXP90\n")
	fmt.Printf("does not read — so the difference in BaseXP90 is the scale and nothing else.\n\n")
	fmt.Printf("`dxp` is the largest per-player |dBaseXP90| in expected points per 90.\n")
	fmt.Printf("`share` is the assist term's share of BaseXP90 for the position's median\n")
	fmt.Printf("player — what AGENTS.md's position-wide-multiplier rule asks for. `inv` is\n")
	fmt.Printf("within-position ordering inversions over the registered pool.\n\n")
	fmt.Printf("⚠️ An inversion is NECESSARY for a different fifteen and nowhere near\n")
	fmt.Printf("SUFFICIENT — Optimize is a knapsack against a budget. No cell was replayed.\n\n")

	fmt.Printf("%-14s %4s | %8s %9s %8s %6s %5s\n",
		"season pos", "thr", "shA%", "dxp", "share", "inv", "n")

	// assistPoints, recovered from the model's own output rather than written down.
	// It must be one value across every player, position, season and cutoff.
	recovered, haveRecovered := 0.0, false

	for _, pair := range pairs {
		for _, thr := range engineScaleCutoffs() {
			ship, shipBoot := EngineAt(pair.Cur, pair.Prior, thr, SimConfig{Weights: cfg.Weights})

			// The counterfactual bootstrap: a copy, with the exposed elements'
			// realised assists removed. Copying is what keeps the shipped arm and
			// every other figure in this file untouched.
			sums := engineFitInputs(shipBoot, 0)
			dropBoot := *shipBoot
			dropBoot.Elements = append([]fpl.Element(nil), shipBoot.Elements...)
			for i := range dropBoot.Elements {
				el := &dropBoot.Elements[i]
				if el.Assists > 0 && el.ExpectedAssists.Float() == 0 {
					el.Assists = 0
				}
			}
			_, fx := PointInTime(pair.Cur, pair.Prior, thr)
			drop := analysis.NewEngineFull(&dropBoot, fx, cfg.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			drop.Priors = SimConfig{Weights: cfg.Weights}.priors(pair.Cur, pair.Prior)
			drop.Recent = SimConfig{Weights: cfg.Weights}.recentIndex(pair.Cur, thr)
			drop.TeamForm = newTeamFormIndex(pair.Cur, thr)

			for pos := 2; pos <= 4; pos++ {
				tt := sums[pos]
				if tt == nil {
					continue
				}
				dS := shippedScale(tt).Assists
				dropA, dropX := tt.assists-tt.exAssists, tt.xA-tt.bandXA
				if fitStatus(tt.assists, tt.xA) != "fit" || fitStatus(dropA, dropX) != "fit" {
					continue
				}
				delta := droppedScale(tt).Assists - dS
				if delta == 0 {
					continue
				}

				var maxAbs float64
				var shares []float64
				var n, inversions int
				type ranked struct{ before, after float64 }
				var rows []ranked
				for i := range shipBoot.Elements {
					el := &shipBoot.Elements[i]
					if el.ElementType != pos || el.Minutes <= 0 {
						continue
					}
					mb := ship.Metrics(el)
					ma := drop.Metrics(&dropBoot.Elements[i])
					d := ma.BaseXP90 - mb.BaseXP90
					if abs(d) > maxAbs {
						maxAbs = abs(d)
					}
					// assistPoints = dBaseXP90 / (XA90 * dS), exact by construction —
					// but recovered only where the signal clears float noise. `d` is a
					// difference of two BaseXP90 values near 4.0, so on a player whose
					// XA90 is ~1e-17 the subtraction cancels everything and the ratio is
					// noise divided by noise. Guarding on `d` rather than on XA90 is the
					// right test, because it is the cancellation that bites.
					if mb.XA90 > 0 && abs(d) > 1e-9*(1+abs(mb.BaseXP90)) {
						got := d / (mb.XA90 * delta)
						if !haveRecovered {
							recovered, haveRecovered = got, true
						} else if abs(got-recovered) > 1e-6*(1+abs(recovered)) {
							t.Fatalf("%s %s thr=%d: the assist points constant recovers as "+
								"%.9f here and %.9f elsewhere. dBaseXP90 = XA90 * "+
								"assistPoints * dScale is how the scale is supposed to "+
								"enter Score; if it does not hold, this whole table is "+
								"measuring something else",
								pair.Name, posShort(pos), thr, got, recovered)
						}
						if mb.BaseXP90 > 0 {
							shares = append(shares, mb.XA90*mb.XAScale*recovered/mb.BaseXP90)
						}
					}
					rows = append(rows, ranked{mb.BaseXP90, ma.BaseXP90})
					n++
				}
				for i := range rows {
					for j := i + 1; j < len(rows); j++ {
						if (rows[i].before < rows[j].before) != (rows[i].after < rows[j].after) {
							inversions++
						}
					}
				}
				med := stats.Median(shares)
				fmt.Printf("%-14s %4d | %7.2f%% %9.5f %7.2f%% %6d %5d\n",
					pair.Name+" "+posShort(pos), thr,
					pctShift(dS, droppedScale(tt).Assists), maxAbs, 100*med, inversions, n)
			}
		}
	}
	if haveRecovered {
		fmt.Printf("\nassist points recovered from the model's own output, one value "+
			"everywhere: %.6f\n", recovered)
	}
}

func reportElementKnifeEdge(pairs []seasonPair) {
	fmt.Printf("\n=== what the exact-zero criterion reaches, at ELEMENT aggregate level\n\n")
	fmt.Printf("Outfield elements (2-4). `as@0` is assists on elements whose season-to-date\n")
	fmt.Printf("xA is exactly zero; `as@%.2f` and `as@%.2f` are assists on elements at or\n",
		elementBands[1], elementBands[2])
	fmt.Printf("below those bands. `sh@%.2f` is as@0/as@%.2f and `sh@%.2f` is as@0/as@%.2f —\n",
		elementBands[1], elementBands[1], elementBands[2], elementBands[2])
	fmt.Printf("the fraction of near-zero-expectation assists the exact-zero criterion\n")
	fmt.Printf("captures, at each edge.\n\n")
	fmt.Printf("⚠️ **BOTH shares are printed because only ONE of them is comparable with the\n")
	fmt.Printf("sibling file**, and an earlier version of this table quoted the wrong one.\n")
	fmt.Printf("`conversionfit_diag_test.go` computes `zero/(zero+band)` at an edge of %.2f\n",
		knifeEdgeBand)
	fmt.Printf("and reports 15.0-17.4%% FPL-fed. A wider denominator lowers a share\n")
	fmt.Printf("MECHANICALLY, so setting the %.2f column beside that figure attributes to\n",
		elementBands[2])
	fmt.Printf("the population what the band edge did. Compare `sh@%.2f` and nothing else.\n\n",
		elementBands[1])
	fmt.Printf("⚠️ And the argument that the element criterion MUST reach less than the row\n")
	fmt.Printf("criterion does not survive its own arithmetic. It is true that an element\n")
	fmt.Printf("aggregate is zero only when every one of its rows is, which shrinks the\n")
	fmt.Printf("numerator — but the near-zero DENOMINATOR shrinks for the same reason, and\n")
	fmt.Printf("the shipped-state numbers below do not order the way the argument predicts.\n")
	fmt.Printf("Whether the element criterion reaches less of its phenomenon is UNSETTLED.\n\n")
	fmt.Printf("⚠️ Both band edges are ASSERTED, not calibrated. And at thr=38 these are\n")
	fmt.Printf("ratios of SINGLE-DIGIT counts — the counts are printed so a share is never\n")
	fmt.Printf("read without them.\n\n")
	fmt.Printf("Two cutoffs, because this is a season-to-date aggregate and the answer is a\n")
	fmt.Printf("function of when you look: thr=6 is early, where a zero aggregate is common,\n")
	fmt.Printf("and thr=38 is the completed season.\n\n")

	fmt.Printf("%-9s %4s | %7s %7s %7s | %8s %8s\n",
		"season", "thr", "as@0", "as@band", "as@wide", "sh@band", "sh@wide")
	pooled := map[int][3]float64{}
	for _, pair := range pairs {
		for _, thr := range []int{6, 38} {
			boot, _ := PointInTime(pair.Cur, pair.Prior, thr)
			var at [3]float64
			for i, band := range elementBands {
				sums := engineFitInputs(boot, band)
				for pos := 2; pos <= 4; pos++ {
					if tt := sums[pos]; tt != nil {
						at[i] += tt.exAssists
					}
				}
			}
			narrow, wide := 0.0, 0.0
			if at[1] > 0 {
				narrow = 100 * at[0] / at[1]
			}
			if at[2] > 0 {
				wide = 100 * at[0] / at[2]
			}
			q := pooled[thr]
			pooled[thr] = [3]float64{q[0] + at[0], q[1] + at[1], q[2] + at[2]}
			fmt.Printf("%-9s %4d | %7.0f %7.0f %7.0f | %7.1f%% %7.1f%%\n",
				pair.Name, thr, at[0], at[1], at[2], narrow, wide)
		}
	}

	// ⚠️ **The pooled row is the only one that can be read**, and the per-season rows
	// above are printed so that it is never quoted without its counts. A season's
	// share is 0/2, 1/1, 6/12 — single-digit counts that cannot order against the
	// sibling's 15.0-17.4% in either direction, and 100.0% on a denominator of 1 is
	// not a rate.
	//
	// ⚠️ **And the pooled point estimate runs OPPOSITE to the retracted argument**,
	// which the retraction above should not be read as leaving open in both
	// directions. It is still not established — different populations (rows against
	// element aggregates), different denominators, and assists cluster within
	// elements so the counts are not independent — but the direction is the reverse
	// of "the element criterion must reach less".
	fmt.Printf("\n%-9s %4s | %7s %7s %7s | %8s %8s\n",
		"POOLED", "thr", "as@0", "as@band", "as@wide", "sh@band", "sh@wide")
	for _, thr := range []int{6, 38} {
		q := pooled[thr]
		narrow, wide := 0.0, 0.0
		if q[1] > 0 {
			narrow = 100 * q[0] / q[1]
		}
		if q[2] > 0 {
			wide = 100 * q[0] / q[2]
		}
		fmt.Printf("%-9s %4d | %7.0f %7.0f %7.0f | %7.1f%% %7.1f%%\n",
			"all six", thr, q[0], q[1], q[2], narrow, wide)
	}
	fmt.Printf("\nCompare the POOLED sh@band against the sibling's 15.0-17.4%% and nothing\n")
	fmt.Printf("else. %s is still that many clustered samples, not a test.\n",
		seasonsLabel(len(pairs)))
}

// engineScaleSummary sweeps EVERY cutoff 0..38 rather than the printed subset, so
// the census is over the whole season rather than fifteen deadlines. It also
// carries the one-implementation guard, for the reason given at the call.
//
// ⚠️ **Both channels are censused, not just assists.** An earlier version tracked
// the assist channel alone here and tabulated goals only at the printed cutoffs,
// which made "the goal channel is empty at every cutoff of every season" a claim
// over 15 cutoffs wearing the clothes of a claim over 39. The goal channel is not
// forced to be empty — `calibrateExpectedStats` has no coverage gate — so it has to
// be measured wherever the assist channel is.
func engineScaleSummary(t *testing.T, pairs []seasonPair, cfg config.Config) {
	fmt.Printf("\n=== census over every cutoff 0..38, BOTH channels\n\n")
	fmt.Printf("`flr/clp/fit` counts the 39 cutoffs by which bound was binding. `nex` is how\n")
	fmt.Printf("many carry at least one exposed element and `mx` is the largest count at any\n")
	fmt.Printf("of them. The goal half is censused here rather than inferred from the printed\n")
	fmt.Printf("table, because nothing forces it to be empty.\n\n")

	fmt.Printf("%-9s %-4s | %4s %4s %4s %5s %4s | %4s %4s %4s %5s %4s\n",
		"season", "pos",
		"Gflr", "Gclp", "Gfit", "Gnex", "Gmx",
		"Aflr", "Aclp", "Afit", "Anex", "Amx")

	type cell struct {
		gFlr, gClp, gFit, gNex, gMax int
		aFlr, aClp, aFit, aNex, aMax int
		// sh holds the largest-MAGNITUDE signed shift under each criterion, and shMin
		// the smallest over fitted cutoffs. Signed, because every shift here is <= 0 by
		// arithmetic and reporting magnitudes made this file read as though the
		// direction were unknown — the sibling reports signed and the two must compare.
		sh [3]float64
		// dropUnfit counts, per band, the cutoffs where the SHIPPED cell was fitted
		// but the counterfactual was not — where a shift would have been the floor or
		// the clamp rather than the football.
		dropUnfit [3]int
		shMin     float64
		hasSh     bool
		at        int
		// The six entry deadlines, per CELL rather than per position: a maximum over
		// six is still a maximum, and 66 fitted-and-exactly-zero cells is the figure
		// that argues the verdict.
		entryWorst                           float64
		entryShift, entryZero, entryUnfitted int
	}
	byKey := map[[2]int]*cell{}

	var anyExposed, anyClean, gkpExposed int
	var closedFormChecked int
	// allFitted collects EVERY fitted outfield cutoff's shift, which is the
	// population the replay actually stands at — see the block printed below.
	var allFitted []float64
	entry := map[int]bool{}
	for _, e := range engineScaleEntries() {
		entry[e] = true
	}

	for si, pair := range pairs {
		for thr := 0; thr <= 38; thr++ {
			boot, _ := PointInTime(pair.Cur, pair.Prior, thr)
			var sums [3]map[int]*elementTotals
			for i, band := range elementBands {
				sums[i] = engineFitInputs(boot, band)
			}
			// The one-implementation guard runs HERE rather than beside the printed
			// table, because this loop is the one that visits every cutoff. Guarding
			// only the printed subset would leave the census and the maxima — which
			// are taken over all 39 — resting on an unchecked traversal.
			checkAgainstEngine(t, pair, thr, sums[0], cfg)
			for pos := 1; pos <= 4; pos++ {
				tt := sums[0][pos]
				if tt == nil {
					continue
				}
				c := byKey[[2]int{si, pos}]
				if c == nil {
					c = &cell{at: -1}
					byKey[[2]int{si, pos}] = c
				}

				// The goal half of the census.
				if tt.exG > 0 {
					c.gNex++
				}
				if tt.exG > c.gMax {
					c.gMax = tt.exG
				}
				switch fitStatus(tt.goals, tt.xG) {
				case "flr":
					c.gFlr++
				case "clp":
					c.gClp++
				default:
					c.gFit++
				}

				// The assist half, which is the one the shift is reported on.
				if tt.exA > 0 {
					anyExposed++
					c.aNex++
					if pos == 1 {
						gkpExposed++
					}
				} else {
					anyClean++
				}
				if tt.exA > c.aMax {
					c.aMax = tt.exA
				}
				switch fitStatus(tt.assists, tt.xA) {
				case "flr":
					c.aFlr++
					if entry[thr] {
						c.entryUnfitted++
					}
					continue
				case "clp":
					c.aClp++
					if entry[thr] {
						c.entryUnfitted++
					}
					continue
				}
				c.aFit++

				for i := range elementBands {
					b := sums[i][pos]

					// ⚠️ **The COUNTERFACTUAL has to be fitted too, and gating on the
					// shipped cell alone is a bug this table shipped once.** At a wider
					// band the excluded elements take real expectation with them, so
					// `xA - bandXA` can fall back under the thin-sample floor — and then
					// `droppedScale` returns the neutral 1.0 and the "shift" reported is
					// the floor firing, not the football. It bit exactly where it does
					// most damage: the two largest numbers in the old `sh@wide` column,
					// −62.96% and −57.32%, were both this, at cells whose denominator
					// crossed 20.0 on the way down. It cannot arise at band 0, where the
					// two denominators are equal by construction, which is why guarding
					// only there found nothing.
					dropA, dropX := b.assists-b.exAssists, b.xA-b.bandXA
					if fitStatus(dropA, dropX) != "fit" {
						c.dropUnfit[i]++
						continue
					}

					v := pctShift(shippedScale(b).Assists, droppedScale(b).Assists)
					// ⚠️ **The sign is deductive at band 0 ONLY, and asserting it at the
					// wider bands would be asserting a measurement.** At band 0 an
					// excluded element contributes nothing to the denominator, so the
					// drop arm is `(A-A_b)/X` and cannot exceed `S`. At a wider band it
					// is `(A-A_b)/(X-X_b)`, which EXCEEDS `S` whenever the excluded
					// elements convert worse than the position does — `A_b/X_b < A/X`.
					// That never happens in this archive, because near-zero-expectation
					// elements convert absurdly high, but it is a fact about football
					// rather than about arithmetic, and a failure message calling it
					// impossible would blame the traversal for it.
					//
					// It is also an invariant of the RATIO rather than of
					// `CalibrationRatio`: a floored drop arm returns 1.0 regardless, so
					// a shipped scale below 1.0 would report a rise. Both halves exist
					// in the archive separately — the smallest fitted shipped assist
					// scale is 0.9259 — so the fit guard above is what keeps this honest.
					if i == 0 && v > 0 {
						t.Errorf("%s %s thr=%d: the refit RAISED the exact-zero scale by "+
							"%.4f%%, with both arms on the plain ratio. At band 0 an "+
							"excluded element contributes nothing to the denominator, so "+
							"this cannot happen unless the drop arm is summing something "+
							"else", pair.Name, posShort(pos), thr, v)
					}
					if v < c.sh[i] {
						c.sh[i] = v
						if i == 0 {
							c.at = thr
						}
					}
					if i != 0 {
						continue
					}
					if !c.hasSh || v > c.shMin {
						c.shMin = v
					}
					c.hasSh = true
					if pos >= 2 {
						allFitted = append(allFitted, v)
					}
					if entry[thr] {
						if v == 0 {
							c.entryZero++
						} else {
							c.entryShift++
						}
						if v < c.entryWorst {
							c.entryWorst = v
						}
					}
					// The closed form: with `S = A/x` and `S' = (A-A_b)/x`, the
					// denominator cancels and the percentage shift is EXACTLY
					// `-100*A_b/A` — the exposed elements' share of the position's
					// REALISED assists, with xA playing no part. That is what makes the
					// early cutoffs win: A is smallest there.
					//
					// Both arms are known to be on the plain ratio here, because the
					// shipped cell reached `c.aFit` and the drop arm cleared the guard
					// above, so nothing is skipped and the algebra always holds.
					want := 0.0
					if b.assists > 0 {
						want = -100 * b.exAssists / b.assists
					}
					if abs(v-want) > 1e-9*(1+abs(want)) {
						t.Errorf("%s %s thr=%d: the shift is %.9f%% and the closed form "+
							"-100*A_b/A gives %.9f%%. If these disagree the fit is no "+
							"longer a plain ratio over one denominator, and the reading "+
							"that xA cancels does not survive it",
							pair.Name, posShort(pos), thr, v, want)
					}
					closedFormChecked++
				}
			}
		}
	}

	for si, pair := range pairs {
		for pos := 1; pos <= 4; pos++ {
			c := byKey[[2]int{si, pos}]
			if c == nil {
				continue
			}
			fmt.Printf("%-9s %-4s | %4d %4d %4d %5d %4d | %4d %4d %4d %5d %4d\n",
				pair.Name, posShort(pos),
				c.gFlr, c.gClp, c.gFit, c.gNex, c.gMax,
				c.aFlr, c.aClp, c.aFit, c.aNex, c.aMax)
		}
	}

	fmt.Printf("\n=== the assist shift, SIGNED, and what it looks like at the six entry deadlines\n\n")
	fmt.Printf("⚠️ `sh@0max` is the largest-magnitude shift over ALL 39 CUTOFFS, which is an\n")
	fmt.Printf("ARGMAX — the cutoff that won is the one whose thin early-season denominator\n")
	fmt.Printf("flattered it most, so read it as a bound and never as a level. `at` prints\n")
	fmt.Printf("where it landed. `sh@0min` is the smallest over fitted cutoffs, which says\n")
	fmt.Printf("whether every fitted cell moves or only some.\n\n")
	fmt.Printf("The `entry` block is per CELL over the six replay deadlines, a set fixed by\n")
	fmt.Printf("the grid rather than chosen on the outcome: `sh` fitted and moved, `0` fitted\n")
	fmt.Printf("and exactly zero, `--` floored or clamped so no shift is defined. Those last\n")
	fmt.Printf("two are DIFFERENT and an earlier version of this table conflated them.\n\n")
	fmt.Printf("`worst` is the largest-magnitude shift over those six alone.\n\n")

	fmt.Printf("⚠️ `sh@band` and `sh@wide` remove elements carrying real if small expectation,\n")
	fmt.Printf("so they measure a DIFFERENT estimand rather than a more generous version of\n")
	fmt.Printf("the same one. `xb`/`xw` count the cutoffs where the SHIPPED cell was fitted\n")
	fmt.Printf("but the COUNTERFACTUAL was not — excluded there, because the reported shift\n")
	fmt.Printf("would have been the floor firing on the drop arm rather than the football.\n")
	fmt.Printf("A non-zero `xb`/`xw` means that column is over a smaller population than\n")
	fmt.Printf("`sh@0max`, and it must not be read as a bound on the exposure.\n\n")

	fmt.Printf("%-9s %-4s | %8s %5s %8s | %3s %3s %3s %8s | %8s %3s %8s %3s\n",
		"season", "pos", "sh@0max", "at", "sh@0min",
		"sh", "0", "--", "worst", "sh@band", "xb", "sh@wide", "xw")

	var outfieldWorst []float64
	var eShift, eZero, eUnfit int
	for si, pair := range pairs {
		for pos := 1; pos <= 4; pos++ {
			c := byKey[[2]int{si, pos}]
			if c == nil {
				continue
			}
			if pos >= 2 {
				eShift += c.entryShift
				eZero += c.entryZero
				eUnfit += c.entryUnfitted
			}
			// A position with no FITTED cutoff has no shift to report at all, and
			// printing 0.00% there would read as "measured, and zero" when it means
			// "the floor or the clamp held at every one of the 39".
			if c.aFit == 0 {
				fmt.Printf("%-9s %-4s | %8s %5s %8s | %3d %3d %3d %8s | %8s %3d %8s %3d\n",
					pair.Name, posShort(pos), "--", "--", "--",
					c.entryShift, c.entryZero, c.entryUnfitted, "--",
					"--", c.dropUnfit[1], "--", c.dropUnfit[2])
				continue
			}
			at := "-"
			if c.at >= 0 {
				at = fmt.Sprintf("%d", c.at)
			}
			if pos >= 2 {
				outfieldWorst = append(outfieldWorst, c.entryWorst)
			}
			fmt.Printf("%-9s %-4s | %7.2f%% %5s %7.2f%% | %3d %3d %3d %7.2f%% | %7.2f%% %3d %7.2f%% %3d\n",
				pair.Name, posShort(pos), c.sh[0], at, c.shMin,
				c.entryShift, c.entryZero, c.entryUnfitted, c.entryWorst,
				c.sh[1], c.dropUnfit[1], c.sh[2], c.dropUnfit[2])
		}
	}

	// Generated rather than read off the table by hand: a figure copied out of a
	// printed table has no generator, and nothing re-derives it when the table moves.
	// Sorted for the extreme; the median comes from stats.Median, which is the one
	// implementation of that quantity in this tree.
	sort.Float64s(outfieldWorst)
	var zeros int
	for _, v := range outfieldWorst {
		if v == 0 {
			zeros++
		}
	}
	if n := len(outfieldWorst); n > 0 {
		med := stats.Median(outfieldWorst)
		fmt.Printf("\nentry-deadline worst-case over the %d OUTFIELD season-positions: "+
			"median %.2f%%, largest %.2f%%, exactly zero in %d of them\n",
			n, med, outfieldWorst[0], zeros)
		fmt.Printf("⚠️ each of those %d is itself a maximum over six deadlines. PER CELL, of "+
			"the %d outfield entry cells: %d moved, %d fitted and did not, %d not fitted\n",
			n, eShift+eZero+eUnfit, eShift, eZero, eUnfit)
	}
	fmt.Printf("closed form -100*A_b/A asserted on %d cells where BOTH arms are on the plain "+
		"ratio\n", closedFormChecked)

	// ⚠️ **The distribution the replay actually stands at.** `Simulate` rebuilds this
	// engine at `through = gw-1` for EVERY gameweek from `start` to 38 — at
	// `simulate.go:1195` for the transfer decision, `:1259` for the weekly eleven and
	// `:1276` for `Hold` — so the six entry deadlines govern the opening fifteen and
	// nothing else. A cell entering at GW1 stands at every cutoff from 0 to 37,
	// including the early ones where the largest shifts live.
	//
	// So THIS is the headline distribution and the entry block above is a special
	// case. An earlier version of this file led with the entry deadlines, which
	// understated where the shift actually bites.
	sort.Float64s(allFitted)
	if n := len(allFitted); n > 0 {
		med := stats.Median(allFitted)
		var moved int
		for _, v := range allFitted {
			if v != 0 {
				moved++
			}
		}
		fmt.Printf("\nover ALL %d fitted outfield cutoff-cells, which is what the replay "+
			"stands at: median %.2f%%, non-zero in %d of them, largest %.2f%%\n",
			n, med, moved, allFitted[0])
	}

	// The liveness check. A count that is non-zero everywhere, or zero everywhere,
	// has discriminated nothing — the first would mean the criterion admits any
	// element and the second that this diagnostic never found its population. Both
	// halves must hold, and neither is guaranteed by construction.
	// ⚠️ Split, because a floored cell's exposure is unreachable by construction and
	// pooling the two invites reading the total as the population that can matter.
	fmt.Printf("\nliveness: %d of %d position-cutoff cells carry at least one exposed "+
		"element (%d of them GOALKEEPER cells, where the floor makes it unreachable, "+
		"leaving %d outfield); %d carry none\n",
		anyExposed, anyExposed+anyClean, gkpExposed, anyExposed-gkpExposed, anyClean)
	if anyExposed == 0 {
		t.Errorf("no cell anywhere carries an exposed element: the criterion found nothing, " +
			"so nothing above discriminates between the exposure existing and not")
	}
	if anyClean == 0 {
		t.Errorf("every cell carries an exposed element: the criterion admits the whole " +
			"population, so nothing above discriminates either")
	}
}

// checkAgainstEngine is the one-implementation guard. The sums above are a second
// traversal of Boot.Elements, so what they produce is checked against the engine
// itself at every cutoff this file reports on.
//
// It reads the engine through EngineAt rather than NewEngineFull, because that is
// the wiring Simulate uses and an unwired engine has already produced one wrong
// diagnosis in this package.
//
// # ⚠️ Two checks, because the scale ALONE has no power where the floor binds
//
// The obvious check is `PlayerMetrics.XGScale/XAScale` against `shippedScale`, and
// it is the one that matters for every shift printed here, since a shift is only
// read on a fitted cell. But `CalibrationRatio` is **constant at 1.0 below
// `minCalibrationSample`**, so on a floored cell that comparison is `1 == 1` for any
// sums whatsoever — and a floored cell is exactly where this file makes its
// strongest claim, that the keeper channel is absorbed entirely at all 39 cutoffs of
// all six seasons. Measured: zeroing every keeper's totals inside `engineFitInputs`
// left the whole test green and `gkpCensus` still printing its verdict.
//
// So the raw sums are checked too, and **deliberately by a second summation written
// out here rather than by calling `engineFitInputs`**. That is the one place in this
// file where duplicating a loop is correct: a guard that shares an implementation
// with the thing it guards cannot fail when that implementation is wrong, which is
// precisely the hole this closes. The standing rule against two implementations is
// about *quantities the code relies on*; a tripwire is the exception, and saying so
// is cheaper than the next reader "simplifying" it back.
func checkAgainstEngine(t *testing.T, pair seasonPair, thr int,
	sums map[int]*elementTotals, cfg config.Config) {
	t.Helper()
	e, eboot := EngineAt(pair.Cur, pair.Prior, thr, SimConfig{Weights: cfg.Weights})

	// The independent sums, over the ENGINE's own bootstrap.
	type raw struct{ goals, xG, assists, xA float64 }
	byPos := map[int]*raw{}
	for i := range eboot.Elements {
		el := &eboot.Elements[i]
		r := byPos[el.ElementType]
		if r == nil {
			r = &raw{}
			byPos[el.ElementType] = r
		}
		r.goals += float64(el.GoalsScored)
		r.xG += el.ExpectedGoals.Float()
		r.assists += float64(el.Assists)
		r.xA += el.ExpectedAssists.Float()
	}

	seen := map[int]bool{}
	for i := range eboot.Elements {
		el := &eboot.Elements[i]
		if seen[el.ElementType] {
			continue
		}
		seen[el.ElementType] = true
		tt := sums[el.ElementType]
		if tt == nil {
			t.Fatalf("%s thr=%d: the engine carries element_type %d and this file's "+
				"traversal does not", pair.Name, thr, el.ElementType)
		}

		// The raw sums, which have power on a floored cell where the scale has none.
		// Bit-for-bit: both walk eboot.Elements in the same order, so any difference
		// is a real one rather than a reassociation.
		r := byPos[el.ElementType]
		if tt.goals != r.goals || tt.xG != r.xG || tt.assists != r.assists || tt.xA != r.xA {
			t.Fatalf("%s thr=%d pos %d: this file's traversal sums "+
				"%.17g/%.17g/%.17g/%.17g (goals/xG/assists/xA) and an independent walk "+
				"of the engine's own bootstrap gives %.17g/%.17g/%.17g/%.17g. Every "+
				"count and every census in this file is over the wrong population",
				pair.Name, thr, el.ElementType,
				tt.goals, tt.xG, tt.assists, tt.xA,
				r.goals, r.xG, r.assists, r.xA)
		}

		// And the scale the engine actually hands to baseXP90.
		want := shippedScale(tt)
		m := e.Metrics(el)
		if m.XGScale != want.Goals || m.XAScale != want.Assists {
			// Printed to the last bit and with the difference beside it, because the
			// two disagree by design only in the last bits and a four-decimal message
			// would show a mismatch as two identical numbers.
			t.Fatalf("%s thr=%d pos %d: the engine fits %.17g/%.17g and this file's "+
				"traversal of the same bootstrap gives %.17g/%.17g (differing by "+
				"%.3g/%.3g). The two populations have diverged, so every shift printed "+
				"here is a shift in something other than the scale Score reads",
				pair.Name, thr, el.ElementType, m.XGScale, m.XAScale,
				want.Goals, want.Assists,
				m.XGScale-want.Goals, m.XAScale-want.Assists)
		}
	}
}
