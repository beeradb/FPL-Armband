package backtest

// Whether the residual criterion is a residual at all, season by season.
//
//	DIAG=1 scripts/replay -run TestDiagResidualXGCoverage -v -timeout 20m
//
// # Why this gates the fourth gate arm rather than merely annotating it
//
// AxisTransferGateResidual accepts on the sign of `realised − xPoints`, which
// `analysis.XPointsResidual` computes as `(Goals − XG)*goalPoints + (Assists −
// XA)*assistPoints` plus the two defensive channels. On a row where **XG is zero**
// that first term is not a residual: it is `Goals × goalPoints`, which is "did he
// score" — the strongest hindsight anywhere in this catalogue, and precisely the
// artefact the arm exists to rule out. A season made mostly of such rows would
// return an arm that is a near-copy of the points arm and looks exactly like the
// finding it was built to test for.
//
// It matters more here than for any sibling for a second reason. R is a difference
// of two similar-sized quantities, so relative error in xG passes through it almost
// undiluted, where the same error is diluted in X by everything xPoints keeps
// realised.
//
// # What this measures, and what it does not
//
// It counts **archive rows**, not the legs the gate actually judged. The judged
// population is not recoverable without threading a counter through
// `perfectGateResidual`, which runs inside cells that execute concurrently — a
// shared counter there is the fatal concurrent-map-write class this project has
// already taken a live run down with, and a per-cell one is plumbing through
// SimConfig for a diagnostic. The archive population is a superset and strictly
// more conservative: if coverage is clean over every appearance, it is clean over
// any subset of them.
//
// ⚠️ It is therefore a screen and not a measurement of the arm. A clean table
// licenses reading the pooled figure; a dirty one condemns it. It cannot tell you
// how much of a dirty season's gain came from the degenerate rows.
//
// Two cuts, because the transferable population is not the appearance population:
// every row with minutes, and every row with an hour. A 12-minute cameo has near-
// zero xG legitimately, and counting it as missing coverage would overstate the
// problem on exactly the rows no transfer is ever made for.
//
// The share of residual MASS is the decisive column rather than the row count.
// Zero-xG rows that are also zero-goal rows contribute nothing to the criterion,
// so a season can be 40% uncovered by rows and unaffected in what the gate reads.

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

// residualOf is a row's conversion residual, taken as Points minus xPoints rather
// than by calling analysis.XPointsResidual directly.
//
// Deliberate: xPointsOf is the ONE mapping from an archive row to the instrument's
// input, and a second literal here would be a transposition nobody would catch —
// the exact failure that comment records. This composes the mapping instead, so
// this diagnostic reads the same quantity the gate does by construction and cannot
// carry its own copy of the thing it is checking.
func residualOf(p *Player, g GW) float64 {
	return float64(g.Points) - xPointsOf(p, g)
}

// degenerateAttacking is what a row's REALISED attacking returns contribute to
// the residual with no expectation subtracted from them: the goal channel where
// the row carries a goal and no xG, and the assist channel where it carries an
// assist and no xA. Both are zero on a row that has the underlying to be a
// residual of.
//
// This is the ungated half of `analysis.XPointsResidual`. The clean sheet is
// gated on `XGC > 0` because a zero xGC is missing data rather than a certain
// clean sheet; the two attacking channels have no equivalent gate, so on a
// zero-underlying row `(Goals − XG*s)*goalPoints` collapses to `Goals*goalPoints`
// — "did he score", with nothing subtracted.
//
// ⚠️ A zero here is NOT by itself a defect, which is the reading the exposure
// tables below stand or fall on. A won penalty and a deflected assist genuinely
// carry ~0 expectation, so most of this is legitimate. What the size of it bounds
// is how much of the criterion could be hindsight, not how much is.
//
// Taken by DIFFERENCING the real function against the same row with the return
// removed, never by a local copy of the per-position points table: analysis keeps
// that table unexported, and a second copy of a scoring constant in a diagnostic
// is this record's signature failure in the worst place for it — nothing else
// would notice the day FPL reprices a midfielder's goal.
func degenerateAttacking(p *Player, g GW) (goalPts, assistPts float64) {
	r := residualOf(p, g)
	if g.XG == 0 && g.Goals > 0 {
		z := g
		z.Goals = 0
		goalPts = abs(r - residualOf(p, z))
	}
	if g.XA == 0 && g.Assists > 0 {
		z := g
		z.Assists = 0
		assistPts = abs(r - residualOf(p, z))
	}
	return goalPts, assistPts
}

func TestDiagResidualXGCoverage(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)

	fmt.Printf("\n=== does the residual criterion have xG to be a residual OF?\n")
	fmt.Printf("Rows with minutes but XG == 0 do not give the gate a residual: on\n")
	fmt.Printf("them XPointsResidual's attacking term collapses to Goals*goalPoints,\n")
	fmt.Printf("which is 'did he score' — the strongest hindsight in the catalogue.\n")
	fmt.Printf("If the uncovered share of residual MASS is non-trivial in a season,\n")
	fmt.Printf("AxisTransferGateResidual's figure there is not readable as a\n")
	fmt.Printf("conversion result, and only the covered seasons are.\n\n")

	fmt.Printf("%-9s %8s %8s %7s | %8s %8s | %8s %9s | %8s\n",
		"season", "rows", "noXG", "%rows", "noXG&GS", "noXA&AS", "%|resid|", "%degen",
		"degen_pts")
	for _, pair := range pairs {
		s := pair.Cur
		var rows, noXG, scoredNoXG, assistedNoXA int
		var mass, noXGMass, degenMass float64
		for _, p := range s.Players {
			for _, g := range p.GWs {
				if g.Minutes <= 0 {
					continue
				}
				rows++
				mass += abs(residualOf(p, g))
				// The assist channel has the identical shape — (Assists − XA) ×
				// assistPoints — so the same degeneracy is available to it and
				// checking only xG would be half an answer. Counted before the
				// xG branch returns, because a row can have xG and no xA.
				//
				// ⚠️ The two terms are accumulated SEPARATELY, assist first, and
				// that ordering is not cosmetic. Float addition is not
				// associative, so folding them as `degenMass += gp + ap` shifts
				// the last ULP on any row where both channels fire — and there is
				// no such row post-repair, so a before-and-after check at the
				// default data state cannot detect the change it introduces. This
				// keeps the original order, which is what makes the refactor's
				// bit-identity claim true under `FPL_NO_XG_REPAIR=1` as well,
				// where the goal channel carries 868/924/327 rows.
				gp, ap := degenerateAttacking(p, g)
				degenMass += ap
				if g.XA == 0 && g.Assists > 0 {
					assistedNoXA++
				}
				if g.XG != 0 {
					continue
				}
				noXG++
				noXGMass += abs(residualOf(p, g))
				if g.Goals > 0 {
					// The strictly degenerate population: no xG recorded and a
					// goal scored, so the criterion's attacking term is
					// Goals*goalPoints with nothing subtracted — "did he score".
					scoredNoXG++
					degenMass += gp
				}
			}
		}
		pct := func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return 100 * float64(a) / float64(b)
		}
		share := func(a float64) float64 {
			if mass == 0 {
				return 0
			}
			return 100 * a / mass
		}
		// degen_pts is the same quantity as %degen with the denominator taken off.
		// It is printed beside the share because a share cannot be compared with a
		// points figure from anywhere else in this project, and the exposure table
		// below needs the level.
		fmt.Printf("%-9s %8d %8d %6.1f%% | %8d %8d | %7.1f%% %8.2f%% | %8.1f\n",
			s.Name, rows, noXG, pct(noXG, rows),
			scoredNoXG, assistedNoXA, share(noXGMass), share(degenMass), degenMass)
	}

	fmt.Printf("\n⚠️ `noXG` is NOT a coverage defect on its own, and the first reading\n")
	fmt.Printf("of this table got that wrong. About half of every season's appearances\n")
	fmt.Printf("carry no xG because the player took no shot, which is a true zero and\n")
	// A RECORDED reading, and its count stays a literal for a reason the derived
	// version got wrong: the flatness was observed on the six-season default,
	// which is the only grid carrying both backfilled and FPL-fed seasons.
	// Printing len(pairs) here would re-assert "backfilled and FPL-fed alike"
	// over FPL_SWEEP_SEASONS=default, whose four played seasons contain no fully
	// backfilled one — a claim about a contrast the population cannot make.
	fmt.Printf("not a missing one — and on the six-season default grid the share was flat\n")
	fmt.Printf("across all of them, backfilled and FPL-fed alike, which is what rules out\n")
	fmt.Printf("the backfill as the cause. `%%|resid|` inherits that flaw: on those rows the residual\n")
	fmt.Printf("is mostly the assist, clean-sheet and concede channels, all legitimate.\n")
	fmt.Printf("\n`%%degen` is the column that decides: the share of total absolute\n")
	fmt.Printf("residual mass contributed by a return with NO expectation against it\n")
	fmt.Printf("— a goal on a zero-xG row or an assist on a zero-xA row, where the\n")
	fmt.Printf("criterion really is 'did he do it' with nothing subtracted.\n")
	fmt.Printf("`noXG&GS` and `noXA&AS` are how many rows each of those is.\n")
	fmt.Printf("\nBoth channels are checked because XPointsResidual replaces them with\n")
	fmt.Printf("the identical (realised - expected) * points shape, so screening only\n")
	fmt.Printf("the goal channel would answer half the question.\n")
	fmt.Printf("\nThis counts ARCHIVE rows, not the legs the gate judged: the judged\n")
	fmt.Printf("population needs a counter inside a concurrently-executed cell. The\n")
	fmt.Printf("archive population is a superset, so a clean table here is a clean\n")
	fmt.Printf("table for any subset of it, and a dirty one condemns the pooled\n")
	fmt.Printf("figure without saying how much of the gain it explains.\n")
	fmt.Printf("\n⚠️ Cell PRIORS read the previous season, so a covered season entered\n")
	fmt.Printf("from an uncovered one is not thereby a covered cell — but the gate\n")
	fmt.Printf("oracle reads the CURRENT season's rows only, which is what this is.\n")
}

// TestDiagPostRepairAttackingExposure sizes the ungated attacking channel where
// the Understat harvest was supposed to have closed it.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagPostRepairAttackingExposure -v
//
// # The question
//
// `analysis.XPointsResidual` gates its clean-sheet channel on `XGC > 0` and gates
// neither attacking channel. On the natively-fed seasons that exposure is small
// and mostly LEGITIMATE — a won penalty and a deflected assist really do carry ~0
// expectation. What was unmeasured is the POST-REPAIR exposure: the seasons whose
// xG and xA come from the Understat backfill, where a zero can also mean the
// harvest never saw the row.
//
// # The cut
//
// Each season is split at its own repaired window, taken from `xgRepairs` rather
// than restated, so a season is compared with ITSELF wherever it carries both:
// 2022-23 is repaired GW1-15 and native GW16-38, which is the sharpest contrast
// available — same players, same league, same scoring, one half backfilled. The
// fully backfilled seasons have no native half and the FPL-fed ones no repaired
// half, so those rows are a between-season comparison and carry every confound
// that implies.
//
// # ⚠️ `covered` does NOT certify that a zero is genuine, and the first reading of
// this table said it did
//
// `covered` means the row carries xG, which was read as "something priced this row,
// so the zero xA is real". **That silently changes instrument between the arms.** On
// a native row the thing returning zero is FPL's own `expected_assists`; on a
// repaired row it is Understat's xA, which is keyed on a KEY PASS and is
// structurally more zero-prone than FPL's assist, which also pays for a won penalty,
// a deflection and a rebound. So `covered` certifies that the harvest reached the
// row and its own metric returned zero — not that the expectation is genuinely ~0.
//
// That is not a quibble about wording: **pooled, all of the repaired windows'
// elevation sits in `covered`** — the bucket the first reading declared benign and
// therefore never tested. Both columns are printed with their rates for that reason,
// and the per-assist-row denominator is the one to read.
//
// ⚠️ "All of it" is a statement about the POOLED comparison and does not hold on the
// within-2022-23 contrast this test calls its sharpest, where `blank` is elevated by
// roughly the same factor as `covered` and simply has too few events to resolve.
// Quote which comparison you mean.
//
// ⚠️ `cov`, `blank` and `reach` are computed for the ASSIST channel only. The same
// decidability argument applies to goals — a goal with no xG on a row carrying xA
// was reached by the harvest — and it costs nothing post-repair, where the goal
// channel is identically zero. Under `FPL_NO_XG_REPAIR=1` it is not free: 2020-21
// reads 4747 goal points against 2715 assist points, so the split describes the
// smaller half and `blk/100as` is not the whole exposure there.
//
// # ⚠️ What this is not
//
//   - It is a COUNT of archive rows, not a replayed points difference, so no
//     detection threshold applies to the levels. A threshold WOULD apply to any
//     claim that the exposure changes what a replay scores, and this makes no
//     such claim.
//   - It is not a gate proposal, in either direction. A naive `xg+xa > 0` test
//     would refuse the legitimate majority, so the shape a fix would need is a
//     season/gameweek CAPABILITY gate of the `DefconScoredIn` kind — a decision of
//     its own, on the scoring path, and not this diagnostic's to take.
//   - **Mass share is not decision share.** Every exposed row carries a realised
//     return, so it is a high-|residual| row, so it is exactly what a criterion
//     accepting on a positive residual fires on. A small share of absolute residual
//     mass is therefore a LOWER bound on leverage over decisions, not an upper one.
//   - `blank` bounds what the harvest missed **among rows carrying an FPL assist**,
//     which is a much narrower population than "rows". `reach` is what makes the
//     bound readable: `stats/understat_xg_backfill.py` drops rows where xG and xA
//     are both zero, so an `(element, gameweek)` absence is undecidable — but an
//     ELEMENT the crosswalk never mapped appears nowhere in the repair file at all,
//     and that mode is checkable. `reach` counts blank rows whose element does
//     appear, for some gameweek, in the season's repair file.
//
// # Data state
//
// The default is POST-repair. `FPL_NO_XG_REPAIR=1` reports the pre-repair state and
// is a different data state entirely, so the two runs must be labelled and never
// pooled.
//
// It also disables the xGC reconstruction — but that reaches **nothing printed
// here**, and saying otherwise would be a caveat pointing at the wrong risk.
// `degenerateAttacking` differences the residual against the same row with one field
// zeroed, and neither the clean-sheet nor the concede term reads `Goals` or
// `Assists`, so both cancel exactly. Every column below is a function of `Goals`,
// `Assists`, `XG`, `XA` and the position points tables alone.
func TestDiagPostRepairAttackingExposure(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := loadPairsOrSkip(t, cfg)

	state := "POST-repair (default)"
	if noXGRepair() {
		state = "PRE-repair (FPL_NO_XG_REPAIR=1, xGC reconstruction also off)"
	}
	fmt.Printf("\n=== the ungated attacking channel, by repair window\n")
	fmt.Printf("data state: %s\n\n", state)
	fmt.Printf("A row is EXPOSED when XPointsResidual prices a realised return with\n")
	fmt.Printf("no expectation subtracted: a goal on a zero-xG row, or an assist on a\n")
	fmt.Printf("zero-xA row. `covered` and `blank` split the assist exposure by whether\n")
	fmt.Printf("the row carries xG. ⚠️ `covered` is NOT a certificate that the zero is\n")
	fmt.Printf("genuine — the metric returning it is FPL's in the native arm and\n")
	fmt.Printf("Understat's in the repaired one. See the header.\n\n")
	fmt.Printf("`asRows` is rows with an assist, and it is the DENOMINATOR to read:\n")
	fmt.Printf("a rate over all appearances confounds how often a player assists with\n")
	fmt.Printf("how often an assist has no expectation against it. `/100as` columns are\n")
	fmt.Printf("conditional on an assist. `reach` is blank rows whose ELEMENT appears in\n")
	fmt.Printf("the season's repair file for some gameweek — printed for repaired windows\n")
	fmt.Printf("only, since a native window has no harvest to have reached it.\n\n")

	fmt.Printf("%-9s %-16s %7s %7s | %5s %7s | %5s %7s | %5s %8s %9s | %5s %8s %9s | %7s\n",
		"season", "window", "rows", "asRows", "GS", "GS_pts", "AS", "AS_pts",
		"cov", "cov_pts", "cov/100as", "blank", "blk_pts", "blk/100as", "reach")
	for _, pair := range pairs {
		s := pair.Cur
		spec, repaired := xgRepairs[s.Name]
		harvested := harvestedElements(t, s.Name)

		// One accumulator per half. Index 0 is the repaired window, 1 the native
		// remainder; a season with no repair spec puts everything in 1.
		type half struct {
			rows, asRows, gs, as, covered, blank, reach int
			gsPts, coveredPts, blankPts                 float64
		}
		var halves [2]half
		for _, p := range s.Players {
			for gw, g := range p.GWs {
				if g.Minutes <= 0 {
					continue
				}
				i := 1
				if repaired && gw >= spec.FirstGW && gw <= spec.LastGW {
					i = 0
				}
				h := &halves[i]
				h.rows++
				if g.Assists > 0 {
					h.asRows++
				}
				gp, ap := degenerateAttacking(p, g)
				if gp > 0 {
					h.gs++
					h.gsPts += gp
				}
				if ap > 0 {
					h.as++
					// Points rather than rows, because a row can carry two
					// assists and the counts then understate what the criterion
					// actually reads off it. Reading `blank x 3` off the row count
					// is how the first report of this table got 225 where the
					// column says 228.
					if g.XG > 0 {
						h.covered++
						h.coveredPts += ap
					} else {
						h.blank++
						h.blankPts += ap
						if harvested[p.ID] {
							h.reach++
						}
					}
				}
			}
		}

		for i, h := range halves {
			if h.rows == 0 {
				continue
			}
			window := "native"
			if i == 0 {
				// `xgRepairs` is a static table and does not vary with the
				// switch, so under FPL_NO_XG_REPAIR these rows are the window
				// that WOULD have been repaired and was not. Labelling them
				// "repaired" there would make every row assert the opposite of
				// the run — recoverable from the data-state line above it, but
				// this is the state where an operator most needs the label,
				// since the window split is the only thing separating the arms.
				verb := "repaired"
				if noXGRepair() {
					verb = "UNrepaired"
				}
				window = fmt.Sprintf("%s GW%d-%d", verb, spec.FirstGW, spec.LastGW)
			}
			per100 := func(n int) float64 {
				if h.asRows == 0 {
					return 0
				}
				return 100 * float64(n) / float64(h.asRows)
			}
			reach := fmt.Sprintf("%d/%d", h.reach, h.blank)
			if i != 0 {
				reach = "-" // no harvest ran here; see the header
			}
			fmt.Printf("%-9s %-16s %7d %7d | %5d %7.1f | %5d %7.1f | %5d %8.1f %9.2f | %5d %8.1f %9.2f | %7s\n",
				s.Name, window, h.rows, h.asRows, h.gs, h.gsPts,
				h.as, h.coveredPts+h.blankPts,
				h.covered, h.coveredPts, per100(h.covered),
				h.blank, h.blankPts, per100(h.blank), reach)
		}
	}

	fmt.Printf("\nRead `cov/100as` and `blk/100as`, not the raw counts: the windows are\n")
	fmt.Printf("different lengths and contain different numbers of assists.\n")
	fmt.Printf("\n2022-23 is the only season carrying both halves, so it is the only row\n")
	fmt.Printf("pair that holds the football constant, and it is therefore the contrast\n")
	fmt.Printf("this table is FOR. A verdict taken from the pooled between-season\n")
	fmt.Printf("comparison instead is taken from the blunter instrument — that mixes\n")
	fmt.Printf("fully-repaired seasons against fully-native ones and carries every era\n")
	fmt.Printf("confound with it. The first reading of this table did exactly that.\n")
	fmt.Printf("\n⚠️ `cov` and `blank` are one family, not two independent tests. Six\n")
	fmt.Printf("contrasts are available off this table; correct for the family before\n")
	fmt.Printf("quoting any single p, and do not read the one that fails to resolve as\n")
	fmt.Printf("the answer while the others resolve.\n")
	fmt.Printf("\nNothing in this test proposes a gate, in either direction. See the\n")
	fmt.Printf("header for why a per-row `xg+xa > 0` test would be the wrong shape, and\n")
	fmt.Printf("for why a share of residual mass is a lower bound on leverage over\n")
	fmt.Printf("decisions rather than an upper one.\n")
}

// harvestedElements is the set of element ids the Understat crosswalk reached for
// a season, taken from the embedded repair file: an element appearing on any row,
// for any gameweek.
//
// It exists to make `blank` readable. `stats/understat_xg_backfill.py` drops rows
// where xG and xA are both zero before writing, so a missing `(element, gameweek)`
// cannot be told apart from one the harvest priced at 0/0 — that mode is
// undecidable from here. A missing ELEMENT is not: a player the crosswalk never
// mapped appears on no row at all. So this separates "the harvest never saw this
// player" from "the harvest saw him and its own metric returned zero", which is the
// half of the caveat that can be retired.
//
// Read directly rather than through applyXGRepair, which returns aggregate counts
// and, under `FPL_NO_XG_REPAIR=1`, does not run. Crosswalk reach is a fact about the
// harvest and does not vary with that switch.
func harvestedElements(t *testing.T, season string) map[int]bool {
	t.Helper()
	out := map[int]bool{}
	f, err := repairData.Open("repairdata/" + season + "-xg.csv")
	if err != nil {
		return out // no repair file: a native season, nothing to have reached
	}
	defer f.Close()
	r := csv.NewReader(f)
	head, err := r.Read()
	if err != nil {
		t.Fatalf("harvest reach %s: reading header: %v", season, err)
	}
	col := -1
	for i, h := range head {
		if strings.TrimSpace(h) == "element" {
			col = i
		}
	}
	if col < 0 {
		t.Fatalf("harvest reach %s: no \"element\" column — the repair file's shape "+
			"has moved and this reach count would silently read as zero", season)
	}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("harvest reach %s: %v", season, err)
		}
		el, err := strconv.Atoi(strings.TrimSpace(rec[col]))
		if err != nil {
			t.Fatalf("harvest reach %s: non-numeric element %q", season, rec[col])
		}
		out[el] = true
	}
	return out
}
