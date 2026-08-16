package backtest

// Oracles: replay modes that grant the policy hindsight it could not have had.
//
// The point is never to score well. It is to put an **upper bound** on what some
// capability could be worth before anyone builds it — "is this worth building at
// all?" answered by measurement rather than by argument. Two oracles have already
// paid for themselves that way: perfect team news and perfect price timing.
//
// See the oracle-design document. This file is the shared machinery, so the third and
// later oracles inherit the guarantees rather than re-deriving them. Before it,
// there were three oracles, three mechanisms and zero shared guarantees.
//
// # The three rules this file exists to enforce
//
// **Information and decision oracles are different things.** An *information*
// oracle corrects an input the model was wrong about — who was available, what
// the price was — and leaves the policy free to decide on the corrected input. A
// *decision* oracle picks the outcome directly. They bound different quantities,
// "better data" against "better judgement given the same data", and a figure that
// mixes them bounds neither.
//
// **A decision oracle must be confined to one axis or it degenerates.** Given
// hindsight over every choice at once the replay simply plays the season
// perfectly and the number means nothing. That is why Decision is a single
// enum-typed field rather than a set of booleans: "at most one axis" is expressed
// by the type and cannot be violated by a caller who did not read the comment.
//
// **Oracles live in internal/backtest and nowhere else.** A minutes oracle will
// tempt someone to hook analysis.Engine's expected-minutes path, which is
// *shipped* code. An information oracle must instead be expressible as a
// perturbation of what PointInTimeWith hands the model, so the live scoring
// engine stays oracle-free. Nothing in internal/analysis may learn that oracles
// exist.
//
// # What is deliberately absent
//
// The oracle-design document's catalogue names five more oracles — team strength,
// minutes, and the chip-week, armband and transfer-gate decision axes. That
// catalogue is no longer readable from this repository, so the five names in the
// line above are now the only record of them: do not shorten it, and add to it
// rather than assuming the list can be recovered elsewhere. **None of
// their constants is declared here**, because a constant that nothing implements
// is a silent no-op of exactly the class this project keeps shipping: an arm
// labelled "ORACLE[decision:armband]" that changes nothing, measures nothing and
// reports a clean null. Each constant lands in the commit that wires its hook.
// Validate refuses anything outside the implemented set for the same reason.

import (
	"fmt"
	"os"
	"strings"
)

// InfoOracle names an input the model was wrong about. A bitmask: several may be
// on at once, because each corrects a different input and they compose.
type InfoOracle uint

const (
	// OracleAvailability marks anyone who finished the season with no minutes as
	// unavailable from the start. It bounds perfect team news — narrowly, since
	// it sees only the players who never appear at all and not the far larger
	// population who play until October and then stop. See statusAt.
	OracleAvailability InfoOracle = 1 << iota

	// OracleTransactPrice gives every transfer the best price available around
	// the decision: buy at the cheapest in the surrounding window, sell at the
	// dearest, still paying FPL's half-of-any-rise rule. It bounds the whole
	// economic case for acting quickly, and it perturbs the **transaction** seam
	// only — the optimiser is still quoted the real NowCost on the bootstrap. See
	// bestBuyPrice.
	//
	// # It was not an upper bound until the sale settled where the search was told
	//
	// The arm bought low and **sold low**. `decide` quoted the search
	// `bestSellPrice`, the window maximum, and then settled the sale by recomputing
	// through `market`, which under this oracle is `bestBuyPrice` — the window
	// *minimum*. So the gate was promised money the wallet never received, and the
	// arm was neither an upper bound nor a coherent policy. It was invisible on the
	// baseline and only there: un-oracled, both sides collapse to the same
	// `marketPrice` call and bank the identical number, which is why no recorded
	// figure could distinguish the two forms. **Every price-timing figure in the
	// record came from the defective arm.** See settleSale.
	//
	// # Measured on the corrected arm, and it does not resolve
	//
	// 24 cells, four seasons by six entry points, against the shipped baseline
	// (which comes back byte-identical: POLICY 35871, HOLD 31987, 595 moves):
	//
	//	POLICY  +0.394 pts/gw  naive SE 0.375  CR2 SE 0.415  df 3  t 0.95  ~+15/season
	//	the three held rungs  +0.000 exactly, every cell — the declared invariance
	//
	// The detection threshold for *this* comparison is 3.182 x 0.415 = 1.32 a
	// gameweek, about 50 a season, so the estimate is a third of what would be
	// needed. It is kept anyway, and as **standing instrumentation** rather than as
	// a measurement that failed: the arm is now correct, so if the points model ever
	// improves to the point where price timing does resolve, the instrument is in
	// place to detect it. That is the BankLookahead precedent — kept off because it
	// is the correct comparison to make.
	//
	// **The resolution trigger is arithmetic, and it names the right quantity.**
	// With three degrees of freedom the bar is t_crit 3.182, not 2, so at today's
	// point estimate the **standard error** would have to fall from 0.415 to about
	// **0.124** per gameweek — a factor of 3.3. More seasons do not get there on
	// their own: the six-season grid scales the threshold by 0.660, which still
	// leaves a factor of 2.2. And the quantity that must improve is the standard
	// error, which is driven by the replay's path chaos — one flipped transfer
	// changes the squad for every remaining week — and **not** average predictive
	// accuracy, which would move the point estimate instead. Those are different
	// numbers, and this record already documents a better predictor making a worse
	// policy.
	OracleTransactPrice

	// OracleMinutes tells the model exactly how much football each player is
	// about to play, per match his club plays, over the **next few gameweeks**.
	//
	// It is the generalisation of which OracleAvailability is the degenerate
	// case, and it bounds the whole rotation-risk family — MinutesHalfLife,
	// BlendMinutesK, the blank-run penalty, the squad pool's
	// minimum-expected-minutes cliff — at once.
	//
	// **The window is bounded and the first version's was not.** It used to
	// average the whole remainder of the season, which cannot express "starts
	// GW20, subbed GW21, missed GW22-27, back GW28" and handed a player with a
	// broken leg to the model as a mildly-reduced ever-present every week he was
	// in hospital. Every figure recorded against the unbounded form measures a
	// season-average question and is superseded rather than merely stale.
	//
	// **It perturbs the recency index, not the bootstrap**, which is a deviation
	// from the oracle-design document made because the design's seam does not work for
	// this quantity: in-season blendRates replaces the bootstrap's minutes with
	// Engine.Recent outright, and minutes on the bootstrap are the denominator of
	// every counting rate, so rewriting them there could not leave the per-90
	// rates alone. Both seams are manufactured by this package and neither is in
	// internal/analysis. See minutesoracle.go, which is where the whole argument
	// is written down.
	OracleMinutes

	// OracleLineups tells the model who is picked over the decision window —
	// started, came off the bench, or did not feature — and prices each state at
	// the player's own conditional average rather than at what he actually
	// played.
	//
	// It is the **reachable half** of OracleMinutes. Selection is a fact a real
	// system can partly acquire, from press conferences, injury reporting and
	// rotation patterns; the exact minute a manager makes a substitution is not
	// known at the deadline by anybody, including the manager. So the pair is a
	// decomposition rather than two measurements: lineups bounds what better team
	// news could buy, and minutes-minus-lineups is the irreducible residual.
	//
	// **It travels down MinutesPerMatch, not StartShare**, and that is forced
	// rather than chosen: nothing in the shipped scoring model consumes start
	// information at all, so a StartShare arm would stamp itself, change nothing
	// and report a clean null. See lineupsoracle.go, where the channel trace is
	// written down.
	//
	// OracleAvailability is the degenerate case of *this* oracle rather than of
	// OracleMinutes: it fires on a player's season *total* minutes, so it knows
	// only about players who never appeared all year and is blind to every injury
	// that resolves. It is a selection fact at the coarsest possible resolution.
	OracleLineups

	// OracleFeatures tells the model, at the deadline, whether each player will
	// actually feature in the gameweek about to be played — and nothing else.
	//
	// It exists because OracleLineups **cannot reach a flagged player at all**,
	// which was found by measuring rather than by reading. Restricting the lineups
	// oracle to the players the model's own reconstruction flags returned exactly
	// zero: 0.0% of 918 gameweeks changed, with the unflagged arm reproducing the
	// unrestricted one digit for digit. The population is not empty — 3.79% of
	// held player-gameweeks carry a flag — so that is a structural inertness and
	// not a null.
	//
	// The mechanism, traced in source rather than asserted: `Score` is multiplied
	// by `availabilityFactor` (metrics.go:1536-1547), which returns **exactly 0**
	// for `i`, `u`, `s` and `n` (metrics.go:1868-1878). The lineups oracle
	// rewrites the recency index, which reaches `ExpectedMinutes` — and the
	// weekly re-pick consumes nothing but `Score` and position: formation, bench
	// order, captaincy and the autosub loop all key on one or the other. So
	// hindsight about a flagged player's minutes multiplies by nothing, and the
	// result is exactly zero rather than merely small.
	//
	// ⚠️ That holds of `Score`, **not** of every consumer of `ExpectedMinutes`,
	// which an earlier version of this comment claimed. `SettledMinutes` is
	// written from the same blended minutes and gates the squad pool OUTSIDE the
	// zero; what keeps a flagged player out of the squad path is the separate
	// `Status` filter in squad.go, a different guarantee. Note `statusAt`'s reconstruction
	// never emits `d`, the doubtful flag — the *archive* carries no
	// `chance_of_playing` — so on the reconstruction the 0.5 rung of
	// availabilityFactor is unreachable and every flag is total. That is a
	// property of the reconstruction and not of the harness: OracleTeamNews reads
	// crawled payloads that do carry `d`, and OracleTeamNewsChance reaches the
	// percentage itself.
	//
	// ⚠️ **Do not explain the near-null with "FPL's autosub already captures most
	// of it".** That was asserted here and is REFUTED by TestDiagAutosubValue.
	// Autosubs cover **38.5% of starter blanks** — 1,681 blanks, 647 covered — and
	// formation legality blocks more of them (640) than an idle bench does (394).
	// A substitute returns **2.51 points against a playing starter's 4.72**, so per
	// blank the autosub recovers about 0.97 of 4.72, a fifth rather than most. It
	// is also a downgrade in KIND: midfielders and forwards supply 994 of the
	// blanks while defenders supply 351 of the 647 substitutions. And the bench is
	// a considered decision the substitution overrides — a substitute scores
	// **0.735 of his own per-appearance average** as a midfielder, 0.864 as a
	// forward.
	//
	// The near-null is real; the reason is different and narrower. Re-ordering the
	// bench with perfect foresight, from the same fifteen and under the same
	// formation rules, is worth **+0.1005 points per blank**. At 1.831 blanks per
	// gameweek (1,681 over 918 cell-gameweeks) that is **+7.0 over 38 gameweeks**.
	//
	// ⚠️ **It was recorded here as +4.7, and that was a units error of mine.** The
	// figure came from dividing the pooled total by the 36 **cells**, and a cell is
	// not a season: the six entry points give cells of 38/33/28/23/18/13 gameweeks,
	// mean 25.5. The standing convention in AGENTS.md is to hold a paired
	// difference **per gameweek** and multiply by 38, never to divide a pooled
	// total by the cell count.
	// So the autosub is not capturing the value; **there is very little value on a
	// bench to capture.** That much is measured.
	//
	// *Hypothesis, untested:* the reason is that `XIValue` credits bench players at
	// zero, so the transfer search sells bench quality for eleven quality every
	// time and the fifteen converges on eleven good players and four who cannot
	// cover. The record already measures that convergence — the eleven changes for
	// rotation ten times in three seasons — but nothing here connects it to this
	// residual. A deep-squad arm would test it, and `BenchWeight` already exists to
	// build one.
	//
	// The consequence points the same way regardless: what team news is worth
	// lives in the TRANSFER, not in the weekly re-pick — and that is now measured
	// rather than inferred. `TestDiagTeamNewsTransferValue` runs both metrics over
	// the same 36 cells with the opening fifteen pinned identical, so the two
	// channels separate as a paired diff-in-diff inside one process:
	//
	//	HOLD, the eleven only        +0.1023 pts/gw   = +3.9 a season
	//	POLICY, eleven AND transfers +1.4342 pts/gw   = +54.5 a season
	//	the difference               +1.3319 pts/gw   = +50.6 a season
	//
	// ⚠️ **The difference is NOT a channel decomposition and must not be called
	// "the transfer channel", which it was.** Writing it out: POLICY = T + X(policy
	// squad) and HOLD = X(baseline squad, frozen all season), so the residual is
	// T plus the bracket [X(policy) − X(baseline)], which is neither zero nor
	// signed — the baseline makes about 25 transfers per cell path, so by spring
	// the two squads are barely related, and the oracled policy buys players who
	// feature and therefore has fewer blanks left to re-pick around. The residual
	// is a real estimand — **what the ability to ACT adds to the value of the
	// news** — and that is what it should be called.
	//
	// **Fourteen times the eleven-only channel, and that RATIO is what the arm
	// establishes.** The sizes are another matter, below.
	//
	// ⚠️ **The "two instruments agreeing" claim once made here is RETRACTED, and it
	// was manufactured by the units error above.** Corrected, the autosub
	// counterfactual is ~7.0 against this arm's +3.9 — and they are not the same
	// quantity in any case: `bestLegalPromotion` maximises *realised points* over a
	// fixed eleven with no armband, while this oracle supplies only *whether he
	// features* and then re-picks the eleven **and the captain** on `Score`. A
	// coherent ordering of two different estimands is not corroboration. Nor would
	// agreement on the eleven-only rung have supported the POLICY rung, which is
	// measured through a different decision and is the number in dispute.
	//
	// ⚠️ **It does not resolve, and it is carried by two seasons.** Season means run
	// +22.7 / +130.6 / +21.5 / **−24.8** / +54.2 / +122.9. Clustered by season the
	// sd is 61.41, the SE is 25.07 and **t = 2.174 against a df-5 critical value of
	// 2.571** — so this comparison's own threshold is **64.5 a season** and the
	// estimate sits below it. **2021-22 and 2025-26 supply 77.5% of the sum**; drop
	// them and the other four average **+18.4 at t = 1.13**. The median season mean
	// is +22.7, less than half the mean. Exactly one leave-one-out subset clears
	// its bar, and it is the one that drops the sole negative season.
	//
	// Note the largest column, 2021-22, is one of the two seasons where `XGC` is
	// unrepaired, so defenders and keepers are scored with **neither** the clean
	// sheet nor the goals-conceded deduction. The six seasons are not exchangeable
	// draws, which is what a season-clustered SE assumes.
	//
	// The naive clustered t above is the **ceiling** of the honest one: R's CR2
	// with Satterthwaite df will inflate the SE and push df below 5. **No CR2
	// verdict exists**, and the standard path refuses this arm — `runPolicySweep`
	// fails a cell whose `Oracles` differ from the variant's, and `FeaturesFrom`
	// varies by start point by design.
	//
	// The mediator says the policy genuinely acts rather than merely scoring
	// differently: 904 transfers become 949 and 94 hits become 123.
	//
	// **But the hits are not where the value is, and assuming they were was
	// wrong.** Run again with `MaxHits: 0`, so the manager holds a free transfer
	// for the news rather than paying to react to it:
	//
	//	free transfers only  +1.4025 pts/gw  = +53.3 a season   815 → 830 moves
	//	hits allowed         +1.4342 pts/gw  = +54.5 a season   904 → 949 moves
	//	what the hits added  +0.0317 pts/gw  =  +1.2 a season
	//
	// So essentially the whole of it is available without paying, which is the
	// reassuring answer: the case for team news does not rest on a behaviour
	// anybody would have to be talked into. And the volume mediator restates a
	// finding this file already has four confirmations of — with hits available
	// the policy makes three times as many extra moves (+45 against +15).
	//
	// ⚠️ **+1.2 is not a measurement that hits pay nothing.** It is **2% of this
	// comparison's own detection threshold** and has no standard error, so it is
	// indistinguishable from zero in either direction. What it establishes is the
	// negative: the +53 headline is **not** an artifact of an aggression setting.
	// `MaxHits` is the only field that differs between the two arms and each
	// carries its own within-policy baseline, so the design is clean; it is the
	// reading that must stay narrow.
	//
	// This is therefore the missing channel rather than a third resolution of the
	// same one. It is also the one the *question* is about: "perfect team news"
	// means knowing at the deadline who is playing, and the model's own
	// representation of that fact is `Status`, not `MinutesPerMatch`.
	//
	// # How it differs from OracleTeamNews, which is the pairing worth having
	//
	// OracleTeamNews replaces `Status` with what **FPL published** at that
	// deadline, recovered from crawled bootstraps. This replaces it with what
	// **turned out to be true**. So one bounds a *source* and the other bounds the
	// *fact*, and the gap between them is what a source better than FPL's own feed
	// could be worth — which is the question, since FPL's flags are late by
	// reputation and the lineup-prediction sites move hours earlier. Validate
	// refuses the pair in one arm, because a contrast cannot be carried by a
	// single figure.
	//
	// Two deliberate limits. A blank gameweek leaves the honest reconstruction
	// alone — the calendar is published, so pretending an oracle is needed for it
	// would credit hindsight with a fact the model already has. And it says
	// *whether* he features, never how much: that is OracleMinutes' question, and
	// keeping them disjoint is what lets the pair be read as a decomposition.
	//
	// # The standing framing for every oracle in this family
	//
	// **Assume the manager is trying to get perfect news BECAUSE he has transfers
	// to spend on it, and would take a hit to use one.** That is the operating
	// context, and it settles which measurement is the headline whenever a member
	// of this family is run:
	//
	//   - the `POLICY` metric is the one that answers the question, because the
	//     whole value of knowing is being able to ACT;
	//   - `HOLD` is a control here rather than a result — it prices what the
	//     knowledge is worth to somebody who cannot transfer, which is nobody;
	//   - an arm that pins the squad is measuring a constrained version of the
	//     question and must say so in its own headline rather than in a footnote.
	//
	// ⚠️ **Do not assume a hit.** The realistic behaviour is *holding a free
	// transfer in reserve until team news lands* — a timing decision, not a
	// willingness to pay four points — and the free-only arm is therefore the
	// honest headline. It happens not to matter here (the hits are worth +1.2 a
	// season on top), but that is a measured fact rather than a safe default, and
	// an arm that quietly banks its headline on hits is answering a question about
	// aggression rather than about information.
	//
	// Recorded because the first three measurements in this family all defaulted
	// to `HOLD` — it is the quiet metric and this file recommends it for scoring
	// constants — and so all three answered "what is team news worth if you may
	// not act on it". The answer to that is about 4 points a season. The answer to
	// the real question is about 50.
	//
	// # A third limit, which is NOT deliberate and makes this a dirty bound
	//
	// A one-gameweek fact acquires permanent force on the opening squad. `"u"`
	// maps to `"unavailable"`, and `Optimize` **removes** such a player from the
	// pool rather than zeroing his score — so anyone who happens to miss the entry
	// gameweek cannot be bought at all, for the whole season.
	//
	// Measured at a GW1 entry on 2025-26: **391 of the 690 registered players
	// record no minutes in GW1**, so all 391 are barred. Twelve of them go on to
	// play 2,000+ minutes — Gravenberch (2,991 minutes, 144 points) and Foden
	// (2,078, 131) among them. A manager with genuinely perfect team news buys
	// Gravenberch and benches him for one week; this arm cannot own him at all.
	//
	// So the figure is **not a clean upper bound**, and the distortion is unsigned:
	// it removes players the model would rightly have avoided this week and
	// players it would rightly have held all season, and lands entirely on squad
	// selection. `TestDiagLineupEventValue` closes the channel deliberately by
	// holding the fifteen fixed and un-oracled, which is why its figures are
	// clean; **anyone wiring `oracleVariant(Oracles{Info: OracleFeatures}, ...)`
	// into `runPolicySweep` inherits it and must not present the result as a
	// bound.** The honest sweep version restricts the rewrite to gameweeks after
	// the entry deadline, or declares the held rungs unreachable.
	//
	// `FeatureScope` restricts it to one side of the flag. That restriction is the
	// point rather than a convenience: the unrestricted arm is dense, and dense
	// arms on this harness need 135-239 points a season to resolve, while the
	// sparse availability arm needed 14.
	//
	// # The flagged arm is inert here too, for a SECOND and unrelated reason
	//
	// Measured on the six-season grid, `features: flagged` reads **exactly 0.000**
	// — same as `lineups: flagged`, and it would be easy to file both under the
	// availabilityFactor zero above. They are different failures and only the
	// lineups one is about scoring.
	//
	// This one is about the data. The reconstruction is **sticky and therefore
	// conservative**: `p.Status` is the END-of-season status and `p.NewsAdded` is
	// the LAST news item, so a player reads flagged only from the date of the news
	// that stuck. Someone injured in October who returned in December and finished
	// the season fit reads available all year — the reconstruction never flags
	// him at all. So a flag here means *terminal*, and the population an oracle
	// could correct is the players it flags who nonetheless played:
	//
	//	68 of 150,051 flagged player-gameweeks across the pool  (0.05%)
	//	 3 of     522 inside a held fifteen                     (0.57%)
	//
	// **Almost nothing to be right about.** That is a fact about the archive's
	// availability reconstruction, and it is the sharpest available argument for
	// why the recovered-capture work exists: `OracleTeamNews` reads crawled
	// bootstraps that carry the flag as it stood *that week*, including the
	// doubtful cases and every absence that later resolved — the population this
	// one cannot see by construction.
	//
	// So the flagged/unflagged partition is not a useful instrument **on the
	// reconstruction**, and the whole of both channels' value lands on the
	// unflagged side. Do not read that as "flags do not matter"; read it as this
	// harness's flags being nearly disjoint from the interesting cases.
	OracleFeatures

	// OracleOmniscient replaces every aggregate the bootstrap carries with the
	// football that has not been played yet.
	//
	// **A test fixture and never a report.** It is the positive control for the
	// oracle machinery: an arm told exactly what every player is about to do and
	// scoring like the shipped model means hindsight is not reaching a decision,
	// which is a broken harness rather than a small effect. Every other oracle is
	// a negative control at heart — it declares what it must not move, and passing
	// means nothing moved — so nothing else in the catalogue can fail that way.
	//
	// Reportable is false for it and validateOracleArms refuses any sweep arm
	// carrying it, so its number cannot reach a means file, a cells file or an
	// inference table. See omniscience.go.
	OracleOmniscient

	// OracleTeamNews replaces statusAt's reconstruction with FPL's own
	// availability flag as it stood at each deadline, recovered from crawled
	// bootstrap payloads.
	//
	// It is the non-degenerate version of OracleAvailability. That one fires on a
	// season *total* of zero minutes and so sees only players who never appear at
	// all; this one carries the doubtful flags and every absence that later
	// resolved, which is the population the record has repeatedly said carries the
	// cost.
	//
	// **It needs a source and refuses to run without one.** A bit with no data
	// behind it would stamp an arm, change nothing and report a clean null — the
	// failure this file exists to prevent — so Validate rejects it when
	// Oracles.News is nil rather than leaving it to a liveness check discovered
	// after an hours-long sweep. See teamnews.go.
	OracleTeamNews

	// OracleTeamNewsChance additionally hands the model FPL's published
	// chance_of_playing_next_round, which the replay has never once seen: nothing
	// in PointInTimeWith or PreSeasonWith sets that field, so availabilityFactor
	// has only ever taken its coarse status branch.
	//
	// It is meaningless alone and Validate says so. The percentage *overrides* the
	// flag inside availabilityFactor, so an arm carrying the percentage over a
	// reconstructed status would price flagged players on real data and everyone
	// else on the reconstruction — a chimera bounding neither. Composed with
	// OracleTeamNews it is the second rung of a decomposition, and the contrast
	// between the two arms is what the granularity is worth once the flag is
	// already right.
	OracleTeamNewsChance
)

// implementedInfo is every bit with a hook behind it. Validate rejects the rest,
// so a raw cast cannot produce an arm that stamps an oracle and measures none.
const implementedInfo = OracleAvailability | OracleTransactPrice | OracleMinutes |
	OracleLineups | OracleOmniscient | OracleTeamNews | OracleTeamNewsChance |
	OracleFeatures

// infoName is the stamp fragment for one bit. Names are short because they end up
// in a CSV column, a console table and an R block header.
var infoName = map[InfoOracle]string{
	OracleAvailability:   "availability",
	OracleTransactPrice:  "prices",
	OracleMinutes:        "minutes",
	OracleLineups:        "lineups",
	OracleOmniscient:     "omniscient",
	OracleTeamNews:       "teamnews",
	OracleTeamNewsChance: "teamnews_chance",
	OracleFeatures:       "features",
}

// infoOrder fixes the order names appear in a stamp, so the stamp is canonical
// and usable as a join key rather than depending on map iteration.
var infoOrder = []InfoOracle{
	OracleAvailability, OracleTransactPrice, OracleMinutes, OracleLineups,
	OracleOmniscient, OracleTeamNews, OracleTeamNewsChance, OracleFeatures,
}

// DecisionAxis names the ONE choice a decision oracle may make with hindsight.
//
// The catalogue's remaining axes — the starting eleven and the opening fifteen —
// are deliberately not here: see the file comment. Each lands in the commit that
// wires its hook, so the stamp and the invariance declaration arrive together with
// the behaviour rather than being retro-fitted around it.
type DecisionAxis int

const (
	AxisNone DecisionAxis = iota

	// AxisChipWeek picks which gameweek a scoring chip is played in.
	//
	// It is the cheapest oracle in the catalogue and the strictest: the replay
	// already records what each chip would have been worth in every week, on the
	// squad actually held, so the oracle is an argmax over a slice that is already
	// in the result and it changes **no decision whatsoever**. That total
	// invariance is why it is built first — an arm that must reproduce the
	// baseline byte for byte in every collected metric is the sharpest available
	// test of the harness itself. See chipWeekOracle.
	AxisChipWeek

	// AxisArmband hands the captaincy to whoever actually scored most, from the
	// eleven the model itself picked.
	//
	// It bounds better *judgement given the same data* on the largest single
	// contributor to the held metric: the variance decomposition puts the armband
	// at +4.779 points a gameweek statically, ahead of weekly re-picking and ahead
	// of autosubs. The effort is two overridden return values.
	//
	// One consequence to state rather than let someone discover: an oracle captain
	// necessarily played, so the vice-captain fallback never fires under it, and
	// the figure therefore bounds captain **and** vice jointly. That is still one
	// axis — who wears the armband — but it is not a bound on the captain alone.
	// See armbandOracle.
	AxisArmband

	// AxisTransferGate accepts or rejects the model's own proposals, knowing what
	// each one went on to return.
	//
	// The model still *proposes*; the oracle only says yes or no. That separates
	// the gate from the search, which no other measurement in this package does,
	// and it bounds the entire minimum-gain / free-transfer-charge / hit-threshold
	// family at once: no constant in that family can be worth more than a gate
	// that is right every time.
	//
	// The payoff is decisive rather than incremental. The record already puts the
	// whole gate inside a ~300-point noise band, so if a *perfect* gate over the
	// model's own proposals comes in below the transfer metric's detection
	// threshold, no constant in that family can ever be resolved on this harness
	// and the tuning programme for it closes. See gateOracle and gate.go.
	AxisTransferGate

	// AxisTransferGateXPoints is AxisTransferGate with the oracle's criterion
	// swapped from realised points to EXPECTED points from realised underlying —
	// xG for goals, xA for assists, a per-fixture exp(-xGC) for the clean sheet and
	// a Poisson floor for the concede deduction, with appearance, bonus, saves,
	// cards and defensive contribution left realised.
	//
	// # It is one arm and it decides a programme
	//
	// The proposal to score individual transfer decisions on underlying rests on
	// underlying being a sufficient statistic for whether a decision was good. This
	// tests that directly, and it is the only honest way to: both arms are still
	// SCORED on realised POLICY points, so what changes is what the oracle knows,
	// never how the season is counted.
	//
	//	perfect hindsight on POINTS      ~106 a season (recorded)
	//	perfect hindsight on UNDERLYING  this axis
	//
	// Recover most of that 106 and underlying is measuring decision quality.
	// Recover a fraction and it is not, and the programme fails here rather than
	// after a build.
	//
	// ⚠️ It exists because the two positive controls the design first named — the
	// vice-captain fallback and the perfect armband — are BYTE-IDENTICAL ZERO on
	// any transfer metric: `decide` never reads the captain, so neither can change
	// which moves are made. A gate on them could neither pass nor fail.
	//
	// ⚠️ **It is NOT a lower bound**, which a first version of this comment claimed.
	// Two biases run in opposite directions and neither has been sized: the points
	// arm optimises the very quantity both arms are scored on, which pushes the
	// ratio DOWN, while xPoints is a residual on four channels only and therefore
	// retains realised minutes, bonus, saves, cards and defcon — hindsight the
	// criterion keeps — which pushes it UP.
	//
	// ⚠️ **And this axis has not "never seen a goal".** Minutes is retained, and the
	// record calls minutes the sell side's ENTIRE error (−0.100 for a sold player who
	// keeps playing against −2.223 for the 13% who stop). What the axis is invariant
	// to is whether a shot went IN, not whether the player was there to take it.
	AxisTransferGateXPoints

	// AxisTransferGateResidual is AxisTransferGate with the criterion cut down to
	// the CONVERSION RESIDUAL alone — realised points minus expected points from
	// realised underlying, which is the part of a decision's return that underlying
	// did not predict.
	//
	// # On realised points it is a POSITIVE CONTROL and evidence of nothing
	//
	// This is the correction that matters most about this axis, and the arm was
	// nearly built with the opposite reading attached. analysis.XPoints is *defined*
	// as Points minus XPointsResidual, so the three criteria are an exact additive
	// decomposition of one quantity:
	//
	//	criterion_P = X + R − 4h    perfectGate
	//	criterion_X = X     − 4h    perfectGateXPoints
	//	criterion_R =     R − 4h    this axis
	//
	// All three arms are SCORED on realised points, and realised points *are* X + R
	// identically. An oracle that foresees any additive component of the metric it
	// is scored on, and accepts on that component's sign, raises the metric **by
	// construction**: E[X+R | R>0] > E[X] with no decision quality anywhere in the
	// mechanism. So "the residual arm gains on realised points" is the expected
	// outcome under the luck-harvesting hypothesis and under its negation alike, and
	// discriminates neither. Read it as a positive control — a *small* figure there
	// is a wiring fault, not a discovery.
	//
	// # What discriminates is the other metric
	//
	// `policy_xpoints`, the season scored on the accumulated-xPoints instrument. A
	// residual gate that improved the squad's *underlying* did something real; one
	// that moves realised points and leaves `policy_xpoints` flat harvested the
	// unforecastable component and nothing else. Realised points stays PRIMARY;
	// this is the secondary discriminator, declared in advance so that reading two
	// metrics is not two tests dressed as one.
	//
	// ⚠️ Not against zero. xPoints retains realised **bonus**, and bonus is awarded
	// largely for goals and assists — the very channels the residual replaces — so
	// this arm's `policy_xpoints` gain is expected to be positive and materially
	// smaller than the xPoints arm's. The reading is the RATIO against the xPoints
	// arm on the same metric, never a sign against nothing.
	//
	// ⚠️ **Its gain is not a share of the points arm's gain, and no ratio of the two
	// may be quoted as one.** The criteria decompose; the gains do not. The arms
	// hold different squads from week one, and each component gate charges the whole
	// four points for a hit the composite charges once.
	//
	// ⚠️ Like its siblings it retains realised minutes, bonus, saves, cards and
	// defcon — those channels are not part of what analysis.XPointsResidual
	// replaces, so they cancel out of the subtraction entirely. What is left is the
	// four replaced channels' conversion error and nothing else, which is narrower
	// than "luck" and should be quoted that way. ⚠️ And on a row with no xG at all,
	// XPointsResidual degenerates to Goals × goalPoints — "did he score", the
	// strongest hindsight in this catalogue — so the pooled figure is unreadable
	// without the coverage table TestDiagResidualXGCoverage prints.
	AxisTransferGateResidual

	// AxisTransferGateAntiResidual is AxisTransferGateResidual with the criterion's
	// sign flipped and the hit charge left alone: accept iff `−ΔR − 4h > 0`.
	//
	// # What the pair buys that neither arm buys alone
	//
	// The residual arm's `policy_xpoints` level is negative, and the open question
	// is whether that sign is information or arithmetic. Two criteria that are exact
	// negations of each other pay the same veto cost against the baseline, so the
	// contrast between them cancels that cost and leaves only the antisymmetric
	// part — which is the only part that could be information about ΔX.
	//
	// ⚠️ **Its null is NOT zero, and the offset is the size of the threshold.** The
	// two accept sets are disjoint with a dead band of width 8h, and h is zero for
	// the large majority of packages, so for most of the offered stream they
	// *partition* it. Per offered package, with μ the mean underlying gain and p the
	// residual arm's accept mass:
	//
	//	ANTI − RES = −cov(ΔX, sign ΔR) + μ·(1 − 2p)
	//
	// Only the first term is the design's quantity. The second vanishes at p = ½ or
	// μ = 0, and neither is known. AxisTransferGateAcceptAll exists to identify it —
	// read that constant before quoting this contrast against anything.
	//
	// ⚠️ **On realised points this is a NEGATIVE control by construction**, the exact
	// mirror of the sibling's positive one: `Points = X + R` identically and this
	// accepts on the sign of `−R`. Its realised-points level is guaranteed negative,
	// it is the liveness check and nothing else, and the contrast against the
	// residual arm on that metric is doubly constructed.
	//
	// ⚠️ **It is ANTI-informative, not X-uninformative.** `−ΔR` is exactly as
	// informative about ΔX as `ΔR` is, so this arm does not on its own answer what a
	// criterion carrying no information about ΔX would read. Only the pair does, and
	// only against the accept-everything reference. See perfectGateAntiResidual.
	AxisTransferGateAntiResidual

	// AxisTransferGateAcceptAll accepts every package the search proposes. It is
	// the only member of this family that is not an oracle at all: it is granted
	// hindsight and declines to read it.
	//
	// It is filed here because it replaces the same predicate through the same
	// hook, and it is worth having for two separate reasons.
	//
	// **It identifies the antisymmetric pair's null.** With `C` the veto cost both
	// gate arms share and `T` the accept-everything value of the whole proposal
	// stream, this arm's level is `C + T` while `ANTI + RES ≈ 2C + T`, so `C` and
	// `T` come out by subtraction in the run's own units and the contrast's
	// pre-registered null is `T·(1 − 2p̂)`, with `p̂` read straight off the mediator
	// as `moves(RES) / moves(ACCEPTALL)`.
	//
	// **It is the no-gate policy, which this project has never measured.** Every
	// other arm here bounds the gate-constant family from above by being right every
	// time; this bounds it from below by never refusing. The structural half of the
	// rule still binds — the week's allowance and the one-hit limit are checked
	// outside the gate — so it removes the VALUE bar and not the transfer budget.
	//
	// ⚠️ It also bypasses `shippedAccept` entirely, exactly as its siblings do, so
	// `min_gain` and `free_transfer_value` do not apply inside it. That is what
	// "no gate" means here and it is not a separate finding.
	AxisTransferGateAcceptAll
)

// implementedAxes is every decision axis with a hook behind it. Validate rejects
// the rest, for the same reason implementedInfo exists: an axis with no hook
// stamps an arm, changes nothing and reads downstream as a clean null.
var implementedAxes = map[DecisionAxis]bool{
	AxisChipWeek:                 true,
	AxisArmband:                  true,
	AxisTransferGate:             true,
	AxisTransferGateXPoints:      true,
	AxisTransferGateResidual:     true,
	AxisTransferGateAntiResidual: true,
	AxisTransferGateAcceptAll:    true,
}

// axisName is the stamp fragment for one axis.
var axisName = map[DecisionAxis]string{
	AxisChipWeek:                 "chipweek",
	AxisArmband:                  "armband",
	AxisTransferGate:             "transfergate",
	AxisTransferGateXPoints:      "transfergatexp",
	AxisTransferGateResidual:     "transfergateres",
	AxisTransferGateAntiResidual: "transfergateanti",
	AxisTransferGateAcceptAll:    "transfergateall",
}

// cellMetricColumns are the per-cell CSV columns a sweep collects a comparable
// series for, and therefore the columns MustNotMove may name.
//
// It lives here, beside the declarations, rather than in the sweep that builds
// the series: an oracle that declares an invariance on a column nothing collects
// has declared an invariance nobody checks, which is worse than declaring none.
// invarianceSeries is asserted against this list, so the two cannot drift.
// The accumulated-xPoints pair is here for the same reason the captaincy rungs
// are: `hold_xpoints` is a HOLD metric, so every axis that declares "a transfer
// decision cannot reach the held fifteen" has to be able to say so about both
// readings of it. Left out, a gate oracle that moved the xPoints column would
// pass Tier 2 while its points twin was pinned — an invariance that holds on one
// expression of a quantity and is unchecked on the other, which is this package's
// signature failure wearing the guard's own clothes.
var cellMetricColumns = []string{
	"policy_points", "hold_points", "hold_fixedcap_points", "hold_nocap_points",
	"hold_xpoints", "policy_xpoints",
	"moves", "hits",
}

// Oracles is the hindsight granted to one replay cell.
//
// It lives on SimConfig rather than in an environment variable, which buys four
// things an env var cannot:
//
//   - **Both arms toggle in one process**, so a comparison pairs properly.
//   - **Parallel-safe.** A process-global bit breaks the moment cells run
//     concurrently: one variant's os.Setenv would be read by another's cell.
//   - **No hot-path environment scan.** statusAt used to call os.Getenv twice per
//     player per gameweek per cell — tens of millions of linear scans of the
//     environment per sweep.
//   - **The stamp cannot disagree with what ran**, because it is derived from the
//     same value the simulation consumed. With an env var, provenance is a second
//     mechanism that must be kept in sync, and two expressions of one quantity
//     drifting apart silently is this package's signature bug.
//
// FeatureScope restricts OracleFeatures — never OracleTeamNews, which does not
// consult it — to one side of the model's own availability flag, so the family
// can be read as a partition rather than as one total.
//
// The two halves answer different questions, and on this archive only one of
// them has a population:
//
//   - FeaturesFlagged bounds perfect USE of information the model already has:
//     the reconstruction says "injured", the truth says whether he played.
//
//     ⚠️ **That population is nearly empty here, so the arm is uninformative
//     rather than sparse-but-live.** statusAt reconstructs from the END-of-season
//     status and the LAST news item, so it fires only on TERMINAL absences — a
//     player who returns in February finishes the season fit and reads available
//     all year, never flagged at all. Measured over six seasons and the whole
//     player pool: 12 of 32,501 flagged player-gameweeks recorded any minutes,
//     0.04%, and zero of them in two seasons. Expect under one firing across a
//     36-cell grid.
//
//     An earlier version of this comment said the reverse — that a player
//     returning in February "stays at a hard zero until May". That is REFUTED by
//     the reconstruction's own rule, and it was the reason this arm was expected
//     to be worth measuring.
//
//     So the question it was built for — what perfect use of a DOUBTFUL flag is
//     worth — belongs on OracleTeamNews, whose crawled bootstraps carry `d` and
//     carry every absence that later resolved. The scope split is not
//     implemented there.
//
//   - FeaturesUnflagged bounds what no amount of reading a flag could have told
//     you. This is the dense side and is very nearly the whole population:
//     45-50% of available-and-has-a-row player-gameweeks record zero minutes,
//     12-16% even among 1,800-minute players.
type FeatureScope int

const (
	// FeaturesAll is every player, which is the dense arm.
	FeaturesAll FeatureScope = iota
	// FeaturesFlagged is only players the honest reconstruction marks unavailable.
	FeaturesFlagged
	// FeaturesUnflagged is only players it marks available.
	FeaturesUnflagged
)

var featureScopeName = map[FeatureScope]string{
	FeaturesAll: "", FeaturesFlagged: "flagged", FeaturesUnflagged: "unflagged",
}

// admits reports whether a reconstructed status is inside this scope.
func (n FeatureScope) admits(reconstructed string) bool {
	switch n {
	case FeaturesFlagged:
		return reconstructed != "a"
	case FeaturesUnflagged:
		return reconstructed == "a"
	}
	return true
}

type Oracles struct {
	Info     InfoOracle
	Decision DecisionAxis

	// FeatureScope restricts OracleFeatures. Refused by Validate when that bit is
	// off — a scope with no oracle behind it is the same failure mode as a bit
	// with no hook, and this package refuses those by name.
	FeatureScope FeatureScope

	// FeaturesFrom is the first gameweek OracleFeatures may speak about. Zero
	// means every gameweek, which is the unrestricted arm.
	//
	// It exists to separate the two things knowing a lineup is worth, which is the
	// distinction the whole family kept collapsing.
	//
	// **Set it to the entry gameweek PLUS ONE.** The opening build is
	// `PointInTimeWith(cur, prior, start-1, ...)`, which evaluates `statusAt` at
	// the gameweek about to be played — `start` itself — so `FeaturesFrom: start`
	// fires *on the build*. That is not hypothetical: it was the first version, and
	// the opening fifteen differed in 35 of 36 cells until the squad invariance
	// caught it. At `start+1` the fifteen comes back **byte-identical**, which is
	// the checkable form of the claim.
	//
	// That is the question a manager is actually asking. You find out an hour
	// before the deadline that your midfielder is not in the squad; the value of
	// knowing is that you *sell him and buy someone who is playing*, not that you
	// reorder your bench. Bench reordering was measured first, at **+0.1005 points
	// per blank — about 7.0 over 38 gameweeks** (TestDiagAutosubValue), and it is a
	// bound on the wrong channel. ⚠️ It is recorded elsewhere in this file's
	// history as "+4.7 a season"; that was a `pooled/36` conversion and is
	// retracted — see the units warning above `OracleFeatures`.
	//
	// It also removes a real artifact rather than only isolating a channel.
	// `Optimize` drops `"unavailable"` players from the **opening-squad** pool
	// outright, so without this gate a one-gameweek absence bars a player from the
	// initial build — 391 of 690 registered players at a GW1 entry on 2025-26. The
	// weekly transfer search is unaffected either way, since it ranks over
	// `AllMetrics` rather than the filtered pool, so he can still be bought later.
	FeaturesFrom int
	// Composite permits one information and one decision oracle in the same arm.
	// Off by default and refused by the reporting path, because such a figure
	// bounds neither "better data" nor "better judgement given the same data". A
	// forbidden combination is cheaper to relax later than a shipped ambiguity is
	// to retract.
	Composite bool

	// News is the recovered team news OracleTeamNews reads, and nil for every
	// other arm.
	//
	// It is on this value rather than on SimConfig for the reason the whole type
	// exists: the stamp is derived from the same value the simulation consumed, so
	// an arm cannot claim hindsight it was not given. A source attached with no bit
	// set is inert and harmless; a bit set with no source is refused by Validate,
	// because that is the silent-null direction.
	//
	// **The dynamic type must be comparable** — a pointer, in practice.
	// runPolicySweep compares two Oracles with != to check that an arm's hindsight
	// does not vary by cell, and a map- or slice-backed implementation would panic
	// there rather than fail a check. See TeamNews.
	News TeamNews
}

// Has reports whether an information oracle is on.
func (o Oracles) Has(i InfoOracle) bool { return o.Info&i != 0 }

// Active reports whether this cell has any hindsight at all.
func (o Oracles) Active() bool { return o.Info != 0 || o.Decision != AxisNone }

// Reportable is false for an arm whose figure must never leave the test that
// produced it.
//
// Only omniscience is unreportable, and the reason is not that its number is
// large — every oracle's number is an upper bound and several are large. It is
// that omniscience bounds *nothing a capability could be*: it is an arm told the
// answers, built to prove the apparatus transmits information at all. A figure
// with no referent is the worst thing to leave lying in a means file, because a
// hindsight bound already looks exactly like a score and this one has not even
// got a question attached.
//
// Enforced at the sweep boundary rather than at the means row — see
// omniscience.go for why refusing only the means row, or only the cells, is
// weaker than refusing the arm.
func (o Oracles) Reportable() bool { return !o.Has(OracleOmniscient) }

// Validate refuses a combination that would produce an uninterpretable figure,
// or one that would silently measure nothing.
func (o Oracles) Validate() error {
	if extra := o.Info &^ implementedInfo; extra != 0 {
		return fmt.Errorf("oracle: information bits %#b have no implementation — "+
			"a bit with no hook stamps an oracle and measures nothing, which is "+
			"indistinguishable from a real null", uint(extra))
	}
	if o.Decision != AxisNone && !implementedAxes[o.Decision] {
		return fmt.Errorf("oracle: decision axis %d has no implementation — the "+
			"axes named by the oracle-design document land with their hooks, one commit each",
			int(o.Decision))
	}
	// Minutes and lineups are two *resolutions* of one quantity and both rewrite
	// MinutesPerMatch on the same seam, so an arm carrying both is not a
	// composition — it is one oracle silently winning over the other, and its
	// stamp would name a decomposition it did not run. Refused rather than
	// ordered, because whichever order recentIndex happened to test first would
	// become an undocumented precedence rule.
	if o.Has(OracleMinutes) && o.Has(OracleLineups) {
		return fmt.Errorf("oracle: minutes and lineups are two resolutions of one " +
			"quantity on one seam, so composing them measures neither — lineups is " +
			"the reachable half of minutes and the pair is a decomposition, run as " +
			"two arms against a common baseline")
	}
	// A percentage read through a reconstructed flag bounds neither arm of the
	// decomposition it belongs to. availabilityFactor takes the percentage in
	// preference to the status, so this combination would price flagged players on
	// recovered data and everyone else on statusAt — two availability models inside
	// one figure.
	if o.Has(OracleTeamNewsChance) && !o.Has(OracleTeamNews) {
		return fmt.Errorf("oracle: the published chance of playing needs the real " +
			"status underneath it — availabilityFactor prefers the percentage to the " +
			"flag, so a chance-only arm mixes recovered news with the reconstruction " +
			"and bounds neither. Run teamnews and teamnews+teamnews_chance as two " +
			"arms against a common baseline")
	}
	// The one refusal that catches the failure this package keeps paying for: a
	// declared oracle with no data behind it, which changes nothing, passes every
	// negative control and reports a clean null indistinguishable from a real one.
	// Refused here rather than by the liveness check, because the liveness check is
	// only reached after the sweep has already run.
	if o.Has(OracleTeamNews) && o.News == nil {
		return fmt.Errorf("oracle: %s carries no TeamNews source, so it would replay "+
			"the reconstruction and report it as recovered team news — an arm that "+
			"reaches nothing reports the same clean null as a real one", o.Stamp())
	}
	// Two resolutions of one fact on one seam, exactly as minutes and lineups are.
	// Both rewrite Element.Status, so an arm carrying both is not a composition: it
	// is one oracle silently winning over the other, and its stamp would name a
	// decomposition it did not run.
	if o.Has(OracleTeamNews) && o.Has(OracleAvailability) {
		return fmt.Errorf("oracle: teamnews and availability are two resolutions of " +
			"one fact on one seam (Element.Status), so composing them measures " +
			"neither — availability is the degenerate case that fires on a season " +
			"total of zero minutes, and the pair is a decomposition")
	}
	if o.Info != 0 && o.Decision != AxisNone && !o.Composite {
		return fmt.Errorf("oracle: %s mixes an information oracle with a decision "+
			"oracle, which bounds neither better data nor better judgement given "+
			"the same data. Set Composite if that ambiguity is deliberate", o.Stamp())
	}
	// Availability fires on a season TOTAL and features on the coming gameweek, so
	// both write Status and whichever ran last would win. Same seam clash as
	// minutes-against-lineups, refused for the same reason: features subsumes
	// availability, since a player with no minutes all season has none in any
	// single gameweek either.
	if o.Has(OracleAvailability) && o.Has(OracleFeatures) {
		return fmt.Errorf("oracle: availability and features both rewrite Status " +
			"on one seam, and features subsumes availability — a player with no " +
			"minutes all season has none in any gameweek. Run them as two arms")
	}
	// The pair that is genuinely tempting to compose, and must not be. Recovered
	// team news is what FPL PUBLISHED at the deadline; features is what turned out
	// to be TRUE. They write the same field, so an arm carrying both reports
	// whichever ran last — and the interesting quantity is the *contrast* between
	// them, which is what a better source than FPL could be worth. A single figure
	// cannot carry a contrast.
	if o.Has(OracleTeamNews) && o.Has(OracleFeatures) {
		return fmt.Errorf("oracle: teamnews and features both rewrite Element.Status " +
			"— one with what FPL published at the deadline, one with what turned out " +
			"to be true. The quantity worth having is the gap between them, so run " +
			"them as two arms against a common baseline")
	}
	if o.FeaturesFrom != 0 && !o.Has(OracleFeatures) {
		return fmt.Errorf("oracle: FeaturesFrom %d is set with no features oracle "+
			"to gate, which would stamp a restriction and measure the baseline",
			o.FeaturesFrom)
	}
	if o.FeatureScope != FeaturesAll && !o.Has(OracleFeatures) {
		return fmt.Errorf("oracle: FeatureScope %q is set with no features oracle to "+
			"restrict, which would stamp a restriction and measure the baseline",
			featureScopeName[o.FeatureScope])
	}
	if o.Composite && (o.Info == 0 || o.Decision == AxisNone) {
		return fmt.Errorf("oracle: Composite is set with nothing to compose " +
			"(it permits one information and one decision oracle in one arm)")
	}
	return nil
}

// Kind is the coarse classification written beside the stamp in the per-cell CSV:
// none, info, decision or composite.
//
// Two columns rather than one because they answer different questions. The stamp
// says *which* oracle and joins to a declaration; the kind says what class of
// bound the number is, which is what stops an information figure being quoted as
// a judgement figure.
func (o Oracles) Kind() string {
	switch {
	case o.Info != 0 && o.Decision != AxisNone:
		return "composite"
	case o.Info != 0:
		return "info"
	case o.Decision != AxisNone:
		return "decision"
	default:
		return "none"
	}
}

// Stamp is the canonical, sorted, stable join key for an oracle state.
//
// "-" when inert, never blank: blank means "not measured" in the cell schema, and
// every row does know its oracle state. Otherwise "info:availability",
// "info:availability+prices", and — once an axis exists — "decision:armband".
func (o Oracles) Stamp() string {
	var parts []string
	if names := o.infoNames(); len(names) > 0 {
		parts = append(parts, "info:"+strings.Join(names, "+"))
	}
	// A restriction is part of what ran, so it has to be part of the stamp. Two
	// arms restricted to opposite halves of the pool are different measurements,
	// and stamping both "info:features" would make them unjoinable to their own
	// declarations — the one guarantee this stamp exists to give. Empty for
	// FeaturesAll, so every unrestricted arm's stamp is unchanged and no recorded
	// figure moves.
	if n := featureScopeName[o.FeatureScope]; n != "" {
		parts = append(parts, "scope:"+n)
	}
	if o.FeaturesFrom != 0 {
		parts = append(parts, fmt.Sprintf("from:%d", o.FeaturesFrom))
	}
	if o.Decision != AxisNone {
		name, ok := axisName[o.Decision]
		if !ok {
			// Validate refuses this, but Stamp is called from reporting paths that
			// do not, and a stamp that quietly named the axis by number would be
			// unjoinable to any declaration.
			name = fmt.Sprintf("axis%d", int(o.Decision))
		}
		parts = append(parts, "decision:"+name)
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "|")
}

// infoNames lists the information oracles on, in the canonical order.
func (o Oracles) infoNames() []string {
	var out []string
	for _, bit := range infoOrder {
		if o.Info&bit != 0 {
			out = append(out, infoName[bit])
		}
	}
	// An undeclared bit is a programming error Validate refuses, but Stamp is
	// called from reporting paths that do not, and a stamp that silently omitted
	// the bit would be a lie in the one column that exists to prevent lies.
	if extra := o.Info &^ implementedInfo; extra != 0 {
		out = append(out, fmt.Sprintf("unknown%#b", uint(extra)))
	}
	return out
}

// MustNotMove returns the per-cell CSV columns this oracle must leave *exactly*
// as the un-oracled baseline left them.
//
// Falsification is far cheaper than confirmation here: a violated invariance
// shows up in one cell, where confirming an effect on the transfer metric needs a
// season-scale effect this harness can barely resolve. So every oracle declares
// what it must not move and the harness checks it, rather than an operator
// checking a column by eye — which is how the price oracle's invariance was
// verified until now.
//
// # Combining declarations is an intersection, not a union
//
// Each bit declares what *it* cannot move. With availability and prices both on,
// the held rungs legitimately move, because availability changes which fifteen is
// bought. A union would demand they stay put and fail a correct run; the
// intersection is the honest combined claim. An oracle that declares nothing —
// availability — therefore silences the check for any arm it appears in, which is
// correct and worth knowing before reading a composite arm's invariance as proof
// of anything.
func (o Oracles) MustNotMove() []string {
	first := true
	var out []string
	for _, bit := range infoOrder {
		if o.Info&bit == 0 {
			continue
		}
		cols := mustNotMoveFor(bit)
		if first {
			out, first = cols, false
			continue
		}
		out = intersect(out, cols)
	}
	if o.Decision != AxisNone {
		cols := mustNotMoveForAxis(o.Decision)
		if first {
			out = cols
		} else {
			out = intersect(out, cols)
		}
	}
	return out
}

// mustNotMoveForAxis is one decision axis's declaration.
//
// The chip-week axis declares **everything**, which is the strongest statement in
// the catalogue and the reason it is built first: chips are scored off per-week
// gains the replay already records, so the oracle reads a slice and changes no
// decision at all. Any movement in any collected metric means the argmax has
// somehow reached the simulation, and the figure would be measuring something
// other than chip timing.
func mustNotMoveForAxis(a DecisionAxis) []string {
	switch a {
	case AxisChipWeek:
		return append([]string(nil), cellMetricColumns...)
	case AxisArmband:
		// Four free, razor-sharp checks from columns every sweep already emits.
		//
		// `decide` never reads the captain, so a hindsight armband that changed a
		// transfer count or a hit would be reaching an axis it declared it cannot
		// reach — and a transfer count is an integer counted without noise, where a
		// fraction of a point a gameweek is not visible at all.
		//
		// The no-captain rung doubles nobody, so it cannot see an armband. The
		// fixed-captain rung pins the armband to the day-one pick, which is read
		// from an un-oracled engine and deliberately left that way: an oracled
		// "frozen" captain would be neither frozen nor an oracle, and leaving it
		// alone buys a fourth invariance instead.
		return []string{"moves", "hits", "hold_nocap_points", "hold_fixedcap_points"}
	case AxisTransferGate:
		// All three held rungs, and HOLD's xPoints reading with them. The held
		// metric buys the opening fifteen and never transfers, so a gate — however
		// clairvoyant — cannot reach it on either metric. If any of them moves, the
		// oracle is changing squad selection rather than transfer acceptance, and
		// its figure would bound something other than the gate.
		return []string{"hold_points", "hold_fixedcap_points", "hold_nocap_points",
			"hold_xpoints"}
	case AxisTransferGateXPoints:
		// ⚠️ This case was MISSING and fell through to the default's nil, so Tier 2
		// checked nothing at all for the arm — while the banner announced it as
		// "declares no cell invariance; it rests on the input diff", a guarantee that
		// structurally cannot cover a DECISION axis, since the input diff covers
		// InfoOracle only. An arm announced as resting on a check that does not exist
		// for it is worse than one announced as unchecked.
		//
		// Nothing caught it: TestMustNotMoveNamesRealColumns validates the names of
		// whatever is declared, so an empty list passes vacuously.
		//
		// The same three rungs as the sibling axis and for the same reason — the held
		// metric buys the opening fifteen and never transfers, so a gate cannot reach
		// it whatever criterion it judges on. The R inference did report exact zeros
		// on all three, but verifying after the fact in another language is not the
		// same as declaring it here.
		//
		// `hold_xpoints` joins them: it is HOLD scored the other way, so the same
		// sentence is true of it, and it is the column this axis is most likely to
		// be quoted beside — the gate it bounds judges on xPoints.
		return []string{"hold_points", "hold_fixedcap_points", "hold_nocap_points",
			"hold_xpoints"}
	case AxisTransferGateResidual:
		// The same four as both siblings, and written out rather than shared with
		// them: an axis that declares its invariance by pointing at another axis's
		// list stops being able to disagree, and these three declarations are three
		// separate claims that happen to coincide today.
		//
		// The claim is the same one and it is as strong here: the held metric buys
		// the opening fifteen and never transfers, so no gate reaches it, whatever
		// criterion the gate judges on. `hold_xpoints` is included for the reason
		// recorded on the sibling — HOLD scored the other way is still HOLD, and an
		// invariance that holds on one expression of a quantity and is unchecked on
		// the other is this package's signature failure.
		//
		// ⚠️ Non-empty is the point. The sibling's case above was MISSING and fell
		// through to the default's nil, so its arm declared invariances it never
		// checked while TestMustNotMoveNamesRealColumns passed vacuously over an
		// empty list. TestEveryDecisionAxisDeclaresAnInvariance now fails on nil, so
		// the same omission cannot recur silently on a fourth axis.
		return []string{"hold_points", "hold_fixedcap_points", "hold_nocap_points",
			"hold_xpoints"}
	case AxisTransferGateAntiResidual:
		// The fourth statement of the same four columns, written out for the reason
		// the case above gives: an axis that declares its invariance by pointing at
		// a sibling's list stops being able to disagree with it, and these are
		// separate claims that happen to coincide.
		//
		// The claim holds here for the same reason it holds for all three siblings,
		// and the sign flip does not weaken it: the held metric buys the opening
		// fifteen and never transfers, so no gate reaches it whatever criterion it
		// judges on and whichever way that criterion points. `hold_xpoints` is in
		// the list because HOLD scored the other way is still HOLD.
		//
		// ⚠️ Non-empty is the point. AxisTransferGateXPoints once fell through to
		// the default's nil and checked nothing at all while its banner announced a
		// guarantee that structurally could not cover it.
		return []string{"hold_points", "hold_fixedcap_points", "hold_nocap_points",
			"hold_xpoints"}
	case AxisTransferGateAcceptAll:
		// The same four again, written out again, and the argument reaches this arm
		// even though it is not an oracle: what pins the held rungs is that HOLD
		// makes no transfers, so a *predicate over transfers* cannot reach it. An
		// arm that accepts every transfer is still only a predicate over transfers.
		//
		// This is the arm where the temptation to share a list is strongest, since
		// it is the one member of the family with no hindsight — and sharing would
		// be precisely how a real disagreement, if one ever arose, would be hidden.
		return []string{"hold_points", "hold_fixedcap_points", "hold_nocap_points",
			"hold_xpoints"}
	default:
		return nil
	}
}

// mustNotMoveFor is one bit's declaration.
//
// The declarations differ sharply, which is the useful part. The price oracle
// cannot reach squad building at all, so all three held rungs are pinned — that
// is the check which proved it reaches only the transfer path. The availability
// oracle legitimately moves every metric, so it declares nothing and rests on the
// input diff instead.
func mustNotMoveFor(bit InfoOracle) []string {
	switch bit {
	case OracleTransactPrice:
		// `hold_xpoints` joins the three rungs on the same argument: the price
		// oracle cannot reach squad building, and that is a claim about the held
		// fifteen rather than about how its weeks are scored.
		return []string{"hold_points", "hold_fixedcap_points", "hold_nocap_points",
			"hold_xpoints"}
	default:
		return nil
	}
}

// MustMove returns the per-cell CSV columns this oracle must move in at least one
// cell — the *liveness* declaration, and the mirror of MustNotMove.
//
// # Why a negative control is not enough
//
// Every Tier 1 and Tier 2 guarantee in this package is a refusal: an oracle
// declares what it cannot reach, the harness checks, and passing means nothing
// moved. An arm that is wired wrong and reaches *nothing* passes all of them
// perfectly. That is the failure OracleOmniscient exists to catch in general, and
// it is a genuine hole for one implemented oracle in particular.
//
// ⚠️ **"OracleTransactPrice is the arm with no liveness evidence of any kind" was
// true when written and is not the reason this exists any more** — every
// implemented info bit now declares `moves`. The paragraph is kept because the
// *argument* is what generalises, and because it names the one arm whose headline
// is a null and so has the least to fall back on.
//
// The liveness check earns its keep on a hazard the negative controls cannot see:
// `statusAt` short-circuits on `FPL_NO_AVAILABILITY` **before** any oracle branch
// (simulate.go), so a sweep run with that variable exported turns every
// Status-writing oracle into a total no-op while every arm still runs and still
// stamps itself. Tier 1 would catch it, and Tier 1 is a unit test that does not
// run inside a sweep.
//
// **OracleTransactPrice is the arm with no liveness evidence of any kind.** It
// declares no bootstrapFields, so Tier 1 has nothing to observe; MustNotMove is a
// must-*not* set, so Tier 2 can only confirm absence; its diagnostic asserts
// nothing beyond running the sweep; and its headline is a null. An inert arm
// reports exactly that. Compare the minutes oracle, whose mediator is counted
// (607 of 612 squads differ), and the armband, whose mediator is counted (480 of
// 612 captains differ).
//
// `moves` is the right column for it and the headline is not. Judging liveness on
// `policy_points` would prove the arm live from the same number the arm exists to
// report, which is circular; a transfer count is an integer counted without
// noise, and on the corrected arm it differs in 18 of 24 cells while the *total*
// barely moves (595 against 593). A mediator that is invisible in the aggregate
// and unmistakable per cell is exactly what this check is for.
//
// The union rather than the intersection: each bit's liveness claim is its own,
// and an arm composing two oracles should show both of them working.
func (o Oracles) MustMove() []string {
	var out []string
	seen := map[string]bool{}
	add := func(cols []string) {
		for _, c := range cols {
			if !seen[c] {
				seen[c], out = true, append(out, c)
			}
		}
	}
	for _, bit := range infoOrder {
		if o.Info&bit != 0 {
			add(mustMoveFor(bit))
		}
	}
	if o.Decision != AxisNone {
		add(mustMoveForAxis(o.Decision))
	}
	return out
}

// mustMoveFor is one bit's liveness declaration.
//
// `moves` for the three oracles that legitimately move every metric, and it is
// the right column for all of them for the same reason it is right for prices:
// judging liveness on the headline would prove the arm live from the number the
// arm exists to report, and a transfer count is an integer counted without noise.
// Minutes and lineups both have a counted mediator in their diagnostic as well —
// how many gameweeks the held fifteen differs — but a mediator that lives in a
// test's own printout is only checked when that test is run, where this is
// checked by the harness after every grid.
//
// Empty for omniscience, which is a fixture that never reaches a sweep.
func mustMoveFor(bit InfoOracle) []string {
	switch bit {
	case OracleTransactPrice, OracleMinutes, OracleLineups, OracleAvailability,
		OracleTeamNews, OracleFeatures:
		return []string{"moves"}
	case OracleTeamNewsChance:
		// Deliberately empty, and this is the one place the union in MustMove
		// matters. The percentage only ever composes with OracleTeamNews, which
		// already claims `moves`; claiming it here as well would let the *pair*
		// pass its liveness check on the flag arm's behaviour while the percentage
		// itself was inert. Its liveness is Tier 1's instead — it declares
		// ChanceOfPlayingNextRound and the input diff fails if that never moves.
		return nil
	default:
		return nil
	}
}

// mustMoveForAxis is one decision axis's liveness declaration.
//
// **Most are empty, and the chip-week axis is why the field cannot simply be
// "something must move"**: it declares that *everything* must stay put, because its
// whole output is SimResult.ChipOracle and it changes no decision at all. Its
// liveness is checked by its observe hook instead. A blanket rule would have failed
// the one arm whose invariance is total.
//
// ⚠️ This comment read "All three are empty" while the catalogue had five axes and
// then seven — stale from the commit that added the second gate arm. It is now
// phrased as a rule rather than a count, because a count of the cases below is
// exactly the thing that goes stale beside them.
//
// The two gate arms added 2026-08-16 declare `moves`, and the reason is that the
// harness checks this after **every** grid while a diagnostic's own assertions only
// run when somebody runs that diagnostic — and these two are the arms whose bespoke
// checks a review found weakest. For the accept-everything arm it is guaranteed by
// construction rather than merely expected: `moveLimit` bounds every arm at
// `free + 1` moves a week and an arm that refuses nothing reaches the bound every
// week, so it cannot leave a transfer count where the shipped gate left it.
func mustMoveForAxis(a DecisionAxis) []string {
	switch a {
	case AxisTransferGateAntiResidual, AxisTransferGateAcceptAll:
		// Written as one case deliberately: this is a claim about the *column*
		// rather than about either criterion, and unlike a must-NOT-move list there
		// is nothing here for two axes to disagree about — `moves` is `moves`.
		return []string{"moves"}
	default:
		return nil
	}
}

func intersect(a, b []string) []string {
	in := map[string]bool{}
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if in[s] {
			out = append(out, s)
		}
	}
	return out
}

// bootstrapFields declares the fpl.Element fields an information oracle may
// perturb on the bootstrap PointInTimeWith returns.
//
// This is the Tier 1 guarantee, and it is the one neither existing oracle had: a
// table-driven test replays PointInTime against PointInTimeWith over every season
// pair and every gameweek, diffs the two bootstraps field by field, and fails if
// anything outside this set differs. Seconds, and no replay.
//
// It matters most for the oracle that does not exist yet. Minutes multiply into
// every rate, so a minutes oracle that touched a per-90 rate would quietly become
// a points oracle, and no amount of care at the call site catches that as
// reliably as a field diff does.
//
// An empty list is a real declaration meaning "this oracle changes nothing the
// model reads at the information seam" — which is exactly the price oracle's
// claim, since it perturbs the transaction seam instead.
func (i InfoOracle) bootstrapFields() []string {
	switch i {
	case OracleAvailability, OracleFeatures:
		// Both rewrite exactly one field, and it is the same one — which is why
		// Validate refuses them in the same arm. They differ only in resolution:
		// availability fires on a season total, features on the coming gameweek.
		return []string{"Status"}
	case OracleTransactPrice:
		return nil
	case OracleMinutes, OracleLineups:
		// The bootstrap comes back byte-identical, which is the *stronger* of the
		// two claims these oracles make and the one the design most insists on:
		// minutes there are the denominator of every counting rate, so an oracle
		// that touched them would silently become a points oracle. They perturb the
		// recency index instead, which carries its minutes fields separately from
		// its rate fields — pinned by TestMinutesOraclePerturbsOnlyMinutes.
		return nil
	case OracleTeamNews:
		// The same single field the reconstruction writes, which is the point: this
		// is a *replacement* for statusAt and not an addition to it. Anything else
		// moving would mean the recovered payload had reached a price, a minute or a
		// registration, and the join is on permanent player code precisely so it
		// cannot reach a different footballer.
		return []string{"Status"}
	case OracleTeamNewsChance:
		// The field nothing in the replay has ever set. Declared separately from
		// Status so the two arms of the decomposition have different Tier 1
		// signatures — an arm that claimed the percentage and moved only the flag
		// would be caught by the mediator rather than by a reader's attention.
		return []string{"ChanceOfPlayingNextRound"}
	case OracleOmniscient:
		// The one oracle that *is* allowed to become a points oracle, because that
		// is what it is for. The declaration still matters, and what it excludes is
		// the useful half: price, availability, club and position are absent, so an
		// omniscient arm cannot quietly be a composition of oracles reported as one.
		return omniscientFields
	default:
		return nil
	}
}

// OraclesFromEnv seeds an oracle state from the environment.
//
// The environment is demoted to a *seed*: it is read once where a cell config is
// constructed, never on the hot path and never as the authority. The names are
// kept because they appear throughout the research record, and a figure recorded
// against FPL_ORACLE_PRICES must stay findable.
//
// **Neither may ever become a default.** Every figure in AGENTS.md was measured
// without them, so switching one on would inflate all of them at once and make
// the record incomparable with itself.
func OraclesFromEnv() Oracles {
	var o Oracles
	if os.Getenv("FPL_ORACLE_AVAILABILITY") != "" {
		o.Info |= OracleAvailability
	}
	if os.Getenv("FPL_ORACLE_PRICES") != "" {
		o.Info |= OracleTransactPrice
	}
	return o
}
