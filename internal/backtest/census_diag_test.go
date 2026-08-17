package backtest

// A census of the fixture calendar, per season and per REPLAY CELL, and an audit
// of where the anchored chip plan actually lands.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBlanksAndDoublesCensus -v
//
// # Why a per-cell count and not just a per-season one
//
// `TestDiagFixtureCalendar` already prints the per-season calendar, and it is the
// right instrument for "what did this season's fixture list look like". It cannot
// answer the question a doubles-and-blanks experiment actually turns on, because
// it has no entry-point axis: a cell entering at GW26 plays thirteen gameweeks,
// and every double before GW26 is invisible to it.
//
// That matters because of the rule this package keeps paying for. **A cell that
// contains no double cannot express a doubles mechanism**, so a null from it is a
// comparison that never ran rather than evidence of anything — and a power
// calculation assuming all 36 cells are live would then be wrong in the
// optimistic direction. This diagnostic is what makes that checkable before a
// sweep is spent rather than after.
//
// # What is counted, and what it is keyed on
//
// Everything here is read off the **fixture list** through `teamGameweeks`, never
// off player rows. Two reasons, and they pull in opposite directions:
//
//   - A blank is exactly the case where no player row exists, so counting rows
//     would make a blank invisible.
//   - A real double gameweek has the identical shape to the archive's duplicate
//     rows — the defect `rowGuardReport` documents — so a count keyed on
//     `(element, gameweek)` would conflate them. `loadFixtures` is upstream of
//     both row guards and cannot be affected by either, which is what makes the
//     fixture list the safe side to count from.
//
// # An empty round is not twenty clubs blanking
//
// 2022-23 has no fixture at all filed under GW7 — the round postponed after the
// Queen's death, whose matches were redistributed into later events. The naive
// reading is "twenty clubs blank", and it is wrong twice over: no manager faced
// that week as a blank to plan around, and `teamGameweeks` does not mark the
// round as played at all, so no chip rule can anchor to it. It is counted and
// reported separately, and excluded from the blank totals.
//
// # Two windows, and they are not the same window
//
// A cell entering at GW n **plays** [n, 38] — `Simulate`'s loop is
// `for gw := start; gw <= 38`. A chip may only be placed in [n+1, 38], because
// `sightedWeeks` and `findAnchors` both require `gw > start`. So a double in the
// entry week itself is football the cell scores and a chip can never be spent on.
// Both windows are reported; conflating them overstates what the chip axis can
// reach.
//
// # Data state
//
// The counts come from `Season.Fixtures`, populated by `loadFixtures` from the
// archive's `fixtures.csv` with 2019-20's restart rounds renumbered into 1..38.
// **No repair switch bears on any figure here.** `FPL_NO_XG_REPAIR`,
// `FPL_NO_XGC_REPAIR`, `FPL_NO_XG_AGGREGATE` and `FPL_NO_STARTS_REPAIR` all act on
// per-player rate columns; none of them adds, removes or re-labels a fixture. The
// two row guards drop `merged_gw.csv` rows and likewise never touch the fixture
// list. `FPL_SWEEP_SEASONS` and `FPL_SWEEP_STARTS` do change which cells are
// counted, which is why the grid is printed rather than assumed.

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"armband/internal/analysis"
)

// censusRow is one (season, entry gameweek) cell.
type censusRow struct {
	season string
	start  int

	gameweeks int // rounds the cell plays, [start, 38]

	// Counted over the PLAYED window [start, 38].
	doubleRounds, doubleClubGWs int
	blankRounds, blankClubGWs   int
	emptyRounds                 int

	// Counted over the CHIP-ELIGIBLE window [start+1, 38], and further gated on
	// minAnchorClubs — the bar `sightedWeeks` applies before it will spend a chip
	// on a week. A cell can be live for football and dead for the chip axis.
	anchorableDoubleRounds int
	anchorableBlankRounds  int
}

func (r censusRow) hasDouble() bool { return r.doubleRounds > 0 }
func (r censusRow) hasBlank() bool  { return r.blankRounds > 0 }

// weekCensus is one gameweek of one season.
type weekCensus struct {
	gw                 int
	fixtures           int
	doubling, blanking int
	played             bool
}

// censusOf reads a season's calendar into one row per gameweek.
//
// It is `teamGameweeks` plus arithmetic and holds no second copy of the counting
// rule, which is the point: `findAnchors`, `sightedWeeks` and
// `TestDiagFixtureCalendar` all read the same function, so a census that
// disagreed with the placement rule would be a census of a calendar no chip rule
// can see.
func censusOf(s *Season) []weekCensus {
	played, count, teams := teamGameweeks(s.Fixtures)
	out := make([]weekCensus, 0, 38)
	for gw := 1; gw <= 38; gw++ {
		w := weekCensus{gw: gw, played: played[gw]}
		for team := range teams {
			switch n := count[gw][team]; {
			case n == 0:
				w.blanking++
			case n >= 2:
				w.doubling++
			}
			w.fixtures += count[gw][team]
		}
		w.fixtures /= 2
		out = append(out, w)
	}
	return out
}

func TestDiagBlanksAndDoublesCensus(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== blanks and doubles, per season and per cell\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("Counted from the fixture list via teamGameweeks, never from player rows:\n")
	fmt.Printf("a blank is the case where no row exists, and a real double has the same\n")
	fmt.Printf("shape as the archive's duplicate rows. No repair switch bears on any of it.\n")
	fmt.Printf("A cell entering at GW n plays [n,38]; a chip may only be placed in [n+1,38].\n")
	fmt.Printf("minAnchorClubs = %d is the bar sightedWeeks applies before spending a chip.\n",
		minAnchorClubs)

	// ---- Part 1: the calendar, per season ----

	fmt.Printf("\n--- the rounds themselves\n")
	fmt.Printf("%-9s %-4s %5s %8s %8s\n", "season", "gw", "fixt", "doubling", "blanking")
	seasonWeeks := map[string][]weekCensus{}
	for _, p := range pairs {
		weeks := censusOf(p.Cur)
		seasonWeeks[p.Name] = weeks

		// Every club plays 38 matches, and the whole of a season's doubling is
		// paid for by its blanking. This is the arithmetic identity that makes
		// the census self-checking: a fixture list that lost or gained a match
		// breaks it, and a count keyed on the wrong thing breaks it too.
		_, count, teams := teamGameweeks(p.Cur.Fixtures)
		for team := range teams {
			total := 0
			for gw := 1; gw <= 38; gw++ {
				total += count[gw][team]
			}
			if total != 38 {
				t.Errorf("%s: club %d plays %d matches, want 38 — the fixture list "+
					"is not a whole season, so every count below is of something else",
					p.Name, team, total)
			}
		}

		for _, w := range weeks {
			if !w.played {
				fmt.Printf("%-9s %-4d %5d %8s %8s   round not played at all\n",
					p.Name, w.gw, 0, "-", "-")
				continue
			}
			if w.doubling == 0 && w.blanking == 0 {
				continue
			}
			fmt.Printf("%-9s %-4d %5d %8d %8d\n",
				p.Name, w.gw, w.fixtures, w.doubling, w.blanking)
		}
	}

	// ---- Part 2: per cell ----

	fmt.Printf("\n--- per cell, over the window the cell plays\n")
	fmt.Printf("anchorable counts the chip-eligible window [start+1,38] at or above the bar.\n")
	fmt.Printf("%-9s %-6s %4s | %7s %8s | %7s %8s | %6s | %10s %10s\n",
		"season", "entry", "gws", "D-rnds", "D-clubgw", "B-rnds", "B-clubgw",
		"empty", "anchD-rnds", "anchB-rnds")

	var rows []censusRow
	for _, p := range pairs {
		weeks := seasonWeeks[p.Name]
		for _, start := range starts {
			r := censusRow{season: p.Name, start: start, gameweeks: 38 - start + 1}
			for _, w := range weeks {
				if w.gw < start {
					continue
				}
				if !w.played {
					r.emptyRounds++
					continue
				}
				if w.doubling > 0 {
					r.doubleRounds++
					r.doubleClubGWs += w.doubling
				}
				if w.blanking > 0 {
					r.blankRounds++
					r.blankClubGWs += w.blanking
				}
				if w.gw > start && w.doubling >= minAnchorClubs {
					r.anchorableDoubleRounds++
				}
				if w.gw > start && w.blanking >= minAnchorClubs {
					r.anchorableBlankRounds++
				}
			}
			rows = append(rows, r)
			fmt.Printf("%-9s GW%-4d %4d | %7d %8d | %7d %8d | %6d | %10d %10d\n",
				r.season, r.start, r.gameweeks,
				r.doubleRounds, r.doubleClubGWs,
				r.blankRounds, r.blankClubGWs,
				r.emptyRounds,
				r.anchorableDoubleRounds, r.anchorableBlankRounds)
		}
	}

	liveD, liveB, liveBoth, anchD, anchB := 0, 0, 0, 0, 0
	for _, r := range rows {
		if r.hasDouble() {
			liveD++
		}
		if r.hasBlank() {
			liveB++
		}
		if r.hasDouble() && r.hasBlank() {
			liveBoth++
		}
		if r.anchorableDoubleRounds > 0 {
			anchD++
		}
		if r.anchorableBlankRounds > 0 {
			anchB++
		}
	}
	fmt.Printf("\ncells containing at least one double : %d of %d\n", liveD, len(rows))
	fmt.Printf("cells containing at least one blank  : %d of %d\n", liveB, len(rows))
	fmt.Printf("cells containing both                : %d of %d\n", liveBoth, len(rows))
	fmt.Printf("cells a chip could anchor to a double: %d of %d\n", anchD, len(rows))
	fmt.Printf("cells a chip could anchor to a blank : %d of %d\n", anchB, len(rows))

	// A census that finds nothing is indistinguishable from a census that did not
	// run, which is this package's signature failure. The archive's calendars are
	// finished history and every one of them carries doubles, so a zero here is a
	// broken instrument rather than a quiet season.
	if liveD == 0 || liveB == 0 {
		t.Errorf("no cell contains a double (%d) or none contains a blank (%d): the "+
			"calendar cannot be read, because every archived season has both",
			liveD, liveB)
	}

	// ---- Part 3: where the anchored plan actually puts the chips ----

	fmt.Printf("\n--- where the anchored plan lands, and on what\n")
	fmt.Printf("anchoredPlan is sightedWeeks at full sight, masked by matchedChips —\n")
	fmt.Printf("the intersection of what EVERY arm of the anchored sweep can place.\n")
	fmt.Printf("controlPlan is the same chips at fixed offsets from entry (%d/%d/%d).\n",
		controlOffsets.benchBoost, controlOffsets.freeHit, controlOffsets.tripleCaptain)
	fmt.Printf("%-9s %-6s | %-22s | %-22s\n", "season", "entry",
		"anchored bb/fh/tc", "control bb/fh/tc")

	type placement struct{ placed, onFeature int }
	var anchoredBB, anchoredFH, anchoredTC placement
	var controlBB, controlFH, controlTC placement

	describe := func(weeks []weekCensus, gw int, double bool) string {
		if gw == 0 {
			return "  -   "
		}
		n := 0
		for _, w := range weeks {
			if w.gw == gw {
				if double {
					n = w.doubling
				} else {
					n = w.blanking
				}
			}
		}
		return fmt.Sprintf("%2d(%2d)", gw, n)
	}
	tally := func(weeks []weekCensus, gw int, double bool, p *placement) {
		if gw == 0 {
			return
		}
		p.placed++
		for _, w := range weeks {
			if w.gw != gw {
				continue
			}
			n := w.blanking
			if double {
				n = w.doubling
			}
			if n > 0 {
				p.onFeature++
			}
		}
	}

	for _, p := range pairs {
		weeks := seasonWeeks[p.Name]
		for _, start := range starts {
			a := anchoredPlan(p.Cur, start)
			c := controlPlan(p.Cur, start)

			tally(weeks, a.BenchBoost, true, &anchoredBB)
			tally(weeks, a.FreeHit, false, &anchoredFH)
			tally(weeks, a.TripleCaptain, true, &anchoredTC)
			tally(weeks, c.BenchBoost, true, &controlBB)
			tally(weeks, c.FreeHit, false, &controlFH)
			tally(weeks, c.TripleCaptain, true, &controlTC)

			fmt.Printf("%-9s GW%-4d | %s %s %s | %s %s %s\n",
				p.Name, start,
				describe(weeks, a.BenchBoost, true),
				describe(weeks, a.FreeHit, false),
				describe(weeks, a.TripleCaptain, true),
				describe(weeks, c.BenchBoost, true),
				describe(weeks, c.FreeHit, false),
				describe(weeks, c.TripleCaptain, true))

			// The liveness half of the confinement pairing. A bench boost the
			// anchored rule places must sit on a week carrying at least
			// minAnchorClubs doubling clubs — that is what sightedWeeks' bar
			// asserts, and checking it here is what would catch the bar being
			// bypassed by a later edit. Confinement alone ("no chip moved") can
			// only fail; this must move and does.
			for _, chk := range []struct {
				name   string
				gw     int
				double bool
			}{
				{"bench boost", a.BenchBoost, true},
				{"free hit", a.FreeHit, false},
				{"triple captain", a.TripleCaptain, true},
			} {
				if chk.gw == 0 {
					continue
				}
				if chk.gw <= start {
					t.Errorf("%s GW%d: anchored %s placed at GW%d, at or before entry",
						p.Name, start, chk.name, chk.gw)
				}
				for _, w := range weeks {
					if w.gw != chk.gw {
						continue
					}
					n := w.blanking
					if chk.double {
						n = w.doubling
					}
					if n < minAnchorClubs {
						t.Errorf("%s GW%d: anchored %s sits on GW%d with %d clubs, "+
							"below minAnchorClubs %d — the bar sightedWeeks asserts "+
							"is not being applied",
							p.Name, start, chk.name, chk.gw, n, minAnchorClubs)
					}
				}
			}
		}
	}

	n := len(rows)
	fmt.Printf("\n%-22s %8s %14s\n", "chip", "placed", "on the feature")
	for _, l := range []struct {
		name string
		p    placement
	}{
		{"anchored bench boost", anchoredBB},
		{"anchored free hit", anchoredFH},
		{"anchored triple captain", anchoredTC},
		{"control bench boost", controlBB},
		{"control free hit", controlFH},
		{"control triple captain", controlTC},
	} {
		fmt.Printf("%-22s %5d/%-3d %11d/%-3d\n",
			l.name, l.p.placed, n, l.p.onFeature, l.p.placed)
	}

	// ---- Part 4: the baseline sweep plays no chips at all ----
	//
	// Stated in the output rather than left to be rediscovered. `sweepConfig`
	// sets no Chips, no Chips2 and no ChipPlanner, so every constant sweep and
	// every transfer-policy sweep in this package runs with four chips unplayed.
	// Chip weeks exist only in the diagnostics that install a planner.
	sc := sweepConfig(cfg, 1, false)
	if sc.Chips != (analysis.ChipPlan{}) || sc.Chips2 != (analysis.ChipPlan{}) ||
		sc.ChipPlanner != nil {
		t.Errorf("sweepConfig now carries a chip plan (%v/%v/planner=%v); the note "+
			"below is stale and the per-cell chip audit above describes the wrong "+
			"population", sc.Chips, sc.Chips2, sc.ChipPlanner != nil)
	}
	fmt.Printf("\nsweepConfig installs no chip plan: the ordinary paired sweep plays\n")
	fmt.Printf("zero chips in %d of %d cells. Chip weeks are a property of the chip\n", n, n)
	fmt.Printf("diagnostics alone, not of the harness.\n")

	// ---- Part 5: the record's own first-half figure, recomputed ----
	//
	// AGENTS.md records "the first half of the season holds 15 of 189 doubling
	// club-gameweeks — and 11 of the 15 are one COVID-rescheduled 2020-21 round —
	// while 2025-26, the only two-set season, holds none". That claim is what
	// `ChipResetGW` rests on, so it is recomputed here rather than trusted.
	fmt.Printf("\n--- the two-set claim, recomputed\n")
	firstHalf, whole := 0, 0
	perSeasonFirstHalf := map[string]int{}
	for _, p := range pairs {
		for _, w := range seasonWeeks[p.Name] {
			whole += w.doubling
			if w.gw <= ChipResetGW-1 {
				firstHalf += w.doubling
				perSeasonFirstHalf[p.Name] += w.doubling
			}
		}
	}
	names := make([]string, 0, len(perSeasonFirstHalf))
	for k := range perSeasonFirstHalf {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Printf("doubling club-gameweeks: %d in GW1-%d, %d over the whole grid\n",
		firstHalf, ChipResetGW-1, whole)
	for _, k := range names {
		fmt.Printf("  %-9s %3d in the first half\n", k, perSeasonFirstHalf[k])
	}
}
