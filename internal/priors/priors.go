// Package priors loads a completed season's player totals, so the model has
// something to fall back on when FPL's own aggregates reset.
//
// # Why this exists
//
// FPL's bootstrap carries last season's totals only until GW1 completes, then
// overwrites them with this season's running figures. That is fine at the end of
// a season and useless at the start of one: after two gameweeks the model would
// be reasoning from two matches with no memory of the thirty-eight before them.
//
// The fix is a prior — last season as a baseline that current-season data
// displaces as the sample grows. That requires having last season after FPL has
// thrown it away, which means capturing it from somewhere else.
//
// # Source
//
// github.com/olbauday/FPL-Core-Insights, a public dataset of FPL API snapshots
// taken every gameweek and published as CSV. Two properties make it usable where
// a scrape would not be:
//
//   - It is keyed by FPL's own identifiers. players.csv carries player_code,
//     which is stable across seasons — unlike the element id, which FPL
//     reassigns every summer. So the join is by integer key, never by name.
//     Name joins across football data sources are how one player's numbers end
//     up attributed to another, and this codebase has been bitten by name
//     matching within a single source already.
//
//   - Its columns are FPL's own field names, so the values are the model's
//     actual inputs rather than a third party's reinterpretation of them.
//
// Verified against the live API: Haaland's final 2025-26 row reads 2,953
// minutes, 27 goals and 25.50 xG, matching FPL's bootstrap exactly.
//
// # What this needs is PAST data, which is why it is a snapshot and not a feed
//
// The prior is always a season that has finished, and a finished season is
// immutable. So a fetch happens once per season and then never again: Load
// caches indefinitely, has no staleness check of any kind, and reaches the
// network only when the file is absent. If upstream disappears tomorrow,
// everything already fetched keeps working, and the only exposure left is a
// fresh environment bootstrapping a season it has never seen.
//
// Data going forward is a different problem with its own solution, and it does
// not come from here: `internal/capture` takes dated `bootstrap-static` payloads
// on a schedule and `internal/wayback` recovers historical ones. Do not grow
// this package towards a live feed — that is what those are for.
//
// ⚠️ Corrected 2026-08-16. This section used to justify itself with "it is a
// single-maintainer repository, and the previous community archive
// (vaastav/Fantasy-Premier-League) stopped weekly updates after 2024-25". That
// was false, and it was load-bearing: it was the stated reason for depending on
// this source at all. Checked against the upstream repository —
// `data/2025-26/gws/merged_gw.csv` serves 200 at 5,445,456 bytes, and the last
// commits touching `data/2025-26` are "Add gw38 data" and "Final 2025-26
// update", both 2026-06-17. That archive is current. The argument now rests on
// immutability, which is a property of the data rather than a guess about how
// long a maintainer will keep going.
package priors

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"armband/internal/analysis"
)

const sourceURL = "https://raw.githubusercontent.com/olbauday/FPL-Core-Insights/main/data"

// Player is one player's end-of-season totals, as FPL reported them on the
// final gameweek.
type Player struct {
	// Code is FPL's permanent player identifier, stable across seasons. The
	// element id is not, so never key on that.
	Code    int    `json:"code"`
	WebName string `json:"web_name"`
	Team    int    `json:"team_code"`
	Pos     string `json:"position"`

	Minutes int `json:"minutes"`
	Starts  int `json:"starts"`

	Goals         int `json:"goals_scored"`
	Assists       int `json:"assists"`
	CleanSheets   int `json:"clean_sheets"`
	GoalsConceded int `json:"goals_conceded"`
	Saves         int `json:"saves"`
	Bonus         int `json:"bonus"`
	YellowCards   int `json:"yellow_cards"`
	RedCards      int `json:"red_cards"`
	TotalPoints   int `json:"total_points"`

	XG  float64 `json:"expected_goals"`
	XA  float64 `json:"expected_assists"`
	XGC float64 `json:"expected_goals_conceded"`

	DefCon    int `json:"defensive_contribution"`
	Tackles   int `json:"tackles"`
	CBI       int `json:"clearances_blocks_interceptions"`
	Recovered int `json:"recoveries"`

	// PenaltiesOrder is the set-piece rank on the final gameweek, 0 for none.
	// Held across seasons it is the only way to see a duty change hands.
	PenaltiesOrder int `json:"penalties_order"`

	// Gameweeks is how many gameweek snapshots this player appeared in, which
	// says how much of the season the totals actually cover.
	Gameweeks int `json:"gameweeks"`
}

// Season is a completed season's totals, keyed by player code.
type Season struct {
	Name    string          `json:"season"`
	Fetched time.Time       `json:"fetched"`
	Players map[int]*Player `json:"players"`
}

// ThinSeason is the minutes below which a season stops being trusted on its own
// and older ones are blended in. Half a season.
//
// An alias, not a second declaration: the bar is analysis.ThinSeason, and the
// gate that reads it is analysis.ShouldBlendPrior. Three packages express this
// rule and all three import internal/analysis, so there is exactly one number
// and one predicate to change.
const ThinSeason = analysis.ThinSeason

// LoadBlended loads several completed seasons, most recent first, and collapses
// them into one prior.
//
// Only players whose most recent season is thin AND non-zero reach back — see
// analysis.ShouldBlendPrior for both halves of that gate. Seasons that cannot be
// fetched are skipped rather than failing the load, so a missing archive degrades
// to what is available.
func LoadBlended(ctx context.Context, cacheDir string, seasons []string, halfLife float64) (*Season, error) {
	if len(seasons) == 0 {
		return nil, fmt.Errorf("no seasons given")
	}
	first, err := Load(ctx, cacheDir, seasons[0])
	if err != nil {
		return nil, err
	}
	if halfLife <= 0 || len(seasons) == 1 {
		return first, nil
	}

	loaded := []*Season{first}
	for _, name := range seasons[1:] {
		s, err := Load(ctx, cacheDir, name)
		if err != nil {
			continue // an older season is optional
		}
		loaded = append(loaded, s)
	}
	if len(loaded) == 1 {
		return first, nil
	}

	// Capability is a fact about each season, so probe once per season rather than
	// once per player: each probe is an O(players) scan, and inside the loop below
	// that would be quadratic. backtest's mirror of this hoists it the same way.
	type seasonCap struct{ noXG, noXGC, noDefCon bool }
	caps := make([]seasonCap, len(loaded))
	for i, s := range loaded {
		caps[i] = seasonCap{
			noXG:     !s.has(func(p *Player) bool { return p.XG != 0 }),
			noXGC:    !s.has(func(p *Player) bool { return p.XGC != 0 }),
			noDefCon: !s.has(func(p *Player) bool { return p.DefCon != 0 }),
		}
	}

	out := &Season{Name: first.Name + " (blended)", Fetched: first.Fetched,
		Players: map[int]*Player{}}
	for code, p := range first.Players {
		// The gate. A full season stands alone, and a season of no minutes at
		// all is handed on untouched — carrying Minutes 0 is what routes him to
		// shrinkToLeague downstream, which is the shipped answer for a player
		// with no usable history and a better one than a two-year-old season.
		if !analysis.ShouldBlendPrior(p.Minutes) {
			out.Players[code] = p
			continue
		}
		var hist []analysis.PriorSeasonStats
		for i, s := range loaded {
			q, ok := s.Get(code)
			if !ok || q.Minutes == 0 {
				continue
			}
			hist = append(hist, analysis.PriorSeasonStats{
				// The same projection Adapter.Get hands the live path, so the blended
				// prior and the unblended one cannot disagree about what a field means.
				// It was a second copy of the field list here — see priorFrom.
				PriorPlayer: priorFrom(q),
				SeasonsAgo:  i,
				// Measured from what this mirror actually published for the season,
				// not from its name — the same rule the archive path follows, and
				// necessary here for a different reason: this source's coverage is
				// its own, so neither FPL's boundaries nor the archive's describe it.
				NoXG:     caps[i].noXG,
				NoXGC:    caps[i].noXGC,
				NoDefCon: caps[i].noDefCon,
			})
		}
		if len(hist) < 2 {
			out.Players[code] = p
			continue
		}
		b := analysis.BlendPriors(hist, halfLife)
		merged := *p
		merged.Minutes, merged.Starts = b.Minutes, b.Starts
		merged.XG, merged.XA, merged.XGC = b.XG, b.XA, b.XGC
		merged.DefCon, merged.Bonus, merged.Saves = b.DefCon, b.Bonus, b.Saves
		merged.YellowCards, merged.RedCards = b.Yellow, b.Red
		out.Players[code] = &merged
	}
	return out, nil
}

// has reports whether any player in the season satisfies f — the season-level
// capability probe. See backtest.Season.HasXG for why this is asked of the season
// rather than of the player: per player it would flag every keeper as having no
// expected goals.
func (s *Season) has(f func(*Player) bool) bool {
	if s == nil {
		return false
	}
	for _, p := range s.Players {
		if f(p) {
			return true
		}
	}
	return false
}

// Get returns the player, and whether he was found.
func (s *Season) Get(code int) (*Player, bool) {
	if s == nil {
		return nil, false
	}
	p, ok := s.Players[code]
	return p, ok
}

// Load returns a completed season's totals, from cache when present.
//
// A finished season does not change, so the cache has no expiry: once written
// it is authoritative and the network is never touched again. That is
// deliberate — see the package comment on treating the source as a snapshot.
func Load(ctx context.Context, cacheDir, season string) (*Season, error) {
	cachePath := filepath.Join(cacheDir, "priors-"+season+".json")
	if b, err := os.ReadFile(cachePath); err == nil {
		var s Season
		if err := json.Unmarshal(b, &s); err == nil && len(s.Players) > 0 {
			return &s, nil
		}
	}

	s, err := fetch(ctx, season)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err == nil {
		if b, err := json.Marshal(s); err == nil {
			_ = os.WriteFile(cachePath, b, 0o644)
		}
	}
	return s, nil
}

// fetch downloads and joins the two files that make up a season.
func fetch(ctx context.Context, season string) (*Season, error) {
	// players.csv is the join table: it carries both the season's element id and
	// the permanent player_code.
	codeByID, meta, err := loadPlayers(ctx, season)
	if err != nil {
		return nil, err
	}
	return loadStats(ctx, season, codeByID, meta)
}

func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "armband/1.0 (+personal FPL analysis)")
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return resp.Body, nil
}

func loadPlayers(ctx context.Context, season string) (map[int]int, map[int]*Player, error) {
	body, err := get(ctx, fmt.Sprintf("%s/%s/players.csv", sourceURL, season))
	if err != nil {
		return nil, nil, err
	}
	defer body.Close()

	rows := csv.NewReader(body)
	head, err := rows.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("players.csv: %w", err)
	}
	col := index(head)
	for _, want := range []string{"player_code", "player_id", "web_name"} {
		if _, ok := col[want]; !ok {
			return nil, nil, fmt.Errorf("players.csv has no %q column; the upstream schema has changed", want)
		}
	}

	codeByID := map[int]int{}
	meta := map[int]*Player{}
	for {
		rec, err := rows.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("players.csv: %w", err)
		}
		code, id := atoi(rec, col, "player_code"), atoi(rec, col, "player_id")
		if code == 0 || id == 0 {
			continue
		}
		codeByID[id] = code
		meta[code] = &Player{
			Code:    code,
			WebName: str(rec, col, "web_name"),
			Team:    atoi(rec, col, "team_code"),
			Pos:     str(rec, col, "position"),
		}
	}
	return codeByID, meta, nil
}

// loadStats walks the per-gameweek snapshots and keeps the latest row for each
// player. The rows are cumulative — after GW6 Haaland's row reads 503 minutes,
// not the 90 he played that week — so the final row is the season total.
func loadStats(ctx context.Context, season string, codeByID map[int]int, meta map[int]*Player) (*Season, error) {
	body, err := get(ctx, fmt.Sprintf("%s/%s/playerstats.csv", sourceURL, season))
	if err != nil {
		return nil, err
	}
	defer body.Close()

	rows := csv.NewReader(body)
	rows.FieldsPerRecord = -1
	head, err := rows.Read()
	if err != nil {
		return nil, fmt.Errorf("playerstats.csv: %w", err)
	}
	col := index(head)
	for _, want := range []string{"id", "gw", "minutes", "expected_goals"} {
		if _, ok := col[want]; !ok {
			return nil, fmt.Errorf("playerstats.csv has no %q column; the upstream schema has changed", want)
		}
	}

	out := &Season{Name: season, Fetched: time.Now(), Players: map[int]*Player{}}
	latest := map[int]int{} // code -> highest gameweek seen

	for {
		rec, err := rows.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("playerstats.csv: %w", err)
		}
		code, ok := codeByID[atoi(rec, col, "id")]
		if !ok {
			continue
		}
		gw := atoi(rec, col, "gw")

		p := out.Players[code]
		if p == nil {
			p = &Player{Code: code}
			if m := meta[code]; m != nil {
				*p = *m
			}
			out.Players[code] = p
		}
		p.Gameweeks++
		if gw < latest[code] {
			continue
		}
		latest[code] = gw

		p.Minutes = atoi(rec, col, "minutes")
		p.Starts = atoi(rec, col, "starts")
		p.Goals = atoi(rec, col, "goals_scored")
		p.Assists = atoi(rec, col, "assists")
		p.CleanSheets = atoi(rec, col, "clean_sheets")
		p.GoalsConceded = atoi(rec, col, "goals_conceded")
		p.Saves = atoi(rec, col, "saves")
		p.Bonus = atoi(rec, col, "bonus")
		p.YellowCards = atoi(rec, col, "yellow_cards")
		p.RedCards = atoi(rec, col, "red_cards")
		p.TotalPoints = atoi(rec, col, "total_points")
		p.XG = atof(rec, col, "expected_goals")
		p.XA = atof(rec, col, "expected_assists")
		p.XGC = atof(rec, col, "expected_goals_conceded")
		p.DefCon = atoi(rec, col, "defensive_contribution")
		p.Tackles = atoi(rec, col, "tackles")
		p.CBI = atoi(rec, col, "clearances_blocks_interceptions")
		p.Recovered = atoi(rec, col, "recoveries")
		p.PenaltiesOrder = atoi(rec, col, "penalties_order")
	}

	if len(out.Players) == 0 {
		return nil, fmt.Errorf("no players joined for %s; check the upstream schema", season)
	}
	return out, nil
}

func index(head []string) map[string]int {
	m := make(map[string]int, len(head))
	for i, h := range head {
		m[h] = i
	}
	return m
}

func str(rec []string, col map[string]int, name string) string {
	if i, ok := col[name]; ok && i < len(rec) {
		return rec[i]
	}
	return ""
}

func atoi(rec []string, col map[string]int, name string) int {
	// Values arrive as "12", "12.0" or empty, so parse via float.
	return int(atof(rec, col, name))
}

func atof(rec []string, col map[string]int, name string) float64 {
	s := str(rec, col, name)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
