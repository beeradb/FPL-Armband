package fpl

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

// Num handles FPL's habit of returning numbers as JSON strings
// ("expected_goals": "25.50") in some fields and as bare numbers in others.
type Num float64

func (n *Num) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*n = 0
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			// Some fields are non-numeric placeholders; treat as zero.
			*n = 0
			return nil
		}
		*n = Num(f)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		*n = 0
		return nil
	}
	*n = Num(f)
	return nil
}

func (n Num) Float() float64 { return float64(n) }

// Bootstrap is the payload of /api/bootstrap-static/.
type Bootstrap struct {
	// Season is which season's game this payload describes, in the archive's
	// "YYYY-YY" spelling, and it is the ONE field here that FPL does not publish.
	//
	// `json:"-"` because it is not in the response and never will be: the live
	// endpoint serves one season and has no reason to name it. It is empty on
	// everything fetched from the API, and **empty means the live game**, which
	// `analysis.ScoringRulesFor` turns into today's scoring rules.
	//
	// It exists because the *replay* fabricates bootstraps for finished seasons,
	// and FPL's scoring rules have changed inside the archive's span — a
	// goalkeeper's goal paid 6 before the modern 10. Without a season on the
	// payload, every engine built from it would have to be pinned by hand, which
	// is the shape of this project's most expensive wiring bug: `Simulate` builds
	// three engines and a patch once wired two. `Engine` derives its rules from
	// this field in `NewEngineFull`, so the derived engines — `WeekEngine` and
	// the plan's horizon engine, both of which re-construct from `e.Boot` —
	// inherit the pin with no assignment to miss.
	//
	// Set at exactly two places, both of which build a season's bootstrap from a
	// `*backtest.Season` that already carries the name: `PointInTimeWith` and
	// `PreSeasonWith`. `TestNoReplayBootstrapIsBuiltWithoutASeason` parses that
	// package for a third literal and fails if it carries no `Season`, **including
	// one nothing calls yet** — which the behavioural
	// `TestEveryReplayBootstrapCarriesItsSeason` cannot see, because it can only
	// iterate the two constructors that exist.
	Season string `json:"-"`

	Events       []Event       `json:"events"`
	Teams        []Team        `json:"teams"`
	ElementTypes []ElementType `json:"element_types"`
	Elements     []Element     `json:"elements"`
	TotalPlayers int           `json:"total_players"`
	Chips        []Chip        `json:"chips"`
	GameSettings GameSettings  `json:"game_settings"`
	GameConfig   GameConfig    `json:"game_config"`
}

type GameSettings struct {
	SquadSquadsize  int `json:"squad_squadsize"`
	SquadTotalSpend int `json:"squad_total_spend"`
	SquadTeamLimit  int `json:"squad_team_limit"`
}

// GameConfig is FPL's own published points table and squad rules.
//
// It is parsed for one purpose: so a test can assert the model's hardcoded
// scoring constants against the authority rather than against a comment. Nothing
// in the scoring path reads it, deliberately — see TestScoringConstantsMatchFPL
// for why reading it live would be worse than checking it.
//
// This is the fifth time this project has found a number it needed sitting in a
// response it already fetched, after the bank in SquadPrices, per-gameweek
// defensive contribution, fixture kickoff times and FPL's own club strength. The
// standing lesson is to check the payload before recording a limit.
type GameConfig struct {
	Scoring Scoring   `json:"scoring"`
	Rules   GameRules `json:"rules"`

	// ScoringRaw is every key FPL published under `scoring`, kept in raw form
	// alongside the typed view.
	//
	// The typed struct can only check terms somebody already thought of, and this
	// project's costliest scoring gap was the opposite failure: defensive
	// contribution arrived as a NEW category in 2025-26 and the model was blind to
	// it for a season while a test suite full of assertions passed. Retaining the
	// keys lets a test notice a term nobody has named yet — see
	// TestFPLPaysNothingTheModelDoesNotPrice.
	ScoringRaw map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the typed view and retains the raw scoring keys beside
// it. Two passes over the same bytes, because encoding/json cannot deliver both a
// struct and its unknown keys from one.
func (g *GameConfig) UnmarshalJSON(b []byte) error {
	// A local type without this method, or the decode below recurses forever.
	type plain GameConfig
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*g = GameConfig(p)

	var raw struct {
		Scoring map[string]json.RawMessage `json:"scoring"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	g.ScoringRaw = raw.Scoring
	return nil
}

// PaidScoringKeys returns the scoring terms FPL currently awards something for,
// sorted.
//
// Most of the published table is zero — bps, the ICT components, the manager
// terms, the expected-goals fields — so listing every key would make a guard
// against new terms fire on noise. A term with a non-zero value is one the game
// actually pays, which is the set the model must account for.
//
// A per-position term counts as paid if any position is non-zero, so goalkeepers
// being excluded from defensive contribution does not hide the category.
func (g GameConfig) PaidScoringKeys() []string {
	var out []string
	for k, v := range g.ScoringRaw {
		var any1 any
		if err := json.Unmarshal(v, &any1); err != nil {
			continue
		}
		if nonZero(any1) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func nonZero(v any) bool {
	switch t := v.(type) {
	case float64:
		return t != 0
	case bool:
		return t
	case map[string]any:
		for _, e := range t {
			if nonZero(e) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if nonZero(e) {
				return true
			}
		}
	}
	return false
}

// ByPosition is a scoring value that differs by position.
//
// FPL keys these by position short name rather than by the element_type id the
// rest of this program uses, so converting between the two is a place a silent
// mismatch could live — comparing a goalkeeper's constant against a forward's
// published value would still produce a number. Resolve the mapping with
// Bootstrap.PositionByType, which reads it from the payload's own
// element_types rather than assuming 1 is GKP.
type ByPosition struct {
	GKP float64 `json:"GKP"`
	DEF float64 `json:"DEF"`
	MID float64 `json:"MID"`
	FWD float64 `json:"FWD"`
}

// ForShortName returns the value for a position short name, and whether the name
// was recognised. The bool matters: an unrecognised name must not read as zero,
// which is a legitimate published value for several of these terms.
func (b ByPosition) ForShortName(short string) (float64, bool) {
	switch short {
	case "GKP":
		return b.GKP, true
	case "DEF":
		return b.DEF, true
	case "MID":
		return b.MID, true
	case "FWD":
		return b.FWD, true
	}
	return 0, false
}

// PositionByType maps element_type id to position short name, from the payload's
// own element_types rather than from an assumption that 1 is GKP.
//
// Hardcoding that mapping is safe enough in the scoring path, where it has been
// stable for years and is covered by every other test. It is NOT safe in a test
// whose whole job is to detect a mismatch between this program and FPL: a
// renumbering would make the test compare the wrong pairs and still pass.
func (b Bootstrap) PositionByType() map[int]string {
	out := make(map[int]string, len(b.ElementTypes))
	for _, et := range b.ElementTypes {
		out[et.ID] = et.SingularNameShort
	}
	return out
}

// Scoring is the published points table.
//
// Two things it does NOT carry, both of which the model needs and must keep
// hardcoded. The per-match divisors are absent: "saves: 1" without the "per 3"
// and "goals_conceded: -1" without the "per 2". And the defensive-contribution
// thresholds — ten actions for a defender, twelve for everyone else — are absent
// entirely, so the DefensiveContribution values below say what an award is worth
// and not what earns it.
type Scoring struct {
	LongPlay  float64 `json:"long_play"`  // 2, at sixty minutes or more
	ShortPlay float64 `json:"short_play"` // 1, for recording any minutes at all

	GoalsScored           ByPosition `json:"goals_scored"`
	CleanSheets           ByPosition `json:"clean_sheets"`
	GoalsConceded         ByPosition `json:"goals_conceded"`
	DefensiveContribution ByPosition `json:"defensive_contribution"`

	Assists float64 `json:"assists"`
	Saves   float64 `json:"saves"`
	Bonus   float64 `json:"bonus"`

	YellowCards     float64 `json:"yellow_cards"`
	RedCards        float64 `json:"red_cards"`
	OwnGoals        float64 `json:"own_goals"`
	PenaltiesSaved  float64 `json:"penalties_saved"`
	PenaltiesMissed float64 `json:"penalties_missed"`
}

// GameRules is the squad and transfer rulebook.
type GameRules struct {
	SquadSquadplay  int `json:"squad_squadplay"`   // 11 of the 15 score
	SquadSquadsize  int `json:"squad_squadsize"`   // 15
	SquadTeamLimit  int `json:"squad_team_limit"`  // 3 per club
	SquadTotalSpend int `json:"squad_total_spend"` // 1000 tenths = £100.0m

	// TransfersSellOnFee is the share of a price rise the seller does NOT keep:
	// 0.5, so an owned player must rise £0.2m before the selling price moves
	// £0.1m. This project reconstructs selling prices from it and had it
	// hardcoded.
	TransfersSellOnFee float64 `json:"transfers_sell_on_fee"`

	// MaxExtraFreeTransfers is 4, and the bank limit is this plus the one earned
	// each week — which is where 5 comes from.
	MaxExtraFreeTransfers int `json:"max_extra_free_transfers"`

	ElementSellAtPurchasePrice bool `json:"element_sell_at_purchase_price"`

	// ViceCaptainEnabled confirms from FPL's own payload that the armband passes
	// when the captain records no minutes.
	ViceCaptainEnabled bool `json:"sys_vice_captain_enabled"`

	// UICurrencyMultiplier is 10: prices are published in tenths of a million.
	UICurrencyMultiplier int `json:"ui_currency_multiplier"`
}

type Chip struct {
	Name       string `json:"name"`
	Number     int    `json:"number"`
	StartEvent int    `json:"start_event"`
	StopEvent  int    `json:"stop_event"`
}

type Event struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	DeadlineTime    time.Time `json:"deadline_time"`
	Finished        bool      `json:"finished"`
	IsPrevious      bool      `json:"is_previous"`
	IsCurrent       bool      `json:"is_current"`
	IsNext          bool      `json:"is_next"`
	AverageScore    int       `json:"average_entry_score"`
	HighestScore    *int      `json:"highest_score"`
	MostCaptainedID *int      `json:"most_captained"`
}

type Team struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Position  int    `json:"position"`
	Points    int    `json:"points"`
	Played    int    `json:"played"`
	// Strength is FPL's coarse 1-5 rating. Pre-season it is the *only* one
	// populated, and confusingly it arrives in strength_overall_home while this
	// field is null; the granular 1000-1400 ratings appear closer to the season.
	Strength            int `json:"strength"`
	StrengthOverallHome int `json:"strength_overall_home"`
	StrengthOverallAway int `json:"strength_overall_away"`
	StrengthAttackHome  int `json:"strength_attack_home"`
	StrengthAttackAway  int `json:"strength_attack_away"`
	StrengthDefenceHome int `json:"strength_defence_home"`
	StrengthDefenceAway int `json:"strength_defence_away"`
}

type ElementType struct {
	ID                int    `json:"id"`
	SingularName      string `json:"singular_name"`
	SingularNameShort string `json:"singular_name_short"`
	SquadSelect       int    `json:"squad_select"`
	SquadMinPlay      int    `json:"squad_min_play"`
	SquadMaxPlay      int    `json:"squad_max_play"`
}

// Element is one player. Note that between seasons, the aggregate stat fields
// (minutes, expected_goals, total_points...) still hold the PREVIOUS season's
// totals until the first gameweek of the new season completes.
type Element struct {
	ID          int    `json:"id"`
	Code        int    `json:"code"`
	FirstName   string `json:"first_name"`
	SecondName  string `json:"second_name"`
	WebName     string `json:"web_name"`
	Team        int    `json:"team"`
	ElementType int    `json:"element_type"`

	// Region is an opaque nationality code. FPL publishes no lookup table for
	// it, but players sharing a code share a nationality, which is enough to
	// group a squad by international commitments.
	Region *int `json:"region"`
	// TeamJoinDate identifies summer signings, whose aggregate stats were
	// accumulated at a different club.
	TeamJoinDate string `json:"team_join_date"`
	BirthDate    string `json:"birth_date"`

	NowCost         int `json:"now_cost"`
	CostChangeEvent int `json:"cost_change_event"`
	CostChangeStart int `json:"cost_change_start"`

	Status                   string     `json:"status"` // a=available d=doubtful i=injured s=suspended u=unavailable n=not in squad
	News                     string     `json:"news"`
	NewsAdded                *time.Time `json:"news_added"`
	ChanceOfPlayingThisRound *int       `json:"chance_of_playing_this_round"`
	ChanceOfPlayingNextRound *int       `json:"chance_of_playing_next_round"`

	TotalPoints   int `json:"total_points"`
	EventPoints   int `json:"event_points"`
	Minutes       int `json:"minutes"`
	Starts        int `json:"starts"`
	GoalsScored   int `json:"goals_scored"`
	Assists       int `json:"assists"`
	CleanSheets   int `json:"clean_sheets"`
	GoalsConceded int `json:"goals_conceded"`
	Saves         int `json:"saves"`
	Bonus         int `json:"bonus"`
	BPS           int `json:"bps"`
	YellowCards   int `json:"yellow_cards"`
	RedCards      int `json:"red_cards"`

	ExpectedGoals              Num `json:"expected_goals"`
	ExpectedAssists            Num `json:"expected_assists"`
	ExpectedGoalInvolvements   Num `json:"expected_goal_involvements"`
	ExpectedGoalsConceded      Num `json:"expected_goals_conceded"`
	ExpectedGoalsPer90         Num `json:"expected_goals_per_90"`
	ExpectedAssistsPer90       Num `json:"expected_assists_per_90"`
	ExpectedGIPer90            Num `json:"expected_goal_involvements_per_90"`
	ExpectedGCPer90            Num `json:"expected_goals_conceded_per_90"`
	CleanSheetsPer90           Num `json:"clean_sheets_per_90"`
	SavesPer90                 Num `json:"saves_per_90"`
	StartsPer90                Num `json:"starts_per_90"`
	DefensiveContribution      int `json:"defensive_contribution"`
	DefensiveContributionPer90 Num `json:"defensive_contribution_per_90"`

	Influence  Num `json:"influence"`
	Creativity Num `json:"creativity"`
	Threat     Num `json:"threat"`
	ICTIndex   Num `json:"ict_index"`

	Form              Num `json:"form"`
	PointsPerGame     Num `json:"points_per_game"`
	EPThis            Num `json:"ep_this"`
	EPNext            Num `json:"ep_next"`
	ValueForm         Num `json:"value_form"`
	ValueSeason       Num `json:"value_season"`
	SelectedByPercent Num `json:"selected_by_percent"`

	TransfersIn       int `json:"transfers_in"`
	TransfersOut      int `json:"transfers_out"`
	TransfersInEvent  int `json:"transfers_in_event"`
	TransfersOutEvent int `json:"transfers_out_event"`

	PenaltiesOrder            *int   `json:"penalties_order"`
	PenaltiesText             string `json:"penalties_text"`
	CornersAndIndirectFKOrder *int   `json:"corners_and_indirect_freekicks_order"`
	CornersAndIndirectFKText  string `json:"corners_and_indirect_freekicks_text"`
	DirectFreekicksOrder      *int   `json:"direct_freekicks_order"`
	DirectFreekicksText       string `json:"direct_freekicks_text"`
}

// Fixture is one match from /api/fixtures/.
//
// Finished and FinishedProvisional are not the same signal. Verified live against
// the production API, 2026-08-22: a fixture with a locked-in final score and its
// bonus points already posted in each player's stats can still read
// Finished == false for 16+ hours after full time — Finished tracks some later,
// unpredictably-delayed administrative confirmation, not the final whistle.
// FinishedProvisional flips at full time, once the match's own numbers (goals,
// bonus, defensive contributions) are locked in. Anywhere a caller needs "this
// match's stats are final and safe to treat as a completed unit of evidence",
// that is FinishedProvisional, not Finished.
type Fixture struct {
	ID                  int        `json:"id"`
	Event               *int       `json:"event"`
	KickoffTime         *time.Time `json:"kickoff_time"`
	Finished            bool       `json:"finished"`
	FinishedProvisional bool       `json:"finished_provisional"`
	Started             bool       `json:"started"`
	TeamH               int        `json:"team_h"`
	TeamA               int        `json:"team_a"`
	TeamHScore          *int       `json:"team_h_score"`
	TeamAScore          *int       `json:"team_a_score"`
	TeamHDifficulty     int        `json:"team_h_difficulty"`
	TeamADifficulty     int        `json:"team_a_difficulty"`
}

// ElementSummary is /api/element-summary/{id}/.
type ElementSummary struct {
	Fixtures    []PlayerFixture `json:"fixtures"`
	History     []HistoryEntry  `json:"history"`
	HistoryPast []PastSeason    `json:"history_past"`
}

type PlayerFixture struct {
	ID          int        `json:"id"`
	Event       *int       `json:"event"`
	EventName   string     `json:"event_name"`
	IsHome      bool       `json:"is_home"`
	Difficulty  int        `json:"difficulty"`
	TeamH       int        `json:"team_h"`
	TeamA       int        `json:"team_a"`
	KickoffTime *time.Time `json:"kickoff_time"`
	Finished    bool       `json:"finished"`
}

type HistoryEntry struct {
	Element      int  `json:"element"`
	Fixture      int  `json:"fixture"`
	Round        int  `json:"round"`
	OpponentTeam int  `json:"opponent_team"`
	WasHome      bool `json:"was_home"`
	TotalPoints  int  `json:"total_points"`
	Minutes      int  `json:"minutes"`
	Starts       int  `json:"starts"`
	GoalsScored  int  `json:"goals_scored"`
	Assists      int  `json:"assists"`
	CleanSheets  int  `json:"clean_sheets"`
	Bonus        int  `json:"bonus"`
	BPS          int  `json:"bps"`
	Value        int  `json:"value"`

	ExpectedGoals            Num `json:"expected_goals"`
	ExpectedAssists          Num `json:"expected_assists"`
	ExpectedGoalInvolvements Num `json:"expected_goal_involvements"`
	ExpectedGoalsConceded    Num `json:"expected_goals_conceded"`
}

type PastSeason struct {
	SeasonName  string `json:"season_name"`
	StartCost   int    `json:"start_cost"`
	EndCost     int    `json:"end_cost"`
	TotalPoints int    `json:"total_points"`
	Minutes     int    `json:"minutes"`
	Starts      int    `json:"starts"`
	GoalsScored int    `json:"goals_scored"`
	Assists     int    `json:"assists"`
	CleanSheets int    `json:"clean_sheets"`
	Bonus       int    `json:"bonus"`

	GoalsConceded         int `json:"goals_conceded"`
	Saves                 int `json:"saves"`
	YellowCards           int `json:"yellow_cards"`
	RedCards              int `json:"red_cards"`
	DefensiveContribution int `json:"defensive_contribution"`

	ExpectedGoals            Num `json:"expected_goals"`
	ExpectedAssists          Num `json:"expected_assists"`
	ExpectedGoalInvolvements Num `json:"expected_goal_involvements"`
	ExpectedGoalsConceded    Num `json:"expected_goals_conceded"`
}

// Entry is /api/entry/{id}/ — a manager's overall record.
type Entry struct {
	ID                   int    `json:"id"`
	Name                 string `json:"name"`
	PlayerFirstName      string `json:"player_first_name"`
	PlayerLastName       string `json:"player_last_name"`
	SummaryOverallPoints int    `json:"summary_overall_points"`
	SummaryOverallRank   int    `json:"summary_overall_rank"`
	SummaryEventPoints   int    `json:"summary_event_points"`
	SummaryEventRank     *int   `json:"summary_event_rank"`
	CurrentEvent         *int   `json:"current_event"`
	LastDeadlineBank     *int   `json:"last_deadline_bank"`
	LastDeadlineValue    *int   `json:"last_deadline_value"`
}

// EntryPicks is /api/entry/{id}/event/{event}/picks/.
type EntryPicks struct {
	ActiveChip   *string `json:"active_chip"`
	EntryHistory struct {
		Event              int  `json:"event"`
		Points             int  `json:"points"`
		TotalPoints        int  `json:"total_points"`
		Rank               *int `json:"rank"`
		OverallRank        *int `json:"overall_rank"`
		Bank               int  `json:"bank"`
		Value              int  `json:"value"`
		EventTransfers     int  `json:"event_transfers"`
		EventTransfersCost int  `json:"event_transfers_cost"`
	} `json:"entry_history"`
	Picks []Pick `json:"picks"`
}

type Pick struct {
	Element       int  `json:"element"`
	Position      int  `json:"position"`
	Multiplier    int  `json:"multiplier"`
	IsCaptain     bool `json:"is_captain"`
	IsViceCaptain bool `json:"is_vice_captain"`
}

// EntryHistory is /api/entry/{id}/history/ — per-gameweek record plus chips played.
type EntryHistory struct {
	Current []struct {
		Event              int  `json:"event"`
		Points             int  `json:"points"`
		TotalPoints        int  `json:"total_points"`
		Rank               *int `json:"rank"`
		OverallRank        *int `json:"overall_rank"`
		Bank               int  `json:"bank"`
		Value              int  `json:"value"`
		EventTransfers     int  `json:"event_transfers"`
		EventTransfersCost int  `json:"event_transfers_cost"`
		PointsOnBench      int  `json:"points_on_bench"`
	} `json:"current"`
	Chips []struct {
		Name  string `json:"name"`
		Event int    `json:"event"`
	} `json:"chips"`
}

// EventLive is /api/event/{id}/live/ — every player's stats for one gameweek, live
// during play and final once FPL finishes scoring it.
type EventLive struct {
	Elements []LiveElement `json:"elements"`
}

type LiveElement struct {
	ID    int       `json:"id"`
	Stats LiveStats `json:"stats"`
}

// LiveStats is the subset of FPL's live payload this codebase reads. The real
// response carries many more fields (BPS, ICT, expected goals...); only what the
// spectator team page draws is parsed here, matching this package's practice
// elsewhere of not carrying a field nothing reads.
type LiveStats struct {
	Minutes               int `json:"minutes"`
	GoalsScored           int `json:"goals_scored"`
	Assists               int `json:"assists"`
	CleanSheets           int `json:"clean_sheets"`
	Saves                 int `json:"saves"`
	DefensiveContribution int `json:"defensive_contribution"`
}

// ByID looks up one player's live stats for this gameweek, nil if he is not in
// the payload. A live event's elements list is keyed by the same permanent
// element id Bootstrap uses, so a caller already holding one (from
// Bootstrap.ElementByID, say) can chain straight through.
func (el *EventLive) ByID(id int) *LiveStats {
	if el == nil {
		return nil
	}
	for i := range el.Elements {
		if el.Elements[i].ID == id {
			return &el.Elements[i].Stats
		}
	}
	return nil
}

// HasExpected reports whether this past season's `expected_goals`,
// `expected_assists` and `expected_goals_conceded` mean anything.
//
// # FPL returns an explicit zero, not an absent field
//
// Checked live on 2026-08-13 against players whose careers span the boundary —
// Dunk from 2017/18 and Gabriel from 2020/21 — every season before 2022/23
// carries `"0.00"` for all three while recording three thousand real minutes.
// The statistics did not exist yet: FPL introduced them in December 2022.
//
// So this is the "unknown against zero" distinction the rest of this codebase
// keeps in the type system (`Engine.Bank` is a `*int` for the same reason), and
// here the payload gives us no room to keep it there — a float64 arrives either
// way. The consequence is that any caller reading these fields for an old season
// receives a *number*, and a centre-half with 3,151 minutes reads as having
// conceded nothing expected. A hole that arrives as data.
//
// # Why a season boundary rather than a zero test
//
// Testing `xGC == 0` would be wrong in both directions and the second is the
// dangerous one. A goalkeeper genuinely records `expected_goals` of 0.00 in a
// season the data covers — Pope did in 2022/23 — so a zero test would silently
// discard a real observation. The boundary is a fact about the feed, not about
// the player, and this project already gates the transfer bank, defensive
// contribution and the expected-goals repair by season for the same reason: a
// rule keyed on a value is a rule nobody can reason about later.
func (p PastSeason) HasExpected() bool { return p.seasonStartYear() >= 2022 }

// HasDefCon reports whether `defensive_contribution` means anything.
//
// ⚠️ **This boundary is 2024/25 and it is NOT the archive's**, which carries the
// per-gameweek column only for 2025-26 — see AGENTS.md's season table. FPL has
// back-filled the season aggregate one year further than the weekly series, so
// the live path can see a season the replay cannot. Checked live: Dunk 126 and
// Gabriel 159 in 2024/25, both 0 in 2023/24 and every season before it.
//
// Two boundaries for one quantity is worth stating rather than reconciling: they
// describe different sources, and the API's is the wider one.
func (p PastSeason) HasDefCon() bool { return p.seasonStartYear() >= 2024 }

// seasonStartYear parses the opening year out of FPL's "2018/19" label.
//
// Returns 0 when the label is not that shape, which makes both predicates above
// answer "no data" — the safe direction, since it drops a season from the
// statistics it cannot supply rather than blending a zero into them.
func (p PastSeason) seasonStartYear() int {
	if len(p.SeasonName) < 4 {
		return 0
	}
	y, err := strconv.Atoi(p.SeasonName[:4])
	if err != nil {
		return 0
	}
	return y
}
