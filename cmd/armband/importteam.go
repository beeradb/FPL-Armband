package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"

	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// buildImport is buildState's own translation of importWindow into the client contract —
// kept beside importTeam rather than inside webroutes.go because the two are one feature.
// It never decides anything itself: importWindow says whether the feature is on and which
// gameweek's picks it would fetch, and sess carries what THIS reader has already done
// about it (imported, or chosen to start fresh).
func buildImport(events []fpl.Event, sess session) viewmodel.Import {
	importEvent, nextEvent, open := importWindow(events)
	return viewmodel.Import{
		Open:    open,
		Event:   importEvent,
		Next:    nextEvent,
		Skipped: sess.ImportSkipped,
		Entry:   sess.Entry,
	}
}

// entryUncached and picksUncached are importTeam's seam over the FPL client — see the
// fetchEntry/fetchPicks fields' doc comment on squadServer for why a test needs one.
func (s *squadServer) entryUncached(ctx context.Context, id int) (*fpl.Entry, error) {
	if s.fetchEntry != nil {
		return s.fetchEntry(ctx, id)
	}
	if s.client == nil {
		return nil, fmt.Errorf("no FPL client is configured")
	}
	return s.client.EntryUncached(ctx, id)
}

func (s *squadServer) picksUncached(ctx context.Context, entryID, event int) (*fpl.EntryPicks, error) {
	if s.fetchPicks != nil {
		return s.fetchPicks(ctx, entryID, event)
	}
	if s.client == nil {
		return nil, fmt.Errorf("no FPL client is configured")
	}
	return s.client.PicksUncached(ctx, entryID, event)
}

// importTeam handles PUT /api/import: a visitor pastes their FPL Team ID (or the whole
// points-page URL) and this fetches their existing squad from FPL and stores it as their
// session's team, the same store saveSession writes.
//
// The steps below run in this exact order, and the order is load-bearing — each comment
// says why it may not move, the same discipline saveSession's own doc comment uses.
func (s *squadServer) importTeam(w http.ResponseWriter, r *http.Request) {
	// 1. Clamped before anything else parses the body. The body is the one thing a
	// caller can make arbitrarily expensive, and this route carries one short field.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)

	// 2. PUT only, and for the same reason saveSession is: PUT forces a browser to
	// preflight a cross-origin request, and nothing here answers OPTIONS. A POST with a
	// form-encoded or text/plain body is a "simple request" that ships with no
	// preflight at all, so any page open in the reader's browser could otherwise drive
	// an import of ITS OWN choosing into the reader's session.
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", "PUT")
		http.Error(w, "the import route takes a PUT", http.StatusMethodNotAllowed)
		return
	}

	// 3. CSRF, before the mutex and before any network call — refused admission must
	// not queue behind, or pay for, work a flood of refusals could otherwise trigger.
	// The token is the strongest claim available and settles it on its own; failing
	// that, a browser's own Sec-Fetch-Site/Origin (sameOrigin) is checked rather than a
	// second implementation of the same test saveSession already carries.
	if !s.tokenOK(r.Header.Get("X-Armband-Token")) && !sameOrigin(r) {
		http.Error(w, "cross-origin writes are refused", http.StatusForbidden)
		return
	}

	// 4. The gameweek gate, BEFORE any parsing and BEFORE any network call — a closed
	// window means there is nothing to import regardless of what the reader typed, so
	// there is no reason to validate their input or spend an FPL round trip finding
	// that out. See importWindow's own doc comment for what "open" means.
	importEvent, _, open := importWindow(s.engine.Boot.Events)
	if !open {
		http.Error(w, "There is nothing to import yet — Gameweek 1 has not been "+
			"played. Pick your fifteen here and we'll take it from there.",
			http.StatusConflict)
		return
	}

	// 5. Parse the body and validate the id's SHAPE. Still no network call: a
	// malformed id is refused before FPL is ever asked about it.
	var req struct {
		Entry any `json:"entry"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "That is not a Team ID. It's the number in your FPL "+
			"points-page URL, between /entry/ and /event/.", http.StatusBadRequest)
		return
	}
	id, err := fpl.ParseEntryID(entryFieldAsString(req.Entry))
	if err != nil {
		http.Error(w, "That is not a Team ID. It's the number in your FPL "+
			"points-page URL, between /entry/ and /event/.", http.StatusBadRequest)
		return
	}

	// 6. The FPL fetches happen here, BEFORE the render lock is taken.
	//
	// s.mu serialises every render on this server — see lockRender's own comment — and
	// a render already takes real time. Holding it across two outbound HTTP calls to
	// FPL, on top of that, would stall every OTHER visitor's page behind this one
	// import's network latency, for however long FPL takes to answer a stranger's
	// request. Nothing below this point touches state the lock protects, so there is
	// nothing to hold it for yet.
	// The entry fetch's only purpose here is to confirm FPL has heard of this id at
	// all: v1 imports the fifteen from Picks, not anything Entry itself carries. It is
	// still fetched and checked first, deliberately, so a bad id is refused as "no
	// such team" rather than as the picks route's more confusing "no squad" answer.
	if _, err := s.entryUncached(r.Context(), id); err != nil {
		// FPL's own response text must never reach the browser — see main.go's own
		// note on the same rule for price-history errors. The full error, which may
		// carry up to 200 bytes of FPL's response body, goes to stderr only.
		fmt.Fprintf(os.Stderr, "serve: import: fetching entry %d: %v\n", id, err)
		if errors.Is(err, fpl.ErrNotFound) {
			http.Error(w, fmt.Sprintf("FPL has no team with the ID %d. Check the "+
				"number in your points-page URL and try again.", id), http.StatusNotFound)
			return
		}
		http.Error(w, "FPL is not answering just now. Try again in a minute, or "+
			"start fresh — you can import later.", http.StatusServiceUnavailable)
		return
	}

	picks, err := s.picksUncached(r.Context(), id, importEvent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: import: fetching picks for entry %d gw%d: %v\n",
			id, importEvent, err)
		if errors.Is(err, fpl.ErrNotFound) {
			http.Error(w, fmt.Sprintf("That team has no Gameweek %d squad, so there "+
				"is nothing to import yet. Start fresh and we'll plan from here.",
				importEvent), http.StatusConflict)
			return
		}
		http.Error(w, "FPL is not answering just now. Try again in a minute, or "+
			"start fresh — you can import later.", http.StatusServiceUnavailable)
		return
	}

	// 7. Map every pick's season-scoped element id to the player's PERMANENT code.
	// This codebase keys everything downstream — session.Squad included — on the code,
	// never the element id, because ids are reassigned every summer. A pick that fails
	// to resolve refuses the WHOLE import rather than storing fourteen players and
	// silently dropping the fifteenth: a partial fifteen is not a team, the same rule
	// squadFromCodes already enforces for a reload.
	byElement := make(map[int]int, len(s.engine.Boot.Elements))
	for i := range s.engine.Boot.Elements {
		el := &s.engine.Boot.Elements[i]
		byElement[el.ID] = el.Code
	}
	sortedPicks := append([]fpl.Pick(nil), picks.Picks...)
	sort.Slice(sortedPicks, func(i, j int) bool { return sortedPicks[i].Position < sortedPicks[j].Position })

	var squad, xi, bench []int
	var captain, vice int
	for _, p := range sortedPicks {
		code, ok := byElement[p.Element]
		if !ok {
			http.Error(w, "We could not rebuild that squad from this season's "+
				"player list. Start fresh and we'll plan from here.",
				http.StatusInternalServerError)
			return
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
	}
	if len(squad) != 15 {
		// FPL always answers with fifteen picks; this is the fallback for a squad
		// that does not, rather than storing whatever fraction of one arrived.
		http.Error(w, "We could not rebuild that squad from this season's player "+
			"list. Start fresh and we'll plan from here.", http.StatusInternalServerError)
		return
	}

	// 8. Build the new session, preserving standing corrections and replacing the team.
	in := s.readValidSession(r).fromImport(id, squad, xi, bench, captain, vice)

	// 9. Run it through the SAME validator every other session write goes through —
	// see saveSession's own comment. It should always pass for a freshly-fetched FPL
	// squad; running it anyway keeps one single statement of what a storable session
	// is, rather than a second belief about it living here.
	if err := s.validateSession(in); err != nil {
		fmt.Fprintf(os.Stderr, "serve: import: fetched squad failed validation: %v\n", err)
		http.Error(w, "We could not rebuild that squad from this season's player "+
			"list. Start fresh and we'll plan from here.", http.StatusInternalServerError)
		return
	}

	// 10. The render lock, taken only now — see step 6's comment for why it could not
	// be taken any earlier.
	defer s.lockRender("import")()

	stateBody, err := s.buildState(r, in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "serve: import: %v\n", err)
		http.Error(w, "the squad could not be built just now — try reloading",
			http.StatusInternalServerError)
		return
	}

	// 11. The cookie is written only AFTER the state build has succeeded — see
	// saveSession's own comment on why the other order is a defect: it would store an
	// HttpOnly cookie the page has no way to clear, for a session that cannot render.
	//
	// ⚠️ This handler must NEVER write to s.cfg and must NEVER call config.Save. v1
	// imports the fifteen only, not the budget: the assembly budget stays the existing
	// hypothetical £100.0m assumption regardless of what this reader's real bank is.
	// Making the budget real is a separate, future change — do not "fix" that here by
	// reaching for config or for s.cfg.EntryID, which is the agent-facing concept this
	// whole feature exists NOT to reuse (see session.Entry's own doc comment).
	if err := in.write(w); err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	// 12. The document itself.
	writeState(w, stateBody)
}

// entryFieldAsString normalises the JSON body's "entry" value to a string before it
// reaches fpl.ParseEntryID, which only ever validates a string. The client may reasonably
// send either a JSON string (the raw text of the input box) or a JSON number (if it
// parsed the box itself first) — this accepts both rather than requiring the client to
// pick one shape and get it exactly right.
func entryFieldAsString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// encoding/json decodes every JSON number into a float64 when the target is
		// `any`. 'f'/-1 is the shortest representation that round-trips, which for
		// an integer-valued float — the only kind an entry id ever is — prints with
		// no decimal point at all.
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}
