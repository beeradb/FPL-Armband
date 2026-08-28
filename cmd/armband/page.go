package main

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"sort"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
	"armband/internal/viewmodel"
)

// squadPageBuild is one run of the shared squad pipeline: the optimal fifteen,
// the page built around it, and the budget facts the terminal view also prints.
//
// A struct rather than five positional returns because the two callers want
// different halves — `squad` reads the squad and the budget lines, `serve` reads
// the page — and a ladder of named-but-untyped returns is how the old present.HTML
// family ended up with a subtitle in its title.
type squadPageBuild struct {
	Page present.Page
	// Squad is the fifteen the page is built around. The terminal pitch prints
	// it; the page carries its own copy, which the renderer owns.
	Squad *analysis.Squad
	// BudgetLine is "Budget £Xm: <source>" — what the build was allowed to
	// spend and whose money it was. Source answers the same question on its own
	// for the pitch, which prints the figure separately.
	BudgetLine, Source string
	// OverrideEffects is the before/after a standing minutes correction causes
	// to the model's own score, keyed by permanent player code — the News tab's
	// effect line, computed here because only this package has both the engine
	// and the resolved override list at once. See overrideEffects. Nil when
	// wantPage is false: nothing downstream of the terminal command reads it.
	OverrideEffects map[int]viewmodel.Effect
}

// buildSquadPage runs the optimiser and, when a page is wanted, assembles
// the squad page around it.
//
// This is the one pipeline behind `armband squad -html` and `armband serve`,
// so a page written to disk and a page served over HTTP cannot drift into two
// pages. Everything in it reads config or the engine and hands
// internal/present flat structs it can only draw — the boundary this file
// exists to keep.
//
// wantPage gates the page half. The terminal `armband squad` wants the
// fifteen only, and the page half is not free: the transfer plan fetches the
// owned squad from FPL, the watchlist and the research targets each pass the
// whole pool, and WeekViews re-optimises chip weeks. Computing all of it to
// throw it away would give the plain command network fetches and a transfer
// search it never had.
//
// now is the clock the override staleness rule reads, and it is a parameter
// rather than a time.Now() inside because it is the build's ONLY dependence on
// wall time — everything else here is a function of the bootstrap and the
// config. Passing it makes the whole page reproducible, which is what the
// visual-regression goldens are compared against. It is deliberately not an
// environment variable: a new FPL_* switch has to be registered in
// internal/snapshot's fingerprint list, and a switch that can change what the
// model computes belongs in the fingerprint, not in a page renderer.
// pageOpts is everything the build needs beyond the config and the engine.
//
// A struct rather than ten parameters, and more importantly the place the squad CHOICE is
// documented: there are three ways a fifteen reaches the page and they cost wildly
// different amounts. Fixed is a reload and runs no search; Optimised is the true optimum;
// neither set is a new session and gets a seeded, varied squad.
type pageOpts struct {
	Weeks    int
	WantPage bool
	// Now is the clock the override staleness rule reads, and it is passed rather than
	// read because it is the build's ONLY dependence on wall time -- everything else is a
	// function of the bootstrap and the config. Passing it makes the build reproducible.
	// Deliberately not an environment variable: a new FPL_* switch has to be registered
	// in internal/snapshot's fingerprint list, and a switch that can change what the
	// model computes does not belong in a page build.
	Now time.Time

	// Fixed is a fifteen by permanent code -- the reader's saved team. When it is set and
	// still resolves, the optimiser does not run at all: the fifteen is known and only
	// the arrangement has to be worked out. This is the reload path.
	Fixed []int
	// Seed varies which of several good squads a new session is given. Ignored when
	// Fixed resolves or Optimised is set. See buildVariedSquad.
	Seed int64
	// Optimised asks for the true optimum, which is what the Optimize button means.
	Optimised bool
	// Arrange is the reader's own lineup within Fixed. Empty means the model's choice.
	Arrange arrangement
}

func buildSquadPage(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine, opts pageOpts) (squadPageBuild, error) {

	weeks, wantPage, now := opts.Weeks, opts.WantPage, opts.Now

	// Stage timings for the page build (FPL_SERVE_TIMINGS=1), printed to stderr.
	//
	// Kept rather than removed once the snappy-page work had used it. The
	// optimiser is the binding constraint on what this project can afford to
	// run, so it gets optimised repeatedly, and every one of those sittings
	// starts by asking which stage the time is in. Rebuilding this each time
	// invites a subtly different instrument each time — and one that is absent
	// while the question is being asked is an instrument nobody reaches for.
	//
	// It is additive printing and reaches nothing the build computes, which is
	// why the fingerprint guard skips it rather than recording it: a run with it
	// set is the same run. Pair it with FPL_CPU_PROFILE below, which names where
	// to write a pprof profile of the same pipeline.
	//
	// Every mark also feeds armband_page_build_seconds{stage=...}, unconditionally
	// — the stderr print stays opt-in behind FPL_SERVE_TIMINGS, but the metric is
	// the always-on version of the same question, for a deployment rather than a
	// sitting at a terminal. The same duration feeds both, so they cannot disagree.
	last := time.Now()
	mark := func(s string) {
		// Deliberately the wall clock, and deliberately not the `now`
		// parameter: this measures how long the build took, which is a fact
		// about this machine, while `now` is the date the page is ABOUT. A
		// pinned `now` must not make the timings read as zero.
		at := time.Now()
		appMetrics.pageBuildSeconds.WithLabelValues(s).Observe(at.Sub(last).Seconds())
		if os.Getenv("FPL_SERVE_TIMINGS") != "" {
			fmt.Fprintf(os.Stderr, "TIMING %-14s %6.0fms\n", s, float64(at.Sub(last).Milliseconds()))
		}
		last = at
	}

	if pf := os.Getenv("FPL_CPU_PROFILE"); pf != "" {
		f, err := os.Create(pf)
		if err != nil {
			return squadPageBuild{}, err
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			return squadPageBuild{}, err
		}
		defer func() {
			pprof.StopCPUProfile()
			_ = f.Close()
		}()
	}

	budget, source, err := e.AssemblyBudget()
	if err != nil {
		return squadPageBuild{}, err
	}
	// BenchWeight is deliberately left at zero, which Optimize reads as "use the
	// configured weight" — config.json's bench_weight, backfilled to
	// analysis.DefaultBenchWeight. This used to name 0.02 while `brief`'s
	// model-optimal squad named 0.10, so two commands each claiming to print the
	// best fifteen from the same money printed different fifteens. See
	// TestSquadBuildersDoNotNameABenchWeight.
	req := analysis.OptimizeRequest{
		Budget:             budget,
		MinMinutes:         analysis.PoolMinMinutes,
		MinExpectedMinutes: analysis.PoolMinExpectedMinutes,
	}
	for _, note := range applyRoster(cfg, e, &req) {
		fmt.Printf("\n%s\n", dim(note))
	}
	// The chip plan comes from THIS request's config, not from the one the process
	// started with. The engine's field is set once in main; a served page whose reader has
	// placed a chip needs their plan, and assigning it here is what makes the placement
	// reach ApplyChipPlan and the week views.
	//
	// Both of the engine's fields this touches are restored on the way out. The server
	// holds ONE engine for every request, and ApplyChipPlan shortens Weights.Horizon when a
	// wildcard is planned — correct for the squad being built, and permanent unless undone.
	// A reader who planned a wildcard for gameweek 3 truncated the engine to two gameweeks
	// and left it there: for himself after he removed the chip, and for everyone else using
	// the same process. The CLI exits and never noticed.
	priorChips, priorHorizon := e.Chips, e.Weights.Horizon
	defer func() { e.Chips, e.Weights.Horizon = priorChips, priorHorizon }()
	e.Chips = cfg.Chips
	for _, note := range e.ApplyChipPlan(&req) {
		fmt.Printf("\n%s\n", dim("chip plan: "+note))
	}
	// The three ways a fifteen is chosen. See pageOpts.
	var sq *analysis.Squad
	if len(opts.Fixed) > 0 {
		// Rebuilt from the reader's saved team. Nil means a player has gone -- out of
		// the game, or a code the bootstrap no longer carries -- and a partial fifteen
		// is not a team, so it falls through to a fresh build rather than drawing
		// fourteen players and saying nothing.
		sq = squadFromCodes(e, e.AllMetrics(), opts.Fixed, opts.Arrange)

		// ⚠️ And the stored team must still obey the standing corrections.
		//
		// A block means "never picked, anywhere". A lock means "in every squad". Those
		// bind through `req`, and `req` is only read on the branch below -- so a stored
		// squad silently outranked both, and blocking a player left him on the pitch
		// with a badge as the only sign anything had happened.
		//
		// Falling back to a rebuild is the honest answer rather than a repair: the
		// reader has just said this fifteen is wrong, so the model picks a new one under
		// the correction. Their ARRANGEMENT is lost with it, which is unavoidable —
		// there is no eleven that both keeps their shape and omits a player it names.
		if violatesRoster(sq, req) {
			sq = nil
		}
	}
	if sq == nil {
		var err error
		if opts.Optimised || opts.Seed == 0 {
			sq, err = e.Optimize(req)
		} else {
			sq, err = buildVariedSquad(e, req, opts.Seed)
		}
		if err != nil {
			return squadPageBuild{}, err
		}
	}
	mark("optimize")

	// Say what it was allowed to spend, not merely what it spent. A £102.0m
	// squad is a bug at the opening allowance and correct on a wildcard budget,
	// and the reader cannot tell which without the source.
	budgetLine := fmt.Sprintf("Budget £%.1fm: %s", float64(budget)/10, source)

	if !wantPage {
		return squadPageBuild{Squad: sq, BudgetLine: budgetLine, Source: source}, nil
	}

	// One eleven under a five-gameweek heading is a mis-statement, so the
	// page carries a tab per gameweek with that week's eleven, each
	// player's opponent, and any chip the plan puts there.
	// Deliberately allowed to exceed the scoring horizon. The squad is
	// OPTIMISED over the horizon, but a chip is usually planned outside it —
	// a wildcard at GW6 is invisible on a five-week view — and seeing which
	// week a chip lands in is most of why anyone opens this page.
	span := weeks
	if span <= 0 {
		span = e.Weights.Horizon
	}
	views := e.WeekViews(sq.Players, span, req)
	mark("weekviews")
	// Projected transfers for the squad you actually own, when there is one.
	// The reason there is not is carried through and printed, because an
	// absent section cannot distinguish "no squad yet" from "no move is
	// worth making", and the second is a recommendation.
	plan, why := bestPlanForOwnedSquad(ctx, cfg, client, e)
	mark("transfer plan")

	// The two views behind the eleven. Built here because every value in them
	// comes from config or the engine, and the renderer may reach for neither.
	bound, live, lapsed := pageOverrides(cfg, e, sq.Players, now)
	mark("overrides")
	effects := overrideEffects(e, live)
	var excluded []present.Override
	for _, o := range live {
		if o.Kind == "exclude" {
			excluded = append(excluded, o)
		}
	}
	page := present.Page{
		Title:              fmt.Sprintf("Optimal squad — next %d gameweeks", e.Weights.Horizon),
		Subtitle:           budgetLine,
		Squad:              *sq,
		Plan:               plan,
		NoPlan:             why,
		Weeks:              views,
		Brief:              squadBrief(cfg, e),
		Analysis:           readAnalysis(),
		Horizon:            e.Weights.Horizon,
		Overrides:          bound,
		FixtureLoadInScore: e.FixtureLoadInScore(),
		Reasoning:          reasoningFor(cfg, e, sq.Players, live, lapsed),
		Watch:              watchlistFor(e, *sq, excluded, bound, cfg.Review.MinGainForTransfer),
		Codes:              elementCodes(e),
		Teams:              clubShortNames(e),
	}
	mark("page assemble")
	return squadPageBuild{
		Page: page, Squad: sq, BudgetLine: budgetLine, Source: source,
		OverrideEffects: effects,
	}, nil
}

// clubShortNames lists the clubs in the bootstrap, for the watchlist's team
// filter. Sorted so the select reads like a table rather than an API dump.
func clubShortNames(e *analysis.Engine) []string {
	var names []string
	for i := range e.Boot.Teams {
		names = append(names, e.Boot.Teams[i].ShortName)
	}
	sort.Strings(names)
	return names
}

// elementCodes maps element id to permanent player code.
//
// The page's write actions post the CODE, never the element id: ids are
// reassigned every summer, and an override keyed on one comes back next August
// attached to a different footballer. The bootstrap is the one place both ids
// meet, so this is where the map is built — the renderer receives it and
// never asks.
func elementCodes(e *analysis.Engine) map[int]int {
	m := make(map[int]int, len(e.Boot.Elements))
	for i := range e.Boot.Elements {
		m[e.Boot.Elements[i].ID] = e.Boot.Elements[i].Code
	}
	return m
}

// Assembling the squad page's three views.
//
// # Why this lives here and not in internal/present
//
// Every value below comes from the engine or from config, and the renderer is not
// allowed to reach for either. That is what keeps one deadline, one chip plan, one
// override list and one research categorisation in the program rather than two that
// can disagree — the standing rule that a quantity has one implementation, applied to
// the boundary between deciding and drawing.
//
// So this file reads config and calls the engine, and hands internal/present flat
// structs it can only draw.

// pageOverrides binds every standing correction to the element it acts on, and
// returns the same set again as cards for the "why" view.
//
// # Why the binding is by permanent code and not by element id
//
// Element ids are reassigned every summer. An override keyed on one comes back next
// August attached to a different footballer — which is why config stores the code and
// why applyRoster already builds this map before touching the optimiser. Building it
// the same way here is not duplication of the *decision*, it is the same lookup: the
// alternative is matching on name, and two players have shared a surname before.
//
// # Why club overrides ride on player rows
//
// A TeamOverride is not a player, but it moves every defensive term of every player at
// that club. A reader looking at a defender needs to know his number carries a hand-set
// club correction; putting the badge only on some club-level list would mean the fact
// never reaches the row where it changes a decision. It is drawn dashed rather than
// solid so it reads as inherited rather than personal.
func pageOverrides(cfg config.Config, e *analysis.Engine, squad []analysis.PlayerMetrics,
	now time.Time) (map[int]present.Override, []present.Override, []present.Override) {

	gw := 1
	if next := e.Boot.NextEvent(); next != nil {
		gw = next.ID
	}

	byCode := map[int]int{}
	for i := range e.Boot.Elements {
		byCode[e.Boot.Elements[i].Code] = e.Boot.Elements[i].ID
	}
	// Squad membership, for the "which of your fifteen does this club correction
	// touch" line. Without it a reader cannot tell whether a club override is doing
	// anything to *this* squad, which is the only question he asks of one.
	inSquad := map[int]analysis.PlayerMetrics{}
	for _, p := range squad {
		inSquad[p.ID] = p
	}

	bound := map[int]present.Override{}
	var live, lapsed []present.Override

	// player builds one card from a player-scoped override. `lapsedNow` is passed in
	// rather than re-derived: config.Roster already decided which list this came from,
	// and re-deciding it here would be a second implementation of Expired. `bind`
	// decides whether the card takes the per-element badge slot: one slot exists, so
	// the first list in precedence order that speaks for a player keeps it.
	player := func(kind, label string, o config.RosterOverride, lapsedNow, bind bool) present.Override {
		ov := present.Override{
			Kind: kind, Label: label, Reason: o.Reason, Player: o.Name, Code: o.Code,
			SetOn: o.SetOn, Until: lapses(o.UntilGameweek),
			Checked:      checkedAge(o, now),
			CheckAge:     o.CheckAge(now),
			NeverChecked: o.LastChecked == "",
			NeedsCheck:   !lapsedNow && o.NeedsCheck(now, gw),
			Lapsed:       lapsedNow,
		}
		if id, ok := byCode[o.Code]; ok {
			if p, ok := inSquad[id]; ok {
				ov.Team, ov.Pos, ov.Price, ov.Score = p.Team, p.Position, p.Price, p.Score
				ov.InSquad = true
			} else if el := e.Boot.ElementByID(id); el != nil {
				m := e.Metrics(el)
				ov.Team, ov.Pos, ov.Price, ov.Score = m.Team, m.Position, m.Price, m.Score
			}
			if !lapsedNow && bind {
				bound[id] = ov
			}
		}
		return ov
	}

	lock, exclude, expired := cfg.Roster.Active(gw)
	for _, o := range lock {
		label := "LOCK"
		if o.MustStart {
			label = "LOCK XI"
		}
		live = append(live, player("lock", label, o, false, true))
	}
	for _, o := range exclude {
		live = append(live, player("exclude", "EXCL", o, false, true))
	}
	// The value is part of the badge and is never dropped. "MIN 88" holds a backup
	// keeper in the eleven and "MIN 15" writes an injured defender down: opposite
	// interventions, and a badge reading "MIN" cannot tell them apart.
	mins, minsExpired := cfg.Roster.ActiveMinutes(gw)
	for _, o := range mins {
		label := "MIN"
		if o.ExpectedMinutes != nil {
			label = fmt.Sprintf("MIN %.0f", *o.ExpectedMinutes)
		}
		// A minutes correction takes the badge slot only when no lock or
		// exclusion already owns it. First writer wins, and lock and exclude
		// ran first: the page's lock button reads this slot to decide whether
		// a player is locked, so a MIN badge over a lock would render an OFF
		// button on a player the optimiser is forced to keep — and clicking it
		// would re-write the lock, replacing the reason the agent set with the
		// page's canned one and clearing MustStart. The same guard keeps an
		// excluded player's EXCL badge, which the watchlist's skip set reads.
		if o.ExpectedMinutes != nil && o.Confirmed {
			// Confirmed decides whether rotation_risk can read "nailed" and was
			// found to be invisible on this page entirely. Appended to Label,
			// not Reason: Reason is the raw stored text, reused verbatim by the
			// News tab (buildNews copies it into NewsItem.Body), so mutating it
			// here would silently rewrite what a REPORTED row says. Label is
			// this page's own compact badge and has no other reader.
			//
			// Only the confirmed case adds a marker; leaving the unconfirmed
			// case bare keeps the ordinary "MIN 88" badge unchanged for the
			// common case, which is what most minutes overrides are.
			label += " (confirmed)"
		}
		_, taken := bound[byCode[o.Code]]
		live = append(live, player("minutes", label, o, false, !taken))
	}
	for _, o := range expired {
		lapsed = append(lapsed, player("lapsed", "LAPSED", o, true, true))
	}
	for _, o := range minsExpired {
		lapsed = append(lapsed, player("lapsed", "LAPSED MIN", o, true, true))
	}

	teams, teamsExpired := cfg.Roster.ActiveTeams(gw)
	club := func(o config.TeamOverride, lapsedNow bool) present.Override {
		label := o.Team
		if o.XGCFactor > 0 {
			label = fmt.Sprintf("%s ×%.2f", o.Team, o.XGCFactor)
		}
		// The club's full name in the title, and Team left empty so the header's
		// `.club` span drops out. Set naively, "ARS" appeared three times in one line
		// — badge, title and club span — which reads as a rendering stutter rather
		// than as emphasis.
		name := o.Team
		if t := teamByShortName(e, o.Team); t != nil {
			name = t.Name
		}
		ov := present.Override{
			Kind: "club", Label: label, Reason: o.Reason, Player: name + " — club correction",
			SetOn: o.SetOn, Until: lapses(o.UntilGameweek),
			Checked:      clubCheckedAge(o, now),
			CheckAge:     o.CheckAge(now),
			NeverChecked: o.LastChecked == "",
			NeedsCheck:   !lapsedNow && o.NeedsCheck(now, gw),
			Lapsed:       lapsedNow, Inherited: true,
		}
		for _, p := range squad {
			if p.Team == o.Team {
				ov.AffectsInSquad = append(ov.AffectsInSquad, p.Name)
			}
		}
		if !lapsedNow {
			for i := range e.Boot.Elements {
				el := &e.Boot.Elements[i]
				if t := e.Boot.TeamByID(el.Team); t != nil && t.ShortName == o.Team {
					// A player already carrying a personal override keeps it: his own
					// correction is the one that explains his own number, and two
					// badges on one row is the density this page is trying to avoid.
					if _, taken := bound[el.ID]; !taken {
						bound[el.ID] = ov
					}
				}
			}
		}
		return ov
	}
	for _, o := range teams {
		live = append(live, club(o, false))
	}
	for _, o := range teamsExpired {
		lapsed = append(lapsed, club(o, true))
	}

	// Urgency, then effect, then staleness, then taxonomy.
	//
	// Sorting by kind alone put the override HOLDING A PLAYER IN THE STARTING ELEVEN
	// fifth of nine, behind exclusions on players the optimiser had already declined.
	// The order a reader needs is: which of these is doing something to the squad I
	// am looking at, and which has gone longest without anyone checking it.
	rank := map[string]int{"lock": 1, "exclude": 2, "minutes": 3, "club": 4}
	sort.SliceStable(live, func(i, j int) bool {
		a, b := live[i], live[j]
		if a.NeedsCheck != b.NeedsCheck {
			return a.NeedsCheck
		}
		if a.InSquad != b.InSquad {
			return a.InSquad
		}
		// Never-verified before merely stale, then oldest first. An override nobody
		// has ever re-read is the one most likely to be wrong, and its CheckAge is
		// the age of the DECISION rather than of any check, so the two are not
		// comparable as a single number.
		if a.NeverChecked != b.NeverChecked {
			return a.NeverChecked
		}
		if a.CheckAge != b.CheckAge {
			return a.CheckAge > b.CheckAge // oldest first
		}
		if rank[a.Kind] != rank[b.Kind] {
			return rank[a.Kind] < rank[b.Kind]
		}
		return a.Player < b.Player
	})
	return bound, live, lapsed
}

// overrideEffects computes the before/after a standing minutes correction
// causes to the model's own score — the News tab's effect line, e.g. "pts a
// week 4.20 -> 3.15". A genuine model quantity, so it is computed here,
// against the same engine that scored the page, rather than left for
// internal/viewmodel to derive.
//
// Only "minutes" corrections get one. A lock or an exclude changes which
// SQUAD is built, not this player's own Score — analysis.Engine.Metrics
// answers exactly the same for a locked player whether or not he is locked —
// so there is no comparable before/after to report, and the map simply has
// no entry for his code rather than a zero that would read as "no effect".
func overrideEffects(e *analysis.Engine, live []present.Override) map[int]viewmodel.Effect {
	var out map[int]viewmodel.Effect
	for _, o := range live {
		if o.Kind != "minutes" || o.Code == 0 {
			continue
		}
		el := e.Boot.ElementByCode(o.Code)
		if el == nil || !e.HasMinutesOverride(el.Code) {
			continue
		}
		with := e.Metrics(el)
		without := e.NaturalMetrics(el)
		const flatBand = 0.005 // pts/gw: below this, "up" or "down" would claim more precision than the model has
		dir := "flat"
		switch {
		case with.Score > without.Score+flatBand:
			dir = "up"
		case with.Score < without.Score-flatBand:
			dir = "down"
		}
		if out == nil {
			out = map[int]viewmodel.Effect{}
		}
		out[o.Code] = viewmodel.Effect{
			Label:     "pts a week",
			Was:       fmt.Sprintf("%.2f", without.Score),
			Now:       fmt.Sprintf("%.2f", with.Score),
			Direction: dir,
		}
	}
	return out
}

func teamByShortName(e *analysis.Engine, short string) *fpl.Team {
	for i := range e.Boot.Teams {
		if e.Boot.Teams[i].ShortName == short {
			return &e.Boot.Teams[i]
		}
	}
	return nil
}

// checkedAge renders when an override was last verified, with its age, or "never".
//
// Never is printed rather than left blank. An override whose reason has never been
// re-checked against the news is the one most likely to be wrong, and a blank cell
// reads as "recently" to anyone scanning.
func checkedAge(o config.RosterOverride, now time.Time) string {
	return withAge(o.LastChecked, o.CheckAge(now))
}

func clubCheckedAge(o config.TeamOverride, now time.Time) string {
	return withAge(o.LastChecked, o.CheckAge(now))
}

// lapses phrases the expiry as a clause rather than a fragment.
//
// The briefing's table can say "after GW10" because its column header supplies the
// verb. On the page the same string sits in a free-running meta row between "set
// 2026-08-05" and "checked 2026-08-07", where "after GW10" reads as a continuation of
// the date before it — "set 2026-08-05 · after GW10". So the page carries its own
// phrasing rather than changing the table's, where it is correct.
func lapses(gw int) string {
	if gw > 0 {
		return fmt.Sprintf("lapses after GW%d", gw)
	}
	// A clause, to match "lapses after GW10" — the meta row reads "set 2026-08-05 ·
	// checked 2026-08-07 · does not lapse", not a bare fragment butted against the
	// date before it.
	return "does not lapse"
}

func withAge(lastChecked string, age int) string {
	s := checkedOrNever(lastChecked)
	if age > 0 {
		s = fmt.Sprintf("%s (%dd)", s, age)
	}
	return s
}

// newsChecked is the News tab's FPL-status freshness line — "FPL status last
// read 3 minutes ago · next read at 15:00" — built from the disk cache's own
// modification time, which IS the fetch: see Client.BootstrapFetchedAt.
// Empty before anything has been cached.
//
// ⚠️ The "next read at" half describes the cache's TTL contract, not a
// promise that THIS process will act on it. `armband serve` holds one
// *fpl.Client for its whole life and Client.Bootstrap memoizes its result in
// memory forever (see that method's own comment), so nothing here actually
// refetches until the process restarts, however old the disk file gets. The
// line is still honest about what it says — when the data being served was
// fetched, and when the cache mechanism would next consider it stale — it
// just does not (yet) describe a live polling loop. Fixing that is a
// separate, pre-existing gap this pass did not take on.
func newsChecked(client *fpl.Client, now time.Time) string {
	if client == nil {
		return ""
	}
	fetched := client.BootstrapFetchedAt()
	if fetched.IsZero() {
		return ""
	}
	next := fetched.Add(client.CacheTTL())
	return fmt.Sprintf("FPL status last read %s · next read at %s",
		minutesAgo(now.Sub(fetched)), next.Format("15:04"))
}

// minutesAgo phrases a duration the way the News tab's freshness lines do.
// Negative (a clock skew, or `now` pinned behind the fetch in a test) reads
// as "just now" rather than a nonsensical negative count.
func minutesAgo(d time.Duration) string {
	m := int(d.Minutes())
	switch {
	case m <= 0:
		return "just now"
	case m == 1:
		return "1 minute ago"
	default:
		return fmt.Sprintf("%d minutes ago", m)
	}
}

// newsReadChecked is the News tab's OTHER freshness line — when team news was
// last read and recorded, as distinct from FPL's own feed above. It is the
// closest honest proxy this store has: the roster and team corrections in cfg
// ARE the team news a human has read and told the model about (NOTES.md §3),
// so the most recent date any of them was set or re-checked is the most
// recent read.
//
// ⚠️ Day granularity, not minutes. The store keeps a DATE
// (RosterOverride.SetOn / LastChecked), not a timestamp — widening that is a
// real change to every write site this pass did not make, so this reads
// coarser than the design's own illustrative "41 minutes ago". No "next read
// at" half either: nothing schedules the reading process yet, so a predicted
// next read would be a promise this system cannot keep. See State.News.ReadChecked.
func newsReadChecked(cfg config.Config, now time.Time) string {
	best := mostRecentTeamNewsDate(cfg)
	if best == "" {
		return ""
	}
	read, err := time.Parse("2006-01-02", best)
	if err != nil {
		return ""
	}
	days := int(now.Sub(read).Hours() / 24)
	switch {
	case days <= 0:
		return "Team news last read today"
	case days == 1:
		return "Team news last read yesterday"
	default:
		return fmt.Sprintf("Team news last read %d days ago", days)
	}
}

// mostRecentTeamNewsDate scans every roster and club correction for the
// latest SetOn or LastChecked date. "2006-01-02" sorts lexicographically, so
// a plain string comparison is a correct date comparison.
func mostRecentTeamNewsDate(cfg config.Config) string {
	best := ""
	consider := func(dates ...string) {
		for _, d := range dates {
			if d > best {
				best = d
			}
		}
	}
	for _, o := range cfg.Roster.Exclude {
		consider(o.SetOn, o.LastChecked)
	}
	for _, o := range cfg.Roster.Lock {
		consider(o.SetOn, o.LastChecked)
	}
	for _, o := range cfg.Roster.Minutes {
		consider(o.SetOn, o.LastChecked)
	}
	for _, o := range cfg.Roster.Teams {
		consider(o.SetOn, o.LastChecked)
	}
	return best
}

// watchlistFor ranks the players outside the fifteen against the starter each would
// have to displace.
//
// # Why the benchmark is the weakest STARTER and not the weakest squad member
//
// A transfer has to win a place in the eleven to pay for itself. Measuring a candidate
// against the fifteenth man — a £4.0m bench filler scoring 0.03 — makes every player in
// the league look like an upgrade and the column carries no information. Against the
// weakest starter in his own position it answers the question actually being asked.
//
// # Why excluded players are not in the ranked list
//
// They are not candidates. A standing exclusion is a decision already taken, and
// putting Saliba at the top of the defender list every week — where he would sit, since
// the exclusion does not lower his score — would be an invitation to a transfer the
// page itself forbids. They get their own block, with the reason in full.
//
// # Why one list, not one per position
//
// The watchlist is a single list of the players outside the fifteen, with the
// position as a column the reader can filter and sort on. Per-position groups
// hid how positions trade against each other — the eighth-best keeper is not a
// stronger candidate than the thirtieth-best midfielder just because both were
// eighth on their own list.
//
// # Why the whole pool, not a pre-cut hundred
//
// This builds rows for EVERY player outside the fifteen. The cut to the best
// hundred is a display cap the renderer applies to the unfiltered view, not a
// selection made here: cut here and a filtered query could only ever find
// players who made the hundred — a £4.0m promoted defender scores low and
// would be unfilterable, which is exactly the player a reader filters for.
func watchlistFor(e *analysis.Engine, sq analysis.Squad, excluded []present.Override,
	bound map[int]present.Override, gate float64) *present.Watchlist {

	owned := map[int]bool{}
	for _, p := range sq.Players {
		owned[p.ID] = true
	}
	// Excluded players are removed by code-bound element id, not by name.
	skip := map[int]bool{}
	for id, o := range bound {
		if o.Kind == "exclude" {
			skip[id] = true
		}
	}

	// The benchmark comes from the starting eleven only. A bench keeper is not
	// the keeper a new keeper displaces.
	bench := map[string]present.WatchBenchmark{}
	for _, p := range sq.StartingXI {
		if b, ok := bench[p.Position]; !ok || p.Score < b.Score {
			bench[p.Position] = present.WatchBenchmark{
				Position: p.Position, Name: p.Name, Score: p.Score, Price: p.Price,
			}
		}
	}

	all := e.AllMetrics()
	sort.SliceStable(all, func(i, j int) bool { return all[i].Score > all[j].Score })

	w := &present.Watchlist{Excluded: excluded, Gate: gate}
	for _, m := range all {
		if owned[m.ID] || skip[m.ID] {
			continue
		}
		b := bench[m.Position]
		d := m.Score - b.Score
		// Against the gate, not against zero. A positive gap says "better than
		// what you have"; only the gate says "worth a transfer", and the page
		// prints that threshold two tabs away.
		w.Rows = append(w.Rows, present.WatchRow{
			Player: m, Delta: d, ClearsGate: present.ClearsGate(d, gate),
		})
	}
	for _, r := range w.Rows {
		if r.ClearsGate {
			w.Clearing++
		}
	}
	w.Count = len(w.Rows)
	// The legend names the benchmark per position, in canonical order.
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		if b, ok := bench[pos]; ok {
			w.Benchmarks = append(w.Benchmarks, b)
		}
	}
	return w
}

// reasoningFor is the "why this eleven" view: what a human overrode, where the model
// says it is blind, what it cannot see at all, and the gate it decides under.
//
// Everything here is already computed and already printed once — into a Markdown
// briefing written for a reasoning layer, where a human never sees it. What is left out
// is listed in the design spec and is deliberate: the twenty-club competition table,
// the twenty-club fixture table, the thirty-row league-wide flagged list and the
// agent's own task protocol are either answered better per-player elsewhere on this
// page or are instructions to a machine.
func reasoningFor(cfg config.Config, e *analysis.Engine, squad []analysis.PlayerMetrics,
	live, lapsed []present.Override) *present.Reasoning {

	r := &present.Reasoning{Overrides: live, Lapsed: lapsed}

	for _, c := range e.ResearchTargets(squad) {
		r.Research = append(r.Research, present.ResearchGroup{
			Name: c.Name, Why: c.Why, Ask: c.Ask, Targets: c.Targets,
		})
	}

	// The same four blind spots the briefing states, in the same words. They are a
	// property of the model rather than of a run, which is why they are a literal
	// here and not a computed list — and why they belong on the page at all: a reader
	// deciding from these numbers has to know which questions they cannot answer.
	r.Blind = []string{
		"Transfer news, press conferences, tactical changes, a player who has just lost his place.",
		"New signings from abroad, who carry no Premier League data at all.",
		"The difference between \"injured for three months\" and \"not picked\" — both show as " +
			"low expected minutes, so check minutes-per-start to tell them apart.",
		"Whether a club is still in a competition. That list is hand-maintained and goes " +
			"stale the moment a club is knocked out.",
	}

	if breaks := e.PostBreakGameweeks(); len(breaks) > 0 {
		var gws []int
		for gw := range breaks {
			gws = append(gws, gw)
		}
		sort.Ints(gws)
		var parts []string
		for _, gw := range gws {
			parts = append(parts, fmt.Sprintf("GW%d (%.0f-day break)", gw, breaks[gw]))
		}
		r.Breaks = "Gameweeks following an international break: " + joinComma(parts)
	}

	p := cfg.Review
	r.Policy = present.Policy{
		MinGainTransfer: p.MinGainForTransfer,
		MinGainHit:      p.MinGainForHit,
		BankUpTo:        p.BankUpTo,
		MaxHitsPerWeek:  p.MaxHitsPerWeek,
		Rules:           p.Rules,
	}
	return r
}

func joinComma(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += ", "
		}
		s += p
	}
	return s
}
