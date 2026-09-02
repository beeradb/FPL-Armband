package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"armband/internal/analysis"
	"armband/internal/backtest"
	"armband/internal/config"
	"armband/internal/present"
	"armband/internal/stats"
)

// cmdBacktest replays a past season's GW1 squad selection.
//
// It answers one question: given only what was knowable before a ball was
// kicked, does the optimiser pick players who go on to return? Everything else
// the system does — team news, predicted line-ups, role changes — is
// unreproducible in hindsight, so this measures the floor the model provides
// rather than the output of a real review.
func cmdBacktest(ctx context.Context, cfg config.Config, season string, payoffGWs int) error {
	prior, err := backtest.PriorSeasonName(season)
	if err != nil {
		return err
	}

	fmt.Printf("\nReplaying %s, using %s as the pre-season evidence.\n", season, prior)
	cur, err := backtest.Load(ctx, cfg.CacheDir, season)
	if err != nil {
		return fmt.Errorf("loading %s: %w", season, err)
	}
	// Checked before anything is built, so the oldest playable season fails with a
	// sentence rather than with the panic PreSeason would raise. The prior is
	// deliberately NOT checked: a prior-only season is a legitimate prior, which is
	// the whole point of the distinction.
	if err := cur.PlayableAsCurrent(); err != nil {
		return err
	}
	pri, err := backtest.Load(ctx, cfg.CacheDir, prior)
	if err != nil {
		return fmt.Errorf("loading %s: %w", prior, err)
	}
	fmt.Printf("%s\n\n", dim(fmt.Sprintf("%d players, %d fixtures; prior season %d players",
		len(cur.Players), len(cur.Fixtures), len(pri.Players))))

	// A past season has no congestion or role-risk configuration, and no chip
	// plan. Leaving them empty keeps the replay to what it is testing.
	boot, fx := backtest.PreSeasonWith(cur, pri, backtest.OraclesFromEnv())
	e := analysis.NewEngineFull(boot, fx, cfg.Weights, analysis.Congestion{}, analysis.RoleRisk{})

	sq, err := e.Optimize(analysis.OptimizeRequest{
		MinMinutes: analysis.PoolMinMinutes, MinExpectedMinutes: analysis.PoolMinExpectedMinutes, BenchWeight: analysis.DefaultBenchWeight,
	})
	if err != nil {
		return fmt.Errorf("optimising: %w", err)
	}

	fmt.Printf("Model's GW1 squad — %s, £%.1fm, captain %s\n\n", sq.Formation, sq.TotalCost, sq.Captain.Name)
	show := func(tag string, ps []analysis.PlayerMetrics) {
		for _, p := range ps {
			actual := 0
			if q := cur.Players[p.ID]; q != nil {
				actual = q.TotalPoints
			}
			fmt.Printf("  %s %-4s %-17s £%5.1fm   modelled %5.2f   actual season %4d\n",
				tag, p.Position, p.Name, p.Price, p.Score, actual)
		}
	}
	show("XI ", byPosition(sq.StartingXI))
	show("bch", byPosition(sq.Bench))

	res := backtest.Score("model", cur, sq.Players, sq.StartingXI, sq.Captain.ID, sq.ViceCaptain.ID)
	dist := backtest.RandomSquads(cur, analysis.DefaultBudget, 2000, 42)
	ceiling := backtest.Ceiling(cur, analysis.DefaultBudget)

	fmt.Printf("\n%-30s%12s\n", "", "squad points")
	fmt.Printf("%-30s%12d\n", "model", res.SquadPoints)
	if len(dist) > 0 {
		// One decimal, because the median of an even number of squads is a
		// half-integer as often as not and this column used to truncate it away.
		fmt.Printf("%-30s%12.1f\n", "random legal squad, median", stats.Median(dist))
		fmt.Printf("%-30s%12d\n", "random legal squad, 90th pct", dist[len(dist)*9/10])
		fmt.Printf("%-30s%11.1f%%\n", "→ model percentile", backtest.Percentile(dist, res.SquadPoints))
	}
	fmt.Printf("%-30s%12d %s\n", "perfect hindsight", ceiling, dim("(unreachable — knows every final total)"))

	fmt.Printf("\n%-30s%12d %s\n", "chosen XI + captain, autosubs", res.XIPoints,
		dim("(XI held all season, so a floor)"))
	fmt.Printf("%-30s%+11.1fm %s\n", "squad value change", float64(res.ValueChange)/10,
		dim("(the model has no concept of price)"))

	// Stage 2: play the season out week by week under several transfer policies.
	// One policy proves nothing on one season — the spread between them is the
	// only way to tell a real effect from noise.
	//
	// A second hit in one week is no longer offered — moveLimit caps at every
	// banked transfer plus one. It needs a specific reason, usually an injury,
	// which is the judgement layer's job rather than something a scoring model
	// should go hunting for. The row is gone rather than duplicated: with the
	// cap in place it produced results identical to the one-hit policy.
	policies := []struct {
		name     string
		maxMoves int
		maxHits  int
	}{
		{"one transfer a week", 1, 0},
		{"multi-transfer, no hits", 0, 0},
		{"multi-transfer, 1 hit/wk", 0, 1},
	}
	// Counted from the slice rather than written out. It said "four" for as long
	// as the fourth policy had been gone, which is the two-implementations-of-one-
	// quantity failure in its cheapest form: nothing can disagree with a len().
	fmt.Printf("\n%s\n", dim(fmt.Sprintf("simulating 38 gameweeks under %d transfer policies...",
		len(policies))))
	base := backtest.SimConfig{
		Weights: cfg.Weights, MinGain: cfg.Review.MinGainForTransfer,
		MinGainHit: cfg.Review.MinGainForHit,
		// The banking rule changed mid-window, so the replay uses the one that
		// was actually in force rather than today's.
		BankUpTo: backtest.BankLimitFor(season),
		FreeCost: cfg.Review.FreeTransferValue,
		Budget:   analysis.DefaultBudget,
		// Pick the eleven on the imminent gameweek, which is what makes the
		// fixture-load term reachable: a club playing twice this week is worth
		// roughly twice as much, and a horizon average dilutes that to nothing.
		// Worth +33 points a season at t = +5.74 with squad selection
		// unchanged — see analysis.fixtureLoadFor.
		WeeklyXI: true,
		// The two transfer-policy switches a user can actually set, carried onto
		// the replay so `armband backtest` scores the policy the config describes.
		//
		// ⚠️ Without these the command replays the SHIPPED policy under the user's
		// other settings: a manager turns banking on, runs a backtest, and gets a
		// byte-identical null from a setting that never arrived — which is exactly
		// the reading the standing rule says to check the mediator for, produced by
		// the very feature that added the mediator. One config field, one
		// consumer, named here so the link is greppable in both directions.
		BankLookahead:        cfg.Review.BankTransfersLookahead,
		PrepareBenchBoost:    cfg.Review.PrepareForChips,
		PrepareTripleCaptain: cfg.Review.PrepareForChips,
		// The scheduled early floor, carried onto the replay for the same
		// reason as the banking switch above: `armband backtest` scores the
		// policy the config describes, and a user who edits `early_floor` must
		// not get a byte-identical null. (The measurement sweepConfig
		// deliberately does NOT map it — the sweep baseline stays the flat
		// machine every banked cell was measured on.)
		EarlyFloor: cfg.Review.EarlyFloor,
		// The hit ceiling, for the identical reason. `max_hits_per_week` is
		// carried onto the replay below via `pol.maxHits`, and without this the
		// ceiling that decides what that number can MEAN would stay at the
		// shipped 1 — so a user raising both would get the same silent null the
		// comment above is about, from the knob added to abolish it.
		HitCeiling: cfg.Review.HitCeiling,
		// One config field drives both: AnticipateGate has no effect unless
		// AnticipateChips is also set (enforced again in decide), and a
		// mismatched pair — scoring a move on a shortened horizon while still
		// charging it over the full one — over-credits near-term fixture
		// spikes by construction. Exposing a second, independently-settable
		// field here would let a user reintroduce exactly that bias by
		// omission; see ReviewPolicy.AnticipateChips.
		AnticipateChips: cfg.Review.AnticipateChips,
		AnticipateGate:  cfg.Review.AnticipateChips,
		// Diagnostic override for sweeping what the opening fifteen credits a
		// bench player at. See SimConfig.openingBenchWeight — the shipped 0.02
		// is what broke when the optimiser was fixed.
		BenchWeight: envFloat("FPL_BENCH_WEIGHT"),
		// Enter the season at a later deadline. Averaging a sweep over several
		// start points averages over transfer *paths*, which is where nearly all
		// of the replay's parameter sensitivity lives — see SimConfig.StartGW.
		StartGW: int(envFloat("FPL_START_GW")),
		// Hindsight, off unless FPL_ORACLE_AVAILABILITY or FPL_ORACLE_PRICES is
		// set. Both used to be read deep inside the replay — one per call in
		// statusAt, one at package init — so this command got them for free
		// without ever naming them. Now that they ride on the config, the seed
		// has to be explicit here or the documented switches would go silently
		// inert on this path while still appearing in docs/replay.md.
		Oracles: backtest.OraclesFromEnv(),
	}
	// And the four option-value levers, on the same rule and for the same reason
	// as the two switches above: a config field with no consumer returns the
	// shipped behaviour under a setting the user believes arrived.
	applyOptionValue(&base, cfg.OptionValue)
	// Diagnostic override for the 3/14/3 attack/defence bands. Zero is off.
	if bs := envFloat("FPL_BAND_STRENGTH"); bs > 0 {
		base.Weights.BandStrength = bs
	}
	// Chips, off unless asked for. Reported when set: a season played with a
	// wildcard is not comparable with one played without, and nothing else in
	// this command's output would say which you are looking at.
	// `anchored` derives the plan from the season's own calendar: BOTH sets,
	// all four chips, fallbacks where a set's window has no qualifying week.
	// That is the plan a real manager plays — every chip the season grants is
	// used, in the set that grants it. See backtest.FullAnchoredPlan. Checked
	// BEFORE the spec parser, which would refuse the word as a plan.
	var chips, chips2 analysis.ChipPlan
	if strings.TrimSpace(os.Getenv("FPL_CHIP_PLAN")) == "anchored" {
		sch := backtest.FullAnchoredPlan(cur, base.StartGW)
		chips, chips2 = sch.First, sch.Second
		// A late entry's window may be too small for every chip the season
		// grants. Refuse rather than print a plan the replay can never play —
		// the silent loss is the shape this whole family of guards exists for.
		if backtest.PlacedChips(chips) < 4 {
			return fmt.Errorf("anchored plan: entry GW%d leaves too few playable "+
				"weeks to place every first-set chip; pass an explicit plan "+
				"(FPL_CHIP_PLAN=<spec>) instead", base.StartGW)
		}
		if backtest.ChipSetsFor(season) >= 2 && backtest.PlacedChips(chips2) < 4 {
			return fmt.Errorf("anchored plan: entry GW%d leaves too few playable "+
				"weeks to place every second-set chip; pass an explicit plan "+
				"(FPL_CHIP_PLAN=<spec>) instead", base.StartGW)
		}
	} else {
		chips, chips2, err = chipPlanFromEnv(cfg, season)
	}
	if err != nil {
		return err
	}
	// Validated against the season, not merely parsed. A second-set chip on a
	// season that granted one set would otherwise just play, and the output has no
	// column that would say it had.
	if err := backtest.ValidateChipSets(season, chips, chips2); err != nil {
		return fmt.Errorf("FPL_CHIP_PLAN: %w", err)
	}
	base.Chips, base.Chips2 = chips, chips2
	if chips != (analysis.ChipPlan{}) || chips2 != (analysis.ChipPlan{}) {
		fmt.Printf("%s\n", dim(fmt.Sprintf("chip plan: %s", describeChipSets(chips, chips2))))
	}
	// Advisory only, and deliberately not a hard error the way ValidateChipSets
	// above is: a live run's plan may legitimately spend fewer than every chip
	// a season granted — a manager choosing to hold one back is not a bug —
	// and this command is not a sweep whose whole row the guard must refuse.
	// It exists so a plan that concentrates entirely in one half, exactly the
	// way `anchoredPlan`'s first version once did, is at least printed rather
	// than silently played. See backtest.ValidateChipSpend.
	if err := backtest.ValidateChipSpend(season, base.StartGW, chips, chips2); err != nil {
		fmt.Printf("%s\n", dim(fmt.Sprintf("note: %v", err)))
	}
	fmt.Printf("\n%-30s%12s%10s%8s%10s\n", "playing the season out", "points", "transfers", "hits", "value")
	var sim *backtest.SimResult
	for _, pol := range policies {
		c := base
		c.MaxMoves, c.MaxHits = pol.maxMoves, pol.maxHits
		r, err := backtest.Simulate(cur, pri, c)
		if err != nil {
			return fmt.Errorf("simulating %s: %w", pol.name, err)
		}
		if pol.maxHits == cfg.Review.MaxHitsPerWeek && pol.maxMoves == 0 {
			sim = r // the configured policy, used for the detail below
		}
		fmt.Printf("%-30s%12d%10d%8d%+9.1fm\n", pol.name, r.Points, r.Transfers, r.Hits,
			float64(r.EndValue-r.StartValue)/10)
	}
	if sim == nil {
		return fmt.Errorf("configured policy produced no result")
	}
	// The fair baseline keeps the same fifteen but still re-picks the eleven and
	// captain weekly. Comparing against a frozen eleven would credit transfers
	// with the value of simply playing your best players, which is free.
	// `base`, not a fresh config: the baseline has to be the same model under the
	// same settings, differing only in that it never transfers. Building one from
	// Weights alone silently dropped everything else — most visibly StartGW, which
	// left a mid-season entry compared against a baseline scored over the whole
	// season, but PriorHalfLife and OlderPriors would go the same way.
	hold := backtest.Hold(cur, pri, base, sim.OpeningSquad)
	fmt.Printf("%-30s%12d %s\n", "hold the opening fifteen", hold,
		dim("(no transfers, but XI re-picked weekly)"))
	fmt.Printf("%-30s%+12d %s\n", "→ transfers were worth", sim.Points-hold,
		dim(fmt.Sprintf("after %d points of hits", sim.HitCost)))
	reportWeeks(sim, backtest.HoldWeekly(cur, pri, base, sim.OpeningSquad))

	// A page you can click through, when one is asked for. The terminal report
	// above is a season in one scroll, which is the wrong shape for the question
	// this page answers: what changed, and in which week. Thirty-eight tabs and a
	// transfer list do that, and a scrollback cannot.
	if path := os.Getenv("FPL_REPLAY_HTML"); path != "" {
		n := 10
		if v := envFloat("FPL_REPLAY_GWS"); v > 0 {
			n = int(v)
		}
		if err := writeReplayHTML(path, season, cur, sim, n); err != nil {
			return err
		}
		fmt.Printf("\n%s\n", dim("Replay written to "+path))
	}

	// One machine-readable line for sweeps, so a harness does not have to grep
	// three differently-formatted rows and get the column wrong. Which metric to
	// read is the whole question: `hold` moves a quarter as much as `policy` for
	// the same parameter nudge, because it carries one squad decision instead of
	// a season of compounding transfer decisions. Use it for scoring constants
	// and `policy` only for constants that are *about* transfers.
	if os.Getenv("FPL_SWEEP") != "" {
		// The effective start, not the raw field: unset means 1, and a sweep
		// that logs 0 invites someone to conclude the flag did nothing.
		start := base.StartGW
		if start < 1 {
			start = 1
		}
		fmt.Printf("SWEEP season=%s start=%d weeks=%d hold=%d policy=%d transfers=%d hits=%d\n",
			season, start, len(sim.Weeks), hold, sim.Points, sim.Transfers, sim.Hits)
		// The squad the *simulation* opened with, which is not the one printed
		// above: that one is built separately with DefaultBenchWeight hardcoded,
		// so it does not move when the replay's bench weight does. Checking a
		// bench-weight result against it would compare three identical squads.
		for _, id := range sim.OpeningSquad {
			if p := cur.Players[id]; p != nil {
				fmt.Printf("SQUAD %s %s £%.1fm mins=%d pts=%d\n",
					[]string{"", "GKP", "DEF", "MID", "FWD"}[p.Type],
					p.WebName, float64(p.NowCost)/10, p.Minutes, p.TotalPoints)
			}
		}
	}
	reportMoves(cur, sim, payoffGWs, cfg.Weights.Horizon)

	duds, wasted := backtest.Duds(cur, sq.Players, 5.0, 60)

	if len(duds) == 0 {
		fmt.Println("\nNo significant duds.")
		return nil
	}
	var never int
	for _, d := range duds {
		if d.NeverPlayed {
			never++
		}
	}
	fmt.Printf("\nPicks at £5.0m+ returning under 60 points — £%.1fm of £%.1fm (%.0f%% of budget):\n",
		wasted, sq.TotalCost, wasted/sq.TotalCost*100)
	for _, d := range duds {
		fmt.Printf("  %-17s £%5.1fm  %4d pts   %s\n", d.Name, d.Price, d.Points, d.Reason)
	}
	if never > 0 {
		fmt.Printf("\n%s\n", strings.TrimSpace(fmt.Sprintf(
			"%d of these never played a minute. That is not a modelling error — the model has no\n"+
				"way to know a player has left the league, and one web search would have caught it.\n"+
				"This is the gap the judgement layer exists to fill.", never)))
	}
	return nil
}

// byPosition orders a squad GK, DEF, MID, FWD and by score within each, which is
// how a team sheet reads. The optimiser returns them by score alone, so the
// eleven arrives interleaved and is hard to check against a real squad.
func byPosition(ps []analysis.PlayerMetrics) []analysis.PlayerMetrics {
	order := map[string]int{"GKP": 0, "DEF": 1, "MID": 2, "FWD": 3}
	out := append([]analysis.PlayerMetrics(nil), ps...)
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].Position] != order[out[j].Position] {
			return order[out[i].Position] < order[out[j].Position]
		}
		return out[i].Score > out[j].Score
	})
	return out
}

// reportWeeks prints the season gameweek by gameweek, against the baseline of
// holding the opening fifteen.
//
// The running gap is the number to read. A season total says transfers were
// worth some amount; this says when — whether the gain accumulated steadily or
// came from three weeks in September, which is the difference between a policy
// that works and one that got lucky early.
func reportWeeks(sim *backtest.SimResult, hold []int) {
	fmt.Printf("\nWeek by week, against holding the opening fifteen:\n\n")
	fmt.Printf("  %-5s %6s %5s %5s %-16s %6s %5s %7s %8s\n",
		"gw", "points", "hits", "tr", "captain", "capt", "hold", "vs hold", "value")

	var total, holdTotal int
	for i, w := range sim.Weeks {
		h := 0
		if i < len(hold) {
			h = hold[i]
		}
		total += w.Net
		holdTotal += h

		hits, tr := "", ""
		if w.HitCost > 0 {
			hits = fmt.Sprintf("-%d", w.HitCost)
		}
		if w.Transfers > 0 {
			// The count alone cannot tell a two-transfer week that paid a hit
			// from one that spent two banked frees, and the difference is the
			// whole cost of the week. "1+1h" names it.
			if w.HitCost > 0 {
				tr = fmt.Sprintf("%d+%dh", w.Transfers-(w.HitCost/4), w.HitCost/4)
			} else {
				tr = fmt.Sprintf("%d", w.Transfers)
			}
		}
		line := fmt.Sprintf("  GW%-3d %6d %5s %5s %-16s %6d %5d %+7d %7.1fm",
			w.GW, w.Net, hits, tr, w.Captain, w.CaptainPts, h, total-holdTotal,
			float64(w.Value)/10)
		// Dim the quiet weeks so the ones that moved the season stand out.
		if w.Transfers == 0 && abs(w.Net-h) < 5 {
			fmt.Printf("%s\n", dim(line))
		} else {
			fmt.Printf("%s\n", line)
		}
	}
	fmt.Printf("\n  %-5s %6d %25s %13d %+7d\n", "total", total, "", holdTotal, total-holdTotal)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// reportMoves prints every transfer the configured policy made, scored against
// what the two players actually went on to do.
//
// The modelled gain is what the policy believed. Only the outcome column says
// whether it was right, and the two disagreeing is the interesting case — a run
// of confident, wrong moves is a calibration problem, while a wide spread around
// a positive mean is just variance and should be left alone.
func reportMoves(cur *backtest.Season, sim *backtest.SimResult, payoff, decided int) {
	if len(sim.Moves) == 0 {
		fmt.Println("\nNo transfers made.")
		return
	}
	verdicts := backtest.Judge(cur, sim.Moves, payoff)

	if payoff == decided {
		fmt.Printf("\nEvery transfer, scored over the horizon it was justified on:\n\n")
	} else {
		// Judging over a longer window than the policy decided on is a
		// deliberate question, not a mismatch: with one transfer a week you are
		// buying someone to start for months, so a five-week verdict on a
		// season-long commitment is the wrong test.
		fmt.Printf("\nEvery transfer, scored over %d gameweeks %s:\n\n", payoff,
			dim(fmt.Sprintf("(the policy decided on %d — a longer window asks whether the "+
				"move held up, not whether it started well)", decided)))
	}
	fmt.Printf("  %-5s %-17s %-17s %9s %8s %8s %8s  %s\n",
		"gw", "out", "in", "modelled", "in pts", "out pts", "net", "cost")

	var net, good, hits, ghosts, ghostNet int
	var modelled float64
	for _, v := range verdicts {
		tag := ""
		cost := dim("free")
		if v.Hit {
			tag = " -4"
			cost = red("HIT -4")
			hits++
		}
		if !v.OutPlayed {
			ghosts++
			ghostNet += v.Net()
			tag += " *"
		}
		line := fmt.Sprintf("  GW%-3d %-17s %-17s %+8.2f%-3s %8d %8d %+8d  %s",
			v.GW, v.Out, v.In, v.Gain, tag, v.InPoints, v.OutPoints, v.Net(), cost)
		if v.Net() < 0 {
			fmt.Printf("%s\n", dim(line))
		} else {
			fmt.Printf("%s\n", line)
			good++
		}
		net += v.Net()
		modelled += v.Gain * float64(v.Weeks)
	}

	fmt.Printf("\n  %d transfers, %d hits. %d gained points, %d lost them.\n",
		len(verdicts), hits, good, len(verdicts)-good)
	fmt.Printf("  modelled %+.0f points over the horizons, actually returned %+d.\n", modelled, net)
	if ghosts > 0 {
		fmt.Printf("  %s\n", dim(fmt.Sprintf(
			"* %d of these sold a player who never appeared again in the window, worth %+d of\n"+
				"  that total. Those are upper bounds: an autosub would have covered him for free,\n"+
				"  so the eleven would not actually have carried the zero.", ghosts, ghostNet)))
	}
	fmt.Printf("  %s\n", dim("modelled is the change in the ELEVEN including captaincy; actual is raw points\n"+
		"  for both players whether or not they were picked. The two are not on the same\n"+
		"  footing, so read the sign and the ranking rather than the difference."))
}

// envFloat reads a float from the environment, returning 0 when unset or
// unparseable. Used only for diagnostic sweeps.
//
// It was a byte-identical second implementation of backtest.EnvFloat, which is
// where the switches it reads are also consumed — so the two could have drifted on
// how a mistyped value is treated while every arm they gate still printed.
func envFloat(name string) float64 { return backtest.EnvFloat(name) }

// chipPlanFromEnv reads the chip plan the replay should play, and defaults to none.
//
// # Why this is opt-in rather than simply reading the config
//
// The replay has always played a chipless season, and every figure this command has
// ever printed was measured that way. Reading cfg.Chips by default would move the
// season total for everyone who has a plan in their config — silently, since the
// output has no column that says which policy produced it. That is the shape of a
// contamination event in this project's own ledger, so the default stays put and
// seeing chips is something you ask for.
//
// `config` plays the plan you already keep; an explicit list plays one you do not
// have to save. Unrecognised names are an error rather than a skip, because a
// mistyped chip that silently does nothing returns a season indistinguishable from
// a chipless one — the byte-identical null this project has been caught by before.
// A season from 2025-26 grants a SECOND set from GW20, so a name may carry a `2`
// suffix — `wc2=28` is the second wildcard. `config` fills the first set from your
// saved plan and leaves the second empty, because config.json holds one plan and
// inventing four more weeks on your behalf is a decision, not a default.
func chipPlanFromEnv(cfg config.Config, season string) (first, second analysis.ChipPlan, err error) {
	spec := strings.TrimSpace(os.Getenv("FPL_CHIP_PLAN"))
	if spec == "" {
		return analysis.ChipPlan{}, analysis.ChipPlan{}, nil
	}
	if spec == "config" {
		// Split on the reset rather than dropping everything into the first set.
		// the team file may hold one flat plan, so a real 2025-26 plan will have a chip
		// after GW19 — and putting that in the first set makes `config` a hard
		// error on the one season the second set exists for. On a season with the
		// reset, a chip at GW20 or later IS a second-set chip; there is nothing
		// else it could be. On a one-set season nothing moves, because
		// ValidateChipSets does not police halves there.
		// An explicit two-set plan is taken as written; only a flat one is split.
		// Splitting a plan that already says which set each chip belongs to would
		// override the author on the one point they were explicit about.
		if cfg.Chips.Second != (analysis.ChipPlan{}) {
			return cfg.Chips.First, cfg.Chips.Second, nil
		}
		if backtest.ChipSetsFor(season) < 2 {
			return cfg.Chips.First, analysis.ChipPlan{}, nil
		}
		a, b := splitOnChipReset(cfg.Chips.First)
		return a, b, nil
	}
	// Parsed by `analysis.ParseChipSchedule` rather than here. This function
	// carried its own slot table, which was the third expression of the same
	// eight names — and the one most likely to drift, since it is the only one a
	// sweep author types by hand.
	s, err := analysis.ParseChipSchedule(spec)
	if err != nil {
		return analysis.ChipPlan{}, analysis.ChipPlan{}, fmt.Errorf("FPL_CHIP_PLAN: %w", err)
	}
	return s.First, s.Second, nil
}

// replayTitle names a replay page for the weeks it actually shows.
//
// "first N" is wrong when N is every week there is: a page headed "first 38
// gameweeks" reads as a truncation of something longer, and a reader who wants the
// whole season cannot tell they already have it. The comparison is against the
// weeks the replay produced rather than against 38, because a mid-season entry has
// fewer and is still complete.
func replayTitle(season string, shown, played int) string {
	if shown >= played {
		return fmt.Sprintf("Replay — %s, all %d gameweeks", season, shown)
	}
	return fmt.Sprintf("Replay — %s, first %d gameweeks", season, shown)
}

// describeChipPlan renders a plan for the terminal, e.g. "wildcard GW6, bench boost GW8".
// Rendered through `ChipSchedule.Entries` rather than a local table. This
// carried its own four-name list and had already drifted: it ordered wildcard,
// bench boost, triple captain, free hit, while every other renderer uses play
// order, so the replay page listed a plan differently from `armband chips`.
// That is the drift the shared type exists to prevent, surviving inside the
// commit that introduced the type. Found by review.
//
// Takes a single ChipPlan because both callers hold one half at a time; the
// labels are therefore unsuffixed, and describeChipSets names the halves.
func describeChipPlan(p analysis.ChipPlan) string {
	var out []string
	for _, c := range (analysis.ChipSchedule{First: p}).Entries() {
		out = append(out, fmt.Sprintf("%s GW%d", strings.ToLower(c.Label), c.GW))
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
}

// freeHitCaveat explains what a free-hit week's squad table is showing.
//
// The replay records the PERMANENT fifteen in Week.Squad and the BORROWED eleven
// in Week.XI, which is correct — the free hit reverts, so the permanent squad is
// the state every later decision is taken from. But it means the table on this
// page is not the team that scored the header's points, and almost none of the
// borrowed eleven appears in it, so nearly every row files as a substitute. Saying
// so is the only honest option available without carrying a second fifteen.
func freeHitCaveat(freeHit, hasSquad bool) string {
	if !freeHit {
		return ""
	}
	if hasSquad {
		return "Free hit — the table is the BORROWED fifteen that scored these points, " +
			"handed back after this gameweek. The permanent squad sat out and its " +
			"points did not count this week."
	}
	return "Free hit — the borrowed fifteen that scored these points was not " +
		"recorded. The squad below is the PERMANENT one, which sat out: its points " +
		"did not count this week."
}

// splitOnChipReset sorts one flat plan into the two sets a reset season grants.
//
// config.json carries a single four-slot plan, which is the right shape for the
// live agent and the wrong shape for a season with two sets. A chip at GW20 or
// later cannot be a first-set chip on such a season — the first set expires at the
// GW19 deadline — so the placement determines the set with nothing to infer.
func splitOnChipReset(p analysis.ChipPlan) (first, second analysis.ChipPlan) {
	move := func(gw int, a, b *int) {
		if gw >= backtest.ChipResetGW {
			*b = gw
			return
		}
		*a = gw
	}
	move(p.Wildcard, &first.Wildcard, &second.Wildcard)
	move(p.FreeHit, &first.FreeHit, &second.FreeHit)
	move(p.BenchBoost, &first.BenchBoost, &second.BenchBoost)
	move(p.TripleCaptain, &first.TripleCaptain, &second.TripleCaptain)
	return first, second
}

// describeChipSets names both halves, labelled, because "wildcard GW6, wildcard
// GW28" in one list reads as a plan that plays a chip it does not have.
func describeChipSets(first, second analysis.ChipPlan) string {
	if second == (analysis.ChipPlan{}) {
		return describeChipPlan(first)
	}
	return fmt.Sprintf("first half — %s; second half — %s",
		describeChipPlan(first), describeChipPlan(second))
}

// writeReplayHTML renders the first n gameweeks of a replay as a clickable page.
//
// The conversion lives here rather than in internal/present because it needs the
// season's archive: a replay records element ids, and turning those into names,
// clubs and what each player actually scored that week is this package's job. The
// renderer stays ignorant of the archive, which is what lets one template serve both
// a forecast and a finished season.
func writeReplayHTML(path, season string, cur *backtest.Season, sim *backtest.SimResult, n int) error {
	if n > len(sim.Weeks) {
		n = len(sim.Weeks)
	}
	short := map[int]string{}
	for _, t := range cur.Teams {
		short[t.ID] = t.ShortName
	}
	pos := map[int]string{1: "GKP", 2: "DEF", 3: "MID", 4: "FWD"}

	// Who each club played, per gameweek. Built from the archive's own fixture list
	// so a genuine blank stays empty and a double reads as both matches — the two
	// cases the card is there to distinguish.
	opp := map[int]map[int]string{}
	for _, fx := range cur.Fixtures {
		if fx.Event == nil {
			continue
		}
		gw := *fx.Event
		if opp[gw] == nil {
			opp[gw] = map[int]string{}
		}
		add := func(team int, label string) {
			if prev := opp[gw][team]; prev != "" {
				opp[gw][team] = prev + " + " + label
				return
			}
			opp[gw][team] = label
		}
		add(fx.TeamH, short[fx.TeamA]+" (H)")
		add(fx.TeamA, short[fx.TeamH]+" (A)")
	}

	// What a player actually scored in a gameweek. Not on the Week struct, which
	// records who was held rather than what they returned.
	got := func(id, gw int) int {
		if p := cur.Players[id]; p != nil {
			if g, ok := p.GWs[gw]; ok {
				return g.Points
			}
		}
		return 0
	}
	// A move is judged over the horizon it was made on, not the rest of the season:
	// the policy re-decides every week, so a GW8 move is not a commitment to May.
	over := func(id, gw, weeks int) int {
		t := 0
		for g := gw; g < gw+weeks; g++ {
			t += got(id, g)
		}
		return t
	}

	var out []present.ReplayWeek
	var prev []int
	for i := 0; i < n; i++ {
		w := sim.Weeks[i]
		xi := map[int]bool{}
		for _, id := range w.XI {
			xi[id] = true
		}
		fresh := map[int]bool{}
		if prev != nil {
			held := map[int]bool{}
			for _, id := range prev {
				held[id] = true
			}
			for _, id := range w.Squad {
				if !held[id] {
					fresh[id] = true
				}
			}
		}

		rw := present.ReplayWeek{
			GW: w.GW, Points: w.Net, HitCost: w.HitCost,
			Captain: w.Captain, CaptainPts: w.CaptainPts,
			Value: w.Value, Bank: w.Bank,
			// The replay's own count, not the length of the Move list below: a
			// wildcard's swaps are counted and not recorded, so deriving the
			// season total from the rendered rows reported 30 for a season this
			// command called 37.
			Transfers: w.Transfers,
			// Wildcard only. A free hit does rebuild, but only for one week; the
			// borrowed fifteen is Week.FreeHitSquad and the page renders IT —
			// the permanent squad sat out and its points did not count.
			Rebuilt: w.Wildcard,
			Caveat:  freeHitCaveat(w.FreeHit, len(w.FreeHitSquad) > 0),
		}
		if len(w.FreeHitSquad) > 0 {
			for _, id := range w.FreeHitSquad {
				p := cur.Players[id]
				if p == nil {
					continue
				}
				rw.FreeHitSquad = append(rw.FreeHitSquad, present.ReplayPlayer{
					Name: p.WebName, Team: short[p.Team], Position: pos[p.Type],
					Points: got(id, w.GW), Started: xi[id],
					Captain: p.WebName == w.Captain,
					// Not New: the borrowed fifteen was never bought — it is
					// handed back next gameweek.
					Opponent: opp[w.GW][p.Team],
				})
			}
		}
		switch {
		case w.Wildcard:
			rw.Chip = "wildcard"
		case w.FreeHit:
			rw.Chip = "free hit"
		case w.BenchBoost:
			rw.Chip = "bench boost"
		case w.TripleCaptain:
			rw.Chip = "triple captain"
		}
		for _, id := range w.Squad {
			p := cur.Players[id]
			if p == nil {
				continue
			}
			rw.Squad = append(rw.Squad, present.ReplayPlayer{
				Name: p.WebName, Team: short[p.Team], Position: pos[p.Type],
				Points: got(id, w.GW), Started: xi[id],
				Captain: p.WebName == w.Captain, New: fresh[id],
				Opponent: opp[w.GW][p.Team],
			})
		}
		for _, m := range sim.Moves {
			if m.GW != w.GW {
				continue
			}
			rw.Moves = append(rw.Moves, present.ReplayMove{
				Out: m.Out, In: m.In, Gain: m.Gain, Hit: m.Hit,
				OutGot: over(m.OutID, m.GW, 5), InGot: over(m.InID, m.GW, 5),
			})
		}
		out = append(out, rw)
		prev = w.Squad
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write replay HTML: %w", err)
	}
	defer f.Close()
	title := replayTitle(season, n, len(sim.Weeks))
	if err := present.HTMLReplay(f, out, title, ""); err != nil {
		return fmt.Errorf("render replay HTML: %w", err)
	}
	return f.Close()
}
