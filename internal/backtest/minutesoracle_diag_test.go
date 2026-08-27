package backtest

// What is perfect knowledge of minutes worth, and how much of it could anyone
// have had?
//
//	DIAG=1 EXP=ORACLEMINUTES FPL_CELLS=/tmp/oracleminutes/cells.csv \
//	    go test ./internal/backtest -run '^TestDiagMinutesOracle$' -count=1 -v -timeout 6h
//	Rscript stats/sweep_inference.R /tmp/oracleminutes/cells.csv
//
// # Three arms, because one number bounded two different things
//
// The first version of this diagnostic ran one oracled arm and reported a single
// figure for "perfect minutes". That conflates two facts with completely
// different reachability:
//
//   - **Selection** — did he start, come off the bench, or not feature. Partly
//     acquirable: press conferences, injury reporting, rotation patterns. This is
//     what a judgement layer is *for*.
//   - **Quantity given selection** — the 62nd-minute hook, the 20-minute cameo,
//     the full ninety. Not knowable at the deadline by anybody, including the
//     manager who will make the substitution.
//
// So the grid runs `OracleAvailability`, `OracleLineups` and `OracleMinutes`
// against one un-oracled baseline, and the interesting quantity is a subtraction.
// Lineups is the reachable bound; **minutes minus lineups is the irreducible
// residual**; availability is the same selection fact at the coarsest possible
// resolution — it fires on a season *total* of zero minutes, so it sees only
// players who never appear all year and no injury that resolves.
//
// **They are nested in information and not in reach**, which matters for reading
// the contrasts. Availability perturbs the bootstrap and so moves the pre-season
// squad build; the other two perturb `Engine.Recent`, which `blendRates` does not
// consult at `played == 0`. At a GW1 entry availability therefore reaches a
// decision the other two provably cannot, and `hold` under availability can
// legitimately exceed `hold` under lineups there without the nesting being broken.
// Only the minutes-versus-lineups contrast shares a seam.
//
// A fourth oracled arm runs the *old* window with the corrected denominator. Two
// defects shipped in one commit and the difference between them was being
// attributed to the window by argument; this measures it.
//
// That is the armband decomposition's lesson applied here. Perfect captaincy
// measured +210 a season, and the number only became useful once it was split
// against the model's own weekly captain and a day-one pinned one, which showed
// the span of captaincy *rules* to be about 28 — the rest being an order
// statistic nobody could have picked.
//
// # It declares no cell invariance, so the mediator is what proves the wiring
//
// These oracles legitimately move every collected metric, because knowing how
// much football a player is about to play changes which fifteen is bought, which
// eleven is fielded, who is captained and every transfer thereafter. Tier 1 does
// the confinement work instead — TestMinutesOraclePerturbsOnlyMinutes asserts that
// only the four minutes fields of the recency index move, for both arms, and
// tierOneCases asserts the bootstrap comes back byte-identical — and that leaves
// nothing for Tier 2 to check.
//
// So the only evidence that an arm is *live* rather than stamped and inert is a
// mediator, counted rather than inferred from points. Two are used. The harness
// checks `moves` after the grid (Tier 3), and this prints **the number of
// gameweeks in which the fifteen held under hindsight differs from the fifteen
// held without it** — an integer, counted exactly.
//
// The opening fifteen is reported beside it because it isolates a different
// channel and because it is expected to be **zero at a GW1 entry**. That is the
// pre-season blindness minutesoracle.go states rather than discovers: the recency
// index is not consulted before a gameweek is played, so a squad bought at the
// GW1 deadline gets no minutes hindsight at all and those cells measure the
// oracle's effect on transfers alone.
//
// # Two provenance columns printed before any of it
//
// **The conditional prices**, because the lineups arm is only as honest as they
// are: a substitute who reads seventy minutes is not a real player, and the
// number is invisible from the points table.
//
// **The reconstructed-start exposure.** `merged_gw.csv` records no starts for
// 2022-23 through GW15, and reconstructStarts infers them by ranking minutes
// within a club-gameweek — whose own recorded boundary is *"never as evidence
// about an individual rotation or returning player"*, which is exactly what the
// lineups arm consumes. The prices are built from recorded rows only; the
// classification is not, and the share affected is printed rather than hidden.

import (
	"fmt"
	"sort"
	"testing"
)

func TestDiagMinutesOracle(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== minutes, decomposed: what could be known and what could be had.\n")
	fmt.Printf("Baseline is the shipped model. Positive means hindsight gains, so each\n")
	fmt.Printf("mean is an upper bound. The three oracled arms are NESTED:\n")
	fmt.Printf("  availability  a season total of zero minutes — never played at all\n")
	fmt.Printf("  lineups       who is picked over the window, priced at conditional averages\n")
	fmt.Printf("  minutes       the realised minutes themselves\n")
	fmt.Printf("The subtraction is the point: lineups is the REACHABLE part, and\n")
	fmt.Printf("minutes minus lineups is the residual nobody could have bought.\n")
	fmt.Printf("They declare no cell invariance — they move everything legitimately —\n")
	fmt.Printf("so the mediators below are the only proof the arms are not inert.\n")

	reportOraclePrices(t)

	base := map[string]*SimResult{}
	rows := map[string][]minutesMediator{}

	baseline := policyVariant{label: "real (ships)", apply: func(sc *SimConfig) {}}
	baseline.observe = func(pair seasonPair, start int, res *SimResult) {
		base[cellKey(pair, start)] = res
	}

	variants := []policyVariant{baseline}
	for _, arm := range []struct {
		oracle InfoOracle
		label  string
		key    string
		apply  func(*SimConfig)
	}{
		{OracleAvailability, "never played at all", "availability", nil},
		{OracleLineups, "perfect lineups", "lineups", nil},
		{OracleMinutes, "perfect minutes", "minutes", nil},
		// The isolating arm. Two defects were corrected in one commit — the
		// unbounded window and the denominator that counted the player's own rows
		// instead of his club's fixtures — so the drop from the recorded figure has
		// two candidate causes and no run separates them. This is the old *window*
		// with the corrected denominator, so the difference between it and the arm
		// above is the window alone. It costs one arm of replay and it turns a
		// causal sentence into a measurement.
		{OracleMinutes, "season-average window", "seasonwindow",
			func(sc *SimConfig) { sc.OracleWindow = 38 }},
	} {
		o := Oracles{Info: arm.oracle}
		name := arm.key
		v := oracleVariant(o, arm.label, arm.apply)
		v.observe = func(pair seasonPair, start int, res *SimResult) {
			b, ok := base[cellKey(pair, start)]
			if !ok {
				t.Errorf("%s@%d: the %s arm ran a cell the baseline did not, so the "+
					"mediator has nothing to compare against", pair.Name, start, name)
				return
			}
			rows[name] = append(rows[name], mediatorFor(pair.Name, start, b, res))
		}
		variants = append(variants, v)
	}

	runPolicySweep(t, variants, starts)

	for _, name := range []string{"availability", "lineups", "minutes", "seasonwindow"} {
		reportMinutesMediator(t, name, rows[name])
	}
}

// reportOraclePrices prints what the lineups arm pays for a start and for a
// substitute appearance, and how much of each season's start data was
// reconstructed rather than recorded.
//
// Printed before the grid because both are provenance for the arm below, and
// because a conditional average that is wrong produces a perfectly plausible
// points table.
func reportOraclePrices(t *testing.T) {
	t.Helper()
	fmt.Printf("\nCONDITIONAL PRICES — what the lineups arm pays per state, in minutes.\n")
	fmt.Printf("A start is most of a match; a substitute appearance is 15-25 minutes and\n")
	fmt.Printf("must not reach sixty, where appearance points and the clean sheet step.\n")
	fmt.Printf("'recon' is the share of club-gameweeks whose `starts` this parser inferred\n")
	fmt.Printf("from minutes: usable in aggregate, explicitly not as individual evidence.\n")
	fmt.Printf("%-9s %8s %8s %8s %8s %8s %8s\n",
		"season", "start", "sub", "GKPst", "DEFst", "MIDst", "recon")
	for _, pair := range sweepPairNames() {
		cur := loadForInputDiff(t, pair[1])
		tab := newConditionalTable(cur)
		league, ok := tab.leagueMinutes()
		if !ok {
			fmt.Printf("%-9s  no conditional average at all\n", pair[1])
			continue
		}
		pos := func(p int) float64 {
			cm, ok := tab.forPlayer(-1, p)
			if !ok {
				return 0
			}
			return cm.start
		}
		// Exposure over the windows the arm actually classifies, not over the
		// season. A season-wide share is diluted by every gameweek outside a
		// window — 2022-23's reconstruction stops at GW15 — and, being a season
		// constant, cannot vary with the window at all, which is the tell that it
		// is answering a different question.
		classified, recon := reconstructedInWindows(cur, defaultOracleWindow)
		share := 0.0
		if classified > 0 {
			share = float64(recon) / float64(classified)
		}
		fmt.Printf("%-9s %8.1f %8.1f %8.1f %8.1f %8.1f %7.1f%%\n",
			pair[1], league.start, league.sub, pos(1), pos(2), pos(3), 100*share)
	}
}

func cellKey(pair seasonPair, start int) string {
	return fmt.Sprintf("%s@%d", pair.Name, start)
}

// minutesMediator is one cell's count of how far the hindsight squad drifted
// from the honest one.
type minutesMediator struct {
	season             string
	start              int
	openingDiff        int // of the fifteen bought, how many differ
	weeks, weeksDiff   int // gameweeks, and those where the held fifteen differs
	baseMoves, oMoves  int
	basePoints, points int
}

// mediatorFor counts the two squad differences for one cell.
//
// Both are set differences over player ids, not positional comparisons: the two
// arms order their fifteen independently, so comparing slot by slot would report a
// permutation as a change and manufacture a mediator out of nothing.
func mediatorFor(season string, start int, base, oracle *SimResult) minutesMediator {
	m := minutesMediator{
		season: season, start: start,
		weeks:      len(oracle.Weeks),
		baseMoves:  base.Transfers,
		oMoves:     oracle.Transfers,
		basePoints: base.Points,
		points:     oracle.Points,
	}
	m.openingDiff = countMissing(oracle.OpeningSquad, base.OpeningSquad)
	// The two arms play the same gameweeks in the same order, because StartGW and
	// the season are the same cell. Zip rather than match on Week.GW so a length
	// mismatch is loud rather than silently truncating the comparison.
	n := len(base.Weeks)
	if len(oracle.Weeks) < n {
		n = len(oracle.Weeks)
	}
	for i := 0; i < n; i++ {
		if countMissing(oracle.Weeks[i].Squad, base.Weeks[i].Squad) > 0 {
			m.weeksDiff++
		}
	}
	return m
}

// countMissing is how many of a are not in b.
func countMissing(a, b []int) int {
	in := make(map[int]bool, len(b))
	for _, id := range b {
		in[id] = true
	}
	n := 0
	for _, id := range a {
		if !in[id] {
			n++
		}
	}
	return n
}

func reportMinutesMediator(t *testing.T, arm string, rows []minutesMediator) {
	t.Helper()
	if len(rows) == 0 {
		t.Errorf("the %s arm observed no cells, so nothing above was measured", arm)
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].start != rows[j].start {
			return rows[i].start < rows[j].start
		}
		return rows[i].season < rows[j].season
	})
	fmt.Printf("\nMEDIATOR (%s) — how far the hindsight squad drifted from the honest one.\n", arm)
	fmt.Printf("'opening' is of fifteen; for lineups and minutes it is 0 at a GW1 entry by\n")
	fmt.Printf("construction, because the recency index is not consulted before a gameweek\n")
	fmt.Printf("is played. Availability perturbs the bootstrap instead and does move it.\n")
	fmt.Printf("%-9s %6s %8s %7s %7s %7s %7s %9s %9s\n",
		"season", "start", "opening", "weeks", "differ", "moves", "oracle", "points", "oracle")
	var opening, weeks, weeksDiff, baseMoves, oMoves, basePts, pts int
	for _, r := range rows {
		fmt.Printf("%-9s %6d %8d %7d %7d %7d %7d %9d %9d\n",
			r.season, r.start, r.openingDiff, r.weeks, r.weeksDiff,
			r.baseMoves, r.oMoves, r.basePoints, r.points)
		opening += r.openingDiff
		weeks += r.weeks
		weeksDiff += r.weeksDiff
		baseMoves += r.baseMoves
		oMoves += r.oMoves
		basePts += r.basePoints
		pts += r.points
	}
	fmt.Printf("%-9s %6s %8d %7d %7d %7d %7d %9d %9d\n", "all", "",
		opening, weeks, weeksDiff, baseMoves, oMoves, basePts, pts)

	if weeksDiff == 0 {
		t.Errorf("the %s arm held a squad identical to the honest one in every "+
			"gameweek of every cell — an oracle that changes nothing measures "+
			"nothing and reports it as a clean null", arm)
	}
}
