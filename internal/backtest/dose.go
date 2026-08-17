package backtest

// The per-cell fixture dose: how much doubling and blanking a replay cell could
// actually act on.
//
// # Why a dose at all
//
// The 36 cells of a sweep are not exchangeable with respect to a doubles or
// blanks treatment. A cell entering at GW26 sits close to its doubles, so the
// opening fifteen already captures them and there is less left for the transfer
// policy to add; a cell entering at GW1 has the whole calendar ahead of it and
// almost none of it visible to the squad build. Pooling 36 paired differences and
// calling the spread noise penalises the comparison for structure that can be
// modelled instead.
//
// So the dose is emitted per cell, as a covariate. ⚠️ **Nothing here fits a
// slope**, and a dose-response is a separate act needing its own pre-registration
// against the three traps below.
//
// # Three traps, and any one of them manufactures a slope
//
//   - **92% of doubles fall after GW19.** A late-entry cell therefore has more
//     dose per gameweek AND fewer gameweeks, so dose and denominator move
//     together. A per-gameweek outcome regressed on dose picks up the entry point
//     rather than the doubles unless entry gameweek is in the model.
//   - **The 36 cells collapse to about 14 distinct doses**, with effective seasons
//     around 4.4, because cells within a season are nested and share nearly all
//     their doubles. A standard error computed as though 36 rows were independent
//     is wrong by roughly that ratio.
//   - **The dose is HINDSIGHT.** It reads `cur.Fixtures` in full, so it knows every
//     double from GW1 where a real manager learns of one as cup rounds resolve —
//     realistically two to six gameweeks ahead. This record already flags the same
//     leak on `fixtureLoadFor` and calls its `+33` optimistic for it. As a
//     *covariate* that is defensible and is why the columns are computed this way:
//     the dose is identical across the arms of a cell, so it cannot flatter one arm
//     over another, and a covariate is allowed to know things the policy does not.
//     It is **not** defensible as a description of what a manager could have
//     planned for, and a dose-response quoted as "a policy that targets doubles is
//     worth X per double" would be making exactly that claim. Nothing derives an
//     announcement-lag-corrected dose, and the archive carries no announcement
//     dates to derive one from.
//
// # Counted from the fixture list, never from player rows
//
// Two reasons pulling in opposite directions, both recorded by the census this
// shares its arithmetic with: a blank is exactly the case where no player row
// exists, so counting rows would make a blank invisible; and a real double
// gameweek has the identical shape to the archive's duplicate rows, so a count
// keyed on `(element, gameweek)` would conflate them. `loadFixtures` is upstream
// of both row guards and cannot be affected by either.

// FixtureDose is one cell's actionable fixture load, in club-gameweeks.
//
// Exported so a diagnostic can hold one without building a cellRow, which is the
// same reason `chipReadings` is its own type.
type FixtureDose struct {
	// ActDoubles and ActBlanks count the ACTIONABLE window [start+1, 38].
	//
	// A cell entering at GW n PLAYS [n, 38] — `Simulate`'s loop is
	// `for gw := start; gw <= 38` — but can ACT only from n+1, because the opening
	// fifteen is chosen at the entry deadline. A double in the entry week is
	// football the cell scores and no transfer can ever be banked into, so
	// counting it as dose credits the mechanism with a week it could not act on.
	ActDoubles, ActBlanks int

	// LateDoubles and LateBlanks count the window beyond the opening squad's own
	// horizon, [start+H, 38].
	//
	// This is the sharper quantity and the one nobody had defined. The opening
	// fifteen is built on a horizon of H gameweeks anchored at the entry week, so
	// its window is `[start, start+H-1]` and every double inside `[start+1,
	// start+H-1]` was already visible to and priced by the squad build. What is
	// left for the TRANSFER POLICY to add is `[start+H, 38]`.
	//
	// ⚠️ It is a subset of the actionable pair by construction, so
	// `LateDoubles <= ActDoubles` always. A row where it is not is a bug in this
	// function rather than an unusual season.
	LateDoubles, LateBlanks int
}

// DoseFor is one cell's dose, read off the season's fixture list.
//
// `horizon` is the opening squad's own lookahead — `Weights.Horizon` — which is
// what separates the two windows. Pass the value the cell was actually built at,
// not the shipped 5: an arm that varies the horizon varies its own dose, and a
// column taken from a constant would then describe a different cell.
func DoseFor(cur *Season, start, horizon int) FixtureDose {
	played, count, teams := teamGameweeks(cur.Fixtures)
	doubling := map[int]int{}
	blanking := map[int]int{}
	for gw := 1; gw <= 38; gw++ {
		// ⚠️ **An empty round is not twenty clubs blanking.** `count[gw]` is nil
		// for a round with no fixture at all — 2022-23's GW7, postponed after the
		// Queen's death and redistributed into later events — so the natural loop
		// reads every club as blanking and reports a twenty-club blank that no
		// manager ever faced. Gating on `played` at the source is what stops the
		// next reader inheriting the trap.
		if !played[gw] {
			continue
		}
		for team := range teams {
			switch n := count[gw][team]; {
			case n == 0:
				blanking[gw]++
			case n >= 2:
				doubling[gw]++
			}
		}
	}
	return doseOver(doubling, blanking, start, horizon)
}

// doseOver is the windowing itself, over already-counted club-gameweeks.
//
// Split out from DoseFor so the two windows can be exercised on a synthetic
// calendar: a test that needs the network to check an off-by-one is a test that
// skips when the answer matters, and the off-by-one here is a systematic bias
// rather than noise — the entry week's own double is exactly the one a reader
// would expect to count.
func doseOver(doubling, blanking map[int]int, start, horizon int) FixtureDose {
	if horizon < 1 {
		horizon = 1
	}
	var d FixtureDose
	for gw := start + 1; gw <= 38; gw++ {
		d.ActDoubles += doubling[gw]
		d.ActBlanks += blanking[gw]
		// ⚠️ **The opening engine's window is `[start, start+H-1]`, so the first
		// UNSEEN week is `start+H`.** This read `gw > start+horizon`, which dropped
		// `start+H` from the late dose — off by one, systematic, of fixed sign, and
		// with 92% of doubles falling after GW19 the dropped week is a late week for
		// the later entries. The test asserting it enshrined the same error. Found
		// in review.
		if gw >= start+horizon {
			d.LateDoubles += doubling[gw]
			d.LateBlanks += blanking[gw]
		}
	}
	return d
}
