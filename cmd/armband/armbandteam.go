package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// armbandTeamState answers /api/armband-team: what the site's own squad is doing, for
// anyone, always the same document for every requester.
//
// # Why this does not reuse buildState
//
// buildState reads a session -- a reader's own locks, arrangement, saved fifteen -- and
// this page has no reader to have one. Threading an empty session through the interactive
// path would work today, but it would tie a page that must never change per visitor to a
// function whose entire job is to change per visitor; the next field someone adds to
// session handling would silently reach this page too. Calling buildSquadPage directly
// asks for one thing: the page built around a specific fifteen for s.cfg, the config this
// whole binary runs. Nothing here reads a cookie.
//
// # Why Fixed/Arrange are the house account's REAL picks, not the model's own optimum
//
// ⚠️ Corrected 2026-08-21. This used to call buildSquadPage with no Fixed and no Arrange
// at all -- the documented "straight optimum" path -- which draws whichever fifteen the
// model currently rates best and captions it "our team". That is a DIFFERENT fifteen from
// the one config.EntryID's real FPL account actually holds the moment the two disagree,
// which is routine: the model's own opinion moves with every price change and every
// bootstrap refresh, while the real squad only moves on a transfer. Caught by inspection
// of the live page (Enzo Fernández shown in a slot the real account had Szoboszlai), not
// by any test -- nothing exercised this wiring before. houseRealPicks fetches the actual
// fifteen for the most recently closed gameweek and this now runs the same Fixed/Arrange
// path buildState's own reload case does; a fetch failure or an unresolved pick falls
// through to the model's optimum exactly as that path already does for an ordinary
// reader's stale saved team (see buildSquadPage's own comment on sq == nil).
func (s *squadServer) armbandTeamState(w http.ResponseWriter, r *http.Request) {
	defer s.lockRender("armband-team-state")()

	now := s.now()
	event := latestClosedEvent(s.engine.Boot, now)
	fixed, arrange := houseRealPicks(r.Context(), s.client, s.engine.Boot, s.cfg.EntryID, event)

	b, err := buildSquadPage(r.Context(), *s.cfg, s.client, s.engine, pageOpts{
		Weeks:    s.weeks,
		WantPage: true,
		Now:      now,
		Fixed:    fixed,
		Arrange:  arrange,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: armband-team: %v\n", err)
		http.Error(w, "the team could not be built just now — try reloading", http.StatusInternalServerError)
		return
	}

	houseEntry, houseHistory := houseTeamSources(r.Context(), s.client, s.cfg.EntryID)
	houseLive, houseMatchStatus, houseOpponent, resultState := houseLiveSources(r.Context(), s.client, s.engine.Boot, event)

	// event is nil for a season with no closed gameweek yet (see latestClosedEvent) --
	// resultEvent/eventAverage then stay zero, which HouseTeam's own omitempty tags
	// already treat as "nothing to report", the same honest absence buildHouseTeam
	// gives every other figure it has no data for.
	var resultEvent, eventAverage int
	if event != nil {
		resultEvent = event.ID
		eventAverage = event.AverageScore
	}

	st, err := viewmodel.Build(viewmodel.Input{
		Page:              b.Page,
		Boot:              s.engine.Boot,
		Cfg:               *s.cfg,
		Now:               now,
		Chips:             s.cfg.Chips,
		HouseEntry:        houseEntry,
		HouseHistory:      houseHistory,
		HouseLive:         houseLive,
		HouseMatchStatus:  houseMatchStatus,
		HouseOpponent:     houseOpponent,
		HouseMultiplier:   arrange.Mult,
		HouseResultEvent:  resultEvent,
		HouseResultState:  resultState,
		HouseEventAverage: eventAverage,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: armband-team: %v\n", err)
		http.Error(w, "the team could not be built just now — try reloading", http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: armband-team: %v\n", err)
		http.Error(w, "the team could not be built just now — try reloading", http.StatusInternalServerError)
		return
	}
	writeState(w, body)
}

// houseLiveSources fetches the most recently CLOSED gameweek's live per-player stats and
// every club's match status ("scheduled"/"live"/"fulltime"/"finished") for it -- the "is
// this game being played right now" and "did he clear the DC bar" data the team page draws.
//
// # Why the relevant gameweek is found by deadline, not Bootstrap.CurrentEvent
//
// Boot.CurrentEvent (IsCurrent) is FPL's OWN answer to "which gameweek is being played",
// and it is set by FPL's backend on its own schedule, not the instant a deadline passes.
// Observed directly on 2026-08-21: GW1's deadline was 17:30 UTC, and bootstrap-static
// still answered is_current=false, is_next=true at 17:54 -- 24 minutes past deadline with
// kickoff imminent. Waiting on that flag means this page reports no live data for however
// long FPL takes to flip it, which could be the whole first match. A deadline is a fact
// this server already has and needs no such lag: the relevant gameweek is simply the one
// with the LATEST deadline that has already passed.
func latestClosedEvent(boot *fpl.Bootstrap, now time.Time) *fpl.Event {
	var best *fpl.Event
	for i := range boot.Events {
		e := &boot.Events[i]
		if e.DeadlineTime.After(now) {
			continue
		}
		if best == nil || e.DeadlineTime.After(best.DeadlineTime) {
			best = e
		}
	}
	return best
}

// houseLiveSources fetches the current gameweek's live per-player stats, every club's
// match status for it, every club's own fixture in it, and the gameweek's own overall
// state. All come from an always-fresh fetch (fpl.Client.Live, fpl.Client.FixturesLive),
// never the memoised Client.Bootstrap/Client.Fixtures the rest of this process uses -- a
// live score that only updates when the pod restarts is not live. A nil event (a fetch
// failure finding it, or a season with no closed gameweek yet) answers (nil, nil, nil,
// ""): viewmodel.buildHouseTeam then leaves every player's MatchStatus/Opponent/DefCon/
// Saves at their zero value, which is the honest answer to "what happened in a gameweek
// that has not started or does not exist".
//
// Takes the event rather than finding it itself: armbandTeamState finds it once
// (latestClosedEvent) and hands it to both this and houseRealPicks, so the two agree on
// which gameweek's live stats belong beside which gameweek's real fifteen.
//
// The returned state is computed HERE, not left to the client, because it is exactly the
// class of derivation viewmodel.Import's own comment forbids: the fifteen players on the
// pitch may not cover all twenty clubs in a gameweek, so a client inferring "is this
// gameweek over" from their match_status fields alone could reach the wrong answer for a
// club nobody on the squad plays for. The opponent map is computed here for the SAME
// reason Input.HouseOpponent gives for not letting viewmodel fall back to a player's own
// forward-looking Fixtures[0]: only this function already holds ResultEvent's own fixture
// list, fetched fresh, to answer "who did this club actually play".
// fixtureMatchStatus is one fixture's club-facing status: "scheduled", "live",
// "fulltime" or "finished". A pure function of the fixture alone (no fetch, no
// clock), pulled out of houseLiveSources' loop so the three-way split added
// 2026-08-22 is unit-testable without a live client -- see
// TestFixtureMatchStatusThreeWay.
//
// Three states, not two, as of 2026-08-22 -- see fpl.Fixture's own comment on
// Finished vs FinishedProvisional. Verified live 2026-08-22 against
// /api/fixtures/?event=1: five of six fixtures had started, finished AND
// played out to a locked-in score, with Finished still false -- f.Started
// alone (the pre-fix condition) reported all five as "live", which is what
// put an "in progress" dot and a "provisional" asterisk on finished matches
// (see team.js's liveDot and ptsHtml). The window this splits out is real and
// routinely long: FPL sets Finished only once bonus is applied and checked,
// which is exactly the state a reader is most likely to find this page in.
func fixtureMatchStatus(f fpl.Fixture) string {
	switch {
	case f.Finished:
		return "finished" // bonus applied and checked; nothing here moves again
	case f.FinishedProvisional:
		return "fulltime" // played out, locked-in score, bonus not yet confirmed
	case f.Started:
		return "live" // actually being played right now
	}
	return "scheduled"
}

func houseLiveSources(ctx context.Context, client *fpl.Client, boot *fpl.Bootstrap, event *fpl.Event) (*fpl.EventLive, map[string]string, map[string]viewmodel.Fixture, string) {
	if client == nil || boot == nil || event == nil {
		return nil, nil, nil, ""
	}
	live, err := client.Live(ctx, event.ID)
	if err != nil {
		live = nil
	}
	fixtures, err := client.FixturesLive(ctx)
	if err != nil {
		return live, nil, nil, ""
	}
	idToShort := make(map[int]string, len(boot.Teams))
	for _, t := range boot.Teams {
		idToShort[t.ID] = t.ShortName
	}
	status := make(map[string]string, len(boot.Teams))
	opponent := make(map[string]viewmodel.Fixture, len(boot.Teams))
	// kickoff tracks the KickoffTime opponent[club] currently reflects, so a genuine
	// double gameweek (two fixtures for one club inside the same event) keeps the
	// earlier kickoff rather than whichever fixture the API happened to list last --
	// this page draws one opponent chip, not two (see TeamPlayer.Opponent's own
	// comment).
	kickoff := make(map[string]*time.Time, len(boot.Teams))
	setOpponent := func(club string, fx viewmodel.Fixture, at *time.Time) {
		if club == "" {
			return
		}
		if prev, seen := kickoff[club]; seen && prev != nil && (at == nil || at.After(*prev)) {
			return
		}
		opponent[club] = fx
		kickoff[club] = at
	}
	// resultState starts empty (nothing seen yet) and is set to "final" or "live" by
	// the first matching fixture, then only ever downgraded from "final" to "live" --
	// never the other way, so one fixture still in progress is enough to keep the
	// whole gameweek "live". f.Finished (not FinishedProvisional) is the bar, because
	// FPL sets Finished only after bonus is applied -- "final" here means the scores
	// on this page will not move again, matching HouseTeam.ResultState's own contract.
	resultState := ""
	for _, f := range fixtures {
		if f.Event == nil || *f.Event != event.ID {
			continue
		}
		s := fixtureMatchStatus(f)
		home := idToShort[f.TeamH]
		away := idToShort[f.TeamA]
		if home != "" {
			status[home] = s
		}
		if away != "" {
			status[away] = s
		}
		setOpponent(home, viewmodel.Fixture{Gameweek: event.ID, Opponent: away, Home: true, Difficulty: f.TeamHDifficulty}, f.KickoffTime)
		setOpponent(away, viewmodel.Fixture{Gameweek: event.ID, Opponent: home, Home: false, Difficulty: f.TeamADifficulty}, f.KickoffTime)
		if f.Finished {
			if resultState == "" {
				resultState = "final"
			}
		} else {
			resultState = "live"
		}
	}
	return live, status, opponent, resultState
}

// houseRealPicks fetches the house account's actual squad for the most recently closed
// gameweek and translates it into buildSquadPage's Fixed/Arrange inputs -- see
// armbandTeamState's own comment for why the page needs this rather than the model's own
// optimum.
//
// Cached (fpl.Client.Picks), not Uncached: config.EntryID is the operator-configured id
// EntryUncached's own doc comment carves out as the safe case for the ordinary cached
// path, unlike an anonymous visitor's typed-in Team ID -- and a closed gameweek's picks
// never change, so there is no reason to spend a live FPL round trip on every page view.
//
// A nil event, a fetch failure, or picks that fail to translate all answer (nil,
// arrangement{}) -- the same honest fallback picksToFixed documents.
func houseRealPicks(ctx context.Context, client *fpl.Client, boot *fpl.Bootstrap, entryID int, event *fpl.Event) ([]int, arrangement) {
	if client == nil || boot == nil || entryID == 0 || event == nil {
		return nil, arrangement{}
	}
	picks, err := client.Picks(ctx, entryID, event.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: armband-team: fetching picks for entry %d gw%d: %v\n",
			entryID, event.ID, err)
		return nil, arrangement{}
	}
	return picksToFixed(boot, picks)
}

// picksToFixed translates an FPL picks response into a fifteen by permanent code and its
// arrangement -- the same translation importTeam does for a reader's imported squad,
// element id resolved to the code this codebase keys everything on because element ids are
// reassigned every summer.
//
// Kept as its own function, separate from the fetch, so it is testable without an FPL
// client: a fabricated *fpl.EntryPicks against a fixture's bootstrap covers this fully.
//
// A pick that fails to resolve, or a response short of fifteen, answers (nil,
// arrangement{}) rather than a partial squad -- buildSquadPage's own documented behaviour
// for an unresolved Fixed set is to fall back to the model's optimum, which is the right
// answer here too: an honestly-computed squad beats drawing fourteen players and calling
// it a team.
func picksToFixed(boot *fpl.Bootstrap, picks *fpl.EntryPicks) ([]int, arrangement) {
	if boot == nil || picks == nil {
		return nil, arrangement{}
	}
	byElement := make(map[int]int, len(boot.Elements))
	for i := range boot.Elements {
		el := &boot.Elements[i]
		byElement[el.ID] = el.Code
	}
	sorted := append([]fpl.Pick(nil), picks.Picks...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })

	var squad, xi, bench []int
	var captain, vice int
	mult := make(map[int]int, len(sorted))
	for _, p := range sorted {
		code, ok := byElement[p.Element]
		if !ok {
			return nil, arrangement{}
		}
		squad = append(squad, code)
		if p.Position <= 11 {
			xi = append(xi, code)
		} else {
			bench = append(bench, code)
		}
		if p.IsCaptain {
			captain = code
		}
		if p.IsViceCaptain {
			vice = code
		}
		// Keyed by code, same as everything else here -- see arrangement.Mult's own
		// comment for why the results page needs this at all.
		mult[code] = p.Multiplier
	}
	if len(squad) != 15 {
		return nil, arrangement{}
	}
	return squad, arrangement{XI: xi, Bench: bench, Captain: captain, Vice: vice, Mult: mult}
}
