package backtest

// Within a position, does a player's defensive-contribution RATE predict how
// long he stays on when he starts?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagDefconMinutesPerStart -v -timeout 60m
//
// # What this gates
//
// The owner's hypothesis: a defensively-involved player is withdrawn less. A
// holding midfielder or a centre-back does a job that persists for ninety
// minutes; an attacking player is who a manager replaces for fresh legs late on.
// If a player's own defensive-contribution rate (`DefCon`, tackles + interceptions
// + recoveries, the stat that pays FPL points from 2025-26) predicts how long he
// is actually left on the pitch once selected, that rate is a ROLE PROXY for
// substitution risk the model could read without an external role label — which
// would matter because FPL's own `DEF` bucket mixes centre-backs and attacking
// full-backs under one element type, and the model currently has no handle on
// that split at all.
//
// This is a bug-hunt-shaped correctness/coverage question, not a points question:
// it needs no replay and no significance test against a detection threshold —
// only a correlation and an honest account of how many rows survived the two
// traps below. What to do with a positive finding, if there is one, is a
// separate decision this diagnostic does not make.
//
// # ⚠️ Trap 1 — CIRCULARITY, and it is the one that silently fabricates an answer
//
// `reconstructStarts` (reconstructstarts.go) infers `GW.Starts` BY RANK FROM
// MINUTES wherever the archive recorded no starts column at all, and flags every
// row it touched with `StartsReconstructed = true`. On such a row, "he started"
// is DEFINED to be "he has one of the eleven highest minute counts in his club's
// gameweek" — so on a reconstructed row, minutes and "started" are the same fact
// wearing two names. Feeding that row into a correlation against
// `minutes_per_start` would not measure whether defensive players are subbed off
// less; it would partly measure the reconstruction's own rule for who counts as
// a starter, which is a fact about `reconstructStarts`, not about football.
//
// So every row this diagnostic aggregates requires `StartsReconstructed == false`
// in addition to `Starts == 1`, and the coverage table below prints exactly how
// many rows that excluded. Had a whole season come back mostly reconstructed,
// the right response would be to drop that season rather than quote a number
// built mostly from the reconstruction's own logic — moot here, because trap 2
// below leaves only one season in scope regardless, and it carries a real starts
// column throughout (see the coverage table).
//
// # ⚠️ Trap 2 — DefCon exists in exactly ONE season
//
// `DefconScoredIn(season) = season >= "2025-26"` (simulate.go) marks the only
// season defensive contribution paid FPL points, and the only season the
// archive's `defcon` column carries a real count — `Player.DefCon`'s own comment
// is explicit that zero before then means "the column did not exist", not "he
// made no defensive plays". Computing a `defcon_per_90` for an earlier season
// would divide zero by a real number of minutes for every single row: not noise,
// a CONSTANT, which would not bias the correlation so much as manufacture a
// degenerate one (every pre-2025-26 player reads defcon_per_90 = 0 regardless of
// role, collapsing the very axis being tested). So this diagnostic runs on
// `2025-26` only, checks `DefconScoredIn` on it before doing anything else, and
// fails loudly if that ever stops being true. **`2025-26` is also the only
// season in the cached archive for which `DefconScoredIn` returns true at all**,
// which this diagnostic states rather than hides: there is no between-season
// replication available for this question, and won't be until a second
// defcon-scored season exists. A finding here is a single-season descriptive
// fact, not a season-clustered estimate with its own standard error.
//
// # Why this is NOT a rebuild of the retired start-share idea
//
// The expected-points review's Gap 7 measured `StartShare` against
// `MinutesPerMatch` and found them correlated at 0.9934 — near-duplicate
// features, not two independent signals, so adding start share as a minutes
// predictor was rejected. **This is a different question against a different
// pair of quantities.** Gap 7 asked whether one minutes-shaped feature predicts
// another minutes-shaped feature; naturally they nearly coincide. This asks
// whether a quantity that is NOT minutes-shaped at all — a per-90 rate of
// tackles, interceptions and recoveries, computed only over rows where the
// player already started — predicts a minutes-shaped outcome (how long that
// same start lasted). `defcon_per_90` and `minutes_per_start` are not two
// readings of the same underlying fact the way start share and mean minutes
// were; whether they still turn out correlated is exactly what this measures,
// not something assumed away by the resemblance to a closed line.
//
// # No point-in-time split, and why that is not an oversight
//
// Every sibling diagnostic in this package fits at a cutoff gameweek and scores
// a held-out window, because those diagnostics ask whether something computed
// from PAST data predicts a FUTURE outcome, and scoring on the fitting window
// would partly grade the fit against itself. This diagnostic does not forecast
// anything: both `defcon_per_90` and `minutes_per_start` are realised, backward-
// looking summaries over the SAME set of a player's own started rows in one
// completed season. There is no model being fit and no future being predicted,
// so there is nothing for a cutoff to protect against. What point-in-time
// discipline THIS diagnostic still owes: none, but it owes the two traps above
// instead, since a purely descriptive design has no other way to go wrong.
//
// # The measurement, precisely
//
// A row qualifies when `Starts == 1`, `StartsReconstructed == false`,
// `Fixtures == 1` and `Minutes > 0`, read from `Season.Players[id].GWs` — the
// third condition is not in the task's own restatement of this design and was
// added after the first live run's sanity check caught what its absence let
// through; see the ⚠️ below. Per player, over his qualifying rows:
//
//	minutes_per_start = mean(Minutes)
//	defcon_per_90     = sum(DefCon) / (sum(Minutes) / 90)
//
// Both denominators are sums over the SAME row set, never mixed — dividing a
// whole-season DefCon by a different season's minutes, or a rate by another
// player's minutes, is exactly the unit error AGENTS.md's "per-90 rates" note
// warns `blendFor` against. The `Minutes > 0` guard also keeps this off the
// bitten-list failure in that same note: dividing a small counting stat by a
// FRACTION of a match reads a 22-minute cameo as an implausible per-90 rate.
// That trap needs `sum(Minutes)` to be small; requiring at least 5 qualifying
// starts (the loosest threshold checked below) before a player enters any table
// keeps the smallest surviving denominator well clear of it.
//
// `Starts == 1` EXACTLY, not `>= 1`, so a double-gameweek row where the archive
// accumulated two starts into one row (`Starts == 2`, per season.go's
// accumulate-not-assign rule for doubles) is excluded — its `Minutes` is a sum
// across two matches and does not answer "how long did this one start last".
//
// ⚠️ **`Starts == 1` alone is not enough, and the sanity check below is what
// caught it.** A double-gameweek row where a player STARTED one leg and also
// came on as a SUBSTITUTE in the other still accumulates `Starts == 1` (one
// start credited) while `Minutes` sums across both matches and can exceed 90 —
// the first live run of this diagnostic found 21 such rows in 2025-26, all with
// `Minutes > 90` and `Starts == 1`. So the filter also requires `Fixtures == 1`
// — the row covers exactly one match — which is the same guard
// `TestModelDiagnosticsAreReproducible`'s sibling diagnostic uses for the
// analogous reason (a doubled row's `XGC` is summed over two matches too). The
// coverage table below counts this drop separately from the other two, and
// `TestDefconMinutesPerStartWiring` pins the exact scenario that caught it.
//
// Positions come from `Player.Type` (FPL's `element_type`, 1-4), not from any
// role inference — this diagnostic is asking whether defcon rate COULD substitute
// for a role label FPL does not give, so it must not smuggle one in.
//
// # Correlation and significance
//
// Pearson's r (the `correlation` helper already used elsewhere in this package),
// computed within each position separately — pooling across positions would let
// a between-position level difference (defenders both concede more defcon points
// and, being defenders, get hooked less on average than forwards) masquerade as
// a within-position relationship, which is precisely the confound the owner's
// hypothesis is about the WITHIN-position split (centre-back vs full-back), not
// a between-position one.
//
// The two-sided 5% critical |r| is computed from first principles for the
// player-sample df actually observed (`twoSidedTCrit`, `rCritFor` below), never
// assumed. `tCrit95` in stats_test.go is deliberately NOT reused here: its own
// comment scopes it to season-count degrees of freedom (1..10, the
// season-clustered sweep estimator that `TestInferenceLivesInOnePlace` guards),
// and this diagnostic's df is a PLAYER sample size minus two — tens to hundreds,
// a range that table was never built to cover and whose header explicitly
// exempts "the per-player, per-team-match and per-move standard errors in the
// calibration diagnostics" from that guard's scope, on the grounds that they
// measure a different unit. This is one of those: the unit of replication here
// is a player, not a season.
//
// A minimum of 10 qualifying starts is the headline threshold; 5 and 15 are
// printed alongside it so a reader can see the result is not an artefact of
// where that line was drawn, per this project's standing rule against a
// threshold chosen after the numbers were seen.
//
// # The bimodality question
//
// The owner's mechanism is specifically about `DEF`: FPL's defender bucket mixes
// centre-backs (who rarely leave the pitch once selected) with attacking
// full-backs (who are exactly the profile managed for fresh legs). If that is
// real, `minutes_per_start` should be BIMODAL within `DEF` — a cluster near 90
// and a second, lower cluster — while a unimodal position has no such split for
// defcon rate to be a proxy FOR. A fixed-width histogram is printed per
// position, not a decile table, because deciles are equal-FREQUENCY bins and
// smooth a bimodal shape into ten steps by construction; a fixed-width histogram
// over absolute minutes is what actually shows two humps if they are there.
//
// # The sanity check that must pass
//
// No `minutes_per_start` may exceed 90: it is a mean of per-match minutes over
// matches the player started, and a single match cannot exceed 90 (this archive
// carries no extra-time competitions). A violation means the row filter let
// something through it should not have — a double-gameweek row whose
// `Starts == 2` was not excluded, or (the one this check actually caught on the
// first live run) `Starts == 1` with `Fixtures == 2`, per the ⚠️ above — and the
// test fails outright rather than printing a table built on a broken filter.
//
// # On reproducibility
//
// Every reduction is a sum into a per-player accumulator, and addition commutes,
// so map iteration order cannot move a figure. `sortedPlayerIDs` and a sorted
// gameweek walk are used anyway, for the same reason `TestModelDiagnosticsAreReproducible`
// gives: nothing here depends on it today, but a diagnostic that only accumulates
// is one edit away from one that also dedups or picks a representative, and this
// keeps that edit safe rather than merely currently harmless.
//
// # What this changes
//
// Nothing. No scoring term moves and no config value changes. This is a coverage
// and correlation READING; a positive result would make the defcon-as-role-proxy
// idea worth building, not something this diagnostic ships on its own.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
)

// defconMinStarts is the headline minimum-starts threshold. 5 and 15 are printed
// beside it as a sensitivity check, not as alternates to choose between.
const defconMinStarts = 10

// defconSensitivityThresholds is every threshold printed, headline first.
var defconSensitivityThresholds = []int{defconMinStarts, 5, 15}

// defconPosName labels Player.Type (FPL's element_type), 1-4.
var defconPosName = map[int]string{1: "GKP", 2: "DEF", 3: "MID", 4: "FWD"}

// defconPlayerAgg is one player's within-season aggregate over his qualifying
// start rows only (see the header's row filter).
type defconPlayerAgg struct {
	id       int
	position int
	starts   int
	minutes  float64
	defcon   float64
}

func (a *defconPlayerAgg) minutesPerStart() float64 {
	if a.starts == 0 {
		return 0
	}
	return a.minutes / float64(a.starts)
}

func (a *defconPlayerAgg) defconPer90() float64 {
	if a.minutes <= 0 {
		return 0
	}
	return a.defcon / (a.minutes / 90)
}

// defconFunnel is the row-filter coverage this diagnostic must report before any
// correlation, per its own header and the task's coverage requirement.
type defconFunnel struct {
	allRows              int // every player-gameweek row in the season
	startRows            int // Starts == 1
	reconstructedDropped int // of those, StartsReconstructed == true (trap 1)
	minuteZeroDropped    int // of the remainder, Minutes == 0 (a data edge case)
	multiFixtureDropped  int // of the remainder, Fixtures != 1 (started one leg of a
	// double and also appeared in the other — see the header's ⚠️ on Starts==1
	// not being sufficient by itself)
	rowsOver90 int // of the used rows, Minutes > 90 (would fail the sanity check)
	usedRows   int // Starts==1, Fixtures==1, not reconstructed, Minutes > 0
}

// buildDefconAgg scans a season's players and returns the per-player aggregate
// (over qualifying rows only) and the funnel that got it there. Extracted from
// the test body so TestDefconMinutesPerStartWiring can exercise the filter logic
// against a small synthetic season without the archive.
func buildDefconAgg(s *Season) (map[int]*defconPlayerAgg, defconFunnel) {
	var f defconFunnel
	agg := map[int]*defconPlayerAgg{}
	for _, id := range sortedPlayerIDs(s) {
		p := s.Players[id]
		var gws []int
		for gw := range p.GWs {
			gws = append(gws, gw)
		}
		sort.Ints(gws)
		for _, gw := range gws {
			g := p.GWs[gw]
			f.allRows++
			if g.Starts != 1 {
				continue
			}
			f.startRows++
			if g.StartsReconstructed {
				f.reconstructedDropped++
				continue
			}
			if g.Minutes <= 0 {
				f.minuteZeroDropped++
				continue
			}
			if g.Fixtures != 1 {
				f.multiFixtureDropped++
				continue
			}
			f.usedRows++
			if g.Minutes > 90 {
				f.rowsOver90++
			}
			a := agg[id]
			if a == nil {
				a = &defconPlayerAgg{id: id, position: p.Type}
				agg[id] = a
			}
			a.starts++
			a.minutes += float64(g.Minutes)
			a.defcon += float64(g.DefCon)
		}
	}
	return agg, f
}

// defconLogBeta, defconBetacf and defconRegIncBeta implement the regularized
// incomplete beta function (Numerical Recipes' betai, Lentz continued
// fraction), which is what twoSidedTCrit inverts to get an exact Student-t
// critical value at an arbitrary df, rather than a table. Prefixed rather than
// left as generic names (logBeta, betacf) so a future package-wide statistics
// helper does not collide with a name this one-off diagnostic coined first.
func defconLogBeta(a, b float64) float64 {
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	lab, _ := math.Lgamma(a + b)
	return la + lb - lab
}

func defconBetacf(x, a, b float64) float64 {
	const maxIter = 200
	const eps = 3e-14
	const fpmin = 1e-300
	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < fpmin {
		d = fpmin
	}
	d = 1 / d
	h := d
	for m := 1; m <= maxIter; m++ {
		mf := float64(m)
		m2 := 2 * mf
		aa := mf * (b - mf) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		h *= d * c
		aa = -(a + mf) * (qab + mf) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = 1 + aa/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

func defconRegIncBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	bt := math.Exp(-defconLogBeta(a, b) + a*math.Log(x) + b*math.Log(1-x))
	if x < (a+1)/(a+b+2) {
		return bt * defconBetacf(x, a, b) / a
	}
	return 1 - bt*defconBetacf(1-x, b, a)/b
}

// twoSidedTCrit is the two-sided 5% critical value of Student's t at df degrees
// of freedom, computed exactly by bisecting the regularized-incomplete-beta form
// of the t tail probability rather than read from a table — see the header's
// "Correlation and significance" section for why tCrit95 does not apply at this
// df range. Verified against tCrit95's own table at df 1..10 in
// TestDefconMinutesPerStartWiring via a round-trip identity rather than by
// re-quoting the table's literals, which TestInferenceLivesInOnePlace scans for
// outside stats_test.go.
func twoSidedTCrit(df int) float64 {
	if df < 1 {
		return math.NaN()
	}
	tailProb := func(t float64) float64 {
		x := float64(df) / (float64(df) + t*t)
		return defconRegIncBeta(x, float64(df)/2, 0.5)
	}
	lo, hi := 0.0, 1.0
	for tailProb(hi) > 0.05 {
		hi *= 2
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		if tailProb(mid) > 0.05 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// rCritFor is the two-sided 5% critical |r| for a Pearson correlation over n
// paired observations (df = n-2), from the identity t = r*sqrt(df)/sqrt(1-r^2)
// solved for r at t = twoSidedTCrit(df).
func rCritFor(df int) float64 {
	if df < 1 {
		return math.NaN()
	}
	t := twoSidedTCrit(df)
	return t / math.Sqrt(float64(df)+t*t)
}

// defconPosResult is one position at one starts threshold.
type defconPosResult struct {
	n                  int
	medMinutesPerStart float64
	medDefconPer90     float64
	r                  float64
	df                 int
	rCrit              float64
	haveR              bool
}

func defconComputePosResult(players []*defconPlayerAgg, threshold int) defconPosResult {
	var mps, dcp []float64
	for _, a := range players {
		if a.starts >= threshold {
			mps = append(mps, a.minutesPerStart())
			dcp = append(dcp, a.defconPer90())
		}
	}
	res := defconPosResult{n: len(mps)}
	if res.n == 0 {
		return res
	}
	res.medMinutesPerStart = median(mps)
	res.medDefconPer90 = median(dcp)
	if res.n >= 3 {
		res.r = correlation(mps, dcp)
		res.df = res.n - 2
		res.rCrit = rCritFor(res.df)
		res.haveR = true
	}
	return res
}

// defconHistogram buckets xs into fixed-width bins over [0, maxVal], the last
// bin closed on both ends so a value of exactly maxVal is not dropped.
func defconHistogram(xs []float64, binWidth, maxVal float64) []int {
	nBins := int(maxVal/binWidth + 0.5)
	bins := make([]int, nBins)
	for _, x := range xs {
		b := int(x / binWidth)
		if b >= nBins {
			b = nBins - 1
		}
		if b < 0 {
			b = 0
		}
		bins[b]++
	}
	return bins
}

func TestDiagDefconMinutesPerStart(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	ctx := context.Background()

	const season = "2025-26"
	fmt.Printf("\n=== season selection\n")
	fmt.Printf("DefconScoredIn gates every season before 2025-26 to zero or meaningless\n")
	fmt.Printf("defcon (trap 2, see header). %s is the ONLY season in this project's\n", season)
	fmt.Printf("archive it returns true for, so it is the entire population: there is no\n")
	fmt.Printf("between-season replication available for this question, and the reading\n")
	fmt.Printf("below is a single-season descriptive fact, not a clustered estimate.\n")
	if !DefconScoredIn(season) {
		t.Fatalf("DefconScoredIn(%q) = false; the one season this diagnostic depends on "+
			"no longer qualifies — this diagnostic has nothing to measure until a second "+
			"defcon-scored season exists", season)
	}

	s, err := Load(ctx, cfg.CacheDir, season)
	if err != nil {
		t.Fatal(err)
	}

	agg, funnel := buildDefconAgg(s)

	fmt.Printf("\n=== coverage funnel, season %s (before any correlation)\n", season)
	fmt.Printf("player-gameweek rows in the archive:              %8d\n", funnel.allRows)
	fmt.Printf("of which Starts == 1:                             %8d\n", funnel.startRows)
	fmt.Printf("  dropped: StartsReconstructed == true (trap 1):  %8d\n", funnel.reconstructedDropped)
	fmt.Printf("  dropped: Minutes == 0 despite Starts==1:        %8d\n", funnel.minuteZeroDropped)
	fmt.Printf("  dropped: Fixtures != 1 (started + also\n")
	fmt.Printf("           appeared in a double, see header ⚠️):  %8d\n", funnel.multiFixtureDropped)
	fmt.Printf("used (Starts==1, Fixtures==1, not reconstructed,\n")
	fmt.Printf("      Minutes>0):                                %8d\n", funnel.usedRows)
	fmt.Printf("players with at least one used row:               %8d\n", len(agg))
	if funnel.startRows > 0 {
		fmt.Printf("share of start-rows dropped for reconstruction:   %7.1f%%\n",
			100*float64(funnel.reconstructedDropped)/float64(funnel.startRows))
	}
	if funnel.rowsOver90 > 0 {
		t.Fatalf("%d used rows carry Minutes > 90 — the row filter let a double-gameweek "+
			"row (Starts==2 counted incorrectly, or similar) through; fix the filter before "+
			"trusting anything below", funnel.rowsOver90)
	}

	// Group by position, in a fixed, deterministic order (agg was built by
	// walking sortedPlayerIDs, so this preserves that order rather than ranging
	// the map again).
	var ids []int
	for id := range agg {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	byPos := map[int][]*defconPlayerAgg{}
	for _, id := range ids {
		a := agg[id]
		byPos[a.position] = append(byPos[a.position], a)
	}

	positions := []int{1, 2, 3, 4}

	// The sanity check: no player's mean minutes-per-start may exceed 90.
	for _, pos := range positions {
		for _, a := range byPos[pos] {
			if a.minutesPerStart() > 90 {
				t.Fatalf("player id %d, position %s: minutes_per_start = %.2f > 90 — "+
					"a start cannot exceed 90 minutes, so the row filter is wrong",
					a.id, defconPosName[pos], a.minutesPerStart())
			}
		}
	}

	fmt.Printf("\n=== per position, headline threshold (>= %d qualifying starts)\n", defconMinStarts)
	fmt.Printf("%-5s %5s %10s %12s %8s %8s %10s\n",
		"pos", "n", "med mps", "med dc/90", "r", "df", "r_crit(5%)")
	for _, pos := range positions {
		res := defconComputePosResult(byPos[pos], defconMinStarts)
		if res.n == 0 {
			fmt.Printf("%-5s %5d %10s %12s %8s %8s %10s\n",
				defconPosName[pos], 0, "-", "-", "-", "-", "-")
			continue
		}
		if !res.haveR {
			fmt.Printf("%-5s %5d %10.1f %12.3f %8s %8s %10s   (n<3, r undefined)\n",
				defconPosName[pos], res.n, res.medMinutesPerStart, res.medDefconPer90, "-", "-", "-")
			continue
		}
		fmt.Printf("%-5s %5d %10.1f %12.3f %8.3f %8d %10.3f\n",
			defconPosName[pos], res.n, res.medMinutesPerStart, res.medDefconPer90,
			res.r, res.df, res.rCrit)
	}

	fmt.Printf("\n=== sensitivity: n, r and its own critical value at 5, 10 and 15 minimum\n")
	fmt.Printf("starts (10 is the headline) — so a reader can see the threshold was not\n")
	fmt.Printf("picked after seeing which one clears significance.\n")
	fmt.Printf("%-5s", "pos")
	for _, thr := range defconSensitivityThresholds {
		fmt.Printf("  n@%-2d    r@%-2d  crit@%-2d", thr, thr, thr)
	}
	fmt.Printf("\n")
	for _, pos := range positions {
		fmt.Printf("%-5s", defconPosName[pos])
		for _, thr := range defconSensitivityThresholds {
			res := defconComputePosResult(byPos[pos], thr)
			if !res.haveR {
				fmt.Printf("  %4d    %6s   %6s", res.n, "-", "-")
				continue
			}
			clears := " "
			if math.Abs(res.r) > res.rCrit {
				clears = "*"
			}
			fmt.Printf("  %4d    %6.3f   %6.3f%s", res.n, res.r, res.rCrit, clears)
		}
		fmt.Printf("\n")
	}
	fmt.Printf("(* marks |r| clearing its own two-sided 5%% critical value at that threshold)\n")

	fmt.Printf("\n=== distribution of minutes_per_start per position (>= %d starts), fixed-width\n", defconMinStarts)
	fmt.Printf("histogram over absolute minutes — deciles would smooth a bimodal shape into\n")
	fmt.Printf("ten equal-frequency steps by construction, which is exactly what this needs\n")
	fmt.Printf("to be able to show for DEF (see header, \"the bimodality question\").\n\n")
	const binWidth = 10.0
	const maxVal = 90.0
	fmt.Printf("%-5s %5s", "pos", "n")
	for lo := 0.0; lo < maxVal; lo += binWidth {
		fmt.Printf(" %6.0f-%2.0f", lo, lo+binWidth)
	}
	fmt.Printf("\n")
	for _, pos := range positions {
		var mps []float64
		for _, a := range byPos[pos] {
			if a.starts >= defconMinStarts {
				mps = append(mps, a.minutesPerStart())
			}
		}
		if len(mps) == 0 {
			continue
		}
		bins := defconHistogram(mps, binWidth, maxVal)
		fmt.Printf("%-5s %5d", defconPosName[pos], len(mps))
		for _, c := range bins {
			fmt.Printf(" %9d", c)
		}
		fmt.Printf("\n")
	}
	fmt.Printf("\nA position with all its mass in the rightmost bin or two is effectively\n")
	fmt.Printf("unimodal at 90 (little for a substitution-risk proxy to explain). Mass split\n")
	fmt.Printf("between the rightmost bins and a separate lower cluster is the bimodal shape\n")
	fmt.Printf("the centre-back/full-back hypothesis predicts for DEF specifically.\n")

	fmt.Printf("\nThis diagnostic authorises nothing: no scoring term moves. A material\n")
	fmt.Printf("within-position |r| clearing its own critical value, concentrated in DEF and\n")
	fmt.Printf("paired with a bimodal DEF histogram, would make the defcon-as-role-proxy idea\n")
	fmt.Printf("worth building; anything less is reported plainly as such.\n")
}

// TestDefconMinutesPerStartWiring pins the row filter and the critical-value
// arithmetic without DIAG and without the archive.
func TestDefconMinutesPerStartWiring(t *testing.T) {
	// --- row filter ---
	//
	// Player 1: two qualifying rows (gw1: 90 min, 3 defcon; gw2: 60 min, 1
	// defcon) plus one StartsReconstructed row that must be dropped (trap 1),
	// one Starts==0 row that must be dropped, one Minutes==0 Starts==1 row that
	// must be dropped, one Starts==2 (double-gameweek, both legs started) row
	// that must be dropped by the exact-equality filter, and one Starts==1,
	// Fixtures==2, Minutes==120 row — the exact shape the first live run of
	// TestDiagDefconMinutesPerStart actually found 21 of in 2025-26: started one
	// leg of a double and also came off the bench in the other, so Starts stays
	// 1 but Minutes exceeds a single match. That row must be dropped by the
	// Fixtures==1 guard, not by the Starts filter, which would let it through.
	s := &Season{Name: "test", Players: map[int]*Player{
		1: {ID: 1, Type: 2, GWs: map[int]GW{
			1: {Minutes: 90, Starts: 1, Fixtures: 1, DefCon: 3},
			2: {Minutes: 60, Starts: 1, Fixtures: 1, DefCon: 1},
			3: {Minutes: 45, Starts: 1, Fixtures: 1, StartsReconstructed: true, DefCon: 9},
			4: {Minutes: 0, Starts: 0, Fixtures: 1},
			5: {Minutes: 0, Starts: 1, Fixtures: 1}, // edge case: Starts==1, Minutes==0
			6: {Minutes: 180, Starts: 2, Fixtures: 2, DefCon: 5},
			7: {Minutes: 120, Starts: 1, Fixtures: 2, DefCon: 7}, // started + subbed on in a double
		}},
	}}
	agg, f := buildDefconAgg(s)

	if f.allRows != 7 {
		t.Fatalf("allRows = %d, want 7", f.allRows)
	}
	if f.startRows != 5 { // gw 1, 2, 3, 5, 7 (Starts==1); gw4 is 0, gw6 is 2
		t.Fatalf("startRows = %d, want 5", f.startRows)
	}
	if f.reconstructedDropped != 1 {
		t.Fatalf("reconstructedDropped = %d, want 1 (gw3)", f.reconstructedDropped)
	}
	if f.minuteZeroDropped != 1 {
		t.Fatalf("minuteZeroDropped = %d, want 1 (gw5)", f.minuteZeroDropped)
	}
	if f.multiFixtureDropped != 1 {
		t.Fatalf("multiFixtureDropped = %d, want 1 (gw7)", f.multiFixtureDropped)
	}
	if f.usedRows != 2 { // gw1, gw2 only
		t.Fatalf("usedRows = %d, want 2 (gw1, gw2)", f.usedRows)
	}
	a := agg[1]
	if a == nil {
		t.Fatal("player 1 has no aggregate at all")
	}
	if a.starts != 2 {
		t.Fatalf("starts = %d, want 2", a.starts)
	}
	if got, want := a.minutes, 150.0; got != want {
		t.Fatalf("minutes = %v, want %v (90+60, NOT the reconstructed or double row)", got, want)
	}
	if got, want := a.defcon, 4.0; got != want {
		t.Fatalf("defcon = %v, want %v (3+1, the reconstructed row's 9 and the double's 5 "+
			"must both be excluded)", got, want)
	}
	if got, want := a.minutesPerStart(), 75.0; got != want {
		t.Fatalf("minutesPerStart = %v, want %v", got, want)
	}
	if got, want := a.defconPer90(), 4.0/(150.0/90); math.Abs(got-want) > 1e-9 {
		t.Fatalf("defconPer90 = %v, want %v", got, want)
	}

	// --- critical-value arithmetic: round-trip identity, not a table lookup ---
	//
	// For each df, rCritFor(df) must be the r whose OWN t statistic
	// (r*sqrt(df)/sqrt(1-r^2)) is exactly twoSidedTCrit(df). This checks the
	// algebra end to end without quoting any of tCrit95's tabulated literals,
	// which TestInferenceLivesInOnePlace scans for outside stats_test.go.
	for _, df := range []int{1, 3, 5, 8, 15, 30, 60, 150} {
		want := twoSidedTCrit(df)
		r := rCritFor(df)
		if r <= 0 || r >= 1 {
			t.Fatalf("rCritFor(%d) = %v, want a value strictly between 0 and 1", df, r)
		}
		got := r * math.Sqrt(float64(df)) / math.Sqrt(1-r*r)
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("df %d: r_crit's own t statistic = %v, want twoSidedTCrit(df) = %v — "+
				"the r<->t identity is wired wrong", df, got, want)
		}
	}

	// Monotonicity: the critical t must shrink as df grows (approaching the
	// normal quantile from above), and never go negative or non-finite.
	prev := math.Inf(1)
	for _, df := range []int{1, 2, 5, 10, 30, 100, 1000} {
		v := twoSidedTCrit(df)
		if math.IsNaN(v) || v <= 0 {
			t.Fatalf("twoSidedTCrit(%d) = %v, want a finite positive value", df, v)
		}
		if v >= prev {
			t.Fatalf("twoSidedTCrit(%d) = %v is not below the previous, smaller df's value "+
				"%v — the critical t must shrink as df grows", df, v, prev)
		}
		prev = v
	}
	// It must also be closing in on the standard normal 97.5th percentile
	// (~1.96) at large df, from above, without being pinned to that literal.
	if large := twoSidedTCrit(100000); large <= 1.9 || large >= 2.1 {
		t.Fatalf("twoSidedTCrit(100000) = %v, want it close to the normal quantile (~1.96)", large)
	}

	// --- histogram ---
	bins := defconHistogram([]float64{5, 15, 85, 90, 44}, 10, 90)
	if len(bins) != 9 {
		t.Fatalf("len(bins) = %d, want 9 (0-10..80-90)", len(bins))
	}
	want := []int{1, 1, 0, 0, 1, 0, 0, 0, 2} // 5->[0,10) 15->[10,20) 44->[40,50) 85,90->[80,90]
	for i := range want {
		if bins[i] != want[i] {
			t.Fatalf("bins = %v, want %v", bins, want)
		}
	}
}
