package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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
	st, err := viewmodel.Build(viewmodel.Input{
		Page:         b.Page,
		Boot:         s.engine.Boot,
		Cfg:          *s.cfg,
		Now:          now,
		Chips:        s.cfg.Chips,
		HouseEntry:   houseEntry,
		HouseHistory: houseHistory,
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
