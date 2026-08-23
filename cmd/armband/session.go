package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/viewmodel"
)

// session is one reader's work: the team they have arranged, and the corrections they have
// made to the model's view of it.
//
// # Why it is a cookie, and what that buys
//
// The application had no persistence at all: a lock changed a JavaScript Set, a deleted
// override filtered a JavaScript array, and a reload discarded both while the page said
// otherwise. This is the store that makes those real.
//
// The cookie is HttpOnly, so the client never reads it — it sends its state, the server
// stores it and answers with the recomputed document. That keeps one direction of truth:
// the page never holds an opinion the server has not seen.
//
// # Everything here is a permanent player CODE, never an element id
//
// Element ids are reassigned every summer. This project has already shipped an override
// keyed on one and had it come back in August attached to a different footballer, which is
// why config keys everything on the code. A cookie outlives a deploy and can outlive a
// season rollover in a browser left open, so it obeys the same rule.
//
// # Why the FIFTEEN is stored, not just the changes
//
// Two reasons, and the second is the one that matters. A reader who arranges a team expects
// it back; and with the fifteen known, a reload does not need the optimiser at all — the
// lineup is rebuilt with analysis.BestXI, which is arithmetic rather than a search. The
// page went from re-running a one-second optimisation on every load to running it only when
// there is no team yet or the reader asks for one.
type session struct {
	// Version is what lets the shape change without stranding a reader on a cookie that
	// no longer parses. An unrecognised version is discarded rather than guessed at.
	Version int `json:"v"`

	// Seed picks which of several good opening squads this reader gets, and is stored so
	// the answer is stable across reloads. See buildVariedSquad.
	Seed int64 `json:"seed,omitempty"`
	// Optimised records that the reader pressed Optimize and wants the true optimum
	// rather than a varied one.
	Optimised bool `json:"opt,omitempty"`

	// Squad is the fifteen, by code. XI and Bench are the arrangement of it; Bench is in
	// substitution order with the reserve keeper first.
	Squad   []int `json:"squad,omitempty"`
	XI      []int `json:"xi,omitempty"`
	Bench   []int `json:"bench,omitempty"`
	Captain int   `json:"cap,omitempty"`
	Vice    int   `json:"vc,omitempty"`

	// Lock and Exclude are the standing corrections, mutually exclusive per player.
	Lock    []int `json:"lock,omitempty"`
	Exclude []int `json:"excl,omitempty"`

	// Dismissed are overrides read from config that this reader has cleared for this
	// session. They are recorded by the code they act on rather than deleted from the
	// file: a browser must not edit the standing record, and `serve -persist` is the
	// deliberate way to do that.
	Dismissed []int `json:"dis,omitempty"`

	// Chips maps a gameweek number to a chip key. Stored as strings because a JSON object
	// cannot have integer keys, and a map rather than a list because one gameweek holds
	// at most one chip.
	Chips map[string]string `json:"chips,omitempty"`

	// Entry is the imported team's FPL entry (Team) id, 0 if nothing has been imported.
	//
	// This is deliberately NOT config.EntryID. That field is the agent-facing "my team"
	// concept, and squadServer.cfg is one *config.Config shared by every visitor to this
	// process — writing a visitor's imported id there would leak it into every other
	// visitor's page on the next build. The imported id belongs to this one reader, so it
	// lives in the per-visitor session cookie instead, the same way Squad and the rest of
	// this reader's team do.
	Entry int `json:"entry,omitempty"`
	// ImportSkipped records that the reader chose "start fresh" over importing, so the
	// offer is not shown again on the next reload of the same session.
	ImportSkipped bool `json:"noimp,omitempty"`
}

// sessionVersion is the shape below. Bump it when a field changes meaning rather than when
// one is added — an added field decodes as its zero value, which is correct.
const sessionVersion = 1

// sessionCookieName carries the session. Base64 because a bare JSON object contains commas
// and braces, which the cookie grammar forbids; base64's alphabet is entirely cookie-safe.
const sessionCookieName = "fpl_session"

// maxSessionBytes is the encoded ceiling.
//
// Browsers drop a cookie over about 4 KB, silently, and a silently dropped session looks
// exactly like a reader whose work was never saved. The fifteen plus the arrangement plus
// the corrections is a few hundred bytes, so hitting this means something unbounded got in
// — and refusing loudly beats saving nothing quietly.
const maxSessionBytes = 3500

func readSession(r *http.Request) session {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return session{}
	}
	raw, err := base64.StdEncoding.DecodeString(c.Value)
	if err != nil {
		return session{}
	}
	var s session
	if err := json.Unmarshal(raw, &s); err != nil {
		return session{}
	}
	if s.Version != sessionVersion {
		// A cookie from an older shape is discarded rather than migrated. There is one
		// version and nobody has a stale one worth rescuing; when that stops being true,
		// migrate here rather than guessing field by field.
		return session{}
	}
	return s.settled()
}

// readValidSession is readSession plus the rule the write path enforces.
//
// The two used to differ, and the gap was reachable: cookies are not scoped by port, so
// anything the browser loads from another service on 127.0.0.1 can set fpl_session for this
// one. A fifteen the write path refuses -- the same player fifteen times, say -- would then
// be rebuilt on every read, producing an empty eleven and a zero captain that the client
// throws on, in an HttpOnly cookie the page cannot clear. The reader's only escape was
// devtools.
//
// Discarding beats repairing. A session that fails the rule is not a session with one bad
// field; it is one this server did not write, and guessing at what it meant is how a store
// acquires a second set of semantics.
func (s *squadServer) readValidSession(r *http.Request) session {
	sess := readSession(r)
	if sess.empty() {
		return sess
	}
	if err := s.validateSession(sess); err != nil {
		fmt.Fprintf(os.Stderr, "serve: discarding a stored session that is not legal: %v\n", err)
		return session{}
	}
	return sess
}

// settled resolves the reader's dismissals against their own lists.
//
// A dismissal has two jobs, and only one of them survives being applied late. It suppresses
// an override that came from config.json — that one has to be re-applied on every request,
// because config is not the session's to edit. And it clears a lock or a block the reader
// made themselves — and that one is a deletion, so leaving it to be re-applied later means
// the stored list and the model disagree for as long as the session lives. The page redraws
// its badges from the stored list, so the reader sees a block the model is not applying.
//
// Settling here makes the session say what the model will do. Callers do not filter.
func (s session) settled() session {
	if len(s.Dismissed) == 0 {
		return s
	}
	gone := map[int]bool{}
	for _, code := range s.Dismissed {
		gone[code] = true
	}
	drop := func(codes []int) []int {
		out := codes[:0:0]
		for _, c := range codes {
			if !gone[c] {
				out = append(out, c)
			}
		}
		return out
	}
	s.Lock, s.Exclude = drop(s.Lock), drop(s.Exclude)
	return s
}

// write stores the session, or clears the cookie when it holds nothing — an empty session
// and a dead cookie must not read differently on the next request.
func (s session) write(w http.ResponseWriter) error {
	if s.empty() {
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Path: "/", MaxAge: -1})
		return nil
	}
	s.Version = sessionVersion
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	value := base64.StdEncoding.EncodeToString(raw)
	if len(value) > maxSessionBytes {
		return fmt.Errorf("the session is %d bytes encoded, over the %d-byte ceiling — "+
			"a browser would drop it silently and the reader's work would vanish without "+
			"an error", len(value), maxSessionBytes)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
		// HttpOnly: the client never reads this. It sends its state and the server
		// answers with the document; one direction of truth.
		HttpOnly: true,
	})
	return nil
}

func (s session) empty() bool {
	return len(s.Squad) == 0 && len(s.XI) == 0 && len(s.Lock) == 0 && len(s.Exclude) == 0 &&
		len(s.Dismissed) == 0 && len(s.Chips) == 0 && s.Seed == 0 && !s.Optimised &&
		s.Entry == 0 && !s.ImportSkipped
}

// forPlanner strips the corrections that belong to the analysis layer rather than to the
// reader, leaving the ones that describe the WORLD.
//
// The distinction is the whole point. A minutes override is team news — "he is nailed, the
// record just cannot see it yet" — and the planner wants it, because it is a correction to
// what the model KNOWS. A lock is a decision — "build every squad around this player" —
// taken by the agent for its own weekly recommendation, and the planner must not inherit
// it, because the reader is the one choosing here and a squad silently built around
// somebody else's conclusion is not a suggestion, it is a constraint wearing one.
//
// Exclusions stay. An exclusion is nearly always the world speaking — a player out for the
// season, gone in the transfer window — and its failure mode is mild: the reader sees a
// player he cannot pick and a stated reason, rather than a squad built around a choice he
// did not make. The reader's OWN locks, from the session, are applied after this and are
// untouched: they are his.
func forPlanner(cfg config.Config) config.Config {
	cfg.Roster.Lock = nil
	return cfg
}

// chipsInto folds the reader's chip placements into a plan the engine will act on.
//
// The session stores gameweek → chip key, which is how the page thinks about a chip: it
// belongs to a week. Which of FPL's two chip SETS that week falls in is a fact about the
// competition rather than about the reader, so ChipSchedule.Place answers it from the feed's
// own windows. Deciding it here would put a second statement of the season's shape in the
// session store, and it would be the wrong one — the obvious guess, "everything is a
// first-set chip", is wrong for eighteen gameweeks of every season.
//
// The reader's placement REPLACES the configured one for that chip rather than merging with
// it. Two answers to "when is the wildcard" is the thing this codebase pays for most often,
// and the person looking at the page should win on their own page.
//
// A placement in no window is dropped. validateSession refuses those, so reaching this is a
// stored session written before a rule changed rather than anything a caller sent today.
func (s session) chipsInto(plan analysis.ChipSchedule, boot *fpl.Bootstrap) analysis.ChipSchedule {
	// Ranged in gameweek order, not map order.
	//
	// Two placements of the same chip resolve by whichever is applied last, and ranging a Go
	// map makes that unspecified per call — so the plan flipped between them on every
	// request, EffectiveHorizon truncated the engine or did not, and the reader was served a
	// different fifteen on each reload with nothing having changed. Sorting makes the answer
	// a function of the session alone. The earlier week wins, which is the one the reader
	// reaches first.
	weeks := make([]int, 0, len(s.Chips))
	byWeek := make(map[int]string, len(s.Chips))
	for week, key := range s.Chips {
		gw, err := strconv.Atoi(week)
		if err != nil || gw <= 0 {
			continue
		}
		weeks = append(weeks, gw)
		byWeek[gw] = key
	}
	sort.Sort(sort.Reverse(sort.IntSlice(weeks)))
	for _, gw := range weeks {
		if placed, ok := plan.Place(boot, byWeek[gw], gw); ok {
			plan = placed
		}
	}
	return plan
}

// applyTo layers the session's standing corrections over the config.
//
// The session WINS on conflict — a session exclusion clears a config lock for this page and
// never for the file — so the controls always express the reader's latest decision without
// touching config.json. In persist mode the session is ignored and the config is the one
// store.
func (s session) applyTo(cfg config.Config, e *analysis.Engine, today string) config.Config {
	dismissed := map[int]bool{}
	for _, code := range s.Dismissed {
		dismissed[code] = true
	}
	// A dismissal removes the override from THIS page's config copy. The file is
	// untouched; `serve -persist` is how a correction actually leaves the record.
	cfg.Roster.Exclude = keepUndismissed(cfg.Roster.Exclude, dismissed)
	cfg.Roster.Lock = keepUndismissed(cfg.Roster.Lock, dismissed)
	cfg.Roster.Minutes = keepUndismissed(cfg.Roster.Minutes, dismissed)

	name := func(code int) string {
		for i := range e.Boot.Elements {
			if e.Boot.Elements[i].Code == code {
				return e.Boot.Elements[i].WebName
			}
		}
		return ""
	}
	set := func(mode string, codes []int, reason string) {
		for _, code := range codes {
			n := name(code)
			if n == "" {
				// A code the bootstrap no longer resolves is dropped rather than
				// carried: it would render as a nameless row nobody could clear.
				continue
			}
			_ = cfg.Roster.Set(mode, config.RosterOverride{
				Code: code, Name: n, Reason: reason, SetOn: today, LastChecked: today,
			}, nil)
		}
	}
	// ⚠️ The session's own corrections are filtered by the dismissals too, and they are
	// filtered BEFORE they are applied.
	//
	// They used to be re-added after the filter, which made the cross on a
	// session-created override inert by construction: the reader blocked a player, pressed
	// the cross, and nothing happened. Worse, a blocked market player is dropped from the
	// market list, so there was no row left anywhere to open his sheet from and toggle him
	// back — the only recovery was deleting the cookie.
	cfg.Chips = s.chipsInto(cfg.Chips, e.Boot)
	set("lock", s.Lock, "locked from the planner — browser session")
	set("exclude", s.Exclude, "blocked from the planner — browser session")
	return cfg
}

func keepUndismissed(in []config.RosterOverride, dismissed map[int]bool) []config.RosterOverride {
	if len(dismissed) == 0 {
		return in
	}
	out := in[:0:0]
	for _, o := range in {
		if !dismissed[o.Code] {
			out = append(out, o)
		}
	}
	return out
}

// arrangement is what the client needs to draw its own controls: which players this reader
// has pinned or barred, and which chips they have placed.
//
// Deliberately not the whole session. The fifteen and the eleven reach the client through
// the SQUAD, already resolved to element ids and already arranged — sending them twice
// would be two representations of one thing, and the client would have to choose which to
// believe.
func (s session) arrangement() viewmodel.Session {
	return viewmodel.Session{
		Locked:  s.Lock,
		Blocked: s.Exclude,
		Chips:   s.Chips,
		// Sent although nothing draws it: the client rebuilds its pending session from
		// the document, so an omitted field is a field it will overwrite with nothing.
		Dismissed: s.Dismissed,
	}
}

// applyAction mirrors config.Roster.Set/Remove semantics on the session store: lock and
// exclude are mutually exclusive, and unlock/unboot lift one list.
//
// It serves the form-POST write path in serve.go, which predates the application and is the
// fallback for a reader with no JavaScript.
func (s session) applyAction(action string, code int) session {
	drop := func(codes []int, code int) []int {
		out := codes[:0:0]
		for _, c := range codes {
			if c != code {
				out = append(out, c)
			}
		}
		return out
	}
	add := func(codes []int, code int) []int {
		for _, c := range codes {
			if c == code {
				return codes
			}
		}
		return append(codes, code)
	}
	// Making a correction on a player un-dismisses him. Without this the lock is stored and
	// then deleted by settled() on the very next read — forever, with nothing reporting it —
	// because Dismissed is a standing veto and nothing else prunes it. The client's
	// toggleCorrection does the same thing for the same reason.
	s.Dismissed = drop(s.Dismissed, code)

	switch action {
	case "lock":
		s.Exclude = drop(s.Exclude, code)
		s.Lock = add(s.Lock, code)
	case "boot":
		s.Lock = drop(s.Lock, code)
		s.Exclude = add(s.Exclude, code)
	case "unlock":
		s.Lock = drop(s.Lock, code)
	case "unboot":
		s.Exclude = drop(s.Exclude, code)
	}
	return s
}

// fromImport returns a new session built from an imported FPL squad.
//
// It preserves this session's standing corrections — Lock, Exclude, Dismissed, Chips —
// because those describe the reader's own decisions and the world as they have corrected
// it, neither of which an import should discard. It does NOT preserve Squad, XI, Bench,
// Captain or Vice: an import REPLACES the team, it does not merge with whatever fifteen
// (if any) the reader had before. Optimised is left false, deliberately — an imported
// fifteen is the reader's own team from FPL, not the model's optimum, so the Optimize
// control should read as available rather than already pressed.
func (s session) fromImport(entry int, squad, xi, bench []int, captain, vice int) session {
	return session{
		Lock:      s.Lock,
		Exclude:   s.Exclude,
		Dismissed: s.Dismissed,
		Chips:     s.Chips,
		Squad:     squad,
		XI:        xi,
		Bench:     bench,
		Captain:   captain,
		Vice:      vice,
		Entry:     entry,
	}
}
