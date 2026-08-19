package backtest

import "armband/internal/analysis"

// minAnchorClubs is how many clubs a gameweek must involve before any rule here
// will treat it as an anchor worth spending a chip on.
//
// **Asserted, not measured**, and the lag columns are sensitive to it — so it is
// stated rather than buried. Without a bar at all, "the first week nothing better
// is visible ahead of" is satisfied by any local maximum however trivial, and the
// lag arms committed to two-club features in September: at 2023-24 from a GW1
// entry, every lag arm played its bench boost on a **two**-club double in GW7 and
// its free hit on a two-club blank in GW2, against full sight's seven-club double
// in GW34 and twelve-club blank in GW29. The reveal-lag columns would then have
// been measuring the absence of a size threshold rather than short sight, and the
// file's closing instruction — "if the value survives at four it is strategy" —
// would have read a strawman as a verdict.
//
// Four is the floor of the range the arms are choosing among rather than a tuned
// value: the full-sight anchors across the five archived seasons involve 4 to 12
// clubs. A two-club double reaches at most two of a fifteen and, in expectation,
// about one and a half — which no manager holds a chip for.
const minAnchorClubs = 4

// PlacedChips is how many of a plan's four chips have a week. A full plan is
// 4; a short one means the window could not hold every chip, and a caller that
// promised the season's full grant must refuse it rather than play fewer.
func PlacedChips(p analysis.ChipPlan) int {
	n := 0
	for _, gw := range []int{p.Wildcard, p.BenchBoost, p.FreeHit, p.TripleCaptain} {
		if gw > 0 {
			n++
		}
	}
	return n
}

// Full anchored chip plans: the complete plan the user-facing replay and the
// shipped configuration run — BOTH sets, all four chips, anchored on the
// calendar within each set's window, with use-it-or-lose-it fallbacks.
//
// This is deliberately separate from `anchoredPlan` in anchored_diag_test.go,
// which is the measurement corner: that planner is masked by `matchedChips`
// (never placing a wildcard, and dropping the triple captain when the calendar
// cannot fit it for every arm) and its banked results were measured with that
// mask. This one is the functional plan a manager actually plays: every chip
// the season grants is placed, in the set that grants it, and no chip is left
// unplayed when its set's window has no qualifying week — FPL expires the set
// and an unplayed chip scores nothing at all.
//
// # The rules, stated so the plan is a plan rather than a search
//
//   - Each set's window: [start+1, 19] for the first set of a two-set season,
//     [20, 38] for the second, [start+1, 38] for a one-set season.
//   - Bench boost: the window's biggest double (most doubling clubs, ties to
//     the earliest week), bar `minAnchorClubs`; fallback the set's last week.
//   - Triple captain: the window's second-biggest double (distinct week);
//     fallback the week before the set's end.
//   - Free hit: the window's biggest blank; fallback two weeks before the end.
//   - Wildcard: GW4 for the first set — the record's one reliable wildcard
//     week, the opening fifteen being built on the season's weakest
//     information — clamped into the window; for the second set, the week
//     before the set's bench boost (it prepares the boost), else the reset
//     week.
//   - One chip per gameweek: picks are taken in the order above, and a later
//     pick that collides shifts down until the week is free.
//
// A window with no qualifying week — 2025-26's first half holds no doubles or
// blanks at all — still plays every chip, on the fallbacks. That is the
// use-it-or-lose-it rule: the set expires and the chip is simply lost if not
// played.
func FullAnchoredPlan(cur *Season, start int) analysis.ChipSchedule {
	played, count, teams := teamGameweeks(cur.Fixtures)
	if len(teams) == 0 {
		return analysis.ChipSchedule{}
	}
	if start < 0 {
		start = 0
	}
	sets := ChipSetsFor(cur.Name)

	clubs := func(gw int, want func(int) bool) int {
		n := 0
		for team := range teams {
			if want(count[gw][team]) {
				n++
			}
		}
		return n
	}
	isDouble := func(n int) bool { return n >= 2 }
	isBlank := func(n int) bool { return n == 0 }

	// anchor returns the window's biggest week for `want`, excluding `taken`,
	// ties to the earliest; 0 when nothing meets the bar.
	anchor := func(lo, hi int, want func(int) bool, taken map[int]bool) int {
		best, bestGW := 0, 0
		for gw := lo; gw <= hi; gw++ {
			if !played[gw] || taken[gw] {
				continue
			}
			if n := clubs(gw, want); n >= minAnchorClubs && n > best {
				best, bestGW = n, gw
			}
		}
		return bestGW
	}

	// claim marks a week taken, searching for the nearest free week — down
	// first, then up — and reports false when the window is full. A pick
	// cannot land at or below the entry point: the window's floor is start+1,
	// so a full window is the honest failure rather than a collision that
	// silently reuses a week.
	claim := func(taken map[int]bool, gw, lo, hi int) (int, bool) {
		for i := gw; i >= lo; i-- {
			if !taken[i] {
				taken[i] = true
				return i, true
			}
		}
		for i := gw + 1; i <= hi; i++ {
			if !taken[i] {
				taken[i] = true
				return i, true
			}
		}
		return 0, false
	}

	var sch analysis.ChipSchedule
	for set := 1; set <= sets; set++ {
		// The window is entry-aware: chips are only worth planning in weeks
		// the replay will actually play, so a late entry gets the weeks it
		// has left rather than a plan it can never reach.
		lo, hi := start+1, 38
		if sets == 2 && set == 1 {
			hi = ChipResetGW - 1
		} else if sets == 2 {
			lo = ChipResetGW
			if start+1 > lo {
				lo = start + 1
			}
		}
		if lo < 1 {
			lo = 1
		}
		if lo > hi {
			continue
		}
		taken := map[int]bool{}
		ok := true
		bb := anchor(lo, hi, isDouble, taken)
		if bb == 0 {
			bb = hi
		}
		bb, ok = claim(taken, bb, lo, hi)
		if !ok {
			bb = 0
		}
		tc := anchor(lo, hi, isDouble, taken)
		if tc == 0 {
			tc = hi - 1
			if tc < lo {
				tc = lo
			}
		}
		if ok {
			tc, ok = claim(taken, tc, lo, hi)
		}
		if !ok {
			tc = 0
		}
		fh := anchor(lo, hi, isBlank, taken)
		if fh == 0 {
			fh = hi - 2
			if fh < lo {
				fh = lo
			}
		}
		if ok {
			fh, ok = claim(taken, fh, lo, hi)
		}
		if !ok {
			fh = 0
		}
		wc := 0
		if ok {
			switch {
			case sets == 2 && set == 2:
				wc = bb - 1 // prepares the boost
				if wc < lo {
					wc = lo
				}
			default:
				wc = 4 // the recorded one reliable wildcard week
				if wc <= start {
					wc = start + 1
				}
			}
			wc, ok = claim(taken, wc, lo, hi)
			if !ok {
				wc = 0
			}
		}

		p := analysis.ChipPlan{
			Wildcard: wc, BenchBoost: bb, FreeHit: fh, TripleCaptain: tc,
		}
		if set == 1 {
			sch.First = p
		} else {
			sch.Second = p
		}
	}
	return sch
}
