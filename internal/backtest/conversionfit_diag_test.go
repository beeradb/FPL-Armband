package backtest

// What the exposed rows do to the FITTED conversion scale, and therefore to
// everybody else's residual.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagExposedReturnScaleShift -v
//
// # The quantity, and why it is not the one already banked
//
// `TestDiagPostRepairAttackingExposure` sizes the exposed ROWS themselves — a
// realised return that `XPointsResidual` prices with no expectation subtracted —
// and `TestDiagResidualXGCoverage` reports their share of residual mass at
// **0.90-2.06% by season** (`%degen`, six-season post-repair grid; 129-264
// points). That is the direct cost, carried by those rows.
//
// ⚠️ **The two ranges are not co-indexed, and quoting them as one interval is a
// mistake this comment made once.** `%degen` bottoms at 0.90% on 2023-24, which
// is 132 points; `degen_pts` bottoms at 129 on 2024-25, which is 0.91%. Different
// rows. Take a pair from a row, never an endpoint from each range.
//
// This asks what they do to everyone else. `Season.conversionFit` is a ratio of
// realised over expected over the whole league. An exposed row puts its realised
// return in the NUMERATOR and nothing in the denominator, so the fitted scale for
// the position comes out higher — and a higher scale subtracts more expectation
// from every OTHER row of that position, including all the well-measured ones.
//
// # The conservation identity
//
// Write the fit as `S = A/X`, with `A` the position-season's realised events on
// counted rows and `X` its expected total. An exposed row contributes `A_b` to
// `A` and exactly nothing to `X`, so `S' = (A − A_b)/X` and `ΔS = −A_b/X`. Every
// row's residual moves by `−ΔS · xᵢ · pts`, all of one sign, and summing over the
// position gives `(A_b/X) · X · pts = A_b · pts`.
//
// ⚠️ **The verb is "offset", not "relocated", and an earlier draft of this
// comment had it wrong.** Nothing moves off the exposed rows: an exposed row's
// residual is `Assists · pts` and is INVARIANT to the scale, because its own xA
// is zero. What the in-sample fit does is force the position's attacking residual
// to sum to zero, so the `+A_b·pts` sitting on the exposed rows is exactly
// cancelled by `−A_b·pts` spread across the well-measured ones. `dropExposedReturns`
// deletes the compensation and leaves the exposed rows untouched.
//
// So the two costs are **equal in magnitude and opposite in sign**. What follows
// is that they may not be ADDED — not that they are interchangeable. They land on
// disjoint row sets at wildly different concentration: 42-86 rows a season
// (`A_rw`, summed over positions) at `+pts` each, against several thousand at
// −0.01 to −0.05 each. Every consumer of this instrument reads a per-row or
// per-package value rather than a position total, so equal totals do not make the
// two fungible.
//
// ⚠️ **And it is false for keepers.** `minCalibrationSample` floors GKP at 1.0, so
// a keeper carries the direct cost with NO offsetting term at all. That shows up
// as a non-zero `atk_tot` under BOTH arms: 2.58 to 28.07 points a season, and
// 2024-25's +0.0369 per appearance exceeds SEVENTEEN of the eighteen offsets
// `drop` would introduce for an outfielder — every one but 2020-21 FWD at 0.0483.
// State the identity as "equal and opposite in the eighteen FITTED cells"; in GKP
// the direct cost stands alone. ⚠️ An earlier draft said "larger than anything",
// which the table below contradicts.
//
// That the FLOOR does it, rather than the clamp, is settled by arithmetic rather
// than inferred from the scale printing 1.000. `minCalibrationSample` is 20.0 and
// GKP season xA over this population is 0.954 / 1.100 / 1.410 / 1.400 for
// 2022-23 to 2025-26 — an order of magnitude under it. And the clamp is excluded
// rather than merely unlikely: unfloored, 2024-25 GKP would fit 11/1.410 = 7.8,
// which clamps to 3.0, not 1.0. So a printed 1.000 identifies the floor uniquely.
//
// # ⚠️ `drop` BREAKS the in-sample identity, which is why it is not a proposal
//
// `xpoints.go` rests a whole section on the fit being IN SAMPLE — "the
// position-mean attacking residual afterwards is exactly zero, by arithmetic
// rather than approximately", which is what makes the instrument report
// within-position deviation only. That holds because the numerator sums the SAME
// rows the residual is then evaluated on. Fitting on a strict subset and scoring
// on everything breaks the correspondence and reintroduces a per-position level
// offset of +0.0088 to +0.0483 per appearance, in all eighteen fitted cells.
//
// The identity needs TWO conditions and an earlier draft named only one:
// `CalibrationRatio` must have returned the plain ratio (no floor, no clamp), AND
// the residual must be evaluated on exactly the rows the fit summed — `atk` sums
// every row with minutes, while the fit sums only rows in a gameweek the season
// RECORDS the channel for. Post-repair those coincide. **Pre-repair they do
// not**: under `FPL_NO_XG_REPAIR=1`, 2020-21 and 2021-22 record no xG or xA at
// all, every position falls back to 1.0, and `atk_tot` reads 1410 to 4530 points
// — the raw unscaled attacking residual, since no scale is subtracting anything.
// Attributing a non-zero row to the keeper floor alone is wrong in that state,
// which is why the assertion is gated on `fullyCovered` as well.
//
// It is ASSERTED rather than eyeballed, on the total rather than the printed
// per-appearance mean. Four printed decimals establish only `|mean| < 5e-5`,
// which over 3,745 defender appearances permits 0.19 points of un-cancelled
// residual — three orders of magnitude short of "exactly zero, by arithmetic".
//
// ⚠️ **The property is OWNED by `TestTheFittedScaleZeroesThePositionMeanAttackingResidual`**,
// which pins it on a fixture, weights both channels 1 and 1 so no scoring
// constant is copied, and carries a crossed-scale mutation guard. Do not delete
// that test on the strength of this one, or the reverse. They are not the same
// check: it proves the identity holds and cannot be faked; this one establishes
// that it holds over the REAL archive population the verdict here is about, which
// a fixture cannot say. The arm the shipped path resolves is pinned separately
// again, by `TestTheShippedFitCountsTheExposedReturns` — and it had to be, because
// the fixture those tests share contains no exposed row, so both were byte-identical
// across the two arms and neither could see the flip.
//
// # ⚠️ The exact-zero test is a DISPLAY threshold, and that is the decisive fact
//
// The archive publishes `expected_assists` to **two decimal places**, and the
// smallest non-zero value in 2024-25's `merged_gw.csv` is exactly 0.01. So
// `XA == 0` does not mean "no expectation": it conflates a genuine zero, a true
// xA below 0.005 rounded away by the file format, and — in repaired seasons — a
// harvest attribution failure. It is a quantization boundary, not a semantic one.
//
// The third table measures what that costs. The bucket just ABOVE the knife edge
// carries several times as many assists as the exact-zero bucket while
// contributing a small share of the position's xA, so it inflates the ratio
// nearly as much per assist — and no variant of the repair touches it. Measured
// `zero_share`, the share of near-zero-expectation assists the exact-zero test
// captures, outfield, post-repair:
//
//	2020-21  52.1%     2022-23  23.8%     2024-25  15.0%
//	2021-22  46.4%     2023-24  15.6%     2025-26  17.4%
//
// **That argument stands on its own.** It does not depend on the mechanism story,
// it does not depend on the in-sample identity, and it applies to every variant
// below unless someone first defines the population on a THRESHOLD rather than on
// `== 0`.
//
// ⚠️ **And the ordering is the repair windows**, which is a second finding rather
// than a caveat. The two fully Understat-fed seasons reach ~half, 2022-23 (repaired
// GW1-15) sits between, and the three FPL-fed seasons reach ~15-17% — with 2022-23
// falling to 14.6% under `FPL_NO_XG_REPAIR=1`, i.e. to its native half alone. So
// the exact-zero test is a far better proxy on Understat's key-pass xA, which
// genuinely returns 0, than on FPL's `expected_assists`, which returns 0.01-0.05.
// ⚠️ **That does NOT weigh the instrument-mismatch reading of the banked Fisher
// OR of 2.02 against its alternative, and it is not independent of it** — the
// `zero_share` numerator is the OR's own exposed population, re-described against
// a new denominator. The ordering follows mechanically from the two feeds'
// publication conventions, so it would appear whether or not the OR reflects real
// football, and something near-certain under both hypotheses discriminates
// nothing. An earlier draft called it "independent evidence for"; withdrawn on
// both words.
//
// What it does establish is that the confound is **demonstrated and unquantified**
// rather than conceivable, and that the OR's standing as a COVERAGE statement is
// withdrawn: a ratio near 2 between a repaired and a native window is what
// quantization alone would produce with no difference in the football. It does not
// refute an underlying coverage difference either — the two cannot be separated on
// a population defined by `== 0`.
//
// # What this does NOT dispose of
//
// The verdict is about the variant measured here — fit on a subset, score on
// everything. Two others are untouched by it and are neither built nor measured:
//
//   - **Drop from the fit AND from the scoring.** Fitting `S' = (A−A_b)/X` and
//     evaluating over the same subset restores the zero-mean identity exactly.
//     Its cost is that `xPointsOver` stops being a total over the window.
//   - **Gate the assist channel on `XA > 0`**, the shape the clean sheet already
//     uses for `XGC > 0`. `xpoints.go` argues for that shape in its own voice —
//     "this file's whole design is that a channel it cannot get right is one it
//     does not replace". It would remove the original hindsight, which neither
//     the shipped fit nor the variant here touches.
//     ⚠️ **The display-threshold argument does NOT reach `XGC > 0`, and someone
//     will try to carry it there.** On gameweeks recording xGC, rows with minutes
//     and `xgc == 0` number 427/524/450 a season for 2023-24 to 2025-26 — but at
//     60+ minutes, the only rows FPL pays a clean sheet on, they number **2, 0
//     and 6**. A played hour essentially always carries at least 0.01, so the
//     quantization boundary does not bite there.
//     The counter is that
//     `XGC == 0` is missing data while `XA == 0` on a won penalty is a CORRECT
//     zero. That is the crux and nobody has measured it.
//
// # It cannot move replayed points, and that is a code fact
//
// `Player.Conversion` is read at exactly one non-test site, `xPointsOf`
// (`simulate.go`). So no setting of THIS fit can move `hold_points` or
// `policy_points` at shipped config; its only decision channel is the gate oracle
// arms. That converts the whole question from a points question into an estimand
// one — and it means a change here would supersede every banked `hold_xpoints`
// and `policy_xpoints` figure and the 0.6402 recovered fraction at
// `stats/snapshots/2026-08-15-gatescaled/`.
//
// # ⚠️ THERE ARE TWO CONVERSION SCALES, AND THE OTHER ONE IS LIVE ON `Score`
//
// This file's verdict is about `Season.conversionFit` and reaches no further.
// `Engine.calibrateExpectedStats` (`analysis/metrics.go`) builds `e.xScale[pos]`
// through the SAME `CalibrationRatio`, over ungated season-to-date element
// totals — and `scaleFor` is read by `baseXP90` and `fixtureSensitiveAt`, which
// multiply `XA90` by `sc.Assists`. So the exposed-return construction exists on
// the SCORING path too: an FPL assist whose `expected_assists` is zero enters
// that numerator with nothing behind it in the denominator, exactly as here.
//
// Its exposure has **never been sized**, and the population differs — the engine
// fits over per-element season aggregates rather than per-gameweek rows, so an
// "exposed element" there is a player whose whole season xA is zero, which is
// rarer and not measured by anything in this file. Do not carry any number from
// this file to that scale, and do not read "cannot move replayed points" as
// covering both. Naming them apart is the check.
//
// # ⚠️ The goal channel is inert in BOTH data states, and it is forced
//
// `G_ev` is 0 in every cell under the default AND under `FPL_NO_XG_REPAIR=1`.
// That is not a coverage claim: both `dropExposedReturns` and `exposureTally`
// gate on `recordsXG[gw]`, and `underlyingCoverage` already excludes an uncovered
// gameweek from the fit entirely.
//
// ⚠️ So the sibling's pre-repair 868/924/327 rows **must not be quoted here.**
// They are its UNGATED population — rows in gameweeks the season records no xG
// for, which is exactly what this fit already refuses. Importing that count would
// tell a reader to expect a movement this table can never produce, which is the
// "byte-identical null that reads as a result" shape. An earlier draft imported it.
//
// # Two definitional cautions
//
// Rows are GAMEWEEK-ACCUMULATED, so "a row with assists > 0 and XA == 0" is
// match-ambiguous: a double gameweek pairing `(1 assist, xA 0.03)` with
// `(1 assist, xA 0.00)` accumulates to `XA = 0.03` and is not counted, though the
// match-level truth holds one exposed assist. The arm is self-consistent, because
// the fit sees the same accumulated rows, but the population is not the
// match-level one.
//
// And nothing is replayed, so no detection threshold applies to any level printed
// here. These are properties of the instrument.
//
// # It does not mutate the loaded season
//
// `harness_test.go`'s parsed-season cache is process-global and its callers must
// treat a `*Season` as read-only, because a diagnostic that edited one would
// silently change what every later test in the process measures. So the refitted
// arm is read through a per-player STRUCT COPY carrying the alternative scale,
// and the cached season is never written to.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// exposureTally counts the realised goals and assists on rows the season's own
// coverage says it records the channel for, and which carry no underlying of
// their own — the exact population `dropExposedReturns` removes.
//
// It counts EVENTS and ROWS separately, because a row can carry two assists while
// the fit's numerator counts events. Reading one as the other is how the sibling
// table's first report got 225 where its column said 228.
//
// The POINTS those events are worth come from `degenerateAttacking`, which
// differences the real residual against the same row with the return removed. A
// literal points value here would be a second copy of a per-season scoring
// constant — the 2026/27 rules already make `XPointsRules.Assist` per-season —
// and nothing would notice the day it changed.
type exposureTally struct {
	goals, assists         map[int]int
	goalRows, assistRows   map[int]int
	goalPoints, assistPnts map[int]float64
}

func newExposureTally(s *Season) exposureTally {
	recordsXG, recordsXA := s.underlyingCoverage()
	b := exposureTally{
		goals: map[int]int{}, assists: map[int]int{},
		goalRows: map[int]int{}, assistRows: map[int]int{},
		goalPoints: map[int]float64{}, assistPnts: map[int]float64{},
	}
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		for gw := 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.Minutes <= 0 {
				continue
			}
			gp, ap := degenerateAttacking(p, g)
			if recordsXG[gw] && g.XG == 0 && g.Goals > 0 {
				b.goals[p.Type] += g.Goals
				b.goalRows[p.Type]++
				b.goalPoints[p.Type] += gp
			}
			if recordsXA[gw] && g.XA == 0 && g.Assists > 0 {
				b.assists[p.Type] += g.Assists
				b.assistRows[p.Type]++
				b.assistPnts[p.Type] += ap
			}
		}
	}
	return b
}

// fullyCovered reports whether every row with minutes sits in a gameweek this
// season records the channel for — one of the two conditions the in-sample
// identity needs, and the one that fails under FPL_NO_XG_REPAIR.
func fullyCovered(s *Season) bool {
	recordsXG, recordsXA := s.underlyingCoverage()
	for _, p := range s.Players {
		for gw, g := range p.GWs {
			if g.Minutes > 0 && (!recordsXG[gw] || !recordsXA[gw]) {
				return false
			}
		}
	}
	return true
}

func TestDiagExposedReturnScaleShift(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)

	state := "POST-repair (default)"
	if noXGRepair() {
		state = "PRE-repair (FPL_NO_XG_REPAIR=1, xGC reconstruction also off)"
	}
	fmt.Printf("\n=== how far the conversion scale moves if exposed returns leave the fit\n")
	fmt.Printf("data state: %s\n\n", state)
	fmt.Printf("An EXPOSED return is a realised goal or assist on a row whose season\n")
	fmt.Printf("records that channel and whose own underlying is zero. Shipped, it\n")
	fmt.Printf("enters the fit's numerator with nothing in the denominator; `drop`\n")
	fmt.Printf("removes it from that channel only. `ev` is EVENTS and `rw` is ROWS.\n\n")
	fmt.Printf("⚠️ `G_ev` is 0 in every cell in BOTH data states, and that is forced\n")
	fmt.Printf("rather than measured: the fit already excludes a gameweek the season\n")
	fmt.Printf("records no xG for. The goal arm is INERT here. Do not quote the\n")
	fmt.Printf("sibling table's pre-repair row counts against this one — they are its\n")
	fmt.Printf("ungated population, which is a different question.\n\n")

	fmt.Printf("%-9s %-4s | %6s %6s %7s %5s | %6s %6s %7s %5s %5s\n",
		"season", "pos",
		"G_ship", "G_drop", "G_d%", "G_ev",
		"A_ship", "A_drop", "A_d%", "A_ev", "A_rw")
	for _, pair := range pairs {
		s := pair.Cur
		ship := s.conversionFit(countExposedReturns)
		drop := s.conversionFit(dropExposedReturns)
		exp := newExposureTally(s)

		for _, pos := range sortedPositions(ship) {
			a, b := ship[pos], drop[pos]
			fmt.Printf("%-9s %-4s | %6.3f %6.3f %6.2f%% %5d | %6.3f %6.3f %6.2f%% %5d %5d\n",
				s.Name, posShort(pos),
				a.Goals, b.Goals, pctShift(a.Goals, b.Goals), exp.goals[pos],
				a.Assists, b.Assists, pctShift(a.Assists, b.Assists),
				exp.assists[pos], exp.assistRows[pos])
		}
	}

	fmt.Printf("\n=== what that does to the instrument, per position-season\n\n")
	fmt.Printf("`|atk|` is total ABSOLUTE attacking residual in points — the honest\n")
	fmt.Printf("denominator, because the clean-sheet and concede channels cannot move\n")
	fmt.Printf("here and dominate a defender's total residual while a forward has no\n")
	fmt.Printf("clean-sheet channel at all. `moved` is the total absolute change under\n")
	fmt.Printf("`drop`, and `%%moved` is that as a share of `|atk|`.\n\n")
	fmt.Printf("`atk_tot` is the SIGNED total attacking residual under the shipped fit.\n")
	fmt.Printf("It is zero BY ARITHMETIC wherever two conditions hold: CalibrationRatio\n")
	fmt.Printf("returned the plain ratio (no floor, no clamp), and the season records\n")
	fmt.Printf("both channels in every gameweek with rows, so the rows the fit summed\n")
	fmt.Printf("are the rows the residual is evaluated on. That is the in-sample identity\n")
	fmt.Printf("xpoints.go calls the property making this instrument report WITHIN-position\n")
	fmt.Printf("deviation only, and it is asserted below, not merely printed.\n\n")
	fmt.Printf("`atk/app'` is the mean attacking residual per appearance under `drop` —\n")
	fmt.Printf("the per-position level offset the refit REINTRODUCES.\n\n")
	fmt.Printf("⚠️ Where `atk_tot` is non-zero, one of the two conditions failed: GKP\n")
	fmt.Printf("always (minCalibrationSample floors its scale at 1.0, so a keeper carries\n")
	fmt.Printf("the exposed rows' cost with NO offset), and under FPL_NO_XG_REPAIR=1 every\n")
	fmt.Printf("position of a season recording no xG at all.\n\n")
	fmt.Printf("`per_xA` is the decision-scaled unit: points per unit of xA DIFFERENCE\n")
	fmt.Printf("between two players of this position, over whatever window is summed.\n")
	fmt.Printf("⚠️ Within a position the refit is a REWEIGHTING BY xA, not a level — it\n")
	fmt.Printf("reorders whenever two players differ in xA, which is AGENTS.md's own\n")
	fmt.Printf("qualification of 'a bias shared by a position is not an ordering error'.\n")
	fmt.Printf("The distribution of (xA_in - xA_out) over real packages is UNMEASURED,\n")
	fmt.Printf("and xpoints.go has already retracted one claim for resting on a\n")
	fmt.Printf("distribution of package margins nobody produced. Do not repeat it.\n\n")
	fmt.Printf("⚠️ No cross-position column is printed, deliberately. xpoints.go says\n")
	fmt.Printf("perfectGateXPoints compares packages whose in and out players 'routinely\n")
	fmt.Printf("differ in position'; the source refuses that. RankSwaps skips a candidate\n")
	fmt.Printf("whose Position differs from the man leaving, RankPairs draws BOTH the\n")
	fmt.Printf("upgrade and every funding downgrade from frontier[cur.Position] (all\n")
	fmt.Printf("analysis/swaps.go), and diffSquads returns nil rather than emit a package\n")
	fmt.Printf("whose positions do not reconcile (backtest/unified.go). Structurally it\n")
	fmt.Printf("cannot be otherwise: squadQuota fixes 2/5/5/3, so every legal package has\n")
	fmt.Printf("the same position multiset on both sides and perfectGateXPoints — which\n")
	fmt.Printf("sums xPointsOver(in) minus xPointsOver(out) over the package — cancels any\n")
	fmt.Printf("per-position level exactly, however the moves are paired.\n\n")

	fmt.Printf("%-9s %-4s | %7s %9s %8s %7s %10s %9s %8s\n",
		"season", "pos", "rows", "|atk|", "moved", "%moved",
		"atk_tot", "atk/app'", "per_xA")
	for _, pair := range pairs {
		s := pair.Cur
		ship := s.conversionFit(countExposedReturns)
		drop := s.conversionFit(dropExposedReturns)
		exp := newExposureTally(s)
		covered := fullyCovered(s)

		// Both arms are read through xPointsOf, the ONE mapping from an archive
		// row to the instrument's input. The cached season is never written to.
		shipped := residualByPosition(t, s, ship)
		refitted := residualByPosition(t, s, drop)

		for _, pos := range sortedPositions(ship) {
			a, b := shipped[pos], refitted[pos]
			if a == nil || a.rows == 0 {
				// A position the fit knows about but nobody played a minute in.
				// element_type 5 — the 2024-25 assistant managers — is exactly
				// this: 322 archive rows, none of them with minutes.
				continue
			}
			// b is non-nil with the same rows and the same keys: the population
			// residualByPosition builds is a function of p.Type and g.Minutes
			// alone, and the conversion scale touches neither.
			var moved float64
			for key, r := range a.byRow {
				moved += abs(b.byRow[key] - r)
			}
			pct := 0.0
			if a.atkMass > 0 {
				pct = 100 * moved / a.atkMass
			}

			// ⚠️ The floor's sentinel is a neutral 1.0 in both channels, which is
			// how a floored cell is recognised here without a second copy of
			// minCalibrationSample. A clamped cell is NOT recognised, and would
			// fail the assertions below; nothing is near the [0.5, 3.0] bounds
			// today (FWD assists run 1.96-2.13), so read a failure as "check the
			// clamp" before reading it as a broken fit.
			neutral := analysis.ConversionScale{Goals: 1, Assists: 1}
			fitted := covered && ship[pos] != neutral

			if fitted {
				// The in-sample identity, on the TOTAL. A per-appearance mean
				// printed to four decimals would pass at a thousand times this
				// tolerance and establish nothing.
				if abs(a.atkSigned) > 1e-9*(1+a.atkMass) {
					t.Errorf("%s %s: the shipped fit leaves %.6f points of signed "+
						"attacking residual over %d appearances, where the in-sample "+
						"fit makes it exactly zero. Check CalibrationRatio's clamp "+
						"first; otherwise the property xpoints.go rests its "+
						"within-position reading on does not hold",
						s.Name, posShort(pos), a.atkSigned, a.rows)
				}

				// The conservation identity. BOTH channels, because `moved` is
				// over both: an assists-only expectation would error falsely the
				// day the goal channel stops being inert, and would be skipped
				// silently if only the goal scale moved.
				want := exp.goalPoints[pos] + exp.assistPnts[pos]
				if abs(moved-want) > 1e-9*(1+abs(want)) {
					t.Errorf("%s %s: the refit moved %.6f points of residual but the "+
						"exposed returns are worth %.6f. Check CalibrationRatio's "+
						"clamp first; otherwise the fit is no longer a plain ratio "+
						"over one denominator, and the reading that the two costs "+
						"are equal and opposite does not survive that",
						s.Name, posShort(pos), moved, want)
				}
			}

			// pts is derived from the exposed rows rather than written down, for
			// the reason exposureTally gives. Undefined where there are none —
			// which is also exactly where the scale did not move.
			perXA := 0.0
			if n := exp.assists[pos]; n > 0 {
				pts := exp.assistPnts[pos] / float64(n)
				perXA = pts * abs(drop[pos].Assists-ship[pos].Assists)
			}
			fmt.Printf("%-9s %-4s | %7d %9.1f %8.1f %6.2f%% %10.6f %9.4f %8.4f\n",
				s.Name, posShort(pos), a.rows, a.atkMass, moved, pct,
				a.atkSigned, b.atkSigned/float64(b.rows), perXA)
		}
	}

	reportAssistKnifeEdge(t, pairs)

	fmt.Printf("\n⚠️ Nothing here is a points figure. No cell was replayed, so no\n")
	fmt.Printf("detection threshold applies and none of this sizes an arm.\n")
	fmt.Printf("\n⚠️ `drop` is NOT a proposal, and the record already argues against it:\n")
	fmt.Printf("see underlyingCoverage. Two variants it does NOT dispose of — dropping\n")
	fmt.Printf("from the fit AND the scoring, and gating the assist channel on XA > 0 —\n")
	fmt.Printf("are named in this file's header, unbuilt and unmeasured.\n")
}

// knifeEdgeBand is the upper edge of the "near zero but recorded" bucket.
//
// ⚠️ ASSERTED, not measured: five times the archive's own quantum of 0.01, chosen
// to be small enough that a row inside it is indistinguishable in kind from a row
// rounded to zero. Nothing calibrates it, and the point it supports — that the
// exact-zero test lands on a display boundary — does not depend on where exactly
// it sits, only on the bucket above zero being populated.
const knifeEdgeBand = 0.05

// reportAssistKnifeEdge shows what the `XA == 0` test reaches and what it misses.
func reportAssistKnifeEdge(t *testing.T, pairs []seasonPair) {
	t.Helper()
	fmt.Printf("\n=== the exact-zero test is a DISPLAY threshold\n\n")
	fmt.Printf("The archive publishes expected_assists to TWO DECIMAL PLACES; the\n")
	fmt.Printf("smallest non-zero value in the file is 0.01. So `XA == 0` conflates a\n")
	fmt.Printf("genuine zero with a true xA below 0.005 rounded away by the format,\n")
	fmt.Printf("and in repaired seasons with a harvest attribution failure. It is a\n")
	fmt.Printf("quantization boundary, not a semantic one.\n\n")
	fmt.Printf("Outfield rows (element_type 2-4) with minutes, in gameweeks the season\n")
	fmt.Printf("records xA for — the fit's own population. `zero_share` is the share of\n")
	fmt.Printf("near-zero-expectation assists the exact-zero test actually captures:\n")
	fmt.Printf("zero / (zero + band). A low share means every variant of the repair\n")
	fmt.Printf("acts on a minority of the phenomenon it is named after.\n\n")
	fmt.Printf("⚠️ The band edge %.2f is ASSERTED, not measured — five times the\n", knifeEdgeBand)
	fmt.Printf("archive's quantum. The argument needs the bucket to be populated, not\n")
	fmt.Printf("the edge to be in any particular place.\n\n")

	fmt.Printf("%-9s | %8s | %8s %8s | %8s %8s | %7s\n",
		"season", "as@xA=0", "as@band", "xA@band", "as@above", "xA@above", "zero_share")
	for _, pair := range pairs {
		s := pair.Cur
		_, recordsXA := s.underlyingCoverage()
		var zero, band, above int
		var bandXA, aboveXA float64
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			if p.Type < 2 || p.Type > 4 {
				continue
			}
			for gw := 1; gw <= 38; gw++ {
				g, ok := p.GWs[gw]
				if !ok || g.Minutes <= 0 || !recordsXA[gw] {
					continue
				}
				switch {
				case g.XA == 0:
					zero += g.Assists
				case g.XA <= knifeEdgeBand:
					band += g.Assists
					bandXA += g.XA
				default:
					above += g.Assists
					aboveXA += g.XA
				}
			}
		}
		zeroShare := 0.0
		if zero+band > 0 {
			zeroShare = 100 * float64(zero) / float64(zero+band)
		}
		fmt.Printf("%-9s | %8d | %8d %8.2f | %8d %8.2f | %6.1f%%\n",
			s.Name, zero, band, bandXA, above, aboveXA, zeroShare)
	}
}

// residualPos is one position's conversion residual under a given scale.
type residualPos struct {
	rows int
	// atkMass and atkSigned are the ATTACKING channels alone: the absolute total,
	// which is the only honest denominator for a change this fit can make, and
	// the signed total, which the in-sample fit drives to exactly zero.
	atkMass, atkSigned float64
	byRow              map[rowKey]float64
}

// rowKey identifies a player-gameweek, so the two arms can be differenced row by
// row rather than only in aggregate. An aggregate difference would net a rise on
// one row against a fall on another and report a movement of zero where the
// instrument had moved for everybody.
type rowKey struct {
	id, gw int
}

// attackingOf is the goals-and-assists half of a row's residual, taken by
// differencing the real function against the same row with both realised counts
// and both underlying rates zeroed.
//
// Differenced rather than re-expressed for the reason `degenerateAttacking`
// gives: `analysis` keeps the per-position points tables unexported and a second
// copy of one in a diagnostic is this record's signature failure. Verified in
// review against XPointsResidual: the clean-sheet branch reads XGC, Minutes,
// CleanSheets, Fixtures and Position, the concede branch reads GoalsConceded,
// XGC, Fixtures and Position, and neither gate is disturbed by zeroing the four,
// so both defensive terms cancel exactly. Zeroing XG and XA can only relax the
// missing-scale panic, never trip it.
func attackingOf(p *Player, g GW) float64 {
	z := g
	z.Goals, z.Assists, z.XG, z.XA = 0, 0, 0, 0
	return residualOf(p, g) - residualOf(p, z)
}

// residualByPosition totals the conversion residual under `scales`, WITHOUT
// touching the season.
//
// The cached *Season is shared process-wide and is contractually read-only, so
// the alternative scale is carried on a per-player struct copy. The copy shares
// GWs and Rules by reference and neither is written; only Conversion differs,
// which is the single field xPointsOf resolves the scale through.
func residualByPosition(t *testing.T, s *Season, scales map[int]analysis.ConversionScale) map[int]*residualPos {
	t.Helper()
	out := map[int]*residualPos{}
	for _, id := range sortedSeasonPlayerIDs(s) {
		p := s.Players[id]
		sc, ok := scales[p.Type]
		if !ok {
			// conversionFit creates an accumulator for every element_type
			// present among the players before it looks at a gameweek, so every
			// such position has an entry. Asserted rather than defaulted to
			// neutral: a silent 1.0 here would be a second copy of
			// calibrateConversion's fallback policy, and would read as a real
			// measurement.
			t.Fatalf("%s: the fit has no scale for element_type %d, which "+
				"conversionFit is supposed to cover", s.Name, p.Type)
		}
		shadow := *p
		shadow.Conversion = sc
		for gw := 1; gw <= 38; gw++ {
			g, ok := p.GWs[gw]
			if !ok || g.Minutes <= 0 {
				continue
			}
			r := out[p.Type]
			if r == nil {
				r = &residualPos{byRow: map[rowKey]float64{}}
				out[p.Type] = r
			}
			atk := attackingOf(&shadow, g)
			r.rows++
			r.atkMass += abs(atk)
			r.atkSigned += atk
			r.byRow[rowKey{id, gw}] = residualOf(&shadow, g)
		}
	}
	return out
}

func sortedPositions(m map[int]analysis.ConversionScale) []int {
	out := make([]int, 0, len(m))
	for pos := range m {
		out = append(out, pos)
	}
	sort.Ints(out)
	return out
}

// pctShift is drop against shipped as a percentage, and returns 0 where there is
// no shipped value to be a percentage of.
func pctShift(shipped, dropped float64) float64 {
	if shipped == 0 {
		return 0
	}
	return 100 * (dropped - shipped) / shipped
}
