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
// ⚠️ **"Every cell contains a double" is a necessary condition and licenses
// nothing on its own.** The dose is what matters and it is wildly unequal — read
// the per-cell columns and the season totals, not the live-cell count. Cells
// within a season are nested and share nearly all their doubles, so the
// entry-point axis varies the *state* the doubles are met in rather than the
// treatment. A doubles arm is a season-clustered design first and a 36-cell one
// second, and the two readings differ by a lot.
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
// # Three windows, and they are not the same window
//
// A cell entering at GW n **plays** [n, 38] — `Simulate`'s loop is
// `for gw := start; gw <= 38`. It can **act** only on [n+1, 38], because the
// opening fifteen is chosen at the entry deadline, so a double in the entry week
// is football the cell scores and no transfer can be banked into. And a **chip**
// reaches [n+1, 38] gated by `minAnchorClubs`, a bar the two planners apply that
// nothing else does.
//
// All three are reported, because they answer different questions and the middle
// one is the dose to weight or regress on. Conflating them credits the mechanism
// with weeks it could not have acted on.
//
// ⚠️ The [n+1, 38] restriction is a property of `sightedWeeks` and `findAnchors`,
// which both require `gw > start` — **not** of the harness. `Simulate`'s
// scoring-chip switch and its free-hit build sit outside its `if gw > start`
// block, so a plan naming the entry week itself would be played.
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
	"sort"
	"testing"

	"armband/internal/analysis"
)

// censusRow is one (season, entry gameweek) cell.
type censusRow struct {
	season string
	start  int

	// gameweeks is the WIDTH of the window, 38-start+1, which is the record's
	// 38/33/28/23/18/13 convention. It is not the number of rounds carrying
	// football: 2022-23 has an empty round, so at a GW1 entry this reads 38 while
	// only 37 were played. The `empty` column is what corrects it, and anything
	// using this as a denominator on that season is about 2.7% low.
	gameweeks int

	// Counted over the PLAYED window [start, 38] — the football the cell scores.
	doubleRounds, doubleClubGWs int
	blankRounds, blankClubGWs   int
	emptyRounds                 int

	// Counted over the ACTIONABLE window [start+1, 38], ungated by any bar.
	//
	// This is the dose a policy can actually respond to, and it is the column to
	// weight or regress on rather than `doubleClubGWs`. A double in the entry week
	// itself is football the cell scores and no transfer can ever be banked into,
	// because the opening fifteen is chosen at the entry deadline — so counting it
	// as dose credits the mechanism with a week it could not have acted on.
	actionableDoubleClubGWs int
	actionableBlankClubGWs  int

	// The same window narrowed again, to what falls BEYOND the opening squad's
	// own horizon — the dose the transfer policy can actually add, since
	// everything inside [start+1, start+H-1] was already visible to the squad
	// build. Both pairs come from `DoseFor`, which the cells file's `dose_*`
	// columns also read, so the census checks the shipped quantity rather than
	// restating it.
	lateDoubleClubGWs int
	lateBlankClubGWs  int

	// Counted over the same window and further gated on minAnchorClubs — the bar
	// `sightedWeeks` applies before it will spend a chip on a week. A cell can be
	// live for football, live for transfers, and dead for the chip axis.
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
// It reads `teamGameweeks` for the per-club match counts, which is the safe side
// to count from. It does re-spell the *classification* — zero is a blank, two or
// more is a double — and that expression now exists four times in this package:
// here, in `calendarWeek`, in `findAnchors` and inside `sightedWeeks`. Three of
// those are deliberate, because `TestAnchoredPlanSitsOnTheCalendarMaxima` and
// this file's Part 3 both work by cross-checking one against another; `censusOf`
// against `calendarWeek` is not, and `censusOf` strictly supersedes it — it adds
// `played` and gets the fixture count right. Collapsing `TestDiagFixtureCalendar`
// onto this function is the right follow-up and is left out of this change
// because it rewrites another diagnostic's output. Neither
// `TestTheCopiedExpressionsHaveOneImplementation` nor
// `TestTheSharedCellQuantitiesHaveOneImplementation` matches this idiom, so
// nothing will catch a fifth copy.
//
// # blanking is zero on an unplayed round, not twenty
//
// `count[gw]` is nil for a round with no fixtures, so the natural loop reads
// every club as blanking and reports 2022-23 GW7 as a twenty-club blank — the
// exact misreading this file's header rejects. Parts 1 and 2 skip unplayed rounds
// before reading the field, so they were already safe; Part 3 looks a chip's week
// up directly and was not. Fixing it at the source rather than at each reader is
// what stops the next reader inheriting the trap.
func censusOf(s *Season) []weekCensus {
	played, count, teams := teamGameweeks(s.Fixtures)
	out := make([]weekCensus, 0, 38)
	for gw := 1; gw <= 38; gw++ {
		w := weekCensus{gw: gw, played: played[gw]}
		if !w.played {
			out = append(out, w)
			continue
		}
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
	requireDiag(t)
	cfg := loadConfig(t)
	pairs := loadPairs(t, cfg)
	starts := sweepStarts()

	fmt.Printf("\n=== blanks and doubles, per season and per cell\n")
	fmt.Printf("%s\n", gridLabel(len(pairs), len(starts)))
	fmt.Printf("Counted from the fixture list via teamGameweeks, never from player rows:\n")
	fmt.Printf("a blank is the case where no row exists, and a real double has the same\n")
	fmt.Printf("shape as the archive's duplicate rows. No repair switch bears on any of it.\n")
	fmt.Printf("A cell entering at GW n PLAYS [n,38] but can ACT only on [n+1,38], and a\n")
	fmt.Printf("chip reaches [n+1,38] gated by minAnchorClubs = %d. Three windows, three\n",
		minAnchorClubs)
	fmt.Printf("different questions; the middle one is the dose to weight or regress on.\n")

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

	fmt.Printf("\n--- per cell\n")
	fmt.Printf("D/B-clubgw count the PLAYED window [start,38] — the football the cell scores.\n")
	fmt.Printf("act-D/act-B count the ACTIONABLE window [start+1,38] ungated by any bar, and\n")
	fmt.Printf("this is the dose to weight or regress on: a double in the entry week cannot\n")
	fmt.Printf("be transferred into, because the opening fifteen is chosen at that deadline.\n")
	fmt.Printf("anch-D/anch-B are the same window at or above minAnchorClubs — the chip axis.\n")
	fmt.Printf("late-D/late-B narrow the actionable window again, to [start+H, 38] —\n")
	fmt.Printf("what falls BEYOND the opening squad's own horizon, which is the dose the\n")
	fmt.Printf("transfer policy can add rather than inherit. act-* and late-* both come\n")
	fmt.Printf("from DoseFor, the same function the cells file's dose_* columns read.\n")
	fmt.Printf("⚠️  92%% of doubles fall after GW19, so dose and denominator move together;\n")
	fmt.Printf("and these cells carry far fewer distinct doses than rows. Do not regress\n")
	fmt.Printf("on either column without putting entry gameweek in the model.\n")
	fmt.Printf("%-9s %-6s %4s | %6s %8s | %6s %8s | %5s | %6s %6s | %6s %6s | %6s %6s\n",
		"season", "entry", "gws", "D-rnds", "D-clubgw", "B-rnds", "B-clubgw",
		"empty", "act-D", "act-B", "anch-D", "anch-B", "late-D", "late-B")

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
				if w.gw > start {
					if w.doubling >= minAnchorClubs {
						r.anchorableDoubleRounds++
					}
					if w.blanking >= minAnchorClubs {
						r.anchorableBlankRounds++
					}
				}
			}
			// ⚠️ The actionable dose comes from `DoseFor` rather than being
			// counted again here. It was a second implementation of the same
			// window the moment the cells file grew a `dose_act_*` column, and
			// the two would have agreed on the day they were written — which is
			// exactly the drift this package's scans exist for. The census now
			// checks the shipped one rather than restating it.
			d := DoseFor(p.Cur, start, cfg.Weights.Horizon)
			r.actionableDoubleClubGWs = d.ActDoubles
			r.actionableBlankClubGWs = d.ActBlanks
			r.lateDoubleClubGWs = d.LateDoubles
			r.lateBlankClubGWs = d.LateBlanks
			rows = append(rows, r)
			fmt.Printf("%-9s GW%-4d %4d | %6d %8d | %6d %8d | %5d | %6d %6d | %6d %6d | %6d %6d\n",
				r.season, r.start, r.gameweeks,
				r.doubleRounds, r.doubleClubGWs,
				r.blankRounds, r.blankClubGWs,
				r.emptyRounds,
				r.actionableDoubleClubGWs, r.actionableBlankClubGWs,
				r.anchorableDoubleRounds, r.anchorableBlankRounds,
				r.lateDoubleClubGWs, r.lateBlankClubGWs)
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

	type placement struct{ placed, onFeature, unplayed int }
	var anchoredBB, anchoredFH, anchoredTC placement
	var controlBB, controlFH, controlTC placement
	rejected := map[string]int{}
	rejectExample := map[string]string{}

	// `!` marks a chip placed on a round that carries no fixture at all. It is not
	// a decoration: `controlWeeks` has no `played` gate, so at a 2022-23 GW1 entry
	// the control bench boost lands on GW7 — the voided round. Without the marker
	// that prints as "0", indistinguishable from an ordinary week where nobody
	// doubles, and the control arm's confound is invisible.
	describe := func(weeks []weekCensus, gw int, double bool) string {
		if gw == 0 {
			return "   -   "
		}
		n, played := 0, false
		for _, w := range weeks {
			if w.gw == gw {
				played = w.played
				if double {
					n = w.doubling
				} else {
					n = w.blanking
				}
			}
		}
		flag := " "
		if !played {
			flag = "!"
		}
		return fmt.Sprintf("%2d(%2d)%s", gw, n, flag)
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
			if !w.played {
				p.unplayed++
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

			// Would the harness actually replay this plan?
			//
			// `Simulate` calls `ValidateChipSets` before its first gameweek, and
			// `runPolicySweep` records a rejection as an INFEASIBLE cell rather
			// than fatalling — so an arm can quietly lose cells while every
			// printed number stays plausible. That is the failure this file's
			// header says it exists to prevent, and Part 3 was committing it: it
			// printed plans as good placements without ever asking whether they
			// were playable.
			//
			// It fires today, on the two-set rule. `ChipSetsFor("2025-26")` is 2,
			// and a FIRST-set chip at or after `ChipResetGW` is refused — but
			// `sightedWeeks` and `controlWeeks` know nothing about the reset, and
			// 2025-26's only anchors are GW33 and GW34. The defect belongs to
			// those two planners rather than to this census, which is why this
			// counts and reports rather than failing: making the census red would
			// invite deleting the check instead of fixing the planners.
			for _, v := range []struct {
				arm  string
				plan analysis.ChipPlan
			}{{"anchored", a}, {"control", c}} {
				// ✅ **The defect this counted is REPAIRED**, and the check stays,
				// because a repair with no check is how the next one goes
				// unnoticed. `Simulate` now routes a planner's output through
				// `SplitChipSets` before validating it, so a late chip in a
				// two-set season lands in the SECOND set instead of being
				// refused — and this asks the same question of the same plan by
				// the same route. A non-zero count now means either the repair
				// has regressed, or a planner has found a way to be illegal that
				// the split deliberately does not hide: two chips in one week.
				sch := SplitChipSets(p.Name, v.plan)
				if err := ValidateChipSets(p.Name, sch.First, sch.Second); err != nil {
					rejected[v.arm]++
					rejectExample[v.arm] = fmt.Sprintf("%s GW%d: %v", p.Name, start, err)
				}
			}

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

			// Two static tripwires, and they are NOT the liveness half of the
			// pairing — an earlier version of this comment claimed they were and
			// it was wrong. Both are guaranteed to pass by construction:
			// `sightedWeeks`' `place` already returns on `gw <= start`, and its
			// `firstUnbeaten` already skips a week below the bar, off the same
			// `teamGameweeks` call this reads. So they can only fail if a later
			// edit removes one of those, which makes them a second CONFINEMENT
			// check. Worth keeping as tripwires, worth not mislabelling.
			//
			// The check with real power is the one below the loop, which asks
			// whether `Simulate` would accept these plans at all. That one moves,
			// and it fails today.
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
	fmt.Printf("\n%-24s %9s %16s %10s\n",
		"chip", "placed", "on the feature", "unplayed")
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
		fmt.Printf("%-24s %5d/%-3d %12d/%-3d %10d\n",
			l.name, l.p.placed, n, l.p.onFeature, l.p.placed, l.p.unplayed)
	}
	fmt.Printf("\n\"on the feature\" is 100%% for every anchored arm BY CONSTRUCTION —\n")
	fmt.Printf("minAnchorClubs guarantees it. The informative column is the control's.\n")
	fmt.Printf("\"unplayed\" counts chips burned on a round carrying no fixture at all.\n")

	fmt.Printf("\nplans ValidateChipSets would REFUSE, so Simulate records the cell\n")
	fmt.Printf("as infeasible rather than replaying it:\n")
	for _, arm := range []string{"anchored", "control"} {
		fmt.Printf("  %-9s %2d of %d cells", arm, rejected[arm], n)
		if rejected[arm] > 0 {
			fmt.Printf("   e.g. %s", rejectExample[arm])
		}
		fmt.Printf("\n")
	}
	fmt.Printf("A rejected cell is a comparison that never ran. Size an anchored-chips\n")
	fmt.Printf("arm on the surviving count, never on %d.\n", n)

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
