package backtest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// serveArchive points the loader at a stub archive built from the given files.
//
// Separate from stubArchive, which serves one fixed season and varies the *status*
// of one file. This one varies the *content*, which is what a parser guard has to be
// tested against: the defects here are two rows among twenty-odd thousand, and the
// only honest way to check a guard finds them is to hand it a file where the answer
// is known by construction rather than by having counted the real archive once.
func serveArchive(t *testing.T, files map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if i := strings.Index(path, "/"); i >= 0 {
			path = path[i+1:]
		}
		body, ok := files[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	was := archiveURL
	archiveURL = srv.URL
	t.Cleanup(func() { archiveURL = was })
}

// TestTheRowGuardsDropPhantomMatchesAndKeepRealDoubles is the regression for both
// halves of rowGuardReport, and the second half is the one that matters.
//
// A guard that drops a repeated row is one line, and the reason this test exists is
// that a **real double gameweek looks like the defect**: the same player, the same
// gameweek, two rows, two `Fixtures`. That is the shape `loadGameweeks` was changed
// to accumulate rather than overwrite in the first place, at a recorded cost of
// +115 points a season on POLICY, so a guard keyed on (element, gameweek) instead of
// (element, fixture) would silently re-introduce the largest harness bug in this
// project's record while removing two much smaller ones.
//
// So the stub contains one of each, deliberately adjacent: element 1 plays a genuine
// double in gameweek 2, and one of that double's fixtures is ALSO filed a week late.
// The correct answer is 150 minutes over 2 fixtures in gameweek 2 and no gameweek 3
// at all.
func TestTheRowGuardsDropPhantomMatchesAndKeepRealDoubles(t *testing.T) {
	serveArchive(t, map[string]string{
		"players_raw.csv": "id,code,element_type,team,web_name,minutes,starts," +
			"total_points,now_cost,status,news_added\n" +
			"1,1001,3,1,Real,240,3,20,60,a,\n" +
			"2,1002,2,2,Other,90,1,6,45,a,\n",
		"teams.csv": "id,name,short_name,strength,strength_overall_home," +
			"strength_overall_away,strength_attack_home,strength_attack_away," +
			"strength_defence_home,strength_defence_away\n" +
			"1,Stub City,STU,3,1100,1100,1100,1100,1100,1100\n" +
			"2,Stub Town,STW,3,1100,1100,1100,1100,1100,1100\n",
		// Fixture 3 is filed under event 2. The gameweek row below claims 3.
		"fixtures.csv": "id,event,team_h,team_a,team_h_difficulty," +
			"team_a_difficulty,team_h_score,team_a_score,kickoff_time\n" +
			"1,1,1,2,3,3,1,0,2099-08-10T14:00:00Z\n" +
			"2,2,1,2,3,3,1,0,2099-08-17T14:00:00Z\n" +
			"3,2,2,1,3,3,0,0,2099-08-19T14:00:00Z\n",
		"gws/merged_gw.csv": "element,fixture,GW,minutes,total_points,value,starts," +
			"kickoff_time\n" +
			"1,1,1,90,6,60,1,2099-08-10T14:00:00Z\n" +
			// Byte-identical repeat of the row above: the 2025-26 defect.
			"1,1,1,90,6,60,1,2099-08-10T14:00:00Z\n" +
			// A genuine double: two fixtures, one gameweek, and it must survive.
			"1,2,2,90,7,60,1,2099-08-17T14:00:00Z\n" +
			"1,3,2,60,7,60,1,2099-08-19T14:00:00Z\n" +
			// Fixture 3 again, filed a week late with nothing in it: the 2019-20
			// defect. Dropping it is the whole point; keeping it would give the
			// player a phantom third gameweek and halve his GW3 minutes rate.
			"1,3,3,0,0,60,0,2099-08-26T14:00:00Z\n" +
			"2,1,1,90,6,45,1,2099-08-10T14:00:00Z\n",
	})

	s, err := Load(context.Background(), t.TempDir(), "2099-00")
	if err != nil {
		t.Fatal(err)
	}
	if s.RowGuards == nil {
		t.Fatal("no row guard report, so Load would treat its own output as a stale cache")
	}
	if s.RowGuards.Misfiled != 1 || s.RowGuards.Duplicate != 1 {
		t.Errorf("guards report %d misfiled and %d duplicate, want 1 and 1",
			s.RowGuards.Misfiled, s.RowGuards.Duplicate)
	}

	p := s.Players[1]
	if p == nil {
		t.Fatal("element 1 did not load")
	}
	for _, tc := range []struct {
		gw, fixtures, minutes int
		why                   string
	}{
		{1, 1, 90, "one match played twice in the file is still one match"},
		{2, 2, 150, "a real double gameweek, which the guard must not touch"},
		{3, 0, 0, "the misfiled row's week, which never happened"},
	} {
		g := p.GWs[tc.gw]
		if g.Fixtures != tc.fixtures || g.Minutes != tc.minutes {
			t.Errorf("GW%d: %d fixtures %d minutes, want %d and %d — %s",
				tc.gw, g.Fixtures, g.Minutes, tc.fixtures, tc.minutes, tc.why)
		}
	}
}

// TestOnlyTheCalendarGuardIsInertWithoutAFixtureList pins the degradation, and the
// two guards degrade *differently* — which is the point of the test.
//
// 2016-17 and 2017-18 publish no fixtures.csv. Guard A compares a row's gameweek
// against the calendar, so with no calendar it has nothing to check and must go quiet:
// the wrong answer is to read "not in the list" as "misfiled" and drop every row in
// the season, which would be perfectly silent — a prior with no minutes reads as a
// player who never played, the shape of the starts defect this package already carries
// a reconstruction for.
//
// **Guard B needs no calendar at all.** "A player cannot appear twice in one fixture"
// is self-contradictory on its own, so a duplicate is detectable in a season with no
// fixtures.csv and must still be caught. An earlier version gated both guards behind
// the calendar and so switched duplicate detection off for those two seasons for a
// reason that applies only to the other guard.
func TestOnlyTheCalendarGuardIsInertWithoutAFixtureList(t *testing.T) {
	serveArchive(t, map[string]string{
		"players_raw.csv": "id,code,element_type,team,web_name,minutes,starts," +
			"total_points,now_cost,status,news_added\n" +
			"1,1001,3,1,Real,180,2,12,60,a,\n",
		"gws/merged_gw.csv": "element,fixture,GW,minutes,total_points,value,starts," +
			"kickoff_time\n" +
			"1,1,1,90,6,60,1,2099-08-10T14:00:00Z\n" +
			// A repeat, with no calendar to check it against.
			"1,1,1,90,6,60,1,2099-08-10T14:00:00Z\n" +
			"1,2,2,90,6,60,1,2099-08-17T14:00:00Z\n",
	})

	s, err := Load(context.Background(), t.TempDir(), "2099-01")
	if err != nil {
		t.Fatal(err)
	}
	if !s.PriorOnly() {
		t.Fatalf("want prior-only with no teams.csv or fixtures.csv, absent = %v", s.Absent)
	}
	if s.RowGuards.Misfiled != 0 {
		t.Errorf("the calendar guard dropped %d rows on a season with no calendar; it "+
			"must be inert, or every row in 2016-17 and 2017-18 goes and the seasons "+
			"read as football nobody played", s.RowGuards.Misfiled)
	}
	if s.RowGuards.Duplicate != 1 {
		t.Errorf("the duplicate guard caught %d rows, want 1 — it does not need a "+
			"fixture list to know a player cannot play the same fixture twice",
			s.RowGuards.Duplicate)
	}
	if m := s.Players[1].GWs[1].Minutes + s.Players[1].GWs[2].Minutes; m != 180 {
		t.Errorf("%d minutes survived the guards, want 180", m)
	}
}

// TestDiagWeeklyRowsReconcileWithSeasonTotals is the invariant behind the duplicate
// guard, and it is worth more than the guard is.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagWeeklyRowsReconcile -v
//
// `merged_gw.csv` and `players_raw.csv` are two views of one season, and FPL derives
// the second from the first, so a player's gameweek rows must sum to his season
// totals. That makes this a **free detector for any row-level defect at all** —
// duplicated, missing or misfiled — rather than a check on the two defects that
// happen to be known. The 2025-26 duplicates were originally found this way: they were
// the entire drift in that season, +163 minutes, +27 points, +4 goals and +4 bonus,
// and closing them took it to exactly zero.
//
// This project's own record is that invariants beat reviewers decisively for this
// class of failure, and the reason is visible here: nobody has to remember what a
// phantom row does to `Fixtures`, because the sum simply stops matching.
//
// # Two residuals are tolerated, and they are named rather than absorbed
//
// 2018-19 is short by **3 minutes** and 2024-25 by **17 minutes and 1 point**, out of
// roughly 748,000 minutes a season — about 0.002%. Both predate these guards and
// neither is understood; they are left as a recorded oddity rather than chased.
//
// They are pinned **by season and to the exact figure** rather than admitted by a
// blanket tolerance, which matters more than it looks: a tolerance wide enough to
// admit 17 minutes is wide enough to hide a duplicated appearance, and it would let a
// clean season quietly acquire a residual without failing. Every season not named
// above must reconcile **exactly**. **Widening this is the wrong response to a
// failure** — the right response is to find the rows.
func TestDiagWeeklyRowsReconcileWithSeasonTotals(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)

	// Named residuals, so a season quietly acquiring one fails rather than passing
	// under a blanket tolerance. Anything not listed here must reconcile exactly.
	knownShort := map[string]struct{ minutes, points int }{
		"2018-19": {3, 0},
		"2024-25": {17, 1},
	}

	t.Log("season    minutes drift  points drift  goals  bonus")
	for _, name := range []string{
		"2018-19", "2019-20", "2020-21", "2021-22",
		"2022-23", "2023-24", "2024-25", "2025-26",
	} {
		s := loadSeason(t, cfg, name)
		var wkMin, wkPts, wkGoals, wkBonus int
		var agMin, agPts, agGoals, agBonus int
		for _, id := range sortedSeasonPlayerIDs(s) {
			p := s.Players[id]
			agMin += p.Minutes
			agPts += p.TotalPoints
			agGoals += p.Goals
			agBonus += p.Bonus
			for _, g := range p.GWs {
				wkMin += g.Minutes
				wkPts += g.Points
				wkGoals += g.Goals
				wkBonus += g.Bonus
			}
		}
		dMin, dPts := wkMin-agMin, wkPts-agPts
		dGoals, dBonus := wkGoals-agGoals, wkBonus-agBonus
		t.Logf("%-9s %13d  %12d  %5d  %5d", name, dMin, dPts, dGoals, dBonus)

		want := knownShort[name]
		if dMin != -want.minutes || dPts != -want.points || dGoals != 0 || dBonus != 0 {
			t.Errorf("%s: weekly rows minus season totals is %d minutes, %d points, "+
				"%d goals, %d bonus; want %d, %d, 0, 0. A positive drift is rows "+
				"counted twice and a negative one is rows missing — find them rather "+
				"than widening this test",
				name, dMin, dPts, dGoals, dBonus, -want.minutes, -want.points)
		}
	}
}

// TestDiagArchiveRowGuards is the same guards against the real archive, and it
// records the counts rather than only checking they are small.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagArchiveRowGuards -v
//
// DIAG-gated because it fetches every season the grid names, on the same terms as
// every other archive-reading check here.
//
// The numbers are asserted exactly, which is the point. A guard reporting "some rows
// dropped" is a guard nobody can audit; these are the counts the 2026-08-12 column
// audit found by reading the CSVs directly, so a disagreement means either the
// archive was re-published or the guard has started eating something it should not.
// Either is worth a failing test rather than a quietly different replay.
//
// ⚠️ **It parses into a temporary cache directory, and that is the whole difference
// between this test and a decoration.** `RowGuards` is serialised, so a run against
// the shared cache compares `want` against the counts recorded at some *earlier*
// parse — it would keep passing on stale numbers even if the archive were
// re-published with the rows corrected, which is precisely the event the paragraph
// above claims it catches. An empty cache costs a re-download of eight seasons, about
// eight seconds, and buys the property the test is named for.
func TestDiagArchiveRowGuards(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	cfg.CacheDir = t.TempDir()

	// The audit's counts. Every season the grid can reach, so a zero here is a
	// measured zero rather than a season nobody looked at.
	want := map[string]rowGuardReport{
		"2018-19": {},
		"2019-20": {Misfiled: 59},
		"2020-21": {},
		"2021-22": {},
		"2022-23": {},
		"2023-24": {},
		"2024-25": {},
		"2025-26": {Duplicate: 10},
	}
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)

	t.Log("season    misfiled  duplicate")
	for _, name := range names {
		// Load rather than loadSeason, and into the temporary cache above: the
		// process-global map in loadSeason would hand back a season parsed by
		// whichever test got there first, and the on-disk cache would hand back
		// counts from an earlier parse. The report has to be about THIS parse of
		// the CSVs or it is not an archive check.
		s, err := Load(t.Context(), cfg.CacheDir, name)
		if err != nil {
			t.Fatal(err)
		}
		got := *s.RowGuards
		t.Logf("%-9s %8d  %9d", name, got.Misfiled, got.Duplicate)
		// The counts only. Guards is a schema version rather than a measurement,
		// and folding it into the comparison would make every entry in the table
		// above need editing whenever a guard is added.
		if got.Misfiled != want[name].Misfiled || got.Duplicate != want[name].Duplicate {
			t.Errorf("%s: guards dropped %d misfiled and %d duplicate, want %d and %d",
				name, got.Misfiled, got.Duplicate,
				want[name].Misfiled, want[name].Duplicate)
		}
		if got.Guards != rowGuardCount {
			t.Errorf("%s: parsed by a parser reporting %d guards, want %d — a cached "+
				"season from an older parser would drop fewer rows than this one does",
				name, got.Guards, rowGuardCount)
		}
	}
}
