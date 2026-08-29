package backtest

// A CENSUS of realised total minutes per club per single-fixture gameweek,
// against the owner's observation: "990 is a cap too. With red cards and
// injuries it is below that."
//
//	DIAG=1 go test ./internal/backtest -run TestDiagRealisedMinutesCensus -v -timeout 60m
//
// # What this feeds
//
// A proposal to "anchor expected minutes to 990" is anchoring to eleven players
// times ninety — the ceiling if nobody is ever sent off and no substitute goes
// unused. The owner's point is that a red card removes a player with no
// replacement, so the club plays out the remainder of the match with ten men and
// its REALISED total minutes falls short of 990 that gameweek. If the true mean
// realised total sits meaningfully below 990, that ceiling is the wrong anchor to
// aim at. This census measures how far short, on realised archive data, and
// isolates how much of the shortfall a red card can explain.
//
// # CENSUS ONLY
//
// No correlation, no slope, no model, no verdict on what to do about it. Counts
// and means, printed plainly, including anything that surprises.
//
// # What is counted, and the one restriction that matters
//
// Per club per gameweek: realised total minutes = sum of Player.GWs[gw].Minutes
// over every player whose season-level Team is that club. Summed only over
// gameweeks the club played EXACTLY ONE fixture (`teamGameweeks`' club-gameweek
// count == 1) — a double gameweek's ceiling is 1980, not 990, and pooling the two
// populations would corrupt every mean below. A blank gameweek contributes no row
// at all (no player carries a GWs entry for a match his club did not play), so it
// is excluded automatically rather than needing its own filter.
//
// Red cards are read the same way: Player.GWs[gw].Red, summed per club-gameweek.
//
// # The one known imprecision, named rather than hidden
//
// `Player.Team` is parsed once from players_raw.csv and is a season-end
// snapshot, not a week-by-week record (the same caveat teamshare_test.go's
// shareClub type carries for the identical reason). A player transferred
// mid-season has his EARLIER gameweeks attributed to his LATER club here. That
// misattributes individual club-gameweek rows on both ends of a transfer; it
// does not fabricate or delete a row.
//
// ⚠️ MEASURED, because "expected to be small" was too vague and an earlier
// reading of this diagnostic mistook the consequence for a defect in the
// archive. Re-keying the same 4,181-row partition on the club a player actually
// turned out for (an independent pass over the raw merged_gw.csv, outside this
// package):
//
//	                    mean      sd     >990          max
//	season-end club    985.23   36.76   430 (10.3%)   1246
//	actual club        985.42    9.60    77 ( 1.8%)   1061
//
// So it is a redistribution, not an inflation: it moves the pooled MEAN by 0.19
// minutes and the SPREAD by a factor of 3.8. The effect on the means really is
// small; the effect on any standard deviation reported here is not, and a reader
// quoting one from this diagnostic is quoting mostly artefact. 853 rows (20.4%)
// carry a wrong total, and 449 of those are TOO LOW — the old club's side of the
// same transfer, which no ">990" test can see.
//
// Named rather than corrected, because correcting it needs a per-fixture team
// resolution this package does not carry for minutes (only doublegwcensus's
// fixture-ID cross-check does, and only for the double/blank question).
//
// # Grid
//
// sweepPairNames() via loadPairs, which defaults to the six-season extended grid
// (FPL_SWEEP_SEASONS selects another). Only the "played" half of each pair
// (seasonPair.Cur) is read; this is realised-data-only, so there is no
// point-in-time model to fit and the prior season is unused.

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"armband/internal/fpl"
)

// clubGW is one club's realised totals in one single-fixture gameweek.
type clubGW struct {
	season       string
	gw, club     int
	minutes, red int
}

func TestDiagRealisedMinutesCensus(t *testing.T) {
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)

	// The grid size is DERIVED, never written. `sweepPairNames` returns six pairs by
	// default but `FPL_SWEEP_SEASONS` moves that within a run, so a hardcoded
	// "six-season" here would label a grid that did not run — which is exactly what
	// `TestPrintedGridLabelsAreDerived` exists to catch, and did catch on the first
	// version of this file.
	fmt.Printf("\n=== grid: sweepPairNames(), %s (FPL_SWEEP_SEASONS selects another)\n",
		seasonsLabel(len(pairs)))

	bySeason := map[string][]clubGW{}
	teamsBySeason := map[string][]fpl.Team{}
	var all []clubGW

	for _, p := range pairs {
		s := p.Cur
		teamsBySeason[s.Name] = s.Teams
		_, count, _ := teamGameweeks(s.Fixtures)

		// Single-fixture (gw, club) pairs only — see the header on why doubles
		// and blanks must not enter this population.
		single := map[[2]int]bool{}
		var doubles, blanks int
		for gw, teams := range count {
			for club, n := range teams {
				switch {
				case n == 1:
					single[[2]int{gw, club}] = true
				case n >= 2:
					doubles++
				default:
					blanks++
				}
			}
		}

		rows := map[[2]int]*clubGW{}
		for _, id := range sortedPlayerIDs(s) {
			pl := s.Players[id]
			for gw, g := range pl.GWs {
				key := [2]int{gw, pl.Team}
				if !single[key] {
					continue
				}
				r := rows[key]
				if r == nil {
					r = &clubGW{season: s.Name, gw: gw, club: pl.Team}
					rows[key] = r
				}
				r.minutes += g.Minutes
				r.red += g.Red
			}
		}

		var keys [][2]int
		for k := range rows {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i][0] != keys[j][0] {
				return keys[i][0] < keys[j][0]
			}
			return keys[i][1] < keys[j][1]
		})
		var seasonRows []clubGW
		for _, k := range keys {
			seasonRows = append(seasonRows, *rows[k])
		}
		bySeason[s.Name] = seasonRows
		all = append(all, seasonRows...)

		fmt.Printf("%-9s single-fixture club-gws: %5d   double club-gws: %4d   blank club-gws: %4d\n",
			s.Name, len(seasonRows), doubles, blanks)
	}

	if len(all) < 100 {
		t.Skipf("only %d single-fixture club-gameweeks pooled; too thin to census", len(all))
	}

	// -----------------------------------------------------------------
	// Section 1: minutes distribution, per season and pooled.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== realised total minutes per club per single-fixture gameweek ===\n")
	fmt.Printf("%-9s %6s | %7s %7s %6s %6s | %8s\n",
		"season", "n", "mean", "sd", "min", "max", "==990")

	var names []string
	for s := range bySeason {
		names = append(names, s)
	}
	sort.Strings(names)

	printMinutesRow := func(label string, rows []clubGW) {
		mean, sd, lo, hi := minutesStats(rows)
		at990 := 0
		for _, r := range rows {
			if r.minutes == 990 {
				at990++
			}
		}
		fmt.Printf("%-9s %6d | %7.1f %7.1f %6d %6d | %6.1f%%\n",
			label, len(rows), mean, sd, lo, hi, 100*float64(at990)/float64(len(rows)))
	}
	for _, s := range names {
		printMinutesRow(s, bySeason[s])
	}
	printMinutesRow("pooled", all)

	// The max column runs well past 990 (pooled max 1246), which is above what
	// eleven men on a single pitch for one match can produce and is therefore
	// not football — a data artefact, and worth naming rather than leaving as
	// an unexplained number in the table above. The leading suspect is the
	// Player.Team snapshot imprecision named in this file's header: a player
	// transferred mid-season has ALL his gameweeks attributed to his
	// season-END club, so a club that bought in January can show two players'
	// worth of minutes for one XI on a week its purchase's former club also
	// fielded him. Reported here as a count and a sample, not corrected.
	var over990 []clubGW
	for _, r := range all {
		if r.minutes > 990 {
			over990 = append(over990, r)
		}
	}
	sort.Slice(over990, func(i, j int) bool { return over990[i].minutes > over990[j].minutes })
	// ⚠️ Do NOT call this tail "impossible" here. ~80% of it is this census's own
	// season-end attribution, not a defect in the archive, and a banner saying
	// otherwise is what produced one wrong finding already. See the comment above.
	fmt.Printf("\n=== the tail above 990 (MOSTLY this census's own season-end attribution; see comment) ===\n")
	fmt.Printf("club-gws with minutes > 990: %d of %d (%.2f%%)\n",
		len(over990), len(all), 100*float64(len(over990))/float64(len(all)))
	n := len(over990)
	if n > 8 {
		n = 8
	}
	for _, r := range over990[:n] {
		name := teamShortName(teamsBySeason[r.season], r.club)
		fmt.Printf("  %-9s GW%-3d %-4s  minutes=%d  (+%d over 990)\n",
			r.season, r.gw, name, r.minutes, r.minutes-990)
	}

	// -----------------------------------------------------------------
	// Section 2: red cards, per club-gameweek.
	// -----------------------------------------------------------------
	fmt.Printf("\n=== red cards, per club per single-fixture gameweek ===\n")
	fmt.Printf("%-9s %6s | %10s %14s | %10s %10s\n",
		"season", "n", "total reds", "reds/club-gw", "cgw w/ red", "share")

	printRedRow := func(label string, rows []clubGW) {
		var totalRed, withRed int
		for _, r := range rows {
			totalRed += r.red
			if r.red > 0 {
				withRed++
			}
		}
		fmt.Printf("%-9s %6d | %10d %14.4f | %10d %9.2f%%\n",
			label, len(rows), totalRed, float64(totalRed)/float64(len(rows)),
			withRed, 100*float64(withRed)/float64(len(rows)))
	}
	for _, s := range names {
		printRedRow(s, bySeason[s])
	}
	printRedRow("pooled", all)

	// -----------------------------------------------------------------
	// Section 3: mean realised minutes split by red-card presence, and the
	// residual decomposition against the 990 ceiling. Pooled only — the
	// red-card subsample is thin per season.
	// -----------------------------------------------------------------
	var noRed, withRed []clubGW
	for _, r := range all {
		if r.red > 0 {
			withRed = append(withRed, r)
		} else {
			noRed = append(noRed, r)
		}
	}
	meanAll, _, _, _ := minutesStats(all)
	meanNoRed, _, _, _ := minutesStats(noRed)
	meanWithRed, _, _, _ := minutesStats(withRed)
	pRed := float64(len(withRed)) / float64(len(all))
	pNoRed := 1 - pRed

	fmt.Printf("\n=== mean realised minutes, split by red-card presence (pooled) ===\n")
	fmt.Printf("no red card in the club-gw (n=%d, %.2f%%): mean %.2f\n",
		len(noRed), 100*pNoRed, meanNoRed)
	fmt.Printf(">=1 red card in the club-gw (n=%d, %.2f%%): mean %.2f\n",
		len(withRed), 100*pRed, meanWithRed)

	fmt.Printf("\n=== residual: how much of the (990 - mean) shortfall red cards explain ===\n")
	shortfall := 990 - meanAll
	fromRed := pRed * (990 - meanWithRed)
	fromNoRed := pNoRed * (990 - meanNoRed)
	fmt.Printf("pooled mean:                          %.2f  (shortfall from 990: %.2f)\n", meanAll, shortfall)
	fmt.Printf("identity check, fromRed + fromNoRed:  %.2f  (should equal the shortfall above)\n",
		fromRed+fromNoRed)
	fmt.Printf("shortfall attributable to red-card club-gws:     %8.2f  (%.1f%% of the shortfall)\n",
		fromRed, pct(fromRed, shortfall))
	fmt.Printf("shortfall in club-gws with NO red card at all:   %8.2f  (%.1f%% of the shortfall)\n",
		fromNoRed, pct(fromNoRed, shortfall))
	if shortfall > 0 && fromNoRed > 0 {
		fmt.Printf("\nThat second line is red cards ruled OUT as the cause: even a club-gameweek\n")
		fmt.Printf("with no red card at all averages %.2f minutes, %.2f short of 990. Named\n",
			meanNoRed, 990-meanNoRed)
		fmt.Printf("rather than attributed — candidates this census does not investigate include\n")
		fmt.Printf("unused substitutes, other-cause dismissals or abandonments the red-card\n")
		fmt.Printf("column does not capture, and archive data gaps.\n")
	}

	fmt.Printf("\nThis census authorises nothing and changes no scoring term.\n")
}

// minutesStats returns mean, population sd, min and max of the minutes field
// over rows. Returns all-zero on an empty slice rather than dividing by zero,
// since a census must not panic on a thin split (e.g. a season with no
// red-card club-gameweeks at all).
func minutesStats(rows []clubGW) (mean, sd float64, lo, hi int) {
	if len(rows) == 0 {
		return 0, 0, 0, 0
	}
	lo, hi = rows[0].minutes, rows[0].minutes
	var sum float64
	for _, r := range rows {
		sum += float64(r.minutes)
		if r.minutes < lo {
			lo = r.minutes
		}
		if r.minutes > hi {
			hi = r.minutes
		}
	}
	mean = sum / float64(len(rows))
	var ss float64
	for _, r := range rows {
		d := float64(r.minutes) - mean
		ss += d * d
	}
	sd = math.Sqrt(ss / float64(len(rows)))
	return mean, sd, lo, hi
}

// pct is num as a percentage of denom, 0 when denom is 0 rather than NaN or Inf
// — denom is a shortfall that could in principle be exactly zero if a pooled
// mean ever landed exactly on 990.
func pct(num, denom float64) float64 {
	if denom == 0 {
		return 0
	}
	return 100 * num / denom
}
