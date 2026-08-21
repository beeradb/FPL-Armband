package analysis

import (
	"context"
	"math"
	"testing"
	"time"

	"armband/internal/fpl"
)

// FPL publishes its entire points table and squad rulebook in `bootstrap-static`
// under `game_config`, in a response this program already fetches on every run,
// and for the life of the project nothing parsed it — `GameSettings` read three
// fields and stopped. Every scoring constant in `metrics.go` was therefore
// asserted only by a comment next to it.
//
// # Why this is a test and not a wiring change
//
// The obvious move is to READ the table at runtime instead of hardcoding it. That
// would be worse, for a reason this project has already paid for twice.
//
// The replay scores four archived seasons, and each was played under the rules in
// force at the time. `BankLimitFor` exists precisely because FPL changed the
// transfer bank from 2 to 5 for 2024-25, and `DefconScoredIn` exists because
// defensive contribution did not exist before 2025-26 — in both cases the fix was
// to pin the rule to the season rather than to take today's value. A model that
// read live scoring constants would silently re-score 2022-23 under 2026/27 rules
// and every recorded figure in AGENTS.md would become incomparable with itself.
//
// So the constants stay hardcoded and this test makes them falsifiable. That is
// the same shape as `SquadPrices.Exact`, which checks a reconstruction against
// FPL's own published team value: a number that can be shown wrong is worth more
// than one that can only be trusted.
//
// # The position mapping is derived, not assumed
//
// FPL keys the per-position values by short name and this program keys them by
// `element_type` id. The mapping is read from the payload's own `element_types`,
// because a test whose job is to catch a mismatch must not contain the assumption
// it is checking — if FPL ever renumbered the positions, a hardcoded map would
// compare a keeper against a forward's published value and still pass.
func scoringConfig(t *testing.T) (fpl.GameConfig, map[int]string) {
	t.Helper()
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	boot, err := c.Bootstrap(context.Background())
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	gc := boot.GameConfig
	// An absent or empty game_config must not read as "everything is zero", which
	// would make every assertion below vacuously comparable against a model
	// constant of zero. Nothing here is zero in a real payload.
	if gc.Scoring.LongPlay == 0 && gc.Scoring.Assists == 0 {
		t.Skip("game_config.scoring absent or empty — FPL may have moved it; " +
			"re-check the payload shape before trusting this test's silence")
	}
	pos := boot.PositionByType()
	for _, want := range []int{1, 2, 3, 4} {
		if pos[want] == "" {
			t.Fatalf("element_type %d has no short name in element_types; the "+
				"position mapping this test derives is incomplete", want)
		}
	}
	return gc, pos
}

// TestScoringConstantsMatchFPL asserts every hardcoded points value against FPL's
// own published table.
func TestScoringConstantsMatchFPL(t *testing.T) {
	gc, pos := scoringConfig(t)
	sc := gc.Scoring

	// Per-position terms. Each model map is keyed by element_type, each published
	// value by short name, and the comparison goes through the derived mapping.
	perPosition := []struct {
		what      string
		model     map[int]float64
		published fpl.ByPosition
	}{
		{"goals scored", goalPoints, sc.GoalsScored},
		{"clean sheets", cleanSheetPoints, sc.CleanSheets},
	}
	for _, c := range perPosition {
		for elementType, short := range pos {
			want, ok := c.published.ForShortName(short)
			if !ok {
				t.Errorf("%s: FPL publishes no value for position %q "+
					"(element_type %d)", c.what, short, elementType)
				continue
			}
			if got := c.model[elementType]; got != want {
				t.Errorf("%s for %s: model has %g, FPL publishes %g",
					c.what, short, got, want)
			}
		}
	}

	// Flat terms. appearancePoints is long_play — the value a per-90 rate should
	// carry, since a full match always clears the hour — and shortPlayPoints is
	// the single point for turning up at all, which the model paid nothing for
	// until recently.
	flat := []struct {
		what            string
		model           float64
		published       float64
		publishedSigned bool // FPL publishes a negative; the model subtracts a positive
	}{
		{"assists", assistPoints, sc.Assists, false},
		{"appearance at sixty minutes (long_play)", appearancePoints, sc.LongPlay, false},
		{"appearance for any minutes (short_play)", shortPlayPoints, sc.ShortPlay, false},
		{"yellow card", yellowCardPoints, sc.YellowCards, true},
		{"red card", redCardPoints, sc.RedCards, true},
		// The three channels only the realised decomposition prices. The model
		// predicts none of them — nobody forecasts an own goal — but
		// DecomposeMatch has to pay them to reconcile against the archive's
		// total_points, and an unchecked constant there would show up as a
		// residual blamed on something else.
		{"own goal", ownGoalPoints, sc.OwnGoals, true},
		{"penalty saved", penaltySavedPoints, sc.PenaltiesSaved, false},
		{"penalty missed", penaltyMissedPoints, sc.PenaltiesMissed, true},
	}
	for _, c := range flat {
		want := c.published
		if c.publishedSigned {
			want = -want
		}
		if c.model != want {
			t.Errorf("%s: model has %g, FPL publishes %g", c.what, c.model, c.published)
		}
	}

	// Defensive contribution: the award is worth 2, and goalkeepers are excluded.
	// The model expresses the exclusion through defconThreshold rather than
	// through a per-position value, so both halves are checked.
	for elementType, short := range pos {
		want, _ := sc.DefensiveContribution.ForShortName(short)
		if short == "GKP" {
			if want != 0 {
				t.Errorf("FPL now pays goalkeepers %g for defensive contribution; "+
					"the model excludes them entirely", want)
			}
			continue
		}
		if want != defConPoints {
			t.Errorf("defensive contribution for %s: model has %g, FPL publishes %g",
				short, defConPoints, want)
		}
		if defconThreshold(elementType) <= 0 {
			t.Errorf("%s earns %g for defensive contribution but the model has no "+
				"threshold for element_type %d", short, want, elementType)
		}
	}

	// Goals conceded. FPL publishes the per-block deduction and not the block
	// size, so what is checkable is WHICH positions take it and that the
	// magnitude is one point — the model's poissonFloorDiv credits whole blocks
	// at 1.0 with no multiplier, so a change to −2 per block would be silent.
	for elementType, short := range pos {
		published, _ := sc.GoalsConceded.ForShortName(short)
		modelHasIt := concedeBlock[elementType] > 0
		publishedHasIt := published != 0
		if modelHasIt != publishedHasIt {
			t.Errorf("goals conceded for %s: model deducts=%v, FPL publishes %g",
				short, modelHasIt, published)
		}
		if publishedHasIt && math.Abs(published) != 1 {
			t.Errorf("goals conceded for %s: FPL publishes %g per block, but the "+
				"model credits whole blocks at 1.0 with no multiplier — the "+
				"deduction size is not expressible without a code change",
				short, published)
		}
	}

	// Saves. Same shape: the value is published, the divisor is not.
	if sc.Saves != 1 {
		t.Errorf("FPL publishes %g points per save block; the model credits whole "+
			"blocks at 1.0 with no multiplier", sc.Saves)
	}

	// Bonus is added from a historical rate rather than modelled per match, so
	// there is no constant to compare — but a change away from 1 point per bonus
	// unit would invalidate that rate's scale.
	if sc.Bonus != 1 {
		t.Errorf("FPL publishes %g per bonus unit; Bonus90 assumes 1, so the "+
			"historical rate is no longer in points", sc.Bonus)
	}
}

// TestSquadAndTransferRulesMatchFPL asserts the squad rulebook, including three
// tables the model hardcodes that FPL publishes per position.
func TestSquadAndTransferRulesMatchFPL(t *testing.T) {
	gc, _ := scoringConfig(t)
	r := gc.Rules

	if r.SquadSquadsize != SquadSize {
		t.Errorf("squad size: model has %d, FPL publishes %d", SquadSize, r.SquadSquadsize)
	}
	if r.SquadTeamLimit != MaxPerClub {
		t.Errorf("players per club: model has %d, FPL publishes %d", MaxPerClub, r.SquadTeamLimit)
	}
	if r.SquadTotalSpend != DefaultBudget {
		t.Errorf("opening budget: model has %d tenths, FPL publishes %d",
			DefaultBudget, r.SquadTotalSpend)
	}
	// The eleven that scores. Checked against the model's formation bounds rather
	// than a constant, because the model has no single "11" — it enforces the
	// count through xiMin and xiMax, whose maxima must be able to reach it and
	// whose minima must not exceed it.
	var minSum, maxSum int
	for _, v := range xiMin {
		minSum += v
	}
	for _, v := range xiMax {
		maxSum += v
	}
	if r.SquadSquadplay < minSum || r.SquadSquadplay > maxSum {
		t.Errorf("FPL fields %d players; the model's formation bounds span %d to %d",
			r.SquadSquadplay, minSum, maxSum)
	}

	// The bank limit is max_extra_free_transfers plus the one earned each week.
	// This is where 5 comes from, and it was 2 before 2024-25 — which is why the
	// replay pins it per season instead of reading it.
	if got, want := r.MaxExtraFreeTransfers+1, 5; got != want {
		t.Errorf("FPL now banks %d free transfers (max_extra_free_transfers=%d); "+
			"the current-season rule the replay uses says %d — check BankLimitFor",
			got, r.MaxExtraFreeTransfers, want)
	}

	// The selling fee: half of any rise is kept by the game, which is what makes a
	// squad's selling value lower than its market value and is reconstructed by
	// SquadPrices.
	if r.TransfersSellOnFee != 0.5 {
		t.Errorf("FPL publishes a sell-on fee of %g; the selling-price "+
			"reconstruction assumes half of any rise", r.TransfersSellOnFee)
	}
	if r.ElementSellAtPurchasePrice {
		t.Error("FPL now sells at purchase price, so the sell-on fee is moot and " +
			"the selling-price reconstruction is modelling a rule that is gone")
	}

	// The vice-captain rule, confirmed from FPL's own payload rather than from
	// the observation that the replay was forfeiting the bonus.
	if !r.ViceCaptainEnabled {
		t.Error("FPL reports the vice-captain disabled; the replay passes the " +
			"armband when the captain records no minutes")
	}

	// Prices are in tenths of a million everywhere in this program.
	if r.UICurrencyMultiplier != 10 {
		t.Errorf("FPL publishes a currency multiplier of %d; every price in this "+
			"program is a tenth of a million", r.UICurrencyMultiplier)
	}
}

// TestSquadQuotasAndFormationBoundsMatchFPL checks the three per-position tables
// against `element_types`, which publishes all of them.
//
// These are the constraints the optimiser searches under, so an error here would
// not mis-score a player — it would make every squad the program has ever
// produced illegal, or legal and needlessly restricted.
func TestSquadQuotasAndFormationBoundsMatchFPL(t *testing.T) {
	c := fpl.New(t.TempDir(), 24*time.Hour, 24*time.Hour)
	boot, err := c.Bootstrap(context.Background())
	if err != nil {
		t.Skipf("FPL API unreachable: %v", err)
	}
	if len(boot.ElementTypes) == 0 {
		t.Skip("no element_types in payload")
	}
	for _, et := range boot.ElementTypes {
		short := et.SingularNameShort
		if got, want := squadQuota[short], et.SquadSelect; got != want {
			t.Errorf("squad quota for %s: model has %d, FPL publishes %d", short, got, want)
		}
		if got, want := xiMin[short], et.SquadMinPlay; got != want {
			t.Errorf("minimum starting %s: model has %d, FPL publishes %d", short, got, want)
		}
		if got, want := xiMax[short], et.SquadMaxPlay; got != want {
			t.Errorf("maximum starting %s: model has %d, FPL publishes %d", short, got, want)
		}
	}
}

// TestFPLPaysNothingTheModelDoesNotPrice is the inverse guard, and it is the one
// worth having.
//
// Every assertion above checks a term somebody already thought of. This
// project's costliest scoring gap was the opposite failure: **defensive
// contribution arrived as a new category in 2025-26 and the model scored it as
// zero for a season** while a full test suite passed, because nothing was
// watching for a term nobody had named. It is worth 0.84 of a defender's per-90
// score and the largest term after appearance for a defensive one.
//
// So this asserts a closed set: of the terms FPL currently *pays* something for,
// every one is either priced by the model or listed here with the reason it is
// not. It fails when FPL starts paying for anything new — which is the cheapest
// possible way to notice.
//
// Terms published at zero are ignored deliberately. Roughly twenty of the
// thirty-five keys are zero — bps, the ICT components, the manager terms, the
// expected-goals fields — and treating them as scoring categories would make this
// fire constantly on things the game does not pay.
func TestFPLPaysNothingTheModelDoesNotPrice(t *testing.T) {
	gc, _ := scoringConfig(t)

	// Priced: the model has a constant for it and baseXP90 uses it.
	priced := map[string]string{
		"long_play":              "appearancePoints",
		"short_play":             "shortPlayPoints",
		"goals_scored":           "goalPoints",
		"assists":                "assistPoints",
		"clean_sheets":           "cleanSheetPoints",
		"goals_conceded":         "concedeBlock",
		"saves":                  "savesBlock",
		"defensive_contribution": "defConPoints",
		"bonus":                  "Bonus90, from the player's historical rate",
		"yellow_cards":           "yellowCardPoints",
		"red_cards":              "redCardPoints",
	}

	// Deliberately not priced, each with the reason. AGENTS.md records the
	// measurement behind the first three: 11, 14 and 29 occurrences across a whole
	// league season, worth a fraction of a point each.
	//
	// "Not priced" means the FORECAST does not price them — `baseXP90` has no term
	// for an own goal and never will. `DecomposeMatch`, which splits a match FPL
	// has already paid for, does price all three, because a realised decomposition
	// that omitted them would push them into a residual and blame them on
	// whichever channel the reader was looking at.
	ignored := map[string]string{
		"penalties_saved":    "5 pts but 11 occurrences a season across the league",
		"penalties_missed":   "−2 pts, 14 occurrences a season",
		"own_goals":          "−2 pts, 29 occurrences a season",
		"special_multiplier": "not a scoring term; a multiplier FPL applies itself",
	}

	for _, k := range gc.PaidScoringKeys() {
		if _, ok := priced[k]; ok {
			continue
		}
		if _, ok := ignored[k]; ok {
			continue
		}
		t.Errorf("FPL pays for %q and the model prices nothing for it. If it is a "+
			"new scoring category, this is the defensive-contribution failure "+
			"repeating — that one cost a defender 0.84 points per 90 for a whole "+
			"season while every test passed. Add a term, or add %q to the ignored "+
			"set with the reason and the size.", k, k)
	}

	// And the other direction: a term the model prices that FPL has stopped
	// paying for. Silent over-crediting rather than under-crediting.
	paid := map[string]bool{}
	for _, k := range gc.PaidScoringKeys() {
		paid[k] = true
	}
	for k, where := range priced {
		if !paid[k] {
			t.Errorf("the model prices %q (%s) but FPL no longer pays for it — "+
				"every player carrying that term is now over-credited", k, where)
		}
	}
}

// TestWhatTheScoringTableCannotVerify records the three quantities the model must
// keep guessing, so nobody reads the assertions above as covering them.
//
//   - the saves divisor — "saves: 1" with no "per 3";
//   - the goals-conceded divisor — "goals_conceded: −1" with no "per 2";
//   - the defensive-contribution thresholds, ten actions for a defender and
//     twelve for everyone else, absent entirely.
//
// **The saves divisor is the uncomfortable one.** FPL restructured goalkeeper
// saves for 2026/27, so the single scoring term most likely to have moved is one
// of the three the authority does not publish. This test cannot check it; it
// exists so that limitation is stated in code rather than rediscovered.
//
// It asserts only that the model still HAS its own value for each, which is a
// weak claim on purpose — the point is the doc comment.
func TestWhatTheScoringTableCannotVerify(t *testing.T) {
	if savesBlock <= 0 {
		t.Error("no saves divisor, and FPL does not publish one")
	}
	for pos := range concedeBlock {
		if concedeBlock[pos] <= 0 {
			t.Errorf("no goals-conceded divisor for element_type %d, and FPL does "+
				"not publish one", pos)
		}
	}
	for _, pos := range []int{2, 3, 4} {
		if defconThreshold(pos) <= 0 {
			t.Errorf("no defensive-contribution threshold for element_type %d, and "+
				"FPL does not publish one", pos)
		}
	}
}
