package analysis

import (
	"math"
	"testing"
)

// mustRefuse fails unless fn panics.
//
// A helper rather than four copies of the same deferred recover, because the
// thing being asserted is uniform and a copied `recover()` that lost its
// `t.Error` would silently stop checking anything.
func mustRefuse(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("XPointsResidual accepted %s. A position the rules have no "+
				"entry for prices its goals channel at the map's ZERO and still "+
				"returns a plausible number — the silent null this guard exists "+
				"to stop.", what)
		}
	}()
	fn()
}

// TestXPointsRefusesAPositionItHasNoRulesFor is the regression test for a silent
// zero that had never been exercised.
//
// # The bug
//
// `XPointsResidual` read `goalPoints[g.Position]` as a bare map index. Go returns
// the zero value for a missing key, so a position the table has never heard of
// priced its goals channel at **nothing** and the row still returned a number of
// the right order of magnitude — the appearance, bonus, saves and card points are
// all left realised, so nothing looks wrong. That is this record's signature
// failure: a null that reads as a result.
//
// # Why this is a real position and not a hypothetical
//
// FPL ran assistant managers as `element_type` 5 for 2024-25. The archive holds
// **322 rows, accumulating to 312 player-gameweeks and carrying 1,861 points**,
// and the instrument has never had a goal value for them.
// `stats/xpoints_common.py` — the offline mirror — already skipped them with an
// explicit `if et not in GOAL: continue`; the Go side did not, which is the
// two-implementations shape one step further in, with the copy being the *safer*
// of the two.
//
// It is unreachable on today's replay path, and that is a fact about a caller
// rather than about this function: `PointInTimeWith` publishes element_types 1-4
// only, so a manager resolves to position "?" and `squadQuota["?"]` is 0.
//
// ⚠️ **The silent side is the ARCHIVE, not the live payload**, and a first version
// of this comment said "nothing stops a sixth position arriving" without that
// distinction. `TestScoringConstantsMatchFPL` iterates FPL's *published*
// `element_types` and errors on one the model has no value for, so a live arrival
// is loud. But it skips outright on a payload with no `game_config`, and nothing
// before 2024-25 GW16 has one — which is exactly how element_type 5 arrived in
// the archive unnoticed. That is the side this guard is on.
//
// # The three things this pins
//
//   - the refusal happens at all, rather than a goal being priced at zero;
//   - it happens BEFORE the blank-gameweek return, because a manager records no
//     minutes and a guard placed after it would never fire on the one population
//     it exists for;
//   - a zero-valued ScoringRules is refused on the same route, so a `Player` built
//     outside `Load` cannot price a whole season's goals at nothing.
func TestXPointsRefusesAPositionItHasNoRulesFor(t *testing.T) {
	// element_type 5, shaped like an archived assistant-manager row: no minutes,
	// no underlying, points that came from channels this instrument never
	// replaces. ⚠️ **The zero minutes are the point.** With the guard moved below
	// the `Minutes <= 0` return this row comes back 0 and the test fails, which is
	// what makes it a statement about the guard's PLACEMENT and not merely about
	// its existence.
	manager := XPointsGW{Position: 5, Fixtures: 1, Minutes: 0, Points: 6}
	mustRefuse(t, "an archived assistant-manager row (element_type 5, no minutes)",
		func() { _ = XPointsResidual(manager, neutralScale, modernRules) })

	// And the same position with a goal on it, which is the shape the silent zero
	// actually costs points on: unguarded this returns the assists channel alone
	// and prices the goal at nothing.
	scoring := XPointsGW{Position: 5, Fixtures: 1, Minutes: 90, Points: 8, Goals: 1}
	mustRefuse(t, "element_type 5 with a goal on it",
		func() { _ = XPointsResidual(scoring, neutralScale, modernRules) })

	// A `Player` whose Rules were never resolved. Every position is unpriced then,
	// so this is the same guard reached from the other direction — and it is the
	// one that fires if `resolveScoringRules` is ever dropped from `repaired()`.
	var unresolved ScoringRules
	mustRefuse(t, "a zero-valued ScoringRules on an ordinary forward",
		func() {
			_ = XPointsResidual(XPointsGW{Position: 4, Fixtures: 1, Minutes: 90,
				Points: 6, Goals: 1}, neutralScale, unresolved)
		})

	// # The positive control
	//
	// A guard that refused everything would pass every assertion above, so each
	// position the rules DO price must go through and must price a goal at a
	// non-zero value. This is what stops the "fix" of emptying the table.
	for _, pos := range []int{1, 2, 3, 4} {
		if !modernRules.Prices(pos) {
			t.Errorf("element_type %d is not priced by the shipped rules; the "+
				"refusal above is refusing footballers", pos)
			continue
		}
		row := XPointsGW{Position: pos, Fixtures: 1, Minutes: 90, Points: 6, Goals: 1}
		bare := row
		bare.Goals = 0
		// Differenced so the clean-sheet and concede channels — which differ by
		// position and are not this test's subject — cancel exactly.
		got := XPointsResidual(row, neutralScale, modernRules) -
			XPointsResidual(bare, neutralScale, modernRules)
		if got <= 0 {
			t.Errorf("element_type %d prices a goal at %v; a position in the table "+
				"must be worth something for scoring", pos, got)
		}
	}
}

// TestTheUnpricedPositionTestIsKeyPresenceAndNotValue.
//
// ⚠️ The roster test has to be **key presence**, and this is the assertion that
// says why. `CleanSheet[4]` is legitimately 0 — FPL pays a forward nothing for a
// clean sheet — and `ConcedeBlock` legitimately has no entry for midfielders or
// forwards. A value test on either would refuse a forward, and a value test on
// `Goal` would refuse any position FPL ever stopped paying for a goal.
//
// This is the same rule as AGENTS.md's config convention — "an empty list is a
// statement and only a missing key is an omission", implemented there as `hasKey`
// in `internal/config/config.go` — arriving in the scoring table.
//
// It also pins the roster's own consistency, which is the tripwire that survives
// the next amendment: `Prices` reads `Goal` alone, while the clean-sheet and
// concede channels are still bare map reads. A position present in `Goal` and
// absent from `CleanSheet` would pass the guard and silently lose a channel —
// the bug being fixed, one channel over.
func TestTheUnpricedPositionTestIsKeyPresenceAndNotValue(t *testing.T) {
	if !modernRules.Prices(4) {
		t.Error("forwards are not priced, and they are the position whose clean " +
			"sheet is zero and whose concede block is absent — the roster test has " +
			"become a value test")
	}
	if got, ok := modernRules.CleanSheet[4]; !ok || got != 0 {
		t.Errorf("a forward's clean sheet is %v (present %v); it must be PRESENT "+
			"and zero, or the fixture above is not testing what it claims", got, ok)
	}
	if _, ok := modernRules.ConcedeBlock[4]; ok {
		t.Error("a forward now has a concede block; the fixture above is not " +
			"testing what it claims")
	}
	if modernRules.Prices(0) || modernRules.Prices(5) {
		t.Error("Prices accepts a position with no goal value")
	}

	// Every position the roster admits must be present in the clean-sheet table
	// too, because that channel is read as a bare map index and its zero is a
	// legitimate rule. Without this a position added to `Goal` alone would be
	// guarded on goals and silently unguarded on the clean sheet.
	for pos := range modernRules.Goal {
		if _, ok := modernRules.CleanSheet[pos]; !ok {
			t.Errorf("element_type %d is priced for a goal and absent from the "+
				"clean-sheet table. `Prices` reads Goal alone, so this position "+
				"passes the guard and then reads `CleanSheet[%d]` as a bare index — "+
				"its clean-sheet channel is deleted silently, which is the bug this "+
				"file exists to stop, one channel over", pos, pos)
		}
	}
}

// TestXPointsPricesEachSeasonUnderItsOwnRules is the regression test for the
// second tripwire: the instrument had no per-season pin at all.
//
// # The mechanism, which is the point rather than the size
//
// `goalPoints` and its siblings are today's rules, and
// `TestScoringConstantsMatchFPL` asserts them against FPL's published
// `game_config` on every run. That test is what keeps them honest — and it is
// therefore also the mechanism that would have carried the next rule change
// **backwards over the whole archive**, silently re-pricing every replayed
// season under a rule nobody played under. `BankLimitFor` and `DefconScoredIn`
// exist to prevent exactly that one layer away; the instrument had no
// equivalent.
//
// # What is asserted, and on what evidence
//
// The one change this repository can demonstrate is the goalkeeper's goal.
// **6 is decoded from the archive** — Alisson 2020-21 GW36 reconstructs to
// exactly 6 with every other channel accounted for, and it is the only
// goalkeeper goal in the archive. **10 is published**, in the captured 2025-26
// `game_config`. `scoringrules.go` carries the arithmetic and the caveat on the
// boundary between them, which is *not* established.
//
// So the assertions are: an archived season keeps the value the archive
// measured, the modern season takes whatever FPL publishes today, and the two
// tables differ. The last is the one that fails if `ScoringRulesFor` is ever
// "simplified" back into a function that returns the current table for every
// season, which is the tripwire.
func TestXPointsPricesEachSeasonUnderItsOwnRules(t *testing.T) {
	old := ScoringRulesFor("2020-21")
	modern := ScoringRulesFor(keeperGoalRuleChangeSeason)

	if got := old.Goal[1]; got != keeperGoalPointsBeforeTheRuleChange {
		t.Errorf("a goalkeeper's goal in 2020-21 is priced at %v, want %v — that "+
			"value is decoded from the archive's only goalkeeper goal, not asserted",
			got, keeperGoalPointsBeforeTheRuleChange)
	}
	if got, want := modern.Goal[1], goalPoints[1]; got != want {
		t.Errorf("a goalkeeper's goal in %s is priced at %v; the current season "+
			"must take FPL's published value of %v",
			keeperGoalRuleChangeSeason, got, want)
	}

	// ⚠️ The fixture has to be able to fail. If FPL ever moves the modern value
	// back onto the archived one the two tables coincide, every assertion below
	// passes on arithmetic, and this test stops covering the tripwire. Say so
	// rather than leave it to be rediscovered.
	if modern.Goal[1] == old.Goal[1] {
		t.Fatalf("the archived and modern tables both price a goalkeeper's goal at "+
			"%v, so nothing below can distinguish a per-season table from today's "+
			"one. Find a channel that still differs, or this test is vacuous",
			modern.Goal[1])
	}

	// The channel reaches the residual, which is the half a constant test cannot
	// see: a table that differed and was never read would pass everything above.
	// Differenced against the same row with no goal, so every other channel
	// cancels and what is left is exactly the goal's price.
	scored := XPointsGW{Position: 1, Fixtures: 1, Minutes: 90, Points: 10,
		Goals: 1, GoalsConceded: 1, XGC: 1.2}
	bare := scored
	bare.Goals = 0
	priced := func(r ScoringRules) float64 {
		return XPointsResidual(scored, neutralScale, r) -
			XPointsResidual(bare, neutralScale, r)
	}
	if got := priced(old); math.Abs(got-keeperGoalPointsBeforeTheRuleChange) > 1e-12 {
		t.Errorf("2020-21 prices a goalkeeper's goal at %v through the residual, "+
			"want %v — the season's table is not reaching the arithmetic",
			got, keeperGoalPointsBeforeTheRuleChange)
	}
	if got, want := priced(modern), goalPoints[1]; math.Abs(got-want) > 1e-12 {
		t.Errorf("%s prices a goalkeeper's goal at %v through the residual, want %v",
			keeperGoalRuleChangeSeason, got, want)
	}

	// And the amendment must be the ONLY difference. A table that drifted on some
	// other channel would change figures nobody has agreed to move, on seasons
	// whose values this repository has no evidence about.
	for pos, v := range modern.Goal {
		if pos == 1 {
			continue
		}
		if old.Goal[pos] != v {
			t.Errorf("element_type %d's goal is %v in 2020-21 and %v in %s; the "+
				"only amendment this repository has evidence for is the "+
				"goalkeeper's", pos, old.Goal[pos], v, keeperGoalRuleChangeSeason)
		}
	}
	if old.Assist != modern.Assist {
		t.Errorf("an assist is %v in 2020-21 and %v in %s, and nothing here "+
			"establishes that it moved", old.Assist, modern.Assist,
			keeperGoalRuleChangeSeason)
	}
	for pos, v := range modern.CleanSheet {
		if old.CleanSheet[pos] != v {
			t.Errorf("element_type %d's clean sheet is %v in 2020-21 and %v in %s",
				pos, old.CleanSheet[pos], v, keeperGoalRuleChangeSeason)
		}
	}
	for pos, v := range modern.ConcedeBlock {
		if old.ConcedeBlock[pos] != v {
			t.Errorf("element_type %d's concede block is %v in 2020-21 and %v in %s",
				pos, old.ConcedeBlock[pos], v, keeperGoalRuleChangeSeason)
		}
	}
}

// TestTheSeasonRulesAreACopyAndNotAnAlias.
//
// `ScoringRulesFor` amends the table it returns. If it handed back the package's
// own maps the first archived season resolved would rewrite `goalPoints` for the
// whole process — and `goalPoints` is what `baseXP90` scores live players
// through, so an instrument-only change would have reached the scoring path.
//
// That is not a hypothetical shape: it is the failure mode of every "just return
// the map" simplification, and it is invisible in a diff.
func TestTheSeasonRulesAreACopyAndNotAnAlias(t *testing.T) {
	before := goalPoints[1]
	r := ScoringRulesFor("2020-21")
	if goalPoints[1] != before {
		t.Fatalf("resolving an archived season rewrote goalPoints[1] from %v to %v "+
			"— ScoringRulesFor is aliasing the package's tables and the instrument "+
			"has reached the scoring path", before, goalPoints[1])
	}
	// And the other direction, in case the amendment happened to be a no-op.
	r.Goal[2], r.CleanSheet[2], r.ConcedeBlock[2] = 99, 99, 99
	if goalPoints[2] == 99 || cleanSheetPoints[2] == 99 || concedeBlock[2] == 99 {
		t.Error("writing to a returned rules table reached the package constants")
	}
}
