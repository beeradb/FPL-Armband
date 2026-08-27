package backtest

// Can the banking rule's waiting arm fire at all at shipped config?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBankingReachability -v -timeout 2h
//
// # The question
//
// The one banked BankLookahead arm reads 236 consulted weeks, 169 weighed, and
// **0 banked**. So in 72% of consulted weeks the rule had a real choice and chose
// to act now every time, and a zero of that shape has two readings that license
// opposite conclusions: waiting is genuinely worth nothing on this football, or
// waiting is near-unreachable *by construction* and the zero is a fact about the
// specification.
//
// The hypothesis, stated as a hypothesis: `shouldBank` prices the later arm at
// `gw+1` over `horizon-1` on today's board with `free+1`, so its only two
// differences from the now-arm are ONE EXTRA MOVE and ONE FEWER GAMEWEEK OF GAIN.
// `MoveLimit` is `free + hits`, so at the shipped one hit the now-arm already
// reaches two moves and the pair search already enumerates two-move packages —
// the extra free transfer buys a capability only where the best package needs
// three moves — while the shorter horizon costs a flat `1/h`, a 20% haircut at
// the shipped horizon of 5.
//
// # What this is NOT
//
// It is not a points comparison. Nothing here is a threshold, a p-value or a
// verdict about what banking is worth, and the two arms it prints are internal
// package valuations rather than replayed points. It is a liveness question about
// an instrument: whether the branch a tandem sweep would attribute a result to
// can execute at all.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// TestTheBankLogChangesNoDecision is the claim SimConfig.bankLog makes about
// itself, asserted rather than traced. It is TestTheGateLogChangesNoDecision's
// twin, and exists for the same reason: an observer that reaches the decision
// path would make every figure this file prints a description of a season nobody
// else replays.
//
// It also covers the enumeration split the probe needed. `shouldBank` used to
// call one function that enumerated and valued in one step; it now enumerates
// once and values the same packages at two horizons, and a slip in that
// refactoring would move the shipped decision without anything else noticing.
// `bankingArm` is the arm to check it on, because at shipped config the rule
// banks zero times and a season that never takes the banked branch cannot show a
// change in it.
func TestTheBankLogChangesNoDecision(t *testing.T) {
	cur, prior, base := chipSim(t)

	for _, c := range []struct {
		name string
		sc   SimConfig
	}{
		{"shipped hits, lookahead on", func() SimConfig {
			s := base
			s.BankLookahead = true
			return s
		}()},
		// The arm that actually banks, so the branch the probe sits in front of
		// is executed rather than merely compiled.
		{"no hits, lookahead on (banks)", bankingArm(base)},
	} {
		quiet, err := Simulate(cur, prior, c.sc)
		if err != nil {
			t.Fatalf("%s baseline: %v", c.name, err)
		}
		logged := c.sc
		var n int
		logged.bankLog = func(bankProbe) { n++ }
		noisy, err := Simulate(cur, prior, logged)
		if err != nil {
			t.Fatalf("%s logged: %v", c.name, err)
		}
		if n == 0 {
			t.Errorf("%s: the bank log was never called, so this comparison is "+
				"vacuous — every arm here sets BankLookahead", c.name)
		}
		if quiet.Points != noisy.Points || quiet.Transfers != noisy.Transfers ||
			quiet.Hits != noisy.Hits ||
			quiet.Banking != noisy.Banking ||
			squadHash(quiet.OpeningSquad) != squadHash(noisy.OpeningSquad) {
			t.Errorf("%s: the bank log changed the season it observes: points %d "+
				"against %d, moves %d against %d, hits %d against %d, banking %+v "+
				"against %+v, squads %s against %s", c.name,
				quiet.Points, noisy.Points, quiet.Transfers, noisy.Transfers,
				quiet.Hits, noisy.Hits, quiet.Banking, noisy.Banking,
				squadHash(quiet.OpeningSquad), squadHash(noisy.OpeningSquad))
		}
	}
}

// TestTheBankProbeReportsTheArmsShouldBankCompared pins the probe to the
// decision, so it cannot drift into describing a comparison that did not happen.
//
// The probe is what the reachability reading below rests on, and this project's
// standing rule is that a diagnostic must never carry its own copy of the thing
// it checks. So the assertion is an identity rather than a value: the boolean
// `shouldBank` returned must be exactly `Later > Now` on the two numbers the
// probe reported, in every consulted week, and the guarded weeks must report no
// arms at all.
func TestTheBankProbeReportsTheArmsShouldBankCompared(t *testing.T) {
	cur, prior, base := chipSim(t)

	sc := bankingArm(base)
	var probes []bankProbe
	sc.bankLog = func(p bankProbe) { probes = append(probes, p) }
	res, err := Simulate(cur, prior, sc)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != res.Banking.ConsultedWeeks {
		t.Fatalf("the probe fired in %d weeks and the rule was consulted in %d — "+
			"a probe that misses a week reports a funnel narrower than the one that ran",
			len(probes), res.Banking.ConsultedWeeks)
	}
	banked, weighed := 0, 0
	for _, p := range probes {
		if p.Banked {
			banked++
		}
		if p.Weighed {
			weighed++
		}
		if p.Guard != 0 {
			if p.Now != 0 || p.Later != 0 {
				t.Errorf("GW%d: a guard refused before anything was priced and the "+
					"probe reports arms %v/%v", p.GW, p.Now, p.Later)
			}
			continue
		}
		// analysis.PreferWaiting rather than a bare `>`. The tie direction is a
		// function precisely so it has one implementation, and a copy here would
		// fail with "the probe has drifted" on the day somebody legitimately moved
		// the rule — accusing the instrument of the change it was watching for.
		if got := analysis.PreferWaiting(p.Now, p.Later); got != p.Banked {
			t.Errorf("GW%d: the probe reports now %v, later %v and banked=%v — the "+
				"decision and the arms it is attributed to have come apart",
				p.GW, p.Now, p.Later, p.Banked)
		}
		// The two counterfactual arms are re-pricings of package sets the decision
		// itself enumerated, so each must bracket its own arm in the one direction
		// arithmetic forces: more horizon is worth more, on a FIXED package set.
		//
		// ⚠️ There is deliberately no assertion between NoHaircut and Now. Those
		// are different package sets, and `bestPair` ranks on gain while
		// PackageValue charges per move, so the wider search can legitimately
		// return something worth less. Asserting a sign there would fail on a
		// correct run.
		if p.NoHaircut < p.Later {
			t.Errorf("GW%d: the later arm priced over the full horizon is %v, below "+
				"its own %v over one week fewer", p.GW, p.NoHaircut, p.Later)
		}
		if p.NoExtraMove > p.Now {
			t.Errorf("GW%d: the now arm priced over one week fewer is %v, above its "+
				"own %v over the full horizon", p.GW, p.NoExtraMove, p.Now)
		}
		// The identity behind the whole decomposition: with the preparation
		// switches off the two arms differ only by their move limit, so an
		// identical candidate list forces an identical no-haircut value.
		if p.SamePackages && p.NoHaircut != p.Now {
			t.Errorf("GW%d: the two arms enumerated the same candidate list and the "+
				"no-haircut re-pricing is %v against the now arm's %v — the same "+
				"packages at the same horizon cannot be worth different amounts",
				p.GW, p.NoHaircut, p.Now)
		}
	}
	if banked != res.Banking.BankedWeeks || weighed != res.Banking.WeighedWeeks {
		t.Errorf("the probe counted %d banked and %d weighed weeks; the mediator "+
			"counted %d and %d", banked, weighed,
			res.Banking.BankedWeeks, res.Banking.WeighedWeeks)
	}
	if banked == 0 {
		t.Fatal("this arm banked nothing, so the identity above was only ever " +
			"checked on the false branch — see TestTheBankingRuleActuallyFires")
	}
}

// bankReach is one arm's probes, reduced to the counts the reachability question
// is answered from.
type bankReach struct {
	label     string
	cells     int
	consulted int
	// The two guards, counted apart. Collapsing them hides which constraint bound:
	// the ceiling says the allowance was already at BankUpTo, the horizon says the
	// season was ending, and an arm that banks often should hit the first more.
	guardCeiling, guardHorizon int
	weighed                    int
	banked                     int
	extraMoveWouldFlip         int // NoHaircut > Now: waiting wins if it costs no week
	extraMoveHurts             int // NoHaircut < Now: the wider search returned a worse-VALUED package
	moreMovesAvailable         int // LimitLater > Limit: the extra transfer widened the limit at all
	laterPackageIsBigger       int // the later arm's winning package spends more moves
	// samePackages is weeks the two arms enumerated the IDENTICAL candidate list,
	// which is the decisive count. An extra-move channel of zero means one of two
	// things — the wider limit found nothing new, or it found something that lost
	// on value — and only this separates them. See bankProbe.SamePackages.
	samePackages int
	// noPairNow is weeks the now-arm's move limit was below 2, so transferPackages
	// skipped bestPair entirely and the arm could only offer a single swap.
	// pairUnlocked is the subset where the later arm's limit reached 2, so waiting
	// bought the paired downgrade-and-upgrade rather than merely a wider one.
	// This is the mechanism claim in two counts: MoveLimit is free + hits, so the
	// hit allowance decides whether the pair is reachable without waiting at all.
	noPairNow, pairUnlocked int
	// The same three counts restricted to weeks the now-arm ALREADY had a pair
	// (limit >= 2), which is the boundary shipped config lives on and the one its
	// own arm can never leave. Without this the MaxHits: 0 control is a control on
	// the 1 -> 2 boundary, and shipped config only ever visits 2 -> 3 and above.
	wideWeeks, wideFlip, wideSame int
	freeSum                       int
	limitSum                      int
	limitLaterSum                 int
	ratios                        []float64 // Later/Now on weeks where Now > 0
	haircut                       []float64 // (Now - NoExtraMove)/Now, the horizon cost alone
	extra                         []float64 // (NoHaircut - Now)/Now, the extra move alone
}

// unguarded is the denominator every count below belongs over, and the reason
// the allowance sums are accumulated past the guard rather than before it: a
// mean taken over 236 weeks printed beside counts taken over 226 is two
// denominators in one line.
func (r *bankReach) unguarded() int {
	return r.consulted - r.guardCeiling - r.guardHorizon
}

func (r *bankReach) add(p bankProbe) {
	r.consulted++
	switch p.Guard {
	case analysis.BankGuardCeiling:
		r.guardCeiling++
		return
	case analysis.BankGuardHorizon:
		r.guardHorizon++
		return
	}
	r.freeSum += p.Free
	r.limitSum += p.Limit
	r.limitLaterSum += p.LimitLater
	if p.Weighed {
		r.weighed++
	}
	if p.Banked {
		r.banked++
	}
	if p.LimitLater > p.Limit {
		r.moreMovesAvailable++
	}
	if p.SamePackages {
		r.samePackages++
	}
	if p.Limit < 2 {
		r.noPairNow++
		if p.LimitLater >= 2 {
			r.pairUnlocked++
		}
	} else {
		r.wideWeeks++
		if p.NoHaircut > p.Now {
			r.wideFlip++
		}
		if p.SamePackages {
			r.wideSame++
		}
	}
	if p.NoHaircut > p.Now {
		r.extraMoveWouldFlip++
	}
	if p.NoHaircut < p.Now {
		r.extraMoveHurts++
	}
	if p.LaterMoves > p.NowMoves {
		r.laterPackageIsBigger++
	}
	if p.Now > 0 {
		r.ratios = append(r.ratios, p.Later/p.Now)
		r.haircut = append(r.haircut, (p.Now-p.NoExtraMove)/p.Now)
		r.extra = append(r.extra, (p.NoHaircut-p.Now)/p.Now)
	}
}

// quantiles reports the minimum, median, 90th percentile and maximum of a
// sample, which is what "how close did it get" needs and a mean does not: the
// question is whether the waiting arm ever approached the acting arm, not where
// the bulk sits.
//
// The minimum is here because the extra-move channel is NOT sign-constrained —
// `bestPair` ranks on gain and `PackageValue` charges per move — so a summary
// running only upward would hide the wider search returning something worse.
func quantiles(xs []float64) (min, med, p90, max float64) {
	if len(xs) == 0 {
		n := math.NaN()
		return n, n, n, n
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	at := func(q float64) float64 {
		i := int(q * float64(len(s)-1))
		return s[i]
	}
	return s[0], at(0.5), at(0.9), s[len(s)-1]
}

// TestDiagBankingReachability answers whether the waiting arm can fire at shipped
// config, and if not, which of its two channels is responsible.
//
// The grid is the one the recorded 236/169/0 was measured on: four seasons
// (FPL_SWEEP_SEASONS=default) entered at GW1 and GW16, WeeklyXI true, BankUpTo
// pinned at sweepBankLimit (5). Both arms of the MaxHits contrast run on it, so
// the mechanism claim — that `MoveLimit` is `free + hits` and the hit allowance
// already grants the move banking would wait for — is read off the same football
// rather than against a remembered figure.
func TestDiagBankingReachability(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := sweepPairNames()
	starts := []int{1, 16}

	arms := []struct {
		label   string
		maxHits int // -1 means the shipped setting
	}{
		{"shipped max_hits", -1},
		{"max_hits = 0", 0},
	}
	out := make([]*bankReach, len(arms))
	for i, a := range arms {
		out[i] = &bankReach{label: a.label}
	}

	for _, pair := range pairs {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		for _, start := range starts {
			for i, a := range arms {
				sc := sweepConfig(cfg, start, true)
				sc.BankLookahead = true
				if a.maxHits >= 0 {
					sc.MaxHits = a.maxHits
				}
				r := out[i]
				sc.bankLog = r.add
				res, err := Simulate(cur, prior, sc)
				if err != nil {
					t.Fatalf("%s %s from GW%d: %v", a.label, pair[1], start, err)
				}
				r.cells++
				if res.Banking.ConsultedWeeks == 0 {
					t.Fatalf("%s %s from GW%d consulted the rule 0 times — the arm "+
						"is not running the lever it names", a.label, pair[1], start)
				}
			}
		}
	}

	fmt.Printf("\nBANKING REACHABILITY — %s from GW1 and GW16, WeeklyXI=true,\n",
		seasonsLabel(len(pairs)))
	fmt.Printf("bank_up_to %d, decision horizon %v, min_gain %v, free_transfer_value %v.\n",
		sweepBankLimit, sweepConfig(cfg, 1, true).decisionHorizon(),
		cfg.Review.MinGainForTransfer, cfg.Review.FreeTransferValue)
	fmt.Printf("No chip preparation, no chips planned. Counts and distributions only:\n")
	fmt.Printf("no points figure, no threshold and no verdict about what banking is worth.\n")
	// The data state, printed rather than assumed. An oracle left on inflates
	// every figure at once, and the repair switches decide which archive ran.
	fmt.Printf("FPL_SWEEP_SEASONS=%q, FPL_NO_XG_REPAIR=%q, FPL_NO_XGC_REPAIR=%q, "+
		"oracles %+v.\n", os.Getenv("FPL_SWEEP_SEASONS"),
		os.Getenv("FPL_NO_XG_REPAIR"), os.Getenv("FPL_NO_XGC_REPAIR"),
		OraclesFromEnv())

	for _, r := range out {
		n := r.unguarded()
		fmt.Printf("\n--- %s, %d cells ---\n", r.label, r.cells)
		fmt.Printf("consulted %d, guarded %d (ceiling %d, horizon %d), weighed %d, "+
			"BANKED %d\n", r.consulted, r.guardCeiling+r.guardHorizon,
			r.guardCeiling, r.guardHorizon, r.weighed, r.banked)
		if n > 0 {
			// Every mean and every count on this page is over the SAME n. Mixing
			// the consulted and unguarded denominators in one line was the first
			// draft's defect.
			fmt.Printf("over the %d unguarded weeks: mean free at decision %.3f, "+
				"mean move limit now %.3f, later %.3f\n", n,
				float64(r.freeSum)/float64(n), float64(r.limitSum)/float64(n),
				float64(r.limitLaterSum)/float64(n))
			fmt.Printf("weeks the extra transfer widened the LIMIT (limit_later > "+
				"limit_now): %d of %d\n", r.moreMovesAvailable, n)
			fmt.Printf("weeks the two arms enumerated the IDENTICAL candidate list: "+
				"%d of %d\n", r.samePackages, n)
			fmt.Printf("weeks the NOW arm could not enumerate a pair at all "+
				"(limit_now < 2): %d of %d,\n  of which waiting unlocked one "+
				"(limit_later >= 2): %d\n", r.noPairNow, n, r.pairUnlocked)
			fmt.Printf("weeks the later arm's winning package spends MORE moves: %d of %d\n",
				r.laterPackageIsBigger, n)
			fmt.Printf("weeks waiting would win if it cost no gameweek "+
				"(no_haircut > now): %d of %d\n", r.extraMoveWouldFlip, n)
			fmt.Printf("weeks the wider search returned a WORSE-valued package "+
				"(no_haircut < now): %d of %d\n", r.extraMoveHurts, n)
			fmt.Printf("weeks the haircut alone would win: 0 of %d by arithmetic — "+
				"same packages,\n  less horizon, so the now-arm priced short can "+
				"never exceed itself priced long\n", n)
			// The boundary shipped config actually lives on, isolated. Shipped
			// MaxHits can never leave it; MaxHits: 0 spends part of its weeks here
			// and part at 1 -> 2, and only this subset compares like with like.
			fmt.Printf("RESTRICTED to weeks the now-arm ALREADY had a pair "+
				"(limit_now >= 2, the 2->3+ boundary): %d weeks,\n"+
				"  identical candidate list in %d, waiting would win if free in %d\n",
				r.wideWeeks, r.wideSame, r.wideFlip)
		}
		min, med, p90, max := quantiles(r.ratios)
		fmt.Printf("later/now over %d weeks with a live now-arm: min %.4f, "+
			"median %.4f, p90 %.4f, max %.4f\n", len(r.ratios), min, med, p90, max)
		min, med, p90, max = quantiles(r.haircut)
		fmt.Printf("horizon channel (now - now_short)/now:      min %.4f, median "+
			"%.4f, p90 %.4f, max %.4f\n", min, med, p90, max)
		min, med, p90, max = quantiles(r.extra)
		fmt.Printf("extra-move channel (no_haircut - now)/now:  min %.4f, median "+
			"%.4f, p90 %.4f, max %.4f\n", min, med, p90, max)
	}

	fmt.Printf("\nHow to read it. `later/now` at or below 1 in every week is the " +
		"rule declining;\nthe two channels below it say why. The extra-move channel " +
		"is what one more\ntransfer buys with the horizon cost removed, and the " +
		"horizon channel is what\nthe week costs with the move limit held. If the " +
		"extra-move column is zero in\nnearly every week, the haircut is not needed " +
		"to explain the zero at all.\n\n")
	fmt.Printf("⚠️ The identical-candidate-list count is what makes that " +
		"attributable.\nAn extra-move channel of zero has two causes and they " +
		"generalise differently:\nthe wider limit found nothing new (RankPairs " +
		"builds a multi-downgrade set only\nwhere no single sale funds the " +
		"upgrade), or it found something that topped\nthe GAIN ranking bestPair " +
		"sorts on and then lost on value. Read the\nRESTRICTED block, not the " +
		"pooled one: shipped max_hits can never reach\nlimit_now < 2, so the 1->2 " +
		"boundary is a neighbouring path rather than the\none under test.\n\n")
	fmt.Printf("⚠️ limit_now >= 2 at shipped max_hits is FORCED, not observed: the " +
		"weekly\naccrual runs before the search so free >= 1, and MoveLimit adds " +
		"the hit.\nLikewise limit_later > limit_now with max_moves unset. Neither " +
		"count is\nevidence about football.\n\n")
	fmt.Printf("⚠️ The channel figures are %%.4f prints, so a 0.0000 bounds the " +
		"channel\nbelow 1e-4. The identical-candidate-list count is the exact " +
		"statement.\n")
}
