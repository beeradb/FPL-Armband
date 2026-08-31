package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/toolrunner"

	"armband/internal/analysis"
	"armband/internal/config"
	"armband/internal/fpl"
)

// Toolbox wires the FPL data layer into tools Claude can call.
type Toolbox struct {
	Client *fpl.Client
	Engine *analysis.Engine
	Cfg    config.Config
	// ConfigPath lets competition-status corrections persist between runs.
	ConfigPath string

	// TeamPath is the owner's team file, and may be empty. A LOCK is the only
	// thing this toolbox writes that lives there rather than in ConfigPath —
	// `set_player_status` modes "lock" and "start" both write `Roster.Lock` —
	// and config.SavePair refuses such a change when this is empty rather than
	// dropping it or writing a key config.Load would then refuse.
	TeamPath string

	// OnCall is invoked before each tool executes, for terminal progress.
	OnCall func(name, summary string)

	// mu guards read-modify-write of Cfg. The tool runner fans calls out
	// through an errgroup, so several tools can be persisting config at the
	// same moment — see updateConfig.
	mu sync.Mutex
}

// updateConfig applies a change to the persisted config under a lock.
//
// Every config-writing tool is a read-modify-write of the whole file, and the
// tool runner executes tools concurrently. Without the lock, three
// set_player_status calls issued in one turn each start from the same snapshot,
// each add their own player, and each write the entire file — so two of the
// three findings vanish with no error anywhere.
//
// That is not hypothetical: the first live run after the tool shipped recorded
// five overrides and persisted two. The batches were (Isak, Saliba, Dubravka)
// and (Raya, Gabriel); the survivors were Dubravka and Gabriel, the last writer
// in each.
func (t *Toolbox) updateConfig(mutate func(cfg *config.Config) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	cfg := t.Cfg
	if err := mutate(&cfg); err != nil {
		return err
	}
	if t.ConfigPath != "" {
		// SavePair, not Save: a mutation may have touched a setting that lives
		// in the team file (a lock, from set_player_status). Save would drop it
		// silently, because those fields are `json:"-"` on Config. SavePair
		// writes it where it belongs, or refuses when there is nowhere to put
		// it — and refuses before writing anything, so t.Cfg below is not
		// advanced past a change that did not persist.
		if err := config.SavePair(t.ConfigPath, t.TeamPath, t.Cfg, cfg); err != nil {
			return err
		}
	}
	t.Cfg = cfg
	return nil
}

func textResult(v any) (anthropic.BetaToolResultBlockParamContentUnion, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return anthropic.BetaToolResultBlockParamContentUnion{}, err
	}
	return anthropic.BetaToolResultBlockParamContentUnion{
		OfText: &anthropic.BetaTextBlockParam{Text: string(b)},
	}, nil
}

func errResult(format string, args ...any) (anthropic.BetaToolResultBlockParamContentUnion, error) {
	return anthropic.BetaToolResultBlockParamContentUnion{
		OfText: &anthropic.BetaTextBlockParam{Text: "ERROR: " + fmt.Sprintf(format, args...)},
	}, nil
}

func (t *Toolbox) note(name, summary string) {
	if t.OnCall != nil {
		t.OnCall(name, summary)
	}
}

// Tools returns every tool the agent can use.
func (t *Toolbox) Tools() ([]anthropic.BetaTool, error) {
	var tools []anthropic.BetaTool
	add := func(tool anthropic.BetaTool, err error) error {
		if err != nil {
			return err
		}
		tools = append(tools, tool)
		return nil
	}

	for _, mk := range []func() (anthropic.BetaTool, error){
		t.gameweekStatus, t.searchPlayers, t.getPlayer, t.teamFixtures,
		t.optimizeSquad, t.myTeam, t.suggestTransfers, t.chipPlan, t.squadStatus,
		t.researchTargets, t.setPlayerStatus, t.setPriceForecast,
		t.competitionStatus, t.updateCompetition,
	} {
		if err := add(mk()); err != nil {
			return nil, err
		}
	}
	return tools, nil
}

// --- Compact row types keep tool output token-efficient ------------------

type playerRow struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	Team       string   `json:"team"`
	Pos        string   `json:"pos"`
	Price      float64  `json:"price"`
	Score      float64  `json:"score"` // expected pts/GW over the horizon
	Value      float64  `json:"score_per_m"`
	XGI90      float64  `json:"xgi90"`
	XGC90      float64  `json:"xgc90,omitempty"`
	Mins       int      `json:"mins"`
	ExpMins    float64  `json:"expected_minutes_per_gw"`
	Rotation   string   `json:"rotation_risk"`
	MinsRel    float64  `json:"mins_reliability"`
	Pts        int      `json:"pts_last_season"`
	Own        float64  `json:"own_pct"`
	FDR        float64  `json:"avg_fdr"`
	SetPiece   string   `json:"set_pieces,omitempty"`
	Avail      string   `json:"availability,omitempty"`
	News       string   `json:"news,omitempty"`
	Finish     float64  `json:"goals_minus_xg"`
	NewClub    string   `json:"new_signing_joined,omitempty"`
	RestRisk   string   `json:"rest_risk,omitempty"`
	Congest    float64  `json:"congestion_factor"`
	CongestWhy []string `json:"congestion_reasons,omitempty"`
	RoleFactor float64  `json:"role_certainty_factor"`
	RoleWhy    []string `json:"role_risk_reasons,omitempty"`
	// Avail is the availability discount, and it is the term that most often
	// explains a score the agent would otherwise call inexplicable: a ruled-out
	// player scores 0 because this is 0, not because his underlying numbers
	// collapsed. Carried under the standing rule that every scoring term is a
	// reported multiplier.
	//
	// Omitted at exactly 1.0 — the same treatment as Load, and for the same reason:
	// a column of 1.0s on every row of every search is replayed on every subsequent
	// API call and paid for repeatedly.
	//
	// A POINTER, not a bare float with omitempty. `omitempty` on a float64 drops
	// 0.0, and 0.0 is the one value this field exists to carry — a player FPL has
	// ruled out, whose score is zero for that reason and no other. The bare-float
	// version would have silently omitted exactly the case it was added for, which
	// is the same trap that keeps AvailabilityFactor un-omitempty on PlayerMetrics.
	AvailFactor *float64 `json:"availability_factor,omitempty"`
	// Load is how many matches the club plays per gameweek across the scoring
	// horizon: 1.0 normally, up to 2.0 for a double gameweek, below 1.0 when the
	// club blanks. It is a shipped multiplier on expected points and can move a
	// player's worth by a factor of two, so it belongs here under this project's
	// rule that every scoring term is a *reported* multiplier — the agent has to
	// be able to explain a number rather than assert it.
	//
	// Omitted at exactly 1.0, which is almost every player almost every week. A
	// field on every row of every search is replayed on every subsequent API call,
	// and a column of 1.0s is pure cost.
	//
	// The number alone is not enough, because whether it is already inside the
	// `score` beside it depends on the consumer: at the shipped horizon it is not,
	// and the transfer search's xi_gain_per_gw does include it. Every payload that
	// embeds a playerRow therefore attaches the explanation through
	// noteFixtureLoad, which fires only when a row actually carries this field.
	Load float64 `json:"fixtures_per_gameweek,omitempty"`
}

func row(m analysis.PlayerMetrics) playerRow {
	r := playerRow{
		ID: m.ID, Name: m.Name, Team: m.Team, Pos: m.Position, Price: m.Price,
		Score: round(m.Score, 2), Value: round(m.ValueScore, 3),
		XGI90: round(m.XGI90, 3), Mins: m.Minutes,
		ExpMins: round(m.ExpectedMinutes, 1), Rotation: m.RotationRisk,
		MinsRel: round(m.MinutesRating, 2),
		Pts:     m.TotalPoints, Own: m.Ownership, FDR: round(m.AvgDifficulty, 2),
		SetPiece: m.SetPieceNote, Finish: round(m.Finishing, 2),
		RestRisk: m.RestRisk,
		Congest:  round(m.Congestion, 3), CongestWhy: m.CongestionNotes,
		RoleFactor: round(m.RoleFactor, 3), RoleWhy: m.RoleNotes,
	}
	// Anything but a fully available player. Set through a pointer so a ruled-out
	// player's 0.0 survives to the agent rather than being dropped as an empty value.
	if m.AvailabilityFactor != 1 {
		a := round(m.AvailabilityFactor, 2)
		r.AvailFactor = &a
	}
	// A double or a blank gameweek in the horizon, and nothing otherwise. The
	// threshold is analysis.FixtureLoadIsNotable so the CLI and this agree.
	if analysis.FixtureLoadIsNotable(m.FixtureLoad) {
		r.Load = round(m.FixtureLoad, 2)
	}
	if m.NewSigning {
		r.NewClub = m.JoinedDate
	}
	if m.Position == "GKP" || m.Position == "DEF" {
		r.XGC90 = round(m.XGC90, 3)
	}
	if m.Status != "available" {
		r.Avail = m.Status
	}
	if m.News != "" {
		r.News = m.News
	}
	return r
}

// fixtureLoadNote explains playerRow.Load, in one place, for the payloads that
// carry it.
//
// Whether the horizon `score` already includes the multiplier is asked of the
// engine rather than asserted: at the shipped five-gameweek horizon it does not —
// the term is confined to the imminent-week eleven and the transfer objective —
// and at a configured horizon of 1 the same engine *is* the imminent-week view and
// it does. Getting that backwards would have the agent explaining a double
// gameweek twice or not at all.
func fixtureLoadNote(e *analysis.Engine) string {
	s := "fixtures_per_gameweek is how many matches the club plays per gameweek over " +
		"the scoring horizon: above 1.0 is a double gameweek, below 1.0 a blank. It " +
		"multiplies expected points when the eleven is picked for the imminent gameweek, " +
		"and inside the transfer objective. "
	if e.FixtureLoadInScore() {
		return s + "At this horizon it is already inside `score`."
	}
	return s + "It is NOT inside the `score` beside it, which is a horizon average — so " +
		"a club with a double is worth more than its score says, and one that blanks less."
}

// noteFixtureLoad attaches the explanation to a payload when any of its rows
// carries a fixture load, and adds nothing when none does.
//
// One helper rather than a line at each of the four payloads that embed playerRow.
// A field arriving without the statement of whether it is already inside `score` is
// the exact defect this change is correcting, and four independent copies of the
// condition is how three of them would come to be missing it. Load is zero when the
// club plays exactly once a gameweek, so a payload about an ordinary week is
// untouched — these results are replayed on every subsequent API call.
// It reports whether it added anything, so a payload that has more to say about the
// same field can extend the note rather than write a second one.
func noteFixtureLoad(out map[string]any, e *analysis.Engine, rows ...[]playerRow) bool {
	for _, rs := range rows {
		for _, r := range rs {
			if r.Load != 0 {
				out[fixtureLoadNoteKey] = fixtureLoadNote(e)
				return true
			}
		}
	}
	return false
}

const fixtureLoadNoteKey = "fixtures_per_gameweek_note"

func round(f float64, places int) float64 {
	p := 1.0
	for i := 0; i < places; i++ {
		p *= 10
	}
	return float64(int(f*p+0.5*sign(f))) / p
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// --- get_gameweek_status --------------------------------------------------

type gameweekStatusInput struct{}

func (t *Toolbox) gameweekStatus() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"get_gameweek_status",
		"Get the current state of the season: which gameweek is next, its deadline, "+
			"which gameweek is in progress, and which chips are available. "+
			"Call this first in any analysis so your advice is anchored to the right deadline.",
		func(ctx context.Context, _ gameweekStatusInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			t.note("get_gameweek_status", "checking season state")
			boot := t.Engine.Boot

			out := map[string]any{}
			if next := boot.NextEvent(); next != nil {
				out["next_gameweek"] = next.ID
				out["next_gameweek_name"] = next.Name
				out["deadline_utc"] = next.DeadlineTime.Format("2006-01-02T15:04:05Z")
				out["deadline_uk"] = next.DeadlineTime.Format("Mon 2 Jan 2006 15:04 MST")
			}
			if cur := boot.CurrentEvent(); cur != nil {
				out["current_gameweek"] = cur.ID
				out["current_gameweek_finished"] = cur.Finished
			}
			finished := 0
			for _, e := range boot.Events {
				if e.Finished {
					finished++
				}
			}
			out["gameweeks_completed"] = finished
			out["total_gameweeks"] = len(boot.Events)
			out["season_started"] = finished > 0
			if finished == 0 {
				out["note"] = "Season has not started. Player stats in this dataset are last season's totals, " +
					"which is the correct baseline for gameweek 1 planning. Account for transfers, " +
					"new signings (who will show 0 minutes) and promoted teams."
			}

			var chips []string
			for _, c := range boot.Chips {
				chips = append(chips, c.Name)
			}
			out["chips_in_season"] = chips
			out["total_managers"] = boot.TotalPlayers
			return textResult(out)
		},
	)
}

// --- search_players -------------------------------------------------------

type searchPlayersInput struct {
	Position           string  `json:"position,omitempty" jsonschema:"description=Filter by position: GKP DEF MID or FWD. Omit for all."`
	Team               string  `json:"team,omitempty" jsonschema:"description=Filter by club, short name (ARS MCI LIV) or full name."`
	Query              string  `json:"query,omitempty" jsonschema:"description=Fuzzy name search, e.g. 'salah' or 'gabriel'."`
	MaxPrice           float64 `json:"max_price,omitempty" jsonschema:"description=Maximum price in millions, e.g. 7.5"`
	MinPrice           float64 `json:"min_price,omitempty" jsonschema:"description=Minimum price in millions."`
	MinMinutes         int     `json:"min_minutes,omitempty" jsonschema:"description=Exclude players below this many total minutes last season."`
	MinExpectedMinutes float64 `json:"min_expected_minutes,omitempty" jsonschema:"description=Exclude players averaging fewer than this many minutes per gameweek. This is the rotation-risk filter: 75 keeps only nailed starters, 60 likely starters, 40 excludes fringe players."`
	MaxOwnership       float64 `json:"max_ownership,omitempty" jsonschema:"description=Maximum selected-by percentage, for finding differentials."`
	MinCongestion      float64 `json:"min_congestion_factor,omitempty" jsonschema:"description=Exclude players below this congestion factor. 1.0 is no European or international load; 0.9 excludes heavily congested players."`
	MaxAvgFDR          float64 `json:"max_avg_fdr,omitempty" jsonschema:"description=Maximum average fixture difficulty over the horizon (1 easiest, 5 hardest)."`
	SetPiecesOnly      bool    `json:"set_pieces_only,omitempty" jsonschema:"description=Only players on penalties corners or direct free kicks."`
	PenaltyTakersOnly  bool    `json:"penalty_takers_only,omitempty" jsonschema:"description=Only first- or second-choice penalty takers."`
	ExcludeNewSignings bool    `json:"exclude_new_signings,omitempty" jsonschema:"description=Exclude players who joined their club this summer, whose stats were earned elsewhere and whose role is unproven."`
	IncludeUnavailable bool    `json:"include_unavailable,omitempty" jsonschema:"description=Include injured suspended or unavailable players. Default false."`
	SortBy             string  `json:"sort_by,omitempty" jsonschema:"description=One of: score (default, expected pts per GW), value (score per million), xgi90, xgc90, points, form, ownership, price, fdr."`
	Limit              int     `json:"limit,omitempty" jsonschema:"description=Max results, default 15, cap 40. Rows are replayed on every later API call, so ask for the smallest set that answers the question."`
}

func (t *Toolbox) searchPlayers() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"search_players",
		"Search and rank players with filters. This is your main exploration tool. "+
			"'score' is the model's expected FPL points per gameweek over the fixture horizon, "+
			"combining per-90 underlying stats, fixture difficulty, set-piece duty, minutes "+
			"reliability and injury status. 'score_per_m' is that divided by price. "+
			"Use filters to answer questions like 'best defenders under 5.0m with good fixtures'.",
		func(ctx context.Context, in searchPlayersInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			desc := []string{}
			if in.Position != "" {
				desc = append(desc, in.Position)
			}
			if in.Query != "" {
				desc = append(desc, "'"+in.Query+"'")
			}
			if in.MaxPrice > 0 {
				desc = append(desc, fmt.Sprintf("<=£%.1fm", in.MaxPrice))
			}
			if in.Team != "" {
				desc = append(desc, in.Team)
			}
			t.note("search_players", strings.Join(desc, " "))

			all := t.Engine.AllMetrics()

			var teamFilter string
			if in.Team != "" {
				tm := t.Engine.Boot.TeamByName(in.Team)
				if tm == nil {
					return errResult("unknown team %q", in.Team)
				}
				teamFilter = tm.ShortName
			}

			var nameMatch map[int]bool
			if in.Query != "" {
				nameMatch = map[int]bool{}
				for _, e := range t.Engine.Boot.FindPlayers(in.Query) {
					nameMatch[e.ID] = true
				}
				if len(nameMatch) == 0 {
					return errResult("no player matches %q", in.Query)
				}
			}

			pos := strings.ToUpper(strings.TrimSpace(in.Position))

			var out []analysis.PlayerMetrics
			for _, m := range all {
				if pos != "" && m.Position != pos {
					continue
				}
				if teamFilter != "" && m.Team != teamFilter {
					continue
				}
				if nameMatch != nil && !nameMatch[m.ID] {
					continue
				}
				if in.MaxPrice > 0 && m.Price > in.MaxPrice+1e-9 {
					continue
				}
				if in.MinPrice > 0 && m.Price < in.MinPrice-1e-9 {
					continue
				}
				// Scaled on this player's OWN club, exactly as Optimize's pool
				// filter scales it. The two are one quantity — "what does a
				// season-total minutes floor mean right now" — and the agent
				// asking which players clear a floor must get the same answer as
				// the optimiser deciding which players it may buy, or the agent
				// recommends a footballer the optimiser cannot reach.
				if m.Minutes < t.Engine.ScaledMinMinutesFor(m.TeamID, in.MinMinutes) {
					continue
				}
				if in.MinExpectedMinutes > 0 && m.SettledMinutes < in.MinExpectedMinutes {
					continue
				}
				if in.MaxOwnership > 0 && m.Ownership > in.MaxOwnership {
					continue
				}
				if in.MaxAvgFDR > 0 && m.AvgDifficulty > in.MaxAvgFDR {
					continue
				}
				if in.MinCongestion > 0 && m.Congestion < in.MinCongestion {
					continue
				}
				if in.ExcludeNewSignings && m.NewSigning {
					continue
				}
				if in.SetPiecesOnly && m.SetPieceNote == "" {
					continue
				}
				if in.PenaltyTakersOnly && (m.PenaltyOrder == nil || *m.PenaltyOrder > 2) {
					continue
				}
				if !in.IncludeUnavailable && m.Status != "available" && m.Status != "doubtful" {
					continue
				}
				out = append(out, m)
			}

			sortMetrics(out, in.SortBy)

			limit := in.Limit
			if limit <= 0 {
				limit = 15
			}
			if limit > 40 {
				limit = 40
			}
			total := len(out)
			if len(out) > limit {
				out = out[:limit]
			}

			rows := make([]playerRow, len(out))
			for i, m := range out {
				rows[i] = row(m)
				// Reason strings repeat almost verbatim across every player in a
				// result set and are replayed on every subsequent API call. Drop
				// them from list output; get_player carries the full detail.
				rows[i].CongestWhy = nil
				rows[i].RoleWhy = nil
			}
			res := map[string]any{
				"matched":         total,
				"returned":        len(rows),
				"fixture_horizon": t.Engine.Weights.Horizon,
				"players":         rows,
			}
			noteFixtureLoad(res, t.Engine, rows)
			return textResult(res)
		},
	)
}

func sortMetrics(ms []analysis.PlayerMetrics, by string) {
	switch strings.ToLower(strings.TrimSpace(by)) {
	case "value", "score_per_m":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].ValueScore > ms[j].ValueScore })
	case "xgi90":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].XGI90 > ms[j].XGI90 })
	case "xgc90":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].XGC90 < ms[j].XGC90 })
	case "points":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].TotalPoints > ms[j].TotalPoints })
	case "form":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Form > ms[j].Form })
	case "ownership":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Ownership > ms[j].Ownership })
	case "price":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Price > ms[j].Price })
	case "fdr":
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].AvgDifficulty < ms[j].AvgDifficulty })
	default:
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Score > ms[j].Score })
	}
}

// --- get_player -----------------------------------------------------------

type getPlayerInput struct {
	PlayerID int    `json:"player_id,omitempty" jsonschema:"description=Player id from search_players. Preferred over name."`
	Name     string `json:"name,omitempty" jsonschema:"description=Player name if you don't have the id."`
	Seasons  bool   `json:"include_past_seasons,omitempty" jsonschema:"description=Include previous-season totals. Useful for judging whether a breakout is real."`
	Recent   int    `json:"recent_matches,omitempty" jsonschema:"description=Include this many recent match performances (0 = none)."`
}

func (t *Toolbox) getPlayer() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"get_player",
		"Deep dive on one player: full metric breakdown, every upcoming fixture with "+
			"difficulty, set-piece duty, injury news, optional past-season history and "+
			"recent match-by-match returns. Use this before committing to a transfer.",
		func(ctx context.Context, in getPlayerInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			var el *fpl.Element
			if in.PlayerID > 0 {
				el = t.Engine.Boot.ElementByID(in.PlayerID)
				if el == nil {
					return errResult("no player with id %d", in.PlayerID)
				}
			} else if in.Name != "" {
				matches := t.Engine.Boot.FindPlayers(in.Name)
				if len(matches) == 0 {
					return errResult("no player matches %q", in.Name)
				}
				el = matches[0]
			} else {
				return errResult("provide player_id or name")
			}

			t.note("get_player", el.WebName)
			m := t.Engine.Metrics(el)

			out := map[string]any{
				"player": m,
				"expected_points_breakdown": map[string]float64{
					"base_per_90":              round(m.BaseXP90, 3),
					"set_piece_bonus_per_90":   round(m.SetPieceXP90, 3),
					"fixture_adjusted_per_90":  round(m.FixtureAdjXP90, 3),
					"minutes_reliability":      round(m.MinutesRating, 3),
					"final_score_per_gameweek": round(m.Score, 3),
				},
				// Beside the breakdown rather than inside it: the breakdown is a chain
				// of factors that multiply out to final_score_per_gameweek, and at the
				// shipped horizon this one does not.
				//
				// The *number* is not repeated here — "player" above marshals the whole
				// PlayerMetrics, so it is already at player.fixtures_per_gameweek, and
				// two copies of one quantity in a payload replayed on every later API
				// call is cost for nothing. What this adds is the part the number cannot
				// say: whether it is inside the score, and what it means.
				"fixture_load": map[string]any{
					"applied_to_score": t.Engine.FixtureLoadInScore(),
					"note":             fixtureLoadNote(t.Engine),
				},
			}
			// All remaining fixtures, not just the horizon.
			out["all_upcoming_fixtures"] = t.Engine.TeamFixtures(el.Team, 10)

			if in.Seasons || in.Recent > 0 {
				sum, err := t.Client.ElementSummary(ctx, el.ID)
				if err != nil {
					out["history_error"] = err.Error()
				} else {
					if in.Seasons {
						out["past_seasons"] = pastSeasonsForTool(sum.HistoryPast)
					}
					if in.Recent > 0 {
						h := sum.History
						if len(h) > in.Recent {
							h = h[len(h)-in.Recent:]
						}
						out["recent_matches"] = h
					}
				}
			}
			return textResult(out)
		},
	)
}

// --- get_team_fixtures ----------------------------------------------------

type teamFixturesInput struct {
	Teams     []string `json:"teams,omitempty" jsonschema:"description=Club short or full names. Omit for all 20 clubs."`
	Gameweeks int      `json:"gameweeks,omitempty" jsonschema:"description=How many upcoming gameweeks to show. Default is the configured horizon."`
}

func (t *Toolbox) teamFixtures() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"get_team_fixtures",
		"Fixture difficulty outlook per club over the next N gameweeks, sorted easiest "+
			"run first. Use this to spot which clubs' attackers and defenders to target "+
			"or avoid, and to time transfers around fixture swings.",
		func(ctx context.Context, in teamFixturesInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			n := in.Gameweeks
			if n <= 0 {
				n = t.Engine.Weights.Horizon
			}
			if n > 15 {
				n = 15
			}
			t.note("get_team_fixtures", fmt.Sprintf("next %d GWs", n))

			var wanted map[int]bool
			if len(in.Teams) > 0 {
				wanted = map[int]bool{}
				for _, name := range in.Teams {
					tm := t.Engine.Boot.TeamByName(name)
					if tm == nil {
						return errResult("unknown team %q", name)
					}
					wanted[tm.ID] = true
				}
			}

			type teamRun struct {
				Team     string                  `json:"team"`
				AvgFDR   float64                 `json:"avg_fdr"`
				Fixtures []analysis.FixtureBrief `json:"fixtures"`
			}
			var runs []teamRun
			for i := range t.Engine.Boot.Teams {
				tm := &t.Engine.Boot.Teams[i]
				if wanted != nil && !wanted[tm.ID] {
					continue
				}
				fx := t.Engine.TeamFixtures(tm.ID, n)
				var sum int
				for _, f := range fx {
					sum += f.Difficulty
				}
				avg := 0.0
				if len(fx) > 0 {
					avg = float64(sum) / float64(len(fx))
				}
				runs = append(runs, teamRun{Team: tm.ShortName, AvgFDR: round(avg, 2), Fixtures: fx})
			}
			sort.SliceStable(runs, func(i, j int) bool { return runs[i].AvgFDR < runs[j].AvgFDR })
			return textResult(map[string]any{
				"gameweeks": n,
				"note":      "Difficulty 1 = easiest, 5 = hardest. Sorted by easiest run first.",
				"teams":     runs,
			})
		},
	)
}

// --- optimize_squad -------------------------------------------------------

type optimizeSquadInput struct {
	Budget             *float64      `json:"budget_m,omitempty" jsonschema:"description=Total budget in millions. Omit it and the tool uses the real budget — your squad's selling value plus the bank in-season, the £100.0m allowance before it. Pass it only to ask a what-if."`
	LockNames          []string      `json:"lock_players,omitempty" jsonschema:"description=Player names or ids that must be in the squad."`
	ExcludeNames       []string      `json:"exclude_players,omitempty" jsonschema:"description=Player names or ids to keep out of the squad."`
	MinMinutes         int           `json:"min_minutes,omitempty" jsonschema:"description=Exclude players below this many total minutes. Default 600. Set 0 to allow new signings with no data."`
	MinExpectedMinutes float64       `json:"min_expected_minutes,omitempty" jsonschema:"description=Rotation-risk floor for squad members, in minutes per gameweek. Cheap bench fodder at or below 4.5m is exempt. Default 55."`
	BenchWeight        float64       `json:"bench_weight,omitempty" jsonschema:"description=How much bench players count, 0-1. Omit it: the configured weight is the measured one and every other squad this project builds uses it. Pass a value only to ask a what-if, and note it overrides the measured setting for this call — low values buy cheap bench fodder and spend the money on the XI, which was tried and is not better."`
	PriceChanges       []priceChange `json:"price_changes,omitempty" jsonschema:"description=What-if prices for named players, in millions. Use it to ask what the squad looks like AFTER price changes you expect tonight: run the tool once without this and once with it, and compare. The model cannot see price movement; you can, from transfer traffic and the press."`
}

// priceChange is one projected price for one player.
type priceChange struct {
	Name  string  `json:"player" jsonschema:"description=Player name or id."`
	Price float64 `json:"price_m" jsonschema:"description=The price to assume, in millions, e.g. 8.6."`
}

func (t *Toolbox) optimizeSquad() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"optimize_squad",
		"Build the highest-scoring legal 15-man squad under FPL rules: "+
			"2 GKP / 5 DEF / 5 MID / 3 FWD, max 3 players per club, within the budget "+
			"actually available — this squad's selling value plus the bank once the "+
			"season is running, FPL's £100.0m allowance before it. Returns the squad, "+
			"the best starting XI and formation, and captain suggestion. "+
			"'starting_xi_score' is the plain sum of the eleven; 'expected_points' adds "+
			"the captain's score again, since the armband doubles one player, and is what "+
			"the team is actually expected to return. The optimiser maximises the latter, "+
			"so it prefers a squad built around one high scorer over a flat squad of the "+
			"same total. "+
			"Use lock_players and exclude_players to test scenarios. "+
			"Treat the output as a strong starting point to critique, not a final answer — "+
			"it optimises the model's score and knows nothing about news you may have.",
		func(ctx context.Context, in optimizeSquadInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			out, err := t.optimizeSquadFor(in)
			if err != nil {
				return errResult("%v", err)
			}
			return textResult(out)
		},
	)
}

// optimizeSquadFor is the tool's actual work, separated from the tool plumbing
// so it can be exercised directly by tests.
func (t *Toolbox) optimizeSquadFor(in optimizeSquadInput) (map[string]any, error) {
	// The real budget unless the caller is asking a what-if. £100m is
	// what you had in August; in-season the money that answers "the
	// best fifteen available" is this squad's selling value plus the
	// bank. An entry whose squad cannot be priced is an error rather
	// than a licence to assume — see Engine.AssemblyBudget.
	budget, budgetSource, err := t.Engine.AssemblyBudget()
	if err != nil {
		return nil, err
	}
	if in.Budget != nil {
		budget = int(*in.Budget*10 + 0.5)
		budgetSource = "supplied by the caller"
	}
	t.note("optimize_squad", fmt.Sprintf("£%.1fm budget (%s)",
		float64(budget)/10, budgetSource))

	// Standing overrides bind every call. A per-call list adds to them
	// rather than replacing them, so a scenario cannot quietly reinstate
	// a player an earlier run established was unavailable.
	stdLock, stdStart, stdExclude, rosterNotes := t.rosterSets()

	req := analysis.OptimizeRequest{
		Budget:             budget,
		MinMinutes:         in.MinMinutes,
		MinExpectedMinutes: in.MinExpectedMinutes,
		BenchWeight:        in.BenchWeight,
		LockIDs:            stdLock,
		StartIDs:           stdStart,
		ExcludeIDs:         stdExclude,
	}
	if in.MinMinutes == 0 {
		req.MinMinutes = 600
	}
	if in.MinExpectedMinutes == 0 {
		req.MinExpectedMinutes = 55
	}
	if len(in.PriceChanges) > 0 {
		req.PriceOverride = map[int]int{}
		for _, pc := range in.PriceChanges {
			if pc.Price <= 0 {
				continue
			}
			ids, err := t.resolveIDs([]string{pc.Name})
			if err != nil || len(ids) == 0 {
				continue
			}
			req.PriceOverride[ids[0]] = int(pc.Price*10 + 0.5)
		}
	}

	// Append rather than assign: a per-call scenario adds to the standing
	// overrides, it does not silently drop them.
	extraLock, err := t.resolveIDs(in.LockNames)
	if err != nil {
		return nil, err
	}
	extraExclude, err := t.resolveIDs(in.ExcludeNames)
	if err != nil {
		return nil, err
	}
	req.LockIDs = append(req.LockIDs, extraLock...)
	req.ExcludeIDs = append(req.ExcludeIDs, extraExclude...)

	sq, err := t.Engine.Optimize(req)
	if err != nil {
		return nil, err
	}

	xi, bench := compactRows(sq.StartingXI), compactRows(sq.Bench)
	out := map[string]any{
		"formation":          sq.Formation,
		"budget_m":           round(float64(budget)/10, 1),
		"budget_source":      budgetSource,
		"total_cost_m":       sq.TotalCost,
		"budget_remaining_m": sq.Remaining,
		"starting_xi_score":  round(sq.XIScore, 2),
		"expected_points":    round(sq.ExpectedPoints, 2),
		"squad_score":        round(sq.SquadScore, 2),
		"club_counts":        sq.ClubCounts,
		"starting_xi":        xi,
		"bench":              bench,
		"suggested_captain":  sq.Captain.Name,
		"suggested_vice":     sq.ViceCaptain.Name,
	}
	noteFixtureLoad(out, t.Engine, xi, bench)
	if len(rosterNotes) > 0 {
		out["standing_overrides"] = rosterNotes
	}
	return out, nil
}

func compactRows(ms []analysis.PlayerMetrics) []playerRow {
	out := make([]playerRow, len(ms))
	for i, m := range ms {
		out[i] = row(m)
	}
	return out
}

// resolveIDs turns a mixed list of names and numeric ids into player ids.
func (t *Toolbox) resolveIDs(names []string) ([]int, error) {
	var ids []int
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		var id int
		if _, err := fmt.Sscanf(n, "%d", &id); err == nil && id > 0 {
			if t.Engine.Boot.ElementByID(id) != nil {
				ids = append(ids, id)
				continue
			}
		}
		matches := t.Engine.Boot.FindPlayers(n)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no player matches %q", n)
		}
		ids = append(ids, matches[0].ID)
	}
	return ids, nil
}

// --- get_my_team ----------------------------------------------------------

type myTeamInput struct {
	EntryID  int `json:"entry_id,omitempty" jsonschema:"description=FPL manager id. Defaults to the configured one."`
	Gameweek int `json:"gameweek,omitempty" jsonschema:"description=Which gameweek's squad to fetch. Defaults to the most recent completed one."`
}

func (t *Toolbox) myTeam() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"get_my_team",
		"Fetch the user's current FPL squad, bank balance, team value, overall rank and "+
			"points. Only works once the season has started and at least one deadline has "+
			"passed. Call this before recommending transfers so advice is grounded in what "+
			"they actually own.",
		func(ctx context.Context, in myTeamInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			entryID := in.EntryID
			if entryID == 0 {
				entryID = t.Cfg.EntryID
			}
			if entryID == 0 {
				return errResult("no FPL manager id configured. Set \"entry_id\" in config.json " +
					"(find it in the URL of your points page: /entry/<ID>/event/1). " +
					"Without it you can still get squad suggestions from optimize_squad.")
			}
			t.note("get_my_team", fmt.Sprintf("entry %d", entryID))

			entry, err := t.Client.Entry(ctx, entryID)
			if err != nil {
				return errResult("fetching manager %d: %v", entryID, err)
			}

			gw := in.Gameweek
			if gw == 0 {
				if entry.CurrentEvent != nil {
					gw = *entry.CurrentEvent
				}
			}
			if gw == 0 {
				return textResult(map[string]any{
					"manager":   entry.PlayerFirstName + " " + entry.PlayerLastName,
					"team_name": entry.Name,
					"note": "No squad is visible yet — the season has not started or the first " +
						"deadline has not passed. Use optimize_squad to plan an opening squad.",
				})
			}

			picks, err := t.Client.Picks(ctx, entryID, gw)
			if err != nil {
				return errResult("fetching picks for gameweek %d: %v", gw, err)
			}

			type squadPick struct {
				playerRow
				Position    int  `json:"squad_position"`
				Captain     bool `json:"is_captain,omitempty"`
				ViceCaptain bool `json:"is_vice_captain,omitempty"`
				Starting    bool `json:"starting"`
			}
			var out []squadPick
			var rows []playerRow
			for _, p := range picks.Picks {
				el := t.Engine.Boot.ElementByID(p.Element)
				if el == nil {
					continue
				}
				r := row(t.Engine.Metrics(el))
				rows = append(rows, r)
				out = append(out, squadPick{
					playerRow:   r,
					Position:    p.Position,
					Captain:     p.IsCaptain,
					ViceCaptain: p.IsViceCaptain,
					Starting:    p.Position <= 11,
				})
			}

			res := map[string]any{
				"manager":                entry.PlayerFirstName + " " + entry.PlayerLastName,
				"team_name":              entry.Name,
				"gameweek":               gw,
				"overall_rank":           entry.SummaryOverallRank,
				"total_points":           entry.SummaryOverallPoints,
				"bank_m":                 float64(picks.EntryHistory.Bank) / 10,
				"squad_value_m":          float64(picks.EntryHistory.Value) / 10,
				"active_chip":            picks.ActiveChip,
				"transfers_made_this_gw": picks.EntryHistory.EventTransfers,
				"transfer_cost_this_gw":  picks.EntryHistory.EventTransfersCost,
				"squad":                  out,
			}
			noteFixtureLoad(res, t.Engine, rows)
			return textResult(res)
		},
	)
}

// --- suggest_transfers ----------------------------------------------------

type suggestTransfersInput struct {
	SquadIDs     []int    `json:"current_squad_ids,omitempty" jsonschema:"description=The 15 player ids you currently own. If omitted the tool fetches them from the configured manager id."`
	Bank         *float64 `json:"bank_m,omitempty" jsonschema:"description=Money in the bank in millions. Omit it and the tool uses the real balance; pass it only to ask a what-if."`
	MaxTransfers int      `json:"max_transfers,omitempty" jsonschema:"description=How many swaps to consider, default 1."`
	MinMinutes   int      `json:"min_minutes,omitempty" jsonschema:"description=Minimum minutes for replacement candidates. Default 600."`
}

func (t *Toolbox) suggestTransfers() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"suggest_transfers",
		"Given a current 15-man squad, rank transfers by what they do to the starting "+
			"eleven, respecting budget, position and the 3-per-club limit. Returns 1-for-1 "+
			"candidates plus funded pairs — a premium the bank cannot reach on its own "+
			"together with the sale that pays for him, which no single swap can find. Each "+
			"carries the gain per gameweek, netted against both the cost of a free "+
			"transfer and the cost of a -4 hit, so you can judge whether to move at all.",
		func(ctx context.Context, in suggestTransfersInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			out, err := t.suggestTransfersFor(ctx, in)
			if err != nil {
				return errResult("%v", err)
			}
			return textResult(out)
		},
	)
}

// sellsLocked reports whether any funding leg of a pair sells a locked player.
func sellsLocked(p analysis.Pair, locked map[int]bool) bool {
	for _, d := range p.Downs {
		if locked[d.Out.ID] {
			return true
		}
	}
	return false
}

// suggestTransfersFor is the tool's actual work, separated from the tool
// plumbing so it can be exercised directly by tests.
func (t *Toolbox) suggestTransfersFor(ctx context.Context, in suggestTransfersInput) (map[string]any, error) {
	squadIDs := in.SquadIDs

	// Where the money comes from, in order of authority. The balance used to be
	// read only when the tool fetched the squad itself, so a caller that passed
	// current_squad_ids and left bank_m out searched with £0.0m — and an empty
	// bank does not error, it just reports no affordable upgrade, which is
	// indistinguishable from a squad that has nothing worth buying. Whether the
	// caller supplied the fifteen must not decide whether the money exists.
	bank, bankSource := 0.0, "none known"
	if t.Engine.Bank != nil {
		bank, bankSource = float64(*t.Engine.Bank)/10, "reconstructed from your squad's price history"
	}

	if len(squadIDs) == 0 {
		if t.Cfg.EntryID == 0 {
			return nil, fmt.Errorf("provide current_squad_ids, or set \"entry_id\" in config.json")
		}
		entry, err := t.Client.Entry(ctx, t.Cfg.EntryID)
		if err != nil {
			return nil, fmt.Errorf("fetching manager: %v", err)
		}
		if entry.CurrentEvent == nil {
			return nil, fmt.Errorf("no squad available yet — the season has not started")
		}
		picks, err := t.Client.Picks(ctx, t.Cfg.EntryID, *entry.CurrentEvent)
		if err != nil {
			return nil, fmt.Errorf("fetching picks: %v", err)
		}
		for _, p := range picks.Picks {
			squadIDs = append(squadIDs, p.Element)
		}
		// FPL's own figure for the gameweek just fetched, so it outranks the
		// reconstruction.
		bank, bankSource = float64(picks.EntryHistory.Bank)/10, "FPL"
	}
	// An explicit value wins over both, including an explicit zero — that is a
	// scenario question ("what could I do with nothing in the bank?"), and the
	// pointer is what makes it distinguishable from not asking at all.
	if in.Bank != nil {
		bank, bankSource = *in.Bank, "supplied by the caller"
	}
	if len(squadIDs) != 15 {
		return nil, fmt.Errorf("expected 15 players in the squad, got %d", len(squadIDs))
	}
	t.note("suggest_transfers", fmt.Sprintf("£%.1fm in bank (%s)", bank, bankSource))

	var squad []analysis.PlayerMetrics
	for _, id := range squadIDs {
		el := t.Engine.Boot.ElementByID(id)
		if el == nil {
			return nil, fmt.Errorf("unknown player id %d", id)
		}
		squad = append(squad, t.Engine.Metrics(el))
	}
	state := analysis.NewSquadState(squad)
	// FPL pays you what you paid plus half of any rise, never the market price.
	// Nil when no session is configured, which means sell-at-market.
	state.Sell = t.Engine.SellPrices

	minMins := in.MinMinutes
	if minMins == 0 {
		minMins = 600
	}

	// Standing overrides bind here too. Excluding a player from squad
	// builds while the transfer search still offers to buy him is worse
	// than not excluding him at all — the squad avoids him and the weekly
	// review walks straight back into him.
	stdLock, stdStart, stdExclude, rosterNotes := t.rosterSets()
	stdLock = append(stdLock, stdStart...)
	excluded := map[int]bool{}
	for _, id := range stdExclude {
		excluded[id] = true
	}
	locked := map[int]bool{}
	for _, id := range stdLock {
		locked[id] = true
	}

	// Candidates are filtered before the search, not inside it: a player
	// who is injured or has barely played is not a transfer target
	// whatever the model scores him at.
	// ⚠️ minMins is a SEASON TOTAL and is scaled per club before comparison, exactly as
	// this file already does at searchPlayers above and as Optimize's own pool filter
	// does. That comment says why in one line — "the two are one quantity" — and this
	// loop was the copy that did not obey it.
	//
	// Unscaled, on fresh-season aggregates, 0 of 609 players cleared 600 and the most
	// anyone had played was 90: the pool came back empty and suggest_transfers answered
	// "nothing would improve this squad" for every caller, for about seven gameweeks.
	// The same defect was found and fixed in cmd/armband/transfers.go's own pool; this
	// was the third live copy, and it survived that fix because the guard on it reads
	// transfers.go and nothing else.
	scaledFloor := map[int]int{}
	floorFor := func(teamID int) int {
		if v, ok := scaledFloor[teamID]; ok {
			return v
		}
		v := t.Engine.ScaledMinMinutesFor(teamID, minMins)
		scaledFloor[teamID] = v
		return v
	}
	var pool []analysis.PlayerMetrics
	for _, c := range t.Engine.AllMetrics() {
		if excluded[c.ID] {
			continue
		}
		if c.Minutes < floorFor(c.TeamID) {
			continue
		}
		if c.Status != "available" && c.Status != "doubtful" {
			continue
		}
		pool = append(pool, c)
	}

	bankTenths := int(bank*10 + 0.5)
	horizon := float64(t.Engine.Weights.Horizon)
	// A free transfer is not a costless transfer. Replaying three
	// seasons, judging free moves on a bare per-gameweek threshold
	// churned — twelve round-trips, players sold and bought back weeks
	// later — and charging for the move removed them. See
	// config.Review.FreeTransferValue for why the charge is not 4.
	//
	// Tapered when `option_value.taper_free_transfer_value` is on: the charge
	// then falls with the season's remaining life and rises into a congested run.
	// Through config.OptionValuePolicy.FreeTransferCharge, which delegates to the
	// same analysis.TransferHoldFactorFor the replay's `decide` calls — so this
	// tool, the live banking rule and the replayed policy cannot disagree about
	// what a transfer costs. A no-op returning exactly 1 when the lever is off.
	chargeGW := 1
	if ev := t.Engine.Boot.NextEvent(); ev != nil {
		chargeGW = ev.ID
	}
	// The scheduled early floor applies to the BASE, before the taper's curve —
	// schedule first, curve second, exactly as the replay's `decide` composes
	// them. See config.ReviewPolicy.EffectiveFloor.
	baseCharge, _ := t.Cfg.Review.EffectiveFloor(chargeGW)
	freeCost := t.Cfg.OptionValue.FreeTransferCharge(
		baseCharge, t.Engine, squadIDs, chargeGW)

	type swap struct {
		Out          playerRow `json:"out"`
		In           playerRow `json:"in"`
		CostDelta    float64   `json:"cost_change_m"`
		ScoreGain    float64   `json:"xi_gain_per_gw"`
		GainOverFree float64   `json:"net_gain_if_transfer_is_free"`
		GainOverHit  float64   `json:"net_gain_if_it_costs_a_4pt_hit"`
		WorthAFree   bool      `json:"worth_spending_a_free_transfer"`
		// Timing, present only when a forecast was recorded for one of the two
		// players. Omitted otherwise, since this is replayed on every call.
		Timing string `json:"price_timing,omitempty"`
	}
	// A move is timing-sensitive if the man arriving is about to get more
	// expensive or the man leaving is about to be worth less. Both argue for
	// acting tonight rather than at the deadline; neither argues for making a
	// move that is not otherwise worth it.
	timingFor := func(out, in analysis.PlayerMetrics) string {
		var notes []string
		if f, ok := t.Engine.PriceForecast(in.ID); ok && f.Direction == "rise" {
			notes = append(notes, in.Name+" "+f.Note())
		}
		if f, ok := t.Engine.PriceForecast(out.ID); ok && f.Direction == "fall" {
			notes = append(notes, out.Name+" "+f.Note())
		}
		return strings.Join(notes, "; ")
	}
	var swaps []swap
	// Every row on either side of a move, so the fixture-load note can be attached
	// exactly when one of them has a double or a blank in the horizon.
	var moved []playerRow
	for _, sw := range analysis.RankSwaps(state, pool, bankTenths) {
		if locked[sw.Out.ID] {
			continue // a locked player is not for sale
		}
		total := sw.Gain * horizon
		out, in := row(sw.Out), row(sw.In)
		moved = append(moved, out, in)
		swaps = append(swaps, swap{
			Out: out, In: in,
			CostDelta:    round(sw.In.Price-sw.Out.Price, 1),
			ScoreGain:    round(sw.Gain, 2),
			GainOverFree: round(total-freeCost, 2),
			GainOverHit:  round(total-4, 2),
			WorthAFree:   total >= freeCost,
			Timing:       timingFor(sw.Out, sw.In),
		})
	}
	limit := 15
	if in.MaxTransfers > 0 && in.MaxTransfers < limit {
		limit = in.MaxTransfers * 8
	}
	if len(swaps) > limit {
		swaps = swaps[:limit]
	}

	// Funded moves: a premium the bank cannot reach, plus the sales that
	// pay for him. These are invisible to any one-for-one search, because
	// each funding sale lowers the eleven on its own and is rejected before
	// the upgrade is ever considered.
	type fundingLeg struct {
		Out playerRow `json:"sell"`
		In  playerRow `json:"buy"`
	}
	type pair struct {
		BuyOut    playerRow    `json:"upgrade_out"`
		BuyIn     playerRow    `json:"upgrade_in"`
		FundedBy  []fundingLeg `json:"funded_by"`
		Transfers int          `json:"transfers_required"`
		ScoreGain float64      `json:"xi_gain_per_gw"`
		// Every leg is priced. All-banked is the cheapest case; one short
		// of that means the last leg costs a -4.
		GainAllBanked float64 `json:"net_gain_if_all_transfers_banked"`
		GainOneHit    float64 `json:"net_gain_if_one_short_and_taking_a_hit"`
		WorthIt       bool    `json:"worth_it_with_the_transfers_banked"`
	}
	var pairs []pair
	// A funded move spends one transfer on the premium and the rest on the
	// sales that pay for him, so the funding legs are capped by what is
	// banked. Free transfers are not published by the API, so assume the
	// full allowance and let the agent discount from there.
	maxDowns := t.Cfg.Review.BankUpTo - 1
	if maxDowns < 1 {
		maxDowns = 1
	}
	for _, pr := range analysis.RankPairs(state, pool, bankTenths, maxDowns, 5) {
		if locked[pr.Up.Out.ID] || sellsLocked(pr, locked) {
			continue
		}
		total := pr.Gain * horizon
		n := float64(pr.Moves())
		legs := make([]fundingLeg, 0, len(pr.Downs))
		for _, d := range pr.Downs {
			dOut, dIn := row(d.Out), row(d.In)
			moved = append(moved, dOut, dIn)
			legs = append(legs, fundingLeg{Out: dOut, In: dIn})
		}
		upOut, upIn := row(pr.Up.Out), row(pr.Up.In)
		moved = append(moved, upOut, upIn)
		pairs = append(pairs, pair{
			BuyOut: upOut, BuyIn: upIn,
			FundedBy:      legs,
			Transfers:     pr.Moves(),
			ScoreGain:     round(pr.Gain, 2),
			GainAllBanked: round(total-n*freeCost, 2),
			GainOneHit:    round(total-(n-1)*freeCost-4, 2),
			WorthIt:       total >= n*freeCost,
		})
	}

	out := map[string]any{
		"bank_m":                  bank,
		"fixture_horizon":         t.Engine.Weights.Horizon,
		"free_transfer_value":     freeCost,
		"standing_overrides":      rosterNotes,
		"budget_prices":           t.Engine.Budget.Label(),
		"price_forecasts_checked": t.Engine.PriceForecastCount(),
		"note": "xi_gain_per_gw is what the move does to the STARTING ELEVEN per " +
			"gameweek, not to the player — upgrading a bench player who still will not " +
			"start scores zero, which is what it is worth. " +
			"A free transfer is NOT free: it is charged free_transfer_value, because " +
			"replaying three seasons without that charge produced twelve round-trips, " +
			"players sold and bought back weeks later for nothing. Prefer moves where " +
			"worth_spending_a_free_transfer is true, and treat a marginal one as a " +
			"reason to roll the transfer instead. The charge is deliberately below 4 — " +
			"charging a full hit's worth measurably refused good transfers too. " +
			"funded_pairs are premiums the bank cannot reach on their own, each with " +
			"the sale that pays for it; they cost two transfers and are invisible to " +
			"the one-for-one list, so compare them against it before dismissing the cost.",
		"candidates":   swaps,
		"funded_pairs": pairs,
	}
	// The transfer objective is the one consumer that *does* apply fixture load, so
	// both halves have to be said at once and in the right order: each row's `score`
	// excludes a double gameweek and xi_gain_per_gw includes it. An agent told only
	// the first would add the double back by hand and count it twice.
	if noteFixtureLoad(out, t.Engine, moved) {
		out[fixtureLoadNoteKey] = out[fixtureLoadNoteKey].(string) +
			" xi_gain_per_gw here DOES include it — this search is that consumer — so a " +
			"double gameweek can justify a move the raw scores do not, and it is already " +
			"counted. Do not add it again."
	}
	// Only when the money could not be established. A bank that is really zero
	// and a bank nobody looked up produce identical output — no affordable
	// upgrades — so the one case that needs saying is the one where the number
	// is an assumption. Said always, it would be replayed on every later call.
	if bankSource == "none known" {
		out["bank_warning"] = "No bank balance was available, so this search ran " +
			"with £0.0m and considered only moves that fund themselves. If you know " +
			"the real balance, call again with bank_m."
	}
	// Only when it matters: tool output is replayed on every subsequent API
	// call, so an always-present reassurance would be paid for repeatedly.
	if !t.Engine.Budget.Verified {
		out["budget_warning"] = "Selling prices are unverified, so affordability here " +
			"is a guess: FPL pays what you paid plus half of any rise, never the market " +
			"price, so some of these may be unaffordable. Say so when recommending one."
	}
	// Complete plans, so the agent is not left assembling moves into squads and
	// guessing who would then start. The eleven shown is the one that would
	// actually be fielded this week, not the horizon-averaged one the decision
	// is made on — see analysis.Plan.
	type planRow struct {
		Moves     []string `json:"moves"`
		Transfers int      `json:"transfers"`
		GainPerGW float64  `json:"xi_gain_per_gw"`
		CostM     float64  `json:"net_cost_m"`
		Formation string   `json:"formation_this_week"`
		XI        []string `json:"xi_this_week"`
		Bench     []string `json:"bench_in_sub_order"`
		Captain   string   `json:"captain"`
		DependsOn string   `json:"hinges_on"`
		GainIfOut float64  `json:"xi_gain_if_he_does_not_play"`
		Survives  bool     `json:"still_worth_it_without_him"`
	}
	var plans []planRow
	for _, p := range analysis.BuildPlans(state, pool, t.Engine.WeekEngine(),
		bankTenths, maxDowns+1, 5) {
		row := planRow{
			Transfers: p.Transfers,
			GainPerGW: round(p.GainPerGW, 2),
			CostM:     round(float64(p.Spend)/10, 1),
			Formation: p.Formation,
			Captain:   p.Captain.Name,
			DependsOn: p.DependsOn.Name,
			GainIfOut: round(p.GainIfOut, 2),
			Survives:  p.SurvivesLoss,
		}
		for _, m := range p.Moves {
			row.Moves = append(row.Moves, m.Out.Name+" -> "+m.In.Name)
		}
		for _, x := range p.XI {
			row.XI = append(row.XI, x.Name)
		}
		for _, b := range p.Bench {
			row.Bench = append(row.Bench, b.Name)
		}
		plans = append(plans, row)
	}
	out["plans"] = plans
	out["plans_note"] = "Complete options: the moves, and the eleven you would " +
		"actually field this week. xi_gain_per_gw is measured on the scoring " +
		"horizon, which is the number to decide on; the eleven is picked on this " +
		"week's fixture, which is what you would really put out. They can disagree, " +
		"and that is expected."
	out["timing_note"] = "hinges_on is the player whose absence would cost the plan " +
		"most, computed by zeroing him in BOTH the new squad and the current one — " +
		"so it names the player you would be wrong about, not merely your best. " +
		"Use it to decide WHEN to act: if still_worth_it_without_him is false and " +
		"his status is unresolved, waiting for team news is worth more than the " +
		"price rise you would avoid by moving tonight. A well-timed move saves " +
		"roughly 0.1m; a starter ruled out costs far more than that. If his status " +
		"IS settled, there is nothing left to learn and the price forecast decides."

	if t.Engine.PriceForecastCount() == 0 {
		out["price_timing"] = "No price forecasts recorded. If you are close to making " +
			"one of these moves, search a price-change estimator and call " +
			"set_price_forecast: acting before a rise saves the rise, and this is the " +
			"only thing that makes moving tonight better than moving at the deadline."
	}
	return out, nil
}

// --- get_chip_plan --------------------------------------------------------

type chipPlanInput struct{}

func (t *Toolbox) chipPlan() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"get_chip_plan",
		"Get the chip strategy: each chip's legal gameweek window, the manager's "+
			"current plan, any rule violations, and how the plan changes squad "+
			"construction. Also reports blank and double gameweeks. Call this at the "+
			"start of every weekly review — chips expire, and the plan determines "+
			"whether a transfer is even worth making.",
		func(ctx context.Context, _ chipPlanInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			t.note("get_chip_plan", "chip windows and plan")
			e := t.Engine
			plan := t.Cfg.Chips

			horizon, horizonWhy := e.EffectiveHorizon(plan)
			_, benchWhy := e.SuggestBenchWeight(plan)

			counts := map[int]int{}
			for _, f := range e.Fixtures {
				if f.Event != nil {
					counts[*f.Event]++
				}
			}
			var irregular []map[string]any
			for gw, n := range counts {
				if n != 10 {
					kind := "blank"
					if n > 10 {
						kind = "double"
					}
					irregular = append(irregular, map[string]any{"gameweek": gw, "type": kind, "fixtures": n})
				}
			}
			sort.Slice(irregular, func(i, j int) bool {
				return irregular[i]["gameweek"].(int) < irregular[j]["gameweek"].(int)
			})

			out := map[string]any{
				"windows": e.ChipWindows(),
				// Keyed by slot, so the second set is addressable rather than
				// collapsed onto the first: "wc1", "bb2" and the rest. Only
				// planned slots appear, which keeps the replayed tool output
				// short — this JSON is re-sent on every subsequent API call.
				"plan":   plan.All(),
				"issues": e.ValidateChipPlan(plan),
				// Separate from "issues" on purpose: these are legal, they are
				// simply unspent. Collapsing them would tell the agent a plan is
				// broken when it is merely incomplete, and the two want
				// different advice.
				"unspent_chips":             e.UnplannedChips(plan),
				"effective_squad_horizon":   horizon,
				"blank_or_double_gameweeks": irregular,
				// The legend is not decoration. This payload carries three
				// vocabularies — slot keys in "plan", FPL's own names in
				// "windows", display labels in "issues" — and without it the
				// model has to infer that "bb1" is the same chip as "bboost".
				// One sentence, paid for once per call, against a wrong
				// inference about which chip is planned.
				"note": "Only one chip may be played per gameweek, and unused chips expire at " +
					"the end of their window. Plan keys are slots: wc/fh/bb/tc for " +
					"wildcard/free hit/bench boost/triple captain, suffixed 1 or 2 for which " +
					"of the two sets a season grants — so bb2 is the second-set bench boost, " +
					"and it is the chip \"windows\" calls bboost. Only planned slots appear.",
			}
			if horizonWhy != "" {
				out["horizon_reason"] = horizonWhy
			}
			if benchWhy != "" {
				out["bench_weight_reason"] = benchWhy
			}
			if len(irregular) == 0 {
				out["blank_double_note"] = "None scheduled yet. Blanks and doubles emerge later " +
					"from cup postponements, so a Free Hit held for a blank is a bet one appears " +
					"before the chip expires."
			}
			return textResult(out)
		},
	)
}

// --- get_squad_status -----------------------------------------------------

type squadStatusInput struct {
	PlayerIDs []int `json:"player_ids,omitempty" jsonschema:"description=Players to check. Omit to use the manager's current squad."`
}

func (t *Toolbox) squadStatus() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"get_squad_status",
		"Availability and transfer-budget check: flags every injured, doubtful or "+
			"suspended player in the squad with FPL's official news text and reported "+
			"chance of playing, and reports free transfers and money in the bank. "+
			"FPL's news field is terse and often lags press conferences — pair this "+
			"with a web search for current team news before deciding.",
		func(ctx context.Context, in squadStatusInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			t.note("get_squad_status", "availability and budget")
			out := map[string]any{}

			ids := in.PlayerIDs
			if len(ids) == 0 {
				if t.Cfg.EntryID == 0 {
					out["squad_note"] = "No entry_id configured and no player_ids given, so " +
						"there is no squad to check. Ask the manager for their squad, or use " +
						"optimize_squad to plan from scratch."
				} else {
					entry, err := t.Client.Entry(ctx, t.Cfg.EntryID)
					if err != nil {
						return errResult("fetching manager: %v", err)
					}
					if entry.CurrentEvent == nil {
						out["squad_note"] = "Season has not started; no squad is visible yet."
					} else {
						picks, err := t.Client.Picks(ctx, t.Cfg.EntryID, *entry.CurrentEvent)
						if err != nil {
							return errResult("fetching picks: %v", err)
						}
						for _, p := range picks.Picks {
							ids = append(ids, p.Element)
						}
						out["bank_m"] = float64(picks.EntryHistory.Bank) / 10
						out["squad_value_m"] = float64(picks.EntryHistory.Value) / 10
					}
					if h, err := t.Client.History(ctx, t.Cfg.EntryID); err == nil {
						if ft := fpl.FreeTransfers(h); ft == fpl.UnlimitedTransfers {
							out["free_transfers"] = "unlimited"
							out["free_transfers_note"] = "The first deadline has not passed, so the " +
								"initial squad can still be changed freely. Transfer economy does not " +
								"apply yet — build the best squad, do not conserve moves."
						} else {
							out["free_transfers"] = ft
							out["free_transfers_note"] = "Reconstructed from transfer history — FPL " +
								"does not publish this directly. Verify against the site before acting."
						}
						var played []map[string]any
						for _, c := range h.Chips {
							played = append(played, map[string]any{"chip": c.Name, "gameweek": c.Event})
						}
						out["chips_already_played"] = played
					}
				}
			}

			var concerns, fine []playerRow
			for _, id := range ids {
				el := t.Engine.Boot.ElementByID(id)
				if el == nil {
					continue
				}
				m := t.Engine.Metrics(el)
				r := row(m)
				switch {
				case m.Status != "available", m.News != "",
					m.ChancePlay != nil && *m.ChancePlay < 100:
					concerns = append(concerns, r)
				default:
					fine = append(fine, r)
				}
			}
			out["flagged"] = concerns
			out["clear"] = len(fine)
			if len(concerns) == 0 && len(ids) > 0 {
				out["flagged_note"] = "No availability flags on any squad player."
			}
			// Only the flagged rows are returned, so only they can carry the field.
			noteFixtureLoad(out, t.Engine, concerns)
			return textResult(out)
		},
	)
}

// --- get_competition_status / update_competition_status -------------------

type competitionStatusInput struct{}

func (t *Toolbox) competitionStatus() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"get_competition_status",
		"Which clubs are committed to European and domestic cup competitions, over "+
			"what dates, and how long ago that was last verified. Call this FIRST in a "+
			"weekly review, before scoring anyone: a club knocked out of Europe should "+
			"no longer carry a rotation penalty, and one that progresses should. "+
			"Competition participation is not in the FPL API, so this reflects what was "+
			"last configured — check it against current results with a web search and "+
			"correct it with update_competition_status.",
		func(ctx context.Context, _ competitionStatusInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			t.note("get_competition_status", "who is still in what")
			e := t.Engine
			now := time.Now()

			type clubStatus struct {
				Club     string                       `json:"club"`
				Active   []analysis.CompetitionWindow `json:"active_now,omitempty"`
				Upcoming []analysis.CompetitionWindow `json:"upcoming,omitempty"`
				Finished []analysis.CompetitionWindow `json:"finished,omitempty"`
			}

			var out []clubStatus
			for i := range e.Boot.Teams {
				club := e.Boot.Teams[i].ShortName
				windows := append(append([]analysis.CompetitionWindow{}, e.Cong.European[club]...),
					e.Cong.DomesticCups[club]...)
				if len(windows) == 0 {
					continue
				}
				cs := clubStatus{Club: club}
				for _, w := range windows {
					switch {
					case w.Active(now):
						cs.Active = append(cs.Active, w)
					case w.End != "":
						cs.Finished = append(cs.Finished, w)
					default:
						cs.Upcoming = append(cs.Upcoming, w)
					}
				}
				out = append(out, cs)
			}

			res := map[string]any{
				"today": now.Format("2006-01-02"),
				"clubs": out,
				"note": "An empty end_date means the club is assumed still involved. Set one " +
					"via update_competition_status when a club is eliminated, so its players " +
					"stop being penalised for midweek football they will not play.",
			}
			if days, ok := e.StatusAge(); ok {
				res["last_verified_days_ago"] = days
				if days > 7 {
					res["staleness_warning"] = "Status is more than a week old. Verify against " +
						"current results before relying on it."
				}
			} else {
				res["last_verified"] = "never — treat as unverified and check the web"
			}
			return textResult(res)
		},
	)
}

type updateCompetitionInput struct {
	Club        string   `json:"club" jsonschema:"required,description=Club short name, e.g. ARS."`
	Competition string   `json:"competition" jsonschema:"required,description=UCL, UEL, UECL, or a cup name such as 'League Cup'."`
	StartDate   string   `json:"start_date,omitempty" jsonschema:"description=YYYY-MM-DD. Omit to leave an existing window's start unchanged."`
	EndDate     string   `json:"end_date,omitempty" jsonschema:"description=YYYY-MM-DD the club's involvement ended — elimination or the final. This is the main field to set in-season."`
	MatchDates  []string `json:"match_dates,omitempty" jsonschema:"description=Known fixture dates. When set, only gameweeks near these carry a penalty, which is much more accurate than assuming every week has a midweek match."`
	Note        string   `json:"note,omitempty" jsonschema:"description=Why — 'lost play-off', 'eliminated in round of 16', plus the source."`
	Remove      bool     `json:"remove,omitempty" jsonschema:"description=Delete this competition for this club entirely, for a club that never entered."`
}

func (t *Toolbox) updateCompetition() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"update_competition_status",
		"Correct a club's European or cup involvement, then re-score every player "+
			"with the new information. Use it after verifying current results on the "+
			"web: set an end_date when a club is eliminated, add a window when one "+
			"progresses, or add match_dates once a draw is made. Changes apply "+
			"immediately to every subsequent tool call and are saved to config.",
		func(ctx context.Context, in updateCompetitionInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			club := strings.ToUpper(strings.TrimSpace(in.Club))
			if t.Engine.Boot.TeamByName(club) == nil {
				return errResult("unknown club %q", in.Club)
			}
			comp := strings.TrimSpace(in.Competition)
			if comp == "" {
				return errResult("competition is required")
			}
			t.note("update_competition_status", club+" "+comp)

			european := !strings.Contains(strings.ToLower(comp), "cup")
			existing := t.Engine.Cong.DomesticCups
			if european {
				existing = t.Engine.Cong.European
			}

			// Copy before editing: the slice backing this club's windows may be
			// read by a scoring tool running concurrently in the same turn.
			windows := append([]analysis.CompetitionWindow{}, existing[club]...)
			idx := -1
			for i, w := range windows {
				if strings.EqualFold(w.Competition, comp) &&
					(in.StartDate == "" || w.Start == in.StartDate) {
					idx = i
					break
				}
			}

			switch {
			case in.Remove:
				if idx < 0 {
					return errResult("%s has no %s window to remove", club, comp)
				}
				windows = append(windows[:idx], windows[idx+1:]...)
			case idx >= 0:
				if in.StartDate != "" {
					windows[idx].Start = in.StartDate
				}
				if in.EndDate != "" {
					windows[idx].End = in.EndDate
				}
				if len(in.MatchDates) > 0 {
					windows[idx].MatchDates = in.MatchDates
				}
				if in.Note != "" {
					windows[idx].Note = in.Note
				}
			default:
				windows = append(windows, analysis.CompetitionWindow{
					Competition: comp, Start: in.StartDate, End: in.EndDate,
					MatchDates: in.MatchDates, Note: in.Note,
				})
			}
			// Goes through the engine so the swap and the congestion rebuild happen
			// under its write lock — other tools may be scoring off this right now.
			updated := t.Engine.SetCompetitionWindows(club, european, windows,
				time.Now().Format("2006-01-02"))

			// Persist so the correction survives to next week's review.
			saved := "saved to " + t.ConfigPath
			if t.ConfigPath == "" {
				saved = "not saved (no config path)"
			} else if err := t.updateConfig(func(cfg *config.Config) error {
				cfg.Congestion = updated
				return nil
			}); err != nil {
				saved = "could not save: " + err.Error()
			}

			return textResult(map[string]any{
				"club":        club,
				"competition": comp,
				"windows_now": windows,
				"persistence": saved,
				"effect": "All subsequent scoring reflects this. Re-run searches or " +
					"optimize_squad if you scored players before making this change.",
			})
		},
	)
}

// --- roster overrides -----------------------------------------------------

// rosterIDs resolves the standing overrides to this season's element ids.
//
// Overrides are stored by permanent player code because element ids are
// reassigned every summer; resolving late means an override set in August still
// points at the same footballer in May.
func (t *Toolbox) rosterIDs() (lock, exclude []int, notes []string) {
	lock, _, exclude, notes = t.rosterSets()
	return lock, exclude, notes
}

// rosterSets separates locks that only require squad membership from those that
// require a starting place.
func (t *Toolbox) rosterSets() (lock, start, exclude []int, notes []string) {
	gw := 1
	if ev := t.Engine.Boot.NextEvent(); ev != nil {
		gw = ev.ID
	}

	byCode := map[int]int{}
	for i := range t.Engine.Boot.Elements {
		el := &t.Engine.Boot.Elements[i]
		byCode[el.Code] = el.ID
	}
	// Guarded: t.Cfg is reassigned wholesale by updateConfig, under t.mu, and
	// the tool runner fans a turn's calls out through an errgroup — so another
	// tool call's config write can land in the middle of this read. This is the
	// same torn-read shape minutesOverrideFor exists to prevent on the engine
	// side, just on the config struct rather than the override maps.
	// TestConcurrentSetPlayerStatusMinutesCallsDoNotRaceTheConfirmedReadback
	// found it live, under -race, through this exact call.
	t.mu.Lock()
	lockOs, excludeOs, expired := t.Cfg.Roster.Active(gw)
	t.mu.Unlock()
	for _, o := range lockOs {
		id, ok := byCode[o.Code]
		if !ok {
			continue
		}
		if o.MustStart {
			start = append(start, id)
			notes = append(notes, "must start: "+o.String())
			continue
		}
		lock = append(lock, id)
		notes = append(notes, "locked into the squad: "+o.String())
	}
	for _, o := range excludeOs {
		if id, ok := byCode[o.Code]; ok {
			exclude = append(exclude, id)
			notes = append(notes, "excluded: "+o.String())
		}
	}
	for _, o := range expired {
		notes = append(notes, "lapsed, no longer applied: "+o.String())
	}
	return lock, start, exclude, notes
}

type setPlayerStatusInput struct {
	Player  string   `json:"player" jsonschema:"description=Player name or id."`
	Minutes *float64 `json:"expected_minutes,omitempty" jsonschema:"description=With mode 'minutes': what he actually plays per gameweek. 90 for a nailed starter the data understates, 0 for someone out."`
	Mode    string   `json:"mode" jsonschema:"description=PREFER 'minutes'. One of: minutes (correct the expected-minutes figure and let the model re-decide - the right tool when the number is wrong, e.g. a returning injury or a promoted-club starter), start (must be in the STARTING ELEVEN - use when the squad is built around him), lock (must be in the squad but may be benched - use for a cheap enabler you need available), exclude (never picked or bought), confirm (re-verified against the news, still applies - optionally with an updated reason or until_gameweek), clear (remove any override)."`
	Reason  string   `json:"reason" jsonschema:"description=Why. Shown back on every future run so a reader can tell when it no longer applies."`
	Until   int      `json:"until_gameweek,omitempty" jsonschema:"description=Gameweek this lapses after. Omit for indefinite, which is reported as needing review every run."`
	// Confirmed is a pointer for the same reason Minutes is: OMITTING it must
	// mean something different from explicitly passing false. A plain bool
	// defaults to false when left out of a call, and mode 'minutes' is the
	// path the tool description tells the agent to PREFER for any correction —
	// including a routine re-estimate of a player already confirmed nailed. If
	// omitting this field read as false, that ordinary follow-up call would
	// silently un-confirm him the moment the caller forgot to restate
	// confirmed:true, which defeats the reason this field exists. Nil now
	// means "leave whatever is already on file," and Roster.Set resolves it.
	Confirmed *bool `json:"confirmed,omitempty" jsonschema:"description=With mode 'minutes' ONLY: pass true when you are asserting this as SETTLED FACT rather than a hedge - e.g. a confirmed starting role, a nailed-on new signing, an announced long-term injury with no return in doubt. Pass false for anything you would describe as provisional, a prediction, a first start, or a 'rather than a nailed X' judgement call - false reads as an honest 'not yet established' and is the safer default. OMIT this field entirely for a routine expected_minutes correction that says nothing new about confidence - omitting PRESERVES whatever confirmed state the player already has on file, rather than resetting it to false. This is the ONLY thing that lets the rotation_risk label read 'nailed'; the expected_minutes value alone no longer decides it, because a high number and genuine confidence are different claims."`
}

// savedTo names the file an override of this mode will actually land in.
//
// A LOCK lives in the team file and everything else in the config, so a single
// "saved to <config path>" would be false for two of the six modes — and this
// string is what the model reports back to the reader, so a wrong one is a
// wrong answer rather than a cosmetic slip.
//
// Its own method rather than a block inside the handler: that handler is
// already over the complexity threshold and the ratchet only turns down.
func (t *Toolbox) savedTo(mode string) string {
	if t.ConfigPath == "" {
		return "not saved (no config path)"
	}
	if mode != "lock" && mode != "start" {
		return "saved to " + t.ConfigPath
	}
	if t.TeamPath == "" {
		return "NOT saved: a lock lives in the team file and no -team path is set"
	}
	return "saved to " + t.TeamPath
}

func (t *Toolbox) setPlayerStatus() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"set_player_status",
		"Lock a player into every squad, or exclude him from every squad and transfer. "+
			"Reach for mode 'minutes' first. It supplies a fact the model lacks — what a "+
			"player actually plays — and lets the optimiser re-decide whether he is worth "+
			"his price, which it may well decline. lock and start override that judgement "+
			"instead of informing it, so use them only when the problem is not the minutes. "+
			"Also the way to record that you re-checked one: mode 'confirm' refreshes the "+
			"verification date and, if the situation has moved, the reason and expiry. "+
			"Use this when you establish something the model cannot see: a player ruled "+
			"out for weeks, one who has lost his place, or one the squad must be built "+
			"around. Unlike the lock_players and exclude_players arguments to "+
			"optimize_squad, which apply to a single call, this persists to config and "+
			"binds every later run and every tool — including suggest_transfers, so an "+
			"excluded player is never offered as a buy. Always give a reason and, where "+
			"you can, a gameweek it lapses after.",
		func(ctx context.Context, in setPlayerStatusInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			t.note("set_player_status", fmt.Sprintf("%s %s", in.Mode, in.Player))
			mode := strings.ToLower(strings.TrimSpace(in.Mode))
			switch mode {
			case "minutes", "start", "lock", "exclude", "confirm", "clear":
			default:
				return errResult("mode must be minutes, start, lock, exclude, confirm or clear, got %q", in.Mode)
			}
			if mode != "clear" && mode != "confirm" && strings.TrimSpace(in.Reason) == "" {
				return errResult("a reason is required: it is what lets a later run " +
					"decide whether the override still applies")
			}
			ids, err := t.resolveIDs([]string{in.Player})
			if err != nil || len(ids) == 0 {
				return errResult("no player matching %q", in.Player)
			}
			el := t.Engine.Boot.ElementByID(ids[0])
			if el == nil || el.Code == 0 {
				return errResult("%q has no permanent player code; cannot store an override", in.Player)
			}
			name := el.WebName

			now := time.Now().Format("2006-01-02")
			saved := t.savedTo(mode)
			if mode == "minutes" && in.Minutes == nil {
				return errResult("mode 'minutes' needs expected_minutes — what he actually " +
					"plays per gameweek")
			}
			if err := t.updateConfig(func(cfg *config.Config) error {
				return cfg.Roster.Set(mode, config.RosterOverride{
					Code: el.Code, Name: name, Reason: in.Reason,
					SetOn: now, LastChecked: now, UntilGameweek: in.Until,
					ExpectedMinutes: in.Minutes,
				}, in.Confirmed)
			}); err != nil {
				return errResult("%v", err)
			}
			// The engine is scoring off this right now, so install it here too
			// rather than waiting for the next run.
			//
			// Through the guarded setter, NOT by writing the map. This handler
			// runs concurrently with every other tool in the turn — the runner
			// fans them out through an errgroup — and the scoring path reads the
			// same maps. The original bare write was a `fatal error: concurrent
			// map writes` waiting for the second override in a turn, which the
			// prompt actively asks for, and Go does not let a program recover
			// from that.
			if mode == "minutes" {
				// Read back what Roster.Set just resolved Confirmed to, rather
				// than re-deriving the carry-forward rule here: in.Confirmed is
				// only ever a caller's raw, possibly-nil input, and Set already
				// decided the actual value (explicit, or carried forward from
				// the existing override) as the one thing t.Cfg now records.
				// Recomputing it a second time here is exactly the kind of copy
				// this project has seen drift.
				//
				// Taken under t.mu, deliberately a second, separate acquisition
				// from updateConfig's own: the tool runner fans a turn's calls
				// out through an errgroup, so another set_player_status call for
				// a different player can reassign t.Cfg — a whole-struct copy —
				// between this handler's updateConfig call returning and this
				// read. An unguarded read here is the exact torn-read shape
				// minutesOverrideFor exists to prevent on the engine side, just
				// on Cfg instead of the override maps.
				t.mu.Lock()
				confirmed := false
				if existing, ok := t.Cfg.Roster.MinutesFor(el.Code); ok {
					confirmed = existing.Confirmed
				}
				t.mu.Unlock()
				t.Engine.SetMinutesOverride(el.Code, *in.Minutes, in.Until, confirmed)
			} else if mode == "clear" {
				t.Engine.ClearMinutesOverride(el.Code)
			}

			lock, exclude, notes := t.rosterIDs()
			return textResult(map[string]any{
				"player":      name,
				"mode":        mode,
				"persistence": saved,
				"active_now":  notes,
				"counts":      map[string]int{"locked": len(lock), "excluded": len(exclude)},
				"effect": "Binds optimize_squad and suggest_transfers from here on, and " +
					"every future run. Re-run either if you already called it.",
			})
		},
	)
}

// --- research_targets -----------------------------------------------------

type researchTargetsInput struct{}

// researchSquad is the model's own best fifteen, included in research_targets so
// that its non-nailed members are checked too — the recommendation depends on
// them starting.
//
// # It must not touch the shared engine, and it used to
//
// This read `t.Engine.ApplyChipPlan(&req)` before optimising. That call MUTATES
// the receiver: it writes `Engine.Weights.Horizon` and rebuilds
// `Engine.byTeamUpcoming` (see analysis.ApplyChipPlan and buildFixtureIndex).
// The toolbox holds ONE engine for the whole run, so the write was permanent and
// unguarded, and it cost two separate things:
//
//   - **A leak.** Nothing put the horizon back, so every later `optimize_squad`
//     and `suggest_transfers` in the run scored on the shortened horizon and
//     every one asked earlier scored on the full one. The prompt mandates this
//     tool in the standard review procedure, so the answer depended on the order
//     the model happened to call its tools in — one quantity, two values, chosen
//     by the LLM. Measured live at the current gameweek with a wildcard planned
//     three weeks out: horizon 5 became horizon 2 and stayed there.
//   - **A race.** The tool runner fans a turn's calls out through an errgroup —
//     as the `set_player_status` handler below says at length — so this ran
//     beside tools whose scoring path READS `byTeamUpcoming` while
//     `buildFixtureIndex` assigned a fresh map over it. Go does not let a program
//     recover from a concurrent map read and write.
//
// The fix is to not make the call rather than to save and restore around it. A
// save/restore is what the web builder does (cmd/armband/page.go), and it is
// right there because the page is BUILDING the squad the plan describes; it
// still would not close the race, because the siblings read the index during the
// window. This tool's output is a list of players to go and read the news about
// — its own budget resolution is best-effort for exactly that reason — so it has
// no claim on the shared engine's horizon.
//
// # What is dropped, and what is kept
//
// Only the mutating half. `ApplyChipPlan` does two independent things, and the
// bench-boost half — `SuggestBenchWeight`, which returns a number and writes
// nothing to the receiver — is kept and called directly below. Dropping the
// whole call would have thrown that away too, and it fires on a different chip:
// a planned boost inside the horizon is what stops cheap non-playing fodder
// being free, so the reference fifteen would have stopped being the one whose
// bench is worth reading the news about.
//
// ⚠️ **This changes what the tool returns when a wildcard is planned**: the
// reference fifteen is built on the configured horizon rather than the truncated
// one, so different names reach the shortlist.
//
// ⚠️ **And in one case it moves `Score` itself.** `EffectiveHorizon`'s wildcard
// branch returns `gw - nextGW`, whose minimum is 1 — a wildcard planned for the
// very next gameweek. At a horizon of exactly 1 `FixtureLoadInScore()` turns
// true in the shipped configuration, and `Metrics` then applies
// `Score *= FixtureLoad` to every player. So the old code multiplied by fixture
// load in that case and this does not. It is the closest wildcard a plan can
// name and a perfectly ordinary thing to plan, so it is stated rather than
// waved past: the scores this tool's shortlist is drawn from differ from the
// scores it used to draw them from, for that one placement.
//
// Whether `optimize_squad` and `suggest_transfers` SHOULD apply the chip plan is
// a real question and a separate one — they do not call it today, and the
// accident this removes is not an argument that they should.
func (t *Toolbox) researchSquad() []analysis.PlayerMetrics {
	// BenchWeight left at zero so Optimize reads the configured weight, which is
	// what every other squad this project builds is scored on — unless a planned
	// bench boost raises it, below.
	req := analysis.OptimizeRequest{MinMinutes: 600, MinExpectedMinutes: 55}
	// The safe half of ApplyChipPlan, called directly. SuggestBenchWeight reads
	// Weights.BenchWeight and the schedule and returns a number; unlike the
	// horizon half it writes nothing to the shared engine, so it is safe beside
	// the other tools in the turn. The empty reason is the no-op, matching
	// ApplyChipPlan's own guard rather than testing the weight for equality.
	if bw, why := t.Engine.SuggestBenchWeight(t.Engine.Chips); why != "" {
		req.BenchWeight = bw
	}
	// Best effort, unlike everywhere else the budget is resolved. This output is
	// a list of players to go and read the news about, not a squad to buy, so an
	// unpriceable squad should cost the extra names rather than the whole step.
	// Zero leaves Optimize on its default.
	if budget, _, err := t.Engine.AssemblyBudget(); err == nil {
		req.Budget = budget
	}
	sq, err := t.Engine.Optimize(req)
	if err != nil {
		return nil
	}
	return sq.Players
}

func (t *Toolbox) researchTargets() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"research_targets",
		"List the players the model is most likely to be wrong about, grouped by the kind of "+
			"blind spot, so web searches go where they are worth spending. This is the inverse "+
			"of checking team news for players you already like: it names players you would "+
			"never have shortlisted *because* the model scored them wrongly. A promoted club's "+
			"nailed starter scores 0.00 exactly like their fourth-choice keeper, since neither "+
			"has Premier League minutes. Each group carries the reason the model is blind and "+
			"the question to answer. Call it before finalising any recommendation.",
		func(ctx context.Context, _ researchTargetsInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			t.note("research_targets", "where the model is blind")

			squad := t.researchSquad()

			type row struct {
				Name  string  `json:"name"`
				Team  string  `json:"team"`
				Pos   string  `json:"pos"`
				Price float64 `json:"price"`
				Own   float64 `json:"own_pct"`
				Score float64 `json:"score"`
				Mins  int     `json:"mins_last_season"`
			}
			type group struct {
				Blindspot string `json:"blindspot"`
				Why       string `json:"why"`
				Ask       string `json:"ask"`
				Players   []row  `json:"players"`
			}

			var out []group
			for _, c := range t.Engine.ResearchTargets(squad) {
				g := group{Blindspot: c.Name, Why: c.Why, Ask: c.Ask}
				for _, p := range c.Targets {
					g.Players = append(g.Players, row{
						Name: p.Name, Team: p.Team, Pos: p.Position, Price: p.Price,
						Own: p.Ownership, Score: round(p.Score, 2), Mins: p.Minutes,
					})
				}
				out = append(out, g)
			}
			if len(out) == 0 {
				return textResult(map[string]any{
					"targets": []group{},
					"note":    "nothing flagged — check the thresholds rather than assuming there is nothing to learn",
				})
			}
			return textResult(map[string]any{"targets": out})
		},
	)
}

// --- set_price_forecast ---------------------------------------------------

type priceForecastEntry struct {
	Player      string  `json:"player" jsonschema:"description=Player name or FPL id."`
	Direction   string  `json:"direction" jsonschema:"description=rise or fall."`
	Probability float64 `json:"probability,omitempty" jsonschema:"description=Chance of the change tonight, 0-1. Omit if the source gives a flag rather than a number."`
	Source      string  `json:"source" jsonschema:"description=Who says so, e.g. LiveFPL. Recorded so the advice can be audited."`
}

type setPriceForecastInput struct {
	Forecasts []priceForecastEntry `json:"forecasts" jsonschema:"description=Tonight's expected price changes for players you are considering."`
}

func (t *Toolbox) setPriceForecast() (anthropic.BetaTool, error) {
	return toolrunner.NewBetaToolFromJSONSchema(
		"set_price_forecast",
		"Record tonight's expected FPL price changes, which you get by searching a "+
			"third-party estimator (LiveFPL, Fantasy Football Fix and similar publish "+
			"a percentage chance per player). This does NOT change any player's score — "+
			"a rising price says nothing about how many points he returns. It changes "+
			"WHEN to act: if you are going to make a transfer anyway, making it before "+
			"a rise saves the rise, and before a fall saves the fall. "+
			"Only worth doing for players you are actually considering. "+
			"Not saved between runs: a forecast is true for one evening.",
		func(ctx context.Context, in setPriceForecastInput) (anthropic.BetaToolResultBlockParamContentUnion, error) {
			t.note("set_price_forecast", fmt.Sprintf("%d players", len(in.Forecasts)))
			out := map[int]analysis.PriceForecast{}
			var recorded, unknown []string
			for _, f := range in.Forecasts {
				matches := t.Engine.Boot.FindPlayers(f.Player)
				if len(matches) == 0 {
					unknown = append(unknown, f.Player)
					continue
				}
				dir := strings.ToLower(strings.TrimSpace(f.Direction))
				if dir != "rise" && dir != "fall" {
					unknown = append(unknown, f.Player+" (direction must be rise or fall)")
					continue
				}
				el := matches[0]
				out[el.ID] = analysis.PriceForecast{
					Direction: dir, Probability: f.Probability, Source: f.Source,
				}
				recorded = append(recorded, fmt.Sprintf("%s: %s", el.WebName,
					out[el.ID].Note()))
			}
			t.Engine.SetPriceForecasts(out)
			return textResult(map[string]any{
				"recorded": recorded,
				"unknown":  unknown,
				"note": "Recorded for this run only. Use it to decide whether to act " +
					"tonight rather than at the deadline; it does not change any score.",
			})
		})
}

// pastSeasonsForTool renders history_past with the statistics FPL never measured
// OMITTED rather than reported as zero.
//
// FPL returns `"0.00"` for expected goals, assists and goals conceded in every
// season before 2022/23, and for defensive contribution before 2024/25, beside
// the player's real minutes. Handed to the agent unaltered, a centre-half with
// 3,151 minutes in 2018/19 arrives carrying `expected_goals_conceded: "0.00"` —
// a number, not a gap — and there is nothing in the payload to tell the model it
// is an absence. The failure mode is a confident sentence about an elite
// defensive season that never happened.
//
// This is the same defect as the one analysis.PriorSeasonStats.NoXG/NoXGC/NoDefCon exist
// for, one layer out and on the worse path: that one is inert until
// prior_half_life is turned on, and this one is live on every `seasons: true`
// lookup. Tool output is also replayed on every subsequent API call, so a wrong
// number here is paid for repeatedly.
//
// Omitted rather than zeroed or flagged because a missing key is the one encoding
// an LLM cannot misread as a measurement.
func pastSeasonsForTool(past []fpl.PastSeason) []map[string]any {
	out := make([]map[string]any, 0, len(past))
	for _, s := range past {
		b, err := json.Marshal(s)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}
		if !s.HasExpected() {
			for _, k := range []string{"expected_goals", "expected_assists",
				"expected_goal_involvements", "expected_goals_conceded"} {
				delete(m, k)
			}
		}
		if !s.HasDefCon() {
			delete(m, "defensive_contribution")
		}
		out = append(out, m)
	}
	return out
}
