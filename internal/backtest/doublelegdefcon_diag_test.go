package backtest

// A DESCRIPTIVE split, explicitly under-powered by design: among players who
// started at least one leg of a 2025-26 double gameweek, do higher-defcon
// players start BOTH legs more often?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDoubleGameweekDefconTercile -v -timeout 60m
//
// # The question this feeds
//
// The owner's hypothesis is specific to doubles, and sharper than the general
// defcon-as-role-proxy question TestDiagDefconMinutesPerStart already answers:
// in a double gameweek a manager deliberately reaches into the rotation pool,
// hoping for ~1.5 games out of a marginal player. The payoff there is not a
// few minutes of scaling but a WHOLE EXTRA FIXTURE — did he start one leg or
// both? If defcon rate predicts managerial trust, the players a manager trusts
// enough to start twice in one gameweek should skew toward higher defcon.
//
// # ⚠️ PRE-COMMITTED BEFORE RUNNING: THIS CANNOT RESOLVE, AND SAYS SO
//
// TestDiagDoubleGameweekCensus already established the population this
// measurement has to work with: 3 double gameweeks, 10 club-doubles total
// (GW26 x2 clubs, GW33 x6, GW36 x2), 409 real rows, starts split 273/52/84
// for 0/1/2 legs.
//
// Rotation in a double is a MANAGER'S decision for a CLUB, not an independent
// coin flip per player. A midfielder-row count in the dozens is not that many
// independent observations — it is a handful of club-doubles each contributing
// several same-club players at once, and GW33 alone supplies six of the ten
// club-doubles. The effective independent sample is nearer ten than the row
// count. Defenders are additionally imbalanced in the general population (38
// of 50 real rows started both legs), which further limits what a defender
// split here can show.
//
// So: ⚠️ **no p-value, no t-statistic, no critical value is computed anywhere
// in this file.** That would be inference on pseudo-replicated data, exactly
// the error this project's own record warns against. Only proportions and
// counts are reported, plus the per-club-double breakdown (section 3 below) so
// a reader sees directly how few independent units back any proportion.
//
// # Population, precisely
//
// Season 2025-26 (the only season `DefconScoredIn` allows), rows with
// `Fixtures == 2` and `Starts >= 1`, restricted to players whose SEASON
// `defcon_per_90` is stable: at least `defconMinStarts` (10) qualifying
// single-fixture starts, using the IDENTICAL aggregate `buildDefconAgg`
// computes for `TestDiagDefconMinutesPerStart` — same threshold constant, same
// function, not a second implementation of "enough starts to trust a rate".
//
// That aggregate only ever counts `Fixtures == 1` rows (see its own header),
// so a player's season `defcon_per_90` here is computed entirely OFF the
// double-gameweek rows this diagnostic studies — there is no channel by which
// starting both legs of a double could inflate the very rate used to predict
// it.
//
// Every `Fixtures == 2` row is cross-checked against `fixtures.csv` exactly
// as `TestDiagDoubleGameweekCensus` does: the actual fixture IDs behind the
// row (via `gameweekRows`, not a re-derivation of the row guards) must share a
// club that `fixtures.csv` itself records playing twice that gameweek
// (`commonClub`, `clubHasDouble`, both defined in the census file and reused
// here rather than reimplemented). A row that fails this check is a phantom
// candidate and is dropped, printed individually, never silently folded in.
//
// # What to measure, and why terciles
//
// Within DEF and MID separately (FWD printed for completeness only — the
// population is far too small there to interpret; see the census's Q5), split
// the population's distinct players into three defcon terciles by their
// season `defcon_per_90` (ties broken by player id, ascending, for
// determinism — same convention `defconTopByPoints` uses). Equal-COUNT
// terciles, not equal-width bins: the question is about the ranking, not
// about absolute rate levels which differ in scale across positions.
//
// Per tercile: the number of qualifying ROWS it contributes (a player who
// appears in more than one club-double contributes one row per appearance,
// which is the same row-counting convention the census uses throughout — see
// its Q3-Q5), the tercile's median `defcon_per_90` (over its distinct
// players), and the proportion of its rows with `Starts == 2` (both legs)
// against `Starts >= 1` (the population's own definition, so this proportion
// is always well-formed).
//
// # The honesty column: the per-club-double breakdown
//
// Because a club's rotation decision is the real unit of replication, section
// 3 below lists all ten club-doubles from the census (identified the same
// way: a `(gw, club)` pair from `fixtures.csv` alone) and, for the position
// being read, how many qualifying rows each one contributed and what share of
// THOSE started both legs. A reader who sees one club-double supplying most of
// a tercile's rows knows immediately that the tercile's proportion is closer
// to one club's single decision than to an average over several independent
// managers.
//
// # Pre-registered reading — committed to BEFORE seeing any numbers
//
//   - Top tercile starts both materially more often than the bottom (>= 15
//     percentage points), and the ordering across all three terciles is
//     monotone: a DIRECTION worth re-testing once 2026-27 adds roughly ten
//     more club-doubles to pool against. Still not a basis for building
//     anything.
//   - Flat, non-monotone, or reversed: the line dies here and is recorded as
//     dead.
//
// Either way this does not resolve, and no scoring change, sweep or replay
// follows from this file no matter what the numbers show.
//
// # What this changes
//
// Nothing. DIAGNOSTIC ONLY — no scoring term moves and no config value
// changes.

import (
	"context"
	"fmt"
	"sort"
	"testing"
)

// doubleLegClubKey identifies one club's double gameweek: which gameweek,
// which club. This is the unit the "honesty column" (section 3) counts over,
// since a club's rotation call for one double is the real independent
// observation, not any individual player-row inside it.
type doubleLegClubKey struct {
	gw, club int
}

// doubleLegRow is one qualifying observation: an established player's
// cross-checked-real Fixtures==2, Starts>=1 row, tagged with the club-double
// it came from and the season defcon_per_90 that predicts it. defconPer90 is
// carried on the row (rather than looked up again from a separate map at
// print time) because a player can contribute more than one row and every
// copy must read the identical, already-computed rate.
type doubleLegRow struct {
	id       int
	pos      int
	starts   int // 1 or 2, since the population itself requires Starts >= 1
	defconP9 float64
	key      doubleLegClubKey
}

// doubleLegPlayerRate is one distinct player's season defcon_per_90, the unit
// terciles are split over (never rows — a player's rate does not change
// across however many club-doubles he appears in).
type doubleLegPlayerRate struct {
	id   int
	rate float64
}

// doubleLegTerciles splits players (already meant to be exactly the
// population for one position) into three equal-COUNT groups by ascending
// rate, ties broken by id ascending for determinism. Equal-count, not
// equal-width, because the question is about RANKING within the position's
// own spread, not about an absolute rate level that differs in scale between
// DEF and MID. Group sizes differ by at most one when the population is not
// divisible by three; any remainder lands in the later groups, matching the
// boundary arithmetic `n/3` and `2*n/3` give under integer division (e.g.
// n=14 -> sizes 4,5,5, matching the task's own pre-computed FWD figure).
func doubleLegTerciles(players []doubleLegPlayerRate) [3][]doubleLegPlayerRate {
	sorted := append([]doubleLegPlayerRate(nil), players...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].rate != sorted[j].rate {
			return sorted[i].rate < sorted[j].rate
		}
		return sorted[i].id < sorted[j].id
	})
	n := len(sorted)
	i1, i2 := n/3, 2*n/3
	var out [3][]doubleLegPlayerRate
	out[0] = sorted[:i1]
	out[1] = sorted[i1:i2]
	out[2] = sorted[i2:]
	return out
}

// doubleLegGroupStats reduces the rows belonging to a set of players (a
// tercile, or any other subset) to the row count and the both-legs count.
// Extracted as a pure function of (rows, id-set) so
// TestDoubleLegDefconTercileWiring can pin the arithmetic against a synthetic
// row list without touching the archive.
func doubleLegGroupStats(rows []doubleLegRow, ids map[int]bool) (nRows, nBoth int) {
	for _, r := range rows {
		if !ids[r.id] {
			continue
		}
		nRows++
		if r.starts == 2 {
			nBoth++
		}
	}
	return
}

// doubleLegClubStats is doubleLegGroupStats' sibling keyed on the club-double
// itself rather than a player set — section 3's honesty column.
func doubleLegClubStats(rows []doubleLegRow, key doubleLegClubKey) (nRows, nBoth int) {
	for _, r := range rows {
		if r.key != key {
			continue
		}
		nRows++
		if r.starts == 2 {
			nBoth++
		}
	}
	return
}

func pctOf(nBoth, nRows int) string {
	if nRows == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%5.1f%% (%d/%d)", 100*float64(nBoth)/float64(nRows), nBoth, nRows)
}

func TestDiagDoubleGameweekDefconTercile(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	const season = "2025-26"
	fmt.Printf("\n=== season selection ===\n")
	fmt.Printf("DefconScoredIn gates every season before 2025-26 to zero or meaningless\n")
	fmt.Printf("defcon. %s is the only season this project's archive returns true for, so\n", season)
	fmt.Printf("it is also the only season this diagnostic can run against.\n")
	if !DefconScoredIn(season) {
		t.Fatalf("DefconScoredIn(%q) = false; this diagnostic has nothing to measure", season)
	}

	s, err := Load(ctx, cfg.CacheDir, season)
	if err != nil {
		t.Fatal(err)
	}
	if s.RowGuards == nil || s.RowGuards.Guards < rowGuardCount {
		t.Fatalf("season loaded with RowGuards=%+v: the duplicate-row guard did not run at "+
			"the current version, so any Fixtures==2 row below could be a phantom rather than "+
			"a real double — Load should have refused this cache", s.RowGuards)
	}

	// Season-basis defcon_per_90 per player, from the SAME aggregate and
	// threshold TestDiagDefconMinutesPerStart uses. Built entirely from
	// Fixtures==1 rows (see buildDefconAgg's own header), so it cannot be
	// contaminated by the Fixtures==2 rows this diagnostic studies below.
	agg, _ := buildDefconAgg(s)

	// Fixture index and the club-double map, built ONLY from fixtures.csv's
	// Event/TeamH/TeamA — identical construction to TestDiagDoubleGameweekCensus,
	// so the "10 club-doubles" this file reports is the same list, not a second
	// count that could quietly disagree with it.
	fixByID := map[int]struct{ event, teamH, teamA int }{}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		fixByID[f.ID] = struct{ event, teamH, teamA int }{*f.Event, f.TeamH, f.TeamA}
	}
	matchesPerClubGW := map[int]map[int]int{}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		gw := *f.Event
		if matchesPerClubGW[gw] == nil {
			matchesPerClubGW[gw] = map[int]int{}
		}
		matchesPerClubGW[gw][f.TeamH]++
		matchesPerClubGW[gw][f.TeamA]++
	}
	doubleClubsByGW := map[int][]int{}
	var allClubDoubles []doubleLegClubKey
	for gw, teams := range matchesPerClubGW {
		var clubs []int
		for team, n := range teams {
			if n >= 2 {
				clubs = append(clubs, team)
			}
		}
		if len(clubs) > 0 {
			sort.Ints(clubs)
			doubleClubsByGW[gw] = clubs
			for _, c := range clubs {
				allClubDoubles = append(allClubDoubles, doubleLegClubKey{gw: gw, club: c})
			}
		}
	}
	sort.Slice(allClubDoubles, func(i, j int) bool {
		if allClubDoubles[i].gw != allClubDoubles[j].gw {
			return allClubDoubles[i].gw < allClubDoubles[j].gw
		}
		return allClubDoubles[i].club < allClubDoubles[j].club
	})
	fmt.Printf("\n=== club-doubles this season (fixtures.csv alone) ===\n")
	fmt.Printf("total: %d\n", len(allClubDoubles))
	for _, k := range allClubDoubles {
		fmt.Printf("  GW%-3d club %d\n", k.gw, k.club)
	}

	// Re-walk the guarded row stream for the actual fixture IDs behind every
	// player-gameweek row (gameweekRows — the same walker loadGameweeks and
	// the census use, not a second implementation of the guard).
	fixtureIDsByPlayerGW := map[int]map[int][]int{}
	if _, err := s.gameweekRows(ctx, func(rec []string, col map[string]int, p *Player, gw int) {
		fid := ival(rec, col, "fixture")
		if fid <= 0 {
			return
		}
		if fixtureIDsByPlayerGW[p.ID] == nil {
			fixtureIDsByPlayerGW[p.ID] = map[int][]int{}
		}
		fixtureIDsByPlayerGW[p.ID][gw] = append(fixtureIDsByPlayerGW[p.ID][gw], fid)
	}); err != nil {
		t.Fatal(err)
	}

	var rowsOut []doubleLegRow
	var candidateRows, phantomCount, notStartedCount, notEstablishedCount int
	for _, id := range sortedPlayerIDs(s) {
		p := s.Players[id]
		byGW := fixtureIDsByPlayerGW[id]
		var gws []int
		for gw := range byGW {
			gws = append(gws, gw)
		}
		sort.Ints(gws)
		for _, gw := range gws {
			ids := byGW[gw]
			if len(ids) != 2 {
				continue
			}
			candidateRows++
			g, ok := p.GWs[gw]
			if !ok || g.Fixtures != 2 {
				t.Fatalf("player %d (%s) gw %d: this diagnostic's own re-walk found 2 fixture "+
					"ids %v but Season.Players records Fixtures=%v — the two walks of the same "+
					"guarded reader disagree", id, p.WebName, gw, ids, g.Fixtures)
			}
			f0, ok0 := fixByID[ids[0]]
			f1, ok1 := fixByID[ids[1]]
			var common int
			if ok0 && ok1 {
				common = commonClub(f0, f1)
			}
			if common == 0 || !clubHasDouble(doubleClubsByGW, gw, common) {
				phantomCount++
				fmt.Printf("PHANTOM candidate dropped: player id=%d (%s) gw=%d fixtures=%v\n",
					id, p.WebName, gw, ids)
				continue
			}
			if g.Starts < 1 {
				notStartedCount++
				continue
			}
			a := agg[id]
			if a == nil || a.starts < defconMinStarts {
				notEstablishedCount++
				continue
			}
			rowsOut = append(rowsOut, doubleLegRow{
				id:       id,
				pos:      p.Type,
				starts:   g.Starts,
				defconP9: a.defconPer90(),
				key:      doubleLegClubKey{gw: gw, club: common},
			})
		}
	}

	fmt.Printf("\n=== coverage funnel (this diagnostic's own population) ===\n")
	fmt.Printf("candidate rows (Fixtures==2, fixture-id pair present):       %5d\n", candidateRows)
	fmt.Printf("  dropped: phantom (fails the fixture cross-check):         %5d\n", phantomCount)
	fmt.Printf("  dropped: Starts == 0 (population requires Starts >= 1):   %5d\n", notStartedCount)
	fmt.Printf("  dropped: season defcon_per_90 not stable (< %d qualifying\n", defconMinStarts)
	fmt.Printf("           single-fixture starts, buildDefconAgg's own basis): %2d\n", notEstablishedCount)
	fmt.Printf("used (the population this file measures):                  %5d\n", len(rowsOut))

	playersInvolved := map[int]bool{}
	for _, r := range rowsOut {
		playersInvolved[r.id] = true
	}
	fmt.Printf("distinct established players in the population:            %5d\n", len(playersInvolved))

	positions := []int{2, 3, 4} // DEF, MID, FWD — GK is out of scope, per the task
	interpret := map[int]bool{2: true, 3: true, 4: false}

	for _, pos := range positions {
		var posRows []doubleLegRow
		rateByID := map[int]float64{}
		for _, r := range rowsOut {
			if r.pos != pos {
				continue
			}
			posRows = append(posRows, r)
			rateByID[r.id] = r.defconP9
		}
		var players []doubleLegPlayerRate
		for id, rate := range rateByID {
			players = append(players, doubleLegPlayerRate{id: id, rate: rate})
		}

		fmt.Printf("\n=== %s: population ===\n", defconPosName[pos])
		fmt.Printf("distinct established players: %d, qualifying rows: %d\n", len(players), len(posRows))
		if !interpret[pos] {
			fmt.Printf("(FWD — printed for completeness only, per the task: the population here is\n")
			fmt.Printf("far too small to interpret. No pre-registered reading is applied below.)\n")
		}
		if len(players) == 0 {
			fmt.Printf("(no qualifying players at this position — nothing to split into terciles)\n")
			continue
		}

		terciles := doubleLegTerciles(players)
		tercileLabel := [3]string{"bottom", "middle", "top"}
		var bothProp [3]float64
		var haveProp [3]bool

		fmt.Printf("\n--- 1/2: defcon terciles ---\n")
		fmt.Printf("%-8s %5s %12s %10s %s\n", "tercile", "nRows", "medDefcon90", "nPlayers", "share started BOTH legs (of Starts>=1)")
		for i, group := range terciles {
			ids := map[int]bool{}
			var rates []float64
			for _, pl := range group {
				ids[pl.id] = true
				rates = append(rates, pl.rate)
			}
			nRows, nBoth := doubleLegGroupStats(posRows, ids)
			med := 0.0
			if len(rates) > 0 {
				med = median(rates)
			}
			fmt.Printf("%-8s %5d %12.3f %10d %s\n", tercileLabel[i], nRows, med, len(group), pctOf(nBoth, nRows))
			if nRows > 0 {
				bothProp[i] = float64(nBoth) / float64(nRows)
				haveProp[i] = true
			}
		}

		fmt.Printf("\n--- 3: per-club-double breakdown (the honesty column) ---\n")
		fmt.Printf("%-10s %5s %s\n", "club-dbl", "nRows", "share started BOTH legs")
		for _, k := range allClubDoubles {
			nRows, nBoth := doubleLegClubStats(posRows, k)
			label := fmt.Sprintf("GW%d/c%d", k.gw, k.club)
			if nRows == 0 {
				fmt.Printf("%-10s %5d %s\n", label, 0, "n/a (no qualifying player from this position)")
				continue
			}
			fmt.Printf("%-10s %5d %s\n", label, nRows, pctOf(nBoth, nRows))
		}

		fmt.Printf("\n--- 4: monotonicity ---\n")
		if haveProp[0] && haveProp[1] && haveProp[2] {
			diffPP := (bothProp[2] - bothProp[0]) * 100
			monotone := bothProp[0] <= bothProp[1] && bothProp[1] <= bothProp[2]
			fmt.Printf("bottom=%.1f%%  middle=%.1f%%  top=%.1f%%  (top minus bottom = %.1f pp, monotone=%v)\n",
				bothProp[0]*100, bothProp[1]*100, bothProp[2]*100, diffPP, monotone)
			if interpret[pos] {
				switch {
				case monotone && diffPP >= 15:
					fmt.Printf("READING: top tercile starts both materially more often (>=15pp) and the\n")
					fmt.Printf("ordering is monotone — a DIRECTION worth re-testing once 2026-27 adds more\n")
					fmt.Printf("club-doubles. NOT a basis for building anything on its own.\n")
				default:
					fmt.Printf("READING: flat, non-monotone, or below the 15pp bar — the line dies here for\n")
					fmt.Printf("%s and should be recorded as dead.\n", defconPosName[pos])
				}
			}
		} else {
			fmt.Printf("one or more terciles has zero qualifying rows — no proportion to compare\n")
		}
	}

	fmt.Printf("\nThis is a descriptive split, explicitly under-powered by design (see header).\n")
	fmt.Printf("It does not resolve either way, and authorises no scoring change, sweep or\n")
	fmt.Printf("replay regardless of what the numbers above show.\n")
}

// TestDoubleLegDefconTercileWiring pins the tercile split and the two group-
// reduction helpers against synthetic data, without DIAG and without the
// archive.
func TestDoubleLegDefconTercileWiring(t *testing.T) {
	// --- doubleLegTerciles: n=14 -> 4,5,5 (the task's own pre-computed FWD
	// figure), ties broken by id ascending ---
	var players []doubleLegPlayerRate
	for i := 1; i <= 14; i++ {
		players = append(players, doubleLegPlayerRate{id: i, rate: float64(i)})
	}
	groups := doubleLegTerciles(players)
	if len(groups[0]) != 4 || len(groups[1]) != 5 || len(groups[2]) != 5 {
		t.Fatalf("group sizes = %d,%d,%d, want 4,5,5", len(groups[0]), len(groups[1]), len(groups[2]))
	}
	if groups[0][0].id != 1 || groups[2][len(groups[2])-1].id != 14 {
		t.Fatalf("terciles not sorted ascending by rate: bottom starts %d, top ends %d",
			groups[0][0].id, groups[2][len(groups[2])-1].id)
	}
	// Ties: two players on the identical rate must break by id ascending, and
	// must not be split unpredictably across group boundaries by sort
	// instability.
	tied := []doubleLegPlayerRate{{id: 3, rate: 1}, {id: 1, rate: 1}, {id: 2, rate: 1}}
	tg := doubleLegTerciles(tied)
	if tg[0][0].id != 1 {
		t.Fatalf("tie-break: bottom tercile's first id = %d, want 1 (ascending id on equal rate)", tg[0][0].id)
	}
	// n=0: no panic, three empty groups.
	empty := doubleLegTerciles(nil)
	if len(empty[0]) != 0 || len(empty[1]) != 0 || len(empty[2]) != 0 {
		t.Fatalf("empty input produced non-empty groups: %v", empty)
	}
	// Input slice must not be mutated (caller reuses it for rateByID elsewhere).
	before := append([]doubleLegPlayerRate(nil), players...)
	doubleLegTerciles(players)
	for i := range players {
		if players[i] != before[i] {
			t.Fatalf("doubleLegTerciles mutated its input at index %d", i)
		}
	}

	// --- doubleLegGroupStats: counts rows by an id set, both-legs by Starts==2 ---
	rows := []doubleLegRow{
		{id: 1, starts: 2, key: doubleLegClubKey{gw: 33, club: 10}},
		{id: 1, starts: 1, key: doubleLegClubKey{gw: 36, club: 10}}, // same player, second club-double
		{id: 2, starts: 1, key: doubleLegClubKey{gw: 33, club: 10}},
		{id: 3, starts: 2, key: doubleLegClubKey{gw: 33, club: 20}},
	}
	nRows, nBoth := doubleLegGroupStats(rows, map[int]bool{1: true, 2: true})
	if nRows != 3 { // player 1 contributes two rows, player 2 one
		t.Fatalf("nRows = %d, want 3 (player 1's two rows plus player 2's one)", nRows)
	}
	if nBoth != 1 { // only the first of player 1's two rows started both legs
		t.Fatalf("nBoth = %d, want 1", nBoth)
	}

	// --- doubleLegClubStats: same reduction, keyed on club-double instead of player set ---
	nRows, nBoth = doubleLegClubStats(rows, doubleLegClubKey{gw: 33, club: 10})
	if nRows != 2 || nBoth != 1 { // rows 1 (player 1, starts 2) and 3 (player 2, starts 1)
		t.Fatalf("GW33/club10: nRows=%d nBoth=%d, want 2,1", nRows, nBoth)
	}

	// --- pctOf ---
	if got, want := pctOf(0, 0), "n/a"; got != want {
		t.Fatalf("pctOf(0,0) = %q, want %q", got, want)
	}
	if got := pctOf(1, 2); got != " 50.0% (1/2)" {
		t.Fatalf("pctOf(1,2) = %q, want %q", got, " 50.0% (1/2)")
	}
}
