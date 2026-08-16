package backtest

// Two free diagnostics that gate two queued modelling changes. Neither replays a
// season, so both cost one test run rather than hours.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagFreeGates -v -timeout 30m
//
// # Gate one: how much do team-mates' clean-sheet estimates differ, and why
//
// **The framing this gate was written under was WRONG, and the correction is why
// cssplit_test.go and csvalue_test.go exist. Read those before this.**
//
// The original claim was that a clean sheet is a team event, so eleven team-mates
// having eleven different probabilities must be error, and `baseXP90` computing it
// from `m.XGC90` — FPL's *per-player* expected goals conceded per 90, measured
// while that player was on the pitch — must therefore be wrong.
//
// It is not. FPL awards a clean sheet for reaching 60 minutes without conceding
// WHILE ON THE PITCH, so a full-back withdrawn at 70 minutes with the score 0-0
// keeps his four points when the side concedes at 85. It is a player event
// conditioned on his own minutes window. Team-mates SHOULD differ, and a
// habitually substituted defender's lower per-90 figure is partly a real
// measurement of shorter exposure rather than noise about a shared quantity.
//
// So this gate measures a MIXTURE and cannot separate its parts. Two populations
// are reported to bracket it, and neither is a clean bound:
//
//   - strict, at everPresentShare of the club's minutes. Intended as a lower
//     bound, and compromised: filtering on minutes selects AGAINST full-backs
//     specifically, which is the exact variable driving the effect. One to seven
//     clubs per season qualify, so read it as a centre-back population.
//   - loose, at 50%+. Contains real exposure signal, so it is not error.
//
// What IS a valid check, and the one to use after any change to the clean sheet:
// among players who play exactly 90 minutes in every appearance, exposure is
// identical, so the HAZARD component must be identical within a club.
//
// # Gate two: do a club's players' expected goals sum to anything?
//
// Nothing constrains them. This sums the model's expected goals across each
// club's whole squad and compares to what the club actually scored. The number
// that decides whether the team-goals anchor is worth building is not the mean
// ratio — a mean error is a league-wide level bias, which this project has
// repeatedly found an argmax cannot see — but the **spread across clubs**, which
// is a per-club bias the anchor would remove and which nothing has measured.
//
// It also reports each club's summed expected minutes, which should be 990 per
// gameweek (eleven players times ninety). That separates the two ways the sum can
// be wrong: shares that do not add up, against minutes that do not add up.

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// everPresentShare is how much of his club's available minutes a player must have
// taken to count as ever-present. Ever-present is the right population for gate
// one because it removes the one legitimate reason two team-mates' expected goals
// conceded could differ — that they were on the pitch for different matches.
const everPresentShare = 0.95

func TestDiagFreeGates(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)

	// Two cutoffs rather than one, because both quantities are blends whose
	// weight on the current season rises through the year: a mid-season and a
	// late-season reading say whether what we find is a property of the model or
	// of how little football it had seen.
	cutoffs := []int{19, 30}

	type clubRow struct {
		season, cut    string
		team           int
		nEver, nAll    int
		minCSA, maxCSA float64
		minXGC, maxXGC float64
		minCS, maxCS   float64
		xMinsSum       float64
		impliedGoals   float64
		actualGoals    float64
		matches        int
	}
	var rows []clubRow

	for _, pair := range sweepPairNames() {
		prior := loadSeason(t, cfg, pair[0])
		cur := loadSeason(t, cfg, pair[1])
		idx := newPriorIndex(prior)

		for _, cut := range cutoffs {
			boot, fx := PointInTime(cur, prior, cut)
			e := analysis.NewEngineFull(boot, fx, cfg.Weights,
				analysis.Congestion{}, analysis.RoleRisk{})
			e.Priors = idx
			e.Recent = newRecentIndexWith(cur, cut,
				cfg.Weights.MinutesHalfLife, cfg.Weights.RateHalfLife)

			// Matches played per club up to the cutoff, counted from the fixture
			// list rather than from player rows, because a blank gameweek has no
			// row at all and summing rows would silently undercount.
			matches := map[int]int{}
			for _, f := range cur.Fixtures {
				if f.Event == nil || *f.Event > cut {
					continue
				}
				matches[f.TeamH]++
				matches[f.TeamA]++
			}

			// Accumulate per club.
			type acc struct {
				xgcs    []float64
				xgcsAll []float64
				xMins   float64
				implied float64
				goals   int
			}
			byTeam := map[int]*acc{}
			get := func(id int) *acc {
				if byTeam[id] == nil {
					byTeam[id] = &acc{}
				}
				return byTeam[id]
			}

			for i := range boot.Elements {
				el := &boot.Elements[i]
				p := cur.Players[el.ID]
				if p == nil {
					continue
				}
				a := get(el.Team)
				m := e.Metrics(el)

				// Gate two, attacking side: expected goals per gameweek is a per-90
				// rate scaled by expected minutes. Summed over the squad this is the
				// club's implied goals per gameweek.
				a.implied += m.XG90 * m.ExpectedMinutes / 90.0
				a.xMins += m.ExpectedMinutes

				var mins, goals int
				for gw := 1; gw <= cut; gw++ {
					g, ok := p.GWs[gw]
					if !ok {
						continue
					}
					mins += g.Minutes
					goals += g.Goals
				}
				a.goals += goals

				// Gate one: only defenders and keepers collect a clean sheet worth
				// having, and only the ever-present remove the confound.
				if el.ElementType > 2 {
					continue
				}
				avail := matches[el.Team] * 90
				if avail == 0 {
					continue
				}
				share := float64(mins) / float64(avail)
				// The loose population: anyone who played half his club's minutes.
				// It reintroduces the confound this test excludes — two team-mates
				// really were on the pitch for different matches — so read it as an
				// upper bound where the strict column is a lower bound.
				if share >= 0.50 {
					a.xgcsAll = append(a.xgcsAll, m.XGC90)
				}
				if share < everPresentShare {
					continue
				}
				a.xgcs = append(a.xgcs, m.XGC90)
			}

			var teams []int
			for id := range byTeam {
				teams = append(teams, id)
			}
			sort.Ints(teams)
			for _, id := range teams {
				a := byTeam[id]
				r := clubRow{
					season: pair[1], cut: fmt.Sprintf("GW%d", cut), team: id,
					nEver: len(a.xgcs), nAll: len(a.xgcsAll), xMinsSum: a.xMins,
					impliedGoals: a.implied,
					actualGoals:  float64(a.goals),
					matches:      matches[id],
				}
				if len(a.xgcs) > 0 {
					r.minXGC, r.maxXGC = a.xgcs[0], a.xgcs[0]
					for _, v := range a.xgcs {
						r.minXGC = math.Min(r.minXGC, v)
						r.maxXGC = math.Max(r.maxXGC, v)
					}
					// The clean-sheet probability at a factor of 1, which is what
					// the term reduces to before the coupling and the xGC factor.
					r.minCS = math.Exp(-r.maxXGC)
					r.maxCS = math.Exp(-r.minXGC)
				}
				if len(a.xgcsAll) > 0 {
					lo, hi := a.xgcsAll[0], a.xgcsAll[0]
					for _, v := range a.xgcsAll {
						lo = math.Min(lo, v)
						hi = math.Max(hi, v)
					}
					r.minCSA, r.maxCSA = math.Exp(-hi), math.Exp(-lo)
				}
				rows = append(rows, r)
			}
		}
	}

	// ---------------------------------------------------------------- gate one
	fmt.Printf("\n=== GATE ONE: how much do team-mates' clean-sheet estimates differ?\n")
	fmt.Printf("NOTE the premise this gate was built on is refuted: a clean sheet is a\n")
	fmt.Printf("PLAYER event (60+ minutes without conceding WHILE ON THE PITCH), so\n")
	fmt.Printf("team-mates should differ and part of this spread is real exposure signal.\n")
	fmt.Printf("See TestDiagCleanSheetSplit and TestDiagCleanSheetExposure.\n")
	fmt.Printf("Spread of expected goals conceded per 90, and of the implied clean-sheet\n")
	fmt.Printf("probability exp(-xGC), across each club's EVER-PRESENT defenders and keepers\n")
	fmt.Printf("(>=%.0f%% of the club's available minutes).\n", everPresentShare*100)
	fmt.Printf("strict = ever-present only; NOT a clean lower bound, because filtering on\n")
	fmt.Printf("minutes selects against full-backs, the very players the effect is about.\n")
	fmt.Printf("loose = played 50%%+ of club minutes; contains real exposure signal.\n\n")
	fmt.Printf("%-9s %-5s %7s %9s %9s %7s %9s %9s\n",
		"season", "cut", "nStrict", "meanCSsp", "maxCSsp", "nLoose", "meanCSsp", "maxCSsp")

	type key struct{ season, cut string }
	agg := map[key][]clubRow{}
	var keys []key
	for _, r := range rows {
		k := key{r.season, r.cut}
		if _, seen := agg[k]; !seen {
			keys = append(keys, k)
		}
		agg[k] = append(agg[k], r)
	}
	worstCS := 0.0
	for _, k := range keys {
		var csSprd, csAll []float64
		for _, r := range agg[k] {
			if r.nEver >= 2 {
				csSprd = append(csSprd, r.maxCS-r.minCS)
			}
			if r.nAll >= 2 {
				csAll = append(csAll, r.maxCSA-r.minCSA)
			}
		}
		if len(csAll) == 0 {
			continue
		}
		ms, xs := 0.0, 0.0
		if len(csSprd) > 0 {
			ms, xs = meanOf(csSprd), maxOf(csSprd)
		}
		fmt.Printf("%-9s %-5s %7d %9.4f %9.4f %7d %9.4f %9.4f\n",
			k.season, k.cut, len(csSprd), ms, xs,
			len(csAll), meanOf(csAll), maxOf(csAll))
		worstCS = math.Max(worstCS, maxOf(csAll))
	}
	fmt.Printf("\nWorst single club, loose population: %.4f of clean-sheet probability\n", worstCS)
	fmt.Printf("between two team-mates, or %.2f points per 90 at 4 points a clean sheet.\n", worstCS*4)
	fmt.Printf("Do NOT read that as error: the measured exposure effect is worth 0.16-0.32\n")
	fmt.Printf("points per substituted appearance and is REAL. Ignore the 2022-23 rows —\n")
	fmt.Printf("its prior season carries no expected goals, so the model runs crippled.\n")

	// ---------------------------------------------------------------- gate two
	fmt.Printf("\n=== GATE TWO: do a club's expected goals sum to the club's goals?\n")
	fmt.Printf("Per club: summed expected minutes (should be 990 = 11 x 90), the model's\n")
	fmt.Printf("implied goals per gameweek, and what the club actually scored per match.\n")
	fmt.Printf("THE NUMBER THAT DECIDES THE TEAM-GOALS ANCHOR IS THE SPREAD ACROSS CLUBS,\n")
	fmt.Printf("not the mean: a league-wide level error is invisible to an argmax, a\n")
	fmt.Printf("per-club one is not.\n\n")
	fmt.Printf("%-9s %-5s %5s %9s %9s %9s %9s %9s %9s\n",
		"season", "cut", "clubs", "meanXMin", "meanImp", "meanAct",
		"meanRatio", "sdRatio", "rangeRat")

	for _, k := range keys {
		var ratios, xmins, imp, act []float64
		for _, r := range agg[k] {
			if r.matches == 0 || r.impliedGoals <= 0 {
				continue
			}
			perMatch := r.actualGoals / float64(r.matches)
			ratios = append(ratios, perMatch/r.impliedGoals)
			xmins = append(xmins, r.xMinsSum)
			imp = append(imp, r.impliedGoals)
			act = append(act, perMatch)
		}
		if len(ratios) < 4 {
			continue
		}
		lo, hi := maxOf(ratios), maxOf(ratios)
		for _, v := range ratios {
			lo = math.Min(lo, v)
			hi = math.Max(hi, v)
		}
		fmt.Printf("%-9s %-5s %5d %9.1f %9.3f %9.3f %9.3f %9.3f %6.2f-%.2f\n",
			k.season, k.cut, len(ratios), meanOf(xmins), meanOf(imp), meanOf(act),
			meanOf(ratios), sd(ratios), lo, hi)
	}

	fmt.Printf("\nRead sdRatio against meanRatio. A mean far from 1.0 with a small spread is\n")
	fmt.Printf("a league-wide calibration offset and the anchor would not help. A large\n")
	fmt.Printf("spread is a per-club bias, which is what the anchor removes.\n")
}

func maxOf(xs []float64) float64 {
	m := xs[0]
	for _, v := range xs {
		m = math.Max(m, v)
	}
	return m
}
