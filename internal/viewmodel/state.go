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

	// News is the News tab: freshness of the two sources it draws from, and every
	// row it has to show. See News.
	News News `json:"news"`

	// Import is the team-import affordance's whole state. See Import.
	Import Import `json:"import"`

	// Results is one entry's squad for one gameweek, run through buildResults — the site's
	// own (config.EntryID, on /armband-team) or a reader's own imported squad (session.Entry,
	// on GET /api/results). Nil when there is no entry to describe, since there is then no
	// real team to show. See Results.
	Results *Results `json:"results,omitempty"`

	// Transfers is what has changed since the reader's imported baseline, and what it
	// would cost at FPL. Present only when the session carries an imported entry
	// (session.Entry != 0); absent otherwise, which is how the client knows there is
	// nothing to draw. See Transfers.
	Transfers *Transfers `json:"transfers,omitempty"`
}

// Transfers is the transfer bar and panel's whole state: the free-transfer allowance, what
// the reader's current pitch has changed against the fifteen they imported, and what that
// would cost at FPL if they made it for real. Built in cmd/armband's buildState, from
// session.Base/session.Squad and one FPL history read — see that function's own comment.
//
// A transfer is position-matched diff(Base, Squad), out-for-in, computed from two lists of
// permanent codes already in the session cookie: no network call, no search, and no opinion
// about whether a move is a good idea — that is Suggest's job (GET /api/transfers), which
// this type has nothing to do with.
type Transfers struct {
	// Free is fpl.FreeTransfers' reconstruction, or fpl.UnlimitedTransfers (-1) before
	// the first deadline. Never a guess of 1 — see FreeTransfers' own doc comment for why
	// guessing low is the expensive direction.
	Free int `json:"free"`
	// FreeUnknown is set when the history read failed. The count and the cost are then
	// both withheld and the reason is said out loud, per this project's own rule that a
	// capability which vanishes quietly is the failure being prevented.
	FreeUnknown bool `json:"free_unknown,omitempty"`

	// Moves is Squad against Base, position-matched, out-for-in. Absent when there is
	// nothing to diff against (NoBaseline) or the diff is not trustworthy (BaselineStale).
	Moves []Move `json:"moves,omitempty"`
	// Hits is max(0, len(Moves)-Free), zero when Free is unlimited or unknown.
	Hits int `json:"hits,omitempty"`
	// Cost is Hits * fpl.HitCost.
	Cost int `json:"cost,omitempty"`

	// BaseEvent is the gameweek Base was fetched for.
	BaseEvent int `json:"base_event,omitempty"`
	// BaselineStale: FPL's current event has moved past BaseEvent, so the reader's real
	// squad may have changed and this diff describes a week that is over. The count and
	// cost are withheld rather than shown against a baseline this build no longer trusts.
	BaselineStale bool `json:"baseline_stale,omitempty"`
	// NoBaseline: nothing on record to diff against.
	NoBaseline bool `json:"no_baseline,omitempty"`
	// FreeHitBase says NoBaseline is because the imported week was a free hit, which is a
	// different sentence from "you have not imported".
	FreeHitBase bool `json:"free_hit_base,omitempty"`
}

// Move is one player leaving and one arriving, either from the reader's own baseline diff
// (Transfers.Moves) or from a suggested plan (GET /api/transfers). Every field is already
// resolved for display — the client renders it and computes nothing.
type Move struct {
	Pos      string  `json:"pos"`
	OutCode  int     `json:"out_code"`
	OutName  string  `json:"out_name"`
	OutClub  string  `json:"out_club"`
	OutPrice float64 `json:"out_price"`
	InCode   int     `json:"in_code"`
	InName   string  `json:"in_name"`
	InClub   string  `json:"in_club"`
	InPrice  float64 `json:"in_price"`
}

// Results is one entry's actual gameweek result — /armband-team's whole document for the
// site's own squad, and GET /api/results' whole document for a reader's own imported one.
// The page's tense is "what happened", never "what might happen" — see ResultState.
//
// Nothing in this type says whose entry it is; the identity lives entirely in which entry
// id the caller fetched with (config.EntryID or session.Entry), never in a field here — see
// buildResults's own comment.
//
// ResultEvent/ResultState/EventAverage all describe the SAME gameweek: the one the caller
// chose (latestClosedEvent for the spectator page, the requested ?gw= for the results
// route), the one whose picks are on the pitch and whose live stats are on the cards. These
// replace CurrentEvent/CurrentProjected, which are gone from the contract entirely rather
// than merely stopped being rendered: CurrentEvent came from the rail's own Current
// gameweek while the pitch, the live stats and the match statuses all came from a chosen
// closed gameweek, and those are routinely different gameweeks the moment a deadline has
// passed but the rail has not rolled over — so the old footer was routinely labelling one
// week's projection next to another week's actual eleven. Leaving the fields in the
// contract, even unrendered, would leave the trap for the next reader.
type Results struct {
	OverallPoints int `json:"overall_points"`
	OverallRank   int `json:"overall_rank,omitempty"`

	// ResultEvent is the gameweek this page describes, chosen server-side — by
	// latestClosedEvent on the spectator page, by the requested ?gw= on GET
	// /api/results. The client must not infer it from anything on the pitch.
	ResultEvent int `json:"result_event,omitempty"`

	// ResultState is "live" while any fixture in ResultEvent is short of
	// fpl.Fixture.Finished, "final" once every one is — computed in houseLiveSources,
	// which already holds the fixture list, and NEVER derived here or client-side from
	// the fifteen players' MatchStatus: the squad may not cover all twenty clubs, so a
	// club with no fixture status representation would leave the state a guess. Finished
	// is the correct bar rather than FinishedProvisional, because FPL sets Finished only
	// after bonus is applied — "final" means the scores on this page will not move again.
	//
	// ⚠️ Deliberately still two-value as of 2026-08-22, even though TeamPlayer.MatchStatus
	// gained a third state ("fulltime") the same day. This field answers "is the WHOLE
	// gameweek final", which a per-club fulltime/live split does not change — a third
	// value here would be a second vocabulary for the same fact. Do not "complete" it to
	// match MatchStatus; see that field's own comment for the distinction it carries
	// instead.
	ResultState string `json:"result_state,omitempty"`

	// EventAverage is fpl.Event.AverageScore for ResultEvent — every FPL manager's mean
	// score that week, already parsed off Boot with no new fetch. It is what makes a bare
	// points total mean something.
	EventAverage int `json:"event_average,omitempty"`

	// History is every gameweek FPL has scored so far, oldest first — the actual points
	// the fielded eleven returned, not a projection.
	History []Result `json:"history,omitempty"`

	// LivePoints is this gameweek's score summed from the squad while the gameweek is
	// still being played, and it is set ONLY while ResultState is "live".
	//
	// It exists because FPL's own EntryHistory reports Points: 0 for a gameweek it has
	// not finished scoring, so the figure this page's headline is actually about does
	// not exist yet in the place the finished-gameweek figure comes from. Showing that
	// 0 above eleven cards that each show points is the defect this replaces (caught
	// live 2026-08-22, result_state "live", history[last].points 0 against an XI
	// visibly scoring).
	//
	// Summed as Points * Multiplier over XI and Bench together, minus the gameweek's
	// transfer-hit cost (Result.Hit) so the live figure and the settled figure
	// mean the same thing: a benched player's multiplier is 0, so the bench
	// contributes nothing without needing a second rule, and the captain's double is
	// already carried by his own multiplier.
	//
	// ⚠️ It does NOT model auto-substitutions, and it cannot: FPL applies those after
	// the gameweek finishes, so during play there is no fact to report. A starter who
	// ends on zero minutes is worth zero here and may be worth more once FPL settles.
	// That makes this a floor, not a forecast, which is the honest shape for a live
	// figure — the LIVE chip beside the gameweek name already tells the reader the
	// number is still moving, so this field carries no second label for it.
	LivePoints int `json:"live_points,omitempty"`

	// Formation, Captain and Vice describe the same fifteen XI/Bench name, by ID — this
	// result's own pitch (the dedicated /armband-team page's, or GET /api/results'),
	// entirely separate from Squad above. It exists because both are a spectator view, not
	// the interactive builder: no locks, no leave-outs, no role band, no reliability
	// meter. See TeamPlayer.
	Formation string       `json:"formation,omitempty"`
	Captain   int          `json:"captain,omitempty"`
	Vice      int          `json:"vice,omitempty"`
	XI        []TeamPlayer `json:"xi,omitempty"`
	Bench     []TeamPlayer `json:"bench,omitempty"`
}

// Result is one completed gameweek's actual result — not just the points, but the
// context that makes a bare total mean something: where it ranked that week, whether a
// hit paid for it, and how many points sat unused on the bench. Rank/Hit/BenchPoints are
// already parsed off fpl.EntryHistory.Current, so this is a copy, not a second fetch.
type Result struct {
	Event  int `json:"event"`
	Points int `json:"points"`
	// Rank is that gameweek's rank among all FPL managers, nil if FPL has not settled
	// it yet (the same "absence over a wrong assertion" rule the rest of this package
	// follows).
	Rank *int `json:"rank,omitempty"`
	// Hit is EventTransfersCost — points given up for extra transfers that gameweek.
	// Rendered as a negative on the page; stored here as FPL's own positive cost.
	Hit int `json:"hit,omitempty"`
	// BenchPoints is PointsOnBench — what the fielded eleven left unused. The number
	// every FPL manager looks for after a big bench score.
	BenchPoints int `json:"bench_points,omitempty"`
}

// TeamPlayer is one player on a results spectator pitch — the site's own team or a
// reader's own imported one. Deliberately a smaller
// vocabulary than Player: no Role, no Reliability, no Override, no Availability — those
// describe a decision the interactive builder is helping a reader make, and a spectator
// page reader is not making one. Opponent and this season's counting stats instead: what
// he is up against, and what he has actually done.
type TeamPlayer struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Club  string  `json:"club"`
	Pos   string  `json:"pos"`
	Price float64 `json:"price"`

	// Opponent is the fixture the club actually played in ResultEvent -- the gameweek
	// this page reports on -- never the model's forward-looking "next fixture". They
	// diverge the moment ResultEvent's own deadline has passed: the model's fixture
	// window is anchored on the NEXT planning gameweek (see analysis.Engine's own
	// fromEvent), so during and after a live gameweek the upcoming fixture is a
	// DIFFERENT match from the one that earned the goals, assists and points on this
	// same card, under a header naming the gameweek that was actually played (found
	// live 2026-08-22: ResultEvent 1, opponent chip showing GW2's fixture). Computed
	// server-side in cmd/armband.houseLiveSources from the same fixture list that
	// decides MatchStatus, so the chip and liveDot beside it always describe the same
	// match. Nil when the club had no fixture in ResultEvent (a blank gameweek) --
	// never a fallback to the next one, which is exactly what produced the bug. On a
	// genuine double gameweek, the earlier kickoff wins; this page draws one chip, not
	// two.
	Opponent *Fixture `json:"opponent,omitempty"`

	// MatchStatus is his club's fixture in the gameweek this page is showing:
	// "scheduled", "live", "fulltime" or "finished". Empty before a season has a
	// current gameweek at all (see buildResults) — there is then nothing to
	// report a status about. Together with Minutes this decides the card's state
	// — see team.js cardState, the ONE place that derivation happens.
	//
	// "fulltime" (added 2026-08-22) is the gap between fpl.Fixture's own
	// FinishedProvisional and Finished: the match has been played out and its
	// score locked in, but FPL has not yet applied and checked bonus. That
	// window is real and often long (see cmd/armband.houseLiveSources), and
	// collapsing it into "live" is what put an "in progress" dot and a
	// "provisional" asterisk on matches that had already ended — caught live
	// 2026-08-22 for five of six gameweek-one fixtures at once. "fulltime"
	// behaves like "live" everywhere on the card EXCEPT the live dot, which
	// answers a narrower question ("is this being played right now") that
	// "fulltime" is specifically the "no" answer to — see cardState and liveDot.
	MatchStatus string `json:"match_status,omitempty"`

	// Minutes is this gameweek's minutes. Not rendered as a figure — it is the one
	// fact that separates "finished, did not play" from "finished, played, earned
	// nothing", which are different cards.
	Minutes int `json:"minutes"`

	// Points is this gameweek's total_points for this player, UNMULTIPLIED — the raw
	// FPL figure, never his contribution to the team score. The captain's doubling is
	// Multiplier's job and the client's arithmetic, the same separation app.js already
	// keeps for the projected figure.
	Points int `json:"points"`

	// Multiplier is his FPL pick multiplier for this gameweek: 0 on the bench, 1 in
	// the XI, 2 as captain, 3 under Triple Captain. Sourced from fpl.Pick.Multiplier
	// via arrangement.Mult (see picksToFixed, keyed by permanent code the same way
	// XI/Bench/Captain are) and never derived client-side from the Captain id —
	// deriving it would silently render a Triple Captain week as a double.
	Multiplier int `json:"multiplier"`

	// Bonus is FPL's settled bonus for this gameweek, already inside Points. Zero
	// until FPL applies it, which is the same moment MatchStatus becomes "finished" —
	// see houseLiveSources.
	Bonus int `json:"bonus"`

	// Goals, Assists, CleanSheets and the rest of this gameweek's counting stats are
	// an honest "as of now" at every status — zero before kickoff, live during the
	// match, final once FPL finishes scoring it. Never a season total: a card that
	// said "9 goals" before a ball had been kicked this season was the bug an earlier
	// version of this page shipped with.
	Goals       int `json:"goals"`
	Assists     int `json:"assists"`
	CleanSheets int `json:"clean_sheets"`

	GoalsConceded   int `json:"goals_conceded"`
	YellowCards     int `json:"yellow_cards"`
	RedCards        int `json:"red_cards"`
	OwnGoals        int `json:"own_goals"`
	PenaltiesSaved  int `json:"penalties_saved"`
	PenaltiesMissed int `json:"penalties_missed"`

	// DefCon is this gameweek's defensive-action count and DefConReached is
	// whether it has cleared analysis.DefConThreshold for his position — the
	// same all-or-nothing bar the model prices (see that function's own
	// comment), never a partial-credit read of a bar that has not been
	// cleared. Both nil for a goalkeeper, who FPL does not score on defensive
	// actions at all (see Saves), and both nil until his match has kicked off —
	// zero DC before kickoff is a fact, not yet an answer to "did he clear it".
	DefCon        *int  `json:"def_con,omitempty"`
	DefConReached *bool `json:"def_con_reached,omitempty"`

	// Saves is this gameweek's save count. Goalkeepers only — nil for an
	// outfielder, the same way DefCon is nil for a goalkeeper.
	Saves *int `json:"saves,omitempty"`

	// Breakdown is how Points was earned, in FPL's own scoring order, one entry
	// per non-zero component, plus a trailing "Captain (×N)" line when
	// Multiplier doubles or triples it. Computed server-side, in
	// scoreBreakdown, for the same reason LivePoints is: the client renders
	// decided numbers and never derives a scoring quantity itself — see the
	// Import type's own governing comment.
	//
	// Nil whenever the computed channels do not sum EXACTLY to Points (logged
	// to stderr when that happens, at runtime) — a breakdown that disagrees
	// with the number above it is worse than no breakdown, so it is withheld
	// rather than shown wrong. Also nil before a position is known (no boot) or
	// before a match has produced any non-zero channel (nothing to explain
	// yet).
	//
	// Rendered in team.js as the title on .tppts. ⚠️ Unreachable on a touch
	// device, the same limitation Defect 3's asterisk legend exists to partly
	// answer for a DIFFERENT mark — title has no touch equivalent at all, and
	// this ships anyway because the owner asked for the hover specifically;
	// a tap-target follow-up is a separate decision, not solved here.
	Breakdown []ScoreLine `json:"breakdown,omitempty"`
}

// ScoreLine is one component of a player's gameweek score: what he did, and what
// FPL paid for it. Points may be negative (a card, an own goal, a missed
// penalty, goals conceded). Detail is a short pre-formatted clarifier — "2 × 6",
// "12 CBIRT" — empty when the label and points already say everything ("Bonus",
// a single clean sheet). See scoreBreakdown for how this is built and why it is
// never a second implementation of FPL's scoring table.
type ScoreLine struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Points int    `json:"points"`
}

// Import is the team-import affordance's whole state — whether it may be offered right
// now, which gameweek's picks it would fetch, and what this reader has already done about
// it.
//
// The client draws the control iff Open, and never itself decides whether a gameweek has
// been played — that is exactly the kind of derivation this package's own governing rule
// forbids the client from doing. See cmd/armband.importWindow for the rule (FPL only
// serves a gameweek's picks once its deadline has passed, which doubles as "is the feature
// on") and why it lives in exactly one place.
type Import struct {
	Open bool `json:"open"`
	// Event is the gameweek whose picks would be imported — the one just gone. Zero
	// when Open is false.
	Event int `json:"event,omitempty"`
	// Next is the gameweek being planned — the one the imported squad would be built
	// for. Zero when Open is false.
	Next int `json:"next,omitempty"`
	// Skipped reports that this reader has already chosen "start fresh", so the client
	// should not offer the control again for this session.
	Skipped bool `json:"skipped,omitempty"`
	// Entry is the FPL entry (Team) id this reader has already imported, 0 if none.
	Entry int `json:"entry,omitempty"`
}

// News is the News tab's own data, kept apart from Player so a page that has not
// opened the tab does not carry it, and so the tab's two clocks live beside the
// items they describe rather than beside the squad.
type News struct {
	// Checked is a pre-formatted freshness line for FPL's own status feed —
	// "FPL status last read 3 minutes ago · next read at 15:00". Server-formatted
	// for the reason every other staleness figure in this contract is: the client
	// has no honest way to compute "ago" against a clock it does not share with
	// the server, and app.js's own comment on needsCheck/age/flag makes the same
	// argument at length. Empty when nothing has been fetched yet.
	Checked string `json:"checked,omitempty"`

	// ReadChecked is the same idea for the team news a human (soon, an automated
	// process) has read and recorded — "Team news last read 3 days ago" — on its
	// own, separate and much less frequent cadence. Two sources, two clocks, so
	// one string cannot honestly describe both.
	//
	// ⚠️ Last-read only, deliberately coarser than the illustrative "41 minutes
	// ago" the design was drawn against: the store behind this (config's roster
	// corrections) keeps a DATE, not a timestamp, so this reads in days. Widening
	// it to minutes would need every writer of a roster correction to start
	// saving a full timestamp instead of a date, which is a real change this pass
	// did not make. There is also no "next read at" half: nothing schedules the
	// reading process yet, so a predicted next read would be a promise this
	// system cannot keep. Add both refinements together, when the process is
	// automated and hands them the resolution to match.
	ReadChecked string `json:"read_checked,omitempty"`

	// Items is every row the tab draws: FPL's own status feed and the team news
	// that has been read and recorded, in one shape so the client is not
	// stitching two lists into one row itself. Grouping and sorting THIS list is
	// the client's job — the design's own rule for the rotation band applies here
	// too, filtering and sorting a list the server sent is not derivation.
	Items []NewsItem `json:"items,omitempty"`
}

// NewsItem is one row: something a source said, when, and — when it can be
// computed honestly — what it changed about the model's own number.
type NewsItem struct {
	// Source is who says so: "FPL" for the status feed, "REPORTED" for team news
	// a human has read and recorded. It carries no severity of its own — FPL's
	// status IS a risk and REPORTED is an input the model already consumed, not a
	// human overruling it, so the source is a label rather than a channel.
	Source string `json:"source"`

	Player     string `json:"player,omitempty"`
	PlayerCode int    `json:"player_code,omitempty"`
	Club       string `json:"club,omitempty"`
	Pos        string `json:"pos,omitempty"`

	// Body is what was said: FPL's own News string, or a correction's Reason.
	Body string `json:"body"`
	// When is a pre-formatted fragment for this one row — "set 2026-08-08" or
	// "checked 3d ago" — never a raw timestamp the client would format itself.
	When string `json:"when,omitempty"`
	// Flag is FPL's status letter for a status row (i, d, s, u), empty for a
	// reported row — REPORTED carries no channel of its own, so there is nothing
	// to flag.
	Flag string `json:"flag,omitempty"`

	// Effect is the before/after this input causes to the model's own number,
	// when one exists and can be computed honestly — a real model quantity, never
	// derived client-side. Absent, not zero, for a row with no comparable
	// before/after: a lock or an exclude changes which SQUAD is built, not this
	// player's own score, so there is nothing here to report.
	Effect *Effect `json:"effect,omitempty"`
}

// Effect is a before/after a model input causes, all three strings
// pre-formatted server-side because the comparison itself is a model quantity.
type Effect struct {
	Label string `json:"label"`
	Was   string `json:"was"`
	Now   string `json:"now"`
	// Direction is "up", "down" or "flat", so the client can colour the change
	// without re-deriving which way it went from two formatted strings.
	Direction string `json:"direction"`
}

// Gameweek is one entry on the rail.
//
// The rail is data-driven and deliberately unbounded: HANDOFF.md forbids hard-coding a
// window ("don't hard-code five"), because the planning horizon slides as the season
// advances and a fixed count would start lying in May.
type Gameweek struct {
	Number   int       `json:"gw"`
	Deadline time.Time `json:"deadline"`
	// Current marks the next gameweek a reader can still act on — the rail's NOW dot.
	// Exactly one gameweek carries it, or none once the season is over. Found from each
	// gameweek's own deadline against the server's clock, not FPL's IsCurrent/IsNext
	// flags — see buildGameweeks's own comment for why those lag a real deadline.
	Current bool `json:"current"`
	// Closed reports that this gameweek's deadline has passed — selection is locked, so
	// the interactive builder has nothing left to offer for it. A closed gameweek only
	// appears in this list at all when the reader has imported their real picks (see
	// State.Import); otherwise buildGameweeks drops it rather than showing a stale
	// hypothetical plan for a squad that can no longer change.
	Closed bool `json:"closed,omitempty"`
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

	// Playable lists the chips the competition allows in this gameweek, by key. It is
	// sent per week rather than assumed, because it is not the same every week: gameweek
	// one offers only the bench boost and the triple captain.
	//
	// The client draws its chip row from this. It must not decide for itself which chips
	// exist in a week — that is a rule about the competition, and a second copy of it here
	// would disagree with the model the first time either changed.
	Playable []ChipOption `json:"playable,omitempty"`

	// ChipWindow is the chip window this gameweek falls in, when it falls in one
	// at all: its own last gameweek and how many of its four chips are still
	// genuinely unspent. See analysis.Engine.ChipWindowStatusAt for why this
	// cannot be the client's own arithmetic — chiefly that "unspent" has to
	// count a chip already played earlier in the window, which a rail of
	// current-and-upcoming weeks cannot see. The client's only arithmetic on
	// these fields is EndsGW − this gameweek's Number + 1, a subtraction of two
	// integers the server sent, not a rule about the competition.
	ChipWindow *ChipWindow `json:"chip_window,omitempty"`
}

// ChipWindow is one season chip window's state, attached to the gameweek that
// falls inside it.
type ChipWindow struct {
	// EndsGW is the window's last gameweek — 19, then 38.
	EndsGW int `json:"window_ends_gw"`
	// Size is how many chips this window grants. 4 today, sent rather than
	// hard-coded so the client's "n of 4 left" is not a constant it has to
	// change the day the competition does.
	Size int `json:"window_size,omitempty"`
	// Remaining is chips genuinely unspent in this window, counting ones
	// already played — the fix for app.js:625, which only ever looked at the
	// rail and so could not see a chip spent before the rail's own start.
	Remaining int `json:"remaining_in_window"`
	// EndsAt is a pre-formatted date for the window's deadline, e.g. "Tue 29
	// Dec" — absent when the deadline is not known.
	EndsAt string `json:"window_ends_at,omitempty"`
}

// ChipOption is one chip the reader may place in a gameweek.
type ChipOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`

	// Gain is the delta against playing no chip that week, in points per
	// gameweek — a genuine model quantity, never derivable client-side.
	//
	// ⚠️ NOT POPULATED by this build. The catalogue's projected-gain feature
	// was drawn, then shelved by the product owner on visual-load grounds
	// (NOTES.md §9.6) — "if it visually clutters things too much then don't
	// do it" — not on merit, so the contract gap this field closes is real
	// either way and the analysis should not have to be redone when the
	// feature comes back. A pointer rather than a bare float so an unpopulated
	// week reads as "not computed" and not as "no gain from this chip".
	//
	// The cost of populating it, for whoever picks this up: Bench Boost and
	// Triple Captain are cheap — both are already computed and printed in
	// prose by chipExplain() in app.js, so wiring them through is mostly
	// plumbing. Free Hit and Wildcard are not: each needs a full squad
	// re-optimisation against the market for that one week — an entire
	// alternative fifteen, discarded afterwards for the free hit — which is
	// the expensive half and was not attempted here. NOTES.md §6.6 also warns
	// against shipping only the cheap two: two numbered chips beside two
	// blank ones reads as "these are the good ones" on the one screen where
	// the reader is making a genuine choice, so a client should render this
	// field only once all four chips carry it.
	Gain *float64 `json:"gain,omitempty"`
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

	// ⚠️ ModelledMinutes was here and is REMOVED. It was Minutes pre-formatted as
	// "90 → 54 modelled", justified as saving the client from pasting a baseline,
	// an arrow and a word — but the client's own row template already draws that
	// arrow, so the two were one quantity in two shapes.
	//
	// It cost a live NaN. The client aliased the STRING into the slot its
	// arithmetic used, so Math.round() rendered "NaN" on the player card and in
	// the News tab's standing band, and two sorts silently compared strings.
	// Minutes is the number; there is now one of it.

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
	// ValueScore is XP per million spent — analysis.PlayerMetrics.ValueScore, copied
	// rather than divided client-side. `Per £m` used to be xpFor(p)/p.pr in app.js, one of
	// three client surfaces computing a model quantity; this closes it.
	ValueScore float64 `json:"value_score"`

	// Override is the standing correction acting on this player, if any.
	Override *Override `json:"override,omitempty"`

	// XG90 and XA90 are FPL's own expected figures per 90, blended across seasons
	// exactly as the rates the model scores are. They are NOT the points: scoring
	// multiplies them by the position's conversion scale. Carried so the market can
	// show the underlying rate a projection rests on, not only the projection.
	XG90 float64 `json:"xg_per_90"`
	XA90 float64 `json:"xa_per_90"`

	// DefConChance is the chance of the defensive-contribution award in a gameweek,
	// 0-1 — analysis.PlayerMetrics.DefConChance, copied. A pointer because nil means
	// the model does not price the term for this player, which is not zero.
	DefConChance *float64 `json:"defensive_contribution_chance,omitempty"`
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
	// standing decision he must change in config.
	//
	// ⚠️ Nothing reads it yet. The client renders the delete control on every override,
	// and the deletion removes a row from a JavaScript array — so a reader can delete a
	// config-sourced correction, watch it vanish, and find the model still applying it.
	// This field is here so that when the store is connected the control can be gated,
	// rather than the client re-deriving which store a correction came from.
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

// Session tells the client where a change it makes will be saved.
//
// The design ships copy promising that work is saved, and the prototype had no persistence
// at all. ⚠️ The client is sent this and does not yet render it, because nothing on the
// page reaches either store: a change lives in the page's memory and a reload discards it.
// It is carried so the sentence can be data rather than a literal once the session store is
// connected.
type Session struct {
	// Store is "session" or "config". In session mode a change lives in a browser
	// cookie and dies with the browser; in config mode it is written to config.json and
	// binds every future run, including the agent's.
	Store string `json:"store"`
	// Writable reports whether the client may write at all.
	Writable bool `json:"writable"`

	// Locked and Blocked are the players this reader has pinned into or barred from every
	// build, by permanent CODE. The client draws its control states from these rather than
	// keeping its own idea of them, so a reload shows what is actually in force instead of
	// an empty set that happens to look the same on a fresh page.
	Locked  []int `json:"locked,omitempty"`
	Blocked []int `json:"blocked,omitempty"`

	// Chips the reader has placed: gameweek number as a string, to a chip key. A JSON
	// object cannot have integer keys.
	Chips map[string]string `json:"chips,omitempty"`

	// Dismissed are the standing overrides this reader has cleared for the session, by
	// permanent CODE.
	//
	// ⚠️ It must be sent even though nothing DRAWS it. The client rebuilds its pending
	// session from this document on every load, so a field the document omits comes back
	// as empty and the next save writes that emptiness through — clear an override, move
	// one player, and the override returns. That is the same "a control that only changed
	// the page" defect this whole surface was written to remove, arriving by a different
	// route.
	Dismissed []int `json:"dismissed,omitempty"`

	// Saved reports that this document was built from a stored team rather than freshly
	// chosen, which is what lets the page say so honestly instead of implying the model
	// just picked it.
	Saved bool `json:"saved"`
	// Optimised reports that the fifteen is the model's best rather than a varied opening
	// squad. It drives whether the Optimize control reads as available or as already done.
	Optimised bool `json:"optimised"`

	// Base and BaseEvent round-trip session.Base/session.BaseEvent — the transfer
	// baseline — the same way Locked/Blocked/Chips/Dismissed round-trip the reader's
	// other corrections. PUT /api/session decodes straight into a fresh session{}, so a
	// save that omitted these would silently erase an already-imported baseline on the
	// very next unrelated save. See session.Base's own doc comment for what the fifteen
	// is and Transfers.BaseEvent for the gameweek it was fetched for.
	Base      []int `json:"base,omitempty"`
	BaseEvent int   `json:"basegw,omitempty"`
}
