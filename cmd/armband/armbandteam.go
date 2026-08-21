package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
// with no Fixed, no Arrange and Seed 0 is the documented "straight optimum" path
// (see pageOpts and the /api/state handler's own comment on a session-less GET) and asks
// for exactly one thing: the model's honest best for s.cfg, the config this whole binary
// runs. Nothing here reads a cookie.
func (s *squadServer) armbandTeamState(w http.ResponseWriter, r *http.Request) {
	defer s.lockRender("armband-team-state")()

	b, err := buildSquadPage(r.Context(), *s.cfg, s.client, s.engine, pageOpts{
		Weeks:    s.weeks,
		WantPage: true,
		Now:      s.now(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: armband-team: %v\n", err)
		http.Error(w, "the team could not be built just now — try reloading", http.StatusInternalServerError)
		return
	}

	now := s.now()
	houseEntry, houseHistory := houseTeamSources(r.Context(), s.client, s.cfg.EntryID)
	houseLive, houseMatchStatus := houseLiveSources(r.Context(), s.client, s.engine.Boot, now)
	st, err := viewmodel.Build(viewmodel.Input{
		Page:             b.Page,
		Boot:             s.engine.Boot,
		Cfg:              *s.cfg,
		Now:              now,
		Chips:            s.cfg.Chips,
		HouseEntry:       houseEntry,
		HouseHistory:     houseHistory,
		HouseLive:        houseLive,
		HouseMatchStatus: houseMatchStatus,
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
// every club's match status ("scheduled"/"live"/"finished") for it -- the "is this game
// being played right now" and "did he clear the DC bar" data the team page draws.
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

// houseLiveSources fetches the current gameweek's live per-player stats and every club's
// match status for it. Both come from an always-fresh fetch (fpl.Client.Live,
// fpl.Client.FixturesLive), never the memoised Client.Bootstrap/Client.Fixtures the rest
// of this process uses -- a live score that only updates when the pod restarts is not
// live. A fetch failure or a season with no closed gameweek yet (pre-season) answers
// (nil, nil): viewmodel.buildHouseTeam then leaves every player's
// MatchStatus/DefCon/Saves at their zero value, which is the honest answer to "what
// happened in a gameweek that has not started or does not exist".
func houseLiveSources(ctx context.Context, client *fpl.Client, boot *fpl.Bootstrap, now time.Time) (*fpl.EventLive, map[string]string) {
	if client == nil || boot == nil {
		return nil, nil
	}
	event := latestClosedEvent(boot, now)
	if event == nil {
		return nil, nil
	}
	live, err := client.Live(ctx, event.ID)
	if err != nil {
		live = nil
	}
	fixtures, err := client.FixturesLive(ctx)
	if err != nil {
		return live, nil
	}
	idToShort := make(map[int]string, len(boot.Teams))
	for _, t := range boot.Teams {
		idToShort[t.ID] = t.ShortName
	}
	status := make(map[string]string, len(boot.Teams))
	for _, f := range fixtures {
		if f.Event == nil || *f.Event != event.ID {
			continue
		}
		s := "scheduled"
		switch {
		case f.Finished:
			s = "finished"
		case f.Started:
			s = "live"
		}
		if home := idToShort[f.TeamH]; home != "" {
			status[home] = s
		}
		if away := idToShort[f.TeamA]; away != "" {
			status[away] = s
		}
	}
	return live, status
}
