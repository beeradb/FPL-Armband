package analysis

import (
	"fmt"
	"os"
)

// The escape hatches for the two changes this file makes to the ENGINE's scoring
// path, which is why there are two rather than one: they are separate defects
// and an arm that moved both would be uninterpretable.
//
// `docs/architecture.md`: "a scoring change that cannot be switched off cannot be
// compared against the behaviour it replaced".
//
//   - **FPL_NO_SEASON_SCORING_RULES=1** makes `ScoringRulesFor` ignore the season
//     and return today's table for everything, which is what the engine did
//     before the pin. It is the arm for the confinement question — does pinning
//     move replayed points? — and it reaches the xPoints instrument too, since
//     both read this one function. Say so when quoting a figure under it.
//   - **FPL_NO_UNPRICED_POSITION_GUARD=1** restores the bare map index: an
//     element_type with no table prices its goals channel at zero and scores on
//     through, and `Engine.Metrics` does not refuse it. It is the arm for the
//     liveness question — does the guard reach anything? — and the answer it is
//     measured against is `BaseXP90` 2.0 against 0 on FPL's 2024-25 assistant
//     managers.
//
// ⚠️ Both are **off** by default and neither is a supported way to run. They exist
// so a measurement can name the behaviour it is comparing against.
var (
	seasonScoringRules    = os.Getenv("FPL_NO_SEASON_SCORING_RULES") == ""
	unpricedPositionGuard = os.Getenv("FPL_NO_UNPRICED_POSITION_GUARD") == ""
)

// FPL's points table, pinned per season. Read by the **engine** and by the
// xPoints instrument, which is one table and not two.
//
// ⚠️ **This file was `xpointsrules.go` and the type was `XPointsRules`**, when
// the instrument was its only reader. `Engine` now scores through it, so a name
// saying "xPoints" would have claimed the instrument reaches the scoring path —
// the opposite of what this record spends a section establishing. Renamed rather
// than copied: a second per-season table is the one failure this project calls
// its signature.
//
// # Why this exists
//
// `goalPoints`, `cleanSheetPoints`, `assistPoints` and `concedeBlock` are
// **today's** rules. `TestScoringConstantsMatchFPL` asserts them against FPL's
// published `game_config` on every run, which is what makes them falsifiable —
// and it is also a mechanism that *forces them forward* the moment FPL changes
// one. Before this file, `XPointsResidual` read those four tables directly, so
// the next rule change would have silently re-priced **every archived season**
// under a rule nobody played under. `baseXP90`, `setPieceXP90` and
// `fixtureSensitiveAt` read them directly too, which is the same defect on the
// path that moves squad selection rather than an instrumentation column.
//
// That is the failure `BankLimitFor` and `DefconScoredIn` already exist to
// prevent one layer away — the transfer bank went 2 to 5 for 2024-25, defensive
// contribution arrived for 2025-26, and in both cases the fix was to pin the
// rule to the season rather than take today's value. The instrument had no
// equivalent. This is it.
//
// # The change that already happened, and what is and is not established
//
// A goalkeeper's goal is worth **10** today, was **10** in 2024-25, and was **6**
// in 2020-21.
//
//   - The 6 is **decoded from the archive**, not asserted. Alisson, 2020-21
//     GW36: 90 minutes, 1 goal, 0 clean sheets, 1 conceded, 2 saves, 2 bonus,
//     and — read off `merged_gw.csv`'s own columns, because the decoding is only
//     unique with them — 0 cards, 0 own goals, 0 penalties missed and 0
//     penalties saved, against `total_points` 10. Every other channel sums to 4
//     (2 appearance + 2 bonus; `floor(1/2)` conceded and `floor(2/3)` saves are
//     both 0), so the goal was paid 6 exactly. Without the own-goal and
//     missed-penalty columns `10 = 2 + 2 + G − 4` would also solve at G = 10,
//     which is why they are named rather than assumed.
//     It is also **robust to the two divisors FPL does not publish**:
//     `floor(1/d)` is 0 for any d ≥ 2 and `floor(2/d)` is 0 for any d ≥ 3, so
//     the 6 does not depend on today's saves and concede rules having held in
//     2020-21. It assumes only that the sixty-minute appearance paid 2 and that
//     bonus was paid as recorded.
//   - It is the **only goalkeeper goal in the archive, 2016-17 to 2025-26**.
//     ⚠️ **Do not check that with a `position == 'GK'` filter on
//     `merged_gw.csv`.** That file carries no `position` column at all before
//     2020-21, and 2021-22 spells 101 of its keeper rows `GKP` rather than `GK`,
//     so the filter returns a clean zero on five seasons for reasons that have
//     nothing to do with football — the byte-identical null this record keeps
//     being caught by. The sound method is to join `players_raw.csv`'s
//     `element_type` onto `merged_gw.csv`'s `element`, which is what this
//     package's own loader does and what `Season.Players[*].Type` therefore is.
//   - The 10 is **published**, and earlier than it looks: FPL added the
//     `game_config` block to the bootstrap **mid-2024-25**. It first appears in
//     this repository's capture at
//     `data/captures/2024-25/GW16-2024-12-14T1200Z/bootstrap-static.json.gz` and
//     runs continuously to GW38, then through every 2025-26 capture and both
//     2026/27 pre-season ones — `goals_scored.GKP = 10` in all of them. That
//     capture is a genuine point-in-time harvest rather than a later backfill
//     wearing a 2024-25 directory name: its `current_event` is 15 and its
//     `scoring` block has no `defensive_contribution` key, which 2025-26's does.
//     ⚠️ **An earlier version of this comment said no capture before 2025-26
//     carried `game_config` at all.** That was checked on GW1 of each season only,
//     and it is false: 23 of 2024-25's 38 captures carry it.
//
// ⚠️ **The boundary is not established, but it is bounded on the late side, and
// this constant is pinned to that bound.** Nothing earlier than 2024-25 GW16
// publishes scoring — the 2021-22 to 2023-24 captures carry `game_settings` with
// no scoring keys and no `game_config` — and the archive holds no goalkeeper goal
// anywhere in 2021-22..2023-24 to decode one from. So the change happened
// somewhere in **2021-22..2024-25**, and `keeperGoalRuleChangeSeason` takes the
// latest season the evidence permits, which is the only end this repository can
// defend. Settling the early end wants FPL's own published rule history rather
// than a run.
//
// # ⚠️ What the residual span costs: a BOUND, not an identity
//
// ⚠️ **An earlier version of this comment said the choice was "inert", and that
// was wrong.** The goals channel is `(Goals − XG·scale)·Goal[pos]`, so `Goal[1]`
// prices the **expected** half too — a goalkeeper with no goal and non-zero xG is
// re-priced by moving the boundary, and reasoning only about goalkeeper *goals*
// misses him. That is this record's "check the mediator" rule failing on the
// author of the mediator argument.
//
// Measured over the repaired archive, goalkeeper rows with minutes and a non-zero
// goals channel, at a GKP goals scale of exactly 1.0 in every season — keeper
// season xG never approaches `minCalibrationSample`, so `CalibrationRatio`
// returns 1 — the 10-to-6 amendment moves xPoints by:
//
//	2018-19  3 rows  −0.574     2022-23  2 rows  −0.920
//	2019-20  3 rows  −1.460     2023-24  1 row   −0.080
//	2020-21  1 row   +3.604     2024-25  2 rows  −0.280   (unmoved at this pin)
//	2021-22  0 rows   0.000     2025-26  1 row   −0.640   (unmoved at this pin)
//
// So **the unresolved span 2021-22..2023-24 is worth 3 rows and 1.00 xPoints in
// total**, against a `HOLD` detection threshold of about 33 points a season and a
// noise floor of 10-12. Two orders of magnitude under the floor, on the
// instrument columns only. That is why an unestablished early boundary is
// tolerable here — and it is a **bound**, not a licence: it stops being
// negligible the first time a goalkeeper scores in a re-harvested season, which
// is exactly the case a pinned rule is for.
//
// # ⚠️ The amendment's EARLY extent is asserted too, and undeclared until now
//
// `season < keeperGoalRuleChangeSeason` applies the 6 back to 2016-17 and before,
// on the evidence of one 2020-21 row. Nothing here establishes that FPL paid 6
// in 2016-17, and the pre-2020-21 rows above (2018-19 and 2019-20, 2.03 xPoints
// between them) carry more of the amendment than 2024-25 does. Treat the whole
// table as "6 is the only pre-modern value this repository can decode", not as
// a rule history.
//
// # What is deliberately NOT pinned
//
// Only the four channels `XPointsResidual` replaces need a rule, because every
// other channel is left at its realised value and therefore carries whichever
// season's rules FPL applied when it paid the points. That is the same argument
// the instrument's own header makes for being a residual rather than a
// re-scoring, and it is what keeps three bonus regimes, the 2025-26 defensive
// contribution and the 2026/27 saves restructure out of this file entirely.
//
// # One implementation
//
// The per-season table is **derived from** the package's current-rules tables
// rather than being a second copy of them: `ScoringRulesFor` starts from
// `goalPoints`, `cleanSheetPoints`, `concedeBlock` and `assistPoints` — the ones
// `TestScoringConstantsMatchFPL` pins to FPL — and applies named amendments
// backwards from there. So a value FPL changes still arrives in exactly one
// place, and what this file adds is the record of *when* it changed.

// ScoringRules is FPL's points table for the four channels the xPoints
// instrument replaces, as it stood in one season.
//
// The zero value prices nothing and is not a usable table. `XPointsResidual`
// refuses it loudly rather than pricing a goal at zero, for the same reason it
// refuses a zero `ConversionScale`: a silent zero here is not a small error, it
// is a whole channel deleted in a way that still returns a plausible number.
type ScoringRules struct {
	// Season is the season these rules were in force for, carried so a refusal
	// can say which table was consulted.
	Season string

	// Goal is points for a goal, by element_type.
	//
	// It is also the **roster of positions these rules price at all**, which
	// `Prices` reads. FPL pays every playing position something for a goal, so a
	// position absent from here is a position the instrument has no table for —
	// see `Prices` for why that has to be a key-presence test and not a value
	// test.
	Goal map[int]float64

	// CleanSheet is points for a clean sheet, by element_type. A forward's entry
	// is present and **zero**, which is a rule and not an absence.
	CleanSheet map[int]float64

	// ConcedeBlock is how many goals conceded in a single match cost a point.
	// Midfielders and forwards take no deduction and are **absent**, which is
	// also a rule and not an omission.
	ConcedeBlock map[int]int

	// Assist is points for an assist, which FPL has paid at every position for
	// the whole of the archive.
	Assist float64
}

// keeperGoalPointsBeforeTheRuleChange is what FPL paid a goalkeeper for a goal
// before `keeperGoalRuleChangeSeason`.
//
// **Measured**, and from the only row in the archive that can measure it:
// Alisson 2020-21 GW36 reconstructs to exactly 6 with every other channel
// accounted for. See this file's header for the arithmetic and for the caveat on
// the boundary, which is *not* measured.
const keeperGoalPointsBeforeTheRuleChange = 6.0

// keeperGoalRuleChangeSeason is the first season this repository can show a
// goalkeeper's goal paying the modern 10.
//
// **Bounded, not measured.** 2024-25 is the *latest* season the evidence permits:
// FPL's own `game_config` publishes `goals_scored.GKP = 10` from that season's
// GW16 capture onward. The *earliest* is unknown — nothing before that publishes
// scoring at all, and no goalkeeper scores in 2021-22..2023-24 — so the true
// boundary is somewhere in 2021-22..2024-25 and this takes the end that is
// defensible from a file in this repository.
//
// The residual span is worth **3 archive rows and 1.00 xPoints in total**, on the
// instrument columns only. Bounded, not inert; the header carries the row list.
//
// ⚠️ **THIS CONSTANT IS POINTS-LOAD-BEARING FROM 2026-08-16, and it was
// instrumentation-only before.** Measured on the 72-cell confinement grid (6
// seasons x 6 entry points x both `WeeklyXI` settings), pinning the engine's rules
// per season is **byte-identical on `policy_points` in 72 of 72** when this
// constant takes the EARLY end of its own unresolved range — and moves **8 of 72**
// when it takes the late end: all six 2022-23 cells by exactly -7 at
// `WeeklyXI=false`, and 2023-24 entry GW11 by -2 at both settings. `hold_points`
// and the opening fifteen never move.
//
// So every replayed point this file moves belongs to the *choice of end*, not to
// the pin. Both seasons are inside the unresolved span and neither value is
// evidence-backed for them, so **the direction carries no claim**: the late end
// applies the pre-modern 6 to three seasons on no direct evidence, and the early
// end leaves them on today's 10 on none either. Whichever end this takes, take it
// deliberately, and re-run the confinement if it moves — the affected cells are in
// the banked record at their pre-change values.
const keeperGoalRuleChangeSeason = "2024-25"

// ScoringRulesFor is the points table that was in force in a season.
//
// Seasons are compared as strings, exactly as `BankLimitFor` and
// `DefconScoredIn` compare them: the "YYYY-YY" spelling the archive uses orders
// correctly under `<`.
//
// ⚠️ **A name outside that spelling does NOT sort to the oldest rules, and a
// first version of this comment said it did.** Go compares bytes, so
// `"test" < "2024-25"` is **false** — 't' is 0x74 and '2' is 0x32 — and every
// letter-initial fixture name therefore gets the MODERN table. Only the empty
// string falls below. That is inert on the fixtures in this tree, because none of
// them gives a goalkeeper a goal or an xG, but it is the opposite of what the
// comment claimed and nobody would have found out from a passing test.
//
// ⚠️ **The empty string is the LIVE game and takes today's rules**, and this is
// the one case that had to be written down rather than left to the ordering.
// `""` is the only string that really does sort below every real season, so
// without this clause `NewEngineFull` on a bootstrap fetched from FPL — which
// carries no season name, because the payload does not publish one — would score
// every live goalkeeper's goal at the **pre-2024-25** 6. That is not an
// instrument column: it is `Score`, therefore the ordering, therefore which
// goalkeeper gets bought. `TestTheLiveEngineScoresUnderTodaysRules` fails if this
// clause is removed, because a byte-comparison bug in the direction of the older
// table is silent everywhere else.
func ScoringRulesFor(season string) ScoringRules {
	r := ScoringRules{
		Season:       season,
		Goal:         make(map[int]float64, len(goalPoints)),
		CleanSheet:   make(map[int]float64, len(cleanSheetPoints)),
		ConcedeBlock: make(map[int]int, len(concedeBlock)),
		Assist:       assistPoints,
	}
	// Today's rules first, from the tables TestScoringConstantsMatchFPL pins to
	// FPL's own published game_config. Copied rather than aliased so a caller
	// cannot reach through this struct and mutate the package's constants.
	for pos, v := range goalPoints {
		r.Goal[pos] = v
	}
	for pos, v := range cleanSheetPoints {
		r.CleanSheet[pos] = v
	}
	for pos, v := range concedeBlock {
		r.ConcedeBlock[pos] = v
	}

	// An unnamed season is not an old season — it is no season, which is the live
	// game. Return before the amendments rather than after, so a future amendment
	// cannot re-open the hole this closes by being written above the return.
	//
	// The escape hatch returns here too, and deliberately at the same point: with
	// the pin off, every season IS the live game's table, which is exactly the
	// behaviour that shipped before this file existed.
	if season == "" || !seasonScoringRules {
		return r
	}

	// Then the amendments, backwards. One so far.
	if season < keeperGoalRuleChangeSeason {
		r.Goal[1] = keeperGoalPointsBeforeTheRuleChange
	}
	return r
}

// GoalPoints is what these rules pay a position for a goal.
//
// ⚠️ **It refuses an element_type the table has no entry for, and the test is
// KEY PRESENCE rather than value.** Go's bare map index returns the zero value
// for a missing key and cannot distinguish it from a stored zero — and a stored
// zero is a real FPL rule in this table's siblings, since a forward's clean sheet
// pays exactly 0. So `if v := r.Goal[pos]; v > 0` would be a different question
// from the one being asked, and would additionally refuse any position FPL ever
// stopped paying for a goal. `Prices` is the same test spelled as a predicate;
// this is the accessor for callers that want the number.
//
// # Why a refusal rather than a zero
//
// A zero here is not a small error. It deletes the goals channel and leaves the
// appearance, bonus, saves and card channels realised, so the row comes back at
// the right order of magnitude and reads as a footballer who takes no chances —
// this record's signature failure, a null that looks like a result.
//
// # Where this may and may not fire
//
// It is an **invariant**, not the policy. `Engine.Metrics` refuses to score an
// unpriced position at its own door, before any of the five pricing sites in
// `metrics.go` is reached, because the population that exists — FPL's 2024-25
// assistant managers, `element_type` 5, present in the live GW38 bootstrap as
// `MNG` and in the replay's own bootstrap by `through` 26 (their first archive
// row is GW23; 26 is the earliest entry point the shipped grid carries them at)
// — must not crash a run. This accessor is what fails if a sixth call site is
// ever added inside the door, or if the door is removed.
func (r ScoringRules) GoalPoints(pos int) float64 {
	v, ok := r.Goal[pos]
	if !ok {
		if !unpricedPositionGuard {
			// The bare map index, restored: zero, silently, exactly as before.
			return 0
		}
		panic(fmt.Sprintf("no scoring rules for element_type %d in season %q: "+
			"pricing its goals channel at zero would return a plausible number "+
			"with a whole channel deleted. Give the position a table, or keep it "+
			"out of the scoring path — Engine.Metrics is the door", pos, r.Season))
	}
	return v
}

// ScoringRules reports the points table this engine is scoring through.
//
// Read-only, and for asking *which season's rules arrived* rather than for
// scoring: the engine reads `e.rules` directly. It exists because a pin that is
// never set is a byte-identical null, indistinguishable from a pin that does
// nothing, and the only way to tell the two apart is to ask the engine what it
// got. That is the same argument `DefconCleanFactorFor` and its neighbours in
// `teamstrength.go` make, and like them it has no production callers.
//
// ⚠️ **It returns a COPY, and asking callers to be careful instead was the first
// version.** The maps are the engine's own; a single write through a shared
// handle would re-price every player scored after it *and* be a data race, since
// the tool runner reads one engine from several goroutines at once. A clone costs
// three small map copies on a call nothing makes on the hot path, which is a
// worse trade than a comment only if the comment is always read.
func (e *Engine) ScoringRules() ScoringRules { return e.rules.clone() }

// clone is a deep copy of the three maps, so a returned table cannot reach back
// into the one an engine is scoring through. `ScoringRulesFor` builds its own
// maps from the package constants and does not need it; `Engine.ScoringRules`
// hands out an existing table and does.
func (r ScoringRules) clone() ScoringRules {
	out := ScoringRules{
		Season:       r.Season,
		Goal:         make(map[int]float64, len(r.Goal)),
		CleanSheet:   make(map[int]float64, len(r.CleanSheet)),
		ConcedeBlock: make(map[int]int, len(r.ConcedeBlock)),
		Assist:       r.Assist,
	}
	for k, v := range r.Goal {
		out.Goal[k] = v
	}
	for k, v := range r.CleanSheet {
		out.CleanSheet[k] = v
	}
	for k, v := range r.ConcedeBlock {
		out.ConcedeBlock[k] = v
	}
	return out
}

// CleanSheetPoints is what these rules pay a position for a clean sheet.
//
// ⚠️ **Key presence, and here the reason is unmissable**: a forward's clean sheet
// is **0 and present**, which is a rule. So this returns the stored zero happily
// and refuses only an *absent* key — the exact distinction a bare map index
// cannot make and a `> 0` test gets backwards.
//
// Its production callers are all behind `Engine.Metrics`'s door, so the refusal
// cannot fire on a real position. What it does catch is an `Engine` built as a
// bare literal rather than through `NewEngineFull`: its rules are the zero value,
// every map is nil, and a bare `r.CleanSheet[pos]` would return 0 for a
// goalkeeper — deleting the whole clean-sheet channel while every other term
// still computed. That is not hypothetical. It is what a test in this package was
// doing on the day this accessor was written, and the bare index there returned 0
// for a defender with no error anywhere.
func (r ScoringRules) CleanSheetPoints(pos int) float64 {
	v, ok := r.CleanSheet[pos]
	if !ok {
		if !unpricedPositionGuard {
			return 0
		}
		panic(fmt.Sprintf("no clean-sheet rule for element_type %d in season %q: "+
			"a zero here is a legitimate FPL rule for a forward and an ABSENT key "+
			"is an unresolved table, and the two must not be confused — an Engine "+
			"built outside NewEngineFull has neither", pos, r.Season))
	}
	return v
}

// Prices reports whether these rules price an element_type at all.
//
// ⚠️ **Key presence, never value.** `CleanSheet[4]` is legitimately 0 and
// `ConcedeBlock` legitimately has no entry for midfielders or forwards, so a
// value test cannot tell "FPL pays this position nothing here" from "this
// position is not in the table". `Goal` is the roster because it is the one
// channel FPL pays every playing position for.
//
// The case this is for is `element_type` 5, the assistant managers FPL ran for
// 2024-25 — **322 archive rows, which accumulate to 312 player-gameweeks**
// carrying 1,861 points, and a position the instrument has never had a goal value
// for. (The two counts are the doubles distinction this package is built around:
// `loadGameweeks` accumulates, so a row count is not a player-gameweek count, and
// 312 is what `XPointsResidual` would actually be called on.) Read through a bare
// map index they price at zero and the row scores as if it were a footballer who
// took no chances, which is a null that looks like a result.
//
// ⚠️ **This said "closes the class in the INSTRUMENT only" until the engine was
// brought onto this table**, naming `metrics.go`'s five bare `goalPoints[pos]`
// reads as the larger, open exposure. They are closed now — `Engine.Metrics`
// refuses an unpriced position at the door and the five sites read `GoalPoints`
// — and the two halves are the same table, not two.
//
// ⚠️ It is still not a proof, and two exposures are left standing deliberately.
//
//   - `CleanSheetTermFor` in `teamstrength.go` reads `cleanSheetPoints` directly.
//     It is a calibration accessor with no production callers, and it answers
//     about **today's** rules on purpose — a calibration fitted against the live
//     model wants the live table. If it ever gains a production caller it needs a
//     season, and nothing here would notice.
//   - `appearancePoints`, `defConPoints`, `savesBlock` and the card deductions are
//     package constants read straight, and they are **not** season-pinned. They
//     are forced forward by `TestScoringConstantsMatchFPL` exactly as the four
//     channels here were. That is the same defect, unclosed, on four more terms —
//     declared rather than fixed, because this table is scoped to the channels the
//     xPoints instrument replaces and widening it is a separate decision with its
//     own confinement run to do.
func (r ScoringRules) Prices(pos int) bool {
	_, ok := r.Goal[pos]
	return ok
}
