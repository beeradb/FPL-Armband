package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
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

// leakedTeamStrings names the owner-only content ownerConfig fills in, that must
// never appear verbatim in a document served to every visitor. A raw substring
// search rather than a field-by-field decode on purpose: the point of this pair
// of tests is to catch a FUTURE leak through either route without having to
// predict which field of the response would carry it.
func leakedTeamStrings(t *testing.T, cfg config.Config) []string {
	t.Helper()
	if len(cfg.Roster.Lock) == 0 || cfg.Roster.Lock[0].Reason == "" {
		t.Fatal("ownerConfig's fixture has no lock reason to search for")
	}
	if len(cfg.Criteria) == 0 || cfg.Criteria[0] == "" {
		t.Fatal("ownerConfig's fixture has no criteria to search for")
	}
	return []string{cfg.Roster.Lock[0].Reason, cfg.Criteria[0]}
}

// TestArmbandTeamDoesNotLeakTeamSettings pins the second of the two routes
// PR #163's own commit message flagged as bypassing forPlanner entirely --
// armbandTeamState used to build the page from *s.cfg directly. Unlike
// TestArmbandTeamStillRenders above, this asserts on the DOCUMENT rather than
// on the fact that one still comes back: the owner's roster lock and chip
// plan must not reach this public, unauthenticated, same-for-everyone route,
// even when a team.json happens to be loaded into this process.
func TestArmbandTeamDoesNotLeakTeamSettings(t *testing.T) {
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
		t.Fatalf("GET %s answered %d, want 200:\n%s", routeArmbandTeamState, w.Code, w.Body.String())
	}

	body := w.Body.String()
	for _, leak := range leakedTeamStrings(t, cfg) {
		if strings.Contains(body, leak) {
			t.Errorf("the owner's team.json setting reached the public armband-team "+
				"document: %q\nbody: %.2000s", leak, body)
		}
	}

	var st struct {
		Overrides struct {
			Live []struct {
				Kind string `json:"kind"`
			} `json:"live"`
		} `json:"overrides"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatalf("the response did not decode: %v", err)
	}
	for _, ov := range st.Overrides.Live {
		if ov.Kind == "lock" || ov.Kind == "lockXI" {
			t.Errorf("the owner's roster lock reached the public armband-team "+
				"document as an override: %+v", ov)
		}
	}
}

// TestChipTeamsDoesNotLeakTeamSettings is TestChipTeamsDoesNotPublishOurOwnChipPlan's
// sibling, run against the whole document rather than two named fields:
// apiChipTeams used to build from *s.cfg directly, and this pins that no
// content sourced from ownerConfig's five owner-only settings reaches the
// public wildcard/free-hit document, however future code gets there.
//
// This does NOT cover every channel a raw cfg opens on this route. With
// WantPage: false, buildSquadPage returns before pageOverrides ever runs
// (see its own "if not wantPage" branch), so b.Page.Overrides is always nil
// here and a lock's reason text can never reach a player's card through it,
// unlike on /api/armband-team, where WantPage is true. What a raw cfg does
// still change here is which players applyRoster forces into the rebuild
// (LockIDs/StartIDs, see below): a real but silent effect on selection, not
// a text leak. TestChipTeamsNeverForcesInTheOwnersLockedPlayer pins
// that one directly, rather than through an HTTP round trip that cannot
// tell "the model chose them" from "the lock forced them" apart.
func TestChipTeamsDoesNotLeakTeamSettings(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true
	cfg := ownerConfig(t, s)
	s.cfg = &cfg
	// Past GW1's own deadline, so nextOpenEvent lands on GW2 -- the fixture's
	// bootstrap opens the wildcard and free hit there (see
	// TestChipTeamsGoodRequestAnswersTheExpectedShape's own comment). At GW1
	// neither chip is allowed, wc/fh stay nil, and no squad is ever rebuilt --
	// which would make this test pass for a reason that has nothing to do
	// with the leak it exists to catch.
	s.clock = func() time.Time { return s.engine.Boot.Events[0].DeadlineTime.Add(time.Hour) }

	w := getChipTeams(t, s, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s answered %d, want 200:\n%s", routeWildcardState, w.Code, w.Body.String())
	}

	body := w.Body.String()
	for _, leak := range leakedTeamStrings(t, cfg) {
		if strings.Contains(body, leak) {
			t.Errorf("the owner's team.json setting reached the public wildcard "+
				"document: %q\nbody: %.2000s", leak, body)
		}
	}
}

// chipTeamsPlayerIDs walks GET /api/wildcard's decoded document generically
// (map[string]any, not a struct), collecting chip_teams.wildcard/free_hit.xi/bench[].id.
func chipTeamsPlayerIDs(t *testing.T, w *httptest.ResponseRecorder) map[int]bool {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("the response did not decode: %v", err)
	}
	ct, _ := doc["chip_teams"].(map[string]any)
	if ct == nil {
		t.Fatal("chip_teams is missing from the response")
	}
	ids := map[int]bool{}
	for _, key := range []string{"wildcard", "free_hit"} {
		team, _ := ct[key].(map[string]any)
		if team == nil {
			t.Fatalf("chip_teams.%s did not rebuild, and both must for this test to say anything", key)
		}
		for _, side := range []string{"xi", "bench"} {
			players, _ := team[side].([]any)
			for _, raw := range players {
				p, _ := raw.(map[string]any)
				id, _ := p["id"].(float64)
				ids[int(id)] = true
			}
		}
	}
	return ids
}

// TestChipTeamsNeverForcesInTheOwnersLockedPlayer is the deterministic pin
// TestChipTeamsDoesNotLeakTeamSettings's own comment promises: a string
// search cannot tell the model choosing a player apart from the owner's
// lock forcing him in, because neither the lock's reason nor its existence
// is rendered as text on this route. What CAN be told apart is squad
// membership: before apiChipTeams built from forPlanner(*s.cfg), an
// owner's lock silently added a player to every visitor's public
// wildcard/free-hit rebuild who the model would not otherwise have picked.
//
// A baseline (unlocked) build names a candidate the model left out on its
// own, so a pass here cannot be explained by the model wanting him anyway.
func TestChipTeamsNeverForcesInTheOwnersLockedPlayer(t *testing.T) {
	s := fixtureServer(t)
	s.wildcardEnabled = true
	s.clock = func() time.Time { return s.engine.Boot.Events[0].DeadlineTime.Add(time.Hour) }

	baseline := chipTeamsPlayerIDs(t, getChipTeams(t, s, nil))

	var lockID, lockCode int
	for _, m := range s.engine.AllMetrics() {
		if baseline[m.ID] {
			continue
		}
		if m.Position != "MID" || m.Minutes < 1500 || m.Price >= 7.0 {
			continue
		}
		if el := s.engine.Boot.ElementByID(m.ID); el != nil {
			lockID, lockCode = m.ID, el.Code
			break
		}
	}
	if lockID == 0 {
		t.Skip("no suitable not-picked lock candidate in this fixture")
	}

	locked := *s.cfg
	locked.Roster.Lock = []config.RosterOverride{{Code: lockCode, Name: "lock-candidate", Reason: "test"}}
	s.cfg = &locked
	s.chips = nil

	got := chipTeamsPlayerIDs(t, getChipTeams(t, s, nil))
	if !got[lockID] {
		return
	}
	t.Errorf("locking element %d (code %d) forced him into the public wildcard/free-hit "+
		"rebuild, though the unlocked baseline never picked him", lockID, lockCode)
}
