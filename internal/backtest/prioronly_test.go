package backtest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"armband/internal/fpl"
)

// stubArchive serves a minimal archive, with a chosen status for one file.
//
// It exists because the property that makes prior-only loading safe — a **404** means
// the season does not publish a file, and any other failure is a hard error — cannot be
// exercised against the real archive, which serves neither on demand. The alternative
// is to trust a branch nobody has ever run, and the branch in question is the one that
// decides whether a transient network fault silently removes a season from a grid.
func stubArchive(t *testing.T, brokenFile string, status int) {
	t.Helper()
	files := map[string]string{
		// One player, one gameweek. Enough for Load's schema checks: players present,
		// a status so hasAvailability passes, and minutes so reconstructStarts gives
		// hasStarts something to find.
		"players_raw.csv": "id,code,element_type,team,web_name,minutes,starts," +
			"total_points,now_cost,status,news_added\n" +
			"1,461358,4,1,Stub,90,1,6,100,a,\n",
		"gws/merged_gw.csv": "element,GW,minutes,total_points,value,starts," +
			"kickoff_time\n1,1,90,6,100,1,2099-08-10T14:00:00Z\n",
		"teams.csv": "id,name,short_name,strength,strength_overall_home," +
			"strength_overall_away,strength_attack_home,strength_attack_away," +
			"strength_defence_home,strength_defence_away\n" +
			"1,Stub City,STU,3,1100,1100,1100,1100,1100,1100\n",
		"fixtures.csv": "id,event,team_h,team_a,team_h_difficulty," +
			"team_a_difficulty,team_h_score,team_a_score,kickoff_time\n" +
			"1,1,1,1,3,3,1,0,2099-08-10T14:00:00Z\n",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /<season>/<file>, with the season segment stripped.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if i := strings.Index(path, "/"); i >= 0 {
			path = path[i+1:]
		}
		if path == brokenFile {
			w.WriteHeader(status)
			return
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

// TestAMissingTeamsFileLoadsAsPriorOnly is the load half of the feature.
//
// `PreSeason` reads only season aggregates from the prior and takes `Teams` from the
// *current* season, so teams.csv and fixtures.csv are needed to **play** a season and
// not to **be** one. 2018-19 publishes players, gameweeks and fixtures and no clubs,
// and `Load` refused it outright — which was the whole blocker on extending the prior
// axis, not any reconstruction.
func TestAMissingTeamsFileLoadsAsPriorOnly(t *testing.T) {
	for _, tc := range []struct {
		name, broken string
		wantAbsent   []string
	}{
		{"no teams.csv", "teams.csv", []string{"teams.csv"}},
		{"no fixtures.csv", "fixtures.csv", []string{"fixtures.csv"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubArchive(t, tc.broken, http.StatusNotFound)
			s, err := Load(t.Context(), t.TempDir(), "2099-00")
			if err != nil {
				t.Fatalf("Load refused a season missing only %s: %v", tc.broken, err)
			}
			if got := strings.Join(s.Absent, ","); got != strings.Join(tc.wantAbsent, ",") {
				t.Errorf("Absent is %v, want %v", s.Absent, tc.wantAbsent)
			}
			if !s.PriorOnly() {
				t.Error("PriorOnly() is false for a season missing a file it needs " +
					"to be played")
			}
			if err := s.PlayableAsCurrent(); err == nil {
				t.Error("PlayableAsCurrent() accepted a prior-only season")
			} else if !strings.Contains(err.Error(), tc.broken) {
				t.Errorf("the refusal does not name the missing file: %v", err)
			}
			// The players are the point: a prior-only season is a complete record of
			// what footballers did, and that is what a prior consumes.
			if len(s.Players) == 0 {
				t.Error("no players parsed, so the season is useless as a prior too")
			}
		})
	}
}

// TestAServerErrorIsNotAMissingFile is the half that stops this becoming a silent
// degradation.
//
// A 404 is the server saying the file does not exist. A 500, a timeout or a truncated
// body is something going wrong, and treating any of them as absence would let a
// transient fault turn a playable season into a prior-only one — removing it from every
// grid it appears in while every number still printed. That is this package's signature
// failure and it is the reason errNoSuchFile is a 404 and nothing else.
func TestAServerErrorIsNotAMissingFile(t *testing.T) {
	for _, status := range []int{
		http.StatusInternalServerError, http.StatusForbidden,
		http.StatusTooManyRequests, http.StatusBadGateway,
	} {
		stubArchive(t, "teams.csv", status)
		if _, err := Load(t.Context(), t.TempDir(), "2099-00"); err == nil {
			t.Errorf("status %d loaded successfully; only 404 may mean the archive "+
				"does not publish a file", status)
		}
	}
}

// TestAbsentIsCheckedAgainstWhatWasParsed pins the marker to the data.
//
// The marker must not be a field somebody forgot to set. Both directions matter and
// only one is obvious: a season claiming teams.csv is absent must have no teams, and a
// season with no teams must say so. The second is the schema check — a cached file
// written by a parser that could not read teams.csv, or one truncated by an interrupted
// write, would otherwise read as a legitimate prior-only season.
func TestAbsentIsCheckedAgainstWhatWasParsed(t *testing.T) {
	full := func() *Season {
		return &Season{Name: "2099-00", Players: map[int]*Player{1: {ID: 1}},
			Teams: []fpl.Team{{ID: 1}}, Fixtures: []fpl.Fixture{{ID: 1}}}
	}
	for _, tc := range []struct {
		name string
		mut  func(*Season)
		want bool
	}{
		{"complete and silent", func(*Season) {}, true},
		{"teams absent and declared", func(s *Season) {
			s.Teams, s.Absent = nil, []string{"teams.csv"}
		}, true},
		{"fixtures absent and declared", func(s *Season) {
			s.Fixtures, s.Absent = nil, []string{"fixtures.csv"}
		}, true},
		{"both absent and declared", func(s *Season) {
			s.Teams, s.Fixtures = nil, nil
			s.Absent = []string{"fixtures.csv", "teams.csv"}
		}, true},
		// The stale-cache case, and the one worth having a test for.
		{"teams absent and NOT declared", func(s *Season) { s.Teams = nil }, false},
		{"fixtures absent and NOT declared", func(s *Season) { s.Fixtures = nil }, false},
		{"declared absent but present anyway", func(s *Season) {
			s.Absent = []string{"teams.csv"}
		}, false},
		{"an unrecognised filename", func(s *Season) {
			s.Absent = []string{"understat.csv"}
		}, false},
	} {
		s := full()
		tc.mut(s)
		if got := s.absentIsConsistent(); got != tc.want {
			t.Errorf("%s: absentIsConsistent()=%v, want %v (Absent=%v, %d teams, "+
				"%d fixtures)", tc.name, got, tc.want, s.Absent,
				len(s.Teams), len(s.Fixtures))
		}
	}
}

// TestPlayingAPriorOnlySeasonFailsLoudly is the guard, at both kinds of entry point.
//
// `Simulate` has an error channel and must use it. `PreSeason` and `PointInTime` do not,
// and they are the choke point every playing path passes through — Hold, HoldWeekly and
// HoldCaptaincyWeekly all reach one of them — so they panic. Checking only in Simulate
// would leave those three open, and a prior-only season played would produce a squad
// with no clubs and no fixtures: a replay that is entirely plausible and entirely
// meaningless.
func TestPlayingAPriorOnlySeasonFailsLoudly(t *testing.T) {
	priorOnly := &Season{Name: "2018-19", Absent: []string{"teams.csv"},
		Players: map[int]*Player{1: {ID: 1, Code: 461358, Minutes: 900, GWs: map[int]GW{}}}}
	ok := &Season{Name: "2019-20", Teams: []fpl.Team{{ID: 1}},
		Players: map[int]*Player{1: {ID: 1, Code: 461358, GWs: map[int]GW{}}}}

	if _, err := Simulate(priorOnly, ok, SimConfig{}); err == nil {
		t.Error("Simulate played a prior-only season")
	} else if !strings.Contains(err.Error(), "prior-only") {
		t.Errorf("Simulate's refusal does not say why: %v", err)
	}

	// A prior-only season as the PRIOR must be fine — that is the entire point of the
	// distinction, and a guard that refused it would make the feature pointless.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("PreSeason panicked with a prior-only season as the PRIOR "+
					"(%v); it reads only season aggregates from the prior, which is "+
					"why such a season is usable at all", r)
			}
		}()
		PreSeason(ok, priorOnly)
	}()

	for _, tc := range []struct {
		name string
		call func()
	}{
		{"PreSeason", func() { PreSeason(priorOnly, ok) }},
		{"PointInTime pre-season", func() { PointInTime(priorOnly, ok, 0) }},
		{"PointInTime mid-season", func() { PointInTime(priorOnly, ok, 5) }},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("%s accepted a prior-only season as the season being "+
						"played", tc.name)
					return
				}
				if msg, _ := r.(string); !strings.Contains(msg, "prior-only") {
					t.Errorf("%s panicked without saying why: %v", tc.name, r)
				}
			}()
			tc.call()
		}()
	}
}
