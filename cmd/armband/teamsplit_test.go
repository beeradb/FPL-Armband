package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// ownerConfig is the fixture's config with every one of the owner's own team
// settings filled in — the file `armband -team team.json` produces in memory.
//
// It is what a reader must inherit NONE of, and what the owner must keep ALL of
// on his own page under -persist. Both tests below start here, so a setting
// added to TeamConfig has one place to be added rather than two.
func ownerConfig(t *testing.T, s *squadServer) config.Config {
	t.Helper()
	team := config.TeamConfig{
		Chips: analysis.ChipSchedule{
			First:  analysis.ChipPlan{Wildcard: 6, FreeHit: 16, BenchBoost: 8, TripleCaptain: 9},
			Second: analysis.ChipPlan{Wildcard: 20, FreeHit: 36, BenchBoost: 38, TripleCaptain: 37},
		},
		HypotheticalBudget: 103.5,
		Criteria:           []string{"Never own more than one Spurs player."},
		Lock: []config.RosterOverride{{
			Code:        s.engine.Boot.Elements[2].Code,
			Name:        s.engine.Boot.Elements[2].WebName,
			Reason:      "the squad is built around him",
			SetOn:       "2026-08-31",
			LastChecked: "2026-08-31",
		}},
		LeadHours: 11,
	}
	return team.ApplyTo(*s.cfg)
}

// TestAReaderInheritsNoneOfTheOwnersTeamSettings is the whole-struct assertion
// the field-by-field version cannot make.
//
// `config.Config.Team()` extracts exactly the settings that belong to one
// manager. Comparing the extraction against the zero TeamConfig means a setting
// ADDED to TeamConfig later is covered by this test the day it is added,
// without anybody remembering to extend a list of field names — which is the
// failure mode the old deny-list version of forPlanner had, and the reason the
// chip plan went unstripped for as long as it did.
func TestAReaderInheritsNoneOfTheOwnersTeamSettings(t *testing.T) {
	s := fixtureServer(t)
	owner := ownerConfig(t, s)

	if reflect.DeepEqual(owner.Team(), config.TeamConfig{}) {
		t.Fatal("the fixture's owner settings are empty, so this test would pass " +
			"against a completely broken forPlanner")
	}

	today := s.now().Format("2006-01-02")
	got := session{Version: sessionVersion}.applyTo(forPlanner(owner), s.engine, today)

	if diff := got.Team(); !reflect.DeepEqual(diff, config.TeamConfig{}) {
		t.Errorf("a reader inherited the owner's own settings: %+v\n\n"+
			"forPlanner is an allow-list: anything a reader may see is copied in "+
			"by name. A setting reaching here was named by mistake.", diff)
	}
}

// TestForPlannerClassifiesEveryConfigField is the guard that makes the
// allow-list an allow-list rather than a comment claiming to be one.
//
// A field added to `config.Config` and named in neither list below fails this
// test, so the classification is FORCED at the moment the field is added.
// Without it, "inverted to an allow-list" degrades over a few releases into "an
// allow-list plus whatever nobody thought about", which is a deny-list again.
//
// The direction of the residual risk is the point of the inversion. Forgetting
// a field here means the planner does not see a setting — visible, and cheap.
// Under the deny-list, forgetting meant a manager's private decision reached
// every visitor, invisibly.
func TestForPlannerClassifiesEveryConfigField(t *testing.T) {
	// Carried verbatim: facts about football, about the deployment, or about
	// this process. See forPlanner for the argument on each group.
	carried := map[string]bool{
		"Weights": true, "Congestion": true, "RoleRisk": true,
		"OptionValue": true,
		"EntryID":     true, "WildcardEnabled": true,
		"ReportDir": true, "CacheDir": true, "CacheMinutes": true,
		"PlayerCacheMinutes": true, "SnapshotDir": true, "XGCExternalDir": true,
		"Model": true, "Effort": true, "MaxIterations": true,
	}
	// Dropped whole: one manager's own settings.
	dropped := map[string]bool{
		"Chips": true, "HypotheticalBudget": true, "Criteria": true,
	}
	// Split: carried in part. Asserted separately below, because "equal" and
	// "zero" are both wrong answers for these two.
	split := map[string]bool{"Review": true, "Roster": true}

	s := fixtureServer(t)
	in := ownerConfig(t, s)
	// Every carried field has to be non-zero in the input, or "it survived" is
	// not something this test actually established.
	in.SnapshotDir = "/archive/current"
	in.XGCExternalDir = "/xgc"
	in.EntryID = 2785902
	in.WildcardEnabled = true
	in.Roster.Teams = []config.TeamOverride{{Team: "ARS", XGCFactor: 1.15, Reason: "x", SetOn: "2026-08-31"}}

	out := forPlanner(in)
	iv, ov := reflect.ValueOf(in), reflect.ValueOf(out)
	typ := iv.Type()

	var unclassified []string
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		switch {
		case carried[name]:
			if iv.Field(i).IsZero() {
				t.Errorf("%s is zero in the fixture, so 'it was carried' asserts "+
					"nothing — give it a value in this test", name)
				continue
			}
			if !reflect.DeepEqual(iv.Field(i).Interface(), ov.Field(i).Interface()) {
				t.Errorf("%s is classified as carried but forPlanner changed it", name)
			}
		case dropped[name]:
			if iv.Field(i).IsZero() {
				t.Errorf("%s is zero in the fixture, so 'it was dropped' asserts "+
					"nothing — give it a value in this test", name)
				continue
			}
			if !ov.Field(i).IsZero() {
				t.Errorf("%s is classified as the owner's and reached the planner: %+v",
					name, ov.Field(i).Interface())
			}
		case split[name]:
			// checked below
		default:
			unclassified = append(unclassified, name)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Errorf("config.Config has fields forPlanner has not classified: %v\n\n"+
			"Decide for each one whether a READER of the site should inherit it, "+
			"then copy it into forPlanner's allow-list and name it here, or leave "+
			"it out and name it here as dropped. A field nobody classifies is how "+
			"the chip plan reached every visitor.", unclassified)
	}

	// Review splits: the thresholds and the free-text rules are a published
	// surface, the lead time is the owner's cron.
	if out.Review.MinGainForTransfer != in.Review.MinGainForTransfer {
		t.Error("the transfer threshold did not reach the planner; page.go reads " +
			"it into the watchlist every reader sees")
	}
	if !reflect.DeepEqual(out.Review.Rules, in.Review.Rules) {
		t.Error("review_policy.rules did not reach the planner. It is rendered " +
			"under \"The rules it is deciding under\", and the template gates that " +
			"whole section on the list being non-empty — clearing it deletes a " +
			"published section and the thresholds shown beside it")
	}
	if out.Review.LeadHours != 0 {
		t.Errorf("the owner's scheduled-run lead time reached the planner: %v",
			out.Review.LeadHours)
	}

	// Roster splits: the team-news research describes the world, a lock is a
	// decision.
	if !reflect.DeepEqual(out.Roster.Minutes, in.Roster.Minutes) {
		t.Error("roster.minutes did not reach the planner; it is the published team news")
	}
	if !reflect.DeepEqual(out.Roster.Exclude, in.Roster.Exclude) {
		t.Error("roster.exclude did not reach the planner")
	}
	if !reflect.DeepEqual(out.Roster.Teams, in.Roster.Teams) {
		t.Error("the club corrections did not reach the planner")
	}
	if len(out.Roster.Lock) != 0 {
		t.Errorf("the owner's locks reached the planner: %+v", out.Roster.Lock)
	}
}

// TestPersistKeepsTheOwnersOwnTeamSettings is the other side of the strip, and
// the regression an over-eager fix introduces.
//
// `serve.go` deliberately skips `forPlanner` under `-persist`: there the page IS
// the owner's, he writes back to his own files from it, and his own settings
// must bind. A previous fix stripped the chip plan inside `chipsInto` instead,
// which runs on every path — so it would have taken his chips off his own page,
// silently, with the leak test still green. That is why this asserts on the
// whole TeamConfig rather than on the chips alone.
func TestPersistKeepsTheOwnersOwnTeamSettings(t *testing.T) {
	s := fixtureServer(t)
	owner := ownerConfig(t, s)

	today := s.now().Format("2006-01-02")
	// -persist passes the config through UNSTRIPPED, exactly as
	// effectiveCfgFrom does.
	got := session{Version: sessionVersion}.applyTo(owner, s.engine, today)

	if !reflect.DeepEqual(got.Team(), owner.Team()) {
		t.Errorf("under -persist the owner lost his own settings\n  got:  %+v\n  want: %+v",
			got.Team(), owner.Team())
	}
}

// TestArmbandTeamStillRenders is the regression a wrong split causes.
//
// `/armband-team` is the spectator page — what the site's own squad is actually
// doing, always, for anyone — and it is built entirely from `config.EntryID`.
// `entry_id` READS as personal and is deliberately public: moving it to the
// team file, which the deployed server is never given, deletes the page. This
// exercises the whole handler rather than the classification, because the
// classification is exactly what a wrong split gets right on paper.
//
// ⚠️ It needs a non-nil client and does NOT need the network. `buildSquadPage`
// reaches `ownedTransferBoard`, which calls `client.Entry` unguarded once
// `entry_id` is set — a nil client panics there. A real client against a
// throwaway cache is enough: every one of these fetches is allowed to fail, and
// the documented fallback for an unresolved squad is the model's own optimum.
// So the assertion holds whether or not FPL is reachable, and this neither
// skips nor flakes on it.
func TestArmbandTeamStillRenders(t *testing.T) {
	s := fixtureServer(t)
	cfg := ownerConfig(t, s)
	cfg.EntryID = 2785902
	s.cfg = &cfg
	s.client = fpl.New(t.TempDir(), time.Hour, time.Hour)

	req := httptest.NewRequest("GET", routeArmbandTeamState, nil)
	req.Host = "127.0.0.1:8080"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d, want 200:\n%s", routeArmbandTeamState,
			w.Code, w.Body.String())
	}
	var st struct {
		Squad struct {
			Players []struct {
				Name string `json:"name"`
			} `json:"players"`
			XI    []int `json:"xi"`
			Bench []int `json:"bench"`
		} `json:"squad"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the house-team document is not JSON: %v", err)
	}
	if len(st.Squad.Players) != 15 {
		t.Errorf("the house team rendered %d players, want 15", len(st.Squad.Players))
	}
	if len(st.Squad.XI) != 11 || len(st.Squad.Bench) != 4 {
		t.Errorf("the house team's pitch is %d + %d, want 11 + 4",
			len(st.Squad.XI), len(st.Squad.Bench))
	}
}
