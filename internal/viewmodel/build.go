package viewmodel

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
)

// Input is everything Build needs. It is a struct rather than six parameters because the
// callers are a request handler and, later, a Wails binding, and both should be able to
// see at the call site which of these they have forgotten.
type Input struct {
	// Page is the assembled squad page. See the package comment for why this package
	// currently reads the HTML renderer's view model rather than assembling its own.
	Page present.Page
	// Boot supplies the gameweek deadlines, which are the one thing the rail needs and
	// the page does not carry.
	Boot *fpl.Bootstrap
	Cfg  config.Config
	// Now must be the same instant the page was built with, or the staleness figures in
	// the overrides will have been decided against a different clock than the one the
	// client is told about.
	Now time.Time
	// Persist reports whether writes go to config.json rather than the browser session.
	Persist bool

	// Session is what the reader has arranged and corrected, carried through so the page
	// can draw its own controls from it rather than keeping a second idea of them. Zero
	// for a caller with no session, which is every caller but the HTTP one.
	Session Session
	// Saved reports that the fifteen came from a stored team rather than being chosen
	// now, and Optimised that it is the model's best rather than a varied opening. They
	// are separate because a saved team may or may not have started as the optimum, and
	// the page says something different about each.
	Saved     bool
	Optimised bool

	// Writable is whether the write route will accept THIS caller — which is not the same
	// question as whether this request carried a token, because the token is required only
	// where a save can reach config.json. The client draws its controls from this, so
	// getting it wrong either hides a control that works or offers one that will be
	// refused, and the second is the failure this surface exists to avoid.
	Writable bool

	// Chips is the season's chip schedule — placements for both sets — read from
	// the same engine the squad was built against. It supplies the rail's chip
	// window fields (State.Gameweeks[i].ChipWindow), which need to know what is
	// planned and where, not merely what the competition allows.
	Chips analysis.ChipSchedule

	// NewsChecked and NewsReadChecked are the News tab's two freshness lines,
	// pre-formatted by the caller for the reason every staleness figure in this
	// contract is: this package has no clock of its own to compute "ago"
	// against, and formatting it here would be the derivation the package
	// comment forbids. See State.News.
	NewsChecked, NewsReadChecked string

	// OverrideEffects is the before/after a standing minutes correction causes
	// to the model's own score, keyed by the player's permanent code — a
	// genuine model quantity, computed by the caller via
	// analysis.Engine.NaturalMetrics because this package may not compute one
	// itself. A code with no entry gets no Effect: the field is absent rather
	// than a misleading zero, which is also what a lock or an exclude gets,
	// since neither changes this player's own score.
	OverrideEffects map[int]Effect

	// Import is the team-import affordance's whole state, computed by the caller via
	// cmd/armband.importWindow — this package may not decide whether a gameweek has been
	// played any more than it may compute any other model quantity. See State.Import.
	Import Import

	// EarliestResultEvent bounds how far back buildGameweeks reaches into boot.Events for
	// FINISHED gameweeks that have already dropped out of the planning horizon (p.Weeks) —
	// see that function's own comment for why a settled week is not there at all. It is
	// fpl.EarliestResultEvent of the reader's own EntryHistory, resolved by the caller
	// (cmd/armband, from the same cached History fetch freeTransfersFor already makes),
	// because this package has no license to decide which past gameweeks the reader
	// actually has a result for — the same rule Import's own comment states. Zero means
	// "unknown" (no import, or the history read failed) and adds no past gameweeks at all
	// rather than guessing a bound.
	EarliestResultEvent int

	// Entry and History are the FPL manager record buildResults describes — either
	// config.EntryID's (armbandTeamState, the site's own spectator page) or a reader's own
	// imported session.Entry (GET /api/results) — fetched by the caller as an ordinary
	// cached client call, exactly like Boot. This package has no license to know which of
	// the two it was handed; see buildResults's own comment. Nil when the entry id is
	// unset or the fetch failed; Build then leaves State.Results nil rather than showing a
	// partial or stale figure. See State.Results.
	Entry   *fpl.Entry
	History *fpl.EntryHistory

	// Live is the ONE gameweek's live per-player stats (fpl.Client.Live), nil before that
	// gameweek has a fixture kicked off or if the fetch failed. MatchStatus is
	// "scheduled"/"live"/"fulltime"/"finished" per club short name for that same gameweek,
	// computed by the caller from a freshly-fetched fixture list (fpl.Client.FixturesLive
	// — NOT Client.Fixtures, which memoises for the life of the process and would answer a
	// stale "has this kicked off yet" for as long as the pod runs). This package may not
	// fetch either itself; it only arranges what the caller already fetched, same as
	// Entry/History.
	Live        *fpl.EventLive
	MatchStatus map[string]string

	// Opponent is ResultEvent's own fixture per club short name — computed by
	// the caller, in houseLiveSources, from the SAME freshly-fetched fixture list
	// that already decides MatchStatus, so the two always describe the same
	// match. This package may not fall back to a player's own forward-looking
	// Fixtures[0] for this: that list is anchored on the model's NEXT planning
	// gameweek (see analysis.Engine's own fromEvent) and drops ResultEvent's own
	// fixture the moment its deadline has passed, which is what silently swapped in
	// next week's opponent beside this week's counting stats — see
	// TeamPlayer.Opponent's own comment. A club absent from this map had no fixture
	// in ResultEvent (a blank gameweek) and TeamPlayer.Opponent stays nil for it.
	Opponent map[string]Fixture

	// Multiplier is this gameweek's pick multiplier, keyed by permanent code —
	// the SAME keyspace armbandteam.picksToFixed already uses for arrangement's
	// XI/Bench/Captain/Vice, because element ids are reassigned every summer and code
	// is what survives that. buildResults looks it up via the already-resolved
	// Player.Code rather than re-deriving anything: getting this keying wrong (id
	// instead of code) silently zeroes every multiplier and renders the whole pitch
	// as bench.
	Multiplier map[int]int

	// ResultEvent/ResultState/EventAverage describe the ONE gameweek
	// the result is about — event.ID and event.AverageScore for the event the caller
	// chose (latestClosedEvent, or the requested ?gw=), and the "live"/"final" state
	// houseLiveSources computed from the fixture list it already holds. This package may
	// not decide any of the three itself, for the same reason Import may not decide
	// whether a gameweek has been played — see Import's own comment.
	ResultEvent  int
	ResultState  string
	EventAverage int

	// ChipTeams is GET /api/wildcard's whole input, supplied only by that route's
	// handler (chipteams.go). Nil for every other caller, which is how Build knows
	// there is no such document to build. See ChipTeamsInput.
	ChipTeams *ChipTeamsInput
}

// ChipTeamsInput is what buildChipTeams needs to answer GET /api/wildcard. Every
// field is decided by the caller for the same reason ResultEvent/ResultState/
// EventAverage above are: this package may not decide which gameweek is next,
// whether a chip is playable, or what a rebuild rests on.
type ChipTeamsInput struct {
	Event    int
	Deadline time.Time

	Budget        float64
	BudgetSource  string
	BudgetWarning string
	Caveat        string

	// Wildcard and FreeHit are the two rebuilt WeekViews, nil when the
	// competition does not allow that chip in Event (analysis.PlayableChips).
	// Unavailable then carries the sentence to render instead.
	Wildcard            *analysis.WeekView
	FreeHit             *analysis.WeekView
	WildcardUnavailable string
	FreeHitUnavailable  string

	PlayedWildcardGW []int
	PlayedFreeHitGW  []int

	// TodaySquad is the house account's actual fifteen today -- the same
	// PlayerMetrics buildSquadPage's Fixed/Arrange path already resolved for the
	// handler's own call, reused here rather than fetched a second time. Nil when
	// that fetch failed, which leaves Changes/Out/KeptIDs on both chips at their
	// zero value: an absence rendered as an absence, never as "nothing changes".
	TodaySquad []analysis.PlayerMetrics

	// Codes and Overrides let buildChipTeams call buildPlayer exactly as buildSquad
	// does -- elementCodes(s.engine) and the house build's own Page.Overrides.
	Codes     map[int]int
	Overrides map[int]present.Override
}

// Build translates an assembled page into the client contract.
//
// It returns an error rather than a best effort, and the only error it can raise is a
// non-finite number. That is not defensive: encoding/json REFUSES to marshal NaN or
// +Inf, so a single bad float turns the whole document into a 500 with a message about
// "unsupported value" and no indication of which player caused it. Every score here is a
// float division somewhere upstream, and a player with zero expected minutes is a
// division this model does perform. Failing here names the field.
func Build(in Input) (*State, error) {
	p := in.Page

	s := &State{
		Now:     in.Now,
		Horizon: p.Horizon,
		Clubs:   p.Teams,
		Session: Session{
			Store:     "session",
			Writable:  in.Writable,
			Locked:    in.Session.Locked,
			Blocked:   in.Session.Blocked,
			Chips:     in.Session.Chips,
			Dismissed: in.Session.Dismissed,
			Saved:     in.Saved,
			Optimised: in.Optimised,
		},
	}
	if in.Persist {
		s.Session.Store = "config"
	}

	s.Squad = buildSquad(p)
	s.Gameweeks = buildGameweeks(p, in.Boot, in.Chips, in.Now, in.Import.Entry != 0, in.EarliestResultEvent)
	s.Results = buildResults(s.Squad, in.Entry, in.History, in.Boot,
		in.Live, in.MatchStatus, in.Opponent, in.Multiplier,
		in.ResultEvent, in.ResultState, in.EventAverage)
	s.Market = buildMarket(p)
	s.Overrides = buildOverrides(p)
	s.News = buildNews(s, in)
	s.Import = in.Import
	s.ChipTeams = buildChipTeams(in)
	if p.Reasoning != nil {
		s.Blind = p.Reasoning.Blind
		s.Policy = Policy{
			MinGainTransfer: p.Reasoning.Policy.MinGainTransfer,
			MinGainHit:      p.Reasoning.Policy.MinGainHit,
			BankUpTo:        p.Reasoning.Policy.BankUpTo,
			MaxHitsPerWeek:  p.Reasoning.Policy.MaxHitsPerWeek,
			Rules:           p.Reasoning.Policy.Rules,
		}
	}

	if err := checkFinite(s); err != nil {
		return nil, err
	}
	return s, nil
}

func buildSquad(p present.Page) Squad {
	sq := Squad{
		Formation:  p.Squad.Formation,
		Captain:    p.Squad.Captain.ID,
		Vice:       p.Squad.ViceCaptain.ID,
		XIScore:    p.Squad.XIScore,
		Expected:   p.Squad.ExpectedPoints,
		Cost:       p.Squad.TotalCost,
		Bank:       p.Squad.Remaining,
		ClubCounts: p.Squad.ClubCounts,
	}
	for _, m := range p.Squad.Players {
		sq.Players = append(sq.Players, buildPlayer(m, p.Codes, p.Overrides))
	}
	for _, m := range p.Squad.StartingXI {
		sq.XI = append(sq.XI, m.ID)
	}
	for _, m := range p.Squad.Bench {
		sq.Bench = append(sq.Bench, m.ID)
		// A plain sum of reported scores — what a bench boost would pay. See the
		// warning on Squad.BenchScore: this is deliberately not the bench's value
		// to the optimiser, which weights each slot by the chance it is used.
		sq.BenchScore += m.Score
	}
	return sq
}

// buildPlayer turns one PlayerMetrics into the client's Player.
//
// Every field is a copy. If a value needs working out, it is worked out in
// internal/analysis and copied here — that is the whole discipline of this package.
//
// Takes codes and overrides directly, rather than a present.Page, because
// buildChipTeams has PlayerMetrics for a rebuilt fifteen with no Page of its
// own behind it — only the bootstrap's code map and the page's override set,
// both of which it already has from the caller that built the house squad.
func buildPlayer(m analysis.PlayerMetrics, codes map[int]int, overrides map[int]present.Override) Player {
	pl := Player{
		ID:            m.ID,
		Code:          codes[m.ID],
		Name:          m.Name,
		Club:          m.Team,
		Pos:           m.Position,
		Price:         m.Price,
		XP:            m.Score,
		Per90:         m.FixtureAdjXP90,
		Minutes:       m.ExpectedMinutes,
		Role:          m.RotationRisk,
		Reliability:   m.MinutesRating,
		StartShare:    m.StartShare,
		Ownership:     m.Ownership,
		Status:        m.Status,
		News:          m.News,
		Availability:  m.AvailabilityFactor,
		AvgDifficulty: m.AvgDifficulty,
		ValueScore:    m.ValueScore,
		XG90:          m.XG90,
		XA90:          m.XA90,
		DefConChance:  m.DefConChance,
	}
	for _, f := range m.Fixtures {
		pl.Fixtures = append(pl.Fixtures, Fixture{
			Gameweek:   f.Event,
			Opponent:   f.Opponent,
			Home:       f.Home,
			Difficulty: f.Difficulty,
		})
	}
	if ov, ok := overrides[m.ID]; ok {
		o := convertOverride(ov)
		pl.Override = &o
	}
	return pl
}

// buildGameweeks joins the week views the build produced to the deadlines the bootstrap
// carries.
//
// The rail's length is whatever the build was asked for. Nothing here caps it: the design
// note is explicit that the planning window slides and must not be hard-coded, and a rail
// that silently stopped at five would start lying the first time a chip was planned
// outside it — which is most of why anyone opens the page.
//
// # Why Current is found by deadline, not Bootstrap.CurrentEvent/IsCurrent
//
// This used to prefer IsCurrent, falling back to IsNext. That is FPL's OWN answer to
// "which gameweek is being played", set by FPL's backend on its own schedule — not the
// instant a deadline passes. Observed directly on 2026-08-21: GW1's deadline was 17:30
// UTC, and bootstrap-static still answered is_current=false, is_next=true at 17:54, 24
// minutes past deadline with kickoff imminent. A reader who opened the planner in that
// window would have been shown GW1 as the one to act on, when it was already locked. A
// deadline is a fact this server already has and needs no such lag: Current is simply the
// gameweek with the EARLIEST deadline that has not yet passed — the next one a reader can
// still do anything about.
//
// # Why a closed, un-imported gameweek is dropped from the rail entirely
//
// Once a gameweek's deadline passes, its entry in p.Weeks is the model's hypothetical
// best-XI for a squad the reader can no longer change — a Monday-morning-quarterback
// opinion about a locked decision, not a plan. That is worth showing only when it can be
// replaced by the reader's ACTUAL picks (imported — see State.Import). Closed marks this
// case; the client fetches GET /api/results and renders that instead of the plan the
// moment it sees Closed true, for exactly the reason above — see app.js's renderRail.
//
// # Past gameweeks that have already left the planning horizon
//
// p.Weeks comes from analysis.Engine.WeekViews, which is built from upcomingEvents and
// drops a gameweek the instant every one of its fixtures is FPL-Finished (weekview.go) —
// it is a PLANNING horizon and has no reason to know about the past. So a gameweek that
// finished, say, a week ago is not "closed" in the sense above at all: it is simply
// absent, imported or not. Those are added here from boot.Events instead — the bootstrap
// carries every gameweek FPL has ever played — marked Closed exactly like the
// still-live case above, so the client's one rule ("Closed means fetch results, not a
// plan") covers both without needing to tell them apart.
//
// Bounded by earliestResultEvent (see Input.EarliestResultEvent's own comment): only
// FINISHED events at or after it are added, and only for an imported reader. Do NOT
// widen span/the horizon passed to WeekViews to reach these instead — that would re-run
// the optimiser over gameweeks nobody is planning, on every request, for a squad that can
// no longer change. boot.Events already has everything a past result needs (a deadline
// and a Finished flag); the plan fields (Chip, Projected, Formation, Playable, ChipWindow,
// Rebuilt) are left zero on a past Gameweek because there is no plan for a week that is
// over.
func buildGameweeks(p present.Page, boot *fpl.Bootstrap, chips analysis.ChipSchedule, now time.Time, imported bool, earliestResultEvent int) []Gameweek {
	deadline := map[int]time.Time{}
	current := 0
	if boot != nil {
		var earliestOpen *fpl.Event
		for i := range boot.Events {
			e := &boot.Events[i]
			deadline[e.ID] = e.DeadlineTime
			if e.DeadlineTime.Before(now) {
				continue
			}
			if earliestOpen == nil || e.DeadlineTime.Before(earliestOpen.DeadlineTime) {
				earliestOpen = e
			}
		}
		if earliestOpen != nil {
			current = earliestOpen.ID
		}
	}
	out := make([]Gameweek, 0, len(p.Weeks))
	// seen tracks every gameweek already placed from p.Weeks, so a past event sourced
	// from boot.Events below can never duplicate one the planning horizon still carries.
	// In practice a Finished event never reaches p.Weeks at all (see this function's own
	// comment on upcomingEvents), so this is a defensive guard rather than a load-bearing
	// one.
	seen := map[int]bool{}
	for _, w := range p.Weeks {
		closed := !deadline[w.Event].IsZero() && deadline[w.Event].Before(now)
		if closed && !imported {
			continue
		}
		seen[w.Event] = true
		var playable []ChipOption
		for _, key := range analysis.PlayableChips(boot, w.Event) {
			playable = append(playable, ChipOption{Key: key, Label: analysis.ChipLabel(key)})
		}
		out = append(out, Gameweek{
			Number:     w.Event,
			Deadline:   deadline[w.Event],
			Current:    w.Event == current,
			Closed:     closed,
			Chip:       w.Chip,
			Projected:  w.Expected,
			Formation:  w.Formation,
			Rebuilt:    w.Rebuilt,
			Playable:   playable,
			ChipWindow: buildChipWindow(boot, chips, w.Event, deadline),
		})
	}

	var past []Gameweek
	if imported && boot != nil && earliestResultEvent > 0 {
		for i := range boot.Events {
			e := &boot.Events[i]
			if !e.Finished || e.ID < earliestResultEvent || seen[e.ID] {
				continue
			}
			past = append(past, Gameweek{
				Number:   e.ID,
				Deadline: e.DeadlineTime,
				Closed:   true,
			})
		}
		sort.Slice(past, func(i, j int) bool { return past[i].Number < past[j].Number })
	}
	return append(past, out...)
}

// buildResults arranges ONE entry's manager record into the results page's contract — the
// site's own (armbandTeamState, config.EntryID) or a reader's own imported squad
// (GET /api/results, session.Entry). It takes plain data — a squad, an entry, live stats,
// which gameweek — and assembles a result; it has no branch on and no way to know which of
// the two callers reached it, the same rule this package's own comment states for every
// other field. If a difference between the two callers ever needs expressing, it is
// expressed by the CALLER — which entry id it fetched with — never by a parameter this
// function switches on.
//
// resultEvent/resultState/eventAverage all come from the caller (armbandTeamState or the
// results route, both via a chosen event and houseLiveSources) rather than being found
// here: this package may not decide which gameweek is being described or whether it has
// finished, the same rule Import's own comment states for the import affordance. This used
// to instead look up "the gameweek the rail marks Current" out of gws and pair it with
// sq.Expected — a different gameweek from the one the pitch and live stats actually
// describe the moment a deadline has passed but the rail has not rolled over. Removing gws
// (no longer used here at all) removes the temptation to reach for it again.
func buildResults(sq Squad, entry *fpl.Entry, history *fpl.EntryHistory, boot *fpl.Bootstrap,
	live *fpl.EventLive, matchStatus map[string]string, opponent map[string]Fixture, multiplier map[int]int,
	resultEvent int, resultState string, eventAverage int) *Results {
	if entry == nil {
		return nil
	}
	res := &Results{
		OverallPoints: entry.SummaryOverallPoints,
		OverallRank:   entry.SummaryOverallRank,
		ResultEvent:   resultEvent,
		ResultState:   resultState,
		EventAverage:  eventAverage,
		Formation:     sq.Formation,
		Captain:       sq.Captain,
		Vice:          sq.Vice,
	}
	if history != nil {
		for _, gw := range history.Current {
			res.History = append(res.History, Result{
				Event:       gw.Event,
				Points:      gw.Points,
				Rank:        gw.Rank,
				Hit:         gw.EventTransfersCost,
				BenchPoints: gw.PointsOnBench,
			})
		}
		// FPL's own record of what was actually played, not the plan -- see
		// Results.Chip's own comment.
		for _, c := range history.Chips {
			if c.Event == resultEvent {
				res.Chip = analysis.ChipLabel(c.Name)
				break
			}
		}
	}

	byID := make(map[int]Player, len(sq.Players))
	for _, p := range sq.Players {
		byID[p.ID] = p
	}
	teamPlayer := func(id int) (TeamPlayer, bool) {
		p, ok := byID[id]
		if !ok {
			return TeamPlayer{}, false
		}
		tp := TeamPlayer{ID: p.ID, Name: p.Name, Club: p.Club, Pos: p.Pos, Price: p.Price}
		// Opponent comes from the caller's ResultEvent-keyed map, NOT p.Fixtures[0] —
		// that field is the model's forward-looking fixture window (see
		// Input.Opponent's own comment) and index 0 stopped being ResultEvent's
		// fixture the moment its deadline passed. A club absent from the map had no
		// fixture in ResultEvent (a blank gameweek); leave Opponent nil rather than
		// falling back to the model's list, which is exactly what produced the bug.
		if f, ok := opponent[p.Club]; ok {
			tp.Opponent = &f
		}
		tp.MatchStatus = matchStatus[p.Club]
		// multiplier is keyed by permanent code (see Input.Multiplier's own
		// comment), the same keyspace Player.Code already carries — looked up via
		// p.Code, NOT p.ID. Using p.ID here is the exact mistake that silently zeroes
		// every multiplier and renders the whole pitch as bench, because element ids
		// and codes are different numbers for the same player.
		tp.Multiplier = multiplier[p.Code]

		stats := live.ByID(p.ID)
		if stats == nil {
			return tp, true
		}
		// An honest "as of now" at every status — zero before kickoff is correct, not
		// merely a placeholder for it.
		tp.Minutes = stats.Minutes
		tp.Points = stats.TotalPoints
		tp.Bonus = stats.Bonus
		tp.Goals = stats.GoalsScored
		tp.Assists = stats.Assists
		tp.CleanSheets = stats.CleanSheets
		tp.GoalsConceded = stats.GoalsConceded
		tp.YellowCards = stats.YellowCards
		tp.RedCards = stats.RedCards
		tp.OwnGoals = stats.OwnGoals
		tp.PenaltiesSaved = stats.PenaltiesSaved
		tp.PenaltiesMissed = stats.PenaltiesMissed

		// DefCon/Saves stay nil before kickoff on purpose: "did he clear the bar"
		// has no honest answer yet, and a red pill on a match that has not
		// started would read as a verdict rather than an absence of one.
		//
		// ⚠️ "fulltime" MUST stay in this list alongside "live" and "finished" --
		// this is the highest-risk line in the 2026-08-22 three-way status change.
		// FPL locks in DefCon and saves the moment FinishedProvisional flips (see
		// fpl.Fixture's own comment: "the match's own numbers ... are locked in"),
		// not at Finished, so gating on "finished" alone would silently strip
		// DefCon and saves from every ended-but-unsettled match -- a regression in
		// the OPPOSITE direction from the live-dot/asterisk bug this change fixes.
		// TestResultsFulltimeStillGetsDefConAndSaves pins it.
		if tp.MatchStatus != "live" && tp.MatchStatus != "finished" && tp.MatchStatus != "fulltime" {
			return tp, true
		}
		// The hover breakdown (see TeamPlayer.Breakdown's own comment) needs the
		// real element_type -- goal and clean-sheet values are position-priced --
		// the same lookup DefConReached below already needs, done once here so
		// neither has to look the element up twice.
		var pos int
		if boot != nil {
			if el := boot.ElementByID(p.ID); el != nil {
				pos = el.ElementType
			}
		}
		if pos != 0 {
			tp.Breakdown = scoreBreakdown(pos, *stats, tp.Points, tp.Multiplier)
		}
		if p.Pos == "GKP" {
			saves := stats.Saves
			tp.Saves = &saves
			return tp, true
		}
		dc := stats.DefensiveContribution
		tp.DefCon = &dc
		if pos != 0 {
			reached := dc >= analysis.DefConThreshold(pos)
			tp.DefConReached = &reached
		}
		return tp, true
	}
	for _, id := range sq.XI {
		if tp, ok := teamPlayer(id); ok {
			res.XI = append(res.XI, tp)
		}
	}
	for _, id := range sq.Bench {
		if tp, ok := teamPlayer(id); ok {
			res.Bench = append(res.Bench, tp)
		}
	}
	// LivePoints only while ResultState is "live" -- see its own doc comment for why
	// EntryHistory's Points is not usable yet at that state. XI and Bench together,
	// never just XI, because a benched player's Multiplier is already 0 and adding him
	// changes nothing -- a separate "bench excluded" branch would be a second rule for
	// a fact the multiplier already carries.
	if resultState == "live" {
		var hit int
		for _, h := range res.History {
			if h.Event == resultEvent {
				hit = h.Hit
				break
			}
		}
		var sum int
		for _, p := range res.XI {
			sum += p.Points * p.Multiplier
		}
		for _, p := range res.Bench {
			sum += p.Points * p.Multiplier
		}
		res.LivePoints = sum - hit
	}
	return res
}

// buildChipWindow reports the chip window this gameweek falls in, or nil when
// it falls in none — past the last chip window a season grants, for
// instance. ChipWindowStatusFor is a plain function of the bootstrap and the
// chip schedule, the same shape as PlayableChips above it, so this package
// calls it directly rather than reaching for an Engine it must not hold.
func buildChipWindow(boot *fpl.Bootstrap, chips analysis.ChipSchedule, gw int, deadline map[int]time.Time) *ChipWindow {
	status := analysis.ChipWindowStatusFor(boot, chips, gw)
	if status.EndsGW == 0 {
		return nil
	}
	cw := &ChipWindow{
		EndsGW:    status.EndsGW,
		Size:      status.Size,
		Remaining: status.Remaining,
	}
	if d, ok := deadline[status.EndsGW]; ok && !d.IsZero() {
		cw.EndsAt = d.Format("Mon 2 Jan")
	}
	return cw
}

// buildChipTeams arranges GET /api/wildcard's whole document from what the
// caller already decided: the gameweek, the budget a rebuild may spend, the
// two rebuilt WeekViews (or why a chip has none this week), and the plan/
// played facts read off config and FPL's own history. Called from Build when
// the caller supplies ChipTeamsInput; nil otherwise -- the same shape
// buildResults already has for Results.
func buildChipTeams(in Input) *ChipTeams {
	ci := in.ChipTeams
	if ci == nil {
		return nil
	}
	ct := &ChipTeams{
		Event:               ci.Event,
		Deadline:            ci.Deadline,
		Budget:              ci.Budget,
		BudgetSource:        ci.BudgetSource,
		BudgetWarning:       ci.BudgetWarning,
		Caveat:              ci.Caveat,
		WildcardUnavailable: ci.WildcardUnavailable,
		FreeHitUnavailable:  ci.FreeHitUnavailable,
		PlayedWildcardGW:    ci.PlayedWildcardGW,
		PlayedFreeHitGW:     ci.PlayedFreeHitGW,
	}
	if ci.Wildcard != nil {
		ct.Wildcard = buildChipTeam(*ci.Wildcard, ci.Budget, ci.Codes, ci.Overrides, ci.TodaySquad)
	}
	if ci.FreeHit != nil {
		ct.FreeHit = buildChipTeam(*ci.FreeHit, ci.Budget, ci.Codes, ci.Overrides, ci.TodaySquad)
	}
	return ct
}

// buildChipTeam turns one rebuilt WeekView into the client's ChipTeam.
//
// It arranges; it does not derive, with one exception this package's own
// standing rule invites the opposite reading on and so must say so here:
// Cost, ClubCounts, Changes, Out and KeptIDs are a plain sum, count and set
// intersection over two PlayerMetrics lists this function was handed --
// membership and arithmetic over already-decided values (Price, Team, ID),
// not a model quantity computed fresh. today is nil when the account's real
// fifteen could not be fetched, and Changes/Out/KeptIDs are then left at
// their zero value rather than a misleading "nothing changes".
func buildChipTeam(wv analysis.WeekView, budget float64, codes map[int]int,
	overrides map[int]present.Override, today []analysis.PlayerMetrics) *ChipTeam {

	ct := &ChipTeam{
		Formation:  wv.Formation,
		Captain:    wv.Captain.ID,
		Vice:       wv.ViceCaptain.ID,
		XIScore:    wv.XIScore,
		Expected:   wv.Expected,
		ClubCounts: map[string]int{},
		// wv.RebuildFailed is the one signal that tells a reader (and this
		// function's own Changes/Out below) "the model never looked" from "the
		// model looked and nothing changed" -- see ChipTeam.RebuildFailed's own
		// comment.
		RebuildFailed: wv.RebuildFailed,
	}
	if wv.RebuildFailed {
		// wv.Caveat is dual-purpose on WeekView -- a thin-evidence note when
		// Rebuilt, a "did not run" note when RebuildFailed, never both at once
		// (see WeekView.Caveat's own comment). Only copy it here under
		// RebuildFailed, so RebuildCaveat never carries the OTHER message under
		// the wrong name.
		ct.RebuildCaveat = wv.Caveat
	}

	// The opponent chip for THIS gameweek, never the model's forward-looking
	// fixture window -- Player.Fixtures is the model's ordinary multi-week
	// window and is NOT ResultEvent-correct the moment a deadline has passed
	// (see TeamPlayer.Opponent's own comment for the same mistake made once
	// already, on a different page). WeekView.Opponents is keyed by player id
	// and IS this gameweek's fixture by construction, so the projection card
	// reuses the existing Fixtures slot rather than growing a fourth type,
	// carrying exactly the one (or two, on a double) fixture that gameweek
	// has. Empty for a blanking club -- no fallback -- and the card renders a
	// dash.
	weekPlayer := func(m analysis.PlayerMetrics) Player {
		pl := buildPlayer(m, codes, overrides)
		pl.Fixtures = nil
		for _, f := range wv.Opponents[m.ID] {
			pl.Fixtures = append(pl.Fixtures, Fixture{
				Gameweek: f.Event, Opponent: f.Opponent, Home: f.Home, Difficulty: f.Difficulty,
			})
		}
		return pl
	}

	rebuilt := map[int]bool{}
	for _, m := range wv.Squad {
		rebuilt[m.ID] = true
		ct.Cost += m.Price
		ct.ClubCounts[m.Team]++
	}
	ct.Bank = budget - ct.Cost

	for _, m := range wv.XI {
		ct.XI = append(ct.XI, weekPlayer(m))
	}
	for _, m := range wv.Bench {
		ct.Bench = append(ct.Bench, weekPlayer(m))
	}

	for _, m := range today {
		if rebuilt[m.ID] {
			ct.KeptIDs = append(ct.KeptIDs, m.ID)
		} else {
			ct.Changes++
			ct.Out = append(ct.Out, m.Name)
		}
	}
	return ct
}

func buildMarket(p present.Page) Market {
	var m Market
	if p.Watch == nil {
		return m
	}
	m.Count = p.Watch.Count
	m.Clearing = p.Watch.Clearing
	m.Gate = p.Watch.Gate
	for _, r := range p.Watch.Rows {
		m.Rows = append(m.Rows, MarketRow{
			Player:     buildPlayer(r.Player, p.Codes, p.Overrides),
			Delta:      r.Delta,
			ClearsGate: r.ClearsGate,
		})
	}
	for _, b := range p.Watch.Benchmarks {
		m.Benchmarks = append(m.Benchmarks, Benchmark{
			Pos:   b.Position,
			Name:  b.Name,
			Score: b.Score,
			Price: b.Price,
		})
	}
	for _, o := range p.Watch.Excluded {
		m.Excluded = append(m.Excluded, convertOverride(o))
	}
	return m
}

func buildOverrides(p present.Page) Overrides {
	var o Overrides
	if p.Reasoning == nil {
		return o
	}
	for _, v := range p.Reasoning.Overrides {
		o.Live = append(o.Live, convertOverride(v))
	}
	for _, v := range p.Reasoning.Lapsed {
		o.Lapsed = append(o.Lapsed, convertOverride(v))
	}
	// Due and Oldest are asked of the renderer's own type rather than recounted, so the
	// count on the page and the count in the API cannot disagree.
	o.Due = p.Reasoning.Due()
	o.Oldest = p.Reasoning.Oldest()
	return o
}

// buildNews assembles the News tab: the two freshness lines the caller
// pre-formatted, plus every row — FPL's own status feed, drawn straight off
// the squad players this document already built, and the team news that has
// been read and recorded, which is the same set State.Overrides.Live already
// carries, in this tab's row shape. Nothing here is derived: both freshness
// strings are copied from Input, and the before/after on a reported row is
// copied from Input.OverrideEffects, computed by the caller because this
// package may not compute a model quantity itself.
func buildNews(s *State, in Input) News {
	n := News{Checked: in.NewsChecked, ReadChecked: in.NewsReadChecked}

	// FPL's own status feed: only players it has actually flagged. Availability
	// is the same multiplier the score itself was reduced by, so gating on it
	// rather than parsing the status string agrees with the model by
	// construction instead of restating its own rule.
	for _, pl := range s.Squad.Players {
		if pl.Availability >= 1 || pl.News == "" {
			continue
		}
		n.Items = append(n.Items, NewsItem{
			Source: "FPL",
			Player: pl.Name, PlayerCode: pl.Code, Club: pl.Club, Pos: pl.Pos,
			Body: pl.News,
			Flag: pl.Status,
		})
	}

	// Team news read and recorded: the Overrides tab's own live list, drawn
	// again in the News tab's row shape rather than re-read from config, so
	// the two tabs cannot disagree about what is currently in force.
	for _, o := range s.Overrides.Live {
		item := NewsItem{
			Source: "REPORTED",
			Player: o.Player, PlayerCode: o.Code, Club: o.Club, Pos: o.Pos,
			Body: o.Reason,
			When: o.Checked,
		}
		if e, ok := in.OverrideEffects[o.Code]; ok {
			effect := e
			item.Effect = &effect
		}
		n.Items = append(n.Items, item)
	}

	return n
}

func convertOverride(o present.Override) Override {
	return Override{
		Kind:         o.Kind,
		Code:         o.Code,
		Session:      o.Session,
		Label:        o.Label,
		Reason:       o.Reason,
		Player:       o.Player,
		Club:         o.Team,
		Pos:          o.Pos,
		SetOn:        o.SetOn,
		Until:        o.Until,
		Checked:      o.Checked,
		NeedsCheck:   o.NeedsCheck,
		CheckAge:     o.CheckAge,
		NeverChecked: o.NeverChecked,
		Lapsed:       o.Lapsed,
		Inherited:    o.Inherited,
		InSquad:      o.InSquad,
		Affects:      o.AffectsInSquad,
		Flag:         o.Flag(),
	}
}

// checkFinite walks the built state and refuses any float that JSON cannot carry.
//
// It walks by reflection rather than checking the fields it knows about, because the
// fields it knows about are exactly the ones somebody remembered. A new projection added
// to Player two months from now is caught by this and would not be caught by a list.
//
// The path in the error is the field chain, not the player's name: the point is to send
// whoever reads the 500 to the line of Go that produced the number.
func checkFinite(v any) error {
	return walkFinite(reflect.ValueOf(v), "")
}

func walkFinite(v reflect.Value, path string) error {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		if f := v.Float(); math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("viewmodel: %s is %v, which encoding/json cannot marshal "+
				"— the whole document would fail rather than this one field", path, f)
		}
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return walkFinite(v.Elem(), path)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			if !t.Field(i).IsExported() {
				continue
			}
			if err := walkFinite(v.Field(i), path+"."+t.Field(i).Name); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := walkFinite(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := walkFinite(iter.Value(), fmt.Sprintf("%s[%v]", path, iter.Key())); err != nil {
				return err
			}
		}
	}
	return nil
}
