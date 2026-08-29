package backtest

// Is FPL's difficulty ladder an adequate stand-in for the opponent's own defence?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagFixtureReconciliation -v -timeout 60m
//
// # What this gates
//
// The engine scales a team's attacking rate by `analysis.AttackMultiplier`, which
// is a five-rung ladder keyed on FPL's own 1-5 fixture difficulty. On that path it
// never consults the opponent's *own modelled defensive estimate*, even though the
// engine computes one for every player in the league — `XGC90`, blended, shrunk and
// read point-in-time exactly like the attacking rates are. FPL's difficulty is an
// editorial rating with five values; our defensive estimate is continuous, moves
// weekly, and is built from the same archive the rest of the model is built from.
// Substituting one for the other is the reconciliation nobody has priced, and it is
// testable as prediction rather than as an argument.
//
// # ⚠️ The obvious version of this test is wrong, and this is not it
//
// The tempting design is: for one fixture, compare the home side's attacking
// estimate against the away side's defensive estimate and call the gap an
// inconsistency. **That would manufacture a finding.** The attacking estimate is
// "home's goals against an *average* opponent"; the defensive estimate is "away's
// concessions against an *average* opponent". Neither is the match expectation, and
// in the standard form the match expectation is roughly `home attack x away
// defensive weakness / league average`. So a strong attack meeting a strong defence
// **should** produce two different numbers. Their divergence is not error — it is
// the information a joint model exists to combine, and a diagnostic built to drive
// it to zero would be driving out signal.
//
// So nothing here compares the two sides of a fixture. Every arm predicts **one**
// quantity, per team per match, and is scored against what that team actually did.
//
// # The arms
//
// Each predicts a team's expected goals in one match. All three share the same
// fixture-blind attacking rate, built at the cutoff, and differ only in the
// multiplier applied to it — which is the whole point: the multiplier is the only
// thing under test.
//
//	rate = sum over the club's registered players of XG90 x XGScale x ExpectedMinutes / 90
//
// the identical expression `TestDiagTeamGoalShare` uses, for the reason it gives
// there: every factor is already a reported multiplier on PlayerMetrics, and
// summing across positions with different XGScale values is right for a club-level
// question because FPL pays a defender's goal more and still counts it once.
//
//   - **arm 0 (reference, not in the decision rule)** — multiplier 1.0. No fixture
//     term at all. It is here because "A beats B" is uninteresting if neither beats
//     a constant: if both ladders land on top of this row, the engine's fixture
//     response is carrying nothing at this horizon and the question of *which*
//     opponent signal to use does not arise.
//   - **arm A (shipped)** — `analysis.AttackMultiplier(this team's FPL difficulty
//     for this fixture)`. It reads the shipped function rather than restating the
//     ladder, so this asks about the ladder that ships and not one nothing uses.
//   - **arm B (reconciled)** — the opponent's own modelled defensive strength,
//     divided by the league mean of that quantity so it is a multiplier on the same
//     scale as A, centred on 1 the way the ladder's middle rung is.
//   - **arm C (blend)** — a geometric blend of A's and B's multipliers, swept over
//     0.25 / 0.50 / 0.75. Geometric because the metric is a log-scale one, so the
//     blend and the loss live in the same space.
//
// # How a club's defence is aggregated, and why it is a mean rather than a sum
//
// `XGC90` is per-90 **while the player is on the pitch**, and all eleven players
// concede the same goals simultaneously. So a club's defensive rate is a weighted
// *mean* of its players' rates, not a sum: summing would produce a figure about
// eleven times the true one and would additionally scale with squad size, which is
// not a football quantity. The weight is `ExpectedMinutes`, so a player the model
// expects to play concedes-weight in proportion to how much of the club's defending
// he is expected to be present for, and a fringe player's noisy rate is discounted
// rather than counted equally:
//
//	defence = sum(XGC90 x ExpectedMinutes) / sum(ExpectedMinutes)
//
// **`TeamXGCFactor` needs no separate application here.** `Engine.Metrics` already
// multiplies it into `m.XGC90` before returning, so reading `m.XGC90` reads the
// corrected figure; applying it again would square it. That is the desynchronised-
// mirror bug class this project keeps a list of, and the cheapest way not to have it
// is to read the one place the quantity is assembled.
//
// The league mean is the unweighted mean **across clubs**, which is the denominator
// that makes the arm a multiplier on 1 for an average opponent. A minutes-weighted
// league mean would weight clubs by squad depth, which is not what "average
// opponent" means.
//
// # The target, and what it is not
//
// Primary: the team's realised **expected** goals in the match. Realised goals in a
// single match are close to Poisson and are mostly variance a fixture multiplier
// cannot reach, so scoring against them would grade every arm on finishing. They are
// reported in the second column anyway, as the contrast, on the precedent
// `TestDiagTeamGoalShare` set — the gap between the two columns is the finishing
// noise the naive design would have mistaken for a result.
//
// The unit of observation is a **team-gameweek**, not a team-fixture, because the
// archive's player rows are per gameweek and a double gameweek is one row covering
// two matches. So realised expected goals are divided by the number of matches the
// club actually played that gameweek, and the arms' multipliers are averaged over
// the same matches. Both sides are then per-match figures over the same set of
// fixtures, which is the only way the ratio means anything. A gameweek where the
// fixture list and the played record disagree on the count is dropped and counted,
// rather than divided by whichever number came to hand.
//
// # Metric
//
// Root-mean-square of `log(predicted / realised)`, so being 2x high and 2x low count
// alike. The same metric `TestDiagTeamGoalShare` scores its club-form blend on,
// chosen for comparability with the record rather than freshly invented.
//
// # ⚠️ The noise floor is computed FIRST, and every threshold is stated against the
// excess over it
//
// This project has pre-registered a threshold against a raw quantity that contained
// a floor larger than the effect, and had to retire it rather than apply it. So the
// floor comes first here and the decision rule never touches a raw number.
//
// Two floors are computed, and the difference between them is itself a result.
//
// **The pre-registered instrument is `splitHalfFloor`**, the method
// `TestDiagTeamGoalShare` already carries: each club's held-out window cut into two
// halves of its own matches, the log ratio of the two half-window means, the spread
// of that across clubs, halved. It is computed first and printed first.
//
// ⚠️ **It measures the wrong unit for this diagnostic, and the conversion does not
// survive contact with the data.** `splitHalfFloor` is the sampling sd of a
// **full-window mean**; every arm here is scored on **single matches**. Converting
// one to the other means multiplying by sqrt(matches per club), on the reasoning
// that a mean of k draws has 1/k the variance. That reasoning is exact for a mean of
// logs and only approximate for the log of a mean, and expected goals per match is
// right-skewed enough for the gap to matter: for a lognormal with log-sd sigma the
// log of a k-match mean has variance `(exp(sigma^2)-1)/k` rather than `sigma^2/k`,
// which is 29% high at sigma = 0.7. Multiplied back up by sqrt(k) the error
// compounds, and **measured, the extrapolated floor exceeds the raw rms of both arms
// in two of six seasons** — an excess of exactly zero for both, a ratio of two zeroes,
// and a season contributing a fabricated 0 to the cluster mean. A floor larger than
// the quantity it is a floor for is a defect of the extrapolation, not a measurement,
// and applying a decision rule through it would be the Gap 3 mistake wearing the
// opposite hat: a threshold stated against a quantity that cannot carry it.
//
// **So the floor actually applied is measured on the unit the arms are scored on**,
// with no extrapolation in it: the rms log error of a **per-club oracle** that
// predicts each club's own geometric mean expected goals over the held-out window.
// It is the same idea — a club against itself, no model — asked directly at
// single-match resolution instead of at window resolution and scaled. It is the best
// any fixture-blind predictor could do even knowing the answer, so an arm below it
// is an arm whose fixture term has beaten a club-level oracle.
//
// ⚠️ **Read what either floor contains.** It is the spread of a club's single
// matches around *its own window mean*, so it holds both the realisation noise no
// model can reach **and** the genuine match-to-match variation in the true
// expectation — which is exactly what the arms are trying to predict. Both are
// therefore **upper bounds** on the irreducible floor, and the excess left over is a
// **lower bound** on each arm's error.
//
// That bound points in an uncomfortable direction and it is stated rather than
// buried: subtracting too much from both arms *inflates* the percentage gap between
// them, because the subtraction shrinks the smaller excess proportionally more. So
// the excess-based gain is generous to arm B. A B that fails to clear its bar on the
// excess column has failed on the friendlier of the two readings, which makes a null
// there robust; a B that clears it only on the excess column and not on the raw one
// has not established anything. Both columns are printed for that reason, and the
// decision is stated on both.
//
// # Two known asymmetries between the arms, both favouring A
//
// Neither is a defect to be fixed here; both are reasons a null is safe and a win
// would be understated.
//
//  1. **A carries home advantage and B does not.** FPL's difficulty differs for the
//     home and away side of the same fixture, so arm A gets a venue term for free.
//     Arm B is the opponent's defensive rate, which has no venue in it at all. This
//     is the arm as pre-registered, and adding a venue term to it would be building
//     the joint model rather than testing whether the substitution is worth building.
//  2. **A's difficulty ratings are the archive's end-of-season values.** FPL revises
//     team strength mid-season and the revisions are outcome-driven, so a
//     difficulty read from `fixtures.csv` for a GW30 match may embed information
//     that did not exist at GW19. The engine's defensive estimate carries no such
//     hindsight: it is `Metrics` at the cutoff and nothing after it. So arm A is
//     given a small amount of hindsight this diagnostic cannot remove, and arm B is
//     given none.
//
// Everything else is point-in-time. The fixture list the arms read is the one
// `PointInTime` returns, whose scorelines and `Finished` flags are stripped past the
// cutoff; realised outcomes come from the raw season and are never touched by the
// prediction side. Only players registered at the cutoff enter either side, for the
// reason `TestDiagTeamGoalShare` records: a January arrival is not something a GW19
// model got wrong, and leaving him in the denominator alone makes the club that
// bought him read as under-rated.
//
// # The decision rule, committed before the numbers were seen
//
// On arm B against arm A, by rms log error, stated on the excess over the floor:
//
//   - **B beats A by more than 5% and clears a season-clustered t of 3.182 on four
//     clusters (three degrees of freedom):** the reconciliation is real, and the
//     item escalates to a replay — which this work does **not** authorise and which
//     needs its own decision.
//   - **B beats A but does not clear season-clustering:** unresolved, and not built.
//     This is the outcome the record predicts most often; a real effect of moderate
//     size looks exactly like this on four clusters.
//   - **B does not beat A:** the hypothesis dies. The fixture-difficulty null result
//     does not trace to the ladder being the wrong opponent signal.
//
// ⚠️ **The grid is `sweepPairNames()`, which now returns SIX seasons and not the
// four the rule was written against.** That widens the comparison rather than
// changing it: six clusters is df 5 and a critical t of 2.571, a *lower* bar than
// the four-season 3.182 the rule names, so a null on six is a null on four with room
// to spare. The printed critical value is always `tCrit95` of the cluster count
// actually run, never the number in this comment, so the two cannot drift.
// `FPL_SWEEP_SEASONS=default` narrows it to the four the pre-registration named, and
// the verdict should be read on both.
//
// ⚠️ **Gameweek-clustered t is reported and is NOT the decision statistic.** It is
// printed because the prediction benchmark prints both and the two habitually
// disagree by a factor of two or more. The season is the like-for-like unit with a
// replay's own standard error, and four seasons will never do better than three
// degrees of freedom — a permanent limit, not a fixable one. A gameweek cluster
// cannot carry its own split-half floor either, since a club plays one match in it,
// so the gameweek column reuses the pooled floor; one more reason it is context.
//
// ⚠️ **An interior optimum for arm C is a shape, not an argmax.** If C beats both
// endpoints that is evidence the two multipliers carry different information. If C's
// best cell sits at an endpoint it is not, and is not reported as if it were.
//
// # What this changes
//
// Nothing. It is a diagnostic: no shipped number moves, no scoring term is added,
// and arm B existing and winning would not make it shippable.
//
// # On reproducibility
//
// Every reduction here is an accumulation into a per-team-gameweek record, and
// addition commutes, so map order cannot change a figure. The two places order could
// matter both choose rows rather than adding them — the printed tables — and both
// sort first: seasons by name, gameweeks by number, arms by their fixed order in the
// slice that declares them. Players are visited through `boot.Elements`, a slice, and
// through `sortedPlayerIDs`, for the reason that function's comment gives.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// reconCutoff is the gameweek every arm is built through, and reconFrom the first
// gameweek scored. Named rather than inlined because the out-of-sample split IS the
// design — `calibrateExpectedStats` runs on the season to date, so an arm scored on
// the window it was calibrated on would be partly fitted to its own answer.
const (
	reconCutoff = 19
	reconFrom   = 20
)

// reconObs is one club in one gameweek of the held-out window: the fixture-blind
// rate every arm shares, the two multipliers the arms disagree about, and what the
// club went on to do, both per match.
type reconObs struct {
	season string
	gw     int
	club   string
	// rate is the club's fixture-blind expected goals per match, at the cutoff.
	rate float64
	// fdr is arm A's multiplier and def is arm B's, each already averaged over the
	// club's matches in this gameweek so a double is one observation rather than
	// two half-weighted ones.
	fdr, def float64
	// actXG is the target and actG is the contrast, both per match.
	actXG, actG float64
}

// reconMul is the multiplier an arm applies, as a geometric blend of the two
// candidates: weight 0 is arm A exactly, weight 1 is arm B exactly, and the cells in
// between are arm C.
//
// One function so the three arms cannot drift apart — the endpoints are the arms
// themselves rather than separate expressions that happen to agree today, which is
// this project's most-repeated bug written the other way round.
func reconMul(o reconObs, w float64) float64 {
	return math.Pow(o.fdr, 1-w) * math.Pow(o.def, w)
}

// reconPred is what an arm predicts for one club-gameweek.
func reconPred(o reconObs, w float64) float64 { return o.rate * reconMul(o, w) }

// reconRMS is the root-mean-square of log(predicted / realised) for one arm over a
// set of observations, and 0 when there is nothing to score.
func reconRMS(obs []reconObs, w float64, target func(reconObs) float64) float64 {
	var ss, n float64
	for _, o := range obs {
		a := target(o)
		p := reconPred(o, w)
		if a <= 0 || p <= 0 {
			continue
		}
		d := math.Log(p / a)
		ss += d * d
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(ss / n)
}

// reconRMSLevelled is reconRMS with one free level constant removed: the mean log
// residual is subtracted before squaring, which is the best a single multiplicative
// correction fitted on the held-out window itself could do.
//
// It is an oracle and is never a decision statistic. It exists because the arms
// share a level error — the fixture-blind rate's calibration out of sample, which
// `TestDiagTeamGoalShare` measured moving 0.921 to 1.118 across seasons — and a
// level term common to two arms does not cancel from the ratio of their rms values,
// it damps it. Printing both says how much of any gap the shared level is hiding.
func reconRMSLevelled(obs []reconObs, w float64, target func(reconObs) float64) float64 {
	var ds []float64
	for _, o := range obs {
		a := target(o)
		p := reconPred(o, w)
		if a <= 0 || p <= 0 {
			continue
		}
		ds = append(ds, math.Log(p/a))
	}
	if len(ds) == 0 {
		return 0
	}
	m := meanOf(ds)
	var ss float64
	for _, d := range ds {
		ss += (d - m) * (d - m)
	}
	return math.Sqrt(ss / float64(len(ds)))
}

// reconClubFloor is the noise floor measured on the unit the arms are scored on:
// the rms log error of an oracle that knows each club's own geometric mean expected
// goals over the held-out window and nothing else.
//
// It is `splitHalfFloor`'s idea — a club against itself, with no model in it — asked
// at single-match resolution instead of at window resolution and extrapolated. The
// extrapolation is what fails; see the header. Here the residual is taken directly
// against the per-club level, so there is nothing to convert and nothing to
// approximate.
//
// A club is keyed by season as well as name, because a club is a different side in a
// different season and pooling two of them into one level would put genuine
// between-season change into the floor.
//
// Returns 0 when no club clears three matches, which is not a floor of zero but the
// absence of one; a caller printing it will see a floor of 0 and an excess equal to
// the raw figure, which is visible rather than silent.
func reconClubFloor(obs []reconObs) float64 {
	type key struct{ season, club string }
	logs := map[key][]float64{}
	for _, o := range obs {
		if o.actXG <= 0 {
			continue
		}
		k := key{o.season, o.club}
		logs[k] = append(logs[k], math.Log(o.actXG))
	}
	var keys []key
	for k := range logs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].season != keys[j].season {
			return keys[i].season < keys[j].season
		}
		return keys[i].club < keys[j].club
	})
	var ss, n float64
	for _, k := range keys {
		vs := logs[k]
		if len(vs) < 3 {
			continue
		}
		m := meanOf(vs)
		for _, v := range vs {
			ss += (v - m) * (v - m)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(ss / n)
}

// reconExcess is what is left of an rms once the noise floor is removed in
// quadrature. Clamped at zero: an arm scoring below the floor has not beaten physics,
// it has told you the floor estimate is loose, and a negative under a square root
// would be an arithmetic answer to a measurement question.
func reconExcess(rms, floor float64) float64 {
	return math.Sqrt(math.Max(0, rms*rms-floor*floor))
}

// reconGain is how much better b is than a, as a percentage, and 0 when a is 0.
func reconGain(a, b float64) float64 {
	if a <= 0 {
		return 0
	}
	return 100 * (1 - b/a)
}

// reconArmA and reconArmB are the two blend weights the decision rule is written
// about. Named because "0" and "1" at a call site say nothing about which arm is
// which, and the rule is stated on B against A.
const (
	reconArmA = 0.0
	reconArmB = 1.0
)

// reconBlends are arm C's cells. Three interior points, so an interior optimum shows
// as a shape across them rather than as an argmax over a grid dense enough to find
// one by chance.
var reconBlends = []float64{0.25, 0.50, 0.75}

func TestDiagFixtureReconciliation(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	// Coverage first, before any table, and here it is load-bearing twice over.
	// Arm B is built entirely from expected goals conceded, and four of the
	// archive's seasons carry none natively — `applyXGCRepair` reconstructs those
	// rows from the opposition's expected goals. A reconstructed xGC is a
	// legitimate defensive measurement and it is a *different* one: it is what the
	// opposition created, with no information about when in the match it came. So
	// the share of arm B's input that is reconstructed belongs above the result,
	// not in a footnote, and a season whose cutoff rests on almost no rows should
	// be visible before its numbers are read rather than after.
	fmt.Printf("\n=== coverage: the rows behind each fit, through GW%d\n", reconCutoff)
	fmt.Printf("%-10s %10s %10s %12s %10s\n",
		"season", "xG rows", "xGC rows", "xGC rebuilt", "xG repair")

	var all []reconObs
	obsBySeason := map[string][]reconObs{}
	clubsBySeason := map[string][]shareClub{}
	// matchesBySeason is the mean number of held-out matches per club, which is the
	// sqrt(n) that converts splitHalfFloor's full-window sampling sd into the
	// per-match sd this diagnostic scores against.
	matchesBySeason := map[string]float64{}
	var mismatched, dropped int

	for _, pair := range sweepPairNames() {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatal(err)
		}

		var xgRows, xgcRows, xgcRebuilt float64
		for _, id := range sortedPlayerIDs(cur) {
			for gw, g := range cur.Players[id].GWs {
				if gw > reconCutoff {
					continue
				}
				if g.XG > 0 {
					xgRows++
				}
				if g.XGC > 0 {
					xgcRows++
					if g.XGCReconstructed {
						xgcRebuilt++
					}
				}
			}
		}
		var rebuiltPct float64
		if xgcRows > 0 {
			rebuiltPct = 100 * xgcRebuilt / xgcRows
		}
		fmt.Printf("%-10s %10.0f %10.0f %11.0f%% %10v\n",
			cur.Name, xgRows, xgcRows, rebuiltPct, cur.XGRepair.Applied)

		boot, fx := PointInTime(cur, prior, reconCutoff)
		// Horizon 1 because the quantity is "goals in a match". A five-gameweek
		// fixture average would answer a different question, and it is the same
		// reason the prediction benchmark pins horizon 1.
		w := cfg.Weights
		w.Horizon = 1
		e := analysis.NewEngineFull(boot, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = newPriorIndex(prior)
		e.Recent = newRecentIndexWith(cur, reconCutoff, w.MinutesHalfLife, w.RateHalfLife)

		// The cutoff-time model of every club: the attacking rate all arms share,
		// and the minutes-weighted defensive rate arm B reads off the opponent.
		type clubModel struct{ rate, xgcNum, xgcDen float64 }
		models := map[int]*clubModel{}
		registered := map[int]bool{}
		for i := range boot.Elements {
			el := &boot.Elements[i]
			m := e.Metrics(el)
			c := models[el.Team]
			if c == nil {
				c = &clubModel{}
				models[el.Team] = c
			}
			c.rate += m.XG90 * m.XGScale * m.ExpectedMinutes / 90
			// m.XGC90 already carries TeamXGCFactor — Engine.Metrics applies it
			// before returning. Reapplying it here would square a club-level
			// correction, which is the mirrored-quantity bug this project keeps a
			// list of.
			c.xgcNum += m.XGC90 * m.ExpectedMinutes
			c.xgcDen += m.ExpectedMinutes
			registered[el.ID] = true
		}

		var clubIDs []int
		for id := range models {
			clubIDs = append(clubIDs, id)
		}
		sort.Ints(clubIDs)

		defence := map[int]float64{}
		var defSum, defN float64
		for _, id := range clubIDs {
			c := models[id]
			if c.xgcDen <= 0 || c.xgcNum <= 0 {
				continue
			}
			defence[id] = c.xgcNum / c.xgcDen
			defSum += defence[id]
			defN++
		}
		if defN < 2 {
			t.Fatalf("%s: only %.0f clubs carry a defensive estimate at GW%d; arm B "+
				"has nothing to divide by", cur.Name, defN, reconCutoff)
		}
		leagueDef := defSum / defN

		// One record per club per held-out gameweek.
		type key struct{ team, gw int }
		type acc struct {
			// fdrSum, defSum and n are the prediction side, from the point-in-time
			// fixture list.
			fdrSum, defSum, n float64
			// played, goals and xg are the realised side, from the raw season.
			played, goals, xg float64
			// noDefence marks a gameweek where an opponent carried no defensive
			// estimate at the cutoff, which makes arm B undefined rather than 1.
			noDefence bool
		}
		byGW := map[key]*acc{}
		get := func(team, gw int) *acc {
			k := key{team, gw}
			if a := byGW[k]; a != nil {
				return a
			}
			a := &acc{}
			byGW[k] = a
			return a
		}

		// The prediction side reads `fx`, the list `PointInTime` returns, rather
		// than the raw season. Opponent identity and difficulty survive that filter
		// untouched and the scorelines do not, so reading from it is a structural
		// statement that nothing here can see a result — not an argument that it
		// happens not to.
		for _, f := range fx {
			if f.Event == nil || *f.Event < reconFrom || *f.Event > 38 {
				continue
			}
			for _, side := range []struct {
				team, opp, difficulty int
			}{
				{f.TeamH, f.TeamA, f.TeamHDifficulty},
				{f.TeamA, f.TeamH, f.TeamADifficulty},
			} {
				a := get(side.team, *f.Event)
				a.n++
				a.fdrSum += analysis.AttackMultiplier(side.difficulty)
				d, ok := defence[side.opp]
				if !ok {
					a.noDefence = true
					continue
				}
				a.defSum += d / leagueDef
			}
		}

		// Realised goals come from the scorelines, which are exact and include the
		// ones an opponent put into his own net. Realised expected goals have no
		// such source and are summed from the player rows, which is what team
		// expected goals is.
		for _, f := range cur.Fixtures {
			if f.Event == nil || *f.Event < reconFrom || *f.Event > 38 {
				continue
			}
			if f.TeamHScore == nil || f.TeamAScore == nil {
				continue // not played, or the archive did not record it
			}
			h := get(f.TeamH, *f.Event)
			a := get(f.TeamA, *f.Event)
			h.played++
			a.played++
			h.goals += float64(*f.TeamHScore)
			a.goals += float64(*f.TeamAScore)
		}

		// The denominator must cover the same players as the numerator, so the
		// realised expected goals are summed over registered players only — the
		// measured cost of not doing so is in TestDiagTeamGoalShare's comment, and
		// it is the same order as the effect this diagnostic is sized for.
		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			if !registered[id] {
				continue
			}
			for gw, g := range p.GWs {
				if gw < reconFrom || gw > 38 {
					continue
				}
				get(p.Team, gw).xg += g.XG
			}
		}

		// Assemble, in a fixed order, and keep the per-club realised series the
		// noise floor needs.
		var keys []key
		for k := range byGW {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].team != keys[j].team {
				return keys[i].team < keys[j].team
			}
			return keys[i].gw < keys[j].gw
		})

		floorClubs := map[int]*shareClub{}
		var seasonObs []reconObs
		for _, k := range keys {
			a := byGW[k]
			switch {
			case a.n == 0 || a.played == 0:
				// A blank, or a fixture the archive never recorded a result for.
				dropped++
				continue
			case a.n != a.played:
				// The fixture list and the played record disagree on how many
				// matches this club had this gameweek. Dividing by whichever
				// number came to hand is how a per-match figure silently becomes
				// a per-two-match one, so the row goes rather than the choice.
				mismatched++
				continue
			case a.noDefence || a.xg <= 0:
				dropped++
				continue
			}
			c := models[k.team]
			if c == nil || c.rate <= 0 {
				dropped++
				continue
			}
			seasonObs = append(seasonObs, reconObs{
				season: cur.Name,
				gw:     k.gw,
				club:   teamShortName(cur.Teams, k.team),
				rate:   c.rate,
				fdr:    a.fdrSum / a.n,
				def:    a.defSum / a.n,
				actXG:  a.xg / a.played,
				actG:   a.goals / a.played,
			})
			fc := floorClubs[k.team]
			if fc == nil {
				fc = &shareClub{
					name:        teamShortName(cur.Teams, k.team),
					xgByGW:      map[int]float64{},
					matchesByGW: map[int]float64{},
				}
				floorClubs[k.team] = fc
			}
			fc.xgByGW[k.gw] += a.xg
			fc.matchesByGW[k.gw] += a.played
			fc.matches += a.played
		}
		if len(seasonObs) == 0 {
			continue
		}

		var fcIDs []int
		for id := range floorClubs {
			fcIDs = append(fcIDs, id)
		}
		sort.Ints(fcIDs)
		var cs []shareClub
		var matchSum float64
		for _, id := range fcIDs {
			cs = append(cs, *floorClubs[id])
			matchSum += floorClubs[id].matches
		}
		clubsBySeason[cur.Name] = cs
		if len(cs) > 0 {
			matchesBySeason[cur.Name] = matchSum / float64(len(cs))
		}
		obsBySeason[cur.Name] = seasonObs
		all = append(all, seasonObs...)
	}

	if len(obsBySeason) < 2 {
		t.Skipf("only %d season(s) produced observations; there is no between-season "+
			"spread and therefore no decision statistic", len(obsBySeason))
	}

	var seasons []string
	for s := range obsBySeason {
		seasons = append(seasons, s)
	}
	sort.Strings(seasons)

	fmt.Printf("\n%d club-gameweeks over %d seasons. %d dropped (blank, no result, no "+
		"opponent\ndefensive estimate, or no realised expected goals) and %d dropped for a "+
		"fixture\ncount the played record disagrees with.\n",
		len(all), len(seasons), dropped, mismatched)

	// -----------------------------------------------------------------
	// The noise floor, FIRST, before any threshold is stated.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== the noise floor, measured with no model in it\n")
	fmt.Printf("`split-half` is the pre-registered instrument: each club's held-out window\n")
	fmt.Printf("cut into two halves of its own matches, the log ratio of the halves taken\n")
	fmt.Printf("across clubs. It is the sampling sd of a FULL-WINDOW mean, and the arms are\n")
	fmt.Printf("scored on SINGLE matches, so `extrapolated` scales it by sqrt(matches per\n")
	fmt.Printf("club). `club oracle` is the same idea measured directly at single-match\n")
	fmt.Printf("resolution: the rms log error of knowing each club's own window geometric\n")
	fmt.Printf("mean and nothing else. `A raw` is arm A's raw rms, for comparison.\n\n")
	fmt.Printf("%-10s %8s %10s %11s %13s %12s %9s\n",
		"season", "clubs", "matches", "split-half", "extrapolated", "club oracle", "A raw")
	floorBySeason := map[string]float64{}
	extrapBySeason := map[string]float64{}
	xgTarget := func(o reconObs) float64 { return o.actXG }
	goalTarget := func(o reconObs) float64 { return o.actG }
	var overshoot int
	for _, s := range seasons {
		cs := clubsBySeason[s]
		win := splitHalfFloor(cs)
		ext := win * math.Sqrt(matchesBySeason[s])
		f := reconClubFloor(obsBySeason[s])
		floorBySeason[s] = f
		extrapBySeason[s] = ext
		ra := reconRMS(obsBySeason[s], reconArmA, xgTarget)
		if ext >= ra {
			overshoot++
		}
		fmt.Printf("%-10s %8d %10.1f %11.4f %13.4f %12.4f %9.4f\n",
			s, len(cs), matchesBySeason[s], win, ext, f, ra)
	}
	var poolClubs []shareClub
	var poolMatches float64
	for _, s := range seasons {
		poolClubs = append(poolClubs, clubsBySeason[s]...)
		poolMatches += matchesBySeason[s] * float64(len(clubsBySeason[s]))
	}
	poolWindow := splitHalfFloor(poolClubs)
	poolExtrap := poolWindow * math.Sqrt(poolMatches/float64(len(poolClubs)))
	poolFloor := reconClubFloor(all)
	fmt.Printf("%-10s %8d %10.1f %11.4f %13.4f %12.4f %9.4f\n", "POOLED", len(poolClubs),
		poolMatches/float64(len(poolClubs)), poolWindow, poolExtrap, poolFloor,
		reconRMS(all, reconArmA, xgTarget))

	fmt.Printf("\nThe extrapolated column is NOT the floor applied below, and the reason is\n")
	fmt.Printf("in the table: it exceeds arm A's own raw rms in %d of %d seasons. A floor\n",
		overshoot, len(seasons))
	fmt.Printf("larger than the quantity it is a floor for zeroes both arms' excess and\n")
	fmt.Printf("makes their ratio a ratio of zeroes. That is a defect of converting a\n")
	fmt.Printf("full-window sd to a per-match one through sqrt(k) — exact for a mean of\n")
	fmt.Printf("logs, only approximate for the log of a mean, and expected goals per match\n")
	fmt.Printf("is skewed enough for the gap to compound. The club oracle asks the same\n")
	fmt.Printf("question at the resolution the arms are scored at, with nothing to convert.\n")
	fmt.Printf("\nEither floor is the spread of a club's matches around ITS OWN window mean,\n")
	fmt.Printf("so it contains the match-to-match variation the arms are trying to predict\n")
	fmt.Printf("as well as the noise none of them can reach. It is an UPPER bound on the\n")
	fmt.Printf("irreducible floor, and the excess below is a LOWER bound on each arm's\n")
	fmt.Printf("error. Subtracting too much inflates the percentage GAP between two arms,\n")
	fmt.Printf("so the excess column is the generous reading for arm B and the raw column\n")
	fmt.Printf("is the strict one. Both are printed; neither is quoted alone.\n")
	sink.emitAll("fixture_reconciliation", "GW19 fit, GW20-38 scored", "noise floor",
		len(poolClubs),
		measure{"split-half sd of a full-window mean", poolWindow},
		measure{"that extrapolated to per-match by sqrt(k)", poolExtrap},
		measure{"seasons where the extrapolation exceeds arm A's raw rms",
			float64(overshoot)},
		measure{"per-match floor applied, the club-mean oracle", poolFloor})

	// -----------------------------------------------------------------
	// The arms, pooled.
	// -----------------------------------------------------------------
	type arm struct {
		name string
		w    float64
	}
	arms := []arm{{"arm A  shipped FDR ladder", reconArmA}}
	for _, w := range reconBlends {
		arms = append(arms, arm{fmt.Sprintf("arm C  blend, %.2f on defence", w), w})
	}
	arms = append(arms, arm{"arm B  opponent's own defence", reconArmB})

	fmt.Printf("\n=== the arms, against realised expected goals per match\n")
	fmt.Printf("Model built through GW%d, scored on GW%d-38. Lower is better. `excess` is\n",
		reconCutoff, reconFrom)
	fmt.Printf("the raw rms with the per-match floor removed in quadrature; `levelled` is\n")
	fmt.Printf("the raw rms with one free level constant fitted on the held-out window,\n")
	fmt.Printf("which is an ORACLE and is printed to show how much of any gap between the\n")
	fmt.Printf("arms a shared calibration error is damping.\n\n")

	fmt.Printf("%-32s %10s %10s %10s | %10s\n",
		"arm", "rms log", "excess", "levelled", "vs goals")
	// arm 0 first: no fixture term at all. If nothing below beats it, the question
	// of WHICH opponent signal to use does not arise.
	flat := make([]reconObs, len(all))
	copy(flat, all)
	for i := range flat {
		flat[i].fdr, flat[i].def = 1, 1
	}
	flatRMS := reconRMS(flat, reconArmA, xgTarget)
	fmt.Printf("%-32s %10.4f %10.4f %10.4f | %10.4f\n",
		"arm 0  no fixture term at all", flatRMS, reconExcess(flatRMS, poolFloor),
		reconRMSLevelled(flat, reconArmA, xgTarget), reconRMS(flat, reconArmA, goalTarget))
	for _, a := range arms {
		r := reconRMS(all, a.w, xgTarget)
		fmt.Printf("%-32s %10.4f %10.4f %10.4f | %10.4f\n",
			a.name, r, reconExcess(r, poolFloor),
			reconRMSLevelled(all, a.w, xgTarget), reconRMS(all, a.w, goalTarget))
		sink.emitAll("fixture_reconciliation", "GW19 fit, GW20-38 scored", a.name, len(all),
			measure{"rms log error against realised expected goals", r},
			measure{"excess over the per-match noise floor", reconExcess(r, poolFloor)},
			measure{"rms log error against realised goals", reconRMS(all, a.w, goalTarget)})
	}

	rawA := reconRMS(all, reconArmA, xgTarget)
	rawB := reconRMS(all, reconArmB, xgTarget)
	exA := reconExcess(rawA, poolFloor)
	exB := reconExcess(rawB, poolFloor)
	fmt.Printf("\nB against A, pooled: %+.2f%% on the raw rms, %+.2f%% on the excess.\n",
		reconGain(rawA, rawB), reconGain(exA, exB))
	fmt.Printf("Positive means arm B is BETTER. The decision rule reads the excess column,\n")
	fmt.Printf("and pooled is not the decision statistic — the season clustering below is.\n")

	// -----------------------------------------------------------------
	// Season clustering: the decision statistic.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== season-clustered, which IS the decision statistic\n")
	fmt.Printf("Each season's own club-oracle floor, its own arms, its own gain. Four\n")
	fmt.Printf("seasons is three degrees of freedom and no number of club-gameweeks\n")
	fmt.Printf("improves on that.\n\n")
	fmt.Printf("%-10s %8s %9s %9s %9s %9s %10s\n",
		"season", "n", "floor", "A raw", "B raw", "A excess", "B excess")
	var gains, rawGains []float64
	for _, s := range seasons {
		obs := obsBySeason[s]
		f := floorBySeason[s]
		ra := reconRMS(obs, reconArmA, xgTarget)
		rb := reconRMS(obs, reconArmB, xgTarget)
		ea := reconExcess(ra, f)
		eb := reconExcess(rb, f)
		gains = append(gains, reconGain(ea, eb))
		rawGains = append(rawGains, reconGain(ra, rb))
		fmt.Printf("%-10s %8d %9.4f %9.4f %9.4f %9.4f %10.4f\n",
			s, len(obs), f, ra, rb, ea, eb)
	}
	fmt.Printf("\n%-10s %8s %9s\n", "season", "gain %", "raw gain %")
	for i, s := range seasons {
		fmt.Printf("%-10s %8.2f %9.2f\n", s, gains[i], rawGains[i])
	}
	gMean, gSE := meanAndSE(gains)
	rMean, rSE := meanAndSE(rawGains)
	df := len(gains) - 1
	var gT, rT float64
	if gSE > 0 {
		gT = gMean / gSE
	}
	if rSE > 0 {
		rT = rMean / rSE
	}
	fmt.Printf("%-10s %8.2f %9.2f\n", "MEAN", gMean, rMean)
	fmt.Printf("%-10s %8.2f %9.2f   (SE)\n", "", gSE, rSE)
	fmt.Printf("%-10s %8.2f %9.2f   at df %d, critical t %.3f\n",
		"t", gT, rT, df, tCrit95(df))
	sink.emitAll("fixture_reconciliation", "GW19 fit, GW20-38 scored",
		"B against A, season-clustered", len(gains),
		measure{"mean per-season gain on the excess, percent", gMean},
		measure{"its standard error across seasons", gSE},
		measure{"mean per-season gain on the raw rms, percent", rMean},
		measure{"its standard error across seasons", rSE})

	// -----------------------------------------------------------------
	// Gameweek clustering: context only.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== gameweek-clustered, reported for CONTEXT and not a decision\n")
	fmt.Printf("A gameweek cluster cannot carry a floor of its own — a club plays one match\n")
	fmt.Printf("in it and there is no half to split — so the excess column here borrows the\n")
	fmt.Printf("pooled floor, which is the wrong floor for a single week and zeroes the\n")
	fmt.Printf("quiet ones. Both readings are printed with their own cluster counts, and\n")
	fmt.Printf("the dropped weeks are named, because a mean over the weeks that survived a\n")
	fmt.Printf("floor is a mean over the weeks the floor happened to like.\n")
	fmt.Printf("The season is the unit that is like-for-like with a replay's own standard\n")
	fmt.Printf("error; this whole block is here because the prediction benchmark prints\n")
	fmt.Printf("both and the two habitually disagree by a factor of two or more.\n\n")
	byWeek := map[int][]reconObs{}
	for _, o := range all {
		byWeek[o.gw] = append(byWeek[o.gw], o)
	}
	var weeks []int
	for gw := range byWeek {
		weeks = append(weeks, gw)
	}
	sort.Ints(weeks)
	var weekGains, weekRaw []float64
	var weekDropped []int
	for _, gw := range weeks {
		obs := byWeek[gw]
		if len(obs) < 10 {
			continue
		}
		ra := reconRMS(obs, reconArmA, xgTarget)
		rb := reconRMS(obs, reconArmB, xgTarget)
		weekRaw = append(weekRaw, reconGain(ra, rb))
		ea := reconExcess(ra, poolFloor)
		if ea <= 0 {
			weekDropped = append(weekDropped, gw)
			continue
		}
		weekGains = append(weekGains, reconGain(ea, reconExcess(rb, poolFloor)))
	}
	wMean, wSE := meanAndSE(weekGains)
	wrMean, wrSE := meanAndSE(weekRaw)
	var wT, wrT float64
	if wSE > 0 {
		wT = wMean / wSE
	}
	if wrSE > 0 {
		wrT = wrMean / wrSE
	}
	fmt.Printf("%-12s %8s %9s %8s %8s\n", "reading", "clusters", "mean %", "SE", "t")
	fmt.Printf("%-12s %8d %9.2f %8.2f %8.2f\n", "raw rms", len(weekRaw), wrMean, wrSE, wrT)
	fmt.Printf("%-12s %8d %9.2f %8.2f %8.2f\n", "excess", len(weekGains), wMean, wSE, wT)
	if len(weekDropped) > 0 {
		fmt.Printf("\n%d week(s) dropped from the excess reading for a zero excess: %v\n",
			len(weekDropped), weekDropped)
	}
	sink.emitAll("fixture_reconciliation", "GW19 fit, GW20-38 scored",
		"B against A, gameweek-clustered", len(weekRaw),
		measure{"mean per-gameweek gain on the raw rms, percent", wrMean},
		measure{"its standard error across gameweeks", wrSE},
		measure{"mean per-gameweek gain on the excess, percent", wMean},
		measure{"gameweeks dropped from the excess for a zero excess",
			float64(len(weekDropped))})

	// -----------------------------------------------------------------
	// Arm C: is there a shape, or is the best cell at an endpoint?
	// -----------------------------------------------------------------
	fmt.Printf("\n=== arm C: do the two multipliers carry different information?\n")
	fmt.Printf("An interior optimum is a SHAPE and an endpoint is not. C beating both ends\n")
	fmt.Printf("is evidence the ladder and the defensive estimate know different things; C\n")
	fmt.Printf("peaking at 0.00 or 1.00 is just arm A or arm B again.\n\n")
	fmt.Printf("%-14s %10s %10s\n", "weight", "rms log", "excess")
	bestW, bestEx := math.Inf(1), math.Inf(1)
	for _, w := range append(append([]float64{reconArmA}, reconBlends...), reconArmB) {
		r := reconRMS(all, w, xgTarget)
		ex := reconExcess(r, poolFloor)
		fmt.Printf("%-14.2f %10.4f %10.4f\n", w, r, ex)
		if ex < bestEx {
			bestEx, bestW = ex, w
		}
	}
	interior := bestW > reconArmA && bestW < reconArmB
	fmt.Printf("\nbest cell: weight %.2f, which is %s.\n", bestW,
		map[bool]string{true: "INTERIOR — a shape",
			false: "an ENDPOINT — no shape, and not reported as one"}[interior])
	fmt.Printf("\n⚠️ This column is POOLED and carries no clustering, so it is descriptive\n")
	fmt.Printf("and not a resolution. An interior cell says the two multipliers are not the\n")
	fmt.Printf("same signal; it does not say the difference survives the season spread, and\n")
	fmt.Printf("the season table above is where that is answered. Read the SPACING too: a\n")
	fmt.Printf("curve whose cells sit inside the width of the B-against-A gain is a shape in\n")
	fmt.Printf("the same sense a flat line is.\n")
	sink.emitAll("fixture_reconciliation", "GW19 fit, GW20-38 scored", "arm C", len(all),
		measure{"best blend weight on the opponent's defence", bestW},
		measure{"its excess over the floor", bestEx})

	// -----------------------------------------------------------------
	// The rule, applied.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== the decision rule, fixed before this was run\n")
	fmt.Printf("B beats A by more than 5%% on the excess AND clears the season-clustered\n")
	fmt.Printf("critical t: the reconciliation is real and escalates to a replay, which\n")
	fmt.Printf("this diagnostic does NOT authorise. B beats A and does not clear: recorded\n")
	fmt.Printf("as unresolved and NOT built. B does not beat A: the hypothesis dies and\n")
	fmt.Printf("the fixture-difficulty null does not trace to this substitution.\n\n")
	fmt.Printf("  gain on the excess      %+.2f%% against the 5%% bar\n", gMean)
	fmt.Printf("  season-clustered t      %+.2f against %.3f at df %d\n",
		gT, tCrit95(df), df)
	fmt.Printf("  the same on the RAW rms %+.2f%%, t %+.2f\n", rMean, rT)
	switch {
	case gMean > 5 && gT > tCrit95(df):
		fmt.Printf("\n  => B WINS on both. Escalate; do not ship.\n")
	case gMean > 0:
		fmt.Printf("\n  => B is ahead but does not clear both bars. UNRESOLVED, not built.\n")
	default:
		fmt.Printf("\n  => B does NOT beat A. The hypothesis dies.\n")
	}
	fmt.Printf("\nThe rule is stated on the excess. The raw line is printed beside it\n")
	fmt.Printf("because a verdict that flips between the two would mean the floor is doing\n")
	fmt.Printf("the deciding, and a floor is not a result. Read whether they agree.\n")
	fmt.Printf("\nTwo asymmetries both favour A and are not corrected: arm A carries a venue\n")
	fmt.Printf("term through FPL's home and away difficulties and arm B has none, and A's\n")
	fmt.Printf("difficulties are the archive's end-of-season values, which FPL revises\n")
	fmt.Printf("mid-season on outcomes. So a null here is the robust direction and a win\n")
	fmt.Printf("would be understated.\n")
}

// TestReconcileArmsAreWiredToTheirEndpoints pins the one thing about this
// diagnostic that could be wrong while every printed number stayed plausible.
//
// The three arms are one function of a blend weight, which is deliberate — two
// expressions that happen to agree today are this project's most-repeated bug, and
// `Simulate` wiring two of three engines is the canonical instance. The cost of
// collapsing them is that a sign slip or a swapped exponent inside `reconMul` would
// silently relabel arm A as arm B, and the table would still print.
//
// So: weight 0 must be the FDR multiplier alone, weight 1 the opponent's defensive
// multiplier alone, and every interior cell must sit between the two. Runs without
// DIAG and without the archive, because a guard that only runs on the slow path is
// not a guard.
func TestReconcileArmsAreWiredToTheirEndpoints(t *testing.T) {
	o := reconObs{rate: 1.4, fdr: 1.15, def: 0.80}

	if got := reconMul(o, reconArmA); math.Abs(got-o.fdr) > 1e-12 {
		t.Fatalf("arm A multiplier = %v, want the FDR ladder's %v", got, o.fdr)
	}
	if got := reconMul(o, reconArmB); math.Abs(got-o.def) > 1e-12 {
		t.Fatalf("arm B multiplier = %v, want the opponent's defence %v", got, o.def)
	}
	if got, want := reconPred(o, reconArmA), o.rate*o.fdr; math.Abs(got-want) > 1e-12 {
		t.Fatalf("arm A prediction = %v, want %v — the shared attacking rate is not "+
			"reaching the arm", got, want)
	}

	// Every blend cell strictly between the endpoints, and moving monotonically from
	// one to the other, so a winning cell can be read as a shape.
	lo, hi := math.Min(o.fdr, o.def), math.Max(o.fdr, o.def)
	prev := reconMul(o, reconArmA)
	for _, w := range reconBlends {
		got := reconMul(o, w)
		if got <= lo || got >= hi {
			t.Fatalf("blend %.2f gives %v, outside the endpoints (%v, %v)", w, got, lo, hi)
		}
		if got >= prev {
			t.Fatalf("blend %.2f gives %v, which is not moving toward the defensive "+
				"endpoint from %v — the weight is applied to the wrong term", w, got, prev)
		}
		prev = got
	}

	// The floor is removed in quadrature and clamped, not subtracted: an arm scoring
	// below the floor says the floor estimate is loose, and a negative under the
	// square root would be an arithmetic answer to a measurement question.
	if got := reconExcess(0.4, 0.5); got != 0 {
		t.Fatalf("excess of an rms below the floor = %v, want 0", got)
	}
	if got, want := reconExcess(0.5, 0.3), 0.4; math.Abs(got-want) > 1e-12 {
		t.Fatalf("excess = %v, want %v", got, want)
	}

	// Positive means the second argument is better. The decision rule is written on
	// that convention and a flipped sign would read a loss as a win.
	if got := reconGain(0.50, 0.45); got <= 0 {
		t.Fatalf("gain from 0.50 to 0.45 = %v, want positive — a lower rms is better", got)
	}
	if got := reconGain(0.45, 0.50); got >= 0 {
		t.Fatalf("gain from 0.45 to 0.50 = %v, want negative", got)
	}
}
