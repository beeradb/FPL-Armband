package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"armband/internal/present"
	"armband/internal/report"
)

// cmdBrief writes everything the deterministic model knows as one self-contained
// Markdown document, for handing to a reasoning layer that is not this program's
// API client — a Claude Code session, say.
//
// It costs nothing to run, which is the point: internal/analysis never calls the
// LLM, so the only thing the API was ever paying for was the judgement on top.
// The brief carries the same standing brief, the same five-step protocol and the
// same numbers the agent tools expose, so the two paths should reach the same
// decision from the same evidence.
//
// It is deliberately verbose. Context is cheap for a human-driven session and a
// follow-up round trip is not, so the brief errs toward self-contained rather
// than minimal — the opposite of the tool-output rule, where every field is
// replayed on every subsequent API call and paid for repeatedly.
func cmdBrief(ctx context.Context, cfg config.Config, client *fpl.Client, e *analysis.Engine, writeReport bool, htmlPath string) error {
	var b strings.Builder
	now := time.Now()

	next := e.Boot.NextEvent()
	gwName, deadline := "unknown", "unknown"
	if next != nil {
		gwName = next.Name
		deadline = next.DeadlineTime.Format("Mon 2 Jan 2006 15:04 MST")
	}

	fmt.Fprintf(&b, "# FPL briefing — %s\n\n", gwName)
	fmt.Fprintf(&b, "*Generated %s. Every number below is deterministic model output; "+
		"no judgement has been applied to it yet.*\n\n", now.Format("Mon 2 Jan 2006 15:04 MST"))

	fmt.Fprintf(&b, "- **Deadline**: %s", deadline)
	if next != nil {
		if left := time.Until(next.DeadlineTime); left > 0 {
			fmt.Fprintf(&b, " — %s from now", humanDuration(left))
		} else {
			b.WriteString(" — **passed**")
		}
	}
	b.WriteString("\n")

	played := 0
	for _, ev := range e.Boot.Events {
		if ev.Finished {
			played++
		}
	}
	fmt.Fprintf(&b, "- **Gameweeks completed**: %d of 38\n", played)
	if played == 0 {
		b.WriteString("- **Pre-season**: every aggregate below is *last season's* total. " +
			"Players who changed clubs carry their old club's output, players arriving from " +
			"abroad show nothing at all, and set-piece hierarchies may already have changed.\n")
	}
	fmt.Fprintf(&b, "- **Scoring horizon**: %d gameweeks\n", e.Weights.Horizon)
	// An unverified budget is stated here, at the top, alongside the other
	// things that qualify every number below it. Burying it in a log line means
	// a run that quietly lost its session produces a report that looks exactly
	// like a good one.
	if w := e.Budget.Warning(); w != "" {
		fmt.Fprintf(&b, "\n> **%s**\n>\n"+
			"> Every transfer suggestion below may spend money this squad does not have. "+
			"FPL pays what you paid plus half of any rise, never the market price. "+
			"Treat affordability as unconfirmed and check it against the site before acting.\n",
			w)
	}
	b.WriteString("\n")

	briefTask(&b, cfg, gwName)
	briefOverrides(&b, cfg, e, now)
	briefCompetitions(&b, e, now)
	briefChips(&b, e, cfg)
	briefSquad(ctx, &b, client, e, cfg)
	briefConcerns(&b, e)
	optimal, err := briefOptimal(&b, cfg, e)
	if err != nil {
		return err
	}
	briefResearch(&b, e, optimal)
	briefFixtures(&b, e)
	briefBlindSpots(&b, e)

	out := b.String()
	fmt.Print("\n" + out)

	if writeReport {
		meta := map[string]string{"gameweek": gwName, "deadline": deadline}
		path, err := report.Write(cfg.ReportDir, "FPL Briefing", out, meta)
		if err != nil {
			return err
		}
		fmt.Printf("\n%s\n", dim("Briefing written to "+path))
	}
	// The same document as a page, when one is asked for. Markdown stays the
	// primary form: the brief exists to be handed to a reasoning layer, and that
	// reads Markdown better than it reads HTML. The page is for a human.
	if htmlPath != "" {
		f, err := os.Create(htmlPath)
		if err != nil {
			return fmt.Errorf("write briefing HTML: %w", err)
		}
		defer f.Close()
		sub := fmt.Sprintf("%s · deadline %s", gwName, deadline)
		if err := present.Doc(f, out, "FPL briefing — "+gwName, sub); err != nil {
			return fmt.Errorf("render briefing HTML: %w", err)
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Printf("%s\n", dim("Briefing page written to "+htmlPath))
	}
	return nil
}

// briefOverrides lists the standing player locks and exclusions.
//
// These bind every squad build and transfer search, so a briefing that omits
// them is describing a different model from the one that produced the numbers
// below it — a player can be absent from every table here for a reason recorded
// weeks ago, and without this section there is nothing to read that explains it.
func briefOverrides(b *strings.Builder, cfg config.Config, e *analysis.Engine, now time.Time) {
	gw := 1
	if next := e.Boot.NextEvent(); next != nil {
		gw = next.ID
	}
	lock, exclude, expired := cfg.Roster.Active(gw)
	// Minutes and team corrections bind the squad exactly as locks and exclusions do,
	// and for a while this section listed neither.
	//
	// That is the failure the section exists to prevent, arriving inside it. On the
	// 2026-08-13 GW1 brief it listed two exclusions and omitted six minutes overrides
	// and one club correction — and two of the omitted six, Kinsky at 88 minutes and
	// van Ewijk at 85, were the reason those players appeared in the squad at all.
	// Kinsky's own note says he "scores 0.41 at £4.5m off 630 minutes as a backup". A
	// reader was shown a starting keeper with no way to learn that a human had put him
	// there. Every kind of override is listed here now; `Roster` has four and this
	// section reads all four.
	minutes, minutesExpired := cfg.Roster.ActiveMinutes(gw)
	teams, teamsExpired := cfg.Roster.ActiveTeams(gw)
	if len(lock)+len(exclude)+len(expired)+
		len(minutes)+len(minutesExpired)+len(teams)+len(teamsExpired) == 0 {
		return
	}

	b.WriteString("\n## Standing player overrides\n\n")
	b.WriteString("Set by earlier reviews and binding on every squad build and transfer " +
		"search below. An expiry date is a guess made when the injury happened and it is " +
		"wrong in both directions: a setback lets the override lapse while the player is " +
		"still out, and a player back early is never reconsidered at all. Anything marked " +
		"**CHECK** is due a look at this week's news.\n\n")
	b.WriteString("| | Player | Reason | Set | Lapses | Last checked | |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")

	row := func(kind string, o config.RosterOverride) {
		until := "indefinite"
		if o.UntilGameweek > 0 {
			until = fmt.Sprintf("after GW%d", o.UntilGameweek)
		}
		checked := o.LastChecked
		if checked == "" {
			checked = "never"
		}
		if age := o.CheckAge(now); age > 0 {
			checked = fmt.Sprintf("%s (%dd)", checked, age)
		}
		flag := ""
		if o.NeedsCheck(now, gw) {
			flag = "**CHECK**"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			kind, o.Name, o.Reason, o.SetOn, until, checked, flag)
	}
	for _, o := range lock {
		row("locked in", o)
	}
	for _, o := range exclude {
		row("excluded", o)
	}
	// Minutes overrides are labelled with the value they set, because "minutes" alone
	// does not say whether a player was written up or written down — 88 for a backup
	// keeper and 15 for an injured defender are opposite interventions and the reader
	// needs to see which one is holding a player in the squad below.
	for _, o := range minutes {
		kind := "minutes"
		if o.ExpectedMinutes != nil {
			kind = fmt.Sprintf("minutes → %.0f", *o.ExpectedMinutes)
		}
		row(kind, o)
	}
	for _, o := range expired {
		row("lapsed", o)
	}
	for _, o := range minutesExpired {
		row("lapsed (minutes)", o)
	}
	// Club corrections are not player rows, so they get their own line rather than
	// being forced into the table's shape.
	for _, o := range append(append([]config.TeamOverride(nil), teams...), teamsExpired...) {
		lapsed := ""
		if o.UntilGameweek > 0 && gw > o.UntilGameweek {
			lapsed = " — **lapsed**"
		}
		what := o.Team
		if o.XGCFactor > 0 {
			what = fmt.Sprintf("%s expected goals conceded ×%.2f", o.Team, o.XGCFactor)
		}
		fmt.Fprintf(b, "| club | %s | %s | %s | %s | %s |%s\n",
			what, o.Reason, o.SetOn, until(o.UntilGameweek), checkedOrNever(o.LastChecked), lapsed)
	}
	b.WriteString("\n")
}

func until(gw int) string {
	if gw > 0 {
		return fmt.Sprintf("after GW%d", gw)
	}
	return "indefinite"
}

func checkedOrNever(s string) string {
	if s == "" {
		return "never"
	}
	return s
}

// briefTask states the standing brief and the protocol, so whoever reasons over
// this works to the same rules the agent does rather than inventing its own.
func briefTask(b *strings.Builder, cfg config.Config, gwName string) {
	fmt.Fprintf(b, "---\n\n## Your task\n\nRun the weekly review for %s and finish with a "+
		"clear verdict: the specific move with players named, or an explicit \"no move\", "+
		"plus the one thing that would change your mind before the deadline.\n\n", gwName)

	b.WriteString("Work these steps in order. Each constrains the next, and jumping " +
		"straight to \"which transfer\" is how managers burn transfers on problems a " +
		"planned wildcard would have fixed for free.\n\n")
	b.WriteString("0. **Competition status** — is it still accurate? It changes every score below.\n")
	b.WriteString("1. **Chip strategy** — re-derive each unused chip's gameweek from scratch.\n")
	b.WriteString("2. **Transfers and budget** — what is actually available.\n")
	b.WriteString("3. **Availability** — including team news the model cannot see. Search the web.\n")
	b.WriteString("4. **Research targets** — where the model is structurally blind. Search these " +
		"even though you were not considering them; that is the point.\n")
	b.WriteString("5. **Decide** — including deciding nothing, which is a first-class outcome.\n\n")

	if len(cfg.Criteria) > 0 {
		b.WriteString("### My criteria\n\nFollow these. Say so explicitly when the data argues against one.\n\n")
		for _, c := range cfg.Criteria {
			fmt.Fprintf(b, "- %s\n", c)
		}
		b.WriteString("\n")
	}

	p := cfg.Review
	b.WriteString("### Review policy\n\n")
	// The flat pair with the schedule stated beside it — the brief must not
	// tell a manager the settled bar while the tools apply the early floor
	// through GW8.
	fmt.Fprintf(b, "| Threshold | Value |\n|---|---|\n")
	fmt.Fprintf(b, "| Min modelled gain to spend a free transfer | %.2f pts/GW |\n", p.MinGainForTransfer)
	if p.EarlyFloor.UntilGameweek > 0 {
		fmt.Fprintf(b, "| Early floor (through GW%d) | %.1f pts charge / %.2f gain |\n",
			p.EarlyFloor.UntilGameweek, p.EarlyFloor.FreeTransferValue, p.EarlyFloor.MinGainForTransfer)
	}
	fmt.Fprintf(b, "| Min net gain across the horizon to justify a -4 | %.2f pts |\n", p.MinGainForHit)
	fmt.Fprintf(b, "| Bank transfers up to | %d |\n", p.BankUpTo)
	// The EFFECTIVE cap: MoveLimit clamps the allowance to the ceiling, so a
	// configured 2 under the shipped ceiling of 1 is 1 everywhere a solver runs.
	// Printing the raw setting told the reader a policy no code implements.
	fmt.Fprintf(b, "| Max hits per week | %d |\n",
		analysis.MoveLimit(0, p.MaxHitsPerWeek, 0, p.HitCeiling))
	fmt.Fprintf(b, "| Always act on a ruled-out starter | %v |\n", p.AlwaysActOnInjury)
	// Both settings are stated whether on or off, because the agent's job here is
	// to reason inside the standing policy and "the policy does not do this" is as
	// binding as "the policy does this". A row that appeared only when enabled
	// would leave the off state unwritten, and an unwritten constraint is one the
	// agent will talk past.
	fmt.Fprintf(b, "| Bank a transfer when next week buys more | %v |\n", p.BankTransfersLookahead)
	fmt.Fprintf(b, "| Prepare the squad for a planned chip | %v |\n\n", p.PrepareForChips)
	for _, r := range p.Rules {
		fmt.Fprintf(b, "- %s\n", r)
	}
	b.WriteString("\n")
}

func briefCompetitions(b *strings.Builder, e *analysis.Engine, now time.Time) {
	b.WriteString("---\n\n## Step 0 — Competition status\n\n")
	if days, ok := e.StatusAge(); ok {
		warn := ""
		if days > 7 {
			warn = " — **stale, re-verify against current results**"
		}
		fmt.Fprintf(b, "Last verified %d days ago%s.\n\n", days, warn)
	} else {
		b.WriteString("**Never verified.** Check this against actual results before trusting any score below.\n\n")
	}
	b.WriteString("Participation is not in the FPL API, so this is only as good as the last check. " +
		"A club knocked out should stop carrying a rotation penalty; one that wins a play-off should start.\n\n")

	// Asked over the scoring horizon, not over today.
	//
	// This table used to ask ActiveCampaigns what was live on the date the brief
	// ran, and the brief scores five gameweeks ahead. In mid-August that horizon
	// contains a Conference League play-off, the first Champions and Europa League
	// matchdays and a League Cup round for all twenty clubs — and the table called
	// every club "league only", directly under an instruction telling the agent that
	// competition status "changes every score below". The horizon is the window the
	// scores are computed over, so it is the window the question has to be asked in.
	next := e.Boot.NextEvent()
	if next == nil {
		// No upcoming gameweek, so there is no horizon and today is the only honest
		// question. Asking the horizon one anyway would resolve GW1-5's *past*
		// deadlines and report last August's competitions as current.
		b.WriteString("No upcoming gameweek, so there is no scoring horizon to ask about; " +
			"this is what the file says is live today.\n\n")
		competitionTable(b, e, "Live today", func(club string) []string {
			var out []string
			for _, w := range e.ActiveCampaigns(club, now) {
				s := w.Competition + " from " + w.Start
				if w.End != "" {
					s += " to " + w.End
				}
				out = append(out, s)
			}
			return out
		})
		return
	}

	from, to := next.ID, next.ID+e.Weights.Horizon-1
	// What the horizon changes and what it does not. Six of the eight congestion
	// penalties ship at 1.00 — all three European ones among them — so naming a
	// club's Champions League round is a statement about the *calendar*, not a claim
	// that the score moved. Only the domestic cup and the long-haul return price
	// anything today. An earlier draft of this line said a midweek round inside the
	// horizon "is already priced into every score below", which the shipped
	// configuration flatly denies.
	fmt.Fprintf(b, "Asked over the scoring horizon, GW%d-%d, rather than over today: "+
		"a midweek round a fortnight out is inside the window these scores are "+
		"computed over, and invisible to a question about this afternoon.\n\n", from, to)
	b.WriteString("Which of it reaches a score is a separate matter — the three European " +
		"penalties ship at 1.00, so those rows are calendar rather than arithmetic, and " +
		"only the domestic cup (0.97) and the long-haul return (0.86) move a number. " +
		"Read the gameweeks as what to check team news against.\n\n")
	competitionTable(b, e, "Competitions live in the horizon", func(club string) []string {
		var out []string
		for _, c := range e.CampaignsOverGameweeks(club, from, to) {
			// The start date is the auditable field — it is what a reviewer checks
			// against a result — so it is kept, and the gameweeks are added beside it.
			// Listing only the gameweeks would hide an eliminated club's window in every
			// week it has no round, which is precisely the staleness this section is for.
			s := c.Window.Competition + " from " + c.Window.Start
			if c.Window.End != "" {
				s += " to " + c.Window.End
			}
			if len(c.Gameweeks) > 0 {
				s += " — " + gameweekList(c.Gameweeks)
			} else {
				s += " — no round inside the horizon"
			}
			out = append(out, s)
		}
		return out
	})
}

// competitionTable renders one row per club from whatever the caller can say
// about it, so the horizon question and the date question cannot drift apart in
// formatting or in how they report a club with nothing to say.
func competitionTable(b *strings.Builder, e *analysis.Engine, heading string,
	forClub func(club string) []string) {
	fmt.Fprintf(b, "| Club | %s |\n|---|---|\n", heading)
	for i := range e.Boot.Teams {
		club := e.Boot.Teams[i].ShortName
		parts := forClub(club)
		if len(parts) == 0 {
			parts = []string{"*league only*"}
		}
		fmt.Fprintf(b, "| %s | %s |\n", club, strings.Join(parts, "; "))
	}
	b.WriteString("\n")
}

// gameweekList renders the gameweeks a competition touches as "GW4" or "GW4, GW7".
func gameweekList(gws []int) string {
	parts := make([]string, len(gws))
	for i, gw := range gws {
		parts[i] = fmt.Sprintf("GW%d", gw)
	}
	return strings.Join(parts, ", ")
}

func briefChips(b *strings.Builder, e *analysis.Engine, cfg config.Config) {
	b.WriteString("---\n\n## Step 1 — Chip strategy\n\n")
	b.WriteString("**The gameweeks below are a hypothesis, not a commitment.** Re-derive each " +
		"unused chip from what is known now and say whether that differs from the plan on file.\n\n")

	b.WriteString("| Chip | Window | Planned |\n|---|---|---|\n")
	for _, w := range e.ChipWindows() {
		// Asked per window rather than per chip name, so a second-set chip is
		// reported against the window that actually holds it.
		//
		// The fallback matters: asking only about this window reported a chip
		// planned OUTSIDE it as "unplanned", where the old label lookup showed
		// the gameweek. That loses the user's own plan from the one column whose
		// job is to show it, and it hides the very thing ValidateChipPlan is
		// about to complain about. Found by review.
		gw := "unplanned"
		if v := cfg.Chips.WeekIn(w.Name, w.Start, w.Stop); v > 0 {
			gw = fmt.Sprintf("GW%d", v)
		} else if v := cfg.Chips.Next(w.Name, 1); v > 0 {
			gw = fmt.Sprintf("GW%d (outside this window)", v)
		}
		fmt.Fprintf(b, "| %s | GW%d-%d | %s |\n", w.Label, w.Start, w.Stop, gw)
	}
	b.WriteString("\n")

	if problems := e.ValidateChipPlan(cfg.Chips); len(problems) > 0 {
		b.WriteString("**Issues with the current plan:**\n\n")
		for _, p := range problems {
			fmt.Fprintf(b, "- %s\n", p)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("Plan is legal: every chip has a distinct gameweek inside its window.\n\n")
	}

	if h, why := e.EffectiveHorizon(cfg.Chips); why != "" {
		fmt.Fprintf(b, "- %s Optimiser horizon reduced to %d gameweeks.\n", why, h)
	}
	if _, why := e.SuggestBenchWeight(cfg.Chips); why != "" {
		fmt.Fprintf(b, "- %s\n", why)
	}
	b.WriteString("\n")
}

func briefSquad(ctx context.Context, b *strings.Builder, client *fpl.Client, e *analysis.Engine,
	cfg config.Config) {

	b.WriteString("---\n\n## Step 2 — Squad, transfers and budget\n\n")
	if cfg.EntryID == 0 {
		b.WriteString("No `entry_id` configured, so there is no squad to read. Treat this as " +
			"building from scratch.\n\n")
		return
	}

	entry, err := client.Entry(ctx, cfg.EntryID)
	if err != nil {
		fmt.Fprintf(b, "Could not read entry %d: %v\n\n", cfg.EntryID, err)
		return
	}
	fmt.Fprintf(b, "**%s %s** — entry %d.\n\n", entry.PlayerFirstName, entry.PlayerLastName, cfg.EntryID)

	if h, err := client.History(ctx, cfg.EntryID); err == nil {
		if ft := fpl.FreeTransfers(h); ft == fpl.UnlimitedTransfers {
			b.WriteString("- **Free transfers: unlimited.** The first deadline has not passed, " +
				"so the squad can still be rebuilt freely. Transfer economy does not apply yet — " +
				"build the best squad, do not conserve moves.\n")
		} else {
			fmt.Fprintf(b, "- **Free transfers: %d** (reconstructed from history — FPL does not "+
				"publish this, so verify on the site if a decision turns on it).\n", ft)
		}
		if len(h.Chips) > 0 {
			var used []string
			for _, c := range h.Chips {
				used = append(used, fmt.Sprintf("%s (GW%d)", c.Name, c.Event))
			}
			fmt.Fprintf(b, "- Chips already played: %s\n", strings.Join(used, ", "))
		}
	}

	if entry.CurrentEvent == nil {
		b.WriteString("- No squad is visible yet: FPL only exposes picks after a deadline has " +
			"passed. This is expected before GW1, not an error.\n\n")
		return
	}

	picks, err := client.Picks(ctx, cfg.EntryID, *entry.CurrentEvent)
	if err != nil {
		fmt.Fprintf(b, "- Could not read picks: %v\n\n", err)
		return
	}
	fmt.Fprintf(b, "- Bank £%.1fm, squad value £%.1fm\n\n",
		float64(picks.EntryHistory.Bank)/10, float64(picks.EntryHistory.Value)/10)

	b.WriteString("### Current squad\n\n")
	briefPlayerTable(b, func(yield func(analysis.PlayerMetrics)) {
		for _, p := range picks.Picks {
			if el := e.Boot.ElementByID(p.Element); el != nil {
				yield(e.Metrics(el))
			}
		}
	})
}

// briefConcerns lists everything the model already knows is wrong: unavailable
// players, rotation risk, role uncertainty and post-tournament load.
func briefConcerns(b *strings.Builder, e *analysis.Engine) {
	b.WriteString("---\n\n## Step 3 — What the model already flags\n\n")
	b.WriteString("These are FPL's own status flags plus the model's risk terms. **They are not " +
		"team news.** Search the web for anyone you intend to rely on: FPL's `news` field is " +
		"terse and lags press conferences by days.\n\n")

	all := e.AllMetrics()
	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })

	var doubts, rested []analysis.PlayerMetrics
	for _, m := range all {
		switch {
		case isUnavailable(m):
			doubts = append(doubts, m)
		case m.RestRisk != "":
			rested = append(rested, m)
		}
	}

	// Rank the flagged table by price, not score, and rank before truncating.
	// An injured player scores ~0, so score-ordering buries exactly the expensive
	// names that matter — capping first dropped Saliba from the list entirely.
	sort.Slice(doubts, func(i, j int) bool { return doubts[i].Price > doubts[j].Price })
	const maxFlagged = 30
	truncated := 0
	if len(doubts) > maxFlagged {
		truncated = len(doubts) - maxFlagged
		doubts = doubts[:maxFlagged]
	}

	b.WriteString("### Flagged unavailable or doubtful\n\n")
	if len(doubts) == 0 {
		b.WriteString("*None.*\n\n")
	} else {
		b.WriteString("| Player | Club | £ | Status | News |\n|---|---|---|---|---|\n")
		for _, m := range doubts {
			news := m.News
			if news == "" {
				news = "—"
			}
			chance := m.Status
			if m.ChancePlay != nil {
				chance = fmt.Sprintf("%s (%d%%)", m.Status, *m.ChancePlay)
			}
			fmt.Fprintf(b, "| %s | %s | %.1f | %s | %s |\n", m.Name, m.Team, m.Price, chance, news)
		}
		if truncated > 0 {
			fmt.Fprintf(b, "\n*%d further flagged players below £%.1fm omitted.*\n",
				truncated, doubts[len(doubts)-1].Price)
		}
		b.WriteString("\n")
	}

	if len(rested) > 0 {
		fmt.Fprintf(b, "### Carrying a post-tournament rest discount (%d)\n\n", len(rested))
		var names []string
		for _, m := range rested {
			names = append(names, fmt.Sprintf("%s (%s)", m.Name, m.Team))
		}
		fmt.Fprintf(b, "%s\n\n", strings.Join(names, ", "))
	}
}

func briefOptimal(b *strings.Builder, cfg config.Config, e *analysis.Engine) ([]analysis.PlayerMetrics, error) {
	budget, source, err := e.AssemblyBudget()
	if err != nil {
		return nil, err
	}

	b.WriteString("---\n\n## Model-optimal squad\n\n")
	fmt.Fprintf(b, "A multi-dimensional knapsack under FPL's rules, not a recommendation. "+
		"Critique it: it cannot see team news, and it optimises a number. "+
		"Built with £%.1fm (%s). Once the season is running that is the wildcard "+
		"budget, so this is the fifteen a chip could buy rather than anything "+
		"reachable by transfer.\n\n", float64(budget)/10, source)

	// BenchWeight is deliberately left at zero. Zero is not "the bench is
	// worthless" — Optimize reads it as "use the configured weight", which is
	// config.json's bench_weight backfilled to analysis.DefaultBenchWeight. Naming
	// a weight here instead would make this command disagree with `armband squad`
	// and would make the config key inert. See TestSquadBuildersDoNotNameABenchWeight.
	req := analysis.OptimizeRequest{Budget: budget, MinMinutes: 600,
		MinExpectedMinutes: 55}
	// The overrides listed at the top of this document must bind the squad
	// printed in it, or the brief contradicts itself.
	applyRoster(cfg, e, &req)
	for _, note := range e.ApplyChipPlan(&req) {
		fmt.Fprintf(b, "- Chip plan: %s\n", note)
	}
	sq, err := e.Optimize(req)
	if err != nil {
		fmt.Fprintf(b, "\nOptimiser failed: %v\n\n", err)
		return nil, nil
	}
	fmt.Fprintf(b, "\n**Formation %s — £%.1fm spent, £%.1fm left. Projected XI %.1f pts/GW "+
		"(%.1f with the captain doubled).**\n\n",
		sq.Formation, sq.TotalCost, sq.Remaining, sq.XIScore, sq.ExpectedPoints)
	fmt.Fprintf(b, "Captain %s, vice %s.\n\n", sq.Captain.Name, sq.ViceCaptain.Name)

	briefSquadTable(b, sq)

	b.WriteString("### Best available by position\n\n")
	all := e.AllMetrics()
	sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
	for _, pos := range []string{"GKP", "DEF", "MID", "FWD"} {
		fmt.Fprintf(b, "**%s**\n\n", pos)
		briefPlayerTable(b, func(yield func(analysis.PlayerMetrics)) {
			n := 0
			for _, m := range all {
				if m.Position == pos && n < 12 {
					yield(m)
					n++
				}
			}
		})
	}

	return sq.Players, nil
}

// briefSquadTable prints the fifteen as ONE table, in position order throughout,
// with the bench under an in-table header rather than in a table of its own.
//
// # Why one table
//
// Two tables invited reading the bench as a separate thing, and a reader comparing a
// starter with his own backup had to look in two places and re-sort both. Position
// order is the order a team sheet is read in.
//
// # Why the bench keeps a substitution number
//
// **Bench order is not presentation, it is the autosub priority**, and sorting the
// bench by position silently destroys it. FPL uses a bench player when a starter
// records no minutes, *in bench order*, which is why this model's derived slot weights
// are P(one starter blanks), P(two) and P(three) rather than a flat weight — the first
// outfield slot is worth several times the third. Re-ordering the rows without saying
// so would present a substitution queue as a position grouping.
//
// So the rows sort by position like everything else, and each bench row carries its
// real position in the queue. The reserve keeper is marked separately because he is not
// in the outfield queue at all: he covers exactly one player, the starting keeper.
func briefSquadTable(b *strings.Builder, sq *analysis.Squad) {
	// The queue is read from the ORIGINAL slice, before any sorting, because that
	// order is the datum. Outfielders are numbered; the reserve keeper is not part of
	// the same queue and is labelled rather than numbered.
	queue := map[int]string{}
	n := 0
	for _, p := range sq.Bench {
		if p.Position == "GKP" {
			queue[p.ID] = "reserve GK"
			continue
		}
		n++
		queue[p.ID] = fmt.Sprintf("sub %d", n)
	}

	b.WriteString("### Squad\n\n")
	b.WriteString(briefTableHeader)
	for _, p := range byPosition(sq.StartingXI) {
		briefPlayerRow(b, p, "")
	}
	b.WriteString("| **— bench —** | | | | | | | |\n")
	for _, p := range byPosition(sq.Bench) {
		briefPlayerRow(b, p, queue[p.ID])
	}
	b.WriteString("\n")
}

const briefTableHeader = "| Pos | Player | Club | £ | Score | Exp mins | FDR | Flags |\n" +
	"|---|---|---|---|---|---|---|---|\n"

func briefPlayerTable(b *strings.Builder, each func(func(analysis.PlayerMetrics))) {
	b.WriteString(briefTableHeader)
	each(func(p analysis.PlayerMetrics) { briefPlayerRow(b, p, "") })
	b.WriteString("\n")
}

// briefPlayerRow is the single implementation of a player row. `extra` is an
// additional leading flag — the bench's substitution order uses it — and is empty
// everywhere else. Kept as one function because two row builders drifting apart is how
// a table starts quietly disagreeing with the one above it.
func briefPlayerRow(b *strings.Builder, p analysis.PlayerMetrics, extra string) {
	var flags []string
	if extra != "" {
		flags = append(flags, extra)
	}
	if p.RotationRisk != "" {
		flags = append(flags, p.RotationRisk)
	}
	if p.NewSigning {
		flags = append(flags, "new signing")
	}
	if p.RoleFactor < 1 {
		flags = append(flags, fmt.Sprintf("role x%.2f", p.RoleFactor))
	}
	if p.RestRisk != "" {
		flags = append(flags, "rest discount")
	}
	if p.MatchesAvailable < analysis.GameweeksPerSeason {
		flags = append(flags, fmt.Sprintf("mins /%d", p.MatchesAvailable))
	}
	if isUnavailable(p) {
		flags = append(flags, strings.ToUpper(p.Status))
	}
	if p.SetPieceNote != "" {
		flags = append(flags, p.SetPieceNote)
	}
	f := strings.Join(flags, ", ")
	if f == "" {
		f = "—"
	}
	fmt.Fprintf(b, "| %s | %s | %s | %.1f | %.2f | %.0f | %.1f | %s |\n",
		p.Position, p.Name, p.Team, p.Price, p.Score, p.ExpectedMinutes, p.AvgDifficulty, f)
}

func briefFixtures(b *strings.Builder, e *analysis.Engine) {
	h := e.Weights.Horizon
	fmt.Fprintf(b, "---\n\n## Fixtures over the next %d gameweeks\n\n", h)
	b.WriteString("Lower average difficulty is better. Congestion is the European, cup, " +
		"turnaround and international-travel multiplier on expected minutes.\n\n")
	b.WriteString("| Club | Avg FDR | Opponents |\n|---|---|---|\n")

	type row struct {
		short string
		avg   float64
		opps  string
	}
	var rows []row
	for i := range e.Boot.Teams {
		t := &e.Boot.Teams[i]
		fx := e.TeamFixtures(t.ID, h)
		if len(fx) == 0 {
			continue
		}
		var total float64
		var opps []string
		for _, f := range fx {
			total += float64(f.Difficulty)
			ha := "H"
			if !f.Home {
				ha = "A"
			}
			opps = append(opps, fmt.Sprintf("%s(%s,%d)", f.Opponent, ha, f.Difficulty))
		}
		rows = append(rows, row{t.ShortName, total / float64(len(fx)), strings.Join(opps, " ")})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].avg < rows[j].avg })
	for _, r := range rows {
		fmt.Fprintf(b, "| %s | %.2f | %s |\n", r.short, r.avg, r.opps)
	}
	b.WriteString("\n")
}

func briefBlindSpots(b *strings.Builder, e *analysis.Engine) {
	b.WriteString("---\n\n## What this model cannot see\n\n")
	b.WriteString("- Transfer news, press conferences, tactical changes, a player who has just lost his place.\n")
	b.WriteString("- New signings from abroad, who carry no Premier League data at all.\n")
	b.WriteString("- The difference between \"injured for three months\" and \"not picked\". " +
		"Both show as low expected minutes — check minutes-per-start to tell them apart.\n")
	b.WriteString("- Whether a club is still in a competition. That is Step 0, and it is hand-maintained.\n\n")

	breaks := e.PostBreakGameweeks()
	if len(breaks) > 0 {
		var gws []int
		for gw := range breaks {
			gws = append(gws, gw)
		}
		sort.Ints(gws)
		var parts []string
		for _, gw := range gws {
			parts = append(parts, fmt.Sprintf("GW%d (%.0f-day break)", gw, breaks[gw]))
		}
		fmt.Fprintf(b, "Gameweeks following an international break: %s\n\n", strings.Join(parts, ", "))
	}
	b.WriteString("A model number is a prior. Your job is to update it.\n")
}

// isUnavailable reports a player FPL has flagged. PlayerMetrics.Status carries
// the human-readable label ("available", "doubtful", …) rather than FPL's raw
// single-letter code, so comparing against "a" silently marks the entire pool
// unavailable.
func isUnavailable(p analysis.PlayerMetrics) bool {
	return p.Status != "" && p.Status != "available"
}

func humanDuration(d time.Duration) string {
	switch {
	case d > 48*time.Hour:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d > 2*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
}

// briefResearch lists where the model is most likely to be wrong, so the agent
// spends its searches on blind spots rather than on players it already rates.
//
// The existing team-news step is reactive — it verifies players already under
// consideration. That cannot catch a player scored so low he was never
// considered, which is exactly how a promoted club's nailed starter gets missed.
func briefResearch(b *strings.Builder, e *analysis.Engine, squad []analysis.PlayerMetrics) {
	cats := e.ResearchTargets(squad)
	b.WriteString("---\n\n## Step 4 — Research targets\n\n")
	if len(cats) == 0 {
		b.WriteString("Nothing flagged. That is unusual — check the thresholds rather than " +
			"assuming the model has nothing to learn.\n\n")
		return
	}
	b.WriteString("**The model cannot see roles.** It infers minutes from last season's totals, " +
		"which say nothing about a manager's plan for this one. Step 3 verifies players you are " +
		"already considering; this step goes the other way, and names where the model is " +
		"structurally blind so you look at players you would never have shortlisted.\n\n")
	b.WriteString("Search each one. A target that turns out to be a non-story costs one search; " +
		"a miss costs a season. Report what you found, including the blanks.\n\n")

	for _, c := range cats {
		fmt.Fprintf(b, "### %s\n\n%s\n\n**Ask:** %s\n\n", c.Name, c.Why, c.Ask)
		b.WriteString("| Player | Club | Pos | £ | Owned | Score | Mins |\n|---|---|---|---|---|---|---|\n")
		for _, p := range c.Targets {
			fmt.Fprintf(b, "| %s | %s | %s | %.1f | %.1f%% | %.2f | %d |\n",
				p.Name, p.Team, p.Position, p.Price, p.Ownership, p.Score, p.Minutes)
		}
		b.WriteString("\n")
	}
}
