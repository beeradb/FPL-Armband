// Package viewmodel is the contract between the model and every client that draws it.
//
// # Why this package exists at all
//
// There are three hosts for one application: `armband serve` over HTTP, a Wails desktop
// build binding to Go directly, and a website. If the shape a client reads were defined
// by an HTTP handler, the Wails build would have to restate it, and two statements of one
// shape is this codebase's most expensive recurring bug. So the contract is a package of
// plain structs with no transport in it. serve marshals them; Wails returns them; the
// website marshals them from somewhere else. None of them gets a vote on the fields.
//
// # The governing rule: nothing here computes a model quantity
//
// Every number in State is read off analysis.Squad, analysis.PlayerMetrics,
// present.Watchlist or config. This package arranges; it does not derive. The rule is
// inherited from internal/present, whose package comment says the same thing for the same
// reason, and it is what lets the client be dumb: if a figure is not in this struct, the
// client does not get to work it out, it gets added here.
//
// The one place that needs care is the difference between arranging and deriving. Summing
// the bench players' reported Score is arranging — the numbers are already on the page.
// Deciding a role band from expected minutes would be deriving, and that is why Role is a
// passthrough of analysis.rotationLabel rather than a switch in JavaScript.
//
// # Why it currently reads present.Page
//
// The page assembly — the override precedence, the watchlist's benchmarks, the research
// groups — lives in cmd/armband/page.go and hands internal/present a flat Page. Rather
// than fork that assembly, Build translates the Page that already exists. That makes this
// package depend on the HTML renderer it is replacing, which is backwards, and it is
// temporary: when present's page half retires, the assembly and the view types move here
// and the import goes. The translation below does not change when that happens.
package viewmodel

import "time"

// State is everything one client needs to draw the application once.
//
// It is a single document rather than a set of endpoints because the client renders every
// panel from one consistent snapshot. Two fetches would let the pitch and the market
// disagree about the same player, and the page's whole claim is that it shows its
// working.
type State struct {
	// Now is the server's clock, and the client must use it rather than the browser's.
	// Every staleness figure in Overrides was computed against this instant; a client
	// that reached for Date.now() would age an override against a different clock than
	// the one that flagged it, and the two would disagree by a day at midnight.
	Now time.Time `json:"now"`

	// Horizon is how many gameweeks the squad was optimised over. It is the answer to
	// "over what?" for every projection here, and the design's voice rule forbids
	// phrasing it as a fixed number of weeks in copy — so the client is given the value
	// rather than a sentence.
	Horizon int `json:"horizon"`

	Gameweeks []Gameweek `json:"gameweeks"`
	Squad     Squad      `json:"squad"`
	Market    Market     `json:"market"`
	Overrides Overrides  `json:"overrides"`
	Blind     []string   `json:"blind"`
	Policy    Policy     `json:"policy"`

	// Clubs is every club short name in the bootstrap, for the market's filter. The
	// client holds the club COLOURS, which are design data rather than model output;
	// this list is what tells it which clubs exist.
	Clubs []string `json:"clubs"`

	// Session reports where a change the reader makes will be stored, so the page can
	// say so instead of the reader finding out later. See Session.
	Session Session `json:"session"`
}

// Gameweek is one entry on the rail.
//
// The rail is data-driven and deliberately unbounded: HANDOFF.md forbids hard-coding a
// window ("don't hard-code five"), because the planning horizon slides as the season
// advances and a fixed count would start lying in May.
type Gameweek struct {
	Number   int       `json:"gw"`
	Deadline time.Time `json:"deadline"`
	// Current marks the gameweek being played or next to be played — the rail's NOW
	// dot. Exactly one gameweek carries it, or none once the season is over.
	Current bool `json:"current"`
	// Chip is the chip the plan puts in this week: "Wildcard", "Free Hit",
	// "Bench Boost", "Triple Captain", or empty. Spelled as the engine spells it.
	Chip string `json:"chip,omitempty"`
	// Projected is what the eleven fielded that week is expected to return, under
	// whatever chip is planned for it. Zero outside the weeks the build computed.
	Projected float64 `json:"projected,omitempty"`
	// Formation is the shape that week wants, which is not always this week's shape.
	Formation string `json:"formation,omitempty"`
	// Rebuilt marks a week whose fifteen is a fresh squad for a wildcard or free hit
	// rather than the one you own. The distinction matters to a reader: a free hit's
	// fifteen is handed back the following week and a wildcard's is kept.
	Rebuilt bool `json:"rebuilt,omitempty"`
}

// Squad is the fifteen and how it lines up.
//
// XI and Bench are ids rather than repeated players, because the same player must not
// appear twice in one document: two copies is two things to keep in step, and the client
// mutates the eleven as the reader drags cards around.
type Squad struct {
	Players []Player `json:"players"`

	XI    []int `json:"xi"`
	Bench []int `json:"bench"`
	// Captain and Vice are element ids. Vice is 0 when none is set — which was a real
	// bug once: the replay forfeited the armband entirely when the captain did not
	// play and no vice existed, so the absence is worth carrying explicitly.
	Captain   int    `json:"captain"`
	Vice      int    `json:"vice"`
	Formation string `json:"formation"`

	// XIScore is the plain sum of the eleven, and Expected adds the armband. Both come
	// off analysis.Squad; the client must not add the captain twice.
	XIScore  float64 `json:"xi_score"`
	Expected float64 `json:"expected"`

	// BenchScore is the plain sum of the four bench players' reported scores — what a
	// bench boost would pay, and what the design's "not counting" cell shows.
	//
	// ⚠️ It is NOT the bench's value to the optimiser. That figure weights each slot by
	// the probability it is ever used, which is far smaller and answers a different
	// question ("what is this bench worth to me?" rather than "what would it pay if it
	// all counted?"). Do not present one as the other.
	BenchScore float64 `json:"bench_score"`

	Cost float64 `json:"cost"`
	// Bank is the money left, in millions.
	Bank float64 `json:"bank"`
	// ClubCounts is players held per club, for the max-three rule the pitch enforces.
	ClubCounts map[string]int `json:"club_counts"`
}

// Player is one footballer as every surface draws him: the pitch card, the bench strip,
// the market row and the detail sheet all read this.
type Player struct {
	// ID is the season-scoped element id and is what the client addresses him by.
	// Code is the permanent FPL code and is what any WRITE must be keyed on — element
	// ids are reassigned every summer, so an override stored against one comes back in
	// August attached to a different footballer.
	ID   int `json:"id"`
	Code int `json:"code"`

	Name  string  `json:"name"`
	Club  string  `json:"club"`
	Pos   string  `json:"pos"`
	Price float64 `json:"price"`

	// XP is the headline: expected points per gameweek over the horizon, after
	// fixtures, minutes risk and availability. Per90 is the same before the minutes
	// model is applied, which is what the detail sheet shows the arithmetic from.
	XP    float64 `json:"xp"`
	Per90 float64 `json:"per90"`

	// Minutes is expected minutes per gameweek, and Role is the band the model puts
	// that number in. Role is sent rather than derived because the thresholds belong to
	// analysis.rotationLabel, and the design speaks in roles while the evidence stays
	// the minute count — so the client needs both and must invent neither.
	//
	// ⚠️ There are FIVE bands, not the four HANDOFF.md lists: nailed, likely starter,
	// rotation risk, squad player, fringe. The client must render whatever arrives.
	Minutes     float64 `json:"minutes"`
	Role        string  `json:"role"`
	Reliability float64 `json:"reliability"`
	StartShare  float64 `json:"start_share"`

	Ownership float64 `json:"ownership"`

	// Status is FPL's availability letter and News is its explanation. News is the
	// only free text here that comes from outside this program.
	Status string `json:"status,omitempty"`
	News   string `json:"news,omitempty"`
	// Availability is the multiplier Status produces. Carried rather than inferred:
	// re-deriving it client-side would be a second implementation of a table that
	// already exists, and its most important value is 0 — a ruled-out player, whose
	// score is zero for that reason and no other.
	Availability float64 `json:"availability"`

	Fixtures []Fixture `json:"fixtures"`
	// AvgDifficulty is the mean FDR over the horizon, already computed.
	AvgDifficulty float64 `json:"avg_fdr"`

	// Override is the standing correction acting on this player, if any.
	Override *Override `json:"override,omitempty"`
}

// Fixture is one upcoming match, as the FDR strip draws it.
type Fixture struct {
	Gameweek   int    `json:"gw"`
	Opponent   string `json:"opp"`
	Home       bool   `json:"home"`
	Difficulty int    `json:"fdr"`
}

// Override is a human correction to the model, as the Overrides tab draws it.
//
// Every field is carried pre-decided: whether it needs a re-check, how old it is, whether
// it ever WAS checked. The staleness rule lives in internal/config and the client does not
// get a copy of it — a second implementation of "is this override still live" is exactly
// the failure this project has paid for most often, and the design's own note that the
// rule is fourteen days is already wrong (it is seven).
type Override struct {
	// Kind is lock, lockXI, exclude, minutes or club. Code is 0 for a club override,
	// which has no player to write against.
	Kind string `json:"kind"`
	Code int    `json:"code,omitempty"`

	// Session marks an override this browser session set, rather than one read from
	// config.json. It is the difference between a control the reader can clear and a
	// standing decision he must change in config — so it drives whether the delete
	// button is live, and the page says which store it is looking at.
	Session bool `json:"session"`

	// Label always carries the VALUE, never just the kind: "MIN 88" and "MIN 15" are
	// opposite interventions and a badge reading "MIN" tells the reader nothing.
	Label  string `json:"label"`
	Reason string `json:"reason"`

	Player string `json:"player,omitempty"`
	Club   string `json:"club,omitempty"`
	Pos    string `json:"pos,omitempty"`

	SetOn   string `json:"set_on,omitempty"`
	Until   string `json:"until,omitempty"`
	Checked string `json:"checked,omitempty"`

	// NeedsCheck is the only state here with an action attached. CheckAge is its age in
	// days, carried separately so the flag can prioritise: nine identical CHECK badges
	// cannot, which is what a flag is for.
	NeedsCheck   bool `json:"needs_check"`
	CheckAge     int  `json:"check_age"`
	NeverChecked bool `json:"never_checked,omitempty"`
	// Lapsed marks one past its gameweek. Kept rather than dropped, because deleting it
	// would hide that a player's treatment CHANGED.
	Lapsed bool `json:"lapsed,omitempty"`
	// Inherited marks a club-level correction riding on a player's row, drawn dashed so
	// the reader can see it is not about him.
	Inherited bool `json:"inherited,omitempty"`
	// InSquad marks one acting on a player actually in the fifteen — the distinction
	// that decides whether an override is doing something now or is a note.
	InSquad bool `json:"in_squad,omitempty"`
	// Affects names the members of the fifteen a club override touches. Empty is a real
	// answer, "it reaches nobody you own", and the client must say so rather than
	// rendering nothing: a silent absence reads as the page having forgotten to ask.
	Affects []string `json:"affects,omitempty"`
	// Flag is the badge text, rendered here because it encodes the never-verified case
	// that CheckAge alone cannot express.
	Flag string `json:"flag,omitempty"`
}

// Overrides is the tab: what is binding now, and what has run out.
type Overrides struct {
	Live   []Override `json:"live"`
	Lapsed []Override `json:"lapsed"`
	// Due is how many live overrides need a re-check and Oldest names the worst, stated
	// once at the top because a flag that is on for every card cannot prioritise.
	Due    int    `json:"due"`
	Oldest string `json:"oldest,omitempty"`
}

// Market is the Players tab: everyone worth considering, measured against the man they
// would actually displace.
type Market struct {
	Rows []MarketRow `json:"rows"`
	// Benchmarks is the weakest starter per position — the player a transfer in that
	// position has to beat. One per position rather than repeated on every row.
	Benchmarks []Benchmark `json:"benchmarks"`
	// Excluded are players a standing override keeps out of every squad. They are NOT
	// candidates and are NOT in Rows: they are decisions, and the reason is carried in
	// full, because "why is this obviously good player not here" is the first question
	// a reader asks of this list.
	Excluded []Override `json:"excluded"`

	// Count is how many candidates there are and Clearing how many reach the gate.
	// Gate is the threshold itself. All three are here so the page can answer whether
	// any of this is actionable, rather than leaving the reader to compare a column of
	// deltas against a number printed on another tab.
	Count    int     `json:"count"`
	Clearing int     `json:"clearing"`
	Gate     float64 `json:"gate"`
}

// MarketRow is one candidate.
type MarketRow struct {
	Player Player `json:"player"`
	// Delta is against the weakest starter in his position, not against a rank. "The
	// eighth best midfielder" answers nothing; the gap on the man you are currently
	// starting is the whole decision.
	Delta float64 `json:"delta"`
	// ClearsGate reports whether Delta reaches the threshold the policy actually
	// applies. It is sent rather than compared client-side because colouring the gap
	// against ZERO was the page recommending in colour what it refused in prose.
	ClearsGate bool `json:"clears_gate"`
}

// Benchmark is the weakest starter in one position. The price rides along with the score
// because the delta column answers half the question and the reader was left doing the
// subtraction by hand.
type Benchmark struct {
	Pos   string  `json:"pos"`
	Name  string  `json:"name"`
	Score float64 `json:"score"`
	Price float64 `json:"price"`
}

// Policy is the gate the transfer decision is made under, so "no move" reads as a
// decision with thresholds behind it rather than an empty panel.
type Policy struct {
	MinGainTransfer float64  `json:"min_gain_transfer"`
	MinGainHit      float64  `json:"min_gain_hit"`
	BankUpTo        int      `json:"bank_up_to"`
	MaxHitsPerWeek  int      `json:"max_hits_per_week"`
	Rules           []string `json:"rules,omitempty"`
}

// Session tells the client where a change it makes will land.
//
// The design ships copy promising that work is saved, and until this shipped that copy
// was false — the prototype had no persistence at all. Rather than hard-code a sentence,
// the client is told which store is live and says so.
type Session struct {
	// Store is "session" or "config". In session mode a change lives in a browser
	// cookie and dies with the browser; in config mode it is written to config.json and
	// binds every future run, including the agent's.
	Store string `json:"store"`
	// Writable reports whether the client may write at all.
	Writable bool `json:"writable"`
}
