package backtest

// Do a club's players' expected goals add up to the club?
//
//	DIAG=1 go test ./internal/backtest -run TestDiagTeamGoalShare -v -timeout 60m
//
// # What this gates
//
// The expected-points review's Gap 3 — "player attacking rates are anchored to
// nothing" — is the largest modelling idea on that list. AIrsenal estimates each
// player's probability of being the scorer or assister of *a goal his team scores*,
// so its estimand is a share of a team total by construction. We multiply each
// player's own blended rate by a league-wide per-position calibration ratio and
// then by a fixture multiplier, and nothing constrains a club's players' expected
// goals to sum to anything in particular.
//
// The review gates the whole item on one free out-of-sample check rather than on a
// replay, and this is it: **sum the model's per-player expected goals for a club
// and see whether the total is right, club by club.** If every club lands close,
// the anchor has nothing to correct and the item drops cheaply. If some clubs are
// systematically far out, that is a bias the replay would struggle to see — a
// detection threshold belongs to a *comparison* rather than to the harness, and the
// median across 23 real ones is 39 points a season spanning 3.9 to 232 — and this
// would move every transfer involving those clubs.
//
// # Why this is the redesigned version and not the one the review first proposed
//
// The original diagnostic was "sum the model's per-player expected goals per
// team-match and compare to the club's actual goals". That is wrong twice over, and
// both corrections are in the gap's own warning block.
//
//   - **Against actual goals it largely measures finishing.** A club out-scoring
//     its expected goals is real football, not model error, and it would fire the
//     gate on variance the anchor cannot fix.
//   - **Pooled, the ratio is ≈1 by construction.** `calibrateExpectedStats` already
//     forces the league-season identity per position — it scales expected goals to
//     awarded goals across the whole league each run — so the mean was never the
//     question. The **spread across clubs** is.
//
// So the primary target here is the club's realised **expected** goals over a
// held-out window, and realised goals are reported beside it as a contrast. The gap
// between the two columns is the finishing variance the original design would have
// mistaken for a finding, which is worth seeing rather than arguing about.
//
// # The design
//
// Fit at GW19 and score GW20-38. Out of sample matters here for the same reason it
// does everywhere else in this package: `calibrateExpectedStats` runs on the season
// to date, so a club's modelled total scored against the same window it was
// calibrated on would be partly fitted to its own answer.
//
// Per club, the modelled figure is
//
//	sum over the club's players of  XG90 x XGScale x ExpectedMinutes / 90
//
// which is expected *awarded* goals per match — every factor already exposed on
// PlayerMetrics, because every scoring term in this project is a reported
// multiplier. Summing across positions with different `XGScale` values is exactly
// right for this question: the identity being tested is about the club, and FPL
// pays a defender's goal more than a forward's but counts it once.
//
// Expected minutes per club per match is printed beside it and is the more
// diagnostic of the two. Eleven players are on the pitch for ninety minutes, so a
// club's expected minutes must come to about 990 a match. It is a pure arithmetic
// check with no football in it, and a club systematically over or under says the
// share problem is in the *minutes* rather than in the rates — a different repair
// entirely, and a much cheaper one.
//
// # The decision rule, committed before the numbers were seen
//
// This project's own discipline: an argmax over noisy arms manufactures effects,
// and a threshold chosen after the fact is the same failure wearing a different
// hat. So, on the expected-goals column:
//
//   - Spread across clubs **under about 10%** (sd of the ratio): Gap 3 drops. There
//     is no between-club bias for a normaliser to remove, and the recorded warning
//     that renormalising reorders the whole pool — nothing forces you to own players
//     from any particular club — makes it a change with risk and no measured upside.
//   - **A material tail beyond ±30%**, which is the figure the gap itself names:
//     the item stays live and the next step is the anchor, built as
//     renormalise-within-club-then-multiply so within-club ordering is untouched.
//   - Between the two: unresolved, and it should be recorded as unresolved rather
//     than as either verdict. That is the expected reading, and it is not evidence
//     against an effect.
//
// Note what this diagnostic cannot say even at its loudest. A between-club level
// error is a real bias, and correcting a measured bias has lost this project points
// five times. It gates whether the idea is worth building; it does not license
// shipping one.
//
// # On reproducibility
//
// Every reduction here is an accumulation — sums into a per-club record — and
// addition commutes, so map order cannot change a figure. The one place it could is
// the printed tail, which *chooses* rows, so the clubs are sorted by id before
// anything is appended and by ratio before anything is printed. That is the
// clean-sheet diagnostic's recorded failure avoided rather than repeated; see
// TestModelDiagnosticsAreReproducible.

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// shareCutoff is the gameweek the model is built through, and shareFrom the first
// gameweek scored. They are named rather than inlined because the out-of-sample
// split IS the design: moving the cutoff changes what the diagnostic measures, and
// a reader should not have to find two integers in a loop to see that.
const (
	shareCutoff = 19
	shareFrom   = 20
	// shareMid splits the held-out window in two, so the first stretch can be used
	// to predict the second. It is the midpoint rather than a tuned value: picking
	// the split that gave the best answer would be an argmax over the thing being
	// measured.
	shareMid = 28
)

// shareClub is one club in one season: what the model said at the cutoff, and what
// the club went on to do.
type shareClub struct {
	name string
	// Modelled, at the cutoff: expected goals and expected minutes per match.
	modelGoals, modelMinutes float64
	// Realised, over the held-out window. actGoals and actXG are totals; matches is
	// the denominator that turns them into per-match figures.
	actGoals, actXG, matches float64
	// The held-out window split into two halves of the club's own matches, which is
	// the noise control. See the split-half section: it is measured without the
	// model, so it says how much of the spread above is the denominator rather than
	// the numerator.
	xgByGW      map[int]float64
	matchesByGW map[int]float64
	// The club's own fixture difficulty over the held-out window, from FPL's 1-5
	// rating on the side the club is actually on. Kept per gameweek as well as
	// pooled, so a sub-window can be adjusted for the fixtures it actually
	// contained rather than for the window's average.
	diffSum, diffN float64
	easeByGW       map[int]float64
	easeNByGW      map[int]float64
}

// meanEase is the club's mean attacking fixture multiplier over a gameweek range,
// on the shipped ladder — above 1 for an easy run and below for a hard one.
//
// It reads `analysis.AttackMultiplier` rather than restating the ladder, so this
// asks whether the model's OWN fixture response explains the club-level signal. A
// local copy of the numbers would be asking about a ladder nothing ships.
func (c shareClub) meanEase(lo, hi int) float64 {
	var sum, n float64
	for gw, s := range c.easeByGW {
		if gw >= lo && gw <= hi {
			sum += s
			n += c.easeNByGW[gw]
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

// windowXG is the club's realised expected goals per match over a gameweek range,
// and false when the club played too few matches in it to mean anything.
func (c shareClub) windowXG(lo, hi int) (float64, bool) {
	var xg, m float64
	for gw, n := range c.matchesByGW {
		if gw >= lo && gw <= hi {
			m += n
			xg += c.xgByGW[gw]
		}
	}
	if m < 4 || xg <= 0 {
		return 0, false
	}
	return xg / m, true
}

// meanDifficulty is the club's average FPL fixture difficulty over the window.
func (c shareClub) meanDifficulty() float64 {
	if c.diffN == 0 {
		return 0
	}
	return c.diffSum / c.diffN
}

// splitHalf is the club's expected goals per match in the first and second halves
// of the held-out window, split at the midpoint of its own matches so a club with a
// postponement is still cut in two even parts.
//
// The second result is false when the club does not have at least three matches on
// each side, where a ratio of two tiny samples is noise about noise.
func (c shareClub) splitHalf() (first, second float64, ok bool) {
	var gws []int
	for gw := range c.matchesByGW {
		gws = append(gws, gw)
	}
	sort.Ints(gws)

	half := c.matches / 2
	var seen, fx, fm, sx, sm float64
	for _, gw := range gws {
		if seen < half {
			fx += c.xgByGW[gw]
			fm += c.matchesByGW[gw]
		} else {
			sx += c.xgByGW[gw]
			sm += c.matchesByGW[gw]
		}
		seen += c.matchesByGW[gw]
	}
	if fm < 3 || sm < 3 {
		return 0, 0, false
	}
	return fx / fm, sx / sm, true
}

// realisedXG and realisedGoals are the two targets, per match. They are methods so
// the table below can name the column it wants without a closure over a struct
// literal repeated at every signature.
func (c shareClub) realisedXG() float64 {
	if c.matches <= 0 {
		return 0
	}
	return c.actXG / c.matches
}

func (c shareClub) realisedGoals() float64 {
	if c.matches <= 0 {
		return 0
	}
	return c.actGoals / c.matches
}

// ratioAgainst is modelled over realised, and 0 when there is nothing to divide by.
func (c shareClub) ratioAgainst(realised float64) float64 {
	if realised <= 0 {
		return 0
	}
	return c.modelGoals / realised
}

func TestDiagTeamGoalShare(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	sink := openModelSinkFor(t.Logf)
	defer sink.close()
	ctx := context.Background()

	// Coverage first, before any table. The archive's expected-goals series begins
	// at GW16 of 2022-23 and the repair fills the rest, so a GW19 cutoff in that
	// season rests almost entirely on reconstructed data — and a diagnostic that
	// silently ran on a season with three real gameweeks of xG behind its fit would
	// print a plausible table. This is the `starts` lesson applied before the fact
	// rather than after it.
	fmt.Printf("\n=== coverage: expected-goals rows behind each fit\n")
	fmt.Printf("%-10s %10s %12s %12s\n", "season", "cutoff", "xG rows <= cut", "repaired")

	var all []shareClub
	bySeason := map[string][]shareClub{}

	for _, pair := range sweepPairNames() {
		prior, err := Load(ctx, cfg.CacheDir, pair[0])
		if err != nil {
			t.Fatal(err)
		}
		cur, err := Load(ctx, cfg.CacheDir, pair[1])
		if err != nil {
			t.Fatal(err)
		}

		var xgRows float64
		for _, id := range sortedPlayerIDs(cur) {
			for gw, g := range cur.Players[id].GWs {
				if gw <= shareCutoff && g.XG > 0 {
					xgRows++
				}
			}
		}
		fmt.Printf("%-10s %10d %12.0f %12v\n", cur.Name, shareCutoff, xgRows,
			cur.XGRepair.Applied)

		boot, fx := PointInTime(cur, prior, shareCutoff)
		// Horizon 1 so ExpectedMinutes and the rates are read per match rather than
		// through a five-gameweek fixture average. The quantity here is "goals in a
		// match", so a horizon-averaged view would be answering a different question
		// — the same reason the prediction benchmark pins horizon 1.
		w := cfg.Weights
		w.Horizon = 1
		e := analysis.NewEngineFull(boot, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
		e.Priors = newPriorIndex(prior)
		e.Recent = newRecentIndexWith(cur, shareCutoff, w.MinutesHalfLife, w.RateHalfLife)

		byTeam := map[int]*shareClub{}
		get := func(id int) *shareClub {
			if c := byTeam[id]; c != nil {
				return c
			}
			c := &shareClub{
				name:        teamShortName(cur.Teams, id),
				xgByGW:      map[int]float64{},
				matchesByGW: map[int]float64{},
				easeByGW:    map[int]float64{},
				easeNByGW:   map[int]float64{},
			}
			byTeam[id] = c
			return c
		}

		// Modelled, at the cutoff. boot.Elements is a slice, so this is ordered.
		//
		// PointInTime filters through registeredBy, so this is the players in the
		// game at the cutoff — which is the population a manager could actually pick
		// from, and therefore the right numerator.
		registered := map[int]bool{}
		for i := range boot.Elements {
			el := &boot.Elements[i]
			m := e.Metrics(el)
			c := get(el.Team)
			c.modelGoals += m.XG90 * m.XGScale * m.ExpectedMinutes / 90
			c.modelMinutes += m.ExpectedMinutes
			registered[el.ID] = true
		}

		// Realised, over the held-out window. Goals come from the fixture
		// scorelines, which are exact; a sum of the club's players' goals would
		// miss the ones an opponent scored into his own net. Expected goals have no
		// such source and are summed from the player rows, which is what team
		// expected goals is.
		for _, f := range cur.Fixtures {
			if f.Event == nil || *f.Event < shareFrom || *f.Event > 38 {
				continue
			}
			if f.TeamHScore == nil || f.TeamAScore == nil {
				continue // not played, or the archive did not record it
			}
			h, a := get(f.TeamH), get(f.TeamA)
			h.actGoals += float64(*f.TeamHScore)
			a.actGoals += float64(*f.TeamAScore)
			h.matches++
			a.matches++
			h.matchesByGW[*f.Event]++
			a.matchesByGW[*f.Event]++
			h.diffSum += float64(f.TeamHDifficulty)
			a.diffSum += float64(f.TeamADifficulty)
			h.diffN++
			a.diffN++
			h.easeByGW[*f.Event] += analysis.AttackMultiplier(f.TeamHDifficulty)
			a.easeByGW[*f.Event] += analysis.AttackMultiplier(f.TeamADifficulty)
			h.easeNByGW[*f.Event]++
			a.easeNByGW[*f.Event]++
		}
		// **The denominator must cover the same players as the numerator.** It did
		// not in the first version of this diagnostic, and the effect was large:
		// summing realised expected goals over every player in the season puts a
		// January arrival's output in the denominator with nothing above it, so a
		// club that bought in the window reads as though the model under-rated it.
		// Measured, unregistered players carry a mean 4.6% of a club's realised
		// GW20-38 expected goals and up to 30% for one club-season — an error of the
		// same order as the entire effect this diagnostic was built to size, and one
		// the split-half control cannot see, since both halves are realised data
		// containing the same arrivals.
		//
		// Excluding them is the right call rather than a convenience. A player who
		// was not in the game at GW19 is not something a GW19 model got wrong; he is
		// something no GW19 model could have known, and a static club-level anchor
		// fitted at GW19 would not fix him either. What it does mean is that the
		// held-out window is the club MINUS its winter business, which is worth
		// saying wherever the figure is quoted.
		var skipped, kept float64
		for _, id := range sortedPlayerIDs(cur) {
			p := cur.Players[id]
			c := get(p.Team)
			for gw, g := range p.GWs {
				if gw < shareFrom || gw > 38 {
					continue
				}
				if !registered[id] {
					skipped += g.XG
					continue
				}
				kept += g.XG
				c.actXG += g.XG
				c.xgByGW[gw] += g.XG
			}
		}
		if kept+skipped > 0 {
			fmt.Printf("%-10s %10s %12s %11.1f%%\n", cur.Name, "", "unregistered at cutoff:",
				100*skipped/(kept+skipped))
		}

		var ids []int
		for id := range byTeam {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			c := *byTeam[id]
			if c.matches < 10 {
				continue
			}
			bySeason[cur.Name] = append(bySeason[cur.Name], c)
			all = append(all, c)
		}
	}

	if len(all) < 40 {
		t.Skipf("only %d club-seasons", len(all))
	}

	// -----------------------------------------------------------------
	// Per season: the mean and, the thing this exists for, the spread.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== modelled club expected goals per match, against the held-out window\n")
	fmt.Printf("Model built through GW%d, scored on GW%d-38. Ratio is modelled over realised,\n",
		shareCutoff, shareFrom)
	fmt.Printf("so 1.000 is perfect. Read the sd and the tail, not the mean — the mean is\n")
	fmt.Printf("held near 1 by calibrateExpectedStats and was never the question.\n\n")
	fmt.Printf("%-10s %5s | %-30s | %-30s\n", "season", "clubs",
		"vs realised xG", "vs realised goals")
	fmt.Printf("%-10s %5s | %7s %7s %6s %6s | %7s %7s %6s %6s\n",
		"", "", "mean", "sd", "min", "max", "mean", "sd", "min", "max")

	var names []string
	for s := range bySeason {
		names = append(names, s)
	}
	sort.Strings(names)

	for _, s := range names {
		cs := bySeason[s]
		xm, xs, xlo, xhi := ratioStats(cs, shareClub.realisedXG)
		gm, gs, glo, ghi := ratioStats(cs, shareClub.realisedGoals)
		fmt.Printf("%-10s %5d | %7.3f %7.3f %6.2f %6.2f | %7.3f %7.3f %6.2f %6.2f\n",
			s, len(cs), xm, xs, xlo, xhi, gm, gs, glo, ghi)
		sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", s, len(cs),
			measure{"mean ratio against realised expected goals", xm},
			measure{"spread across clubs, sd of that ratio", xs},
			measure{"lowest club", xlo},
			measure{"highest club", xhi})
	}
	xm, xs, xlo, xhi := ratioStats(all, shareClub.realisedXG)
	gm, gs, glo, ghi := ratioStats(all, shareClub.realisedGoals)
	fmt.Printf("%-10s %5d | %7.3f %7.3f %6.2f %6.2f | %7.3f %7.3f %6.2f %6.2f\n",
		"POOLED", len(all), xm, xs, xlo, xhi, gm, gs, glo, ghi)

	// The tail the gap's own wording names.
	var beyond30 int
	for _, c := range all {
		if r := c.ratioAgainst(c.realisedXG()); r > 0 && math.Abs(r-1) > 0.30 {
			beyond30++
		}
	}
	fmt.Printf("\n%d of %d club-seasons are more than 30%% out against realised expected goals\n",
		beyond30, len(all))
	sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "pooled", len(all),
		measure{"mean ratio against realised expected goals", xm},
		measure{"spread across clubs, sd of that ratio", xs},
		measure{"club-seasons more than 30% out", float64(beyond30)},
		measure{"mean ratio against realised goals", gm},
		measure{"spread across clubs against realised goals", gs})

	// -----------------------------------------------------------------
	// The noise control, without which the spread above means nothing.
	// -----------------------------------------------------------------
	//
	// The ratio's denominator is a club's expected goals over about nineteen
	// matches, which is a *sample*, and it carries its own sampling error. Some of
	// the spread above is therefore the denominator wobbling rather than the model
	// being wrong about the club, and a decision rule read off the raw figure would
	// convict the model of the archive's variance.
	//
	// This measures that floor with no model in it at all: split each club's
	// held-out window into two halves of its own matches and take the ratio of the
	// two. A club is being compared with *itself*, so every point of spread here is
	// sampling error plus genuine within-season change, and neither is anything an
	// anchor could fix.
	//
	// Converting to the scale of the table above: the two halves are each about
	// half the window, so the variance of the split-half log ratio is about four
	// times the variance of a full-window mean — two independent halves, each with
	// twice the variance of the whole. So the denominator's contribution is roughly
	// the split-half sd divided by two, and what is left after removing it in
	// quadrature is the most the model can be held responsible for.
	//
	// **The pre-committed rule is unchanged and this does not move it.** It was
	// written about how far the model is from the clubs; this is what makes the
	// printed number that quantity rather than that quantity plus the archive's own
	// noise. Applying a threshold to a figure known to contain a large term the
	// threshold was not meant to cover would be the mistake, not correcting for it.
	// **The modelled figure above carries no fixture term, and that inflates the
	// spread.** `modelGoals` sums XG90 x XGScale x ExpectedMinutes/90, and the
	// attacking multiplier is applied downstream in fixtureSensitiveAt, never in
	// that sum — so a fixture-blind prediction is being compared against output
	// earned against real opponents. Some of the spread is therefore the fixture
	// list rather than the model, and any correlation between the ratio and
	// difficulty is partly mechanical.
	//
	// Putting the realised side on neutral terms too, using the model's own ladder,
	// is the like-for-like comparison. The gap between the two figures is what the
	// fixture list was contributing to a number this diagnostic reports as model
	// error.
	var neutral []shareClub
	for _, c := range all {
		e := c.meanEase(shareFrom, 38)
		if e <= 0 {
			continue
		}
		adj := c
		adj.actXG = c.actXG / e
		neutral = append(neutral, adj)
	}
	if len(neutral) > 2 {
		_, nsd, _, _ := ratioStats(neutral, shareClub.realisedXG)
		fmt.Printf("\nspread against realised xG                        %.3f\n", xs)
		fmt.Printf("spread with BOTH sides at neutral difficulty      %.3f\n", nsd)
		fmt.Printf("\nThe modelled figure carries no fixture term, so the first number\n")
		fmt.Printf("includes the fixture list. The second is the like-for-like one.\n")
		sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "fixture-neutral spread",
			len(neutral), measure{"spread across clubs at neutral difficulty", nsd})
	}

	fmt.Printf("\n=== the noise floor: each club against ITSELF, first half against second\n")
	denom := splitHalfFloor(all)
	fmt.Printf("pooled: %d club-seasons, implied sampling sd of the denominator %.3f\n",
		len(all), denom)

	// **Pooled is the wrong level to judge this at, and the pooled figure conceals
	// the result.** Two things are mixed into the 0.222 above. One is the spread of
	// clubs *within* a season, which is what a between-club anchor addresses. The
	// other is the season means moving as a block — 0.921 to 1.118 across four
	// seasons — which is `calibrateExpectedStats` not landing on 1.000 out of
	// sample. That is a league-wide level error, identical for every club in a
	// season, and no per-club normaliser touches it. Pooling them inflates the
	// headline with a term the proposed fix cannot reach.
	//
	// So decompose, and then judge season by season, because four seasons is the
	// unit this project's inference works in and three degrees of freedom is what it
	// has. A figure that survives pooling and dies on clustering is the failure this
	// file's canonical resolution block exists to prevent.
	var withinVar, meanOfMeans float64
	for _, s := range names {
		m, sd, _, _ := ratioStats(bySeason[s], shareClub.realisedXG)
		withinVar += sd * sd / float64(len(names))
		meanOfMeans += m / float64(len(names))
	}
	var betweenVar float64
	for _, s := range names {
		m, _, _, _ := ratioStats(bySeason[s], shareClub.realisedXG)
		betweenVar += (m - meanOfMeans) * (m - meanOfMeans) / float64(len(names))
	}
	fmt.Printf("\npooled spread %.3f decomposes into within-season %.3f and a\n", xs,
		math.Sqrt(withinVar))
	fmt.Printf("between-SEASON level shift of %.3f (%.0f%% of the variance), which is a\n",
		math.Sqrt(betweenVar), 100*betweenVar/(withinVar+betweenVar))
	fmt.Printf("league-wide calibration term no per-club anchor addresses.\n")

	fmt.Printf("\n%-10s %8s %8s %9s\n", "season", "sd", "floor", "excess")
	var exs []float64
	for _, s := range names {
		_, sd, _, _ := ratioStats(bySeason[s], shareClub.realisedXG)
		f := splitHalfFloor(bySeason[s])
		ex := math.Sqrt(math.Max(0, sd*sd-f*f))
		exs = append(exs, ex)
		fmt.Printf("%-10s %8.3f %8.3f %9.3f\n", s, sd, f, ex)
	}
	exMean, exSE := meanAndSE(exs)
	fmt.Printf("%-10s %8s %8s %9.3f   SE %.3f, t = %.2f at df %d\n", "MEAN", "", "",
		exMean, exSE, exMean/exSE, len(exs)-1)
	// tCrit95 has no df 0 and would panic on one season; it cannot get one. The
	// `len(all) < 40` skip above needs two seasons to clear, since a season
	// contributes at most twenty clubs.
	fmt.Printf("\nAt df %d the two-sided 5%% critical t is %.3f, which this project's canonical\n",
		len(exs)-1, tCrit95(len(exs)-1))
	fmt.Printf("block records and which only more SEASONS can improve on — the df is a\n")
	fmt.Printf("season count minus one, and no number of entry points moves it.\n")

	// The tail needs a null too. "11 of 80 beyond 30%" is quoted against an implicit
	// baseline of zero, and the baseline is not zero: with a denominator carrying a
	// sampling sd of `denom`, a model that was perfect about every club still puts
	// some clubs past the bar. Reported so the comparison is 11 against that number
	// rather than 11 against nothing.
	expected := expectedBeyond(xm, denom, 0.30) * float64(len(all))
	fmt.Printf("\n%d of %d club-seasons beyond 30%%, against %.1f expected from the\n",
		beyond30, len(all), expected)
	fmt.Printf("denominator's own sampling noise alone.\n")

	sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "noise floor", len(all),
		measure{"implied sampling sd of the denominator", denom},
		measure{"within-season spread across clubs", math.Sqrt(withinVar)},
		measure{"between-season level shift", math.Sqrt(betweenVar)},
		measure{"mean per-season excess over the floor", exMean},
		measure{"its standard error across seasons", exSE},
		measure{"club-seasons beyond 30% expected from noise alone", expected})

	// -----------------------------------------------------------------
	// Persistence, which is what "systematically out" has to mean.
	// -----------------------------------------------------------------
	//
	// This is the measurement the gap actually needs and the one the first version of
	// this diagnostic did not make. The wording being gated is that "some clubs are
	// **systematically** 30% out", and a spread on its own cannot say that: a club
	// drawn 30% high one season and 30% low the next is not systematically anything,
	// and no anchor fitted on history would help it.
	//
	// A static between-club level error — the thing the proposed normaliser removes —
	// **must** repeat for the same club across seasons. Unforecastable change must
	// not. So correlate each club's ratio in one season against its own ratio in the
	// next. It costs nothing: the data is already collected.
	//
	// Read it as the deciding column. A spread that does not persist is a spread the
	// anchor inherits rather than fixes.
	fmt.Printf("\n=== persistence: does a club that is out stay out?\n")
	byClub := map[string]map[string]float64{}
	for _, s := range names {
		for _, c := range bySeason[s] {
			if byClub[c.name] == nil {
				byClub[c.name] = map[string]float64{}
			}
			byClub[c.name][s] = c.ratioAgainst(c.realisedXG())
		}
	}
	var xs1, ys1 []float64
	for i := 0; i+1 < len(names); i++ {
		a, b := names[i], names[i+1]
		var clubs []string
		for name := range byClub {
			clubs = append(clubs, name)
		}
		sort.Strings(clubs)
		for _, name := range clubs {
			u, okA := byClub[name][a]
			v, okB := byClub[name][b]
			if okA && okB && u > 0 && v > 0 {
				xs1 = append(xs1, u)
				ys1 = append(ys1, v)
			}
		}
	}
	r := correlation(xs1, ys1)
	fmt.Printf("%d consecutive-season club pairs, correlation of the ratio with itself: %+.3f\n",
		len(xs1), r)
	fmt.Printf("\nA static between-club level error MUST persist for the same club; only the\n")
	fmt.Printf("persistent part is what an anchor fitted on history could remove. Near zero\n")
	fmt.Printf("means the spread is real and still not addressable this way.\n")
	sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "persistence", len(xs1),
		measure{"correlation of a club's ratio with its own next season", r})

	// -----------------------------------------------------------------
	// What CLOCK does the club-level error run on?
	// -----------------------------------------------------------------
	//
	// The persistence result above measures one lag — a whole season — because that
	// is the lag the proposed fix operates at: learn a fixed per-club offset from
	// history and apply it forward. It refutes *that*. It says nothing about
	// structure with a shorter half-life, and the negative sign is a hint there is
	// some: independent noise gives zero correlation, and something that reverts
	// gives what was measured.
	//
	// The mechanism worth suspecting is **lag rather than bias**. A club's estimate
	// is built from player rates that blend this season against a prior at a fixed
	// weight, so a side that changes character mid-season — a new manager, a signing,
	// a tactical shift — is tracked slowly by construction. That is not a fixed
	// offset an annual anchor would remove; it is a delay, and the repair for a delay
	// is a shorter memory, not a constant.
	//
	// The clean test is a forecasting one, and it avoids the trap the obvious version
	// falls into. Correlating a club's error in one block against the next block
	// **cannot work** while the model is held fixed: both ratios share the same
	// numerator, so a common term is being correlated with itself and the answer is
	// positive whatever the football does. So ask the question as prediction instead.
	// Predict each club's expected goals per match over the closing stretch, from:
	//
	//   - the model, built at the cutoff;
	//   - the club's own realised output over the preceding stretch, which is what a
	//     pure recency rule would use;
	//   - a geometric blend of the two, which IS the normaliser this gap proposes,
	//     with the blend weight saying how much club-level correction is worth.
	//
	// Scored as the root-mean-square of log(predicted / realised), so a ratio of 2x
	// and one of 0.5x count the same, which they should.
	fmt.Printf("\n=== what clock does the club-level error run on?\n")
	fmt.Printf("Predicting each club's GW%d-38 expected goals per match. `model` is built at\n",
		shareMid+1)
	fmt.Printf("GW%d; `recent` is the club's own GW%d-%d output, which is what a pure recency\n",
		shareCutoff, shareFrom, shareMid)
	fmt.Printf("rule would use; the blends are the normaliser this gap actually proposes.\n\n")

	type pred struct {
		model, recent, actual float64
	}
	var ps []pred
	for _, c := range all {
		recent, ok1 := c.windowXG(shareFrom, shareMid)
		actual, ok2 := c.windowXG(shareMid+1, 38)
		if !ok1 || !ok2 || c.modelGoals <= 0 {
			continue
		}
		ps = append(ps, pred{model: c.modelGoals, recent: recent, actual: actual})
	}

	fmt.Printf("%-34s %10s\n", "predictor", "rms log err")
	logRMS := func(f func(pred) float64) float64 {
		var ss float64
		for _, p := range ps {
			d := math.Log(f(p) / p.actual)
			ss += d * d
		}
		return math.Sqrt(ss / float64(len(ps)))
	}
	mOnly := logRMS(func(p pred) float64 { return p.model })
	rOnly := logRMS(func(p pred) float64 { return p.recent })
	fmt.Printf("%-34s %10.4f\n", "the model alone", mOnly)
	fmt.Printf("%-34s %10.4f\n", "the club's own recent output alone", rOnly)
	best, bestW := mOnly, 0.0
	for _, w := range []float64{0.25, 0.5, 0.75} {
		v := logRMS(func(p pred) float64 {
			return math.Pow(p.model, 1-w) * math.Pow(p.recent, w)
		})
		fmt.Printf("%-34s %10.4f\n",
			fmt.Sprintf("blended, weight %.2f on recent", w), v)
		if v < best {
			best, bestW = v, w
		}
	}
	fmt.Printf("\n%d club-windows. If `recent` beats `model`, club-level recency carries\n", len(ps))
	fmt.Printf("signal the model is not using and the error has a SHORT clock. If the model\n")
	fmt.Printf("wins and every blend is worse, there is nothing at this timescale either and\n")
	fmt.Printf("the anchor is closed at both ends rather than only at the annual one.\n")
	if bestW > 0 {
		fmt.Printf("\nbest blend: weight %.2f on recent, %.1f%% better than the model alone\n",
			bestW, 100*(1-best/mOnly))
	} else {
		fmt.Printf("\nNo blend beats the model alone.\n")
	}

	// Pooled is not a verdict here any more than anywhere else in this file. The
	// season is the clustering level, and a 13% pooled gain carried by one season is
	// a different object from one that shows up in all of them. Printed per season so
	// the reader can see which it is rather than take the mean on trust.
	fmt.Printf("\n%-10s %8s %8s %9s %9s\n", "season", "n", "model", "blend .50", "better by")
	var gains []float64
	// The season behind each gain, kept in step with it: a season that fails the
	// club-window filter below is skipped, so gains is not indexed by names and
	// the concentration line under the table has to name a season.
	var gainSeasons []string
	for _, s := range names {
		var sp []pred
		for _, c := range bySeason[s] {
			recent, ok1 := c.windowXG(shareFrom, shareMid)
			actual, ok2 := c.windowXG(shareMid+1, 38)
			if !ok1 || !ok2 || c.modelGoals <= 0 {
				continue
			}
			sp = append(sp, pred{model: c.modelGoals, recent: recent, actual: actual})
		}
		if len(sp) < 5 {
			continue
		}
		rms := func(f func(pred) float64) float64 {
			var ss float64
			for _, p := range sp {
				d := math.Log(f(p) / p.actual)
				ss += d * d
			}
			return math.Sqrt(ss / float64(len(sp)))
		}
		m := rms(func(p pred) float64 { return p.model })
		b := rms(func(p pred) float64 {
			return math.Sqrt(p.model * p.recent)
		})
		gains = append(gains, 100*(1-b/m))
		gainSeasons = append(gainSeasons, s)
		fmt.Printf("%-10s %8d %8.4f %9.4f %8.1f%%\n", s, len(sp), m, b, 100*(1-b/m))
	}
	if len(gains) < 2 {
		// One season cannot carry a cluster mean, an SE or a critical value, and
		// each of the three below would answer anyway: meanAndSE returns zeros,
		// gMean/gSE is Inf, and tCrit95 has no df 0 to look up.
		t.Skipf("only %d season(s) cleared the club-window filter; there is no "+
			"between-season spread to report", len(gains))
	}
	gMean, gSE := meanAndSE(gains)
	fmt.Printf("%-10s %8s %8s %9s %8.1f%%   SE %.1f, t = %.2f at df %d\n",
		"MEAN", "", "", "", gMean, gSE, gMean/gSE, len(gains)-1)
	// The original line read "a pooled gain carried by two of four seasons", and
	// **agreement and concentration are two different statistics**. Counting
	// positives answers the first and not the second: four positive seasons where
	// one is 94% of the mean is exactly the shape this project's bench-slot
	// retraction was built on. So both are printed, each under its own word, and
	// concentration is the leave-one-out the record already argues in.
	positive := 0
	for _, g := range gains {
		if g > 0 {
			positive++
		}
	}
	// The season that moves the mean most, and what the mean is without it.
	worst, loo, moved := 0, gMean, -1.0
	for i := range gains {
		var rest []float64
		for j, g := range gains {
			if j != i {
				rest = append(rest, g)
			}
		}
		m := meanOf(rest)
		if d := math.Abs(m - gMean); d > moved {
			worst, loo, moved = i, m, d
		}
	}
	fmt.Printf("\nAgainst the %.3f that %d clusters demand. The gain is POSITIVE in %d of\n",
		tCrit95(len(gains)-1), len(gains), positive)
	fmt.Printf("%d seasons, which is agreement and not size; for concentration, dropping\n", len(gains))
	fmt.Printf("%s alone takes the pooled %.1f%% to %.1f%%. Both are needed: seasons\n",
		gainSeasons[worst], gMean, loo)
	fmt.Printf("agreeing on a sign while one carries the magnitude is not a result. On the\n")
	fmt.Printf("four-season run the two positive seasons were the same pair carrying the\n")
	fmt.Printf("spread above, one of which is the season whose first fifteen gameweeks of\n")
	fmt.Printf("expected goals are an Understat backfill.\n")
	sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "error timescale, clustered",
		len(gains),
		measure{"mean per-season gain from the 0.50 blend, percent", gMean},
		measure{"its standard error across seasons", gSE})
	sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "error timescale", len(ps),
		measure{"rms log error, model alone", mOnly},
		measure{"rms log error, club's own recent output", rOnly},
		measure{"best blend weight on recent", bestW})

	// Fixture runs are a separate hypothesis with a separate signature. The model
	// already applies a per-fixture difficulty multiplier; if it applies too little
	// of one, a club with an easy run out-scores its estimate and the ratio moves
	// with the run. Cheap to check and it distinguishes "the rates are wrong" from
	// "the fixture response is mis-scaled at club level".
	var ds, rs []float64
	for _, c := range all {
		if r := c.ratioAgainst(c.realisedXG()); r > 0 && c.diffN > 0 {
			ds = append(ds, c.meanDifficulty())
			rs = append(rs, r)
		}
	}
	fmt.Printf("\n=== is it fixture runs?\n")
	fmt.Printf("%d clubs: correlation of the ratio with mean fixture difficulty over the\n", len(ds))
	fmt.Printf("scored window: %+.3f\n", correlation(ds, rs))
	fmt.Printf("\nThe model already scales by difficulty per fixture. A strong correlation here\n")
	fmt.Printf("would say it scales by too LITTLE and the club-level error is really the\n")
	fmt.Printf("fixture response; near zero says the two are separate problems.\n")
	sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "fixture runs", len(ds),
		measure{"correlation of the ratio with mean fixture difficulty", correlation(ds, rs)})

	// -----------------------------------------------------------------
	// Is "form" just the fixtures it was earned against?
	// -----------------------------------------------------------------
	//
	// The blend above says a club's recent output predicts its next stretch. That
	// leaves the question of *what* recent output is measuring, and there are two
	// candidates with different consequences.
	//
	//   - The club changed — a manager, a signing, a shift in how it plays. That
	//     persists into the next stretch and is worth tracking.
	//   - The club just played the three worst defences in the league. That does
	//     NOT persist, because fixtures revert by construction: everyone plays
	//     everyone. Worse, the model already applies a per-fixture difficulty
	//     multiplier, so crediting it again as "form" would double-count it.
	//
	// The two are separable and the test is direct. Adjust each club's recent
	// output for the difficulty of the fixtures it actually faced, and see what
	// that does to its predictive power. **If form is really fixtures, adjusting
	// should destroy it. If form is real team change, adjusting should sharpen
	// it** — because the fixture component is noise for this purpose and removing
	// noise from a predictor helps.
	//
	// The adjustment is the model's own attacking ladder, so this asks whether the
	// model's existing fixture response already explains the club-level signal
	// rather than inventing a second scale to argue about.
	fmt.Printf("\n=== is club form just the fixtures it was earned against?\n")

	var rawErr, adjErr, n2 float64
	var formVals, diffVals []float64
	for _, c := range all {
		recent, ok1 := c.windowXG(shareFrom, shareMid)
		actual, ok2 := c.windowXG(shareMid+1, 38)
		if !ok1 || !ok2 || c.modelGoals <= 0 {
			continue
		}
		// Mean attacking multiplier over the club's own fixtures in each window,
		// from the shipped ladder. Dividing the recent window by the ease of the
		// fixtures that produced it leaves output at neutral difficulty;
		// multiplying by the next window's ease puts it back on that window's
		// terms, which is what a fair prediction of it needs.
		easeRecent := c.meanEase(shareFrom, shareMid)
		easeNext := c.meanEase(shareMid+1, 38)
		if easeRecent <= 0 || easeNext <= 0 {
			continue
		}
		blendRaw := math.Sqrt(c.modelGoals * recent)
		blendAdj := math.Sqrt(c.modelGoals * (recent / easeRecent * easeNext))
		d := math.Log(blendRaw / actual)
		rawErr += d * d
		d = math.Log(blendAdj / actual)
		adjErr += d * d
		n2++

		formVals = append(formVals, math.Log(recent/c.modelGoals))
		diffVals = append(diffVals, easeRecent)
	}
	if n2 > 2 {
		raw := math.Sqrt(rawErr / n2)
		adj := math.Sqrt(adjErr / n2)
		fmt.Printf("%d club-windows, predicting GW%d-38 with the 50/50 blend:\n\n",
			int(n2), shareMid+1)
		fmt.Printf("  %-46s %.4f\n", "recent output as it stands", raw)
		fmt.Printf("  %-46s %.4f\n", "recent output adjusted for fixture ease", adj)
		fmt.Printf("\n  correlation of form with the ease of the fixtures that produced it: %+.3f\n",
			correlation(diffVals, formVals))
		// Does a club's past fixture ease predict its future fixture ease? This is
		// the load-bearing claim behind treating the fixture component of form as
		// noise, and it deserves a number rather than the assertion that "everyone
		// plays everyone". If past ease DID predict future ease, the fixture part
		// of form would carry real information about the next stretch and
		// discarding it would be throwing away signal.
		//
		// Note what this does NOT say. The next window's fixtures are known in
		// advance — the fixture list and its difficulty ratings are published — and
		// the model prices them per fixture downstream. The question here is only
		// whether *last* window's opponents tell you anything about *next*
		// window's, which is what carrying them through a backward-looking form
		// term would assume.
		var e1, e2 []float64
		for _, c := range all {
			a, b := c.meanEase(shareFrom, shareMid), c.meanEase(shareMid+1, 38)
			if a > 0 && b > 0 {
				e1 = append(e1, a)
				e2 = append(e2, b)
			}
		}
		fmt.Printf("\n  correlation of a club's PAST fixture ease with its FUTURE ease: %+.3f\n",
			correlation(e1, e2))
		fmt.Printf("  (%d club-windows. Near zero means last window's opponents say nothing\n",
			len(e1))
		fmt.Printf("  about next window's, so the fixture part of form cannot transfer —\n")
		fmt.Printf("  which is a different claim from fixtures being unpredictable, since\n")
		fmt.Printf("  the coming fixtures are published and the model already prices them.)\n")
		sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "fixture ease persistence",
			len(e1), measure{"correlation of past ease with future ease", correlation(e1, e2)})

		fmt.Printf("\nAdjusting HELPS means form is partly real team change and the fixture\n")
		fmt.Printf("component was noise in it. Adjusting HURTS means the model's ladder is\n")
		fmt.Printf("already over-correcting, and what looks like form is the residue the\n")
		fmt.Printf("ladder leaves — in which case the lever is the ladder, not a club term.\n")
		sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "form against fixtures",
			int(n2),
			measure{"rms log error, recent output raw", raw},
			measure{"rms log error, recent output fixture-adjusted", adj},
			measure{"correlation of form with fixture ease",
				correlation(diffVals, formVals)})
	}

	// -----------------------------------------------------------------
	// The arithmetic check with no football in it.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== expected minutes per club per match, which must be about 990\n")
	fmt.Printf("Eleven players for ninety minutes. This has no football in it, so a club\n")
	fmt.Printf("systematically out here says the share error is in the MINUTES rather than\n")
	fmt.Printf("in the rates — a different and much cheaper repair than the anchor.\n")
	fmt.Printf("Read the sd, not the mean: it is the only figure in this diagnostic with no\n")
	fmt.Printf("sampling term at all, since 990 is arithmetic rather than a measurement, and\n")
	fmt.Printf("it BOUNDS the minutes channel small rather than ruling it out.\n\n")
	var mm, msd, mlo, mhi float64
	mlo, mhi = math.Inf(1), math.Inf(-1)
	for _, c := range all {
		mm += c.modelMinutes
		mlo = math.Min(mlo, c.modelMinutes)
		mhi = math.Max(mhi, c.modelMinutes)
	}
	mm /= float64(len(all))
	for _, c := range all {
		msd += (c.modelMinutes - mm) * (c.modelMinutes - mm)
	}
	msd = math.Sqrt(msd / float64(len(all)))
	fmt.Printf("mean %.0f, sd %.0f, min %.0f, max %.0f, against 990 for eleven full matches\n",
		mm, msd, mlo, mhi)
	sink.emitAll("team_goal_share", "GW19 fit, GW20-38 scored", "expected minutes per club",
		len(all),
		measure{"mean expected minutes per club per match", mm},
		measure{"spread across clubs", msd})

	// -----------------------------------------------------------------
	// The clubs themselves, so a tail can be read rather than inferred.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== the five furthest out in each direction, against realised expected goals\n")
	rows := append([]shareClub(nil), all...)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].ratioAgainst(rows[i].realisedXG()) <
			rows[j].ratioAgainst(rows[j].realisedXG())
	})
	fmt.Printf("%-6s %8s %8s %8s\n", "club", "modelled", "realised", "ratio")
	for i, c := range rows {
		if i >= 5 && i < len(rows)-5 {
			continue
		}
		if i == len(rows)-5 {
			fmt.Printf("%-6s %8s %8s %8s\n", "...", "", "", "")
		}
		fmt.Printf("%-6s %8.2f %8.2f %8.3f\n",
			c.name, c.modelGoals, c.realisedXG(), c.ratioAgainst(c.realisedXG()))
	}

	fmt.Printf("\nDecision rule, fixed before this was run: sd under about 0.10 drops Gap 3,\n")
	fmt.Printf("a material tail beyond 30%% keeps it live, anything between is unresolved\n")
	fmt.Printf("and should be recorded as unresolved. A between-club level error is a real\n")
	fmt.Printf("bias either way, and correcting a measured bias has lost this project points\n")
	fmt.Printf("five times — this gates whether the anchor is worth building, nothing more.\n")
}

// splitHalfFloor is the sampling sd of a full-window realised mean, estimated
// model-free by comparing each club's first half of the held-out window against its
// own second half.
//
// Two independent half-window means each carry about twice the variance of the
// whole, so the variance of their ratio is about four times it and the sd divides by
// two. Done in logs: the identity is a log-scale result, and a right-skewed ratio
// has a larger raw sd than log sd, which would overstate the floor and understate
// what is left for the model — biased toward dropping this item, which is the wrong
// direction for a diagnostic to be conservative in.
//
// What it measures is sampling noise **plus genuine within-season change**, and both
// belong in the floor for this question: a club that improves after January is not
// something a static anchor fitted at GW19 could have fixed either. So read the
// residual as "spread a static between-club correction could address" rather than as
// "the model's error", which is a larger and different quantity.
func splitHalfFloor(cs []shareClub) float64 {
	var ls []float64
	for _, c := range cs {
		f, s, ok := c.splitHalf()
		if !ok || f <= 0 || s <= 0 {
			continue
		}
		ls = append(ls, math.Log(f/s))
	}
	if len(ls) < 2 {
		return 0
	}
	var mean float64
	for _, v := range ls {
		mean += v
	}
	mean /= float64(len(ls))
	var sd float64
	for _, v := range ls {
		sd += (v - mean) * (v - mean)
	}
	return math.Sqrt(sd/float64(len(ls))) / 2
}

// meanAndSE was a seventh implementation of the mean and its standard error, in
// the package whose stats_test.go header records consolidating six.
//
// It is now a one-line forward to that file's meanSE, which computes the same
// quantity with the same two-value guard. Kept as a name because two call sites
// read better with it than with the shared one.
func meanAndSE(vs []float64) (mean, se float64) { return meanSE(vs) }

// expectedBeyond is the share of clubs a PERFECT model would still put more than
// `tol` away from 1, given a denominator whose sampling sd is `sd`. It is the null
// the tail count needs: without it "11 of 80" is compared against zero, which is not
// the right comparison.
func expectedBeyond(mean, sd, tol float64) float64 {
	if sd <= 0 {
		return 0
	}
	hi := (1 + tol - mean) / sd
	lo := (1 - tol - mean) / sd
	return (1 - normalCDF(hi)) + normalCDF(lo)
}

// normalCDF is the standard normal distribution function, via the error function.
func normalCDF(z float64) float64 { return 0.5 * (1 + math.Erf(z/math.Sqrt2)) }

// correlation is Pearson's r over paired observations, 0 when undefined.
func correlation(xs, ys []float64) float64 {
	n := float64(len(xs))
	if n < 3 {
		return 0
	}
	var sx, sy, sxx, syy, sxy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
		sxx += xs[i] * xs[i]
		syy += ys[i] * ys[i]
		sxy += xs[i] * ys[i]
	}
	den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}

// ratioStats is the mean, spread, minimum and maximum of modelled over realised
// across a set of clubs. The spread is the figure the decision rule reads; the mean
// is held near 1 by calibrateExpectedStats and is reported only so a reader can see
// that it is.
func ratioStats(cs []shareClub, realised func(shareClub) float64) (mean, sd, lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	var rs []float64
	for _, c := range cs {
		v := c.ratioAgainst(realised(c))
		if v <= 0 {
			continue
		}
		rs = append(rs, v)
		mean += v
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	if len(rs) == 0 {
		return 0, 0, 0, 0
	}
	mean /= float64(len(rs))
	for _, v := range rs {
		sd += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(sd / float64(len(rs))), lo, hi
}

// teamShortName resolves a club id to its three-letter code, falling back to the
// id so an unmatched club is visible in the table rather than blank.
func teamShortName(teams []fpl.Team, id int) string {
	for _, t := range teams {
		if t.ID == id {
			return t.ShortName
		}
	}
	return fmt.Sprintf("#%d", id)
}
