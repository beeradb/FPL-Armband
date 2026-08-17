package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"armband/internal/agent"
	"armband/internal/analysis"
	"armband/internal/backtest"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
	"armband/internal/priors"
	"armband/internal/recent"
	"armband/internal/report"
)

const usage = `armband — an AI Fantasy Premier League analyst

Usage:
  armband review            Weekly decision: chips, transfers, injuries, then a verdict
  armband advise            Full pre-deadline analysis, written to a Markdown report
  armband brief             Full deterministic briefing as Markdown (no AI, no API cost)
  armband squad             Run the optimiser only (no AI, no API cost)
  armband transfers         Best transfers for the squad you own, as a team sheet
                            (no AI, no API cost)
  armband fixtures          Fixture difficulty table (no AI, no API cost)
  armband due               Run the review only if a deadline is near (for cron)
  armband schedule          Print a crontab line for the due command
  armband nations           Nationality codes, for post-tournament rest config
  armband priors            Cache last season's totals before FPL overwrites them
  armband capture           Archive the live payload, dated and immutable, so
                            point-in-time questions become answerable later. Run it
                            weekly and ideally shortly before each deadline; every
                            week missed is data that cannot be recovered.
                            Use "capture -list" to audit the series for gaps.
  armband backfill <season|all>
                            Recover the same point-in-time team news for a FINISHED
                            season from the Internet Archive's crawls of FPL, one
                            snapshot per gameweek, always from strictly before the
                            deadline. "-coverage" reports what is on disk without
                            fetching; "-show <gw>" prints one gameweek's availability.
  armband backtest <season> [payoff-gws]
                            Replay a past season, e.g. 2023-24. The optional second
                            argument judges each transfer over that many gameweeks
                            instead of the configured horizon.
  armband congestion        International breaks, turnarounds and European load
  armband verify-competitions
                            Record that competition status was checked against the web,
                            with nothing to correct (no AI, no API cost)
  armband chips             Chip windows, plan validation, blanks and doubles
  armband snapshot          Model-and-harness accuracy snapshot from a sweep's
                            per-cell CSVs (no AI, no API cost, no network).
                            Run with -h for its own flags.
  armband reviewkey         Write a review record's key.csv from the staged index,
                            so TestReviewCoversTheCurrentCode knows what the record
                            covers. Stage your change first. Run with -h for flags.

Flags:
  -config string   Path to config file (default "config.json")
  -no-report       Don't write a Markdown report
  -model string    Override the model from config
  -effort string   Override effort: low|medium|high|xhigh|max
  -refresh         Ignore cached FPL data and refetch
  -plain           Print squads as a plain list rather than a pitch. The list
                   carries per-player detail the pitch has no room for —
                   rotation risk, role factors, set-piece notes
  -html string     Also write the squad, transfer sheet or briefing to a
                   self-contained HTML file — squad, transfers and brief all
                   take it. It opens offline from a file:// URL and fetches
                   nothing
  -weeks int       Gameweeks in the HTML week-by-week view. Defaults to the
                   scoring horizon, and may exceed it — a chip is usually
                   planned outside the horizon, so a wildcard at GW6 is
                   invisible on a five-week view

Examples:
  Flags go BEFORE the command. Go's flag package stops parsing at the command
  name, so a flag placed after it is read by nobody — armband rejects that
  rather than printing a squad and silently writing no file.

  armband squad                     the optimal fifteen, drawn as a pitch
  armband -plain squad              the same as a list, with the per-player
                                     detail the pitch has no room for
  armband -html squad.html squad    and write a self-contained page beside it
  armband -html s.html -weeks 8 squad
                                     an eight-gameweek view, so a wildcard
                                     planned past the horizon is visible
  armband -refresh brief            ignore the cache, then the full briefing
  armband -config alt.json review   the weekly decision, alternate config

  armband squad -html squad.html    WRONG, and an error rather than a squad
                                     printed with the file never written

  Four commands parse their own flags, which therefore go AFTER the command:
  capture, backfill, snapshot and reviewkey. Their flags still come before their
  own positional arguments, for the same reason — a FlagSet stops at the first
  non-flag argument too, so "backfill 2023-24 -coverage" reads -coverage as a
  second season name and crawls the archive it was meant to avoid.

  armband capture -list             audit the capture series for gaps
  armband backfill -coverage all    report what is on disk without fetching

The agent reads Anthropic credentials from the environment: ANTHROPIC_API_KEY,
ANTHROPIC_AUTH_TOKEN, or a profile created with ` + "`ant auth login`" + `.
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
}

// globalFlags are the flags every command shares, as distinct from the ones
// capture, backfill, snapshot and reviewkey register on their own FlagSet.
type globalFlags struct {
	cfgPath  *string
	noReport *bool
	model    *string
	effort   *string
	refresh  *bool
	plain    *bool
	htmlOut  *string
	weeks    *int
}

// registerGlobalFlags registers them on fs. run() passes flag.CommandLine,
// which is exactly what flag.Parse parses, so behaviour is unchanged by this
// being a function.
//
// It is a function so that a TEST can build the same set on a throwaway FlagSet
// and ask Go's own parser what it does. Both guards in usageflags_test.go were
// written first against a hand-model — an AST scan for the flag names, and a
// hand-rolled argument splitter that carried its own list of which flags are
// boolean — and both are the shape AGENTS.md warns about twice over: "one
// quantity, two implementations", and "a diagnostic must never carry its own
// copy of the thing it is checking". fs.VisitAll and fs.Parse replace both.
func registerGlobalFlags(fs *flag.FlagSet) *globalFlags {
	return &globalFlags{
		cfgPath:  fs.String("config", "config.json", "path to config file"),
		noReport: fs.Bool("no-report", false, "don't write a Markdown report"),
		model:    fs.String("model", "", "override model"),
		effort:   fs.String("effort", "", "override effort"),
		refresh:  fs.Bool("refresh", false, "ignore cached FPL data"),
		plain:    fs.Bool("plain", false, "print squads as a plain list rather than a pitch"),
		htmlOut:  fs.String("html", "", "also write the squad to a self-contained HTML file"),
		weeks:    fs.Int("weeks", 0, "gameweeks to show in the HTML week view (default: the scoring horizon)"),
	}
}

func run() error {
	f := registerGlobalFlags(flag.CommandLine)
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	cfgPath, noReport, model, effort := f.cfgPath, f.noReport, f.model, f.effort
	refresh, plain, htmlOut, weeks := f.refresh, f.plain, f.htmlOut, f.weeks

	cmd := flag.Arg(0)
	if cmd == "" || cmd == "help" || cmd == "-h" {
		fmt.Print(usage)
		return nil
	}
	if err := rejectFlagsAfterCommand(cmd, flag.Args()[1:]); err != nil {
		return err
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	// Diagnostic sweeps, before anything is built from the config. See sweep.go.
	applyWeightOverrides(&cfg)

	// The accuracy snapshot reads CSVs off disk and talks to nobody. Dispatched
	// here rather than in the switch below, which runs after the bootstrap and
	// fixture fetches — a reporting command has no business needing the network,
	// and would fail on a plane for no reason.
	if cmd == "snapshot" {
		return runSnapshot(cfg, flag.Args()[1:])
	}
	// Same argument, one step further: writing a review record's key reads the git
	// index and nothing else.
	if cmd == "reviewkey" {
		return runReviewKey(flag.Args()[1:])
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *effort != "" {
		cfg.Effort = *effort
	}

	ttl := time.Duration(cfg.CacheMinutes) * time.Minute
	if *refresh {
		ttl = 0
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := fpl.New(cfg.CacheDir, ttl)

	// Capture is dispatched before the engine is built, and that is deliberate
	// rather than an optimisation. It archives a *moment*, so it must not be
	// downstream of anything that could serve it from cache, and it needs nothing
	// the engine provides — no scoring, no priors, no recency. Building the engine
	// first would also make an archival command fail on an unrelated modelling
	// error, which is the wrong trade for the one command whose input cannot be
	// re-fetched later.
	if cmd == "capture" {
		return cmdCapture(ctx, cfg, client, flag.Args()[1:])
	}

	// Backfill is dispatched here for the same reason, plus one of its own: it
	// talks to the Internet Archive and the season archive and never to FPL, so
	// building an engine from a live bootstrap would be a network round trip and a
	// scoring pass in service of a command that reads neither.
	if cmd == "backfill" {
		return cmdBackfill(ctx, cfg, flag.Args()[1:])
	}

	fmt.Fprint(os.Stderr, dim("Loading FPL data... "))
	boot, err := client.Bootstrap(ctx)
	if err != nil {
		return fmt.Errorf("fetching FPL data: %w", err)
	}
	fixtures, err := client.Fixtures(ctx)
	if err != nil {
		return fmt.Errorf("fetching fixtures: %w", err)
	}
	engine := analysis.NewEngineFull(boot, fixtures, cfg.Weights, cfg.Congestion, cfg.RoleRisk)
	engine.Chips = cfg.Chips
	// A planned free hit fields a different fifteen that week, so the permanent
	// squad is not scored on it. The analysis layer can override the set.
	engine.ApplyFreeHitToScoring()

	// Whose money the model is spending. With no entry there is no squad to
	// price and the budget is openly hypothetical; with one, failing to price it
	// is an error rather than a reason to assume £100m. See Engine.AssemblyBudget.
	engine.Entry = cfg.EntryID
	engine.HypotheticalBudget = int(cfg.HypotheticalBudget*10 + 0.5)

	// Selling prices, preferring the public reconstruction. FPL gives you what
	// you paid plus half of any rise, so a squad cannot be sold for its market
	// value and any search that assumes otherwise is spending money that is not
	// there.
	//
	// The reconstruction needs no credential and, unlike my-team, it can be
	// checked: FPL's own team value is the squad's selling value, so a
	// reconstruction that sums to it is proved right rather than trusted. A
	// session is only a fallback for when it cannot be made exact.
	switch played := engine.GameweeksPlayed(); {
	case cfg.EntryID == 0:
		engine.Budget = analysis.AssumedBudget(
			"No entry_id in config.json, so there is no squad to price.")
	case played == 0:
		// Nothing has been bought, so market price is the selling price.
		engine.Budget = analysis.VerifiedBudget()
	default:
		sp, err := client.SquadPrices(ctx, cfg.EntryID, played)
		switch {
		case err != nil:
			// The error is deliberately NOT interpolated. Every failure from the FPL
			// client formats as "GET /entry/<id>/history/: …", and on the status-code
			// branch it carries up to 200 bytes of FPL's response body — and this
			// string is rendered verbatim into the alerts block of a page the user
			// hands around or screenshots. An entry id is only semi-private, but it
			// is the one path by which account-scoped data reaches that file, and the
			// reader gains nothing from the URL. The full error still goes to stderr.
			fmt.Fprintf(os.Stderr, "\n%s\n", dim("price history unavailable: "+err.Error()))
			engine.Budget = analysis.AssumedBudget(
				"Could not read this squad's price history")
		case sp.Exact():
			engine.SellPrices = sp.Sell
			engine.Bank, engine.SquadValue = &sp.Bank, &sp.Value
			engine.Budget = analysis.VerifiedBudget()
		default:
			// Close but unproven. Use them — they are far better than market
			// prices — and say plainly that they are not exact.
			engine.SellPrices = sp.Sell
			// The bank and the team value are FPL's own figures rather than
			// part of the reconstruction, so both are exact even when the
			// per-player prices that failed the checksum are not. It is only
			// the split between the fifteen that is in doubt, and the squad
			// builder spends the total.
			engine.Bank, engine.SquadValue = &sp.Bank, &sp.Value
			engine.Budget = analysis.DriftingBudget(sp.Drift())
		}
	}
	if w := engine.Budget.Warning(); w != "" {
		fmt.Fprintln(os.Stderr, warn("! "+w))
	}

	// Once the season starts FPL overwrites last season's aggregates, so the model
	// needs a prior to shrink toward. Cached indefinitely, and absent it simply
	// scores on whatever this season has produced so far.
	//
	// Two prior sources, and in the shipped configuration pre-season reaches
	// neither, which is deliberate:
	//
	//   - The multi-season blend (recent.LoadPriors) runs whenever
	//     prior_half_life is set, pre-season included. It ships at 0 — off — because
	//     its benefit is not measurable on the archive: it exists for a star whose
	//     immediate prior season is an injury artefact, and neither replayed season
	//     contains that case. See Weights.PriorHalfLife.
	//   - The single-season cached prior is gated on gameweeks having been played,
	//     because before GW1 FPL's own aggregates *are* last season's totals. A
	//     single-season prior would be a second copy of the numbers already in the
	//     bootstrap, so there is nothing for it to add; the gate is what encodes
	//     that, not an oversight.
	//
	// So engine.Priors is nil pre-season by default, and the scoring path is built
	// for it: blend.go returns before shrinkToLeague when Priors is nil, so nobody
	// is pulled toward a positional mean on the strength of a prior that was never
	// loaded.
	//
	// This is not the recorded bug about priors having to load whatever the season
	// state. That one was about the *blend* being gated on gameweeks played, on the
	// mistaken reasoning that pre-season aggregates already are last season — true
	// of a single-season prior and false of a blend, which is a better summary of
	// last season than last season is. The blend is now ungated, and only the
	// single-season path carries the gate, where the reasoning holds.
	{
		loaded := false
		if cfg.Weights.PriorHalfLife > 0 {
			fmt.Fprint(os.Stderr, dim("Fetching past seasons... "))
			if p, err := recent.LoadPriors(ctx, client, boot,
				cfg.Weights.PriorHalfLife, recent.DefaultConcurrency); err == nil {
				engine.Priors = p
				loaded = true
				fmt.Fprintf(os.Stderr, "%s\n", dim(fmt.Sprintf("%d players", p.Fetched)))
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", dim("unavailable: "+err.Error()))
			}
		}
		if !loaded && engine.GameweeksPlayed() > 0 {
			if s, err := priors.Load(ctx, cfg.CacheDir, priorSeasonName(engine)); err == nil {
				engine.Priors = priors.Adapter{S: s}
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", dim("no prior season available: "+err.Error()+
					" — run `armband priors`"))
			}
		}
	}

	if engine.GameweeksPlayed() > 0 {

		// Minutes are a statement about the present and the bootstrap only
		// publishes season totals, so a player who lost his place in November
		// still reads as an ever-present. Match history fixes that; it costs one
		// request per player who has played, cached, and nothing if it fails.
		fmt.Fprint(os.Stderr, dim("Fetching match history... "))
		if f, err := recent.Load(ctx, client, boot, cfg.Weights.MinutesHalfLife,
			recent.DefaultConcurrency); err == nil {
			engine.Recent = f
			msg := fmt.Sprintf("%d players", f.Fetched)
			if f.Failed > 0 {
				msg += fmt.Sprintf(", %d unavailable (season average used)", f.Failed)
			}
			fmt.Fprintf(os.Stderr, "%s\n", dim(msg))
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", dim("unavailable: "+err.Error()+
				" — minutes fall back to season averages"))
		}
	}
	// Before anything is scored: the corrections change every number that
	// follows, in every command.
	{
		gw := 1
		if next := boot.NextEvent(); next != nil {
			gw = next.ID
		}
		for _, note := range applyMinutesOverrides(cfg, engine, gw) {
			fmt.Fprintf(os.Stderr, "%s\n", dim(note))
		}
	}
	fmt.Fprintf(os.Stderr, "%s\n", dim(fmt.Sprintf("%d players, %d fixtures", len(boot.Elements), len(fixtures))))

	switch cmd {
	case "brief":
		return cmdBrief(ctx, cfg, client, engine, !*noReport, *htmlOut)
	case "squad":
		return cmdSquad(ctx, cfg, client, engine, *plain, *htmlOut, *weeks)
	case "transfers":
		return cmdTransfers(ctx, cfg, client, engine, *plain, *htmlOut)
	case "fixtures":
		return cmdFixtures(engine, cfg)
	case "nations":
		return cmdNations(engine)
	case "priors":
		return cmdPriors(ctx, cfg, engine)
	case "backtest":
		season := flag.Arg(1)
		if season == "" {
			return fmt.Errorf("backtest needs a season, e.g. armband backtest 2023-24")
		}
		// An optional second argument widens the window a transfer is judged
		// over. It changes only the report, never the decisions: the policy
		// still chose on the configured horizon, and rescoring the same moves
		// over a longer window is how you ask whether they were commitments
		// that held up rather than five-week punts.
		payoff := cfg.Weights.Horizon
		if a := flag.Arg(2); a != "" {
			n, err := strconv.Atoi(a)
			if err != nil || n < 1 || n > 38 {
				return fmt.Errorf("payoff horizon must be 1-38 gameweeks, got %q", a)
			}
			payoff = n
		}
		return cmdBacktest(ctx, cfg, season, payoff)
	case "congestion":
		return cmdCongestion(engine)
	case "verify-competitions":
		return cmdVerifyCompetitions(cfg, *cfgPath, engine)
	case "chips":
		return cmdChips(engine, cfg)
	case "advise":
		return cmdAgent(ctx, cfg, *cfgPath, client, engine, advicePrompt(engine), "FPL Advice", !*noReport)
	case "due":
		return cmdDue(ctx, cfg, *cfgPath, client, engine, !*noReport)
	case "schedule":
		return cmdSchedule(cfg)
	case "review":
		return cmdAgent(ctx, cfg, *cfgPath, client, engine, reviewPrompt(engine), "Weekly Review", !*noReport)
	default:
		return fmt.Errorf("unknown command %q — run `armband help`", cmd)
	}
}

// applyRoster puts the standing player overrides onto an optimiser request, so
// the free deterministic commands solve the same problem the agent does.
//
// Without this the two disagree silently: brief prints the overrides at the top
// and then an optimal squad containing the very players they exclude, which is
// worse than not showing them at all.
func applyRoster(cfg config.Config, e *analysis.Engine, req *analysis.OptimizeRequest) []string {
	gw := 1
	if next := e.Boot.NextEvent(); next != nil {
		gw = next.ID
	}
	applyMinutesOverrides(cfg, e, gw)
	lock, exclude, _ := cfg.Roster.Active(gw)
	if len(lock)+len(exclude) == 0 {
		return nil
	}
	byCode := map[int]int{}
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		byCode[el.Code] = el.ID
	}
	var notes []string
	for _, o := range lock {
		id, ok := byCode[o.Code]
		if !ok {
			continue
		}
		if o.MustStart {
			req.StartIDs = append(req.StartIDs, id)
			notes = append(notes, "must start: "+o.Name+" — "+o.Reason)
			continue
		}
		req.LockIDs = append(req.LockIDs, id)
		notes = append(notes, "locked into the squad: "+o.Name+" — "+o.Reason)
	}
	for _, o := range exclude {
		if id, ok := byCode[o.Code]; ok {
			req.ExcludeIDs = append(req.ExcludeIDs, id)
			notes = append(notes, "excluded: "+o.Name+" — "+o.Reason)
		}
	}
	return notes
}

// applyMinutesOverrides installs the analysis layer's minutes corrections on the
// engine, before anything is scored.
//
// This has to happen before Metrics is called for the first time, not as part of
// building an optimiser request, because every score in every command depends on
// it — the briefing tables, the transfer search and the squad all read the same
// engine.
func applyMinutesOverrides(cfg config.Config, e *analysis.Engine, gw int) []string {
	active, _ := cfg.Roster.ActiveMinutes(gw)
	if len(active) == 0 {
		return nil
	}
	if e.MinutesOverride == nil {
		e.MinutesOverride = map[int]float64{}
	}
	if e.MinutesOverrideUntil == nil {
		e.MinutesOverrideUntil = map[int]int{}
	}
	var notes []string
	for _, o := range active {
		if o.ExpectedMinutes == nil {
			continue
		}
		e.MinutesOverride[o.Code] = *o.ExpectedMinutes
		note := fmt.Sprintf("minutes set to %.0f for %s — %s",
			*o.ExpectedMinutes, o.Name, o.Reason)
		// A return date makes the override a claim about specific gameweeks, so
		// it is prorated across the horizon rather than applied to all of it.
		if o.UntilGameweek > 0 {
			e.MinutesOverrideUntil[o.Code] = o.UntilGameweek
			note += fmt.Sprintf(" [through GW%d, prorated across the horizon]", o.UntilGameweek)
		}
		notes = append(notes, note)
	}
	for _, o := range applyTeamOverrides(cfg, e, gw) {
		notes = append(notes, o)
	}
	return notes
}

// applyTeamOverrides applies club-level corrections, currently expected goals
// conceded. A defence that lost the player it was built around keeps its old
// xGC, and no per-player override can say so — the quantity belongs to the club.
func applyTeamOverrides(cfg config.Config, e *analysis.Engine, gw int) []string {
	active, _ := cfg.Roster.ActiveTeams(gw)
	if len(active) == 0 {
		return nil
	}
	if e.TeamXGCFactor == nil {
		e.TeamXGCFactor = map[int]float64{}
	}
	var notes []string
	for _, o := range active {
		if o.XGCFactor <= 0 || o.XGCFactor == 1 {
			continue
		}
		team := e.Boot.TeamByName(o.Team)
		if team == nil {
			notes = append(notes, fmt.Sprintf(
				"team override for %s ignored: no such club in the pool", o.Team))
			continue
		}
		e.TeamXGCFactor[team.ID] = o.XGCFactor
		note := fmt.Sprintf("%s expected goals conceded x%.2f — %s",
			o.Team, o.XGCFactor, o.Reason)
		if o.UntilGameweek > 0 {
			note += fmt.Sprintf(" [through GW%d]", o.UntilGameweek)
		}
		notes = append(notes, note)
	}
	return notes
}

// priorSeasonNames lists the n completed seasons before this one, most recent
// first.
func priorSeasonNames(e *analysis.Engine, n int) []string {
	var out []string
	name := priorSeasonName(e)
	for i := 0; i < n && name != ""; i++ {
		out = append(out, name)
		name = seasonBefore(name)
	}
	return out
}

// seasonBefore turns "2025-2026" into "2024-2025".
//
// seasonBefore is backtest.PriorSeasonName with this caller's error policy.
//
// The format has to be preserved exactly, because it is a cache key and a URL
// path segment upstream. priorSeasonName emits four-digit years; emitting
// "2024-25" instead made every older season a 404, LoadBlended skipped them all,
// and the blend silently degraded to the single season it was meant to improve
// on — with no error and no visible difference except the thing not working.
//
// # It was a third implementation, and the three disagreed
//
// `backtest.PriorSeasonName` returns an error and guards the year; this returned
// `""` silently; `cmd/priorblend`'s copy called `log.Fatalf`. Worse, they derived
// the new end year from different fields — this one from the *end*, the canonical
// one from the *start* — so they agreed only on well-formed input. `"2024-30"`
// gave "2023-24" from one and "2023-29" from the others.
//
// The empty string stays as *this function's* contract, because its only caller
// loops "while the name is non-empty" to walk back through seasons and an error
// there would be noise. What is gone is the second parse.
//
// # The format is preserved, and delegating without that was a live regression
//
// `priorSeasonName` emits FOUR-digit years — "2025-2026" — and `PriorSeasonName`
// always answers in the archive's "YYYY-YY". A first version of this consolidation
// forwarded straight to it, which made the walk return "2025-2026" followed by
// "2024-25": the first element in one format and every later one in the other,
// which is precisely the 404-and-degrade-silently failure the paragraph above
// records. So the *arithmetic* is shared and the *formatting* stays here, because
// the format is this caller's concern and not the archive's.
func seasonBefore(season string) string {
	var start, end int
	if _, err := fmt.Sscanf(season, "%d-%d", &start, &end); err != nil {
		return ""
	}
	// Normalise to the archive's form, do the arithmetic once in the shared
	// function, then restore the caller's width.
	prior, err := backtest.PriorSeasonName(fmt.Sprintf("%d-%02d", start, end%100))
	if err != nil {
		return ""
	}
	if end <= 99 {
		return prior
	}
	var pStart, pEnd int
	if _, err := fmt.Sscanf(prior, "%d-%d", &pStart, &pEnd); err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", pStart, pStart+1)
}

func advicePrompt(e *analysis.Engine) string {
	next := e.Boot.NextEvent()
	gw := "the upcoming gameweek"
	if next != nil {
		gw = next.Name
	}
	return fmt.Sprintf(`Give me your full recommendation for %s.

Work through it properly:
1. Confirm the deadline and season state.
2. Verify competition status before scoring anyone. Which clubs are actually in Europe
   and the domestic cups, over what dates? Correct anything stale — it changes every
   score that follows.
3. Settle the chip plan. Say which gameweek you would use each chip and why. Do this
   before building the squad, not after: a planned wildcard shortens the horizon worth
   optimising for, and a planned bench boost means buying fifteen playable footballers
   rather than eleven and cheap fodder.
4. If I have a squad, review it and tell me what's wrong with it. If I don't, build one.
5. Identify the best transfer or transfers, and say clearly whether any hit is worth taking.
6. Pick a captain and vice-captain, with the reasoning.
7. Flag anyone in my squad or your recommendations who carries real risk this week.
   Check current team news on the web for anyone you are relying on — the model cannot
   see press conferences, and pre-season minutes are last season's.
8. Record what you learned, and re-check what was already recorded. Anywhere you
   overrode the model on availability or role, call set_player_status so it survives to
   next week. Then go through every standing override and verify it against current news
   — confirm it, push the date out after a setback, or clear it if he is fit. A player
   returning early is the one that costs you, because the exclusion quietly holds and he
   never appears in a single list again. Report what you checked and what changed.

Finish with a short "Do this" section listing the concrete actions in order.`, gw)
}

func reviewPrompt(e *analysis.Engine) string {
	next := e.Boot.NextEvent()
	gw := "the upcoming gameweek"
	if next != nil {
		gw = next.Name
	}
	return fmt.Sprintf(`Run the weekly review for %s.

Work the five steps in order — competition status first, then chip strategy,
transfers and budget, availability including current team news from the web, then
the decision. Do not skip ahead to picking a transfer.

The availability step has two halves and the second is easy to skip. First, anything
new the model cannot see — a player ruled out, one who has lost his place, one who has
won one back — recorded with set_player_status rather than only written down.

Second, every standing override gets re-checked against this week's news, not just the
ones about to lapse. An expiry date is a guess made when the injury happened. A player
back early is the expensive miss, because the exclusion holds and nothing draws your
attention to a player who simply never appears in any list. Confirm each one, push the
date out if there has been a setback, or clear it. Say in the report which you checked
and what you found, even when the answer is "no change".

Competition status comes first because it changes the numbers everything else rests
on. If you score players and then find a club is out of Europe, the earlier work is
invalid.

Finish with a clear verdict: either the specific move with the players named, or an
explicit "no move this week", plus the one thing that would change your mind before
the deadline.`, gw)
}

// cmdAgent runs one prompt and writes its answer to a report.
func cmdAgent(ctx context.Context, cfg config.Config, configPath string, client *fpl.Client, engine *analysis.Engine,
	prompt, title string, writeReport bool) error {

	if prompt == "" {
		// Every caller passes a canned prompt. An empty one would ask the model
		// nothing, print nothing and write no report — the silent no-op this
		// package guards against everywhere else.
		return fmt.Errorf("cmdAgent: empty prompt for %q", title)
	}

	if !hasCredentials() {
		return fmt.Errorf("no Anthropic credentials found.\n" +
			"  Set one with:  export ANTHROPIC_API_KEY=sk-ant-...\n" +
			"  Get a key at:  https://platform.claude.com/settings/keys\n" +
			"\nThe `squad` and `fixtures` commands work without credentials.")
	}

	onCall := func(name, summary string) {
		if summary != "" {
			fmt.Fprintf(os.Stderr, "  %s %s %s\n", dim("→"), name, dim("("+summary+")"))
			return
		}
		fmt.Fprintf(os.Stderr, "  %s %s\n", dim("→"), name)
	}

	a, err := agent.New(cfg, configPath, client, engine, onCall)
	if err != nil {
		return err
	}

	meta := map[string]string{"model": cfg.Model, "effort": cfg.Effort}
	if next := engine.Boot.NextEvent(); next != nil {
		meta["gameweek"] = next.Name
		meta["deadline"] = next.DeadlineTime.Format("Mon 2 Jan 2006 15:04 MST")
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", dim("Thinking..."))
	start := time.Now()
	answer, err := a.Ask(ctx, prompt)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", dim(fmt.Sprintf("(%.0fs)", time.Since(start).Seconds())))
	fmt.Println(answer)
	fmt.Fprintf(os.Stderr, "\n%s\n", dim(a.LastUsage.Summary()))

	// No emptiness check: Ask errors rather than returning an empty answer.
	if writeReport {
		path, err := report.Write(cfg.ReportDir, title, answer, meta)
		if err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\n%s\n", dim("Report written to "+path))
	}
	return nil
}

// cmdSquad runs the optimiser with no LLM involvement.
func cmdSquad(ctx context.Context, cfg config.Config, client *fpl.Client,
	e *analysis.Engine, plain bool, htmlPath string, weeks int) error {
	budget, source, err := e.AssemblyBudget()
	if err != nil {
		return err
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
		return err
	}

	// Say what it was allowed to spend, not merely what it spent. A £102.0m
	// squad is a bug at the opening allowance and correct on a wildcard budget,
	// and the reader cannot tell which without the source.
	budgetLine := fmt.Sprintf("Budget £%.1fm: %s", float64(budget)/10, source)

	if htmlPath != "" {
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
		}
		if err := writeSquadHTML(htmlPath, page); err != nil {
			return err
		}
		fmt.Printf("\n%s\n", dim("wrote "+htmlPath))
	}

	// The pitch is the default because a squad is a shape before it is a list.
	// -plain keeps the per-player detail line, which carries rotation risk, role
	// factors and set-piece notes that the pitch has no room for — so it is the
	// view to reach for when a specific number is in question, not a fallback.
	if !plain {
		present.Squad(os.Stdout, *sq, present.TerminalTheme(),
			fmt.Sprintf("Optimal squad over the next %d gameweeks", e.Weights.Horizon))
		fmt.Printf("  %s  %s\n\n", dim("budget "), dim(source))
		return nil
	}

	fmt.Printf("\nOptimal squad — formation %s, £%.1fm spent, £%.1fm left\n",
		sq.Formation, sq.TotalCost, sq.Remaining)
	fmt.Printf("%s\n", budgetLine)
	fmt.Printf("Projected starting XI: %.1f pts/gameweek over the next %d gameweeks\n",
		sq.XIScore, e.Weights.Horizon)
	fmt.Printf("With the captain doubled: %.1f pts/gameweek\n\n", sq.ExpectedPoints)

	fmt.Println("STARTING XI")
	for _, p := range sq.StartingXI {
		printPlayer(p)
	}
	fmt.Println("\nBENCH")
	for _, p := range sq.Bench {
		printPlayer(p)
	}
	fmt.Printf("\nCaptain: %s   Vice: %s\n", sq.Captain.Name, sq.ViceCaptain.Name)
	return nil
}

// writeSquadHTML writes the team sheet to a local file. Deliberately a file and
// not a hosted page: it is self-contained, it opens offline, and nothing about a
// squad needs to leave the machine to be looked at.
// squadBrief is the handful of facts that decide what the squad below means.
//
// Built here rather than in internal/present because every one of them comes from the
// engine or the config, and the renderer is not allowed to reach for either — that is
// what keeps one deadline, one chip plan and one alert list in the program rather than
// two that can disagree.
func squadBrief(cfg config.Config, e *analysis.Engine) *present.Brief {
	b := &present.Brief{Horizon: e.Weights.Horizon}
	gw := 1
	if next := e.Boot.NextEvent(); next != nil {
		gw = next.ID
		b.Gameweek = next.Name
		b.Deadline = next.DeadlineTime.Format("Mon 2 Jan, 15:04 MST")
		if left := time.Until(next.DeadlineTime); left > 0 {
			b.Countdown = humanDuration(left) + " from now"
		}
	}
	for _, ev := range e.Boot.Events {
		if ev.Finished {
			b.Played++
		}
	}
	if b.Played == 0 {
		b.Transfers = "unlimited"
	}
	for _, c := range cfg.Chips.Entries() {
		b.Chips = append(b.Chips, fmt.Sprintf("%s GW%d", strings.ToLower(c.Label), c.GW))
	}
	lock, exclude, _ := cfg.Roster.Active(gw)
	mins, _ := cfg.Roster.ActiveMinutes(gw)
	teams, _ := cfg.Roster.ActiveTeams(gw)
	b.Overrides = len(lock) + len(exclude) + len(mins) + len(teams)

	// Alerts are things that should change a decision, not a summary of the page.
	if w := e.Budget.Warning(); w != "" {
		b.Alerts = append(b.Alerts, w+" — affordability below is unconfirmed.")
	}
	if b.Played == 0 {
		b.Alerts = append(b.Alerts, "Pre-season: every aggregate is last season's, "+
			"so players who changed clubs carry their old club's output and arrivals "+
			"from abroad show nothing at all.")
	}
	// Singularised rather than "override(s)". A parenthesised plural is the one
	// register on this page that reads like a form letter, and this line is the
	// page's own statement that the squad below is not the model's unaided answer —
	// which is the last sentence that should sound automated.
	if b.Overrides > 0 {
		noun, verb := "overrides", "bind"
		if b.Overrides == 1 {
			noun, verb = "override", "binds"
		}
		b.Alerts = append(b.Alerts, fmt.Sprintf("%d hand-set %s %s this squad; "+
			"it is not the model's unaided answer.", b.Overrides, noun, verb))
	}
	return b
}

func writeSquadHTML(path string, p present.Page) error {
	// 0600 rather than os.Create's 0644. The page carries the full text of every
	// standing override reason, FPL's injury notes, and the squad's value and bank —
	// none of it a credential, all of it this user's, and on a shared host 0644 hands
	// it to every other local account. It grew a great deal when the reasoning and
	// watchlist views were added; the mode had not been revisited since it was a
	// team sheet.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write squad HTML: %w", err)
	}
	defer f.Close()
	if err := present.Render(f, p); err != nil {
		return fmt.Errorf("render squad HTML: %w", err)
	}
	return f.Close()
}

func printPlayer(p analysis.PlayerMetrics) {
	var notes []string
	if p.RotationRisk != "" {
		notes = append(notes, p.RotationRisk)
	}
	if p.NewSigning {
		notes = append(notes, "new signing")
	}
	if p.RoleFactor < 1 {
		notes = append(notes, fmt.Sprintf("role risk x%.2f", p.RoleFactor))
	}
	if p.RestRisk != "" {
		notes = append(notes, "rest risk")
	}
	if p.MatchesAvailable < analysis.GameweeksPerSeason {
		notes = append(notes, fmt.Sprintf("mins /%d, %s", p.MatchesAvailable, p.TournamentAbsence))
	}
	if p.SetPieceNote != "" {
		notes = append(notes, p.SetPieceNote)
	}
	// How many matches the club plays per gameweek across the horizon — above 1 is
	// a double gameweek, below 1 a blank. Worth up to a factor of two on what a
	// player returns, and until now the one scoring term that appeared in no output
	// at all. Silent for an ordinary fixture run, which is nearly everyone.
	//
	// Phrased as a fixture count and not as a multiplier on the score printed beside
	// it, because at the shipped horizon it is *not* in that score: the term is
	// confined to the imminent-week eleven and the transfer objective. Calling it
	// "x1.40 on the score" here would be plainly wrong.
	if analysis.FixtureLoadIsNotable(p.FixtureLoad) {
		notes = append(notes, fmt.Sprintf("plays %.2f matches/gw", p.FixtureLoad))
	}
	suffix := ""
	if len(notes) > 0 {
		suffix = "  " + dim(strings.Join(notes, ", "))
	}
	fmt.Printf("  %-3s  %-18s %-4s  £%5.1fm  %5.2f pts/gw  %4.0f min/gw  fdr %.1f%s\n",
		p.Position, p.Name, p.Team, p.Price, p.Score, p.ExpectedMinutes, p.AvgDifficulty, suffix)
}

// cmdNations lists FPL's opaque nationality codes alongside example players, so
// you can populate rest_regions in config after a summer tournament.
func cmdNations(e *analysis.Engine) error {
	type group struct {
		region  int
		players []string
		count   int
	}
	byRegion := map[int]*group{}
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.Region == nil {
			continue
		}
		g := byRegion[*el.Region]
		if g == nil {
			g = &group{region: *el.Region}
			byRegion[*el.Region] = g
		}
		g.count++
		// Show the most recognisable names: those with the most minutes.
		if el.Minutes > 1500 && len(g.players) < 4 {
			g.players = append(g.players, el.WebName)
		}
	}

	var gs []*group
	for _, g := range byRegion {
		gs = append(gs, g)
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i].count > gs[j].count })

	fmt.Println("\nFPL nationality codes (the API publishes no country names).")
	fmt.Println("Identify a nation by its example players, then add the code to")
	fmt.Println("\"rest_regions\" in config.json to apply a post-tournament discount.")
	fmt.Println()
	fmt.Printf("  %-8s %-7s %s\n", "code", "players", "examples")
	for _, g := range gs {
		if g.count < 3 {
			continue
		}
		fmt.Printf("  %-8d %-7d %s\n", g.region, g.count, strings.Join(g.players, ", "))
	}
	return nil
}

// cmdFixtures prints the fixture difficulty table.
func cmdFixtures(e *analysis.Engine, cfg config.Config) error {
	n := cfg.Weights.Horizon

	type run struct {
		team string
		avg  float64
		fx   []analysis.FixtureBrief
	}
	var runs []run
	for i := range e.Boot.Teams {
		t := &e.Boot.Teams[i]
		fx := e.TeamFixtures(t.ID, n)
		var sum int
		for _, f := range fx {
			sum += f.Difficulty
		}
		avg := 0.0
		if len(fx) > 0 {
			avg = float64(sum) / float64(len(fx))
		}
		runs = append(runs, run{t.ShortName, avg, fx})
	}
	for i := 0; i < len(runs); i++ {
		for j := i + 1; j < len(runs); j++ {
			if runs[j].avg < runs[i].avg {
				runs[i], runs[j] = runs[j], runs[i]
			}
		}
	}

	fmt.Printf("\nFixture difficulty, next %d gameweeks (easiest run first)\n\n", n)
	for _, r := range runs {
		fmt.Printf("  %-4s  %.2f  ", r.team, r.avg)
		for _, f := range r.fx {
			loc := "a"
			if f.Home {
				loc = "H"
			}
			fmt.Printf("%s%s(%d) ", f.Opponent, loc, f.Difficulty)
		}
		fmt.Println()
	}
	return nil
}

func hasCredentials() bool {
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	// An `ant auth login` profile also works; the SDK finds it on disk.
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if _, err := os.Stat(home + "/.config/anthropic/credentials"); err == nil {
		return true
	}
	return false
}

// warn is deliberately not dim. An unverified budget is the one condition that
// must not blend into the loading chatter: it is silent, plausible and wrong.
func warn(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return "\033[1;33m" + s + "\033[0m"
}

func dim(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return "\033[2m" + s + "\033[0m"
}

func fromGW(next *fpl.Event) int {
	if next == nil {
		return 1
	}
	return next.ID
}

func mustParse(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// cmdCongestion shows what the congestion model derived from the calendar, and
// what it still needs configuring.
func cmdCongestion(e *analysis.Engine) error {
	breaks := e.PostBreakGameweeks()
	var gws []int
	for gw := range breaks {
		gws = append(gws, gw)
	}
	sort.Ints(gws)

	fmt.Println("\nDERIVED FROM THE CALENDAR")
	fmt.Println("\n  Gameweeks following an international break:")
	if len(gws) == 0 {
		fmt.Println("    none found")
	}
	for _, gw := range gws {
		fmt.Printf("    GW%-2d  (%.0f-day break beforehand)\n", gw, breaks[gw])
	}

	horizon := e.Weights.Horizon
	next := e.Boot.NextEvent()
	from := 1
	if next != nil {
		from = next.ID
	}
	fmt.Printf("\n  Tight turnarounds inside the GW%d-%d horizon:\n", from, from+horizon-1)
	found := false
	for i := range e.Boot.Teams {
		t := &e.Boot.Teams[i]
		for _, f := range e.TeamFixtures(t.ID, horizon) {
			if d, ok := e.RestDays(t.ID, f.Event); ok && d < 4 {
				fmt.Printf("    %-4s GW%-2d  %.1f days' rest\n", t.ShortName, f.Event, d)
				found = true
			}
		}
	}
	if !found {
		fmt.Println("    none — every club has 4+ days between league fixtures")
	}

	printCompetitionStatus(e)

	fmt.Println("\nREQUIRES CONFIGURATION (not available from the FPL API)")
	cg := e.Cong
	fmt.Printf("\n  Clubs with a European campaign: %d\n", len(cg.European))
	fmt.Printf("  Clubs with a domestic cup campaign: %d\n", len(cg.DomesticCups))
	fmt.Printf("\n  Long-haul nationality codes: %v\n", cg.LongHaulRegions)
	fmt.Printf("  Regular-international codes: %v\n", cg.RegularIntlRegions)
	if len(cg.LongHaulRegions) == 0 {
		fmt.Println("    Run `armband nations` to map codes to countries, then set")
		fmt.Println("    \"congestion.long_haul_regions\" for South America, Africa, Asia.")
	}

	fmt.Printf("\n  Penalties: UCL %.2f  UEL %.2f  UECL %.2f  <4d rest %.2f  <3d rest %.2f\n",
		cg.UCLPenalty, cg.UELPenalty, cg.UECLPenalty, cg.ShortRestPenalty, cg.VeryShortRest)
	fmt.Printf("             post-break %.2f  long-haul return %.2f\n",
		cg.PostBreakPenalty, cg.LongHaulPenalty)
	return nil
}

// printCompetitionStatus lists which clubs are committed to European and
// domestic cup competitions, over what dates, and how stale that record is.
// Shared by cmdCongestion (read-only) and cmdVerifyCompetitions (which also
// stamps it as freshly checked).
func printCompetitionStatus(e *analysis.Engine) {
	now := time.Now()
	fmt.Println("\nCOMPETITION STATUS")
	if days, ok := e.StatusAge(); ok {
		stale := ""
		if days > 7 {
			stale = "  <-- stale, re-verify"
		}
		fmt.Printf("  last verified %d days ago%s\n", days, stale)
	} else {
		fmt.Println("  never verified — run `armband review` to refresh from the web")
	}

	var clubs []string
	for i := range e.Boot.Teams {
		clubs = append(clubs, e.Boot.Teams[i].ShortName)
	}
	sort.Strings(clubs)
	fmt.Println()
	for _, c := range clubs {
		windows := append(append([]analysis.CompetitionWindow{}, e.Cong.European[c]...),
			e.Cong.DomesticCups[c]...)
		if len(windows) == 0 {
			continue
		}
		var parts []string
		for _, w := range windows {
			state := "upcoming"
			switch {
			case w.End != "" && now.After(mustParse(w.End)):
				state = "finished"
			case w.Active(now):
				state = "ACTIVE"
			}
			span := w.Start
			if w.End != "" {
				span += "→" + w.End
			}
			parts = append(parts, fmt.Sprintf("%s %s [%s]", w.Competition, span, state))
		}
		fmt.Printf("  %-4s %s\n", c, strings.Join(parts, ", "))
	}
}

// cmdVerifyCompetitions is the free, deterministic path to recording that
// competition status was checked against the web — the agent tool
// (update_competition_status) does this too, but only as a side effect of an
// LLM run, and only when a window actually needs correcting. A human who
// checked and found nothing to change had no way to record that without
// paying for a review. This shows current status, then stamps LastVerified
// as of today and saves it — no windows change, no LLM call.
func cmdVerifyCompetitions(cfg config.Config, cfgPath string, e *analysis.Engine) error {
	printCompetitionStatus(e)

	today := time.Now().Format("2006-01-02")
	cfg.Congestion = e.MarkCompetitionsVerified(today)

	if cfgPath == "" {
		fmt.Println("\nNo config path set — status shown but not saved.")
		return nil
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("\nStamped as verified today (%s) and saved to %s.\n", today, cfgPath)
	fmt.Println("Run this after checking competition status against the web with nothing to correct — " +
		"it costs nothing. Use `update_competition_status` (via review or advise) when a window actually needs changing.")
	return nil
}

// cmdChips shows chip windows, validates the configured plan, and reports
// whether any blank or double gameweeks are known yet.
func cmdChips(e *analysis.Engine, cfg config.Config) error {
	nextGW := 1
	if n := e.Boot.NextEvent(); n != nil {
		nextGW = n.ID
	}

	fmt.Printf("\nCHIP WINDOWS (upcoming half, from GW%d)\n\n", nextGW)
	for _, w := range e.ChipWindows() {
		fmt.Printf("  %-15s GW%d-%d\n", w.Label, w.Start, w.Stop)
	}

	plan := cfg.Chips
	fmt.Println("\nYOUR PLAN")
	// Canonical order — first set then second, each in the order a season plays
	// them — rather than alphabetical, which interleaved the two sets and read
	// as one muddled list.
	entries := plan.Entries()
	for _, c := range entries {
		fmt.Printf("  %-25s GW%d\n", c.Label, c.GW)
	}
	if len(entries) == 0 {
		fmt.Println("  nothing planned — set \"chip_plan\" in config.json")
	}

	if problems := e.ValidateChipPlan(plan); len(problems) > 0 {
		fmt.Println("\nISSUES")
		for _, p := range problems {
			fmt.Printf("  - %s\n", p)
		}
	} else {
		fmt.Println("\n  Plan is legal: every chip has a distinct gameweek inside its window.")
	}

	if h, why := e.EffectiveHorizon(plan); why != "" {
		fmt.Printf("\nEFFECT ON SQUAD BUILDING\n\n  %s\n", why)
		fmt.Printf("  Optimiser horizon reduced to %d gameweeks.\n", h)
	}
	if _, why := e.SuggestBenchWeight(plan); why != "" {
		fmt.Printf("  %s\n", why)
	}

	// Blanks and doubles determine when Free Hit and Bench Boost are worth most.
	fmt.Println("\nBLANK / DOUBLE GAMEWEEKS")
	counts := map[int]int{}
	for _, f := range e.Fixtures {
		if f.Event != nil {
			counts[*f.Event]++
		}
	}
	irregular := false
	var gws []int
	for gw := range counts {
		gws = append(gws, gw)
	}
	sort.Ints(gws)
	for _, gw := range gws {
		if counts[gw] != 10 {
			kind := "BLANK"
			if counts[gw] > 10 {
				kind = "DOUBLE"
			}
			fmt.Printf("  GW%-2d %s (%d fixtures)\n", gw, kind, counts[gw])
			irregular = true
		}
	}
	if !irregular {
		fmt.Println("  None scheduled yet — every club plays exactly once in every gameweek.")
		fmt.Println("  Blanks and doubles appear later, once cup progress forces postponements,")
		fmt.Println("  so a Free Hit held for a blank is a bet that one materialises before it expires.")
	}
	return nil
}

// cmdPriors caches a completed season's totals and reports the join quality.
//
// FPL carries last season's aggregates only until GW1 completes, then
// overwrites them. Once that happens there is no way to recover them from the
// official API, so this wants running before the first deadline of a season.
func cmdPriors(ctx context.Context, cfg config.Config, e *analysis.Engine) error {
	season := priorSeasonName(e)
	fmt.Printf("\nLoading %s totals (cached indefinitely — a finished season does not change)\n", season)

	s, err := priors.Load(ctx, cfg.CacheDir, season)
	if err != nil {
		return fmt.Errorf("loading priors for %s: %w", season, err)
	}

	withMins := 0
	for _, p := range s.Players {
		if p.Minutes > 0 {
			withMins++
		}
	}
	fmt.Printf("  %d players, %d with minutes\n\n", len(s.Players), withMins)

	// Cross-check against the live API. Before GW1 the two describe the same
	// thing and must agree; afterwards FPL has moved on and they cannot.
	played := e.GameweeksPlayed()
	var matched, differ, orphan int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		p, ok := s.Get(el.Code)
		if !ok {
			if el.Minutes > 0 {
				orphan++
			}
			continue
		}
		matched++
		if p.Minutes != el.Minutes || p.Starts != el.Starts {
			differ++
		}
	}
	fmt.Printf("  matched to the current squad by player code: %d\n", matched)
	fmt.Printf("  live players with minutes but no prior:      %d\n", orphan)
	if played == 0 {
		fmt.Printf("  disagreements with the live API:             %d", differ)
		if matched > 0 {
			fmt.Printf(" (%.1f%%)", float64(differ)/float64(matched)*100)
		}
		fmt.Println("\n\n  Pre-season, so FPL still holds these same totals and the two should agree.")
		fmt.Println("  The few that differ are players FPL has re-entered with a blank record —")
		fmt.Println("  exactly the history the prior exists to keep.")
	} else {
		fmt.Printf("\n  %d gameweeks played, so FPL has overwritten last season and no longer\n", played)
		fmt.Println("  agrees with the prior. That is the point of having it.")
	}
	fmt.Println()
	return nil
}

// priorSeasonName is the season before the one now being played.
func priorSeasonName(e *analysis.Engine) string {
	start := time.Now()
	if next := e.Boot.NextEvent(); next != nil {
		start = next.DeadlineTime
	}
	y := start.Year()
	if start.Month() < time.July {
		y-- // January to June still belongs to the season that began last year
	}
	return fmt.Sprintf("%d-%d", y-1, y)
}

// commandsThatParseTheirOwnFlags are the subcommands that read flag.Args()[1:]
// themselves. Everything after the command name belongs to them and must not be
// second-guessed here.
var commandsThatParseTheirOwnFlags = map[string]bool{
	"snapshot":  true, // runSnapshot has its own FlagSet
	"reviewkey": true, // runReviewKey takes -out and -rev
	"capture":   true, // cmdCapture takes -list and friends
	"backfill":  true, // cmdBackfill takes -coverage, -per-gameweek and friends
}

// rejectFlagsAfterCommand turns a silent no-op into an error.
//
// Go's flag package stops parsing at the first non-flag argument, so
// `armband squad -html out.html` parses ZERO flags: the command is "squad" and
// `-html out.html` sits unread in flag.Args(). Nothing errors, the squad prints
// normally, and the file the user asked for is never written. That is this
// project's most-repeated failure shape — a request that looks like it worked.
//
// The rule is documented AND guarded, because the two answer different
// questions: the worked examples under "Examples:" say what to type, and this
// says what went wrong to someone who typed something else.
// TestEveryGlobalFlagAppearsInTheUsageText keeps that list honest.
//
// ⚠️ This comment used to argue the opposite — that the ordering was "worth a
// guard rather than a line in the usage text, because the usage text is what
// someone reads once and the mistake is one they make every time". The premise
// is right and the conclusion does not follow: a guard states the rule only to
// someone who has ALREADY broken it. The cost was visible in the tree rather
// than hypothetical. -plain and -html arrived at dbeee98 and -weeks at 7d346f9;
// 4546732 — "Docs as reference: a replay guide, and sixteen claims that were not
// true" — then rewrote README.md's flag sentence without checking it against the
// code, leaving five of eight listed. So the flag that writes the HTML page was
// documented in this string and nowhere else, and the ordering rule it needs was
// documented nowhere at all.
func rejectFlagsAfterCommand(cmd string, rest []string) error {
	if commandsThatParseTheirOwnFlags[cmd] {
		return nil
	}
	for _, a := range rest {
		if len(a) > 1 && strings.HasPrefix(a, "-") {
			return fmt.Errorf("flags must come before the command: try "+
				"`armband %s %s` rather than `armband %s %s`.\n"+
				"Go's flag package stops parsing at the command name, so %s "+
				"would be ignored silently", a, cmd, cmd, a, a)
		}
	}
	return nil
}

// readAnalysis loads the reasoning layer's verdict, if one has been written.
//
// A path rather than generated text, because this binary does not judge: internal
// /analysis is pure computation and the whole point of the split is that it never
// pretends otherwise. Whoever ran the review writes their verdict to a file and the
// page carries it; nothing here invents one, and an absent file simply means the
// page shows the model's output with no judgement attached, which is honest.
func readAnalysis() string {
	path := os.Getenv("FPL_ANALYSIS_MD")
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("%s\n", dim("analysis file unreadable, page will omit it: "+err.Error()))
		return ""
	}
	return string(b)
}
