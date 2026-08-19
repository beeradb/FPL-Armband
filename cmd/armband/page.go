package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
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
}

// buildSquadPage runs the optimiser and assembles the squad page.
//
// This is the one pipeline behind both `armband squad -html` and `armband
// serve`, so a page written to disk and a page served over HTTP cannot drift
// into two pages. Everything in it reads config or the engine and hands
// internal/present flat structs it can only draw — the boundary this file
// exists to keep.
func buildSquadPage(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine, weeks int) (squadPageBuild, error) {

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
		MinMinutes:         600,
		MinExpectedMinutes: 55,
	}
	for _, note := range applyRoster(cfg, e, &req) {
		fmt.Printf("\n%s\n", dim(note))
	}
	for _, note := range e.ApplyChipPlan(&req) {
		fmt.Printf("\n%s\n", dim("chip plan: "+note))
	}
	sq, err := e.Optimize(req)
	if err != nil {
		return squadPageBuild{}, err
	}

	// Say what it was allowed to spend, not merely what it spent. A £102.0m
	// squad is a bug at the opening allowance and correct on a wildcard budget,
	// and the reader cannot tell which without the source.
	budgetLine := fmt.Sprintf("Budget £%.1fm: %s", float64(budget)/10, source)

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
	views := e.WeekViews(sq.Players, span)
	// Projected transfers for the squad you actually own, when there is one.
	// The reason there is not is carried through and printed, because an
	// absent section cannot distinguish "no squad yet" from "no move is
	// worth making", and the second is a recommendation.
	plan, why := bestPlanForOwnedSquad(ctx, cfg, client, e)

	// The two views behind the eleven. Built here because every value in them
	// comes from config or the engine, and the renderer may reach for neither.
	bound, live, lapsed := pageOverrides(cfg, e, sq.Players, time.Now())
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
	return squadPageBuild{Page: page, Squad: sq, BudgetLine: budgetLine, Source: source}, nil
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
	// and re-deciding it here would be a second implementation of Expired.
	player := func(kind, label string, o config.RosterOverride, lapsedNow bool) present.Override {
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
			if !lapsedNow {
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
		live = append(live, player("lock", label, o, false))
	}
	for _, o := range exclude {
		live = append(live, player("exclude", "EXCL", o, false))
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
		live = append(live, player("minutes", label, o, false))
	}
	for _, o := range expired {
		lapsed = append(lapsed, player("lapsed", "LAPSED", o, true))
	}
	for _, o := range minsExpired {
		lapsed = append(lapsed, player("lapsed", "LAPSED MIN", o, true))
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
	return "indefinite — review"
}

func withAge(lastChecked string, age int) string {
	s := checkedOrNever(lastChecked)
	if age > 0 {
		s = fmt.Sprintf("%s (%dd)", s, age)
	}
	return s
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
// The watchlist is a single list of the hundred best-scoring players outside the
// fifteen, with the position as a column the reader can filter and sort on. Per-
// position groups hid how positions trade against each other — the eighth-best
// keeper is not a stronger candidate than the thirtieth-best midfielder just
// because both were eighth on their own list.
const watchCount = 100

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
		if len(w.Rows) >= watchCount {
			break
		}
		if owned[m.ID] || skip[m.ID] {
			continue
		}
		b := bench[m.Position]
		d := m.Score - b.Score
		// Against the gate, not against zero. A positive gap says "better than
		// what you have"; only the gate says "worth a transfer", and the page
		// prints that threshold two tabs away.
		w.Rows = append(w.Rows, present.WatchRow{
			Player: m, Delta: d, ClearsGate: gate > 0 && d >= gate,
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
