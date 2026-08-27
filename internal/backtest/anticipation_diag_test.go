package backtest

// Does a policy that knows a chip is coming take a different position?
//
//	DIAG=1 EXP=ANTICIPATE go test ./internal/backtest \
//	    -run '^TestDiagChipAnticipation$' -v -timeout 4h
//
// # The hypothesis, and why it is the record's own
//
// Fixture value has failed to pay five separate ways, and every one of those
// tests concluded that you buy a *run* of fixtures and runs converge. The
// record's own open question is whether that null and the wildcard null are the
// same missing mechanism: **every one of those tests measured fixture-chasing
// without a cheap exit.** A wildcard's worth may not be repair at all but option
// value — it makes a concentrated, short-window position affordable, because
// unwinding it costs nothing. A free hit is the same shape at one week's
// resolution, and a bench boost is its mirror, magnifying a good week rather than
// deleting a bad one.
//
// # The unwired function is the finding, before any number
//
// `Engine.Chips` is set nowhere in internal/backtest. The replay reads
// SimConfig.Chips only to decide which gameweek to *play* a chip in, so
// `EffectiveHorizon`, `SuggestBenchWeight`, `ApplyChipPlan` and
// `ApplyFreeHitToScoring` — all live in internal/analysis, all exercised by the
// agent — have been dead in every replayed season. The record's "the policy plays
// identically whether or not it holds a wildcard" is therefore an unwired
// function rather than a modelling gap, and every chip figure in the record was
// produced by a blind policy.
//
// # Why this is measurable where wildcard TIMING is not
//
// The record's verdict is that wildcard timing cannot be measured here: a chip
// week is worth −267 to +91 within one season, so any trigger is graded by luck
// of placement. **Both arms here play the chips in the same weeks.** Placement
// becomes a nuisance factor shared by the pair and cancels in the paired
// difference, which is the same trick that let the vice-captain fix resolve.
//
// # Mediators first, points later, and that ordering is the point
//
// Falsification is about two orders of magnitude cheaper than confirmation on
// this harness: the transfer metric's detection threshold on this grid is 107 to
// 139 points a season, the whole chip ceiling is about 42, and chip timing is
// 8.3. **An anticipation effect is very unlikely to resolve on points.** So this
// asks the cheap question first — does the policy behave differently at all, and
// in the direction the option-value story predicts — and a null here kills the
// idea for minutes rather than hours.
//
// Three counted quantities, none of them the headline:
//
//   - **weeks the squad differs**, split into the weeks before each chip and the
//     rest. Anticipation can only act *before* a chip, so a difference appearing
//     only afterwards would be path drift rather than the mechanism.
//   - **club concentration** — the largest number of players held from one club,
//     averaged over the pre-chip weeks. This is what a free exit is supposed to
//     make affordable, and it is the direction the option-value story predicts.
//   - **transfers made before the chip.** Two opposite predictions live here and
//     the sign is informative either way: a shortened horizon makes a fixture run
//     look better (more moves), while a move discarded by the wildcard has fewer
//     weeks to repay (fewer). Only the first is wired — see
//     SimConfig.AnticipateChips — so a fall would mean something other than the
//     lever moved it.
//
// # What this arm does NOT carry
//
// **The bench boost.** `XIValue` takes a squad and no bench weight, so
// `SuggestBenchWeight` has nowhere to go on the transfer path; it reaches only
// the opening squad. An arm claiming three mechanisms while one of them is
// structurally inert is the silent no-op this package keeps paying for, so it is
// named rather than counted.
//
// **The opening squad.** `cfg.anticipate` is called at exactly one engine site —
// the one the weekly transfer decision runs against — and
// `TestOnlyTheTransferEngineAnticipatesChips` pins that. So this arm cannot see
// the fifteen you *buy* three weeks before a wildcard tears it up, which is a
// real part of the option-value story and the half the record's "the policy plays
// identically whether or not it holds a wildcard" was mostly about.
//
// That is also what makes `HOLD` byte-identical: it is a **construction
// guarantee**, not evidence. It is worth having as a leak check — if the lever
// had reached squad construction, HOLD would have moved — and it says nothing
// about whether anticipation is worth anything.

import (
	"fmt"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// chipOffsets place the two chips relative to the cell's entry gameweek.
//
// Relative rather than absolute because the grid enters at GW1, 6, 11, 16, 21 and
// 26, and a fixed GW4 wildcard cannot exist in four of those. The wildcard sits
// three gameweeks after entry on the one mechanism the record supports with sign
// consistency across four seasons — the opening fifteen is built on the weakest
// information of the season and three gameweeks of real minutes is a large
// information gain. The free hit is placed well clear of it; one chip per
// gameweek is an FPL rule and checkChipPlan enforces it.
//
// **Asserted, not swept.** Sweeping the placement is the thing the record says
// cannot be done here, and picking the best of eleven weeks would be fitting to
// noise. Both arms use the same weeks, which is what makes the comparison mean
// anything.
const (
	wildcardOffset = 3
	freeHitOffset  = 10
)

func TestDiagChipAnticipation(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== does a policy that knows a chip is coming position differently?\n")
	fmt.Printf("Both arms play the SAME chips in the SAME weeks — a wildcard at\n")
	fmt.Printf("entry+%d and a free hit at entry+%d. Only whether the weekly transfer\n",
		wildcardOffset, freeHitOffset)
	fmt.Printf("decision KNOWS about them varies, so placement — the thing that makes\n")
	fmt.Printf("wildcard timing unmeasurable here — cancels in the paired difference.\n")
	fmt.Printf("HOLD must be byte-identical: this arm touches the transfer engine only,\n")
	fmt.Printf("and the held metric never transfers.\n")
	fmt.Printf("The mediators below are the deliverable. The points column is not\n")
	fmt.Printf("expected to resolve: the transfer metric needs 107-139 a season here.\n")

	withChips := func(sc *SimConfig) {
		sc.Chips = analysis.ChipPlan{
			Wildcard: sc.startGW() + wildcardOffset,
			FreeHit:  sc.startGW() + freeHitOffset,
		}
	}

	base := map[string]*SimResult{}
	var rows []anticipationRow

	blind := policyVariant{label: "chips played, policy blind", apply: withChips}
	blind.observe = func(pair seasonPair, start int, res *SimResult) {
		base[cellKey(pair, start)] = res
	}

	seeing := policyVariant{
		label: "chips played, policy anticipates",
		apply: func(sc *SimConfig) {
			withChips(sc)
			sc.AnticipateChips = true
		},
	}
	seeing.observe = func(pair seasonPair, start int, res *SimResult) {
		b, ok := base[cellKey(pair, start)]
		if !ok {
			t.Errorf("%s@%d: the anticipating arm ran a cell the blind one did not",
				pair.Name, start)
			return
		}
		rows = append(rows, anticipationFor(t, pair, start, b, res))
	}

	// The third arm shortens the gate in step, which is what makes the free-exit
	// model internally consistent. Scoring-only is the mismatched combination:
	// a move judged on a one-to-four-week expectation and charged over five.
	var bothRows []anticipationRow
	both := policyVariant{
		label: "chips played, policy anticipates, gate follows",
		apply: func(sc *SimConfig) {
			withChips(sc)
			sc.AnticipateChips = true
			sc.AnticipateGate = true
		},
	}
	both.observe = func(pair seasonPair, start int, res *SimResult) {
		b, ok := base[cellKey(pair, start)]
		if !ok {
			t.Errorf("%s@%d: the gate-following arm ran a cell the blind one did not",
				pair.Name, start)
			return
		}
		bothRows = append(bothRows, anticipationFor(t, pair, start, b, res))
	}

	runPolicySweep(t, []policyVariant{blind, seeing, both}, starts)
	fmt.Printf("\n--- ARM 2: scoring shortened, gate NOT (the mismatched one) ---\n")
	reportAnticipation(t, rows)
	fmt.Printf("\n--- ARM 3: scoring and gate shortened together (coherent) ---\n")
	reportAnticipation(t, bothRows)
}

// anticipationRow is one cell's answer to "did anticipating change anything".
type anticipationRow struct {
	season           string
	start            int
	preWeeks         int // gameweeks strictly before a chip
	preDiff, sumDiff int // of those, and of all weeks, where the squad differs
	blindConc        float64
	seeConc          float64 // mean largest holding from one club, pre-chip
	blindTop3        float64
	seeTop3          float64 // players in the three largest clubs: uncensored, ~6 to 9 of 15
	blindFDR         float64
	seeFDR           float64 // mean fixture difficulty faced, pre-chip. Lower is easier
	blindPre, seePre int     // transfers made before a chip
	blindPts, seePts int
}

// anticipationFor counts the mediators for one cell.
//
// Club concentration is computed from the season's own player-to-club map rather
// than from anything the simulation reports, so it cannot be an artefact of what
// the policy chose to record.
func anticipationFor(t *testing.T, pair seasonPair, start int, blind, seeing *SimResult) anticipationRow {
	t.Helper()
	r := anticipationRow{
		season: pair.Name, start: start,
		blindPts: blind.Points, seePts: seeing.Points,
	}
	club := map[int]int{}
	for _, p := range pair.Cur.Players {
		club[p.ID] = p.Team
	}
	chips := map[int]bool{start + wildcardOffset: true, start + freeHitOffset: true}
	preChip := func(gw int) bool {
		return gw < start+wildcardOffset ||
			(gw > start+wildcardOffset && gw < start+freeHitOffset)
	}

	n := len(blind.Weeks)
	if len(seeing.Weeks) < n {
		n = len(seeing.Weeks)
	}
	byGW := fixturesByGameweek(pair.Cur.Fixtures)
	var bc, sc, bf, sf, bt, st []float64
	for i := 0; i < n; i++ {
		bw, sw := blind.Weeks[i], seeing.Weeks[i]
		differs := countMissing(sw.Squad, bw.Squad) > 0
		if differs {
			r.sumDiff++
		}
		// preChip already excludes both chip weeks — a week equal to either offset
		// fails both of its clauses — so everything past this point is a
		// transfer-eligible week before a chip. A second `if !chips[gw]` test
		// inside the loop looked like a guard and was dead code; the map is kept
		// because `preChip` is the thing that has to stay in step with it.
		if !preChip(bw.GW) || chips[bw.GW] {
			continue
		}
		r.preWeeks++
		if differs {
			r.preDiff++
		}
		bc = append(bc, maxPerClub(bw.Squad, club))
		sc = append(sc, maxPerClub(sw.Squad, club))
		bt = append(bt, topThreeClubs(bw.Squad, club))
		st = append(st, topThreeClubs(sw.Squad, club))
		// Both arms' difficulty is taken in the same gameweek from the same
		// fixture list, so the comparison is of the squads and nothing else — and
		// a week is admitted to **both** means or to neither, which is what makes
		// the per-cell difference of the two means paired. Appending them
		// independently, as this did, would let a week in which one squad happens
		// to hold nobody with a fixture enter one mean and not the other; rare, and
		// silently unpaired when it fires.
		bd, bok := meanSquadDifficulty(bw.Squad, club, byGW, bw.GW)
		sd, sok := meanSquadDifficulty(sw.Squad, club, byGW, sw.GW)
		if bok && sok {
			bf = append(bf, bd)
			sf = append(sf, sd)
		}
		r.blindPre += bw.Transfers
		r.seePre += sw.Transfers
	}
	r.blindConc, r.seeConc = meanOf(bc), meanOf(sc)
	r.blindTop3, r.seeTop3 = meanOf(bt), meanOf(st)
	r.blindFDR, r.seeFDR = meanOf(bf), meanOf(sf)
	return r
}

// meanSquadDifficulty is the average FPL difficulty of the fixtures a squad's
// players face in one gameweek, from the squad's own point of view.
//
// This is the mediator the concentration one cannot supply, and it is the direct
// test. Club concentration answers "did it stack one team", and it came back
// nearly flat — but it had a hidden premise: that what limits a concentrated
// position is the cost of unwinding it. It is not, it is FPL's three-per-club
// cap, which both arms already sit against. A fixture bet need not be
// concentrated at all: eleven players from eleven clubs can all have an easy
// week. So the quantity that actually tests the option-value story is whether the
// squad was pointed at easier fixtures in the weeks before the chip.
//
// A club playing twice contributes both fixtures and a club not playing
// contributes nothing, which is right: a blank is not an easy fixture, it is no
// fixture, and averaging it in as a zero would read a blank as the easiest
// possible week.
//
// Lower is easier, so anticipation chasing good fixtures shows as a *fall*.
func meanSquadDifficulty(squad []int, club map[int]int, byGW map[int][]fpl.Fixture, gw int) (float64, bool) {
	var sum, n float64
	for _, id := range squad {
		t := club[id]
		for _, f := range byGW[gw] {
			switch t {
			case f.TeamH:
				sum += float64(f.TeamHDifficulty)
				n++
			case f.TeamA:
				sum += float64(f.TeamADifficulty)
				n++
			}
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / n, true
}

// fixturesByGameweek indexes the season's fixtures by the gameweek they fall in.
// Unscheduled fixtures carry a nil event and are skipped rather than defaulted.
func fixturesByGameweek(fx []fpl.Fixture) map[int][]fpl.Fixture {
	byGW := map[int][]fpl.Fixture{}
	for _, f := range fx {
		if f.Event == nil {
			continue
		}
		byGW[*f.Event] = append(byGW[*f.Event], f)
	}
	return byGW
}

// topThreeClubs is the share of a fifteen drawn from its three largest clubs.
//
// It exists because `maxPerClub` is **censored** and cannot express the
// hypothesis: FPL caps a squad at three players per club, so that statistic lives
// entirely in 2.7 to 3.0 and a move of 0.10 is a third of the whole available
// range rather than the flatness it looks like. Reading it as a null was wrong in
// both directions — the statistic cannot test the claim, and what it does show
// leans toward the mechanism.
//
// The sum of the top three ranges over roughly 6 to 9 of 15 and is not capped in
// the region that matters, and it distinguishes the thing a fixture bet actually
// looks like: not one club stacked, but three clubs with the same good week.
func topThreeClubs(squad []int, club map[int]int) float64 {
	n := map[int]int{}
	for _, id := range squad {
		n[club[id]]++
	}
	var counts []int
	for _, c := range n {
		counts = append(counts, c)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(counts)))
	var sum float64
	for i := 0; i < 3 && i < len(counts); i++ {
		sum += float64(counts[i])
	}
	return sum
}

// maxPerClub is the largest number of a squad's players drawn from one club —
// the quantity a free exit is supposed to make affordable.
//
// **Censored at 3 by FPL's squad rule**, so it cannot falsify the option-value
// claim on its own. Kept because it is what the first version of this diagnostic
// reported and the record needs to be able to reproduce it; read topThreeClubs
// beside it.
func maxPerClub(squad []int, club map[int]int) float64 {
	n := map[int]int{}
	best := 0
	for _, id := range squad {
		n[club[id]]++
		if n[club[id]] > best {
			best = n[club[id]]
		}
	}
	return float64(best)
}

func reportAnticipation(t *testing.T, rows []anticipationRow) {
	t.Helper()
	if len(rows) == 0 {
		t.Fatal("the anticipating arm observed no cells")
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].start != rows[j].start {
			return rows[i].start < rows[j].start
		}
		return rows[i].season < rows[j].season
	})
	fmt.Printf("\nMEDIATORS — did knowing about the chip change the position taken?\n")
	fmt.Printf("'pre' counts only gameweeks before a chip, which is the only place\n")
	fmt.Printf("anticipation can act; a difference appearing only after one is drift.\n")
	fmt.Printf("'conc' is the mean largest holding from a single club, pre-chip: the\n")
	fmt.Printf("concentrated position a free exit is supposed to make affordable.\n")
	fmt.Printf("'fdr' is the mean FPL difficulty of the fixtures the squad actually\n")
	fmt.Printf("faced, pre-chip. LOWER IS EASIER, so chasing good weeks shows as a fall.\n")
	fmt.Printf("It is the direct test: a fixture bet need not be concentrated in one\n")
	fmt.Printf("club, so 'conc' can be flat while this moves.\n")
	fmt.Printf("%-9s %6s %6s %6s %6s %7s %7s %6s %6s %6s %6s %8s %8s\n",
		"season", "start", "preGW", "preDif", "allDif",
		"concB", "concA", "fdrB", "fdrA", "mvB", "mvA", "ptsB", "ptsA")
	var preW, preD, allD, mvB, mvA, pB, pA int
	var cB, cA, fB, fA, fD, tB, tA []float64
	for _, r := range rows {
		fmt.Printf("%-9s %6d %6d %6d %6d %7.2f %7.2f %6.2f %6.2f %6d %6d %8d %8d\n",
			r.season, r.start, r.preWeeks, r.preDiff, r.sumDiff,
			r.blindConc, r.seeConc, r.blindFDR, r.seeFDR,
			r.blindPre, r.seePre, r.blindPts, r.seePts)
		preW += r.preWeeks
		preD += r.preDiff
		allD += r.sumDiff
		mvB += r.blindPre
		mvA += r.seePre
		pB += r.blindPts
		pA += r.seePts
		cB = append(cB, r.blindConc)
		cA = append(cA, r.seeConc)
		tB = append(tB, r.blindTop3)
		tA = append(tA, r.seeTop3)
		fB = append(fB, r.blindFDR)
		fA = append(fA, r.seeFDR)
		fD = append(fD, r.seeFDR-r.blindFDR)
	}
	fmt.Printf("%-9s %6s %6d %6d %6d %7.2f %7.2f %6.2f %6.2f %6d %6d %8d %8d\n", "all", "",
		preW, preD, allD, meanOf(cB), meanOf(cA), meanOf(fB), meanOf(fA),
		mvB, mvA, pB, pA)

	// 'conc' is capped at 3 by FPL's squad rule, so it lives in 2.7 to 3.0 and
	// cannot express the hypothesis. The top-three sum is not capped where it
	// matters and is the honest version of the same question.
	fmt.Printf("\nconcentration: max-per-club %.3f -> %.3f (CENSORED at 3.00, so a\n",
		meanOf(cB), meanOf(cA))
	fmt.Printf("  move of 0.10 is a third of the whole available range, not flatness)\n")
	fmt.Printf("  players in the top three clubs %.3f -> %.3f of 15 (uncensored)\n",
		meanOf(tB), meanOf(tA))

	// The paired difference is what carries the sign, cell by cell, rather than a
	// difference of two means over cells that need not contain the same weeks.
	easier, harder := 0, 0
	for _, d := range fD {
		switch {
		case d < 0:
			easier++
		case d > 0:
			harder++
		}
	}
	fmt.Printf("\nfixture difficulty, paired per cell: mean %+.4f (negative = the\n",
		meanOf(fD))
	fmt.Printf("anticipating arm faced EASIER fixtures before the chip), easier in %d\n",
		easier)
	fmt.Printf("cells, harder in %d, unchanged in %d of %d.\n",
		harder, len(fD)-easier-harder, len(fD))

	// Cells within a season replay the same football through the same lever, so
	// 24 is not 24 independent draws and a cell-level sign count overstates its
	// own evidence. The season is the honest unit and there are four of them,
	// which floors a clustered sign test at p = 1/16 even when all four agree —
	// so this can never reach significance and is reported as a shape, which is
	// the form this record already trusts ("all four season means are positive").
	bySeason := map[string][]float64{}
	for i, r := range rows {
		bySeason[r.season] = append(bySeason[r.season], fD[i])
	}
	var names []string
	for s := range bySeason {
		names = append(names, s)
	}
	sort.Strings(names)
	seasonsEasier := 0
	fmt.Printf("\nclustered to the season, which is the unit that is independent:\n")
	for _, s := range names {
		m := meanOf(bySeason[s])
		if m < 0 {
			seasonsEasier++
		}
		fmt.Printf("  %-9s %+.4f\n", s, m)
	}
	fmt.Printf("  easier in %d of %d seasons. A clustered sign test floors at\n",
		seasonsEasier, len(names))
	fmt.Printf("  p = 1/16 = 0.0625 one-sided at 4 of 4, so read the shape, not a p.\n")
	fmt.Printf("  And note the sign is near-mechanical: shortening the fixture window\n")
	fmt.Printf("  makes the scorer prefer good NEAR fixtures, which is what this\n")
	fmt.Printf("  mediator measures. It is an implementation check, not evidence\n")
	fmt.Printf("  about football.\n")

	fmt.Printf("\nRead the mediators, not the points: this comparison needs 107-139\n")
	fmt.Printf("points a season to resolve and the whole chip ceiling is about 42.\n")

	if preD == 0 {
		t.Error("the anticipating policy held an identical squad in every gameweek " +
			"before every chip, in every cell — the free-exit lever reaches no " +
			"decision at all, and a points run would report the clean null an inert " +
			"arm reports")
	}
}
