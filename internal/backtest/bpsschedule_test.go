package backtest

// The 2015-16 BPS schedule, decoded exactly from the archive, and what removing
// the `tackled` penalty does to the bonus ranking.
//
//	DIAG=1 go test ./internal/backtest -run 'TestDiagBPSSchedule|TestDiagBPSTackled' -v -timeout 10m
//
// # Why this exists
//
// TestDiagBPSRuleChange prices four of FPL's five 2026/27 bonus changes on
// 2025-26. It cannot price the fifth — the removal of the −1 BPS penalty for
// being **tackled** — because 2025-26's archive carries no such column, and it
// says so: "this diagnostic understates the gain to the players the removed
// penalty was hurting... the MID and FWD columns are therefore lower bounds, and
// they are the two positions that change was aimed at."
//
// The column exists. Not in 2025-26, but in **2016-17, 2017-18 and 2018-19**,
// where `tackled` carries real per-gameweek counts and vanishes from 2019-20
// onward. That is the sixth instance of this record's standing rule — a claim of
// the form "the archive does not have X" is unverified until someone greps for X,
// and it is **season-scoped**: the season you happened to check is not the archive.
// (The seventh followed immediately: "the archive carries realised bps totals and
// not the coefficient schedule", refuted by the decode below.)
//
// # Two diagnostics, and the first is worth more than the second
//
// TestDiagBPSSchedule **decodes the entire 2015-16 BPS schedule** from the
// archive's own numbers. Every coefficient is solved for by least squares over
// the component counts and then required to reproduce the recorded `bps`
// **exactly, on every played row**. That turns a class of question this record
// currently answers by reading FPL announcements into one it answers by
// measurement.
//
// TestDiagBPSTackled uses the decoded coefficient to price the tackled channel.
//
// # Why the acceptance gate is "identically zero" and not "a good fit"
//
// This is the trap, and it was hit during review before it was written down.
// Coding one **sibling** feature wrong — `saves` as floor(saves/3) rather than raw
// at 2 BPS — returns `tackled = −1.045` with residual sd 0.568 and 88% of rows
// exact. That looks like a healthy fit, and it rounds to the right answer *here*,
// but it is a 4.5% error leaked in from an unrelated mis-specification and nothing
// guarantees the next leak lands under 0.5. A high R², a small residual and
// "rounds to an integer" are all inadequate. The only sound gate is that the
// reconstruction reproduces every recorded `bps` with **max deviation 0**, and a
// singular pivot is a hard failure rather than a silently-zero coefficient.
//
// # What this can and cannot be used for
//
// The two channels **do not add**. Bonus is a rank over a fixed ~6-point pool per
// match, so the CBI change and the tackled change move the same players past each
// other in the same rankings; measuring them separately and summing is wrong. The
// *full five-change* 2026/27 figure is unmeasurable on any archived season, because
// the saves restructure is defined against a 2025-26 baseline no season carries
// alongside a `tackled` column.
//
// But the joint **CBI-plus-tackled** arm is measurable here and is **unrun**: these
// three seasons carry both columns, the schedule is decoded, and at big-chance rate
// 0.00 the saves leg contributes nothing. It is the arm that would sign the keeper
// result, and the non-additivity is the reason to run it rather than a reason it
// cannot be run.
//
// Transfer to modern football is qualified twice. The 3rd/4th BPS gap is measured
// here on both eras because that is the only channel a fixed BPS perturbation acts
// through. And the modern *tackled rate* cannot be checked at all, since the column
// stops in 2019-20 — that is the irreducible assumption, and it is the larger of
// the two.
//
// Nothing here ships. See the scoring-model note for why a bonus
// correction is declined on three grounds this measurement leaves untouched.

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"armband/internal/stats"
)

// tackledSeasons are the seasons carrying a per-gameweek `tackled` count. Checked
// against the headers directly rather than assumed: the column is present in these
// three and absent from 2019-20 onward.
var tackledSeasons = []string{"2016-17", "2017-18", "2018-19"}

// tackledBPS is what the schedule pays for being tackled. It is **asserted against
// the decode** by TestDiagBPSSchedule rather than believed, and TestDiagBPSTackled
// reads it rather than folding a −1 into a `+`.
//
// Hardcoding it in the arm would be this record's signature failure in miniature:
// one quantity with two implementations, where a re-decode returning anything else
// would move the coefficient table and leave every figure in the arm unchanged.
const tackledBPS = -1

// bpsComponents is one player-fixture's raw BPS inputs, kept beside the bpsRow
// that the award machinery already consumes. Splitting it this way lets
// groupByFixture and award be reused unchanged — award is the load-bearing rule
// and must not acquire a second implementation.
type bpsComponents struct {
	Minutes, Goals, Assists, CleanSheet        int
	Saves, PenSaved, PenMissed, PenConceded    int
	OwnGoals, Yellow, Red                      int
	ErrGoal, ErrAttempt                        int
	BigChanceCreated, BigChanceMissed, KeyPass int
	Crosses, Tackles, Tackled, CBI, Recoveries int
	Dribbles, Fouls, Offside, TargetMissed     int
	WinningGoals, AttemptedPass, CompletedPass int
	Pos                                        string
}

// bpsFeature is one term of the schedule: a name and how to read it off a row.
//
// The floor divisors are part of the *specification*, not of the fit. A schedule
// that pays 1 BPS per two clearances is not linear in clearances, and fitting it
// as though it were is precisely the mis-specification the header warns about.
type bpsFeature struct {
	name string
	of   func(bpsComponents) float64
}

// bpsSchedule is the specification being decoded. Order is fixed so the solved
// coefficients can be read against it.
//
// `goal_gk` is deliberately a separate term rather than folded into `goal_def`,
// which would be an assumption rather than a measurement. In the event **no
// goalkeeper scores in any of the three seasons**, so the term is identically zero
// and solveBPS drops it with a logged note — dropping an all-zero column leaves the
// reconstruction exact, whereas a *nonzero* unidentified column is a hard failure.
func bpsSchedule() []bpsFeature {
	pos := func(want string) func(bpsComponents) float64 {
		return func(c bpsComponents) float64 {
			if c.Pos == want {
				return float64(c.Goals)
			}
			return 0
		}
	}
	// passBand returns 1 when the row's completed-pass ratio falls in [lo, hi).
	// Gated at 30 attempted passes, below which FPL awards nothing.
	passBand := func(lo, hi float64) func(bpsComponents) float64 {
		return func(c bpsComponents) float64 {
			if c.AttemptedPass < 30 {
				return 0
			}
			r := float64(c.CompletedPass) / float64(c.AttemptedPass)
			if r >= lo && r < hi {
				return 1
			}
			return 0
		}
	}
	n := func(f func(bpsComponents) int) func(bpsComponents) float64 {
		return func(c bpsComponents) float64 { return float64(f(c)) }
	}
	return []bpsFeature{
		{"min_1_59", func(c bpsComponents) float64 {
			if c.Minutes > 0 && c.Minutes < 60 {
				return 1
			}
			return 0
		}},
		{"min_60plus", func(c bpsComponents) float64 {
			if c.Minutes >= 60 {
				return 1
			}
			return 0
		}},
		{"goal_gk", pos("GK")},
		{"goal_def", pos("DEF")},
		{"goal_mid", pos("MID")},
		{"goal_fwd", pos("FWD")},
		{"assist", n(func(c bpsComponents) int { return c.Assists })},
		{"clean_sheet_gk_def", func(c bpsComponents) float64 {
			if c.Minutes >= 60 && (c.Pos == "GK" || c.Pos == "DEF") {
				return float64(c.CleanSheet)
			}
			return 0
		}},
		// Raw, at 2 BPS a save. NOT floor(saves/3) — see the header; that single
		// mis-specification is what leaked 4.5% into the tackled coefficient.
		{"saves", n(func(c bpsComponents) int { return c.Saves })},
		{"pen_saved", n(func(c bpsComponents) int { return c.PenSaved })},
		{"pen_missed", n(func(c bpsComponents) int { return c.PenMissed })},
		{"pen_conceded", n(func(c bpsComponents) int { return c.PenConceded })},
		{"own_goal", n(func(c bpsComponents) int { return c.OwnGoals })},
		{"yellow", n(func(c bpsComponents) int { return c.Yellow })},
		{"red", n(func(c bpsComponents) int { return c.Red })},
		{"error_goal", n(func(c bpsComponents) int { return c.ErrGoal })},
		{"error_attempt", n(func(c bpsComponents) int { return c.ErrAttempt })},
		{"big_chance_created", n(func(c bpsComponents) int { return c.BigChanceCreated })},
		{"big_chance_missed", n(func(c bpsComponents) int { return c.BigChanceMissed })},
		{"key_pass", n(func(c bpsComponents) int { return c.KeyPass })},
		{"open_play_cross", n(func(c bpsComponents) int { return c.Crosses })},
		{"tackles", n(func(c bpsComponents) int { return c.Tackles })},
		{"cbi_per_2", n(func(c bpsComponents) int { return c.CBI / 2 })},
		{"recoveries_per_3", n(func(c bpsComponents) int { return c.Recoveries / 3 })},
		{"dribbles", n(func(c bpsComponents) int { return c.Dribbles })},
		{"fouls", n(func(c bpsComponents) int { return c.Fouls })},
		{"offside", n(func(c bpsComponents) int { return c.Offside })},
		{"target_missed", n(func(c bpsComponents) int { return c.TargetMissed })},
		{"tackled", n(func(c bpsComponents) int { return c.Tackled })},
		{"winning_goal", n(func(c bpsComponents) int { return c.WinningGoals })},
		{"pass_70_80", passBand(0.70, 0.80)},
		{"pass_80_90", passBand(0.80, 0.90)},
		{"pass_90_plus", passBand(0.90, math.Inf(1))},
	}
}

// TestDiagBPSSchedule recovers every 2015-16 BPS coefficient from the archive and
// requires the reconstruction to be exact on every played row.
func TestDiagBPSSchedule(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	feats := bpsSchedule()
	all := map[string][]oldBPSSeason{}
	var pooledRows []bpsRow
	var pooledComp []bpsComponents

	for _, season := range tackledSeasons {
		rows, comps, dupes := loadTackledSeason(t, season)
		all[season] = []oldBPSSeason{{season, rows, comps}}
		t.Logf("%s: %d player-fixtures, %d matches, %d duplicate (element,fixture) rows",
			season, len(rows), countFixtures(rows), dupes)
		for i, r := range rows {
			if comps[i].Minutes > 0 {
				pooledRows = append(pooledRows, r)
				pooledComp = append(pooledComp, comps[i])
			}
		}
	}
	_ = all

	t.Logf("")
	t.Logf("Decoding the schedule on %d played player-fixtures pooled over %s.",
		len(pooledComp), strings.Join(tackledSeasons, ", "))

	coef := solveBPS(t, feats, pooledComp, pooledRows)

	// Every coefficient must land on an integer. A fractional one means the
	// specification is wrong, not that FPL pays fractions.
	t.Logf("")
	t.Logf("%-22s %10s %8s", "term", "solved", "integer")
	ints := make([]int, len(feats))
	for i, f := range feats {
		r := math.Round(coef[i])
		if math.Abs(coef[i]-r) > 1e-6 {
			t.Errorf("coefficient for %q solved to %.6f, which is not an integer. "+
				"The specification is wrong — a fractional coefficient means a term "+
				"is missing or mis-encoded, not that FPL pays fractions.", f.name, coef[i])
		}
		ints[i] = int(r)
		t.Logf("%-22s %10.6f %8d", f.name, coef[i], ints[i])
	}

	// The gate. Reproduce every recorded bps exactly, per season, or fail.
	t.Logf("")
	t.Logf("Reconstruction against recorded bps, per season:")
	total, exact, worst := 0, 0, 0
	for _, season := range tackledSeasons {
		s := all[season][0]
		n, ok, dev := checkSchedule(feats, ints, s.rows, s.comps)
		total, exact = total+n, exact+ok
		if dev > worst {
			worst = dev
		}
		t.Logf("  %-8s %6d played rows, %6d exact, max deviation %d", season, n, ok, dev)
		sink.emitAll("bps_schedule_decode", season, "reconstruction", n,
			measure{"exact_rows", float64(ok)},
			measure{"max_deviation", float64(dev)},
		)
	}
	if exact != total || worst != 0 {
		t.Fatalf("the reconstruction is not exact: %d of %d rows, max deviation %d. "+
			"A near-exact fit is not evidence — a mis-specified sibling feature leaks "+
			"into every other coefficient, so nothing solved above can be quoted.",
			exact, total, worst)
	}
	t.Logf("")
	t.Logf("EXACT: %d of %d played rows, max deviation 0. The schedule is decoded, "+
		"not estimated.", exact, total)

	// The two coefficients this settles that the record currently reads off
	// announcements rather than measures.
	for i, f := range feats {
		if f.name == "tackled" || f.name == "pen_saved" {
			t.Logf("  %s = %d", f.name, ints[i])
		}
		// Pin the constant the tackled arm consumes to the decode, so the two
		// cannot drift apart silently.
		if f.name == "tackled" && ints[i] != tackledBPS {
			t.Fatalf("the decode says tackled = %d and TestDiagBPSTackled applies "+
				"tackledBPS = %d. Update the constant: the arm would otherwise report "+
				"a plausible wrong magnitude while this table showed the truth.",
				ints[i], tackledBPS)
		}
	}
}

type oldBPSSeason struct {
	name  string
	rows  []bpsRow
	comps []bpsComponents
}

// solveBPS solves the normal equations for the schedule's coefficients.
//
// This is a decoding rather than an inference: the system is exact, so there is no
// estimation error to report and every standard error is zero. A singular pivot
// means a term is unidentified on this population — which really happens, since no
// goalkeeper scores in some seasons — and is a hard failure rather than a silently
// returned zero.
func solveBPS(t *testing.T, feats []bpsFeature, comps []bpsComponents, rows []bpsRow) []float64 {
	t.Helper()
	k := len(feats)
	// A feature that is identically zero on this population is a different animal
	// from a collinear one, and only the second is dangerous. An all-zero column
	// contributes nothing to any row, so dropping it leaves the reconstruction
	// exact and merely records that the term is unmeasurable here. A *nonzero*
	// singular column would mean two terms cannot be told apart, which would
	// silently fold one into the other — that stays a hard failure below.
	//
	// This really fires: no goalkeeper scored in 2016-17, 2017-18 or 2018-19, so
	// `goal_gk` cannot be measured on THIS POPULATION.
	// ⚠️ Not on the archive at large — 2020-21 GW36 holds one, and it is what
	// decodes the pre-modern keeper goal value in `analysis.ScoringRulesFor`. That
	// season carries no `tackled` column and no full component set, so it cannot
	// join this fit; the two statements do not conflict.
	var keep []int
	for i, f := range feats {
		nonzero := false
		for _, c := range comps {
			if f.of(c) != 0 {
				nonzero = true
				break
			}
		}
		if nonzero {
			keep = append(keep, i)
			continue
		}
		t.Logf("term %q is identically zero on all %d rows — unmeasurable on this "+
			"population, and dropped. It cannot affect the reconstruction.",
			f.name, len(comps))
	}

	m := len(keep)
	xtx := make([][]float64, m)
	for i := range xtx {
		xtx[i] = make([]float64, m+1)
	}
	x := make([]float64, m)
	for n, c := range comps {
		for i, fi := range keep {
			x[i] = feats[fi].of(c)
		}
		y := float64(rows[n].BPS)
		for i := 0; i < m; i++ {
			if x[i] == 0 {
				continue
			}
			for j := 0; j < m; j++ {
				xtx[i][j] += x[i] * x[j]
			}
			xtx[i][m] += x[i] * y
		}
	}
	// Gaussian elimination with partial pivoting. A singular pivot here is a
	// *nonzero* column that cannot be told apart from another one, which is the
	// dangerous case: solving anyway would fold two terms into one and every
	// coefficient downstream would be quietly wrong.
	// Note the row swaps reorder *equations*, not variables: after elimination the
	// pivot for column j sits in row j, so variable j is always kept feature
	// keep[j]. Permuting the variable indices by the row swaps would be a bug.
	for col := 0; col < m; col++ {
		best, bv := col, math.Abs(xtx[col][col])
		for r := col + 1; r < m; r++ {
			if v := math.Abs(xtx[r][col]); v > bv {
				best, bv = r, v
			}
		}
		if bv < 1e-9 {
			t.Fatalf("term %q is collinear with another on this population (singular "+
				"pivot on a column that is not identically zero). Solving anyway would "+
				"fold two terms into one and every coefficient would be wrong.",
				feats[keep[col]].name)
		}
		xtx[col], xtx[best] = xtx[best], xtx[col]
		for r := 0; r < m; r++ {
			if r == col {
				continue
			}
			f := xtx[r][col] / xtx[col][col]
			if f == 0 {
				continue
			}
			for c := col; c <= m; c++ {
				xtx[r][c] -= f * xtx[col][c]
			}
		}
	}
	out := make([]float64, k)
	for i := 0; i < m; i++ {
		out[keep[i]] = xtx[i][m] / xtx[i][i]
	}
	return out
}

// checkSchedule rebuilds bps from the integer schedule and compares to the record.
func checkSchedule(feats []bpsFeature, coef []int, rows []bpsRow, comps []bpsComponents) (n, exact, worst int) {
	for i, c := range comps {
		if c.Minutes == 0 {
			continue
		}
		n++
		sum := 0
		for j, f := range feats {
			sum += coef[j] * int(f.of(c))
		}
		if d := sum - rows[i].BPS; d == 0 {
			exact++
		} else if a := int(abs(float64(d))); a > worst {
			worst = a
		}
	}
	return n, exact, worst
}

// TestDiagBPSTackled prices the removal of the tackled penalty.
func TestDiagBPSTackled(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	sink := openModelSinkFor(t.Logf)
	defer sink.close()

	type seasonResult struct {
		name  string
		byPos map[string]bpsTotals
	}
	var results []seasonResult
	// Keyed by PLAYER-SEASON, not by player. FPL reassigns element ids every
	// summer — this repository has already shipped one bug from keying on them
	// across seasons — so pooling three seasons by element would merge different
	// footballers under one id. A player-season is also the right unit here: a
	// tackled rate belongs to the season it was earned in.
	pooled := map[int]bpsTotals{}
	const seasonStride = 1_000_000

	for si, season := range tackledSeasons {
		rows, comps, dupes := loadTackledSeason(t, season)
		grid := fmt.Sprintf("%s, %d matches, %d player-fixtures",
			season, countFixtures(rows), len(rows))
		t.Logf("")
		t.Logf("population: %s (%d duplicate rows discarded)", grid, dupes)

		// Gate one, exactly as TestDiagBPSRuleChange does it: reproduce the recorded
		// awards from the recorded BPS. If the tie rule differs by era, nothing below
		// this line means anything.
		var checked, wrong int
		for _, g := range groupByFixture(rows) {
			bps := make([]int, len(g))
			for i, r := range g {
				bps[i] = r.BPS
			}
			for i, got := range award(bps) {
				checked++
				if got != g[i].Bonus {
					wrong++
					if wrong <= 5 {
						t.Logf("  tie rule disagrees: fixture %d %s bps %d recorded %d computed %d",
							g[i].Fixture, g[i].Name, g[i].BPS, g[i].Bonus, got)
					}
				}
			}
		}
		if wrong > 0 {
			t.Fatalf("%s: award() reproduced %d of %d recorded bonus values. The tie "+
				"rule or the fixture grouping differs in this era, so every "+
				"recomputation below is wrong.", season, checked-wrong, wrong)
		}
		t.Logf("tie rule validated: %d of %d recorded awards reproduced exactly", checked, checked)

		// Removing a −1 line item adds back one BPS per occurrence. This is the same
		// subtractive design as the CBI arm: every line item FPL left alone cancels,
		// so none of them has to be modelled.
		byPos := map[string]bpsTotals{}
		idx := map[[2]int]int{}
		for i, r := range rows {
			idx[[2]int{r.Element, r.Fixture}] = i
		}
		for _, g := range groupByFixture(rows) {
			oldB := make([]int, len(g))
			newB := make([]int, len(g))
			for i, r := range g {
				c := comps[idx[[2]int{r.Element, r.Fixture}]]
				oldB[i] = r.BPS
				// Removing a line item subtracts its contribution. tackledBPS is
				// negative, so this adds one BPS back per tackle.
				newB[i] = r.BPS - tackledBPS*c.Tackled
			}
			oa, na := award(oldB), award(newB)
			for i, r := range g {
				c := comps[idx[[2]int{r.Element, r.Fixture}]]
				one := bpsTotals{
					element: r.Element, name: r.Name, pos: r.Pos,
					old: oa[i], new: na[i], minutes: r.Minutes,
					tackled: c.Tackled,
				}
				p := byPos[r.Pos]
				p.pos = r.Pos
				p.add(one)
				byPos[r.Pos] = p

				k := si*seasonStride + r.Element
				q := pooled[k]
				q.element, q.pos = k, r.Pos
				q.name = fmt.Sprintf("%s [%s]", r.Name, season)
				q.add(one)
				pooled[k] = q
			}
		}
		results = append(results, seasonResult{season, byPos})
		for _, pos := range []string{"GK", "DEF", "MID", "FWD"} {
			a := byPos[pos]
			sink.emitAll("bps_tackled", grid, pos, a.minutes,
				measure{"old_per90", a.per90old()},
				measure{"new_per90", a.per90new()},
				measure{"shift_pct", a.shiftPct()},
			)
		}
	}

	t.Logf("")
	t.Logf("Realised bonus per 90, recorded rules against the tackled penalty removed.")
	t.Logf("A keeper is tackled ~0.00 times per 90, so he gains nothing and is")
	t.Logf("displaced downward by everyone who does. That is the finding.")
	t.Logf("%-6s %-9s %8s %8s %9s %9s %8s", "pos", "season", "old", "new", "old/90", "new/90", "shift")
	for _, pos := range []string{"GK", "DEF", "MID", "FWD"} {
		for _, r := range results {
			a := r.byPos[pos]
			t.Logf("%-6s %-9s %8d %8d %9.4f %9.4f %+7.1f%%",
				pos, r.name, a.old, a.new, a.per90old(), a.per90new(), a.shiftPct())
		}
	}

	// The within-position split, cut on the statistic the rule acts on. This is the
	// part an argmax can see: a shift shared by a whole position is invisible to a
	// search that consumes an ordering, and this one is not shared.
	t.Logf("")
	t.Logf("Terciles by tackled per 90, 900+ minute player-seasons, %s pooled:",
		seasonsLabel(len(tackledSeasons)))
	key := bpsTotals.tackled90
	for _, pos := range []string{"GK", "DEF", "MID", "FWD"} {
		buckets := terciles(pooled, pos, 900, key)
		if len(buckets) == 0 {
			continue
		}
		// A cut whose boundaries fall inside a tied block is not a cut. Keepers are
		// tackled 0.00 times per 90, so every GK tercile boundary sits inside the
		// tie and the split is decided entirely by the sort's element tie-break —
		// which, since the key is season-major, silently orders them by SEASON. The
		// block would read as a within-position gradient and be a season trend, and
		// the monotonicity guard below would then be asserting something about
		// season ordering rather than about tackled/90.
		if lo, hi, ok := tercileBoundariesTie(pooled, pos, 900, key); ok {
			t.Logf("  %s: SKIPPED — the tercile boundaries are tied (%.2f and %.2f "+
				"tackled/90), so the cut carries no information about the statistic "+
				"and would order by the tie-break instead.", pos, lo, hi)
			continue
		}
		t.Logf("  %s:", pos)
		var shifts []float64
		for _, b := range buckets {
			t.Logf("    %-8s n=%3d  tackled/90 %5.2f  bonus/90 %.4f -> %.4f  %+.1f%%",
				b.label, b.n, key(b.tot), b.tot.per90old(), b.tot.per90new(), b.tot.shiftPct())
			if b.label != "middle" {
				t.Logf("         e.g. %s", b.edge)
			}
			shifts = append(shifts, b.tot.shiftPct())
			sink.emitAll("bps_tackled", seasonsLabel(len(tackledSeasons))+" pooled",
				fmt.Sprintf("%s, %s tackled third", pos, b.label), b.n,
				measure{"old_per90", b.tot.per90old()},
				measure{"new_per90", b.tot.per90new()},
				measure{"shift_pct", b.tot.shiftPct()},
			)
		}
		// The guard. Removing a penalty can only ever move bonus toward the players
		// who were paying it, so the shift must rise with tackled/90 inside every
		// position. This is the check that a whole-position direction cannot give:
		// keepers *must* lose, and so must a low-tackled defender, so "nobody loses"
		// would be the bug, not the finding.
		for i := 1; i < len(shifts); i++ {
			if shifts[i] < shifts[i-1] {
				t.Errorf("%s: shift is not monotone in tackled/90 (%+.1f%% then %+.1f%%). "+
					"Removing a −1 line item can only move bonus toward the players who "+
					"were paying it, so this is a bug rather than a result.",
					pos, shifts[i-1], shifts[i])
			}
		}
	}

	// Transferability. A fixed BPS perturbation only bites through the 3rd/4th
	// boundary, so how tight that boundary is decides how much of the effect
	// survives into modern football. Measured on both eras rather than asserted.
	t.Logf("")
	t.Logf("The 3rd/4th BPS gap — the only channel a fixed perturbation acts through:")
	t.Logf("%-9s %10s %10s %12s", "season", "mean gap", "median", "% within 1")
	for _, season := range append(append([]string{}, tackledSeasons...), bpsRuleSeason) {
		rows, _, _ := loadBPSRowsFor(t, season)
		if rows == nil {
			// Never drop a row silently. The 2025-26 line is the whole of the
			// measured transferability qualification, and a timeout that removed it
			// would leave three old seasons reading as a complete comparison.
			t.Fatalf("could not load %s for the 3rd/4th gap comparison. This row is "+
				"the entire measured basis for 'the modern boundary is looser', so "+
				"continuing would print a table that looks complete and is not.", season)
		}
		mean, med, within := thirdFourthGap(rows)
		t.Logf("%-9s %10.2f %10.0f %9.1f%%", season, mean, med, within)
		sink.emitAll("bps_tackled", season, "third-fourth gap", countFixtures(rows),
			measure{"mean_gap", mean},
			measure{"median_gap", med},
			measure{"pct_within_1", within},
		)
	}
	t.Logf("")
	t.Logf("A looser modern boundary means fewer awards flip, so these figures are")
	t.Logf("mildly INFLATED for 2026/27 football. The modern tackled RATE cannot be")
	t.Logf("checked at all — the column stops in 2019-20 — and that is the larger")
	t.Logf("assumption of the two.")
}

// tercileBoundariesTie reports whether either tercile boundary falls inside a
// block of equal keys, which makes the split an artefact of the tie-break rather
// than a cut on the statistic.
func tercileBoundariesTie(byPlayer map[int]bpsTotals, pos string, minMinutes int, key func(bpsTotals) float64) (lo, hi float64, tied bool) {
	var ks []float64
	for _, v := range byPlayer {
		if v.pos == pos && v.minutes >= minMinutes {
			ks = append(ks, key(v))
		}
	}
	n := len(ks)
	if n < 3 {
		return 0, 0, false
	}
	sort.Float64s(ks)
	k := n / 3
	return ks[k], ks[n-k], ks[k-1] == ks[k] || ks[n-k-1] == ks[n-k]
}

// thirdFourthGap reports how tight the bonus boundary is: the BPS distance between
// the third and fourth highest scorer in a match.
func thirdFourthGap(rows []bpsRow) (mean, median, withinOne float64) {
	var gaps []float64
	for _, g := range groupByFixture(rows) {
		if len(g) < 4 {
			continue
		}
		bps := make([]int, 0, len(g))
		for _, r := range g {
			bps = append(bps, r.BPS)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(bps)))
		gaps = append(gaps, float64(bps[2]-bps[3]))
	}
	if len(gaps) == 0 {
		return 0, 0, 0
	}
	var sum, n1 float64
	for _, g := range gaps {
		sum += g
		if g <= 1 {
			n1++
		}
	}
	// stats.Median rather than the package's `median` wrapper: this function's
	// named return shadows it. The record quotes this function's *mean* and its
	// within-1 percentage, not this column, so the estimator change moves nothing
	// that is written down.
	return sum / float64(len(gaps)), stats.Median(gaps), n1 / float64(len(gaps)) * 100
}

// loadTackledSeason reads one of the old seasons: the award machinery's bpsRow,
// the raw components beside it, and the duplicate count.
//
// Position is joined from players_raw.csv, because these seasons' merged_gw.csv
// has no `position` column — the 2025-26 loader reads one and this one cannot.
func loadTackledSeason(t *testing.T, season string) ([]bpsRow, []bpsComponents, int) {
	t.Helper()
	pos := loadElementTypes(t, season)
	rows, comps, dupes := loadBPSRowsFor(t, season)
	if rows == nil {
		t.Skipf("archive unreachable for %s", season)
	}
	var missing int
	for i := range rows {
		p, ok := pos[rows[i].Element]
		if !ok {
			missing++
			continue
		}
		rows[i].Pos = p
		comps[i].Pos = p
	}
	if missing > 0 {
		t.Fatalf("%s: %d player-fixtures have no element_type in players_raw.csv. "+
			"An unmatched row would be silently pooled under the empty position and "+
			"read as a whole extra group.", season, missing)
	}
	return rows, comps, dupes
}

var elementTypeName = map[int]string{1: "GK", 2: "DEF", 3: "MID", 4: "FWD"}

func loadElementTypes(t *testing.T, season string) map[int]string {
	t.Helper()
	r, c, col, err := rows(context.Background(), season, "players_raw.csv")
	if err != nil {
		t.Skipf("archive unreachable: %v", err)
	}
	defer c.Close()
	for _, need := range []string{"id", "element_type"} {
		if _, ok := col[need]; !ok {
			t.Fatalf("players_raw.csv for %s has no %q column", season, need)
		}
	}
	out := map[int]string{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("players_raw.csv: %v", err)
		}
		name, ok := elementTypeName[ival(rec, col, "element_type")]
		if !ok {
			continue
		}
		out[ival(rec, col, "id")] = name
	}
	return out
}

// loadBPSRowsFor reads one season's merged_gw.csv into the award machinery's row
// type plus the raw components. Returns nil rows if the archive is unreachable.
//
// Every column is checked for presence rather than read with ival and hoped for.
// ival returns 0 for an absent column, so a missing `tackled` would yield a delta
// of exactly zero on every row and print a clean, complete-looking table showing
// the rule change does nothing. That is this package's signature failure.
func loadBPSRowsFor(t *testing.T, season string) ([]bpsRow, []bpsComponents, int) {
	t.Helper()
	r, c, col, err := rows(context.Background(), season, "gws/merged_gw.csv")
	if err != nil {
		return nil, nil, 0
	}
	defer c.Close()

	// The 2025-26 file is read here only for the 3rd/4th gap, which needs bps and
	// fixture alone; the old seasons need the whole component set.
	need := []string{"element", "fixture", "bps", "bonus", "minutes", "name"}
	// Whether the component set is required is decided by WHICH SEASON this is,
	// never by whether the columns happen to be present. Gating on presence would
	// make the one column this whole diagnostic is about the only one not
	// hard-checked: if the archive renamed `tackled`, every position would read
	// +0.0%, the terciles would print 0.00 with named edge players, the
	// monotonicity guard would pass vacuously on 0 < 0, and the test would report
	// PASS. That is a complete-looking table asserting the rule change does
	// nothing, which is exactly the failure the file header claims to guard.
	old := false
	for _, s := range tackledSeasons {
		if s == season {
			old = true
			break
		}
	}
	if old {
		need = append(need,
			"goals_scored", "assists", "clean_sheets", "saves", "penalties_saved",
			"penalties_missed", "penalties_conceded", "own_goals", "yellow_cards",
			"red_cards", "errors_leading_to_goal", "errors_leading_to_goal_attempt",
			"big_chances_created", "big_chances_missed", "key_passes",
			"open_play_crosses", "tackles", "tackled",
			"clearances_blocks_interceptions", "recoveries", "dribbles", "fouls",
			"offside", "target_missed", "winning_goals", "attempted_passes",
			"completed_passes")
	}
	for _, n := range need {
		if _, ok := col[n]; !ok {
			t.Fatalf("merged_gw.csv for %s has no %q column. Without it the "+
				"recomputation would silently read zero and report that the rule "+
				"change does nothing.", season, n)
		}
	}

	var outRows []bpsRow
	var outComp []bpsComponents
	seen := map[[2]int]bool{}
	dupes := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s merged_gw.csv: %v", season, err)
		}
		row := bpsRow{
			Element: ival(rec, col, "element"),
			Fixture: ival(rec, col, "fixture"),
			// These files are latin-1; the name is display-only, so coerce it to
			// valid UTF-8 rather than letting a mangled byte reach a log line.
			Name:    strings.ToValidUTF8(sval(rec, col, "name"), "?"),
			Minutes: ival(rec, col, "minutes"),
			BPS:     ival(rec, col, "bps"),
			Bonus:   ival(rec, col, "bonus"),
		}
		if row.Element == 0 || row.Fixture == 0 {
			continue
		}
		k := [2]int{row.Element, row.Fixture}
		if seen[k] {
			dupes++
			continue
		}
		seen[k] = true

		var comp bpsComponents
		comp.Minutes = row.Minutes
		if old {
			comp.Goals = ival(rec, col, "goals_scored")
			comp.Assists = ival(rec, col, "assists")
			comp.CleanSheet = ival(rec, col, "clean_sheets")
			comp.Saves = ival(rec, col, "saves")
			comp.PenSaved = ival(rec, col, "penalties_saved")
			comp.PenMissed = ival(rec, col, "penalties_missed")
			comp.PenConceded = ival(rec, col, "penalties_conceded")
			comp.OwnGoals = ival(rec, col, "own_goals")
			comp.Yellow = ival(rec, col, "yellow_cards")
			comp.Red = ival(rec, col, "red_cards")
			comp.ErrGoal = ival(rec, col, "errors_leading_to_goal")
			comp.ErrAttempt = ival(rec, col, "errors_leading_to_goal_attempt")
			comp.BigChanceCreated = ival(rec, col, "big_chances_created")
			comp.BigChanceMissed = ival(rec, col, "big_chances_missed")
			comp.KeyPass = ival(rec, col, "key_passes")
			comp.Crosses = ival(rec, col, "open_play_crosses")
			comp.Tackles = ival(rec, col, "tackles")
			comp.Tackled = ival(rec, col, "tackled")
			comp.CBI = ival(rec, col, "clearances_blocks_interceptions")
			comp.Recoveries = ival(rec, col, "recoveries")
			comp.Dribbles = ival(rec, col, "dribbles")
			comp.Fouls = ival(rec, col, "fouls")
			comp.Offside = ival(rec, col, "offside")
			comp.TargetMissed = ival(rec, col, "target_missed")
			comp.WinningGoals = ival(rec, col, "winning_goals")
			comp.AttemptedPass = ival(rec, col, "attempted_passes")
			comp.CompletedPass = ival(rec, col, "completed_passes")
		}
		outRows = append(outRows, row)
		outComp = append(outComp, comp)
	}
	if len(outRows) == 0 {
		t.Fatalf("no rows parsed for %s", season)
	}
	return outRows, outComp, dupes
}
