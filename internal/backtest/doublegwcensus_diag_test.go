package backtest

// A CENSUS, not a measurement: does 2025-26 carry enough double-gameweek rows
// to make the owner's real question answerable at all?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDoubleGameweekCensus -v -timeout 60m
//
// # The question this feeds
//
// The owner's hypothesis is specific to doubles: in a double gameweek a manager
// deliberately reaches into the rotation pool, hoping for ~1.5 games out of a
// marginal player, and the quantity that matters there is not a few minutes of
// scaling but whether he starts ONE leg or BOTH — a whole extra fixture. Whether
// `defcon_per_90` (the sibling diagnostic's role-proxy candidate) predicts THAT
// binary outcome is a real, different question from the one
// TestDiagDefconMinutesPerStart already answers (minutes-per-start on a single
// fixture). But defensive contribution exists in exactly one season
// (`DefconScoredIn`), so there is no pooling across seasons, and this census
// exists to say cheaply whether 2025-26 alone carries enough double-gameweek
// rows — and enough variation in how many of the two legs a player started — to
// support that measurement before anyone builds it.
//
// # CENSUS ONLY
//
// No correlation, no slope, no model. Raw counts, printed plainly, including
// "not answerable on this data" if that is what they show. Nothing here changes
// a scoring term or a config value.
//
// # ⚠️ The one trap this file exists to not fall into
//
// season.go's rowGuardReport documents ten byte-for-byte duplicated rows in the
// 2025-26 archive, for elements 100 (Kroupi) and 391 (Gannon-Doak): the parser
// used to read `(element, fixture)` as a heuristic rather than a key, and Kroupi
// read `Fixtures == 2` in nine ORDINARY single gameweeks as a result — a
// duplicated single row, not a real second match. `loadGameweeks`'s row guard
// (`rowGuardCount`, `Season.RowGuards`) now drops a repeated `(element,
// fixture)` pair before it can increment `Fixtures`, and `Load` refuses a
// cached file whose `RowGuards.Guards` is below the current count — so a stale
// cache cannot silently reintroduce this. But "the guard should have fixed it"
// is exactly the kind of claim this project's CLAUDE.md says to verify rather
// than assert, so this census cross-checks every `Fixtures == 2` row it counts
// against `fixtures.csv` directly, two independent ways:
//
//  1. It re-walks `gws/merged_gw.csv` itself (through `gameweekRows`, the exact
//     guarded walker `loadGameweeks` uses — not a second implementation of the
//     guard) and records the actual fixture IDs behind each player-gameweek's
//     row count, rather than trusting the accumulated `Fixtures` integer alone.
//     A genuine double's two fixture IDs must share exactly one common club
//     (the two matches that club played); a row pair with no club in common
//     could not be a real double no matter what `Fixtures` says.
//  2. It separately counts, per gameweek, how many distinct clubs `fixtures.csv`
//     itself records playing twice — built ONLY from `Event`/`TeamH`/`TeamA`,
//     never touching a player row — so the club-level double list this census
//     reports in section 1 cannot be contaminated by anything wrong with a
//     player's `Fixtures` field.
//
// Any player-gameweek row whose two fixture IDs share no common club is
// reported individually as a phantom candidate rather than silently dropped,
// and Kroupi and Gannon-Doak are checked by name explicitly.
//
// # Why fixture IDs, not Player.Team, do the cross-check
//
// `Player.Team` is parsed once from `players_raw.csv` and is a snapshot, not a
// week-by-week record — a mid-season transfer would misattribute a player's
// earlier gameweeks to his later club. Deriving the "doubling club" from the
// pair of fixture IDs actually behind a player's two rows sidesteps that
// entirely: it needs no assumption about which club he was on in gameweek N.
import (
	"context"
	"fmt"
	"sort"
	"testing"
)

func TestDiagDoubleGameweekCensus(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	const season = "2025-26"
	fmt.Printf("\n=== season selection ===\n")
	fmt.Printf("DefconScoredIn gates every season before 2025-26 to zero or meaningless\n")
	fmt.Printf("defcon. %s is the only season this project's archive returns true for, so\n", season)
	fmt.Printf("it is also the only season this census can run against.\n")
	if !DefconScoredIn(season) {
		t.Fatalf("DefconScoredIn(%q) = false; this census has nothing to count", season)
	}

	s, err := Load(ctx, cfg.CacheDir, season)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("\n=== row guard status (rules out the Kroupi/Gannon-Doak duplicate rows at the source) ===\n")
	if s.RowGuards == nil || s.RowGuards.Guards < rowGuardCount {
		t.Fatalf("season loaded with RowGuards=%+v: the duplicate-row guard did not run at "+
			"the current version, so any Fixtures==2 row below could be a phantom rather than "+
			"a real double — Load should have refused this cache", s.RowGuards)
	}
	fmt.Printf("RowGuards = %+v\n", *s.RowGuards)
	fmt.Printf("Duplicate rows dropped: %d (season.go's own comment records exactly 10, from\n", s.RowGuards.Duplicate)
	fmt.Printf("Kroupi and Gannon-Doak, as the entire duplicate population in this archive)\n")

	// Independent re-walk of the guarded row stream, capturing the actual fixture
	// ID behind every surviving row rather than trusting the accumulated
	// Fixtures integer. Same walker loadGameweeks uses (gameweekRows), so this is
	// not a second implementation of the guard — only a second thing done with
	// its output.
	fixtureIDsByPlayerGW := map[int]map[int][]int{}
	var totalRows, uncheckableRows int
	if _, err := s.gameweekRows(ctx, func(rec []string, col map[string]int, p *Player, gw int) {
		totalRows++
		fid := ival(rec, col, "fixture")
		if fid <= 0 {
			// Bypasses BOTH row guards (see gameweekRows: both live inside
			// `if fid > 0`) — reported so a nonzero count here is visible rather
			// than silently trusted as "the guard covers everything".
			uncheckableRows++
			return
		}
		if fixtureIDsByPlayerGW[p.ID] == nil {
			fixtureIDsByPlayerGW[p.ID] = map[int][]int{}
		}
		fixtureIDsByPlayerGW[p.ID][gw] = append(fixtureIDsByPlayerGW[p.ID][gw], fid)
	}); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("\nrows surviving the guard: %d, of which %d carry no usable fixture id\n",
		totalRows, uncheckableRows)
	fmt.Printf("(uncheckable rows bypass both guards entirely; 0 here means every surviving\n")
	fmt.Printf("row in this season is fixture-verifiable)\n")

	// Fixture index, for turning a fixture ID into its two clubs and its event.
	fixByID := map[int]struct{ event, teamH, teamA int }{}
	for _, f := range s.Fixtures {
		if f.Event == nil {
			continue
		}
		fixByID[f.ID] = struct{ event, teamH, teamA int }{*f.Event, f.TeamH, f.TeamA}
	}

	// --- Q1/Q2: gameweeks with a genuine club double, from fixtures.csv ALONE ---
	// Built only from Event/TeamH/TeamA — never touches a player row, so this
	// list cannot be contaminated by anything wrong with a player's Fixtures field.
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
	var doubleGWs []int
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
			doubleGWs = append(doubleGWs, gw)
		}
	}
	sort.Ints(doubleGWs)

	fmt.Printf("\n=== Q1/Q2: gameweeks with >=1 club playing twice, from fixtures.csv alone ===\n")
	fmt.Printf("distinct gameweeks: %d\n", len(doubleGWs))
	for _, gw := range doubleGWs {
		fmt.Printf("  GW%-3d  %d club(s): %v\n", gw, len(doubleClubsByGW[gw]), doubleClubsByGW[gw])
	}
	if len(doubleGWs) == 0 {
		fmt.Printf("  (none — the double-gameweek population in 2025-26 is empty; the owner's\n")
		fmt.Printf("   question is not answerable on this data, full stop)\n")
	}

	// --- Q3-Q6: player-gameweek rows with Fixtures==2, cross-checked against
	// the fixture IDs actually behind them ---
	type row struct {
		id, pos, starts int
		phantom         bool
		reason          string
	}
	var raw []row
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
			// Cross-check against the production accumulation: the same guarded
			// walk should have produced the same count on Season.Players.
			if g, ok := p.GWs[gw]; !ok || g.Fixtures != 2 {
				t.Fatalf("player %d (%s) gw %d: this census's own re-walk found 2 fixture "+
					"ids %v but Season.Players records Fixtures=%v — the two walks of the "+
					"same guarded reader disagree", id, p.WebName, gw, ids, g.Fixtures)
			}
			r := row{id: id, pos: p.Type, starts: p.GWs[gw].Starts}
			f0, ok0 := fixByID[ids[0]]
			f1, ok1 := fixByID[ids[1]]
			if !ok0 || !ok1 {
				r.phantom = true
				r.reason = "fixture id not found in fixtures.csv"
			} else {
				common := commonClub(f0, f1)
				if common == 0 {
					r.phantom = true
					r.reason = fmt.Sprintf("no club common to fixtures %d and %d", ids[0], ids[1])
				} else if !clubHasDouble(doubleClubsByGW, gw, common) {
					r.phantom = true
					r.reason = fmt.Sprintf("club %d not in the fixtures.csv double list for GW%d", common, gw)
				}
			}
			if r.phantom {
				fmt.Printf("\nPHANTOM candidate: player id=%d (%s) gw=%d fixtures=%v: %s\n",
					id, p.WebName, gw, ids, r.reason)
			}
			raw = append(raw, r)
		}
	}

	// Kroupi (100) and Gannon-Doak (391), checked by name explicitly per the
	// task's own warning.
	fmt.Printf("\n=== named check: Kroupi (id 100) and Gannon-Doak (id 391) ===\n")
	for _, id := range []int{100, 391} {
		p := s.Players[id]
		if p == nil {
			fmt.Printf("  id %d: not present in this season's roster\n", id)
			continue
		}
		found := false
		for _, r := range raw {
			if r.id == id {
				found = true
				fmt.Printf("  id %d (%s): Fixtures==2 row found, phantom=%v (%s)\n", id, p.WebName, r.phantom, r.reason)
			}
		}
		if !found {
			fmt.Printf("  id %d (%s): no Fixtures==2 row at all — the row guard removed the "+
				"duplicate rows before any Fixtures==2 could be counted\n", id, p.WebName)
		}
	}

	var phantomCount int
	var real []row
	for _, r := range raw {
		if r.phantom {
			phantomCount++
			continue
		}
		real = append(real, r)
	}

	fmt.Printf("\n=== Q3: player-gameweek rows with Fixtures==2 ===\n")
	fmt.Printf("raw (Fixtures==2, before cross-check):  %d\n", len(raw))
	fmt.Printf("phantom (failed the fixture cross-check): %d\n", phantomCount)
	fmt.Printf("real (used for everything below):         %d\n", len(real))

	// --- Q4: Starts distribution over the real population ---
	startsDist := map[int]int{}
	for _, r := range real {
		startsDist[r.starts]++
	}
	fmt.Printf("\n=== Q4: distribution of Starts over real double-gameweek rows ===\n")
	fmt.Printf("Starts==0: %d\nStarts==1: %d\nStarts==2: %d\n",
		startsDist[0], startsDist[1], startsDist[2])
	for k := range startsDist {
		if k < 0 || k > 2 {
			fmt.Printf("Starts==%d (unexpected value): %d\n", k, startsDist[k])
		}
	}

	// --- Q5: same, by position ---
	fmt.Printf("\n=== Q5: Starts distribution by position ===\n")
	positions := []int{1, 2, 3, 4}
	fmt.Printf("%-5s %8s %8s %8s %8s\n", "pos", "n", "starts0", "starts1", "starts2")
	for _, pos := range positions {
		var n, s0, s1, s2 int
		for _, r := range real {
			if r.pos != pos {
				continue
			}
			n++
			switch r.starts {
			case 0:
				s0++
			case 1:
				s1++
			case 2:
				s2++
			}
		}
		fmt.Printf("%-5s %8d %8d %8d %8d\n", defconPosName[pos], n, s0, s1, s2)
	}

	// --- Q6: of the players with a real double-gameweek row, how many have
	// >=10 starts across the season (the population defcon_per_90 was measured
	// on in the sibling diagnostic) ---
	playersInvolved := map[int]bool{}
	for _, r := range real {
		playersInvolved[r.id] = true
	}
	var establishedCount int
	for id := range playersInvolved {
		if s.Players[id].Starts >= 10 {
			establishedCount++
		}
	}
	fmt.Printf("\n=== Q6: established players (season Starts >= 10) among those with a real\n")
	fmt.Printf("double-gameweek row ===\n")
	fmt.Printf("distinct players with a real double-gameweek row: %d\n", len(playersInvolved))
	fmt.Printf("of which >= 10 season starts:                     %d\n", establishedCount)

	fmt.Printf("\nThis census authorises nothing and changes no scoring term. Read section\n")
	fmt.Printf("Q1/Q2 first: if the double-gameweek population itself is a single gameweek or\n")
	fmt.Printf("a handful of clubs, and Q4's Starts==1 count is small, the owner's question is\n")
	fmt.Printf("not answerable on this data — which is the single most likely and most useful\n")
	fmt.Printf("outcome to report plainly.\n")
}

// commonClub returns the club id common to both fixtures, or 0 if there is
// none (which makes the pair NOT a real double for any single club) or more
// than one (two clubs playing each other twice in the same gameweek, which
// this project's archive has never recorded but which this helper reports as
// the first common club found rather than crashing, since a census must not
// panic on an unexpected shape).
func commonClub(a, b struct{ event, teamH, teamA int }) int {
	as := []int{a.teamH, a.teamA}
	bs := []int{b.teamH, b.teamA}
	for _, x := range as {
		for _, y := range bs {
			if x == y {
				return x
			}
		}
	}
	return 0
}

func clubHasDouble(doubleClubsByGW map[int][]int, gw, club int) bool {
	for _, c := range doubleClubsByGW[gw] {
		if c == club {
			return true
		}
	}
	return false
}
