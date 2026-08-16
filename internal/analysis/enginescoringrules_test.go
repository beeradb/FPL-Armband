package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// The two silent-failure traps on the ENGINE's scoring path.
//
// `scoringrules_test.go` pins the table's arithmetic and the instrument's use of
// it. These pin the engine's: that a position the table has no entry for is
// refused rather than priced at the map's zero, and that the table an engine
// scores through is the one that was in force in the season it is scoring.
//
// ⚠️ **Neither result transports from the instrument's half.** `XPointsResidual`
// writes a column nothing scores; `baseXP90` writes `Score`, therefore the
// ordering, therefore which footballers get bought. The instrument's
// byte-identical confinement said nothing whatever about this side.

// rulesEngine is an engine over a bootstrap naming one season, with the
// element_types and elements a test asks for.
//
// A helper because the season has to arrive through `fpl.Bootstrap.Season` —
// that is the whole design, there being no setter to call — and a test that
// built the Engine literally would get the zero-valued rules that
// `TestTheEngineRefusesToScoreAPositionItHasNoRulesFor` exists to make loud.
func rulesEngine(season string, els ...fpl.Element) (*Engine, *fpl.Element) {
	b := &fpl.Bootstrap{
		Season: season,
		Teams:  []fpl.Team{{ID: 1, ShortName: "AAA", Strength: 3}},
		ElementTypes: []fpl.ElementType{
			{ID: 1, SingularNameShort: "GKP"}, {ID: 2, SingularNameShort: "DEF"},
			{ID: 3, SingularNameShort: "MID"}, {ID: 4, SingularNameShort: "FWD"},
		},
		Elements: els,
	}
	for i := 1; i <= 38; i++ {
		b.Events = append(b.Events, fpl.Event{ID: i, Name: "Gameweek"})
	}
	e := NewEngineFull(b, nil, DefaultWeights(), Congestion{}, RoleRisk{})
	return e, &e.Boot.Elements[0]
}

// keeperWithXG is a goalkeeper who has played a full season and carries a small
// expected-goals rate — the only shape the one rule change this repository can
// demonstrate actually reaches.
//
// ⚠️ **A keeper who has never scored is still re-priced**, and reasoning only
// about goalkeeper *goals* misses him: the goals channel is a RATE times the
// goal's price, so `Goal[1]` prices the expected half too. That is the mistake
// the instrument's own author made and had to retract, recorded in
// `scoringrules.go`'s header, and it is why this fixture carries xG and no goal.
func keeperWithXG(elementType int) fpl.Element {
	return fpl.Element{
		ID: 1, Code: 1, WebName: "Keeper", ElementType: elementType, Team: 1,
		NowCost: 50, Status: "a", Minutes: 38 * 90, Starts: 38,
		ExpectedGoalsPer90: 0.02, ExpectedAssistsPer90: 0, ExpectedGCPer90: 1.2,
	}
}

// TestTheEngineRefusesToScoreAPositionItHasNoRulesFor is the regression test for
// the first defect: a bare map index on the engine's scoring path.
//
// # The bug
//
// `baseXP90`, `setPieceXP90` and `fixtureSensitiveAt` read `goalPoints[pos]` as a
// bare map index at five sites. Go returns the zero value for a missing key and
// cannot distinguish it from a stored zero — and a stored zero is a legitimate
// FPL rule in this table's siblings, since a forward's clean sheet pays exactly
// 0. So a position the model has never heard of had its goals channel priced at
// **nothing** while the appearance, bonus and card channels stayed realised, and
// the row came back at exactly 2.0 base expected points: a plausible number with
// a whole channel deleted.
//
// **A value test cannot fix this.** `if v := goalPoints[pos]; v > 0` asks a
// different question and additionally refuses any position FPL ever stops paying
// for a goal. The fix is key presence, in `ScoringRules.Prices`.
//
// # The population, and why it is not the instrument's
//
// FPL ran assistant managers as `element_type` 5 for 2024-25, and unlike the
// instrument's population this one is reachable on both live paths: the captured
// GW38 2024-25 bootstrap publishes 20 of them as `MNG`, and the replay's own
// `PointInTimeWith` carries the same 20 from `through` 26, which is an entry
// point the shipped sweep grid uses. `AllMetrics` scores every element, so the
// engine met them at a deadline it really picks a squad at.
//
// # What is asserted
//
//   - the priced half is refused, and the DESCRIPTIVE half is not. Refusing the
//     whole record would lose the name, the price and the minutes, which are
//     facts that need no points table;
//   - the refusal is `Prices`, so it survives a position whose clean sheet or
//     concede block is legitimately zero or absent;
//   - and the accessor behind the door refuses too, which is what fails if the
//     door is deleted or a sixth pricing site is added behind it.
func TestTheEngineRefusesToScoreAPositionItHasNoRulesFor(t *testing.T) {
	const unpriced = 5 // FPL's assistant managers
	manager := fpl.Element{
		ID: 1, Code: 99, WebName: "Manager", ElementType: unpriced, Team: 1,
		NowCost: 15, Status: "a", Minutes: 38 * 90, Starts: 38,
		// Deliberately given underlying numbers. A manager records none, which is
		// why the engine's zero Score never exposed this — so the fixture is the
		// case the guard is FOR: a position with minutes on it.
		ExpectedGoalsPer90: 0.5, ExpectedAssistsPer90: 0.4, ExpectedGCPer90: 1.0,
	}
	e, el := rulesEngine("2025-26", manager)

	if e.rules.Prices(unpriced) {
		t.Fatalf("element_type %d is priced by the shipped rules, so this fixture "+
			"is not testing an unpriced position at all", unpriced)
	}

	m := e.Metrics(el)
	// The priced half. Every one of these is computed through the four channels
	// the season's rules carry, and none of them may be invented.
	for _, c := range []struct {
		name string
		got  float64
	}{
		{"BaseXP90", m.BaseXP90},
		{"SetPieceXP90", m.SetPieceXP90},
		{"FixtureAdjXP90", m.FixtureAdjXP90},
		{"Score", m.Score},
	} {
		if c.got != 0 {
			t.Errorf("an unpriced element_type scored %s = %v. The engine has no "+
				"points table for it, so every priced quantity must be zero — a "+
				"partial figure is a plausible number with a channel silently "+
				"deleted, which is the defect this guards", c.name, c.got)
		}
	}
	// ⚠️ The mirror, and it is what makes the assertion above about the DOOR
	// rather than about an empty return. Refusing to price him is not refusing to
	// describe him.
	if m.Name != "Manager" || m.Minutes != 38*90 || m.Price == 0 {
		t.Errorf("the descriptive half was dropped too (name %q, minutes %d, price "+
			"%v). Who he is and what he costs need no points table, and losing them "+
			"turns a scoped refusal into a silent disappearance",
			m.Name, m.Minutes, m.Price)
	}
	// ⚠️ **The multipliers, and this is the assertion a first version of the door
	// failed.** `AvailabilityFactor`, `Congestion` and `RoleFactor` are neutral at
	// **1**, not 0, and a zero is not "unset" to the things that read them:
	// `present/card.go` renders `availability x0.00` through a branch that exists
	// so a ruled-out player's zero survives, and `tools.go` passes `avail_factor`
	// to the model through a pointer for the same reason. Left at Go's zero, a
	// refused row asserts that an available player is unavailable — a different
	// and wronger claim than "the model cannot price him".
	if m.AvailabilityFactor != 1 || m.Congestion != 1 || m.RoleFactor != 1 {
		t.Errorf("a refused, AVAILABLE element reports availability %v, congestion "+
			"%v, role %v. All three are multipliers whose neutral value is 1, and a "+
			"0 is read downstream as 'ruled out' rather than 'not computed' — so "+
			"the row would tell a human and the agent something false about him",
			m.AvailabilityFactor, m.Congestion, m.RoleFactor)
	}

	// # The positive control
	//
	// A door that refused everything would satisfy every assertion above. Each
	// position FPL really pays must go through and must score something.
	for _, pos := range []int{1, 2, 3, 4} {
		fe, fel := rulesEngine("2025-26", fpl.Element{
			ID: 1, Code: 1, WebName: "Player", ElementType: pos, Team: 1,
			NowCost: 50, Status: "a", Minutes: 38 * 90, Starts: 38,
			ExpectedGoalsPer90: 0.5, ExpectedAssistsPer90: 0.4, ExpectedGCPer90: 1.0,
		})
		if got := fe.Metrics(fel).BaseXP90; got <= 0 {
			t.Errorf("element_type %d scores BaseXP90 %v; the door is refusing "+
				"footballers", pos, got)
		}
	}

	// And the accessor behind the door, which is the invariant rather than the
	// policy. This is what fails if the `Prices` check in `Metrics` is deleted, or
	// if a sixth pricing site is added inside it.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("ScoringRules.GoalPoints returned a value for an unpriced " +
					"element_type instead of refusing. It is the tripwire behind the " +
					"door in Engine.Metrics — with it silent, deleting that door " +
					"restores the original bug with nothing failing")
			}
		}()
		_ = e.rules.GoalPoints(unpriced)
	}()
}

// TestTheEngineScoresEachSeasonUnderItsOwnRules is the regression test for the
// second defect: the engine's scoring rules were not pinned per season.
//
// # The bug, which is prospective and therefore easy to dismiss
//
// `goalPoints` and its siblings are **today's** rules, and
// `TestScoringConstantsMatchFPL` asserts them against FPL's published
// `game_config` on every run. That test is what keeps them honest — and it is
// exactly the mechanism that would carry the *next* rule change backwards over
// the whole archive, silently re-scoring every replayed season under a rule
// nobody played under. `BankLimitFor` and `DefconScoredIn` exist to stop that for
// the transfer bank and for defensive contribution; the engine had no equivalent,
// while the instrument gained one first.
//
// # What is asserted, and on what evidence
//
// The one change this repository can demonstrate is the goalkeeper's goal: 10
// today, 6 in 2020-21, the 6 **decoded** from Alisson's 2020-21 GW36 row and the
// 10 **published** in FPL's own `game_config`. See `scoringrules.go` for the
// arithmetic and for the caveat that the boundary between them is bounded rather
// than measured.
//
// The assertion is differenced through `baseXP90` — the same fixture at two
// season names — so the appearance, clean-sheet, concede, saves and bonus
// channels are common to both and cancel exactly. What is left is the goals
// channel and nothing else.
//
// ⚠️ **The fixture carries xG and no goal on purpose.** A keeper who never scores
// is still re-priced, because the channel is a rate times the price.
func TestTheEngineScoresEachSeasonUnderItsOwnRules(t *testing.T) {
	const gkp = 1
	old, modern := "2020-21", "2025-26"

	oldPrice := ScoringRulesFor(old).Goal[gkp]
	newPrice := ScoringRulesFor(modern).Goal[gkp]
	// ⚠️ The fixture has to be able to fail. If FPL ever moves the modern value
	// back onto the archived one, every assertion below passes on arithmetic and
	// this test silently stops covering the tripwire.
	if oldPrice == newPrice {
		t.Fatalf("%s and %s both price a goalkeeper's goal at %v, so nothing below "+
			"can distinguish a per-season table from today's one. Find a channel "+
			"that still differs, or this test is vacuous", old, modern, oldPrice)
	}

	oe, oel := rulesEngine(old, keeperWithXG(gkp))
	ne, nel := rulesEngine(modern, keeperWithXG(gkp))
	om, nm := oe.Metrics(oel), ne.Metrics(nel)

	// Everything but the goals channel is identical between the two engines, so
	// the difference in base expected points is the goal price difference times
	// the (identical) blended, scaled expected-goals rate.
	rate := nm.XG90 * nm.XGScale
	if rate <= 0 {
		t.Fatalf("the fixture keeper's scaled expected-goals rate is %v; with no "+
			"rate the goals channel is zero under both tables and this test "+
			"measures nothing", rate)
	}
	if om.XG90 != nm.XG90 || om.XGScale != nm.XGScale {
		t.Fatalf("the two engines disagree about the keeper's rate (%v x %v against "+
			"%v x %v), so the difference below is not the goal price alone",
			om.XG90, om.XGScale, nm.XG90, nm.XGScale)
	}

	want := rate * (newPrice - oldPrice)
	if got := nm.BaseXP90 - om.BaseXP90; math.Abs(got-want) > 1e-9 {
		t.Errorf("a goalkeeper's base expected points differ by %v between %s and "+
			"%s; the goal prices are %v and %v against a scaled rate of %v, so the "+
			"difference must be %v. The season's own table is not reaching the "+
			"engine's arithmetic — %s is being scored under %s's rules",
			got, old, modern, oldPrice, newPrice, rate, want, old, modern)
	}

	// And the amendment must be the ONLY difference. A position the archive has
	// no evidence about must score identically under both.
	for _, pos := range []int{2, 3, 4} {
		a, ael := rulesEngine(old, keeperWithXG(pos))
		b, bel := rulesEngine(modern, keeperWithXG(pos))
		if x, y := a.Metrics(ael).BaseXP90, b.Metrics(bel).BaseXP90; x != y {
			t.Errorf("element_type %d scores %v in %s and %v in %s. The only rule "+
				"change this repository has evidence for is the goalkeeper's goal, "+
				"so an outfielder moving means the table drifted on a channel "+
				"nobody has agreed to move", pos, x, old, y, modern)
		}
	}
}

// TestTheLiveEngineScoresUnderTodaysRules is the guard on the one special case in
// `ScoringRulesFor`, and it is the one whose absence would be a shipped scoring
// bug rather than a replay one.
//
// FPL's payload carries no season name, so a bootstrap fetched from the API has
// `Season == ""`. Seasons are compared as strings — the same way `BankLimitFor`
// and `DefconScoredIn` compare them — and `""` is the ONE string that really does
// sort below every real season under Go's byte comparison. Without the explicit
// clause, `"" < "2024-25"` is true and the live engine would price every
// goalkeeper's goal at the pre-2024-25 **6**.
//
// That is not an instrumentation column. It is `Score`, therefore the ordering,
// therefore which goalkeeper the optimiser buys — and it would have arrived
// silently, as a side effect of pinning the replay.
func TestTheLiveEngineScoresUnderTodaysRules(t *testing.T) {
	live := ScoringRulesFor("")
	for pos, want := range goalPoints {
		if got := live.Goal[pos]; got != want {
			t.Errorf("an unnamed season prices element_type %d's goal at %v; FPL "+
				"pays %v today. A bootstrap fetched from the API carries no season "+
				"name, so this IS the live game, and the empty string is the one "+
				"name that sorts below every archived season", pos, got, want)
		}
	}
	if live.Assist != assistPoints {
		t.Errorf("an unnamed season pays %v for an assist, want today's %v",
			live.Assist, assistPoints)
	}

	// End to end, through the engine, because a table that was right and never
	// read would pass everything above.
	const gkp = 1
	le, lel := rulesEngine("", keeperWithXG(gkp))
	me, mel := rulesEngine("2025-26", keeperWithXG(gkp))
	if got, want := le.Metrics(lel).BaseXP90, me.Metrics(mel).BaseXP90; got != want {
		t.Errorf("a live engine scores a goalkeeper at %v and a current-season one "+
			"at %v. The live path is on a different points table from the season "+
			"whose rules it is playing under", got, want)
	}
}

// TestTheReportedRulesCannotReachTheEngine.
//
// `Engine.ScoringRules()` exists so a diagnostic can ask *which* table arrived —
// a pin nobody set is a byte-identical null and the only way to tell it from a
// pin that does nothing is to ask. That makes it a handle on the maps the engine
// scores through, and a first version returned them directly with a comment
// asking callers to treat the result as immutable.
//
// Two things are wrong with a comment there. A write would re-price every player
// scored after it — silently, since the engine has no idea it happened — and it
// would be a **data race**, because the agent's tool runner reads one engine from
// several goroutines at once and this project has already been killed by a
// concurrent map access. Neither is something a caller can be asked to remember.
//
// So the accessor clones, and this is the test that says so rather than a
// paragraph asking nicely.
func TestTheReportedRulesCannotReachTheEngine(t *testing.T) {
	const gkp = 1
	e, el := rulesEngine("2020-21", keeperWithXG(gkp))
	before := e.Metrics(el).BaseXP90
	if before <= 0 {
		t.Fatalf("the fixture keeper scores %v, so a re-pricing would be "+
			"invisible and this test is vacuous", before)
	}

	// A caller mutating what it was handed, which is exactly what the comment used
	// to ask them not to do.
	got := e.ScoringRules()
	got.Goal[gkp] = 9999
	got.CleanSheet[gkp] = 9999
	got.ConcedeBlock[gkp] = 9999

	if after := e.Metrics(el).BaseXP90; after != before {
		t.Errorf("writing to the table returned by ScoringRules() re-priced the "+
			"engine: a goalkeeper went from %v to %v. The accessor is handing out "+
			"the engine's own maps, so any caller can silently change what every "+
			"later player is worth — and do it from one of several goroutines the "+
			"tool runner scores in at once", before, after)
	}
}

// TestDerivedEnginesInheritTheSeasonsRules.
//
// `WeekEngine` and `engineAt` build a second, horizon-1 engine from the first,
// and `TestDerivedEnginesCarryEverySource` enumerates what they must carry
// across — by reflection, which sees exported fields only. `Engine.rules` is
// unexported, so that guard is structurally blind to it.
//
// It is carried anyway, and by construction rather than by an assignment: both
// builders call `NewEngineFull(e.Boot, ...)`, and the rules are derived from
// `Boot.Season`. That is the whole reason the season lives on the bootstrap
// instead of on the engine — an `Engine.Rules` field would have had to be copied
// in two more places, and this project's most expensive wiring bug is exactly the
// copy that was not made.
//
// Asserted rather than argued, because "it follows from the constructor" is what
// was true of the recency index too, right up until someone changed the
// constructor.
func TestDerivedEnginesInheritTheSeasonsRules(t *testing.T) {
	const gkp = 1
	e, _ := rulesEngine("2020-21", keeperWithXG(gkp))
	want := e.rules.Goal[gkp]
	if want == goalPoints[gkp] {
		t.Fatalf("2020-21 prices a goalkeeper's goal identically to today (%v), so "+
			"a derived engine that had fallen back to today's table would be "+
			"indistinguishable and this test is vacuous", want)
	}

	for name, derived := range map[string]*Engine{
		"WeekEngine": e.WeekEngine(),
		"engineAt":   e.engineAt(1),
	} {
		if derived.rules.Season != e.rules.Season {
			t.Errorf("%s carries rules for season %q against its parent's %q",
				name, derived.rules.Season, e.rules.Season)
		}
		if got := derived.rules.Goal[gkp]; got != want {
			t.Errorf("%s prices a goalkeeper's goal at %v against its parent's %v. "+
				"The eleven that gets fielded and the transfer that gets made would "+
				"be scored under different rules", name, got, want)
		}
	}
}
