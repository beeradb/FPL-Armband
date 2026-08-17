package analysis

import "fmt"

// Realised points, decomposed into the channels FPL actually pays.
//
// # Why this exists, and why it is here rather than in a diagnostic
//
// A question this project keeps meeting is what share of a player's return is the
// **appearance floor** — the one or two points FPL pays for turning up, which a
// selected player collects with certainty — against everything else, which he does
// not. Answering it needs realised points split by channel, per match.
//
// It lives in `analysis` because that is where the points table lives. A
// diagnostic in another package would have to carry its own copy of the goal
// values, the concede block, the saves divisor and the card deductions, and this
// record's standing rule is that a diagnostic must never carry its own copy of the
// thing it is checking. `ExpectedCleanSheets` is exported for the same reason.
//
// # Two rule sources, and the second one is an exposure
//
// The four channels `ScoringRules` carries — goals, assists, the clean sheet and
// the concede deduction — are taken from the SEASON's table, so an archived season
// is priced under the rules it was played under. That is what `ScoringRulesFor`
// exists for.
//
// The rest are package constants read straight, and they are **not season-pinned**:
// appearance, saves, the cards, defensive contribution, own goals and the two
// penalty channels. `ScoringRules`' own docstring declares that gap for four of
// them ("that is the same defect, unclosed, on four more terms — declared rather
// than fixed"), and widening that table is a separate decision with its own
// confinement run to do. Nothing here widens it.
//
// What this file adds instead is a way to **measure** the exposure rather than
// argue about it: `DecomposeMatch` returns what it priced, a caller compares the
// total against the archive's own `total_points`, and a season where a rule really
// did change shows up as a non-zero residual on the rows it changed. A silent
// mis-pricing is the failure this record catalogues repeatedly; a residual column
// is what makes it loud.

// AppearanceMinutes is the minute FPL's second appearance point starts at.
//
// **Asserted, not measured, and it cannot be pinned the way the rest of the table
// is**: `bootstrap-static.game_config.scoring` publishes `long_play` and
// `short_play` — the two payouts — and publishes no threshold at all, so
// `TestScoringConstantsMatchFPL` has nothing to compare this against. It is FPL's
// published rule in prose, unchanged for the whole of the archive this repository
// reads.
//
// It is named here rather than left as a bare literal because the decomposition
// below turns on it three times, and because a reader who wants to know whether
// the appearance rule is season-dependent should find one place that answers.
// The bare `60`s elsewhere in this package are the clean-sheet gate, which is a
// different rule that happens to coincide, and they are deliberately left alone.
const AppearanceMinutes = 60

const (
	// ownGoalPoints, penaltySavedPoints and penaltyMissedPoints complete the
	// realised table. FPL publishes all three in `game_config.scoring`
	// (`own_goals` -2, `penalties_saved` 5, `penalties_missed` -2) and
	// TestScoringConstantsMatchFPL asserts them, so they are falsifiable in the
	// same way as the rest — but they are today's values applied to every season,
	// exactly like the four terms ScoringRules' docstring already declares.
	//
	// As positive magnitudes where FPL publishes a negative, matching the house
	// style set by yellowCardPoints and redCardPoints.
	ownGoalPoints       = 2.0
	penaltySavedPoints  = 5.0
	penaltyMissedPoints = 2.0
	// savePoints is what one block of savesBlock saves pays a goalkeeper.
	savePoints = 1.0
)

// RealisedMatch is one player's archive row for ONE match.
//
// A parameter object rather than an argument list, on the same argument
// `XPointsGW` makes: nine of these are counts of different things and a caller
// transposing two would produce a plausible number rather than a compile error.
//
// ⚠️ **One match, not one gameweek.** A double gameweek's two rows must be
// decomposed separately and added afterwards, because three of the channels below
// are per-match step functions — the appearance floor, the concede block and the
// saves block — and none of them survives being applied to a summed row. A 180
// minute gameweek is two appearances of 2, not one.
type RealisedMatch struct {
	Position int // element_type

	Minutes         int
	Goals           int
	Assists         int
	CleanSheets     int // 0 or 1; the archive already applies FPL's sixty-minute gate
	GoalsConceded   int // while on the pitch
	Saves           int
	Bonus           int
	Yellow          int
	Red             int
	OwnGoals        int
	PenaltiesSaved  int
	PenaltiesMissed int

	// DefCon is the raw defensive-action count (CBIT for a defender, CBIRT for
	// everyone else), NOT the award. The threshold is applied below.
	DefCon int
	// DefConPaid says whether this season pays defensive contribution at all.
	//
	// An explicit flag rather than an inference from `DefCon == 0`, because the
	// archive publishes the column only from 2025-26 and every earlier row reads
	// zero — so inferring would price "the category did not exist" identically to
	// "he made no defensive actions". That is this record's byte-identical-null
	// trap in miniature. `backtest.DefconScoredIn` is the one place the season
	// boundary lives; callers pass its answer.
	DefConPaid bool
}

// MatchPoints is one match's realised points, split by channel.
//
// Every field is signed the way it enters the total, so the deductions are
// negative here even though the constants above are magnitudes. Summing the
// fields gives Total.
type MatchPoints struct {
	// Appearance is the only channel a selected player collects with certainty.
	Appearance float64

	Goals      float64
	Assists    float64
	CleanSheet float64
	Conceded   float64
	Saves      float64
	DefCon     float64
	Bonus      float64
	Cards      float64
	OwnGoals   float64
	Penalties  float64
}

// Total is the sum of the channels — what this table says FPL paid.
func (m MatchPoints) Total() float64 {
	return m.Appearance + m.Goals + m.Assists + m.CleanSheet + m.Conceded +
		m.Saves + m.DefCon + m.Bonus + m.Cards + m.OwnGoals + m.Penalties
}

// DecomposeMatch prices one match under the season's rules.
//
// The season enters through `r`, which a caller builds with `ScoringRulesFor`. A
// zero-valued `ScoringRules` is refused rather than pricing every goal at nothing,
// on exactly the argument `XPointsResidual` makes for its own refusal.
func DecomposeMatch(g RealisedMatch, r ScoringRules) MatchPoints {
	if !r.Prices(g.Position) {
		panic(fmt.Sprintf("analysis: DecomposeMatch has no scoring rules for "+
			"element_type %d in season %q — a position with no goal value must not "+
			"be scored as one with a goal value of zero; see ScoringRulesFor",
			g.Position, r.Season))
	}
	var m MatchPoints

	// ⚠️ Cards are priced BEFORE the minutes gate, and that ordering is measured
	// rather than reasoned.
	//
	// A player can be booked or sent off having played no minutes at all — from
	// the bench, in the tunnel, or at half time as an unused substitute — and FPL
	// deducts for it. Measured across the whole archive, 2016-17 to 2025-26: **14
	// rows** carry a card with zero minutes, thirteen yellows and one red (Matheus
	// Nunes, 2022-23 GW28), and every one of them is a `total_points` of −1 or −3.
	// With the deduction behind the gate those rows are the ONLY rows in 253,509
	// that fail to reconcile against the archive's own column.
	//
	// Tiny, and worth the two lines anyway: a decomposition whose residual is
	// identically zero can be trusted as a decomposition, and one that is
	// "basically zero" invites the next reader to attribute a real defect to the
	// same rounding.
	m.Cards = -(float64(g.Yellow)*yellowCardPoints + float64(g.Red)*redCardPoints)

	if g.Minutes <= 0 {
		// No appearance. Every remaining channel needs a player who was on the
		// pitch, so returning here keeps an unplayed match from depending on ten
		// separate facts each being zero.
		return m
	}

	m.Appearance = shortPlayPoints
	if g.Minutes >= AppearanceMinutes {
		m.Appearance = appearancePoints
	}

	m.Goals = float64(g.Goals) * r.Goal[g.Position]
	m.Assists = float64(g.Assists) * r.Assist
	m.CleanSheet = float64(g.CleanSheets) * r.CleanSheet[g.Position]

	// The concede deduction: one point per full block, and absent from the map
	// for the two positions FPL does not deduct from. Key presence rather than a
	// value test, because a block size of zero is a division by zero and not a
	// rule.
	if d, ok := r.ConcedeBlock[g.Position]; ok && d > 0 {
		m.Conceded = -float64(g.GoalsConceded / d)
	}

	m.Saves = float64(g.Saves/savesBlock) * savePoints
	if g.DefConPaid && g.DefCon >= defconThreshold(g.Position) {
		m.DefCon = defConPoints
	}
	m.Bonus = float64(g.Bonus)
	m.OwnGoals = -float64(g.OwnGoals) * ownGoalPoints
	m.Penalties = float64(g.PenaltiesSaved)*penaltySavedPoints -
		float64(g.PenaltiesMissed)*penaltyMissedPoints
	return m
}
