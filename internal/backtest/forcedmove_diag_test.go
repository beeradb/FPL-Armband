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
// The seasons are the replay's six, so the rates describe exactly the population
// the taper's congestion half acts on.

import (
	"fmt"
	"math"
	"os"
	"testing"
)

func TestDiagForcedMoveCensus(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	mirrorArchive(t, cfg)

	type seasonTally struct {
		name       string
		playedSGL  int // club-gameweeks with exactly one fixture
		playedDBL  int // club-gameweeks with two or more
		atRisk     int // regular (element, club) pairs with three preceding appearances
		breaksSGL  int
		breaksDBL  int
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
		// double's legs; a blank week never appears.
		type seqEntry struct{ gw, minutes, fixtures int }
		seqs := map[[2]int][]seqEntry{}
		for _, r := range sm.Rows {
			key := [2]int{r.Element, r.Club}
			seq := seqs[key]
			if len(seq) > 0 && seq[len(seq)-1].gw == r.GW {
				seq[len(seq)-1].minutes += r.Minutes
				continue
			}
			seqs[key] = append(seq, seqEntry{r.GW, r.Minutes, sm.clubGWFixtures(r.Club, r.GW)})
		}
		// Price in tenths per (element, gw), for the pre-break bracket. The rows
		// are sorted by (GW, element), so a simple map keeps the loop linear.
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
				if dbl {
					st.breaksDBL++
				} else {
					st.breaksSGL++
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
		totals = append(totals, st)
	}

	fmt.Printf("\n=== forced-move census: availability breaks in doubling vs ordinary weeks\n")
	fmt.Printf("%-9s %6s %6s %6s %6s %9s %9s %7s\n",
		"season", "playS", "playD", "brkS", "brkD", "rate/S", "rate/D", "ratio")
	var sumS, sumD, brkS, brkD int
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
	}
	fmt.Printf("%-9s %6d %6d %6d %6d %9.4f %9.4f %7.2f\n",
		"pooled", sumS, sumD, brkS, brkD,
		rate(brkS, sumS), rate(brkD, sumD),
		(rate(brkD, sumD) / rate(brkS, sumS)))
	fmt.Println()

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
The hypothesis under test: the break rate is HIGHER in doubling weeks than in
ordinary ones — the mechanism the taper's congestion half prices. A pooled ratio
near or below 1 means the channel has no mass in the archive and a
decay-versus-congestion decomposition arm would be measuring an asserted premise
rather than testing it. A ratio clearly above 1 sizes the channel's mass and the
premium bracket row answers whose breaks those are. No threshold, no p: this is a
rate against a calendar, not a comparison between two replayed arms.
`)
}
