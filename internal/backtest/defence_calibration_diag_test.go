package backtest

// Does XGC90 need the calibration ratio the attack side already has?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDefenceCalibrationRatio -v -timeout 60m
//
// # The asymmetry, verified before this file was written
//
// `calibrateExpectedStats` (internal/analysis/metrics.go) accumulates exactly
// four totals per position — goals, xG, assists, xA — and forces the attack
// side onto realised football every run via `CalibrationRatio(actual,
// expected)`. `ConversionScale` (metrics.go) carries only `Goals` and
// `Assists` fields. `CalibrationRatio` is called in exactly four places in
// this repository (metrics.go's own definition and its one production call
// site, season.go's mirror of that call, and enginescale_diag_test.go's own
// mirror for a different diagnostic) and none of them passes it an XGC90-
// derived total. So the defence side has no analogous correction: whatever
// `XGC90` says, `baseXP90` prices, uncorrected.
//
// # Why this is the question, not a repeat of TestDiagAttackDefenceCoherence
//
// The sibling diagnostic in this file's package found the attack side reads
// about 8% above what the defence side implies for the SAME fixture, in
// every season it ran on, and that the lean survives with both fixture
// multipliers forced off — so it is not the fixture ladders. It also ruled
// out `XGScale`: the fitted per-position ratios it printed were close to 1,
// with MID fitted *above* 1, so removing `XGScale` would widen the gap rather
// than close it. See that test's own header for the numbers; they are not
// restated here as a fact this file re-derives, only as the reason this file
// exists.
//
// What is left is the possibility the sibling's header names but does not
// measure: that `XGC90` itself is not calibrated to realised football, the
// way `calibrateExpectedStats` calibrates goals and assists. This file
// answers exactly that, and only that — one number, not a rebuild of the
// coherence check. It does not read `FixtureMultipliersFor`'s ATTACK side at
// all, does not pair fixtures, and produces no worst-offenders table; every
// one of those belongs to the question "do the two sides agree about one
// match", not to "is this one input calibrated".
//
// # The measurement
//
// Realised goals conceded, divided by the model's own implied goals conceded
// from XGC90 — the same ACTUAL/EXPECTED shape `CalibrationRatio` already
// uses for goals and assists, applied to the one total it has never seen.
//
// Implied goals conceded for one club-gameweek is the club's `XGC90`,
// aggregated as the minutes-weighted MEAN over its registered players at the
// fit cutoff — never a sum, for the reason every diagnostic in this package
// gives: `XGC90` is a per-90 rate while a player is on the pitch, and all
// eleven players concede the same goals simultaneously, so summing would read
// roughly eleven times the true figure.
//
// ⚠️ **Deliberately NOT the sibling's `cf` (`DefconCleanFactorFor`).** The
// coherence diagnostic folds `cf` into its defence-side aggregate because it
// is inverting `cleanSheetProb`, and `cf` is part of that formula's exponent
// — omitting it there would measure a different quantity from the one the
// engine scores. This file is not inverting anything; it is asking whether
// the raw INPUT, XGC90, already agrees with realised football, the same
// question `calibrateExpectedStats` asks of xG and xA before any downstream
// term (band adjustment, fixture multiplier, clean-sheet coupling) touches
// it. Folding `cf` in here would answer "is XGC90-times-cf calibrated",
// which conflates two knobs and is not what was asked.
//
// # The fixture-multiplier decision, made and documented rather than assumed
//
// `calibrateExpectedStats`'s own "expected" total (`el.ExpectedGoals`) is
// FPL's raw season-cumulative xG field — nothing per-fixture is layered onto
// it before the actual/expected ratio is taken. The direct analogue for the
// defence side is therefore FIXTURE-BLIND: project each club's cutoff-time
// XGC90 forward unchanged, exactly the slot a defence-side calibration ratio
// would occupy if it existed — before `Engine.FixtureMultipliersFor`'s
// defensive ladder, the same way `XGScale` sits before the attack side's
// fixture multiplier in the coherence diagnostic's own club model. That
// reading is reported as PRIMARY below for that reason.
//
// But XGC90 has to be projected across nineteen gameweeks of different
// opponents, and the engine's own scored path DOES read the defensive ladder
// on every one of them (`baseXP90`'s clean-sheet term reads `m.XGC90 x def`).
// So a FIXTURE-ADJUSTED reading — XGC90 times `FixtureMultipliersFor`'s own
// defensive multiplier for each specific match the club actually played, from
// the point-in-time fixture list — is reported alongside it as SECONDARY,
// answering "is the number the engine actually prices calibrated", which is
// an arguably-equally-valid framing of the same question. Both are printed;
// neither is silently preferred in the numbers, only in which is called
// primary in this comment.
//
// # Unit, and the double-gameweek convention
//
// One observation per club per gameweek. A double gameweek is ONE row: its
// two fixtures' defensive multipliers are summed and then the row's implied
// figure is divided by matches actually played, so a double contributes the
// same weight as a single to every sum below — never double weight from
// playing two matches, and never the unweighted single-match rate either
// when the two opponents' difficulties differ. See `defCalRates` and its
// pinned test for the arithmetic this claim rests on. This is the identical
// discipline `TestDiagAttackDefenceCoherence` and `TestDiagFixtureReconciliation`
// use for their own realised-goals readings.
//
// A team-gameweek where the point-in-time fixture count and the archive's
// played-match count disagree is dropped and counted, never divided by
// whichever number came to hand — same discipline as both siblings.
//
// # Point-in-time discipline
//
// Fit at GW19 (`cohCutoff`, shared with the coherence diagnostic in this
// package — one constant, not a second copy of the same cutoff), scored on
// GW20-38 (`cohFrom`). `calibrateExpectedStats` and `XGC90`'s own blend both
// run on the season to date, so scoring inside the fitting window would be
// partly fitted to its own answer — the same reason every diagnostic in this
// package uses this split. `sweepPairNames()`, registered-at-cutoff players
// only (`PointInTime`'s own filter), horizon 1 (a club's implied concession
// in a specific gameweek's matches, not a multi-gameweek average). The
// defensive multiplier is read from `fx`, the fixture list `PointInTime`
// strips of future scorelines and `Finished` flags, never from
// `cur.Fixtures` — nothing here can see a result before it happened.
// Realised goals conceded, used only for the actual side of the ratio, comes
// from `cur.Fixtures` (scorelines), restricted to the same registered-at-
// cutoff clubs.
//
// # The coverage constraint
//
// The defence side rests entirely on XGC90, and most of this archive's
// seasons carry no native expected-goals-conceded field — AGENTS.md's
// archive-and-data section: "Expected goals conceded is reconstructed for the
// four seasons that carry none." A ratio computed on reconstructed XGC90
// measures the reconstruction, not the football, so coverage through the fit
// cutoff is printed per season BEFORE any ratio table, with the reconstructed
// share flagged beside it — the same discipline the coherence sibling uses.
//
// # Per-position reporting
//
// Goals conceded is a club quantity, not a player one, so "per position" here
// does not mean splitting the target the way `calibrateExpectedStats` splits
// goals and assists. It means checking whether the SAME club-level rate reads
// consistently depending on which position group of registered players
// supplies it — a keeper's XGC90 reflects goals conceded while he specifically
// was on the pitch, a forward's reflects goals conceded while HE was, and
// they need not be built from the same minutes. If the ratio computed from
// keepers alone and from defenders alone disagree materially, that is
// evidence about WHERE a future correction would need to live, which is
// exactly the reporting `XGScale`'s own per-position split gives the attack
// side. This is context, printed pooled and per season for the whole-squad
// aggregate (`ALL`, the primary reading) and pooled only for the four
// position subsets.
//
// # What this changes
//
// Nothing. This is measurement only, per the design note that authorised it.
// No scoring term moves, no config field is added, and a real ratio here does
// not by itself license a correction — AGENTS.md's standing rule that
// correcting a measured bias has lost this project points five times applies
// here exactly as it does to the coherence diagnostic.
//
// # On reproducibility
//
// Every figure below is an accumulation into a per-club-gameweek or
// per-season total, and addition commutes, so map iteration order cannot
// change anything printed here — there is no ranked or selected table in this
// file for order to matter to.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// defCalVariant is one way of picking the registered players whose XGC90
// feeds a club's implied concession rate — the whole squad (the primary
// reading) or one position group (context; see header, "Per-position
// reporting").
type defCalVariant struct {
	name  string
	match func(elementType int) bool
}

var defCalVariants = []defCalVariant{
	{"ALL", func(int) bool { return true }},
	{"GKP", func(p int) bool { return p == 1 }},
	{"DEF", func(p int) bool { return p == 2 }},
	{"MID", func(p int) bool { return p == 3 }},
	{"FWD", func(p int) bool { return p == 4 }},
}

// defCalRates turns one club-gameweek's accumulated fixture data into the
// row's per-match implied concession figures, fixture-blind and fixture-
// adjusted. Pure and archive-free, so the arithmetic the whole diagnostic
// rests on can be pinned directly — see
// TestDefenceCalibrationRatesAreWiredCorrectly.
//
// clubRate is the club's cutoff-time XGC90, minutes-weighted mean over
// registered players (see header). defMulSum is the SUM of
// Engine.FixtureMultipliersFor's defensive return across every fixture the
// club played this gameweek — one per match, so a double contributes two
// terms. nFx is how many fixtures that was.
//
// blind is exactly clubRate, independent of nFx or defMulSum: the fixture-
// blind reading by definition does not read the opponent, so a double
// gameweek's per-match average is unchanged from a single's. adj is clubRate
// times the MEAN defensive multiplier across the gameweek's fixtures — the
// "both sides divided by matches actually played" convention applied to a
// quantity built by summing one multiplier per match.
func defCalRates(clubRate, defMulSum, nFx float64) (blind, adj float64) {
	if nFx <= 0 || clubRate <= 0 {
		return 0, 0
	}
	return clubRate, clubRate * defMulSum / nFx
}

// defCalRatio is actual/implied, both already summed across whatever rows
// are in scope — the same shape CalibrationRatio itself uses (metrics.go:
// t.goals / t.xG, sums of totals, never a mean of per-row ratios), and false
// when there is nothing to divide by. Unlike CalibrationRatio this does not
// default to neutral on a thin sample: a diagnostic measuring whether XGC90
// is calibrated must not print 1.0 for "not measured", which would be
// indistinguishable from "measured and found perfect".
func defCalRatio(actual, implied float64) (float64, bool) {
	if implied <= 0 {
		return 0, false
	}
	return actual / implied, true
}

func TestDiagDefenceCalibrationRatio(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	fmt.Printf("\n=== what this measures\n")
	fmt.Printf("realised goals conceded / this model's implied goals conceded from XGC90,\n")
	fmt.Printf("fit at GW%d, scored GW%d-38 — the calibration ratio calibrateExpectedStats\n", cohCutoff, cohFrom)
	fmt.Printf("computes for goals and assists but never for XGC90. See this file's header\n")
	fmt.Printf("for the fixture-multiplier decision (both readings are printed) and why cf\n")
	fmt.Printf("is deliberately excluded.\n")

	fmt.Printf("\n=== coverage: the rows behind each fit, through GW%d\n", cohCutoff)
	fmt.Printf("%-10s %10s %10s %12s\n", "season", "xG rows", "xGC rows", "xGC rebuilt")

	type key struct{ variant, season string }
	sumActual := map[key]float64{}
	sumBlind := map[key]float64{}
	sumAdj := map[key]float64{}
	rows := map[key]int{}

	var seasons []string
	var totalDropped, totalMismatched, totalDoubles int

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
		flag := ""
		if rebuiltPct >= 50 {
			flag = "  <- mostly reconstructed, not native"
		}
		fmt.Printf("%-10s %10.0f %10.0f %11.0f%%%s\n", cur.Name, xgRows, xgcRows, rebuiltPct, flag)

		boot, fx := PointInTime(cur, prior, cohCutoff)
		w := cfg.Weights
		w.Horizon = 1
		e := analysis.NewEngineFull(boot, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = newPriorIndex(prior)
		e.Recent = newRecentIndexWith(cur, cohCutoff, w.MinutesHalfLife, w.RateHalfLife)

		// Cutoff-time club rate per variant: minutes-weighted mean XGC90 over
		// registered players matching that variant's position filter.
		type accum struct{ num, den float64 }
		acc := map[string]map[int]*accum{}
		for _, v := range defCalVariants {
			acc[v.name] = map[int]*accum{}
		}
		for i := range boot.Elements {
			el := &boot.Elements[i]
			m := e.Metrics(el)
			for _, v := range defCalVariants {
				if !v.match(el.ElementType) {
					continue
				}
				a := acc[v.name][el.Team]
				if a == nil {
					a = &accum{}
					acc[v.name][el.Team] = a
				}
				a.num += m.XGC90 * m.ExpectedMinutes
				a.den += m.ExpectedMinutes
			}
		}
		rate := map[string]map[int]float64{}
		for _, v := range defCalVariants {
			rate[v.name] = map[int]float64{}
			for team, a := range acc[v.name] {
				if a.den > 0 && a.num > 0 {
					rate[v.name][team] = a.num / a.den
				}
			}
		}

		// Per (team, gameweek): the point-in-time fixture count, the sum of
		// this club's OWN defensive multiplier across those fixtures, the
		// archive's played-match count and realised goals conceded.
		type tgKey struct{ team, gw int }
		type gwAcc struct{ nFx, defMulSum, played, conceded float64 }
		byGW := map[tgKey]*gwAcc{}
		get := func(team, gw int) *gwAcc {
			k := tgKey{team, gw}
			if a := byGW[k]; a != nil {
				return a
			}
			a := &gwAcc{}
			byGW[k] = a
			return a
		}

		for _, f := range fx {
			if f.Event == nil || *f.Event < cohFrom || *f.Event > 38 {
				continue
			}
			gw := *f.Event
			_, defH := e.FixtureMultipliersFor(analysis.FixtureBrief{
				Event: gw, OpponentID: f.TeamA, Difficulty: f.TeamHDifficulty,
			})
			_, defA := e.FixtureMultipliersFor(analysis.FixtureBrief{
				Event: gw, OpponentID: f.TeamH, Difficulty: f.TeamADifficulty,
			})
			h := get(f.TeamH, gw)
			h.nFx++
			h.defMulSum += defH
			a := get(f.TeamA, gw)
			a.nFx++
			a.defMulSum += defA
		}
		for _, f := range cur.Fixtures {
			if f.Event == nil || *f.Event < cohFrom || *f.Event > 38 {
				continue
			}
			if f.TeamHScore == nil || f.TeamAScore == nil {
				continue // not played, or the archive did not record it
			}
			h := get(f.TeamH, *f.Event)
			h.played++
			h.conceded += float64(*f.TeamAScore)
			a := get(f.TeamA, *f.Event)
			a.played++
			a.conceded += float64(*f.TeamHScore)
		}

		var dropped, mismatched, doubles int
		for k, a := range byGW {
			switch {
			case a.nFx == 0 || a.played == 0:
				dropped++
			case a.nFx != a.played:
				mismatched++
			default:
				if a.nFx >= 2 {
					doubles++
				}
				actualPerMatch := a.conceded / a.played
				for _, v := range defCalVariants {
					r, ok := rate[v.name][k.team]
					if !ok {
						continue
					}
					blind, adj := defCalRates(r, a.defMulSum, a.nFx)
					if blind <= 0 || adj <= 0 {
						continue
					}
					sk := key{v.name, cur.Name}
					sumActual[sk] += actualPerMatch
					sumBlind[sk] += blind
					sumAdj[sk] += adj
					rows[sk]++
				}
			}
		}
		totalDropped += dropped
		totalMismatched += mismatched
		totalDoubles += doubles
		if rows[key{"ALL", cur.Name}] > 0 {
			seasons = append(seasons, cur.Name)
		}
	}

	if len(seasons) < 2 {
		t.Skipf("only %d season(s) produced observations; there is no between-season "+
			"spread to report", len(seasons))
	}
	sort.Strings(seasons)

	fmt.Printf("\n%d team-gameweeks dropped for no point-in-time fixture or no played match, "+
		"%d dropped\nfor a fixture count the played record disagrees with, %d were doubles "+
		"(one row, both\nsides divided by matches played — see header). All counts are pooled "+
		"across the ALL\nvariant's rows only; a position subset can only ever drop MORE rows "+
		"than ALL, never\nfewer, since it is a subset of the same club-gameweeks.\n",
		totalDropped, totalMismatched, totalDoubles)

	// -----------------------------------------------------------------
	// Primary reading: the ALL variant, per season and pooled, both the
	// fixture-blind and fixture-adjusted implied-concession readings.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== primary: whole-squad XGC90, actual goals conceded / implied\n")
	fmt.Printf("%-10s %6s %10s %12s %8s %12s %8s\n",
		"season", "rows", "actual", "implied", "ratio", "implied", "ratio")
	fmt.Printf("%-10s %6s %10s %12s %8s %12s %8s\n",
		"", "", "(sum)", "(blind)", "blind", "(adj)", "adj")

	var seasonRatioBlind, seasonRatioAdj []float64
	var poolActual, poolBlind, poolAdj float64
	for _, s := range seasons {
		sk := key{"ALL", s}
		a, b, adj := sumActual[sk], sumBlind[sk], sumAdj[sk]
		rb, okB := defCalRatio(a, b)
		ra, okA := defCalRatio(a, adj)
		if okB {
			seasonRatioBlind = append(seasonRatioBlind, rb)
		}
		if okA {
			seasonRatioAdj = append(seasonRatioAdj, ra)
		}
		fmt.Printf("%-10s %6d %10.1f %12.1f %8.3f %12.1f %8.3f\n",
			s, rows[sk], a, b, rb, adj, ra)
		poolActual += a
		poolBlind += b
		poolAdj += adj
	}
	poolRatioBlind, _ := defCalRatio(poolActual, poolBlind)
	poolRatioAdj, _ := defCalRatio(poolActual, poolAdj)
	fmt.Printf("%-10s %6s %10.1f %12.1f %8.3f %12.1f %8.3f   POOLED (sum of sums, not "+
		"mean of ratios)\n", "", "", poolActual, poolBlind, poolRatioBlind, poolAdj, poolRatioAdj)

	fmt.Printf("\n=== season-clustered significance, ratio against the null of 1 (no missing " +
		"calibration)\n")
	df := len(seasons) - 1
	crit := tCrit95(df)
	report := func(label string, rs []float64) {
		mean, se := meanSE(rs)
		var tv float64
		if se > 0 {
			tv = (mean - 1) / se
		}
		fmt.Printf("%-10s mean %.4f   SE %.4f   t %.2f   df %d   critical %.3f   %s\n",
			label, mean, se, tv, df, crit,
			map[bool]string{true: "clears its critical t", false: "does not clear its critical t"}[math.Abs(tv) > crit])
	}
	report("blind", seasonRatioBlind)
	report("adjusted", seasonRatioAdj)
	fmt.Printf("(critical value is tCrit95(%d) = %.3f, printed rather than assumed — see "+
		"stats_test.go)\n", df, crit)

	meanBlind, seBlind := meanSE(seasonRatioBlind)
	meanAdj, seAdj := meanSE(seasonRatioAdj)
	sink.emitAll("defence_calibration_ratio", "GW19 fit, GW20-38 scored", "ALL, pooled", len(seasons),
		measure{"pooled ratio, fixture-blind", poolRatioBlind},
		measure{"pooled ratio, fixture-adjusted", poolRatioAdj},
		measure{"season-clustered mean ratio, fixture-blind", meanBlind},
		measure{"season-clustered SE, fixture-blind", seBlind},
		measure{"season-clustered mean ratio, fixture-adjusted", meanAdj},
		measure{"season-clustered SE, fixture-adjusted", seAdj})

	// -----------------------------------------------------------------
	// Context: does the ratio depend on which position group supplies the
	// club rate? Pooled only — see header, "Per-position reporting".
	// -----------------------------------------------------------------
	fmt.Printf("\n=== context: pooled ratio by which position group's XGC90 built the club rate\n")
	fmt.Printf("Not a second significance test — a between-position spread here says where a\n")
	fmt.Printf("future correction would need to live, not whether one is warranted.\n\n")
	fmt.Printf("%-6s %6s %10s %8s %8s\n", "pos", "rows", "actual", "blind", "adj")
	for _, v := range defCalVariants {
		var a, b, adj float64
		var n int
		for _, s := range seasons {
			sk := key{v.name, s}
			a += sumActual[sk]
			b += sumBlind[sk]
			adj += sumAdj[sk]
			n += rows[sk]
		}
		rb, _ := defCalRatio(a, b)
		ra, _ := defCalRatio(a, adj)
		fmt.Printf("%-6s %6d %10.1f %8.3f %8.3f\n", v.name, n, a, rb, ra)
		sink.emitAll("defence_calibration_ratio", "GW19 fit, GW20-38 scored", v.name+", pooled", n,
			measure{"pooled ratio, fixture-blind", rb},
			measure{"pooled ratio, fixture-adjusted", ra})
	}

	fmt.Printf("\nThis diagnostic authorises no scoring change. A ratio far from 1 says XGC90 " +
		"is\nmiscalibrated against realised football; what to do about it, if anything, is a " +
		"\nseparate decision this diagnostic does not make — correcting a measured bias has " +
		"\nlost this project points five times (AGENTS.md, Standing rules).\n")
}

// TestDefenceCalibrationRatesAreWiredCorrectly pins the arithmetic
// TestDiagDefenceCalibrationRatio is built on: that the blind reading never
// reads the fixture multiplier, that the adjusted reading uses the MEAN
// multiplier across a double's two fixtures rather than their sum, and that
// defCalRatio guards a zero denominator instead of returning a fabricated
// number. Runs without DIAG and without the archive.
func TestDefenceCalibrationRatesAreWiredCorrectly(t *testing.T) {
	// A single fixture at multiplier 1: both readings equal the raw rate.
	blind, adj := defCalRates(1.4, 1.0, 1.0)
	if blind != 1.4 || adj != 1.4 {
		t.Fatalf("single fixture, multiplier 1: blind=%v adj=%v, want both 1.4", blind, adj)
	}

	// A single fixture at a multiplier other than 1: adj scales with it,
	// blind must not move at all — getting this backwards would make the
	// "fixture-blind" reading not actually fixture-blind.
	blind, adj = defCalRates(1.4, 1.2, 1.0)
	if blind != 1.4 {
		t.Fatalf("blind read the fixture multiplier: got %v, want the unchanged rate 1.4", blind)
	}
	if math.Abs(adj-1.4*1.2) > 1e-12 {
		t.Fatalf("adj = %v, want clubRate x multiplier = %v", adj, 1.4*1.2)
	}

	// A double gameweek, two different opponents (multipliers 0.7 and 1.4,
	// sum 2.1, mean 1.05): blind is STILL the single-match rate, never
	// doubled — the whole point of the "divided by matches played"
	// convention — and adj is the rate times the MEAN multiplier, not the
	// sum, which would silently double-count a double gameweek's weight.
	blind, adj = defCalRates(1.0, 0.7+1.4, 2.0)
	if blind != 1.0 {
		t.Fatalf("blind on a double = %v, want the unchanged single-match rate 1.0", blind)
	}
	if math.Abs(adj-1.05) > 1e-12 {
		t.Fatalf("adj on a double = %v, want clubRate x mean multiplier = 1.05", adj)
	}

	// Zero fixtures this gameweek, or no club rate: both undefined, reported
	// as 0 rather than a divide-by-zero silently entering a sum as NaN or
	// +Inf.
	if blind, adj := defCalRates(1.0, 0, 0); blind != 0 || adj != 0 {
		t.Fatalf("zero fixtures: blind=%v adj=%v, want both 0", blind, adj)
	}
	if blind, adj := defCalRates(0, 1.0, 1.0); blind != 0 || adj != 0 {
		t.Fatalf("zero club rate: blind=%v adj=%v, want both 0", blind, adj)
	}

	// defCalRatio: the CalibrationRatio shape, sums not row means, and a
	// guarded zero denominator rather than a fabricated neutral value.
	if r, ok := defCalRatio(108, 100); !ok || math.Abs(r-1.08) > 1e-12 {
		t.Fatalf("defCalRatio(108, 100) = %v, ok=%v, want 1.08, true", r, ok)
	}
	if r, ok := defCalRatio(0, 0); ok {
		t.Fatalf("defCalRatio(0, 0) reported ok=true (r=%v) — a zero denominator must be "+
			"undefined, not read as a perfect ratio", r)
	}
	if r, ok := defCalRatio(50, 0); ok {
		t.Fatalf("defCalRatio with a zero denominator reported ok=true (r=%v)", r)
	}
	// Actual conceded of exactly 0 (a clean sheet) is real football, not a
	// missing observation — it must reduce the ratio, not be treated as
	// undefined the way a zero denominator is.
	if r, ok := defCalRatio(0, 1.3); !ok || r != 0 {
		t.Fatalf("defCalRatio(0, 1.3) = %v, ok=%v, want 0, true — a clean sheet is a real "+
			"zero, not an undefined observation", r, ok)
	}
}
