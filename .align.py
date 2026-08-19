"""Align the contract, the session and the squad rebuild.

One-shot, run from the repository root, then deleted.
"""

import io


def edit(path, pairs):
    s = io.open(path, encoding='utf-8').read()
    for old, new in pairs:
        if s.count(old) != 1:
            raise SystemExit('%s: expected 1 of %r, found %d' % (path, old[:80], s.count(old)))
        s = s.replace(old, new)
    io.open(path, 'w', encoding='utf-8').write(s)


# ---------------------------------------------------------------- the contract
edit('internal/viewmodel/state.go', [
    ('''type Session struct {
	// Store is "session" or "config". In session mode a change lives in a browser
	// cookie and dies with the browser; in config mode it is written to config.json and
	// binds every future run, including the agent's.
	Store string `json:"store"`
	// Writable reports whether the client may write at all.
	Writable bool `json:"writable"`
}''',
     '''type Session struct {
	// Store is "session" or "config". In session mode a change lives in a browser
	// cookie and dies with the browser; in config mode it is written to config.json and
	// binds every future run, including the agent's.
	Store string `json:"store"`
	// Writable reports whether the client may write at all.
	Writable bool `json:"writable"`

	// Locked and Blocked are the players this reader has pinned into or barred from every
	// build, by permanent CODE. The client draws the control states from these rather
	// than keeping its own idea of them, so a reload shows what is actually in force.
	Locked  []int `json:"locked,omitempty"`
	Blocked []int `json:"blocked,omitempty"`

	// Chips the reader has assigned: gameweek number as a string, to a chip key. A JSON
	// object cannot have integer keys.
	Chips map[string]string `json:"chips,omitempty"`

	// Saved reports that this document was built from a stored team rather than freshly
	// chosen. It is what lets the page say "your saved team" honestly instead of implying
	// the model just picked it.
	Saved bool `json:"saved"`
	// Optimised reports that the fifteen is the model's best rather than a varied opening
	// squad. It drives whether the Optimize control reads as available or as done.
	Optimised bool `json:"optimised"`
}'''),
    ('''	// Now must be the same instant the page was built with, or the staleness figures in
	// the overrides will have been decided against a different clock than the one the
	// client is told about.
	Now time.Time
	// Persist reports whether writes go to config.json rather than the browser session.
	Persist bool
}''',
     '''	// Now must be the same instant the page was built with, or the staleness figures in
	// the overrides will have been decided against a different clock than the one the
	// client is told about.
	Now time.Time
	// Persist reports whether writes go to config.json rather than the browser session.
	Persist bool

	// Session is what the reader has arranged and corrected, carried through so the page
	// can draw its own controls from it. Zero for a caller with no session, which is
	// every caller but the HTTP one.
	Session Session
	// Optimised and Saved describe how the fifteen was chosen. See Session.
	Optimised bool
	Saved     bool
}'''),
])

edit('internal/viewmodel/build.go', [
    ('''		Session: Session{
			Store:    "session",
			Writable: p.Token != "",
		},''',
     '''		Session: Session{
			Store:     "session",
			Writable:  p.Token != "",
			Locked:    in.Session.Locked,
			Blocked:   in.Session.Blocked,
			Chips:     in.Session.Chips,
			Saved:     in.Saved,
			Optimised: in.Optimised,
		},'''),
])

# ---------------------------------------------------------------- the session
s = io.open('cmd/armband/session.go', encoding='utf-8').read()
s += '''
// arrangement is what the client needs to draw its own controls: which players this reader
// has pinned or barred, and which chips they have placed.
//
// Deliberately not the whole session. The fifteen and the eleven reach the client through
// the SQUAD, already resolved to element ids and already arranged -- sending them twice
// would be two representations of one thing, and the client would have to decide which to
// believe.
func (s session) arrangement() viewmodel.Session {
	return viewmodel.Session{
		Locked:  s.Lock,
		Blocked: s.Exclude,
		Chips:   s.Chips,
	}
}
'''
s = s.replace('''	"armband/internal/analysis"
	"armband/internal/config"
)''', '''	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/viewmodel"
)''')
io.open('cmd/armband/session.go', 'w', encoding='utf-8').write(s)

# ---------------------------------------------------------------- serve.go
edit('cmd/armband/serve.go', [
    ('func (s *squadServer) effectiveCfg(r *http.Request) config.Config {\n\treturn s.effectiveCfgFrom(readSessionOverrides(r))\n}',
     'func (s *squadServer) effectiveCfg(r *http.Request) config.Config {\n\treturn s.effectiveCfgFrom(readSession(r))\n}'),
])

print('aligned')
