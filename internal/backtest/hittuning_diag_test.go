package backtest

// Transfer-hit tuning: the measurement the hit branch's no-gain-bar asymmetry
// was preserved for.
//
//	DIAG=1 EXP=HITTUNE FPL_CELLS=/tmp/hittune.csv FPL_HITS_CSV=/tmp/hittune-hits.csv \
//	  scripts/replay -run TestDiagHitTuning -v -timeout 2h
//
// # The question
//
// The user, 2026-08-18, watching the 2025-26 replay take two transfers in GW2,
// GW5 and GW33: "we take so many hits. there's no way that pays off... measure
// which hits 'worked out', i.e. did we gain 4 more points than we would have if
// we'd have just waited another week? or was it a net loss. If we lose too many
// bets it's a sign we're not tuned well."
//
// # Why the knob is MinGainHit and not a new gain bar — the kink, stated up front
//
// The plan originally swept a per-gameweek gain bar on the hit branch. The plan
// review showed that bar cannot bind: a hit is accepted iff
// Gain·H − 4 − FreeCost·(n−h) beats the alternative by MinGainHit, so the
// branch's implied per-gameweek bar is (MinGainHit + 4)/H = 7/H = 1.4 pts/gw at
// the shipped horizon — 3.5× the free single's 0.4. Rungs at 0.2-0.6 sit below
// the existing bar and would have been a null by construction. **The knob that
// binds is MinGainHit (3.0, net across the horizon, NEVER swept — banked
// provenance stamps it and no sweep has moved it).** Raising it raises the bar
// on both hit channels with the horizon scaling built in.
//
// # The arms — six, one grid
//
//   - the MinGainHit ladder: 3 (shipped) / 4 / 5 / 6, on the flat machine.
//   - "mgh3, floored machine": the SHIPPED target — early floor {1.0, 0.2}
//     through GW8 + the override-mode corner (anchoredPlan + AnticipateChips +
//     BankLookahead + WeeklyXI). The floor lowers the pair's gain bar to 0.2
//     exactly in the user's GW2/GW5 window, so the flat ladder may understate
//     the bar; this arm is the shipping-target baseline, outside the family.
//   - "no hits (wait)": MaxHits 0 — the wait-everything counterfactual at
//     season level.
//   - "mgh3, full plan (shipped)": the floored machine on the shipped
//     USER-FACING planner — FullAnchoredPlan, both chip sets, all four chips.
//     The machine the user watches, for the holding criterion to be live
//     against free hits, bench boosts and wildcards. Outside the family, like
//     the floored machine.
//
// Six seasons × six entry points, 36 cells per arm, 252 cells, POLICY.
//
// # Registered contrasts (Holm over the THREE rungs only)
//
//	4 vs 3, 5 vs 3, 6 vs 3 — each its own threshold, POLICY, paired per cell.
//	The floored-machine arm and the no-hits arm are outside the family, each
//	with its own threshold: the first is a machine contrast, the second a
//	different field (MaxHits), and spending the Holm budget on a contrast
//	that cannot fail is what the budget exists to refuse.
//
// ⚠️ **The points arm is a VETO only, pre-registered.** The whole hit program
// is ~0.8-3.2 hits per cell (~8 points a season) against this grid's ~26-39
// point thresholds — unmeasurable by design. A resolving rung must be
// non-negative on points; a null is a tie and a measured loss does not ship.
//
// # The verdict quantities, and which are registered
//
// The cells CSV carries no per-move detail, so the verdicts ride a sidecar
// (FPL_HITS_CSV): per package — a funded pair is ONE package, its legs' gains
// zeroed individually and its hit charged once — sweep, arm, season, entry,
// GW, package size, hit_net (the package's realised net over the decided
// horizon, hit charge included), out_played (all sold legs appeared — a sold
// player who never appears overstates the hit's net, because an autosub
// covers him for free; the record puts that at 19% of transfers), hit (which
// packages paid the −4 — the registered per-hit rates filter on it), the
// in-player ids, and wildcard_after (the week after the plan's wildcard).
// The no-hits arm emits its FREE packages the same way, so a hit's
// wait-counterfactual — the same in-player bought later, free — can be
// matched post-hoc.
//
// Registered: (a) the shipped arm's per-hit loss rate against the gate's OWN
// bar — a calibrated gate gives ~50% by truncation at net < 3, so a rate
// clearly above that is the "not tuned well" signal; (b) the season-level
// shipped-vs-no-hits paired contrast. Descriptive: the wildcard-week-after
// split, and the availability-adjusted rates beside the raw. ⚠️ The workedOut
// wait-match the first measurement registered is SUPERSEDED by the
// holding-window criterion below (the user's ruling, 2026-08-18) — the match
// reached only 48% of hits and its window was the very artefact being
// replaced.
//
// # The holding-window criterion — the user's ruling, and the two splits
//
// 2026-08-18, after the horizon measurement landed: "We should only judge if
// it was +4 net points before they were either transferred out, or we
// wildcarded. Anything less than +4 is not worth it for a hit." And:
// "account for free hits and bench boost too. Captaincy too." And: "separate
// hits due to injury (the replaced player stops playing) vs due to
// preference."
//
// The horizon criterion judges over the decided horizon — five model
// gameweeks — which credits a hit with weeks the player later spent in a
// different squad and cuts it off before a long hold pays. The holding
// criterion replaces it:
//
//   - The window: per leg, from the transfer week until the earlier of the
//     in-player's sale, a wildcard, or the season's end — the span the squad
//     actually held him.
//   - The in-side: Week.Contrib — the incoming player's recorded squad
//     contribution, from the same scoring pass that produced the week's
//     Gross: autosubs in, the armband's copies to the captain (or the vice
//     under the fallback), every bench player on a bench-boost week, and
//     nothing on a free-hit week (the borrowed fifteen was what scored).
//   - The out-side: the sold player's raw points — the Judge convention, the
//     counterfactual eleven being unknowable — skipped on a free-hit week,
//     and a week whose sold player was the previous week's captain is
//     flagged, because raw points then understate what the squad gave up.
//   - The bar: a hit worked iff the package's holding net is ≥ +4; a free
//     transfer's bar is 0 (descriptive).
//   - The split: a hit whose sold player stops appearing over the window
//     (forced — without the move the squad was fielding a blank) is reported
//     apart from one whose sold player kept playing (preference — the real
//     bet). The tuned-bets population is the preference one.
//
// Registered: (H) the holding-clearance share — % of hit packages clearing +4
// in the hold — per arm, split forced vs preference; (H') the rung pattern —
// the preference-hit share by MinGainHit rung 3/4/5/6, where a monotone rise
// is the bar doing its job on the user's criterion (suggestive evidence to
// re-open the shipping rule; reported, not shipped). The horizon criterion
// stays reported beside it, so the record carries what each says.
//
// ⚠️ The free-hit, bench-boost and wildcard cuts are LIVE only in the
// full-plan arm; in the other six arms no chip plays, so the cuts bind
// nowhere there (a fact, reported). ⚠️ Confinement: the six existing arms'
// 216 cells must come back byte-identical to the first bank on every integer
// column — Contrib is recording-only, so no decision may flip.
//
// # The shipping rule — pre-registered, so the decision is not post-hoc
//
// The smallest rung clears all of: (a) its availability-adjusted loss rate
// (net < 3) is below the shipped arm's by more than the paired noise of the
// per-cell rates; (b) its POLICY point estimate is ≥ 0 and non-negative in
// ≥ 5 of 6 season means; (c) HOLD byte-identical (code fact); (d) the hit
// reduction is not offset move-for-move by free transfers (moves − hits must
// not rise by the full hit reduction — the bar refused, it did not convert).
// A byte-identical rung is NOT a shipping outcome: "suggestive" means
// pre-registered direction + consistency + no measured loss — looser than
// established, still excluding a tie.

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"testing"
)

func TestDiagHitTuning(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== transfer-hit tuning: MinGainHit ladder, the floored machine, and no hits\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("FPL_SWEEP_SEASONS=%s  FPL_SWEEP_STARTS=%s  entry points %v\n\n",
		gridEnv("FPL_SWEEP_SEASONS", "extended (the shipped default)"),
		gridEnv("FPL_SWEEP_STARTS", "1,6,11,16,21,26 (the shipped default)"), starts)

	// The sidecar: per-package verdict rows, one writer for all arms. Opened
	// before any cell runs; flushed per row so a killed run keeps its partials.
	path := os.Getenv("FPL_HITS_CSV")
	var side *csv.Writer
	if path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		side = csv.NewWriter(f)
		side.Write([]string{"sweep", "arm", "season", "start_gw", "gw",
			"n_moves", "hit_net", "out_played", "wildcard_after", "in_ids", "hit",
			"hold_net", "hold_out_played", "hold_weeks", "out_was_captain"})
	}

	// emit writes one row per package this cell's replay actually took. A
	// funded pair is one package: the legs' verdicts sum, and the hit is
	// charged once, exactly as the gate judged it.
	emit := func(arm string, pair seasonPair, start int, res *SimResult, wcWeeks []int) {
		if side == nil {
			return
		}
		byGW := map[int][]Verdict{}
		for _, v := range Judge(pair.Cur, res.Moves, cfg.Weights.Horizon) {
			byGW[v.GW] = append(byGW[v.GW], v)
		}
		for gw, vs := range byGW {
			net, played, hit := 0, true, false
			var ins []string
			for _, v := range vs {
				net += v.Net()
				played = played && v.OutPlayed
				hit = hit || v.Hit
				ins = append(ins, strconv.Itoa(v.InID))
			}
			// The no-hits arm emits its free packages too — the horizon and
			// holding nets of a free move are the free channel's verdicts.
			after := false
			for _, wc := range wcWeeks {
				if wc > 0 && gw == wc+1 {
					after = true
				}
			}
			holdNet, holdWeeks, allPlayed, outWasCap := holdingNet(pair.Cur, res, start, vs, wcWeeks)
			side.Write([]string{"HITTUNE", arm, pair.Name, strconv.Itoa(start),
				strconv.Itoa(gw), strconv.Itoa(len(vs)), strconv.Itoa(net),
				strconv.FormatBool(played), strconv.FormatBool(after),
				joinIDs(ins), strconv.FormatBool(hit),
				strconv.Itoa(holdNet), strconv.FormatBool(allPlayed),
				strconv.Itoa(holdWeeks), strconv.FormatBool(outWasCap)})
		}
		side.Flush()
	}

	floored := func(sc *SimConfig) {
		sc.ChipPlanner = anchoredPlan
		sc.AnticipateChips = true
		sc.BankLookahead = true
		sc.WeeklyXI = true
		sc.EarlyFloor.FreeTransferValue = 1.0
		sc.EarlyFloor.MinGainForTransfer = 0.2
		sc.EarlyFloor.UntilGameweek = 8
	}
	fullPlan := func(sc *SimConfig) {
		floored(sc)
		// The shipped user-facing planner, both sets and all four chips,
		// installed through the variant's plan hook — a ChipPlan cannot
		// express two wildcards, so the planner field cannot carry it.
		sc.ChipPlanner = nil
	}
	arms := []policyVariant{
		{label: "mgh3, flat (shipped)", apply: func(sc *SimConfig) {},
			observe: func(pair seasonPair, start int, res *SimResult) {
				emit("mgh3, flat (shipped)", pair, start, res, nil)
			}},
		{label: "mgh4, flat", apply: func(sc *SimConfig) { sc.MinGainHit = 4 },
			observe: func(pair seasonPair, start int, res *SimResult) {
				emit("mgh4, flat", pair, start, res, nil)
			}},
		{label: "mgh5, flat", apply: func(sc *SimConfig) { sc.MinGainHit = 5 },
			observe: func(pair seasonPair, start int, res *SimResult) {
				emit("mgh5, flat", pair, start, res, nil)
			}},
		{label: "mgh6, flat", apply: func(sc *SimConfig) { sc.MinGainHit = 6 },
			observe: func(pair seasonPair, start int, res *SimResult) {
				emit("mgh6, flat", pair, start, res, nil)
			}},
		{label: "mgh3, floored machine", apply: floored,
			observe: func(pair seasonPair, start int, res *SimResult) {
				// The resolved plan, both sets, exactly as Simulate routed it.
				sch := SplitChipSets(pair.Cur.Name, anchoredPlan(pair.Cur, start))
				emit("mgh3, floored machine", pair, start, res,
					[]int{sch.First.Wildcard, sch.Second.Wildcard})
			}},
		{label: "no hits (wait)", apply: func(sc *SimConfig) { sc.MaxHits = 0 },
			observe: func(pair seasonPair, start int, res *SimResult) {
				emit("no hits (wait)", pair, start, res, nil)
			}},
		{label: "mgh3, full plan (shipped)", apply: fullPlan,
			plan: FullAnchoredPlan,
			observe: func(pair seasonPair, start int, res *SimResult) {
				sch := FullAnchoredPlan(pair.Cur, start)
				emit("mgh3, full plan (shipped)", pair, start, res,
					[]int{sch.First.Wildcard, sch.Second.Wildcard})
			}},
	}
	runPolicySweep(t, arms, starts)
}

// holdingNet is the package's realised net over its actual holding window —
// the user's criterion, +4 net before the in-players are sold or a wildcard
// replaces the squad. Per leg, each gameweek from the transfer to the earlier
// of the in-player's sale, a wildcard, or the season's end accrues the
// incoming player's recorded squad contribution (Week.Contrib — autosubs,
// armband and bench boost inside it, a free-hit week contributing nothing)
// minus the sold player's raw points. The out-side is the Judge convention —
// the counterfactual eleven is unknowable — and a week whose sold player was
// the previous week's captain is flagged, because raw points understate what
// the squad gave up there. weeks is the longest leg's window; allPlayed is
// whether every sold leg appeared over its window (the forced-vs-preference
// split).
func holdingNet(s *Season, res *SimResult, start int, vs []Verdict, wcWeeks []int) (net, weeks int, allPlayed, outWasCaptain bool) {
	allPlayed = true
	// week returns the recorded week for a gameweek. Weeks holds one entry per
	// gameweek from the entry point onward — `for gw := start` — so a gameweek
	// is indexed gw-start, and the GW field is the tripwire if that ever
	// changes.
	week := func(gw int) (Week, bool) {
		i := gw - start
		if i < 0 || i >= len(res.Weeks) {
			return Week{}, false
		}
		w := res.Weeks[i]
		if w.GW != gw {
			panic(fmt.Sprintf("holdingNet: weeks misaligned at gw %d (found %d, start %d)", gw, w.GW, start))
		}
		return w, true
	}
	for _, v := range vs {
		end := 39 // exclusive: the first gameweek the in-player was no longer held
		for _, m := range res.Moves {
			if m.GW > v.GW && m.GW < end && m.OutID == v.InID {
				end = m.GW
			}
		}
		for _, wc := range wcWeeks {
			if wc > v.GW && wc < end {
				end = wc
			}
		}
		leg := 0
		for gw := v.GW; gw < end && gw <= 38; gw++ {
			w, ok := week(gw)
			if !ok {
				continue
			}
			if w.FreeHit {
				// The permanent squad sat that week out on both sides — the
				// borrowed fifteen was what scored.
				continue
			}
			leg += w.Contrib[v.InID]
			if p := s.Players[v.OutID]; p != nil {
				leg -= p.GWs[gw].Points
			}
		}
		net += leg
		if end-v.GW > weeks {
			weeks = end - v.GW
		}
		allPlayed = allPlayed && minutesOver(s, v.OutID, v.GW, end-v.GW) > 0
		if v.GW > start {
			if w, ok := week(v.GW - 1); ok {
				if p := s.Players[v.OutID]; p != nil {
					outWasCaptain = outWasCaptain || w.Captain == p.WebName
				}
			}
		}
	}
	return
}

// joinIDs joins a package's in-player ids with a pipe, so the wait-match in R
// can pair a hit package with the no-hits arm's later free purchase of the
// same player.
func joinIDs(ins []string) string {
	out := ""
	for i, id := range ins {
		if i > 0 {
			out += "|"
		}
		out += id
	}
	return out
}
