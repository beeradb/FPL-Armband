package backtest

// Does the live engine emit two numbers about the same fixture that contradict
// each other?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagAttackDefenceCoherence -v -timeout 60m
//
// # What this gates
//
// The owner's framing: we cannot simultaneously say a team will score 2 goals
// and its opponent is likely to keep a clean sheet. That is a CORRECTNESS
// question, not a points question — it needs no significance test, no replay,
// and it is closer to a bug hunt than to an experiment. The output is a
// descriptive fact about the engine: how often, and by how much, do its own
// attack and defence estimates disagree about one match.
//
// # ⚠️ How this differs from TestDiagFixtureReconciliation
//
// The sibling diagnostic in this package (reconcile_diag_test.go) compares two
// FIXTURE-BLIND quantities: a club's attacking rate against an AVERAGE
// opponent, and a club's defensive rate against an AVERAGE opponent. Those two
// are SUPPOSED to differ — a strong attack meeting a strong defence should
// produce different numbers, and driving that gap to zero would drive out the
// signal a joint model exists to combine. That comparison would manufacture a
// finding here.
//
// This diagnostic instead pairs BOTH sides' OWN opponent adjustment for THIS
// specific fixture, so both are estimates of the same quantity — the goals the
// attacking side scores in this match. But a second reading below (the
// "reference arm") shows that even this pairing does not reduce disagreement
// to a pure defect signal, because the two sides are still built from
// different halves of the model with different scale corrections. See
// "Why disagreement is not automatically a defect" below.
//
//   - **Attack side**, team A playing team B: sum over A's registered players
//     of `XG90 x XGScale x ExpectedMinutes / 90` (A's fixture-blind attacking
//     rate, identical to the expression TestDiagTeamGoalShare and
//     TestDiagFixtureReconciliation both use — every factor is already a
//     reported multiplier on PlayerMetrics), scaled by the attacking
//     multiplier `Engine.FixtureMultipliersFor` returns for A's OWN fixture
//     entry (`Difficulty`: A's FPL difficulty in this match, `OpponentID`: B).
//     That reads the engine's own calibrated path — the same function
//     `fixtureAdjustedXP90` reads through `fixtureMultipliersFor` — so it
//     carries the band adjustment (`Weights.BandStrength`) and the
//     magnitude-difficulty switch exactly as the scored path does.
//   - **Defence side**, team B being attacked by team A: `sweep.go`'s
//     `cleanSheetProb(xgc90, def, cf) = cleanSheetScale x
//     exp(-cleanSheetXGCFactor x xgc90 x def x cf)` is read here as a Poisson
//     zero — P(clean sheet) = P(0 goals conceded) = exp(-lambda) — so
//     inverting it gives the engine's own implied expected goals conceded:
//
//	lambda = -ln(cleanSheetProb) = cleanSheetXGCFactor x xgc90 x def x cf - ln(cleanSheetScale)
//
//     `def` is B's OWN defensive multiplier for this fixture, read the
//     identical way the attack side is: the SAME `FixtureMultipliersFor` call
//     on B's own fixture entry (`Difficulty`: B's FPL difficulty in this
//     match, `OpponentID`: A), reading its second return value — both numbers
//     come off B's single per-match difficulty rating through two different
//     ladders (`attackMultiplier` vs `defenceMultiplier`, exported as
//     `AttackMultiplier` and `DefenceMultiplier`) and two different
//     band-adjustment functions.
//
// # Defence-side aggregation, precisely, including `cf`
//
// `xgc90` is aggregated to a club as a minutes-weighted MEAN across B's
// registered players, never a sum — `TestDiagFixtureReconciliation`'s header
// gives the full argument: xGC90 is a per-90 rate while a player is on the
// pitch, and all eleven players concede the same goals simultaneously, so
// summing would read roughly eleven times the true figure.
//
// `cf` — `defconCleanFactor` in teamstrength.go — is folded into the SAME
// minutes-weighted mean rather than treated separately:
//
//	teamXGCxCF = sum(XGC90_i x cf_i x ExpectedMinutes_i) / sum(ExpectedMinutes_i)
//
// `cf` is 1 for a goalkeeper, a midfielder, a forward, and a defender with no
// defensive-contribution rate — `defconCleanFactor` returns 1 unless
// `elementType == 2` (a defender) — and otherwise
// `clamp((dc90/defconReference)^DefConCleanCoupling, 0.75, 1.35)`, a coupling
// that ships non-zero (`DefConCleanCoupling` = 0.3), so it is live in the
// scored path and dropping it would silently omit a factor the engine actually
// applies to every defender's clean-sheet term. Folding `XGC90_i x cf_i`
// together before the minutes weighting is the direct minutes-weighted
// extension of the per-player exponent argument the formula above already
// names — it is not a separate invented step.
//
// ⚠️ **What this means the resulting figure is NOT**: it is not "the club's
// implied goals conceded" in the sense of one number every player's clean
// sheet term reads from. Each defender's OWN `cf` differs by his own
// defensive-contribution rate, so the true per-player exponent argument varies
// PLAYER BY PLAYER even within one club in one match. `defRate` here is a
// club-level average of that per-player quantity, weighted the same way the
// attacking side's club-level rate already is, and it is the best one-number
// summary this diagnostic can build without scoring eleven separate
// implied-concession figures per match — but it is an aggregate, not a
// quantity the scored path itself ever materialises as a single number.
//
// # ⚠️ The with/without-calibration split is a PROVENANCE GUARD, not two
// findings, at the values this runs at
//
// `cleanSheetXGCFactor` defaults to 1.0 (`sweep.go`, `envDefaultAbove`) and
// `cleanSheetScale` defaults to 1.0 (`sweep.go`, `envScaleStrict`). Both are
// printed with their actual run values below. **At 1.0 and 1.0 the
// with-calibration and structural readings are numerically identical** —
// there is no fitted correction in this run to separate from the raw
// estimators. The split exists so a run under `FPL_CS_XGC_FACTOR` or
// `FPL_CS_SCALE` (a sweep) prints two genuinely different numbers instead of
// silently reusing whichever one happened to run first; it is not evidence of
// two distinct findings about the shipped configuration, and the output below
// says so explicitly rather than leaving a reader to notice the two tables
// match.
//
// # Live scale asymmetries this diagnostic does NOT remove
//
// The calibration split above answers a narrower question than "are the two
// sides on the same scale." Three asymmetries are structural, live in the
// shipped configuration, and untouched by forcing `cleanSheetXGCFactor` to 1:
//
//   - **`XGScale`** (`CalibrationRatio(actual, expected)` in metrics.go,
//     fitted per position each run) puts the attack side's rate in REALISED-GOAL
//     units, while the defence side's `XGC90` stays in EXPECTED-goals-conceded
//     units. The two conversion ratios need not match, they are fitted
//     per-position so they differ by squad composition, and the fitted values
//     this run actually used are printed below rather than assumed.
//   - **`TeamXGCFactor`** (`Engine.TeamXGCFactor`, a `map[int]float64`) is a
//     manual per-club correction multiplied into `m.XGC90` (metrics.go, where
//     `Engine.Metrics` assembles it) with no attack-side twin — a one-sided
//     lever. It is populated only by `cmd/armband/main.go`'s
//     `applyTeamOverrides`, which this diagnostic's bare `NewEngineFull` never
//     calls, so it is `nil` and inert HERE. It is live in production whenever
//     a team override sets `xgc_factor` (the shipped `config.json` currently
//     carries one, at 1.15), which this diagnostic does not reproduce — a gap
//     between what this measures and what a live run would show, stated
//     rather than hidden.
//   - **The two ladders have different gains, and are keyed on two DIFFERENT
//     difficulty integers.** `AttackMultiplier`: 1.30 / 1.15 / 1.00 / 0.85 /
//     0.72 (difficulty 1..5), span 1.81x. `DefenceMultiplier`: 0.70 / 0.85 /
//     1.00 / 1.20 / 1.40, span 2.00x and asymmetric at the middle rungs (0.85
//     vs 1.20 either side of 1.00, against the attack ladder's 1.15 vs 0.85).
//     A's own difficulty and B's own difficulty in the same fixture are two
//     separate FPL integers, not mirror images of one rating, so this gain
//     mismatch enters even a perfectly-calibrated pair of estimators.
//
// None of these three is something a diagnostic can net out after the fact
// without inventing a second scale to argue about — which is exactly the trap
// TestDiagTeamGoalShare's header warns against elsewhere in this package. So
// they are reported as CONTEXT for the divergence below, not subtracted from
// it, and the run's actual fitted values are printed rather than assumed from
// a prior season.
//
// # Why disagreement is not automatically a defect: the reference arm
//
// Because of the asymmetries above, two estimators built from different
// halves of the model should NOT be expected to agree exactly even where the
// engine is behaving exactly as designed. So the fixture-adjusted divergence
// alone is not interpretable — it needs a baseline. The reference arm forces
// BOTH fixture multipliers to 1 (the fixture-blind state a club-level
// comparison would be in, ignoring opponent identity entirely) and measures
// the SAME divergence statistic on the SAME club pairs. If the fixture-adjusted
// reading is smaller than this baseline, the opponent-specific adjustment is
// doing reconciling work. If it is not smaller — or is larger — the fixture
// terms are not bringing the two sides together, and that is the honest
// headline rather than "the engine is incoherent by X%": the ladders'
// mismatched gains (above) can by themselves make an already-adjusted pair
// diverge MORE than the unadjusted pair did, and the reference arm is what
// tells the two apart.
//
// # ⚠️ Conditioning the "worst offenders" on fixture difficulty
//
// Because the two ladders' gains differ and are keyed on different
// difficulties, an observation's raw divergence is PARTLY a deterministic
// function of which (attacker's difficulty, defender's difficulty) cell it
// falls in — an extreme cell (a 1 meeting a 5) inflates divergence by
// construction, independent of anything either team actually did. A
// worst-offenders list ranked on raw divergence alone is therefore partly
// selected on fixture-difficulty extremity, which would look like
// incoherence while being deliberate — if under-examined, accidental — ladder
// asymmetry. So every observation's divergence is also reported as a
// RESIDUAL: the raw divergence minus the mean divergence of every other
// observation that shares its exact (attacker difficulty, defender
// difficulty) cell. The worst-offenders table is printed on the raw ranking
// (what a reader would actually notice), but each row carries its residual
// too, and the table states explicitly how many of the raw top 15 remain in
// the residual top 15 — whether the extremity survives conditioning on
// fixture difficulty or is explained by it.
//
// # Point-in-time discipline
//
// Fit at GW19, scored on GW20-38, matching every other diagnostic in this
// package. `sweepPairNames()`, horizon 1 (the quantity is "goals in THIS
// match", not a five-gameweek average — the same reason the prediction
// benchmark and both sibling diagnostics pin horizon 1), and only players
// registered at the cutoff enter either team's rate (`PointInTime`'s own
// filter, read via `boot.Elements`). The attacking multiplier and the
// defensive multiplier are both read from `fx`, the fixture list
// `PointInTime` strips of future scorelines and `Finished` flags — never from
// `cur.Fixtures` — so nothing here can see a result it should not. Realised
// expected goals and goals, used only for the secondary reading, come from
// `cur.Fixtures` (scorelines) and `cur.Players` (expected goals), restricted
// to players registered at the cutoff for the reason `TestDiagTeamGoalShare`
// measured: an unregistered player's output in the denominator alone makes
// the club that bought him read as under-rated.
//
// A team-gameweek where the point-in-time fixture count and the archive's
// played-match count disagree is dropped from the realised comparison and
// counted, rather than divided by whichever number came to hand — the same
// discipline `TestDiagFixtureReconciliation` uses.
//
// # ⚠️ Position on double gameweeks, stated explicitly
//
// This package's standing rule: "the doubles guard must key on (element,
// fixture), never (element, gameweek)" — `season.go` ACCUMULATES rows into a
// gameweek total, it never assigns, because a double is two archive rows
// covering two matches. This diagnostic follows that discipline for every
// realised quantity: `xg`, `goals` and `played` are all accumulated with `+=`
// across every fixture in a team-gameweek, never assigned. Where this
// diagnostic differs from `TestDiagFixtureReconciliation`'s handling: the
// PRIMARY coherence comparison here is built PER FIXTURE, not per
// team-gameweek, because pairing an attacker against "the opponent's defence"
// requires knowing WHICH opponent, and a double gameweek can hold two
// different ones. So the divergence statistics, the systematic/symmetric
// reading and the worst-offenders table are all one row per fixture per
// direction, with no team-gameweek averaging in them at all. Only the
// SECONDARY "closer to realised goals" reading needs a realised target, and
// the archive has no fixture-level split of a double's output — only a
// team-gameweek total — so both of a double's fixtures are compared against
// the SAME per-team-gameweek realised average there. That affects a small,
// counted population (`doubles` below) and only the secondary reading.
//
// # Metric
//
// `log(attackPred / impliedLambda)`, so a factor of 2 high and a factor of 2
// low count alike. Positive means the attack side predicts more goals than
// the defence side implies conceded; negative the reverse.
//
// # What this changes
//
// Nothing. No shipped number moves, no scoring term changes, and no decision
// rule is pre-registered — there is no threshold at which self-contradiction
// becomes acceptable, so a gate here would be theatre. The output is a fact
// about the engine; what to do about it, if anything, is a separate decision
// this diagnostic does not make. **Correcting a measured bias has lost this
// project points five times (AGENTS.md, Standing rules)** — a real divergence
// here would not by itself license a fix.
//
// # On reproducibility
//
// Every reduction is an accumulation into a per-club or per-team-gameweek
// record, and addition commutes, so map order cannot change a figure. The one
// place order could matter is the printed "worst offenders" table, which
// CHOOSES rows, so observations are appended in a fixed order (seasons sorted
// by name, `fx` visited in slice order) and sorted by divergence before
// printing — the clean-sheet diagnostic's recorded failure, avoided rather
// than repeated.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// cohCutoff is the gameweek every fixture's model quantities are built
// through, and cohFrom the first gameweek scored — matching both sibling
// diagnostics' out-of-sample split, for the same reason: `calibrateExpectedStats`
// runs on the season to date, so a fixture inside the fitting window is partly
// fitted to its own answer.
const (
	cohCutoff = 19
	cohFrom   = 20
)

// cohObs is one directed fixture observation: a club attacking, and the
// specific opponent's defence it is attacking into, both adjusted for THIS
// match and nothing else.
type cohObs struct {
	season             string
	gw                 int
	attacker, defender string
	// rate is the attacker's fixture-blind expected goals per match, at the
	// cutoff. atkMul is the attacker's own attacking multiplier for this
	// fixture, from Engine.FixtureMultipliersFor on the attacker's own
	// difficulty entry. atkDiff is that entry's raw FPL difficulty integer,
	// kept for the difficulty-pair conditioning below.
	rate, atkMul float64
	atkDiff      int
	// defRate is the defender's fixture-blind implied expected goals conceded
	// per match, at the cutoff, BEFORE cleanSheetXGCFactor, cleanSheetScale
	// and the fixture multiplier: the minutes-weighted mean of XGC90 x cf
	// across the defender's registered players (see header). defMul is the
	// defender's own defensive multiplier for this fixture, from the SAME
	// FixtureMultipliersFor call that supplies the opposing side's atkMul
	// when it is the defender's own turn to attack. defDiff is its raw FPL
	// difficulty integer.
	defRate, defMul float64
	defDiff         int
	// actXG and actG are the ATTACKER's realised expected goals and goals per
	// match over the team-gameweek this fixture falls in — the shared target
	// both sides are trying to predict, since one team's goals scored is the
	// other's goals conceded. See the header's note on doubles: a double
	// gameweek's two fixtures share one realised figure here.
	actXG, actG float64
}

// blind is o with both fixture multipliers forced to 1 — the reference arm.
// See the header's "Why disagreement is not automatically a defect".
func (o cohObs) blind() cohObs {
	b := o
	b.atkMul, b.defMul = 1, 1
	return b
}

// cohAttackPred is what the attack side predicts for this match.
func cohAttackPred(o cohObs) float64 { return o.rate * o.atkMul }

// cohImpliedLambda is the defence side's implied Poisson mean for goals
// conceded in this match: cleanSheetProb read as P(0 goals) = exp(-lambda),
// inverted. False when the result would be non-positive.
func cohImpliedLambda(o cohObs, factor, scale float64) (float64, bool) {
	lambda := factor*o.defRate*o.defMul - math.Log(scale)
	if lambda <= 0 {
		return 0, false
	}
	return lambda, true
}

// cohDivergence is log(attack / implied-lambda) for one observation at one
// (factor, scale) reading, and false when either side is undefined.
func cohDivergence(o cohObs, factor, scale float64) (float64, bool) {
	a := cohAttackPred(o)
	b, ok := cohImpliedLambda(o, factor, scale)
	if a <= 0 || !ok {
		return 0, false
	}
	return math.Log(a / b), true
}

func TestDiagAttackDefenceCoherence(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	factor, scale := analysis.CleanSheetState()
	fmt.Printf("\n=== the calibration constants this diagnostic is built around\n")
	fmt.Printf("cleanSheetXGCFactor = %.4f   (scales the exponent in lambda = factor x xgc x\n", factor)
	fmt.Printf("                            def x cf - ln(scale); 1.0 is no correction)\n")
	fmt.Printf("cleanSheetScale     = %.4f   (the -ln(scale) shift on the SAME exponent; 1.0\n", scale)
	fmt.Printf("                            is no shift, since ln(1) = 0)\n")
	fmt.Printf("BandStrength        = %.4f   (band adjustment inside FixtureMultipliersFor;\n", cfg.Weights.BandStrength)
	fmt.Printf("                            0 means the ladders below run unmodified)\n")
	if factor == 1 && scale == 1 {
		fmt.Printf("\n⚠️ Both constants are at their shipped defaults. The with-calibration and\n")
		fmt.Printf("structural readings below are therefore NUMERICALLY IDENTICAL in this run —\n")
		fmt.Printf("there is no fitted correction to separate from the raw estimators. This split\n")
		fmt.Printf("is a provenance guard for a run under FPL_CS_XGC_FACTOR / FPL_CS_SCALE (a\n")
		fmt.Printf("sweep), not two separate findings about the shipped configuration.\n")
	} else {
		fmt.Printf("\n⚠️ At least one constant is off its default in this run, so the two readings\n")
		fmt.Printf("below are expected to differ — that difference IS the calibration correction,\n")
		fmt.Printf("not incoherence.\n")
	}

	fmt.Printf("\n=== the two ladders' shipped gains, read from the model — not restated\n")
	fmt.Printf("%-11s %6s %6s %6s %6s %6s\n", "", "d=1", "d=2", "d=3", "d=4", "d=5")
	fmt.Printf("%-11s", "attacking")
	for d := 1; d <= 5; d++ {
		fmt.Printf(" %6.3f", analysis.AttackMultiplier(d))
	}
	fmt.Printf("\n%-11s", "defensive")
	for d := 1; d <= 5; d++ {
		fmt.Printf(" %6.3f", analysis.DefenceMultiplier(d))
	}
	fmt.Printf("\nspans: attacking %.2fx (0.72..1.30), defensive %.2fx (0.70..1.40) — different\n",
		analysis.AttackMultiplier(1)/analysis.AttackMultiplier(5),
		analysis.DefenceMultiplier(5)/analysis.DefenceMultiplier(1))
	fmt.Printf("gains, and each keyed on a DIFFERENT team's own difficulty integer in the same\n")
	fmt.Printf("fixture — a live, structural asymmetry the calibration split above does not\n")
	fmt.Printf("touch. See the difficulty-pair conditioning below for its effect on the ranked\n")
	fmt.Printf("worst-offenders table.\n")

	// Coverage first, before any table — the same discipline the sibling
	// diagnostics use, and load-bearing for the same reason: the defence side
	// is built entirely from expected goals conceded, and four of the
	// archive's seasons carry none natively.
	fmt.Printf("\n=== coverage: the rows behind each fit, through GW%d\n", cohCutoff)
	fmt.Printf("%-10s %10s %10s %12s\n", "season", "xG rows", "xGC rows", "xGC rebuilt")

	var all []cohObs
	obsBySeason := map[string][]cohObs{}
	var totalDropped, totalMismatched, totalDoubles int
	// xgScaleSum/N: pooled mean XGScale by position, printed as context for
	// the "live scale asymmetries" section — computed from the run's own
	// fitted values, never assumed.
	xgScaleSum := map[int]float64{}
	xgScaleN := map[int]float64{}
	posName := map[int]string{1: "GKP", 2: "DEF", 3: "MID", 4: "FWD"}

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
				if gw > cohCutoff {
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
		fmt.Printf("%-10s %10.0f %10.0f %11.0f%%\n", cur.Name, xgRows, xgcRows, rebuiltPct)

		boot, fx := PointInTime(cur, prior, cohCutoff)
		w := cfg.Weights
		w.Horizon = 1
		e := analysis.NewEngineFull(boot, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = newPriorIndex(prior)
		e.Recent = newRecentIndexWith(cur, cohCutoff, w.MinutesHalfLife, w.RateHalfLife)

		// The cutoff-time model of every club: the fixture-blind attacking
		// rate and the fixture-blind implied-concession baseline, both from
		// registered players only.
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
			cf := e.DefconCleanFactorFor(el.ElementType, m.DefCon90)
			c.xgcNum += m.XGC90 * cf * m.ExpectedMinutes
			c.xgcDen += m.ExpectedMinutes
			registered[el.ID] = true
			xgScaleSum[el.ElementType] += m.XGScale
			xgScaleN[el.ElementType]++
		}
		rate := map[int]float64{}
		defRate := map[int]float64{}
		for id, c := range models {
			if c.rate > 0 {
				rate[id] = c.rate
			}
			if c.xgcDen > 0 && c.xgcNum > 0 {
				defRate[id] = c.xgcNum / c.xgcDen
			}
		}

		// Per (team, gameweek): the fixture count PointInTime's list carries,
		// the archive's played-match count, realised goals and realised
		// expected goals. Mirrors TestDiagFixtureReconciliation's acc.
		type key struct{ team, gw int }
		type acc struct{ nFx, played, goals, xg float64 }
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

		for _, f := range fx {
			if f.Event == nil || *f.Event < cohFrom || *f.Event > 38 {
				continue
			}
			get(f.TeamH, *f.Event).nFx++
			get(f.TeamA, *f.Event).nFx++
		}
		for _, f := range cur.Fixtures {
			if f.Event == nil || *f.Event < cohFrom || *f.Event > 38 {
				continue
			}
			if f.TeamHScore == nil || f.TeamAScore == nil {
				continue // not played, or the archive did not record it
			}
			h, a := get(f.TeamH, *f.Event), get(f.TeamA, *f.Event)
			h.played++
			a.played++
			h.goals += float64(*f.TeamHScore)
			a.goals += float64(*f.TeamAScore)
		}
		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			if !registered[id] {
				continue
			}
			for gw, g := range p.GWs {
				if gw < cohFrom || gw > 38 {
					continue
				}
				get(p.Team, gw).xg += g.XG
			}
		}

		realXG := map[key]float64{}
		realG := map[key]float64{}
		var dropped, mismatched, doubles int
		for k, a := range byGW {
			switch {
			case a.nFx == 0 || a.played == 0:
				dropped++
			case a.nFx != a.played:
				mismatched++
			case a.xg <= 0:
				dropped++
			default:
				realXG[k] = a.xg / a.played
				realG[k] = a.goals / a.played
				if a.nFx >= 2 {
					doubles++
				}
			}
		}
		totalDropped += dropped
		totalMismatched += mismatched
		totalDoubles += doubles

		// Now the fixtures themselves, read from fx (point-in-time) so
		// nothing past the cutoff leaks in through the difficulty ratings.
		var seasonObs []cohObs
		for _, f := range fx {
			if f.Event == nil || *f.Event < cohFrom || *f.Event > 38 {
				continue
			}
			gw := *f.Event
			atkH, defH := e.FixtureMultipliersFor(analysis.FixtureBrief{
				Event: gw, OpponentID: f.TeamA, Difficulty: f.TeamHDifficulty,
			})
			atkA, defA := e.FixtureMultipliersFor(analysis.FixtureBrief{
				Event: gw, OpponentID: f.TeamH, Difficulty: f.TeamADifficulty,
			})

			// Direction 1: home attacks, away defends.
			if rH, ok := rate[f.TeamH]; ok {
				if dA, ok2 := defRate[f.TeamA]; ok2 {
					if xg, ok3 := realXG[key{f.TeamH, gw}]; ok3 {
						seasonObs = append(seasonObs, cohObs{
							season: cur.Name, gw: gw,
							attacker: teamShortName(cur.Teams, f.TeamH),
							defender: teamShortName(cur.Teams, f.TeamA),
							rate:     rH, atkMul: atkH, atkDiff: f.TeamHDifficulty,
							defRate: dA, defMul: defA, defDiff: f.TeamADifficulty,
							actXG: xg, actG: realG[key{f.TeamH, gw}],
						})
					}
				}
			}
			// Direction 2: away attacks, home defends.
			if rA, ok := rate[f.TeamA]; ok {
				if dH, ok2 := defRate[f.TeamH]; ok2 {
					if xg, ok3 := realXG[key{f.TeamA, gw}]; ok3 {
						seasonObs = append(seasonObs, cohObs{
							season: cur.Name, gw: gw,
							attacker: teamShortName(cur.Teams, f.TeamA),
							defender: teamShortName(cur.Teams, f.TeamH),
							rate:     rA, atkMul: atkA, atkDiff: f.TeamADifficulty,
							defRate: dH, defMul: defH, defDiff: f.TeamHDifficulty,
							actXG: xg, actG: realG[key{f.TeamA, gw}],
						})
					}
				}
			}
		}
		if len(seasonObs) == 0 {
			continue
		}
		obsBySeason[cur.Name] = seasonObs
		all = append(all, seasonObs...)
	}

	if len(obsBySeason) < 2 {
		t.Skipf("only %d season(s) produced observations; there is no between-season "+
			"spread to report", len(obsBySeason))
	}

	fmt.Printf("\n%d directed fixture observations over %d seasons (%d dropped for a blank "+
		"or no\nrealised expected goals, %d dropped for a fixture count the played record "+
		"disagrees\nwith, %d team-gameweeks were doubles sharing one realised figure across "+
		"two\nfixtures — see the header's position on doubles). Drops affect only the "+
		"realised-goals\nreading, never the coherence one.\n",
		len(all), len(obsBySeason), totalDropped, totalMismatched, totalDoubles)

	fmt.Printf("\n=== live scale asymmetry: XGScale by position, this run's fitted values\n")
	fmt.Printf("The attack side is these units; the defence side is raw XGC90 units. Pooled\n")
	fmt.Printf("across seasons for a single illustrative figure — not a claim requiring\n")
	fmt.Printf("season-level inference, just the run's own numbers instead of assumed ones.\n")
	for _, pos := range []int{1, 2, 3, 4} {
		if xgScaleN[pos] > 0 {
			fmt.Printf("  %-4s %.4f  (n=%.0f player-seasons)\n",
				posName[pos], xgScaleSum[pos]/xgScaleN[pos], xgScaleN[pos])
		}
	}
	fmt.Printf("TeamXGCFactor: nil in this diagnostic's engine (never populated outside\n")
	fmt.Printf("cmd/armband's applyTeamOverrides) — inert here, live and one-sided in\n")
	fmt.Printf("production wherever a team override sets xgc_factor.\n")

	var seasons []string
	for s := range obsBySeason {
		seasons = append(seasons, s)
	}
	sort.Strings(seasons)

	// -----------------------------------------------------------------
	// 1. The distribution of the divergence, fixture-adjusted and structural.
	// -----------------------------------------------------------------
	type distStats struct {
		divs              []float64
		meanD, medAbs     float64
		beyond15, beyond2 int
	}
	distribution := func(obs []cohObs, fac, sc float64) distStats {
		var d distStats
		for _, o := range obs {
			if v, ok := cohDivergence(o, fac, sc); ok {
				d.divs = append(d.divs, v)
			}
		}
		if len(d.divs) == 0 {
			return d
		}
		var abs []float64
		var sum float64
		for _, v := range d.divs {
			abs = append(abs, math.Abs(v))
			sum += v
			if math.Abs(v) > math.Log(1.5) {
				d.beyond15++
			}
			if math.Abs(v) > math.Log(2) {
				d.beyond2++
			}
		}
		d.meanD = sum / float64(len(d.divs))
		d.medAbs = median(abs)
		return d
	}
	printDist := func(label string, d distStats) {
		fmt.Printf("\n=== %s\n", label)
		if len(d.divs) == 0 {
			fmt.Printf("no observations.\n")
			return
		}
		fmt.Printf("%d observations. median |log ratio| %.4f (a %.0f%% typical gap).\n",
			len(d.divs), d.medAbs, 100*(math.Exp(d.medAbs)-1))
		fmt.Printf("beyond 1.5x: %d (%.1f%%).   beyond 2x: %d (%.1f%%).\n",
			d.beyond15, 100*float64(d.beyond15)/float64(len(d.divs)),
			d.beyond2, 100*float64(d.beyond2)/float64(len(d.divs)))
		fmt.Printf("mean signed log ratio: %+.4f (positive = attack side predicts more than\n", d.meanD)
		fmt.Printf("the defence side implies conceded).\n")
	}

	withD := distribution(all, factor, scale)
	printDist("WITH calibration — what the engine publishes", withD)
	structD := distribution(all, 1.0, 1.0)
	if factor == 1 && scale == 1 {
		fmt.Printf("\n=== STRUCTURAL (both constants forced to 1) — IDENTICAL to the reading above\n")
		fmt.Printf("printed for completeness only; see the provenance-guard note up top.\n")
	} else {
		printDist("STRUCTURAL — both constants forced to 1", structD)
	}

	fmt.Printf("\n=== the reference arm: same statistic, both fixture multipliers forced to 1\n")
	fmt.Printf("Two estimators built from different halves of the model are not expected to\n")
	fmt.Printf("agree exactly even where the engine is working as designed (see header). This\n")
	fmt.Printf("is the baseline: no opponent adjustment on either side, so any reduction below\n")
	fmt.Printf("it is what the opponent-specific fixture terms actually bought.\n\n")
	var blindAll []cohObs
	for _, o := range all {
		blindAll = append(blindAll, o.blind())
	}
	blindD := distribution(blindAll, factor, scale)
	printDist("fixture-blind baseline (both multipliers = 1)", blindD)
	if blindD.medAbs > 0 {
		reduction := 100 * (1 - withD.medAbs/blindD.medAbs)
		fmt.Printf("\nfixture-adjusted median |log ratio| %.4f against the fixture-blind baseline's\n",
			withD.medAbs)
		fmt.Printf("%.4f: a %+.1f%% change (positive = the fixture terms REDUCE the gap; negative\n",
			blindD.medAbs, reduction)
		fmt.Printf("= the fixture terms make the two sides disagree MORE than doing nothing would).\n")
		if reduction <= 0 {
			fmt.Printf("\n=> The opponent adjustment does NOT reconcile the two sides on this reading.\n")
			fmt.Printf("This is the ladder-gain asymmetry, not evidence the engine improves\n")
			fmt.Printf("coherence by pricing the fixture at all.\n")
		} else {
			fmt.Printf("\n=> The opponent adjustment reduces the gap relative to a fixture-blind\n")
			fmt.Printf("comparison of the same club pairs.\n")
		}
	}

	sink.emitAll("attack_defence_coherence", "GW19 fit, GW20-38 scored", "with calibration", len(withD.divs),
		measure{"cleanSheetXGCFactor", factor},
		measure{"cleanSheetScale", scale},
		measure{"median absolute log divergence", withD.medAbs},
		measure{"fraction beyond 1.5x", ratio(withD.beyond15, len(withD.divs))},
		measure{"fraction beyond 2x", ratio(withD.beyond2, len(withD.divs))},
		measure{"mean signed log divergence", withD.meanD})
	sink.emitAll("attack_defence_coherence", "GW19 fit, GW20-38 scored", "fixture-blind reference", len(blindD.divs),
		measure{"median absolute log divergence", blindD.medAbs},
		measure{"mean signed log divergence", blindD.meanD})

	// -----------------------------------------------------------------
	// 2. Systematic or symmetric? Fixture-adjusted vs the reference arm,
	//    per season, so the same table answers both.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== systematic (a consistent sign) or symmetric (noise between two estimators)?\n")
	fmt.Printf("Per-season mean signed log divergence, adjusted (with-calibration) against the\n")
	fmt.Printf("fixture-blind reference arm. A consistent sign across seasons in the adjusted\n")
	fmt.Printf("column is a calibration offset — cheap to fix, since it is one number. Compare\n")
	fmt.Printf("against the reference column: if the two barely differ, the sign is coming from\n")
	fmt.Printf("the scale asymmetries above, not from fixture pricing.\n\n")
	fmt.Printf("%-10s %8s %10s %10s %8s\n", "season", "n", "adjusted", "reference", "positive%%")
	var perSeasonAdjusted, perSeasonBlind []float64
	for _, s := range seasons {
		var ds, bs []float64
		var pos int
		for _, o := range obsBySeason[s] {
			if d, ok := cohDivergence(o, factor, scale); ok {
				ds = append(ds, d)
				if d > 0 {
					pos++
				}
			}
			if d, ok := cohDivergence(o.blind(), factor, scale); ok {
				bs = append(bs, d)
			}
		}
		if len(ds) == 0 {
			continue
		}
		m := meanOf(ds)
		bm := meanOf(bs)
		perSeasonAdjusted = append(perSeasonAdjusted, m)
		perSeasonBlind = append(perSeasonBlind, bm)
		fmt.Printf("%-10s %8d %10.4f %10.4f %7.1f%%\n", s, len(ds), m, bm,
			100*float64(pos)/float64(len(ds)))
	}
	adjMean, adjSE := meanSE(perSeasonAdjusted)
	blindMean, blindSE := meanSE(perSeasonBlind)
	df := len(perSeasonAdjusted) - 1
	var adjT, blindT float64
	if adjSE > 0 {
		adjT = adjMean / adjSE
	}
	if blindSE > 0 {
		blindT = blindMean / blindSE
	}
	fmt.Printf("%-10s %8s %10.4f %10.4f\n", "MEAN", "", adjMean, blindMean)
	fmt.Printf("%-10s %8s %10.4f %10.4f   (SE)\n", "", "", adjSE, blindSE)
	fmt.Printf("%-10s %8s %10.2f %10.2f   t at df %d, critical %.3f\n",
		"t", "", adjT, blindT, df, tCrit95(df))
	fmt.Printf("\nadjusted %s its own critical t; the fixture-blind reference %s its own.\n",
		map[bool]string{true: "clears", false: "does not clear"}[math.Abs(adjT) > tCrit95(df)],
		map[bool]string{true: "clears", false: "does not clear"}[math.Abs(blindT) > tCrit95(df)])
	sink.emitAll("attack_defence_coherence", "GW19 fit, GW20-38 scored", "season-clustered sign",
		len(perSeasonAdjusted),
		measure{"mean per-season signed log divergence, adjusted", adjMean},
		measure{"its standard error across seasons", adjSE},
		measure{"mean per-season signed log divergence, fixture-blind reference", blindMean},
		measure{"its standard error across seasons", blindSE})

	// -----------------------------------------------------------------
	// 3. Difficulty-pair conditioning, and the worst offenders against it.
	// -----------------------------------------------------------------
	type diffCell struct{ atk, def int }
	cellSum := map[diffCell]float64{}
	cellN := map[diffCell]float64{}
	for _, o := range all {
		if d, ok := cohDivergence(o, factor, scale); ok {
			c := diffCell{o.atkDiff, o.defDiff}
			cellSum[c] += d
			cellN[c]++
		}
	}
	fmt.Printf("\n=== divergence by (attacker difficulty, defender difficulty) cell\n")
	fmt.Printf("Mean with-calibration log divergence per cell. A pattern concentrated at the\n")
	fmt.Printf("corners (1,5) / (5,1) is the ladder's mismatched gains, not incoherence.\n\n")
	fmt.Printf("%-4s", "a\\d")
	for d := 1; d <= 5; d++ {
		fmt.Printf(" %8d", d)
	}
	fmt.Printf("\n")
	for a := 1; a <= 5; a++ {
		fmt.Printf("%-4d", a)
		for d := 1; d <= 5; d++ {
			c := diffCell{a, d}
			if cellN[c] > 0 {
				fmt.Printf(" %8.3f", cellSum[c]/cellN[c])
			} else {
				fmt.Printf(" %8s", "-")
			}
		}
		fmt.Printf("\n")
	}

	fmt.Printf("\n=== the worst offenders, with-calibration reading, raw AND residual\n")
	fmt.Printf("So a human can eyeball whether the engine's claim is absurd on its face — how\n")
	fmt.Printf("the owner found this in the first place. `residual` subtracts the mean\n")
	fmt.Printf("divergence of every OTHER observation sharing this exact difficulty pair, which\n")
	fmt.Printf("is what remains once the ladder-gain asymmetry for that specific pair is\n")
	fmt.Printf("netted out.\n\n")
	type ranked struct {
		o        cohObs
		d, resid float64
	}
	var rk []ranked
	for _, o := range all {
		if d, ok := cohDivergence(o, factor, scale); ok {
			// residual excludes this observation's own contribution to the
			// cell mean, so a cell of size 1 nets to exactly 0 rather than
			// trivially matching itself.
			c := diffCell{o.atkDiff, o.defDiff}
			n := cellN[c]
			var others float64
			if n > 1 {
				others = (cellSum[c] - d) / (n - 1)
			}
			rk = append(rk, ranked{o, d, d - others})
		}
	}
	sort.Slice(rk, func(i, j int) bool { return math.Abs(rk[i].d) > math.Abs(rk[j].d) })
	const topN = 15
	fmt.Printf("%-10s %4s %-24s %5s %10s %10s\n",
		"season", "gw", "attacker vs defender", "diffs", "raw", "residual")
	rawTop := map[int]bool{}
	for i, r := range rk {
		if i >= topN {
			break
		}
		rawTop[i] = true
		fmt.Printf("%-10s %4d %-24s (%d,%d) %10.3f %10.3f\n",
			r.o.season, r.o.gw, r.o.attacker+" vs "+r.o.defender,
			r.o.atkDiff, r.o.defDiff, r.d, r.resid)
	}
	// Which of the raw top N remain in the residual top N?
	byResid := make([]ranked, len(rk))
	copy(byResid, rk)
	sort.Slice(byResid, func(i, j int) bool { return math.Abs(byResid[i].resid) > math.Abs(byResid[j].resid) })
	residSet := map[string]bool{}
	for i := 0; i < topN && i < len(byResid); i++ {
		r := byResid[i]
		residSet[fmt.Sprintf("%s|%d|%s|%s", r.o.season, r.o.gw, r.o.attacker, r.o.defender)] = true
	}
	survived := 0
	for i := 0; i < topN && i < len(rk); i++ {
		r := rk[i]
		key := fmt.Sprintf("%s|%d|%s|%s", r.o.season, r.o.gw, r.o.attacker, r.o.defender)
		if residSet[key] {
			survived++
		}
	}
	fmt.Printf("\n%d of the top %d by RAW divergence remain in the top %d by RESIDUAL "+
		"divergence.\n", survived, topN, topN)
	if survived == topN {
		fmt.Printf("All of them survive conditioning on fixture difficulty: this is not the\n")
		fmt.Printf("ladder, these specific matches disagree beyond what their difficulty pair\n")
		fmt.Printf("predicts for every other match sharing it.\n")
	} else if survived == 0 {
		fmt.Printf("None of them survive: the raw worst-offenders list is fully explained by\n")
		fmt.Printf("which difficulty cell each fixture fell in — this is the ladder asymmetry,\n")
		fmt.Printf("not incoherence.\n")
	} else {
		fmt.Printf("Partial survival: some of the raw extremity is the ladder, some is not —\n")
		fmt.Printf("read the residual column above for which is which, row by row.\n")
	}
	sink.emitAll("attack_defence_coherence", "GW19 fit, GW20-38 scored", "difficulty conditioning",
		len(rk), measure{"raw top-N surviving in residual top-N", float64(survived)})

	// -----------------------------------------------------------------
	// 4. Secondary, DESCRIPTIVE ONLY: which side reads closer to realised
	//    goals. Not gated, not a which-to-trust verdict — see header.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== secondary, DESCRIPTIVE ONLY — not gated, not a which-to-trust verdict\n")
	fmt.Printf("Both sides claim to predict the SAME thing — the attacker's realised expected\n")
	fmt.Printf("goals in this team-gameweek. rms log error against that shared target, printed\n")
	fmt.Printf("per season for transparency. This has no noise floor computed for it and no\n")
	fmt.Printf("season-clustered significance test — it is context, not a conclusion.\n\n")
	fmt.Printf("%-10s %8s %10s %10s\n", "season", "n", "attack rms", "defence rms")
	var atkSS, concSS, n float64
	for _, s := range seasons {
		var as, cs, sn float64
		for _, o := range obsBySeason[s] {
			if o.actXG <= 0 {
				continue
			}
			a := cohAttackPred(o)
			c, ok := cohImpliedLambda(o, factor, scale)
			if a <= 0 || !ok {
				continue
			}
			da := math.Log(a / o.actXG)
			dc := math.Log(c / o.actXG)
			as += da * da
			cs += dc * dc
			sn++
		}
		if sn == 0 {
			continue
		}
		fmt.Printf("%-10s %8.0f %10.4f %10.4f\n", s, sn, math.Sqrt(as/sn), math.Sqrt(cs/sn))
		atkSS += as
		concSS += cs
		n += sn
	}
	var atkRMS, concRMS float64
	if n > 0 {
		atkRMS = math.Sqrt(atkSS / n)
		concRMS = math.Sqrt(concSS / n)
	}
	fmt.Printf("%-10s %8.0f %10.4f %10.4f   POOLED (descriptive)\n", "", n, atkRMS, concRMS)
	fmt.Printf("\nThe pooled attack-side figure reads %s than the pooled defence-side figure.\n",
		map[bool]string{true: "lower", false: "higher (or equal)"}[atkRMS < concRMS])
	fmt.Printf("This is a descriptive reading of ONE run with no significance test attached —\n")
	fmt.Printf("it is not a basis for a which-to-trust decision without further work.\n")
	sink.emitAll("attack_defence_coherence", "GW19 fit, GW20-38 scored", "closer to reality, descriptive",
		int(n),
		measure{"rms log error, attack side", atkRMS},
		measure{"rms log error, defence side, with calibration", concRMS})

	fmt.Printf("\nThis diagnostic authorises nothing: no scoring term moves, and there is no\n")
	fmt.Printf("pre-registered threshold at which self-contradiction becomes acceptable. It is\n")
	fmt.Printf("a fact about the engine, reported plainly either way.\n")
}

// TestCoherenceReadingsAreWiredCorrectly pins the one thing about this
// diagnostic that could be wrong while every printed number stayed plausible:
// that the implied-lambda inversion reads the exponent and the -ln(scale)
// shift with the correct signs, that a bigger calibration factor narrows the
// divergence rather than widening it, and that the reference arm actually
// neutralises both fixture multipliers. Runs without DIAG and without the
// archive.
func TestCoherenceReadingsAreWiredCorrectly(t *testing.T) {
	o := cohObs{rate: 2.0, atkMul: 1.1, defRate: 1.4, defMul: 0.9}

	// attackPred does not depend on the defence side at all.
	if got, want := cohAttackPred(o), 2.0*1.1; math.Abs(got-want) > 1e-12 {
		t.Fatalf("cohAttackPred = %v, want %v", got, want)
	}

	// At factor=1, scale=1, implied lambda is exactly the raw product — no
	// shift, no scaling.
	raw := o.defRate * o.defMul
	got, ok := cohImpliedLambda(o, 1.0, 1.0)
	if !ok || math.Abs(got-raw) > 1e-12 {
		t.Fatalf("implied lambda at factor=1, scale=1 = %v, ok=%v, want the unscaled "+
			"product %v", got, ok, raw)
	}

	// factor scales the exponent linearly.
	got, ok = cohImpliedLambda(o, 1.35, 1.0)
	if !ok || math.Abs(got-1.35*raw) > 1e-12 {
		t.Fatalf("implied lambda at factor=1.35 = %v, want %v", got, 1.35*raw)
	}

	// scale enters as -ln(scale): a scale of exp(-2) must ADD exactly 2 to
	// lambda, since -ln(exp(-2)) = 2. Getting this sign backwards was exactly
	// the defect this test exists to catch.
	got, ok = cohImpliedLambda(o, 1.0, math.Exp(-2))
	if !ok || math.Abs(got-(raw+2)) > 1e-9 {
		t.Fatalf("implied lambda at scale=exp(-2) = %v, want raw+2 = %v — the -ln(scale) "+
			"shift has the wrong sign or is not wired in", got, raw+2)
	}

	// A calibration factor ABOVE 1 raises implied lambda, which must LOWER
	// the divergence (attack over a bigger number) — the sign a flipped
	// factor would get backwards.
	dLow, ok1 := cohDivergence(o, 1.0, 1.0)
	dHigh, ok2 := cohDivergence(o, 2.0, 1.0)
	if !ok1 || !ok2 {
		t.Fatalf("divergence undefined for a strictly positive observation")
	}
	if !(dHigh < dLow) {
		t.Fatalf("divergence at factor 2.0 (%v) is not below factor 1.0 (%v) — a larger "+
			"calibration factor must reduce the gap, since it raises the denominator", dHigh, dLow)
	}

	// The reference arm must zero out BOTH multipliers, and nothing else.
	b := o.blind()
	if b.atkMul != 1 || b.defMul != 1 {
		t.Fatalf("blind() = %+v, want both multipliers forced to 1", b)
	}
	if b.rate != o.rate || b.defRate != o.defRate {
		t.Fatalf("blind() changed a rate it should have left alone: %+v vs %+v", b, o)
	}

	// A non-positive side must report undefined, never a divergence — log of
	// a non-positive ratio is not "no coherence", it is undefined, and the
	// diagnostic must not print a fabricated number for it.
	zero := cohObs{rate: 0, atkMul: 1, defRate: 1, defMul: 1}
	if _, ok := cohDivergence(zero, 1.0, 1.0); ok {
		t.Fatalf("divergence reported ok=true for a zero attack prediction")
	}
	// A scale so large that -ln(scale) drives lambda negative must also
	// report undefined, not a negative "implied goals conceded".
	if _, ok := cohImpliedLambda(o, 0.01, 1e6); ok {
		t.Fatalf("implied lambda reported ok=true for a scale that drives it negative")
	}
}
