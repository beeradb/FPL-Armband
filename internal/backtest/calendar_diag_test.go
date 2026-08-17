package backtest

// What shape is the fixture calendar, and is there anything for a chip to anchor to?
//
// The standard human chip strategy is anchored on the calendar rather than on a
// week number: build toward a double gameweek, free hit the blank, wildcard into
// or out of the double. Every chip week ever measured on this harness is a
// hardcoded number (`Wildcard: 20`), a fixed offset from entry (entry+3), or a
// hindsight argmax over per-week gains. **Nothing has ever anchored a chip to the
// fixture calendar**, so the strategy that dominates real play is untested here.
//
// This runs first and costs seconds, because it can kill the idea before anything
// expensive is built: if a season's calendar has no doubles worth building toward,
// or its blanks and doubles do not sit where the strategy assumes, then anchoring
// has nothing to anchor to and the sweep behind it is wasted runtime.
//
// # Two things it must report honestly
//
// **A double is a *team*-gameweek, not a player-gameweek.** The record counts
// them that way — 42 in 2022-23 falling to 10 in each of the last two seasons —
// and the trend matters more than the level: the seasons where this strategy has
// the most to work with are the oldest, and 2022-23 is the held-out season.
//
// **The replay's fixture list is final, and FPL's is not.** The archive knows
// every double from GW1 where a real manager learns of one as cup rounds resolve,
// realistically two to six gameweeks ahead. So the *lead time* column below is the
// one that decides whether an anchored chip plan can be measured honestly at all:
// a strategy that builds toward a GW34 double from August is trading on months of
// hindsight nobody had. The record already flags this for the fixture-load work
// and calls its +33 optimistic; here it is worse, because the timing *is* the
// strategy.
//
// This diagnostic cannot measure lead time — the archive carries no announcement
// dates — so it reports the calendar and says plainly that the guard has to come
// from elsewhere.

import (
	"fmt"
	"os"
	"sort"
	"testing"
)

type calendarWeek struct {
	gw       int
	doubling int // clubs with 2+ fixtures
	blanking int // clubs with none
	fixtures int
}

func TestDiagFixtureCalendar(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)

	fmt.Printf("\n=== the fixture calendar, and whether a chip has anything to anchor to\n")
	fmt.Printf("A double is a CLUB-gameweek with two or more matches; a blank is a club\n")
	fmt.Printf("with none in a gameweek that was played. Counted from the fixture list,\n")
	fmt.Printf("not from player rows — a blank is exactly the case where no row exists.\n")

	for _, p := range pairs {
		played, count, teams := teamGameweeks(p.Cur.Fixtures)
		if len(played) == 0 {
			t.Errorf("%s: no fixtures carry a gameweek, so the calendar cannot be read",
				p.Name)
			continue
		}
		var weeks []calendarWeek
		var gws []int
		for gw := range played {
			gws = append(gws, gw)
		}
		sort.Ints(gws)
		var totD, totB int
		for _, gw := range gws {
			w := calendarWeek{gw: gw}
			for team := range teams {
				switch n := count[gw][team]; {
				case n == 0:
					w.blanking++
				case n >= 2:
					w.doubling++
					w.fixtures += n
				default:
					w.fixtures += n
				}
			}
			totD += w.doubling
			totB += w.blanking
			weeks = append(weeks, w)
		}

		fmt.Printf("\n%s — %d clubs, %d gameweeks with fixtures\n",
			p.Name, len(teams), len(gws))
		fmt.Printf("  double club-gameweeks %3d, blank club-gameweeks %3d\n", totD, totB)

		// Only the irregular weeks are worth printing: a season is mostly tens.
		fmt.Printf("  %-6s %8s %8s   %s\n", "gw", "doubling", "blanking", "half")
		var big, blank calendarWeek
		for _, w := range weeks {
			if w.doubling == 0 && w.blanking == 0 {
				continue
			}
			half := "first"
			if w.gw > 19 {
				half = "SECOND"
			}
			fmt.Printf("  %-6d %8d %8d   %s\n", w.gw, w.doubling, w.blanking, half)
			if w.doubling > big.doubling {
				big = w
			}
			if w.blanking > blank.blanking {
				blank = w
			}
		}
		switch {
		case big.doubling == 0:
			fmt.Printf("  no double gameweek at all: nothing to build toward.\n")
		default:
			fmt.Printf("  largest double: GW%d with %d clubs doubling.\n",
				big.gw, big.doubling)
		}
		switch {
		case blank.blanking == 0:
			fmt.Printf("  no blank gameweek at all: nothing to free hit.\n")
		default:
			fmt.Printf("  largest blank:  GW%d with %d clubs blanking.\n",
				blank.gw, blank.blanking)
		}
		if big.doubling > 0 && blank.blanking > 0 {
			fmt.Printf("  the pair sit %d gameweeks apart (blank GW%d, double GW%d).\n",
				big.gw-blank.gw, blank.gw, big.gw)
		}
	}

	fmt.Printf("\nWhat this cannot tell you: WHEN each double became knowable. The archive\n")
	fmt.Printf("carries no announcement dates, and the replay's fixture list is final from\n")
	fmt.Printf("GW1 where FPL's resolves as cup rounds finish. Any anchored chip plan\n")
	fmt.Printf("measured on this harness therefore has months of lead time nobody had, and\n")
	fmt.Printf("needs a point-in-time guard from outside this diagnostic before its number\n")
	fmt.Printf("means anything.\n")
}
