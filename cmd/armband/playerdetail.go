package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// playerFetchLimit bounds how many upstream element-summary fetches playerDetail will have
// in flight at once, process-wide.
//
// This route is unauthenticated and reachable from any page open in the reader's browser —
// the Host check stops DNS rebinding, not an ordinary same-host fetch() from another tab or
// a hostile page. It is unauthenticated for the same reason /api/state is (see playerDetail
// below), but an unbounded call here costs more: without a limit, a burst across the
// bootstrap's ~590 codes would fire that many concurrent requests to FPL's own public API
// from the reader's IP -- a plausible way to get the reader rate-limited by FPL, not a way
// to steal anything. Sized like recent.DefaultConcurrency, which measured this same upstream
// endpoint comfortably fast at six.
const playerFetchLimit = 6

var playerFetchSem = make(chan struct{}, playerFetchLimit)

// elementSummary is the seam over Client.ElementSummary — see the fetchSummary field's doc
// comment on squadServer for why a test needs one.
//
// The semaphore sits here rather than in internal/fpl.Client, because it is a property of
// THIS ROUTE'S exposure (unauthenticated, browser-reachable, fanning out across every code
// in the bootstrap) rather than of the client generally — every other caller of
// ElementSummary runs from a CLI process with no such fan-out. A test that sets
// s.fetchSummary bypasses it entirely, which is correct: the seam replaces this whole path,
// throttle included.
func (s *squadServer) elementSummary(ctx context.Context, id int) (*fpl.ElementSummary, error) {
	if s.fetchSummary != nil {
		return s.fetchSummary(ctx, id)
	}
	if s.client == nil {
		return nil, fmt.Errorf("no FPL client is configured")
	}
	select {
	case playerFetchSem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-playerFetchSem }()
	return s.client.ElementSummary(ctx, id)
}

// playerDetail answers the depth behind one player's card: last season's record and this
// season's match-by-match log, at prefixPlayer+"{code}".
//
// # Why this does not take the server's mutex
//
// That lock serialises the optimiser, and this route touches neither the squad nor config —
// it fetches one player's history and arranges it. That is safe to run alongside a squad
// rebuild because the only shared state it reads is s.engine.Boot, which is built once at
// startup and never reassigned; concurrent reads of it are exactly as safe as concurrent
// reads of a value nothing ever mutates again. playerFetchSem is what bounds the concurrency
// this lack of a lock permits — see elementSummary.
//
// # Why this is unauthenticated, like /api/state
//
// This is a read of finished football, keyed on a code the caller only has because the state
// document already sent it on every Player. Answering it costs nothing an attacker did not
// already have a cheaper way to see, and it must never grow a write.
func (s *squadServer) playerDetail(w http.ResponseWriter, r *http.Request) {
	// GET only, like every other read route on this server states its method explicitly.
	// The handler has no side effect regardless, so this is a courtesy to a caller rather
	// than a guard against one.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "the player route takes a GET", http.StatusMethodNotAllowed)
		return
	}

	code, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, prefixPlayer))
	if err != nil || code <= 0 {
		http.Error(w, "the player route takes a permanent player code", http.StatusBadRequest)
		return
	}

	var el *fpl.Element
	for i := range s.engine.Boot.Elements {
		if s.engine.Boot.Elements[i].Code == code {
			el = &s.engine.Boot.Elements[i]
			break
		}
	}
	if el == nil {
		http.Error(w, "no player has that code", http.StatusNotFound)
		return
	}

	es, err := s.elementSummary(r.Context(), el.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: element summary for player code %d: %v\n", code, err)
		http.Error(w, "could not load that player's history just now", http.StatusBadGateway)
		return
	}

	// Built once per request rather than cached on the server: it is a handful of map
	// inserts over teams that number in the twenties, and caching it would be a second
	// place team names can go stale relative to s.engine.Boot.
	teams := make(map[int]string, len(s.engine.Boot.Teams))
	for _, t := range s.engine.Boot.Teams {
		teams[t.ID] = t.ShortName
	}

	body, err := json.Marshal(viewmodel.BuildPlayerDetail(
		es, s.engine.Boot.PositionShort(el.ElementType), teams))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeState(w, body)
}
