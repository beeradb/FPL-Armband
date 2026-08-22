package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// apiResults answers GET /api/results?gw=N: the SESSION's own imported entry's actual
// result for one gameweek — the same document armbandTeamState builds for the site's own
// squad (config.EntryID), generalised to any reader who has imported a team
// (session.Entry). See buildResults's own comment for why this is the SAME builder and
// not a second one: nothing below passes buildResults anything that says whose entry this
// is, only the picks, the live stats and the chosen event.
//
// # Why this refuses rather than falling back
//
// buildSquadPage's Fixed/Arrange path falls through to the model's optimum when a stored
// fifteen does not resolve — the right answer for the interactive builder, where "no
// stored team" means "build me one". It is the WRONG answer here: a request scoped to
// "what happened in gameweek N" has no optimum to fall back to, only a fact it either has
// or does not. So a picks fetch that fails, or an entry this process cannot reach, answers
// with a status rather than quietly rendering a squad nobody asked to see, labelled as
// this gameweek's result.
//
// # Ordering, and why the FPL fetches run before the lock
//
// The same discipline importTeam's numbered steps document: the method and session gates
// run first and cost nothing; the gw is validated, and the deadline checked, before any
// network call, so a bad or premature request is refused without spending a round trip;
// the FPL fetches then run OUTSIDE s.mu, so one reader's network latency cannot stall
// every other reader's render behind it (see importTeam's step 6 comment); buildSquadPage
// — the one piece that touches the shared engine — runs under the lock, same as every
// other render on this server.
//
// # Why this is unauthenticated
//
// Like /api/state and /api/player, this reads only the caller's own session cookie and
// mutates nothing, so there is no write for a CSRF token to protect — see
// sessionWriteNeedsToken's own comment for the route this rule does NOT extend to.
func (s *squadServer) apiResults(w http.ResponseWriter, r *http.Request) {
	// GET only, like every other read route on this server states its method explicitly.
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "the results route takes a GET", http.StatusMethodNotAllowed)
		return
	}

	sess := s.readValidSession(r)
	if sess.Entry == 0 {
		http.Error(w, "Import a team first — there is no squad to show a result for.",
			http.StatusConflict)
		return
	}

	gw, err := strconv.Atoi(r.URL.Query().Get("gw"))
	if err != nil || gw <= 0 {
		http.Error(w, "gw must be a positive gameweek number", http.StatusBadRequest)
		return
	}
	event := eventByID(s.engine.Boot, gw)
	if event == nil {
		http.Error(w, fmt.Sprintf("gameweek %d does not exist", gw), http.StatusBadRequest)
		return
	}

	now := s.now()
	if event.DeadlineTime.After(now) {
		// FPL serves no picks until a gameweek's deadline has passed — the same
		// condition importWindow already gates the import affordance on. Saying so is
		// the correct answer; there is nothing to fetch that would make this open.
		http.Error(w, fmt.Sprintf("Gameweek %d has not been played yet — there is no "+
			"result to show.", gw), http.StatusConflict)
		return
	}

	// Everything below is a network round trip, and every one of them runs before the
	// render lock is taken — see this function's own comment.
	//
	// Entry first, and refused immediately on failure: no fallback, so there is no point
	// spending the picks round trip too when this request already has no result to show.
	entry, err := s.entryCached(r.Context(), sess.Entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: results: fetching entry %d: %v\n", sess.Entry, err)
		http.Error(w, "We could not load your squad for that gameweek. Try again in a "+
			"minute.", http.StatusConflict)
		return
	}
	var history *fpl.EntryHistory
	if s.client != nil {
		if h, herr := s.client.History(r.Context(), sess.Entry); herr == nil {
			history = h
		}
	}
	picks, err := s.picksCached(r.Context(), sess.Entry, event.ID)
	var fixed []int
	var arrange arrangement
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: results: fetching picks for entry %d gw%d: %v\n",
			sess.Entry, event.ID, err)
	} else {
		fixed, arrange = picksToFixed(s.engine.Boot, picks)
	}
	// Always-fresh, never cached — see houseLiveSources' own comment. A nil s.client (a
	// test with no live client configured) degrades to the same honest absence a fetch
	// failure gives: no live overlay, and ResultState "" rather than a guess.
	live, matchStatus, opponent, resultState := houseLiveSources(r.Context(), s.client, s.engine.Boot, event)

	// No fallback: a squad that did not resolve means there is no result to show — not an
	// invitation to render the model's optimum captioned as one. See this function's own
	// comment.
	if fixed == nil {
		http.Error(w, "We could not load your squad for that gameweek. Try again in a "+
			"minute.", http.StatusConflict)
		return
	}

	defer s.lockRender("results")()

	b, err := buildSquadPage(r.Context(), *s.cfg, s.client, s.engine, pageOpts{
		Weeks:    s.weeks,
		WantPage: true,
		Now:      now,
		Fixed:    fixed,
		Arrange:  arrange,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: results: %v\n", err)
		http.Error(w, "the result could not be built just now — try reloading", http.StatusInternalServerError)
		return
	}

	st, err := viewmodel.Build(viewmodel.Input{
		Page:         b.Page,
		Boot:         s.engine.Boot,
		Cfg:          *s.cfg,
		Now:          now,
		Chips:        s.cfg.Chips,
		Entry:        entry,
		History:      history,
		Live:         live,
		MatchStatus:  matchStatus,
		Opponent:     opponent,
		Multiplier:   arrange.Mult,
		ResultEvent:  event.ID,
		ResultState:  resultState,
		EventAverage: event.AverageScore,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: results: %v\n", err)
		http.Error(w, "the result could not be built just now — try reloading", http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: results: %v\n", err)
		http.Error(w, "the result could not be built just now — try reloading", http.StatusInternalServerError)
		return
	}
	writeResult(w, body, resultState)
}

// entryCached and picksCached are the CACHED path over Client.Entry and Client.Picks, for
// GET /api/results — reusing squadServer's existing fetchEntry/fetchPicks test seams,
// the same ones importTeam's UNcached calls go through (see squadServer's own doc
// comment on those fields), because a test only needs to fake the round trip, not which
// cache tier production takes.
//
// # Why the cached path, unlike PUT /api/import
//
// Client.EntryUncached's own doc comment carves out the ordinary cached path for an id
// "the OPERATOR configured... or one covered by the same trust boundary" and refuses it
// for PUT /api/import, where an anonymous visitor can make the disk cache's file count
// grow just by typing digits into a form, whether or not a real team exists behind them.
// session.Entry is narrower than that: it can only be an id that already passed
// EntryUncached's own existence check at import time, and — the design's own corrected
// note — a closed gameweek's picks never change, so a fetched entry or squad is safe to
// keep on the cached path houseRealPicks already argues for the site's own account. See
// the design note this route was built from for the correction this replaced.
func (s *squadServer) entryCached(ctx context.Context, id int) (*fpl.Entry, error) {
	if s.fetchEntry != nil {
		return s.fetchEntry(ctx, id)
	}
	if s.client == nil {
		return nil, fmt.Errorf("no FPL client is configured")
	}
	return s.client.Entry(ctx, id)
}

func (s *squadServer) picksCached(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
	if s.fetchPicks != nil {
		return s.fetchPicks(ctx, entryID, event)
	}
	if s.client == nil {
		return nil, fmt.Errorf("no FPL client is configured")
	}
	return s.client.Picks(ctx, entryID, event)
}

// eventByID finds one gameweek by its FPL event id, or nil if the bootstrap does not
// carry it — the results route's ?gw= lookup, and its own answer to "gw does not exist
// in the bootstrap".
func eventByID(boot *fpl.Bootstrap, id int) *fpl.Event {
	if boot == nil {
		return nil
	}
	for i := range boot.Events {
		if boot.Events[i].ID == id {
			return &boot.Events[i]
		}
	}
	return nil
}

// writeResult sends a GET /api/results document, cached according to whether the
// gameweek it describes can still change — see routeResults' own design note.
//
// "final" (every fixture in the gameweek is FPL-Finished, bonus applied and checked)
// never changes again, so the response may cache hard. "live" and "fulltime" can still
// move — a live score updates on every request, and a fulltime one is still waiting on
// bonus — so those get the ordinary no-store every other document on this server uses
// (see writeState).
//
// `private`, not `public`: this document is the SESSION's own squad, read off a cookie a
// shared cache cannot see, so a proxy or CDN caching it under the request URL alone would
// serve one reader's result to the next reader who asks for the same gameweek. `private`
// keeps the hard caching in the reader's own browser, where the cookie that produced this
// exact document is the only thing that can ask for it again.
func writeResult(w http.ResponseWriter, body []byte, resultState string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if resultState == "final" {
		w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	_, _ = w.Write(body)
}
