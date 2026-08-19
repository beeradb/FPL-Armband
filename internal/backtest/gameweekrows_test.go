package backtest

import (
	"context"
	"testing"
)

// TestGameweekRowsYieldsOneCallbackPerMatch pins the seam a per-match diagnostic
// reads the archive through.
//
// # Why this needs a test of its own
//
// `loadGameweeks` and `gameweekRows` used to be one function, and splitting them
// created a second consumer of the two row guards. The guards are the most
// expensive thing in this package's history to get wrong — a real double gameweek
// has the identical shape to the archive's duplicate rows, and a guard keyed on
// (element, gameweek) rather than (element, fixture) re-introduces the
// +115-a-season doubles bug while fixing two much smaller ones.
//
// So there are two properties to hold, and they pull in opposite directions:
//
//   - The walk must yield **one callback per surviving match**, because the whole
//     reason it exists is that `GW` has already summed a double's two matches
//     together and three of FPL's channels are per-match step functions. An
//     iterator that accumulated would be indistinguishable from `Player.GWs` and
//     silently useless.
//   - It must apply **exactly the guards `Load` applies**, so a diagnostic and a
//     replay disagree about no row. Asserted by comparing the second walk's report
//     against the one `Load` recorded, rather than against hardcoded counts — a
//     literal would pass while the two walks diverged.
//
// The stub is the row-guard fixture: element 1 has a byte-identical duplicate in
// gameweek 1, a genuine double in gameweek 2, and one leg of that double also
// filed a week late.
func TestGameweekRowsYieldsOneCallbackPerMatch(t *testing.T) {
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
		"fixtures.csv": "id,event,team_h,team_a,team_h_difficulty," +
			"team_a_difficulty,team_h_score,team_a_score,kickoff_time\n" +
			"1,1,1,2,3,3,1,0,2099-08-10T14:00:00Z\n" +
			"2,2,1,2,3,3,1,0,2099-08-17T14:00:00Z\n" +
			"3,2,2,1,3,3,0,0,2099-08-19T14:00:00Z\n",
		"gws/merged_gw.csv": "element,fixture,GW,minutes,total_points,value,starts," +
			"kickoff_time\n" +
			"1,1,1,90,6,60,1,2099-08-10T14:00:00Z\n" +
			"1,1,1,90,6,60,1,2099-08-10T14:00:00Z\n" +
			"1,2,2,90,7,60,1,2099-08-17T14:00:00Z\n" +
			"1,3,2,60,7,60,1,2099-08-19T14:00:00Z\n" +
			"1,3,3,0,0,60,0,2099-08-26T14:00:00Z\n" +
			"2,1,1,90,6,45,1,2099-08-10T14:00:00Z\n",
	})

	s, err := Load(context.Background(), t.TempDir(), "2099-00")
	if err != nil {
		t.Fatal(err)
	}
	if s.RowGuards == nil {
		t.Fatal("Load recorded no row guard report; there is nothing to compare against")
	}

	type seen struct{ gw, minutes int }
	byElement := map[int][]seen{}
	report, err := s.gameweekRows(context.Background(),
		func(rec []string, col map[string]int, p *Player, gw int) {
			byElement[p.ID] = append(byElement[p.ID], seen{gw, ival(rec, col, "minutes")})
		})
	if err != nil {
		t.Fatal(err)
	}

	// One walk, one set of guards.
	if report.Misfiled != s.RowGuards.Misfiled || report.Duplicate != s.RowGuards.Duplicate {
		t.Errorf("the iterator dropped %d misfiled and %d duplicate rows where Load "+
			"recorded %d and %d. The two walks must be one implementation, or a "+
			"diagnostic and a replay disagree about which rows are real.",
			report.Misfiled, report.Duplicate,
			s.RowGuards.Misfiled, s.RowGuards.Duplicate)
	}

	// Per match, not per gameweek. Gameweek 2 must arrive as two callbacks
	// carrying 90 and 60 — a single callback carrying 150 would mean the seam had
	// accumulated, and every per-match figure read through it would be wrong in
	// exactly the way this file exists to prevent.
	got := byElement[1]
	want := []seen{{1, 90}, {2, 90}, {2, 60}}
	if len(got) != len(want) {
		t.Fatalf("element 1 arrived as %d callbacks %v, want %d %v — a double "+
			"gameweek is two matches and must stay two rows here",
			len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("callback %d is %+v, want %+v", i, got[i], want[i])
		}
	}

	// The report is returned rather than assigned, so a second walk over an
	// already-loaded season cannot overwrite what the load recorded.
	if s.RowGuards == report {
		t.Error("gameweekRows handed back the season's own report object; a second " +
			"walk must not be able to mutate what Load recorded")
	}
}
