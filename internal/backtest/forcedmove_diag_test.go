package backtest

// The forced-move census: how often does a regular simply stop playing, and does
// the rate rise in congested weeks.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagForcedMoveCensus -v -timeout 30m
//
// # The question, and whose question it is
//
// The insurance reading of a held transfer says its value is P(forced move soon) x
// the cost of being caught short — a compulsory transfer (the injury you must
// answer) clusters where the fixtures do, because congestion drives injuries and
// rotation. The taper's congestion half prices exactly that and is **asserted,
// not measured**: `CongestionSensitivity` 1.0 and `CongestionHorizon` 5 were
// picked on mechanism with no count behind them. The option-decay 2x2 then read
// the taper's interaction as a point-estimate reversal of the congestion-tax
// prediction — the one channel with a mechanism prediction is the one that ran the
// wrong way. Before any decomposition arm is spent on decay-versus-congestion,
// the cheap question is whether the congestion channel has ANY mass to price:
// does a regular's availability actually break more often in doubling weeks than
// in ordinary ones?
//
// This is the count the forced-move note names as owed first: **from the archive,
// no replay, no threshold** — a rate against a calendar. It needs no detection
// threshold because nothing is being separated; the decision it feeds is whether
// a decomposition arm is worth designing, not which arm wins.
//
// ⚠️ The insurance identity has more than the break rate, and this census
// reports all of it. The prevalence half is the break rate; the cost half is the
// burden — the points a caught-short manager misses when the break lands. A break
// on the eve of a double misses two matches' worth of the player's own points,
// one in an ordinary week, so a prevalence ratio below 1 can still leave the
// cost-weighted channel with mass (added 2026-08-18 on the user's note: "it may
// not be as prevalent in doubles, but it can be twice as costly when it happens"
// — measured at 1.34x). And the third channel, added on the user's second note
// the same day — "that's not really the goal of a double, you want full
// appearance points": the appearance SHORTFALL, a regular who misses a 60-minute
// leg of the week, measured at 2.07x expected missed appearance points.
//
// # The proxy, and what it deliberately is not
//
// "Forced" has no expression in an archive: a row cannot say whether a player was
// injured, rested, dropped or sold. The proxy is an **availability break**: a
// player who appeared in each of his club's previous three played gameweeks and
// then records zero minutes in the next two. Three appearances is the regularity
// bar — a two-week cameo is not a forced move when it stops — and two consecutive
// zero weeks is the persistence bar, because a one-week rest is not a move a
// manager must answer. ⚠️ The proxy misses breaks by injury-then-sale (the
// player leaves the archive's weekly rows entirely — the same shape the record's
// "replay bought players who had left" class names), and it cannot see the
// GW1-GW3 window, where nobody yet has three appearances. It measures the floor
// of the mechanism's mass, not the mass.
//
// # The calendar is the club's, not the league's
//
// A club-gameweek is the unit: the player's weekly minutes are summed over the
// club's fixtures that week (one leg of a double is not a separate week), a blank
// week (zero fixtures) is skipped rather than read as an absence, and the
// before/after sequences run over **played** club-gameweeks only. The club itself
// is the fixture's, via `was_home`, not `players_raw`'s end-of-season team — a
// January transfer is a new (element, club) pair and starts its sequence over.
// A congested week is one where the club plays two or more matches
// (`clubGWFixtures >= 2`).
//
// The seasons are the replay's six, so the rates describe the same calendar the
// taper's congestion half acts on — the replays' squads are not involved.

import (
	"fmt"
	"math"
	"testing"

	"armband/internal/analysis"
)

func TestDiagForcedMoveCensus(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	mirrorArchive(t, cfg)

	type seasonTally struct {
		name       string
		playedSGL  int // club-gameweeks with exactly one fixture
		playedDBL  int // club-gameweeks with two or more
		atRisk     int // regular (element, club) pairs with three preceding appearances
		breaksSGL  int
		breaksDBL  int
		burdenSGL  float64 // sum of expected missed points over single-week breaks
		burdenDBL  float64 // the same over doubling-week breaks
		shortSGL   int     // regular-weeks missing an appearance point (legs60 < fixtures)
		shortDBL   int
		appSGL     float64 // missed appearance points: (fixtures - legs60) x 2, summed
		appDBL     float64
		bracketSGL map[string]int
		bracketDBL map[string]int
	}

	var totals []seasonTally
	for _, name := range []string{
		"2020-21", "2021-22", "2022-23", "2023-24", "2024-25", "2025-26",
	} {
		sm := loadMatches(t, cfg, name)
		if !sm.HasCalendar {
			t.Logf("%s: no calendar; skipped", name)
			continue
		}

		// Per (element, club), the sequence of played club-gameweeks with the
		// player's minute totals, ascending. The map holds the sum over a
		// double's legs; a blank week never appears. legsPlayed counts the legs
		// with any minutes and legs60 the legs with at least the
		// appearance-point threshold, so "full appearance points in a double" —
		// the user's reading of what a double is FOR — is expressible exactly: a
		// missed leg costs 2 appearance points, and a leg of 1-59 minutes has
		// banked 1 and misses the second.
		type seqEntry struct{ gw, minutes, fixtures, points, legsPlayed, legs60 int }
		seqs := map[[2]int][]seqEntry{}
		leg60 := func(minutes int) int {
			if minutes >= analysis.AppearanceMinutes {
				return 1
			}
			return 0
		}
		legPlayed := func(minutes int) int {
			if minutes > 0 {
				return 1
			}
			return 0
		}
		for _, r := range sm.Rows {
			key := [2]int{r.Element, r.Club}
			seq := seqs[key]
			if len(seq) > 0 && seq[len(seq)-1].gw == r.GW {
				seq[len(seq)-1].minutes += r.Minutes
				seq[len(seq)-1].points += r.Points
				seq[len(seq)-1].legs60 += leg60(r.Minutes)
				seq[len(seq)-1].legsPlayed += legPlayed(r.Minutes)
				continue
			}
			seqs[key] = append(seq, seqEntry{r.GW, r.Minutes,
				sm.clubGWFixtures(r.Club, r.GW), r.Points, legPlayed(r.Minutes), leg60(r.Minutes)})
		}
		// Price in tenths per (element, gw), for the pre-break bracket.
		priceAt := map[[2]int]int{}
		for _, r := range sm.Rows {
			if r.Value > 0 {
				priceAt[[2]int{r.Element, r.GW}] = r.Value
			}
		}

		st := seasonTally{
			name:       name,
			bracketSGL: map[string]int{},
			bracketDBL: map[string]int{},
		}
		for key, seq := range seqs {
			for i := 3; i+1 < len(seq); i++ {
				// The regularity bar: three consecutive appearances, in the
				// played-week sequence (blanks were never added, so consecutive
				// here means consecutive played weeks). The exposure — the
				// denominator — is every regular-week, congested or not; the
				// break is the subset of them where the player then stops.
				if seq[i-3].minutes == 0 || seq[i-2].minutes == 0 || seq[i-1].minutes == 0 {
					continue
				}
				dbl := seq[i].fixtures >= 2
				if dbl {
					st.playedDBL++
				} else {
					st.playedSGL++
				}
				st.atRisk++
				// The break starts at i: zero this week AND the next played week.
				if seq[i].minutes != 0 || seq[i+1].minutes != 0 {
					continue
				}
				// The cost half of the insurance identity: what a manager caught
				// short misses. The burden is the player's own points per match
				// over his three preceding appearances, times the legs his club
				// plays in the break's start week — one match in an ordinary
				// week, two in a double. A bound on the missed points, not a
				// valuation: the player might have scored less than his average,
				// and a manager with a free transfer pays only the difference.
				ppm := rate(seq[i-3].points+seq[i-2].points+seq[i-1].points,
					seq[i-3].fixtures+seq[i-2].fixtures+seq[i-1].fixtures)
				if dbl {
					st.breaksDBL++
					st.burdenDBL += ppm * float64(seq[i].fixtures)
				} else {
					st.breaksSGL++
					st.burdenSGL += ppm * float64(seq[i].fixtures)
				}
				// The bracket is the player's price the week before the break —
				// the market's view, not the outcome.
				b := priceBracket(priceAt[[2]int{key[0], seq[i-1].gw}])
				if dbl {
					st.bracketDBL[b]++
				} else {
					st.bracketSGL[b]++
				}
			}
		}
		// The appearance-shortfall channel — the user's reading of what a double
		// is FOR: full appearance points in every leg, not merely any minutes in
		// the week. A shortfall is a regular-week with fewer 60-minute legs than
		// the club plays fixtures. The missed appearance points are the EXACT
		// FPL valuation: a leg never entered costs both points (2), a leg of
		// 1-59 minutes has banked the first and misses the second (1). The break
		// above is the all-legs extreme of the same channel, so shortfall counts
		// are a superset of it. No persistence bar: a shortfall needs none, so
		// the loop runs to the end of the sequence (unlike the break loop's
		// i+1 bound, which exists to see the next played week).
		for _, seq := range seqs {
			for i := 3; i < len(seq); i++ {
				if seq[i-3].minutes == 0 || seq[i-2].minutes == 0 || seq[i-1].minutes == 0 {
					continue
				}
				if seq[i].legs60 >= seq[i].fixtures {
					continue
				}
				missed := 2*float64(seq[i].fixtures-seq[i].legsPlayed) +
					float64(seq[i].legsPlayed-seq[i].legs60)
				dbl := seq[i].fixtures >= 2
				if dbl {
					st.shortDBL++
					st.appDBL += missed
				} else {
					st.shortSGL++
					st.appSGL += missed
				}
			}
		}
		totals = append(totals, st)
	}

	fmt.Printf("\n=== forced-move census: availability breaks in doubling vs ordinary weeks\n")
	fmt.Printf("%-9s %6s %6s %6s %6s %9s %9s %7s\n",
		"season", "playS", "playD", "brkS", "brkD", "rate/S", "rate/D", "ratio")
	var sumS, sumD, brkS, brkD int
	var burS, burD float64
	for _, st := range totals {
		ratio := math.NaN()
		if st.breaksSGL > 0 && st.breaksDBL > 0 && st.playedSGL > 0 && st.playedDBL > 0 {
			ratio = (float64(st.breaksDBL) / float64(st.playedDBL)) /
				(float64(st.breaksSGL) / float64(st.playedSGL))
		}
		fmt.Printf("%-9s %6d %6d %6d %6d %9.4f %9.4f %7.2f\n",
			st.name, st.playedSGL, st.playedDBL, st.breaksSGL, st.breaksDBL,
			rate(st.breaksSGL, st.playedSGL), rate(st.breaksDBL, st.playedDBL), ratio)
		sumS += st.playedSGL
		sumD += st.playedDBL
		brkS += st.breaksSGL
		brkD += st.breaksDBL
		burS += st.burdenSGL
		burD += st.burdenDBL
	}
	fmt.Printf("%-9s %6d %6d %6d %6d %9.4f %9.4f %7.2f\n",
		"pooled", sumS, sumD, brkS, brkD,
		rate(brkS, sumS), rate(brkD, sumD),
		(rate(brkD, sumD) / rate(brkS, sumS)))
	fmt.Println()

	// The cost half: the insurance value is P(break) x missed points, and a break
	// on the eve of a double misses two matches' worth. Expected burden per
	// regular club-gameweek in each week type.
	expS := burS / float64(sumS)
	expD := burD / float64(sumD)
	fmt.Printf("%-9s %10s %10s %7s\n", "burden", "exp/S", "exp/D", "ratio")
	fmt.Printf("%-9s %10.4f %10.4f %7.2f\n", "pooled", expS, expD, expD/expS)

	// The appearance-shortfall channel: what a double is FOR is full appearance
	// points in every leg, and a regular playing one leg of two has missed one.
	var shS, shD int
	var apS, apD float64
	for _, st := range totals {
		shS += st.shortSGL
		shD += st.shortDBL
		apS += st.appSGL
		apD += st.appDBL
	}
	rateShortS := rate(shS, sumS)
	rateShortD := rate(shD, sumD)
	expApS := apS / float64(sumS)
	expApD := apD / float64(sumD)
	fmt.Printf("\n=== the appearance-shortfall channel: a regular missing a 60-minute leg\n")
	fmt.Printf("%-9s %8s %8s %8s %8s %10s %10s %7s\n",
		"", "shortS", "shortD", "rate/S", "rate/D", "appPts/S", "appPts/D", "ratio")
	fmt.Printf("%-9s %8d %8d %8.4f %8.4f %10.4f %10.4f %7.2f\n",
		"pooled", shS, shD, rateShortS, rateShortD, expApS, expApD, expApD/expApS)

	// The premium question: the insurance reading names premiums specifically —
	// they play more, so congestion exposure concentrates on them.
	fmt.Printf("%-8s %7s %7s\n", "bracket", "brkS", "brkD")
	aggS, aggD := map[string]int{}, map[string]int{}
	for _, st := range totals {
		for b, n := range st.bracketSGL {
			aggS[b] += n
		}
		for b, n := range st.bracketDBL {
			aggD[b] += n
		}
	}
	order := []string{"<5.0", "5.0-6.4", "6.5-7.9", "8.0-9.9", "10.0+"}
	for _, b := range order {
		fmt.Printf("%-8s %7d %7d\n", b, aggS[b], aggD[b])
	}

	fmt.Printf(`
The hypothesis under test: forced-move demand is HIGHER in congested weeks. The
prevalence half is the rate ratio above; the insurance value is P(break) x the
points a caught-short manager misses, and the cost half is the burden ratio — a
break on the eve of a double misses two matches' worth of the player's own points,
one in an ordinary week. The channel has mass only where the burden ratio exceeds
1 even if the rate ratio does not. No threshold, no p: this is a rate against a
calendar, not a comparison between two replayed arms.
`)
}
