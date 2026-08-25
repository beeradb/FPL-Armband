package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"armband/internal/analysis"
	"armband/internal/viewmodel"
)

// apiChipTeams answers GET /api/wildcard: what a wildcard and a free hit would
// each buy in the next gameweek nobody has played yet, off the house
// account's own money -- the OTHER tense from /api/armband-team, which is
// what our real fifteen DID. See the design note this implements for why
// "each week" means the upcoming gameweek and not the configured chip_plan's
// GW6/GW16, and why this is one document for every requester rather than a
// per-reader tool.
//
// # Ordering
//
// The same numbered discipline apiResults documents: the cheap gates first,
// the network before the lock, the engine under it. Network here is optional
// on both counts (the house picks and the account's chip history) -- a
// failure degrades the document to an absence of the 9/15 figure and the OUT
// list, never to an error, because the two rebuilt fifteens do not depend on
// either fetch. See buildSquadPage's own call below for why that's true even
// when the picks fetch fails: Fixed simply falls through to the model's own
// optimum, and this handler only reads that fallback back out through
// housePicks, never through what buildSquadPage actually returned.
//
// # Why this is unauthenticated AND publicly cacheable
//
// Unlike GET /api/results, this document reads no session and no cookie --
// it is built entirely from config.EntryID, the account /armband-team
// already describes to everyone the same way. So the response carries
// Cache-Control: public, not private: a shared cache serving this URL to
// every requester is correct here, where it would leak one reader's own
// squad on /api/results. That safety is a property of THIS HANDLER, not of
// the URL -- if this route ever grows a session input, it needs its own
// uncached nginx location first, the same way the two session-scoped routes
// already do. See writeChipTeams.
func (s *squadServer) apiChipTeams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "the wildcard route takes a GET", http.StatusMethodNotAllowed)
		return
	}

	now := s.now()
	event := nextOpenEvent(s.engine.Boot, now)
	if event == nil {
		http.Error(w, "The season is over — there is no gameweek left to project.",
			http.StatusConflict)
		return
	}

	// The competition's own verdict, not a rule restated here (see
	// analysis.PlayableChips's own comment). Neither being allowed -- gameweek
	// one -- is a real, 200 state: the page's job then is to say why, not to
	// answer an error for a state that is not one.
	var wcAllowed, fhAllowed bool
	for _, key := range analysis.PlayableChips(s.engine.Boot, event.ID) {
		switch key {
		case "wildcard":
			wcAllowed = true
		case "freehit":
			fhAllowed = true
		}
	}

	// Network, off the lock: the house account's real fifteen today (for the
	// 9/15 figure and the OUT list) and its chip history (for "have we played
	// this yet"). Both optional -- see this function's own comment.
	housePicks, houseArrange := houseRealPicks(r.Context(), s.client, s.engine.Boot,
		s.cfg.EntryID, latestClosedEvent(s.engine.Boot, now))
	var playedWildcard, playedFreeHit []int
	if s.client != nil {
		if hist, err := s.client.History(r.Context(), s.cfg.EntryID); err == nil && hist != nil {
			for _, c := range hist.Chips {
				switch c.Name {
				case "wildcard":
					playedWildcard = append(playedWildcard, c.Event)
				case "freehit":
					playedFreeHit = append(playedFreeHit, c.Event)
				}
			}
		}
	}

	defer s.lockRender("chip-teams")()

	// The house fifteen, through the same roster-corrected pipeline every
	// other surface uses. WantPage: false is load-bearing -- it returns before
	// WeekViews, the transfer plan, the watchlist and the overrides, and with
	// Fixed resolving it runs no optimiser search at all: this call exists to
	// get the house fifteen, not to build a page. See buildSquadPage's own
	// pageOpts comment.
	b, err := buildSquadPage(r.Context(), *s.cfg, s.client, s.engine, pageOpts{
		WantPage: false,
		Now:      now,
		Fixed:    housePicks,
		Arrange:  houseArrange,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: wildcard: %v\n", err)
		http.Error(w, "the wildcard team could not be built just now — try reloading",
			http.StatusInternalServerError)
		return
	}
	// housePicks nil means the fetch failed, and buildSquadPage's Fixed/Arrange
	// path then falls through to the model's own optimum -- correct for THAT
	// call's purpose, wrong for this one: b.Squad.Players would then be a
	// rebuild compared against a rebuild. Only trust it as "today's real
	// fifteen" when the fetch that was supposed to produce it actually did.
	var todaySquad []analysis.PlayerMetrics
	if housePicks != nil {
		todaySquad = b.Squad.Players
	}

	budget, source, err := s.engine.AssemblyBudget()
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: wildcard: assembly budget: %v\n", err)
		http.Error(w, "the wildcard team could not be built just now — try reloading",
			http.StatusInternalServerError)
		return
	}

	// The caller's own roster constraints -- locks, exclusions -- so a wildcard
	// this route publishes never shows a player the operator has explicitly
	// blocked (see internal/analysis's ChipWeekView doc comment, FINDING 1).
	// The notes applyRoster prints are for a terminal and mean nothing here.
	var req analysis.OptimizeRequest
	_ = applyRoster(*s.cfg, s.engine, &req)

	// ⚠️ Do not assign s.engine.Chips here. buildSquadPage does and restores it;
	// this handler passes the chip by name to ChipWeekView and must not touch
	// the schedule on the shared engine -- that is what makes the result safe
	// to cache.
	var wc, fh *analysis.WeekView
	cached := s.chipCacheGet(event.ID)
	switch {
	case cached != nil:
		wc, fh = cached.wc, cached.fh
	default:
		if wcAllowed {
			v := s.engine.ChipWeekView(b.Squad.Players, event.ID, "Wildcard", req)
			wc = &v
		}
		if fhAllowed {
			v := s.engine.ChipWeekView(b.Squad.Players, event.ID, "Free Hit", req)
			fh = &v
		}
		s.chipCacheSet(event.ID, wc, fh)
	}

	codes := elementCodes(s.engine)
	ci := &viewmodel.ChipTeamsInput{
		Event:            event.ID,
		Deadline:         event.DeadlineTime,
		Budget:           float64(budget) / 10,
		BudgetSource:     source,
		BudgetWarning:    s.engine.Budget.Warning(),
		Wildcard:         wc,
		FreeHit:          fh,
		PlayedWildcardGW: playedWildcard,
		PlayedFreeHitGW:  playedFreeHit,
		TodaySquad:       todaySquad,
		Codes:            codes,
		Overrides:        b.Page.Overrides,
	}
	if !wcAllowed {
		ci.WildcardUnavailable = fmt.Sprintf(
			"The wildcard is not open in gameweek %d — FPL does not allow it yet.", event.ID)
	}
	if !fhAllowed {
		ci.FreeHitUnavailable = fmt.Sprintf(
			"The free hit is not open in gameweek %d — FPL does not allow it yet.", event.ID)
	}
	if wc != nil && wc.Caveat != "" {
		ci.Caveat = wc.Caveat
	} else if fh != nil {
		ci.Caveat = fh.Caveat
	}

	st, err := viewmodel.Build(viewmodel.Input{
		Now:       now,
		Boot:      s.engine.Boot,
		Cfg:       *s.cfg,
		Chips:     s.cfg.Chips,
		ChipTeams: ci,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: wildcard: %v\n", err)
		http.Error(w, "the wildcard team could not be built just now — try reloading",
			http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: wildcard: %v\n", err)
		http.Error(w, "the wildcard team could not be built just now — try reloading",
			http.StatusInternalServerError)
		return
	}
	writeChipTeams(w, body)
}

// writeChipTeams sends GET /api/wildcard's document.
//
// public, not private — see apiChipTeams's own comment for why this is the
// one place this route differs from writeResult/writeState on purpose: this
// document has no session or cookie input at all, so a cache shared across
// every requester is correct rather than a leak. 300s: long enough that the
// deployment's proxy cache actually helps against an ungated, unrated route
// running two Optimize passes on a miss, short enough that a roster
// correction (which invalidates the in-process cache immediately, see
// invalidateChipCache) is not held stale at the edge for long either.
func writeChipTeams(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(body)
}

// chipCache holds the two rebuilt fifteens for one gameweek.
//
// It exists because each is a full Optimize run and this route is public and
// ungated. It is keyed on the gameweek because the gameweek is the only input
// that moves within a process: Boot and Fixtures are memoised for the life of
// Client (see Client.Bootstrap), and Priors, Recent, the minutes overrides,
// SellPrices, SquadValue and Bank are all set once in main. The cached
// WeekViews therefore cannot go stale except at a deadline, which changes the
// key.
//
// The ONE input that can change under a running process is config, under
// -persist: persistCorrections replaces s.cfg and a roster change moves the
// optimiser's answer. It calls invalidateChipCache. Nothing else may.
//
// Guarded by s.mu, the render mutex -- the same lock the build itself takes,
// so there is one lock and no second ordering to get wrong. Every accessor
// below assumes the caller already holds it.
type chipCache struct {
	event int
	wc    *analysis.WeekView
	fh    *analysis.WeekView
}

// chipCacheGet answers the cached pair for gw, or nil on a miss. Must be
// called under s.mu (apiChipTeams calls it from inside lockRender).
func (s *squadServer) chipCacheGet(gw int) *chipCache {
	if s.chips != nil && s.chips.event == gw {
		return s.chips
	}
	return nil
}

// chipCacheSet stores the pair just built for gw. Must be called under s.mu.
func (s *squadServer) chipCacheSet(gw int, wc, fh *analysis.WeekView) {
	s.chips = &chipCache{event: gw, wc: wc, fh: fh}
}

// invalidateChipCache drops the cached pair, so the next request rebuilds
// under whatever config now says. Called from persistCorrections, where s.cfg
// is replaced -- see chipCache's own comment. Must be called under s.mu;
// persistCorrections already runs under lockRender("session").
func (s *squadServer) invalidateChipCache() {
	s.chips = nil
}
