package backtest

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
	"os"
	"time"
)

// PointInTime reconstructs what FPL was showing after gameweek `through`.
//
// Aggregates are accumulated from the per-gameweek rows, prices are taken from
// that week, and gameweeks 1..through are marked finished so the model's data
// window matches. Feed it 0 and it returns the pre-season view.
//
// Nothing after `through` is visible. That is the whole discipline of a replay:
// a simulation that can see next week's returns proves nothing.
//
// This is the inert wrapper: no hindsight of any kind. Everything that does not
// deliberately grant some — which is every caller outside a named oracle
// diagnostic — should keep calling it, and the fact that every existing test
// still compiles against it unchanged is the check that PointInTimeWith did not
// quietly change the default view of the world.
func PointInTime(cur, prior *Season, through int) (*fpl.Bootstrap, []fpl.Fixture) {
	return PointInTimeWith(cur, prior, through, Oracles{})
}

// PointInTimeWith is PointInTime with an oracle state.
//
// **This is the information seam, and it is genuinely a single point.** Together
// with PreSeasonWith it is the sole funnel through which the replay manufactures
// the model's view of the world: everything downstream — the engine, the
// optimiser, the transfer search — reads only the bootstrap and fixture list
// these two return. Any oracle that corrects *what the model believed* belongs
// here and nowhere else, which is what keeps internal/analysis oracle-free.
//
// The one honest complication is that prices enter the replay twice: as
// Element.NowCost here, which is what the optimiser is quoted, and again through
// marketPrice and wallet.sellPrices in decide, which is what the wallet actually
// pays. OracleTransactPrice perturbs the second and not the first — defensible
// for a *timing* oracle, since the opening squad has no timing choice to get
// right, and now a stated property rather than an accident of where a hook went.
func PointInTimeWith(cur, prior *Season, through int, o Oracles) (*fpl.Bootstrap, []fpl.Fixture) {
	if through <= 0 {
		return PreSeasonWith(cur, prior, o)
	}
	mustBePlayable(cur)
	b := &fpl.Bootstrap{
		// The season this bootstrap describes, which is what pins the engine's
		// scoring rules — see fpl.Bootstrap.Season and analysis.ScoringRulesFor.
		// FPL's rules have changed inside the archive's span, so an unnamed
		// bootstrap would replay a finished season under today's table.
		Season: cur.Name,
		Teams:  cur.Teams,
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
	}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{
			ID: i, Name: fmt.Sprintf("Gameweek %d", i), Finished: i <= through,
		})
	}

	defcon := DefconScoredIn(cur.Name)
	cutoff := gameweekStart(cur, through+1)
	// Only players the archive shows in the game by `through`. Everyone else is
	// a later registration who could not be bought at this deadline and has no
	// price that existed — see pool.go, which is the whole argument.
	reg := registeredBy(cur, through)
	for _, p := range cur.Players {
		if !reg.has(p.ID) {
			continue
		}
		el := fpl.Element{
			ID: p.ID, Code: p.Code, WebName: p.WebName, ElementType: p.Type,
			Team: p.Team, NowCost: reg.price(p, through), Status: statusAt(p, through+1, cutoff, o),
		}
		var xg, xa, xgc float64
		for gw := 1; gw <= through; gw++ {
			g, ok := p.GWs[gw]
			if !ok {
				continue
			}
			el.Minutes += g.Minutes
			el.Starts += g.Starts
			el.TotalPoints += g.Points
			el.Bonus += g.Bonus
			el.GoalsScored += g.Goals
			el.Assists += g.Assists
			el.CleanSheets += g.CleanSheets
			el.GoalsConceded += g.GoalsConceded
			el.Saves += g.Saves
			el.YellowCards += g.Yellow
			el.RedCards += g.Red
			if defcon {
				el.DefensiveContribution += g.DefCon
			}
			xg += g.XG
			xa += g.XA
			xgc += g.XGC
		}
		el.ExpectedGoals, el.ExpectedAssists, el.ExpectedGoalsConceded =
			fpl.Num(xg), fpl.Num(xa), fpl.Num(xgc)
		if el.Minutes > 0 {
			per90 := 90 / float64(el.Minutes)
			el.ExpectedGoalsPer90 = fpl.Num(xg * per90)
			el.ExpectedAssistsPer90 = fpl.Num(xa * per90)
			el.ExpectedGCPer90 = fpl.Num(xgc * per90)
			el.DefensiveContributionPer90 =
				fpl.Num(float64(el.DefensiveContribution) * per90)
		}
		// The deadline the model is standing at is gameweek `through+1`: everything
		// up to `through` has been played and the squad being chosen is for the next
		// one. That is also the gameweek FPL's chance_of_playing_next_round speaks
		// about, so the two agree by construction rather than by an offset somebody
		// has to remember.
		applyTeamNews(&el, cur.Name, through+1, o)
		b.Elements = append(b.Elements, el)
	}
	sort.Slice(b.Elements, func(i, j int) bool { return b.Elements[i].ID < b.Elements[j].ID })
	if o.Has(OracleOmniscient) {
		applyOmniscience(b, cur, through)
	}
	return b, playedFixtures(cur.Fixtures, through)
}

// statusAt is what FPL would have been showing about a player's availability at
// a given date, as far as the archive can say.
//
// The replay hardcoded "a" for everyone, and this file justified that with "the
// archive carries no availability flags". That was wrong — players_raw.csv has
// status, news and news_added — but the flags are an *end-of-season snapshot*,
// so what can honestly be reconstructed is narrow.
//
// Only a final status of "u" or "i" is carried back, and only from the moment
// its news was posted. Both are states a player is in *at the end of the
// season*, so pairing them with the timestamp says "he was unavailable from
// here to the finish", which is true of a departure or a season-ending injury
// and is the case worth catching: the replay otherwise buys players who never
// appear again, which the backtest output has complained about for as long as
// it has existed.
//
// Deliberately not carried back: "d" (doubtful) and "s" (suspended), which are
// transient and whose end-of-season value says nothing about the rest of the
// year. And nothing at all can be said about an injury that resolved — a player
// out from September to November finishes the season fit and looks fine
// throughout. That is why this does not unblock re-deriving BlendMinutesK,
// whose whole problem is the transient cases.
//
// FPL_NO_AVAILABILITY is **not** an oracle and deliberately stays an
// environment read rather than moving onto the Oracles value: it *blinds* the
// model to a reconstruction it legitimately has, which is the pre-fix baseline,
// and blinding is not hindsight. Putting it on the same value would invite a
// later reader to treat the two as the same kind of switch. It is also read per
// call, which is what lets TestDiagAvailabilityImpact toggle it between cells in
// one process and pair properly.
func statusAt(p *Player, gw int, cutoff time.Time, o Oracles) string {
	if os.Getenv("FPL_NO_AVAILABILITY") != "" {
		return "a"
	}
	// OracleFeatures is applied to the *honest* reconstruction, and has to be:
	// FeatureScope restricts by what the model could have known at the deadline, so
	// the scope test must not read a status the oracle has already rewritten or
	// the restriction would be defined by its own output.
	//
	// Distinct from OracleTeamNews, which replaces the same field with what FPL
	// actually published — recovered from crawled payloads. That one bounds a
	// source; this one bounds the truth, so their difference is what a source
	// better than FPL's own feed could be worth. Validate refuses the pair.
	if o.Has(OracleFeatures) && gw >= o.FeaturesFrom {
		if st := statusAt(p, gw, cutoff, Oracles{}); o.FeatureScope.admits(st) {
			// A blank gameweek is left alone. His club not playing is on the
			// published calendar, so treating it as team news would credit
			// hindsight with a fact the model already has — and the model prices
			// the blank through fixtureLoadFor, not through Status.
			if g, ok := p.GWs[gw]; ok {
				if g.Minutes > 0 {
					return "a"
				}
				return "u"
			}
		}
	}
	// OracleAvailability is hindsight, on purpose and off by default.
	//
	// A player who finished the season with no minutes was, in almost every
	// case, knowable in advance — Kane's move to Bayern was reported for weeks
	// before FPL flagged it on 12 August, the day after the GW1 deadline. The
	// live system has a judgement layer that reads the news and would catch it;
	// the replay has no such thing and AGENTS.md is explicit that it therefore
	// measures the floor the model provides rather than the system's output.
	//
	// So this exists to size that gap, and must not become the default: every
	// figure in AGENTS.md was measured without it, and switching it on silently
	// would inflate all of them and make the record incomparable. Read any
	// number produced with it as an upper bound on what perfect team news is
	// worth, never as the model's score.
	//
	// p.Minutes is the archive's **season total**, so this catches exactly the
	// players who never appear at all — not the far larger population who play
	// until October and then stop. That is the whole of what it may be said to
	// bound; see TestDiagAvailabilityOracle, which sizes both populations.
	if o.Has(OracleAvailability) && p.Minutes == 0 {
		return "u"
	}
	if p.Status != "u" && p.Status != "i" {
		return "a"
	}
	if cutoff.IsZero() || p.NewsAdded == "" {
		return "a"
	}
	added, err := time.Parse(time.RFC3339, p.NewsAdded)
	if err != nil {
		// The archive writes a couple of shapes; a date alone is enough.
		if added, err = time.Parse("2006-01-02", p.NewsAdded[:min(10, len(p.NewsAdded))]); err != nil {
			return "a"
		}
	}
	if added.After(cutoff) {
		return "a"
	}
	return p.Status
}

// gameweekStart is when a gameweek's first match kicked off, used as the date
// the model is standing on. Zero when unknown, which makes statusAt a no-op.
func gameweekStart(s *Season, gw int) time.Time {
	var out time.Time
	for _, f := range s.Fixtures {
		if f.Event == nil || *f.Event != gw || f.KickoffTime == nil {
			continue
		}
		if out.IsZero() || f.KickoffTime.Before(out) {
			out = *f.KickoffTime
		}
	}
	return out
}

// playedFixtures reveals results only for gameweeks already completed.
//
// The archive carries every score for the whole season, so handing them over
// wholesale would let anything reading results — the attack/defence bands, most
// obviously — rate clubs on matches that had not been played. Everything after
// `through` keeps its difficulty rating and loses its scoreline.
func playedFixtures(all []fpl.Fixture, through int) []fpl.Fixture {
	out := make([]fpl.Fixture, 0, len(all))
	for _, f := range all {
		if f.Event != nil && *f.Event <= through {
			f.Finished = f.TeamHScore != nil && f.TeamAScore != nil
		} else {
			f.Finished = false
			f.TeamHScore, f.TeamAScore = nil, nil
		}
		out = append(out, f)
	}
	return out
}

// SimResult is a full season played out week by week.
type SimResult struct {
	Points int

	// XPoints is the same season on the accumulated-xPoints instrument: every
	// gameweek scored by `weekScoreWithChip` from the *identical* eleven, autosubs,
	// armband outcome and chip, with `analysis.XPoints` substituted per
	// player-gameweek, then the hit cost subtracted exactly as `Points` subtracts
	// it. A hit is money actually paid, not a conversion channel, so leaving it in
	// one total and out of the other would make the two metrics answer different
	// questions about the same policy.
	//
	// **Nothing on the shipped scoring path reads it**, and that is the whole
	// design: it is a second reading of a season the replay already played, for
	// sweeps to be judged on. Nothing here decides anything — `decide`, `pickXI`
	// and the optimiser never see it.
	XPoints float64

	Transfers    int
	Hits         int
	HitCost      int // points surrendered to hits
	StartValue   int
	EndValue     int
	OpeningSquad []int
	Moves        []Move
	Weeks        []Week

	// ChipOracle is what a perfectly-timed scoring chip would have been worth,
	// and is nil unless AxisChipWeek is on.
	//
	// Nil rather than a zero struct on purpose: a zero-valued result reads
	// identically to "the oracle ran and found nothing worth playing", which is
	// the silent-no-op shape this package keeps paying for. A caller that finds
	// nil knows the axis was not requested.
	ChipOracle *ChipOracle

	// Armband is the armband oracle's mediator, nil unless AxisArmband is on.
	//
	// Every oracle must declare a quantity it *must* move as well as ones it must
	// not: an oracle that is wired and inert reports a clean null, which is
	// indistinguishable from a real one and is this project's signature failure.
	// For the armband that quantity is how often the hindsight captain differs
	// from the model's, counted here rather than inferred from a points
	// difference — a points difference of zero has two explanations and a change
	// count of zero has one.
	Armband *ArmbandOracle

	// Banking is the transfer-banking mediator, filled by every simulated season
	// rather than by one arm. See BankingMediator.
	//
	// A value and not a pointer, unlike the two oracles above, because it is not
	// conditional: every season runs weekly decisions and every decision holds
	// some allowance, so there is no "the axis was not requested" state to
	// represent. What plays the pointer's role is DecisionWeeks — zero exactly
	// when no week reached `decide`, which is what a SimResult assembled by hand
	// carries and is why bankingOf gates on it.
	Banking BankingMediator

	// FixtureRuns is the fixture-run mediator, filled by every simulated season
	// beside Banking and for the same reason. See FixtureRunMediator.
	FixtureRuns FixtureRunMediator

	// TransferHold, Wildcard, BenchBoost and FreeHit are the four option-value
	// levers' mediators, and ChipPrep the preparation channel's. Values rather
	// than pointers, beside Banking and for its reason: every season runs weekly
	// decisions, so there is no "the axis was not requested" state.
	//
	// ⚠️ **What plays the pointer's role is NOT one field across all five.** For
	// the taper and the preparation credit it is `ConsultedWeeks`, which is zero
	// exactly when the lever was off. For the three CHIP triggers it is
	// `OfferedWeeks`: their `ConsultedWeeks` is also zero when the lever ran and
	// was blocked all season, which `eligible` does whenever a plan already places
	// that chip. An earlier version of this comment asserted the single reading
	// over all five and was wrong for three of them.
	TransferHold TransferHoldMediator
	Wildcard     ChipTriggerMediator
	BenchBoost   ChipTriggerMediator
	FreeHit      ChipTriggerMediator
	ChipPrep     ChipPrepMediator

	// GateFloor is the gate-floor counterfactual: how many proposals the
	// SHIPPED gate would have answered differently, split at the quiet
	// boundary. Counted on every arm, so a floor arm's null is readable
	// against its own count and a baseline arm reads zero.
	GateFloor GateFloorMediator

	// RepairSeries is the held-versus-fresh distance observed every gameweek, and
	// is nil unless SimConfig.RecordRepairCost is set. See RepairWeek.
	//
	// Nil rather than a zero slice for the ChipOracle reason: an empty series
	// reads identically to "the observer ran and every week was priceable at zero
	// changes", which is the silent-no-op shape this package keeps paying for.
	RepairSeries []RepairWeek
}

// RepairWeek is one gameweek's reading of how far the model's optimum sits from
// the squad in hand, on two squads at once.
//
// # What question it exists to settle
//
// `TestDiagWildcardTrigger` measured the repair cost once per cell and found it
// large from the first week it could be taken. Two mechanisms predict that and the
// firing rule cannot separate them, because it fires on first consultation and
// then becomes ineligible — so the cost is never seen as a SERIES on a fixed
// squad:
//
//   - **churn**, a rate. The model has just re-scored everyone, so its preferences
//     move a long way each week and the gap should SHRINK as data accumulates;
//   - **a standing gap**, a level. Any held fifteen differs from a fresh
//     unconstrained argmax over the whole pool at every cutoff, because the argmax
//     is not constrained by what is already owned. Non-zero and roughly FLAT.
//
// The two squads discriminate them. On the frozen fifteen the squad is constant
// while the football is not, so decay accumulates and a rising series is decay. On
// the evolving fifteen the policy is repairing every week, so a persistent
// non-zero level is the standing gap. ⚠️ **They answer different questions and
// must not be pooled.**
//
// # Nothing acts on it
//
// This is an observation, never an input. It is recorded after the point-in-time
// engine is built and before any decision, it is written to the result and read by
// no branch, and `TestTheRepairSeriesChangesNoDecision` pins that by replaying a
// cell with the observer on and off and requiring every point, every transfer and
// every weekly fifteen to be identical. A repair cost that could trigger, transfer
// or rebuild would be the wildcard state trigger again, which is a closed line.
//
// # Pricing, stated rather than assumed
//
// `Budget` and `FrozenBudget` are `wallet.value`, the squad's SELLING value — so
// FPL's half-of-any-rise rule is already charged, exactly as a wildcard would pay
// it. `FrozenGrossBudget` prices the same frozen fifteen at market instead, and
// `FrozenGrossChanges` is the change count that budget buys. The gap between the
// two frozen change counts BOUNDS how much of the frozen series is the
// selling-price friction rather than football — bounds, not subtracts: the two
// pricings also differ in budget size (the gross budget is larger by the
// accumulated tax), so the fresh optimum there solves a larger knapsack, and a
// negative gap means the extra budget upgraded away from the frozen squad. The
// friction itself grows all season on a squad that never sells, which is why the
// bound is needed.
type RepairWeek struct {
	GW int

	// Changes, Free and Cost are the EVOLVING squad — the fifteen the policy
	// actually holds entering this gameweek's decision.
	//
	// Free is the allowance the decision will actually have, with this week's
	// accrual applied, which is what `repairCost` is priced against at the trigger
	// site. Cost is `4 x max(0, Changes - Free)`.
	Changes int
	Free    int
	Cost    float64
	// Budget is the selling value the fresh optimum was given, in tenths.
	Budget int
	// OK is false when the rebuild failed — an empty pool, a budget that cannot be
	// established — which is NO READING rather than a repair cost of zero. The two
	// license opposite conclusions.
	OK bool

	// FrozenChanges, FrozenFree, FrozenCost and FrozenBudget are the same four
	// against the OPENING fifteen, held all season and never sold: the squad the
	// `HOLD` arm scores. FrozenFree is the allowance an arm that never transfers
	// would carry, accruing weekly to the bank limit.
	FrozenChanges int
	FrozenFree    int
	FrozenCost    float64
	FrozenBudget  int
	FrozenOK      bool

	// FrozenGrossChanges is the frozen fifteen re-read at its MARKET value, so the
	// half-of-any-rise selling tax is not charged. FrozenGrossBudget is that
	// budget. See the type comment: the gap between this and FrozenChanges bounds
	// the friction channel — it is not a clean subtraction.
	FrozenGrossChanges int
	FrozenGrossBudget  int
	FrozenGrossOK      bool

	// FreshChurn is the week-to-week movement of the FRESH optimum itself: how
	// many players differ between this week's from-scratch fifteen and last
	// week's. Computed as changesBetween(prevFresh, fresh), which for two
	// fifteen-player sets is symmetric, so the direction of the set difference
	// does not matter. The other columns of RepairWeek measure a distance
	// between a held squad and the optimum; this measures how far the optimum
	// MOVES when one
	// gameweek of data arrives — the direct observable of "one gameweek rewrites
	// the world", which the standing-gap series cannot see.
	//
	// The first observed week has no predecessor, so FreshChurnOK is false there
	// — NO READING rather than zero, the same convention as the OK flags above.
	// The fresh squad is the one already computed for the EVOLVING arm, so the
	// column costs no extra `Optimize` call.
	FreshChurn   int
	FreshChurnOK bool
}

// FixtureRunMediator is what the weekly transfer decision did about fixture
// runs: how often the 3/14/3 band distinction existed at all, how many moves
// were made while it did, and whether those moves went toward the better run.
//
// # Why this is counted rather than inferred
//
// "Do not build a custom fixture-difficulty rating, do not target the worst
// defences" is a closed line, and the mechanism behind it is real: you never buy
// a fixture, you buy a run of them, and runs converge. But every recorded arm in
// that family ran one lever at a time, and this project's standing rule is that a
// one-at-a-time null is a *simple-effect* null — true of the shipped
// configuration and silent about any other. So the tandem case (banking plus chip
// preparation plus the bands) is untested rather than refuted, and it is about to
// be run.
//
// A flat result from that run has at least three explanations:
//
//   - the bands were never computed, because too few fixtures had been played;
//   - they were computed and the policy made no transfers to express them with;
//   - it made transfers and they went nowhere in particular on the bands.
//
// Those license opposite conclusions, and points columns cannot tell them apart.
// The banking funnel that landed beside this made exactly that argument and paid
// for itself immediately: it showed the banking rule had a real choice in 169 of
// 236 weeks and declined every time, which is a completely different statement
// from "nothing ever cleared the bar".
//
// # What it deliberately does NOT contain
//
// There is no "would the decision have differed with the bands off" column. That
// is the counterfactual a reader most wants, and getting it honestly costs a
// second full transfer search every week — the search is the expensive part of a
// replay, and what is affordable to run is the binding constraint on this whole
// enterprise. The decision-level arrival question is answered once, by a test
// (TestTheFixtureRunLeverReachesTheTransferDecision), rather than per week by a
// column. ⚠️ So `RunMoves` is "the move changed band exposure", NOT "the bands
// caused the move". Do not quote it as the second.
type FixtureRunMediator struct {
	// ReadyWeeks is how many of BankingMediator.DecisionWeeks the bands existed
	// in. It shares that denominator deliberately: both mediators are counted on
	// weeks that reached `decide`, so the two funnels are commensurable and a
	// tandem arm can be read across them without a second denominator.
	//
	// It is NOT a configuration fact. The bands need bandMinMatches played by
	// enough clubs, so this is zero for the opening five or six gameweeks of every
	// cell however the lever is set — which is a real constraint on a GW1 cell and
	// was invisible before this column.
	//
	// ⚠️ **It counts weeks the band channel could REACH SCORING, not weeks the
	// ratings existed**, and the two come apart under `FPL_MAGNITUDE`, where the
	// bands compute and `fixtureMultipliersFor` returns before consulting them.
	// See analysis.BandChannelLive: counting readiness alone would report a
	// live-looking mediator off a bypassed lever, which is the precise inversion
	// this block exists to prevent.
	ReadyWeeks int
	// Moves is how many transfers with a RESOLVABLE pair of players were made in
	// those ready weeks. The denominator for RunMoves and WorseMoves, and the
	// column that separates "the policy never acted" from "the policy acted and
	// the bands did not show up in what it chose".
	//
	// ⚠️ It is **not** the season's `moves` column, which counts every transfer in
	// every week including the ones before any rating existed. The two names are
	// one word apart and the denominators are different; on one 2025-26 cell they
	// read 36 and 31.
	Moves int
	// RunMoves and WorseMoves are how many of Moves raised and lowered the
	// incoming player's banded exposure against the outgoing player's. Moves that
	// did neither — both clubs mid-band, which "runs converge" predicts is the
	// modal case — are `Moves - RunMoves - WorseMoves`.
	//
	// ⚠️ **WorseMoves is what gives RunMoves a null**, and it is the reason there
	// are two counts rather than one. With only RunMoves, `Moves - RunMoves` pools
	// "traded the run away" with "the bands had nothing to say about this move",
	// and those license opposite conclusions: 11 of 31 reads as a third against an
	// implied half, where the honest statement may be 11 better, 0 worse and 20
	// ties. Exposure bounds that mixture and does not identify it — 11 positives
	// summing +2 with 20 ties, and 11 summing +14 against 6 summing −12, are the
	// same two numbers.
	//
	// ⚠️ **Counts of MOVES, not of weeks**, unlike every column in the banking
	// funnel. A week can carry several. Read them against Moves, never against
	// ReadyWeeks.
	RunMoves, WorseMoves int
	// Exposure is the signed sum, over Moves, of the incoming player's run less
	// the outgoing player's, in banded fixtures. See analysis.FixtureRun.Net.
	//
	// The magnitude beside the counts: 30 moves that each buy one extra banded
	// fixture and 30 that trade one for one are the same RunMoves-over-Moves ratio
	// and very different interventions. Signed, so a negative sum is a normal
	// reading rather than a fault.
	//
	// ⚠️ **Unweighted.** The model's own band coefficients are deliberately
	// asymmetric on the attacking side — attackBandTarget 0.23 against
	// attackBandAvoid 0.15 — and the clean sheet enters through `exp(-x)`, so a
	// count of target opponents less a count of avoid opponents is not
	// proportional to the adjustment the engine applied on either side. It is a
	// count of fixtures and never a proxy for what those fixtures were worth.
	Exposure int
}

// GateFloorMediator is what the gate-floor counterfactual counted: proposals
// the shipped gate (FreeCost 2.0, MinGain 0.4) would have answered differently
// from the arm's gate, split at quietBoundaryGW. A zero on both halves of a
// floor arm means the floor never changed an answer — the comparison never ran,
// whatever the points say.
type GateFloorMediator struct {
	Le28 int
	Gt28 int
}

// TransferHoldMediator is what the free-transfer taper did: how often it was
// asked, how often it actually moved the charge, and how often the moved charge
// changed the gate's answer.
//
// # Why three counts and not one
//
// The same argument the banking funnel beside it paid for. A taper arm reading
// flat has four explanations, and they license opposite conclusions:
//
//   - the lever was off, so nothing ran;
//   - it ran and the factor was 1 everywhere, so the curve is inert at this
//     setting — which is a statement about the SETTING and not about tapering;
//   - it moved the charge and no gate answer changed, so the charge is not the
//     binding constraint on this policy;
//   - it changed answers and the points did not move, which is the only one of the
//     four that is a result about football.
//
// A points column cannot separate them and this can, at the cost of three integers
// and two sums.
//
// ⚠️ **Flips counts GATE ANSWERS, not weeks and not moves.** A week offering a
// funded pair and a solo swap asks the gate three times, so this can exceed
// `ConsultedWeeks`. Read it as a rate over `GateCalls`, which is in the block for
// exactly that reason.
//
// ⚠️ **A flip is not a changed transfer.** The gate is asked about candidates in
// order and `decide` returns on a refusal, so a flip on the first call changes the
// week and a flip on a later one may not. Counting the changed transfers honestly
// would need a second search, which is what this project cannot afford per week.
type TransferHoldMediator struct {
	// ConsultedWeeks is how many decision weeks the taper was asked in. Counted
	// rather than read off the switch, so the mediator says what ran.
	ConsultedWeeks int
	// PricedWeeks is how many of those the factor differed from 1.
	//
	// ⚠️ **It equals ConsultedWeeks by construction and is a TRIPWIRE, not a
	// reading.** With a usable window the normalised decay is 1 at exactly one
	// point in the season and different everywhere else, so a live taper prices
	// every consulted week. It therefore cannot separate "the curve is inert at
	// this setting" — the column that can is `ftv_mean_charge` read against
	// `free_transfer_value`, which is why both are in the block. What this
	// catches is the factor arriving as a literal 1, which is what a deleted or
	// short-circuited join looks like.
	PricedWeeks int
	// GateCalls is how many times the gate was asked in consulted weeks, and Flips
	// how many of those the shipped charge would have answered differently.
	//
	// The counterfactual is evaluated through `gateDecision`, which is pure, and
	// NOT through `acceptTransfer`, which logs. Under a gate oracle `gateDecision`
	// ignores the charge entirely, so Flips is necessarily 0 there — a true
	// reading, and the reason the two are counted rather than assumed.
	GateCalls, Flips int
	// ChargeSum and LoadSum are the applied charge and the squad's forward fixture
	// density, summed over ConsultedWeeks. Sums rather than means so a cell can be
	// re-pooled downstream; the division happens in MeanCharge and MeanLoad alone.
	ChargeSum, LoadSum float64
}

// MeanCharge is the average free-transfer charge a consulted week ran with, and
// MeanLoad the average forward fixture density it was read from.
//
// Zero when nothing was consulted. A caller needing to tell that apart from a
// genuine zero reads ConsultedWeeks, exactly as bankingOf does for the banking
// funnel.
func (m TransferHoldMediator) MeanCharge() float64 {
	if m.ConsultedWeeks <= 0 {
		return 0
	}
	return m.ChargeSum / float64(m.ConsultedWeeks)
}

// MeanLoad is the average forward fixture density. See MeanCharge.
func (m TransferHoldMediator) MeanLoad() float64 {
	if m.ConsultedWeeks <= 0 {
		return 0
	}
	return m.LoadSum / float64(m.ConsultedWeeks)
}

// ChipTriggerMediator is what one chip's state rule did: how often it was
// consulted, how often it had a real reading to weigh, and the week it fired in.
//
// # Chip preparation had NO mediator at all, and this closes that
//
// `PrepareBenchBoost` and `PrepareTripleCaptain` have shipped, been swept and been
// crossed in a 2x2 with nothing counting whether the credit was ever non-zero. So
// every one of those readings is a points column with no funnel behind it, and
// "the credit never fired" and "it fired and bought nothing" are one number. The
// preparation channel gets its own counters here beside the trigger's.
//
// ⚠️ **FiredGW is the whole deliverable for the wildcard.** The replay cannot
// value a wildcard — it replaces all fifteen and the within-season spread swamps
// it — so the question this lever answers is a decision count: does it fire, and
// when. The recorded closure of the wildcard-trigger line rests on the tested
// trigger firing at GW2, when the model has least data, so the falsifier for a
// REPAIR-COST trigger is whether it does the same. A cells file carrying FiredGW
// answers that; a points column never could.
//
// ⚠️ **BarSum is over ConsultedWeeks and is NOT a mean of the bar at the firing
// week.** A bar that decays is a different number every week, so a single figure
// for "the bar" does not exist; the sum plus the count is what a reader can
// reconstruct a trajectory's level from, and the firing week's own bar is
// FiredBar.
type ChipTriggerMediator struct {
	// OfferedWeeks is how many decision weeks the rule was OFFERED — before its
	// own eligibility guards, so it counts weeks the lever was switched on
	// regardless of whether anything let it act.
	//
	// ⚠️ **It exists because ConsultedWeeks == 0 does NOT mean "the lever was
	// off".** `eligible` refuses the whole season when a plan already places that
	// chip, so a 2x2 crossing calendar placement with a trigger reads an all-zero
	// funnel in the planned corner — a lever that RAN and correctly declined,
	// wearing the exact clothes of one that was never wired. Those license
	// opposite conclusions and the funnel could not tell them apart.
	//
	// So the readings are: `offered = 0` is the lever off; `offered > 0` with
	// `consulted = 0` is the lever on and blocked all season, which today means a
	// plan owns the chip; `consulted > 0` with `weighed = 0` is the rule reaching
	// its bar with no reading to weigh.
	OfferedWeeks int
	// ConsultedWeeks is how many of OfferedWeeks passed the eligibility guards and
	// the rule was actually asked in.
	ConsultedWeeks int
	// WeighedWeeks is how many of those produced a finite reading to compare
	// against the bar. It separates "the rule never had a number" from "it had one
	// and the bar refused it", which is the banking funnel's WeighedWeeks argument
	// applied to a chip.
	WeighedWeeks int
	// FiredGW is the gameweek the chip was played in, or 0 if the rule never
	// fired. FiredValue is the reading that cleared, and FiredBar the bar it
	// cleared — both zero when FiredGW is zero.
	FiredGW    int
	FiredValue float64
	FiredBar   float64
	// ValueSum and BarSum are the reading and the bar summed over WeighedWeeks,
	// so a cell carries the level as well as the count.
	ValueSum, BarSum float64
}

// ChipPrepMediator is what the chip-preparation credit did, which nothing counted
// until now. See ChipTriggerMediator for the argument.
//
// ⚠️ **CreditWeeks counts weeks the credit was NON-ZERO**, which is the mediator
// question — did the lever reach the decision — and not weeks it was switched on.
// `chipCreditFor` returns zero whenever no prepared chip falls inside the horizon,
// which is most of a season even with the switch set, so the two differ by a lot.
type ChipPrepMediator struct {
	// ConsultedWeeks is decision weeks a preparation switch was on in.
	ConsultedWeeks int
	// CreditWeeks is how many of those carried a non-zero credit.
	CreditWeeks int
	// BenchSum and CaptainSum are the credits summed over CreditWeeks.
	//
	// ⚠️ **They are NOT points, and multiplying by the horizon does not recover
	// a point value.** `ChipCreditAt` sets `Bench = 1/horizon` — a dimensionless
	// FRACTION OF THE HORIZON the chip falls in, which the objective multiplies by
	// a squad's own bench value downstream. So `BenchSum x horizon` recovers
	// `CreditWeeks` and nothing more, and at a fixed horizon these two columns
	// carry no information the count beside them does not.
	//
	// They are kept because the horizon is NOT fixed — `effectiveHorizon` shortens
	// it at the end of a season and `anticipate` shortens it before a chip — so a
	// sum that diverges from `CreditWeeks/horizon` says the window moved, which is
	// a real fact about the arm. Read them as a horizon audit, never as a level.
	// The credit's point value is not recorded anywhere and would need the squad
	// it was applied to.
	BenchSum, CaptainSum float64
}

// ⚠️ **These columns describe ONE ARM's own path, and a difference between two
// arms is a POLICY-metric difference like any other.**
//
// The first thing a tandem sweep invites is subtracting these columns between a
// band-on and a band-off arm. That difference carries the full transfer-path
// divergence — the two arms stop holding the same squad after their first
// disagreement — with no pairing at the move level, no standard error, and of
// order thirty observations a cell. It needs a threshold exactly as a points
// column does, and this project's recorded floor for the transfer path is 303
// points of spread.
//
// One 2025-26 cell shows the shape: RunMoves went 11 -> 9 while Exposure went
// +2 -> +9, two counts moving in opposite directions. That is what n around 31
// on two divergent paths looks like, and it is not a finding.

// ArmbandOracle counts how often hindsight disagreed with the model about who
// should wear the armband.
type ArmbandOracle struct {
	Weeks   int // gameweeks the armband was chosen in
	Changed int // of those, how many the oracle captained differently
}

// BankingMediator is what the weekly transfer decision did with its allowance:
// how often the banking rule declined a move, and how much allowance it had to
// decline with.
//
// # Why this is counted rather than inferred
//
// The recorded verdict is that the policy never banks a transfer and that
// banking is not the fix. Nothing counted whether shouldBank ever fired, so that
// null could not be told apart from a comparison that never ran — the arm wired,
// the rule unreachable, and a byte-identical result reported as a tie. This
// package's standing rule is that a byte-identical result is not a tie until its
// mediator has been checked, and the ArmbandOracle field above states the same
// argument in its own terms: a points difference of zero has two explanations
// and a count of zero has one.
//
// FreeHeld is here for the other half of the reading. shouldBank returns false
// outright once the allowance is already at cfg.BankUpTo, so an arm whose
// decisions all run at the ceiling would report zero banked weeks for a reason
// that has nothing to do with the comparison being made. That reason is only
// visible with the allowance recorded beside the count.
type BankingMediator struct {
	// DecisionWeeks is how many gameweeks reached `decide`: every week played
	// except the first, which buys the squad, and any week a wildcard or a free
	// hit took the transfer decision away.
	DecisionWeeks int
	// ConsultedWeeks is how many of those weeks shouldBank was actually asked
	// in. Counted rather than read off cfg.BankLookahead so that the mediator
	// says what ran instead of what was configured — the same rule the cells
	// file's arm block is built on.
	ConsultedWeeks int
	// WeighedWeeks is how many of ConsultedWeeks the rule had a real choice to
	// weigh: it got past both early guards and at least one of the two arms was
	// worth something.
	//
	// # Why a zero is otherwise a mixture
	//
	// shouldBank has three false exits, not two. The allowance already sitting
	// at cfg.BankUpTo and a horizon of one gameweek are guards, and the third is
	// simply `later > now` coming out false — which INCLUDES the degenerate case
	// where both are zero because nothing cleared MinGain in either week. That is
	// not a corner: this record already holds that nothing clears the gain
	// threshold at any price after GW28, so a large share of a GW1 cell's late
	// weeks are expected to reach it.
	//
	// Without this count, `banked_weeks = 0` pools "the rule weighed a real
	// choice and preferred acting now" with "there was nothing to weigh", and
	// those license opposite conclusions — the first is a verdict on the banking
	// rule and the second is closer to a comparison that could not run. Both
	// numbers are already computed inside shouldBank, so separating them costs a
	// counter and recovering them later costs a full sweep.
	WeighedWeeks int
	// BankedWeeks is how many of WeighedWeeks it answered "wait" in.
	//
	// ⚠️ **A count, not a rate.** Cells run 38, 33, 28, 23, 18 and 13 gameweeks,
	// so pooling this across entry points without DecisionWeeks weights the
	// earliest regime nearly three times as heavily — the standing conversion
	// rule, arriving through a mediator instead of through points.
	BankedWeeks int
	// FreeHeld is the sum, over DecisionWeeks, of the free transfers in hand
	// when the search ran — after the week's accrual and before anything was
	// spent, which is the number moveLimit is taken from and the number
	// shouldBank compares against cfg.BankUpTo.
	//
	// A sum rather than a mean so the cell that carries it can be re-pooled
	// downstream; MeanFreeAtDecision is the one place the division happens.
	// ⚠️ This is NOT Week.Free, which is recorded after the decision has spent
	// what it spent. Both are wanted and they are different quantities.
	FreeHeld int
}

// MeanFreeAtDecision is the average transfer allowance a decision week ran with,
// and the sole definition of the free_at_decision column.
//
// Zero when nothing was decided. A caller that needs to tell that apart from a
// genuine mean of zero — which cannot occur, since the accrual runs before the
// search — reads DecisionWeeks, and bankingOf does exactly that.
func (b BankingMediator) MeanFreeAtDecision() float64 {
	if b.DecisionWeeks <= 0 {
		return 0
	}
	return float64(b.FreeHeld) / float64(b.DecisionWeeks)
}

// weekBanking is one gameweek's contribution to BankingMediator: what the search
// ran with, whether the banking rule was asked, whether it had anything to weigh,
// and whether it said wait.
type weekBanking struct {
	Free      int
	Consulted bool
	Weighed   bool
	Banked    bool
}

// weekHold is one gameweek's contribution to TransferHoldMediator and to
// ChipPrepMediator: what the taper priced, whether the price moved a gate answer,
// and what the preparation credit was worth.
//
// The two mediators share one per-week struct because both are read off the same
// `decide` call and neither is worth a second return value. They are separate
// mediators downstream because they are separate levers.
//
// ⚠️ **GateCalls and Flips count the BESPOKE search only.** `unifiedDecide` has
// its own accept expressions and receives the tapered charge but is not counted
// here, so a unified arm would report a live taper with zero gate calls. Unified
// does not ship and `Simulate` already refuses it alongside the preparation
// switches; recorded so nobody reads a zero there as inertness.
type weekHold struct {
	Consulted bool
	Load      float64
	Factor    float64
	Charge    float64
	GateCalls int
	Flips     int
	// FloorFlipsLe28 and FloorFlipsGt28 are the gate-floor counterfactual, split
	// at the recorded quiet boundary (GW28 — nothing clears the gain threshold
	// at any price after it). Unlike the taper's flips these are counted on
	// EVERY arm and every proposal: the counterfactual asks whether the SHIPPED
	// gate would have answered differently, so a floor arm's flips are where the
	// floor actually admitted or refused something.
	FloorFlipsLe28 int
	FloorFlipsGt28 int
	Credit         analysis.ChipCredit
}

// ChipWeek is one chip placed with hindsight: which gameweek, and what it would
// have returned there.
//
// Deliberately only these two fields. The median week and the threshold rule the
// chip diagnostic compares against are *not* oracles — they are ordinary readings
// of Week.BenchBoostGain — and computing them here would put a baseline inside the
// hindsight arm, where a later reader could quote it as one.
type ChipWeek struct {
	GW   int
	Gain int
}

// ChipOracle is the AxisChipWeek result: the best week for each scoring chip on
// the squad the season actually held.
type ChipOracle struct {
	BenchBoost    ChipWeek
	TripleCaptain ChipWeek
}

// Week is one gameweek as it played out, for a week-by-week report.
type Week struct {
	GW int
	// Gross is what the eleven scored; Net subtracts that week's hits.
	Gross, Net int
	// GrossXP is Gross on the accumulated-xPoints instrument — the same eleven,
	// the same autosubs, the same armband outcome, the same chip, scored from one
	// `weekScoreWithChip` call. There is no NetXP, because the hit cost is charged
	// once at the season total exactly as it is for Points.
	//
	// Recorded per week for the reason the captaincy rungs' per-gameweek detail is:
	// a concentrated result is only judgeable by looking at what the squad did in
	// the cells carrying it, and the alternative is a diagnostic keeping its own
	// copy of this loop — this package's most-repeated bug, and AGENTS.md names a
	// diagnostic as the worst place for it. Nothing on the scoring path reads it.
	GrossXP float64
	HitCost int
	Captain string
	// CaptainPts is the captain's own return, doubled — the single biggest
	// swing in a gameweek and the decision most worth reviewing separately.
	CaptainPts int
	Transfers  int
	Value      int // squad value in tenths
	Bank       int
	// XI is who actually started, so a report can tell rotation from transfer
	// churn — two very different reasons for the eleven to change.
	XI []int
	// Sell is what each held player would actually raise this week, after FPL's
	// half-of-any-rise rule. Recorded because the difference between this and
	// the market price *is* the thing a team-value question is about, and it
	// cannot be reconstructed afterwards without replaying every purchase.
	Sell map[int]int
	// Free is how many free transfers survived this week's decision — the
	// allowance carried into next week, BEFORE its accrual.
	//
	// ⚠️ **This said "in hand when this week's decision was made" and that was
	// wrong.** It is assigned after `decide` has already spent, so a week that
	// made two moves reports what was left rather than what the search had.
	// Reading it as the decision's allowance under-states it by every transfer
	// spent, and TestDiagBanking's histogram was drawn on exactly that mistake:
	// measured on 2025-26 from GW1 it means 0.55 here against 1.46 for the
	// quantity the label named — a factor of 2.6, and it folds in the opening
	// week, which makes no decision at all.
	//
	// The allowance the search actually ran with is
	// BankingMediator.FreeAtDecision, captured beside `moveLimit`. Both are
	// wanted and they are different numbers: this one says what a manager carries
	// into next week, that one says what this week's search could spend.
	Free int
	// Squad is the whole fifteen held that week, after any transfers. Recorded
	// so a diagnostic can re-ask "what would a different policy have done from
	// here?" at a common state, which is the only way to compare two decision
	// rules without their paths diverging and confounding everything after the
	// first disagreement.
	Squad []int
	// BenchBoostGain and TripleCaptainGain are what each chip would have been
	// worth *this* week, given the squad actually held and the eleven actually
	// fielded. They are recorded every week and played none, so they cost
	// nothing and describe the ceiling: the best a perfectly-timed chip could
	// have done. A policy that has to choose in advance cannot reach it.
	BenchBoostGain    int
	TripleCaptainGain int

	// Which chip was actually played this week, if any.
	Wildcard      bool
	FreeHit       bool
	BenchBoost    bool
	TripleCaptain bool
}

// Move records one transfer the policy chose to make.
type Move struct {
	GW       int
	Out, In  string
	Gain     float64
	Hit      bool
	OutScore float64
	InScore  float64

	// OutID and InID are element ids, kept so a report can score the move
	// against what the two players actually went on to do. The modelled gain is
	// what the policy believed; only these say whether it was right.
	OutID, InID int
}

// Verdict is what a move actually returned, against what it was predicted to.
//
// A transfer is judged over the horizon it was justified on, not the rest of the
// season: the policy re-decides every week, so a move made at GW8 is not a
// commitment to hold until May.
type Verdict struct {
	Move
	// InPoints and OutPoints are what each player scored in the `Weeks`
	// gameweeks from the transfer onward, whether or not he was picked.
	InPoints, OutPoints int
	Weeks               int
	// OutPlayed is whether the sold player appeared at all over the window.
	//
	// When he did not, Net() overstates the transfer: it assumes he would have
	// sat in the eleven scoring nothing, when an autosub would have covered him
	// for free. Across three replayed seasons this is 19% of transfers, and it
	// accounts for most of the gap between modelled and actual gains.
	OutPlayed bool
}

// Net is the points the move actually earned, after the hit if one was taken.
func (v Verdict) Net() int {
	n := v.InPoints - v.OutPoints
	if v.Hit {
		n -= 4
	}
	return n
}

// Judge scores every move against what the two players went on to do over the
// horizon the decision was made on.
func Judge(s *Season, moves []Move, horizon int) []Verdict {
	out := make([]Verdict, 0, len(moves))
	for _, mv := range moves {
		weeks := int(effectiveHorizon(horizon, mv.GW))
		out = append(out, Verdict{
			Move:      mv,
			InPoints:  pointsOver(s, mv.InID, mv.GW, weeks),
			OutPoints: pointsOver(s, mv.OutID, mv.GW, weeks),
			Weeks:     weeks,
			OutPlayed: minutesOver(s, mv.OutID, mv.GW, weeks) > 0,
		})
	}
	return out
}

// pointsOver totals a player's actual returns across a window of gameweeks.
func pointsOver(s *Season, id, from, weeks int) int {
	p := s.Players[id]
	if p == nil {
		return 0
	}
	var n int
	for gw := from; gw < from+weeks && gw <= 38; gw++ {
		n += p.GWs[gw].Points
	}
	return n
}

// xPointsOver totals a player's EXPECTED points from realised underlying across a
// window of gameweeks — the same window and the same bounds as pointsOver, with the
// conversion residual removed.
//
// The pair is deliberate. pointsOver asks "what did this decision return"; this asks
// "what were the chances it bought worth". They differ only on the four channels
// analysis.XPointsResidual replaces, so any gap between two answers is that
// substitution and nothing else — same player, same weeks, same right-censoring at
// GW38.
//
// Returns float64 where pointsOver returns int, because an expectation is not a
// score. Rounding it to an int here would quantise away most of what the instrument
// exists to measure, on a quantity whose whole claim is that it is smoother.
func xPointsOver(s *Season, id, from, weeks int) float64 {
	p := s.Players[id]
	if p == nil {
		return 0
	}
	var n float64
	for gw := from; gw < from+weeks && gw <= 38; gw++ {
		n += xPointsOf(p, p.GWs[gw])
	}
	return n
}

// xPointsOf is the ONE mapping from an archive row to the instrument's input.
//
// It is a function rather than eleven field assignments repeated per caller
// because `analysis.XPointsGW` is a parameter object precisely to make a
// transposition a compile error *within* a call — and that protection is worth
// nothing if two call sites each write their own literal, since a caller that
// swapped `Goals` for `Assists` in one of them would produce a plausible number
// in one metric and not the other. The accumulated-xPoints instrument added the
// second caller (`playerWeek`, on the week-scoring path), so the mapping moved
// here rather than being copied.
func xPointsOf(p *Player, g GW) float64 {
	return analysis.XPoints(analysis.XPointsGW{
		Position:      p.Type,
		Fixtures:      g.Fixtures,
		Minutes:       g.Minutes,
		Points:        g.Points,
		Goals:         g.Goals,
		Assists:       g.Assists,
		CleanSheets:   g.CleanSheets,
		GoalsConceded: g.GoalsConceded,
		XG:            g.XG,
		XA:            g.XA,
		XGC:           g.XGC,
	}, p.Conversion, p.Rules)
}

// minutesOver totals a player's minutes across a window of gameweeks.
func minutesOver(s *Season, id, from, weeks int) int {
	p := s.Players[id]
	if p == nil {
		return 0
	}
	var n int
	for gw := from; gw < from+weeks && gw <= 38; gw++ {
		n += p.GWs[gw].Minutes
	}
	return n
}

// recentIndex is a recency-weighted view of the season so far, built from the
// archive's per-gameweek rows.
type recentIndex struct {
	byCode map[int]analysis.RecentPlayer
}

func (r recentIndex) Get(code int) (analysis.RecentPlayer, bool) {
	p, ok := r.byCode[code]
	return p, ok
}

// newRecentIndex weights each played gameweek by halfLife, counting back from
// `through`. A half-life of zero disables the weighting entirely.
func newRecentIndex(s *Season, through int, halfLife float64) analysis.RecentForm {
	return newRecentIndexWith(s, through, halfLife, 0)
}

// newRecentIndexWith also weights output rates, on their own half-life. Rates
// and minutes are deliberately separate knobs — see analysis.RateHalfLife.
func newRecentIndexWith(s *Season, through int, halfLife, rateHalfLife float64) analysis.RecentForm {
	if halfLife <= 0 && rateHalfLife <= 0 {
		return nil
	}
	if through < 1 {
		return nil
	}
	if halfLife <= 0 {
		halfLife = 1e9 // effectively flat, so minutes are untouched
	}
	out := recentIndex{byCode: map[int]analysis.RecentPlayer{}}
	for _, p := range s.Players {
		if p.Code == 0 {
			continue
		}
		var mins, starts, den float64
		var n int
		// Rates are weighted by minutes as well as recency: a rate is evidence
		// in proportion to the football behind it, so a weighted 90 minutes
		// counts for more than a weighted cameo.
		var rw, xg, xa, xgc, dc, bonus, saves float64
		for gw := 1; gw <= through; gw++ {
			g, ok := p.GWs[gw]
			if !ok {
				continue
			}
			w := math.Pow(0.5, float64(through-gw)/halfLife)
			// Per *match*, not per gameweek. A double gameweek now correctly
			// records 180 minutes, and dividing that by one week would predict
			// 180 for the single gameweeks that follow. Weighting the
			// denominator by the fixture count keeps this a statement about how
			// much of a match he plays, which is what everything downstream —
			// MinutesRating, the sixty-minute threshold, the blend — assumes.
			fx := float64(g.Fixtures)
			if fx < 1 {
				fx = 1
			}
			mins += float64(g.Minutes) * w
			starts += float64(g.Starts) * w
			den += w * fx
			n++

			if rateHalfLife > 0 && g.Minutes > 0 {
				rwk := math.Pow(0.5, float64(through-gw)/rateHalfLife) * float64(g.Minutes) / 90
				rw += rwk
				per := func(v float64) float64 { return v / (float64(g.Minutes) / 90) }
				xg += per(g.XG) * rwk
				xa += per(g.XA) * rwk
				xgc += per(g.XGC) * rwk
				dc += per(float64(g.DefCon)) * rwk
				bonus += per(float64(g.Bonus)) * rwk
				saves += per(float64(g.Saves)) * rwk
			}
		}
		if den == 0 || n == 0 {
			continue
		}
		// Consecutive blanks ending at the cutoff. Only gameweeks his club
		// actually played count: a missing row is a blank gameweek for the club
		// rather than an absence for the player, and conflating the two would
		// flag everyone at a blanking club as injured.
		run := 0
		for gw := through; gw >= 1; gw-- {
			g, ok := p.GWs[gw]
			if !ok {
				continue
			}
			if g.Minutes > 0 {
				break
			}
			run++
		}
		rp := analysis.RecentPlayer{
			MinutesPerMatch: mins / den,
			StartShare:      starts / den,
			Matches:         n,
			BlankRun:        run,
		}
		if rw > 0 {
			rp.Minutes90 = rw
			rp.XG90, rp.XA90, rp.XGC90 = xg/rw, xa/rw, xgc/rw
			rp.DefCon90, rp.Bonus90, rp.Saves90 = dc/rw, bonus/rw, saves/rw
		}
		out.byCode[p.Code] = rp
	}
	return out
}

// minutesHalfLife is the recency setting, taken from the model weights unless
// the replay overrides it.
func (c SimConfig) minutesHalfLife() float64 {
	if c.MinutesHalfLife != 0 {
		return c.MinutesHalfLife
	}
	return c.Weights.MinutesHalfLife
}

// openingBenchWeight is what the opening fifteen credits a bench player at.
//
// It was hardcoded at 0.02 — near enough zero, which is what FPL literally pays
// for a player who does not appear. That is only true if he never appears, and
// autosubs, rotation and injuries mean he does: in the 2024-25 replay the four
// bench slots returned 419 real points.
//
// The misspecification was harmless while the optimiser was too weak to exploit
// it, and stopped being harmless the moment the multi-downgrade funding phase
// landed. Given a correct search, "bench quality is nearly free to sell" is an
// instruction, and it followed it — gutting a 419-point bench down to 75 to buy
// 0.38 of modelled XI, at a cost of 116 real points.
// resolvedMinExpectedMinutes is the opening squad's pool floor as the build
// actually applies it: zero takes the historical 55, and **negative** means no
// floor, since zero is already spoken for and Optimize reads zero as "unset".
//
// ⚠️ This is the switch that decides which arm is the *shipped* one, so a second
// copy of it is worse than a second copy of the predicate was. Review found a
// third copy in TestDiagFloorPopulation and named the failure exactly: if the
// default moved to 60, a probe carrying its own switch would keep building the
// 55 arm, keep printing a clean result, and read as a perfect reproduction. That
// is the byte-identical-null signature with the intervention silently unchanged.
// Every reader of the floor goes through here.
func (c SimConfig) resolvedMinExpectedMinutes() float64 {
	switch {
	case c.MinExpectedMinutes == 0:
		return 55
	case c.MinExpectedMinutes < 0:
		return 0 // Optimize reads zero as "no floor"
	}
	return c.MinExpectedMinutes
}

func (c SimConfig) openingBenchWeight() float64 {
	if c.BenchWeight > 0 {
		return c.BenchWeight
	}
	return analysis.DefaultBenchWeight
}

// SimConfig is the review policy the replay applies each week.
type SimConfig struct {
	// BenchWeight is what the opening squad credits a bench player at. Zero takes
	// analysis.DefaultBenchWeight, the measured value — *not* the historical 0.02,
	// which this comment claimed for as long as the default was 0.02 and never
	// stopped claiming after it moved. See openingBenchWeight.
	BenchWeight float64

	Weights    analysis.Weights
	MinGain    float64 // pts/GW below which a free transfer is not worth spending
	MinGainHit float64 // net points across the horizon needed to justify a -4
	BankUpTo   int     // free transfers may accumulate to this many
	MaxHits    int     // hits per week
	// HitCeiling is the largest value MaxHits can MEAN. Zero takes
	// analysis.DefaultHitCeiling, which is 1 and is what ships.
	//
	// The two are separate knobs because they answer different questions and
	// because collapsing them is what hid the defect. MaxHits is how many hits
	// this arm is willing to spend; HitCeiling is how many the search may
	// express at all, and it clamped MaxHits **unconditionally** — so an arm at
	// `MaxHits: 2` replayed byte-identically to shipped and read as a null.
	//
	// ⚠️ Raising it changes the funded-pair branch as well as the limit: the
	// branch gated on `hitsNeeded <= 1` by a literal, which is the second half of
	// the same clamp. Both now read this field, so an arm cannot raise one and
	// leave the other.
	HitCeiling int
	// MaxMoves caps transfers in a single week, free and hit combined. Zero
	// means the natural limit: every free transfer plus the hit allowance.
	MaxMoves int
	Budget   int
	// MaxFundingSales caps how many sales may fund one upgrade. Zero means the
	// transfer budget allows.
	MaxFundingSales int
	// MinutesHalfLife weights recent gameweeks more heavily when estimating
	// minutes. Zero takes it from Weights.
	MinutesHalfLife float64
	// OlderPriors are seasons before the immediate prior, most recent first,
	// blended in by PriorHalfLife. Zero-length uses the immediate prior alone.
	OlderPriors   []*Season
	PriorHalfLife float64

	// UnifiedGainPerDecision applies MinGain to a whole k-move package rather
	// than to each move, which is what the unified search originally did. It is
	// kept only so the difference can be measured; see unified.go.
	UnifiedGainPerDecision bool
	// UnifiedSurcharge escalates the per-move charge with move count, on top of
	// the per-move gain threshold. Zero is flat per-move pricing.
	UnifiedSurcharge float64

	// PriorMinutesHalfLife and PriorRateHalfLife weight last season's closing
	// gameweeks more heavily than its opening ones, in gameweeks. Zero leaves
	// that half of the prior flat. See newPriorIndexRecent.
	PriorMinutesHalfLife float64
	PriorRateHalfLife    float64

	// SquadFixtureWeight overrides FixtureWeight for the *opening squad only*,
	// leaving the weekly transfer decision on the configured value. Negative
	// means "leave it alone", because zero is a real setting meaning "ignore
	// fixtures entirely when building the fifteen" — the same trap
	// BonusPriorWeight documents.
	SquadFixtureWeight float64
	// SquadFixtureWeightSet distinguishes an explicit zero from an absent value.
	SquadFixtureWeightSet bool

	// BankLookahead lets the policy decline a move this week when a larger
	// package next week is worth more. See shouldBank.
	BankLookahead bool

	// WeeklyXI picks the eleven on the imminent gameweek rather than the horizon
	// average. Only interesting alongside fixture-load scaling, where it lets a
	// double gameweek decide who starts.
	WeeklyXI bool

	// Unified runs the bounded-revision search in place of the bespoke
	// pair-then-singles policy, without needing the env var. See unified.go.
	Unified bool
	// UnifiedPoolFloor is the candidate floor the unified search uses. Zero
	// takes its historical 55; negative removes it, which is what the bespoke
	// search effectively does since it ranks over the whole pool.
	UnifiedPoolFloor float64

	// MinExpectedMinutes is the squad pool's rotation-risk cliff, in minutes per
	// gameweek. Zero takes the historical 55; **negative means no floor at
	// all**, since zero is already spoken for and Optimize reads zero as
	// "unset" too.
	//
	// It filters the *opening squad only*: the weekly transfer search ranks over
	// e.AllMetrics() with no floor at all, so this is a squad-selection constant
	// and belongs on the hold metric.
	MinExpectedMinutes float64

	// DecisionHorizon is how many gameweeks a transfer is given to repay its
	// cost. Zero takes it from Weights.Horizon.
	//
	// It is separate from the fixture window because Horizon does two jobs at
	// once: how far ahead the fixture average looks, and how generous the
	// transfer threshold is. The gate is gain x horizon >= charge, so a longer
	// horizon inflates every gain and pushes marginal moves past the bar —
	// transfer count runs from 55 at horizon 2 to 109 at horizon 8. Sweeping
	// Horizon alone therefore measures both effects mixed together.
	DecisionHorizon int

	// OracleWindow is how many gameweeks ahead an information oracle over the
	// future tells the truth about. Zero takes it from DecisionHorizon.
	//
	// It exists for one measurement rather than as a setting. The first minutes
	// oracle averaged the *whole remainder of the season* and divided by the
	// player's own gameweek rows rather than his club's fixtures, and both
	// defects were corrected in the same commit — so the drop from its recorded
	// figure has two candidate causes and the write-up could only assert one. An
	// arm at window 38 with the corrected denominator isolates them. See
	// SimConfig.oracleWindow and minutesoracle.go.
	OracleWindow int

	// AnticipateChips lets the weekly transfer decision know a chip is coming.
	//
	// # It is a wiring switch, not a model, and that is the finding
	//
	// `Engine.Chips` is never set anywhere in this package. The replay reads
	// SimConfig.Chips only to decide which week to *play* a chip in, so
	// `EffectiveHorizon`, `SuggestBenchWeight`, `ApplyChipPlan` and
	// `ApplyFreeHitToScoring` — all of which exist in internal/analysis and are
	// exercised by the live agent — have been dead in every replayed season.
	//
	// So the record's "the policy plays identically whether or not it holds a
	// wildcard: it never takes a position it could not unwind, because it does not
	// know it will be able to" is not a modelling gap. It is an unwired function,
	// and every chip figure in the record was measured by a blind policy.
	//
	// # What it can and cannot carry
	//
	// **Carries the free exit.** A wildcard at GW n means the current squad only
	// has to serve to n-1, so a short fixture run stops being a bet the policy has
	// to unwind through the gate. That is `EffectiveHorizon`.
	//
	// ⚠️ This said "a wildcard *or free hit*" until `EffectiveHorizon` was
	// corrected. A free hit fields a temporary fifteen for one gameweek and hands
	// the permanent squad back, so it removes one week rather than ending the
	// squad — which is the next bullet's job, not this one. The two bullets were
	// double-counting the same chip.
	//
	// **Carries free-hit avoidance.** The free-hit week is excluded from scoring
	// entirely, so a promising run containing one dreadful week becomes buyable.
	// That is `ApplyFreeHitToScoring`.
	//
	// **Cannot carry the bench boost, and could not express the triple captain at
	// all.** `XIValue` — the transfer objective — took a squad and nothing else,
	// crediting bench players at zero because that is what FPL pays in an ordinary
	// week, so `SuggestBenchWeight` had nowhere to go on this path and reached only
	// the opening squad through `OptimizeRequest.BenchWeight`. Stated here rather
	// than left as an inert third mechanism inside an arm that claims three.
	//
	// `PrepareBenchBoost` and `PrepareTripleCaptain` are the two channels that hole
	// left unmeasured. They are deliberately *not* folded into this switch: this
	// one is about what the squad must survive to, and those are about what it is
	// worth when it gets there.
	//
	// The transfer *gate* horizon is deliberately not shortened with it. A move
	// made two weeks before a wildcard really does have two weeks to repay, so
	// there is a second, opposite lever here — and folding both into one arm would
	// measure their sum and neither.
	AnticipateChips bool

	// AnticipateGate shortens the transfer gate's horizon in step with the
	// scoring one, so the arm is internally consistent.
	//
	// The comment above says the two levers are opposite and that folding them
	// into one arm would measure their sum. That is true of *this* knob against
	// the scoring one, and it is not a reason to leave them mismatched: with
	// AnticipateChips alone a move is scored on a one-to-four-week expectation and
	// then charged over five, which over-credits near-term fixture spikes by
	// construction. Running all three — scoring only, gate only, both — is what
	// separates the levers; running only the first is the one combination that is
	// wrong on purpose.
	//
	// Has no effect unless AnticipateChips is also set — enforced in `decide`
	// rather than merely asserted here, because the engine horizon it reads is
	// only shortened there and an assertion of that shape is what this record
	// keeps finding falsified by a later change.
	//
	// It takes the *shorter* of the shortened engine horizon and
	// `DecisionHorizon`, never the engine's outright. See `decide`: replacing it
	// would re-merge the two jobs `DecisionHorizon` exists to separate.
	AnticipateGate bool

	// PrepareBenchBoost lets the weekly transfer decision build toward a planned
	// bench boost, and PrepareTripleCaptain toward a planned triple captain.
	//
	// # This is the untested half of how the chips are actually played
	//
	// Anchoring the chip *weeks* on the calendar is measured — about +10 a season,
	// flat across two, four, six and full gameweeks of sight. What was never tested
	// is holding fifteen playable footballers into the boost week rather than eleven
	// and four bodies, and owning the right premium in the tripled week. Both were
	// blocked by expression rather than by knowledge: the pool of candidate doublers
	// is knowable weeks ahead from the public fixture list, and the transfer
	// objective simply had no term for either payoff. `OptimizeRequest.BenchBoost`
	// reaches the same mechanism on the opening-squad path and buys 59% more bench
	// quality for £2.5m, which is the size of what the transfer search cannot see.
	//
	// This is why the coherent `AnticipateChips` arm reading +2.5 a season is not
	// evidence against squad preparation: it tested the two levers that were wired
	// and was silent on the two that were not.
	//
	// # They are separate switches on purpose
	//
	// One credits the bench, the other the armband, and they reach different
	// players — the boost is bought at the cheap end of the squad and the triple
	// captain at the expensive end. Running them together as one arm would measure
	// their sum and neither, the mistake `AnticipateGate` exists to avoid.
	//
	// # What they do not do
	//
	// Neither changes the chip *plan*: which weeks the chips are played in is
	// `Chips`/`ChipPlanner`'s job, and holding placement fixed is what makes the
	// difference attributable to preparation. Neither reaches squad construction —
	// the credit lives on `analysis.SquadState`, which only the transfer searches
	// read — so HOLD must come back byte-identical, and that is the confinement
	// check rather than a hope. Neither is read by the unified search, which values
	// squads through `Optimize` rather than through the ranked searches; an arm that
	// set both `Unified` and either of these would be measuring a knob its search
	// never sees, which is the recorded byte-identical-null failure.
	//
	// Both are off by default, so every figure recorded before they existed stays
	// comparable.
	PrepareBenchBoost    bool
	PrepareTripleCaptain bool

	// WildcardIgnoresBoost makes a wildcard played the week before a bench boost
	// rebuild an ordinary squad rather than one built for the chip.
	//
	// It is the per-cell form of the `FPL_WC_IGNORES_BOOST` escape hatch, and it
	// exists because that hatch is a package-level variable: an arm that flipped it
	// in `apply` would leave it set for every arm after it, and the provenance
	// sidecar would stamp whichever value the last arm happened to install. A
	// sweep cannot vary a global safely.
	//
	// Zero — build for the boost — is the shipped behaviour and the sequence the
	// chip is actually played in. Setting it isolates *the wildcard* from
	// *building for the chip*, which is the only way to ask whether the
	// wildcard-into-boost play is worth anything beyond the rebuild itself.
	WildcardIgnoresBoost bool

	// EarlyFloor is the scheduled gate floor: the charge and gain bar applied
	// up to and including UntilGameweek, the shipped constants after. The zero
	// value is off. See config.EarlyFloor.
	EarlyFloor config.EarlyFloor

	// FreeCost is what spending a free transfer is charged, in points across
	// the horizon. Zero reproduces the original policy, where only MinGain
	// gated a free move.
	//
	// It does not taper as the season ends, which was tried on the argument
	// that a transfer banked through the final whistle is worth nothing. The
	// argument is sound about *option value* and wrong about what this charge
	// does: it is a confidence threshold, and a marginal move in May is exactly
	// as likely to be noise as one in September with less time to be right.
	// Tapering to zero scored 2199 against 2208 for holding the full charge,
	// and left 20 transfers after GW33 returning +72 where the flat charge
	// makes 8 returning +89.
	FreeCost float64

	// Chips is the gameweek each chip is played at, zero for never. The replay
	// modelled none of them until this existed, so every figure recorded before
	// it is a chipless season.
	//
	// # They are one decision, not four
	//
	// The reason to model all four together rather than the two scoring chips
	// alone is that the standard play is a *sequence*: wildcard into a squad
	// with a real bench, bench boost it, then transfer the surplus bench value
	// back out. Measuring bench boost on a squad built under the ordinary
	// objective measures a floor, because that objective credits the bench at
	// almost nothing and the fifteen converges on eleven good players and four
	// who cannot cover. The wildcard is what makes the bench affordable, so
	// without it the chip cannot be measured at its real value.
	//
	// A free hit is the other shape: a temporary fifteen for one gameweek,
	// handed back afterwards, which is what a blank gameweek wants.
	Chips analysis.ChipPlan

	// Chips2 is the SECOND set, which FPL grants from GW20 in a season that
	// resets the chips at the halfway point.
	//
	// It is a separate field rather than a longer ChipPlan because the two sets
	// are not interchangeable: a chip from the first set cannot be carried past
	// GW19, and one from the second cannot be played before GW20. Folding them
	// into eight loose integers would let a plan express something FPL forbids,
	// and the validator would be the only thing standing between a sweep and a
	// season nobody could have played.
	//
	// It is also season-gated, and deliberately not applied by default: 2025-26
	// is the only archived season with the rule, and this record already warns
	// against projecting the two-set rule backwards to buy chip observations.
	// Across all six archived first halves there are 15 doubling club-gameweeks
	// out of 189, and 11 of the 15 are one COVID-rescheduled 2020-21 round — so
	// an H1 arm is collinear with "a chip on a plain week" in five seasons of
	// six, rather than in every season, which the shorter phrasing implied.
	//
	// ChipSetsFor is the gate and Simulate applies it, so a caller that sets this
	// on an older season gets an error rather than a season that quietly plays
	// eight chips nobody had. That is deliberate placement: the first version put
	// the check in cmd/armband, which every sweep bypasses.
	//
	// ⚠️ This is the REPLAY side only. The record asked for two sets as a fidelity
	// fix for the LIVE agent, which cannot express "H1 boost at GW14, H2 boost at
	// GW34" and will need to for 2026-27. `analysis.ChipPlan` and `config.Chips`
	// are still one set, so that item is untouched by this field.
	Chips2 analysis.ChipPlan

	// OptionPricing is the decay and congestion curve every lever below shares.
	// Zeroes mean the analysis package's defaults. See analysis.OptionPricing.
	OptionPricing analysis.OptionPricing

	// TaperFreeTransferValue makes FreeCost a function of the season's remaining
	// life and the squad's forward fixture congestion rather than a constant.
	//
	// ⚠️ It changes what a transfer is CHARGED, which is a reluctance to spend —
	// not a term in any forward valuation. `decide` is greedy per week and prices
	// no future state in either arm, so a movement here means "the policy became
	// more or less willing to act in these weeks", never "the policy started
	// planning". See analysis.TransferHoldFactor.
	TaperFreeTransferValue bool

	// WildcardTrigger, BenchBoostTrigger and FreeHitTrigger are state rules that
	// play a chip the plan does not name, against a bar that decays as the chip's
	// own life runs out. Each fires at most once, only for a chip no plan places,
	// and never in a gameweek another chip already occupies.
	//
	// ⚠️ **A zero bar here means a bar of zero**, unlike `config.ChipTrigger`
	// where it means the default. There is no file to backfill from in a sweep and
	// an arm that wanted zero would have no way to say it; the difference is
	// deliberate and is why the two types are not shared.
	WildcardTrigger, BenchBoostTrigger, FreeHitTrigger bool
	// WildcardReservation is the points a repair cost must beat for the wildcard
	// trigger to fire, before decay. BenchBoostBar and FreeHitBar are the same for
	// the two scoring chips, in points of one week's gain.
	WildcardReservation, BenchBoostBar, FreeHitBar float64

	// RecordRepairCost fills SimResult.RepairSeries: the held-versus-fresh
	// distance, observed every gameweek on the evolving fifteen and on the frozen
	// opening one.
	//
	// ⚠️ **It is NOT a fifth option-value lever and must never become one.** The
	// four above change what the season does; this changes only what the season
	// reports, and `TestTheRepairSeriesChangesNoDecision` fails if that stops being
	// true. It sits here rather than in the lever block for that reason, and it is
	// deliberately absent from `TestTheOptionValueLeversAreIndependent`'s table,
	// which is about levers implying one another.
	//
	// It is behind a switch rather than filled unconditionally like the banking and
	// fixture-run mediators because it costs THREE `Optimize` calls a gameweek —
	// the expensive call in this package — where those cost arithmetic. Read the
	// per-week cost before turning it on in anything that is not a diagnostic.
	RecordRepairCost bool

	// StartGW is the gameweek the entry begins at, defaulting to 1.
	//
	// A replay is one path through a season, and one flipped transfer early
	// changes the squad for every week after it — which is where nearly all of
	// the replay's parameter sensitivity comes from. Entering at GW6 instead
	// produces a different path through the same football, so averaging over
	// several start points averages over paths rather than over seasons, of
	// which there are only four with xG.
	//
	// It is a real scenario rather than a synthetic one: FPL lets a manager
	// join at any deadline with a fresh £100m and unlimited transfers, which is
	// exactly what this simulates.
	StartGW int

	// Oracles is the hindsight this cell is granted. Zero — no hindsight — is
	// the only value any figure quoted as a baseline may have been measured
	// under, and Simulate validates it rather than trusting the caller.
	//
	// It is a field rather than an environment variable so that both arms of a
	// comparison can toggle inside one process and pair properly, so that
	// concurrent cells cannot read each other's setting, and so that the
	// provenance stamp is derived from the same value the simulation consumed.
	// See Oracles.
	Oracles Oracles

	// lineupCovers restricts which players OracleLineups is allowed to know about.
	// Nil — every ordinary arm, including every shipped path — means all of them.
	//
	// Unexported on purpose. This is an instrument for one measurement, not a
	// setting: a sweep axis for it would let a restricted arm be mistaken for the
	// oracle itself, and the two answer different questions. It lives here rather
	// than in a test helper so that `recentIndex` stays the single place an oracle
	// is constructed, which is what `TestEveryScoringEngineGetsRecency` exists to
	// protect.
	lineupCovers lineupCoverage

	// gateLog is called once for every package offered to the transfer gate, with
	// what the gate answered and with the two channels the residual family's
	// criteria are built from. Nil — every ordinary arm and every shipped path —
	// means nothing is recorded and nothing is computed.
	//
	// Unexported for the same reason lineupCovers is: it is an instrument for one
	// measurement rather than a setting, and it may not change a decision. Nothing
	// branches on it; `acceptTransfer` calls it *after* the answer is fixed.
	//
	// It exists because a transfer COUNT cannot answer the questions a gate
	// contrast's null needs answered — the accept mass, the mean underlying gain of
	// the offered stream, and the covariance between that gain and the sign of the
	// conversion residual. Equal counts are not the same packages, and the gate
	// diagnostic has recorded owing this statistic since its first re-run.
	gateLog func(gatePackage)

	// bankLog is called once for every week shouldBank is consulted in, with the
	// two arms it compared and the two counterfactuals that attribute the answer
	// to a channel. Nil — every ordinary arm and every shipped path — means
	// nothing is recorded.
	//
	// Unexported for the same reason gateLog is: it is an instrument for one
	// measurement rather than a setting, and it may not change a decision.
	// `shouldBank` calls it *after* its answer is fixed, and computes nothing for
	// it that the comparison did not already compute.
	//
	// It exists because the banking mediator's `banked_weeks` cannot say WHY the
	// rule declined. On the one banked arm the rule weighed a real choice in 169
	// of 236 consulted weeks and chose to act every time, and a count of zero is
	// equally consistent with "waiting is worth nothing here" and with "waiting is
	// unreachable at this configuration by construction" — which license opposite
	// readings of any tandem sweep built on top of it.
	bankLog func(bankProbe)

	// ChipPlanner derives the chip plan from the season, for a rule that depends
	// on the fixture calendar rather than on a week number. Nil takes Chips as
	// given, which is every ordinary arm.
	//
	// **A planner reading the season's own fixture list sees more than a manager
	// does, and how much more depends on what it reads.** This comment used to
	// call it hindsight flatly and say a realistic version needs a reveal lag the
	// archive cannot supply. Both halves were too strong, and `anchored_diag_test.go`
	// carries the correction: *which gameweek* carries a double is largely
	// knowable early, since the fixture list is public and the pool of clubs with
	// postponed matches is visible weeks ahead, while *which clubs* double and the
	// exact magnitude resolve late. A planner that chooses only weeks is using the
	// cheap kind of knowledge. And a reveal lag is supplyable — `laggedPlan` models
	// sight directly by refusing to look more than n gameweeks ahead.
	//
	// So read a full-sight planner as a **target** rather than as a fiction. A
	// planner reading anything the fixture list does not publish in advance —
	// realised points, final minutes — is a different thing and belongs in
	// `Oracles`, where it gets stamped on every cell.
	ChipPlanner func(cur *Season, start int) analysis.ChipPlan
}

// startGW is the gameweek to begin at, defaulting to 1.
func (c SimConfig) startGW() int {
	if c.StartGW >= 1 && c.StartGW <= 38 {
		return c.StartGW
	}
	return 1
}

// Simulate plays a season from gameweek 1, re-deciding every week.
//
// Each week the model is rebuilt from data through the previous gameweek, the
// review policy is applied, and the resulting squad is scored on what actually
// happened. The eleven and the captain are re-chosen weekly from the model's
// scores at that point, which is what a manager does.
//
// The comparison that matters is against holding the opening squad all season.
// Transfers cost free transfers and sometimes four points, so they have to earn
// their place; a policy that churns and finishes level has lost.
func Simulate(cur, prior *Season, cfg SimConfig) (*SimResult, error) {
	// Checked here as well as in PointInTime, so the entry point with an error channel
	// reports rather than panics. PointInTime is the backstop that covers Hold,
	// HoldWeekly and HoldCaptaincyWeekly, which have none.
	if err := cur.PlayableAsCurrent(); err != nil {
		return nil, err
	}
	// A plan derived from the season resolves before anything reads it, so
	// everything downstream sees one already-validated ChipPlan and no caller has
	// to know which of the two ways it arrived.
	//
	// It exists because a variant's apply hook is handed only the SimConfig, and
	// an anchored plan — free hit the biggest blank, boost the biggest double — is
	// a function of the fixture calendar, which differs per season. Resolving here
	// keeps the anchoring rule in the diagnostic that is asking the question
	// rather than putting a policy into the harness.
	if cfg.ChipPlanner != nil {
		// Routed into the sets the season granted them from, rather than dropped
		// wholesale into the first. A planner answers "which week", and which SET
		// that week draws from is a fact about FPL's calendar that every planner
		// would otherwise have to know separately — which is how none of them did,
		// and how an anchored arm silently lost every 2025-26 cell. See
		// SplitChipSets.
		//
		// ⚠️ It overwrites Chips2 as well as Chips. A caller setting both a planner
		// and a literal second set is asking two things to decide one quantity, and
		// the planner is the one that was asked for; the alternative — merging —
		// would let a stale literal collide with a planned week and be refused by
		// the validator for a reason nobody wrote.
		sch := SplitChipSets(cur.Name, cfg.ChipPlanner(cur, cfg.startGW()))
		cfg.Chips, cfg.Chips2 = sch.First, sch.Second
	}
	// And the chip plan, which FPL constrains in ways a config file does not:
	// one chip per gameweek, one set before the GW19 reset and one after it, and
	// a second set only in a season that granted one.
	//
	// Gated HERE, on cur.Name, rather than at the command that happens to have a
	// season string handy. Every sweep and every diagnostic builds a SimConfig
	// literal inside this package and never goes near cmd, so a validator living
	// there gates the one caller that was already careful and none of the ones
	// that matter. That is the recorded rule about naming the CONSUMER rather
	// than a package, arriving from the other side — and the precedent is
	// DefconScoredIn, which is enforced in here for exactly this reason.
	if err := ValidateChipSets(cur.Name, cfg.Chips, cfg.Chips2); err != nil {
		return nil, err
	}
	// And the hindsight, for the same reason: an oracle bit with no hook behind
	// it would stamp a variant and measure nothing, which reads downstream as a
	// clean null rather than as a wiring failure.
	if err := cfg.Oracles.Validate(); err != nil {
		return nil, err
	}
	if err := checkFeaturesFrom(cfg.Oracles, cfg.StartGW); err != nil {
		return nil, err
	}
	// And the chip-preparation switches against the search that cannot read them,
	// for exactly the reason above. `unifiedDecide` values squads through
	// `Optimize` at `BenchWeight: 0` and only *gates* on `XIValue`, so a
	// preparation credit never reaches its objective. Left unguarded, an arm that
	// sets both — or a shell that still has FPL_UNIFIED_TRANSFERS exported from
	// another sweep — returns a byte-identical null with nothing anywhere saying
	// the knob did not arrive, which is this record's signature failure and is
	// precisely how the null would read: "preparation does nothing".
	if (cfg.PrepareBenchBoost || cfg.PrepareTripleCaptain) && (unifiedTransfers || cfg.Unified) {
		return nil, fmt.Errorf("chip preparation is not readable by the unified search: " +
			"it values squads through Optimize rather than through the ranked searches, " +
			"so PrepareBenchBoost/PrepareTripleCaptain would measure a knob its objective " +
			"never sees (unset Unified, or FPL_UNIFIED_TRANSFERS in the environment)")
	}
	idx := cfg.priors(cur, prior)
	start := cfg.startGW()
	// The opening squad is built from what was showing at the start point, so a
	// mid-season entry knows what a mid-season entrant would.
	boot, fx := PointInTimeWith(cur, prior, start-1, cfg.Oracles)
	// The opening squad may be built on a different fixture weight from the one
	// the weekly transfer decision uses. The hypothesis is that difficulty is
	// worth more to a transfer, which deliberately buys a run of fixtures, than
	// to a fifteen picked before any fixture is near — the same split that let
	// fixture load reach transfers without touching squad selection.
	ow := cfg.Weights
	if cfg.SquadFixtureWeight > 0 || cfg.SquadFixtureWeightSet {
		ow.FixtureWeight = cfg.SquadFixtureWeight
	}
	e := analysis.NewEngineFull(boot, fx, ow, analysis.Congestion{}, analysis.RoleRisk{})
	e.Priors = idx
	e.Recent = cfg.recentIndex(cur, start-1)
	e.TeamForm = newTeamFormIndex(cur, start-1)
	minExp := cfg.resolvedMinExpectedMinutes()
	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: cfg.Budget, MinMinutes: 600, MinExpectedMinutes: minExp,
		BenchWeight: cfg.openingBenchWeight(),
	})
	if err != nil {
		return nil, fmt.Errorf("opening squad: %w", err)
	}

	held := make([]int, 0, 15)
	for _, p := range sq.Players {
		held = append(held, p.ID)
	}
	// What the squad cost is what the optimiser was quoted, which is the price
	// carried on the bootstrap it just chose from. Taking it from anywhere else
	// is a bug with a nasty signature: PointInTime prices at `start-1` while
	// squadValue prices at `start`, so a mid-season entry was charged a week of
	// price rises it never paid, opened with a slightly negative bank, and every
	// subsequent week the transfer search would only accept moves that *freed*
	// money. Twenty-four forced downgrades and 463 points on 2023-24.
	buy := func(id int) int {
		if el := boot.ElementByID(id); el != nil {
			return el.NowCost
		}
		return marketPrice(cur, id, start)
	}
	total := 0
	for _, id := range held {
		total += buy(id)
	}
	res := &SimResult{StartValue: total}
	res.OpeningSquad = append([]int(nil), held...)
	w := newWallet(cfg.Budget - res.StartValue)
	for _, id := range held {
		w.bought[id] = buy(id)
	}
	free := 1
	// The frozen arm the repair-cost observer reads: the opening fifteen, the
	// wallet as it stood before a single transfer, and an allowance that accrues
	// and is never spent. All three are snapshots taken HERE, where the opening
	// state exists, rather than reconstructed later from a result — a second
	// construction of the opening squad is this package's most-repeated bug.
	//
	// `frozenFree` starts at 1 beside `free` and takes the same accrual rule
	// `decide` applies, one line below where the week's decision would apply it.
	var frozenWallet *wallet
	frozenFree := free
	// prevFresh carries last week's fresh optimum so the observer can measure
	// how far the optimum MOVES when one gameweek of data arrives — the
	// worldview-rewrite column. Nil until the second observed week; see
	// RepairWeek.FreshChurn.
	var prevFresh []int
	if cfg.RecordRepairCost {
		frozenWallet = w.clone()
	}
	// The armband oracle's mediator, counted rather than inferred. See
	// SimResult.Armband.
	armbandWeeks, armbandChanged := 0, 0
	// The transfer-banking mediator, on the same principle and for every arm
	// rather than one. See SimResult.Banking.
	var banking BankingMediator
	var fixtureRuns FixtureRunMediator
	// The option-value levers' mediators, on the same principle: a lever that is
	// wired and inert reports a clean null, which is indistinguishable from a real
	// one. See TransferHoldMediator and ChipTriggerMediator.
	var hold TransferHoldMediator
	var prep ChipPrepMediator
	// The gate-floor counterfactual, on every arm for the same reason: a floor
	// arm's null is unreadable without a count of the proposals the shipped gate
	// would have answered differently. See GateFloorMediator.
	var floor GateFloorMediator
	// One state per triggered chip, so a rule fires at most once and never in a
	// gameweek another chip already occupies. `triggered` is what makes a fired
	// chip visible to the week's scoring switch; the mediators record why.
	trig := newChipTriggers(cfg)

	for gw := start; gw <= 38; gw++ {
		week := Week{GW: gw}

		// Decide before the gameweek, using only what had happened before it.
		// Never on the first week played: the squad was just bought.
		if gw > start {
			pb, pf := PointInTimeWith(cur, prior, gw-1, cfg.Oracles)
			pe := analysis.NewEngineFull(pb, pf, cfg.Weights, analysis.Congestion{}, analysis.RoleRisk{})
			pe.Priors = idx
			pe.Recent = cfg.recentIndex(cur, gw-1)
			pe.TeamForm = newTeamFormIndex(cur, gw-1)
			cfg.anticipate(pe, gw)

			// The repair-cost observer, read off the same pre-deadline engine the
			// decision itself runs on and BEFORE anything decides anything. It
			// writes to `res` and to nothing else; no branch below reads it. See
			// RepairWeek for what the two squads discriminate, and
			// TestTheRepairSeriesChangesNoDecision for the pin.
			if cfg.RecordRepairCost {
				// The accrual `decide` applies, applied here first because the
				// frozen arm never reaches `decide` — it makes no transfers, so
				// nothing else would ever advance its allowance.
				if frozenFree < cfg.BankUpTo {
					frozenFree++
				}
				var obs RepairWeek
				obs, prevFresh = observeRepair(pe, cur, w, frozenWallet, held,
					res.OpeningSquad, prevFresh, gw, free, frozenFree, minExp, cfg)
				res.RepairSeries = append(res.RepairSeries, obs)
			}

			// The two chip state rules that take the transfer decision away are
			// consulted BEFORE the switch that would take it away, and both read
			// the pre-deadline engine the decision itself runs on — never
			// Week.BenchBoostGain or Week.TripleCaptainGain, which are computed
			// after the gameweek is scored and are hindsight.
			//
			// Each is behind its own switch, and neither implies the other or the
			// free-transfer taper. See chiptriggers.go for why that independence
			// is load-bearing rather than tidy.
			if trig.eligible(slotWildcard, gw, cfg.WildcardTrigger) {
				// The allowance the week will actually have: the accrual below
				// runs inside every branch of this switch, so reading `free` raw
				// would price the repair against one transfer too few and make
				// the rule fire more often, in exactly the early weeks the
				// recorded closure says a bad trigger fires in.
				avail := free
				if avail < cfg.BankUpTo {
					avail++
				}
				cost, ok := repairCost(pe, cur, w, held, gw, avail, minExp, cfg)
				trig.consult(slotWildcard, gw, cur.Name, cfg.WildcardReservation,
					pe.HoldingCongestion(held, gw, cfg.OptionPricing), cost, ok)
			}
			if trig.eligible(slotFreeHit, gw, cfg.FreeHitTrigger) {
				// ⚠️ **This is the most expensive line in the option-value work
				// and it is deliberately gated hard.** `freeHitValue` calls
				// `freeHitSquad`, which is a full `Optimize` — the expensive call
				// in this package — and unlike the wildcard's the free-hit rule
				// never becomes ineligible on its own, so an ungated version pays
				// for roughly 37 rebuilds a cell. Performance is the second-ranked
				// class in this record, so the guards matter: the rule stops on
				// the first fire, refuses the whole season when a plan owns the
				// chip, and the lever ships off.
				//
				// `blanking` is the cheap pre-filter and it is not a heuristic —
				// a free hit's whole value is fielding a squad that plays in a
				// round the held one does not, so a gameweek where every club the
				// squad owns has a fixture cannot be worth the chip whatever the
				// rebuild finds. It skips the rebuild rather than the reading, so
				// a skipped week is not counted as weighed.
				//
				// The recency and team-form indexes are shared with `pe` rather
				// than rebuilt. They are functions of `(cur, gw-1)` and `pe`
				// already holds them, so a second construction was one quantity
				// computed twice per week — the bench-boost site below already
				// reused `ve`'s.
				if squadBlanks(pe, held) {
					fe := oneWeekEngine(pb, pf, cfg.Weights, idx, pe.Recent, pe.TeamForm)
					v, ok := freeHitValue(fe, cur, w, held, gw, minExp, cfg)
					trig.consult(slotFreeHit, gw, cur.Name, cfg.FreeHitBar,
						fe.HoldingCongestion(held, gw, cfg.OptionPricing), v, ok)
				}
			}

			switch {
			case trig.plays(slotWildcard, gw):
				// A wildcard is unlimited transfers for one week at no hit cost,
				// so it is not a transfer *decision* at all — it is the opening
				// squad problem solved again, at whatever money the manager now
				// has.
				//
				// Free transfers carry over rather than being spent, and still
				// accrue: FPL's chip replaces the week's transfers instead of
				// consuming the bank.
				if free < cfg.BankUpTo {
					free++
				}
				next, n, err := playWildcard(pe, cur, w, held, gw, minExp, cfg)
				if err != nil {
					return nil, fmt.Errorf("wildcard at GW%d: %w", gw, err)
				}
				held = next
				week.Transfers += n
				res.Transfers += n
				week.Wildcard = true

			case trig.plays(slotFreeHit, gw):
				// A free hit reverts everything the following gameweek, so a
				// *permanent* transfer cannot be made in this week at all —
				// running the ordinary decision here would change the squad the
				// manager gets handed back, which FPL does not allow. The free
				// transfer carries over and still accrues, as with a wildcard.
				if free < cfg.BankUpTo {
					free++
				}

			default:
				var moves []Move
				var wb weekBanking
				var wh weekHold
				held, free, moves, wb, wh = decide(pe, cur, held, w, free, gw, cfg)
				// The taper's funnel, and the preparation credit's. Both are read
				// off the decision that was actually taken, on the engine it was
				// taken with, and neither can change one.
				if wh.Consulted {
					hold.ConsultedWeeks++
					if wh.Factor != 1 {
						hold.PricedWeeks++
					}
					hold.GateCalls += wh.GateCalls
					hold.Flips += wh.Flips
					hold.ChargeSum += wh.Charge
					hold.LoadSum += wh.Load
				}
				if cfg.PrepareBenchBoost || cfg.PrepareTripleCaptain {
					prep.ConsultedWeeks++
					if wh.Credit.Bench != 0 || wh.Credit.Captain != 0 {
						prep.CreditWeeks++
						prep.BenchSum += wh.Credit.Bench
						prep.CaptainSum += wh.Credit.Captain
					}
				}
				// The gate-floor counterfactual, counted on the same decisions as
				// everything else above. Split at the quiet boundary: a floor
				// drop's early flips are where the user's hypothesis lives, and
				// its late flips are the canary for a scheduled floor.
				floor.Le28 += wh.FloorFlipsLe28
				floor.Gt28 += wh.FloorFlipsGt28
				// The mediator, accumulated only on weeks that actually reached
				// the decision: a wildcard or a free-hit week is one the banking
				// rule could not have been asked in, and counting it would put a
				// week the arm never governed into the denominator.
				banking.DecisionWeeks++
				banking.FreeHeld += wb.Free
				if wb.Consulted {
					banking.ConsultedWeeks++
				}
				if wb.Weighed {
					banking.WeighedWeeks++
				}
				if wb.Banked {
					banking.BankedWeeks++
				}
				// The fixture-run mediator, on the same weeks and the same engine
				// the decision was actually taken with. `pe` is the transfer
				// engine, so the bands read here are the bands that scored the
				// candidates — not a second opinion computed from the archive.
				//
				// Read after `decide` rather than inside it, which keeps `decide`'s
				// signature alone: everything this needs is the engine, the moves
				// and the horizon, and all three are in scope here. It is
				// read-only — no branch below can change a decision — which is what
				// makes it safe to run on every arm rather than behind a switch.
				if pe.BandChannelLive() {
					fixtureRuns.ReadyWeeks++
					for _, mv := range moves {
						d, ok := bandExposureDelta(pe, mv)
						if !ok {
							continue
						}
						fixtureRuns.Moves++
						fixtureRuns.Exposure += d
						switch {
						case d > 0:
							fixtureRuns.RunMoves++
						case d < 0:
							fixtureRuns.WorseMoves++
						}
					}
				}
				for _, mv := range moves {
					res.Moves = append(res.Moves, mv)
					res.Transfers++
					week.Transfers++
					if mv.Hit {
						res.Hits++
						res.HitCost += 4
						week.HitCost += 4
					}
				}
			}
		}

		// Field the eleven the model would pick from what it knows now.
		vb, vf := PointInTimeWith(cur, prior, gw-1, cfg.Oracles)
		// The eleven is normally picked on the horizon average, which is the
		// shipped behaviour and was measured as worth the same as picking on the
		// imminent match. That measurement predates doubles being visible at
		// all, and a double is the one thing a one-week view should catch:
		// starting a player who plays twice this Saturday is free.
		vw := cfg.Weights
		if cfg.WeeklyXI {
			vw.Horizon = 1
		}
		ve := analysis.NewEngineFull(vb, vf, vw, analysis.Congestion{}, analysis.RoleRisk{})
		ve.Priors = idx
		ve.Recent = cfg.recentIndex(cur, gw-1)
		ve.TeamForm = newTeamFormIndex(cur, gw-1)
		// The free hit fields a temporary fifteen for this gameweek only and
		// hands the permanent squad back afterwards, so it is the one chip that
		// changes *which* squad is scored rather than how. Nothing here touches
		// held or the wallet: the money is the same money, borrowed for a week.
		fielded := held
		if trig.plays(slotFreeHit, gw) {
			// Built on a one-gameweek horizon whatever the eleven is picked on:
			// this squad exists for a single match round, so there is no run of
			// fixtures to average over. Averaging one would be the same mistake
			// as picking an eleven on a five-game view for a double this
			// Saturday, and it is the whole reason a free hit targets a blank.
			he := oneWeekEngine(vb, vf, cfg.Weights, idx, ve.Recent, ve.TeamForm)
			// The error propagates, exactly as the wildcard's does above.
			// Discarding it — which this did until 2026-08-13 — makes a failed
			// build indistinguishable from a chip that was never worth
			// anything: `fielded` stays as `held`, `week.FreeHit` stays false,
			// and the cell scores as an ordinary week with nothing recording
			// that the chip did not fire. A free-hit arm would then read as
			// "worth zero" rather than "did not run", which is the
			// byte-identical-null trap this record names, arriving through a
			// swallowed error rather than an unwired knob.
			temp, err := freeHitSquad(he, cur, w, held, gw, minExp, cfg)
			if err != nil {
				return nil, fmt.Errorf("free hit at GW%d: %w", gw, err)
			}
			fielded = temp
			week.FreeHit = true
		}
		xi, bench, captain, vice := pickXI(ve, fielded)
		// The armband oracle, overriding two return values and nothing else. The
		// eleven above is still the model's own, which is what confines this to one
		// axis. See armbandOracle.
		if cfg.Oracles.Decision == AxisArmband {
			oc, ov := bestArmband(cur, xi, gw)
			if oc != captain {
				armbandChanged++
			}
			armbandWeeks++
			captain, vice = oc, ov
		}

		xiP, benchP := idsToPlayers(cur, xi), idsToPlayers(cur, bench)
		chip := chipNone
		// Either set. Simulate calls ValidateChipSets before any of this runs, and
		// that refusal is what makes this ordered switch safe: an unvalidated plan
		// with two chips in one gameweek would silently drop the second here and
		// score a week the manager could not have played.
		//
		// The check lives in Simulate rather than in the command precisely so that
		// sentence is true of every caller. It was written at `cmd` first, where
		// it gated the one caller that was already careful and none of the sweeps,
		// all of which build a SimConfig literal in this package.
		// The bench boost's state rule, consulted here because this is the point
		// the bench is known: the chip pays the four players `pickXI` just left
		// out, and which four those are is a consequence of the week's transfers.
		//
		// It is deliberately AFTER the free hit above. A free-hit week fields a
		// temporary fifteen the manager does not own, and FPL allows one chip a
		// week regardless — `trig.anyPlays` refuses the overlap — so consulting
		// the boost on a borrowed bench would be pricing a chip that cannot be
		// played on a squad that is not there.
		//
		// ⚠️ **`gw > start` excludes the ENTRY week, and that is a choice rather
		// than an inherited constraint.** The decision block's own `gw > start`
		// exists because the squad was just bought and there is nothing to decide;
		// here the bench is known and the chip could legally be played. It is
		// excluded to keep the rule commensurable with the two above, which sit
		// inside the decision block and cannot reach the entry week — a bench
		// boost available in a window the other two are not would make a 2x2 over
		// the three levers compare different windows. The cost is one candidate
		// week per cell: 1 of 38 at a GW1 entry and **1 of 13 at GW26**, which is
		// where it is worth knowing about.
		if gw > start && trig.eligible(slotBenchBoost, gw, cfg.BenchBoostTrigger) {
			be := oneWeekEngine(vb, vf, cfg.Weights, idx, ve.Recent, ve.TeamForm)
			v, ok := benchBoostValue(be, bench)
			// ⚠️ The load is read from gw+1, so the double the boost is being
			// played FOR does not raise the bar it has to clear. Reading it from
			// gw did exactly that — the reading and the bar both rose together,
			// and the rule was hardest to satisfy in the weeks it exists for.
			trig.consult(slotBenchBoost, gw, cur.Name, cfg.BenchBoostBar,
				be.HoldingCongestion(held, gw, cfg.OptionPricing), v, ok)
		}
		switch {
		case trig.plays(slotBenchBoost, gw):
			chip = chipBenchBoost
			week.BenchBoost = true
		case cfg.plays(slotTripleCaptain, gw):
			chip = chipTripleCaptain
			week.TripleCaptain = true
		}
		ws := weekScoreWithChip(xiP, benchP, gw, captain, vice, chip)
		pts := ws.Points
		// What each scoring chip would have been worth here, always against the
		// *unchipped* week — otherwise a week that actually played one reports
		// its own gain as zero, and the diagnostic that reads these columns
		// would go quiet exactly where a chip was used.
		plain := weekPoints(xiP, benchP, gw, captain, vice)
		week.BenchBoostGain = weekPointsWithChip(xiP, benchP, gw, captain, vice, chipBenchBoost) - plain
		week.TripleCaptainGain = weekPointsWithChip(xiP, benchP, gw, captain, vice, chipTripleCaptain) - plain
		week.Gross = pts
		week.GrossXP = ws.XPoints
		week.Net = pts - week.HitCost
		week.XI = append([]int(nil), xi...)
		week.Squad = append([]int(nil), held...)
		week.Free = free
		// Priced as decide() prices them: on the week the decision was made
		// from, so the recorded figure matches what the search actually saw.
		week.Sell = w.sellPrices(cur, held, gw-1)
		week.Value = squadValue(cur, held, gw)
		week.Bank = w.bank
		if c := cur.Players[captain]; c != nil {
			week.Captain = c.WebName
			week.CaptainPts = c.GWs[gw].Points * 2
		}
		res.Weeks = append(res.Weeks, week)
		res.Points += pts
		res.XPoints += ws.XPoints
	}
	res.Points -= res.HitCost
	res.XPoints -= float64(res.HitCost)
	// Team value as FPL reports it: what the squad would raise, plus the bank.
	// Summing market prices overstates it by half of every rise.
	res.EndValue = w.value(cur, held, 38)
	// The chip-week oracle, after the season is over and reading only what it
	// recorded. Placed here rather than inside the loop so that it *cannot* reach a
	// decision: by this line every transfer is made, every eleven is fielded and
	// every point is scored.
	if cfg.Oracles.Decision == AxisChipWeek {
		res.ChipOracle = placeChips(res.Weeks, bestChipWeek)
	}
	if cfg.Oracles.Decision == AxisArmband {
		res.Armband = &ArmbandOracle{Weeks: armbandWeeks, Changed: armbandChanged}
	}
	res.Banking = banking
	res.FixtureRuns = fixtureRuns
	res.TransferHold = hold
	res.ChipPrep = prep
	res.GateFloor = floor
	// Copied out rather than shared, so a caller holding a SimResult cannot reach
	// back into the season's live trigger state. The three are read separately
	// because the levers are switchable separately — a null on one has to be
	// readable without reference to the others.
	res.Wildcard = *trig.med[slotWildcard]
	res.FreeHit = *trig.med[slotFreeHit]
	res.BenchBoost = *trig.med[slotBenchBoost]
	return res, nil
}

// bandExposureDelta is what one move did to the squad's banded fixture exposure:
// the incoming player's run less the outgoing player's, over the horizon the
// engine scored them on.
//
// Reports `ok` false when either element cannot be resolved, so an unresolvable
// move is left out of the count rather than entered as a zero. Those are
// different facts, and a zero delta is a real and common observation — most
// transfers are between two mid-band clubs — so silently folding failures into it
// would put a data gap inside the measurement.
//
// ⚠️ The two players may be different positions, in a funded pair. Each side is
// read on its own position's band, which is the right comparison: the question is
// what the squad's exposure did, and a defender's exposure is to the opponent's
// attack while a forward's is to its defence.
func bandExposureDelta(e *analysis.Engine, mv Move) (int, bool) {
	out := e.Boot.ElementByID(mv.OutID)
	in := e.Boot.ElementByID(mv.InID)
	if out == nil || in == nil {
		return 0, false
	}
	// e.Weights.Horizon, not cfg's: this must be the window Metrics scored the
	// candidates over, and ApplyChipPlan may already have shortened it for a week
	// before a chip. Reading the configured horizon here would report on a run
	// nobody was scored against.
	h := e.Weights.Horizon
	outRun := e.FixtureRunFor(out.Team, h, out.ElementType)
	inRun := e.FixtureRunFor(in.Team, h, in.ElementType)
	return inRun.Net() - outRun.Net(), true
}

// decide applies the review policy for one gameweek.
//
// Transfers are made one at a time and the board is re-read between each, since
// swapping a striker changes which swap is next best. Free transfers are spent
// first; beyond them a transfer costs four points and has to earn them back.
//
// The horizon shrinks toward the end of a season. A hit taken at GW36 has two
// gameweeks to repay itself, not five, and a policy that ignores that will keep
// buying form in May.
//
// The fourth and fifth returns are the banking and free-transfer-taper mediators'
// per-week contributions. They are returned rather than accumulated through a
// pointer so that this function still computes nothing it does not also return,
// and so a caller that ignores one — there are none today — cannot end up with a
// half-filled counter.
func decide(e *analysis.Engine, s *Season, held []int, w *wallet, free, gw int, cfg SimConfig) ([]int, int, []Move, weekBanking, weekHold) {
	bank := w.bank
	// Prices as they stood when the decision was made, which is the end of the
	// previous gameweek — the same point the engine's data comes from.
	priced := gw - 1
	sell := w.sellPrices(s, held, priced)
	market := func(id int) int { return marketPrice(s, id, priced) }
	if cfg.Oracles.Has(OracleTransactPrice) {
		// Perfect price timing, hindsight, off by default. See OracleTransactPrice.
		market = func(id int) int { return bestBuyPrice(s, id, priced) }
		sell = map[int]int{}
		for _, id := range held {
			sell[id] = bestSellPrice(w, s, id, priced)
		}
	}
	// What money freed by a move is worth, in points.
	//
	// A downgrade that banks £0.5m looks purely negative to a points model, and
	// it is not: the money can be converted into a better player, and it keeps
	// working until the season ends. That value decays — the same £0.5m banked
	// at GW35 has four gameweeks to be spent rather than thirty — which is
	// exactly the asymmetry the flat charge cannot express.
	//
	// This goes on the gain side, not into FreeTransferValue. That charge looks
	// like an opportunity cost and empirically is not one: pricing it at a
	// hit's four points scores below charging nothing, exempting an
	// about-to-expire transfer changes nothing, and tapering it late costs
	// points. What it actually filters is moves too small to tell from noise, so
	// it stays a flat threshold and money is valued separately.
	//
	// For a same-position swap the positional floor cancels — the slot must be
	// filled either way — so money freed is simply the selling price of the man
	// leaving less the price of the man arriving, and all of it is discretionary.
	moneyPts := func(freedTenths int) float64 {
		if budgetWeight <= 0 || freedTenths == 0 {
			return 0
		}
		return float64(freedTenths) * e.PointsPerTenth() *
			float64(e.GameweeksRemaining()) * budgetWeight
	}
	freedBy := func(outID, inID int) int { return sell[outID] - market(inID) }

	// settle moves money for real: the man leaving raises his selling price, the
	// man arriving costs his market price, and the wallet remembers what was
	// paid so his own sale is priced correctly later.
	//
	// **The sale settles at the price the search was quoted**, which is what makes
	// the price oracle a genuine upper bound rather than a mixture. Un-oracled the
	// two are the same number by construction — `sell` is `w.sellPrice` of the same
	// `marketPrice` this closure would recompute — so this is byte-identical on the
	// baseline. Under OracleTransactPrice they are not: the search is quoted
	// `bestSellPrice`, the window *maximum*, while `market` has been rebound to
	// `bestBuyPrice`, the window *minimum*. Settling the sale through `market`
	// therefore had the oracled arm buy low and sell low, promising money in the
	// gate that the wallet never received. Invisible on the baseline, because there
	// both sides collapse to one call.
	//
	// It goes through `sellAt` rather than `sell` because `bestSellPrice` is
	// already a *selling* price — a maximum over a window of `w.sellPrice` — and
	// `sell` would apply FPL's half-of-any-rise rule to it a second time.
	settle := func(mv Move) {
		settleSale(w, sell, mv.OutID, market(mv.OutID))
		w.buy(mv.InID, market(mv.InID))
		bank = w.bank
	}
	// This week's free transfer arrives, capped at the bank limit. It is the ONLY
	// accrual on this path — the wildcard and free-hit branches in Simulate do
	// their own because they return before reaching here.
	if free < cfg.BankUpTo {
		free++
	}

	// The gate charges `gain x horizon`, so which horizon it reads has to match
	// the one the score was formed on or the arm is mismatched in a known
	// direction. With AnticipateChips the engine's horizon has already been
	// shortened by ApplyChipPlan for the weeks before a chip, and scoring a move
	// on a one-to-four-week expectation while charging it over five over-credits
	// near-term fixture spikes by construction — which is enough on its own to
	// move transfer counts and fixture difficulty, with no option value involved.
	//
	// Reading e.Weights.Horizon rather than recomputing the shortening is
	// deliberate: EffectiveHorizon is the single definition of how long the squad
	// must serve, and a second copy of that rule here is the drift this package
	// keeps paying for.
	//
	// It **shortens** rather than replaces, and that distinction is not cosmetic.
	// `Horizon` used to do two jobs — the fixture-average window and the transfer
	// threshold — and `SimConfig.DecisionHorizon` exists precisely to separate
	// them. Assigning `e.Weights.Horizon` outright re-merged the two: a cell with
	// DecisionHorizon 3 and Weights.Horizon 5 would have gated at 5 all season,
	// silently, in every week no chip was near. Nothing sets both today, which is
	// the only reason this never fired. The `AnticipateChips` guard is what makes
	// the field comment's "no effect unless" literally true rather than
	// approximately true.
	gateHorizon := cfg.decisionHorizon()
	if cfg.AnticipateGate && cfg.AnticipateChips && e.Weights.Horizon < gateHorizon {
		gateHorizon = e.Weights.Horizon
	}
	horizon := effectiveHorizon(gateHorizon, gw)
	freeCost := cfg.FreeCost
	gainBar := cfg.MinGain
	// The scheduled floor, off by default and byte-identical when off. It
	// overrides the BASE charge and gain bar before the taper below scales
	// them, so a scheduled arm with the taper on pays the schedule's charge
	// through the taper's curve — schedule first, curve second. The
	// counterfactual above still re-prices against the shipped constants,
	// which is what makes a schedule arm's flips mean the schedule.
	if cfg.EarlyFloor.UntilGameweek > 0 && gw <= cfg.EarlyFloor.UntilGameweek {
		freeCost = cfg.EarlyFloor.FreeTransferValue
		gainBar = cfg.EarlyFloor.MinGainForTransfer
	}
	// The option-value taper, off by default and byte-identical when off.
	//
	// It reprices what a free transfer is CHARGED, from a constant to a function
	// of the season's remaining life and the squad's forward fixture congestion.
	// The two channels are the reserved exit (decay) and the insurance against
	// forced demand (congestion) — see analysis.TransferHoldFactor for which is
	// which, and for why neither makes this search forward-looking.
	//
	// ⚠️ The load is read from the clubs the squad ACTUALLY HOLDS, so it is a
	// property of the arm's own path and not of the calendar alone. Two arms in
	// one cell can therefore read different loads in the same gameweek, which is
	// normal for a mediator column and would be a fault in a dose column.
	var wh weekHold
	if cfg.TaperFreeTransferValue {
		wh.Consulted = true
		// Both through the one composite, so the replay and the three live sites
		// cannot read the load over different windows. It starts at gw+1: the
		// question is what a transfer is worth UNSPENT, and the decay half
		// excludes this gameweek for the same reason. See TransferHoldFactorFor.
		wh.Load = e.HoldingCongestion(held, gw, cfg.OptionPricing)
		wh.Factor = e.TransferHoldFactorFor(held, gw, cfg.OptionPricing)
		// The local freeCost, so a scheduled base flows through the curve —
		// schedule first, curve second, as the schedule's comment above
		// commits. When the schedule is off this is cfg.FreeCost and the
		// line is what it always was.
		freeCost = freeCost * wh.Factor
		wh.Charge = freeCost
	}
	// accept is the gate, plus the taper's counterfactual.
	//
	// Every accept expression in this function goes through it rather than
	// through `acceptTransfer` directly, so the flip count cannot describe a
	// different population from the decisions actually taken. The counterfactual
	// re-prices the SAME proposal at the untapered charge and asks
	// `gateDecision` — which is pure, where `acceptTransfer` logs, so asking
	// twice would otherwise double every row of the gate diagnostic's stream.
	//
	// ⚠️ Under a gate oracle `gateDecision` ignores the charge entirely, so the
	// flip count is necessarily zero there. That is a true reading of an arm in
	// which the taper cannot act, and it is why the count is measured rather
	// than assumed.
	accept := func(p transferProposal) bool {
		ok := acceptTransfer(cfg, s, p)
		// The gate-floor counterfactual, on every arm and every proposal: would
		// the SHIPPED gate have answered differently? Re-prices the same
		// proposal at the shipped constants and asks `gateDecision` — pure, like
		// the taper's counterfactual, and the same population as the decisions
		// actually taken. The hit branch carries no gain bar, and the
		// counterfactual preserves that rather than installing one.
		base := p
		base.FreeCost = config.Default().Review.FreeTransferValue
		if base.GainBar != noGainBar {
			base.GainBar = config.Default().Review.MinGainForTransfer
		}
		if gateDecision(cfg, s, base) != ok {
			if gw <= quietBoundaryGW {
				wh.FloorFlipsLe28++
			} else {
				wh.FloorFlipsGt28++
			}
		}
		if !wh.Consulted {
			return ok
		}
		wh.GateCalls++
		taperBase := p
		taperBase.FreeCost = cfg.FreeCost
		if gateDecision(cfg, s, taperBase) != ok {
			wh.Flips++
		}
		return ok
	}
	// What a planned chip adds to a squad's value this week. Zero unless one of
	// the preparation switches is on and its chip falls inside the horizon, so
	// every other arm is byte-identical.
	credit := cfg.chipCreditFor(e, gw, horizon)
	wh.Credit = credit

	var moves []Move
	hits := 0
	limit := moveLimit(free, cfg.MaxHits, cfg.MaxMoves, cfg.HitCeiling)
	// The allowance the search is about to run with: after this week's accrual,
	// before anything is spent. Captured beside moveLimit because it is the same
	// number moveLimit is taken from — recording it anywhere later would report
	// what survived the decision instead. See BankingMediator.FreeHeld.
	wb := weekBanking{Free: free}

	// Is next week's bigger allowance worth more than this week's move?
	if cfg.BankLookahead {
		// Counted here rather than from cfg, so that a future guard added in
		// front of shouldBank shows up as consulted weeks falling rather than as
		// a banking arm that quietly stops firing.
		wb.Consulted = true
		bankIt, weighed := shouldBank(e, held, bank, free, limit, gw, horizon, freeCost, gainBar, cfg, sell)
		wb.Weighed = weighed
		if bankIt {
			wb.Banked = true
			// Banking is NOT spending, and it is not a second grant either.
			//
			// ⚠️ This branch used to increment `free` again, on top of the weekly
			// accrual thirty lines above, so a banked week ended two transfers up
			// where FPL grants one. It was self-defeating rather than merely
			// generous: `shouldBank`'s first guard refuses once the allowance is at
			// `BankUpTo`, so an arm that manufactured allowance climbed to the
			// ceiling at double speed and then could never bank again — and every
			// later week's `free_at_decision` carried the inflation with nothing
			// recording where it came from. Nothing pinned the arithmetic, and the
			// only consumer is behind `BankLookahead`, which shipped off and has no
			// banked sweep, so no recorded figure moves.
			// TestABankedWeekAccruesExactlyOneTransfer is the pin.
			return held, free, nil, wb, wh
		}
	}

	// One search for both jobs, when enabled. See unified.go.
	if unifiedTransfers || cfg.Unified {
		moves, hits = unifiedDecide(e, s, held, bank, free, limit, gw, horizon, freeCost, cfg, sell)
		for _, mv := range moves {
			held = applyMove(held, mv)
			settle(mv)
		}
		free -= len(moves) - hits
		if free < 0 {
			free = 0
		}
		return held, free, moves, wb, wh
	}

	// A premium cannot be funded one swap at a time: the downgrade that pays for
	// him lowers the eleven on its own and is rejected before the upgrade is ever
	// considered. When there is room for two moves, look for the pair together.
	if limit >= 2 {
		// One transfer buys the premium; the rest can fund him. With the modern
		// five-transfer bank that is genuinely several sales, not just one.
		maxDowns := limit - 1
		if cfg.MaxFundingSales > 0 && cfg.MaxFundingSales < maxDowns {
			maxDowns = cfg.MaxFundingSales
		}
		if pair, ok := bestPair(e, held, bank, maxDowns, sell, credit); ok {
			n := len(pair.moves)
			// Every leg is judged on the single combined gain, since none of
			// them stands up alone — that is the whole point of grouping them.
			hitsNeeded := 0
			if free < n {
				hitsNeeded = n - free
			}

			// The alternative is never "do nothing": it is to spend the free
			// transfer on the best single move and keep the four points. So the
			// pair has to beat that, after paying for its own hits. Comparing
			// the raw gains instead had the policy buying a premium every time
			// one looked good, taking a -4 to do it, and the replay lost 43
			// points a season to hits that a single swap would have beaten.
			solo, _, _ := bestSwap(e, held, bank, sell, credit)
			soloValue := 0.0
			if solo.Gain*horizon >= freeCost && solo.Gain >= gainBar {
				soloValue = solo.Gain*horizon - freeCost
			}
			// Every leg is priced, hits explicitly and free transfers at what
			// they could have bought instead. Charging the week once rather
			// than per move was tried, on the argument that a funded pair is a
			// single decision: it scored 2110 against 2151, because two
			// transfers really are twice the scarce resource and pricing them
			// as one brought the churn back.
			pairMoney := 0.0
			for i := range pair.moves {
				pairMoney += moneyPts(freedBy(pair.moves[i].OutID, pair.moves[i].InID))
			}

			// The structural half — one hit at most, and the package must fit in
			// the week's allowance — is a legality question and stays here. Only
			// the *value* judgement goes through the gate, which is what an oracle
			// over the gate is entitled to overrule.
			// ⚠️ `hitsNeeded <= 1` was a LITERAL here, and it is the second
			// half of MoveLimit's clamp: lifting one without the other would
			// widen the limit and leave the funded pair refusing anything that
			// used the extra move. Both read cfg.HitCeiling now.
			ok := hitsNeeded <= cfg.hitCeiling() && n <= limit && accept(transferProposal{
				Moves: pair.moves, Gain: pair.gain, Money: pairMoney,
				Hits: hitsNeeded, Alternative: soloValue, Strict: true,
				GainBar: gainBar, Horizon: horizon, FreeCost: freeCost, GW: gw,
			})
			if ok {
				for i, mv := range pair.moves {
					mv.GW = gw
					mv.Gain = 0
					if i == 0 {
						mv.Gain = pair.gain // reported once, on the pair
					}
					if i < hitsNeeded {
						mv.Hit = true
						hits++
					}
					held = applyMove(held, mv)
					settle(mv)
					moves = append(moves, mv)
				}
				free -= n - hitsNeeded
				if free < 0 {
					free = 0
				}
				limit -= n
			}
		}
	}

	for range make([]struct{}, limit) {
		best, _, bestIn := bestSwap(e, held, bank, sell, credit)
		if bestIn.ID == 0 {
			break
		}

		money := moneyPts(freedBy(best.OutID, best.InID))
		useHit := free == 0
		// One move, so the package is the move. The alternative is doing nothing,
		// worth zero — unlike the funded pair, whose alternative is spending the
		// free transfer on the best single move.
		one := transferProposal{
			Moves: []Move{best}, Gain: best.Gain, Money: money,
			Horizon: horizon, FreeCost: freeCost, GW: gw,
		}
		switch {
		case !useHit && accept(one.withBar(gainBar)):
			free--
		case useHit && hits < cfg.MaxHits && accept(one.asHit()):
			best.Hit = true
			hits++
		default:
			// Nothing left worth doing this week.
			return held, free, moves, wb, wh
		}
		best.GW = gw
		held = applyMove(held, best)
		settle(best)
		moves = append(moves, best)
	}
	return held, free, moves, wb, wh
}

// applyMove returns the squad with one player swapped for another.
// shouldBank decides whether to make no transfer this week because a larger
// package next week is worth more.
//
// # The gap it closes
//
// A premium upgrade usually needs more than one move: the money is locked in a
// player of a different position, so buying him means selling a forward *and*
// funding the gap. RankPairs can express that and almost never gets the chance,
// because the weekly decision is greedy — it spends a free transfer the moment
// any move clears the gate, and the gate is low enough that small moves clear it
// constantly. TestDiagBanking puts 74% of weeks at zero or one transfer in hand,
// and five weeks in four seasons at three moves or more.
//
// The traced cost: in 2025-26 the model rated Haaland above Salah from GW7 and
// could not buy him until GW13, because the switch needed a forward sold and the
// capital freed, and nothing ever compared "spend one now" against "bank two and
// buy the premium".
//
// # What it does and does not assume
//
// It does not predict the future. It asks something answerable entirely from
// today's board: **what is the best package I could afford with one more
// transfer, and is it worth more than the best I can afford now, even after
// losing a gameweek of it?**
//
// Waiting is charged honestly. Next week's package earns over a horizon one
// shorter, because a week of the gain is gone, while this week's move is
// credited in full. So banking has to win on the *size* of what it unlocks
// rather than on optimism about timing.
//
// Both arms are priced on today's board, which is the approximation — next
// week's will differ. It is the same approximation every gain estimate here
// already makes, and it applies to both sides equally.
//
// # It has THREE false exits, and the second return value separates them
//
// Two are guards — the allowance already at its ceiling, and a horizon of one
// gameweek — and the third is `later > now` simply coming out false. That third
// exit hides a degenerate case: the package valuation returns 0 when nothing clears
// MinGain, so `0 > 0` is a refusal by a rule that had nothing to weigh, and it
// counts identically to a rule that weighed a real choice and preferred acting
// now. This record already holds that nothing clears the gain threshold at any
// price after GW28, so that case is common rather than exotic.
//
// `weighed` is true when the rule got past both guards and at least one arm was
// worth something — the population a banked-weeks count is a rate over. See
// BankingMediator.WeighedWeeks.
// ⚠️ **The guards, the comparison and the weighed rule are all
// analysis.AdviseBank's**, and the only thing left here is the enumeration.
// They were spelled out inline while the live path spelled them again, which is
// exactly the drift the extraction claims to have removed — left, on the first
// attempt, in the part of the rule that is judgement rather than arithmetic.
//
// The guards run BEFORE the arms are priced, because valuing them means two full
// transfer searches and the whole point of a guard is to decline before paying
// for that. AdviseBank re-checks them, which is idempotent and cheap.
func shouldBank(e *analysis.Engine, held []int, bank, free, limit, gw int,
	horizon, freeCost, minGain float64, cfg SimConfig, sell map[int]int) (bank_ bool, weighed bool) {

	// ⚠️ **`freeCost` here is the NOW-arm's charge, and the later arm is priced at
	// the same one.** With `TaperFreeTransferValue` on, next week's charge is lower
	// — one gameweek less window — so the later arm is charged too much and the
	// comparison leans toward acting.
	//
	// ⚠️ **The bound is ABSOLUTE, not relative.** A first draft said "under 2% of
	// the charge" and that is wrong: the per-week step is `h²/((r+h)(r+h-1))` times
	// the normalised charge, which is small in mid-season and **100%** at `r = 1`,
	// because next week's charge is exactly zero. What holds everywhere is the
	// absolute size — at the shipped half-life and `FreeCost` 2.0 the largest
	// one-week step is **0.358 points**, at GW37. Against a gate whose bar is a
	// package's whole horizon value, that is the right order to call negligible;
	// "2%" was the right conclusion from the wrong quantity.
	//
	// ⚠️ **The CONGESTION half is not bounded by that argument at all.** It reads a
	// different five-gameweek window at `gw+1`, and a window that drops a doubling
	// round can move the factor by a large fraction in one step. Unbounded here and
	// unmeasured.
	//
	// Left uncorrected deliberately: **both arms already price on today's board**,
	// and adding one forward-looking term to a comparison that is otherwise
	// entirely present-tense makes the approximation harder to state rather than
	// smaller. Named rather than fixed, so nobody reads it as an oversight.
	if guard := analysis.BankGuardFor(free, cfg.BankUpTo, horizon); guard != analysis.BankGuardNone {
		if cfg.bankLog != nil {
			cfg.bankLog(bankProbe{GW: gw, Guard: guard, Free: free,
				Limit: limit, Horizon: horizon})
		}
		return false, false
	}
	// The later arm is priced at *next* week's decision, so its chip credit is
	// too: a boost one week nearer is one week less to amortise it over, which is
	// the whole quantity this comparison turns on.
	limitLater := moveLimit(free+1, cfg.MaxHits, cfg.MaxMoves, cfg.HitCeiling)
	pkgNow := transferPackages(e, held, bank, limit, gw, horizon, cfg, sell)
	pkgLater := transferPackages(e, held, bank, limitLater, gw+1, horizon-1, cfg, sell)
	bestNow, now := analysis.BestPackage(pkgNow, horizon, freeCost, minGain)
	bestLater, later := analysis.BestPackage(pkgLater, horizon-1, freeCost, minGain)
	a := analysis.AdviseBank(free, cfg.BankUpTo, horizon, now, later)
	if cfg.bankLog != nil {
		cfg.bankLog(bankProbe{
			GW: gw, Free: free, Limit: limit, LimitLater: limitLater,
			Horizon: horizon, Now: now, Later: later,
			NowMoves: bestNow.Moves, LaterMoves: bestLater.Moves,
			// The two channels the comparison is made of, each with the other
			// held off. Both re-price package sets this call already enumerated,
			// so the probe costs no extra transfer search — which is what makes
			// it affordable to ask on every decision week.
			//
			// Each re-prices the OTHER arm's horizon onto a package set whose chip
			// credit was amortised over its own, so both carry the same caveat and
			// both are exact with the preparation switches off. See the field docs.
			NoHaircut:    analysis.BestPackageValue(pkgLater, horizon, freeCost, minGain),
			NoExtraMove:  analysis.BestPackageValue(pkgNow, horizon-1, freeCost, minGain),
			SamePackages: samePackages(pkgNow, pkgLater),
			Banked:       a.Bank,
			Weighed:      a.Weighed(),
		})
	}
	return a.Bank, a.Weighed()
}

// bankProbe is one week's banking comparison, opened up far enough to attribute
// its answer to a channel. Nothing branches on it and nothing is computed for it
// that the decision did not already compute.
//
// # Why the two counterfactual arms are here rather than in a test
//
// The shipped comparison differs from its own alternative in exactly two ways —
// the later arm gets ONE MORE MOVE and ONE FEWER GAMEWEEK of gain — and a count
// of how often it fired cannot say which of the two decided it. Reconstructing
// them outside `shouldBank` would mean a second copy of the enumeration, in the
// one place this package least wants one: a diagnostic is what everything else is
// checked against.
type bankProbe struct {
	// GW is the gameweek the decision was taken in.
	GW int
	// Guard is why the comparison was never made, or BankGuardNone.
	//
	// ⚠️ **A guarded probe still carries Free, Limit and Horizon** — those are the
	// state the guard was applied to and they cost nothing — and only the priced
	// fields below are zero, because a guard refuses before any package is valued.
	// The reachability diagnostic depends on that: it sums the allowance over the
	// unguarded weeks and would silently average in zeros if this literal were
	// trimmed to match a comment claiming everything is blank.
	Guard analysis.BankGuard
	// Free is the allowance the search ran with, and Limit/LimitLater the move
	// limits the two arms received — `MoveLimit`'s output, which is what the
	// hypothesis about `free + hits` is about.
	Free, Limit, LimitLater int
	// Horizon is what the now-arm was valued over; the later arm got one fewer.
	Horizon float64
	// Now and Later are the two arms as AdviseBank saw them.
	Now, Later float64
	// NowMoves and LaterMoves are how many transfers each arm's winning package
	// spends, or 0 where nothing cleared the floor.
	NowMoves, LaterMoves int
	// NoHaircut is the later arm with the horizon cost removed: next week's
	// package set, priced over the full horizon. It isolates what the EXTRA MOVE
	// buys, so `NoHaircut > Now` is the weeks where waiting would win if it were
	// free.
	//
	// NoExtraMove is the mirror: this week's package set priced over the short
	// horizon, isolating the haircut. It can never exceed Now — same packages,
	// less horizon, and PackageValue is increasing in horizon — so it sizes the
	// haircut rather than testing it.
	//
	// ⚠️ **NoHaircut is NOT sign-constrained against Now.** `bestPair` returns
	// `RankPairs(...)[0]`, an argmax over **gain**, while `PackageValue` charges
	// `freeCost` per **move** — so a wider limit can substitute a higher-gain,
	// costlier package that is worth less once charged. `NoHaircut < Now` is a
	// real observation and the diagnostic counts it rather than assuming it away.
	//
	// ⚠️ **The chip caveat applies to BOTH, not only to NoHaircut.** Each
	// re-prices a package set whose gains carry a chip credit amortised over the
	// other arm's horizon, so with either preparation switch on both are off by
	// one week's amortisation. With both off — every arm this probe has been run
	// on — `chipCreditFor` returns zero and both re-pricings are exact.
	NoHaircut, NoExtraMove float64
	// SamePackages is whether the two arms enumerated the identical candidate
	// list, which separates the two ways the extra-move channel can read zero.
	//
	// ⚠️ **This is the distinction the counterfactual on its own cannot make.**
	// `RankPairs` builds a multi-downgrade set only for upgrades that no single
	// funding sale can reach (`if single || maxDowns < 2 { continue }`), and
	// `bestPair` returns `pairs[0]` ranked on **gain** before the caller prices it
	// on value. So a wider limit can enumerate nothing new at all, or enumerate
	// something that tops the gain ranking and then loses on value — and those are
	// a *structural* inertness and a *football* one. Without this field an
	// extra-move channel of zero pools them.
	SamePackages bool
	// Banked and Weighed are what shouldBank returned.
	Banked, Weighed bool
}

// samePackages reports whether two enumerations produced the identical candidate
// list. Order matters and is stable: `transferPackages` appends the solo swap
// then the pair, so a difference is a difference in what was found.
func samePackages(a, b []analysis.TransferPackage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// transferPackages is what the best set of moves up to `limit` could be, as
// candidate packages — the enumeration on its own, without the valuation.
//
// The enumeration is this package's — it prices sales through the wallet, which
// knows what each player was bought for — and the *valuation* is
// analysis.BestPackage, which the live transfer command shares through its
// BestPackageValue face. Splitting it
// that way is deliberate: the two callers legitimately find their candidates by
// different routes, and the thing they must not disagree about is what a
// candidate is worth once found.
//
// It returns the packages rather than the winning value because the banking probe
// needs to price ONE package set over two different horizons, which is the only
// way to separate the horizon channel from the move-limit channel without
// enumerating twice. A wrapper folding the two back together would either cost a
// second full transfer search per decision week — the search is the expensive part
// of a replay — or put a second copy of the enumeration in a diagnostic, which is
// the thing a diagnostic may least carry.
//
// The `horizon` argument reaches the packages only through the chip credit, which
// is amortised over it. Everything else about a candidate is horizon-free.
func transferPackages(e *analysis.Engine, held []int, bank, limit, gw int,
	horizon float64, cfg SimConfig, sell map[int]int) []analysis.TransferPackage {

	if limit < 1 {
		return nil
	}
	credit := cfg.chipCreditFor(e, gw, horizon)
	var packages []analysis.TransferPackage
	if solo, _, in := bestSwap(e, held, bank, sell, credit); in.ID != 0 {
		packages = append(packages, analysis.TransferPackage{Gain: solo.Gain, Moves: 1})
	}
	if limit >= 2 {
		maxDowns := limit - 1
		if cfg.MaxFundingSales > 0 && cfg.MaxFundingSales < maxDowns {
			maxDowns = cfg.MaxFundingSales
		}
		if pair, ok := bestPair(e, held, bank, maxDowns, sell, credit); ok {
			packages = append(packages,
				analysis.TransferPackage{Gain: pair.gain, Moves: len(pair.moves)})
		}
	}
	return packages
}

func applyMove(held []int, mv Move) []int {
	out := make([]int, 0, len(held))
	for _, id := range held {
		if id == mv.OutID {
			out = append(out, mv.InID)
			continue
		}
		out = append(out, id)
	}
	return out
}

// HitCost is what a transfer beyond the free ones costs, in points.
const HitCost = 4.0

// DefconScoredIn reports whether defensive contribution paid points in a season.
//
// FPL introduced the category for 2025-26: two points for ten defensive actions
// by a defender, twelve by anyone else. Awarding it in an earlier replay would
// be scoring a rule nobody played under, in exactly the way BankLimitFor exists
// to prevent for the transfer bank.
//
// The archive enforces it today by accident — it carries no defensive_contribution
// before 2025-26, so there is nothing to award. That is not a guarantee. The
// underlying actions were always recorded and a future backfill would silently
// hand three replayed seasons a scoring category they never had, which is the
// quiet kind of wrong this file is full of. Hence an explicit gate.
func DefconScoredIn(season string) bool { return season >= "2025-26" }

// BankLimitFor is how many free transfers could be banked in a given season.
//
// FPL raised the cap from 2 to 5 for 2024-25. Replaying 2023-24 under the
// modern rule would let the policy save up transfers it could not actually have
// saved, and every conclusion drawn from that season would be about a game
// nobody played.
//
// **It cannot express 2019-20 and does not try.** See TransferPathComparable.
func BankLimitFor(season string) int {
	if season < "2024-25" {
		return 2
	}
	return 5
}

// TransferPathComparable reports whether a season's transfer path and wallet are
// samples of the same process as every other season's.
//
// **2019-20 is the one that is not, and no bank limit can express it.** FPL granted
// *unlimited* free transfers before the GW30+ deadline after the COVID restart, and
// froze prices for three months either side of it. So that season's transfer decisions
// were taken under a rule with no analogue — BankLimitFor("2019-20") answers 2, which
// is right for the first twenty-nine gameweeks and meaningless for the rest — and its
// team value did not move while every other season's did.
//
// Its **scoring is fine**, which is exactly the split this package already draws between
// its two metrics: `HOLD` re-picks the eleven and the captain weekly and makes no
// transfers, so it is unaffected by any of this, while `POLICY` is the transfer decision
// and is not. That is why 2019-20 can extend the season axis for scoring constants and
// not for transfer constants, and why extendedPairNames uses it only as a prior.
//
// It exists as a function rather than a paragraph because this package's record is
// mostly of facts that were true, written down, and then violated — a hand-maintained
// override list that outlived its situation, a cache version that was not a schema
// check, a bank rule that needed BankLimitFor in the first place. A season added to a
// POLICY grid on a Tuesday will not have this comment read to it.
func TransferPathComparable(season string) bool { return season != "2019-20" }

// The price oracle: OracleTransactPrice gives every transfer the best price
// available around the decision, which is hindsight and is off by default.
//
// It answers one question: **what is perfect price timing worth?** A manager who
// moves the moment a gameweek ends never pays a rise and never eats a fall,
// where one who waits for the deadline does both. That is the entire economic
// case for acting at 2am rather than at breakfast, and for a price forecast at
// all, so it is worth knowing the ceiling before building toward it.
//
// It is a *timing* oracle, not a knowledge oracle: the policy still chooses the
// same players from the same information, and only the prices it transacts at
// change. So the gap it opens is what price movement alone is worth, cleanly
// separated from knowing who will score.
//
// Deliberately generous — buys at the cheapest price in the surrounding window
// and sells at the dearest — so the answer is an upper bound rather than an
// estimate. If a bound this loose is small, nothing tighter can be large.
//
// It was a package-level var read from FPL_ORACLE_PRICES at init, which meant
// measuring it needed two processes and a comparison by eye, and a diagnostic
// that wanted both arms in one run had to mutate a global from a variant's apply
// and restore it in a defer that a panic mid-sweep would skip. It is now a bit on
// SimConfig.Oracles, seeded from the same environment variable where a cell
// config is constructed. Never make it the default: every figure in AGENTS.md was
// measured without it, the same rule OracleAvailability carries.

// The chip-week oracle: AxisChipWeek says which gameweek a scoring chip is
// played in, knowing what every week turned out to be worth.
//
// # Why the signature is an index and not a plan
//
// chipWeekOracle may return one int, chosen from a slice the replay has already
// finished computing. That return type *is* the confinement the one-axis rule
// asks for: it cannot move a transfer, buy a player, pick an eleven or hand the
// armband to anyone, because an index into an existing slice is the only thing it
// can express. Compare the alternative — a generic "decision oracle" taking the
// whole simulation state — which would satisfy the rule only by comment.
//
// # It changes nothing, which is the point
//
// Week.BenchBoostGain and Week.TripleCaptainGain are recorded every gameweek and
// against the *unchipped* week, so the oracle reads a slice that already exists
// and no chip is actually played. AxisChipWeek therefore declares that every
// collected metric must be byte-identical to the un-oracled baseline — the
// strongest invariance in the catalogue, and the reason this axis is built first.
// If any of them moves, the argmax has reached the simulation and the figure is
// not about chip timing.
//
// The figure it bounds is chip *timing*: oracle minus the best an honest policy
// could do, since a policy that cannot see the rest of the season must take the
// first week clearing a bar. What it does **not** bound is chip *preparation* —
// bench boost pays all fifteen and this measures it on a squad built under an
// objective that credits the bench at almost nothing. That gap is real, is
// recorded in the chips note, and no argmax over these gains can see it.
type chipWeekOracle func(gains []int) int

// bestChipWeek is the shipped chip-week oracle: the argmax, first week on a tie.
// Returns -1 for an empty season, which the caller reads as "no chip to place".
func bestChipWeek(gains []int) int {
	best := -1
	for i, g := range gains {
		if best < 0 || g > gains[best] {
			best = i
		}
	}
	return best
}

// placeChips runs the chip-week oracle over a played season.
//
// It reads res.Weeks and writes res.ChipOracle. Nothing else, by construction:
// the season is over by the time it is called.
func placeChips(weeks []Week, pick chipWeekOracle) *ChipOracle {
	at := func(gain func(Week) int) ChipWeek {
		gains := make([]int, 0, len(weeks))
		for _, w := range weeks {
			gains = append(gains, gain(w))
		}
		i := pick(gains)
		if i < 0 || i >= len(weeks) {
			return ChipWeek{}
		}
		return ChipWeek{GW: weeks[i].GW, Gain: gains[i]}
	}
	return &ChipOracle{
		BenchBoost:    at(func(w Week) int { return w.BenchBoostGain }),
		TripleCaptain: at(func(w Week) int { return w.TripleCaptainGain }),
	}
}

// The armband oracle: AxisArmband captains whoever actually scored most, from
// the eleven the model itself picked.
//
// # The signature is the confinement
//
// It is handed the eleven and may return two ids, both of which must already be
// in it. So it cannot change who starts, who is bought, who is sold or how the
// season is scored — the only thing it can express is which two of eleven names
// wear the armband, which is exactly one axis. `decide` never reads the captain
// at all, which is why AxisArmband can pin transfer count and hits as
// invariances: those are integers counted without noise, where a fraction of a
// point a gameweek is invisible.
//
// # It bounds captain and vice jointly, not the captain alone
//
// FPL passes the armband to the vice-captain whenever the captain records no
// minutes. An oracle captain necessarily played — he is chosen from the players
// who did — so under this oracle the fallback never fires. The bound is
// therefore over "who wears the armband", both names at once, and not over the
// captain alone. The oracle-design document asks for that to be said in the report
// rather than discovered, and this is where it is said.
//
// **The accounting runs the other way from the first version of this comment,
// which said the figure "includes the ~16 points a season the vice rule is
// separately worth".** It does not: viceCaptainFallback (replay.go) defaults
// **on**, so the *baseline* arm already banks the vice bonus in the 9.6% of
// weeks its captain blanks. The oracle arm has no use for it. So 210 is measured
// **net of** the vice rule — the gain over a baseline that already has it — and
// against an FPL_NO_VICE_CAPTAIN=1 baseline the same oracle would read about 226.
//
// The operational conclusion is unchanged and is the half that matters: a later
// refinement of the vice rule draws on this same 210 and must **not** be bounded
// separately, because both names are already inside this axis. Commit 0102d0d's
// message carries the inverted arithmetic; history is not rewritten, so this
// note is the correction.
//
// # One pathological case, not modelled
//
// If every eleven-member who played finished the week on zero or fewer points,
// the true optimum is to double *nobody*, which FPL permits only by naming two
// players who both blanked. This picks the best available scorer instead. It
// needs an eleven in which nobody who played scored a point — a player who
// appears at all banks one — so it is worth a fraction of a point in a season
// that will not occur, and modelling it would cost the property above that an
// oracle captain always played.
type armbandOracle func(cur *Season, xi []int, gw int) (captain, vice int)

// bestArmband is the shipped armband oracle: the two highest realised scorers in
// the eleven who actually recorded minutes.
//
// Ties break on the lower element id, so the choice is deterministic. That
// matters more than it looks: this package's cells run concurrently and a
// map-iteration tiebreak has already made Optimize non-deterministic once.
func bestArmband(cur *Season, xi []int, gw int) (captain, vice int) {
	type scored struct {
		id, pts int
	}
	var played []scored
	for _, id := range xi {
		p := cur.Players[id]
		if p == nil {
			continue
		}
		g := p.GWs[gw]
		if g.Minutes == 0 {
			continue
		}
		played = append(played, scored{id: id, pts: g.Points})
	}
	sort.Slice(played, func(i, j int) bool {
		if played[i].pts != played[j].pts {
			return played[i].pts > played[j].pts
		}
		return played[i].id < played[j].id
	})
	// Nobody played: nothing can be doubled whoever is named, so name the eleven's
	// own first two rather than zero — zero is "no captain", a third rung of the
	// held metric, and quietly returning it here would silently score this week
	// under a different rule.
	if len(played) == 0 {
		if len(xi) > 1 {
			return xi[0], xi[1]
		}
		if len(xi) == 1 {
			return xi[0], xi[0]
		}
		return 0, 0
	}
	captain = played[0].id
	vice = captain
	if len(played) > 1 {
		vice = played[1].id
	}
	return captain, vice
}

// oracleWindow is how many gameweeks either side of the decision the price
// oracle may reach. Two is enough to cover a move made immediately after one
// gameweek for a deadline a week later.
const oracleWindow = 2

// settleSale banks a sale at the price the search was quoted for it.
//
// A named function rather than four lines inside `settle`, because the property
// it enforces is the one that makes the price oracle a bound rather than a
// mixture, and it is invisible at the call site: un-oracled, the quote and the
// recomputation are the same number, so the two forms are indistinguishable on
// every figure this project has ever recorded.
//
// Under OracleTransactPrice they are not the same number. The search is quoted
// `bestSellPrice` — the window *maximum*, already a selling price — while `market`
// is rebound to `bestBuyPrice`, the window *minimum*. Settling through `market`
// therefore made the oracled arm buy low and sell low: it promised money in the
// gate that the wallet never received, so the arm was neither an upper bound nor
// a coherent policy. `sellAt` rather than `sell` because the quote has already
// paid FPL's half-of-any-rise rule and `sell` would charge it twice.
//
// `market` is still the fallback, and it is not dead: `quoted` is built from the
// squad held at the top of the week, so a player bought earlier in the same week
// and sold later in it has no quote. Settling him at the map's zero would be a
// silent behaviour change on the un-oracled path, which must stay byte-identical.
func settleSale(w *wallet, quoted map[int]int, id, market int) int {
	if got, ok := quoted[id]; ok {
		return w.sellAt(id, got)
	}
	return w.sell(id, market)
}

// bestBuyPrice is the cheapest this player was around the decision.
func bestBuyPrice(s *Season, id, gw int) int {
	best := marketPrice(s, id, gw)
	if best == 0 {
		return 0
	}
	for g := gw - oracleWindow; g <= gw+oracleWindow; g++ {
		if g < 1 || g > 38 {
			continue
		}
		if v := marketPrice(s, id, g); v > 0 && v < best {
			best = v
		}
	}
	return best
}

// bestSellPrice is the most this player would have raised around the decision,
// still paying FPL's half-of-any-rise rule on the way out.
func bestSellPrice(w *wallet, s *Season, id, gw int) int {
	best := w.sellPrice(id, marketPrice(s, id, gw))
	for g := gw - oracleWindow; g <= gw+oracleWindow; g++ {
		if g < 1 || g > 38 {
			continue
		}
		if m := marketPrice(s, id, g); m > 0 {
			if v := w.sellPrice(id, m); v > best {
				best = v
			}
		}
	}
	return best
}

// effectiveHorizon is how many gameweeks a transfer has left to repay itself.
//
// A hit taken at GW36 has three gameweeks to earn back its four points, not the
// configured five. Without this the policy keeps buying form in May, when there
// is no season left for it to pay off in.
func effectiveHorizon(configured, gw int) float64 {
	h := float64(configured)
	if left := float64(38 - gw + 1); left < h {
		h = left
	}
	if h < 1 {
		h = 1
	}
	return h
}

// moveLimit is how many transfers the policy may make in one week: every free
// transfer plus one, unless capped.
//
// The "plus one" is a single hit. Two hits in a week is an edge case — it needs
// a specific reason, usually an injury, and that is the judgement layer's job
// rather than something a scoring model should go looking for. Allowing it here
// mostly widened the search space so the policy could find expensive ways to
// chase noise: on three replayed seasons the two-hit policy never won.
// ⚠️ Moved to analysis.MoveLimit and delegated, because the live transfer
// command needs the same arithmetic to ask the same banking question. The
// reasoning above is kept for the replay's reader; it lives with the code there.
//
// ⚠️ The "plus one" is now `SimConfig.HitCeiling`, zero meaning
// `analysis.DefaultHitCeiling` — so every existing arm is byte-identical and an
// arm wanting the two-hit week has somewhere to set it. See MoveLimit.
func moveLimit(free, maxHits, maxMoves, hitCeiling int) int {
	return analysis.MoveLimit(free, maxHits, maxMoves, hitCeiling)
}

// hitCeiling resolves SimConfig.HitCeiling's zero to the shipped default, so the
// funded-pair branch and MoveLimit cannot disagree about what an unset field means.
func (c SimConfig) hitCeiling() int {
	if c.HitCeiling <= 0 {
		return analysis.DefaultHitCeiling
	}
	return c.HitCeiling
}

// chipCredit prices a chip planned inside this decision's horizon, per gameweek.
//
// # It is amortised over the gate's horizon, and that is not a free choice
//
// The gate charges `gain x horizon`, so a gain expressed per gameweek is worth
// its horizon multiple by the time it is compared with a transfer's cost. A chip
// pays *once*: the bench scores in one week, the armband triples in one week. So
// the per-gameweek credit is the one-off premium divided by the same horizon the
// gate is about to multiply it back by, and the two cancel to what the chip
// actually pays. Reading a different horizon here — `Weights.Horizon`, or the
// unshortened `DecisionHorizon` — would credit the chip a multiple of its own
// value, and would do it in exactly the weeks the arm is being judged on.
//
// The window is closed at the near end and open at the far one: a chip in *this*
// gameweek still counts, because a transfer made now plays in it.
//
// It reads `Chips` rather than `ChipPlanner` for the same reason `decide` reads
// `e.Weights.Horizon`: `Simulate` resolves the planner into `cfg.Chips` once, at
// the top, and a second resolution here would be a second copy of the placement
// rule.
// # A wildcard between here and the chip ends the window
//
// The squad being valued does not survive a wildcard: `playWildcard` replaces all
// fifteen. So a credit that reached past one would spend this week's free
// transfers buying a bench for a fifteen that is about to be torn up, and the
// preparation it paid for would arrive in the rebuild for nothing. That is not
// hypothetical — it is the shape of the wildcard-into-boost sequence, which is
// the play the chip actually lives in and the next arm anyone will run.
//
// `analysis.EffectiveHorizon` already stops at a wildcard for the opening-squad
// path, and this window did not, which is one quantity computed two ways. The
// `AnticipateChips`/`AnticipateGate` pair happens to mask it by shortening the
// gate horizon, but only when both are set, and neither is set in the sweep this
// was measured on.
//
// The free hit is deliberately *not* a barrier. It fields a temporary fifteen for
// one week and hands the permanent squad straight back, so a chip beyond it is
// still played by the squad being valued now.
// ⚠️ **The rule moved to analysis.ChipCreditAt and this is now a delegation.**
// It had to: the live transfer command needs the same window, and writing it a
// second time there is the failure this package is named for. The doc above is
// kept because it is what the *replay* reads it for; the rule's own reasoning —
// why it amortises over the gate's horizon, why a wildcard walls it and a free
// hit does not, why it asks across both chip sets — lives with the code in
// internal/analysis/banking.go.
func (c SimConfig) chipCredit(gw int, horizon float64) analysis.ChipCredit {
	return analysis.ChipCreditAt(c.schedule(),
		c.PrepareBenchBoost, c.PrepareTripleCaptain, gw, horizon)
}

// wildcardBuildsForBoost is whether a wildcard rebuilding the week before a bench
// boost optimises the fifteen for the chip.
//
// The package-level variable is the global escape hatch and the config field is
// the per-cell one; either turning it off turns it off. A sweep must use the
// field, because an arm cannot vary a global without leaving it set for every arm
// that follows.
func (c SimConfig) wildcardBuildsForBoost() bool {
	return wildcardBuildsForBoost && !c.WildcardIgnoresBoost
}

// chipCreditFor is chipCredit plus the chip week's own per-club fixture counts,
// which need the engine and so cannot live on the pure config method the unit
// tests pin.
//
// Only the bench channel needs them: it values fifteen players in one particular
// gameweek, where the triple captain's armband is already the eleven's own
// maximum and carries whatever fixture load the eleven was scored on.
func (c SimConfig) chipCreditFor(e *analysis.Engine, gw int, horizon float64) analysis.ChipCredit {
	cr := c.chipCredit(gw, horizon)
	if cr.Bench > 0 {
		// The same bench boost chipCredit just credited, not the field — with two
		// sets those differ, and loading the wrong week's fixture counts prepares
		// the squad for the wrong gameweek while reporting success.
		cr.WeekLoad = e.FixtureCountsIn(c.nextChip(slotBenchBoost, gw))
	}
	return cr
}

// pairedMove is a downgrade and an upgrade evaluated as one decision.
type pairedMove struct {
	moves []Move
	out   []analysis.PlayerMetrics
	in    []analysis.PlayerMetrics
	gain  float64
}

// bestPair finds the best downgrade-plus-upgrade combination available.
//
// The search itself lives in analysis.RankPairs, so the replay and the live
// agent tool cannot drift apart on it. See internal/analysis/swaps.go for why a
// premium is unreachable one swap at a time.
func bestPair(e *analysis.Engine, held []int, bank, maxDowns int, sell map[int]int,
	credit analysis.ChipCredit) (pairedMove, bool) {

	squad := squadMetrics(e, held)
	if len(squad) < 15 {
		return pairedMove{}, false
	}
	st := analysis.NewSquadState(squad)
	st.Sell = sell
	st.Chip = credit
	pairs := analysis.RankPairs(st, e.AllMetrics(), bank, maxDowns, 1)
	if len(pairs) == 0 {
		return pairedMove{}, false
	}
	p := pairs[0]
	// The upgrade is listed first so the report reads as the decision it was:
	// buy the premium, then fund it from elsewhere.
	pm := pairedMove{gain: p.Gain}
	add := func(sw analysis.Swap) {
		pm.moves = append(pm.moves, Move{
			Out: sw.Out.Name, In: sw.In.Name, OutID: sw.Out.ID, InID: sw.In.ID,
			OutScore: sw.Out.Score, InScore: sw.In.Score,
		})
		pm.out = append(pm.out, sw.Out)
		pm.in = append(pm.in, sw.In)
	}
	add(p.Up)
	for _, d := range p.Downs {
		add(d)
	}
	return pm, true
}

// squadMetrics scores the held players at the current point in the season.
func squadMetrics(e *analysis.Engine, held []int) []analysis.PlayerMetrics {
	var squad []analysis.PlayerMetrics
	for _, id := range held {
		if el := e.Boot.ElementByID(id); el != nil {
			squad = append(squad, e.Metrics(el))
		}
	}
	return squad
}

// bestSwap finds the single most valuable legal transfer available right now,
// scored by what it does to the eleven rather than to the player.
//
// This originally ranked by in.Score - out.Score. Replaying 2025-26 under that
// objective, 16 of 22 transfers sold a player who was not in the eleven and four
// moved the eleven by exactly zero — one of them a -4 hit for a keeper who never
// started. The ranking was inverted where it mattered: Tuanzebe to Gudmundsson
// scored +2.89 and bought +0.83 of eleven, while selling a declining Salah
// scored +0.60 and bought all +0.60 of it.
func bestSwap(e *analysis.Engine, held []int, bank int, sell map[int]int,
	credit analysis.ChipCredit) (Move, analysis.PlayerMetrics, analysis.PlayerMetrics) {

	squad := squadMetrics(e, held)
	st := analysis.NewSquadState(squad)
	st.Sell = sell
	st.Chip = credit
	swaps := analysis.RankSwaps(st, e.AllMetrics(), bank)
	if len(swaps) == 0 {
		return Move{}, analysis.PlayerMetrics{}, analysis.PlayerMetrics{}
	}
	b := swaps[0]
	return Move{
		Out: b.Out.Name, In: b.In.Name, Gain: b.Gain,
		OutScore: b.Out.Score, InScore: b.In.Score,
		OutID: b.Out.ID, InID: b.In.ID,
	}, b.Out, b.In
}

// pickXI chooses the best legal eleven and captain from a squad, by the model's
// scores at that moment.
// checkChipPlan rejects a plan FPL would not allow.
//
// Only one chip may be played per gameweek. `analysis.ValidateChipPlan` already
// knows this, but it validates against the *live* bootstrap's chip windows and
// also reports unplanned chips as problems, so it cannot be used to gate a
// replay. The rule that actually changes what a season scores is re-checked
// here — without it the replay will happily play a wildcard and a bench boost
// in the same week, which is exactly the illegal sequence the first version of
// TestDiagChipSequence measured.
func checkChipPlan(p analysis.ChipPlan) error {
	byGW := map[int][]string{}
	for name, gw := range map[string]int{
		"wildcard": p.Wildcard, "free hit": p.FreeHit,
		"bench boost": p.BenchBoost, "triple captain": p.TripleCaptain,
	} {
		if gw > 0 {
			byGW[gw] = append(byGW[gw], name)
		}
	}
	for gw, names := range byGW {
		if len(names) > 1 {
			sort.Strings(names)
			return fmt.Errorf("only one chip may be played per gameweek: GW%d has %s",
				gw, strings.Join(names, " and "))
		}
	}
	return nil
}

// playWildcard rebuilds the squad outright and settles the money, returning the
// new fifteen and how many players changed.
//
// The wildcard is not a transfer decision. It is the opening-squad problem
// solved again at whatever the manager is now worth, with no move limit and no
// hit cost — so it goes through Optimize rather than through decide().
//
// Two things it must get right, both of which are easy to get wrong here.
// **The budget is selling value plus bank**, not squad market value: FPL pays
// only half of any rise, so a squad whose prices have gone up cannot be
// rebought at its headline value. `wallet.value` already computes exactly that.
// And **the wallet has to be settled player by player** — every player sold at
// his selling price and every new one bought at market — or the purchase prices
// that drive future selling prices are wrong for the rest of the season.
//
// If the bench boost is played the NEXT week, the rebuild is told so, and the
// objective then counts all fifteen. That is the sequence the chip is actually
// used in, and it is the whole reason the wildcard is worth modelling: without
// it the bench boost is measured on a squad built to have four non-players.
func playWildcard(e *analysis.Engine, cur *Season, w *wallet, held []int, gw int,
	minExp float64, cfg SimConfig) ([]int, int, error) {

	budget := w.value(cur, held, gw-1)
	// FPL allows one chip per gameweek, so a wildcard can never *be* the bench
	// boost week — it prepares for the one after it, which is the sequence the
	// chip is actually used in. A boost further out than that is left to
	// ordinary transfers and to SuggestBenchWeight's amortised weighting.
	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: budget, MinMinutes: 600, MinExpectedMinutes: minExp,
		BenchWeight: cfg.openingBenchWeight(),
		BenchBoost:  cfg.wildcardBuildsForBoost() && cfg.plays(slotBenchBoost, gw+1),
	})
	if err != nil {
		return nil, 0, err
	}

	next := make([]int, 0, 15)
	for _, p := range sq.Players {
		next = append(next, p.ID)
	}
	keep := map[int]bool{}
	for _, id := range next {
		keep[id] = true
	}
	had := map[int]bool{}
	for _, id := range held {
		had[id] = true
	}

	changed := 0
	for _, id := range held {
		if !keep[id] {
			w.sell(id, marketPrice(cur, id, gw-1))
			changed++
		}
	}
	for _, id := range next {
		if !had[id] {
			w.buy(id, marketPrice(cur, id, gw-1))
		}
	}
	return next, changed, nil
}

// freeHitSquad builds the temporary fifteen a free hit fields for one gameweek.
//
// It is deliberately *not* applied to held or to the wallet. FPL hands the
// permanent squad back the following week, so a free hit changes what is scored
// this week and nothing else — including the purchase prices, which must keep
// describing the squad the manager still owns.
//
// The caller supplies a one-gameweek engine; see the call site for why.
//
// # Clubs that do not play are excluded, and that is a selection guard
//
// The chip is spent on a blank round — 2023-24 GW29 blanked twelve clubs of
// twenty — so "who plays at all" is most of the problem. Scoring alone does not
// answer it. `fixtureLoadFor` now takes a blanking club's Score to zero, which
// keeps its players out of the ELEVEN, but the builder still has four bench
// slots to fill and is indifferent between two footballers worth nothing, so it
// takes whoever is cheapest. Pre-fix, with the load blind to blanks entirely,
// that GW29 build held thirteen blanking players and would have fielded TWO.
//
// So the guard has to reach the pool, not just the objective. This is the same
// shape as the one in `WeekViews`, which zeroes a blanking player's score *after*
// calling Optimize — correct for the display and too late for the selection.
func freeHitSquad(e *analysis.Engine, cur *Season, w *wallet, held []int, gw int,
	minExp float64, cfg SimConfig) ([]int, error) {

	sq, err := e.Optimize(analysis.OptimizeRequest{
		Budget: w.value(cur, held, gw-1), MinMinutes: 600,
		MinExpectedMinutes: minExp, BenchWeight: cfg.openingBenchWeight(),
		ExcludeIDs: e.ElementsWithoutFixtures(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, 15)
	for _, p := range sq.Players {
		out = append(out, p.ID)
	}
	return out, nil
}

func pickXI(e *analysis.Engine, held []int) (xi, bench []int, captain, vice int) {
	var squad []analysis.PlayerMetrics
	for _, id := range held {
		if el := e.Boot.ElementByID(id); el != nil {
			squad = append(squad, e.Metrics(el))
		}
	}
	chosen, benched, _ := analysis.BestXI(squad)
	for _, p := range chosen {
		xi = append(xi, p.ID)
	}
	for _, p := range benched {
		bench = append(bench, p.ID)
	}
	captain, vice = captainAndVice(chosen)
	return xi, bench, captain, vice
}

// captainAndVice ranks a picked XI by Score and returns the top two, the same
// ranking xiValueShrunk uses — so the replay's captain choice and the
// objective that scored the squad never disagree about who wears the armband.
func captainAndVice(pick []analysis.PlayerMetrics) (captain, vice int) {
	var capScore, viceScore float64
	for _, p := range pick {
		switch {
		case p.Score > capScore:
			viceScore, vice = capScore, captain
			capScore, captain = p.Score, p.ID
		case p.Score > viceScore:
			viceScore, vice = p.Score, p.ID
		}
	}
	return captain, vice
}

func idsToPlayers(s *Season, ids []int) []*Player {
	var out []*Player
	for _, id := range ids {
		if p := s.Players[id]; p != nil {
			out = append(out, p)
		}
	}
	return out
}

func squadValue(s *Season, ids []int, gw int) int {
	total := 0
	for _, id := range ids {
		total += marketPrice(s, id, gw)
	}
	return total
}

// Priors adapts a completed season to the interface the scoring engine wants,
// so a replay shrinks toward last season exactly as a live run does.
//
// Without this a simulated GW2 decision is made from one match of data with
// nothing to temper it, and the model produces gains of nine points a gameweek
// from noise. The blend is not an optional refinement in a replay; it is the
// difference between a simulation and a random-number generator.
type Priors struct{ S *Season }

// Get returns last season's totals for a player, by permanent code.
func (p Priors) Get(code int) (*analysis.PriorPlayer, bool) {
	for _, q := range p.S.Players {
		if q.Code == code && q.Minutes > 0 {
			pp := PriorFrom(q)
			return &pp, true
		}
	}
	return nil, false
}

// PriorFrom projects an archived season into the shape the scoring model consumes.
//
// # One projection, because there were four
//
// This field list existed four times — here, in `stat()`, in `newPriorIndexRecent`
// and in `cmd/priorblend`'s `priorOf` — and **all four omitted `DefCon`**, while the
// two live paths (`internal/priors/adapter.go`, `internal/recent/priors.go`) carried
// it. One quantity, two field sets, which is this record's signature failure; the
// arithmetic downstream was already shared, so it was only the projection that had
// drifted.
//
// The omission was not a missing term. `blendRates` mixes toward whatever the prior
// says, and its pre-season branch *replaces* the estimate with it outright, so a
// prior of zero tells the model a defender contributed nothing defensively rather
// than telling it nothing at all.
//
// # Zero still means two things, and the caller resolves which
//
// A player with no defensive actions and a season that could not record any both
// arrive here as 0, because the archive's column does not exist before 2025-26. This
// function does not guess: it reports the total. Callers that **price** it gate on
// `DefconScoredIn` (see `PreSeasonWith`), and callers that **blend** it across
// seasons must say so with `PriorSeasonStats.NoDefCon`. ✅ **All of them now do**,
// closed 2026-08-14 by `d27c5c9`: three of the four archive-side constructions
// omitted the flags, and `PriorStatsFrom` below now takes the `Capability` as a
// REQUIRED argument, so the omission is unspellable rather than merely fixed.
// ⚠️ This sentence read "which three of them still do not" until 2026-08-15, twenty
// lines above the doc comment recording the fix — the same file asserting a defect
// open and closed at once.
func PriorFrom(q *Player) analysis.PriorPlayer {
	return analysis.PriorPlayer{
		Minutes: q.Minutes, Starts: q.Starts,
		XG: q.XG, XA: q.XA, XGC: q.XGC, DefCon: q.DefCon,
		Bonus: q.Bonus, Saves: q.Saves, Yellow: q.Yellow, Red: q.Red,
	}
}

// Capability is what a season's source actually measured, read off the loaded
// totals rather than inferred from its name. See Season.HasXG for why.
type Capability struct{ NoXG, NoXGC, NoDefCon bool }

// CapabilityOf probes a loaded season once. Hoist it out of any per-player loop:
// each field is an O(players) scan, so calling it per player is quadratic.
func CapabilityOf(s *Season) Capability {
	return Capability{NoXG: !s.HasXG(), NoXGC: !s.HasXGC(), NoDefCon: !s.HasDefCon()}
}

// PriorStatsFrom is the whole blend input for one archived season: the projection
// and the capability flags together.
//
// # Why the flags are not optional here
//
// Without them a season that could not record a statistic contributes a measured
// zero, which is the absence-as-data failure `BlendPriors`' three denominators exist
// to prevent. That is easy to forget, and it was: three of the four archive-side
// constructions omitted them, and a first pass at fixing that reached two.
//
// Taking the capability as a required argument is what makes the omission
// unspellable — a caller must have thought about it to compile. `cmd/priorblend`
// and the replay now build their blend input through this one function, so the two
// binaries cannot disagree about what a season measured, which they did.
func PriorStatsFrom(q *Player, ago int, c Capability) analysis.PriorSeasonStats {
	return analysis.PriorSeasonStats{
		PriorPlayer: PriorFrom(q),
		SeasonsAgo:  ago,
		NoXG:        c.NoXG,
		NoXGC:       c.NoXGC,
		NoDefCon:    c.NoDefCon,
	}
}

// priorIndex builds the code lookup once, since Simulate rebuilds the engine
// every gameweek and a linear scan per lookup would dominate the run.
type priorIndex struct{ byCode map[int]*analysis.PriorPlayer }

func newPriorIndex(s *Season) priorIndex {
	return newPriorIndexMulti([]*Season{s}, 0)
}

// newPriorIndexRecent builds the prior from last season's *per-gameweek* rows,
// weighting the closing weeks more heavily than the opening ones.
//
// # The gap this fills
//
// Three recency knobs exist and none of them does this. RateHalfLife and
// MinutesHalfLife weight gameweeks *within the current season*; prior_half_life
// blends *across seasons*, two-ago against one-ago. The prior season itself is a
// flat total, so a player who lost his place in March counts the same as one who
// won it, and a player who broke through in February is averaged down by the
// autumn he spent on the bench.
//
// That matters because "the prior is stale" is the measured reason a heavier
// prior fails: raising BlendRateK to 12, 16 and 24 loses monotonically, and
// loses *most* late in a season. If staleness is what makes the prior expensive,
// weighting it toward the end of last season is the direct fix — the closing
// weeks are the best available statement of the role a player carries into
// August.
//
// # Minutes and rates take separate half-lives, on purpose
//
// The standing finding in this project is that they behave oppositely: minutes
// reward sharp recency because it removes a *bias* — a dropped player reading as
// an ever-present — while rates punish it, because a short window chases
// finishing variance and buys accuracy on the average player at the cost of the
// tail an argmax lives in. There is no reason that flips just because the season
// changed, so they are knobs rather than one knob.
//
// Zero for either leaves that half flat, which reproduces the old behaviour
// exactly.
//
// Half-lives are in gameweeks, counting back from the last one the player
// appeared in.
func newPriorIndexRecent(s *Season, minutesHalfLife, rateHalfLife float64) priorIndex {
	if minutesHalfLife <= 0 && rateHalfLife <= 0 {
		return newPriorIndex(s)
	}
	m := map[int]*analysis.PriorPlayer{}
	for _, q := range s.Players {
		if q.Code == 0 || q.Minutes == 0 {
			continue
		}
		// The same projection the flat path uses. It was a fourth copy of the field
		// list, and being an *actively swept* arm it is the one where a divergence
		// would have been measured and attributed to the half-life rather than to
		// the missing field.
		p := PriorFrom(q)
		last := 0
		for gw := range q.GWs {
			if gw > last {
				last = gw
			}
		}
		if last > 0 {
			if minutesHalfLife > 0 {
				// Weighted mean minutes per gameweek, rescaled to a season so
				// the evidence weight downstream keeps its usual units.
				var num, den float64
				for gw, g := range q.GWs {
					w := math.Pow(0.5, float64(last-gw)/minutesHalfLife)
					num += float64(g.Minutes) * w
					den += w
				}
				if den > 0 {
					perGW := num / den
					p.Minutes = int(perGW * float64(analysis.GameweeksPerSeason))
					if q.Minutes > 0 {
						p.Starts = int(float64(q.Starts) *
							float64(p.Minutes) / float64(q.Minutes))
					}
				}
			}
			if rateHalfLife > 0 {
				// Rates are weighted by minutes as well as recency, because a
				// rate is evidence in proportion to the football behind it.
				var rw, xg, xa, xgc, dc, bonus, saves float64
				for gw, g := range q.GWs {
					if g.Minutes == 0 {
						continue
					}
					w := math.Pow(0.5, float64(last-gw)/rateHalfLife) *
						float64(g.Minutes) / 90
					rw += w
					per := func(v float64) float64 { return v / (float64(g.Minutes) / 90) }
					xg += per(g.XG) * w
					xa += per(g.XA) * w
					xgc += per(g.XGC) * w
					dc += per(float64(g.DefCon)) * w
					bonus += per(float64(g.Bonus)) * w
					saves += per(float64(g.Saves)) * w
				}
				if rw > 0 {
					// ⚠️ DefCon must be re-based with the others. This block
					// re-expresses every total against the minutes the minutes
					// branch just rewrote, and a total left at the flat season
					// figure is then divided by a SMALLER denominator downstream —
					// blendRates reads per90(DefCon, Minutes) — inflating it by
					// fullMinutes/recencyMinutes, up to about 3x for a player who
					// lost his place in the spring. It was omitted until
					// 2026-08-14, in the arm this record calls actively swept, so
					// the error would have been measured and attributed to the
					// half-life.
					n90 := float64(p.Minutes) / 90
					p.XG = xg / rw * n90
					p.XA = xa / rw * n90
					p.XGC = xgc / rw * n90
					p.DefCon = int(dc/rw*n90 + 0.5)
					p.Bonus = int(bonus / rw * n90)
					p.Saves = int(saves / rw * n90)
				}
			}
		}
		m[q.Code] = &p
	}
	return priorIndex{m}
}

// thinPrior is the minutes below which the immediate prior season stops being
// trusted on its own. Half a season.
//
// An alias, not a second declaration — see analysis.ThinSeason, which is also
// where the gate itself (analysis.ShouldBlendPrior) lives. This copy exists only
// so the replay's own source reads in its own terms.
const thinPrior = analysis.ThinSeason

// newPriorIndexMulti blends several completed seasons into one prior, most
// recent first. A half-life of zero uses the first season alone.
func newPriorIndexMulti(seasons []*Season, halfLife float64) priorIndex {
	// Capability is a fact about each SEASON, so it is resolved once per season
	// rather than once per player, and from what the loader actually produced so
	// the repairs and their switches are followed automatically.
	caps := make([]Capability, len(seasons))
	for i, s := range seasons {
		caps[i] = CapabilityOf(s)
	}
	stat := func(q *Player, ago int) analysis.PriorSeasonStats {
		return PriorStatsFrom(q, ago, caps[ago])
	}
	m := map[int]*analysis.PriorPlayer{}
	if halfLife <= 0 || len(seasons) < 2 {
		for _, q := range seasons[0].Players {
			if q.Code > 0 && q.Minutes > 0 {
				p := stat(q, 0).PriorPlayer
				m[q.Code] = &p
			}
		}
		return priorIndex{m}
	}

	byCode := make([]map[int]*Player, len(seasons))
	for i, s := range seasons {
		byCode[i] = s.ByCode()
	}
	for code := range byCode[0] {
		// The gate, in the two directions it excludes. A full season is the best
		// evidence there is about a player, and blending an older one into it
		// dilutes genuine improvement — which is most players, most of the time.
		// The older seasons are a fallback for the case where the recent sample
		// is an artefact, not a general smoothing.
		recent, ok := byCode[0][code]
		if !ok {
			continue
		}
		if !analysis.ShouldBlendPrior(recent.Minutes) {
			// Not blended. Emit him exactly as the halfLife <= 0 branch above
			// would: a season with minutes becomes his prior, and a season with
			// NONE leaves him out of the index altogether, so Get reports no
			// prior and blendRates sends him to shrinkToLeague. Falling through
			// to the blend instead would replace that with a two-year-old
			// season, which is the half of this feature that measured worse.
			if recent.Minutes > 0 {
				p := stat(recent, 0).PriorPlayer
				m[code] = &p
			}
			continue
		}
		var hist []analysis.PriorSeasonStats
		for i := range seasons {
			if q, ok := byCode[i][code]; ok && q.Minutes > 0 {
				hist = append(hist, stat(q, i))
			}
		}
		if len(hist) == 0 {
			continue
		}
		p := analysis.BlendPriors(hist, halfLife)
		if p.Minutes > 0 {
			m[code] = &p
		}
	}
	return priorIndex{m}
}

func (p priorIndex) Get(code int) (*analysis.PriorPlayer, bool) {
	q, ok := p.byCode[code]
	return q, ok
}

// HoldResult is the honest baseline: the opening fifteen kept all season, but
// the eleven and captain re-chosen weekly from what the model knows.
//
// Comparing weekly transfers against a frozen eleven would credit transfers
// with the value of simply picking your best players each week, which every
// manager does for free.
func Hold(cur, prior *Season, cfg SimConfig, held []int) int {
	total := 0
	for _, n := range HoldWeekly(cur, prior, cfg, held) {
		total += n
	}
	return total
}

// HoldWeekly is the same baseline reported gameweek by gameweek, so a report can
// show where transfers actually earned their keep rather than only the total.
func HoldWeekly(cur, prior *Season, cfg SimConfig, held []int) []int {
	return HoldCaptaincyWeekly(cur, prior, cfg, held).Full
}

// HoldCaptaincy carries the held-fifteen baseline scored three ways, differing
// only in what happens to the armband. Every slice has one entry per gameweek
// played, from cfg.startGW() to 38.
//
// # What the three rungs are, and why they exist
//
// `Full` is HOLD exactly as every figure in AGENTS.md is measured: the eleven and
// the captain are both re-picked each week from what the model knows, and the
// vice-captain takes over when the captain records no minutes. That is what FPL
// pays and it is the metric a scoring constant is judged on.
//
// The other two exist because the armband **doubles a player's realised return,
// so it doubles his contribution to the metric's variance and not only to its
// mean**. HOLD's residual spread on this harness is about 1.03 points per
// gameweek and the crossed variance fit attributes all of it to within-season
// path noise rather than to genuine season-to-season difference, which makes
// "how much of that path noise is one doubled footballer" a measurable question
// instead of a rhetorical one:
//
//	FixedCaptain  the eleven is still re-picked weekly, but the armband is
//	              pinned to whoever the model would have captained in the week
//	              the squad was bought — so the weekly *churn* in who is
//	              captained is removed while the doubling itself stays.
//	NoCaptain     the eleven is re-picked weekly and nobody is doubled at all,
//	              which removes the armband's variance contribution entirely.
//
// **Neither is a model of FPL and neither may replace HOLD.** FPL doubles a
// captain every week, so a metric that does not is further from the game, not
// closer to it — these are candidate lower-noise *instruments* for tuning, and
// an instrument is only worth having if it keeps the signal it is meant to
// measure. That is what the positive controls in TestDiagCaptaincyNoise are for.
//
// One consequence of pinning the captain is worth stating rather than
// discovering: a frozen captain who drops out of a later week's eleven is not
// doubled that week, because weekPointsWithChip only doubles a captain it finds
// in the eleven. The frozen vice takes over when *he* is in the eleven, which is
// the same rule, and otherwise nobody is doubled. So FixedCaptain sits slightly
// below Full in level. That is harmless here: every comparison built on these is
// a paired difference between two arms scored the same way, and a level shift
// shared by both arms cancels.
type HoldCaptaincy struct {
	Full         []int
	FixedCaptain []int
	NoCaptain    []int

	// The same three rungs on the accumulated-xPoints instrument, gameweek by
	// gameweek and aligned with the slices above. `FullXP` is HOLD's xPoints — the
	// mirror of `Full`, which is HOLD's points — and is what the cells file's
	// `hold_xpoints` column is summed from.
	//
	// They come out of the same `weekScore` calls the points rungs do, so there is
	// no second weekly pass and no second expression of "which eleven, which
	// armband, which autosubs". Nothing shipped reads them; a scoring figure moving
	// because these exist would mean the projection is not a projection.
	FullXP         []float64
	FixedCaptainXP []float64
	NoCaptainXP    []float64

	// Per-gameweek detail, for forensics rather than for any metric. One entry
	// per gameweek, aligned with the slices above.
	//
	// These exist because a concentrated result — a mean that is three cells of
	// thirty-six — can only be judged by looking at what the squad actually did
	// in those cells, and the alternative was a second copy of this loop inside a
	// diagnostic. That is this package's most-repeated bug, and AGENTS.md names a
	// diagnostic as the worst place for it. Populated unconditionally because
	// they are already computed here; nothing on the scoring path reads them, so
	// no measured figure can move.
	GW      []int
	XI      [][]int
	Bench   [][]int
	Captain []int
	Vice    []int
}

// HoldCaptaincyWeekly scores the held fifteen under all three captaincy rules in
// one pass.
//
// One pass rather than three functions: the expensive part is rebuilding the
// engine for each of up to 38 gameweeks, which every rung shares, so the two
// extra rungs cost two more weekPoints calls per week plus one engine build for
// the day-one armband — under 3% on top of a call that was already paying for
// thirty-eight. It is also the reason HoldWeekly delegates here instead of
// keeping its own loop: this package's most-repeated bug is two implementations
// of one quantity, and a second copy of the weekly pick would be exactly that.
func HoldCaptaincyWeekly(cur, prior *Season, cfg SimConfig, held []int) HoldCaptaincy {
	// Through SimConfig.priors, which also closes a divergence: this site built
	// the prior with newPriorIndexMulti unconditionally while Simulate's honoured
	// PriorMinutesHalfLife and PriorRateHalfLife, so a run with either set gave
	// the *squad* a recency-weighted prior and the weekly re-pick a flat one —
	// two expressions of one quantity, differing. Both default to zero, so no
	// figure measured on the standard grid moves.
	idx := cfg.priors(cur, prior)
	start := cfg.startGW()

	// The frozen armband is read from the engine as it stood the week the squad
	// was bought — the same point PointInTime is called from for the opening
	// squad itself, so "who you would have captained on day one" means the same
	// thing here as everywhere else in this package.
	// The held path is where every oracle diagnostic in this package enters, and
	// it was the one path that never checked the combination it was handed. Every
	// refusal in Validate — teamnews against availability, features against
	// teamnews, a scope with no oracle, a source-less teamnews arm — was therefore
	// advisory here, and a caller who got it wrong would get a silent no-op or one
	// oracle quietly overwriting another rather than an error. Simulate has always
	// validated; this is the same guarantee on the metric this file scores
	// constants with.
	if err := cfg.Oracles.Validate(); err != nil {
		panic("backtest: " + err.Error())
	}
	if err := checkFeaturesFrom(cfg.Oracles, start); err != nil {
		panic("backtest: " + err.Error())
	}
	fb, ff := PointInTimeWith(cur, prior, start-1, cfg.Oracles)
	fe := analysis.NewEngineFull(fb, ff, cfg.Weights, analysis.Congestion{}, analysis.RoleRisk{})
	fe.Priors = idx
	fe.Recent = cfg.recentIndex(cur, start-1)
	fe.TeamForm = newTeamFormIndex(cur, start-1)
	// Deliberately NOT oracled, even when AxisArmband is on. This rung's whole
	// definition is "the armband pinned to the day-one pick", and a hindsight
	// day-one pick would be neither pinned nor an instrument. Leaving it alone also
	// buys an invariance: AxisArmband declares hold_fixedcap_points must not move,
	// which is a free razor-sharp check on a column every sweep already emits.
	_, _, frozenCapt, frozenVice := pickXI(fe, held)

	var out HoldCaptaincy
	// From the same gameweek the policy started at, or the baseline would be
	// scored over a longer season than the thing it is a baseline for.
	for gw := start; gw <= 38; gw++ {
		b, fx := PointInTimeWith(cur, prior, gw-1, cfg.Oracles)
		e := analysis.NewEngineFull(b, fx, cfg.Weights, analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = idx
		e.Recent = cfg.recentIndex(cur, gw-1)
		e.TeamForm = newTeamFormIndex(cur, gw-1)
		xi, bench, captain, vice := pickXI(e, held)
		// The armband oracle reaches the held metric too, and must: HOLD is where
		// the armband's contribution actually lives — the variance decomposition
		// puts it at +4.779 points a gameweek there, ahead of weekly re-picking and
		// autosubs. Only the Full rung, which is the one that models what FPL pays.
		if cfg.Oracles.Decision == AxisArmband {
			captain, vice = bestArmband(cur, xi, gw)
		}
		xiP, benchP := idsToPlayers(cur, xi), idsToPlayers(cur, bench)
		out.GW = append(out.GW, gw)
		out.XI = append(out.XI, append([]int(nil), xi...))
		out.Bench = append(out.Bench, append([]int(nil), bench...))
		out.Captain = append(out.Captain, captain)
		out.Vice = append(out.Vice, vice)
		full := weekScore(xiP, benchP, gw, captain, vice)
		fixed := weekScore(xiP, benchP, gw, frozenCapt, frozenVice)
		out.Full = append(out.Full, full.Points)
		out.FullXP = append(out.FullXP, full.XPoints)
		out.FixedCaptain = append(out.FixedCaptain, fixed.Points)
		out.FixedCaptainXP = append(out.FixedCaptainXP, fixed.XPoints)
		// Captain id 0 is no player, so nothing is doubled and the vice-captain
		// fallback cannot fire either. Passing 0 rather than adding a flag keeps
		// one scoring function: weekPointsWithChip looks the captain up in the
		// eleven and finds nobody.
		none := weekScore(xiP, benchP, gw, 0, 0)
		out.NoCaptain = append(out.NoCaptain, none.Points)
		out.NoCaptainXP = append(out.NoCaptainXP, none.XPoints)
	}
	return out
}

// checkFeaturesFrom refuses a gate that cannot bind, in either direction.
//
// `Oracles.Validate` cannot do this: it sees the oracle state and not the entry
// gameweek, and the two only meet where a cell is actually run. Both failures are
// silent AND stamped, which is the shape this package refuses by name everywhere
// else:
//
//   - at or below the entry gameweek the gate never binds, because the weekly loop
//     only ever evaluates `statusAt` from `start` onward — so the arm runs the
//     UNRESTRICTED oracle while stamping `from:N`. That is the shipped
//     `min_gain` 0.0-against-0.4 pattern: a constant whose gate is already bound;
//   - above 38 it never fires, so the arm reproduces the baseline byte for byte
//     while stamping itself as oracled, and R joins a clean null that is
//     indistinguishable from a wiring failure.
func checkFeaturesFrom(o Oracles, start int) error {
	f := o.FeaturesFrom
	if f == 0 {
		return nil
	}
	if start >= 1 && f <= start {
		return fmt.Errorf("oracle: FeaturesFrom %d with entry gameweek %d gates "+
			"nothing — the opening build evaluates statusAt at %d and the weekly "+
			"loop never goes earlier, so this runs the unrestricted oracle while "+
			"stamping a restriction. Use start+1", f, start, start)
	}
	if f > 38 {
		return fmt.Errorf("oracle: FeaturesFrom %d is past the end of the season, "+
			"so the oracle never fires and the arm reports the baseline under an "+
			"oracled stamp", f)
	}
	return nil
}
